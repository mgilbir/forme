package layout

import (
	"github.com/mgilbir/forme/style"
)

// letter-spacing at an element boundary, CSS Text §8.2.
//
// The property adds space after each typographic character unit, and the
// question this answers is *whose* letter-spacing applies between two
// characters that are in different elements. The answer is the innermost
// element containing both of them, which is not the same as either character's
// own — and reading either character's own is what this engine did.
//
// The suite states it in a page of rows. Two of them are enough to see the
// shape, both from letter-spacing-203, and both are one paragraph in Ahem with
// three A's and three B's in it:
//
//	<p class="ls1"><span class="ls0">AAA</span><span class="ls0">BBB</span></p>
//	    sets   AAA BBB      one gap, and it is the paragraph's
//
//	<p><span class="ls1">AAA</span><span class="ls1">BBB</span></p>
//	    sets   A A AB B B   spacing inside each span and none between them
//
// The same six characters, the same two values, opposite answers — and the only
// thing that differs is which element holds the boundary. Taking the spacing
// from the run before it gives the second row's answer for the first row and the
// first row's for the second.
//
// # What is adjusted
//
// The width of the run, and nothing else. Spacing *within* a run is unchanged,
// so the glyphs are drawn exactly where they were; what moves is where the run
// after it begins, which is the gap the rule is about. A backend is handed one
// spacing per run and could not express a different last one — and does not need
// to, because layout places every run itself and the gap is between two of them.

// linkLetterSpacing gives every run the spacing that belongs at its far edge,
// where the run after it is in a different element.
//
// It does nothing at all for a paragraph whose boxes agree about the property,
// which is every paragraph that does not set it twice.
func (l *layouter) linkLetterSpacing(items []inlineItem) []inlineItem {
	for i := range items {
		if !isSpacedRun(items[i]) {
			continue
		}
		if cursiveTrackingSuppresses(items[i]) {
			// §8.2's cursive tracking: this run took no spacing after any of
			// its characters, so there is none after its last one to exchange
			// for the boundary's. Reaching the arithmetic below would take a
			// spacing off a width that never had one — and it is not the same
			// question as "is the declared spacing zero", which is a run that
			// still takes the boundary's value. letter-spacing-203 is that
			// case, twice over.
			continue
		}
		j, ok := nextSpacedRun(items, i)
		if !ok {
			// Nothing follows it in this block, so the spacing after its last
			// character is its own — and hangs, as §4.1.2 leaves it.
			continue
		}
		want, ok := l.boundarySpacing(items[i], items[j])
		if !ok || want == items[i].Spacing.Letter {
			continue
		}
		// The run's advance holds one spacing per character, the last of them
		// included. Exchange that one for the boundary's.
		items[i].Width = items[i].Width.Sub(items[i].Spacing.Letter).Add(want)
	}
	return items
}

// isSpacedRun reports whether an item is a run of text that carries
// letter-spacing: not a tab, whose advance is a distance rather than a string,
// and not one of the markers that stand for something else.
func isSpacedRun(item inlineItem) bool {
	return item.Text != "" && item.Face != nil && !item.Tab && !item.Forced &&
		!item.Inset && item.AtomicBox == nil && item.Float == nil && item.Abs == nil
}

// nextSpacedRun finds the run whose first character is on the far side of this
// one's last, passing over what is not on the line.
func nextSpacedRun(items []inlineItem, i int) (int, bool) {
	for j := i + 1; j < len(items); j++ {
		switch {
		case items[j].Abs != nil || items[j].Float != nil || items[j].Inset:
			// Out of flow, or an inline box's own edge: neither is a character,
			// so neither is what the spacing goes in front of. The edge is
			// exactly what this walk has to look past — the boundary it marks is
			// the one being asked about.
			continue
		case !isSpacedRun(items[j]):
			return 0, false
		}
		return j, true
	}
	return 0, false
}

// boundarySpacing is the letter-spacing of the innermost element containing both
// runs.
//
// It walks up from each box marking what it meets, which is linear in the depth
// of the tree and needs no precomputed ancestry. A document nests inline boxes a
// handful deep; the walk is over that and not over the paragraph.
//
// The second result is false where there is no common ancestor to ask, which a
// well-formed tree does not produce and a caller should not act on.
func (l *layouter) boundarySpacing(a, b inlineItem) (style.Unit, bool) {
	box := commonAncestor(heldBox(a.Box), heldBox(b.Box))
	if box == nil {
		return 0, false
	}
	return l.spacingFor(box).Letter, true
}

// commonAncestor is the innermost box containing both, or nil.
//
// It answers two questions that turn out to be the same one. §8.2's is whose
// letter-spacing applies between two characters in different elements; §5.1's is
// whose white-space governs a soft wrap opportunity between them. Both are "the
// innermost element containing both of them", and the walk that finds it belongs
// in one place — see flatten.go, which asked the second question with a second
// copy of this until they were put together.
//
// Nil where the two are in different trees, which a well-formed document does
// not produce and which is answered rather than assumed: each caller keeps the
// answer it had before it asked.
func commonAncestor(a, b *Box) *Box {
	if a == nil || b == nil {
		return nil
	}
	seen := map[*Box]bool{}
	for p := a; p != nil; p = p.Parent {
		seen[p] = true
	}
	for p := b; p != nil; p = p.Parent {
		if seen[p] {
			return p
		}
	}
	return nil
}

// trailingSpacing is the letter-spacing after the last character of a line,
// which §8.2 adds and which hangs past the end rather than counting towards it.
//
// It is asked of the runs in logical order and looks past an inline box's own
// edge, which is not a character and carries no spacing of its own.
func trailingSpacing(runs []inlineItem) style.Unit {
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Inset {
			continue
		}
		if !isSpacedRun(runs[i]) {
			return 0
		}
		return runs[i].Spacing.Letter
	}
	return 0
}

// cursiveTrackingSuppresses reports whether §8.2 takes the letter-spacing off a
// run entirely.
//
// A run of a cursive script takes none after any of its characters, the last one
// included — see paragraph.SpacedUnits — and flatten.go cuts a run where the
// answer changes, so a run is one or the other and never both.
//
// It is asked of a *run of text*. An item with no text is not one: an inline
// box's edge is not a character, and an atomic inline is a character unit
// letter-spacing goes after like any other. Neither is what this rule is about,
// and answering for them would take a spacing off a run that has one.
func cursiveTrackingSuppresses(item inlineItem) bool {
	return item.Text != "" && spacedUnits(item.Text) == 0
}

// trackingOf is the letter-spacing a run is drawn with and measured to: the
// declared value, or nothing where §8.2 forbids it.
//
// A display list carries one number per run — an advance added after every
// glyph — so the run's number has to be the answer rather than the declaration.
func trackingOf(item inlineItem) style.Unit {
	if cursiveTrackingSuppresses(item) {
		return 0
	}
	return item.Spacing.Letter
}
