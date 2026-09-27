package layout

import (
	"slices"
	"sort"

	"github.com/mgilbir/forme/style"
)

// The pour as it was written before it was made linear: split the remainder at
// one column height, copy what is below, and do it again. It is kept here as
// the oracle the linear pour is compared against, because it is the definition
// of what a pour produces and the new one is an optimisation of it.

func splitAtByCopy(f *Fragment, y style.Unit) (top, bottom *Fragment, ok bool) {
	if f == nil {
		return nil, nil, true
	}
	above, below := *f, *f
	above.Lines, above.Children = nil, nil
	below.Lines, below.Children = nil, nil

	for _, line := range f.Lines {
		switch {
		case line.Rect.Bottom() <= y:
			above.Lines = append(above.Lines, line)
		case line.Rect.Y >= y:
			line.Rect.Y = line.Rect.Y.Sub(y)
			below.Lines = append(below.Lines, line)
		default:
			// A line box straddling the cut. A line is not divisible — it is
			// the unit fragmentation works in — so this is not a height the
			// caller may cut at, and columnBreaks and clearOfLinesByScan are
			// what stop it choosing one.
			return nil, nil, false
		}
	}
	for _, c := range f.Children {
		switch {
		case subtreeBottom(c) <= y:
			kept := *c
			above.Children = append(above.Children, &kept)
		case c.BorderRect.Y >= y:
			moved := *c
			moved.BorderRect.Y = moved.BorderRect.Y.Sub(y)
			below.Children = append(below.Children, &moved)
		default:
			// A box the cut goes through. Its content is divided by the same
			// rule, one level down, and the two halves keep the box between
			// them: a fragmented box is one box in two pieces, so they share a
			// Box and everything that asks what generated them gets one answer.
			t, b, fine := sliceBoxByCopy(c, y)
			if !fine {
				return nil, nil, false
			}
			if t != nil {
				above.Children = append(above.Children, t)
			}
			if b != nil {
				below.Children = append(below.Children, b)
			}
		}
	}
	if len(above.Lines) == 0 && len(above.Children) == 0 {
		return nil, &below, true
	}
	if len(below.Lines) == 0 && len(below.Children) == 0 {
		return &above, nil, true
	}
	return &above, &below, true
}

func fillColumnsByCopy(f *Fragment, c columns, height style.Unit) bool {
	if height <= 0 {
		return false
	}
	bands := make([]*Fragment, 0, c.n)
	rest := f
	start := style.Unit(0)
	for i := 0; i < c.n && rest != nil; i++ {
		cut := clearOfLinesByScan(f, start, start.Add(height))
		top, bottom, ok := splitAtByCopy(rest, cut.Sub(start))
		start = cut
		if !ok {
			return false
		}
		bands = append(bands, top)
		rest = bottom
	}
	if rest != nil {
		// More content than the columns hold. §3.6 overflows it out of the last
		// column, which is a fragmentation of its own and is not done here.
		return false
	}
	f.Lines, f.Children = nil, nil
	for i, band := range bands {
		if band == nil {
			continue
		}
		dx := c.width.Add(c.gap).Mul(float64(i))
		for _, line := range band.Lines {
			line.Rect.X = line.Rect.X.Add(dx)
			f.Lines = append(f.Lines, line)
		}
		for _, child := range band.Children {
			child.BorderRect.X = child.BorderRect.X.Add(dx)
			// The column each child is in is part of what a pour makes: an
			// outer pour reads it (see avoidZones).
			child.column = i + 1
			f.Children = append(f.Children, child)
		}
	}
	return true
}

// fillColumnsByCopyWith is the literal pour with the column ends a forced break
// and a balanced height ask for: each column ends at the first forced break
// after it begins, if that comes within a column height, and otherwise — where
// breaks is given — at the last breakpoint within a column height, found by
// walking the list. With overflow, the columns go on past c.n until nothing is
// left: css-multicol-1 §8.2's overflow columns, placed as the next column
// would be, and refused past maxOverflowColumns of them.
func fillColumnsByCopyWith(f *Fragment, c columns, height style.Unit,
	forced, breaks []style.Unit, overflow bool) bool {

	if height <= 0 {
		return false
	}
	bands := make([]*Fragment, 0, c.n)
	rest := f
	start := style.Unit(0)
	for i := 0; (i < c.n || overflow) && rest != nil; i++ {
		if i >= c.n && len(rest.Lines) == 0 && len(rest.Children) == 0 {
			// Nothing left, and nothing was ever there: a split with nothing
			// above keeps what is below whether or not it holds anything.
			rest = nil
			break
		}
		if i-c.n >= maxOverflowColumns {
			return false
		}
		cut := start.Add(height)
		ended := false
		for _, b := range forced {
			if b > start && b <= cut {
				cut, ended = b, true
				break
			}
		}
		if !ended && breaks != nil {
			for _, b := range breaks {
				if b > start && b <= start.Add(height) {
					cut = b
				}
			}
		}
		if !ended && breaks == nil {
			cut = clearOfLinesByScan(f, start, cut)
		}
		top, bottom, ok := splitAtByCopy(rest, cut.Sub(start))
		if !ok {
			return false
		}
		bands = append(bands, top)
		rest = bottom
		start = cut
	}
	if rest != nil {
		return false
	}
	f.Lines, f.Children = nil, nil
	for i, band := range bands {
		if band == nil {
			continue
		}
		dx := c.width.Add(c.gap).Mul(float64(i))
		for _, line := range band.Lines {
			line.Rect.X = line.Rect.X.Add(dx)
			f.Lines = append(f.Lines, line)
		}
		for _, child := range band.Children {
			child.BorderRect.X = child.BorderRect.X.Add(dx)
			// The column each child is in is part of what a pour makes: an
			// outer pour reads it (see avoidZones).
			child.column = i + 1
			f.Children = append(f.Children, child)
		}
	}
	return true
}

// clearOfLinesByScan is clearOfLines found the long way. While some line
// straddles the cut, the cut moves to the highest top among the lines that
// do, and the lines are looked at again. That is a fixed point rather than
// clearOfLines' merged bands, so the two agreeing is a check on the merging.
//
// Where that reaches the column's start or above it, the answer depends on
// the line the original cut was inside. If it is the only line there, and no
// other line overlaps it, it is taller than the column and overflows it: the
// column ends at its bottom. Otherwise the cut is left where it was, and the
// split refuses it.
//
// It is asked of the tree as it was before the pour, in the coordinates the
// cuts are made in, as clearOfLines is: where the lines were is a fact about
// the content and not about the pour. The two coincide for anything a layout
// makes. They part only for the generator's boxes shorter than their own top
// edge, which get that edge back in every column the pour cuts them across.
func clearOfLinesByScan(f *Fragment, start, y style.Unit) style.Unit {
	var lines [][2]style.Unit
	var walk func(g *Fragment, at style.Unit)
	walk = func(g *Fragment, at style.Unit) {
		for _, line := range g.Lines {
			lines = append(lines, [2]style.Unit{at.Add(line.Rect.Y), at.Add(line.Rect.Bottom())})
		}
		for _, c := range g.Children {
			walk(c, at.Add(c.ContentRect().Y))
		}
	}
	walk(f, 0)
	straddles := func(l [2]style.Unit, cut style.Unit) bool { return l[0] < cut && cut < l[1] }

	cut := y
	for {
		top, straddled := cut, false
		for _, l := range lines {
			if straddles(l, cut) && l[0] < top {
				top, straddled = l[0], true
			}
		}
		if !straddled {
			return cut
		}
		if top > start {
			cut = top
			continue
		}
		// The line the original cut is inside, if it is alone.
		var only [2]style.Unit
		count := 0
		for _, l := range lines {
			if straddles(l, y) {
				only, count = l, count+1
			}
		}
		if count != 1 {
			return y
		}
		for _, l := range lines {
			if l != only && l[1] > l[0] && l[0] < only[1] && only[0] < l[1] {
				return y
			}
		}
		return only[1]
	}
}

// balancedHeightByScan is the shortest height the content fits in, found by
// trying every height a column can have — every breakpoint, and every distance
// between two, which is what a column that begins at one and ends at another
// is — in increasing order.
func balancedHeightByScan(breaks, forced []style.Unit, n int) (style.Unit, bool) {
	if len(breaks) == 0 {
		return 0, true
	}
	candidates := append([]style.Unit(nil), breaks...)
	for i := range breaks {
		for j := i + 1; j < len(breaks); j++ {
			candidates = append(candidates, breaks[j].Sub(breaks[i]))
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	for _, h := range candidates {
		if h > 0 && fitsColumnsByScan(breaks, forced, n, h) {
			return h, true
		}
	}
	if breaks[len(breaks)-1] <= 0 {
		return breaks[len(breaks)-1], true
	}
	return 0, false
}

// fitsColumns reports whether content whose breakpoints are these fits in n
// columns of the given height, filled greedily, with a column ending at every
// forced break but one at the very end.
func fitsColumnsByScan(breaks, forced []style.Unit, n int, height style.Unit) bool {
	if len(breaks) == 0 {
		return true
	}
	used, start := 1, style.Unit(0)
	last := breaks[len(breaks)-1]
	for _, at := range breaks {
		if at.Sub(start) > height {
			// This piece does not fit in the column being filled, so the
			// column ended at the breakpoint before it, and the piece that did
			// not fit begins the next.
			used++
			start = previousBreakByScan(breaks, at)
			if at.Sub(start) > height {
				// One piece taller than a whole column. No number of columns
				// holds it, and a taller column is the only answer.
				return false
			}
		}
		if at < last && slices.Contains(forced, at) {
			used++
			start = at
		}
	}
	return used <= n
}

// previousBreak is the breakpoint before this one, or zero.
func previousBreakByScan(breaks []style.Unit, at style.Unit) style.Unit {
	prev := style.Unit(0)
	for _, b := range breaks {
		if b >= at {
			break
		}
		prev = b
	}
	return prev
}

func sliceBoxByCopy(c *Fragment, y style.Unit) (top, bottom *Fragment, ok bool) {
	// The cut in the child's own content coordinates, which is where its lines
	// and children are measured from. It may fall outside them at either end: a
	// box whose own border box is entirely above the cut can still hold content
	// that reaches past it, which is what a float overflowing its parent is.
	inner := y.Sub(c.ContentRect().Y)
	t, b, fine := splitAtByCopy(c, inner)
	if !fine {
		return nil, nil, false
	}
	// How much of the box's own border box falls on each side. A box whose
	// content overflows it has one of these at its full height and the other at
	// nothing, which is the fragment that carries the overflow onward.
	topH := style.Max(0, style.Min(c.BorderRect.Bottom(), y).Sub(c.BorderRect.Y))
	bottomH := style.Max(0, c.BorderRect.Bottom().Sub(style.Max(c.BorderRect.Y, y)))
	// A box with nothing in it is divided by its own extent and not by its
	// content, which is what splitAt above reports on: a fragment holding no
	// lines and no children is nothing on both sides as far as that walk can
	// see, and a float is exactly such a box. So each side is made here where
	// the box reaches into it and the walk found nothing to put there.
	if t == nil && topH > 0 {
		empty := *c
		empty.Lines, empty.Children = nil, nil
		t = &empty
	}
	if b == nil && bottomH > 0 {
		empty := *c
		empty.Lines, empty.Children = nil, nil
		b = &empty
	}
	if topH > 0 && bottomH > 0 && refusesToSlice(c) {
		// The box itself is being cut, and it is one whose picture slicing
		// cannot draw. A box merely *holding* content that crosses the cut is
		// not cut at all and is not refused: it is in one column with a
		// zero-height fragment of itself in the next.
		return nil, nil, false
	}
	if t != nil {
		// The first fragment: its top edge is the box's own, its bottom edge is
		// the cut and has nothing on it.
		t.BorderRect.H = topH
		if bottomH > 0 {
			t.Border.Bottom, t.Padding.Bottom, t.Margin.Bottom = 0, 0, 0
		}
		t.contentH = t.BorderRect.H
	}
	if b != nil {
		// And the last: it begins at the cut with nothing on that edge, and
		// keeps the box's own bottom.
		b.BorderRect.Y = style.Max(0, c.BorderRect.Y.Sub(y))
		b.BorderRect.H = bottomH
		if topH > 0 {
			b.Border.Top, b.Padding.Top, b.Margin.Top = 0, 0, 0
		}
		b.contentH = b.BorderRect.H
	}
	return t, b, true
}

// subtreeBottom is how far a fragment's own box and everything it holds reach
// below its parent's content edge.
//
// It is not the border box, and the difference is what a float is: a float
// inside a container taller than the container overflows it, and the container's
// own rectangle says nothing about where the float ends. A cut chosen from the
// container's box alone would put the whole float in one column.
func subtreeBottom(f *Fragment) style.Unit {
	if f == nil {
		return 0
	}
	out := f.BorderRect.Bottom()
	inner := f.ContentRect()
	for _, line := range f.Lines {
		out = style.Max(out, inner.Y.Add(line.Rect.Bottom()))
	}
	for _, c := range f.Children {
		out = style.Max(out, inner.Y.Add(subtreeBottom(c)))
	}
	return out
}
