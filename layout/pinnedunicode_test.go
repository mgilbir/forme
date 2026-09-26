package layout

import "testing"

// The character questions layout asks are answered from the release the
// engine's tables are generated from, and not from the toolchain's. Every
// character below was assigned by Unicode 16, after the 15.0.0 package unicode
// answers from under Go 1.26 — the release go.mod names — and each question was
// package unicode's; see paragraph's test of the same name.
func TestTheCharacterQuestionsAreThePinnedReleases(t *testing.T) {
	// A letter with a capital is lower case to small-caps, and the Cyrillic
	// tje has had one since Unicode 16.
	if got := caseOf(0x1C8A); got != caseLower {
		t.Errorf("caseOf(U+1C8A CYRILLIC SMALL LETTER TJE) = %d, want lower case", got)
	}
	if got := caseOf(0x1C89); got != caseUpper {
		t.Errorf("caseOf(U+1C89 CYRILLIC CAPITAL LETTER TJE) = %d, want upper case", got)
	}
	if lower, upper := hasCase("ᲊᲉ"); !lower || !upper {
		t.Errorf("hasCase(tje, TJE) = %v, %v; the text has both cases", lower, upper)
	}
	// Punctuation before the first letter is part of ::first-letter, and
	// U+10D6E GARAY HYPHEN is punctuation.
	if got, want := firstLetterLen("\U00010D6Ea b"), len("\U00010D6Ea"); got != want {
		t.Errorf("firstLetterLen(Garay hyphen, a) = %d, want %d", got, want)
	}
	// A mark continues the word it is written on.
	g := &hyphenGather{}
	if !g.continuesTheWord(0x10D50) {
		t.Error("a Garay letter does not begin a word to hyphenate")
	}
	g.word = append(g.word, 'a')
	if !g.continuesTheWord(0x0897) {
		t.Error("U+0897 ARABIC PEPET, a mark, ends the word it is written on")
	}
}
