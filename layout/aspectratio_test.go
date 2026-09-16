package layout

import "testing"

// ratioHeight lays a document out and answers the border-box height of #d.
func ratioHeight(t *testing.T, decl string) float64 {
	t.Helper()
	frag := layoutOf(t, 600, `<div id="outer"><div id="d"></div></div>`,
		`body{margin:0} #outer{width:320px} #d{`+decl+`}`)
	return find(t, frag, "d").BorderRect.H.Px()
}

// ratioHeightOfFilledBox is the same with a line of text in the box, so that a
// ratio which is not one has a height to fall back to that is not zero.
//
// An empty box cannot tell "this is not a ratio" from "this ratio works out at
// nothing": both are nought pixels. "16/0" read as a number is an infinite
// ratio and a height of zero, which is exactly what an empty box with no ratio
// at all comes out as — so the fixture that refuses it has to have something
// inside it.
func ratioHeightOfFilledBox(t *testing.T, decl string) float64 {
	t.Helper()
	frag := layoutOf(t, 600, `<div id="outer"><div id="d">x</div></div>`,
		`body{margin:0} #outer{width:320px} #d{font: 16px/20px serif; `+decl+`}`)
	return find(t, frag, "d").BorderRect.H.Px()
}

// TestAnAspectRatioGivesAnEmptyBoxItsHeight is the shape a document reaches for
// wherever a box has to keep its proportions with nothing inside it to give
// them — a placeholder, a map, a chart area, the frame a video would go in.
//
// Before this the box was as tall as its content, which for an empty one is
// nothing at all.
func TestAnAspectRatioGivesAnEmptyBoxItsHeight(t *testing.T) {
	if got := ratioHeight(t, ``); got != 0 {
		t.Fatalf("an empty box with no ratio is %gpx tall; the fixture is wrong", got)
	}
	for _, c := range []struct {
		decl string
		want float64
	}{
		{`aspect-ratio: 16/9`, 180},   // 320 / (16/9)
		{`aspect-ratio: 16 / 9`, 180}, // the spaces the grammar allows
		{`aspect-ratio: 1`, 320},
		{`aspect-ratio: 2`, 160},
		{`aspect-ratio: 0.5`, 640},
		// "auto || <ratio>": the keyword beside a ratio is the replaced-element
		// form, where the element's own ratio wins and this is the fallback. A
		// box with no ratio of its own is what is left, so the declared one is
		// what it gets.
		{`aspect-ratio: auto 16/9`, 180},
		{`aspect-ratio: 16/9 auto`, 180},
	} {
		if got := ratioHeight(t, c.decl); got != c.want {
			t.Errorf("%s made the box %gpx tall, want %gpx (the width is 320)",
				c.decl, got, c.want)
		}
	}
}

// TestADeclaredHeightBeatsTheRatio.
//
// §4.1 gives a *preferred* ratio, which is what a box falls back on where an
// axis is not otherwise decided. A declared height decides it.
func TestADeclaredHeightBeatsTheRatio(t *testing.T) {
	if got := ratioHeight(t, `aspect-ratio: 16/9; height: 50px`); got != 50 {
		t.Errorf("the box is %gpx tall; a declared height decides the axis and the "+
			"ratio is what a box falls back on", got)
	}
}

// TestAnUnusableRatioIsNoRatio.
//
// §4.1 makes a ratio with a zero or a negative in it invalid rather than
// degenerate, and that matters: read as a number it would be an infinite or a
// negative height, and a box of either is not what the declaration asked for.
func TestAnUnusableRatioIsNoRatio(t *testing.T) {
	// The box holds one line, so a refused ratio leaves it a line tall and a
	// ratio read as a number does not.
	line := ratioHeightOfFilledBox(t, ``)
	if line <= 0 {
		t.Fatalf("a box with a line in it is %gpx tall; the fixture is wrong", line)
	}
	for _, decl := range []string{
		`aspect-ratio: auto`,
		`aspect-ratio: 0`,
		`aspect-ratio: 16/0`,
		`aspect-ratio: 0/9`,
		`aspect-ratio: -16/9`,
		`aspect-ratio: 16/-9`,
		`aspect-ratio: none`,
		`aspect-ratio: 16 9`,
		`aspect-ratio: 16/`,
		`aspect-ratio: /9`,
	} {
		if got := ratioHeightOfFilledBox(t, decl); got != line {
			t.Errorf("%s made the box %gpx tall and a line is %gpx; it is not a "+
				"ratio this engine can use, so the box keeps the height its "+
				"content gives it", decl, got, line)
		}
	}
}

// TestTheOtherDirectionIsReported is the narrowing, stated as a test so that it
// is a decision rather than a gap somebody finds.
//
// A box whose height is declared and whose width is auto is the other
// direction. A block's auto width fills its containing block by §10.3.3 rather
// than being shrunk to a ratio, and changing that reaches into the width
// arithmetic rather than sitting after it — so the page is the one the document
// would have had, and the author is told which half did nothing.
func TestTheOtherDirectionIsReported(t *testing.T) {
	_, findings := layoutWith(t, StandardFonts(),
		`<div id="outer"><div id="d"></div></div>`,
		`body{margin:0} #outer{width:320px} #d{aspect-ratio: 16/9; height: 50px}`)
	found := false
	for _, f := range findings {
		if f.Property == "aspect-ratio" {
			found = true
		}
	}
	if !found {
		t.Error("a ratio that decided neither axis said nothing; the author has no " +
			"other way to learn that half of the declaration did nothing")
	}
	// And a ratio that *did* decide the height says nothing, or the report is
	// noise on every document that uses the property as intended.
	_, quiet := layoutWith(t, StandardFonts(),
		`<div id="outer"><div id="d"></div></div>`,
		`body{margin:0} #outer{width:320px} #d{aspect-ratio: 16/9}`)
	for _, f := range quiet {
		if f.Property == "aspect-ratio" {
			t.Errorf("a ratio that decided the height reported %q", f.Message)
		}
	}
}
