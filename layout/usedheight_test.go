package layout

import (
	"testing"
)

// What a percentage height inside a box resolves against, CSS 2.1 §10.5.
//
// "The percentage is calculated with respect to the height of the containing
// block", and §10.7's minimum and maximum are applied before there is one — so a
// box at "height: 200px; max-height: 100px" is a hundred pixels tall and a child
// at "height: 100%" is a hundred. It was two: the height handed down was the one
// the box declared rather than the one it used.

// pairHeights lays out a box and a percentage-height child and returns both.
func pairHeights(t *testing.T, parent, child string) (float64, float64) {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p"><div id="c"></div></div>`,
		noDefaults+`#p { width: 100px; `+parent+` } #c { width: 50px; `+child+` }`)
	return find(t, root, "p").BorderRect.H.Px(), find(t, root, "c").BorderRect.H.Px()
}

// TestAPercentageHeightResolvesAgainstTheUsedHeight.
func TestAPercentageHeightResolvesAgainstTheUsedHeight(t *testing.T) {
	for _, tc := range []struct {
		what, parent string
		want         float64
	}{
		{"a maximum below the declared height", "height: 200px; max-height: 100px", 100},
		{"a minimum above it", "height: 50px; min-height: 150px", 150},
		{"both, with the minimum winning", "height: 20px; min-height: 150px; max-height: 100px", 150},
		{"a maximum that does not bind", "height: 80px; max-height: 100px", 80},
		{"no limits at all", "height: 120px", 120},
	} {
		gotParent, gotChild := pairHeights(t, tc.parent, "height: 100%")
		if gotParent != tc.want {
			t.Errorf("%s: the box is %gpx tall, want %g", tc.what, gotParent, tc.want)
		}
		if gotChild != tc.want {
			t.Errorf("%s: the child is %gpx tall and its containing block %gpx; a "+
				"percentage resolves against the height the box used",
				tc.what, gotChild, gotParent)
		}
	}
}

// TestAMaximumAloneDoesNotMakeAHeightDefinite is the containment half.
//
// A box with only a maximum is still as tall as its content, and CSS 2.1 makes a
// percentage against a height that "depends on content height" compute to auto.
// Clamping the *absence* of a height would turn that into a number and give
// every such child a height the specification says it does not have.
func TestAMaximumAloneDoesNotMakeAHeightDefinite(t *testing.T) {
	for _, parent := range []string{
		"max-height: 100px",
		"min-height: 50px",
		"min-height: 50px; max-height: 100px",
	} {
		_, child := pairHeights(t, parent, "height: 100%")
		if child != 0 {
			t.Errorf("with %q the child is %gpx tall; its containing block's height "+
				"depends on its content, so the percentage is auto", parent, child)
		}
	}
}

// TestTheChildsOwnPercentageLimitsUseTheSameHeight. §10.7 resolves a percentage
// "min-height" or "max-height" against the containing block's height like any
// other percentage, so they see the used height too.
func TestTheChildsOwnPercentageLimitsUseTheSameHeight(t *testing.T) {
	// The parent uses 100px, so the child's "max-height: 50%" is 50px and clamps
	// its declared 80px.
	_, child := pairHeights(t, "height: 200px; max-height: 100px", "height: 80px; max-height: 50%")
	if child != 50 {
		t.Errorf("the child is %gpx tall; half of its containing block's used "+
			"hundred pixels is fifty", child)
	}
	// And its "min-height: 50%" raises a smaller declared height to the same.
	_, child = pairHeights(t, "height: 200px; max-height: 100px", "height: 10px; min-height: 50%")
	if child != 50 {
		t.Errorf("the child is %gpx tall, want 50", child)
	}
}
