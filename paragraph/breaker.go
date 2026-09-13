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
	key := measureKey{face: face, text: text, size: size, spacing: sp, how: how}
	if got, ok := br.measured[key]; ok {
		return got
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
	case how.Upright:
		// A run set upright on a line of vertical text advances one em per
		// character, and the face's horizontal advances say nothing about it.
		// CSS Writing Modes §4.4: where a face states no vertical metrics the
		// UA synthesizes them, and the em box is the synthesis. See UprightUnits
		// for what counts as a character here.
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
	br.measured[key] = w
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
		face: face, whole: how.MergeBefore + text + how.MergeAfter,
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
// for, and is what the fallback measures when the shaping cannot be shared —
// an upright run, whose advance is a count of characters rather than a sum of
// glyphs, and a run that is part of a merge group, whose string is not its own.
func (br *Breaker) spanWidth(item Item, from, to int, piece Item) style.Unit {
	if item.Face == nil || item.Upright {
		return br.MeasureSpacedInContext(item.Face, piece.Text, item.Size, item.Spacing,
			piece.shaping())
	}
	whole, base := item.Text, 0
	before, after, kerns := item.PreContext, item.PostContext, item.ContextKerns
	if item.MergePre != "" || item.MergePost != "" {
		// A run of a merge group: its string is the group's, so the shaping to
		// share is the group's and this run is a stretch inside it. That is the
		// arithmetic below with a longer string and a base of its own —
		// mergedSpan does it for a whole run and this does it for a stretch.
		//
		// Measuring the stretch on its own is what this used to do, and it
		// undid the one thing a merge group is for. The runs of a group tile it
		// exactly because the two ends of each are rounded separately; a stretch
		// rounded on its own does not, so the pieces of an item stopped adding
		// up to the item. "abc" in a box two characters wide sets "ab" and "c"
		// at 1228 and 615, and "<span>a</span><span>bc</span>" — the same word,
		// the same group — set the second line at 614.
		//
		// It is reached where a word is broken *inside*, which is overflow-wrap
		// and break-all, and nowhere else: a line that ends between two runs
		// ends at a run's own edge, which the tiling already handles. That is
		// why FuzzRunTiling never saw it — it lays every case out under nowrap,
		// so nothing is ever cut.
		run, at := item.Text, 0
		if c := item.Cut; c != nil {
			// And already a stretch of a longer run as well, which is the shape
			// a word broken across lines takes. MergePre and MergePost are the
			// *run's* neighbours in the group, so the group's string is built
			// from the whole run and this stretch sits that much further in.
			run, at = c.Text, item.CutAt
			// The contexts come from the cut for the same reason the string
			// does. A planted version that left them as the item's own moved
			// nothing, and the shape that would tell them apart is a face whose
			// context changes a width *and* a word broken three ways across a
			// box boundary — which is where the defect this does not fix lives,
			// so there is no honest fixture for it yet. See the note at the end
			// of TestAWordCutInsideTilesTheSameWay.
			before, after, kerns = c.Before, c.After, c.Kerns
		}
		whole, base = item.MergePre+run+item.MergePost, len(item.MergePre)+at
		before, after = shape.GroupContext(before, after, item.MergePre, item.MergePost)
	} else if c := item.Cut; c != nil {
		// Already a stretch of a longer run: the shaping to share is that run's,
		// and this stretch sits inside it. See Item.Cut.
		whole, base = c.Text, item.CutAt
		before, after, kerns = c.Before, c.After, c.Kerns
	}
	key := groupKey{
		face: item.Face, whole: whole, before: before, after: after,
		kerns: kerns, off: item.Off,
	}
	// The two ends rounded separately, so that the pieces of one item add up to
	// the item's own rounded width. See shape.GroupSpan.
	head, through := shape.GroupSpan(br.advances(key), base+from, base+to, item.Size.Px())
	lo, _ := style.FromPx(head)
	hi, _ := style.FromPx(through)
	return hi.Sub(lo).Add(SpacingAdvance(piece.Text, item.Spacing))
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
func (br *Breaker) clustersOf(item Item) clusters {
	if item.Cut == nil {
		return clusters{all: segment.Boundaries(nil, item.Text)}
	}
	// Keyed on the run's text rather than on the RunCut, because a line that
	// resumes inside a word divides the *original* item again from the offset
	// it reached — so the two lines name the same run through two different
	// values, and only the text they share tells them apart.
	all, ok := br.bounds[item.Cut.Text]
	if !ok {
		all = segment.Boundaries(nil, item.Cut.Text)
		if br.bounds == nil {
			br.bounds = map[string][]int{}
		}
		br.bounds[item.Cut.Text] = all
	}
	base := item.CutAt
	// The boundaries strictly inside this stretch: the one at its own start is
	// not a place to cut it, and neither is anything at or past its end.
	// Boundaries leaves both ends of a text out for the same reason.
	end := sort.SearchInts(all, base+len(item.Text))
	return clusters{all: all[:end], base: base, from: sort.SearchInts(all, base+1)}
}

// advances is the group's cumulative advances, shaped once and kept.
func (br *Breaker) advances(key groupKey) []float64 {
	if cum, ok := br.grouped[key]; ok {
		return cum
	}
	glyphs := key.face.ShapeGroup(key.whole, key.before, key.after, key.kerns, key.off)
	cum := shape.GroupAdvances(glyphs, len(key.whole))
	if br.grouped == nil {
		br.grouped = map[groupKey][]float64{}
	}
	br.grouped[key] = cum
	return cum
}

// groupKey identifies one shaping of one merge group: everything that decides
// what the glyphs come out as, and nothing that differs between the runs
// sharing them.
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
	// ContextKerns says the neighbours above are set in this run's own face, so
	// a pair that spans the boundary is this font's pair. See Item.ContextKerns.
	ContextKerns bool
	// Upright says the run stands upright on a line of vertical text, so its
	// advance is one em per typographic character unit rather than the face's.
	// See Item.Upright.
	Upright bool
	// Off is what a document turned off: a font's own rules that a CSS property
	// or a CSS Text rule has overruled. See shape.Features.
	Off shape.Features
}
