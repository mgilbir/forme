package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A cmap entry naming a glyph the font has not got is not a mapping.
//
// There is no such glyph: it has no outline, no advance, and nothing to put in a
// subset — so a character "mapped" to one is a character the font cannot set,
// which is what being unmapped means. Keeping the entry says the opposite to
// everything downstream and every one of them believes it: the shaper records
// the index as used, the width table answers nought, the subsetter cannot keep
// it, and the page carries a code the embedded program has no glyph for, with
// nothing reported at any step because the font appeared to have said so.
func TestACmapEntryPastTheGlyphCountIsNotAMapping(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 600, HasShape: true},
			{Rune: 'c', Advance: 700, HasShape: true},
		},
	})

	// The font as written maps all three, which is the control: without it the
	// assertion below would hold of a font whose cmap was never read.
	whole := ParseSFNT(data, 1<<20)
	if whole == nil {
		t.Fatal("the fixture font did not parse")
	}
	for _, r := range "abc" {
		if gid, ok := whole.Cmap[r]; !ok || gid <= 0 {
			t.Fatalf("the fixture does not map %q at all (%d, %v)", r, gid, ok)
		}
	}
	if whole.NumGlyphs < 4 {
		t.Fatalf("the fixture declares %d glyphs, so lowering the count below "+
			"its mappings is not possible", whole.NumGlyphs)
	}

	// Now the same bytes with maxp saying the font has two glyphs — .notdef and
	// one more — which leaves the mappings for the other two naming glyphs that
	// are not there.
	tables := SFNTTables(data)
	maxp := tables["maxp"]
	if len(maxp) < 6 {
		t.Fatal("the fixture has no maxp to lower")
	}
	maxp[4], maxp[5] = 0, 2

	cut := ParseSFNT(data, 1<<20)
	if cut == nil {
		t.Fatal("the font stopped parsing when its glyph count was lowered")
	}
	if cut.NumGlyphs != 2 {
		t.Fatalf("the glyph count is %d, so the fixture did not take", cut.NumGlyphs)
	}
	for r, gid := range cut.Cmap {
		if gid >= cut.NumGlyphs || gid < 0 {
			t.Errorf("%q is still mapped to glyph %d of a font declaring %d",
				r, gid, cut.NumGlyphs)
		}
	}
	// And the one mapping that is still inside the font is still there, so the
	// filter drops what it should and nothing else.
	kept := 0
	for _, gid := range cut.Cmap {
		if gid > 0 && gid < cut.NumGlyphs {
			kept++
		}
	}
	if kept == 0 {
		t.Error("every mapping was dropped, including the one still inside the font")
	}
}

// TestAFontWithNoGlyphCountKeepsItsCmap is the case the filter must not touch.
//
// A font whose maxp this reader cannot take apart declares no count rather than
// a count of none, and filtering against nought would empty every cmap in it —
// turning a font this engine can only partly read into one it reads as having no
// characters at all.
func TestAFontWithNoGlyphCountKeepsItsCmap(t *testing.T) {
	m := map[rune]int{'a': 1, 'b': 9999}
	got := onlyDeclaredGlyphs(m, 0)
	if len(got) != 2 {
		t.Errorf("a font declaring no glyph count kept %d of its 2 mappings", len(got))
	}
}

// TestEveryMappingPastTheCountGoes pins the rule itself, away from any font.
func TestEveryMappingPastTheCountGoes(t *testing.T) {
	for _, tc := range []struct {
		in   map[rune]int
		n    int
		want map[rune]int
		what string
	}{
		{map[rune]int{'a': 0, 'b': 1}, 2, map[rune]int{'a': 0, 'b': 1}, "all inside"},
		{map[rune]int{'a': 1, 'b': 2}, 2, map[rune]int{'a': 1}, "one past the end"},
		{map[rune]int{'a': -1, 'b': 1}, 2, map[rune]int{'b': 1}, "a negative index"},
		{map[rune]int{'a': 5, 'b': 6}, 2, nil, "nothing left, which is no cmap"},
	} {
		got := onlyDeclaredGlyphs(tc.in, tc.n)
		if len(got) != len(tc.want) {
			t.Errorf("%s: kept %v, want %v", tc.what, got, tc.want)
			continue
		}
		for r, gid := range tc.want {
			if got[r] != gid {
				t.Errorf("%s: kept %v, want %v", tc.what, got, tc.want)
				break
			}
		}
	}
}
