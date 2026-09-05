package layout

import (
	"strings"
	"testing"
)

// CSS Grid Layout 2's box, before any of its layout.
//
// The tests here are about what kind of box "display: grid" makes, which is a
// question the box tree answers and not the track sizing. Until this existed
// the answer was "an inline box": the value was not in the display table at
// all, so it fell through to the initial value, and a container's own width,
// height, border and background were dropped on the floor while the finding
// said the box had been laid out as a block.

// gridRect is the border box of the element with the given id, or false if the
// element generated no fragment at all.
func gridRect(t *testing.T, htmlSrc, cssSrc, id string) (Rect, bool) {
	t.Helper()
	f := fragmentFor(layoutOf(t, 1000, htmlSrc, cssSrc), id)
	if f == nil {
		return Rect{}, false
	}
	return f.BorderRect, true
}

// TestAGridContainerIsABlockLevelBox. "display: grid" is block-level outside
// and a grid inside, and "inline-grid" is the same box on a line — which is
// what the display table says and what every other two-part display value in it
// already did.
//
// The failure it replaces is not subtle once seen: an unrecognised display
// value becomes an inline box, and an inline box has no width, no height and no
// border box of its own to paint a background on. A container asked to be
// 200px by 50px with a red background drew nothing at all.
func TestAGridContainerIsABlockLevelBox(t *testing.T) {
	const doc = `<div id="g">a</div><div>after</div>`
	const sized = `#g { width: 200px; height: 50px; background: red }`

	got, ok := gridRect(t, doc, `#g { display: grid }`+sized, "g")
	if !ok {
		t.Fatalf("a grid container generated no fragment, so its own geometry is gone")
	}
	if got.W.Px() != 200 || got.H.Px() != 50 {
		t.Errorf("the grid container is %v; it asked to be 200px by 50px", got)
	}

	// The same box as a block, which is what it is outside: same rectangle.
	block, _ := gridRect(t, doc, `#g { display: block }`+sized, "g")
	if got != block {
		t.Errorf("the grid container is at %v and the same box as a block is at %v",
			got, block)
	}
}

// TestAnInlineGridIsAnAtomicInline. The other half of the pair: inline outside,
// grid inside — a box that sits on a line and still has a size of its own.
//
// Courier at 20px is one character to 12px, so "x" in front of it puts the box
// at exactly 12: a block-level box would be at nought and on a line of its own,
// and a span would have no box at all.
func TestAnInlineGridIsAnAtomicInline(t *testing.T) {
	const doc = `<p>x<span id="g">a</span>y</p>`
	const css = `body { margin: 0 } p { margin: 0; font-family: Courier; font-size: 20px;` +
		` line-height: 20px } #g { display: inline-grid; width: 60px; height: 20px }`

	got, ok := gridRect(t, doc, css, "g")
	if !ok {
		t.Fatalf("an inline grid generated no fragment")
	}
	if got.X.Px() != 12 || got.W.Px() != 60 || got.H.Px() != 20 {
		t.Errorf("the inline grid is %v; it asked to be 60px by 20px and follows "+
			"one character on the line", got)
	}
}

// TestAFloatedInlineGridIsAGrid is §9.7's blockification, which turns an inline
// box that floats or is positioned into a block-level one — and keeps the inner
// half of the two-value displays that have one. A floated inline-grid is a
// block-level grid, not a block.
func TestAFloatedInlineGridIsAGrid(t *testing.T) {
	for _, c := range []struct{ what, css string }{
		{"floated", `#g { display: inline-grid; float: left }`},
		{"positioned", `#g { display: inline-grid; position: absolute }`},
	} {
		b := boxOf(t, `<div id="g">a</div>`, c.css, "g")
		if b == nil {
			t.Fatalf("%s: no box", c.what)
		}
		if b.Outer != OuterBlock || b.Inner != InnerGrid {
			t.Errorf("a %s inline grid is %v outside and %v inside, want a "+
				"block-level grid", c.what, b.Outer, b.Inner)
		}
	}
}

// TestAGridContainerSealsItsOwnFormattingContext. §2 of Grid: a grid container
// establishes an independent formatting context. That is true whatever this
// engine does inside it, and it is what stops a float escaping and a margin
// collapsing through — the two ways a box that does not seal its context leaks
// into the page around it.
func TestAGridContainerSealsItsOwnFormattingContext(t *testing.T) {
	// A float inside the container does not reach out and shorten the line of
	// the paragraph below it.
	const floating = `<div id="g"><div id="f">float</div></div><p id="p">after</p>`
	const floatCSS = `#g { display: grid; width: 300px } #f { float: left; width: 100px;` +
		` height: 100px } #p { margin: 0 }`
	p, ok := gridRect(t, floating, floatCSS, "p")
	if !ok {
		t.Fatalf("the paragraph after the container generated no fragment")
	}
	if p.X.Px() != 8 {
		t.Errorf("the paragraph is at x=%v, so the float inside the grid "+
			"container reached out of it", p.X)
	}

	// And a margin inside does not collapse through the container's own edge:
	// the container starts where its parent's content starts, not 40px down.
	const collapsing = `<div id="wrap"><div id="g"><div id="in">x</div></div></div>`
	const marginCSS = `#wrap { margin: 0 } #g { display: grid } #in { margin-top: 40px }`
	g, ok := gridRect(t, collapsing, marginCSS, "g")
	if !ok {
		t.Fatalf("the grid container generated no fragment")
	}
	if g.Y.Px() != 8 {
		t.Errorf("the grid container's top is at y=%v, so the margin inside it "+
			"collapsed through its edge", g.Y)
	}
}

// TestTheAnonymousBlockRulesDoNotReachIntoAGrid. A grid container lays out its
// own children, so §9.2.1.1's split and CSS Display §2.1's anonymous block are
// not for it: a block inside an inline-grid belongs to the grid, and lifting it
// out would empty the very box that was meant to hold it.
func TestTheAnonymousBlockRulesDoNotReachIntoAGrid(t *testing.T) {
	const doc = `<p>before<span id="g"><div id="in">block</div></span>after</p>`
	const css = `#g { display: inline-grid; background: red }`

	g := fragmentFor(layoutOf(t, 1000, doc, css), "g")
	if g == nil {
		t.Fatalf("the inline grid generated no fragment")
	}
	if fragmentFor(g, "in") == nil {
		t.Errorf("the block inside the inline grid was lifted out of it, so the " +
			"box that was meant to hold it is empty")
	}
}

// TestTheTwoValueDisplayNamesTheGridToo. "display: block grid" and "display:
// inline grid" are the long forms of the same two values, and the table that
// reads them is the same table.
func TestTheTwoValueDisplayNamesTheGridToo(t *testing.T) {
	const doc = `<div id="g">a</div>`
	for _, c := range []struct{ value, want string }{
		{"block grid", "block"},
		{"inline grid", "inline"},
	} {
		b := boxOf(t, doc, `#g { display: `+c.value+` }`, "g")
		if b == nil {
			t.Fatalf("%q produced no box", c.value)
		}
		if b.Inner != InnerGrid || !strings.HasPrefix(b.Outer.String(), c.want) {
			t.Errorf("%q made a box that is %v outside and %v inside", c.value,
				b.Outer, b.Inner)
		}
	}
}
