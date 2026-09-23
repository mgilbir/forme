package paragraph

import (
	"math"
	"strings"
	"testing"
)

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

// TestParseNumberIsNeverNaN is audit C44: the spellings that made a NaN out of
// a number, when this reader accumulated the digits in a float and multiplied
// by a power of ten. "0e400" was nought times an infinity, and a mantissa too
// long for a float64 under "e-400" was an infinity times nought — and a NaN
// passes every "< 0" and "> 1" guard its callers wrote, so "opacity: 0e400"
// reached the page as one.
//
// The values are written down rather than compared with anything, because a
// comparison with a second reader passes when both are wrong.
func TestParseNumberIsNeverNaN(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
		ok   bool
	}{
		// Nought, whatever the exponent.
		{"0e400", 0, true},
		{"-0e400", 0, true},
		{"0e999999999999999999999", 0, true},
		{"0.000e-400", 0, true},
		// Below the smallest float64: nought, and a number. It was refused,
		// because the exponent's digits were bounded without regard to sign.
		{"1e-400", 0, true},
		{"1e-500", 0, true},
		// A mantissa past the largest float64 brought back by its exponent.
		{strings.Repeat("9", 400) + "e-400", 1, true},
		{strings.Repeat("1", 400) + "e-399", 1.1111111111111112, true},
		// Past the largest float64: refused, which is this reader's policy and
		// not the tokenizer's — see ParseNumber.
		{"1e400", 0, false},
		{"-1e400", 0, false},
		{strings.Repeat("9", 310), 0, false},
		{"1e99999999999999999999", 0, false},
	} {
		got, ok := ParseNumber(tc.in)
		name := tc.in
		if len(name) > 30 {
			name = name[:12] + "…" + name[len(name)-10:]
		}
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("ParseNumber(%q) = %v", name, got)
			continue
		}
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v, %v", name, got, ok, tc.want, tc.ok)
		}
		// A negative zero is a zero: nothing that reads this is inside a math
		// function, where the sign would mean something.
		if ok && got == 0 && math.Signbit(got) {
			t.Errorf("ParseNumber(%q) is a negative zero", name)
		}
	}
}
