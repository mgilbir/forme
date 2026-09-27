package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/diag"
)

// A finding quotes the document, and is made text in Recorder.record, which
// every finding passes through. See internal/diag, and
// html/messagetext_test.go for the crasher that found the shape.

// TestAFindingAboutAnElementIsText is a path built from an id that is not
// UTF-8 and a message quoting a URL that holds a C1 control. quoteValue made
// the second text-like already, except that it let C1 through; nothing made
// the first text at all.
func TestAFindingAboutAnElementIsText(t *testing.T) {
	built := Build(Input{HTML: "<img src=\"a\u0085\" id=q\x93>"})
	found := false
	for _, f := range built.Findings {
		for _, s := range [...]string{f.Message, f.Path, f.Selector, f.Property} {
			if !diag.IsText(s) {
				t.Errorf("a finding that is not text: %q in %+v", s, f)
			}
		}
		if f.Path == "html > body > img#q�" &&
			strings.HasPrefix(f.Message, `the image at "a?" was not loaded`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no finding about the image among %+v", built.Findings)
	}
}

// TestEveryFindingIsMadeText drives the recorder directly, with a finding
// written as a careless new call site would write it.
func TestEveryFindingIsMadeText(t *testing.T) {
	rec := NewRecorder(nil)
	rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedProperty,
		Message:  "a\x93",
		Path:     "b\x01",
		Selector: "c\u0085",
		Property: "d\x7f",
		Source:   Source{HTMLOffset: -1, CSSOffset: 3, Sheet: "s\x1b"},
	})
	got := rec.Findings()
	if len(got) != 1 {
		t.Fatalf("recorded %d findings", len(got))
	}
	f := got[0]
	if f.Message != "a�" || f.Path != "b?" || f.Selector != "c?" || f.Property != "d?" {
		t.Errorf("recorded %+v", f)
	}
	// The sheet's name is the caller's to look a file up by, and is kept; it
	// is made text where it is shown.
	if f.Source.Sheet != "s\x1b" {
		t.Errorf("the sheet's name became %q", f.Source.Sheet)
	}
	if s := f.Error(); !diag.IsText(s) {
		t.Errorf("the finding is shown as %q, which is not text", s)
	}
}
