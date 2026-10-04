package shape

import (
	"slices"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A substitution may name a glyph the font does not have: here a multiple
// substitution of 'a' into itself and glyph 99, of a font of two. Shaping hands
// it back, as HarfBuzz does; Used does not list it, since no subset can keep it
// and /CIDSet cannot say the program has it. FuzzLoadAndUse found it through a
// morx insertion (testdata/fuzz/FuzzLoadAndUse/c09716e5cdd298e5), whose
// subset was then held to a glyph past the font's end.
func TestUsedListsOnlyGlyphsTheFontHas(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{Type: 2, Subtables: [][]byte{
				fonttest.MultipleSubst([]int{1}, [][]int{{1, 99}}),
			}}}, map[string][]int{"ccmp": {0}}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs("a")
	var ids []int
	for _, g := range glyphs {
		ids = append(ids, g.GID)
	}
	if !slices.Equal(ids, []int{1, 99}) {
		t.Fatalf("shaped as %v, want [1 99]: the test no longer names a glyph past the end", ids)
	}
	if used := f.Used(); !slices.Equal(used, []int{1}) {
		t.Errorf("Used is %v, want [1]", used)
	}
	if _, kept, err := f.SubsetGlyphs(); err != nil || !slices.Contains(kept, 1) {
		t.Errorf("subset kept %v: %v", kept, err)
	}
}
