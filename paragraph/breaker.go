package paragraph

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
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
	if how.Upright {
		// A run set upright on a line of vertical text advances one em per
		// character, and the face's horizontal advances say nothing about it.
		// CSS Writing Modes §4.4: where a face states no vertical metrics the
		// UA synthesizes them, and the em box is the synthesis. See UprightUnits
		// for what counts as a character here.
		w = size.Mul(float64(UprightUnits(text)))
	} else {
		// The two ends of the run within the group it was shaped with, each
		// rounded to a layout unit, rather than the difference rounded once.
		// Every run of a group then begins where the one before it ended and
		// the widths add up to the group's own rounded width — where rounding
		// the differences leaves a word written in three runs a sixty-fourth of
		// a pixel from the same word written in one. See
		// shape.MeasureShapedMergedSpan.
		head, through := face.MeasureShapedMergedSpan(text, size.Px(),
			how.Before, how.After, how.MergeBefore, how.MergeAfter,
			how.ContextKerns, how.Off)
		lo, _ := style.FromPx(head)
		hi, _ := style.FromPx(through)
		w = hi.Sub(lo)
	}
	w = w.Add(SpacingAdvance(text, sp))
	br.measured[key] = w
	return w
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
