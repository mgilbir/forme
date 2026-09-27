package paragraph

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/internal/charprop"
)

// text-spacing-trim, CSS Text 4 §8.2 as the rest of this engine numbers it (§8.5
// in the Editor's Draft of 2026), and the clauses of it this engine does.
//
// A full-width bracket is the punctuation glyph plus half an em of blank, and
// the property decides when that blank is kept. Its initial value is "normal",
// which is not "leave everything alone":
//
//	normal      full-width opening punctuation is full-width at the start of a
//	            line and full-width closing punctuation is full-width at the end
//	            of one — *or half-width if it does not fit on the line before
//	            justification*. Spacing between adjacent punctuation collapses.
//	space-all   every full-width punctuation stays full-width.
//	space-first as trim-start, except that opening punctuation keeps its blank
//	            on the first line of the block and after every forced break.
//	trim-start  normal, except that opening punctuation is half-width at the
//	            start of every line.
//	trim-both   opening punctuation is half-width at the start of every line
//	            and closing punctuation at the end of every line, fitting or not.
//	trim-all    every full-width punctuation is half-width wherever it is.
//
// space-first's own sentence says "otherwise as normal", and normal keeps the
// blank at every line start; the section's summary table ("yes except on the
// first line"), its compatibility note ("trim-both would have been appropriate
// typographically, except that [the content is] written to expect the first
// line to be set as for space-all"), and the suite's
// text-spacing-trim-start-001, whose space-first reference half-widths the
// bracket that begins a wrapped line and no other, all read it as trim-start
// with the first line spared. That is the reading taken.
//
// # What is implemented
//
//   - The clause in italics above: a full-width **closing** punctuation at the
//     end of a line that the line would not otherwise hold takes its
//     half-width form. It is the clause that changes where a line breaks from
//     the end.
//   - The line-*start* clause of trim-start, space-first and trim-both: a
//     full-width **opening** punctuation that begins a line takes its
//     half-width form, on every line or on every line the value does not
//     spare. See OpeningTrim.
//
// "space-all" is implemented by there being nothing to do, and "auto" is read
// as "normal", which is a choice the value leaves to the user agent.
//
// Not done, and reported where a document asks for them: trim-both's end
// clause (it trims a closing punctuation that fits, which the end clause here
// never does) and trim-all. Nor is the collapsing of spacing between adjacent
// punctuation, which every value but space-all asks for; it is not reported,
// because "normal" asks for it too and a finding on every document holding CJK
// text would say nothing about the document that has it.
type SpacingTrim struct {
	// TrimClosingAtEnd trims a full-width closing punctuation the line would
	// not otherwise hold. True for every value but space-all.
	TrimClosingAtEnd bool
	// TrimOpeningAtStart is which line starts trim a full-width opening
	// punctuation: none, for normal and space-all; every one, for trim-start
	// and trim-both; and every one but the block's first line and the lines
	// after a forced break, for space-first.
	TrimOpeningAtStart OpeningTrim
}

// OpeningTrim is which lines §8.2 half-widths an opening punctuation at the
// start of.
type OpeningTrim uint8

const (
	// OpeningTrimNone keeps the blank at the start of every line.
	OpeningTrimNone OpeningTrim = iota
	// OpeningTrimEveryLine takes it at the start of every line.
	OpeningTrimEveryLine
	// OpeningTrimAfterSoftWrap takes it at the start of a line that a soft
	// wrap began, and keeps it on the first line and after a forced break.
	OpeningTrimAfterSoftWrap
)

// Trims reports whether a line that begins as described takes the trim: first
// for the block's first line, forced for a line after a forced break.
func (o OpeningTrim) Trims(first, forced bool) bool {
	switch o {
	case OpeningTrimEveryLine:
		return true
	case OpeningTrimAfterSoftWrap:
		return !first && !forced
	}
	return false
}

// SpacingTrimOf reads the property, and names a value whose rule this engine
// does not follow in full.
//
// An unknown value is invalid and the cascade drops an invalid declaration
// whole, so it answers as the initial value does and reports nothing: the
// element is set as though nobody had written a declaration.
//
// trim-both is followed at the start of a line and not at its end, where it
// trims a closing punctuation whether or not the line needs it; it is named,
// and trimmed at the end as normal trims there. trim-all is named and set as
// normal is, since it trims characters in the middle of a line, which nothing
// here does.
func SpacingTrimOf(value string) (SpacingTrim, string) {
	switch ascii.Lower(ascii.TrimCSSSpace(value)) {
	case "space-all":
		return SpacingTrim{}, ""
	case "space-first":
		return SpacingTrim{TrimClosingAtEnd: true, TrimOpeningAtStart: OpeningTrimAfterSoftWrap}, ""
	case "trim-start":
		return SpacingTrim{TrimClosingAtEnd: true, TrimOpeningAtStart: OpeningTrimEveryLine}, ""
	case "trim-both":
		return SpacingTrim{TrimClosingAtEnd: true, TrimOpeningAtStart: OpeningTrimEveryLine}, "trim-both"
	case "trim-all":
		return SpacingTrim{TrimClosingAtEnd: true}, "trim-all"
	}
	// "normal", "auto", and anything invalid, which the cascade has already
	// turned into the initial value by the time this is asked.
	return SpacingTrim{TrimClosingAtEnd: true}, ""
}

// TrimsAsClosingPunctuation reports whether a character is one §8.2 trims at the
// end of a line.
//
// Unicode's Pe — the closing half of a bracket pair — and no wider a set than
// that. §8.2 names "full-width closing punctuation", and the *full-width* half
// of the test is not made here: it is made by the face, which states a
// half-width form for the glyphs it has one for and for no others. A font
// without 'halt' trims nothing, a half-width bracket has no blank to give up
// and is not covered, and neither case needs a width table of its own to say so.
//
// Pf — the closing quotation marks — is deliberately out. A full-width closing
// quote is not bracket punctuation in the sense §8.2's classes divide, and this
// engine trims only what it has a document to check it against.
func TrimsAsClosingPunctuation(r rune) bool {
	return charprop.Is(r, charprop.Pe)
}

// TrailingClosingPunctuation is how many bytes at the end of a run §8.2 would
// trim, which is one character or none.
func TrailingClosingPunctuation(text string) int {
	last, size := rune(0), 0
	for i, r := range text {
		last, size = r, len(text)-i
	}
	if size > 0 && TrimsAsClosingPunctuation(last) {
		return size
	}
	return 0
}

// TrimsAsOpeningPunctuation reports whether a character is one §8.2 trims at the
// start of a line: its fullwidth opening punctuation.
//
// The property's own class, which is narrower than Unicode's Ps: an opening punctuation
// in the CJK Symbols and Punctuation block or of East Asian Width F, and the
// two opening quotation marks U+2018 and U+201C, which are Pi and which
// Chinese sets full-width. The *full-width* half of the test is made again by
// the face, which states a half-width form only for the glyphs that have a
// blank half to give up — a "“" a Japanese face sets proportionally is covered
// by nothing and trims nothing.
func TrimsAsOpeningPunctuation(r rune) bool {
	if r == 0x2018 || r == 0x201C {
		return true
	}
	return charprop.Is(r, charprop.Ps) &&
		((r >= 0x3000 && r <= 0x303F) || inRanges(r, eastAsianFullwidthRanges[:]))
}

// LeadingOpeningPunctuation is how many bytes at the start of a run §8.2 would
// trim at the start of a line, which is one character or none.
func LeadingOpeningPunctuation(text string) int {
	for _, r := range text {
		if TrimsAsOpeningPunctuation(r) {
			return utf8.RuneLen(r)
		}
		return 0
	}
	return 0
}
