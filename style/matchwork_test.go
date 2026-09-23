package style

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// Matching work, per document.
//
// The bound per match is ten thousand steps, and it held: what it did not do was
// bound anything else. Audit C20 is its shape — rules of the form ".nopeK .a .a
// … b" against forty nested .a and N <b> spend the whole per-match budget on
// every pair, so 1 KB of CSS and 30 KB of markup took 33 s, and the cost is the
// product of the two. Audit C113 is the structural pseudo-classes, which asked
// their position of the siblings one element at a time and were n² in a list.
//
// The measure is the matcher's own steps where that is what changed, because
// steps are the work and have no noise, and time where the old cost was not
// counted in steps at all.

// ruleWorkOf styles a document and reports the matching steps every rule spent,
// and the Styled result.
func ruleWorkOf(t *testing.T, doc *html.Node, src string) (int, Styled, *Styler) {
	t.Helper()
	rules, errs := css.ParseStylesheet(src)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	got, s := applyIn(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}, nil, Media{})
	total := 0
	for _, w := range s.budget.rules {
		total += w.spent
	}
	return total, got, s
}

// c20 is the audit's fixture: depth nested div.a with leaves <b> inside the
// deepest, and rules ".nopeK .a .a … (14×) b".
func c20(rules, leaves int) (string, string) {
	const depth = 40
	doc := strings.Repeat(`<div class="a">`, depth) + strings.Repeat("<b>x</b>", leaves) +
		strings.Repeat("</div>", depth)
	var css strings.Builder
	for k := 0; k < rules; k++ {
		fmt.Fprintf(&css, ".nope%d %sb { color: red }\n", k, strings.Repeat(".a ", 14))
	}
	return doc, css.String()
}

// TestMatchingWorkIsLinearInRulesAndElements is C20. Each of those rules is a
// chain that cannot match — no element is .nopeK — and it cost the per-match
// bound on every <b> because every way of placing fourteen .a on forty ancestors
// was tried. It now costs one walk of the ancestors (see matchResult), so the
// work is the rules times the leaves times the depth, and four times either is
// four times the work.
func TestMatchingWorkIsLinearInRulesAndElements(t *testing.T) {
	work := func(rules, leaves int) int {
		t.Helper()
		doc, src := c20(rules, leaves)
		total, got, _ := ruleWorkOf(t, parseDoc(t, doc), src)
		if got.Incomplete {
			t.Fatalf("%d rules on %d leaves tripped the budget: a chain that cannot "+
				"match is being tried in every arrangement", rules, leaves)
		}
		// A walk of the forty ancestors, and the fourteen compounds: well under a
		// hundred steps a pair, where the per-match bound is ten thousand.
		if per := total / (rules * leaves); per > 100 {
			t.Errorf("%d rules on %d leaves cost %d steps a pair", rules, leaves, per)
		}
		return total
	}
	base := work(2, 100)
	for _, tc := range []struct{ rules, leaves int }{{8, 100}, {2, 400}} {
		if got := work(tc.rules, tc.leaves); float64(got) > 8*float64(base) {
			t.Errorf("%d rules on %d leaves cost %d steps against %d for 2 on 100: "+
				"%.1f times for four times the input", tc.rules, tc.leaves, got, base,
				float64(got)/float64(base))
		}
	}
}

// TestAnExpensiveRuleIsSwitchedOffAndNothingElseIs is the document-wide half of
// the bound. A rule whose every match runs to the per-match bound — which a
// selector can still do, with arguments nested in arguments — has an allowance
// for the document and is switched off once it has spent it; the work it is
// allowed does not grow with the document faster than the ordinary per-element
// allowance. The rules beside it are not touched: they apply to every element,
// before and after it was switched off. And all of it is reported, at the rule.
func TestAnExpensiveRuleIsSwitchedOffAndNothingElseIs(t *testing.T) {
	const depth = 40
	run := func(paragraphs int) int {
		t.Helper()
		doc := parseDoc(t, strings.Repeat(`<div class="x">`, depth)+
			strings.Repeat(`<p class="p">x</p>`, paragraphs)+strings.Repeat("</div>", depth))
		src := expensiveSelector + " { color: red }\n" +
			"p { font-family: plain }\n.x .p { font-style: italic }"
		_, got, s := ruleWorkOf(t, doc, src)

		if !got.Incomplete {
			t.Fatalf("%d paragraphs: the expensive rule tripped nothing", paragraphs)
		}
		w := s.budget.rules[0]
		if !w.off {
			t.Errorf("%d paragraphs: the expensive rule spent %d steps on %d tries "+
				"and was not switched off", paragraphs, w.spent, w.tries)
		}
		// Its spending stops within one match of its allowance.
		if limit := allowance(w.tries) + maxMatchSteps + 1; w.spent > limit {
			t.Errorf("%d paragraphs: the expensive rule spent %d steps, over its "+
				"allowance of %d", paragraphs, w.spent, limit)
		}
		// The other two are untouched, on every paragraph.
		for i := 1; i < len(s.budget.rules); i++ {
			if o := s.budget.rules[i]; o.off || o.trips > 0 {
				t.Errorf("an ordinary rule was cut short: %+v", o)
			}
		}
		n := 0
		doc.Walk(func(e *html.Node) bool {
			if e.Type == html.ElementNode && e.Name == "p" {
				n++
				cs := got.Styles[e]
				if cs.Get("font-family") != "plain" || cs.Get("font-style") != "italic" {
					t.Errorf("paragraph %d has font-family %q and font-style %q; the "+
						"rules beside the expensive one must still apply", n,
						cs.Get("font-family"), cs.Get("font-style"))
				}
				if strings.Contains(cs.Get("color"), "255, 0, 0") {
					t.Errorf("paragraph %d took the expensive rule, which selects nothing", n)
				}
			}
			return true
		})
		// And the report names the rule, at its offset.
		named := false
		for _, f := range got.Findings {
			if f.Offset == 0 && strings.Contains(f.Message, "not tried against any element after") {
				named = true
			}
		}
		if !named {
			t.Errorf("no finding says the expensive rule was switched off: %v", got.Findings)
		}
		return w.spent
	}
	// Once switched off, the rule costs nothing more: four times the paragraphs
	// is not four times its work, and certainly not the per-match bound on each.
	a, b := run(100), run(400)
	if float64(b) > 2*float64(a) {
		t.Errorf("the expensive rule spent %d steps on 100 paragraphs and %d on 400", a, b)
	}
}

// TestABudgetRunOutInsideNotIsNotAMatch. ":not()" turns its argument's "no"
// into "yes", and a budget that ran out inside the argument is a "no" nobody
// decided. Taken as the argument's answer it applied the rule to the element —
// a rule on an element it may not select, which is wrong rather than
// incomplete, and only incomplete is reported. It is incomplete: the match
// trips and says so, and does not match.
func TestABudgetRunOutInsideNotIsNotAMatch(t *testing.T) {
	doc := parseDoc(t, deepChain(200))
	m := NewMatcher(doc)
	sel := selectorsOf(t, "p:not("+expensiveSelector+")")[0]
	if m.Match(sel, doc.Element("p")) {
		t.Error("a match whose :not() argument ran out of budget was taken as a match")
	}
	if !m.Tripped() {
		t.Error("the argument did not trip the budget, so this checks nothing")
	}
}

// siblingList is n <li> under one <ul>, every third of them class "x", with an
// <hr> between every fifth so that the of-type family has something to skip.
func siblingList(n int) string {
	var b strings.Builder
	b.WriteString("<ul>")
	for i := 0; i < n; i++ {
		if i%5 == 4 {
			b.WriteString("<hr>")
		}
		if i%3 == 0 {
			b.WriteString(`<li class="x">i</li>`)
		} else {
			b.WriteString("<li>i</li>")
		}
	}
	b.WriteString("</ul>")
	return b.String()
}

var structuralSelectors = []string{
	"li:nth-child(2n+1)", "li:nth-last-child(2n+1)",
	"li:nth-of-type(3n)", "li:nth-last-of-type(3n)",
	"li:first-of-type", "li:last-of-type", "hr:only-of-type",
	"li:nth-child(odd of .x)", "li:nth-last-child(odd of .x)",
}

// TestStructuralPseudoClassesAreLinearInSiblings is C113. Every element asked
// its position of its siblings: indexOf walked from the first of them, and for
// :nth-last-* copied the whole list backwards first. That is n² over a list —
// 16,000 <li> took 2.7 s for :nth-last-child, and 12.5 s with "of S" — and none
// of it was counted as a step. Positions are now worked out once per parent, and the
// walk that works them out is counted where it is done (Matcher.work), so the
// work can be read rather than timed: each selector is matched against every
// child of lists of n and 4n, and the work is bounded at eight times.
func TestStructuralPseudoClassesAreLinearInSiblings(t *testing.T) {
	work := func(sel css.Selector, n int) int {
		doc := parseDoc(t, siblingList(n))
		m := NewMatcher(doc)
		for _, k := range doc.Element("ul").Children {
			m.Match(sel, k)
		}
		if m.Tripped() {
			t.Fatalf("matching over %d siblings tripped the budget", n)
		}
		steps, _ := m.takeWork()
		return steps
	}
	const small, large = 2000, 8000
	for _, src := range structuralSelectors {
		sel := selectorsOf(t, src)[0]
		a, b := work(sel, small), work(sel, large)
		if ratio := float64(b) / float64(a); ratio > 8 {
			t.Errorf("%s: %d siblings cost %d steps and %d cost %d, %.1f times for "+
				"four times the list; linear is four, and asking each element's "+
				"position of its siblings is sixteen", src, small, a, large, b, ratio)
		}
	}
}

// TestStructuralPositionsAreThePositions checks the remembered positions against
// the definition, counted out by hand for every child of a mixed list — so the
// memo is the same answer as the walk it replaced and not merely a fast one.
func TestStructuralPositionsAreThePositions(t *testing.T) {
	doc := parseDoc(t, siblingList(40))
	kids := []*html.Node{}
	for _, c := range doc.Element("ul").Children {
		if c.Type == html.ElementNode {
			kids = append(kids, c)
		}
	}
	isX := func(n *html.Node) bool { return hasClass(n, "x") }
	position := func(i int, same func(*html.Node) bool, last bool) int {
		if !same(kids[i]) {
			return 0
		}
		p := 0
		for j := range kids {
			if same(kids[j]) && ((!last && j <= i) || (last && j >= i)) {
				p++
			}
		}
		return p
	}
	m := NewMatcher(doc)
	ofX := selectorsOf(t, "li:nth-child(1 of .x)")[0].Compounds[0].Pseudos[0].Of
	for i, k := range kids {
		sameType := func(n *html.Node) bool { return strings.EqualFold(n.Name, k.Name) }
		every := func(*html.Node) bool { return true }
		for _, tc := range []struct {
			what      string
			got, want int
		}{
			{"nth-child", m.childPosition(k, nil, false), position(i, every, false)},
			{"nth-last-child", m.childPosition(k, nil, true), position(i, every, true)},
			{"nth-of-type", m.typePosition(k, false), position(i, sameType, false)},
			{"nth-last-of-type", m.typePosition(k, true), position(i, sameType, true)},
			{"nth-child of .x", m.childPosition(k, ofX, false), position(i, isX, false)},
			{"nth-last-child of .x", m.childPosition(k, ofX, true), position(i, isX, true)},
		} {
			if tc.got != tc.want {
				t.Errorf("child %d <%s>: %s is %d, want %d", i, k.Name, tc.what, tc.got, tc.want)
			}
		}
	}
}

// TestAnOfListOverALongListDoesNotTripOnItsFirstElement. Working out "of S" for
// a whole list is one match per child, and done inside the first element's match
// it was that element's budget that paid: a list of more than a few thousand
// tripped the per-match bound on its first item, every time. Each child is now
// matched with a budget of its own, and the work is charged to the rule instead.
func TestAnOfListOverALongListDoesNotTripOnItsFirstElement(t *testing.T) {
	const n = 15000
	doc := parseDoc(t, siblingList(n))
	m := NewMatcher(doc)
	sel := selectorsOf(t, "li:nth-last-child(1 of li.x)")[0]
	var lastX *html.Node
	for _, c := range doc.Element("ul").Children {
		if c.Type == html.ElementNode && hasClass(c, "x") {
			lastX = c
		}
	}
	first := doc.Element("li")
	if m.Match(sel, first) {
		t.Error("the first li matched :nth-last-child(1 of li.x)")
	}
	if !m.Match(sel, lastX) {
		t.Error("the last li.x did not match :nth-last-child(1 of li.x)")
	}
	if m.Tripped() {
		t.Errorf("an \"of S\" list over %d siblings tripped the per-match budget", n)
	}
	if steps, _ := m.takeWork(); steps < n {
		t.Errorf("the list cost %d steps over %d siblings; the work of matching S "+
			"against each is not being counted", steps, n)
	}
}

// TestAClassOrIdRuleIsNotWalkedForAnElementWithout is the index's new half: a
// rule whose subject needs a class or an id is filed under it, and an element
// without it never reaches the rule.
func TestAClassOrIdRuleIsNotWalkedForAnElementWithout(t *testing.T) {
	doc := parseDoc(t, indexDoc)
	s := &Styler{matcher: NewMatcher(doc), seen: map[string]bool{}, attrOffset: -1}
	rules := newRuleSet(s.prepare([]Sheet{author(t, indexSheet)}))

	walked := func(n *html.Node) map[string]bool {
		out := map[string]bool{}
		rules.forEach(n, func(r *preparedRule) bool {
			out[selectorText(r)] = true
			return true
		})
		return out
	}
	plain := walked(elementFor(t, doc, "li:first-child"))
	for _, sel := range []string{".c", "#x", ".d", "p.c.d", "#x.c", ".C"} {
		if plain[sel] {
			t.Errorf("an <li> with no class or id walked %q", sel)
		}
	}
	// And one that carries them reaches every rule filed under what it carries,
	// through each of its classes — written with a tab and two spaces between
	// them, which is how the attribute's set is split.
	carrying := walked(elementFor(t, doc, "#y"))
	for _, sel := range []string{".d", ".C", "li.e, #y", "#y ::after"} {
		if !carrying[sel] {
			t.Errorf("<li class=\"d C\" id=\"y\"> did not walk %q: %v", sel, carrying)
		}
	}
	tabbed := walked(elementFor(t, doc, "li.e"))
	if !tabbed[".d"] || !tabbed["li.e, #y"] {
		t.Errorf("<li class=\"\\te  d\"> did not walk both of its classes' rules: %v", tabbed)
	}
}
