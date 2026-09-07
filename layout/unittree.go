package layout

import "github.com/mgilbir/forme/style"

// An ordered collection keyed by a coordinate, for the two places in this file's
// neighbourhood where a page can otherwise make a memmove quadratic.
//
// Both are sorted lists that a float is spliced into as it is placed and out of
// again when a subtree is laid out a second time: the staircase's breakpoints,
// and the list of float bottoms the placement search steps down. A slice
// answers every query correctly and pays Θ(n) for every edit, so the cost of a
// page depends on the order its floats arrive in — see the measurements over
// bottomSet and stair.
//
// # Why AVL, and why one of them
//
// AVL because the balance is decided by the tree and not by the input: a skip
// list or a treap is fewer lines and needs randomness, which nothing else in
// layout has, and a fixed seed would let a document that knows it choose values
// that degenerate the tree. Untrusted input is the whole reason any of this is
// here.
//
// One of them because two AVL trees in one package is two sets of rotations to
// get right and one place to fix a mistake in. What the two users differ in is
// the item, and an item is anything that can say which coordinate it sits at.
type unitKeyed interface {
	// unitKey is where this item sits. Two items may share a key: two floats
	// can end at the same y, and both entries have to be there so that removing
	// one leaves the other.
	unitKey() style.Unit
}

// unitTree is the collection. The zero value is empty and ready.
type unitTree[T unitKeyed] struct {
	root *unitNode[T]
	n    int
}

type unitNode[T unitKeyed] struct {
	item        T
	height      int8
	left, right *unitNode[T]
}

// len is how many items the tree holds.
func (t *unitTree[T]) len() int { return t.n }

// insert adds an item. An item whose key is already there is added beside it
// rather than replacing it.
func (t *unitTree[T]) insert(item T) {
	t.root = unitInsert(t.root, item)
	t.n++
}

// removeKey takes out one item with the given key, and reports whether it found
// one.
func (t *unitTree[T]) removeKey(k style.Unit) bool {
	var removed bool
	t.root, removed = unitRemove(t.root, k)
	if removed {
		t.n--
	}
	return removed
}

// firstAbove is the leftmost item whose key is strictly greater than k.
//
// The descent keeps the best candidate seen: at an item above k, that item is a
// candidate and anything better is to its left; at an item at or below k,
// nothing in its left subtree can help.
func (t *unitTree[T]) firstAbove(k style.Unit) (item T, ok bool) {
	for n := t.root; n != nil; {
		if n.item.unitKey() > k {
			item, ok = n.item, true
			n = n.left
		} else {
			n = n.right
		}
	}
	return item, ok
}

// firstAtOrAbove is the leftmost item whose key is at least k.
func (t *unitTree[T]) firstAtOrAbove(k style.Unit) (item T, ok bool) {
	for n := t.root; n != nil; {
		if n.item.unitKey() >= k {
			item, ok = n.item, true
			n = n.left
		} else {
			n = n.right
		}
	}
	return item, ok
}

// lastAtOrBelow is the rightmost item whose key is at most k.
func (t *unitTree[T]) lastAtOrBelow(k style.Unit) (item T, ok bool) {
	for n := t.root; n != nil; {
		if n.item.unitKey() <= k {
			item, ok = n.item, true
			n = n.right
		} else {
			n = n.left
		}
	}
	return item, ok
}

// lastBelow is the rightmost item whose key is strictly less than k.
func (t *unitTree[T]) lastBelow(k style.Unit) (item T, ok bool) {
	for n := t.root; n != nil; {
		if n.item.unitKey() < k {
			item, ok = n.item, true
			n = n.right
		} else {
			n = n.left
		}
	}
	return item, ok
}

// rangeFrom calls f with every item whose key is at least lo, in order, until f
// returns false.
//
// A walk rather than an iterator type: every caller here consumes the items as
// it goes, and a stack of nodes built on the spot is both shorter and does not
// have to say what happens if the tree changes under it. It does not.
func (t *unitTree[T]) rangeFrom(lo style.Unit, f func(T) bool) {
	// The path down to the first item at or above lo, so that the walk can
	// continue upwards from there without parent pointers.
	var stack []*unitNode[T]
	for n := t.root; n != nil; {
		if n.item.unitKey() >= lo {
			stack = append(stack, n)
			n = n.left
		} else {
			n = n.right
		}
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !f(n.item) {
			return
		}
		for m := n.right; m != nil; m = m.left {
			stack = append(stack, m)
		}
	}
}

// all returns every item in order, which is what the tests compare against a
// sorted slice.
func (t *unitTree[T]) all() []T {
	out := make([]T, 0, t.n)
	t.rangeFrom(style.MinUnit, func(item T) bool {
		out = append(out, item)
		return true
	})
	return out
}

func unitHeight[T unitKeyed](n *unitNode[T]) int8 {
	if n == nil {
		return 0
	}
	return n.height
}

func unitFixHeight[T unitKeyed](n *unitNode[T]) {
	l, r := unitHeight(n.left), unitHeight(n.right)
	if l > r {
		n.height = l + 1
	} else {
		n.height = r + 1
	}
}

func unitBalance[T unitKeyed](n *unitNode[T]) int8 {
	return unitHeight(n.left) - unitHeight(n.right)
}

func unitRotateRight[T unitKeyed](n *unitNode[T]) *unitNode[T] {
	l := n.left
	n.left = l.right
	l.right = n
	unitFixHeight(n)
	unitFixHeight(l)
	return l
}

func unitRotateLeft[T unitKeyed](n *unitNode[T]) *unitNode[T] {
	r := n.right
	n.right = r.left
	r.left = n
	unitFixHeight(n)
	unitFixHeight(r)
	return r
}

// unitRebalance restores the AVL invariant at one node after its subtrees
// changed.
func unitRebalance[T unitKeyed](n *unitNode[T]) *unitNode[T] {
	unitFixHeight(n)
	switch b := unitBalance(n); {
	case b > 1:
		if unitBalance(n.left) < 0 {
			n.left = unitRotateLeft(n.left)
		}
		return unitRotateRight(n)
	case b < -1:
		if unitBalance(n.right) > 0 {
			n.right = unitRotateRight(n.right)
		}
		return unitRotateLeft(n)
	}
	return n
}

// unitInsert adds an item, sending an equal key right so that items sharing one
// keep the order they arrived in — which nothing depends on, but which makes the
// tree's contents a function of the input alone.
func unitInsert[T unitKeyed](n *unitNode[T], item T) *unitNode[T] {
	if n == nil {
		return &unitNode[T]{item: item, height: 1}
	}
	if item.unitKey() < n.item.unitKey() {
		n.left = unitInsert(n.left, item)
	} else {
		n.right = unitInsert(n.right, item)
	}
	return unitRebalance(n)
}

// unitRemove takes out one node holding k.
func unitRemove[T unitKeyed](n *unitNode[T], k style.Unit) (*unitNode[T], bool) {
	if n == nil {
		return nil, false
	}
	var removed bool
	switch {
	case k < n.item.unitKey():
		n.left, removed = unitRemove(n.left, k)
	case k > n.item.unitKey():
		n.right, removed = unitRemove(n.right, k)
	default:
		removed = true
		switch {
		case n.left == nil:
			return n.right, true
		case n.right == nil:
			return n.left, true
		}
		// Two children: the in-order successor takes this node's place and is
		// then removed from where it was. It is the smallest key in the right
		// subtree, so exactly one node there holds it and this recursion ends.
		next := n.right
		for next.left != nil {
			next = next.left
		}
		n.item = next.item
		n.right, _ = unitRemove(n.right, next.item.unitKey())
	}
	if !removed {
		return n, false
	}
	return unitRebalance(n), true
}
