package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
)

// CSS Grid 2 §8.3 to §8.5, placed. Every expectation is worked out in the
// comment beside it from the specification's steps; gridCSS's Courier makes a
// character 12px and a line 20px.

// TestTheSparseCursorMovesToADefiniteColumn is §8.5 step 4 for an item with a
// definite column and no row, packed sparsely: the cursor goes to its column,
// a row further down when that column is behind the cursor, so the items after
// it are dealt from there (audit C149).
func TestTheSparseCursorMovesToADefiniteColumn(t *testing.T) {
	const doc = `<div id="g"><div id="x">x</div><div id="y">y</div><div id="z">z</div></div>`
	const three = `#g { width: 300px; grid-template-columns: 100px 100px 100px }`

	// X goes to (row 1, column 1) and leaves the cursor there. Y's column 1 is
	// not behind it, but the cell is X's, so Y drops to row 2. The cursor is
	// then (row 2, column 1), and Z, dealt from there, is at row 2, column 2.
	// Z was dealt from where X left the cursor and landed at row 1, column 2,
	// in front of Y.
	wantCells(t, gridCells(t, doc, three+`#y { grid-column: 1 }`),
		[][4]float64{{0, 0, 100, 20}, {0, 20, 100, 20}, {100, 20, 100, 20}},
		"an item placed in the first column after one that took it")

	// Y's column 3 is past the cursor, so it stays on row 1; Z starts the next
	// row.
	wantCells(t, gridCells(t, doc, three+`#y { grid-column: 3 }`),
		[][4]float64{{0, 0, 100, 20}, {200, 0, 100, 20}, {0, 20, 100, 20}},
		"an item placed ahead of the cursor")

	// Behind the cursor, with the cell above free. In four columns X asks for
	// column 3 and leaves the cursor there; Y asks for column 1, which is
	// behind it, so Y goes a row down even though row 1, column 1 is empty —
	// the specification's "if this is less than the previous column position
	// of the cursor, increment the row position by 1". Z is dealt from Y.
	wantCells(t, gridCells(t, doc, `#g { width: 400px;
		grid-template-columns: 100px 100px 100px 100px }
		#x { grid-column: 3 } #y { grid-column: 1 }`),
		[][4]float64{{200, 0, 100, 20}, {0, 20, 100, 20}, {100, 20, 100, 20}},
		"an item placed behind the cursor")

	// Dense packing goes back to the start for every item: Y drops to row 2 in
	// column 1, and Z fills the hole at row 1, column 2.
	wantCells(t, gridCells(t, doc, three+`#g { grid-auto-flow: dense } #y { grid-column: 1 }`),
		[][4]float64{{0, 0, 100, 20}, {0, 20, 100, 20}, {100, 0, 100, 20}},
		"the same, packed densely")
}

// TestARowWithNoRoomGrowsColumns is §8.5 step 2: an item locked to a row goes
// to the earliest column where it overlaps nothing, and step 3 counts the
// implicit columns from where step 2 put the items — so a row asked for by
// more items than it has columns makes more columns (audit C149).
func TestARowWithNoRoomGrowsColumns(t *testing.T) {
	// Two 100px columns, three items on row 1: the third makes a third
	// column, which grid-auto-columns makes 50px. It was put over the first.
	wantCells(t, gridCells(t, `<div id="g"><div>a</div><div>b</div><div>c</div></div>`,
		`#g { width: 300px; grid-template-columns: 100px 100px; grid-auto-columns: 50px }
		#g > div { grid-row: 1 }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}, {200, 0, 50, 20}},
		"three items locked to a row of two columns")

	// The rows it spans all have to be free. A is at row 2, column 1; B is
	// locked to rows 1 and 2, so column 1 is taken in its second row and it
	// goes to column 2. Only its first row was asked about, and it was put
	// over A, at x=0. (Row 1 holds only B, whose 20px row 2 already covers,
	// so §12.5 leaves row 1 at nothing and both items are at the top.)
	wantCells(t, gridCells(t, `<div id="g"><div id="a">a</div><div id="b">b</div></div>`,
		`#g { width: 300px; grid-template-columns: 100px 100px }
		#a { grid-row: 2; grid-column: 1 } #b { grid-row: 1 / span 2 }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}},
		"an item locked to two rows")
}

// TestRowLockedItemsKeepTheirOrderWhenSparse is step 2's two packings. Sparse:
// past any item this step already put in that row. Dense: the earliest column
// that fits, holes included.
func TestRowLockedItemsKeepTheirOrderWhenSparse(t *testing.T) {
	// Three 100px columns and 50px implicit ones. A is at row 1, column 2.
	// B, locked to row 1 and two columns wide, does not fit at column 1
	// (column 2 is A's) and goes to columns 3 and 4: x=200, 100 + 50 wide. C,
	// locked to row 1, is past B when sparse: column 5, x=350. Dense, it goes
	// back to the hole at column 1.
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div><div id="c">c</div></div>`
	const css = `#g { width: 500px; grid-template-columns: 100px 100px 100px;
		grid-auto-columns: 50px }
		#a { grid-row: 1; grid-column: 2 } #b { grid-row: 1; grid-column: span 2 }
		#c { grid-row: 1 }`
	wantCells(t, gridCells(t, doc, css),
		[][4]float64{{100, 0, 100, 20}, {200, 0, 150, 20}, {350, 0, 50, 20}}, "sparse")
	wantCells(t, gridCells(t, doc, css+`#g { grid-auto-flow: row dense }`),
		[][4]float64{{100, 0, 100, 20}, {200, 0, 150, 20}, {0, 0, 100, 20}}, "dense")
}

// TestTwoSpansAreTheFirst is §8.3.1: "If the placement contains two spans,
// remove the one contributed by the end grid-placement property." It took the
// larger of the two (audit C150).
func TestTwoSpansAreTheFirst(t *testing.T) {
	got, ok := gridRect(t, `<div id="g"><div id="i">x</div></div>`,
		noDefaults+`#g { display: grid; width: 400px;
		grid-template-columns: 100px 100px 100px 100px }
		#i { grid-column: span 2 / span 3 }`, "i")
	if !ok || got.W.Px() != 200 {
		t.Errorf("span 2 / span 3 is %v wide, want two 100px columns", got.W.Px())
	}
}

// TestASpanBeforeTheFirstLineMakesTracksBeforeIt is §8.3.1 and §7.5: "span 3 /
// 2" ends at line 2 and begins three tracks before it, which is two tracks in
// front of the explicit grid. The grid makes them — implicit tracks before the
// explicit ones — and §7.6 sizes them from grid-auto-columns counted
// backwards. The start was clamped to line 1, so the item covered three
// explicit columns it had not asked for (audit C150).
func TestASpanBeforeTheFirstLineMakesTracksBeforeIt(t *testing.T) {
	// grid-auto-columns: 30px 50px. The last implicit track before the
	// explicit grid takes the last size, 50, and the one before it 30. So the
	// columns are 30 50 | 100 100 100, the item covers 30 + 50 + 100 = 180,
	// and B, dealt from the first track there is, is at 180.
	wantCells(t, gridCells(t, `<div id="g"><div id="a">a</div><div id="b">b</div></div>`,
		`#g { width: 500px; grid-template-columns: 100px 100px 100px;
		grid-auto-columns: 30px 50px } #a { grid-column: span 3 / 2 }`),
		[][4]float64{{0, 0, 180, 20}, {180, 0, 100, 20}},
		"span 3 / 2 in three explicit columns")

	// The same on the block axis: two 20px rows before the explicit one.
	wantCells(t, gridCells(t, `<div id="g"><div id="a">a</div></div>`,
		`#g { width: 100px; grid-template-columns: 100px; grid-template-rows: 30px;
		grid-auto-rows: 20px } #a { grid-row: span 3 / 2 }`),
		[][4]float64{{0, 0, 100, 70}}, "span 3 / 2 on the rows")
}

// TestAShorthandAndItsLonghandMeetInTheCascade is audit C107, laid out: the
// more specific longhand wins over the shorthand it belongs to, and a more
// specific grid-column over grid-area.
func TestAShorthandAndItsLonghandMeetInTheCascade(t *testing.T) {
	const doc = `<div id="g"><div id="a" class="x">a</div></div>`
	const three = `#g { width: 300px; grid-template-columns: 100px 100px 100px }`

	// grid-column: 1 / 2 against grid-column-start: 3 on the id: the lines
	// are 3 and 2, which §8.3.1 swaps, so the item is column 2, at 100. It was
	// column 1: the shorthand won.
	wantCells(t, gridCells(t, doc, three+`.x { grid-column: 1 / 2 } #a { grid-column-start: 3 }`),
		[][4]float64{{100, 0, 100, 20}}, "a longhand more specific than its shorthand")

	// grid-area: p against grid-column: 2 on the id: the row is p's and the
	// column is 2. The area won outright.
	wantCells(t, gridCells(t, doc, three+`#g { grid-template-areas: ". . p" }
		.x { grid-area: p } #a { grid-column: 2 }`),
		[][4]float64{{100, 0, 100, 20}}, "grid-column more specific than grid-area")
}

// TestAnAreaIsFoundByTheLinesItNames is §8.3's <custom-ident>: an area of the
// template gives its edges the names "area-start" and "area-end", and a name
// at a start or end edge finds them. The names are case-sensitive, as the
// template's are.
func TestAnAreaIsFoundByTheLinesItNames(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div></div>`
	const grid = `#g { width: 300px; grid-template-columns: 100px 100px 100px;
		grid-template-areas: ". Main Main" }`
	for _, place := range []string{
		`grid-area: Main`,
		`grid-column: Main`,
		`grid-column: Main-start / Main-end`,
		`grid-column-start: Main; grid-column-end: Main`,
	} {
		wantCells(t, gridCells(t, doc, grid+`#a { `+place+` }`),
			[][4]float64{{100, 0, 200, 20}}, place)
	}
}

// TestAnAbsolutelyPositionedGridChildIsAlignedByItsOwnSelf is the grid half of
// audit C153: §10.1 aligns the box in the content box as the sole item of an
// area that is the whole of it, by its own justify-self and align-self, or the
// container's justify-items and align-items where those are "auto". It was at
// the start corner whatever it said.
func TestAnAbsolutelyPositionedGridChildIsAlignedByItsOwnSelf(t *testing.T) {
	grid := func(container, self string) string {
		return `<div style="display: grid; position: relative; width: 300px; height: 100px; ` +
			container + `"><div>a</div><div id="p" style="position: absolute; ` + self +
			`">x</div></div>`
	}
	for _, c := range []struct {
		what, container, self string
		want                  [2]float64
	}{
		// The box is 12 by 20 in a 300 by 100 content box.
		{"at its initial values", "", "", [2]float64{0, 0}},
		{"justify-self: end", "", "justify-self: end", [2]float64{288, 0}},
		{"align-self: end", "", "align-self: end", [2]float64{0, 80}},
		{"centred both ways", "", "justify-self: center; align-self: center",
			[2]float64{144, 40}},
		{"auto defers to the container", "justify-items: end; align-items: center",
			"", [2]float64{288, 40}},
		{"its own beats the container's", "justify-items: end", "justify-self: start",
			[2]float64{0, 0}},
		{"a right-to-left grid starts on the right", "direction: rtl", "",
			[2]float64{288, 0}},
		{"and ends on the left", "direction: rtl", "justify-self: end", [2]float64{0, 0}},
		// An offset the author gave is not a static position, and nothing moves.
		{"an explicit offset", "", "justify-self: end; left: 10px; top: 10px",
			[2]float64{10, 10}},
	} {
		r := fcRect(t, grid(c.container, c.self), ``, "p")
		if got := [2]float64{r[0], r[1]}; got != c.want {
			t.Errorf("%s: the box is at %v, want %v", c.what, got, c.want)
		}
	}

	// A grid with no items still places the box in its content box, as tall as
	// its minimum height.
	if got := fcRect(t, `<div style="display: grid; position: relative; width: 300px; `+
		`min-height: 100px; align-items: end"><div id="p" style="position: absolute">x</div></div>`,
		``, "p"); got[1] != 80 {
		t.Errorf("in an empty grid at its minimum height the box is at y=%g, want 80", got[1])
	}
}

// TestARowFilledByItsItemsCostsTheItems is step 2's cost. Items locked to one
// row and packed densely each look for the earliest free column, and each was
// found by walking every column the ones before it had filled: n items were n²
// windows asked about. Four times the items is to be about four times the
// work.
func TestARowFilledByItsItemsCostsTheItems(t *testing.T) {
	setup := func(n int) []*gridItem {
		items := make([]*gridItem, n)
		for i := range items {
			items[i] = &gridItem{place: [2]gridPlacement{
				{start: 0, definite: true, span: 1}, {span: 1}}}
		}
		return items
	}
	small, large := setup(1000), setup(4000)
	c := costtest.Time(t, "placing a row of densely packed items",
		func() { placeItems(copyGridItems(small), 1, true) },
		func() { placeItems(copyGridItems(large), 1, true) })
	if c.Ratio > 8 {
		t.Errorf("four times the items took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}

// TestARowThatGrowsColumnsIsClamped is §7.1's limit on the axis step 2 grows,
// lowered so that it can be watched: items locked to one row past the last
// column the engine makes are put in it, and the document is told.
func TestARowThatGrowsColumnsIsClamped(t *testing.T) {
	defer func(n int) { maxGridTracks = n }(maxGridTracks)
	maxGridTracks = 10
	root, said := gridFinding(t, `<div style="display:grid;width:600px">`+
		strings.Repeat(`<div style="grid-row:1">x</div>`, 12)+`</div>`,
		RuleLimit, "reach past 10 tracks")
	if !said {
		t.Fatal("twelve items locked to a row in a grid of ten columns said nothing")
	}
	grid := root.Children[0].Children[0]
	columns := map[float64]bool{}
	for _, c := range grid.Children {
		columns[c.BorderRect.X.Px()] = true
	}
	if len(columns) != 10 {
		t.Errorf("the items are in %d columns, want the 10 the limit allows", len(columns))
	}
}
