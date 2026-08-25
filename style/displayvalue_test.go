package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// An unrecognised "display" value, and what CSS 2.1 §4.2 says happens to the
// declaration holding it.
//
// It is dropped, and what stands is whatever the cascade would have produced
// without it — for an HTML element, the user agent sheet's own answer. Reading
// it as the property's *initial* value instead is a different thing entirely:
// the initial value is what the property means when nobody has set it, and here
// somebody did set it, wrongly. For a div the two answers are "block" and
// "inline", which is as far apart as display goes.

// displayOf runs a two-origin cascade — a user agent sheet that makes divs
// blocks, and an author sheet — and returns the div's computed display.
func displayOf(t *testing.T, decl string) (string, []Finding) {
	t.Helper()
	ua, errs := css.ParseStylesheet(`div { display: block }`)
	if len(errs) != 0 {
		t.Fatalf("the user agent sheet reported %v", errs)
	}
	author, errs := css.ParseStylesheet(`#target { ` + decl + ` }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet %q reported %v", decl, errs)
	}
	doc := parseDoc(t, `<div id="target">x</div>`)
	got := Apply(doc, []Sheet{
		{Origin: OriginUserAgent, Rules: ua},
		{Origin: OriginAuthor, Rules: author},
	})
	n := elementFor(t, doc, "#target")
	return got.Styles[n]["display"], got.Findings
}

// TestAnUnrecognisedDisplayLeavesTheUserAgentsValueStanding is the bug.
//
// "display: absolute" is what CSS2/abspos/static-fixed-inside-abspos writes —
// the author meant "position" — on the div whose background is the green square
// the test is about.
func TestAnUnrecognisedDisplayLeavesTheUserAgentsValueStanding(t *testing.T) {
	for _, decl := range []string{
		"display: absolute",
		"display: ABSOLUTE",
		"display: fixed",
		"display: -moz-box",
		"display: blah",
		"display: block block block block",
		"display: \"block\"",
		"display: 3",
	} {
		got, _ := displayOf(t, decl)
		if got != "block" {
			t.Errorf("%q computed to %q, want \"block\" — the declaration is invalid, "+
				"so what stands is the user agent sheet's", decl, got)
		}
	}
}

// TestTheDropIsReportedWithoutBeingCalledUnsupported. The author has no other
// way to learn their declaration went nowhere, and nothing is missing from the
// engine — CSS says what to do about an invalid declaration and this is it.
func TestTheDropIsReportedWithoutBeingCalledUnsupported(t *testing.T) {
	_, findings := displayOf(t, "display: absolute")
	found := false
	for _, f := range findings {
		if f.Property != "display" {
			continue
		}
		found = true
		if f.Unsupported {
			t.Errorf("the finding is marked unsupported: %q", f.Message)
		}
		if !strings.Contains(f.Message, "absolute") {
			t.Errorf("the finding does not name the value: %q", f.Message)
		}
	}
	if !found {
		t.Errorf("nothing was reported for %q", "display: absolute")
	}
}

// TestEveryDisplayCSSDefinesStillApplies is the containment argument, and the
// half that must not be got wrong: dropping a value that really is a display
// would silently put the user agent sheet back in charge of an element the
// author had restyled.
func TestEveryDisplayCSSDefinesStillApplies(t *testing.T) {
	for _, want := range []string{
		"none", "contents", "inline", "inline-block", "list-item",
		"table", "inline-table", "table-row-group", "table-header-group",
		"table-footer-group", "table-row", "table-cell",
		"table-column-group", "table-column", "table-caption",
		"flex", "inline-flex", "grid", "inline-grid",
		"ruby", "ruby-base", "ruby-text", "ruby-base-container",
		"ruby-text-container", "flow-root", "run-in", "math",
		// The two-value syntax of css-display-3.
		"inline flow-root", "block flow", "inline flow", "block flex",
		"inline grid", "list-item block flow", "block ruby",
		// And the one value here that no specification defines, which this
		// engine reads as a block because css-overflow-4 is written around it.
		"-webkit-box",
	} {
		got, _ := displayOf(t, "display: "+want)
		if got != want {
			t.Errorf("%q computed to %q; it is a display value and must stand", want, got)
		}
	}
}

// TestTheCSSWideKeywordsAreNotDisplayValuesAndAreNotDropped. They are values of
// every property, resolved after this point, and a check that does not know them
// would throw away "display: inherit" as a stray word.
func TestTheCSSWideKeywordsAreNotDisplayValuesAndAreNotDropped(t *testing.T) {
	for _, kw := range []string{"inherit", "initial", "unset", "revert"} {
		got, _ := displayOf(t, "display: "+kw)
		if got == "block" {
			t.Errorf("%q computed to \"block\", which is what the user agent sheet "+
				"said; the declaration was dropped", kw)
		}
	}
}

// TestAValueThisEngineCannotReadIsNotJudged. Custom properties are not
// substituted here, so "display: var(--d)" is a declaration whose value is not
// known yet. Calling it invalid would be deciding it on no evidence.
func TestAValueThisEngineCannotReadIsNotJudged(t *testing.T) {
	if got, _ := displayOf(t, "display: var(--d)"); got == "block" {
		t.Errorf("\"display: var(--d)\" computed to \"block\"; the declaration was " +
			"dropped for being unreadable rather than for being wrong")
	}
}

// TestThePrefixedIdiomWorks is what the rule buys an ordinary page. An author
// who writes a prefixed value and then the real one is relying on the first
// being thrown away by everything that does not know it.
func TestThePrefixedIdiomWorks(t *testing.T) {
	if got, _ := displayOf(t, "display: -moz-box; display: flex"); got != "flex" {
		t.Errorf("the pair computed to %q, want \"flex\"", got)
	}
	if got, _ := displayOf(t, "display: flex; display: -moz-box"); got != "flex" {
		t.Errorf("the pair computed to %q, want \"flex\" — the invalid declaration "+
			"came last and must still be the one that is thrown away", got)
	}
}
