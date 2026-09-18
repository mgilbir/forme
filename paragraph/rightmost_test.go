package paragraph

import (
	"math/rand"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Which item of a line is drawn furthest right, which is where the line's own
// trailing spacing hangs.
//
// Rightmost answers it in one comparison per item, from the lowest embedding
// level seen since the item that currently holds the place. That is UAX #9 L2
// read for a single pair rather than applied to a whole line, and the whole
// reason for it is cost: the fill asks this of every candidate it considers, and
// reordering the line each time is quadratic in its items.
//
// A shortcut that cheap has to be checked against the thing it stands in for, so
// these run the reordering itself and compare.

// reorderedRightmost is UAX #9 L2 applied in full: from the highest level down
// to the lowest odd one, reverse every contiguous run of items at or above that
// level. Whatever ends up last is the rightmost.
//
// It is the specification's own algorithm written out, and it is deliberately
// the slow obvious one — a reference that shared code with the thing under test
// would agree with it for the wrong reason.
func reorderedRightmost(levels []int) int {
	order := make([]int, len(levels))
	for i := range order {
		order[i] = i
	}
	highest, lowestOdd := 0, -1
	for _, l := range levels {
		if l > highest {
			highest = l
		}
		if l%2 == 1 && (lowestOdd < 0 || l < lowestOdd) {
			lowestOdd = l
		}
	}
	if lowestOdd >= 0 {
		for lv := highest; lv >= lowestOdd; lv-- {
			for i := 0; i < len(order); i++ {
				if levels[order[i]] < lv {
					continue
				}
				j := i
				for j < len(order) && levels[order[j]] >= lv {
					j++
				}
				for a, b := i, j-1; a < b; a, b = a+1, b-1 {
					order[a], order[b] = order[b], order[a]
				}
				i = j - 1
			}
		}
	}
	return order[len(order)-1]
}

func trackedRightmost(levels []int) int {
	var r Rightmost
	for i, l := range levels {
		r.Add(l, style.Unit(i))
	}
	return int(r.Tail())
}

// TestTheRightmostTrackerAgreesWithAReordering over the level patterns a line
// actually takes: one direction, a run of the other inside it, a run at the end,
// and the nested overrides that make three and four levels.
func TestTheRightmostTrackerAgreesWithAReordering(t *testing.T) {
	for _, levels := range [][]int{
		{0}, {1}, {0, 0, 0}, {1, 1, 1},
		{0, 0, 1, 1}, {1, 1, 2, 2}, {1, 2, 2, 1}, {0, 1, 0},
		{1, 2, 1, 2}, {2, 1, 2}, {0, 1, 2, 1, 0}, {1, 1, 2, 2, 1},
		{0, 2, 1}, {3, 2, 1, 2, 3}, {0, 0, 1, 2, 2, 1, 0},
	} {
		if got, want := trackedRightmost(levels), reorderedRightmost(levels); got != want {
			t.Errorf("levels %v: the tracker says item %d is rightmost and the "+
				"reordering says %d", levels, got, want)
		}
	}
}

// TestTheRightmostTrackerAgreesOverRandomLevels is the same claim where nobody
// chose the pattern.
//
// The levels are bounded at four because that is past anything a document
// reaches — an override inside an override inside a right-to-left paragraph is
// level three — and the lengths are short because the interesting patterns are
// short: what the tracker can get wrong is one comparison, and a long line is
// the same comparison repeated.
func TestTheRightmostTrackerAgreesOverRandomLevels(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 20000; n++ {
		levels := make([]int, 1+rng.Intn(8))
		for i := range levels {
			levels[i] = rng.Intn(5)
		}
		if got, want := trackedRightmost(levels), reorderedRightmost(levels); got != want {
			t.Fatalf("levels %v: the tracker says item %d is rightmost and the "+
				"reordering says %d", levels, got, want)
		}
	}
}

// TestAnEmptyLineHasNothingHanging: the tracker is asked before anything is
// placed, on every line, and the fill subtracts what it answers.
func TestAnEmptyLineHasNothingHanging(t *testing.T) {
	var r Rightmost
	if got := r.Tail(); got != 0 {
		t.Errorf("an empty line hangs %v", got)
	}
	r.Add(0, 64)
	r.Reset()
	if got := r.Tail(); got != 0 {
		t.Errorf("a line that was reset hangs %v", got)
	}
}
