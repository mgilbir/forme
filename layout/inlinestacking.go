package layout

import "github.com/mgilbir/forme/html"

// The stacking level of an inline box.
//
// §9.9 gives a positioned box a stacking level, and CSS Color 4 gives one to a
// box with an opacity below one, and neither says "block". A <span> with
// "position: relative; z-index: 1" is painted at level one of the context
// around it, over every in-flow block that comes after it; one with "z-index:
// -1" is painted at step 3, under its own block's content; and a translucent
// span is painted where a "z-index: 0" box is, as one group. What the span
// contains goes with it: its words, the backgrounds of the inline boxes inside
// it, the inline-blocks and floats written inside it, and the blocks §9.2.1.1
// lifted out of it.
//
// None of that has a fragment of its own to be sorted by. An inline box is a
// fragment per line on LineFragment.Boxes and a run per piece of text on
// LineFragment.Runs, and what is written inside it hangs from its block's
// fragment as though the span were not there. So the paint used to put all of
// it at step 6 with the rest of the block's text, and the span's level moved
// nothing. What is here is the missing sort key: an inlineLevel stands for one
// such element, and holds everything the paint would otherwise have met
// piecemeal on its block's lines and among its block's children, so that the
// stacking context around it can sort it as one entry and paint it whole.
//
// # Which levels are inside which
//
// A mark belongs to the innermost inline level around it. A level is painted
// in the context of the innermost level around it that *seals* — makes a
// stacking context, by a z-index or an opacity — and in the context of the
// block's own stacking context where none does. That is the rule gather and
// hoist already follow for fragments: a "z-index: auto" positioned box is
// painted as a unit and the positioned boxes inside it are hoisted into the
// context around it. So a "position: relative" span inside another, neither
// with a z-index, is two levels side by side in the same context, and the
// inner one's words are not inside the outer one's unit; with "z-index: 1" on
// the outer one, the inner one is a level inside the outer one's context.
//
// Everything is worked out once, before the paint, by findInlineLevels, and
// kept per element: the pieces of a span broken around a block are copies of
// it in two anonymous blocks, and one span is one level whichever copy is met.

// inlineLevel is one non-atomic inline box that is painted at a stacking level
// of its own.
type inlineLevel struct {
	// box is the element's first box met, which carries its style, z-index
	// and position; every copy of the element carries the same ones.
	box *Box
	// up is the level whose stacking context this one is painted in, or nil
	// where that is the stacking context of the block the lines are in.
	up *inlineLevel
	// top is the outermost level in that chain: the one the block's own
	// context sorts, and that everything in this level is painted inside.
	top *inlineLevel
	// sub is every level whose up is this one, in the order they were met.
	sub []*inlineLevel
	// parts is what the level paints, in document order: the block fragments
	// whose lines carry its own marks, and the fragments it holds — what was
	// written inside it and hangs from a block, and the blocks lifted out of
	// it.
	parts []levelPart
	// linesOf and listedOn are the last block whose lines this level was added
	// as a part of and listed on, so that a block of a thousand words inside
	// one span makes one part and one entry. The pre-pass meets each block's
	// lines together, so the last one is the only one to compare against.
	// linesPart is the index in parts of that block's part.
	linesOf, listedOn *Fragment
	linesPart         int
}

// levelPart is one entry of inlineLevel.parts.
type levelPart struct {
	frag *Fragment
	// lines says the part is frag's lines — marks, the ones on them whose
	// innermost level is this one — and not frag itself.
	lines bool
	marks []lineMark
}

// lineMark is one mark on a block's lines: the inline box fragment at index
// box of line's Boxes, or where box is -1 the run at index run of its Runs.
type lineMark struct{ line, box, run int }

// seals reports whether the level makes a stacking context of its own.
func (l *inlineLevel) seals() bool { return sealsItsDescendants(l.box) }

// owns reports whether b is a box of the level's own element.
func (l *inlineLevel) owns(b *Box) bool {
	if b == l.box {
		return true
	}
	return b != nil && b.Element != nil && b.Element == l.box.Element && b.Pseudo == l.box.Pseudo
}

// levelKey is what makes two boxes one level: their element and which of its
// pseudo-elements, or the box itself for one no element generated.
type levelKey struct {
	el     *html.Node
	pseudo string
	box    *Box
}

func levelKeyOf(b *Box) levelKey {
	if b.Element == nil {
		return levelKey{box: b}
	}
	return levelKey{el: b.Element, pseudo: b.Pseudo}
}

// onALine reports whether a box is walked through on the way from a mark on a
// line to the block the line is in: a non-atomic, non-replaced inline box, or
// a run of text.
func onALine(b *Box) bool {
	return b.Outer == OuterInline && b.Replaced == nil && !isAtomicInline(b)
}

// innerLevelOf is the innermost inline level around what box b paints,
// counting b itself, or nil where there is none between b and its block.
//
// It is memoized per box, and every box on the walk is given the answer, so
// asking it of every run on every line is linear in the boxes and not in the
// runs times their depth. A text box is walked through and not counted: it
// carries its parent element's whole computed style, position and opacity
// included, and counting it would make a second level of one span.
func (p *painter) innerLevelOf(b *Box) *inlineLevel {
	var path []*Box
	var found *inlineLevel
	for cur := b; cur != nil && onALine(cur); cur = cur.Parent {
		if l, ok := p.innerLevels[cur]; ok {
			found = l
			break
		}
		path = append(path, cur)
		if !cur.IsText() && stacksAsLevel(cur) {
			found = p.levelFor(cur)
			break
		}
	}
	if len(path) > 0 && p.innerLevels == nil {
		p.innerLevels = map[*Box]*inlineLevel{}
	}
	for _, c := range path {
		p.innerLevels[c] = found
	}
	return found
}

// levelFor is the level of a stacking inline box's element, made the first
// time it is asked for.
func (p *painter) levelFor(b *Box) *inlineLevel {
	k := levelKeyOf(b)
	if l, ok := p.levels[k]; ok {
		return l
	}
	if p.levels == nil {
		p.levels = map[levelKey]*inlineLevel{}
	}
	l := &inlineLevel{box: b}
	p.levels[k] = l
	if around := p.innerLevelOf(b.Parent); around != nil {
		if around.seals() {
			l.up = around
		} else {
			l.up = around.up
		}
	}
	l.top = l
	if l.up != nil {
		l.up.sub = append(l.up.sub, l)
		l.top = l.up.top
	}
	return l
}

// levelHolding is the level that paints a fragment hanging from a block, or
// nil when the fragment is painted where the block's own context puts it.
//
// A block lifted out of an inline box is held by the innermost level it was
// lifted out of, and anything else by the innermost level it was written
// inside. What the level does with it depends on the fragment: one that
// stacks on its own account — positioned, or a stacking context — is sorted
// among the levels of the level's context if the level seals, and hoisted
// past it into the context around it if it does not, exactly as a positioned
// box inside a "z-index: auto" one is. Anything else is the level's content.
func (p *painter) levelHolding(c *Fragment) *inlineLevel {
	var in *inlineLevel
	if from := c.Box.splitFrom; len(from) > 0 {
		in = p.innerLevelOf(from[len(from)-1])
	} else {
		in = p.innerLevelOf(c.Box.Parent)
	}
	if in == nil || !stacksAsLevel(c.Box) || in.seals() {
		return in
	}
	return in.up
}

// outlineContext is the level whose stacking context paints the outline of an
// inline box's fragment, or nil for the context of the block it is on.
func (p *painter) outlineContext(b *Box) *inlineLevel {
	l := p.innerLevelOf(b)
	if l == nil || l.seals() {
		return l
	}
	return l.up
}

// findInlineLevels walks the fragment tree once, before the paint, and puts
// every mark and every fragment that an inline level paints into it.
//
// lineLevels records, per block, the outermost levels its lines and its
// children are painted inside, which is what a gather of that block adds to
// the context it is gathering for.
func (p *painter) findInlineLevels(f *Fragment) {
	if f == nil || f.Box == nil {
		return
	}
	for li := range f.Lines {
		line := &f.Lines[li]
		for i, bx := range line.Boxes {
			if bx != nil && bx.Box != nil {
				p.onLinesOf(f, p.innerLevelOf(bx.Box), lineMark{line: li, box: i, run: -1})
			}
		}
		for i := range line.Runs {
			if b := line.Runs[i].Box; b != nil {
				p.onLinesOf(f, p.innerLevelOf(b), lineMark{line: li, box: -1, run: i})
			}
		}
	}
	for _, c := range f.Children {
		if c == nil || c.Box == nil {
			continue
		}
		if h := p.levelHolding(c); h != nil {
			h.parts = append(h.parts, levelPart{frag: c})
		}
		p.findInlineLevels(c)
	}
}

// onLinesOf records that mark m on f's lines is l's own.
func (p *painter) onLinesOf(f *Fragment, l *inlineLevel, m lineMark) {
	if l == nil {
		return
	}
	if l.linesOf != f {
		l.linesOf, l.linesPart = f, len(l.parts)
		l.parts = append(l.parts, levelPart{frag: f, lines: true})
	}
	part := &l.parts[l.linesPart]
	part.marks = append(part.marks, m)
	if t := l.top; t.listedOn != f {
		t.listedOn = f
		if p.lineLevels == nil {
			p.lineLevels = map[*Fragment][]*inlineLevel{}
		}
		p.lineLevels[f] = append(p.lineLevels[f], t)
	}
}

// addLevel puts a level among the stacking levels of the context being
// gathered, once, and hoists what it does not seal in.
func (p *painter) addLevel(lv *layers, l *inlineLevel) {
	if lv.levels[l] {
		return
	}
	if lv.levels == nil {
		lv.levels = map[*inlineLevel]bool{}
	}
	lv.levels[l] = true
	z, _ := usedZIndex(l.box)
	lv.positioned = append(lv.positioned, stackLevel{level: l, z: z, key: p.orderKey(l.box)})
	if !l.seals() {
		p.hoistLevel(l, lv)
	}
}

// hoistLevel is hoist for a level that is painted as a unit: the stacking
// levels inside what it holds belong to the context around it.
//
// Its own lines need nothing here. A level inside it that does not seal is not
// one of its subs — it is painted in the same context, and the block's
// lineLevels already list it — and one that holds anything stacking on its own
// account has handed it to that context too (see levelHolding).
func (p *painter) hoistLevel(l *inlineLevel, lv *layers) {
	for _, pt := range l.parts {
		if !pt.lines {
			p.hoist(pt.frag, lv)
		}
	}
}

// gatherLevel is gather for an inline level: its parts sorted into Appendix
// E's layers, and, when it seals, the levels inside it.
func (p *painter) gatherLevel(l *inlineLevel, lv *layers, collect bool) {
	for _, pt := range l.parts {
		if pt.lines {
			lv.content = append(lv.content, contentItem{frag: pt.frag, scope: l, marks: pt.marks})
			continue
		}
		p.gatherOwn(pt.frag, lv, collect)
	}
	if collect {
		for _, s := range l.sub {
			p.addLevel(lv, s)
		}
	}
}

// paintLevel paints an inline level in the order a stacking context paints
// its root, or, for one that does not seal, the order unit paints a positioned
// box with "z-index: auto": the same layers without the levels, which were
// hoisted.
//
// Step 1, the root's own background and border, is the fragments of the
// element's own inline box on each line. The inline boxes inside it are its
// content and are painted with its words in step 6, in the line's order.
func (p *painter) paintLevel(l *inlineLevel) {
	seal := l.seals()
	lv := &layers{}
	p.gatherLevel(l, lv, seal)

	for _, pt := range l.parts {
		for _, m := range pt.marks {
			if m.box < 0 {
				continue
			}
			if box := pt.frag.Lines[m.line].Boxes[m.box]; l.owns(box.Box) {
				p.inlineDecorations(box, pt.frag.clipContent)
			}
		}
	}

	sortLevels(lv.positioned)
	at := 0
	for at < len(lv.positioned) && lv.positioned[at].z < 0 {
		p.stackLevel(lv.positioned[at])
		at++
	}
	for _, g := range lv.blocks {
		p.decorations(g)
	}
	for _, g := range lv.tables {
		p.paintCollapsed(g)
	}
	for _, g := range lv.floats {
		p.unit(g)
	}
	for _, g := range lv.content {
		p.contentItem(g)
	}
	for ; at < len(lv.positioned); at++ {
		p.stackLevel(lv.positioned[at])
	}
	if seal {
		p.levelOutlines(l)
	}
}

// levelOutlines is step 10 for a level that seals: the outlines of the inline
// box pieces whose outline context is this level, and of every fragment it
// holds that does not open a context of its own.
//
// The pieces are this level's marks and those of every level inside it that
// does not seal, which is painted in this context and whose outlines are
// therefore this context's too. Every mark is one level's, so none is met
// twice.
func (p *painter) levelOutlines(l *inlineLevel) {
	var visit func(x *inlineLevel)
	visit = func(x *inlineLevel) {
		for _, pt := range x.parts {
			if pt.lines {
				var boxes []*Fragment
				for _, m := range pt.marks {
					if m.box >= 0 {
						boxes = append(boxes, pt.frag.Lines[m.line].Boxes[m.box])
					}
				}
				p.outlinePieces(pt.frag, boxes, l)
				continue
			}
			if !opensAContext(pt.frag) {
				p.outlineWalk(pt.frag)
			}
		}
		for _, s := range x.sub {
			if !s.seals() {
				visit(s)
			}
		}
	}
	visit(l)
}

// # Order-modified document order
//
// Appendix E breaks a tie between two boxes at one stacking level by document
// order, and css-display-3 §3 changes the document for the children of a flex
// or grid container:
//
//	Flex and grid containers lay out their contents in order-modified document
//	order ... This also affects the painting order [CSS2], exactly as if the
//	flex/grid items were reordered in the source document. Absolutely-
//	positioned children of a flex/grid container are treated as having order:
//	0 for the purpose of determining their painting order relative to flex/grid
//	items.
//
// "As if reordered in the source" is about a whole item, everything inside it
// included, so the key of a box is not one number. It is where the box is
// among the children of each flex or grid container above it, outermost
// first, and then its own document position; two keys are compared at the
// first container they do not share a child of. Box.Order is document order
// and is still what decides wherever no container reorders anything.

// orderStep is one entry of a paint-order key: the child of container on the
// way to the box, its order value, and its document position. The last step
// of a key has no container and is the box's own document position.
type orderStep struct {
	container *Box
	order     int
	doc       int
}

// orderKey is a box's position in order-modified document order.
func (p *painter) orderKey(b *Box) []orderStep {
	return append(p.orderPrefix(b), orderStep{doc: b.Order})
}

// orderPrefix is the steps above a box: one for each flex or grid container it
// is inside, naming the child of that container it is inside. It is memoized,
// and a box with no such container above it has none and costs no slice.
func (p *painter) orderPrefix(b *Box) []orderStep {
	var path []*Box
	var prefix []orderStep
	for cur := b; cur != nil; cur = cur.Parent {
		if pre, ok := p.orderPrefixes[cur]; ok {
			prefix = pre
			break
		}
		path = append(path, cur)
	}
	if p.orderPrefixes == nil {
		p.orderPrefixes = map[*Box][]orderStep{}
	}
	for i := len(path) - 1; i >= 0; i-- {
		cur := path[i]
		if up := cur.Parent; up != nil && (up.Inner == InnerFlex || up.Inner == InnerGrid) {
			order := 0
			if isFlexOrGridItem(cur) {
				order = orderOf(cur)
			}
			// A copy, never an append to the parent's slice: two children of
			// one container would otherwise write their steps into the same
			// spare capacity.
			next := make([]orderStep, len(prefix), len(prefix)+1)
			copy(next, prefix)
			prefix = append(next, orderStep{container: up, order: order, doc: cur.Order})
		}
		p.orderPrefixes[cur] = prefix
	}
	return prefix
}

// compareOrder compares two paint-order keys.
func compareOrder(a, b []orderStep) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, y := a[i], b[i]
		if x.container != nil && x.container == y.container && x.order != y.order {
			if x.order < y.order {
				return -1
			}
			return 1
		}
		if x.doc != y.doc {
			if x.doc < y.doc {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}
