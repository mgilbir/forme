package css

import (
	"math"
	"strconv"
	"strings"
)

// Reading a <number>, once, for everything that reads one.
//
// CSS Syntax §4.3.12 says which text is a number and §4.3.13 what number it is,
// and there are two kinds of reader in this engine: the tokenizer, which meets
// the text in a stylesheet, and every reader that starts from a string the
// cascade kept — a line-height multiplier, an opacity, a ratio, an SVG length.
// They used to answer the second question two ways. The tokenizer handed the
// text to strconv.ParseFloat; the string readers accumulated the digits in a
// float and multiplied by a power of ten, which is how "0e400" came out as
// NaN — nought times an infinity — and how a mantissa of three hundred and ten
// digits did the same the other way round. A NaN is the one value every guard
// written against a nonsense number lets through, because it is neither
// greater nor less than anything.
//
// Both now ask this.

// ParseNumber reads s as exactly one <number> — "[+-]? [digits] [. digits]
// [e [+-]? digits]" with a digit somewhere before the exponent — and nothing
// else: no white space around it, no unit, and none of the spellings Go's own
// number syntax has and CSS's does not ("nan", "inf", "0x1p3", "1_0").
//
// ok says s is a number at all. inRange says its value is one a float64 can
// hold: a number whose magnitude is past the largest one comes back as the
// infinity of its sign with inRange false, and what to do about it is the
// caller's — the tokenizer clamps it, because it has to hand back a token
// whatever the text says, and a reader deciding whether a declaration exists
// may refuse it. Nothing this returns is ever NaN.
func ParseNumber(s string) (value float64, inRange, ok bool) {
	n, end := scanNumber(s)
	if end == 0 || end != len(s) {
		return 0, false, false
	}
	if v, in, short := shortValue(s); short {
		return v, in, true
	}
	value, inRange = n.value()
	return value, inRange, true
}

// maxShortNumber is the longest text shortValue hands to ParseFloat as it
// stands.
//
// ParseFloat rounds a text of this length exactly: its inexactness is only in
// how many of a very long mantissa's digits it counts into the exponent, which
// is hundreds of digits away from here. So a number the length of anything
// written by hand is read straight from the text, with no normal form built —
// which is an allocation per number, and the tokenizer's fast path is
// otherwise allocation-free — and only a text past it takes the careful road.
const maxShortNumber = 64

// shortValue is value's answer for a short text, read by ParseFloat directly.
// s must already be a number's text and nothing else — scanNumber's whole
// match — which is what makes Go's own number syntax ("nan", "inf", "0x1p3",
// "1_0") unreachable here: none of it is a CSS number, so none of it gets this
// far. short is false for a text too long to be read this way.
func shortValue(s string) (value float64, inRange, short bool) {
	if len(s) > maxShortNumber {
		return 0, false, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if math.IsInf(v, 0) {
		return v, false, true
	}
	if err != nil {
		// A range error that is not an overflow is an underflow, and
		// ParseFloat's answer for it is the nought of its sign, which is
		// §4.3.13's too.
		return v, true, true
	}
	return v, true, true
}

// NumberValue is §4.3.13's value of a number the tokenizer has already read,
// with ParseNumber's inRange. repr must be a number's text and nothing else.
func NumberValue(repr string) (value float64, inRange bool) {
	n, end := scanNumber(repr)
	if end == 0 || end != len(repr) {
		// Not a number's text. The tokenizer never hands one over; a caller
		// that did gets nought rather than a value read from part of it.
		return 0, true
	}
	if v, in, short := shortValue(repr); short {
		return v, in
	}
	return n.value()
}

// number is a <number> taken apart: its sign, its digits with the decimal
// point's position among them, and its exponent.
type number struct {
	negative bool
	// intDigits and fracDigits are the digits before and after the decimal
	// point, each a slice of the text read. They are joined only by value,
	// on the road a short number never takes, so that reading one allocates
	// nothing.
	intDigits, fracDigits string
	// exponent is the written exponent, saturated far past anything that
	// could change the answer — see maxExponent.
	exponent int64
}

// maxExponent is where an exponent stops being accumulated. A float64 spans
// about 10^-324 to 10^308, and the digits before the point can move the
// exponent by as many places as the text is long, so a bound past both is the
// same answer as no bound: 10^12 is further than any text this reads.
const maxExponent = 1e12

// scanNumber reads a number from the start of s, §4.3.12's way, and returns it
// and where it ended: zero where there is none.
//
// A dot counts only with a digit after it, and an exponent only with a digit
// after its "e" and optional sign — so "1." and "1e" end after the "1", which
// is the tokenizer's answer and makes them not numbers when the whole string
// has to be one.
func scanNumber(s string) (number, int) {
	var n number
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		n.negative = s[i] == '-'
		i++
	}
	start := i
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	intDigits := s[start:i]
	fracDigits := ""
	if i+1 < len(s) && s[i] == '.' && isASCIIDigit(s[i+1]) {
		i++
		f := i
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
		}
		fracDigits = s[f:i]
	}
	if intDigits == "" && fracDigits == "" {
		return number{}, 0
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		negExp := false
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			negExp = s[j] == '-'
			j++
		}
		if j < len(s) && isASCIIDigit(s[j]) {
			var e int64
			for j < len(s) && isASCIIDigit(s[j]) {
				if e < maxExponent {
					e = e*10 + int64(s[j]-'0')
				}
				j++
			}
			if negExp {
				e = -e
			}
			n.exponent = e
			i = j
		}
	}
	n.intDigits, n.fracDigits = intDigits, fracDigits
	return n, i
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// value is the number §4.3.13 says the parts are, rounded once to the nearest
// float64, and whether that float64 is finite.
//
// It is strconv.ParseFloat that rounds, and it is handed a normal form of the
// number — "0.<significant digits>e<exponent>" — rather than the author's text,
// because ParseFloat's own reading of a long text is not exact: it counts the
// digits before the point into the exponent only up to a bound, so a mantissa
// of a hundred thousand digits under an exponent of minus a hundred thousand —
// which is 0.111… — came out as nought. In the normal form there are no digits
// before the point and the exponent is the whole of the magnitude, which
// ParseFloat saturates correctly to nought or an infinity however large it is.
func (n number) value() (float64, bool) {
	sign := ""
	if n.negative {
		sign = "-"
	}
	digits := n.intDigits + n.fracDigits
	d := strings.TrimLeft(digits, "0")
	// The point moves left by the zeros taken off the front: 0.0012 is 0.12
	// times 10^-2.
	point := int64(len(n.intDigits)) - int64(len(digits)-len(d))
	// Nought is "0.e<exponent>", which ParseFloat reads as nought whatever the
	// exponent: 0e400 is 0, and not the 0·10^400 that was NaN. The sign is
	// kept, because §4.3.13 keeps it.
	d = strings.TrimRight(d, "0")
	v, err := strconv.ParseFloat(sign+"0."+d+"e"+strconv.FormatInt(point+n.exponent, 10), 64)
	if math.IsInf(v, 0) {
		// Past the largest float64.
		return v, false
	}
	if err != nil {
		// Not reachable: the text is a sign, digits and an exponent built just
		// above, and a range error is the only one ParseFloat has for such
		// text. Nought rather than whatever came back, so that even a fault
		// here cannot hand a caller a value nobody wrote.
		return 0, true
	}
	return v, true
}
