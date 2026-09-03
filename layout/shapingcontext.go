package layout

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Shaping across an inline element boundary, CSS Text §8.1.
//
// A cursive script chooses each letter's shape from its neighbours. A run is not
// always a whole word — "ع<span>ع</span>ع" is one Arabic word written as three
// text nodes — and §8.1 says so in as many words:
//
//	the boundary between two inline elements does not break shaping
//
// Shaped a run at a time, every letter of that word comes out in its isolated
// form: three letters standing apart where a reader of the script expects one
// joined word. It is not a subtle difference and it is not rare — a <b>, an <em>
// or a coloured <span> inside a word is ordinary markup.
//
// # What is done
//
// Each run is given the text either side of it, and the shaper chooses the forms
// as though the whole were one run. The run is still shaped, measured and drawn
// on its own — the context decides which shapes, and nothing else.
//
// So a ligature that *spans* the boundary is not formed: a lam-alef written with
// the lam in one run and the alef in another stays two letters. The suite has a
// test of exactly that, shaping_lig-000, and it is the one of the twenty-three
// that this does not fix. It cannot be fixed this way and probably not any way
// that keeps a run measurable on its own: the glyph would belong to two runs at
// once, and either both draw it or one of them draws nothing.
//
// # Where the boundary does break shaping
//
// §8.1 continues: shaping is broken where the two sides differ in the styling
// that shaping depends on. What that comes to here is
//
//   - a different face, which is what a different font-family or a real bold
//     or italic gives;
//   - a different size;
//   - different letter-spacing or word-spacing;
//   - anything between them that occupies room — a margin, a border or padding
//     on the inline box, an atomic inline, a tab, a forced break.
//
// The suite tests the pairs: shaping-004 through -007 set a margin, padding,
// border and font-size to values that change nothing and ask for the letters to
// join, and shaping-008 through -011 set the same four to values that do and ask
// for them not to. So a *zero* inset is passed over and a non-zero one stops the
// context, which is why the width is read rather than the presence of the item.

// # What an intrinsic measurement cannot see
//
// This runs twice: once over the items a line will be filled from, and once over
// the items an intrinsic width is measured from. The second has no resolved
// embedding levels — nothing has laid the text out yet — so it reads every
// boundary as though the levels either side matched, and an isolation boundary
// looks like an ordinary one.
//
// So the two can disagree about text whose shaping an isolate breaks, and a box
// shrink-wrapped around such text is measured from forms it will not be drawn
// in. Measured on three Arabic letters with an isolate around the middle one,
// the box comes out 62.2px against 60.8px of runs — a difference of about a
// pixel and a half at 40px, and in the direction that leaves room rather than
// taking it away. There is no test of it: what it is is a *disagreement*
// between two measurements, and pinning either number would pin this font.
//
// The alternative is not measuring with the context at all, and that is worse
// for far more documents: every ordinary Arabic word split by a <span> would be
// measured as isolated letters. It also costs the suite's
// line-break-shaping-001 and gains nothing.
//
// Doing it properly means resolving the embedding levels during an intrinsic
// measurement, which is a much larger change than this one.

// linkShapingContext gives every run of text the characters either side of it,
// where the boundary between them does not break shaping, and re-measures the
// ones whose width the context changes.
//
// Nothing happens at all unless the face can be changed by its context, and
// there are two ways it can: positional forms, and pair kerning across the
// boundary. A face with neither takes the first branch of the loop and is left
// exactly as it was — no second measurement, no memo entries, no allocation.
//
// Kerning was the later of the two and the reading was narrower before it: the
// gate asked only about positional forms, so every Latin and CJK document was
// waved through and "A<span>V" lost its pair. It is the same rule of §8.1 in
// both cases — the boundary between two inline elements does not break shaping
// — and pair positioning is as much a part of shaping as a form is.
func (l *layouter) linkShapingContext(items []inlineItem) []inlineItem {
	joins := false
	for i := range items {
		if isShapedRun(items[i]) && contextCanChange(items[i].Face) {
			joins = true
			break
		}
	}
	if !joins {
		return items
	}
	for i := range items {
		if !isShapedRun(items[i]) || !contextCanChange(items[i].Face) {
			continue
		}
		before, after := "", ""
		// Whether a pair that spans the boundary is this font's to apply, which
		// is a different question from whether the context reaches the run at
		// all. See Item.ContextKerns.
		kerns := true
		if j, ok := shapingNeighbour(items, i, -1); ok {
			before = items[j].Text
			kerns = kerns && items[j].Face == items[i].Face
		}
		if j, ok := shapingNeighbour(items, i, +1); ok {
			after = items[j].Text
			kerns = kerns && items[j].Face == items[i].Face
		}
		if before == "" && after == "" {
			continue
		}
		items[i].PreContext, items[i].PostContext = before, after
		items[i].ContextKerns = kerns
		// The advance changes with the form, so what was measured without the
		// context is not what will be drawn with it. Measuring again is the
		// whole reason the context has to be settled before the lines are
		// filled rather than at paint time.
		items[i].Width = l.br.MeasureSpacedInContext(items[i].Face, items[i].Text,
			items[i].Size, items[i].Spacing, itemShaping(&items[i]))
	}
	return items
}

// isShapedRun reports whether an item is a run of text that a face shapes: not a
// tab, whose advance is a distance rather than a string, and not one of the
// markers that stand for something which is not text at all.
func isShapedRun(item inlineItem) bool {
	return item.Text != "" && item.Face != nil && !item.Tab && !item.Forced &&
		!item.Inset && item.AtomicBox == nil && item.Float == nil && item.Abs == nil
}

// shapingNeighbour finds the run that gives item i its context on one side, or
// says there is none.
//
// step is -1 for the text before and +1 for the text after. The walk passes over
// the items that are not on the page — an inline box's own inset where it has no
// width, and the record of a box that is out of flow — and stops at anything
// else, because anything else is either room between the two runs or is not text.
func shapingNeighbour(items []inlineItem, i, step int) (int, bool) {
	for j := i + step; j >= 0 && j < len(items); j += step {
		switch {
		case items[j].Abs != nil || items[j].Float != nil:
			// Out of flow: written between the two runs and drawn somewhere
			// else entirely, so it stands between nothing.
			continue
		case items[j].Inset:
			// An inline box's own margin, border and padding. A zero one is a
			// boundary and nothing more — which is what shaping-004 through
			// -006 are — and one with width is room between the letters, which
			// is what -009 through -011 are.
			if facingInset(items[j], items[i].Level&1 == 1) == 0 {
				continue
			}
			return 0, false
		case !isShapedRun(items[j]):
			return 0, false
		}
		if !sameShaping(items[i], items[j]) {
			return 0, false
		}
		return j, true
	}
	return 0, false
}

// facingInset is how much of an inline box's own horizontal margin, border and
// padding stands at this boundary.
//
// Which physical side that is depends on which way the text runs, not on which
// end of the box the edge is. In right-to-left text the earlier word is drawn to
// the right, so a box that *opens* between two runs presents its right edge to
// the boundary and one that *closes* there presents its left; in left-to-right
// text it is the other way round.
//
// The suite states it in eight rows. boundary-shaping-009 puts a ten-pixel
// padding-right on the span holding the second Arabic word and asks for the
// words to be separated — that padding is between them — and then puts a
// padding-left on the same span and asks for them to be joined, because that one
// is on the far side. Reading Width instead answers the first pair and gets the
// second backwards, and it does so differently depending on the enclosing
// element's direction, which changes nothing about where the padding is drawn.
func facingInset(item inlineItem, rtl bool) style.Unit {
	if item.InsetLead == rtl {
		return item.InsetRight
	}
	return item.InsetLeft
}

// sameShaping reports whether two runs are shaped by the same rules, which is
// what decides whether the boundary between them breaks shaping.
//
// The face is deliberately not among them, and that is the correction the
// suite's cross-font tests forced. Which of its four shapes a letter takes is
// decided by the characters beside it, and a character is the same character
// whichever font sets it — Unicode's joining enforcement, and
// shaping-join-002 and shaping-tatweel-002 and -003, where a zero width joiner
// or a tatweel is pulled from another font by unicode-range and the Arabic
// letters either side must still take their joined forms. A declared font is
// not the question either: a bold <b> in a document that supplied only a
// regular face is set in that face, which is what shaping-002 and shaping-018
// are.
//
// What the face *does* decide is whether a pair across the boundary is this
// font's to apply, which is Item.ContextKerns and is answered by the caller.
//
// The embedding level is the one that is not about styling at all. A bidi
// isolate — <bdi>, dir="auto", unicode-bidi: isolate — raises the level of what
// is inside it, and two runs at different levels are on opposite sides of a
// directional boundary: their characters are not adjacent in the reordered text
// and need not be adjacent on the page. So the letters must not join across one,
// and the suite says so in as many words — shaping-012 and shaping-013 are the
// same three Arabic letters with a <bdi> and a dir="auto" in the middle, and
// both read "Test passes if the three Arabic characters DON'T join".
func sameShaping(a, b inlineItem) bool {
	if a.Spacing != b.Spacing || a.Level != b.Level {
		return false
	}
	// Of those three, the face has no test: a planted defect dropping it leaves
	// every one passing, because the only Arabic face in the checkout is one and
	// two runs cannot be set in different ones. It is kept because a face is
	// what a form *is* — a glyph index means nothing outside the font it came
	// from — and recorded here rather than left as an implied claim. The other
	// two are pinned: shaping-012 and -013 for the level, and the letter- and
	// word-spacing families for the spacing.
	// The size is the one that answers differently for the two things a context
	// does.
	//
	// It does not change which *form* a letter takes: an Arabic letter is
	// medial because of the letters beside it and not because of how large it
	// is, and the suite says so directly — shaping-007 sets "font-size: 100%"
	// on the middle letter and shaping-008 sets "120%", and *both* read "Test
	// passes if the three Arabic characters in each box join". What breaks the
	// join is room between the letters, which is -009 through -011: a margin, a
	// padding and a border.
	//
	// It does change what a pair between them would be. A kern is a distance
	// measured in one font at one size, and a pair positioned across a boundary
	// where the sizes differ is a number that belongs to neither of them.
	//
	// So the size breaks the boundary for a face that kerns and not for one
	// that joins. A face that does both is read as joining, because that is the
	// difference a reader sees: a letter in the wrong form is a different
	// letter, and a pair off by a fraction of an em is a gap.
	return a.Size == b.Size || a.Face.HasJoiningForms()
}

// contextCanChange reports whether the text either side of a run can change what
// the run is: which glyphs it is set in, or where they sit.
func contextCanChange(f *shape.Face) bool {
	return f.HasJoiningForms() || f.HasKerning()
}

// itemShaping is everything about how an item is set that its own text does not
// say, gathered for the measure. See paragraph.Shaping.
func itemShaping(it *inlineItem) shaping {
	return shaping{
		Before: it.PreContext, After: it.PostContext,
		ContextKerns: it.ContextKerns, Upright: it.Upright, Off: it.Off,
	}
}
