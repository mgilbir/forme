package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// cmapStateFont is a two-glyph TrueType font — .notdef, then glyphs 1 and 2
// drawn for A and B — whose cmap is replaced by the given subtables, so that
// the only thing that differs between the cases below is the character map.
func cmapStateFont(subs ...fonttest.CmapSub) []byte {
	cmap := SFNTTables(fonttest.SFNTWithCmapSubtables(subs))["cmap"]
	return fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 1000, HasShape: true},
			{Rune: 'B', Advance: 1000, HasShape: true},
		},
		Extra: map[string][]byte{"cmap": cmap},
	})
}

// ghostSegments is WPT's fonts/ahem-visible-zwnj.otf's format-4 subtable,
// segment for segment: A–B and U+200C, through deltas that land on glyphs 35
// and 36 of a font whose maxp declares three.
var ghostSegments = [][3]int{
	{0x41, 0x42, -30},
	{0x200C, 0x200C, -8169},
	{0xFFFF, 0xFFFF, 1},
}

// unreadable is a subtable parseCmapSubtable answers nil for: format 2, which
// it does not read. The bytes after the format number do not matter.
func unreadable() []byte {
	b := fonttest.CmapFormat4([][3]int{{0x41, 0x42, -64}})
	b[1] = 2
	return b
}

// TestAnEmptyCmapSaysWhyItIsEmpty is the three fonts a nil Cmap used to stand
// for alike — no Unicode map, a map this cannot read, and a map that names only
// glyphs the font has not got — and the one it is not.
//
// The refusal of all three is shape.Load's and is the same; what is tested here
// is that the reader keeps the difference, so the refusal can say which it is.
func TestAnEmptyCmapSaysWhyItIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		what string
		subs []fonttest.CmapSub
		want CmapState
	}{
		{
			// The control: the same segments, with deltas that land on the
			// glyphs the font has. Without it the case below could pass for a
			// fixture that never read.
			what: "a map naming glyphs the font has",
			subs: []fonttest.CmapSub{
				{Plat: 0, Enc: 3, Data: fonttest.CmapFormat4([][3]int{{0x41, 0x42, -64}, {0xFFFF, 0xFFFF, 1}})},
				{Plat: 3, Enc: 1, Data: fonttest.CmapFormat4([][3]int{{0x41, 0x42, -64}, {0xFFFF, 0xFFFF, 1}})},
			},
			want: CmapRead,
		},
		{
			what: "ahem-visible-zwnj.otf: a map naming only glyphs past maxp",
			subs: []fonttest.CmapSub{
				{Plat: 0, Enc: 3, Data: fonttest.CmapFormat4(ghostSegments)},
				{Plat: 3, Enc: 1, Data: fonttest.CmapFormat4(ghostSegments)},
			},
			want: CmapNamesNoGlyph,
		},
		{
			// The best-ranked subtable unreadable and the other one ghosts:
			// what the font's readable map holds is the thing to say.
			what: "an unreadable (3,1) beside a (0,3) naming only glyphs past maxp",
			subs: []fonttest.CmapSub{
				{Plat: 0, Enc: 3, Data: fonttest.CmapFormat4(ghostSegments)},
				{Plat: 3, Enc: 1, Data: unreadable()},
			},
			want: CmapNamesNoGlyph,
		},
		{
			what: "a Unicode subtable in a format this does not read",
			subs: []fonttest.CmapSub{{Plat: 3, Enc: 1, Data: unreadable()}},
			want: CmapUnreadable,
		},
		{
			// Read, but every code lands on .notdef, which is no mapping.
			what: "a Unicode subtable mapping nothing but .notdef",
			subs: []fonttest.CmapSub{
				{Plat: 3, Enc: 1, Data: fonttest.CmapFormat4([][3]int{{0x41, 0x41, -0x41}, {0xFFFF, 0xFFFF, 1}})},
			},
			want: CmapUnreadable,
		},
		{
			what: "only a (3,0) symbol subtable",
			subs: []fonttest.CmapSub{
				{Plat: 3, Enc: 0, Data: fonttest.CmapFormat4([][3]int{{0xF041, 0xF042, 0x10000 - 0xF040}, {0xFFFF, 0xFFFF, 1}})},
			},
			want: CmapAbsent,
		},
	} {
		fp := ParseSFNT(cmapStateFont(tc.subs...), 1<<20)
		if fp == nil {
			t.Fatalf("%s: the fixture did not parse", tc.what)
		}
		if fp.NumGlyphs != 3 {
			t.Fatalf("%s: the fixture declares %d glyphs, want 3", tc.what, fp.NumGlyphs)
		}
		if fp.CmapPartial {
			t.Fatalf("%s: the budget stopped the cmap walk, so the case is not the one meant", tc.what)
		}
		if fp.CmapState != tc.want {
			t.Errorf("%s: CmapState is %d, want %d", tc.what, fp.CmapState, tc.want)
		}
		if got := len(fp.Cmap) > 0; got != (tc.want == CmapRead) {
			t.Errorf("%s: Cmap holds %d mappings, and the state is %d", tc.what, len(fp.Cmap), tc.want)
		}
	}
}
