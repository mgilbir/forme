package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Turning kerning off, where there is kerning to turn off.
//
// This engine applies a face's kerning and offers no way to decline it, so
// "font-kerning: none" and "font-feature-settings: \"kern\" off" both ask for
// something it cannot do — and both ask for *nothing at all* when the face has
// no kerning in it. The fourteen standard PDF faces are that case: their metrics
// carry no kern pairs, so a document set in one of them and asking for kerning
// to be turned off gets exactly the page it asked for.
//
// That is not a corner of the suite. Five of its reftests write both
// declarations together — the belt-and-braces an author uses to make a layout
// test robust across UAs — over text in the default serif face, and every one of
// them was held out of the clean count by a finding about a page that is right.

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

// TestTurningOffKerningAFaceHasIsStillReported is the half the change had to
// keep, and it is where the narrowing stops: with a face that kerns, the page
// really does differ from the one the author asked for.
func TestTurningOffKerningAFaceHasIsStillReported(t *testing.T) {
	faces := notoFaces()
	var kerning *shape.Face
	for _, f := range faces {
		if f.HasKerning() {
			kerning = f
			break
		}
	}
	if kerning == nil {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face with kerning in it")
	}
	set := oneFace{kerning}
	for _, css := range []string{
		`font-kerning: none`,
		`font-feature-settings: "kern" off`,
	} {
		got := kerningFindings(t, css, set)
		if len(got) == 0 {
			t.Errorf("%q was not reported, and this face kerns", css)
			continue
		}
		if !got[0].Unsupported() {
			t.Errorf("%q: the finding was not marked unsupported", css)
		}
	}
	// And asking for the kerning this engine already does says nothing, with a
	// kerning face as with any other.
	for _, css := range []string{`font-kerning: auto`, `font-kerning: normal`,
		`font-feature-settings: normal`} {
		if got := kerningFindings(t, css, set); len(got) != 0 {
			t.Errorf("%q reported %q; it asks for what the engine does", css, got[0].Message)
		}
	}
}

// TestAFeatureThatIsNotKerningIsAlwaysReported is the other containment case,
// and the reason the value is judged tag by tag.
//
// "kern" is the one tag this can answer, because a face's kerning is a thing the
// shaping layer knows about. Any other tag is a feature this engine neither
// applies nor asks the face for, so a value naming one is reported whatever the
// face has in it — including in the same value as a "kern" that would have been
// inert on its own.
func TestAFeatureThatIsNotKerningIsAlwaysReported(t *testing.T) {
	for _, css := range []string{
		`font-family: Times; font-feature-settings: "liga" 0`,
		`font-family: Times; font-feature-settings: "smcp"`,
		`font-family: Times; font-feature-settings: "kern" off, "liga" 0`,
		`font-family: Times; font-feature-settings: "halt" 1`,
	} {
		got := kerningFindings(t, css, StandardFonts())
		if len(got) == 0 {
			t.Errorf("%q was not reported; this engine takes no direction about "+
				"which features a face applies", css)
			continue
		}
		if !strings.Contains(got[0].Message, "font-feature-settings") {
			t.Errorf("%q reported %q", css, got[0].Message)
		}
	}
}

// TestTheFindingIsRaisedOncePerDocument, which is what the other value findings
// do: a stylesheet rule on four hundred elements is one thing to be told.
func TestTheFindingIsRaisedOncePerDocument(t *testing.T) {
	faces := notoFaces()
	var kerning *shape.Face
	for _, f := range faces {
		if f.HasKerning() {
			kerning = f
			break
		}
	}
	if kerning == nil {
		t.Skip("set NOTO_FONTS for a face with kerning in it")
	}
	built := Build(Input{
		HTML: `<p id="a">AV</p><p id="b">AV</p><p id="c">AV</p>`,
		CSS:  []Stylesheet{{Source: noDefaults + `p { font-size: 20px; font-kerning: none }`}},
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
		if f.Property == "font-kerning" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("three paragraphs raised the finding %d times, want once", n)
	}
}
