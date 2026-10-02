package shape

import (
	"context"
	"errors"
	"fmt"
)

// ErrRunLimit identifies a rejected input, glyph expansion or shaping work limit.
var ErrRunLimit = errors.New("shape: run limit exceeded")

// RunLimits bounds one ShapeGlyphsContext call. Zero fields select defaults;
// negative fields are invalid. MaxWork counts conservative lookup work units,
// including subtable bytes and the glyphs a lookup can inspect. It does not
// replace the font parser's own budgets or interrupt an individual font read.
type RunLimits struct {
	MaxInputBytes int
	MaxGlyphs     int
	MaxWork       int64
}

// RunInput describes a run and its logical neighbours. The merging, context and
// feature semantics are those of ShapeGlyphsMerged.
type RunInput struct {
	Text, Before, After, MergeBefore, MergeAfter string
	Kerns                                        bool
	Features                                     Features
}

// RunResult contains a completed run and its charged lookup work. Work can be
// deducted from a document-wide budget by the caller. On error the result is zero.
type RunResult struct {
	Glyphs  []Glyph
	Missing int
	Work    int64
}

type runWork struct {
	ctx    context.Context
	glyphs int
	left   int64
	// layouts are the ones this call shaped through, whose limits are the
	// call's own. See layoutLimits.
	layouts []*layout
	input   int
}
type runAbort struct{ err error }

// spend charges n units of work, and size checks a run about to hold n glyphs.
// Both are called in the innermost loops of every shaping pass, bounded or not,
// so each is only the nil check, small enough to be inlined: the unbounded
// entry points pay a comparison, not a call.
func (w *runWork) spend(n int64) {
	if w != nil {
		w.charge(n)
	}
}

func (w *runWork) size(n int) {
	if w != nil {
		w.hold(n)
	}
}

func (w *runWork) charge(n int64) {
	if err := w.ctx.Err(); err != nil {
		panic(runAbort{err})
	}
	if n < 0 || n > w.left {
		panic(runAbort{fmt.Errorf("%w: lookup work", ErrRunLimit)})
	}
	w.left -= n
}

// work is the budget the shaper's face is shaping under, nil for an unbounded
// one or a shaper built without a face.
func (sh shaper) work() *runWork {
	if sh.f == nil {
		return nil
	}
	return sh.f.runWork
}

// shapedThrough records a layout a run was shaped with.
func (w *runWork) shapedThrough(l *layout) {
	if w == nil || l == nil {
		return
	}
	for _, seen := range w.layouts {
		if seen == l {
			return
		}
	}
	w.layouts = append(w.layouts, l)
}

// layoutLimits are the bounds reading the layouts this call shaped through ran
// into, and those of the face's own layout.
//
// They are not the whole of LayoutLimits. That answer is the face's history,
// shared by every clone: the layouts of scripts other calls have set, and the
// ink of glyphs other calls have measured. Rejecting on it would refuse a run
// for a glyph it never draws, and accept or refuse the same run depending on
// what had been shaped before it.
func (w *runWork) layoutLimits(f *Face) []string {
	var out []string
	if f.layout != nil {
		out = append(out, f.layout.limits...)
	}
	for _, l := range w.layouts {
		if l != f.layout {
			out = append(out, l.limits...)
		}
	}
	return out
}

func (w *runWork) hold(n int) {
	w.charge(0)
	if n < 0 || n > w.glyphs {
		panic(runAbort{fmt.Errorf("%w: glyph count", ErrRunLimit)})
	}
}

// ShapeGlyphsContext shapes a bounded run without changing the receiver's used
// glyph record. It returns no glyphs on cancellation, work exhaustion, glyph
// expansion, recursion exhaustion, or a limit reading the layout tables the run
// was shaped with ran into; bounds other runs met on the face do not count. A
// private clone shares the receiver's locked font caches. Font programs must remain
// immutable during calls. Cancellation is checked between shaping phases and
// lookup steps; font parsing and Unicode preprocessing use their own bounds.
func (f *Face) ShapeGlyphsContext(ctx context.Context, in RunInput, limits RunLimits) (result RunResult, err error) {
	if ctx == nil || f == nil {
		return RunResult{}, errors.New("shape: nil context or face")
	}
	if limits.MaxInputBytes < 0 || limits.MaxGlyphs < 0 || limits.MaxWork < 0 {
		return RunResult{}, errors.New("shape: negative run limit")
	}
	if limits.MaxInputBytes == 0 {
		limits.MaxInputBytes = 4096
	}
	if limits.MaxGlyphs == 0 {
		limits.MaxGlyphs = 32768
	}
	if limits.MaxWork == 0 {
		limits.MaxWork = 64 << 20
	}
	left := limits.MaxInputBytes
	for _, s := range []string{in.Text, in.Before, in.After, in.MergeBefore, in.MergeAfter, in.Features.Tags, in.Features.TagsOff, in.Features.Language, f.settingsOn, f.settingsOff} {
		if len(s) > left {
			return RunResult{}, fmt.Errorf("%w: input bytes", ErrRunLimit)
		}
		left -= len(s)
	}
	if err := ctx.Err(); err != nil {
		return RunResult{}, err
	}
	clone := f.Clone()
	clone.runWork = &runWork{ctx: ctx, glyphs: limits.MaxGlyphs, left: limits.MaxWork}
	// Only the private bounded-work signal is caught. Programming errors retain
	// their ordinary panic behaviour, including those in the legacy entry points.
	defer func() {
		if p := recover(); p != nil {
			if stop, ok := p.(runAbort); ok {
				result = RunResult{}
				err = stop.err
			} else {
				panic(p)
			}
		}
	}()
	result.Glyphs, result.Missing = clone.ShapeGlyphsMerged(in.Text, in.Before, in.After, in.MergeBefore, in.MergeAfter, in.Kerns, in.Features)
	clone.runWork.size(len(result.Glyphs))
	if findings := clone.runWork.layoutLimits(clone); len(findings) != 0 {
		return RunResult{}, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
	}
	result.Work = limits.MaxWork - clone.runWork.left
	return result, nil
}

// WithShapingLimits runs synchronous shaping/measurement through a private face
// clone under one shared work budget. This allows paragraph layout to use the
// legacy measurement APIs without silently accepting runtime lookup exhaustion.
// Input limits apply to each measured/shaped run, including its context/settings.
// The callback must use the supplied face on one goroutine, must not clone it,
// and must bound its own non-shaping work and check cancellation between phases.
// It must not retain the face for later shaping. Returned work is zero on error.
// Individual font reads and Unicode preprocessing retain their own bounds.
func (f *Face) WithShapingLimits(ctx context.Context, limits RunLimits, fn func(*Face) error) (work int64, err error) {
	if f == nil || ctx == nil || fn == nil {
		return 0, errors.New("shape: nil shaping scope input")
	}
	if limits.MaxInputBytes < 0 || limits.MaxGlyphs < 0 || limits.MaxWork < 0 {
		return 0, errors.New("shape: negative run limit")
	}
	if limits.MaxInputBytes == 0 {
		limits.MaxInputBytes = 4096
	}
	if limits.MaxGlyphs == 0 {
		limits.MaxGlyphs = 32768
	}
	if limits.MaxWork == 0 {
		limits.MaxWork = 64 << 20
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	clone := f.Clone()
	clone.runWork = &runWork{ctx: ctx, glyphs: limits.MaxGlyphs, left: limits.MaxWork, input: limits.MaxInputBytes}
	defer func() {
		if p := recover(); p != nil {
			if stop, ok := p.(runAbort); ok {
				work = 0
				err = stop.err
			} else {
				panic(p)
			}
		}
	}()
	if err = fn(clone); err != nil {
		return 0, err
	}
	clone.runWork.spend(0)
	if findings := clone.LayoutLimits(); len(findings) != 0 {
		return 0, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
	}
	return limits.MaxWork - clone.runWork.left, nil
}

func (w *runWork) checkInput(f *Face, s string, extra []string, ctx shapeContext) {
	if w == nil || w.input == 0 {
		return
	}
	w.spend(0)
	left := w.input
	check := func(s string) {
		if len(s) > left {
			panic(runAbort{fmt.Errorf("%w: input bytes", ErrRunLimit)})
		}
		left -= len(s)
	}
	for _, s := range []string{s, ctx.before, ctx.after, ctx.mergeBefore, ctx.mergeAfter, ctx.features.Tags, ctx.features.TagsOff, ctx.features.Language, f.settingsOn, f.settingsOff} {
		check(s)
	}
	for _, tag := range extra {
		check(tag)
	}
}
