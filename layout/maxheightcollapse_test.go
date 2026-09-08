package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §8.3.1's bottom margin, and the two limits that stop it collapsing out.
//
// Two margins are adjoining only where nothing separates them, and what has to
// be true for a box and its last in-flow child is that the box's bottom edge and
// the child's bottom margin edge are the *same* edge. A min-height that bound
// puts the box's edge below the child's; a max-height that bound puts it above,
// with the child overflowing past it. Neither is one edge.
//
// Where the margin escapes a box the author capped, everything below the box
// moves down by a distance that came from inside a box whose height was fixed —
// which is the whole reason an author writes the cap.

// bottomOfCappedParent is where the box after a capped parent begins.
func afterCappedParent(t *testing.T, parentCSS string) style.Unit {
	t.Helper()
	root := layoutOf(t, 500,
		`<div id="p" class="cap"><div id="c"></div></div><div id="after"></div>`,
		noDefaults+`#p { `+parentCSS+` } #c { height: 51px; margin-bottom: 10px }
		 #after { height: 20px }`)
	return find(t, root, "after").BorderRect.Y
}

// TestAMaximumSeparatesTheBottomMargins.
//
// A fifty-pixel cap over a fifty-one-pixel child: the box is fifty tall and the
// child hangs out of it, so the child's ten pixels of bottom margin are inside
// an overflowing box rather than reaching the box's own edge, and the box after
// it begins at fifty.
func TestAMaximumSeparatesTheBottomMargins(t *testing.T) {
	px(t, "the box after a capped parent", afterCappedParent(t, "max-height: 50px"), 50)
}

// TestNoMaximumLetsTheBottomMarginsCollapse is the control, and it is what makes
// the test above about the cap rather than about margin collapsing at all: the
// same two boxes with nothing capping them put the child's margin outside the
// parent, so the box after begins ten pixels further down.
func TestNoMaximumLetsTheBottomMarginsCollapse(t *testing.T) {
	px(t, "the box after an uncapped parent", afterCappedParent(t, ""), 61)
}

// TestAMaximumThatDoesNotBindChangesNothing. Only a limit that actually bound
// separates the edges: where the content is shorter than the cap the box ends
// where its content does, the two edges meet, and the margin collapses out as it
// always did.
func TestAMaximumThatDoesNotBindChangesNothing(t *testing.T) {
	px(t, "the box after a parent capped above its content",
		afterCappedParent(t, "max-height: 500px"), 61)
}

// TestAMinimumStillSeparatesTheBottomMargins is the same rule read from the
// other side, and it is here so that widening it to maxima cannot quietly
// undo it.
func TestAMinimumStillSeparatesTheBottomMargins(t *testing.T) {
	px(t, "the box after a parent held open by a minimum",
		afterCappedParent(t, "min-height: 100px"), 100)
}
