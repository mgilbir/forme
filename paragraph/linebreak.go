package paragraph

import (
	"sort"
	"unicode/utf8"
)

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
func noBreakBefore(r rune, lb LineBreak) bool {
	// Below the first range and the common case for Latin text, which is worth
	// a comparison to avoid a search.
	if r < noBreakBeforeRanges[0].lo {
		return false
	}
	// §5.3's tailoring, which is three answers over the same base.
	//
	// normal is the base: UAX #14's unconditional prohibitions and nothing more,
	// which is what this engine did for every value before the property was
	// read. The other two move characters in and out of it, and both directions
	// are needed — strict forbids what normal allows and loose allows what
	// normal forbids — which is why this is a pair of tables rather than one.
	//
	// The hyphens are the exception the base table cannot state. 〜 and ゠ are
	// class NS, so they are in it; §5.3 says a line may begin with one under
	// normal and loose, and may not under strict. So the base is right for
	// strict and the two looser values carve them back out.
	switch {
	case lb.Loose && inLineBreakRanges(r, looseBreakRanges[:]):
		return false
	case isLatinHyphen(r):
		// U+2010 and U+2013. Loose lets a line begin with one and the other
		// three do not — which for auto is a prohibition UAX #14 does not have,
		// exactly as the postfixes below are. See isLatinHyphen.
		return !lb.Loose
	case (lb.Normal || lb.Loose) && isEastAsianHyphen(r):
		return false
	case lb.Strict && inLineBreakRanges(r, strictNoBreakRanges[:]):
		return true
	case (lb.Normal || lb.Strict) && inLineBreakRanges(r, postfixRanges[:]):
		return true
	}
	return inLineBreakRanges(r, noBreakBeforeRanges[:])
}

// §5.3 names four characters as hyphens and does not treat them alike, which is
// why there are two functions here and not one. The suite states the difference
// in the plainest possible terms: line-break-loose-hyphens-001 says "the second
// line starts with a hyphen" and line-break-normal-hyphens-001, over the same
// text, says it "ends with a hyphen".

// isLatinHyphen is U+2010 HYPHEN and U+2013 EN DASH: a line may begin with one
// under "loose" and under nothing else.
//
// Both are class HH as of Unicode 16, which appears in no unconditional rule, so
// the base table says nothing about them and every value would let a line begin
// with one. Two of the three have to be told otherwise.
func isLatinHyphen(r rune) bool { return r == 0x2010 || r == 0x2013 }

// isEastAsianHyphen is U+301C WAVE DASH and U+30A0
// KATAKANA-HIRAGANA DOUBLE HYPHEN: a line may begin with one under "normal" and
// "loose", and may not under "strict" — or under "auto", which is this engine's
// untailored answer and is what the suite's own default-behaviour tests assert.
//
// Both are class NS, so the base table already forbids them and these two values
// are what let them through.
func isEastAsianHyphen(r rune) bool { return r == 0x301C || r == 0x30A0 }

// MayNotBeginLine reports whether the first character of a run is one a line may
// not begin with.
//
// It exists because a break opportunity can arrive from *outside* the run. Inside
// one, SplitAtBreaks withholds an opportunity in front of such a character as it
// meets it; an opportunity carried in from the box before — an ideograph at the
// end of the previous text node offers one, and the next node may be a <span> —
// has no character in that box to be tested against. So the box that receives it
// asks here.
//
// "中中<span>〜</span>文" is the shape, and the suite has a page of them: the
// character a line may not begin with is written in an element of its own, which
// is exactly what a test that wants to colour it does.
func MayNotBeginLine(text string, lb LineBreak) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	return noBreakBefore(r, lb)
}

// BreaksAfterUnderLoose reports whether a line may end after this character
// because "line-break: loose" says so.
//
// It is the one rule of §5.3 stated the other way round. A currency sign or a
// number sign belongs to the figure that follows it — "￥" and "100" are one
// thing — so no value but loose lets a line end between them, and loose does
// because a newspaper column is narrow enough to need it.
func BreaksAfterUnderLoose(r rune) bool {
	return inLineBreakRanges(r, prefixRanges[:])
}

// inLineBreakRanges searches one of the generated tables, which are sorted and
// disjoint.
func inLineBreakRanges(r rune, table []struct{ lo, hi rune }) bool {
	if len(table) == 0 || r < table[0].lo {
		return false
	}
	i := sort.Search(len(table), func(i int) bool { return table[i].hi >= r })
	return i < len(table) && table[i].lo <= r
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
