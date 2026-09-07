package layout

import "github.com/mgilbir/forme/style"

// The ordered multiset of float bottoms.
//
// One question is asked of it — the smallest float bottom strictly below a given
// y, which is how the placement search advances — and it is built by inserting a
// float's bottom as the float is placed and unbuilt in exactly the reverse order
// when a subtree's floats are discarded and placed again. See floatIndex.rewind.
//
// # Why it is not a sorted slice
//
// It was one, and insertion into a sorted slice is a binary search followed by a
// memmove of everything after the insertion point. That is Θ(n) per float and
// Θ(n²) over a page, and which page you get depends entirely on the *order* the
// bottoms arrive in. Floats appended at the end cost nothing, so a column of
// floats stacked down the page — the common case, and the case the earlier
// measurements were taken on — hid it completely. Floats whose bottoms arrive in
// decreasing order put every one of them at the front, and a row of floats of
// decreasing height is a paragraph of images.
//
// Measured, placing n left floats side by side with decreasing heights against
// the same floats with increasing heights:
//
//	n          decreasing    increasing
//	32,000        0.11 s        0.009 s
//	64,000        0.44 s        0.014 s
//	128,000       4.8 s         0.035 s
//	256,000      26.5 s         0.092 s
//
// Four times the time for twice the floats, and 256,000 floats is under four
// megabytes of markup. A document is untrusted input, so 26 seconds of CPU for a
// page that renders as a stack of grey rectangles is the same finding the rest
// of this file exists to answer — it had simply moved from the queries into the
// list they are answered from.
//
// # Why an AVL tree and not something shorter
//
// A skip list or a treap is fewer lines and needs a source of randomness, which
// nothing else in layout has: the shape would then vary run to run, and a fixed
// seed would let a document that knows the seed choose values that degenerate
// the tree. Height balancing is decided by the tree and not by the input, so a
// page cannot ask for its worst case.
//
// Amortised structures were the other candidate — a sorted slice rebuilt every
// √n insertions, which is Θ(√n) per operation and about a hundred times better
// than the memmove at these sizes. It is still Θ(n^1.5) over a page, which for
// untrusted input is a smaller version of the same problem rather than an
// answer to it.
type bottomSet struct {
	root *bottomNode
	n    int
}

// bottomNode is one value. Duplicates are separate nodes: two floats can end at
// the same y, and removing one must leave the other.
type bottomNode struct {
	value       style.Unit
	height      int8
	left, right *bottomNode
}

// len is how many bottoms are in the set.
func (s *bottomSet) len() int { return s.n }

// insert adds one bottom.
func (s *bottomSet) insert(v style.Unit) {
	s.root = insertBottom(s.root, v)
	s.n++
}

// remove takes out one bottom equal to v, and reports whether it found one.
//
// One occurrence, not all of them: the set is a multiset because the values are
// float bottoms and two floats can end at the same y, and rewind takes out
// exactly the float it is undoing.
func (s *bottomSet) remove(v style.Unit) bool {
	var removed bool
	s.root, removed = removeBottom(s.root, v)
	if removed {
		s.n--
	}
	return removed
}

// firstAbove is the smallest bottom strictly greater than y.
//
// The descent keeps the best candidate seen: at a node above y, that node is a
// candidate and anything better is to its left; at a node at or below y,
// nothing in its left subtree can help.
func (s *bottomSet) firstAbove(y style.Unit) (style.Unit, bool) {
	var best style.Unit
	found := false
	for t := s.root; t != nil; {
		if t.value > y {
			best, found = t.value, true
			t = t.left
		} else {
			t = t.right
		}
	}
	return best, found
}

// values returns the set in order, for the tests that compare it against a
// sorted slice.
func (s *bottomSet) values() []style.Unit {
	out := make([]style.Unit, 0, s.n)
	var walk func(*bottomNode)
	walk = func(t *bottomNode) {
		if t == nil {
			return
		}
		walk(t.left)
		out = append(out, t.value)
		walk(t.right)
	}
	walk(s.root)
	return out
}

func heightOf(t *bottomNode) int8 {
	if t == nil {
		return 0
	}
	return t.height
}

func fixHeight(t *bottomNode) {
	l, r := heightOf(t.left), heightOf(t.right)
	if l > r {
		t.height = l + 1
	} else {
		t.height = r + 1
	}
}

func balanceOf(t *bottomNode) int8 { return heightOf(t.left) - heightOf(t.right) }

func rotateRight(t *bottomNode) *bottomNode {
	l := t.left
	t.left = l.right
	l.right = t
	fixHeight(t)
	fixHeight(l)
	return l
}

func rotateLeft(t *bottomNode) *bottomNode {
	r := t.right
	t.right = r.left
	r.left = t
	fixHeight(t)
	fixHeight(r)
	return r
}

// rebalance restores the AVL invariant at one node after its subtrees changed.
func rebalance(t *bottomNode) *bottomNode {
	fixHeight(t)
	switch b := balanceOf(t); {
	case b > 1:
		if balanceOf(t.left) < 0 {
			t.left = rotateLeft(t.left)
		}
		return rotateRight(t)
	case b < -1:
		if balanceOf(t.right) > 0 {
			t.right = rotateRight(t.right)
		}
		return rotateLeft(t)
	}
	return t
}

// insertBottom adds v, sending a duplicate right so that equal values keep the
// order they arrived in — which nothing depends on, but which makes the tree's
// contents a function of the input alone.
func insertBottom(t *bottomNode, v style.Unit) *bottomNode {
	if t == nil {
		return &bottomNode{value: v, height: 1}
	}
	if v < t.value {
		t.left = insertBottom(t.left, v)
	} else {
		t.right = insertBottom(t.right, v)
	}
	return rebalance(t)
}

// removeBottom takes out one node holding v.
func removeBottom(t *bottomNode, v style.Unit) (*bottomNode, bool) {
	if t == nil {
		return nil, false
	}
	var removed bool
	switch {
	case v < t.value:
		t.left, removed = removeBottom(t.left, v)
	case v > t.value:
		t.right, removed = removeBottom(t.right, v)
	default:
		removed = true
		switch {
		case t.left == nil:
			return t.right, true
		case t.right == nil:
			return t.left, true
		}
		// Two children: the in-order successor takes this node's place, and is
		// then removed from where it was. It is the smallest value in the right
		// subtree, so exactly one node there holds it and this recursion ends.
		next := t.right
		for next.left != nil {
			next = next.left
		}
		t.value = next.value
		t.right, _ = removeBottom(t.right, next.value)
	}
	if !removed {
		return t, false
	}
	return rebalance(t), true
}
