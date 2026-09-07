package layout

import (
	"math/bits"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/mgilbir/forme/style"
)

// The set of float bottoms, against the sorted slice it replaced.
//
// The slice is the oracle rather than the implementation now: it is obviously
// right and obviously too slow, which is the pair that makes a property test
// worth writing. See bottomset.go for the measurements.

// sortedBottoms is the previous implementation, kept here as the reference.
type sortedBottoms []style.Unit

func (s *sortedBottoms) insert(v style.Unit) {
	*s = append(*s, v)
	sort.Slice(*s, func(i, j int) bool { return (*s)[i] < (*s)[j] })
}

func (s *sortedBottoms) remove(v style.Unit) bool {
	for i, got := range *s {
		if got == v {
			*s = append((*s)[:i], (*s)[i+1:]...)
			return true
		}
	}
	return false
}

func (s sortedBottoms) firstAbove(y style.Unit) (style.Unit, bool) {
	for _, v := range s {
		if v > y {
			return v, true
		}
	}
	return 0, false
}

// TestTheBottomSetAgreesWithTheSortedSlice runs an arbitrary sequence of the
// three operations against both and compares after every one.
//
// The values are drawn from a small range on purpose: float bottoms repeat —
// a row of images of the same height gives one value n times — and a multiset
// that collapsed duplicates would answer the queries correctly and then remove
// the wrong number of them when a subtree was laid out again.
func TestTheBottomSetAgreesWithTheSortedSlice(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var got bottomSet
	var want sortedBottoms

	for step := 0; step < 20000; step++ {
		switch {
		case want.len() == 0 || rng.Intn(3) > 0:
			v := style.Unit(rng.Intn(40) - 10)
			got.insert(v)
			want.insert(v)
		default:
			// Removed in reverse order of insertion, which is how rewind uses
			// it, but by *value*: two floats ending at the same y put two equal
			// entries in, and taking either one out has to leave the other.
			v := want[rng.Intn(len(want))]
			if got.remove(v) != want.remove(v) {
				t.Fatalf("step %d: the two disagreed about removing %v", step, v)
			}
		}
		if got.len() != want.len() {
			t.Fatalf("step %d: the set holds %d and the slice holds %d",
				step, got.len(), want.len())
		}
		for y := style.Unit(-12); y <= 42; y++ {
			gv, gok := got.firstAbove(y)
			wv, wok := want.firstAbove(y)
			if gv != wv || gok != wok {
				t.Fatalf("step %d: the first bottom above %v is (%v, %v) and the "+
					"slice says (%v, %v)", step, y, gv, gok, wv, wok)
			}
		}
	}
}

func (s sortedBottoms) len() int { return len(s) }

// TestTheBottomSetStaysBalanced is what makes the operations logarithmic, and it
// is asserted rather than assumed: the whole reason for the tree is that the
// slice's cost depended on the order the values arrived in, and a tree that
// stopped rebalancing would depend on it in exactly the same way.
//
// The orders below are the ones a page produces. Ascending is a column of
// floats stacked down the page; equal is a row of images of one height;
// descending is a row of decreasing heights; and the shuffled one is there so
// that a balance rule tuned to the other three has nothing to hide behind.
func TestTheBottomSetStaysBalanced(t *testing.T) {
	const n = 1 << 14
	for _, c := range []struct {
		name  string
		value func(i int) style.Unit
	}{
		{"ascending", func(i int) style.Unit { return style.Unit(i) }},
		{"descending", func(i int) style.Unit { return style.Unit(n - i) }},
		{"all equal", func(int) style.Unit { return 42 }},
		{"shuffled", func(i int) style.Unit { return style.Unit(i * 2654435761 % n) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			var s bottomSet
			for i := 0; i < n; i++ {
				s.insert(c.value(i))
			}
			// AVL's bound: a tree of n nodes is at most 1.4405·log2(n+2) − 0.328
			// deep. Rounded up, and stated as the bound rather than as the
			// number this tree happens to reach, so that a different but still
			// balanced insertion does not have to be argued about here.
			limit := int8(1.4405*float64(bits.Len(uint(n+2))) + 1)
			if h := heightOf(s.root); h > limit {
				t.Errorf("%d values inserted in %s order gave a tree %d deep, "+
					"and AVL's bound is %d", n, c.name, h, limit)
			}
			if bad := unbalanced(s.root); bad != nil {
				t.Errorf("a node holding %v has subtrees %d and %d deep",
					bad.value, heightOf(bad.left), heightOf(bad.right))
			}
			// And it still holds what was put in, in order.
			vals := s.values()
			if len(vals) != n {
				t.Fatalf("the tree holds %d values, want %d", len(vals), n)
			}
			for i := 1; i < len(vals); i++ {
				if vals[i] < vals[i-1] {
					t.Fatalf("the walk is not in order at %d: %v then %v",
						i, vals[i-1], vals[i])
				}
			}
		})
	}
}

// unbalanced returns a node whose subtrees differ in depth by more than one, or
// whose recorded height is not what its subtrees say.
func unbalanced(t *bottomNode) *bottomNode {
	if t == nil {
		return nil
	}
	if bad := unbalanced(t.left); bad != nil {
		return bad
	}
	if bad := unbalanced(t.right); bad != nil {
		return bad
	}
	b := balanceOf(t)
	if b < -1 || b > 1 {
		return t
	}
	l, r := heightOf(t.left), heightOf(t.right)
	if l < r {
		l = r
	}
	if t.height != l+1 {
		return t
	}
	return nil
}

// TestARowOfEqualFloatsIsLinearInTheirNumber is the shape that made this worth
// changing, and it is the most ordinary one there is: a row of images all the
// same height.
//
// Every one of them ends at the same y, so the staircases stay two steps long
// and the whole cost is the list of bottoms. Inserting into a sorted slice put
// each new value at the front of the run of equal ones and memmoved the rest,
// which is Θ(n) a float and Θ(n²) a page:
//
//	n          sorted slice      tree
//	32,000        0.035 s       0.017 s
//	64,000        0.113 s       0.031 s
//	128,000       0.414 s       0.047 s
//	256,000       1.74 s        0.109 s
//
// Four times the time for twice the floats, three doublings running, against a
// little over two.
//
// The bound is wall-clock, which the file next door explains is not an
// assertion to reach for lightly; it is the right one here for the same reason
// it is there — the quantity under test is time — and the margin is what makes
// it safe.
//
// It is set for the race detector, because `make race` runs this suite under it
// and the detector is what decides the number: the loop takes 0.11 s plainly
// and 1.9 s instrumented on a CI runner, and the slice takes 1.74 s plainly and
// 18.3 s instrumented. So the bound is four times the honest instrumented cost
// and less than half the instrumented quadratic — and forty times the honest
// cost when the detector is off, where the quadratic is still over the line.
func TestARowOfEqualFloatsIsLinearInTheirNumber(t *testing.T) {
	const floats = 256000

	fc := &floatContext{}
	lo, hi := style.Unit(0), style.Unit(1<<29)
	start := time.Now()
	for i := 0; i < floats; i++ {
		r := fc.place(Size{W: 3 * 64, H: 100 * 64}, FloatLeft, 0, lo, hi)
		// The query the placement search makes, so that the list is read as
		// well as written.
		fc.nextBottomBelow(r.Y)
	}
	elapsed := time.Since(start)

	if n := fc.idx.bottoms.len(); n != floats {
		t.Fatalf("the index holds %d bottoms, want %d", n, floats)
	}
	// They really did go side by side: a run that stacked them would have a
	// different list and would not be measuring this.
	if steps := len(fc.idx.left.steps); steps > 4 {
		t.Fatalf("the left staircase has %d steps; the floats are all the same "+
			"height, so this is not the row this test is about", steps)
	}
	if elapsed > 8*time.Second {
		t.Errorf("placing %d floats of one height took %v; the sorted list this "+
			"replaced took 1.74 s and the tree takes about 0.11 s — 18.3 s and "+
			"1.9 s under the race detector, which is what the bound allows for",
			floats, elapsed)
	}
}
