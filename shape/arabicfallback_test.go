package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// arabicFallbackGlyphs is a face that maps the Arabic letters it has and
// their presentation forms, and states no joining forms of its own: the face
// HarfBuzz's Arabic fallback is for. Glyph i+1 is entry i, and each has an
// advance of its own, so that a glyph can be told by its advance too.
func arabicFallbackGlyphs() []fonttest.Glyph {
	var g []fonttest.Glyph
	for i, r := range []rune{
		0x0628, 0xFE8F, 0xFE90, 0xFE91, 0xFE92, // beh, and its isolated, final, initial and medial forms
		0x0644, 0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0, // lam
		0x0627, 0xFE8D, 0xFE8E, // alef, isolated and final
		0xFEFB, 0xFEFC, // lam-alef, isolated and final
		0x0647, 0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC, // heh
		0xF201, // the private-use lellah ligature
	} {
		g = append(g, fonttest.Glyph{Rune: r, Advance: 300 + 10*i, HasShape: true})
	}
	for _, r := range []rune{0x064E, 0x0651, 0xFC60} { // fatha, shadda, shadda with fatha
		g = append(g, fonttest.Glyph{Rune: r, Advance: 0, HasShape: true})
	}
	return g
}

// withoutRune is a fixture's glyphs with one character's glyph drawn as
// nothing the character map reaches: it stays in place, so the others keep
// their numbers.
func withoutRune(g []fonttest.Glyph, r rune) []fonttest.Glyph {
	for i := range g {
		if g[i].Rune == r {
			g[i].Rune = 0xE000
		}
	}
	return g
}

// The fixture's glyphs, by what they are.
const (
	afBeh = 1 + iota
	afBehIsol
	afBehFina
	afBehInit
	afBehMedi
	afLam
	afLamIsol
	afLamFina
	afLamInit
	afLamMedi
	afAlef
	afAlefIsol
	afAlefFina
	afLamAlefIsol
	afLamAlefFina
	afHeh
	afHehIsol
	afHehFina
	afHehInit
	afHehMedi
	afLellah
	afFatha
	afShadda
	afShaddaFatha
)

// arabicFallbackFixtures are the face with nothing but a character map, and
// the same face with a GSUB that declares 'init' and names no lookup for it —
// which is enough for HarfBuzz to take the font at its word and make none of
// the forms.
func arabicFallbackFixtures() map[string][]byte {
	return map[string][]byte{
		"cmap-only": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicCmapOnly", Glyphs: arabicFallbackGlyphs()}),
		// The fallback's place among the font's own rules: its 'rlig' joins a
		// beh and a heh as letters, before the fallback makes forms of them,
		// and its 'calt' replaces the initial beh the fallback made.
		"with-rules": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWithRules", Glyphs: arabicFallbackGlyphs(),
			Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
				{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
					{Components: []int{afBeh, afHeh}, Glyph: afLellah}})}},
				{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{afBehInit}, []int{afLamIsol})}},
			}, map[string][]int{"rlig": {0}, "calt": {1}})}}),
		// No final alef, so no ligature has one as a part: the lam-alef the
		// face maps is not made, even where something the face does not map
		// — a .notdef — stands where the alef would be.
		"no-final-alef": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicNoFinalAlef",
			Glyphs: withoutRune(arabicFallbackGlyphs(), 0xFE8E)}),
		"declares-init": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicDeclaresInit", Glyphs: arabicFallbackGlyphs(),
			Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups(nil, map[string][]int{"init": {}})}}),
	}
}

// TestArabicFallsBackToPresentationForms: a face with no joining forms of its
// own draws them out of its character map, as HarfBuzz does — the four forms,
// the lam-alef and three-part ligatures under 'rlig' stepping over marks, and
// the shadda ligature — and a face that declares a form draws none. Every
// answer is HarfBuzz 14.5.0's for the same face, from the pinned uharfbuzz
// (TestWriteArabicFallbackFixtures writes the faces out to ask it).
func TestArabicFallsBackToPresentationForms(t *testing.T) {
	fonts := arabicFallbackFixtures()
	adv := func(gid int) float64 { return float64(300 + 10*(gid-1)) }
	g := func(gids ...int) []shapedAs {
		var out []shapedAs
		for _, gid := range gids {
			a := adv(gid)
			if gid >= afFatha {
				a = 0
			}
			out = append(out, shapedAs{gid, a, 0, 0})
		}
		return out
	}
	for _, c := range []struct {
		font, text string
		want       []shapedAs
	}{
		// Drawn left to right, so the last letter comes first.
		{"cmap-only", "\u0628\u0628\u0628", g(afBehFina, afBehMedi, afBehInit)},
		{"cmap-only", "\u0628", g(afBehIsol)},
		{"cmap-only", "\u0644\u0627", g(afLamAlefIsol)},
		{"cmap-only", "\u0628\u0644\u0627", g(afLamAlefFina, afBehInit)},
		{"cmap-only", "\u0644\u0644\u0647", g(afLellah)},
		// A heh that joins on is medial, so the ligature, which wants it
		// final, is not made.
		{"cmap-only", "\u0644\u0644\u0647\u0628", g(afBehFina, afHehMedi, afLamMedi, afLamInit)},
		// The forms and the lam-alef step over a mark, which the face places
		// by its ink, having no positioning of its own.
		{"cmap-only", "\u0628\u064E\u0628", []shapedAs{{afBehFina, adv(afBehFina), 0, 0},
			{afFatha, 0, -235, 862}, {afBehInit, adv(afBehInit), 0, 0}}},
		{"cmap-only", "\u0644\u064E\u0627", []shapedAs{{afFatha, 0, -77, 862},
			{afLamAlefIsol, adv(afLamAlefIsol), 0, 0}}},
		{"cmap-only", "\u0628\u0651\u064E", []shapedAs{{afShaddaFatha, 0, -245, 862},
			{afBehIsol, adv(afBehIsol), 0, 0}}},
		{"with-rules", "\u0628\u0647", g(afLellah)},
		{"with-rules", "\u0628\u0628", g(afBehFina, afLamIsol)},
		{"no-final-alef", "\u0628\u0644\u062C", []shapedAs{{0, 0, 0, 0},
			{afLamMedi, adv(afLamMedi), 0, 0}, {afBehInit, adv(afBehInit), 0, 0}}},
		{"declares-init", "\u0628\u0628\u0628", g(afBeh, afBeh, afBeh)},
		{"declares-init", "\u0644\u0627", g(afAlef, afLam)},
	} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatalf("%s: %v", c.font, err)
		}
		got, _ := f.ShapeGlyphs(c.text)
		checkShaped(t, fmt.Sprintf("%s %+q", c.font, c.text), got, c.want)
	}
}

// TestFormsDrawnFromTheCharacterMapFollowTheNeighbours: a face whose forms
// the fallback draws is one whose forms depend on the letters either side, and
// the questions a line breaker asks before it shapes a word apart from its
// neighbours say so. They read the plan's lookups alone, so for this face they
// said the forms could not change, and a word cut at a line's end kept the
// forms it had had whole.
func TestFormsDrawnFromTheCharacterMapFollowTheNeighbours(t *testing.T) {
	fonts := arabicFallbackFixtures()
	for _, c := range []struct {
		font string
		want bool
	}{{"cmap-only", true}, {"declares-init", false}} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatal(err)
		}
		if got := f.FormsFollowNeighbours("\u0628", Features{}); got != c.want {
			t.Errorf("%s: FormsFollowNeighbours = %v, want %v", c.font, got, c.want)
		}
		if got := f.ContextCanChange("\u0628", Features{}); got != c.want {
			t.Errorf("%s: ContextCanChange = %v, want %v", c.font, got, c.want)
		}
		if got := f.FormsFollowNeighbours("\u064E", Features{}); got != c.want {
			t.Errorf("%s: FormsFollowNeighbours of a mark = %v, want %v", c.font, got, c.want)
		}
		if got := f.HasJoiningForms(); got != c.want {
			t.Errorf("%s: HasJoiningForms = %v, want %v", c.font, got, c.want)
		}
	}
}

// TestTheSynthesizedCoverageIsHarfBuzzs: a coverage built here is in the
// format HarfBuzz's Coverage::serialize would write it in — a list where that
// is no more than three times the runs of consecutive glyphs, the runs
// otherwise — and a glyph given twice is kept twice. The format is what
// decides, through the search each is read by, which of two letters sharing a
// glyph a lookup answers for.
func TestTheSynthesizedCoverageIsHarfBuzzs(t *testing.T) {
	for _, c := range []struct {
		glyphs []int
		want   []int // the table, sixteen bits at a time
	}{
		{[]int{3, 9, 20}, []int{1, 3, 3, 9, 20}},
		{[]int{4, 4, 5}, []int{1, 3, 4, 4, 5}},
		{[]int{1, 2, 3, 7}, []int{1, 4, 1, 2, 3, 7}},
		{[]int{1, 2, 3, 4, 5, 6, 7, 20}, []int{2, 2, 1, 7, 0, 20, 20, 7}},
		{[]int{5, 6, 7, 8, 8, 9, 10, 11}, []int{2, 2, 5, 8, 0, 8, 11, 4}},
	} {
		got := serializeCoverage(c.glyphs)
		var words []int
		for i := 0; i+1 < len(got); i += 2 {
			words = append(words, int(got[i])<<8|int(got[i+1]))
		}
		if fmt.Sprint(words) != fmt.Sprint(c.want) {
			t.Errorf("coverage of %v = %v, want %v", c.glyphs, words, c.want)
		}
		// A coverage is read at an offset from the subtable naming it.
		sub := append([]byte{0, 0}, got...)
		for i, g := range c.glyphs {
			if at, ok := coverageIndex(sub, 2, g); !ok || c.glyphs[at] != g {
				t.Errorf("coverage of %v: glyph %d (at %d) reads as index %d, %v", c.glyphs, g, i, at, ok)
			}
		}
	}
}

// TestWriteArabicFallbackFixtures writes the fixtures out for asking HarfBuzz,
// where FORME_ARABIC_FIXTURES names a directory; it does nothing otherwise.
func TestWriteArabicFallbackFixtures(t *testing.T) {
	dir := os.Getenv("FORME_ARABIC_FIXTURES")
	if dir == "" {
		t.Skip("FORME_ARABIC_FIXTURES is not set")
	}
	for name, data := range arabicFallbackFixtures() {
		if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
