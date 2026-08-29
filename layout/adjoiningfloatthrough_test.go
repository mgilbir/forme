package layout

import "testing"

// §9.5.2's hypothetical position when the float the box clears would move with
// it, and the box's own margins collapse through.
//
// The rule the engine implements is the suite's own, from
// adjoining-float-before-clearance: "if the clearance candidate would pull a
// float down with it (due to margin collapsing) if there were no clearance,
// clearance needs to be inserted to separate the two [...] No matter how large
// the margin is, it should still be just below the float." Against such a float
// the margin buys no distance, so the hypothetical position is measured without
// it and clearance is what places the box.
//
// A box whose own margins collapse through was exempted from that, on the
// strength of §9.5.2's parenthesis — "including the case where the element's
// margins collapse through, in which case its bottom margin is also included".
// The parenthesis is about which margins are in the run, not about whether a
// margin that moves the float counts, and the two boxes are in the same
// position: the margin moves the float either way.

// TestAnEmptyClearedBoxDoesNotCarryTheFloatItClears is the bug, in
// CSS2/floats-clear/negative-clearance-after-adjoining-float's arrangement: a
// float and an empty cleared box with a margin far larger than the float is
// tall, both inside a parent whose top edge is open.
func TestAnEmptyClearedBoxDoesNotCarryTheFloatItClears(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p" style="width:100px">
  <div id="f" style="float:left; width:100px; height:50px"></div>
  <div id="c" style="clear:left; margin-top:200px"></div>
</div>
<div id="g" style="width:100px; height:50px"></div>`, "")
	p, f, c, g := find(t, root, "p"), find(t, root, "f"),
		find(t, root, "c"), find(t, root, "g")

	if f.BorderRect.Y != p.BorderRect.Y {
		t.Errorf("the float is at %v and its parent at %v; the cleared box's 200px "+
			"margin left through the parent's open top edge and took the float it "+
			"clears down with it", f.BorderRect.Y, p.BorderRect.Y)
	}
	// Clearance puts it just below the float, however large the margin is.
	if got, want := c.BorderRect.Y, f.BorderRect.Y.Add(picPx(50)); got != want {
		t.Errorf("the cleared box is at %v, want %v (the float's bottom)", got, want)
	}
	// And the clearance is a real separation, so the parent no longer collapses
	// through itself: what follows sits directly under the float.
	if got, want := g.BorderRect.Y, f.BorderRect.Y.Add(picPx(50)); got != want {
		t.Errorf("the block after the parent is at %v, want %v", got, want)
	}
	if got := p.BorderRect.H; got != picPx(50) {
		t.Errorf("the parent is %v tall, want 50px", got)
	}
}

// TestAMarginStillCountsAgainstAFloatItCannotMove is the containment argument,
// and the case the exemption was kept for: a float placed before the block whose
// open edge the margin leaves through does not move with that margin, so the
// margin really does carry the box past it and there is no clearance at all.
//
// Both halves have to hold at once. An engine that measured every hypothetical
// position without the margin would pass the test above and put this box on the
// float's bottom edge, a hundred pixels above where it belongs.
func TestAMarginStillCountsAgainstAFloatItCannotMove(t *testing.T) {
	root := layoutOf(t, 600, `<div id="o" style="overflow:hidden; width:100px">
  <div id="fl" style="float:left; width:50px; height:50px"></div>
  <div id="p" style="width:100px">
    <div id="c" style="clear:left; margin-top:150px"></div>
  </div>
</div>`, "")
	o, fl, c := find(t, root, "o"), find(t, root, "fl"), find(t, root, "c")

	if fl.BorderRect.Y != o.BorderRect.Y {
		t.Fatalf("the float is at %v and the wrapper's content top at %v; the "+
			"fixture is about a float the margin cannot move and this one moved",
			fl.BorderRect.Y, o.BorderRect.Y)
	}
	if got, want := c.BorderRect.Y, o.BorderRect.Y.Add(picPx(150)); got != want {
		t.Errorf("the cleared box is at %v, want %v — its margin carries it well "+
			"below the float, so §9.5.2 gives it no clearance at all", got, want)
	}
	if got := o.BorderRect.H; got != picPx(150) {
		t.Errorf("the wrapper is %v tall, want 150px; the whole run belongs inside "+
			"it", got)
	}
}
