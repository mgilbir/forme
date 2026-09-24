package layout

import (
	"fmt"
	"strings"
	"unicode/utf8"

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
		if isShapedRun(items[i]) && contextCanChange(items[i]) {
			joins = true
			break
		}
	}
	// The groups are found whether or not any face's shaping can change with
	// its context, and that is not the same question.
	//
	// A group is what a run's width is measured *within*: the two ends of the
	// run inside the group's one shaping, each quantized, so that the runs of a
	// group tile it exactly and add up to the group's own width. That is
	// arithmetic and not shaping, and it is wanted for every face. Left to the
	// gate above, a face with no forms, no kerning and no ligatures took none of
	// it — so the same word measured differently depending on how it had been
	// cut into runs, and "letter" written with a zero width space between every
	// pair of letters came out a sixty-fourth of a pixel wider than "letter".
	// See TestOneWordMeasuresTheSameHoweverItIsCutIntoRuns.
	//
	// The neighbours of every run are found in one pass each way, and the text
	// of the paragraph is gathered once so that every context is a slice of it.
	// Both used to be per run: each run walked past every invisible item beside
	// it to find its neighbour, and built its context by concatenating all of
	// them — so a word with sixteen thousand soft hyphens in it, each its own
	// item, was sixteen thousand walks and sixteen thousand strings of up to
	// sixteen thousand characters: 17.7 s and 2.2 GB for 32 KB of markup (audit
	// C22, which 09-06/C15 fixed for the merge groups and not for this).
	//
	// Neither is built for a paragraph with fewer than two items of text, which
	// has no neighbour to find and no group — and most boxes hold one run.
	texts := 0
	for i := range items {
		if items[i].Text != "" {
			if texts++; texts > 1 {
				break
			}
		}
	}
	if texts < 2 {
		return items
	}
	nb := contextNeighbours(items)
	var text runText
	built := false
	textOf := func() runText {
		if !built {
			text, built = newRunText(items), true
		}
		return text
	}
	groups := mergeGroupTexts(items, nb, textOf)
	if !joins && groups.empty() {
		return items
	}
	text = textOf()
	for i := range items {
		if !isShapedRun(items[i]) {
			continue
		}
		before, after := "", ""
		// Whether a pair that spans the boundary is this font's to apply, which
		// is a different question from whether the context reaches the run at
		// all. See Item.ContextKerns.
		kerns := true
		if contextCanChange(items[i]) {
			if n := nb.before[i]; n.ok {
				var lost bool
				before, lost = text.before(n.j, i, nb.blank[n.j])
				l.contextCut(lost)
				kerns = kerns && sameFaceRules(items[n.j], items[i])
			}
			if n := nb.after[i]; n.ok {
				var lost bool
				after, lost = text.after(i, n.j, nb.blank[n.j])
				l.contextCut(lost)
				kerns = kerns && sameFaceRules(items[n.j], items[i])
			}
		}
		// And the text either side that may contribute *glyphs* and not only
		// forms, which is a third question and the strictest of them. It is the
		// whole of the group rather than the neighbour alone: every run of one
		// has to shape the same string. See mergeGroupTexts.
		mergePre, mergePost := groups.around(items, i)
		if before == "" && after == "" && mergePre == "" && mergePost == "" {
			continue
		}
		items[i].PreContext, items[i].PostContext = before, after
		items[i].MergePre, items[i].MergePost = mergePre, mergePost
		// And the group as the one string its runs share, so that measuring
		// each of them finds the group's shaping without building the group's
		// text again. See paragraph.Item.MergeGroup.
		items[i].MergeGroup = groups.whole(i)
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

// contextCut reports, once for the layout, that a run's context was cut short of
// the invisible characters between it and its neighbour. See maxContextRunes.
func (l *layouter) contextCut(lost bool) {
	const key = "shaping context cut"
	if !lost || l.reportedOnce[key] {
		return
	}
	l.reportedOnce[key] = true
	l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
		"more than %d invisible characters stand between two runs of text, and only "+
			"the %d nearest each run are given to the shaper as its context; a letter "+
			"that joins across all of them is shaped as though they ended there",
		maxContextRunes, maxContextRunes))
}

// sameFaceRules reports whether a pair spanning the boundary between two runs is
// this font's pair to apply.
//
// The face is the obvious half: a pair is stated by one font over two of its own
// glyphs, and a glyph index means nothing outside the font it came from. See
// TestAFaceChangeIsNotKernedAcross.
//
// What the two runs turned off or asked for is the other half, and it is the
// same argument. The neighbour's glyphs are found by shaping its text with *this*
// run's rules, so where the two disagree the pair is looked up between a glyph on
// the page and one that is not — a small capital beside a letter set as an
// ordinary one, or the reverse. That is not a pair the font stated about
// anything, and a page is better without it than with a distance measured
// between a glyph and a glyph that was never drawn.
func sameFaceRules(a, b inlineItem) bool {
	return a.Face == b.Face && a.Off == b.Off
}

// isShapedRun reports whether an item is a run of text that a face shapes: not a
// tab, whose advance is a distance rather than a string, and not one of the
// markers that stand for something which is not text at all.
func isShapedRun(item inlineItem) bool {
	return item.Text != "" && item.Face != nil && !item.Tab && !item.Forced &&
		!item.Inset && item.AtomicBox == nil && item.Float == nil && item.Abs == nil
}

// shapingNeighbour finds the run that gives item i its context on one side, or
// says there is none. step is -1 for the text before and +1 for the text after.
//
// It is the question contextNeighbours answers for every item at once, asked
// for one; see there for the rules. It builds the whole table to answer, so it
// is for a caller asking once — the runs of a paragraph are given theirs from
// one table.
func shapingNeighbour(items []inlineItem, i, step int) (int, bool) {
	nb := contextNeighbours(items)
	n := nb.after[i]
	if step < 0 {
		n = nb.before[i]
	}
	return n.j, n.ok
}

// neighbourAt is what the walk for one side of one run found: the run that
// gives it its context, or failing that the invisible item furthest from it
// that the walk passed, with ok saying there is either.
type neighbourAt struct {
	j  int
	ok bool
}

// neighbours is contextNeighbours' answer: both sides of every item, and which
// items draw nothing.
type neighbours struct {
	before, after []neighbourAt
	blank         []bool
}

// contextNeighbours finds, for every run, the run that gives it its context on
// each side — in one pass each way.
//
// The walk from a run passes over the items that are not on the page — an
// inline box's own inset where it has no width, the record of a box that is out
// of flow, and text that draws nothing — and stops at anything else, because
// anything else is either room between the two runs or is not text. What it
// stops at is the neighbour when it is a run shaped by the same rules, and
// otherwise there is none.
//
// Where there is none, the answer is the invisible item furthest along that it
// passed, if it passed one: a zero width joiner written at the edge of a box is
// the whole of the context, and its characters are what says which form the
// letter beside it takes. The suite's shaping-join-002 is a table cell holding
// "&zwj;&#x0627;&zwj;" and nothing else.
//
// Text that draws nothing is passed and *kept* — the context gathers it back —
// because a shaper reads the ignorable characters as transparent and a context
// with a hole in it is a different context. The soft hyphen is the one a
// document writes inside a word: SplitAtBreaks keeps it, so it is an item of its
// own, and treating it as a run gave the run on each side a context of one
// invisible character and nothing else. Arabic either side of a "&shy;" came
// out in isolated forms.
//
// An inset is passed when it is zero and stops the walk when it is not. That is
// shaping-004 through -006, a zero margin, border and padding, against -009
// through -011, where there is room between the letters. Which side of the box
// faces the boundary depends on the direction of the run asking — see
// facingInset — so there are two tables, one for each parity of its level.
//
// Room an item declares is the same rule from the item's side: §8.1 breaks
// shaping where there is room between the characters, and an inside list
// marker's half-em is that room. A run with room after it has no neighbour
// after it, and a run with room is no neighbour before another. No document
// reaches the second — the only thing that spends room is an inside marker,
// which leads its line — and it stays because the first would otherwise be a
// rule about markers rather than about room.
//
// # Why it is a table
//
// Asked one run at a time, the walk passed every invisible item between a run
// and its neighbour, and a run of invisible items is itself runs: each soft
// hyphen of a word written with ten thousand of them walked past all the others.
// Everything the walk decides before it reaches the item it stops at is about
// the items, not about the run asking, except which side of an inset faces the
// run — so for each direction and each parity one pass carries the last item the
// walk would stop at and the invisible item nearest it, and each run then asks
// only about that item.
func contextNeighbours(items []inlineItem) neighbours {
	n := len(items)
	nb := neighbours{
		before: make([]neighbourAt, n),
		after:  make([]neighbourAt, n),
		blank:  make([]bool, n),
	}
	outOfFlow := func(k int) bool { return items[k].Abs != nil || items[k].Float != nil }
	// asks says an item is a run, which is what has neighbours; the rest are
	// only walked past or stopped at.
	asks := make([]bool, n)
	for k := range items {
		nb.blank[k] = !outOfFlow(k) && drawsNothing(items[k])
		asks[k] = isShapedRun(items[k])
	}
	// passes says the walk goes on past item k, for a run of direction rtl.
	passes := func(k int, rtl bool) bool {
		return outOfFlow(k) || nb.blank[k] ||
			(items[k].Inset && facingInset(items[k], rtl) == 0)
	}
	// stopAt is the walk's answer once it has reached candidate c, with blank
	// the invisible item it passed nearest c, or -1 for none.
	stopAt := func(i, c, blank int, back bool) neighbourAt {
		passed := neighbourAt{j: max(blank, 0), ok: blank >= 0}
		switch {
		case c < 0, items[c].Inset, !isShapedRun(items[c]),
			!sameShaping(items[i], items[c]):
			return passed
		case back && items[c].Room != 0:
			return passed
		}
		return neighbourAt{j: c, ok: true}
	}
	for parity := 0; parity < 2; parity++ {
		rtl := parity == 1
		c, blank := -1, -1
		for i := 0; i < n; i++ {
			if asks[i] && int(items[i].Level&1) == parity {
				nb.before[i] = stopAt(i, c, blank, true)
			}
			switch {
			case !passes(i, rtl):
				c, blank = i, -1
			case nb.blank[i] && blank < 0:
				blank = i
			}
		}
		c, blank = -1, -1
		for i := n - 1; i >= 0; i-- {
			if asks[i] && int(items[i].Level&1) == parity {
				if items[i].Room != 0 {
					nb.after[i] = neighbourAt{}
				} else {
					nb.after[i] = stopAt(i, c, blank, false)
				}
			}
			switch {
			case !passes(i, rtl):
				c, blank = i, -1
			case nb.blank[i] && blank < 0:
				blank = i
			}
		}
	}
	return nb
}

// textBetween is the text of the items in [from, to), which is what a run on one
// side of them reads as its context.
//
// It is a range and not one item because the neighbour walk looks through what
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

// maxContextRunes is how much of the text either side of a run is handed to the
// shaper as context: the characters nearest the boundary, up to this many.
//
// The shaper cannot use more. A contextual rule matches at most sixty-four
// glyphs in this engine — see shape's maxContextLength, which is HarfBuzz's —
// and the joining scan stops at the first character that is not transparent,
// which in any written word is the next letter. HarfBuzz itself keeps five
// characters of context either side of a buffer.
//
// What the bound does cut is a run of more than this many invisible characters
// between two letters, which the joining scan would have walked through. Such a
// context is cut and the document is told — see layouter.contextCut.
const maxContextRunes = 64

// runText is the text of every item of a paragraph, gathered once, so that a
// run's context is a slice of it rather than a string of its own.
type runText struct {
	all string
	// at is where each item's text begins in all, with one more entry for the
	// end of the last.
	at []int
}

func newRunText(items []inlineItem) runText {
	t := runText{at: make([]int, len(items)+1)}
	var b strings.Builder
	for k, it := range items {
		t.at[k] = b.Len()
		b.WriteString(it.Text)
	}
	t.at[len(items)] = b.Len()
	t.all = b.String()
	return t
}

// span is the text of the items in [from, to).
func (t runText) span(from, to int) string { return t.all[t.at[from]:t.at[to]] }

// before is the context before the run at i, whose neighbour is j: the text of
// [j, i), cut to the maxContextRunes nearest i. lost says the cut took more than
// the head of the neighbour's own text — some of the invisible characters
// between, which is a context the shaper is not given. blank says j is itself
// one of those, the walk having found no neighbour past them.
func (t runText) before(j, i int, blank bool) (string, bool) {
	s := t.span(j, i)
	cut := len(s)
	for k := 0; k < maxContextRunes && cut > 0; k++ {
		_, size := utf8.DecodeLastRuneInString(s[:cut])
		cut -= size
	}
	if cut == 0 {
		return s, false
	}
	keep := t.at[j+1]
	if blank {
		keep = t.at[j]
	}
	return s[cut:], t.at[j]+cut > keep
}

// after is the context after the run at i, whose neighbour is j: the text of
// (i, j], cut to the maxContextRunes nearest i. lost and blank are before's.
func (t runText) after(i, j int, blank bool) (string, bool) {
	s := t.span(i+1, j+1)
	end := 0
	for k := 0; k < maxContextRunes && end < len(s); k++ {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}
	if end == len(s) {
		return s, false
	}
	keep := t.at[j]
	if blank {
		keep = t.at[j+1]
	}
	return s[:end], t.at[i+1]+end < keep
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
	// So the size breaks the boundary for a run that kerns and not for one
	// that joins. A run that does both is read as joining, because that is the
	// difference a reader sees: a letter in the wrong form is a different
	// letter, and a pair off by a fraction of an em is a gap.
	//
	// The run and not the face: whether a letter's form follows its
	// neighbours is a question about the rules its script selects. Asked of
	// the face, a Latin word in a font that also sets Arabic was read as
	// joining, and a kern pair was positioned across a change of size between
	// two of its runs — the number that belongs to neither.
	return a.Size == b.Size || a.Face.FormsFollowNeighbours(a.Text, a.Off)
}

// contextCanChange reports whether the text either side of a run can change what
// the run is: which glyphs it is set in, or where they sit.
//
// It is the run's question and the face answers it for the run: from the rules
// the run's script and language select, and for everything this package reads
// from a context — the forms a cursive or Indic run takes, the script a run of
// digits or punctuation is set in, and the pair kerned across the edge. It was
// asked of the face, as whether the font had forms, kerning or 'liga' under any
// script at all, so a font whose rules for the run's script were somewhere else
// was answered for a script the run is not in, and a span of punctuation
// between two Chinese words was shaped as though it had no neighbours. See
// shape.Face.ContextCanChange.
func contextCanChange(it inlineItem) bool {
	return it.Face.ContextCanChange(it.Text, it.Off)
}

// itemShaping is everything about how an item is set that its own text does not
// say, gathered for the measure. See paragraph.Shaping.
func itemShaping(it *inlineItem) shaping {
	return shaping{
		Before: it.PreContext, After: it.PostContext,
		MergeBefore: it.MergePre, MergeAfter: it.MergePost, MergeGroup: it.MergeGroup,
		ContextKerns: it.ContextKerns, Upright: it.Upright, Off: it.Off,
	}
}

// The merge group: the text before and after a run that may be shaped *with*
// it, which is every run of the group its boundaries do not break.
//
// The whole group and not the neighbour alone, and that is the correction that
// made this work at all. Shaped a neighbour at a time the runs of one word
// disagree: given "of|f|ice", the first shapes "off" and forms an ff, the last
// shapes "fice" and forms an fi, and the "i" between them belongs to a ligature
// in one reading and to a glyph in the other — so it is drawn by neither and
// falls off the page. One string for the group and each run keeping its own
// slice of the glyphs is the only division that adds up.
type mergeGroups struct {
	// text is the group's whole text, one entry per item, shared by every run
	// of the group; at is where that item's own text begins in it.
	//
	// Both are nil until a group of more than one run is found, so a box whose
	// text is one run — which is most boxes — pays one walk and no allocation.
	text []string
	at   []int
}

// empty reports whether no run shapes with a neighbour, so there is no group.
func (g mergeGroups) empty() bool { return g.text == nil }

// around is the text before and after the run at i that is shaped with it.
//
// Both are slices of the group's one string rather than strings of their own,
// so a group of a thousand runs costs one copy of its text and not a thousand.
func (g mergeGroups) around(items []inlineItem, i int) (before, after string) {
	if i >= len(g.text) || g.text[i] == "" {
		return "", ""
	}
	return g.text[i][:g.at[i]], g.text[i][g.at[i]+len(items[i].Text):]
}

// whole is the text of the group the run at i belongs to, the one string every
// run of it shares, or empty where the run shapes with nobody.
func (g mergeGroups) whole(i int) string {
	if i >= len(g.text) {
		return ""
	}
	return g.text[i]
}

// mergeGroupTexts finds every merge group and the text its runs are shaped
// with, in one walk.
//
// It used to be a walk per run: each run gathered its whole group from itself
// outwards, building the text again from scratch and concatenating it a
// neighbour at a time. That is quadratic in the runs of a group and worse in
// their bytes — four thousand adjacent spans took twenty-nine seconds and
// about 1.6 GB of strings, and four thousand adjacent spans is one line of
// generated markup. Separated by spaces the same four thousand took a quarter
// of a second, because a space ends the group.
//
// A group is a contiguous range of items: the runs that shape together, and
// everything written between them, which textBetween gathers because a shaper
// reads an ignorable character as transparent and a context with a hole in it
// is a different context.
func mergeGroupTexts(items []inlineItem, nb neighbours, text func() runText) mergeGroups {
	var g mergeGroups
	// How many break opportunities come before each item, so that whether
	// there is one between two runs is a subtraction. It was a walk over the
	// items between them, asked of every run — and the items between two runs
	// can be every soft hyphen of a word.
	var breaks []int
	// Which translucent inline box each run is inside, asked of every
	// boundary between two runs and answered once per box. See translucency.
	tr := translucency{}
	for i := 0; i < len(items); {
		if !isShapedRun(items[i]) {
			i++
			continue
		}
		if breaks == nil {
			breaks = make([]int, len(items)+1)
			for k, it := range items {
				breaks[k+1] = breaks[k]
				if it.BreakBefore {
					breaks[k+1]++
				}
			}
		}
		// How far the group reaches: the last run that shares glyphs with the
		// one before it.
		last := i
		for {
			n := nb.after[last]
			if !n.ok || !sharesGlyphsWith(items, last, n.j, breaks, tr) {
				break
			}
			last = n.j
		}
		if last == i {
			// A run that shapes with nobody. Its group is itself, and the two
			// contexts are empty — which is what the empty string here says,
			// without building anything.
			i++
			continue
		}
		if g.text == nil {
			g.text, g.at = make([]string, len(items)), make([]int, len(items))
		}
		group := text().span(i, last+1)
		at := 0
		for k := i; k <= last; k++ {
			g.text[k], g.at[k] = group, at
			at += len(items[k].Text)
		}
		i = last + 1
	}
	return g
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
func sharesGlyphsWith(items []inlineItem, from, to int, breaks []int, tr translucency) bool {
	a, b := items[from], items[to]
	if !sameShaping(a, b) || a.Size != b.Size || a.Face != b.Face {
		return false
	}
	// And the font's own rules the two runs turned off or asked for. One
	// shaping of one string applies one set of them, so two runs that disagree
	// cannot be that shaping — whichever set were used, one of the runs would
	// be cut out of a string that was not shaped the way it is drawn.
	//
	// It is visible with small capitals, which are the first of these that adds
	// a glyph rather than suppressing one. "of<span>f</span>ice" with the span
	// in small capitals is one group: the plain runs shape "office" and form the
	// ffi ligature, the span shapes it with 'smcp' and forms none, and the page
	// gets the ligature *and* a small capital F — the letter drawn twice, in the
	// middle of a word.
	//
	// The three suppressing flags have the same fault and no such symptom, since
	// the two shapings of the group agree everywhere the ligature is not.
	if a.Off != b.Off {
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
	// Nor moved by a relative offset, which the painter adds to each run's
	// pen position: a glyph owned by one run is drawn at that run's offset,
	// so a ligature across "position: relative; top: 5px" left half of it
	// where it was and moved the other half with the span.
	//
	// Which way a run's glyphs stand is the one other thing drawn per run,
	// and it cannot differ here: it is decided by the block the page was
	// turned at (see uprightText), so every run of one paragraph has it.
	if a.Offset != b.Offset {
		return false
	}
	if !samePaint(heldBox(a.Box), heldBox(b.Box), tr) {
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
	//
	// breaks counts the opportunities before each item; see mergeGroupTexts.
	return breaks[to+1] == breaks[from+1]
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
//
// And the two things the painter decides per run and not per glyph: whether
// the run is drawn at all, and how translucent it is. A glyph is drawn by the
// run that owns it, so a ligature across a "visibility: hidden" boundary took
// its letters with it — "o<span hidden>f</span>fice" formed the ffi in the
// hidden run and the visible "fice" drew "ce" — or brought hidden ones along:
// "of<span hidden>f</span>ice" drew the hidden f inside the visible run's
// ligature (audit C100). Opacity is the same case, now that a translucent
// inline box dims its own runs: which runs it dims is decided by the inline
// box that is innermost among the translucent ones around each run, and two
// runs with different ones are two alphas. Every property the painter reads
// per run belongs here or in sharesGlyphsWith, which is the rule this list is
// kept by.
func samePaint(a, b *Box, tr translucency) bool {
	if a == nil || b == nil {
		return a == b
	}
	for _, name := range [...]string{"color", "font-style", "font-weight"} {
		if a.Style.Get(name) != b.Style.Get(name) {
			return false
		}
	}
	return isHidden(a) == isHidden(b) && tr.inline(a) == tr.inline(b)
}

// translucency is, for each box of one paragraph, the innermost non-atomic
// inline box around it, itself included, that asks for an opacity below one, or
// nil. It is the walk the painter's inlineDim makes, and stops where that one
// does: at the block, or at an atomic inline, whose opacity is the fragment's
// and not the run's.
//
// It is a memo because every boundary between two runs asks it of both, and the
// walk is up every inline box around the run, parsing each one's opacity: a
// paragraph of spans nested d deep with a word in each paid d walks of up to d
// boxes. The painter has the same walk in inlineDim and memoizes it for the
// same reason. The answer from a box is the answer from every box the walk from
// it passes, so the walk stops at the first box already answered and fills in
// the ones it passed.
type translucency map[*Box]*Box

// inline is the translucent inline box around b.
func (t translucency) inline(b *Box) *Box {
	var path []*Box
	var found *Box
	for cur := b; cur != nil; cur = cur.Parent {
		if got, ok := t[cur]; ok {
			found = got
			break
		}
		path = append(path, cur)
		if next, done := translucentStep(cur); done {
			found = next
			break
		}
	}
	for _, c := range path {
		t[c] = found
	}
	return found
}

// translucentStep is one box of translucency's walk: whether the walk ends
// at cur, and with what.
func translucentStep(cur *Box) (found *Box, done bool) {
	switch {
	case cur.Outer != OuterInline, cur.Replaced != nil, isAtomicInline(cur):
		return nil, true
	case cur.IsText():
		return nil, false
	case groupsItsPaint(cur):
		return cur, true
	}
	return nil, false
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
