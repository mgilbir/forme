package style

import (
	"math"
	"testing"
)

// A length is quantised downwards, and that is one decision with one reason.
//
// A Unit is a sixty-fourth of a pixel, so almost every length a stylesheet names
// has to be moved to one: 0.87em of 200px is 174px, which is exact, but 4ch of a
// 7.8px digit is 31.2px and is not. Which way it moves is a choice, and rounding
// to the nearest is the accurate one — half a sixty-fourth of error either way,
// against a whole one always downwards.
//
// It is not the choice this engine makes, because accuracy is not what a layout
// engine is asked for here. What it is asked for is that a box built out of n
// things of size x holds n of them, and that is a property of the *direction*
// rather than of the size of the error: a whole number of units taken downwards
// from n·x is never less than n times a whole number taken downwards from x,
// and under rounding it can be less by half a unit per part.
//
// The suite says it three ways. units-005 puts a hundred floats of 0.87em in a
// box of 8.7em and shows red through the gap; both overflow-wrap-*-003 put
// "PASS FAIL" in a box of 4ch and break the word because "PASS" measures wider
// than the four digits the box was built from.
func TestALengthIsQuantisedDownwards(t *testing.T) {
	for _, tc := range []struct {
		px   float64
		want Unit
		what string
	}{
		{1, 64, "a whole pixel"},
		{0.5, 32, "half a pixel, which is a whole number of units"},
		{7.8, 499, "7.8px, which is 499.2 units"},
		{31.2, 1996, "31.2px, which is 1996.8 units — rounding gives 1997"},
		{0.01, 0, "a hundredth of a pixel, which is less than a unit"},
		{-7.8, -499, "a negative, which moves towards zero and not away from it"},
		{0, 0, "nothing"},
	} {
		if got, ok := FromPx(tc.px); !ok || got != tc.want {
			t.Errorf("%s: FromPx(%v) is %v (ok=%v), want %v", tc.what, tc.px, got, ok, tc.want)
		}
	}

	// And Mul, which is the same conversion from the other side: a length
	// already in units multiplied by a plain number. The cases are the ones
	// where the two directions differ, which is the only kind that says
	// anything — 665.33 is 665 whichever rule is applied.
	for _, tc := range []struct {
		u    Unit
		f    float64
		want Unit
		what string
	}{
		{1997, 1.0 / 3, 665, "a third of 1997 units, which is 665.67"},
		{3, 0.5, 1, "half of three units"},
		{1996, 1.0 / 3, 665, "a third of 1996 units, which is 665.33"},
		{-3, 0.5, -1, "half of minus three, which moves towards zero"},
	} {
		if got := tc.u.Mul(tc.f); got != tc.want {
			t.Errorf("%s: %v.Mul(%v) is %v, want %v", tc.what, tc.u, tc.f, got, tc.want)
		}
	}
}

// TestNPartsFitAContainerOfNTimesTheirSize is the property the choice above is
// made for, asserted as a property rather than as a table.
//
// It is what a stylesheet writing "width: 4ch" beside four digits depends on,
// and what "width: 8.7em" holding ten floats of "0.87em" depends on, and it
// fails under rounding for a great many x — a hundred of them below, if the
// direction is ever changed back.
func TestNPartsFitAContainerOfNTimesTheirSize(t *testing.T) {
	// Sizes chosen to land all over the sixty-fourths rather than on them: a
	// twentieth of a pixel is 3.2 units, a font's advance is whatever the font
	// says, and a percentage of an odd width is worse.
	sizes := []float64{0.05, 0.87, 1.0 / 3, 7.8, 7.796875, 11.11, 0.001, 123.456}
	counts := []int{2, 3, 4, 7, 10, 16, 100}
	for _, x := range sizes {
		part, ok := FromPx(x)
		if !ok {
			t.Fatalf("FromPx(%v) saturated", x)
		}
		for _, n := range counts {
			whole, ok := FromPx(x * float64(n))
			if !ok {
				t.Fatalf("FromPx(%v) saturated", x*float64(n))
			}
			if got := part.Mul(float64(n)); got > whole {
				t.Errorf("%d parts of %vpx are %v and a container of %d of them "+
					"is %v: the parts do not fit", n, x, got, n, whole)
			}
			// And the same sum done by addition rather than by multiplication,
			// which is how a line adds its runs up.
			var sum Unit
			for i := 0; i < n; i++ {
				sum = sum.Add(part)
			}
			if sum > whole {
				t.Errorf("%d parts of %vpx add up to %v and a container of %d of "+
					"them is %v: the parts do not fit", n, x, sum, n, whole)
			}
		}
	}
}

// TestRoundingIsWhatTheDirectionIsChosenAgainst shows the choice has teeth: the
// same property, asked of the rounding this engine used to do, fails.
//
// Without this the test above says only "the current arithmetic satisfies the
// current arithmetic", and would go on saying it if the direction were changed.
func TestRoundingIsWhatTheDirectionIsChosenAgainst(t *testing.T) {
	round := func(px float64) Unit { return Unit(math.Round(px * 64)) }
	broken := 0
	for _, x := range []float64{0.05, 0.87, 1.0 / 3, 7.8, 11.11, 123.456} {
		for _, n := range []int{2, 3, 4, 7, 10, 16, 100} {
			if round(x)*Unit(n) > round(x*float64(n)) {
				broken++
			}
		}
	}
	if broken == 0 {
		t.Error("rounding to the nearest unit fits the parts in the container " +
			"for every size tried, so the direction this engine quantises in " +
			"is not what makes the property above hold")
	}
}
