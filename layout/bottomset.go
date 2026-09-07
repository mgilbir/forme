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
// floatBottom is one float's bottom edge, which is its own key.
type floatBottom style.Unit

func (b floatBottom) unitKey() style.Unit { return style.Unit(b) }

// bottomSet is the multiset, over the tree in unittree.go.
type bottomSet struct {
	tree unitTree[floatBottom]
}

// len is how many bottoms are in the set.
func (s *bottomSet) len() int { return s.tree.len() }

// insert adds one bottom.
func (s *bottomSet) insert(v style.Unit) { s.tree.insert(floatBottom(v)) }

// remove takes out one bottom equal to v, and reports whether it found one.
//
// One occurrence, not all of them: the set is a multiset because the values are
// float bottoms and two floats can end at the same y, and rewind takes out
// exactly the float it is undoing.
func (s *bottomSet) remove(v style.Unit) bool { return s.tree.removeKey(v) }

// firstAbove is the smallest bottom strictly greater than y.
func (s *bottomSet) firstAbove(y style.Unit) (style.Unit, bool) {
	b, ok := s.tree.firstAbove(y)
	return style.Unit(b), ok
}

// values returns the set in order, for the tests that compare it against a
// sorted slice.
func (s *bottomSet) values() []style.Unit {
	items := s.tree.all()
	out := make([]style.Unit, len(items))
	for i, b := range items {
		out[i] = style.Unit(b)
	}
	return out
}
