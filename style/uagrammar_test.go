package style_test

import (
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/layout"
	"github.com/mgilbir/forme/style"
)

// TestTheUserAgentSheetIsValidCSS: every declaration in the engine's own
// default stylesheet is one the value grammar accepts. A grammar stricter than
// the sheet it is applied to drops the engine's own defaults — on every
// document, before an author has written a line — so this is the first thing a
// grammar that is wrong about a property breaks.
func TestTheUserAgentSheetIsValidCSS(t *testing.T) {
	rules, errs := css.ParseStylesheet(layout.UserAgentCSS)
	if len(errs) != 0 {
		t.Fatalf("the user agent sheet did not parse: %v", errs)
	}
	doc, _, _ := html.Parse("<p>x</p>")
	got := style.Apply(doc, []style.Sheet{{Origin: style.OriginUserAgent, Rules: rules}})
	for _, f := range got.Findings {
		t.Errorf("the user agent sheet raised %q (%s)", f.Message, f.Property)
	}
}
