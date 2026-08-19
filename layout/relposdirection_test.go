package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §9.4.3's over-constrained horizontal offset, and which edge wins it.
//
// "left" and "right" are opposite descriptions of one displacement, so a box
// given both is over-constrained and one of them has to lose. The specification
// says which in as many words: "if the direction property of the containing
// block is ltr, the value of left wins and right becomes -left. If direction of
// the containing block is rtl, right wins and left is ignored."
//
// Only the first half was implemented, so a box in a right-to-left block moved
// the wrong way — out of its container rather than into it, by twice the
// distance the author wrote.
//
// The rule is about being over-constrained and nothing else. With one of the two
// auto there is nothing to contradict, and the other rule applies whichever
// direction the block runs in: "if only one is auto, its used value becomes the
// negation of the other". Reading the direction there as well is the mistake
// this change nearly made, and it is worth two of the three tests that moved.

// relOffset is how far a relatively positioned box moved from where the flow
// put it.
func relOffset(t *testing.T, containerCSS, boxCSS string) style.Unit {
	t.Helper()
	at := func(css string) style.Unit {
		got := fillsOf(paintOf(t,
			`<div id="c"><div id="b"></div></div>`,
			`#c { width: 200px; `+containerCSS+` }
			 #b { width: 50px; height: 20px; background: rgb(0,0,255); `+css+` }`), blue)
		if len(got) != 1 {
			t.Fatalf("%d fills for %q: %v", len(got), css, got)
		}
		return got[0].X
	}
	return at(boxCSS).Sub(at(""))
}

// TestRightWinsWhenTheContainingBlockRunsRightToLeft is the bug.
func TestRightWinsWhenTheContainingBlockRunsRightToLeft(t *testing.T) {
	const both = `position: relative; left: 30px; right: 30px`
	if got := relOffset(t, `direction: ltr`, both); got != bgpx(30) {
		t.Errorf("in a left-to-right block the box moved %v, want 30 — left wins", got)
	}
	if got := relOffset(t, `direction: rtl`, both); got != bgpx(-30) {
		t.Errorf("in a right-to-left block the box moved %v, want -30 — right wins "+
			"and moves the box away from the right edge", got)
	}
}

// TestOnlyOneOffsetIsNotOverConstrained is the half the fix nearly broke. With
// one of the two auto the box is not over-constrained, so the direction decides
// nothing and the missing offset is the negation of the given one.
func TestOnlyOneOffsetIsNotOverConstrained(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		if got := relOffset(t, `direction: `+dir,
			`position: relative; left: 30px`); got != bgpx(30) {
			t.Errorf("direction: %s, left alone: the box moved %v, want 30", dir, got)
		}
		if got := relOffset(t, `direction: `+dir,
			`position: relative; right: 30px`); got != bgpx(-30) {
			t.Errorf("direction: %s, right alone: the box moved %v, want -30", dir, got)
		}
	}
}

// TestTheVerticalAxisHasNoSuchRule. §9.4.3 gives the direction rule to the
// horizontal axis alone: "top" wins over "bottom" whatever the block's
// direction, because direction is about the inline axis and nothing else.
func TestTheVerticalAxisHasNoSuchRule(t *testing.T) {
	vertical := func(containerCSS, boxCSS string) style.Unit {
		at := func(css string) style.Unit {
			got := fillsOf(paintOf(t,
				`<div id="c"><div id="b"></div></div>`,
				`#c { width: 200px; height: 200px; `+containerCSS+` }
				 #b { width: 50px; height: 20px; background: rgb(0,0,255); `+css+` }`), blue)
			if len(got) != 1 {
				t.Fatalf("%d fills: %v", len(got), got)
			}
			return got[0].Y
		}
		return at(boxCSS).Sub(at(""))
	}
	const both = `position: relative; top: 30px; bottom: 30px`
	for _, dir := range []string{"ltr", "rtl"} {
		if got := vertical(`direction: `+dir, both); got != bgpx(30) {
			t.Errorf("direction: %s: the box moved %v vertically, want 30 — top wins "+
				"either way", dir, got)
		}
	}
}

// TestTheDirectionReadIsTheContainingBlocksOwn.
//
// The property is inherited, so the two agree unless the box declares one of its
// own — and a box that does must not thereby decide which of its own offsets to
// obey. An inline box between them is passed over for the same reason: it is not
// a containing block, and "direction: rtl" on a <span> orders what is inside it
// rather than moving a positioned box out of it.
func TestTheDirectionReadIsTheContainingBlocksOwn(t *testing.T) {
	const both = `position: relative; left: 30px; right: 30px`
	// The box says ltr and its block says rtl: the block decides, so right wins.
	if got := relOffset(t, `direction: rtl`, both+`; direction: ltr`); got != bgpx(-30) {
		t.Errorf("the box moved %v; it declared ltr and the block it is in is rtl, "+
			"and §9.4.3 asks the block", got)
	}
	// And the other way round.
	if got := relOffset(t, `direction: ltr`, both+`; direction: rtl`); got != bgpx(30) {
		t.Errorf("the box moved %v; it declared rtl and the block it is in is ltr", got)
	}
	// An inline between them changes nothing, because it is not a block. The
	// box under test is inline too: a *block* inside a span is lifted out of it
	// by §9.2.1.1, so the span would never be in its ancestry and the fixture
	// would prove nothing.
	inline := func(css string) style.Unit {
		got := fillsOf(paintOf(t,
			`<div id="c"><span id="s"><span id="b">x</span></span></div>`,
			`#c { width: 200px; direction: rtl; font-size: 20px }
			 #s { direction: ltr }
			 #b { background: rgb(0,0,255); `+css+` }`), blue)
		if len(got) != 1 {
			t.Fatalf("%d fills for %q: %v", len(got), css, got)
		}
		return got[0].X
	}
	if moved := inline(`position: relative; left: 30px; right: 30px`).Sub(inline(``)); moved != bgpx(-30) {
		t.Errorf("an inline box moved %v through an ltr span in an rtl block, want "+
			"-30 — a span is not a containing block", moved)
	}
}
