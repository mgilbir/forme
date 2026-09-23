package layout

import (
	"math/rand"
	"testing"

	"github.com/mgilbir/forme/style"
)

// keyedItem is an item whose key repeats and whose identity does not.
type keyedItem struct {
	key style.Unit
	id  int
}

func (k keyedItem) unitKey() style.Unit { return k.key }

// TestATreeWithRepeatedKeysLosesNoItem: removing a key takes out one item with
// that key and leaves every other item exactly once. Removing a node with two
// children moves its in-order successor into its place, and the successor used
// to be taken out again by its *key* — which, where keys repeat, can find
// another node first, losing that one's item and keeping the successor's
// twice (audit C140, unittree.go's comment claiming "exactly one node there
// holds it").
func TestATreeWithRepeatedKeysLosesNoItem(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		var tree unitTree[keyedItem]
		want := map[style.Unit]int{} // how many items each key holds
		next := 0
		for op := 0; op < 60; op++ {
			k := style.Unit(rng.Intn(4))
			if rng.Intn(3) > 0 {
				tree.insert(keyedItem{key: k, id: next})
				next++
				want[k]++
			} else if tree.removeKey(k) != (want[k] > 0) {
				t.Fatalf("round %d: removing key %d reported the wrong answer", round, k)
			} else if want[k] > 0 {
				want[k]--
			}
			seen := map[int]bool{}
			got := map[style.Unit]int{}
			var last style.Unit = -1
			for _, it := range tree.all() {
				if seen[it.id] {
					t.Fatalf("round %d op %d: item %d is in the tree twice: %v",
						round, op, it.id, tree.all())
				}
				seen[it.id] = true
				if it.key < last {
					t.Fatalf("round %d op %d: out of order: %v", round, op, tree.all())
				}
				last = it.key
				got[it.key]++
			}
			for k, n := range want {
				if got[k] != n {
					t.Fatalf("round %d op %d: key %d holds %d items, want %d",
						round, op, k, got[k], n)
				}
			}
		}
	}
}
