package layout

import "testing"

// A line that cannot break, beside a float that leaves it too little room.
//
// §9.5 shifts a line box downwards when it is "too small to contain any
// content", which is what keeps a paragraph from being clipped to nothing
// between two facing floats. It is a rule about *fitting*, and fitting is a
// question a line that cannot break cannot answer differently anywhere: text
// under "white-space: pre" or "nowrap" is one unbreakable run, so a band lower
// down holds no more of it than the band it is in. Moving it only moves the
// overflow down the page.
//
// The suite's pre-float-001 draws that five times over: five "white-space: pre"
// blocks of ten characters' width beside a three-character float, with the
// content growing a character at a time. Its reference puts every one of them
// beside its float, including the one whose first run is wider than the room.

const besideFloatCSS = `body { margin: 0 } .box { width: 200px }
	.f { float: left; width: 60px; height: 40px }
	.l { float: left; width: 100px; height: 40px }
	.r { float: right; width: 100px; height: 40px }
	#t { font-family: Courier; font-size: 20px; line-height: 20px }`

// firstTextAt is where the first run of a document is drawn. The y is a
// baseline and not a line top, so the tests below state it as a distance from
// the same text laid out with nothing in its way.
func firstTextAt(t *testing.T, markup string) (x, y float64) {
	t.Helper()
	root := layoutOf(t, 800, `<div class="box">`+markup+`</div>`, besideFloatCSS)
	for _, op := range Paint(root) {
		if o, ok := op.(DrawText); ok {
			return o.At.X.Px(), o.At.Y.Px()
		}
	}
	t.Fatal("nothing was drawn")
	return 0, 0
}

// besideFloat is that document with a 60px float before it, and the baseline it
// would have had without one.
func besideFloat(t *testing.T, css, text string) (x, y, clear float64) {
	t.Helper()
	x, y = firstTextAt(t,
		`<div class="f"></div><div id="t" style="`+css+`">`+text+`</div>`)
	_, clear = firstTextAt(t, `<div id="t" style="`+css+`">`+text+`</div>`)
	return x, y, clear
}

// TestAnUnbreakableLineStaysBesideTheFloat. Fifteen characters of 12px is 180,
// and the band beside the float is 140.
func TestAnUnbreakableLineStaysBesideTheFloat(t *testing.T) {
	for _, ws := range []string{"pre", "nowrap"} {
		x, y, plain := besideFloat(t, "white-space:"+ws, "123456789012345")
		if y != plain {
			t.Errorf("under white-space: %s the line dropped %gpx, want 0 — a "+
				"line that cannot break gains nothing by moving", ws, y-plain)
		}
		if x != 60 {
			t.Errorf("under white-space: %s the text begins at x=%g, want 60 — "+
				"it cannot break, so no band below holds any more of it than "+
				"this one and moving it only moves the overflow", ws, x)
		}
	}
}

// TestAWordThatCouldHaveFitDropsBelowTheFloat is the rule that must not be lost
// with it: a line that *can* break is shifted, because the band below really
// does hold what this one cannot.
func TestAWordThatCouldHaveFitDropsBelowTheFloat(t *testing.T) {
	x, y, plain := besideFloat(t, "", "123456789012345")
	if x != 0 || y-plain != 40 {
		t.Errorf("the word begins at x=%g and %gpx below where it would have "+
			"been, want 0 and 40 — one word of 180px does not fit in 140 and "+
			"the float is 40 tall", x, y-plain)
	}
}

// TestALineWithNoRoomAtAllIsStillShifted. §9.5's own case, and the one that
// keeps a paragraph between two facing floats from being clipped away: there is
// no room, so there is nothing for the line to overflow *from*.
func TestALineWithNoRoomAtAllIsStillShifted(t *testing.T) {
	x, y := firstTextAt(t, `<div class="l"></div><div class="r"></div>`+
		`<div id="t" style="white-space:pre">abc</div>`)
	_, plain := firstTextAt(t, `<div id="t" style="white-space:pre">abc</div>`)
	if x != 0 || y-plain != 40 {
		t.Errorf("the text is at x=%g and %gpx below where it would have been, "+
			"want 0 and 40 — the two floats leave the line nothing at all",
			x, y-plain)
	}
}
