package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The glyphs of the stand-in fixtures, by index: glyph 0 is .notdef and the
// rest follow the order spaceFallbackFixtures gives them in.
const (
	sfSpace  = 1
	sfOne    = 2
	sfStop   = 3
	sfA      = 4
	sfHyphen = 5
	sfB      = 6
)

// spaceFallbackFixtures are four faces with no glyph for any space separator
// but U+0020, or not even that.
//
// "em2048" has 2048 units to the em, so that every fraction of one has to be
// rounded, a digit and a full stop for the figure and punctuation spaces to
// take their widths from, a hyphen for the non-breaking one, and a kern
// between its space and a 'b' — which is applied to the width the separator
// was given, not to the space's own. "bare" has a space and a letter and
// nothing else, so the figure and punctuation spaces keep the space's width
// and U+2011 has nothing to stand in for it. "substituted" puts its space
// through a multiple substitution and a ligature. "nospace" has no space at
// all.
func spaceFallbackFixtures() map[string][]byte {
	em2048 := []fonttest.Glyph{
		{Rune: ' ', Advance: 532},
		{Rune: '1', Advance: 1139, HasShape: true},
		{Rune: '.', Advance: 569, HasShape: true},
		{Rune: 'a', Advance: 1000, HasShape: true},
		{Rune: 0x2010, Advance: 682, HasShape: true},
		{Rune: 'b', Advance: 1100, HasShape: true},
	}
	return map[string][]byte{
		"em2048": fonttest.SFNT(fonttest.SFNTOptions{Name: "StandIn", UnitsPerEm: 2048, Glyphs: em2048,
			Extra: map[string][]byte{
				"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable(
					[]fonttest.KernPair{{Left: sfSpace, Right: sfB, Adjust: -100}})}}},
					map[string][]int{"kern": {0}}),
			}}),
		"bare": fonttest.SFNT(fonttest.SFNTOptions{Name: "StandInBare", Glyphs: []fonttest.Glyph{
			{Rune: ' ', Advance: 250},
			{Rune: 'a', Advance: 500, HasShape: true},
		}}),
		// The substitutions a stand-in goes through: 'ccmp' takes the space
		// apart into two, and 'liga' joins a 'b' and a space into an 'a'.
		"substituted": fonttest.SFNT(fonttest.SFNTOptions{Name: "StandInSubstituted", Glyphs: []fonttest.Glyph{
			{Rune: ' ', Advance: 250},
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 600, HasShape: true},
		}, Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
				{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{1}, [][]int{{1, 1}})}},
				{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{3, 1}, Glyph: 2}})}},
			}, map[string][]int{"ccmp": {0}, "liga": {1}}),
		}}),
		"nospace": fonttest.SFNT(fonttest.SFNTOptions{Name: "StandInNone", Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
		}}),
	}
}

// TestASpaceTheFaceLacksIsItsSpace: a space separator the face has no glyph
// for is drawn with its U+0020 at the separator's own width, and U+2011 with
// its U+2010, as HarfBuzz draws them; and neither is counted missing. Every
// answer is HarfBuzz 14.5.0's for the same face, from the pinned uharfbuzz
// (TestWriteSpaceFallbackFixtures writes the faces out to ask it).
//
// They were .notdef, counted missing, so layout went to another face for a
// character HarfBuzz — and every browser shaping with it — sets in this one.
func TestASpaceTheFaceLacksIsItsSpace(t *testing.T) {
	fonts := spaceFallbackFixtures()
	// 2048 units to the em, so a thousandth of an em is 2.048 of them.
	u := func(v int) float64 { return float64(v) * 1000 / 2048 }
	for _, c := range []struct {
		font, text string
		want       []shapedAs
		missing    int
	}{
		{"em2048", "\u3000", []shapedAs{{sfSpace, u(2048), 0, 0}}, 0},
		{"em2048", "\u2001", []shapedAs{{sfSpace, u(2048), 0, 0}}, 0},
		{"em2048", "\u2003", []shapedAs{{sfSpace, u(2048), 0, 0}}, 0},
		{"em2048", "\u2000", []shapedAs{{sfSpace, u(1024), 0, 0}}, 0},
		{"em2048", "\u2002", []shapedAs{{sfSpace, u(1024), 0, 0}}, 0},
		{"em2048", "\u2004", []shapedAs{{sfSpace, u(683), 0, 0}}, 0},
		{"em2048", "\u2005", []shapedAs{{sfSpace, u(512), 0, 0}}, 0},
		{"em2048", "\u2006", []shapedAs{{sfSpace, u(341), 0, 0}}, 0},
		{"em2048", "\u2009", []shapedAs{{sfSpace, u(410), 0, 0}}, 0},
		{"em2048", "\u200A", []shapedAs{{sfSpace, u(128), 0, 0}}, 0},
		{"em2048", "\u205F", []shapedAs{{sfSpace, u(455), 0, 0}}, 0},
		{"em2048", "\u202F", []shapedAs{{sfSpace, u(266), 0, 0}}, 0},
		{"em2048", "\u2007", []shapedAs{{sfSpace, u(1139), 0, 0}}, 0},
		{"em2048", "\u2008", []shapedAs{{sfSpace, u(569), 0, 0}}, 0},
		{"em2048", "\u00A0", []shapedAs{{sfSpace, u(532), 0, 0}}, 0},
		{"em2048", "\u2011", []shapedAs{{sfHyphen, u(682), 0, 0}}, 0},
		// The kern is applied to the width the separator was given.
		{"em2048", "a\u2003b", []shapedAs{{sfA, u(1000), 0, 0}, {sfSpace, u(1948), 0, 0}, {sfB, u(1100), 0, 0}}, 0},
		{"em2048", "a b", []shapedAs{{sfA, u(1000), 0, 0}, {sfSpace, u(432), 0, 0}, {sfB, u(1100), 0, 0}}, 0},
		// U+1680 is a space separator HarfBuzz does not stand in for.
		{"em2048", "\u1680", []shapedAs{{0, 0, 0, 0}}, 1},
		{"bare", "\u2007", []shapedAs{{sfSpace, 250, 0, 0}}, 0},
		{"bare", "\u2008", []shapedAs{{sfSpace, 250, 0, 0}}, 0},
		{"bare", "\u202F", []shapedAs{{sfSpace, 125, 0, 0}}, 0},
		{"bare", "\u3000", []shapedAs{{sfSpace, 1000, 0, 0}}, 0},
		{"bare", "\u2011", []shapedAs{{0, 0, 0, 0}}, 1},
		// Each part of a stand-in taken apart is still the separator, and a
		// ligature made with one is not: its advance is its own.
		{"substituted", "\u3000", []shapedAs{{1, 1000, 0, 0}, {1, 1000, 0, 0}}, 0},
		{"substituted", "\u2002", []shapedAs{{1, 500, 0, 0}, {1, 500, 0, 0}}, 0},
		{"substituted", "b\u3000", []shapedAs{{2, 500, 0, 0}, {1, 1000, 0, 0}}, 0},
		{"substituted", "b ", []shapedAs{{2, 500, 0, 0}, {1, 250, 0, 0}}, 0},
		{"nospace", "\u3000", []shapedAs{{0, 0, 0, 0}}, 1},
		{"nospace", "\u00A0", []shapedAs{{0, 0, 0, 0}}, 1},
	} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatalf("%s: %v", c.font, err)
		}
		got, missing := f.ShapeGlyphs(c.text)
		what := fmt.Sprintf("%s %+q", c.font, c.text)
		checkShaped(t, what, got, c.want)
		if missing != c.missing {
			t.Errorf("%s: %d reported missing, want %d", what, missing, c.missing)
		}
	}
}

// TestAStandInIsMeasuredAndEncodedAsItIsDrawn: Measure, Encode and a Stack's
// choice of face give the answer the shaper gives, which is what they are
// for: a text measured at one width and drawn at another, or written with a
// .notdef the page does not show, is a disagreement nothing would report.
func TestAStandInIsMeasuredAndEncodedAsItIsDrawn(t *testing.T) {
	fonts := spaceFallbackFixtures()
	f, err := Load(fonts["em2048"])
	if err != nil {
		t.Fatal(err)
	}
	const text = "a\u3000\u2011\u2009"
	shaped, _ := f.ShapeGlyphs(text)
	if got, want := f.Measure(text, 1000), MeasureGlyphs(shaped, 1000); got != want {
		t.Errorf("Measure(%+q) = %v, and it is shaped %v wide", text, got, want)
	}
	codes, missing := f.Encode(text)
	want := []byte{0, sfA, 0, sfSpace, 0, sfHyphen, 0, sfSpace}
	if string(codes) != string(want) || missing != 0 {
		t.Errorf("Encode(%+q) = % x with %d missing, want % x with none", text, codes, missing, want)
	}

	// A second face that has the ideographic space is not reached for it:
	// the first sets it, as the shaper would.
	other := fonttest.SFNT(fonttest.SFNTOptions{Name: "HasIdeographicSpace", Glyphs: []fonttest.Glyph{
		{Rune: 0x3000, Advance: 1000},
	}})
	g, err := Load(other)
	if err != nil {
		t.Fatal(err)
	}
	runs, missing := NewStack(f, g).ShapeRuns("a\u3000a")
	if len(runs) != 1 || runs[0].Face != f || missing != 0 {
		t.Errorf("a stack of the face and one with U+3000 set %d runs (%d missing), want one in the first face",
			len(runs), missing)
	}
}

// TestAStandInIsNotCoverage: StandsIn tells a character a face has from one
// it would only draw with a stand-in, which is what a fallback library needs to
// choose by what its faces have.
func TestAStandInIsNotCoverage(t *testing.T) {
	fonts := spaceFallbackFixtures()
	for _, c := range []struct {
		font string
		r    rune
		want bool
	}{
		{"em2048", 0x3000, true},
		{"em2048", 0x2011, true},
		{"em2048", 0x2000, true},
		{"em2048", ' ', false},
		{"em2048", 'a', false},
		{"em2048", 0x2010, false},
		{"em2048", 0x1680, false},
		{"bare", 0x2011, false},
		{"nospace", 0x3000, false},
	} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatal(err)
		}
		if got := f.StandsIn(c.r); got != c.want {
			t.Errorf("%s: StandsIn(U+%04X) = %v, want %v", c.font, c.r, got, c.want)
		}
	}
}

// TestWriteSpaceFallbackFixtures writes the fixtures out for asking HarfBuzz,
// where FORME_SPACE_FIXTURES names a directory; it does nothing otherwise.
func TestWriteSpaceFallbackFixtures(t *testing.T) {
	dir := os.Getenv("FORME_SPACE_FIXTURES")
	if dir == "" {
		t.Skip("FORME_SPACE_FIXTURES is not set")
	}
	for name, data := range spaceFallbackFixtures() {
		if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
