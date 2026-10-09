package style

import (
	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
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
//     function past sRGB, min() over a percentage it cannot yet resolve, a
//     unit nothing here resolves: the declaration is dropped as well, so the
//     one before it stands — which is the fallback an author writes such a
//     declaration after — and the finding says the engine is missing something.
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
	return ascii.Lower(v.Token.Value), true
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
	// readsCalc says the reader of this slot evaluates a calc() of any type
	// the slot takes — a number or an angle as well as a length — so that one
	// is valid rather than valid CSS this engine does not compute.
	readsCalc bool
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
				if unresolvedUnits[ascii.Lower(t.Unit)] {
					return unevaluated("the unit " + ascii.Lower(t.Unit))
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
	kindAngle
	kindTime
	kindFrequency
	kindResolution
	kindFlex
)

// unitKind is what a dimension's unit measures, kindNone for a unit CSS does
// not define.
func unitKind(unit string) mathKind {
	u := ascii.Lower(unit)
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

// mathFunctions are CSS Values 4's math functions, which mathfn.go reads and
// runs.
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
//
// The type is the evaluator's own: the function is read by the code that
// would run it, so the grammar cannot call valid what the reader then cannot
// read.
func mathTerm(v css.ComponentValue, n numeric) verdict {
	name := ascii.Lower(v.Token.Value)
	if otherFunctions[name] {
		return unevaluated(name + "()")
	}
	if !mathFunctions[name] {
		return invalid
	}
	p, ok := compileMath(v, mathScope{typecheck: true, pctAs: n.percentAs()})
	if !ok || !n.takes(p.typ) {
		return invalid
	}
	if p.evaluable && n.reads(p) {
		return valid
	}
	return unevaluated(name + "()")
}

// percentAs is what a percentage in this slot resolves against, for a math
// function's type: a length where the slot takes one, and otherwise a
// percentage of its own, which §10.9 does not let add to a number.
func (n numeric) percentAs() mathKind {
	if n.length {
		return kindLength
	}
	return kindPercent
}

// takes reports whether a slot holds what a math function computes to.
func (n numeric) takes(t mathType) bool {
	if t.pct && !n.percent {
		return false
	}
	switch t.kind {
	case kindNumber:
		return n.number
	case kindLength:
		return n.length
	case kindPercent:
		return n.percent
	case kindAngle:
		return n.angle
	}
	return false
}

// reads reports whether this slot's reader evaluates a math function it holds.
//
// A length's reader is ParseLength, which evaluates any of them that resolves
// to a length: a percentage in it is folded into LengthCalc, or, under min()
// or the like, carried as LengthMath for Resolve to run against the basis. A
// slot whose reader evaluates a number or an angle as well says so with
// readsCalc. Anywhere else, a number, an integer, an angle or a bare
// percentage given by a math function is valid CSS this engine does not
// compute.
func (n numeric) reads(p mathProgram) bool {
	return n.readsCalc || n.length && p.typ.kind == kindLength
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
	name := ascii.Lower(v.Token.Value)
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
		if v.IsFunction() && (mathFunctions[ascii.Lower(v.Token.Value)] ||
			otherFunctions[ascii.Lower(v.Token.Value)]) {
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
	if v.IsFunction() && (ascii.EqualFold(v.Token.Value, "url") ||
		ascii.EqualFold(v.Token.Value, "src")) {
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
		switch ascii.Lower(v.Token.Value) {
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
