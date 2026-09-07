package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The presentation features, and how much of the run they see.
//
// The Indic model applies 'pres', 'abvs', 'blws', 'psts' and 'haln' after the
// final reordering and *constrained to the syllable*, which is the difference
// between it and the Khmer model — Khmer's are "applied all at once after
// clearing syllables" and this engine's Khmer pass does exactly that. The Indic
// pass did the same thing, so a lookup could join the end of one syllable to the
// start of the next.
//
// No bundled face shows it, because a font whose rules are narrow enough never
// produces one. So the fixture declares the rule a font would have to declare
// for the difference to be visible: a 'pres' ligature over two consonants, which
// in text are two syllables and never one.

// devaPresAcrossSyllables declares a 'pres' ligature of two Ka — which are two
// syllables when written together — and one of a Ka and the sign that follows
// it, which are one.
//
// gidKaWithI is named for another rule; here it is only somewhere for the second
// ligature to go.
func devaPresAcrossSyllables() devaFeature {
	return devaLigatures("pres",
		fonttest.Ligature{Components: []int{gidDKa, gidDKa}, Glyph: gidKaKa},
		fonttest.Ligature{Components: []int{gidDKa, gidAAMatra}, Glyph: gidKaWithI},
	)
}

// TestAPresentationFeatureSeesOneSyllable.
func TestAPresentationFeatureSeesOneSyllable(t *testing.T) {
	f := devaFace(t, devaPresAcrossSyllables())

	// Two Ka are two syllables: each consonant carries its own inherent vowel,
	// and nothing joins them. A ligature the font declares over the pair is a
	// ligature across a syllable boundary, and the model does not offer one.
	s := str(devKa, devKa)
	wantGIDs(t, shapedGIDs(t, f, s), []int{gidDKa, gidDKa}, s)

	// And the control, which is what says the font's rule works at all: a Ka and
	// the sign written on it are one syllable, and there the same feature
	// ligates.
	one := str(devKa, devAAMatra)
	wantGIDs(t, shapedGIDs(t, f, one), []int{gidKaWithI}, one)
}
