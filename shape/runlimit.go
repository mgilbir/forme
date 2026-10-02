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
}
type runAbort struct{ err error }

func (w *runWork) spend(n int64) {
	if w == nil {
		return
	}
	if err := w.ctx.Err(); err != nil {
		panic(runAbort{err})
	}
	if n < 0 || n > w.left {
		panic(runAbort{fmt.Errorf("%w: lookup work", ErrRunLimit)})
	}
	w.left -= n
}
func (w *runWork) size(n int) {
	if w == nil {
		return
	}
	w.spend(0)
	if n < 0 || n > w.glyphs {
		panic(runAbort{fmt.Errorf("%w: glyph count", ErrRunLimit)})
	}
}

// ShapeGlyphsContext shapes a bounded run without changing the receiver's used
// glyph record. It returns no glyphs on cancellation, work exhaustion, glyph
// expansion, recursion exhaustion or reported font-layout truncation. A private
// clone shares the receiver's locked font caches. Font programs must remain
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
	if findings := clone.LayoutLimits(); len(findings) != 0 {
		return RunResult{}, fmt.Errorf("%w: font layout: %v", ErrRunLimit, findings)
	}
	result.Work = limits.MaxWork - clone.runWork.left
	return result, nil
}
