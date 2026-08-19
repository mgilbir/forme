package paragraph

import "testing"

// CSS's <number> carries a sign.
//
// The grammar is "[+|-]? [digits] [. digits]?", and the sign is part of it. This
// read the digits and refused the sign, so "line-height: +5" was not a number at
// all and the property fell back to normal — a fifth of the height the author
// asked for, from a declaration that is valid CSS and that the suite writes
// exactly that way.
//
// The range belongs to the property and not to the parser. line-height and
// tab-size must both be non-negative and say so themselves; a parser that
// enforced the commonest range would be wrong for the next caller rather than
// silent about it.
func TestParseNumberTakesASign(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
		ok   bool
	}{
		{"5", 5, true},
		{"+5", 5, true},
		{"-5", -5, true},
		{"5.5", 5.5, true},
		{"+5.5", 5.5, true},
		{"-5.5", -5.5, true},
		{"+.5", 0.5, true},
		{"-.5", -0.5, true},
		{"0", 0, true},
		{"+0", 0, true},
		// A sign and nothing else is not a number.
		{"+", 0, false},
		{"-", 0, false},
		{"", 0, false},
		// The sign leads, and only once.
		{"5+", 0, false},
		{"++5", 0, false},
		{"+-5", 0, false},
		{"5-5", 0, false},
		// Still not a number for the other reasons.
		{"5px", 0, false},
		{"abc", 0, false},
		// Two dots is not a number, and the second one is what refuses it
		// rather than the digits after it.
		{"5.5.5", 0, false},
	} {
		got, ok := ParseNumber(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
