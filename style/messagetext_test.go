package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/diag"
)

// A finding quotes the stylesheet and the document, and is made text in
// appendBounded, which every finding this stage produces passes through. See
// internal/diag, and html/messagetext_test.go for the crasher that found the
// shape.

// TestAStylingFindingIsText is the messages this stage writes itself, each
// quoting a value as the stylesheet wrote it.
func TestAStylingFindingIsText(t *testing.T) {
	for _, tc := range []struct{ sheet, want string }{
		{"p{color:a\x01b}", `"color: a?b" is not a colour, so the declaration was dropped`},
		{"p{width:1a\x1b}", `"width: 1a?" is not a valid value of width, so the declaration was dropped`},
		{"@media a\x01b {p{}}", `the media query "a?b" asks about "a?b", which this engine ` +
			`cannot answer, so the rules inside it were not applied`},
	} {
		doc, _, _ := html.Parse("<p>x</p>")
		rules, _ := css.ParseStylesheet(tc.sheet)
		got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
		found := false
		for _, f := range got.Findings {
			if !diag.IsText(f.Message) || !diag.IsText(f.Property) {
				t.Errorf("%q: a finding that is not text: %q (%q)", tc.sheet, f.Message, f.Property)
			}
			found = found || f.Message == tc.want
		}
		if !found {
			t.Errorf("%q: no finding %q among %+v", tc.sheet, tc.want, got.Findings)
		}
	}
}

// TestEveryStylingFindingIsMadeText drives the one place a finding is
// recorded, with one written as a careless new call site would write it.
func TestEveryStylingFindingIsMadeText(t *testing.T) {
	got := appendBounded(nil, Finding{Message: "<a\x93> \x01", Property: "b\u0085"})
	if len(got) != 1 {
		t.Fatalf("recorded %d findings", len(got))
	}
	if want := "<a�> ?"; got[0].Message != want {
		t.Errorf("the message is %q, want %q", got[0].Message, want)
	}
	if want := "b?"; got[0].Property != want {
		t.Errorf("the property is %q, want %q", got[0].Property, want)
	}
}
