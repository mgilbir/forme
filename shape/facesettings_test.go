package shape

import (
	"slices"
	"testing"
)

// CSS Fonts 4 §7.2's second step: the font-feature-settings an @font-face rule
// states about the face it loads, above the features on by default and below
// everything a document asks of a run. See Face.WithFeatureSettings.
//
// Each case is one face's settings and one run's request, and names the flat
// list the two come to — the one a browser hands HarfBuzz, the face's settings
// first and the run's after, a later setting of a tag overriding an earlier one.
// The expected glyphs and advances are HarfBuzz 14.5.0's for that list, shaped
// through uharfbuzz 0.56.2 over the fixtures TestWritePrecedenceFixtures writes
// out, the same fonts TestFeatureSettingsHaveTheLastWord holds the other steps
// against.
func TestAFacesOwnSettingsAreTheSecondStep(t *testing.T) {
	fonts := map[string]*Face{}
	for name, data := range precedenceFixtures() {
		f, err := Load(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fonts[name] = f
	}
	ligature := []shapedAs{{goAB, 1100, 0, 0}}
	apart := []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}}
	kerned := []shapedAs{{goB, 560, 0, 0}, {goA, 500, 0, 0}}
	unkerned := []shapedAs{{goB, 600, 0, 0}, {goA, 500, 0, 0}}
	small := []shapedAs{{goX, 700, 0, 0}}
	plainA := []shapedAs{{goA, 500, 0, 0}}
	half := float64(lkTighten / 2)
	legacyKerned := []shapedAs{{lkA, lkAdvance + half, 0, 0}, {lkV, lkAdvance + half, half, 0}}
	legacyPlain := []shapedAs{{lkA, lkAdvance, 0, 0}, {lkV, lkAdvance, 0, 0}}
	alternate := []shapedAs{{2, 900, 0, 0}}
	syllable := []shapedAs{{1, 1000, 0, 0}}

	on := func(tag string) FeatureSetting { return FeatureSetting{Tag: tag, On: true} }
	off := func(tag string) FeatureSetting { return FeatureSetting{Tag: tag} }

	for _, c := range []struct {
		what, font, text string
		face             []FeatureSetting
		run              Features
		harfbuzz         string
		want             []shapedAs
	}{
		// Step 2 against step 1: the face turning a default off, and turning
		// on a feature nothing is given by default.
		{`face "liga" 0`, "features", "ab", []FeatureSetting{off("liga")}, Features{}, "-liga", apart},
		{`face "kern" 0`, "features", "ba", []FeatureSetting{off("kern")}, Features{}, "-kern", unkerned},
		{`face "kern" 0, legacy table`, "legacy", "AV", []FeatureSetting{off("kern")}, Features{}, "-kern", legacyPlain},
		{`face "smcp" 1`, "features", "a", []FeatureSetting{on("smcp")}, Features{}, "smcp", small},
		{`Hangul, face "calt" 0`, "hangul", "가", []FeatureSetting{off("calt")}, Features{}, "-calt", syllable},

		// A tag set twice takes the last setting.
		{`face "liga" 0, "liga" 1`, "features", "ab", []FeatureSetting{off("liga"), on("liga")},
			Features{}, "liga", ligature},
		{`face "liga" 1, "liga" 0`, "features", "ab", []FeatureSetting{on("liga"), off("liga")},
			Features{}, "-liga", apart},

		// Step 3 over step 2: font-kerning and the font-variant properties.
		{`face "kern" 1; font-kerning: none`, "features", "ba", []FeatureSetting{on("kern")},
			Features{NoKerning: true}, "-kern", unkerned},
		{`face "kern" 1; font-kerning: none, legacy table`, "legacy", "AV", []FeatureSetting{on("kern")},
			Features{NoKerning: true}, "-kern", legacyPlain},
		// font-kerning: normal asks for the kerning at this step, where auto
		// asks nothing and leaves the face's "kern" 0 standing (the second
		// case of this list).
		{`face "kern" 0; font-kerning: normal`, "features", "ba", []FeatureSetting{off("kern")},
			Features{KerningOn: true}, "kern", kerned},
		{`face "kern" 0; font-kerning: normal, legacy table`, "legacy", "AV", []FeatureSetting{off("kern")},
			Features{KerningOn: true}, "kern", legacyKerned},
		{`face "liga" 1; font-variant-ligatures: none`, "features", "ab", []FeatureSetting{on("liga")},
			Features{NoOptionalLigatures: true, NoContextualAlternates: true}, "-liga,-clig,-dlig,-hlig", apart},
		{`face "smcp" 0; font-variant-caps: small-caps`, "features", "a", []FeatureSetting{off("smcp")},
			Features{Caps: CapsSmall}, "smcp", small},
		{`Hangul, face "calt" 1; font-variant-ligatures: no-contextual`, "hangul", "가",
			[]FeatureSetting{on("calt")}, Features{NoContextualAlternates: true}, "-calt", syllable},

		// Step 4 over step 2: letter-spacing turning the optional ligatures off.
		{`face "liga" 1; letter-spacing`, "features", "ab", []FeatureSetting{on("liga")},
			Features{NoOptionalLigatures: true}, "-liga,-clig,-dlig,-hlig", apart},

		// Step 5 over step 2, both ways: the property has the last word.
		{`face "liga" 0; "liga" 1`, "features", "ab", []FeatureSetting{off("liga")},
			Features{Tags: "liga"}, "liga", ligature},
		{`face "kern" 0; "kern" 1`, "features", "ba", []FeatureSetting{off("kern")},
			Features{Tags: "kern"}, "kern", kerned},
		{`face "kern" 0; "kern" 1, legacy table`, "legacy", "AV", []FeatureSetting{off("kern")},
			Features{Tags: "kern"}, "kern", legacyKerned},
		{`face "smcp" 1; "smcp" 0`, "features", "a", []FeatureSetting{on("smcp")},
			Features{TagsOff: "smcp"}, "-smcp", plainA},
		{`Hangul, face "calt" 0; "calt" 1`, "hangul", "가", []FeatureSetting{off("calt")},
			Features{Tags: "calt"}, "calt", alternate},
	} {
		face := fonts[c.font].WithFeatureSettings(c.face)
		got, missing := face.ShapeGlyphsInContext(c.text, "", "", c.run)
		if missing != 0 {
			t.Fatalf("%s: %d characters have no glyph", c.what, missing)
		}
		checkShaped(t, c.what+" ("+c.harfbuzz+") over "+c.text, got, c.want)

		// And by every other way into shaping, which all have to find the
		// face's settings for themselves: with no request at all where the
		// run asks nothing, and through a Stack, which shapes its runs by a
		// path of its own.
		if c.run == (Features{}) {
			got, _ := face.ShapeGlyphs(c.text)
			checkShaped(t, c.what+" through ShapeGlyphs", got, c.want)
			st := NewStack(face)
			runs, _ := st.ShapeRuns(c.text)
			var all []Glyph
			for _, r := range runs {
				all = append(all, r.Glyphs...)
			}
			checkShaped(t, c.what+" through a Stack", all, c.want)
		}
	}
}

// TestAFacesSettingsDoNotReachTheFaceTheyCameFrom: WithFeatureSettings is a
// copy, and the face it was made from shapes as it did.
func TestAFacesSettingsDoNotReachTheFaceTheyCameFrom(t *testing.T) {
	f, err := Load(precedenceFixtures()["features"])
	if err != nil {
		t.Fatal(err)
	}
	g := f.WithFeatureSettings([]FeatureSetting{{Tag: "liga"}})
	got, _ := f.ShapeGlyphs("ab")
	checkShaped(t, "the original face", got, []shapedAs{{goAB, 1100, 0, 0}})
	got, _ = g.ShapeGlyphs("ab")
	checkShaped(t, "the copy", got, []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}})
	// And the original again, after the copy has built and cached its plan
	// in the cache the two share: the settings are part of what a plan is
	// kept under.
	got, _ = f.ShapeGlyphs("ab")
	checkShaped(t, "the original face after the copy", got, []shapedAs{{goAB, 1100, 0, 0}})
	if len(f.FeatureSettings()) != 0 {
		t.Errorf("the original face states settings: %v", f.FeatureSettings())
	}
}

// TestAFacesSettingsReachTheKerningQuestions: the pair kerned across a run's
// edge and whether a neighbour can move a run are both 'kern', and follow the
// face's settings as the run's own kerning does.
func TestAFacesSettingsReachTheKerningQuestions(t *testing.T) {
	fonts := precedenceFixtures()
	f, err := Load(fonts["features"])
	if err != nil {
		t.Fatal(err)
	}
	noKern := f.WithFeatureSettings([]FeatureSetting{{Tag: "kern"}})
	got, _ := noKern.ShapeGlyphsInContext("b", "", "a", Features{})
	checkShaped(t, `face "kern" 0: b before a in the next run`, got, []shapedAs{{goB, 600, 0, 0}})
	got, _ = noKern.ShapeGlyphsInContext("b", "", "a", Features{Tags: "kern"})
	checkShaped(t, `face "kern" 0; "kern" 1: b before a in the next run`, got, []shapedAs{{goB, 560, 0, 0}})

	legacy, err := Load(fonts["legacy"])
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.ContextCanChange("A", Features{}) {
		t.Fatal("the legacy face's neighbour cannot kern it with nothing asked; the case below proves nothing")
	}
	if legacy.WithFeatureSettings([]FeatureSetting{{Tag: "kern"}}).ContextCanChange("A", Features{}) {
		t.Error(`face "kern" 0: ContextCanChange says a neighbour can still kern the run`)
	}
	if !legacy.WithFeatureSettings([]FeatureSetting{{Tag: "kern", On: true}}).
		ContextCanChange("A", Features{NoKerning: true, Tags: "kern"}) {
		t.Error(`face "kern" 1; "kern" 1 over font-kerning: none: ContextCanChange says no neighbour can kern the run`)
	}
}

// TestTurnsOffReadsTheFacesSettings is turnsOff with the second step in it,
// asked directly.
func TestTurnsOffReadsTheFacesSettings(t *testing.T) {
	for _, c := range []struct {
		what string
		f    Features
		tag  string
		off  bool
	}{
		{"face off", Features{faceOff: "kern"}, "kern", true},
		{"face on", Features{faceOn: "kern"}, "kern", false},
		{"face on, font-kerning: none", Features{faceOn: "kern", NoKerning: true}, "kern", true},
		{"face off, font-kerning: normal", Features{faceOff: "kern", KerningOn: true}, "kern", false},
		{"font-kerning: normal and none", Features{KerningOn: true, NoKerning: true}, "kern", true},
		{"font-kerning: normal, property off", Features{KerningOn: true, TagsOff: "kern"}, "kern", true},
		{"face off, property on", Features{faceOff: "kern", Tags: "kern"}, "kern", false},
		{"face on, property off", Features{faceOn: "calt", TagsOff: "calt"}, "calt", true},
		{"face on, no-contextual", Features{faceOn: "calt", NoContextualAlternates: true}, "calt", true},
		{"face off, small-caps", Features{faceOff: "smcp", Caps: CapsSmall}, "smcp", false},
		{"face off, another variant", Features{faceOff: "smcp", Numeric: NumericOldstyle}, "smcp", true},
		// A tag is matched whole.
		{"face off kerx", Features{faceOff: "kerx"}, "kern", false},
	} {
		if got := c.f.turnsOff(c.tag); got != c.off {
			t.Errorf("%s: turnsOff(%q) = %v, want %v", c.what, c.tag, got, c.off)
		}
	}
}

// TestFeatureSettingsAreSettled: one entry per tag, the last setting of each,
// in tag order; and a tag the settled form cannot carry is left out.
func TestFeatureSettingsAreSettled(t *testing.T) {
	f, err := Load(precedenceFixtures()["features"])
	if err != nil {
		t.Fatal(err)
	}
	g := f.WithFeatureSettings([]FeatureSetting{
		{Tag: "smcp", On: true}, {Tag: "liga"}, {Tag: "kern"}, {Tag: "smcp"},
		{Tag: "liga", On: true}, {Tag: "a,bc", On: true}, {Tag: "abc"}, {Tag: "ab\x7fc"},
	})
	want := []FeatureSetting{{Tag: "kern"}, {Tag: "liga", On: true}, {Tag: "smcp"}}
	if got := g.FeatureSettings(); !slices.Equal(got, want) {
		t.Errorf("FeatureSettings() = %v, want %v", got, want)
	}
}
