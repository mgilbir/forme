package paragraph

import (
	"strings"
	"testing"
)

// SplitAtWordSeparators, and the property that matters is where the cut falls:
// after the separator, because §8.3's spacing goes after it and a run's width
// can only say what is at its end.

func TestTheSeparatorEndsItsPiece(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"a b", []string{"a ", "b"}},
		{"ab cd ef", []string{"ab ", "cd ", "ef"}},
		{"a b", []string{"a ", "b"}},
		{" b", []string{" ", "b"}},
		// The other four separators §8.3 names, which an author of Ethiopic,
		// Aegean, Ugaritic or Phoenician writes instead of a space.
		{"a፡b", []string{"a፡", "b"}},
		{"a\U00010100b", []string{"a\U00010100", "b"}},
		{"a\U0001039Fb", []string{"a\U0001039F", "b"}},
		{"a\U0001091Fb", []string{"a\U0001091F", "b"}},
	} {
		got := SplitAtWordSeparators(tc.text)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%q split to %q, want %q", tc.text, got, tc.want)
		}
		if joined := strings.Join(got, ""); joined != tc.text {
			t.Errorf("%q split to %q, which joins back to %q", tc.text, got, joined)
		}
	}
}

// TestNothingIsCutWhereNothingWouldChange. A run with no separator in it, and a
// run whose separator is already its last character, are both whole — the
// spacing after a trailing separator is already at the end of the width.
//
// Returning nothing rather than a one-element slice is what the caller tests, so
// this is the difference between cutting every run of every document with
// word-spacing on it and cutting the ones that need it.
func TestNothingIsCutWhereNothingWouldChange(t *testing.T) {
	for _, text := range []string{"", "abc", "a ", "abc ", " ", "፡"} {
		if got := SplitAtWordSeparators(text); got != nil {
			t.Errorf("%q split to %q, and there is nothing in it to cut", text, got)
		}
	}
}

// TestTheCutAgreesWithTheCount. The two halves of §8.3 in this file have to say
// the same thing about which characters they are: a character the count charges
// spacing for and the cut does not end a run on is one whose spacing is measured
// and never drawn, which is the bug this exists to prevent.
func TestTheCutAgreesWithTheCount(t *testing.T) {
	for _, r := range []rune{' ', ' ', '፡', '\U00010100', '\U00010101',
		'\U0001039F', '\U0001091F', 'a', '\t', ' ', '　'} {
		text := "x" + string(r) + "y"
		counted := countWordSeparators(text) == 1
		cut := len(SplitAtWordSeparators(text)) == 2
		if counted != cut {
			t.Errorf("%U: counted=%v but cut=%v; the spacing charged for a character "+
				"is the spacing that has to reach the end of a run", r, counted, cut)
		}
	}
}
