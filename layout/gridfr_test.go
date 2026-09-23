package layout

import (
	"strings"
	"testing"
)

// CSS Grid Layout 2 §12.6 to §12.8: what is spent once the base sizes are
// known. The fixture is gridCSS's, Courier at 20px, so one character is 12px
// and one line 20px, and every number below is worked out in the comment
// beside it from the specification's own steps rather than read off the
// engine.

// TestAFlexibleTrackGivesWayToContentWiderThanItsShare is §12.7.1's "find the
// size of an fr", and the step in it that was missing: a flexible track whose
// base size is more than its share is treated as inflexible and the share is
// found again without it (audit C103).
func TestAFlexibleTrackGivesWayToContentWiderThanItsShare(t *testing.T) {
	word := func(n int) string { return strings.Repeat("x", n) }

	// 1fr 1fr in 400px, a 300px word in the first. The share is 400 / 2 = 200,
	// which is less than 300, so the first track is inflexible at 300; the
	// second shares what is left, 400 - 300 = 100, alone. It was 300 and 200,
	// and the grid came to 500 inside 400.
	got := gridCells(t, `<div id="g"><div>`+word(25)+`</div><div>b</div></div>`,
		`#g { width: 400px; grid-template-columns: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 300, 20}, {300, 0, 100, 20}},
		"two fractions, the first holding a 300px word")

	// 1fr 1fr 2fr in 600px, a 240px word in the first: the share is 600 / 4 =
	// 150, the first is inflexible at 240, and 360 is shared over three: 120
	// each, so 120 and 240.
	got = gridCells(t, `<div id="g"><div>`+word(20)+`</div><div>b</div><div>c</div></div>`,
		`#g { width: 600px; grid-template-columns: 1fr 1fr 2fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 240, 20}, {240, 0, 120, 20}, {360, 0, 240, 20}},
		"three fractions, the first holding a 240px word")

	// The restart twice. 1fr 1fr 1fr in 300px with words of 180 and 72: the
	// share is 100, so the first (180) is taken out; 120 over two is 60, which
	// is less than 72, so the second is taken out too; the third has 300 -
	// 180 - 72 = 48.
	got = gridCells(t, `<div id="g"><div>`+word(15)+`</div><div>`+word(6)+
		`</div><div>c</div></div>`,
		`#g { width: 300px; grid-template-columns: 1fr 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 180, 20}, {180, 0, 72, 20}, {252, 0, 48, 20}},
		"three fractions, two of them holding words wider than their share")
}

// TestAutoHeightFlexibleRowsShareOneFr is §12.7 where the free space is
// indefinite, which is the rows of a grid with no height: one fr is the
// largest of each flexible row's base per unit of its factor, and the rows
// are that many of their factors. They were each as tall as their own content
// (audit C104).
func TestAutoHeightFlexibleRowsShareOneFr(t *testing.T) {
	// One line and three: "xxxxxxxx" is 96px and a column is 100px, so the
	// second item is three lines, 60px.
	const doc = `<div id="g"><div>a</div><div>xxxxxxxx xxxxxxxx xxxxxxxx</div></div>`

	// 1fr 1fr: max(20 / 1, 60 / 1) = 60, so both rows are 60 and the grid 120.
	// They were 20 and 60.
	got := gridCells(t, doc, `#g { width: 100px; grid-template-columns: 100px;
		grid-template-rows: 1fr 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 60}, {0, 60, 100, 60}}, "1fr 1fr rows")

	// 1fr 2fr: max(20 / 1, 60 / 2) = 30, so the rows are 30 and 60.
	got = gridCells(t, doc, `#g { width: 100px; grid-template-columns: 100px;
		grid-template-rows: 1fr 2fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 30}, {0, 30, 100, 60}}, "1fr 2fr rows")

	// The container is as tall as its rows.
	g := fragmentFor(layoutOf(t, 1000, doc, gridCSS+`#g { width: 100px;
		grid-template-columns: 100px; grid-template-rows: 1fr 1fr }`), "g")
	if g == nil || g.BorderRect.H.Px() != 120 {
		t.Errorf("the grid of two 60px rows is %v", g.BorderRect)
	}
}

// TestAutoHeightFlexibleRowsKeepToTheContainersLimits is §12.7's last clause:
// a fraction that would make the grid smaller than its min-height or larger
// than its max-height is found again, as though the free space were definite
// and the room were the limit.
func TestAutoHeightFlexibleRowsKeepToTheContainersLimits(t *testing.T) {
	const doc = `<div id="g"><div>xxxxxxxx xxxxxxxx xxxxxxxx</div><div>a</div></div>`
	const grid = `#g { width: 100px; grid-template-columns: 100px; `

	// 1fr 2fr with a 60px item in the first row and 20px in the second: one
	// fr is max(60 / 1, 20 / 2) = 60, the rows 60 and 120, the grid 180. At
	// max-height 100 the fr is found in 100: the share is 100 / 3, which is
	// less than the first row's 60, so that row is taken out and the second
	// has 40 over its factor of two: one fr is 20, and the row is 40.
	got := gridCells(t, doc, grid+`grid-template-rows: 1fr 2fr; max-height: 100px }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 60}, {0, 60, 100, 40}},
		"1fr 2fr rows held to max-height 100")

	// 1fr 1fr with 60 and 20: one fr of 60 is a grid of 120, and at
	// min-height 200 the fr is found in 200 instead: 100 each.
	got = gridCells(t, doc, grid+`grid-template-rows: 1fr 1fr; min-height: 200px }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 100}, {0, 100, 100, 100}},
		"1fr 1fr rows held to min-height 200")
}

// TestAnAutoHeightGridGrowsItsRowsToTheirLimits is §12.6 under the constraint
// an auto-height grid's rows are sized under: the free space is infinite, so
// every row that is not flexible is its growth limit. A row written
// "minmax(10px, 100px)" is a hundred pixels high whatever it holds. It was ten,
// and the line in it overflowed.
func TestAnAutoHeightGridGrowsItsRowsToTheirLimits(t *testing.T) {
	got := gridCells(t, `<div id="g"><div>a</div><div>b</div></div>`,
		`#g { width: 100px; grid-template-columns: 100px;
		grid-template-rows: minmax(10px, 100px) minmax(10px, max-content) }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 100}, {0, 100, 100, 20}},
		"two minmax() rows in a grid with no height")
}

// TestFactorsBelowOneLeaveTheRestToTheAutomaticTracks is §12.8 after §12.7:
// flexible factors adding to less than one spend only that fraction of the
// leftover, and what they leave is free space like any other, which "stretch"
// gives to the automatic tracks.
//
// "0.5fr auto" in 400px holding one character each: the bases are 12 and 12,
// the leftover for the fraction is 400 - 12 = 388 over a factor sum of one,
// so 0.5fr is 194; 400 - 194 - 12 = 194 is still over and goes to the auto
// track, which is 206.
func TestFactorsBelowOneLeaveTheRestToTheAutomaticTracks(t *testing.T) {
	got := gridCells(t, `<div id="g"><div>a</div><div>b</div></div>`,
		`#g { width: 400px; grid-template-columns: 0.5fr auto }`)
	wantCells(t, got, [][4]float64{{0, 0, 194, 20}, {194, 0, 206, 20}},
		"half a fraction beside an automatic track")
}

// TestASpanningItemTakesTheDistributedSpace is §11.1: the space
// justify-content and align-content put between two tracks enlarges the
// gutter, and a gutter an item spans is part of its area (audit C154).
func TestASpanningItemTakesTheDistributedSpace(t *testing.T) {
	// Two 100px columns in 400px under space-between sit at 0 and 300, so an
	// item on lines 1 to 3 reaches from 0 to 400.
	got := gridCells(t, `<div id="g"><div id="i">a</div></div>`,
		`#g { width: 400px; grid-template-columns: 100px 100px;
		justify-content: space-between } #i { grid-column: 1 / 3 }`)
	wantCells(t, got, [][4]float64{{0, 0, 400, 20}}, "a stretched item across two columns")

	// Aligned rather than stretched, it is its own 12px and sits at the end of
	// that 400px area.
	got = gridCells(t, `<div id="g"><div id="i">a</div></div>`,
		`#g { width: 400px; grid-template-columns: 100px 100px;
		justify-content: space-between } #i { grid-column: 1 / 3; justify-self: end }`)
	wantCells(t, got, [][4]float64{{388, 0, 12, 20}}, "an item aligned to the end of two columns")

	// The block axis: two 20px rows in 200px under space-between sit at 0 and
	// 180, so an item on both reaches from 0 to 200.
	got = gridCells(t, `<div id="g"><div id="i">a</div></div>`,
		`#g { width: 100px; height: 200px; grid-template-columns: 100px;
		grid-template-rows: 20px 20px; align-content: space-between }
		#i { grid-row: 1 / 3 }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 200}}, "a stretched item across two rows")
}

// TestAutoFitCollapsesOnlyWhatItRepeated is §7.2.3.2: the empty repeated
// tracks of an auto-fit are collapsed, and a track the stylesheet wrote beside
// the repetition is not one of them. Every empty track was collapsed.
func TestAutoFitCollapsesOnlyWhatItRepeated(t *testing.T) {
	// 100px and then 300 / 50 = 6 repetitions. The item in the second column
	// is 100px in; with the empty first column collapsed it was at 0.
	got := gridCells(t, `<div id="g"><div id="i">a</div></div>`,
		`#g { width: 400px; grid-template-columns: 100px repeat(auto-fit, 50px) }
		#i { grid-column: 2 }`)
	wantCells(t, got, [][4]float64{{100, 0, 50, 20}}, "an item after a written track")

	// The written track after the repetition: six 50px repetitions and then
	// 100px, which is column 7. An item in column 1 and one in column 7 leave
	// the five repeated tracks between them empty, and those collapse, so the
	// second is at 50 and 100 wide.
	got = gridCells(t, `<div id="g"><div>a</div><div id="i">b</div></div>`,
		`#g { width: 400px; grid-template-columns: repeat(auto-fit, 50px) 100px }
		#i { grid-column: 7 }`)
	wantCells(t, got, [][4]float64{{0, 0, 50, 20}, {50, 0, 100, 20}},
		"an item in the written track after the repetition")

	// And an empty written track stands. One item in the repetition leaves
	// the other five repetitions and the written 100px empty; the five
	// collapse and the 100px does not, so the columns come to 150 and
	// justify-content: end puts the item at 400 - 150 = 250. With the written
	// track collapsed too they came to 50, and it was at 350.
	got = gridCells(t, `<div id="g"><div>a</div></div>`,
		`#g { width: 400px; grid-template-columns: repeat(auto-fit, 50px) 100px;
		justify-content: end }`)
	wantCells(t, got, [][4]float64{{250, 0, 50, 20}}, "an empty written track beside the repetition")
}
