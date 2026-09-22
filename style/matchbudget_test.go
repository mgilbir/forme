package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// TestOneExpensiveSelectorDoesNotSwitchMatchingOff is the budget as a global
// kill switch.
//
// The budget bounds the work one selector may spend on one element, and the
// flag the walk read to decide whether it was spent was the same one the caller
// reads to learn that some match was cut short — and nothing reset it. So one
// deep selector on one element turned matching off for the rest of the
// document: every later rule on every later element was skipped, and the page
// was styled by whatever happened to come before the selector that tripped.
func TestOneExpensiveSelectorDoesNotSwitchMatchingOff(t *testing.T) {
	// A tree deep enough for the expensive selector to be expensive on, and a
	// paragraph outside it. (A chain of combinators is not expensive any more;
	// see TestMatchBudgetTripsAndIsReported for what is.)
	html := strings.Replace(deepChain(200), `id="deep"`, `id="a"`, 1) + `<p id="b">y</p>`

	src := expensiveSelector + " { color: rgb(255, 0, 0) }\n#a, #b { color: rgb(0, 128, 0) }"
	rules, errs := css.ParseStylesheet(src)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, html)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})

	if !got.Incomplete {
		t.Fatal("the budget did not trip on this fixture, so the test is checking nothing")
	}
	// The later rule still applies — to the element the budget tripped on, and
	// to every element after it.
	for _, id := range []string{"#a", "#b"} {
		cs := got.Styles[elementFor(t, doc, id)]
		if !strings.Contains(cs["color"], "0, 128, 0") {
			t.Errorf("%s is %q; a budget spent on one selector must not stop the "+
				"selectors after it from matching", id, cs["color"])
		}
	}
}

// TestTheBudgetIsStillReportedWhenItTrips is the other half: the caller has to
// be told, because a page styled less than its stylesheet describes is a page
// that is quietly wrong. It is told twice over: that matching stopped early, and
// at which rule — and not at the rule beside it that cost nothing.
func TestTheBudgetIsStillReportedWhenItTrips(t *testing.T) {
	src := "p { color: blue }\n" + expensiveSelector + " { color: red }"
	rules, _ := css.ParseStylesheet(src)
	doc := parseDoc(t, deepChain(200))
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if !got.Incomplete {
		t.Fatal("the budget did not trip on this fixture, so the test is checking nothing")
	}
	if found, _ := says(got.Findings, "matching stopped early"); !found {
		t.Errorf("the budget tripped and raised %v", got.Findings)
	}
	at := strings.Index(src, expensiveSelector)
	named := false
	for _, f := range got.Findings {
		if f.Offset == at && strings.Contains(f.Message, "this rule's selector") {
			named = true
		}
		if f.Offset == 0 {
			t.Errorf("the rule \"p\" was named, and it cost nothing: %s", f.Message)
		}
	}
	if !named {
		t.Errorf("no finding names the rule at byte %d: %v", at, got.Findings)
	}
}

// TestAnOrdinaryStylesheetNeverTripsTheBudget is what makes the two above worth
// having: the bound is on a pathological selector and must not be reachable by
// a page doing nothing wrong.
func TestAnOrdinaryStylesheetNeverTripsTheBudget(t *testing.T) {
	var html strings.Builder
	for i := 0; i < 50; i++ {
		html.WriteString(`<div class="row"><span class="cell">x</span></div>`)
	}
	rules, _ := css.ParseStylesheet(
		`.row .cell { color: red } div span { font-weight: bold } #a > p + p { margin: 0 }`)
	doc := parseDoc(t, html.String())
	if got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}); got.Incomplete {
		t.Error("an ordinary stylesheet over an ordinary document tripped the budget")
	}
}
