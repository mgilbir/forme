package style

import (
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
)

// calc(), from CSS Values and Units, and what reads one: a length, an angle, a
// number or a percentage. The functions themselves — calc() and the rest of
// CSS Values 4 §10 — are read and run in mathfn.go; this is where a result
// becomes the value its reader holds.
//
// # Why it is evaluated here and not later
//
// A calc() is a length like any other, and this file's job is to turn one into
// the two numbers Length holds: how many units it is, and what percentage of its
// containing block. Everything else in an expression can be settled where the
// expression is read — the font-relative units against the LengthContext the
// caller already supplies, the arithmetic against itself — so a calc() reaches
// layout as a length and layout never learns that it was one.
//
// The percentage is the part that cannot be settled here, because what it is a
// percentage *of* is not known until the containing block is. So it is carried
// alongside the absolute part rather than folded into it, which is what
// LengthCalc is for: "calc(100% - 2em)" is two ems below all of something, and
// neither half of that can be dropped.
//
// A percentage under min(), max() or another function that is not linear in it
// cannot be carried that way — "min(50%, 300px)" is not some pixels plus some
// per cent of anything — and such a length is deferred: see mathfn.go.
//
// # What is refused
//
// A type error, which in this grammar means adding a length to a number,
// multiplying two lengths, or dividing by anything but a number. These are not
// approximated: an expression that does not typecheck is not a length, and CSS
// says the declaration containing it is invalid. The caller reports it and the
// declaration before it stands, which is what a browser does.
//
// Division by zero is not among them. CSS Values 3 made it a parse error, and
// this engine refused it so; Values 4 makes it an infinity, which a top-level
// calculation clamps to the largest value its context holds — "calc(1px / 0)"
// is the widest length there is, as it is in a browser.

// evalLength reads a math function as a length.
//
// deferred says the function is a length this cannot fold, because a
// percentage in it is under a function that is not linear in it. ok is false
// for one that is not a length at all: the declaration is invalid and the
// caller drops it.
func evalLength(fn css.ComponentValue, ctx LengthContext) (l Length, deferred, ok bool) {
	p, ok := compileMath(fn, mathScope{ctx: ctx, pctAs: kindLength})
	if !ok || p.typ.kind != kindLength {
		return Length{}, false, false
	}
	if p.deferred {
		return Length{}, true, false
	}
	v, ok := evalMath(p.code, mathBasis{})
	if !ok {
		return Length{}, false, false
	}
	abs, pct := censoredUnit(v.v), censored(v.pct)
	if pct == 0 {
		return Length{Kind: LengthAbsolute, Value: abs}, false, true
	}
	if abs == 0 {
		return Length{Kind: LengthPercent, Percent: pct}, false, true
	}
	return Length{Kind: LengthCalc, Value: abs, Percent: pct}, false, true
}

// resolveMathLength reads a math function as a length whose percentages are of
// basis, which is known: the font-size case, whose percentages are of the
// parent's size.
func resolveMathLength(fn css.ComponentValue, ctx LengthContext, basis Unit) (Unit, bool) {
	p, ok := compileMath(fn, mathScope{ctx: ctx, pctAs: kindLength})
	if !ok || p.typ.kind != kindLength {
		return 0, false
	}
	v, ok := evalMath(p.code, mathBasis{of: float64(basis), known: true})
	if !ok {
		return 0, false
	}
	return censoredUnit(v.v), true
}

// censored is CSS Values 4 §10.9.2 for a number at the top of a calculation:
// "If a top-level calculation would produce a value whose numeric part is NaN,
// it instead act as though the numeric part is 0", one that would be infinite
// is the largest value of its sign, and a signed zero is the unsigned one.
//
// Only at the top, as the specification says: inside the expression an
// infinity has to be able to meet another and cancel to NaN, and a zero has to
// keep its sign, or "calc(1 / calc(-5 * 0))" would be plus infinity rather
// than minus.
func censored(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return 0
	case math.IsInf(v, 1):
		return math.MaxFloat64
	case math.IsInf(v, -1):
		return -math.MaxFloat64
	}
	return v + 0 // -0 + 0 is +0
}

// calcOperator reads one of a set of single-character operators.
func calcOperator(v css.ComponentValue, of string) (byte, bool) {
	if !v.IsToken() || v.Token.Kind != css.Delim || len(v.Token.Value) != 1 {
		return 0, false
	}
	c := v.Token.Value[0]
	if strings.IndexByte(of, c) < 0 {
		return 0, false
	}
	return c, true
}

// ParseAngle reads an <angle>: a dimension in deg, grad, rad or turn, or a
// math function whose type is <angle>, in degrees. The <zero> some grammars
// allow in an angle's place is theirs to read; "calc(0)" is a number and not
// an angle. A percentage is not an angle here and does not add to one.
func ParseAngle(vals []css.ComponentValue) (float64, bool) {
	deg, _, ok := parseAngleIn(vals, kindPercent)
	return deg, ok
}

// ParseAnglePercentage reads an <angle-percentage>: an angle, a percentage, or a
// math function over both, as its degrees and its percentage apart, since what
// the percentage is of is the caller's — and so one with a percentage under
// min() or the like, which cannot be held apart, is refused.
//
// A NaN the arithmetic makes is censored to zero, as §10.9.2 says. An infinity
// is refused rather than clamped as a length's is: an angle has no largest
// value to stand for one, and the direction of an enormous one is noise.
func ParseAnglePercentage(vals []css.ComponentValue) (deg, pct float64, ok bool) {
	return parseAngleIn(vals, kindAngle)
}

func parseAngleIn(vals []css.ComponentValue, pctAs mathKind) (deg, pct float64, ok bool) {
	p, ok := compileOne(vals, mathScope{pctAs: pctAs}, func(t css.Token) bool {
		return t.Kind == css.Dimension || t.Kind == css.Percentage
	})
	if !ok || p.typ.kind != kindAngle || p.deferred {
		return 0, 0, false
	}
	v, ok := evalMath(p.code, mathBasis{})
	if !ok || math.IsInf(v.v, 0) || math.IsInf(v.pct, 0) {
		return 0, 0, false
	}
	return censored(v.v), censored(v.pct), true
}

// compileOne reads a value that is one component: a math function, or one of
// the tokens the caller takes, as a program of one value.
func compileOne(vals []css.ComponentValue, s mathScope, takes func(css.Token) bool) (mathProgram, bool) {
	vals = skipSpace(vals)
	if len(vals) == 0 || len(skipSpace(vals[1:])) != 0 {
		return mathProgram{}, false
	}
	switch v := vals[0]; {
	case isMathFunction(v):
		return compileMath(v, s)
	case v.IsToken() && takes(v.Token):
		c := mathCompiler{scope: s, evaluable: true}
		n, ok := c.token(v.Token)
		return mathProgram{code: string(n.code), typ: n.typ, evaluable: true}, ok
	}
	return mathProgram{}, false
}

// degreesPer is how many degrees one of an angle unit is.
func degreesPer(unit string) (float64, bool) {
	switch ascii.Lower(unit) {
	case "deg":
		return 1, true
	case "grad":
		return 0.9, true
	case "rad":
		return 180 / math.Pi, true
	case "turn":
		return 360, true
	}
	return 0, false
}

// skipSpace drops the white space in front of a value.
func skipSpace(vals []css.ComponentValue) []css.ComponentValue {
	for len(vals) > 0 && vals[0].IsToken() && vals[0].Token.Kind == css.Whitespace {
		vals = vals[1:]
	}
	return vals
}

// ParseNumberPercentage reads a <number> or a <percentage>, or a math function
// of either, as a number, a percentage being a hundredth. A percentage here is
// a percentage of its own and not a number, so a function that adds one to a
// number does not type-check, as §10.9 says; one that makes a NaN is zero, and
// one that makes an infinity is refused, for the reason an angle's is.
func ParseNumberPercentage(vals []css.ComponentValue) (float64, bool) {
	p, ok := compileOne(vals, mathScope{pctAs: kindPercent}, func(t css.Token) bool {
		return t.Kind == css.Number || t.Kind == css.Percentage
	})
	if !ok || (p.typ.kind != kindNumber && p.typ.kind != kindPercent) {
		return 0, false
	}
	v, ok := evalMath(p.code, mathBasis{})
	if !ok || math.IsInf(v.v, 0) {
		return 0, false
	}
	out := censored(v.v)
	if p.typ.kind == kindPercent {
		out /= 100
	}
	return out, true
}
