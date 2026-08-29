package layout

import "testing"

// A cell's own border, CSS 2.1 §17.6.2.
//
// "The width of the table's border is half of the collapsed border at its edge"
// — the cell's edge, and not the grid line the cell sits on. A line is as wide
// as the widest border anywhere along it, and the two are the same number in
// every table whose cells agree about their borders, which is why this could be
// read as the line's width for a long time without anything showing.
//
// Where they differ, the space between a cell's own half and the line is content
// box. border-collapse-006 is a table built out of nothing but that difference,
// and its own comment does the arithmetic:
//
//	> the column needs to be big enough to contain:
//	>  - Half of the left border from the 1st row, i.e. 75px.
//	>  - Half of the right border form the 2nd row, i.e. 50px.
//	> Therefore, the column is max(75px, 50px) = 75px wide,
//	> and it goes from position 75px to position 150px.
//	> In the 1st row there is no remaining available space,
//	> so the div becomes 0px wide.
//	> In the 2nd row there are 75px - 50px = 25px of available space,
//	> so the div becomes 25px wide.

// TestACellsBorderIsHalfOfItsOwnEdgeAndNotOfTheLine is that table, read as
// geometry rather than as a picture.
//
// One column, two rows. The first row's cell has a 150px left border and no
// right one; the second's has a 100px right border and no left one. So the left
// grid line is 150 wide and the right one 100, the table keeps the outer half of
// each — 75 and 50 — and the column between them is 75, which is the wider of
// the two inner halves.
//
// The two cells then divide that 75 differently, and that is the whole test: the
// first has 75 of left border and nothing left over, the second has none and 25
// of content before its own 50 of right border.
func TestACellsBorderIsHalfOfItsOwnEdgeAndNotOfTheLine(t *testing.T) {
	root := layoutOf(t, 600,
		`<table id=t>
		   <tr><td id=a></td></tr>
		   <tr><td id=b></td></tr>
		 </table>`,
		collapsing+`
		 #a { border-left: 150px solid green }
		 #b { border-right: 100px solid green }
		 td { height: 25px }`)

	for _, tc := range []struct {
		id             string
		bx, bw, cx, cw float64
		why            string
	}{
		{"t", 0, 200, 75, 75,
			"75 of outer left half, 75 of column and 50 of outer right half"},
		{"a", 75, 75, 150, 0,
			"75 of its own left border fills the column and leaves no content"},
		{"b", 75, 75, 75, 25,
			"no left border of its own, so the column's first 25px are content " +
				"and the last 50 are its own right border"},
	} {
		f := find(t, root, tc.id)
		b, c := f.BorderRect, f.ContentRect()
		if b.X.Px() != tc.bx || b.W.Px() != tc.bw {
			t.Errorf("#%s border box is at %g wide %g, want %g wide %g — %s",
				tc.id, b.X.Px(), b.W.Px(), tc.bx, tc.bw, tc.why)
		}
		if c.X.Px() != tc.cx || c.W.Px() != tc.cw {
			t.Errorf("#%s content box is at %g wide %g, want %g wide %g — %s",
				tc.id, c.X.Px(), c.W.Px(), tc.cx, tc.cw, tc.why)
		}
	}
}

// TestACellKeepsHalfOfTheBorderThatWonAndNotOfItsOwn is the other half of the
// rule, and the half a fix that read the declaration would get wrong.
//
// What a cell keeps is half of the *collapsed* border at its edge, which is
// whatever won there — so a cell whose own border lost keeps half of its
// neighbour's. #a declares forty pixels on its right and #b four on its left;
// forty wins, and both cells keep twenty of it. #b keeping two would put its
// content eighteen pixels inside a border it is not allowed to overlap.
func TestACellKeepsHalfOfTheBorderThatWonAndNotOfItsOwn(t *testing.T) {
	root := layoutOf(t, 600,
		`<table id=t><tr><td id=a></td><td id=b></td></tr></table>`,
		collapsing+`
		 td { width: 20px; height: 20px }
		 #a { border-right: 40px solid green }
		 #b { border-left: 4px solid green }`)
	if got := find(t, root, "a").ContentRect(); got.X.Px() != 0 || got.W.Px() != 20 {
		t.Errorf("#a's content is at %g wide %g, want 0 wide 20",
			got.X.Px(), got.W.Px())
	}
	if got := find(t, root, "b").ContentRect().X.Px(); got != 60 {
		t.Errorf("#b's content starts at %gpx, want 60: the 40px border won on "+
			"that edge, so #b keeps 20 of it and not 2 of the 4 it declared", got)
	}
}

// TestACellSpanningTwoDifferentBordersKeepsHalfOfTheWider is the case a cell
// edge has more than one answer, and the one place this engine has to choose.
//
// A cell's edge is one number and a spanning cell's edge is not: #s reaches down
// two rows, and the cells to its left declare forty pixels and four. §17.6.2
// says "half of the collapsed border at its edge" and does not say which half
// when the edge has two. The widest is the only answer that cannot be wrong —
// the content box has to clear every border drawn along that edge, and the
// narrower one leaves it eighteen pixels inside the wider.
func TestACellSpanningTwoDifferentBordersKeepsHalfOfTheWider(t *testing.T) {
	root := layoutOf(t, 600,
		`<table id=t>
		   <tr><td id=p></td><td id=s rowspan=2></td></tr>
		   <tr><td id=q></td></tr>
		 </table>`,
		collapsing+`
		 td { width: 20px; height: 20px }
		 #p { border-right: 40px solid green }
		 #q { border-right: 4px solid green }`)
	if got := find(t, root, "s").ContentRect().X.Px(); got != 60 {
		t.Errorf("#s's content starts at %gpx, want 60: its left edge meets a "+
			"40px border and a 4px one, and it keeps half of the wider", got)
	}
	// And the cells on the other side keep half of their own, which is what says
	// the number above is a choice between two and not one of them everywhere.
	if got := find(t, root, "q").ContentRect().W.Px(); got != 38 {
		t.Errorf("#q's content is %gpx wide, want 38: 2 of its own 4px border is "+
			"all it gives up on that edge", got)
	}
}
