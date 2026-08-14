package layout

import (
	"github.com/mgilbir/forme/paragraph"
	"strings"

	"github.com/mgilbir/forme/shape"
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

// LineFragment is a line box: one row of text within a block.
type LineFragment struct {
	// Rect is the line box in the same coordinates as the fragment holding it.
	Rect Rect
	// Baseline is the distance from the top of the line box to the baseline the
	// text sits on. Painting needs it, and it is not derivable afterwards —
	// half-leading is split above and below the text.
	Baseline style.Unit
	// Runs are the pieces of text on the line, in *reading* order — the order
	// they were written, which on a line that mixes directions is not the order
	// they are drawn in. Where each one goes is its own X.
	//
	// The two are kept apart on purpose. The order here is the order the runs
	// reach the content stream, and so the order a reader extracting text from
	// the finished page gets them in; a right-to-left paragraph has to be drawn
	// the way it reads and copied out the way it was written, and only the X
	// decides the first.
	Runs []TextRun
	// Boxes are the fragments of the inline boxes that have a background or a
	// border on this line, in tree order — so an inner box's decoration is
	// painted over the box it is inside.
	//
	// They are fragments rather than a shape of their own because everything that
	// paints a background already works on one, and they hang from the line
	// rather than from the block's children for two reasons: Appendix E paints
	// them with the line's own content and not with the block backgrounds, and
	// one inline box produces one of them *per line* — see inlinepaint.go for
	// §8.6's slice model, which is the whole of why this is plural.
	//
	// A box with nothing to draw has none. The overwhelming majority of the
	// inline boxes in a document are an <em> or an <a> with no background and no
	// border, and a rectangle for each of them on each line would be work in
	// proportion to the document that nothing would ever read.
	Boxes []*Fragment
}

// TextRun is a piece of text on a line, set in one face at one size.
type TextRun struct {
	Text string
	// Face is what it is set in, and Size the font size.
	Face *shape.Face
	Size style.Unit
	// X is the offset from the left of the line box, and Width the advance.
	X, Width style.Unit
	// Box is the inline box the text came from, which carries the colour and
	// the decoration painting will need.
	Box *Box
	// Decorations are the lines ruled across this run: CSS 2.1 §16.3.1's
	// underline, overline and line-through. They are on the run rather than
	// derived from Box at paint time because a decoration belongs to whichever
	// *ancestor* declared it, and that box's colour is the line's colour — see
	// textdecoration.go, where the difference between propagating and inheriting
	// is worked through.
	Decorations []textDecoration
	// RTL says the run reads right to left, so its glyphs are drawn from the
	// right edge of its box towards the left and its brackets are mirrored.
	//
	// It is on the run rather than derived from the text because it is not a
	// property of the text: a run of punctuation between two Hebrew words is
	// right-to-left and has nothing in it that says so. The algorithm decided it
	// from the neighbours, which are other runs by the time anything paints this
	// one.
	RTL bool
	// LetterSpacing is what letter-spacing added after each character of this
	// run. It is carried into painting as well as into the width because the two
	// have to agree: the width decided where the next run starts, and glyphs
	// drawn without the same spacing would leave a gap the size of the whole
	// run's spacing before it.
	LetterSpacing style.Unit
	// Offset is §9.4.3's relative displacement, accumulated over the inline
	// boxes this run sits inside.
	//
	// It is on the run rather than on a single fragment because a <span> that
	// spans a line break has one fragment per line: a line box holds runs, and
	// the box's own background and border are a Boxes entry on each of the lines
	// it reaches. Both are moved by the same displacement and are given it
	// separately — this one at paint time, the fragment's in absolutise, because
	// a background image is placed against the rectangle its box is drawn at.
	Offset Point
	// Shift is how far this run's own baseline sits below the line box's,
	// which is §10.8.1's vertical-align applied to the inline box the run came
	// from. It is negative for a run that is raised.
	//
	// It is a length on the run rather than a line of its own because a line
	// box has exactly one baseline — §10.8's alignment is *against* that
	// baseline — and every run on the line is placed relative to it. Folding it
	// into Offset would have been shorter and is wrong: Offset is §9.4.3's
	// relative positioning, which moves a box after layout without changing
	// anything about the line, whereas this displacement is part of what
	// decided the line's height.
	Shift style.Unit
}

// heldBox and heldFragment take back what an itemRef holds.
//
// This is the layer that knows what one is, and these two are the only place
// that says so. They return the zero value rather than panicking on a ref of the
// other kind, because the caller has already decided which kind it is asking for
// by the field it read it from — a float ref is never a fragment.
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
				parent.Children = append(parent.Children,
					l.floatChild(heldBox(items[i].Float), width, origin, y, style.MaxUnit, 0, 0))
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
					kid := l.floatChild(heldBox(f.Box), width, origin, y, baseRoom.Sub(f.Used), lh, 0)
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
						X: x, Width: item.Width, Box: heldBox(item.Box), Offset: item.Offset,
						Decorations: decorations, LetterSpacing: item.Spacing.Letter,
						RTL:   item.Level&1 == 1,
						Shift: shift,
					})
				}
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
				if forced || next >= len(items) {
					used = style.Max(used, style.Min(total, avail))
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
				rtl := lineBaseIsRTL(b, runs)
				shift := lineIndent.Add(l.alignLine(b, rtl, avail, used))
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
				decor.addLine(len(parent.Lines), runs, xs,
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
				parent.Children = append(parent.Children,
					l.floatChild(heldBox(f.Box), width, origin, y, lineWidth.Sub(f.Used), lh, 0))
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

// strutFor measures the block's own font at its own size.
func (l *layouter) strutFor(b *Box) strut {
	s := strut{Height: l.lineHeight(b), Baseline: l.baselineOf(b, l.lineHeight(b))}
	face, ok := l.fontFor(b)
	if !ok {
		return s
	}
	d := face.Descriptor()
	upem := float64(face.UnitsPerEm())
	if upem == 0 {
		return s
	}
	if ascent, descent, ok := l.lineExtents(b, face); ok {
		s.Ascent, s.Descent = ascent, descent
	}
	// The x-height "vertical-align: middle" is measured against.
	//
	// The face's own when it states one, and otherwise the same two estimates
	// decorationMetrics falls back to — seven tenths of the cap height, or half
	// an em. The three cases and their order are deliberately identical to that
	// function's, because the two are one question: a document whose strike and
	// whose middle-aligned box sat at different heights would be answering it
	// twice, differently, for no reason a reader could see.
	//
	// Nothing this engine read stated an x-height when this was written, so the
	// estimate was the only answer available and the agreement was free. The
	// standard fourteen state one now, and keeping the two in step costs this
	// branch. TestTheStrikeAndAMiddleAlignedBoxUseOneXHeight is what says they
	// are still in step.
	s.XHeight = b.FontSize.Mul(0.5)
	switch {
	case d.Has(shape.MetricXHeight) && d.XHeight > 0:
		s.XHeight = b.FontSize.Mul(float64(d.XHeight) / upem)
	case d.CapHeight > 0:
		// Cap height is reported, and x-height is about seven tenths of it for
		// the faces that report either.
		s.XHeight = b.FontSize.Mul(float64(d.CapHeight) / upem * 0.7)
	}
	return s
}

// leading is how far a run of text in an inline box reaches above and below the
// baseline it sits on: CSS 2.1 §10.8.1's leading, half above the font's ascent
// and half below its descent.
//
// It is the same arithmetic the strut is measured by, and deliberately so — the
// strut *is* an inline box, the block's own, and the only thing that makes it
// special is that it is on every line whether or not anything else is. Sharing
// the formula is what keeps a line of plain text exactly as tall as it was: the
// text box inherits the block's font and line-height, so its two numbers are the
// strut's two numbers and the maximum below changes nothing.
//
// The half-leading may be negative — "line-height: 0" asks for a box shorter
// than its own type — and it is passed on rather than clamped, because that is
// precisely how a stylesheet packs lines closer than the font wants.
func (l *layouter) leading(b *Box) (above, below style.Unit) {
	h := l.lineHeight(b)
	above = l.baselineOf(b, h)
	return above, h.Sub(above)
}

// verticalAlignOf reads the vertical-align property of an inline-level box.
func (l *layouter) verticalAlignOf(b *Box) (vAlign, style.Unit) {
	raw := strings.ToLower(strings.TrimSpace(b.Style["vertical-align"]))
	switch raw {
	case "", "baseline":
		return vAlignBaseline, 0
	case "top":
		return vAlignTop, 0
	case "bottom":
		return vAlignBottom, 0
	case "middle":
		return vAlignMiddle, 0
	case "text-top":
		return vAlignTextTop, 0
	case "text-bottom":
		return vAlignTextBottom, 0
	case "sub":
		// The specification leaves the distance to the engine. A fifth of the
		// font size is what browsers use.
		return vAlignBaseline, style.Unit(0).Sub(b.FontSize.Mul(0.2))
	case "super":
		return vAlignBaseline, b.FontSize.Mul(0.33)
	}
	// A length raises the box by that much; a percentage is of the line-height,
	// which is the one property whose percentages are of line-height rather than
	// of the containing block. It is the box's *own* line-height and not the
	// block's: §10.8.1 says "a percentage of the 'line-height' value" with no
	// qualification, and an unqualified percentage in CSS is of the element's own
	// value of the property named.
	if length, ok := l.parseLength(b, "vertical-align"); ok {
		if v, ok := length.Resolve(l.lineHeight(b), true); ok {
			return vAlignBaseline, v
		}
	}
	return vAlignBaseline, 0
}

// vAlignFor combines an inline box's own vertical-align with what the boxes
// around it already asked for.
func (l *layouter) vAlignFor(b *Box, in vAlignState) vAlignState {
	own, lift := l.verticalAlignOf(b)
	switch own {
	case vAlignTop, vAlignBottom:
		// A new aligned subtree, placed against the line box. Nothing outside it
		// carries in: whatever raised the boxes around this one, this one's top
		// or bottom is the line's.
		return vAlignState{LineAlign: own, Subtree: b}
	case vAlignBaseline:
		in.Raise = in.Raise.Add(lift)
		return in
	}
	// "middle", "text-top" or "text-bottom": a position against the parent, so
	// it replaces the accumulated displacement and stays in whatever subtree it
	// was already in.
	//
	// The reset is written twice and that was established rather than assumed.
	// alignedExtents reads raise only in its baseline case, so clearing it here
	// changes no answer today: planted on its own, this line decides nothing and
	// no test in the package moves. Planted *together* with a raise added to one
	// of the keyword cases, the pair is caught — so the rule is guarded, by
	// whichever of the two a later change leaves standing. It is kept because it
	// is what makes the field's documented meaning true, and a stale displacement
	// travelling in a struct that calls it the displacement is the sort of thing
	// the next reader spends an afternoon on.
	in.Align, in.Raise = own, 0
	return in
}

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
	l.checkGlyphs(b, face)

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

		para, start, end := frame.Bidi.Add(p.Text)
		item := inlineItem{
			BidiPara: para, BidiStart: start, BidiEnd: end,
			Text: p.Text, Box: b, Face: face, Size: size,
			Leads: true, Above: above, Below: below,
			// §10.8.1's vertical-align, which a text box cannot be asked for
			// itself: the property is not inherited, so the anonymous box holding
			// a <span>'s words carries the initial value however the span was
			// aligned. The frame brought the answer down from the boxes the walk
			// is inside.
			Valign: frame.Valign,
			// An opportunity carried in from the piece before is offered to
			// anything but a space. UAX #14's LB7 — "do not break before spaces"
			// — is an earlier rule than every rule that creates one, so a space
			// belongs to the unit in front of it and the break falls after it.
			// The piece's own opportunity still stands, which is what puts the
			// break after a preserved space rather than losing it.
			BreakBefore: p.BreakBefore || (state.BreakOpportunity && !p.Space),
			Space:       p.Space, Collapsible: p.Collapsible,
			TrimAtEnd: p.TrimAtEnd,
			Tab:       p.Tab, TabStop: tabStop, TabFloor: tabFloor,
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
			NoWrap:    !ws.Wrap, Offset: offset,
			BreakWord:   ow.BreakWord,
			Anywhere:    ow.Anywhere,
			Decorations: decorations, Spacing: spacing,
		}
		if !p.Tab {
			// A tab is measured against a tab stop when it lands, so there is
			// nothing to measure here and the face's own advance for U+0009 —
			// whatever a face happens to give a character it has no glyph for —
			// would be the wrong number to carry.
			item.Width = l.br.MeasureSpaced(face, p.Text, size, spacing)
		}
		out = append(out, item)
		state = inlineState{AfterCollapsibleSpace: p.Collapsible}
	}
	return out, inlineState{
		BreakOpportunity:      endedAtBreak,
		AfterCollapsibleSpace: state.AfterCollapsibleSpace,
	}
}

// tabStop is the distance between two tab stops, which is what tab-size sets.
//
// A number is a count of space advances in the box's own font, which is why
// this needs the face; a length is itself. The initial value is 8, the width
// every terminal and every editor has used for a tab since they had one.
func (l *layouter) tabStop(b *Box, face *shape.Face) style.Unit {
	raw := strings.TrimSpace(b.Style["tab-size"])
	if n, ok := parseNumber(raw); ok {
		return l.br.Measure(face, " ", b.FontSize).Mul(n)
	}
	if v, ok := l.lengthOf(b, "tab-size", 0); ok && v >= 0 {
		return v
	}
	return l.br.Measure(face, " ", b.FontSize).Mul(8)
}

// lineClamp is how many lines CSS Overflow 4 lets this block show, or zero for
// no limit.
//
// Two spellings, and the second is not a synonym for the first. The unprefixed
// property is read on its own; the prefixed one is only the clamp when the two
// declarations that made it work in the engine it came from are there as well —
// §"Legacy" gives the trio as "display: -webkit-box", "-webkit-box-orient:
// vertical" and "-webkit-line-clamp". A document that writes only
// "-webkit-line-clamp" on an ordinary block is not asking for anything, and
// browsers give it nothing.
//
// A count of zero or less clamps nothing rather than clamping everything away:
// the value is an integer with no stated floor, and a block with no lines at all
// is not something a stylesheet can plausibly be asking for.
func (l *layouter) lineClamp(b *Box) int {
	if n, ok := positiveInteger(b.Style["line-clamp"]); ok {
		return n
	}
	if !strings.EqualFold(strings.TrimSpace(b.Style["display"]), "-webkit-box") ||
		!strings.EqualFold(strings.TrimSpace(b.Style["-webkit-box-orient"]), "vertical") {
		return 0
	}
	if n, ok := positiveInteger(b.Style["-webkit-line-clamp"]); ok {
		return n
	}
	return 0
}

// balanceCaps is the balanced width for each line, by the item it starts at.
//
// One number would not do, because §5.1 balances "each group of lines separated
// by a forced line break" on its own: a headline of two lines with a <br> in the
// middle is two groups of one, and balancing the pair together would move the
// break the author wrote. text-wrap-balance-004 is that exactly — a <section>
// with a <br> in it, checked against two <div>s balanced separately.
//
// The text indent belongs to the first group alone, for the same reason it
// belongs to the first line: §16.1 gives it to the first formatted line of the
// element, and the line after a <br> is not one.
//
// A nil result means no cap anywhere, which is what a box that does not balance
// gets and is what capAt reads as MaxUnit.
func (l *layouter) balanceCaps(b *Box, items []inlineItem, width, indent style.Unit) []style.Unit {
	if !strings.EqualFold(strings.TrimSpace(b.Style["text-wrap-style"]), "balance") {
		return nil
	}
	caps := make([]style.Unit, len(items))
	for i := range caps {
		caps[i] = style.MaxUnit
	}
	start := 0
	for i := 0; i <= len(items); i++ {
		if i < len(items) && !items[i].Forced {
			continue
		}
		ind := style.Unit(0)
		if start == 0 {
			ind = indent
		}
		w := l.br.BalanceWidth(items[start:i], width, ind)
		for j := start; j < i; j++ {
			caps[j] = w
		}
		start = i + 1
	}
	return caps
}

// reportOverflow names content too wide for the box holding it.
//
// It is reported once per piece of text rather than once per line, because a
// paragraph containing one impossible word would otherwise complain on every
// line it wraps to.
func (l *layouter) ReportOverflow(item inlineItem, width style.Unit) {
	what := "the text " + quoteValue(item.Text)
	key := item.Text
	if item.Atomic != nil {
		// A replaced element has no text to name it by, and two different
		// images of the same width are two findings rather than one — so the
		// key is where it is in the document rather than what it says.
		what = "the image"
		key = "\x00replaced\x00" + PathOf(heldBox(item.Box).Element)
	}
	if l.reportedOverflow[key] {
		return
	}
	l.reportedOverflow[key] = true
	l.rec.ReportDetail(Finding{
		Rule: RuleUnbreakableOverflow,
		Message: what + " is " +
			fmtPx(item.Width) + " wide and cannot be broken, in a space " +
			fmtPx(width) + " wide; the part past the edge will not be drawn",
		Path: PathOf(heldBox(item.Box).Element),
	})
}

// lineHeight resolves the line-height property.
//
// "normal" is the face's own recommendation, which for the metrics this engine
// has means about 1.2 times the size — the figure every renderer uses when a
// face does not say otherwise. A bare number is a multiplier rather than a
// length, which is the one place in CSS where that is true and the reason
// line-height is usually written that way: a multiplier inherits as a ratio and
// a length inherits as a fixed distance.
func (l *layouter) lineHeight(b *Box) style.Unit {
	value := strings.ToLower(strings.TrimSpace(b.Style["line-height"]))
	if value == "" || value == "normal" {
		return l.normalLineHeight(b)
	}
	if n, ok := parseNumber(value); ok {
		return b.FontSize.Mul(n)
	}
	if length, ok := l.parseLength(b, "line-height"); ok {
		if v, ok := length.Resolve(b.FontSize, true); ok {
			return v
		}
	}
	return l.normalLineHeight(b)
}

// normalLineHeight is "line-height: normal".
//
// Worth recording what changed when the font finally got asked. Ahem states
// ascent 800, descent -200 and a line gap of zero, which comes to exactly one
// em — right for a face whose every glyph is an em square, and a figure no
// constant would have produced. Noto Sans comes to more than 1.2. Neither is
// something an engine can guess, which is the whole argument for asking.
func (l *layouter) normalLineHeight(b *Box) style.Unit {
	face, ok := l.fontFor(b)
	if !ok {
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	var gap float64
	if d := face.Descriptor(); d.Has(shape.MetricLineGap) {
		gap = float64(d.LineGap)
	}
	// One multiplication over the summed ratio rather than three over the terms.
	// A layout unit is a 64th of a pixel and each product is rounded to one, so
	// adding three rounded products is not the same number as rounding the sum —
	// it is out by up to a unit and a half, which is enough to move a line.
	h := b.FontSize.Mul((top - bottom + gap) / upem)
	if h <= 0 {
		// A face stating metrics that sum to nothing would collapse every line
		// on the page. It is not a value to pass on.
		return b.FontSize.Mul(normalLineHeightFallbackFactor)
	}
	return h
}

// lineExtents is lineMetrics at a box's font size.
func (l *layouter) lineExtents(b *Box, face *shape.Face) (ascent, descent style.Unit, ok bool) {
	top, bottom, upem, ok := lineMetrics(face)
	if !ok {
		return 0, 0, false
	}
	return b.FontSize.Mul(top / upem), b.FontSize.Mul(-bottom / upem), true
}

// baselineOf is where the text sits within a line box.
//
// The line box is usually taller than the text, and the difference — the leading
// — is split equally above and below it. That is what makes a paragraph's lines
// evenly spaced rather than crowded against their tops.
func (l *layouter) baselineOf(b *Box, lineHeight style.Unit) style.Unit {
	face, ok := l.fontFor(b)
	if !ok {
		return lineHeight.Mul(0.8)
	}
	ascent, descent, ok := l.lineExtents(b, face)
	if !ok {
		return lineHeight.Mul(0.8)
	}
	halfLeading := lineHeight.Sub(ascent).Sub(descent).Div(2)
	return halfLeading.Add(ascent)
}

// reportWordBreak reports a word-break value this engine reads as normal.
//
// "break-all" is implemented; "keep-all" and "auto-phrase" are not, and both
// *remove* or *move* opportunities rather than adding them — keep-all stops CJK
// text breaking between two ideographs, and auto-phrase moves a Korean break to
// a phrase boundary. Ignoring either breaks a line somewhere the author said not
// to, which no amount of looking at the page reveals as a missing feature.
//
// Once per value per box, for the same reason checkScript is once per script.
func (l *layouter) reportWordBreak(b *Box, value string) {
	if l.reportedWordBreak == nil {
		l.reportedWordBreak = map[string]bool{}
	}
	if l.reportedWordBreak[value] {
		return
	}
	l.reportedWordBreak[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "word-break",
		Message: value + " was read as normal, so a line may break where the " +
			"value asked it not to",
		Path: PathOf(b.Element),
	})
}

// reportLineBreak reports a line-break value this engine reads as auto.
//
// Unlike its word-break counterpart it is conditional on the text, and the
// condition is what keeps it honest. loose, normal and strict differ from auto
// only in how strictly CJK text may break — around small kana, around iteration
// marks, before centred punctuation — and this engine's whole CJK rule is
// "between two ideographs", which all three leave alone. Over Latin text the
// three values provably change nothing, and the suite says so: pre-wrap-004,
// -005 and -006 exist to assert that "XX    XX" wraps the same under loose,
// normal and strict as under auto. Warning there would be crying wolf on a page
// that is correct.
//
// So the report is made where the difference could show, which is text with an
// ideograph in it — the only text this engine breaks by a rule the three values
// have anything to say about.
func (l *layouter) reportLineBreak(b *Box, value string) {
	if !strings.ContainsFunc(b.Text, isIdeographic) {
		return
	}
	if l.reportedLineBreak == nil {
		l.reportedLineBreak = map[string]bool{}
	}
	if l.reportedLineBreak[value] {
		return
	}
	l.reportedLineBreak[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "line-break",
		Message: value + " was read as auto, so CJK text may break where the " +
			"value asked it not to",
		Path: PathOf(b.Element),
	})
}

// checkScript reports text this engine cannot break or order correctly.
//
// It is the unsupported-script guardrail of §6.3, and it is an error by default
// for the reason given there: unbroken or unordered text still looks like text,
// so the failure mode looks like success. A paragraph of Thai run together as
// one word overflows silently; a line of Arabic laid out left to right reads as
// a rendering bug rather than as something this engine declined to do.
func (l *layouter) checkScript(b *Box) {
	for _, r := range b.Text {
		if script, bad := unsupportedScript(r); bad {
			key := script + "\x00" + b.Style["font-family"]
			if l.reportedScripts[key] {
				return
			}
			l.reportedScripts[key] = true
			l.rec.ReportDetail(Finding{
				Rule:    RuleUnsupportedScript,
				Message: script,
				Path:    PathOf(b.Element),
			})
			return
		}
	}
}

// checkGlyphs reports characters the chosen face has no glyph for.
//
// This is the glyph-missing guardrail of §6.3, an error by default because tofu
// is the purest form of silent garbage: a reader who sees a row of boxes where
// letters should be blames their PDF viewer, not the document, and the author
// never hears about it at all.
//
// It is reported once per character rather than once per occurrence, because
// what an author needs to know is *which* characters their font cannot set —
// hearing it four hundred times about the same one is not four hundred times as
// useful.
func (l *layouter) checkGlyphs(b *Box, face *shape.Face) {
	// The question has to be the one *drawing* answers, and it was not.
	//
	// This asked face.GlyphID, which is whether the face has a glyph mapped to
	// a code point. Shaping asks something different and gets a different
	// answer: a no-break space has no glyph of its own and is set as a space, a
	// bidi override has none and takes no room at all, and the same goes for
	// every fixed-width Unicode space and every zero-width format control. All
	// of them draw correctly, and all of them were being reported — at Error
	// severity, the one that stops a document being produced.
	//
	// Measured over the reftest suite, that was the single most common finding
	// in the whole engine: 154 documents reported a missing glyph for the
	// no-break space alone, and 260 documents were kept out of the clean-pass
	// count by nothing else. A guardrail wrong that often is worse than no
	// guardrail, because the reports it is right about are buried.
	//
	// Shaping the whole run first is also what makes this cheap: the answer is
	// almost always that nothing is missing, and only then is it worth walking
	// the characters to find out which.
	if !missesVisible(face, b.Text) {
		return
	}
	for _, r := range b.Text {
		if r == '\n' || r == '\t' || marksNoPaper(r) {
			continue
		}
		if _, missing := face.ShapeGlyphs(string(r)); missing == 0 {
			continue
		}
		key := string(r) + "\x00" + face.Name()
		if l.reportedGlyphs[key] {
			continue
		}
		l.reportedGlyphs[key] = true
		l.rec.ReportDetail(Finding{
			Rule: RuleGlyphMissing,
			Message: "the face " + quoteValue(face.Name()) + " has no glyph for " +
				describeRune(r) + ", which is set as a space, so the character is " +
				"missing from the page and from the text extracted out of it",
			Path: PathOf(b.Element),
		})
	}
}

// faceForText is fontFor, with a substitution when the family the document asked
// for cannot set the text.
//
// The standard fourteen faces cover Latin and nothing else, so a document with a
// Hebrew word in it gets a face that cannot encode the letters — and the encoder
// substitutes a space for each, which means the word is absent from the page and
// from the text extracted out of it rather than showing as boxes a reader would
// notice. That is what checkGlyphs reports, and reporting it is not the same as
// fixing it: a caller that has a face with the letters should be able to say so.
//
// FallbackFontSet is how it says so, and this is the only place that asks. The
// substitution is reported, because it changes the metrics and therefore where
// every line breaks — the same reason a missing family is reported.
//
// Two limits, both deliberate and both visible in what they leave behind:
//
// It is per box. A box whose text mixes scripts that no single face covers keeps
// the family's face and reports the missing glyphs, because choosing one face
// for the box cannot help it. Cutting a run into per-face pieces is what
// shape.Stack does and it reaches into measurement, line breaking and the
// content stream; until that exists, this handles the common shape — a run of
// text that is all one script.
//
// It is cached per box rather than per family, because the answer depends on the
// text. Shaping a run to find out whether it is covered is not free, and
// itemsFor is on the hot path.
func (l *layouter) faceForText(b *Box) (*shape.Face, bool) {
	face, ok := l.fontFor(b)
	if !ok || b.Text == "" {
		return face, ok
	}
	if got, cached := l.textFaces[b]; cached {
		return got, got != nil
	}
	chosen := face
	if missesVisible(face, b.Text) {
		if set, canFall := l.fontSet.(FallbackFontSet); canFall {
			bold := isBold(b.Style["font-weight"])
			italic := isItalic(b.Style["font-style"])
			if alt, found := set.FaceFor(b.Text, bold, italic); found {
				chosen = alt
				l.rec.ReportDetail(Finding{
					Rule: RuleFontFallback,
					Message: "no face for " + quoteValue(b.Style["font-family"]) +
						" could set this text, so " + quoteValue(alt.Name()) +
						" was used for it; the metrics and the line breaks will differ",
					Path:     PathOf(b.Element),
					Property: "font-family",
				})
			}
		}
	}
	l.textFaces[b] = chosen
	return chosen, true
}
