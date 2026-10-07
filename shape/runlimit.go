package shape

import (
	"context"
	"errors"
	"fmt"
)

// ErrRunLimit identifies a rejected input, glyph expansion or shaping work limit.
var ErrRunLimit = errors.New("shape: run limit exceeded")

// RunLimits bounds one ShapeGlyphsContext or ShapeGlyphsBounded call. Zero fields select defaults;
// negative fields are invalid. It does not replace the font parser's own
// budgets or interrupt an individual font read.
//
// MaxWork counts the lookup work the run does: a unit for each lookup at each
// position, each subtable tried, each ligature and each rule a subtable then
// tries, and each glyph a match or a search for a base steps over. It is what
// is read and not what could have been: a subtable was charged its size at
// every position whether or not it covered the glyph, and the default refused
// 140 bytes of Latin and 8 of Devanagari. Real text costs tens of units a
// byte, and a few hundred in the costliest scripts and fonts — Noto Sans's
// Devanagari and Noto Nastaliq Urdu — so the defaults admit MaxInputBytes of
// any of them with fifty times to spare, and a font's runaway rules are still
// stopped where the units they spend run out.
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
	// input bounds the bytes of each run shaped in a WithShapingLimits scope,
	// zero where the caller checked the one run itself.
	input int
	// layouts are the ones this call shaped through, whose limits are the
	// call's own. See layoutLimits.
	layouts []*layout
	// untilCtx counts down the charges to the next ask of ctx; see ctxEvery.
	// Zero, as a new budget has it, asks at the first.
	untilCtx int
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

// ctxEvery is how many charges pass between asks of the context. Asking is a
// call through an interface, and for a cancellable context a lock, at every
// lookup step: about as much again as the step it guards, on a short run.
// Sixty-four steps are microseconds, which is as soon as a cancellation needs
// noticing.
const ctxEvery = 64

// checkCtx asks the context now, at a boundary between phases, rather than
// when the count next comes round.
func (w *runWork) checkCtx() {
	w.untilCtx = 0
	w.charge(0)
}

func (w *runWork) charge(n int64) {
	if w.untilCtx--; w.untilCtx <= 0 {
		w.untilCtx = ctxEvery
		if err := w.ctx.Err(); err != nil {
			panic(runAbort{err})
		}
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

// withDefaults is the limits with each zero field at its default, or an error
// for a negative one.
func (limits RunLimits) withDefaults() (RunLimits, error) {
	if limits.MaxInputBytes < 0 || limits.MaxGlyphs < 0 || limits.MaxWork < 0 {
		return RunLimits{}, errors.New("shape: negative run limit")
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
	return limits, nil
}

// ShapeGlyphsContext shapes a bounded run without changing the receiver's used
// glyph record. It returns no glyphs on cancellation, work exhaustion, glyph
// expansion, recursion exhaustion, or a limit reading the layout tables the run
// was shaped with ran into; bounds other runs met on the face do not count. A
// private clone shares the receiver's locked font caches. Font programs must remain
// immutable during calls. Cancellation is checked between shaping phases and
// lookup steps; font parsing and Unicode preprocessing use their own bounds.
//
// It clones the face on every call. A caller shaping many runs, which keeps
// clones of its own already, saves that with ShapeGlyphsBounded.
func (f *Face) ShapeGlyphsContext(ctx context.Context, in RunInput, limits RunLimits) (RunResult, error) {
	if ctx == nil || f == nil {
		return RunResult{}, errors.New("shape: nil context or face")
	}
	if err := f.checkRun(ctx, in, &limits); err != nil {
		return RunResult{}, err
	}
	return f.Clone().shapeBounded(ctx, in, limits)
}

// ShapeGlyphsBounded is ShapeGlyphsContext on the receiver itself, for a caller
// that owns the face: one of its own clones, used on one goroutine at a time,
// as ShapeGlyphs is. It shapes under the same limits and fails the same ways,
// but makes no clone, and reuses its budget from one call to the next, so a run
// costs what ShapeGlyphs costs it and the charging of work.
//
// The glyphs it shapes are recorded on the receiver as ShapeGlyphs records
// them, for the caller to merge as it does theirs. A run that fails may have
// recorded some before it stopped. It is an error to call it on a face that is
// already shaping under a budget, such as the one WithShapingLimits hands out.
func (f *Face) ShapeGlyphsBounded(ctx context.Context, in RunInput, limits RunLimits) (RunResult, error) {
	if ctx == nil || f == nil {
		return RunResult{}, errors.New("shape: nil context or face")
	}
	if f.runWork != nil {
		return RunResult{}, errors.New("shape: face is already shaping under a budget")
	}
	if err := f.checkRun(ctx, in, &limits); err != nil {
		return RunResult{}, err
	}
	return f.shapeBounded(ctx, in, limits)
}

// checkRun settles the limits' defaults and refuses, before any shaping, a run
// whose input is over them or whose context is already done.
func (f *Face) checkRun(ctx context.Context, in RunInput, limits *RunLimits) error {
	settled, err := limits.withDefaults()
	if err != nil {
		return err
	}
	*limits = settled
	left := limits.MaxInputBytes
	for _, s := range []string{in.Text, in.Before, in.After, in.MergeBefore, in.MergeAfter, in.Features.Tags, in.Features.TagsOff, in.Features.Language, f.settingsOn, f.settingsOff} {
		if len(s) > left {
			return fmt.Errorf("%w: input bytes", ErrRunLimit)
		}
		left -= len(s)
	}
	return ctx.Err()
}

// shapeBounded shapes one run on f under the limits, which checkRun has
// settled. The budget is f's spare one, reset, and is off the face again when
// the call returns, whether it shaped or stopped.
func (f *Face) shapeBounded(ctx context.Context, in RunInput, limits RunLimits) (result RunResult, err error) {
	w := f.spareWork
	if w == nil {
		w = &runWork{}
		f.spareWork = w
	}
	*w = runWork{ctx: ctx, glyphs: limits.MaxGlyphs, left: limits.MaxWork, layouts: w.layouts[:0]}
	f.runWork = w
	// Only the private bounded-work signal is caught. Programming errors retain
	// their ordinary panic behaviour, including those in the legacy entry points.
	defer func() {
		f.runWork = nil
		// The context is the caller's, and is not kept past its call.
		w.ctx = nil
		if p := recover(); p != nil {
			if stop, ok := p.(runAbort); ok {
				result = RunResult{}
				err = stop.err
			} else {
				panic(p)
			}
		}
	}()
	result.Glyphs, result.Missing = f.ShapeGlyphsMerged(in.Text, in.Before, in.After, in.MergeBefore, in.MergeAfter, in.Kerns, in.Features)
	w.size(len(result.Glyphs))
	if findings := w.layoutLimits(f); len(findings) != 0 {
		return RunResult{}, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
	}
	result.Work = limits.MaxWork - w.left
	return result, nil
}

// WithShapingLimits runs synchronous shaping/measurement through a private face
// clone under one shared work budget. This allows paragraph layout to use the
// legacy measurement APIs without silently accepting runtime lookup exhaustion.
// Input limits apply to each measured/shaped run, including its context/settings.
// The callback must use the supplied face on one goroutine, must not clone it,
// and must bound its own non-shaping work and check cancellation between phases.
// It must not retain the face for later shaping; one that is kept shapes
// unbounded once the call returns. Returned work is zero on error. A scope is
// refused for limits reading the layouts it shaped through ran into, as
// ShapeGlyphsContext is.
// Individual font reads and Unicode preprocessing retain their own bounds.
func (f *Face) WithShapingLimits(ctx context.Context, limits RunLimits, fn func(*Face) error) (work int64, err error) {
	if f == nil || ctx == nil || fn == nil {
		return 0, errors.New("shape: nil shaping scope input")
	}
	if limits, err = limits.withDefaults(); err != nil {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	clone := f.Clone()
	clone.runWork = &runWork{ctx: ctx, glyphs: limits.MaxGlyphs, left: limits.MaxWork, input: limits.MaxInputBytes}
	defer func() {
		// A face kept past the scope, against its contract, shapes as an
		// unbounded clone does, rather than panicking where nothing recovers
		// once the scope's context is done.
		clone.runWork = nil
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
	clone.runWork.checkCtx()
	if findings := clone.runWork.layoutLimits(clone); len(findings) != 0 {
		return 0, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
	}
	return limits.MaxWork - clone.runWork.left, nil
}

func (w *runWork) checkInput(f *Face, s string, extra []string, ctx shapeContext) {
	if w == nil || w.input == 0 {
		return
	}
	w.checkCtx()
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

// ShapingBudget is one budget of shaping work, under a context, shared by
// every face put under it: what a document costs to set, rather than what each
// of its runs costs alone. It is WithShapingLimits for a caller whose shaping
// and measuring goes through more faces than one, and through code that does
// not take a face as an argument — layout's, which finds its faces as it goes.
//
// Each run shaped or measured on a face under it is held to MaxInputBytes and
// MaxGlyphs, as a run under WithShapingLimits is, and the work they all do is
// charged to the one MaxWork. A budget is used on one goroutine, by Run.
type ShapingBudget struct {
	limits RunLimits
	work   *runWork
	faces  []*Face
	done   bool
}

// NewShapingBudget is a budget under ctx, with the limits' zero fields at
// their defaults. It is an error for a nil context, a negative limit, or a
// context already done.
func NewShapingBudget(ctx context.Context, limits RunLimits) (*ShapingBudget, error) {
	if ctx == nil {
		return nil, errors.New("shape: nil context")
	}
	limits, err := limits.withDefaults()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &ShapingBudget{limits: limits,
		work: &runWork{ctx: ctx, glyphs: limits.MaxGlyphs, left: limits.MaxWork, input: limits.MaxInputBytes}}, nil
}

// Bound puts a face the caller owns under the budget, until Run returns:
// everything shaped or measured on it is charged to the budget and held to its
// limits. A face already under this budget is left as it is. It is an error
// for a face under another budget, or a scope's, and once Run has returned.
//
// The face is the caller's to own, as ShapeGlyphsBounded's is — its own
// clone, not one shared with other goroutines — since the budget is written to
// by everything shaped on it.
func (b *ShapingBudget) Bound(f *Face) error {
	switch {
	case b == nil || f == nil:
		return errors.New("shape: nil budget or face")
	case b.done:
		return errors.New("shape: the budget has been spent")
	case f.runWork == b.work:
		return nil
	case f.runWork != nil:
		return errors.New("shape: face is already shaping under a budget")
	}
	f.runWork = b.work
	b.faces = append(b.faces, f)
	return nil
}

// Run calls fn, which shapes and measures on faces put under the budget —
// before it is called or while it runs — and reports the work they did.
//
// A run that reaches a limit, or finds the context done, stops fn where it
// is, and Run reports why: an error wrapping ErrRunLimit, or the context's.
// So does reading a layout table the faces shaped with running into one of
// the font's own limits, as for WithShapingLimits. fn's own error is returned
// as it is. Work is zero on any error. Every face is taken off the budget when
// Run returns, and shapes unbounded after; the budget is spent, and Run may
// not be called again.
func (b *ShapingBudget) Run(fn func() error) (work int64, err error) {
	if b == nil || fn == nil {
		return 0, errors.New("shape: nil budget or function")
	}
	if b.done {
		return 0, errors.New("shape: the budget has been spent")
	}
	defer func() {
		b.done = true
		for _, f := range b.faces {
			if f.runWork == b.work {
				f.runWork = nil
			}
		}
		// The context is the caller's, and is not kept past its call.
		b.work.ctx = nil
		if p := recover(); p != nil {
			if stop, ok := p.(runAbort); ok {
				work, err = 0, stop.err
			} else {
				panic(p)
			}
		}
	}()
	if err := fn(); err != nil {
		return 0, err
	}
	b.work.checkCtx()
	for _, f := range b.faces {
		if findings := b.work.layoutLimits(f); len(findings) != 0 {
			return 0, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
		}
	}
	return b.limits.MaxWork - b.work.left, nil
}
