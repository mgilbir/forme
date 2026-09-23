package layout

import (
	"cmp"
	"slices"
	"sort"
	"strconv"
	"strings"

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
	raw := strings.TrimSpace(b.Style.Get("column-count"))
	if raw == "" || strings.EqualFold(raw, "auto") {
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
	if strings.EqualFold(strings.TrimSpace(b.Style.Get("column-fill")), "auto") {
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
	if height <= 0 {
		return false, cannotDivide
	}
	p := &pour{ends: map[*Fragment]style.Unit{}}
	rest := p.pend(*f, f)
	bands := make([]*Fragment, 0, min(c.n, len(f.Lines)+len(f.Children)+1))
	spent := false
	for i := 0; i < c.n && !spent; i++ {
		top, below, ok := rest.split(height)
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

// cannotDivide is why a pour that needs a cut through something drawn, or more
// columns than there are, is refused.
const cannotDivide = "its content cannot be divided where a column would end " +
	"without cutting through something that is drawn"

// balancedHeight is §3.5's "balance": the shortest the columns can be while
// still holding the content between them.
//
// The candidates are the breakpoints and nothing else, because a column ends at
// one: a height between two of them holds exactly as much as the lower of the
// two and is taller for nothing. So the search is over the list rather than over
// the numbers, and the answer is the first candidate the content fits inside.
//
// First, and found by halving rather than by trying each in turn. Whether the
// content fits is monotone in the height: filled greedily, a taller column ends
// at or after the breakpoint a shorter one did, so every later column begins no
// earlier and fewer of them are needed, and a piece too tall for a column is too
// tall for every shorter one. So the answers along the list are all "no" and
// then all "yes", and the boundary is found in log(breaks) fits rather than in
// one per breakpoint — which, at a fit per breakpoint over every breakpoint,
// was quadratic in the lines before it was anything else.
func balancedHeight(breaks []style.Unit, n int) (style.Unit, bool) {
	i := sort.Search(len(breaks), func(i int) bool { return fitsColumns(breaks, n, breaks[i]) })
	if i == len(breaks) {
		return 0, false
	}
	return breaks[i], true
}

// fitsColumns reports whether content whose breakpoints are these fits in n
// columns of the given height, filled greedily.
//
// The breakpoints are sorted and distinct, so the one before a breakpoint is
// the one before it in the list, and the walk carries it rather than searching
// for it. It stops as soon as the columns are more than n: the rest cannot make
// the answer yes.
func fitsColumns(breaks []style.Unit, n int, height style.Unit) bool {
	if len(breaks) == 0 {
		return true
	}
	used, start, prev := 1, style.Unit(0), style.Unit(0)
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
	if v := strings.TrimSpace(f.Box.Style.Get("background-image")); v != "" &&
		!strings.EqualFold(v, "none") {
		return true
	}
	return strings.EqualFold(
		strings.TrimSpace(f.Box.Style.Get("box-decoration-break")), "clone")
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
	return strings.EqualFold(strings.TrimSpace(b.Style.Get("column-span")), "all")
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

	breaks := sortedBreaks(columnBreaks(frag, 0, nil))
	if len(breaks) == 0 {
		// Nothing to divide. The columns are as tall as nothing, which is what
		// an empty container is either way.
		return contentHeight, true
	}
	// §3.5, and the two cases are which of the heights is *given*.
	//
	// A container told how tall to be has its column height decided for it: the
	// columns are that tall and the content is cut at multiples of it, whichever
	// value column-fill has. "balance" is consulted, as §3.5 puts it, "only if
	// the length of columns has been constrained" — and where it has, the
	// constraint is the length, so there is nothing left for balancing to
	// choose.
	//
	// A container with an automatic height has no such number, and the answer is
	// the shortest its content fits in. That is the case balancing exists for and
	// is what the initial value asks for.
	height := declared
	if !hasHeight {
		got, ok := balancedHeight(breaks, cols.n)
		if !ok {
			l.reportColumns(b, cols.n, "its content does not divide into that "+
				"many columns of any height this engine can choose")
			return 0, false
		}
		height = got
	}
	if ok, why := fillColumns(frag, cols, height); !ok {
		l.reportColumns(b, cols.n, why)
		return 0, false
	}
	return height, true
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
