package layout

import (
	"github.com/mgilbir/forme/style"
	"sort"
)

// The background and the border of a non-replaced inline box.
//
// # Why an inline box is not a rectangle
//
// Everything else this engine paints a background on is a box with one border
// rectangle. An inline box is not: CSS 2.1 §8.6 gives it a *slice* model, so a
// <span> broken across three lines paints three fragments, and the three are not
// interchangeable. The left margin, border and padding belong to the fragment
// the box begins on and the right ones to the fragment it ends on; the fragments
// between carry neither.
//
// The room those insets take is already reserved — insetItems does it, and it
// takes the same decisions this file does about which side belongs to which
// fragment, from the same two flags. What was missing was the ink.
//
// # What decides a fragment's height
//
// Not the line box. §10.6.1 makes the height of a non-replaced inline box's
// content area depend on the font rather than on the line it sits in, so a
// fragment reaches the font's ascent above the baseline and its descent below,
// whatever the line-height is. That is why a "line-height: 3" paragraph does not
// paint a triple-height stripe behind a highlighted span, and why a
// "line-height: 0.5" one paints a stripe *taller* than the line it is on.
//
// The vertical padding and border are then added outside that, and they are
// painted even though §8.3, §8.4 and §8.5 keep them out of layout: an inline
// box's vertical padding bleeds over the lines above and below without moving
// any of them. That asymmetry — the ink is there and the layout does not know
// about it — is the reason these fills are marked Overhang, and FillRect says
// what that costs.
//
// # §8.6's bidi box model, and where it still stops
//
// A fragment's extent here is the span from the leftmost to the rightmost of the
// items that belong to it, insets included — so it follows insetSides without
// having to know about it. §8.6 puts the left margin, border and padding on the
// box's leftmost generated box; insetSides puts the room for them there, this
// takes the leftmost edge of that room as the fragment's, and the two agree by
// construction rather than by two readings of the same rule.
//
// What is still not done is a box whose content a reordering splits into two
// visual pieces on one line. It paints one rectangle covering both, where a
// browser paints two.
//
// An outline is painted, and on the same fragments: it is a ring drawn outside
// each fragment's border edge, in the step 10 of the stacking context the box
// is in — see painter.outlines, which reads outline-style, outline-width and
// outline-color the way the border painting reads theirs. §8.6's slicing is not
// applied to it: CSS 2.1 §18.4 says that "in contrast to borders, the outline is
// not open at the line box's end or start", so no piece's ring is left open.
// For a box broken across lines, CSS UI 4 §5 says the outline should be one
// outline, or a minimum set of outlines, enclosing all of the box's pieces, so
// where two pieces' rings meet they are drawn as the one shape round both —
// see painter.joinedOutline — and where they do not each piece keeps its own
// ring.

// maxInlineDecorations bounds how many of these fragments one document may
// produce.
//
// The count is a product of two things a document controls independently: the
// number of line boxes an inline box is broken across, and how deeply inline
// boxes are nested — a background on each of 200 nested spans over 500 lines is
// a hundred thousand fragments from a document of a few kilobytes. Neither
// existing bound holds it: the HTML parser's nesting cap and maxBoxes bound the
// *tree*, and this is a product of the tree with the lines, which is bounded by
// neither.
//
// Only a box with something to draw is counted, which is what keeps an ordinary
// document at zero — see paintedInlines. The cap is what holds the document that
// draws on every one of them. A link counts as something to draw, because its
// fragments are made by the same slicing and a document of <a> nested in <a> —
// which XHTML's parser, unlike HTML's, does not undo — is the same product.
// An ordinary document's links are one fragment per line each, a few thousand
// on a page at the very most.
//
// 65536 is far past any real document. This engine lays out one page, so a
// fragment per line of it with a hundred nested backgrounds is still an order of
// magnitude below.
//
// It is a variable rather than a constant so that a test can lower it far enough
// to watch it fire. A bound that has only ever been observed not to trip is one
// nobody knows works.
var maxInlineDecorations = 1 << 16

// inlinePiece is the part of one inline box that lies on one line box, before
// the box model has been resolved against it.
//
// It is recorded rather than turned into a fragment on the spot because whether
// it is the box's *last* piece is not known until the whole inline formatting
// context has been laid out, and that is what decides which fragment carries the
// box's right margin, border and padding.
type inlinePiece struct {
	box *Box
	// line is the index of the line box in the block fragment's Lines.
	line int
	// left and right are the extents of the box's items on that line, measured
	// from the block's content edge. They are the *margin* extents where the
	// piece carries an inset, because the inset item insetItems emits covers the
	// margin as well as the border and the padding.
	left, right style.Unit
	// baseline is where *this box's* baseline sits, from the block's content
	// edge: the line's own, moved by §10.8.1's vertical-align applied to this
	// box. It is the box's rather than the line's so that a raised <span>'s
	// background is raised with its words — the ink and the room it sits in have
	// to come apart nowhere.
	baseline style.Unit
	// first says no earlier line held this box, so this piece begins it.
	first bool
	// scale is text-fit's factor for the line this piece is on, which is what
	// the box's own content area is measured at. One where nothing fits.
	scale float64
	// offset is §9.4.3's displacement of a text piece, which has no record of
	// its own to read it from; zero for every other piece. See textLinkArea.
	offset Point
}

// inlineDecor collects the pieces over one inline formatting context.
//
// One of these serves a whole call to inlineContent rather than a whole
// document, which is what makes "first" and "last" mean the right thing when a
// block is laid out twice: settle throws a fragment away and lays the box out
// again, and a record kept on the layouter would remember the discarded
// attempt's lines and refuse to begin the box a second time.
type inlineDecor struct {
	l *layouter
	// containing is the width a percentage margin or padding on an inline box
	// resolves against, which is the containing block's and not the line's.
	containing style.Unit
	// strut is the block's own line metrics, which is what §10.8.1 measures a
	// vertical-align keyword against — "text-top" is the top of the parent's
	// content area. It is the same value stackLine used, so a box's ink and its
	// text are moved by the same arithmetic.
	strut  strut
	pieces []inlinePiece
	// last is the index of the most recent piece made for each box, which is
	// what finish needs to find the fragment that ends it.
	last map[*Box]int
}

// addLine records what each painting inline box occupied on one line.
//
// at is where the line box's own left edge sits within the block's content box,
// with §16.2's alignment shift already in it, so that adding an item's offset
// within the line gives a coordinate in the same space the line boxes are in.
func (d *inlineDecor) addLine(index int, items []inlineItem, xs, widths []style.Unit,
	at, baseline style.Unit, stack *lineStack, scale float64) {

	// The items in the order they are *drawn*, because §8.6's pieces are visual:
	// a box whose content the reordering cut in two generates two boxes, and the
	// one in the middle of them belongs to something else. Walking the logical
	// order and taking a box's extremes would draw one piece straight through
	// the other box's words.
	order := make([]int, 0, len(items))
	for k := range items {
		// An item with no width puts no ink anywhere, so it cannot come between
		// two pieces of a box and make them two. A bidi control the shaper drops
		// is such an item, and it belongs to whichever box the author wrote it
		// in — so one written *outside* a span, between two of that span's own
		// items, cut the span into pieces that then drew a border apiece.
		// bidi-011 is a <span> holding an override with the matching pop after
		// it, and it came out as three boxes with two seams.
		if items[k].Width == 0 && !items[k].Inset {
			continue
		}
		order = append(order, k)
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })

	// The boxes the item before this one sat in, outermost first, each with the
	// piece it has open and the extent it has covered so far: the chain of the
	// last item, kept as a stack. A box absent from the item before this one
	// has been interrupted, so what follows is a new piece rather than more of
	// the old one — and a box in both is at the same depth of both chains,
	// because a chain is the painting boxes above an item, outermost first.
	//
	// The stack is what keeps a line linear in what it holds. Walking the whole
	// chain of every item and widening every piece in it cost the depth for each
	// item, and d bordered spans nested round a word are 3d items, each inside
	// up to d of them: the square of the depth. Here an item widens only the
	// innermost open piece, and a piece hands what it covered to the one round
	// it when it closes, so each item costs what it changes: the boxes it opens
	// and the ones it closes, each once.
	type openPiece struct {
		box         *Box
		piece       int // index in d.pieces, or -1 when room refused one
		left, right style.Unit
	}
	var open []openPiece
	// close pops the open down to n boxes, writing each piece's extent and
	// handing it to the piece round it.
	close := func(n int) {
		for len(open) > n {
			top := open[len(open)-1]
			open = open[:len(open)-1]
			if top.piece >= 0 {
				d.pieces[top.piece].left, d.pieces[top.piece].right = top.left, top.right
			}
			if len(open) > 0 {
				under := &open[len(open)-1]
				under.left, under.right = min(under.left, top.left), max(under.right, top.right)
			}
		}
	}

	for _, k := range order {
		item := items[k]
		chain := d.l.inlineChain(item)
		left := at.Add(xs[k])
		// The width the item took on *this* line, which for a space a
		// justified line stretched is more than the font gave it.
		right := left.Add(widths[k])
		// Less the letter-spacing gap at its far edge, which is not part of any
		// box: §8.2 puts the gap *between* two typographic character units, so a
		// box's ink stops at its last glyph and the space after it belongs to
		// the paragraph. letter-spacing-nesting-003 asks for it in as many
		// words — "the green rectangle does not extend beyond B to C".
		//
		// At the right edge whichever way the run reads, which is where the
		// drawing puts it — see gapNeighbour. The same gap is what lineOffsets
		// moves the box's own ending edge across.
		if sp := item.EdgeLetterSpacing; sp != 0 && !item.Inset {
			right = right.Sub(sp)
		}
		// How much of the item before's chain this one shares. The two agree
		// up to some depth and nowhere below it, so the walk down from the
		// shorter length stops at the first box they share, and it passes only
		// boxes that are about to be closed.
		keep := min(len(open), len(chain))
		for keep > 0 && open[keep-1].box != chain[keep-1] {
			keep--
		}
		close(keep)
		for _, box := range chain[keep:] {
			piece := -1
			if d.room(box) {
				_, seen := d.last[box]
				// §10.8.1's vertical-align on this box, which moved its text and
				// has to move its ink by exactly as much. It is the *box's* own
				// accumulation and not the item's: the item carries the sum down
				// to the innermost box it sits in, and a fragment for a box
				// halfway up that chain is placed by the sum down to itself.
				//
				// The extents are the box's line-height split around its
				// baseline — §10.8's inline box — rather than the font's content
				// area the fragment is drawn over, which is §10.6.1's and a
				// different question.
				base := baseline
				var offset Point
				if box.IsText() {
					// Text a "display: contents" link stands for, which is on
					// no record of the flattening's: those are the element
					// boxes'. Its alignment and its displacement are its
					// item's, as its runs' are. See textLinkArea.
					if item.Valign.Aligned() {
						base = base.Add(stack.Shift(item.Valign, item.Above, item.Below))
					}
					offset = item.Offset
				} else if va, ok := d.l.inlineAligns[box]; ok {
					above, below := d.l.leadingAt(box, box.FontSize.Mul(scale))
					base = base.Add(stack.Shift(va, above, below))
				}
				d.pieces = append(d.pieces, inlinePiece{
					box: box, line: index, left: left, right: right,
					baseline: base, first: !seen, scale: scale, offset: offset,
				})
				if d.last == nil {
					d.last = make(map[*Box]int)
				}
				piece = len(d.pieces) - 1
				d.last[box] = piece
			}
			open = append(open, openPiece{box: box, piece: piece, left: left, right: right})
		}
		if n := len(open); n > 0 {
			// The item widens the innermost piece it is in; the ones round that
			// learn of it when it closes.
			top := &open[n-1]
			top.left, top.right = min(top.left, left), max(top.right, right)
		}
	}
	close(0)
}

// room reports whether another piece may be recorded, and says so once when it
// refuses.
func (d *inlineDecor) room(b *Box) bool {
	if d.l.inlineDecorations < maxInlineDecorations {
		d.l.inlineDecorations++
		return true
	}
	if !d.l.inlineDecorCapped {
		d.l.inlineDecorCapped = true
		d.l.rec.Report(RuleLimit, AtHTML(offsetOf(b)),
			"more inline boxes have a background, a border or a link to paint, over "+
				"more lines, than this engine will draw; the rest were left undrawn, "+
				"a link among them is not in the display list there, and "+
				"their text is unaffected")
	}
	return false
}

// finish turns the pieces into fragments and hangs each on its line.
//
// The box model is resolved here rather than in addLine because §8.6's slice
// model needs both ends of the box: a fragment carries the box's left margin,
// border and padding only if it begins the box, and its right ones only if it
// ends it, and the second of those is a fact about every other line.
func (d *inlineDecor) finish(parent *Fragment) {
	carry := d.insetCarriers()
	for i := range d.pieces {
		p := &d.pieces[i]
		b := p.box
		if p.line >= len(parent.Lines) {
			// A bounds check on an index the caller computed, not a case that
			// happens: addLine is given the position the line is about to be
			// appended at, on the statement before the append. It is here because
			// the alternative to a skip is a panic on an untrusted document, and
			// because the two are far enough apart in inlineContent for a later
			// change to break the invariant quietly.
			continue
		}

		if b.IsText() {
			// Text standing directly in a "display: contents" link, which is
			// on the line for its link's area and for nothing else: the
			// element around it generated no box, so there is no margin,
			// border, padding or outline of its own to read — the text box
			// carries the element's style, and reading them there would give
			// it those of the box that is not there. See Box.contentsLink.
			d.textLinkArea(parent, p)
			continue
		}

		margin := d.l.edges(b, "margin", d.containing)
		border := d.l.borderWidths(b)
		padding := d.l.paddingOf(b, d.containing)

		// §8.6, and the same two flags insetItems reads, mapped to sides the
		// same way: a piece of an inline box split by a block inside it does not
		// begin or does not end the box, and neither does a line that continues
		// one. Which physical side "begins" it is the containing block's
		// business — see splitInsetSides.
		noLeft, noRight := splitInsetSides(b)
		// §8.6 names two of a box's pieces and gives the rest nothing: the one at
		// the end the box *begins* on, on the first line it appears on, carries
		// the box's starting inset, and the one at the other end on the last line
		// carries the ending inset. Which physical end begins it is the box's own
		// direction — see beginsAtRight — and insetCarriers has already found the
		// two pieces.
		//
		//	All other generated boxes for the element have no horizontal
		//	margins, borders or padding.
		//
		// Reading "the first piece" as "the left inset" is the left-to-right half
		// of the rule written down as though it were the whole of it.
		c := carry[b]
		startsRight := beginsAtRight(b)
		keepLeft := (!startsRight && i == c.start) || (startsRight && i == c.end)
		keepRight := (startsRight && i == c.start) || (!startsRight && i == c.end)
		if !keepLeft || noLeft {
			margin.Left, border.Left, padding.Left = 0, 0, 0
		}
		if !keepRight || noRight {
			margin.Right, border.Right, padding.Right = 0, 0, 0
		}
		// §8.3: margin-top and margin-bottom do not apply to a non-replaced
		// inline box at all. They are zeroed so that the fragment says what was
		// used — a border and a padding on this axis are painted and a margin is
		// not — and nothing reads them: MarginRect is never asked of one of these,
		// which a planted defect confirmed by setting both to 99 and breaking no
		// test. It is a statement about the value rather than a computation
		// anything depends on, and it is written down as one.
		margin.Top, margin.Bottom = 0, 0

		// §10.6.1: the content area is the font's, not the line's — and text-fit
		// scales the size the font is used at, so the box's own ink grows and
		// shrinks with the type inside it. The suite's
		// text-fit/grow-per-line-all-line-height is a one-pixel lime border round
		// two letters on a line scaled by two, and asks for a box twice as tall.
		st := d.l.strutAt(b, b.FontSize.Mul(p.scale))
		x := p.left.Add(margin.Left)
		frag := &Fragment{
			Box: b, Margin: margin, Border: border, Padding: padding,
			Outline: d.l.outlineWidth(b),
			BorderRect: Rect{
				X: x,
				Y: p.baseline.Sub(st.Ascent).Sub(padding.Top).Sub(border.Top),
				W: p.right.Sub(margin.Right).Sub(x),
				H: st.Ascent.Add(st.Descent).
					Add(padding.Vertical()).Add(border.Vertical()),
			},
			// §9.4.3's displacement, accumulated over the inline boxes this one
			// sits inside and including its own. It is folded into the position
			// by absolutise rather than applied at paint time, because a
			// background image is placed against the rectangle the box is drawn
			// at and this is the only rectangle it has.
			Offset: d.l.inlineOffsets[b],
		}
		if b.areaLink() != nil {
			// The link's area on this line, which is this fragment's border
			// box. A copy, so that an <a> with a background as well, whose
			// fragment is also a Boxes entry, is not moved twice by
			// absolutise. See LineFragment.links.
			lf := *frag
			parent.Lines[p.line].links = append(parent.Lines[p.line].links, &lf)
			if !d.l.inlinePaints(b) && !b.Position.positioned() {
				// In the chain for its link and for nothing else: it has no
				// ink, and a Boxes entry is ink to everything that reads one.
				continue
			}
		}
		parent.Lines[p.line].Boxes = append(parent.Lines[p.line].Boxes, frag)
		if b.Position.positioned() {
			// Recorded for §10.1: an absolutely positioned descendant of this
			// box is placed against the bounding box of its first and last
			// fragments. They are in the line's coordinates here and are made
			// absolute with everything else — see absolutise — and the
			// candidates that read them are placed after that.
			//
			// Through the journal, because a pass that is thrown away has to take
			// them back: an item laid out to be measured recorded its fragments
			// here too, and the first of them — which is never made absolute,
			// being in a tree nobody keeps — was the corner a box inside it was
			// positioned against. See speculative.go.
			d.l.addInlineFragment(b, frag)
		}
	}
}

// textLinkArea hangs the area of a piece of text a "display: contents" link
// stands for on its line: §10.6.1's content area of the text, as an inline
// box's is, with no inset of any kind. See Box.contentsLink.
//
// It is not charged to room here: addLine did, when it made the piece.
func (d *inlineDecor) textLinkArea(parent *Fragment, p *inlinePiece) {
	b := p.box
	st := d.l.strutAt(b, b.FontSize.Mul(p.scale))
	parent.Lines[p.line].links = append(parent.Lines[p.line].links, &Fragment{
		Box: b,
		BorderRect: Rect{
			X: p.left, Y: p.baseline.Sub(st.Ascent),
			W: p.right.Sub(p.left), H: st.Ascent.Add(st.Descent),
		},
		// §9.4.3's displacement of the boxes around the text, as its runs
		// are drawn at; the baseline has §10.8.1's already. Both are its
		// item's, from addLine.
		Offset: p.offset,
	})
}

// insetEnds is the two pieces of one box that carry its insets.
type insetEnds struct{ start, end int }

// insetCarriers finds them, per box.
//
// §8.6 asks for the piece at one end of the *first line* the box appears on and
// the piece at the other end of the *last line* — where the ends are physical
// and which is which is the box's own direction. With one piece per line those
// are the first and last pieces, which is what this used to assume; with a box
// the reordering cut into several pieces on one line they are the extremes of
// that line, and every piece between them carries nothing.
//
// Two walks of the pieces: one for each box's first and last line, and one
// for the pieces at the ends of those. It walked every piece once for every
// box, which a line of d painting spans made the square of d.
func (d *inlineDecor) insetCarriers() map[*Box]insetEnds {
	type span struct {
		first, last int
		ends        insetEnds
	}
	lines := map[*Box]*span{}
	for i := range d.pieces {
		p := d.pieces[i]
		s, ok := lines[p.box]
		if !ok {
			lines[p.box] = &span{first: p.line, last: p.line, ends: insetEnds{-1, -1}}
			continue
		}
		s.first, s.last = min(s.first, p.line), max(s.last, p.line)
	}
	for i := range d.pieces {
		p := d.pieces[i]
		s := lines[p.box]
		startsRight := beginsAtRight(p.box)
		// The end the box begins on, on its first line: the rightmost piece
		// when it begins at its right, and the leftmost otherwise.
		if p.line == s.first && (s.ends.start < 0 || further(p, d.pieces[s.ends.start], startsRight)) {
			s.ends.start = i
		}
		// And the other end on its last line.
		if p.line == s.last && (s.ends.end < 0 || further(p, d.pieces[s.ends.end], !startsRight)) {
			s.ends.end = i
		}
	}
	out := make(map[*Box]insetEnds, len(lines))
	for b, s := range lines {
		out[b] = s.ends
	}
	return out
}

// further reports whether a is nearer the right end than b, or nearer the left
// end when right is false.
func further(a, b inlinePiece, right bool) bool {
	if right {
		return a.right > b.right
	}
	return a.left < b.left
}

// inlineChain is the inline boxes an item sits inside that need a fragment on
// its line — see paintedInlines — outermost first.
func (l *layouter) inlineChain(item inlineItem) []*Box {
	start := heldBox(item.Box)
	if start == nil {
		return nil
	}
	if item.AtomicBox != nil {
		// A replaced element or an inline-block has a fragment of its own, and
		// that fragment's background and border are painted by the machinery
		// every other box uses. What is wanted here is what encloses it.
		start = start.Parent
	}
	return l.paintedInlines(start)
}

// paintedInlines walks up from a box to the inline boxes around it, keeping the
// ones with a background, a border or an outline, the positioned ones and the
// links.
//
// The walk stops at the first ancestor that is not an inline box, which is the
// block container whose lines these are — and at an atomic inline, which is a
// formatting context of its own and paints itself.
//
// A text box is walked *through* rather than kept, and that is load-bearing
// rather than tidiness: a text box carries its parent element's whole computed
// style, background-color and all, so keeping it would paint the parent's
// background a second time — and would paint a *block's* background over its own
// text, since the text box inside a <p> is an inline box by this test.
//
// A box's chain is its parent's with at most the box itself added, and it is
// built that way: the walk goes up only as far as the first box already
// answered, and each box on the way down extends the chain above it. Walked
// whole from every box, a paragraph of spans nested d deep with a word in each
// asked d boxes each whether they paint — the square of the depth, parsing
// backgrounds and borders all the way. The chains share their prefixes, so they
// are read-only, which the one caller already treats them as.
func (l *layouter) paintedInlines(b *Box) []*Box {
	if b == nil {
		return nil
	}
	if got, ok := l.inlineChains[b]; ok {
		return got
	}
	// Up to the first box answered, or to where every chain ends.
	var path []*Box
	var out []*Box
	for cur := b; cur != nil; cur = cur.Parent {
		if got, ok := l.inlineChains[cur]; ok {
			out = got
			break
		}
		path = append(path, cur)
		if cur.Outer != OuterInline || cur.Replaced != nil || isAtomicInline(cur) {
			// The block container whose lines these are, or an atomic inline,
			// which is a formatting context of its own and paints itself: no
			// chain reaches past either, and neither is on one.
			out = nil
			break
		}
	}
	// Down again, outermost first, which is tree order among boxes that nest —
	// and so the order Appendix E paints them in, each over the one it is
	// inside.
	for i := len(path) - 1; i >= 0; i-- {
		cur := path[i]
		switch {
		case cur.Outer != OuterInline || cur.Replaced != nil || isAtomicInline(cur):
			out = nil
		case cur.IsText():
			// Walked through, unless it is text a "display: contents" link
			// stands for, which has an area of its own to be given and
			// nothing to paint. See Box.contentsLink and finish.
			if cur.contentsLink != nil {
				out = append(out[:len(out):len(out)], cur)
			}
		case l.inlinePaints(cur) || cur.Position.positioned() || cur.areaLink() != nil:
			// A *positioned* inline box is kept whether or not it draws
			// anything, because §10.1 forms the containing block of an
			// absolutely positioned descendant from the padding boxes of this
			// box's own fragments — so the fragments have to exist. It paints
			// nothing extra: a fragment with no background and no border draws
			// nothing, exactly as it did when there was no fragment at all.
			//
			// A link is kept for the same reason: its fragments are the area
			// the display list's Link covers. They go on the line's links and
			// not on its Boxes unless the box has ink as well — see finish.
			//
			// A copy rather than an append in place, which would write into
			// the spare room of the chain above, and so into a sibling's.
			out = append(out[:len(out):len(out)], cur)
		}
		l.inlineChains[cur] = out
	}
	return out
}

// inlinePaints reports whether an inline box has anything to draw.
//
// It is asked before a fragment is made rather than after, because the fragment
// is what costs: an ordinary document's inline boxes are <em> and <a> with no
// background and no border, and making a rectangle for each of them on each line
// would be work in proportion to the document that nothing would ever read.
//
// An outline is something to draw. Leaving it out made an outline on a <span>
// with no background or border no fragment at all, so there was nothing for the
// outline pass to ring (audit C96).
func (l *layouter) inlinePaints(b *Box) bool {
	if got, ok := l.inlineDraws[b]; ok {
		return got
	}
	draws := l.hasOwnBackground(b) || l.borderWidths(b) != (Edges{}) || l.outlineWidth(b) > 0
	l.inlineDraws[b] = draws
	return draws
}
