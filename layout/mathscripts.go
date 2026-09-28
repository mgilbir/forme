package layout

import (
	"strconv"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Scripts: MathML Core §3.4's <msub>, <msup>, <msubsup>, <mmultiscripts>,
// <munder>, <mover> and <munderover>.
//
// Every one of them is a base with scripts round it, and every one is laid out
// in two halves that do not meet: along the inline axis, where each child goes
// is a matter of widths, italic corrections and accent attachments alone; and
// along the block axis, how far each script is raised or lowered is a matter of
// ascents, descents and the font's constants alone. The inline half is written
// once, over mathH, and serves both the layout and the min-content and
// max-content sizes (§3.3.1.2's pattern: "calculated like the inline size, using
// the min-content inline size instead").
//
// Where the specification's text contradicts itself, the reading taken is the
// one that keeps a child inside the box the same algorithm gives its parent,
// and each is said where it is taken.

// mathH is what the inline-axis half of a layout reads of a child: its inline
// size (laid out, or one of its intrinsic sizes), its italic correction and its
// top accent attachment.
type mathH struct {
	w         style.Unit
	italic    style.Unit
	accent    style.Unit
	hasAccent bool
}

func mathHOf(k mathLaid) mathH {
	h := mathH{w: k.width, accent: k.accent, hasAccent: k.hasAccent}
	if k.hasItalic {
		h.italic = k.italic
	}
	return h
}

// mathScriptPair is a subscript and a superscript sharing a column, by their
// index among the element's in-flow children, or -1 where there is none: an
// <msub>'s one pair has no superscript, an <msup>'s no subscript.
type mathScriptPair struct{ sub, sup int }

// mathScriptsForm is the shape of a scripted element that §3.4 lays out by its
// own algorithm: the base, the pairs before it and after it, and the
// <mprescripts> between them, all by index among the in-flow children.
type mathScriptsForm struct {
	base       int
	pre, post  []mathScriptPair
	prescripts int
}

// mathKind is which of MathML Core's algorithms lays an element out.
type mathKind uint8

const (
	mathKindRow mathKind = iota
	mathKindSpace
	mathKindPadded
	mathKindFraction
	mathKindScripts
	mathKindUnderOver
)

// mathAlgorithm is an element's algorithm, with what it needs to know of the
// children: which is the base and which the scripts, or which is under and
// which over.
type mathAlgorithm struct {
	kind    mathKind
	scripts mathScriptsForm
	// under and over are the in-flow indices of an <munder>, <mover> or
	// <munderover>'s scripts, or -1.
	under, over int
	// why is the reason an element with an algorithm of its own is laid out as
	// a row: its children are not the ones the algorithm is written for.
	why string
}

// mathAlgorithmOf decides an element's algorithm from its name and its in-flow
// children. MathML Core lays out an element whose children are not the ones
// its algorithm is written for — an <mfrac> with three, an <msub> with one —
// as a row; that is the specification's answer and is done, and why says what
// was wrong, since the author wrote something MathML says is invalid.
func (l *layouter) mathAlgorithmOf(b *Box, kids []*Box) mathAlgorithm {
	count := func(n int) mathAlgorithm {
		return mathAlgorithm{kind: mathKindRow,
			why: "<" + mathName(b) + "> has " + strconv.Itoa(len(kids)) + " children, and MathML Core lays out one with any number but " +
				strconv.Itoa(n) + " as a row"}
	}
	name := mathName(b)
	switch name {
	case "mspace":
		return mathAlgorithm{kind: mathKindSpace}
	case "mpadded":
		return mathAlgorithm{kind: mathKindPadded}
	case "mfrac":
		if len(kids) != 2 {
			return count(2)
		}
		return mathAlgorithm{kind: mathKindFraction}
	case "msub", "msup", "munder", "mover":
		if len(kids) != 2 {
			return count(2)
		}
	case "msubsup", "munderover":
		if len(kids) != 3 {
			return count(3)
		}
	case "mmultiscripts":
		form, ok := mathMultiscriptsForm(kids)
		if !ok {
			return mathAlgorithm{kind: mathKindRow,
				why: "<mmultiscripts> is not a base followed by pairs of scripts, with at most one <mprescripts> and pairs after it, so MathML Core lays it out as a row"}
		}
		return mathAlgorithm{kind: mathKindScripts, scripts: form}
	default:
		return mathAlgorithm{kind: mathKindRow}
	}
	one := func(sub, sup int) mathAlgorithm {
		return mathAlgorithm{kind: mathKindScripts,
			scripts: mathScriptsForm{post: []mathScriptPair{{sub: sub, sup: sup}}, prescripts: -1}}
	}
	switch name {
	case "msub":
		return one(1, -1)
	case "msup":
		return one(-1, 1)
	case "msubsup":
		return one(1, 2)
	}
	// §3.4.2.1: an under- or overscript on an operator with movablelimits, in
	// compact mathematics, is a sub- or superscript.
	if !mathStyleNormal(b) {
		if op, ok := l.mathOperator(kids[0]); ok && op.movablelimits {
			switch name {
			case "munder":
				return one(1, -1)
			case "mover":
				return one(-1, 1)
			default:
				return one(1, 2)
			}
		}
	}
	switch name {
	case "munder":
		return mathAlgorithm{kind: mathKindUnderOver, under: 1, over: -1}
	case "mover":
		return mathAlgorithm{kind: mathKindUnderOver, under: -1, over: 1}
	}
	return mathAlgorithm{kind: mathKindUnderOver, under: 1, over: 2}
}

// mathMultiscriptsForm is §3.4.3's valid <mmultiscripts>: a base that is not
// an <mprescripts>, an even number of postscripts, and optionally an
// <mprescripts> followed by an even number of prescripts.
func mathMultiscriptsForm(kids []*Box) (mathScriptsForm, bool) {
	isPre := func(k *Box) bool { return mathName(k) == "mprescripts" }
	if len(kids) == 0 || isPre(kids[0]) {
		return mathScriptsForm{}, false
	}
	form := mathScriptsForm{prescripts: -1}
	i := 1
	pairs := func(into *[]mathScriptPair) bool {
		for i < len(kids) && !isPre(kids[i]) {
			if i+1 >= len(kids) || isPre(kids[i+1]) {
				return false
			}
			*into = append(*into, mathScriptPair{sub: i, sup: i + 1})
			i += 2
		}
		return true
	}
	if !pairs(&form.post) {
		return mathScriptsForm{}, false
	}
	if i < len(kids) {
		form.prescripts = i
		i++
		if !pairs(&form.pre) || i < len(kids) {
			return mathScriptsForm{}, false
		}
	}
	return form, true
}

// mathScriptsX is the inline half of §3.4.1 and §3.4.3: the inline size of the
// math content and each child's inline offset, by in-flow index.
//
// space is SpaceAfterScript; loic the base's italic correction where it is a
// large operator (a subscript moves back under it by that) and ic its italic
// correction where it is not (a superscript moves out past it by that).
//
// The prescripts come first, each pair right-aligned in a column as wide as
// its wider script and followed by SpaceAfterScript; then the base, and the
// <mprescripts> where the base is; then each pair of postscripts, its
// subscript at the column's start less loic and its superscript at the
// column's start plus ic.
//
// §3.4.3's inline size for the postscripts reads "x + the subscript's size +
// LargeOpItalicCorrection" and "x − ItalicCorrection" for the superscript: the
// opposite signs to the placement two paragraphs later and to §3.4.1's three
// elements, of which the note says a one-pair <mmultiscripts> is the same. The
// size here is the placement's, and each column's extent ends after its
// SpaceAfterScript, as <msub>'s and <msup>'s do; so the three elements and an
// <mmultiscripts> of one pair are one algorithm.
func mathScriptsX(space, loic, ic style.Unit, form mathScriptsForm, kids []mathH) (style.Unit, []style.Unit) {
	x := make([]style.Unit, len(kids))
	w := func(i int) style.Unit {
		if i < 0 {
			return 0
		}
		return kids[i].w
	}
	offset := style.Unit(0)
	for _, p := range form.pre {
		offset = offset.Add(space)
		col := style.Max(w(p.sub), w(p.sup))
		x[p.sub] = offset.Add(col).Sub(w(p.sub))
		x[p.sup] = offset.Add(col).Sub(w(p.sup))
		offset = offset.Add(col)
	}
	x[form.base] = offset
	if form.prescripts >= 0 {
		x[form.prescripts] = offset
	}
	offset = offset.Add(w(form.base))
	size := offset
	for _, p := range form.post {
		if p.sub >= 0 {
			x[p.sub] = offset.Sub(loic)
			size = style.Max(size, x[p.sub].Add(w(p.sub)).Add(space))
		}
		if p.sup >= 0 {
			x[p.sup] = offset.Add(ic)
			size = style.Max(size, x[p.sup].Add(w(p.sup)).Add(space))
		}
		offset = offset.Add(style.Max(w(p.sub), w(p.sup))).Add(space)
	}
	return size, x
}

// mathUnderOverX is the inline half of §3.4.2: the base, the underscript and
// the overscript centred on one another — the overscript by its top accent
// attachment where it has one — with the scripts of a large operator moved
// half its italic correction apart, the overscript forward and the
// underscript back. under and over are -1 where there is none.
//
// §3.4.2.4 measures the overscript's extent from its top accent attachment and
// then places it by half its width. The two agree only where the attachment is
// the middle, and the placement here is the extent's, so an accent whose
// attachment is not its middle is inside the box computed for it.
//
// An odd width is split with the extra sixty-fourth of a pixel after the
// middle, which keeps each child's extent exactly its width.
func mathUnderOverX(loic style.Unit, kids []mathH, base, under, over int) (style.Unit, []style.Unit) {
	x := make([]style.Unit, len(kids))
	half := loic.Div(2)
	// Each child's start, relative to the middle of the base.
	start := make([]style.Unit, len(kids))
	start[base] = style.Unit(0).Sub(kids[base].w.Div(2))
	if under >= 0 {
		start[under] = style.Unit(0).Sub(kids[under].w.Div(2)).Sub(half)
	}
	if over >= 0 {
		attach := kids[over].w.Div(2)
		if kids[over].hasAccent {
			attach = kids[over].accent
		}
		start[over] = style.Unit(0).Sub(attach).Add(half)
	}
	lo, hi := start[base], start[base].Add(kids[base].w)
	for _, i := range []int{under, over} {
		if i >= 0 {
			lo = style.Min(lo, start[i])
			hi = style.Max(hi, start[i].Add(kids[i].w))
		}
	}
	for _, i := range []int{base, under, over} {
		if i >= 0 {
			x[i] = start[i].Sub(lo)
		}
	}
	return hi.Sub(lo), x
}

// mathScriptsItalics is LargeOpItalicCorrection and ItalicCorrection of
// §3.4.1: a base that is an embellished operator with the largeop property
// gives its italic correction to its subscripts, pulling them back under it,
// and any other base gives it to its superscripts, pushing them out past it.
func (l *layouter) mathScriptsItalics(base *Box, italic style.Unit) (loic, ic style.Unit) {
	if op, ok := l.mathOperator(base); ok && op.largeop {
		return italic, 0
	}
	return 0, italic
}

// mathScripts lays out §3.4.1's three elements, §3.4.3's <mmultiscripts>, and
// an under- and overscript §3.4.2.1 turns into scripts. The base is laid out
// with the stretch constraint the element was given, and the scripts with none.
func (l *layouter) mathScripts(b *Box, kids []*Box, form mathScriptsForm, containing style.Unit, s mathStretch) mathContent {
	laid := make([]mathLaid, len(kids))
	for i, k := range kids {
		if i == form.base {
			laid[i] = l.mathBox(k, containing, s)
		} else {
			laid[i] = l.mathBox(k, containing, mathStretch{})
		}
	}
	base := laid[form.base]
	loic, ic := l.mathScriptsItalics(kids[form.base], mathHOf(base).italic)
	m := l.mathFontFor(b)
	hs := make([]mathH, len(laid))
	for i, k := range laid {
		hs[i] = mathHOf(k)
	}
	width, xs := mathScriptsX(m.constant(shape.MathSpaceAfterScript), loic, ic, form, hs)

	// The shifts, each the greatest any pair asks for (§3.4.3), and a pair
	// with both scripts pushed apart until the gap between them is the least
	// the font allows (§3.4.1.4).
	subShiftOf := func(sub mathLaid) style.Unit {
		return style.Max(style.Max(m.constant(shape.MathSubscriptShiftDown),
			sub.inkAscent.Sub(m.constant(shape.MathSubscriptTopMax))),
			m.constant(shape.MathSubscriptBaselineDropMin).Add(base.inkDescent))
	}
	up := shape.MathSuperscriptShiftUp
	if mathShiftCompact(b) {
		up = shape.MathSuperscriptShiftUpCramped
	}
	supShiftOf := func(sup mathLaid) style.Unit {
		return style.Max(style.Max(m.constant(up),
			m.constant(shape.MathSuperscriptBottomMin).Add(sup.inkDescent)),
			base.inkAscent.Sub(m.constant(shape.MathSuperscriptBaselineDropMax)))
	}
	var subShift, supShift style.Unit
	pairs := append(append([]mathScriptPair{}, form.pre...), form.post...)
	for _, p := range pairs {
		var down, rise style.Unit
		if p.sub >= 0 {
			down = subShiftOf(laid[p.sub])
		}
		if p.sup >= 0 {
			rise = supShiftOf(laid[p.sup])
		}
		if p.sub >= 0 && p.sup >= 0 {
			down, rise = l.mathSubSupGap(m, laid[p.sub], laid[p.sup], down, rise)
		}
		subShift, supShift = style.Max(subShift, down), style.Max(supShift, rise)
	}

	c := mathContent{width: width, ascent: base.ascent, descent: base.descent}
	c.kids = append(c.kids, mathPlaced{laid: base, x: xs[form.base]})
	if form.prescripts >= 0 {
		c.kids = append(c.kids, mathPlaced{laid: laid[form.prescripts], x: xs[form.prescripts]})
	}
	for _, p := range pairs {
		if p.sub >= 0 {
			k := laid[p.sub]
			c.ascent = style.Max(c.ascent, k.ascent.Sub(subShift))
			c.descent = style.Max(c.descent, k.descent.Add(subShift))
			c.kids = append(c.kids, mathPlaced{laid: k, x: xs[p.sub], shift: style.Unit(0).Sub(subShift)})
		}
		if p.sup >= 0 {
			k := laid[p.sup]
			c.ascent = style.Max(c.ascent, k.ascent.Add(supShift))
			c.descent = style.Max(c.descent, k.descent.Sub(supShift))
			c.kids = append(c.kids, mathPlaced{laid: k, x: xs[p.sup], shift: supShift})
		}
	}
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	return c
}

// mathSubSupGap is §3.4.1.4's adjustment of a pair's two shifts: where the gap
// between the subscript's ink and the superscript's is less than
// SubSuperscriptGapMin, the superscript is raised first — no further than
// puts its bottom at SuperscriptBottomMaxWithSubscript — and the subscript
// lowered by whatever is left.
func (l *layouter) mathSubSupGap(m mathFont, sub, sup mathLaid, down, rise style.Unit) (style.Unit, style.Unit) {
	min := m.constant(shape.MathSubSuperscriptGapMin)
	gap := down.Sub(sub.inkAscent).Add(rise.Sub(sup.inkDescent))
	if gap >= min {
		return down, rise
	}
	if delta := m.constant(shape.MathSuperscriptBottomMaxWithSubscript).Sub(rise.Sub(sup.inkDescent)); delta > 0 {
		delta = style.Min(delta, min.Sub(gap))
		rise, gap = rise.Add(delta), gap.Add(delta)
	}
	if delta := min.Sub(gap); delta > 0 {
		down = down.Add(delta)
	}
	return down, rise
}

// mathShiftCompact reports whether a box's math-shift is compact: TeX's
// "cramped", in which a superscript is raised less.
func mathShiftCompact(b *Box) bool {
	return ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("math-shift")), "compact")
}

// mathAccentAttr reports whether an <munder>, <mover> or <munderover>'s
// accent or accentunder attribute is "true", in any case.
func mathAccentAttr(b *Box, name string) bool {
	if b.Element == nil {
		return false
	}
	v, ok := b.Element.Attr(name)
	return ok && ascii.EqualFold(v, "true")
}

// mathUnderOver lays out §3.4.2's <munder>, <mover> and <munderover>, their
// children by §3.4.2.2's algorithm for stretching operators along the inline
// axis.
//
// How far a script is from the base depends on what the base is. Under and
// over a large operator the scripts are limits, placed by the font's limit
// constants; on either side of an operator stretched across the line, by its
// stretch stack constants; and otherwise by the underbar and overbar gaps — an
// accent over a short base sits at AccentBaseHeight whatever the base is.
//
// The specification states each distance as a shift of the script's baseline
// from the base's ink. Its first two cases put the script's own ascent (or
// descent) inside the shift; the third, the bars' gap, does not, and so would
// lay an underscript's top over the base's bottom by its ascent with a "gap"
// between the base and the underscript's baseline. The gap is between the ink
// here, as it is in the first two cases and as the WPT tests of these
// constants (underover-parameters-3, -4) measure it: the base's bottom to the
// underscript's top, the overscript's bottom to the base's top. The same
// reading gives the line-ascent and line-descent, whose formulas in the text
// leave out the base's ink, and so would put a script outside the box.
func (l *layouter) mathUnderOver(b *Box, kids []*Box, under, over int, containing style.Unit, s mathStretch) mathContent {
	laid := l.mathStretchInline(kids, containing, s)
	base := laid[0]
	m := l.mathFontFor(b)
	loic, _ := l.mathScriptsItalics(kids[0], mathHOf(base).italic)
	hs := make([]mathH, len(laid))
	for i, k := range laid {
		hs[i] = mathHOf(k)
	}
	width, xs := mathUnderOverX(loic, hs, 0, under, over)
	c := mathContent{width: width, ascent: base.ascent, descent: base.descent, centred: true}
	c.kids = append(c.kids, mathPlaced{laid: base, x: xs[0]})

	op, isOp := l.mathOperator(kids[0])
	largeop := isOp && op.largeop
	stretched := isOp && op.stretchy && op.inlineAxis()
	if under >= 0 {
		u := laid[under]
		var drop, extra style.Unit
		switch {
		case largeop:
			drop = style.Max(m.constant(shape.MathLowerLimitBaselineDropMin),
				m.constant(shape.MathLowerLimitGapMin).Add(u.inkAscent))
		case stretched:
			drop = style.Max(m.constant(shape.MathStretchStackBottomShiftDown),
				m.constant(shape.MathStretchStackGapAboveMin).Add(u.inkAscent))
		default:
			gap := m.constant(shape.MathUnderbarVerticalGap)
			if mathAccentAttr(b, "accentunder") {
				gap = 0
			}
			drop = gap.Add(u.inkAscent)
			extra = m.constant(shape.MathUnderbarExtraDescender)
		}
		shift := base.inkDescent.Add(drop)
		c.ascent = style.Max(c.ascent, u.ascent.Sub(shift))
		c.descent = style.Max(c.descent, u.descent.Add(shift).Add(extra))
		c.kids = append(c.kids, mathPlaced{laid: u, x: xs[under], shift: style.Unit(0).Sub(shift)})
	}
	if over >= 0 {
		o := laid[over]
		var rise, extra style.Unit
		switch {
		case largeop:
			rise = style.Max(m.constant(shape.MathUpperLimitBaselineRiseMin),
				m.constant(shape.MathUpperLimitGapMin).Add(o.inkDescent))
		case stretched:
			rise = style.Max(m.constant(shape.MathStretchStackTopShiftUp),
				m.constant(shape.MathStretchStackGapBelowMin).Add(o.inkDescent))
		default:
			gap := m.constant(shape.MathOverbarVerticalGap)
			if mathAccentAttr(b, "accent") {
				gap = maxZero(m.constant(shape.MathAccentBaseHeight).Sub(base.ascent))
			}
			rise = gap.Add(o.inkDescent)
			extra = m.constant(shape.MathOverbarExtraAscender)
		}
		shift := base.inkAscent.Add(rise)
		c.ascent = style.Max(c.ascent, o.ascent.Add(shift).Add(extra))
		c.descent = style.Max(c.descent, o.descent.Sub(shift))
		c.kids = append(c.kids, mathPlaced{laid: o, x: xs[over], shift: shift})
	}
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	return c
}

// mathStretchInline is §3.4.2.2's algorithm for stretching operators along the
// inline axis: the base first, with the element's own constraint where it is
// an embellished operator being stretched; then every other child but the
// stretchy operators of inline axis, with none; and those last, to the widest
// of the children laid out in the step before — or to nought, where there
// were none.
func (l *layouter) mathStretchInline(kids []*Box, containing style.Unit, s mathStretch) []mathLaid {
	out := make([]mathLaid, len(kids))
	first := 0
	if s.any() {
		out[0] = l.mathBox(kids[0], containing, s)
		first = 1
	}
	var toStretch []int
	target := mathStretch{inline: true}
	for i := first; i < len(kids); i++ {
		if op, ok := l.mathOperator(kids[i]); ok && op.stretchy && op.inlineAxis() {
			toStretch = append(toStretch, i)
			continue
		}
		out[i] = l.mathBox(kids[i], containing, mathStretch{})
		target.size = style.Max(target.size, out[i].width)
	}
	for _, i := range toStretch {
		out[i] = l.mathBox(kids[i], containing, target)
	}
	return out
}
