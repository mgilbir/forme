package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A zero-width space between two spaces, which §4.1.1 does not collapse.
//
// The rule collapses white space that is *adjacent*: "any collapsible space
// immediately following another collapsible space is collapsed to have zero
// advance width". A zero-width space is not white space — it is a character —
// so two spaces with one between them are not immediately following anything
// and both survive.
//
// The suite states it as an equivalence in a comment of its own, "U+00A0 is
// exactly equivalent to U+200B U+0020 U+200B", and tests it four times: a cell
// holding five rows that must each be five characters wide, one row written with
// three no-break spaces and two written with spaces fenced by zero-width ones.
//
// The character was dropped outright — "a break opportunity and nothing else" —
// so by the time the collapsing ran the spaces were adjacent and two of the
// three vanished. The rows came out three characters wide inside a five-character
// box, which is a red square showing through a green one.

// spanWidth is the width of the background of the one span in a document, which
// is the width of the content inside it.
func spanWidth(t *testing.T, inner string) style.Unit {
	t.Helper()
	got := fillsOf(paintOf(t, `<div id="d">X<span id="s">`+inner+`</span>X</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }
		 #s { background: rgb(0,128,0) }`), green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds for %q: %v", len(got), inner, got)
	}
	return got[0].W
}

// TestAZeroWidthSpaceKeepsTheSpacesEitherSideOfItApart is the suite's
// equivalence, measured.
func TestAZeroWidthSpaceKeepsTheSpacesEitherSideOfItApart(t *testing.T) {
	// Three no-break spaces: three characters, and nothing collapses because a
	// no-break space is not white space for this purpose at all.
	want := spanWidth(t, "\u00a0\u00a0\u00a0")
	if want != bgpx(36) {
		t.Fatalf("three no-break spaces are %v wide, want 36 — three 12px characters",
			want)
	}
	for _, tc := range []struct{ what, inner string }{
		{"spaces fenced on both sides", "​ ​ ​ ​"},
		{"spaces fenced between them", " ​ ​ "},
	} {
		if got := spanWidth(t, tc.inner); got != want {
			t.Errorf("%s: %v wide, and three no-break spaces are %v; the suite calls "+
				"the two spellings exactly equivalent", tc.what, got, want)
		}
	}
}

// TestAdjacentSpacesStillCollapse is the containment case, and it is the rule
// itself: with nothing between them, three spaces are one.
func TestAdjacentSpacesStillCollapse(t *testing.T) {
	if got := spanWidth(t, "   "); got != bgpx(12) {
		t.Errorf("three adjacent spaces are %v wide, want 12 — they collapse to one",
			got)
	}
	// And across an inline boundary, which is the half of the rule that needs
	// the state to travel between boxes. It is measured between the two Xs
	// rather than by the first span's background: the span that loses its space
	// is the *second* one, and the first is 12px wide either way.
	if got := xSpan(t, `X<span> </span><span> </span>X`); got != bgpx(24) {
		t.Errorf("a space in one span followed by a space in another put the two Xs "+
			"%v apart, want 24 — one for the X and one for the single space they "+
			"collapse to", got)
	}
}

// xSpan is the distance between the first and last runs of text a document set.
func xSpan(t *testing.T, markup string) style.Unit {
	t.Helper()
	var xs []style.Unit
	for _, op := range paintOf(t, `<div id="d">`+markup+`</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }`) {
		if v, ok := op.(DrawText); ok {
			xs = append(xs, v.At.X)
		}
	}
	if len(xs) < 2 {
		t.Fatalf("%d runs of text for %q", len(xs), markup)
	}
	return xs[len(xs)-1].Sub(xs[0])
}

// TestAZeroWidthSpaceInABoxOfItsOwnStillSeparates. The character may be the
// whole content of an element, which is how it is written when a stylesheet
// rather than the prose decides where a word may break — and the state has to
// cross that boundary for the rule to see it.
func TestAZeroWidthSpaceInABoxOfItsOwnStillSeparates(t *testing.T) {
	got := fillsOf(paintOf(t,
		`<div id="d">X<span id="s"> </span><span>​</span><span> </span>X</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }
		 #s { background: rgb(0,128,0) }`), green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds: %v", len(got), got)
	}
	if got[0].W != bgpx(12) {
		t.Errorf("the first span is %v wide, want 12", got[0].W)
	}
	// The second space survived, so the run from the first X to the last is
	// three characters rather than two.
	if got := xSpan(t, `X<span> </span><span>​</span><span> </span>X`); got != bgpx(36) {
		t.Errorf("the two Xs are %v apart, want 36 — one for the X and two for the "+
			"spaces the zero-width space kept apart", got)
	}
}
