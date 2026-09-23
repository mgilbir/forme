package layout

import "github.com/mgilbir/forme/style"

// Grid placement and track sizing as they were written before they were made
// proportional to the items: a dense occupancy of every cell, and track sizing
// that asked every item about every track. Kept as the oracle the sparse and
// bucketed versions are compared against, because they are the definition of
// where an item goes and how big a track is.

func placeItemsDense(items []*gridItem, columns int, dense bool) int {
	grid := &denseOccupancy{columns: columns}
	var flow []*gridItem
	for _, it := range items {
		switch {
		case it.place[0].definite && it.place[1].definite:
			it.row, it.column = it.place[0].start, it.place[1].start
			grid.fill(it)
		case it.place[0].definite:
			it.row = it.place[0].start
			it.column = grid.freeInRow(it.row, it.place[1].span)
			grid.fill(it)
		default:
			flow = append(flow, it)
		}
	}
	row, column := 0, 0
	for _, it := range flow {
		if dense {
			// §8.5's dense packing: the cursor goes back to the start for every
			// item, so a small one later in the document fills a hole a wide
			// one left behind. Sparse packing is the default because it keeps
			// the items in the order they were written; dense trades that for
			// a grid with no gaps in it.
			row, column = 0, 0
		}
		if it.place[1].definite {
			// A definite column and no row: the item drops down the column
			// until it finds a row with room for it, starting from the cursor's
			// row so that the order the items were written in is kept.
			it.column = it.place[1].start
			it.row = grid.freeInColumn(row, it.column, it.place[0].span,
				it.place[1].span)
			grid.fill(it)
			continue
		}
		it.row, it.column = grid.next(row, column, it.place[0].span, it.place[1].span)
		grid.fill(it)
		row, column = it.row, it.column+it.place[1].span
		if column >= grid.columns {
			row, column = row+1, 0
		}
	}
	return grid.rows
}

// denseOccupancy is which cells are taken, which is all §8.5 needs to remember.
//
// It grows downwards and never sideways: the number of columns is settled
// before any of this runs — the template says how many there are — and a row is
// made whenever an item needs one that is not there yet.
type denseOccupancy struct {
	columns int
	rows    int
	taken   []bool
}

func (g *denseOccupancy) at(row, column int) bool {
	if column < 0 || column >= g.columns || row < 0 {
		return false
	}
	if i := row*g.columns + column; i < len(g.taken) {
		return g.taken[i]
	}
	return false
}

func (g *denseOccupancy) fill(it *gridItem) {
	end := it.row + it.place[0].span
	if end > g.rows {
		g.rows = end
	}
	for len(g.taken) < g.rows*g.columns {
		g.taken = append(g.taken, false)
	}
	for r := it.row; r < end; r++ {
		for c := it.column; c < it.column+it.place[1].span && c < g.columns; c++ {
			g.taken[r*g.columns+c] = true
		}
	}
}

// free reports whether a band of cells is empty and inside the grid.
func (g *denseOccupancy) free(row, column, rowSpan, columnSpan int) bool {
	if column < 0 || column+columnSpan > g.columns {
		return false
	}
	for r := row; r < row+rowSpan; r++ {
		for c := column; c < column+columnSpan; c++ {
			if g.at(r, c) {
				return false
			}
		}
	}
	return true
}

// freeInRow is the first column in one row where a span will fit.
func (g *denseOccupancy) freeInRow(row, span int) int {
	for c := 0; c+span <= g.columns; c++ {
		if g.free(row, c, 1, span) {
			return c
		}
	}
	return 0
}

// freeInColumn is the first row at or after one where a span will fit in a
// given column.
func (g *denseOccupancy) freeInColumn(from, column, rowSpan, columnSpan int) int {
	for r := from; ; r++ {
		if g.free(r, column, rowSpan, columnSpan) {
			return r
		}
		if r > g.rows+len(g.taken) {
			// Unreachable while the grid grows downwards: a row past the last
			// filled one is empty. The bound is here because the loop has no
			// other end, and a document is untrusted.
			return r
		}
	}
}

// next is where the cursor finds room for an item, walking along the columns
// and then down.
func (g *denseOccupancy) next(row, column, rowSpan, columnSpan int) (int, int) {
	for r := row; ; r++ {
		start := 0
		if r == row {
			start = column
		}
		for c := start; c+columnSpan <= g.columns; c++ {
			if g.free(r, c, rowSpan, columnSpan) {
				return r, c
			}
		}
		if r > g.rows+len(g.taken) {
			return r, 0
		}
	}
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
