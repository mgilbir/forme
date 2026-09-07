package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Where a pair positioning lookup looks for the next pair.
//
// The specification: a lookup moves past *both* glyphs where ValueFormat2 is
// non-zero and past the first alone where it is zero. So in "ABC", with pairs
// (A,B) and (B,C), the second pair applies only if the first stated no second
// record — and this advanced past one glyph either way, so it applied a pair a
// conforming shaper never looks for.
//
// It cannot be read off the numbers: a font may state a second record of all
// zeroes, and that is not the same as stating none.

// pairFace builds a three-glyph face with the given kern subtable.
func pairFace(t *testing.T, gpos []byte) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Pairs",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 500, HasShape: true},
			{Rune: 'c', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"GPOS": gpos},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

func advancesOf(t *testing.T, f *Face, s string) []float64 {
	t.Helper()
	gs, _ := f.ShapeGlyphs(s)
	out := make([]float64, len(gs))
	for i, g := range gs {
		out[i] = g.XAdvance
	}
	return out
}

// TestASecondValueRecordConsumesTheGlyphItAdjusted.
func TestASecondValueRecordConsumesTheGlyphItAdjusted(t *testing.T) {
	const gidA, gidB, gidC = 1, 2, 3
	pairs := []fonttest.KernPair{
		{Left: gidA, Right: gidB, Adjust: -100},
		{Left: gidB, Right: gidC, Adjust: -50},
	}

	// With no second record, the lookup moves past the first glyph alone, so B
	// begins the next pair and both adjustments apply. This is ordinary kerning
	// and is why "AVA" kerns twice.
	one := advancesOf(t, pairFace(t, fonttest.GPOS(pairs)), "abc")
	if len(one) != 3 {
		t.Fatalf("shaping \"abc\" gave %d glyphs", len(one))
	}
	if one[0] != 400 || one[1] != 450 || one[2] != 500 {
		t.Errorf("with no second record the advances are %v, want [400 450 500] — "+
			"both pairs apply", one)
	}

	// With one stated — even all zeroes — the lookup moves past both glyphs, so
	// B is not the first glyph of the next pair and (B,C) is never looked for.
	both := advancesOf(t, pairFace(t, fonttest.GPOSBothSides(pairs)), "abc")
	if len(both) != 3 {
		t.Fatalf("shaping \"abc\" gave %d glyphs", len(both))
	}
	if both[0] != 400 || both[1] != 500 || both[2] != 500 {
		t.Errorf("with a second record stated the advances are %v, want "+
			"[400 500 500] — the pair that adjusted B consumed it", both)
	}
}

// TestASecondValueRecordIsStillApplied is the control: consuming the glyph is
// about where the *next* pair is looked for and not about what this one does.
func TestASecondValueRecordIsStillApplied(t *testing.T) {
	const gidA, gidB = 1, 2
	f := pairFace(t, fonttest.GPOSBothSides([]fonttest.KernPair{
		{Left: gidA, Right: gidB, Adjust: -100, SecondAdjust: -30},
	}))
	got := advancesOf(t, f, "ab")
	if len(got) != 2 {
		t.Fatalf("shaping \"ab\" gave %d glyphs", len(got))
	}
	if got[0] != 400 || got[1] != 470 {
		t.Errorf("the advances are %v, want [400 470] — both halves of the record "+
			"are applied", got)
	}
}
