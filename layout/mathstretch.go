package layout

import (
	"strconv"

	"github.com/mgilbir/forme/bidi"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Stretching and enlarging operators: MathML Core §3.2.4.3.
//
// An <mo> of one character is drawn by a construction of the first available
// font's MATH table, rather than as its text, in two cases. A stretchy one
// that a formula asks to cover something — a parenthesis beside a fraction,
// an arrow over a wide base — is the smallest of its glyph, its size variants
// and its glyph assembly that covers it (§5.3.2, shape.MathTable.Stretch). A
// large operator in display mathematics is its first size variant at least
// DisplayOperatorMinHeight tall, or its largest.
//
// Where the construction is the very glyph the operator's text is drawn with,
// the token is laid out as its text, as any token is, and only moved: a
// parenthesis beside a "1" is still the parenthesis, and is drawn as the text
// it is. Otherwise the token is the construction — its width, its ink, its
// italic correction — drawn as glyphs (DrawGlyphs), since no character names
// a size variant or a piece of an assembly.
//
// Where the font has no MATH table, no glyph for the character, or no
// construction for it along the axis asked, §3.2.4.3 lays the operator out as
// its text. The first is reported once for the face (see mathTableOf); the
// third is reported where the formula asked the operator to be larger than
// its glyph. In a right-to-left formula the glyph is the character's mirrored
// one (see mathGlyphFor), and a character the font cannot mirror is laid out
// as its text too, which is reported where it was to be stretched or
// enlarged.

// mathOpDrawn is what §3.2.4.3 decided an operator is drawn as.
type mathOpDrawn struct {
	st shape.MathStretch
	// delta is how far the construction is moved down from the baseline:
	// §3.2.4.3's Δ, which puts the middle of a stretched glyph at the middle
	// of what it was stretched to cover.
	delta style.Unit
	// gid is the operator's own glyph in the first available font: the one
	// its text is drawn with.
	gid int
}

// isBase reports whether the construction is the operator's own glyph.
func (d mathOpDrawn) isBase() bool { return len(d.st.Parts) == 0 && d.st.Glyph == d.gid }

// mathOperatorDrawn decides whether an <mo> is drawn as a construction of its
// first available font, and which; false where it is laid out as its text.
// op is the properties of the embellished operator the <mo> is the core of.
func (l *layouter) mathOperatorDrawn(b *Box, op *mathOp, s mathStretch) (mathOpDrawn, bool) {
	if !op.single || !mathOnlyText(b) {
		return mathOpDrawn{}, false
	}
	m := l.mathFontFor(b)
	if m.table == nil || m.scale == 0 {
		return mathOpDrawn{}, false
	}
	gid, got := mathGlyphFor(b, m.face, op.char)
	if got == mathGlyphUnmirrorable && mathWouldConstruct(b, op, s) {
		l.rec.ReportDetail(Finding{
			Rule:   RuleMathFallback,
			Source: sourceOf(boxElement(b)),
			Message: "the font " + quoteValue(m.face.Name()) + " has no mirrored form of " + describeRune(op.char) +
				" (no 'rtlm' form of its glyph, and no mirror character to draw), so in this right-to-left " +
				"formula it is laid out as its text, not stretched or enlarged",
			Path:     PathOf(b.Element),
			Property: "direction",
		})
	}
	if got != mathGlyphFound || l.overBudget() {
		// Past the layout's work budget nothing more is built: what is left
		// of the document is laid out empty, and the budget says so.
		return mathOpDrawn{}, false
	}
	fu := func(u style.Unit) float64 { return u.Px() / m.scale }
	switch {
	case op.stretchy && op.inlineAxis():
		if !s.inline {
			return mathOpDrawn{}, false
		}
		st, ok := m.table.Stretch(gid, false, fu(s.size))
		if !ok {
			l.mathNoConstruction(b, m, op, s.size > m.units(m.table.Glyph(gid).Width))
			return mathOpDrawn{}, false
		}
		return mathOpDrawn{st: l.mathBounded(b, st), gid: gid}, true
	case op.stretchy:
		if !s.block {
			return mathOpDrawn{}, false
		}
		glyph := m.table.Glyph(gid)
		height := m.units(glyph.Ascent).Add(m.units(glyph.Descent))
		ascent, descent := l.mathStretchTarget(b, m, op, s, height)
		st, ok := m.table.Stretch(gid, true, fu(ascent.Add(descent)))
		if !ok {
			l.mathNoConstruction(b, m, op, ascent.Add(descent) > height)
			return mathOpDrawn{}, false
		}
		st = l.mathBounded(b, st)
		a, d := m.units(st.Ascent), m.units(st.Descent)
		return mathOpDrawn{st: st, gid: gid, delta: a.Sub(d).Sub(ascent.Sub(descent)).Div(2)}, true
	case op.largeop && mathStyleNormal(b):
		// "Use the MathVariants table to try and find a glyph of height at
		// least DisplayOperatorMinHeight. If none is found, fall back to the
		// largest non-base glyph. If none is found, fall back to the layout
		// algorithm of 3.2.1.1." The table lists the glyph itself among its
		// variants, and it may be found: an operator already that tall is
		// its text.
		min := m.constant(shape.MathDisplayOperatorMinHeight)
		chosen, largest := -1, -1
		for _, v := range m.table.Variants(gid, true) {
			if m.units(float64(v.Advance)) >= min {
				chosen = v.Glyph
				break
			}
			if v.Glyph != gid {
				largest = v.Glyph
			}
		}
		if chosen < 0 {
			chosen = largest
		}
		if chosen < 0 {
			l.mathNoConstruction(b, m, op, true)
			return mathOpDrawn{}, false
		}
		return mathOpDrawn{st: m.table.Glyph(chosen), gid: gid}, true
	}
	return mathOpDrawn{}, false
}

// mathGlyphResult is what mathGlyphFor found.
type mathGlyphResult int

const (
	mathGlyphFound mathGlyphResult = iota
	// mathGlyphMissing is a face with no glyph for the character at all.
	mathGlyphMissing
	// mathGlyphUnmirrorable is a right-to-left formula and a character that
	// is to be mirrored, which the face has no mirrored form of.
	mathGlyphUnmirrorable
)

// mathGlyphFor is MathML Core's algorithm to "get a glyph corresponding to a
// character c given a directionality dir" (§5.3.2, Editor's Draft of 27 July
// 2026), in a face: the glyph an operator is stretched or enlarged from
// (§3.2.4.3), and a radical sign (§3.3.3.1), with its size variants and its
// assembly, since those are the glyph's.
//
// In a left-to-right formula it is the character's glyph. In a right-to-left
// one it is, in this order: the face's 'rtlm' form of that glyph (see
// shape.Face.MirroredForm); for a character that is Bidi_Mirrored, the glyph
// of its mirror character — "(" drawn as ")" — or nothing, where it has no
// mirror character (U+221A, U+2211, U+222B) or the face no glyph for that;
// and for any other character its own glyph. The 'rtlm' form comes first:
// it is the one a font draws the constructions of, where a line of text asks
// for Unicode's mirror first and the font's form only for what that left.
func mathGlyphFor(b *Box, face *shape.Face, c rune) (int, mathGlyphResult) {
	g, ok := face.GlyphID(c)
	if !ok {
		return 0, mathGlyphMissing
	}
	if !mathRTL(b) {
		return g, mathGlyphFound
	}
	if form, ok := face.MirroredForm(c); ok {
		return form, mathGlyphFound
	}
	if !bidi.Mirrored(c) {
		return g, mathGlyphFound
	}
	if m, ok := bidi.MirrorOf(c); ok {
		if gm, ok := face.GlyphID(m); ok {
			return gm, mathGlyphFound
		}
	}
	return 0, mathGlyphUnmirrorable
}

// mathWouldConstruct reports whether §3.2.4.3 would draw an operator as a
// construction of its font rather than as its text, were there a glyph to
// build it from: a stretchy one asked to cover something along its axis, or
// a large operator in display mathematics.
func mathWouldConstruct(b *Box, op *mathOp, s mathStretch) bool {
	switch {
	case op.stretchy && op.inlineAxis():
		return s.inline
	case op.stretchy:
		return s.block
	}
	return op.largeop && mathStyleNormal(b)
}

// mathBounded is a construction within the bound on an assembly, and charged
// to the layout's work budget: an assembly shape.MaxMathAssemblyGlyphs could
// not build to the whole of its target is built short of it and reported, and
// every assembly is charged its pieces. Once the budget is spent, no more are
// built (see mathOperatorDrawn and mathSurdFor), and the rest of the document
// is laid out empty, as it is past the budget for any other reason.
func (l *layouter) mathBounded(b *Box, st shape.MathStretch) shape.MathStretch {
	if st.Capped {
		l.rec.ReportDetail(Finding{
			Rule:     RuleLimit,
			Source:   sourceOf(boxElement(b)),
			Message:  "a stretched operator would take more than " + strconv.Itoa(shape.MaxMathAssemblyGlyphs) + " glyphs to reach the size asked, and is drawn with that many",
			Path:     PathOf(b.Element),
			Property: "stretchy",
		})
	}
	if len(st.Parts) > 0 {
		l.charge(b, len(st.Parts))
	}
	return st
}

// mathNoConstruction reports an operator the font cannot draw any larger,
// where the formula asked it to be.
func (l *layouter) mathNoConstruction(b *Box, m mathFont, op *mathOp, asked bool) {
	if !asked {
		return
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleMathFallback,
		Source: sourceOf(boxElement(b)),
		Message: "the font " + quoteValue(m.face.Name()) + " has no larger forms of " + describeRune(op.char) +
			", so it is drawn at its text size where the formula asks for it larger",
		Path:     PathOf(b.Element),
		Property: "stretchy",
	})
}

// mathStretchTarget is §3.2.4.3's target for an operator stretched along the
// block axis: the ink the constraint asks it to cover, made symmetric about
// the math axis where the operator is symmetric, and brought within its
// minsize and maxsize — a percentage of either being of height, the height of
// the operator's own glyph. minsize is 100% where the operator states none,
// so that a parenthesis alone is its own height; maxsize is unbounded.
func (l *layouter) mathStretchTarget(b *Box, m mathFont, op *mathOp, s mathStretch, height style.Unit) (ascent, descent style.Unit) {
	axis := m.constant(shape.MathAxisHeight)
	ascent, descent = s.ascent, s.descent
	if op.symmetric {
		half := style.Max(ascent.Sub(axis), descent.Add(axis))
		ascent, descent = half.Add(axis), half.Sub(axis)
	}
	minsize := height
	if length, ok := l.mathLengthAttr(b, "minsize"); ok {
		minsize = mathResolve(length, height)
	}
	minsize = maxZero(minsize)
	maxsize, bounded := style.Unit(0), false
	if length, ok := l.mathLengthAttr(b, "maxsize"); ok {
		maxsize, bounded = style.Max(mathResolve(length, height), minsize), true
	}
	// "(Tascent − AxisHeight) × size / T + AxisHeight", which keeps an
	// operator symmetric about the axis symmetric.
	scaled := func(size, t style.Unit) (style.Unit, style.Unit) {
		a := maxZero(ascent.Sub(axis).Mul(float64(size) / float64(t)).Add(axis))
		return a, size.Sub(a)
	}
	switch t := ascent.Add(descent); {
	case t <= 0:
		ascent = minsize.Div(2).Add(axis)
		descent = minsize.Sub(ascent)
	case t < minsize:
		ascent, descent = scaled(minsize, t)
	case bounded && maxsize < t:
		ascent, descent = scaled(maxsize, t)
	}
	return ascent, descent
}

// mathOpContent is an operator's math content box where it is drawn as a
// construction: the construction's advance and ink, moved down by Δ, and its
// italic correction (§3.2.4.3's last paragraph), with the construction to draw.
func (l *layouter) mathOpContent(b *Box, d mathOpDrawn, text string) mathContent {
	m := l.mathFontFor(b)
	if shadow := ascii.TrimCSSSpace(b.Style.Get("text-shadow")); len(l.decorationsFor(b)) > 0 ||
		shadow != "" && !ascii.EqualFold(shadow, "none") {
		// The lines and shadows a run of text is drawn with are the run's,
		// and a construction is not a run: they are not drawn under it.
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   sourceOf(boxElement(b)),
			Message:  "a stretched or enlarged operator is drawn as its font's glyphs, without the text-decoration or text-shadow its text would have had",
			Path:     PathOf(b.Element),
			Property: "text-decoration",
		})
	}
	c := mathContent{
		width:   m.units(d.st.Width),
		ascent:  m.units(d.st.Ascent).Sub(d.delta),
		descent: m.units(d.st.Descent).Add(d.delta),
		italic:  m.units(float64(d.st.ItalicsCorrection)), hasItalic: true,
	}
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	c.glyphs = append(c.glyphs, mathGlyphMark{width: c.width, baseline: style.Unit(0).Sub(d.delta),
		draw: mathGlyphDraw{text: text, glyphs: mathGlyphsOf(m.face, d.st), face: m.face, size: b.FontSize}})
	return c
}

// mathGlyphsOf is a construction as the glyphs a DrawGlyphs draws: one glyph
// advancing by its width, or an assembly's pieces each placed by its offset
// and advancing nothing — in thousandths of an em, as shaping states them.
func mathGlyphsOf(face *shape.Face, st shape.MathStretch) []shape.Glyph {
	k := 1000 / float64(face.UnitsPerEm())
	if len(st.Parts) == 0 {
		return []shape.Glyph{{GID: st.Glyph, XAdvance: st.Width * k}}
	}
	out := make([]shape.Glyph, len(st.Parts))
	for i, p := range st.Parts {
		out[i] = shape.Glyph{GID: p.Glyph, XOffset: p.X * k, YOffset: p.Y * k}
	}
	return out
}
