package layout

import (
	"slices"
	"strings"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
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

// markOpeningPunctuation is §8.2's trim at the start of a line, cut into the
// runs the way markClosingPunctuation cuts the end's: a full-width opening
// punctuation at a place a line could begin becomes an item of its own,
// carrying how much narrower its half-width form is. Whether a line does begin
// there is the fill's to know, and it takes the trim then; see
// paragraph.Item.TrimStart.
//
// # What the half-width form is
//
// The font's, and not half of anything: the glyph as 'halt' positions it. For
// an opening bracket that is a shorter advance *and* the ink moved back into
// the space the blank vacated, so a trim here is not only a width. The run is
// drawn with 'halt' asked for (see startTrimmedFeatures) and the width taken
// off is what the same shaping says the feature takes off, measured rather
// than read from the face's table: shaping is what picks the glyph — a
// Chinese "“" is not the glyph its code point maps to in the cmap — and it is
// the same shaping a backend and the reference's font-feature-settings reach,
// so the width the line is filled to is the width the run is drawn at.
//
// # When it is not taken
//
//   - A face that states no 'halt' for the glyph gives the run nothing to take,
//     and it is not cut out.
//   - A run whose font-feature-settings already asks for 'halt' is set
//     half-width wherever it is, and has no blank left to give up. The suite's
//     references are written that way — text-spacing-trim-start-002-ref sets
//     each line's bracket in a <halt> element *and* declares trim-start — and
//     trimming them again took a second half em off each.
//   - A run whose font-feature-settings turns 'halt' off has refused the
//     font's half-width forms, and the property's collapsing rules let a user
//     agent decline to trim by "font features". It is set whole.
//   - An upright run of vertical text. Its half-width form is the one 'vhal'
//     states, on the other axis, and nothing here asks for it: that is named
//     in a finding rather than guessed from the horizontal one. A sideways
//     run is the horizontal line turned, and is trimmed as one.
//   - A character that hangs. §8.4 has put it outside the line, where it has no
//     blank inside the line to give up.
func (l *layouter) markOpeningPunctuation(items []inlineItem, st spacingTrim) []inlineItem {
	var out []inlineItem
	for i, item := range items {
		head, tail, cut := l.openingCandidate(items, i, st)
		if !cut {
			if out != nil {
				out = append(out, item)
			}
			continue
		}
		if out == nil {
			out = make([]inlineItem, 0, len(items)+4)
			out = append(out, items[:i]...)
		}
		out = append(out, head)
		if tail.Text != "" {
			out = append(out, tail)
		}
	}
	if out == nil {
		return items
	}
	return out
}

// openingCandidate is items[i] cut for the trim: the punctuation as an item of
// its own carrying the trim, and the rest of the run after it, if any. cut is
// false where the item is not a candidate or its face has no half-width form
// for the character, and then the item is to be kept as it is.
func (l *layouter) openingCandidate(items []inlineItem, i int, st spacingTrim) (head, tail inlineItem, cut bool) {
	item := items[i]
	if !l.opensALine(items, i, st) {
		return item, inlineItem{}, false
	}
	n := leadingOpeningPunctuation(item.Text)
	head = item
	if n < len(item.Text) {
		head, tail = l.br.SplitItem(item, n)
	}
	head.TrimStart = l.openingTrimOf(head)
	if head.TrimStart == 0 {
		return item, inlineItem{}, false
	}
	head.TrimStartOn = spacingTrimFor(head, st).TrimOpeningAtStart
	return head, tail, true
}

// opensALine reports whether items[i] is a candidate for the trim: a run of
// text starting with a full-width opening punctuation, under a value that
// trims at some line starts, at a place a line could begin. It reports an
// upright candidate as not done, once per value.
func (l *layouter) opensALine(items []inlineItem, i int, st spacingTrim) bool {
	item := items[i]
	if !canTrimAsClosing(item) || leadingOpeningPunctuation(item.Text) == 0 {
		return false
	}
	own := spacingTrimFor(item, st)
	if own.TrimOpeningAtStart == paragraph.OpeningTrimNone || !couldBeginALine(items, i) {
		return false
	}
	if item.Upright {
		if b := elementBoxOf(heldBox(item.Box)); b != nil {
			l.reportSpacingTrimUpright(b, b.Style.Get("text-spacing-trim"))
		}
		return false
	}
	return true
}

// couldBeginALine reports whether a line could begin at items[i]: at the start
// of the content, after a forced break, or at a break opportunity. Everything
// that is not content — an inline box's own edge, a box out of flow, a
// collapsible space, a bidi control — is stepped back over, because none of it
// stops what follows it beginning the line.
//
// The fill has the last word and this only has to be generous: a candidate
// that never begins a line is an item cut in two that is set exactly as it was.
func couldBeginALine(items []inlineItem, i int) bool {
	if items[i].BreakBefore {
		return true
	}
	for j := i - 1; j >= 0; j-- {
		it := items[j]
		switch {
		case it.Forced:
			return true
		case it.Inset || it.Float != nil || it.Abs != nil || it.Collapsible ||
			paragraph.IsBidiControlOnly(it.Text):
			continue
		}
		return false
	}
	return true
}

// elementBoxOf is the box a finding about a run is named by: a text box carries
// its element's style and has no element of its own to point at.
func elementBoxOf(b *Box) *Box {
	for b != nil && b.IsText() && b.Parent != nil {
		b = b.Parent
	}
	return b
}

// openingTrimOf is how much narrower the item's text is with 'halt' asked for
// than without, measured by the shaping that will draw it; zero where the
// feature changes nothing and where the run turns it off. See
// markOpeningPunctuation.
//
// A run that already asks for 'halt' needs no case of its own: it is measured
// with the feature both times, so the two widths are one and the trim is zero.
// A run that turns it off does, because asking for the feature again puts the
// tag in both of its lists, and the shaping then applies it.
//
// Measured in the item's own context and without its merge group: the group
// is shaped as one string under one set of features, and asking it for
// 'halt' would ask it of every character in it rather than of this one.
func (l *layouter) openingTrimOf(item inlineItem) style.Unit {
	if item.Face == nil || item.Text == "" || hasFeatureTag(item.Off.TagsOff, "halt") {
		return 0
	}
	how := shaping{Before: item.PreContext, After: item.PostContext,
		ContextKerns: item.ContextKerns, Off: item.Off}
	full := l.br.MeasureSpacedInContext(item.Face, item.Text, item.Size, item.Spacing, how)
	how.Off = startTrimmedFeatures(item.Off)
	half := l.br.MeasureSpacedInContext(item.Face, item.Text, item.Size, item.Spacing, how)
	if half >= full {
		return 0
	}
	return full.Sub(half)
}

// startTrimmedFeatures is the features a run the fill trimmed at the start of
// its line is drawn with: its own, and 'halt'.
func startTrimmedFeatures(off shape.Features) shape.Features {
	if hasFeatureTag(off.Tags, "halt") {
		return off
	}
	tags := []string{"halt"}
	if off.Tags != "" {
		tags = append(tags, strings.Split(off.Tags, ",")...)
	}
	// The settled form: sorted, so that two runs asking for the same features
	// share the shaping memo's entry. See shape.Features.Tags.
	slices.Sort(tags)
	off.Tags = strings.Join(tags, ",")
	return off
}

// hasFeatureTag reports whether a settled comma-separated tag list names tag.
func hasFeatureTag(list, tag string) bool {
	for _, t := range strings.Split(list, ",") {
		if t == tag {
			return true
		}
	}
	return false
}

// runFeatures is what a run is drawn with: its item's features, and 'halt'
// where the fill set it at the start of a line in its half-width form. The
// width the line took off it is the width that feature takes off, so drawing
// it any other way would put the ink half an em from where the line left room
// for it.
func runFeatures(item inlineItem) shape.Features {
	if item.StartTrimmed {
		return startTrimmedFeatures(item.Off)
	}
	return item.Off
}
