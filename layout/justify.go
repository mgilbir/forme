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

// justifyLine spreads slack across the word spaces of one line, and reports
// whether it found anywhere to put it.
//
// runs and atomics are everything positioned on the line: the text and the
// inline-level boxes — an image, an inline-block — which move with the text
// around them or the line comes apart.
//
// contentEnd is where the line's own content stops, which is not where the line
// box does: trailing white space hangs past it under §4.1.2 and must not be
// stretched, because it is not between two words and because it is not on the
// page in the first place.
func justifyLine(runs []TextRun, atomics []*Fragment, contentEnd, slack style.Unit) bool {
	if slack <= 0 || len(runs) == 0 {
		return false
	}

	// The expansion opportunities, in the order they appear across the line.
	// X order and not slice order: a right-to-left line's runs are stored in
	// logical order and drawn in visual order, and what is being divided up is
	// the visual gap between the two margins.
	type slot struct {
		x     style.Unit
		run   int // index into runs, or -1 for an atomic
		child int // index into atomics
	}
	slots := make([]slot, 0, len(runs)+len(atomics))
	for i := range runs {
		slots = append(slots, slot{x: runs[i].X, run: i, child: -1})
	}
	for i := range atomics {
		slots = append(slots, slot{x: atomics[i].BorderRect.X, run: -1, child: i})
	}
	sort.SliceStable(slots, func(a, b int) bool { return slots[a].x < slots[b].x })

	// A space is an opportunity only when something follows it on the line. A
	// run of spaces at the visual end is the hang, and stretching it would push
	// the line's last word off the edge to make room for white space nobody can
	// see.
	last := -1
	for i, s := range slots {
		if s.run < 0 || !justifiableSpace(runs[s.run].Text) {
			last = i
		}
	}
	expands := func(i int, s slot) bool {
		return i < last && s.run >= 0 && justifiableSpace(runs[s.run].Text) &&
			s.x < contentEnd
	}
	n := 0
	for i, s := range slots {
		if expands(i, s) {
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
	for i, s := range slots {
		if s.run >= 0 {
			runs[s.run].X = runs[s.run].X.Add(acc)
		} else {
			atomics[s.child].BorderRect.X = atomics[s.child].BorderRect.X.Add(acc)
		}
		if !expands(i, s) {
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
		runs[s.run].Width = runs[s.run].Width.Add(step)
		acc = acc.Add(step)
	}
	return true
}
