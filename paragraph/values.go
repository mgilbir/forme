package paragraph

import (
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

// ParseNumber reads a bare number, which line-height accepts as a multiplier.
func ParseNumber(s string) (float64, bool) {
	var v float64
	var seenDigit, seenDot bool
	frac := 0.1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			if seenDot {
				v += float64(c-'0') * frac
				frac /= 10
			} else {
				v = v*10 + float64(c-'0')
			}
		case c == '.' && !seenDot:
			seenDot = true
		default:
			return 0, false
		}
	}
	return v, seenDigit
}

// strconvFormat renders a length for a diagnostic, to a tenth of a pixel — more
// precision than that is noise in a message a person reads.
func strconvFormat(v float64) string {
	return strconv.FormatFloat(float64(int(v*10+0.5))/10, 'f', -1, 64)
}
