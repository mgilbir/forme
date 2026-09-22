package paragraph

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
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
// # Why this is not handed to strconv.ParseFloat
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
//
// # And why it is not written out here either
//
// It was, and the arithmetic made the NaN the spelling had been kept from
// making. The digits were accumulated in a float and multiplied by a power of
// ten, so "0e400" was nought times an infinity, and a mantissa of three hundred
// and ten digits under "e-400" was an infinity times nought: NaN both ways, and
// "opacity: 0e400" reached the page as one. CSS says "0e400" is nought.
//
// So the reading is css.ParseNumber's, which the tokenizer's own numbers go
// through as well: one grammar and one conversion for every <number> in the
// engine, and a value that is never NaN.
//
// What is still decided here is the one thing the two kinds of reader decide
// differently. A number past the largest float64 is refused rather than
// clamped: this decides whether there is a declaration at all, and an infinite
// multiplier reaches arithmetic that clamps it out of sight, so the page would
// come out set on a number nobody wrote. The tokenizer clamps instead, because
// it has to hand back a token whatever the text says.
//
// And a negative zero comes back as zero. §4.3.13 keeps the sign, and it is
// meaningful inside a math function, which is not where any caller of this is;
// everywhere else a -0 is a 0 that a division turns into the wrong infinity.
func ParseNumber(s string) (float64, bool) {
	v, inRange, ok := css.ParseNumber(s)
	if !ok || !inRange {
		return 0, false
	}
	if v == 0 {
		return 0, true
	}
	return v, true
}

// strconvFormat renders a length for a diagnostic, to a tenth of a pixel — more
// precision than that is noise in a message a person reads.
func strconvFormat(v float64) string {
	return strconv.FormatFloat(float64(int(v*10+0.5))/10, 'f', -1, 64)
}
