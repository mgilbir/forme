package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §17.5.3's vertical-align on a table cell, when the cell declares a height.
//
// Two heights are in play and the code had only one. §17.5.3 makes a declared
// height on a cell a *minimum* — "the height of a cell box is the maximum of the
// cell's specified height and the minimum height required by the content" — so a
// cell asking for an inch makes its row an inch tall with one word in it. Then
// the cell's *content* is aligned within whatever height the row settled on.
//
// The slack to align through is the difference between those two, and the code
// measured it against the cell's used height. A cell with "height: 1in" already
// filled its inch, so there was no slack, and "vertical-align: bottom" had
// nothing to move: the word stayed at the top of a box four times its size,
// which is the one thing the declaration was written to prevent.

// cellText is the baseline the one run of text in a cell was drawn at.
func cellText(t *testing.T, html, css string) style.Unit {
	t.Helper()
	var got []style.Unit
	for _, op := range paintOf(t, html, css) {
		if v, ok := op.(DrawText); ok {
			got = append(got, v.At.Y)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d runs of text, want 1: %v", len(got), got)
	}
	return got[0]
}

// The two ways a table is written. The rules are the same for both, and the
// suite tests §17.5.3 through the CSS-display spelling.
var cellFixtures = []struct{ what, html, css string }{
	{"a table built from display properties",
		`<div id="t"><div id="r"><div id="c">X</div></div></div>`,
		`#t { display: table } #r { display: table-row }`},
	{"a table written as one",
		`<table><tr><td id="c">X</td></tr></table>`,
		`table { border-spacing: 0 } td { padding: 0 }`},
}

// TestADeclaredHeightDoesNotEatTheAlignmentSlack is the bug.
func TestADeclaredHeightDoesNotEatTheAlignmentSlack(t *testing.T) {
	for _, f := range cellFixtures {
		at := func(align string) style.Unit {
			return cellText(t, f.html, f.css+`
				#c { display: table-cell; height: 96px; width: 96px; font-size: 20px;
				     vertical-align: `+align+` }`)
		}
		top, middle, bottom := at("top"), at("middle"), at("bottom")
		if !(top < middle && middle < bottom) {
			t.Errorf("%s: top %v, middle %v, bottom %v; a cell an inch tall holding "+
				"one 20px line has three quarters of an inch to move the line through",
				f.what, top, middle, bottom)
		}
		// The three are evenly spaced, because the slack is halved for middle
		// and spent entirely for bottom.
		if middle.Sub(top) != bottom.Sub(middle) {
			t.Errorf("%s: middle is %v below top and bottom is %v below middle; "+
				"middle is half the slack and bottom is all of it",
				f.what, middle.Sub(top), bottom.Sub(middle))
		}
	}
}

// TestTheSlackIsTheRowHeightLessTheContent, as a number rather than an ordering.
func TestTheSlackIsTheRowHeightLessTheContent(t *testing.T) {
	html := `<div id="t"><div id="r"><div id="c">X</div></div></div>`
	css := `#t { display: table } #r { display: table-row }
		#c { display: table-cell; width: 96px; font-size: 20px; line-height: 20px; `

	// With no declared height the cell is exactly its content and there is no
	// slack at all, whatever the alignment asks for.
	short := cellText(t, html, css+`vertical-align: top }`)
	if got := cellText(t, html, css+`vertical-align: bottom }`); got != short {
		t.Errorf("a cell with no room to spare moved its content by %v", got.Sub(short))
	}
	// With a 96px height and a 20px line there are 76px of slack, all of which
	// "bottom" spends.
	tall := cellText(t, html, css+`height: 96px; vertical-align: top }`)
	low := cellText(t, html, css+`height: 96px; vertical-align: bottom }`)
	if got := low.Sub(tall); got != bgpx(76) {
		t.Errorf("bottom moved the content by %v; 96px of cell less 20px of line "+
			"is 76px of slack", got)
	}
}

// TestTheCellBoxItselfDoesNotMove is the other half of §17.5.3, and the reason
// this moves content rather than the box: a cell's background and border fill
// the row whatever the alignment, and only what is inside slides.
func TestTheCellBoxItselfDoesNotMove(t *testing.T) {
	box := func(align string) Rect {
		ops := paintOf(t,
			`<div id="t"><div id="r"><div id="c">X</div></div></div>`,
			`#t { display: table } #r { display: table-row }
			 #c { display: table-cell; height: 96px; width: 96px; font-size: 20px;
			      background: rgb(0,0,255); vertical-align: `+align+` }`)
		got := fillsOf(ops, blue)
		if len(got) != 1 {
			t.Fatalf("%d cell backgrounds: %v", len(got), got)
		}
		return got[0]
	}
	if top, bottom := box("top"), box("bottom"); top != bottom {
		t.Errorf("the cell box is %v aligned top and %v aligned bottom; the box "+
			"fills its row either way and only its content moves", top, bottom)
	}
}

// TestADeclaredHeightStillSizesTheRow is the containment case, and the one this
// change could most easily have broken: the used height is still what the row is
// sized from, and only the *alignment* asks the other question.
func TestADeclaredHeightStillSizesTheRow(t *testing.T) {
	ops := paintOf(t,
		`<div id="t"><div id="r"><div id="c">X</div></div></div>`,
		`#t { display: table; background: rgb(0,0,255) }
		 #r { display: table-row }
		 #c { display: table-cell; height: 96px; width: 96px; font-size: 20px }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d table backgrounds: %v", len(got), got)
	}
	if got[0].H != bgpx(96) {
		t.Errorf("the table is %v tall and its one cell asked for 96px; §17.5.3 "+
			"makes a declared height a minimum, and a minimum still binds", got[0].H)
	}
}

// TestAFloatInsideACellCountsAsItsContent. The height a box's content came to
// includes the floats it contains, because a cell establishes a formatting
// context and §10.6.7 makes those floats part of what it holds. A cell whose
// only content is a float would otherwise have no content at all to align, and
// bottom-aligning it would drop the float out of the bottom of the row.
func TestAFloatInsideACellCountsAsItsContent(t *testing.T) {
	ops := paintOf(t,
		`<div id="t"><div id="r"><div id="c"><div id="f"></div></div></div></div>`,
		`#t { display: table } #r { display: table-row }
		 #c { display: table-cell; height: 96px; width: 96px; vertical-align: bottom }
		 #f { float: left; width: 20px; height: 20px; background: rgb(0,0,255) }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d floats: %v", len(got), got)
	}
	// The cell runs from 8 to 104; a 20px float bottom-aligned ends at 104.
	if bottom := got[0].Y.Add(got[0].H); bottom != bgpx(104) {
		t.Errorf("the float ends at %v and the cell at 104; a float inside a cell "+
			"is part of what the cell holds", bottom)
	}
}
