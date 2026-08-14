package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// u is a whole number of pixels as the engine holds it.
func u(px float64) style.Unit {
	v, _ := style.FromPx(px)
	return v
}

// §10.8.1's aligned subtrees, stacked at the level the stacking happens at.
//
// These build the items directly rather than a document. A line's height is
// decided from three things — each item's extents, its vertical-align and the
// strut — and a document reaches them only through a font, so an assertion about
// the arithmetic written as HTML is an assertion about Courier as much as about
// the rule. Building the items says what the rule does and nothing else, and it
// can express a line no stylesheet conveniently produces.
//
// It is possible at all because the items no longer name a box: an atomic
// inline's extents are two lengths and a flag saying it has them, and what the
// item stands for is opaque. See itemRef.

// atomicOfExtents is an item that is a box on the line reaching the given
// distances from its own baseline. The ref only has to be non-nil — nothing in
// the stacking looks at it.
func atomicOfExtents(ascent, descent style.Unit, v VAlignState) Item {
	return Item{Atomic: new(int), Ascent: ascent, Descent: descent, Valign: v}
}

// TestOneAlignedSubtreesBoxesAreStackedTogether is the rule that the boxes in
// one "vertical-align: top" subtree are a single thing when the line is sized.
//
// §10.8.1 aligns the *subtree*, not each box in it: the top of the whole group
// goes against the top of the line box, so the room it needs is the deepest
// descent below the highest ascent, measured across every box in it. Gathering
// them separately asks the line for the tallest single box instead, which is a
// smaller number whenever one box reaches higher and another reaches lower.
//
// More than one box is needed, and so are their particular extents. A subtree
// of one box gathers identically either way, and so does a subtree whose tallest box is
// also its deepest — which is every subtree in a document made of pictures,
// because a replaced element is all ascent. That is why the engine ran with the
// grouping unguarded: the arithmetic only diverges for a subtree holding
// something with a descent, which is an inline-block whose baseline is its last
// line of text.
func TestOneAlignedSubtreesBoxesAreStackedTogether(t *testing.T) {
	// A strut of 20px with its baseline 16px down, so the line before any
	// subtree is considered is 20px tall with 4px below the baseline.
	s := Strut{Height: u(20), Baseline: u(16)}

	// One subtree, named by a token that is not a box and does not have to be.
	subtree := new(int)
	top := VAlignState{LineAlign: VAlignTop, Subtree: subtree}

	// Three boxes, and both the count and the order are the fixture rather than
	// arbitrary. The group keeps a running highest and a running deepest, and a
	// comparison that is never reached is a comparison no defect in it can be
	// seen through: each box after the first has to beat what stands on exactly
	// one of the two numbers.
	//
	// A small box to open the group, a picture that is all ascent to raise the
	// highest, and an inline-block hanging below its own baseline to lower the
	// deepest.
	items := []Item{
		atomicOfExtents(u(10), 0, top),
		atomicOfExtents(u(40), 0, top),
		atomicOfExtents(u(10), u(30), top),
	}

	ls := StackLine(items, s)

	// Gathered together: the group reaches 40 above its baseline and 30 below,
	// so it needs 70px and the line grows to it.
	if got := ls.Height.Px(); got != 70 {
		t.Errorf("a line holding one aligned subtree of three boxes is %gpx tall, want 70 "+
			"— the group's own 40px ascent over its own 30px descent. 40px means the "+
			"group kept one box's extents rather than the highest and the deepest "+
			"across all three", got)
	}
	if len(ls.groups) != 1 {
		t.Errorf("the three boxes made %d aligned subtrees, want 1 — they carry the same "+
			"subtree token, and telling them apart is what §10.8.1 is about", len(ls.groups))
	}
	if got := ls.groups[0].Ascent.Px(); got != 40 {
		t.Errorf("the subtree's ascent is %gpx, want 40 — the highest of its boxes", got)
	}
	if got := ls.groups[0].Descent.Px(); got != 30 {
		t.Errorf("the subtree's descent is %gpx, want 30 — the deepest of its boxes", got)
	}
}

// TestTwoAlignedSubtreesAreStackedApart is the same rule from the other side:
// boxes carrying *different* subtree tokens must not be merged.
//
// Without it the test above is satisfied by a "gather" that puts everything in
// one group and never compares anything, which would size a line holding a
// top-aligned box and a bottom-aligned one as though they were one subtree.
func TestTwoAlignedSubtreesAreStackedApart(t *testing.T) {
	s := Strut{Height: u(20), Baseline: u(16)}
	first := VAlignState{LineAlign: VAlignTop, Subtree: new(int)}
	second := VAlignState{LineAlign: VAlignTop, Subtree: new(int)}

	items := []Item{
		atomicOfExtents(u(40), 0, first),
		atomicOfExtents(u(10), u(30), second),
	}

	ls := StackLine(items, s)

	if len(ls.groups) != 2 {
		t.Fatalf("two boxes in different subtrees made %d groups, want 2", len(ls.groups))
	}
	// Each is sized alone, and the line takes the larger: 40px against 10+30.
	if got := ls.Height.Px(); got != 40 {
		t.Errorf("a line holding two separate aligned subtrees is %gpx tall, want 40 — "+
			"the taller of the two taken on its own. 70px means they were merged", got)
	}
}
