package layout

import (
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
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
		if c.Box != nil && overflowIsScrollable(c.Box.Style) {
			// A box whose overflow is not visible has no baseline to give: what
			// is inside it may be scrolled away, so a line of it is not a line
			// anything outside can be aligned to. §10.8.1 says so about an
			// inline-block and the same sentence is what a block container
			// passes up — its own baseline is its bottom margin edge, and that
			// is what it contributes here.
			//
			// Without this the search walked straight through such a box and
			// took a line from inside it: baseline-block-with-overflow-001 is a
			// 30px "overflow: hidden" div inside an inline-block, and its own
			// bottom edge is where the text beside it belongs.
			return inset.Add(c.BorderRect.Y).Add(c.BorderRect.H).Add(c.Margin.Bottom), true
		}
		if bl, ok := lastLineBaseline(c); ok {
			return inset.Add(c.BorderRect.Y).Add(bl), true
		}
	}
	if n := len(f.Lines); n > 0 {
		line := f.Lines[n-1]
		return inset.Add(line.Rect.Y).Add(line.Baseline), true
	}
	if f.Marker != nil {
		// A list item whose content is empty still has a marker, and the marker
		// sits on a line box: §12.5.1 puts it in the item's principal box, and
		// the item is not a box with nothing on any line just because the author
		// wrote no words in it.
		//
		// It matters only for an inline-block, which is the one box whose
		// baseline this decides — and the wrong answer there is not subtle. With
		// no line box found, §10.8.1 falls through to "the bottom margin edge",
		// so an empty item's marker was drawn a whole ascent below where a
		// browser puts it: the square of "<div style=display:inline-block><span
		// style=display:list-item></span></div>" hung below the line instead of
		// sitting on it. Six of the suite's list tests are that document.
		return inset.Add(f.Marker.At.Y), true
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
			//
			// The displacement of the inline boxes it was written inside goes
			// with it, for the same reason a float's does — §9.4.3's relative
			// positioning moves a box and everything written in it, and a
			// static position is where the box *would have been*, which is
			// after that move. abspos-inline-008 is a relative span inside a
			// relative div, offset the one against the other, and the box in it
			// belongs where the two cancel.
			out = append(out, inlineItem{Abs: child, Offset: frame.Offset})
			continue
		}
		if child.Float != FloatNone {
			// Out of flow, so it neither takes width on the line nor breaks it:
			// it is recorded where it was written and placed when the line it
			// belongs to is known. The state passes straight through, because
			// "a <span class=float></span>b" is still one word followed by
			// another with a space between them.
			// The displacement of the inline boxes it was written inside goes
			// with it: relative positioning is applied after layout, so an
			// inline's offset moves a float it contains without changing the
			// band the text around it flows past. See Item.Offset.
			out = append(out, inlineItem{Float: child, Offset: frame.Offset})
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
			// CSS Text §5.1's exception, on the near side: the opportunity in
			// front of the picture is not there when the character before it
			// holds on to it. "a&#8288;<img>" is a word joiner and a picture,
			// and the whole point of writing one is that they stay together.
			if state.AfterBinding {
				item.BreakBefore = false
			}
			// And §5.1's rule about *which element* decides, which applies to
			// this opportunity as it does to one between two characters: "the
			// white-space property on the nearest common ancestor of the two
			// characters controls breaking". A picture is a character unit for
			// this purpose, so a line may not end in front of one inside a
			// "white-space: nowrap" span — and two inline-blocks written side by
			// side under "pre" or "nowrap" wrapped anyway, which is the one
			// thing those values are for.
			//
			// Only the opportunity is refused. Marking the item NoWrap as well
			// would say something else — that a line may not *begin* here — and
			// Item.NoWrap conflates the two: the rewind branch in the fill reads
			// it and would decline to go back to the space before the span,
			// where a break is perfectly legal.
			if prev, ok := state.AfterBox.(*Box); ok && item.BreakBefore {
				if anc := commonAncestor(prev, child); anc != nil &&
					!whiteSpaceFor(anc.Style).Wrap {
					item.BreakBefore = false
				}
			}
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
			state.AfterBox = child
			state.AfterCollapsibleSpace = false
			// The far side of the same exception. Which character follows is
			// not known here — it may be in another text node — so what is
			// recorded is that the opportunity came from a picture, and the
			// code that sees the next character decides.
			state.AfterAtomic = true
			state.AfterBinding = false
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
			// It is an inline element, so §10.8.1 puts its leading on the line
			// it ends like any other box's — and the leading is its own
			// line-height, which it inherited from whatever it is inside. A
			// <br> alone in a "line-height: 200px" span made a line the height
			// of the block's strut instead, and everything after it sat two
			// hundred pixels too high. The preserved segment break below is the
			// same character by another spelling and has always done this.
			// ::before and ::after on a <br> are boxes like any others. They
			// were being dropped with the element, because the break replaced
			// the box rather than standing between its two halves.
			for _, gen := range child.Children {
				if gen.Pseudo == "before" {
					out, state = l.collectInline(gen, out, state, frame)
				}
			}
			brAbove, brBelow := l.leading(child)
			out = append(out, inlineItem{Box: child, Forced: true,
				Leads: true, Above: brAbove, Below: brBelow,
				Valign: frame.Valign})
			// It ends a bidi paragraph too: CSS makes a forced break a paragraph
			// separator, so the direction of what follows is decided afresh
			// rather than by the first strong character of the block.
			frame.Bidi.BreakParagraph()
			// What follows is at the start of a line, so a collapsible space
			// there is removed rather than indenting it.
			state = startOfContext()
			for _, gen := range child.Children {
				if gen.Pseudo == "after" {
					out, state = l.collectInline(gen, out, state, frame)
				}
			}
			continue
		}
		if child.Element != nil && strings.EqualFold(child.Element.Name, "wbr") &&
			len(child.Children) == 0 {
			// A break opportunity the author wrote, and the counterpart of the
			// <br> above: that one ends a line wherever it falls, this one only
			// says a line *may* end here.
			//
			// It was reaching the layout as an inline box with nothing in it,
			// which produces no items and so no opportunity — "aaaa<wbr>bbbb"
			// set as one unbreakable word in a box four characters wide, and
			// nothing said so. Eighteen of the suite's reftests write one.
			//
			// The opportunity is recorded on the state rather than emitted as a
			// zero-width space in the text, and the difference is what the
			// element means. HTML calls it a line break opportunity and nothing
			// else: it marks no boundary in the *text*, so "sur<wbr/>name" is
			// one word and text-transform: capitalize gives it one capital,
			// which a space in the text — of any width — would not.
			//
			// It is unconditional, as a space's is. UAX #14's "a line may not
			// begin with this character" is applied to the opportunities an
			// ideograph defers and not to the one after a space, and an author
			// who writes <wbr> has said where the line may end.
			//
			// An element with a child is one word-space-transform has turned
			// into a zero width space of its own — see the box builder — and
			// that space is a break opportunity in its own right, so this case
			// is for the empty one only.
			state.BreakOpportunity = true
			state.AfterBox = child
			state.AfterAtomic = false
			state.AfterBinding = false
			state.AfterDeferred = false
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
			before := len(out)
			out, state = l.collectInline(child, out, state, inner)
			if any {
				// The trailing edge takes no opportunity of its own: a line
				// cannot end between a box's last word and its own closing
				// margin, so whatever was carried in passes through to whatever
				// comes after the box.
				out = append(out, trail)
			} else if len(out) == before {
				// An inline box that put nothing on the line — no text, no
				// picture, and no margin, border or padding of its own — is
				// still on it. §10.8.1 says so by name: "empty inline elements
				// generate empty inline boxes, but these boxes still have
				// margins, padding, borders and a line height, and thus
				// influence these calculations just like elements with content".
				//
				// So it contributes its leading and nothing else. The suite's
				// empty-inline-003 is the shape — a "line-height: 5" span with
				// no content, in a "line-height: 1" block — and its reference
				// draws the line five times the height this engine gave it.
				//
				// Only where the box emitted nothing at all. Where it has an
				// inset the two items above already carry the same leading, and
				// where it has content the content does; a third copy would
				// change no height and would put an item on the line that
				// nothing else knows about.
				out = append(out, l.leadingItem(child))
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
	margin := l.edges(b, "margin", containing)
	border := l.borderWidths(b)
	padding := l.paddingOf(b, containing)
	edges := margin.Add(border).Add(padding)

	left, right := edges.Left, edges.Right
	// Whether the box *has* a margin, border or padding on each side, which is
	// not the same question as whether they come to something. §9.4.2 asks the
	// first — "inline elements with non-zero margins, padding, or borders" — and
	// the two differ wherever one cancels another: a "border-right: 200px"
	// against a "margin-right: -200px" sums to nothing and still draws two
	// hundred pixels of border, so the line it is on is not one of the
	// zero-height ones. Read as a sum it emitted nothing, the line did not
	// exist, and the border was not drawn at all — which is
	// margin-padding-clear/margin-right-114, a green square that came out blank.
	anyLeft := margin.Left != 0 || border.Left != 0 || padding.Left != 0
	anyRight := margin.Right != 0 || border.Right != 0 || padding.Right != 0
	noLeft, noRight := splitInsetSides(b)
	if noLeft {
		left, anyLeft = 0, false
	}
	if noRight {
		right, anyRight = 0, false
	}
	if !anyLeft && !anyRight {
		return inlineItem{}, inlineItem{}, false
	}
	// Both physical values on both items, so that a later stage can ask which
	// side of the box faces a boundary — see shapingcontext.go. Width is one of
	// them and is chosen by insetSides, which cannot happen until the levels are
	// resolved.
	// The box's own leading, which §10.8.1 counts on every line the box is on
	// whether or not there is anything of its own to draw. An empty
	// "<span style='font-size: 200px; border: 1px solid'></span>" is two hundred
	// pixels tall and made a twenty-pixel line, because the only items on it
	// were these two and neither carried a height.
	above, below := l.leading(b)
	item := inlineItem{Box: b, Inset: true, InsetLeft: left, InsetRight: right,
		Above: above, Below: below, Leads: true}
	lead, trail = item, item
	lead.InsetLead = true
	lead.Width, trail.Width = left, right
	// Which side each item stands for, so that a later stage can ask whether
	// *this* edge carries anything rather than whether the box does. See
	// Item.Edged.
	lead.Edged, trail.Edged = anyLeft, anyRight
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
	runsOfBox := l.faceRunsFor(b, face, b.Text)
	for _, run := range runsOfBox {
		l.checkGlyphs(b, run.Face, run.Text)
	}
	l.reportWhollySubstituted(b, face, runsOfBox)

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
	// §5.3's loose tailoring is qualified "in Chinese and Japanese", and which
	// of those the text is comes from the language tag's *script* rather than
	// from the property. See paragraph.WritingSystemOf.
	lb.ChineseOrJapanese = boxWritingSystem(b).ChineseOrJapanese()
	if unhandledLine != "" {
		l.reportLineBreak(b, unhandledLine)
	}
	hy, unhandledHyphens := hyphensOf(b.Style["hyphens"])
	lang := boxLanguage(b)
	if hy.Auto && !hyphenatesLanguage(lang) {
		// "auto" asks for the language's own dictionary, and there is one
		// language here. A document in another gets the manual behaviour and
		// is told so — which is the report that used to be raised for every
		// "auto" whatever the language.
		unhandledHyphens = "auto"
	}
	if unhandledHyphens != "" {
		l.reportHyphens(b, unhandledHyphens)
	}
	// text-autospace is applied between two runs rather than inside one — see
	// autospace.go — so nothing here reads the value. What is read here is
	// whether the document asked for a part of it this engine does not do, which
	// is a question about the box and belongs where the other three are asked.
	l.reportKerning(b, face)
	autospace, unhandledAutospace := autospaceOf(b.Style["text-autospace"])
	if unhandledAutospace != "" {
		l.reportAutospace(b, unhandledAutospace)
	}
	pieces, endedAtBreak := splitAtBreaks(b.Text, ws, wb, lb, hy)
	pieces = collapsibleSeparators(pieces, wordSpaceTransformValue(b.Style))
	if points := l.hyphenPoints[b]; len(points) > 0 {
		var endsAtHyphen bool
		pieces, endsAtHyphen = hyphenatePieces(pieces, points)
		// A point at the very end of this box is an opportunity for whatever
		// box comes next, which is what a soft hyphen ending a node already
		// does. "high<span>way</span>" is the shape: the point falls between
		// the two text boxes, so neither of them holds it on its own.
		endedAtBreak = endedAtBreak || endsAtHyphen
	}
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
	// An opportunity this box's first character refused, waiting on whatever
	// follows the box. See the branch that sets it.
	heldAtEdge := false
	// §5.1's rule about which element decides, resolved once for the box.
	//
	// The boundary in front of this box's first character has a character on
	// each side of it and they are in different boxes, so it belongs to neither:
	// what governs it is the white-space of the nearest common ancestor. Inside
	// the box there is no boundary to cross and the box's own value is the
	// answer, which is what noWrap holds.
	//
	// It is asked whether or not an opportunity has already been established at
	// that boundary, and that is deliberate. §5.1's sentence is about the
	// boundary rather than about an opportunity that reached it, and the
	// opportunities are not all found in one place: word-break: break-all makes
	// one at this very edge, and it is made further down this function, after
	// this has been decided. Gating on the incoming state answered "no
	// opportunity yet" for exactly that case and cost a reftest —
	// break-boundary-2-chars-001, which writes "abc<span>xyz</span>def" under
	// break-all with the span set to pre and asks for the break at both edges of
	// the span.
	noWrap := !ws.Wrap
	boundaryNoWrap := noWrap
	// And §3's break-spaces at the same boundary, for the same reason and by the
	// same rule about who decides.
	//
	// UAX #14's LB7 is "× SP", so an opportunity carried in from another box is
	// withheld from a space — a line may not end in front of one. break-spaces
	// is the value that overrules it, and CSS Text §3 says so in the words that
	// name this case: "there is a soft wrap opportunity after every preserved
	// white space character, including between white space characters".
	//
	// SplitAtBreaks applies that inside a run, so "ああ␣␣␣␣ああ" in three
	// ideographs of room sets three, three and two. Putting a <span> — even an
	// empty one — between the first space and the second split the text into two
	// boxes, the opportunity between them was withheld as LB7's, and the fill
	// found nowhere to break the second line: it rewound to the last opportunity
	// it had, which was between the two ideographs, and set one character on the
	// first line. trailing-ideographic-space-break-spaces-005 and -006 are that
	// document.
	boundaryBreakSpaces := ws.BreakSpaces
	if prev, ok := in.AfterBox.(*Box); ok {
		gov := commonAncestor(prev, b)
		if in.AfterCollapsibleSpace {
			// Except where a space left the opportunity, which is its own and
			// not the boundary's. §3 gives it to the space — "there is a soft
			// wrap opportunity after every white space character" — so what
			// decides whether it may be taken is the element the space is in.
			//
			// The ancestor's answer is the same one everywhere the two elements
			// agree, which is everywhere white-space is inherited rather than
			// declared. Where they do not, the suite's
			// white-space-wrap-after-nowrap-001 is the document: a nowrap block
			// holding a wrapping span whose last character is a space, and then
			// more of the block's own text. The common ancestor is the block and
			// says no; the space is in the span and says yes, and the reference
			// breaks the line.
			gov = prev
		}
		if gov != nil {
			boundaryNoWrap = !whiteSpaceFor(gov.Style).Wrap
			boundaryBreakSpaces = whiteSpaceFor(gov.Style).BreakSpaces
		}
	}
	// Read off the ancestor rather than off this box, because §5.1 gives the
	// boundary to the innermost element containing both characters. When that
	// note was written it was a distinction no document made — white-space
	// inherits, so the two agree everywhere it is not declared, and the suite
	// gave the same 5594 clean passes either way. The exception above is a
	// document that does declare it on both, and there the two part.
	//
	// The narrower reading — that only an opportunity left behind by a *space*
	// may be taken by one, since §3's sentence is about the space that leaves it
	// — was written first and could not be made to fail. An opportunity reaches
	// a box boundary only when the text before it ended at one, and no document
	// tells the two readings apart: the reftest suite gives 5594 clean passes
	// either way with no test moving, and neither does a fixture built for the
	// case the wider reading would get wrong — an atomic inline, then a space in
	// a box of its own, then a float, which is the shape flatten.go already
	// records a measurement about for LB7. It cost a field on the shared state
	// and three places that had to keep it up to date, so it is gone and this is
	// the note that says it was tried.
	for i, p := range pieces {
		// CSS Text §5.1's exception, on the far side: the opportunity a picture
		// left behind is not offered to a character that holds on to it.
		//
		// Only the first piece can be the one next to the picture — after that
		// there is text in between — and it is written as the index rather than
		// as a flag the loop clears, because that is a thing a reader can check
		// against the loop rather than against every path out of it.
		if i == 0 && state.AfterAtomic && bindsToAtomicInline(p.Text) {
			state.BreakOpportunity = false
		}
		// And the general form of the same thing. An opportunity carried in from
		// the box before is offered to whatever begins this one, and a line may
		// not begin with a closing bracket, a hyphen or a non-starter whichever
		// box the character happens to be written in. SplitAtBreaks withholds it
		// *inside* a run; across a boundary there is no character in the earlier
		// box to test, so the box receiving the opportunity is what asks.
		//
		// The suite writes it as "中中<span>〜</span>文" — the character a line
		// may not begin with in an element of its own, which is what a test that
		// wants to colour it does — and its whole line-break strictness family
		// is that shape.
		//
		// Only an opportunity an ideograph deferred, which is the same subset the
		// rule is applied to inside a run: a break after a space is not one this
		// withholds, and never has been — "AA )BB" breaks after the space. The
		// two have to agree, or the answer depends on whether the author wrote a
		// <span>.
		//
		// Not after an atomic inline either, and §5.1 says why in as many words:
		// there is an opportunity before and after each one "even when adjacent
		// to a character that would normally suppress them". A picture followed
		// by a closing bracket may still be wrapped away from it. The exception
		// to the exception is the three binding classes, which the branch above
		// is. It falls out of AfterDeferred as well — a picture is not an
		// ideograph — and is written out because it is a rule rather than a
		// coincidence.
		if i == 0 && state.BreakOpportunity && state.AfterDeferred &&
			!state.AfterAtomic && !lb.Anywhere && mayNotBeginLine(p.Text, lb) {
			state.BreakOpportunity = false
			// Refused, not deleted. A prohibition moves an opportunity rather
			// than dropping one — "× CL" says a line may not begin with a
			// closing bracket and says nothing against one beginning with what
			// comes after it — and SplitAtBreaks holds it forward for exactly
			// that reason inside a run. Across a boundary the character that
			// would take it is in a third box, so the hold has to travel.
			//
			heldAtEdge = true
		}
		// And the hold taken up, one piece later. The piece after the one that
		// refused it is the next boundary the opportunity could fall on.
		//
		// Where the box runs out first the hold leaves with it, which is the
		// single-character span the suite writes: "字字<span>、</span>字字" has
		// the character that takes the opportunity in a third box.
		//
		// The mayNotBeginLine test is the correct reading of the rule — a line
		// may not begin with a closing bracket however many are written in a row,
		// so a second refusal should keep holding — and it has no test, which is
		// a different thing from being covered. It cannot fire as the pieces come
		// out today: SplitAtBreaks does not cut in front of a character a line
		// may not begin with, so two of them are one piece ("、）中" splits as
		// "、）" and "中"), and every piece after the first begins where a flush
		// happened. A planted defect that deleted it moved no test and no
		// reftest. It stays because the rule is real and the day the split cuts
		// differently is not a day anyone will remember this.
		if i > 0 && heldAtEdge {
			if lb.Anywhere || !mayNotBeginLine(p.Text, lb) {
				state.BreakOpportunity, heldAtEdge = true, false
			}
		}
		// §5.2's break-all treats every alphabetic, numeric and ideographic
		// character in this box as ID — and that includes the first one. UAX #14
		// allows a line to end between whatever precedes an ID and the ID itself,
		// so "aaaaaaa<span style='word-break: break-all'>bbb</span>" may break at
		// the span even though the run before it may not be broken inside.
		//
		// SplitAtBreaks cannot see that boundary: it is given one box's text, and
		// this boundary has a character on each side of it in two different
		// boxes. So the box that changed the class is the one that says so, which
		// is also the right place for it — the value is the *later* character's,
		// and the later character is this one.
		//
		// line-break: anywhere is here for the same reason and a stronger one:
		// §5.3 puts an opportunity around every typographic character unit, and
		// the edge of an inline box is not an exception it carves out.
		//
		// A line still may not *begin* with a closing bracket or a non-starter,
		// which is the rule the branch above applies to an opportunity arriving
		// from another box — so it is applied to this one too, and by the same
		// exemption line-break: anywhere overrules it.
		//
		// The index test is the correct reading of the rule and has no test,
		// which is a different thing from being covered. The rule is about one
		// boundary — the box's leading edge, the only one SplitAtBreaks could not
		// see — and every piece after the first already carries the opportunity
		// from the split itself, so dropping it moves nothing: 5556 clean passes
		// either way, and no reftest changes its answer. That is recorded here
		// rather than left as an implied claim.
		//
		// The *far* edge is not done, and the reason is that the suite does not
		// agree with itself about it. UAX #14 allows a line to end after an ID
		// whatever follows, so the symmetric rule would offer an opportunity
		// after the last character of a break-all box, and
		// word-break-break-all-inline-009 asks for exactly that. But
		// word-break-break-all-inline-007 asks for the opposite over the same
		// shape — "<span class=test>bbbbbbb</span>cccccc", whose reference puts
		// the span's last b on the line with "cccccc" and lets it overflow — and
		// there is no reading of §5.2 that gives both. Implemented with the
		// non-starter rule applied on the far side, the two trade one for one:
		// 009 goes clean, 007 goes red, and 5556 stays 5556. So the question is
		// left where the working group left it — 004, 007 and 010 are all marked
		// tentative — rather than settled by picking the fixture that suits this
		// engine.
		if i == 0 && (wb.BreakAll || lb.Anywhere) && !p.Space &&
			(lb.Anywhere || !mayNotBeginLine(p.Text, lb)) {
			state.BreakOpportunity = true
		}
		// An ideograph that begins a box, which is the ideograph rule's other
		// half arriving at a boundary. UAX #14 allows a line to end between a
		// letter or a number and an ideograph, and SplitAtBreaks offers that
		// opportunity inside a run — but it is given one box's text, and this
		// boundary has a character on each side of it in two different boxes.
		//
		// So the box holding the *later* character says so, which is the same
		// place and the same argument as break-all's rule above. What travels is
		// only what the earlier character was, which no amount of looking at
		// this box could recover.
		if i == 0 && !state.AfterAtomic && !wb.KeepAll &&
			state.AfterLetterUnit && startsIdeographic(p.Text) {
			state.BreakOpportunity = true
		}
		pieceNoWrap := noWrap
		if i == 0 {
			pieceNoWrap = boundaryNoWrap
		}
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
		if p.ZeroWidth {
			// A character that separates and nothing else. It produces no item —
			// there is nothing to draw and nothing to measure — and what it
			// leaves behind is the fact that it was there: the space after it
			// does not follow the space before it. See Piece.ZeroWidth.
			state.AfterCollapsibleSpace = false
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
		// A tab is never asked: it is measured against a tab stop rather than
		// through a face at all, so a face for it would be a number nothing
		// reads. A space is asked but never *split* — see below.
		runs := []faceRun{{Text: p.Text, Face: face}}
		if !p.Tab {
			got := l.faceRunsFor(b, face, p.Text)
			// A space piece takes a face and keeps its shape. Splitting one
			// would share the flags below between two items that cannot both
			// own them, and there is nothing to split anyway: a space piece is
			// one character, or a run of the same character, so one face
			// answers for all of it.
			//
			// It is asked at all because the width of a space can be the whole
			// of its meaning. An ideographic space is one em and the standard
			// PDF faces give it their ordinary space's quarter em, which left
			// "ああ　" four pixels short of the "あああ" it has to cover.
			if !p.Space || len(got) == 1 {
				runs = got
			}
		}
		// And cut again where §8.1 wants a gap inside one of them.
		//
		// The gap itself is opened by insertAutospace, which is a pass over the
		// finished items and sees every boundary *between* two runs. A boundary
		// inside a run is invisible to it — a backend is handed one advance per
		// run and could not express a one-off gap in the middle of one — so the
		// run is cut here and the boundary becomes one it can see.
		//
		// The value read is the box's own, and that is the right one: both
		// characters are in this box, so the innermost element containing them
		// both is this box. Where the two are in different boxes the cut is not
		// needed and the ancestor is found by the pass instead.
		if !p.Tab && !p.Space {
			if parts := splitAtAutospace(p.Text, autospace); len(parts) > 1 {
				runs = cutRunsAt(runs, parts)
			}
			// And again where §8.2's cursive tracking begins or ends, for the
			// same reason: a run carries one letter-spacing, so a run holding
			// an Arabic letter beside a Latin one cannot say that only one of
			// them is followed by a gap. See SplitAtCursiveTracking.
			//
			// Only where there is a spacing to place. A document that declares
			// none takes the scan and no cut, which is nearly all of them.
			if spacing.Letter != 0 {
				if parts := splitAtCursiveTracking(p.Text); len(parts) > 1 {
					runs = cutRunsAt(runs, parts)
				}
			}
			// And after each word separator, for the same reason again: §8.3's
			// spacing goes after the character, and a run carries a width and
			// no way to say where inside it the extra room is. A space is
			// already a piece of its own — a line may end after one — so what
			// this is for is the separator that offers no break, the no-break
			// space above all.
			if spacing.Word != 0 {
				if parts := splitAtWordSeparators(p.Text); len(parts) > 1 {
					runs = cutRunsAt(runs, parts)
				}
			}
		}
		for ri, run := range runs {
			para, start, end := frame.Bidi.Add(run.Text)
			item := l.textItem(textItemArgs{
				b: b, p: p, run: run, size: size, frame: frame, ws: ws,
				above: above, below: below, boxFace: face,
				offset: offset, spacing: spacing,
				decorations: decorations, ow: ow, state: state,
				tabStop: tabStop, tabFloor: tabFloor,
				para: para, bidiStart: start, bidiEnd: end,
				// Only the first run of a piece may begin a line. The rest are
				// the middle of a word that happens to change face, and a break
				// there would cut a word in two for a reason no reader can see.
				first: ri == 0, last: ri == len(runs)-1,
				spaceMayTakeIt: i == 0 && boundaryBreakSpaces,
				// Only the box's first piece sits at the boundary the state
				// carried in; everything after it is inside the box.
				noWrap: pieceNoWrap,
			})
			out = append(out, item)
		}
		state = inlineState{
			AfterCollapsibleSpace: p.Collapsible,
			// Whether the piece ended on a character that would hold on to a
			// picture after it. A piece is a run between two opportunities, so
			// its last character is the one next to whatever comes next.
			AfterBinding: endsBinding(p.Text),
			// Whether the opportunity this piece leaves behind is one an
			// ideograph deferred. SplitAtBreaks defers those and takes them at
			// the next character; a piece that ends in one has handed the
			// decision to whatever comes after it, which may be another box.
			AfterDeferred: endsIdeographic(p.Text),
			// Whether the character before the next boundary is one an
			// ideograph may be broken away from. See the rule above.
			AfterLetterUnit: endsLetterUnit(p.Text),
		}
	}
	return out, inlineState{
		BreakOpportunity:      endedAtBreak || heldAtEdge,
		AfterCollapsibleSpace: state.AfterCollapsibleSpace,
		AfterBinding:          state.AfterBinding,
		AfterDeferred:         state.AfterDeferred,
		AfterLetterUnit:       state.AfterLetterUnit,
		AfterBox:              b,
	}
}

// bindsToAtomicInline reports whether text begins with a character that holds
// on to an atomic inline before it.
func bindsToAtomicInline(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	return r != utf8.RuneError && paragraph.BindsToAtomicInline(r)
}

// endsBinding reports whether text ends with one that holds on to an atomic
// inline after it.
func endsBinding(text string) bool {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r != utf8.RuneError && paragraph.BindsToAtomicInline(r)
}

// startsIdeographic reports whether a piece begins with an ideograph, which is a
// character a line may begin with and may end in front of.
func startsIdeographic(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	return r != utf8.RuneError && paragraph.IsIdeographic(r)
}

// endsLetterUnit reports whether a piece ends on a typographic letter unit that
// is not itself an ideograph, which is the far side of the boundary
// startsIdeographic asks about. See inlineState.AfterLetterUnit.
func endsLetterUnit(text string) bool {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r != utf8.RuneError && paragraph.IsLetterUnit(r) && !paragraph.IsIdeographic(r)
}

// endsIdeographic reports whether a piece ends on the one character that leaves
// an opportunity *deferred* rather than taken: an ideograph, which offers a
// break after itself and lets the next character decide whether it is real.
//
// It is the question inlineState.AfterDeferred carries across a box boundary.
func endsIdeographic(text string) bool {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r != utf8.RuneError && paragraph.IsIdeographic(r)
}

// textItemArgs is what one text item is built from. It is a struct because the
// list is long and every one of them is read, which is the shape that makes a
// positional call unreadable.
type textItemArgs struct {
	b *Box
	// boxFace is the face the box itself declared, so that a run set in another
	// can be told apart without asking again.
	boxFace     *shape.Face
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
	// spaceMayTakeIt says the opportunity carried in from another box is one a
	// space may begin a line at, which is white-space: break-spaces and nothing
	// else.
	//
	// Only the box's first piece sits at that boundary; every piece after it is
	// inside the box, where SplitAtBreaks has already decided. That is the
	// correct reading and has no test, which is a different thing from being
	// covered: the state carries no opportunity past the first piece — it is
	// rebuilt from the piece at the end of each turn of the loop — so setting
	// this on every piece changes nothing. Recorded here rather than left as an
	// implied claim.
	spaceMayTakeIt bool
	// noWrap is whether a line may begin at this item. It is the box's own
	// white-space for everything inside the box, and the nearest common
	// ancestor's for the one item that sits at a boundary carried in from
	// another box — see §5.1 and the note beside State.AfterBox.
	noWrap bool
}

func (l *layouter) textItem(a textItemArgs) inlineItem {
	b, p, ws := a.b, a.p, a.ws
	// The leading the caller measured is the box's declared face's, and this run
	// may be in another: the fallback stack finds one for text the declared face
	// cannot set, and §10.8.1 measures against the font the run is *in*. See
	// leadingInFace.
	above, below := a.above, a.below
	if a.run.Face != nil && a.run.Face != a.boxFace && usesNormalLineHeight(b) {
		above, below = l.leadingInFace(b, a.run.Face)
	}
	item := inlineItem{
		BidiPara: a.para, BidiStart: a.bidiStart, BidiEnd: a.bidiEnd,
		Text: a.run.Text, Box: b, Face: a.run.Face, Size: a.size,
		Leads: true, Above: above, Below: below,
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
		//
		// break-spaces is the one value that overrules LB7, and it is the
		// caller that knows — see spaceMayTakeIt, which is that value asked
		// of the element the boundary belongs to.
		//
		// With that carve-out made, nothing left in the suite depends on the
		// exclusion: dropping "!p.Space" outright gives the same 5594 clean
		// passes, and before the carve-out it gave 5594 against 5592 — so the
		// two reftests this rule is for were the whole of what the exclusion
		// was costing, and the whole of what it was measurably doing. It stays
		// because LB7 is a real rule and dropping it would be a wider change
		// with no specification behind it, and because the note beside the
		// atomic inline above records a case that was measured when it was
		// added. What is *not* claimed is that a test here would catch its
		// removal.
		BreakBefore: a.first && (p.BreakBefore ||
			(a.state.BreakOpportunity && (!p.Space || a.spaceMayTakeIt))),
		Space: p.Space, Collapsible: p.Collapsible,
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
		NoWrap:    a.noWrap, Offset: a.offset,
		BreakWord:   a.ow.BreakWord,
		Anywhere:    a.ow.Anywhere,
		Decorations: a.decorations, Spacing: a.spacing,
	}
	if p.Hyphen && a.last {
		// The hyphen this piece would print, measured now: the line breaking has
		// no face to ask and needs the width before it can decide whether the
		// line may break here at all. Only the last run of a piece has an end
		// for a hyphen to be at — a piece cut in two by a change of face is one
		// word, and the hyphen belongs after all of it.
		var face *shape.Face
		item.HyphenText, face = l.hyphenRun(b, a.run.Face, b.Style["hyphenate-character"])
		item.Hyphen = l.br.MeasureSpaced(face, item.HyphenText, a.size, a.spacing)
		if face != a.run.Face {
			// Set in another face, so measured against it: §10.8.1's rule for
			// the run itself, four lines up, and for the same reason.
			item.HyphenFace = face
			item.HyphenAbove, item.HyphenBelow = above, below
			if usesNormalLineHeight(b) {
				item.HyphenAbove, item.HyphenBelow = l.leadingInFace(b, face)
			}
		}
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

// cutRunsAt re-cuts a piece's face runs so that every boundary in parts is also
// a boundary between two runs.
//
// The two cuts are independent — one follows the faces the text needs, the other
// follows §8.1's character classes — and the result is the finer of the two. It
// is written as a merge rather than as a second pass over the string because a
// run already carries which face set it, and that has to travel with each half.
func cutRunsAt(runs []faceRun, parts []string) []faceRun {
	// Where each part begins, as an offset into the piece.
	cuts := make(map[int]bool, len(parts))
	at := 0
	for _, part := range parts {
		at += len(part)
		cuts[at] = true
	}
	out := make([]faceRun, 0, len(runs)+len(parts))
	at = 0
	for _, run := range runs {
		start := 0
		for i := 1; i < len(run.Text); i++ {
			if cuts[at+i] {
				out = append(out, faceRun{
					Text: run.Text[start:i], Face: run.Face, substituted: run.substituted,
				})
				start = i
			}
		}
		out = append(out, faceRun{
			Text: run.Text[start:], Face: run.Face, substituted: run.substituted,
		})
		at += len(run.Text)
	}
	return out
}

// collapsibleSeparators marks the pieces an expanded virtual word separator
// became as white space that a line edge may take away.
//
// A separator is a place a line may end that the property asks to be shown, and
// showing it must not put a space where a line begins: the suite's
// word-space-transform-010 writes "<wbr>あ<wbr>い<wbr>" fifteen ways and every
// one of them has to draw "あ　い", flush against the padding at both ends.
// §4.1.2's first rule is the one that says so, and it is about *collapsible*
// white space — which under "ideographic-space" the character no longer is,
// because U+3000 is one of §4.1's other space separators and nothing collapses
// one of those.
//
// So the piece is marked rather than the character, and it is marked here
// because this is the last stage that still knows the property. The test is
// that the piece is exactly the separator, which cannot tell one the property
// made from one the author wrote in a document that asks for the same
// character — a document that sets "ideographic-space" and also types U+3000
// where a line begins. That is the whole of the confusion, no test in the suite
// is about it, and the alternative is carrying a bit through the text itself,
// which every stage between here and Phase I would have to preserve.
func collapsibleSeparators(pieces []piece, wst wordSpaceTransform) []piece {
	if !wst.Transforms() || wst.Separator == " " {
		// An ordinary space is already collapsible and already trimmed; there
		// is nothing here it does not have.
		return pieces
	}
	for i := range pieces {
		if pieces[i].Space && pieces[i].Text == wst.Separator {
			pieces[i].Collapsible, pieces[i].TrimAtEnd = true, true
		}
	}
	return pieces
}

// leadingItem is what an inline box that puts nothing on a line contributes to
// it: its own leading, at no width.
//
// It is marked Inset because that is what every stage below already understands
// an item to be when it is a box's own edge rather than something drawn — it
// takes no room, it ends no word, it offers no break opportunity, and a line
// made of nothing else is still §9.4.2's zero-height line, because
// contentOnLine asks Edged and this is not edged.
func (l *layouter) leadingItem(b *Box) inlineItem {
	// The box's own strut rather than its leading, so that an empty inline box
	// whose font and line-height are its parent's reaches exactly as far as the
	// block's strut does and ties with it. The two are the same measurement by
	// two routes and they round differently: taking the leading here moved the
	// baseline of "un<span></span>broken" by two hundredths of a pixel, which is
	// line-breaking-015.
	st := l.strutFor(b)
	return inlineItem{Box: b, Inset: true, Above: st.Baseline,
		Below: st.Height.Sub(st.Baseline), Leads: true, LeadingOnly: true}
}

// hyphenRun is the character a broken word ends with and the face to set it in.
//
// The face is asked because the character is chosen for its typography and not
// for the font in hand: U+2010 HYPHEN is the right one and a great many faces do
// not have it. Courier is one of them, so every monospaced document here was
// hyphenated with U+002D HYPHEN-MINUS — permitted by §6.1, and not what a
// reference that writes U+2010 draws.
//
// §8.1's fallback is what an ordinary run gets when its declared families cannot
// set a character, and a hyphen is text like any other. The same call the marker
// goes through finds a face that has it; where nothing does, hyphenCharacter's
// own choice of U+002D stands, because a hyphen nobody can draw is worse than
// the wrong hyphen.
//
// hyphens-vs-float-clearance-001 and -002 are the suite's case, and they are
// exact to the pixel once the character matches: four floated monospace boxes,
// each hyphenating one long word around a float.
func (l *layouter) hyphenRun(b *Box, face *shape.Face, value string) (string, *shape.Face) {
	// The face U+2010 would be set in, found the way any other character's is.
	// hyphenCharacter chooses between the two hyphens by asking whether the face
	// has the better one, and the face it should ask is the one the character
	// would be drawn in rather than the one the box declared.
	auto := l.faceThatHas(b, face, autoHyphen)
	text := hyphenCharacter(value, auto)
	if text == autoHyphen {
		return text, auto
	}
	// A string the author wrote, or U+002D where no face had U+2010. Both are
	// set in whatever can set them, for the same reason.
	return text, l.faceThatHas(b, face, text)
}

// autoHyphen is U+2010 HYPHEN: the character "hyphenate-character: auto" asks
// for where anything can draw it.
const autoHyphen = "\u2010"

// faceThatHas is the face a run of text would be set in: the box's own where it
// can set the text, and §8.1's fallback where it cannot.
func (l *layouter) faceThatHas(b *Box, face *shape.Face, text string) *shape.Face {
	if face == nil || text == "" {
		return face
	}
	if _, covered := face.GlyphID(firstRune(text)); covered {
		return face
	}
	if runs := l.faceRunsFor(b, face, text); len(runs) > 0 && runs[0].Face != nil {
		return runs[0].Face
	}
	return face
}
