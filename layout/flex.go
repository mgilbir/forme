package layout

import (
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
	// "auto" — which defers to the item's own width rather than naming a size.
	basis     style.Unit
	basisAuto bool
}

// flexValuesOf reads the three properties off an item.
//
// A value the grammar does not allow gives the initial one, which is what the
// cascade would have produced had the declaration been thrown out — the same
// answer floatOf and clearOf give, and for the same reason.
func (l *layouter) flexValuesOf(b *Box, containing style.Unit) flexValues {
	out := flexValues{grow: 0, shrink: 1, basisAuto: true}
	if v, ok := parseNumber(trimmedLower(b.Style["flex-grow"])); ok && v >= 0 {
		out.grow = v
	}
	if v, ok := parseNumber(trimmedLower(b.Style["flex-shrink"])); ok && v >= 0 {
		out.shrink = v
	}
	switch basis := trimmedLower(b.Style["flex-basis"]); basis {
	case "", "auto", "content":
		// "content" is "size it as though flex-basis were auto and width were
		// auto", which for the boxes this file accepts is what auto already
		// comes to: the gate refuses a percentage main size, so the only other
		// thing auto could defer to is a declared length, and "content" says to
		// ignore that. It is folded in here rather than given a case of its own
		// because the difference cannot be reached — the gate refuses every box
		// where the two would disagree.
	default:
		if v, ok := l.lengthOf(b, "flex-basis", containing); ok && v >= 0 {
			out.basis, out.basisAuto = v, false
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
type flexAxis struct{ column bool }

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

// place puts a fragment at a main and cross position.
func (a flexAxis) place(f *Fragment, main, cross style.Unit) {
	if a.column {
		f.BorderRect.Y, f.BorderRect.X = main, cross
		return
	}
	f.BorderRect.X, f.BorderRect.Y = main, cross
}

// axisOf reads flex-direction. The gate has refused the two reversed values, so
// what is left is which of the two axes the items run along.
func (l *layouter) axisOf(b *Box) flexAxis {
	return flexAxis{column: trimmedLower(b.Style["flex-direction"]) == "column"}
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
	// one of them and neither depends on the item's size.
	mainSurround, crossMargin style.Unit
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
	case "", "row", "column":
	default:
		// The two reversed directions put the main axis's start at the far end,
		// which reverses the order the items are placed in and the end each
		// alignment measures from. That is the same reversal "rtl" asks for
		// below, and it is one change, not two.
		return "its items run backwards along their axis"
	}
	if isRTL(b) {
		// Every offset below is measured from the left edge, because that is
		// where a left-to-right inline axis starts — the main axis of a row and
		// the cross axis of a column. Under "rtl" it starts at the right, so
		// packing at the start, the meaning of "left" and "right", and the
		// order the items are placed in all reverse together.
		return "its inline axis runs from the right, which reverses every " +
			"position on it"
	}
	switch trimmedLower(b.Style["flex-wrap"]) {
	case "", "nowrap":
	default:
		return "its items wrap onto more than one line"
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
	switch trimmedLower(b.Style["align-items"]) {
	case "", "normal", "stretch", "flex-start", "start", "self-start",
		"flex-end", "end", "self-end", "center":
	default:
		return "its items are aligned across the line by a rule this engine " +
			"does not apply, such as to a shared baseline"
	}
	items, outOfFlow := 0, false
	for _, c := range b.Children {
		if c.Outer == OuterInline && c.Inner == InnerText &&
			strings.TrimSpace(c.Text) != "" {
			// §4's anonymous flex item. It is a box this engine never made —
			// a run of text in a flex container becomes one item rather than
			// taking part in an inline formatting context — and making it is a
			// change to the box tree rather than to layout.
			return "it holds text that is not inside an element, which becomes " +
				"an item of its own"
		}
		if c.IsText() || c.Anonymous() {
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
		switch trimmedLower(c.Style["align-self"]) {
		case "", "auto", "normal", "stretch", "flex-start", "start", "self-start",
			"flex-end", "end", "self-end", "center":
		default:
			return "one of its items is aligned across the line by a rule this " +
				"engine does not apply, such as to a shared baseline"
		}
		if v, ok := parseNumber(trimmedLower(c.Style["order"])); ok && v != 0 {
			return "one of its items asks to be moved in the order"
		}
		if hasAutoMargin(c) {
			// §9.5 gives an auto main-axis margin the free space before
			// justify-content sees any of it, and §9.6 gives an auto cross-axis
			// one the room left across the line before align-self does. Both are
			// a second way of distributing the same budget, and both would have
			// to be settled before the alignment below is even asked.
			return "one of its items has an automatic margin, which takes the " +
				"free space before the alignment does"
		}
		if l.percentageMainSize(c, a) {
			// A percentage of the container's own main size, which is what this
			// is resolving. §9.2 answers it — the percentage is against the
			// container's inner size, which is known — but the item's own
			// intrinsic contribution is then circular, and the circularity is
			// what this refuses rather than guesses at.
			return "one of its items sizes itself as a percentage of the row " +
				"it is being fitted into"
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

// hasAutoMargin reports whether a box declares an automatic margin on either
// axis. Both axes distribute free space to one before the alignment properties
// see it; see the clause in refusesToFlex that refuses them.
func hasAutoMargin(b *Box) bool {
	return trimmedLower(b.Style["margin-left"]) == "auto" ||
		trimmedLower(b.Style["margin-right"]) == "auto" ||
		trimmedLower(b.Style["margin-top"]) == "auto" ||
		trimmedLower(b.Style["margin-bottom"]) == "auto"
}

// percentageMainSize reports whether a box's own main-axis size is a percentage.
func percentageMainSize(value string) bool {
	v := trimmedLower(value)
	return strings.HasSuffix(v, "%") && v != "%"
}

func (l *layouter) percentageMainSize(b *Box, a flexAxis) bool {
	return percentageMainSize(b.Style[a.mainName()]) ||
		percentageMainSize(b.Style["flex-basis"]) ||
		percentageMainSize(b.Style[a.minName()]) ||
		percentageMainSize(b.Style[a.maxName()])
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
	items := l.flexItems(b, a, width, origin)
	if len(items) == 0 {
		return 0
	}

	// §8 of Box Alignment: the gaps come out of the main axis before anything
	// else does. They are not free space that an item could grow into and not
	// space that shrinking gives back — a row of "flex: 1" items divides what is
	// left after the gaps, which is why every number below is measured against
	// the line and not against the container's own size.
	gap := l.flexGap(b, a, width)
	gaps := gap.Mul(float64(len(items) - 1))
	line := l.flexLine(b, a, items, gaps, width, origin)
	l.resolveFlexibleLengths(items, line)

	// §9.4: each item laid out at the size it was given. A row learns its cross
	// size from this — the line is as tall as the tallest item — while a column
	// settled every item's width before it could measure a height at all.
	mark := len(l.deferred)
	starts := make([]int, len(items))
	for i, it := range items {
		starts[i] = len(l.deferred)
		it.frag = l.layOutFlexItem(it, a, width, origin, it.target, true)
	}
	end := len(l.deferred)

	cross := width
	if !a.column {
		cross = 0
		for _, it := range items {
			if h := it.outerCross(it.frag.BorderRect.H); h > cross {
				cross = h
			}
		}
		if h, ok := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite); ok {
			// §9.4's "definite cross size of the flex container", which the
			// single line takes whole. An item stretched to it reaches the
			// container's own edge rather than the tallest of its siblings.
			cross = h
		}

		// §9.6's stretch, which is what "align-items: normal" comes to. An item
		// that states its own height keeps it; one that does not is laid out
		// again at the line's height, because a box's height changes where its
		// content sits inside it and cannot be applied afterwards.
		//
		// A column has nothing to do here: its cross size is the container's
		// own width, which was known before any of this and is already the size
		// each item was laid out at.
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
			want := maxZero(cross.Sub(it.crossMargin))
			if l.alignOf(b, it) == crossStretch && l.stretchesAcross(a, it) &&
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

	// §9.5's main-axis placement and §9.6's cross-axis placement, in that order
	// because the first needs the free space and the second needs the line.
	used := style.Unit(0)
	for _, it := range items {
		used = used.Add(it.outer(it.target))
	}
	justify, free := l.justifyOf(b, a), line.Sub(used)

	along := style.Unit(0)
	for i, it := range items {
		at := along.Add(justifyOffset(justify, free, len(items), i))
		a.place(it.frag, at.Add(a.mainStart(it.margin)), l.crossOffset(b, a, it, cross))
		parent.Children = append(parent.Children, it.frag)
		along = along.Add(it.outer(it.target)).Add(gap)
	}
	if a.column {
		// The height the container came to: the line the items divided, and the
		// gaps that were taken off it before they did.
		return line.Add(gaps)
	}
	return cross
}

// flexLine is the main size the items divide.
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
func (l *layouter) flexLine(b *Box, a flexAxis, items []*flexItem,
	gaps, width style.Unit, origin flow) style.Unit {

	if !a.column {
		return width.Sub(gaps)
	}
	if h, ok := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite); ok {
		return maxZero(h.Sub(gaps))
	}
	line := style.Unit(0)
	for _, it := range items {
		line = line.Add(it.outer(it.hypothetical))
	}
	return line
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
)

// justifyOf reads justify-content, whose initial value "normal" behaves as
// flex-start in a flex container. "left" and "right" are the physical pair, and
// in a left-to-right row they are the same two ends.
func (l *layouter) justifyOf(b *Box, a flexAxis) flexJustify {
	value := trimmedLower(b.Style["justify-content"])
	if a.column && (value == "left" || value == "right") {
		// §6.2: the two physical keywords name an inline-axis side, and a
		// column's main axis is not the inline axis. They have nothing to say
		// about it and behave as "start".
		return justifyStart
	}
	switch value {
	case "flex-end", "end", "right":
		return justifyEnd
	case "center":
		return justifyCenter
	case "space-between":
		return justifyBetween
	case "space-around":
		return justifyAround
	case "space-evenly":
		return justifyEvenly
	}
	return justifyStart
}

// alignOf is an item's cross-axis alignment: its own align-self where it states
// one, and the container's align-items where it does not.
//
// §6.2's "auto" on align-self means "whatever the container says", and "normal"
// on either means stretch in a flex container. The two keywords arrive at the
// same behaviour by different routes and both are kept as themselves in the
// computed value, which is why this is a lookup and not a comparison against a
// resolved string.
func (l *layouter) alignOf(b *Box, it *flexItem) flexAlign {
	self := trimmedLower(it.box.Style["align-self"])
	if self == "" || self == "auto" {
		self = trimmedLower(b.Style["align-items"])
	}
	switch self {
	case "flex-start", "start", "self-start":
		return crossStart
	case "flex-end", "end", "self-end":
		return crossEnd
	case "center":
		return crossCenter
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

// crossOffset is where an item's border box sits across the line.
//
// A stretched item is at the top for the same reason a start-aligned one is:
// stretching has already given it the line's height, so there is nowhere else
// for it to be. The other two measure back from the line's far edge, which is
// what makes them right whether the line is this item's height or taller.
//
// The room left over is not floored at zero. An item taller than its line — the
// container stated a height and an item overran it — hangs off the top under
// flex-end and off both edges under center, which is what an alignment that is
// not "safe" means: it keeps the relationship it names and lets the overflow
// fall where the arithmetic puts it.
func (l *layouter) crossOffset(b *Box, a flexAxis, it *flexItem, cross style.Unit) style.Unit {
	left := cross.Sub(it.outerCross(a.crossOf(it.frag.BorderRect)))
	switch l.alignOf(b, it) {
	case crossEnd:
		return left.Add(a.crossStart(it.margin))
	case crossCenter:
		return left.Div(2).Add(a.crossStart(it.margin))
	}
	return a.crossStart(it.margin)
}

// stretchesAcross reports whether an item is stretched to the line's height.
//
// §9.6: an item whose cross size is auto is stretched. Its caller has already
// asked whether the alignment is a stretch at all, and the gate has refused the
// auto cross-axis margins that would take the room first, so what is left to ask
// is whether the item stated a height of its own — a box that did is the height
// it asked for, and stretching it would be overruling the declaration.
func (l *layouter) stretchesAcross(a flexAxis, it *flexItem) bool {
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
func (l *layouter) flexItems(b *Box, a flexAxis, width style.Unit, origin flow) []*flexItem {
	var out []*flexItem
	for _, c := range b.Children {
		if c.IsText() || c.Anonymous() || c.outOfFlow() {
			// The gate has refused a container holding any of these except an
			// anonymous box with nothing in it, which is not an item.
			continue
		}
		it := &flexItem{
			box:     c,
			values:  l.flexValuesOf(c, width),
			margin:  l.edges(c, "margin", width),
			border:  l.borderWidths(c),
			padding: l.paddingOf(c, width),
		}
		it.mainSurround = a.mainEdge(it.margin).
			Add(a.mainEdge(it.border)).Add(a.mainEdge(it.padding))
		it.crossMargin = a.crossEdge(it.margin)
		out = append(out, it)
	}
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
		it.base = l.flexBaseSize(it, a, width)
		it.min, it.max = l.flexMainLimits(it, a, width)
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
	if declared, ok := l.intrinsicLength(it.box, a.crossName()); ok {
		return declared.Add(edge)
	}
	if l.alignOf(b, it) == crossStretch && l.stretchesAcross(a, it) {
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
func (l *layouter) flexBaseSize(it *flexItem, a flexAxis, width style.Unit) style.Unit {
	if !it.values.basisAuto {
		return maxZero(it.values.basis.Sub(l.sizingEdgeOf(it.box, width)))
	}
	if declared, ok := l.intrinsicLength(it.box, a.mainName()); ok {
		return declared
	}
	if a.column {
		// The max-content size of a block along its own block axis is what its
		// content came to, which is the measurement already taken.
		return it.measured
	}
	return l.contentWidths(it.box).max
}

// sizingEdgeOf is what box-sizing takes out of a declared size.
func (l *layouter) sizingEdgeOf(c *Box, containing style.Unit) style.Unit {
	_, inset := l.sizingInset(c, containing)
	return inset
}

// flexMainLimits is an item's own minimum and maximum main size.
//
// The minimum is §4.5's automatic one where the item declares none, and it is
// the clause that stops a row of text collapsing to nothing: a flex item's
// automatic minimum is its content-based minimum size, so a word cannot be
// shrunk narrower than the word. It is capped by a declared size, because an
// item that asked to be narrower than its content asked for that.
func (l *layouter) flexMainLimits(it *flexItem, a flexAxis, width style.Unit) (min, max style.Unit) {
	c := it.box
	max = style.MaxUnit
	if v, ok := l.intrinsicLength(c, a.maxName()); ok {
		max = v
	}
	if !l.isAuto(c, a.minName()) {
		if v, ok := l.intrinsicLength(c, a.minName()); ok {
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
	if declared, ok := l.intrinsicLength(c, a.mainName()); ok && declared < min {
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
