package costtest

import (
	"testing"
	"time"
)

// sink keeps the work below from being optimised away.
var sink int

// walk is n steps of work that does not allocate.
func walk(n int) {
	s := 0
	for i := range n {
		s += i ^ (s >> 3)
	}
	sink += s
}

// TestTimeTellsLinearFromQuadratic is the helper against work whose curve is
// known: n steps against 4n, and n² against (4n)². The bound every cost test
// uses is eight, and a helper that could not put the one under it and the
// other over it would be deciding those tests by itself.
func TestTimeTellsLinearFromQuadratic(t *testing.T) {
	const n = 20000
	linear := Time(t, "a linear walk", func() { walk(n) }, func() { walk(4 * n) })
	if linear.Ratio > 8 {
		t.Errorf("a linear walk read as %v", linear)
	}
	const m = 300
	quadratic := Time(t, "a quadratic walk", func() { walk(m * m) }, func() { walk(16 * m * m) })
	if quadratic.Ratio < 8 {
		t.Errorf("a quadratic walk read as %v", quadratic)
	}
	// Both measurements were made long enough to measure, and as long as each
	// other whatever the curve: the calls are counted out for each side.
	for _, r := range []Result{linear, quadratic} {
		a := r.Small * time.Duration(r.SmallCalls)
		b := r.Large * time.Duration(r.LargeCalls)
		if a < Window/2 || b < Window/2 {
			t.Errorf("%v: the measurements were %v and %v, under the window", r, a, b)
		}
		if a > 2*b || b > 2*a {
			t.Errorf("%v: the measurements were %v and %v, not the same length", r, a, b)
		}
	}
}

// TestAFixtureBuiltOnceIsNotMeasured: work whose first call builds something
// it keeps — a memoised fixture, a table built lazily — is measured after the
// building, so that the building does not make the measurements look long and
// cut how many calls and how many timings each is given.
func TestAFixtureBuiltOnceIsNotMeasured(t *testing.T) {
	const n = 20000
	built := map[int]bool{}
	at := func(n int) func() {
		return func() {
			if !built[n] {
				built[n] = true
				walk(2000 * n) // far longer than longWindow
			}
			walk(n)
		}
	}
	r := Time(t, "a walk whose first call builds a fixture", at(n), at(4*n))
	if r.Timings != Timings {
		t.Errorf("%v: measured %d times; the building is not a measurement", r, r.Timings)
	}
	if got := r.Small * time.Duration(r.SmallCalls); got < Window/2 {
		t.Errorf("%v: the measurement was %v, under the window", r, got)
	}
}
