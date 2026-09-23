package layout

import "github.com/mgilbir/forme/style"

// Grid placement and track sizing written the simplest way: a table of every
// cell, and track sizing that asks every item about every track. Kept as the
// oracle the sparse and bucketed versions are compared against, because they
// are the definition of where an item goes and how big a track is.
//
// The placement is §8.5 as the specification words it, cell by cell and with
// the specification's own cursor — the corner of the last item placed, not
// the place after it — so that it also checks the argument placeItems makes
// for keeping its cursor one item further on.

func placeItemsDense(items []*gridItem, columns int, dense bool) (rows, cols int) {
	grid := &denseOccupancy{columns: columns, taken: map[[2]int]bool{}}
	// Step 1: both lines named.
	for _, it := range items {
		if it.place[0].definite && it.place[1].definite {
			it.row, it.column = it.place[0].start, it.place[1].start
			grid.fill(it)
		}
	}
	// Step 2: locked to a row. Sparse: the earliest column past any item this
	// step put in that row; dense: the earliest column. Either way it may be
	// past the last column, which makes more.
	last := map[int]int{}
	for _, it := range items {
		if !it.place[0].definite || it.place[1].definite {
			continue
		}
		it.row = it.place[0].start
		c := 0
		if !dense {
			c = last[it.row]
		}
		for !grid.free(it.row, c, it.place[0].span, it.place[1].span) {
			c++
		}
		it.column = c
		grid.fill(it)
		last[it.row] = c + it.place[1].span
	}
	// Step 3 is the columns the caller counted and what step 2 grew them by.
	// Step 4: everything else, from the cursor.
	row, column := 0, 0
	for _, it := range items {
		if it.place[0].definite {
			continue
		}
		if it.place[1].definite {
			if dense {
				row = 0
			} else if it.place[1].start < column {
				row++
			}
			column = it.place[1].start
			for !grid.free(row, column, it.place[0].span, it.place[1].span) {
				row++
			}
			it.row, it.column = row, column
			grid.fill(it)
			continue
		}
		if dense {
			row, column = 0, 0
		}
		for {
			for column+it.place[1].span <= grid.columns &&
				!grid.free(row, column, it.place[0].span, it.place[1].span) {
				column++
			}
			if column+it.place[1].span <= grid.columns {
				break
			}
			row, column = row+1, 0
		}
		it.row, it.column = row, column
		grid.fill(it)
	}
	return grid.rows, grid.columns
}

// denseOccupancy is every cell that is taken.
type denseOccupancy struct {
	columns int
	rows    int
	taken   map[[2]int]bool
}

func (g *denseOccupancy) fill(it *gridItem) {
	end := it.row + it.place[0].span
	if end > g.rows {
		g.rows = end
	}
	if reach := it.column + it.place[1].span; reach > g.columns {
		g.columns = reach
	}
	for r := it.row; r < end; r++ {
		for c := it.column; c < it.column+it.place[1].span; c++ {
			g.taken[[2]int{r, c}] = true
		}
	}
}

// free reports whether a band of cells is empty.
func (g *denseOccupancy) free(row, column, rowSpan, columnSpan int) bool {
	for r := row; r < row+rowSpan; r++ {
		for c := column; c < column+columnSpan; c++ {
			if g.taken[[2]int{r, c}] {
				return false
			}
		}
	}
	return true
}

// resolveTracksByScan is the scan for the base sizes and growth limits. What
// is spent after them — §12.6 to §12.8 — is written out again here step for
// step rather than shared, except for the flexible tracks' fr, whose own
// oracle is frSizeBySpec.
func (l *layouter) resolveTracksByScan(tracks []gridTrack, asks []trackAsk,
	gap, room style.Unit, definite, stretch bool, lo, hi style.Unit) {

	limits := make([]style.Unit, len(tracks))
	for i := range tracks {
		var min, max style.Unit
		for _, a := range asks {
			if a.span != 1 || a.from != i {
				continue
			}
			min, max = style.Max(min, a.min), style.Max(max, a.max)
		}
		tracks[i].base = resolveTrackSize(tracks[i].min, min, max, min)
		limits[i] = resolveTrackSize(tracks[i].max, min, max, max)
	}
	spreadSpanningAsksByScan(tracks, limits, asks, gap)
	if !definite {
		// §12.6 under a max-content constraint: every inflexible track at its
		// growth limit. §12.7 with indefinite free space: one fr is the most
		// any flexible track or any item crossing one asks for.
		flexible := false
		for i := range tracks {
			if tracks[i].flexible() {
				flexible = true
			} else if limits[i] > tracks[i].base {
				tracks[i].base = limits[i]
			}
		}
		if !flexible {
			return
		}
		fr := style.Unit(0)
		for _, t := range tracks {
			if t.flexible() {
				fr = style.Max(fr, t.base.Div(max(t.max.factor, 1)))
			}
		}
		for _, a := range asks {
			if a.from < 0 || a.from+a.span > len(tracks) {
				continue
			}
			for i := a.from; i < a.from+a.span; i++ {
				if tracks[i].flexible() {
					one, _ := frSizeBySpec(tracks[a.from:a.from+a.span], gap, a.max)
					fr = style.Max(fr, one)
					break
				}
			}
		}
		grown := append([]gridTrack(nil), tracks...)
		sum := style.Unit(0)
		for i := range grown {
			if grown[i].flexible() && fr.Mul(grown[i].max.factor) > grown[i].base {
				grown[i].base = fr.Mul(grown[i].max.factor)
			}
			sum = sum.Add(grown[i].base)
		}
		if sum >= lo && sum <= hi {
			copy(tracks, grown)
			return
		}
		// Outside the container's limits: the fr again, against the limit.
		one, _ := frSizeBySpec(tracks, 0, style.Clamp(sum, lo, hi))
		for i := range tracks {
			if tracks[i].flexible() && one.Mul(tracks[i].max.factor) > tracks[i].base {
				tracks[i].base = one.Mul(tracks[i].max.factor)
			}
		}
		return
	}
	free := room.Sub(sumTracks(tracks))
	if free <= 0 {
		return
	}
	free = growToLimitsByScan(tracks, limits, free)
	one, factors := frSizeBySpec(tracks, 0, room)
	flexible := false
	for i := range tracks {
		if !tracks[i].flexible() {
			continue
		}
		flexible = true
		if one.Mul(tracks[i].max.factor) > tracks[i].base {
			tracks[i].base = one.Mul(tracks[i].max.factor)
		}
	}
	if flexible {
		if factors >= 1 {
			return
		}
		free = room.Sub(sumTracks(tracks))
	}
	if stretch {
		stretchAutoTracks(tracks, free)
	}
}

// frSizeBySpec is §12.7.1 as it is written: find the share, take out every
// flexible track whose base is more than its share, and start again, until
// none is.
func frSizeBySpec(tracks []gridTrack, gap, space style.Unit) (style.Unit, float64) {
	fixed := make([]bool, len(tracks))
	for pass := 0; pass <= len(tracks); pass++ {
		leftover := space.Sub(gap.Mul(float64(len(tracks) - 1)))
		factors := 0.0
		for i, t := range tracks {
			if t.flexible() && !fixed[i] {
				factors += t.max.factor
				continue
			}
			leftover = leftover.Sub(t.base)
		}
		if factors == 0 {
			return 0, 0
		}
		one := leftover.Div(max(factors, 1))
		again := false
		for i, t := range tracks {
			if t.flexible() && !fixed[i] && one.Mul(t.max.factor) < t.base {
				fixed[i], again = true, true
			}
		}
		if !again {
			return one, factors
		}
	}
	return 0, 0
}

func spreadSpanningAsksByScan(tracks []gridTrack, limits []style.Unit, asks []trackAsk,
	gap style.Unit) {

	widest := 1
	for _, a := range asks {
		if a.span > widest {
			widest = a.span
		}
	}
	for span := 2; span <= widest; span++ {
		for _, a := range asks {
			if a.span != span || a.from < 0 || a.from+span > len(tracks) {
				continue
			}
			covered := gap.Mul(float64(span - 1))
			intrinsic := 0
			for i := a.from; i < a.from+span; i++ {
				covered = covered.Add(tracks[i].base)
				if tracks[i].min.kind != trackFixed {
					intrinsic++
				}
			}
			if intrinsic == 0 {
				continue
			}
			if short := a.min.Sub(covered); short > 0 {
				share := short.Div(float64(intrinsic))
				for i := a.from; i < a.from+span; i++ {
					if tracks[i].min.kind != trackFixed {
						tracks[i].base = tracks[i].base.Add(share)
						limits[i] = style.Max(limits[i], tracks[i].base)
					}
				}
			}
			// The growth limits take the same treatment with the item's
			// max-content ask, so that a spanning item can grow the tracks it
			// covers as far as it would have grown one of its own.
			room := gap.Mul(float64(span - 1))
			for i := a.from; i < a.from+span; i++ {
				room = room.Add(limits[i])
			}
			if short := a.max.Sub(room); short > 0 {
				share := short.Div(float64(intrinsic))
				for i := a.from; i < a.from+span; i++ {
					if tracks[i].min.kind != trackFixed {
						limits[i] = limits[i].Add(share)
					}
				}
			}
		}
	}
}

// growToLimits is §12.6: the free space is shared equally between the tracks
// that can still take it, and a track that reaches its growth limit stops.
//
// Equally and not in proportion, which is the specification's word and is what
// brings two columns of very different content closer together than their
// content is. The loop cannot run more times than there are tracks: every pass
// either spends everything or freezes at least one track.
func growToLimitsByScan(tracks []gridTrack, limits []style.Unit, free style.Unit) style.Unit {
	for pass := 0; pass <= len(tracks) && free > 0; pass++ {
		growing := 0
		for i := range tracks {
			if !tracks[i].flexible() && tracks[i].base < limits[i] {
				growing++
			}
		}
		if growing == 0 {
			return free
		}
		share := free.Div(float64(growing))
		for i := range tracks {
			if tracks[i].flexible() || tracks[i].base >= limits[i] {
				continue
			}
			want := style.Min(share, limits[i].Sub(tracks[i].base))
			tracks[i].base = tracks[i].base.Add(want)
			free = free.Sub(want)
		}
	}
	return free
}
