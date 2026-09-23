package style

import (
	"testing"
)

// Selectors that are valid and match nothing on a page, matched. What the
// parser gives them is tested in css/inapplicable_test.go; this is what a
// document gets from them, which is where "the rest of the list stands" and "a
// wrong no never widens a rule" either show or do not.

const formDoc = `<div>
<a id="plain" class="active" href="x">link</a>
<input id="box" type="checkbox" checked>
<span id="on" class="on">on</span>
<p id="para">text</p>
</div>`

// TestTheRestOfAListWithHoverApplies is audit C25, end to end. The
// framework shape — a hover state and a class in one rule — styled nothing,
// because the :hover took the list with it.
func TestTheRestOfAListWithHoverApplies(t *testing.T) {
	for _, tc := range []struct {
		css, target string
	}{
		{`a:hover, .active { font-family: styled }`, "#plain"},
		{`.btn:is(:hover, :focus), a.active { font-family: styled }`, "#plain"},
		{`input:checked, .on { font-family: styled }`, "#on"},
		{`p:has(img), #para { font-family: styled }`, "#para"},
		{`a::selection, .active { font-family: styled }`, "#plain"},
		{`input::placeholder, .on { font-family: styled }`, "#on"},
	} {
		doc := parseDoc(t, formDoc)
		if got := styleOf(t, doc, []Sheet{author(t, tc.css)}, tc.target,
			"font-family"); got != "styled" {
			t.Errorf("%s: %s resolved to %q; the selector that matches nothing "+
				"here took the rest of its list with it", tc.css, tc.target, got)
		}
	}
}

// TestASelectorThatMatchesNothingMatchesNothing: kept is not applied. The
// checkbox is checked in the markup and this engine does not read that, so the
// rule is missing from it — which is reported — and is not on it.
func TestASelectorThatMatchesNothingMatchesNothing(t *testing.T) {
	for _, tc := range []struct {
		css, target string
	}{
		{`a:hover { font-family: styled }`, "#plain"},
		{`input:checked { font-family: styled }`, "#box"},
		{`a::before:hover { content: "x"; font-family: styled }`, "#plain"},
		// A pseudo-element cannot be an argument: "div:is(::before)" was an
		// :is() whose argument had an empty compound, and so selected every
		// element it was written on.
		{`p:is(::before) { font-family: styled }`, "#para"},
		{`p:is(::before, .nonesuch) { font-family: styled }`, "#para"},
	} {
		doc := parseDoc(t, formDoc)
		if got := styleOf(t, doc, []Sheet{author(t, tc.css)}, tc.target,
			"font-family"); got == "styled" {
			t.Errorf("%s matched %s", tc.css, tc.target)
		}
	}
}

// TestNotHoverIsEveryElement: nothing is hovered on a page, so the negation of
// :hover is every element, which is the page a browser prints.
func TestNotHoverIsEveryElement(t *testing.T) {
	doc := parseDoc(t, formDoc)
	if got := styleOf(t, doc, []Sheet{author(t, `p:not(:hover) { font-family: styled }`)},
		"#para", "font-family"); got != "styled" {
		t.Errorf("p:not(:hover) resolved to %q; nothing is hovered, so it is every p", got)
	}
}

// TestAnUnansweredNegationIsNotEverything: ":not(:checked)" cannot be read as
// "every element" because :checked matched nothing — the checked box is checked.
// The rule is missing from elements instead, and the page says so.
func TestAnUnansweredNegationIsNotEverything(t *testing.T) {
	for _, rule := range []string{
		`input:not(:checked) { font-family: styled }`,
		`input:not(:is(:checked, .x)) { font-family: styled }`,
		`input:nth-child(2 of :not(:disabled)) { font-family: styled }`,
	} {
		doc := parseDoc(t, formDoc)
		if got := styleOf(t, doc, []Sheet{author(t, rule)}, "#box",
			"font-family"); got == "styled" {
			t.Errorf("%s styled the checked box: a \"no\" this engine cannot vouch "+
				"for widened a rule under a negation", rule)
		}
		if found, unsupported := says(findingsOf(t, formDoc, rule), "matches nothing"); !found || !unsupported {
			t.Errorf("%s: found=%v unsupported=%v; the rule is missing from the "+
				"page and that has to be claimed", rule, found, unsupported)
		}
	}
}

// TestAnUnansweredOfSMovesNoPosition: "of S" decides which siblings are counted,
// so a selector in it that answers "no" where the document says "yes" moves
// every position after it. Here the checked box is the second of ".x, :checked"
// and the last span is the third; not counting the box made the span the
// second, and the rule landed on it.
func TestAnUnansweredOfSMovesNoPosition(t *testing.T) {
	const doc = `<div><span class="x">a</span><input class="c" type="checkbox" checked>` +
		`<span id="third" class="x">b</span></div>`
	got := styleOf(t, parseDoc(t, doc), []Sheet{author(t,
		`:nth-child(2 of .x, :checked) { font-family: styled }`)}, "#third", "font-family")
	if got == "styled" {
		t.Error("the third of \".x, :checked\" was selected as the second: a " +
			"\"no\" this engine cannot vouch for moved a position")
	}
}

// TestScopeIsTheRootOutsideAScope: Selectors 4 §8.3 makes :scope the root
// element where there is no scoping root, and a stylesheet has none. It was an
// unknown name, and took its rule's whole list with it.
func TestScopeIsTheRootOutsideAScope(t *testing.T) {
	doc := parseDoc(t, formDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`:scope > body { font-family: scoped } .nonesuch, :scope { font-family: root }`)},
		"body", "font-family")
	if got != "scoped" {
		t.Errorf(":scope > body resolved to %q; :scope is the root element here", got)
	}
	if got := styleOf(t, parseDoc(t, formDoc), []Sheet{author(t,
		`.nonesuch, :scope { font-family: root }`)}, "html", "font-family"); got != "root" {
		t.Errorf(":scope did not select the root element: %q", got)
	}
}

// TestAnIsWithAPseudoElementHasNoPseudoElementSpecificity: ":is(*, ::before)"
// is ":is(*)", whose specificity is nothing — and so a later "*" rule wins by
// order. Counting the pseudo-element made the earlier rule win.
func TestAnIsWithAPseudoElementHasNoPseudoElementSpecificity(t *testing.T) {
	doc := parseDoc(t, formDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`:is(*, ::before) { font-family: loses } * { font-family: wins }`)},
		"#para", "font-family")
	if got != "wins" {
		t.Errorf("the later \"*\" rule lost to \":is(*, ::before)\" (%q); the "+
			"pseudo-element was counted in a selector that cannot hold one", got)
	}
}
