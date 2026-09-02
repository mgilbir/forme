package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An inline box whose content ends at a forced break, and where its closing
// edge goes.
//
// §8.4 puts a box's padding-right at the end of its *last* fragment, and §16.1's
// forced break ends the line wherever it falls. Put together they say that a box
// with nothing after the break has no fragment below it: there is nothing left of
// the box to put there. The suite's content-175 is an inline box whose ":after"
// is a preserved newline, and its reference is one straight navy stripe an em
// longer than the text — not a stripe with a second, shorter one beneath it.

var navy = style.RGBA{B: 128, A: 1}

// breakInsetCSS sets the text in a face whose advance is known, so that the
// stripe's length can be stated rather than measured.
const breakInsetCSS = `body { margin: 0 }
	#d { width: 400px; font-family: Courier; font-size: 20px; line-height: 1 }
	span { padding-right: 20px; background: navy }`

// TestABoxEndingAtAForcedBreakKeepsItsClosingEdge.
func TestABoxEndingAtAForcedBreakKeepsItsClosingEdge(t *testing.T) {
	got := fillsOf(paintOf(t, `<div id="d"><span>ab<br></span></div>`, breakInsetCSS), navy)
	if len(got) != 1 {
		t.Fatalf("the box drew %d stripes, want 1 — the padding after the "+
			"break went on a line of its own: %v", len(got), got)
	}
	// "ab" in 20px Courier is two characters of 12px, and the padding is 20.
	if got[0].W.Px() != 44 {
		t.Errorf("the stripe is %gpx wide, want 44 — the text and the padding "+
			"after it", got[0].W.Px())
	}
}

// TestABoxThatGoesOnPastAForcedBreakKeepsItsEdgeBelow is the other side: a box
// with content after the break does have a fragment there, and its closing edge
// belongs to it.
func TestABoxThatGoesOnPastAForcedBreakKeepsItsEdgeBelow(t *testing.T) {
	got := fillsOf(paintOf(t, `<div id="d"><span>ab<br>cd</span></div>`, breakInsetCSS), navy)
	if len(got) != 2 {
		t.Fatalf("the box drew %d stripes, want 2: %v", len(got), got)
	}
	if got[0].W.Px() != 24 {
		t.Errorf("the first stripe is %gpx wide, want 24 — the box goes on "+
			"below, so its padding is not on this line", got[0].W.Px())
	}
	if got[1].W.Px() != 44 {
		t.Errorf("the second stripe is %gpx wide, want 44", got[1].W.Px())
	}
	// A line apart, which is the whole of what "below" means here. The
	// stripes are the box's own content area and stand a little above the line
	// box, so it is the distance between them that is stated and not either y.
	if d := got[1].Y.Sub(got[0].Y).Px(); d != 20 {
		t.Errorf("the two stripes are %gpx apart, want 20", d)
	}
}

// TestOnlyTheBoxesThatEndAtTheBreakComeWithIt. A box that *opens* after the
// break opens on the line below, and its leading edge must not be dragged up
// with the closing edges around it.
func TestOnlyTheBoxesThatEndAtTheBreakComeWithIt(t *testing.T) {
	got := fillsOf(paintOf(t,
		`<div id="d"><span>ab<br></span><span class="lead">cd</span></div>`,
		breakInsetCSS+"\n\t.lead { padding-left: 20px; padding-right: 0 }"), navy)
	if len(got) != 2 {
		t.Fatalf("the two boxes drew %d stripes, want 2: %v", len(got), got)
	}
	if got[0].W.Px() != 44 {
		t.Errorf("the first stripe is %gpx wide, want 44", got[0].W.Px())
	}
	// Twenty of padding-left and two characters of twelve, beginning at the
	// block's own edge. A leading edge dragged onto the line above would leave
	// the text at nothing and the padding beside the first stripe.
	if d := got[1].Y.Sub(got[0].Y).Px(); d != 20 {
		t.Errorf("the second box's stripe is %gpx below the first, want 20", d)
	}
	if got[1].X.Px() != 0 || got[1].W.Px() != 44 {
		t.Errorf("the second box's stripe is %gpx wide at x=%g, want 44 at 0 — "+
			"it opens after the break and its padding opens the line below",
			got[1].W.Px(), got[1].X.Px())
	}
}

// TestANestedBoxEndingAtAForcedBreakComesWithItToo, because the closing edges
// arrive as a run and stopping at the first would leave the outer box's padding
// behind.
func TestANestedBoxEndingAtAForcedBreakComesWithItToo(t *testing.T) {
	got := fillsOf(paintOf(t,
		`<div id="d"><span><span>ab<br></span></span></div>`, breakInsetCSS), navy)
	for _, f := range got {
		if f.Y != got[0].Y {
			t.Errorf("the stripes are at y=%g and y=%g — both boxes end at the "+
				"break and neither has a fragment below it, so they are on one "+
				"line", got[0].Y.Px(), f.Y.Px())
		}
	}
	// The inner box holds "ab" and its own padding; the outer holds the inner
	// and its own, so the wider of the two is 64.
	var widest style.Unit
	for _, f := range got {
		if f.W > widest {
			widest = f.W
		}
	}
	if widest.Px() != 64 {
		t.Errorf("the outer stripe is %gpx wide, want 64", widest.Px())
	}
}
