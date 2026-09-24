package layout

import (
	"cmp"
	"slices"
	"sort"
	"strconv"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// CSS Multi-column Layout 1: a block whose content is poured into columns.
//
// # What it is
//
// A multicol container lays its content out once, in a box one column wide, and
// then cuts the result into columns and stands them side by side. That is not an
// implementation shortcut — it is what the specification describes, and it is
// why the feature is called fragmentation rather than layout: the content is
// laid out and *then* divided, so nothing inside it needs to know how many
// columns there are or where they end.
//
// It is also this engine's first fragmentation of any kind. Nothing else here
// cuts a laid-out box in two — the page is one page and a block is one block —
// so the pieces below are the primitive rather than a use of one.
//
// # What is not done
//
// A great deal, and each of it is refused with a finding rather than
// approximated. Anything that would have to be cut *through* is refused: a box
// with a border, a background or an outline crossing a column boundary is two
// halves of a drawn thing, and CSS says which edges each half keeps.
// "column-span" is refused because a spanning element divides the multicol into
// two of them.
//
// A float is not refused, and this comment used to say it was. It is laid out
// in the tall column with everything else and cut with it — a float is a box
// like any other to the pour, sliced where a column ends — and it is placed in
// the multicol container's own formatting context, because §2 makes a multicol
// container establish one. See sealsFloats: before it did, the float's uncut
// rectangle was left in the parent's context after the pour, and the text after
// the container was indented round it.
//
// The gate is the same shape as the writing-mode one and for the same reason:
// what is refused is laid out exactly as it was before this file existed, in one
// column, and is reported — which is the honest answer — while what is accepted
// is the answer CSS gives.

// columnFill is CSS Multi-column §3.5's column-fill.
type columnFill uint8

const (
	// columnBalance is the initial value: the columns are as short as they can
	// be while holding the content, and equal.
	columnBalance columnFill = iota
	// columnAuto fills each column to the available height in turn, so the last
	// one is as short as what is left.
	columnAuto
)

// columns is what §3.4's algorithm resolves a box's declarations to.
type columns struct {
	// n is the used column count and width the used column width. The two are
	// resolved together: §3.4 takes the count and the width as constraints and
	// produces a pair that fills the available space exactly.
	n     int
	width style.Unit
	gap   style.Unit
	fill  columnFill
}

// columnGap is §6.1's gap, whose initial value "normal" is one em for a multicol
// container.
//
// Resolved here rather than in the cascade because an em is the box's own, and a
// computed value of "1em" would resolve against whatever font the reader of the
// computed style happened to have.
func (l *layouter) columnGap(b *Box) style.Unit {
	if v, ok := l.lengthOf(b, "column-gap", 0); ok && v >= 0 {
		return v
	}
	return b.FontSize
}

// columnsFor resolves §3.4's algorithm for a box, or says the box is not a
// multicol container.
//
// available is the width the content has: the box's content box, which the
// columns and the gaps between them divide exactly.
//
// The algorithm is the specification's, written out in its own order. Both
// declarations are constraints and neither is a result: a count says how many
// columns there are to be and a width says how narrow they may be, and where
// both are given the count is a maximum that the width may reduce.
func (l *layouter) columnsFor(b *Box, available style.Unit) (columns, bool) {
	count, hasCount := columnCount(b)
	width, hasWidth := l.lengthOf(b, "column-width", 0)
	if hasWidth && width <= 0 {
		// A zero or negative width is not a length this can divide by. §3.2
		// makes it invalid; the cascade admits it, so it is declined here.
		hasWidth = false
	}
	if !hasCount && !hasWidth {
		// "column-width: auto" and "column-count: auto" together: not a
		// multicol container at all, which is the ordinary case for every box
		// in every document.
		return columns{}, false
	}
	gap := l.columnGap(b)
	out := columns{gap: gap, fill: columnFillOf(b)}
	switch {
	case !hasWidth:
		out.n = count
	case !hasCount:
		out.n = fitCount(available, width, gap)
	default:
		if n := fitCount(available, width, gap); n < count {
			out.n = n
		} else {
			out.n = count
		}
	}
	if out.n < 1 {
		out.n = 1
	}
	// The used width, which is what is left once the gaps are taken out, shared
	// equally. It is not the declared column-width: that is a minimum the used
	// value is at least as large as, and the columns fill the box.
	out.width = available.Sub(gap.Mul(float64(out.n - 1))).Div(float64(out.n))
	if out.width < 0 {
		out.width = 0
	}
	return out, true
}

// fitCount is how many columns of a given width fit in the available space with
// a gap between each pair: §3.4's "floor((available + gap) / (width + gap))".
func fitCount(available, width, gap style.Unit) int {
	if width+gap <= 0 {
		return 1
	}
	n := int((available.Add(gap)).Px() / (width.Add(gap)).Px())
	if n < 1 {
		return 1
	}
	return n
}

// columnCount reads §3.1's property: a positive integer, or auto.
func columnCount(b *Box) (int, bool) {
	raw := ascii.TrimCSSSpace(b.Style.Get("column-count"))
	if raw == "" || ascii.EqualFold(raw, "auto") {
		return 0, false
	}
	n, ok := positiveInteger(raw)
	if !ok || n < 1 {
		return 0, false
	}
	return n, true
}

// columnFillOf reads §3.5's property.
func columnFillOf(b *Box) columnFill {
	if ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("column-fill")), "auto") {
		return columnAuto
	}
	return columnBalance
}

// A pour divides a fragment's content at a column height, again and again, and
// is this engine's only fragmentation.
//
// One cut is simple to state. The fragment given is the one holding the
// content — a multicol container, or a box inside one — and the cut is a
// distance down its *content* box. What falls above it stays where it is, and
// what falls below it moves up so that the cut becomes its own zero. A box the
// cut goes through is copied, once for each side, and each copy keeps the lines
// and the children that fall on its side: that is what CSS describes for a
// fragmented box, and it is why the two halves are fragments of one box rather
// than two boxes — they share a Box, so everything that asks what generated
// them gets one answer. The cut is refused where it would go through something
// drawn: a line, or a border, background or outline CSS says which half keeps.
//
// A pour is that cut made once per column, each time to what the last one left
// below. Made literally — copy everything below the cut, then cut the copy —
// it copied every remaining line once per column, and a paragraph of a word
// per line in a thousand columns was thirty-two thousand lines copied thirty-two
// thousand times: seventy-six seconds for 64 KB of markup.
//
// So what is left below a cut is not copied. It is a pending fragment: the
// original's lines and children, untouched, with the distance they have moved
// up held once beside them instead of written into each. A line or a box that
// no cut goes through moves by exactly that distance and nothing else, so it is
// copied once, into the column it lands in, when it lands there. Which column
// that is follows from where it is, so each is found by keeping the untouched
// items in order of where they end and where they begin and taking them off the
// front as the cuts pass them. Only a box a cut goes through is copied at the
// cut, and its content is pending in turn.
//
// The order of what lands in a column is the order the original held it in,
// which is what a copy of the remainder would have kept: a box's children are
// painted in that order and some overlap.
//
// multicol_reference_test.go keeps the literal pour, and the tests hold this
// one to it.

// pending is a fragment whose content has not been copied yet.
type pending struct {
	// root is the fragment's own box: what a cut copies for each side. Its
	// lines and children are not read; the content is src's.
	root Fragment
	src  *Fragment
	// shift is how far src's untouched content has moved up, in src's content
	// coordinates.
	shift style.Unit

	// The untouched lines and children of src, each listed twice: by where it
	// ends, which is the order the cuts take them off above, and by where it
	// begins, which is the order they are found straddling one. gone marks the
	// ones already taken by the other list.
	linesByEnd, linesByStart []int
	lineGone                 []bool
	linesLeft                int
	kidsByEnd, kidsByStart   []int
	kidGone                  []bool
	kidsLeft                 int

	// cut is the children a cut has gone through, each the part below the cut
	// and each at its place among src's children. See split.
	cut []cutKid

	p *pour
}

// cutKid is a child a cut went through: the part of it still to be poured, and
// the index of the child it is part of, which is where it goes in the list.
type cutKid struct {
	index int
	rest  *pending
}

// pour is one pour's state: the ends of the original subtrees, which are asked
// for again and again and do not change, and how many fragments it has made.
// See maxPourPieces.
type pour struct {
	ends map[*Fragment]style.Unit
	made int
}

// end is subtreeBottom, once per fragment per pour.
func (p *pour) end(f *Fragment) style.Unit {
	if v, ok := p.ends[f]; ok {
		return v
	}
	out := f.BorderRect.Bottom()
	inner := f.ContentRect()
	for _, line := range f.Lines {
		out = style.Max(out, inner.Y.Add(line.Rect.Bottom()))
	}
	for _, c := range f.Children {
		out = style.Max(out, inner.Y.Add(p.end(c)))
	}
	p.ends[f] = out
	return out
}

// pend makes a fragment pending, with nothing moved yet.
func (p *pour) pend(root Fragment, src *Fragment) *pending {
	n := &pending{root: root, src: src, p: p}
	n.linesByEnd = make([]int, len(src.Lines))
	n.linesByStart = make([]int, len(src.Lines))
	for i := range src.Lines {
		n.linesByEnd[i], n.linesByStart[i] = i, i
	}
	slices.SortStableFunc(n.linesByEnd, func(a, b int) int {
		return cmp.Compare(src.Lines[a].Rect.Bottom(), src.Lines[b].Rect.Bottom())
	})
	slices.SortStableFunc(n.linesByStart, func(a, b int) int {
		return cmp.Compare(src.Lines[a].Rect.Y, src.Lines[b].Rect.Y)
	})
	n.lineGone, n.linesLeft = make([]bool, len(src.Lines)), len(src.Lines)

	n.kidsByEnd = make([]int, len(src.Children))
	n.kidsByStart = make([]int, len(src.Children))
	for i := range src.Children {
		n.kidsByEnd[i], n.kidsByStart[i] = i, i
	}
	slices.SortStableFunc(n.kidsByEnd, func(a, b int) int {
		return cmp.Compare(p.end(src.Children[a]), p.end(src.Children[b]))
	})
	slices.SortStableFunc(n.kidsByStart, func(a, b int) int {
		return cmp.Compare(src.Children[a].BorderRect.Y, src.Children[b].BorderRect.Y)
	})
	n.kidGone, n.kidsLeft = make([]bool, len(src.Children)), len(src.Children)
	return n
}

// empty says nothing is left to pour.
func (n *pending) empty() bool {
	return n.linesLeft == 0 && n.kidsLeft == 0 && len(n.cut) == 0
}

// extent is subtreeBottom of what is left: how far the fragment and the content
// still in it reach below its parent's content edge.
func (n *pending) extent() style.Unit {
	out := n.root.BorderRect.Bottom()
	var content style.Unit
	has := false
	// The last of the by-end lists is the furthest-reaching of what is left,
	// once the ones already taken are dropped off that end as well.
	for k := len(n.linesByEnd); k > 0 && n.lineGone[n.linesByEnd[k-1]]; k-- {
		n.linesByEnd = n.linesByEnd[:k-1]
	}
	if k := len(n.linesByEnd); k > 0 {
		content, has = n.src.Lines[n.linesByEnd[k-1]].Rect.Bottom().Sub(n.shift), true
	}
	for k := len(n.kidsByEnd); k > 0 && n.kidGone[n.kidsByEnd[k-1]]; k-- {
		n.kidsByEnd = n.kidsByEnd[:k-1]
	}
	if k := len(n.kidsByEnd); k > 0 {
		v := n.p.end(n.src.Children[n.kidsByEnd[k-1]]).Sub(n.shift)
		if !has || v > content {
			content, has = v, true
		}
	}
	for _, c := range n.cut {
		v := c.rest.extent()
		if !has || v > content {
			content, has = v, true
		}
	}
	if has {
		out = style.Max(out, n.root.ContentRect().Y.Add(content))
	}
	return out
}

// materialise copies out everything still pending, as a fragment.
func (n *pending) materialise() *Fragment {
	n.p.made += 1 + n.linesLeft + n.kidsLeft
	f := n.root
	f.Lines, f.Children = nil, nil
	for i, line := range n.src.Lines {
		if n.lineGone[i] {
			continue
		}
		line.Rect.Y = line.Rect.Y.Sub(n.shift)
		f.Lines = append(f.Lines, line)
	}
	kids := make([]cutKid, 0, n.kidsLeft+len(n.cut))
	for i := range n.src.Children {
		if !n.kidGone[i] {
			kids = append(kids, cutKid{index: i})
		}
	}
	kids = append(kids, n.cut...)
	slices.SortStableFunc(kids, func(a, b cutKid) int { return a.index - b.index })
	for _, k := range kids {
		if k.rest != nil {
			f.Children = append(f.Children, k.rest.materialise())
			continue
		}
		moved := *n.src.Children[k.index]
		moved.BorderRect.Y = moved.BorderRect.Y.Sub(n.shift)
		f.Children = append(f.Children, &moved)
	}
	return &f
}

// split cuts what is left at y, a distance down the fragment's content box as it
// now stands. It returns the part above as a fragment, and leaves the part below
// pending in n with y as its new zero, or says the cut goes through something
// that cannot be cut.
//
// top is nil where nothing was above, and below is false where nothing was left
// below — in which case n is spent. Where nothing was above, n is kept whether
// or not anything is left in it, which is what the literal cut did: a fragment
// with no content comes back as itself below the cut, so a pour of one never
// ends.
func (n *pending) split(y style.Unit) (top *Fragment, below, ok bool) {
	at := n.shift.Add(y) // the cut, in src's coordinates
	above := n.root
	above.Lines, above.Children = nil, nil

	// Lines. Everything ending at or above the cut is above it; anything left
	// that begins above it goes through it, and a line cannot be divided.
	var took []int
	for len(n.linesByEnd) > 0 {
		i := n.linesByEnd[0]
		if n.lineGone[i] {
			n.linesByEnd = n.linesByEnd[1:]
			continue
		}
		if n.src.Lines[i].Rect.Bottom() > at {
			break
		}
		n.linesByEnd = n.linesByEnd[1:]
		n.lineGone[i], n.linesLeft = true, n.linesLeft-1
		took = append(took, i)
	}
	for len(n.linesByStart) > 0 && n.lineGone[n.linesByStart[0]] {
		n.linesByStart = n.linesByStart[1:]
	}
	if len(n.linesByStart) > 0 && n.src.Lines[n.linesByStart[0]].Rect.Y < at {
		// A line box straddling the cut. A line is not divisible — it is the
		// unit fragmentation works in — so this is not a height the caller may
		// cut at, and columnBreaks is what stops it choosing one.
		return nil, false, false
	}
	slices.Sort(took)
	for _, i := range took {
		line := n.src.Lines[i]
		line.Rect.Y = line.Rect.Y.Sub(n.shift)
		above.Lines = append(above.Lines, line)
	}

	// Children. The ones already cut first: each is a box of its own, placed in
	// the coordinates of what is left, and asked the three questions a child
	// is asked — above, below, or through.
	var kids []placedKid
	still := n.cut
	n.cut = nil
	for _, c := range still {
		switch {
		case c.rest.extent() <= y:
			kids = append(kids, placedKid{c.index, c.rest.materialise()})
		case c.rest.root.BorderRect.Y >= y:
			c.rest.root.BorderRect.Y = c.rest.root.BorderRect.Y.Sub(y)
			n.cut = append(n.cut, c)
		default:
			t, b, fine := c.rest.slice(y)
			if !fine {
				return nil, false, false
			}
			if t != nil {
				kids = append(kids, placedKid{c.index, t})
			}
			if b != nil {
				n.cut = append(n.cut, cutKid{c.index, b})
			}
		}
	}
	// Then the untouched ones: everything ending at or above the cut is above
	// it, whole, and anything left that begins above it is one the cut goes
	// through.
	for len(n.kidsByEnd) > 0 {
		i := n.kidsByEnd[0]
		if n.kidGone[i] {
			n.kidsByEnd = n.kidsByEnd[1:]
			continue
		}
		if n.p.end(n.src.Children[i]) > at {
			break
		}
		n.kidsByEnd = n.kidsByEnd[1:]
		n.kidGone[i], n.kidsLeft = true, n.kidsLeft-1
		kept := *n.src.Children[i]
		kept.BorderRect.Y = kept.BorderRect.Y.Sub(n.shift)
		kids = append(kids, placedKid{i, &kept})
	}
	for {
		for len(n.kidsByStart) > 0 && n.kidGone[n.kidsByStart[0]] {
			n.kidsByStart = n.kidsByStart[1:]
		}
		if len(n.kidsByStart) == 0 || n.src.Children[n.kidsByStart[0]].BorderRect.Y >= at {
			break
		}
		i := n.kidsByStart[0]
		n.kidsByStart = n.kidsByStart[1:]
		n.kidGone[i], n.kidsLeft = true, n.kidsLeft-1
		// A box the cut goes through. Its content is divided by the same rule,
		// one level down, and the two halves keep the box between them.
		c := n.src.Children[i]
		root := *c
		root.BorderRect.Y = root.BorderRect.Y.Sub(n.shift)
		t, b, fine := n.p.pend(root, c).slice(y)
		if !fine {
			return nil, false, false
		}
		if t != nil {
			kids = append(kids, placedKid{i, t})
		}
		if b != nil {
			n.cut = append(n.cut, cutKid{i, b})
		}
	}
	n.p.made += len(took) + len(kids)
	slices.SortStableFunc(kids, func(a, b placedKid) int { return a.index - b.index })
	for _, k := range kids {
		above.Children = append(above.Children, k.frag)
	}
	slices.SortStableFunc(n.cut, func(a, b cutKid) int { return a.index - b.index })

	// What is left moves up by the cut. The cut pieces already have: slice
	// places the part below at its own zero, and one that was below the cut
	// was moved above.
	n.shift = at

	if len(above.Lines) == 0 && len(above.Children) == 0 {
		return nil, true, true
	}
	if n.empty() {
		return &above, false, true
	}
	return &above, true, true
}

type placedKid struct {
	index int
	frag  *Fragment
}

// slice divides one box at a height in its parent's content coordinates, and
// gives each half the edges CSS Fragmentation assigns it. n is the box, with
// its content still pending; the part below the cut is n again, the part above
// is copied out.
//
// §4.4's "box-decoration-break: slice", which is the initial value and the only
// one this engine does: the box is rendered as though it were never divided and
// then cut, so no border appears at the cut on either side. The top border and
// padding go to the first fragment, the bottom to the last, and the two sides go
// to both — which is what draws a bordered box as one shape running down several
// columns rather than as several boxed-off pieces.
//
// The other value, "clone", gives every fragment the whole border and its own
// background. It is a different picture and this does not produce it; a box
// declaring it is refused by canColumn.
func (n *pending) slice(y style.Unit) (top *Fragment, bottom *pending, ok bool) {
	c := n.root
	// The cut in the box's own content coordinates, which is where its lines and
	// children are measured from. It may fall outside them at either end: a box
	// whose own border box is entirely above the cut can still hold content
	// that reaches past it, which is what a float overflowing its parent is.
	inner := y.Sub(c.ContentRect().Y)
	t, below, fine := n.split(inner)
	if !fine {
		return nil, nil, false
	}
	b := n
	if !below {
		b = nil
	}
	// How much of the box's own border box falls on each side. A box whose
	// content overflows it has one of these at its full height and the other at
	// nothing, which is the fragment that carries the overflow onward.
	topH := style.Max(0, style.Min(c.BorderRect.Bottom(), y).Sub(c.BorderRect.Y))
	bottomH := style.Max(0, c.BorderRect.Bottom().Sub(style.Max(c.BorderRect.Y, y)))
	// A box with nothing in it is divided by its own extent and not by its
	// content, which is what the split above reports on: a fragment holding no
	// lines and no children is nothing on both sides as far as that walk can
	// see, and a float is exactly such a box. So each side is made here where
	// the box reaches into it and the walk found nothing to put there.
	if t == nil && topH > 0 {
		empty := c
		empty.Lines, empty.Children = nil, nil
		t = &empty
	}
	if b == nil && bottomH > 0 {
		b = n.p.pend(c, &Fragment{})
	}
	if topH > 0 && bottomH > 0 && refusesToSlice(&c) {
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
		r := &b.root
		r.BorderRect.Y = style.Max(0, c.BorderRect.Y.Sub(y))
		r.BorderRect.H = bottomH
		if topH > 0 {
			r.Border.Top, r.Padding.Top, r.Margin.Top = 0, 0, 0
		}
		r.contentH = r.BorderRect.H
	}
	return t, b, true
}

// columnBreaks collects the heights at which a column may end, in increasing
// order and measured down the fragment's content box.
//
// They are the bottoms of the line boxes and of the boxes that hold none: a line
// is the unit fragmentation works in, so a cut anywhere else would go through
// one. Everything the balancing below chooses comes from this list, which is
// what makes splitAt's refusal of a straddled line a guard rather than a case.
func columnBreaks(f *Fragment, at style.Unit, out []style.Unit) []style.Unit {
	if f == nil {
		return out
	}
	for _, line := range f.Lines {
		out = append(out, at.Add(line.Rect.Bottom()))
	}
	for _, c := range f.Children {
		if len(c.Lines) == 0 && len(c.Children) == 0 {
			out = append(out, at.Add(c.BorderRect.Bottom()))
			continue
		}
		// The child's own content origin and not its border box: its lines are
		// measured from the first, and a border of five pixels would otherwise
		// put every breakpoint inside it five pixels out — which is a cut
		// through a line, and is refused rather than drawn wrong.
		out = columnBreaks(c, at.Add(c.ContentRect().Y), out)
		out = append(out, at.Add(c.BorderRect.Bottom()))
	}
	return out
}

// maxPourPieces bounds how many fragments one pour may make.
//
// A pour makes one copy of every line and box it moves and one more of every
// box a cut goes through, and the second is not bounded by the content: a box
// a million pixels tall in columns a pixel high is cut a million times, and one
// nested a hundred deep is cut a hundred times at each. The column count is
// bounded, and so is the depth, and their product is not a number to make
// fragments up to. A pour that would pass this is refused like any other the
// engine cannot make, and the content is laid out in one column and reported.
//
// A variable so that a test can lower it and watch it fire.
var maxPourPieces = 1 << 18

// fillColumns pours a subtree laid out in one tall column into n columns of a
// given height, side by side, and says why not where it cannot.
//
// It is the whole of the layout half: the content was laid out once, at the
// column width, by the ordinary block code that knows nothing about columns, and
// this cuts the result into bands and stands them beside each other.
func fillColumns(f *Fragment, c columns, height style.Unit) (bool, string) {
	return fillColumnsWith(f, c, height, nil)
}

// columnEnds is what decides where a column ends besides its height: the
// breaks the content asked to avoid, the ones it forces, and — for a balanced
// pour — the breakpoints a column may end at.
type columnEnds struct {
	avoid *avoidZones
	// forced is where a column must end, sorted: CSS Fragmentation 3's forced
	// breaks between boxes. See forcedColumnBreaks.
	forced []style.Unit
	// breaks, where it is set, is the sorted breakpoints a column may end at,
	// and a column ends at the last of them that fits rather than at exactly a
	// column height below where it began. That is the pour balancing measured
	// (see fitsColumns), and a pour made at the height it chose has to end its
	// columns where it counted them ending, or a column that began at a forced
	// break could end a column height further on in the middle of a line.
	breaks []style.Unit
}

// fillColumnsWith is fillColumns with the column ends the content asked for.
//
// Each column ends a column height below where it began, unless:
//
//   - a forced break comes first. The column ends there, however much room is
//     left in it, and nothing moves it: CSS Fragmentation 3 §3.1 says a forced
//     break value "overrides any avoid break value that also applies at that
//     break point", and §4.4 relaxes only the rules for unforced breaks when
//     there are not enough places to break;
//   - it is balanced, and it ends at the last breakpoint that fits;
//   - that is somewhere a break is avoided, and then it ends at the last
//     break before it that is not. See avoidZones.
//
// Where there is no such break — the box that asked not to be broken is taller
// than a column — the column ends where it would have, inside the box. That is
// CSS Fragmentation 3 §4.4's instruction and not a shortcut: "avoid" is the
// first rule relaxed when there are not enough break opportunities that
// satisfy it, because the alternative is content that is never shown.
func fillColumnsWith(f *Fragment, c columns, height style.Unit, ends *columnEnds) (bool, string) {
	if height <= 0 {
		return false, cannotDivide
	}
	if ends == nil {
		ends = &columnEnds{}
	}
	p := &pour{ends: map[*Fragment]style.Unit{}}
	rest := p.pend(*f, f)
	bands := make([]*Fragment, 0, min(c.n, len(f.Lines)+len(f.Children)+1))
	spent := false
	forced := ends.forced
	// start is where the column being filled begins, in the content
	// coordinates of f, which is what the breakpoints are measured in.
	var start style.Unit
	for i := 0; i < c.n && !spent; i++ {
		cut := start.Add(height)
		for len(forced) > 0 && forced[0] <= start {
			forced = forced[1:]
		}
		switch {
		case len(forced) > 0 && forced[0] <= cut:
			cut = forced[0]
		case ends.breaks != nil:
			if b, ok := lastBreakIn(ends.breaks, start, cut); ok {
				cut = b
			}
		case ends.avoid.forbids(cut):
			if b, ok := ends.avoid.lastAllowed(start, cut); ok {
				cut = b
			}
		}
		top, below, ok := rest.split(cut.Sub(start))
		start = cut
		if !ok {
			return false, cannotDivide
		}
		if p.made > maxPourPieces {
			return false, "its content would be cut into more than " +
				strconv.Itoa(maxPourPieces) + " pieces, which is more than this " +
				"engine will make for one box"
		}
		bands = append(bands, top)
		spent = !below
	}
	if !spent {
		// More content than the columns hold. §3.6 overflows it out of the last
		// column, which is a fragmentation of its own and is not done here.
		return false, cannotDivide
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
			f.Children = append(f.Children, child)
		}
	}
	return true, ""
}

// lastBreakIn is the latest of the sorted breakpoints after start and at or
// before end, if there is one.
func lastBreakIn(breaks []style.Unit, start, end style.Unit) (style.Unit, bool) {
	i := sort.Search(len(breaks), func(i int) bool { return breaks[i] > end }) - 1
	if i < 0 || breaks[i] <= start {
		return 0, false
	}
	return breaks[i], true
}

// cannotDivide is why a pour that needs a cut through something drawn, or more
// columns than there are, is refused.
const cannotDivide = "its content cannot be divided where a column would end " +
	"without cutting through something that is drawn"

// balancedHeight is §3.5's "balance": the shortest the columns can be while
// still holding the content between them, where each column ends at the last
// breakpoint that fits in it and at every forced break.
//
// The answer is a height and not a breakpoint. It was the first breakpoint the
// content fitted under, on the argument that a column ends at one, so a height
// between two of them holds exactly as much as the lower. That is true of the
// first column, which begins at nought, and of no other: the second begins
// where the first ended, and the height it needs is the distance between two
// breakpoints, which need not be one. Pieces of 30, 10 and 60 in two columns
// fit in 60 — 40 in the first, 60 in the second — and the first breakpoint
// they fit under was 100, one column holding everything. A forced break makes
// it the ordinary case: every column after one begins wherever the break was.
//
// So the search is over heights, and found by halving. Whether the content
// fits is monotone in the height: filled greedily, a taller column ends at or
// after the breakpoint a shorter one did, so every later column begins no
// earlier and fewer of them are needed; a forced break ends a column wherever
// it is, which a taller column does not change; and a piece too tall for a
// column is too tall for every shorter one. So the answers over the heights are
// all "no" and then all "yes", and the boundary is found in as many fits as the
// height has bits — thirty-two walks of the breakpoints at most, whatever
// their number, where a fit per breakpoint was quadratic in the lines.
func balancedHeight(breaks, forced []style.Unit, n int) (style.Unit, bool) {
	if len(breaks) == 0 {
		return 0, true
	}
	hi := breaks[len(breaks)-1]
	if hi <= 0 {
		return hi, true
	}
	if !fitsColumns(breaks, forced, n, hi) {
		// Not even one column the whole height of the content: the forced
		// breaks ask for more columns than there are.
		return 0, false
	}
	lo := style.Unit(0) // does not fit, or is nothing
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if fitsColumns(breaks, forced, n, mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi, true
}

// fitsColumns reports whether content whose breakpoints are these fits in n
// columns of the given height, filled greedily, with a column ending at each
// forced break. forced is sorted, and every forced break is one of the
// breakpoints.
//
// The breakpoints are sorted and distinct, so the one before a breakpoint is
// the one before it in the list, and the walk carries it rather than searching
// for it. It stops as soon as the columns are more than n: the rest cannot make
// the answer yes.
//
// A forced break at the last breakpoint ends a column after which there is
// nothing, and makes none: a pour cut there has nothing left to put in the
// next one.
func fitsColumns(breaks, forced []style.Unit, n int, height style.Unit) bool {
	if len(breaks) == 0 {
		return true
	}
	used, start, prev := 1, style.Unit(0), style.Unit(0)
	last := breaks[len(breaks)-1]
	for _, at := range breaks {
		if at.Sub(start) > height {
			// This piece does not fit in the column being filled, so the column
			// ended at the breakpoint before it, and the piece begins the next.
			used++
			if used > n {
				return false
			}
			start = prev
			if at.Sub(start) > height {
				// One piece taller than a whole column. No number of columns
				// holds it, and a taller column is the only answer.
				return false
			}
		}
		prev = at
		for len(forced) > 0 && forced[0] < at {
			forced = forced[1:]
		}
		if len(forced) > 0 && forced[0] == at && at < last {
			// The column ends here whatever room is left in it.
			used++
			if used > n {
				return false
			}
			start = at
		}
	}
	return true
}

// refusesToSlice reports whether a box is one this engine will not cut through.
//
// What it refuses is what slicing cannot draw rather than everything that draws
// at all. A border and a background colour are sliceable — §4.4 says how, and
// sliceBox does it — and these are not:
//
//   - A background image, whose position is stated relative to the box that was
//     never divided. Slicing it means painting each fragment with the image
//     placed for the whole, which the painter has no way to express.
//   - An outline, which §4.4 leaves to the UA and which this engine draws as one
//     ring round one border box.
//   - A banded background, which is a table's row or column showing through and
//     is not a rectangle to begin with.
//   - "box-decoration-break: clone", which asks for the other picture.
func refusesToSlice(f *Fragment) bool {
	if f == nil || f.Box == nil {
		return true
	}
	if len(f.bgBands) > 0 || f.Outline > 0 {
		return true
	}
	if v := ascii.TrimCSSSpace(f.Box.Style.Get("background-image")); v != "" &&
		!ascii.EqualFold(v, "none") {
		return true
	}
	return ascii.EqualFold(
		ascii.TrimCSSSpace(f.Box.Style.Get("box-decoration-break")), "clone")
}

// canColumn is whether a box's content is of a kind this engine can pour into
// columns, and why not where it is not.
//
// It is asked of the box tree before anything is laid out, for the reason the
// writing-mode gate is: the content of a multicol container is laid out at the
// *column* width, so a box that turns out to be unfragmentable afterwards has
// been measured against the wrong line length. What is refused here is laid out
// in one column at the box's own width, exactly as it was before this file
// existed, and is reported.
func (l *layouter) canColumn(b *Box) string {
	if b.Outer != OuterBlock || (b.Inner != InnerFlow && b.Inner != InnerFlowRoot) {
		return "it is not an ordinary block box"
	}
	if writingModeOf(b).vertical() {
		// Two fragmentations at once. The columns of a vertical multicol
		// container run down the page and its lines run across them, and the
		// turn is written for a box whose content is one column.
		return "its writing mode is vertical, and this engine turns a box or " +
			"columns it, not both"
	}
	return l.subtreeCanColumn(b, b)
}

func (l *layouter) subtreeCanColumn(root, b *Box) string {
	if b != root {
		if b.Position != PositionStatic {
			return "it holds a positioned box"
		}
		if b.Replaced != nil || b.Control != nil || b.ListItem || b.MarkerImage != nil {
			return "it holds a replaced element, a form control or a list marker"
		}
		switch b.Inner {
		case InnerFlow, InnerText, InnerFlowRoot:
			// A flow root is ordinary block layout that seals its own floats,
			// which is what a float is and what "overflow: hidden" makes. It
			// divides like any other block.
		default:
			return "it holds a table or another box with sizing rules of its own"
		}
		if spansColumns(b) {
			// §6.3's "column-span: all" divides the multicol container into two
			// of them with the spanning element between, which is a second
			// container and not a column.
			return "\"column-span: all\" is declared inside it, which divides " +
				"the container rather than filling a column"
		}
	}
	for _, c := range b.Children {
		if why := l.subtreeCanColumn(root, c); why != "" {
			return why
		}
	}
	return ""
}

// spansColumns reads §6.3's column-span.
func spansColumns(b *Box) bool {
	return ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("column-span")), "all")
}

// reportColumns says a box asked for columns and did not get them.
func (l *layouter) reportColumns(b *Box, n int, why string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "this box asked for " + plural(n, "column") + " and was laid out in " +
			"one, because " + why,
		Path:     PathOf(b.Element),
		Property: "column-count",
	})
}

func plural(n int, what string) string {
	if n == 1 {
		return "one " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}

// pourIntoColumns divides a multicol container's content and returns the height
// the container comes to, or says the content could not be divided.
//
// The height is §3.5's, and which of the two it is depends on the box: a
// container told how tall to be and asked to fill its columns in turn takes that
// height and lets the last column end short, and one that is not takes the
// shortest height its content fits in — which is what "balance", the initial
// value, asks for.
func (l *layouter) pourIntoColumns(b *Box, frag *Fragment, cols columns,
	contentHeight, declared style.Unit, hasHeight bool) (style.Unit, bool) {

	// The forced breaks, which are places a column may end as well as places
	// it must: a break between two boxes separated by a margin is at neither
	// box's edge. See forcedColumnBreaks.
	forced := l.forcedColumnBreaks(frag)
	breaks := sortedBreaks(append(columnBreaks(frag, 0, nil), forced...))
	if len(breaks) == 0 {
		// Nothing to divide. The columns are as tall as nothing, which is what
		// an empty container is either way.
		return contentHeight, true
	}
	if len(forced) >= cols.n {
		// Each forced break begins a column, and there are not that many. The
		// ones it would take are §3.6's overflow columns, which are not made
		// here, and nor is the pour.
		l.reportColumns(b, cols.n, "its content forces "+plural(len(forced), "column break")+
			", which take "+plural(len(forced)+1, "column")+", and the columns that "+
			"would overflow the box are not made")
		return 0, false
	}
	// The breaks the content asked to avoid: CSS Fragmentation 3's
	// "break-inside", "break-before" and "break-after". They are taken off the
	// list the balancing chooses from, so the columns come out as tall as it
	// takes not to break where the author said not to, and they steer where
	// fillColumnsWith ends each column. A forced break is never taken off: it
	// overrides an avoid at the same place.
	avoid := avoidZonesOf(frag)
	breaks = avoid.allowed(breaks, forced)
	ends := &columnEnds{avoid: avoid, forced: forced}
	// §3.5, and the two cases are which of the heights is *given*.
	//
	// A container told how tall to be has its column height decided for it: the
	// columns are that tall and the content is cut at multiples of it, whichever
	// value column-fill has, and anew from each forced break. "balance" is
	// consulted, as §3.5 puts it, "only if the length of columns has been
	// constrained" — and where it has, the constraint is the length, so there is
	// nothing left for balancing to choose.
	//
	// A container with an automatic height has no such number, and the answer is
	// the shortest its content fits in. That is the case balancing exists for and
	// is what the initial value asks for, and each column then ends where the
	// balancing counted it ending.
	height := declared
	if !hasHeight {
		got, ok := balancedHeight(breaks, forced, cols.n)
		if !ok {
			l.reportColumns(b, cols.n, "its content does not divide into that "+
				"many columns of any height this engine can choose")
			return 0, false
		}
		height = got
		ends.breaks = breaks
	}
	if ok, why := fillColumnsWith(frag, cols, height, ends); !ok {
		l.reportColumns(b, cols.n, why)
		return 0, false
	}
	return height, true
}

// avoidZones is where a multicol container's content asked not to be broken, as
// sorted, disjoint, closed ranges of heights down its content box.
//
// CSS Fragmentation 3 §3 gives three properties and they come to one shape
// here. "break-inside: avoid" on a box forbids a cut strictly inside it — its
// top and its bottom edges are breaks *around* it, which are allowed. "break-
// before: avoid" forbids every cut between the box and whatever is before it,
// and "break-after: avoid" every cut between the box and whatever follows it.
// §3.1 propagates the second two through a parent's edge: the break before a
// first child is the break before its parent too, and the break after a last
// child the break after its parent. So "between" runs from the bottom of the
// previous in-flow box to the top of this one, however many parents' edges lie
// between them, and the same forwards.
//
// "avoid-column" is the same request made of columns alone, and is read as
// "avoid" here because columns are the only fragmentation there is. "avoid-page"
// and "avoid-region" ask nothing of a column and are satisfied everywhere else
// by an engine that does not break pages or regions at all.
//
// Heights are in layout units, so a range open at both ends — the inside of a
// box — is the closed range one unit in from each end.
type avoidZones struct {
	zones []avoidZone
	// breaks is the breakpoints no zone forbids, once allowed has been asked:
	// the places lastAllowed may end a column early.
	breaks []style.Unit
}

type avoidZone struct{ lo, hi style.Unit }

// avoidZonesOf collects the zones of a laid-out multicol container's content,
// in the coordinates columnBreaks uses. It returns nil where nothing asks.
func avoidZonesOf(f *Fragment) *avoidZones {
	z := &avoidZones{}
	const far = style.MaxUnit / 4
	z.collect(f, 0, -far, far)
	if len(z.zones) == 0 {
		return nil
	}
	slices.SortFunc(z.zones, func(a, b avoidZone) int { return cmp.Compare(a.lo, b.lo) })
	merged := z.zones[:1]
	for _, r := range z.zones[1:] {
		last := &merged[len(merged)-1]
		if r.lo <= last.hi.Add(1) {
			last.hi = style.Max(last.hi, r.hi)
			continue
		}
		merged = append(merged, r)
	}
	z.zones = merged
	return z
}

// collect walks f's children. at is where f's content box begins; before is
// where the in-flow content before f's first child ends, and after where the
// content after its last child begins — the reach a "break-before" on the
// first child or a "break-after" on the last has, once propagated.
func (z *avoidZones) collect(f *Fragment, at, before, after style.Unit) {
	var flow []*Fragment
	for _, c := range f.Children {
		top, bottom := at.Add(c.BorderRect.Y), at.Add(c.BorderRect.Bottom())
		if avoidsBreak(c.Box, "break-inside") && bottom.Sub(top) >= 2 {
			z.zones = append(z.zones, avoidZone{top.Add(1), bottom.Sub(1)})
		}
		if c.Box == nil || !c.Box.outOfFlow() {
			flow = append(flow, c)
		} else {
			// A float's own content still asks about breaks inside it.
			z.collect(c, at.Add(c.ContentRect().Y), top, bottom)
		}
	}
	for i, c := range flow {
		top, bottom := at.Add(c.BorderRect.Y), at.Add(c.BorderRect.Bottom())
		prev, next := before, after
		if i > 0 {
			prev = at.Add(flow[i-1].BorderRect.Bottom())
		}
		if i+1 < len(flow) {
			next = at.Add(flow[i+1].BorderRect.Y)
		}
		if avoidsBreak(c.Box, "break-before") {
			z.zones = append(z.zones, avoidZone{style.Min(prev, top), top})
		}
		if avoidsBreak(c.Box, "break-after") {
			z.zones = append(z.zones, avoidZone{bottom, style.Max(next, bottom)})
		}
		z.collect(c, at.Add(c.ContentRect().Y), prev, next)
	}
}

// avoidsBreak reads the two values of a break property that ask a column not to
// end at a place. A fragment with no box of its own asks nothing.
func avoidsBreak(b *Box, property string) bool {
	if b == nil {
		return false
	}
	switch ascii.Lower(ascii.TrimCSSSpace(b.Style.Get(property))) {
	case "avoid", "avoid-column":
		return true
	}
	return false
}

// breakKind is what a "break-before" or "break-after" value asks of a column.
type breakKind uint8

const (
	// noForcedBreak is "auto", an avoid value, or a value that is not a
	// column's to make: a page's (§3.1: "if the flow is not paginated, they
	// have no effect", and this engine does not paginate — style says so where
	// they are declared) or a region's.
	noForcedBreak breakKind = iota
	// breakColumn is "column", and css-break-4's "always", whose break is "that
	// of the immediately-containing fragmentation context": in a multicol
	// container, a column.
	breakColumn
	// breakAll is css-break-4's "all", which breaks "through all containing
	// fragmentation contexts": a column here, and a page — which is not made.
	breakAll
)

// forcedBreakOf reads a break property as the break it forces in a column. A
// fragment with no box of its own forces nothing.
func forcedBreakOf(b *Box, property string) breakKind {
	if b == nil {
		return noForcedBreak
	}
	switch ascii.Lower(ascii.TrimCSSSpace(b.Style.Get(property))) {
	case "column", "always":
		return breakColumn
	case "all":
		return breakAll
	}
	return noForcedBreak
}

// forcedColumnBreaks is where a multicol container's content forces a column to
// end, as sorted, distinct heights down its content box — the coordinates
// columnBreaks uses.
//
// CSS Fragmentation 3 §4.1: a forced break is at a class A break point, the
// place between two adjacent in-flow siblings, when the "break-after" of the
// first or the "break-before" of the second forces one. §3.1 propagates the
// values at a box's edges: a "break-before" on a first in-flow child is its
// parent's, and a "break-after" on a last child too — so the break asked for
// before the first paragraph in a section is the break before the section.
// Propagation "stops before it breaks through the nearest matching
// fragmentation context": what reaches the container's own first or last child
// is at the start or the end of the columns, where there is nothing to break
// from or to, and does nothing. A nested multicol container is the nearest
// such context for everything inside it, so its content is its own pour's and
// is not read here; the container's own values are this one's.
//
// The height a column ends at is where the next box's own top margin begins,
// held no higher than the bottom of the box before. §5.2 says a forced break
// truncates the margins before it and keeps the ones after: the next column
// begins with the next box's margin, and not with the one the box before
// carried down.
//
// Out-of-flow boxes are not in the flow that breaks. A float's own values, and
// anything inside it, are reported by reportForcedBreaks and not made.
func (l *layouter) forcedColumnBreaks(f *Fragment) []style.Unit {
	var out []style.Unit
	l.collectForced(f, 0, &out)
	if len(out) < 2 {
		return out
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// collectForced walks f's in-flow children, whose content box begins at at, and
// reports whether its first child forces a break before it and its last one
// after it — the two values §3.1 propagates to f.
func (l *layouter) collectForced(f *Fragment, at style.Unit, out *[]style.Unit) (before, after bool) {
	var prev *Fragment
	var prevAfter bool
	for _, c := range f.Children {
		if c.Box != nil && c.Box.outOfFlow() {
			continue
		}
		cb := forcedBreakOf(c.Box, "break-before") != noForcedBreak
		ca := forcedBreakOf(c.Box, "break-after") != noForcedBreak
		if c.Box == nil || !l.isMulticol(c.Box) {
			first, last := l.collectForced(c, at.Add(c.ContentRect().Y), out)
			cb, ca = cb || first, ca || last
		}
		if prev == nil {
			before = cb
		} else if prevAfter || cb {
			bottom := at.Add(prev.BorderRect.Bottom())
			*out = append(*out, style.Max(bottom, at.Add(c.BorderRect.Y).Sub(c.Margin.Top)))
		}
		prev, prevAfter = c, ca
	}
	return before, prevAfter
}

// isMulticol reports whether a box is a multicol container: one that asks for
// columns, whether or not it gets them.
func (l *layouter) isMulticol(b *Box) bool {
	_, ok := l.columnsFor(b, 0)
	return ok
}

// reportForcedBreaks says where a forced column break was asked for and is not
// made, over the whole box tree. What is made is decided by the fragments, in
// forcedColumnBreaks; what is reported is decided here by where the box is,
// because a break the pour never sees — inside a float, inside an inline-block
// — is exactly the one the fragments cannot say anything about.
//
//   - A box in the normal flow of a multicol container whose content is
//     poured: its "column" and "always" are column breaks and are made, or
//     are at an edge of the columns where CSS says they do nothing. Its "all"
//     is a column break and a page break, and the page break is reported.
//   - A box anywhere else inside a multicol container — a float, or a block
//     inside an inline-level box — is not in the flow the columns divide.
//     §3.1 says a UA "should" break before and after a float too; this does
//     not move a float to the next column, and says so.
//   - A box in no multicol container: "column" has no effect by §3.1 ("if the
//     flow is not within a multi-column context"), and is not reported;
//     "always" and "all" are page breaks, and are.
//
// A multicol container whose content is refused is reported whole by
// reportColumns, and what is inside it is not reported again.
func (l *layouter) reportForcedBreaks(root *Box) {
	type visit struct {
		b   *Box
		ctx columnContext
	}
	stack := []visit{{root, outsideColumns}}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// A run of text and an anonymous box declare nothing of their own.
		for _, property := range []string{"break-before", "break-after"} {
			if v.b.IsText() || v.b.Anonymous() {
				break
			}
			if kind := forcedBreakOf(v.b, property); kind != noForcedBreak {
				l.reportForcedBreak(v.b, property, kind, v.ctx)
			}
		}
		inner := v.ctx
		switch {
		case v.ctx == refusedColumns:
		case l.isMulticol(v.b):
			inner = inColumnFlow
			if l.canColumn(v.b) != "" {
				inner = refusedColumns
			}
		case v.ctx == inColumnFlow && (v.b.Outer != OuterBlock || v.b.outOfFlow()):
			// A float, or a box on a line: what is inside it is not in the
			// flow the columns divide.
			inner = outsideColumnFlow
		}
		for i := len(v.b.Children) - 1; i >= 0; i-- {
			c := v.b.Children[i]
			ctx := inner
			if inner == inColumnFlow && c.outOfFlow() {
				ctx = outsideColumnFlow
			}
			stack = append(stack, visit{c, ctx})
		}
	}
}

// columnContext is where a box is, for reportForcedBreaks.
type columnContext uint8

const (
	outsideColumns columnContext = iota
	inColumnFlow
	outsideColumnFlow
	refusedColumns
)

// reportForcedBreak raises the finding for one declaration reportForcedBreaks
// found not made, or not wholly.
func (l *layouter) reportForcedBreak(b *Box, property string, kind breakKind, ctx columnContext) {
	value := ascii.Lower(ascii.TrimCSSSpace(b.Style.Get(property)))
	var why string
	switch {
	case ctx == refusedColumns:
		return
	case ctx == inColumnFlow && kind == breakAll:
		why = "the column break is made, and the page break it also asks for is " +
			"not: this engine does not break a document into pages"
	case ctx == inColumnFlow:
		return
	case ctx == outsideColumnFlow:
		why = "no break is made there: the box is not in the flow its multi-column " +
			"container divides — a float, or inside an inline-level box — and " +
			"this engine does not move one to the next column"
	case value == "column":
		// No effect outside a multi-column context, which is what happens.
		return
	default:
		why = "no break is made there: outside a multi-column container it is a " +
			"page break, and this engine does not break a document into pages"
	}
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Source:   AtHTML(offsetOf(b)),
		Message:  "\"" + property + ": " + value + "\" is not applied; " + why,
		Path:     PathOf(b.Element),
		Property: property,
	})
}

// forbids reports whether a column may not end at y.
func (z *avoidZones) forbids(y style.Unit) bool {
	if z == nil {
		return false
	}
	i := sort.Search(len(z.zones), func(i int) bool { return z.zones[i].hi >= y })
	return i < len(z.zones) && z.zones[i].lo <= y
}

// allowed is the sorted breakpoints less the ones a zone forbids, except two
// kinds. The last is where the content ends, and so where the last column must
// end whatever was asked; without it the balancing would not know how far the
// content reaches. And a forced break (forced, sorted) overrides an avoid at the
// same place, so no zone forbids one.
func (z *avoidZones) allowed(breaks, forced []style.Unit) []style.Unit {
	if z == nil || len(breaks) == 0 {
		return breaks
	}
	out := make([]style.Unit, 0, len(breaks))
	k := 0
	for i, b := range breaks {
		for k < len(z.zones) && z.zones[k].hi < b {
			k++
		}
		for len(forced) > 0 && forced[0] < b {
			forced = forced[1:]
		}
		if i == len(breaks)-1 || k == len(z.zones) || z.zones[k].lo > b ||
			(len(forced) > 0 && forced[0] == b) {
			out = append(out, b)
		}
	}
	z.breaks = out
	return out
}

// lastAllowed is the latest height after start and at or before end that no
// zone forbids and that is a breakpoint — the latest place a column may end
// early, rather than inside something that asked not to be broken.
//
// It is a breakpoint and not simply the edge of the zone, because a column
// ends between lines and between boxes and nowhere else: the zone's edge may
// be the middle of the line before it.
//
// The allowed breakpoints are sorted, so it is one search: every one of them is
// allowed but possibly the last, which is where the content ends and is a
// place the last column ends whatever was asked.
func (z *avoidZones) lastAllowed(start, end style.Unit) (style.Unit, bool) {
	i := sort.Search(len(z.breaks), func(i int) bool { return z.breaks[i] > end }) - 1
	if i < 0 || z.breaks[i] <= start {
		return 0, false
	}
	return z.breaks[i], true
}

// sortedBreaks puts the breakpoints in order and drops the repeats, which the
// walk produces where a box ends at its last line.
func sortedBreaks(in []style.Unit) []style.Unit {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(i, j int) bool { return in[i] < in[j] })
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
