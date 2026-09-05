package layout

import (
	"slices"
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS Flexible Box Layout 1: a row of items sized from a shared budget.
//
// # What a flex container is, in one paragraph
//
// A block stacks its children and gives each one the whole width. A flex
// container lays them along an axis and then *negotiates*: each item states a
// size it would like, the container adds them up, and whatever is left over —
// or missing — is shared out in proportions the items themselves declare. That
// negotiation is §9.7, it is the whole of the feature, and everything else in
// this file exists to hand it three numbers per item and to place what it
// returns.
//
// # What is laid out here, and what is refused
//
// One line, running left to right, its items packed and aligned by any of the
// keywords that name a position on an axis. That is "display: flex" and the
// handful of declarations that go with it in most real documents — and it is the
// slice whose arithmetic can be stated exactly.
//
// Everything else is refused with a finding and laid out as it was before this
// file existed, which is as an ordinary block. The gate is the same shape as
// writing-mode's and multicol's, and for the same reason: a box refused here is
// the page this engine drew yesterday and is *reported*, while a box laid out
// wrongly is a page that is plausible and silent. See refusesToFlex, where each
// clause is stated as a condition on the box rather than as a list of features,
// because "this container wraps its lines" is what tells the reader what to
// change.
//
// # Why the suite cannot check this
//
// It has no document that lays out a flex container. The four that name a flex
// property are floats-wrap-bfc-with-margin-*, and every one of them has a
// script in it. So the reftest count is a *regression* check here and nothing
// more — if it moves at all, something else broke — and the evidence that this
// is right is the arithmetic in layout/flex_test.go, where Courier at 20px
// makes every advance a whole number and every share of the free space an exact
// one. That is the same standard the vertical writing modes were held to, and
// for the same reason: a whole-subtree arrangement that is consistently wrong
// agrees with itself.

// flexValues is what one item declares about how it takes its share.
type flexValues struct {
	// grow and shrink are §7.1's two factors. Neither is a length: they are
	// proportions, and what they are proportions *of* differs — the free space
	// for grow, and the free space scaled by each item's base size for shrink,
	// which is what stops a small item shrinking to nothing beside a large one.
	grow, shrink float64
	// basis is the declared flex-basis, and basisAuto says the keyword was
	// "auto" — which defers to the item's own size rather than naming one.
	// basisContent is the keyword that defers to the item's *content* instead,
	// which is a third answer and not a spelling of either.
	basis        style.Unit
	basisAuto    bool
	basisContent bool
}

// flexValuesOf reads the three properties off an item.
//
// A value the grammar does not allow gives the initial one, which is what the
// cascade would have produced had the declaration been thrown out — the same
// answer floatOf and clearOf give, and for the same reason.
func (l *layouter) flexValuesOf(b *Box, room flexRoom) flexValues {
	out := flexValues{grow: 0, shrink: 1, basisAuto: true}
	if v, ok := parseNumber(trimmedLower(b.Style["flex-grow"])); ok && v >= 0 {
		out.grow = v
	}
	if v, ok := parseNumber(trimmedLower(b.Style["flex-shrink"])); ok && v >= 0 {
		out.shrink = v
	}
	switch trimmedLower(b.Style["flex-basis"]) {
	case "", "auto":
	case "content":
		// "content" is "size it as though flex-basis were auto and the main
		// size were auto", which is not the same as "auto" and differs from it
		// exactly where the item declared a main size of its own: auto defers
		// to the declaration and this one ignores it.
		out.basisContent = true
	default:
		// A percentage basis is of the line, and where the line has no size it
		// is indefinite — which leaves the basis auto, deferring to whatever
		// the item says about itself.
		if length, ok := l.parseLength(b, "flex-basis"); ok {
			if v, ok := length.Resolve(room.main, room.definite); ok && v >= 0 {
				out.basis, out.basisAuto = v, false
			}
		}
	}
	return out
}

// flexAxis is which way the main axis runs, and it is the only thing in this
// file that knows whether a main size is a width or a height.
//
// Everything §9 says is stated about a main axis and a cross axis, and it is
// true of a column exactly as it is of a row — the algorithm does not change,
// the two axes swap. Naming that swap once is what keeps the resolution in
// §9.7, the packing in §9.5 and the alignment in §9.6 written the way the
// specification writes them, instead of twice each with the words exchanged.
//
// A column's cross axis is horizontal, and a horizontal size is the one this
// engine always knows: a block's width comes from its containing block and its
// height comes from its content. That asymmetry is why a column measures its
// items before it sizes them, and it is the only place the two directions are
// not each other's mirror image.
//
// Which end each axis starts at is the other half. Three declarations move it —
// flex-direction's two reversed values, wrap-reverse, and direction: rtl — and
// they compose rather than override: a right-to-left row-reverse runs left to
// right, because reversing twice is not reversing. They are kept apart here
// rather than folded into one flag because the alignment keywords need to tell
// them apart: "flex-start" follows the flex axis, "start" follows the writing
// mode, and "left" follows neither.
type flexAxis struct {
	column bool
	// reverse is flex-direction's, rtl is direction's, and wrapReverse is
	// flex-wrap's. Each names one declaration and nothing more.
	reverse, rtl, wrapReverse bool
}

// mainReversed and crossReversed are which way each axis actually runs, once
// the declarations above have been composed.
//
// A row's main axis is the inline axis, so "rtl" reverses it and "column" is
// not affected by it at all; a row's cross axis is the block axis, which no
// writing mode this engine lays out runs backwards. A column is the same
// sentence with the two axes exchanged.
func (a flexAxis) mainReversed() bool {
	if a.column {
		return a.reverse
	}
	return a.reverse != a.rtl
}

func (a flexAxis) crossReversed() bool {
	if a.column {
		return a.wrapReverse != a.rtl
	}
	return a.wrapReverse
}

// The property that names a size along each axis. There is no table for these:
// a main size in a column is a height and the whole of the difference is which
// of six property names is read.
func (a flexAxis) mainName() string {
	if a.column {
		return "height"
	}
	return "width"
}

func (a flexAxis) crossName() string {
	if a.column {
		return "width"
	}
	return "height"
}

func (a flexAxis) minName() string { return "min-" + a.mainName() }
func (a flexAxis) maxName() string { return "max-" + a.mainName() }
func (a flexAxis) gapName() string {
	// The gap between one item and the next is *across* the axis they run
	// along: items in a row are separated by a column gap, and items stacked in
	// a column are separated by a row gap. The names are the ones a grid gave
	// them, where a column gap really does sit between two columns.
	if a.column {
		return "row-gap"
	}
	return "column-gap"
}

// The edge sums along each axis, and the two rect fields they measure.
func (a flexAxis) mainEdge(e Edges) style.Unit {
	if a.column {
		return e.Vertical()
	}
	return e.Horizontal()
}

func (a flexAxis) crossEdge(e Edges) style.Unit {
	if a.column {
		return e.Horizontal()
	}
	return e.Vertical()
}

func (a flexAxis) mainOf(r Rect) style.Unit {
	if a.column {
		return r.H
	}
	return r.W
}

func (a flexAxis) crossOf(r Rect) style.Unit {
	if a.column {
		return r.W
	}
	return r.H
}

// mainStart and crossStart are the two margin edges an item is placed from.
func (a flexAxis) mainStart(e Edges) style.Unit {
	if a.column {
		return e.Top
	}
	return e.Left
}

func (a flexAxis) crossStart(e Edges) style.Unit {
	if a.column {
		return e.Left
	}
	return e.Top
}

// The same four sides as places rather than as values, so that a filled-in
// automatic margin can be written back to the side that asked for it.
func (a flexAxis) mainStartOf(e *Edges) *style.Unit {
	if a.column {
		return &e.Top
	}
	return &e.Left
}

func (a flexAxis) mainEndOf(e *Edges) *style.Unit {
	if a.column {
		return &e.Bottom
	}
	return &e.Right
}

func (a flexAxis) crossStartOf(e *Edges) *style.Unit {
	if a.column {
		return &e.Left
	}
	return &e.Top
}

func (a flexAxis) crossEndOf(e *Edges) *style.Unit {
	if a.column {
		return &e.Right
	}
	return &e.Bottom
}

// mainEnd and crossEnd are the other side of each: the trailing physical edge.
func (a flexAxis) mainEnd(e Edges) style.Unit {
	if a.column {
		return e.Bottom
	}
	return e.Right
}

func (a flexAxis) crossEnd(e Edges) style.Unit {
	if a.column {
		return e.Right
	}
	return e.Bottom
}

// place puts a fragment at a main and cross position.
func (a flexAxis) place(f *Fragment, main, cross style.Unit) {
	if a.column {
		f.BorderRect.Y, f.BorderRect.X = main, cross
		return
	}
	f.BorderRect.X, f.BorderRect.Y = main, cross
}

// axisOf reads the three declarations that decide which way each axis runs.
func (l *layouter) axisOf(b *Box) flexAxis {
	direction := trimmedLower(b.Style["flex-direction"])
	return flexAxis{
		column:      direction == "column" || direction == "column-reverse",
		reverse:     direction == "row-reverse" || direction == "column-reverse",
		rtl:         isRTL(b),
		wrapReverse: trimmedLower(b.Style["flex-wrap"]) == "wrap-reverse",
	}
}

// mainAt and crossAt turn a position measured from an axis's start into one
// measured from the container's leading edge, which is the only place the
// direction of an axis is applied.
//
// Everything above works in one direction: the first item is at nought, the
// packing measures from there, and the free space is at the far end. Mirroring
// that answer is exactly what running the axis backwards means, and doing it
// once here is what keeps §9.3, §9.5, §9.6 and §9.7 written the way they are
// written — a line whose items were laid out backwards would have to reverse
// the order it breaks in, the end it packs from, and the edge each alignment
// measures to, all separately and all consistently.
func (a flexAxis) mainAt(pos, outer, container style.Unit) style.Unit {
	if a.mainReversed() {
		return container.Sub(pos).Sub(outer)
	}
	return pos
}

func (a flexAxis) crossAt(pos, outer, container style.Unit) style.Unit {
	if a.crossReversed() {
		return container.Sub(pos).Sub(outer)
	}
	return pos
}

// flexItem is one in-flow child of a flex container, with everything §9 needs
// to size it and everything §9.7 fills in.
type flexItem struct {
	box    *Box
	values flexValues
	// margin, border and padding are the item's own, resolved once. The sum of
	// the three is what separates an item's *content* size — which is what §9.7
	// distributes — from the room it takes on the line.
	margin, border, padding Edges
	// mainSurround is that sum along the main axis, and crossMargin is the
	// margins alone across it. Both are taken once: every line of §9.7 needs
	// one of them and neither depends on the item's size. Both are added to
	// when an automatic margin is filled in, because an auto margin that has
	// taken the free space is a margin of that size and takes that much room.
	mainSurround, crossMargin style.Unit
	// auto is which of the four margins were declared "auto", one per edge and
	// non-zero where they were. It is an Edges rather than four booleans so
	// that it can go through the same permutation the lengths do — a box inside
	// a turned subtree has its edges rearranged, and the answer has to be about
	// the same sides the margins ended up on.
	auto Edges
	// cross is the item's used cross size as a border box, and hasCross says it
	// is known. A column resolves it before anything else — an item's height
	// cannot be measured until its width is settled — while a row learns it
	// only after the items are laid out, which is what §9.4 does in that order.
	cross    style.Unit
	hasCross bool
	// measured is what the item's content came to along the main axis, which
	// only a column asks for and only a column can answer: it is a height, and
	// a height is found by laying the item out rather than by reading it.
	measured style.Unit
	// line is which of the container's lines the item landed on, which is what
	// the cross-axis half of §9.4 and §9.6 is asked about.
	line int
	// order is §5.4's, which decides where the item sits among its siblings and
	// nothing else about it.
	order int
	// base is §9.2's flex base size and hypothetical is that clamped by the
	// item's own minimum and maximum, both as content sizes.
	base, hypothetical style.Unit
	// min and max are the item's own limits on the main axis. min is not
	// always declared: §4.5 gives a flex item an automatic minimum size, which
	// is the smallest it can be without its content spilling out.
	min, max style.Unit
	// target is what §9.7 resolved, frozen says it stopped moving, and frag is
	// the item laid out at that size.
	target style.Unit
	frozen bool
	frag   *Fragment
}

// addMargin puts a filled-in automatic margin on one side of the item, and on
// the fragment if there is one yet: the used value of an auto margin is the room
// it took, and a fragment that reported zero would be describing a box that is
// not where it was put.
func (it *flexItem) addMargin(side *style.Unit, by style.Unit) {
	*side = side.Add(by)
	if it.frag != nil {
		it.frag.Margin = it.margin
	}
}

// outer is the room an item takes on the line: its content size and everything
// around it.
func (it *flexItem) outer(content style.Unit) style.Unit {
	return content.Add(it.mainSurround)
}

// outerCross is the room it takes across the line, which is its border box and
// the margins either side of it. Only the margins, because a cross size in this
// file is carried as a border box — see flexItem.cross.
func (it *flexItem) outerCross(border style.Unit) style.Unit {
	return border.Add(it.crossMargin)
}

// refusesToFlex is why a flex container is laid out as a block anyway, or the
// empty string if it is arranged.
//
// Every clause is a way for a row with the initial alignment to stop being the
// picture the document asked for, and each is stated as a condition on the box.
// The set is deliberately narrow, and narrowing is the safe direction: a box
// refused here is laid out exactly as it was before this file existed and is
// reported, which is the honest answer; a box arranged that should not have
// been is a page that is quietly wrong.
func (l *layouter) refusesToFlex(b *Box, containing style.Unit) string {
	a := l.axisOf(b)
	switch trimmedLower(b.Style["flex-direction"]) {
	case "", "row", "column", "row-reverse", "column-reverse":
	default:
		return "its main axis is not one of the four flex-direction names"
	}
	switch trimmedLower(b.Style["flex-wrap"]) {
	case "", "nowrap":
	case "wrap", "wrap-reverse":
		if a.column {
			// A wrapping column's lines are columns side by side, and each is
			// as wide as the widest item on it — but an item stretched across
			// its line is as wide as the line, which is the number being
			// worked out. A row has no such knot: an item's height does not
			// decide its own line's height by way of its width.
			return "its items wrap into columns, where how wide a line is and " +
				"how wide the items on it are decide each other"
		}
	default:
		return "its lines wrap by a rule this engine does not apply"
	}
	switch trimmedLower(b.Style["align-content"]) {
	case "", "normal", "stretch", "flex-start", "start", "flex-end", "end",
		"center", "space-between", "space-around", "space-evenly":
	default:
		return "its lines are placed by a rule this engine does not apply"
	}
	switch trimmedLower(b.Style["justify-content"]) {
	case "", "normal", "flex-start", "start", "left", "flex-end", "end", "right",
		"center", "space-between", "space-around", "space-evenly":
	default:
		// The two this does not do are §6.2's overflow alignment — "safe" and
		// "unsafe" — which are a second answer to what happens when the line is
		// over-full, and the baseline set, which aligns along an axis this is
		// not about.
		return "its items are packed by a rule this engine does not apply"
	}
	if why := refusesAlignment(trimmedLower(b.Style["align-items"]), a); why != "" {
		return "its items are " + why
	}
	items, outOfFlow := 0, false
	for _, c := range b.Children {
		if c.IsText() || (c.Anonymous() && len(c.Children) == 0) {
			// What is left of a text child once §4's anonymous item has been
			// made is white space that collapses to nothing, which is not
			// content and not an item. See wrapFlexText in box.go.
			continue
		}
		if c.outOfFlow() {
			// Noted rather than refused on the spot. §4.1 takes such a box out
			// of the flow and gives it a static position from the container's
			// own alignment — which is a different position from the block
			// stacking's only where there is an arrangement to differ from. A
			// container whose only children are out of flow has no items, and
			// an empty box is the same box laid out either way.
			outOfFlow = true
			continue
		}
		items++
		if why := refusesAlignment(trimmedLower(c.Style["align-self"]), a); why != "" {
			return "one of its items is " + why
		}
	}
	if outOfFlow && items > 0 {
		return "it holds a floated or absolutely positioned box beside its " +
			"items, and that box is placed against the container rather than " +
			"taking a place in the row"
	}
	_ = containing
	return ""
}

// refusesAlignment is why an item cannot be aligned across the line the way it
// asked to be, or the empty string if it can.
//
// Two of §6.2's keywords are missing here and one of them only sometimes. "last
// baseline" aligns the *bottom* line of an item's text, which is a second
// baseline this engine does not find; the overflow keywords "safe" and "unsafe"
// are a second answer to what happens when an item does not fit, and this file
// has one already.
//
// "baseline" is refused across a reversed cross axis, and the reason is the
// mirror in crossAt. Every other alignment here names an end, and a mirrored end
// is the other end; a baseline is a line *inside* each item, at a different
// depth in each, so mirroring moves every item by its own depth and the
// baselines that met before it do not meet after. A column is refused for a
// plainer reason: its cross axis is horizontal, and the baseline of a box across
// its inline axis is not a thing this engine finds at all.
func refusesAlignment(value string, a flexAxis) string {
	switch value {
	case "", "auto", "normal", "stretch", "flex-start", "start", "self-start",
		"flex-end", "end", "self-end", "center":
		return ""
	case "baseline", "first baseline":
		if a.column {
			return "aligned to a shared baseline down a column, where a " +
				"baseline runs across the axis they would be aligned on"
		}
		if a.crossReversed() {
			return "aligned to a shared baseline on lines that stack from the " +
				"far side, where mirroring the line would move each item by " +
				"its own depth and part the baselines again"
		}
		return ""
	}
	return "aligned across the line by a rule this engine does not apply, " +
		"such as to the last baseline of their text"
}

// autoMarginEdges is which of a box's margins were declared "auto": one per
// edge, non-zero where the declaration was the keyword.
//
// It is built as an Edges and passed through the same arrangement the resolved
// margins are, because a box inside a turned subtree has its edges rearranged
// and the two answers have to be about the same sides. The value on each edge
// is a flag and not a length; nothing adds them up.
func (l *layouter) autoMarginEdges(b *Box) Edges {
	auto := func(name string) style.Unit {
		if trimmedLower(b.Style["margin-"+name]) == "auto" {
			return 1
		}
		return 0
	}
	return l.asLaidOut(b, Edges{
		Top:    auto("top"),
		Right:  auto("right"),
		Bottom: auto("bottom"),
		Left:   auto("left"),
	})
}

// fillMainAutoMargins is §9.5's first sentence, which comes before the packing
// and often instead of it.
//
// An automatic margin on the main axis takes the free space that is left on the
// line, and where there are several they take an equal share each. Only then is
// there anything for justify-content to place — and after this there is not,
// because the margins have absorbed all of it. That is the specification's
// order and it is why "margin-left: auto" pushes an item to the far end however
// the container is packed.
//
// The share goes into the item's own margin rather than being remembered
// separately, so everything downstream — the room the item takes on the line,
// the fragment's own reported margins, and the free space justify-content is
// handed — follows from the one number. A margin that has taken the free space
// *is* a margin that size.
func (l *layouter) fillMainAutoMargins(a flexAxis, line []*flexItem, main, gap style.Unit) {
	used := gap.Mul(float64(len(line) - 1))
	autos := 0
	for _, it := range line {
		used = used.Add(it.outer(it.target))
		if a.mainStart(it.auto) != 0 {
			autos++
		}
		if a.mainEnd(it.auto) != 0 {
			autos++
		}
	}
	free := main.Sub(used)
	if autos == 0 || free <= 0 {
		// §9.5's other half: with nothing to take, an auto margin is zero,
		// which is what it already is here.
		return
	}
	share := free.Div(float64(autos))
	for _, it := range line {
		if a.mainStart(it.auto) != 0 {
			it.addMargin(a.mainStartOf(&it.margin), share)
			it.mainSurround = it.mainSurround.Add(share)
		}
		if a.mainEnd(it.auto) != 0 {
			it.addMargin(a.mainEndOf(&it.margin), share)
			it.mainSurround = it.mainSurround.Add(share)
		}
	}
}

// fillCrossAutoMargins is §9.6's version of the same rule, one item at a time
// rather than one line: the room left across the line goes to whichever of the
// item's cross-axis margins asked for it, and to both equally where both did —
// which is the idiom for centring one item in a line.
//
// It runs before the stretch, because an item with an automatic cross margin is
// not stretched: it asked for the leftover as margin, and an item stretched to
// its line has no leftover to give.
func (l *layouter) fillCrossAutoMargins(a flexAxis, it *flexItem, cross style.Unit) {
	autos := 0
	if a.crossStart(it.auto) != 0 {
		autos++
	}
	if a.crossEnd(it.auto) != 0 {
		autos++
	}
	if autos == 0 {
		return
	}
	free := cross.Sub(it.outerCross(a.crossOf(it.frag.BorderRect)))
	if free <= 0 {
		return
	}
	share := free.Div(float64(autos))
	if a.crossStart(it.auto) != 0 {
		it.addMargin(a.crossStartOf(&it.margin), share)
		it.crossMargin = it.crossMargin.Add(share)
	}
	if a.crossEnd(it.auto) != 0 {
		it.addMargin(a.crossEndOf(&it.margin), share)
		it.crossMargin = it.crossMargin.Add(share)
	}
}

// orderOf reads §5.4's order, which is an integer and nothing else.
//
// A value with a fraction in it is not an integer, so the declaration is thrown
// out and the initial value stands — the grammar has nowhere to put the half,
// and rounding it would be inventing a value the author did not write. That is
// the same answer flexValuesOf gives a factor it cannot read.
//
// The magnitude is bounded because only the relative order matters and an
// unbounded one would overflow the comparison. A document that reaches the
// bound has items whose order differs by more than a billion, and they tie —
// which leaves them in document order, the answer they would have had anyway.
func orderOf(b *Box) int {
	s := trimmedLower(b.Style["order"])
	sign := 1
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	}
	if s == "" {
		return 0
	}
	const bound = 1 << 30
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
		if n > bound {
			n = bound
		}
	}
	return sign * n
}

// flexContent lays a flex container's items out along its main axis and returns
// the height they came to.
//
// It is called in place of the block stacking, from children, exactly as a
// table's own placement is — the two are the layout modes whose children are
// not a flow.
func (l *layouter) flexContent(b *Box, parent *Fragment, width style.Unit,
	origin flow) style.Unit {

	a := l.axisOf(b)

	// The room the items are sized against, asked for before they are gathered
	// because an item may size itself as a share of it. A row's is its width and
	// a column's is a height it was told; a column that was told none has a main
	// size that is indefinite here and settled below, out of what the items
	// asked for.
	main, definite := l.statedMain(b, a, width, origin)
	items := l.flexItems(b, a, flexRoom{main: main, definite: definite, width: width}, origin)
	if len(items) == 0 {
		return 0
	}

	// §8 of Box Alignment: the gaps come out of the main axis before anything
	// else does. They are not free space that an item could grow into and not
	// space that shrinking gives back — a row of "flex: 1" items divides what is
	// left after the gaps, which is why every number below is measured against
	// the line and not against the container's own size.
	gap, crossGap := l.flexGap(b, a, width), l.flexCrossGap(b, a, width)
	if !definite {
		main = gap.Mul(float64(len(items) - 1))
		for _, it := range items {
			main = main.Add(it.outer(it.hypothetical))
		}
	}

	// §9.3: which items are on which line. A container that does not wrap has
	// one line whatever it holds.
	lines := flexLines(items, main, gap, l.wraps(b))
	for _, ln := range lines {
		l.resolveFlexibleLengths(ln, main.Sub(gap.Mul(float64(len(ln)-1))))
		// §9.5 before §9.4, because an automatic margin decides how much room
		// the item takes on the line and the item is about to be laid out.
		l.fillMainAutoMargins(a, ln, main, gap)
	}

	// §9.4: each item laid out at the size it was given. A row learns its lines'
	// cross sizes from this — a line is as tall as the tallest item on it —
	// while a column settled every item's width before it could measure a
	// height at all.
	mark := len(l.deferred)
	starts := make([]int, len(items))
	for i, it := range items {
		starts[i] = len(l.deferred)
		it.frag = l.layOutFlexItem(it, a, width, origin, it.target, true)
	}
	end := len(l.deferred)

	crosses, cross := l.flexCrossSizes(b, a, lines, crossGap, width, origin)
	for _, it := range items {
		l.fillCrossAutoMargins(a, it, crosses[it.line].size)
	}

	leadCross, betweenLines := l.alignContentSpacing(b, a, crosses, cross, crossGap)

	if !a.column {
		// §9.6's stretch, which is what "align-items: normal" comes to. An item
		// that states its own height keeps it; one that does not is laid out
		// again at its line's height, because a box's height changes where its
		// content sits inside it and cannot be applied afterwards.
		//
		// A column has nothing to do here: its cross size is the container's
		// own width, which was known before any of this and is already the size
		// each item was laid out at.
		//
		// The out-of-flow boxes a discarded layout found are discarded with it,
		// which is the same rule the block re-layout keeps and is kept here the
		// long way round. An absolutely positioned box inside a fragment that
		// was thrown away hangs off a fragment nobody will paint, and leaving
		// its record in place spends the budget in placeAbsolutes on a box that
		// is not on the page — enough of them and a document is told it holds
		// more out-of-flow boxes than this engine will place, which would be a
		// false report of a broken page.
		//
		// The records are contiguous per item and in item order, so the list is
		// rebuilt in that order rather than cut back to a mark: cutting back to
		// the first re-laid item would take every later item's records with it.
		// first is held before the rebuild starts, because the loop assigns
		// l.deferred as it goes and the ranges below are indexes into what the
		// first pass produced.
		first := l.deferred[:end]
		kept := append([]absCandidate(nil), first[:mark]...)
		for i, it := range items {
			stop := end
			if i+1 < len(items) {
				stop = starts[i+1]
			}
			want := maxZero(crosses[it.line].size.Sub(it.crossMargin))
			if l.alignOf(b, a, it) == crossStretch && l.stretchesAcross(a, it) &&
				it.frag.BorderRect.H != want {
				it.cross, it.hasCross = want, true
				l.deferred = kept
				it.frag = l.layOutFlexItem(it, a, width, origin, it.target, true)
				kept = l.deferred
				continue
			}
			kept = append(kept, first[starts[i]:stop]...)
		}
		l.deferred = kept
	}

	// §9.5's main-axis placement and §9.6's cross-axis placement, line by line,
	// in that order because the first needs the free space on the line and the
	// second needs the line's size.
	justify := l.justifyOf(b, a)
	at := leadCross
	for n, ln := range lines {
		used := gap.Mul(float64(len(ln) - 1))
		for _, it := range ln {
			used = used.Add(it.outer(it.target))
		}
		free := main.Sub(used)

		along := style.Unit(0)
		for i, it := range ln {
			// Both positions are of the item's *margin* box and are measured
			// from the start of their axis. Turning them into the page's own
			// coordinates is two steps, in this order: mirror where the axis
			// runs backwards, then step in by the leading margin — which is the
			// physical one either way, because a margin is a side of the box
			// and not an end of an axis.
			outer, outerCross := it.outer(it.target),
				it.outerCross(a.crossOf(it.frag.BorderRect))
			pos := along.Add(justifyOffset(justify, free, len(ln), i))
			across := at.Add(l.crossOffset(b, a, it, crosses[n]))
			a.place(it.frag,
				a.mainAt(pos, outer, main).Add(a.mainStart(it.margin)),
				a.crossAt(across, outerCross, cross).Add(a.crossStart(it.margin)))
			parent.Children = append(parent.Children, it.frag)
			along = along.Add(it.outer(it.target)).Add(gap)
		}
		at = at.Add(crosses[n].size).Add(crossGap).Add(betweenLines)
	}
	if a.column {
		// The height the container came to is the main size its items divided.
		return main
	}
	return cross
}

// wraps reports whether the container's lines break, which is §5.2.
//
// Both wrapping values wrap. Which end the lines are stacked from is the
// difference between them and is the cross axis's direction, which flexAxis
// carries; here the question is only whether there can be more than one line.
func (l *layouter) wraps(b *Box) bool {
	switch trimmedLower(b.Style["flex-wrap"]) {
	case "wrap", "wrap-reverse":
		return true
	}
	return false
}

// flexLines is §9.3: which items sit on which line.
//
// An item goes on the line in hand while it fits and starts a new one when it
// does not, measured against the *hypothetical* main sizes — what each item
// asked for before any of the free space moved. That order matters: the sizes
// §9.7 resolves are a consequence of which items share a line, so breaking
// against them would be circular.
//
// A line always holds at least one item. An item wider than the whole container
// has nowhere narrower to go, and pushing it to a line of its own is what makes
// the overflow one item's rather than the following item's as well.
func flexLines(items []*flexItem, main, gap style.Unit, wraps bool) [][]*flexItem {
	if !wraps {
		for _, it := range items {
			it.line = 0
		}
		return [][]*flexItem{items}
	}
	var out [][]*flexItem
	var line []*flexItem
	used := style.Unit(0)
	for _, it := range items {
		room := it.outer(it.hypothetical)
		if len(line) > 0 && used.Add(gap).Add(room) > main {
			out = append(out, line)
			line, used = nil, 0
		}
		if len(line) > 0 {
			used = used.Add(gap)
		}
		it.line = len(out)
		line = append(line, it)
		used = used.Add(room)
	}
	return append(out, line)
}

// flexCrossSizes is §9.4's step 8: how far across each line reaches, and how far
// the container does.
//
// A container that does not wrap has one line and, where its cross size is a
// number, that line takes the whole of it — the specification's own step, and
// what makes an item stretch to the container's edge rather than to the tallest
// of its siblings. Every other line is as big as the largest thing on it,
// including the single line of a container that was allowed to wrap and did
// not: it is the wrapping that decides this and not how many lines came of it,
// because align-content has the leftover to place either way.
func (l *layouter) flexCrossSizes(b *Box, a flexAxis, lines [][]*flexItem,
	crossGap, width style.Unit, origin flow) (crosses []lineCross, cross style.Unit) {

	container, definite := width, true
	if !a.column {
		container, definite = l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite)
	}
	crosses = make([]lineCross, len(lines))
	for i, ln := range lines {
		// The items aligned to a shared baseline are measured in two halves —
		// what is above that baseline and what is below — because the line has
		// to hold the deepest of each, and the two need not come from the same
		// item. The rest are measured whole, as they are aligned whole.
		var above, below style.Unit
		for _, it := range ln {
			outer := it.outerCross(a.crossOf(it.frag.BorderRect))
			if l.alignOf(b, a, it) != crossBaseline {
				if outer > crosses[i].size {
					crosses[i].size = outer
				}
				continue
			}
			base := baselineOf(a, it)
			above = max(above, base)
			below = max(below, outer.Sub(base))
		}
		crosses[i].above = above
		if sum := above.Add(below); sum > crosses[i].size {
			crosses[i].size = sum
		}
	}
	if !l.wraps(b) && definite {
		crosses[0].size = container
	}
	if definite {
		return crosses, container
	}
	for i, c := range crosses {
		if i > 0 {
			cross = cross.Add(crossGap)
		}
		cross = cross.Add(c.size)
	}
	return crosses, cross
}

// lineCross is what a line came to across the axis: how far it reaches, and how
// far into it sits the baseline the items that asked for one share.
type lineCross struct{ size, above style.Unit }

// baselineOf is how far the item's first baseline is from the start of its
// margin box, across the line.
//
// §9.4.8's fallback for an item that has no baseline — an empty box, a picture,
// anything with no text on a line — is to make one from its border box, and the
// edge it makes it from is the end: a box with nothing in it sits on the
// baseline rather than straddling it, which is what an inline-block with no
// content does in a line of text and is the same rule.
func baselineOf(a flexAxis, it *flexItem) style.Unit {
	start := a.crossStart(it.margin)
	if v, ok := firstBaseline(it.frag); ok {
		return start.Add(v)
	}
	return start.Add(a.crossOf(it.frag.BorderRect))
}

// alignContentSpacing is §9.6's align-content: where the lines sit in a cross
// size that is bigger than they are.
//
// It has nothing to say to a container that does not wrap, and there is no
// clause here that says so: such a container has one line, §9.4 gives that line
// the container's whole cross size, and a line that fills what it is in leaves
// nothing to place. The distinction is real and not a technicality — the same
// declaration on the same document moves the line of a container that was
// allowed to wrap and did not, because that line is only as big as its content.
// The property that moves an item inside its line is align-items; this moves
// the lines.
//
// The initial value stretches: the leftover is divided equally between the lines
// and added to each, which is why a wrapping container with a height has lines
// taller than their content. Every other value packs them and leaves the space
// where the keyword says, using the same arithmetic as justify-content on the
// other axis — they are one property in Box Alignment and it is one function
// here.
func (l *layouter) alignContentSpacing(b *Box, a flexAxis, crosses []lineCross,
	cross, crossGap style.Unit) (lead, between style.Unit) {

	used := crossGap.Mul(float64(len(crosses) - 1))
	for _, c := range crosses {
		used = used.Add(c.size)
	}
	free := cross.Sub(used)
	switch trimmedLower(b.Style["align-content"]) {
	case "", "normal", "stretch":
		if free <= 0 {
			return 0, 0
		}
		share := free.Div(float64(len(crosses)))
		for i := range crosses {
			crosses[i].size = crosses[i].size.Add(share)
		}
		return 0, 0
	}
	align := l.alignContentOf(b, a)
	// The lines are placed one after another, so what each one needs is where it
	// begins relative to the one before: the first line's own offset, and the
	// difference between consecutive offsets for the rest. Every distribution
	// this uses spaces its slots evenly, so that difference is one number.
	lead = justifyOffset(align, free, len(crosses), 0)
	if len(crosses) > 1 {
		between = justifyOffset(align, free, len(crosses), 1).Sub(lead)
	}
	return lead, between
}

// alignContentOf reads align-content, whose values are justify-content's and
// mean the same thing on the other axis.
func (l *layouter) alignContentOf(b *Box, a flexAxis) flexJustify {
	switch value := trimmedLower(b.Style["align-content"]); value {
	case "space-between":
		return justifyBetween
	case "space-around":
		return justifyAround
	case "space-evenly":
		return justifyEvenly
	default:
		// The rest name an end of the cross axis, which is the question
		// align-items asks about one item and this asks about every line.
		switch crossAlignment(value, a) {
		case crossEnd:
			return justifyEnd
		case crossCenter:
			return justifyCenter
		}
		return justifyStart
	}
}

// statedMain is the container's inner main size where it has one, which is what
// an item that sizes itself as a share of the line is a share of.
//
// A row's is its width, which its own containing block gave it: definite, and
// the same number whatever the items do. A column's is its height, and a height
// is usually not stated at all — §9.4 then makes the container's main size the
// sum of what its items asked for, which leaves nothing over to distribute.
// That is not a special case bolted on: it is what "indefinite main size" comes
// to. A line exactly as long as the items asked for has nothing left over and
// nothing missing, so §9.7 resolves every item to the size it asked for and
// neither flex-grow nor flex-shrink has anything to do — which is why a column
// of "flex: 1" items does not fill a container that never said how tall it was.
//
// Nothing wraps against a size arrived at this way, and §9.3 would have to say
// what to do if it could: a line breaks against the room left on it, and a
// container whose main size is the sum of its items always has exactly enough.
// It cannot arise — the gate refuses a wrapping column, and a row's main size is
// its width, which its containing block always states — and it is worth knowing
// that the two clauses meet rather than merely never having been seen to.
func (l *layouter) statedMain(b *Box, a flexAxis, width style.Unit,
	origin flow) (main style.Unit, definite bool) {

	if !a.column {
		return width, true
	}
	if h, ok := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite); ok {
		return h, true
	}
	return 0, false
}

// flexGap is the room §8 of Box Alignment leaves between one item and the next.
//
// column-gap's initial value is the keyword "normal", and what it computes to
// depends on who is asking: one em in a multi-column container, and zero in a
// flex one. That is why the cascade keeps the keyword rather than resolving it —
// see the entry in style/property.go, and l.columnGap, which answers the same
// question the other way for the same declaration.
//
// A negative gap is not a smaller one. The grammar does not allow it, so the
// declaration is thrown out and the initial value stands, which is the answer
// every other invalid length in this file gets.
func (l *layouter) flexGap(b *Box, a flexAxis, width style.Unit) style.Unit {
	if v, ok := l.lengthOf(b, a.gapName(), width); ok && v >= 0 {
		return v
	}
	return 0
}

// flexCrossGap is the room left between one line and the next, which is the gap
// along the other axis: rows of items are separated by a row gap, and the
// columns a wrapping column container makes by a column gap.
func (l *layouter) flexCrossGap(b *Box, a flexAxis, width style.Unit) style.Unit {
	return l.flexGap(b, flexAxis{column: !a.column}, width)
}

// flexJustify is §9.5's justify-content, and flexAlign is §9.6's align-items and
// align-self, which take the same values as each other on one axis each.
type flexJustify uint8

const (
	justifyStart flexJustify = iota
	justifyEnd
	justifyCenter
	justifyBetween
	justifyAround
	justifyEvenly
)

type flexAlign uint8

const (
	crossStretch flexAlign = iota
	crossStart
	crossEnd
	crossCenter
	crossBaseline
)

// justifyOf reads justify-content, whose initial value "normal" behaves as
// flex-start in a flex container.
//
// Three families of keyword name the same two ends by different routes, and
// they part company the moment an axis is reversed. "flex-start" is the end the
// items are laid out from, whichever end that is. "start" is the writing mode's
// — the inline start of a row, the block start of a column — which
// flex-direction can turn away from but "rtl" cannot, because "rtl" moves the
// writing mode and the axis together. "left" and "right" are the page's own,
// and they are the only pair that has to be resolved against everything at
// once.
//
// Each is expressed as the flex-relative value that lands in the same place,
// because that is the one the packing below is written in.
func (l *layouter) justifyOf(b *Box, a flexAxis) flexJustify {
	atStart, atEnd := justifyStart, justifyEnd
	switch trimmedLower(b.Style["justify-content"]) {
	case "flex-end":
		return atEnd
	case "center":
		return justifyCenter
	case "space-between":
		return justifyBetween
	case "space-around":
		return justifyAround
	case "space-evenly":
		return justifyEvenly
	case "end":
		if a.reverse {
			return atStart
		}
		return atEnd
	case "start":
		if a.reverse {
			return atEnd
		}
		return atStart
	case "left", "right":
		// §6.2: a column's main axis is not the inline axis, so neither
		// keyword has anything to say about it and both behave as "start".
		if a.column {
			if a.reverse {
				return atEnd
			}
			return atStart
		}
		wantsEnd := trimmedLower(b.Style["justify-content"]) == "right"
		if a.mainReversed() {
			wantsEnd = !wantsEnd
		}
		if wantsEnd {
			return atEnd
		}
		return atStart
	}
	return atStart
}

// alignOf is an item's cross-axis alignment: its own align-self where it states
// one, and the container's align-items where it does not.
//
// §6.2's "auto" on align-self means "whatever the container says", and "normal"
// on either means stretch in a flex container. The two keywords arrive at the
// same behaviour by different routes and both are kept as themselves in the
// computed value, which is why this is a lookup and not a comparison against a
// resolved string.
func (l *layouter) alignOf(b *Box, a flexAxis, it *flexItem) flexAlign {
	self := trimmedLower(it.box.Style["align-self"])
	if self == "" || self == "auto" {
		self = trimmedLower(b.Style["align-items"])
	}
	return crossAlignment(self, a)
}

// crossAlignment turns one of §6.2's keywords into the end of the cross axis it
// names.
//
// "flex-start" is the end the lines are stacked from. "start" and "self-start"
// are the writing mode's block start for a row and its inline start for a
// column, which is the same end until wrap-reverse turns the cross axis round —
// and only wrap-reverse, because "rtl" moves the writing mode with the axis.
func crossAlignment(value string, a flexAxis) flexAlign {
	switch value {
	case "flex-start":
		return crossStart
	case "flex-end":
		return crossEnd
	case "center":
		return crossCenter
	case "baseline", "first baseline":
		return crossBaseline
	case "start", "self-start":
		if a.wrapReverse {
			return crossEnd
		}
		return crossStart
	case "end", "self-end":
		if a.wrapReverse {
			return crossStart
		}
		return crossEnd
	}
	return crossStretch
}

// justifyOffset is §9.5's distribution: how far past its packed position the
// item at index i begins, out of n on a line with this much space left over.
//
// Each answer is a fraction of the whole free space rather than a gap added once
// per item, because a gap has to be rounded to a layout unit and adding a
// rounded gap n times puts the last item up to n half-units short of where the
// arithmetic says it goes. Taking the fraction each time bounds the error at
// half a unit no matter how many items there are.
//
// The fallbacks for an over-full line are §4.4 of Box Alignment, not a
// convenience: with nothing to distribute, space-between would want a negative
// gap and pull the items back over each other, so it packs at the start, and the
// two that fill the ends centre instead. The ones that were already packing —
// end and centre — keep their arithmetic, and overflow the near edge as much as
// the far one, which is what an unsafe alignment means.
func justifyOffset(justify flexJustify, free style.Unit, n, i int) style.Unit {
	if free < 0 {
		switch justify {
		case justifyBetween:
			return 0
		case justifyAround, justifyEvenly:
			justify = justifyCenter
		}
	}
	switch justify {
	case justifyEnd:
		return free
	case justifyCenter:
		return free.Div(2)
	case justifyBetween:
		// A single item has no pair to sit between and stays at the start,
		// which is what one interval over a zero-length row of gaps comes to.
		// The denominator is floored rather than special-cased because the
		// special case would be a branch no document can reach: with one item
		// the only index is zero, and zero of anything is zero.
		return free.Mul(float64(i) / float64(max(n-1, 1)))
	case justifyAround:
		// Half a share before the first and after the last, a whole one between
		// each pair, which is the same as i+½ shares of the n the line is cut
		// into.
		return free.Mul((float64(i) + 0.5) / float64(n))
	case justifyEvenly:
		return free.Mul(float64(i+1) / float64(n+1))
	}
	return 0
}

// crossOffset is how far into its line an item's margin box begins.
//
// A stretched item is at the start for the same reason a start-aligned one is:
// stretching has already given it the line's whole size, so there is nowhere
// else for it to be. The other two measure back from the line's far end, which
// is what makes them right whether the line is this item's size or bigger.
//
// The room left over is not floored at zero. An item taller than its line — the
// container stated a height and an item overran it — hangs off the start under
// flex-end and off both ends under center, which is what an alignment that is
// not "safe" means: it keeps the relationship it names and lets the overflow
// fall where the arithmetic puts it.
func (l *layouter) crossOffset(b *Box, a flexAxis, it *flexItem, line lineCross) style.Unit {
	left := line.size.Sub(it.outerCross(a.crossOf(it.frag.BorderRect)))
	switch l.alignOf(b, a, it) {
	case crossEnd:
		return left
	case crossCenter:
		return left.Div(2)
	case crossBaseline:
		// The item is moved down by as much as the deepest item's baseline is
		// deeper than its own, which is what puts the two on the same line. An
		// item that *is* the deepest does not move at all, so the group sits
		// against the start of the line rather than floating in the middle of
		// it — which is §9.4.8's other half and falls out of the arithmetic.
		return line.above.Sub(baselineOf(a, it))
	}
	return 0
}

// stretchesAcross reports whether an item is stretched to the line's height.
//
// §9.6: an item whose cross size is auto and whose cross-axis margins are not
// is stretched. Its caller has already asked whether the alignment is a stretch
// at all, so what is left is the two ways an item can have said otherwise
// itself: a size of its own — a box that stated one is that size, and
// stretching it would be overruling the declaration — and an automatic margin,
// which asked for the room across the line that stretching would use up.
func (l *layouter) stretchesAcross(a flexAxis, it *flexItem) bool {
	if a.crossStart(it.auto) != 0 || a.crossEnd(it.auto) != 0 {
		return false
	}
	name := a.crossName()
	return l.isAuto(it.box, name) || trimmedLower(it.box.Style[name]) == ""
}

// layOutFlexItem lays one item out at the sizes the container has settled on.
//
// main is a content size and is the one §9.7 resolved; hasMain says it is
// resolved yet, which it is not while a column is still measuring what its
// items would like to be. The cross size comes off the item itself — a border
// box, because that is what an item is aligned and stretched by — and it is
// known for a column from the start and for a row only after this has run once.
//
// The sizes are forced rather than declared, which is what forcedGeometry is
// for and is the same path an absolutely positioned box and a table cell take:
// the item goes through ordinary block layout with its geometry decided by the
// caller, so everything inside it — floats, lines, its own children — works
// exactly as it does anywhere else.
func (l *layouter) layOutFlexItem(it *flexItem, a flexAxis, width style.Unit,
	origin flow, main style.Unit, hasMain bool) *Fragment {

	geom := &forcedGeometry{margin: it.margin}
	inner := style.Unit(0)
	if it.hasCross {
		inner = maxZero(it.cross.Sub(a.crossEdge(it.border)).Sub(a.crossEdge(it.padding)))
	}
	switch {
	case a.column:
		geom.width = inner
		if hasMain {
			geom.height, geom.hasHeight = maxZero(main), true
		}
	default:
		geom.width = main
		if it.hasCross {
			geom.height, geom.hasHeight = inner, true
		}
	}
	return outOfClamp(l, func() *Fragment {
		f, _ := l.blockIn(it.box, width,
			flow{ctx: &floatContext{}, cbHeight: origin.cbHeight, cbDefinite: origin.cbDefinite},
			geom)
		return f
	})
}

// flexItems gathers the container's items and gives each its flex base size and
// hypothetical main size, which is §9.2.
func (l *layouter) flexItems(b *Box, a flexAxis, room flexRoom, origin flow) []*flexItem {
	width := room.width
	var out []*flexItem
	for _, c := range b.Children {
		if c.IsText() || (c.Anonymous() && len(c.Children) == 0) || c.outOfFlow() {
			// The same three the gate passed over: white space that collapses
			// to nothing, and a box that is out of the flow rather than in the
			// row. An anonymous box with something in it *is* an item — §4's
			// own, made in box.go, or the table repair's around a stray cell.
			continue
		}
		it := &flexItem{
			box:     c,
			values:  l.flexValuesOf(c, room),
			margin:  l.edges(c, "margin", width),
			border:  l.borderWidths(c),
			padding: l.paddingOf(c, width),
		}
		it.mainSurround = a.mainEdge(it.margin).
			Add(a.mainEdge(it.border)).Add(a.mainEdge(it.padding))
		it.crossMargin = a.crossEdge(it.margin)
		it.auto = l.autoMarginEdges(c)
		it.order = orderOf(c)
		out = append(out, it)
	}

	// §5.4's order-modified document order, which is where every other
	// question about an item is answered from: which line it lands on, where on
	// that line it sits, and — because the fragments are made in this order —
	// which of two overlapping items is painted over the other.
	//
	// The sort is stable, and that is the property rather than an
	// implementation note: items that named the same order keep the order the
	// document put them in, and the initial value being zero is what makes
	// "order: 1" mean "after everything that did not ask".
	slices.SortStableFunc(out, func(x, y *flexItem) int { return x.order - y.order })
	// A column measures each item by laying it out, and every one of those
	// layouts is thrown away — the fragment that is kept is the one made at the
	// size §9.7 settles on. What they took out of the flow goes with them; see
	// the same argument at the row's stretch pass, which cannot cut the list
	// back so simply because it keeps some of what it laid out.
	mark := len(l.deferred)
	for _, it := range out {
		if a.column {
			// §9.2 in the order a column has to take it. An item's height is
			// what its content comes to at the width it is going to be, so the
			// cross size is settled first and the main size measured against
			// it; a row asks neither question, because a width is given rather
			// than found.
			it.cross, it.hasCross = l.crossSizeOf(b, a, it, width), true
			it.measured = l.measuredMain(it, a, width, origin)
		}
		it.base = l.flexBaseSize(it, a, room)
		it.min, it.max = l.flexMainLimits(it, a, room)
		it.hypothetical = style.Clamp(it.base, it.min, it.max)
	}
	if a.column {
		l.deferred = l.deferred[:mark]
	}
	return out
}

// crossSizeOf is an item's used cross size as a border box, which a column has
// to settle before it can measure anything.
//
// §9.4 stretches an item whose cross size is auto to the line, and a single-line
// column's line is the container's own content box — so a stretched item is as
// wide as its container less its own margins. An item that is aligned instead is
// as wide as it needs to be and no wider: §9.2's "fit-content", which is the
// item's preferred width held between what its content demands and what the
// container has room for.
func (l *layouter) crossSizeOf(b *Box, a flexAxis, it *flexItem, width style.Unit) style.Unit {
	edge := a.crossEdge(it.border).Add(a.crossEdge(it.padding))
	room := maxZero(width.Sub(it.crossMargin))
	across := flexRoom{main: width, definite: true, width: width}
	if declared, ok := l.mainLength(it.box, flexAxis{column: !a.column}, a.crossName(), across); ok {
		// The item's own cross size, which is the main size of the axis this
		// one is not: a column's cross axis is a row's main axis, and what
		// box-sizing takes out of a declared size is decided the same way on
		// either.
		return declared.Add(edge)
	}
	if l.alignOf(b, a, it) == crossStretch && l.stretchesAcross(a, it) {
		return room
	}
	want := l.contentWidths(it.box)
	fit := style.Clamp(maxZero(room.Sub(edge)), want.min, want.max)
	return fit.Add(edge)
}

// measuredMain is what an item's content comes to along the main axis, which is
// a question only a column asks.
//
// It is asked by laying the item out at its cross size and reading the height
// that came back, because a height is not a number a stylesheet holds: it is
// what the text, the lines and the boxes inside came to. That is one layout per
// item beyond the one that is kept, and it is the price of a column — §9.2's
// "size the item into the available space" is a measurement wherever the main
// axis is the block axis.
func (l *layouter) measuredMain(it *flexItem, a flexAxis, width style.Unit,
	origin flow) style.Unit {

	f := l.layOutFlexItem(it, a, width, origin, 0, false)
	return maxZero(a.mainOf(f.BorderRect).Sub(a.mainEdge(it.border)).
		Sub(a.mainEdge(it.padding)))
}

// flexBaseSize is §9.2's step 3: what the item would like to be before any of
// the free space is shared out.
//
// The order is the specification's. A declared flex-basis wins outright; "auto"
// defers to the item's own width; and a width of auto falls to the item's
// max-content size, which is the size it would take if the line were as wide as
// it liked. That last one is why a row of three words comes out as three words
// wide rather than three equal thirds.
func (l *layouter) flexBaseSize(it *flexItem, a flexAxis, room flexRoom) style.Unit {
	if !it.values.basisAuto {
		return maxZero(it.values.basis.Sub(l.sizingEdgeOf(it.box, a, room.width)))
	}
	if declared, ok := l.mainLength(it.box, a, a.mainName(), room); ok &&
		!it.values.basisContent {
		return declared
	}
	if a.column {
		// The max-content size of a block along its own block axis is what its
		// content came to, which is the measurement already taken.
		return it.measured
	}
	return l.contentWidths(it.box).max
}

// flexRoom is what the container offers an item that sizes itself against it:
// the main size the items are being fitted to, whether that is a number at all,
// and the width its percentages resolve against.
//
// The two sizes are not the same question. A percentage *main* size is of the
// line — "width: 50%" in a row is half the row, "height: 50%" down a column is
// half the column — while a percentage padding or margin is of the container's
// width whichever axis it is on, because that is what CSS 2.1 §8 says and it is
// true on both.
type flexRoom struct {
	main     style.Unit
	definite bool
	width    style.Unit
}

// mainLength is one of the item's own main-axis sizes, resolved as a content
// size, or false where the declaration is not a length this can answer.
//
// It exists because "box-sizing: border-box" means a declared size includes the
// border and the padding *on the axis that size is on*, and which axis that is
// is the container's question rather than the property's: the same "height:
// 100px" is a main size down a column and a cross size across a row.
// intrinsicLength answers for a width, which is right wherever a main size is
// one and wrong by the difference between the two insets everywhere else.
//
// The padding is resolved against the container's own width, on both axes,
// because that is what a percentage padding is a percentage of — and unlike
// intrinsic sizing, which has to ask before there is an answer, a flex item is
// being fitted into a line whose width is already known.
//
// A percentage *size* is of the line, and §9.2's answer where the line has no
// size — a column that was never told how tall to be — is that the declaration
// is indefinite and the item falls back to what auto would have given it. That
// is not a refusal: it is what "height: 50%" means inside a box whose height is
// its content, which is the same rule block layout follows for the same reason.
func (l *layouter) mainLength(c *Box, a flexAxis, property string, room flexRoom) (style.Unit, bool) {
	length, ok := l.parseLength(c, property)
	if !ok {
		return 0, false
	}
	v, ok := length.Resolve(room.main, room.definite)
	if !ok {
		return 0, false
	}
	return maxZero(v.Sub(l.sizingEdgeOf(c, a, room.width))), true
}

// sizingEdgeOf is what box-sizing takes out of a declared main size.
func (l *layouter) sizingEdgeOf(c *Box, a flexAxis, containing style.Unit) style.Unit {
	horizontal, vertical := l.sizingInset(c, containing)
	if a.column {
		return vertical
	}
	return horizontal
}

// flexMainLimits is an item's own minimum and maximum main size.
//
// The minimum is §4.5's automatic one where the item declares none, and it is
// the clause that stops a row of text collapsing to nothing: a flex item's
// automatic minimum is its content-based minimum size, so a word cannot be
// shrunk narrower than the word. It is capped by a declared size, because an
// item that asked to be narrower than its content asked for that.
func (l *layouter) flexMainLimits(it *flexItem, a flexAxis, room flexRoom) (min, max style.Unit) {
	c := it.box
	max = style.MaxUnit
	if v, ok := l.mainLength(c, a, a.maxName(), room); ok {
		max = v
	}
	if !l.isAuto(c, a.minName()) {
		if v, ok := l.mainLength(c, a, a.minName(), room); ok {
			return v, max
		}
	}
	// §4.5's automatic minimum. It is reached through the *initial* value rather
	// than through a keyword an author wrote, which is why min-width's initial
	// value is "auto" in the registry and not the "0" CSS 2.1 gave it: an item
	// carrying a computed zero is indistinguishable from one whose author asked
	// for zero, and asking for zero is the idiom for defeating this very rule.
	min = l.contentWidths(c).min
	if a.column {
		// §4.5's content size suggestion along the block axis: the smallest a
		// block can be without its content spilling out of it is the size its
		// content came to, because a block does not reflow to be shorter. It is
		// what makes a column of paragraphs refuse to shrink, which is the
		// behaviour authors meet as "my flex item will not get smaller".
		min = it.measured
	}
	if declared, ok := l.mainLength(c, a, a.mainName(), room); ok && declared < min {
		min = declared
	}
	if max < min {
		min = max
	}
	return min, max
}

// resolveFlexibleLengths is §9.7, which is the whole of what a flex container
// is for.
//
// The shape of it: every item states a size it would like, the container adds
// up what that comes to, and the difference between that and the room available
// is shared out in the proportions the items declared. An item that hits its own
// minimum or maximum on the way is *frozen* at that size and the loop runs
// again over the rest — which is why this is a loop and not a division. Freezing
// one item changes every other item's share, and stopping after one pass leaves
// a row whose parts do not add up to the whole.
//
// Growing and shrinking are not symmetric, and the asymmetry is the part worth
// reading twice. Growth is shared in proportion to flex-grow alone: two items
// with the same factor take the same number of pixels however big they started.
// Shrinkage is shared in proportion to flex-shrink *scaled by the item's base
// size*, so a long item gives up more than a short one with the same factor —
// which is what stops a two-character item being shrunk out of existence beside
// a paragraph.
func (l *layouter) resolveFlexibleLengths(items []*flexItem, inner style.Unit) {
	// §9.7.1: which factor is in use, decided once from the hypothetical sizes
	// and not revisited. An item frozen later can push the total past the line
	// without turning this into a shrink.
	hypothetical := style.Unit(0)
	for _, it := range items {
		hypothetical = hypothetical.Add(it.outer(it.hypothetical))
	}
	growing := hypothetical < inner

	// §9.7.2: the items that cannot move. A zero factor is the obvious one; the
	// other is an item whose hypothetical size is already on the far side of its
	// base — it has been clamped in the direction the line is about to push it,
	// so pushing it further would take it away from the size it asked for.
	//
	// The zero-factor half is the specification's step and is kept as one, but
	// it decides nothing on its own: distributeFlexSpace skips a zero factor
	// anyway, so an item left unfrozen here takes its base size, is clamped to
	// the same number the freeze would have given it, and is frozen at the end
	// of the first pass. Planting its removal changed no result. What it saves
	// is that pass, and what it is worth beyond that is that §9.7's steps can be
	// read against this in order.
	for _, it := range items {
		factor := it.values.shrink
		if growing {
			factor = it.values.grow
		}
		switch {
		case factor == 0,
			growing && it.base > it.hypothetical,
			!growing && it.base < it.hypothetical:
			it.frozen, it.target = true, it.hypothetical
		default:
			it.target = it.base
		}
	}

	// §9.7.3's initial free space, kept for the clause in 9.7.4b that stops a
	// row of items with factors adding to less than one from taking all of it.
	initial := freeFlexSpace(items, inner)

	// The loop cannot run more times than there are items: every pass either
	// exits or freezes at least one, which is what §9.7.4e guarantees. The bound
	// is written down anyway, because a loop over a floating-point condition in
	// a document the engine did not write is exactly where a hang comes from.
	for pass := 0; pass <= len(items); pass++ {
		unfrozen := 0
		factors := 0.0
		for _, it := range items {
			if it.frozen {
				continue
			}
			unfrozen++
			if growing {
				factors += it.values.grow
			} else {
				factors += it.values.shrink
			}
		}
		if unfrozen == 0 {
			return
		}

		remaining := freeFlexSpace(items, inner)
		if factors < 1 {
			// §9.7.4b. The items between them asked for less than the whole of
			// the free space, so they get that fraction of it and the rest is
			// left over — which is what makes "flex-grow: 0.5" mean half.
			if scaled := initial.Mul(factors); absUnit(scaled) < absUnit(remaining) {
				remaining = scaled
			}
		}

		if remaining != 0 {
			l.distributeFlexSpace(items, remaining, factors, growing)
		}

		// §9.7.4d and e: clamp, add up which way the clamping went, and freeze
		// the items that were pushed past a limit. The sign of the total is what
		// decides *which* ones: if the clamping took size away overall then the
		// items at their minimum are the ones holding the line, and freezing the
		// others would freeze the wrong half.
		total := style.Unit(0)
		for _, it := range items {
			if it.frozen {
				continue
			}
			clamped := style.Clamp(it.target, it.min, it.max)
			total = total.Add(clamped.Sub(it.target))
			it.target = clamped
		}
		for _, it := range items {
			if it.frozen {
				continue
			}
			switch {
			case total == 0:
				it.frozen = true
			case total > 0:
				it.frozen = it.target == it.min
			case total < 0:
				it.frozen = it.target == it.max
			}
		}
	}
}

// distributeFlexSpace is §9.7.4c: the free space shared out among the items
// that can still move.
func (l *layouter) distributeFlexSpace(items []*flexItem, remaining style.Unit,
	factors float64, growing bool) {

	if growing {
		for _, it := range items {
			if it.frozen || it.values.grow == 0 {
				continue
			}
			it.target = it.base.Add(remaining.Mul(it.values.grow / factors))
		}
		return
	}
	// The scaled factors, which is the asymmetry: shrinkage is proportional to
	// flex-shrink times the base size, so the item with more to give gives more.
	scaled := 0.0
	for _, it := range items {
		if it.frozen {
			continue
		}
		scaled += it.values.shrink * it.base.Px()
	}
	if scaled == 0 {
		return
	}
	for _, it := range items {
		if it.frozen || it.values.shrink == 0 {
			continue
		}
		share := it.values.shrink * it.base.Px() / scaled
		it.target = it.base.Sub(absUnit(remaining).Mul(share))
	}
}

// freeFlexSpace is what the line has left over: its inner size less what the
// items take at the sizes they are currently held at.
//
// A frozen item is counted at its target and an unfrozen one at its base, which
// is §9.7.3 and §9.7.4b in one sentence — an item that has stopped moving takes
// the room it settled on, and one that has not takes the room it started from.
func freeFlexSpace(items []*flexItem, inner style.Unit) style.Unit {
	used := style.Unit(0)
	for _, it := range items {
		if it.frozen {
			used = used.Add(it.outer(it.target))
		} else {
			used = used.Add(it.outer(it.base))
		}
	}
	return inner.Sub(used)
}

// absUnit is the magnitude of a length, which §9.7.4b and §9.7.4c both need:
// the free space is negative when the line is over-full, and both the clause
// that caps it and the one that shares it out are about how much there is
// rather than which way it goes.
func absUnit(v style.Unit) style.Unit {
	if v < 0 {
		return -v
	}
	return v
}

// flexes reports whether a flex container is one this engine arranges, and
// reports it where it is not.
//
// Once per container, because the finding is about the box and a container
// asked twice — the intrinsic pass and the layout — would say it twice.
func (l *layouter) flexes(b *Box, width style.Unit) bool {
	why := l.refusesToFlex(b, width)
	if why == "" {
		return true
	}
	if l.reportedFlex == nil {
		l.reportedFlex = map[*Box]bool{}
	}
	if !l.reportedFlex[b] {
		l.reportedFlex[b] = true
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "this flex container was laid out as a block because " + why +
				"; its items are stacked rather than arranged",
			Path:     PathOf(b.Element),
			Property: "display",
		})
	}
	return false
}
