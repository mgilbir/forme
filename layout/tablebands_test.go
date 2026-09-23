package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A separated table's row, column and group backgrounds are shown through the
// cells, and the bands they are shown through are the cells. See tableBands.

// bandsIn counts the band rectangles in a laid-out tree.
func bandsIn(f *Fragment) int {
	if f == nil {
		return 0
	}
	n := len(f.bgBands)
	for _, c := range f.Children {
		n += bandsIn(c)
	}
	return n
}

// TestTableBandsAreProportionalToTheCells is audit C13. A first row spanning
// the column cap made every later row 4096 columns wide, and every row was
// given a band per column whether a cell was there or not: 20,000 rows of one
// cell each were eighty million rectangles and 1.7 GB. A band is a cell now,
// so the rows and their cells are what the bands come to.
func TestTableBandsAreProportionalToTheCells(t *testing.T) {
	const rows = 300
	root := layoutOf(t, 800, `<table><tr>`+strings.Repeat(`<td colspan=1000></td>`, 5)+
		`</tr>`+strings.Repeat(`<tr><td></td></tr>`, rows)+`</table>`)
	// One band for each row's one cell, five for the first row's, and nothing
	// for the four thousand columns nobody put a cell in.
	if n := bandsIn(root); n != rows+5 {
		t.Errorf("the table was given %d bands for %d cells", n, rows+5)
	}
}

// TestTableBandsCostTheCellsNotTheSlots measures the same shape where it is
// quadratic: a table n columns wide and n rows deep, its first row n cells and
// every other row one, has 2n cells and n² slots. Every row, every column, and
// a group of each across the whole table, are asked for their bands. Four times
// n is four times the cells, and was sixteen times the slots.
func TestTableBandsCostTheCellsNotTheSlots(t *testing.T) {
	l := newLayouter(&Box{}, Size{}, nil, nil)
	w, _ := style.FromPx(10)
	setup := func(n int) *tableBands {
		g := &tableGrid{cols: n}
		cols, colX := make([]style.Unit, n), make([]style.Unit, n)
		rowY, rowH := make([]style.Unit, n), make([]style.Unit, n)
		for i := 0; i < n; i++ {
			cols[i], colX[i] = w, w.Mul(float64(i))
			rowH[i], rowY[i] = w, w.Mul(float64(i))
			g.cells = append(g.cells, &tableCell{box: &Box{}, row: 0, col: i, rowSpan: 1, colSpan: 1})
			if i > 0 {
				g.cells = append(g.cells, &tableCell{box: &Box{}, row: i, col: 0, rowSpan: 1, colSpan: 1})
			}
		}
		return l.newTableBands(&Box{}, g, cols, colX, rowY, rowH, true)
	}
	ask := func(tb *tableBands, n int) int {
		got := len(tb.rows(0, n-1)) + len(tb.columns(0, n-1))
		for i := 0; i < n; i++ {
			got += len(tb.rows(i, i)) + len(tb.columns(i, i))
		}
		return got
	}
	small, large := setup(500), setup(2000)
	var got int
	lo, hi, ratio := layoutScaling(func() { ask(small, 500) }, func() { got = ask(large, 2000) })
	// Each cell once for its row, once for its column and once for each group.
	if want := 4 * (2*2000 - 1); got != want {
		t.Fatalf("a table of %d cells was given %d bands; want %d", 2*2000-1, got, want)
	}
	if ratio > 8 {
		t.Errorf("four times the rows and columns took %.1f times as long (%v against %v); "+
			"the bands are to cost the cells and not the slots", ratio, hi, lo)
	}
}

// TestABandIsACell pins what a background is shown through, in a table with
// room between its cells.
func TestABandIsACell(t *testing.T) {
	root := layoutOf(t, 800, `<table style="border-spacing:10px">`+
		`<colgroup><col id=wide span=2><col></colgroup>`+
		`<tr id=full><td style="width:40px">a</td><td style="width:40px">b</td><td style="width:40px">c</td></tr>`+
		`<tr id=short><td>d</td></tr>`+
		`<tr id=spanned><td colspan=2>e</td><td>f</td></tr>`+
		`<tr id=empty style="height:20px"></tr>`+
		`</table>`, noDefaults)
	row := func(id string) *Fragment {
		for _, f := range fragmentsOf(root, id) {
			return f
		}
		t.Fatalf("no fragment for #%s", id)
		return nil
	}
	px := func(v float64) style.Unit { u, _ := style.FromPx(v); return u }

	if n := len(row("full").bgBands); n != 3 {
		t.Errorf("a row of three cells has %d bands", n)
	}
	// A row holding one cell of three is shown through that cell, and the two
	// slots nobody filled show the table, like the spacing does.
	if n := len(row("short").bgBands); n != 1 {
		t.Errorf("a row with one cell has %d bands; the slots with no cell show the table", n)
	}
	// A cell across two columns is one box, and the spacing inside it is part
	// of it: one band as wide as both columns and the space between.
	spanned := row("spanned").bgBands
	if len(spanned) != 2 {
		t.Fatalf("a row of a two-column cell and a one-column cell has %d bands", len(spanned))
	}
	full := row("full").bgBands
	if want := full[1].X.Add(full[1].W).Sub(full[0].X); spanned[0].W != want {
		t.Errorf("the spanning cell's band is %vpx wide; its box, two columns and the "+
			"%vpx between, is %vpx", spanned[0].W.Px(), full[1].X.Sub(full[0].X.Add(full[0].W)).Px(),
			want.Px())
	}
	if gap := full[1].X.Sub(full[0].X.Add(full[0].W)); gap != px(10) {
		t.Errorf("the fixture's spacing came out at %vpx", gap.Px())
	}
	// A row with no cells shows nothing: one band of no size, which is not
	// the same as no bands, and paints nothing.
	if empty := row("empty").bgBands; len(empty) != 1 || !empty[0].Empty() {
		t.Errorf("a row with no cells has bands %v; it is to show nothing", empty)
	}
	// One <col span=2> is one box across two columns, shown through the cells
	// of both: a, b, d and e — the last once, as one cell.
	if n := len(row("wide").bgBands); n != 4 {
		t.Errorf("a column element spanning two columns has %d bands; its cells are four", n)
	}
}

// TestTableBandsAreBounded is maxTableBands firing: a table whose cells span
// more than the bound would give its rows and columns no bands, paints their
// backgrounds across the spacing as well, and says so.
func TestTableBandsAreBounded(t *testing.T) {
	defer func(n int) { maxTableBands = n }(maxTableBands)
	maxTableBands = 1
	built := Build(Input{HTML: `<table><tr><td rowspan=20>a</td><td>b</td></tr>` +
		strings.Repeat(`<tr><td>c</td></tr>`, 19) + `</table>`})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	root := Layout(built.Root, Size{W: w, H: w}, nil, rec)
	said := false
	for _, f := range rec.Findings() {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "spacing between the cells") {
			said = true
		}
	}
	if !said {
		t.Error("the bound on bands was passed and nothing was reported")
	}
	if n := bandsIn(root); n != 0 {
		t.Errorf("%d bands were made past the bound", n)
	}
}

// TestAColumnSpanIsBandedOnce is the same shape one level up: a <col span=n>
// is one box widened across n columns, and its bands were asked for again at
// every column it widened across — the span times the cells beneath it, all
// but the last answer thrown away. Both places a column is emitted from, inside
// a column group and outside one, are measured.
func TestAColumnSpanIsBandedOnce(t *testing.T) {
	l := newLayouter(&Box{}, Size{}, nil, nil)
	w, _ := style.FromPx(10)
	setup := func(n int, grouped bool) (*tableGrid, []style.Unit, []style.Unit, *tableBands) {
		col := &Box{}
		g := &tableGrid{cols: n, rows: []tableRowInfo{{box: &Box{}, group: -1}}}
		cols, colX := make([]style.Unit, n), make([]style.Unit, n)
		for i := 0; i < n; i++ {
			cols[i], colX[i] = w, w.Mul(float64(i))
			g.colBoxes = append(g.colBoxes, col)
			g.cells = append(g.cells, &tableCell{box: &Box{}, row: 0, col: i, rowSpan: 1, colSpan: 1})
		}
		if grouped {
			g.colGroups = []tableSpan{{box: &Box{}, first: 0, count: n}}
		}
		return g, cols, colX, l.newTableBands(&Box{}, g, cols, colX, []style.Unit{0}, []style.Unit{w}, true)
	}
	for _, grouped := range []bool{false, true} {
		sg, sc, sx, sb := setup(2000, grouped)
		lg, lc, lx, lb := setup(8000, grouped)
		var small, large *Fragment
		lo, hi, ratio := layoutScaling(func() {
			small = &Fragment{}
			l.paintableColumns(small, sg, sb, sc, sx, 0, w)
		}, func() {
			large = &Fragment{}
			l.paintableColumns(large, lg, lb, lc, lx, 0, w)
		})
		// The column's 8000 cells, and the group's as many again.
		want := 8000
		if grouped {
			want *= 2
		}
		if n := bandsIn(large); n != want {
			t.Fatalf("grouped %v: one column across 8000 cells was given %d bands; want %d",
				grouped, n, want)
		}
		if ratio > 8 {
			t.Errorf("grouped %v: four times the span took %.1f times as long (%v against %v); "+
				"a column's bands are to be asked for once", grouped, ratio, hi, lo)
		}
	}
}
