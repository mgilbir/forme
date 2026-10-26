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
	order := reordered(levels)
	return order[len(order)-1]
}

// reordered is the visual order L2 gives a line of items at these levels,
// leftmost first.
func reordered(levels []int) []int {
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
	return order
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

// TestTheTrackerPassesOverACharacterThatDrawsNothing is the same claim with
// runs of zero-width formatting characters among the items, which Place passes
// over in finding the letter whose spacing hangs.
//
// The reference reorders the whole line, takes the last item that draws
// something, and adds the gap of the item at the very end where that item is
// one that draws nothing. Each item's number is distinct, so a sum that came
// from the wrong pair cannot pass for the right one: a letter's spacing is its
// position plus one, and a formatting run's gap a hundred times that, or zero
// in a quarter of them so that the runs with no gap are tried as well.
//
// It found the first repair wrong before it was committed. That one carried the
// tail of the item that held the place over to the formatting run, and
// [0 1 1] with the middle item a mark — "x&#x61C;<a>ع</a>" — is drawn x, ع,
// mark: the letter beside the mark is the one placed after it.
func TestTheTrackerPassesOverACharacterThatDrawsNothing(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 20000; n++ {
		items := make([]Item, 1+rng.Intn(8))
		levels := make([]int, len(items))
		tails := make([]style.Unit, len(items))
		for i := range items {
			levels[i] = rng.Intn(5)
			items[i] = Item{Text: "x", Level: levels[i]}
			tails[i] = style.Unit(i + 1)
			if rng.Intn(2) == 0 {
				items[i].Text = "\u200B"
				tails[i] = style.Unit(100 * (i + 1))
				if rng.Intn(4) == 0 {
					tails[i] = 0
				}
			}
		}
		var want style.Unit
		order := reordered(levels)
		for p := len(order) - 1; p >= 0; p-- {
			if items[order[p]].Text == "x" {
				want = tails[order[p]]
				break
			}
		}
		if last := order[len(order)-1]; items[last].Text != "x" {
			want = want.Add(tails[last])
		}
		var r Rightmost
		for i := range items {
			r.Place(items[i], tails[i])
		}
		if got := r.Tail(); got != want {
			texts := make([]string, len(items))
			for i := range items {
				texts[i] = items[i].Text
			}
			t.Fatalf("levels %v, texts %q, tails %v: the tracker hangs %d and the "+
				"reordering %d", levels, texts, tails, got, want)
		}
	}
}
