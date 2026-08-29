package layout

import (
	"testing"
)

// Which of a pair of offsets gives way when an absolutely positioned box is
// pinned by both, CSS 2.1 §10.3.7.
//
//	If none of the three is 'auto': if both 'margin-left' and 'margin-right'
//	are 'auto', solve the equation under the extra constraint that the two
//	margins get equal values [...] If [they are not], the values are
//	over-constrained and one of the used values will have to be ignored. If
//	the 'direction' property of the containing block has the value 'ltr', the
//	specified value of 'right' is ignored [...]. If 'rtl', 'left' is ignored.
//
// Read as "the end always gives way", a right-to-left box pinned by both sat
// against the wrong edge — the offset the author meant to hold it was the one
// dropped. The suite's absolute-replaced-width-071 and -076 are that shape.

// absBox lays out one absolutely positioned box and returns its border rect.
func absBox(t *testing.T, dir, box string) Rect {
	t.Helper()
	root := layoutOf(t, 600, `<div id="cb"><div id="a"></div></div>`,
		noDefaults+`#cb { direction: `+dir+`; position: relative; width: 200px; `+
			`height: 100px } #a { position: absolute; height: 10px; `+box+` }`)
	return find(t, root, "a").BorderRect
}

// TestAnOverConstrainedBoxDropsTheOffsetItsDirectionNames.
func TestAnOverConstrainedBoxDropsTheOffsetItsDirectionNames(t *testing.T) {
	const pinned = "left: 20px; right: 30px; width: 50px"
	// Left to right: "right" is ignored and the box stays at its left offset.
	if got := absBox(t, "ltr", pinned).X; got != bgpx(20) {
		t.Errorf("in a left-to-right containing block the box is at %v, want 20 — "+
			"the right offset is the one ignored", got)
	}
	// Right to left: "left" is ignored and the box stays at its right offset,
	// which is 200 less the 30 it asked for and its own 50.
	if got := absBox(t, "rtl", pinned).X; got != bgpx(120) {
		t.Errorf("in a right-to-left containing block the box is at %v, want 120 — "+
			"the left offset is the one ignored", got)
	}
}

// TestTheDroppedOffsetIsDroppedAfterTheMargins. The margins are given, so they
// are part of the equation the offset is solved from rather than something the
// solved offset then displaces.
func TestTheDroppedOffsetIsDroppedAfterTheMargins(t *testing.T) {
	const pinned = "left: 20px; right: 30px; width: 50px; margin-left: 10px"
	if got := absBox(t, "rtl", pinned).X; got != bgpx(120) {
		t.Errorf("the box is at %v, want 120 — the right offset holds its right "+
			"edge, so its own margin-left moves the border box back to where the "+
			"width leaves it", got)
	}
	if got := absBox(t, "ltr", pinned).X; got != bgpx(30) {
		t.Errorf("the box is at %v, want 30 — the left offset and the margin", got)
	}
}

// TestABoxWithRoomToSpareIsNotOverConstrained is the containment half: the rule
// only fires where every one of the three is given, and it must not reach the
// branches that resolve an auto margin or an auto offset.
func TestABoxWithRoomToSpareIsNotOverConstrained(t *testing.T) {
	for _, tc := range []struct {
		what, box string
		wantLTR   float64
		wantRTL   float64
	}{
		// An auto width takes the slack, so nothing is over-constrained and the
		// box spans both offsets whichever way the text runs.
		{"an auto width", "left: 20px; right: 30px", 20, 20},
		// Auto margins centre it, in both directions.
		{"auto margins", "left: 20px; right: 30px; width: 50px; margin: 0 auto", 70, 70},
		// One offset auto: the other decides, and there is nothing to drop.
		{"no right offset", "left: 20px; width: 50px", 20, 20},
		{"no left offset", "right: 30px; width: 50px", 120, 120},
	} {
		if got := absBox(t, "ltr", tc.box).X; got != bgpx(tc.wantLTR) {
			t.Errorf("%s, ltr: the box is at %v, want %v", tc.what, got, bgpx(tc.wantLTR))
		}
		if got := absBox(t, "rtl", tc.box).X; got != bgpx(tc.wantRTL) {
			t.Errorf("%s, rtl: the box is at %v, want %v", tc.what, got, bgpx(tc.wantRTL))
		}
	}
}

// TestTheVerticalAxisAlwaysDropsTheBottom. §10.6.4 has no direction clause: a
// box pinned by "top" and "bottom" with a height keeps its top whatever the
// text does, because there is no writing direction on that axis to ask.
func TestTheVerticalAxisAlwaysDropsTheBottom(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		root := layoutOf(t, 600, `<div id="cb"><div id="a"></div></div>`,
			noDefaults+`#cb { direction: `+dir+`; position: relative; width: 200px; `+
				`height: 200px } #a { position: absolute; top: 20px; bottom: 30px; `+
				`height: 50px; width: 10px }`)
		if got := find(t, root, "a").BorderRect.Y; got != bgpx(20) {
			t.Errorf("%s: the box is at %v, want 20 — the bottom offset is the one "+
				"ignored, whichever way the text runs", dir, got)
		}
	}
}
