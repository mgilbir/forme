package shape

import (
	"os"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A renumbered subset whose glyphs map no character.
//
// A CID-keyed CFF is renumbered when it is subsetted, and its character map is
// written again for the glyphs kept (cffrenumber.go). Where none of them has a
// character — a page drawn in only .notdef, or only in forms reached through
// the font's rules, a vertical form or a ligature — the map it gets maps
// nothing, because there is nothing true for it to map. Load refuses such a
// program, as it refuses any face that can set no text.
//
// The subset is not wrong for it. It is the program a document embeds, and a
// CIDFontType0 is addressed by CID, never through its character map, so it is
// embedded as it is. FreeType, HarfBuzz and fontTools all read it. So Subset
// does not refuse it, and nothing invents a mapping to make Load take it.
// What is held instead is what the subset is for.

// subsetFaces are faces whose subsets are renumbered: the CFF2 fixture, which
// is embedded as a CID-keyed CFF, and a CID-keyed CFF from the CJK corpus
// where it is fetched.
func subsetFaces(t *testing.T) map[string]*Face {
	t.Helper()
	out := map[string]*Face{}
	data, err := os.ReadFile("../testdata/harfbuzz/fonts/CFF2Blend.otf")
	if err != nil {
		t.Fatal(err)
	}
	if out["CFF2Blend.otf"], err = Load(data); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("NOTO_CJK") != "" {
		if out["NotoSansJP-Regular.otf"], err = Load(fonttest.CJKFile(t, "NotoSansJP-Regular.otf")); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestASubsetWhoseGlyphsMapNoCharacterIsStillItsFont subsets each face for
// nothing but .notdef, and for .notdef and a glyph no character maps.
func TestASubsetWhoseGlyphsMapNoCharacterIsStillItsFont(t *testing.T) {
	for name, f := range subsetFaces(t) {
		mapped := map[int]bool{}
		for _, gid := range f.Cmap() {
			mapped[gid] = true
		}
		unmapped := -1
		for gid := 1; gid < f.NumGlyphs() && unmapped < 0; gid++ {
			if !mapped[gid] {
				unmapped = gid
			}
		}
		uses := [][]int{nil}
		if unmapped > 0 {
			uses = append(uses, []int{unmapped})
		}
		for _, use := range uses {
			g := f.Clone()
			for _, gid := range use {
				g.used[gid] = true
			}
			prog, kept, err := g.SubsetGlyphs()
			if err != nil {
				t.Fatalf("%s using %v: a subset for glyphs no character maps was refused: %v", name, use, err)
			}
			if !keptMapNoCharacter(f, kept) {
				t.Fatalf("%s: glyphs %v are kept, and one of them is mapped", name, kept)
			}
			if _, err := Load(prog); err == nil || !strings.Contains(err.Error(), "character map") {
				t.Errorf("%s using %v: Load answers %v, and the subset maps no character", name, use, err)
			}
			// What checkSubset asks of it, which is what FuzzLoadAndUse asks
			// of every subset.
			checkSubset(t, g)
		}
	}
}

// TestASubsetWithAMappedGlyphLoads is the other side: once a kept glyph has a
// character, the subset maps it and Load reads it back, and keptMapNoCharacter
// says so — so the allowance above is never taken for a subset that lost a
// mapping it should have kept.
func TestASubsetWithAMappedGlyphLoads(t *testing.T) {
	for name, f := range subsetFaces(t) {
		r, gid := rune(-1), -1
		for c, g := range f.Cmap() {
			if g > 0 && (r < 0 || c < r) {
				r, gid = c, g
			}
		}
		g := f.Clone()
		g.used[gid] = true
		prog, kept, err := g.SubsetGlyphs()
		if err != nil {
			t.Fatal(err)
		}
		if keptMapNoCharacter(f, kept) {
			t.Fatalf("%s: glyph %d, which %U maps to, is kept and read as unmapped", name, gid, r)
		}
		sub, err := Load(prog)
		if err != nil {
			t.Fatalf("%s: the subset keeping %U cannot be read back: %v", name, r, err)
		}
		if got, ok := sub.GlyphID(r); !ok || got == 0 {
			t.Errorf("%s: the subset does not map %U", name, r)
		}
		checkSubset(t, g)
	}
}
