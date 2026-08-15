package layout

import (
	"fmt"
	"strings"
	"testing"
)

// What Compose reports beyond the display list.
//
// Refused and Truncated are not summaries of Findings and cannot be recomputed
// from it. A rule counts the moment it fires — before the list deduplicates,
// and before the bound cuts it — so a document can be refused, or have had its
// report cut, with nothing in the list saying so. Every test here is about that
// gap, because it is the one a reader of the struct would not expect.

// TestComposeSaysWhenItCutTheFindings is the bound being visible at all.
//
// The recorder stops recording at maxFindings and says so, for the stated
// reason that a caller must never present a cut list as a complete one. Compose
// dropped that answer, which made every backend do the thing the flag exists to
// prevent: report five problems about a document that has four hundred.
func TestComposeSaysWhenItCutTheFindings(t *testing.T) {
	// The bound is lowered rather than the document inflated. Reaching five
	// hundred means five hundred findings differing in rule, message and place,
	// and the recorder folds duplicates hard enough that no honest document
	// produces them — a flood of six hundred at-rules tops out at 201 against
	// the real bound. What is under test is what happens once the list fills,
	// and that does not depend on where the ceiling is.
	defer func(n int) { maxFindings = n }(maxFindings)
	maxFindings = 20

	var css strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&css, "@nonsense%d { a: b }\n", i)
	}
	out := Compose(Input{
		HTML: `<p>x</p>`,
		CSS:  []Stylesheet{{Source: css.String()}},
	}, Options{Page: A4})

	if len(out.Findings) != maxFindings {
		t.Fatalf("%d findings recorded, want the bound of %d — this test needs to "+
			"cross it or it asserts nothing", len(out.Findings), maxFindings)
	}
	if !out.Truncated {
		t.Error("the findings were cut and Composed does not say so")
	}

	// And the other way: a document under the bound must not claim truncation.
	if Compose(Input{HTML: `<p>x</p>`}, Options{Page: A4}).Truncated {
		t.Error("a one-paragraph document reported truncated findings")
	}
}

// TestComposeKeepsBuildsVerdictWhenItsListOverflowed is the defect the flag
// above uncovered, and the worse half of it.
//
// Build keeps its own recorder and Compose replays its findings into a fresh
// one. That carries everything except the two answers that are not findings —
// so a build that overflowed, and was refused by a finding the bound dropped,
// came out of Compose as a document with nothing wrong with it.
func TestComposeKeepsBuildsVerdictWhenItsListOverflowed(t *testing.T) {
	defer func(n int) { maxFindings = n }(maxFindings)
	maxFindings = 5

	// The flood goes in a <style> element, which Build parses before the
	// stylesheets the caller passed; the fatal finding comes from the cascade,
	// which runs once every sheet is in. So the blocking one arrives with the
	// list already full, which is the case under test — and the skip below says
	// so rather than passing quietly if that ordering ever stops holding.
	var flood strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&flood, "@nonsense%d { a: b }\n", i)
	}
	out := Compose(Input{
		HTML:   `<style>` + flood.String() + `</style><p class="b">x</p>`,
		CSS:    []Stylesheet{{Source: `.b { mix-blend-mode: multiply }`}},
		Policy: Policy{RuleUnsupportedProperty: Error},
	}, Options{Page: A4})

	if !out.Truncated {
		t.Fatalf("only %d findings and no truncation; this test needs the list to "+
			"overflow", len(out.Findings))
	}
	for _, f := range out.Findings {
		if f.Severity == Error {
			t.Skip("the blocking finding survived the bound on this run, so the " +
				"assertion below would pass without the fix")
		}
	}
	if !out.Refused {
		t.Error("a document refused during the build came out of Compose as fine: " +
			"the finding that refused it was dropped by the bound, and the verdict " +
			"was taken from what was left rather than from Build")
	}
}

// TestRefusedCountsBeforeTheListDoes is why Refused is a field and not
// something a caller could work out.
//
// Asserted on the recorder, where the ordering lives: arranging it through
// Compose means finding five hundred warnings that each carry a distinct place,
// which the test above has to lower the bound to avoid.
func TestRefusedCountsBeforeTheListDoes(t *testing.T) {
	r := NewRecorder(nil)
	// Fill the list with warnings, each at a distinct place so none is folded
	// into another.
	for i := 0; i < maxFindings; i++ {
		r.ReportDetail(Finding{
			Rule:    RuleInvalidCSS,
			Message: "filler",
			Path:    fmt.Sprintf("html > body > p:nth-child(%d)", i),
		})
	}
	if len(r.Findings()) != maxFindings {
		t.Fatalf("the list holds %d findings; this test needs it full", len(r.Findings()))
	}
	if r.Failed() {
		t.Fatal("a warning marked the render failed")
	}

	// Now the one that refuses, arriving with the list already full.
	r.ReportDetail(Finding{Rule: RuleMinFontSize, Message: "3pt", Path: "html > body > p.tiny"})

	if !r.Failed() {
		t.Error("a rule at Error severity did not mark the render failed")
	}
	if !r.Truncated() {
		t.Error("the list was full and the recorder does not say it was cut")
	}
	for _, f := range r.Findings() {
		if f.Severity == Error {
			t.Fatal("the blocking finding fitted in the list after all, so this " +
				"test is not exercising the case it describes")
		}
	}
}
