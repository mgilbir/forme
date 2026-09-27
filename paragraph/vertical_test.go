package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
)

// The table is Unicode's statement and this is the check that it says what
// UAX #50 says, at the handful of characters the writing-mode gate turns on.
//
// The Latin row is the one that matters most: it is what makes a paragraph of
// English turnable at all, and a table that called it upright would leave the
// feature reporting every document it was written for.
func TestWhichCharactersStandUprightInVerticalText(t *testing.T) {
	for _, c := range []struct {
		r       rune
		upright bool
		what    string
	}{
		{'A', false, "Latin capital A, class R"},
		{'z', false, "Latin small z, class R"},
		{'7', false, "digit seven, class R"},
		{' ', false, "space, class R"},
		{0x00AD, false, "soft hyphen, class R"},
		{0x2010, false, "hyphen, class R"},
		{0x05D0, false, "Hebrew alef, class R"},
		{0x0627, false, "Arabic alef, class R"},
		{0x3042, true, "hiragana a, class U"},
		{0x4E00, true, "the ideograph one, class U"},
		{0xAC00, true, "Hangul syllable ga, class U"},
		{0xFF21, true, "fullwidth Latin capital A, class U"},
		{0x201C, false, "left double quotation mark, class Tr: a face may set a " +
			"vertical form, and without one it lies down"},
		{0x3008, false, "left angle bracket, class Tr, and for the same reason"},
		{0x00A9, true, "copyright sign, class U"},
		{0x3001, true, "ideographic comma, class Tu: upright without a vertical form"},
		{0x3041, true, "hiragana small a, class Tu"},
		{0x20000, true, "an unassigned CJK extension code point, upright by the header's default"},
	} {
		if got := IsUpright(c.r); got != c.upright {
			t.Errorf("IsUpright(%#04x) = %v, want %v: %s", c.r, got, c.upright, c.what)
		}
	}
	if HasUprightText("hyphenation") {
		t.Error("a word of English has an upright character in it")
	}
	if !HasUprightText("hyphen一ation") {
		t.Error("a word with one ideograph in it does not")
	}
	if HasUprightText("") {
		t.Error("the empty string does")
	}
}

// TestUprightUnitStartsAreTheUnitsCounted: the starts are where the characters
// UprightUnits counts begin, and there are as many as it counts — a mark and a
// default ignorable are in the cluster before them or in none.
func TestUprightUnitStartsAreTheUnitsCounted(t *testing.T) {
	for _, c := range []struct {
		text string
		want []int
	}{
		{"", nil},
		{"abc", []int{0, 1, 2}},
		{"ab́cd", []int{0, 1, 4, 5}},
		{"a‍b", []int{0, 4}},
		{"́a", []int{2}},
		{"กิx", []int{0, 6}},
	} {
		got := AppendUprightUnitStarts(nil, c.text)
		if len(got) != len(c.want) {
			t.Errorf("%q: starts %v, want %v", c.text, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: starts %v, want %v", c.text, got, c.want)
				break
			}
		}
		if n := UprightUnits(c.text); n != len(got) {
			t.Errorf("%q: %d starts and UprightUnits counts %d", c.text, len(got), n)
		}
	}
}

// TestMixedOrientationIsCutBetweenClusters.
//
// "text-orientation: mixed" asks UAX #50 of each typographic character unit,
// and the run is cut wherever the answer changes. What is pinned is where the
// cuts fall and what each part is asked:
//
//   - a Latin word beside two ideographs is three parts;
//   - a combining mark goes with the character in front of it, and the cluster
//     faces the way its first character does, which is §3.2.1's "a base
//     character upright and a combining mark attached to it sideways" that
//     does not make sense;
//   - an enclosing mark stands its whole cluster up, whatever the base is —
//     the keycap digit;
//   - a space is its own character: U+0020 lies down between two ideographs,
//     and the ideographic space U+3000 stands with them.
func TestMixedOrientationIsCutBetweenClusters(t *testing.T) {
	for _, c := range []struct {
		text    string
		parts   []string
		upright []bool
	}{
		{"ab\u65e5\u672ccd", []string{"ab", "\u65e5\u672c", "cd"}, []bool{false, true, false}},
		{"e\u0301\u65e5", []string{"e\u0301", "\u65e5"}, []bool{false, true}},
		{"\u65e5\u3099a", []string{"\u65e5\u3099", "a"}, []bool{true, false}},
		// The acute is R on its own, and stands up with the ideograph it is on.
		{"\u65e5\u0301a", []string{"\u65e5\u0301", "a"}, []bool{true, false}},
		{"a1\u20e3b", []string{"a", "1\u20e3", "b"}, []bool{false, true, false}},
		{"\u65e5 \u672c", []string{"\u65e5", " ", "\u672c"}, []bool{true, false, true}},
		{"\u65e5\u3000\u672c", []string{"\u65e5\u3000\u672c"}, []bool{true}},
		{"abc", []string{"abc"}, []bool{false}},
		{"", []string{""}, []bool{false}},
	} {
		got := SplitAtOrientation(c.text)
		if len(got) != len(c.parts) {
			t.Errorf("SplitAtOrientation(%q) = %q, want %q", c.text, got, c.parts)
			continue
		}
		for i := range got {
			if got[i] != c.parts[i] {
				t.Errorf("SplitAtOrientation(%q) = %q, want %q", c.text, got, c.parts)
				break
			}
			if u := UprightInMixed(got[i]); u != c.upright[i] {
				t.Errorf("UprightInMixed(%q) = %v, want %v", got[i], u, c.upright[i])
			}
		}
	}
}

// TestSplittingAtOrientationIsLinearInTheRun.
//
// The cut is asked of every piece of text on a mixed vertical line, and a
// piece is as long as a word the document never breaks: a line of ideographs
// and Latin letters alternating is the case that cuts the most. Four times the
// text should cost four times as much; a split that searched for each cut from
// the start of the run would cost sixteen.
func TestSplittingAtOrientationIsLinearInTheRun(t *testing.T) {
	splitOf := func(n int) func() {
		text := strings.Repeat("\u65e5a", n)
		if got := len(SplitAtOrientation(text)); got != 2*n {
			t.Fatalf("%d alternations cut into %d parts, want %d", n, got, 2*n)
		}
		return func() { SplitAtOrientation(text) }
	}
	const small, large = 4000, 16000
	c := costtest.Time(t, "splitting n alternations", splitOf(small), splitOf(large))
	if c.Ratio > 8 {
		t.Errorf("splitting %d alternations took %v and %d took %v, a factor of %.1f "+
			"for four times the text", small, c.Small, large, c.Large, c.Ratio)
	}
}
