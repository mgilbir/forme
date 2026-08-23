package layout

import "testing"

// min-width and max-width on a table cell, CSS 2.1 §10.4.
//
// The two apply to "all elements but non-replaced inline elements, table rows,
// and row groups", and a cell is on the list. The column was asked and the cell
// was not, so "width: 3in; max-width: 1in" on a cell came out three inches wide
// and a cell with a minimum and no width came out as wide as the letter in it.
// max-width-applies-to-007 and min-width-applies-to-007 are those two, and each
// is a square that was not square.

// cellAndTable lays out a two-cell table and returns the first cell's width and
// the table's, both in pixels.
func cellAndTable(t *testing.T, tableCSS, cellCSS string) (float64, float64) {
	t.Helper()
	root := layoutOf(t, 600,
		`<div id="table"><div id="row"><div id="cell">x</div><div id="c2">y</div></div></div>`,
		noDefaults+`#table { display: table; `+tableCSS+` }
		 #row { display: table-row }
		 #cell, #c2 { display: table-cell; height: 96px }
		 #cell { `+cellCSS+` }`)
	return find(t, root, "cell").BorderRect.W.Px(), find(t, root, "table").BorderRect.W.Px()
}

// TestACellsOwnLimitsDecideItsColumn, under both algorithms.
//
// The two are separate code paths — the automatic one works from what each cell
// demands, the fixed one from what the first row declares — and a fixture using
// only one leaves the other untested. The fixed algorithm needs a table with a
// width of its own; without one it falls back to the automatic algorithm
// whatever table-layout asks for, which is what the suite's own fixtures do and
// is why they exercise only that half.
func TestACellsOwnLimitsDecideItsColumn(t *testing.T) {
	for _, tc := range []struct {
		table, cell string
		want        float64
		what        string
	}{
		// A maximum below the declared width wins: the cell is an inch, and the
		// text in it overflows, which is what §10.4 asks for.
		{``, `width: 300px; max-width: 100px`, 100, "auto: a maximum under the width"},
		{`width: 400px; table-layout: fixed`, `width: 300px; max-width: 100px`, 100,
			"fixed: a maximum under the width"},
		// A minimum with no width at all still settles the column. Without it
		// the cell is as wide as the one letter in it.
		{``, `min-width: 100px`, 100, "auto: a minimum alone"},
		{`width: 400px; table-layout: fixed`, `min-width: 100px`, 100, "fixed: a minimum alone"},
		// §10.4's own order: the maximum is applied first and the minimum to its
		// result, so a minimum above a maximum wins.
		{``, `width: 50px; min-width: 100px; max-width: 80px`, 100, "auto: a minimum above the maximum"},
		{`width: 400px; table-layout: fixed`, `width: 50px; min-width: 100px; max-width: 80px`, 100,
			"fixed: a minimum above the maximum"},
	} {
		got, _ := cellAndTable(t, tc.table, tc.cell)
		if got != tc.want {
			t.Errorf("%s: the cell is %gpx, want %g", tc.what, got, tc.want)
		}
	}
}

// TestACellWithoutLimitsIsUnchanged is the containment case, and it is most of
// every table: a cell that declares neither must be sized exactly as it was.
func TestACellWithoutLimitsIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		table, cell string
		want        float64
		what        string
	}{
		{``, ``, 8, "auto, nothing declared: one Courier character"},
		{``, `width: 300px`, 300, "auto, a width alone"},
		{`width: 400px; table-layout: fixed`, `width: 300px`, 300, "fixed, a width alone"},
	} {
		got, _ := cellAndTable(t, tc.table, tc.cell)
		if got != tc.want {
			t.Errorf("%s: the cell is %gpx, want %g", tc.what, got, tc.want)
		}
	}
}

// TestAPercentageLimitOnACellIsLeftAlone, for the reason the column's own case
// gives: what a percentage width is a percentage *of* is the table's width, and
// the table's width is the number this is being asked to help work out. Reading
// one here would resolve it against something that is not settled yet.
func TestAPercentageLimitOnACellIsLeftAlone(t *testing.T) {
	got, _ := cellAndTable(t, ``, `width: 300px; max-width: 50%`)
	if got != 300 {
		t.Errorf("the cell is %gpx, want 300 — a percentage maximum is not read here", got)
	}
}
