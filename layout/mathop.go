package layout

import (
	"sort"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// Operators: MathML Core §3.2.4 and appendix B.
//
// An <mo> is spaced, stretched and enlarged by properties it does not state:
// a "+" has thick spaces either side of it and a "(" none, a "∑" is drawn
// larger in display mathematics, a "|" grows to the height of what is beside
// it. Those defaults come from the operator dictionary, looked up by the
// operator's text and its form — whether it is written before its operand,
// after it or between two — and the form is inferred from where the operator
// is in the formula. The attributes on the <mo> override each of them.
//
// The properties are those of an "embellished operator": an <mo> with
// scripts on it, or wrapped in a row with nothing else that is not space, is
// still an operator, spaced as one — "∑" with limits under and over it is
// spaced as the "∑" is — and its properties are its core <mo>'s, looked up in
// the form the embellished whole has where it stands.

// mathForm is an operator's form.
type mathForm uint8

const (
	formInfix mathForm = iota
	formPrefix
	formPostfix
)

// mathOpCategory is a row of figure 26.
type mathOpCategory uint8

const (
	opDefault mathOpCategory = iota
	opForceDefault
	opA
	opB
	opC
	opD
	opE
	opF
	opG
	opH
	opI
	opJ
	opK
	opL
	opM
)

// mathOpDefaults is figure 26: each category's spacing, in ems, and its
// properties.
var mathOpDefaults = map[mathOpCategory]struct {
	lspace, rspace                              float64
	stretchy, symmetric, largeop, movablelimits bool
}{
	opDefault:      {5.0 / 18, 5.0 / 18, false, false, false, false},
	opForceDefault: {5.0 / 18, 5.0 / 18, false, false, false, false},
	opA:            {5.0 / 18, 5.0 / 18, true, false, false, false},
	opB:            {4.0 / 18, 4.0 / 18, false, false, false, false},
	opC:            {3.0 / 18, 3.0 / 18, false, false, false, false},
	opD:            {0, 0, false, false, false, false},
	opE:            {0, 0, false, false, false, false},
	opF:            {0, 0, true, true, false, false},
	opG:            {0, 0, true, true, false, false},
	opH:            {3.0 / 18, 3.0 / 18, false, true, true, false},
	opI:            {0, 0, true, false, false, false},
	opJ:            {3.0 / 18, 3.0 / 18, false, true, true, true},
	opK:            {0, 0, false, false, false, false},
	opL:            {3.0 / 18, 0, false, false, false, false},
	opM:            {0, 3.0 / 18, false, false, false, false},
}

// mathOpEncodings maps figure 26's four-bit encodings to their categories.
var mathOpEncodings = map[uint16]mathOpCategory{
	0x0: opA, 0x4: opB, 0x8: opC, 0x1: opD, 0x2: opE, 0x5: opF,
	0x6: opG, 0x9: opH, 0xA: opI, 0xD: opJ, 0xC: opK,
}

// mathOpCategoryOf is B.1's "algorithm to determine the category of an
// operator (Content, Form)".
func mathOpCategoryOf(content string, form mathForm) mathOpCategory {
	rs := []rune(content)
	utf16 := 0
	for _, r := range rs {
		if r > 0xFFFF {
			utf16 += 2
		} else {
			utf16++
		}
	}
	var c rune
	switch {
	case utf16 == 1:
		c = rs[0]
		if c >= 0x0320 && c <= 0x03FF {
			return opDefault
		}
	case utf16 == 2 && len(rs) == 1:
		// A surrogate pair: the two Arabic operators of category I in postfix
		// form, and nothing else.
		if (rs[0] == 0x1EEF0 || rs[0] == 0x1EEF1) && form == formPostfix {
			return opI
		}
		return opDefault
	case utf16 == 2:
		if rs[1] == 0x0338 || rs[1] == 0x20D2 {
			// A negated operator is looked up as the operator it negates.
			c = rs[0]
			break
		}
		i := -1
		for k, s := range mathOpTwoASCII {
			if s == content {
				i = k
			}
		}
		if i < 0 {
			return opDefault
		}
		c = 0x0320 + rune(i)
	default:
		return opDefault
	}
	if form == formInfix && (c == 0x007C || c == 0x223C) {
		return opForceDefault
	}
	if form == formPrefix && containsRune(mathOpCategoryL[:], c) {
		return opL
	}
	if form == formInfix && containsRune(mathOpCategoryM[:], c) {
		return opM
	}
	var key rune
	switch {
	case c <= 0x03FF:
		key = c
	case c >= 0x2000 && c <= 0x2BFF:
		key = c - 0x1C00
	default:
		return opDefault
	}
	key += []rune{0, 0x1000, 0x2000}[form]
	i := sort.Search(len(mathOpEntries), func(i int) bool {
		return rune(mathOpEntries[i]%0x4000) >= key
	})
	if i < len(mathOpEntries) && rune(mathOpEntries[i]%0x4000) == key {
		return mathOpEncodings[mathOpEntries[i]/0x1000]
	}
	return opDefault
}

func containsRune(set []rune, r rune) bool {
	for _, c := range set {
		if c == r {
			return true
		}
	}
	return false
}

// mathOpInlineAxis reports whether a character's intrinsic stretch axis is
// inline: figure 28.
func mathOpIsInlineAxis(r rune) bool {
	i := sort.Search(len(mathOpInlineAxis), func(i int) bool { return mathOpInlineAxis[i] >= r })
	return i < len(mathOpInlineAxis) && mathOpInlineAxis[i] == r
}

// The element categories §2.1 and §3.2 define the embellished operators and
// the space-like elements by.

// mathName is the element name of a box laid out as MathML — a MathML element
// whose display is math — and empty for every other box. An anonymous row a
// MathML element wraps its children in answers "mrow".
func mathName(b *Box) string {
	if b == nil || b.Inner != InnerMath {
		return ""
	}
	if b.Element == nil {
		return "mrow"
	}
	return b.Element.Name
}

// mathCoreElements are the MathML Core elements (§2.1); any other MathML
// element is an unknown one, laid out as an <mrow>.
var mathCoreElements = setOfNames(
	"annotation", "annotation-xml", "maction", "math", "merror", "mfrac", "mi",
	"mmultiscripts", "mn", "mo", "mover", "mpadded", "mphantom", "mprescripts",
	"mroot", "mrow", "ms", "mspace", "msqrt", "mstyle", "msub", "msubsup",
	"msup", "mtable", "mtd", "mtext", "mtr", "munder", "munderover", "semantics")

func setOfNames(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// isMathGrouping is §2.1's grouping elements: maction, math, merror,
// mphantom, mprescripts, mrow, mstyle, semantics and the unknown MathML
// elements — and the anonymous row.
func isMathGrouping(b *Box) bool {
	switch n := mathName(b); n {
	case "maction", "math", "merror", "mphantom", "mprescripts", "mrow", "mstyle", "semantics":
		return true
	case "":
		return false
	default:
		return !mathCoreElements[n]
	}
}

// isMathScripted is §2.1's scripted elements.
func isMathScripted(b *Box) bool {
	switch mathName(b) {
	case "mmultiscripts", "mover", "msub", "msubsup", "msup", "munder", "munderover":
		return true
	}
	return false
}

// isMathToken reports whether a box is a MathML token element laid out as
// MathML, whose content is text rather than MathML: mi, mn, mo, mtext, ms, and
// annotation and annotation-xml, which §3.7 lays out as mtext.
func isMathToken(b *Box) bool {
	switch mathName(b) {
	case "mi", "mn", "mo", "mtext", "ms", "annotation", "annotation-xml":
		return true
	}
	return false
}

// mathInFlow is a box's in-flow children: every child but an absolutely
// positioned one, since a float inside MathML does not float (§2.2.2).
func mathInFlow(b *Box) []*Box {
	out := make([]*Box, 0, len(b.Children))
	for _, c := range b.Children {
		if c.Position.outOfFlow() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// mathClass is what the embellished-operator and space-like questions answer
// for one box, worked out once.
type mathClass struct {
	spaceLike   bool
	embellished bool
	core        *Box // the core <mo>, when embellished
}

// mathClassOf is §3.2.4.1's embellished operator and §3.2.5.1's space-like
// element, for a box, memoized: every row asks it of each of its children,
// and each answer is built from the children's.
func (l *layouter) mathClassOf(b *Box) mathClass {
	if got, ok := l.mathClasses[b]; ok {
		return got
	}
	var c mathClass
	switch name := mathName(b); {
	case name == "mo":
		c = mathClass{embellished: true, core: b}
	case name == "mtext" || name == "mspace":
		c.spaceLike = true
	case isMathScripted(b) || name == "mfrac":
		if kids := mathInFlow(b); len(kids) > 0 {
			if k := l.mathClassOf(kids[0]); k.embellished {
				c = mathClass{embellished: true, core: k.core}
			}
		}
	case isMathGrouping(b) || name == "mpadded":
		c.spaceLike = true
		var core *Box
		ops := 0
		for _, k := range mathInFlow(b) {
			kc := l.mathClassOf(k)
			if !kc.spaceLike {
				c.spaceLike = false
			}
			switch {
			case kc.embellished:
				ops++
				core = kc.core
			case !kc.spaceLike:
				ops = 2 // something that is neither: not embellished
			}
		}
		if ops == 1 {
			c.embellished, c.core = true, core
		}
	}
	if l.mathClasses == nil {
		l.mathClasses = map[*Box]mathClass{}
	}
	l.mathClasses[b] = c
	return c
}

// mathOp is an embellished operator's properties, §3.2.4.2, resolved to
// lengths for the layout.
type mathOp struct {
	core *Box
	form mathForm
	// text is the core operator's text, and char its one character where it
	// is one.
	text   string
	char   rune
	single bool

	lspace, rspace                              style.Unit
	stretchy, symmetric, largeop, movablelimits bool
}

// inlineAxis is the stretch axis of the embellished operator: inline where the
// core operator is a single character of inline intrinsic axis, block
// otherwise.
func (o *mathOp) inlineAxis() bool {
	return o.single && mathOpIsInlineAxis(o.char)
}

// mathOperator is the properties of the embellished operator e, where it is
// one.
//
// They are the same for every embellished operator with the same core: the
// form they are looked up in is the form of the outermost of them, which is
// the one that stands in a row. "∑" with limits is looked up as the prefix
// operator the <munderover> is in the row, and not as the first child of the
// <munderover> it is inside.
func (l *layouter) mathOperator(e *Box) (*mathOp, bool) {
	c := l.mathClassOf(e)
	if !c.embellished {
		return nil, false
	}
	core := c.core
	if got, ok := l.mathOps[core]; ok {
		return got, true
	}
	if l.mathOps == nil {
		l.mathOps = map[*Box]*mathOp{}
	}
	outer := core
	for p := outer.Parent; p != nil; p = p.Parent {
		if pc := l.mathClassOf(p); !pc.embellished || pc.core != core {
			break
		}
		outer = p
	}
	op := &mathOp{core: core, text: mathTokenText(core)}
	if rs := []rune(op.text); len(rs) == 1 {
		op.char, op.single = rs[0], true
	}

	// The form: the attribute on the core operator, or where the embellished
	// operator stands.
	explicit := false
	if v, ok := core.Element.Attr("form"); ok {
		switch {
		case ascii.EqualFold(v, "infix"):
			op.form, explicit = formInfix, true
		case ascii.EqualFold(v, "prefix"):
			op.form, explicit = formPrefix, true
		case ascii.EqualFold(v, "postfix"):
			op.form, explicit = formPostfix, true
		}
	}
	if !explicit {
		op.form = l.mathFormOf(outer)
	}

	// The category, and the fallback §3.2.4.2 takes where the form was
	// inferred and the dictionary has nothing for it.
	cat := opDefault
	if mathOnlyText(core) && op.text != "" {
		cat = mathOpCategoryOf(op.text, op.form)
		if cat == opDefault && !explicit {
			for _, f := range []mathForm{formInfix, formPostfix, formPrefix} {
				if cat = mathOpCategoryOf(op.text, f); cat != opDefault {
					break
				}
			}
		}
	}
	d := mathOpDefaults[cat]
	op.stretchy, op.symmetric, op.largeop, op.movablelimits = d.stretchy, d.symmetric, d.largeop, d.movablelimits
	em := func(f float64) style.Unit { return core.FontSize.Mul(f) }
	op.lspace, op.rspace = em(d.lspace), em(d.rspace)

	// The attributes, where present and valid, on the core operator.
	boolAttr := func(name string, into *bool) {
		if v, ok := core.Element.Attr(name); ok {
			switch {
			case ascii.EqualFold(v, "true"):
				*into = true
			case ascii.EqualFold(v, "false"):
				*into = false
			}
		}
	}
	boolAttr("stretchy", &op.stretchy)
	boolAttr("symmetric", &op.symmetric)
	boolAttr("largeop", &op.largeop)
	boolAttr("movablelimits", &op.movablelimits)
	// lspace and rspace: a <length-percentage>, a percentage of the value the
	// dictionary gave.
	space := func(name string, into *style.Unit) {
		if length, ok := l.mathLengthAttr(core, name); ok {
			*into = mathResolve(length, *into)
		}
	}
	space("lspace", &op.lspace)
	space("rspace", &op.rspace)
	l.mathOps[core] = op
	return op, true
}

// mathOnlyText reports whether a token's content is text and nothing else.
func mathOnlyText(b *Box) bool {
	for _, c := range b.Children {
		if !c.IsText() {
			return false
		}
	}
	return true
}

// mathResolve resolves a <length-percentage> against what a percentage is of.
func mathResolve(length style.Length, basis style.Unit) style.Unit {
	v, ok := length.Resolve(basis, true)
	if !ok {
		return basis
	}
	return v
}

// mathLengthAttr reads a MathML attribute whose value is a <length-percentage>,
// in the element's own font, and false where it is absent or not one.
func (l *layouter) mathLengthAttr(b *Box, name string) (style.Length, bool) {
	if b.Element == nil {
		return style.Length{}, false
	}
	v, ok := b.Element.Attr(name)
	if !ok {
		return style.Length{}, false
	}
	vals, errs := css.ParseComponentValues(v)
	if len(errs) != 0 {
		return style.Length{}, false
	}
	var kept []css.ComponentValue
	for _, c := range vals {
		if c.IsToken() && c.Token.Kind == css.Whitespace {
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) != 1 {
		return style.Length{}, false
	}
	switch k := kept[0]; {
	case k.IsFunction():
	case k.IsToken() && (k.Token.Kind == css.Dimension || k.Token.Kind == css.Percentage ||
		k.Token.Kind == css.Number && k.Token.Number == 0):
	default:
		return style.Length{}, false
	}
	length, ok := l.lengthOfValues(b, kept)
	if !ok || length.Kind == style.LengthAuto {
		return style.Length{}, false
	}
	return length, true
}

// mathFormOf is §3.2.4.2's algorithm for the form of an embellished operator
// that states none: prefix as the first of several in a row, postfix as the
// last, postfix as a script, and infix otherwise. The rows are the grouping
// elements, <mpadded> and <msqrt>, and "several" and "first" and "last" are
// counted without the space-like children.
func (l *layouter) mathFormOf(e *Box) mathForm {
	p := e.Parent
	if p == nil {
		return formInfix
	}
	if isMathGrouping(p) || mathName(p) == "mpadded" || mathName(p) == "msqrt" {
		ends := l.mathRowEnds(p)
		if ends.several {
			switch e {
			case ends.first:
				return formPrefix
			case ends.last:
				return formPostfix
			}
		}
		return formInfix
	}
	if isMathScripted(p) {
		for _, k := range p.Children {
			if !k.Position.outOfFlow() {
				if k != e {
					return formPostfix
				}
				break
			}
		}
	}
	return formInfix
}

// mathEnds is a row's first and last in-flow children that are not
// space-like, and whether there are several such.
type mathEnds struct {
	first, last *Box
	several     bool
}

// mathRowEnds is a row's ends, worked out once for the row: every operator in
// it asks, and a walk of the row for each would be the square of the row.
func (l *layouter) mathRowEnds(p *Box) mathEnds {
	if got, ok := l.mathEnds[p]; ok {
		return got
	}
	var out mathEnds
	n := 0
	for _, k := range mathInFlow(p) {
		if l.mathClassOf(k).spaceLike {
			continue
		}
		if n == 0 {
			out.first = k
		}
		out.last = k
		n++
	}
	out.several = n > 1
	if l.mathEnds == nil {
		l.mathEnds = map[*Box]mathEnds{}
	}
	l.mathEnds[p] = out
	return out
}

// mathTokenText is a token element's text: its text children's, joined, with
// the white space at either end that a line would drop dropped.
func mathTokenText(b *Box) string {
	var sb strings.Builder
	for _, c := range b.Children {
		if c.IsText() {
			sb.WriteString(c.Text)
		}
	}
	return strings.Trim(sb.String(), " \t\n\f\r")
}
