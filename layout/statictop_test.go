package layout

import (
	"strings"
	"testing"
)

// Where a block-level out-of-flow box sits when it was written among words,
// CSS 2.1 §10.6.4.
//
//	the static position for 'top' is the distance from the top edge of the
//	containing block to the top margin edge of a hypothetical box that would
//	have been the first box of the element if its 'position' property had been
//	'static'
//
// The hypothetical box is block-level, so it would have *split* the inline
// content it was written among: what precedes it stays on the line above and its
// own top edge is that line's bottom. Ours took the line's top, so an absolutely
// positioned box written after a word sat a line too high.

// staticTop lays out one absolutely positioned box among words and returns where
// it landed.
func staticTop(t *testing.T, markup string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-size: 20px; line-height: 20px; width: 300px } `+
			`#b { height: 30px } `+
			`#a { position: absolute; width: 10px; height: 10px }`)
	return find(t, root, "a").BorderRect.Y.Px()
}

// TestAStaticTopIsBelowTheLineItFollows.
func TestAStaticTopIsBelowTheLineItFollows(t *testing.T) {
	for _, tc := range []struct {
		what, markup string
		want         float64
	}{
		{"after a word", `x<div id="a"></div>`, 20},
		{"after two lines", `x<br>y<div id="a"></div>`, 40},
		// Nothing precedes it on the line, so the hypothetical block is above
		// that line rather than below it and the line's top is the answer.
		{"before any word", `<div id="a"></div>x`, 0},
		// And a block before it is not a line at all: the walk over blocks
		// already had this right and must keep it.
		{"after a block", `<div id="b"></div><div id="a"></div>`, 30},
	} {
		if got := staticTop(t, tc.markup); got != tc.want {
			t.Errorf("%s: the box is at %gpx, want %g", tc.what, got, tc.want)
		}
	}
}

// TestAStaticTopFollowsTheLastLineHoweverManyThereAre. The number is the
// block's own height rather than one written down: a box after all of a
// paragraph's lines sits where the next block would, which is the bottom of the
// last one.
func TestAStaticTopFollowsTheLastLineHoweverManyThereAre(t *testing.T) {
	markup := strings.Repeat("word ", 40) + `<div id="a"></div>`
	root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-size: 20px; line-height: 20px; width: 300px } `+
			`#a { position: absolute; width: 10px; height: 10px }`)
	lines := len(find(t, root, "d").Lines)
	if lines < 3 {
		t.Fatalf("the fixture made %d line(s); it is meant to wrap several times", lines)
	}
	want := find(t, root, "d").BorderRect.H
	if got := find(t, root, "a").BorderRect.Y; got != want {
		t.Errorf("the box is at %v and the %d lines before it come to %v", got, lines, want)
	}
}

// TestAnInlineLevelBoxKeepsThePenPosition is the containment half, and §10.3.7's
// own words are the reason: with a static position there is no blockification,
// so the hypothetical box of an absolutely positioned <span> is on the line
// where it was written. Only a box that was block-level to begin with gets the
// line below.
func TestAnInlineLevelBoxKeepsThePenPosition(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d">x<span id="a"></span></div>`,
		noDefaults+`#d { font-size: 20px; line-height: 20px; width: 300px } `+
			`#a { position: absolute; width: 10px; height: 10px }`)
	if got := find(t, root, "a").BorderRect.Y.Px(); got != 0 {
		t.Errorf("an absolutely positioned <span> is at %gpx, want 0 — it was "+
			"inline-level before §9.7 blockified it, so its hypothetical box is on "+
			"the line it was written on", got)
	}
}

// TestAPseudoElementIsInlineLevelToo, which is the same rule reaching the walk
// that had not recorded it.
//
// A pseudo-element is inline by default, so "::after { position: absolute }" has
// a hypothetical static box on the line it was written on. The generated-content
// walk never set the flag that says so, so every positioned pseudo-element read
// as block-level — invisible until a block-level one started taking the line
// below, and then it was a line out.
func TestAPseudoElementIsInlineLevelToo(t *testing.T) {
	for _, tc := range []struct{ what, css string }{
		{"an inline pseudo-element", ``},
		{"one declared inline", `display: inline;`},
	} {
		root := layoutOf(t, 600, `<div id="d">x</div>`,
			noDefaults+`#d { font-size: 20px; line-height: 20px; width: 300px } `+
				`#d::after { content: "y"; `+tc.css+` position: absolute; `+
				`width: 10px; height: 10px; background: rgb(0,0,255) }`)
		got := fillsOf(Paint(root), blue)
		if len(got) != 1 {
			t.Fatalf("%s: %d blue fills, want the pseudo-element's", tc.what, len(got))
		}
		if got[0].Y != 0 {
			t.Errorf("%s: it is at %v, want 0 — it was inline-level before §9.7 "+
				"blockified it", tc.what, got[0].Y)
		}
	}
	// And one the stylesheet made block-level takes the line below, which is
	// what says the flag is read rather than assumed.
	root := layoutOf(t, 600, `<div id="d">x</div>`,
		noDefaults+`#d { font-size: 20px; line-height: 20px; width: 300px } `+
			`#d::after { content: "y"; display: block; position: absolute; `+
			`width: 10px; height: 10px; background: rgb(0,0,255) }`)
	got := fillsOf(Paint(root), blue)
	if len(got) != 1 {
		t.Fatalf("%d blue fills, want the pseudo-element's", len(got))
	}
	if got[0].Y != bgpx(20) {
		t.Errorf("a block-level pseudo-element is at %v, want 20 — its hypothetical "+
			"box would have split the line", got[0].Y)
	}
}
