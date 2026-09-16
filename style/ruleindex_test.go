package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// The index from an element's name to the rules that could select it.
//
// Its whole correctness is one sentence: a rule that matches must never be
// skipped. A rule that is *walked* and does not match costs a comparison; a
// rule that is skipped and would have matched is a declaration the page never
// gets, and nothing downstream can tell the difference from the author not
// having written it.
//
// So it is checked against the thing it replaced — a linear scan of every rule
// — over a document written to have one of every shape a subject compound can
// take.

const indexSheet = `
	p { color: red }
	P { background-color: red }
	div p { margin-top: 1px }
	div > p { margin-bottom: 1px }
	.c { padding-top: 1px }
	#x { padding-bottom: 1px }
	* { outline-width: 1px }
	[data-k] { border-top-width: 1px }
	[hidden] { border-bottom-width: 1px }
	p, div, span { border-left-width: 1px }
	p, .c { border-right-width: 1px }
	:root { top: 1px }
	p::before { content: "a" }
	div::after { content: "b" }
	*::before { left: 1px }
	p:not(.c) { right: 1px }
	:is(p, div) { bottom: 1px }
	li:nth-child(2) { z-index: 1 }
	SPAN { line-height: 2 }
	td + td { width: 1px }
	tr ~ tr { height: 1px }
`

const indexDoc = `<!DOCTYPE html><html><body>
<div id="x" class="c"><p data-k="v">one<span hidden>two</span></p></div>
<div><p class="c">three</p></div>
<ul><li>a</li><li>b</li></ul>
<table><tr><td>1</td><td>2</td></tr><tr><td>3</td></tr></table>
<P>four</P>
</body></html>`

// TestTheIndexSkipsNoRuleThatMatches walks every element of a document and
// requires the set of rules the index yields to hold every rule a linear scan
// finds a match for.
//
// It is stated as containment rather than as equality on purpose: the index is
// allowed to be *loose* — a rule it yields that does not match costs nothing
// but a comparison — and is not allowed to be tight.
func TestTheIndexSkipsNoRuleThatMatches(t *testing.T) {
	doc := parseDoc(t, indexDoc)
	s := &Styler{matcher: NewMatcher(doc), seen: map[string]bool{}, attrOffset: -1}
	rules := newRuleSet(s.prepare([]Sheet{author(t, indexSheet)}))

	elements, matched := 0, 0
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		elements++
		yielded := map[*preparedRule]bool{}
		rules.forEach(n, func(r *preparedRule) bool {
			yielded[r] = true
			return true
		})
		for _, pseudo := range []string{"", "before", "after"} {
			for i := range rules.rules {
				r := &rules.rules[i]
				if _, ok := s.matchSpecificityFor(r, n, pseudo); !ok {
					continue
				}
				matched++
				if !yielded[r] {
					t.Errorf("<%s>: a rule that matches was skipped by the index: %v",
						n.Name, selectorText(r))
				}
			}
		}
		return true
	})
	// The fixture has to reach both halves, or the containment above is vacuous.
	if elements < 15 || matched < 30 {
		t.Fatalf("the fixture visited %d elements and found %d matches; it is "+
			"not exercising the index", elements, matched)
	}
}

// TestTheIndexIsWorthHaving asserts the other half, which is why it exists: a
// rule that names a type is not walked for an element of another name.
//
// Without this the test above passes on an index that yields every rule, which
// is the change being undone.
func TestTheIndexIsWorthHaving(t *testing.T) {
	doc := parseDoc(t, indexDoc)
	s := &Styler{matcher: NewMatcher(doc), seen: map[string]bool{}, attrOffset: -1}
	rules := newRuleSet(s.prepare([]Sheet{author(t, indexSheet)}))

	li := elementFor(t, doc, "li:first-child")
	walked := 0
	rules.forEach(li, func(r *preparedRule) bool {
		walked++
		return true
	})
	if walked >= len(rules.rules) {
		t.Errorf("an <li> walked %d of %d rules; the index skipped nothing",
			walked, len(rules.rules))
	}
	// And the one rule that names li is among them, so the saving is not made
	// by skipping something that mattered.
	found := false
	rules.forEach(li, func(r *preparedRule) bool {
		if strings.Contains(selectorText(r), "li") {
			found = true
		}
		return true
	})
	if !found {
		t.Error("the rule naming li was not walked for an <li>")
	}
}

// TestANameTheIndexCannotFoldIsNotIndexed.
//
// The matcher compares a type with strings.EqualFold, which is Unicode's
// folding; the index folds ASCII. They agree on every name an element has and
// could disagree on one no element has, and "disagrees only about names nobody
// uses" is not an argument for an index that decides whether a rule is looked
// at. Such a type is filed under nothing and walked for everything.
func TestANameTheIndexCannotFoldIsNotIndexed(t *testing.T) {
	if got := subjectsOf(selectorsOf(t, "p, div")); len(got) != 2 {
		t.Errorf("subjectsOf(p, div) = %v, want two names", got)
	}
	if got := subjectsOf(selectorsOf(t, "p, .c")); got != nil {
		t.Errorf("subjectsOf(p, .c) = %v; a selector with no type can select "+
			"anything, so the rule is not indexed", got)
	}
	// A type with a byte above ASCII.
	if got := subjectsOf(selectorsOf(t, "élément")); got != nil {
		t.Errorf("subjectsOf(élément) = %v; the fold is ASCII's and this name "+
			"is not, so the rule goes to the matcher as it always did", got)
	}
	// And the case fold itself, which is what makes "<P>" reach "p { }".
	if got := subjectsOf(selectorsOf(t, "P")); len(got) != 1 || got[0] != "p" {
		t.Errorf("subjectsOf(P) = %v, want [p]", got)
	}
}

// selectorsOf parses a selector list the way a rule's is parsed.
func selectorsOf(t *testing.T, src string) []css.Selector {
	t.Helper()
	vals, _ := css.ParseComponentValues(src)
	sels, _, ok := css.ParseSelectorList(vals)
	if !ok {
		t.Fatalf("the selector %q was refused", src)
	}
	return sels
}

// selectorText renders a rule's selectors for a message.
func selectorText(r *preparedRule) string {
	var parts []string
	for _, sel := range r.selectors {
		var b strings.Builder
		for _, c := range sel.Compounds {
			b.WriteString(c.Type)
			for _, id := range c.IDs {
				b.WriteString("#" + id)
			}
			for _, class := range c.Classes {
				b.WriteString("." + class)
			}
			b.WriteString(" ")
		}
		if sel.PseudoElement != "" {
			b.WriteString("::" + sel.PseudoElement)
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return strings.Join(parts, ", ")
}
