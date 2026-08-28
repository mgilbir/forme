package layout

import "testing"

// Whether a box passes a baseline up to the box outside it.
//
// §10.8.1 gives an inline-block the baseline of its last in-flow line box, and
// that line box may be several boxes down: a block container has no baseline of
// its own, so the search walks into its children until it finds a line. What it
// must not walk into is a box whose overflow is not visible. What is inside one
// may be scrolled away, so a line of it is not a line anything outside can be
// aligned to — the box's own bottom margin edge is what it contributes, which
// is the same sentence §10.8.1 applies to an inline-block that has one.
//
// baseline-block-with-overflow-001 is five sections of exactly that, and it is
// where the text beside the box sits that says which answer was taken.

// baselineBesideAnInlineBlock lays out an inline-block holding a 30px block, and
// returns where the baseline of the text beside it falls in the paragraph.
func baselineBesideAnInlineBlock(t *testing.T, innerClass string) float64 {
	t.Helper()
	root := layoutOf(t, 600,
		`<div id="p"><span class="outer"><span class="inner `+innerClass+
			`">Y</span></span>XX</div>`,
		`#p { font-family: Courier; font-size: 20px }
		 .outer { display: inline-block; padding-bottom: 20px }
		 .inner { display: block; width: 30px; height: 30px }
		 .hidden { overflow: hidden }
		 .empty { visibility: hidden }`)
	f := find(t, root, "p")
	if len(f.Lines) == 0 {
		t.Fatal("#p has no lines")
	}
	return f.Lines[0].Rect.Y.Add(f.Lines[0].Baseline).Px()
}

// TestABoxWithHiddenOverflowGivesItsBottomEdgeAndNotItsText.
//
// The inner box is 30px tall and its text sits well above that. With the
// overflow visible the text's own baseline is what reaches the line outside;
// with it hidden, the box's bottom edge is.
func TestABoxWithHiddenOverflowGivesItsBottomEdgeAndNotItsText(t *testing.T) {
	if got := baselineBesideAnInlineBlock(t, "hidden"); got != 30 {
		t.Errorf("with overflow: hidden the baseline is at %gpx, want the inner "+
			"box's bottom edge at 30", got)
	}
	visible := baselineBesideAnInlineBlock(t, "")
	if visible == 30 {
		t.Fatal("the fixture is wrong: the visible case gives the same answer, so " +
			"the test above measures nothing")
	}
	if visible > 30 {
		t.Errorf("with overflow: visible the baseline is at %gpx, which is below the "+
			"inner box's bottom edge; the text inside it is higher than that", visible)
	}
}

// TestAnEmptyBoxStillFallsToTheOuterEdge is the case the rule above must not
// swallow: with nothing to give a baseline anywhere inside, §10.8.1's other
// sentence applies and the inline-block's own bottom margin edge is the answer —
// which is 50 here, because of the outer box's 20px of bottom padding.
func TestAnEmptyBoxStillFallsToTheOuterEdge(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="p"><span class="outer"><span class="inner"></span></span>XX</div>`,
		`#p { font-family: Courier; font-size: 20px }
		 .outer { display: inline-block; padding-bottom: 20px }
		 .inner { display: block; width: 30px; height: 30px }`)
	f := find(t, root, "p")
	got := f.Lines[0].Rect.Y.Add(f.Lines[0].Baseline).Px()
	if got != 50 {
		t.Errorf("the baseline is at %gpx, want the inline-block's bottom margin "+
			"edge at 50", got)
	}
}

// TestAnEmptyBoxWithHiddenOverflowGivesItsOwnEdge. Its own, and not the box
// outside it: the two differ by the outer box's padding, which is what makes
// this a test of *which* bottom edge was taken.
func TestAnEmptyBoxWithHiddenOverflowGivesItsOwnEdge(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="p"><span class="outer"><span class="inner hidden"></span></span>XX</div>`,
		`#p { font-family: Courier; font-size: 20px }
		 .outer { display: inline-block; padding-bottom: 20px }
		 .inner { display: block; width: 30px; height: 30px }
		 .hidden { overflow: hidden }`)
	f := find(t, root, "p")
	got := f.Lines[0].Rect.Y.Add(f.Lines[0].Baseline).Px()
	if got != 30 {
		t.Errorf("the baseline is at %gpx, want the inner box's own bottom edge at "+
			"30 rather than the outer's at 50", got)
	}
}
