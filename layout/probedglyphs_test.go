package layout

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// A face asked whether it could set some text has not set any of it.
//
// What a face records as used is what it is embedded with: a backend writes the
// subset Face.SubsetGlyphs keeps, and that is every glyph the face recorded.
// The coverage check that chooses a run's face (paragraph.MissesVisible) used
// to answer by shaping the text in the face being asked, and shaping records
// every glyph it makes — so a face that was asked and then not chosen kept the
// glyphs it would have drawn, in a file that never draws them.

// freshNamedFonts is the fallback library and four of its faces by family name,
// loaded for this test alone: what a face records is shared by every document
// laid out in it, and the shared harness set has recorded everything the suite
// ever drew.
func freshNamedFonts(t *testing.T) (FontSet, map[string]*shape.Face) {
	t.Helper()
	dir := fonttest.NotoDir(t)
	named := map[string]*shape.Face{}
	for family, file := range map[string]string{
		"noto sans":        "NotoSans-Regular.ttf",
		"noto sans hebrew": "NotoSansHebrew-Regular.ttf",
		"noto sans arabic": "NotoSansArabic-Regular.ttf",
		"noto sans jp":     "NotoSansJP-VF.ttf",
	} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		face, err := loadSuiteFace(data)
		if err != nil {
			t.Fatal(err)
		}
		named[family] = face
	}
	fallback := notoFaces()
	if len(fallback) == 0 {
		t.Fatal("the fallback library loaded nothing")
	}
	return suiteFonts{standard: StandardFonts(), fallback: fallback, named: named}, named
}

// glyphsOnPage is every glyph a display list draws, by face.
func glyphsOnPage(ops []Op, into map[*shape.Face]map[int]bool) {
	mark := func(f *shape.Face, gid int) {
		if into[f] == nil {
			into[f] = map[int]bool{}
		}
		into[f][gid] = true
	}
	for _, op := range ops {
		switch o := op.(type) {
		case DrawGlyphs:
			for _, g := range o.Glyphs {
				mark(o.Face, g.GID)
			}
		case DrawText:
			gs, _ := ShapedGlyphs(o)
			for _, g := range gs {
				mark(o.Face, g.GID)
			}
		case FilterGroup:
			glyphsOnPage(o.Ops, into)
		case ClipPath:
			glyphsOnPage(o.Ops, into)
		}
	}
}

// TestAFaceOnlyAskedRecordsNoGlyph lays out documents where a face the page
// draws in is also asked about text it does not end up setting, and holds what
// each face recorded to what the page draws.
//
// Glyph 0 is allowed: every subset keeps it, so recording it changes nothing.
func TestAFaceOnlyAskedRecordsNoGlyph(t *testing.T) {
	for _, c := range []struct{ what, html string }{
		{
			// A Latin letter carrying a Hebrew point is one cluster, and Noto
			// Sans has the letter and not the point: it is asked about the
			// pair and the pair goes elsewhere. Shaping the pair to ask
			// recorded Noto Sans's x, which nothing on the page is set in.
			"a cluster the primary face has half of",
			`<p style="font-family: 'Noto Sans'">word x&#x5B8; שלום</p>`,
		},
	} {
		set, _ := freshNamedFonts(t)
		out := Compose(Input{HTML: c.html, Fonts: set}, Options{Page: A4})
		drawn := map[*shape.Face]map[int]bool{}
		glyphsOnPage(out.Ops, drawn)
		if len(drawn) == 0 {
			t.Fatalf("%s: nothing was drawn, so this checks nothing", c.what)
		}
		for face, gids := range drawn {
			var extra []int
			for _, g := range face.Used() {
				if g != 0 && !gids[g] {
					extra = append(extra, g)
				}
			}
			slices.Sort(extra)
			if len(extra) > 0 {
				t.Errorf("%s: %s records glyphs %v that the page does not draw, "+
					"and its embedded subset keeps them", c.what, face.Name(), extra)
			}
		}
	}
}
