package layout

import (
	"github.com/mgilbir/forme/style"
)

// Control characters, which CSS Text 3 requires to be *visible*.
//
// §white-space-processing: a character in Unicode's Cc category other than tab,
// line feed and carriage return is rendered as a visible glyph, which the UA
// synthesizes — conventionally a box showing the code point in hexadecimal. The
// suite says it in the title of sixty of its documents: "Control characters must
// be visible: U+0001".
//
// This engine set them as spaces and reported a missing glyph. The report was
// exact — the character *was* missing from the page — and the answer to it is
// not to stop reporting but to draw the thing.
//
// # What is drawn
//
// A box, and not the hexadecimal digits inside it. "Must be visible" is the
// requirement and the digits are the "should": four digits inside one character's
// advance means type at a quarter of the size, which on paper at a normal text
// size is a smudge rather than a number. A box is honest about what it is — a
// character that could not be shown — and is what a reader needs to know.
//
// # What is not touched
//
// The advance. A face gives a control character the width of its .notdef, layout
// has already spent it, and the box is drawn inside it — so nothing about where
// the text sits changes, and a document that has one of these in it is laid out
// exactly as it was before.

// isVisibleControl reports whether a character must be drawn as a visible glyph.
//
// Tab, line feed and carriage return are named out of it by the specification:
// each is white space with a meaning the white-space processing has already
// acted on. Form feed goes with them — CSS 2.1 counts it among the white space a
// document may be written with, so drawing a box for one would put a mark on the
// page where an author put a page break in their source.
func isVisibleControl(r rune) bool {
	switch r {
	case '\t', '\n', '\r':
		return false
	}
	return r < 0x20 || (r >= 0x7F && r <= 0x9F)
}

// hasVisibleControl reports whether text holds one.
func hasVisibleControl(text string) bool {
	for _, r := range text {
		if isVisibleControl(r) {
			return true
		}
	}
	return false
}

// controlOf returns the character a run stands for, when the run is one control
// character and nothing else.
//
// Runs are split so that this is the only shape a control character reaches
// painting in — see faceRunsFor, which cuts one out of the text around it for
// this reason as well as for the face.
func controlOf(text string) (rune, bool) {
	var got rune
	n := 0
	for _, r := range text {
		got = r
		n++
	}
	if n != 1 || !isVisibleControl(got) {
		return 0, false
	}
	return got, true
}

// controlBox is the ring drawn for one control character, inside the advance the
// face already gave it.
//
// at is the run's origin on its baseline, advance the width layout spent on it,
// and size the font size — which is what the box is proportioned from rather
// than the advance, so that a face with a wide .notdef does not get a box of a
// different shape from one with a narrow one.
func controlBox(at Point, advance, size style.Unit, colour style.RGBA, turn runTurn) []Op {
	// Two thirds of the em tall, which sits between the baseline and about the
	// cap height, and inset from the advance so two of them in a row do not
	// touch.
	h := size.Mul(0.66)
	inset := size.Mul(0.08)
	thick := size.Mul(0.05)
	if thick < 1 {
		thick = 1
	}
	w := advance.Sub(inset).Sub(inset)
	if w <= 0 || h <= 0 {
		return nil
	}
	// The ring is built in the run's own axes — along the line for its advance,
	// off the baseline for its height — and placed afterwards. On a line that
	// runs down the page that is the whole of the difference: built in page
	// coordinates it came out lying across the page between two letters set one
	// above the other, and hanging off the top of the box as well. See placeRun.
	x := inset
	y := style.Unit(0).Sub(h)
	ring := func(r Rect) Op {
		return FillRect{Rect: placeRun(r, at, turn), Color: colour, Overhang: true}
	}
	if w <= thick.Mul(2) || h <= thick.Mul(2) {
		// Too small for a ring to have a hole. A solid mark is still a visible
		// glyph, which is the requirement; an empty one would be the fault this
		// exists to fix, and it is what a ring drawn without this check becomes
		// when its two edges meet in the middle.
		return []Op{ring(Rect{x, y, w, h})}
	}
	return []Op{
		ring(Rect{x, y, w, thick}),
		ring(Rect{x, y.Add(h).Sub(thick), w, thick}),
		ring(Rect{x, y.Add(thick), thick, h.Sub(thick).Sub(thick)}),
		ring(Rect{x.Add(w).Sub(thick), y.Add(thick), thick, h.Sub(thick).Sub(thick)}),
	}
}
