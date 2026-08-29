package layout

import (
	"github.com/mgilbir/forme/paragraph"
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
// where the run beside it there is in a different element.
//
// It does nothing at all for a paragraph whose boxes agree about the property,
// which is every paragraph that does not set it twice.
func (l *layouter) linkLetterSpacing(items []inlineItem) []inlineItem {
	// The order the runs are *drawn* in, which is the order the gaps fall in.
	// See gapNeighbour, which is the whole of why this is asked for.
	order := lineVisualOrder(items)
	at := visualPositions(order, len(items))
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
		j, ok := gapNeighbour(items, order, at, i)
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

// visualPositions inverts a visual order into a position per item, so that a run
// can be found among its neighbours without a scan.
//
// A nil order is the logical one, which is what LineVisualOrder returns for
// every line of a left-to-right document and what most documents are.
func visualPositions(order []int, n int) []int {
	if order == nil {
		return nil
	}
	at := make([]int, n)
	for p, i := range order {
		at[i] = p
	}
	return at
}

// gapNeighbour finds the run on the other side of the gap this one carries.
//
// A run's advance holds one spacing per character, the last of them included,
// and that last one is the gap. It sits at the run's **right edge whatever the
// run's direction**, because that is where the drawing puts it: a run is drawn
// from its own origin accumulating glyph advances, the shaper hands back an
// Arabic or Hebrew run's glyphs already in visual order, and the spacing is
// added after each of them — so the one after the last glyph drawn is at the
// right of the run and not at its left.
//
// That was checked against the display list rather than reasoned about, because
// the reasoning is easy to get backwards: a right-to-left run laid out in a
// "dir=rtl" block *does* grow leftwards when a letter-spacing is added, which
// looks like the gap arriving on its left and is really the block aligning the
// run's right edge.
//
// So the run across the gap is the visually next one, and the whole point of
// asking is that §8.2's boundary rule is about two characters that are actually
// next to each other.
//
// # Why visual and not logical
//
// Because the specification says "letter spacing is inserted after bidi
// reordering", and because the two readings of that give different answers to
// two families of the suite and only this one satisfies both.
//
// letter-spacing-bidi-001 is "a<span>bא</span>ב" with the span's letter-spacing
// at 1ch and the div's at zero. Its assert is that "letter spacing cannot apply
// to any of the letters in the span, since they get split apart", and the line
// has to come out four characters wide. Logically b and א are a pair inside the
// span, so the logical reading puts a gap between them; visually the line reads
// a, b, ב, א, and the pairs across the gaps are (a,b), (b,ב) and (ב,א) — every
// one of which has the *div* as its innermost common ancestor. Four characters.
//
// The reading that is *not* right is "drop the gap where the two characters end
// up in different level runs", which is the obvious first summary of the same
// assert. CSS2's bidi-005 through bidi-010 disprove it: each builds "a" to "m"
// out of nested RLO and LRO overrides and asks it to render identically to the
// same letters written plainly, with "letter-spacing: 1em" on both. Every
// character there is its own level run, so dropping the gap at each boundary
// would take the paragraph to half its width. Under visual adjacency every pair
// is governed by the paragraph, every gap is one em, and the two agree.
func gapNeighbour(items []inlineItem, order, at []int, i int) (int, bool) {
	return nextInDirection(items, order, at, i, 1)
}

// nextInDirection walks the visual order one way from a run, passing over what
// is not a character and stopping at anything that is not a run of text.
func nextInDirection(items []inlineItem, order, at []int, i, step int) (int, bool) {
	pos := i
	if at != nil {
		pos = at[i]
	}
	n := len(items)
	for p := pos + step; p >= 0 && p < n; p += step {
		j := p
		if order != nil {
			j = order[p]
		}
		switch {
		case items[j].Abs != nil || items[j].Float != nil || items[j].Inset:
			// Out of flow, or an inline box's own edge: neither is a character,
			// so neither is what the spacing goes beside. The edge is exactly
			// what this walk has to look past — the boundary it marks is the one
			// being asked about.
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
	return cursiveTrackingSuppressesText(item.Text)
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

// trailingSpacingOf is everything at a run's far edge that a line ending there
// leaves out: the letter-spacing after its last character and §8.1's gap to
// whatever follows.
//
// It is the breaker's own answer rather than a second one. The intrinsic pass
// and the fill are two measurements of the same line, and a line whose measure
// differs between them is a box shrink-wrapped to a width its own content does
// not have.
func trailingSpacingOf(item inlineItem) style.Unit {
	return paragraph.TrailingSpacing(item)
}
