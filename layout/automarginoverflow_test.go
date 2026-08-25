package layout

import (
	"testing"
)

// What an auto margin is worth when the box is already too wide, CSS 2.1
// §10.3.3.
//
//	If 'width' is not 'auto' and [border-left-width + padding-left + width +
//	padding-right + border-right-width] plus any of 'margin-left' or
//	'margin-right' that are not 'auto' is larger than the width of the
//	containing block, then any 'auto' values for 'margin-left' or
//	'margin-right' are, for the following rules, treated as zero.
//
// It is easy to read past, because it is about what the rules below see rather
// than about a result. Without it an auto margin took a negative share of a
// slack there was none of and pulled the box further out: a 110px box with
// "margin-left: auto" in a 100px parent started ten pixels *left* of it.

// autoMarginBox lays out one box in a 100px parent and returns its border rect.
func autoMarginBox(t *testing.T, parent, child string) Rect {
	t.Helper()
	return autoMarginFragment(t, parent, child).BorderRect
}

// autoMarginFragment is the same, with the resolved margins as well — which is
// what the rule is written about. The box's position only shows the *left* one:
// nothing in block flow is placed by a right margin, so a right margin left
// unresolved is invisible in the geometry and visible here.
func autoMarginFragment(t *testing.T, parent, child string) *Fragment {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p"><div id="c"></div></div>`,
		noDefaults+`#p { width: 100px; `+parent+` } #c { height: 10px; `+child+` }`)
	return find(t, root, "c")
}

// TestAnAutoMarginIsZeroWhenTheBoxOverflows.
func TestAnAutoMarginIsZeroWhenTheBoxOverflows(t *testing.T) {
	for _, tc := range []struct{ what, child string }{
		{"on the left", "width: 110px; margin-left: auto"},
		{"on the right", "width: 110px; margin-right: auto"},
		{"on both, which would otherwise centre it", "width: 110px; margin: 0 auto"},
		// The suite's fixture: the width fits and the border and padding are
		// what push it over.
		{"pushed over by the border and padding",
			"width: 110px; margin-left: auto; padding: 0 10px; " +
				"border: 0 solid; border-left-width: 10px; border-right-width: 10px"},
	} {
		f := autoMarginFragment(t, "", tc.child)
		if got := f.BorderRect.X; got != 0 {
			t.Errorf("%s: the box starts at %v, want 0 — there is no slack for an "+
				"auto margin to take a share of, so it is treated as zero", tc.what, got)
		}
		// And *both* of them are zero, which the position alone cannot say:
		// nothing in block flow is placed by a right margin, so one left with a
		// negative share of the slack moves nothing and is wrong all the same.
		// The over-constrained rule below then gives the end margin whatever the
		// equation needs, which is where the overflow goes.
		if got := f.Margin.Left; got != 0 {
			t.Errorf("%s: the left margin came out %v, want 0", tc.what, got)
		}
	}
}

// TestADeclaredMarginSurvivesTheRule. §10.3.3 clears the *auto* ones, and a
// margin the author wrote is not one: the box starts at it, and whatever the
// overflow comes to goes to the end margin instead.
func TestADeclaredMarginSurvivesTheRule(t *testing.T) {
	f := autoMarginFragment(t, "", "width: 110px; margin-right: auto; margin-left: 5px")
	if got := f.Margin.Left; got != bgpx(5) {
		t.Errorf("the declared left margin came out %v, want 5 — the rule is about "+
			"the auto ones", got)
	}
	if got := f.BorderRect.X; got != bgpx(5) {
		t.Errorf("the box starts at %v, want 5 — its declared left margin and no "+
			"share of a slack there is none of", got)
	}
}

// TestAnAutoMarginStillTakesTheSlackWhenThereIsSome is the containment half. The
// rule fires on the overflow and nothing else: a box that fits is centred by two
// auto margins and pushed right by one, exactly as before.
func TestAnAutoMarginStillTakesTheSlackWhenThereIsSome(t *testing.T) {
	for _, tc := range []struct {
		what, child string
		want        float64
	}{
		{"one auto margin", "width: 50px; margin-left: auto", 50},
		{"two auto margins", "width: 50px; margin: 0 auto", 25},
		{"a box exactly as wide as its parent", "width: 100px; margin-left: auto", 0},
		{"an auto margin beside a declared one",
			"width: 50px; margin-left: auto; margin-right: 10px", 40},
	} {
		if got := autoMarginBox(t, "", tc.child).X; got != bgpx(tc.want) {
			t.Errorf("%s: the box starts at %v, want %v", tc.what, got, bgpx(tc.want))
		}
	}
}

// TestAnOverflowingBoxStillHangsOffTheEndItsDirectionNames.
//
// Once the auto margins are zero the equation is over-constrained, and §10.3.3
// resolves that by the containing block's direction like everything else: the
// box stays at the start edge and overflows the end one. So a right-to-left
// parent puts it against the right and lets it hang off the left.
func TestAnOverflowingBoxStillHangsOffTheEndItsDirectionNames(t *testing.T) {
	ltr := autoMarginBox(t, "direction: ltr", "width: 110px; margin-left: auto")
	if ltr.X != 0 {
		t.Errorf("in a left-to-right parent the box starts at %v, want 0", ltr.X)
	}
	rtl := autoMarginBox(t, "direction: rtl", "width: 110px; margin-left: auto")
	if rtl.X != bgpx(-10) {
		t.Errorf("in a right-to-left parent the box starts at %v, want -10 — it is "+
			"pinned to the right edge and hangs off the left", rtl.X)
	}
}

// TestAnAutoWidthIsNotAffected. The rule's own first words are "if 'width' is
// not 'auto'": a box with an auto width has no overflow to speak of, because the
// width is what absorbs whatever the margins leave.
func TestAnAutoWidthIsNotAffected(t *testing.T) {
	got := autoMarginBox(t, "", "margin-left: auto; margin-right: 20px")
	if got.X != 0 || got.W != bgpx(80) {
		t.Errorf("the box is at %v and %v wide; an auto width takes what the margins "+
			"leave and an auto margin beside it is zero", got.X, got.W)
	}
}
