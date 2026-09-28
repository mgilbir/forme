package style

import (
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
)

// calc(), from CSS Values and Units.
//
// An expression here is a length, or an angle. The two share the grammar and
// the arithmetic and differ only in the unit their dimensions measure: a
// length's is resolved against the LengthContext, and an angle's is a number of
// degrees wherever it is written. Adding one to the other is a type error, as
// adding either to a number is.
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
// # What is refused
//
// A type error, which in this grammar means adding a length to a number,
// multiplying two lengths, or dividing by anything but a number. These are not
// approximated: an expression that does not typecheck is not a length, and CSS
// says the declaration containing it is invalid. The caller reports it and the
// declaration before it stands, which is what a browser does.
//
// Division by zero is the same case and is refused for the same reason rather
// than saturating: a length of infinity is not what anybody wrote.
//
// min(), max() and clamp() are not implemented. They are separate functions
// rather than part of this grammar, and each has its own argument rules; what is
// here is calc() and the parentheses inside it.

// calcTerm is a value part-way through an expression: a plain number, or a
// dimension — a length or an angle — with a percentage part beside it.
//
// The kinds are kept apart because the grammar treats them differently — a
// number may multiply a length and a length may not multiply a length — and
// because "calc(2 * 3)" is a number, which is not a length and not accepted as
// one. A bare percentage is neither a length nor an angle until it is added to
// one, and so has neither flag.
type calcTerm struct {
	number   float64
	abs      Unit
	deg      float64
	pct      float64
	isNumber bool
	isLength bool
	isAngle  bool
}

// evalCalc reads a calc() function's arguments into a length.
//
// The second result says the expression was well formed. A malformed one is not
// a length and not a value this engine merely fails to compute: the declaration
// is invalid and the caller drops it.
func evalCalc(vals []css.ComponentValue, ctx LengthContext) (Length, bool) {
	t, rest, ok := calcSum(vals, ctx)
	if !ok || len(skipSpace(rest)) != 0 || t.isNumber || t.isAngle {
		return Length{}, false
	}
	t.pct = censored(t.pct)
	if t.pct == 0 {
		return Length{Kind: LengthAbsolute, Value: t.abs}, true
	}
	if t.abs == 0 {
		return Length{Kind: LengthPercent, Percent: t.pct}, true
	}
	return Length{Kind: LengthCalc, Value: t.abs, Percent: t.pct}, true
}

// calcSum is the "+" and "-" level: the loosest-binding one, so it is read last
// and calls into the tighter one for each of its operands.
func calcSum(vals []css.ComponentValue, ctx LengthContext) (calcTerm, []css.ComponentValue, bool) {
	left, rest, ok := calcProduct(vals, ctx)
	if !ok {
		return calcTerm{}, nil, false
	}
	for {
		// CSS requires white space around + and -, and the requirement is not
		// decoration: without it "calc(1px -2px)" would be a subtraction or a
		// length followed by a negative length depending on which way you
		// squint, and the tokenizer has already chosen the second. So an
		// operator is a delimiter with space on *both* sides, and anything else
		// ends the sum.
		//
		// Both sides, which is what §10.1 says and what this checked on one:
		// "calc(1px +-2px)" was read as a subtraction, because the space in
		// front was there and the tokenizer had already made "-2px" a negative
		// length — so a declaration no browser accepts came out as minus one
		// pixel rather than as the mistake it is.
		after := skipSpace(rest)
		if len(after) == len(rest) || len(after) == 0 {
			return left, rest, true
		}
		op, isOp := calcOperator(after[0], "+-")
		if !isOp {
			return left, rest, true
		}
		if tail := after[1:]; len(tail) == 0 || !tail[0].IsToken() ||
			tail[0].Token.Kind != css.Whitespace {
			return calcTerm{}, nil, false
		}
		right, more, ok := calcProduct(after[1:], ctx)
		if !ok {
			return calcTerm{}, nil, false
		}
		left, ok = calcAdd(left, right, op == '-')
		if !ok {
			return calcTerm{}, nil, false
		}
		rest = more
	}
}

// calcProduct is the "*" and "/" level, which binds tighter and needs no space
// around its operators.
func calcProduct(vals []css.ComponentValue, ctx LengthContext) (calcTerm, []css.ComponentValue, bool) {
	left, rest, ok := calcValue(vals, ctx)
	if !ok {
		return calcTerm{}, nil, false
	}
	for {
		after := skipSpace(rest)
		if len(after) == 0 {
			return left, rest, true
		}
		op, isOp := calcOperator(after[0], "*/")
		if !isOp {
			return left, rest, true
		}
		right, more, ok := calcValue(after[1:], ctx)
		if !ok {
			return calcTerm{}, nil, false
		}
		if op == '*' {
			left, ok = calcMul(left, right)
		} else {
			left, ok = calcDiv(left, right)
		}
		if !ok {
			return calcTerm{}, nil, false
		}
		rest = more
	}
}

// censored is CSS Values 4 §10.9 for the percentage half of a finished
// expression: "If a top-level calculation would produce a value whose numeric
// part is NaN, it instead act as though the numeric part is 0", and one that
// would be infinite is the largest value of its sign.
//
// Every number an expression starts from is finite — the tokenizer clamps what
// it reads — but the arithmetic between them is a float's: "calc(1e308% * 10)"
// is an infinity and taking it from itself is NaN. The absolute half cannot do
// either, because a length is a Unit and a Unit saturates. The percentage half
// is a float64 carried to layout as one, and a NaN there is a percentage every
// comparison in a table's column widths lets through.
//
// Only at the top, as the specification says: inside the expression an
// infinity has to be able to meet another and cancel to NaN, or a clamp in
// the middle would make "calc(x * 10 - x * 10)" come out as something other
// than what it is.
func censored(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return 0
	case math.IsInf(v, 1):
		return math.MaxFloat64
	case math.IsInf(v, -1):
		return -math.MaxFloat64
	}
	return v
}

// calcValue is one operand: a number, a length, a percentage, a parenthesised
// sum, or a nested calc().
func calcValue(vals []css.ComponentValue, ctx LengthContext) (calcTerm, []css.ComponentValue, bool) {
	vals = skipSpace(vals)
	if len(vals) == 0 {
		return calcTerm{}, nil, false
	}
	v := vals[0]
	switch {
	case v.IsBlock() && v.Token.Kind == css.LeftParen,
		v.IsFunction() && ascii.EqualFold(v.Token.Value, "calc"):
		inner, rest, ok := calcSum(v.Values, ctx)
		if !ok || len(skipSpace(rest)) != 0 {
			return calcTerm{}, nil, false
		}
		return inner, vals[1:], true

	case v.IsToken() && v.Token.Kind == css.Number:
		return calcTerm{number: v.Token.Number, isNumber: true}, vals[1:], true

	case v.IsToken() && v.Token.Kind == css.Percentage:
		return calcTerm{pct: v.Token.Number}, vals[1:], true

	case v.IsToken() && v.Token.Kind == css.Dimension:
		if deg, ok := degreesPer(v.Token.Unit); ok {
			if math.IsNaN(v.Token.Number) || math.IsInf(v.Token.Number, 0) {
				return calcTerm{}, nil, false
			}
			return calcTerm{deg: v.Token.Number * deg, isAngle: true}, vals[1:], true
		}
		px, known, supported := pxPerUnit(v.Token.Unit, ctx)
		if !supported || !known {
			return calcTerm{}, nil, false
		}
		u, ok := FromPx(v.Token.Number * px)
		if !ok {
			return calcTerm{}, nil, false
		}
		return calcTerm{abs: u, isLength: true}, vals[1:], true
	}
	return calcTerm{}, nil, false
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

// calcAdd is "+" and "-": both operands have to be the same kind of thing, and
// a percentage takes the kind of what it is added to. A length added to an
// angle is carried as both, and refused where the expression is read, since
// nothing reads both: evalCalc refuses an angle and ParseAnglePercentage a
// length.
func calcAdd(a, b calcTerm, minus bool) (calcTerm, bool) {
	if a.isNumber != b.isNumber {
		return calcTerm{}, false
	}
	if minus {
		b.number, b.abs, b.deg, b.pct = -b.number, Unit(0).Sub(b.abs), -b.deg, -b.pct
	}
	if a.isNumber {
		return calcTerm{number: a.number + b.number, isNumber: true}, true
	}
	return calcTerm{abs: a.abs.Add(b.abs), deg: a.deg + b.deg, pct: a.pct + b.pct,
		isLength: a.isLength || b.isLength, isAngle: a.isAngle || b.isAngle}, true
}

// calcMul is "*": one side has to be a number, since a length times a length is
// an area and there is nowhere in CSS to put one.
func calcMul(a, b calcTerm) (calcTerm, bool) {
	switch {
	case a.isNumber && b.isNumber:
		return calcTerm{number: a.number * b.number, isNumber: true}, true
	case a.isNumber:
		return scaled(b, a.number), true
	case b.isNumber:
		return scaled(a, b.number), true
	}
	return calcTerm{}, false
}

// scaled is a dimension or a percentage multiplied by a number.
func scaled(t calcTerm, by float64) calcTerm {
	return calcTerm{abs: t.abs.Mul(by), deg: t.deg * by, pct: t.pct * by,
		isLength: t.isLength, isAngle: t.isAngle}
}

// calcDiv is "/": the divisor has to be a number, and not zero.
func calcDiv(a, b calcTerm) (calcTerm, bool) {
	if !b.isNumber || b.number == 0 {
		return calcTerm{}, false
	}
	if a.isNumber {
		return calcTerm{number: a.number / b.number, isNumber: true}, true
	}
	return calcTerm{abs: a.abs.Div(b.number), deg: a.deg / b.number, pct: a.pct / b.number,
		isLength: a.isLength, isAngle: a.isAngle}, true
}

// ParseAngle reads an <angle>: a dimension in deg, grad, rad or turn, or a
// calc() whose type is <angle>, in degrees. The <zero> some grammars allow in
// an angle's place is theirs to read; "calc(0)" is a number and not an angle.
func ParseAngle(vals []css.ComponentValue) (float64, bool) {
	deg, pct, ok := ParseAnglePercentage(vals)
	if !ok || pct != 0 {
		return 0, false
	}
	return deg, true
}

// ParseAnglePercentage reads an <angle-percentage>: an angle, a percentage, or a
// calc() that sums them, as its degrees and its percentage apart, since what
// the percentage is of is the caller's. A NaN or an infinity the arithmetic
// makes is refused rather than censored as a length's is: an angle has no
// largest value to stand for one.
func ParseAnglePercentage(vals []css.ComponentValue) (deg, pct float64, ok bool) {
	vals = skipSpace(vals)
	if len(vals) == 0 {
		return 0, 0, false
	}
	var t calcTerm
	switch v := vals[0]; {
	case v.IsFunction() && ascii.EqualFold(v.Token.Value, "calc"):
		var rest []css.ComponentValue
		t, rest, ok = calcSum(v.Values, LengthContext{})
		if !ok || len(skipSpace(rest)) != 0 {
			return 0, 0, false
		}
	case v.IsToken() && (v.Token.Kind == css.Dimension || v.Token.Kind == css.Percentage):
		t, _, ok = calcValue(vals[:1], LengthContext{})
		if !ok {
			return 0, 0, false
		}
	default:
		return 0, 0, false
	}
	if len(skipSpace(vals[1:])) != 0 || t.isNumber || t.isLength {
		return 0, 0, false
	}
	if math.IsNaN(t.deg) || math.IsInf(t.deg, 0) || math.IsNaN(t.pct) || math.IsInf(t.pct, 0) {
		return 0, 0, false
	}
	return t.deg, t.pct, true
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

// ParseNumberPercentage reads a <number> or a <percentage>, or a calc() of
// either, as a number, a percentage being a hundredth. A calc() that adds a
// number to a percentage does not type-check, and neither does one that makes
// an infinity or a NaN.
func ParseNumberPercentage(vals []css.ComponentValue) (float64, bool) {
	vals = skipSpace(vals)
	if len(vals) == 0 || len(skipSpace(vals[1:])) != 0 {
		return 0, false
	}
	var t calcTerm
	switch v := vals[0]; {
	case v.IsFunction() && ascii.EqualFold(v.Token.Value, "calc"):
		var rest []css.ComponentValue
		var ok bool
		t, rest, ok = calcSum(v.Values, LengthContext{})
		if !ok || len(skipSpace(rest)) != 0 {
			return 0, false
		}
	case v.IsToken() && v.Token.Kind == css.Number:
		t = calcTerm{number: v.Token.Number, isNumber: true}
	case v.IsToken() && v.Token.Kind == css.Percentage:
		t = calcTerm{pct: v.Token.Number}
	default:
		return 0, false
	}
	out := t.pct / 100
	switch {
	case t.isNumber:
		out = t.number
	case t.isLength || t.isAngle:
		return 0, false
	}
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return 0, false
	}
	return out, true
}
