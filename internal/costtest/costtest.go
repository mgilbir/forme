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
//     it. A test that can count should. The heap counts one thing for every
//     test, the bytes allocated (Allocated), and that is the count for a
//     defect that copies. The engine is not instrumented for its tests,
//     though: other work nothing counts is timed.
//   - Timed (Time). A time is spoiled by whatever else the machine is doing,
//     and every way a busy machine spoils one reads as a steeper curve — the
//     rest of the suite runs beside these tests, and a CI runner is shared. A
//     linear parse read as 10.8 for four times the input with two test suites
//     running beside it, when it was the least of three single calls of a
//     few milliseconds each. Time closes each of the ways that happened; see
//     its comment.
//
// One way a time is spoiled is not the machine's load, and Time cannot close
// it: the cache. Work whose input fits in the processor's cache at n and not
// at 4n pays more for each step at the larger size although it does no more
// steps, and linear work reads as eight. Where that happens depends on the
// machine's cache, and on how much of it the machine's other work is using,
// and not on the code: the phrase separator pass read as 4.4 on a workstation
// and 8.0 to 9.9 on every GitHub runner. So a timed test does one of three
// things. It counts instead, where what the defect grows can be counted
// (Allocated, for a copy). It times an input it reads and keeps with
// TimeCopies, which gives the smaller side the larger's memory. Or, where each
// call makes its memory afresh, it keeps the larger size to a few hundred
// kilobytes, which the level-two cache of every processor these tests run on
// holds.
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

// Copies is how many inputs of the smaller size TimeCopies goes through: the
// larger input is four times the smaller, so four of the smaller are as much
// memory as one of the larger.
const Copies = 4

// TimeCopies is Time for work whose input is too large for the processor's
// cache to hold four times over. small(k) makes the k-th of Copies distinct
// inputs of the smaller size and returns the work on it; each call of the
// smaller side does the work on the next of them in turn. large is the work on
// the one input of the larger size.
//
// A time is a count of steps multiplied by what a step costs, and what a step
// costs depends on whether what it reads is still in the cache. Measured on
// one input, the smaller side reads the same memory call after call, and an
// input that fits the cache at n and not at 4n pays less for each step at the
// smaller size: linear work reads as eight, where the cache happens to end,
// which depends on the machine and on what else is running and not on the
// code. Four inputs of the smaller size are the larger's memory, gone through
// one after another, so each call of either side finds the cache holding what
// the calls before it read and not its own input, and the two sides pay alike
// for a step. Nested bordered spans, a line of a thousand against one of two
// hundred and fifty, read as 5.5 to 8.6 timed on one input with memory being
// streamed on the machine's other cores, and 3.3 to 4.5 through this. It
// narrows what the machine can do to a ratio and does not end it: the same
// streaming still moves a chain of eight thousand items as far as 8.0.
//
// What it does not equalise is memory the work makes for itself: a map built
// afresh by every call is the call's own size at either side.
//
// It is for inputs that are read and kept. Work whose memory is made fresh by
// every call — a document laid out from its markup — cannot be given copies,
// and keeps its sizes small instead; see the package comment.
func TimeCopies(t testing.TB, what string, small func(k int) func(), large func()) Result {
	t.Helper()
	var work [Copies]func()
	for k := range work {
		work[k] = small(k)
	}
	next := 0
	return Time(t, what, func() {
		work[next]()
		next = (next + 1) % Copies
	}, large)
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

// Allocations is how many times each side of an allocated comparison is
// measured, and the least of them is the one kept. See Allocated.
const Allocations = 3

// Allocated compares the bytes that one call of small and one call of large
// allocate — the same work at two sizes — and returns the ratio, large to
// small, logged under what.
//
// It is the count for work whose defect is copying. A string grown a piece at
// a time with +=, a slice copied whole to add one element to it, a prefix
// taken again for every position: each of those is quadratic in the bytes it
// allocates before it is quadratic in anything else, and the bytes a call
// allocates are the same number on any machine, under the race detector, and
// however large the input is beside the processor's cache. That last is what
// a time cannot promise. Linear work over a tree of tens of megabytes costs
// twice as much a node once the tree no longer fits in the cache, so a timed
// ratio of four becomes eight exactly where the larger input crosses the
// cache's size, and where that is depends on the machine: the separator pass
// over twenty thousand leaves and eighty thousand read 4.4 on a workstation and
// 8.0 to 9.9 on every GitHub runner.
//
// It says nothing about work that allocates nothing, so it is only the right
// measure where the defect being guarded allocates; a walk done twice over is
// not seen by it at all.
//
// The sizes want to be large enough that the linear work's own allocation is
// proportional to its input, which below some tens of thousands of elements it
// is not: append doubles a small slice and grows a large one by a quarter, so
// a list grown an element at a time allocates about twice its length while it
// is small and five times once it is large, and linear work read as 6.5 at five
// thousand elements and twenty thousand.
//
// The count is the heap's cumulative allocation (runtime.MemStats.TotalAlloc)
// before and after one call, which is exact for the goroutine making it and is
// spoiled only by what another goroutine allocates in the meantime: the least of
// Allocations measurements is kept, because that can only add. Both are called
// once before anything is measured, as Time does, so that a table built lazily
// on the first call is not counted as the work.
func Allocated(t testing.TB, what string, small, large func()) float64 {
	t.Helper()
	small()
	large()
	a, b := uint64(math.MaxUint64), uint64(math.MaxUint64)
	for range Allocations {
		a = min(a, allocatedBy(small))
		b = min(b, allocatedBy(large))
	}
	if a == 0 {
		t.Fatalf("%s: a call at the smaller size allocated nothing, so there is "+
			"nothing to compare the larger with", what)
	}
	ratio := float64(b) / float64(a)
	t.Logf("%s: %.1f times (%d bytes allocated at the smaller size, %d at the larger)",
		what, ratio, a, b)
	return ratio
}

// allocatedBy is the bytes the heap allocated while f ran.
func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}
