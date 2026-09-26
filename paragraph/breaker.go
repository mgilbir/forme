package paragraph

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
	"sort"

	"github.com/mgilbir/forme/segment"
)

// Breaker is the half of inline layout that is about text rather than about
// boxes.
//
// Everything it does — cutting a run of items into lines, measuring what will
// fit, scoring a set of breaks for text-wrap: balance — is stated over runs of
// characters, the widths a face gives them and the rules in CSS Text and
// Unicode. None of it needs a box tree, a cascade or a document, and the two
// fields below are the whole of what it needs from outside itself.
//
// A Breaker is not safe for concurrent use, and does not need to be. It owns a
// memo of measured runs, which is a plain map: two goroutines breaking through
// one would be a data race, and Go's is fatal rather than merely undefined. The
// shape that is supported is one per run — which is what the layout engine does,
// making its own in Layout, so two documents laid out at once share nothing. See
// race_test.go, which holds both halves of that to the race detector.
//
// It is a receiver of its own rather than more methods on the layouter because
// what a type can reach decides what its code can come to depend on. As methods
// on the layouter these functions could read a computed style, walk to a parent
// or lay a child out, and the only thing keeping them from it was that nobody
// had yet — which is not a property, it is a habit. Here the compiler holds the
// line.
type Breaker struct {
	// run is what has been learned about the run being cut, which is the run
	// the next cut will ask about too. See runIndex.
	run *runIndex

	// measured memoizes the width of a run as it will be set.
	//
	// Measuring is the inner loop of line breaking and the same words recur
	// constantly in a document: every "the" on a page measures the same, and the
	// balancer measures a whole paragraph once per candidate width. The key is
	// everything that scales or shifts the answer — see measureKey.
	measured map[measureKey]style.Unit
	// grouped memoizes the glyphs of a whole merge group, which every run of
	// that group needs and which every run of it used to shape for itself. See
	// mergedSpan.
	grouped map[groupKey][]float64
	// lastGroup and lastAdvances are the entry of grouped asked for last,
	// which is the one a word broken across lines asks for at every cut. A
	// lookup in grouped hashes the key, and the key holds the whole text of
	// the group — so each cut of a long word read the whole word to find a
	// table it already had. Compared instead, the key is two equal pointers.
	lastGroup    groupKey
	lastAdvances []float64
	// bounds memoizes where a divided run may be cut, so that a word broken
	// across many lines has its clusters found once. See Breaker.clustersOf.
	bounds map[string][]int
	// report is where a run that would not fit is said to have overflowed.
	//
	// It is an interface because the finding wants to name the element the run
	// came from, and an element is exactly what this half has been kept from
	// knowing about. The breaking says what happened; the layer that built the
	// items says where.
	report OverflowReporter
}

// OverflowReporter is told that a run of text was wider than the room it had.
//
// §11.1.1 leaves what to do about overflowing content to the formatter, and this
// engine's answer is to lay it out and record a finding rather than to clip it
// silently or to widen the box. The finding needs the element, which is why this
// is a call back out rather than something the breaking does itself.
type OverflowReporter interface {
	ReportOverflow(item Item, width style.Unit)
}

// NewBreaker is the breaker a layout run uses, reporting through the layouter
// that made it.
//
// A nil reporter is allowed and means the findings are dropped. That is not
// defensiveness: a caller that only wants to measure a run, or to ask where a
// paragraph would break, has nowhere to put a finding and no document to name in
// one — and requiring a reporter it would never hear from would make the
// measuring API about the reporting. The breaking itself is unchanged either
// way; §11.1.1's overflow is recorded where a caller asked to hear about it and
// nowhere else.
func NewBreaker(r OverflowReporter) *Breaker {
	if r == nil {
		r = discardFindings{}
	}
	return &Breaker{measured: map[measureKey]style.Unit{}, report: r}
}

// discardFindings is the reporter a breaker made without one uses, so that the
// breaking has one call to make rather than a nil check at every place it might
// report.
type discardFindings struct{}

func (discardFindings) ReportOverflow(Item, style.Unit) {}

// Measure returns the advance width of a string in a face, memoized.
//
// It is the face's own advance and nothing else, which is what the three callers
// that use it want: a tab stop is a multiple of the space advance, "ch" is the
// advance of a zero, and a list marker is set without the text's spacing. Text
// that is laid out on a line goes through measureSpaced instead.
func (br *Breaker) Measure(face *shape.Face, text string, size style.Unit) style.Unit {
	return br.MeasureSpaced(face, text, size, TextSpacing{})
}

// MeasureSpaced is the advance of a run as it will be set, with letter-spacing
// and word-spacing in it.
//
// Measuring is the inner loop of line breaking, and the same words recur
// constantly in a document — every "the" in a page measures the same. The key
// includes the face and the size because both scale the answer, and the spacing
// because it changes it: two boxes at the same size in the same face with
// different letter-spacing must not share an entry. Leaving it out of the key is
// the same memoization bug lengthKey.zeroAdvance records for the "ch" unit, and
// it produces a wrong page only in a document that uses two values.
func (br *Breaker) MeasureSpaced(face *shape.Face, text string, size style.Unit,
	sp TextSpacing) style.Unit {

	return br.MeasureSpacedInContext(face, text, size, sp, Shaping{ContextKerns: true})
}

// MeasureSpacedInContext is MeasureSpaced with the text either side of the run,
// which a cursive script needs: a letter's advance depends on the form it takes
// and the form depends on its neighbours.
//
// The context is part of the memo key for the same reason the spacing is. The
// same three letters measure differently at the start of a word and in the
// middle of one, and an entry shared between the two would give a line filled to
// one width and painted at another.
func (br *Breaker) MeasureSpacedInContext(face *shape.Face, text string, size style.Unit,
	sp TextSpacing, how Shaping) style.Unit {

	if text == "" {
		return 0
	}
	// A run of a merge group is not memoised here. Its key would hold the
	// group's text on either side of it, so finding the entry hashed the whole
	// group once per run of it — the quadratic mergedSpan's own memo exists to
	// take out — and the answer is a subtraction from the group's shaping,
	// which that memo already holds. See Item.MergeGroup.
	merged := how.MergeBefore != "" || how.MergeAfter != ""
	key := measureKey{face: face, text: text, size: size, spacing: sp, how: how}
	if !merged {
		if got, ok := br.measured[key]; ok {
			return got
		}
	}
	// MeasureShaped returns the advance in the units the size was given in, so a
	// size in CSS pixels gives an advance in CSS pixels.
	//
	// Shaped, and not the cheaper per-rune sum beside it. What a line breaker
	// measures has to be what the page draws: it measures a word to decide
	// whether it fits and the backend then shapes the same word to draw it, and
	// if the two disagree the line is filled to one width and painted at
	// another, with nothing in either call's output to show it. A ligature, a
	// kern pair and a contextual substitution are all invisible to the sum and
	// all change the advance.
	//
	// It is affordable here and nowhere else because of the memo above: the same
	// words recur constantly in a document, so the shaping happens once per
	// distinct word rather than once per measurement. A face whose codes are
	// characters — the standard PDF fonts — substitutes and kerns nothing, and
	// MeasureShaped hands those straight back to the sum.
	var w style.Unit
	switch {
	case how.Upright && face.StatesVerticalMetrics():
		// A run set upright on a line of vertical text advances by its glyphs'
		// vertical advances — the face's 'vmtx', as shaping with
		// Features.Vertical reports them in Glyph.YAdvance — and its horizontal
		// advances say nothing about it. A backend drawing the run steps its pen
		// by those same advances, so the width the line is filled to is the one
		// the glyphs take. The two ends are rounded apart, as a merge group's
		// are; see uprightSpan.
		head, through := uprightSpan(br.advances(uprightKey(face, text, how.Before,
			how.After, how.Off)), 0, len(text), size)
		w = through.Sub(head)
	case how.Upright:
		// A face that states no vertical metrics: CSS Writing Modes §4.4 has
		// the UA synthesize them, and the synthesis is the em box — one em per
		// character. Shaping's own synthesis for such a face, the height of its
		// line (HarfBuzz's), is not CSS's. See UprightUnits for what counts as
		// a character here, and shape.Face.StatesVerticalMetrics.
		w = size.Mul(float64(UprightUnits(text)))
	case face == nil:
		// An item with text and no face. What a glyph advances is the face's to
		// say and there is none, so the answer is zero — which is a caller's
		// mistake shown on the page rather than a nil dereference in a process
		// that was doing something else. Every item this package builds for
		// itself carries a face; this is the contract for one it is handed.
	default:
		// The two ends of the run within the group it was shaped with, each
		// rounded to a layout unit, rather than the difference rounded once.
		// Every run of a group then begins where the one before it ended and
		// the widths add up to the group's own rounded width — where rounding
		// the differences leaves a word written in three runs a sixty-fourth of
		// a pixel from the same word written in one. See
		// shape.MeasureShapedMergedSpan.
		head, through := br.mergedSpan(face, text, size.Px(), how)
		lo, _ := style.FromPx(head)
		hi, _ := style.FromPx(through)
		w = hi.Sub(lo)
	}
	w = w.Add(SpacingAdvance(text, sp))
	if !merged {
		br.measured[key] = w
	}
	return w
}

// MeasurePx is the advance of a run in CSS pixels, before it is quantized to a
// layout unit.
//
// It is for the units a face measures — "ch" and "ic" — whose value the
// stylesheet then *multiplies*: "width: 16ch" is sixteen of this number.
// Quantizing first and multiplying after is sixteen truncated sixty-fourths,
// which is up to fifteen of them narrower than the one truncation of the
// product — so the box holds fifteen characters where the author asked for
// sixteen and the sixteenth wraps. Multiplying first and quantizing once gives
// the box exactly the width the same text measures, because the text reaches
// the same arithmetic.
//
// Every other caller wants Measure, which is this quantized: a length that ends
// up on the page has to be a layout unit, and the memo behind Measure is what
// makes line breaking affordable.
func (br *Breaker) MeasurePx(face *shape.Face, text string, size style.Unit) float64 {
	if face == nil || text == "" {
		return 0
	}
	head, through := br.mergedSpan(face, text, size.Px(), Shaping{ContextKerns: true})
	return through - head
}

// mergedSpan is where a run sits within the group it is shaped with.
//
// The group is shaped once and kept. Every run of a group asks for the same
// string — that is what a group is — and each used to shape it for itself, so a
// paragraph written as a thousand adjacent spans shaped a thousand characters a
// thousand times over, and held the glyphs of each. Two thousand of them
// allocated 1.2 GB and took nearly three seconds; the same document is now the
// work of shaping it once.
//
// The context is the group's rather than the run's, because a side the group
// already holds supplies its own — so the runs in the middle of a group share
// one entry and only the two at its ends bring anything of their own. See
// shape.GroupContext.
func (br *Breaker) mergedSpan(face *shape.Face, text string, size float64,
	how Shaping) (head, through float64) {

	if how.MergeBefore == "" && how.MergeAfter == "" {
		// Not a group: the run is shaped on its own, in its own context, and
		// there is nothing to share.
		return face.MeasureShapedMergedSpan(text, size, how.Before, how.After,
			"", "", how.ContextKerns, how.Off)
	}
	before, after := shape.GroupContext(how.Before, how.After, how.MergeBefore, how.MergeAfter)
	key := groupKey{
		face: face, whole: mergedText(how.MergeBefore, text, how.MergeAfter, how.MergeGroup),
		before: before, after: after, kerns: how.ContextKerns, off: how.Off,
	}
	return shape.GroupSpan(br.advances(key), len(how.MergeBefore),
		len(how.MergeBefore)+len(text), size)
}

// spanWidth is the width of one stretch of an item, taken from a single shaping
// of the whole of it.
//
// It is what makes a word broken across many lines linear work. Measured
// piecewise, the rest of the word is shaped again at every line it is cut at —
// twenty thousand characters in two-hundred-pixel lines shaped eleven million
// characters, twelve seconds and nine gigabytes for one long word. Shaped once,
// each line's question is a sum over the glyphs it covers.
//
// The item's own text is the string, so the stretch's context is the rest of
// the item and comes from the text itself, exactly as it does for a merge
// group; the outer context is the item's own. piece is the item the width is
// for, and is what the fallback measures when the shaping cannot be shared — a
// run that is part of a merge group, whose string is not its own, and one with
// no face.
//
// A run set upright in a face that states vertical metrics is shaped upright
// once, and each stretch is a sum over its glyphs' vertical advances, as a
// horizontal run's is over its horizontal ones. In a face that states none its
// advance is a count of its characters, an em each (CSS Writing Modes §4.4),
// and where the run is long enough to have a table the count is taken from it
// like the spacing is, so that the rest of an upright word is not read again
// at every line either. See UprightUnits.
func (br *Breaker) spanWidth(item Item, from, to int, piece Item) style.Unit {
	if item.Face.StatesVerticalMetrics() && item.Upright && piece.Text != "" {
		whole, base, before, after := item.uprightRun()
		head, through := uprightSpan(br.advances(uprightKey(item.Face, whole, before,
			after, item.Off)), base+from, base+to, item.Size)
		return through.Sub(head).Add(br.spacingIn(item, from, to, piece.Text))
	}
	if item.Face != nil && item.Upright && piece.Text != "" {
		if n, ok := br.uprightIn(item, from, to, piece.Text); ok {
			return item.Size.Mul(float64(n)).Add(br.spacingIn(item, from, to, piece.Text))
		}
	}
	if item.Face == nil || item.Upright {
		return br.MeasureSpacedInContext(item.Face, piece.Text, item.Size, item.Spacing,
			piece.shaping())
	}
	whole, base, before, after, kerns := item.group()
	key := groupKey{
		face: item.Face, whole: whole, before: before, after: after,
		kerns: kerns, off: item.Off,
	}
	// The two ends rounded separately, so that the pieces of one item add up to
	// the item's own rounded width. See shape.GroupSpan.
	head, through := shape.GroupSpan(br.advances(key), base+from, base+to, item.Size.Px())
	lo, _ := style.FromPx(head)
	hi, _ := style.FromPx(through)
	return hi.Sub(lo).Add(br.spacingIn(item, from, to, piece.Text))
}

// tabulateFrom is the length below which a run is counted rather than
// tabulated. Reading a short run costs less than building a table for it, and
// the run the breaker holds is then left for the long one it is cutting.
const tabulateFrom = 256

// cutRun is the run an item is a stretch of and where the item begins in it:
// the run it was cut from, or its own text where it has not been cut.
func (it Item) cutRun() (run string, base int) {
	if it.Cut != nil {
		return it.Cut.Text, it.CutAt
	}
	return it.Text, 0
}

// stretchOf is where text[from:to] of an item lies in the run it is a stretch
// of, and whether the run's tables can be asked about it at all: the run has to
// be long enough to have them, and the stretch has to be inside the run and be
// the piece's own length. Anything else is counted, which is what every count
// here did before there were tables.
func (it Item) stretchOf(from, to int, text string) (run string, lo, hi int, ok bool) {
	run, base := it.cutRun()
	lo, hi = base+from, base+to
	if len(run) < tabulateFrom || lo < 0 || hi < lo || hi > len(run) || hi-lo != len(text) {
		return "", 0, 0, false
	}
	if strictSpans && run[lo:hi] != text {
		panic("stretchOf: the run at the stretch is not the piece's own text")
	}
	return run, lo, hi, true
}

// spacingIn is SpacingAdvance for a stretch of an item the breaker is cutting,
// answered from tables built once for the run instead of by reading the stretch
// again at every candidate. See runIndex.
func (br *Breaker) spacingIn(item Item, from, to int, text string) style.Unit {
	sp := item.Spacing
	if sp.Letter == 0 && sp.Word == 0 {
		return 0
	}
	run, lo, hi, ok := item.stretchOf(from, to, text)
	if !ok {
		return SpacingAdvance(text, sp)
	}
	r := br.runOf(run)
	var out style.Unit
	if sp.Letter != 0 {
		out = out.Add(sp.Letter.Mul(float64(br.spacedUnitsIn(r, lo, hi, text))))
	}
	if sp.Word != 0 {
		out = out.Add(sp.Word.Mul(float64(r.wordSeparators(lo, hi))))
	}
	return out
}

// spacedUnitsIn is SpacedUnits(text), where text is run[lo:hi].
func (br *Breaker) spacedUnitsIn(r *runIndex, lo, hi int, text string) int {
	if idx := br.unitsOf(r); idx != nil && idx.covers(lo, hi) {
		return idx.spacedUnits(lo, hi)
	}
	return SpacedUnits(text)
}

// uprightIn is UprightUnits for a stretch of an item, from the run's table.
func (br *Breaker) uprightIn(item Item, from, to int, text string) (int, bool) {
	run, lo, hi, ok := item.stretchOf(from, to, text)
	if !ok {
		return 0, false
	}
	if idx := br.unitsOf(br.runOf(run)); idx != nil && idx.covers(lo, hi) {
		return idx.uprightUnits(lo, hi), true
	}
	return 0, false
}

// trailingSpacing is TrailingSpacing for an item the breaker may be cutting.
//
// The question it ends in — does §8.2 take the spacing off this run entirely —
// is a count of the run's units, and for a stretch of a run of a cursive script
// that count reads the whole stretch: its first character is cursive, so nothing
// short of the end can say that no unit follows. The fill asks it of the rest of
// a word at every candidate on every line, so it is answered from the run's
// table where the item is a stretch of one.
func (br *Breaker) trailingSpacing(item Item) style.Unit {
	return trailingSpacingCounted(item, func() int {
		if run, lo, hi, ok := item.stretchOf(0, len(item.Text), item.Text); ok {
			return br.spacedUnitsIn(br.runOf(run), lo, hi, item.Text)
		}
		return SpacedUnits(item.Text)
	})
}

// spanPx is an item's own text measured under one shaping, in CSS pixels and
// before any quantization — so that two of them can be subtracted without each
// having been rounded first.
func (br *Breaker) spanPx(item Item, how Shaping) (width float64, ok bool) {
	head, through := br.mergedSpan(item.Face, item.Text, item.Size.Px(), how)
	return through - head, true
}

// LineEndCorrection is what to add to an item's width because it ends a line.
//
// A pair adjusts the *left* glyph's advance, so a face that kerns shrinks an
// item's last glyph against the first character of whatever follows it. That is
// right while the two are next to each other and wrong once a line break has
// come between them: they are not adjacent, and there is nothing on this line
// for the shortened advance to make room for. So the amount is given back.
//
// It is the difference between two measurements of the same item rather than a
// measurement on its own, so that everything else the width carries — §8.1's
// gap, the letter-spacing the boundary rule exchanged — survives being
// corrected. Both are taken over the group the item is shaped in, which is what
// a measurement of the item alone cannot do: an item that is a stretch of a
// longer run has a string that is not its own, and reconstructing one from its
// text and its merge neighbours gives a string the run never had. That is how
// this came to do nothing at all for a word broken inside by overflow-wrap —
// "A" with "ATAR" after it rebuilt the group as "AATAR", which holds no "V", so
// both halves of the subtraction measured the same thing and the correction
// came out zero.
//
// Everything *before* the item stays in both. A ligature the group forms across
// the boundary in front of it is still formed and the context that chose its
// glyphs still chooses them; only the far side goes.
func (br *Breaker) LineEndCorrection(item Item) style.Unit {
	if item.Face == nil || item.Upright || item.Text == "" {
		return 0
	}
	whole, base, before, after, _ := item.group()
	end := base + len(item.Text)
	if end > len(whole) || base > end {
		// The item is not the stretch of its own group its fields describe,
		// which is a caller's mistake rather than a document's. Correcting a
		// width from a string it did not come from is worse than not correcting
		// it, so nothing is done.
		return 0
	}
	// Windows cut from the group's own string, and *bounded* ones. Measuring
	// the group truncated at the item's end is the obvious way to ask this and
	// is quadratic: the truncation is a different string for every line the
	// word is broken across, so the shaping memo never answers twice and each
	// line end shapes another prefix of the word. One long word went from 109ms
	// to 331ms at sixteen thousand characters before this was measured, which
	// is the fault SplitHead's note describes being fixed once already.
	//
	// The two measurements differ only in what follows, so everything about the
	// leading edge — the context that chose the glyphs, a ligature the group
	// forms in front of the item — is the same in both and cancels. What is
	// left is the boundary at the far end, which is the whole of what is being
	// taken off.
	// The item's own flag and not the cut's. A cut boundary is one the font
	// states its pairs over whatever the outer context was — splitItemAt sets
	// this to true for exactly that reason — and the cut remembers what the
	// *original* item carried, which for a word with no neighbours is false.
	// Measuring with the rest of the word as context rather than as glyphs puts
	// the pair on that boundary, so the flag is what decides whether there is
	// anything to give back at all: taken from the cut it is false, both
	// measurements come out alike, and the correction is silently nought.
	lead := ContextBefore(before + ContextBefore(whole[:base]))
	trail := ContextAfter(ContextAfter(whole[end:]) + after)
	how := Shaping{Before: lead, After: trail, ContextKerns: item.ContextKerns, Off: item.Off}
	withTail, _ := br.spanPx(item, how)
	how.After = ""
	alone, _ := br.spanPx(item, how)
	if delta := alone - withTail; delta != 0 {
		// Applied to the group's own measurement rather than replacing it, and
		// that is not a detail. The item's width is two ends of a *group*
		// rounded separately, so that the runs of one word tile it exactly;
		// measuring the item on its own rounds two different ends and the two
		// regimes disagree by a sixty-fourth. "AVATAR" came out 2338 whole and
		// 2339 spanned — one unit, and the fault
		// TestAWordCutInsideTilesTheSameWay is about.
		//
		// So the group's far end moves by the amount the kern was worth and is
		// rounded once, and the near end never enters the answer.
		_, through := shape.GroupSpan(br.advances(groupKey{
			face: item.Face, whole: whole, before: before, after: after,
			kerns: item.ContextKerns, off: item.Off,
		}), base, end, item.Size.Px())
		was, _ := style.FromPx(through)
		now, _ := style.FromPx(through + delta)
		return now.Sub(was)
	}
	return 0
}

// clusters is where an item's text may be cut, as offsets into that item's own
// text, drawn from one analysis of the run the item is a stretch of.
type clusters struct {
	// all is the whole run's boundaries, base where this item begins in the
	// run, and from the index of the first boundary inside the item.
	all  []int
	base int
	from int
}

func (c clusters) len() int     { return len(c.all) - c.from }
func (c clusters) at(i int) int { return c.all[c.from+i] - c.base }

// clustersOf finds where an item may be cut.
//
// A run divided for a line is divided again on the next line, and the tail is
// what is left of the same run — so its cluster boundaries are the run's, taken
// from where the tail begins. Found from the tail's own text instead they were
// found again for every line, over everything still to come: a word of twenty
// thousand characters in narrow lines analysed eleven million characters and
// allocated a boundary list the length of the remaining word each time.
//
// An item that has not been cut yet names the same run as the stretches that
// will be cut from it, through the same string, so the boundaries its first line
// finds are the ones every line after it reads.
func (br *Breaker) clustersOf(item Item) clusters {
	run, base := item.cutRun()
	all := br.runBounds(run)
	// The boundaries strictly inside this stretch: the one at its own start is
	// not a place to cut it, and neither is anything at or past its end.
	// Boundaries leaves both ends of a text out for the same reason.
	end := sort.SearchInts(all, base+len(item.Text))
	return clusters{all: all[:end], base: base, from: sort.SearchInts(all, base+1)}
}

// runBounds is segment.Boundaries of a run, memoized.
//
// Keyed on the run's text rather than on the RunCut, because a line that
// resumes inside a word divides the *original* item again from the offset it
// reached — so the two lines name the same run through two different values,
// and only the text they share tells them apart. A long run is found through
// the run the breaker holds, which compares the text rather than hashing it; a
// short one is looked up, which costs no more than comparing it would.
func (br *Breaker) runBounds(run string) []int {
	if len(run) >= tabulateFrom {
		return br.clusterBounds(br.runOf(run))
	}
	all, ok := br.bounds[run]
	if !ok {
		all = segment.Boundaries(nil, run)
		if br.bounds == nil {
			br.bounds = map[string][]int{}
		}
		br.bounds[run] = all
	}
	return all
}

// advances is the group's cumulative advances, shaped once and kept.
func (br *Breaker) advances(key groupKey) []float64 {
	if br.lastAdvances != nil && br.lastGroup == key {
		// Equal, and perhaps spelled in another string: the comparison after
		// this one is then of one pointer. See runIndex for why that matters.
		br.lastGroup = key
		return br.lastAdvances
	}
	cum, ok := br.grouped[key]
	if !ok && key.off.Vertical {
		// A run set upright: the pen moves down the page by each glyph's
		// vertical advance, which shaping states growing upwards. See
		// uprightKey.
		glyphs, _ := key.face.ShapeGlyphsInContext(key.whole, key.before, key.after, key.off)
		for i := range glyphs {
			glyphs[i].XAdvance = -glyphs[i].YAdvance
		}
		cum = shape.GroupAdvances(glyphs, len(key.whole))
	} else if !ok {
		glyphs := key.face.ShapeGroup(key.whole, key.before, key.after, key.kerns, key.off)
		cum = shape.GroupAdvances(glyphs, len(key.whole))
	}
	if !ok {
		if br.grouped == nil {
			br.grouped = map[groupKey][]float64{}
		}
		br.grouped[key] = cum
	}
	br.lastGroup, br.lastAdvances = key, cum
	return cum
}

// uprightKey is the shaping of a run set upright: its text and its context,
// shaped with Features.Vertical and nothing else merged in. A merge group is a
// horizontal run's — it is about glyphs a ligature or a kern forms across a
// boundary, and neither is applied down a line (see shape.Features.Vertical) —
// so an upright run is shaped as the backend drawing it shapes it: its own
// text, between its neighbours.
func uprightKey(face *shape.Face, text, before, after string, off shape.Features) groupKey {
	off.Vertical = true
	return groupKey{face: face, whole: text, before: before, after: after, off: off}
}

// uprightSpan is where a stretch of an upright run starts and ends along the
// line, each end rounded to a layout unit apart, so that the stretches of one
// run add up to the run's own rounded advance. See shape.GroupSpan.
func uprightSpan(cum []float64, lo, hi int, size style.Unit) (head, through style.Unit) {
	h, t := shape.GroupSpan(cum, lo, hi, size.Px())
	head, _ = style.FromPx(h)
	through, _ = style.FromPx(t)
	return head, through
}

// groupKey identifies one shaping of one merge group: everything that decides
// what the glyphs come out as, and nothing that differs between the runs
// sharing them.
// strictSpans turns the assumption stretchOf rests on — that the stretch
// named by from and to is the piece's own text — into a panic. It is off, and
// it was on for a full run of the corpus suite, reftests included, which is
// where the assumption was checked rather than assumed. The length test below
// stands in for it and falls back to counting rather than guessing.
const strictSpans = false

type groupKey struct {
	face          *shape.Face
	whole         string
	before, after string
	kerns         bool
	off           shape.Features
}

type measureKey struct {
	face    *shape.Face
	text    string
	size    style.Unit
	spacing TextSpacing
	// Everything else about how the run is set, all of which changes the
	// answer and so belongs in the key. It is one field rather than five
	// because a fact added to Shaping and forgotten here would give two runs
	// one entry — the memoization bug this key already has two comments about.
	how Shaping
}

// Shaping is everything about how a run of text is set that its own text does
// not say.
//
// It travels together because it is asked together: the measure needs all of it
// to give an answer, the memo needs all of it to tell two runs apart, and a
// caller that has one of these facts almost always has the rest. Five loose
// parameters is what it was, and the fifth was one too many.
type Shaping struct {
	// Before and After are the text either side of the run, where the boundary
	// between it and its neighbour did not break shaping. See Item.PreContext.
	Before, After string
	// MergeBefore and MergeAfter say that side may contribute glyphs and not
	// only forms. See Item.MergePre.
	MergeBefore, MergeAfter string
	// MergeGroup is the two of them and the run as one string, where the caller
	// has it. See Item.MergeGroup.
	MergeGroup string
	// ContextKerns says the neighbours above are set in this run's own face, so
	// a pair that spans the boundary is this font's pair. See Item.ContextKerns.
	ContextKerns bool
	// Upright says the run stands upright on a line of vertical text, so its
	// advance is the face's vertical one, or one em per typographic character
	// unit where the face states none. See Item.Upright.
	Upright bool
	// Off is what a document turned off: a font's own rules that a CSS property
	// or a CSS Text rule has overruled. See shape.Features.
	Off shape.Features
}
