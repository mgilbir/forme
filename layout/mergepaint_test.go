package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A ligature is one glyph, drawn once, by the run that owns it — so two runs
// may share one only where everything the painter decides per run is the same
// for both. The merge group was built from the colour, the face, the size and
// the baseline, and the painter also decides per run whether it is drawn at
// all, how translucent it is and where a relative offset moves it (audit
// C100).

// paintedInNoto lays a document out in the embedded Noto Sans, whose ffi and
// fi ligatures are what make a merge group matter, and paints it.
func paintedInNoto(t *testing.T, htmlSrc, cssSrc string) []DrawText {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults +
		`p { font-size: 20px }` + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, notoSet{face: embeddedFallback(t)},
		NewRecorder(nil))
	var out []DrawText
	for _, op := range Paint(frag) {
		if d, ok := op.(DrawText); ok {
			out = append(out, d)
		}
	}
	return out
}

// TestAGlyphIsNotSharedAcrossWhatThePainterDecidesPerRun: the middle "f" of
// "office" is set apart from its neighbours by each property in turn. No drawn
// run may be shaped with the f's text, because a glyph that took the f in
// would be drawn by a run the f is not painted as.
//
// The plain word is the control: there the three runs are one group, so the
// assertion below can see a group when there is one.
func TestAGlyphIsNotSharedAcrossWhatThePainterDecidesPerRun(t *testing.T) {
	merged := func(runs []DrawText) bool {
		for _, r := range runs {
			if r.MergePre != "" || r.MergePost != "" {
				return true
			}
		}
		return false
	}
	if runs := paintedInNoto(t, `<p>of<span>f</span>ice</p>`, ``); !merged(runs) {
		t.Fatalf("a plain span inside a word made no group, so this test cannot "+
			"see one: %+v", runs)
	}
	for _, decl := range []string{
		"visibility: hidden",
		"opacity: 0.5",
		"position: relative; top: 5px",
	} {
		for _, markup := range []string{
			`<p>o<span id="f">f</span>fice</p>`,
			`<p>of<span id="f">f</span>ice</p>`,
		} {
			for _, r := range paintedInNoto(t, markup, `#f { `+decl+` }`) {
				if r.MergePre != "" || r.MergePost != "" {
					t.Errorf("%s, %s: the run %q is shaped with %q and %q, across "+
						"a boundary the painter draws each side of differently",
						decl, markup, r.Text, r.MergePre, r.MergePost)
				}
			}
		}
	}
}

// TestNoVisibleLetterIsLostToAHiddenOne is the page the defect made: every
// visible letter of "o<hidden f>fice" is drawn by a visible run. Shaped with
// the hidden f, the ffi ligature belonged to the hidden run and "fi" was drawn
// by nobody.
func TestNoVisibleLetterIsLostToAHiddenOne(t *testing.T) {
	var drawn int
	for _, r := range paintedInNoto(t, `<p>o<span style="visibility: hidden">f</span>fice</p>`, ``) {
		glyphs, _ := ShapedGlyphs(r)
		drawn += len(glyphs)
		if strings.Contains(r.MergePre+r.MergePost, "f") && r.Text == "fice" {
			t.Errorf("the visible run %q is shaped with the hidden f", r.Text)
		}
	}
	// "o", and "fice" as an fi ligature and two letters at the fewest.
	if drawn < 4 {
		t.Errorf("the visible runs draw %d glyphs, too few for \"o\" and \"fice\"", drawn)
	}
}
