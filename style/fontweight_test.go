package style

import "testing"

// TestRelativeWeightsComputeAgainstTheParent is CSS Fonts 4 §2.2.1's table,
// through the cascade: each row's two keywords over an inherited weight, the
// number a child then inherits, and a step taken twice down two levels.
func TestRelativeWeightsComputeAgainstTheParent(t *testing.T) {
	for _, tc := range []struct {
		parent, value, want string
	}{
		{"50", "bolder", "400"}, {"50", "lighter", "50"},
		{"100", "bolder", "400"}, {"100", "lighter", "100"},
		{"349", "bolder", "400"}, {"349", "lighter", "100"},
		{"normal", "bolder", "700"}, {"normal", "lighter", "100"},
		{"549.5", "bolder", "700"}, {"549.5", "lighter", "100"},
		{"550", "bolder", "900"}, {"550", "lighter", "400"},
		{"bold", "bolder", "900"}, {"bold", "lighter", "400"},
		{"750", "bolder", "900"}, {"750", "lighter", "700"},
		{"900", "bolder", "900"}, {"900", "lighter", "700"},
		{"950", "bolder", "950"}, {"950", "lighter", "700"},
		{"300", "700", "700"}, {"300", "bold", "bold"},
	} {
		doc := parseDoc(t, `<div id="d"><p id="p"><span id="s">x</span></p></div>`)
		styled := Apply(doc, []Sheet{author(t,
			`#d { font-weight: `+tc.parent+` } #p { font-weight: `+tc.value+` }`)})
		got := styled.Styles[elementFor(t, doc, "#p")].Get("font-weight")
		if got != tc.want {
			t.Errorf("font-weight: %s inside %s computed to %q, want %q", tc.value, tc.parent, got, tc.want)
		}
		if inherited := styled.Styles[elementFor(t, doc, "#s")].Get("font-weight"); inherited != got {
			t.Errorf("a child of font-weight: %s inside %s inherited %q, want %q",
				tc.value, tc.parent, inherited, got)
		}
	}

	// Twice: lighter inside lighter inside a black heading is 700, then 400.
	doc := parseDoc(t, `<h1 id="h"><span id="a"><span id="b">x</span></span></h1>`)
	styled := Apply(doc, []Sheet{author(t, `#h { font-weight: 900 } span { font-weight: lighter }`)})
	if a, b := styled.Styles[elementFor(t, doc, "#a")].Get("font-weight"),
		styled.Styles[elementFor(t, doc, "#b")].Get("font-weight"); a != "700" || b != "400" {
		t.Errorf("lighter twice inside 900 computed to %q then %q, want 700 then 400", a, b)
	}

	// The root has no parent, and steps from the initial weight.
	doc = parseDoc(t, `<p>x</p>`)
	styled = Apply(doc, []Sheet{author(t, `html { font-weight: bolder }`)})
	if got := styled.Styles[elementFor(t, doc, "html")].Get("font-weight"); got != "700" {
		t.Errorf("bolder on the root computed to %q, want 700", got)
	}

	// A pseudo-element steps from the element it belongs to.
	doc = parseDoc(t, `<p id="p">x</p>`)
	styled = Apply(doc, []Sheet{author(t, `#p { font-weight: 600 } #p::first-line { font-weight: bolder }`)})
	for key, cs := range styled.Pseudo {
		if key.Name == "first-line" {
			if got := cs.Get("font-weight"); got != "900" {
				t.Errorf("::first-line's bolder over 600 computed to %q, want 900", got)
			}
			return
		}
	}
	t.Error("no ::first-line style was computed")
}

// TestTheFontShorthandResetsTheVariations: CSS Fonts 4 §2.8's font resets
// font-optical-sizing and font-variation-settings to their initial values, as
// it does every other font longhand it does not set.
func TestTheFontShorthandResetsTheVariations(t *testing.T) {
	for property, want := range map[string]string{
		"font-optical-sizing":     "auto",
		"font-variation-settings": "normal",
	} {
		got, _ := winner(t, `#p { font-optical-sizing: none; font-variation-settings: "wght" 700 }
			#p { font: 12px serif }`, property)
		if got != want {
			t.Errorf("font: 12px serif left %s %q, want it reset to %q", property, got, want)
		}
	}
}
