package layout

import "testing"

// A flex container's own sizes reach its lines, and its items' own sizes reach
// what the container gives them.
//
// Flexbox §9 is written about a container whose size is what block layout says
// it is — min-height and max-height included — and about items whose sizes the
// container negotiates within their own limits. Both halves were missing
// somewhere, and in each case the page looked plausible: the container's box
// was clamped by block layout after the items had been arranged inside a size
// it did not have. The fixture is flexintrinsic_test.go's, 12px a character
// and 20px a line.

// fcRect is #id's border box as x, y, width, height.
func fcRect(t *testing.T, htmlSrc, cssSrc, id string) [4]float64 {
	t.Helper()
	root, _ := fcLayout(t, htmlSrc, cssSrc)
	r := find(t, root, id).BorderRect
	return [4]float64{r.X.Px(), r.Y.Px(), r.W.Px(), r.H.Px()}
}

func wantRect(t *testing.T, what string, got, want [4]float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: x=%g y=%g w=%g h=%g, want x=%g y=%g w=%g h=%g",
			what, got[0], got[1], got[2], got[3], want[0], want[1], want[2], want[3])
	}
}

// TestAColumnFillsItsMinimumHeight is audit C29's sticky footer. §9.2's step 4
// sizes the container by the rules of the context it is in, and a block's
// min-height is one of them: the main size is 300 and the "flex: 1" main takes
// what the header and footer leave.
func TestAColumnFillsItsMinimumHeight(t *testing.T) {
	doc := `<div style="display: flex; flex-direction: column; min-height: 300px; width: 100px">` +
		`<div>H</div><div id="m" style="flex: 1">M</div><div id="ft">F</div></div>`
	wantRect(t, "the main", fcRect(t, doc, ``, "m"), [4]float64{0, 20, 100, 260})
	wantRect(t, "the footer", fcRect(t, doc, ``, "ft"), [4]float64{0, 280, 100, 20})

	// A stated height below the minimum is the minimum.
	wantRect(t, "a height under a minimum",
		fcRect(t, `<div style="display: flex; flex-direction: column; height: 50px; min-height: 100px; width: 100px">`+
			`<div id="m" style="flex: 1">M</div></div>`, ``, "m"), [4]float64{0, 0, 100, 100})

	// justify-content has the room to place the item in.
	wantRect(t, "centred down a column at its minimum",
		fcRect(t, `<div style="display: flex; flex-direction: column; min-height: 200px; `+
			`justify-content: center; width: 100px"><div id="i">A</div></div>`, ``, "i"),
		[4]float64{0, 90, 100, 20})
}

// TestAColumnShrinksToItsMaximumHeight: the same step with max-height, which
// takes room away. Two 80px items in a column held to 100 shrink in
// proportion to their bases — 50 each.
func TestAColumnShrinksToItsMaximumHeight(t *testing.T) {
	doc := `<div style="display: flex; flex-direction: column; max-height: 100px; width: 100px">` +
		`<div id="a" style="height: 80px; min-height: 0">A</div>` +
		`<div id="b" style="height: 80px; min-height: 0">B</div></div>`
	wantRect(t, "the first", fcRect(t, doc, ``, "a"), [4]float64{0, 0, 100, 50})
	wantRect(t, "the second", fcRect(t, doc, ``, "b"), [4]float64{0, 50, 100, 50})
}

// TestARowsLineIsHeldBetweenItsContainersLimits is §9.4's step 8: a single line
// is clamped to the container's min and max cross sizes, so a row at
// "min-height: 200px" has a line 200 tall to stretch or align in.
func TestARowsLineIsHeldBetweenItsContainersLimits(t *testing.T) {
	row := func(css string) string {
		return `<div style="display: flex; width: 300px; ` + css + `"><div id="i">A</div></div>`
	}
	wantRect(t, "stretched to a minimum", fcRect(t, row("min-height: 200px"), ``, "i"),
		[4]float64{0, 0, 12, 200})
	wantRect(t, "centred in a minimum", fcRect(t, row("min-height: 200px; align-items: center"), ``, "i"),
		[4]float64{0, 90, 12, 20})
	wantRect(t, "at the end of a minimum", fcRect(t, row("min-height: 200px; align-items: flex-end"), ``, "i"),
		[4]float64{0, 180, 12, 20})
	wantRect(t, "stretched to a maximum", fcRect(t, row("max-height: 10px"), ``, "i"),
		[4]float64{0, 0, 12, 10})
	// A stated height is clamped as well.
	wantRect(t, "a height over a maximum", fcRect(t, row("height: 100px; max-height: 40px"), ``, "i"),
		[4]float64{0, 0, 12, 40})
}

// TestWrappedLinesShareAMinimumHeight is §9.4's step 15 and align-content's
// initial stretch: two lines of 20 in a row at "min-height: 300px" are two
// lines of 150.
func TestWrappedLinesShareAMinimumHeight(t *testing.T) {
	doc := `<div style="display: flex; flex-wrap: wrap; width: 20px; min-height: 300px">` +
		`<div id="a">A</div><div id="b">B</div></div>`
	wantRect(t, "the first line", fcRect(t, doc, ``, "a"), [4]float64{0, 0, 12, 150})
	wantRect(t, "the second line", fcRect(t, doc, ``, "b"), [4]float64{0, 150, 12, 150})
}

// TestAnAutoHeightColumnHoldsItsItemsContent is audit C37. A "flex: 1" pane
// that may shrink to nothing collapsed to 0px in a column with no height, and
// the next block was drawn over it. The shorthand's basis is "0%", as every
// browser writes it, and a percentage basis against an indefinite main size is
// "content" (§7.2.3) — so the pane's hypothetical size is its text, and the
// column, which is the sum of those, holds it.
func TestAnAutoHeightColumnHoldsItsItemsContent(t *testing.T) {
	for _, pane := range []string{"flex: 1; overflow: hidden", "flex: 1; min-height: 0",
		"flex: 1 1 0%; overflow: hidden", "flex-grow: 1; flex-basis: 50%; min-height: 0"} {
		doc := `<div style="display: flex; flex-direction: column"><div id="i" style="` + pane +
			`">a</div></div><div id="after">after</div>`
		if got := fcRect(t, doc, ``, "i"); got[3] != 20 {
			t.Errorf("%s: the pane is %gpx tall, want 20", pane, got[3])
		}
		if got := fcRect(t, doc, ``, "after"); got[1] != 20 {
			t.Errorf("%s: the next block is at y=%g, want 20", pane, got[1])
		}
	}

	// "content" and not "auto": a percentage basis that cannot resolve sets
	// the item's declared width aside. A row measured for a float has no
	// width to resolve 50% of, so the item asks for its text, not its 30px.
	wantWidth(t, "an indefinite percentage basis beside a width",
		fcWidth(t, `<div id="x" style="float: left; display: flex">`+
			`<div style="flex-basis: 50%; width: 30px">AAAA</div></div>`, ``, "x"), 48)

	// The column's height is the sum of its items' hypothetical main sizes, as
	// every browser has it: a basis in a column is a height the author wrote,
	// whether or not the item may shrink below it.
	wantRect(t, "a basis the item may shrink from",
		fcRect(t, `<div id="c" style="display: flex; flex-direction: column; width: 100px">`+
			`<div style="flex-basis: 100px">a</div></div>`, ``, "c"), [4]float64{0, 0, 100, 100})
	wantRect(t, "an inflexible basis",
		fcRect(t, `<div id="c" style="display: flex; flex-direction: column; width: 100px">`+
			`<div style="flex: 0 0 100px">a</div></div>`, ``, "c"), [4]float64{0, 0, 100, 100})
	// An explicit zero in pixels is zero, and "overflow: hidden" lets the item
	// be it: that is what the author wrote, and what browsers draw.
	wantRect(t, "flex: 1 1 0px with overflow: hidden",
		fcRect(t, `<div id="c" style="display: flex; flex-direction: column; width: 100px">`+
			`<div style="flex: 1 1 0px; overflow: hidden">a</div></div>`, ``, "c"), [4]float64{0, 0, 100, 0})
}

// TestFlexOneDistributesFromZeroInADefiniteColumn. The "0%" basis is 0% of a
// main size that is known, which is zero, so "flex: 1" still divides a column
// with a height into equal parts however much each part holds — the reason
// authors write the shorthand, and what "0%" must not take away.
func TestFlexOneDistributesFromZeroInADefiniteColumn(t *testing.T) {
	doc := `<div style="display: flex; flex-direction: column; height: 100px; width: 100px">` +
		`<div id="a" style="flex: 1">A</div><div id="b" style="flex: 1">B<br>B</div></div>`
	wantRect(t, "the first", fcRect(t, doc, ``, "a"), [4]float64{0, 0, 100, 50})
	wantRect(t, "the second", fcRect(t, doc, ``, "b"), [4]float64{0, 50, 100, 50})
	// And a row, whose main size is always definite.
	row := `<div style="display: flex; width: 300px"><div id="a" style="flex: 1">A</div>` +
		`<div id="b" style="flex: 1">BBBBBBBBBB</div></div>`
	wantRect(t, "the first in a row", fcRect(t, row, ``, "a"), [4]float64{0, 0, 150, 20})
	wantRect(t, "the second in a row", fcRect(t, row, ``, "b"), [4]float64{150, 0, 150, 20})
}

// TestAStretchedItemKeepsItsOwnLimits is audit C106. §9.4's stretch is "as for
// width: auto", and §9.8 says the stretched size is clamped to the item's own
// min and max cross size.
func TestAStretchedItemKeepsItsOwnLimits(t *testing.T) {
	col := func(item string) string {
		return `<div style="display: flex; flex-direction: column; width: 400px">` + item + `</div>`
	}
	wantRect(t, "max-width down a column",
		fcRect(t, col(`<div id="i" style="max-width: 50px">A</div>`), ``, "i"), [4]float64{0, 0, 50, 20})
	// The clamp comes before the item is measured: its height is what its
	// text comes to at the width it will have, two lines at 50px.
	wantRect(t, "max-width down a column, measured at that width",
		fcRect(t, col(`<div id="i" style="max-width: 50px">AAAA AAAA</div>`), ``, "i"), [4]float64{0, 0, 50, 40})
	wantRect(t, "min-width down a column",
		fcRect(t, col(`<div id="i" style="min-width: 500px">A</div>`), ``, "i"), [4]float64{0, 0, 500, 20})
	wantRect(t, "max-width with padding down a column",
		fcRect(t, col(`<div id="i" style="max-width: 50px; padding: 0 5px">A</div>`), ``, "i"),
		[4]float64{0, 0, 60, 20})
	wantRect(t, "a percentage max-width down a column",
		fcRect(t, col(`<div id="i" style="max-width: 25%">A</div>`), ``, "i"), [4]float64{0, 0, 100, 20})
	wantRect(t, "a keyword max-width down a column",
		fcRect(t, col(`<div id="i" style="max-width: max-content">AA</div>`), ``, "i"), [4]float64{0, 0, 24, 20})
	// A wrapping column stretches its items to their line after the lines are
	// found, and the limit holds there too.
	wantRect(t, "max-width in a wrapping column",
		fcRect(t, `<div style="display: flex; flex-direction: column; flex-wrap: wrap; width: 400px">`+
			`<div id="i" style="max-width: 5px">AAAA</div><div>BBBBBBBBBB</div></div>`, ``, "i"),
		[4]float64{0, 0, 5, 20})
	// Across a row the cross size is a height, which the item's own layout
	// holds; these say it still does.
	wantRect(t, "max-height across a row",
		fcRect(t, `<div style="display: flex; height: 100px"><div id="i" style="max-height: 30px">A</div></div>`,
			``, "i"), [4]float64{0, 0, 12, 30})
	wantRect(t, "min-height across a row",
		fcRect(t, `<div style="display: flex; height: 100px"><div id="i" style="min-height: 130px">A</div></div>`,
			``, "i"), [4]float64{0, 0, 12, 130})
}

// TestAStretchedGridItemKeepsItsOwnLimits is audit C106's grid half: §11's
// stretch is "as for width: auto" too.
func TestAStretchedGridItemKeepsItsOwnLimits(t *testing.T) {
	grid := func(template, item string) string {
		return `<div style="display: grid; grid-template-columns: ` + template +
			`; grid-template-rows: 100px">` + item + `</div>`
	}
	wantRect(t, "max-width in a wide column",
		fcRect(t, grid("400px", `<div id="i" style="max-width: 50px">A</div>`), ``, "i"),
		[4]float64{0, 0, 50, 100})
	wantRect(t, "min-width in a narrow column",
		fcRect(t, grid("40px", `<div id="i" style="min-width: 100px">A</div>`), ``, "i"),
		[4]float64{0, 0, 100, 100})
	wantRect(t, "a percentage max-width is of the area",
		fcRect(t, grid("400px", `<div id="i" style="max-width: 10%">A</div>`), ``, "i"),
		[4]float64{0, 0, 40, 100})
	// The block axis was already held by the item's own layout, which is handed
	// the stretched height; these say it still is.
	wantRect(t, "max-height in a tall row",
		fcRect(t, grid("400px", `<div id="i" style="max-height: 30px">A</div>`), ``, "i"),
		[4]float64{0, 0, 400, 30})
	wantRect(t, "min-height in a short row",
		fcRect(t, grid("400px", `<div id="i" style="min-height: 130px">A</div>`), ``, "i"),
		[4]float64{0, 0, 400, 130})
}

// TestATableIsAnItem is audit C38. A <table> in a flex or grid container is an
// item, and the item is §17.4's wrapper; css-tables-3 uses the item properties
// on the wrapper, which inherits and so had none of them.
func TestATableIsAnItem(t *testing.T) {
	table := func(style string) string {
		return `<table id="t" style="border-spacing: 0; ` + style + `"><tr><td style="padding: 0">T</td></tr></table>`
	}
	wantRect(t, "flex: 1",
		fcRect(t, `<div style="display: flex; width: 400px">`+table("flex: 1")+`<div>B</div></div>`, ``, "t"),
		[4]float64{0, 0, 388, 20})
	wantRect(t, "order: 2 puts it after the item",
		fcRect(t, `<div style="display: flex; width: 400px">`+table("order: 2")+`<div>BB</div></div>`, ``, "t"),
		[4]float64{24, 0, 12, 20})
	wantRect(t, "align-self: flex-end",
		fcRect(t, `<div style="display: flex; width: 400px; height: 100px">`+table("align-self: flex-end")+
			`</div>`, ``, "t"), [4]float64{0, 80, 12, 20})
	wantRect(t, "stretched across a column",
		fcRect(t, `<div style="display: flex; flex-direction: column; width: 400px">`+table("")+
			`</div>`, ``, "t"), [4]float64{0, 0, 400, 20})
	wantRect(t, "grid-column: 3",
		fcRect(t, `<div style="display: grid; grid-template-columns: 50px 50px 50px">`+
			table("grid-column: 3")+`</div>`, ``, "t"), [4]float64{100, 0, 50, 20})
	wantRect(t, "justify-self: end in a grid",
		fcRect(t, `<div style="display: grid; grid-template-columns: 100px">`+
			table("justify-self: end")+`</div>`, ``, "t"), [4]float64{88, 0, 12, 20})
	// A table that is not an item keeps §17.5.2's automatic width.
	wantRect(t, "a table in a block",
		fcRect(t, `<div style="width: 400px">`+table("")+`</div>`, ``, "t"), [4]float64{0, 0, 12, 20})
}

// TestAnAbsolutelyPositionedChildIsAlignedByItsOwnAlignSelf is audit C153 and
// the size half of §4.1: the box is aligned in the container's content box "as
// if it were the sole flex item ... of [its] used size", by the container's
// justify-content and its own align-self.
func TestAnAbsolutelyPositionedChildIsAlignedByItsOwnAlignSelf(t *testing.T) {
	flex := func(container, self string) string {
		return `<div style="display: flex; position: relative; width: 300px; height: 100px; ` + container +
			`"><div>a</div><div id="p" style="position: absolute; ` + self + `">x</div></div>`
	}
	at := func(doc string) [2]float64 {
		r := fcRect(t, doc, ``, "p")
		return [2]float64{r[0], r[1]}
	}
	for _, c := range []struct {
		what, container, self string
		want                  [2]float64
	}{
		{"align-self: flex-end", "", "align-self: flex-end", [2]float64{0, 80}},
		{"align-self: center", "", "align-self: center", [2]float64{0, 40}},
		{"align-self overrides align-items", "align-items: flex-end", "align-self: flex-start", [2]float64{0, 0}},
		{"align-self: auto defers to align-items", "align-items: center", "align-self: auto", [2]float64{0, 40}},
		{"justify-content: flex-end", "justify-content: flex-end", "", [2]float64{288, 0}},
		{"justify-content: space-around", "justify-content: space-around", "", [2]float64{144, 0}},
		{"a column", "flex-direction: column; justify-content: center; align-items: flex-end", "",
			[2]float64{288, 40}},
		{"a reversed column", "flex-direction: column-reverse", "", [2]float64{0, 80}},
		{"wrap-reverse", "flex-wrap: wrap-reverse", "", [2]float64{0, 80}},
		{"a right-to-left row", "direction: rtl", "", [2]float64{288, 0}},
		{"a right-to-left row at its end", "direction: rtl; justify-content: flex-end", "", [2]float64{0, 0}},
		{"a right-to-left reversed row", "direction: rtl; flex-direction: row-reverse", "", [2]float64{0, 0}},
		{"margins are part of the box aligned", "justify-content: flex-end; align-items: flex-end",
			"margin: 3px 5px", [2]float64{283, 77}},
		// An offset the author gave is not a static position, and nothing moves.
		{"an explicit offset", "justify-content: flex-end; align-items: flex-end", "left: 10px; top: 10px",
			[2]float64{10, 10}},
	} {
		if got := at(flex(c.container, c.self)); got != c.want {
			t.Errorf("%s: the box is at %v, want %v", c.what, got, c.want)
		}
	}

	// A container with no items at all still places the box in its content
	// box, and that box is as tall as its minimum.
	if got := fcRect(t, `<div style="display: flex; position: relative; width: 300px; min-height: 100px; `+
		`align-items: center"><div id="p" style="position: absolute">x</div></div>`, ``, "p"); got[1] != 40 {
		t.Errorf("in an empty container at its minimum height the box is at y=%g, want 40", got[1])
	}
	if got := fcRect(t, `<div style="display: flex; flex-direction: column; position: relative; `+
		`width: 300px; min-height: 100px; justify-content: center">`+
		`<div id="p" style="position: absolute">x</div></div>`, ``, "p"); got[1] != 40 {
		t.Errorf("in an empty column at its minimum height the box is at y=%g, want 40", got[1])
	}
}
