package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Sinhala.
//
// It reorders — "කෙ" is a consonant and a vowel sign written after it and drawn
// before it, which is the whole reason a syllabic shaper exists — and it was in
// neither of the two lists that decide whether a run is given one. So its text
// came out in the order it was stored: every word with a kombuva in it had the
// vowel on the wrong side of its letter.
//
// The table already carried it. usetable.go gives U+0DCA the halant class that
// only Sinhala uses and U+0D9A onwards the base class, and nothing asked.

const (
	sinKa       = 0x0D9A // SINHALA LETTER ALPAPRAANA KAYANNA
	sinKombuva  = 0x0DD9 // SINHALA VOWEL SIGN KOMBUVA, written after, drawn before
	sinAela     = 0x0DCF // SINHALA VOWEL SIGN AELA-PILLA, written and drawn after
	sinAlLakuna = 0x0DCA // SINHALA SIGN AL-LAKUNA, the halant
	sinNa       = 0x0DB1 // SINHALA LETTER DANTAJA NAYANNA
)

const (
	gidSinKa = 1 + iota
	gidSinKombuva
	gidSinAela
	gidSinAlLakuna
	gidSinNa
)

// sinhalaFace is a font with the five Sinhala characters below and no features
// at all. The reordering under test is the shaper's own — no font declares it —
// so a font with nothing in it is the right fixture.
func sinhalaFace(t *testing.T) *Face {
	t.Helper()
	runes := []rune{sinKa, sinKombuva, sinAela, sinAlLakuna, sinNa}
	glyphs := make([]fonttest.Glyph, len(runes))
	for i, r := range runes {
		glyphs[i] = fonttest.Glyph{Rune: r, Advance: 400 + 10*i, HasShape: true}
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Sinhala", Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestSinhalaIsShapedBySyllable is the defect: the script reached no syllabic
// shaper at all.
func TestSinhalaIsShapedBySyllable(t *testing.T) {
	if !usesSyllabicShaper(scriptOf(sinKa)) {
		t.Error("a Sinhala run is not shaped by syllable, so nothing reorders it")
	}
	if !universalScripts[scriptTagOf(t, sinKa)] {
		t.Errorf("Sinhala is not in the universal engine's list")
	}
}

// TestAPreBaseSinhalaVowelIsDrawnBeforeItsLetter is what that costs on a page.
//
// The kombuva is written after the consonant and drawn before it. Left in
// storage order it comes out on the wrong side of every letter it is written
// with, which is most Sinhala text.
func TestAPreBaseSinhalaVowelIsDrawnBeforeItsLetter(t *testing.T) {
	f := sinhalaFace(t)
	s := str(sinKa, sinKombuva)
	wantGIDs(t, shapedGIDs(t, f, s), []int{gidSinKombuva, gidSinKa}, s)
}

// TestAPostBaseSinhalaVowelStaysWhereItIsWritten is the control: a shaper that
// reversed everything would pass the test above, and the aela-pilla is written
// after its letter and drawn there.
func TestAPostBaseSinhalaVowelStaysWhereItIsWritten(t *testing.T) {
	f := sinhalaFace(t)
	s := str(sinKa, sinAela)
	wantGIDs(t, shapedGIDs(t, f, s), []int{gidSinKa, gidSinAela}, s)
}

// TestASinhalaConjunctKeepsItsOrder pins the halant, which is the character
// usetable.go carries a class of its own for: the al-lakuna is both a halant
// and a vowel modifier, and a syllable stacked with one is written and drawn in
// the same order.
func TestASinhalaConjunctKeepsItsOrder(t *testing.T) {
	f := sinhalaFace(t)
	s := str(sinKa, sinAlLakuna, sinNa)
	wantGIDs(t, shapedGIDs(t, f, s), []int{gidSinKa, gidSinAlLakuna, gidSinNa}, s)
}

// scriptTagOf is the OpenType tag a character's script is written under.
func scriptTagOf(t *testing.T, r rune) string {
	t.Helper()
	tags := scriptTags(scriptOf(r))
	if len(tags) == 0 {
		t.Fatalf("U+%04X belongs to no script this package names", r)
	}
	return tags[0]
}
