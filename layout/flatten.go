package layout

import (
	"strings"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// Turning an inline subtree into the runs a line is built from.
//
// The tree is flattened because a line break can fall anywhere, including inside
// an <em> — so what goes on a line is a sequence of runs and not a sequence of
// boxes. What each run carries is therefore everything about its box that the
// breaking or the painting will still need once the box itself is gone.
//
// An atomic inline is the exception that shapes the rest: an inline-block or a
// replaced element is laid out *here*, before the line it will sit on has been
// chosen, because nothing about that line can change its size.

// isAtomicInline reports whether an inline-level box takes part in a line as a
// box rather than as a run of words.
//
// An inline-block is one. That is the whole of what "inline-block" means and it
// is easy to under-implement: an engine that walked into it the way it walks
// into a <span> would flatten its content onto the surrounding line, and the
// box's own width, height, background, border and padding would all quietly do
// nothing — a shape of failure that looks like the declarations were ignored
// rather than like the box was.
//
// An inline table and an inline flex container are atomic too, and are
// deliberately not here: neither has a layout to be atomic *with* yet, and
// giving them a box before they have contents to put in it would produce an
// empty rectangle where the author expected a table.
func isAtomicInline(b *Box) bool {
	return b.Outer == OuterInline && b.Inner == InnerFlowRoot
}

// atomicItem lays out an atomic inline and makes the line item for it.
//
// The item may always begin a line. CSS Text §5.1 says to treat an atomic inline
// as U+FFFC OBJECT REPLACEMENT CHARACTER for the purpose of line breaking, and
// UAX #14 puts that character in class CB, whose rule LB20 is "break before and
// after unresolved CB" — so a picture is never welded to the word beside it, and
// two pictures side by side are two units rather than one. What comes *after*
// the item is the caller's business, because the rule there has an exception;
// see where the state is set.
func (l *layouter) atomicItem(b *Box, frame inlineFrame) inlineItem {
	item := inlineItem{
		Box: b, AtomicBox: b, Size: b.FontSize,
		BreakBefore: true, Offset: frame.Offset,
	}
	if frame.Measuring {
		// No fragment: the caller wants a width, and the widths of an atomic
		// inline are what intrinsic.go computes from the box tree.
		return item
	}

	var frag *Fragment
	if b.Replaced != nil {
		frag = l.replacedFragment(b, frame)
	} else {
		frag = l.inlineBlockFragment(b, frame)
	}
	box := frag.MarginRect()
	item.Atomic = frag
	item.Width = box.W
	item.Ascent, item.Descent = box.H, 0
	item.Valign = l.vAlignFor(b, frame.Valign)

	// §10.8.1: an inline-block's baseline is the baseline of its last in-flow
	// line box. With no line box at all it is the bottom margin edge — which is
	// also a replaced element's, so the value set above already says so.
	//
	// An overflow that is not visible does not simply fall back to the bottom
	// margin edge, which is what CSS 2.1 said and what CSS 2.2 corrected: it is
	// the *higher* of the two candidates. The correction matters because the
	// 2.1 rule made "overflow: auto" on a one-line box drop the whole box below
	// its neighbours' baseline, which is a visible jump from a declaration that
	// was only ever about clipping.
	if b.Replaced == nil {
		baseline, ok := lastLineBaseline(frag)
		if b.TableWrapper {
			// §10.8.1 again, and a different sentence of it: "the baseline of an
			// 'inline-table' is the baseline of the first row of the table".
			//
			// The first row and not the last line box, which is the rule for an
			// inline-block and the one an inline-table was sharing. A table of
			// two rows therefore sat on its *second* one: the word beside it
			// lined up with the bottom row and the rest of the table hung above
			// the text, which is inline-table-002a and its neighbours.
			//
			// The wrapper is what arrives here — §17.4 puts one around every
			// table, and for an inline-table it is the atomic inline — so the
			// search starts outside the table and finds the first line box in
			// it, which is in the first cell of the first row.
			baseline, ok = firstBaseline(frag)
		}
		if ok {
			bl := baseline
			ascent := frag.Margin.Top.Add(bl)
			if overflowIsScrollable(b.Style) {
				// A smaller ascent is a baseline further up the page, which is
				// what "higher" means here.
				ascent = style.Min(ascent, box.H)
			}
			item.Ascent = ascent
			item.Descent = box.H.Sub(ascent)
		}
	}
	return item
}

// replacedFragment lays out an inline-level replaced box.
//
// Its margins are its own — an "auto" margin on an inline-level box is zero
// rather than a share of anything, which l.edges already produces — and its
// size comes from §10.3.2. What it does not get here is a position: that is the
// line's to decide.
func (l *layouter) replacedFragment(b *Box, frame inlineFrame) *Fragment {
	margin := l.edges(b, "margin", frame.Containing)
	border := l.borderWidths(b)
	padding := l.edges(b, "padding", frame.Containing)
	size := l.replacedSize(b, frame.Containing, frame.CbHeight, frame.CbDefinite)

	frag := &Fragment{
		Box: b, Margin: margin, Border: border, Padding: padding,
		Outline: l.outlineWidth(b),
		BorderRect: Rect{
			W: size.W.Add(padding.Horizontal()).Add(border.Horizontal()),
			H: size.H.Add(padding.Vertical()).Add(border.Vertical()),
		},
		// §9.4.3's offset, accumulated over the inline boxes around it, plus
		// its own when it is itself relatively positioned. It travels on the
		// fragment rather than being folded into the position for the reason
		// layout.go gives: the box still occupies where the flow put it.
		Offset: frame.Offset,
	}
	if b.Position == PositionRelative {
		d := l.relativeOffset(b, frame.Containing, frame.CbHeight, frame.CbDefinite)
		frag.Offset = Point{X: frame.Offset.X.Add(d.X), Y: frame.Offset.Y.Add(d.Y)}
	}
	if b.Position.positioned() {
		// §10.1 makes any positioned box a containing block, and an image with
		// "position: relative" is the everyday way to hang a caption on one.
		l.positioned[b] = frag
	}
	return frag
}

// inlineBlockFragment lays out an inline-block.
//
// Its width is CSS 2.1 §10.3.9's: shrink-to-fit, the same formula a float uses,
// against whatever the containing block leaves after its own margins, border
// and padding. Everything else is ordinary block layout, run through blockIn
// with the width handed to it — margin collapsing inside, floats contained,
// line breaking, list markers and the height rules are all the same, and a
// second implementation of them would agree with the first on the day it was
// written and on no day after.
func (l *layouter) inlineBlockFragment(b *Box, frame inlineFrame) *Fragment {
	margin := l.edges(b, "margin", frame.Containing)
	border := l.borderWidths(b)
	padding := l.edges(b, "padding", frame.Containing)

	width, ok := l.explicitWidth(b, frame.Containing)
	if !ok {
		room := frame.Containing.
			Sub(margin.Horizontal()).
			Sub(border.Horizontal()).
			Sub(padding.Horizontal())
		width = l.shrinkToFit(b, maxZero(room))
	}
	width = l.clampWidth(b, width, frame.Containing)

	// A fresh formatting context, because an inline-block establishes one:
	// no float inside it escapes and none outside reaches in. That is not a
	// choice made here — it is what "flow-root" means, and blockIn would make
	// one anyway for a box that seals its margins.
	frag := outOfClamp(l, func() *Fragment {
		f, _ := l.blockIn(b, frame.Containing,
			flow{ctx: &floatContext{}, cbHeight: frame.CbHeight, cbDefinite: frame.CbDefinite},
			&forcedGeometry{margin: margin, width: width})
		return f
	})
	if b.Position == PositionRelative {
		d := l.relativeOffset(b, frame.Containing, frame.CbHeight, frame.CbDefinite)
		frag.Offset = Point{X: frame.Offset.X.Add(d.X), Y: frame.Offset.Y.Add(d.Y)}
	} else {
		frag.Offset = frame.Offset
	}
	return frag
}

// lastLineBaseline finds the baseline of the last line box in a subtree, as a
// distance from the top of that subtree's border box.
//
// "Last in the normal flow" is what §10.8.1 asks for, so the walk goes
// backwards through the children and skips everything out of flow: a float at
// the end of an inline-block does not give the box its baseline, and neither
// does an absolutely positioned caption hanging off it.
func lastLineBaseline(f *Fragment) (style.Unit, bool) {
	inset := f.Border.Top.Add(f.Padding.Top)
	for i := len(f.Children) - 1; i >= 0; i-- {
		c := f.Children[i]
		if c.Box == nil || c.Box.outOfFlow() {
			continue
		}
		if bl, ok := lastLineBaseline(c); ok {
			return inset.Add(c.BorderRect.Y).Add(bl), true
		}
	}
	if n := len(f.Lines); n > 0 {
		line := f.Lines[n-1]
		return inset.Add(line.Rect.Y).Add(line.Baseline), true
	}
	return 0, false
}

// collectInline flattens an inline subtree into measurable items.
//
// The tree is flattened because a line break can fall anywhere, including inside
// an <em> — so what goes on a line is a sequence of runs, not a sequence of
// boxes. Each item keeps the box it came from, which is what painting needs to
// know its colour.
func (l *layouter) collectInline(b *Box, out []inlineItem, state inlineState, frame inlineFrame) ([]inlineItem, inlineState) {
	for _, child := range b.Children {
		if child.Position.outOfFlow() {
			// Out of flow and, unlike a float, out of the way: it takes no width
			// on the line, breaks nothing and shortens nothing. All that is kept
			// is where it was written, which is its static position.
			out = append(out, inlineItem{Abs: child})
			continue
		}
		if child.Float != FloatNone {
			// Out of flow, so it neither takes width on the line nor breaks it:
			// it is recorded where it was written and placed when the line it
			// belongs to is known. The state passes straight through, because
			// "a <span class=float></span>b" is still one word followed by
			// another with a space between them.
			out = append(out, inlineItem{Float: child})
			continue
		}
		if child.Replaced != nil || isAtomicInline(child) {
			// One neutral character in the paragraph, before the item is built,
			// so that the ordering sees a picture between two words as something
			// that is there rather than as a gap between them.
			para, start, end := frame.Bidi.Object()
			// An atomic inline: a replaced element, or an inline-block. It is
			// one unbreakable thing with a size of its own, and it is laid out
			// here — before the line it will sit on has even been chosen —
			// because nothing about that line can change its size. That is the
			// whole difference between an atomic inline and an ordinary inline
			// box, whose extent is whatever its words turn out to need and
			// which therefore has to be flattened into the run.
			item := l.atomicItem(child, frame)
			item.BidiPara, item.BidiStart, item.BidiEnd = para, start, end
			out = append(out, item)
			// LB20's other half: a line may also begin after the picture, so
			// "<img/><img/>" is two units and the second wraps when it does not
			// fit. What this is not is an opportunity for a *space* to take —
			// LB7 says not to break before one and is the earlier rule, so the
			// space stays with the picture and the break falls after the space
			// where LB18 puts it. itemsFor is where that exception lives, since
			// it is the only place that knows a piece is a space.
			//
			// The distinction is visible rather than theoretical: a float after
			// "<img/> " goes on the line after the picture, and offering the
			// opportunity to the space put it one line further down again.
			state.BreakOpportunity = true
			state.AfterCollapsibleSpace = false
			continue
		}
		if child.IsText() {
			var items []inlineItem
			items, state = l.itemsFor(child, state, frame)
			out = append(out, items...)
			continue
		}
		if child.Element != nil && strings.EqualFold(child.Element.Name, "br") {
			// A line break the author wrote. It is not a break *opportunity* —
			// it ends the line wherever it falls, even mid-word and even on a
			// line with room to spare.
			out = append(out, inlineItem{Box: child, Forced: true})
			// It ends a bidi paragraph too: CSS makes a forced break a paragraph
			// separator, so the direction of what follows is decided afresh
			// rather than by the first strong character of the block.
			frame.Bidi.BreakParagraph()
			// What follows is at the start of a line, so a collapsible space
			// there is removed rather than indenting it.
			state = startOfContext()
			continue
		}
		if child.Outer == OuterInline {
			inner := frame
			if child.Position == PositionRelative {
				d := l.relativeOffset(child, frame.Containing, frame.CbHeight, frame.CbDefinite)
				inner.Offset = Point{
					X: frame.Offset.X.Add(d.X),
					Y: frame.Offset.Y.Add(d.Y),
				}
			}
			// §10.8.1's vertical-align, composed with what the boxes outside
			// this one already asked for. It is recorded against the box as well
			// as carried down, because the box's own background and border are
			// moved by it and they are made from the box rather than from the
			// items — see inlineDecor.finish.
			inner.Valign = l.vAlignFor(child, frame.Valign)
			if inner.Valign.Aligned() && !frame.Measuring {
				l.inlineAligns[child] = inner.Valign
			}
			if inner.Offset != (Point{}) && !frame.Measuring {
				// The box's own displacement, which its background and border are
				// drawn at. It is recorded here because this is the only walk that
				// has it: the items carry the offset of whatever box they came
				// from, which for a nested inline is not this one's.
				l.inlineOffsets[child] = inner.Offset
			}
			// The formatting codes unicode-bidi stands for, around the box's
			// contents. This is the one walk that sees where an inline box begins
			// and ends, and an embedding or an isolate is exactly a pair of
			// characters at those two points.
			open, closing := bidiControls(child)
			frame.Bidi.Enter(open)
			lead, trail, any := l.insetItems(child, frame.Containing)
			if any {
				// A break opportunity carried in belongs to the box's leading
				// edge and not to its first word: a line may end before
				// "<span style='margin-left: 99px'>word</span>", and it may not
				// end between that margin and the word it pushes along.
				//
				// Whether a collapsible space may still collapse into the one
				// before it is a different question and is left alone. §4.1.1's
				// fourth rule collapses across an inline boundary, and a margin
				// on the boundary does not make two spaces into one space each.
				lead.BreakBefore = state.BreakOpportunity
				state.BreakOpportunity = false
				out = append(out, lead)
			}
			out, state = l.collectInline(child, out, state, inner)
			if any {
				// The trailing edge takes no opportunity of its own: a line
				// cannot end between a box's last word and its own closing
				// margin, so whatever was carried in passes through to whatever
				// comes after the box.
				out = append(out, trail)
			}
			frame.Bidi.Leave(open, closing)
		}
	}
	return out, state
}

// insetItems is the room a non-replaced inline box's own margin, border and
// padding take on the line.
//
// # Why an inline box needs this at all
//
// §8.3, §8.4 and §8.5 each say the same thing about the horizontal axis and the
// opposite about the vertical: margin-left, border-left-width and padding-left
// apply to a non-replaced inline box and push its content along, while
// margin-top and margin-bottom do not apply at all and a vertical padding or
// border bleeds over the lines above and below without changing the height of
// any of them.
//
// This engine had none of it. A "<span style='margin-left: 96px'>" set its text
// where the span before it ended, and the measurement that found it is worth
// naming because the tests were not looking for this: css/CSS2/text's
// letter-spacing and word-spacing families check their property by drawing the
// same picture twice, once with the property and once with an equivalent margin
// on an inline box. The engine got letter-spacing and word-spacing right and the
// margin wrong, so 34 tests failed and read as spacing failures.
//
// # What is emitted, and what a line does with it
//
// Two items with no text: one before the box's content and one after, carrying
// the summed inset of that side. They are ordinary items in every other respect,
// which is what makes the rest of §9.4.2 come out on its own:
//
//   - The inset is added once at the box's real start and once at its real end,
//     not once per line, which is what §8.6's "box-decoration-break: slice"
//     asks for on a box broken across lines.
//   - A line box holding an inline box with a non-zero horizontal margin,
//     border or padding is not one of §9.4.2's zero-height line boxes, and it
//     is not one here either — there is an item on it, so the line exists and
//     takes the strut's height. A box whose insets are all zero emits nothing
//     and so still produces no line, which is what keeps an empty "<span></span>"
//     from putting a blank line into every document.
//   - The intrinsic widths pick it up without being told: widthsOf's default
//     case adds an unbreakable item to both the running word and the line.
//
// The pair is emitted together or not at all, and that is what the third result
// says. A box with a margin on one side only used to emit the one item that had
// a width in it; both are needed now, because §8.6 can move the width from one
// to the other and there has to be something at the far end to move it to. The
// item that ends up empty draws nothing and takes no room, so nothing but
// insetSides can tell the difference.
//
// # §8.6's bidi box model
//
// The left inset is emitted before the box's content and the right after it, in
// *logical* order. On a line the reordering does not touch — every line of a
// left-to-right document — logical order is physical order and there is nothing
// more to do. On a line it does touch, the item emitted first is drawn last.
//
// §8.6 is physical on both sides of its rule: with either direction, "the
// leftmost generated box" carries the left margin, border and padding and "the
// rightmost" carries the right ones, and what the direction decides is only
// *which line box* of a box broken across several. So the fix is not to swap on
// the direction property. Swapping on the declared direction was tried and
// measured nine clean passes worse, because "direction: rtl" with the initial
// "unicode-bidi: normal" changes the box model and does not reorder anything —
// the swap assumed an order that had not changed.
//
// insetSides does it on the *resolved embedding level* of the box's content
// instead, which is the thing that actually says whether the content was
// reversed, and which is known only after resolveBidi has run.
//
// What is painted in that room is inlinepaint.go's, and the two have to agree
// about §8.6 or the ink and the space it sits in would come apart: this decides
// which side's inset is *reserved* on which piece, and that decides which side's
// border is *drawn* on which fragment. Both read the same two flags, and a test
// that plants a defect in either of them fails on the other's assertions.
//
// An outline is still not drawn, on an inline box or on any other — nothing in
// this engine paints one.
// splitInsetSides turns "does the box begin here" into "which physical side",
// which is the same question only in a left-to-right containing block.
//
// §8.6's slice model gives a box broken by a block inside it its own inset on
// the pieces at its two ends and nothing on the joins, and which *physical*
// side that is depends on which way the containing block runs. In a
// right-to-left one the piece the box begins on is the rightmost, so what
// belongs to it is the right inset: "<span style='padding-right: 10px'>" holding
// nothing but a block reserves its ten pixels *before* the block under rtl and
// after it under ltr. block-in-inline-empty-002 and -004 are that pair, written
// once each way with their references in the two orders.
//
// The containing block's direction and not the box's own, for the reason
// resolveWidth gives where it settles the over-constrained case: a box that
// declares "direction: rtl" is saying which way its contents run, not which
// side of its parent it hangs from.
//
// That choice is a reading rather than a measurement, and it is worth saying
// which. Every document in the suite sets the direction on an ancestor and lets
// the span inherit it, so the two answers agree on all of them and no test can
// tell them apart — planting the box's own direction here changes nothing. It
// follows resolveWidth because being wrong the same way twice is at least
// consistent, and because the alternative reads the property as saying
// something about the box's own outside, which it does not.
func splitInsetSides(b *Box) (noLeft, noRight bool) {
	noLeft, noRight = b.noLeadInset, b.noTrailInset
	if beginsAtRight(b) {
		noLeft, noRight = noRight, noLeft
	}
	return noLeft, noRight
}

// beginsAtRight reports whether the physical side an inline box *begins* on is
// its right, which is what its own "direction: rtl" makes it.
//
// §8.6 states the rule twice, once per direction, and the two halves differ only
// in which physical side goes with which end of the box:
//
//	When the element's 'direction' property is 'ltr', the leftmost generated box
//	of the first line box in which the element appears has the left margin, left
//	border and left padding [...] When the element's 'direction' property is
//	'rtl', the rightmost generated box of the first line box [...] has the right
//	padding, right border and right margin.
//
// "The element's" — its own, not its containing block's, and the suite settles
// it rather than leaving it a reading. CSS2/box has the four combinations as
// four documents against four references: rtl-span-only is a "direction: rtl"
// span inside a left-to-right block and its reference gives the first line the
// *right* inset, while ltr-span-only is the mirror and gives the first line the
// left one. The containing block decides where the content sits; the element
// decides which of its own edges begins it.
//
// An earlier note here said no test could tell the two apart and followed
// resolveWidth on that basis. That was wrong about the suite: those two
// documents exist for exactly this question.
func beginsAtRight(b *Box) bool {
	return isRTL(b)
}

func (l *layouter) insetItems(b *Box, containing style.Unit) (lead, trail inlineItem, any bool) {
	edges := l.edges(b, "margin", containing).
		Add(l.borderWidths(b)).
		Add(l.paddingOf(b, containing))

	left, right := edges.Left, edges.Right
	noLeft, noRight := splitInsetSides(b)
	if noLeft {
		left = 0
	}
	if noRight {
		right = 0
	}
	if left == 0 && right == 0 {
		return inlineItem{}, inlineItem{}, false
	}
	item := inlineItem{Box: b, Inset: true}
	lead, trail = item, item
	lead.InsetLead = true
	lead.Width, trail.Width = left, right
	return lead, trail, true
}

// itemsFor cuts one text box into items at its break opportunities and measures
// each, applying the half of §4.1.1 that could not be done per node.
func (l *layouter) itemsFor(b *Box, in inlineState, frame inlineFrame) ([]inlineItem, inlineState) {
	offset := frame.Offset
	face, ok := l.faceForText(b)
	if !ok {
		return nil, in
	}
	l.checkScript(b)
	// Per face-run rather than per box: a character the family's face cannot set
	// is not missing from the page if a fallback face set it, and reporting it
	// would be this engine calling its own correct output a failure. The runs
	// are the same ones the items below are built from, so what is checked is
	// exactly what is drawn.
	for _, run := range l.faceRunsFor(b, face, b.Text) {
		l.checkGlyphs(b, run.Face, run.Text)
	}

	size := b.FontSize
	ws := whiteSpaceFor(b.Style)
	// Both are read once per text box rather than once per piece: they are
	// inherited properties, so every piece of one box has the same answer, and
	// the decorations are memoized across the whole tree besides.
	decorations := l.decorationsFor(b)
	spacing := l.spacingFor(b)
	// §10.8.1's leading, for the same reason and read the same number of times.
	// It is the inline box's own line-height and font rather than the block's,
	// which is the whole of what makes a <span> set larger than the paragraph
	// around it grow the line it is on.
	above, below := l.leading(b)
	ow := overflowWrapOf(b.Style)
	wb, unhandled := wordBreakOf(b.Style["word-break"])
	if unhandled != "" {
		l.reportWordBreak(b, unhandled)
	}
	lb, unhandledLine := lineBreakOf(b.Style["line-break"])
	if unhandledLine != "" {
		l.reportLineBreak(b, unhandledLine)
	}
	pieces, endedAtBreak := splitAtBreaks(b.Text, ws, wb, lb)
	if len(pieces) == 0 {
		// A box that produced nothing passes an opportunity through rather than
		// swallowing it — and it may have created one of its own, which is what
		// a <span> holding a single zero-width space is. Either source counts.
		in.BreakOpportunity = in.BreakOpportunity || endedAtBreak
		return nil, in
	}

	var tabStop, tabFloor style.Unit
	for _, p := range pieces {
		if p.Tab {
			tabStop = l.tabStop(b, face)
			// §4.1.2's threshold is half of "ch", which is the advance of "0" in
			// the box's own font — the same measurement the "ch" unit is, taken
			// here rather than through lengthOf because there is no declaration
			// to parse. A face with no digit gives nothing to halve, and then the
			// threshold is absent rather than zero: absent means every shift is
			// long enough, which is the behaviour of every engine that does not
			// implement the rule and is the one that cannot move a tab stop by
			// mistake.
			tabFloor = l.br.Measure(face, "0", size).Div(2)
			break
		}
	}

	out := make([]inlineItem, 0, len(pieces))
	state := in
	for _, p := range pieces {
		if p.Segment {
			// A segment break that survived Phase I is a break the author
			// wrote, and it ends the line as firmly as a <br> does — and ends a
			// bidi paragraph with it, for the same reason.
			out = append(out, inlineItem{Box: b, Face: face, Size: size, Forced: true,
				Offset: offset, Leads: true, Above: above, Below: below,
				Valign: frame.Valign})
			frame.Bidi.BreakParagraph()
			state = startOfContext()
			continue
		}
		if p.Collapsible && state.AfterCollapsibleSpace {
			// §4.1.1's fourth rule: a collapsible space following another
			// collapses to zero advance width, across an inline boundary as
			// readily as within one — so "a <span> </span> b" sets one space
			// and not three. It keeps its break opportunity, which is what the
			// rule's parenthesis is for.
			state.BreakOpportunity = true
			continue
		}

		// The faces this piece needs, which for everything that is not mixed
		// script is the one face the box chose.
		//
		// A space and a tab are never split: a space is one character, and a tab
		// is measured against a tab stop rather than through a face at all, so
		// splitting either would be arranging for the flags below to be shared
		// between two items that cannot both own them.
		runs := []faceRun{{Text: p.Text, Face: face}}
		if !p.Space && !p.Tab {
			runs = l.faceRunsFor(b, face, p.Text)
		}
		for ri, run := range runs {
			para, start, end := frame.Bidi.Add(run.Text)
			item := l.textItem(textItemArgs{
				b: b, p: p, run: run, size: size, frame: frame, ws: ws,
				above: above, below: below, offset: offset, spacing: spacing,
				decorations: decorations, ow: ow, state: state,
				tabStop: tabStop, tabFloor: tabFloor,
				para: para, bidiStart: start, bidiEnd: end,
				// Only the first run of a piece may begin a line. The rest are
				// the middle of a word that happens to change face, and a break
				// there would cut a word in two for a reason no reader can see.
				first: ri == 0, last: ri == len(runs)-1,
			})
			out = append(out, item)
		}
		state = inlineState{AfterCollapsibleSpace: p.Collapsible}
	}
	return out, inlineState{
		BreakOpportunity:      endedAtBreak,
		AfterCollapsibleSpace: state.AfterCollapsibleSpace,
	}
}

// textItemArgs is what one text item is built from. It is a struct because the
// list is long and every one of them is read, which is the shape that makes a
// positional call unreadable.
type textItemArgs struct {
	b           *Box
	p           paragraph.Piece
	run         faceRun
	size        style.Unit
	frame       inlineFrame
	ws          paragraph.WhiteSpace
	above       style.Unit
	below       style.Unit
	offset      paragraph.Point
	spacing     textSpacing
	decorations []textDecoration
	ow          paragraph.OverflowWrap
	state       inlineState
	tabStop     style.Unit
	tabFloor    style.Unit
	para        int
	bidiStart   int
	bidiEnd     int
	first       bool
	last        bool
}

func (l *layouter) textItem(a textItemArgs) inlineItem {
	b, p, ws := a.b, a.p, a.ws
	item := inlineItem{
		BidiPara: a.para, BidiStart: a.bidiStart, BidiEnd: a.bidiEnd,
		Text: a.run.Text, Box: b, Face: a.run.Face, Size: a.size,
		Leads: true, Above: a.above, Below: a.below,
		// §10.8.1's vertical-align, which a text box cannot be asked for
		// itself: the property is not inherited, so the anonymous box holding
		// a <span>'s words carries the initial value however the span was
		// aligned. The frame brought the answer down from the boxes the walk
		// is inside.
		Valign: a.frame.Valign,
		// An opportunity carried in from the piece before is offered to
		// anything but a space. UAX #14's LB7 — "do not break before spaces"
		// — is an earlier rule than every rule that creates one, so a space
		// belongs to the unit in front of it and the break falls after it.
		// The piece's own opportunity still stands, which is what puts the
		// break after a preserved space rather than losing it.
		BreakBefore: a.first && (p.BreakBefore || (a.state.BreakOpportunity && !p.Space)),
		Space:       p.Space, Collapsible: p.Collapsible,
		// A trailing space is trimmed off the end of a line, and only the
		// last run of a piece has an end for one to be at.
		TrimAtEnd: p.TrimAtEnd && a.last,
		Tab:       p.Tab, TabStop: a.tabStop, TabFloor: a.tabFloor,
		// §4.1.2's fourth rule, which is three answers and not one.
		//
		// What reaches it is whatever rule 3 left: under a collapsing value
		// that is the other space separators and the preserved tabs, and
		// under a preserving one it is the spaces as well. The rule then
		// names the values one at a time.
		//
		//   - normal, nowrap and pre-line — which is exactly ws.collapse —
		//     hang the sequence *unconditionally*. It never takes room.
		//   - pre-wrap hangs it unconditionally too, "unless the sequence is
		//     followed by a forced line break, in which case it must
		//     conditionally hang the sequence instead". A conditional hang
		//     takes room and gives it up only where the room is not there.
		//   - break-spaces is named to say the sequence does *not* hang: the
		//     spaces are data and take room even when they overflow.
		//   - pre is not in the list at all, so nothing hangs under it. That
		//     is not an omission to be read past: a line under pre ends only
		//     where the author ended it, so its trailing spaces are before a
		//     forced break or the end of the block, and the rule's whole
		//     subject is what to do at a wrap.
		//
		// The distinction is invisible on the page and decides two intrinsic
		// widths, which is where hangsHard is read. See widthsOf.
		Hangs:     p.Space && !p.Collapsible && !ws.BreakSpaces && (ws.Collapse || ws.Wrap),
		HangsHard: p.Space && !p.Collapsible && ws.Collapse,
		NoWrap:    !ws.Wrap, Offset: a.offset,
		BreakWord:   a.ow.BreakWord,
		Anywhere:    a.ow.Anywhere,
		Decorations: a.decorations, Spacing: a.spacing,
	}
	if !p.Tab {
		// A tab is measured against a tab stop when it lands, so there is
		// nothing to measure here and the face's own advance for U+0009 —
		// whatever a face happens to give a character it has no glyph for —
		// would be the wrong number to carry.
		item.Width = l.br.MeasureSpaced(a.run.Face, a.run.Text, a.size, a.spacing)
	}
	return item
}
