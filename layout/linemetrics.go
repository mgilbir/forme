package layout

import (
	"github.com/mgilbir/forme/paragraph"
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
func (l *layouter) strutFor(b *Box) strut { return l.strutAt(b, b.FontSize) }

// strutAt is strutFor at a size other than the box's own, which is what
// text-fit's scaling produces: css-text-5 scales the size the text is *set* in
// without changing the computed font-size, so the strut is measured at the one
// and every font-relative length the document wrote is still resolved against
// the other.
func (l *layouter) strutAt(b *Box, size style.Unit) strut {
	h := l.lineHeightAt(b, size)
	s := strut{Height: h, Baseline: l.baselineAt(b, h, size)}
	face, ok := l.fontFor(b)
	if !ok {
		return s
	}
	d := face.Descriptor()
	upem := float64(face.UnitsPerEm())
	if upem == 0 {
		return s
	}
	if ascent, descent, ok := l.lineExtentsAt(b, face, size); ok {
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
	s.XHeight = size.Mul(0.5)
	switch {
	case d.Has(shape.MetricXHeight) && d.XHeight > 0:
		s.XHeight = size.Mul(float64(d.XHeight) / upem)
	case d.CapHeight > 0:
		// Cap height is reported, and x-height is about seven tenths of it for
		// the faces that report either.
		s.XHeight = size.Mul(float64(d.CapHeight) / upem * 0.7)
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
	face, _ := l.fontFor(b)
	return l.leadingInFace(b, face)
}

// leadingAt is leading at a size other than the box's own. See strutAt.
func (l *layouter) leadingAt(b *Box, size style.Unit) (above, below style.Unit) {
	face, _ := l.fontFor(b)
	return l.leadingInFaceAt(b, face, size)
}

// leadingInFace is leading for a run set in a face the box did not declare.
//
// §10.8.1 measures an inline box's leading against "the font", and a run the
// first available font could not set is not in that font: the fallback stack
// found another, and it is the other one's ascent and descent that decide how
// far the run reaches. A Japanese word in a paragraph declared in a Latin serif
// was laid out to the serif's metrics — the same 111.6px at 100px whatever set
// it — and the line was as much too short as the two faces differ.
//
// It changes nothing for the run the declared face *can* set, which is every run
// of almost every document: the face passed in is then the face fontFor returns
// and the two answers are one.
//
// It is asked only where the line-height is "normal", and the boundary is not
// where it first looks. A declared line-height fixes the *height* whatever the
// font — but the half-leading inside it is what is left once the face's own
// ascent and descent are taken out, so asking this for a run in another face
// gives that run a different baseline offset from the rest of its box, and the
// line grows past the height the document declared.
//
// §10.8.1 puts the half-leading on the inline *box*, and a run the fallback
// stack found is not a box of its own. Under "normal" the question does not
// arise: the height is the ascent and the descent, so there is no leading to
// halve and every run sits on the one baseline with its own extents above and
// below it — which is the union the specification asks for.
//
// Measured: asked for a declared line-height as well, it costs 18 clean passes
// and takes line-height-201 — its own family's fixture for exactly that — with
// it.
func (l *layouter) leadingInFace(b *Box, face *shape.Face) (above, below style.Unit) {
	return l.leadingInFaceAt(b, face, b.FontSize)
}

func (l *layouter) leadingInFaceAt(b *Box, face *shape.Face, size style.Unit) (above, below style.Unit) {
	h := l.lineHeightInFaceAt(b, face, size)
	above = l.baselineInFaceAt(b, face, h, size)
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
// A length is itself. A number is a count of *space advances*, and §8.1 says
// which space: "a <number> represents the measure as a multiple of the space
// character's advance width (U+0020), including its associated letter-spacing
// and word-spacing", measured "in the block container".
//
// Both halves of that were wrong, and each is a suite test by name.
//
// The measure is taken in the block container and not in the box the tab is in.
// A tab inside a <span> set larger than the paragraph took the span's space and
// came out however much wider the span was, so a run of tabbed lines stopped
// lining up the moment one of them had a word emphasised in it.
// tab-size-block-ancestor and tab-size-integer-004 and -005 are that.
//
// The *value*, though, is the inline's own: tab-size applies to inline boxes,
// so a span may set stops of its own. Those are two different questions about
// two different boxes, and tab-size-inline-001 and -002 are what says so.
//
// And the space's advance *including* the two spacings, which is not the same as
// the advance the shaper returns: an author who tracks a paragraph out by two
// pixels has widened every character in it, the space among them, so the tab
// stop is eight of the wider space rather than eight of the narrower one plus
// two. tab-size-spacing-001, -002 and -003 are that.
func (l *layouter) tabStop(b *Box, face *shape.Face) style.Unit {
	// The *value* is the one on the box the tab is in. tab-size applies to
	// inline boxes, so "<span style='tab-size: 5'>" sets the stops for the tabs
	// inside it whatever the paragraph around it asked for —
	// tab-size-inline-001 and -002 are that, and are the half a fix keyed on the
	// block alone gets wrong.
	if v, ok := l.lengthOf(b, "tab-size", 0); ok && v >= 0 &&
		!isNumberValue(b.Style["tab-size"]) {
		return v
	}
	n := 8.0
	if got, ok := parseNumber(strings.TrimSpace(b.Style["tab-size"])); ok && got >= 0 {
		// Non-negative for the same reason line-height is: CSS Text gives
		// tab-size a range of its own and the parser does not.
		n = got
	}
	// The *measure* is the block container's. A text box's parent chain reaches
	// one: the box the tab is in is inline, or is the block itself.
	block := b
	for block.Parent != nil && block.Outer != OuterBlock {
		block = block.Parent
	}
	return l.spaceAdvance(block, face).Mul(n)
}

// spaceAdvance is what one space costs in a box: the glyph's advance and the two
// spacings that go with it.
//
// A tab is measured in these, so the face has to be the one the *block* sets
// rather than the one the tab happens to sit in. Where the block has no face of
// its own to ask — a fragment tree built by hand, a family that loaded nothing —
// the caller's face stands in, which is the answer that was there before any of
// this and is right for every document whose block and inline agree.
func (l *layouter) spaceAdvance(block *Box, fallback *shape.Face) style.Unit {
	face := fallback
	if f, ok := l.fontFor(block); ok {
		face = f
	}
	if face == nil {
		return 0
	}
	// And the face that would *set* the space, which is not always the block's
	// first family. §8.1 names a character, and a character is set in the first
	// available font that has a glyph for it — a family with no U+0020 in it
	// answers with .notdef, whose advance has nothing to do with a space.
	// tab-size-integer-005 declares such a font first and says so in a comment
	// of its own.
	if f, ok := l.faceWithGlyph(block, ' '); ok {
		face = f
	}
	s := l.spacingFor(block)
	// Measured the way the block's own lines are measured. A tab stop is a
	// multiple of the space's advance, and on an upright vertical line a space
	// advances one em like every other character — so a stop counted in the
	// face's horizontal advance is a stop in the wrong unit, and a tab in such a
	// block landed at three fifths of the column it belongs in.
	return l.br.MeasureSpacedInContext(face, " ", block.FontSize,
		paragraph.TextSpacing{},
		shaping{ContextKerns: true, Upright: l.uprightText(block), Off: l.featuresFor(block)}).
		Add(s.Letter).Add(s.Word)
}

// isNumberValue reports whether a value is a bare number rather than a length,
// which is what tells tab-size's two forms apart.
func isNumberValue(raw string) bool {
	_, ok := parseNumber(strings.TrimSpace(raw))
	return ok
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
	face, _ := l.fontFor(b)
	return l.lineHeightInFace(b, face)
}

// lineHeightAt is lineHeight at a size other than the box's own. See strutAt.
func (l *layouter) lineHeightAt(b *Box, size style.Unit) style.Unit {
	face, _ := l.fontFor(b)
	return l.lineHeightInFaceAt(b, face, size)
}

// usesNormalLineHeight reports whether a box's line-height comes from its font
// rather than from a value the document wrote, which is the only case where the
// face a run is set in can change how tall the run is.
func usesNormalLineHeight(b *Box) bool {
	value := strings.ToLower(strings.TrimSpace(b.Style["line-height"]))
	return value == "" || value == "normal"
}

// lineHeightInFace is lineHeight for a run set in a face the box did not
// declare. See leadingInFace.
func (l *layouter) lineHeightInFace(b *Box, face *shape.Face) style.Unit {
	return l.lineHeightInFaceAt(b, face, b.FontSize)
}

// lineHeightInFaceAt is lineHeightInFace at a size other than the box's own.
//
// Only "normal" reads that size. A declared line-height is a value of the
// document's, and css-text-5 is explicit that text-fit "does not affect the
// font-size computed value, and thus does not affect font-size-relative length
// values of other properties. For example, 'line-height: 1.5em' ... [is] not
// affected". So the two branches below resolve against the box's own size and
// not the scaled one, and the suite's grow-per-line-all writes both cases side
// by side to say so.
func (l *layouter) lineHeightInFaceAt(b *Box, face *shape.Face, size style.Unit) style.Unit {
	value := strings.ToLower(strings.TrimSpace(b.Style["line-height"]))
	if value == "" || value == "normal" {
		return l.normalLineHeightInFaceAt(b, face, size)
	}
	if n, ok := parseNumber(value); ok && n >= 0 {
		// A negative multiplier is not a line-height: §10.8.1 says the value
		// "must be non-negative", so it is invalid and the property falls back
		// as though nothing had been said. The parser takes a sign because CSS's
		// <number> has one; the range is this property's own.
		return b.FontSize.Mul(n)
	}
	if length, ok := l.parseLength(b, "line-height"); ok {
		if v, ok := length.Resolve(b.FontSize, true); ok {
			return v
		}
	}
	return l.normalLineHeightInFaceAt(b, face, size)
}

// normalLineHeight is "line-height: normal".
//
// Worth recording what changed when the font finally got asked. Ahem states
// ascent 800, descent -200 and a line gap of zero, which comes to exactly one
// em — right for a face whose every glyph is an em square, and a figure no
// constant would have produced. Noto Sans comes to more than 1.2. Neither is
// something an engine can guess, which is the whole argument for asking.
func (l *layouter) normalLineHeight(b *Box) style.Unit {
	face, _ := l.fontFor(b)
	return l.normalLineHeightInFace(b, face)
}

func (l *layouter) normalLineHeightInFace(b *Box, face *shape.Face) style.Unit {
	return l.normalLineHeightInFaceAt(b, face, b.FontSize)
}

func (l *layouter) normalLineHeightInFaceAt(b *Box, face *shape.Face, size style.Unit) style.Unit {
	if face == nil {
		return size.Mul(normalLineHeightFallbackFactor)
	}
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return size.Mul(normalLineHeightFallbackFactor)
	}
	var gap float64
	if d := face.Descriptor(); d.Has(shape.MetricLineGap) {
		gap = float64(d.LineGap)
	}
	// One multiplication over the summed ratio rather than three over the terms.
	// A layout unit is a 64th of a pixel and each product is rounded to one, so
	// adding three rounded products is not the same number as rounding the sum —
	// it is out by up to a unit and a half, which is enough to move a line.
	h := size.Mul((top - bottom + gap) / upem)
	if h <= 0 {
		// A face stating metrics that sum to nothing would collapse every line
		// on the page. It is not a value to pass on.
		return size.Mul(normalLineHeightFallbackFactor)
	}
	return h
}

// lineExtents is lineMetrics at a box's font size.
func (l *layouter) lineExtents(b *Box, face *shape.Face) (ascent, descent style.Unit, ok bool) {
	return l.lineExtentsAt(b, face, b.FontSize)
}

func (l *layouter) lineExtentsAt(b *Box, face *shape.Face, size style.Unit) (ascent, descent style.Unit, ok bool) {
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return 0, 0, false
	}
	return size.Mul(top / upem), size.Mul(-bottom / upem), true
}

// baselineOf is where the text sits within a line box.
//
// The line box is usually taller than the text, and the difference — the leading
// — is split equally above and below it. That is what makes a paragraph's lines
// evenly spaced rather than crowded against their tops.
func (l *layouter) baselineOf(b *Box, lineHeight style.Unit) style.Unit {
	face, _ := l.fontFor(b)
	return l.baselineInFace(b, face, lineHeight)
}

// baselineAt is baselineOf at a size other than the box's own. See strutAt.
func (l *layouter) baselineAt(b *Box, lineHeight, size style.Unit) style.Unit {
	face, _ := l.fontFor(b)
	return l.baselineInFaceAt(b, face, lineHeight, size)
}

func (l *layouter) baselineInFace(b *Box, face *shape.Face, lineHeight style.Unit) style.Unit {
	return l.baselineInFaceAt(b, face, lineHeight, b.FontSize)
}

func (l *layouter) baselineInFaceAt(b *Box, face *shape.Face, lineHeight, size style.Unit) style.Unit {
	if face == nil {
		return lineHeight.Mul(0.8)
	}
	ascent, descent, ok := l.lineExtentsAt(b, face, size)
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
	// What the caller is told is decided in itemsFor, once the runs are known —
	// see noteSubstitution. It cannot be decided here: the question worth asking
	// is which characters actually moved, and the runs are what that is.
	l.textFaces[b] = face
	return face, true
}
