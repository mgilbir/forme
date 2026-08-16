package layout

import (
	"testing"
)

// What the suite's fallback library is actually for.
//
// The Noto faces here are not a shortcut for anything the documents ask for —
// no suite document names Noto — they are coverage for the scripts the fourteen
// standard PDF faces do not have. Which scripts those are is not a matter of
// taste: it is whatever the suite writes, and the engine says so itself, one
// glyph-missing finding per character it could not set.
//
// This pins the answer, because the list in notoFaces is a list of file names
// and nothing about a file name says which characters are inside it. A face
// dropped from that list, or renamed upstream, or fetched from a variant that
// carries a different subset, takes its script with it silently — and the only
// thing that would notice is the glyph-missing count, which is not what the
// ratchet reads.

// TestTheFallbackLibraryCoversWhatTheSuiteWrites names each block that the
// standard faces lack and the suite uses.
//
// The characters are the ones the suite's own documents set: the Ogham feather
// marks of trailing-ogham-001, the Coptic and Deseret of the bicameral
// text-transform tests, and the Roman numerals of the two beside them.
func TestTheFallbackLibraryCoversWhatTheSuiteWrites(t *testing.T) {
	if notoDir(t) == "" {
		return
	}
	set, ok := fontSetForWPT().(FallbackFontSet)
	if !ok {
		t.Fatal("the suite's font set offers no fallback at all")
	}
	for _, c := range []struct{ what, text string }{
		{"Hebrew", "א"},
		{"Arabic", "ا"},
		{"Armenian", "Ա"},
		{"Georgian", "ა"},
		{"Ogham", "ᚋᚌᚐᚑ᚛᚜"},
		{"Coptic", "ϢϤϦϨϪϬϮ"},
		{"Deseret", "𐐀𐐁𐐨𐐩"},
		{"Number Forms", "ⅬⅭⅮⅯⅼⅽⅾⅿ"},
	} {
		// Per character, because that is how the fallback is asked: a face that
		// covers one block covers no other, and asking for the whole string at
		// once is the question that has no answer — see layout/facerun.go.
		for _, r := range c.text {
			if _, found := set.FaceFor(string(r), false, false); !found {
				t.Errorf("%s: no face in the suite library can set %q (U+%04X)",
					c.what, string(r), r)
			}
		}
	}
}

// TestTheStandardFacesStillLackThem is the other half, and it is what keeps the
// test above from being about nothing: if a standard face grew these characters
// the fallback would never be asked, and a library that had lost them would
// still pass.
func TestTheStandardFacesStillLackThem(t *testing.T) {
	face, ok := StandardFonts().Face("serif", false, false)
	if !ok {
		t.Fatal("no serif face")
	}
	for _, r := range []rune{'ᚋ', 'Ϣ', 'Ⅼ', '𐐀'} {
		if _, missing := face.ShapeGlyphs(string(r)); missing == 0 {
			t.Errorf("the standard serif face has a glyph for U+%04X, so the "+
				"fallback library is not what puts it on the page", r)
		}
	}
}
