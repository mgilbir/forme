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

// CSS Grid Layout 2 §12, laid out.
//
// The suite has one document that arranges a grid — text-indent's anonymous
// item test, which is about the items and not the tracks — so the reftest count
// is a regression check here and nothing more. What says this is right is the
// arithmetic below, and the fixture is chosen so that every number in it is a
// whole one: Courier at 20px puts one character at exactly 12px, and every
// container width divides its free space exactly.

const gridCSS = `body { margin: 0 }
	#g { display: grid; font-family: Courier; font-size: 20px; line-height: 20px }
	#g > div { font-family: Courier; font-size: 20px; line-height: 20px }`

// gridCells is where each item of #g was placed, in pixels.
func gridCells(t *testing.T, htmlSrc, extra string) []Rect {
	t.Helper()
	g := fragmentFor(layoutOf(t, 1000, htmlSrc, gridCSS+extra), "g")
	if g == nil {
		t.Fatalf("the grid container generated no fragment")
	}
	out := make([]Rect, 0, len(g.Children))
	for _, c := range g.Children {
		out = append(out, c.BorderRect)
	}
	return out
}

func wantCells(t *testing.T, got []Rect, want [][4]float64, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d items, want %d: %v", what, len(got), len(want), got)
	}
	for i := range want {
		g := [4]float64{got[i].X.Px(), got[i].Y.Px(), got[i].W.Px(), got[i].H.Px()}
		if g != want[i] {
			t.Errorf("%s: item %d is at %v, want %v\n  whole grid: %v",
				what, i, g, want[i], got)
		}
	}
}

// The four-item fixture: one, two, three and four characters, so that which
// item is in which cell can be read off the row.
const fourItems = `<div id="g"><div>a</div><div>bb</div><div>ccc</div><div>dddd</div></div>`

// TestItemsAreDealtIntoTheColumnsInOrder is §8.5's automatic placement: one
// cell each, along the columns and then down, which is the whole of the
// placement in a grid where no item names a line.
func TestItemsAreDealtIntoTheColumnsInOrder(t *testing.T) {
	got := gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 100px 100px }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 100, 20}, {100, 0, 100, 20},
		{0, 20, 100, 20}, {100, 20, 100, 20},
	}, "four items in two fixed columns")

	// Three columns and four items make a second row holding one, and the
	// container is as deep as the two rows come to.
	got = gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 100px 100px 100px }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 100, 20}, {100, 0, 100, 20}, {200, 0, 100, 20},
		{0, 20, 100, 20},
	}, "four items in three columns")
}

// TestAFractionTakesItsShareOfWhatIsLeft is §12.7. An "fr" is not a length: it
// is a share of the space the other tracks did not take, which is why "1fr 2fr"
// is a third and two thirds and why the same declaration in a wider container
// gives wider columns.
func TestAFractionTakesItsShareOfWhatIsLeft(t *testing.T) {
	got := gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 20}, {150, 0, 150, 20},
		{0, 20, 150, 20}, {150, 20, 150, 20},
	}, "two equal fractions")

	got = gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 1fr 2fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 100, 20}, {100, 0, 200, 20},
		{0, 20, 100, 20}, {100, 20, 200, 20},
	}, "one fraction against two")

	// A length beside a fraction takes its size first and the fraction takes
	// the rest, which is the two-column page every document has.
	got = gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 60px 1fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 60, 20}, {60, 0, 240, 20},
		{0, 20, 60, 20}, {60, 20, 240, 20},
	}, "a length beside a fraction")
}

// TestFactorsBelowOneTakeOnlyThatFractionOfTheSpaceInAGrid is §12.7.1's clause
// for factors adding to less than one, which is what gives "0.25fr" its
// meaning: the tracks between them asked for half the container and take
// exactly that, leaving the rest empty. Without it a lone "0.5fr" track would
// fill the container, which is the same picture "1fr" gives.
func TestFactorsBelowOneTakeOnlyThatFractionOfTheSpaceInAGrid(t *testing.T) {
	got := gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 0.25fr 0.25fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 75, 20}, {75, 0, 75, 20},
		{0, 20, 75, 20}, {75, 20, 75, 20},
	}, "factors adding to a half")

	// Exactly one is the boundary and the clause does not apply at it.
	got = gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 0.5fr 0.5fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 20}, {150, 0, 150, 20},
		{0, 20, 150, 20}, {150, 20, 150, 20},
	}, "factors adding to one")
}

// TestAnAutomaticTrackGrowsFromItsContentToTheContainer is the difference
// between "auto" and "max-content", and it is the one worth stating twice:
// both start at what their content needs, and only "auto" takes what is left
// over afterwards.
//
// The columns hold one and three characters, and two and four: 36px and 48px of
// content in a 300px container leaves 216px, and half of that on each is 144
// and 156.
func TestAnAutomaticTrackGrowsFromItsContentToTheContainer(t *testing.T) {
	got := gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: auto auto }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 144, 20}, {144, 0, 156, 20},
		{0, 20, 144, 20}, {144, 20, 156, 20},
	}, "two automatic columns")

	got = gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: max-content max-content }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 36, 20}, {36, 0, 48, 20},
		{0, 20, 36, 20}, {36, 20, 48, 20},
	}, "two max-content columns")
}

// TestAnAutomaticTrackDoesNotOverflowTheContainerItIsIn is §12.4's two numbers,
// and the reason a track has two: a column starts at the width below which its
// content would spill out and grows towards the width at which it would stop
// wrapping, as far as the container allows and no further.
//
// "ab cd" is 60px of max-content and 24px of min-content, so two such columns
// in a 100px container cannot both have the whole width they would like. They
// come out 50 and 50 rather than 60 and 60 — the room was shared equally and
// ran out — and each item wraps onto a second line, which is what a column
// sized at max-content would have hidden by overflowing instead.
func TestAnAutomaticTrackDoesNotOverflowTheContainerItIsIn(t *testing.T) {
	const two = `<div id="g"><div>ab cd</div><div>ef gh</div></div>`
	got := gridCells(t, two, `#g { width: 100px; grid-template-columns: auto auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 50, 40}, {50, 0, 50, 40}}, "two narrow columns")

	// The same content in a container with room for it: 60 and 60 of content,
	// and the 180px left over shared equally.
	got = gridCells(t, two, `#g { width: 300px; grid-template-columns: auto auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}}, "two roomy columns")
}

// TestRepeatWritesTheSameTrackAgain. §7.2.3.1's repeat() with a count is a
// spelling of the track list it holds, and it is how every real grid is
// written.
func TestRepeatWritesTheSameTrackAgain(t *testing.T) {
	got := gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: repeat(2, 1fr) }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 20}, {150, 0, 150, 20},
		{0, 20, 150, 20}, {150, 20, 150, 20},
	}, "two repeated fractions")

	// A repeat of more than one track, and a track beside it.
	got = gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: 60px repeat(2, 1fr) }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 60, 20}, {60, 0, 120, 20}, {180, 0, 120, 20},
		{0, 20, 60, 20},
	}, "a length and two repeated fractions")
}

// TestTheGapsComeOutOfTheTracks. A grid reads both gaps, one per axis, and they
// are taken off the container before the tracks divide it — so two fractions in
// a 300px container with a 20px gap are 140 each and not 150.
func TestTheGapsComeOutOfTheTracks(t *testing.T) {
	got := gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: 1fr 1fr; column-gap: 20px; row-gap: 10px }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 140, 20}, {160, 0, 140, 20},
		{0, 30, 140, 20}, {160, 30, 140, 20},
	}, "two columns and two rows with gaps")

	// The "gap" shorthand is the pair, in the order row then column.
	got = gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: 1fr 1fr; gap: 10px 20px }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 140, 20}, {160, 0, 140, 20},
		{0, 30, 140, 20}, {160, 30, 140, 20},
	}, "the same two gaps written as one")
}

// TestARowIsAsTallAsTheTallestThingInIt, and an item fills the cell it is in on
// both axes — which is what "align-items: normal" and "justify-items: normal"
// come to in a grid.
func TestARowIsAsTallAsTheTallestThingInIt(t *testing.T) {
	const two = `<div id="g"><div>a</div><div>b<br>c</div></div>`
	got := gridCells(t, two, `#g { width: 300px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 150, 40}, {150, 0, 150, 40}},
		"a one-line item beside a two-line one")
}

// TestAContainerWithAHeightSharesItOutBetweenTheRows. §12.8 again, on the other
// axis: the rows start at their content and the automatic ones take what the
// container has over.
func TestAContainerWithAHeightSharesItOutBetweenTheRows(t *testing.T) {
	got := gridCells(t, fourItems,
		`#g { width: 300px; height: 200px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 100}, {150, 0, 150, 100},
		{0, 100, 150, 100}, {150, 100, 150, 100},
	}, "two rows in a container twice their height")

	// A stated row height is stated, and the rows below it start after it.
	got = gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: 1fr 1fr; grid-template-rows: 60px }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 60}, {150, 0, 150, 60},
		{0, 60, 150, 20}, {150, 60, 150, 20},
	}, "one stated row and one implicit one")
}

// TestTextInsideAGridContainerBecomesAnItemOfItsOwn is §6's anonymous grid
// item, which is CSS Flexible Box Layout §4's rule in the other specification
// and in the same words. It is what the one document in the suite that arranges
// a grid is about.
func TestTextInsideAGridContainerBecomesAnItemOfItsOwn(t *testing.T) {
	got := gridCells(t, `<div id="g">ab<div>c</div></div>`,
		`#g { width: 300px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}},
		"a run of text beside an element")

	// White space that collapses to nothing is not an item, or every document
	// written with a newline between its elements would have twice the items it
	// wrote.
	got = gridCells(t, "<div id=\"g\">\n  <div>a</div>\n  <div>b</div>\n</div>",
		`#g { width: 300px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}},
		"two items with newlines between them")
}

// TestAFloatInAGridContainerIsAnItem is §6's other sentence — "float and clear
// have no effect on a grid item" — and it is the flex rule again: a float is
// out of the flow everywhere else, and a box out of the flow is not an item at
// all.
func TestAFloatInAGridContainerIsAnItem(t *testing.T) {
	got := gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: 1fr 1fr } #g > div:first-child { float: left }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 150, 20}, {150, 0, 150, 20},
		{0, 20, 150, 20}, {150, 20, 150, 20},
	}, "a floated item in a grid")
}

// TestAnAbsolutelyPositionedChildOfAGridIsStillPlaced. It is not an item — §10.1
// takes it out of the flow — and it is not nobody's business either: the block
// walk is what records such a box for placing, and this file is in its place.
func TestAnAbsolutelyPositionedChildOfAGridIsStillPlaced(t *testing.T) {
	for _, doc := range []string{
		`<div id="g"><div id="p" style="position: absolute">x</div></div>`,
		`<div id="g"><div>a</div><div id="p" style="position: absolute">x</div></div>`,
	} {
		root := layoutOf(t, 1000, doc, gridCSS+`#g { width: 300px; grid-template-columns: 1fr }`)
		if n := fragmentsWithID(root, "p"); n != 1 {
			t.Errorf("the absolutely positioned box is on the page %d times, want "+
				"once: %s", n, doc)
		}
	}
}

// TestOrderMovesAGridItemToAnotherCell. §6.2's order-modified document order is
// what the automatic flow deals from, so an item that asks to come last is
// placed in the last cell — and the sort is stable, so the items that did not
// ask keep the order the document put them in.
func TestOrderMovesAGridItemToAnotherCell(t *testing.T) {
	got := gridCells(t, `<div id="g"><div id="a">a</div><div>bb</div><div>ccc</div></div>`,
		`#g { width: 300px; grid-template-columns: 100px 100px } #a { order: 1 }`)
	wantCells(t, got, [][4]float64{
		{0, 0, 100, 20}, {100, 0, 100, 20}, {0, 20, 100, 20},
	}, "three items with the first sent to the end")
}

// TestAGridContainerThisEngineCannotArrangeSaysSo.
//
// The gate. Each of these is a grid this file does not lay out, and each must
// come out as the column of blocks it was before — and be reported, because a
// table of tracks silently laid out as a stack is exactly the plausible
// wrongness the finding exists for.
func TestAGridContainerThisEngineCannotArrangeSaysSo(t *testing.T) {
	for _, c := range []struct{ what, css, names string }{
		{"a named line", `#g { grid-template-columns: [start] 1fr }`, "does not size"},
		{"a flexible minimum", `#g { grid-template-columns: minmax(1fr, 2fr) }`, "does not size"},
		{"a minmax of one thing", `#g { grid-template-columns: minmax(100px) }`, "does not size"},
		{"a fit-content", `#g { grid-template-columns: fit-content(100px) }`, "does not size"},
		{"two automatic repeats",
			`#g { grid-template-columns: repeat(auto-fill, 50px) repeat(auto-fill, 50px) }`,
			"does not size"},
		{"a nested repeat", `#g { grid-template-columns: repeat(2, repeat(2, 1fr)) }`, "does not size"},
		{"a named row", `#g { grid-template-rows: [top] 20px }`, "does not size"},
		{"a ragged template", `#g { grid-template-areas: "a b" "c" }`, "not all the same length"},
		{"an area in two places", `#g { grid-template-areas: "a b" "b a" }`, "do not touch"},
		{"a flow that is neither", `#g { grid-auto-flow: sideways }`,
			"flow this engine does not follow"},
		{"implicit tracks sized by a function this engine cannot read",
			`#g { grid-auto-rows: fit-content(50px) }`, "implicit tracks"},
		{"tracks on a baseline", `#g { align-content: baseline }`, "aligned by a rule"},
		{"items on a baseline", `#g { align-items: baseline }`, "aligned by a rule"},
		{"a safe alignment", `#g { justify-content: safe center }`, "aligned by a rule"},
		{"items aligned down the page by a side of it",
			`#g { align-items: left }`, "names a side across it"},
		{"an item in an area nobody drew", `#g > div:first-child { grid-area: header }`,
			"the template does not draw"},
		{"an item at a named line", `#g > div:first-child { grid-column: main }`, "cannot find"},
		{"an item counting from the end", `#g > div:first-child { grid-column: -1 }`, "cannot find"},
		{"an item spanning to a name", `#g > div:first-child { grid-row: span main }`, "cannot find"},
		{"an item on a baseline", `#g > div:first-child { align-self: baseline }`, "aligned by a rule"},
		{"an automatic margin", `#g > div:first-child { margin-left: auto }`, "automatic margin"},
	} {
		t.Run(c.what, func(t *testing.T) {
			got := Compose(Input{HTML: fourItems, CSS: []Stylesheet{{
				Source: gridCSS + `#g { width: 300px; grid-template-columns: 1fr 1fr }` + c.css}}},
				Options{})
			var said string
			for _, f := range got.Findings {
				if strings.Contains(f.Message, "grid container") {
					said = f.Message
				}
			}
			if said == "" {
				t.Fatalf("nothing was reported about a grid with %s, so a table of "+
					"tracks laid out as a stack says nothing about it: %v",
					c.what, got.Findings)
			}
			if !strings.Contains(said, c.names) {
				t.Errorf("the finding for %s is %q, which does not name %q",
					c.what, said, c.names)
			}
			// And it really was laid out as a column of blocks.
			row := gridCells(t, fourItems,
				`#g { width: 300px; grid-template-columns: 1fr 1fr }`+c.css)
			for i, it := range row {
				if it.X.Px() != 0 || it.W.Px() != 300 {
					t.Errorf("item %d of a refused grid is at x=%v and %vpx wide, "+
						"want a full-width block at nought", i, it.X, it.W)
				}
			}
		})
	}
}

// TestAnArrangedGridSaysNothing is the containment argument: the finding must
// not fire on the containers this file does arrange, or every grid document in
// the world would carry a report of a page that is right.
func TestAnArrangedGridSaysNothing(t *testing.T) {
	for _, css := range []string{
		`#g { width: 300px }`,
		`#g { width: 300px; grid-template-columns: 1fr 1fr }`,
		`#g { width: 300px; grid-template-columns: repeat(3, 100px); gap: 10px }`,
		`#g { width: 300px; grid-template-columns: auto max-content min-content }`,
		`#g { width: 300px; grid-template-rows: 40px 40px; grid-auto-flow: row }`,
		`#g { width: 300px; justify-content: normal; align-items: stretch }`,
		`#g { width: 300px; justify-content: space-between; align-items: center }`,
		`#g { width: 300px } #g > div:first-child { justify-self: end; align-self: start }`,
		`#g { width: 300px } #g > div { order: 0; grid-column: auto }`,
		`#g { width: 300px; grid-template-columns: 100px 100px } #g > div:first-child { grid-column: 2 }`,
		`#g { width: 300px; grid-template-columns: 100px 100px } #g > div:first-child { grid-row: 1 / 3 }`,
		`#g { width: 300px; grid-template-areas: "a b" "c d" }`,
		`#g { width: 300px; grid-auto-rows: 50px; grid-auto-columns: 50px }`,
		`#g { width: 300px; direction: rtl; grid-template-columns: 100px 100px }`,
		`#g { width: 300px; grid-auto-flow: column dense }`,
	} {
		got := Compose(Input{HTML: fourItems,
			CSS: []Stylesheet{{Source: gridCSS + css}}}, Options{})
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "grid") || f.Property == "display" {
				t.Errorf("%q reported %q, and the container was arranged", css, f.Message)
			}
		}
	}
}

// TestATrackStopsGrowingAtItsLimit is §12.6's freeze, and it is the clause that
// tells "max-content" from "auto" a second time: growing every track equally
// until the space runs out would give a column more room than its content can
// use, and take that room from the column that could.
//
// One character beside four, in a 300px container: the max-content column stops
// at the 12px it asked for and the automatic one takes the other 288.
func TestATrackStopsGrowingAtItsLimit(t *testing.T) {
	const two = `<div id="g"><div>a</div><div>dddd</div></div>`
	got := gridCells(t, two, `#g { width: 300px; grid-template-columns: max-content auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 12, 20}, {12, 0, 288, 20}},
		"a max-content column beside an automatic one")

	// Two automatic columns where only one of them can use more room: "ab cd"
	// starts at the 24px of its longest word and would stop wrapping at 60,
	// while "dddd" is 48 either way. The 228px over is offered to both, the
	// second is full at once, and the first takes 36 of it to reach its limit —
	// and only then is what is *still* left shared equally between them.
	//
	// Growing them equally from the start would give the first column all 228
	// and leave the second at 48: a column stretched far past the width its
	// content could use, beside one that could have used it.
	const wrapping = `<div id="g"><div>ab cd</div><div>dddd</div></div>`
	got = gridCells(t, wrapping, `#g { width: 300px; grid-template-columns: auto auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 156, 20}, {156, 0, 144, 20}},
		"one column that can grow beside one that cannot")

	// The same two columns with only 48px to give: 36 of it takes the first to
	// its limit and the 12 left over is split.
	got = gridCells(t, wrapping, `#g { width: 120px; grid-template-columns: auto auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 66, 20}, {66, 0, 54, 20}},
		"one column that can grow, in a narrow container")
}

// TestAnItemsOwnWidthIsWhatItAsksOfItsColumn. §12.5's contributions are the
// item's, not its content's: a box that stated a width is that wide whatever
// its words would do, and the column is sized to hold it.
func TestAnItemsOwnWidthIsWhatItAsksOfItsColumn(t *testing.T) {
	const two = `<div id="g"><div>a</div><div>b</div></div>`
	got := gridCells(t, two,
		`#g { width: 300px; grid-template-columns: max-content max-content }`+
			`#g > div:first-child { width: 80px }`)
	wantCells(t, got, [][4]float64{{0, 0, 80, 20}, {80, 0, 12, 20}},
		"an item that stated a width of its own")
}

// TestWhatAMeasuringLayoutTookOutOfTheFlowGoesWithIt. Every item is laid out
// twice — once at its column's width to find out how tall it is, and once at
// the whole cell — and the first answer is thrown away. An absolutely
// positioned box found inside a discarded fragment hangs off a fragment nobody
// will paint, and leaving its record on placeAbsolutes' list spends that list's
// budget on a box that is not on the page.
//
// The cap is lowered to make the arithmetic small: three out-of-flow boxes and
// a limit of four. Before this was handled the grid recorded six.
func TestWhatAMeasuringLayoutTookOutOfTheFlowGoesWithIt(t *testing.T) {
	held := maxAbsolutes
	defer func() { maxAbsolutes = held }()
	maxAbsolutes = 4

	const doc = `<div id="g">` +
		`<div>a<i id="p1" style="position: absolute">1</i></div>` +
		`<div>b<i id="p2" style="position: absolute">2</i></div>` +
		`<div>c<i id="p3" style="position: absolute">3</i></div></div>`
	const css = `#g { width: 300px; grid-template-columns: 1fr 1fr }`

	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: gridCSS + css}}}, Options{})
	for _, f := range got.Findings {
		if f.Rule == RuleLimit {
			t.Errorf("three out-of-flow boxes with a limit of four reported %q, "+
				"so the measuring layouts are still on the list", f.Message)
		}
	}

	// And the three that are real are still placed.
	root := layoutOf(t, 1000, doc, gridCSS+css)
	for _, id := range []string{"p1", "p2", "p3"} {
		if n := fragmentsWithID(root, id); n != 1 {
			t.Errorf("%s is on the page %d times, want once", id, n)
		}
	}
}

// TestJustifyContentPlacesTheTracks is §10.3, and it is the same six answers
// justify-content gives a flex line — one property in Box Alignment, one
// function here. The columns are 100px each in a 300px container, so there is
// 100px over and each keyword is a different answer to where it goes.
//
// A fixed track is what makes this visible at all: an automatic one would have
// taken the leftover itself, which is what "normal" means and is why nothing
// moves without one of these.
func TestJustifyContentPlacesTheTracks(t *testing.T) {
	const fixed = `#g { width: 300px; grid-template-columns: 100px 100px }`
	for _, c := range []struct {
		value string
		want  [][4]float64
	}{
		{"normal", [][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}}},
		{"start", [][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}}},
		{"end", [][4]float64{{100, 0, 100, 20}, {200, 0, 100, 20}}},
		{"center", [][4]float64{{50, 0, 100, 20}, {150, 0, 100, 20}}},
		{"space-between", [][4]float64{{0, 0, 100, 20}, {200, 0, 100, 20}}},
		// Two shares of 50: half of one at each end and a whole one between.
		{"space-around", [][4]float64{{25, 0, 100, 20}, {175, 0, 100, 20}}},
		// Three gaps of 33 and a third, which is not a whole number of layout
		// units and is why the offsets are taken from the whole free space
		// rather than added up one gap at a time.
		{"space-evenly", [][4]float64{{33.328125, 0, 100, 20}, {166.671875, 0, 100, 20}}},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := gridCells(t, `<div id="g"><div>a</div><div>bb</div></div>`,
				fixed+`#g { justify-content: `+c.value+` }`)
			wantCells(t, got, c.want, "two fixed columns packed by "+c.value)
		})
	}
}

// TestAlignContentPlacesTheRows is the same property on the other axis, and the
// difference between it and align-items is the difference between moving the
// rows and moving what is in them: two 20px rows in a 200px container leave
// 160px, and "center" puts the pair of them in the middle rather than each item
// in the middle of a stretched row.
func TestAlignContentPlacesTheRows(t *testing.T) {
	const tall = `#g { width: 300px; height: 200px; grid-template-columns: 100px 100px }`

	got := gridCells(t, fourItems, tall+`#g { align-content: center }`)
	wantCells(t, got, [][4]float64{
		{0, 80, 100, 20}, {100, 80, 100, 20},
		{0, 100, 100, 20}, {100, 100, 100, 20},
	}, "two rows centred down the container")

	// With nothing said the rows take the room instead, which is §12.8, and the
	// items fill them.
	got = gridCells(t, fourItems, tall)
	wantCells(t, got, [][4]float64{
		{0, 0, 100, 100}, {100, 0, 100, 100},
		{0, 100, 100, 100}, {100, 100, 100, 100},
	}, "two rows stretched down the container")

	// And align-items moves the items inside those stretched rows.
	got = gridCells(t, fourItems, tall+`#g { align-items: center }`)
	wantCells(t, got, [][4]float64{
		{0, 40, 100, 20}, {100, 40, 100, 20},
		{0, 140, 100, 20}, {100, 140, 100, 20},
	}, "items centred in stretched rows")
}

// TestAnAlignedItemIsItsOwnSize is §10.5, and it is the half of the alignment
// that is not arithmetic: an item that is stretched is the size of its cell,
// and an item that is aligned is the size of its own content — fit-content,
// held down to the cell and up to what its words need.
//
// The four items are one, two, three and four characters, so their own widths
// are 12, 24, 36 and 48 in cells of 100.
func TestAnAlignedItemIsItsOwnSize(t *testing.T) {
	const fixed = `#g { width: 300px; grid-template-columns: 100px 100px }`

	wantCells(t, gridCells(t, fourItems, fixed+`#g { justify-items: start }`), [][4]float64{
		{0, 0, 12, 20}, {100, 0, 24, 20},
		{0, 20, 36, 20}, {100, 20, 48, 20},
	}, "items at the start of their cells")

	wantCells(t, gridCells(t, fourItems, fixed+`#g { justify-items: center }`), [][4]float64{
		{44, 0, 12, 20}, {138, 0, 24, 20},
		{32, 20, 36, 20}, {126, 20, 48, 20},
	}, "items centred in their cells")

	wantCells(t, gridCells(t, fourItems, fixed+`#g { justify-items: end }`), [][4]float64{
		{88, 0, 12, 20}, {176, 0, 24, 20},
		{64, 20, 36, 20}, {152, 20, 48, 20},
	}, "items at the end of their cells")

	// "left" and "right" are the same two ends in a grid whose columns run left
	// to right, which is the only kind this engine arranges.
	wantCells(t, gridCells(t, fourItems, fixed+`#g { justify-items: right }`), [][4]float64{
		{88, 0, 12, 20}, {176, 0, 24, 20},
		{64, 20, 36, 20}, {152, 20, 48, 20},
	}, "items at the right of their cells")
}

// TestAnItemAlignsItselfBeforeItsContainerDoes. justify-self and align-self are
// the item's own answer, and "auto" — their initial value — is what defers to
// the container's.
func TestAnItemAlignsItselfBeforeItsContainerDoes(t *testing.T) {
	const fixed = `#g { width: 300px; grid-template-columns: 100px 100px }`

	// One item aligned in a grid that stretches the rest.
	wantCells(t, gridCells(t, fourItems, fixed+`#g > div:first-child { justify-self: end }`),
		[][4]float64{
			{88, 0, 12, 20}, {100, 0, 100, 20},
			{0, 20, 100, 20}, {100, 20, 100, 20},
		}, "one item aligned by itself")

	// And the other way: one item stretched in a grid that aligns the rest.
	wantCells(t, gridCells(t, fourItems,
		fixed+`#g { justify-items: center } #g > div:first-child { justify-self: stretch }`),
		[][4]float64{
			{0, 0, 100, 20}, {138, 0, 24, 20},
			{32, 20, 36, 20}, {126, 20, 48, 20},
		}, "one item stretched by itself")

	// "auto" is not a fourth alignment: it is the container's.
	wantCells(t, gridCells(t, fourItems,
		fixed+`#g { justify-items: end } #g > div:first-child { justify-self: auto }`),
		[][4]float64{
			{88, 0, 12, 20}, {176, 0, 24, 20},
			{64, 20, 36, 20}, {152, 20, 48, 20},
		}, "an item deferring to its container")
}

// TestAnAlignedTrackIsNotStretched is the one interaction between the two
// halves: §12.8 gives the space left over to the automatic tracks *because*
// justify-content and align-content are at "normal", and any other value asks
// for that space to be left where the alignment puts it instead.
//
// Two automatic columns of 36px and 48px of content in a 300px container: with
// nothing said they take 144 and 156, and centred they stay at 36 and 48 with
// the 216px over split either side.
func TestAnAlignedTrackIsNotStretched(t *testing.T) {
	wantCells(t, gridCells(t, fourItems, `#g { width: 300px; grid-template-columns: auto auto }`),
		[][4]float64{
			{0, 0, 144, 20}, {144, 0, 156, 20},
			{0, 20, 144, 20}, {144, 20, 156, 20},
		}, "two automatic columns with nothing said")

	wantCells(t, gridCells(t, fourItems,
		`#g { width: 300px; grid-template-columns: auto auto; justify-content: center }`),
		[][4]float64{
			{108, 0, 36, 20}, {144, 0, 48, 20},
			{108, 20, 36, 20}, {144, 20, 48, 20},
		}, "two automatic columns centred instead")
}

// TestMinmaxWritesTheTwoEndsOfATrackSeparately is §7.2.2. Every track has two
// sizing functions and minmax() is the spelling that writes them apart: a
// column that may not be narrower than 100px and takes a share of what is left
// over is the responsive layout every document has.
func TestMinmaxWritesTheTwoEndsOfATrackSeparately(t *testing.T) {
	const two = `<div id="g"><div>ab cd</div><div>dddd</div></div>`

	// Room for both minimums and more: the fraction decides, and the minimum
	// does nothing.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: minmax(100px, 1fr) minmax(100px, 1fr) }`),
		[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}}, "two fractions above their minimums")

	// One minimum bigger than its share: it holds, and the other fraction takes
	// what is left rather than its own half.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: minmax(200px, 1fr) minmax(100px, 1fr) }`),
		[][4]float64{{0, 0, 200, 20}, {200, 0, 100, 20}}, "a minimum bigger than its share")

	// No room for either: the minimums hold and the grid overflows, which is
	// what a minimum is for — a column narrower than that was not wanted.
	wantCells(t, gridCells(t, two,
		`#g { width: 100px; grid-template-columns: minmax(80px, 1fr) minmax(80px, 1fr) }`),
		[][4]float64{{0, 0, 80, 20}, {80, 0, 80, 20}}, "two minimums in a container too narrow")

	// A maximum that is a length caps the growing, and what it did not take
	// goes to the automatic track beside it.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: minmax(min-content, 100px) auto }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 200, 20}}, "a capped column beside an automatic one")

	// A maximum that is a length stops the stretching too, which is the one
	// place the two ends have to be told apart: the *maximum* decides whether a
	// track takes what is left over, so "minmax(auto, 60px)" stops at 60 and
	// the automatic track beside it takes the other 240.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: minmax(auto, 60px) auto }`),
		[][4]float64{{0, 0, 60, 20}, {60, 0, 240, 20}}, "a capped column that does not stretch")

	// And it composes with repeat(), which is how it is written in practice.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: repeat(2, minmax(50px, 1fr)) }`),
		[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}}, "a repeated minmax")
}

// TestASingleTrackSizeIsBothEnds, except for the two that are not: "auto" is
// the largest minimum its items need at the low end and max-content at the
// high one, and a flexible track is "auto" at the low end — which is why "1fr"
// never comes out narrower than the words in it.
func TestASingleTrackSizeIsBothEnds(t *testing.T) {
	const two = `<div id="g"><div>ab cd</div><div>dddd</div></div>`

	// A container with no room to give: the fraction cannot go below the
	// longest word in it, so the two columns come out at their content's
	// minimum — 24px for the wider of "ab" and "cd", 48 for "dddd" — and
	// overflow rather than taking half the container each. The row is as tall
	// as the item that had to wrap.
	wantCells(t, gridCells(t, two, `#g { width: 40px; grid-template-columns: 1fr 1fr }`),
		[][4]float64{{0, 0, 24, 40}, {24, 0, 48, 40}}, "two fractions with nothing to divide")

	// A fixed track is its length at both ends whatever its content needs.
	wantCells(t, gridCells(t, two, `#g { width: 300px; grid-template-columns: 20px 20px }`),
		[][4]float64{{0, 0, 20, 40}, {20, 0, 20, 40}}, "two fixed columns narrower than their text")
}

// TestAutoFillPutsInAsManyTracksAsWillFit is §7.2.3.2: "repeat(auto-fill, …)"
// is not a count but a question about the container, and the answer is the
// largest number of repetitions that does not overflow it.
//
// It is the responsive layout the whole feature is written for — a row of cards
// that becomes two rows on a narrower page — and the size each track counts as
// is its *maximum* where that is a length and its minimum otherwise, which is
// what makes "minmax(200px, 1fr)" fit as many 200px columns as there is room
// for rather than one column of everything.
func TestAutoFillPutsInAsManyTracksAsWillFit(t *testing.T) {
	const three = `<div id="g"><div>a</div><div>b</div><div>c</div></div>`

	// Three 100px columns go into 300px exactly.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fill, 100px) }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}, {200, 0, 100, 20}},
		"three columns that fit exactly")

	// 140px is the minimum, so only two fit — and the two then take a half of
	// the container each, because their maximum is a fraction. The third item
	// starts a second row.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)) }`),
		[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}, {0, 20, 150, 20}},
		"two columns and a second row")

	// The gaps are counted with the tracks: 100px columns with 20px between
	// them fit twice in 300px and not three times.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; column-gap: 20px; grid-template-columns: repeat(auto-fill, 100px) }`),
		[][4]float64{{0, 0, 100, 20}, {120, 0, 100, 20}, {0, 20, 100, 20}},
		"two columns and a gap")

	// A track written beside the repetition takes its room first, and what is
	// left is what the repetition is counted against: 200px of it, four times
	// 50. Six items are enough to reach the end of the row and see that there
	// are five columns and not six.
	const six = `<div id="g"><div>a</div><div>b</div><div>c</div>` +
		`<div>d</div><div>e</div><div>f</div></div>`
	wantCells(t, gridCells(t, six,
		`#g { width: 300px; grid-template-columns: 100px repeat(auto-fill, 50px) }`),
		[][4]float64{
			{0, 0, 100, 20}, {100, 0, 50, 20}, {150, 0, 50, 20},
			{200, 0, 50, 20}, {250, 0, 50, 20}, {0, 20, 100, 20},
		}, "a fixed column beside a repetition")

	// The size a track counts as is its maximum where that is a length: two
	// hundred goes into three hundred once, and the column then takes the whole
	// container because a fraction is what it grows to.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fill, minmax(100px, 200px)) }`),
		[][4]float64{{0, 0, 200, 20}, {0, 20, 200, 20}, {0, 40, 200, 20}},
		"a repetition counted by its maximum")

	// A repetition of something with no definite size cannot be counted, and
	// §7.2.3.2's own answer is one — the track list is still the list, it is
	// simply written once. The gap is there because without it the arithmetic
	// would come to one anyway, and a rule that is only right when a second
	// thing is absent has not been tested.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; column-gap: 20px; grid-template-columns: repeat(auto-fill, auto) }`),
		[][4]float64{{0, 0, 300, 20}, {0, 20, 300, 20}, {0, 40, 300, 20}},
		"a repetition with nothing to count")
}

// TestAutoFitCollapsesTheTracksNoItemLandedIn is the other half of §7.2.3.2,
// and the difference between the two keywords is a difference an author sees at
// once: "auto-fill" leaves the empty columns standing, so three cards in a row
// with room for five stay a fifth of the way across; "auto-fit" collapses them,
// and the three cards share the whole width.
func TestAutoFitCollapsesTheTracksNoItemLandedIn(t *testing.T) {
	const three = `<div id="g"><div>a</div><div>b</div><div>c</div></div>`

	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fill, minmax(60px, 1fr)) }`),
		[][4]float64{{0, 0, 60, 20}, {60, 0, 60, 20}, {120, 0, 60, 20}},
		"five columns with three items in them")

	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fit, minmax(60px, 1fr)) }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}, {200, 0, 100, 20}},
		"the same five with the empty ones collapsed")

	// Nothing is collapsed where nothing is empty: six items in five columns
	// fill them and start a second row.
	const six = `<div id="g"><div>a</div><div>b</div><div>c</div>` +
		`<div>d</div><div>e</div><div>f</div></div>`
	wantCells(t, gridCells(t, six,
		`#g { width: 300px; grid-template-columns: repeat(auto-fit, minmax(60px, 1fr)) }`),
		[][4]float64{
			{0, 0, 60, 20}, {60, 0, 60, 20}, {120, 0, 60, 20},
			{180, 0, 60, 20}, {240, 0, 60, 20}, {0, 20, 60, 20},
		}, "six items in five columns")
}

// The three-item fixture with names, for the placement tests: which item ended
// up where is the whole question, and the widths are all the same so that only
// the positions differ.
const threeCells = `<div id="g"><div id="a">a</div><div id="b">b</div><div id="c">c</div></div>`

const threeColumns = `#g { width: 300px; grid-template-columns: 100px 100px 100px }`

// TestAnItemGoesToTheLineItNamed is §8.3 and the first two steps of §8.5: an
// item that named a line is placed there, and the automatic flow deals the rest
// into what is left — around it, not over it.
func TestAnItemGoesToTheLineItNamed(t *testing.T) {
	// The first item asks for the second column. The other two are dealt from
	// the start, so one lands in front of it.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column: 2 }`),
		[][4]float64{{100, 0, 100, 20}, {0, 0, 100, 20}, {200, 0, 100, 20}},
		"one item in the second column")

	// A line for the row instead, which the flow then works around: the item is
	// on the second row and the two after it fill the first.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-row: 2 }`),
		[][4]float64{{0, 20, 100, 20}, {0, 0, 100, 20}, {100, 0, 100, 20}},
		"one item in the second row")

	// Both lines, which is a cell.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-row: 2; grid-column: 3 }`),
		[][4]float64{{200, 20, 100, 20}, {0, 0, 100, 20}, {100, 0, 100, 20}},
		"one item in a named cell")

	// The longhands say the same thing, and an end line with no start is one
	// track wide ending there — line 3 is the far edge of the second column.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column-start: 3 }`),
		[][4]float64{{200, 0, 100, 20}, {0, 0, 100, 20}, {100, 0, 100, 20}},
		"an item that named only its start")
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column-end: 3 }`),
		[][4]float64{{100, 0, 100, 20}, {0, 0, 100, 20}, {200, 0, 100, 20}},
		"an item that named only its end")
}

// TestAnItemSpansTheTracksItWasGiven. A span is a cell that covers more than
// one track, and it covers the gaps between them too — they separate the tracks
// from each other and there is nothing between a track and itself.
func TestAnItemSpansTheTracksItWasGiven(t *testing.T) {
	// Two lines apart is two tracks wide.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column: 2 / 4 }`),
		[][4]float64{{100, 0, 200, 20}, {0, 0, 100, 20}, {0, 20, 100, 20}},
		"an item across two columns")

	// A pair of lines the wrong way round is the same pair: §8.3 swaps them
	// rather than throwing the declaration out.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column: 3 / 1 }`),
		[][4]float64{{0, 0, 200, 20}, {200, 0, 100, 20}, {0, 20, 100, 20}},
		"an item between two lines written backwards")

	// "span 2" is the same width from wherever the flow puts it.
	wantCells(t, gridCells(t, threeCells, threeColumns+`#a { grid-column: span 2 }`),
		[][4]float64{{0, 0, 200, 20}, {200, 0, 100, 20}, {0, 20, 100, 20}},
		"an item spanning two columns from the flow")

	// The gaps between the spanned tracks belong to the item: two 90px columns
	// and the 20px between them.
	wantCells(t, gridCells(t, threeCells,
		`#g { width: 300px; grid-template-columns: 90px 90px 90px; column-gap: 20px }`+
			`#a { grid-column: 1 / 3 }`),
		[][4]float64{{0, 0, 200, 20}, {220, 0, 90, 20}, {0, 20, 90, 20}},
		"an item across two columns and the gap between them")
}

// TestASpanningItemAsksTheTracksItCoversTogether is §12.5's other half, and the
// reason it is a second pass: an item across two columns that needs 96px says
// nothing about either column on its own — any pair adding to 96 would hold it
// — so the columns are sized from the items *inside* them first, and only what
// is still missing is shared out between them.
func TestASpanningItemAsksTheTracksItCoversTogether(t *testing.T) {
	const wide = `<div id="g"><div id="a">aaaaaaaa</div><div id="b">b</div><div id="c">c</div></div>`

	// Two max-content columns: one character each from the items in them, then
	// 72px short of the eight-character item across both, shared equally.
	wantCells(t, gridCells(t, wide,
		`#g { width: 400px; grid-template-columns: max-content max-content }`+
			`#a { grid-column: 1 / 3 }`),
		[][4]float64{{0, 0, 96, 20}, {0, 20, 48, 20}, {48, 20, 48, 20}},
		"a spanning item over two content columns")

	// The gaps between the spanned tracks are room the item already has, so
	// they come off the shortfall: 12 and 12 of content with 20 between them is
	// 44 of the 96 it needs, and the 52 missing is shared.
	wantCells(t, gridCells(t, wide,
		`#g { width: 400px; grid-template-columns: max-content max-content; column-gap: 20px }`+
			`#a { grid-column: 1 / 3 }`),
		[][4]float64{{0, 0, 96, 20}, {0, 20, 38, 20}, {58, 20, 38, 20}},
		"a spanning item over two content columns with a gap")

	// A fixed column takes no share of it: the stylesheet said 30px and an item
	// spanning it does not overrule that, so the whole shortfall goes to the
	// automatic column beside it — which then stretches into the room left over.
	wantCells(t, gridCells(t, wide,
		`#g { width: 400px; grid-template-columns: 30px auto } #a { grid-column: 1 / 3 }`),
		[][4]float64{{0, 0, 400, 20}, {0, 20, 30, 20}, {30, 20, 370, 20}},
		"a spanning item over a fixed column and an automatic one")

	// And down the rows: three lines of text across two rows of one line each
	// makes both rows taller, by half the shortfall each.
	const tall = `<div id="g"><div id="a">a<br>b<br>c</div><div id="d">d</div><div id="e">e</div></div>`
	wantCells(t, gridCells(t, tall,
		`#g { width: 300px; grid-template-columns: 100px 100px } #a { grid-row: 1 / 3 }`),
		[][4]float64{{0, 0, 100, 60}, {100, 0, 100, 30}, {100, 30, 100, 30}},
		"a spanning item down two rows")
}

// TestAnItemThatNamesALineThisEngineCannotFindIsRefused. What is left out of the
// grammar is what needs a name: a line called something, an area, and a number
// counted back from the end of the grid — which is a real value and needs the
// far edge of a grid that is still being worked out.
func TestAnItemThatNamesALineThisEngineCannotFindIsRefused(t *testing.T) {
	for _, css := range []string{
		`#a { grid-column: main }`,
		`#a { grid-column: 1 / main }`,
		`#a { grid-column: -1 }`,
		`#a { grid-row: span main }`,
		`#a { grid-area: header }`,
		`#a { grid-column: 1 / 2 / 3 }`,
	} {
		got := Compose(Input{HTML: threeCells, CSS: []Stylesheet{{
			Source: gridCSS + threeColumns + css}}}, Options{})
		said := false
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "grid container") {
				said = true
			}
		}
		if !said {
			t.Errorf("nothing was reported about %q, so an item placed somewhere "+
				"this engine cannot find says nothing about it", css)
		}
	}
}

// The three-item fixture for the area tests, named so that each item can be put
// somewhere by name.
const threeAreas = `<div id="g"><div id="h">h</div><div id="n">n</div><div id="m">m</div></div>`

// TestATemplateOfAreasDrawsTheGrid is §7.3, which is the other way a grid is
// authored: the container draws a picture of itself, one string per row and one
// word per cell, and the items say which part of the picture they are in and
// never count a line.
//
// The picture also *makes* the explicit grid. Two words per row is two columns
// whether or not grid-template-columns named them.
func TestATemplateOfAreasDrawsTheGrid(t *testing.T) {
	const named = `#h { grid-area: head } #n { grid-area: nav } #m { grid-area: main }`

	// A header across the top of two columns, and two items under it. Nothing
	// said how wide the columns are, so they are automatic and share the
	// container.
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-template-areas: "head head" "nav main" }`+named),
		[][4]float64{{0, 0, 300, 20}, {0, 20, 150, 20}, {150, 20, 150, 20}},
		"a header across two columns")

	// The same picture with the columns named: the areas say where, the tracks
	// say how wide.
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-template-columns: 100px 200px;`+
			` grid-template-areas: "head head" "nav main" }`+named),
		[][4]float64{{0, 0, 300, 20}, {0, 20, 100, 20}, {100, 20, 200, 20}},
		"a template beside a set of tracks")

	// The tracks a template draws are *explicit* ones with no size of their own,
	// which is "auto" — not implicit tracks, so grid-auto-columns has nothing
	// to say about them.
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-auto-columns: 50px;`+
			` grid-template-areas: "head head" "nav main" }`+named),
		[][4]float64{{0, 0, 300, 20}, {0, 20, 150, 20}, {150, 20, 150, 20}},
		"a template beside a size for the tracks it did not draw")

	// A dot is a cell nobody named, and it is not an area: the header covers
	// one column and the second is left empty.
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-template-areas: "head ." "nav main" }`+named),
		[][4]float64{{0, 0, 150, 20}, {0, 20, 150, 20}, {150, 20, 150, 20}},
		"a template with an empty cell")

	// A run of dots is the same empty cell as one dot, which is how §7.3 lets a
	// template be lined up in the stylesheet.
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-template-areas: "head ..." "nav  main" }`+named),
		[][4]float64{{0, 0, 150, 20}, {0, 20, 150, 20}, {150, 20, 150, 20}},
		"a template lined up with dots")
}

// TestAnAreaCoversTheRectangleItWasDrawnIn. An area is a band of tracks on each
// axis, so a name written over two rows and two columns is one item across four
// cells.
func TestAnAreaCoversTheRectangleItWasDrawnIn(t *testing.T) {
	const doc = `<div id="g"><div id="h">h</div><div id="n">n</div></div>`
	wantCells(t, gridCells(t, doc,
		`#g { width: 300px; grid-template-columns: 100px 100px 100px;`+
			` grid-template-rows: 30px 30px;`+
			` grid-template-areas: "big big side" "big big side" }`+
			`#h { grid-area: big } #n { grid-area: side }`),
		[][4]float64{{0, 0, 200, 60}, {200, 0, 100, 60}},
		"an area two rows deep and two columns wide")
}

// TestGridAreaAlsoWritesFourLines. §8.4's other form: the shorthand is the four
// lines written together, in the order row-start, column-start, row-end,
// column-end — the block axis first, as everything in Box Alignment is, and not
// the reading order the slashes suggest.
func TestGridAreaAlsoWritesFourLines(t *testing.T) {
	wantCells(t, gridCells(t, threeAreas,
		`#g { width: 300px; grid-template-columns: 100px 100px }`+
			`#h { grid-area: 1 / 2 / 3 / 3 }`),
		[][4]float64{{100, 0, 100, 40}, {0, 0, 100, 20}, {0, 20, 100, 20}},
		"an item placed by four lines")
}

// TestATemplateThatDoesNotDrawAGridIsRefused. The two ways §7.3 says a template
// is invalid are the two this refuses, and refusing is the whole of the answer:
// a template that does not describe a grid describes nothing, and guessing at
// what was meant would put boxes somewhere no stylesheet asked for.
func TestATemplateThatDoesNotDrawAGridIsRefused(t *testing.T) {
	for _, css := range []string{
		// Rows of different lengths.
		`#g { grid-template-areas: "a b" "c" }`,
		// A name in two places with a hole between them.
		`#g { grid-template-areas: "a b" "b a" }`,
		// The same, in one row.
		`#g { grid-template-areas: "a b a" }`,
		// A name that is not one.
		`#g { grid-template-areas: "1a b" }`,
		// Something that is not a string at all.
		`#g { grid-template-areas: a b }`,
	} {
		got := Compose(Input{HTML: threeAreas, CSS: []Stylesheet{{
			Source: gridCSS + `#g { width: 300px }` + css}}}, Options{})
		said := false
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "grid container") {
				said = true
			}
		}
		if !said {
			t.Errorf("nothing was reported about %q, so a template that draws no "+
				"grid says nothing about it", css)
		}
	}
}

// TestATemplateMakesEveryRowItDraws, including one no item landed in. §7.3's
// picture is the explicit grid, so a template of two rows has two rows whether
// or not both are filled — and the gap between them is real, which is the one
// place an empty row is visible: the container is as deep as the row that is
// there, the gap, and the row that is empty.
func TestATemplateMakesEveryRowItDraws(t *testing.T) {
	const one = `<div id="g"><div id="h">h</div></div>`
	const drawn = `#g { width: 300px; grid-template-areas: "a" "b"; row-gap: 10px }` +
		`#h { grid-area: a }`

	got, ok := gridRect(t, one, gridCSS+drawn, "g")
	if !ok {
		t.Fatalf("the grid container generated no fragment")
	}
	if got.H.Px() != 30 {
		t.Errorf("the container is %vpx deep; it holds a 20px row, a 10px gap "+
			"and a row the template drew and nobody filled", got.H)
	}

	// The same picture with only the row that is used: 20px and no gap.
	got, _ = gridRect(t, one,
		gridCSS+`#g { width: 300px; grid-template-areas: "a"; row-gap: 10px } #h { grid-area: a }`,
		"g")
	if got.H.Px() != 20 {
		t.Errorf("the container is %vpx deep and its template draws one row", got.H)
	}
}

// TestTheGridGrowsToHoldAnItemPlacedPastIt is §7.5's implicit grid. An item that
// named a column past the last one drawn is not left hanging off the end: the
// grid grows to hold it, and the tracks it grew by are sized by
// grid-auto-columns.
//
// Before this the item was placed at the far edge of the explicit grid with no
// width at all — a box in the document and a sliver on the page.
func TestTheGridGrowsToHoldAnItemPlacedPastIt(t *testing.T) {
	const two = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`

	// Two explicit columns and an item in the fourth: the two implicit columns
	// are 40px each because that is what the stylesheet asked for.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: 100px 100px; grid-auto-columns: 40px }`+
			`#a { grid-column: 4 }`),
		[][4]float64{{240, 0, 40, 20}, {0, 0, 100, 20}}, "an item in an implicit column")

	// A span wider than the grid grows it too: four columns for an item that
	// asked for four, and the two implicit ones share what is left.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: 100px 100px } #a { grid-column: span 4 }`),
		[][4]float64{{0, 0, 300, 20}, {0, 20, 100, 20}}, "an item spanning past the grid")
}

// TestGridAutoRowsSizesTheRowsNobodyDrew is the other half of §7.5, and the one
// a card grid uses: the rows are made as the items need them, and
// grid-auto-rows says how tall each is.
func TestGridAutoRowsSizesTheRowsNobodyDrew(t *testing.T) {
	const two = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`

	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: 100px 100px; grid-auto-rows: 50px }`),
		[][4]float64{{0, 0, 100, 50}, {100, 0, 100, 50}}, "a row given a height of its own")

	// A list of sizes is taken in turn and started again at the end, which is
	// how a grid gives its rows alternating heights without drawing any of them.
	const four = `<div id="g"><div>a</div><div>b</div><div>c</div><div>d</div></div>`
	wantCells(t, gridCells(t, four,
		`#g { width: 300px; grid-template-columns: 100px; grid-auto-rows: 50px 30px }`),
		[][4]float64{
			{0, 0, 100, 50}, {0, 50, 100, 30},
			{0, 80, 100, 50}, {0, 130, 100, 30},
		}, "four rows of two alternating heights")

	// The list is counted from the first row *nobody drew*, not from the top of
	// the grid: a drawn row is the size it was drawn at, and the list starts
	// after it.
	const three = `<div id="g"><div>a</div><div>b</div><div>c</div></div>`
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: 100px; grid-template-rows: 20px;`+
			` grid-auto-rows: 50px 30px }`),
		[][4]float64{{0, 0, 100, 20}, {0, 20, 100, 50}, {0, 70, 100, 30}},
		"a drawn row and two undrawn ones")

	// The initial value is "auto", which is what makes an undrawn row as tall
	// as what lands in it.
	wantCells(t, gridCells(t, two,
		`#g { width: 300px; grid-template-columns: 100px 100px; grid-auto-rows: auto }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}}, "a row as tall as its content")
}

// TestARightToLeftGridStartsItsColumnsOnTheRight. A grid's rows run down the
// page whatever the writing mode is, and its columns run from whichever side
// the text starts at — so under "rtl" the first column is the rightmost one and
// every position across the container is measured from that edge.
//
// It is one mirror at the end rather than a reversal threaded through the
// sizing, for the reason layout/flex.go gives: every position on the axis turns
// together, so turning them one at a time is the same answer arrived at more
// ways than it needs.
func TestARightToLeftGridStartsItsColumnsOnTheRight(t *testing.T) {
	const columns = `#g { width: 300px; grid-template-columns: 100px 100px }`

	wantCells(t, gridCells(t, threeCells, columns),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}, {0, 20, 100, 20}},
		"three items in two columns")

	wantCells(t, gridCells(t, threeCells, columns+`#g { direction: rtl }`),
		[][4]float64{{200, 0, 100, 20}, {100, 0, 100, 20}, {200, 20, 100, 20}},
		"the same three from the right")

	// A named line counts from the same end: the second column of a
	// right-to-left grid is the second from the right.
	wantCells(t, gridCells(t, threeCells, columns+`#g { direction: rtl } #a { grid-column: 2 }`),
		[][4]float64{{100, 0, 100, 20}, {200, 0, 100, 20}, {200, 20, 100, 20}},
		"an item in the second column of a right-to-left grid")
}

// TestTheKeywordsTurnWithTheColumns. §6.2 again, one specification down: "start"
// is where the columns begin, which is the right edge here; "left" is the left
// edge whatever the direction is, and in a right-to-left grid that is where
// they end.
func TestTheKeywordsTurnWithTheColumns(t *testing.T) {
	const rtl = `#g { width: 300px; grid-template-columns: 100px 100px; direction: rtl }`

	// The tracks packed at the end of the axis, which is the left of the page.
	wantCells(t, gridCells(t, threeCells, rtl+`#g { justify-content: end }`),
		[][4]float64{{100, 0, 100, 20}, {0, 0, 100, 20}, {100, 20, 100, 20}},
		"tracks packed at the end")

	// "left" names the same place by the other route.
	wantCells(t, gridCells(t, threeCells, rtl+`#g { justify-content: left }`),
		[][4]float64{{100, 0, 100, 20}, {0, 0, 100, 20}, {100, 20, 100, 20}},
		"tracks packed at the left")

	// And an item at the start of its cell is against the cell's right edge.
	wantCells(t, gridCells(t, threeCells, rtl+`#g { justify-items: start }`),
		[][4]float64{{288, 0, 12, 20}, {188, 0, 12, 20}, {288, 20, 12, 20}},
		"items at the start of their cells")

	// "left" on an item is the left of the page, which is the far end of its
	// cell here — the same distinction one line up, one property down.
	wantCells(t, gridCells(t, threeCells, rtl+`#g { justify-items: left }`),
		[][4]float64{{200, 0, 12, 20}, {100, 0, 12, 20}, {200, 20, 12, 20}},
		"items at the left of their cells")
}

// TestAColumnFlowDealsDownTheColumns is §8.5 with the axes exchanged: the items
// fill a column to the bottom and start the next, and the tracks the grid grows
// by are columns rather than rows.
//
// The rows are what the template drew, for the reason the columns are in a row
// flow: the axis the items are dealt *along* is the one that has to be as long
// as it was written, and the other grows as far as the items reach.
func TestAColumnFlowDealsDownTheColumns(t *testing.T) {
	const four = `<div id="g"><div>a</div><div>b</div><div>c</div><div>d</div></div>`

	// Two rows drawn, so two items to a column and two columns of them.
	wantCells(t, gridCells(t, four,
		`#g { width: 300px; grid-template-rows: 20px 20px; grid-auto-flow: column }`),
		[][4]float64{
			{0, 0, 150, 20}, {0, 20, 150, 20},
			{150, 0, 150, 20}, {150, 20, 150, 20},
		}, "four items down two columns")

	// The columns the grid grew by are implicit, so grid-auto-columns sizes
	// them — all of them, because nothing drew any.
	wantCells(t, gridCells(t, four,
		`#g { width: 300px; grid-template-rows: 20px 20px; grid-auto-flow: column;`+
			` grid-auto-columns: 50px }`),
		[][4]float64{
			{0, 0, 50, 20}, {0, 20, 50, 20},
			{50, 0, 50, 20}, {50, 20, 50, 20},
		}, "implicit columns given a width")

	// With no rows drawn there is one, and the items run across it. That row is
	// implicit too, so grid-auto-rows gives it its height.
	wantCells(t, gridCells(t, four, `#g { width: 300px; grid-auto-flow: column }`),
		[][4]float64{
			{0, 0, 75, 20}, {75, 0, 75, 20}, {150, 0, 75, 20}, {225, 0, 75, 20},
		}, "four items along one row")
	wantCells(t, gridCells(t, four,
		`#g { width: 300px; grid-auto-flow: column; grid-auto-rows: 50px }`),
		[][4]float64{
			{0, 0, 75, 50}, {75, 0, 75, 50}, {150, 0, 75, 50}, {225, 0, 75, 50},
		}, "one implicit row given a height")

	// An item that named a row past the end of a column flow grows the grid
	// down before anything is dealt, because the rows are the axis the flow
	// fills along and it has to know how long they are.
	wantCells(t, gridCells(t, threeCells, `#g { width: 300px; grid-auto-flow: column } #a { grid-row: 3 }`),
		[][4]float64{{0, 40, 300, 20}, {0, 0, 300, 20}, {0, 20, 300, 20}},
		"a column flow with an item in the third row")
}

// TestADenseFlowGoesBackForTheHoles. The cursor of a sparse flow never goes
// back, so an item too wide for what is left of a row leaves a hole that stays
// empty. "dense" is the other answer: every item is placed from the start of
// the grid, so a later, narrower one fills it.
//
// The two are a trade and the default says which way: sparse keeps the items in
// the order they were written, dense keeps the grid whole.
func TestADenseFlowGoesBackForTheHoles(t *testing.T) {
	const wide = `#g { width: 300px; grid-template-columns: 100px 100px 100px }` +
		`#a { grid-column: span 2 } #b { grid-column: span 2 }`

	// Sparse: the third item goes after the second, leaving the top right
	// corner empty.
	wantCells(t, gridCells(t, threeCells, wide),
		[][4]float64{{0, 0, 200, 20}, {0, 20, 200, 20}, {200, 20, 100, 20}},
		"a sparse flow leaving a hole")

	// Dense: it goes back and fills it.
	wantCells(t, gridCells(t, threeCells, wide+`#g { grid-auto-flow: row dense }`),
		[][4]float64{{0, 0, 200, 20}, {0, 20, 200, 20}, {200, 0, 100, 20}},
		"a dense flow filling it")
}
