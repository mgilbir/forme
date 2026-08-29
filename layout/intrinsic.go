package layout

import (
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// Intrinsic widths: how wide a box wants to be when nothing tells it.
//
// Everything else in this engine sizes a box from the outside in — a width is
// declared, or it fills what the containing block leaves. A float cannot be
// sized that way. CSS 2.1 §10.3.5 gives it the *shrink-to-fit* width, and
// shrink-to-fit is defined in terms of two numbers that come from the content:
//
//   - the **preferred width**, which is how wide the content would be if it were
//     never broken — every line of a paragraph run together as one;
//   - the **preferred minimum width**, which is how narrow the box can get
//     before the content overflows it — the widest thing that cannot be broken,
//     usually the longest word.
//
// The result is min(max(minimum, available), preferred): as wide as the content
// wants, capped by the space there is, and never squeezed below what would
// overflow.
//
// This is not an optimisation and it is not a nicety. A float whose width filled
// its containing block would leave nothing beside it, so no line box would ever
// be shortened and the whole feature would appear to work while doing nothing.
//
// # What is approximated, and where that shows
//
// The two widths are computed over the box tree rather than by a trial layout,
// which is what makes them cheap and is where they are inexact:
//
//   - A percentage width contributes as though it were auto. CSS Sizing says a
//     percentage that cannot be resolved behaves as auto for intrinsic sizing,
//     which is the same answer for the common case and not for a percentage
//     larger than the content.
//   - Break opportunities are found within each text box rather than across the
//     boundary between two, so "<em>super</em>market" is measured as two words
//     rather than one. That makes the minimum a little small; it never makes it
//     too large, so a float sized by it fits where it should.
//   - A table is measured by §17.5.2.2's own two passes rather than by the walk
//     here, because a table's width is a property of its columns and not of any
//     one of its children. That lives in tablelayout.go and is reached from
//     measureWidths.

// intrinsicWidths is the pair, measured across a box's *margin* box so that the
// caller can compare them against a containing block directly.
type intrinsicWidths struct {
	min, max style.Unit
}

// keywordWidth resolves "width: min-content" and "width: max-content" to the
// number they name.
//
// The two are the only intrinsic keywords this engine accepts as a declared
// width, and they are the two that need nothing but the content: CSS Sizing §3.2
// says to "use the min-content size" or "the max-content size in the relevant
// axis", and both of those are numbers this file already computes for every box
// so that a float can shrink to fit. What is left out is left out for a reason
// rather than for want of time:
//
//   - fit-content and stretch are sizes *of the space available*, and the space
//     available is not a property of the box. A float's is the band it lands in,
//     which is being searched for when its width is asked for, so the two would
//     have to be resolved together. They stay reported by checkIntrinsicSizing.
//   - The height half of both keywords is, per §3.2, "equivalent to its
//     automatic size" for a block size — which is what this engine already does
//     with a height it cannot parse, so the page is right and the finding is the
//     only thing wrong. Correcting the finding is not the same work as this and
//     is not done here.
//
// box-sizing is deliberately not applied. §3.3 is explicit: "non-quantitative
// values such as auto and min-content are not influenced by the box-sizing
// property", so the keyword names a content width under either value and
// subtracting the padding and border from it — which is what every other
// declared width here does — would be wrong by exactly those edges.
//
// It is refused for the boxes whose width is decided somewhere else. A table's
// comes from §17.5.2 over its columns and a replaced element's from its own
// intrinsic size, and neither reads this; answering here would produce a number
// that is then ignored, and the finding those boxes still get is the honest
// report.
func (l *layouter) keywordWidth(b *Box) (style.Unit, bool) {
	if !acceptsKeywordWidth(b) {
		return 0, false
	}
	switch bareSizingKeyword(b.Style["width"]) {
	case "min-content":
		return l.contentWidths(b).min, true
	case "max-content":
		return l.contentWidths(b).max, true
	}
	return 0, false
}

// fitContentWidth is the keyword the two above leave out, answered where the
// space available is known.
//
// §3.1: fit-content is min(max-content, max(min-content, available)), which is
// CSS 2.1 §10.3.5's shrink-to-fit over the space the containing block leaves —
// so what this adds over the two above is not a formula but a caller that has
// the number. resolveWidth has it: it is the only place a box's width is
// decided against a containing block that has already been resolved.
//
// It is the one keyword whose answer differs from "auto" only for an in-flow
// block. A float, an inline-block and an absolutely positioned box all shrink to
// fit when their width is auto, so for those three fit-content asks for the
// width they already have; a plain block fills its containing block instead, and
// that is the difference this is for.
func (l *layouter) fitContentWidth(b *Box, available style.Unit) (style.Unit, bool) {
	if !acceptsKeywordWidth(b) || bareSizingKeyword(b.Style["width"]) != "fit-content" {
		return 0, false
	}
	return l.shrinkToFit(b, available), true
}

// keywordLimit resolves an intrinsic keyword on min-width or max-width to a
// content width.
//
// §3.1 again, and the limits are the easier half: a minimum or a maximum is a
// number compared against a width rather than a width to be laid out to, so
// there is no formula to interleave with the margins. fit-content is left out
// for the reason keywordWidth leaves it out and clampWidth cannot fix — the
// clamp is applied after the margins are resolved and does not know what space
// was available — and stretch with it.
func (l *layouter) keywordLimit(b *Box, property string) (style.Unit, bool) {
	if !acceptsKeywordWidth(b) {
		return 0, false
	}
	switch bareSizingKeyword(b.Style[property]) {
	case "min-content":
		return l.contentWidths(b).min, true
	case "max-content":
		return l.contentWidths(b).max, true
	}
	return 0, false
}

// acceptsKeywordWidth reports whether a box's width is decided by the sizing
// path that reads the keyword.
func acceptsKeywordWidth(b *Box) bool {
	if b.Replaced != nil || b.TableWrapper {
		return false
	}
	switch b.Inner {
	case InnerFlow, InnerFlowRoot:
		return true
	}
	return false
}

// shrinkToFit is CSS 2.1 §10.3.5's formula.
//
// available is what the containing block leaves after this box's own margins,
// border and padding, so the widths compared against it are content widths.
func (l *layouter) shrinkToFit(b *Box, available style.Unit) style.Unit {
	got := l.contentWidths(b)
	return style.Min(style.Max(got.min, available), got.max)
}

// outerWidths returns a box's intrinsic widths including its own margins,
// border and padding — what it takes up in a parent.
//
// The box's own §10.4 limits are applied here, and applying them is the
// difference between measuring what a box *would* like to be and measuring what
// it will actually take up. A child with "max-width: 4em" holding eight ems of
// unbreakable text is four ems wide when it is laid out, so a parent shrinking
// to fit it needs four and not eight — and a parent that asked for eight would
// be twice as wide as its own content with the rest of the page showing through.
func (l *layouter) outerWidths(b *Box, containing style.Unit) intrinsicWidths {
	inner := l.contentWidths(b)
	if declared, ok := l.intrinsicLength(b, "width"); ok {
		inner = intrinsicWidths{min: declared, max: declared}
	} else if declared, ok := l.keywordWidth(b); ok {
		// A child sized to an intrinsic keyword contributes that one number to
		// both of its parent's, exactly as a declared length does: a box at
		// "width: min-content" is that wide whether or not the parent wraps, so
		// a parent measured with the child's *own* pair would be as wide as the
		// child's longest line and would leave the page showing through.
		//
		// fit-content is deliberately not here even though it names a keyword.
		// Its contribution is the pair unchanged — min(max-content,
		// max(min-content, available)) is min-content at its narrowest and
		// max-content at its widest — which is what falling through to
		// contentWidths already gives, and keywordWidth does not claim it.
		inner = intrinsicWidths{min: declared, max: declared}
	}
	lo, hi := style.Unit(0), style.MaxUnit
	if v, ok := l.intrinsicLength(b, "min-width"); ok {
		lo = v
	}
	if v, ok := l.intrinsicLength(b, "max-width"); ok {
		hi = v
	}
	// §10.4's order — a minimum below a contradicting maximum still wins — is
	// style.Clamp's own: it applies the maximum first and the minimum last. A
	// "hi = max(hi, lo)" was written here first and then deleted, because
	// planting its removal changed nothing that could be measured: it can never
	// be the clause that decides, and a guard that decides nothing reads as
	// defence and is decoration.
	inner.min = style.Clamp(inner.min, lo, hi)
	inner.max = style.Clamp(inner.max, lo, hi)

	edges := l.edges(b, "margin", containing).Horizontal().
		Add(l.borderWidths(b).Horizontal()).
		Add(l.paddingOf(b, containing).Horizontal())
	return intrinsicWidths{
		min: maxZero(inner.min.Add(edges)),
		max: maxZero(inner.max.Add(edges)),
	}
}

// intrinsicLength reads a sizing property for use in an intrinsic measurement,
// where there is no containing block to resolve a percentage against.
//
// CSS Sizing says a percentage that cannot be resolved behaves as "auto" for
// intrinsic sizing — as *no declaration*, not as zero. Resolving one against a
// basis of nought is the plausible wrong answer and was the one here: a float
// holding a child at "width: 50%" measured that child as nothing at all, so the
// float shrank to the widest of its *other* children and the child overflowed
// it. The size is a content size, so box-sizing's inset comes off it, and that
// inset is itself resolved against nothing for the same reason.
func (l *layouter) intrinsicLength(b *Box, property string) (style.Unit, bool) {
	length, ok := l.parseLength(b, property)
	if !ok || length.Kind != style.LengthAbsolute {
		return 0, false
	}
	inset, _ := l.sizingInset(b, 0)
	return maxZero(length.Value.Sub(inset)), true
}

// contentWidths returns the two widths of a box's content box, memoized.
func (l *layouter) contentWidths(b *Box) intrinsicWidths {
	if got, ok := l.intrinsic[b]; ok {
		return got
	}
	// Recorded before the recursion rather than after. The box tree is a tree
	// and cannot cycle, but a memo written only on the way out would recompute
	// a shared subtree once per path to it, and the whole point of measuring
	// this way rather than by trial layout is that it is cheap.
	l.intrinsic[b] = intrinsicWidths{}
	got := l.measureWidths(b)
	l.intrinsic[b] = got
	return got
}

func (l *layouter) measureWidths(b *Box) intrinsicWidths {
	if b.Replaced != nil {
		// A replaced element has no preference between a narrow box and a wide
		// one: it is exactly one width and cannot be broken, so both numbers
		// are that width. This is what stops a floated image shrinking to
		// nothing — without it, contentWidths would walk a box with no children
		// and answer zero, and the float would be a sliver with the picture
		// hanging out of it.
		w := l.replacedIntrinsicWidth(b)
		return intrinsicWidths{min: w, max: w}
	}
	if b.IsText() {
		return l.textWidths(b)
	}
	// A table's two widths come from its grid rather than from stacking its
	// children, and §17.4's wrapper is as wide as the table inside it. Both are
	// next door in tablelayout.go, which is where the grid is.
	if b.Inner == InnerTable {
		return l.tableContentWidths(b)
	}
	if b.TableWrapper {
		return l.tableWrapperWidths(b)
	}
	if hasInlineChild(b) {
		return l.inlineWidths(b)
	}

	// Block children stack, so each one's demand is independent of the others'
	// and the box needs the widest.
	var out intrinsicWidths
	for _, c := range b.Children {
		if c.Outer != OuterBlock {
			continue
		}
		// A float among block children is measured too. It is out of flow, but
		// it is still inside this box, and a box narrower than the float it
		// contains would push the float out of itself.
		got := l.outerWidths(c, 0)
		out.min = style.Max(out.min, got.min)
		out.max = style.Max(out.max, got.max)
	}
	return out
}

// inlineWidths measures a run of inline content.
func (l *layouter) inlineWidths(b *Box) intrinsicWidths {
	// The frame is empty because §9.4.3's offset is applied after layout and so
	// changes no width: a relatively positioned inline demands exactly the room
	// it would have demanded without the declaration. It is marked as a
	// measurement so that nothing on the way down is laid out — see
	// inlineFrame.measuring for why that is a correctness rule and not a saving.
	items, _ := l.collectInline(b, l.markerItems(b), startOfContext(), inlineFrame{Measuring: true})
	// The same context the lines will be filled with, so that a box sized to its
	// content is sized to the content it will hold: a joined Arabic word is not
	// as wide as the same letters set apart.
	items = l.linkShapingContext(items)
	// And the same hanging punctuation, for the same reason and more plainly: a
	// character that hangs outside the line is not part of what the line has to
	// hold, so a box shrink-wrapped around it would otherwise reserve room for a
	// bracket that will be drawn in its margin. Every one of the suite's
	// fixtures for the property floats its boxes, which is what makes this the
	// half that decides the page rather than a refinement of it.
	hp, _ := hangingPunctuationOf(b.Style["hanging-punctuation"])
	items = l.hangPunctuation(items, hp)
	items = l.linkLetterSpacing(items)
	// §8.1's ideograph spacing, after the letter-spacing boundary rule and for
	// the same reason it is a pass over the finished items: both are gaps
	// *between* two runs, and neither can be decided while one of them is still
	// being built. They add to the same width and are independent — a document
	// that sets letter-spacing across an ideograph boundary gets both.
	items = l.insertAutospace(items)
	got, split := l.widthsOf(items)

	// §16.1's indent moves the first line and no other, so the box is as wide as
	// the greater of its indented first line and its widest later one. Those are
	// different lines, which is why neither "the widest line" nor "the widest
	// line plus the indent" is the answer and why the measurement above is split
	// in two.
	//
	// A single-line box cannot tell the three apart, which is how the old
	// arithmetic — the indent added to the widest line — survived so long. A box
	// with a forced break in it can: "text-indent: 3em" on a two-line div came
	// out three ems wider than the wider of its lines needed.
	//
	// The minimum is the half that matters, and it was not measured at all. A
	// float in a container narrower than its content is sized to its min-content
	// width; with the indent left out of that, a float holding an indented
	// paragraph was sized to the paragraph alone, and its first line started
	// three ems in and ran three ems past the border.
	//
	// A negative indent — a hanging one — belongs here for the same reason and
	// with the opposite sign: it *narrows* the first line, and a box sized as
	// though it had not is wider than the content asks for by the whole hang. A
	// line hung further out than it is long asks for nothing, and needs no clamp
	// to say so: the other half of each maximum is a width, widths are not
	// negative, and a negative candidate therefore loses to it.
	//
	// The percentage form contributes nothing, because there is no containing
	// block to take a percentage of while an intrinsic width is being measured.
	// CSS Sizing says such a percentage behaves as auto here, which is what a
	// basis of zero produces.
	// Which half the indent is on is §7.1's two modifiers. "hanging" puts it on
	// every line but the first, which is the other half of the same split and
	// exactly as exact. "each-line" puts it on the first line of every forced
	// segment, and the split does not model those: it knows the first line and
	// the rest, and the rest holds soft-wrapped lines as well. So both halves
	// take it, which over-states the width of a box whose widest line is a
	// soft-wrapped one. That is the safe direction — a box measured too wide
	// leaves room nothing uses, a box measured too narrow has its indented line
	// running out of it — and it is the same approximation the property's own
	// definition invites, since a soft wrap in one width is a forced segment
	// start in another.
	if indent, mode := l.textIndent(b, 0); indent != 0 {
		first, rest := split.first, split.rest
		// A half with no width is a box that has no such line — the "rest" of a
		// single-line box, above all. Indenting it would ask for room for a line
		// that does not exist, and with a wide indent that room is what the box
		// would be sized to.
		indentRest := func() {
			if rest.min > 0 {
				rest.min = rest.min.Add(indent)
			}
			if rest.max > 0 {
				rest.max = rest.max.Add(indent)
			}
		}
		switch {
		case mode.eachLine:
			first.min, first.max = first.min.Add(indent), first.max.Add(indent)
			indentRest()
		case mode.hanging:
			indentRest()
		default:
			first.min, first.max = first.min.Add(indent), first.max.Add(indent)
		}
		got.min = style.Max(rest.min, first.min)
		got.max = style.Max(rest.max, first.max)
		// A hang can take the preferred width below the minimum, and the two mean
		// something contradictory in that order: shrink-to-fit is
		// min(max(minimum, available), preferred), so a preferred width below the
		// minimum is a ceiling under a floor and the floor loses.
		//
		// It happens where the first line is the *only* line and yet not the
		// widest run — a box broken by an opportunity rather than by a forced
		// break, whose preferred width is one long line and whose minimum is its
		// widest piece. Hang that single line by more than its first piece is
		// wide and the preferred width falls under the minimum, which came out as
		// a float sized to the hang rather than to the piece that still has to
		// fit in it.
		got.max = style.Max(got.max, got.min)
	}
	return got
}

// textWidths measures one text box.
//
// It goes through the same items line breaking would, rather than reading the
// pieces itself, so that the two cannot disagree about what a tab is worth or
// about which space survives — a disagreement that shows up as a float sized to
// a width the text it holds does not need.
func (l *layouter) textWidths(b *Box) intrinsicWidths {
	// No bidi builder: an intrinsic width is a sum over the items and over the
	// widest of them, and neither depends on the order they are set in.
	items, _ := l.itemsFor(b, startOfContext(), inlineFrame{})
	got, _ := l.widthsOf(items)
	return got
}

// widthsOf is the pair over a flattened run of inline items.
//
// The two numbers differ in exactly one way: the maximum lets everything sit on
// one line and so adds the pieces up, while the minimum breaks at every
// opportunity and so takes the widest unbreakable run. A forced break — a <br>,
// or a newline in preserved white space — ends a line in both.
// The second result splits the same measurement in two: what the first line and
// the first unbreakable run came to, and what the widest of the *rest* came to.
// Only one caller wants it and only for one reason: §16.1's indent moves the
// first line and no other, so the box is as wide as the greater of the indented
// first line and everything after it. See inlineWidths.
func (l *layouter) widthsOf(items []inlineItem) (out intrinsicWidths, split lineSplit) {
	firstRun, firstLine := false, false
	// edge is the trailing run of collapsible space, which §4.1.2 removes at the
	// end of a line: a box sized to include it would be wider than the text it
	// holds by however many spaces happened to end each line.
	//
	// A *conditionally* hanging trailing space is deliberately not subtracted
	// from the maximum, because no line here is wrapped. Every line these widths
	// are measured over ends at a forced break or at the end of the content, and
	// §4.1.2 makes pre-wrap's hang at those two conditional: the space takes room
	// unless taking it would overflow. A box at its own preferred width cannot
	// overflow, so the space takes room — which is what
	// white-space-intrinsic-size-004 says in as many words.
	//
	// An *unconditionally* hanging one is subtracted, because it never takes
	// room at all. That is the answer §4.1.2 gives for normal, nowrap and
	// pre-line, and what reaches the end of a line under those three is the
	// other space separators and the preserved tabs — the spaces themselves
	// having been removed by the rule before it. A box sized to its content was
	// a stemline or an ideographic space wider than the text it holds.
	//
	// The minimum is the other way round for the conditional case: a box at its
	// min-content width is precisely the one that cannot spare the room, so the
	// space hangs and is not measured. Under pre nothing hangs at all, so
	// nothing is subtracted from either — white-space-intrinsic-size-013 and
	// -015 are the two halves of that, and they disagree on purpose.
	//
	// runEdge is the same measurement over the *unbreakable* run, and it is not
	// the same number. Where the text wraps the two agree trivially, because a
	// space that a line may end at ends the run as well and there is nothing left
	// in it to trim. Where the text may not wrap — "white-space: nowrap", or a
	// space that offers no opportunity at all — the space joins the run, and a
	// minimum measured without this is wider than the text by the trailing space
	// of every line. That is not hypothetical: "width: min-content" on a nowrap
	// box ending in an ogham space mark, which §4.1.2 removes by name, came out
	// one stemline wider than the same text without it.
	// runContent says the run being built holds something. It is asked only by
	// the first-run measurement, and it is asked because a break opportunity at
	// the very start of the content ends a run that never began — an atomic
	// inline offers one before itself, so "<div><span/></div>" ends an empty run
	// before its only box. Taking that as the first run made the first line hold
	// nothing, and an indent added to nothing is the indent alone.
	var runContent bool
	var line, run, edge, runEdge style.Unit
	// The letter-spacing after the last character, which §8.2 adds and which
	// hangs past the end of a line rather than counting towards it. A box
	// shrink-wrapped around its text was one spacing wider than the text: "abc"
	// at 12px a character with 12px of spacing is 60 wide and came out 72, with
	// the extra drawn past the right edge of the box that was sized for it.
	//
	// It is the same subtraction the line layout makes with trailingSpacing, and
	// it is kept beside edge because the two answer the same shape of question:
	// what is at the end of the line that the line is not as wide as. Where a
	// trailing space is trimmed the spacing after it goes with it, so the tail
	// is left where it was rather than moved to the space — the last character
	// that counts is the one in front.
	var lineTail, runTail style.Unit
	// floats is how much of the line's width is taken by the floats standing
	// beside it.
	//
	// At the maximum width nothing wraps, so the whole of the inline content is
	// one line and every float in it is beside that line rather than above or
	// below it: "<div float:left><div float:right width:100px></div><span
	// inline-block width:100px></span></div>" shrinks to 200 and not to 100,
	// which is what intrinsic-size-float-and-line asserts by painting the
	// container red and covering it exactly.
	//
	// The *minimum* is the other question and is answered where it always was.
	// There the text wraps under the float, so the two stand one above the
	// other and the widest of them is what the box must hold.
	//
	// A float that clears begins a new row of them, and what was beside the
	// last row is not beside this one — letter-spacing-206 is eight floated
	// paragraphs, every one of them "clear: left", and summing them makes the
	// box that holds them eight paragraphs wide.
	var floats style.Unit
	// The spacing an item leaves behind it is read straight off the item. The
	// line's own version of this — trailingSpacing — asks isSpacedRun first,
	// and here that predicate has nothing to do: every item that reaches one of
	// the assignments below is text or a space, an inline box's own edge, which
	// is skipped where it is met, or an atomic inline, which is set to nothing
	// where it is met. Planting the predicate in changed no width in any fixture
	// including a tab, whose spacing is carried in its advance rather than on
	// the item, so it is left out rather than written as a guard that cannot
	// fail.
	endRun := func() {
		w := run.Sub(runEdge).Sub(runTail)
		switch {
		case runContent && !firstRun:
			split.first.min, firstRun = w, true
		default:
			split.rest.min = style.Max(split.rest.min, w)
		}
		out.min = style.Max(out.min, w)
		run, runEdge, runContent, runTail = 0, 0, false, 0
	}
	endLine := func() {
		endRun()
		w := line.Sub(edge).Sub(lineTail).Add(floats)
		if !firstLine {
			split.first.max, firstLine = w, true
		} else {
			split.rest.max = style.Max(split.rest.max, w)
		}
		out.max = style.Max(out.max, w)
		line, edge, lineTail = 0, 0, 0
		// floats is not reset with them, and that is the point of keeping it
		// apart from the line at all. A forced break ends a line and does not
		// clear anything: two right floats written either side of a <br> stand
		// beside each other, so the box has to be wide enough for both of them
		// and for the widest line beside them. Only "clear" ends a row of
		// floats, and it does it where it is met.
	}

	for k, item := range items {
		// Whether a line may end after this item. It is the next item's own
		// answer to whether it may begin one, which is the same question read
		// from the other side, and it is needed for the space case below: not
		// every space offers an opportunity. U+2007 FIGURE SPACE holds a column
		// of digits together and U+202F NARROW NO-BREAK SPACE holds a number to
		// its unit, and a minimum width that broke at either would be the width
		// of one digit.
		breaks := k+1 >= len(items) || items[k+1].BreakBefore
		switch {
		case item.HangStart || item.HangEnd:
			// §8.4's hanging punctuation. It is drawn outside the line, so it is
			// not part of what a line has to hold and not part of what a box
			// shrink-wrapped around one has to be wide enough for. Skipping it
			// outright is the whole of that: the character is its own item, so
			// there is no run to take it out of and nothing else to adjust.
			//
			// Every one of the suite's fixtures for the property floats its
			// boxes, so this is the half that decides the page rather than a
			// refinement of it — a float sized as though the bracket were inside
			// the line is a box a character too wide with the bracket drawn in
			// its margin anyway.
			continue
		case item.Float != nil:
			// A float beside text is as wide as it is whether or not the text
			// wraps, so it raises both numbers on its own rather than joining
			// the run of words.
			got := l.outerWidths(heldBox(item.Float), 0)
			out.min = style.Max(out.min, got.min)
			out.max = style.Max(out.max, got.max)
			if f := heldBox(item.Float); f != nil && f.Clear != ClearNone {
				// The row of floats this one goes below is finished, and the
				// width that row needed has to be kept now: nothing after the
				// clear will ask for it again, because this float is not beside
				// it. What it needed is the floats in it and the content that
				// was standing beside them.
				//
				// The content is counted again in the row below, which is the
				// approximation this makes and the safe direction to make it
				// in: at the maximum width there is one line and it is beside
				// every row, so a box measured this way is wide enough for the
				// row it built and never narrower than one.
				out.max = style.Max(out.max, line.Sub(edge).Sub(lineTail).Add(floats))
				floats = 0
			}
			floats = floats.Add(got.max)
			// Out of flow, so §16.1's indent does not move it and it is not part
			// of what the indent is added to. See lineSplit.
			split.rest.min = style.Max(split.rest.min, got.min)
			split.rest.max = style.Max(split.rest.max, got.max)

		case item.Abs != nil:
			// Out of flow: it takes no width on the line, so it contributes to
			// neither number.

		case item.AtomicBox != nil:
			// An atomic inline cannot be broken, so it joins the unbreakable
			// run rather than ending it — "an <img> in a sentence" is one word,
			// a picture and another word, with the picture as unsplittable as
			// either. It contributes its own two widths, which for a replaced
			// element are the same number and for an inline-block are its
			// content's.
			if item.BreakBefore {
				endRun()
			}
			got := l.outerWidths(heldBox(item.AtomicBox), 0)
			run, runContent = run.Add(got.min), true
			line = line.Add(got.max)
			// Content, so a space before it is no longer trailing. Without this
			// a picture after a space would be measured into a box short by the
			// space's width — the same slip the text case below avoids.
			edge, runEdge = 0, 0
			// A picture is not a character and no spacing follows it.
			lineTail, runTail = 0, 0

		case item.Forced:
			endLine()

		case item.Space:
			w := item.Width
			if item.Tab {
				// Tab stops are measured from the block's content edge, and on
				// an unbroken line that edge is where this measurement started.
				// The letter-spacing after the tab is added the same way line
				// breaking adds it, so the two cannot disagree about how wide a
				// tab-separated line is.
				w = tabAdvance(line, item.TabStop, item.TabFloor)
			}
			if item.NoWrap || !breaks {
				// Text that may not break has one width, not two. A space in it
				// is a space and not an opportunity, and an engine that ended
				// the unbreakable run here would give a nowrap paragraph a
				// minimum width of its longest word — so a float holding one
				// would be sized to a fraction of the text it then overflows.
				run, runContent = run.Add(w), true
				if item.TrimAtEnd || item.Hangs {
					runEdge = runEdge.Add(w)
				} else {
					runEdge, runTail = 0, trailingSpacingOf(item)
				}
			} else {
				// The run ends at the space — but on which side of it depends on
				// whether the space is still there when the line ends.
				//
				// Under normal and pre-wrap it is not: §4.1.2 removes a
				// collapsible one and hangs a preserved one, so the run before it
				// is what has to fit and the space costs the minimum nothing.
				//
				// Under break-spaces it is. That value's whole point is that "a
				// sequence of preserved white space always takes up space,
				// including at the end of the line", and the opportunity it
				// offers is *after* each space rather than before — so the space
				// belongs to the run in front of it and the narrowest that run
				// can be is one space wider. "123    8" in four characters wraps
				// to "123 " and "   8", and a minimum of three said the four
				// would not fit.
				if !item.TrimAtEnd && !item.Hangs {
					run, runContent = run.Add(w), true
				}
				endRun()
			}
			line = line.Add(w)
			if item.TrimAtEnd || item.HangsHard {
				edge = edge.Add(w)
			} else {
				edge, lineTail = 0, trailingSpacingOf(item)
			}

		case item.Anywhere && !item.NoWrap:
			// overflow-wrap: anywhere. §5.5 gives this value opportunities that
			// *are* counted here, which is the only thing that distinguishes it
			// from break-word: a shrink-to-fit box holding one long word narrows
			// to the widest character in it rather than to the whole word.
			//
			// It ends the unbreakable run on both sides and contributes its own
			// widest cluster, because a run of text that may break between any
			// two characters is not part of anything unbreakable.
			endRun()
			// Counted with the rest rather than with the first line, which is
			// the conservative reading in both directions: a positive indent
			// does not widen it and a negative one does not narrow it. See
			// lineSplit.
			got := l.widestCluster(item)
			out.min = style.Max(out.min, got)
			split.rest.min = style.Max(split.rest.min, got)
			line = line.Add(item.Width)
			edge, runEdge = 0, 0
			lineTail, runTail = trailingSpacingOf(item), trailingSpacingOf(item)

		default:
			if item.BreakBefore && !item.NoWrap {
				endRun()
			}
			run, runContent = run.Add(item.Width), true
			line = line.Add(item.Width)
			edge, runEdge = 0, 0
			if !item.Inset {
				// An inline box's own edge is not a character, so it neither
				// carries spacing nor clears the spacing of the text before it.
				lineTail, runTail = trailingSpacingOf(item), trailingSpacingOf(item)
			}
		}
	}
	endLine()
	return out, split
}

// lineSplit is a content's intrinsic widths with its first line held apart from
// the rest.
//
// The two are needed separately because §16.1 moves one line and not the others,
// so neither "the widest line" nor "the widest line plus the indent" is the
// answer: a box is as wide as the greater of its indented first line and its
// widest later one, and those are different lines.
type lineSplit struct {
	first, rest intrinsicWidths
}

// widestCluster is an item's min-content contribution under overflow-wrap:
// anywhere — the widest grapheme cluster in it.
//
// The cluster and not the character, for the reason every other cut in this
// engine is at a cluster: a line may not end inside one, so a box narrower than
// the widest cluster could not hold the text however hard it broke it.
//
// Each cluster is measured on its own rather than summed from a running total,
// which is exactly the approximation this refuses elsewhere — here it is not an
// approximation, because a single cluster's width is what it is. What is lost is
// kerning between two clusters, and there is none: they are never adjacent on a
// line this width.
func (l *layouter) widestCluster(item inlineItem) style.Unit {
	if item.Face == nil || item.Text == "" {
		return item.Width
	}
	var widest style.Unit
	prev := 0
	for _, at := range append(segment.Boundaries(nil, item.Text), len(item.Text)) {
		w := l.br.MeasureSpaced(item.Face, item.Text[prev:at], item.Size, item.Spacing)
		widest = style.Max(widest, w)
		prev = at
	}
	return widest
}
