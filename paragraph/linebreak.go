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
