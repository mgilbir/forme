package layout

import (
	"github.com/mgilbir/forme/style"
)

// §8.2's trim, cut into the runs before anything is measured against a line.
//
// The same shape as hangingpunctuation.go's markStopsAndCommas, and for the same
// reason: a trim is a width, a width is a property of an item, and the character
// whose width changes has to be an item of its own before the fill can change it.
// What the fill then does with it is a different answer to the same question —
// the hang puts the character past the end of the line and the trim takes the
// blank half out of it — and both are asked only of a line that would not
// otherwise hold the character.
//
// The amount is the face's and not a half of anything. A full-width bracket is
// the glyph plus half an em of blank, and which side the blank is on is what
// makes the mark opening or closing; OpenType states both facts per glyph in
// 'halt', which shape reads into a table it applies to nothing. So a face that
// says nothing about a glyph trims nothing, and no assumption is made about a
// font that has not been asked.
//
// Whether anything is trimmed is asked of each item's own value and not of the
// block's. The property inherits, so the two agree for almost every document,
// and they part company exactly where an inner element declares it: a span
// asking for "normal" inside a "space-all" paragraph was never trimmed, because
// the paragraph's value turned the whole pass off before the span was asked
// (audit C145). The same walk reports each run's own value where it asks for
// what is not done, which the block's report alone could not: a span declaring
// "trim-start" inside a paragraph that declares nothing was never mentioned.
func (l *layouter) markClosingPunctuation(items []inlineItem, st spacingTrim) []inlineItem {
	any := false
	for i := range items {
		if !canTrimAsClosing(items[i]) {
			continue
		}
		b := heldBox(items[i].Box)
		for b != nil && b.IsText() && b.Parent != nil {
			// Named by its element: a text box carries the element's style
			// and has no element of its own for a finding to point at.
			b = b.Parent
		}
		if b != nil {
			if _, unhandled := spacingTrimOf(b.Style.Get("text-spacing-trim")); unhandled != "" {
				l.reportSpacingTrim(b, unhandled)
			}
		}
		if !any && spacingTrimFor(items[i], st).TrimClosingAtEnd &&
			trailingClosingPunctuation(items[i].Text) != 0 {
			any = true
		}
	}
	if !any {
		return items
	}
	out := make([]inlineItem, 0, len(items)+4)
	for i, item := range items {
		n := 0
		if canTrimAsClosing(item) && spacingTrimFor(item, st).TrimClosingAtEnd &&
			couldEndALine(items, i) {
			n = trailingClosingPunctuation(item.Text)
		}
		switch {
		case n == 0:
			out = append(out, item)
		case n == len(item.Text):
			item.TrimEnd = trimWidthOf(item, item.Text)
			out = append(out, item)
		default:
			head, tail := l.br.SplitItem(item, len(item.Text)-n)
			tail.TrimEnd = trimWidthOf(tail, tail.Text)
			out = append(out, head, tail)
		}
	}
	return out
}

// canTrimAsClosing reports whether an item is a run of text a closing bracket
// could be cut out of.
//
// Not one §8.4 has already claimed. A character that hangs is outside the line
// and its width is not in the line's measure at all, so there is no blank left
// in it to trim and marking it would offer the fill a choice between two answers
// to one question.
func canTrimAsClosing(item inlineItem) bool {
	return item.Text != "" && item.Face != nil && !item.Tab && !item.Forced &&
		!item.Inset && item.AtomicBox == nil && item.Float == nil && item.Abs == nil &&
		!item.HangStart && !item.HangEnd && !item.MayHangEnd
}

// trimWidthOf is what the face says the item's last character gives up, in
// layout units, or zero where it says nothing.
//
// The adjustment is in font units and the sign is the font's: 'halt' states the
// trim as a *negative* advance, being an adjustment to apply. What an item
// carries is how much narrower the trimmed form is, which is a width and is
// positive, so the sign is turned here rather than at the four places that would
// otherwise each have to remember which way round it was.
//
// A positive adjustment — a font stating that its half-width form is *wider* —
// answers zero rather than widening anything. It is not a trim, and §8.2 has
// nothing to say about it.
func trimWidthOf(item inlineItem, text string) style.Unit {
	if item.Face == nil {
		return 0
	}
	var last rune
	for _, r := range text {
		last = r
	}
	gid, ok := item.Face.GlyphID(last)
	if !ok {
		return 0
	}
	_, dAdvance, ok := item.Face.HalfWidthTrim(gid)
	if !ok || dAdvance >= 0 {
		return 0
	}
	upem := float64(item.Face.UnitsPerEm())
	if upem == 0 {
		return 0
	}
	return item.Size.Mul(float64(-dAdvance) / upem)
}

// spacingTrimFor is the property as it applies to the character that would be
// trimmed, which is the value on the box that character is *in*.
//
// hangingFor gives the argument at length: the property inherits, so the two
// agree for almost every document and part company where the rule is written on
// an inner element and nowhere else.
func spacingTrimFor(item inlineItem, block spacingTrim) spacingTrim {
	b := heldBox(item.Box)
	if b == nil {
		return block
	}
	st, _ := spacingTrimOf(b.Style.Get("text-spacing-trim"))
	return st
}
