package layout

import "testing"

// "clear" on a <br>, which CSS does not have and HTML does.
//
// §9.5.2 applies the property to block-level elements, and a <br> is not one —
// so on the face of it "clear: both" on one does nothing. It does something in
// every browser, and it does it because HTML says so rather than because CSS
// does: the element's clear *attribute* maps to the property, and
// "<br clear=all>" is older than the property is. The suite turns on it in
// border-conflict-style-107, which lays sixteen floated tables out in four rows
// with "br { clear: both }" between them.

const brClearCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px }
	.l { float: left; width: 50px; height: 50px }
	.r { float: right; width: 50px; height: 50px }`

// afterTheBreak is where the text after a <br> is drawn.
func afterTheBreak(t *testing.T, floats, brCSS string) (x, y float64) {
	t.Helper()
	root := layoutOf(t, 200,
		`<div id="d">`+floats+`x<br style="`+brCSS+`">y</div>`, brClearCSS)
	for _, op := range Paint(root) {
		if o, ok := op.(DrawText); ok && o.Text == "y" {
			return o.At.X.Px(), o.At.Y.Px()
		}
	}
	t.Fatal("the text after the break was not drawn")
	return 0, 0
}

// TestABreakClearsTheFloatItIsToldTo.
func TestABreakClearsTheFloatItIsToldTo(t *testing.T) {
	plainX, plainY := afterTheBreak(t, `<div class="l"></div>`, "")
	if plainX != 50 {
		t.Fatalf("without clearance the second line begins at x=%g, want 50 — "+
			"the fixture cannot say what it means to say", plainX)
	}
	for _, clear := range []string{"left", "both"} {
		x, y := afterTheBreak(t, `<div class="l"></div>`, "clear:"+clear)
		if x != 0 || y-plainY != 30 {
			t.Errorf("with clear: %s the second line begins at x=%g and %gpx "+
				"lower, want 0 and 30 — the float is 50 tall and the line it "+
				"would have been on began at 20", clear, x, y-plainY)
		}
	}
}

// TestABreakClearsOnlyTheSideItNames.
func TestABreakClearsOnlyTheSideItNames(t *testing.T) {
	_, plain := afterTheBreak(t, `<div class="l"></div>`, "")
	x, y := afterTheBreak(t, `<div class="l"></div>`, "clear:right")
	if x != 50 || y != plain {
		t.Errorf("with clear: right beside a left float the line begins at "+
			"(%g,%+g), want (50,+0) — there is no right float to clear",
			x, y-plain)
	}
}

// TestABreakWithNoClearIsAPlainBreak, which is the row a planted defect that
// clears unconditionally would otherwise pass.
func TestABreakWithNoClearIsAPlainBreak(t *testing.T) {
	x, _ := afterTheBreak(t, `<div class="l"></div>`, "")
	if x != 50 {
		t.Errorf("a <br> with no clear put the next line at x=%g, want 50 — it "+
			"is beside the float like any other line", x)
	}
}

// HTML's <br clear> attribute, which is older than the property it sets.
//
// "<br clear=all>" is how a page cleared a float before CSS existed, and HTML's
// rendering section still maps it: "left", "right", "all" and "both" to the
// property's values, "none" to none. It is a presentational hint rather than a
// thing layout reads, which is a place in the cascade — an author rule beats it
// and a user-agent rule does not.

// clearedBy is where the border under a <br> lands, given the markup for it.
//
// Two floats of different heights, because one would not tell the four values
// apart: with a single left float, "left", "all" and "both" all clear to the
// same place and a mapping that sent "all" to "left" would look right. The
// right float is the taller, so clearing it is a different number from clearing
// the left one.
func clearedBy(t *testing.T, br string) float64 {
	t.Helper()
	root := layoutOf(t, 300,
		`<div id="d"><div class="l"></div><div class="r"></div>`+
			`<div class="c">`+br+`</div></div>`,
		`body{margin:0}
	.l { float: left; width: 15px; height: 60px }
	.r { float: right; width: 15px; height: 100px }
	.c { border-bottom: 5px solid black }`)
	for _, op := range Paint(root) {
		if v, ok := op.(FillRect); ok && !v.Rect.Empty() && v.Rect.W.Px() == 300 {
			return v.Rect.Y.Px()
		}
	}
	t.Fatal("the fixture drew no border")
	return 0
}

// TestTheBreakClearAttributeIsTheProperty.
func TestTheBreakClearAttributeIsTheProperty(t *testing.T) {
	// Without it the line sits at the top, beside the float; with it the line
	// is pushed past the float's 60px.
	plain := clearedBy(t, `<br>`)
	if plain >= 60 {
		t.Fatalf("with no clear at all the border is at %g, want above the shorter "+
			"float's 60px — the fixture cannot say what it means to say", plain)
	}
	for _, c := range []struct {
		attr string
		at   float64
	}{
		// The left float is 60 tall and the right one 100, so each value lands
		// at a number the others do not.
		{`clear="left"`, 60},
		{`clear="right"`, 100},
		{`clear="all"`, 100},
		{`clear="both"`, 100},
		{`clear="ALL"`, 100},
		// Not values, so no hint at all and the line stays where it was.
		{`clear="none"`, plain},
		{`clear="nonsense"`, plain},
		{`clear=""`, plain},
	} {
		if got := clearedBy(t, `<br `+c.attr+`>`); got != c.at {
			t.Errorf("<br %s> put the border at %g, want %g", c.attr, got, c.at)
		}
	}
}

// TestTheAttributeIsAHintAndNotAnInlineStyle.
//
// Where it sits in the cascade is the half that is easy to get wrong. A hint is
// in the author origin at zero specificity, so any author rule at all beats it —
// including "* { clear: none }" — and no user-agent rule ever does. A layout
// that read the attribute directly would have neither.
func TestTheAttributeIsAHintAndNotAnInlineStyle(t *testing.T) {
	at := func(css string) float64 {
		t.Helper()
		root := layoutOf(t, 300,
			`<div id="d"><div class="l"></div><div class="r"></div>`+
				`<div class="c"><br clear="all"></div></div>`,
			`body{margin:0}
	.l { float: left; width: 15px; height: 60px }
	.r { float: right; width: 15px; height: 100px }
	.c { border-bottom: 5px solid black }
	`+css)
		for _, op := range Paint(root) {
			if v, ok := op.(FillRect); ok && !v.Rect.Empty() && v.Rect.W.Px() == 300 {
				return v.Rect.Y.Px()
			}
		}
		t.Fatal("the fixture drew no border")
		return 0
	}
	if at("") < 100 {
		t.Fatal("the attribute did not clear at all, so the checks below say nothing")
	}
	// An author rule of the lowest specificity there is still beats it.
	if got := at(`* { clear: none }`); got >= 100 {
		t.Errorf("with \"* { clear: none }\" the border is at %g; a hint is in the "+
			"author origin at zero specificity and any author rule beats it", got)
	}
	// And an inline style beats it, which is the same statement from the other
	// end: a hint that were an inline style would tie with one and win on order.
	root := layoutOf(t, 300,
		`<div id="d"><div class="l"></div><div class="r"></div><div class="c">`+
			`<br clear="all" style="clear: none"></div></div>`,
		`body{margin:0}
	.l { float: left; width: 15px; height: 60px }
	.r { float: right; width: 15px; height: 100px }
	.c { border-bottom: 5px solid black }`)
	for _, op := range Paint(root) {
		if v, ok := op.(FillRect); ok && !v.Rect.Empty() && v.Rect.W.Px() == 300 {
			if v.Rect.Y.Px() >= 100 {
				t.Errorf("an inline \"clear: none\" lost to the attribute; the " +
					"attribute is a hint and sits below every declaration")
			}
		}
	}
}
