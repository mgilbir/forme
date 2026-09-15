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
		// Only the last resort has these, which is what it is for.
		{"Regional indicators", "🇮🇱"},
		// The characters of the emoji sequences line-breaking-013 and -014 are
		// written in. They are worth naming separately from the blocks above
		// because no one face has them *together*: the person is in Noto Sans
		// Symbols and the skin tone in Unifont Upper, so the cluster they form
		// is the one the engine has to cut rather than set in a single face.
		// Each of them has to be findable on its own for that cut to land
		// anywhere, which is what this checks.
		{"Emoji the suite sequences", "⛹🏿♀🤹"},
	} {
		// Per character, because that is how the fallback is asked: a face that
		// covers one block covers no other. Asking for a whole *cluster* at
		// once is a question that can have no answer — no face need hold every
		// character of one — and layout/facerun.go answers it by cutting the
		// cluster into stretches one face can set, which is only possible when
		// the characters are findable one at a time.
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

// TestTheLastResortLeavesNothingUnset.
//
// The library ends in a face that covers almost everything, so the question it
// answers is not "which script is this" but "is there anything at all this
// document would have to set as a space". There should not be, and a character
// nobody thought to add a face for is exactly the case that used to slip
// through: it drew nothing, said "no glyph", and the page was missing a letter.
//
// The characters below are deliberately obscure and from four different planes.
// None of them is in the suite; that is the point — the guarantee is meant to
// hold for a document nobody has seen.
func TestTheLastResortLeavesNothingUnset(t *testing.T) {
	if notoDir(t) == "" {
		return
	}
	set, ok := fontSetForWPT().(FallbackFontSet)
	if !ok {
		t.Fatal("the suite's font set offers no fallback at all")
	}
	for _, c := range []struct {
		what string
		r    rune
	}{
		{"Runic", 'ᚠ'},
		{"Cherokee", 'Ꭰ'},
		{"Thaana", 'ހ'},
		{"Syriac", 'ܐ'},
		{"Braille", '⠁'},
		{"Linear B", '𐀀'},
		{"Gothic", '𐌰'},
		{"Old Italic", '𐌀'},
		{"Musical symbols", '𝄞'},
		{"Mathematical alphanumerics", '𝔄'},
	} {
		if _, found := set.FaceFor(string(c.r), false, false); !found {
			t.Errorf("%s: nothing in the library can set %q (U+%04X), so a "+
				"document using it would draw a space", c.what, string(c.r), c.r)
		}
	}
}
