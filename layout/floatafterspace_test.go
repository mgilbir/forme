package layout

import "testing"

// A float written after a space, CSS 2.1 §9.5 and §4.1.2 together.
//
// A left float shifts left until it touches the containing block's edge, and it
// goes beside the line it was written on when there is room for it there. What
// counts as room is where the line has got to — and §4.1.2's third rule takes a
// collapsible space off the end of a line, so a space between the content and
// the float is not on the finished line and is not something the float has to
// get past.
//
// The documents below are two boxes of 96px in 192px of room, which is exactly
// enough for both and one pixel short of enough for anything more. Nothing but
// the rule under test can decide them: an engine that counts the space has four
// pixels too few and puts the float on the next line, and there is no other way
// to be four pixels out.

const floatAfterSpaceCSS = `
html, body { margin: 0; padding: 0 }
#d { width: 192px; height: 96px }
#b, #f { display: inline-block; width: 96px; height: 96px }
#f { float: left }
`

// TestAFloatIgnoresTheCollapsibleSpaceWrittenBeforeIt is the rule, and the pair
// of documents is the assertion: the same two boxes with and without a newline
// between the tags have to land in the same place.
//
// Source white space is not layout. An author who writes the two elements on
// separate lines has written the same document as one who does not, and an
// engine that placed the float differently for the two was reading the markup's
// indentation as content.
func TestAFloatIgnoresTheCollapsibleSpaceWrittenBeforeIt(t *testing.T) {
	for _, tc := range []struct{ name, html string }{
		{"a space between the tags",
			`<div id="d"><div id="b"></div> <div id="f"></div></div>`},
		{"a newline between the tags",
			"<div id=\"d\"><div id=\"b\"></div>\n<div id=\"f\"></div></div>"},
		{"nothing between the tags",
			`<div id="d"><div id="b"></div><div id="f"></div></div>`},
	} {
		root := layoutOf(t, 400, tc.html, noDefaults+floatAfterSpaceCSS)
		d := find(t, root, "d")
		f, b := find(t, root, "f"), find(t, root, "b")
		if got := relY(t, f, d); got.Px() != 0 {
			t.Errorf("%s: the float is %gpx down, want 0 — it fits beside the "+
				"line and §4.1.2 does not leave the space before it there",
				tc.name, got.Px())
		}
		if got := relX(t, f, d); got.Px() != 0 {
			t.Errorf("%s: the float is at %gpx, want 0 — a left float shifts "+
				"left until it touches the containing block's edge",
				tc.name, got.Px())
		}
		// And the inline-block moved over to make room, which is what says the
		// float really is beside it rather than merely drawn at the top.
		if got := relX(t, b, d); got.Px() != 96 {
			t.Errorf("%s: the inline-block is at %gpx, want 96 — the line box "+
				"begins at the float's right edge", tc.name, got.Px())
		}
	}
}

// TestAFloatStillWaitsForContentThatFillsTheLine is the containment case, and
// the reason the test above cannot be satisfied by never pushing a float down.
//
// One pixel wider on the box before it and there is no longer room, so the float
// goes below the line — which is §9.5.1's rule 6 doing what it is for.
func TestAFloatStillWaitsForContentThatFillsTheLine(t *testing.T) {
	root := layoutOf(t, 400,
		`<div id="d"><div id="b"></div> <div id="f"></div></div>`,
		noDefaults+floatAfterSpaceCSS+`#b { width: 97px }`)
	d, f := find(t, root, "d"), find(t, root, "f")
	if got := relY(t, f, d); got.Px() <= 0 {
		t.Errorf("the float is %gpx down, want more than 0 — 97 and 96 do not "+
			"fit in 192, so it belongs below the line", got.Px())
	}
	if got := relX(t, f, d); got.Px() != 0 {
		t.Errorf("the float is at %gpx, want 0 — pushed down is still flush left",
			got.Px())
	}
}

// TestAFloatClearsAPreservedSpaceBeforeIt is the other containment case: only a
// space the end of a line removes is discounted.
//
// Under "white-space: pre" the space is content, it stays on the line, and the
// float is that much further along — 96 plus a space is more than 192 less 96,
// so it goes below. The two documents differ in nothing but the white-space
// value, which is what makes this a statement about §4.1.2 rather than about
// floats.
func TestAFloatClearsAPreservedSpaceBeforeIt(t *testing.T) {
	const src = `<div id="d"><div id="b"></div> <div id="f"></div></div>`
	root := layoutOf(t, 400, src, noDefaults+floatAfterSpaceCSS+`#d { white-space: pre }`)
	d, f := find(t, root, "d"), find(t, root, "f")
	if got := relY(t, f, d); got.Px() <= 0 {
		t.Errorf("the float is %gpx down, want more than 0 — a preserved space "+
			"is content and the float has to clear it", got.Px())
	}
}
