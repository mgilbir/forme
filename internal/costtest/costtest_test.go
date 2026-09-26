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

// bytesSink keeps the allocations below from being optimised away.
var bytesSink []byte

// pieces allocates n pieces of the same size, which is linear in n by
// construction: the helper is being checked, not the allocator's size classes.
// Or, copying, it builds a buffer of n bytes a byte at a time and copies the
// whole of it for every byte — the += that Allocated is for.
func pieces(n int, copying bool) {
	if !copying {
		for range n {
			bytesSink = make([]byte, 64)
		}
		return
	}
	var b []byte
	for i := range n {
		next := make([]byte, len(b)+1)
		copy(next, b)
		next[i] = byte(i)
		b = next
	}
	bytesSink = b
}

// TestAllocatedTellsLinearFromQuadratic is the helper against allocation whose
// curve is known, and it is as sharp as a count, because it is one: n pieces
// and 4n read as four, and copying the whole of a buffer for each byte added
// to it as sixteen.
func TestAllocatedTellsLinearFromQuadratic(t *testing.T) {
	const n = 4000
	if r := Allocated(t, "pieces", func() { pieces(n, false) }, func() { pieces(4*n, false) }); r != 4 {
		t.Errorf("four times the pieces allocated %.2f times as much; want four", r)
	}
	if r := Allocated(t, "copying", func() { pieces(n, true) }, func() { pieces(4*n, true) }); r < 15 || r > 17 {
		t.Errorf("copying four times the bytes allocated %.1f times as much; want about sixteen", r)
	}
}

// TestAllocatedIsNotSpoiledByAnotherGoroutine: what another goroutine allocates
// while a side is measured is counted with it, and the least of the
// measurements is what keeps it out. A goroutine allocating a megabyte during
// each side's first measurement cannot move the answer.
func TestAllocatedIsNotSpoiledByAnotherGoroutine(t *testing.T) {
	const n = 4000
	noisy := func(n int) func() {
		calls := 0
		return func() {
			calls++
			// The first call is the one Allocated makes before measuring;
			// the second is the first measurement.
			if calls == 2 {
				done := make(chan bool)
				go func() { bytesSink = make([]byte, 1<<20); done <- true }()
				<-done
			}
			pieces(n, false)
		}
	}
	if r := Allocated(t, "pieces beside a goroutine", noisy(n), noisy(4*n)); r != 4 {
		t.Errorf("four times the pieces beside another goroutine read as %.2f; want four", r)
	}
}

// TestTimeCopiesGoesThroughTheCopies: every copy is made before anything is
// measured, and the smaller side's calls go through them in turn, so that each
// is called as often as the others to within one call. A comparison that kept
// calling the first would be measuring one input again, which is the cache
// reading TimeCopies exists to avoid.
func TestTimeCopiesGoesThroughTheCopies(t *testing.T) {
	const n = 20000
	var made, called [Copies]int
	measuring := false
	r := TimeCopies(t, "a linear walk over copies", func(k int) func() {
		if measuring {
			t.Fatalf("copy %d was made after the measuring began", k)
		}
		made[k]++
		return func() { called[k]++; walk(n) }
	}, func() { measuring = true; walk(4 * n) })
	for k := range made {
		if made[k] != 1 {
			t.Errorf("copy %d was made %d times, want once", k, made[k])
		}
	}
	lo, hi := called[0], called[0]
	for _, c := range called {
		lo, hi = min(lo, c), max(hi, c)
	}
	if lo == 0 || hi-lo > 1 {
		t.Errorf("the copies were called %v times; each should be called as often as the others", called)
	}
	if r.Ratio > 8 {
		t.Errorf("a linear walk over copies read as %v", r)
	}
}
