package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The fixture faces of the model tests in syriac_test.go, hebrew_test.go and
// usesubstituted_test.go: each a face that states the features one of
// HarfBuzz's script models turns on, and nothing else. Every answer those
// tests hold this package to is HarfBuzz 14.5.0's for the same face, from the
// pinned uharfbuzz; TestWriteModelFixtures writes them out to ask it.

// glyphsFor makes one glyph per rune, glyph i+1 for rune i, each with the
// advance given, and ink of its own.
func glyphsFor(runes []rune, advances []int) []fonttest.Glyph {
	g := make([]fonttest.Glyph, len(runes))
	for i, r := range runes {
		g[i] = fonttest.Glyph{Rune: r, Advance: advances[i], HasShape: true}
	}
	return g
}

func modelFixtures() map[string][]byte {
	out := map[string][]byte{}

	// Syriac: an Alaph, a Beth and a Dalath, and the forms of each the
	// joining model can choose, under 'syrc'.
	out["syriac-alaph"] = fonttest.SFNT(fonttest.SFNTOptions{Name: "SyriacAlaph",
		Glyphs: glyphsFor(
			[]rune{0x0710, 0x0712, 0x0715, 0xE100, 0xE101, 0xE102, 0xE103, 0xE104, 0xE105, 0xE106, 0xE107, 0x0654, 0x0742},
			[]int{400, 500, 450, 410, 420, 430, 440, 510, 520, 530, 460, 0, 0}),
		Extra: map[string][]byte{"GSUB": fonttest.GSUBFormsIn(map[string][2][]int{
			"fina": {{1, 2, 3}, {4, 10, 11}},
			"fin2": {{1}, {5}},
			"fin3": {{1}, {6}},
			"med2": {{1}, {7}},
			"init": {{2}, {8}},
			"medi": {{2}, {9}},
		}, map[string]fonttest.Script{"syrc": fonttest.AllFeatures(6)})}})

	// Syriac's abbreviation mark, which 'stch' takes apart into a fixed piece,
	// a repeating one and another fixed one; a space; a Gamal that 'ccmp' takes
	// apart, and a Heth two of which 'rlig' joins, so that the word a stretch
	// spans is made of glyphs substitutions made.
	out["syriac-stch"] = fonttest.SFNT(fonttest.SFNTOptions{Name: "SyriacStch",
		Glyphs: glyphsFor(
			[]rune{0x0712, 0x070F, 0xE200, 0xE201, 0xE202, ' ', 0x0713, 0xE203, 0x0717, 0xE205},
			[]int{500, 300, 100, 40, 120, 250, 520, 260, 540, 700}),
		Extra: map[string][]byte{"GSUB": fonttest.GSUBTable(
			[]fonttest.Lookup{
				{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{2}, [][]int{{3, 4, 5}})}},
				{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{7}, [][]int{{8, 8}})}},
				{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{9, 9}, Glyph: 10}})}},
			},
			[]fonttest.Feature{{Tag: "stch", Lookups: []int{0}}, {Tag: "ccmp", Lookups: []int{1}}, {Tag: "rlig", Lookups: []int{2}}},
			map[string]fonttest.Script{"syrc": fonttest.AllFeatures(3)})}})

	// Hebrew: letters, points and the presentation forms of some pairs, in a
	// face that positions no marks and in one that declares 'mark'.
	hebrew := glyphsFor(
		[]rune{0x05D0, 0x05B7, 0xFB2E, 0x05D1, 0x05BC, 0xFB31, 0x05B8, 0x05B0, 0x05BD, 0x05E9, 0x05C1, 0xFB49, 0xFB2C, 0x05B4},
		[]int{600, 0, 610, 620, 0, 630, 0, 0, 0, 700, 0, 710, 720, 0})
	out["hebrew"] = fonttest.SFNT(fonttest.SFNTOptions{Name: "HebrewForms", Glyphs: hebrew})
	out["hebrew-mark"] = fonttest.SFNT(fonttest.SFNTOptions{Name: "HebrewMark", Glyphs: hebrew,
		Extra: map[string][]byte{"GPOS": fonttest.GPOSTable(nil,
			[]fonttest.Feature{{Tag: "mark"}},
			map[string]fonttest.Script{"hebr": fonttest.AllFeatures(1)})}})

	// Newa: two letters, a vowel sign, the halant, a ra, a nukta, and what
	// 'pref' and 'rphf' make. Each feature is contextual rules that match at a
	// letter and substitute the glyph after it.
	newa := glyphsFor(
		[]rune{0x1140E, 0x11431, 0x11440, 0xE400, 0x11442, 0xE401, 0x1142B, 0x11446, 0xE402},
		[]int{600, 610, 300, 350, 0, 200, 620, 0, 90})
	out["newa"] = fonttest.SFNT(fonttest.SFNTOptions{Name: "NewaSubstituted", Glyphs: newa,
		Extra: map[string][]byte{"GSUB": fonttest.GSUBTable(
			[]fonttest.Lookup{
				{Type: 5, Subtables: [][]byte{fonttest.SequenceContext3([][]int{{2}, {3}}, []fonttest.SeqLookup{{At: 1, Lookup: 1}})}},
				{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{3}, []int{4})}},
				{Type: 5, Subtables: [][]byte{
					fonttest.SequenceContext3([][]int{{7}, {5}}, []fonttest.SeqLookup{{At: 1, Lookup: 3}}),
					fonttest.SequenceContext3([][]int{{1}, {8}}, []fonttest.SeqLookup{{At: 1, Lookup: 3}}),
				}},
				{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{5, 8}, []int{6, 9})}},
			},
			[]fonttest.Feature{{Tag: "pref", Lookups: []int{0}}, {Tag: "rphf", Lookups: []int{2}}},
			map[string]fonttest.Script{"newa": fonttest.AllFeatures(2)})}})
	return out
}

// TestWriteModelFixtures writes the fixtures out for asking HarfBuzz, where
// FORME_MODEL_FIXTURES names a directory; it does nothing otherwise.
func TestWriteModelFixtures(t *testing.T) {
	dir := os.Getenv("FORME_MODEL_FIXTURES")
	if dir == "" {
		t.Skip("FORME_MODEL_FIXTURES is not set")
	}
	for name, data := range modelFixtures() {
		if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// modelCase is one text and HarfBuzz's answer for it, in the oracle's own
// notation: glyph,advance[,dx,dy] per glyph, in the order they are drawn.
type modelCase struct{ text, want, why string }

// checkModelCases shapes each case in a fixture face and holds it to
// HarfBuzz's answer. The faces have 1000 units to the em, so the answer's
// units are this package's thousandths.
func checkModelCases(t *testing.T, font string, cases []modelCase) {
	t.Helper()
	f, err := Load(modelFixtures()[font])
	if err != nil {
		t.Fatalf("%s: %v", font, err)
	}
	for _, c := range cases {
		want, err := parseExpectedGlyphs(c.want)
		if err != nil {
			t.Fatalf("%s %+q: %v", font, c.text, err)
		}
		shaped := make([]shapedAs, len(want))
		for i, g := range want {
			shaped[i] = shapedAs{g.gid, float64(g.adv), float64(g.dx), float64(g.dy)}
		}
		got, _ := f.ShapeGlyphs(c.text)
		checkShaped(t, fmt.Sprintf("%s %+q: %s", font, c.text, c.why), got, shaped)
	}
}
