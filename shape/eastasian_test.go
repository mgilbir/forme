package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// CSS Fonts 4 §6.9's font-variant-east-asian: which national standard's forms a
// run's ideographs take, whether its characters are set on the ideographic
// advance or their own, and whether its kana are the small forms an annotation
// is set in.
//
// A set like the figures beside it, and nine tags where §6.7 has eight. What is
// different about it is the reach: the width pair acts on Latin as well, because
// a Japanese font draws the ASCII letters twice — once proportionally and once
// on the em — and "full-width" asks for the second drawing.

// Glyph indices in eastAsianFace, in the order the glyphs are declared.
const (
	eaGidHan   = 1 + iota // 漢, what the six national forms cover
	eaGidA                // 'A', what 'fwid' covers
	eaGidWideA            // 'Ａ', what 'pwid' covers
	eaGidKana             // あ, what 'ruby' covers
	eaGidJis78
	eaGidJis83
	eaGidJis90
	eaGidJis04
	eaGidSimplified
	eaGidTraditional
	eaGidFullWidth
	eaGidProportional
	eaGidRuby
)

// eastAsianFace declares all nine of §6.9's features, each over a character of
// the class it really covers.
//
// Built rather than fetched, and for a reason the fetched faces cannot answer:
// Noto Sans JP declares six of the nine — 'jp78', 'jp83', 'jp90', 'fwid',
// 'pwid' and 'ruby' — and none of 'jp04', 'smpl' or 'trad'. A test written
// against it would report that three of the nine do nothing and could not tell
// that from their doing nothing because the engine forgot to ask.
func eastAsianFace(t *testing.T) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "EastAsian",
		Glyphs: []fonttest.Glyph{
			{Rune: '漢', Advance: 1000, HasShape: true},
			{Rune: 'A', Advance: 500, HasShape: true},
			{Rune: 'Ａ', Advance: 1000, HasShape: true},
			{Rune: 'あ', Advance: 1000, HasShape: true},
			{Rune: 0xE000, Advance: 1000, HasShape: true}, // 漢.jp78
			{Rune: 0xE001, Advance: 1000, HasShape: true}, // 漢.jp83
			{Rune: 0xE002, Advance: 1000, HasShape: true}, // 漢.jp90
			{Rune: 0xE003, Advance: 1000, HasShape: true}, // 漢.jp04
			{Rune: 0xE004, Advance: 1000, HasShape: true}, // 漢.smpl
			{Rune: 0xE005, Advance: 1000, HasShape: true}, // 漢.trad
			{Rune: 0xE006, Advance: 1000, HasShape: true}, // A.fwid
			{Rune: 0xE007, Advance: 500, HasShape: true},  // Ａ.pwid
			{Rune: 0xE008, Advance: 1000, HasShape: true}, // あ.ruby
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"jp78": {{eaGidHan}, {eaGidJis78}},
				"jp83": {{eaGidHan}, {eaGidJis83}},
				"jp90": {{eaGidHan}, {eaGidJis90}},
				"jp04": {{eaGidHan}, {eaGidJis04}},
				"smpl": {{eaGidHan}, {eaGidSimplified}},
				"trad": {{eaGidHan}, {eaGidTraditional}},
				"fwid": {{eaGidA}, {eaGidFullWidth}},
				"pwid": {{eaGidWideA}, {eaGidProportional}},
				"ruby": {{eaGidKana}, {eaGidRuby}},
			}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestEachEastAsianFeatureIsAskedForOnItsOwn.
func TestEachEastAsianFeatureIsAskedForOnItsOwn(t *testing.T) {
	f := eastAsianFace(t)
	const text = "漢AＡあ"
	plain := []int{eaGidHan, eaGidA, eaGidWideA, eaGidKana}
	for _, c := range []struct {
		what string
		e    EastAsian
		want []int
	}{
		{"normal", 0, plain},
		{"jis78", EastAsianJis78, []int{eaGidJis78, eaGidA, eaGidWideA, eaGidKana}},
		{"jis83", EastAsianJis83, []int{eaGidJis83, eaGidA, eaGidWideA, eaGidKana}},
		{"jis90", EastAsianJis90, []int{eaGidJis90, eaGidA, eaGidWideA, eaGidKana}},
		{"jis04", EastAsianJis04, []int{eaGidJis04, eaGidA, eaGidWideA, eaGidKana}},
		{"simplified", EastAsianSimplified,
			[]int{eaGidSimplified, eaGidA, eaGidWideA, eaGidKana}},
		{"traditional", EastAsianTraditional,
			[]int{eaGidTraditional, eaGidA, eaGidWideA, eaGidKana}},
		{"full-width", EastAsianFullWidth,
			[]int{eaGidHan, eaGidFullWidth, eaGidWideA, eaGidKana}},
		{"proportional-width", EastAsianProportionalWidth,
			[]int{eaGidHan, eaGidA, eaGidProportional, eaGidKana}},
		{"ruby", EastAsianRuby, []int{eaGidHan, eaGidA, eaGidWideA, eaGidRuby}},
		{"a national form and a width", EastAsianJis83 | EastAsianFullWidth,
			[]int{eaGidJis83, eaGidFullWidth, eaGidWideA, eaGidKana}},
		{"all three groups", EastAsianJis04 | EastAsianProportionalWidth | EastAsianRuby,
			[]int{eaGidJis04, eaGidA, eaGidProportional, eaGidRuby}},
	} {
		got, _ := f.ShapeGlyphsInContext(text, "", "", Features{EastAsian: c.e})
		if !equalInts(gids(got), c.want) {
			t.Errorf("%s set %q as %v, want %v", c.what, text, gids(got), c.want)
		}
	}
}

// TestEastAsianFeaturesReturnTheirTagsInOrder.
func TestEastAsianFeaturesReturnTheirTagsInOrder(t *testing.T) {
	for _, c := range []struct {
		e    EastAsian
		want []string
	}{
		{0, nil},
		{EastAsianJis78, []string{"jp78"}},
		{EastAsianRuby, []string{"ruby"}},
		{EastAsianTraditional | EastAsianFullWidth, []string{"trad", "fwid"}},
		{EastAsianJis04 | EastAsianProportionalWidth | EastAsianRuby,
			[]string{"jp04", "pwid", "ruby"}},
	} {
		if got := c.e.Features(); !equalStringSlices(got, c.want) {
			t.Errorf("EastAsian(%b).Features() = %v, want %v", c.e, got, c.want)
		}
	}
}

// TestTheEastAsianFormsOfARealFaceAreTheOnesHarfBuzzGives.
//
// The synthetic face above says the engine asks for the right tag; this says
// the tags mean what a real font means by them. Noto Sans JP declares six of
// the nine, and the numbers are HarfBuzz's over the fetched face:
//
//	骨辻  plain  14419 12867
//	骨辻  jp83   14419 16335     the second character is a different ideograph
//	骨辻  jp90   14419 16335     the same one — 1983 and 1990 agree here
//	骨辻  jp78   14419 12867     and 1978's form is the default
//	A1    plain  34 17461
//	A1    fwid   15356 15340     the Latin letters on the ideographic advance
//	Ａ    pwid   34               and back again
func TestTheEastAsianFormsOfARealFaceAreTheOnesHarfBuzzGives(t *testing.T) {
	f, err := Load(fonttest.NotoFile(t, "NotoSansJP-VF.ttf"))
	if err != nil {
		t.Fatalf("loading Noto Sans JP: %v", err)
	}
	for _, tag := range []string{"jp78", "jp83", "jp90", "fwid", "pwid", "ruby"} {
		if !offers(f, tag) {
			t.Fatalf("Noto Sans JP declares %v and not %s; the numbers here no "+
				"longer describe it", f.Features(), tag)
		}
	}
	for _, tag := range []string{"jp04", "smpl", "trad"} {
		if offers(f, tag) {
			t.Fatalf("Noto Sans JP now declares %s, so it is no longer the face "+
				"the synthetic fixture above exists for", tag)
		}
	}
	for _, c := range []struct {
		what, text string
		e          EastAsian
		want       []int
	}{
		{"plain", "骨辻", 0, []int{14419, 12867}},
		{"jis78", "骨辻", EastAsianJis78, []int{14419, 12867}},
		{"jis83", "骨辻", EastAsianJis83, []int{14419, 16335}},
		{"jis90", "骨辻", EastAsianJis90, []int{14419, 16335}},
		{"plain", "A1", 0, []int{34, 17461}},
		{"full-width", "A1", EastAsianFullWidth, []int{15356, 15340}},
		{"proportional-width", "Ａ", EastAsianProportionalWidth, []int{34}},
	} {
		got, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{EastAsian: c.e})
		if !equalInts(gids(got), c.want) {
			t.Errorf("%s set %q as %v, want HarfBuzz's %v",
				c.what, c.text, gids(got), c.want)
		}
	}
	// And the width the run takes follows the glyphs, which is the half of
	// "full-width" a reader sees: two Latin letters on the ideographic advance
	// are two ems, where the same two proportionally are barely one.
	plain, _ := f.ShapeGlyphsInContext("A1", "", "", Features{})
	wide, _ := f.ShapeGlyphsInContext("A1", "", "", Features{EastAsian: EastAsianFullWidth})
	if got, want := MeasureGlyphs(wide, 1000), 2000.0; got != want {
		t.Errorf("\"A1\" at full width measures %g, want %g — one em each", got, want)
	}
	if MeasureGlyphs(plain, 1000) >= MeasureGlyphs(wide, 1000) {
		t.Errorf("the proportional run measures %g and the full-width one %g",
			MeasureGlyphs(plain, 1000), MeasureGlyphs(wide, 1000))
	}
}
