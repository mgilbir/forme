package paragraph

import (
	"math"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/style"
)

// Reading and writing the small values a line needs.
//
// A line-height that is a bare number, a line-clamp that is a count, and a length
// written into a finding. Each is a CSS value read without a cascade or written
// without a backend, which is why they sit here and not either side of it.

// PositiveInteger reads a whole number above zero, which is the only form of
// either clamp property this engine acts on.
func PositiveInteger(value string) (int, bool) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, false
	}
	// CSS's <integer> takes a leading sign, and "+3" is three. It was refused,
	// so "-webkit-line-clamp: +3" — which is an integer written the way the
	// grammar allows — clamped nothing.
	if s[0] == '+' {
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > maxClampLines {
			return maxClampLines, true
		}
	}
	return n, n > 0
}

func FmtPx(u style.Unit) string {
	return strconvFormat(u.Px()) + "px"
}

// ParseNumber reads CSS Syntax §4.3.3's <number> — which line-height accepts as
// a multiplier, and which every other reader that starts from a string rather
// than a token needs too.
//
// # Why this is written out rather than handed to strconv.ParseFloat
//
// ParseFloat is Go's number syntax, not CSS's, and it accepts four spellings
// CSS has no way to write: "nan", "inf" (and "infinity"), the hexadecimal float
// "0x1p3", and the underscore-separated "1_0". The tokenizer reads every one of
// them as an identifier, so a declaration holding one is invalid and dropped —
// but a reader that starts from a string never sees a token.
//
// **The NaN is the dangerous one, because it is the one each caller already
// tried to stop.** Every one of them has a guard against a nonsense number —
// "n < 0" for a percentage, "v <= 0" for a dimension, "wn <= 0" for a ratio —
// and not one of them stops a NaN, because a NaN is neither greater than nor
// less than anything and all three comparisons are false for it. The value that
// most needs refusing is the value that walks straight through.
//
// "aspect-ratio: nan" made a box thirty-three million pixels tall before six
// such readers were brought here: that is style.Unit saturating rather than any
// arithmetic the author asked for — the lengths are defended, FromPx refuses a
// NaN and Mul and Div clamp — but nothing refused the *declaration*, so a box
// was sized by a value nobody could have written, in silence.
func ParseNumber(s string) (float64, bool) {
	var v float64
	var seenDigit, seenDot bool
	frac := 0.1
	// CSS's <number> is "[+|-]? [digits] [. digits]?", and the sign is part of
	// it: "line-height: +5" is five, and the suite writes one exactly that way.
	// A caller that may not take a negative says so itself — every one of them
	// has a range of its own, and a parser that enforced the commonest one would
	// be wrong for the next caller rather than silent about it.
	sign := 1.0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	// And an exponent, which the grammar has and this did not: <number> is
	// "[+-]? [digits ['.' digits]? | '.' digits] [e [+-]? digits]?", so
	// "line-height: 1e2" is a hundred. It was refused as though the "e" were a
	// letter in the middle of a number.
	//
	// The digits after a dot are required too. "5." is not a number by the
	// grammar — there is no production for a dot with nothing after it — and
	// reading it as five accepted a value no browser does.
	digits := s
	exponent := 0.0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		digits = s[:i]
		exp, ok := exponentOf(s[i+1:])
		if !ok {
			return 0, false
		}
		exponent = exp
	}
	sawDotWithNothingAfter := false
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			sawDotWithNothingAfter = false
			if seenDot {
				v += float64(c-'0') * frac
				frac /= 10
			} else {
				v = v*10 + float64(c-'0')
			}
		case c == '.' && !seenDot:
			seenDot, sawDotWithNothingAfter = true, true
		default:
			return 0, false
		}
	}
	if !seenDigit || sawDotWithNothingAfter {
		return 0, false
	}
	n := sign * v * math.Pow(10, exponent)
	if math.IsInf(n, 0) {
		// The exponent bound below is coarse on purpose — it stops the loop
		// reading a thousand digits — and it is *inside* the range where the
		// answer stops existing: 400 passes it and 10^400 is an infinity, which
		// is how "line-height: 1e400" became a multiplier of +Inf. Nothing
		// downstream turns that back into a mistake, because every length
		// operation clamps: the page came out set on the largest line there is,
		// as though the author had asked for it.
		//
		// Refused rather than clamped, which is the same answer the bound gives
		// one digit later and the answer §4.2 gives a value a property cannot
		// take. The tokenizer clamps instead, and that is not a disagreement:
		// it has to hand back a token whatever the text says, and this decides
		// whether there is a declaration at all.
		return 0, false
	}
	return n, true
}

// exponentOf reads the digits after an "e", with their own optional sign.
func exponentOf(s string) (float64, bool) {
	sign := 1.0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}
	v := 0.0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + float64(c-'0')
		if v > 400 {
			// Past anything a length can be, and past what math.Pow returns a
			// finite answer for. A number this large is not a mistake to
			// compute carefully; it is one to refuse.
			return 0, false
		}
	}
	return sign * v, true
}

// strconvFormat renders a length for a diagnostic, to a tenth of a pixel — more
// precision than that is noise in a message a person reads.
func strconvFormat(v float64) string {
	return strconv.FormatFloat(float64(int(v*10+0.5))/10, 'f', -1, 64)
}
