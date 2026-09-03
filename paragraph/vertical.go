package paragraph

import (
	"unicode"

	"github.com/mgilbir/forme/segment"
)

// UAX #50, Unicode Vertical Text Layout: which way a character faces on a line
// of vertical text.
//
// One question is asked of it, and it is asked by a gate rather than by a
// typesetter. This engine sets vertical text by turning a horizontal page
// ninety degrees clockwise, which rotates every character on it; that is what
// "text-orientation: mixed" asks for wherever a character's Vertical_Orientation
// is R, and it is not what mixed asks for anywhere else. So the engine needs to
// know whether a paragraph contains a character it would draw wrong, and this is
// how it knows. See layout/writingmode.go for what is done with the answer and
// cmd/genvertical for how the table is built.

// IsUpright reports whether a character stands upright on a line of vertical
// text — UAX #50's U, or Tu falling back to it.
//
// A character this returns true for cannot be set by rotating a horizontal line,
// so a block containing one is not a block this engine can turn.
func IsUpright(r rune) bool { return inLineBreakRanges(r, uprightRanges[:]) }

// HasUprightText reports whether any character in a string stands upright.
//
// One character is enough. A paragraph of Latin with a single ideograph in it
// is a paragraph with an upright character in it, and turning the page would
// lay that one character on its side — a difference of exactly one glyph, which
// is a difference a reader of Japanese sees immediately and a reftest sees at
// all.
func HasUprightText(s string) bool {
	for _, r := range s {
		if IsUpright(r) {
			return true
		}
	}
	return false
}

// OrientationMix reports which of the two orientations the characters of a run
// need under "text-orientation: mixed": whether any of them stands upright, and
// whether any of them lies along the line.
//
// Both, and not one, because the answer that matters is whether the run needs
// *both* — that is the one a quarter turn cannot draw, and it is the only one
// worth refusing. A run that is entirely upright is a run this engine can set,
// by setting it the way "text-orientation: upright" asks for; a run with
// nothing upright in it is the turn itself.
//
// A character that marks no paper is skipped, which is what makes the answer
// about the picture rather than about the string. The orientation of a space is
// unobservable — it is blank whichever way up it is — and counting one would
// make "日本 と" a mixture and refuse a page that has only one orientation on it.
func OrientationMix(text string) (upright, rotated bool) {
	for _, r := range text {
		if unicode.IsSpace(r) || MarksNoPaper(r) || IsDefaultIgnorable(r) {
			continue
		}
		if IsUpright(r) {
			upright = true
			continue
		}
		rotated = true
	}
	return upright, rotated
}

// UprightUnits counts the characters of a run that take an advance when the run
// is set upright.
//
// One em each, and the count is what the width is made of — see
// Breaker.MeasureSpacedInContext. What is left out is what takes no room in any
// mode: a character nothing is drawn for, and a combining mark, which is drawn
// on the character in front of it rather than after it.
//
// The unit is the grapheme cluster, which is what §4.4 and CSS Text §2 both
// mean: a Thai letter with a vowel sign on it stands upright as one character
// and takes one em, not two. It is the same unit SpacedUnits counts for
// letter-spacing, asked of the same text and answered the same way — the two
// rules are one definition and would be a bug apart.
func UprightUnits(text string) int {
	n := 0
	for i, start := 0, 0; start < len(text); i++ {
		end := len(text)
		if bounds := segment.Boundaries(nil, text); i < len(bounds) {
			end = bounds[i]
		}
		for _, r := range text[start:end] {
			if IsDefaultIgnorable(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
				continue
			}
			n++
			break
		}
		start = end
	}
	return n
}
