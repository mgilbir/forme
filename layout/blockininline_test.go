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

// A float written inside a relatively positioned inline.
//
// It is the same rule as the block above and reaches the page by a different
// road: a float is not lifted out of the inline by splitInline — it stays where
// it was written, because the formatting context it has to flow around is the
// one it was written in — but it *is* placed long after the walk that knew which
// inlines it was inside, and the offset was lost on the way.
//
// What must not move with it is the hole. §9.4.3's offset is applied after
// layout, so the band the float reserved stays where it was and the text around
// it still flows past that band. That is the difference between a relative
// offset and a margin, and it is why this is an offset on the fragment rather
// than a different position.
func TestAFloatInsideARelativelyPositionedInlineMovesWithIt(t *testing.T) {
	const doc = `<div id="c">x<span id="s"><div id="f"></div></span>yyyy</div>`
	const base = `#c { width: 300px; font-size: 20px }
		#f { float: left; width: 50px; height: 50px; background: rgb(0,0,255) }`

	moved := blockFill(t, doc, base+`
		#s { position: relative; left: 40px; display: inline }`, blue)
	plain := blockFill(t, doc, base+`#s { display: inline }`, blue)

	if got := moved.X.Sub(plain.X); got != bgpx(40) {
		t.Errorf("the inline's 40px offset moved the float by %v", got)
	}
	if moved.Y != plain.Y {
		t.Errorf("the float moved vertically, from %v to %v", plain.Y, moved.Y)
	}
}

// TestTheHoleAFloatLeftDoesNotMoveWithIt is the containment half, and the half
// that says this is an offset rather than a placement.
func TestTheHoleAFloatLeftDoesNotMoveWithIt(t *testing.T) {
	const doc = `<div id="c">x<span id="s"><div id="f"></div></span>yyyy</div>`
	const base = `#c { width: 300px; font-size: 20px }
		#f { float: left; width: 50px; height: 50px; background: rgb(0,0,255) }`

	where := func(css string) []Point {
		var out []Point
		for _, op := range paintOf(t, doc, css) {
			if v, ok := op.(DrawText); ok {
				out = append(out, v.At)
			}
		}
		return out
	}
	moved := where(base + `#s { position: relative; left: 40px; display: inline }`)
	plain := where(base + `#s { display: inline }`)
	if len(moved) == 0 || len(moved) != len(plain) {
		t.Fatalf("%d runs against %d; the fixture must set the same text either way",
			len(moved), len(plain))
	}
	for i := range moved {
		if moved[i] != plain[i] {
			t.Errorf("run %d moved from %v to %v; the float's band is where the "+
				"float was placed, and a relative offset does not move it",
				i, plain[i], moved[i])
		}
	}
}

// TestABlockBrokenOutOfAnInlineStacksWithIt.
//
// §9.9 gives the inline a stacking level and §E.2 paints everything it contains
// at that level. The block is still something it contains, so a "z-index: 2"
// span puts the block it was broken around in front of a "z-index: 1" box —
// which the box tree, where the block is a sibling of the span's halves and of
// nothing else, has no way to say on its own.
func TestABlockBrokenOutOfAnInlineStacksWithIt(t *testing.T) {
	order := func(css string) []style.RGBA {
		var out []style.RGBA
		for _, op := range paintOf(t,
			`<div id="under"></div><span id="s"><div id="over"></div></span>`, css) {
			if r, ok := op.(FillRect); ok && (r.Color == blue || r.Color == green) {
				out = append(out, r.Color)
			}
		}
		return out
	}
	const boxes = `
		#under { position: relative; z-index: 1; background: rgb(0,0,255);
		         width: 100px; height: 100px }
		#over { background: rgb(0,128,0); width: 100px; height: 100px }`

	got := order(boxes + `#s { position: relative; z-index: 2; top: -100px }`)
	if len(got) != 2 {
		t.Fatalf("%d fills, want the two boxes: %v", len(got), got)
	}
	if got[0] != blue || got[1] != green {
		t.Errorf("the fills came out %v; the span is at z-index 2 and the box under "+
			"it at 1, so the block the span contains is painted last", got)
	}

	// Containment: with the span unpositioned the block is ordinary content of
	// the flow, painted in tree order and behind anything positioned. Without
	// this the test above would pass on tree order alone.
	got = order(boxes + `#s { display: inline }
		#under { position: relative; z-index: 1; background: rgb(0,0,255);
		         width: 100px; height: 100px; margin-top: 100px }`)
	if len(got) != 2 {
		t.Fatalf("%d fills, want the two boxes: %v", len(got), got)
	}
	if got[0] != green || got[1] != blue {
		t.Errorf("the fills came out %v; nothing round the block is positioned, so "+
			"it is painted with the blocks and the positioned box goes over it", got)
	}
}

// TestASplitOutBlockIsPaintedExactlyOnce.
//
// Two halves of the painter decide what a stacking context holds: gather, which
// skips a box it believes was hoisted, and hoist, which does the hoisting. A box
// the first thinks is hoisted and the second does not hoist is painted nowhere
// at all — and "nowhere at all" is a page that looks like a rendering choice
// rather than a bug, so it is worth an assertion of its own.
func TestASplitOutBlockIsPaintedExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		what string
		css  string
	}{
		{"inside a positioned wrapper", `
			#w { position: relative }
			#s { position: relative; z-index: 2; display: inline }`},
		{"inside a float", `
			#w { float: left; width: 200px }
			#s { position: relative; display: inline }`},
		{"inside an inline-block", `
			#w { display: inline-block; width: 200px }
			#s { position: relative; display: inline }`},
		{"inside nothing in particular", `#s { position: relative; display: inline }`},
	} {
		ops := paintOf(t,
			`<div id="w"><span id="s"><div id="b">x</div></span></div>`,
			tc.css+`
			#b { background: rgb(0,0,255); width: 50px; height: 50px }`)
		if got := fillsOf(ops, blue); len(got) != 1 {
			t.Errorf("%s: the block was painted %d times, want once: %v",
				tc.what, len(got), got)
		}
	}
}
