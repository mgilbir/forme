package paragraph

import "sort"

// Where a line may not begin — UAX #14's unconditional prohibitions.
//
// The break opportunities this package offers come from CSS Text §5: between
// typographic character units under break-all, around every one under
// line-break: anywhere, and — the one that matters here — between ideographs,
// which is what lets CJK wrap without spaces. UAX #14 is what says those
// opportunities are not all real.
//
// A line may not begin with a closing bracket. Nor with a full stop, an
// exclamation mark, a hyphen, or one of the marks Japanese calls a non-starter:
// the iteration marks, the small kana, the sound marks. Every one of them
// belongs to the text before it, and a line that starts with one reads as a
// mistake even to someone who cannot read the language — which is why it is the
// first thing the suite's line-break tests check, ninety-three times over.
//
// # What this is not
//
// It is not UAX #14. The algorithm is thirty pair rules over a class table, and
// most of them depend on what came *before* the break as well as after it: a
// quotation mark may or may not begin a line depending on what quoted it, and a
// numeric separator depends on whether a number surrounds it. Those cannot be
// answered by looking at one character, and this does not pretend to.
//
// What it is, is the subset that can: the rules written "× X" with nothing on
// the left. Everything else CSS Text needs is elsewhere in this package, where
// the characters it concerns — the spaces, the segment breaks, the tabs — are
// already handled one at a time.
//
// # Why the opposite question needs no table
//
// A line may not *end* with an opening bracket either, which is UAX #14's LB14.
// There is nothing here for it, and nothing is missing: the opportunity after an
// ideograph is deferred until the following character is known, so a break after
// an opening bracket is one that was never offered rather than one withdrawn.
// A bracket is not an ideograph, so it defers nothing.
func noBreakBefore(r rune) bool {
	// Below the first range and the common case for Latin text, which is worth
	// a comparison to avoid a search.
	if r < noBreakBeforeRanges[0].lo {
		return false
	}
	i := sort.Search(len(noBreakBeforeRanges), func(i int) bool {
		return noBreakBeforeRanges[i].hi >= r
	})
	return i < len(noBreakBeforeRanges) && noBreakBeforeRanges[i].lo <= r
}

// BindsToAtomicInline reports whether a line may not break between this
// character and an atomic inline beside it.
//
// CSS Text §5.1, in full because the exception is the interesting part: "For
// Web-compatibility there is a soft wrap opportunity before and after each
// replaced element or other atomic inline, even when adjacent to a character
// that would normally suppress them, including U+00A0 NO-BREAK SPACE. However,
// with the exception of U+00A0 NO-BREAK SPACE, there must be no soft wrap
// opportunity between atomic inlines and adjacent characters belonging to the
// Unicode GL, WJ, or ZWJ line breaking classes."
//
// So a picture may be wrapped away from the word next to it — that is the
// Web-compatibility half, and it is why an atomic inline is not simply glued to
// whatever precedes it — and may not be wrapped away from a word joiner, a
// narrow no-break space, or a Tibetan delimiter. A no-break space is class GL
// and breaks anyway, which is the sentence's own exception and the reason this
// is a function rather than a table lookup.
//
// It is exported because the boundary it is about is not in this package. A
// piece of text and an atomic inline are two different things in the layout,
// and only the code that lays a line out sees them next to each other.
func BindsToAtomicInline(r rune) bool {
	// The exception, which the table cannot carry: U+00A0 is class GL and this
	// rule does not apply to it.
	if r == 0x00A0 {
		return false
	}
	if r < bindingRanges[0].lo {
		return false
	}
	i := sort.Search(len(bindingRanges), func(i int) bool {
		return bindingRanges[i].hi >= r
	})
	return i < len(bindingRanges) && bindingRanges[i].lo <= r
}
