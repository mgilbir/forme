package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestGlyphExtentsIsHarfBuzzsAnswer: GlyphExtents is the per-glyph box the
// engine's ink questions are asked of, in HarfBuzz's form. The box of 口 in
// NotoSansJP-VF at weight 400 — the face the reftest harness lends the engine
// for Japanese — is what uharfbuzz 0.56.2 (HarfBuzz 14.5.0) answers for it with
// set_variations({"wght": 400}): x_bearing 127, y_bearing 735, width 750,
// height -790. Every glyph of that instance and of the harness's other
// fallback faces was compared out of tree the same way and agrees; the CFF
// and colour faces are held to HarfBuzz in tree by cffink_test.go and
// colrink_test.go through the same glyphExtents.
func TestGlyphExtentsIsHarfBuzzsAnswer(t *testing.T) {
	face, err := LoadInstance(fonttest.NotoFile(t, "NotoSansJP-VF.ttf"), map[string]float64{"wght": 400})
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := face.GlyphIDForTest('口')
	if !ok || gid != 3482 {
		t.Fatalf("口 maps to glyph %d (%v), want 3482", gid, ok)
	}
	xb, yb, w, h, ok := face.GlyphExtents(gid)
	if !ok || xb != 127 || yb != 735 || w != 750 || h != -790 {
		t.Errorf("口 has extents %d %d %d %d (%v), want HarfBuzz's 127 735 750 -790",
			xb, yb, w, h, ok)
	}
	// A space is a glyph with no ink, which is an answer and not a failure.
	space, _ := face.GlyphIDForTest(' ')
	if xb, yb, w, h, ok := face.GlyphExtents(space); !ok || xb|yb|w|h != 0 {
		t.Errorf("the space has extents %d %d %d %d (%v), want zeros and true", xb, yb, w, h, ok)
	}
	// A glyph the face does not have, and a face with no glyph program.
	for _, g := range []int{-1, face.NumGlyphs()} {
		if _, _, _, _, ok := face.GlyphExtents(g); ok {
			t.Errorf("glyph %d, which the face does not have, has extents", g)
		}
	}
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, ok := std.GlyphExtents(36); ok {
		t.Error("a standard face answered for a glyph index; it states its boxes by name")
	}
}
