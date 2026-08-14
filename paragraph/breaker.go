package paragraph

import (
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
