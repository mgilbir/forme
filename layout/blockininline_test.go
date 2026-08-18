package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A block §9.2.1.1 broke an inline around, and what the inline still does to it.
//
// The rule reads as though the block leaves the inline entirely: the inline is
// broken in two, the block becomes a sibling of the halves, and nothing about
// the inline is above it in the box tree. Two things about the inline reach the
// block anyway, because they are about what the *element* contains rather than
// about the boxes:
//
//   - a relative offset moves everything the inline contains, and the block it
//     was broken around is still something it contains;
//   - a text-decoration is drawn across everything it contains, for the same
//     reason.
//
// Both were lost, and the second one silently: the block came out in the right
// place with no underline on it, which reads as a document that never asked for
// one.

// blockFill is the background of the one block in a document, which is where the
// block ended up.
func blockFill(t *testing.T, htmlSrc, cssSrc string, colour style.RGBA) Rect {
	t.Helper()
	got := fillsOf(paintOf(t, htmlSrc, cssSrc), colour)
	if len(got) != 1 {
		t.Fatalf("%d fills of %v, want 1: %v", len(got), colour, got)
	}
	return got[0]
}

// TestAnInlinesRelativeOffsetMovesTheBlockItWasBrokenAround.
//
// The suite writes it with the offset and a negative margin that cancel:
// "left: 2em" on the inline and "margin-left: -2em" on the block, which must
// land in the same place as a block with neither. Applying one and not the other
// put it two ems to the left — off the edge of its own container.
func TestAnInlinesRelativeOffsetMovesTheBlockItWasBrokenAround(t *testing.T) {
	moved := blockFill(t,
		`<div id="c">A<span id="s">B<div id="b">C</div>B</span>A</div>`,
		`#c { width: 200px; font-size: 20px }
		 #s { position: relative; left: 40px; display: inline }
		 #b { background: rgb(0,0,255); height: 10px; margin-left: -40px }`,
		blue)
	plain := blockFill(t,
		`<div id="c">A<span id="s">B<div id="b">C</div>B</span>A</div>`,
		`#c { width: 200px; font-size: 20px }
		 #s { display: inline }
		 #b { background: rgb(0,0,255); height: 10px }`,
		blue)
	if moved.X != plain.X {
		t.Errorf("the block is at %v and one with neither offset nor margin is at "+
			"%v; a 40px offset and a -40px margin cancel", moved.X, plain.X)
	}
}

// TestTheOffsetIsAppliedOnceIsNotEnough — it also has to be applied at all, and
// the equality above would hold if *both* were dropped. This pins the offset on
// its own.
func TestAnInlinesOffsetIsActuallyApplied(t *testing.T) {
	with := blockFill(t,
		`<div id="c">A<span id="s">B<div id="b">C</div>B</span>A</div>`,
		`#c { width: 200px; font-size: 20px }
		 #s { position: relative; left: 40px; display: inline }
		 #b { background: rgb(0,0,255); height: 10px }`,
		blue)
	without := blockFill(t,
		`<div id="c">A<span id="s">B<div id="b">C</div>B</span>A</div>`,
		`#c { width: 200px; font-size: 20px }
		 #s { display: inline }
		 #b { background: rgb(0,0,255); height: 10px }`,
		blue)
	if got := with.X.Sub(without.X); got != bgpx(40) {
		t.Errorf("the inline's 40px offset moved the block by %v", got)
	}
}

// TestNestedInlinesEachMoveTheBlock: the block is broken out of both, so both
// offsets apply.
func TestNestedInlinesEachMoveTheBlock(t *testing.T) {
	with := blockFill(t,
		`<div id="c">A<span id="o">B<span id="i">B<div id="b">C</div>B</span>B</span>A</div>`,
		`#c { width: 300px; font-size: 20px }
		 #o { position: relative; left: 10px; display: inline }
		 #i { position: relative; left: 20px; display: inline }
		 #b { background: rgb(0,0,255); height: 10px }`,
		blue)
	without := blockFill(t,
		`<div id="c">A<span id="o">B<span id="i">B<div id="b">C</div>B</span>B</span>A</div>`,
		`#c { width: 300px; font-size: 20px }
		 #b { background: rgb(0,0,255); height: 10px }`,
		blue)
	if got := with.X.Sub(without.X); got != bgpx(30) {
		t.Errorf("two nested offsets of 10 and 20 moved the block by %v, want 30", got)
	}
}

// TestAnInlinesDecorationReachesTheBlock is the silent half.
func TestAnInlinesDecorationReachesTheBlock(t *testing.T) {
	lines := func(html, css string) int {
		n := 0
		for _, op := range paintOf(t, html, css) {
			if r, ok := op.(FillRect); ok && r.Rect.H > 0 && r.Rect.H < bgpx(3) {
				n++
			}
		}
		return n
	}
	if got := lines(`<p><span id="s"><div>b</div></span></p>`,
		`#s { text-decoration: underline }`); got != 1 {
		t.Errorf("%d decoration lines for a block inside an underlined inline, want 1", got)
	}
	// The ordinary case, which must not have changed.
	if got := lines(`<p id="p">a<span>b</span></p>`,
		`#p { text-decoration: underline }`); got != 2 {
		t.Errorf("%d decoration lines for two runs under a decorated block, want 2", got)
	}
	// And a link, which is where a reader meets this: the UA sheet underlines
	// a:link, and a card built as <a><div>…</div></a> is the common shape.
	if got := lines(`<a href="#"><div>b</div></a>`, ``); got != 1 {
		t.Errorf("%d decoration lines for a block inside a link, want 1", got)
	}
}

// TestABlockNotBrokenOutOfAnythingIsUnchanged is the containment case: the new
// path is taken only by a block that really was lifted out of an inline.
func TestABlockNotBrokenOutOfAnythingIsUnchanged(t *testing.T) {
	with := blockFill(t,
		`<div id="c"><div id="b">C</div></div>`,
		`#c { width: 200px; position: relative; left: 40px }
		 #b { background: rgb(0,0,255); height: 10px }`, blue)
	// The offset belongs to the containing block, which moves the whole thing —
	// so the block moves with it exactly once.
	plain := blockFill(t,
		`<div id="c"><div id="b">C</div></div>`,
		`#c { width: 200px } #b { background: rgb(0,0,255); height: 10px }`, blue)
	if got := with.X.Sub(plain.X); got != bgpx(40) {
		t.Errorf("a relatively positioned parent moved its block child by %v, want 40", got)
	}
}
