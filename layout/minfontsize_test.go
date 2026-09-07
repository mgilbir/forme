package layout

import (
	"strings"
	"testing"
)

// TestTheSmallestTextOnThePageIsWhatIsChecked is the guardrail asked about the
// wrong thing.
//
// §6.1's floor is about text a reader cannot read, and what a reader sees is a
// run. It was asked of the block container's own font size: a paragraph at 20px
// holding a two-pixel span was checked at twenty and drawn at two, and two
// points is the size this guard exists for. An inline box setting its own size
// is ordinary markup — a footnote marker, a legal line, a caption.
func TestTheSmallestTextOnThePageIsWhatIsChecked(t *testing.T) {
	for _, tc := range []struct{ name, html string }{
		{"a span inside a paragraph",
			`<p style="font-size:20px">big <span style="font-size:2px">tiny</span></p>`},
		{"a span nested two deep",
			`<p style="font-size:20px"><b><span style="font-size:2px">tiny</span></b></p>`},
		{"the whole paragraph",
			`<p style="font-size:2px">tiny</p>`},
		{"a list marker",
			`<ul style="font-size:2px"><li>tiny</li></ul>`},
	} {
		got := Compose(Input{HTML: tc.html}, Options{})
		if !hasRule(got.Findings, RuleMinFontSize) {
			t.Errorf("%s: raised %v, want the min-font-size finding",
				tc.name, ruleNames(got.Findings))
		}
	}
}

// TestOrdinaryTextIsNotReportedAsTooSmall is the control: a guard that reported
// every page would pass the test above and say nothing about any document.
func TestOrdinaryTextIsNotReportedAsTooSmall(t *testing.T) {
	for _, tc := range []struct{ name, html string }{
		{"a paragraph", `<p>ordinary</p>`},
		{"a small span in a large paragraph",
			`<p style="font-size:20px">big <span style="font-size:10px">small</span></p>`},
		{"a list", `<ul><li>one</li><li>two</li></ul>`},
		{"a long document", `<p>` + strings.Repeat("word ", 200) + `</p>`},
	} {
		got := Compose(Input{HTML: tc.html}, Options{})
		if hasRule(got.Findings, RuleMinFontSize) {
			t.Errorf("%s: reported text as too small: %v", tc.name, got.Findings)
		}
	}
}

// TestABlockWhoseTextIsAllLargerIsNotReported is the false positive the old
// question produced in the other direction: a block whose own size is tiny but
// which draws nothing at that size has no unreadable text on it.
func TestABlockWhoseTextIsAllLargerIsNotReported(t *testing.T) {
	got := Compose(Input{
		HTML: `<p style="font-size:2px"><span style="font-size:20px">big</span></p>`,
	}, Options{})
	if hasRule(got.Findings, RuleMinFontSize) {
		t.Errorf("a paragraph that draws nothing at its own size was reported: %v",
			got.Findings)
	}
}
