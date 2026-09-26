package layout

import (
	"testing"
)

// CSS Fonts 4 §5.1: "User agents must match these names case insensitively,
// using the "Default Caseless Matching" algorithm", "without normalizing the
// strings involved and without applying any language-specific tailorings",
// with CaseFolding.txt's status C and F. The names were lowered with
// strings.ToLower, which is not that algorithm: it keeps ß apart from SS, final
// ς apart from Σ, and ſ apart from s.

// TestAnAtFontFaceFamilyMatchesCaselessly is the document's own families: a
// name declared in one spelling and asked for in another resolves to the
// declared face where the two fold alike, and not where they do not.
func TestAnAtFontFaceFamilyMatchesCaselessly(t *testing.T) {
	want, err := loadRealFace()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		declared, asked string
		match           bool
		why             string
	}{
		{"Trial", "TRIAL", true, "ASCII, as before"},
		{"Straße", "STRASSE", true, "ß folds to ss"},
		{"STRASSE", "straße", true, "and the other way round"},
		{"\u212Aelvin", "kelvin", true, "the KELVIN SIGN folds to k"},
		{"ΣΟΦΟΣ", "σοφος", true, "Σ and final ς both fold to σ"},
		{"İnce", "i\u0307nce", true, "İ folds to i and a combining dot"},
		{"İnce", "Ince", false, "and not to i: no Turkic tailoring"},
		{"Ince", "ınce", false, "dotless ı folds to nothing but itself"},
		{"a\u030Ange", "ånge", false, "no normalization: §5.1's own note"},
	} {
		res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
		built := Build(Input{
			HTML:      `<p>x</p>`,
			CSS:       []Stylesheet{{Source: `@font-face { font-family: "` + tc.declared + `"; src: url(trial.ttf) }`}},
			Resources: res,
		})
		face, ok := built.Fonts.Face(tc.asked, false, false)
		got := ok && face != nil && face.Measure("Hamburgefonstiv", 20) == want.Measure("Hamburgefonstiv", 20)
		if got != tc.match {
			t.Errorf("%s: %q declared and %q asked for: matched %v, want %v", tc.why,
				tc.declared, tc.asked, got, tc.match)
		}
	}
}

// TestAFontFamilyListMatchesCaselessly is the same through the cascade: a
// font-family property naming the family in another spelling sets the text in
// the declared face, which measures differently from the standard one it would
// fall back to.
func TestAFontFamilyListMatchesCaselessly(t *testing.T) {
	width := func(family string) float64 {
		res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
		built := Build(Input{
			HTML: `<span id="s">Hamburgefonstiv</span>`,
			CSS: []Stylesheet{{Source: `@font-face { font-family: "Straße"; src: url(trial.ttf) }
				body { margin: 0 } #s { float: left; font-size: 20px; font-family: ` + family + ` }`}},
			Resources: res,
		})
		root := Layout(built.Root, Size{W: upx(t, 800), H: upx(t, 800)}, built.Fonts, nil)
		return find(t, root, "s").BorderRect.W.Px()
	}
	declared := width(`"Straße"`)
	if fallback := width(`"Nothing Like It", serif`); fallback == declared {
		t.Fatalf("the declared face and the fallback are both %gpx wide, so this proves "+
			"nothing", declared)
	}
	for _, family := range []string{`STRASSE`, `"strasse"`, `"STRAßE"`} {
		if got := width(family); got != declared {
			t.Errorf("font-family: %s is %gpx wide, want the declared face's %gpx", family,
				got, declared)
		}
	}
}

// TestTheFallbackListMatchesCaselessly is the family list walked per character:
// two families with disjoint unicode-ranges, named in the list in spellings
// other than their declarations' — and other than any lowercasing of them,
// which is what an ASCII or a strings.ToLower key would find. Each still sets
// the character it was declared for, which takes both the restricted-list
// check and the per-family lookup folding the names alike.
func TestTheFallbackListMatchesCaselessly(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"a.ttf": realFont(), "b.ttf": realFont()}}
	built := Build(Input{
		HTML: `<div id="d">ab</div>`,
		CSS: []Stylesheet{{Source: `
			@font-face { font-family: "Straße"; src: url(a.ttf); unicode-range: U+0061 }
			@font-face { font-family: "ΣΟΦΟΣ"; src: url(b.ttf); unicode-range: U+0062 }
			#d { font-family: "STRAßE", "σοφος" }`}},
		Resources: res,
	})
	box := findBox(t, built.Root, "d")
	l := &layouter{fontSet: built.Fonts, rec: NewRecorder(nil), fonts: map[fontKey]resolvedFont{}}
	primary, _ := l.fontFor(box)
	runs := l.faceRunsFor(box, primary, "ab")
	if len(runs) != 2 || runs[0].Text != "a" || runs[1].Text != "b" || runs[0].Face == runs[1].Face {
		t.Errorf("the text was cut as %v; each family sets the character it was declared "+
			"for", runsText(runs))
	}
}

// TestLocalAndTheStandardFacesMatchCaselessly: a local() name is looked up in
// the caller's set, whose standard faces are keyed by family, and folds the
// same way — "CONſOLAS", whose long s folds to s, is Consolas, which the
// standard set answers with Courier. A generic family is a keyword, though,
// and a keyword is ASCII case-insensitive: "SERIF" is the keyword, and
// "ſerif", which only folds to it, is a name nobody has.
func TestLocalAndTheStandardFacesMatchCaselessly(t *testing.T) {
	std := StandardFonts()
	courier, ok := std.Face("Courier", false, false)
	if !ok {
		t.Fatal("the standard set has no Courier")
	}
	for _, name := range []string{"CONSOLAS", "CONſOLAS", "ConSolaS"} {
		if f, ok := std.Face(name, false, false); !ok || f != courier {
			t.Errorf("%q: the standard set answered %v, want Courier", name, ok)
		}
	}
	if _, ok := std.Face("SERIF", false, false); !ok {
		t.Error(`"SERIF" is the generic keyword, and the standard set did not answer it`)
	}
	for _, name := range []string{"ſerif", "ſans-serif", "monoſpace"} {
		if _, ok := std.Face(name, false, false); ok {
			t.Errorf("%q folds to a generic family's name and is not the keyword; the "+
				"standard set answered it", name)
		}
	}

	// And through local() in an @font-face rule.
	built := Build(Input{
		HTML: `<p>x</p>`,
		CSS:  []Stylesheet{{Source: `@font-face { font-family: Mono; src: local("CONſOLAS") }`}},
	})
	face, ok := built.Fonts.Face("Mono", false, false)
	if !ok || face.Measure("iiii", 20) != courier.Measure("iiii", 20) {
		t.Errorf(`local("CONſOLAS") did not resolve to Courier (found %v); findings: %v`,
			ok, built.Findings)
	}
}
