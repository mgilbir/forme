package layout

import (
	"github.com/mgilbir/forme/paragraph"

	"github.com/mgilbir/forme/style"
)

// Inline layout: text into lines.
//
// §1 of the rendering proposal calls this the deceptive one, and it is right —
// line boxes, breaking, baseline alignment and whitespace at line edges are
// individually modest and collectively larger than flexbox. What is here is the
// part that puts words on a page: measuring runs against a real face, finding
// where a line may break, and stacking the lines.
//
// # Where a line may break, and what is refused
//
// Doing this properly is UAX #14, a table-driven Unicode algorithm keyed on a
// line-breaking class per code point. What is implemented is a subset, and the
// boundary of the subset is stated here rather than discovered:
//
//   - after a space or a preserved tab;
//   - after a run of preserved spaces, and — under break-spaces — after each
//     one of them;
//   - after an "other space separator" that UAX #14 puts in class BA or ID —
//     everything in the Zs category except U+0020 and U+00A0, less the two the
//     algorithm calls GL, which are U+2007 FIGURE SPACE and U+202F NARROW
//     NO-BREAK SPACE. break-spaces overrides the two exceptions, because that
//     value puts an opportunity after every separator without qualification;
//   - at a zero-width space, which is how an author marks an opportunity
//     inside a word;
//   - after a hyphen that is not the last character of its run;
//   - on both sides of an ideograph.
//
// Two of the omissions are worth naming because they are the ones that show. A
// break is not taken *before* an ideograph or an ideographic space where the
// text before it is Latin, though LB31 allows one; and the class table is a
// handful of ranges rather than the whole of it. Both err towards fewer
// opportunities, which overflows a line rather than breaking it in the wrong
// place — the direction that fails visibly.
//
// That covers Latin, Greek, Cyrillic and the CJK scripts, which is most of what
// a document generator sets. Everything else UAX #14 says is *not* done, and the
// family that matters is not approximated: the scripts that need a dictionary to
// find a word boundary — Thai, Lao, Khmer, Burmese.
//
// It is refused through the finding vocabulary rather than guessed at, because
// §6.3 is exactly right about it: unbroken text still looks like text, so the
// failure mode looks like success. A paragraph of Thai run together as one
// unbreakable word overflows silently and reads as a rendering bug rather than
// as an unimplemented feature.
//
// The other refusal that used to be here — the bidirectional reordering a
// right-to-left line needs — is not a refusal any more. bidi.go resolves the
// levels over each paragraph and this file reorders the runs of each line once
// it has been broken; see visualRuns below.
//
// word-break: break-all *is* implemented, and the machinery it needed is worth
// naming because it turned out to be the answer to a question this file had
// wrong for everything, not only for break-all.
//
// It asks for a break *between two characters*, and CSS Text §2 says which
// positions those are: a soft wrap opportunity falls between typographic
// character units, which is the grapheme cluster. Breaking inside one splits a
// letter from its accent.
//
// The shaper's clusters are the obvious candidate for those positions and are
// not them, which was measured rather than assumed — graphemecluster_test.go
// shapes five strings whose UAX #29 segmentation is not in doubt and finds
// forme's clusters finer than the grapheme clusters in every one: a base with
// two combining marks breaks between the marks, a keycap breaks off its digit,
// conjoining Hangul breaks into three, a flag breaks into two letters, and a
// Thai spacing mark leaves its consonant. In a right-to-left run the clusters
// are not even ordered — the glyphs come back as they are drawn — so "the
// cluster changed" does not name a position in the text.
//
// So internal/grapheme is a UAX #29 segmenter over the *characters*, which is
// where this belongs anyway since splitAtBreaks never sees a glyph. Every cut
// this file makes now goes through it, not only break-all's, and that caught a
// break the ideograph rule had been offering since it was written: a Hangul
// syllable spelled as a precomposed LV plus its own trailing jamo was cut in
// two, so a line could end in the middle of one letter.
//
// keep-all and auto-phrase are still not implemented, and both *remove* or
// *move* an opportunity rather than adding one — so ignoring either breaks a
// line where the author said not to, and both are reported.
//
// overflow-wrap is not implemented and is reported. It is a different shape of
// problem from break-all rather than a smaller one: its opportunities exist
// only when the line cannot otherwise fit, so it is not a question about the
// text at all but about what the breaker should do having failed once.
//
// # White space at a line edge
//
// The rest of CSS Text §4 lives here, and whitespace.go explains why it is
// split across three stages rather than done once. What this file owns is
// §4.1.2: the collapsible space removed at each end of a line, the tab stops a
// preserved tab advances to, and the preserved space that hangs past the line's
// end instead of pushing the next word down.
//
// The hang has two strengths and the difference is not cosmetic. A line that
// ended at a soft wrap hangs its trailing white space *unconditionally* — it is
// never counted for fit or alignment. A line that ended at a forced break, which
// includes the end of the content, hangs it *conditionally*: it hangs only if it
// does not otherwise fit, so a space that fits is content and the line is
// aligned around it. inlineContent applies the second and alignedWidth the
// first.

// # What relative positioning does to an inline box, and what it does not
//
// §9.4.3 applies to an inline box like any other, and the offset is carried on
// each run rather than on one fragment because an inline box does not have one:
// a <span> broken across a line contributes to two line boxes and to neither
// exclusively. It has a fragment *per line* — see LineFragment.Boxes and
// inlinepaint.go — and the offset reaches those the same way, recorded per box
// as the walk accumulates it and folded into the position by absolutise.
//
// What is given up is the *stacking level*. §9.9 makes a positioned inline a
// positioned box, painted at Appendix E step 7 with everything else positioned;
// its runs are painted at step 6 with the rest of the block's text. The two
// differ only where a relatively positioned inline's text overlaps a positioned
// box that comes earlier in the document — every other pair is ordered the same
// way by both rules, because step 6 already comes after every block background
// and every float and before every positioned box. Closing it would mean
// splitting a line box's runs into stacking levels, which is a change to the
// shape of a line rather than an addition to it.

// heldBox and heldFragment take back what an itemRef holds.
//
// This is the layer that knows what one is, and these two are the only place
// that says so. They return the zero value rather than panicking on a ref of the
// other kind, because the caller has already decided which kind it is asking for
// by the field it read it from — a float ref is never a fragment.
// shiftedBy adds the displacement of the inline boxes a float was written
// inside to the fragment it was laid out into.
//
// It is added to the fragment's offset rather than to its position because
// §9.4.3's offset is applied after layout: the float still occupies the band it
// was placed in, and the text around it still flows past that band. What moves
// is only what is drawn — which is exactly what an offset is, and is why an
// inline with "left: 2em" round a float moves the float and does not move the
// hole it left.
func shiftedBy(f *Fragment, d Point) *Fragment {
	if f != nil {
		f.Offset.X = f.Offset.X.Add(d.X)
		f.Offset.Y = f.Offset.Y.Add(d.Y)
	}
	return f
}

func heldBox(r itemRef) *Box {
	b, _ := r.(*Box)
	return b
}

func heldFragment(r itemRef) *Fragment {
	f, _ := r.(*Fragment)
	return f
}

// inlineContent lays a box's inline children into lines and returns the height
// they need.
//
// # Why this drives the line breaking rather than calling it
//
// Before floats, every line in a block was the same width and the whole
// paragraph could be broken in one go. It cannot be now. The width available to
// a line depends on which floats overlap the y the line starts at, and that y
// depends on how tall the lines above it were — so the available width is not
// known until the previous line has been placed. Each line is therefore measured
// against its own band, one at a time.
//
// The floats themselves are met *while* the lines are being built, which is the
// second reason the loop is here: a float's position depends on the line it
// appears on, and the lines after it depend on the float.
//
// # What decides where a finished line sits
//
// Two things move a line's content within the band it was measured against, and
// they are added rather than chosen between: §16.1's text-indent, which applies
// to the first line box only and is taken off the room that line has for text,
// and §16.2's text-align, which distributes whatever is left over. Both are
// applied after the line has been broken, because the width a line is *aligned*
// at is not the width it was *broken* at — §4.1.2 excludes a hanging space "for
// fit, alignment, or justification", and the fit has already happened. textalign.go
// has the rest of that argument.
func (l *layouter) inlineContent(b *Box, parent *Fragment, width style.Unit, origin flow) style.Unit {
	st := l.strutFor(b)
	// The block container is one bidi paragraph — or several, where it holds a
	// forced break — and its own unicode-bidi may wrap the whole of it in an
	// override. Both are decided before the walk, because the walk is what fills
	// the paragraph in.
	_, open, closing := paragraphDirection(b)
	para := newBidiBuilder(open)
	items, _ := l.collectInline(b, l.markerItems(b), startOfContext(), inlineFrame{
		Containing: width, CbHeight: origin.cbHeight, CbDefinite: origin.cbDefinite,
		Strut: st, Bidi: para,
	})
	para.Leave(open, closing)
	if len(items) == 0 {
		return 0
	}
	items = l.resolveBidi(b, items, para)

	lo, hi := origin.x, origin.x.Add(width)

	// §16.1's indent, which applies to the first line box this container
	// generates and to no other. It is resolved once: a percentage is of this
	// box's own content width, which is the width being laid out in.
	indent := l.textIndent(b, width)
	firstLine := true
	_ = firstLine

	// CSS Overflow 4's line-clamp: how many more lines the clamps in force
	// allow this block, and the room the mark will need on the last of them.
	//
	// Not this box's own property. The clamp counts *descendant* line boxes, so
	// a block inside a clamp container is under a clamp it never declared and a
	// clamp container's own budget has already been spent by whichever of its
	// descendants was laid out first. Both are the same question, and clamp.go
	// answers it — including whether anything is discarded at all, which is why
	// there is no count of the content here any more.
	maxLines, clampEllipsis, clamped := l.clampRoom()

	// §5.1's balancing, as a cap on how wide a line may be broken.
	//
	// It is a cap and not a width: the line boxes still span the band, and only
	// the *breaking* is done in the narrower measure. Narrowing the box itself
	// would move a centred line and shorten a right-aligned one, which is a
	// different rendering from the one balancing asks for.
	balanceCaps := l.balanceCaps(b, items, width, indent)
	if balanceCaps != nil && clamped && maxLines > 0 {
		// Balancing a clamped block is a different question, because the clamp
		// has already decided how many lines there are: any width at all
		// produces that many, so "the narrowest width with the same line count"
		// asks nothing. What must not change is how much of the content is
		// *shown* — §5.1 evens out the lines, it does not throw more away — so
		// the search is over the reach instead. See balanceClampedWidth.
		w := l.br.BalanceClampedWidth(items, width, indent, clampEllipsis, maxLines)
		for i := range balanceCaps {
			balanceCaps[i] = w
		}
	}
	// Whether any line of a balanced box turned out to be shortened by a float,
	// which is what says the balancing has to be done again in the widths those
	// lines really had.
	balanceMetFloat := false

	// The inline boxes with a background or a border, gathered as the lines are
	// built and turned into fragments once the last line is known: §8.6 puts a
	// box's trailing margin, border and padding on the piece it ends with, and
	// which piece that is cannot be answered until there are no more.
	var decor inlineDecor

	// Where this box's lines begin, so that a first attempt at them can be taken
	// back. Balancing beside a float needs the widths the lines actually had,
	// and those are not known until the lines exist: the floats inside the box
	// are placed as the loop meets them, and what shortens a line is decided by
	// the lines above it. So the box is laid out once to find out, thrown away,
	// and laid out again in the measure that answer gives.
	//
	// It is thrown away with the same three handles the per-line retry above
	// uses — the float context, the out-of-flow queue and the fragment's own
	// children — plus its lines. Anything else the pass touched is a memo keyed
	// by box, and recomputing it gives the same answer.
	linesAt, kidsAt := len(parent.Lines), len(parent.Children)
	ctxAt, absAt := origin.ctx.mark(), len(l.deferred)

	var y style.Unit
	// The width each line turned out to have, which is the band a float left it.
	var bands []style.Unit
	// The width to break each line at, chosen by the scored search. It is per
	// line rather than per box because the answer is a break *set*: with the
	// lines no longer all the same width there is no single measure that
	// produces it. See balanceScoredCaps.
	var lineCaps []style.Unit
	// The bands the pass before this one measured, so that the two can be
	// compared. A float met part-way down the box is placed on the line it
	// appears on, so laying the lines out differently can move it — and then the
	// widths the search was given are not the widths it produced.
	var wasBands []style.Unit
	// iByte is how far into items[i] the next line begins, which is other than
	// zero only where overflow-wrap ended the line inside a word.
	var iByte int
	for pass := 0; ; pass++ {
		y, iByte = 0, 0
		bands = bands[:0]
		firstLine = true
		balanceMetFloat = false
		decor = inlineDecor{l: l, containing: width, strut: st}
		for i := 0; i < len(items); {
			// Where this pass started, so that the foot of the loop can tell whether
			// it moved. Nothing in the body increments the cursor on its own: it is
			// carried entirely by what breakOneLine hands back.
			wasI, wasByte := i, iByte
			// A float that begins a line is placed before the line is measured,
			// because it is one of the floats the line has to avoid. §9.5.1 rule 4
			// puts its top at the top of the line box it belongs to.
			for iByte == 0 && i < len(items) && items[i].Float != nil {
				parent.Children = append(parent.Children, shiftedBy(
					l.floatChild(heldBox(items[i].Float), width, origin, y, style.MaxUnit, 0, 0),
					items[i].Offset))
				i++
			}
			if i >= len(items) {
				break
			}

			left, right := origin.ctx.bandAt(origin.y.Add(y), lo, hi)
			// What has to fit beside a float is what this line will actually start
			// with, which is the *remainder* of a word the line before broke — not
			// the whole word, whose width would push the line down past a float it
			// has room beside.
			firstItem := items[i]
			if iByte > 0 {
				_, firstItem = l.br.SplitItem(firstItem, iByte)
			}
			y, left, right = l.roomForLine(firstItem, origin, y, left, right, lo, hi)

			// The indent is taken off the room the first line has for text, not off
			// the line box: the line box still spans the band, and its content starts
			// further in. Taking it off the box instead would make a "text-align:
			// right" first line end short of the right edge by the indent, which is
			// the opposite of what §16.1 asks for.
			lineIndent := style.Unit(0)
			if firstLine {
				lineIndent = indent
			}
			// The room the ellipsis needs on this line, which is the last one the
			// clamp allows and nothing before it.
			ending := l.clampEndingHere()
			lineEllipsis := style.Unit(0)
			if ending != nil {
				lineEllipsis = ending.ellipsis
			}

			// A line is shortened by every float its *box* meets, not only by the
			// ones its top edge meets — §9.5's "line boxes created next to the float
			// are shortened to make room for the margin box of the float", where
			// "next to" is a relation between two rectangles. So the band has to be
			// asked over the line's height, and the line's height is not known until
			// the line has been broken against a band.
			//
			// The circle is broken the way browsers break it: break against the band
			// at the top, measure, ask again over the height that produced, and
			// break again if the answer moved. A float whose top is below the top of
			// the line is exactly the case a single-y query cannot see, and the suite
			// tests it by name in floats-wrap-top-below-inline-*.
			var (
				runs     []inlineItem
				next     int
				nextByte int
				mid      []midLineBox
				forced   bool
				lh, bl   style.Unit
				stack    lineStack
				midKids  []*Fragment
				after    []midLineBox
				baseRoom = right.Sub(left)
			)
			// Where this line's own floats begin, so that an attempt that has to be
			// made again can put the context back as it found it. The floats that
			// *started* the line are before the mark and stay.
			midMark, midAbs := origin.ctx.mark(), len(l.deferred)
			for attempt := 0; ; attempt++ {
				origin.ctx.truncate(midMark)
				l.deferred = l.deferred[:midAbs]
				midKids = midKids[:0]

				runs, next, nextByte, mid, forced = l.br.BreakOneLine(items, i, iByte,
					// The cap is a *line* width, so the indent comes off it and not
					// off the band before it: the search counted the first line's
					// room as the balanced width less the indent, and taking the
					// indent off the band instead makes the two disagree by exactly
					// the indent on the one line it applies to.
					style.Min(right.Sub(left), lineCap(balanceCaps, lineCaps, i, len(parent.Lines))).
						Sub(lineIndent).Sub(lineEllipsis),
					left.Sub(lo).Add(lineIndent))
				stack = stackLine(runs, st)
				lh, bl = stack.Height, stack.Baseline

				// A float met part-way along the line is placed *now*, before the
				// band is asked again, because §9.5 shortens "the current and
				// subsequent line boxes" — the line it appears on included. Placing
				// it after the line was settled, which is what this used to do, left
				// the words it should have pushed aside sitting underneath it: the
				// suite's floats-001, -006, -029, -030 and -031 each put a float
				// after the content of a line and check that the content moved.
				//
				// The room it is offered is measured in the band the line started
				// from rather than in the one it has been narrowed to. That is what
				// makes the loop settle: narrowing the band and then asking again
				// whether the float fits in what is left of it counts the float
				// twice, and a float that fits on one attempt stops fitting on the
				// next and starts fitting again on the one after. Against the
				// original band the advance can only shrink as the band narrows, so
				// a float that fits goes on fitting.
				//
				// A float that ends up *below* the line — because it did not fit
				// beside what is already there, or because it cleared something —
				// is taken back out and placed once the line is settled. It is not
				// beside the line, so it does not shorten it, and leaving it in
				// would: a line box is a little taller than the box on it, so a
				// float sitting under the line reaches a few pixels into the range
				// the band is asked over and narrows it to nothing. floats-
				// placement-006 is that exactly, an inline-block and a "clear: both"
				// float that between them drove the line two hundred pixels down
				// the page.
				//
				// From the first such float onwards the rest are placed after the
				// line too, in order. Rolling one back is a truncation, so it can
				// only be the last thing in the context; and a float placed while an
				// earlier one is missing is placed against the wrong obstacles.
				after = after[:0]
				for _, f := range mid {
					if f.Abs {
						continue
					}
					if len(after) > 0 {
						after = append(after, f)
						continue
					}
					held, heldAbs := origin.ctx.mark(), len(l.deferred)
					kid := shiftedBy(
						l.floatChild(heldBox(f.Box), width, origin, y, baseRoom.Sub(f.Used), lh, 0),
						f.Offset)
					if kid.MarginRect().Y > y {
						origin.ctx.truncate(held)
						// The out-of-flow boxes the discarded layout found go with
						// it, or the float would defer each of them twice when it is
						// laid out again after the line.
						l.deferred = l.deferred[:heldAbs]
						after = append(after, f)
						continue
					}
					midKids = append(midKids, kid)
				}

				if lh <= 0 || attempt >= paragraph.MaxLineFits {
					break
				}
				top := origin.y.Add(y)
				nl, nr := origin.ctx.bandOver(top, top.Add(lh), lo, hi)
				if nl == left && nr == right {
					break
				}
				// The narrower band may leave no room at all, in which case the line
				// drops past the float exactly as it would have at its top edge.
				ny, nl, nr := l.roomForLine(items[i], origin, y, nl, nr, lo, hi)
				if ny == y && nl == left && nr == right {
					break
				}
				y, left, right = ny, nl, nr
			}
			// On the line the clamp ends at, a unit too wide to sit beside the
			// ellipsis is not shown at all. Everywhere else this engine sets an
			// over-long word anyway, because the alternative is losing it; here
			// losing it is exactly what the clamp asks for, and the mark is what
			// stands in its place. "123456789012" against nine characters less an
			// ellipsis leaves the line holding nothing but the mark, which is what
			// line-clamp-004 draws.
			if lineEllipsis > 0 {
				var used style.Unit
				for _, r := range runs {
					used = used.Add(r.Width)
				}
				if used > right.Sub(left).Sub(lineIndent).Sub(lineEllipsis) {
					runs, next, nextByte = nil, len(items), 0
					stack = stackLine(runs, st)
					lh, bl = stack.Height, stack.Baseline
				}
			}

			lineWidth := right.Sub(left)
			if balanceCaps != nil && lineWidth < width {
				balanceMetFloat = true
			}
			bands = append(bands, lineWidth)
			textWidth := lineWidth.Sub(lineIndent)
			if len(runs) > 0 || forced || lineEllipsis > 0 {
				line := LineFragment{
					Rect:     Rect{X: left.Sub(lo), Y: y, W: right.Sub(left), H: lh},
					Baseline: bl,
				}
				// Rules L1 and L2 over the runs of this line: where each of them
				// sits, which on a line mixing directions is not the order they
				// were collected in.
				xs, total := l.lineOffsets(runs)
				// §4.1.2's hang comes in two strengths and the difference shows
				// exactly here. Where the line ended at a soft wrap, its trailing
				// white space hangs *unconditionally* and is never counted, which is
				// what alignedWidth does. Where it ended at a forced break — a <br>,
				// a preserved newline, or simply the end of the content — it hangs
				// *conditionally*: "it hangs only if it does not otherwise fit in the
				// line", so a space that fits is content and the line is aligned
				// around it. The specification's own example is a centred pre-wrap
				// paragraph reading " 0 " in five characters, which centres as three
				// and not as two.
				//
				// The two are not a matter of degree. Counting the space on a
				// soft-wrapped line pushes a right-aligned line a space clear of the
				// edge; not counting it on the last line centres " 0 " off-centre.
				//
				// "Only if it does not otherwise fit" is a rule about each character
				// and not about the sequence, which is the second half of the same
				// paragraph: the UA "may also visually collapse the character advance
				// widths of any that would otherwise overflow". So a sequence that
				// fits counts entirely, one that does not counts up to the line's
				// edge, and the part past the edge hangs. Taking it as all-or-nothing
				// is the reading that was here, and it centres a line of five
				// characters and thirty-two spaces as though it were five: the page
				// shows the letter two characters right of where every browser puts
				// it, and the same document without the spaces is correct, so the
				// fault reads as an alignment bug rather than a white-space one.
				// What the line's own content may occupy. The clamp's mark sits
				// at the end of it, so on that line it is the line less the mark
				// — both for the hang below and for the alignment further down,
				// since the mark is where the line ends and alignment is about
				// where the content ends.
				avail := textWidth.Sub(lineEllipsis)
				used := alignedWidth(runs, total)
				// A last line, in the sense §16.2 and §4.1.2 both use: the
				// author ended it, or the content did. It is where the
				// conditional hang applies, and it is the line justification
				// leaves alone.
				lastLine := forced || next >= len(items)
				if lastLine {
					used = style.Max(used, style.Min(total, avail))
				}
				rtl := lineBaseIsRTL(b, runs)
				align, spread := lineAlignment(b, rtl, lastLine)

				// Justification, before anything is placed.
				//
				// It has to come first because it is the one thing on a line
				// that moves the items *unequally*: the alignment is a single
				// offset the whole line takes, and everything downstream — the
				// runs, the atomic inlines, an inline box's own background and
				// border — is derived from these positions and that one offset.
				// Justifying afterwards moved the text and left the ink of the
				// boxes it was inside where the unjustified line had put it, so
				// a <span> around a justified line was painted a space short of
				// its own last word.
				//
				// The widths travel separately rather than being written back
				// into the items, because the items are the paragraph's and this
				// line is one way of setting them: a re-layout at another width
				// has to start from the widths the font gave.
				widths := make([]style.Unit, len(runs))
				for k := range runs {
					widths[k] = runs[k].Width
				}
				if spread {
					// The method, which is reported here rather than where the
					// property is read: this is the only place that knows a
					// line is being justified at all.
					if _, unhandled := justificationOf(b); unhandled != "" {
						l.reportTextJustify(b, unhandled)
					}
					// A line with nowhere to put the slack is left where it is,
					// and nothing is reported about it: CSS Text 3 §7.3 says a
					// line with no expansion opportunity is aligned as start,
					// so that *is* the conforming rendering.
					justifyItems(runs, xs, widths, hangingTail(runs), avail.Sub(used))
				}
				// Atomic inlines are placed as children of the block rather than as
				// runs, so aligning the line has to move them too. The range is
				// noted here because floats placed before the line are already in
				// this slice and must not move: a float is out of flow and
				// text-align says nothing about it.
				atomicStart := len(parent.Children)
				for k, item := range runs {
					x := xs[k]
					if item.Atomic != nil {
						// Placed as a child of the block rather than as a run,
						// because it is a box: it has a background, a border, a
						// padding and possibly a subtree of its own, and every one
						// of those is painted by machinery that works on fragments.
						// Its margin box hangs from the line's baseline by its own
						// ascent, which is what puts a picture on the line of type
						// and an inline-block's last line of text on it.
						f := heldFragment(item.Atomic)
						f.BorderRect.X = line.Rect.X.Add(x).Add(f.Margin.Left)
						f.BorderRect.Y = y.Add(stack.AtomicTop(item)).Add(f.Margin.Top)
						parent.Children = append(parent.Children, f)
						continue
					}
					if item.Inset {
						// An inline box's own margin, border and padding. It has
						// taken its room on the line already — lineOffsets counted
						// its width, so everything after it is where it belongs —
						// and it draws nothing, so it becomes no run. A run with no
						// text would reach the content stream as an empty
						// text-showing operator and reach a reader extracting the
						// page as nothing at all.
						continue
					}
					shift := style.Unit(0)
					decorations := item.Decorations
					if item.Valign.Aligned() {
						shift = stack.Shift(item.Valign, item.Above, item.Below)
						// §16.3.1: a decoration declared by an ancestor is drawn at
						// *that* box's position and is not moved by the alignment of
						// what it crosses — "text decorations on inline boxes are
						// drawn across the entire element, going across any descendant
						// elements without paying any attention to their presence". So
						// three spans at three different vertical-aligns under one
						// overlining div are crossed by one straight line, not three
						// stepped ones.
						//
						// The copy is made only for a run something moved, which is
						// almost none of them.
						decorations = l.decorationsAt(decorations, &stack)
					}
					line.Runs = append(line.Runs, TextRun{
						Text: item.Text, Face: item.Face, Size: item.Size,
						X: x, Width: widths[k], Box: heldBox(item.Box), Offset: item.Offset,
						Decorations: decorations, LetterSpacing: item.Spacing.Letter,
						RTL:   item.Level&1 == 1,
						Shift: shift,
					})
				}
				// Which *side* it hangs off, which is not a second way of saying how
				// much. §4.1.2 hangs the white space past the line's end, and the
				// end of a right-to-left line is its left edge: rule L1 has already
				// given the trailing spaces the paragraph's own level, so
				// lineOffsets draws them leftmost — at the positions before the
				// first word rather than after the last.
				//
				// So on such a line the content does not begin where alignLine put
				// it, it begins a hang further in. Aligning without this leaves
				// every right-to-left pre-wrap line pushed right by the width of the
				// space it was meant to hang, which is what the ten dir=rtl
				// pre-wrap-align tests measure. It is invisible in a left-to-right
				// document, where the hang follows the content and moves nothing.
				shift := l.alignLine(b, align, avail, used)
				if !rtl {
					// §16.1's indent is measured from the line's *start* edge,
					// and only a left-to-right line starts at the left. The room
					// the line had was already shortened by the indent — see
					// avail — so on a right-to-left line the alignment has
					// already put the content an indent short of the right edge,
					// and adding it again would move it in the wrong direction
					// by twice the distance the author wrote.
					shift = shift.Add(lineIndent)
				}
				if rtl {
					shift = shift.Sub(total.Sub(used))
				}
				if shift != 0 {
					for k := range line.Runs {
						line.Runs[k].X = line.Runs[k].X.Add(shift)
					}
					for k := atomicStart; k < len(parent.Children); k++ {
						parent.Children[k].BorderRect.X =
							parent.Children[k].BorderRect.X.Add(shift)
					}
				}
				// The inline boxes on this line, recorded now because the alignment
				// has moved everything on it for the last time. The index is where
				// the line is about to go, since a fragment cannot be hung on it
				// until §8.6 knows which piece of its box it is.
				decor.addLine(len(parent.Lines), runs, xs, widths,
					line.Rect.X.Add(shift), line.Rect.Y.Add(line.Baseline), &stack)
				if lineEllipsis > 0 && ending.face != nil {
					// The mark goes where the line's own content ends, which is not
					// where the line box does: an aligned line may have been moved,
					// and a right-to-left one ends at its left. alignedWidth is the
					// same measure the alignment used.
					at := shift.Add(used)
					if lineBaseIsRTL(b, runs) {
						at = shift.Sub(lineEllipsis)
					}
					// The clamp container's font and box, not this block's: the
					// mark belongs to the container's root inline box even when
					// the line it lands on came from a descendant.
					line.Runs = append(line.Runs, TextRun{
						Text: blockEllipsis, Face: ending.face, Size: ending.size,
						X: at, Width: lineEllipsis, Box: ending.box,
					})
				}
				parent.Lines = append(parent.Lines, line)
				l.clampLine()
				// The indent belongs to the first line box that actually exists. A run
				// of inline content that produced none — the collapsible space between
				// two block children — has not used it up.
				firstLine = false
			}

			// The floats met along the line were placed while it was being fitted —
			// the line had to be broken against them — but their fragments join the
			// tree here, after the line's own boxes, so that paint order is the
			// order the display list has always had.
			parent.Children = append(parent.Children, midKids...)
			for _, f := range after {
				parent.Children = append(parent.Children, shiftedBy(
					l.floatChild(heldBox(f.Box), width, origin, y, lineWidth.Sub(f.Used), lh, 0),
					f.Offset))
			}

			// An absolutely positioned box met along the line is dealt with once the
			// line is settled, because until it is neither its top nor its left edge
			// is known, and both are what it needs.
			for _, f := range mid {
				if f.Abs {
					// §10.6.4's static position for a box written among the words:
					// the top of the line box it appeared on, and the pen position
					// it appeared at. The x is taken from the line's own left edge
					// rather than the block's, so a box written beside a float
					// records the position it would really have had.
					//
					// §10.3.7 asks for the same position from the other side, because
					// a right-to-left containing block anchors "right" rather than
					// "left". f.used is the advance in *logical* order, which is from
					// the left on a left-to-right line and from the right on a
					// right-to-left one — so the two are mirror images and neither is
					// the negation of the other: one counts from the line's left edge
					// and the other from the block's content right edge, and the line
					// need not reach either.
					abs := heldBox(f.Box)
					if !abs.staticInline {
						// §10.3.7's hypothetical box for a box that would have been
						// block-level had it been static: a block box, whose margin
						// edges are the containing block's content edges. Only the
						// line it was written on is taken from here — which is what
						// makes this different from the block walk's answer and the
						// reason the box is collected among the words at all.
						//
						// Reading the pen position instead is invisible until
						// something moves the line's own left edge, and then it is
						// not: float-in-inline-001 and position-absolute-007 in the
						// suite each write such a box beside a float and check that
						// the float did not carry it along.
						l.deferAbsolute(abs, parent, 0, y, 0, 0)
						continue
					}
					l.deferAbsolute(abs, parent, left.Sub(lo).Add(f.Used), y,
						width.Sub(right.Sub(lo).Sub(f.Used)), 0)
				}
			}

			i, iByte = next, nextByte
			// The cursor has to move, and only forwards. A break that hands back the
			// position it was given is a bug in breakOneLine rather than anything a
			// document can ask for — but the loop has no increment of its own, so
			// such a break is not a wrong line, it is a render that never finishes:
			// the same line is measured, appended and measured again until the
			// process is killed. Two of the mutation plants over this function reach
			// exactly that state, and unguarded each one takes 25GB before the OOM
			// killer arrives.
			//
			// So the cursor is pushed past the item it stalled on. What that costs is
			// the rest of one item, and what it buys is the same bargain paragraph.MaxLineFits
			// strikes: output that is slightly wrong beats output that never comes.
			//
			// The comparison is a function of its own because it is the part that can
			// be got wrong quietly: the recovery below cannot be reached by any
			// document, so a test can only reach the decision.
			if !cursorAdvanced(wasI, wasByte, i, iByte) {
				i, iByte = wasI+1, 0
			}
			if len(runs) > 0 || forced || lineEllipsis > 0 {
				// Only a line that exists occupies a line's height. A run of inline
				// content that is nothing but the collapsible space between two
				// block children produces no line box at all, and giving it one
				// would put a blank line into every document whose markup is
				// indented.
				//
				// The clamp's own line exists even when nothing of the content fits
				// on it, because the ellipsis is on it: a block clamped to three
				// lines whose third holds only the mark is three lines tall.
				y = y.Add(lh)
			}
			if l.clampReached() {
				// The clamp: everything after this line is discarded, which is what
				// "continue: discard" means and what the ellipsis just said. It is
				// asked of the clamps and not of this box's line count, because
				// the line that used the last of the budget may have been made by
				// a block laid out before this one.
				break
			}
		}
		if pass > 0 && sameUnits(bands, wasBands) {
			// The lines came out in the widths the search was given, so the
			// answer is consistent with itself and there is nothing to redo.
			break
		}
		if pass >= paragraph.MaxBalancePasses-1 {
			// It has not settled and will not be given more attempts. What is on
			// the page is a balanced layout measured in *some* set of bands the
			// box really produced, which is a great deal closer than one
			// measured in a width it never had.
			break
		}
		if !balanceMetFloat || clamped {
			// Nothing to do again: the box does not balance, or no float reached
			// into it. A clamped box is left to the first answer as well — the
			// clamp's own search is over how far the content reaches rather than
			// over a line count, and the two have not been put together.
			break
		}
		wasBands = append(wasBands[:0], bands...)
		parent.Lines = parent.Lines[:linesAt]
		parent.Children = parent.Children[:kidsAt]
		origin.ctx.truncate(ctxAt)
		l.deferred = l.deferred[:absAt]
		// The scored search first, because it is the one that can answer when
		// the lines have different room. Where it declines — a paragraph too
		// long to search, or one whose lines cannot be made to come out at the
		// count the first pass found — the width search stands, and its answer
		// is at least measured in the right bands.
		lineCaps = l.br.BalanceScoredCaps(items, bands, indent, len(bands))
		if lineCaps == nil {
			w := l.br.BalanceWidthInBands(items, bands, width, indent)
			for i := range balanceCaps {
				balanceCaps[i] = w
			}
		}
	}
	decor.finish(parent)
	return y
}

// roomForLine moves a line down past floats that leave it no usable width.
//
// CSS 2.1 §9.5 says a line box that is too small to hold any content is shifted
// downwards until it either fits or there are no floats left. Without this a
// paragraph beside two facing floats would have every line clipped to nothing
// and the text would vanish — a failure that produces a page with a hole in it
// and no other symptom, which is the shape §6 exists to prevent.
//
// It moves only for the *first* item of the line. An item that does not fit on a
// full-width line is genuinely too wide and is reported as an overflow rather
// than chased down the page for ever.
func (l *layouter) roomForLine(first inlineItem, origin flow, y, left, right, lo, hi style.Unit) (
	style.Unit, style.Unit, style.Unit) {

	for left != lo || right != hi {
		if right.Sub(left) > 0 && (first.Space || first.Width <= right.Sub(left)) {
			break
		}
		next, ok := origin.ctx.nextBottomBelow(origin.y.Add(y))
		if !ok {
			break
		}
		y = next.Sub(origin.y)
		left, right = origin.ctx.bandAt(origin.y.Add(y), lo, hi)
	}
	return y, left, right
}
