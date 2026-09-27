package style

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
)

// What MathML Core adds to the cascade (§2.1.3–§2.1.6 and §4).
//
// Three inherited properties: math-style, whether a formula is set to take
// the room of display mathematics ("normal") or to keep its height down
// ("compact"); math-shift, which of two heights a superscript is raised to;
// and math-depth, how many levels of script a part of a formula is inside,
// which is what "font-size: math" scales the text by. And the attributes a
// MathML element carries that mean a declaration: dir, mathcolor,
// mathbackground, mathsize, displaystyle, scriptlevel, and a few that only one
// element has.
//
// math-depth's computed value is an integer, and it is computed here because
// two of its three forms are relative to what is inherited — "add(1)" is one
// deeper than the parent, and "auto-add" is one deeper only where the parent
// is compact — and "font-size: math" on the same element needs the answer:
// the scale it applies is decided by how far the depth moved.

var (
	mathDepthID = registry.ids["math-depth"]
)

// maxMathDepth bounds math-depth's computed value either way.
//
// The value is the document's to state, and add() accumulates it: a thousand
// nested "add(1000000000)"s would overflow the integer. The bound is far past
// any depth that means anything — each level scales the text by 0.71, so a
// depth of a hundred is a font size of about 10⁻¹⁵ of the root's, and well
// before the bound the size a scale gives either underflows to nothing or
// overflows what a length can hold, which "font-size: math" reports by
// leaving the size unresolved (see mathFontSize).
const maxMathDepth = 1 << 20

// resolveMathDepth computes math-depth: MathML Core §4.5.
func (s *Styler) resolveMathDepth(b *styleBuilder, parent ComputedStyle) {
	v := ascii.TrimCSSSpace(b.cs.Get("math-depth"))
	inherited, compact := 0, false
	if !parent.IsZero() {
		inherited, _ = strconv.Atoi(parent.Get("math-depth"))
		compact = ascii.EqualFold(ascii.TrimCSSSpace(parent.Get("math-style")), "compact")
	}
	depth, ok := mathDepthOf(v, inherited, compact)
	if !ok {
		// Every value the grammar admits reads, and a calc() it admits as
		// valid CSS this engine does not evaluate is reported and dropped
		// before this, leaving the inherited depth. Were a value not to read,
		// the inherited depth is what a declaration saying nothing computes to.
		depth = inherited
	}
	depth = max(-maxMathDepth, min(maxMathDepth, depth))
	if text := strconv.Itoa(depth); text != v {
		b.set(mathDepthID, s.interner().value(text))
	}
}

// mathDepthOf is math-depth's computed value for a specified one, given the
// inherited depth and whether the inherited math-style is compact.
func mathDepthOf(v string, inherited int, compact bool) (int, bool) {
	switch {
	case ascii.EqualFold(v, "auto-add"):
		if compact {
			return inherited + 1, true
		}
		return inherited, true
	case len(v) > 4 && ascii.EqualFold(v[:4], "add("):
		n, ok := integerLiteral(strings.TrimSuffix(v[4:], ")"))
		if !ok {
			return 0, false
		}
		return inherited + n, true
	}
	return integerLiteral(v)
}

// integerLiteral reads a CSS <integer> as the grammar admitted it, bounded:
// a number of more digits than an int holds is as deep as the bound anyway.
func integerLiteral(v string) (int, bool) {
	v = ascii.TrimCSSSpace(v)
	n, err := strconv.ParseInt(strings.TrimPrefix(v, "+"), 10, 64)
	if err != nil {
		if !errors.Is(err, strconv.ErrRange) {
			return 0, false
		}
		// Out of range: the sign decides which end of the bound.
		if strings.HasPrefix(v, "-") {
			return -maxMathDepth, true
		}
		return maxMathDepth, true
	}
	return int(max(-maxMathDepth, min(maxMathDepth, n))), true
}

// MathMetrics is a Metrics that can also answer the one font question
// "font-size: math" asks: what the first available font of a computed style
// says a script and a script's script are scaled by — its MATH table's
// scriptPercentScaleDown and scriptScriptPercentScaleDown — and whether it has
// a MATH table at all. The scales are fractions, MathML Core §5.1's: the
// percentages over a hundred, or 0.71 and 0.5041 where the font states nought
// or has no constants. hasMath false is a font with no MATH table, whose
// scaling §4.5 does by 0.71 a level without reading either.
//
// It is an interface of its own rather than a method of Metrics so that a
// caller's existing Metrics keeps working: one that does not implement it
// scales by 0.71 a level, as for a font with no MATH table.
type MathMetrics interface {
	MathScaleDowns(cs ComputedStyle, size Unit) (script, scriptScript float64, hasMath bool)
}

// mathFontScale is §4.5's scale factor for "font-size: math": from the
// inherited math-depth a to the computed one b, with the inherited first
// available font's two scale-downs where it has a MATH table.
func mathFontScale(a, b int, script, scriptScript float64, hasMath bool) float64 {
	if a == b {
		return 1
	}
	invert := false
	if b < a {
		a, b, invert = b, a, true
	}
	e := b - a
	s := 1.0
	if hasMath {
		switch {
		case a <= 0 && b >= 2:
			s *= scriptScript
			e -= 2
		case a == 1:
			s *= scriptScript / script
			e--
		case b == 1:
			s *= script
			e--
		}
	}
	s *= math.Pow(0.71, float64(e))
	if invert {
		return 1 / s
	}
	return s
}

// mathFontSize is "font-size: math" for one element: the parent's size times
// the scale for how far math-depth moved. The second result is false where
// the answer is no length at all — a scale that underflows to nothing or a
// size past what a length can hold — which leaves the declaration for layout
// to report against the element.
func mathFontSize(parentSize Unit, from, to int, m Metrics, fontStyle ComputedStyle) (Unit, bool) {
	script, scriptScript, hasMath := 0.71, 0.5041, false
	if mm, ok := m.(MathMetrics); ok && from != to {
		script, scriptScript, hasMath = mm.MathScaleDowns(fontStyle, parentSize)
	}
	s := mathFontScale(from, to, script, scriptScript, hasMath)
	if s <= 0 || math.IsInf(s, 0) || math.IsNaN(s) {
		return parentSize, false
	}
	return FromPx(parentSize.Px() * s)
}

// isMathFontSize reports whether a font-size is the keyword math.
func isMathFontSize(v string) bool {
	return ascii.EqualFold(ascii.TrimCSSSpace(v), "math")
}

// mathMLHints are the attributes of a MathML element that mean a declaration:
// the global ones of MathML Core §2.1.4–§2.1.6 and the three of one element
// each (§3.2.2 mathvariant, §3.2.5 mspace's dimensions, §3.3.6 mpadded's
// width). A value that is not what the attribute takes — a mathcolor that is
// not a <color>, a scriptlevel that is not an integer — sets nothing, which
// is what each section says an absent or invalid attribute does.
func mathMLHints(n *html.Node) map[string][]css.ComponentValue {
	var out map[string][]css.ComponentValue
	set := func(property string, vals []css.ComponentValue) {
		if out == nil {
			out = map[string][]css.ComponentValue{}
		}
		out[property] = vals
	}
	// A value read as its property reads it: tokenized, and admitted only if
	// the property's own grammar takes it whole.
	as := func(property, attr string) ([]css.ComponentValue, bool) {
		vals, errs := css.ParseComponentValues(attr)
		if len(errs) != 0 {
			return nil, false
		}
		vals = trimValues(vals)
		if len(vals) == 0 || !judgeValue(property, vals).ok {
			return nil, false
		}
		return vals, true
	}
	ident := func(word string) []css.ComponentValue {
		return []css.ComponentValue{{Token: css.Token{Kind: css.Ident, Value: word}}}
	}
	for _, a := range n.Attrs {
		v := a.Value
		switch a.Name {
		case "dir":
			// §2.1.4: an ASCII case-insensitive ltr or rtl.
			switch {
			case ascii.EqualFold(v, "ltr"):
				set("direction", ident("ltr"))
			case ascii.EqualFold(v, "rtl"):
				set("direction", ident("rtl"))
			}
		case "mathcolor":
			if vals, ok := as("color", v); ok {
				set("color", vals)
			}
		case "mathbackground":
			if vals, ok := as("background-color", v); ok {
				set("background-color", vals)
			}
		case "mathsize":
			// A <length-percentage>, which font-size's grammar is wider than:
			// it takes the size keywords too, and "mathsize=large" is not a
			// length.
			if vals, ok := as("font-size", v); ok && isLengthPercentage(vals) {
				set("font-size", vals)
			}
		case "displaystyle":
			switch {
			case ascii.EqualFold(v, "true"):
				set("math-style", ident("normal"))
			case ascii.EqualFold(v, "false"):
				set("math-style", ident("compact"))
			}
		case "scriptlevel":
			if vals, ok := scriptLevelHint(v); ok {
				set("math-depth", vals)
			}
		case "mathvariant":
			// §3.2.2: on an <mi>, "normal" cancels the automatic italic.
			// Every other value, and the attribute on any other element, has
			// no effect in MathML Core.
			if n.Name == "mi" && ascii.EqualFold(v, "normal") {
				set("text-transform", ident("none"))
			}
		}
	}
	switch n.Name {
	case "mspace":
		mspaceHints(n, as, set)
	case "mpadded":
		// §3.3.6.1: the width attribute, "present, valid and not a
		// percentage", sets the width property.
		if v, ok := n.Attr("width"); ok {
			if vals, ok := as("width", v); ok && isLength(vals) {
				set("width", vals)
			}
		}
	}
	return out
}

// mspaceHints are §3.2.5's: width sets the width property, and height and
// depth together set the height property to their sum — each only where it
// is present, valid and not a percentage. (The height alone is also the
// element's line-ascent, which layout reads from the attribute.)
func mspaceHints(n *html.Node, as func(property, attr string) ([]css.ComponentValue, bool),
	set func(string, []css.ComponentValue)) {

	length := func(name string) ([]css.ComponentValue, bool) {
		v, ok := n.Attr(name)
		if !ok {
			return nil, false
		}
		vals, errs := css.ParseComponentValues(v)
		vals = trimValues(vals)
		if len(errs) != 0 || !single(num(lengthPctSlot))(items(vals)).ok || !isLength(vals) {
			return nil, false
		}
		return vals, true
	}
	if vals, ok := length("width"); ok && judgeValue("width", vals).ok {
		set("width", vals)
	}
	h, hasHeight := length("height")
	d, hasDepth := length("depth")
	switch {
	case hasHeight && hasDepth:
		sum := "calc(" + serialize(h) + " + " + serialize(d) + ")"
		if vals, ok := as("height", sum); ok {
			set("height", vals)
		}
	case hasHeight:
		if judgeValue("height", h).ok {
			set("height", h)
		}
	case hasDepth:
		if judgeValue("height", d).ok {
			set("height", d)
		}
	}
}

// scriptLevelHint is §2.1.6's mapping: "+U" is add(U), "-U" add(-U), and "U"
// the integer U, where U is an unsigned integer — digits with no sign of
// their own.
func scriptLevelHint(v string) ([]css.ComponentValue, bool) {
	v = ascii.TrimSpace(v)
	sign := ""
	if strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		sign, v = v[:1], v[1:]
	}
	if v == "" {
		return nil, false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return nil, false
		}
	}
	u, _ := integerLiteral(v)
	num := func(n int) css.ComponentValue {
		return css.ComponentValue{Token: css.Token{Kind: css.Number, Number: float64(n),
			IsInteger: true, Repr: strconv.Itoa(n)}}
	}
	switch sign {
	case "+":
		return []css.ComponentValue{{Token: css.Token{Kind: css.Function, Value: "add"},
			Values: []css.ComponentValue{num(u)}}}, true
	case "-":
		return []css.ComponentValue{{Token: css.Token{Kind: css.Function, Value: "add"},
			Values: []css.ComponentValue{num(-u)}}}, true
	}
	return []css.ComponentValue{num(u)}, true
}

// isLength reports whether a value is one length: a dimension, or a math
// function, and not a percentage.
func isLength(vals []css.ComponentValue) bool {
	if len(vals) != 1 {
		return false
	}
	v := vals[0]
	if v.IsFunction() {
		return !mentionsPercentage(v.Values)
	}
	return v.IsToken() && (v.Token.Kind == css.Dimension ||
		v.Token.Kind == css.Number && v.Token.Number == 0)
}

// isLengthPercentage is isLength with a percentage allowed.
func isLengthPercentage(vals []css.ComponentValue) bool {
	if len(vals) == 1 && vals[0].IsToken() && vals[0].Token.Kind == css.Percentage {
		return true
	}
	if len(vals) == 1 && vals[0].IsFunction() {
		return true
	}
	return isLength(vals)
}

func mentionsPercentage(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Percentage {
			return true
		}
		if len(v.Values) > 0 && mentionsPercentage(v.Values) {
			return true
		}
	}
	return false
}

// trimValues is a value without the white space around it.
func trimValues(vals []css.ComponentValue) []css.ComponentValue {
	for len(vals) > 0 && vals[0].IsToken() && vals[0].Token.Kind == css.Whitespace {
		vals = vals[1:]
	}
	for len(vals) > 0 && vals[len(vals)-1].IsToken() && vals[len(vals)-1].Token.Kind == css.Whitespace {
		vals = vals[:len(vals)-1]
	}
	return vals
}
