package layout

import "testing"

// Where a box that may not overlap a float puts its own margins.
//
// §9.5 says only that the border box "must not overlap the margin box of any
// floats"; it does not say where beside the float the box goes, and a margin on
// the side facing the float is exactly the case it leaves open. Two readings are
// possible. One stacks the margin onto the float's edge, so a "margin-left:
// 20px" box beside a 50px float starts at 70. The other keeps the margin what
// §10.3.3 says it is — a distance from the containing block's edge — and lets
// the float's edge act only as a floor under the border box, so the same box
// starts at 50, and a "margin-left: 80px" one at 80.
//
// Every browser takes the second, and the suite says so in documents this
// engine cannot run because they build themselves with a script:
// floats-wrap-bfc-with-margin-001 (passed by Chrome, Firefox and Safari) sets
// margins of 0 to 14px beside a 14px float and its reference leaves them out
// altogether, because "the bfc's margin is going to 'overlap' the float" and
// "doesn't impact the testcase's rendering"; -003 (passed by all three) sets 15
// to 28px and draws the box at the margin, not at float plus margin. Its notes
// name the three cases every test below is one of:
//
//	A. the margin is on the float's side, and simply overlaps the float;
//	B. the margin is on the line-start side and the float on the line-end
//	   side, and the margin pushes the border box onto the float, so the box
//	   goes below it;
//	C. the margin is on the line-end side and the float on the line-start
//	   side, and the box stays beside the float with its margin running off the
//	   end of the containing block.
//
// Every number below is worked out from that rule, and each document is chosen
// so that the old reading gives a different one.

// bfcBesideFloat lays out a wrapper holding one float and one flow root and
// returns the flow root's left edge, top and border-box width relative to the
// wrapper.
func bfcBesideFloat(t *testing.T, wrapper, float, bfc string) (x, y, w float64) {
	t.Helper()
	css := noDefaults + `
	#w { ` + wrapper + ` }
	#f { height: 20px; ` + float + ` }
	#r { overflow: hidden; height: 10px; ` + bfc + ` }`
	root := layoutOf(t, 400, `<section id="w"><div id="f"></div><div id="r"></div></section>`, css)
	wr, r := find(t, root, "w"), find(t, root, "r")
	return relX(t, r, wr).Px(), relY(t, r, wr).Px(), r.BorderRect.W.Px()
}

// TestBFCMarginFacingAFloatIsMeasuredFromTheContainingBlock is case A, in both
// directions and with widths both auto and declared.
func TestBFCMarginFacingAFloatIsMeasuredFromTheContainingBlock(t *testing.T) {
	cases := []struct {
		what                string
		wrapper, float, bfc string
		wantX, wantY, wantW float64
	}{
		// A margin narrower than the float disappears into it: the border box is
		// against the float, and an auto width fills the whole band. Stacking
		// gives 70 and 330.
		{"a left margin narrower than a left float",
			"width: 400px", "float: left; width: 50px", "margin-left: 20px",
			50, 0, 350},
		// A margin wider than the float is still measured from the block's edge.
		// Stacking gives 130 and 270.
		{"a left margin wider than a left float",
			"width: 400px", "float: left; width: 50px", "margin-left: 80px",
			80, 0, 320},
		// The mirror, on the right. Stacking ends the box at 330.
		{"a right margin narrower than a right float",
			"width: 400px", "float: right; width: 50px", "margin-right: 20px",
			0, 0, 350},
		{"a right margin wider than a right float",
			"width: 400px", "float: right; width: 50px", "margin-right: 80px",
			0, 0, 320},
		// A declared width keeps its width and starts where the rule says.
		{"a declared width with a left margin narrower than the float",
			"width: 400px", "float: left; width: 50px", "margin-left: 20px; width: 100px",
			50, 0, 100},
		{"a declared width with a left margin wider than the float",
			"width: 400px", "float: left; width: 50px", "margin-left: 80px; width: 100px",
			80, 0, 100},
		// Where the difference decides whether the box fits at all: a 330px box
		// beside a 50px float in 400 fits against the float, but stacking a 20px
		// margin onto the float asks for 400 and drops it below.
		{"a declared width that fits only with the margin overlapping the float",
			"width: 400px", "float: left; width: 50px", "margin-left: 20px; width: 350px",
			50, 0, 350},
		// In a right-to-left block the float on the right is at the line start,
		// and its margin behaves the same way.
		{"a right margin beside a right float in a right-to-left block",
			"width: 400px; direction: rtl", "float: right; width: 50px", "margin-right: 20px",
			0, 0, 350},
	}
	for _, c := range cases {
		x, y, w := bfcBesideFloat(t, c.wrapper, c.float, c.bfc)
		if x != c.wantX || y != c.wantY || w != c.wantW {
			t.Errorf("%s: the flow root is at (%v, %v) and %v wide, want (%v, %v) and %v wide",
				c.what, x, y, w, c.wantX, c.wantY, c.wantW)
		}
	}
}

// TestBFCNegativeMarginFacingAFloatDoesNotPullItOver is the same rule with a
// negative margin: measured from the block's edge it is outside the float
// altogether, so the float's edge is where the border box starts.
//
// Stacking puts the border box twenty pixels over the float, which is the one
// thing §9.5 forbids; floats-wrap-bfc-with-margin-002 (Chrome and Firefox) draws
// the box against the float for every margin from -1 to -16px.
func TestBFCNegativeMarginFacingAFloatDoesNotPullItOver(t *testing.T) {
	cases := []struct {
		what                string
		wrapper, float, bfc string
		wantX, wantY, wantW float64
	}{
		{"a negative left margin beside a left float",
			"width: 400px", "float: left; width: 50px", "margin-left: -20px",
			50, 0, 350},
		{"a negative right margin beside a right float",
			"width: 400px", "float: right; width: 50px", "margin-right: -20px",
			0, 0, 350},
		// The margin on the side with no float is still the author's to use: a
		// negative one there widens the box past the block, which is what
		// floats-wrap-bfc-with-margin-006 and -007 turn on.
		{"a negative right margin beside a left float",
			"width: 400px", "float: left; width: 50px", "margin-right: -20px",
			50, 0, 370},
	}
	for _, c := range cases {
		x, y, w := bfcBesideFloat(t, c.wrapper, c.float, c.bfc)
		if x != c.wantX || y != c.wantY || w != c.wantW {
			t.Errorf("%s: the flow root is at (%v, %v) and %v wide, want (%v, %v) and %v wide",
				c.what, x, y, w, c.wantX, c.wantY, c.wantW)
		}
	}
}

// TestBFCMarginAtTheLineEndRunsOffTheBlock is cases B and C, which are the
// same arithmetic and different answers because §10.3.3's over-constrained
// equality gives way at the line end.
//
// The numbers are floats-wrap-bfc-with-margin-003's: a block 30 wide, a float
// 14 wide, and a flow root with a 1px border and a 22px margin.
//
//   - Case C: the margin is at the line end and the float at the line start.
//     The box has no room for content, so its width is zero; the equality is
//     over-constrained and the line-end margin is the value that is ignored.
//     The box's border box, two pixels, is against the float. Counting the
//     margin as part of what has to fit drops it below the float.
//   - Case B: the margin is at the line start, the float at the line end. The
//     margin is honoured, so the border box would start at 22 and sit on a
//     float that begins at 16; it goes below the float, at y=20, where it
//     spans 22 to 30.
func TestBFCMarginAtTheLineEndRunsOffTheBlock(t *testing.T) {
	cases := []struct {
		what                string
		wrapper, float, bfc string
		wantX, wantY, wantW float64
	}{
		{"case C, left to right",
			"width: 30px", "float: left; width: 14px", "border: 1px solid; margin-right: 22px",
			14, 0, 2},
		{"case C, right to left",
			"width: 30px; direction: rtl", "float: right; width: 14px", "border: 1px solid; margin-left: 22px",
			14, 0, 2},
		{"case B, left to right",
			"width: 30px", "float: right; width: 14px", "border: 1px solid; margin-left: 22px",
			22, 20, 8},
		{"case B, right to left",
			"width: 30px; direction: rtl", "float: left; width: 14px", "border: 1px solid; margin-right: 22px",
			0, 20, 8},
	}
	for _, c := range cases {
		x, y, w := bfcBesideFloat(t, c.wrapper, c.float, c.bfc)
		if x != c.wantX || y != c.wantY || w != c.wantW {
			t.Errorf("%s: the flow root is at (%v, %v) and %v wide, want (%v, %v) and %v wide",
				c.what, x, y, w, c.wantX, c.wantY, c.wantW)
		}
	}
}

// TestBFCKeepsATrailingMarginThatDoesNotFitRightToLeft is
// TestBFCBoxKeepsATrailingMarginThatDoesNotFit in a right-to-left block, where
// the trailing margin is the left one.
//
// A float on the right 50 wide in a block 100 wide leaves a band of 50, and a
// flow root declaring "width: 50px; margin-left: 1px" fills it exactly. The
// equality is over-constrained and, the containing block being right to left,
// margin-left is the value ignored — so it does not count, and the box stays
// beside the float at x=0. Reading margin-left as the leading margin whatever
// the direction drops it below the float, at y=20.
func TestBFCKeepsATrailingMarginThatDoesNotFitRightToLeft(t *testing.T) {
	x, y, w := bfcBesideFloat(t, "width: 100px; direction: rtl",
		"float: right; width: 50px", "margin-left: 1px; width: 50px")
	if x != 0 || y != 0 || w != 50 {
		t.Errorf("the flow root is at (%v, %v) and %v wide, want (0, 0) and 50 wide", x, y, w)
	}
}
