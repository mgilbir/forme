package shape

import "testing"

// The character questions the shaper asks are answered from the release its
// tables are generated from, and not from the toolchain's. Every character
// below was assigned by Unicode 16, after the 15.0.0 package unicode answers
// from under Go 1.26 — the release go.mod names — and each question was
// package unicode's; see paragraph's test of the same name.
func TestTheCharacterQuestionsAreThePinnedReleases(t *testing.T) {
	// HarfBuzz's automatic fractions take any run of decimal digits either
	// side of U+2044, and the Garay digits are decimal digits.
	runes := []rune{0x10D41, fractionSlash, 0x10D42}
	buf := make([]Glyph, len(runes))
	maskFractions(buf, runes, false)
	if buf[0].mask&maskNumr == 0 || buf[2].mask&maskDnom == 0 {
		t.Errorf("Garay 1⁄2 is not a fraction: masks %b %b %b", buf[0].mask, buf[1].mask, buf[2].mask)
	}
	// A punctuation mark ends an Indic word, and U+1B4E BALINESE INVERTED
	// CARIK SIKI is one.
	if !endsWordForIndic(0x1B4E) {
		t.Error("U+1B4E, a Unicode 16 punctuation mark, does not end an Indic word")
	}
}
