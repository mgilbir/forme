package paragraph

import (
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// Where a line ends.
//
// CSS Text §5 over a sequence of items: fill the line, and where the next thing
// will not fit, go back to the last opportunity that did. What a line may not do
// is stop early, and the two rules that make it not stop early — the hanging
// white space of §4.1.2 and the last-resort cut of §5.5 — are most of what is
// below the greedy loop.

// MaxLineFits bounds how many times one line box may be broken again because
// the band it was broken against turned out to be the wrong one.
//
// Each round is a strictly narrower band or a strictly lower line, so the
// sequence cannot return to a state it has left — but a narrower band can make
// a line taller (a word wraps onto it) as easily as shorter, and a taller line
// meets a different set of floats, so there is no argument that it settles at
// all. Two extra attempts is what the suite's deepest case needs; past that the
// line keeps the break it has, which is a line that is slightly wrong rather
// than a render that does not finish.
//
// A variable so that a test can lower it and watch the bound decide, on the
// model of maxRelayouts.
var MaxLineFits = 2

// BlockEllipsis is what a clamped block puts at the end of its last line.
//
// CSS Overflow 4's "block-ellipsis: auto" is "a UA-defined value", and the
// horizontal ellipsis is the one every engine uses and the one the suite's
// references write.
const BlockEllipsis = "\u2026"

// BreakOneLine fills a single line, greedily, and says where the next one
// starts.
//
// Greedy is what browsers do: a line takes what fits and the next one starts
// after it. The alternative — minimising raggedness across a paragraph, which is
// what TeX does — produces better-looking text and needs the whole paragraph
// before any line is settled, which is a different shape of engine.
//
// One line at a time rather than the whole paragraph, because width is no longer
// a property of the paragraph: with a float beside it, each line has its own.
//
// forced reports that the line ended at a break the author wrote, which is what
// makes an empty line real — "a<br><br>b" leaves a blank line, and an engine
// that dropped empty lines would close the gap up.
//
// lineX is where the line box starts within the block's content box, which a
// float beside it makes something other than zero. It is needed because a tab
// stop is measured from the block's edge and not from the line's.
//
// The returned items carry their resolved widths: a tab's is not known until it
// has a place, so an item on a line is not always the item that came in.
func (br *Breaker) BreakOneLine(items []Item, from, fromByte int, width, lineX style.Unit) (
	line []Item, next, nextByte int, outOfFlow []MidLineBox, forced bool) {

	line, next, nextByte, outOfFlow, forced = br.fillOneLine(items, from, fromByte, width, lineX)
	return withHyphen(items, line, from, next, nextByte, forced), next, nextByte, outOfFlow, forced
}

// withHyphen prints the hyphen a soft hyphen asked for, on the line that broke
// there.
//
// It is here rather than inside the fill because the fill ends a line from nine
// places and every one of them would need it. What decides is where the *next*
// line starts: the item before it is the one that offered the opportunity, and
// it carries the hyphen only if it was the soft hyphen that offered it.
//
// Reading the item before the break rather than the last item on the line is the
// difference between "a<shy>b c" hyphenating and not. In "abc<shy> def" the line
// may end at the space, and trimming that space off leaves the soft-hyphen item
// last on the line — so a version that looked at the line would print a hyphen
// at the end of a word that was not broken.
//
// The item is built rather than copied so that a field added to Item later is
// absent from it by default, which for a synthetic item is the safe answer: it
// carries no break opportunity, is no kind of white space, and is not out of
// flow. What it does carry is everything about how the text beside it is drawn,
// because it is drawn as part of that text.
func withHyphen(items, line []Item, from, next, nextByte int, forced bool) []Item {
	// A forced break is the author's own and hyphenates nothing; the end of the
	// items is the end of the paragraph, where the word was not broken at all;
	// a non-zero offset means the line ended *inside* an item, which is
	// overflow-wrap's cut and not a hyphenation point.
	if forced || nextByte != 0 || next <= from || next >= len(items) {
		return line
	}
	at, ok := hyphenBefore(items, next)
	if !ok {
		return line
	}
	// Capped so the append cannot write into the caller's items.
	return append(line[:len(line):len(line)], Item{
		Text: at.HyphenText, Width: at.Hyphen,
		Box: at.Box, Face: at.Face, Size: at.Size,
		Above: at.Above, Below: at.Below, Valign: at.Valign,
		Decorations: at.Decorations, Spacing: at.Spacing,
		Offset: at.Offset, Leads: at.Leads,
	})
}

// hyphenBefore is the item whose soft hyphen the line ended at, if it did.
//
// The scan looks past an inline box's own inset and past the record of a box
// that is out of flow, which are the two things that can be written between a
// soft hyphen and the word after it without being anything a reader sees. The
// suite asks for both by name: hyphens-span-001 puts a </span> there and
// hyphens-out-of-flow-001 an absolutely positioned box, and both want the same
// hyphen in the same place as the plain word.
func hyphenBefore(items []Item, next int) (Item, bool) {
	for k := next - 1; k >= 0; k-- {
		it := items[k]
		if it.Inset || it.Abs != nil || it.Float != nil {
			continue
		}
		return it, it.Hyphen != 0 && it.HyphenText != ""
	}
	return Item{}, false
}

// fillOneLine is BreakOneLine's greedy fill, before the hyphen is printed.
func (br *Breaker) fillOneLine(items []Item, from, fromByte int, width, lineX style.Unit) (
	line []Item, next, nextByte int, outOfFlow []MidLineBox, forced bool) {

	var used style.Unit
	// content says the line holds something a reader would see. It is not the
	// same as the line being non-empty: an inline box's own margin, border and
	// padding take room without being content, and §4.1.2's rule about the
	// *beginning of a line* is about the text. "<span style='margin-left: 5px'>
	// x</span>" sets one space in from five pixels, not from nine.
	content := false
	// A break opportunity that fell on an inline box's leading edge is a break
	// before the box, and the box's margin travels to the next line with the
	// word it pushes along. That decision cannot be taken when the margin is
	// reached: the margin is an item of its own, the word after it is not an
	// opportunity of its own, and it is the *pair* that has to fit. So where the
	// opportunity was is remembered and the line rewinds to it if the word turns
	// out not to fit.
	//
	// Rewinding rather than looking ahead is what keeps this linear: each item is
	// still visited once, and the position is a single index rather than a scan.
	insetAt, insetLine, insetFlow := -1, 0, 0
	// Where §8.4's allow-end has hung a character on this line, and -1 when
	// nothing is hanging.
	//
	// It is a position rather than a flag because the hang is provisional. The
	// value hangs a stop or comma "at the end of a line", and whether the
	// character is at the end of the line is not known when it is reached — only
	// when something else lands after it. So it is hung, the fill goes on, and
	// the hang is taken back at the top of the next turn if what follows takes
	// room. See the restore below.
	hungAt := -1
	// The most recent point at which this line could have ended, kept for
	// break-spaces. §3's break-spaces value puts the soft wrap opportunity
	// *after* every preserved space and nowhere else, so a space belongs to the
	// unit that precedes it: "X XX X" in four characters is "X " and "XX X",
	// because the run "XX " — word and the space that follows it, with no
	// opportunity between them — is three characters wide and does not fit after
	// the first two. Greedy filling has to measure that whole run and not just
	// its first item, and this marker is how: the space is placed if it fits, and
	// sends the line back to the last opportunity if it does not.
	//
	// It is a marker rather than a lookahead for the reason insetAt is: each item
	// is visited once on the way forward and the rewind is a single index, so a
	// paragraph costs one pass however many times a line has to be given back.
	// Progress is guaranteed because the marker is only set once the line holds
	// content, so it is always past the item the line started at.
	oppAt, oppLine, oppFlow := -1, 0, 0
	// Where the white space that ends this line begins.
	//
	// §4.1.2's third and fourth rules are both about white space "at the end of a
	// line": the collapsible part of it is removed and what remains hangs. Neither
	// can happen to white space the line breaks *inside*, and breaking inside it
	// is what a greedy fill does on its own — the run is wider than the room left,
	// so the first opportunity in it ends the line and the rest goes to the next
	// one. What that produces is a line of nothing but spaces, which is precisely
	// the thing the two rules exist to prevent.
	//
	// So the run is found before the fill starts and the fill is told not to break
	// in it. It reaches from the last item that is not white space to the end of
	// the line's material — the next forced break, or the last item there is — and
	// an inline box's own inset is passed over rather than ending it, since a
	// margin is not content and a span wrapped around the spaces must not make
	// them breakable again.
	end := len(items)
	for k := from; k < len(items); k++ {
		if items[k].Forced {
			end = k
			break
		}
	}
	tailFrom := end
	for tailFrom > from && isLineTailSpace(items[tailFrom-1]) {
		tailFrom--
	}
	i := from
	for ; i < len(items); i++ {
		item := items[i]
		if i == from && fromByte > 0 {
			// The line begins part-way through an item, because the line before
			// it ended inside this word. The cursor is an index *and* an offset
			// rather than a rewritten items slice: the caller re-runs this over
			// several band widths, so anything written back would be seen by the
			// next attempt and the split would compound.
			_, item = br.SplitItem(item, fromByte)
		}

		// §8.4's hang, taken back.
		//
		// A character hung by allow-end is at the end of the line only until
		// something lands after it, and what lands after it is not known when it
		// is hung. So the hang is undone before the branches below decide
		// anything: the character goes back into the line's measure, and the
		// item that follows meets the width it really has.
		//
		// What does *not* take it back is everything that takes no room — a
		// forced break, an inline box's own edge, a box out of flow. Those
		// legitimately follow a character at the end of a line, and "ab c、<br>"
		// is the suite's row for it.
		//
		// Without this, a comma inside a "white-space: nowrap" span hung and
		// pulled the span apart: the atomic inline after it began a new line,
		// which is the one thing the span was written to prevent. The suite's
		// hanging-punctuation-allow-end writes that row too, and its assert says
		// so in as many words — punctuation does not hang "when a nowrap span
		// prevents breaking before the punctuation".
		if hungAt >= 0 && !item.Forced && !item.Inset && item.Abs == nil && item.Float == nil {
			line[hungAt].Hangs, line[hungAt].HangEnd = false, false
			used = used.Add(line[hungAt].Width)
			hungAt = -1
		}

		if item.Float != nil {
			// Recorded with how far along the line it was reached, which is what
			// decides whether it goes beside this line or below it.
			outOfFlow = append(outOfFlow, MidLineBox{
				Box: item.Float, Used: used, Offset: item.Offset})
			continue
		}

		if item.Abs != nil {
			// Recorded and otherwise ignored. It consumes no width, so the words
			// on the line are placed exactly as they would have been had the box
			// not been written at all — which is what "out of flow" means and is
			// the assertion a test can make that a float cannot.
			outOfFlow = append(outOfFlow, MidLineBox{
				Box: item.Abs, Used: used, Abs: true, Offset: item.Offset,
				AfterContent: contentOnLine(line)})
			continue
		}

		if item.Forced {
			// An instruction rather than an opportunity: the line ends here
			// whatever room is left, and an empty one still occupies its height.
			return trimLineEdge(line), i + 1, 0, outOfFlow, true
		}

		// §4.1.2's first rule: a sequence of collapsible spaces at the
		// beginning of a line is removed. It is the space the break happened
		// at, or the one the author left after a tag, and keeping it would
		// indent every line after the first.
		//
		// Only a *collapsible* one. Preserved white space at the start of a
		// line is content — it is what makes "<pre>    indented</pre>" indent,
		// and it is the whole of the pre-wrap leading-space rule.
		//
		// The test is on the line's *content* rather than on the line being
		// empty, because an inline box's margin is not content — but no
		// document in the suite can tell the two apart, and that is worth
		// recording rather than leaving as an implied claim. Reaching the
		// difference needs a collapsible space that survives collection sitting
		// immediately after an inline box's leading margin, on a line that
		// begins there. The two requirements contradict each other: a line
		// begins at a margin only when a space preceded the box, and a space
		// before the box is exactly what makes §4.1.1 collapse away the one
		// after its opening tag. Planting "len(line) == 0" here moves nothing —
		// 2973 clean passes either way — so this branch is the correct reading
		// of the rule and has no test, which is a different thing from being
		// covered.
		if item.Collapsible && !content {
			continue
		}

		if item.Tab {
			// The distance to the next tab stop, plus whatever letter-spacing adds
			// after the character — a tab is a character like any other for that
			// purpose, and leaving it out would put the run after a tab a spacing
			// to the left of where it is drawn.
			item.Width = TabAdvance(lineX.Add(used), item.TabStop, item.TabFloor)
		}

		// §8.4's allow-end, which is decided here because it is a question about
		// a line: "a stop or comma at the end of a line hangs ... only if it
		// does not otherwise fit prior to justification".
		//
		// So it is asked exactly where the answer is known — the character does
		// not fit, the line holds something already, and putting it outside the
		// line is what lets the line have it. A candidate that fits needs
		// nothing: it is placed like any other character and the value has
		// nothing to say about it. That is the whole of the difference from
		// force-end, which hangs one whether or not it fits.
		//
		// used is not advanced, because the character is outside the line —
		// which is what hanging it means, and is what lets the line hold it.
		//
		// The fill goes on rather than ending here, and that is not a nicety.
		// Whatever follows meets exactly the room the character found, so a
		// letter still overflows and still breaks the line in the same place;
		// what is different is everything that takes *no* room. A forced break
		// after the comma has to end this line rather than begin an empty one
		// below it, and the suite writes that row: "ab c、<br>", whose reference
		// is one line and not two.
		// One character per line falls out of the restore above rather than
		// needing a rule: a stop or a comma takes room, so the restore fires
		// before a second candidate is ever tested and the first is back in the
		// line's measure by then. A guard here was written first and could not
		// be reached — not by two commas, and not by two with a float between
		// them, which is the only thing that stands between two characters and
		// takes no room.
		if item.MayHangEnd && content && !item.NoWrap && overflows(used, item, width) {
			item.Hangs, item.HangEnd = true, true
			hungAt = len(line)
			line = append(line, item)
			continue
		}

		// A hanging space never causes a break: it sits past the line's end
		// rather than moving to the next one. Without this, "XX    XX" under
		// pre-wrap would push the second word down a line for spaces that take
		// no room on the page at all.
		if !item.NoWrap && !item.Hangs && !isTailSpace(item) && i < tailFrom && item.BreakBefore &&
			len(line) > 0 && overflows(used, item, width) {
			// Ending here costs the hyphen as well, where the opportunity is one
			// a soft hyphen offered. If that does not fit, this is not a place
			// the line may end at all and it goes back to one that is — the
			// hyphen is not optional, so a line that cannot hold it has not
			// broken here.
			if h := pendingHyphen(line); h == 0 || used.Add(h) <= width {
				return trimLineEdge(line), i, 0, outOfFlow, false
			} else if oppAt >= 0 {
				return trimLineEdge(line[:oppLine]), oppAt, 0, outOfFlow[:oppFlow], false
			}
			return trimLineEdge(line), i, 0, outOfFlow, false
		}

		// The rewind. The item does not begin a break opportunity of its own,
		// but an inline box opened just before it did, and the pair is what does
		// not fit — so the line ends where the box began and the box's leading
		// margin goes with it.
		if !item.NoWrap && !item.Hangs && i < tailFrom && !item.BreakBefore && !item.Inset &&
			insetAt >= 0 && overflows(used, item, width) {
			return trimLineEdge(line[:insetLine]), insetAt, 0, outOfFlow[:insetFlow], false
		}

		// The rewind to the last opportunity. Something that does not fit and
		// cannot begin a line of its own is the tail of the unit before it, so a
		// line that cannot hold it cannot hold that unit either and ends at the
		// last opportunity instead. Where there is no such opportunity it stays
		// and the line overflows.
		//
		// It was written for break-spaces, whose preserved space is exactly that
		// — data, never dropped to make a line fit — and it was restricted to
		// spaces, which was too narrow. "xy <span>ab</span>cdefgh" in seventy-two
		// pixels put everything on one line and let it overflow: "cdefgh" begins
		// no opportunity, because there is none between a span and the text after
		// it, so nothing sent the line back to the space it had.
		//
		// Atomic inlines are still excluded, and that is measured rather than
		// reasoned. Letting an inline-block or an image rewind costs thirty-two
		// reftests, so the suite says the behaviour they have is the right one
		// and this is not the change that should alter it; extending the rule to
		// text alone moves nothing on the suite either way and fixes the case
		// above, which makes it a strict improvement rather than a trade.
		if (item.Space || item.AtomicBox == nil) && !item.Collapsible &&
			!item.Hangs && i < tailFrom && !item.NoWrap && !item.Inset &&
			!item.BreakBefore && oppAt >= 0 && overflows(used, item, width) {
			return trimLineEdge(line[:oppLine]), oppAt, 0, outOfFlow[:oppFlow], false
		}

		// A single item wider than the line has nowhere to go. It is placed and
		// overflows — breaking inside a word would be worse, since a word split
		// at an arbitrary point reads as a different word — and it is reported,
		// because the part past the edge is simply not drawn and nothing else
		// about the page says so.
		// overflow-wrap, CSS Text §5.5: the last resort.
		//
		// Its opportunities exist only "if there are no otherwise-acceptable
		// break points in the line", and this is the place that knows: every
		// rewind above has been tried and none applied, so ending the line here
		// is the only way not to overflow.
		//
		// The condition is about the *line* and not about one over-wide item,
		// which is the correction that made this do anything at all. A first
		// draft fired only where a single item was wider than the whole line,
		// and the suite's own fixture is not that shape: "XXXX XX" in four
		// characters cuts into pieces that each fit, and it is the run of them
		// that does not. Requiring no rewind target is what keeps it a last
		// resort — a line with a space in it breaks at the space.
		//
		// That last requirement is the correct reading of the rule and has no
		// test, which is a different thing from being covered. It cannot have
		// one: the two rewinds above return before this is reached, and the
		// branch before them takes any item that begins an opportunity of its
		// own, so what is left to arrive here holding a rewind target is an
		// atomic inline or a collapsible space — and breakInsideWord can cut
		// neither, so the branch declines and the line goes on to the same place
		// it would have reached anyway. Instrumented to count the case, no
		// document in the suite reaches it: zero hits over all 5177 reftests.
		// Dropping the conjunct therefore moves nothing, which is why it is
		// recorded here rather than left as an implied claim.
		if item.BreakWord && !item.NoWrap && !item.Hangs && i < tailFrom && !item.Inset && !item.Tab &&
			insetAt < 0 && oppAt < 0 && overflows(used, item, width) {
			// The offset is into items[i]. It is only the cursor's offset away
			// from that when this *is* the item the cursor pointed at: a line
			// that began at a float and reached its first text later is at
			// i > from, where the item is whole.
			base := 0
			if i == from {
				base = fromByte
			}
			// As much of the item as the room left will hold. This is what
			// keeps the fill greedy: a line with "ab" on it and "cdefgh" next
			// takes "cd" as well rather than stopping at the two characters it
			// already has.
			if head, at, ok := br.breakInsideWord(item, width.Sub(used), content); ok {
				line = append(line, head)
				return trimLineEdge(line), i, base + at, outOfFlow, false
			}
			// Nothing of it fits — a single character too wide for the room
			// left, or a space, which has one cluster and cannot be cut. The
			// line ends in front of it and it begins the next one, which is
			// where a preserved space under break-spaces has to go: the value
			// exists so that spaces are data, and dropping one to tidy the line
			// would lose it.
			//
			// Only where the line holds something already. Otherwise there is
			// nothing to end and the item would begin this line again for ever;
			// it is placed, it overflows, and it is reported below.
			if content {
				return trimLineEdge(line), i, base, outOfFlow, false
			}
		}

		// The other end of the same rule: the opportunity a character carries
		// is the one *after* it.
		//
		// overflow-wrap is written over the text it is set on, and a run that
		// allows the last-resort break therefore offers one at its own end —
		// where the run following it, which does not allow one, offers none at
		// its start. overflow-wrap-anywhere-inline-004 is that shape and says
		// so in its own assertion: "should break after the last character of
		// the inline-block it applies to". Written
		// "X<span style='overflow-wrap:anywhere'>XX</span>XX" in two
		// characters, it is three lines, and without this it is two — the
		// branch above cuts inside the span and then nothing lets the line end
		// at the span's edge.
		//
		// Every condition of the branch above holds here and for the same
		// reasons. There is no separate test that the line holds something:
		// breaksAfterLast asks for a run on it, which is the stronger
		// requirement and implies it.
		//
		// Three of the conjuncts cannot be reached and are recorded rather
		// than dropped, which is the same accounting the branch above keeps
		// for its own. Planted defects removing "!item.BreakWord", "insetAt <
		// 0" and "oppAt < 0" each leave every test passing and the suite at
		// the same count. The first is unreachable because the branch above
		// takes an item that allows the break itself and returns; the other
		// two because the rewinds that set them are tried before either branch
		// is reached. They stay because the two branches are one rule, and a
		// reader given one without the other's conditions would be told they
		// are two.
		if !item.BreakWord && !item.NoWrap && !item.Hangs && i < tailFrom &&
			!item.Inset && !item.Tab && insetAt < 0 && oppAt < 0 &&
			breaksAfterLast(line) && overflows(used, item, width) {
			base := 0
			if i == from {
				base = fromByte
			}
			return trimLineEdge(line), i, base, outOfFlow, false
		}

		if item.Width > width && !content && !item.Space && !item.NoWrap && !item.Inset {
			// An inset is not text and has no text to name in the report. A
			// margin wider than the line is also not the fault the report is
			// about — nothing is clipped, the content is simply pushed past the
			// edge, and the box the author wrote is the box that was drawn.
			br.report.ReportOverflow(item, width)
		}
		// Recorded before the switch below, because that is where content becomes
		// true: an opportunity at the very start of a line is not one the line can
		// be sent back to.
		if item.BreakBefore && content {
			// Not an opportunity the line can be sent back to if the hyphen it
			// would have to print does not fit in the room the line had.
			if h := pendingHyphen(line); h == 0 || used.Add(h) <= width {
				oppAt, oppLine, oppFlow = i, len(line), len(outOfFlow)
			}
		}

		switch {
		case item.Inset && item.BreakBefore && content && insetAt < 0:
			// The line could have ended here. Remember enough to come back.
			insetAt, insetLine, insetFlow = i, len(line), len(outOfFlow)
		case !item.Inset:
			// Something that is not a margin has been placed, so the break
			// before the last box is no longer the one to rewind to: there is a
			// nearer opportunity, or none, and either way this one is spent.
			insetAt = -1
			content = true
		}
		line = append(line, item)
		used = used.Add(item.Width)
	}
	return trimLineEdge(line), i, 0, outOfFlow, false
}

// overflows reports whether placing an item would put the line past its width.
//
// It is not "used plus the item's advance". §8.2's letter-spacing is added after
// the *last* character too, and that last one hangs at the end of the line — so
// the line's measure ends at the last glyph and not a tracking-width past it.
// Counting it wrapped a paragraph set with wide tracking one word early on every
// line, the wider the tracking the earlier, and it contradicted this engine's
// own statement of the rule: layout's TestLetterSpacingIsStillAddedAfterEveryLetter
// has said "it is the trailing one that hangs at the end of a line" since
// letter-spacing was implemented.
//
// The item under test is the one whose trailing spacing would be at the end if
// the line ended here, which is why the subtraction is of its spacing and not of
// the line's last. Where more text follows on the same line that spacing stops
// hanging and becomes internal, and the next item's is discounted instead; the
// invariant that holds throughout is that the measure excludes the trailing
// spacing of whatever is last.
//
// # What the suite says about this, which is nothing
//
// No reftest moved either way when this was written — not one of 6250, in
// either direction — and the three written for it, letter-spacing-200 through
// -202, could not: both documents of each pair reported a font the harness did
// not have. The harness loads a document's own @font-face now and those three
// are untainted failures, so they are reachable and this is no longer the whole
// of what they need. See forme-next-leads.
func overflows(used style.Unit, item Item, width style.Unit) bool {
	return used.Add(item.Width).Sub(trailingSpacing(item)) > width
}

// trailingSpacing is the letter-spacing after an item's last character, which is
// the part of its width the measure above leaves out.
//
// It is not simply the declared value. §8.2's cursive tracking adds none inside
// a cursive script, so such a run has none after its last character either —
// and discounting one it never had makes it a spacing narrower than it is,
// which is a word kept on a line it does not fit on.
func trailingSpacing(item Item) style.Unit { return TrailingSpacing(item) }

// TrailingSpacing is trailingSpacing for the layout package, whose intrinsic
// pass keeps the same account: what a line leaves out when it ends here.
func TrailingSpacing(item Item) style.Unit {
	// §8.1's ideograph spacing sits at the far edge of the run it was added to,
	// and is between two characters that a line break puts on different lines.
	// Two characters on different lines are not adjacent and get no gap.
	out := item.Autospace
	if CursiveTrackingSuppresses(item.Text) {
		return out
	}
	return out.Add(item.Spacing.Letter)
}

// pendingHyphen is the width of the hyphen a line would have to print if it
// ended with the items placed so far.
//
// It looks past an inline box's own inset, which is the same thing trimLineEdge
// does and for the same reason: a </span> between the soft hyphen and the end of
// the line is not content and does not put anything between them.
//
// Zero for every line that does not end at a soft hyphen, which is every line of
// almost every document — so everything this adds to the fill is inert unless
// the text really asked to be hyphenated.
func pendingHyphen(line []Item) style.Unit {
	for k := len(line) - 1; k >= 0; k-- {
		if line[k].Inset {
			continue
		}
		return line[k].Hyphen
	}
	return 0
}

// trimLineEdge is §4.1.2's third rule: a sequence of collapsible spaces at the
// end of a line is removed, "as well as any trailing U+1680 OGHAM SPACE MARK
// whose white-space property is normal, nowrap, or pre-line". Both are
// trimAtEnd, which is why the scan reads that rather than collapsible.
//
// CSS Text removes it because it is the space the break happened at: leaving it
// would make a right-aligned line hang, and a centred one sit off-centre by half
// a space.
//
// Preserved white space is *not* removed, and the difference is deliberate: it
// hangs instead, so it stays in the runs, which is what a reader copying text
// out of the page gets. Removing it would silently drop characters the author
// wrote from the document's text, which is the same class of fault as the
// missing spaces that once made "A heading" extract as "Aheading".
//
// What the hanging affects is the break decision — hangs is what stops a
// trailing space pushing the next word down a line — and the alignment, which
// alignedWidth discounts it from so that a centred line does not sit half a
// space off centre. Justification is the one consumer §4.1.2 names that this
// engine still does not have, and textalign.go reports it rather than setting
// justified text ragged in silence.
// An inline box's own margin, border and padding is not text and does not stop
// the rule reaching the space before it: "<span>word </span>" ends the line with
// a space whether or not the span has a margin, and the span's margin is still
// its margin once the space has gone. So the scan looks past an inset and the
// inset is kept.
func trimLineEdge(line []Item) []Item {
	end := len(line)
	for end > 0 && (line[end-1].TrimAtEnd || line[end-1].Inset) {
		end--
	}
	if end == len(line) {
		return line
	}
	// Cutting the capacity keeps the append below from writing over the items
	// after end, which are still the caller's.
	out := line[:end:end]
	for _, item := range line[end:] {
		if item.Inset {
			out = append(out, item)
		}
	}
	return out
}

// isLineTailSpace reports whether an item can be part of the white space that
// ends a line: the space itself, an inline box's own inset, and a box that is
// out of flow.
//
// The last two are there so that a span wrapped around the spaces, or an
// absolutely positioned box written among them, does not break the run in two
// and make the half before it breakable again. Neither is content — §4.1.2's
// rules are about the text — and neither takes the line anywhere.
func isLineTailSpace(item Item) bool {
	if item.Inset || item.Abs != nil {
		return true
	}
	return isTailSpace(item)
}

// isTailSpace reports whether the end of a line does something to this white
// space: §4.1.2's third rule removes it, or its fourth hangs it.
//
// break-spaces is the value where neither happens — its spaces are data, they
// take room, and §3 puts an opportunity after every one of them — so a line may
// end inside a run of them and this must not say otherwise.
//
// A line may not end *at* one of these, and the half of that the fill did not
// already have is the removed half. §4.1.2's two rules are written over a
// "sequence" of white space, and a sequence is not all one kind: "ああ␣␣ ␣ ␣ああ"
// under white-space: normal is two ideographic spaces, a collapsible one, and
// two more ideographic spaces, all in a row. The ideographic ones hang, so no
// break was taken at them; the collapsible one does not hang, it is merely
// removed — and U+3000 offers an opportunity after itself, so that collapsible
// space began one. The line ended there, the rest of the run went to the line
// below, and a document whose spaces were supposed to hang off one line got a
// second line made of nothing but spaces and a third holding the words. Three
// lines where the specification asks for two.
//
// So the question the fill has to ask at an opportunity is not only "does this
// item hang" but "would ending here leave the line inside white space that the
// end of a line is about" — which is this, and which the paragraph's own
// trailing run (tailFrom) already answered for the one place it could see.
//
// The two questions are asked side by side rather than folded into one, because
// hanging is not only a property of spaces: hanging-punctuation sets Hangs on a
// quotation mark, which is not white space and which this must not claim to be.
func isTailSpace(item Item) bool {
	return item.Space && (item.Hangs || item.TrimAtEnd)
}

// breakInsideWord is overflow-wrap's last resort: the largest prefix of an item
// that fits, cut where a grapheme cluster ends.
//
// The cut is at a cluster boundary and not at a character, for the reason
// break-all's is — CSS Text §2 puts a soft wrap opportunity between typographic
// character units, and a cut inside one separates a letter from its accent. It
// is the same rule and the same table; only when it applies differs.
//
// The prefix is found by bisection over the boundaries rather than by measuring
// the text a cluster at a time. Widths do not add up: a face may kern or ligate
// across the join, so a running total is not the width of the prefix it claims
// to be. Bisection measures each candidate whole, which is exact for the one
// that is chosen, and costs a logarithmic number of measurements rather than
// one per character — which matters, because the input is untrusted and this is
// reached precisely for the longest words in a document.
//
// It reports false when there is nothing to gain: an item with one cluster, or
// one whose first cluster already overflows. Both leave the word to overflow and
// be reported, which is right — a line cannot hold less than one character, and
// breaking one off to leave the rest overflowing anyway would only lose a
// character off the end.
func (br *Breaker) breakInsideWord(item Item, width style.Unit, content bool) (head Item, at int, ok bool) {
	if !item.BreakWord || item.Face == nil || item.Text == "" {
		return Item{}, 0, false
	}
	if width <= 0 && content {
		// No room at all, and a line with something on it can end and let the
		// word begin the next one — where there will be a whole line's room.
		return Item{}, 0, false
	}
	bounds := segment.Boundaries(nil, item.Text)
	if len(bounds) == 0 {
		return Item{}, 0, false // one cluster: nothing to cut
	}

	// The largest boundary whose prefix fits. Bisection needs the predicate to
	// be monotone, and it is for any face whose advances are non-negative: a
	// longer prefix is never narrower. A face with a negative advance would make
	// this pick a cut that is merely *a* fitting one rather than the longest,
	// which is a worse line and not a wrong page.
	lo, hi := 0, len(bounds) // lo is known to fit (the empty prefix), hi is not known
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if mid > len(bounds) {
			break
		}
		// Measured in the context the prefix will be shaped in, which is what
		// SplitItem gives it: the rest of the word follows it whether or not
		// the line does. Measuring it alone picked a cut against one width and
		// then drew the head at another — the two disagree by exactly the
		// difference between a final form and an isolated one.
		cut := bounds[mid-1]
		w := br.MeasureSpacedInContext(item.Face, item.Text[:cut], item.Size, item.Spacing,
			item.PreContext, item.Text[cut:]+item.PostContext)
		if w <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		// Not even one cluster fits. On a line with something on it that is not
		// this line's problem: it ends, and the word begins the next one.
		if content {
			return Item{}, 0, false
		}
		// On an empty line it is. Something has to go on it or the word begins
		// the same line for ever, and what goes on it is one cluster — which
		// overflows, and is the least that can.
		//
		// This is what "width: 0" means, and the suite writes it that way on
		// purpose: overflow-wrap-cluster-001 is two Devanagari clusters in no
		// room at all against a reference of two lines, and the answer is one
		// cluster on each rather than both on one. A word does not become
		// unbreakable because the room ran out completely — it is exactly then
		// that the last resort is for.
		lo = 1
	}
	at = bounds[lo-1]
	head, _ = br.SplitItem(item, at)
	return head, at, true
}

// contentOnLine reports whether what is already on a line would keep it from
// being one of §9.4.2's zero-height line boxes.
//
//	Line boxes that contain no text, no preserved white space, no inline
//	elements with non-zero margins, padding, or borders [...] must be treated
//	as zero-height line boxes.
//
// It is asked for §10.6.4's hypothetical box: an absolutely positioned box would
// have split the inline content it was written among, and whether its own top
// edge is that line's bottom or its top is whether the line above it exists at
// all.
//
// An inline box's own edge counts by what it *is* rather than by what it
// measures — a border cancelled by a negative margin is still a border — and it
// counts on the side the fragment holds. That is Item.Edged, and the two halves
// of it are what abspos/static-inside-inline-002 and -003 are written to tell
// apart.
func contentOnLine(line []Item) bool {
	for _, item := range line {
		switch {
		case item.Inset:
			if item.Edged {
				return true
			}
		case item.Abs != nil || item.Float != nil:
			// Out of flow: written here and drawn somewhere else, so it puts
			// nothing on this line.
		case item.Text != "" || item.Atomic != nil || item.Tab || item.Forced:
			return true
		}
	}
	return false
}

// breaksAfterLast reports whether the last thing on the line is a run that
// allows overflow-wrap's last-resort break, and so offers an opportunity after
// its final character.
//
// Insets and hanging characters are stepped over, and that is not tidiness: a
// span with padding on it ends in a *closing edge*, not in its last character,
// and reading the edge instead of the character asks overflow-wrap of the wrong
// thing — of a box side, which never allows the break. A single pixel of
// padding-right would otherwise turn three lines back into two. A hung
// character is skipped for the neighbouring reason: it sits past the line's end
// and is not what a break here would follow.
//
// An empty line answers false, which is what makes the caller's "the line holds
// something" test unnecessary: a break at the end of the previous run is not a
// break at all when there is no previous run.
func breaksAfterLast(line []Item) bool {
	for i := len(line) - 1; i >= 0; i-- {
		it := line[i]
		if it.Inset || it.Hangs {
			continue
		}
		return it.BreakWord
	}
	return false
}
