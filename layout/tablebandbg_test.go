package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// What a row, column or group background covers in the separated borders model.
//
// §17.5.1 stacks six layers behind a table's content: the table, then the column
// groups, the columns, the row groups, the rows and the cells. The four in the
// middle are boxes drawn *behind the cells they hold* — and §17.6.1 fills the
// space between cells with the table's own background, not theirs. So a column
// is interrupted by a stripe of whatever is behind the table at every row gap,
// and a row is interrupted at every column gap.
//
// The engine painted one solid rectangle across the whole span, so a column with
// a background bled through every horizontal gap and a row through every
// vertical one. The suite states the expected picture unusually plainly: it
// draws the column as one aqua block and then lays white stripes over it, one
// per gap, at exactly the border-spacing.
//
// In the collapsing model there is no spacing to interrupt anything, and the
// whole box is painted — which is both correct and what almost every table is.

// bandsOf returns the rectangles one colour was painted in, top-left first.
func bgBandsOf(t *testing.T, css string, colour style.RGBA) []Rect {
	t.Helper()
	const doc = `<table id="t">
		<colgroup id="g"><col id="c1"/><col id="c2"/></colgroup>
		<tr id="r1"><td>a</td><td>b</td></tr>
		<tr id="r2"><td>c</td><td>d</td></tr>
	</table>`
	return fillsOf(paintOf(t, doc, `
		#t { border-collapse: separate; border-spacing: 10px 20px; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		`+css), colour)
}

// The four cells of the fixture, which is what every band below is one of.
// Columns are 30 wide at x=18 and x=58, rows 40 tall at y=28 and y=88 — a 10px
// gap between the columns and a 20px gap between the rows.
var (
	cellTopLeft     = Rect{X: bgpx(18), Y: bgpx(28), W: bgpx(30), H: bgpx(40)}
	cellTopRight    = Rect{X: bgpx(58), Y: bgpx(28), W: bgpx(30), H: bgpx(40)}
	cellBottomLeft  = Rect{X: bgpx(18), Y: bgpx(88), W: bgpx(30), H: bgpx(40)}
	cellBottomRight = Rect{X: bgpx(58), Y: bgpx(88), W: bgpx(30), H: bgpx(40)}
)

// sameRects compares the bands as a set. Which order they were emitted in is
// not part of the picture — they do not overlap — and asserting one would pin
// the loop rather than the rule.
func sameRects(got []Rect, want ...Rect) bool {
	if len(got) != len(want) {
		return false
	}
	left := map[Rect]int{}
	for _, r := range want {
		left[r]++
	}
	for _, r := range got {
		if left[r] == 0 {
			return false
		}
		left[r]--
	}
	return true
}

// TestAColumnBackgroundSkipsTheRowGaps.
func TestAColumnBackgroundSkipsTheRowGaps(t *testing.T) {
	got := bgBandsOf(t, `#c2 { background: rgb(0,0,255) }`, blue)
	if !sameRects(got, cellTopRight, cellBottomRight) {
		t.Errorf("the column painted %v; it holds two cells with twenty pixels of "+
			"table between them", got)
	}
}

// TestARowBackgroundSkipsTheColumnGaps, which is the same rule on the other
// axis and needs its own case: a fixture of one row and two columns would pass
// with the bands computed per row and nothing knocked out horizontally.
func TestARowBackgroundSkipsTheColumnGaps(t *testing.T) {
	got := bgBandsOf(t, `#r2 { background: rgb(0,128,0) }`, green)
	if !sameRects(got, cellBottomLeft, cellBottomRight) {
		t.Errorf("the row painted %v; it holds two cells with ten pixels of table "+
			"between them", got)
	}
}

// TestAColumnGroupCoversEveryCellItHolds: four cells, four bands, both gaps.
func TestAColumnGroupCoversEveryCellItHolds(t *testing.T) {
	got := bgBandsOf(t, `#g { background: rgb(0,0,255) }`, blue)
	if !sameRects(got, cellTopLeft, cellBottomLeft, cellTopRight, cellBottomRight) {
		t.Errorf("the column group painted %v; it holds all four cells and neither "+
			"gap", got)
	}
}

// TestARowGroupCoversEveryCellItHolds. The rows of a table with no <tbody> are
// still in an anonymous row group, so this is the same shape reached the other
// way — and a row group's fragment is the parent of its rows, which is where the
// bands could most easily be left in the wrong coordinate space.
func TestARowGroupCoversEveryCellItHolds(t *testing.T) {
	const doc = `<table id="t"><tbody id="b">
		<tr><td>a</td><td>b</td></tr>
		<tr><td>c</td><td>d</td></tr>
	</tbody></table>`
	got := fillsOf(paintOf(t, doc, `
		#t { border-collapse: separate; border-spacing: 10px 20px; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		#b { background: rgb(0,0,255) }`), blue)
	if !sameRects(got, cellTopLeft, cellTopRight, cellBottomLeft, cellBottomRight) {
		t.Errorf("the row group painted %v; it holds all four cells and neither gap",
			got)
	}
}

// TestTheCollapsingModelPaintsTheWholeBox is the containment case, and it is
// most tables. With the borders collapsed there is no spacing between cells —
// the property does not apply — so there is nothing to interrupt a background
// and the box is painted whole.
func TestTheCollapsingModelPaintsTheWholeBox(t *testing.T) {
	const doc = `<table id="t">
		<colgroup><col id="c1"/><col id="c2"/></colgroup>
		<tr><td>a</td><td>b</td></tr>
		<tr><td>c</td><td>d</td></tr>
	</table>`
	got := fillsOf(paintOf(t, doc, `
		#t { border-collapse: collapse; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		#c2 { background: rgb(0,0,255) }`), blue)
	if len(got) != 1 {
		t.Errorf("the column painted %d rectangles; with the borders collapsed there "+
			"is no spacing to leave out and the box is one rectangle: %v", len(got), got)
	}
}

// TestABackgroundImageIsPositionedAgainstTheWholeBox is why the bands clip
// rather than being painted.
//
// A picture on a column is positioned against the column and shown through the
// cells. Painting it once per band instead would start it afresh in each one,
// which is a different picture — and the suite tests exactly this, with a
// diagonal gradient placed once at the top left and once at the bottom right of
// a column three rows tall.
func TestABackgroundImageIsPositionedAgainstTheWholeBox(t *testing.T) {
	const doc = `<table id="t">
		<colgroup><col id="c1"/><col id="c2"/></colgroup>
		<tr><td>a</td><td>b</td></tr>
		<tr><td>c</td><td>d</td></tr>
	</table>`
	res := mapResolver{"bg.svg": bgSVG(`width="10" height="10"`)}
	ops := paintWith(t, res, doc, `
		#t { border-collapse: separate; border-spacing: 10px 20px; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		#c2 { background-image: url(bg.svg); background-repeat: no-repeat;
		      background-position: bottom right }`)
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d marks for one no-repeat picture: %v", len(got), got)
	}
	// The column's box runs to y=128, so a bottom-right 10px picture sits at
	// (78, 118) — inside the last cell, which is where it shows.
	if got[0] != (Rect{X: bgpx(78), Y: bgpx(118), W: bgpx(10), H: bgpx(10)}) {
		t.Errorf("the picture is at %v; it is placed against the whole column and "+
			"only *shown* through the cells, so its position is the column's bottom "+
			"right and not any cell's", got[0])
	}
}

// TestABandedBackgroundStillObeysVisibility. The banded path is a second way
// through the painter's decorations, and the two guards the ordinary one opens
// with have to be asked on it as well — a row group with "visibility: hidden" is
// laid out and not drawn like any other box.
func TestABandedBackgroundStillObeysVisibility(t *testing.T) {
	got := bgBandsOf(t, `#c2 { background: rgb(0,0,255); visibility: hidden }`, blue)
	if len(got) != 0 {
		t.Errorf("a hidden column painted %v", got)
	}
}
