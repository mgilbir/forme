package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §9.5.2's clearance is measured against the floats a box has to clear, and a
// float inside the box is not one of them.
//
// The two rules meet awkwardly. Clearance is settled *after* the box's subtree
// has been laid out, because until then its own top margin is not known — and by
// then the floats that subtree added are in the formatting context too. The
// query saw them, so a box with "clear: left" holding one float cleared the
// float outside it, put its own float in at that position, and then cleared that
// one as well.
//
// The answer came out at exactly twice the clearance, which is the shape of a
// feedback loop rather than of an off-by-one: the box moved down by the float it
// was clearing, and then again by its own float that had moved with it. Eleven
// of the suite's tests are that loop, and the ones that name it name it well —
// "float separated from float inside empty cleared block".
//
// The bound is the mark taken before the subtree was laid out, and the float
// index is a prefix summary, so it costs a subscript rather than a scan.

// clearedAt is where a cleared block and the float inside it end up, given a
// float above them.
func clearedAt(t *testing.T, inner string) (block, float style.Unit) {
	t.Helper()
	ops := paintOf(t,
		`<div id="above"></div><div id="c">`+inner+`</div>`,
		`#above { float: left; width: 100px; height: 50px; background: rgb(0,0,255) }
		 #c { clear: left; height: 10px; background: rgb(255,0,0) }
		 #inner { float: left; width: 100px; height: 50px; background: rgb(0,128,0) }`)
	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(red) != 1 {
		t.Fatalf("%d cleared blocks: %v", len(red), red)
	}
	if inner == "" {
		return red[0].Y, 0
	}
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d inner floats: %v", len(got), got)
	}
	return red[0].Y, got[0].Y
}

// TestABoxDoesNotClearTheFloatInsideItself is the bug, stated as the equality it
// is: a float inside the box changes nothing about where the box goes.
func TestABoxDoesNotClearTheFloatInsideItself(t *testing.T) {
	empty, _ := clearedAt(t, "")
	held, inner := clearedAt(t, `<div id="inner"></div>`)
	if held != empty {
		t.Errorf("a cleared block holding a float is at %v and an empty one at %v; "+
			"a float inside a box is not a float that box clears", held, empty)
	}
	if inner != held {
		t.Errorf("the float inside is at %v and its block at %v; a float at the top "+
			"of a block starts where the block does", inner, held)
	}
}

// TestTheClearanceIsTheFloatsBottomAndNotTwiceIt, as a number, so that a change
// making both fixtures wrong in the same way is still caught.
func TestTheClearanceIsTheFloatsBottomAndNotTwiceIt(t *testing.T) {
	block, inner := clearedAt(t, `<div id="inner"></div>`)
	// The float above runs from the body's content top for 50px, so a block
	// clearing it starts 50 below — and the body's own margin is in both.
	plain := fillsOf(paintOf(t, `<div id="c"></div>`,
		`#c { height: 10px; background: rgb(255,0,0) }`), style.RGBA{R: 255, A: 1})
	if len(plain) != 1 {
		t.Fatalf("%d blocks in the control", len(plain))
	}
	if got := block.Sub(plain[0].Y); got != bgpx(50) {
		t.Errorf("the block cleared by %v, want 50 — the float's height, once", got)
	}
	if inner != block {
		t.Errorf("the float inside is at %v and its block at %v", inner, block)
	}
}

// TestAFloatOutsideTheBoxIsStillCleared is the containment case, and it is what
// clearance is for: the bound is on which floats are counted, not on whether any
// are.
func TestAFloatOutsideTheBoxIsStillCleared(t *testing.T) {
	block, _ := clearedAt(t, "")
	plain := fillsOf(paintOf(t, `<div id="c"></div>`,
		`#c { height: 10px; background: rgb(255,0,0) }`), style.RGBA{R: 255, A: 1})
	if got := block.Sub(plain[0].Y); got != bgpx(50) {
		t.Errorf("a cleared block moved %v past a 50px float, want 50", got)
	}
	// And with no float at all it does not move: clearance is a difference.
	none := fillsOf(paintOf(t, `<div id="c"></div>`,
		`#c { clear: left; height: 10px; background: rgb(255,0,0) }`), style.RGBA{R: 255, A: 1})
	if len(none) != 1 || none[0].Y != plain[0].Y {
		t.Errorf("a cleared block with nothing to clear is at %v and an uncleared "+
			"one at %v", none, plain)
	}
}

// TestASecondCleatedBlockClearsTheFirstsFloat. The bound is on the box's *own*
// floats and not on everything below the mark: a float added by an earlier
// sibling is still one this box clears, which is the case that would break if
// the bound were taken once for the whole parent rather than once per child.
func TestASecondClearedBlockClearsTheFirstsFloat(t *testing.T) {
	ops := paintOf(t,
		`<div id="one"><div class="f"></div></div><div id="two"></div>`,
		`#one { clear: left }
		 .f { float: left; width: 100px; height: 50px; background: rgb(0,128,0) }
		 #two { clear: left; height: 10px; background: rgb(255,0,0) }`)
	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	got := fillsOf(ops, green)
	if len(red) != 1 || len(got) != 1 {
		t.Fatalf("%d blocks and %d floats", len(red), len(got))
	}
	if want := got[0].Y.Add(got[0].H); red[0].Y != want {
		t.Errorf("the second cleared block is at %v and the first's float ends at "+
			"%v; a float an earlier sibling added is one this box clears",
			red[0].Y, want)
	}
}
