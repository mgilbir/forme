package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// :visited, which matches nothing.
//
// It sits with the structural selectors rather than with :hover and :checked,
// and the difference is that this one has an answer. There is no browsing
// history here and no way to acquire one, so no link has been visited — not
// "cannot say", but no. The engine already relies on that answer: :link is
// implemented as :any-link, which is only correct because :visited is false.
//
// What refusing it cost was not the report. An unknown pseudo-class invalidates
// the whole selector, so ":link, :visited { … }" — the ordinary way a document
// takes the UA's link styling off — was dropped entirely, taking the :link half
// with it, and the links kept the blue the author had written a rule to remove.
// TestTheLinkHalfOfALinkVisitedRuleStillApplies is that half, and it is the one
// that shows on the page.
//
// These ride on font-family for the reason the rest of this package's tests do:
// a colour property drops a declaration whose value is not a colour, so a
// sentinel that names its own rule survives only on a free ident.

const linkDoc = `<a id="target" href="x">text</a>`

// TestVisitedMatchesNothing. A link is a link; none of them is visited.
func TestVisitedMatchesNothing(t *testing.T) {
	doc := parseDoc(t, linkDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`a { font-family: unvisited } a:visited { font-family: visited }`)},
		"a", "font-family")
	if got != "unvisited" {
		t.Errorf("the link resolved to %q; :visited matched something, and nothing "+
			"here has been visited", got)
	}
}

// TestTheLinkHalfOfALinkVisitedRuleStillApplies is the one that shows on the
// page, and the reason this is a rendering change and not a reporting one.
//
// While :visited was refused the whole selector list was invalid, so the rule
// was dropped and the :link half went with it — the very rule an author writes
// to remove the UA's link styling was the one that never ran.
func TestTheLinkHalfOfALinkVisitedRuleStillApplies(t *testing.T) {
	doc := parseDoc(t, linkDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`a { font-family: ua } :link, :visited { font-family: authors }`)},
		"a", "font-family")
	if got != "authors" {
		t.Errorf("the link resolved to %q, and the author's rule asked for "+
			"%q; the :visited half of the selector list took the :link half "+
			"down with it", got, "authors")
	}
}

// TestAnUnvisitedLinkStillAnswersNot: :not(:visited) is every link, which
// follows from :visited being false and does not follow from it being refused —
// a refused pseudo-class inside :not() invalidates the selector around it.
func TestAnUnvisitedLinkStillAnswersNot(t *testing.T) {
	doc := parseDoc(t, linkDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`a { font-family: plain } a:not(:visited) { font-family: matched }`)},
		"a", "font-family")
	if got != "matched" {
		t.Errorf("a:not(:visited) resolved to %q; no link is visited, so "+
			":not(:visited) is every link", got)
	}
}

// TestVisitedIsNotReportedAsUnsupported. The finding was not merely noise: a
// reftest whose documents raise one is a test whose pass proves nothing, and
// twelve of the suite's were held back by this alone.
func TestVisitedIsNotReportedAsUnsupported(t *testing.T) {
	for _, sel := range []string{
		"a:visited",
		":link, :visited",
		"a:not(:visited)",
		"a:visited::before",
	} {
		vals, _ := css.ParseComponentValues(sel)
		sels, errs, ok := css.ParseSelectorList(vals)
		if !ok || len(sels) == 0 {
			t.Errorf("%q was refused", sel)
		}
		for _, e := range errs {
			t.Errorf("%q reported %q", sel, e.Message)
		}
	}
}

// TestTheInteractiveOnesAreStillRefused is the containment argument, and the
// test this change most needs.
//
// The line moves for :visited and for nothing else. Everything that genuinely
// has no answer here must still be refused — turning "cannot say" into "no"
// across the board is exactly the silent-plausible-wrongness the subset exists
// to prevent, and it is what this change would look like if it went too far.
func TestTheInteractiveOnesAreStillRefused(t *testing.T) {
	for _, sel := range []string{
		"a:hover", "a:focus", "a:active", "input:checked", ":target",
		"input:disabled", "a:focus-within", ":fullscreen",
	} {
		vals, _ := css.ParseComponentValues(sel)
		sels, errs, ok := css.ParseSelectorList(vals)
		if ok || len(sels) != 0 {
			t.Errorf("%q was accepted; a page laid out once cannot answer it", sel)
			continue
		}
		if len(errs) == 0 {
			t.Errorf("%q was refused with no explanation", sel)
		}
	}
	// Whether the finding claims the *page* is wrong is a separate question,
	// answered in css/selector.go by whether the document itself answers the
	// selector: ":disabled" is written in the markup and ":hover" is not. This
	// test is about the line :visited moved and about nothing else, so it asks
	// only that each of these is still refused and still explained.
}
