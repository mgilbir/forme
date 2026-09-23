package style

import (
	"strings"
	"testing"
)

// The value gate, end to end: what the cascade computes when a declaration's
// value is invalid, and when it is valid CSS this engine does not evaluate.
// The grammar itself is tested in grammar_test.go; these ask the cascade.

// winner applies two rules to one element and returns the property's computed
// value and the findings.
func winner(t *testing.T, css, property string) (string, []Finding) {
	t.Helper()
	doc := parseDoc(t, `<div><p id="p">x</p></div>`)
	got := Apply(doc, []Sheet{author(t, css)})
	return got.Styles[elementFor(t, doc, "#p")].Get(property), got.Findings
}

// TestAnInvalidValueDoesNotWinTheCascade is audit C57: CSS 2.1 §4.2 ignores a
// declaration whose value the property does not take, so the one before it
// stands. Each of these was kept, won, and was read by layout as nothing.
func TestAnInvalidValueDoesNotWinTheCascade(t *testing.T) {
	for _, tc := range []struct{ property, good, bad, want string }{
		{"width", "100px", "foo", "100px"},
		{"font-size", "20px", "foo", "20px"},
		{"margin-top", "4px", "10", "4px"},
		{"float", "left", "sideways", "left"},
		{"position", "relative", "bogus", "relative"},
		{"line-height", "2", "foo", "2"},
		{"opacity", "0.5", "red", "0.5"},
		{"text-align-all", "center", "middle", "center"},
		{"z-index", "3", "1.5", "3"},
		{"grid-row-start", "2", "0", "2"},
	} {
		css := `#p { ` + tc.property + `: ` + tc.good + ` } #p { ` +
			tc.property + `: ` + tc.bad + ` }`
		got, findings := winner(t, css, tc.property)
		if got != tc.want {
			t.Errorf("%s: %q after %q computed to %q, want the valid %q to stand",
				tc.property, tc.bad, tc.good, got, tc.want)
		}
		found, unsupported := says(findings, "is not a valid value of "+tc.property)
		if !found || unsupported {
			t.Errorf("%s: %q was not reported as the author's invalid value: %v",
				tc.property, tc.bad, findings)
		}
	}
}

// TestAValueThisEngineDoesNotEvaluateIsReportedAsSuch is audit C28 and C59: a
// modern colour, a math function other than calc() and a system colour are
// correct CSS. The declaration is dropped so that the fallback written before
// it stands, and the finding claims the gap — it used to call each of them the
// author's mistake, which the reftest ratchet counts as a page with nothing
// missing.
func TestAValueThisEngineDoesNotEvaluateIsReportedAsSuch(t *testing.T) {
	for _, tc := range []struct{ property, good, value, want, what string }{
		{"color", "green", "oklch(0.6 0.2 140)", "green", "oklch()"},
		{"color", "green", "lab(50% 40 59)", "green", "lab()"},
		{"color", "green", "hwb(120 0% 0%)", "green", "hwb()"},
		{"color", "green", "color(srgb 1 0 0)", "green", "color()"},
		{"color", "green", "color-mix(in srgb, red, blue)", "green", "color-mix()"},
		{"color", "green", "Canvas", "green", "the system colour Canvas"},
		{"color", "green", "rgb(calc(255) 0 0)", "green", "rgb()"},
		{"color", "green", "light-dark(red, blue)", "green", "light-dark()"},
		{"width", "100px", "min(10px, 50%)", "100px", "min()"},
		{"border-top-color", "green", "oklch(0.5 0.1 20)", "green", "oklch()"},
		// Through the shorthands, whose expanders could not place the part.
		{"border-top-color", "green", "", "green", "oklch()"},
		{"background-color", "green", "", "green", "lab()"},
	} {
		css := `#p { ` + tc.property + `: ` + tc.good + ` } #p { ` + tc.property +
			`: ` + tc.value + ` }`
		switch {
		case tc.value == "" && tc.what == "oklch()":
			css = `#p { border-top-color: green } #p { border: 2px solid oklch(0.5 0.1 20) }`
		case tc.value == "" && tc.what == "lab()":
			css = `#p { background-color: green } #p { background: lab(50% 40 59) }`
		}
		got, findings := winner(t, css, tc.property)
		if got != tc.want {
			t.Errorf("%s: computed %q, want the fallback %q", css, got, tc.want)
		}
		found, unsupported := says(findings, "uses "+tc.what)
		if !found || !unsupported {
			t.Errorf("%s: found=%v unsupported=%v, want an unsupported finding naming %s: %v",
				css, found, unsupported, tc.what, findings)
		}
	}
}

// TestAShorthandPartThatIsValidLandsInItsSlot is the other half of C59: a part
// the expander did not recognise because it could not compute it was a whole
// shorthand called unreadable. A calc() this engine *does* compute is now
// placed and applied.
func TestAShorthandPartThatIsValidLandsInItsSlot(t *testing.T) {
	for _, tc := range []struct{ css, property, want string }{
		{`#p { border: calc(1px + 1px) solid red }`, "border-top-width", "calc(1px + 1px)"},
		{`#p { border: 1px solid currentcolor }`, "border-top-style", "solid"},
		{`#p { font: calc(10px + 2px) serif }`, "font-size", "12px"},
		{`#p { outline: auto }`, "outline-style", "auto"},
	} {
		got, findings := winner(t, tc.css, tc.property)
		if got != tc.want {
			t.Errorf("%s: %s is %q, want %q (%v)", tc.css, tc.property, got, tc.want, findings)
		}
	}
	// And one no slot takes is still the author's mistake.
	_, findings := winner(t, `#p { border: 1px solid florb }`, "border-top-color")
	if found, unsupported := says(findings, "is not a valid value of border"); !found || unsupported {
		t.Errorf("\"border: 1px solid florb\" was not reported as invalid: %v", findings)
	}
	// And a part judged invalid in its longhand drops the whole shorthand: a
	// box shorthand places parts by position and never looked at them.
	got, _ := winner(t, `#p { margin: 5px } #p { margin: 1px foo }`, "margin-top")
	if got != "5px" {
		t.Errorf("\"margin: 1px foo\" was applied, leaving margin-top %q", got)
	}
}

// TestTheFontShorthandResetsEverythingItOwns is audit C112's first half: CSS
// Fonts 4 §2.8 resets every font-variant longhand, font-kerning and
// font-feature-settings, and "font: inherit" inherits all of them.
func TestTheFontShorthandResetsEverythingItOwns(t *testing.T) {
	for property, before := range map[string]string{
		"font-variant-numeric":    "oldstyle-nums",
		"font-variant-ligatures":  "none",
		"font-variant-east-asian": "ruby",
		"font-variant-position":   "super",
		"font-variant-caps":       "small-caps",
		"font-kerning":            "none",
		"font-feature-settings":   `"smcp"`,
	} {
		got, _ := winner(t, `#p { `+property+`: `+before+` } #p { font: 12px serif }`, property)
		want := properties[property].initial
		if got != want {
			t.Errorf("%s: %q survived \"font: 12px serif\" as %q, want the initial %q",
				property, before, got, want)
		}
	}
	doc := parseDoc(t, `<div><p id="p">x</p></div>`)
	styled := Apply(doc, []Sheet{author(t,
		`div { font-kerning: none; font-variant-numeric: oldstyle-nums } #p { font: inherit }`)})
	cs := styled.Styles[elementFor(t, doc, "#p")]
	if cs.Get("font-kerning") != "none" || cs.Get("font-variant-numeric") != "oldstyle-nums" {
		t.Errorf("\"font: inherit\" did not inherit the longhands it owns: kerning %q, numeric %q",
			cs.Get("font-kerning"), cs.Get("font-variant-numeric"))
	}
}

// TestTheFontShorthandTakesCSSFonts4 is C112's second half: a width keyword, a
// numeric weight and an oblique angle are valid, and were each the reason a
// declaration every browser applies was dropped as unreadable.
func TestTheFontShorthandTakesCSSFonts4(t *testing.T) {
	for _, tc := range []struct{ value, property, want string }{
		{"450 12px serif", "font-weight", "450"},
		{"oblique 10deg 12px serif", "font-style", "oblique 10deg"},
		{"condensed 12px serif", "font-size", "12px"},
		{"italic small-caps bold condensed 12px/2 serif", "line-height", "2"},
		{"normal normal normal normal 12px serif", "font-size", "12px"},
	} {
		got, findings := winner(t, `#p { font: 20px monospace } #p { font: `+tc.value+` }`,
			tc.property)
		if got != tc.want {
			t.Errorf("font: %s gave %s %q, want %q (%v)", tc.value, tc.property, got, tc.want,
				findings)
		}
	}
	// The width is valid and this engine has no property for it: the rest is
	// applied and the width is claimed as missing.
	_, findings := winner(t, `#p { font: condensed 12px serif }`, "font-size")
	if found, unsupported := says(findings, "the font width condensed"); !found || !unsupported {
		t.Errorf("the font width was not reported as unsupported: %v", findings)
	}
	// Five slots filled before the size is one too many.
	got, _ := winner(t, `#p { font: 20px monospace } #p { font: normal normal normal normal normal 12px serif }`,
		"font-size")
	if got != "20px" {
		t.Errorf("a font with five prefix parts was applied, leaving %q", got)
	}
}

// TestWhiteSpaceTakesItsLonghandsValues: CSS Text 4 §3 makes white-space
// "<'white-space-collapse'> || <'text-wrap-mode'> || <'white-space-trim'>"
// beside its legacy keywords. The suite writes "preserve-breaks nowrap", and
// with a value grammar calling a refused value the author's mistake, refusing
// it would have been a false report as well as a dropped declaration.
func TestWhiteSpaceTakesItsLonghandsValues(t *testing.T) {
	for _, tc := range []struct{ value, collapse, mode string }{
		{"preserve-breaks nowrap", "preserve-breaks", "nowrap"},
		{"nowrap preserve", "preserve", "nowrap"},
		{"preserve-breaks", "preserve-breaks", "wrap"},
		{"wrap", "collapse", "wrap"},
		{"pre", "preserve", "nowrap"},
	} {
		doc := parseDoc(t, `<p id="p">x</p>`)
		cs := Apply(doc, []Sheet{author(t, `#p { white-space: `+tc.value+` }`)}).
			Styles[elementFor(t, doc, "#p")]
		if c, m := cs.Get("white-space-collapse"), cs.Get("text-wrap-mode"); c != tc.collapse || m != tc.mode {
			t.Errorf("white-space: %s gave %s / %s, want %s / %s", tc.value, c, m,
				tc.collapse, tc.mode)
		}
	}
	// A trim keyword is valid and not done: reported, and the rest applied.
	_, findings := winner(t, `#p { white-space: preserve discard-inner }`, "white-space-collapse")
	if found, unsupported := says(findings, "white-space-trim value discard-inner"); !found || !unsupported {
		t.Errorf("the trim keyword was not reported as unsupported: %v", findings)
	}
	// And two of one kind is not a value.
	if got, _ := winner(t, `#p { white-space: pre } #p { white-space: wrap nowrap }`,
		"text-wrap-mode"); got != "nowrap" {
		t.Errorf("\"white-space: wrap nowrap\" was applied, leaving %q", got)
	}
}

// TestWordWrapIsOverflowWrapsOtherName is audit C111. The two are one property,
// so whichever the cascade puts last wins — and an inherited word-wrap no
// longer outlives an overflow-wrap declared on the element.
func TestWordWrapIsOverflowWrapsOtherName(t *testing.T) {
	for _, tc := range []struct{ css, want string }{
		{`div { word-wrap: break-word } #p { overflow-wrap: normal }`, "normal"},
		{`#p { overflow-wrap: normal; word-wrap: break-word }`, "break-word"},
		{`#p { word-wrap: break-word; overflow-wrap: anywhere }`, "anywhere"},
		{`div { word-wrap: anywhere }`, "anywhere"},
	} {
		if got, _ := winner(t, tc.css, "overflow-wrap"); got != tc.want {
			t.Errorf("%s: overflow-wrap is %q, want %q", tc.css, got, tc.want)
		}
	}
	if _, registered := properties["word-wrap"]; registered {
		t.Error("word-wrap is registered as a property of its own")
	}
}

// TestALogicalColourIsGatedLikeItsPhysicalOne is audit C109.
func TestALogicalColourIsGatedLikeItsPhysicalOne(t *testing.T) {
	got, findings := winner(t,
		`#p { border-left-color: green } #p { border-inline-start-color: 'x' }`,
		"border-left-color")
	if got != "green" {
		t.Errorf("an invalid logical colour won, leaving %q", got)
	}
	if found, _ := says(findings, "is not a colour"); !found {
		t.Errorf("it was not reported: %v", findings)
	}
	colour, _ := styledBy(t, `@supports (border-inline-start-color: 'x') { #target { color: red } }`)
	if colour == "red" {
		t.Error("@supports answered yes about a logical colour the cascade drops")
	}
}

// TestSupportsIsAnsweredAboutTheValue: "(position: bogus)" is false now that
// the value is asked, where it was true for any value of a known property.
func TestSupportsIsAnsweredAboutTheValue(t *testing.T) {
	for _, tc := range []struct {
		condition string
		want      bool
	}{
		{`(position: bogus)`, false},
		{`(position: sticky)`, true},
		{`(width: min(1px, 2px))`, false},
		{`(color: oklch(0.5 0.1 20))`, false},
		{`(word-wrap: break-word)`, true},
		{`(font: condensed 12px serif)`, false},
		{`(width: var(--w))`, false},
	} {
		colour, _ := styledBy(t, `@supports `+tc.condition+` { #target { color: red } }`)
		if got := colour == "red"; got != tc.want {
			t.Errorf("@supports %s answered %v, want %v", tc.condition, got, tc.want)
		}
	}
}

// TestAMalformedSupportsConditionInvalidatesTheRule is audit C158's first
// half: Conditional 3 §2.1 makes a condition that mixes "and" and "or" at one
// level, or puts "not" before anything but a parenthesised term, not a
// condition at all.
func TestAMalformedSupportsConditionInvalidatesTheRule(t *testing.T) {
	for _, condition := range []string{
		`(color: red) and (display: block) or (color: blue)`,
		`(color: red) or (display: block) and (color: blue)`,
		`not not (color: red)`,
		`not (color: red) and (display: block)`,
		`color: red`,
		`red`,
		`(color: red) (display: block)`,
	} {
		colour, findings := styledBy(t, `@supports `+condition+` { #target { color: red } }`)
		if colour == "red" {
			t.Errorf("@supports %s applied its block", condition)
		}
		found := false
		for _, f := range findings {
			if f.Property == "@supports" && strings.Contains(f.Message, "not a valid condition") {
				found = true
				if f.Unsupported {
					t.Errorf("@supports %s was claimed as a gap; it is malformed", condition)
				}
			}
		}
		if !found {
			t.Errorf("@supports %s was dropped without saying it is malformed: %v",
				condition, findings)
		}
	}
	// Grouped, the same mixture is a condition.
	if colour, _ := styledBy(t, `@supports ((color: red) and (display: block)) or (color: nonesuch) { #target { color: red } }`); colour != "red" {
		t.Error("a parenthesised mixture was not read as a condition")
	}
}

// TestALayerPreludeIsANameList is audit C158's second half: "@layer a b" is not
// a layer called "ab", it is not a list of layer names, and the rule is
// dropped.
func TestALayerPreludeIsANameList(t *testing.T) {
	for _, prelude := range []string{"a b", "a..b", ".a", "a,,b", "a.", "1"} {
		got, findings := winner(t, `#p { width: 5px } @layer `+prelude+` { #p { width: 9px } }`,
			"width")
		if got != "5px" {
			t.Errorf("@layer %s applied its block", prelude)
		}
		if found, _ := says(findings, "is not a list of layer names"); !found {
			t.Errorf("@layer %s was not reported: %v", prelude, findings)
		}
	}
	if got, _ := winner(t, `@layer a.b { #p { width: 9px } }`, "width"); got != "9px" {
		t.Errorf("a dotted layer name was refused: width %q", got)
	}
}
