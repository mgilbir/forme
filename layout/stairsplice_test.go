package layout

import (
	"testing"
	"time"

	"github.com/mgilbir/forme/style"
)

// TestARowOfDecreasingFloatsIsNotQuadratic is the other order the staircase can
// be built in, and the one the slice was worst at.
//
// A row of floats of decreasing height gives every float a breakpoint of its
// own, and each new float's span begins above all of them — so the window it
// rewrites is at the *front* of the staircase and the splice moved everything
// already there. Floats stacked down the page splice at the end and cost
// nothing, which is why this never showed in the measurements the index was
// built from.
//
//	n          slice      tree
//	32,000      0.11 s    0.043 s
//	64,000      0.44 s    0.079 s
//	128,000     4.8 s     0.178 s
//	256,000    26.5 s     0.367 s
//
// The bound is wall-clock, for the reason float_scale_test.go gives: the
// quantity under test is time, and the margin is what makes it safe.
//
// It is set for the race detector, because `make race` runs this suite under it
// and the detector is what decides the number: the loop takes 0.28 s plainly
// and 3.3 s instrumented on a CI runner, and the slice takes about 11 s plainly
// and 53 s instrumented. So the bound is three and a half times the honest
// instrumented cost and less than a quarter of the instrumented quadratic — and
// forty times the honest cost when the detector is off.
func TestARowOfDecreasingFloatsIsNotQuadratic(t *testing.T) {
	const floats = 200000

	fc := &floatContext{}
	lo, hi := style.Unit(0), style.Unit(1<<29)
	start := time.Now()
	for i := 0; i < floats; i++ {
		r := fc.place(Size{W: 3 * 64, H: style.Unit((floats - i) * 64)}, FloatLeft, 0, lo, hi)
		// The queries the rest of layout makes of the staircase, so that a scan
		// reintroduced in one of them is caught here as well.
		fc.bandOver(r.Y, r.Bottom(), lo, hi)
	}
	elapsed := time.Since(start)

	// Every float has a step of its own, which is what makes this the shape the
	// test is about: a run that merged them would be measuring something else.
	if steps := fc.idx.left.steps.len(); steps < floats/2 {
		t.Fatalf("the left staircase has %d steps for %d floats of distinct "+
			"heights; this is not the row this test is about", steps, floats)
	}
	if elapsed > 12*time.Second {
		t.Errorf("placing %d floats of decreasing height took %v; the spliced "+
			"slice this replaced took about 11 s and the tree takes about 0.28 s "+
			"— 53 s and 3.3 s under the race detector, which is what the bound "+
			"allows for", floats, elapsed)
	}
}

// TestTheStaircaseComesBackExactly is the undo, over the order that makes the
// splice hardest: every float's window overlaps every earlier one.
//
// rewind is not an optimisation but a correctness requirement — layout places a
// subtree, learns the collapsed margin above it, and places it again — so the
// staircase after removing float k has to be the staircase that existed before
// float k was added, breakpoint for breakpoint. The tree's undo is a set of
// insertions and removals rather than a splice back, so it is a different
// argument from the one the slice made and needs its own check.
func TestTheStaircaseComesBackExactly(t *testing.T) {
	const floats = 200

	fc := &floatContext{}
	lo, hi := style.Unit(0), style.Unit(1<<20)

	// The staircase after each float, recorded as it is built.
	snapshots := make([][]stairStep, 0, floats+1)
	fc.sync()
	snapshots = append(snapshots, fc.idx.left.steps.all())
	for i := 0; i < floats; i++ {
		fc.place(Size{W: 3 * 64, H: style.Unit((floats - i) * 64)}, FloatLeft, 0, lo, hi)
		fc.sync()
		snapshots = append(snapshots, fc.idx.left.steps.all())
	}

	// And unwound one float at a time, comparing against what was recorded.
	for k := floats; k >= 0; k-- {
		fc.idx.rewind(k)
		got := fc.idx.left.steps.all()
		want := snapshots[k]
		if len(got) != len(want) {
			t.Fatalf("after rewinding to %d floats the staircase has %d steps "+
				"and had %d when it was built", k, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("after rewinding to %d floats, step %d is %+v and was "+
					"%+v when it was built", k, i, got[i], want[i])
			}
		}
	}
}
