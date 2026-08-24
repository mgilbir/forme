package layout

import (
	"testing"
)

// How tall the line a <br> ends is, CSS 2.1 §10.8.1.
//
// A <br> is an inline element. The line it ends is a line box containing it, and
// its own leading — the line-height it inherited from whatever it is inside —
// counts towards that line's height like any other box's. Ours took the block's
// strut instead, so a <br> alone in a "line-height: 200px" span made a
// twenty-pixel line and everything after it sat a hundred and eighty pixels too
// high.
//
// The suite's abspos/static-inside-float-inside-inline is the shape that found
// it: the <br> is what puts the float two hundred pixels down, and the static
// position of an absolutely positioned box inside the float follows it.

// brHeight lays out one line ended by a <br> and returns the block's height.
func brHeight(t *testing.T, markup, css string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-size: 20px; line-height: 1 }`+css)
	return find(t, root, "d").BorderRect.H.Px()
}

// TestALineEndedByABreakIsAsTallAsTheBreak.
func TestALineEndedByABreakIsAsTallAsTheBreak(t *testing.T) {
	// The <br> inherits the span's line-height, so the line it ends is the
	// span's height and not the block's.
	if got := brHeight(t, `<span style="line-height: 200px"><br></span>`, ""); got != 200 {
		t.Errorf("a <br> alone in a 200px span made a %gpx line, want 200", got)
	}
	// And the same written on the <br> itself.
	if got := brHeight(t, `<br style="line-height: 200px">`, ""); got != 200 {
		t.Errorf("a <br> at 200px made a %gpx line, want 200", got)
	}
	// Two of them: two lines, each the height of the break that ended it.
	if got := brHeight(t, `<span style="line-height: 200px"><br><br></span>`, ""); got != 400 {
		t.Errorf("two <br>s in a 200px span made %gpx, want 400", got)
	}
}

// TestABreakDoesNotShortenTheLineItEndsIs the other direction: the strut is
// still on the line, so a break shorter than the block's own text does not pull
// the line down to its own height.
func TestABreakDoesNotShortenTheLineItEnds(t *testing.T) {
	if got := brHeight(t, `<span style="line-height: 5px"><br></span>`,
		` #d { line-height: 100px }`); got != 100 {
		t.Errorf("a 5px <br> in a 100px block made a %gpx line, want 100 — the "+
			"block's strut is on the line whatever else is", got)
	}
	// A plain <br> takes the block's line-height, which is the case every
	// document is: nothing about this changes it.
	if got := brHeight(t, `<br>`, ""); got != 20 {
		t.Errorf("a plain <br> made a %gpx line, want the block's 20", got)
	}
	if got := brHeight(t, `x<br>y`, ""); got != 40 {
		t.Errorf("a line, a <br> and a line made %gpx, want 40", got)
	}
}

// TestTheBreakRaisesTheStrutAndNotTheLine.
//
// A forced break has never been in the item stream a line is walked from, and
// putting it there to give it a height costs 648 of the suite's clean passes —
// the stream is walked by the painting, the decorations and the positioning of
// everything on the line. So the *strut* is raised instead, which is the one
// thing on a line that already stands for a box with nothing to draw.
//
// What that has to leave alone is everything else about the line, and the
// visible half of it is where the text sits: raising the strut moves the
// baseline, so a break taller than the text must push the text down and a break
// shorter than it must not move it at all.
func TestTheBreakRaisesTheStrutAndNotTheLine(t *testing.T) {
	baseline := func(markup, css string) float64 {
		t.Helper()
		root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
			noDefaults+`#d { font-size: 20px; line-height: 1 }`+css)
		f := find(t, root, "d")
		if len(f.Lines) == 0 {
			t.Fatalf("%q produced no lines", markup)
		}
		return f.Lines[0].Baseline.Px()
	}
	plain := baseline(`x<br>`, "")
	tall := baseline(`x<span style="line-height: 200px"><br></span>`, "")
	if tall <= plain {
		t.Errorf("the baseline is at %gpx with a 200px break on the line and %gpx "+
			"with a plain one; a taller box on the line pushes the baseline down",
			tall, plain)
	}
	short := baseline(`x<span style="line-height: 5px"><br></span>`, "")
	if short != plain {
		t.Errorf("the baseline is at %gpx with a 5px break on the line and %gpx "+
			"with a plain one; a shorter box does not pull it up", short, plain)
	}
}
