package layout

import (
	"slices"
	"strconv"
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
// The explicit tracks of "grid-template-columns", "grid-template-rows" and
// "grid-template-areas", and the implicit tracks the items reach into, before
// the explicit grid as well as after it, sized by "grid-auto-columns" and
// "grid-auto-rows". A track size may be a length, a percentage, a fraction of
// the free space, one of the three content keywords, or a minmax() of two of
// them; "repeat()" writes a list more than once, a counted number of times or
// as many as fit ("auto-fill" and "auto-fit"). Items are placed by §8.5 in
// order-modified document order: where their grid-row-start, grid-row-end,
// grid-column-start and grid-column-end put them — a line number, a span, or
// the name of a template area — and by the automatic flow, sparse or dense,
// along the rows or down the columns, for whatever they left to it. The gaps
// are "row-gap" and "column-gap", which a grid reads on both axes rather than
// one, and the tracks and items are aligned by Box Alignment's keywords.
//
// What is refused: a named line in a track list, fit-content(), a repeat()
// inside a repeat() or two automatic ones in a list, a flexible minimum, a
// list or a template of more than maxRepeatedTracks tracks, a template that
// does not draw rectangles, a line counted back from the end of the grid or
// named by anything but a template area, a baseline or "safe" alignment, and
// an automatic margin on an item. Each is refused with a finding and laid out
// as it was before this file existed, which is as a column of blocks. The gate is the same shape as
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

	// The inline axis, which is the only one a writing mode turns: a grid's
	// rows run down the page whatever the direction is, and its columns run
	// from whichever side the text starts at. flexAxis is what layout/flex.go
	// asks the same question with, and a grid's columns are a flex row's main
	// axis — same axis, same keywords, same mirror.
	axis := flexAxis{rtl: isRTL(b)}
	areas, _ := l.areasOf(b)
	items := l.gridItems(b, width, areas)
	if len(items) == 0 {
		// A container with nothing to place still has its out-of-flow children
		// to record. Nobody else will: the block walk is what does that, and
		// this is in its place.
		empty, _ := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite)
		l.deferGridOutOfFlow(b, parent, width,
			l.clampHeight(b, empty, width, origin.cbHeight, origin.cbDefinite))
		return 0
	}
	height, definite := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite)
	// A percentage gap is of the container's size on the gap's own axis — Box
	// Alignment §8 — and a row gap was being taken of the container's width.
	// A percentage of a height the container does not state resolves to
	// nothing, which is what a gap of an indefinite size comes to.
	columnGap := l.gridGap(b, "column-gap", width, true)
	rowGap := l.gridGap(b, "row-gap", height, definite)

	columns, rows := l.gridTracks(b, items, areas, width,
		trackRoom{size: width, definite: true, gap: columnGap, property: "grid-template-columns"},
		trackRoom{size: height, definite: definite, gap: rowGap, property: "grid-template-rows"})

	// The block axis takes no axis of its own: nothing this engine lays out
	// runs a grid's rows backwards, and the gate has refused the two keywords
	// that would have to be turned if something did.
	across, down := l.gridAlignment(b, "justify-items", axis),
		l.gridAlignment(b, "align-items", flexAxis{})
	l.sizeColumns(columns, items, width, columnGap, l.gridContentAlignment(b, "justify-content"))

	// §10.3 and §10.4: where the tracks sit in a container that is bigger than
	// they are. With everything at its initial value there is nothing over —
	// the automatic tracks took it — so these offsets are nought and the whole
	// of the arithmetic is skipped by being zero rather than by a branch.
	//
	// The columns' spacing is settled here, before any item is sized, because
	// it is part of an item's area: §11.1 treats the space justify-content puts
	// between two tracks as a widened gutter, and a gutter an item spans is its
	// own. A spanning item was the tracks it covered and the gaps between them,
	// so under "space-between" it stopped where its last track ended and left
	// the distributed space beside it empty (audit C154).
	columnLead, columnBetween := l.trackSpacing(b, "justify-content", axis, columns,
		width, columnGap)

	// How wide each item is used at, which is its cell's width where it is
	// stretched and its own fit-content width where it is aligned instead. It
	// has to be settled before the heights are measured and cannot be settled
	// before the columns are sized: it is the one thing between the two.
	for _, it := range items {
		it.across = l.itemAlignment(it, "justify-self", across, axis)
		it.down = l.itemAlignment(it, "align-self", down, flexAxis{})
		cell := areaSpan(columns, it.column, it.place[1].span, columnGap, columnBetween)
		switch declared, stated := l.gridDeclaredSize(it, "width", cell, true); {
		case stated:
			// An item that stated a width is a box of that width placed in its
			// area, not a box resized to it. §6.6 makes "stretch" apply only
			// where the item's own size in the axis is auto, and this asked
			// nowhere: a fifty-pixel item in a two-hundred-pixel cell came out
			// two hundred, and so did one asking for half the cell.
			it.width = declared
		case it.across == crossStretch:
			it.width = l.gridStretch(it, cell)
		default:
			it.width = l.gridFitContent(it, cell)
		}
	}

	// Each item laid out at that width, which is what tells the rows how tall
	// they are. The fragments are thrown away: an item is laid out again at the
	// size its cell settles on, because a height changes where an item's
	// content sits inside it.
	//
	// Everything the measuring layouts did is thrown away with them, and not
	// only the out-of-flow boxes they found: a positioned box inside one
	// recorded its fragment, and a box positioned against it would have been
	// placed against the measured one. See speculative.go.
	before := l.checkpoint(nil, nil)
	for _, it := range items {
		frag := l.layOutGridItem(it, it.width, 0, false, width, origin)
		it.height = frag.BorderRect.H.Add(it.margin.Vertical())
	}
	l.rollback(before)

	// The container's own min-height and max-height, which an auto-height
	// grid's flexible rows are sized within; see resolveTracks.
	l.sizeRows(rows, items, height, definite, rowGap,
		l.gridContentAlignment(b, "align-content"),
		l.clampHeight(b, 0, width, origin.cbHeight, origin.cbDefinite),
		l.clampHeight(b, style.MaxUnit, width, origin.cbHeight, origin.cbDefinite))

	// §12's answer for the container itself: the tracks and the gaps between
	// them, which is what a grid comes to when nothing states its height.
	inner := gridInner(rows, rowGap)

	rowLead, rowBetween := l.trackSpacing(b, "align-content", flexAxis{}, rows,
		gridInner(rows, rowGap), rowGap)
	if definite {
		rowLead, rowBetween = l.trackSpacing(b, "align-content", flexAxis{}, rows,
			height, rowGap)
	}

	columnEdges, rowEdges := trackEdgesOf(columns), trackEdgesOf(rows)
	parent.baselineChild = 0
	for _, it := range items {
		cellHeight := areaSpan(rows, it.row, it.place[0].span, rowGap, rowBetween)
		// The same clause on the other axis. it.height already holds what the
		// item's own layout came to, which honours a declared height; stretch
		// was overwriting it.
		areaDefinite := definite || tracksDefinite(rows, it.row, it.place[0].span)
		if declared, stated := l.gridDeclaredSize(it, "height", cellHeight, areaDefinite); stated {
			it.height = declared
		} else if it.down == crossStretch {
			it.height = cellHeight
		}
		it.frag = l.layOutGridItem(it, it.width,
			maxZero(it.height.Sub(it.margin.Vertical())), true, width, origin)

		x := columnEdges.start(it.column, columnGap).
			Add(columnLead).Add(columnBetween.Mul(float64(it.column)))
		y := rowEdges.start(it.row, rowGap).
			Add(rowLead).Add(rowBetween.Mul(float64(it.row)))
		x = x.Add(alignmentOffset(it.across,
			areaSpan(columns, it.column, it.place[1].span, columnGap, columnBetween),
			it.width))
		// Everything above is measured from where the columns start, which
		// under "rtl" is the right edge. Mirroring the item's margin box once,
		// here, is what turns that into a place on the page — the same one
		// step layout/flex.go takes for the same reason, and for the same
		// reason it is one step: every position on the axis reverses together.
		it.frag.BorderRect.X = axis.mainAt(x, it.width.Add(it.margin.Horizontal()),
			width).Add(it.margin.Left)
		it.frag.BorderRect.Y = y.
			Add(alignmentOffset(it.down, cellHeight, it.height)).
			Add(it.margin.Top)
		if it.row == 0 && parent.baselineChild == 0 {
			// Grid §11.8: the container's first baseline is the first item in
			// grid order whose area is in the first row, which is not always
			// the first item. See containerFirstBaseline.
			parent.baselineChild = len(parent.Children) + 1
		}
		parent.Children = append(parent.Children, it.frag)
	}
	content := inner
	if definite {
		content = height
	}
	l.deferGridOutOfFlow(b, parent, width,
		l.clampHeight(b, content, width, origin.cbHeight, origin.cbDefinite))
	return content
}

// gridTracks is §7 and §8 for one container: the explicit tracks the templates
// write, the implicit ones the items reach into, and every item placed in them.
//
// It is its own function because two callers ask it. Layout asks with the
// container's width and height, and the intrinsic measurement asks with no
// width at all — a float or a table cell holding a grid needs to know how wide
// the grid wants to be before it has a width to give it, and what it wants is
// its columns, which only this knows how to make. Two copies of the placement
// would be two chances to place the items differently, and the grid measured
// would not be the grid drawn.
func (l *layouter) gridTracks(b *Box, items []*gridItem, areas gridAreas, width style.Unit,
	columnRoom, rowRoom trackRoom) (columns, rows []gridTrack) {

	columns, fit, _ := l.trackList(b, "grid-template-columns", width, columnRoom)
	flow := l.autoFlow(b)
	autoColumns := l.implicitTracks(b, "grid-auto-columns", width)
	autoRows := l.implicitTracks(b, "grid-auto-rows", width)
	for len(columns) < areas.columns {
		// §7.3: the picture makes the explicit grid. A template of two words
		// per row has two columns whether or not grid-template-columns named
		// them, and the ones it did not name are "auto".
		columns = append(columns, autoTrack())
	}
	rows, fitRows, _ := l.trackList(b, "grid-template-rows", width, rowRoom)
	for len(rows) < areas.rows {
		rows = append(rows, autoTrack())
	}

	// §8.3.1: an item may end at a line and span back past the first line of
	// the explicit grid — "span 3 / 2" — and the grid then has implicit tracks
	// *before* the explicit ones. They are made here and every line an item
	// named is moved along by as many, so that the placement below counts
	// from the first track there is, which is where §8.5 starts its cursor.
	// The start was clamped to the first explicit track instead, so the item
	// covered the explicit tracks it did not ask for.
	var explicitColumns, explicitRows int
	columns, explicitColumns, fit = withLeadingTracks(columns, autoColumns,
		roomBefore(items, 1), fit)
	rows, explicitRows, fitRows = withLeadingTracks(rows, autoRows,
		roomBefore(items, 0), fitRows)

	// §7.5: an item that named a track past the explicit grid is not left
	// hanging off the end of it — the grid grows to hold it, and the tracks it
	// grew by are sized by grid-auto-columns or grid-auto-rows.
	//
	// This is also what gives a container with no template at all its one
	// column: every item spans at least one track, so at least one track is
	// asked for. §7.1's "there is always a grid" needs no clause of its own.
	for len(columns) < tracksNeeded(items, 1, explicitColumns) {
		columns = append(columns, implicitTrack(autoColumns, len(columns), explicitColumns))
	}
	for len(rows) < tracksNeeded(items, 0, explicitRows) {
		rows = append(rows, implicitTrack(autoRows, len(rows), explicitRows))
	}

	// §8.5: the items that named a line go where they asked, and the rest are
	// dealt into what is left. The axis the flow fills along is as long as the
	// template and the items made it, and grows only where items locked to one
	// track of the other axis need more room than it has; the other axis is
	// however far the items reached, and grows.
	var clamped bool
	var along, across int
	if flow.column {
		// The algorithm is the same one with the axes exchanged, so the items
		// are turned on their side, dealt, and turned back. Writing it twice
		// would be two chances to write it differently.
		transposeItems(items)
		across, along, clamped = placeItems(items, len(rows), flow.dense)
		transposeItems(items)
	} else {
		along, across, clamped = placeItems(items, len(columns), flow.dense)
	}
	// across is now how many columns the items reach and along how many rows.
	for len(columns) < across {
		columns = append(columns, implicitTrack(autoColumns, len(columns), explicitColumns))
	}
	for len(rows) < along {
		// The implicit rows, sized by grid-auto-rows — which is "auto" unless
		// the stylesheet said otherwise, and is the height every row of a card
		// grid gets when nothing draws them.
		rows = append(rows, implicitTrack(autoRows, len(rows), explicitRows))
	}
	if clamped {
		l.rec.ReportDetail(Finding{
			Rule:   RuleLimit,
			Source: AtHTML(offsetOf(b)),
			Message: "the items of this grid reach past " + strconv.Itoa(maxGridTracks) +
				" tracks, which is as many as this engine makes on one axis; the " +
				"ones that went further were put in the last track and overlap",
			Path:     PathOf(b.Element),
			Property: "grid-auto-flow",
		})
	}

	// §7.2.3.2's "auto-fit": the tracks no item landed in are collapsed. It
	// happens here, after the placement, because "no item landed in it" is a
	// question only the placement can answer — it was asked of the item *count*
	// instead, before anything had been placed, which is the same number only
	// when every item takes one track. One item spanning two of four hundred-
	// pixel tracks left one track standing and came out four hundred pixels
	// wide; two tracks stand now and it comes out two hundred, which is what
	// "auto-fit" means and why it fills a row with three cards where
	// "auto-fill" leaves room for a fourth.
	//
	// Only the tracks the repetition made are collapsed. Every unoccupied track
	// was, so "100px repeat(auto-fit, 50px)" holding one item in its second
	// column lost the 100px column the stylesheet wrote and put the item at the
	// left edge.
	columns = collapseUnusedTracks(columns, items, 1, fit)
	rows = collapseUnusedTracks(rows, items, 0, fitRows)
	return columns, rows
}

// deferGridOutOfFlow records the container's absolutely positioned children,
// which are not items and are placed once the tree is absolute.
//
// §10.1 gives such a box a static position "as if it were the sole grid item in
// a grid area whose edges coincide with the content edges of the grid
// container": the static-position rectangle is the content box, width by
// height, and the box is aligned in it by its own justify-self and align-self,
// the container's justify-items and align-items where those are "auto" — which
// is what itemAlignment asks of an item. It was put at the content box's start
// corner whatever it said, so "justify-self: end" on the box did nothing: the
// same fault audit C153 found in the flex container, and fixed the same way.
//
// The box's size is decided later, in layoutAbsolute, so what is recorded is
// the point the alignment names and how far across the rectangle that is; the
// box is moved back by the same fraction of itself once it is known. With
// every alignment at its initial value the fraction is nought and the point is
// the start corner, which is where the block walk would have put it.
func (l *layouter) deferGridOutOfFlow(b *Box, parent *Fragment, width, height style.Unit) {
	axis := flexAxis{rtl: isRTL(b)}
	across := l.gridAlignment(b, "justify-items", axis)
	down := l.gridAlignment(b, "align-items", flexAxis{})
	index := 0
	for _, c := range b.Children {
		if c.ListItem {
			index++
		}
		if !c.Position.outOfFlow() {
			continue
		}
		self := &gridItem{box: c}
		fx := alignmentFraction(l.itemAlignment(self, "justify-self", across, axis))
		fy := alignmentFraction(l.itemAlignment(self, "align-self", down, flexAxis{}))
		if axis.rtl {
			// Measured from the left, and the columns start on the right.
			fx = 1 - fx
		}
		x, y := width.Mul(fx), height.Mul(fy)
		l.deferAlignedAbsolute(c, parent, x, y, width.Sub(x), index, fx, fy)
	}
}

// alignmentFraction is how far across its area an aligned box sits, as a
// fraction of the room left: none at the start, all of it at the end, and half
// in the middle. "stretch" is the start, since an absolutely positioned box is
// sized by its own width and height rather than by the area.
func alignmentFraction(a flexAlign) float64 {
	switch a {
	case crossEnd:
		return 1
	case crossCenter:
		return 0.5
	}
	return 0
}

// gridAreas is §7.3's template: the names an author draws the grid with, and
// the band of tracks each one covers.
//
// "grid-template-areas: 'head head' 'nav main'" is a picture of the grid, one
// string per row and one word per cell, and it is the other way a grid is
// authored — the items say which area they are in and never count a line. It
// also *makes* the explicit grid: two words per row is two columns whether or
// not grid-template-columns says so.
type gridAreas struct {
	// at is where each name sits, as the same start-and-span pair an item
	// writes by hand. The row is first, as it is everywhere else here.
	at map[string][2]gridPlacement
	// rows and columns are how big the picture is, which is the size of the
	// explicit grid the template draws.
	rows, columns int
}

// areasOf reads the template, or says it is not one this engine can draw.
//
// The two ways a template is invalid are the two §7.3 names: a row with a
// different number of cells from the others is not a rectangle, and a name that
// appears in two places that do not touch is not an area. Both are refused
// rather than repaired — a template that does not describe a grid describes
// nothing, and guessing at what was meant would put boxes somewhere no
// stylesheet asked for.
func (l *layouter) areasOf(b *Box) (gridAreas, bool) {
	raw := strings.TrimSpace(b.Style.Get("grid-template-areas"))
	if raw == "" || strings.EqualFold(raw, "none") {
		return gridAreas{}, true
	}
	vals, _ := css.ParseComponentValues(raw)
	var rows [][]string
	for _, v := range splitValuesOnWhitespace(vals) {
		if len(v) != 1 || !v[0].IsToken() || v[0].Token.Kind != css.String {
			return gridAreas{}, false
		}
		cells := strings.Fields(v[0].Token.Value)
		if len(cells) == 0 {
			return gridAreas{}, false
		}
		if len(rows) > 0 && len(cells) != len(rows[0]) {
			return gridAreas{}, false
		}
		// The picture draws the explicit grid, and the explicit grid is bounded
		// by what a template may write whichever property writes it. The words
		// were not: fifty thousand of them made fifty thousand columns, and the
		// placement and sizing of every item was then paid for across all of
		// them. A template past the bound is refused like any other the engine
		// will not draw.
		if len(cells) > maxRepeatedTracks || len(rows) >= maxRepeatedTracks {
			return gridAreas{}, false
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return gridAreas{}, false
	}

	out := gridAreas{at: map[string][2]gridPlacement{}, rows: len(rows), columns: len(rows[0])}
	seen := map[string][4]int{}
	for r, cells := range rows {
		for c, name := range cells {
			if isNullCell(name) {
				continue
			}
			if !isAreaName(name) {
				return gridAreas{}, false
			}
			box, ok := seen[name]
			if !ok {
				seen[name] = [4]int{r, c, r + 1, c + 1}
				continue
			}
			// The name has been met before, so this cell has to extend the
			// rectangle it is already part of rather than start a second one.
			if r > box[2] || c > box[3] {
				return gridAreas{}, false
			}
			if c < box[1] {
				box[1] = c
			}
			if r+1 > box[2] {
				box[2] = r + 1
			}
			if c+1 > box[3] {
				box[3] = c + 1
			}
			seen[name] = box
		}
	}
	for name, box := range seen {
		// Every cell of the rectangle the name's corners describe has to carry
		// that name, or the name is in two places with a hole between them.
		for r := box[0]; r < box[2]; r++ {
			for c := box[1]; c < box[3]; c++ {
				if rows[r][c] != name {
					return gridAreas{}, false
				}
			}
		}
		out.at[name] = [2]gridPlacement{
			{start: box[0], definite: true, span: box[2] - box[0]},
			{start: box[1], definite: true, span: box[3] - box[1]},
		}
	}
	return out, true
}

// isNullCell reports whether a cell in the template is one nobody named: §7.3
// spells it as a run of dots, so "." and "..." are the same empty cell.
func isNullCell(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] != '.' {
			return false
		}
	}
	return true
}

// isAreaName reports whether a word in the template is a name rather than
// something this engine would have to make sense of.
//
// It is deliberately narrow — letters, digits, dashes and underscores, not
// starting with a digit — because a name here is matched against an item's
// grid lines by string equality, and a name that needed unescaping to compare would be
// compared wrongly rather than refused.
func isAreaName(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// gridPlacement is what one item said about where it goes on one axis: §8.3's
// two lines, read as a start and a span.
//
// A line number is one-based in CSS and zero-based here, and the conversion is
// done once at the edge — "grid-column-start: 2" is the second line, which is
// the first *track's* far edge, so the item starts in track one. Getting that
// wrong is off by one everywhere, which is why it is done in one place and
// named.
type gridPlacement struct {
	// start is the track the item begins in, and definite says the stylesheet
	// named it rather than leaving it to the flow. It is negative for an item
	// that ends at a line and spans back past the first — see roomBefore.
	start    int
	definite bool
	// span is how many tracks it covers, which is at least one.
	span int
}

// placementOf reads the two longhands that place an item on one axis.
//
// The shorthands — grid-row, grid-column and grid-area — are not read here or
// anywhere in layout. The cascade expands them into these four longhands, as it
// does every shorthand, so that a shorthand and a longhand of it decide between
// themselves by cascade order. They were registered as properties of their
// own and read in front of the longhands, so a shorthand beat its longhands
// however specific or late the longhand was: ".x { grid-column: 1 / 2 }
// #a { grid-column-start: 3 }" put the item in column 1 (audit C107).
//
// The grammar this reads is §8.3's without the named lines a template writes:
// a line number, a span of so many tracks, "auto" for either end, and the name
// of an area of grid-template-areas, which is what "grid-area: main" expands
// to on all four longhands. A named line in a track list is refused by the
// gate, and so is a name that no area makes.
//
// The two property names are passed rather than assembled from the axis, and
// that is not a style choice: style/unimplemented_test.go looks for every
// registered property as a literal in the source, and a name built out of
// "grid-" and a variable is a property nothing appears to read.
func (l *layouter) placementOf(b *Box, areas gridAreas, axis int,
	startName, endName string) (gridPlacement, bool) {

	start, ok := lineValue(b.Style.Get(startName), func(name string) (int, bool) {
		return areas.line(name, axis, false)
	})
	if !ok {
		return gridPlacement{}, false
	}
	end, ok := lineValue(b.Style.Get(endName), func(name string) (int, bool) {
		return areas.line(name, axis, true)
	})
	if !ok {
		return gridPlacement{}, false
	}
	return placementFrom(start, end)
}

// line is where a name puts one edge of an item on one axis, as a one-based
// line number of the explicit grid.
//
// §8.3's <custom-ident>: at a start edge the line named "name-start" if there is
// one, at an end edge "name-end", and otherwise the line named "name" itself.
// The only lines with names here are the ones §7.3.2 gives every area of the
// template, "area-start" and "area-end" on both axes, so "main" at a start edge
// is the first line of the area main, and so is "main-start" at either edge.
func (a gridAreas) line(name string, axis int, end bool) (int, bool) {
	suffix := "-start"
	if end {
		suffix = "-end"
	}
	if n, ok := a.lineNamed(name+suffix, axis); ok {
		return n, true
	}
	return a.lineNamed(name, axis)
}

// lineNamed is the line an area's implicit name stands for on one axis.
func (a gridAreas) lineNamed(name string, axis int) (int, bool) {
	if area, found := strings.CutSuffix(name, "-start"); found {
		if at, ok := a.at[area]; ok {
			return at[axis].start + 1, true
		}
	}
	if area, found := strings.CutSuffix(name, "-end"); found {
		if at, ok := a.at[area]; ok {
			return at[axis].start + at[axis].span + 1, true
		}
	}
	return 0, false
}

// gridLine is one end of a placement as it was written: a one-based line, a
// span, or neither for "auto".
type gridLine struct{ line, span int }

// placementFrom turns one pair of line values into a start and a span, by
// §8.3.1's rules for the pairs that do not say a start and an end outright.
//
// A span with no line is a span from wherever the flow puts the item, and
// two spans are the first of them: the specification drops the one the end
// wrote, and the larger of the two was taken. A line with no end is one track
// wide. A line and a span are the tracks from the line on. A span and a line
// end at the line and begin the span before it, which can be before the first
// line of the explicit grid — the start is then negative and the grid grows
// tracks in front (see roomBefore), where it was clamped to the first line.
// Two lines are the tracks between them, swapped when backwards and one track
// when equal.
func placementFrom(from, to gridLine) (gridPlacement, bool) {
	out := gridPlacement{span: 1}
	switch {
	case from.line != 0:
		out.start, out.definite = from.line-1, true
	case from.span > 0:
		// "span n" with no line of its own: the flow decides where, and the
		// span is what it takes when it gets there.
		out.span = from.span
	}
	switch {
	case to.span > 0:
		if from.span == 0 {
			out.span = to.span
		}
	case to.line != 0 && out.definite:
		end := to.line - 1
		if end < out.start {
			out.start, end = end, out.start
		}
		if n := end - out.start; n > 0 {
			out.span = n
		}
	case to.line != 0:
		// An end line with no start: the item ends there and takes the span it
		// asked for, so it begins that many tracks earlier.
		out.start, out.definite = to.line-1-out.span, true
	}
	if out.span < 1 || out.span > maxRepeatedTracks {
		return gridPlacement{}, false
	}
	return out, true
}

// lineValue reads one end of a placement: a line number, a span, a name, or
// nothing.
//
// The number is returned as written — one-based, with nought meaning "not
// given" — because that is the only way to tell "auto" from a line, and CSS has
// no line zero to be confused with it. A name is the one-based line resolve
// finds for it; a name it does not find is refused. The keywords are matched
// without regard to case and a name with it: a name is a <custom-ident>, and
// the areas it is matched against were written with theirs.
func lineValue(raw string, resolve func(string) (int, bool)) (gridLine, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "auto") {
		return gridLine{}, true
	}
	if len(value) >= 4 && strings.EqualFold(value[:4], "span") &&
		(len(value) == 4 || value[4] == ' ' || value[4] == '\t' || value[4] == '\n') {
		rest := strings.TrimSpace(value[4:])
		if rest == "" {
			// "span" on its own is "span 1".
			return gridLine{span: 1}, true
		}
		n, ok := positiveNumber(rest)
		return gridLine{span: n}, ok
	}
	if n, ok := positiveNumber(value); ok {
		return gridLine{line: n}, true
	}
	if isAreaName(value) {
		n, ok := resolve(value)
		return gridLine{line: n}, ok
	}
	return gridLine{}, false
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

// placeItems is §8.5's automatic placement, step by step.
//
// Step 1 puts the items that named both their lines where they asked, whatever
// else is there. Step 2 puts the items locked to a row, in order, at the first
// column where they overlap nothing — and, packed sparsely, past whatever this
// step has already put in that row, so that two of them keep the order they
// were written in. That column may be past the last one there is: step 3's
// implicit grid is counted from where step 2 put them, so a row asked for by
// more items than it has columns grows columns rather than piling the extra
// items on the first. Step 4 deals everything else from a cursor that walks
// along the columns and then down, and that cursor never goes back — which is
// what "sparse" packing means, and what leaves the holes that "dense" would go
// back for.
//
// It returns how many rows and columns the items reach, and whether an item had
// to be clamped to maxGridTracks to fit — see clamp.
func placeItems(items []*gridItem, columns int, dense bool) (rows, cols int, clamped bool) {
	grid := &gridOccupancy{columns: columns, runs: make([][]takenRows, columns)}
	var locked, flow []*gridItem
	for _, it := range items {
		switch {
		case it.place[0].definite && it.place[1].definite:
			it.row, it.column = it.place[0].start, it.place[1].start
			grid.fill(it)
		case it.place[0].definite:
			locked = append(locked, it)
		default:
			flow = append(flow, it)
		}
	}
	// Step 2. after is, per row, the column past the last item this step put
	// in that row; dense packing has no use for it and starts every item at
	// the first column.
	//
	// first is, per row, a column before which every cell of that row is
	// taken. No window that starts before it can be free, whatever its spans,
	// because it includes a taken cell of the row it starts in; and cells are
	// never freed, so it only moves forward. Without it, a thousand items
	// locked to one row and packed densely each walked every column the ones
	// before them had filled — the items squared, where a row may grow to
	// maxGridTracks columns.
	after, first := map[int]int{}, map[int]int{}
	for _, it := range locked {
		it.row = it.place[0].start
		from := first[it.row]
		for from < grid.columns {
			if _, taken := grid.blockedIn(from, it.row, 1); !taken {
				break
			}
			from++
		}
		first[it.row] = from
		if !dense {
			from = max(from, after[it.row])
		}
		it.column = grid.freeInRow(it.row, from, it.place[0].span, it.place[1].span)
		grid.fill(it)
		if !dense {
			after[it.row] = it.column + it.place[1].span
		}
	}
	// Step 4. The cursor is kept as the place just past the last item the step
	// put down, which is the specification's cursor — the last item's own
	// corner — with the cells that item covers already stepped over: every
	// window that begins inside them overlaps it.
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
			// A definite column and no row: the item drops down its column
			// until it finds a row with room for it. Packed sparsely it starts
			// from the cursor's row, and from the row after that if its column
			// is behind the cursor — the specification's "if this is less than
			// the previous column position of the cursor, increment the row
			// position by 1". Neither was done: the cursor did not move, so an
			// item written after this one could be placed before it in the
			// same row, and one behind the cursor was put above it.
			if !dense && it.place[1].start < column {
				row++
			}
			it.column = it.place[1].start
			it.row = grid.freeInColumn(row, it.column, it.place[0].span,
				it.place[1].span)
			grid.fill(it)
		} else {
			it.row, it.column = grid.next(row, column, it.place[0].span, it.place[1].span)
			grid.fill(it)
		}
		row, column = it.row, it.column+it.place[1].span
		if column >= grid.columns {
			row, column = row+1, 0
		}
	}
	return grid.rows, grid.columns, grid.clamped
}

// gridOccupancy is which cells are taken, which is all §8.5 needs to remember.
//
// It grows downwards, and sideways only in §8.5's step 2: the number of columns
// is settled before the flow deals anything — the template and the items that
// named their columns say how many there are — but an item locked to a row
// that has no room left in it makes the columns it needs.
//
// It is kept per column, as the runs of rows the items in that column cover,
// and not as a table of cells. A table of cells is rows times columns, and
// neither is bounded by the document: items of "span 1000" on both axes stack
// a thousand rows apart, and eighty of them made a table of eighty million
// cells, 20 KB of markup allocated 5.7 GB. The runs are as many as the items
// that made them, fewer where items stack, and every question the placement
// asks — is this window free, where does the next one begin — is answered by
// looking a run up rather than by visiting the cells.
//
// bordercollapse.go's line runs and tablelayout.go's "until" are the same idea
// on a table: nothing about a grid may be proportional to its area.
type gridOccupancy struct {
	columns int
	rows    int
	// runs is, per column, the rows taken, as sorted runs that neither overlap
	// nor touch.
	runs [][]takenRows
	// clamped says an item was moved to fit inside maxGridTracks.
	clamped bool
}

// takenRows is the rows from one to before another in one column.
type takenRows struct{ from, to int }

// blockedIn is the run of one column that meets rows [row, row+span), if any.
// A column past the last one there is holds nothing.
func (g *gridOccupancy) blockedIn(column, row, span int) (takenRows, bool) {
	if column >= len(g.runs) {
		return takenRows{}, false
	}
	runs := g.runs[column]
	i, _ := slices.BinarySearchFunc(runs, row, func(r takenRows, row int) int {
		if r.to <= row {
			return -1
		}
		return 1
	})
	if i < len(runs) && runs[i].from < row+span {
		return runs[i], true
	}
	return takenRows{}, false
}

// blocked is the first column of a window whose cells are not all free, and the
// run in it that is in the way.
func (g *gridOccupancy) blocked(row, column, rowSpan, columnSpan int) (int, takenRows, bool) {
	for c := column; c < column+columnSpan; c++ {
		if run, ok := g.blockedIn(c, row, rowSpan); ok {
			return c, run, true
		}
	}
	return 0, takenRows{}, false
}

// clamp is §7.1's limit on the implicit grid, on both axes: an item whose area
// would reach past maxGridTracks tracks has its span cut at the last line, and
// one that would begin past it is put in the last track with a span of one. The
// same numbers a browser clamps to, for the same reason — the grid is as big
// as its items say, and its items are untrusted. Rows are where the flow
// reaches; columns are where step 2 does, a row asked for by more items than
// it has room for.
func (g *gridOccupancy) clamp(it *gridItem) {
	clampAxis := func(start, span *int) {
		if *start+*span <= maxGridTracks {
			return
		}
		g.clamped = true
		if *start >= maxGridTracks {
			*start, *span = maxGridTracks-1, 1
			return
		}
		*span = maxGridTracks - *start
	}
	clampAxis(&it.row, &it.place[0].span)
	clampAxis(&it.column, &it.place[1].span)
}

func (g *gridOccupancy) fill(it *gridItem) {
	g.clamp(it)
	end := it.row + it.place[0].span
	if end > g.rows {
		g.rows = end
	}
	if reach := it.column + it.place[1].span; reach > g.columns {
		g.runs = append(g.runs, make([][]takenRows, reach-g.columns)...)
		g.columns = reach
	}
	for c := max(it.column, 0); c < it.column+it.place[1].span && c < g.columns; c++ {
		g.runs[c] = addRun(g.runs[c], takenRows{from: it.row, to: end})
	}
}

// addRun puts one more run into a column's sorted list, merging it with any it
// overlaps or touches.
func addRun(runs []takenRows, add takenRows) []takenRows {
	// The first run that ends at or after the new one begins, and the first
	// that begins after it ends: everything between merges with it.
	i, _ := slices.BinarySearchFunc(runs, add.from, func(r takenRows, from int) int {
		if r.to < from {
			return -1
		}
		return 1
	})
	j := i
	for j < len(runs) && runs[j].from <= add.to {
		add.from = min(add.from, runs[j].from)
		add.to = max(add.to, runs[j].to)
		j++
	}
	return slices.Replace(runs, i, j, add)
}

// freeInRow is the first column at or after from where an item locked to a row
// fits without overlapping anything, across every row it spans.
//
// There always is one: past the last column there is, nothing is taken. It
// asked only about the item's first row, so an item locked to "1 / span 2" was
// put over one below it in the second, and when a row had no room it answered
// the first column, so the item was put over whatever was there.
func (g *gridOccupancy) freeInRow(row, from, rowSpan, span int) int {
	for c := from; ; {
		if c >= g.columns {
			return c
		}
		k, _, blocked := g.blocked(row, c, rowSpan, span)
		if !blocked {
			return c
		}
		// Every window that includes column k is blocked in these rows by the
		// same run, so the next one worth asking about begins after it.
		c = k + 1
	}
}

// freeInColumn is the first row at or after one where a span will fit in a
// given column.
func (g *gridOccupancy) freeInColumn(from, column, rowSpan, columnSpan int) int {
	if column < 0 || column+columnSpan > g.columns {
		// A window that does not fit across the grid is never free. The
		// placement cannot ask for one — the columns are counted from the
		// items — and the answer the dense table gave, the first row past
		// everything, is kept.
		return max(from, g.rows+1)
	}
	for r := from; ; {
		_, run, blocked := g.blocked(r, column, rowSpan, columnSpan)
		if !blocked {
			return r
		}
		// No window starting before the run ends can be free.
		r = run.to
	}
}

// next is where the cursor finds room for an item, walking along the columns
// and then down.
//
// It asks about windows rather than cells. A window blocked at some column is
// blocked for every start that includes that column, so the walk along a row
// skips past it; and a row in which every window is blocked stays so until the
// earliest of the runs that blocked them ends, so the walk down skips to there.
// Past the last row any item reaches every window is free, so the walk ends.
func (g *gridOccupancy) next(row, column, rowSpan, columnSpan int) (int, int) {
	if columnSpan > g.columns {
		return max(row, g.rows+1), 0
	}
	for r := row; ; {
		start := 0
		if r == row {
			start = column
		}
		// The cursor's own row is walked from the cursor, so the windows before
		// it were not asked about, and the rows below it are asked in full
		// before anything is skipped.
		partial := start > 0
		resume := -1
		for c := start; c+columnSpan <= g.columns; {
			k, run, blocked := g.blocked(r, c, rowSpan, columnSpan)
			if !blocked {
				return r, c
			}
			if resume < 0 || run.to < resume {
				resume = run.to
			}
			c = k + 1
		}
		if resume < 0 || partial {
			// No window to try in this row at all, the cursor having started
			// past the last one that fits; or not every window was tried.
			resume = r + 1
		}
		r = max(resume, r+1)
	}
}

// tracksNeeded is how many tracks on one axis the items ask for, which is the
// explicit grid unless one of them named a line past its end or asked for a
// span wider than it.
func tracksNeeded(items []*gridItem, axis, explicit int) int {
	out := explicit
	for _, it := range items {
		want := it.place[axis].span
		if it.place[axis].definite {
			want = it.place[axis].start + it.place[axis].span
		}
		if want > out {
			out = want
		}
	}
	return out
}

// roomBefore is how many implicit tracks one axis needs before its explicit
// grid — as many as the item reaching furthest back asks for, which is only
// ever an item that ends at a line and spans back past the first — and moves
// every line an item named along by that many, so that they count from the
// first track there will be.
func roomBefore(items []*gridItem, axis int) int {
	lead := 0
	for _, it := range items {
		if p := it.place[axis]; p.definite && -p.start > lead {
			lead = -p.start
		}
	}
	for _, it := range items {
		if it.place[axis].definite {
			it.place[axis].start += lead
		}
	}
	return lead
}

// withLeadingTracks puts lead implicit tracks in front of the explicit ones,
// and says where the explicit grid now ends and where an auto-fit repetition
// now is.
//
// §7.6 sizes them from grid-auto-rows or grid-auto-columns counted backwards:
// the last implicit track before the explicit grid takes the last size in the
// list, the one before it the one before that, and so on round.
func withLeadingTracks(explicit, sizes []gridTrack, lead int,
	fit autoFit) ([]gridTrack, int, autoFit) {

	if lead == 0 {
		return explicit, len(explicit), fit
	}
	out := make([]gridTrack, 0, lead+len(explicit))
	for j := lead; j >= 1; j-- {
		n := len(sizes)
		out = append(out, sizes[((n-j)%n+n)%n])
	}
	out = append(out, explicit...)
	if fit.from < fit.to {
		fit.from, fit.to = fit.from+lead, fit.to+lead
	}
	return out, len(out), fit
}

// transposeItems turns the items on their side: what they said about their rows
// they now say about their columns, and where they end up is read back the same
// way.
//
// It is how a column flow is placed by the row-flow algorithm. §8.5 is one
// algorithm written about one axis, and the specification says as much — "if
// grid-auto-flow is column, swap all rows and columns in the above" — so
// swapping is the honest way to say it here too.
func transposeItems(items []*gridItem) {
	for _, it := range items {
		it.place[0], it.place[1] = it.place[1], it.place[0]
		it.row, it.column = it.column, it.row
	}
}

// gridFlow is §8.5's grid-auto-flow: which axis the items are dealt along, and
// whether the cursor may go back for a hole it left behind.
type gridFlow struct{ column, dense bool }

// autoFlow reads the property, whose two halves are written in either order and
// either alone.
func (l *layouter) autoFlow(b *Box) gridFlow {
	var out gridFlow
	for _, word := range strings.Fields(trimmedLower(b.Style.Get("grid-auto-flow"))) {
		switch word {
		case "column":
			out.column = true
		case "dense":
			out.dense = true
		}
	}
	return out
}

// implicitTrack is the size the next track outside the explicit grid takes: the
// list of implicit sizes, in turn, counted from the first track nobody drew.
//
// Counted from there and not from the top of the grid, because a drawn track is
// the size it was drawn at and the list has nothing to say about it — so a grid
// with one row of its own and "grid-auto-rows: 50px 30px" makes its second row
// fifty and its third thirty.
func implicitTrack(sizes []gridTrack, made, explicit int) gridTrack {
	return sizes[(made-explicit)%len(sizes)]
}

// implicitTracks reads grid-auto-rows or grid-auto-columns, which is §7.5's
// list of sizes the tracks outside the explicit grid take, one after another
// and starting again at the end.
//
// "auto" is the initial value and the commonest one, and it is a list of one:
// every implicit track is as big as what lands in it. A list of two is how a
// grid gives its rows alternating heights without drawing any of them.
func (l *layouter) implicitTracks(b *Box, property string, width style.Unit) []gridTrack {
	tracks, _, ok := l.trackList(b, property, width, trackRoom{})
	if !ok || len(tracks) == 0 {
		return []gridTrack{autoTrack()}
	}
	return tracks
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

// areaSpan is how far an item's area reaches on one axis: the band of tracks
// it spans, and the space content distribution added between them. §11.1 says
// that space enlarges the gutters, and the gutters inside a span are the
// item's; the one before its first track and after its last are not.
func areaSpan(tracks []gridTrack, from, span int, gap, between style.Unit) style.Unit {
	return trackSpan(tracks, from, span, gap).Add(between.Mul(float64(max(span, 1) - 1)))
}

// trackEdges is where each track of one axis begins, less the gaps: the sizes
// of the tracks before it, added up once for the axis. Adding them up again for
// every item that asked, which is what this was, cost the tracks times the
// items.
type trackEdges []style.Unit

func trackEdgesOf(tracks []gridTrack) trackEdges {
	out := make(trackEdges, len(tracks)+1)
	for i, t := range tracks {
		out[i+1] = out[i].Add(t.base)
	}
	return out
}

// start is how far along the axis a track begins: everything before it and the
// gaps between.
func (e trackEdges) start(at int, gap style.Unit) style.Unit {
	return gap.Mul(float64(at)).Add(e[min(max(at, 0), len(e)-1)])
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
func (l *layouter) gridAlignment(b *Box, property string, a flexAxis) flexAlign {
	return crossAlignment(gridAlignmentValue(b.Style.Get(property), a), flexAxis{})
}

func (l *layouter) itemAlignment(it *gridItem, property string, container flexAlign,
	a flexAxis) flexAlign {

	value := gridAlignmentValue(it.box.Style.Get(property), a)
	if value == "" || value == "auto" {
		return container
	}
	return crossAlignment(value, flexAxis{})
}

// gridAlignmentValue is the value with the two spellings that mean nothing here
// taken out: "legacy" is justify-items' initial value and is a rule about
// inheriting a text-align this engine never sets, and "auto" on a *-self
// property means "the container's", which the caller answers.
func gridAlignmentValue(raw string, a flexAxis) string {
	value := trimmedLower(raw)
	switch value {
	case "legacy", "normal":
		return ""
	case "left", "right":
		// The physical pair, which names the same two ends as the logical one
		// until the columns run the other way: "left" is where the tracks start
		// in a left-to-right grid and where they end in a right-to-left one.
		if (value == "left") == a.rtl {
			return "end"
		}
		return "start"
	}
	return value
}

// gridContentAlignment reports whether the tracks on an axis are stretched,
// which is the one thing the *sizing* needs to know about alignment: §12.8
// gives the space left over to the automatic tracks, and every other value
// leaves it for §10.3 to place the tracks in.
func (l *layouter) gridContentAlignment(b *Box, property string) bool {
	switch trimmedLower(b.Style.Get(property)) {
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
func (l *layouter) trackSpacing(b *Box, property string, a flexAxis,
	tracks []gridTrack, room, gap style.Unit) (lead, between style.Unit) {

	free := room.Sub(gridInner(tracks, gap))
	if free == 0 || len(tracks) == 0 {
		return 0, 0
	}
	align := justifyStart
	switch property {
	case "align-content":
		align = l.alignContentOf(b, a)
	default:
		align = l.justifyOf(b, a)
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

// tracksDefinite reports whether the span of tracks an item covers has a size
// that does not depend on what is in it, which is what makes a percentage
// against it resolvable.
//
// A track written as a length is definite; "auto" and a flexible track are what
// the items in them come to, so a percentage of one would be a percentage of
// itself. The container having a definite size on the axis answers it too, and
// the caller asks that first.
func tracksDefinite(tracks []gridTrack, from, span int) bool {
	if span < 1 {
		span = 1
	}
	for i := from; i < from+span; i++ {
		if i < 0 || i >= len(tracks) {
			return false
		}
		if tracks[i].min.kind != trackFixed || tracks[i].max.kind != trackFixed {
			return false
		}
	}
	return true
}

// collapseUnusedTracks drops the tracks of an auto-fit repetition that no item
// occupies, and moves the items onto what is left.
//
// A collapsed track has no size and no gap beside it, so dropping it is what
// collapsing comes to. Every track *inside* an item's span is occupied by that
// item and so is never dropped, which is what keeps each item's tracks
// contiguous after the renumbering and lets the spans stand unchanged.
//
// At least one track is always kept: every item occupies one, and a container
// with no items never reaches here.
func collapseUnusedTracks(tracks []gridTrack, items []*gridItem, axis int,
	fit autoFit) []gridTrack {

	if fit.from >= fit.to {
		return tracks
	}
	used := make([]bool, len(tracks))
	for i := range used {
		// Outside the repetition every track stands, used or not.
		used[i] = i < fit.from || i >= fit.to
	}
	for _, it := range items {
		from, span := it.column, it.place[1].span
		if axis == 0 {
			from, span = it.row, it.place[0].span
		}
		if span < 1 {
			span = 1
		}
		for i := from; i < from+span && i < len(used); i++ {
			if i >= 0 {
				used[i] = true
			}
		}
	}
	renumbered := make([]int, len(tracks))
	out := make([]gridTrack, 0, len(tracks))
	for i, t := range tracks {
		renumbered[i] = len(out)
		if used[i] {
			out = append(out, t)
		}
	}
	if len(out) == len(tracks) {
		return tracks
	}
	if len(out) == 0 {
		return tracks[:1]
	}
	for _, it := range items {
		if axis == 0 {
			if it.row >= 0 && it.row < len(renumbered) {
				it.row = renumbered[it.row]
			}
			continue
		}
		if it.column >= 0 && it.column < len(renumbered) {
			it.column = renumbered[it.column]
		}
	}
	return out
}

// gridStretch is a stretched item's width, as the margin-box size the placement
// works in: its area's, held between the item's own min-width and max-width.
//
// §11's stretch is "as for width: auto", and an automatic size is one the §10.4
// limits hold. The area was used as it stood, so an item at "max-width: 50px"
// in a 400px column was 400 wide and one at "min-width: 100px" in a 40px column
// was 40 (audit C106). The limits resolve against the area, as the item's own
// width does — see gridDeclaredSize, which reads them.
//
// Only the width. A stretched height is handed to the item's own layout, which
// holds it between min-height and max-height itself; the same clamp written
// for it changed nothing when it was planted out.
func (l *layouter) gridStretch(it *gridItem, area style.Unit) style.Unit {
	size := area
	if hi, ok := l.gridDeclaredSize(it, "max-width", area, true); ok {
		size = style.Min(size, hi)
	}
	if lo, ok := l.gridDeclaredSize(it, "min-width", area, true); ok {
		size = style.Max(size, lo)
	}
	return size
}

// gridDeclaredSize is the size an item states for one axis, as the margin-box
// size the placement works in, and whether it stated one at all.
//
// The percentage resolves against the item's grid area, which §6.6 makes the
// containing block for an item's percentages — so "width: 50%" in a
// two-hundred-pixel cell is a hundred pixels rather than the two hundred it
// came out as, and a percentage of an area with no definite size is not a size
// at all.
func (l *layouter) gridDeclaredSize(it *gridItem, property string, area style.Unit,
	definite bool) (style.Unit, bool) {

	length, ok := l.parseLength(it.box, property)
	if !ok || length.Kind == style.LengthAuto {
		return 0, false
	}
	v, ok := length.Resolve(area, definite)
	if !ok {
		return 0, false
	}
	horizontal, vertical := l.sizingInset(it.box, area)
	inset, around := horizontal, it.horizontal()
	if property == "height" {
		inset, around = vertical, it.vertical()
	}
	return maxZero(v.Sub(inset)).Add(around), true
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
		maxZero(width.Sub(gap.Mul(float64(len(columns)-1)))), true, stretch, 0, style.MaxUnit)
}

// gridContentWidths is a grid container's min-content and max-content widths,
// which is Grid §12's answer to CSS Sizing's question: "the sum of the grid
// container's track sizes (including gutters) in the appropriate axis, when
// the grid is sized under a min-content constraint" — or a max-content one.
//
// It is the track sizing algorithm run twice with no width to fill, which is
// what a constraint is: §12.6 says the free space is zero under a min-content
// constraint and infinite under a max-content one, and §12.7 that the flexible
// tracks take no share of the first and are sized from their items in the
// second. The placement is gridTracks', the same one layout makes, so the
// columns measured are the columns drawn.
//
// Before this a grid was measured as a stack of its items, so a float holding
// "grid-template-columns: auto auto" was as wide as its wider cell and its
// second column hung outside it.
//
// The container's width is not known — it is what is being asked — so a
// percentage track is "auto" here, which is §7.2.1's rule for a percentage of a
// size that depends on the tracks. A container that states a width as a length
// does know it, and a percentage or an automatic repetition is counted against
// that. Its percentage gaps are nothing, which is Box Alignment §8.3's rule for
// the same case.
func (l *layouter) gridContentWidths(b *Box) intrinsicWidths {
	areas, _ := l.areasOf(b)
	items := l.gridItems(b, 0, areas)
	if len(items) == 0 {
		return intrinsicWidths{}
	}
	width, known := l.intrinsicLength(b, "width")
	columnGap := l.gridGap(b, "column-gap", width, known)
	height, definite := l.explicitHeight(b, 0, 0, false)
	rowGap := l.gridGap(b, "row-gap", height, definite)
	columns, _ := l.gridTracks(b, items, areas, 0,
		trackRoom{size: width, definite: known, gap: columnGap, property: "grid-template-columns"},
		trackRoom{size: height, definite: definite, gap: rowGap, property: "grid-template-rows"})

	asks := make([]trackAsk, 0, len(items))
	for _, it := range items {
		min, max := l.gridItemWidths(it)
		asks = append(asks, trackAsk{
			from: it.column, span: it.place[1].span, min: min, max: max,
		})
	}
	gaps := columnGap.Mul(float64(len(columns) - 1))

	// Under a min-content constraint: the base sizes, and nothing more. There is
	// no free space for §12.6 to hand out and the flex fraction is zero.
	narrow := slices.Clone(columns)
	trackBasesAndLimits(narrow, asks, columnGap, false)
	var out intrinsicWidths
	out.min = sumTracks(narrow).Add(gaps)

	// Under a max-content constraint: every track that is not flexible grows to
	// its growth limit, and the flexible ones take the fraction §12.7.1 finds.
	wide := slices.Clone(columns)
	growToLimitsUnbounded(wide, trackBasesAndLimits(wide, asks, columnGap, true))
	expandFlexibleTracksUnbounded(wide, asks, columnGap)
	out.max = style.Max(sumTracks(wide).Add(gaps), out.min)
	return out
}

// expandFlexibleTracksUnbounded is §12.7 where the free space is an indefinite
// length, which is what it is under a max-content constraint.
//
// There is no leftover to divide, so the size of one "fr" is found from what
// the tracks and their items ask for instead, and it is the largest of those
// asks: each flexible track's base size per unit of its factor — a factor
// below one counts as one, so a small factor is not an invitation to grow —
// and, for each item that crosses a flexible track, the fr that would let its
// tracks hold its max-content contribution. Every flexible track is then that
// many of its factor, and never less than its base.
//
// That is what makes "1fr 1fr" come out as two equal columns each as wide as
// the wider item, rather than each as wide as its own: the fr is shared. It is
// the same for the rows of a grid whose height is its content's, which is the
// other place the free space is indefinite.
//
// It reports whether there were any flexible tracks.
func expandFlexibleTracksUnbounded(tracks []gridTrack, asks []trackAsk, gap style.Unit) bool {
	fr := style.Unit(0)
	flexible := false
	for _, t := range tracks {
		if !t.flexible() {
			continue
		}
		flexible = true
		if t.max.factor > 1 {
			fr = style.Max(fr, t.base.Div(t.max.factor))
		} else {
			fr = style.Max(fr, t.base)
		}
	}
	if !flexible {
		return false
	}
	for _, a := range asks {
		span := max(a.span, 1)
		if a.from < 0 || a.from+span > len(tracks) {
			continue
		}
		crosses := false
		for i := a.from; i < a.from+span; i++ {
			if tracks[i].flexible() {
				crosses = true
				break
			}
		}
		if crosses {
			one, _ := frSize(tracks[a.from:a.from+span], gap, a.max)
			fr = style.Max(fr, one)
		}
	}
	for i := range tracks {
		if !tracks[i].flexible() {
			continue
		}
		if share := fr.Mul(tracks[i].max.factor); share > tracks[i].base {
			tracks[i].base = share
		}
	}
	return true
}

// frSize is §12.7.1's "find the size of an fr": how big one fr has to be for
// these tracks, and the gaps between them, to fill space.
//
// The fixed tracks take their base sizes first and the flexible ones share what
// is left in proportion to their factors, whose sum counts as one where it is
// less. A flexible track whose share would be smaller than its own base is not
// going to shrink to it — its base is a floor — so it is treated as fixed and
// the share worked out again without it. Every pass either returns or fixes at
// least one more track, so the passes are bounded by how many there are.
//
// It also returns the factors of the tracks still sharing at the end, which is
// what says whether the leftover was spent: all of it when they add to one or
// more, and only that fraction of it when they add to less.
func frSize(tracks []gridTrack, gap, space style.Unit) (style.Unit, float64) {
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

// gridItemWidths is what one item asks of its column: its min-content and
// max-content contributions, which are its content's unless it states a width
// of its own — a box that did is that wide however its words would break — and
// which its own min-width and max-width hold between them either way.
//
// What the item declares is contributionWidths' question, which is the one
// every other parent measuring a child asks. This read the declared width
// alone, so an item at "max-width: 50px" asked its column for its whole line
// and one at "min-width: 100px" for less than it would take.
func (l *layouter) gridItemWidths(it *gridItem) (style.Unit, style.Unit) {
	got := l.contributionWidths(it.box)
	return got.min.Add(it.horizontal()), got.max.Add(it.horizontal())
}

// sizeRows is the same on the block axis, with one difference that is not a
// difference in the algorithm: a row's content size is not asked of the box
// tree but of the layout, because how tall an item is depends on how wide it
// was made. There is no second number for an item to grow towards — a block is
// as tall as it is at the width it was given — so an item's min-content and
// max-content contributions are the same, and a row's two numbers differ only
// where its own sizing functions make them: "minmax(10px, 100px)" is a base of
// ten and a limit of a hundred whatever is in it.
//
// lo and hi are the container's min-height and max-height as content heights,
// which an auto-height grid's flexible rows are held between — see
// resolveTracks.
func (l *layouter) sizeRows(rows []gridTrack, items []*gridItem,
	height style.Unit, definite bool, gap style.Unit, stretch bool, lo, hi style.Unit) {

	asks := make([]trackAsk, 0, len(items))
	for _, it := range items {
		asks = append(asks, trackAsk{
			from: it.row, span: it.place[0].span, min: it.height, max: it.height,
		})
	}
	gaps := gap.Mul(float64(len(rows) - 1))
	l.resolveTracks(rows, asks, gap, maxZero(height.Sub(gaps)), definite, stretch,
		maxZero(lo.Sub(gaps)), maxZero(hi.Sub(gaps)))
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
//
// A room that is not definite — the rows of a grid whose height is its
// content's — is §12.6's max-content constraint, under which the free space is
// infinite: every track that is not flexible grows all the way to its growth
// limit, and §12.7 finds the size of an fr from what the flexible tracks and
// the items crossing them ask for rather than from a leftover there is none of.
// It returned before either, so a row written "minmax(10px, 100px)" was 10px
// high whatever it held, and "1fr 1fr" rows were each as tall as their own
// content rather than sharing one fr (audit C104).
//
// lo and hi are the container's own min and max size on the axis, less the
// gaps, and only an indefinite room reads them: §12.7 sizes the fr again,
// against the limit, when the fraction it found would make the grid smaller
// than the one or larger than the other.
func (l *layouter) resolveTracks(tracks []gridTrack, asks []trackAsk,
	gap, room style.Unit, definite, stretch bool, lo, hi style.Unit) {

	limits := trackBasesAndLimits(tracks, asks, gap, false)
	if !definite {
		growToLimitsUnbounded(tracks, limits)
		before := make([]style.Unit, len(tracks))
		for i := range tracks {
			before[i] = tracks[i].base
		}
		if !expandFlexibleTracksUnbounded(tracks, asks, gap) {
			return
		}
		sum := sumTracks(tracks)
		if sum <= hi && sum >= lo {
			return
		}
		// The fraction the items asked for makes the grid bigger than its
		// max-height or smaller than its min-height, so it is found again as
		// though the free space were definite and the room were the limit.
		for i := range tracks {
			tracks[i].base = before[i]
		}
		expandFlexibleTracks(tracks, style.Clamp(sum, lo, hi))
		return
	}
	free := room.Sub(sumTracks(tracks))
	if free <= 0 {
		return
	}
	free = growToLimits(tracks, limits, free)
	if flexible, spent := expandFlexibleTracks(tracks, room); flexible {
		if spent {
			return
		}
		// Flexible factors adding to less than one leave the rest of the
		// leftover unspent, and §12.8 gives what is still over to the
		// automatic tracks like any other free space. Only then: factors of
		// one or more spend all of it, and what a division leaves over is a
		// rounding remainder, not room.
		free = room.Sub(sumTracks(tracks))
	}
	if stretch {
		stretchAutoTracks(tracks, free)
	}
}

// growToLimitsUnbounded is §12.6 with infinite free space: every track that is
// not flexible is its growth limit, where that is bigger than its base.
func growToLimitsUnbounded(tracks []gridTrack, limits []style.Unit) {
	for i := range tracks {
		if !tracks[i].flexible() && limits[i] > tracks[i].base {
			tracks[i].base = limits[i]
		}
	}
}

// trackBasesAndLimits is §12.4 and §12.5: every track's base size, written into
// it, and its growth limit, returned — the two numbers the rest of the sizing
// spends the free space between.
//
// underMax is §12.5's one clause about the constraint the grid is sized under.
// An "auto" minimum is the largest minimum its items need, except when the
// container itself is being measured at its max-content size: then it is their
// max-content contributions, because a grid asked how wide it would like to be
// is not asking how narrow its tracks can get. It matters only where nothing
// else raises the base to the same place — §12.6 grows every other track to
// its growth limit anyway — and that is a flexible track, which §12.6 does not
// grow. Without it a lone "0.5fr" column holding a line of text measured half
// as wide as the line, and the grid asked for less than its content.
func trackBasesAndLimits(tracks []gridTrack, asks []trackAsk, gap style.Unit,
	underMax bool) []style.Unit {

	// What the items spanning one track ask of it, gathered in one pass over the
	// items rather than one pass per track: asking every item about every track
	// was tracks times items, and a grid has as many of each as its markup says.
	limits := make([]style.Unit, len(tracks))
	mins := make([]style.Unit, len(tracks))
	maxes := make([]style.Unit, len(tracks))
	for _, a := range asks {
		if a.span != 1 || a.from < 0 || a.from >= len(tracks) {
			continue
		}
		mins[a.from] = style.Max(mins[a.from], a.min)
		maxes[a.from] = style.Max(maxes[a.from], a.max)
	}
	for i := range tracks {
		min, max := mins[i], maxes[i]
		auto := min
		if underMax {
			auto = max
		}
		tracks[i].base = resolveTrackSize(tracks[i].min, min, max, auto)
		limits[i] = resolveTrackSize(tracks[i].max, min, max, max)
	}
	spreadSpanningAsks(tracks, limits, asks, gap)
	return limits
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

	// In order of span, and in the items' own order within one span, which is
	// what one pass over the items per span gave — and cost the widest span
	// times the items, where sorting them once costs what they are.
	spanning := make([]trackAsk, 0, len(asks))
	for _, a := range asks {
		if a.span >= 2 {
			spanning = append(spanning, a)
		}
	}
	slices.SortStableFunc(spanning, func(x, y trackAsk) int { return x.span - y.span })
	for _, a := range spanning {
		span := a.span
		if a.from < 0 || a.from+span > len(tracks) {
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

// growToLimits is §12.6: the free space is shared equally between the tracks
// that can still take it, and a track that reaches its growth limit stops.
//
// Equally and not in proportion, which is the specification's word and is what
// brings two columns of very different content closer together than their
// content is. The loop cannot run more times than there are tracks: every pass
// either spends everything or freezes at least one track.
//
// A pass whose share rounds to nothing is the last: it gives nothing, so the
// next is the same pass again. Without that the loop ran its full count over
// the remainder a division left — a unit or two of free space among a
// thousand tracks was a million comparisons to hand out nothing.
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
		if share <= 0 {
			return free
		}
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

// expandFlexibleTracks is §12.7 where the free space is definite. It reports
// whether there were any flexible tracks, and whether they spent the whole of
// the leftover — which they do unless the factors still sharing it add to less
// than one.
//
// The clause worth naming is the one for factors adding to less than one: two
// "0.25fr" tracks between them asked for a quarter of the free space each and
// take exactly that, leaving the rest of the container empty. Without it a lone
// "0.5fr" track would fill the container, which is the same picture "1fr" gives
// and is why the two have to be told apart. It is §9.7.4b of flexbox, in the
// other specification and in the same words.
//
// The size of one fr is frSize's, over every track, and that is where a track
// whose content is wider than its share gives way: "1fr 1fr" in 400px with a
// 300px word in the first column is 300 and 100, because the first track is
// taken out of the sharing and the second has what it left. The share was
// worked out once, over every flexible track, so the second column was 200
// and the grid came to 500 in a 400px box (audit C103).
//
// A flexible track never comes out smaller than its content: the share is what
// the fraction is worth, and the base is what the words inside need.
func expandFlexibleTracks(tracks []gridTrack, room style.Unit) (flexible, spent bool) {
	for _, t := range tracks {
		if t.flexible() {
			flexible = true
			break
		}
	}
	if !flexible {
		return false, false
	}
	one, factors := frSize(tracks, 0, room)
	for i := range tracks {
		if !tracks[i].flexible() {
			continue
		}
		if share := one.Mul(tracks[i].max.factor); share > tracks[i].base {
			tracks[i].base = share
		}
	}
	return true, factors >= 1
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
	// Alone, and asked again, for the reason a flex item is: every item is laid
	// out once to size the rows and again in its cell, and an item that is a
	// grid itself does the same inside. See layOutFlexItem.
	at := aloneFlow(origin.cbHeight, origin.cbDefinite)
	at.again = true
	return outOfClamp(l, func() *Fragment {
		f, _ := l.blockIn(it.box, width, at, geom)
		return f
	})
}

// gridItems gathers the container's items in order-modified document order,
// which is where every question about placement is answered from.
func (l *layouter) gridItems(b *Box, width style.Unit, areas gridAreas) []*gridItem {
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
		it.place[0], _ = l.placementOf(c, areas, 0, "grid-row-start", "grid-row-end")
		it.place[1], _ = l.placementOf(c, areas, 1, "grid-column-start", "grid-column-end")
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
func (l *layouter) gridGap(b *Box, property string, basis style.Unit, definite bool) style.Unit {
	length, ok := l.parseLength(b, property)
	if !ok {
		return 0
	}
	if v, ok := length.Resolve(basis, definite); ok && v >= 0 {
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
	room trackRoom) (tracks []gridTrack, fit autoFit, ok bool) {

	raw := strings.TrimSpace(b.Style.Get(property))
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil, autoFit{}, true
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
	// property is the one the list was written in, which is what a finding
	// about the repetition names.
	property string
}

// autoFit is the tracks an automatic repetition written "auto-fit" made, as
// the run [from, to) of the list; it is empty for every other list. §7.2.3.2
// collapses the ones of these no item landed in, and only these: a track the
// stylesheet wrote beside the repetition stays whether or not anything is in
// it.
type autoFit struct{ from, to int }

// tracksFrom turns a track list into tracks, or returns false for anything in
// it this slice does not size: a named line, a fit-content(), a subgrid, a
// repeat() inside a repeat(), a flexible minimum in a minmax(), or a list
// longer than maxRepeatedTracks.
func (l *layouter) tracksFrom(b *Box, vals []css.ComponentValue, width style.Unit,
	room trackRoom, mayRepeat bool) (tracks []gridTrack, fit autoFit, ok bool) {

	// The list is read in two halves because an automatic repetition cannot be
	// counted until everything else in the list has been: §7.2.3.2 fits as many
	// as will go in what is *left*, so the tracks written beside it have to be
	// sized first. before and after are the tracks either side of it, and one
	// is what it repeats.
	var before, after, one []gridTrack
	auto, fits := false, false
	for _, part := range splitValuesOnWhitespace(vals) {
		// The whole list and not only each repeat(): a list of a hundred
		// "repeat(1000, 1px)" was a hundred thousand tracks from two kilobytes
		// of CSS, each repeat() inside its bound. It is asked as the list is
		// read, so that what is made before the answer is no is one repeat()
		// past the bound and not every one the stylesheet wrote.
		if len(before)+len(one)+len(after) > maxRepeatedTracks {
			return nil, autoFit{}, false
		}
		if len(part) != 1 {
			return nil, autoFit{}, false
		}
		v := part[0]
		if v.IsFunction() && strings.EqualFold(v.Token.Value, "repeat") {
			if !mayRepeat {
				return nil, autoFit{}, false
			}
			got, kind, ok := l.repeatedTracks(b, v.Values, width, room)
			if !ok {
				return nil, autoFit{}, false
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
				return nil, autoFit{}, false
			}
			auto, one, fits = true, got, kind == repeatFit
			continue
		}
		got, ok := l.trackFrom(b, v, room)
		if !ok {
			return nil, autoFit{}, false
		}
		if auto {
			after = append(after, got)
			continue
		}
		before = append(before, got)
	}
	if !auto {
		if len(before) > maxRepeatedTracks {
			return nil, autoFit{}, false
		}
		return before, autoFit{}, len(before) > 0
	}
	// The repetition is bounded like everything else written about the explicit
	// grid, but by the room rather than by the stylesheet: a track of 1px
	// fills a container a million pixels wide a million times, and each of
	// them was made before the count was compared with anything. One
	// repetition past the bound is a list the stylesheet wrote too long, and is
	// refused as the others are; more than that is the container being large,
	// and the repetition stops at the bound and says so.
	n := l.autoRepetitions(one, before, after, room)
	most := (maxRepeatedTracks - len(before) - len(after)) / max(len(one), 1)
	if most < 1 {
		return nil, autoFit{}, false
	}
	if n > most {
		l.rec.ReportDetail(Finding{
			Rule:   RuleLimit,
			Source: AtHTML(offsetOf(b)),
			Message: "an automatic repetition of this grid's tracks would make more " +
				"than " + strconv.Itoa(maxRepeatedTracks) + " of them; it was stopped " +
				"there, and the room past the last one is left empty",
			Path:     PathOf(b.Element),
			Property: room.property,
		})
		n = most
	}
	out := make([]gridTrack, 0, len(before)+n*len(one)+len(after))
	out = append(out, before...)
	for i := 0; i < n; i++ {
		out = append(out, one...)
	}
	if fits {
		fit = autoFit{from: len(before), to: len(out)}
	}
	return append(out, after...), fit, len(out)+len(after) > 0
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
//
// It is the bound on everything a stylesheet writes about the explicit grid: a
// repeat(), a whole track list however many repeat()s it holds, the rows and
// the columns a grid-template-areas draws, a line number and a span. A grid
// that writes more is refused and reported.
const maxRepeatedTracks = 1000

// maxGridTracks bounds how many tracks a grid has on one axis, explicit and
// implicit together.
//
// The explicit grid is bounded by what the stylesheet may write — see
// maxRepeatedTracks — and so is where an item that names its lines can reach.
// The implicit rows an item placed by the flow lands in are bounded by nothing
// the stylesheet says: each item goes below the last, and a thousand items
// spanning a thousand rows reach a million. §7.1 lets an engine clamp the
// grid, and says how: an item reaching past the limit has its span cut at the
// last line, and one beginning past it is put in the last track. Ten thousand
// is where Firefox clamps, and is far past any grid a page is laid out with.
//
// A variable so that a test can lower it and watch it fire.
var maxGridTracks = 10000

// trackFrom reads one track, which is either a size written once or a minmax()
// that writes the two ends separately.
//
// A single size is that size at both ends, and the two that are not are the two
// that mean different things there: "auto" and a flexible track are both
// "whatever the items need" at the low end. Writing them out as a minmax() here
// rather than special-casing them later is what lets the sizing ask one
// question of each end and never ask which spelling it came from.
func (l *layouter) trackFrom(b *Box, v css.ComponentValue, room trackRoom) (gridTrack, bool) {
	if v.IsFunction() && strings.EqualFold(v.Token.Value, "minmax") {
		return l.minmaxTrack(b, v.Values, room)
	}
	size, ok := l.trackSizeFrom(b, v, room)
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
	room trackRoom) (gridTrack, bool) {

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
		size, ok := l.trackSizeFrom(b, one[0][0], room)
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
//
// A percentage is of the container's size *on this track's own axis*, which is
// what room carries. It was resolved against the container's width whichever
// axis the list was for, and always as though that width were definite: so
// "grid-template-rows: 50% 50%" in a four-hundred-pixel-tall box put the second
// row at a hundred — half the width — and in a box with no height at all it
// made two rows of that same number out of ten pixels of content.
//
// A percentage of a size that is not definite is not a size. §7.2.1 says such a
// track behaves as "auto", which is the same answer this gives a row whose
// container states no height.
func (l *layouter) trackSizeFrom(b *Box, v css.ComponentValue,
	room trackRoom) (trackSize, bool) {

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
	size, ok := length.Resolve(room.size, room.definite)
	if !ok {
		return trackSize{kind: trackAuto}, true
	}
	if size < 0 {
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
			"size — a named line, fit-content(), a repeat() inside a repeat(), " +
			"two automatic repeat()s, or a flexible minimum in a minmax() — or " +
			"are more than " + strconv.Itoa(maxRepeatedTracks)
	}
	if _, _, ok := l.trackList(b, "grid-template-rows", width, trackRoom{}); !ok {
		return "its rows are written with something this engine does not size " +
			"— a named line, fit-content(), a repeat() inside a repeat(), two " +
			"automatic repeat()s, or a flexible minimum in a minmax() — or are " +
			"more than " + strconv.Itoa(maxRepeatedTracks)
	}
	areas, ok := l.areasOf(b)
	if !ok {
		return "its cells are named by a template that does not draw a grid: " +
			"either its rows are not all the same length, a name is in two " +
			"places that do not touch, or it draws more than " +
			strconv.Itoa(maxRepeatedTracks) + " rows or columns"
	}
	switch trimmedLower(b.Style.Get("grid-auto-flow")) {
	case "", "row", "column", "dense", "row dense", "dense row",
		"column dense", "dense column":
	default:
		return "its items are placed by a flow this engine does not follow"
	}
	for _, p := range [...]string{"grid-auto-rows", "grid-auto-columns"} {
		if _, _, ok := l.trackList(b, p, width, trackRoom{}); !ok {
			return "its implicit tracks are sized with something this engine " +
				"does not size"
		}
	}
	if why := refusesGridAlignment(b); why != "" {
		return why
	}
	for _, c := range b.Children {
		if c.IsText() || (c.Anonymous() && len(c.Children) == 0) || c.outOfFlow() {
			continue
		}
		_, row := l.placementOf(c, areas, 0, "grid-row-start", "grid-row-end")
		_, column := l.placementOf(c, areas, 1, "grid-column-start", "grid-column-end")
		if !row || !column {
			return "one of its items is placed by a line this engine cannot find: " +
				"a name the template does not draw as an area, a named line, a " +
				"number counted back from the end of the grid, or a span of a " +
				"named line"
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
		switch value := trimmedLower(b.Style.Get(p)); value {
		case "", "normal", "stretch", "auto", "legacy", "start", "end", "center",
			"flex-start", "flex-end", "self-start", "self-end",
			"space-between", "space-around", "space-evenly":
		case "left", "right":
			// The physical pair names a side of the *inline* axis, so it says
			// nothing about the three properties that align down the page.
			// §6.2 throws the declaration out there rather than guessing which
			// end was meant, and a container that wrote one is reported rather
			// than laid out as though it had not.
			if strings.HasPrefix(p, "align-") {
				return "its tracks or its items are aligned down the page by a " +
					"keyword that names a side across it"
			}
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
