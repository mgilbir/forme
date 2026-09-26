// Package costtest measures how the cost of a piece of work grows with its
// input, for the tests that guard a shape of algorithm rather than a speed.
//
// Every such test here asks the same question: the same shape of input at n
// and at four times n (or at two sizes the test names), and how much more the
// larger costs. Linear work comes out at about four and the quadratic it
// replaced at about sixteen, and the bound between them — usually eight — is a
// factor of two from each. A ratio does not care how fast the machine is or
// whether the race detector is making everything fifteen times slower, which
// is why none of these tests bounds a time of its own.
//
// There are two ways to have the two numbers, and the first is always better.
//
//   - Counted (Count). Where the code under test already counts the work it
//     does — a work budget charged per step, a matcher's steps, a font's
//     budget — the count is the same number on a laptop, on a shared CI runner
//     and under the race detector, and nothing the machine is doing can move
//     it. A test that can count should. The engine is not instrumented for its
//     tests, though: work nothing counts is timed.
//   - Timed (Time). A time is spoiled by whatever else the machine is doing,
//     and every way a busy machine spoils one reads as a steeper curve — the
//     rest of the suite runs beside these tests, and a CI runner is shared. A
//     linear parse read as 10.8 for four times the input with two test suites
//     running beside it, when it was the least of three single calls of a
//     few milliseconds each. Time closes each of the ways that happened; see
//     its comment.
package costtest

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
)

// Timings is how many times each side of a timed ratio is measured, and the
// least of them is the one kept: a clock is only ever spoiled upward, so the
// least of several is the one the machine disturbed least.
const Timings = 15

// Window is the least time one measurement of either side is made to take.
// A millisecond is a scheduler's slice, and one lost slice in it doubles it;
// testing.B's default is a second for the same reason, and twenty
// milliseconds is what fifteen timings of each of two sides can afford.
const Window = 20 * time.Millisecond

// longWindow is how long a measurement may be before it is taken fewer than
// Timings times; see Time.
const longWindow = 10 * Window

// minTimings is the fewest times a side is measured however long it takes.
const minTimings = 3

// maxCalls stops the search for a window's length on work too small to
// measure, which a test should not be timing.
const maxCalls = 1 << 22

// Result is one timed comparison: what one call of each side took, how many
// calls made one measurement of it and how many measurements of each side
// were taken, and the ratio of the per-call costs, large to small.
type Result struct {
	Small, Large           time.Duration
	SmallCalls, LargeCalls int
	Timings                int
	Ratio                  float64
}

func (r Result) String() string {
	return fmt.Sprintf("%.1f times (%v a call at the smaller size, the least of %d "+
		"measurements of %d calls; %v at the larger, of %d calls)",
		r.Ratio, r.Small, r.Timings, r.SmallCalls, r.Large, r.LargeCalls)
}

// Time measures small and large — the same work at two sizes — and returns
// what one call of each costs and the ratio between them, large to small. The
// caller bounds the ratio; the result is logged under what, so that a run
// with -v shows every curve and not only the ones that failed.
//
// Each way a busy machine was seen to spoil a ratio is closed:
//
//   - A window too short to measure. Calls are repeated until one
//     measurement takes Window, as testing.B repeats them.
//   - Windows of different lengths. A longer window is more likely to be
//     spoiled, and when both sides make the same number of calls the long one
//     is the large side, so the least of several short timings is compared
//     with the least of several long ones and the curve reads steeper than it
//     is. Each side's calls are counted out separately, so that the two
//     windows are the same length whatever the curve: four calls at n against
//     one at 4n when the work is linear, sixteen when it is quadratic.
//   - Too few chances. Each side is the least of Timings measurements. What
//     the least of many is for is a disturbance as long as the measurement,
//     and on the thread's clock that is a stall of the machine and not a
//     turn of the scheduler; a measurement longer than longWindow is not
//     moved by one, and fifteen of them cost what one of these tests should
//     not — a single call that takes a second, as shaping thirty-two
//     thousand Devanagari syllables does, would be half a minute, and ten
//     times that under the race detector. So a comparison whose
//     measurements are longer than longWindow is measured as many times as
//     fit in what Timings measurements of longWindow would have taken, and
//     never fewer than minTimings.
//   - Load that comes and goes. The two sides are measured alternately, so
//     that a spell of it falls on both.
//   - Time the test did not run. The clock is the thread's own processor
//     clock where the system has one (Linux), with the goroutine held to its
//     thread, so time spent waiting for a processor while something else ran
//     is not counted at all. Elsewhere it is the wall clock, which the other
//     measures have to carry alone.
//   - The collector. It runs to completion before each measurement, so each
//     starts from the same heap and none pays for the garbage of the one
//     before. It is not held off while a measurement runs, although holding it
//     off was tried: work that is quadratic in what it allocates is a defect
//     like any other, and with the collector waiting its garbage cost nothing
//     until the heap was full — a data: stylesheet named by its whole URL, the
//     defect TestADataStylesheetIsNamedShort is about, read as 8.6 with it held
//     off and 14.9 with it running. Linear work collects in proportion to what
//     it allocates, which is linear, and the windows being the same length
//     keeps a collection from landing in one side's measurement and not the
//     other's.
//
// Both are called once before anything is measured, even before the calls
// that make a measurement are counted out, so that every lazily built table is
// built and every memoised fixture made: a fixture built inside the first
// count made that measurement look long, and the calls and the timings were
// cut to fit it.
func Time(t testing.TB, what string, small, large func()) Result {
	t.Helper()
	small()
	large()
	ns, ds := calls(small)
	nl, dl := calls(large)
	// Neither window can be shorter than one call, so the longer of the two
	// is the length both are made.
	w := max(ds, dl)
	ns = stretch(ns, ds, w)
	nl = stretch(nl, dl, w)
	timings := Timings
	if w > longWindow {
		timings = max(minTimings, int(Timings*longWindow/w))
	}
	a, b := time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)
	for range timings {
		a = min(a, window(ns, small))
		b = min(b, window(nl, large))
	}
	if a <= 0 {
		t.Fatalf("%s: %d calls at the smaller size measured %v; there is nothing to "+
			"compare the larger with", what, ns, a)
	}
	r := Result{
		Small: a / time.Duration(ns), Large: b / time.Duration(nl),
		SmallCalls: ns, LargeCalls: nl, Timings: timings,
		Ratio: (float64(b) / float64(nl)) / (float64(a) / float64(ns)),
	}
	t.Logf("%s: %v", what, r)
	return r
}

// calls is how many calls of f make one measurement at least Window long,
// found as testing.B finds its N, by growing it towards what the last
// measurement predicts; and how long that measurement took.
func calls(f func()) (int, time.Duration) {
	n := 1
	for {
		d := window(n, f)
		if d >= Window || n >= maxCalls {
			return n, d
		}
		next := int(1.2 * float64(n) * float64(Window) / float64(max(d, time.Microsecond)))
		n = min(max(next, n+1), 100*n, maxCalls)
	}
}

// stretch is the calls that take w, when n calls took d.
func stretch(n int, d, w time.Duration) int {
	if d <= 0 || d >= w {
		return n
	}
	return min(int(math.Ceil(float64(n)*float64(w)/float64(d))), maxCalls)
}

// window is how long k calls of f take on the clock Time describes, from a
// collected heap.
func window(k int, f func()) time.Duration {
	runtime.GC()
	return onThreadClock(func() {
		for range k {
			f()
		}
	})
}

// Count compares two counts of the same work, at the smaller size and the
// larger, and returns the ratio of the larger to the smaller, logged under
// what. The smaller must have counted something: a comparison of nothing with
// nothing passes whatever the shape, and a fixture that stopped reaching the
// work being counted would pass it silently.
func Count(t testing.TB, what string, small, large int64) float64 {
	t.Helper()
	if small <= 0 {
		t.Fatalf("%s: the smaller input counted %d units of work, so there is no "+
			"work to compare the larger with", what, small)
	}
	ratio := float64(large) / float64(small)
	t.Logf("%s: %.1f times (%d units of work at the smaller size, %d at the larger)",
		what, ratio, small, large)
	return ratio
}
