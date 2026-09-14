package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// boundsFace builds a font of n glyphs, runes from 'A' upwards, with the given
// extra tables. A font with room for many distinct glyph pairs is what a test
// about how many of them a table may hold needs.
func boundsFace(t *testing.T, glyphs int, extra map[string][]byte) *Face {
	t.Helper()
	gs := make([]fonttest.Glyph, glyphs)
	for i := range gs {
		gs[i] = fonttest.Glyph{Rune: rune(0x41 + i), Advance: 500, HasShape: true}
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs, Extra: extra}))
	if err != nil {
		t.Fatalf("loading a font of %d glyphs: %v", glyphs, err)
	}
	return f
}

// TestALigatureTableBeyondTheBoundStopsBeingRead is the bound on how many
// ligatures one table may hold.
//
// Every ligature read is an entry kept for the life of the face, and a font
// states how many it has. Nothing had made one past the bound: every GSUB
// fixture here holds a handful, so the shape suite passes with maxLigatures
// raised.
//
// The ligature under test is the *last* one, which is what makes this discriminate.
// A test using an early ligature would pass whether the tail was read or not.
func TestALigatureTableBeyondTheBoundStopsBeingRead(t *testing.T) {
	const glyphs = 200
	// Distinct pairs, more of them than the bound allows. The last pair is the
	// one the text below is written in.
	var ligs []fonttest.Ligature
	for a := 1; a <= glyphs && len(ligs) <= maxLigatures+8; a++ {
		for b := 1; b <= glyphs && len(ligs) <= maxLigatures+8; b++ {
			ligs = append(ligs, fonttest.Ligature{Components: []int{a, b}, Glyph: 1})
		}
	}
	if len(ligs) <= maxLigatures {
		t.Fatalf("only %d ligatures could be built and the bound is %d; the fixture "+
			"does not reach it", len(ligs), maxLigatures)
	}
	last := ligs[len(ligs)-1]
	// Give the last one a distinctive result so that its firing is visible.
	last.Glyph = 2
	ligs[len(ligs)-1] = last

	f := boundsFace(t, glyphs, map[string][]byte{
		"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{
			Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(ligs)},
		}}, map[string][]int{"liga": {0}}),
	})

	text := string([]rune{rune(0x40 + last.Components[0]), rune(0x40 + last.Components[1])})
	got, _ := f.ShapeGlyphs(text)
	if len(got) == 1 && got[0].GID == last.Glyph {
		t.Errorf("the ligature at position %d of the table was applied; past the "+
			"bound of %d the table stops being read", len(ligs), maxLigatures)
	}
}

// TestAKernTableBeyondItsSubtableBoundStopsBeingRead is the bound on how many
// subtables the legacy kern table may have.
//
// The count is two bytes of the font and each subtable is walked in turn. Past
// the bound the rest are not read, and the kerning that lives there does not
// happen — which is what this asserts, by putting the only pair that matters in
// the last subtable.
//
// The first version of this test aimed at a GSUB lookup's subtable list, which
// is a different bound with a different name (maxSubtableList, the format's own
// maximum) — and it failed, saying that a substitution in subtable 264 had been
// applied. That was the test being wrong rather than the engine, and it is why
// a plant is not the only thing worth watching fail.
func TestAKernTableBeyondItsSubtableBoundStopsBeingRead(t *testing.T) {
	const glyphs = 8
	subs := make([]fonttest.KernSubtable, maxSubtables+8)
	for i := range subs {
		subs[i] = fonttest.KernSubtable{
			Coverage: fonttest.KernHorizontal,
			Pairs:    []fonttest.KernPair{{Left: 5, Right: 6, Adjust: -7}},
		}
	}
	// The only pair the text below uses lives past the bound.
	subs[len(subs)-1] = fonttest.KernSubtable{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: 1, Right: 2, Adjust: -250}},
	}

	f := boundsFace(t, glyphs, map[string][]byte{"kern": fonttest.LegacyKern(subs)})

	// Glyphs 1 and 2 are 'A' and 'B'; unkerned they advance 500 each.
	got, _ := f.ShapeGlyphs("AB")
	if len(got) != 2 {
		t.Fatalf("shaping two glyphs gave %d", len(got))
	}
	if got[0].XAdvance != 500 {
		t.Errorf("the kern pair in subtable %d was applied and moved the advance to "+
			"%v; past the bound of %d the table stops being read",
			len(subs), got[0].XAdvance, maxSubtables)
	}
}
