package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// The @font-face rule's font-feature-settings descriptor, CSS Fonts 4 §4.6: the
// features every run set in the face is shaped with, at §7.2's second step —
// above the face's defaults and below everything the document asks of a run.
//
// The face is forme's bundled Noto Sans, which kerns "AV", so every case is a
// width: "AVAVAV" kerned and not kerned are two numbers, and which one a
// document gets says which request won. The glyph-level oracle for the order
// is shape's TestAFacesOwnSettingsAreTheSecondStep, held against HarfBuzz.

// featureFaceDoc lays out AVAVAV in the family Trial, which one @font-face rule
// with the given extra descriptors loads, and returns the run's width and the
// document's findings.
func featureFaceDoc(t *testing.T, descriptors, css string) (float64, []Finding) {
	t.Helper()
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	out := Compose(Input{
		HTML: `<style>@font-face { font-family: Trial; src: url(trial.ttf); ` + descriptors + ` }
			body { margin: 0 } #d { font-family: Trial; font-size: 20px; float: left; ` + css + ` }
			</style><div id="d">AVAVAV</div>`,
		Resources: res,
	}, Options{})
	d := find(t, out.Root, "d")
	return d.BorderRect.W.Px(), out.Findings
}

// TestAFontFaceStatesItsFeatures is the descriptor applied, and each step of
// §7.2 around it.
func TestAFontFaceStatesItsFeatures(t *testing.T) {
	kerned, _ := featureFaceDoc(t, "", "")
	plain, _ := featureFaceDoc(t, "", "font-kerning: none")
	if kerned >= plain {
		t.Fatalf("AVAVAV is %gpx kerned and %gpx not; the face is not kerning the "+
			"pair, and nothing below can be told apart", kerned, plain)
	}
	for _, c := range []struct {
		descriptor, css string
		want            float64
	}{
		// Step 2 alone.
		{`font-feature-settings: "kern" 0`, "", plain},
		{`font-feature-settings: "kern" off`, "", plain},
		{`font-feature-settings: "kern" 1`, "", kerned},
		{`font-feature-settings: normal`, "", kerned},
		// The last setting of a tag stands.
		{`font-feature-settings: "kern" 0, "kern" 1`, "", kerned},
		{`font-feature-settings: "kern" 1, "kern" 0`, "", plain},
		// And the last declaration that can be read: one that cannot is
		// dropped, and the one before it stands.
		{`font-feature-settings: "kern" 0; font-feature-settings: "kern" bogus`, "", plain},
		{`font-feature-settings: "kern" 0; font-feature-settings: "kern" 1`, "", kerned},
		// Step 3 over step 2: font-kerning. "normal" asks for the kerning and
		// turns it back on; "auto" asks nothing and leaves the face's setting.
		{`font-feature-settings: "kern" 0`, "font-kerning: normal", kerned},
		{`font-feature-settings: "kern" 0`, "font-kerning: auto", plain},
		{`font-feature-settings: "kern" 1`, "font-kerning: none", plain},
		// Step 5 over step 2: the property has the last word.
		{`font-feature-settings: "kern" 0`, `font-feature-settings: "kern" 1`, kerned},
		{`font-feature-settings: "kern" 1`, `font-feature-settings: "kern" 0`, plain},
		// A property that says nothing about the tag leaves the face's setting.
		{`font-feature-settings: "kern" 0`, `font-feature-settings: "liga" 1`, plain},
	} {
		got, findings := featureFaceDoc(t, c.descriptor, c.css)
		if got != c.want {
			t.Errorf("%s with %q: AVAVAV is %gpx, want %gpx (kerned %g, not %g)",
				c.descriptor, c.css, got, c.want, kerned, plain)
		}
		for _, f := range findings {
			if f.Property == "font-feature-settings" && strings.Contains(f.Message, "is not applied") {
				t.Errorf("%s: still reported as not applied: %s", c.descriptor, f.Message)
			}
		}
	}
}

// TestAFontFaceFeatureDescriptorIsJudged: the descriptor takes the property's
// grammar and is judged by the same code, and what cannot be read is said.
func TestAFontFaceFeatureDescriptorIsJudged(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	build := func(descriptor string) Built {
		return Build(Input{
			HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); ` + descriptor + ` }`),
			Resources: res,
		})
	}
	// Not CSS: the property's grammar refuses every one of these.
	for _, bad := range []string{
		`font-feature-settings: "kern" bogus`,
		`font-feature-settings: kern`,
		`font-feature-settings: "kerning"`,
		`font-feature-settings: "kern" -1`,
		`font-feature-settings: "kern" 1.5`,
		`font-feature-settings: inherit`,
	} {
		built := build(bad)
		requireFinding(t, built.Findings, RuleInvalidCSS, `"font-feature-settings" is not a value`)
		if face, ok := built.Fonts.Face("Trial", false, false); !ok || len(face.FeatureSettings()) != 0 {
			t.Errorf("%s: the face was loaded with settings %v", bad, face.FeatureSettings())
		}
	}
	// CSS this engine does not evaluate.
	built := build(`font-feature-settings: "kern" calc(0)`)
	requireFinding(t, built.Findings, RuleUnsupportedValue, "does not evaluate")
	// A tag that is CSS and cannot name a feature here, beside one that can.
	built = build(`font-feature-settings: "a,bc", "kern" 0`)
	requireFinding(t, built.Findings, RuleUnsupportedValue, "cannot name a feature")
	face, _ := built.Fonts.Face("Trial", false, false)
	if got := face.FeatureSettings(); len(got) != 1 || got[0] != (shape.FeatureSetting{Tag: "kern"}) {
		t.Errorf(`"a,bc", "kern" 0 loaded the face with %v, want "kern" off alone`, got)
	}
	// A tag turned on that the face has nothing under: the text is set
	// without it, which is not the page the rule asked for.
	built = build(`font-feature-settings: "zzzz", "kern" 0`)
	requireFinding(t, built.Findings, RuleUnsupportedValue, `asks for "zzzz"`)
	// And one it has is not reported, nor is one turned off.
	built = build(`font-feature-settings: "kern" 1, "liga" 0, "zzzz" 0`)
	for _, f := range built.Findings {
		if f.Property == "font-feature-settings" {
			t.Errorf(`"kern" 1, "liga" 0, "zzzz" 0 reported %q`, f.Message)
		}
	}
}

// TestAFontFacesSettingsAreItsOwn: two rules naming one file are one load, and
// a rule that states settings gets a face of its own for them — so the family
// that states none is set as the file is, and two rules stating the same
// settings share one face.
func TestAFontFacesSettingsAreItsOwn(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML: docWithFontFace(`
			@font-face { font-family: Plain; src: url(trial.ttf); }
			@font-face { font-family: Unkerned; src: url(trial.ttf); font-feature-settings: "kern" 0; }
			@font-face { font-family: Again; src: url(trial.ttf); font-feature-settings: "kern" off; }`),
		Resources: res,
	})
	if n := strings.Count(strings.Join(res.asked, ","), "trial.ttf"); n != 1 {
		t.Errorf("the file was read %d times, want once", n)
	}
	plain, _ := built.Fonts.Face("Plain", false, false)
	unkerned, _ := built.Fonts.Face("Unkerned", false, false)
	again, _ := built.Fonts.Face("Again", false, false)
	if plain == nil || unkerned == nil || again == nil {
		t.Fatalf("a family did not resolve; findings: %v", built.Findings)
	}
	if plain == unkerned {
		t.Error("the family with settings shares the face of the one without")
	}
	if unkerned != again {
		t.Error("two families with the same settings have a face each")
	}
	if len(plain.FeatureSettings()) != 0 {
		t.Errorf("the family with no settings has %v", plain.FeatureSettings())
	}
	kern := plain.MeasureShaped("AVAVAV", 20)
	if got := unkerned.MeasureShaped("AVAVAV", 20); got <= kern {
		t.Errorf(`the "kern" 0 face measures AVAVAV at %g, not wider than the plain face's %g`, got, kern)
	}
}

// TestTheBackendShapesWithTheFacesSettings: a backend shapes a run again from
// its DrawText, and has to get the run layout measured — which it does by
// asking the same face, whose settings travel with it. The glyphs' advances,
// summed, are the width layout gave the run.
func TestTheBackendShapesWithTheFacesSettings(t *testing.T) {
	for _, descriptor := range []string{`font-feature-settings: "kern" 0`, ""} {
		res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
		out := Compose(Input{
			HTML: `<style>@font-face { font-family: Trial; src: url(trial.ttf); ` + descriptor + ` }
				body { margin: 0 } #d { font-family: Trial; font-size: 20px; float: left }
				</style><div id="d">AVAVAV</div>`,
			Resources: res,
		}, Options{})
		var run *DrawText
		for _, op := range out.Ops {
			if v, ok := op.(DrawText); ok && v.Text == "AVAVAV" {
				run = &v
			}
		}
		if run == nil {
			t.Fatalf("%q: no run AVAVAV was drawn", descriptor)
		}
		glyphs, _ := ShapedGlyphs(*run)
		sum := 0.0
		for _, g := range glyphs {
			sum += g.XAdvance
		}
		// A glyph advances in thousandths of an em, whatever the face's own
		// units are; see shape.Glyph.
		drawn := sum * run.Size.Px() / 1000
		width := find(t, out.Root, "d").BorderRect.W.Px()
		if diff := drawn - width; diff > 0.05 || diff < -0.05 {
			t.Errorf("%q: the backend's glyphs come to %gpx and layout gave the run %gpx", descriptor, drawn, width)
		}
	}
}

// TestAFeatureSettingsTagIsReadAsTheStringItIs: a tag is a CSS string, and the
// cascade writes one back with its quotation marks escaped. Read as text cut at
// the first quotation mark of either kind, a tag holding one was read as a
// shorter one and dropped with nothing said; read as tokens it is the tag it
// is — here, one this face has nothing under, and so reported.
func TestAFeatureSettingsTagIsReadAsTheStringItIs(t *testing.T) {
	for _, c := range []struct{ css, tag string }{
		{`font-family: Times; font-feature-settings: "a'bc"`, `a'bc`},
		{`font-family: Times; font-feature-settings: 'a"bc' 1`, `a"bc`},
		{`font-family: Times; font-feature-settings: "a\"bc"`, `a"bc`},
	} {
		on, _ := featureSettingsOf(styleValueOf(t, c.css, "font-feature-settings"))
		if on != c.tag {
			t.Errorf("%s: read as %q, want %q", c.css, on, c.tag)
		}
		if got := kerningFindings(t, c.css, StandardFonts()); len(got) == 0 {
			t.Errorf("%s: nothing was reported, and the face has nothing under the tag", c.css)
		}
	}
	// A tag with a comma in it is CSS, and cannot be carried: said, and the
	// rest applied.
	css := `font-family: Times; font-feature-settings: "a,bc", "kern" 0`
	if on, off := featureSettingsOf(styleValueOf(t, css, "font-feature-settings")); on != "" ||
		len(off) != 1 || off[0] != "kern" {
		t.Errorf("%s: read as on %q, off %v; want \"kern\" off alone", css, on, off)
	}
	got := kerningFindings(t, css, StandardFonts())
	if len(got) == 0 || !strings.Contains(got[0].Message, "cannot name a feature") {
		t.Errorf("%s: reported %v", css, got)
	}
}

// styleValueOf is the value the cascade gives a property on a div declaring
// css: the text layout reads it back from.
func styleValueOf(t *testing.T, css, property string) string {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">x</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	return findBox(t, built.Root, "d").Style.Get(property)
}
