package layout

import (
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Laying out MathML: MathML Core §3.
//
// A formula is a tree of boxes each of which places its children by an
// algorithm of its own — a row side by side on a shared baseline, a fraction
// one over the other about the math axis, a script raised past its base's ink
// — and each of those algorithms is written in terms of a few measurements of
// each child's margin box: its inline size, how far it reaches above and below
// its alphabetic baseline ("line-ascent" and "line-descent"), how far its ink
// reaches ("ink line-ascent" and "ink line-descent"), how far its last glyph
// leans past its advance (the italic correction), and where an accent above it
// is centred (the top accent attachment). mathLaid is those measurements, of a
// child laid out; mathContent is what an algorithm makes of its children: a
// "math content box" of its own measurements, and where each child goes in it.
//
// Every MathML box is a Fragment like any other — backgrounds, borders,
// padding, margins, visibility, colour and painting are the ones every box
// has — and what is MathML's is only where the fragments go. A token's text
// is laid out as the one line of text it is, by the inline layout every
// paragraph is laid out by, and then placed by its ink (see mathtoken.go);
// what a formula draws that is not a box or a line — a fraction bar, a
// radical, a stretched operator — hangs from the fragment that draws it (see
// mathpaint.go).
//
// The root <math> takes part in the page as the inline or the block box its
// display says it is, and its content is laid out by the <mrow> algorithm
// where block layout would have laid out children (see children in layout.go
// and mathBlockContent below). A MathML element whose display is not math is
// laid out as CSS says, and is placed in a formula as a box with a baseline
// (see mathCSSChild).

// mathLaid is a box of a formula, laid out: its fragment, and its measurements
// as the formula reads them — of its margin box, from its alphabetic baseline.
type mathLaid struct {
	box  *Box
	frag *Fragment

	width                 style.Unit
	ascent, descent       style.Unit
	inkAscent, inkDescent style.Unit

	italic    style.Unit
	hasItalic bool
	accent    style.Unit
	hasAccent bool
}

// mathStretch is §3.1.2's stretch size constraint: a target an embellished
// operator stretched along the block axis is to cover — an ink line-ascent and
// line-descent — or one stretched along the inline axis is to reach.
type mathStretch struct {
	block           bool
	ascent, descent style.Unit
	inline          bool
	size            style.Unit
}

func (s mathStretch) any() bool { return s.block || s.inline }

// mathContent is what an element's algorithm makes: its math content box, and
// where each child goes in it.
type mathContent struct {
	width                 style.Unit
	ascent, descent       style.Unit
	inkAscent, inkDescent style.Unit

	italic    style.Unit
	hasItalic bool
	accent    style.Unit
	hasAccent bool

	kids []mathPlaced
	// marks and glyphs are what the element draws that is not a child: see
	// mathMark and mathGlyphMark.
	marks  []mathMark
	glyphs []mathGlyphMark
	// centred says the math content box is centred along the inline axis in
	// a content box wider than it, where the other algorithms put it at the
	// inline start (§3.1.2).
	centred bool
}

// mathMark is a rule a formula draws that is not a box — a fraction bar, a
// radical's overbar — in its element's math content box: from x along the
// inline axis for width, its top edge top above the baseline, height tall.
// full says it spans the element's content box instead, which is wider than
// the math content box where the element states its width: a fraction bar
// does. See mathpaint.go.
type mathMark struct {
	x, width    style.Unit
	top, height style.Unit
	full        bool
}

// mathGlyphMark is a glyph construction a formula draws that is not a box's
// text: a stretched or enlarged operator, a radical sign. It is width wide,
// from x along the inline axis of its element's math content box, with its
// baseline baseline above the element's.
type mathGlyphMark struct {
	x, width style.Unit
	baseline style.Unit
	draw     mathGlyphDraw
}

// mathGlyphDraw is a construction as it is drawn: its origin, on its
// baseline, from the content box of the fragment that draws it; the text it
// stands for; and its glyphs, or none where it is drawn as that text.
type mathGlyphDraw struct {
	at     Point
	text   string
	glyphs []shape.Glyph
	face   *shape.Face
	size   style.Unit
}

// mathPlaced is a child where its algorithm puts it: the inline offset of its
// margin box, from the math content box's inline-start edge, and how far its
// baseline is raised above the content box's.
type mathPlaced struct {
	laid  mathLaid
	x     style.Unit
	shift style.Unit
}

// mathBox lays out one box of a formula: a MathML element by its algorithm, and
// anything else as CSS lays it out.
//
// containing is what a percentage in its margins, borders and padding is of.
// MathML Core leaves what that is open (issues 76 and 77); it is the width of
// the <math> element's content box, which is what every box inside it was
// laid out against before, and is what a <math> in a table cell knows.
//
// It recurses down the formula as deep as the box tree goes, which the box
// builder bounds (maxBoxDepth, and the tree builder's own maxDepth below it)
// and reports where it cuts: a formula nests no deeper than any other markup.
func (l *layouter) mathBox(b *Box, containing style.Unit, s mathStretch) mathLaid {
	if b.Inner != InnerMath {
		return l.mathCSSChild(b, containing)
	}
	l.ensureFontSize(b)
	margin := l.edges(b, "margin", containing)
	border := l.borderWidths(b)
	padding := l.paddingOf(b, containing)

	var frag *Fragment
	var content mathContent
	if isMathToken(b) {
		content, frag = l.mathToken(b, containing, margin, s)
	} else {
		content = l.mathContentOf(b, containing, s)
	}
	if frag == nil {
		frag = &Fragment{Box: b}
	}
	laid := l.mathWrap(b, frag, content, containing, margin, border, padding)
	if !isMathToken(b) {
		// A token's content is laid out by the block and inline layout that
		// queue what is positioned in it themselves.
		l.mathDeferPositioned(b, frag, frag.ContentRect().W)
	}
	return laid
}

// mathDeferPositioned queues the absolutely and fixed positioned children of a
// MathML element whose content its own algorithm lays out, to be placed once
// the tree is absolute.
//
// §3.1.2 lists "layout and positioning of absolutely-positioned and
// fixed-positioned boxes, as described in [CSS-POSITION-3]" as the last step
// of every algorithm, after the in-flow children — the only ones the
// algorithms themselves place (mathInFlow) — have their offsets. Leaving them
// out of the formula without queueing them anywhere drew nothing of them and
// said nothing about it.
//
// MathML Core gives an out-of-flow child no place in a row, a fraction or a
// script, so its static position — where §10.3.7 and §10.6.4 put a box that
// states no offset — is the one point of the element that none of them
// decides: the inline-start corner of its content box. As a point rather than
// a box it is measured from both edges, nought from the start and the content
// width from the end; the start is the right in a right-to-left formula.
func (l *layouter) mathDeferPositioned(b *Box, frag *Fragment, width style.Unit) {
	for _, c := range b.Children {
		if !c.Position.outOfFlow() {
			continue
		}
		x, end := style.Unit(0), width
		if mathRTL(b) {
			x, end = width, 0
		}
		l.deferAbsolute(c, frag, x, 0, end, 0)
	}
}

// mathContentOf is an element's algorithm, for every element but a token,
// whose content is text: see mathToken.
func (l *layouter) mathContentOf(b *Box, containing style.Unit, s mathStretch) mathContent {
	kids := mathInFlow(b)
	alg := l.mathAlgorithmOf(b, kids)
	if alg.why != "" {
		l.rec.ReportDetail(Finding{
			Rule:     RuleInvalidMarkup,
			Source:   sourceOf(boxElement(b)),
			Message:  alg.why,
			Path:     PathOf(b.Element),
			Property: mathName(b),
		})
	}
	switch alg.kind {
	case mathKindSpace:
		return l.mathSpace(b, containing)
	case mathKindPadded:
		return l.mathPadded(b, containing, s)
	case mathKindFraction:
		return l.mathFraction(b, kids[0], kids[1], containing, s)
	case mathKindScripts:
		return l.mathScripts(b, kids, alg.scripts, containing, s)
	case mathKindUnderOver:
		return l.mathUnderOver(b, kids, alg.under, alg.over, containing, s)
	case mathKindSqrt:
		// §3.3.3: the <msqrt>'s children are its base, an anonymous row.
		return l.mathRadical(b, l.mathRow(b, kids, containing, mathStretch{}))
	case mathKindRoot:
		return l.mathRoot(b, kids[0], kids[1], containing)
	}
	return l.mathRow(b, kids, containing, s)
}

// mathWrap turns a math content box into the element's box: the content box,
// sized by the content unless width or height says otherwise, inside its
// padding, border and margin, with its children placed in it. What it answers
// is the element's margin box measured as §3.1.2's extra steps say.
func (l *layouter) mathWrap(b *Box, frag *Fragment, c mathContent, containing style.Unit,
	margin, border, padding Edges) mathLaid {

	width := c.width
	if w, ok := l.explicitWidth(b, containing); ok && mathName(b) != "mspace" {
		width = w
	}
	ascent, descent := c.ascent, c.descent
	if h, ok := l.explicitHeight(b, containing, 0, false); ok && mathName(b) != "mspace" {
		descent = h.Sub(ascent)
	}
	frag.Box = b
	frag.Margin, frag.Border, frag.Padding = margin, border, padding
	frag.Outline, frag.outlineOffset = l.outlineWidth(b), l.outlineOffsetOf(b)
	frag.BorderRect.W = width.Add(padding.Horizontal()).Add(border.Horizontal())
	frag.BorderRect.H = ascent.Add(descent).Add(padding.Vertical()).Add(border.Vertical())
	l.mathPlace(b, frag, c, width, ascent)
	if dx := width.Sub(c.width); dx > 0 && mathRTL(b) {
		// §3.1.2: a content box wider than the math content box has the
		// latter at its inline-start edge, which in a right-to-left formula is
		// the right. mathPlace put the children there; a token's own line,
		// or the blocks of a token holding HTML, are moved to it here.
		for i := range frag.Lines {
			frag.Lines[i].move(dx, 0)
		}
		if isMathToken(b) {
			for _, k := range frag.Children {
				translate(k, dx, 0)
			}
		}
	}
	frag.mathBaseline, frag.hasMathBaseline = ascent, true
	if b.Position == PositionRelative {
		frag.Offset = l.relativeOffset(b, containing, 0, false)
	}
	if b.Position.positioned() {
		l.setPositioned(b, frag)
	}

	top := margin.Top.Add(border.Top).Add(padding.Top)
	bottom := margin.Bottom.Add(border.Bottom).Add(padding.Bottom)
	out := mathLaid{
		box: b, frag: frag,
		width:      width.Add(padding.Horizontal()).Add(border.Horizontal()).Add(margin.Horizontal()),
		ascent:     ascent.Add(top),
		descent:    descent.Add(bottom),
		inkAscent:  c.inkAscent,
		inkDescent: c.inkDescent,
	}
	// A border moves the ink edge on its side out to the border box.
	if border.Top > 0 {
		out.inkAscent = ascent.Add(padding.Top).Add(border.Top)
	}
	if border.Bottom > 0 {
		out.inkDescent = descent.Add(padding.Bottom).Add(border.Bottom)
	}
	start, end := margin.Left.Add(border.Left).Add(padding.Left), margin.Right.Add(border.Right).Add(padding.Right)
	if mathRTL(b) {
		start, end = end, start
	}
	if c.hasItalic {
		out.italic, out.hasItalic = c.italic.Add(end), true
	}
	if c.hasAccent {
		out.accent, out.hasAccent = c.accent.Add(start), true
	}
	return out
}

// mathPlace puts a content box's children in the fragment: each margin box at
// its inline offset — from the right in a right-to-left formula — and its
// baseline where the algorithm raised it.
func (l *layouter) mathPlace(b *Box, frag *Fragment, c mathContent, width, ascent style.Unit) {
	dx := style.Unit(0)
	if c.centred {
		dx = width.Sub(c.width).Div(2)
	}
	rtl := mathRTL(b)
	for _, k := range c.kids {
		x := dx.Add(k.x)
		if rtl {
			x = width.Sub(x).Sub(k.laid.width)
		}
		kf := k.laid.frag
		kf.BorderRect.X = x.Add(kf.Margin.Left)
		kf.BorderRect.Y = ascent.Sub(k.shift).Sub(k.laid.ascent).Add(kf.Margin.Top)
		frag.Children = append(frag.Children, kf)
	}
	for _, mk := range c.marks {
		x, w := dx.Add(mk.x), mk.width
		if mk.full {
			x, w = 0, width
		}
		if rtl {
			x = width.Sub(x).Sub(w)
		}
		frag.mathMarks = append(frag.mathMarks, Rect{X: x, Y: ascent.Sub(mk.top), W: w, H: mk.height})
	}
	for _, g := range c.glyphs {
		x := dx.Add(g.x)
		if rtl {
			x = width.Sub(x).Sub(g.width)
		}
		d := g.draw
		d.at = Point{X: x, Y: ascent.Sub(g.baseline)}
		frag.mathGlyphs = append(frag.mathGlyphs, d)
	}
}

// mathRTL reports whether a box's inline direction is right to left.
func mathRTL(b *Box) bool {
	return ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("direction")), "rtl")
}

// mathCSSChild places a box whose display is not math — an author's "display:
// block" on an <mrow>, an <mtable>'s inline table — in a formula: laid out as
// CSS lays out an inline-block, and measured by its first baseline, or its
// bottom margin edge where it has none.
//
// An <mtable> is the exception (§3.5.1): a CSS table in every other respect,
// "the center of the table is aligned with the math axis" — the axis of its
// own first available font — so that a matrix sits in the middle of the
// brackets around it, where a fraction bar would be, whatever its rows hold.
func (l *layouter) mathCSSChild(b *Box, containing style.Unit) mathLaid {
	frag := l.inlineBlockFragment(b, inlineFrame{Containing: containing})
	frag.BorderRect.X, frag.BorderRect.Y = 0, 0
	h := frag.MarginRect().H
	base := h
	if table := mathTableIn(b); table != nil {
		base = h.Div(2).Add(l.mathFontFor(table).constant(shape.MathAxisHeight))
	} else if v, ok := firstBaseline(frag); ok {
		base = v.Add(frag.Margin.Top)
	}
	w := frag.MarginRect().W
	return mathLaid{box: b, frag: frag, width: w, ascent: base, descent: h.Sub(base),
		inkAscent: base, inkDescent: h.Sub(base)}
}

// mathTableIn is the <mtable> a table wrapper holds, or nil.
func mathTableIn(b *Box) *Box {
	if !b.TableWrapper {
		return nil
	}
	for _, c := range b.Children {
		if c.Inner == InnerTable && c.Element != nil && c.Element.Namespace == html.NamespaceMathML &&
			c.Element.Name == "mtable" {
			return c
		}
	}
	return nil
}

// mathRow is §3.3.1.2's <mrow>: the children side by side on a shared
// baseline, each embellished operator spaced by its lspace and rspace, and a
// slanted child followed by the room its italic correction asks for.
//
// It is also every element whose algorithm is the row's: the grouping
// elements, the <math> root, an unknown MathML element, and the anonymous row
// an element wraps its children in.
func (l *layouter) mathRow(b *Box, kids []*Box, containing style.Unit, s mathStretch) mathContent {
	laid := l.mathStretchBlock(b, kids, containing, s)
	addSpace := mathName(b) == "math" || !l.mathClassOf(b).embellished
	var c mathContent
	x, italic := style.Unit(0), style.Unit(0)
	for i, k := range laid {
		slanted := !l.mathClassOf(k.box).embellished && k.hasItalic && k.italic != 0
		if !slanted {
			x = x.Add(italic)
		}
		op, isOp := l.mathOperator(k.box)
		if isOp && addSpace {
			x = x.Add(op.lspace)
		}
		c.kids = append(c.kids, mathPlaced{laid: laid[i], x: x})
		x = x.Add(k.width)
		italic = 0
		if slanted {
			italic = k.italic
		}
		if isOp && addSpace {
			x = x.Add(op.rspace)
		}
		// The greatest of the children's, which may be below nought where
		// every child is: a row holding a "-" reaches nowhere below its
		// baseline, and is less than nothing deep.
		if i == 0 {
			c.ascent, c.descent, c.inkAscent, c.inkDescent = k.ascent, k.descent, k.inkAscent, k.inkDescent
			continue
		}
		c.ascent = style.Max(c.ascent, k.ascent)
		c.descent = style.Max(c.descent, k.descent)
		c.inkAscent = style.Max(c.inkAscent, k.inkAscent)
		c.inkDescent = style.Max(c.inkDescent, k.inkDescent)
	}
	c.width = x.Add(italic)
	if len(laid) > 0 {
		// §3.3.1.2 sets the row's italic correction to its last child's; the
		// row has one wherever it has a last child that is slanted.
		c.italic, c.hasItalic = italic, italic != 0
	}
	return c
}

// mathStretchBlock lays out a row's children by §3.3.1.1's algorithm for
// stretching operators along the block axis.
//
// Where the row is itself an embellished operator being stretched, the one
// child that is an embellished operator is laid out with the same constraint
// and the rest with none. Otherwise the children that are embellished
// operators with the stretchy property and the block stretch axis are laid out
// last, to the ink the others reach above and below the baseline.
func (l *layouter) mathStretchBlock(b *Box, kids []*Box, containing style.Unit, s mathStretch) []mathLaid {
	out := make([]mathLaid, len(kids))
	if s.any() {
		for i, k := range kids {
			if l.mathClassOf(k).embellished {
				out[i] = l.mathBox(k, containing, s)
			} else {
				out[i] = l.mathBox(k, containing, mathStretch{})
			}
		}
		return out
	}
	var toStretch []int
	for i, k := range kids {
		if op, ok := l.mathOperator(k); ok && op.stretchy && !op.inlineAxis() {
			toStretch = append(toStretch, i)
			continue
		}
		out[i] = l.mathBox(k, containing, mathStretch{})
	}
	if len(toStretch) == 0 {
		return out
	}
	target := mathStretch{block: true}
	if len(toStretch) < len(kids) {
		for i := range kids {
			if out[i].frag == nil {
				continue
			}
			target.ascent = style.Max(target.ascent, out[i].inkAscent)
			target.descent = style.Max(target.descent, out[i].inkDescent)
		}
	}
	for _, i := range toStretch {
		out[i] = l.mathBox(kids[i], containing, target)
	}
	return out
}

// mathSpace is §3.2.5's <mspace>: as wide as its width property, as tall as
// its height property, with its baseline the height attribute below its top.
func (l *layouter) mathSpace(b *Box, containing style.Unit) mathContent {
	var c mathContent
	c.width, _ = l.explicitWidth(b, containing)
	height, _ := l.explicitHeight(b, containing, 0, false)
	// The requested line-ascent: the height attribute's value, clamped at
	// nought, where it is present, valid and not a percentage.
	if length, ok := l.mathLengthAttr(b, "height"); ok && length.Kind == style.LengthAbsolute {
		c.ascent = maxZero(length.Value)
	}
	c.descent = height.Sub(c.ascent)
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	return c
}

// mathPadded is §3.3.6's <mpadded>: its children as a row, in a box whose
// size and whose offset of the row are its attributes'.
//
// The requested depth, where the attribute gives none, is the inner row's
// line-descent. §3.3.6.1 says "line-ascent" there, beside a requested height
// that is the line-ascent for the same case; a depth of the row's height
// would move every <mpadded> that states only a height, and is not what the
// figure or any implementation does.
func (l *layouter) mathPadded(b *Box, containing style.Unit, s mathStretch) mathContent {
	inner := l.mathRow(b, mathInFlow(b), containing, s)
	req := func(name string, dflt style.Unit, clamp bool) style.Unit {
		length, ok := l.mathLengthAttr(b, name)
		if !ok || length.Kind != style.LengthAbsolute {
			return dflt
		}
		if clamp {
			return maxZero(length.Value)
		}
		return length.Value
	}
	height := req("height", inner.ascent, true)
	depth := req("depth", inner.descent, true)
	lspace := req("lspace", 0, true)
	voffset := req("voffset", 0, false)
	c := mathContent{width: inner.width, ascent: height, descent: depth,
		inkAscent: height, inkDescent: depth}
	for _, k := range inner.kids {
		k.x = k.x.Add(lspace)
		k.shift = k.shift.Add(voffset)
		c.kids = append(c.kids, k)
	}
	return c
}

// mathBlockContent is a MathML box that block layout reached: the <math> root,
// or any MathML box laid out as a block. Its content is laid out by its own
// algorithm into the fragment block layout made for it, centred along the
// inline axis where the box is block-level (§2.1.1), and its alphabetic
// baseline is recorded for whatever lines it up with something else — a line
// holding an inline <math>, a table row, a flex line.
func (l *layouter) mathBlockContent(b *Box, parent *Fragment, width style.Unit,
	topOpen, bottomOpen bool, origin flow) style.Unit {
	if isMathToken(b) {
		return l.mathTokenBlockContent(b, parent, width, topOpen, bottomOpen, origin)
	}
	c := l.mathContentOf(b, width, mathStretch{})
	inner := style.Max(width, c.width)
	if b.Outer == OuterBlock && width > c.width {
		c.centred = true
	} else {
		inner = c.width
	}
	l.mathPlace(b, parent, c, inner, c.ascent)
	parent.mathBaseline, parent.hasMathBaseline = c.ascent, true
	l.mathDeferPositioned(b, parent, width)
	return c.ascent.Add(c.descent)
}

// mathSize is a box's intrinsic inline sizes as a formula reads them: its
// min-content and max-content inline sizes, and its italic correction where it
// has one — which a row adds after a slanted child in these as it does when it
// lays the child out.
type mathSize struct {
	min, max  style.Unit
	italic    style.Unit
	hasItalic bool
	// accent is the box's top accent attachment, where it has one, which an
	// <mover> centres its overscript by.
	accent    style.Unit
	hasAccent bool
}

// mathContentSize is a MathML box's min-content and max-content inline sizes,
// of its math content box, by its algorithm (§3). It is not kept: it is asked
// once of each box, by its parent's or — for the box a CSS box holds — by
// contentWidths, which keeps it.
func (l *layouter) mathContentSize(b *Box) mathSize {
	var out mathSize
	switch name := mathName(b); {
	case isMathToken(b):
		out = l.mathTokenSize(b)
	case name == "mspace":
		w, _ := l.explicitWidth(b, 0)
		out = mathSize{min: w, max: w}
	default:
		kids := mathInFlow(b)
		switch alg := l.mathAlgorithmOf(b, kids); alg.kind {
		case mathKindFraction:
			n, d := l.mathOuterSize(kids[0]), l.mathOuterSize(kids[1])
			out = mathSize{min: style.Max(n.min, d.min), max: style.Max(n.max, d.max)}
		case mathKindScripts:
			min, max := l.mathSizesH(kids)
			italic := min[alg.scripts.base].italic
			loic, ic := l.mathScriptsItalics(kids[alg.scripts.base], italic)
			space := l.mathFontFor(b).constant(shape.MathSpaceAfterScript)
			out.min, _ = mathScriptsX(space, loic, ic, alg.scripts, min)
			out.max, _ = mathScriptsX(space, loic, ic, alg.scripts, max)
		case mathKindUnderOver:
			min, max := l.mathSizesH(kids)
			loic, _ := l.mathScriptsItalics(kids[0], min[0].italic)
			out.min, _ = mathUnderOverX(loic, min, 0, alg.under, alg.over)
			out.max, _ = mathUnderOverX(loic, max, 0, alg.under, alg.over)
		case mathKindSqrt:
			// §3.3.3.2: the radical sign's preferred width and the base's.
			out = l.mathRowSize(b, kids)
			surd := l.mathSurdWidth(b)
			out = mathSize{min: out.min.Add(surd), max: out.max.Add(surd)}
		case mathKindRoot:
			// §3.3.3.3: the kerns, the index and the square root of the base.
			base, index := l.mathOuterSize(kids[0]), l.mathOuterSize(kids[1])
			m := l.mathFontFor(b)
			surd := l.mathSurdWidth(b)
			for _, w := range []struct {
				into        *style.Unit
				base, index style.Unit
			}{{&out.min, base.min, index.min}, {&out.max, base.max, index.max}} {
				before, after := mathRootKerns(m, w.index)
				*w.into = before.Add(w.index).Add(after).Add(surd).Add(w.base)
			}
		default:
			out = l.mathRowSize(b, kids)
			if name == "mpadded" {
				if w, ok := l.explicitWidth(b, 0); ok {
					out = mathSize{min: w, max: w}
				}
			}
		}
	}
	return out
}

// mathOuterSize is a child's intrinsic sizes with its own edges, as its parent
// adds them up: a CSS box's as CSS measures it, and a MathML box's content
// sizes — or its width, where it states one — inside its padding, border and
// margin.
func (l *layouter) mathOuterSize(b *Box) mathSize {
	if b.Inner != InnerMath {
		w := l.outerWidths(b, 0)
		return mathSize{min: w.min, max: w.max}
	}
	w := l.mathContentSize(b)
	if v, ok := l.intrinsicLength(b, "width"); ok && mathName(b) != "mspace" {
		w.min, w.max = v, v
	}
	margin, border, padding := l.edges(b, "margin", 0), l.borderWidths(b), l.paddingOf(b, 0)
	edges := margin.Horizontal().Add(border.Horizontal()).Add(padding.Horizontal())
	w.min, w.max = w.min.Add(edges), w.max.Add(edges)
	start, end := margin.Left.Add(border.Left).Add(padding.Left), margin.Right.Add(border.Right).Add(padding.Right)
	if mathRTL(b) {
		start, end = end, start
	}
	if w.hasItalic {
		w.italic = w.italic.Add(end)
	}
	if w.hasAccent {
		w.accent = w.accent.Add(start)
	}
	return w
}

// mathSizesH is each child's intrinsic sizes as the inline half of a layout
// reads them: the min-content sizes, and the max-content ones.
func (l *layouter) mathSizesH(kids []*Box) (min, max []mathH) {
	for _, k := range kids {
		w := l.mathOuterSize(k)
		h := mathH{italic: w.italic, accent: w.accent, hasAccent: w.hasAccent}
		h.w = w.min
		min = append(min, h)
		h.w = w.max
		max = append(max, h)
	}
	return min, max
}

// mathRowSize is §3.3.1.2's min-content and max-content inline sizes of a row:
// its children's, with the operators' spacing, and the italic correction of a
// slanted child before the next child that is not slanted and at the end.
func (l *layouter) mathRowSize(b *Box, kids []*Box) mathSize {
	addSpace := mathName(b) == "math" || !l.mathClassOf(b).embellished
	var min, max, italic style.Unit
	for _, k := range kids {
		w := l.mathOuterSize(k)
		slanted := !l.mathClassOf(k).embellished && w.hasItalic && w.italic != 0
		if !slanted {
			min, max = min.Add(italic), max.Add(italic)
		}
		sp := style.Unit(0)
		op, isOp := l.mathOperator(k)
		if isOp && addSpace {
			sp = op.lspace.Add(op.rspace)
		}
		min, max = min.Add(w.min).Add(sp), max.Add(w.max).Add(sp)
		italic = 0
		if slanted {
			italic = w.italic
		}
	}
	return mathSize{min: min.Add(italic), max: max.Add(italic), italic: italic, hasItalic: italic != 0}
}

// mathTokenSize is a token's: its max-content width — nothing inside a formula
// wraps, as MathML treats white-space as nowrap (§2.2.2) — and, where it is
// one glyph, that glyph's italic correction, which is found by laying the
// token out.
//
// A stretchy operator of block axis is the exception (§3.2.4.3): its sizes are
// the widest form its font can draw it in, whatever it is stretched to later,
// where the font has a construction for it at all.
func (l *layouter) mathTokenSize(b *Box) mathSize {
	c, _ := l.mathToken(b, 0, Edges{}, mathStretch{})
	out := mathSize{min: c.width, max: c.width, italic: c.italic, hasItalic: c.hasItalic,
		accent: c.accent, hasAccent: c.hasAccent}
	if op, ok := l.mathOperator(b); ok && op.core == b && op.single && op.stretchy && !op.inlineAxis() && mathOnlyText(b) {
		if m := l.mathFontFor(b); m.table != nil {
			if gid, ok := m.face.GlyphID(mathMirrored(b, op.char)); ok && m.table.HasConstruction(gid, true) {
				w := m.units(m.table.PreferredStretchWidth(gid))
				out.min, out.max = w, w
			}
		}
	}
	return out
}

// isMathMLRoot reports whether a node is a <math> element.
func isMathMLRoot(n *html.Node) bool {
	return n != nil && n.Namespace == html.NamespaceMathML && n.Name == "math"
}
