package paragraph

import "testing"

// Which characters end a word, for "text-transform: capitalize".
//
// css-text-3 §2.1.1 titlecases "the first typographic letter unit of each word"
// and leaves the definition of a word to the UA, suggesting UAX #29. That
// annex's WB6 and WB7 are the rule this file needs: a MidLetter or MidNumLet
// *between two letters* does not end the word. "Between" is the whole of it —
// the same character with a space after it does end one.
//
// The apostrophe was already here, because without it "don't" comes out
// "Don'T", which is a real word set wrongly rather than a theoretical one. The
// middle dot is the same case in Catalan, where it is a letter: "cancel·lar" is
// one word and came out "Cancel·Lar". The suite's
// text-transform-capitalize-035 is six of those in four languages.

func TestAJoiningCharacterBetweenLettersDoesNotEndAWord(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		// The apostrophes, in both spellings a document may carry.
		{"don't", "Don't"},
		{"don’t", "Don’t"},
		// Catalan's middle dot, which is a letter of the alphabet.
		{"cancel·lar", "Cancel·lar"},
		{"agiuda·nos", "Agiuda·nos"},
		// The rest of UAX #29's two classes.
		{"a:b", "A:b"},
		{"e.g.", "E.g."},
		{"a·b", "A·b"},
		{"a״b", "A״b"},
		{"a‧b", "A‧b"},
		{"a＇b", "A＇b"},
	} {
		if got, _ := TransformText(tc.in, TransformCapitalize, false, ""); got != tc.want {
			t.Errorf("%q capitalised to %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTheSameCharacterWithNoLetterAfterItDoesEndAWord, which is WB6's "between"
// read as the rule it is rather than as a list of characters to ignore.
func TestTheSameCharacterWithNoLetterAfterItDoesEndAWord(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"end. next", "End. Next"},
		{"a· b", "A· B"},
		{"one' two", "One' Two"},
		// And at the very end of the run there is nothing after it at all.
		{"word.", "Word."},
		// WB6 joins a *single* one: two in a row are two separators, and the
		// letter after them begins a word. This is the pair that tells the rule
		// from a list of characters to skip over.
		{"a..b", "A..B"},
		{"don''t", "Don''T"},
		{"a·-b", "A·-B"},
	} {
		if got, _ := TransformText(tc.in, TransformCapitalize, false, ""); got != tc.want {
			t.Errorf("%q capitalised to %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAnOrdinarySeparatorStillEndsAWord is the containment argument: the rule is
// about a handful of joining characters and must not make every punctuation mark
// one.
func TestAnOrdinarySeparatorStillEndsAWord(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"one two", "One Two"},
		{"one-two", "One-Two"},
		{"one,two", "One,Two"},
		{"one/two", "One/Two"},
		{"(one)", "(One)"},
		{"one\ttwo", "One\tTwo"},
	} {
		if got, _ := TransformText(tc.in, TransformCapitalize, false, ""); got != tc.want {
			t.Errorf("%q capitalised to %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAJoiningCharacterAtTheEndOfARunCarriesTheWordOn. A text node may end
// mid-word — "don" in one element and "'t" in the next — and the character that
// would settle WB6 is in the node after this one. Reading it as continuing the
// word is what keeps those two nodes one word.
func TestAJoiningCharacterAtTheEndOfARunCarriesTheWordOn(t *testing.T) {
	for _, in := range []string{"don'", "cancel·", "e."} {
		if !EndsInWord(in) {
			t.Errorf("%q was read as ending a word; what follows it is in the next "+
				"text node and may well be a letter", in)
		}
	}
	for _, in := range []string{"one ", "one-", "one,"} {
		if EndsInWord(in) {
			t.Errorf("%q was read as continuing a word", in)
		}
	}
}
