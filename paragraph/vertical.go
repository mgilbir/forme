package paragraph

import (
	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/segment"
)

// UAX #50, Unicode Vertical Text Layout: which way a character faces on a line
// of vertical text.
//
// This engine sets vertical text by turning a horizontal page ninety degrees
// clockwise, which rotates every character on it; that is what
// "text-orientation: mixed" asks for wherever a character's Vertical_Orientation
// is R, and it is not what mixed asks for anywhere else. A character that stands
// upright is set in a run of its own, measured by its vertical advance and drawn
// standing on the turned line. So the engine needs to know where a paragraph
// changes from one to the other, and this is how it knows: SplitAtOrientation
// cuts the text there and UprightInMixed says which way each part faces. See
// layout/writingmode.go for what is done with the answer and cmd/genvertical
// for how the table is built.

// IsUpright reports whether a character stands upright on a line of vertical
// text — UAX #50's U, or Tu falling back to it.
//
// A character this returns true for cannot be set by rotating a horizontal line,
// so under "mixed" it is set upright, in a run of its own.
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
// It is a question about the picture and not a way to set the text: layout
// asked it once, to refuse a box that needed both, and it sets such a box now
// by cutting its text with SplitAtOrientation. The two differ over white space,
// and on purpose — below.
//
// A character that marks no paper is skipped, which is what makes the answer
// about the picture rather than about the string. The orientation of a space is
// unobservable — it is blank whichever way up it is — so "日本 と" has one
// orientation on the page. Its space is still *set* lying down, at its own
// horizontal advance, because that is what UAX #50 says of U+0020.
func OrientationMix(text string) (upright, rotated bool) {
	for _, r := range text {
		if charprop.WhiteSpace(r) || MarksNoPaper(r) || IsDefaultIgnorable(r) {
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
// is set upright in a face that states no vertical metrics.
//
// One em each, and the count is what the width is made of — CSS Writing Modes
// §4.4's synthesis, see Breaker.MeasureSpacedInContext. A face that states them
// is measured by them instead (shape.Face.StatesVerticalMetrics). What is left out is what takes no room in any
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
	eachUprightUnit(text, func(int) { n++ })
	return n
}

// AppendUprightUnitStarts appends where each of the characters UprightUnits
// counts begins in text, as a byte offset, in order.
//
// It is the same count, placed: what a caller needs to hand each of those
// characters its em when it holds the glyphs rather than the total — a
// backend stepping a pen down an upright run in a face with no vertical
// metrics. The two are one walk so that they cannot disagree about what a
// character is.
func AppendUprightUnitStarts(dst []int, text string) []int {
	eachUprightUnit(text, func(start int) { dst = append(dst, start) })
	return dst
}

// eachUprightUnit calls f with the start of each grapheme cluster of text that
// takes an advance upright: one that holds a character other than a default
// ignorable or a combining mark.
func eachUprightUnit(text string, f func(start int)) {
	eachCluster(text, func(start, end int) {
		for _, r := range text[start:end] {
			if IsDefaultIgnorable(r) || charprop.Is(r, charprop.Mn|charprop.Me) {
				continue
			}
			f(start)
			break
		}
	})
}

// SplitAtOrientation cuts text where "text-orientation: mixed" stops setting
// its characters one way and starts setting them the other: the parts, in
// order, concatenate to text, and each is set wholly upright or wholly
// sideways. A text with one orientation throughout is one part.
//
// The unit is UAX #50 §3.2.1's: a grapheme cluster, whose orientation is its
// first character's — except that a cluster holding an enclosing mark is
// upright whatever it encloses, which is what makes a keycap stand up. CSS
// Writing Modes §5.1.2 asks the same question of each typographic character
// unit, which is the same cluster.
//
// A cut is between clusters and never inside one, because the orientation is
// the whole cluster's: a base upright with its mark lying on its side is not a
// thing a reader has ever seen.
func SplitAtOrientation(text string) []string {
	var parts []string
	start, was, first := 0, false, true
	eachCluster(text, func(from, to int) {
		upright := clusterIsUpright(text[from:to])
		if !first && upright != was {
			parts = append(parts, text[start:from])
			start = from
		}
		was, first = upright, false
	})
	if start < len(text) || len(parts) == 0 {
		parts = append(parts, text[start:])
	}
	return parts
}

// UprightInMixed reports whether "text-orientation: mixed" sets a run upright,
// asked of a run SplitAtOrientation has already made one orientation
// throughout — so its first cluster answers for all of it. The empty run is
// sideways, which is the default UAX #50 gives everything it does not list.
func UprightInMixed(text string) bool {
	upright := false
	eachCluster(text, func(from, to int) {
		if from == 0 {
			upright = clusterIsUpright(text[from:to])
		}
	})
	return upright
}

// clusterIsUpright is UAX #50 §3.2.1's orientation of one grapheme cluster.
func clusterIsUpright(cluster string) bool {
	for _, r := range cluster {
		if charprop.Is(r, charprop.Me) {
			return true
		}
	}
	for _, r := range cluster {
		return IsUpright(r)
	}
	return false
}

// eachCluster calls f with the bounds of each grapheme cluster of text, in
// order.
//
// The boundaries are found once, before the walk. They were found inside it —
// the whole string walked again per cluster, which is the same answer every
// time and quadratic in the length of the run.
func eachCluster(text string, f func(from, to int)) {
	bounds := segment.Boundaries(nil, text)
	for i, start := 0, 0; start < len(text); i++ {
		end := len(text)
		if i < len(bounds) {
			end = bounds[i]
		}
		f(start, end)
		start = end
	}
}
