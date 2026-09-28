package layout

import (
	"slices"
	"strings"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// CSS Writing Modes §9.1: text-combine-upright, which Japanese calls
// tate-chu-yoko — a date's "20" or an initialism's "FM" set across a vertical
// line, in the space of one character.
//
// # What it is on the page
//
// Three things, and each is one of the spec's own sentences.
//
//   - Along the line it is one em: "the effective size of the composition is
//     assumed to be 1em square; anything outside the square is not measured
//     for layout purposes". So it is one item on the line, marked
//     paragraph.Item.Combine, which the breaker measures as an em whatever its
//     text is, and which is never cut.
//   - For everything else — letter-spacing, justification, autospace, the
//     shaping of its neighbours — it "is treated as a single glyph representing
//     the Object Replacement Character U+FFFC", which stands upright. So the
//     item is Upright as well, and the rules that ask about a boundary ask
//     Combine first.
//   - Across the page it is its text set horizontally, "similar to the
//     contents of an inline-block box with a horizontal writing mode", centred
//     in the em square, and the square centred "between the text-over and
//     text-under baselines of its parent inline box" — which is the central
//     baseline, the one an upright run is hung from (centralShift).
//
// # Why this is not a rotation either
//
// The composition's glyphs are neither turned nor stood up: they are a
// horizontal run, drawn across the page on a line that runs down it. The
// display list says so by drawing it as one — a DrawText with neither
// Sideways nor Upright — at the point the turn put the square. That is the
// reason the painter places it (paintCombined) rather than the turn.
//
// # Compression
//
// §9.1.3: the text must fit the em. Where the face has the width variant for
// the number of characters — hwid for two, twid for three, qwid for four — and
// it replaces every one of them, that is what the text is set in; the
// specification says an OpenType implementation "must" use them then. What is
// still wider than an em after that is squeezed across the page to fit, which
// is DrawText.WidthScale. Before either, a composition of more than one
// character has its full-width forms taken back to the characters they were
// made from (§9.1.3.1), so that "text-transform: full-width" on a date does
// not squeeze two ems into one where the digits themselves would fit.
//
// # What is not combined
//
// "digits <n>" is refused: no browser at wpt.fyi passes its reftests, and the
// run rules it adds (maximal runs of ASCII digits of at most n) are a second
// feature. The turn is refused for a box asking for it, as it was for every
// value before this. A composition whose text the fallback stack sets in more
// than one face is laid out as though it did not ask, and said so: one
// composition is one run.

// combineAll reports whether a box's computed text-combine-upright is "all".
func combineAll(b *Box) bool {
	return b != nil && trimmedLower(b.Style.Get("text-combine-upright")) == "all"
}

// combinesText reports whether a text box's text is set as one composition.
//
// It is a vertical-typographic-mode property: §9.1 "only has an effect in
// vertical writing modes", and a sideways mode is a horizontal typographic
// one. And §9.1.1's text run rule: the composition is the whole of a run of
// text not interrupted by a box boundary, and a candidate whose characters
// would carry on into the next box's, or come from the previous box's, is not
// combined at all — "<tcy>12<span>34</span></tcy>" combines nothing, because
// 1234 is one sequence a box boundary interrupts.
func (l *layouter) combinesText(b *Box) bool {
	if !b.IsText() || b.Text == "" || !combineAll(b) {
		return false
	}
	mode, vertical := l.turnedModeOf(b)
	if !vertical || mode.sideways() {
		return false
	}
	return !l.combinesAcross(b, l.prevInContext) && !l.combinesAcross(b, l.nextInContext)
}

// combinesAcross reports whether the first character met walking from b in
// one direction, through box boundaries and past boxes that hold no text, is
// one that would combine too.
//
// The walk stops at what is not text in the line: an atomic inline, a
// replaced element or a forced break is a character that combines with
// nothing. A box out of the flow is not in the line at all and is passed.
func (l *layouter) combinesAcross(b *Box, step func(*Box) *Box) bool {
	for cur := step(b); cur != nil; cur = step(cur) {
		switch {
		case cur.Position.outOfFlow() || cur.Float != FloatNone:
		case cur.Replaced != nil || isAtomicInline(cur) || isForcedBreak(cur):
			return false
		case cur.IsText() && cur.Text != "":
			return combineAll(cur)
		}
	}
	return false
}

// prevInContext is nextInContext the other way: the box written before b,
// without leaving the inline formatting context.
//
// The last thing written inside an inline sibling is what is next to b, so the
// walk goes into it, to its last child, and on down.
func (l *layouter) prevInContext(b *Box) *Box {
	for c := b; c.Parent != nil; c = c.Parent {
		if s := l.prevSiblingOf(c); s != nil {
			for s.Outer == OuterInline && !s.IsText() && len(s.Children) > 0 &&
				!isAtomicInline(s) && s.Replaced == nil {
				s = s.Children[len(s.Children)-1]
			}
			return s
		}
		if c.Parent.Outer != OuterInline {
			return nil
		}
	}
	return nil
}

// combineItems makes a text box's items one composition, where combinesText
// says the box's text is one. items are the box's own, as itemsFor built them
// — a piece at a time, with every break opportunity and every cut — and what
// comes back is the one item holding all of their text.
//
// Built from the items rather than instead of them because everything around
// the composition is the items' already: the opportunity in front of the first
// of them is §9.1.2's "for line breaking before and after the composition, it
// is treated as a regular inline with its actual contents", and the state the
// next box starts from was carried out of the last. What is dropped is what is
// inside: the opportunities between the pieces, the forced breaks §9.1.2
// ignores, and the white space the document leaves at either end, which is
// "processed as at the start/end of such an inline block" — gone, where it
// collapses.
func (l *layouter) combineItems(b *Box, items []inlineItem) []inlineItem {
	first, last := -1, -1
	for i, it := range items {
		if it.Text == "" || it.Forced || (it.Space && it.Collapsible) {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 {
		// Nothing but white space, which the ends of the composition collapse
		// away. There is no composition, and the items are what the text is.
		return items
	}
	var text strings.Builder
	for i := first; i <= last; i++ {
		it := items[i]
		if it.Forced || it.Text == "" {
			continue
		}
		if it.Face != items[first].Face || it.Size != items[first].Size {
			// The fallback stack set part of the text in another face, or small
			// capitals set part of it smaller. One composition is one run drawn
			// in one face at one size, so this is laid out as though it had not
			// asked, and said so.
			l.reportCombine(b, "its text is set in more than one face or size")
			return items
		}
		text.WriteString(it.Text)
	}
	out := items[first]
	out.Text = text.String()
	// The range the bidirectional algorithm knows the composition by covers
	// every character of the box, the white space left off its ends included:
	// the builder was handed all of it, and a range with a hole in it is one
	// the reordering cannot map back.
	for _, it := range items {
		if it.BidiEnd <= it.BidiStart {
			continue
		}
		out.BidiStart = min(out.BidiStart, it.BidiStart)
		out.BidiEnd = max(out.BidiEnd, it.BidiEnd)
	}
	// §9.1.3.1: a composition of more than one character sets its full-width
	// characters in the forms they were made from, before it is compressed.
	if units := paragraph.SpacedUnits(out.Text); units > 1 {
		out.Text = paragraph.FromFullWidth(out.Text)
	}
	out.Combine, out.Upright = true, true
	out.Space, out.Collapsible, out.Tab = false, false, false
	out.TrimAtEnd, out.Hangs, out.HangsHard = false, false, false
	out.BreakWord, out.Anywhere = false, false
	out.Hyphen, out.HyphenText, out.HyphenFace, out.HyphenSkip = 0, "", nil, 0
	out.PreContext, out.PostContext = "", ""
	out.MergePre, out.MergePost, out.MergeGroup = "", "", ""
	out.Off = combineFeatures(out.Face, out.Text, out.Off)
	out.Width = l.br.MeasureSpacedInContext(out.Face, out.Text, out.Size, out.Spacing,
		itemShaping(&out))
	// Across the line the composition is its em square, centred on the
	// central baseline, and the line is at least as tall as that: §9.1.2's
	// "similar to the contents of an inline-block box ... with a line-height
	// of 1em". The box's own leading is still the box's.
	draw, _ := l.centralShift(b, out.Face, out.Size, true)
	half := out.Size.Div(2)
	out.Above = style.Max(out.Above, half.Sub(draw))
	out.Below = style.Max(out.Below, half.Add(draw))

	// What was left off the ends is white space the ends collapse and forced
	// breaks the composition ignores, and nothing of it is on the line. A line
	// may begin in front of the composition where it could begin anywhere in
	// front of its first character — after a space the document left there as
	// readily as before the character itself.
	for _, it := range items[:first] {
		out.BreakBefore = out.BreakBefore || it.BreakBefore
	}
	return []inlineItem{out}
}

// reportCombine names a composition that was not made, and why.
func (l *layouter) reportCombine(b *Box, why string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "\"text-combine-upright: all\" was not applied to this text because " +
			why + "; it is set along the line as though it had not asked",
		Path:     PathOf(boxElement(b)),
		Property: "text-combine-upright",
	})
}

// widthVariants are §9.1.3's OpenType features, by the number of typographic
// character units they fit into one em.
var widthVariants = map[int]string{2: "hwid", 3: "twid", 4: "qwid"}

// combineFeatures is the features a composition is set with: the run's own,
// and the width variant for its number of characters where the face has one
// that replaces every one of them.
//
// "Must use width-specific variants ... in cases where those variants are
// available for all typographic character units in the composition", and
// only then: a variant that narrows the digits and leaves the letter beside
// them would set the composition in two widths. Whether it replaces them all
// is asked of the glyphs, by shaping the text with the feature and without it:
// a face may list the feature and cover only some characters with it. A
// document that turned the feature off by name keeps it off — the width
// variant is the UA's request, and CSS Fonts 4 §7.2 puts
// font-feature-settings above the UA's.
func combineFeatures(face *shape.Face, text string, off shape.Features) shape.Features {
	tag := widthVariants[paragraph.SpacedUnits(text)]
	if tag == "" || face == nil || !faceDeclares(face, tag) ||
		tagListed(off.TagsOff, tag) || tagListed(off.Tags, tag) {
		return off
	}
	with := off
	with.Tags = withTag(off.Tags, tag)
	plain, _ := face.ShapeGlyphsInContext(text, "", "", off)
	varied, _ := face.ShapeGlyphsInContext(text, "", "", with)
	if !everyClusterChanged(plain, varied) {
		return off
	}
	return with
}

// everyClusterChanged reports whether every shaping cluster of a run is set
// in other glyphs by the second shaping than by the first.
func everyClusterChanged(before, after []shape.Glyph) bool {
	was := map[int][]int{}
	for _, g := range before {
		was[g.Cluster] = append(was[g.Cluster], g.GID)
	}
	now := map[int][]int{}
	for _, g := range after {
		now[g.Cluster] = append(now[g.Cluster], g.GID)
	}
	if len(was) == 0 {
		return false
	}
	for cluster, gids := range was {
		if slices.Equal(gids, now[cluster]) {
			return false
		}
	}
	return true
}

// tagListed reports whether a settled tag list (shape.Features.Tags) names a
// tag.
func tagListed(list, tag string) bool {
	return slices.Contains(strings.Split(list, ","), tag)
}

// withTag is a settled tag list with one more tag in it, kept in the sorted
// order the list is settled in. See shape.Features.Tags.
func withTag(list, tag string) string {
	tags := []string{tag}
	if list != "" {
		tags = append(strings.Split(list, ","), tag)
	}
	slices.Sort(tags)
	return strings.Join(tags, ",")
}

// combineFit is how a composition's text fits its em: how wide it is set
// across the page, and the factor it is squeezed by to get there — one where
// it fits as it is.
//
// The width is the text's own advance with the features it is set in, width
// variant included, and with no letter- or word-spacing: §9.1.2 composes it
// "ignoring letter-spacing", and a DrawText has no way to carry word-spacing
// into a run layout did not cut at its spaces.
func (l *layouter) combineFit(item inlineItem) (scale float64, width style.Unit) {
	natural := l.br.MeasureSpacedInContext(item.Face, item.Text, item.Size,
		paragraph.TextSpacing{}, shaping{ContextKerns: true, Off: item.Off})
	if natural <= item.Size || natural <= 0 {
		return 1, natural
	}
	return item.Size.Px() / natural.Px(), item.Size
}

// combinedText is the DrawText for a composition: its text, across the page,
// in the square the line gave it.
//
// at is the run's pen position on the page, which for a turned line is the
// start of the run along the line on its alphabetic baseline; the square
// begins there and runs an em down the line, and across it is centred on the
// central baseline, drawShift from at. The text is centred in the square both
// ways: across the page by its width, and down the page by the face's ascent
// and descent, which is the box a horizontal line of it would be set in.
//
// false where there is no square to place it in — a line that is not turned
// clockwise, which a composition is never made on (combinesText) — and the
// caller then draws the run as it would any other.
func combinedText(run TextRun, at Point, turn runTurn, colour style.RGBA) (DrawText, bool) {
	if !turn.sideways || turn.anticlockwise || run.Face == nil {
		return DrawText{}, false
	}
	centre := runPoint(at, run.drawShift, turn)
	ascent, descent, ok := lineExtentsAt(run.Face, run.Size)
	if !ok {
		return DrawText{}, false
	}
	top := run.Size.Sub(ascent.Add(descent)).Div(2)
	v := DrawText{
		At: Point{
			X: centre.X.Sub(run.combineWidth.Div(2)),
			Y: centre.Y.Add(top).Add(ascent),
		},
		Text:     drawableText(run.Text),
		Face:     run.Face,
		Size:     run.Size,
		Color:    colour,
		Features: run.Features,
	}
	if run.combineScale < 1 {
		v.WidthScale = run.combineScale
	}
	return v, true
}
