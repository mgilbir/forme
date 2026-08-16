package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What the text comparison must and must not see, stated as two cases that pull
// in opposite directions.
//
// A picture comparison is about ink. Two documents that put the same visible
// glyphs in the same places drew the same page, however they batched the runs
// and whatever invisible characters they put between them — a reference writing
// "a&nbsp;b" as one run and a test writing "a", " ", "b" under white-space: pre
// are the same five sixteenths of ink.
//
// The trap is the obvious way to say that. Dropping blank runs and stripping
// blank glyphs from a run's identity makes "a b" and "ab" the same mark at the
// same place — and they are not the same page, because the "b" is a space apart.
// Any relaxation has to keep that difference visible, which means every glyph
// has to carry its own position rather than the group carrying one for all of
// them.

// TestPictureIgnoresHowRunsAreBatchedAroundSpaces is the relaxation.
//
// Same glyphs, same places, different batching — one document splits at the
// space and the other does not. This is white-space-pre-011 and its reference in
// miniature, and it is five of the suite's failures.
func TestPictureIgnoresHowRunsAreBatchedAroundSpaces(t *testing.T) {
	face, adv := picFace(t)
	black := style.RGBA{A: 1}

	whole := []Op{picFacedText(face, "a b", 20, 100, black)}
	split := []Op{
		picFacedText(face, "a", 20, 100, black),
		picFacedText(face, " ", 20+adv("a"), 100, black),
		picFacedText(face, "b", 20+adv("a "), 100, black),
	}
	if !pictureEqual(whole, split, picPage) {
		t.Error("one run \"a b\" did not compare equal to the same glyphs drawn " +
			"as three runs in the same places")
	}
}

// TestPictureSeesAGlyphMovedInsideARun is the trap, and it is what stops the
// relaxation above from being a hole.
//
// "a b" and "ab" begin with the same glyph at the same place and end with the
// same glyph a space's width apart. A comparison holding one position per group
// and ignoring blanks calls them identical; they are two different pages.
func TestPictureSeesAGlyphMovedInsideARun(t *testing.T) {
	face, adv := picFace(t)
	black := style.RGBA{A: 1}

	spaced := []Op{picFacedText(face, "a b", 20, 100, black)}
	tight := []Op{picFacedText(face, "ab", 20, 100, black)}
	if pictureEqual(spaced, tight, picPage) {
		t.Error("\"a b\" compared equal to \"ab\"; the second glyph is a space's " +
			"width apart on the two pages")
	}

	// And the same thing built the other way round, so that a rule keyed on run
	// *count* rather than on glyph position cannot pass this by accident.
	splitSpaced := []Op{
		picFacedText(face, "a", 20, 100, black),
		picFacedText(face, "b", 20+adv("a "), 100, black),
	}
	splitTight := []Op{
		picFacedText(face, "a", 20, 100, black),
		picFacedText(face, "b", 20+adv("a"), 100, black),
	}
	if pictureEqual(splitSpaced, splitTight, picPage) {
		t.Error("two runs a space apart compared equal to the same two abutting")
	}
}

// TestPictureSeesTheFaceAGlyphCameFrom.
//
// A glyph id means nothing on its own: glyph 42 of one font and glyph 42 of
// another are two different shapes, and a comparison keyed on the number alone
// would call any two documents that used different fonts identical wherever
// their numbering happened to line up.
//
// The old comparison had the face in joinRuns' key rather than in the mark, so
// two runs in different faces were different *groups* — but nothing carried the
// face into what was compared, and two groups whose glyph numbers matched paired
// up regardless. Per-glyph marks make that reachable in one run rather than two,
// so the face is named in the identity now and this is what says so.
func TestPictureSeesTheFaceAGlyphCameFrom(t *testing.T) {
	serif, _ := picFace(t)
	mono, ok := StandardFonts().Face("monospace", false, false)
	if !ok || mono == nil {
		t.Skip("the standard font set has no monospace face")
	}
	if serif == mono {
		t.Skip("serif and monospace resolve to the same face here")
	}
	black := style.RGBA{A: 1}

	in := func(f *shape.Face) []Op {
		return []Op{picFacedText(f, "A", 20, 40, black)}
	}
	if pictureEqual(in(serif), in(mono), picPage) {
		t.Error("the same letter in two different faces compared equal; a glyph " +
			"id is only a shape together with the font it is numbered in")
	}
}
