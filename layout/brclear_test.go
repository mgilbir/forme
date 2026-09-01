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
