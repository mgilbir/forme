package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
)

// The value grammar: for every registered property, what its value may be.
//
// # Why there has to be one
//
// CSS 2.1 §4.2 says a declaration whose value the property does not take is
// ignored, and what stands is whatever the cascade would have produced without
// it. "div { width: 100px } div { width: foo }" is a hundred pixels wide. This
// engine had no single place that said what a property's value may be — that
// was decided per property, in the stage that read it — so six properties had
// their value checked early enough to be dropped and every other one let the
// invalid declaration win: "width: foo" beat "width: 100px", layout could make
// nothing of it, and the box was auto with nothing said (audit C57).
//
// The same missing table answered two other questions wrongly. @supports asked
// about the property and not the value, so "(position: bogus)" was true. And a
// value that is correct CSS the engine does not evaluate — "color: oklch(…)",
// "width: min(10px, 50%)", "border: 2px solid lab(…)" — could not be told from
// a mistake, so it was dropped and reported as the author's error, which the
// reftest ratchet reads as a page with nothing missing from it (C28, C59).
//
// So this is one question with one answer, asked by the cascade before a
// declaration is kept, by @supports, and of every longhand a shorthand expands
// into. Its answer is one of three:
//
//   - valid: the declaration is kept;
//   - invalid: §4.2 drops it, and the finding is the author's to act on;
//   - valid, and naming something this engine does not evaluate — a colour
//     function past sRGB, a math function other than calc(), a unit nothing
//     here resolves: the declaration is dropped as well, so the one before it
//     stands — which is the fallback an author writes such a declaration after —
//     and the finding says the engine is missing something.
//
// # What it is, and what it is not
//
// It is each property's value definition from its specification, with the
// CSS-wide keywords taken out beforehand (the cascade handles those) and var()
// taken out too (a declaration using one is unset, not judged). It says whether
// a value is CSS, not whether this engine draws it: "text-justify:
// inter-character" is valid here and reported by the stage that reads it, which
// knows whether the page it laid out differs. A keyword this engine reads as
// another is that stage's to name; a value that is not CSS at all is this one's.
//
// The keyword lists are the current specifications', including the legacy
// spellings every browser still accepts — "writing-mode: tb-rl",
// "word-break: break-word" — because refusing what a browser accepts would drop
// declarations a browser applies. A vendor-prefixed keyword is not accepted:
// it is another engine's spelling, and the author's unprefixed declaration
// beside it is the one CSS describes.

// verdict is the answer about one value or one part of one.
type verdict struct {
	ok bool
	// unsupported names what, in a valid value, this engine does not evaluate:
	// "oklch()", "min()", "the unit lh". Empty when there is nothing.
	unsupported string
}

var (
	valid   = verdict{ok: true}
	invalid = verdict{}
)

func unevaluated(what string) verdict { return verdict{ok: true, unsupported: what} }

// and is the verdict for two parts of one value: invalid if either is, and the
// first thing either does not evaluate.
func (v verdict) and(o verdict) verdict {
	if !v.ok || !o.ok {
		return invalid
	}
	if v.unsupported == "" {
		v.unsupported = o.unsupported
	}
	return v
}

// term judges one component value.
type term func(v css.ComponentValue) verdict

// grammar judges a whole value, given without its whitespace.
type grammar func(items []css.ComponentValue) verdict

// valueGrammars is every registered property's grammar. TestEveryPropertyHasA
// Grammar is what keeps it complete: a property registered without one is a
// declaration whose value nothing checks, which is the gap this closes.
var valueGrammars = map[string]grammar{}

// judgeValue is the grammar's answer for a registered property, or for a
// logical longhand, which takes its physical counterpart's value. It is the
// one place the question is asked; an unknown name is not valid.
func judgeValue(name string, vals []css.ComponentValue) verdict {
	if proxy, ok := logicalProxy(name); ok {
		name = proxy
	}
	g, ok := valueGrammars[name]
	if !ok {
		return invalid
	}
	return g(items(vals))
}

// items is a value without its whitespace. Whitespace separates the parts of a
// value and is significant nowhere in these grammars except inside a function,
// whose arguments are not flattened.
func items(vals []css.ComponentValue) []css.ComponentValue {
	out := make([]css.ComponentValue, 0, len(vals))
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		out = append(out, v)
	}
	return out
}

// Terms.

// kw is one keyword from a set.
func kw(words ...string) term {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return func(v css.ComponentValue) verdict {
		if name, ok := identOf(v); ok && set[name] {
			return valid
		}
		return invalid
	}
}

// identOf is a component's keyword, lower-cased, if it is one.
func identOf(v css.ComponentValue) (string, bool) {
	if !v.IsToken() || v.Token.Kind != css.Ident {
		return "", false
	}
	return strings.ToLower(v.Token.Value), true
}

// either is the first of several terms that takes a component.
func either(terms ...term) term {
	return func(v css.ComponentValue) verdict {
		for _, t := range terms {
			if got := t(v); got.ok {
				return got
			}
		}
		return invalid
	}
}

// Numeric terms.

// numeric says which kinds of number a slot takes.
type numeric struct {
	length, percent, number, angle bool
	// integer asks for an <integer>, which a <number> written with a fraction
	// or an exponent is not.
	integer bool
	// min is the lowest value a literal may have, when hasMin says there is
	// one. A math function is not held to it: its range is enforced by clamping
	// at computed-value time, never by making the declaration invalid.
	min    float64
	hasMin bool
	max    float64
	hasMax bool
}

func (n numeric) inRange(x float64) bool {
	return (!n.hasMin || x >= n.min) && (!n.hasMax || x <= n.max)
}

// nonNeg is the same slot with [0,∞].
func (n numeric) nonNeg() numeric { n.min, n.hasMin = 0, true; return n }

var (
	lengthSlot    = numeric{length: true}
	lengthPctSlot = numeric{length: true, percent: true}
	numberSlot    = numeric{number: true}
	integerSlot   = numeric{number: true, integer: true}
	angleSlot     = numeric{angle: true}
)

// num is a term for a numeric slot.
func num(n numeric) term {
	return func(v css.ComponentValue) verdict {
		if v.IsFunction() {
			return mathTerm(v, n)
		}
		if !v.IsToken() {
			return invalid
		}
		t := v.Token
		switch t.Kind {
		case css.Number:
			if n.number {
				if n.integer && !t.IsInteger {
					return invalid
				}
				if !n.inRange(t.Number) {
					return invalid
				}
				return valid
			}
			// A bare zero is a length, and it is the only number that is.
			if (n.length || n.angle) && t.Number == 0 {
				return valid
			}
			return invalid
		case css.Percentage:
			if n.percent && n.inRange(t.Number) {
				return valid
			}
			return invalid
		case css.Dimension:
			switch unitKind(t.Unit) {
			case kindLength:
				if !n.length || !n.inRange(t.Number) {
					return invalid
				}
				if unresolvedUnits[strings.ToLower(t.Unit)] {
					return unevaluated("the unit " + strings.ToLower(t.Unit))
				}
				return valid
			case kindAngle:
				if !n.angle || !n.inRange(t.Number) {
					return invalid
				}
				return valid
			}
		}
		return invalid
	}
}

// Math functions.

// mathKind is what a math expression computes to.
type mathKind uint8

const (
	kindNone mathKind = iota
	kindNumber
	kindLength
	kindPercent
	// kindLengthPercent is a sum of a length and a percentage, which only a
	// slot that takes both can hold.
	kindLengthPercent
	kindAngle
	kindTime
	kindFrequency
	kindResolution
	kindFlex
)

// unitKind is what a dimension's unit measures, kindNone for a unit CSS does
// not define.
func unitKind(unit string) mathKind {
	u := strings.ToLower(unit)
	if unresolvedUnits[u] {
		return kindLength
	}
	switch u {
	case "px", "pt", "pc", "in", "cm", "mm", "q", "em", "rem", "ch", "ic", "ex",
		"vw", "vh", "vmin", "vmax", "svw", "svh", "svmin", "svmax",
		"lvw", "lvh", "lvmin", "lvmax", "dvw", "dvh", "dvmin", "dvmax":
		return kindLength
	case "deg", "grad", "rad", "turn":
		return kindAngle
	case "s", "ms":
		return kindTime
	case "hz", "khz":
		return kindFrequency
	case "dpi", "dpcm", "dppx", "x":
		return kindResolution
	case "fr":
		return kindFlex
	}
	return kindNone
}

// mathFunctions are CSS Values 4's math functions. Only calc() is evaluated
// here — see calc.go — and the rest are valid CSS this engine does not compute.
var mathFunctions = map[string]bool{
	"calc": true, "min": true, "max": true, "clamp": true, "round": true,
	"mod": true, "rem": true, "abs": true, "sign": true, "sin": true,
	"cos": true, "tan": true, "asin": true, "acos": true, "atan": true,
	"atan2": true, "pow": true, "sqrt": true, "hypot": true, "log": true,
	"exp": true,
}

// otherFunctions are functions CSS defines that may stand in for a value of
// any type and that this engine does not evaluate anywhere: substitution and
// conditional functions. var() is not here; the cascade takes it out before
// any value is judged.
var otherFunctions = map[string]bool{
	"env": true, "attr": true, "if": true, "inherit": true,
	"random": true, "progress": true, "calc-size": true,
	"anchor": true, "anchor-size": true, "sibling-index": true,
	"sibling-count": true,
}

// mathTerm judges a function in a numeric slot.
func mathTerm(v css.ComponentValue, n numeric) verdict {
	name := strings.ToLower(v.Token.Value)
	if otherFunctions[name] {
		return unevaluated(name + "()")
	}
	if !mathFunctions[name] {
		return invalid
	}
	kind, evaluated, ok := mathOf(v)
	if !ok || !n.takes(kind) {
		return invalid
	}
	// calc() is evaluated where a length is read, and nowhere else: a number,
	// an integer or an angle given by one is valid CSS this engine does not
	// compute.
	if evaluated && (n.length || n.percent) && kind != kindNumber {
		return valid
	}
	return unevaluated(name + "()")
}

// takes reports whether a slot holds what a math expression computes to.
func (n numeric) takes(k mathKind) bool {
	switch k {
	case kindNumber:
		return n.number
	case kindLength:
		return n.length
	case kindPercent:
		return n.percent
	case kindLengthPercent:
		return n.length && n.percent
	case kindAngle:
		return n.angle
	}
	return false
}

// mathOf type-checks a math function by CSS Values 4 §10.9, and reports whether
// calc.go evaluates it — only calc() and parentheses, over numbers,
// percentages and the lengths pxPerUnit resolves.
//
// The arithmetic is Values 3's, as calc.go's is: a product needs a number on
// one side, and a quotient a number on the right. An expression that does not
// type-check is not a value, and the declaration holding it is invalid.
func mathOf(fn css.ComponentValue) (kind mathKind, evaluated, ok bool) {
	name := strings.ToLower(fn.Token.Value)
	args := splitOnComma(fn.Values)
	sum := func(vals []css.ComponentValue) (mathKind, bool, bool) { return mathSum(vals) }
	all := func(want int) ([]mathKind, bool) {
		if want > 0 && len(args) != want {
			return nil, false
		}
		out := make([]mathKind, len(args))
		for i, a := range args {
			k, _, ok := sum(a)
			if !ok {
				return nil, false
			}
			out[i] = k
		}
		return out, true
	}
	same := func(kinds []mathKind) (mathKind, bool) {
		k := kinds[0]
		for _, o := range kinds[1:] {
			var ok bool
			if k, ok = addKinds(k, o); !ok {
				return kindNone, false
			}
		}
		return k, true
	}

	switch name {
	case "calc":
		if len(args) != 1 {
			return kindNone, false, false
		}
		return sum(args[0])
	case "min", "max", "hypot":
		kinds, ok := all(0)
		if !ok {
			return kindNone, false, false
		}
		k, ok := same(kinds)
		return k, false, ok
	case "clamp":
		if len(args) != 3 {
			return kindNone, false, false
		}
		var kinds []mathKind
		for i, a := range args {
			if (i == 0 || i == 2) && isNoneArg(a) {
				continue
			}
			k, _, ok := sum(a)
			if !ok {
				return kindNone, false, false
			}
			kinds = append(kinds, k)
		}
		k, ok := same(kinds)
		return k, false, ok
	case "round":
		if len(args) > 0 {
			if s, isIdent := singleIdent(args[0]); isIdent {
				switch s {
				case "nearest", "up", "down", "to-zero":
					args = args[1:]
				}
			}
		}
		if len(args) != 1 && len(args) != 2 {
			return kindNone, false, false
		}
		kinds, ok := all(0)
		if !ok {
			return kindNone, false, false
		}
		k, ok := same(kinds)
		return k, false, ok
	case "mod", "rem", "atan2":
		kinds, ok := all(2)
		if !ok {
			return kindNone, false, false
		}
		k, ok := same(kinds)
		if name == "atan2" {
			k = kindAngle
		}
		return k, false, ok
	case "abs":
		kinds, ok := all(1)
		if !ok {
			return kindNone, false, false
		}
		return kinds[0], false, true
	case "sign":
		_, ok := all(1)
		return kindNumber, false, ok
	case "sin", "cos", "tan":
		kinds, ok := all(1)
		return kindNumber, false, ok && (kinds[0] == kindNumber || kinds[0] == kindAngle)
	case "asin", "acos", "atan":
		kinds, ok := all(1)
		return kindAngle, false, ok && kinds[0] == kindNumber
	case "pow":
		kinds, ok := all(2)
		return kindNumber, false, ok && kinds[0] == kindNumber && kinds[1] == kindNumber
	case "sqrt", "exp":
		kinds, ok := all(1)
		return kindNumber, false, ok && kinds[0] == kindNumber
	case "log":
		if len(args) != 1 && len(args) != 2 {
			return kindNone, false, false
		}
		kinds, ok := all(0)
		if !ok {
			return kindNone, false, false
		}
		for _, k := range kinds {
			if k != kindNumber {
				return kindNone, false, false
			}
		}
		return kindNumber, false, true
	}
	return kindNone, false, false
}

func isNoneArg(vals []css.ComponentValue) bool {
	s, ok := singleIdent(vals)
	return ok && s == "none"
}

// addKinds is the type of a sum.
func addKinds(a, b mathKind) (mathKind, bool) {
	switch {
	case a == b:
		return a, true
	case (a == kindLength || a == kindPercent || a == kindLengthPercent) &&
		(b == kindLength || b == kindPercent || b == kindLengthPercent):
		return kindLengthPercent, true
	}
	return kindNone, false
}

// mathSum reads "a + b - c", where each operand is a product. CSS Values
// requires white space around "+" and "-", which is what tells them from the
// sign of a number; tokens arrive with that already settled.
func mathSum(vals []css.ComponentValue) (mathKind, bool, bool) {
	terms, _, ok := splitOperators(vals, "+-")
	if !ok {
		return kindNone, false, false
	}
	var kind mathKind
	evaluated := true
	for i, t := range terms {
		k, ev, ok := mathProduct(t)
		if !ok {
			return kindNone, false, false
		}
		evaluated = evaluated && ev
		if i == 0 {
			kind = k
			continue
		}
		if kind, ok = addKinds(kind, k); !ok {
			return kindNone, false, false
		}
	}
	return kind, evaluated, true
}

// mathProduct reads "a * b / c".
func mathProduct(vals []css.ComponentValue) (mathKind, bool, bool) {
	terms, ops, ok := splitOperators(vals, "*/")
	if !ok {
		return kindNone, false, false
	}
	kind, evaluated, ok := mathValue(terms[0])
	if !ok {
		return kindNone, false, false
	}
	for i, t := range terms[1:] {
		k, ev, ok := mathValue(t)
		if !ok {
			return kindNone, false, false
		}
		evaluated = evaluated && ev
		switch {
		case ops[i] == '/' && k != kindNumber:
			return kindNone, false, false
		case k == kindNumber:
		case kind == kindNumber:
			kind = k
		default:
			return kindNone, false, false
		}
	}
	return kind, evaluated, true
}

// mathValue reads one operand.
func mathValue(vals []css.ComponentValue) (mathKind, bool, bool) {
	vals = items(vals)
	if len(vals) != 1 {
		return kindNone, false, false
	}
	v := vals[0]
	switch {
	case v.IsBlock() && v.Token.Kind == css.LeftParen:
		return mathSum(v.Values)
	case v.IsFunction():
		name := strings.ToLower(v.Token.Value)
		if !mathFunctions[name] {
			return kindNone, false, false
		}
		k, ev, ok := mathOf(v)
		return k, ev && name == "calc", ok
	case !v.IsToken():
		return kindNone, false, false
	}
	t := v.Token
	switch t.Kind {
	case css.Number:
		return kindNumber, true, true
	case css.Percentage:
		return kindPercent, true, true
	case css.Dimension:
		k := unitKind(t.Unit)
		if k == kindNone {
			return kindNone, false, false
		}
		_, _, supported := pxPerUnit(t.Unit, LengthContext{})
		return k, k == kindLength && supported, true
	case css.Ident:
		switch strings.ToLower(t.Value) {
		case "e", "pi", "infinity", "-infinity", "nan":
			return kindNumber, false, true
		}
	}
	return kindNone, false, false
}

// splitOperators cuts a value at the top-level delimiters in ops, which must
// separate operands: an operator first, last or next to another is not an
// expression.
func splitOperators(vals []css.ComponentValue, ops string) ([][]css.ComponentValue, []byte, bool) {
	var terms [][]css.ComponentValue
	var seen []byte
	start := 0
	for i, v := range vals {
		if !v.IsToken() || v.Token.Kind != css.Delim || len(v.Token.Value) != 1 ||
			!strings.Contains(ops, v.Token.Value) {
			continue
		}
		part := vals[start:i]
		if len(items(part)) == 0 {
			return nil, nil, false
		}
		terms = append(terms, part)
		seen = append(seen, v.Token.Value[0])
		start = i + 1
	}
	last := vals[start:]
	if len(items(last)) == 0 {
		return nil, nil, false
	}
	return append(terms, last), seen, true
}

// Colours.

// colour is a <color>: one this engine reads, or one CSS Color 4 and 5 define
// that it does not compute, which is valid and unevaluated.
func colour(v css.ComponentValue) verdict {
	if _, ok := ParseColor([]css.ComponentValue{v}); ok {
		return valid
	}
	if name, ok := identOf(v); ok {
		switch {
		case name == "currentcolor":
			return valid
		case systemColours[name]:
			return unevaluated("the system colour " + v.Token.Value)
		}
		return invalid
	}
	if !v.IsFunction() {
		return invalid
	}
	name := strings.ToLower(v.Token.Value)
	switch {
	case unevaluatedColourFunctions[name]:
		return unevaluated(name + "()")
	case name == "rgb" || name == "rgba" || name == "hsl" || name == "hsla":
		// ParseColor reads these, and refused this one. It is valid CSS it
		// cannot compute when the arguments hold a math function or the
		// relative syntax's "from" — "rgb(calc(255) 0 0)" — and a mistake
		// otherwise.
		if holdsMathOrFrom(v.Values) {
			return unevaluated(name + "()")
		}
		return invalid
	case otherFunctions[name]:
		return unevaluated(name + "()")
	}
	return invalid
}

// unevaluatedColourFunctions are CSS Color 4 and 5's functions beyond sRGB.
// Their arguments are not checked: a mistake inside one is reported as the
// function this engine does not compute, and that is the one imprecision the
// table allows itself rather than carrying five colour spaces' grammars for
// values it would then drop either way.
var unevaluatedColourFunctions = map[string]bool{
	"lab": true, "lch": true, "oklab": true, "oklch": true, "hwb": true,
	"color": true, "color-mix": true, "light-dark": true, "contrast-color": true,
	"device-cmyk": true,
}

// systemColours are CSS Color 4 §6.2's, and the deprecated ones §6.3 keeps
// valid, lower-cased.
var systemColours = map[string]bool{
	"accentcolor": true, "accentcolortext": true, "activetext": true,
	"buttonborder": true, "buttonface": true, "buttontext": true,
	"canvas": true, "canvastext": true, "field": true, "fieldtext": true,
	"graytext": true, "highlight": true, "highlighttext": true, "linktext": true,
	"mark": true, "marktext": true, "selecteditem": true,
	"selecteditemtext": true, "visitedtext": true,
	"activeborder": true, "activecaption": true, "appworkspace": true,
	"background": true, "buttonhighlight": true, "buttonshadow": true,
	"captiontext": true, "inactiveborder": true, "inactivecaption": true,
	"inactivecaptiontext": true, "infobackground": true, "infotext": true,
	"menu": true, "menutext": true, "scrollbar": true, "threeddarkshadow": true,
	"threedface": true, "threedhighlight": true, "threedlightshadow": true,
	"threedshadow": true, "window": true, "windowframe": true, "windowtext": true,
}

func holdsMathOrFrom(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if v.IsFunction() && (mathFunctions[strings.ToLower(v.Token.Value)] ||
			otherFunctions[strings.ToLower(v.Token.Value)]) {
			return true
		}
		if name, ok := identOf(v); ok && name == "from" {
			return true
		}
	}
	return false
}

// Other terms.

func str(v css.ComponentValue) verdict {
	if v.IsToken() && v.Token.Kind == css.String {
		return valid
	}
	return invalid
}

func url(v css.ComponentValue) verdict {
	if v.IsToken() && v.Token.Kind == css.URL {
		return valid
	}
	if v.IsFunction() && (strings.EqualFold(v.Token.Value, "url") ||
		strings.EqualFold(v.Token.Value, "src")) {
		return valid
	}
	return invalid
}

// image is an <image>: a url, or one of CSS Images' functions. Which of them
// can be painted is the painter's to say; see legalBackgroundImage.
func image(v css.ComponentValue) verdict {
	if url(v).ok {
		return valid
	}
	if v.IsFunction() {
		switch strings.ToLower(v.Token.Value) {
		case "image", "image-set", "-webkit-image-set", "cross-fade", "element",
			"linear-gradient", "radial-gradient", "conic-gradient",
			"repeating-linear-gradient", "repeating-radial-gradient",
			"repeating-conic-gradient", "paint":
			return valid
		}
	}
	return invalid
}

// customIdent is a <custom-ident>: an identifier that is not one of the
// CSS-wide keywords or "default", nor one of the excluded words given.
func customIdent(excluded ...string) term {
	return func(v css.ComponentValue) verdict {
		name, ok := identOf(v)
		if !ok {
			return invalid
		}
		switch name {
		case kwInherit, kwInitial, kwUnset, kwRevert, kwRevertLayer, "default":
			return invalid
		}
		for _, e := range excluded {
			if name == e {
				return invalid
			}
		}
		return valid
	}
}

// Grammars built from terms.

// single is a value of exactly one component.
func single(t term) grammar {
	return func(it []css.ComponentValue) verdict {
		if len(it) != 1 {
			return invalid
		}
		return t(it[0])
	}
}

// oneOf is the first of several grammars that takes the whole value.
func oneOf(gs ...grammar) grammar {
	return func(it []css.ComponentValue) verdict {
		for _, g := range gs {
			if got := g(it); got.ok {
				return got
			}
		}
		return invalid
	}
}

// repeated is between lo and hi components, each taken by t.
func repeated(t term, lo, hi int) grammar {
	return func(it []css.ComponentValue) verdict {
		if len(it) < lo || len(it) > hi {
			return invalid
		}
		out := valid
		for _, v := range it {
			if out = out.and(t(v)); !out.ok {
				return invalid
			}
		}
		return out
	}
}

// anyOrder is CSS's "||": each term at most once, in any order, at least one.
// A component goes to the first unused term that takes it.
func anyOrder(terms ...term) grammar {
	return func(it []css.ComponentValue) verdict {
		if len(it) == 0 || len(it) > len(terms) {
			return invalid
		}
		used := make([]bool, len(terms))
		out := valid
	next:
		for _, v := range it {
			for i, t := range terms {
				if used[i] {
					continue
				}
				if got := t(v); got.ok {
					used[i] = true
					out = out.and(got)
					continue next
				}
			}
			return invalid
		}
		return out
	}
}

// commaList is "g#": one or more values of g separated by commas.
func commaList(g grammar) grammar {
	return func(it []css.ComponentValue) verdict {
		out := valid
		for _, part := range splitOnComma(it) {
			if len(part) == 0 {
				return invalid
			}
			if out = out.and(g(part)); !out.ok {
				return invalid
			}
		}
		return out
	}
}

// fromBool wraps one of the older checks, which answer only yes or no, as a
// grammar over the value with its whitespace.
func fromBool(f func([]css.ComponentValue) bool) grammar {
	return func(it []css.ComponentValue) verdict {
		if f(withSpaces(it)) {
			return valid
		}
		return invalid
	}
}

// withSpaces puts back a space between components, for a check written against
// values as they were declared.
func withSpaces(it []css.ComponentValue) []css.ComponentValue {
	out := make([]css.ComponentValue, 0, 2*len(it))
	for i, v := range it {
		if i > 0 {
			out = append(out, css.ComponentValue{Token: css.Token{Kind: css.Whitespace}})
		}
		out = append(out, v)
	}
	return out
}
