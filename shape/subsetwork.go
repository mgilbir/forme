package shape

import (
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

// subsetWork is one subset's allowance, and what is left of it.
type subsetWork struct {
	budget *font.Budget
	// bytes is what may still be written, of maxBytes.
	bytes, maxBytes int
}

// newSubsetWork is the allowance of a subset of the face: work units and
// bytes, each at its default where it is zero.
func (f *Face) newSubsetWork(work, bytes int) *subsetWork {
	if work == 0 {
		work = maxFontWork
	}
	if bytes == 0 {
		bytes = max(maxCFFSize, 2*programSize(f.data, f.sfntTables()))
	}
	return &subsetWork{budget: font.NewBudget(work), bytes: bytes, maxBytes: bytes}
}

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

// workErr is the error for work that ran out, or nil where it did not.
func (w *subsetWork) workErr() error {
	if err := w.budget.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrSubsetLimit, err)
	}
	return nil
}

// refusal is err, said to be the subset's limit where it is the work running
// out: the CFF walks report the budget's own error, and do not know whose
// budget it is.
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

// assemble writes the sfnt of tables, its bytes spent from w.
func (w *subsetWork) assemble(tables map[string][]byte, otto bool) ([]byte, error) {
	if err := w.write(sfntSize(tables), "the font program"); err != nil {
		return nil, err
	}
	if otto {
		return assembleOTTO(tables), nil
	}
	return assembleSFNT(tables), nil
}
