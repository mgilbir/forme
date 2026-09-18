package paragraph

import (
	"testing"
	"unicode/utf8"
)

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
		if got, _ := TransformText(tc.in, TransformCapitalize, WordClosed, ""); got != tc.want {
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
		if got, _ := TransformText(tc.in, TransformCapitalize, WordClosed, ""); got != tc.want {
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
		if got, _ := TransformText(tc.in, TransformCapitalize, WordClosed, ""); got != tc.want {
			t.Errorf("%q capitalised to %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAJoinerWithNothingInFrontOfItJoinsNothing pins the value rather than the
// agreement.
//
// TestTheTwoWalksAgreeAboutWhereAWordEnds asks that cutting the text changes
// nothing, which is the property the defect broke — and it is satisfied by two
// walks that are consistently wrong. Read the apostrophe of "'a" as joining what
// is in front of it and both spellings come out "'a", agreeing with each other
// and with no browser.
//
// So the letters are written down. WB6 joins a word *between* two letters, and
// what is in front of the first apostrophe here is the beginning of the text.
func TestAJoinerWithNothingInFrontOfItJoinsNothing(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"'a", "'A"},
		{"\u2019a", "\u2019A"},
		{".a", ".A"},
		{"\u00b7a", "\u00b7A"},
		{"'a 'b", "'A 'B"},
		// And the second joiner of a pair, which has a joiner in front of it
		// rather than a letter.
		{"a''b", "A''B"},
		{"a..b", "A..B"},
		// The case the rule exists for is unchanged, which is the containment
		// argument: one joiner between two letters still joins.
		{"don't", "Don't"},
		{"cancel\u00b7lar", "Cancel\u00b7lar"},
	} {
		if got, _ := TransformText(tc.in, TransformCapitalize, WordClosed, ""); got != tc.want {
			t.Errorf("%q capitalised to %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAJoiningCharacterAtTheEndOfARunCarriesTheWordOn. A text node may end
// mid-word — "don" in one element and "'t" in the next — and the character that
// would settle WB6 is in the node after this one. That is what WordPending is:
// not an answer, but the question handed on.
func TestAJoiningCharacterAtTheEndOfARunCarriesTheWordOn(t *testing.T) {
	for _, in := range []string{"don'", "cancel·", "e."} {
		if got := WordStateAfter(in, WordClosed); got != WordPending {
			t.Errorf("%q left state %v; what follows it is in the next text node "+
				"and may well be a letter", in, got)
		}
	}
	for _, in := range []string{"one ", "one-", "one,"} {
		if got := WordStateAfter(in, WordClosed); got != WordClosed {
			t.Errorf("%q left state %v, want the word closed", in, got)
		}
	}
	// And the joiner with nothing in front of it, which is the other half of the
	// same rule and was the defect. An apostrophe joins what is behind it to what
	// is in front, so one that begins a text node joins nothing — unless the node
	// before it left a word open, which is what the incoming state carries.
	for _, in := range []string{"'", "\u00b7", "."} {
		if got := WordStateAfter(in, WordClosed); got != WordClosed {
			t.Errorf("%q with nothing in front of it left state %v; the letter "+
				"after it begins a word", in, got)
		}
		if got := WordStateAfter(in, WordOpen); got != WordPending {
			t.Errorf("%q after a word left state %v; it is the apostrophe of "+
				"\"don\" and \"'t\" set in two nodes", in, got)
		}
		// And after a joiner rather than after a letter, which is the third
		// state earning its place: WB6 is between two *letters*, so a second
		// joiner has no letter in front of it and joins nothing. "a''b" takes
		// two capitals however it is cut.
		if got := WordStateAfter(in, WordPending); got != WordClosed {
			t.Errorf("%q after another joiner left state %v; a joiner is not a "+
				"letter for WB6 to be between", in, got)
		}
	}
}

// TestTheTwoWalksAgreeAboutWhereAWordEnds.
//
// Capitalising a text node needs two answers about words — which letters begin
// one, and whether the node leaves one open for the node after it — and they
// used to come from two pieces of code. capitalizeWords tracked the state as it
// walked; EndsInWord read the last character on its own and called every joining
// character a word, apostrophe included, without asking what was in front of it.
//
// So "'a" in one text node capitalised to "'A" and the same two characters in
// two nodes came out "'a". Both walks go through stepWord now, and what this
// asks is the property that makes that worth doing: cutting the text does not
// change what it is capitalised to.
//
// The cut is every position rather than the interesting one, because which
// position is interesting is exactly what was got wrong.
func TestTheTwoWalksAgreeAboutWhereAWordEnds(t *testing.T) {
	for _, text := range []string{
		"'a", "'", "a'", "don't", "don'", "'t", "cancel·lar", "e.g", "a'b'c",
		"one two", "·a", "a·", "ab", "a1'2b", "one-two", "(one)", "''a",
		"a''b", " 'a", "'a'", "1'2", "a.b.c", "one, two", "’a", "a’b",
	} {
		whole, _ := TransformText(text, TransformCapitalize, WordClosed, "")
		for cut := 1; cut < len(text); cut++ {
			if !utf8.RuneStart(text[cut]) {
				continue
			}
			head, open := TransformText(text[:cut], TransformCapitalize, WordClosed, "")
			tail, _ := TransformText(text[cut:], TransformCapitalize, open, "")
			if got := head + tail; got != whole {
				t.Errorf("%q cut at %d capitalised to %q; in one piece it is %q",
					text, cut, got, whole)
			}
		}
	}
}
