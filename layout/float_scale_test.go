package layout

import (
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// TestFloatPlacementIsLinearInFloatCount guards the shape of bug that float
// placement had and that no correctness test could see.
//
// Every question a floatContext answered used to be a scan of every float in it,
// and placing one float asks several of them — so the work was quadratic while
// every answer was right. It showed only as time: laying out one block of empty
// 3×3 floats took 7 ms at a thousand floats and 2.0 s at thirty-two thousand,
// which is 32× the document for 280× the work, and a hundred thousand floats —
// seven hundred kilobytes of markup, which anyone can hand this engine — took
// eighteen seconds. It is 0.45 s now.
//
// The guard is a ratio: the same loop over n floats and over 4n, which is about
// four for the staircase and sixteen for the scan. Planted — the band, the next
// float bottom, the clearance and the lowest bottom each answered by scanning
// every float, as they were — it reads as 14.4, 14.8 with memory being streamed
// on the machine's other cores, and 15.6 under the race detector.
//
// It was a bound on the clock: two hundred thousand floats in under four
// seconds, which was fifty times the loop's 80 ms on a workstation and a
// twentieth of the scan's ninety seconds. The race job on a GitHub runner took
// 4.2 and 4.6 seconds over the linear loop, because the race detector makes
// every memory access a call and a shared runner is slower again; a bound on
// the clock is a bound on the machine as much as on the code, and no margin
// holds on every machine the suite runs on. A ratio does not care how fast the
// machine is. Four thousand floats and sixteen thousand: at two thousand and
// eight thousand the planted scan, which is a quick walk of a short list, was
// only 13.4 times the loop, and at this size it is past fourteen while the loop
// itself still reads about four under all three conditions.
//
// It drives the context directly rather than a document, for the reason
// html/parse_scale_test.go drives the tokenizer directly: the subject is one
// data structure, and going through Build would spend nine tenths of the time
// parsing and styling the markup that carries the floats and leave the ratio
// measuring the wrong thing.
func TestFloatPlacementIsLinearInFloatCount(t *testing.T) {
	size := Size{W: 3 * 64, H: 3 * 64}
	lo, hi := style.Unit(0), style.Unit(800*64)
	place := func(floats int) func() {
		return func() {
			fc := &floatContext{}
			for i := 0; i < floats; i++ {
				side := FloatLeft
				if i%7 == 0 {
					// Both staircases, and both are asked for every band.
					side = FloatRight
				}
				r := fc.place(size, side, 0, lo, hi)

				// The queries the rest of layout makes, so that a scan
				// reintroduced in any one of them is caught here and not only
				// in place's own search. They are asked about the float just
				// placed, which is where the answers are least likely to be
				// trivial.
				fc.bandOver(r.Y, r.Bottom(), lo, hi)
				fc.nextBottomBelow(r.Y)
				if i%64 == 0 {
					fc.clearance(ClearBoth)
					fc.bottom()
				}
			}
			if n := len(fc.boxes); n != floats {
				t.Fatalf("%d floats were placed, want %d", n, floats)
			}
			// The floats tile the block, so the last one has to have been
			// pushed a long way down: an assertion that the search really ran,
			// rather than a ratio met by a placement that gave up.
			if last := fc.boxes[floats-1].rect; last.Y <= 0 {
				t.Fatalf("the last of %d floats is at y=%d, so nothing was stacked", floats, last.Y)
			}
		}
	}
	const n = 4000
	c := costtest.Time(t, "placing n floats", place(n), place(4*n))
	if c.Ratio > 8 {
		t.Errorf("placing %d floats took %v and %d took %v, a factor of %.1f: the "+
			"staircase is linear, about four, and the scan it replaced is sixteen",
			n, c.Small, 4*n, c.Large, c.Ratio)
	}
}
