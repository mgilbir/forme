package layout

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The font a part of a formula is set with, as MathML's layout asks about it.
//
// Every layout constant MathML Core uses comes from the element's first
// available font (§5): its MATH table where it has one, and otherwise the
// fallback §5.1 gives for each constant — a multiple of the font's default
// rule thickness, a fraction of its x-height, what its OS/2 table recommends
// for scripts, or nought. A formula set in a font with no MATH table is laid
// out by those fallbacks, which §5 warns is no guarantee of good rendering;
// that is reported once per face, where the formula is (see mathFontFor).

// mathFont is one element's first available font at its size.
type mathFont struct {
	face  *shape.Face
	table *shape.MathTable
	size  float64 // px
	// scale is pixels per font unit.
	scale float64
	desc  shape.Descriptor
}

// mathFontFor is the first available font of a box, with its MATH table. It
// is not kept: what it costs is fontFor's lookup and mathTableOf's, both kept
// already, and a copy of the face's descriptor.
func (l *layouter) mathFontFor(b *Box) mathFont {
	face, _ := l.fontFor(b)
	m := mathFont{face: face, size: b.FontSize.Px()}
	if face != nil {
		m.desc = face.Descriptor()
		if upem := face.UnitsPerEm(); upem > 0 {
			m.scale = m.size / float64(upem)
		}
		m.table = l.mathTableOf(b, face)
	}
	return m
}

// mathTableOf is a face's MATH table, asked for once per face in a layout —
// Face.MathTable hands out a table of its own each time it is asked — with
// what reading it could not do reported the first time.
func (l *layouter) mathTableOf(b *Box, face *shape.Face) *shape.MathTable {
	if t, ok := l.mathTables[face]; ok {
		return t
	}
	if l.mathTables == nil {
		l.mathTables = map[*shape.Face]*shape.MathTable{}
	}
	t, err := face.MathTable()
	l.mathTables[face] = t
	src := sourceOf(boxElement(b))
	switch {
	case err != nil:
		l.rec.ReportDetail(Finding{
			Rule:     RuleFontUndecodable,
			Source:   src,
			Message:  "the font " + quoteValue(face.Name()) + " has a MATH table this engine cannot read (" + err.Error() + "), so the formula is laid out by MathML Core's fallbacks",
			Property: "font-family",
		})
	case t == nil:
		// A face with no MATH table sets a formula by §5.1's fallbacks,
		// which is a formula laid out by rules meant for the case where
		// nothing better is known. It is said once per face, at the first
		// formula set in it.
		l.rec.ReportDetail(Finding{
			Rule:   RuleMathFallback,
			Source: src,
			Message: "the font " + quoteValue(face.Name()) + " has no MATH table, so the formula is " +
				"laid out by MathML Core's fallback constants and no operator is stretched",
			Property: "font-family",
		})
	}
	for _, limit := range t.Limits() {
		l.rec.ReportDetail(Finding{
			Rule:     RuleLimit,
			Source:   src,
			Message:  limit + " (" + quoteValue(face.Name()) + ")",
			Property: "font-family",
		})
	}
	return t
}

// units is a length in the font's units, in pixels.
func (m mathFont) units(v float64) style.Unit {
	u, _ := style.FromPx(v * m.scale)
	return u
}

// em is a fraction of an em at the font's size.
func (m mathFont) em(f float64) style.Unit {
	u, _ := style.FromPx(f * m.size)
	return u
}

// ruleThickness is §5.1's default rule thickness: the font's underline
// thickness, or nought where it states none.
func (m mathFont) ruleThickness() style.Unit {
	if m.face != nil && m.desc.Has(shape.MetricUnderline) {
		return m.units(float64(m.desc.UnderlineThickness))
	}
	return 0
}

// xHeight is OS/2's sxHeight, or nought where the font states none.
func (m mathFont) xHeight() style.Unit {
	if m.face != nil && m.desc.Has(shape.MetricXHeight) {
		return m.units(float64(m.desc.XHeight))
	}
	return 0
}

// constant is one of §5.1's layout constants, in pixels: the MATH table's
// where the font has one, the fallback §5.1 gives otherwise. The three
// percentages are not lengths and are answered by percent.
func (m mathFont) constant(c shape.MathConstant) style.Unit {
	if v, ok := m.table.Constant(c); ok {
		return m.units(float64(v))
	}
	rt := m.ruleThickness()
	xh := m.xHeight()
	switch c {
	case shape.MathAxisHeight:
		return xh.Div(2)
	case shape.MathAccentBaseHeight:
		return xh
	case shape.MathSubscriptShiftDown:
		if sub, _, ok := m.face.ScriptOffsets(); ok {
			return m.units(float64(sub))
		}
	case shape.MathSubscriptTopMax, shape.MathSuperscriptBottomMaxWithSubscript:
		return xh.Mul(4).Div(5)
	case shape.MathSuperscriptShiftUp:
		if _, super, ok := m.face.ScriptOffsets(); ok {
			return m.units(float64(super))
		}
	case shape.MathSuperscriptBottomMin:
		return xh.Div(4)
	case shape.MathSubSuperscriptGapMin:
		return rt.Mul(4)
	case shape.MathSpaceAfterScript:
		return m.em(1.0 / 24)
	case shape.MathStackGapMin, shape.MathOverbarVerticalGap, shape.MathUnderbarVerticalGap,
		shape.MathFractionNumDisplayStyleGapMin, shape.MathFractionDenomDisplayStyleGapMin:
		return rt.Mul(3)
	case shape.MathStackDisplayStyleGapMin:
		return rt.Mul(7)
	case shape.MathFractionNumeratorGapMin, shape.MathFractionRuleThickness,
		shape.MathFractionDenominatorGapMin, shape.MathOverbarExtraAscender,
		shape.MathUnderbarExtraDescender, shape.MathRadicalRuleThickness,
		shape.MathRadicalExtraAscender:
		return rt
	case shape.MathRadicalVerticalGap:
		return rt.Mul(1.25)
	case shape.MathRadicalDisplayStyleVerticalGap:
		return rt.Add(xh.Div(4))
	case shape.MathRadicalKernBeforeDegree:
		return m.em(5.0 / 18)
	case shape.MathRadicalKernAfterDegree:
		return m.em(-10.0 / 18)
	}
	// "The default fallback constant", which is nought.
	return 0
}

// radicalDegreeBottomRaise is §5.1's radicalDegreeBottomRaisePercent of a
// length: the table's percentage of it, or 60%, truncated to the unit. It is
// worked in whole numbers, as the percentage is one: 0.6 has no exact binary
// fraction, and a length times it can fall a unit short of three fifths.
func (m mathFont) radicalDegreeBottomRaise(of style.Unit) style.Unit {
	percent := 60
	if v, ok := m.table.Constant(shape.MathRadicalDegreeBottomRaisePercent); ok {
		percent = v
	}
	return style.Unit(int64(of) * int64(percent) / 100)
}
