package layout

import "testing"

// A float whose containing block sits above the top of the formatting context
// it is placed in.
//
// A negative top margin puts it there, and nothing in §9.5.1 says a float may
// not go with it: rule 5 is about the top of the float's own containing block,
// and rule 6 about the floats it was told to clear. Neither is the content top
// of the formatting context root.
//
// floatChild asked the context for a clearance anyway. The answer for a box that
// clears nothing is zero — "the bottom of the lowest float on a cleared side,
// and zero when there is none" — and comparing zero against a negative position
// is a floor. The float left the block it belonged to, landed on the root's
// edge, and every box that then had to avoid it was measured against a float
// somewhere the page does not draw one.
//
// CSS2/floats/new-fc-separates-from-float-2 is the arrangement it shows up in:
// the float moves, so the block that has to drop below it drops to the wrong
// place, so the margin that should have been separated from the float escapes
// instead, and the wrapper lands twelve thousand pixels down the page.

// TestAFloatFollowsItsBlockAboveTheContextTop is the bug, reduced to the one
// fact: a float is at the top of the block it is in.
func TestAFloatFollowsItsBlockAboveTheContextTop(t *testing.T) {
	root := layoutOf(t, 600, `<div id="o" style="overflow:hidden; width:200px; height:200px">
  <div id="w" style="margin-top:-50px">
    <div id="e"><div id="f" style="float:left; width:200px; height:150px"></div></div>
  </div>
</div>`, "")
	o, e, f := find(t, root, "o"), find(t, root, "e"), find(t, root, "f")
	if got, want := e.BorderRect.Y, o.BorderRect.Y.Sub(picPx(50)); got != want {
		t.Fatalf("the block holding the float is at %v, want %v — the fixture is "+
			"about a block above the context top and this one is not", got, want)
	}
	if got := f.BorderRect.Y; got != e.BorderRect.Y {
		t.Errorf("the float is at %v and the block it is in is at %v; a float that "+
			"clears nothing has nothing to be pushed down by", got, e.BorderRect.Y)
	}
	if f.BorderRect.Y == o.BorderRect.Y {
		t.Errorf("the float is exactly on the formatting context root's content " +
			"top, which is the floor this is about")
	}
}

// TestTheDropBelowTheFloatFollowsItToo is the consequence, and the half the
// reftest actually draws: a box that may not overlap the float is placed against
// where the float is.
func TestTheDropBelowTheFloatFollowsItToo(t *testing.T) {
	root := layoutOf(t, 600, `<div id="o" style="overflow:hidden; width:200px; height:200px">
  <div id="w" style="margin-top:-50px">
    <div id="e"><div id="f" style="float:left; width:200px; height:150px"></div></div>
    <div id="b" style="margin-top:12345px; overflow:hidden; width:200px; height:100px"></div>
  </div>
</div>`, "")
	f, b := find(t, root, "f"), find(t, root, "b")
	// The float is 200px wide in a 200px block, so nothing 200px wide fits
	// beside it and a box that establishes a formatting context goes below.
	if got, want := b.BorderRect.Y, f.BorderRect.Y.Add(picPx(150)); got != want {
		t.Errorf("the box below the float is at %v, want %v (the float's bottom)",
			got, want)
	}
	// And the whole point of the arrangement: its enormous top margin is
	// separated from the float rather than collapsing out of the wrapper and
	// carrying the float down with it.
	o := find(t, root, "o")
	if got, want := f.BorderRect.Y, o.BorderRect.Y.Sub(picPx(50)); got != want {
		t.Errorf("the float is at %v, want %v; the margin below it escaped through "+
			"the wrapper's open top edge and took the float with it", got, want)
	}
}

// TestAFloatThatClearsStillClears is the containment argument. The query being
// skipped is the one that implements "clear" on a float, and a float that
// declares it must still go below the floats on that side.
func TestAFloatThatClearsStillClears(t *testing.T) {
	root := layoutOf(t, 600, `<div id="o" style="width:400px">
  <div id="a" style="float:left; width:100px; height:80px"></div>
  <div id="c" style="float:left; clear:left; width:100px; height:40px"></div>
</div>`, "")
	a, c := find(t, root, "a"), find(t, root, "c")
	if got, want := c.BorderRect.Y, a.BorderRect.Y.Add(picPx(80)); got != want {
		t.Errorf("the cleared float is at %v, want %v (below the float it clears); "+
			"there is room beside it, so only \"clear\" puts it there", got, want)
	}
}

// TestAFloatThatClearsIsStillClearedAboveTheContextTop: the two halves together.
// A float above the context top that *does* clear something is still put below
// it, so the guard is on what the box asked for and not on where it is.
func TestAFloatThatClearsIsStillClearedAboveTheContextTop(t *testing.T) {
	root := layoutOf(t, 600, `<div id="o" style="overflow:hidden; width:400px; height:300px">
  <div id="w" style="margin-top:-50px">
    <div id="a" style="float:left; width:100px; height:80px"></div>
    <div id="c" style="float:left; clear:left; width:100px; height:40px"></div>
  </div>
</div>`, "")
	a, c := find(t, root, "a"), find(t, root, "c")
	if got, want := c.BorderRect.Y, a.BorderRect.Y.Add(picPx(80)); got != want {
		t.Errorf("the cleared float is at %v, want %v", got, want)
	}
}
