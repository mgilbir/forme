package paragraph

import (
	"strings"
	"unicode"
)

// text-spacing-trim, CSS Text 4 §8.2, and the one clause of it this engine does.
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
//	space-first normal, except that opening punctuation keeps its blank at the
//	            start of the first line and after every forced break.
//	trim-start  normal, except that opening punctuation is half-width at the
//	            start of every line.
//
// # What is implemented
//
// The clause in italics above, and nothing else: a full-width **closing**
// punctuation at the end of a line that the line would not otherwise hold takes
// its half-width form. That is the clause that changes where a line breaks, and
// it is the one a document can observe without a background or a neighbouring
// character to measure against — the trimmed blank is at the end of the line,
// where there is nothing to move.
//
// "space-all" is implemented by there being nothing to do: it asks for
// full-width everywhere, which is what an engine that trims only the clause
// above already gives it.
//
// The line-*start* clauses of "normal", "space-first" and "trim-start", and the
// collapsing of spacing between adjacent punctuation, are not done. A document
// that declares one of the two values whose whole content is a line-start rule
// is told so; "normal" is not reported, because it is the initial value and a
// finding on every document that holds CJK text at all would say nothing about
// the document that has it.
type SpacingTrim struct {
	// TrimClosingAtEnd trims a full-width closing punctuation the line would
	// not otherwise hold. True for every value but space-all.
	TrimClosingAtEnd bool
}

// SpacingTrimOf reads the property, and names a value whose rule this engine
// does not follow.
//
// An unknown value is invalid and the cascade drops an invalid declaration
// whole, so it answers as the initial value does and reports nothing: the
// element is set as though nobody had written a declaration.
func SpacingTrimOf(value string) (SpacingTrim, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "space-all":
		return SpacingTrim{}, ""
	case "space-first":
		return SpacingTrim{TrimClosingAtEnd: true}, "space-first"
	case "trim-start":
		return SpacingTrim{TrimClosingAtEnd: true}, "trim-start"
	}
	// "normal", and anything invalid, which the cascade has already turned into
	// the initial value by the time this is asked.
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
	return unicode.Is(unicode.Pe, r)
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
