package paragraph

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
