package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A breaker made with no reporter, which is what NewBreaker's nil case is for.
//
// The doc comment there makes a claim: a caller that only wants to measure a
// run, or to ask where a paragraph would break, has nowhere to put a finding and
// no document to name in one, so the findings are dropped — and "the breaking
// itself is unchanged either way".
//
// Nothing had checked either half. discardFindings.ReportOverflow was at 0%
// coverage across every test and all 6253 reftest documents: the tests that pass
// nil never overflow a line, and the engine always passes a real reporter. So
// the one line the nil case exists for had never run, and the claim that the
// breaking is unchanged had never been compared against anything.

// countingReporter is the other side of the comparison.
type countingReporter struct {
	n     int
	items []Item
}

func (r *countingReporter) ReportOverflow(item Item, width style.Unit) {
	r.n++
	r.items = append(r.items, item)
}

// A word wider than its line is §11.1.1's overflow: there is nowhere to break
// it, so it is laid out and reported rather than clipped or the box widened.
const overflowingText = "aaa bbbbbbbbbbbbbbbbbbbb ccc"

func TestABreakerWithNoReporterDropsTheFindingAndBreaksTheSame(t *testing.T) {
	// Each character is 12px in Courier at 12px, so the long word is 240px and
	// the line is 100px: it cannot be broken and cannot fit.
	const width = 100

	rep := &countingReporter{}
	told := NewBreaker(rep)
	wanted := breakAll(t, told, words(t, told, courier(t), overflowingText), width)
	if rep.n == 0 {
		t.Fatalf("a word of %d px in a %d px line raised no overflow finding; "+
			"the fixture is not overflowing and proves nothing about the "+
			"reporter that drops them", 20*12, width)
	}

	// The same text through a breaker with nowhere to put the finding.
	quiet := NewBreaker(nil)
	got := breakAll(t, quiet, words(t, quiet, courier(t), overflowingText), width)

	if len(got) != len(wanted) {
		t.Fatalf("with no reporter the text took %d lines and with one %d: "+
			"%q against %q", len(got), len(wanted), got, wanted)
	}
	for i := range got {
		if got[i] != wanted[i] {
			t.Errorf("line %d is %q with no reporter and %q with one; §11.1.1's "+
				"overflow is recorded where a caller asked to hear about it and "+
				"nowhere else, so the breaking is the same either way",
				i, got[i], wanted[i])
		}
	}
}

// And the nil reporter is not the only way in: a breaker given one hears about
// every overflow, so the count is a property of the text rather than of who is
// listening.
func TestEveryOverflowIsReportedOnce(t *testing.T) {
	const width = 100
	rep := &countingReporter{}
	br := NewBreaker(rep)
	breakAll(t, br, words(t, br, courier(t), overflowingText), width)

	for _, item := range rep.items {
		if item.Text == "" {
			t.Errorf("an overflow was reported for an item with no text; the "+
				"finding names the item that would not fit, and %+v names "+
				"nothing a document could be pointed at", item)
		}
	}

	// One word overflows, and a line that merely ends early is not an overflow.
	// Without this the count could be "every line" and the test above would
	// still pass.
	if rep.n != 1 {
		t.Errorf("the text raised %d overflow findings, want 1: only the long "+
			"word has nowhere to break, and the two short ones fit their lines",
			rep.n)
	}
}
