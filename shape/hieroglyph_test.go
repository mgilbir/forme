package shape

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The universal engine's Egyptian hieroglyph clusters: a sign, what mirrors
// and damages it, the brackets and segment controls around it, and the
// joiners that set the next sign beside, under, over or inside it. See the
// hieroglyph production in use.go.
//
// A cluster is what the engine holds the pre-processing features to — 'ccmp'
// among them is applied a cluster at a time — so a font that ligates every
// ordered pair of its characters under 'ccmp' says, in the glyphs it gives,
// which characters were put in one cluster. hieroglyphPairFace is that font, for the
// Egyptian characters of each category, two brackets, a Balinese letter and
// vowel sign, and the dotted circle.
var hieroglyphPairChars = []rune{
	0x13000, 0x13001, 0x1343C, // G: two signs and an enclosure
	0x13430, 0x13431, 0x13439, // J: vertical, horizontal, insert at middle
	0x13437, 0x13438, // SB, SE: begin and end segment
	0x13440, 0x13447, // HR, HM: mirror, damaged at top start
	'[', ']', // SB and SE: the brackets, which are Common
	0x1B13, 0x1B36, // Balinese ka and the vowel sign ulu
}

// hieroglyphPairFace's glyphs: .notdef, one per character of hieroglyphPairChars, the dotted
// circle, the non-joiner, and then the ligature of each ordered pair.
func hieroglyphPairFace(t *testing.T) *Face {
	t.Helper()
	var glyphs []fonttest.Glyph
	for i, r := range hieroglyphPairChars {
		glyphs = append(glyphs, fonttest.Glyph{Rune: r, Advance: 300 + 10*i, HasShape: true})
	}
	glyphs = append(glyphs, fonttest.Glyph{Rune: dottedCircle, Advance: 500, HasShape: true},
		fonttest.Glyph{Rune: 0x200C, Advance: 0})
	var ligs []fonttest.Ligature
	for a := range hieroglyphPairChars {
		for b := range hieroglyphPairChars {
			glyphs = append(glyphs, fonttest.Glyph{Rune: rune(0xF0000 + a*len(hieroglyphPairChars) + b),
				Advance: 700, HasShape: true})
			ligs = append(ligs, fonttest.Ligature{Components: []int{a + 1, b + 1}, Glyph: len(glyphs)})
		}
	}
	all := fonttest.AllFeatures(1)
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "Pairs",
		Glyphs: glyphs,
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(ligs)}}},
				[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}},
				map[string]fonttest.Script{"DFLT": all, "egyp": all, "bali": all}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestAQuadratIsOneCluster holds the clusters the engine cuts to HarfBuzz's.
// Each line is a string of hieroglyphPairFace's characters by name, and the
// glyphs HarfBuzz 14.5.0 gives for it, "a+b" being the ligature of a and b:
// a pair ligates only inside one cluster.
//
// Before, every Egyptian character was Other and a cluster of its own, so no
// pair of a quadrat ligated; and a closing bracket was Other too, so a vowel
// sign after one was taken onto it where HarfBuzz shows it against a dotted
// circle. A non-joiner no cluster had taken was passed over where HarfBuzz
// shows it against one as well.
func TestAQuadratIsOneCluster(t *testing.T) {
	f := hieroglyphPairFace(t)
	names := []string{"G1", "G2", "G3", "J1", "J2", "J3", "SB", "SE", "HR", "HM", "lbr", "rbr", "ka", "u"}
	index := map[string]int{}
	for i, n := range names {
		index[n] = i
	}
	n := len(hieroglyphPairChars)
	gid := func(name string) int {
		switch name {
		case "dc":
			return n + 1
		}
		if a, b, ok := strings.Cut(name, "+"); ok {
			return n + 3 + index[a]*n + index[b]
		}
		return index[name] + 1
	}
	for _, c := range []struct{ text, want string }{
		{"G1 J1 G2", "G1+J1 G2"},
		{"G1 G2", "G1 G2"},
		{"SB G1 SE", "SB+G1 SE"},
		{"G1 HR HM", "G1+HR HM"},
		{"J1 G1", "J1 G1"},
		{"lbr G1 rbr", "lbr+G1 rbr"},
		{"G1 J2 J3 G2", "G1+J2 J3+G2"},
		{"G1 J1 SB G2", "G1+J1 SB+G2"},
		{"G1 SE J1 G2", "G1+SE J1+G2"},
		{"HM G1", "HM G1"},
		{"G1 HM HR", "G1+HM HR"},
		{"ka rbr u", "ka rbr dc u"},
		{"ka lbr u", "ka lbr+u"},
		{"zwnj ka", "dc ka"},
		{"ka zwnj zwnj ka", "ka dc ka"},
		{"G1 zwnj G2", "G1 G2"},
		{"zwnj G1", "dc G1"},
		{"rbr u", "rbr dc u"},
	} {
		var text []rune
		for _, name := range strings.Fields(c.text) {
			if name == "zwnj" {
				text = append(text, 0x200C)
				continue
			}
			text = append(text, hieroglyphPairChars[index[name]])
		}
		var want []int
		for _, name := range strings.Fields(c.want) {
			want = append(want, gid(name))
		}
		glyphs, _ := f.ShapeGlyphs(string(text))
		var got []int
		for _, g := range glyphs {
			got = append(got, g.GID)
		}
		if !sameGIDs(got, want) {
			t.Errorf("%s: got glyphs %v, HarfBuzz gives %v (%s)", c.text, got, want, c.want)
		}
	}
}

// isolFace has an 'isol' form of a sign and of an opening bracket, under
// 'egyp'.
func isolFace(t *testing.T) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Isol",
		Glyphs: []fonttest.Glyph{
			{Rune: 0x13000, Advance: 500, HasShape: true},
			{Rune: '[', Advance: 300, HasShape: true},
			{Rune: 0xF0000, Advance: 600, HasShape: true},
			{Rune: 0xF0001, Advance: 400, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
					fonttest.SingleSubst([]int{1, 2}, []int{3, 4})}}},
				[]fonttest.Feature{{Tag: "isol", Lookups: []int{0}}},
				map[string]fonttest.Script{"DFLT": fonttest.AllFeatures(1), "egyp": fonttest.AllFeatures(1)}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestAQuadratTakesNoPositionalForm: the engine gives each cluster of a run
// that does not join cursively the positional form of its place among the
// clusters, and a quadrat none — HarfBuzz's setup_topographical_masks passes
// over a hieroglyph cluster as it passes over a character in no cluster, and
// what follows one starts afresh. So a sign alone is not isolated, and an
// opening bracket after one that opens no quadrat is: the bracket is a
// cluster of the ordinary kind. Each line is HarfBuzz 14.5.0's answer.
func TestAQuadratTakesNoPositionalForm(t *testing.T) {
	f := isolFace(t)
	for _, c := range []struct {
		text string
		want []int
	}{
		{"\U00013000", []int{1}},
		{"\U00013000[", []int{1, 4}},
		{"[\U00013000", []int{2, 1}},
		{"\U00013000[\U00013000", []int{1, 2, 1}},
	} {
		glyphs, _ := f.ShapeGlyphs(c.text)
		var got []int
		for _, g := range glyphs {
			got = append(got, g.GID)
		}
		if !sameGIDs(got, c.want) {
			t.Errorf("%+q is drawn as %v, HarfBuzz gives %v", c.text, got, c.want)
		}
	}
}
