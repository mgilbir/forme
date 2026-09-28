package layout

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Radicals: MathML Core §3.3.3's <msqrt> and <mroot>.
//
// A square root is its base — the <msqrt>'s children, as a row — under a bar,
// after the radical sign: U+221A's glyph in the element's first available
// font, stretched along the block axis (§5.3.2) to reach from the bar's top
// to the bottom of the base's ink and a gap. A root is the same with an index,
// raised over the sign's foot and kerned into it by the font's constants.
//
// The sign and the bar are the element's own marks, drawn in its colour and
// only where it is visible (§3.3.3.1): the sign with its ink's top at the
// bar's top, the bar across the base.

// radicalSign is U+221A SQUARE ROOT, the radical glyph of §3.3.3.1.
const radicalSign = '√'

// mathSurd is the radical sign as the layout measures it: its advance, and how
// far it reaches above and below its own baseline; and the sign to draw.
type mathSurd struct {
	width, ascent, descent style.Unit
	draw                   mathGlyphDraw
}

// mathSurdFor is the radical sign stretched to a height.
//
// Where §5.3.2's algorithm fails — the font has no MATH table, or no
// construction for the sign — §3.3.3 has no other answer, and the sign is its
// glyph unstretched, as a browser draws it. A font with no glyph for U+221A
// draws no sign at all, which is reported as any missing glyph is.
func (l *layouter) mathSurdFor(b *Box, m mathFont, height style.Unit) mathSurd {
	text := string(radicalSign)
	if m.face == nil {
		return mathSurd{}
	}
	gid, ok := m.face.GlyphID(radicalSign)
	if !ok {
		l.checkGlyphs(b, m.face, text)
		return mathSurd{}
	}
	if l.overBudget() {
		// See mathOperatorDrawn.
		return mathSurd{}
	}
	if m.table == nil || m.scale == 0 {
		// A face with no glyph metrics of the MATH table's kind — one of the
		// standard faces, or any face with no MATH table — is measured as the
		// text is: its advance, and its character's ink.
		size := b.FontSize.Px()
		w, _ := style.FromPx(m.face.Measure(text, size))
		s := mathSurd{width: w, draw: mathGlyphDraw{text: text, face: m.face, size: b.FontSize}}
		if above, below, ok := m.face.InkExtent(text, size); ok {
			s.ascent, _ = style.FromPx(above)
			s.descent, _ = style.FromPx(below)
		}
		return s
	}
	st, ok := m.table.Stretch(gid, true, height.Px()/m.scale)
	if !ok {
		st = m.table.Glyph(gid)
	}
	st = l.mathBounded(b, st)
	return mathSurd{
		width: m.units(st.Width), ascent: m.units(st.Ascent), descent: m.units(st.Descent),
		draw: mathGlyphDrawOf(m.face, b.FontSize, st, gid, text),
	}
}

// mathGlyphDrawOf is a construction as it is drawn: as its text, where it is
// the glyph the text shapes to, and otherwise as glyphs.
func mathGlyphDrawOf(face *shape.Face, size style.Unit, st shape.MathStretch, gid int, text string) mathGlyphDraw {
	d := mathGlyphDraw{text: text, face: face, size: size}
	if len(st.Parts) == 0 && st.Glyph == gid {
		if gs, missing := face.ShapeGlyphs(text); missing == 0 && len(gs) == 1 && gs[0].GID == gid {
			return d
		}
	}
	d.glyphs = mathGlyphsOf(face, st)
	return d
}

// mathRadical is §3.3.3.2: a radical sign and a bar over a base whose
// content is laid out — an <msqrt>'s row, or an <mroot>'s first child.
func (l *layouter) mathRadical(b *Box, base mathContent) mathContent {
	m := l.mathFontFor(b)
	gap := m.constant(shape.MathRadicalDisplayStyleVerticalGap)
	if !mathStyleNormal(b) {
		gap = m.constant(shape.MathRadicalVerticalGap)
	}
	rule := m.constant(shape.MathRadicalRuleThickness)
	extra := m.constant(shape.MathRadicalExtraAscender)
	surd := l.mathSurdFor(b, m, rule.Add(gap).Add(base.inkAscent).Add(base.inkDescent))

	c := mathContent{width: surd.width.Add(base.width)}
	c.ascent = style.Max(base.ascent, base.inkAscent.Add(gap).Add(rule).Add(extra))
	c.descent = style.Max(base.descent, surd.ascent.Add(surd.descent).Add(extra).Sub(c.ascent))
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	// The base is a row or one child, whose own marks and glyphs are on
	// their fragments: only its children move.
	for _, k := range base.kids {
		k.x = k.x.Add(surd.width)
		c.kids = append(c.kids, k)
	}
	// The bar: its top RadicalExtraAscender below the top of the content,
	// across the base. The sign: its ink's top at the bar's top.
	top := c.ascent.Sub(extra)
	c.marks = append(c.marks, mathMark{x: surd.width, width: base.width, top: top, height: rule})
	c.glyphs = append(c.glyphs, mathGlyphMark{width: surd.width, baseline: top.Sub(surd.ascent), draw: surd.draw})
	if mathRTL(b) {
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   sourceOf(boxElement(b)),
			Message:  "a radical in a right-to-left formula has its sign on the right, and the sign is drawn as it faces in the font, not mirrored",
			Path:     PathOf(b.Element),
			Property: "direction",
		})
	}
	return c
}

// mathRoot is §3.3.3.3's <mroot>: the base under its radical as a square root
// is, B, and the index before it, kerned by RadicalKernBeforeDegree — no less
// than nought — and RadicalKernAfterDegree — no further back than the index is
// wide — with its bottom raised RadicalDegreeBottomRaisePercent of B's height
// above B's bottom.
//
// §3.3.3.3's line-ascent adds the index's line-ascent to its raised bottom
// and leaves out the index's line-descent, which its placement, a sentence
// later, puts under the index's baseline; and its line-descent adds the index's
// line-ascent where its bottom is meant. The extents here are the placement's,
// so an index with a descender stays inside the box.
func (l *layouter) mathRoot(b *Box, base, index *Box, containing style.Unit) mathContent {
	bl := l.mathBox(base, containing, mathStretch{})
	il := l.mathBox(index, containing, mathStretch{})
	inner := l.mathRadical(b, mathContent{width: bl.width, ascent: bl.ascent, descent: bl.descent,
		inkAscent: bl.inkAscent, inkDescent: bl.inkDescent, kids: []mathPlaced{{laid: bl}}})
	m := l.mathFontFor(b)
	before, after := mathRootKerns(m, il.width)
	raise := m.radicalDegreeBottomRaise(inner.ascent.Add(inner.descent)).Sub(inner.descent)
	shift := raise.Add(il.descent)
	x0 := before.Add(il.width).Add(after)

	c := mathContent{width: x0.Add(inner.width)}
	c.ascent = style.Max(inner.ascent, shift.Add(il.ascent))
	c.descent = style.Max(inner.descent, il.descent.Sub(shift))
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	c.kids = append(c.kids, mathPlaced{laid: il, x: before, shift: shift})
	for _, k := range inner.kids {
		k.x = k.x.Add(x0)
		c.kids = append(c.kids, k)
	}
	for _, mk := range inner.marks {
		mk.x = mk.x.Add(x0)
		c.marks = append(c.marks, mk)
	}
	for _, g := range inner.glyphs {
		g.x = g.x.Add(x0)
		c.glyphs = append(c.glyphs, g)
	}
	return c
}

// mathRootKerns is §3.3.3.3's AdjustedRadicalKernBeforeDegree and
// AdjustedRadicalKernAfterDegree for an index of a width.
func mathRootKerns(m mathFont, index style.Unit) (before, after style.Unit) {
	return maxZero(m.constant(shape.MathRadicalKernBeforeDegree)),
		style.Max(style.Unit(0).Sub(index), m.constant(shape.MathRadicalKernAfterDegree))
}

// mathSurdWidth is §3.3.3.2's "preferred inline size of a glyph stretched
// along the block axis" for the radical sign: the widest of its glyph, its
// size variants and its assembly's pieces — the width it is given before the
// base it covers is known. A face with no MATH table gives the sign's own
// width.
func (l *layouter) mathSurdWidth(b *Box) style.Unit {
	m := l.mathFontFor(b)
	if m.face == nil {
		return 0
	}
	gid, ok := m.face.GlyphID(radicalSign)
	if !ok {
		return 0
	}
	if m.table == nil || m.scale == 0 {
		w, _ := style.FromPx(m.face.Measure(string(radicalSign), b.FontSize.Px()))
		return w
	}
	return m.units(m.table.PreferredStretchWidth(gid))
}
