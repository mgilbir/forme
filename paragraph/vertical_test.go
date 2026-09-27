package paragraph

import "testing"

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
