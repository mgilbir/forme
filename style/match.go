// Package style applies a stylesheet to a document.
//
// It is the third of the engine's stages: the html package gives it a tree,
// the css package gives it rules, and what comes out is a styled tree — every
// element with the declarations that won for it.
//
// This file is the first half, selector matching. It is also the first thing
// that *uses* the selector structures the css package builds, which is worth
// saying plainly: until something matches, a selector parser can only be tested
// for shape, and a wrong shape and a right one are indistinguishable.
package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
)

// maxMatchSteps bounds the work one selector may spend on one element.
//
// A chain of combinators is no longer the way to reach it — see matchResult,
// which is what keeps "a b c d e" against a deep tree a sum rather than a
// product. What is left is a selector whose *compounds* are expensive: a
// ":not()" or ":is()" is a whole selector matched on every element the chain
// around it visits. Its answer for an element is remembered, so arguments nested
// inside arguments add rather than multiply — see matchesList — but each still
// costs a walk of the tree the first time it is asked of an element, and a
// selector can ask that of every element under a deep tree. Real selectors
// settle in tens of steps.
//
// It is the early exit and not the whole bound. A bound per match is a bound
// per (selector, element) pair, and a document has as many pairs as it has
// rules times elements: at ten thousand steps each, one kilobyte of CSS and
// thirty of markup took 33 seconds with the bound holding every time. What
// bounds the document is the budget each rule is given for the whole of it —
// see matchBudget.
//
// A budget that trips is *reported*, never silently answered "no match" — see
// Matcher.Tripped. A layout engine that quietly stopped matching would produce
// a document with styles missing and nothing to say so, which is the failure
// mode the whole reporting design exists to prevent.
const maxMatchSteps = 10000

// matchResult is what matching part of a selector found and, when it found
// nothing, how far the failure reaches.
//
// Matching runs right to left, and a combinator that can reach more than one
// element — the descendant's every ancestor, "~"'s every earlier sibling — tries
// each of them in turn. Answered only yes or no, a failure further left sends the
// search back to try the next one, and a chain of k descendant combinators that
// fails at its leftmost compound tries every way of placing the other k-1 on the
// ancestors: ".nonesuch .a .a .a … b" on forty nested .a is a product, and it
// spent the whole per-match budget on every <b> of the audit's document.
//
// Most of those retries cannot succeed, and the reason can be said precisely.
// If compounds[:i] cannot be matched starting from any ancestor of an element,
// they cannot be matched from any ancestor of an ancestor either: those are
// fewer places, not other ones. So a descendant search that runs out says so —
// failsCompletely — and every search to its right stops rather than moving one
// element up to ask again. "~" does the same for earlier siblings, whose own
// earlier siblings are a subset — failsAllSiblings. This is the classification
// WebKit and Blink match with, and it is what keeps a chain of combinators a
// sum: each compound walks the ancestors at most once.
type matchResult uint8

const (
	// matched: the element and everything left of it matched.
	matched matchResult = iota
	// failsLocally: this element does not do, and another may.
	failsLocally
	// failsAllSiblings: neither this element nor any earlier sibling of it will
	// do, so a "~" search stops; an ancestor still may.
	failsAllSiblings
	// failsCompletely: no element the search to the right could try next will
	// do — not this one, its ancestors, or the earlier siblings of either.
	failsCompletely
)

// Matcher applies selectors to one document.
//
// It holds the work budget, which is why matching goes through a value rather
// than a bare function: the budget has to outlive a single call to be worth
// anything, and whether it tripped has to be readable afterwards.
type Matcher struct {
	// root is the document's outermost element, which :root selects.
	root *html.Node

	// kids memoizes each parent's element children, and idx each element's
	// position among them.
	//
	// Without these, every step of a sibling walk rescans the parent's whole
	// child list. The tree does not change while it is being matched, so the
	// memo is safe and is built once per parent that is asked about.
	kids map[*html.Node][]*html.Node
	idx  map[*html.Node]int

	// ofType is each element's position among its siblings of the same name,
	// counted from the start and from the end, one-based; typed says which
	// parents' children it holds. series is the same for ":nth-child(An+B of
	// S)": the position of each child among the children S selects.
	//
	// The structural pseudo-classes are all positions, and a position is a
	// property of the parent's child list rather than of the element. Asked
	// per element by counting the siblings before it, ":nth-last-child" over a
	// list of n items cost n² — and copied the list backwards for every one of
	// them — and so did ":nth-of-type"; with "of S" each of the n² was a match:
	// 16,000 <li> took 2.7 s, 2.3 s and 12.5 s, and a list of ten thousand rows
	// is an ordinary table, not a hostile one. Worked out once per parent they
	// are O(1) per element.
	ofType map[*html.Node][2]int32
	typed  map[*html.Node]bool
	series map[seriesKey]*series

	// nested remembers which elements match which nested rule's parent — see
	// nesting — and lists which match which argument list of :is(), :where()
	// and :not() — see matchesList.
	nested map[nestingKey]bool
	lists  map[listKey]bool

	// steps is the work spent on the match in hand and over says that match ran
	// out; tripped remembers that some match did, for the caller.
	//
	// The two are separate because the budget is *per match*. One selector on
	// one element used to turn matching off for the rest of the document: the
	// flag the walk read was the same one the caller reads, and nothing reset
	// it — so a deep ".x .x .x … p" on one paragraph left every later selector
	// on every later element unmatched, and the page was styled by whatever
	// happened to come before it.
	steps   int
	over    bool
	tripped bool

	// work counts every step charged since the caller last took it, and trips
	// every match that ran out of budget in that time. They are what a caller
	// charges to the rule it was matching, which is how a bound per document is
	// kept without the matcher knowing what a rule is — see matchBudget. work
	// includes what filling a memo cost, which is charged to the match that
	// needed it and not to its budget: see seriesOf.
	work  int
	trips int

	// xml says the document was parsed as XHTML, which decides whether the
	// attribute values below are folded. It is read once here because the
	// alternative is a walk to the document node inside the matching loop.
	xml bool

	// langs is each element's language, which :lang() asks. Asked per match,
	// it was a walk to the root reading every ancestor's attributes, for every
	// element every :lang() rule was tried on. See html.Languages.
	langs html.Languages
}

// NewMatcher prepares to match selectors against a document.
func NewMatcher(doc *html.Node) *Matcher {
	return &Matcher{
		root:   documentElement(doc),
		kids:   map[*html.Node][]*html.Node{},
		idx:    map[*html.Node]int{},
		ofType: map[*html.Node][2]int32{},
		typed:  map[*html.Node]bool{},
		series: map[seriesKey]*series{},
		nested: map[nestingKey]bool{},
		lists:  map[listKey]bool{},
		xml:    doc.XMLDocument(),
	}
}

// documentElement is the <html> element, which is what :root means. It is not
// simply "the node with no parent" — that is the document node, which is not an
// element and which no selector matches.
func documentElement(doc *html.Node) *html.Node {
	if doc == nil {
		return nil
	}
	if doc.Type == html.ElementNode {
		return doc
	}
	for _, c := range doc.Children {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// Tripped reports whether the work budget stopped a match short.
//
// When it has, the answers this Matcher gave are a lower bound: some selectors
// that should have matched did not. A caller must report that rather than render
// as though the stylesheet had been applied in full.
func (m *Matcher) Tripped() bool { return m.tripped }

// Match reports whether an element is selected.
func (m *Matcher) Match(s css.Selector, n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode || len(s.Compounds) == 0 {
		return false
	}
	m.steps, m.over = 0, false
	return m.complex(s.Compounds, len(s.Compounds)-1, n) == matched
}

// takeWork reports the steps charged and the matches that ran out of budget
// since it was last asked, and starts counting again. See Matcher.work.
func (m *Matcher) takeWork() (steps, trips int) {
	steps, trips = m.work, m.trips
	m.work, m.trips = 0, 0
	return steps, trips
}

// complex matches compounds[:i+1] with compounds[i] against n.
//
// It runs right to left, from the subject outwards, which is not a
// micro-optimisation: a document has far more elements than a selector has
// compounds, and the subject is the cheapest thing to reject on. Matching left
// to right would search the tree for the first compound and then check whether
// anything under it was the element in hand.
//
// What it returns when it fails is how far the failure reaches, and the two
// loops stop on it — see matchResult. A budget that runs out fails completely,
// which stops every search at once.
func (m *Matcher) complex(compounds []css.Compound, i int, n *html.Node) matchResult {
	if m.spent() {
		return failsCompletely
	}
	if !m.compound(compounds[i], n) {
		return failsLocally
	}
	if m.over {
		// The budget ran out inside the compound — in an argument of ":not()",
		// whose "no" it turned into a "yes". Nothing that compound said can be
		// trusted, and a match is the one answer that must not come of it: a
		// rule applied to an element it does not select is wrong where a rule
		// missing is only incomplete, and only the second is reported.
		return failsCompletely
	}
	if i == 0 {
		return matched
	}

	switch compounds[i].Combinator {
	case css.Child:
		p := parentElement(n)
		if p == nil {
			return failsCompletely
		}
		// Whatever the parent's answer reaches, this one reaches as far: the
		// earlier siblings of n have the same parent, and n's ancestors and
		// their earlier siblings have that parent's ancestors.
		return m.complex(compounds, i-1, p)

	case css.NextSibling:
		s := m.prevElement(n)
		if s == nil {
			return failsAllSiblings
		}
		return m.complex(compounds, i-1, s)

	case css.SubsequentSibling:
		for s := m.prevElement(n); s != nil; s = m.prevElement(s) {
			switch r := m.complex(compounds, i-1, s); r {
			case matched, failsAllSiblings, failsCompletely:
				return r
			}
		}
		return failsAllSiblings

	default: // Descendant
		for p := parentElement(n); p != nil; p = parentElement(p) {
			switch r := m.complex(compounds, i-1, p); r {
			case matched, failsCompletely:
				return r
			}
		}
		// Every ancestor was tried and none would do, so none of *their*
		// ancestors will either, and nor will the earlier siblings of n or of
		// any of them, whose ancestors are the same elements.
		return failsCompletely
	}
}

// spent charges one step and reports whether the budget is gone.
func (m *Matcher) spent() bool {
	if m.over {
		return true
	}
	m.steps++
	m.work++
	if m.steps > maxMatchSteps {
		m.over, m.tripped = true, true
		m.trips++
		return true
	}
	return false
}

// compound reports whether one element satisfies every part of a compound.
//
// The order is cheapest-first and deliberately so: a type mismatch rejects most
// elements for most selectors, and the pseudo-classes — which may walk siblings
// or recurse into another selector list — are asked last.
//
// The type is compared ASCII case-insensitively, which is what HTML specifies
// for its elements' names and what the attribute values in htmlFoldedAttrs
// already get. It was strings.EqualFold, which is Unicode's simple case
// folding from the toolchain's release: U+212A KELVIN SIGN and U+017F LONG S
// fold to "k" and "s", so "\212Abd" selected <kbd> and "\17Fpan" <span> —
// a match between names that are not the same name. An element name from
// the HTML reader is ASCII, so ASCII's folding is also the only one that can
// tell two of them apart.
func (m *Matcher) compound(c css.Compound, n *html.Node) bool {
	if c.Type != "" && !ascii.EqualFold(c.Type, n.Name) {
		return false
	}
	for _, id := range c.IDs {
		// Two different identifiers in one compound match nothing, which falls
		// out of this rather than needing a rule: an element has one id.
		if v, ok := n.Attr("id"); !ok || v != id {
			return false
		}
	}
	for _, class := range c.Classes {
		if !hasClass(n, class) {
			return false
		}
	}
	for _, a := range c.Attrs {
		if !m.matchAttr(a, n) {
			return false
		}
	}
	for _, p := range c.Pseudos {
		if !m.pseudo(p, n) {
			return false
		}
	}
	return true
}

// hasClass reports whether an element carries a class.
//
// The attribute is a whitespace-separated set, so this is a membership test and
// not a substring one: class="subtitle" must not match ".title".
func hasClass(n *html.Node, want string) bool {
	v, ok := n.Attr("class")
	if !ok {
		return false
	}
	for _, got := range asciiFields(v) {
		if got == want {
			return true
		}
	}
	return false
}

// asciiFields splits on HTML's white space and not on Unicode's.
//
// The two are not the same set, and the difference is a class name. HTML says
// the class attribute is "a set of space-separated tokens" split on *ASCII*
// white space — tab, line feed, form feed, carriage return and space — so
// class="a\u00a0b" is one class whose name holds a no-break space, and .a
// selects nothing. strings.Fields splits on unicode.IsSpace, which takes the
// no-break space and every other space separator with it, so it found two
// classes where the document has one and applied a rule the author did not
// write. The same set decides "~=", which HTML defines the same way.
func asciiFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\n', '\f', '\r', ' ':
			return true
		}
		return false
	})
}

func (m *Matcher) matchAttr(a css.Attr, n *html.Node) bool {
	v, ok := n.Attr(a.Name)
	if !ok {
		return false
	}
	if a.Op == css.AttrExists {
		return true
	}

	got, want := v, a.Value
	if a.Insensitive || (!a.Sensitive && !m.xml && htmlFoldedAttrs[a.Name]) {
		got, want = ascii.Lower(got), ascii.Lower(want)
	}

	switch a.Op {
	case css.AttrEquals:
		return got == want
	case css.AttrIncludes:
		// A whitespace-separated set, like class. An empty value or one
		// containing whitespace can never match, because it is not a member of
		// any such set.
		if want == "" || strings.ContainsAny(want, " \t\n\r\f") {
			return false
		}
		return slices(got, want)
	case css.AttrDashMatch:
		// "en" matches "en" and "en-GB" but not "english". This exists for
		// language subtags, which is why the boundary is the hyphen and not any
		// prefix.
		return got == want || strings.HasPrefix(got, want+"-")
	case css.AttrPrefix:
		return want != "" && strings.HasPrefix(got, want)
	case css.AttrSuffix:
		return want != "" && strings.HasSuffix(got, want)
	case css.AttrSubstring:
		// The empty string is a substring of everything, so the specification
		// makes it match nothing instead — otherwise [href*=""] would select
		// every element with an href, which is what [href] already says.
		return want != "" && strings.Contains(got, want)
	}
	return false
}

func slices(value, want string) bool {
	for _, f := range asciiFields(value) {
		if f == want {
			return true
		}
	}
	return false
}

func (m *Matcher) pseudo(p css.Pseudo, n *html.Node) bool {
	switch p.Kind {
	case css.PseudoRoot:
		return n == m.root

	case css.PseudoEmpty:
		// "Empty" counts text of any kind, including whitespace: a paragraph
		// containing a single space is not empty. Comments do not count, and
		// this tree has none to begin with.
		for _, c := range n.Children {
			if c.Type == html.ElementNode {
				return false
			}
			if c.Type == html.TextNode && c.Text != "" {
				return false
			}
		}
		return true

	case css.PseudoFirstChild:
		return m.prevElement(n) == nil
	case css.PseudoLastChild:
		return m.nextElement(n) == nil
	case css.PseudoOnlyChild:
		return m.prevElement(n) == nil && m.nextElement(n) == nil

	case css.PseudoFirstOfType:
		return m.typePosition(n, false) == 1
	case css.PseudoLastOfType:
		return m.typePosition(n, true) == 1
	case css.PseudoOnlyOfType:
		return m.typePosition(n, false) == 1 && m.typePosition(n, true) == 1

	case css.PseudoNthChild:
		return p.AnB.Matches(m.childPosition(n, p.Of, false))
	case css.PseudoNthLastChild:
		return p.AnB.Matches(m.childPosition(n, p.Of, true))
	case css.PseudoNthOfType:
		return p.AnB.Matches(m.typePosition(n, false))
	case css.PseudoNthLastOfType:
		return p.AnB.Matches(m.typePosition(n, true))

	case css.PseudoNot:
		return !m.matchesList(p.Args, n)

	case css.PseudoIs, css.PseudoWhere:
		return m.matchesList(p.Args, n)

	case css.PseudoNesting:
		return m.nesting(p.Nest, n)

	case css.PseudoLang:
		return m.matchLang(n, p.Langs)

	case css.PseudoAnyLink:
		// :link and :any-link are the same thing once :visited cannot be true,
		// and both are about the document rather than about a person.
		return isLink(n)

	case css.PseudoVisited:
		// Nothing is visited here, and that is an answer rather than a refusal
		// — see the note on css.PseudoVisited. It is the same "no" the case
		// above already assumes in order to read :link as :any-link, so a
		// document cannot get one of the two answers without the other.
		return false

	case css.PseudoNever, css.PseudoUnanswered:
		// ":hover" on a page nobody hovers, and ":checked", which this engine
		// does not work out. Both select nothing, and the parser has already
		// said which of the two it is and made sure the second only ever
		// narrows a rule — see the note at the head of css/selector.go.
		return false
	}
	return false
}

// isLink reports whether an element is one: an <a> or an <area> with an href.
//
// It is one function because two places ask it. The other is the body element's
// "link" attribute, which HTML maps to the colour of "any element that is a
// link" — the same set this selects, and a second reading of "is a link" is a
// second answer waiting to differ from this one.
func isLink(n *html.Node) bool {
	if !ascii.EqualFold(n.Name, "a") && !ascii.EqualFold(n.Name, "area") {
		return false
	}
	return n.HasAttr("href")
}

// complexFrom matches a whole selector with n as its subject, which is what the
// argument of :is(), :not() and :where() asks.
func (m *Matcher) complexFrom(s css.Selector, n *html.Node) bool {
	if len(s.Compounds) == 0 {
		return false
	}
	return m.complex(s.Compounds, len(s.Compounds)-1, n) == matched
}

// nestingKey is one question about "&": does this element match that parent.
type nestingKey struct {
	nest *css.Nesting
	n    *html.Node
}

// nesting matches "&", which is ":is()" over the parent rule's selector list.
//
// The answer is remembered per element, and that is what keeps nesting as cheap
// as it is short. A rule's "&" is its parent's list, whose own "&" is *its*
// parent's, so "& & & & & & & &" nested seven deep asks about the outermost
// rule once per way of placing every level on the ancestors — eight to the
// seventh ways, for 162 bytes of CSS. Asked once per element instead, each level
// costs what its own selector costs, and the whole is the sum of the levels
// rather than their product.
//
// It is safe to remember because the question has no context: whether an
// element matches a selector list as its subject depends on the element and the
// tree, and neither changes while a document is matched. The one answer not
// kept is one the budget cut short, which is a "no" that may be wrong.
func (m *Matcher) nesting(nest *css.Nesting, n *html.Node) bool {
	if nest == nil {
		return false
	}
	key := nestingKey{nest, n}
	if got, ok := m.nested[key]; ok {
		return got
	}
	got := m.matchesAny(nest.Matchable(), n)
	if !m.over {
		m.nested[key] = got
	}
	return got
}

// listKey is one question about an argument list: does this element match it.
// The list is keyed by where it is held, which is the selector it was parsed
// in, and by its length, so that no two lists share a key.
type listKey struct {
	first *css.Selector
	n     int
	el    *html.Node
}

// matchesList matches the argument list of :is(), :where() or :not() with n as
// its subject, and remembers the answer.
//
// It is nesting's memo for the pseudo-classes that were not "&", and it is
// needed for the same reason. An argument is a whole selector, matched against
// every element the selector around it visits — and when the argument holds a
// descendant combinator, that is every ancestor of every element the selector
// around *it* visits, and so on in. ":is(:is(:is(.nowhere .x) .x) .x) p" on a
// paragraph under d nested div.x asked the innermost list about an ancestor
// once per way of choosing one ancestor at each level: d⁴ steps for a selector
// of four compounds, and the per-match budget was the only thing that ended it.
// Asked once per element, each level costs a walk of the ancestors of each
// element it is asked about, whatever is nested in it, and the levels add.
//
// It is safe to remember for nesting's reason: whether an element matches a
// selector list as its subject depends on the element and the tree, and neither
// changes while a document is matched. The one answer not kept is one the budget
// cut short, which is a "no" that may be wrong. A remembered answer is a step,
// so the work a rule's matching does is still counted against its budget for
// the document, and a match that spends its own budget on lookups still trips.
func (m *Matcher) matchesList(sels []css.Selector, n *html.Node) bool {
	if len(sels) == 0 {
		return false
	}
	key := listKey{first: &sels[0], n: len(sels), el: n}
	if got, ok := m.lists[key]; ok {
		if m.spent() {
			return false
		}
		return got
	}
	got := m.matchesAny(sels, n)
	if !m.over {
		m.lists[key] = got
	}
	return got
}

// childPosition is an element's one-based position among its siblings for
// :nth-child and :nth-last-child, counting from the end when last is set and,
// with "of S", counting only the siblings S selects.
//
// It returns 0 for an element with no parent, or one "of S" leaves out of the
// series, and no An+B selects 0.
func (m *Matcher) childPosition(n *html.Node, of []css.Selector, last bool) int {
	i := m.siblingIndex(n)
	if i < 0 {
		return 0
	}
	if len(of) == 0 {
		if last {
			return len(m.kids[n.Parent]) - i
		}
		return i + 1
	}
	s := m.seriesOf(n.Parent, of)
	pos := int(s.pos[i])
	if pos == 0 || !last {
		return pos
	}
	return s.count - pos + 1
}

// seriesKey is one "of S" list under one parent. The list is keyed by where it
// is held, which is the selector it was parsed in: two lists spelled alike in
// two rules are asked about separately, and one list is never asked about twice.
type seriesKey struct {
	of     *css.Selector
	parent *html.Node
}

// series is which of a parent's children an "of S" list selects: pos is each
// child's one-based position among those, zero for one it does not select, and
// count is how many it selects.
type series struct {
	pos   []int32
	count int
}

// seriesOf works out an "of S" list against every child of a parent, once.
//
// Asking S of every earlier sibling for every element is n² matches over a list
// of n, and S is a selector list, so each of them may be expensive. Asked once
// per child the whole list costs n matches, and every element after the first
// reads its position.
//
// Each child is matched with a budget of its own, as though it were the subject
// of a match — which it is — rather than on the budget of the element that
// happened to ask first. Charged to that one element, a list of more than a few
// thousand items would trip the bound on the first of them every time, and the
// work is not that element's: it is shared by all of them. It is still work,
// and it is counted: Matcher.work carries it to the rule that asked, whose
// budget for the document it comes out of. A child whose own match ran out is
// reported through Tripped like any other.
func (m *Matcher) seriesOf(parent *html.Node, of []css.Selector) *series {
	key := seriesKey{of: &of[0], parent: parent}
	if s, ok := m.series[key]; ok {
		return s
	}
	kids := m.children(parent)
	s := &series{pos: make([]int32, len(kids))}
	steps, over := m.steps, m.over
	for i, k := range kids {
		m.steps, m.over = 0, false
		if m.matchesAny(of, k) {
			s.count++
			s.pos[i] = int32(s.count)
		}
	}
	m.steps, m.over = steps, over
	m.series[key] = s
	return s
}

// typePosition is an element's one-based position among its siblings of the
// same name, counting from the end when last is set, or 0 when it has no parent.
//
// "The same name" is what compound compares with ascii.EqualFold, and
// ascii.Lower is that comparison as a key, so the positions are the ones a walk
// comparing names would find. Every child of the parent is placed the first
// time any of them is asked about.
func (m *Matcher) typePosition(n *html.Node, last bool) int {
	parent := n.Parent
	if parent == nil {
		return 0
	}
	if !m.typed[parent] {
		kids := m.children(parent)
		seen := make(map[string]int32, 4)
		keys := make([]string, len(kids))
		for i, k := range kids {
			keys[i] = ascii.Lower(k.Name)
			seen[keys[i]]++
			m.ofType[k] = [2]int32{seen[keys[i]], 0}
		}
		for i, k := range kids {
			at := m.ofType[k]
			at[1] = seen[keys[i]] - at[0] + 1
			m.ofType[k] = at
		}
		m.typed[parent] = true
		m.work += len(kids)
	}
	at, ok := m.ofType[n]
	if !ok {
		return 0
	}
	if last {
		return int(at[1])
	}
	return int(at[0])
}

func (m *Matcher) matchesAny(sels []css.Selector, n *html.Node) bool {
	for _, s := range sels {
		if m.complexFrom(s, n) {
			return true
		}
	}
	return false
}

// matchLang implements :lang(), which reads the nearest lang attribute at or
// above the element — html.Node.Language, the same rule the casing and
// hyphenation readers use, answered once per element for the document by
// m.langs — and compares it with each language range by
// RFC 4647 §3.3.2's extended filtering, as Selectors 4 §7.2 says to.
//
// It was the dash-match of attribute selectors with "*" matching anything.
// That is right for the plain cases — :lang(en) selects an element declared
// "en-GB" — and wrong in the two Level 4 added. A wildcard range matched an
// element whose language the author had marked as not known (lang=""), which
// §7.2 says it does not; and :lang("") matched nothing, where it matches
// exactly those elements. Filtering is also what lets :lang(de-DE) select
// "de-Latn-DE" and :lang("*-CH") select "fr-CH", which dash-match cannot.
func (m *Matcher) matchLang(n *html.Node, langs []string) bool {
	value, ok := m.langs.Of(n)
	// Not tagged: lang="" says so outright, and an element with no lang at or
	// above it has no tag either — nothing this engine reads (it does not
	// read HTTP headers or a Content-Language pragma) gives it one.
	untagged := !ok || value == ""
	for _, want := range langs {
		if want == "" {
			if untagged {
				return true
			}
			continue
		}
		if !untagged && extendedFilter(value, want) {
			return true
		}
	}
	return false
}

// extendedFilter is RFC 4647 §3.3.2: whether a language tag matches an
// extended language range. Both are compared a subtag at a time, ASCII
// case-insensitively. The first subtags must be equal, or the range's must be
// "*"; after that each of the range's subtags must turn up in the tag in order,
// a "*" matching nothing in particular, and the tag's subtags between them may
// be skipped — but not a single-letter one, which introduces an extension or a
// private use and ends what the range can reach.
//
// Selectors 4 §7.2 adds that a range or tag that is not well formed matches
// nothing, so ":lang(åå)" selects nothing rather than an element tagged "åå".
// Well formed is checked here as far as the shape of the subtags (one to eight
// ASCII letters or digits); the canonicalisation to extlang form §7.2 also asks
// for needs the IANA registry, which this engine does not carry, and is not
// done — a tag and a range written in the same form, which is how documents
// and stylesheets write them, compare correctly without it.
func extendedFilter(tag, rng string) bool {
	t := strings.Split(ascii.Lower(tag), "-")
	r := strings.Split(ascii.Lower(rng), "-")
	if !wellFormedSubtags(t, false) || !wellFormedSubtags(r, true) {
		return false
	}
	if r[0] != "*" && r[0] != t[0] {
		return false
	}
	i, j := 1, 1
	for i < len(r) {
		switch {
		case r[i] == "*":
			i++
		case j >= len(t):
			return false
		case r[i] == t[j]:
			i, j = i+1, j+1
		case len(t[j]) == 1:
			return false
		default:
			j++
		}
	}
	return true
}

// wellFormedSubtags reports whether every subtag is one to eight ASCII letters
// or digits — or, in a range, the wildcard "*".
func wellFormedSubtags(subtags []string, wildcard bool) bool {
	for _, st := range subtags {
		if wildcard && st == "*" {
			continue
		}
		if len(st) == 0 || len(st) > 8 {
			return false
		}
		for i := 0; i < len(st); i++ {
			if c := st[i]; !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
				return false
			}
		}
	}
	return true
}

// Tree navigation. Every one of these skips text nodes, because a selector
// speaks about elements and nothing else — "p + p" means two paragraphs with
// only text between them, not two paragraphs with nothing at all.

func parentElement(n *html.Node) *html.Node {
	if n == nil || n.Parent == nil || n.Parent.Type != html.ElementNode {
		return nil
	}
	return n.Parent
}

// children returns a parent's element children, memoized.
func (m *Matcher) children(parent *html.Node) []*html.Node {
	if parent == nil {
		return nil
	}
	if got, ok := m.kids[parent]; ok {
		return got
	}
	out := make([]*html.Node, 0, len(parent.Children))
	for _, c := range parent.Children {
		if c.Type == html.ElementNode {
			out = append(out, c)
		}
	}
	m.kids[parent] = out
	for i, c := range out {
		m.idx[c] = i
	}
	// Work, if work done once for every child rather than once per element: it
	// is counted where it is done, as every sibling walk is, so that what the
	// structural pseudo-classes cost is in the one account a rule is charged
	// from. See Matcher.work.
	m.work += len(parent.Children)
	return out
}

// siblingIndex is an element's position among its parent's element children,
// or -1 if it has no element parent.
func (m *Matcher) siblingIndex(n *html.Node) int {
	parent := n.Parent
	if parent == nil {
		return -1
	}
	m.children(parent) // fills m.idx
	if i, ok := m.idx[n]; ok {
		return i
	}
	return -1
}

func (m *Matcher) prevElement(n *html.Node) *html.Node {
	i := m.siblingIndex(n)
	if i <= 0 {
		return nil
	}
	return m.kids[n.Parent][i-1]
}

func (m *Matcher) nextElement(n *html.Node) *html.Node {
	i := m.siblingIndex(n)
	if i < 0 {
		return nil
	}
	sibs := m.kids[n.Parent]
	if i+1 >= len(sibs) {
		return nil
	}
	return sibs[i+1]
}

// htmlFoldedAttrs are the attributes whose *values* an attribute selector
// compares ASCII case-insensitively, without the "i" flag, for an element in an
// HTML document.
//
// Selectors 4 §6.3.2 hands the list to the host language and HTML §15.1 gives
// it: these are the attributes HTML itself defines as enumerated or as
// case-insensitive keywords, so "dir=RTL" and "dir=rtl" are the same value and a
// selector that told them apart would be telling apart two spellings of one
// thing.
//
// It is not a convenience. The user agent stylesheet is written in these
// selectors — "[dir=rtl]" is where a right-to-left element gets its direction,
// and "input[type=...]" is where a control gets its shape — so an author who
// wrote "RTL", which HTML allows, got a left-to-right page.
//
// Only in an HTML document. HTML states the condition as "attribute selectors on
// an HTML element in an HTML document", and in XML a document may define
// attributes of its own whose values are case-sensitive and happen to share
// these names. Every element in a tree this engine matches against is an HTML
// element — an <svg> keeps its children as unparsed source rather than as nodes
// — so the document is the whole of the condition here.
//
// That "type" is on the list is not a slip to be worked around: HTML's own user
// agent stylesheet writes "ol[type=a s]" and "ol[type=A s]", with the explicit
// case-*sensitivity* flag, precisely because the default for the attribute is
// insensitive. A reading that left the list out would make those two selectors
// the same selector and number every "<ol type=A>" in lower case.
var htmlFoldedAttrs = map[string]bool{
	"accept": true, "accept-charset": true, "align": true, "alink": true, "axis": true,
	"bgcolor": true, "charset": true, "checked": true, "clear": true,
	"codetype": true, "color": true, "compact": true, "declare": true,
	"defer": true, "dir": true, "direction": true, "disabled": true,
	"enctype": true, "face": true, "frame": true, "hreflang": true,
	"http-equiv": true, "lang": true, "language": true, "link": true,
	"media": true, "method": true, "multiple": true, "nohref": true,
	"noresize": true, "noshade": true, "nowrap": true, "readonly": true,
	"rel": true, "rev": true, "rules": true, "scope": true, "scrolling": true,
	"selected": true, "shape": true, "target": true, "text": true,
	"type": true, "valign": true, "valuetype": true, "vlink": true,
}
