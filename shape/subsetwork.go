package shape

import (
	"context"
	"errors"
	"fmt"

	"github.com/mgilbir/forme/font"
)

// What one subset may cost: the work its walks do, and the bytes it writes.
//
// A document that embeds fonts from untrusted input has to know what an
// embedding can cost before it starts, and a subset is most of that cost. Each
// subset draws on one allowance of each, its own, and a subset that would
// spend more than either is refused with ErrSubsetLimit rather than written.
//
// # Work
//
// The work is the font.Budget's units: an operator of a charstring the
// seac closure or the subroutine pruning runs, a Private DICT's bytes and its
// subroutine INDEX's entries, and for a TrueType face a unit for each glyph
// the closure keeps and each component it reads. It is maxFontWork, the
// allowance Load reads the whole font under, and was the CFF subsetter's
// alone; the glyf path spent from nothing and was bounded only by the glyphs
// it read, which are the bytes it writes below.
//
// A glyph a VARC table composes is written out under the VARC allowance a
// subset has always had (varcFaceWork), which counts in VARC's own units; and
// a CFF2 glyph is written as CFF by the face's converter, once per face and
// under the face's allowance (cff2cff.go), whichever subset asked first.
//
// # Bytes
//
// The bytes are what the subset writes: each glyph's record as it is copied
// or written out, every table written again, and the program assembled from
// them. They are counted as they are written, so a subset that would come to
// more stops at the allowance and not after it. The allowance is the larger of
// twice the program the face was loaded from and maxCFFSize, the size a CFF
// program may come to at all.
//
// It is not a bound a subset of a sound font comes near. A subset carries at
// most each kept glyph once, and tables the font already holds, so the program
// is at most about the size of the font; the pieces it is assembled from are
// as much again. What it stops is a font whose glyphs overlap: loca is required
// to ascend and nothing makes it, and offsets 0, L, 0, L, … give every other
// glyph the whole of glyf. Each of those glyphs is sound on its own, and a
// subset keeping all of them was the glyph count times the table.
//
// # Cancellation
//
// A subset made under a context (SubsetGlyphsContext) stops when it is done:
// the context is asked before each glyph is decided, kept, written out or
// copied, and the work budget asks it as the CFF walks draw on it, so a subset
// stops within a glyph's work of being cancelled. It returns the context's
// error, and no program.
//
// # What is not counted
//
// The decisions about each glyph (glyphcheck.go) are the face's and are kept,
// so they are bounded per glyph and not per subset: a glyf record is read at
// most once, and a charstring run under HarfBuzz's cap. The CFF subsetter's
// own fixed bounds stay whatever a subset is allowed: a CFF table larger than
// maxCFFSize is refused, and so are Private DICT regions that come to more
// than the font holds (cffsubset.go).

// ErrSubsetLimit identifies a subset refused because it would spend more than
// it may: more work than its allowance, or more bytes.
var ErrSubsetLimit = errors.New("shape: subset limit exceeded")

// SubsetLimits bounds one SubsetGlyphsContext call. Zero fields select the
// defaults SubsetGlyphs subsets under; negative fields are invalid. They are
// the subset's allowance and do not replace the bounds the subsetter keeps
// whatever it is allowed: see subsetwork.go.
type SubsetLimits struct {
	// MaxWork is the work the subset may do, in the units font.Budget counts:
	// maxFontWork, the allowance Load reads the whole font under, where it is
	// zero.
	MaxWork int
	// MaxBytes is how many bytes the subset may write: the program it returns
	// and the tables it is assembled from. The larger of twice the font
	// program and 64 MiB where it is zero.
	MaxBytes int
}

// subsetWork is one subset's allowance, and what is left of it.
type subsetWork struct {
	ctx    context.Context
	budget *font.Budget
	// bytes is what may still be written, of maxBytes.
	bytes, maxBytes int
}

// newSubsetWork is the allowance of a subset of the face under ctx, each limit
// at its default where it is zero. The limits must not be negative.
func (f *Face) newSubsetWork(ctx context.Context, limits SubsetLimits) *subsetWork {
	work, bytes := limits.MaxWork, limits.MaxBytes
	if work == 0 {
		work = maxFontWork
	}
	if bytes == 0 {
		bytes = max(maxCFFSize, 2*programSize(f.data, f.sfntTables()))
	}
	return &subsetWork{ctx: ctx, budget: font.NewBudgetContext(ctx, work), bytes: bytes, maxBytes: bytes}
}

// SubsetGlyphsContext is SubsetGlyphs under a context and a caller's limits:
// it stops, with ctx's error and no program, when ctx is done, and refuses
// with an error wrapping ErrSubsetLimit a subset that would spend more work
// or write more bytes than limits allow. Zero limits are SubsetGlyphs' own.
//
// It is the one subset entry point that takes them; Subset is SubsetGlyphs
// without the kept glyphs, and a caller with a deadline wants those anyway.
func (f *Face) SubsetGlyphsContext(ctx context.Context, limits SubsetLimits) (program []byte, kept []int, err error) {
	if ctx == nil || f == nil {
		return nil, nil, errors.New("shape: nil context or face")
	}
	if limits.MaxWork < 0 || limits.MaxBytes < 0 {
		return nil, nil, errors.New("shape: negative subset limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return f.subset(f.newSubsetWork(ctx, limits))
}

// poll is the context's error once it is done, and nil until then.
func (w *subsetWork) poll() error { return w.ctx.Err() }

// write spends n bytes on what, or refuses.
func (w *subsetWork) write(n int, what string) error {
	if n > w.bytes {
		w.bytes = 0
		return fmt.Errorf("%w: writing %s takes the subset past the %d bytes it may come to", ErrSubsetLimit, what, w.maxBytes)
	}
	w.bytes -= n
	return nil
}

// charge spends n units of work on what, or refuses.
func (w *subsetWork) charge(n int, what string) error {
	if !w.budget.Charge(n, what) {
		return w.workErr()
	}
	return nil
}

// workErr is the error for work that ran out, or the context's for a budget
// its context stopped, or nil where neither happened.
func (w *subsetWork) workErr() error {
	err := w.budget.Err()
	if err == nil {
		return nil
	}
	if done := w.ctx.Err(); done != nil && errors.Is(err, done) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrSubsetLimit, err)
}

// refusal is err, said to be the subset's limit where it is the work running
// out, and the context's error where it is the context: the CFF walks report
// the budget's own error, and do not know whose budget it is or why it
// stopped.
func (w *subsetWork) refusal(err error) error {
	if w.budget.Exhausted() {
		return w.workErr()
	}
	return err
}

// sfntSize is how many bytes assembleSFNT writes for tables.
func sfntSize(tables map[string][]byte) int {
	n := 12 + 16*len(tables)
	for _, b := range tables {
		n += (len(b) + 3) &^ 3
	}
	return n
}

// assemble writes the sfnt of tables, its bytes spent from w. Every subset is
// assembled here, so this is where its name table is cut to what a subset
// keeps (subsetName), whichever way its outlines were written.
func (w *subsetWork) assemble(tables map[string][]byte, otto bool) ([]byte, error) {
	if name, ok := tables["name"]; ok {
		if cut := subsetName(name); cut != nil && len(cut) < len(name) {
			if err := w.write(len(cut), "the name table"); err != nil {
				return nil, err
			}
			tables["name"] = cut
		}
	}
	if err := w.write(sfntSize(tables), "the font program"); err != nil {
		return nil, err
	}
	if otto {
		return assembleOTTO(tables), nil
	}
	return assembleSFNT(tables), nil
}
