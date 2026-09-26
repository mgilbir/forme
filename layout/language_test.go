package layout

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// The two things audit C63 and C64 found layout did not pass to shaping: the
// language an element is in, and the script each part of a run is in.

// languageNotoSet is a set whose "Noto" family is the embedded Noto Sans, which covers
// Latin, Cyrillic and Devanagari and states Serbian forms for Cyrillic.
func languageNotoSet(t *testing.T) FontSet {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	return namedFaceSet{family: "Noto", face: face, standard: StandardFonts()}
}

// glyphsDrawnFor is the glyphs of every run drawn whose text contains want, as the
// backend shapes them.
func glyphsDrawnFor(t *testing.T, frag *Fragment, want string) ([]int, shape.Features) {
	t.Helper()
	var (
		out []int
		off shape.Features
	)
	for _, op := range Paint(frag) {
		v, ok := op.(DrawText)
		if !ok || !strings.Contains(v.Text, want) {
			continue
		}
		glyphs, _ := ShapedGlyphs(v)
		for _, g := range glyphs {
			out = append(out, g.GID)
		}
		off = v.Features
	}
	if out == nil {
		t.Fatalf("no run drawn holds %q", want)
	}
	return out, off
}

// TestTheLanguageReachesTheFont is C64 end to end. "б" is drawn differently in
// Serbian, and Noto Sans says so under 'SRB '; lang="sr" on the element has to
// reach both the measurement and the backend that shapes the run again.
func TestTheLanguageReachesTheFont(t *testing.T) {
	const css = `body{margin:0} p{font-family:Noto; font-size:20px}`
	plain, off := glyphsDrawnFor(t, laidOutIn(t, languageNotoSet(t), `<p>б</p>`, css), "б")
	if off.Language != "" {
		t.Errorf("a run in no language carries %q", off.Language)
	}
	for _, doc := range []string{
		`<p lang="sr">б</p>`,
		`<div lang="sr-Cyrl-RS"><p><span>б</span></p></div>`,
		`<p lang="SR">б</p>`,
	} {
		serbian, off := glyphsDrawnFor(t, laidOutIn(t, languageNotoSet(t), doc, css), "б")
		if sameGlyphIDs(serbian, plain) {
			t.Errorf("%s: б is drawn as glyphs %v, the same as in no language; the "+
				"font's Serbian form was not reached", doc, serbian)
		}
		if off.Language == "" {
			t.Errorf("%s: the run drawn carries no language", doc)
		}
	}
	// An empty lang is "unknown", and stops an ancestor's from applying.
	unknown, _ := glyphsDrawnFor(t, laidOutIn(t, languageNotoSet(t),
		`<div lang="sr"><p lang="">б</p></div>`, css), "б")
	if !sameGlyphIDs(unknown, plain) {
		t.Errorf(`lang="" inside lang="sr" draws б as %v; want the default %v`, unknown, plain)
	}
}

// TestALatinPrefixDoesNotStopADevanagariSuffixReordering is C63 on the page:
// "PDFकि" set in one face covering both scripts draws कि as कि alone is drawn,
// the vowel sign before its consonant.
func TestALatinPrefixDoesNotStopADevanagariSuffixReordering(t *testing.T) {
	const css = `body{margin:0} p{font-family:Noto; font-size:20px}`
	alone, _ := glyphsDrawnFor(t, laidOutIn(t, languageNotoSet(t), `<p>कि</p>`, css), "कि")
	mixed, _ := glyphsDrawnFor(t, laidOutIn(t, languageNotoSet(t), `<p>PDFकि</p>`, css), "कि")
	if len(mixed) < len(alone) || !sameGlyphIDs(mixed[len(mixed)-len(alone):], alone) {
		t.Errorf("PDFकि is drawn as %v, and कि alone as %v", mixed, alone)
	}
}

// TestAFontReadInPartIsReported: a font whose tables ask for more reading than
// the bound allows is set without what lies past it, and the document is told.
func TestAFontReadInPartIsReported(t *testing.T) {
	face, err := shape.Load(partlyReadFont(20000))
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "Wide", face: face, standard: StandardFonts()}
	_, findings := layoutWith(t, set, `<p>a</p>`, `p{font-family:Wide}`)
	var got []string
	for _, f := range findings {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "WideCoverage") {
			got = append(got, f.Message)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "not read") {
		t.Errorf("a font read in part is reported as %q; want one finding saying what was not read", got)
	}
	// And a font read whole is not reported.
	_, findings = layoutWith(t, languageNotoSet(t), `<p>a</p>`, `p{font-family:Noto}`)
	for _, f := range findings {
		if f.Rule == RuleLimit {
			t.Errorf("Noto Sans, read whole, is reported: %s", f.Message)
		}
	}
}

// partlyReadFont is shape's wideCoverageFont: one PairPos subtable whose
// coverage names the whole glyph space records times over, which is a few
// bytes each asking for sixty-five thousand glyphs. A pair lookup, because the
// flat reading of a face's kerning still expands the glyphs that begin a pair
// at load; a single adjustment is searched where a glyph is met and costs
// nothing to load.
func partlyReadFont(records int) []byte {
	sub := make([]byte, 12)
	binary.BigEndian.PutUint16(sub[0:], 1)      // posFormat 1
	binary.BigEndian.PutUint16(sub[4:], 0x0004) // valueFormat1: XAdvance
	binary.BigEndian.PutUint16(sub[8:], 1)      // one pair set
	binary.BigEndian.PutUint16(sub[10:], 12)    // at 12
	sub = append(sub, 0, 1, 0, 1, 0, 1)
	binary.BigEndian.PutUint16(sub[2:], uint16(len(sub))) // coverage offset
	cov := make([]byte, 4+6*records)
	binary.BigEndian.PutUint16(cov[0:], 2)
	binary.BigEndian.PutUint16(cov[2:], uint16(records))
	for i := 0; i < records; i++ {
		binary.BigEndian.PutUint16(cov[4+6*i:], 0)
		binary.BigEndian.PutUint16(cov[6+6*i:], 0xFFFF)
		binary.BigEndian.PutUint16(cov[8+6*i:], 0)
	}
	sub = append(sub, cov...)
	return fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "WideCoverage",
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOSLookups(
				[]fonttest.Lookup{{Type: 2, Subtables: [][]byte{sub}}},
				map[string][]int{"kern": {0}}),
		},
	})
}

func laidOutIn(t *testing.T, set FontSet, htmlSrc, cssSrc string) *Fragment {
	t.Helper()
	frag, _ := layoutWith(t, set, htmlSrc, cssSrc)
	if frag == nil {
		t.Fatal("the document laid out to nothing")
	}
	return frag
}

func sameGlyphIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
