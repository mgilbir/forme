package layout

import (
	"strings"

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
// A ligature that *spans* the boundary is formed as well, and that is the
// second half of this file rather than something it declines. The runs of a
// group are shaped as one string and the glyphs divided between them by the
// cluster each came from, so the glyph belongs to whichever run holds its first
// character and the other draws nothing for the characters it swallowed. See
// mergeGroupAround, and shape.ShapeGlyphsMerged for the division.
//
// This used to say the case could not be fixed "any way that keeps a run
// measurable on its own", and the answer to that is that a run stays measurable:
// it measures the glyphs it kept. What it took to make true was that the measure
// and the painting go through the same call, and that the group and not the
// neighbour is what is shaped — two runs of one word that shape different
// strings disagree about where a ligature begins, and a character between them
// is drawn by neither.
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
			before = textBetween(items, j, i)
			kerns = kerns && items[j].Face == items[i].Face
		}
		if j, ok := shapingNeighbour(items, i, +1); ok {
			after = textBetween(items, i+1, j+1)
			kerns = kerns && items[j].Face == items[i].Face
		}
		// And the text either side that may contribute *glyphs* and not only
		// forms, which is a third question and the strictest of them. It is the
		// whole of the group rather than the neighbour alone: every run of one
		// has to shape the same string. See mergeGroupAround.
		mergePre, mergePost := l.mergeGroupAround(items, i)
		if before == "" && after == "" {
			continue
		}
		items[i].PreContext, items[i].PostContext = before, after
		items[i].MergePre, items[i].MergePost = mergePre, mergePost
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
	// The last item that draws nothing, which is the answer where there is no
	// run beyond it: a zero width joiner written at the edge of a box is the
	// whole of the context, and its characters are what says which form the
	// letter beside it takes. The suite's shaping-join-002 is a table cell
	// holding "&zwj;&#x0627;&zwj;" and nothing else.
	last, blank := 0, false
	for j := i + step; j >= 0 && j < len(items); j += step {
		switch {
		case items[j].Abs != nil || items[j].Float != nil:
			// Out of flow: written between the two runs and drawn somewhere
			// else entirely, so it stands between nothing.
			continue
		case drawsNothing(items[j]):
			last, blank = j, true
			// A character that sets no paper and takes no room, which between
			// two runs is the same nothing an out-of-flow box is. The soft
			// hyphen is the one a document writes inside a word: SplitAtBreaks
			// keeps it, so it is an item of its own, and treating it as a run
			// gave the run on each side a context consisting of one invisible
			// character and nothing else. Arabic either side of a "&shy;" came
			// out in isolated forms — a joined word broken into letters by a
			// mark that is not drawn at all.
			//
			// The text is *kept* rather than skipped over: textBetween gathers
			// it back, because a shaper reads the ignorable characters as
			// transparent and a context with a hole in it is a different
			// context.
			continue
		case items[j].Inset:
			// An inline box's own margin, border and padding. A zero one is a
			// boundary and nothing more — which is what shaping-004 through
			// -006 are — and one with width is room between the letters, which
			// is what -009 through -011 are.
			if facingInset(items[j], items[i].Level&1 == 1) == 0 {
				continue
			}
			return last, blank
		case !isShapedRun(items[j]):
			return last, blank
		}
		if !sameShaping(items[i], items[j]) {
			return last, blank
		}
		return j, true
	}
	return last, blank
}

// textBetween is the text of the items in [from, to), which is what a run on one
// side of them reads as its context.
//
// It is a range and not one item because shapingNeighbour looks through what
// draws nothing: a soft hyphen between two words is an item, and the word on the
// far side of it needs the word *and* the hyphen, in that order, or the shaper
// is given a context the document does not have.
func textBetween(items []inlineItem, from, to int) string {
	if to-from == 1 {
		return items[from].Text
	}
	var b strings.Builder
	for k := from; k < to; k++ {
		b.WriteString(items[k].Text)
	}
	return b.String()
}

// drawsNothing reports whether an item is text that sets no paper and takes no
// room: every character of it default-ignorable, and no width to stand in.
//
// The width is asked as well as the characters because the two can disagree —
// a face may give an ignorable character an advance, and a character that moves
// the pen is room between the letters however invisible it is. §8.1 breaks
// shaping where there is room between them, which is the rule the insets above
// are read by.
func drawsNothing(item inlineItem) bool {
	if item.Text == "" || item.Width != 0 {
		return false
	}
	for _, r := range item.Text {
		if !isDefaultIgnorable(r) {
			return false
		}
	}
	return true
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
	return f.HasJoiningForms() || f.HasKerning() || f.HasLigatures()
}

// itemShaping is everything about how an item is set that its own text does not
// say, gathered for the measure. See paragraph.Shaping.
func itemShaping(it *inlineItem) shaping {
	return shaping{
		Before: it.PreContext, After: it.PostContext,
		MergeBefore: it.MergePre, MergeAfter: it.MergePost,
		ContextKerns: it.ContextKerns, Upright: it.Upright, Off: it.Off,
	}
}

// mergeGroupAround is the text before and after the run at i that may be shaped
// *with* it, which is every run of the group its boundaries do not break.
//
// The whole group and not the neighbour alone, and that is the correction that
// made this work at all. Shaped a neighbour at a time the runs of one word
// disagree: given "of|f|ice", the first shapes "off" and forms an ff, the last
// shapes "fice" and forms an fi, and the "i" between them belongs to a ligature
// in one reading and to a glyph in the other — so it is drawn by neither and
// falls off the page. One string for the group and each run keeping its own
// slice of the glyphs is the only division that adds up.
func (l *layouter) mergeGroupAround(items []inlineItem, i int) (before, after string) {
	for j := i; ; {
		k, ok := shapingNeighbour(items, j, -1)
		if !ok || !sharesGlyphsWith(items, k, j) {
			break
		}
		before = textBetween(items, k, j) + before
		j = k
	}
	for j := i; ; {
		k, ok := shapingNeighbour(items, j, +1)
		if !ok || !sharesGlyphsWith(items, j, k) {
			break
		}
		after += textBetween(items, j+1, k+1)
		j = k
	}
	return before, after
}

// sharesGlyphsWith reports whether the runs at from and to — which are
// neighbours, with nothing but invisible items between them — may be shaped as
// one string, so that a ligature spanning the boundary is formed.
//
// It is the strictest of the three questions this file asks about a boundary,
// and the other two are why it is asked separately.
//
//   - A *form* crosses almost everything. §8.1's boundary does not break
//     shaping, and the suite's shaping-023 sets the middle Mongolian letter blue
//     and still asks for the three to join.
//   - A *kern pair* crosses a colour but not a font or a size, because a kern is
//     a distance one font states at one size. That is ContextKerns.
//   - A *glyph* crosses neither. One glyph is drawn once, in one colour, on one
//     baseline: a ligature across a change of either would paint half a word in
//     the wrong colour or leave half of it unraised, and the suite writes both —
//     shaping-023 to -025 for the colour and boundary-shaping-002 and -006 for
//     "vertical-align: 1em" and "super".
//
// So this asks for everything sameShaping asks, and then the face and the size —
// which that one leaves open on purpose, because neither decides a form — and
// then that nothing about how the two runs are *painted or placed* differs.
//
// The face is the one of those three with a test of its own already:
// TestAFaceChangeIsNotKernedAcross. A pair positioned across a font change is
// not that font's pair, and a *glyph* across one is not that font's glyph at
// all — a glyph index means nothing outside the font it came from.
func sharesGlyphsWith(items []inlineItem, from, to int) bool {
	a, b := items[from], items[to]
	if !sameShaping(a, b) || a.Size != b.Size || a.Face != b.Face {
		return false
	}
	// Neither side moved by vertical-align, which is the question rather than
	// "both moved alike": VAlignState carries the subtree the box belongs to and
	// compares it by identity, so two runs in different spans differ there even
	// when both sit on the paragraph's own baseline. What a glyph needs is that
	// there is one baseline under it, and an unmoved pair has exactly that.
	if a.Valign.Aligned() || b.Valign.Aligned() {
		return false
	}
	if !samePaint(heldBox(a.Box), heldBox(b.Box)) {
		return false
	}
	// The lines ruled across them, which are drawn from the run and not from the
	// glyph: a run whose characters were swallowed by its neighbour's ligature
	// has no advance left to rule a line over, so an underline that began at the
	// boundary would stop short. shaping-025 is the suite's, and its reference
	// underlines the same three letters set as three runs.
	if !sameDecorations(a.Decorations, b.Decorations) {
		return false
	}
	// A break opportunity between them, which a ligature may not span: the two
	// halves would end up on different lines with the glyph on one of them.
	// Nothing in the suite reaches this — a ligature is inside a word and a word
	// has no opportunity in it — and it is here because the alternative is a
	// rule that holds by coincidence.
	for k := from + 1; k <= to; k++ {
		if items[k].BreakBefore {
			return false
		}
	}
	return true
}

// samePaint reports whether two boxes draw their text the same way.
//
// The computed values as the cascade wrote them, compared as strings: two boxes
// with the same computed value paint alike, and comparing the strings avoids
// resolving a colour and a face for every boundary in the document.
//
// The three are what a glyph carries and cannot carry twice. The colour is the
// obvious one. The style and the weight are here because a face is chosen from
// them, and where the family has no italic or no bold the *same* face is chosen
// and the difference is one a renderer may yet synthesize — so two runs that ask
// for different ones are not one glyph's worth of text however alike their faces
// are today. shaping-024 is that document, with "font-style: italic" on the
// middle letter of three.
func samePaint(a, b *Box) bool {
	if a == nil || b == nil {
		return a == b
	}
	for _, name := range [...]string{"color", "font-style", "font-weight"} {
		if a.Style[name] != b.Style[name] {
			return false
		}
	}
	return true
}

// sameDecorations reports whether two runs carry the same lines, declared by the
// same boxes. A Decoration is compared field for field because Ref is an
// interface and a slice of them is not comparable with ==.
func sameDecorations(a, b []textDecoration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].By != b[i].By {
			return false
		}
	}
	return true
}
