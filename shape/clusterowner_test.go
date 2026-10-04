package shape

import "testing"

// A run cut out of the text it was shaped with takes the glyphs of its own
// characters, whatever cluster shaping merged them into.
//
// In a right-to-left run a zero width non-joiner is taken out and its cluster
// merged into the glyph the pen met before it, which is the letter after it:
// the alef of tatweel-ZWNJ-alef carries the joiner's offset. shaping-no-join-003
// sets the joiner in an element and a font of its own, and cut by clusters the
// group charged the alef's width to the joiner's element and the alef's own run
// drew nothing.
func TestARunTakesTheGlyphsOfItsOwnCharacters(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "NotoSansArabic.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	const tatweel, zwnj, alef = "ـ", "‌", "ا"
	text := tatweel + zwnj + alef
	whole := f.ShapeGroup(text, "", "", true, Features{})
	alefAt := len(tatweel + zwnj)
	merged := false
	for _, g := range whole {
		merged = merged || g.Cluster == len(tatweel)
	}
	if !merged {
		t.Fatal("no glyph carries the joiner's offset, so this test measures nothing")
	}
	cum := GroupAdvances(whole, text)
	joinerHead, joinerThrough := GroupSpan(cum, len(tatweel), alefAt, 1000)
	if joinerThrough != joinerHead {
		t.Errorf("the joiner, which draws nothing, is %v wide", joinerThrough-joinerHead)
	}
	alefHead, alefThrough := GroupSpan(cum, alefAt, len(text), 1000)
	if alefThrough-alefHead != f.GlyphAdvance(whole[0].GID) {
		t.Errorf("the alef is %v wide, and its glyph advances %v", alefThrough-alefHead, f.GlyphAdvance(whole[0].GID))
	}
	// And the alef's run, shaped with the rest merged before it, draws it.
	glyphs, _ := f.ShapeGlyphsMerged(alef, "", "", tatweel+zwnj, "", true, Features{})
	if len(glyphs) != 1 {
		t.Fatalf("the alef's run draws %d glyphs", len(glyphs))
	}
	if glyphs[0].Cluster != 0 {
		t.Errorf("the alef's glyph is at %d of its run", glyphs[0].Cluster)
	}
	joiner, _ := f.ShapeGlyphsMerged(zwnj, "", "", tatweel, alef, true, Features{})
	if len(joiner) != 0 {
		t.Errorf("the joiner's run draws %d glyphs", len(joiner))
	}
}
