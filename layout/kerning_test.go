package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Turning kerning off, and on again.
//
// "font-kerning: none" is applied — see layout/fontfeatures.go — and so is
// font-feature-settings, on and off, above it: CSS Fonts 4 §7.2 puts the
// property that names a feature by tag after everything else that asks for or
// against one. So nothing here is reported about turning kerning off or on,
// whatever the face has in it, and what is reported is a tag turned on that
// the face has nothing under.
//
// The first narrowing this file was written for still holds, for a different
// reason: "font-feature-settings: \"kern\" off" over a face with no kerning
// asks for the page that is already there. Five of the suite's reftests write
// it over text in the default serif face.

// kerningFindings lays a document out in a face and returns what was said about
// the two properties.
func kerningFindings(t *testing.T, css string, set FontSet) []Finding {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">AVAVAV</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-size: 20px; ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, set, rec)
	var out []Finding
	for _, f := range append(append([]Finding(nil), built.Findings...), rec.Findings()...) {
		if f.Property == "font-kerning" || f.Property == "font-feature-settings" {
			out = append(out, f)
		}
	}
	return out
}

// TestTurningOffKerningAFaceHasNotGotIsNotReported is the bug.
func TestTurningOffKerningAFaceHasNotGotIsNotReported(t *testing.T) {
	if face, ok := StandardFonts().Face("Times", false, false); !ok || face.HasKerning() {
		t.Skip("the standard serif face kerns, so this fixture cannot say anything")
	}
	for _, css := range []string{
		`font-family: Times; font-kerning: none`,
		`font-family: Times; font-feature-settings: "kern" off`,
		`font-family: Times; font-feature-settings: "kern" 0`,
		// Both together, which is how the suite writes it.
		`font-family: Times; font-kerning: none; font-feature-settings: "kern" off`,
	} {
		if got := kerningFindings(t, css, StandardFonts()); len(got) != 0 {
			t.Errorf("%q reported %q — the face has no kerning to turn off, so the "+
				"page is the one the declaration asked for", css, got[0].Message)
		}
	}
}

// TestTurningKerningOffOrOnByTagIsApplied is where the second narrowing used to
// stop: with a face that kerns, "kern" off asked for a page this engine did not
// produce, and was reported. It is applied now, and the widths say so. Six
// letters of AVAV are narrower kerned than not, "kern" off is as wide as
// font-kerning: none, and "kern" 1 beside font-kerning: none is as narrow as
// no declaration at all — font-feature-settings has the last word.
func TestTurningKerningOffOrOnByTagIsApplied(t *testing.T) {
	kerning := kerningFallbackFace(t)
	set := oneFace{kerning}
	width := func(css string) float64 {
		t.Helper()
		frag, _ := layoutWith(t, set, `<div id="d">AVAVAV</div>`,
			`body{margin:0} #d{font-size:20px; float:left; `+css+`}`)
		return find(t, frag, "d").BorderRect.W.Px()
	}
	kerned, plain := width(""), width("font-kerning: none")
	if kerned >= plain {
		t.Fatalf("AVAVAV is %gpx kerned and %gpx not; the face is not kerning the "+
			"pair, and nothing below can be told apart", kerned, plain)
	}
	for _, c := range []struct {
		css  string
		want float64
	}{
		{`font-feature-settings: "kern" off`, plain},
		{`font-feature-settings: "kern" 0`, plain},
		{`font-kerning: normal; font-feature-settings: "kern" 0`, plain},
		{`font-kerning: none; font-feature-settings: "kern" 1`, kerned},
		{`font-kerning: none; font-feature-settings: "kern" on`, kerned},
		{`font-kerning: none; font-feature-settings: "kern" 0, "kern" 1`, kerned},
		{`font-feature-settings: "kern" 1, "kern" 0`, plain},
	} {
		if got := width(c.css); got != c.want {
			t.Errorf("%s: AVAVAV is %gpx, want %gpx (kerned %g, not %g)",
				c.css, got, c.want, kerned, plain)
		}
		if got := kerningFindings(t, c.css, set); len(got) != 0 {
			t.Errorf("%s reported %q; it is applied", c.css, got[0].Message)
		}
	}
	// And asking for the kerning this engine already does says nothing, with a
	// kerning face as with any other.
	for _, css := range []string{`font-kerning: auto`, `font-kerning: normal`,
		`font-feature-settings: normal`, `font-kerning: none`} {
		if got := kerningFindings(t, css, set); len(got) != 0 {
			t.Errorf("%q reported %q; it asks for what the engine does", css, got[0].Message)
		}
	}
}

// TestAFeatureTheFaceHasNotGotIsReported is what is left of the finding: a tag
// turned on over a face that has nothing under it. The standard serif face
// carries no OpenType feature, so every tag turned on is one, in the same value
// as a tag turned off or not. A tag turned off over it is not reported, since
// turning off what is not there changes nothing.
func TestAFeatureTheFaceHasNotGotIsReported(t *testing.T) {
	for _, css := range []string{
		`font-family: Times; font-feature-settings: "smcp"`,
		`font-family: Times; font-feature-settings: "kern" off, "smcp"`,
		`font-family: Times; font-feature-settings: "halt" 1`,
	} {
		got := kerningFindings(t, css, StandardFonts())
		if len(got) == 0 {
			t.Errorf("%q was not reported; the face has nothing under the tag", css)
			continue
		}
		if !strings.Contains(got[0].Message, "font-feature-settings") {
			t.Errorf("%q reported %q", css, got[0].Message)
		}
	}
	for _, css := range []string{
		`font-family: Times; font-feature-settings: "liga" 0`,
		`font-family: Times; font-feature-settings: "kern" off, "liga" 0`,
	} {
		if got := kerningFindings(t, css, StandardFonts()); len(got) != 0 {
			t.Errorf("%q reported %q; turning off what is not there is the page "+
				"that is already there", css, got[0].Message)
		}
	}
}

// TestTheFindingIsRaisedOncePerDocument, which is what the other value findings
// do: a stylesheet rule on four hundred elements is one thing to be told.
func TestTheFindingIsRaisedOncePerDocument(t *testing.T) {
	kerning := kerningFallbackFace(t)
	built := Build(Input{
		HTML: `<p id="a">AV</p><p id="b">AV</p><p id="c">AV</p>`,
		CSS: []Stylesheet{{Source: noDefaults +
			"p { font-size: 20px; font-feature-settings: \"zzzz\" }"}},
	})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, oneFace{kerning}, rec)
	n := 0
	for _, f := range rec.Findings() {
		if f.Property == "font-feature-settings" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("three paragraphs raised the finding %d times, want once", n)
	}
}
