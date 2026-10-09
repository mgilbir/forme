package layout

import "testing"

// TestAHugeLengthIsTheLargestLengthHoweverItIsWritten is one length written
// three ways, and the page each makes.
//
// "height: 1e9px" composed at scale one with no finding at all: the length was
// past what a Unit holds, the reader refused it so that a layer above could
// report it, and the layer above read a refused length as no declaration. The
// same height written "calc(1e9 * 1px)" was read — the arithmetic saturates —
// and the page was shrunk to fit the box and said so. "calc(1e9px)" was
// refused like the literal. CSS Values 3 §4 says which is right: a value an
// implementation cannot hold is converted "to the closest value supported".
// So all three are the tallest box there is, and the page reports what that
// box did to it.
func TestAHugeLengthIsTheLargestLengthHoweverItIsWritten(t *testing.T) {
	for _, property := range []string{"height", "width", "margin-top", "padding-left"} {
		var want float64
		for i, value := range []string{
			"calc(1e9 * 1px)", "1e9px", "calc(1e9px)", "1e400px", "99999999in",
		} {
			decl := property + ": " + value
			out := Compose(Input{HTML: `<div style="` + decl + `">x</div>`},
				Options{Page: A4})
			if i == 0 {
				want = out.Scale
				if want >= 0.5 {
					t.Fatalf("%q composed at scale %v; the fixture asks for a box "+
						"too large for the page and has to be shrunk", decl, want)
				}
			}
			if out.Scale != want {
				t.Errorf("%q composed at scale %v, and %s: calc(1e9 * 1px) at %v; "+
					"one length written two ways is one box", decl, out.Scale,
					property, want)
			}
			if !hasRule(out.Findings, RuleMinScale) {
				t.Errorf("%q: no %s finding, so nothing says the page was shrunk; "+
					"the findings were %v", decl, RuleMinScale, out.Findings)
			}
		}
	}
	// The control: an ordinary height, which nothing shrinks.
	if out := Compose(Input{HTML: `<div style="height: 100px">x</div>`},
		Options{Page: A4}); out.Scale != 1 || hasRule(out.Findings, RuleMinScale) {
		t.Errorf("a box a hundred pixels tall composed at scale %v with %v",
			out.Scale, out.Findings)
	}
}
