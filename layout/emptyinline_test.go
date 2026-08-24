package layout

import (
	"testing"
)

// An inline box with nothing in it but its own edges, CSS 2.1 §9.4.2.
//
//	Line boxes that contain no text, no preserved white space, no inline
//	elements with non-zero margins, padding, or borders [...] must be treated
//	as zero-height line boxes.
//
// So a line holding an inline box that *has* one of the three is a line, and the
// box is on it: its border is drawn and its leading counts towards the line's
// height. Each of those three sentences was a separate fault, and the suite's
// margin-padding-clear/margin-right-114 needed all three — a green square that
// came out blank.

// emptyInline lays out one empty inline box and reports what reached the page.
func emptyInline(t *testing.T, span string) (height float64, lines int, fills int) {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d"><span style="`+span+`"></span></div>`,
		noDefaults+`#d { font-size: 20px; line-height: 1 }`)
	f := find(t, root, "d")
	return f.BorderRect.H.Px(), len(f.Lines), len(fillsOfAny(Paint(root)))
}

// TestAnInlineBoxWithNothingButItsOwnEdgesIsOnTheLine.
func TestAnInlineBoxWithNothingButItsOwnEdgesIsOnTheLine(t *testing.T) {
	for _, tc := range []struct{ what, span string }{
		{"a border", "border: 10px solid rgb(0,0,255)"},
		{"a border on one side", "border-right: 10px solid rgb(0,0,255)"},
		{"padding", "padding: 10px"},
		{"a margin", "margin-left: 10px"},
		// The case that has to be asked about each of the three separately: they
		// cancel, so the box takes up no room, and it still has a border to draw.
		{"a border a negative margin cancels",
			"border-right: 10px solid rgb(0,0,255); margin-right: -10px"},
	} {
		h, lines, _ := emptyInline(t, tc.span)
		if lines != 1 {
			t.Errorf("%s: %d line(s), want 1 — a line holding an inline box with one "+
				"of the three is not a zero-height line box", tc.what, lines)
		}
		if h != 20 {
			t.Errorf("%s: the block is %gpx tall, want 20 — the line takes the "+
				"strut's height", tc.what, h)
		}
	}
}

// TestAnInlineBoxWithNoEdgesStillMakesNoLine is the containment half, and the
// half that keeps an empty <span> from putting a blank line into every document.
func TestAnInlineBoxWithNoEdgesStillMakesNoLine(t *testing.T) {
	for _, span := range []string{"", "color: red", "border: 0 solid rgb(0,0,255)",
		"padding: 0", "margin: 0"} {
		h, lines, _ := emptyInline(t, span)
		if lines != 0 || h != 0 {
			t.Errorf("<span style=%q> made %d line(s) and %gpx of height; it has none "+
				"of the three, so its line is one of §9.4.2's zero-height ones",
				span, lines, h)
		}
	}
}

// TestABorderANegativeMarginCancelsIsStillDrawn.
//
// The margin moves what comes after the box; it does not reach back into the
// border. Read as a sum the box had no edges at all, so no item was emitted, the
// line did not exist, and two hundred pixels of green were not drawn — which is
// what margin-right-114 saw.
func TestABorderANegativeMarginCancelsIsStillDrawn(t *testing.T) {
	_, _, plain := emptyInline(t, "border-right: 10px solid rgb(0,0,255)")
	_, _, cancelled := emptyInline(t,
		"border-right: 10px solid rgb(0,0,255); margin-right: -10px")
	if plain == 0 {
		t.Fatal("a border on its own painted nothing, so this test says nothing")
	}
	if cancelled != plain {
		t.Errorf("the border painted %d rectangle(s) with a negative margin against "+
			"it and %d without one", cancelled, plain)
	}
}

// TestAnEmptyInlineBoxLeadsTheLineItIsOn is §10.8.1: the line box is as tall as
// the boxes on it, and an inline box is on it whether or not it has content of
// its own.
//
// paragraph/stacking.go records an attempt at this for *every* empty inline box,
// which cost nineteen tests: an item that stands for nothing is walked by
// everything downstream, and one that is neither TrimAtEnd nor Inset stops the
// trailing-space trimming. These are the box's own edges, which are Inset and
// which every walk already knows about — so the case that note could not have
// is the one this reaches.
func TestAnEmptyInlineBoxLeadsTheLineItIsOn(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="d"><span style="font-size: 200px; line-height: 1; `+
			`border-right: 10px solid rgb(0,0,255)"></span></div>`,
		noDefaults+`#d { font-size: 20px; line-height: 1 }`)
	if h := find(t, root, "d").BorderRect.H.Px(); h != 200 {
		t.Errorf("the block is %gpx tall and the inline box on its line is 200; the "+
			"line box is as tall as what is on it", h)
	}
}

// TestABlockThatSplitsAnInlineKeepsItsFarEdge is the shape margin-right-114 is
// written in: §9.2.1.1 cuts the inline in two around the block, and the border
// on the right belongs to the second half — which has nothing else in it at all.
func TestABlockThatSplitsAnInlineKeepsItsFarEdge(t *testing.T) {
	const span = `<span style="font-size: 200px; line-height: 1; ` +
		`border-right: 200px solid rgb(0,0,255); margin-right: -200px">`
	border := func(html string) Rect {
		t.Helper()
		got := fillsOf(paintOf(t, html, noDefaults+`#d { font-size: 20px }`), blue)
		if len(got) != 1 {
			t.Fatalf("%d blue rectangles, want the one border: %v", len(got), got)
		}
		return got[0]
	}
	split := border(`<div id="d" style="width: 200px">` + span + `<div></div></span></div>`)
	whole := border(`<div id="d" style="width: 200px">` + span + `</span></div>`)
	if split.W != bgpx(200) {
		t.Errorf("the border is %v wide, want 200", split.W)
	}
	// The same border the box would have drawn had nothing split it. Its height
	// is the face's content area rather than the line-height, which is CSS 2.1
	// §10.6.1 and is not what this is about — so it is compared and not written
	// down.
	if split.H != whole.H {
		t.Errorf("the border of the split box is %v tall and of the whole box %v; "+
			"the block between them does not change how tall the inline is",
			split.H, whole.H)
	}
}
