package layout

import (
	"sort"
	"strings"

	"github.com/mgilbir/forme/bidi"
	"github.com/mgilbir/forme/style"
)

// Bidirectional text: the direction and unicode-bidi properties, and the
// reordering they ask for.
//
// # Where the algorithm runs, and where the reordering does
//
// UAX #9 resolves an embedding level per character over a *paragraph*, and CSS
// makes a block container's inline content one — split at every forced line
// break, because a <br> ends a paragraph as surely as a newline in plain text
// does. The reordering is then applied per *line*, after the breaking, and that
// separation is the whole reason this is not a pass over a string: which spaces
// are reset by rule L1, and which stretch is reversed by rule L2, both depend on
// where the line ended, which nothing knows until the line has been filled.
//
// So the work is in three places and each is the only one that could do its
// part:
//
//   - collectInline builds the paragraph as it flattens the tree, because that
//     is the one walk that sees the inline boxes in document order — and the
//     boxes are where unicode-bidi's formatting codes come from.
//   - resolveBidi runs the algorithm and splits the items at level boundaries,
//     so that every item on a line has one direction. An item is the atom the
//     line is built from; one spanning a level boundary could not be placed.
//   - inlineContent reorders the items of each line as it positions them.
//
// # Why the runs and not the text
//
// The reordering is over *runs*, not over the characters inside one. Each run is
// set by forme, which applies the algorithm to the string it is given and hands
// back glyphs in the order they are drawn — so a Hebrew word comes back already
// reversed and with its brackets mirrored, and the levels resolved here decide
// only where each run goes on the line. Reversing the text as well would undo it.
//
// The one thing the shaper cannot know is what a run's direction is when the run
// has no strong character in it: a lone bracket between two Hebrew words is
// right-to-left because of its neighbours, and the neighbours are in other runs
// by then. That is why a right-to-left run is drawn with an explicit override in
// front of it — see paint.go.

func bidiModeOf(b *Box) bidiMode {
	switch strings.ToLower(strings.TrimSpace(b.Style["unicode-bidi"])) {
	case "embed":
		return bidiEmbed
	case "isolate":
		return bidiIsolate
	case "bidi-override":
		return bidiOverride
	case "isolate-override":
		return bidiIsolateOverride
	case "plaintext":
		return bidiPlaintext
	}
	return bidiNormal
}

// isRTL reads the direction property. It is inherited, so every box has an
// answer and the initial one is left to right.
func isRTL(b *Box) bool {
	return strings.EqualFold(strings.TrimSpace(b.Style["direction"]), "rtl")
}

// bidiControls is the pair of formatting codes an inline box stands for.
//
// CSS Writing Modes defines unicode-bidi by naming the codes, which is why this
// is a table rather than a mechanism: "embed" *is* an LRE or an RLE with a
// matching PDF, and "isolate" *is* an LRI or an RLI with a matching PDI. Writing
// it any other way would be inventing a second definition of a property that
// already has one.
func bidiControls(b *Box) (open, close []rune) {
	rtl := isRTL(b)
	switch bidiModeOf(b) {
	case bidiEmbed:
		if rtl {
			return []rune{runeRLE}, []rune{runePDF}
		}
		return []rune{runeLRE}, []rune{runePDF}
	case bidiIsolate:
		if rtl {
			return []rune{runeRLI}, []rune{runePDI}
		}
		return []rune{runeLRI}, []rune{runePDI}
	case bidiOverride:
		if rtl {
			return []rune{runeRLO}, []rune{runePDF}
		}
		return []rune{runeLRO}, []rune{runePDF}
	case bidiIsolateOverride:
		// Both, in that order: the isolate keeps the box's contents from
		// affecting the text around it, and the override forces a direction on
		// everything inside. <bdo dir=rtl> is the element this exists for.
		if rtl {
			return []rune{runeRLI, runeRLO}, []rune{runePDF, runePDI}
		}
		return []rune{runeLRI, runeLRO}, []rune{runePDF, runePDI}
	case bidiPlaintext:
		// A first-strong isolate: the contents decide their own direction, which
		// is what "plaintext" means and what <bdi> and dir=auto ask for.
		return []rune{runeFSI}, []rune{runePDI}
	}
	return nil, nil
}

// paragraphDirection is the base direction of a block container's inline
// content, and the controls its own unicode-bidi wraps that content in.
//
// The two are separate because they answer different halves of the property.
// "direction" sets the paragraph embedding level; "unicode-bidi" says what
// happens to what is inside it, and for a block container only two of its values
// have any effect — an override applies to the whole paragraph, and plaintext
// hands the direction back to the text.
func paragraphDirection(b *Box) (bidi.Direction, []rune, []rune) {
	dir := bidi.LeftToRight
	if isRTL(b) {
		dir = bidi.RightToLeft
	}
	switch bidiModeOf(b) {
	case bidiOverride, bidiIsolateOverride:
		if dir == bidi.RightToLeft {
			return dir, []rune{runeRLO}, []rune{runePDF}
		}
		return dir, []rune{runeLRO}, []rune{runePDF}
	case bidiPlaintext:
		// Rules P2 and P3 over each paragraph of the content. The direction
		// property is ignored, which is what makes this the right value for
		// text whose language the author does not know.
		return bidi.Auto, nil, nil
	}
	return dir, nil, nil
}

// resolveBidi runs the algorithm over the context's paragraphs and splits the
// items so that each has a single embedding level.
//
// Splitting is what makes the rest of inline layout able to ignore all of this.
// A line is built from items and positioned item by item, so an item that
// straddled a level boundary — "abcHEBREW" with no space in it — could not be
// placed at all: half of it belongs at one end of the line and half at the
// other.
func (l *layouter) resolveBidi(b *Box, items []inlineItem, p *bidiBuilder) []inlineItem {
	if p == nil || len(items) == 0 {
		return items
	}
	dir, _, _ := paragraphDirection(b)
	if dir == bidi.LeftToRight && !p.Needed {
		// Every level is zero, the visual order is the logical one, and there is
		// nothing for the reordering to do. This is the common case by a wide
		// margin, and skipping it here is what keeps a page of Latin text from
		// paying for a feature it does not use.
		//
		// The insets are still settled, because which of a box's two of them
		// begins it is §8.6's question about the *element's* direction and not
		// about any character in the paragraph — a "direction: rtl" span holding
		// Latin text reorders nothing and still begins at its right. Skipping
		// this left CSS2/box/rtl-span-only reserving room on the side it was not
		// drawing on.
		insetSides(items)
		return items
	}

	// "unicode-bidi: plaintext" hands the direction to the text, and a paragraph
	// with no strong character in it has nothing to hand over. css-writing-modes
	// says where it comes from instead — the paragraph before it, and the
	// containing block where there is none — which is what a plain text editor
	// does and is what the value is named after.
	//
	// P3's own answer is "otherwise, set it to zero", and that is the right one
	// for text nobody has any other information about. Here there is other
	// information, so P2 is asked without P3 and the answer is carried forward.
	// bidi-lines-002 writes it out: five lines of a right-to-left block, three of
	// them nothing but "!", and the reference puts each of those three on the
	// side the line before it was on.
	carry := dir
	if dir == bidi.Auto {
		carry = bidi.LeftToRight
		if isRTL(b) {
			carry = bidi.RightToLeft
		}
	}
	resolved := make([]*bidi.Paragraph, len(p.Paras))
	for i, text := range p.Paras {
		if len(text) == 0 {
			// Nothing to resolve and nothing to carry: a paragraph with no
			// characters has no strong one either, so it neither takes the
			// direction nor changes it.
			continue
		}
		use := dir
		if dir == bidi.Auto {
			if d, found := bidi.FirstStrong(text); found {
				carry = d
			}
			use = carry
		}
		resolved[i] = bidi.Resolve(text, use)
	}

	out := make([]inlineItem, 0, len(items))
	for _, item := range items {
		if item.BidiPara < 1 || item.BidiPara > len(resolved) {
			out = append(out, item)
			continue
		}
		para := resolved[item.BidiPara-1]
		if para == nil {
			out = append(out, item)
			continue
		}
		out = append(out, l.splitByLevel(item, para)...)
	}
	insetSides(out)
	return out
}

// insetSides is CSS 2.1 §8.6: an inline box's margin, border and padding go on
// the physical side they are named for, whichever way its content reads.
//
// # The rule, and why it is not the direction property
//
// §8.6 says it twice, once per direction, and both halves put the left inset on
// "the leftmost generated box" of the element and the right inset on "the
// rightmost". What the direction property changes is only which *line box* those
// two are looked for on when the element is broken across several — the first or
// the last. On a single line it changes nothing at all.
//
// So the question this has to answer is not "what did direction say" but "was
// this box's content reversed", and only the algorithm knows that. A
// "direction: rtl" span with the initial "unicode-bidi: normal" is not reversed —
// the property is inert on an inline box without an embedding, by design, since a
// box that changed the direction without opening one would reorder text outside
// itself. Swapping on the property was tried and cost nine clean passes.
//
// # How it is done
//
// Two things are decided per box, and both are about the box's content rather
// than about anything declared on it.
//
// The first is whether the two items should swap what they hold. They are
// emitted in logical order with the left inset first; a box whose content
// resolves to an odd level *throughout* will have that content reversed by the
// reordering and the two items reversed with it, so their widths are exchanged
// here. The item that will be drawn last is given the inset that belongs on the
// right, and the one that will be drawn first the inset that belongs on the
// left. Nothing else about either item moves, so line breaking still sees the
// box's leading edge where the box begins.
//
// "Throughout" is the strict reading and it is deliberate. A box whose content
// resolves to more than one level has ends that need not be at its visual edges
// at all — its leftmost generated box may be neither of them — and §8.6 then
// asks for something two items cannot express. Guessing from the first item's
// level is measurably worse: css-text/white-space's tab-bidi-001 has an outer
// span holding a right-to-left <bdo> followed by left-to-right text, and
// swapping on the <bdo>'s level put the span's left border three pixels inside
// where the reference draws it.
//
// The second is the level the two items are *reordered* at, which they need
// because they carry no characters and so the algorithm gave them none. It is
// the lowest level anything inside the box reached. An embedding raises the
// level of what is inside it and leaves the box's own boundary outside, so the
// lowest is the box's own — and both other candidates were tried and are wrong:
//
//   - the neighbouring item's level glues the box's edge to whatever run abuts
//     it, which is the tab-bidi-001 fault again, one border out of place;
//   - the paragraph's base level detaches the edge from its own content, which
//     costs bidi-span-003: a purple-bordered <span> of Latin in a
//     "direction: rtl" div had its opening border thrown to the far end of the
//     line, so the border drawn round one word enclosed two.
//
// # Cost
//
// One pass and no allocation beyond the stack of inline boxes currently open.
// Every question is answered in constant time per box: the counts are subtracted
// from the running totals at the close, and the minimum is folded into the
// enclosing box's as each one pops, so no box ever rescans its own content. The
// stack's depth is the inline nesting, which the HTML parser caps at 256.
func insetSides(items []inlineItem) {
	// noLevel is above every level UAX #9 can produce: MaxDepth is 125 and rule
	// X8 admits one more.
	const noLevel = -1

	// open is one inline box whose lead inset has been seen and whose trail
	// inset has not.
	type open struct {
		box  *Box
		lead int
		// content and odd are the running counts at the moment the box opened.
		// Subtracting them at the close gives the box's own.
		//
		// They are counts rather than a level and a "have we got one yet" flag
		// because zero is a real embedding level — the left-to-right one — so a
		// box that has seen no content and a box whose content is left-to-right
		// have to stay distinguishable. Counting keeps them apart without a
		// sentinel: no content is a count of zero, which swaps nothing.
		content, odd int
		// min is the lowest level seen inside this box so far, or noLevel.
		min int
	}
	var stack []open
	content, odd := 0, 0

	for i := range items {
		if items[i].Inset {
			if items[i].InsetLead {
				stack = append(stack, open{
					box: heldBox(items[i].Box), lead: i,
					content: content, odd: odd, min: noLevel,
				})
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1].box != items[i].Box {
				// The pair is emitted together by insetItems and nested by the
				// recursion that emits it, so this cannot happen. It is checked
				// because the alternative to skipping is swapping two unrelated
				// boxes' insets on a document nobody wrote deliberately.
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// A box's content counts as its parent's too, so the minimum folds
			// outwards as the stack unwinds rather than being recomputed.
			if len(stack) > 0 && top.min < stack[len(stack)-1].min {
				stack[len(stack)-1].min = top.min
			}
			// Which of the box's two physical insets belongs to the end that
			// *begins* it. §8.6 answers with the element's own direction, and
			// that is what beginsAtRight reads.
			//
			// This used to answer with the box's content instead — swap when
			// everything inside resolved to an odd level — which agrees on a box
			// whose direction and content run the same way and on nothing else.
			// A "direction: rtl" span holding Latin text is the case it got
			// wrong, and CSS2/box/rtl-basic is that document.
			if beginsAtRight(heldBox(items[top.lead].Box)) {
				items[top.lead].Width, items[i].Width =
					items[i].Width, items[top.lead].Width
			}
			if top.min != noLevel {
				items[top.lead].InsetLevel, items[top.lead].InsetLevelKnown = top.min, true
				items[i].InsetLevel, items[i].InsetLevelKnown = top.min, true
			}
			continue
		}
		if items[i].Para == nil {
			// No characters of its own: a float marker, or a run in a paragraph
			// the algorithm did not resolve. It says nothing about direction.
			continue
		}
		content++
		if items[i].Level&1 == 1 {
			odd++
		}
		if len(stack) > 0 && items[i].Level < stack[len(stack)-1].min {
			stack[len(stack)-1].min = items[i].Level
		}
	}
}

// placeInsetsBySide is §8.6's other half: an inset is *reserved* on the physical
// side it is drawn on.
//
// The two are separate mechanisms here and have to be made to agree. Which side
// is drawn is settled per line, by inlineDecor.finish, from the box's own
// direction. Which side is reserved is settled by where the inset item lands in
// the visual order — and the item is emitted at the box's logical start, so it
// lands wherever the *content's* bidi order puts it, which is a different
// question with a different answer.
//
// A "direction: rtl" span holding Latin text in a left-to-right line is the case
// where they part: nothing about the line is reordered, so the item stays at the
// left of the words, while §8.6 draws the box's starting inset at their right.
// The reference for CSS2/box/rtl-span-only puts "One" hard against the block's
// left edge with its border and margin beyond it.
//
// So the offsets are corrected after the line is ordered. For each box with an
// inset on this line, the inset belongs at one end of that box's own visual
// extent: the left end if it is the box's left inset, the right end if it is the
// right one. Where it is already there nothing moves; where it is at the other
// end the group is rotated — the content slides over by the inset's width and
// the inset takes the far end. The line's total width does not change, because
// the same items occupy the same span.
//
// It is deliberately narrow. A box whose items are not contiguous in visual
// order, or whose inset is somewhere in the middle of them, is left exactly as
// the reordering placed it: those are the shapes this rotation is not an answer
// for, and moving something in them would be a guess.
func (l *layouter) placeInsetsBySide(runs []inlineItem, order []int) []int {
	// The boxes with an inset on this line, innermost first. An inner box has to
	// be arranged before the box around it, because once it is, its own insets
	// are part of what the outer box's extent has to enclose.
	var boxes []*Box
	seen := map[*Box]bool{}
	for _, k := range order {
		if b := heldBox(runs[k].Box); runs[k].Inset && b != nil && !seen[b] {
			seen[b] = true
			boxes = append(boxes, b)
		}
	}
	sort.SliceStable(boxes, func(a, c int) bool {
		return boxDepth(boxes[a]) > boxDepth(boxes[c])
	})

	for _, b := range boxes {
		// The box's own two insets come out of the order, and everything else
		// keeps the order the reordering gave it.
		lead, trail := -1, -1
		rest := make([]int, 0, len(order))
		for _, k := range order {
			if runs[k].Inset && runs[k].Box == b {
				if runs[k].InsetLead {
					lead = k
				} else {
					trail = k
				}
				continue
			}
			rest = append(rest, k)
		}
		// Where the box's content sits in what is left. Its own inner boxes'
		// insets count as its content, which is what makes the nesting come out
		// right: an outer border encloses an inner margin.
		lo, hi := -1, -1
		for i, k := range rest {
			if !itemInside(runs[k], b) || runs[k].Width == 0 {
				// An item with no width draws nothing, so it is not one of the
				// boxes §8.6 puts an inset at the end of — and letting one stand
				// for the box's edge strands the inset away from the words. A
				// bidi control the author wrote inside a span is such an item,
				// and bidi-011 has one at the far end of the line from the span
				// it belongs to.
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
		if lo < 0 {
			// Nothing of the box's own on this line, so there is no extent to
			// put anything at the ends of. The order stands.
			continue
		}
		// insetSides has already given the lead item the width of whichever
		// physical side begins the box, so the lead goes at that side.
		left, right := lead, trail
		if beginsAtRight(b) {
			left, right = trail, lead
		}
		out := make([]int, 0, len(order))
		out = append(out, rest[:lo]...)
		if left >= 0 {
			out = append(out, left)
		}
		out = append(out, rest[lo:hi+1]...)
		if right >= 0 {
			out = append(out, right)
		}
		out = append(out, rest[hi+1:]...)
		order = out
	}
	return order
}

// boxDepth is how many boxes a box sits inside, which orders a line's inline
// boxes innermost first.
func boxDepth(b *Box) int {
	n := 0
	for c := b; c != nil; c = c.Parent {
		n++
	}
	return n
}

// itemInside reports whether an item's content sits inside an inline box.
//
// It walks the box tree rather than asking inlineChain, and the difference
// matters: inlineChain keeps only the boxes that have something to *paint*, so a
// span with a margin and no border is not in it — and a span with a margin and
// no border is exactly the shape bidi-box-model-011 is built from. Asking the
// painting question here left such a box's own words out of its group, so the
// group was two insets with a word between them that belonged to nobody, and
// the rearrangement below declined to touch it.
func itemInside(item inlineItem, b *Box) bool {
	start := heldBox(item.Box)
	if item.AtomicBox != nil && start != nil {
		start = start.Parent
	}
	for c := start; c != nil; c = c.Parent {
		if c == b {
			return true
		}
	}
	return false
}

// splitByLevel cuts one item where its characters change embedding level.
func (l *layouter) splitByLevel(item inlineItem, para *bidi.Paragraph) []inlineItem {
	levels := para.Levels()
	if item.BidiEnd > len(levels) || item.BidiStart >= item.BidiEnd {
		item.Para = para
		return []inlineItem{item}
	}
	item.Para = para
	item.Level = levels[item.BidiStart]

	// Almost every item is one level throughout — a word is one direction — so
	// the scan for a boundary is the whole cost in the usual case.
	uniform := true
	for i := item.BidiStart + 1; i < item.BidiEnd; i++ {
		if levels[i] != levels[item.BidiStart] {
			uniform = false
			break
		}
	}
	if uniform || item.Text == "" {
		return []inlineItem{item}
	}

	runes := []rune(item.Text)
	if len(runes) != item.BidiEnd-item.BidiStart {
		// The item's text and its place in the paragraph disagree, which can only
		// mean the two were built from different text. Splitting on the levels
		// would cut the string at the wrong place, so the item is left whole at
		// the level of its first character — wrong order rather than wrong text.
		//
		// Instrumented over the suite it never fires: every range is built by
		// bidiBuilder.add from the very text it describes, and splitItem moves
		// text and range together. So this is defence against a range that has
		// drifted rather than a case the layout produces — and the danger it
		// guards is that such drift is otherwise silent, since giving up on the
		// reordering looks like text that simply had one direction.
		return []inlineItem{item}
	}

	var out []inlineItem
	for i := 0; i < len(runes); {
		j := i + 1
		for j < len(runes) && levels[item.BidiStart+j] == levels[item.BidiStart+i] {
			j++
		}
		piece := item
		piece.Text = string(runes[i:j])
		piece.BidiStart = item.BidiStart + i
		piece.BidiEnd = item.BidiStart + j
		piece.Level = levels[item.BidiStart+i]
		piece.Width = l.br.MeasureSpaced(item.Face, piece.Text, item.Size, item.Spacing)
		if i > 0 {
			// A level boundary is not a break opportunity. "abcHEBREW" is one
			// word however many directions it is written in, and a line must not
			// be allowed to end inside it.
			piece.BreakBefore = false
		}
		out = append(out, piece)
		i = j
	}
	return out
}

// lineOffsets is where each of a line's items sits from the line's left edge,
// together with the pen position at the end of it.
//
// The reordering is applied to the *positions* and not to the slice, and that is
// deliberate. Painting reads a run's own X, so the order of the slice makes no
// difference to the page — but it is also the order the runs are written into
// the content stream, which is the order a reader extracts the text in. Keeping
// the slice in logical order is what lets a right-to-left paragraph be drawn the
// way it reads and copied out the way it was written; reordering the slice would
// have traded the second for nothing.
func (l *layouter) lineOffsets(runs []inlineItem) ([]style.Unit, style.Unit) {
	order := lineVisualOrder(runs)
	if order == nil {
		order = make([]int, len(runs))
		for i := range order {
			order[i] = i
		}
	}
	// §8.6 wants each inline box's inset at the ends of *its own* extent, which
	// is a statement about the visual order and so is made here, before anything
	// is measured out. See placeInsetsBySide.
	order = l.placeInsetsBySide(runs, order)

	shift := insetsBesideTheGlyphs(runs, order)

	xs := make([]style.Unit, len(runs))
	var x style.Unit
	for _, k := range order {
		xs[k] = x.Add(shift[k])
		x = x.Add(runs[k].Width)
	}
	return xs, x
}

// insetsBesideTheGlyphs moves an inline box's own edge off the far side of the
// letter-spacing gap and up against the glyphs it belongs to.
//
// §8.2's gap goes *between* two typographic character units, so it belongs to
// neither of the boxes they are in: a box's border and padding are part of the
// box and the gap is not, and the order along the line is
//
//	glyphs · the box's ending edge · the gap · the next box's beginning edge
//
// A run's advance holds the gap after its last character, at its right edge
// whichever way the run reads — see gapNeighbour, which is where that is
// established — so the edge that follows it was being placed a gap's width too
// far along, outside its own box's ink with the gap between the border and the
// letter it is drawn against. letter-spacing-nesting-003 asks for the other
// order in as many words: "the green rectangle does not extend beyond B to C".
//
// The pen is not moved, only the edge: an inset shifted back by the gap leaves a
// hole of exactly that width after it, which is where the gap now is. So nothing
// downstream of the box changes position and the line is the width it was.
//
// Whether a given edge is the one to move is the *box*: an edge whose box the
// run before it is inside is that run's own ending edge and belongs against its
// glyphs, and one whose box it is not is the next box beginning and belongs on
// the far side of the gap.
func insetsBesideTheGlyphs(runs []inlineItem, order []int) []style.Unit {
	shift := make([]style.Unit, len(runs))
	prev := -1
	for _, k := range order {
		if !runs[k].Inset {
			if isSpacedRun(runs[k]) {
				prev = k
			} else {
				prev = -1
			}
			continue
		}
		if prev >= 0 && itemInside(runs[prev], heldBox(runs[k].Box)) {
			shift[k] = shift[k].Sub(runs[prev].EdgeLetterSpacing)
		}
	}
	return shift
}

// lineBaseIsRTL is the inline base direction a line was set in.
//
// It is the block's own direction almost always, and the exception is the reason
// this is not simply isRTL: under "unicode-bidi: plaintext" each bidi paragraph
// decides its own direction from its first strong character, so two lines of one
// block can differ — and "text-align: start", which is the initial value,
// resolves against the line's and not the block's.
func lineBaseIsRTL(b *Box, runs []inlineItem) bool {
	for _, item := range runs {
		if item.Para != nil {
			return item.Para.Level()&1 == 1
		}
	}
	return isRTL(b)
}
