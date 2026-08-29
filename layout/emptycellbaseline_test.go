package layout

import "testing"

// A table cell with nothing in it that makes a line box.
//
// §17.5.3 says such a cell's baseline is "the bottom of content edge of the cell
// box", and taken literally that makes an empty cell the tallest thing in its
// row: it is pushed down until its own bottom edge sits on the row's baseline,
// so a row of cells that each asked for the same height comes out that height
// plus an ascent, and the declaration is counted twice. css-tables-3 states the
// rule every browser follows instead — a cell with no baseline is aligned to the
// start edge and takes no part in the row's baseline at all.
//
// CSS2/normal-flow/width-applies-to-006 is two such rows against a square.

// cellRowHeight is the height of the row #c sits in.
func cellRowHeight(t *testing.T, cells, cellCSS string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="t"><div id="r">`+cells+`</div></div>`,
		`#t { display: table } #r { display: table-row }
		.c { display: table-cell; `+cellCSS+` }`)
	return find(t, root, "r").BorderRect.H.Px()
}

// TestAnEmptyCellDoesNotDeepenItsRow is the bug.
func TestAnEmptyCellDoesNotDeepenItsRow(t *testing.T) {
	alone := cellRowHeight(t, `<div class="c">a</div>`, "height: 48px")
	if alone != 48 {
		t.Fatalf("a lettered cell of 48px makes a row %gpx tall; the fixture only "+
			"says anything if this one is right", alone)
	}
	got := cellRowHeight(t, `<div class="c">a</div><div class="c"></div>`, "height: 48px")
	if got != 48 {
		t.Errorf("adding an empty cell of the same height makes the row %gpx tall, "+
			"want 48 — the empty cell has no baseline to be aligned on", got)
	}
}

// TestTheEmptyCellIsAtTheTopOfTheRow, which is where "aligned to the start edge"
// puts it, and is visible as soon as the row is taller than the cell.
func TestTheEmptyCellIsAtTheTopOfTheRow(t *testing.T) {
	root := layoutOf(t, 600, `<div id="t"><div id="r">
		<div class="c" style="height:60px">a</div>
		<div class="c" id="e"><div id="in" style="height:10px"></div></div>
	</div></div>`,
		`#t { display: table } #r { display: table-row } .c { display: table-cell }`)
	e, in := find(t, root, "e"), find(t, root, "in")
	if got := in.BorderRect.Y.Sub(e.BorderRect.Y); got != 0 {
		t.Errorf("the content of the cell with no baseline sits %v below the cell's "+
			"top, want 0", got)
	}
}

// TestACellWithTextStillAlignsOnTheBaseline is the containment argument, and it
// is what baseline alignment on a table is for: two cells set in different sizes
// read as one line of text.
func TestACellWithTextStillAlignsOnTheBaseline(t *testing.T) {
	root := layoutOf(t, 600, `<div id="t"><div id="r">
		<div class="c" id="big" style="font-size:40px">a</div>
		<div class="c" id="small" style="font-size:10px">b</div>
	</div></div>`,
		`#t { display: table } #r { display: table-row }
		.c { display: table-cell; font-family: Courier }`)
	base := func(id string) float64 {
		f := find(t, root, id)
		return f.BorderRect.Y.Add(f.Lines[0].Rect.Y).Add(f.Lines[0].Baseline).Px()
	}
	if got, want := base("small"), base("big"); got != want {
		t.Errorf("the small cell's baseline is at %g and the big one's at %g; the "+
			"two must line up", got, want)
	}
}

// TestAnEmptyCellStillGetsItsOwnHeight. Not taking part in the baseline is not
// the same as not being there: a cell told to be 48px tall is still 48px tall,
// and a row of nothing but empty cells is as tall as the tallest of them.
func TestAnEmptyCellStillGetsItsOwnHeight(t *testing.T) {
	if got := cellRowHeight(t, `<div class="c"></div>`, "height: 48px"); got != 48 {
		t.Errorf("a row of one empty 48px cell is %gpx tall, want 48", got)
	}
	if got := cellRowHeight(t,
		`<div class="c" style="height:20px"></div><div class="c" style="height:48px"></div>`,
		""); got != 48 {
		t.Errorf("a row of a 20px and a 48px empty cell is %gpx tall, want 48", got)
	}
}
