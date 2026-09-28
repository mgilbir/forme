package layout

import (
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Fractions: MathML Core §3.3.2's <mfrac>.
//
// The numerator is raised and the denominator lowered about the math axis —
// the height, above the baseline, a font centres its fraction bars and its
// "+" and "−" on — each by the larger of the font's own shift for it and what
// keeps its ink a gap clear of the bar. A fraction with no bar is a stack,
// whose two shifts are pushed apart evenly until the gap between the ink is
// the font's least. Both are centred on the wider of the two.
//
// The bar is not a box. It is drawn by the <mfrac>, across its content box,
// in its colour, centred on the axis (see mathpaint.go).

// mathFraction lays out an <mfrac> with its numerator and denominator.
//
// The numerator of an <mfrac> that is itself an embellished operator being
// stretched is stretched with it; the denominator never is.
func (l *layouter) mathFraction(b *Box, num, den *Box, containing style.Unit, s mathStretch) mathContent {
	n := l.mathBox(num, containing, s)
	d := l.mathBox(den, containing, mathStretch{})
	m := l.mathFontFor(b)
	normal := mathStyleNormal(b)
	pick := func(compact, display shape.MathConstant) style.Unit {
		if normal {
			return m.constant(display)
		}
		return m.constant(compact)
	}
	axis := m.constant(shape.MathAxisHeight)
	t := l.mathLineThickness(b, m)
	half := t.Div(2)

	c := mathContent{width: style.Max(n.width, d.width), centred: true}
	var up, down style.Unit
	if t > 0 {
		// §3.3.2.1.
		up = style.Max(pick(shape.MathFractionNumeratorShiftUp, shape.MathFractionNumeratorDisplayStyleShiftUp),
			axis.Add(half).Add(pick(shape.MathFractionNumeratorGapMin, shape.MathFractionNumDisplayStyleGapMin)).Add(n.inkDescent))
		down = style.Max(pick(shape.MathFractionDenominatorShiftDown, shape.MathFractionDenominatorDisplayStyleShiftDown),
			half.Add(pick(shape.MathFractionDenominatorGapMin, shape.MathFractionDenomDisplayStyleGapMin)).Add(d.inkAscent).Sub(axis))
		c.ascent = style.Max(style.Max(up.Add(n.ascent), d.ascent.Sub(down)), axis.Add(t.Sub(half)))
		c.descent = maxZero(style.Max(style.Max(n.descent.Sub(up), down.Add(d.descent)), half.Sub(axis)))
		// The bar: as wide as the content box, its middle on the axis. Its
		// top is the axis plus the half of it above; an odd thickness puts
		// the extra sixty-fourth of a pixel above.
		c.marks = append(c.marks, mathMark{top: axis.Add(t.Sub(half)), height: t})
	} else {
		// §3.3.2.2.
		up = pick(shape.MathStackTopShiftUp, shape.MathStackTopDisplayStyleShiftUp)
		down = pick(shape.MathStackBottomShiftDown, shape.MathStackBottomDisplayStyleShiftDown)
		gap := down.Sub(d.inkAscent).Add(up.Sub(n.inkDescent))
		if delta := pick(shape.MathStackGapMin, shape.MathStackDisplayStyleGapMin).Sub(gap); delta > 0 {
			up = up.Add(delta.Div(2))
			down = down.Add(delta.Sub(delta.Div(2)))
		}
		c.ascent = style.Max(up.Add(n.ascent), d.ascent.Sub(down))
		c.descent = maxZero(style.Max(n.descent.Sub(up), down.Add(d.descent)))
	}
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	c.kids = []mathPlaced{
		{laid: n, x: mathCentredIn(c.width, n.width), shift: up},
		{laid: d, x: mathCentredIn(c.width, d.width), shift: style.Unit(0).Sub(down)},
	}
	return c
}

// mathCentredIn is the offset that centres something of width w in width.
// An odd difference leaves the extra sixty-fourth of a pixel after it.
func mathCentredIn(width, w style.Unit) style.Unit {
	return width.Sub(w).Div(2)
}

// mathLineThickness is §3.3.2's fraction line thickness: the linethickness
// attribute where it is a <length-percentage> — a percentage of the font's
// FractionRuleThickness — and that FractionRuleThickness otherwise. A
// negative thickness is §3.3.2's nought: mathFraction draws a bar only where
// the thickness is more than nought.
//
// MathML 3 let the attribute be "thin", "medium", "thick" or a plain number,
// a multiple of the default. MathML Core does not, and a browser draws the
// default thickness for them; so does this engine, and it says so, since the
// author asked for a thickness and did not get it.
func (l *layouter) mathLineThickness(b *Box, m mathFont) style.Unit {
	dflt := m.constant(shape.MathFractionRuleThickness)
	v, present := b.Element.Attr("linethickness")
	if !present {
		return dflt
	}
	length, ok := l.mathLengthAttr(b, "linethickness")
	if !ok {
		why := "is not a length or a percentage"
		if mathLegacyThickness(v) {
			why = "is MathML 3's, which MathML Core does not have"
		}
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   sourceOf(boxElement(b)),
			Message:  "linethickness=" + quoteValue(v) + " " + why + ", so the fraction bar is the font's default thickness",
			Path:     PathOf(b.Element),
			Property: "linethickness",
		})
		return dflt
	}
	return mathResolve(length, dflt)
}

// mathLegacyThickness reports whether a linethickness value is one of MathML
// 3's forms: a keyword, or a number with no unit — digits, with a sign and a
// decimal point, as CSS writes a number. (strconv would also take "NaN",
// "Inf" and hexadecimal, which no stylesheet or attribute means.)
func mathLegacyThickness(v string) bool {
	v = ascii.Lower(ascii.TrimCSSSpace(v))
	switch v {
	case "thin", "medium", "thick":
		return true
	}
	if v != "" && (v[0] == '+' || v[0] == '-') {
		v = v[1:]
	}
	digits, dots := 0, 0
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] >= '0' && v[i] <= '9':
			digits++
		case v[i] == '.' && dots == 0:
			dots++
		default:
			return false
		}
	}
	return digits > 0
}
