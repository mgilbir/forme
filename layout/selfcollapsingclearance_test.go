package layout

import "testing"

// Where a self-collapsing box *is*, which only clearance can see.
//
// CSS 2.1 §8.3.1 ends with a note about it: "the position of the element's top
// border edge is the same as it would have been if the element had a non-zero
// bottom border". With a bottom border the box's two margins would not have met,
// so its top edge is after its own top margin — not after the single run the two
// collapsed into.
//
// Nothing else asks. The box has no height and paints nothing, so the difference
// between the two positions is invisible until §9.5.2 measures clearance against
// it, and then it is the difference between clearing a float and not clearing it
// at all. CSS2/floats-clear/margin-collapse-clear-013 works the arithmetic out
// in its own source.

// clearedBox lays out a float, a self-collapsing cleared box after it, and a
// block after their parent, and returns where each landed.
func clearedBox(t *testing.T, marginTop, marginBottom string) (parentH, clearedY, nextY float64) {
	t.Helper()
	root := layoutOf(t, 600, `<div id="parent"><div id="fl"></div><div id="cl"></div></div>
		<div id="next"></div>`,
		`#parent { border-top: 1px solid black; width: 300px }
		#fl { float: left; height: 100px; width: 100px }
		#cl { clear: left; margin-top: `+marginTop+`; margin-bottom: `+marginBottom+` }
		#next { height: 100px }`)
	p := find(t, root, "parent")
	return p.BorderRect.H.Px(), find(t, root, "cl").BorderRect.Y.Px(),
		find(t, root, "next").BorderRect.Y.Px()
}

// TestASelfCollapsingBoxClearsFromItsOwnTopMargin is the bug.
//
// A 40px top margin and a 140px bottom one collapse to 140. Measured there the
// box is already below the 100px float and clears nothing, so its whole run
// leaves through the parent's open bottom edge and the parent is nothing but its
// border. Measured at 40, where §8.3.1 puts its top edge, it clears 60 — which
// lands the edge on the float's bottom and leaves the rest of the bottom margin
// inside the parent.
func TestASelfCollapsingBoxClearsFromItsOwnTopMargin(t *testing.T) {
	parentH, clearedY, nextY := clearedBox(t, "40px", "140px")
	if parentH != 201 {
		t.Errorf("the parent is %gpx tall, want 201 — a 1px border over 100px of "+
			"float and the 100px of bottom margin that the top margin does not "+
			"cover", parentH)
	}
	// 8 of body margin, 1 of border, 100 of float.
	if clearedY != 109 {
		t.Errorf("the cleared box's top border edge is at %gpx, want 109 — level "+
			"with the bottom of the float it clears", clearedY)
	}
	if nextY != 209 {
		t.Errorf("the block after the parent is at %gpx, want 209", nextY)
	}
}

// TestAClearedBoxAlreadyBelowTheFloatGetsNoClearance is the containment
// argument, and it is §9.5.2's own rule: clearance is a difference, so a box
// whose top margin alone already carries it past the float gets none.
func TestAClearedBoxAlreadyBelowTheFloatGetsNoClearance(t *testing.T) {
	parentH, clearedY, _ := clearedBox(t, "150px", "10px")
	// The margins collapse to 150; the top edge is at 150, well below the
	// float's 100, so nothing is cleared and the whole run leaves through the
	// parent's bottom edge. The parent is its border over the float.
	if clearedY != 159 {
		t.Errorf("the cleared box is at %gpx, want 159 — its own top margin "+
			"already carries it past the float, so §9.5.2 gives it no clearance",
			clearedY)
	}
	// And with no clearance nothing separates the run, so the whole of it
	// leaves through the parent's open bottom edge and the parent is its border
	// alone — an ordinary block's auto height does not include the floats in it.
	if parentH != 1 {
		t.Errorf("the parent is %gpx tall, want 1: its border and nothing else",
			parentH)
	}
}

// TestABoxWithHeightIsUnchanged. The note is about a box whose two margins
// collapse *through* it. One with a height has a top edge after its top margin
// for the ordinary reason and never reaches this.
func TestABoxWithHeightIsUnchanged(t *testing.T) {
	root := layoutOf(t, 600, `<div id="parent"><div id="fl"></div><div id="cl"></div></div>`,
		`#parent { border-top: 1px solid black; width: 300px }
		#fl { float: left; height: 100px; width: 100px }
		#cl { clear: left; height: 10px; margin-top: 40px; margin-bottom: 140px }`)
	if got := find(t, root, "cl").BorderRect.Y.Px(); got != 109 {
		t.Errorf("a cleared box with a height is at %gpx, want 109", got)
	}
}

// TestTheRunBeforeTheBoxCountsToo. §8.3.1's note puts the top edge after the
// box's own top margin, and "after" is measured from where the flow had reached
// — which includes whatever the preceding siblings' margins collapsed into. A
// version that used the box's own margin alone measures from the wrong place,
// and the error is invisible until that run is what carries the box past the
// float.
func TestTheRunBeforeTheBoxCountsToo(t *testing.T) {
	root := layoutOf(t, 600, `<div id="parent">
		<div id="fl"></div>
		<div id="pre"></div>
		<div id="cl"></div>
	</div>`,
		`#parent { border-top: 1px solid black; width: 300px }
		#fl { float: left; height: 100px; width: 100px }
		#pre { height: 10px; margin-bottom: 150px }
		#cl { clear: left; margin-top: 40px; margin-bottom: 10px }`)
	// The preceding margin of 150 and the box's own 40 collapse to 150, so the
	// box's top edge is at 10 + 150 = 160 — already past the float, and §9.5.2
	// gives it no clearance. 8 of body margin and 1 of border on top of that.
	if got := find(t, root, "cl").BorderRect.Y.Px(); got != 169 {
		t.Errorf("the cleared box is at %gpx, want 169; the 150px margin in front "+
			"of it is part of where its top edge is", got)
	}
}
