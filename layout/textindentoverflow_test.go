package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An indent as wide as the block it indents, CSS Text §7.1.
//
//	The indent is treated as a margin applied to the start edge of the line box.
//
// A margin is not room the alignment may take back. An indent of the block's
// whole width leaves the line nothing to distribute, and a line with nothing to
// distribute begins where the indent put it and overflows — which is the one
// case where the alignment's answer and the indent's disagree, and the indent
// wins.
//
// It matters because the failure is silent and total: the engine set
// "text-indent: 200px; text-align: right" in two hundred pixels at the same
// place "text-indent: 0" would have, so the declaration the author wrote had no
// effect at all. The suite writes it as text-indent/text-indent-overflow, whose
// reference is the same box with a two-hundred-pixel margin on a span and no
// alignment at all.

// indentedBoxAt is where an inline-block's own box was painted in a 200px
// container. It is a box rather than text because the suite's document is one,
// and because a box has an edge to compare where a glyph has a pen position.
func indentedBoxAt(t *testing.T, css string) style.Unit {
	t.Helper()
	ops := paintOf(t, `<div id="c"><div id="g"></div></div>`,
		noDefaults+`#c { width: 200px; `+css+` } `+
			`#g { display: inline-block; width: 50px; height: 20px; background: green }`)
	for _, op := range ops {
		if v, ok := op.(FillRect); ok && v.Rect.W == bgpx(50) {
			return v.Rect.X
		}
	}
	t.Fatalf("no 50px box was painted for %q", css)
	return 0
}

// TestAnIndentIsNotRoomTheAlignmentCanTakeBack.
func TestAnIndentIsNotRoomTheAlignmentCanTakeBack(t *testing.T) {
	// The content edge, which every answer below is measured from.
	edge := indentedBoxAt(t, `text-align: left`)

	for _, tc := range []struct {
		what string
		css  string
		want style.Unit
	}{
		// The indent takes the whole width, so there is nothing left to align
		// in and the box sits at the indent — past the container's own end.
		{"an indent as wide as the block",
			`text-indent: 200px; text-align: right`, bgpx(200)},
		{"and the same for centring, which has no half of nothing to give",
			`text-indent: 200px; text-align: center`, bgpx(200)},
		// Wider still: the line starts further out again, and the alignment
		// still has nothing to say.
		{"an indent wider than the block",
			`text-indent: 250px; text-align: right`, bgpx(250)},
		// The cases with room left are the ones that already worked, and they
		// are here so that a fix for the case above cannot be a rule that
		// ignores the alignment whenever a line is indented at all.
		{"an indent leaving room, right-aligned: the box ends at the block's end",
			`text-indent: 100px; text-align: right`, bgpx(150)},
		{"an indent leaving room, centred: half of what is left",
			`text-indent: 100px; text-align: center`, bgpx(125)},
		{"no indent, right-aligned",
			`text-align: right`, bgpx(150)},
		{"an indent, left-aligned, which the alignment never touched",
			`text-indent: 200px; text-align: left`, bgpx(200)},
	} {
		if got := indentedBoxAt(t, tc.css).Sub(edge); got != tc.want {
			t.Errorf("%s: the box is %v past the content edge, want %v",
				tc.what, got.Px(), tc.want.Px())
		}
	}
}

// TestAnOverfullLineWithNoIndentStillHangsOffTheStart is the other half, and the
// behaviour this must not have broken: without an indent there is no margin to
// be pulled back into, and §16.2's right-aligned line keeps its right edge at
// the block's and lets what does not fit hang off the left. See alignLine.
func TestAnOverfullLineWithNoIndentStillHangsOffTheStart(t *testing.T) {
	edge := indentedBoxAt(t, `text-align: left`)
	// Two boxes that cannot both fit, held on one line: 300px of content in
	// two hundred, right-aligned, so the first hangs a hundred off the left.
	ops := paintOf(t, `<div id="c"><span id="g"></span><span id="h"></span></div>`,
		noDefaults+`#c { width: 200px; text-align: right; white-space: nowrap } `+
			`span { display: inline-block; width: 150px; height: 20px; background: green }`)
	var first style.Unit = 1 << 30
	for _, op := range ops {
		if v, ok := op.(FillRect); ok && v.Rect.W == bgpx(150) && v.Rect.X < first {
			first = v.Rect.X
		}
	}
	if got := first.Sub(edge); got != bgpx(-100) {
		t.Errorf("the first of two 150px boxes on a 200px line is %v past the "+
			"content edge, want -100 — an overfull right-aligned line hangs off "+
			"the start", got.Px())
	}
}
