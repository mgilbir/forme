package shape

import (
	"os"
	"testing"
)

// The half-width forms a CJK face states, read from 'halt' and applied to
// nothing.
//
// The font is the suite's own subset, which exists because the reftests need a
// face whose 'halt' can be relied on. Its numbers are checked against the table
// rather than against a rule of thumb: the trim is *not* "half the advance" for
// both sides of a bracket pair, and the pair below is the case that says so.
func haltFace(t *testing.T) *Face {
	t.Helper()
	data, err := os.ReadFile(
		"../testdata/wpt/fonts/noto/cjk/NotoSansCJKjp-Regular-subset-halt.otf")
	if err != nil {
		t.Skip("the suite's CJK subset is not fetched: ", err)
	}
	face, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func TestTheFaceStatesItsHalfWidthForms(t *testing.T) {
	face := haltFace(t)
	if upem := face.UnitsPerEm(); upem != 1000 {
		t.Fatalf("the face is %d units to the em; the numbers below are 1000's", upem)
	}
	for _, tc := range []struct {
		r        rune
		dx, dAdv int
		ok       bool
		what     string
	}{
		{'）', 0, -500, true,
			"a closing bracket gives up the blank behind it and the ink does not move"},
		{'（', -500, -500, true,
			"an opening bracket gives up the blank in front, so its ink moves back into it"},
		{'、', 0, -500, true, "an ideographic comma"},
		{'国', 0, 0, false, "an ideograph, which is all ink and has no blank to give up"},
		{'a', 0, 0, false, "a Latin letter, which the feature does not cover"},
	} {
		gid, has := face.GlyphID(tc.r)
		if !has {
			t.Errorf("%s (U+%04X): the face has no glyph for it", tc.what, tc.r)
			continue
		}
		dx, dAdv, ok := face.HalfWidthTrim(gid)
		if ok != tc.ok || dx != tc.dx || dAdv != tc.dAdv {
			t.Errorf("%s (U+%04X): dx=%d dAdvance=%d ok=%v, want dx=%d dAdvance=%d ok=%v",
				tc.what, tc.r, dx, dAdv, ok, tc.dx, tc.dAdv, tc.ok)
		}
	}
}

// And it is applied to nothing: reading the feature must not turn it on.
//
// This is the whole risk of the change that added it. defaultPositionFeatures is
// deliberately without 'halt' and 'palt' — see the note beside it, where Noto
// Sans JP's 'palt' narrowing every full-width kana to its ink is the worked
// example — so a reader that filled the ordinary adjustment table would set
// every CJK document a little narrower and nothing in the display list would say
// why.
func TestReadingHalfWidthFormsDoesNotApplyThem(t *testing.T) {
	face := haltFace(t)
	// Through the shaper and not through Measure. Measure sums the advances the
	// metrics table states and never reaches GPOS at all, so it cannot see a
	// positioning feature whether it is applied or not — a planted "halt" in
	// defaultPositionFeatures left it saying exactly what it says now, which is
	// what a decorative check looks like.
	for _, r := range []rune{'）', '（', '、', '国'} {
		glyphs, missing := face.ShapeGlyphsInContext(string(r), "", "", Features{})
		if missing != 0 || len(glyphs) != 1 {
			t.Fatalf("U+%04X shaped to %d glyphs with %d missing", r, len(glyphs), missing)
		}
		full := float64(face.UnitsPerEm())
		if g := glyphs[0]; g.XAdvance != full || g.XOffset != 0 {
			t.Errorf("U+%04X shapes to advance %v offset %v; every glyph here is "+
				"full-width, so both should be %v and 0 — a trim that reached "+
				"the shaping would show as half of one",
				r, g.XAdvance, g.XOffset, full)
		}
	}
}
