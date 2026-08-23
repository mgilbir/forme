package layout

import "testing"

// tab-size, CSS Text §8.1, and the two different boxes it asks about.
//
// "The tab-size property determines the tab size used to render preserved tab
// characters. A <number> represents the measure as a multiple of the space
// character's advance width (U+0020), including its associated letter-spacing
// and word-spacing." And the measure is taken in the *block container*.
//
// So a tab stop is settled by two questions about two boxes: how many, which the
// box the tab is in answers because the property applies to inline boxes; and
// how wide one is, which the block container answers because a paragraph's tab
// stops do not move when one word in it is emphasised.

// tabX is where the character after a preserved tab begins.
func tabX(t *testing.T, markup, css string) float64 {
	t.Helper()
	root := layoutOf(t, 900, `<div id="p">`+markup+`</div>`,
		noDefaults+`#p { font-family: Courier; font-size: 20px; white-space: pre } `+css)
	for _, l := range find(t, root, "p").Lines {
		for _, r := range l.Runs {
			if r.Text == "X" {
				return r.X.Px()
			}
		}
	}
	t.Fatalf("no run of X in %q", markup)
	return 0
}

// TestATabStopIsCountedInTheBlocksSpaces.
//
// A Courier space at 20px is 12px, so eight of them is 96 and four is 48.
func TestATabStopIsCountedInTheBlocksSpaces(t *testing.T) {
	for _, tc := range []struct {
		markup, css string
		want        float64
		what        string
	}{
		{`&#9;X`, ``, 96, "the initial eight"},
		{`&#9;X`, `#p { tab-size: 4 }`, 48, "four"},
		{`&#9;X`, `#p { tab-size: 0 }`, 0, "none at all"},
		{`&#9;X`, `#p { tab-size: 40px }`, 40, "a length, which is itself"},
		{`&#9;X`, `#p { tab-size: 2em }`, 40, "a length in em"},
		// The space is the *block's*, so a span set larger does not widen it.
		{`<span>&#9;X</span>`, `#p { tab-size: 4 } span { font-size: 40px }`, 48,
			"a span set larger than the paragraph"},
		{`<span>&#9;X</span>`, `#p { tab-size: 4 } span { letter-spacing: 20px }`, 48,
			"a span tracked out"},
		// And the two spacings on the *block* do widen it: a space in a tracked
		// paragraph is wider, and a tab stop is counted in those spaces.
		{`&#9;X`, `#p { tab-size: 4; letter-spacing: 5px }`, 68, "letter-spacing on the block"},
		{`&#9;X`, `#p { tab-size: 4; word-spacing: 5px }`, 68, "word-spacing on the block"},
		{`&#9;X`, `#p { tab-size: 4; letter-spacing: 3px; word-spacing: 2px }`, 68, "both"},
		// A length is a length and neither spacing touches it.
		{`&#9;X`, `#p { tab-size: 40px; letter-spacing: 5px }`, 40, "a length, tracked out"},
	} {
		if got := tabX(t, tc.markup, tc.css); got != tc.want {
			t.Errorf("%s: the character after the tab is at %gpx, want %g",
				tc.what, got, tc.want)
		}
	}
}

// TestTheValueComesFromTheBoxTheTabIsIn. tab-size applies to inline boxes, so a
// span may set stops of its own — which is a different question from *how wide*
// one of them is, and is answered by a different box.
func TestTheValueComesFromTheBoxTheTabIsIn(t *testing.T) {
	// The paragraph says two and the span says four: the tab inside the span
	// goes to 48, not to 24.
	got := tabX(t, `<span>&#9;X</span>`, `#p { tab-size: 2 } span { tab-size: 4 }`)
	if got != 48 {
		t.Errorf("the tab is at %gpx, want 48 — the span's own tab-size", got)
	}
	// And outside the span the paragraph's value stands.
	if got := tabX(t, `&#9;X`, `#p { tab-size: 2 } span { tab-size: 4 }`); got != 24 {
		t.Errorf("outside the span the tab is at %gpx, want 24", got)
	}
}

// TestAnInvalidTabSizeIsIgnored. The property takes a non-negative number or
// length; anything else leaves the value that was there, which with no other
// declaration is the initial eight.
func TestAnInvalidTabSizeIsIgnored(t *testing.T) {
	for _, tc := range []struct{ css, what string }{
		{`#p { tab-size: -4 }`, "a negative number"},
		{`#p { tab-size: -1em }`, "a negative length"},
	} {
		if got := tabX(t, `&#9;X`, tc.css); got != 96 {
			t.Errorf("%s: the tab is at %gpx, want the initial eight at 96", tc.what, got)
		}
	}
}
