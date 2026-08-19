package shape

import (
	"os"
	"path/filepath"
	"testing"
)

// Which positioning features apply without being asked for.
//
// A script's LangSys lists every feature *available* for that script, not every
// feature that is on. The optional ones are there to be requested — by
// font-feature-settings, or by a font-variant property — and reading positioning
// lookups from all of them turns every one of them on for every document.
//
// 'palt' is the one that showed it. Proportional alternate widths is a type 1
// adjustment narrowing each full-width kana to the width of the ink in it, which
// is the whole purpose of the feature and the reason it is opt-in: Japanese is
// normally set full-width. Applied by default it made U+3042 971 units instead
// of 1000, so an ideographic space no longer covered the character beside it and
// a column of kana no longer lined up.

// notoJP loads the bundled Japanese face at the weight this engine instances it
// to. It is the font the case was found with, and it declares 'palt'.
func notoJP(t *testing.T) *Face {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that declares palt")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansJP-VF.ttf"))
	if err != nil {
		t.Skipf("the Japanese face is not in the checkout: %v", err)
	}
	f, err := LoadInstance(data, map[string]float64{"wght": 400})
	if err != nil {
		t.Fatalf("the face did not load: %v", err)
	}
	return f
}

// TestAnOptionalPositioningFeatureIsNotAppliedByDefault is the bug, stated as
// the agreement it broke: a glyph's advance is one number, and the two ways of
// asking for it must not differ.
func TestAnOptionalPositioningFeatureIsNotAppliedByDefault(t *testing.T) {
	f := notoJP(t)
	for _, r := range []rune{
		0x3042, // あ, which palt narrows
		0x3044, // い
		0x3000, // the ideographic space, which it does not
		'X',
	} {
		want, ok := f.Advance(r)
		if !ok {
			t.Fatalf("the face has no glyph for U+%04X", r)
		}
		glyphs, _ := f.ShapeGlyphs(string(r))
		var got float64
		for _, g := range glyphs {
			got += g.XAdvance
		}
		if got != want {
			t.Errorf("U+%04X advances %v when measured and %v when shaped; nothing "+
				"was asked for that should have moved it", r, want, got)
		}
	}
}

// TestAKanaKeepsItsFullWidth says the same thing as the number it is about,
// so that a change making both paths agree on the *wrong* number is still
// caught. A kana in this face is one em, which is what makes Japanese line up
// in a grid.
func TestAKanaKeepsItsFullWidth(t *testing.T) {
	f := notoJP(t)
	em := float64(f.UnitsPerEm())
	glyphs, _ := f.ShapeGlyphs("あ")
	var got float64
	for _, g := range glyphs {
		got += g.XAdvance
	}
	if got != em {
		t.Errorf("U+3042 shapes to %v and the em is %v; a full-width kana is one em "+
			"unless a document asks for proportional metrics", got, em)
	}
}

// TestTheDefaultPositioningFeaturesStillApply is the containment case, and the
// half that matters most: the list is a restriction, and a restriction that took
// out too much would leave every mark unattached and every pair unkerned with
// nothing to show for it.
func TestTheDefaultPositioningFeaturesStillApply(t *testing.T) {
	for _, tag := range []string{"kern", "mark", "mkmk", "curs", "dist", "abvm", "blwm"} {
		if !defaultPositionFeatures[tag] {
			t.Errorf("%q is not applied by default, and it is one of the features "+
				"that must be", tag)
		}
	}
	for _, tag := range []string{"palt", "halt", "vpal", "cpsp", "smcp", "onum"} {
		if defaultPositionFeatures[tag] {
			t.Errorf("%q is applied by default, and it is a feature a document has "+
				"to ask for", tag)
		}
	}
}
