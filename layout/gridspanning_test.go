package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Grid Layout 2 §12.5, "Resolve Intrinsic Track Sizes", for the items
// that span more than one track. The fixture is gridCSS's, Courier at 20px, so
// one character is 12px and one line 20px, and every number below is worked
// out in the comment beside it from the specification's steps.

// TestASpanningItemGrowsFlexibleTracksByTheirFactors is step 4: an item that
// crosses a flexible track is accommodated after every other, and the space it
// needs goes to the flexible tracks alone, "according to the ratios of their
// flexible sizing functions rather than distributing space equally". It was
// shared equally over every track the item spanned, flexible or not.
func TestASpanningItemGrowsFlexibleTracksByTheirFactors(t *testing.T) {
	// Rows "1fr 3fr" in a grid with no height. The first item spans both rows
	// and is 100px tall; the other two are a line each, one in each row of the
	// second column. Step 4 takes all three together: the lines ask 20 of
	// their own rows, the spanning item asks 100 of the two, and its 100 is
	// shared a quarter and three quarters — 25 and 75. Each row is the larger
	// of what was asked of it, 25 and 75, and §12.7's fr for an indefinite
	// height is the largest of 25 / 1, 75 / 3 and the spanning item's own 100
	// / 4, all 25. So the rows are 25 and 75 and the grid 100, which is what
	// the item asked for.
	//
	// Shared equally the rows were 50 and 50, the fr 50, and the rows 50 and
	// 150: a grid of 200 around an item of 100.
	const rows = `<div id="g">` +
		`<div style="grid-row: 1 / span 2; height: 100px">a</div>` +
		`<div>b</div><div>c</div></div>`
	got := gridCells(t, rows, `#g { width: 200px; grid-template-columns: 100px 100px; `+
		`grid-template-rows: 1fr 3fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 100}, {100, 0, 100, 25}, {100, 25, 100, 75}},
		"a 100px item across rows of 1fr and 3fr")

	// A factor sum below one: "0.1fr 0.3fr". §12.5 shares that proportion of
	// the space by the factors and the rest equally: 0.4 of 100 by 1 : 3 is
	// 10 and 30, and 0.6 equally is 30 and 30, so 40 and 60. The fr is then
	// the largest of the rows' bases, 40 and 60 (a factor below one counts as
	// one), and 60 × 0.1 and 60 × 0.3 are under both bases, so the rows stay
	// 40 and 60. Equally they were 50 and 50.
	got = gridCells(t, rows, `#g { width: 200px; grid-template-columns: 100px 100px; `+
		`grid-template-rows: 0.1fr 0.3fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 100, 100}, {100, 0, 100, 40}, {100, 40, 100, 60}},
		"a 100px item across rows of 0.1fr and 0.3fr")

	// On the columns, an item that states a width: its minimum contribution is
	// its width, 200, and in a 100px container of "1fr 3fr" that is 50 and
	// 150 by the factors. The one-letter items below it ask 12 of each. The fr
	// of 100 is 25, less than 50, so the first column is inflexible; what is
	// left, 50, is a third of 150, so the second is too: the columns stay 50
	// and 150. Equally they were 100 and 100.
	const across = `<div id="g"><div style="grid-column: span 2; width: 200px">a</div>` +
		`<div>b</div><div>c</div></div>`
	got = gridCells(t, across, `#g { width: 100px; grid-template-columns: 1fr 3fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 200, 20}, {0, 20, 50, 20}, {50, 20, 150, 20}},
		"a 200px item across columns of 1fr and 3fr")
}

// TestASpanningItemAsksFlexibleTracksForNothingAtItsMinimum is Grid §6.6, which
// §12.5 reads an item's minimum contribution through: the automatic minimum
// size of an item that spans more than one track, one of them flexible, is
// zero. So an item with no width of its own across "1fr 3fr" holds neither
// column open by its longest word, and the columns are the container's
// quarter and three quarters. It held both at half its word.
func TestASpanningItemAsksFlexibleTracksForNothingAtItsMinimum(t *testing.T) {
	// A 300px word over "1fr 3fr" in 200px: the word asks nothing at its
	// minimum, the letters below it ask 12 of each column, and the fr of 200
	// is 50 — the columns are 50 and 150, and the word overflows them, which
	// is what a browser draws. Asked for its whole width by the factors it
	// would have made them 75 and 225, both then too wide for their share and
	// the grid 300 wide; shared equally, as it was, 150 and 150.
	doc := `<div id="g"><div style="grid-column: span 2">` + strings.Repeat("x", 25) +
		`</div><div>b</div><div>c</div></div>`
	got := gridCells(t, doc, `#g { width: 200px; grid-template-columns: 1fr 3fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 200, 20}, {0, 20, 50, 20}, {50, 20, 150, 20}},
		"a 300px word across 1fr and 3fr in 200px")

	// And only to the flexible tracks: a 200px item over "auto 1fr" in 100px,
	// with a letter in the second column, grows the flexible column alone, to
	// 200 — the letter is accommodated in the same round, and its 12 is the
	// smaller planned increase — and leaves the "auto" column, which nothing
	// else is in, at nothing. Shared over both it made them 94 and 106.
	doc = `<div id="g"><div style="grid-column: 2">b</div>` +
		`<div style="grid-row: 2; grid-column: span 2; width: 200px">a</div></div>`
	got = gridCells(t, doc, `#g { width: 100px; grid-template-columns: auto 1fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 200, 20}, {0, 20, 200, 20}},
		"a 200px item across auto and 1fr")

	// The same in a float, which is measured before it is laid out. Measured,
	// the word does ask for itself — "limited min-content contributions" under
	// a constraint — and 96px across "1fr 3fr" is 24 and 72, which the
	// letters' 12 do not raise. The fr under a max-content constraint is the
	// largest of 24, 72 / 3 and 96 / 4, all 24, so the float is 96 wide, and
	// laid out at 96 the columns are 24 and 72. Shared equally the word made
	// 48 and 48, the fr 48, and the float 192.
	doc = `<div style="float: left"><div id="g"><div style="grid-column: span 2">` +
		strings.Repeat("x", 8) + `</div><div>b</div><div>c</div></div></div>`
	got = gridCells(t, doc, `#g { grid-template-columns: 1fr 3fr }`)
	wantCells(t, got, [][4]float64{{0, 0, 96, 20}, {0, 20, 24, 20}, {24, 20, 72, 20}},
		"a 96px word across 1fr and 3fr in a float")
}

// TestAGridItemsMinimumContributionIsItsMinimumSize is §12.5's "auto
// minimums" as the specification writes them: while a grid is laid out, a
// track whose minimum is "auto" is grown to its items' minimum contributions,
// which is what the item's min-width comes to where its width is auto — not
// its min-content contribution. "min-width: 0" is the idiom for a "1fr" column
// that may be narrower than a long word, and a scroll container's automatic
// minimum is zero as well; both did nothing.
func TestAGridItemsMinimumContributionIsItsMinimumSize(t *testing.T) {
	doc := func(item string) string {
		return `<div id="g"><div style="` + item + `">` + strings.Repeat("x", 25) +
			`</div><div>b</div></div>`
	}
	const css = `#g { width: 400px; grid-template-columns: 1fr 1fr }`
	// A 300px word with min-width: 0 asks nothing of its column, and the two
	// columns take 200 each; the word overflows its cell, which is what the
	// author asked for.
	for _, item := range []string{"min-width: 0", "overflow: hidden", "min-width: 10%"} {
		wantCells(t, gridCells(t, doc(item), css),
			[][4]float64{{0, 0, 200, 20}, {200, 0, 200, 20}}, item)
	}
	// A stated minimum is what it states: 250 is more than the share of 200,
	// so the first column is inflexible at 250 and the second has 150.
	wantCells(t, gridCells(t, doc("min-width: 250px"), css),
		[][4]float64{{0, 0, 250, 20}, {250, 0, 150, 20}}, "min-width: 250px")
	// With min-width at its initial "auto" the automatic minimum is the
	// content-based one, the word, and it is §12.7.1 that gives way: 300 and
	// 100 (audit C103).
	wantCells(t, gridCells(t, doc(""), css),
		[][4]float64{{0, 0, 300, 20}, {300, 0, 100, 20}}, "min-width: auto")
	// "overflow: clip" is not a scroll container, and keeps its content-based
	// minimum.
	wantCells(t, gridCells(t, doc("overflow: clip"), css),
		[][4]float64{{0, 0, 300, 20}, {300, 0, 100, 20}}, "overflow: clip")

	// A content-based minimum is held to the track's fixed maximum (§6.6): a
	// 96px word in "minmax(auto, 50px)" asks for 50, not 96, even where the
	// container has no room at all.
	got := gridCells(t, `<div id="g"><div>xxxxxxxx</div><div>b</div></div>`,
		`#g { width: 10px; grid-template-columns: minmax(auto, 50px) auto }`)
	if got[1].X.Px() != 50 {
		t.Errorf("a 96px word in minmax(auto, 50px): the next column starts at %g, want 50",
			got[1].X.Px())
	}

	// And measured, where §12.5 reads a "limited" min-content contribution,
	// held to the same maximum: a float round that grid is 50 wide.
	f := fragmentFor(layoutOf(t, 1000, `<div id="f" style="float: left"><div id="g">`+
		`<div>xxxxxxxx</div></div></div>`,
		gridCSS+`#g { grid-template-columns: minmax(auto, 50px) }`), "f")
	if f == nil || f.BorderRect.W.Px() != 50 {
		t.Errorf("a float round a 96px word in minmax(auto, 50px): %v, want 50 wide", f)
	}

	// The same on the rows. Rows "1fr 1fr" in a 40px grid, the first holding
	// three lines, 60px, and the second one line: at "min-height: 0" the rows
	// are the 20 and 20 the fr gives, and the three lines overflow theirs.
	// With the automatic minimum the first row is its 60, too tall for its
	// share of 20, and the second keeps its own 20.
	rows := func(item string) string {
		return `<div id="g"><div style="` + item + `">xxxxxxxx xxxxxxxx xxxxxxxx</div>` +
			`<div>b</div></div>`
	}
	const rowCSS = `#g { width: 100px; height: 40px; grid-template-rows: 1fr 1fr }`
	for _, item := range []string{"min-height: 0", "overflow: hidden"} {
		got := gridCells(t, rows(item), rowCSS)
		if got[1].Y.Px() != 20 {
			t.Errorf("%s: the second row starts at %g, want 20", item, got[1].Y.Px())
		}
	}
	if got := gridCells(t, rows(""), rowCSS); got[1].Y.Px() != 60 {
		t.Errorf("min-height: auto: the second row starts at %g, want 60", got[1].Y.Px())
	}
}

// TestAGrowthLimitSetBySpanningItemsIsInfinitelyGrowable is the example §12.5
// gives for its "infinitely growable" flag, in Peter Salas's words: two "auto"
// columns, a 10px item in the first, and an item across both whose
// min-content contribution is 30 and max-content 100. The first column's base
// and limit are 10 from its own item. The spanning item's 30 goes to the
// second column's base, 20, since the first is at its limit; its min-content
// 30 then makes the second column's infinite limit 20, and marks it
// infinitely growable; and its max-content 100 goes to that column alone,
// whose limit becomes 90, because the first column's limit is finite and not
// marked. Laid out in 60px, the 30 left over grows only the second column: 10
// and 50.
//
// Shared equally at every step, the columns had bases of 20 and 10 and limits
// of 55 and 45, and came out 35 and 25 — the first column wider than anything
// in it.
func TestAGrowthLimitSetBySpanningItemsIsInfinitelyGrowable(t *testing.T) {
	box := func(w int) string {
		return `<span style="display: inline-block; width: ` + itoa(w) + `px"></span>`
	}
	// min-content 30, the widest piece; max-content 30 + 12 + 30 + 12 + 16.
	doc := `<div id="g"><div>` + box(10) + `</div>` +
		`<div style="grid-row: 2; grid-column: span 2">` + box(30) + ` ` + box(30) + ` ` +
		box(16) + `</div></div>`
	got := gridCells(t, doc, `#g { width: 60px; grid-template-columns: auto auto }`)
	// (The spanning item's 100 does not fit in 60, so it is two lines.)
	wantCells(t, got, [][4]float64{{0, 0, 10, 20}, {0, 20, 60, 40}},
		"two auto columns, one spanned by an item of min 30 and max 100")
}

// TestSpaceAnAffectedTrackCannotTakeGoesToTheOthersFirst is §12.5.1's
// "distribute space to non-affected tracks": once every track a round affects
// has reached its growth limit, what an item still needs goes to the other
// tracks it spans, as far as their own limits, "instead of violating the
// growth limits of the affected tracks" — and only then past the limits.
//
// Columns "minmax(10px, 100px) auto" in 10px, with a letter in the second and
// a 150px item across both. The letter makes the "auto" column 12 at base and
// limit. The spanning item needs 150 - 10 - 12 = 128 more; the round for
// intrinsic minimums affects only the "auto" column, which is at its limit, so
// the first column, whose minimum is fixed, takes what it can up to its 100 —
// 90 — and the 38 left goes past the limit to the "auto" column: 100 and 50.
// It all went to the "auto" column: 10 and 140.
func TestSpaceAnAffectedTrackCannotTakeGoesToTheOthersFirst(t *testing.T) {
	doc := `<div id="g"><div style="grid-column: 2">b</div>` +
		`<div style="grid-row: 2; grid-column: span 2; width: 150px">a</div></div>`
	got := gridCells(t, doc, `#g { width: 10px; grid-template-columns: minmax(10px, 100px) auto }`)
	wantCells(t, got, [][4]float64{{100, 0, 50, 20}, {0, 20, 150, 20}},
		"a 150px item across minmax(10px, 100px) and auto")
}

// TestSpanningItemsOfOneSpanDoNotDependOnTheirOrder is §12.5.1's planned
// increase: every item of a group is measured against the tracks as the group
// found them, and a track grows by the most any one of them asked, "This
// prevents the size increases from becoming order-dependent." Two items of
// span two over three "auto" tracks, the first across tracks one and two and
// the second across two and three, each needing 100: each shares its 100
// equally, 50 and 50, and the tracks are 50, 50 and 50. Taken one after the
// other, the second item found track two already at 50 and gave its remaining
// 50 to two and three: 50, 75 and 25, and the other order gave 25, 75 and 50.
func TestSpanningItemsOfOneSpanDoNotDependOnTheirOrder(t *testing.T) {
	px := func(v float64) style.Unit { u, _ := style.FromPx(v); return u }
	for _, order := range [][]trackAsk{
		{{from: 0, span: 2, min: px(100), max: px(100), minimum: px(100)},
			{from: 1, span: 2, min: px(100), max: px(100), minimum: px(100)}},
		{{from: 1, span: 2, min: px(100), max: px(100), minimum: px(100)},
			{from: 0, span: 2, min: px(100), max: px(100), minimum: px(100)}},
	} {
		tracks := []gridTrack{autoTrack(), autoTrack(), autoTrack()}
		trackBasesAndLimits(tracks, order, 0, sizedForLayout)
		for i, tr := range tracks {
			if tr.base != px(50) {
				t.Errorf("items %+v: track %d is %gpx, want 50 (all three: %gpx, %gpx, %gpx)",
					order, i, tr.base.Px(), tracks[0].base.Px(), tracks[1].base.Px(),
					tracks[2].base.Px())
			}
		}
	}

	// And a narrower span is settled before a wider one, whatever order the
	// items come in, and a wider one's need goes first to the tracks that have
	// not reached their growth limits. A span-two item of 80 over the first
	// two of three "auto" tracks makes them 40 and 40, and its min-content and
	// max-content contributions make their growth limits 40 too. A span-three
	// item of 90 then needs 10 more, and the first two are at their limits:
	// the third takes it, 40, 40 and 10. Taken by span but shared equally it
	// was 43.33, 43.33 and 3.33; taken in document order, 30 each and then
	// nothing.
	for _, order := range [][]trackAsk{
		{{from: 0, span: 3, min: px(90), max: px(90), minimum: px(90)},
			{from: 0, span: 2, min: px(80), max: px(80), minimum: px(80)}},
		{{from: 0, span: 2, min: px(80), max: px(80), minimum: px(80)},
			{from: 0, span: 3, min: px(90), max: px(90), minimum: px(90)}},
	} {
		tracks := []gridTrack{autoTrack(), autoTrack(), autoTrack()}
		trackBasesAndLimits(tracks, order, 0, sizedForLayout)
		if got := [3]float64{tracks[0].base.Px(), tracks[1].base.Px(), tracks[2].base.Px()}; got != [3]float64{40, 40, 10} {
			t.Errorf("items %+v: the tracks are %v, want 40, 40 and 10", order, got)
		}
	}
}
