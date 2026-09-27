package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A table's baseline is its first row's.
//
// css-tables-3 §3: "The baseline of a table-root is the baseline of its first
// row", and CSS Box Alignment 3 §9.1 says what a row's is: the shared baseline
// of its baseline-aligned cells when it has any, and otherwise one synthesized
// from "the lowest and highest content edges of all the cells in the row" —
// for an alphabetic baseline, the bottom content edge of the lowest cell, which
// is also CSS 2.1 §17.5.3's wording.
//
// The engine used to find a table's baseline by walking into it for the first
// line box anywhere inside, which is a block container's rule. The two agree
// only when the first row has text in it, and each fixture below is one of the
// ways they part.

var (
	tableBaselineMark = style.RGBA{R: 255, A: 1}
	tableBaselineRow  = style.RGBA{B: 255, A: 1}
)

// tableAgainstBaseline lays out a line holding an inline-table and a 10px
// inline-block, and returns how far the line's baseline is below the top of the
// table's first row. The inline-block has no line box in it, so §10.8.1 puts its
// bottom margin edge on the baseline: the bottom of its fill is where the
// baseline is. The first row's cell is the blue fill.
func tableAgainstBaseline(t *testing.T, table string) style.Unit {
	t.Helper()
	ops := paintOf(t,
		`<div><span id="m"></span>`+table+`</div>`,
		noDefaults+`
		 div { font-size: 10px; line-height: 10px }
		 #m { display: inline-block; width: 10px; height: 10px; background: rgb(255,0,0) }
		 .t { display: inline-table; border-spacing: 0 }
		 .r { display: table-row }
		 .c { display: table-cell; width: 20px; padding: 0 }
		 .first { background: rgb(0,0,255) }
		 .cap { display: table-caption; height: 40px }`)
	mark := soleFill(t, ops, tableBaselineMark, "the inline-block")
	row := soleFill(t, ops, tableBaselineRow, "the first row's cell")
	return mark.Y.Add(mark.H).Sub(row.Y)
}

// TestATableOfEmptyCellsSitsOnItsFirstRow is §9.1's synthesized baseline: a
// first row of empty cells 20px tall puts the table's baseline 20px down, on
// the row's bottom content edge. The walk found no line box at all and left the
// inline-table on its bottom margin edge, fifty pixels down, which is what put
// the second of margin-collapse-134-ref's three inline tables a strut's descent
// lower than every browser draws it.
func TestATableOfEmptyCellsSitsOnItsFirstRow(t *testing.T) {
	got := tableAgainstBaseline(t, `<span class="t">`+
		`<span class="r"><span class="c first" style="height:20px"></span></span>`+
		`<span class="r"><span class="c" style="height:30px"></span></span></span>`)
	if want := bgpx(20); got != want {
		t.Errorf("the baseline is %gpx below the first row's top, want %gpx: the "+
			"bottom content edge of its lowest cell", got.Px(), want.Px())
	}
}

// TestATableDoesNotTakeItsBaselineFromALaterRow: the same empty first row, and
// text in the second. The walk went on past the first row to the first line
// box it could find, which is in the second, and put the baseline there.
func TestATableDoesNotTakeItsBaselineFromALaterRow(t *testing.T) {
	got := tableAgainstBaseline(t, `<span class="t">`+
		`<span class="r"><span class="c first" style="height:20px"></span></span>`+
		`<span class="r"><span class="c">x</span></span></span>`)
	if want := bgpx(20); got != want {
		t.Errorf("the baseline is %gpx below the first row's top, want %gpx: the "+
			"second row's text is not the table's first row", got.Px(), want.Px())
	}
}

// TestACaptionIsNotATablesBaseline: css-tables-3 aligns an inline-table by "the
// table-root box (not the table-wrapper box)", and a caption is the wrapper's.
// The walk started at the wrapper and took the caption's line, which comes
// first.
func TestACaptionIsNotATablesBaseline(t *testing.T) {
	got := tableAgainstBaseline(t, `<span class="t">`+
		`<span class="cap">x</span>`+
		`<span class="r"><span class="c first" style="height:20px"></span></span></span>`)
	if want := bgpx(20); got != want {
		t.Errorf("the baseline is %gpx below the first row's top, want %gpx: the "+
			"caption above the grid is not its first row", got.Px(), want.Px())
	}
}

// TestATableWithTextInItsFirstRowStillSitsOnTheText is the control: where the
// first row has a line box, the row's baseline is that line's, and nothing
// about the synthesized case may move it. The cell is 40px tall and the text is
// at its top, so the baseline is well above the 40px the synthesized rule
// would give.
func TestATableWithTextInItsFirstRowStillSitsOnTheText(t *testing.T) {
	got := tableAgainstBaseline(t, `<span class="t">`+
		`<span class="r"><span class="c first" style="height:40px">x</span></span></span>`)
	if got <= 0 || got >= bgpx(20) {
		t.Errorf("the baseline is %gpx below the first row's top, want the text's, "+
			"within its first 10px line", got.Px())
	}
}
