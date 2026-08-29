package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Where a letter-spacing gap sits, CSS Text §8.2.
//
// The gap goes *between* two typographic character units, so it belongs to
// neither of the boxes they are in. Two things follow, and they are the two
// things measured here:
//
//	glyphs · the box's ending edge · the gap · whatever comes next
//
// A box's ink stops at its last glyph, and the box's own border and padding are
// part of the box and sit against those glyphs rather than across the gap.
//
// letter-spacing-nesting-003 asks for the first in as many words — "the green
// rectangle does not extend beyond B to C" — and letter-spacing-bidi-004 and
// -005 for the second, over spans whose backgrounds must be "the width of a
// single letter".
//
// Courier at 20px is 12px a character, and the letter-spacings below are 12px on
// the block and 24px on the span so that no two of the four numbers on the line
// can be confused for each other.

const gapCSS = `#p { font-family: Courier; font-size: 20px; width: 3000px;
	letter-spacing: 12px } .ls { letter-spacing: 24px }`

// spanBoxAndNext is the span's border box and where the run after it begins,
// both measured from the block's content edge so that the page margin is not
// part of the arithmetic.
func spanBoxAndNext(t *testing.T, css string) (box Rect, next style.Unit) {
	t.Helper()
	root := layoutOf(t, 4000,
		`<div id="p">A<span id="s" class="ls">BC</span>D</div>`, gapCSS+css)
	p := find(t, root, "p")
	origin := p.ContentRect().X
	frags := inlineFragsOf(root, "s")
	if len(frags) != 1 {
		t.Fatalf("the span made %d painted pieces, want 1", len(frags))
	}
	box = frags[0].BorderRect
	box.X = box.X.Sub(origin)
	runs := runsOf(t, root, "p")
	if len(runs) != 3 {
		t.Fatalf("the line has %d runs, want 3 — A, BC and D", len(runs))
	}
	return box, runs[2].X
}

// TestABoxsInkStopsAtItsLastGlyph is the first half.
//
// "A" is 12px of glyph and the 12px gap the block sets between it and "B", so
// the span's letters begin at 24. B and C are 12px each with the span's 24px
// between them, and after C comes the block's 12px again — because the pair
// (C, D) is the block's, not the span's. So the span's ink is 24 to 72 and the
// gap that follows is 72 to 84, outside it.
func TestABoxsInkStopsAtItsLastGlyph(t *testing.T) {
	box, next := spanBoxAndNext(t, ` .ls { background: green }`)
	if got := box.X.Px(); got != 24 {
		t.Errorf("the span's ink starts at %gpx, want 24", got)
	}
	if got := box.X.Add(box.W).Px(); got != 72 {
		t.Errorf("the span's ink ends at %gpx, want 72 — at C, not a gap past it", got)
	}
	if next.Px() != 84 {
		t.Errorf("the run after the span begins at %gpx, want 84", next.Px())
	}
	if gap := next.Sub(box.X.Add(box.W)).Px(); gap != 12 {
		t.Errorf("%gpx separates the span's ink from the next character, want 12 "+
			"— the block's gap, and it is between them rather than inside either", gap)
	}
}

// TestABoxsOwnEdgeSitsAgainstItsGlyphs is the second half, and the one only a
// border can show: a background stops where the ink stops either way, and a
// border is drawn at a place.
//
// Five pixels of border after C, and then the gap. Not the gap and then the
// border, which is where a border lands when the edge is placed at the far side
// of the run's advance — outside its own box's ink, with the space between the
// rule and the letter it is drawn against.
func TestABoxsOwnEdgeSitsAgainstItsGlyphs(t *testing.T) {
	box, next := spanBoxAndNext(t, ` .ls { border-right: 5px solid green }`)
	if got := box.X.Add(box.W).Px(); got != 77 {
		t.Errorf("the span's border box ends at %gpx, want 77 — C ends at 72 and "+
			"the border is the five pixels after it", got)
	}
	if got := next.Px(); got != 89 {
		t.Errorf("the run after the span begins at %gpx, want 89", got)
	}
	if gap := next.Sub(box.X.Add(box.W)).Px(); gap != 12 {
		t.Errorf("%gpx separates the border from the next character, want 12 — "+
			"the gap goes outside the box and not between the border and C", gap)
	}
}

// TestTheLineIsTheWidthItWas is the containment case, and the one that says the
// edge was *moved* rather than the line rearranged.
//
// Nothing downstream of a box may shift: an edge pulled back by the gap leaves a
// hole of exactly that width after it, and the pen is where it always was. So
// the same content with and without a border on the span differs by the border
// and by nothing else.
func TestTheLineIsTheWidthItWas(t *testing.T) {
	_, plain := spanBoxAndNext(t, ` .ls { background: green }`)
	_, bordered := spanBoxAndNext(t, ` .ls { border-right: 5px solid green }`)
	if got := bordered.Sub(plain).Px(); got != 5 {
		t.Errorf("adding a 5px border moved the next character by %gpx, want 5 — "+
			"moving the edge across the gap must not move anything else", got)
	}
}

// TestTheNextBoxBeginsOnTheFarSideOfTheGap is the other side of "whose edge is
// it", and the row that says the rule is about the box rather than about being
// next to a run.
//
// Two spans in a row: "A<span class=ls>B</span><span id=t>C</span>D". Between B
// and C there are two edges — the first span's ending one and the second span's
// beginning one — and the gap goes between them, because it goes between the two
// characters and each edge belongs to the letter on its own side.
//
// So the first span's edge moves back against B and the second span's does not
// move at all. B's letters end at 36 and the gap runs to 48, where the second
// span begins with its five pixels of border and then C.
func TestTheNextBoxBeginsOnTheFarSideOfTheGap(t *testing.T) {
	root := layoutOf(t, 4000,
		`<div id="p">A<span class="ls">B</span><span id="t">C</span>D</div>`,
		gapCSS+` #t { border-left: 5px solid green }`)
	p := find(t, root, "p")
	frags := inlineFragsOf(root, "t")
	if len(frags) != 1 {
		t.Fatalf("the second span made %d painted pieces, want 1", len(frags))
	}
	if got := frags[0].BorderRect.X.Sub(p.ContentRect().X).Px(); got != 48 {
		t.Errorf("the second span begins at %gpx, want 48 — B ends at 36 and the "+
			"gap between B and C runs to 48, which is where the box that holds C "+
			"starts", got)
	}
	runs := runsOf(t, root, "p")
	if len(runs) != 4 {
		t.Fatalf("the line has %d runs, want 4", len(runs))
	}
	if got := runs[2].X.Px(); got != 53 {
		t.Errorf("C is at %gpx, want 53 — the border's five pixels after the gap", got)
	}
}
