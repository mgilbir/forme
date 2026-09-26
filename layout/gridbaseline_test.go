package layout

import (
	"strings"
	"testing"
)

// Grid items aligned by their baselines down a row, which the gate refused.
//
// Box Alignment §9.2: grid items in the same row share an alignment context
// down the grid, and those with the same baseline alignment preference form a
// baseline-sharing group; an item spanning several rows is in the group of its
// first row for a first baseline and of its last row for a last. §9.3 lines
// their baselines up and places the group by its fallback alignment, start for
// "first baseline" and end for "last baseline". CSS Grid 2 §12.5's step 1 sizes
// the rows with that in them: each item is "shimmed" on its start or end side
// by as much as it is moved, so a row holds the group as aligned and not the
// items as they would be at the start.
//
// Courier at 20px, one line 20px high. The baseline of a line is at the same
// depth in every item, so padding is what makes two baselines differ, and every
// expected position below is the padding's arithmetic.

// TestItemsInARowShareTheirFirstBaseline. "a" has 10px of padding above its
// line and "b" none, so b is moved down 10 to meet it. b also has 15px of
// padding below, and the row holds it where it was moved to: 10 + 20 + 15 is
// 45, where the items at the top of the row made 35 and b hung out of it.
func TestItemsInARowShareTheirFirstBaseline(t *testing.T) {
	const doc = `<div id="g"><div style="padding-top: 10px">a</div>` +
		`<div style="padding-bottom: 15px">b</div><div>c</div></div>`
	for _, css := range []string{
		`#g { width: 200px; grid-template-columns: 100px 100px; align-items: baseline }`,
		`#g { width: 200px; grid-template-columns: 100px 100px; align-items: first baseline }`,
		`#g { width: 200px; grid-template-columns: 100px 100px; align-items: baseline first }`,
		`#g { width: 200px; grid-template-columns: 100px 100px }` +
			`#g > div { align-self: baseline }`,
	} {
		arrangedWithoutFinding(t, doc, css)
		wantCells(t, gridCells(t, doc, css), [][4]float64{
			{0, 0, 100, 30}, {100, 10, 100, 35}, {0, 45, 100, 20},
		}, css)
	}
}

// TestItemsInARowShareTheirLastBaseline. "a<br>b" is two lines and "c" one
// with 10px of padding below it, so the last lines' baselines are 20 and 30
// above the bottoms of the two boxes. The group is placed at the end of the
// row, and the first item is shimmed up by 10 to meet the second: the row is
// 40 + 10 = 50, the first item at its top and the second 20 down.
func TestItemsInARowShareTheirLastBaseline(t *testing.T) {
	const doc = `<div id="g"><div>a<br>b</div><div style="padding-bottom: 10px">c</div></div>`
	for _, css := range []string{
		`#g { width: 200px; grid-template-columns: 100px 100px; align-items: last baseline }`,
		`#g { width: 200px; grid-template-columns: 100px 100px; align-items: baseline last }`,
	} {
		arrangedWithoutFinding(t, doc, css)
		wantCells(t, gridCells(t, doc, css), [][4]float64{
			{0, 0, 100, 40}, {100, 20, 100, 30},
		}, css)
	}
}

// TestFirstAndLastBaselinesAreTwoGroups. The preference is part of the group:
// an item aligned by its first baseline does not share with one aligned by its
// last, even in the same row. "a" (first, 10px above) and "b" (first) are a
// group; "c<br>d" (last) is alone and sits at the end of the row. The row is
// the first group's 30 and the lone item's 40, and the group is at its top.
func TestFirstAndLastBaselinesAreTwoGroups(t *testing.T) {
	const doc = `<div id="g"><div style="padding-top: 10px">a</div><div>b</div>` +
		`<div style="align-self: last baseline">c<br>d</div></div>`
	css := `#g { width: 300px; grid-template-columns: 100px 100px 100px; align-items: baseline }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css), [][4]float64{
		{0, 0, 100, 30}, {100, 10, 100, 20}, {200, 0, 100, 40},
	}, css)
}

// TestASpanningItemSharesTheBaselineOfItsFirstRow. An item across two rows
// takes part in first-baseline alignment "within its start-most shared
// alignment context", its first row. Its 10px of padding above puts "b", in
// that row, 10 down; "c", in the second row, is not in the group.
func TestASpanningItemSharesTheBaselineOfItsFirstRow(t *testing.T) {
	const doc = `<div id="g"><div style="grid-row: span 2; padding-top: 10px">a<br>a<br>a</div>` +
		`<div>b</div><div>c</div></div>`
	css := `#g { width: 200px; grid-template-columns: 100px 100px; align-items: baseline }`
	arrangedWithoutFinding(t, doc, css)
	// The first row is b and its shim, 30, and the second c's 20; the span
	// needs 70, and the 20 more is shared between them, so c's row begins at
	// 40.
	wantCells(t, gridCells(t, doc, css), [][4]float64{
		{0, 0, 100, 70}, {100, 10, 100, 20}, {100, 40, 100, 20},
	}, css)
}

// TestAnItemWhoseHeightDependsOnItsRowFallsBack. Grid §11.4: an item whose size
// in the axis depends on an intrinsically sized track would make a cycle, so it
// "does not participate in baseline alignment, and instead uses its fallback
// alignment". "height: 50%" of an automatic row is such an item. It is 30px (a
// line and 10px of padding below, the percentage coming to nothing), and its
// fallback is "safe self-end": at the end of a row that the other item, alone
// in its group, makes 40. Had it taken part, the other item would have been
// shimmed 10 up to meet its baseline and the row made 50.
func TestAnItemWhoseHeightDependsOnItsRowFallsBack(t *testing.T) {
	const doc = `<div id="g"><div style="height: 50%; padding-bottom: 10px">a</div>` +
		`<div>b<br>c</div></div>`
	css := `#g { width: 200px; grid-template-columns: 100px 100px; align-items: last baseline }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css), [][4]float64{
		{0, 10, 100, 30}, {100, 0, 100, 40},
	}, css)
	// In a row of a stated height the percentage is of something known, and
	// the item takes part: 50% of 60 is 30, its border box 40 with the
	// padding, its last baseline 20 above its bottom and the other item's 0.
	// So the other is shimmed 20 up from the end of the row, to its top, and
	// the first is at the end.
	css = `#g { width: 200px; grid-template-columns: 100px 100px; grid-template-rows: 60px;` +
		` align-items: last baseline }`
	wantCells(t, gridCells(t, doc, css), [][4]float64{
		{0, 20, 100, 40}, {100, 0, 100, 40},
	}, css)
}

// TestALastBaselineIsASafeAlignment. The fallback alignment of a last
// baseline is "safe self-end", and the group is placed by it: two lines in a
// 10px row do not rise out of the top of it, as "end" alone would put them.
func TestALastBaselineIsASafeAlignment(t *testing.T) {
	const doc = `<div id="g"><div>a<br>b</div></div>`
	css := `#g { width: 100px; grid-template-rows: 10px; align-items: last baseline }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css), [][4]float64{{0, 0, 100, 40}}, css)
}

// TestAnItemWithNoLineHasABaselineFromItsBorderBox. Box Alignment §9.1: a box
// with no baseline set has one synthesized, and a grid item from its border
// edges — the alphabetic baseline from the line-under edge, the bottom. An
// empty 30px box and a line of text share a first baseline, so the line is
// moved down until its baseline is level with the box's bottom.
func TestAnItemWithNoLineHasABaselineFromItsBorderBox(t *testing.T) {
	const doc = `<div id="g"><div style="height: 30px"></div><div>b</div></div>`
	css := `#g { width: 200px; grid-template-columns: 100px 100px; align-items: baseline }`
	got := gridCells(t, doc, css)
	root := layoutOf(t, 1000, doc, gridCSS+css)
	g := fragmentFor(root, "g")
	b := g.Children[1]
	base, ok := firstBaseline(b)
	if !ok {
		t.Fatal("the line has no baseline")
	}
	if at := b.BorderRect.Y.Add(base).Px(); at != 30 {
		t.Errorf("the line's baseline is at %v, want 30, the bottom of the empty box: %v", at, got)
	}
}

// TestAGridsBaselineIsItsItemsSharedBaseline is Grid §11.6's first step: "If
// any grid items intersecting the first ... non-empty track participate in
// first baseline alignment ..., generate a baseline set from their shared
// alignment baseline". The first item is aligned to the start and has its
// baseline at its line; the other two share theirs, 20px lower. The grid's
// baseline is theirs, where it was the first item's.
func TestAGridsBaselineIsItsItemsSharedBaseline(t *testing.T) {
	const doc = `<div id="g"><div style="align-self: start">a</div>` +
		`<div style="padding-top: 20px">b</div><div>c</div></div>`
	css := `#g { width: 300px; grid-template-columns: 100px 100px 100px; align-items: baseline }`
	g := fragmentFor(layoutOf(t, 1000, doc, gridCSS+css), "g")
	got, ok := containerFirstBaseline(g)
	if !ok {
		t.Fatal("the grid has no baseline")
	}
	c := g.Children[2]
	line, _ := firstBaseline(c)
	if want := g.Border.Top.Add(g.Padding.Top).Add(c.BorderRect.Y).Add(line); got != want {
		t.Errorf("the grid's baseline is %v, want the shared one at %v", got.Px(), want.Px())
	}
	first, _ := firstBaseline(g.Children[0])
	if got == first {
		t.Errorf("the grid's baseline is the first item's, %v", first.Px())
	}

	// The second step: no item of the first row takes part in first-baseline
	// alignment, and one takes part in last-baseline alignment, whose last
	// baseline is the grid's.
	const last = `<div id="g"><div style="align-self: start">a</div>` +
		`<div style="align-self: last baseline">b<br>c</div></div>`
	g = fragmentFor(layoutOf(t, 1000, last, gridCSS+
		`#g { width: 200px; grid-template-columns: 100px 100px; grid-template-rows: 80px }`), "g")
	got, _ = containerFirstBaseline(g)
	c = g.Children[1]
	line, _ = lastLineBaseline(c)
	if want := g.Border.Top.Add(g.Padding.Top).Add(c.BorderRect.Y).Add(line); got != want {
		t.Errorf("the grid's baseline is %v, want the last-baseline item's at %v",
			got.Px(), want.Px())
	}
}

// TestAnAbsolutelyPositionedChildOnABaselineIsAlignedByTheFallback. A box out
// of flow shares no alignment context, and Box Alignment §4.2 aligns it by the
// fallback: the start of its static-position rectangle for a first baseline,
// the end for a last.
func TestAnAbsolutelyPositionedChildOnABaselineIsAlignedByTheFallback(t *testing.T) {
	grid := func(self string) string {
		return `<div style="display: grid; position: relative; width: 300px; height: 100px">` +
			`<div>a</div><div id="p" style="position: absolute; ` + self + `">x</div></div>`
	}
	if r := fcRect(t, grid("align-self: baseline"), ``, "p"); r[1] != 0 {
		t.Errorf("first baseline: the box is at y=%v, want 0", r[1])
	}
	if r := fcRect(t, grid("align-self: last baseline"), ``, "p"); r[1] != 80 {
		t.Errorf("last baseline: the box is at y=%v, want 80", r[1])
	}
}

// TestABaselineThisEngineDoesNotFindIsRefused. What is left refused of
// baseline alignment, each with a finding: an item in a vertical writing mode
// aligned down the rows, whose own lines run across them.
func TestABaselineThisEngineDoesNotFindIsRefused(t *testing.T) {
	for _, css := range []string{
		`#g > div:first-child { align-self: last baseline; writing-mode: vertical-lr }`,
		`#g { align-items: baseline } #g > div:first-child { writing-mode: vertical-rl }`,
	} {
		got := Compose(Input{HTML: fourItems, CSS: []Stylesheet{{Source: gridCSS +
			`#g { width: 300px; grid-template-columns: 1fr 1fr }` + css}}}, Options{})
		said := false
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "grid container") &&
				strings.Contains(f.Message, "vertical writing mode") {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: nothing said the grid was refused: %v", css, got.Findings)
		}
	}
}

// TestASpanningItemSharesTheLastBaselineOfItsLastRow. For a last baseline an
// item across two rows is in the group of the last row it spans. The spanning
// item's last line has 10px of padding under it, so "c", in its last row, is
// shimmed up by 10 from the end of that row: the rows are 20 (b) and 30 (c and
// its shim), which hold the 50px span exactly, and c's baseline meets the
// span's at 20 + the depth of a line. Grouped by the row it starts in, the
// spanning item was alone, c was at the end of a 25px row, 30 down.
func TestASpanningItemSharesTheLastBaselineOfItsLastRow(t *testing.T) {
	const doc = `<div id="g"><div style="grid-row: span 2; align-self: last baseline;` +
		` padding-bottom: 10px">a<br>a</div><div>b</div>` +
		`<div style="align-self: last baseline">c</div></div>`
	css := `#g { width: 200px; grid-template-columns: 100px 100px }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css), [][4]float64{
		{0, 0, 100, 50}, {100, 0, 100, 20}, {100, 20, 100, 20},
	}, css)
}

// TestAScrollingItemsLastBaselineIsItsBottomMarginEdge. Box Alignment §9.1's
// legacy rule: a block container that is a scroll container "always has a last
// baseline set, whose baselines all correspond to its block-end margin edge".
// So the line of "c", aligned by its last baseline with an "overflow: hidden"
// box, sits with its baseline on that box's bottom.
func TestAScrollingItemsLastBaselineIsItsBottomMarginEdge(t *testing.T) {
	const doc = `<div id="g"><div style="overflow: hidden">a<br>b</div><div>c</div></div>`
	css := `#g { width: 200px; grid-template-columns: 100px 100px; align-items: last baseline }`
	arrangedWithoutFinding(t, doc, css)
	g := fragmentFor(layoutOf(t, 1000, doc, gridCSS+css), "g")
	box, c := g.Children[0], g.Children[1]
	line, ok := firstBaseline(c)
	if !ok {
		t.Fatal("c has no baseline")
	}
	if got, want := c.BorderRect.Y.Add(line), box.BorderRect.Bottom(); got != want {
		t.Errorf("c's baseline is at %v, want the scrolling box's bottom at %v",
			got.Px(), want.Px())
	}
}
