package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Where a box that collapses through sits.
//
// It has no height and draws nothing, so for most of a document the question
// never comes up. It comes up the moment something asks: a relatively positioned
// empty wrapper is a containing block, and an absolutely positioned box inside it
// is placed against its padding box.
//
// §8.3.1's rule is the one the ordinary path already followed. A margin that
// leaves through the parent's open top edge moves the *parent*, and the box stays
// where the flow had reached — adding the run to the box as well counts it twice.
// The collapse-through branch was written before that rule and did not have it,
// so an empty wrapper came out one whole collapsed margin below the content it
// was meant to cover, and the box inside it with it.

// throughAt is where an empty relatively positioned wrapper and the box after it
// end up, given the wrapper's margins.
func throughAt(t *testing.T, wrapper string) (inside, after style.Unit) {
	t.Helper()
	ops := paintOf(t, `<div id="r"><div id="a"></div></div><div id="s"></div>`,
		`#r { position: relative; `+wrapper+` }
		 #a { position: absolute; top: 0; left: 0; width: 20px; height: 20px;
		      background: rgb(0,128,0) }
		 #s { height: 20px; background: rgb(255,0,0) }`)
	g := fillsOf(ops, green)
	r := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(g) != 1 || len(r) != 1 {
		t.Fatalf("%d absolute and %d static boxes for %q", len(g), len(r), wrapper)
	}
	return g[0].Y, r[0].Y
}

// TestACollapsedThroughBoxStaysWhereTheFlowReached is the rule, and the suite's
// own fixture: the wrapper's padding top and the box after it are the same
// place, so a green square in the wrapper covers a red one after it exactly.
func TestACollapsedThroughBoxStaysWhereTheFlowReached(t *testing.T) {
	for _, tc := range []struct{ what, margins string }{
		{"a bottom margin", `margin: 0 0 100px 0`},
		{"a top margin", `margin: 100px 0 0 0`},
		{"both, the larger winning", `margin: 40px 0 100px 0`},
		{"no margin at all", `margin: 0`},
	} {
		inside, after := throughAt(t, tc.margins)
		if inside != after {
			t.Errorf("%s: the box inside the wrapper is at %v and the box after it "+
				"at %v; a box that collapses through has no height and the two are "+
				"the same place", tc.what, inside, after)
		}
	}
}

// TestTheMarginStillSeparatesWhatFollows is the containment half: the run is not
// being discarded, it is being spent once. A hundred pixels of margin still puts
// a hundred pixels between the top of the page and what comes after.
func TestTheMarginStillSeparatesWhatFollows(t *testing.T) {
	// Measured between two margins rather than against none, because the body's
	// own 8px top margin collapses into the run: against "margin: 0" the run is
	// the body's 8 and against 100px it is 100, so the difference is 92 and says
	// as much about the body as about the wrapper. Between 100 and 200 the body's
	// margin is in both and cancels.
	_, hundred := throughAt(t, `margin: 0 0 100px 0`)
	_, twoHundred := throughAt(t, `margin: 0 0 200px 0`)
	if got := twoHundred.Sub(hundred); got != bgpx(100) {
		t.Errorf("another 100px of collapsed margin moved what follows by %v, want "+
			"100 — the run is spent once and it is still spent", got)
	}
}

// TestABoxThatDoesNotCollapseThroughIsUnchanged. One pixel of height is enough
// to stop the collapse, and then the wrapper is where the flow reached and the
// margin goes below it — which is what the ordinary path always did and what
// this must not have changed.
func TestABoxThatDoesNotCollapseThroughIsUnchanged(t *testing.T) {
	ops := paintOf(t, `<div id="r"><div id="a"></div></div><div id="s"></div>`,
		`#r { position: relative; margin: 0 0 100px 0; height: 1px }
		 #a { position: absolute; top: 0; left: 0; width: 20px; height: 20px;
		      background: rgb(0,128,0) }
		 #s { height: 20px; background: rgb(255,0,0) }`)
	g := fillsOf(ops, green)
	r := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(g) != 1 || len(r) != 1 {
		t.Fatalf("%d and %d boxes", len(g), len(r))
	}
	if got := r[0].Y.Sub(g[0].Y); got != bgpx(101) {
		t.Errorf("the static box is %v below the wrapper's top, want 101 — one pixel "+
			"of height and a hundred of margin", got)
	}
}

// TestAClosedParentEdgeStillSeparatesInside is the other side of the condition.
// With something already committed inside the parent, the margin no longer
// escapes: it separates this box from what came before it, *inside* the parent,
// and the box does move.
func TestAClosedParentEdgeStillSeparatesInside(t *testing.T) {
	ops := paintOf(t,
		`<div id="p"><div id="first"></div><div id="r"><div id="a"></div></div>`+
			`<div id="s"></div></div>`,
		`#p { border-top: 1px solid rgb(0,0,255) }
		 #first { height: 10px }
		 #r { position: relative; margin: 0 0 100px 0 }
		 #a { position: absolute; top: 0; left: 0; width: 20px; height: 20px;
		      background: rgb(0,128,0) }
		 #s { height: 20px; background: rgb(255,0,0) }`)
	g := fillsOf(ops, green)
	r := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(g) != 1 || len(r) != 1 {
		t.Fatalf("%d and %d boxes", len(g), len(r))
	}
	if g[0].Y != r[0].Y {
		t.Errorf("the wrapper is at %v and what follows at %v; they are still the "+
			"same place, the margin having been spent above both", g[0].Y, r[0].Y)
	}
	// And the margin really was spent: the static box is 100 below the first
	// child rather than beside it.
	if got := r[0].Y.Sub(bgpx(8).Add(bgpx(1)).Add(bgpx(10))); got != bgpx(100) {
		t.Errorf("what follows is %v below the first child's bottom, want 100", got)
	}
}
