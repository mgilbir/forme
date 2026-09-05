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
// One line, running left to right, with every alignment property at its initial
// value. That is "display: flex" and nothing else written down, which is what
// the overwhelming majority of flex containers in real documents are — and it
// is the slice whose arithmetic can be stated exactly.
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

// flexItem is one in-flow child of a flex container, with everything §9 needs
// to size it and everything §9.7 fills in.
type flexItem struct {
	box    *Box
	values flexValues
	// margin, border and padding are the item's own, resolved once. The sum of
	// the three is what separates an item's *content* size — which is what §9.7
	// distributes — from the room it takes on the line.
	margin, border, padding Edges
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
	return content.Add(it.margin.Horizontal()).
		Add(it.border.Horizontal()).Add(it.padding.Horizontal())
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
	switch trimmedLower(b.Style["flex-direction"]) {
	case "", "row":
	default:
		return "its main axis is not a left-to-right row"
	}
	switch trimmedLower(b.Style["flex-wrap"]) {
	case "", "nowrap":
	default:
		return "its items wrap onto more than one line"
	}
	switch trimmedLower(b.Style["justify-content"]) {
	case "", "normal", "flex-start", "start", "left":
	default:
		return "its items are not packed at the start of the line"
	}
	switch trimmedLower(b.Style["align-items"]) {
	case "", "normal", "stretch":
	default:
		return "its items are not stretched across the line"
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
		case "", "auto", "normal", "stretch":
		default:
			return "one of its items is aligned across the line by itself"
		}
		if v, ok := parseNumber(trimmedLower(c.Style["order"])); ok && v != 0 {
			return "one of its items asks to be moved in the order"
		}
		if hasAutoMargin(c) {
			// §9.5 gives an auto margin the free space before justify-content
			// sees any of it, which is a second way of distributing the same
			// budget and is not this one.
			return "one of its items has an automatic margin, which takes the " +
				"free space before the items do"
		}
		if l.percentageMainSize(c) {
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

// hasAutoMargin reports whether a box declares an automatic margin on the main
// axis.
func hasAutoMargin(b *Box) bool {
	return trimmedLower(b.Style["margin-left"]) == "auto" ||
		trimmedLower(b.Style["margin-right"]) == "auto"
}

// percentageMainSize reports whether a box's own main-axis size is a percentage.
func percentageMainSize(value string) bool {
	v := trimmedLower(value)
	return strings.HasSuffix(v, "%") && v != "%"
}

func (l *layouter) percentageMainSize(b *Box) bool {
	return percentageMainSize(b.Style["width"]) ||
		percentageMainSize(b.Style["flex-basis"]) ||
		percentageMainSize(b.Style["min-width"]) ||
		percentageMainSize(b.Style["max-width"])
}

// flexContent lays a flex container's items out along its main axis and returns
// the height they came to.
//
// It is called in place of the block stacking, from children, exactly as a
// table's own placement is — the two are the layout modes whose children are
// not a flow.
func (l *layouter) flexContent(b *Box, parent *Fragment, width style.Unit,
	origin flow) style.Unit {

	items := l.flexItems(b, width)
	if len(items) == 0 {
		return 0
	}
	l.resolveFlexibleLengths(items, width)

	// §9.4: each item laid out at the size it was given, which is what tells
	// the line how tall it is.
	for _, it := range items {
		it.frag = l.layOutFlexItem(b, it, width, origin, 0, false)
	}
	cross := style.Unit(0)
	for _, it := range items {
		if h := it.frag.BorderRect.H.Add(it.margin.Vertical()); h > cross {
			cross = h
		}
	}
	if h, ok := l.explicitHeight(b, width, origin.cbHeight, origin.cbDefinite); ok {
		// §9.4's "definite cross size of the flex container", which the single
		// line takes whole. An item stretched to it reaches the container's own
		// edge rather than the tallest of its siblings.
		cross = h
	}

	// §9.6's stretch, which is what "align-items: normal" comes to in a flex
	// container. An item that states its own height keeps it; one that does not
	// is laid out again at the line's height, because a box's height changes
	// where its content sits inside it and cannot be applied afterwards.
	for _, it := range items {
		want := maxZero(cross.Sub(it.margin.Vertical()))
		if l.stretchesAcross(it) && it.frag.BorderRect.H != want {
			it.frag = l.layOutFlexItem(b, it, width, origin, want, true)
		}
	}

	// §9.5's main-axis placement, packed at the start of the line, and §9.6's
	// cross-axis placement, which for a stretched item is the line's top.
	x := style.Unit(0)
	for _, it := range items {
		it.frag.BorderRect.X = x.Add(it.margin.Left)
		it.frag.BorderRect.Y = it.margin.Top
		parent.Children = append(parent.Children, it.frag)
		x = x.Add(it.outer(it.target))
	}
	return cross
}

// stretchesAcross reports whether an item is stretched to the line's height.
//
// §9.6: an item whose cross size is auto and whose cross-axis margins are not
// auto is stretched. The gate has already refused every align-self this does
// not answer, so what is left to ask is whether the item stated a height of its
// own — a box that did is the height it asked for, and stretching it would be
// overruling the declaration.
func (l *layouter) stretchesAcross(it *flexItem) bool {
	if trimmedLower(it.box.Style["margin-top"]) == "auto" ||
		trimmedLower(it.box.Style["margin-bottom"]) == "auto" {
		return false
	}
	return l.isAuto(it.box, "height") || trimmedLower(it.box.Style["height"]) == ""
}

// layOutFlexItem lays one item out at the main size §9.7 gave it.
//
// The size is forced rather than declared, which is what forcedGeometry is for
// and is the same path an absolutely positioned box and a table cell take: the
// item goes through ordinary block layout, with its width and margins decided
// by the caller, so everything inside it — floats, lines, its own children —
// works exactly as it does anywhere else.
func (l *layouter) layOutFlexItem(b *Box, it *flexItem, width style.Unit,
	origin flow, height style.Unit, hasHeight bool) *Fragment {

	geom := &forcedGeometry{margin: it.margin, width: it.target}
	if hasHeight {
		geom.height, geom.hasHeight = maxZero(height.
			Sub(it.border.Vertical()).Sub(it.padding.Vertical())), true
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
func (l *layouter) flexItems(b *Box, width style.Unit) []*flexItem {
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
		it.base = l.flexBaseSize(c, it.values, width)
		it.min, it.max = l.flexMainLimits(c, width)
		it.hypothetical = style.Clamp(it.base, it.min, it.max)
		out = append(out, it)
	}
	return out
}

// flexBaseSize is §9.2's step 3: what the item would like to be before any of
// the free space is shared out.
//
// The order is the specification's. A declared flex-basis wins outright; "auto"
// defers to the item's own width; and a width of auto falls to the item's
// max-content size, which is the size it would take if the line were as wide as
// it liked. That last one is why a row of three words comes out as three words
// wide rather than three equal thirds.
func (l *layouter) flexBaseSize(c *Box, v flexValues, width style.Unit) style.Unit {
	if !v.basisAuto {
		return maxZero(v.basis.Sub(l.sizingEdgeOf(c, width)))
	}
	if declared, ok := l.intrinsicLength(c, "width"); ok {
		return declared
	}
	return l.contentWidths(c).max
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
func (l *layouter) flexMainLimits(c *Box, width style.Unit) (min, max style.Unit) {
	max = style.MaxUnit
	if v, ok := l.intrinsicLength(c, "max-width"); ok {
		max = v
	}
	if !l.isAuto(c, "min-width") {
		if v, ok := l.intrinsicLength(c, "min-width"); ok {
			return v, max
		}
	}
	// §4.5's automatic minimum. It is reached through the *initial* value rather
	// than through a keyword an author wrote, which is why min-width's initial
	// value is "auto" in the registry and not the "0" CSS 2.1 gave it: an item
	// carrying a computed zero is indistinguishable from one whose author asked
	// for zero, and asking for zero is the idiom for defeating this very rule.
	min = l.contentWidths(c).min
	if declared, ok := l.intrinsicLength(c, "width"); ok && declared < min {
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
