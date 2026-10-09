package style

import (
	"math"
	"testing"

	"github.com/mgilbir/forme/css"
)

// The math functions of CSS Values 4 §10, evaluated: the comparison functions,
// the stepped ones, the sign-related, trigonometric and exponential ones, and
// §10.9.2's infinities, NaN and signed zeros. Most cases are the
// specification's own examples; the rest are its argument-range tables.

// TestMathFunctionsAreEvaluatedAsLengths is each function as a width would
// read it, against calcCtx: an em of 20px, a rem of 16px, a vw of 10px and a
// vh of 5px.
func TestMathFunctionsAreEvaluatedAsLengths(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64 // in px
	}{
		{"min(10px, 2em)", 10},
		{"max(10px, 2em)", 40},
		{"min(1em, 1rem, 30px)", 16},
		{"max(4px)", 4},
		// §10.2's own: ten times the average of vw and vh, at least 12px.
		{"max(10 * (1vw + 1vh) / 2, 12px)", 75},
		{"clamp(12px, 10 * (1vw + 1vh) / 2, 100px)", 75},
		{"clamp(10px, 50px, 30px)", 30},
		{"clamp(10px, 5px, 30px)", 10},
		// MIN wins when MIN > MAX: clamp(MIN, VAL, MAX) is max(MIN, min(VAL,
		// MAX)), and "clamp(100px, ..., 50px) will resolve to 100px".
		{"clamp(100px, 70px, 50px)", 100},
		{"clamp(100px, 20px, 50px)", 100},
		{"clamp(none, 70px, 50px)", 50},
		{"clamp(60px, 10px, none)", 60},
		{"clamp(none, 7px, none)", 7},
		// Nested: a calc() inside min(), min() inside calc(), and deeper.
		{"min(calc(10px + 5px), 12px)", 12},
		{"calc(min(10px, 20px) * 2)", 20},
		{"calc(1em - max(5px, 1rem))", 4},
		{"min(max(1px, 5px), 3px)", 3},
		{"max(min(1px, 5px), min(2px, 3px))", 2},
		{"calc((min(10px, 2em) + max(1px, 2px)) / 4)", 3},
		// round(), §10.3.
		{"round(17px, 5px)", 15},
		{"round(nearest, 17px, 5px)", 15},
		{"round(up, 17px, 5px)", 20},
		{"round(down, 17px, 5px)", 15},
		{"round(to-zero, 17px, 5px)", 15},
		{"round(17.5px, 5px)", 20}, // a tie goes to the upper multiple
		{"round(-17.5px, 5px)", -15},
		{"round(up, -17px, 5px)", -15},
		{"round(down, -17px, 5px)", -20},
		{"round(to-zero, -17px, 5px)", -15},
		{"round(-17px, 5px)", -15},
		{"round(17px, -5px)", 15}, // the multiples of -5px are those of 5px
		{"round(20px, 5px)", 20},
		{"round(1em, 3px)", 21},
		// mod() and rem(), §10.3's examples.
		{"mod(18px, 5px)", 3},
		{"rem(18px, 5px)", 3},
		{"mod(-18px, 5px)", 2},
		{"rem(-18px, 5px)", -3},
		{"mod(18px, -5px)", -2},
		{"rem(18px, -5px)", 3},
		{"mod(-18px, -5px)", -3},
		{"rem(-18px, -5px)", -3},
		// The sign-related ones.
		{"abs(-10px)", 10},
		{"abs(10px - 2em)", 30},
		{"calc(10px * sign(-3em))", -10},
		{"calc(10px * sign(3em))", 10},
		// The exponential ones.
		{"hypot(30px, 40px)", 50},
		{"hypot(-2em)", 40},
		{"hypot(3em, 4em)", 100},
		{"calc(1px * pow(2, 3))", 8},
		{"calc(1rem * pow(1.5, 2))", 36},
		{"calc(1px * sqrt(16))", 4},
		{"calc(1px * log(8, 2))", 3},
		{"calc(1px * exp(0))", 1},
		{"calc(100px * sin(30deg))", 50},
		{"calc(100px * cos(0.25turn) + 1px)", 1},
		{"calc(1px * round(pi * 100))", 314},
		{"calc(1px * round(E * 1000))", 2718},
		// Case-insensitive, as every CSS keyword is.
		{"MIN(10px, 2em)", 10},
		{"calc(1px * round(PI))", 3},
	} {
		got := mustLength(t, tc.in, calcCtx)
		if got.Kind != LengthAbsolute {
			t.Errorf("%s came out as kind %v, want an absolute length", tc.in, got.Kind)
			continue
		}
		if math.Abs(got.Value.Px()-tc.want) > 0.02 {
			t.Errorf("%s is %gpx, want %g", tc.in, got.Value.Px(), tc.want)
		}
	}
}

// TestATopLevelInfinityOrNaNIsCensored is §10.9.2: within a calculation the
// arithmetic is IEEE-754's, and at the top a NaN is zero and an infinity the
// largest value of its sign — for a length, the end of Unit's range.
func TestATopLevelInfinityOrNaNIsCensored(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Unit
	}{
		{"calc(nan * 1px)", 0},
		{"calc(NaN * 1px)", 0},
		{"calc(infinity * 1px)", MaxUnit},
		{"calc(-infinity * 1px)", MinUnit},
		{"calc(1px / 0)", MaxUnit},
		{"calc(-1px / 0)", MinUnit},
		{"calc(0px / 0)", 0},
		{"calc(1px * (infinity - infinity))", 0},
		{"calc(infinity * 1px - infinity * 1px)", 0},
		// NaN infects min() and max(): a max() a NaN is in is NaN, not its
		// other argument.
		{"max(nan * 1px, 5px)", 0},
		{"min(5px, nan * 1px)", 0},
		{"clamp(nan * 1px, 5px, 10px)", 0},
		{"clamp(1px, 5px, nan * 1px)", 0},
		{"min(infinity * 1px, 10px)", calcPx(10)},
		{"max(-infinity * 1px, 10px)", calcPx(10)},
		// §10.9.2's example: the inner calc() is 0⁻ and is not censored,
		// because it is not at the top, so the quotient is −∞.
		{"calc(1px / calc(-5 * 0))", MinUnit},
		{"calc(1px / (-5 * 0))", MinUnit},
		// A written -0 is the unsigned zero.
		{"calc(1px / -0)", MaxUnit},
		{"calc(1px / sign(-0))", MaxUnit},
		// Past the range without an infinity is the end of the range too.
		{"calc(1e7px * 1e7)", MaxUnit},
		{"calc(1e7px * 1e7 - 1e7px * 1e7)", 0},
	} {
		got := mustLength(t, tc.in, calcCtx)
		if got.Kind != LengthAbsolute || got.Value != tc.want {
			t.Errorf("%s is %+v, want the absolute %d units", tc.in, got, tc.want)
		}
	}
	// The percentage half is censored the same way.
	if got := mustLength(t, "calc(infinity * 1%)", calcCtx); got.Kind != LengthPercent ||
		got.Percent != math.MaxFloat64 {
		t.Errorf("calc(infinity * 1%%) is %+v, want the largest percentage", got)
	}
	if got := mustLength(t, "calc(50% * infinity)", calcCtx); got.Kind != LengthPercent ||
		got.Percent != math.MaxFloat64 {
		t.Errorf("calc(50%% * infinity) is %+v: an absent length half is not a zero "+
			"one to make a NaN from", got)
	}
}

// mathNumber runs a calculation of a number and returns its value before any
// censoring, so that a signed zero, an infinity and a NaN can be seen.
func mathNumber(t *testing.T, src string) float64 {
	t.Helper()
	vals, _ := css.ParseComponentValues(src)
	vals = skipSpace(vals)
	if len(vals) != 1 {
		t.Fatalf("%q is not one function", src)
	}
	p, ok := compileMath(vals[0], mathScope{pctAs: kindPercent})
	if !ok || p.typ.kind != kindNumber {
		t.Fatalf("%q is not a number (ok=%v, type %+v)", src, ok, p.typ)
	}
	v, ok := evalMath(p.code, mathBasis{})
	if !ok {
		t.Fatalf("%q did not evaluate", src)
	}
	return v.v
}

// TestTheArgumentRangesAreTheSpecifications is every table in §10.3.1,
// §10.4.1 and §10.5.1, and the signed-zero rules of §10.9.2, inside the
// calculation where they live.
func TestTheArgumentRangesAreTheSpecifications(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	negZero := math.Copysign(0, -1)
	for _, tc := range []struct {
		in   string
		want float64
	}{
		// Signed zeros.
		{"calc(-5 * 0)", negZero},
		{"calc(-0)", 0},
		{"calc(1 / -infinity)", negZero},
		{"min(0, -5 * 0)", negZero},
		{"max(0, -5 * 0)", 0},
		{"clamp(0, -5 * 0, 1)", 0},
		{"sign(-5 * 0)", negZero},
		{"sign(0)", 0},
		{"sign(-3)", -1},
		{"sign(nan)", nan},
		{"abs(-5 * 0)", 0},
		{"abs(-infinity)", inf},
		// round().
		{"round(1, 0)", nan},
		{"round(infinity, infinity)", nan},
		{"round(infinity, 5)", inf},
		{"round(-infinity, 5)", -inf},
		{"round(1, infinity)", 0},
		{"round(-1, infinity)", negZero},
		{"round(to-zero, -1, infinity)", negZero},
		{"round(up, 1, infinity)", inf},
		{"round(up, 0, infinity)", 0},
		{"round(up, -1, infinity)", negZero},
		{"round(down, -1, infinity)", -inf},
		{"round(down, -5 * 0, infinity)", negZero},
		{"round(down, 1, infinity)", 0},
		{"round(-0.4)", negZero}, // upper B is zero, and so 0⁻
		{"round(0.4)", 0},
		{"round(2.5)", 3},
		{"round(-2.5)", -2},
		{"round(down, 2.5)", 2},
		{"round(-5 * 0, 1)", negZero}, // a multiple already, kept exactly
		{"round(nan, 1)", nan},
		// mod() and rem().
		{"mod(1, 0)", nan},
		{"rem(1, 0)", nan},
		{"mod(infinity, 1)", nan},
		{"rem(-infinity, 1)", nan},
		{"mod(1, infinity)", 1},
		{"rem(1, infinity)", 1},
		{"mod(-1, infinity)", nan}, // opposite signs, in mod() only
		{"rem(-1, infinity)", -1},
		{"mod(1, -infinity)", nan},
		{"mod(-5 * 0, infinity)", nan}, // an oppositely signed zero counts
		{"mod(-1, -infinity)", -1},
		{"mod(4, 2)", 0},
		{"mod(4, -2)", negZero}, // the range starts at 0⁻ when B is negative
		{"rem(-4, 2)", negZero},
		{"mod(5.5, 2)", 1.5},
		// Trigonometry, §10.4.1.
		{"sin(infinity)", nan},
		{"cos(-infinity * 1deg)", nan},
		{"sin(-5 * 0)", negZero},
		{"tan(-5 * 0 * 1deg)", negZero},
		{"tan(90deg)", inf},
		{"tan(450deg)", inf},
		{"tan(-270deg)", inf},
		{"tan(-90deg)", -inf},
		{"tan(270deg)", -inf},
		{"sin(90deg)", 1},
		{"cos(0)", 1},
		{"sin(0.25turn)", 1},
		// Exponentials, §10.5.1, and NaN infecting what JavaScript lets it
		// not.
		{"pow(nan, 0)", nan},
		{"pow(1, infinity)", nan},
		{"pow(-1, -infinity)", nan},
		{"pow(0.5, infinity)", 0},
		{"pow(2, -infinity)", 0},
		{"pow(0, -1)", inf},
		{"pow(-5 * 0, -1)", -inf},
		{"pow(-8, 0.5)", nan},
		{"pow(-2, 3)", -8},
		{"pow(nan, 1)", nan},
		{"hypot(infinity, nan)", nan},
		{"hypot(infinity, 1)", inf},
		{"hypot(3, 4)", 5},
		{"sqrt(-1)", nan},
		{"sqrt(-5 * 0)", negZero},
		{"sqrt(infinity)", inf},
		{"log(0)", -inf},
		{"log(-1)", nan},
		{"log(1)", 0},
		{"log(1, 0.5)", 0},
		{"log(8, 1)", nan},
		{"log(8, -2)", nan},
		{"log(100, 10)", 2},
		{"log(infinity)", inf},
		{"exp(-infinity)", 0},
		{"exp(infinity)", inf},
		{"calc(exp(1) - e)", 0},
		// The constants.
		{"calc(pi)", math.Pi},
		{"calc(e)", math.E},
		{"calc(InFiNiTy)", inf},
		{"calc(-infinity)", -inf},
	} {
		got := mathNumber(t, tc.in)
		same := got == tc.want && math.Signbit(got) == math.Signbit(tc.want) ||
			math.IsNaN(got) && math.IsNaN(tc.want)
		if !same && !math.IsNaN(tc.want) && !math.IsInf(tc.want, 0) && tc.want != 0 &&
			math.Abs(got-tc.want) < 1e-12 {
			same = true
		}
		if !same {
			t.Errorf("%s is %v (sign bit %v), want %v (sign bit %v)", tc.in, got,
				math.Signbit(got), tc.want, math.Signbit(tc.want))
		}
	}
}

// TestMathFunctionsTypeCheck is §10.9's type checking: what is not a length
// is not read as one, and a function of the wrong arity or the wrong types is
// not a value at all.
func TestMathFunctionsTypeCheck(t *testing.T) {
	for _, in := range []string{
		"min(1px, 2)",           // a length and a number are not consistent
		"max(1px, 1deg)",        // nor a length and an angle
		"min()",                 // nothing
		"clamp(1px, 2px)",       // two of three
		"clamp(1px, none, 2px)", // the central value cannot be none
		"clamp(1px, 2px, 3px, 4px)",
		"round(1.5px)", // B may be left out for a number only
		"round(sideways, 1px, 2px)",
		"round(up, 1px, 2px, 3px)",
		"mod(1px)",    // two arguments
		"mod(1px, 2)", // of one type
		"rem(1px, 2px, 3px)",
		"abs(1px, 2px)",
		"sign(1px)",   // a number, not a length
		"sin(1px)",    // a number or an angle
		"pow(2px, 2)", // numbers only
		"sqrt(4px)",
		"log(1, 2, 3)",
		"exp(1px)",
		"atan2(1px, 1)",
		"calc(1px, 2px)",      // calc() takes one
		"min(1px +-2px, 3px)", // "+" needs space on both sides
		"min(1px 2px)",
		"min(1px, env(x))", // a substitution function is not typed
		"calc(min(1px, 2px) * 1px)",
		"atan(1)", // an angle, not a length
		"calc(foo)",
		"calc(1px * -pi)", // "-pi" is not a constant
	} {
		vals, _ := css.ParseComponentValues(in)
		l, unsupported, ok := ParseLength(vals, calcCtx)
		if ok || unsupported {
			t.Errorf("%s was read as the length %+v (unsupported=%v); it is not one",
				in, l, unsupported)
		}
	}
}

// TestAPercentageUnderMinIsResolvedAgainstTheBasis is the half of a math
// function that cannot be settled where it is read: "min(50%, 300px)" depends
// on what the percentage is of. It is carried as LengthMath and run by
// Resolve; a linear use of a percentage is still LengthCalc, however deeply
// the functions beside it nest.
func TestAPercentageUnderMinIsResolvedAgainstTheBasis(t *testing.T) {
	for _, tc := range []struct {
		in          string
		basis, want float64 // in px
	}{
		{"min(50%, 300px)", 400, 200},
		{"min(50%, 300px)", 1000, 300},
		{"max(50%, 300px)", 400, 300},
		{"clamp(10px, 5% + 2em, 50%)", 200, 50}, // 10 + 40, under 100
		{"clamp(10px, 5% + 2em, 50%)", 60, 30},  // 3 + 40, over 30
		{"clamp(100px, 50%, 10%)", 400, 100},    // MIN wins over MAX
		{"calc(1px * sign(10%))", 400, 1},
		{"calc(1px * sign(10%))", -400, -1}, // a negative basis, as §10.6 warns
		{"abs(50% - 100px)", 100, 50},
		{"round(50%, 7px)", 100, 49},
		{"round(up, 50%, 7px)", 100, 56},
		{"calc(min(10%, 10px) + 1px)", 50, 6},
		{"calc(100% - min(10%, 10px))", 200, 190},
		{"min(10%, 20%)", 100, 10},
		{"min(10%, 20%)", -100, -20},
		{"mod(100%, 30px)", 100, 10},
		{"hypot(30%, 40px)", 100, 50},
		{"calc(10px * sign(50% - 12px))", 20, -10},
		{"min(50% * infinity, 1px)", 100, 1},
		{"max(nan * 1%, 1px)", 100, 0},                   // NaN infects, then is censored
		{"calc(min(50%, 300px) / 0)", 400, MaxUnit.Px()}, // an infinity, clamped
	} {
		l := mustLength(t, tc.in, calcCtx)
		if l.Kind != LengthMath || !l.HasPercent() || l.Value != 0 || l.Percent != 0 {
			t.Errorf("%s is %+v, want a LengthMath with nothing else in it", tc.in, l)
			continue
		}
		got, ok := l.Resolve(calcPx(tc.basis), true)
		if !ok || math.Abs(got.Px()-tc.want) > 0.02 {
			t.Errorf("%s of %gpx is %gpx (ok=%v), want %g", tc.in, tc.basis, got.Px(), ok,
				tc.want)
		}
		// Of an indefinite basis it is as indefinite as a bare percentage.
		if _, ok := l.Resolve(calcPx(tc.basis), false); ok {
			t.Errorf("%s resolved against an indefinite basis", tc.in)
		}
	}
	// Two lengths written alike are equal, which a map of them relies on, and
	// two written differently are not.
	a, b := mustLength(t, "min(50%, 2em)", calcCtx), mustLength(t, "min(50%,  40px)", calcCtx)
	if a != b {
		t.Errorf("min(50%%, 2em) and min(50%%, 40px) at a 20px em are %+v and %+v", a, b)
	}
	if c := mustLength(t, "min(50%, 41px)", calcCtx); a == c {
		t.Error("min(50%, 40px) and min(50%, 41px) compare equal")
	}
	got := mustLength(t, "calc(min(10px, 2em) + 50% - max(1px, 2px))", calcCtx)
	if got.Kind != LengthCalc || got.Percent != 50 || got.Value.Px() != 8 {
		t.Errorf("a percentage beside min() is %+v, want 50%% plus 8px", got)
	}
}

// TestAFontSizeResolvesAPercentageUnderMin: a font-size's percentages are of
// the parent's size, which is known when it is computed, so min() over one is
// evaluated there.
func TestAFontSizeResolvesAPercentageUnderMin(t *testing.T) {
	for _, tc := range []struct {
		in             string
		parent, wantPx float64
	}{
		{"min(150%, 20px)", 16, 20},
		{"min(150%, 30px)", 16, 24},
		{"clamp(10px, 50%, 30px)", 16, 10},
		{"clamp(10px, 200%, 30px)", 16, 30},
		{"max(80%, 1em)", 20, 20}, // an em in a font-size is the parent's
		{"calc(min(100%, 10px) + 50%)", 20, 20},
		{"calc(10px * sign(50% - 12px))", 30, 10},
		{"calc(10px * sign(50% - 12px))", 16, 0}, // -10px, and a negative size is refused
	} {
		vals, _ := css.ParseComponentValues(tc.in)
		got, _, ok := ResolveFontSize(vals, calcPx(tc.parent), calcPx(16))
		if tc.wantPx == 0 {
			if ok {
				t.Errorf("%s of %gpx was %gpx; a negative size is refused", tc.in,
					tc.parent, got.Px())
			}
			continue
		}
		if !ok || math.Abs(got.Px()-tc.wantPx) > 0.02 {
			t.Errorf("%s of %gpx is %gpx (ok=%v), want %g", tc.in, tc.parent,
				got.Px(), ok, tc.wantPx)
		}
	}
}

// TestTheGrammarAndTheReaderAgreeAboutMathFunctions: the value grammar types a
// math function with the reader's own compiler, so what it calls a valid,
// evaluated length is one ParseLength reads, and what it calls invalid is not.
// A count keeps two answers agreeing about nothing from passing.
func TestTheGrammarAndTheReaderAgreeAboutMathFunctions(t *testing.T) {
	checked := 0
	for _, in := range []string{
		"calc(1px +-2px)", "calc(1px + -2px)", "min(1px, 2px)", "min(1px,2px)",
		"clamp(1px, 2em, 3px)", "round(up, 1px, 2px)", "round(1px)", "mod(1px, 2px)",
		"calc(1px / 0)", "calc(nan * 1px)", "hypot(1px)", "calc(1px * sin(1deg))",
		"min(1px, 2)", "abs(-1px)", "sign(1px)", "calc(1px * sign(1px))",
		"calc(2 * min(1px, 3px) / 2)", "clamp(none, 1px, none)", "max(1em, 1rem)",
		"calc(1px * log(2, 3))", "calc(1px * pow(2, 0.5))", "calc(1px * atan(1) / 1deg)",
	} {
		vals, _ := css.ParseComponentValues(in)
		verdict := judgeValue("width", vals)
		_, _, read := ParseLength(vals, calcCtx)
		if verdict.ok && verdict.unsupported == "" != read || !verdict.ok && read {
			t.Errorf("%s: the grammar judged %+v and the reader read=%v", in, verdict, read)
		}
		checked++
	}
	if checked != 22 {
		t.Fatalf("checked %d values", checked)
	}
}

// TestAMathFunctionInAFontSizeComputesToPixels: a font-size is the one length
// whose computed value this engine serializes, and a min() in it serializes as
// the size it came to.
func TestAMathFunctionInAFontSizeComputesToPixels(t *testing.T) {
	for _, tc := range []struct{ css, want string }{
		{`div { font-size: 20px } #p { font-size: min(150%, 25px) }`, "25px"},
		{`div { font-size: 20px } #p { font-size: clamp(30px, 50%, 20px) }`, "30px"},
		{`div { font-size: 20px } #p { font-size: max(1em, 2rem) }`, "32px"},
	} {
		got, findings := winner(t, tc.css, "font-size")
		if got != tc.want {
			t.Errorf("%s: font-size is %q, want %q (%v)", tc.css, got, tc.want, findings)
		}
	}
}
