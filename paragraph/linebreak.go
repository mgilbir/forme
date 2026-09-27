package paragraph

import (
	"sort"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
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
// It is not UAX #14, which is uax14.go and is what SplitAtBreaks runs. The
// algorithm is thirty-odd pair rules over a class table, and most of them
// depend on what came *before* the break as well as after it: a quotation mark
// may or may not begin a line depending on what quoted it, and a numeric
// separator depends on whether a number surrounds it. Those cannot be answered
// by looking at one character, and this does not pretend to.
//
// What it is, is the subset that can: the rules written "× X" with nothing on
// the left, with line-break's tailoring of them. This package once offered its
// opportunities through it and nothing else; now it answers MayNotBeginLine, for
// a caller that has a character and no context — and that answer is an
// approximation of what the line breaker decides, which needs the context.
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
	case (lb.Normal || lb.Loose) && lb.ChineseOrJapanese && isEastAsianHyphen(r):
		// And only where the text is Chinese or Japanese. §5.3 puts this
		// tailoring under "in Chinese and Japanese", and the suite tests the
		// boundary rather than leaving it a reading:
		// writing-system-line-break-001 sets "line-break: loose" on the same
		// wave dash twice, once in lang=ja and once in lang=ja-Hang — Japanese
		// written in Hangul — and asks for a line to begin with it in the first
		// and not in the second.
		//
		// The writing system and not the language, for the reason
		// writingsystem.go gives: a script subtag says what the text is typeset
		// as, and "ja-Hang" is not typeset as Japanese however it is tagged.
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

// NeedsDictionaryBreaking reports whether a character belongs to a script whose
// words are found by lexical analysis: UAX #14's class SA.
//
// Thai, Lao, Khmer, Myanmar, Tai Le, New Tai Lue, Tai Tham and their
// neighbours. They are written without spaces and without a mark between words
// either, so the only way to know where a line may break is to know the
// language. LB1 resolves the class to AL — "no opportunity anywhere" — and CSS
// Text §5.1 does not accept that: "some form of fallback line breaking must
// occur even if the UA doesn't know how to perform it correctly. Overflowing is
// not allowed."
//
// So a line may end between two typographic character units of such a script,
// which is a place the words are not and is the whole of what this engine can
// offer. It is reported as well as done — see UnsupportedScript — because a
// paragraph broken in the wrong places is a paragraph an author should be told
// about rather than left to find.
func NeedsDictionaryBreaking(r rune) bool {
	return inLineBreakRanges(r, dictionaryRanges[:])
}

// isInseparable reports whether a character is one of UAX #14's class IN, the
// ellipses. "line-break: loose" is the one value that lets a line break between
// two of them; see inseparableRanges and the rule in breaks.go.
func isInseparable(r rune) bool { return inLineBreakRanges(r, inseparableRanges[:]) }

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
// It was how a box that received an opportunity from the box before asked about
// its own first character — "中中<span>〜</span>文", where the character a line
// may not begin with is written in an element of its own. SplitAtBreaksAfter
// decides that boundary itself now, from the BreakContext the box before left,
// by all of UAX #14's rules rather than the ones written "× X"; this is the
// one-character answer, for a caller that has no context to give.
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

// Where a line may not end, and the reason it is a pair and not a character.
//
// noBreakBefore above answers UAX #14's rules written "× X" — the ones that need
// nothing on the left. This is the rest of what a break-all document needs, and
// it is a pair because the rules are: LB11 is "× WJ" *and* "WJ ×", LB12 is
// "GL ×", and neither can be answered by looking at one side.
//
// It is what break-all's opportunities were checked against before SplitAtBreaks
// ran UAX #14 in full: word-break: break-all offers one at every character
// boundary in a word, §5.2 allows breaking "between typographic character
// units", and UAX #14 still says which of those boundaries are not there. The
// scan asks uax14.go now, which treats the letters as ideographs as §5.2 says
// and runs every rule over them; this is the two-character answer GluedPair
// gives a caller with no context.
//
// The suite's word-break-break-all-018, -021 and -022 are one shape —
// "XXXX&nbsp;XXXX X X" in four characters of room — and they are what a break
// either side of a no-break space costs. The right answer sets three characters
// on the first line, because the fourth is glued to the two after it and the
// three of them will not fit; the answer without this rule sets four and hangs
// the no-break space at a line end, which is the one thing its name forbids.
func gluedPair(prev, r rune) bool {
	if prev == 0 {
		return false
	}
	// LB11 "× WJ" and "WJ ×", LB12 "GL ×" and LB12a "× GL", LB8a "ZWJ ×". The
	// three classes are one table because §5.1 wanted the same three for the
	// atomic-inline rule beside them — see BindsToAtomicInline, which excludes
	// U+00A0 where this must not: that exception is about a picture next to a
	// no-break space, and this is about the space itself.
	if isBinding(prev) || isBinding(r) {
		return true
	}
	// A currency sign or a number sign belongs to the figure after it, so a line
	// may not end between them.
	//
	// word-break-break-all-023 and -024 name it: "break-all breaks before the
	// first backslash character because UAX14 rules forbid to break after PR
	// class". U+005C is class PR, which is a surprise until one remembers the
	// class is about what a character introduces rather than what it looks like.
	//
	// §5.3's loose is the one value that lets a newspaper column break there,
	// and there is deliberately no test for it here: this answers without a
	// line-break value, and the relaxation is uax14.go's (lbChar.mayEnd).
	if inLineBreakRanges(prev, prefixRanges[:]) {
		return true
	}
	// LB14, "OP SP* ×": a line may not end after an opening bracket. The bracket
	// belongs to what it opens, and a line ending at one leaves it hanging in
	// the margin with nothing behind it.
	//
	// word-break-break-all-020 is the suite's case and says in its own assertion
	// what it is about: "break-all does not affect rules governing the soft wrap
	// opportunities created by punctuation". It writes "あい）あ（い" in two ems
	// three times over — once plain, once with the breaks written out as markup,
	// and once with break-all — and asks for all three to break at the same
	// points. Ordinary text never asks this function anything, because it offers
	// no opportunity after a bracket to forbid; break-all offers one at every
	// character boundary, and the closing bracket was already refused by the
	// base table while the opening one was not.
	if inLineBreakRanges(prev, openRanges[:]) {
		return true
	}
	return false
}

// GluedPair is gluedPair across a box boundary, where the two characters are in
// different boxes and neither box can ask on its own.
//
// It is the same question layout asks MayNotBeginLine for, one rule along: that
// one is "a line may not begin with this character" and this is "a line may not
// end after that one". Both have to be asked of an opportunity a box makes at
// its own leading edge, because that opportunity is between two characters and
// the box only has one of them.
func GluedPair(prev, r rune) bool { return gluedPair(prev, r) }

// isBinding reports membership of the GL, WJ and ZWJ classes, which is what
// BindsToAtomicInline asks with one character carved out of it.
func isBinding(r rune) bool {
	return inLineBreakRanges(r, bindingRanges[:])
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
	// And a combining mark, which the sentence does not name and the suite asks
	// for anyway.
	//
	// UAX #14's LB9 is why: "do not break a combining character sequence; treat
	// it as if it were the base character". A mark is not a character a line may
	// begin with and not one a line may end before, whatever stands on the other
	// side of it — and §5.1 makes an atomic inline "equivalent to" a character
	// for line breaking, so the sequence rule reaches it like any other.
	//
	// line-breaking-atomic-016 and -017 are the two directions, written by the
	// specification's own editor: "A<CGJ><span>B</span>" and
	// "<span>A</span><CGJ>B", each asserting there is no opportunity at the
	// joiner. A combining grapheme joiner is precisely the character an author
	// writes to say "these two are one thing", so the answer is the one its name
	// asks for.
	if charprop.Is(r, charprop.Mn|charprop.Me) {
		return true
	}
	if r < bindingRanges[0].lo {
		return false
	}
	i := sort.Search(len(bindingRanges), func(i int) bool {
		return bindingRanges[i].hi >= r
	})
	return i < len(bindingRanges) && bindingRanges[i].lo <= r
}

// NeedsPhraseBreaking reports whether a run has text in it that "word-break:
// auto-phrase" would break differently from "normal".
//
// §5.2 allows a line to end only at a phrase boundary, and a phrase is a thing
// the CJK writing systems have. What is asked here is whether a document would
// see the difference — and for a paragraph with no such text in it, it would
// not, because there are no phrases in it to keep whole.
//
// Han, hiragana and katakana are the test, by script. That is the writing the
// rule is about; it is a wider net than any one language and errs towards
// reporting, which is the safe direction for a finding. It used to be
// IsIdeographic, which is UAX #14's ID class and holds Hangul and the emoji as
// well, so a zh-tagged box whose only "ideographs" were emoji or Hangul was told
// its phrases could not be found. Audit C174.
//
// It is half of the question. The other half is which language the text is
// declared to be, because §5.2 gives the value effect only where the UA has a
// model for it — see PhrasesUnfound, which asks both.
func NeedsPhraseBreaking(text string) bool {
	for _, r := range text {
		if isHanOrKana(r) {
			return true
		}
	}
	return false
}
