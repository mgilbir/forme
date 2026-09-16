package layout

import (
	"strings"
	"testing"
)

// TestFontFeatureSettingsAsksTheFaceForTheTag is the property a real document
// uses and the suite does not: a named OpenType feature, by tag.
//
// The suite writes font-feature-settings five times and every one of them names
// "kern" over the default serif face, which carries no kerning — so the whole
// property was reported as unapplied and measured worth zero reftests. Real
// documents are not like that: "tnum" is what an invoice's columns are set with,
// "ss01" is how a face's alternate letterforms are asked for, and neither has a
// font-variant keyword to ask through.
//
// The test is self-checking rather than a pinned number. "onum" through
// font-feature-settings and oldstyle-nums through font-variant-numeric are the
// same request to the face — the property and the descriptor name one feature
// between them — so a box shrunk to fit its figures has to come out the same
// width either way, and a different width from the lining figures it replaces.
// Nothing here has to know what an oldstyle figure looks like.
func TestFontFeatureSettingsAsksTheFaceForTheTag(t *testing.T) {
	set := numericFontSet(t)
	// The declaration goes in the stylesheet rather than a style attribute: its
	// value carries quotation marks, and nesting those inside an attribute's own
	// quotes ends the attribute rather than the tag name. The first version of
	// this measured 600 — the whole line box — because the float had stopped
	// being a float.
	widthOf := func(decl string) float64 {
		t.Helper()
		frag, _ := layoutWith(t, set,
			`<div id="d">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px; float:left; `+decl+`}`)
		return find(t, frag, "d").BorderRect.W.Px()
	}

	plain := widthOf("")
	variant := widthOf("font-variant-numeric: oldstyle-nums")
	if plain == variant {
		t.Fatalf("the figures are %g wide with and without oldstyle-nums; the "+
			"fixture is not reaching the face's own feature", plain)
	}

	if got := widthOf(`font-feature-settings: "onum"`); got != variant {
		t.Errorf(`font-feature-settings: "onum" gave %g and `+
			`font-variant-numeric: oldstyle-nums gave %g; they are one request to `+
			`the face and have to reach it the same way (the lining figures are %g)`,
			got, variant, plain)
	}
	// The forms that mean the same thing. "1" and "on" are the value's own
	// spellings of what a bare tag already says.
	for _, css := range []string{
		`font-feature-settings: "onum" 1`,
		`font-feature-settings: "onum" on`,
		`font-feature-settings: 'onum'`,
	} {
		if got := widthOf(css); got != variant {
			t.Errorf("%s gave %g, want %g — the same feature by another spelling",
				css, got, variant)
		}
	}
}

// TestFontFeatureSettingsReportsWhatItCannotDo is the other half, and it is
// what keeps the half above from being a claim that everything works.
//
// Two things in this property are still not carried out, and they are different
// from each other. A tag turned *off* asks for a feature not to be applied, and
// the features this engine applies without being asked have switches of their
// own — font-variant-ligatures and font-kerning — so the tag is not the way to
// reach them. And a tag turned on that the face has not got is carried out and
// changes nothing, which is what reportCaps says about a face with no small
// capitals.
func TestFontFeatureSettingsReportsWhatItCannotDo(t *testing.T) {
	set := numericFontSet(t)
	findings := func(decl string) []Finding {
		t.Helper()
		_, got := layoutWith(t, set,
			`<div id="d">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px; `+decl+`}`)
		var out []Finding
		for _, f := range got {
			if f.Property == "font-feature-settings" {
				out = append(out, f)
			}
		}
		return out
	}

	// A tag the face has, turned on: applied, and nothing to say about it.
	if got := findings(`font-feature-settings: "onum"`); len(got) != 0 {
		t.Errorf(`"onum" on a face that declares it reported %q; it is applied`,
			got[0].Message)
	}
	// A tag the face has not got: carried out and inert, which is worth saying.
	if got := findings(`font-feature-settings: "zzzz"`); len(got) == 0 {
		t.Error(`"zzzz" on a face that does not declare it said nothing; asking a ` +
			`face for a feature it has not got changes no glyph and the page ` +
			`does not show that it was asked`)
	}
	// A tag turned off: not what this property can do.
	for _, decl := range []string{
		`font-feature-settings: "liga" 0`,
		`font-feature-settings: "liga" off`,
	} {
		got := findings(decl)
		if len(got) == 0 {
			t.Errorf("%s said nothing; a feature is turned off through "+
				"font-variant-ligatures rather than by tag", decl)
			continue
		}
		if !strings.Contains(got[0].Message, "turned off") {
			t.Errorf("%s reported %q, which does not say what was not done",
				decl, got[0].Message)
		}
	}
}

// TestFontFeatureSettingsSettlesTheOrderOfItsTags is what lets two declarations
// naming the same features share the shaped group the breaker memoises.
//
// The order a document writes them in is not the order they are applied in —
// that is the font's, by lookup index — so the two are one request and have to
// be spelt the same way by the time they reach the key.
func TestFontFeatureSettingsSettlesTheOrderOfItsTags(t *testing.T) {
	first, _ := featureSettingsOf(`"tnum", "onum"`)
	second, _ := featureSettingsOf(`"onum", "tnum"`)
	if first != second {
		t.Errorf("the same two features came out as %q and %q; they are one "+
			"request and share a memo entry only if they are spelt alike",
			first, second)
	}
	if first == "" {
		t.Fatal("two tags came out as nothing")
	}
	// And a tag named twice is one tag.
	if once, _ := featureSettingsOf(`"onum", "onum"`); once != "onum" {
		t.Errorf(`"onum" twice came out as %q, want "onum"`, once)
	}
}
