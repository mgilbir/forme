package layout

import "testing"

// §9.5.1's rules read across a containing block, and two of them do not have the
// same precondition.
//
// Rule 3 is about the two floats and nothing else: "the right outer edge of a
// left-floating box may not be to the right of the left outer edge of any
// right-floating box that is to the right of it". They need only share a
// formatting context. The band this engine measures text in is clamped to the
// containing block — correctly, since a float beyond that block's edge cannot
// shorten a line that was never that long — and placing a float against the same
// clamped band lost the rule whenever the other float was outside. The suite
// tests it by name: "float placement around other float in BFC but outside
// containing block".
//
// Rule 7 is the containing block's own edge, and it applies only to "a
// left-floating box that has another left-floating box to its left". With no
// float beside it on its own side, a float wider than its containing block
// overflows — rule 8 — and with one beside it, it may not, and drops below that
// float first.
//
// The four fixtures below are the suite's four, at a tenth of the size. Each is
// a 500px block formatting context holding one 50px float, an inner block
// stepped 100px past that float, and the float under test inside it.

// floatIn lays out the shape all four share and returns the tested float's
// position relative to the formatting context.
func floatIn(t *testing.T, sideCSS, innerCSS, testedCSS string) (x, y float64) {
	t.Helper()
	root := layoutOf(t, 1000,
		`<div id="bfc"><div id="side"></div><div id="inner"><div id="t"></div></div></div>`,
		noDefaults+`
		#bfc { float: left; width: 500px; height: 500px }
		#side { height: 300px; width: 50px; `+sideCSS+` }
		#inner { `+innerCSS+` }
		#t { height: 10px; `+testedCSS+` }`)
	bfc := find(t, root, "bfc")
	tested := find(t, root, "t")
	return relX(t, tested, bfc).Px(), relY(t, tested, bfc).Px()
}

// TestALeftFloatClearsARightFloatOutsideItsContainingBlock is rule 3.
//
// The right float sits at 450 to 500, outside the inner block's 0 to 400. The
// tested left float is 475 wide: it cannot reach 475 without passing 450, so it
// goes below the right float and overflows its own block there.
func TestALeftFloatClearsARightFloatOutsideItsContainingBlock(t *testing.T) {
	x, y := floatIn(t, `float: right`, `margin-right: 100px`, `float: left; width: 475px`)
	if x != 0 || y != 300 {
		t.Errorf("the float is at (%v, %v), want (0, 300): 475px cannot fit in the "+
			"450px left of a right float, and the right float ends at 300", x, y)
	}
}

// TestARightFloatClearsALeftFloatOutsideItsContainingBlock is rule 3 mirrored,
// and it is not the same code path: the left float here is *left* of the
// containing block's own edge, so the clamp that hid it is the other one.
func TestARightFloatClearsALeftFloatOutsideItsContainingBlock(t *testing.T) {
	x, y := floatIn(t, `float: left`, `margin-left: 100px`, `float: right; width: 475px`)
	if x != 25 || y != 300 {
		t.Errorf("the float is at (%v, %v), want (25, 300): placed as far right as "+
			"it can go, 500 less 475 is 25, and 25 is left of the left float's 50",
			x, y)
	}
}

// TestALeftFloatDropsPastALeftFloatOutsideItsContainingBlock is rule 7: the
// float is not too wide for the *band*, it is too wide for its containing block,
// and another float on its own side is beside it.
func TestALeftFloatDropsPastALeftFloatOutsideItsContainingBlock(t *testing.T) {
	x, y := floatIn(t, `float: left`, `margin-left: 100px`, `float: left; width: 425px`)
	if x != 100 || y != 300 {
		t.Errorf("the float is at (%v, %v), want (100, 300): rule 7 forbids the "+
			"overflow while a left float is beside it", x, y)
	}
}

// TestARightFloatDropsPastARightFloatOutsideItsContainingBlock is rule 7
// mirrored, and the answer is a negative x — the float overflows its containing
// block to the *left*, which is what rule 8 permits once nothing is beside it.
func TestARightFloatDropsPastARightFloatOutsideItsContainingBlock(t *testing.T) {
	x, y := floatIn(t, `float: right`, `margin-right: 100px`, `float: right; width: 425px`)
	if x != -25 || y != 300 {
		t.Errorf("the float is at (%v, %v), want (-25, 300): 400 of block less 425 "+
			"of float is -25, and it may not take it until the right float has ended",
			x, y)
	}
}

// TestAFloatMayOverflowUpToTheOtherFloatsEdge is rule 3 read the other way, and
// it is what makes the unclamped band load-bearing rather than merely tidy.
//
// The float is 430 wide in a 400-wide block, so it does not fit inside its own
// containing block — and it fits perfectly well in the 450 that rule 3 actually
// allows it. Nothing on its own side is beside it, so rule 7 does not forbid the
// overflow, and it stays at the top. Measuring it against the clamped band would
// find 400, decide it did not fit, and drop it three hundred pixels for room it
// did not need.
func TestAFloatMayOverflowUpToTheOtherFloatsEdge(t *testing.T) {
	x, y := floatIn(t, `float: right`, `margin-right: 100px`, `float: left; width: 430px`)
	if x != 0 || y != 0 {
		t.Errorf("the float is at (%v, %v), want (0, 0): 430px fits in the 450px "+
			"left of the right float, and only the containing block was narrower",
			x, y)
	}
	// One pixel wider than the room rule 3 leaves, and it drops.
	if x, y := floatIn(t, `float: right`,
		`margin-right: 100px`, `float: left; width: 451px`); x != 0 || y != 300 {
		t.Errorf("a 451px float is at (%v, %v), want (0, 300): one pixel past the "+
			"right float's edge is past it", x, y)
	}
}

// TestARightFloatMayOverflowLeftUpToTheOtherFloatsEdge is the mirror, and it is
// a separate path: the edge that was hiding the constraint is the other one.
func TestARightFloatMayOverflowLeftUpToTheOtherFloatsEdge(t *testing.T) {
	x, y := floatIn(t, `float: left`, `margin-left: 100px`, `float: right; width: 430px`)
	if x != 70 || y != 0 {
		t.Errorf("the float is at (%v, %v), want (70, 0): 500 less 430 is 70, and 70 "+
			"is clear of the left float's 50", x, y)
	}
	if x, y := floatIn(t, `float: left`,
		`margin-left: 100px`, `float: right; width: 451px`); x != 49 || y != 300 {
		t.Errorf("a 451px float is at (%v, %v), want (49, 300): 500 less 451 is 49, "+
			"which is inside the left float, so it waits for it to end", x, y)
	}
}

// TestAFloatInsideItsContainingBlockIsUnaffected is the containment case, and it
// is every ordinary document: with both floats inside the block, the clamped
// band and the unclamped one are the same band.
func TestAFloatInsideItsContainingBlockIsUnaffected(t *testing.T) {
	// No margin on the inner block, so the side float is inside it. A 425px
	// float beside a 50px one fits in 500 and stays at the top.
	x, y := floatIn(t, `float: left`, ``, `float: left; width: 425px`)
	if x != 50 || y != 0 {
		t.Errorf("the float is at (%v, %v), want (50, 0): it fits beside the other "+
			"one and nothing pushes it down", x, y)
	}
	// And one that does not fit goes below it, as it always did.
	x, y = floatIn(t, `float: left`, ``, `float: left; width: 475px`)
	if x != 0 || y != 300 {
		t.Errorf("the float is at (%v, %v), want (0, 300)", x, y)
	}
}
