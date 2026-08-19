package layout

import (
	"sort"
	"strings"

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
	// Which items expand. Nothing here reads a position, and that is worth
	// saying rather than assuming: an earlier version asked whether a space sat
	// past the end of the line's content, which is a question the loop below
	// changes the answer to as it moves things along — so every opportunity
	// after the first looked like a hanging space, and a line with two gaps came
	// out with one stretched and the other not. Asking hangs instead, which is a
	// property of the line's order rather than of anything's position, is what
	// makes one pass and two passes the same answer.
	expands := func(i, k int) bool {
		return i < last && justifiableSpace(items[k].Text) && !hangs[k]
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
