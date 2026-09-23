package html

import "math"

// HTML's two rules for reading a number out of an attribute's text, §2.3.4.1
// and §2.3.4.2, which every attribute holding an integer is read by: a
// control's cols, rows and size, a canvas's width and height, a cell's colspan
// and rowspan, a table's border, a list's start.
//
// They are here, once, because they were written five times across the layout
// and style packages and three of those were strconv.Atoi, which is a
// different rule: it refuses "40px", " 7" and "30.5", where HTML reads forty,
// seven and thirty. "<textarea cols=40px>" was twenty columns wide, the
// default, for a reason nobody reading the attribute would guess (audit C135).
//
// Both rules read the digits at the front and ignore whatever follows them.
// That is what makes a value with a unit or a fraction in it a number rather
// than an error, and it is the whole difference from Atoi.

// MaxInteger is where a value read by these rules stops growing. Every reader
// of an attribute bounds what it will lay out far below this; what this bounds
// is the arithmetic, so that a thousand digits are not a thousand
// multiplications into an overflow. A value past it is still a value — past
// every caller's own limit, which is what a caller then reports.
const MaxInteger = math.MaxInt32

// ParseInteger is §2.3.4.1's rules for parsing integers: ASCII white space is
// skipped, then an optional "-" or "+", then at least one ASCII digit, and the
// digits are the value — anything after them is ignored. It reports false
// where there is no digit to read. A value past MaxInteger in either direction
// comes back as MaxInteger or its negation.
func ParseInteger(s string) (int, bool) {
	i := 0
	for i < len(s) && isASCIIWhitespace(s[i]) {
		i++
	}
	negative := false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		negative = s[i] == '-'
		i++
	}
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return 0, false
	}
	n := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		if n > (MaxInteger-int(s[i]-'0'))/10 {
			// Saturated. The digits after this one cannot bring it back, and
			// are not read.
			n = MaxInteger
			break
		}
		n = n*10 + int(s[i]-'0')
	}
	if negative {
		return -n, true
	}
	return n, true
}

// ParseNonNegativeInteger is §2.3.4.2's rules for parsing non-negative
// integers: §2.3.4.1's rules, and an error for a value below zero. "-0" is
// zero, which is not below zero.
func ParseNonNegativeInteger(s string) (int, bool) {
	n, ok := ParseInteger(s)
	if !ok || n < 0 {
		return 0, false
	}
	return n, true
}

// isASCIIWhitespace is Infra's ASCII white space: tab, line feed, form feed,
// carriage return and space.
func isASCIIWhitespace(c byte) bool {
	return c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}
