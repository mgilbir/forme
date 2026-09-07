package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

func values(t *testing.T, src string) []css.ComponentValue {
	t.Helper()
	vals, _ := css.ParseComponentValues(src)
	return vals
}

// TestAColourFunctionWithAMissingArgumentIsNotAColour.
//
// "rgb(1,2,3,)" is a function whose last argument is missing, not a colour with
// no alpha. Reading it as one made a declaration no browser accepts into an
// opaque colour, which is a page that is plausible and wrong rather than one
// that fell back to what it inherited.
func TestAColourFunctionWithAMissingArgumentIsNotAColour(t *testing.T) {
	for _, tc := range []struct {
		src string
		ok  bool
	}{
		{"rgb(1,2,3)", true},
		{"rgb(1,2,3,0.5)", true},
		{"rgb(1 2 3)", true},
		{"rgb(1 2 3 / 0.5)", true},

		{"rgb(1,2,3,)", false},
		{"rgb(1,2,,3)", false},
		{"rgb(,1,2,3)", false},
		{"rgb(1,2)", false},
		{"rgb(1,2,3,4,5)", false},
	} {
		if _, ok := ParseColor(values(t, tc.src)); ok != tc.ok {
			t.Errorf("%q parses as a colour = %v, want %v", tc.src, ok, tc.ok)
		}
	}
}

// TestCalcNeedsSpaceOnBothSidesOfItsOperator.
//
// CSS Values §10.1 requires white space around + and -, and the requirement is
// not decoration: the tokenizer has already made "-2px" a negative length, so
// "calc(1px +-2px)" is a length followed by another length and not a
// subtraction. Checking the space in front alone read it as minus one pixel.
func TestCalcNeedsSpaceOnBothSidesOfItsOperator(t *testing.T) {
	ctx := LengthContext{FontSize: 16, RootFontSize: 16}
	for _, tc := range []struct {
		src string
		ok  bool
	}{
		{"calc(1px + 2px)", true},
		{"calc(1px - 2px)", true},
		{"calc(1px + 2px - 3px)", true},
		{"calc(2px*3)", true}, // "*" and "/" need no space, and never did

		{"calc(1px +-2px)", false},
		{"calc(1px+ 2px)", false},
		{"calc(1px+2px)", false},
		{"calc(1px -2px)", false},
		{"calc(1px -+2px)", false},
	} {
		if _, _, ok := ParseLength(values(t, tc.src), ctx); ok != tc.ok {
			t.Errorf("%q parses as a length = %v, want %v", tc.src, ok, tc.ok)
		}
	}
}

// TestACharsetSayingUTF8IsPassedOverInSilence.
//
// @charset names the encoding the stylesheet is written in, which is not
// something the cascade applies to anything: it is a fact about the bytes,
// settled before they were parsed. It was reported as an at-rule "not applied
// yet" — the report every unrecognised at-rule gets — so a stylesheet opening
// with the perfectly ordinary `@charset "utf-8";` put its document in the
// bucket of pages carrying something unsupported.
func TestACharsetSayingUTF8IsPassedOverInSilence(t *testing.T) {
	findingsFor := func(src string) []Finding {
		t.Helper()
		rules, _ := css.ParseStylesheet(src)
		s := &Styler{seen: map[string]bool{}, attrOffset: -1}
		var out []preparedRule
		order := 0
		for _, r := range rules {
			s.prepareRule(r, nil, OriginAuthor, &out, &order)
		}
		return s.findings
	}
	for _, src := range []string{
		`@charset "utf-8"; p { color: red }`,
		`@charset "UTF-8";`,
		`@charset "utf8";`,
	} {
		if got := findingsFor(src); len(got) != 0 {
			t.Errorf("%q was reported: %v", src, got)
		}
	}
	// And one naming an encoding this engine cannot decode is the same report
	// the document's own <meta charset> gets.
	for _, src := range []string{
		`@charset "shift_jis";`,
		`@charset "windows-1252";`,
	} {
		got := findingsFor(src)
		if len(got) != 1 {
			t.Errorf("%q gave %d findings, want one: %v", src, len(got), got)
			continue
		}
		if !got[0].Unsupported {
			t.Errorf("%q: the finding is not marked Unsupported", src)
		}
	}
	// An at-rule nothing acts on is still reported, which is what makes the
	// silence above a decision rather than a hole.
	if got := findingsFor(`@supports (display: grid) { p { color: red } }`); len(got) != 1 {
		t.Errorf("@supports gave %d findings, want one: %v", len(got), got)
	}
}

// TestAFontSizeMayBeACalc.
//
// A percentage in a font-size is of the parent's font size, and that is the one
// number a calc() usually cannot be resolved against — but here the context
// already holds it. "calc(100% + 2px)" is two pixels more than the text around
// it, which is how a stylesheet says exactly that, and every browser resolves
// it. It was refused for not being an absolute length, and refused silently:
// the declaration was dropped, the element kept what it inherited, and nothing
// said the rule had not applied.
func TestAFontSizeMayBeACalc(t *testing.T) {
	parent, _ := FromPx(16)
	ctx := LengthContext{FontSize: parent, RootFontSize: parent}
	for _, tc := range []struct {
		src  string
		want float64
		ok   bool
	}{
		{"calc(100% + 2px)", 18, true},
		{"calc(100% - 4px)", 12, true},
		{"calc(50% + 50%)", 16, true},
		{"calc(1em + 2px)", 18, true},
		{"calc(2 * 100%)", 32, true},
		{"120%", 19.2, true},
		{"12px", 12, true},

		// And what is still refused.
		{"calc(0% - 4px)", 0, false},
		{"-2px", 0, false},
		{"calc(100vw + 1px)", 0, false},
	} {
		vals, _ := css.ParseComponentValues(tc.src)
		got, _, ok := ResolveFontSizeIn(vals, ctx)
		if ok != tc.ok {
			t.Errorf("%q resolves = %v, want %v", tc.src, ok, tc.ok)
			continue
		}
		// Within a layout unit, which is what a length rounds to.
		if d := got.Px() - tc.want; ok && (d > 0.01 || d < -0.01) {
			t.Errorf("%q is %gpx against a 16px parent, want %g", tc.src, got.Px(), tc.want)
		}
	}
}
