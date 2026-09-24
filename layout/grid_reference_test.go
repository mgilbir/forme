package layout

import (
	"math"

	"github.com/mgilbir/forme/style"
)

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

// resolveTracksByScan is the base sizes and growth limits by intrinsicSizesBySpec,
// and what is spent after them — §12.6 to §12.8 — written out again here step
// for step rather than shared, except for the flexible tracks' fr, whose own
// oracle is frSizeBySpec.
func (l *layouter) resolveTracksByScan(tracks []gridTrack, asks []trackAsk,
	gap, room style.Unit, definite, stretch bool, lo, hi style.Unit) {

	limits := intrinsicSizesBySpec(tracks, asks, gap, sizedForLayout)
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

// intrinsicSizesBySpec is §12.4 and §12.5 as they are written, and the oracle
// trackBasesAndLimits is held to: every span from one up walked over every
// item, a planned increase kept for every track in the grid and every track
// updated after every round, and the space shared by the words of §12.5.1 —
// equally, freezing the tracks that reach their limit, and again among the
// rest until none does — rather than by the level the sorted rooms give.
func intrinsicSizesBySpec(tracks []gridTrack, asks []trackAsk, gap style.Unit,
	under sizingConstraint) []style.Unit {

	n := len(tracks)
	limit := make([]style.Unit, n)
	infinite := make([]bool, n)
	growable := make([]bool, n)
	for i := range tracks {
		tracks[i].base = 0
		if tracks[i].min.kind == trackFixed {
			tracks[i].base = maxZero(tracks[i].min.size)
		}
		if tracks[i].max.kind == trackFixed {
			limit[i] = style.Max(maxZero(tracks[i].max.size), tracks[i].base)
		} else {
			infinite[i] = true
		}
	}
	valid := func(a trackAsk) bool { return a.span >= 1 && a.from >= 0 && a.from+a.span <= n }
	crosses := func(a trackAsk) bool {
		for i := a.from; i < a.from+a.span; i++ {
			if tracks[i].flexible() {
				return true
			}
		}
		return false
	}
	gaps := func(a trackAsk) style.Unit { return gap.Mul(float64(a.span - 1)) }
	// Grid §6.6, by its three conditions.
	minimum := func(a trackAsk) style.Unit {
		if !a.automatic {
			return a.minimum
		}
		auto, fixed := false, true
		sum := gaps(a)
		for i := a.from; i < a.from+a.span; i++ {
			if tracks[i].min.kind == trackAuto {
				auto = true
			}
			if tracks[i].max.kind == trackFixed {
				sum = sum.Add(maxZero(tracks[i].max.size))
			} else {
				fixed = false
			}
		}
		if !auto || (a.span > 1 && crosses(a)) {
			return a.minimum
		}
		c := a.min
		if fixed && sum < c {
			c = sum
		}
		return style.Max(c, a.minimum)
	}
	limited := func(a trackAsk, c style.Unit) style.Unit {
		sum := gaps(a)
		for i := a.from; i < a.from+a.span; i++ {
			if tracks[i].max.kind != trackFixed {
				return style.Max(c, minimum(a))
			}
			sum = sum.Add(maxZero(tracks[i].max.size))
		}
		return style.Max(style.Min(c, sum), minimum(a))
	}
	intrinsic := func(k trackKind) bool { return k == trackAuto || k == trackMin || k == trackMax }
	maxContent := func(k trackKind) bool { return k == trackAuto || k == trackMax }

	// One round of §12.5.1 over a group.
	round := func(items []trackAsk, flex bool, what accommodation,
		affects func(gridTrack) bool, contribution func(trackAsk) style.Unit) {

		size := func(i int) style.Unit {
			if what == intoLimits && !infinite[i] {
				return limit[i]
			}
			return tracks[i].base
		}
		bound := func(i int) (style.Unit, bool) {
			if infinite[i] || (what == intoLimits && growable[i]) {
				return 0, false
			}
			return limit[i], true
		}
		planned := make([]style.Unit, n)
		considered := make([]bool, n)
		for _, a := range items {
			var affected, other []int
			need := contribution(a).Sub(gaps(a))
			for i := a.from; i < a.from+a.span; i++ {
				need = need.Sub(size(i))
				if affects(tracks[i]) && (!flex || tracks[i].flexible()) {
					affected = append(affected, i)
				} else {
					other = append(other, i)
				}
			}
			if len(affected) == 0 {
				continue
			}
			for _, i := range affected {
				considered[i] = true
			}
			space := float64(maxZero(need))
			if space == 0 {
				continue
			}
			weight := make([]float64, n)
			sum, flexCount := 0.0, 0
			for i := a.from; i < a.from+a.span; i++ {
				if tracks[i].flexible() {
					sum += tracks[i].max.factor
					flexCount++
				}
			}
			for i := a.from; i < a.from+a.span; i++ {
				switch {
				case !flex || !tracks[i].flexible():
					weight[i] = 1
				case sum >= 1:
					weight[i] = tracks[i].max.factor / sum
				default:
					weight[i] = tracks[i].max.factor + (1-sum)/float64(flexCount)
				}
			}
			inc := make([]style.Unit, n)
			// "distributing the space equally among these tracks, freezing a
			// track's item-incurred increase as its affected size +
			// item-incurred increase reaches its limit (and continuing to grow
			// the unfrozen tracks as needed)".
			share := func(set []int, bounded bool) {
				var sharing []int
				for _, i := range set {
					if weight[i] > 0 {
						sharing = append(sharing, i)
					}
				}
				w := func(i int) float64 { return weight[i] }
				if len(sharing) == 0 {
					sharing = set
					w = func(int) float64 { return 1 }
				}
				frozen := map[int]bool{}
				for {
					total := 0.0
					var active []int
					for _, i := range sharing {
						if !frozen[i] {
							active = append(active, i)
							total += w(i)
						}
					}
					if len(active) == 0 {
						return
					}
					froze := false
					for _, i := range active {
						if !bounded {
							break
						}
						b, finite := bound(i)
						if !finite {
							continue
						}
						room := maxZero(b.Sub(size(i).Add(inc[i])))
						if float64(room)*total <= space*w(i) {
							inc[i] = inc[i].Add(room)
							frozen[i] = true
							froze = true
						}
					}
					if froze {
						space = float64(maxZero(need))
						for j := a.from; j < a.from+a.span; j++ {
							space -= float64(inc[j])
						}
						continue
					}
					for _, i := range active {
						inc[i] = inc[i].Add(style.Unit(math.Trunc(space / total * w(i))))
					}
					space = 0
					return
				}
			}
			share(affected, true)
			if space > 0 && len(other) > 0 {
				share(other, true)
			}
			if space > 0 {
				var beyond []int
				for _, i := range affected {
					if what == intoBaseForMaxContent && maxContent(tracks[i].max.kind) ||
						what != intoBaseForMaxContent && intrinsic(tracks[i].max.kind) {
						beyond = append(beyond, i)
					}
				}
				if len(beyond) == 0 && what != intoLimits {
					beyond = affected
				}
				if len(beyond) > 0 {
					share(beyond, false)
				}
			}
			for i := range inc {
				if inc[i] > 0 {
					considered[i] = true
					planned[i] = style.Max(planned[i], inc[i])
				}
			}
		}
		for i := range tracks {
			if !considered[i] {
				continue
			}
			switch {
			case what != intoLimits:
				tracks[i].base = tracks[i].base.Add(planned[i])
			case infinite[i]:
				limit[i], infinite[i], growable[i] = tracks[i].base.Add(planned[i]), false, true
			default:
				limit[i] = limit[i].Add(planned[i])
			}
		}
	}
	group := func(items []trackAsk, flex bool) {
		round(items, flex, intoBaseForMinimums,
			func(t gridTrack) bool { return intrinsic(t.min.kind) },
			func(a trackAsk) style.Unit {
				if under == sizedForLayout {
					return minimum(a)
				}
				return limited(a, a.min)
			})
		round(items, flex, intoBaseForMinimums,
			func(t gridTrack) bool { return t.min.kind == trackMin || t.min.kind == trackMax },
			func(a trackAsk) style.Unit { return a.min })
		if under == underMaxContent {
			round(items, flex, intoBaseForMaxContent,
				func(t gridTrack) bool { return t.min.kind == trackAuto || t.min.kind == trackMax },
				func(a trackAsk) style.Unit { return limited(a, a.max) })
		}
		round(items, flex, intoBaseForMaxContent,
			func(t gridTrack) bool { return t.min.kind == trackMax },
			func(a trackAsk) style.Unit { return a.max })
		for i := range tracks {
			if !infinite[i] && limit[i] < tracks[i].base {
				limit[i] = tracks[i].base
			}
		}
		round(items, flex, intoLimits,
			func(t gridTrack) bool { return intrinsic(t.max.kind) },
			func(a trackAsk) style.Unit { return a.min })
		round(items, flex, intoLimits,
			func(t gridTrack) bool { return maxContent(t.max.kind) },
			func(a trackAsk) style.Unit { return a.max })
		for i := range growable {
			growable[i] = false
		}
	}
	widest := 0
	for _, a := range asks {
		if valid(a) && a.span > widest {
			widest = a.span
		}
	}
	for span := 1; span <= widest; span++ {
		var items []trackAsk
		for _, a := range asks {
			if valid(a) && a.span == span && !crosses(a) {
				items = append(items, a)
			}
		}
		if len(items) > 0 {
			group(items, false)
		}
	}
	var flexItems []trackAsk
	for _, a := range asks {
		if valid(a) && crosses(a) {
			flexItems = append(flexItems, a)
		}
	}
	if len(flexItems) > 0 {
		group(flexItems, true)
	}
	for i := range tracks {
		if infinite[i] {
			limit[i] = tracks[i].base
		}
	}
	return limit
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
