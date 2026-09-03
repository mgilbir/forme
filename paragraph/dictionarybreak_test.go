package paragraph

import "testing"

// Lexical line breaking: where a line may end in a script that writes no spaces
// between its words.
//
// The rule is that there is no rule. UAX #14 gives these scripts class SA and
// declines to say where a line may end in them, because where one word ends and
// the next begins is a fact about the vocabulary and not about the characters.
// So this is the one table in the engine that is a *language*, and these are the
// tests that it is the right language and that it is consulted.
//
// The Thai here is ordinary and checkable: "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e" is Bangkok,
// "\u0e04\u0e37\u0e2d" is "is", "\u0e2a\u0e27\u0e22\u0e07\u0e32\u0e21" is "beautiful". A reader of Thai can check the
// expected segmentation in this file against the sentence, which is the point
// of writing it out rather than counting offsets.

// TestALineEndsBetweenThaiWords.
func TestALineEndsBetweenThaiWords(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		// Bangkok | is | beautiful.
		{"\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e\u0e04\u0e37\u0e2d\u0e2a\u0e27\u0e22\u0e07\u0e32\u0e21", "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e|\u0e04\u0e37\u0e2d|\u0e2a\u0e27\u0e22\u0e07\u0e32\u0e21"},
		// language | Thai | language | Thai.
		{"\u0e20\u0e32\u0e29\u0e32\u0e44\u0e17\u0e22\u0e20\u0e32\u0e29\u0e32\u0e44\u0e17\u0e22", "\u0e20\u0e32\u0e29\u0e32|\u0e44\u0e17\u0e22|\u0e20\u0e32\u0e29\u0e32|\u0e44\u0e17\u0e22"},
	} {
		if got := marks(t, c.text, WordBreak{}, LineBreak{}); got != c.want {
			t.Errorf("%q breaks as %q, want %q", c.text, got, c.want)
		}
	}
}

// TestTheLongerWordWinsWhereItLeavesAWordBehindIt.
//
// Plain longest match is wrong on the ordinary case of a long word that begins
// with a short one, and one word of lookahead is what separates a table lookup
// from a segmentation. "\u0e01\u0e23\u0e38\u0e07" (city) and "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e" (Bangkok) are both
// words, and so is "\u0e40\u0e17\u0e1e" (deity) — so a segmenter that took the shortest
// first would still find words, and would find the wrong ones.
func TestTheLongerWordWinsWhereItLeavesAWordBehindIt(t *testing.T) {
	d := dictionaryFor('\u0e01')
	if d == nil {
		t.Fatal("there is no Thai dictionary")
	}
	for _, w := range []string{"\u0e01\u0e23\u0e38\u0e07", "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e", "\u0e40\u0e17\u0e1e"} {
		if d.nodes[w]&isWord == 0 {
			t.Fatalf("%q is not in the dictionary, so the fixture cannot say what "+
				"it means to say", w)
		}
	}
	const text = "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e"
	if got := marks(t, text, WordBreak{}, LineBreak{}); got != text {
		t.Errorf("%q breaks as %q; it is one word, and the two words inside it "+
			"are not where it divides", text, got)
	}
}

// TestAStretchTheVocabularyDoesNotHaveStillBreaks.
//
// §5.1: "some form of fallback line breaking must occur even if the UA doesn't
// know how to perform it correctly. Overflowing is not allowed." A dictionary
// that found nothing and offered nothing would overflow, so the stretch it
// cannot read falls back to the character boundary — and the search resumes
// after every character, so a word beginning in the middle of it is still found.
func TestAStretchTheVocabularyDoesNotHaveStillBreaks(t *testing.T) {
	// Three Thai letters that are not a word, then Bangkok.
	const text = "\u0e01\u0e02\u0e04\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e"
	const want = "\u0e01|\u0e02|\u0e04|\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e"
	if got := marks(t, text, WordBreak{}, LineBreak{}); got != want {
		t.Errorf("%q breaks as %q, want %q — what the vocabulary has is one word "+
			"and what it does not is one character at a time", text, got, want)
	}
}

// TestManualTurnsWordDetectionOff.
//
// §5.2's "manual": the only places a line may end inside a word are the ones the
// document marked. A box no characters wide overflows rather than dividing text
// somewhere its author did not sanction, which is what the suite's
// word-break-manual-001 asks for.
func TestManualTurnsWordDetectionOff(t *testing.T) {
	const text = "\u0e01\u0e23\u0e38\u0e07\u0e40\u0e17\u0e1e\u0e04\u0e37\u0e2d\u0e2a\u0e27\u0e22\u0e07\u0e32\u0e21"
	if got := marks(t, text, WordBreak{Manual: true}, LineBreak{}); got != text {
		t.Errorf("under manual %q breaks as %q, want no opportunity in it", text, got)
	}
	// And the fallback goes with it: a stretch with no words in it is not
	// divided either, because detection is off rather than unsuccessful.
	const unknown = "\u0e01\u0e02\u0e04"
	if got := marks(t, unknown, WordBreak{Manual: true}, LineBreak{}); got != unknown {
		t.Errorf("under manual %q breaks as %q, want no opportunity in it",
			unknown, got)
	}
	if wb, unhandled := WordBreakOf("manual"); !wb.Manual || unhandled != "" {
		t.Errorf(`WordBreakOf("manual") = %+v, %q; the value is read and not reported`,
			wb, unhandled)
	}
}

// TestAScriptWithAVocabularyIsNotReported.
//
// The finding is about a page that is wrong, and a page broken where its words
// are is not one. The other half — a script with no vocabulary here — is
// asserted in dictionary_test.go.
func TestAScriptWithAVocabularyIsNotReported(t *testing.T) {
	// Thai, Lao, Khmer and Burmese: the four class SA scripts ICU publishes a
	// word list for, and the four this engine carries.
	for _, r := range []rune{'\u0e01', '\u0e81', '\u1780', '\u1000'} {
		if !HasDictionary(r) {
			t.Errorf("%U has no word list here", r)
		}
		if _, ok := UnsupportedScript(r); ok {
			t.Errorf("%U is reported, and this engine can find its words", r)
		}
	}
	// And the rest of class SA, which has no word list to have: Tai Tham,
	// Tai Le, Tai Viet, Myanmar Extended-B. The finding is theirs now.
	for _, r := range []rune{'\u1a20', '\u1950', '\uaa80', '\ua9e0'} {
		if HasDictionary(r) {
			t.Errorf("%U has a word list here; the finding below would be wrong", r)
		}
		if _, ok := UnsupportedScript(r); !ok {
			t.Errorf("%U is not reported, and this engine cannot find its words", r)
		}
	}
}
