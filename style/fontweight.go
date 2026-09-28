package style

import (
	"strconv"

	"github.com/mgilbir/forme/internal/ascii"
)

// Relative font weights: CSS Fonts 4 §2.2.1.
//
// "bolder" and "lighter" are not weights. They name a step from the parent's
// weight, and §2.2 makes font-weight's computed value "a number": the step is
// taken here, where the parent's computed style is at hand, and the number is
// what the element's style holds and what its children inherit.
//
// It was left as the keyword, and the keyword was inherited. Layout read
// "bolder" as bold and "lighter" as not, whatever the parent's weight — so
// "lighter" inside a black (900) heading, which §2.2.1's table computes to
// 700, came out regular, and "bolder" inside a bold paragraph, which it
// computes to 900, came out merely bold; and a child of either inherited the
// word and took the step again from nothing.
//
// Only the two relative keywords are rewritten. "normal" and "bold" are 400
// and 700 by definition and are left as written, since a reader of the
// computed style — layout, and the tests that ask what a rule set — takes
// either spelling.

// fontWeightID is font-weight's place in the registry.
var fontWeightID = registry.ids["font-weight"]

// resolveRelativeWeight rewrites a "bolder" or "lighter" in the style being
// built as the number it computes to against the parent's weight: the
// initial weight, 400, for an element with no parent.
func (s *Styler) resolveRelativeWeight(b *styleBuilder, parent ComputedStyle) {
	v := ascii.Lower(ascii.TrimCSSSpace(b.cs.Get("font-weight")))
	if v != "bolder" && v != "lighter" {
		return
	}
	inherited := 400.0
	if !parent.IsZero() {
		if w, ok := absoluteWeight(parent.Get("font-weight")); ok {
			inherited = w
		}
	}
	w := relativeWeight(inherited, v == "bolder")
	b.set(fontWeightID, s.interner().value(strconv.FormatFloat(w, 'f', -1, 64)))
}

// absoluteWeight reads a computed font-weight: a keyword or a number.
func absoluteWeight(value string) (float64, bool) {
	switch v := ascii.Lower(ascii.TrimCSSSpace(value)); v {
	case "normal":
		return 400, true
	case "bold":
		return 700, true
	default:
		n, err := strconv.ParseFloat(v, 64)
		// ParseFloat reads "nan", "inf" and hexadecimal, none of which is CSS;
		// the grammar has already refused them, and the range is the grammar's.
		if err != nil || !(n >= 1 && n <= 1000) {
			return 0, false
		}
		return n, true
	}
}

// relativeWeight is §2.2.1's table: the weight "bolder" (or "lighter")
// computes to over an inherited weight w.
//
//	inherited w        bolder     lighter
//	w < 100            400        no change
//	100 <= w < 350     400        100
//	350 <= w < 550     700        100
//	550 <= w < 750     900        400
//	750 <= w < 900     900        700
//	900 <= w           no change  700
func relativeWeight(w float64, bolder bool) float64 {
	switch {
	case w < 100:
		if bolder {
			return 400
		}
		return w
	case w < 350:
		if bolder {
			return 400
		}
		return 100
	case w < 550:
		if bolder {
			return 700
		}
		return 100
	case w < 750:
		if bolder {
			return 900
		}
		return 400
	case w < 900:
		if bolder {
			return 900
		}
		return 700
	default:
		if bolder {
			return w
		}
		return 700
	}
}
