package layout

import (
	"github.com/mgilbir/forme/style"
)

// hanging-punctuation, CSS Text §8.4, as it reaches a line box.
//
// "first" hangs an opening bracket or quote into the margin before the first
// formatted line; "last" hangs a closing one past the end of the last. Both are
// about a character that is *on* the line and does not count towards its
// measure, which is a thing this engine already does — §4.1.2 hangs the white
// space at the end of a line the same way, and the machinery for it is what
// this reuses.
//
// # Why the character becomes an item of its own
//
// A hang is a width, and a width is a property of an item. "(This" is one run
// and the bracket is one character of it, so nothing downstream could discount
// the bracket without measuring a substring of a run it was handed whole —
// which is a measurement in the wrong place and a second answer about the same
// text. Cutting the character out into its own item gives every stage that
// already knows how to leave an item out of a line's measure the item to leave
// out: the fill, the alignment and the intrinsic widths all read the same flag.
//
// # Which line
//
// The first item of the block lands on the first line and the last on the last,
// so "the first formatted line" needs no counting: the cut is made once, at the
// two ends of the item list, and the lines fall where they fall. The suite's own
// fixture is a block whose *second* line begins with a bracket too —
// "(This should hang.<br>(This should not." — and that one is not the first
// item, so nothing about it hangs.
//
// # What stops it
//
// A box edge with room in it. "<span style='border-left:1em'>(</span>text" must
// not hang, and the suite says so by name: the reference for
// hanging-punctuation-first compensates that row with a margin instead of an
// indent. So the walk to the first run passes over what takes no room — a zero
// inset, a box out of flow, a collapsible space §4.1.1 removes at the edge of a
// line anyway — and stops at anything that does, which is the same rule
// shapingcontext.go applies to a boundary and for the same reason.

// hangPunctuation cuts the characters §8.4 hangs out of the runs at the two ends
// of a block's content, and marks them.
//
// It returns the items unchanged when the property asks for nothing, which is
// every document that does not use it.
func (l *layouter) hangPunctuation(items []inlineItem, hp hangingPunctuation) []inlineItem {
	if !hp.First && !hp.Last {
		return items
	}
	if hp.First {
		if i, ok := edgeRun(items, +1); ok {
			items = l.cutHang(items, i, true)
		}
	}
	if hp.Last {
		if i, ok := edgeRun(items, -1); ok {
			items = l.cutHang(items, i, false)
		}
	}
	return items
}

// edgeRun finds the run of text at one end of the item list: the first for
// step +1 and the last for step -1.
func edgeRun(items []inlineItem, step int) (int, bool) {
	i := 0
	if step < 0 {
		i = len(items) - 1
	}
	for ; i >= 0 && i < len(items); i += step {
		item := items[i]
		switch {
		case item.Abs != nil || item.Float != nil:
			// Out of flow: written here and drawn somewhere else entirely, so it
			// stands between nothing.
			continue
		case item.Inset:
			// An inline box's own margin, border and padding. A zero one is a
			// boundary and nothing more; one with room in it puts that room
			// between the margin and the character, and the character is then
			// not at the start of the line at all.
			if item.InsetLeft == 0 && item.InsetRight == 0 {
				continue
			}
			return 0, false
		case item.Collapsible:
			// §4.1.1 removes a collapsible space at the edge of a line, so it
			// stands between nothing either. hanging-punctuation-first-whitespace
			// and its "last" counterpart are that row.
			continue
		case item.Text == "" || item.Face == nil || item.Tab || item.Forced ||
			item.AtomicBox != nil:
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// cutHang splits the hanging character out of items[i] and marks it.
func (l *layouter) cutHang(items []inlineItem, i int, atStart bool) []inlineItem {
	item := items[i]
	var n int
	if atStart {
		n = leadingHang(item.Text)
	} else {
		n = trailingHang(item.Text)
	}
	if n == 0 {
		return items
	}
	if n == len(item.Text) {
		// The run is the character. Nothing to cut, and cutting would leave an
		// item with no text in it.
		items[i].HangStart, items[i].HangEnd = atStart, !atStart
		items[i].Hangs = items[i].Hangs || !atStart
		return items
	}
	at := n
	if !atStart {
		at = len(item.Text) - n
	}
	head, tail := l.br.SplitItem(item, at)
	if atStart {
		head.HangStart = true
	} else {
		tail.HangEnd = true
		// The end hang is white space's hang in everything the line filling
		// asks: it is on the line, it takes no room, and it never pushes what
		// follows it to the next line. There is nothing after it — it is the
		// last thing in the block — so the flag says only "do not count me".
		tail.Hangs = true
	}
	out := make([]inlineItem, 0, len(items)+1)
	out = append(out, items[:i]...)
	out = append(out, head, tail)
	out = append(out, items[i+1:]...)
	return out
}

// hangStartWidth is how far the first line reaches back into the margin, which
// is the width of the character hanging there and zero for every other block.
func hangStartWidth(items []inlineItem) style.Unit {
	for _, item := range items {
		if item.HangStart {
			return item.Width
		}
	}
	return 0
}

// hangEndWidth is how far past the end of a line §8.4's closing bracket or
// quote sits, and zero for every line that does not end with one.
//
// It is asked of the line rather than of the block, and the answer is non-zero
// only on the line the last item landed on — which is the last formatted line,
// which is the one the property is about.
func hangEndWidth(runs []inlineItem) style.Unit {
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].HangEnd {
			return runs[i].Width
		}
		if runs[i].Inset {
			continue
		}
		return 0
	}
	return 0
}
