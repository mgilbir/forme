package shape

import "testing"

// The context a run is shaped in, when its neighbour is set in another face.
//
// Font fallback puts the two sides of a boundary in two fonts, and the two
// things the context decides do not both cross it. Which of its four shapes a
// letter takes is decided by the characters beside it, and a character is the
// same character whichever font sets it — Unicode's joining enforcement, and
// the suite's shaping-join-002 and shaping-tatweel-002 and -003, where a zero
// width joiner or a tatweel is pulled from another font by unicode-range and
// the Arabic letters either side must still take their joined forms.
//
// A kerning pair is not. It is one font's statement about two of its own
// glyphs, and a font change is a change in formatting: the pair across that
// boundary is not this font's to apply.

// TestAJoinedShapeCrossesAFaceChange.
func TestAJoinedShapeCrossesAFaceChange(t *testing.T) {
	f := nkoFace(t)
	alone := gidsOf(t, f, "ߞ")
	// The neighbour is a joiner, whichever font holds it.
	across, _ := f.ShapeGlyphsAcrossFaces("ߞ", "", "‍", Features{})
	within, _ := f.ShapeGlyphsInContext("ߞ", "", "‍", Features{})
	if len(across) != 1 || len(within) != 1 {
		t.Fatalf("one letter shaped to %v and %v", across, within)
	}
	if across[0].GID != within[0].GID {
		t.Errorf("the letter takes shape %d beside a joiner in its own face and "+
			"%d beside one in another; a character is the same character "+
			"whichever font sets it", within[0].GID, across[0].GID)
	}
	if across[0].GID == alone[0] {
		t.Errorf("the letter kept its lone shape %d beside a joiner in another "+
			"face", across[0].GID)
	}
}

// TestAKernPairDoesNotCrossAFaceChange is the other half, and the pair is the
// point: an implementation that passed the context to everything would satisfy
// the row above and get this one wrong, and one that passed it to nothing would
// do the reverse.
func TestAKernPairDoesNotCrossAFaceChange(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	pair := kernedPair(t, f, "AV", "AW", "Ta", "Vo", "AT")
	first, second := pair[:1], pair[1:]
	whole := wholeAdvance(f, pair)
	alone := wholeAdvance(f, first)
	within := contextAdvance(f, first, "", second)
	if within == alone {
		t.Fatalf("%q is not kerned across the boundary even within one face, so "+
			"this fixture cannot say what it means to say", pair)
	}
	if within != whole-wholeAdvance(f, second) {
		t.Errorf("within one face %q measures %g across the boundary and %g "+
			"whole", pair, within, whole)
	}
	gs, _ := f.ShapeGlyphsAcrossFaces(first, "", second, Features{})
	if got := advanceOf(gs); got != alone {
		t.Errorf("across a face change %q measures %g, want the unkerned %g — "+
			"the pair belongs to whichever font holds both glyphs, and here "+
			"neither does", pair, got, alone)
	}
}
