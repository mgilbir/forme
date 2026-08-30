package layout

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// text-align: justify — CSS 2.1 §16.2, and the expansion rules of CSS Text 3 §7.
//
// The line has already been broken, positioned and aligned by the time anything
// here runs, and that order is what makes justification a small change rather
// than a second line-breaker: what is left over is a number, and the only
// question is where to put it. The breaking is not revisited, because a line
// that was broken to fit is a line whose slack is by definition less than the
// next word.
//
// # Which lines
//
// Every line but the last one of the block, and not one ended by a forced break.
// A last line set flush on both edges is the "rivers of white" a justified
// paragraph is meant to avoid: it has one word and a whole line to spread it
// over. §16.2 says the last line is aligned as "start" instead, and a line ended
// by <br> or a preserved newline is a last line for this purpose — the author
// ended it, so it is short because they said so rather than because it filled.
//
// # Where the space goes
//
// Between words, at ordinary spaces. CSS 2.1 leaves the algorithm to the UA and
// CSS Text 3 §7.3 makes "inter-word" the behaviour of text-justify: auto for
// scripts that have word separators, which is the case this engine can act on.
//
// It is ordinary spaces only, and not the rest of the word-separator list that
// word-spacing applies to. A no-break space is written to keep two things
// together, and stretching one to line up a margin is the one thing an author
// who typed it asked for it not to do. The Ethiopic and Aegean separators are
// left out with it rather than guessed at.
//
// A line with no space in it — one long word, or a line of Chinese — cannot be
// justified this way and is left where "start" put it, and *that* is what is
// still reported: the page is not what was asked for, and nothing here can make
// it so.

// justifiableSpace reports whether a run is a stretch of the ordinary spaces
// justification expands.
func justifiableSpace(text string) bool {
	if text == "" {
		return false
	}
	return strings.Trim(text, " ") == ""
}

// justifiableHere reports whether §7.3 allows an opportunity at this item.
//
// text-justify applies to inline boxes as well as to block containers — "none:
// justification is disabled: there are no justification opportunities within the
// text" — so a space inside a "text-justify: none" span is not one, even on a
// line the block is justifying. The property is inherited, so the item's own
// computed value is already the answer for wherever it was written; nothing has
// to walk the tree.
//
// The block's own value is read by lineAlignment, which turns a justified line
// into a start-aligned one and never gets here. This is the other half: a block
// that justifies with a span inside it that does not.
func justifiableHere(item inlineItem) bool {
	b := heldBox(item.Box)
	if b == nil {
		return true
	}
	m, _, _ := justificationOf(b)
	return m != justifyNone
}

// justifyItems spreads slack across the word spaces of one line, and reports
// whether it found anywhere to put it.
//
// It works on the line's items and their positions rather than on the runs that
// will be drawn from them, and that is the whole of what makes it correct. A
// line carries more than text: the atomic inlines, and the margin, border and
// padding an inline box contributes at each of its edges. All of them move when
// a space between them grows, and all of them are placed from these positions —
// so spreading the slack here moves everything by construction, where spreading
// it over the drawn runs moved the text and left the boxes behind.
//
// xs and widths are updated in place: xs is where each item sits before the
// line's own alignment offset is added, and widths is what each occupies on
// this line, which for a stretched space is more than the font gave.
//
// hangs marks the white space at the end of the line that §4.1.2 hangs past it
// — see hangingTail. It must not be stretched: it is not between two words and
// it is not on the page in the first place.
func justifyItems(items []inlineItem, xs, widths []style.Unit, hangs []bool, slack style.Unit) bool {
	if slack <= 0 || len(items) == 0 {
		return false
	}

	// The items in the order they appear across the line. X order and not slice
	// order: a right-to-left line's items are stored in logical order and drawn
	// in visual order, and what is being divided up is the visual gap between
	// the two margins.
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })

	// A space is an opportunity only when something follows it on the line. A
	// run of spaces at the visual end is the hang, and stretching it would push
	// the line's last word off the edge to make room for white space nobody can
	// see.
	last := -1
	for i, k := range order {
		if !justifiableSpace(items[k].Text) {
			last = i
		}
	}

	// And nothing at or before a preserved tab, which is CSS Text 4's rule for
	// justifying a line whose white space is not collapsible: "the UA must
	// ensure that tab stops continue to line up as required by the white space
	// processing rules".
	//
	// A tab's advance is the distance from where the line has got to to the next
	// tab stop, so widening a space in front of one does not move what follows
	// it — the tab simply shrinks by as much and the text after it stays put.
	// Until the pen crosses a stop, and then the tab jumps a whole stop and every
	// column on the line after it moves with it. Neither answer is justification:
	// the first spends slack that never reaches the margin, and the second
	// destroys the alignment the tabs were written for.
	//
	// So the slack goes after the last tab and nowhere else, which is what the
	// suite's text-align-justify-tabs-002 asks for by drawing its reference with
	// the spaces after the tab written twice and the ones before it written once.
	// A line whose last tab is at its end — 001, where the tab hangs — has no
	// opportunity left at all and is set exactly as an unjustified line is.
	lastTab := -1
	for i, k := range order {
		if items[k].Tab {
			lastTab = i
		}
	}
	// Which items expand. Nothing here reads a position, and that is worth
	// saying rather than assuming: an earlier version asked whether a space sat
	// past the end of the line's content, which is a question the loop below
	// changes the answer to as it moves things along — so every opportunity
	// after the first looked like a hanging space, and a line with two gaps came
	// out with one stretched and the other not. Asking hangs instead, which is a
	// property of the line's order rather than of anything's position, is what
	// makes one pass and two passes the same answer.
	expands := func(i, k int) bool {
		return i < last && i > lastTab && justifiableSpace(items[k].Text) &&
			!hangs[k] && justifiableHere(items[k])
	}
	n := 0
	for i, k := range order {
		if expands(i, k) {
			n++
		}
	}
	if n == 0 {
		return false
	}

	// The slack divided as evenly as the unit allows, with the remainder spread
	// one unit at a time over the leading gaps rather than dropped. A paragraph
	// whose lines each lost a fraction of a unit would drift away from its own
	// right margin down the page.
	base := slack.Div(float64(n))
	spent := base.Mul(float64(n))
	extra := slack.Sub(spent)

	var acc style.Unit
	seen := 0
	for i, k := range order {
		xs[k] = xs[k].Add(acc)
		if !expands(i, k) {
			continue
		}
		step := base
		if seen < int(extra) {
			step = step.Add(1)
		}
		seen++
		// The space grows and everything after it moves by as much. Widening it
		// without the shift would overlap the next word; shifting without the
		// widening would leave a decoration ruled under the line stopping short
		// of the gap it is meant to cross.
		widths[k] = widths[k].Add(step)
		acc = acc.Add(step)
	}
	return true
}

// justifyBetweenCharacters is §7.3's other method: the slack goes between every
// pair of typographic character units rather than at the word spaces.
//
// It is what "inter-character" asks for, and what "distribute" — the older name
// for the same thing — asks for too. Thai and Chinese are justified this way,
// and so is a line with no space in it at all: "XX" in a box twice its width is
// one X at each edge, which is what the suite's inter-character-001 draws with a
// float and asks this to match.
//
// The extra is returned rather than folded into the items, because it has to
// reach the *drawing* as well as the measure. A backend advances the pen by each
// glyph's own width plus the run's letter-spacing, so putting the slack there is
// what makes the glyphs land where the widths say they will — the same reason
// TextRun.LetterSpacing exists at all.
//
// The count is one short of the units on the line, because the opportunity is
// *between* a pair: n units offer n-1 of them. The trailing extra that
// letter-spacing adds after the last unit falls past the end of the line, where
// nothing follows it and nobody sees it.
func (l *layouter) justifyBetweenCharacters(items []inlineItem, xs, widths []style.Unit,
	hangs []bool, slack style.Unit) (style.Unit, bool) {

	if slack <= 0 || len(items) == 0 {
		return 0, false
	}
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })

	// The same two exclusions the word method makes: white space hanging past
	// the end of the line is not on the page, and nothing at or before a
	// preserved tab may move or the tab stops stop lining up.
	lastTab := -1
	for i, k := range order {
		if items[k].Tab {
			lastTab = i
		}
	}
	units := func(i, k int) int {
		if i <= lastTab || hangs[k] {
			return 0
		}
		if items[k].Atomic != nil || items[k].AtomicBox != nil {
			// A picture is a character unit: the slack goes round it as it goes
			// round a letter.
			return 1
		}
		if items[k].Face == nil || items[k].Text == "" {
			return 0
		}
		return paragraph.SpacedUnits(items[k].Text)
	}
	total := 0
	for i, k := range order {
		total += units(i, k)
	}
	if total < 2 {
		// One unit offers no opportunity, and none offers none.
		return 0, false
	}
	extra := slack.Div(float64(total - 1))
	if extra <= 0 {
		return 0, false
	}

	var acc style.Unit
	for i, k := range order {
		xs[k] = xs[k].Add(acc)
		n := units(i, k)
		if n == 0 {
			continue
		}
		grew := extra.Mul(float64(n))
		widths[k] = widths[k].Add(grew)
		acc = acc.Add(grew)
	}
	return extra, true
}

// writtenWithoutWordSeparators reports whether a line's own text is written in a
// script that does not separate its words with spaces.
//
// It is asked of the *line* rather than of the element, because the element is
// where a language would be declared and a document need not declare one:
// text-align-last-justify-br is a bare "<p>東京<br>京城</p>" with no lang at all,
// and the script is the only evidence there is.
//
// Every letter, and not merely one of them. A line holding a Latin word has word
// spaces to stretch even if it also holds an ideograph, and stretching such a
// line between its characters would pull the Latin word apart — which is what
// inter-character justification does and what no script using spaces wants. So
// the test is that nothing on the line is a letter of a script that has spaces,
// which leaves punctuation and marks to go either way.
//
// A line with no letters at all answers false. There is nothing to be
// script-appropriate about, and the word method's own "no opportunity" answer —
// place it at the start edge, which §7.3 names as the conforming rendering — is
// the right one.
func writtenWithoutWordSeparators(items []inlineItem) bool {
	seen := false
	for _, item := range items {
		if item.Face == nil || item.Text == "" || item.Tab {
			continue
		}
		for _, r := range item.Text {
			switch {
			case isIdeographic(r):
				seen = true
			case unicode.IsLetter(r), unicode.IsDigit(r):
				return false
			}
		}
	}
	return seen
}
