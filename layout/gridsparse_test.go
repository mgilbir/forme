package layout

import (
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// A grid's cost is its items', not its area's. See gridOccupancy and
// maxGridTracks.

// randomGridItems makes a set of items the way gridItems and the gate would: a
// placement on each axis that is a line, a span, or both, with the lines and
// spans small enough that the dense occupancy the placement is compared with
// stays small.
func randomGridItems(r *rand.Rand) []*gridItem {
	n := 1 + r.Intn(12)
	items := make([]*gridItem, n)
	for i := range items {
		it := &gridItem{}
		for axis := 0; axis < 2; axis++ {
			p := gridPlacement{span: 1 + r.Intn(3)}
			if r.Intn(3) == 0 {
				p.start, p.definite = r.Intn(6), true
			}
			it.place[axis] = p
		}
		items[i] = it
	}
	return items
}

func copyGridItems(in []*gridItem) []*gridItem {
	out := make([]*gridItem, len(in))
	for i, it := range in {
		c := *it
		out[i] = &c
	}
	return out
}

// TestGridPlacementIsTheDensePlacement holds the sparse occupancy to a table
// of every cell placed by §8.5 as the specification words it, which is kept in
// grid_reference_test.go as the definition of where an item goes: every item
// in the same cell, and the grid the same number of rows and columns, sparse
// and dense packing both.
func TestGridPlacementIsTheDensePlacement(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 20000; trial++ {
		items := randomGridItems(r)
		explicit := r.Intn(5)
		columns := max(explicit, tracksNeeded(items, 1, explicit))
		dense := r.Intn(2) == 0
		want, got := copyGridItems(items), copyGridItems(items)
		wantRows, wantColumns := placeItemsDense(want, columns, dense)
		gotRows, gotColumns, clamped := placeItems(got, columns, dense)
		if clamped {
			t.Fatalf("trial %d: a grid of %d rows was clamped", trial, gotRows)
		}
		if wantRows != gotRows || wantColumns != gotColumns {
			t.Fatalf("trial %d: %d rows and %d columns by the dense table and %d "+
				"and %d by the runs", trial, wantRows, wantColumns, gotRows, gotColumns)
		}
		for i := range want {
			if want[i].row != got[i].row || want[i].column != got[i].column {
				t.Fatalf("trial %d, item %d (%+v): the dense table puts it at row %d "+
					"column %d and the runs at row %d column %d", trial, i,
					items[i].place, want[i].row, want[i].column, got[i].row, got[i].column)
			}
		}
	}
}

// TestTrackSizingIsTheScan holds the track sizing, which sorts the items once,
// keeps a planned increase only for the tracks a round touches and shares
// space by the level the sorted rooms give, to §12.5 written as it reads —
// intrinsicSizesBySpec — which walks every item once per span, keeps every
// track's planned increase and freezes tracks pass by pass.
func TestTrackSizingIsTheScan(t *testing.T) {
	l := newLayouter(&Box{}, Size{}, nil, nil)
	r := rand.New(rand.NewSource(11))
	kinds := []trackSize{{kind: trackAuto}, {kind: trackMin}, {kind: trackMax},
		{kind: trackFixed, size: 640}, {kind: trackFlex, factor: 1},
		{kind: trackFlex, factor: 0.5}, {kind: trackFlex, factor: 2},
		{kind: trackFlex, factor: 0}, {kind: trackFlex, factor: 0.25}}
	for trial := 0; trial < 20000; trial++ {
		tracks := make([]gridTrack, 1+r.Intn(8))
		for i := range tracks {
			tracks[i] = gridTrack{min: kinds[r.Intn(4)], max: kinds[r.Intn(len(kinds))]}
		}
		asks := make([]trackAsk, r.Intn(10))
		for i := range asks {
			span := 1 + r.Intn(3)
			// A minimum contribution at most the min-content one, which is
			// at most the max-content one, as §12.5's note says they are.
			minimum := style.Unit(r.Intn(1000))
			min := minimum + style.Unit(r.Intn(4000))
			asks[i] = trackAsk{from: r.Intn(len(tracks)), span: span,
				min: min, max: min + style.Unit(r.Intn(8000)),
				minimum: minimum, automatic: r.Intn(2) == 0}
		}
		gap := style.Unit(r.Intn(3) * 64)
		room := style.Unit(r.Intn(40000))
		definite, stretch := r.Intn(2) == 0, r.Intn(2) == 0
		want := append([]gridTrack(nil), tracks...)
		got := append([]gridTrack(nil), tracks...)
		// The container's limits, which an indefinite room is held between.
		lo, hi := style.Unit(0), style.MaxUnit
		if r.Intn(2) == 0 {
			lo = style.Unit(r.Intn(20000))
			hi = lo + style.Unit(r.Intn(20000))
		}
		l.resolveTracksByScan(want, asks, gap, room, definite, stretch, lo, hi)
		l.resolveTracks(got, asks, gap, room, definite, stretch, lo, hi)
		for i := range want {
			if want[i].base != got[i].base {
				t.Fatalf("trial %d: track %d is %d by the scan and %d now (%+v, asks %+v)",
					trial, i, want[i].base, got[i].base, tracks, asks)
			}
		}
		// And §12.5 alone, under each of the three constraints: the base
		// sizes and the growth limits both.
		for _, under := range []sizingConstraint{sizedForLayout, underMinContent, underMaxContent} {
			want := append([]gridTrack(nil), tracks...)
			got := append([]gridTrack(nil), tracks...)
			wantLimits := intrinsicSizesBySpec(want, asks, gap, under)
			gotLimits := trackBasesAndLimits(got, asks, gap, under)
			for i := range want {
				if want[i].base != got[i].base || wantLimits[i] != gotLimits[i] {
					t.Fatalf("trial %d, constraint %d: track %d is %d limited to %d by the "+
						"specification and %d limited to %d now (%+v, asks %+v, gap %d)",
						trial, under, i, want[i].base, wantLimits[i], got[i].base,
						gotLimits[i], tracks, asks, gap)
				}
			}
		}
	}
}

// TestGridCostIsTheItemsNotTheArea is audit C12's shape. Eight items each
// spanning s rows and s columns stack s rows apart, and the dense occupancy
// was the rows times the columns: 8s rows of s cells, filled and searched s² at
// a time. Four times the span is four times the runs and sixteen times the
// cells.
//
// Eight items at 240 and 960, not twenty at 60 and 240. At the smaller sizes
// the work that does not grow with the span — laying twenty one-letter items
// out, twice each — was most of what was timed, and the dense occupancy of
// before de912bf, copied back in, read between 4.6 and 6.2 against a bound of
// 8: the test could not see the defect it was written for. With the items few and the
// spans long the cells are the cost, and the same copy reads 13.7 to 14.2
// while the runs read 2.8. The large side is 7,680 rows of 960 columns, inside
// maxGridTracks and maxRepeatedTracks both, so neither cap is what is timed.
func TestGridCostIsTheItemsNotTheArea(t *testing.T) {
	const items = 8
	doc := func(span int) Built {
		s := strconv.Itoa(span)
		return Build(Input{HTML: `<div style="display:grid">` +
			strings.Repeat(`<div style="grid-row:span `+s+`;grid-column:span `+s+`">x</div>`, items) +
			`</div>`})
	}
	small, large := doc(240), doc(960)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(100000)
	var smallRows, largeRows int
	c := costtest.Time(t, "a grid of items spanning s rows and columns", func() {
		smallRows = gridRowsLaidOut(Layout(small.Root, Size{W: w, H: h}, nil, nil))
	}, func() {
		largeRows = gridRowsLaidOut(Layout(large.Root, Size{W: w, H: h}, nil, nil))
	})
	// The fixture has to be what it says: every item placed below the last,
	// s rows apart, and none clamped into the last track by a cap.
	if smallRows != items || largeRows != items {
		t.Fatalf("%d and %d items were laid out in rows of their own; the fixture "+
			"is meant to stack all %d", smallRows, largeRows, items)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the span took %.1f times as long (%v against %v); the "+
			"occupancy is to cost the items and not the cells", c.Ratio, c.Large, c.Small)
	}
}

// gridRowsLaidOut is how many of the grid's items begin at a different height,
// which for the stacked fixture above is how many were placed where the
// automatic placement puts them rather than clamped together.
func gridRowsLaidOut(root *Fragment) int {
	var grid *Fragment
	var find func(f *Fragment)
	find = func(f *Fragment) {
		if grid != nil {
			return
		}
		if f.Box != nil && f.Box.Inner == InnerGrid {
			grid = f
			return
		}
		for _, c := range f.Children {
			find(c)
		}
	}
	find(root)
	if grid == nil {
		return 0
	}
	seen := map[style.Unit]bool{}
	for _, c := range grid.Children {
		seen[c.BorderRect.Y] = true
	}
	return len(seen)
}

// TestTrackSizingIsLinearInTheTracks is the other quadratic in the grid: every
// track asked every item what it wanted, and every item asked where its track
// began by adding up every track before it. A column of n items is n rows and
// n items, and was n² of both.
func TestTrackSizingIsLinearInTheTracks(t *testing.T) {
	l := newLayouter(&Box{}, Size{}, nil, nil)
	setup := func(n int) ([]gridTrack, []trackAsk) {
		tracks := make([]gridTrack, n)
		asks := make([]trackAsk, n)
		for i := range tracks {
			tracks[i] = autoTrack()
			asks[i] = trackAsk{from: i, span: 1, min: 640, max: 1280, automatic: true}
		}
		return tracks, asks
	}
	st, sa := setup(3000)
	lt, la := setup(12000)
	c := costtest.Time(t, "sizing a column of n tracks", func() {
		l.resolveTracks(st, sa, 0, 1<<24, true, true, 0, style.MaxUnit)
		trackEdgesOf(st).start(len(st)-1, 0)
	}, func() {
		l.resolveTracks(lt, la, 0, 1<<24, true, true, 0, style.MaxUnit)
		trackEdgesOf(lt).start(len(lt)-1, 0)
	})
	if c.Ratio > 8 {
		t.Errorf("four times the tracks took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}

// TestSpreadingIsLinearInTheTracksSpanned is §12.5.1's "distribute equally
// ... freezing a track ... as its affected size + item-incurred increase
// reaches its limit (and continuing to grow the unfrozen tracks as needed)",
// for one item across n tracks whose limits are all different. Done the way
// the words read — share, freeze the tracks the share would take past their
// limits, share again — it is a pass over the tracks for every track that
// freezes, and here every track freezes in turn: n². spread sorts the tracks
// by their room once and walks them.
//
// n "auto" columns, the i-th holding an item that may be nothing wide and would
// like i pixels — a base of nought and a growth limit of i — and one item
// across all of them whose minimum is three quarters of what those limits add
// up to: the round for intrinsic minimums fills the narrow columns to their
// limits one after another, and shares the rest over the wide ones.
func TestSpreadingIsLinearInTheTracksSpanned(t *testing.T) {
	setup := func(n int) ([]gridTrack, []trackAsk) {
		tracks := make([]gridTrack, n)
		asks := make([]trackAsk, 0, n+1)
		sum := style.Unit(0)
		for i := range tracks {
			tracks[i] = autoTrack()
			w, _ := style.FromPx(float64(i + 1))
			sum = sum.Add(w)
			asks = append(asks, trackAsk{from: i, span: 1, min: w, max: w})
		}
		all := sum.Mul(0.75)
		return tracks, append(asks, trackAsk{from: 0, span: n, min: all, max: all, minimum: all})
	}
	st, sa := setup(1000)
	lt, la := setup(4000)
	c := costtest.Time(t, "one item across n tracks of different limits",
		func() { trackBasesAndLimits(st, sa, 0, sizedForLayout) },
		func() { trackBasesAndLimits(lt, la, 0, sizedForLayout) })
	if c.Ratio > 8 {
		t.Errorf("four times the tracks took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}

// TestGrowingToLimitsStopsWhenNothingGrows is the rounding remainder: a unit
// or two of free space among many tracks shares out as nothing, and every pass
// after the one that gave nothing is the same pass again. It ran one per
// track, which is the tracks squared to hand out nothing.
//
// Five hundred tracks and two thousand. At two thousand and eight thousand the
// fixed walk is one pass of a few microseconds over three hundred kilobytes of
// tracks, and a machine streaming memory on its other cores read that as 9.6 to
// 14.6: the smaller set of tracks stayed in the cache and the larger did not.
// Two thousand are eighty kilobytes. Planted — the passes run on after one
// gave nothing — it reads as 16.3.
func TestGrowingToLimitsStopsWhenNothingGrows(t *testing.T) {
	setup := func(n int) ([]gridTrack, []style.Unit) {
		tracks := make([]gridTrack, n)
		limits := make([]style.Unit, n)
		for i := range tracks {
			tracks[i] = autoTrack()
			limits[i] = 1 << 20
		}
		return tracks, limits
	}
	st, sl := setup(500)
	lt, ll := setup(2000)
	var sf, lf style.Unit
	c := costtest.Time(t, "growing n tracks to their limits",
		func() { sf = growToLimits(st, sl, 7) },
		func() { lf = growToLimits(lt, ll, 7) })
	wantS, _ := setup(500)
	if want := growToLimitsByScan(wantS, sl, 7); sf != want || lf != want {
		t.Fatalf("the free space left is %d and %d; the full passes leave %d", sf, lf, want)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the tracks took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}

// gridFinding says whether laying a document out raised a finding with the rule
// and the words given, and returns the root.
func gridFinding(t *testing.T, html string, rule Rule, words string) (*Fragment, bool) {
	t.Helper()
	built := Build(Input{HTML: html, CSS: []Stylesheet{{Source: noDefaults}}})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	root := Layout(built.Root, Size{W: w, H: w}, nil, rec)
	for _, f := range rec.Findings() {
		if f.Rule == rule && strings.Contains(f.Message, words) {
			return root, true
		}
	}
	return root, false
}

// TestAnExplicitGridIsBoundedHoweverItIsWritten holds every way of writing the
// explicit grid to maxRepeatedTracks: the words of grid-template-areas, which
// made fifty thousand columns from one string, and a track list of many
// repeat()s, each inside its own bound. Each is refused and reported, and one
// just inside the bound is not.
func TestAnExplicitGridIsBoundedHoweverItIsWritten(t *testing.T) {
	words := func(n int) string { return strings.TrimSpace(strings.Repeat("a ", n)) }
	for name, tc := range map[string]struct {
		style   string
		refused bool
	}{
		"areas at the bound":       {`grid-template-areas:'` + words(maxRepeatedTracks) + `'`, false},
		"areas past the bound":     {`grid-template-areas:'` + words(maxRepeatedTracks+1) + `'`, true},
		"area rows past the bound": {`grid-template-areas:` + strings.Repeat(`'a' `, maxRepeatedTracks+1), true},
		"repeats at the bound":     {`grid-template-columns:repeat(500,1px) repeat(500,1px)`, false},
		"repeats past the bound":   {`grid-template-columns:repeat(500,1px) repeat(501,1px)`, true},
		// An automatic repetition is at least one, so the tracks beside it and
		// one of it are what the stylesheet wrote.
		"auto-fill at the bound":   {`grid-template-columns:repeat(500,1px) repeat(auto-fill,1px) repeat(499,1px)`, false},
		"auto-fill past the bound": {`grid-template-columns:repeat(500,1px) repeat(auto-fill,1px) repeat(500,1px)`, true},
	} {
		_, refused := gridFinding(t, `<div style="display:grid;`+tc.style+`"><div>x</div></div>`,
			RuleUnsupportedValue, "grid container was laid out as a column")
		if refused != tc.refused {
			t.Errorf("%s: refused is %v", name, refused)
		}
	}
}

// TestATrackListIsRefusedAsItIsRead is the same bound asked early enough to
// matter. A list of many "repeat(1000, 1px)" was refused, but only once every
// repeat() in it had been made: four hundred of them are seven kilobytes of CSS
// and were four hundred thousand tracks first. What reading one costs now is
// what parsing it costs, and a repeat() or two past the bound.
func TestATrackListIsRefusedAsItIsRead(t *testing.T) {
	raw := strings.Repeat("repeat(1000,1px) ", 400)
	built := Build(Input{HTML: `<div id=g style="grid-template-columns:` + raw + `"></div>`})
	b := findBox(t, built.Root, "g")
	l := newLayouter(built.Root, Size{}, nil, nil)
	allocated := func(f func()) uint64 {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	parsing := allocated(func() { css.ParseComponentValues(raw) })
	var ok bool
	reading := allocated(func() { _, _, ok = l.trackList(b, "grid-template-columns", 0, trackRoom{}) })
	if ok {
		t.Fatal("four hundred thousand tracks were not refused")
	}
	tracks := uint64(4 * maxRepeatedTracks * int(unsafe.Sizeof(gridTrack{})))
	if reading > 2*parsing+tracks {
		t.Errorf("refusing the list allocated %d bytes; parsing it is %d, and %d is "+
			"four times the tracks the bound allows", reading, parsing, tracks)
	}
}

// TestAnAutomaticRepetitionIsBounded is the one track count the stylesheet
// does not write: "repeat(auto-fill, …)" is as many as fit, and a quarter-pixel
// track fits 2400 times in 600 pixels. The repetition stops at the bound and
// the document is told, and the grid is still a grid of that many columns —
// before, a list that came to more than the bound was dropped, the grid laid
// out with none, and nothing said.
func TestAnAutomaticRepetitionIsBounded(t *testing.T) {
	root, said := gridFinding(t, `<div style="display:grid;`+
		`grid-template-columns:repeat(auto-fill,0.25px)">`+
		`<div style="grid-column:`+strconv.Itoa(maxRepeatedTracks)+`">x</div></div>`,
		RuleLimit, "automatic repetition")
	if !said {
		t.Fatal("2400 repetitions were made or dropped and nothing was said")
	}
	item := root.Children[0].Children[0].Children[0]
	want, _ := style.FromPx(0.25 * float64(maxRepeatedTracks-1))
	if item.BorderRect.X != want {
		t.Errorf("the item on line %d is at %vpx; the %d quarter-pixel columns "+
			"before it end at %vpx", maxRepeatedTracks, item.BorderRect.X.Px(),
			maxRepeatedTracks-1, want.Px())
	}
}

// TestAnAutomaticRepetitionCostsTheBound measures it: four times the room is
// the same thousand tracks, and was four times as many made before any were
// compared with the bound.
func TestAnAutomaticRepetitionCostsTheBound(t *testing.T) {
	doc := func(px int) Built {
		return Build(Input{HTML: `<div style="display:grid;width:` + strconv.Itoa(px) +
			`px;grid-template-columns:repeat(auto-fill,1px)"><div>x</div></div>`})
	}
	small, large := doc(100000), doc(400000)
	w, _ := style.FromPx(600)
	c := costtest.Time(t, "an automatic repetition in four times the room",
		func() { Layout(small.Root, Size{W: w, H: w}, nil, nil) },
		func() { Layout(large.Root, Size{W: w, H: w}, nil, nil) })
	if c.Ratio > 2 {
		t.Errorf("four times the room took %.1f times as long (%v against %v); the "+
			"repetition is to stop at the bound, not be made and then refused",
			c.Ratio, c.Large, c.Small)
	}
}

// TestTheImplicitGridIsClamped is §7.1's limit, lowered so that it can be
// watched: items the flow puts past the last track are put in it, and the
// document is told.
func TestTheImplicitGridIsClamped(t *testing.T) {
	defer func(n int) { maxGridTracks = n }(maxGridTracks)
	maxGridTracks = 10
	root, said := gridFinding(t, `<div style="display:grid">`+
		strings.Repeat(`<div class=i style="grid-row:span 3">x</div>`, 6)+`</div>`,
		RuleLimit, "reach past 10 tracks")
	if !said {
		t.Fatal("six items of three rows each in a grid of ten rows said nothing")
	}
	grid := root.Children[0].Children[0]
	if len(grid.Children) != 6 {
		t.Fatalf("the grid has %d items", len(grid.Children))
	}
	// The fourth begins at row nine and is cut to two; the fifth and sixth
	// begin past the tenth and are put in it. So the last three share their
	// bottom edge.
	a, b, c := grid.Children[3], grid.Children[4], grid.Children[5]
	if a.BorderRect.Bottom() != b.BorderRect.Bottom() || b.BorderRect != c.BorderRect {
		t.Errorf("the clamped items are at %v, %v and %v", a.BorderRect, b.BorderRect, c.BorderRect)
	}
}
