package paragraph

import (
	"testing"
	"unicode"
)

// TestFoldCaseIsDefaultCaseFolding is CaseFolding.txt's status C and F, on the
// cases that tell folding from lowercasing, and from a folding with a tailoring
// or a normalization in it — each of which CSS Fonts 4 §5.1 rules out for a
// font family name.
func TestFoldCaseIsDefaultCaseFolding(t *testing.T) {
	for _, tc := range []struct{ in, want, why string }{
		{"Helvetica", "helvetica", "ASCII"},
		{"helvetica", "helvetica", "already folded"},
		{"Straße", "strasse", "ß folds to ss (F), where the lowercase keeps it"},
		{"STRASSE", "strasse", "and SS meets it"},
		{"ẞ", "ss", "the capital sharp s folds to ss too (F, not its S)"},
		{"\u212Aelvin", "kelvin", "the KELVIN SIGN folds to k (C)"},
		{"\u212B", "å", "the ANGSTROM SIGN folds to å (C)"},
		{"ΣΟΦΟΣ", "σοφοσ", "capital sigma folds to σ"},
		{"σοφος", "σοφοσ", "and final ς folds to σ as well"},
		{"İstanbul", "i\u0307stanbul", "İ folds to i and a combining dot (F), not to i"},
		{"ISTANBUL", "istanbul", "I folds to i: no Turkic tailoring (T is left out)"},
		{"ıstanbul", "ıstanbul", "dotless ı has no folding"},
		{"ﬁne", "fine", "the ligature folds to its letters (F)"},
		{"ŉ", "ʼn", "a character that folds to two"},
		{"a\u030A", "a\u030A", "no normalization: a and a combining ring stay two"},
		{"å", "å", "and å stays one"},
		{"Ꭰ", "Ꭰ", "Cherokee folds to its capitals, which are already these"},
		{"ꭰ", "Ꭰ", "and its small letters to them"},
		{"\U00010D50", "\U00010D70", "Garay, from Unicode 16, which Go 1.26 does not know"},
		{"a\xffB", "a\uFFFDb", "a byte that is not UTF-8"},
		{"", "", "nothing"},
	} {
		if got := FoldCase(tc.in); got != tc.want {
			t.Errorf("%s: FoldCase(%q) = %q, want %q", tc.why, tc.in, got, tc.want)
		}
	}
	// Nothing to fold comes back as it went in.
	for _, s := range []string{"times", "σοφοσ", "日本語", "ıi"} {
		if got := FoldCase(s); got != s {
			t.Errorf("FoldCase(%q) = %q; it has nothing to fold", s, got)
		}
	}
}

// TestFoldingAgreesWithTheToolchainsOrbits holds the table to a second source.
// Go's unicode.SimpleFold walks the characters that are the same letter in
// another case, from the toolchain's release: a folding makes every member of
// such a set into the same string, or it is not a case folding. Case pairs are
// never unmade (Unicode's stability policy), so every set the older release
// knows must fold together under the newer table.
func TestFoldingAgreesWithTheToolchainsOrbits(t *testing.T) {
	sets := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		next := unicode.SimpleFold(r)
		if next == r || next < r {
			// Visit each set once, from its smallest member.
			continue
		}
		smallest := true
		for o := next; o != r; o = unicode.SimpleFold(o) {
			if o < r {
				smallest = false
				break
			}
		}
		if !smallest {
			continue
		}
		sets++
		want := FoldCase(string(r))
		for o := next; o != r; o = unicode.SimpleFold(o) {
			if got := FoldCase(string(o)); got != want {
				t.Errorf("U+%04X and U+%04X are one letter to Go (Unicode %s), and fold to "+
					"%q and %q", r, o, unicode.Version, want, got)
			}
		}
	}
	if sets < 1400 {
		t.Fatalf("only %d case sets were walked", sets)
	}
}
