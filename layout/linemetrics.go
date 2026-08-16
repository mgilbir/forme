package layout

import (
	"strings"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What a box says about the lines inside it.
//
// Every one of these reads a computed style and answers in layout units: the
// strut, the leading, where the baseline sits, what vertical-align asked for,
// how wide a tab stop is, how many lines a clamp allows. They are the half of
// inline layout that needs a document — forme/paragraph does the arithmetic and
// this decides what numbers go into it.

// strutFor measures the block's own font at its own size.
func (l *layouter) strutFor(b *Box) strut {
	s := strut{Height: l.lineHeight(b), Baseline: l.baselineOf(b, l.lineHeight(b))}
	face, ok := l.fontFor(b)
	if !ok {
		return s
	}
	d := face.Descriptor()
	upem := float64(face.UnitsPerEm())
	if upem == 0 {
		return s
	}
	if ascent, descent, ok := l.lineExtents(b, face); ok {
		s.Ascent, s.Descent = ascent, descent
	}
	// The x-height "vertical-align: middle" is measured against.
	//
	// The face's own when it states one, and otherwise the same two estimates
	// decorationMetrics falls back to — seven tenths of the cap height, or half
	// an em. The three cases and their order are deliberately identical to that
	// function's, because the two are one question: a document whose strike and
	// whose middle-aligned box sat at different heights would be answering it
	// twice, differently, for no reason a reader could see.
	//
	// Nothing this engine read stated an x-height when this was written, so the
	// estimate was the only answer available and the agreement was free. The
	// standard fourteen state one now, and keeping the two in step costs this
	// branch. TestTheStrikeAndAMiddleAlignedBoxUseOneXHeight is what says they
	// are still in step.
	s.XHeight = b.FontSize.Mul(0.5)
	switch {
	case d.Has(shape.MetricXHeight) && d.XHeight > 0:
		s.XHeight = b.FontSize.Mul(float64(d.XHeight) / upem)
	case d.CapHeight > 0:
		// Cap height is reported, and x-height is about seven tenths of it for
		// the faces that report either.
		s.XHeight = b.FontSize.Mul(float64(d.CapHeight) / upem * 0.7)
	}
	return s
}

// leading is how far a run of text in an inline box reaches above and below the
// baseline it sits on: CSS 2.1 §10.8.1's leading, half above the font's ascent
// and half below its descent.
//
// It is the same arithmetic the strut is measured by, and deliberately so — the
// strut *is* an inline box, the block's own, and the only thing that makes it
// special is that it is on every line whether or not anything else is. Sharing
// the formula is what keeps a line of plain text exactly as tall as it was: the
// text box inherits the block's font and line-height, so its two numbers are the
// strut's two numbers and the maximum below changes nothing.
//
// The half-leading may be negative — "line-height: 0" asks for a box shorter
// than its own type — and it is passed on rather than clamped, because that is
// precisely how a stylesheet packs lines closer than the font wants.
func (l *layouter) leading(b *Box) (above, below style.Unit) {
	h := l.lineHeight(b)
	above = l.baselineOf(b, h)
	return above, h.Sub(above)
}

// verticalAlignOf reads the vertical-align property of an inline-level box.
func (l *layouter) verticalAlignOf(b *Box) (vAlign, style.Unit) {
	raw := strings.ToLower(strings.TrimSpace(b.Style["vertical-align"]))
	switch raw {
	case "", "baseline":
		return vAlignBaseline, 0
	case "top":
		return vAlignTop, 0
	case "bottom":
		return vAlignBottom, 0
	case "middle":
		return vAlignMiddle, 0
	case "text-top":
		return vAlignTextTop, 0
	case "text-bottom":
		return vAlignTextBottom, 0
	case "sub":
		// The specification leaves the distance to the engine. A fifth of the
		// font size is what browsers use.
		return vAlignBaseline, style.Unit(0).Sub(b.FontSize.Mul(0.2))
	case "super":
		return vAlignBaseline, b.FontSize.Mul(0.33)
	}
	// A length raises the box by that much; a percentage is of the line-height,
	// which is the one property whose percentages are of line-height rather than
	// of the containing block. It is the box's *own* line-height and not the
	// block's: §10.8.1 says "a percentage of the 'line-height' value" with no
	// qualification, and an unqualified percentage in CSS is of the element's own
	// value of the property named.
	if length, ok := l.parseLength(b, "vertical-align"); ok {
		if v, ok := length.Resolve(l.lineHeight(b), true); ok {
			return vAlignBaseline, v
		}
	}
	return vAlignBaseline, 0
}

// vAlignFor combines an inline box's own vertical-align with what the boxes
// around it already asked for.
func (l *layouter) vAlignFor(b *Box, in vAlignState) vAlignState {
	own, lift := l.verticalAlignOf(b)
	switch own {
	case vAlignTop, vAlignBottom:
		// A new aligned subtree, placed against the line box. Nothing outside it
		// carries in: whatever raised the boxes around this one, this one's top
		// or bottom is the line's.
		return vAlignState{LineAlign: own, Subtree: b}
	case vAlignBaseline:
		in.Raise = in.Raise.Add(lift)
		return in
	}
	// "middle", "text-top" or "text-bottom": a position against the parent, so
	// it replaces the accumulated displacement and stays in whatever subtree it
	// was already in.
	//
	// The reset is written twice and that was established rather than assumed.
	// alignedExtents reads raise only in its baseline case, so clearing it here
	// changes no answer today: planted on its own, this line decides nothing and
	// no test in the package moves. Planted *together* with a raise added to one
	// of the keyword cases, the pair is caught — so the rule is guarded, by
	// whichever of the two a later change leaves standing. It is kept because it
	// is what makes the field's documented meaning true, and a stale displacement
	// travelling in a struct that calls it the displacement is the sort of thing
	// the next reader spends an afternoon on.
	in.Align, in.Raise = own, 0
	return in
}

// tabStop is the distance between two tab stops, which is what tab-size sets.
//
// A number is a count of space advances in the box's own font, which is why
// this needs the face; a length is itself. The initial value is 8, the width
// every terminal and every editor has used for a tab since they had one.
func (l *layouter) tabStop(b *Box, face *shape.Face) style.Unit {
	raw := strings.TrimSpace(b.Style["tab-size"])
	if n, ok := parseNumber(raw); ok {
		return l.br.Measure(face, " ", b.FontSize).Mul(n)
	}
	if v, ok := l.lengthOf(b, "tab-size", 0); ok && v >= 0 {
		return v
	}
	return l.br.Measure(face, " ", b.FontSize).Mul(8)
}

// lineClamp is how many lines CSS Overflow 4 lets this block show, or zero for
// no limit.
//
// Two spellings, and the second is not a synonym for the first. The unprefixed
// property is read on its own; the prefixed one is only the clamp when the two
// declarations that made it work in the engine it came from are there as well —
// §"Legacy" gives the trio as "display: -webkit-box", "-webkit-box-orient:
// vertical" and "-webkit-line-clamp". A document that writes only
// "-webkit-line-clamp" on an ordinary block is not asking for anything, and
// browsers give it nothing.
//
// A count of zero or less clamps nothing rather than clamping everything away:
// the value is an integer with no stated floor, and a block with no lines at all
// is not something a stylesheet can plausibly be asking for.
func (l *layouter) lineClamp(b *Box) int {
	if n, ok := positiveInteger(b.Style["line-clamp"]); ok {
		return n
	}
	if !strings.EqualFold(strings.TrimSpace(b.Style["display"]), "-webkit-box") ||
		!strings.EqualFold(strings.TrimSpace(b.Style["-webkit-box-orient"]), "vertical") {
		return 0
	}
	if n, ok := positiveInteger(b.Style["-webkit-line-clamp"]); ok {
		return n
	}
	return 0
}

// balanceCaps is the balanced width for each line, by the item it starts at.
//
// One number would not do, because §5.1 balances "each group of lines separated
// by a forced line break" on its own: a headline of two lines with a <br> in the
// middle is two groups of one, and balancing the pair together would move the
// break the author wrote. text-wrap-balance-004 is that exactly — a <section>
// with a <br> in it, checked against two <div>s balanced separately.
//
// The text indent belongs to the first group alone, for the same reason it
// belongs to the first line: §16.1 gives it to the first formatted line of the
// element, and the line after a <br> is not one.
//
// A nil result means no cap anywhere, which is what a box that does not balance
// gets and is what capAt reads as MaxUnit.
func (l *layouter) balanceCaps(b *Box, items []inlineItem, width, indent style.Unit) []style.Unit {
	if !strings.EqualFold(strings.TrimSpace(b.Style["text-wrap-style"]), "balance") {
		return nil
	}
	caps := make([]style.Unit, len(items))
	for i := range caps {
		caps[i] = style.MaxUnit
	}
	start := 0
	for i := 0; i <= len(items); i++ {
		if i < len(items) && !items[i].Forced {
			continue
		}
		ind := style.Unit(0)
		if start == 0 {
			ind = indent
		}
		w := l.br.BalanceWidth(items[start:i], width, ind)
		for j := start; j < i; j++ {
			caps[j] = w
		}
		start = i + 1
	}
	return caps
}

// lineHeight resolves the line-height property.
//
// "normal" is the face's own recommendation, which for the metrics this engine
// has means about 1.2 times the size — the figure every renderer uses when a
// face does not say otherwise. A bare number is a multiplier rather than a
// length, which is the one place in CSS where that is true and the reason
// line-height is usually written that way: a multiplier inherits as a ratio and
// a length inherits as a fixed distance.
func (l *layouter) lineHeight(b *Box) style.Unit {
	value := strings.ToLower(strings.TrimSpace(b.Style["line-height"]))
	if value == "" || value == "normal" {
		return l.normalLineHeight(b)
	}
	if n, ok := parseNumber(value); ok {
		return b.FontSize.Mul(n)
	}
	if length, ok := l.parseLength(b, "line-height"); ok {
		if v, ok := length.Resolve(b.FontSize, true); ok {
			return v
		}
	}
	return l.normalLineHeight(b)
}

// normalLineHeight is "line-height: normal".
//
// Worth recording what changed when the font finally got asked. Ahem states
// ascent 800, descent -200 and a line gap of zero, which comes to exactly one
// em — right for a face whose every glyph is an em square, and a figure no
// constant would have produced. Noto Sans comes to more than 1.2. Neither is
// something an engine can guess, which is the whole argument for asking.
func (l *layouter) normalLineHeight(b *Box) style.Unit {
	face, ok := l.fontFor(b)
	if !ok {
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	var gap float64
	if d := face.Descriptor(); d.Has(shape.MetricLineGap) {
		gap = float64(d.LineGap)
	}
	// One multiplication over the summed ratio rather than three over the terms.
	// A layout unit is a 64th of a pixel and each product is rounded to one, so
	// adding three rounded products is not the same number as rounding the sum —
	// it is out by up to a unit and a half, which is enough to move a line.
	h := b.FontSize.Mul((top - bottom + gap) / upem)
	if h <= 0 {
		// A face stating metrics that sum to nothing would collapse every line
		// on the page. It is not a value to pass on.
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	return h
}

// lineExtents is lineMetrics at a box's font size.
func (l *layouter) lineExtents(b *Box, face *shape.Face) (ascent, descent style.Unit, ok bool) {
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return 0, 0, false
	}
	return b.FontSize.Mul(top / upem), b.FontSize.Mul(-bottom / upem), true
}

// baselineOf is where the text sits within a line box.
//
// The line box is usually taller than the text, and the difference — the leading
// — is split equally above and below it. That is what makes a paragraph's lines
// evenly spaced rather than crowded against their tops.
func (l *layouter) baselineOf(b *Box, lineHeight style.Unit) style.Unit {
	face, ok := l.fontFor(b)
	if !ok {
		return lineHeight.Mul(0.8)
	}
	ascent, descent, ok := l.lineExtents(b, face)
	if !ok {
		return lineHeight.Mul(0.8)
	}
	halfLeading := lineHeight.Sub(ascent).Sub(descent).Div(2)
	return halfLeading.Add(ascent)
}

// faceForText is fontFor, with a substitution when the family the document asked
// for cannot set the text.
//
// The standard fourteen faces cover Latin and nothing else, so a document with a
// Hebrew word in it gets a face that cannot encode the letters — and the encoder
// substitutes a space for each, which means the word is absent from the page and
// from the text extracted out of it rather than showing as boxes a reader would
// notice. That is what checkGlyphs reports, and reporting it is not the same as
// fixing it: a caller that has a face with the letters should be able to say so.
//
// FallbackFontSet is how it says so, and this is the only place that asks. The
// substitution is reported, because it changes the metrics and therefore where
// every line breaks — the same reason a missing family is reported.
//
// Two limits, both deliberate and both visible in what they leave behind:
//
// It is per box. A box whose text mixes scripts that no single face covers keeps
// the family's face and reports the missing glyphs, because choosing one face
// for the box cannot help it. Cutting a run into per-face pieces is what
// shape.Stack does and it reaches into measurement, line breaking and the
// content stream; until that exists, this handles the common shape — a run of
// text that is all one script.
//
// It is cached per box rather than per family, because the answer depends on the
// text. Shaping a run to find out whether it is covered is not free, and
// itemsFor is on the hot path.
func (l *layouter) faceForText(b *Box) (*shape.Face, bool) {
	face, ok := l.fontFor(b)
	if !ok || b.Text == "" {
		return face, ok
	}
	if got, cached := l.textFaces[b]; cached {
		return got, got != nil
	}
	// The family's face is kept whatever it is missing, and the characters it
	// cannot set are moved one at a time by faceRunsFor.
	//
	// It did not used to be. A single missing character sent the *whole box* to
	// another face — so a sentence of English with one alef in it was set, every
	// word of it, in a face the author never named, with that face's metrics and
	// that face's line breaks. The finding said so, which was honest, and the
	// page was still wrong in a way nothing could undo downstream.
	//
	// The report stays exactly where it was, and asks exactly what it asked: can
	// one face set the whole of this text. That question is no longer how the
	// face is *chosen* — it is only how the caller is told that the family it
	// asked for could not set the paragraph — so what changes here is the page
	// and not the reporting.
	if missesVisible(face, b.Text) {
		if set, canFall := l.fontSet.(FallbackFontSet); canFall {
			bold := isBold(b.Style["font-weight"])
			italic := isItalic(b.Style["font-style"])
			if alt, found := set.FaceFor(b.Text, bold, italic); found {
				l.rec.ReportDetail(Finding{
					Rule: RuleFontFallback,
					Message: "no face for " + quoteValue(b.Style["font-family"]) +
						" could set this text, so " + quoteValue(alt.Name()) +
						" was used for it; the metrics and the line breaks will differ",
					Path:     PathOf(b.Element),
					Property: "font-family",
				})
			}
		}
	}
	l.textFaces[b] = face
	return face, true
}
