package shape

import "testing"

// TestSyllablesTheModelDoesNotReorderAreSyllables: a symbol cluster — an
// avagraha with a visarga and an udatta — is one syllable, so a font's 'abvs'
// joins the two marks on it, and a character of no Indic category is a
// syllable of its own that 'ccmp' is applied to. Both were passed through a
// glyph at a time, and neither feature reached them. The combining asterisk
// is Other to the model, so a Vedic sign after it is shown against a dotted
// circle. The glyphs are the fixture's: 1 ka, 2 avagraha, 3 visarga, 4 udatta,
// 5 what 'abvs' makes of the two, 6 '!', 7 what 'ccmp' makes of it, 8 the
// asterisk, 9 the Vedic sign U+1CDB, 10 the dotted circle.
func TestSyllablesTheModelDoesNotReorderAreSyllables(t *testing.T) {
	checkModelCases(t, "devanagari", []modelCase{
		{"\u093D\u0903\u0951", "2,500 5,210", "the marks on an avagraha, joined"},
		{"\u0915!", "1,600 7,310", "'ccmp' on a character of no category"},
		{"!", "6,300", "and alone, where the run is Common"},
		{"\u0915\u20F0\u1CDB", "1,600 8,0 10,550 9,0", "a Vedic sign after the asterisk, against a circle"},
		{"\u0915\u1CDB", "1,600 9,0", "and after the letter, not"},
		{"\u093D\u0951", "2,500 4,0", "an udatta alone on an avagraha"},
	})
}

// TestAVowelSpeltTwiceIsShownAgainstACircle: a vowel followed by a sign that
// spells another vowel is shown against a dotted circle — tested on the text as
// written, before normalisation moves the length mark in front of a virama,
// and put in where the face has no circle to draw it with, as .notdef that is
// not counted missing. The circle is a mark where the sign after it is, so a
// rule that steps over marks steps over it. The glyphs are the fixture's: 1 o,
// 2 virama, 3 the length mark, 4 ka, 5 the i sign, 6 what 'pres' makes of a
// circle, 7 the dotted circle.
func TestAVowelSpeltTwiceIsShownAgainstACircle(t *testing.T) {
	checkModelCases(t, "telugu", []modelCase{
		{"\u0C12\u0C4D\u0C55", "1,600 3,0 2,0", "a virama between the vowel and the sign"},
		{"\u0C15\u0C3F\u0C55", "4,610 5,0 7,500 3,0", "the i sign and the length mark, and 'pres' steps over the circle"},
		{"\u0C12\u0C55", "1,600 7,500 3,0", "o and the length mark"},
		{"\u25CC\u0C55", "6,520 3,0", "a circle the text holds is not a mark, and 'pres' replaces it"},
	})
	checkModelCases(t, "telugu-no-circle", []modelCase{
		{"\u0C15\u0C3F\u0C55", "4,610 5,0 0,0 3,0", ".notdef where the face has no circle"},
		{"\u0C12\u0C4D\u0C55", "1,600 3,0 2,0", "and nothing where there is no such sequence"},
	})
	f, err := Load(modelFixtures()["telugu-no-circle"])
	if err != nil {
		t.Fatal(err)
	}
	if _, missing := f.ShapeGlyphs("\u0C15\u0C3F\u0C55"); missing != 0 {
		t.Errorf("the circle the face cannot draw was counted missing: %d", missing)
	}
}

// TestKhmerAndMyanmarReadTheSharedCategories: a digit, a Pao digit and a
// superscript two are what HarfBuzz's one category table says they are to the
// Khmer and Myanmar models — placeholders a vowel sign can be written on, and a
// tone that closes a syllable — where they were characters of no syllable. The
// glyphs are the fixtures': for Myanmar 1 ka, 2 the e sign, 3 '0', 4 the Pao
// zero, 5 superscript two, 6 the circle; for Khmer 1 ka, 2 the aa sign, 3 '1',
// 4 the circle.
func TestKhmerAndMyanmarReadTheSharedCategories(t *testing.T) {
	checkModelCases(t, "myanmar", []modelCase{
		{"0\u1031", "2,300 3,500", "the e sign before the digit it is written on"},
		{"\U000116D0\u1031", "2,300 4,510", "and before a Pao digit"},
		{"\u1000\u00B2", "1,600 5,200", "a superscript closing a syllable"},
		{"\u1000 \u00B2", "1,600 0,0 5,200", "a superscript after a space, against no circle"},
		{"\u1031", "2,300 6,550", "the e sign alone, against a circle"},
	})
	checkModelCases(t, "khmer", []modelCase{
		{"1\u17B6", "3,500 2,300", "the aa sign on a digit"},
		{"\u1780\u17B6", "1,600 2,300", "on a letter"},
		{"\u17B6", "4,550 2,300", "alone, against a circle"},
	})
}
