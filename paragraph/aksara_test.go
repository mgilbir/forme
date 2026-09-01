package paragraph

import "testing"

// Where a line may break in a Brahmic script.
//
// UAX #14's LB28a is four prohibitions *inside* an aksara cluster — between a
// pre-base repha and the letter it belongs to, between a letter and its final
// vowel, and across a virama — and it says nothing against a break between two
// clusters, where LB31's "ALL ÷ ALL" allows one. The scripts the classes cover
// write without spaces, so that boundary is the only opportunity their text
// has. Without it a paragraph of Javanese or Balinese is one unbreakable run
// and overflows its box, which CSS Text §5.1 forbids in as many words: "some
// form of fallback line breaking must occur ... overflowing is not allowed".
//
// The suite's line-breaking-023 is a Javanese paragraph in six ems beside a
// reference it must *not* match — it passes only if the text wraps at all.

// TestALineMayBreakBetweenTwoAksaras.
func TestALineMayBreakBetweenTwoAksaras(t *testing.T) {
	for _, tc := range []struct{ what, text, want string }{
		{"two Javanese letters", "ꦲꦏ", "ꦲ|ꦏ"},
		{"a Javanese letter and its vowel sign", "ꦲꦾꦏ", "ꦲꦾ|ꦏ"},
		{"three Balinese letters", "ᬳᬓᬮ", "ᬳ|ᬓ|ᬮ"},
		{"a Cham letter", "ꨀꨁ", "ꨀ|ꨁ"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %q breaks as %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestALineDoesNotBreakInsideAnAksara is LB28a's own half, and it needs no
// table: a cluster is a grapheme cluster, Unicode 15.1's GB9c keeps a conjunct
// together, and SplitAtBreaks takes an opportunity only at a cluster boundary.
func TestALineDoesNotBreakInsideAnAksara(t *testing.T) {
	for _, tc := range []struct{ what, text string }{
		{"a Javanese conjunct", "ꦏ꧀ꦲ"},
		{"a Balinese conjunct", "ᬳ᭄ᬳ"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.text {
			t.Errorf("%s: %q breaks as %q, want no opportunity in it — the "+
				"virama joins the two letters into one cluster",
				tc.what, tc.text, got)
		}
	}
	// And the boundary in front of such a cluster is still there, so the row
	// above is about the virama and not about the rule being off.
	if got := marks(t, "ꦱꦏ꧀ꦲ", WordBreak{}, LineBreak{}); got != "ꦱ|ꦏ꧀ꦲ" {
		t.Errorf("a letter before a conjunct breaks as %q, want %q", got, "ꦱ|ꦏ꧀ꦲ")
	}
}

// TestKeepAllSuppressesTheAksaraBoundary, which §5.2 asks for by name: the
// value forbids the implicit opportunities between typographic letter units,
// and this is one.
func TestKeepAllSuppressesTheAksaraBoundary(t *testing.T) {
	const text = "ꦲꦏ"
	if got := marks(t, text, WordBreak{KeepAll: true}, LineBreak{}); got != text {
		t.Errorf("under keep-all %q breaks as %q, want no opportunity in it",
			text, got)
	}
}

// TestAScriptThatWritesWithSpacesIsUntouched. Devanagari and the Latin
// alphabet are class AL, not AK, so nothing here reaches them — a rule that
// broke every cluster of every script would cut Hindi and English words in
// half.
func TestAScriptThatWritesWithSpacesIsUntouched(t *testing.T) {
	for _, text := range []string{"abcdef", "एकदो", "ᠮᠣᠩᠭᠣᠯ"} {
		if got := marks(t, text, WordBreak{}, LineBreak{}); got != text {
			t.Errorf("%q breaks as %q, want no opportunity in it", text, got)
		}
	}
}
