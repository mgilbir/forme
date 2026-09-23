package html

import (
	"strings"
	"testing"
)

// TestTheRulesForParsingIntegers is HTML §2.3.4.1 and §2.3.4.2, step by step:
// ASCII white space skipped, one sign, at least one digit, the digits the
// value and whatever follows them ignored.
func TestTheRulesForParsingIntegers(t *testing.T) {
	for _, tc := range []struct {
		in           string
		want         int
		ok           bool
		nonNegative  int
		nonNegativeK bool
	}{
		{"7", 7, true, 7, true},
		{"007", 7, true, 7, true},
		{"+7", 7, true, 7, true},
		{"-7", -7, true, 0, false},
		{"-0", 0, true, 0, true},
		{" \t\n\f\r7", 7, true, 7, true},
		{"40px", 40, true, 40, true},
		{"30.5", 30, true, 30, true},
		{"7 8", 7, true, 7, true},
		// What is not one.
		{"", 0, false, 0, false},
		{"   ", 0, false, 0, false},
		{"+", 0, false, 0, false},
		{"-", 0, false, 0, false},
		{"+-7", 0, false, 0, false},
		{"--7", 0, false, 0, false},
		{".5", 0, false, 0, false},
		{"x7", 0, false, 0, false},
		// A vertical tab is not ASCII white space, and a non-ASCII digit is
		// not an ASCII digit.
		{"\v7", 0, false, 0, false},
		{"٣", 0, false, 0, false},
		// Past the bound, a value still, saturated.
		{strings.Repeat("9", 400), MaxInteger, true, MaxInteger, true},
		{"-" + strings.Repeat("9", 400), -MaxInteger, true, 0, false},
		{"2147483647", MaxInteger, true, MaxInteger, true},
		{"2147483648", MaxInteger, true, MaxInteger, true},
		{"2147483646", MaxInteger - 1, true, MaxInteger - 1, true},
	} {
		got, ok := ParseInteger(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseInteger(%.20q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
		got, ok = ParseNonNegativeInteger(tc.in)
		if ok != tc.nonNegativeK || (ok && got != tc.nonNegative) {
			t.Errorf("ParseNonNegativeInteger(%.20q) = %d, %v; want %d, %v",
				tc.in, got, ok, tc.nonNegative, tc.nonNegativeK)
		}
	}
}
