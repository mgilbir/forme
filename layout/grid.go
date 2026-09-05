package layout

import (
	"slices"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// CSS Grid Layout 2: a table of tracks, and the items that fall into its cells.
//
// # What a grid is, in one paragraph
//
// A flex container arranges its items along one axis and negotiates their sizes
// from a shared budget. A grid does not negotiate with the items at all: it
// works out a set of *tracks* — columns and rows — from what the stylesheet
// says and what the content needs, and then puts each item in a cell of that
// table. The items influence the tracks only through their content sizes, and
// the tracks decide everything else. That inversion is the whole of the
// difference, and it is why this file's shape is track sizing first and items
// second, where layout/flex.go's is items first.
//
// # What is laid out here, and what is refused
//
// The explicit columns of "grid-template-columns", the rows of
// "grid-template-rows" and the implicit rows the items overflow into, with
// every item placed by §8.5's automatic flow: one cell each, left to right and
// then down, in order-modified document order. Track sizes may be a length, a
// percentage, a fraction of the free space, or one of the three content
// keywords, and "repeat()" writes any of them more than once. The gaps are
// "row-gap" and "column-gap", which a grid reads on both axes rather than one.
//
// Everything else is refused with a finding and laid out as it was before this
// file existed, which is as a column of blocks. The gate is the same shape as
// flex's and multicol's, and for the same reason: a box refused here is the
// page this engine drew yesterday and is *reported*, while a box laid out
// wrongly is a page that is plausible and silent. See refusesToGrid, where each
// clause is stated as a condition on the box.
//
// # Why the suite cannot check this
//
// It has one document that lays out a grid — text-indent/anonymous-grid-item-001
// — and that one is about anonymous items rather than about tracks. So the
// reftest count is a regression check here and nothing more, and the evidence
// that this is right is the arithmetic in layout/grid_test.go, where Courier at
// 20px makes every advance a whole number and every share of the free space an
// exact one. That is the standard layout/flex.go is held to and for the same
// reason.

// trackKind is which of §7.2's sizes a track was written as.
type trackKind uint8

const (
	// trackFixed is a length or a percentage: a size the stylesheet states and
	// the content cannot change.
	trackFixed trackKind = iota
	// trackAuto, trackMin and trackMax are the three content keywords. "auto"
	// is max-content that may also be stretched to fill the container, which is
	// what makes it the useful default and not a synonym for either.
	trackAuto
	trackMin
	trackMax
	// trackFlex is a fraction of the space the other tracks left over.
	trackFlex
)

// trackSize is one of §7.2's sizing functions: what a track asks for at one of
// its two ends.
type trackSize struct {
	kind trackKind
	// size is the stated length of a trackFixed, and factor the number in front
	// of the "fr" of a trackFlex.
	size   style.Unit
	factor float64
}

// gridTrack is one column or one row.
//
// Every track has *two* sizing functions and not one — §7.2 says so, and
// minmax() is the spelling that writes them separately. A single value is
// minmax() of itself twice, except for the two that mean different things at
// each end: "auto" is the largest minimum its items need at the low end and
// max-content at the high one, and a flexible track is "auto" at the low end
// and its own share at the high one. That is why "1fr" never comes out narrower
// than the words in it.
type gridTrack struct {
	min, max trackSize
	base     style.Unit
}

// autoTrack is the implicit track: "auto" at both ends, which is what
// grid-auto-rows and grid-auto-columns are set to and what a track this engine
// makes for itself has to be.
func autoTrack() gridTrack {
	return gridTrack{min: trackSize{kind: trackAuto}, max: trackSize{kind: trackAuto}}
}

// flexible and stretches are the two questions the sizing asks about a track's
// *maximum*, which is the end that decides both: a flexible track takes a share
// of the free space, and an automatic one takes what is left after that.
func (t gridTrack) flexible() bool  { return t.max.kind == trackFlex }
func (t gridTrack) stretches() bool { return t.max.kind == trackAuto }

// gridItem is one in-flow child of a grid container, together with the cell it
// was placed in.
type gridItem struct {
	box                     *Box
	margin, border, padding Edges
	// column and row are where the item's cell begins, and place is what it
	// asked for on each axis — the row first, because §8.5 places a row before
	// it places anything in one.
	column, row int
	place       [2]gridPlacement
	order       int
	// across and down are how the item is aligned in its cell on each axis,
	// and width and height are the sizes that gave it. A stretched item is its
	// cell's size; an aligned one is its own, and the difference between the
	// two is what the alignment has to place.
	across, down  flexAlign
	width, height style.Unit
	frag          *Fragment
}

// horizontal and vertical are the room around the item's content on each axis.
func (it *gridItem) horizontal() style.Unit {
	return it.margin.Horizontal().Add(it.border.Horizontal()).Add(it.padding.Horizontal())
}

func (it *gridItem) vertical() style.Unit {
	return it.margin.Vertical().Add(it.border.Vertical()).Add(it.padding.Vertical())
}

// gridContent lays a grid container's items into its tracks and returns the
// height they came to.
//
// The order is §12's: the columns are sized against the container's own width,
// the items are laid out at the widths that gives them, the rows are sized from
// what those layouts came to, and only then is anything placed. A row cannot be
// sized before the columns are, because how tall a paragraph is depends on how
// wide it was allowed to be.
func (l *layouter) gridContent(b *Box, parent *Fragment, width style.Unit,
	origin flow) style.Unit {

	items := l.gridItems(b, width)
	if len(items) == 0 {
		// A container with nothing to place still has its out-of-flow children
		// to record. Nobody else will: the block walk is what does that, and
		// this is in its place.
		l.deferGridOutOfFlow(b, parent, width)
		return 0
	}
	columnGap, rowGap := l.gridGap(b, "column-gap", width), l.gridGap(b, "row-gap", width)
	height, definite := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite)

	columns, fit, _ := l.trackList(b, "grid-template-columns", width,
		trackRoom{size: width, definite: true, gap: columnGap})
	if fit && len(items) < len(columns) {
		// §7.2.3.2's "auto-fit": the tracks that no item landed in are
		// collapsed, which for a grid whose items are dealt in order means the
		// ones past the last item. A collapsed track has no size and no gap
		// beside it, so dropping them is what collapsing comes to — and it is
		// why "auto-fit" fills the row with three cards where "auto-fill"
		// leaves room for the fourth.
		// There is at least one item — a container with none returned above —
		// so there is at least one column left standing.
		columns = columns[:len(items)]
	}
	if len(columns) == 0 {
		// §7.1: a container with no explicit columns still has one, because
		// every item has to be somewhere. The implicit track is "auto", which
		// is the same answer a single-column grid would have been given.
		columns = []gridTrack{autoTrack()}
	}
	rows, _, _ := l.trackList(b, "grid-template-rows", width,
		trackRoom{size: height, definite: definite, gap: rowGap})

	// §8.5: the items that named a line go where they asked, and the rest are
	// dealt into what is left. However many rows that took is how many the grid
	// has.
	used := placeItems(items, len(columns))
	for len(rows) < used {
		// The implicit rows, which are "auto" because grid-auto-rows is.
		rows = append(rows, autoTrack())
	}

	across, down := l.gridAlignment(b, "justify-items"), l.gridAlignment(b, "align-items")
	l.sizeColumns(columns, items, width, columnGap, l.gridContentAlignment(b, "justify-content"))

	// How wide each item is used at, which is its cell's width where it is
	// stretched and its own fit-content width where it is aligned instead. It
	// has to be settled before the heights are measured and cannot be settled
	// before the columns are sized: it is the one thing between the two.
	for _, it := range items {
		it.across = l.itemAlignment(it, "justify-self", across)
		it.down = l.itemAlignment(it, "align-self", down)
		cell := trackSpan(columns, it.column, it.place[1].span, columnGap)
		it.width = cell
		if it.across != crossStretch {
			it.width = l.gridFitContent(it, cell)
		}
	}

	// Each item laid out at that width, which is what tells the rows how tall
	// they are. The fragments are thrown away: an item is laid out again at the
	// size its cell settles on, because a height changes where an item's
	// content sits inside it.
	mark := len(l.deferred)
	for _, it := range items {
		frag := l.layOutGridItem(it, it.width, 0, false, width, origin)
		it.height = frag.BorderRect.H.Add(it.margin.Vertical())
	}
	l.deferred = l.deferred[:mark]

	l.sizeRows(rows, items, height, definite, rowGap,
		l.gridContentAlignment(b, "align-content"))

	// §12's answer for the container itself: the tracks and the gaps between
	// them, which is what a grid comes to when nothing states its height.
	inner := gridInner(rows, rowGap)

	// §10.3 and §10.4: where the tracks sit in a container that is bigger than
	// they are. With everything at its initial value there is nothing over —
	// the automatic tracks took it — so these offsets are nought and the whole
	// of the arithmetic is skipped by being zero rather than by a branch.
	columnLead, columnBetween := l.trackSpacing(b, "justify-content", columns, width, columnGap)
	rowLead, rowBetween := l.trackSpacing(b, "align-content", rows, gridInner(rows, rowGap),
		rowGap)
	if definite {
		rowLead, rowBetween = l.trackSpacing(b, "align-content", rows, height, rowGap)
	}

	for _, it := range items {
		cellHeight := trackSpan(rows, it.row, it.place[0].span, rowGap)
		if it.down == crossStretch {
			it.height = cellHeight
		}
		it.frag = l.layOutGridItem(it, it.width,
			maxZero(it.height.Sub(it.margin.Vertical())), true, width, origin)

		x := trackStart(columns, it.column, columnGap).
			Add(columnLead).Add(columnBetween.Mul(float64(it.column)))
		y := trackStart(rows, it.row, rowGap).
			Add(rowLead).Add(rowBetween.Mul(float64(it.row)))
		it.frag.BorderRect.X = x.
			Add(alignmentOffset(it.across,
				trackSpan(columns, it.column, it.place[1].span, columnGap), it.width)).
			Add(it.margin.Left)
		it.frag.BorderRect.Y = y.
			Add(alignmentOffset(it.down, cellHeight, it.height)).
			Add(it.margin.Top)
		parent.Children = append(parent.Children, it.frag)
	}
	l.deferGridOutOfFlow(b, parent, width)
	if definite {
		return height
	}
	return inner
}

// deferGridOutOfFlow records the container's absolutely positioned children,
// which are not items and are placed once the tree is absolute.
//
// §10.1 gives such a box a static position "as if it were the sole grid item in
// a grid area whose edges coincide with the padding edges of the grid
// container", which with every alignment property at its initial value is the
// content box's start corner — where the block walk would have put it, and
// where it would not have been recorded at all if this did not do it.
func (l *layouter) deferGridOutOfFlow(b *Box, parent *Fragment, width style.Unit) {
	index := 0
	for _, c := range b.Children {
		if c.ListItem {
			index++
		}
		if c.Position.outOfFlow() {
			l.deferAbsolute(c, parent, 0, 0, width, index)
		}
	}
}

// gridPlacement is what one item said about where it goes on one axis: §8.3's
// two lines, read as a start and a span.
//
// A line number is one-based in CSS and zero-based here, and the conversion is
// done once at the edge — "grid-column: 2" is the second line, which is the
// first *track's* far edge, so the item starts in track one. Getting that wrong
// is off by one everywhere, which is why it is done in one place and named.
type gridPlacement struct {
	// start is the track the item begins in, and definite says the stylesheet
	// named it rather than leaving it to the flow.
	start    int
	definite bool
	// span is how many tracks it covers, which is at least one.
	span int
}

// placementOf reads the two properties that place an item on one axis, and the
// shorthand that writes them together.
//
// The grammar this reads is the everyday half of §8.3: a line number, a span of
// so many tracks, or "auto" for either end, with "a / b" writing the two ends
// at once. What it does not read is a named line, which is refused by the gate
// — naming a line is how grid-template-areas works, and areas are a placement
// algorithm of their own.
// The three property names are passed rather than assembled from the axis, and
// that is not a style choice: style/unimplemented_test.go looks for every
// registered property as a literal in the source, and a name built out of
// "grid-" and a variable is a property nothing appears to read.
func (l *layouter) placementOf(b *Box, shorthandName, startName, endName string) (gridPlacement, bool) {
	start, end := trimmedLower(b.Style[startName]), trimmedLower(b.Style[endName])
	if shorthand := trimmedLower(b.Style[shorthandName]); shorthand != "" &&
		shorthand != "auto" {
		one, two, ok := splitOnSlash(shorthand)
		if !ok {
			return gridPlacement{}, false
		}
		// §8.3: the shorthand's second half is "auto" where it was left out,
		// and the longhands lose to it because a shorthand resets what it does
		// not say.
		start, end = one, two
	}
	return placementFrom(start, end)
}

// placementFrom turns one pair of line values into a start and a span.
//
// The four shapes are the ones a stylesheet writes: nothing, a line, a span, or
// a line and something after it. A span with no line is a span from wherever
// the flow puts the item; a line with no end is one track wide; a line and a
// line are the tracks between them, and a line after a line that is not after
// it is one track — §8.3 swaps a backwards pair rather than throwing it out.
func placementFrom(start, end string) (gridPlacement, bool) {
	from, fromSpan, ok := lineValue(start)
	if !ok {
		return gridPlacement{}, false
	}
	to, toSpan, ok := lineValue(end)
	if !ok {
		return gridPlacement{}, false
	}
	out := gridPlacement{span: 1}
	switch {
	case fromSpan > 0 && from == 0:
		// "span n" with no line of its own: the flow decides where, and the
		// span is what it takes when it gets there.
		out.span = fromSpan
	case from != 0:
		out.start, out.definite = from-1, true
	}
	switch {
	case toSpan > 0:
		if toSpan > out.span {
			out.span = toSpan
		}
	case to != 0 && out.definite:
		if to-1 < out.start {
			out.start, to = to-1, out.start+1
		}
		if n := to - 1 - out.start; n > 0 {
			out.span = n
		}
	case to != 0:
		// An end line with no start: the item ends there and is one track wide,
		// which is the same as starting one track earlier.
		out.start, out.definite = to-2, true
		if out.start < 0 {
			out.start = 0
		}
	}
	if out.start < 0 || out.span < 1 || out.span > maxRepeatedTracks {
		return gridPlacement{}, false
	}
	return out, true
}

// lineValue reads one end of a placement: a line number, a span, or nothing.
//
// The number is returned as written — one-based, with nought meaning "not
// given" — because that is the only way to tell "auto" from a line, and CSS has
// no line zero to be confused with it.
func lineValue(raw string) (line, span int, ok bool) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "auto") {
		return 0, 0, true
	}
	if rest, found := strings.CutPrefix(strings.ToLower(value), "span"); found {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			// "span" on its own is "span 1".
			return 0, 1, true
		}
		n, ok := positiveNumber(rest)
		return 0, n, ok
	}
	n, ok := positiveNumber(value)
	return n, 0, ok
}

// positiveNumber reads a whole number above nought, which is every line number
// and every span this slice places.
//
// A negative line counts from the end of the explicit grid, which is a real
// value and not one this reads: it needs the far edge of a grid that is still
// being worked out, and the gate refuses it rather than guessing.
func positiveNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
		n = n*10 + int(value[i]-'0')
		if n > maxRepeatedTracks {
			return 0, false
		}
	}
	return n, n > 0
}

// splitOnSlash cuts a shorthand into its two halves.
func splitOnSlash(value string) (string, string, bool) {
	one, two, found := strings.Cut(value, "/")
	if !found {
		return strings.TrimSpace(one), "", true
	}
	if strings.Contains(two, "/") {
		return "", "", false
	}
	return strings.TrimSpace(one), strings.TrimSpace(two), true
}

// placeItems is §8.5's automatic placement, with the items that named a line
// put where they asked first.
//
// The order is the specification's and each step is there for a reason the step
// before it could not have known. An item that named both its lines goes where
// it said, whatever else is there. An item that named only its row goes in that
// row, at the first place its span will fit. Everything else is dealt from a
// cursor that walks along the columns and then down, and that cursor never goes
// back — which is what "sparse" packing means, and what leaves the holes that
// "dense" would go back for.
func placeItems(items []*gridItem, columns int) int {
	grid := &gridOccupancy{columns: columns}
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

// gridOccupancy is which cells are taken, which is all §8.5 needs to remember.
//
// It grows downwards and never sideways: the number of columns is settled
// before any of this runs — the template says how many there are — and a row is
// made whenever an item needs one that is not there yet.
type gridOccupancy struct {
	columns int
	rows    int
	taken   []bool
}

func (g *gridOccupancy) at(row, column int) bool {
	if column < 0 || column >= g.columns || row < 0 {
		return false
	}
	if i := row*g.columns + column; i < len(g.taken) {
		return g.taken[i]
	}
	return false
}

func (g *gridOccupancy) fill(it *gridItem) {
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
func (g *gridOccupancy) free(row, column, rowSpan, columnSpan int) bool {
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
func (g *gridOccupancy) freeInRow(row, span int) int {
	for c := 0; c+span <= g.columns; c++ {
		if g.free(row, c, 1, span) {
			return c
		}
	}
	return 0
}

// freeInColumn is the first row at or after one where a span will fit in a
// given column.
func (g *gridOccupancy) freeInColumn(from, column, rowSpan, columnSpan int) int {
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
func (g *gridOccupancy) next(row, column, rowSpan, columnSpan int) (int, int) {
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

// gridInner is what the tracks and the gaps between them come to.
func gridInner(tracks []gridTrack, gap style.Unit) style.Unit {
	out := gap.Mul(float64(len(tracks) - 1))
	for _, t := range tracks {
		out = out.Add(t.base)
	}
	return out
}

// trackSpan is how far a band of tracks reaches: the tracks themselves and the
// gaps between them, which belong to the item that spans them rather than
// separating it from anything.
func trackSpan(tracks []gridTrack, from, span int, gap style.Unit) style.Unit {
	out := gap.Mul(float64(span - 1))
	for i := from; i < from+span && i < len(tracks); i++ {
		out = out.Add(tracks[i].base)
	}
	return out
}

// trackStart is how far along the axis a track begins: everything before it and
// the gaps between.
func trackStart(tracks []gridTrack, at int, gap style.Unit) style.Unit {
	out := gap.Mul(float64(at))
	for i := 0; i < at && i < len(tracks); i++ {
		out = out.Add(tracks[i].base)
	}
	return out
}

// gridAlignment reads one of the four properties that align an item in its
// cell, and itemAlignment asks the item's own before the container's.
//
// The keywords are Box Alignment's and the reader is layout/flex.go's: §6.2
// names the two ends of an axis once for every layout mode that has axes, and a
// grid whose columns run left to right and whose rows run down is the case that
// reader answers with no writing mode to unpick. That is why the value comes
// back as a flexAlign — it is the same value, and having two of them would be
// two ways to spell one specification.
func (l *layouter) gridAlignment(b *Box, property string) flexAlign {
	return crossAlignment(gridAlignmentValue(b.Style[property]), flexAxis{})
}

func (l *layouter) itemAlignment(it *gridItem, property string, container flexAlign) flexAlign {
	value := gridAlignmentValue(it.box.Style[property])
	if value == "" || value == "auto" {
		return container
	}
	return crossAlignment(value, flexAxis{})
}

// gridAlignmentValue is the value with the two spellings that mean nothing here
// taken out: "legacy" is justify-items' initial value and is a rule about
// inheriting a text-align this engine never sets, and "auto" on a *-self
// property means "the container's", which the caller answers.
func gridAlignmentValue(raw string) string {
	value := trimmedLower(raw)
	switch value {
	case "legacy", "normal":
		return ""
	case "left":
		// A grid's columns run left to right here — the gate refuses the other
		// direction — so the physical pair and the logical one name the same
		// two ends.
		return "start"
	case "right":
		return "end"
	}
	return value
}

// gridContentAlignment reports whether the tracks on an axis are stretched,
// which is the one thing the *sizing* needs to know about alignment: §12.8
// gives the space left over to the automatic tracks, and every other value
// leaves it for §10.3 to place the tracks in.
func (l *layouter) gridContentAlignment(b *Box, property string) bool {
	switch trimmedLower(b.Style[property]) {
	case "", "normal", "stretch":
		return true
	}
	return false
}

// trackSpacing is §10.3 and §10.4: where the tracks sit when the container is
// bigger than they are.
//
// It is justify-content and align-content, and it is the same arithmetic
// layout/flex.go packs a line with — one specification, one function. What
// comes back is where the first track begins and how much is added between each
// pair, which is all a grid needs: the tracks are evenly spaced by every value
// that spaces them at all.
func (l *layouter) trackSpacing(b *Box, property string, tracks []gridTrack,
	room, gap style.Unit) (lead, between style.Unit) {

	free := room.Sub(gridInner(tracks, gap))
	if free == 0 || len(tracks) == 0 {
		return 0, 0
	}
	align := justifyStart
	switch property {
	case "align-content":
		align = l.alignContentOf(b, flexAxis{})
	default:
		align = l.justifyOf(b, flexAxis{})
	}
	lead = justifyOffset(align, free, len(tracks), 0)
	if len(tracks) > 1 {
		between = justifyOffset(align, free, len(tracks), 1).Sub(lead)
	}
	return lead, between
}

// alignmentOffset is how far into its cell an aligned item sits: nothing at the
// start, all of what is left at the end, and half of it in the middle.
//
// A stretched item has no offset because it has no room to move in — it was
// made the size of the cell — and an item bigger than its cell has a negative
// one, which is what an alignment that is not "safe" comes to: it keeps the
// relationship it names and lets the overflow fall where the arithmetic puts it.
func alignmentOffset(align flexAlign, cell, used style.Unit) style.Unit {
	switch align {
	case crossEnd:
		return cell.Sub(used)
	case crossCenter:
		return cell.Sub(used).Div(2)
	}
	return 0
}

// gridFitContent is what an item that is not stretched is wide: its
// max-content size held down to the cell and up to its min-content size, which
// is §10.5's "fit-content" and is the same clause a floated box is sized by.
func (l *layouter) gridFitContent(it *gridItem, cell style.Unit) style.Unit {
	min, max := l.gridItemWidths(it)
	return style.Clamp(cell, min, max)
}

// sizeColumns is §12.4 to §12.8 on the inline axis.
//
// A track has two numbers and the difference between them is the whole of the
// algorithm: a base size it may not go below, and a growth limit it may not
// pass. For "auto" they are the items' min-content and max-content
// contributions — the width below which the content would spill out, and the
// width at which it would stop wrapping — and the space between the two is what
// the container has to give away.
//
// That is why "grid-template-columns: auto auto" in a container narrower than
// its content does not overflow: the columns start at min-content, there is
// nothing left over, and neither grows. Sizing them at max-content instead
// would push content off the edge of a box that had room for it.
func (l *layouter) sizeColumns(columns []gridTrack, items []*gridItem,
	width, gap style.Unit, stretch bool) {

	asks := make([]trackAsk, 0, len(items))
	for _, it := range items {
		min, max := l.gridItemWidths(it)
		asks = append(asks, trackAsk{
			from: it.column, span: it.place[1].span, min: min, max: max,
		})
	}
	l.resolveTracks(columns, asks, gap,
		maxZero(width.Sub(gap.Mul(float64(len(columns)-1)))), true, stretch)
}

// gridItemWidths is what one item asks of its column: its min-content and
// max-content contributions, which are its content's unless it states a width
// of its own — a box that did is that wide however its words would break.
func (l *layouter) gridItemWidths(it *gridItem) (style.Unit, style.Unit) {
	if declared, ok := l.intrinsicLength(it.box, "width"); ok {
		return declared.Add(it.horizontal()), declared.Add(it.horizontal())
	}
	got := l.contentWidths(it.box)
	return got.min.Add(it.horizontal()), got.max.Add(it.horizontal())
}

// sizeRows is the same on the block axis, with one difference that is not a
// difference in the algorithm: a row's content size is not asked of the box
// tree but of the layout, because how tall an item is depends on how wide it
// was made. There is no second number to grow towards — a block is as tall as
// it is at the width it was given — so a row's base and its growth limit are
// the same, and only a container that states a height has anything to give the
// rows beyond them.
func (l *layouter) sizeRows(rows []gridTrack, items []*gridItem,
	height style.Unit, definite bool, gap style.Unit, stretch bool) {

	asks := make([]trackAsk, 0, len(items))
	for _, it := range items {
		asks = append(asks, trackAsk{
			from: it.row, span: it.place[0].span, min: it.height, max: it.height,
		})
	}
	l.resolveTracks(rows, asks, gap,
		maxZero(height.Sub(gap.Mul(float64(len(rows)-1)))), definite, stretch)
}

// resolveTracks is §12.4 to §12.8 on either axis: the base sizes, and then what
// the room left over is spent on.
//
// The three ways it is spent run in this order and each takes what the one
// before left. §12.6 grows every track towards its growth limit, which is what
// fills a container out of its own content. §12.7 gives what is still over to
// the flexible tracks in proportion to their factors — and only to them, which
// is why a container with a "1fr" in it stretches nothing else. §12.8 is Box
// Alignment's "stretch", the initial value of justify-content and align-content
// in a grid, and it goes to the automatic tracks alone: "max-content" asked for
// the size of its content and got it, while "auto" is the one that says it will
// take more if there is more.
func (l *layouter) resolveTracks(tracks []gridTrack, asks []trackAsk,
	gap, room style.Unit, definite, stretch bool) {

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
	spreadSpanningAsks(tracks, limits, asks, gap)
	if !definite {
		return
	}
	free := room.Sub(sumTracks(tracks))
	if free <= 0 {
		return
	}
	free = growToLimits(tracks, limits, free)
	if expandFlexibleTracks(tracks, room) {
		return
	}
	if stretch {
		stretchAutoTracks(tracks, free)
	}
}

// resolveTrackSize turns one sizing function into a number, given what the
// items in the track need.
//
// "auto" is the one that answers differently at each end, and the caller says
// which end it is asking about by what it passes as its own: the largest
// minimum at the low end, max-content at the high one. A flexible function
// arrives here only as a maximum — the gate refuses minmax() with a flexible
// minimum, and a bare "1fr" is written out as minmax(auto, 1fr) — and the
// number it comes to is never read, because §12.4 gives a track with a flexible
// maximum a growth limit equal to its base and §12.6 leaves it alone until
// §12.7 hands it a share.
//
// A growth limit smaller than the base size is left as it is rather than
// clamped up to it. Nothing needs the clamp: the limit is only ever read as
// "how much further may this grow", and a track whose base is already past it
// grows by nothing either way.
func resolveTrackSize(f trackSize, min, max, auto style.Unit) style.Unit {
	switch f.kind {
	case trackFixed:
		return maxZero(f.size)
	case trackMin:
		return min
	case trackMax:
		return max
	}
	return auto
}

// trackAsk is what one item needs of the tracks it covers: where it starts, how
// many it spans, and the two sizes it would like across them.
type trackAsk struct {
	from, span int
	min, max   style.Unit
}

// spreadSpanningAsks is §12.5's other half: an item that covers more than one
// track asks something of all of them together, and what it asks is shared out.
//
// An item spanning two columns that needs 300px says nothing about either
// column on its own — any pair of widths adding to 300 would hold it — so the
// tracks are sized from the items inside them first, and only what is *still*
// missing is spread. That is why this runs after the single-track pass and in
// order of span: a wider item's ask is measured against tracks that the
// narrower ones have already grown.
//
// The shortfall goes to the tracks that can take it, which are the ones sized
// from their content. A fixed track is the size it states whatever spans it,
// and giving it a share would be overruling the stylesheet with an item.
func spreadSpanningAsks(tracks []gridTrack, limits []style.Unit, asks []trackAsk,
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
func growToLimits(tracks []gridTrack, limits []style.Unit, free style.Unit) style.Unit {
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

// expandFlexibleTracks is §12.7, and reports whether there were any.
//
// The clause worth naming is the one for factors adding to less than one: two
// "0.25fr" tracks between them asked for a quarter of the free space each and
// take exactly that, leaving the rest of the container empty. Without it a lone
// "0.5fr" track would fill the container, which is the same picture "1fr" gives
// and is why the two have to be told apart. It is §9.7.4b of flexbox, in the
// other specification and in the same words.
//
// A flexible track never comes out smaller than its content: the share is what
// the fraction is worth, and the base is what the words inside need.
func expandFlexibleTracks(tracks []gridTrack, room style.Unit) bool {
	factors := 0.0
	fixed := style.Unit(0)
	for _, t := range tracks {
		if t.flexible() {
			factors += t.max.factor
			continue
		}
		fixed = fixed.Add(t.base)
	}
	if factors == 0 {
		return false
	}
	leftover := maxZero(room.Sub(fixed))
	each := leftover
	if factors > 1 {
		each = leftover.Div(factors)
	}
	for i := range tracks {
		if !tracks[i].flexible() {
			continue
		}
		if share := each.Mul(tracks[i].max.factor); share > tracks[i].base {
			tracks[i].base = share
		}
	}
	return true
}

// stretchAutoTracks is §12.8: what is still over goes to the automatic tracks,
// equally, because justify-content and align-content are at "normal" and
// "normal" behaves as "stretch" in a grid.
func stretchAutoTracks(tracks []gridTrack, free style.Unit) {
	autos := 0
	for _, t := range tracks {
		if t.stretches() {
			autos++
		}
	}
	if autos == 0 || free <= 0 {
		return
	}
	each := free.Div(float64(autos))
	for i := range tracks {
		if tracks[i].stretches() {
			tracks[i].base = tracks[i].base.Add(each)
		}
	}
}

func sumTracks(tracks []gridTrack) style.Unit {
	out := style.Unit(0)
	for _, t := range tracks {
		out = out.Add(t.base)
	}
	return out
}

// layOutGridItem lays one item out at the size its cell gives it.
//
// The sizes are forced rather than declared, which is the path a flex item and
// a table cell take: the item goes through ordinary block layout with its
// geometry decided by the caller, so everything inside it works as it does
// anywhere else. The width is the column's less the item's own margins and
// edges, because a cell is a room and the item's border box goes inside it.
func (l *layouter) layOutGridItem(it *gridItem, column, row style.Unit, hasRow bool,
	width style.Unit, origin flow) *Fragment {

	geom := &forcedGeometry{
		margin: it.margin,
		width:  maxZero(column.Sub(it.horizontal())),
	}
	if hasRow {
		geom.height, geom.hasHeight = maxZero(row.
			Sub(it.border.Vertical()).Sub(it.padding.Vertical())), true
	}
	return outOfClamp(l, func() *Fragment {
		f, _ := l.blockIn(it.box, width,
			flow{ctx: &floatContext{}, cbHeight: origin.cbHeight, cbDefinite: origin.cbDefinite},
			geom)
		return f
	})
}

// gridItems gathers the container's items in order-modified document order,
// which is where every question about placement is answered from.
func (l *layouter) gridItems(b *Box, width style.Unit) []*gridItem {
	var out []*gridItem
	for _, c := range b.Children {
		if c.IsText() || (c.Anonymous() && len(c.Children) == 0) || c.outOfFlow() {
			continue
		}
		it := &gridItem{
			box:     c,
			margin:  l.edges(c, "margin", width),
			border:  l.borderWidths(c),
			padding: l.paddingOf(c, width),
			order:   orderOf(c),
		}
		// The gate has read these already and refused the container where it
		// could not; what comes back here is what it accepted.
		it.place[0], _ = l.placementOf(c, "grid-row", "grid-row-start", "grid-row-end")
		it.place[1], _ = l.placementOf(c,
			"grid-column", "grid-column-start", "grid-column-end")
		out = append(out, it)
	}
	// §6.2's order-modified document order, and the sort is stable for the
	// reason it is in a flex container: items that named the same order keep
	// the order the document put them in, which is what makes "order: 1" mean
	// "after everything that did not ask".
	slices.SortStableFunc(out, func(x, y *gridItem) int { return x.order - y.order })
	return out
}

// gridGap reads one of the two gaps. "normal" is zero in a grid, as it is in a
// flex container and as it is not in a multi-column one; see flexGap, which
// makes the same argument about the same keyword.
func (l *layouter) gridGap(b *Box, property string, width style.Unit) style.Unit {
	if v, ok := l.lengthOf(b, property, width); ok && v >= 0 {
		return v
	}
	return 0
}

// trackList reads one of the two templates, or says it holds something this
// slice does not size.
//
// "none" and an empty declaration are no tracks at all, which is not a refusal:
// a container with no explicit columns has one implicit column, and one with no
// explicit rows has as many implicit rows as its items need.
func (l *layouter) trackList(b *Box, property string, width style.Unit,
	room trackRoom) (tracks []gridTrack, fit, ok bool) {

	raw := strings.TrimSpace(b.Style[property])
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil, false, true
	}
	vals, _ := css.ParseComponentValues(raw)
	return l.tracksFrom(b, vals, width, room, true)
}

// trackRoom is what an automatic repetition is counted against: how much the
// container has along the axis, whether that is a number at all, and the gap
// that goes between one track and the next.
//
// Only "repeat(auto-fill, …)" reads it. Every other track list is the same list
// however big the container is, which is why the rest of this file never asks.
type trackRoom struct {
	size     style.Unit
	definite bool
	gap      style.Unit
}

// tracksFrom turns a track list into tracks, or returns false for anything in
// it this slice does not size: a named line, a minmax(), a subgrid, a repeat()
// that is not a plain count.
func (l *layouter) tracksFrom(b *Box, vals []css.ComponentValue, width style.Unit,
	room trackRoom, mayRepeat bool) (tracks []gridTrack, fit, ok bool) {

	// The list is read in two halves because an automatic repetition cannot be
	// counted until everything else in the list has been: §7.2.3.2 fits as many
	// as will go in what is *left*, so the tracks written beside it have to be
	// sized first. before and after are the tracks either side of it, and one
	// is what it repeats.
	var before, after, one []gridTrack
	auto := false
	for _, part := range splitValuesOnWhitespace(vals) {
		if len(part) != 1 {
			return nil, false, false
		}
		v := part[0]
		if v.IsFunction() && strings.EqualFold(v.Token.Value, "repeat") {
			if !mayRepeat {
				return nil, false, false
			}
			got, kind, ok := l.repeatedTracks(b, v.Values, width, room)
			if !ok {
				return nil, false, false
			}
			if kind == repeatCounted {
				if auto {
					after = append(after, got...)
					continue
				}
				before = append(before, got...)
				continue
			}
			if auto {
				// §7.2.3.2 allows one automatic repetition in a track list, and
				// the reason is arithmetic rather than taste: two of them would
				// each be counted against the room the other had not taken yet.
				return nil, false, false
			}
			auto, one, fit = true, got, kind == repeatFit
			continue
		}
		got, ok := l.trackFrom(b, v, width)
		if !ok {
			return nil, false, false
		}
		if auto {
			after = append(after, got)
			continue
		}
		before = append(before, got)
	}
	if !auto {
		return before, false, len(before) > 0
	}
	out := append([]gridTrack(nil), before...)
	for i, n := 0, l.autoRepetitions(one, before, after, room); i < n; i++ {
		out = append(out, one...)
	}
	out = append(out, after...)
	if len(out) > maxRepeatedTracks {
		return nil, false, false
	}
	return out, fit, len(out) > 0
}

// autoRepetitions is §7.2.3.2: how many times a "repeat(auto-fill, …)" goes
// into what the container has left.
//
// The room it is counted against is the container's own size less the tracks
// written beside it and all the gaps, and the size a track counts as is its
// *maximum* where that is a length and its minimum otherwise — which is what
// makes "repeat(auto-fill, minmax(200px, 1fr))" fit as many 200px columns as
// there is room for rather than one column of everything.
//
// One is the answer wherever the question cannot be asked: a repeated list that
// is all content-sized has nothing to divide by, and a container with no
// definite size along the axis has nothing to divide — its room is nought,
// which the arithmetic below reaches without a clause of its own. That is the
// specification's own fallback and not a bail-out: the track list is still the
// list, it is simply written once.
func (l *layouter) autoRepetitions(one, before, after []gridTrack, room trackRoom) int {
	each := style.Unit(0)
	for _, t := range one {
		size, ok := definiteTrackSize(t)
		if !ok {
			return 1
		}
		each = each.Add(size)
	}
	each = each.Add(room.gap.Mul(float64(len(one))))
	if each <= 0 {
		return 1
	}
	left := room.size.Add(room.gap)
	for _, t := range append(append([]gridTrack(nil), before...), after...) {
		size, _ := definiteTrackSize(t)
		left = left.Sub(size).Sub(room.gap)
	}
	if left <= 0 {
		return 1
	}
	n := int(left / each)
	if n < 1 {
		return 1
	}
	return n
}

// definiteTrackSize is what a track counts as while the repetitions are being
// worked out: a length if either end is one, and nothing if neither is.
func definiteTrackSize(t gridTrack) (style.Unit, bool) {
	if t.max.kind == trackFixed {
		return maxZero(t.max.size), true
	}
	if t.min.kind == trackFixed {
		return maxZero(t.min.size), true
	}
	return 0, false
}

// repeatKind is which of repeat()'s two counts was written: a number, which is
// a spelling of the list it holds, or "auto-fill", which is a question about
// the container.
type repeatKind uint8

const (
	repeatCounted repeatKind = iota
	repeatFill
	repeatFit
)

// repeatedTracks expands "repeat(<integer>, <track-list>)" and reads the count
// of "repeat(auto-fill, <track-list>)" without expanding it — the caller does
// that, once it knows what the rest of the list took.
//
// "auto-fit" is "auto-fill" with the empty tracks collapsed afterwards, and
// collapsing is the caller's too: which tracks are empty is a question about
// where the items landed, which is not known here.
func (l *layouter) repeatedTracks(b *Box, args []css.ComponentValue,
	width style.Unit, room trackRoom) ([]gridTrack, repeatKind, bool) {

	comma := -1
	for i, v := range args {
		if v.IsToken() && v.Token.Kind == css.Comma {
			comma = i
			break
		}
	}
	if comma < 0 {
		return nil, repeatCounted, false
	}
	head := splitValuesOnWhitespace(args[:comma])
	if len(head) != 1 || len(head[0]) != 1 || !head[0][0].IsToken() {
		return nil, repeatCounted, false
	}
	token := head[0][0].Token

	one, _, ok := l.tracksFrom(b, args[comma+1:], width, room, false)
	if !ok {
		return nil, repeatCounted, false
	}
	if token.Kind == css.Ident {
		switch strings.ToLower(token.Value) {
		case "auto-fill":
			return one, repeatFill, true
		case "auto-fit":
			return one, repeatFit, true
		}
		return nil, repeatCounted, false
	}
	if token.Kind != css.Number || !token.IsInteger {
		return nil, repeatCounted, false
	}
	count := int(token.Number)
	if count <= 0 || count > maxRepeatedTracks {
		return nil, repeatCounted, false
	}
	var out []gridTrack
	for i := 0; i < count; i++ {
		out = append(out, one...)
	}
	if len(out) > maxRepeatedTracks {
		return nil, repeatCounted, false
	}
	return out, repeatCounted, true
}

// maxRepeatedTracks bounds what a repeat() may expand to. A document is
// untrusted and "repeat(1000000, 1fr)" is a line of CSS; the bound is far above
// any grid anyone lays out and turns a memory exhaustion into a finding.
const maxRepeatedTracks = 1000

// trackFrom reads one track, which is either a size written once or a minmax()
// that writes the two ends separately.
//
// A single size is that size at both ends, and the two that are not are the two
// that mean different things there: "auto" and a flexible track are both
// "whatever the items need" at the low end. Writing them out as a minmax() here
// rather than special-casing them later is what lets the sizing ask one
// question of each end and never ask which spelling it came from.
func (l *layouter) trackFrom(b *Box, v css.ComponentValue, width style.Unit) (gridTrack, bool) {
	if v.IsFunction() && strings.EqualFold(v.Token.Value, "minmax") {
		return l.minmaxTrack(b, v.Values, width)
	}
	size, ok := l.trackSizeFrom(b, v, width)
	if !ok {
		return gridTrack{}, false
	}
	switch size.kind {
	case trackAuto, trackFlex:
		return gridTrack{min: trackSize{kind: trackAuto}, max: size}, true
	}
	return gridTrack{min: size, max: size}, true
}

// minmaxTrack reads §7.2.2's minmax(), whose two arguments are the track's two
// sizing functions and are not interchangeable: the minimum may not be
// flexible, because a track that took a share of the free space as its *floor*
// would be asking for the space before there was any to have.
func (l *layouter) minmaxTrack(b *Box, args []css.ComponentValue,
	width style.Unit) (gridTrack, bool) {

	var parts [][]css.ComponentValue
	var cur []css.ComponentValue
	for _, v := range args {
		if v.IsToken() && v.Token.Kind == css.Comma {
			parts = append(parts, cur)
			cur = nil
			continue
		}
		cur = append(cur, v)
	}
	parts = append(parts, cur)
	if len(parts) != 2 {
		return gridTrack{}, false
	}
	var out gridTrack
	for i, part := range parts {
		one := splitValuesOnWhitespace(part)
		if len(one) != 1 || len(one[0]) != 1 {
			return gridTrack{}, false
		}
		size, ok := l.trackSizeFrom(b, one[0][0], width)
		if !ok {
			return gridTrack{}, false
		}
		if i == 0 {
			if size.kind == trackFlex {
				return gridTrack{}, false
			}
			out.min = size
			continue
		}
		out.max = size
	}
	return out, true
}

// trackSizeFrom reads one sizing function.
func (l *layouter) trackSizeFrom(b *Box, v css.ComponentValue,
	width style.Unit) (trackSize, bool) {

	if v.IsToken() && v.Token.Kind == css.Ident {
		switch strings.ToLower(v.Token.Value) {
		case "auto":
			return trackSize{kind: trackAuto}, true
		case "min-content":
			return trackSize{kind: trackMin}, true
		case "max-content":
			return trackSize{kind: trackMax}, true
		}
		return trackSize{}, false
	}
	if v.IsToken() && v.Token.Kind == css.Dimension &&
		strings.EqualFold(v.Token.Unit, "fr") {
		if v.Token.Number < 0 {
			return trackSize{}, false
		}
		return trackSize{kind: trackFlex, factor: v.Token.Number}, true
	}
	length, ok := l.lengthOfValues(b, []css.ComponentValue{v})
	if !ok {
		return trackSize{}, false
	}
	size, ok := length.Resolve(width, true)
	if !ok || size < 0 {
		return trackSize{}, false
	}
	return trackSize{kind: trackFixed, size: size}, true
}

// splitValuesOnWhitespace is style's splitter, which is not exported and is two
// lines. A track list is written as space-separated values and read that way.
func splitValuesOnWhitespace(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	var cur []css.ComponentValue
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, v)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// refusesToGrid is why a grid container is laid out as a column of blocks
// anyway, or the empty string if it is arranged.
//
// Every clause is a way for the table of tracks to stop being the picture the
// document asked for, and each is stated as a condition on the box. The set is
// deliberately narrow, and narrowing is the safe direction: a box refused here
// is laid out exactly as it was before this file existed and is reported, which
// is the honest answer; a box arranged that should not have been is a page that
// is quietly wrong.
func (l *layouter) refusesToGrid(b *Box, width style.Unit) string {
	if _, _, ok := l.trackList(b, "grid-template-columns", width, trackRoom{}); !ok {
		return "its columns are written with something this engine does not " +
			"size, such as a named line, a minmax() or a repeat() that counts " +
			"how many will fit"
	}
	if _, _, ok := l.trackList(b, "grid-template-rows", width, trackRoom{}); !ok {
		return "its rows are written with something this engine does not size, " +
			"such as a named line, a minmax() or a repeat() that counts how " +
			"many will fit"
	}
	switch trimmedLower(b.Style["grid-template-areas"]) {
	case "", "none":
	default:
		return "its cells are named by a template, which places items by name " +
			"rather than in order"
	}
	switch trimmedLower(b.Style["grid-auto-flow"]) {
	case "", "row":
	default:
		// "column" fills down before across and "dense" goes back for the
		// holes an item that did not fit left behind. Both are placement
		// algorithms of their own, and this slice has one.
		return "its items are placed by a flow this engine does not follow"
	}
	switch trimmedLower(b.Style["grid-auto-rows"]) {
	case "", "auto":
	default:
		return "its implicit rows are given a size of their own"
	}
	switch trimmedLower(b.Style["grid-auto-columns"]) {
	case "", "auto":
	default:
		return "its implicit columns are given a size of their own"
	}
	if why := refusesGridAlignment(b); why != "" {
		return why
	}
	if isRTL(b) {
		// The columns are laid out from the left edge, which in a right-to-left
		// grid is where they end rather than where they start. It is the same
		// reversal a flex row waited on, and it is one change to the placement
		// rather than a condition on each track.
		return "its columns run from the right, which reverses every position " +
			"across it"
	}
	for _, c := range b.Children {
		if c.IsText() || (c.Anonymous() && len(c.Children) == 0) || c.outOfFlow() {
			continue
		}
		switch trimmedLower(c.Style["grid-area"]) {
		case "", "auto":
		default:
			// The four-part shorthand, which is how an item is put in a named
			// area — and an area is a placement algorithm of its own.
			return "one of its items is placed by area rather than by line"
		}
		_, row := l.placementOf(c, "grid-row", "grid-row-start", "grid-row-end")
		_, column := l.placementOf(c,
			"grid-column", "grid-column-start", "grid-column-end")
		if !row || !column {
			return "one of its items names a line this engine cannot find, " +
				"such as a name from a template or a number counted back from " +
				"the end of the grid"
		}
		if why := refusesGridAlignment(c); why != "" {
			return why
		}
		if auto := l.autoMarginEdges(c); auto != (Edges{}) {
			// §10.2 gives an auto margin the room left in the cell before the
			// alignment properties see any of it, which is a second way of
			// spending the same space and is not this one.
			return "one of its items has an automatic margin, which takes the " +
				"room left in its cell"
		}
	}
	return ""
}

// refusesGridAlignment is the alignment half of the gate, which is the same six
// properties on the container and on an item.
//
// What is left to refuse is the two that name something other than an end of an
// axis. A baseline alignment lines the *text* of the items in a row up with
// each other, which is a measurement across a row rather than a position in a
// cell; "safe" and "unsafe" are a second answer to what happens when an item
// does not fit, and this has one already — the overflow falls where the
// arithmetic puts it.
func refusesGridAlignment(b *Box) string {
	for _, p := range [...]string{"justify-content", "align-content",
		"justify-items", "align-items", "justify-self", "align-self"} {
		switch value := trimmedLower(b.Style[p]); value {
		case "", "normal", "stretch", "auto", "legacy", "start", "end", "center",
			"flex-start", "flex-end", "self-start", "self-end", "left", "right",
			"space-between", "space-around", "space-evenly":
		default:
			return "its tracks or its items are aligned by a rule this engine " +
				"does not apply, such as to a shared baseline"
		}
	}
	return ""
}

// arrangesGrid reports whether the container is arranged here, and says so when
// it is not.
//
// The finding is raised once per box rather than once per declaration, which is
// the argument layout/flex.go's own gate makes: whether the page is wrong is a
// question about the box, and the same stylesheet may be right for one
// container and wrong for the next.
func (l *layouter) arrangesGrid(b *Box, width style.Unit) bool {
	why := l.refusesToGrid(b, width)
	if why == "" {
		return true
	}
	if l.reportedGrid == nil {
		l.reportedGrid = map[*Box]bool{}
	}
	if !l.reportedGrid[b] {
		l.reportedGrid[b] = true
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "this grid container was laid out as a column of blocks because " +
				why + "; its items are stacked rather than placed in cells",
			Path:     PathOf(b.Element),
			Property: "display",
		})
	}
	return false
}
