package layout

import (
	"fmt"
	"strings"
	"testing"
)

// TestComposeReturnsTheSheetItLaidOutOn.
//
// The ops are measured from the corner of the content box, so the margins are
// how a backend gets from the corner of the paper to the origin — and the
// document may have changed both. An @page rule sets the size and the margins,
// and a caller with only the Options it passed in has the sheet it *asked* for
// rather than the one that was used.
func TestComposeReturnsTheSheetItLaidOutOn(t *testing.T) {
	asked := PageSizePt(595, 842).WithMarginPt(20) // A4, 20pt
	out := Compose(Input{
		HTML: `<p>x</p>`,
		CSS:  []Stylesheet{{Source: `@page { size: 200pt 300pt; margin: 10pt }`}},
	}, Options{Page: asked})

	if out.Page.Width == asked.Width && out.Page.Height == asked.Height {
		t.Errorf("Compose returned the sheet it was asked for (%v x %v); the "+
			"document's @page rule set another one",
			out.Page.Width.Pt(), out.Page.Height.Pt())
	}
	// Within a layout unit, which is what a length rounds to.
	near := func(got, want float64) bool { return got-want < 0.01 && want-got < 0.01 }
	if got := out.Page.Width.Pt(); !near(got, 200) {
		t.Errorf("the page is %gpt wide, want 200 — what @page said", got)
	}
	if got := out.Page.Height.Pt(); !near(got, 300) {
		t.Errorf("the page is %gpt tall, want 300 — what @page said", got)
	}
	if got := out.Page.Margin.Left.Pt(); !near(got, 10) {
		t.Errorf("the left margin is %gpt, want 10 — the offset a backend "+
			"translates the ops by", got)
	}
	// And it is the geometry the ops were actually laid out in: the content box
	// is 180pt wide, so a box of 100% width is 180pt and not 555.
	if out.Page.Content().W != out.Page.Width.Sub(out.Page.Margin.Horizontal()) {
		t.Error("the returned page's content box is not its own width less its margins")
	}
}

// TestComposeReturnsTheCallersSheetWhereTheDocumentSaysNothing is the control:
// almost every document has no @page rule, and the answer then is what was
// asked for.
func TestComposeReturnsTheCallersSheetWhereTheDocumentSaysNothing(t *testing.T) {
	asked := PageSizePt(400, 500).WithMarginPt(15)
	out := Compose(Input{HTML: `<p>x</p>`}, Options{Page: asked})
	if out.Page != asked {
		t.Errorf("Compose returned %+v for a document that says nothing about its "+
			"own page, want %+v", out.Page, asked)
	}
}

// What Compose keeps of what the build found.
//
// Build had a recorder of its own and Compose replayed its finished list into a
// second one. A replay carries the findings and nothing else the recorder knew,
// and three things went with it.

// TestTheCountsSurviveIntoTheComposedDocument.
//
// A document that makes one mistake forty times is one finding and forty
// occurrences, and the count is what lets a report say so past the
// deduplication that makes the list readable. Replayed into a second recorder
// the finding arrives once — already deduplicated — and the count says one.
func TestTheCountsSurviveIntoTheComposedDocument(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, `<p id="e%d" style="color: ;">x</p>`, i)
	}
	out := Compose(Input{HTML: body.String()}, Options{})

	if n := len(out.Findings); n != 1 {
		t.Fatalf("%d findings for forty copies of one mistake, want one: %v",
			n, out.Findings)
	}
	if got := out.Counts[RuleInvalidCSS]; got != 40 {
		t.Errorf("%s fired %d times by Compose's count, want 40 — the count is "+
			"what a report says \"and 39 more\" from", RuleInvalidCSS, got)
	}
}

// TestWhatTheBoundCutIsStillCounted.
//
// The bound is on the list, and a document that fills it is told so — but
// "truncated" on its own says only that something is missing. The counts are
// what say *what*: a page shrunk past its minimum is counted whether or not
// there was room to write it down, so a report can name it after a build that
// used up the list. Compose exposed no counts at all, so there was nothing to
// say it with.
func TestWhatTheBoundCutIsStillCounted(t *testing.T) {
	old := maxFindings
	defer func() { maxFindings = old }()
	maxFindings = 5

	var css, body strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&css, "#e%d { color: rgb(%d) }\n", i, i)
		fmt.Fprintf(&body, `<p id="e%d">x</p>`, i)
	}
	// And one box far too large for the sheet, so the page has to be shrunk.
	css.WriteString(`#big { width: 4000pt; height: 4000pt }`)
	body.WriteString(`<div id="big">x</div>`)

	out := Compose(Input{HTML: body.String(), CSS: []Stylesheet{{Source: css.String()}}},
		Options{MinScale: 0.9})
	if !out.Truncated {
		t.Fatalf("the list was not truncated, so this document does not reach the "+
			"bound: %d findings", len(out.Findings))
	}
	if out.Counts[RuleMinScale] == 0 {
		t.Errorf("the page was shrunk past the minimum and nothing counted it; "+
			"counts: %v", out.Counts)
	}
}
