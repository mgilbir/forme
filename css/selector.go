package css

import (
	"fmt"
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
)

// Selectors, from Selectors Level 4 — parsed and given a specificity here, and
// matched against a document in the layer above, which needs a document to match
// against.
//
// # The subset, and the line it is drawn on
//
// A PDF page is static. It has no pointer, no focus and no form state, and a
// selector that asks about one of those is asking a question whose answer on a
// page laid out once is "no": nothing is hovered, nothing has focus, nothing is
// selected. That is the answer a browser printing the same document gives, and
// it is the one given here.
//
// The line is *dynamism*, not familiarity. Everything the document itself
// determines is in, including the whole structural family — :nth-child(),
// :first-of-type, :empty, :root — and :link, which looks like one of the
// interactive ones and is not: whether an <a> has an href is a fact about the
// document.
//
// # Three kinds of selector this does not match, and what each is
//
// Selectors 4 separates a selector that is *invalid* from one that is valid and
// matches nothing, and the difference is the rest of the rule. An invalid
// selector invalidates its whole selector list (§3.3), so the rule is dropped;
// a valid one that matches nothing takes its own element out and leaves every
// other selector in the list standing. "a:hover, .active { }" styles .active in
// every browser. It did not here, because :hover was a parse failure, and the
// list semantics spread it to a selector that had nothing to do with it — the
// standard shape of a framework's stylesheet, dropped whole.
//
//   - A selector the medium answers "no" to — :hover, :focus, ::selection — is a
//     simple selector that parses and never matches (PseudoNever). The author is
//     told, and the finding does not claim the page is wrong, because it is not.
//   - A selector the *document* answers and this engine does not — :checked,
//     :disabled, :has() — parses too, and also matches nothing
//     (PseudoUnanswered), but that "nothing" is this engine's gap and the finding
//     says so as Unsupported. Where "matches nothing" would widen a rule rather
//     than narrow it — under :not(), or in the "of S" that decides what an
//     :nth-child() counts — the whole pseudo-class around it is unanswered too,
//     so a rule is only ever missing from elements, never applied to ones it
//     does not select.
//   - A selector no specification defines, or one the grammar forbids, is
//     invalid, as it is in a browser, and takes its list with it.
//
// :visited is the first kind, and was the first of it: see PseudoVisited.

// Combinator joins two compound selectors.
type Combinator uint8

const (
	// Descendant is the space in "a b": b anywhere inside a.
	Descendant Combinator = iota
	// Child is ">": b directly inside a.
	Child
	// NextSibling is "+": b immediately after a.
	NextSibling
	// SubsequentSibling is "~": b anywhere after a, under the same parent.
	SubsequentSibling
)

func (c Combinator) String() string {
	switch c {
	case Child:
		return ">"
	case NextSibling:
		return "+"
	case SubsequentSibling:
		return "~"
	}
	return " "
}

// AttrOp is how an attribute selector compares.
type AttrOp uint8

const (
	// AttrExists is "[a]" — the attribute is present, whatever its value.
	AttrExists AttrOp = iota
	// AttrEquals is "[a=v]".
	AttrEquals
	// AttrIncludes is "[a~=v]" — v is one of a whitespace-separated list.
	AttrIncludes
	// AttrDashMatch is "[a|=v]" — v, or v followed by "-". It exists for
	// language subtags, where "en" should match "en-GB".
	AttrDashMatch
	// AttrPrefix is "[a^=v]".
	AttrPrefix
	// AttrSuffix is "[a$=v]".
	AttrSuffix
	// AttrSubstring is "[a*=v]".
	AttrSubstring
)

func (o AttrOp) String() string {
	switch o {
	case AttrEquals:
		return "="
	case AttrIncludes:
		return "~="
	case AttrDashMatch:
		return "|="
	case AttrPrefix:
		return "^="
	case AttrSuffix:
		return "$="
	case AttrSubstring:
		return "*="
	}
	return ""
}

// Attr is one attribute selector.
type Attr struct {
	// Name is the attribute name as written. HTML lowercases attribute names,
	// so the layer that matches folds it; keeping it as written lets a
	// diagnostic quote the author.
	Name string
	Op   AttrOp
	// Value is empty when Op is AttrExists.
	Value string
	// Insensitive is the "i" flag of "[a=v i]", which asks for an
	// ASCII case-insensitive comparison of the *value*.
	Insensitive bool
	// Sensitive is the "s" flag, and it is a third state rather than the
	// absence of "i".
	//
	// It used to be recorded as Insensitive=false, on the reasoning that a
	// sensitive comparison is the default — and for an attribute of the
	// author's own it is. It is not the default for the forty-odd attributes
	// HTML defines as enumerated or as keywords, whose values a selector
	// compares ASCII case-insensitively with no flag at all, and "s" is how a
	// selector asks for the comparison those attributes do not get.
	//
	// HTML's own user agent stylesheet is the caller that needs it:
	// "ol[type=a s]" and "ol[type=A s]" are two different list numberings, and
	// with the flag dropped they are one selector.
	Sensitive bool
}

// PseudoKind names a pseudo-class this engine implements, or one of the two
// ways a valid one it does not implement is kept: PseudoNever and
// PseudoUnanswered. A name no specification defines is refused at parse time,
// so there is no kind for "unknown".
type PseudoKind uint8

const (
	// Structural pseudo-classes: everything the document's own shape decides.
	PseudoRoot PseudoKind = iota
	PseudoEmpty
	PseudoFirstChild
	PseudoLastChild
	PseudoOnlyChild
	PseudoFirstOfType
	PseudoLastOfType
	PseudoOnlyOfType
	PseudoNthChild
	PseudoNthLastChild
	PseudoNthOfType
	PseudoNthLastOfType

	// Logical combinations.
	PseudoNot
	PseudoIs
	PseudoWhere

	// PseudoLang is :lang(), which reads the document's own language
	// declaration.
	PseudoLang

	// PseudoAnyLink is :link and :any-link, both of which mean "an element with
	// an href" once :visited cannot be true.
	PseudoAnyLink

	// PseudoVisited is :visited, which matches nothing.
	//
	// It is in the subset rather than refused with the interactive ones, and the
	// difference is that this one has an answer. A document laid out here is
	// rendered from a resolver with no browsing history and no way to acquire
	// one, so no link in it has been visited — not "unknown", but no. The
	// engine already relies on exactly that: :link is implemented as :any-link,
	// which is only correct because :visited cannot be true.
	//
	// Refusing it made the engine contradict itself and cost real rendering. An
	// unknown pseudo-class invalidates the whole selector, so ":link, :visited
	// { color: inherit }" — which is how a document neutralises the UA's link
	// styling, and how sixty of the suite's documents open — was dropped
	// entirely, taking the :link half with it. The links then kept the UA blue
	// that the author had written a rule to remove.
	//
	// It is also what a browser does. :visited styling is restricted almost to
	// nothing for privacy, and a renderer that treats every link as unvisited is
	// the private answer as well as the true one.
	PseudoVisited

	// PseudoNesting is "&" in a rule nested inside another: the parent rule's
	// selector list, as a unit. Pseudo.Nest says which parent. See Nesting.
	//
	// It has no name an author can write after a colon — it is spelled "&" —
	// so it is not in pseudoClasses.
	PseudoNesting

	// PseudoNever is a valid pseudo-class or pseudo-element that a page laid out
	// once never satisfies — :hover, :focus, ::selection — and so matches
	// nothing. Name is what was written, colons and all.
	//
	// It is :visited's answer given to the rest of the family. Nobody hovers a
	// printed page, so "no" is not a guess about the element but the truth
	// about the medium, and it is the truth under :not() as much as anywhere:
	// ":not(:hover)" is every element.
	PseudoNever

	// PseudoUnanswered is a valid pseudo-class or pseudo-element whose answer
	// the document holds and this engine does not work out — ":checked" on an
	// <input checked>, ":has()" — and so matches nothing, reported as
	// Unsupported. Name is what was written.
	//
	// Unlike PseudoNever, "no" here may be wrong, so it is only given where a
	// wrong "no" narrows a rule. A :not() or an ":nth-child(… of S)" that holds
	// one anywhere inside it is itself unanswered, because matching nothing
	// inside a negation matches everything outside it. Args and Of are kept for
	// diagnostics and are never matched.
	PseudoUnanswered
)

// Pseudo is one pseudo-class in a compound selector.
type Pseudo struct {
	Kind PseudoKind
	// Name is the pseudo-class as the author wrote it, for diagnostics.
	Name string

	// AnB is set for the four :nth-* kinds.
	AnB AnB
	// Of is the "of S" of ":nth-child(An+B of S)", empty when absent.
	Of []Selector
	// Args is set for :not(), :is() and :where().
	Args []Selector
	// Langs is set for :lang().
	Langs []string
	// Nest is set for PseudoNesting, and is the parent rule "&" stands for.
	// It is shared by every "&" written against that rule, never copied.
	Nest *Nesting
}

// Nesting is a style rule as the rules nested inside it see it: the thing their
// "&" means.
//
// CSS Nesting 1 §3 makes "&" the parent's selector list *as a unit*, with the
// matching and the specificity of ":is(<that list>)". It is not text to be
// pasted in where the "&" is. It was, once — the parent's component values
// spliced into every "&" before parsing — and that is a copy of the parent per
// use, so a rule whose selector is "& & & & & & & &" is eight copies of its
// parent, each of which is eight copies of *its* parent: 162 bytes of CSS
// became gigabytes of selector at seven levels deep. Pasting also cannot say
// where one selector of the list ends, which is how ".card { h2, p { } }"
// scoped "h2" to the card and let "p" select every paragraph in the document.
//
// So the parent is parsed once, and every "&" written against it holds a
// pointer to this. A selector nested any number of levels deep is as large as
// what its author wrote.
type Nesting struct {
	// Selectors is the parent's list exactly as parsed, pseudo-elements and
	// all. It is what a declaration written directly inside a nested @media
	// applies to, which is the parent rule's own selectors and not "&" — see
	// the nested declarations rule of CSS Nesting §3.2.
	Selectors []Selector

	// matchable is the part of Selectors that "&" can stand for, and spec is
	// the specificity it contributes, both settled once here rather than per
	// use.
	matchable []Selector
	spec      Specificity

	// unanswered says some selector "&" stands for holds a PseudoUnanswered,
	// at any depth, so that an "&" under a :not() is unanswered too. It is
	// worked out once here, and a nested parent's own "&" reads its parent's,
	// so asking it never walks more than one level.
	unanswered bool
}

// NewNesting makes the parent that the rules nested in a style rule are
// relative to. Build it once per parent rule and share it: its identity is
// what a matcher may remember an answer against.
//
// A selector with a pseudo-element is left out of what "&" means. "&" is
// ":is()", and a pseudo-element is not valid inside ":is()" — Selectors 4 §4.2
// says ":is()" "cannot represent pseudo-elements" — so "div::before { & { } }"
// selects nothing, and not the div, and "*, ::before { & * { } }" has the
// specificity of "*" alone. The WPT tests
// css-nesting/contextually-invalid-selectors-001 and -003 are these two cases.
func NewNesting(parent []Selector) *Nesting {
	n := &Nesting{Selectors: parent}
	for _, s := range parent {
		if s.PseudoElement != "" {
			continue
		}
		n.matchable = append(n.matchable, s)
		n.spec = n.spec.max(s.Specificity)
	}
	n.unanswered = holdsUnanswered(n.matchable)
	return n
}

// holdsUnanswered reports whether any of sels holds a PseudoUnanswered, at any
// depth. An "&" reads its parent's answer, which was settled when the parent
// was made, rather than walking the parent again.
func holdsUnanswered(sels []Selector) bool {
	for _, s := range sels {
		for _, c := range s.Compounds {
			for _, ps := range c.Pseudos {
				switch {
				case ps.Kind == PseudoUnanswered:
					return true
				case ps.Kind == PseudoNesting && ps.Nest != nil && ps.Nest.unanswered:
					return true
				case holdsUnanswered(ps.Args), holdsUnanswered(ps.Of):
					return true
				}
			}
		}
	}
	return false
}

// Matchable is the selector list "&" stands for: an element matches "&" when it
// matches any of these. It may be empty, and then "&" matches nothing.
func (n *Nesting) Matchable() []Selector { return n.matchable }

// Specificity is what "&" contributes to a selector it is in, which is that of
// ":is()" over the same list: the most specific of them, whichever one an
// element happens to match.
func (n *Nesting) Specificity() Specificity { return n.spec }

// Compound is a run of simple selectors that all constrain the same element,
// together with the combinator joining it to the compound before it.
type Compound struct {
	// Combinator joins this compound to the one before it. It is meaningless on
	// the first compound of a selector, where it is Descendant.
	Combinator Combinator

	// Type is the element name, empty if none was written. Universal is "*".
	// Both may be absent, which is what ".c" is.
	Type      string
	Universal bool

	// IDs is every "#name" in the compound. More than one is legal and is not a
	// mistake to be corrected here: "#a#b" matches nothing, and "#a#a" matches
	// what "#a" matches while counting twice towards specificity, which is a
	// long-standing way to raise a rule's weight without touching the document.
	// Refusing either would reject stylesheets that browsers accept.
	IDs     []string
	Classes []string
	Attrs   []Attr
	Pseudos []Pseudo
}

// Selector is one complex selector: compound selectors joined by combinators.
//
// The last compound is the *subject* — the element the selector selects. That
// matters more than it looks: matching runs right to left, from the subject
// outwards, because a document has far more elements than a selector has
// compounds and the subject is the cheapest thing to reject on.
type Selector struct {
	Compounds []Compound

	// PseudoElement is "before", "after", "first-line", "first-letter" or
	// "marker", empty when there is none. It is on the selector rather than on
	// a compound because at most one may appear and only on the subject.
	PseudoElement string

	Specificity Specificity

	// Offset is the byte offset in the source at which the selector begins.
	Offset int
}

// Specificity is the (a, b, c) of Selectors Level 4 §17: identifiers, then
// classes and attributes and pseudo-classes, then element names and
// pseudo-elements. It decides which of two declarations wins when both apply.
type Specificity struct{ A, B, C int }

// Less reports whether s loses to other. The three components are compared in
// order and do not carry: a thousand classes lose to one identifier, which is
// why this is not a single number.
func (s Specificity) Less(other Specificity) bool {
	if s.A != other.A {
		return s.A < other.A
	}
	if s.B != other.B {
		return s.B < other.B
	}
	return s.C < other.C
}

func (s Specificity) String() string { return fmt.Sprintf("(%d,%d,%d)", s.A, s.B, s.C) }

// add sums two specificities, which is what building a compound does.
func (s Specificity) add(o Specificity) Specificity {
	return Specificity{s.A + o.A, s.B + o.B, s.C + o.C}
}

// max returns the more specific of two, which is what :is() and :not()
// contribute.
func (s Specificity) max(o Specificity) Specificity {
	if s.Less(o) {
		return o
	}
	return s
}

// pseudoClasses is the implemented subset, keyed by the lowercased name. A
// pseudo-class absent from here, from dynamicPseudoClasses and from
// unimplementedPseudoClasses is not a pseudo-class at all.
var pseudoClasses = map[string]PseudoKind{
	"root":             PseudoRoot,
	"empty":            PseudoEmpty,
	"first-child":      PseudoFirstChild,
	"last-child":       PseudoLastChild,
	"only-child":       PseudoOnlyChild,
	"first-of-type":    PseudoFirstOfType,
	"last-of-type":     PseudoLastOfType,
	"only-of-type":     PseudoOnlyOfType,
	"nth-child":        PseudoNthChild,
	"nth-last-child":   PseudoNthLastChild,
	"nth-of-type":      PseudoNthOfType,
	"nth-last-of-type": PseudoNthLastOfType,
	"not":              PseudoNot,
	"is":               PseudoIs,
	"matches":          PseudoIs, // the old spelling of :is()
	"where":            PseudoWhere,
	"lang":             PseudoLang,
	"link":             PseudoAnyLink,
	"any-link":         PseudoAnyLink,
	"visited":          PseudoVisited,
	// Selectors 4 §8.3: with no scoping root — and a stylesheet has none
	// outside @scope, which this engine does not apply — :scope is the root
	// element, with the specificity of a pseudo-class. That is :root exactly.
	"scope": PseudoRoot,
}

// dynamicPseudoClasses are correct CSS whose answer is not in the markup alone:
// they ask about a pointer, a keyboard, form state or browsing history, and a
// page laid out once has none of those in motion.
//
// They are listed rather than lumped in with the unknown so that the diagnostic
// can say which it is. "no such pseudo-class" sends an author looking for a
// typo; ":hover cannot apply to a printed page" tells them the truth, which is
// that the rule was understood and selects nothing here.
//
// Every one of them is a valid selector that matches nothing — see the note at
// the head of this file. Whether the finding also claims the page differs from
// the one CSS describes is a second question, and stateOnAPage is what answers
// it.
var dynamicPseudoClasses = map[string]bool{
	"active": true, "hover": true, "focus": true, "focus-visible": true,
	"focus-within": true, "target": true, "target-within": true,
	"checked": true, "indeterminate": true, "default": true, "disabled": true,
	"enabled": true, "read-only": true, "read-write": true,
	"placeholder-shown": true, "valid": true, "invalid": true,
	"in-range": true, "out-of-range": true, "required": true, "optional": true,
	"user-valid": true, "user-invalid": true, "autofill": true,
	"playing": true, "paused": true, "muted": true, "seeking": true,
	"buffering": true, "stalled": true, "fullscreen": true, "modal": true,
	"popover-open": true, "picture-in-picture": true, "current": true,
	"past": true, "future": true, "local-link": true, "defined": true,
	"host": true, "host-context": true,
}

// stateOnAPage is the half of dynamicPseudoClasses whose answer is written in
// the document rather than made by a reader.
//
// The split is the difference between PseudoNever and PseudoUnanswered. Nobody
// hovers a printed page, so ":hover" selects nothing there and a browser
// printing the same page applies it exactly as little — the page is the one CSS
// describes. But "<input disabled>" is disabled on paper as much as on screen,
// so ":disabled { color: grey }" asks for grey text that is not there, and an
// author has no way to find that out except by being told.
//
// ":defined" is on it for the same reason from the other end: with no custom
// elements every element is defined, so the rule matches *everything*, and
// matching nothing takes style from every box it named.
var stateOnAPage = map[string]bool{
	"checked": true, "indeterminate": true, "default": true,
	"disabled": true, "enabled": true, "read-only": true, "read-write": true,
	"placeholder-shown": true, "valid": true, "invalid": true,
	"in-range": true, "out-of-range": true, "required": true, "optional": true,
	"defined": true,
}

// userActionPseudoClasses are the only pseudo-classes that may follow a
// pseudo-element: Selectors 4 §3.6.3 allows "::before:hover" and nothing else of
// the kind. Every one of them is a PseudoNever here.
var userActionPseudoClasses = map[string]bool{
	"hover": true, "active": true, "focus": true, "focus-visible": true,
	"focus-within": true,
}

// unimplementedPseudoClasses are the pseudo-classes Selectors 4 defines that the
// document answers and this engine does not work out. They are PseudoUnanswered:
// valid, matching nothing, and reported as a gap. Their arguments are not read,
// since nothing here would match them.
//
// They are named so that "p:has(img), .figure" keeps ".figure", which a browser
// does and which an unknown name would not; a name that is on no list is still
// invalid, as it is in a browser, and takes its list with it.
var unimplementedPseudoClasses = map[string]bool{
	"has": true, "dir": true, "blank": true, "nth-col": true, "nth-last-col": true,
}

// pseudoElements is the implemented subset.
var pseudoElements = map[string]bool{
	"before": true, "after": true,
	"first-line": true, "first-letter": true,
	"marker": true,
}

// otherPseudoElements are the pseudo-elements CSS Pseudo 4 and its neighbours
// define that this engine does not generate, and which of the two kinds each is:
// true for one a page laid out once never has (PseudoNever), false for one a
// browser would draw on the same page and this engine does not
// (PseudoUnanswered). A pseudo-element on neither list is invalid.
//
// ::placeholder is the one whose answer is not obvious. A browser *does* draw an
// empty field's placeholder on a page nobody has touched, so a rule styling it
// asks for ink that is missing — which is a finding about this engine and not
// about the medium.
var otherPseudoElements = map[string]bool{
	// Nothing is selected, targeted or spell-checked.
	"selection": true, "target-text": true, "highlight": true,
	"spelling-error": true, "grammar-error": true,
	// There is no top layer.
	"backdrop": true,
	// A shadow tree needs scripting, which never runs here, so there is no
	// shadow tree to have a part of.
	"part": true, "slotted": true,
	// Drawn in a browser, and not here.
	"placeholder": false, "file-selector-button": false, "details-content": false,
	"cue": false, "cue-region": false,
}

// reasonForPseudoElement adds why a pseudo-element matches nothing here, when
// there is something better to say than "not implemented".
//
// The distinction is the same one dynamicPseudoClasses draws: an author whose
// ::selection rule did nothing is helped by learning that a printed page has no
// selection, and misled by a message that suggests the name was wrong.
func reasonForPseudoElement(lower string) string {
	switch lower {
	case "selection", "target-text", "highlight", "spelling-error", "grammar-error":
		return ": a page laid out once has nothing selected or highlighted"
	case "backdrop":
		return ": there is no top layer on a printed page"
	case "part", "slotted":
		return ": a shadow tree needs scripting, which this engine never runs"
	case "placeholder", "file-selector-button":
		return ": form fields are not drawn here"
	}
	return ""
}

// legacyPseudoElements may be written with one colon, because they predate the
// two-colon notation and every browser still accepts them.
var legacyPseudoElements = map[string]bool{
	"before": true, "after": true, "first-line": true, "first-letter": true,
}

// ParseSelectorList parses the prelude of a style rule into selectors.
//
// A selector that cannot be parsed — malformed, or naming a pseudo-class no
// specification defines — is dropped and reported. If *any* of them is dropped
// the whole list is invalid: that is what the specification requires, and it is
// the safe direction, because a rule whose selector list was silently narrowed
// applies to fewer elements than its author asked for. ok reports whether the
// list survived intact.
//
// A valid selector this engine does not match — ":hover", ":checked" — is not
// dropped. It is kept, it matches nothing, and it is reported; the rest of the
// list stands, as it does in a browser. See the note at the head of this file.
//
// This is a rule at the top of a stylesheet, where an "&" is the root element
// with no specificity — see selParser.nesting. A rule written inside another is
// read with ParseNestedSelectorList.
func ParseSelectorList(vals []ComponentValue) (sels []Selector, errs []Error, ok bool) {
	return parseSelectorList(vals, nil)
}

// ParseNestedSelectorList parses the prelude of a style rule written inside
// another, whose "&" is parent.
//
// CSS Nesting 1 §2.1 makes the prelude a list of *relative* selectors, and each
// one is read on its own:
//
//   - One that begins with a combinator — "> p", "+ .x" — is relative to the
//     parent through that combinator: "& > p". It is, even when an "&" appears
//     later in it, because the combinator has to be relative to something.
//   - One that contains "&" anywhere, at any depth, is taken as written: the
//     author has said where the parent goes.
//   - Any other is a descendant of the parent: "p" is "& p".
//
// Per selector, and never once for the whole list: ".card { h2, p { } }" is
// "& h2, & p", and ".card { & h2, p { } }" is "& h2, & p" too. Reading the
// list as one unit prefixed the parent to its first selector only, so the
// second selected every paragraph in the document.
//
// Whether a selector "contains &" is asked of what was written, before anything
// in it is parsed. An "&" inside an argument that a forgiving ":is()" goes on
// to drop still counts: ".x { :is(.y, !&) { } }" selects .y anywhere, and not
// only inside .x — which is the WPT test
// css-nesting/nest-containing-forgiving.
//
// A nil parent is a rule at the top of a stylesheet, and is ParseSelectorList.
func ParseNestedSelectorList(vals []ComponentValue, parent *Nesting) (sels []Selector, errs []Error, ok bool) {
	return parseSelectorList(vals, parent)
}

func parseSelectorList(vals []ComponentValue, parent *Nesting) (sels []Selector, errs []Error, ok bool) {
	p := &selParser{nest: parent}
	out, all := p.list(vals, 0)
	if p.tooDeep {
		return nil, p.errs, false
	}
	// Usability is "every selector written was understood", not "nothing was
	// reported". The two differ inside :is() and :where(), which are forgiving:
	// an argument they could not use is dropped and still reported, and the rule
	// around it stands. Tying ok to the error list would make those two fatal
	// and defeat the whole point of a forgiving selector list.
	if !all || len(out) == 0 {
		// Nothing is returned when the list is unusable, rather than the part
		// of it that parsed. A caller that forgot to check ok would otherwise
		// apply the rule to the selectors that survived — which is a rule the
		// author never wrote, narrower than the one they did, and with nothing
		// about the resulting page to say so.
		return nil, p.errs, false
	}
	return out, p.errs, true
}

// maxSelectorDepth bounds recursion through :is(), :not() and :where().
//
// The component-value tree is already capped, so this cannot be driven far by
// nesting alone; it is here because the recursion is mutual and a bound that is
// only implied by another bound is one that a later change can remove without
// noticing.
const maxSelectorDepth = 32

type selParser struct {
	errs []Error
	// nest is the parent rule "&" refers to, or nil at the top of a stylesheet.
	nest *Nesting
	// unanswered counts the PseudoUnanswered selectors read so far, including
	// an "&" whose parent holds one. A :not() or an "of S" reads it before and
	// after its argument, which is how it learns that the argument holds one
	// at any depth without walking it again.
	unanswered int
	// tooDeep says maxSelectorDepth was reached. What lay below it was not
	// read, which is this engine stopping short rather than the author
	// writing something invalid, so a forgiving list may not forgive it: the
	// whole list is refused, as it was before forgiving lists could be
	// emptied.
	tooDeep bool
}

// nesting is the simple selector "&" is parsed into.
//
// Inside a nested rule it is the parent, by reference. At the top of a
// stylesheet there is no parent, and CSS Nesting §3 makes "&" there mean
// ":scope" — which, with no scoping root, is the root element — with no
// specificity at all: the WPT tests css-nesting/top-level-is-scope and
// top-level-parent-pseudo-specificity. ":where(:root)" is exactly that, in
// terms this package already has.
func (p *selParser) nesting() Pseudo {
	if p.nest != nil {
		if p.nest.unanswered {
			p.unanswered++
		}
		return Pseudo{Kind: PseudoNesting, Name: "&", Nest: p.nest}
	}
	root := Selector{
		Compounds:   []Compound{{Pseudos: []Pseudo{{Kind: PseudoRoot, Name: "root"}}}},
		Specificity: Specificity{0, 1, 0},
	}
	return Pseudo{Kind: PseudoWhere, Name: "&", Args: []Selector{root}}
}

// containsNesting reports whether an "&" was written anywhere in vals, at any
// depth — inside ":not(&)" as much as at the top.
func containsNesting(vals []ComponentValue) bool {
	for _, v := range vals {
		if v.IsToken() && v.Token.IsDelim('&') {
			return true
		}
		if len(v.Values) > 0 && containsNesting(v.Values) {
			return true
		}
	}
	return false
}

func (p *selParser) fail(off int, msg string) {
	p.add(Error{Offset: off, Message: msg})
}

func (p *selParser) unsupported(off int, msg string) {
	p.add(Error{Offset: off, Message: msg, Unsupported: true})
}

// inapplicable reports a selector that is correct CSS and matches nothing here,
// which is a different thing from one this engine has not implemented.
//
// ":hover" on a page laid out once is not a rule that was dropped. It is a rule
// whose condition is false — there is no pointer, so nothing is hovered, and a
// browser showing the same page unhovered applies it exactly as little. The same
// goes for "::selection" over a page with no selection. The author is still told,
// because a rule that did nothing is worth knowing about; what is not claimed is
// that the page differs from the one CSS describes.
//
// Unsupported is what that claim is made with, and it is what the reftest
// ratchet reads: a document whose only finding is one of these is rendered
// exactly as it should be, and counting it as vacuous hid four of the suite's
// tests behind a rule that changed nothing.
//
// That claim is only true because the selector is kept, as a PseudoNever, and
// the rest of its rule's list still applies. When it was a parse failure,
// "a:hover, .active" lost ".active" too, and a page missing the style of every
// .active element was counted as rendered exactly as it should be.
func (p *selParser) inapplicable(off int, msg string) {
	p.add(Error{Offset: off, Message: msg})
}

func (p *selParser) add(e Error) {
	switch {
	case len(p.errs) > maxErrors:
		return
	case len(p.errs) == maxErrors:
		p.errs = append(p.errs, Error{
			Offset:  e.Offset,
			Message: "further problems in this stylesheet were not reported",
		})
	default:
		p.errs = append(p.errs, e)
	}
}

// list splits on top-level commas and parses each complex selector. all reports
// whether every one of them survived, which is what the callers that are not
// forgiving need to know.
func (p *selParser) list(vals []ComponentValue, depth int) (sels []Selector, all bool) {
	if depth > maxSelectorDepth {
		if !p.tooDeep {
			p.fail(offsetOf(vals), "selectors are nested too deeply to read")
		}
		p.tooDeep = true
		return nil, false
	}
	parts := splitOnComma(vals)
	var out []Selector
	for _, part := range parts {
		if s, ok := p.complex(part, depth); ok {
			out = append(out, s)
		}
	}
	return out, len(out) == len(parts)
}

// splitOnComma divides a selector list. The commas are at the top level by
// construction: one inside a function or a block belongs to that function.
func splitOnComma(vals []ComponentValue) [][]ComponentValue {
	var out [][]ComponentValue
	start := 0
	for i, v := range vals {
		if v.IsToken() && v.Token.Kind == Comma {
			out = append(out, vals[start:i])
			start = i + 1
		}
	}
	return append(out, vals[start:])
}

func offsetOf(vals []ComponentValue) int {
	if len(vals) > 0 {
		return vals[0].Token.Offset
	}
	return 0
}

// complex parses one complex selector: compounds joined by combinators.
func (p *selParser) complex(vals []ComponentValue, depth int) (Selector, bool) {
	vals = trimWhitespace(vals)
	if len(vals) == 0 {
		p.fail(offsetOf(vals), "an empty selector")
		return Selector{}, false
	}

	out := Selector{Offset: vals[0].Token.Offset}
	combinator := Descendant
	i := 0
	// written is the pseudo-element a compound ended with, as written — one this
	// engine generates, which is also out.PseudoElement, or one that matches
	// nothing here, which is not — for the rule that nothing may follow it.
	written := ""

	// A selector of a nested rule's own list is relative to the parent — see
	// ParseNestedSelectorList for the three cases. Only at the top of that
	// list: a selector inside ":is()" is not relative to anything, and "&"
	// written there is where the author put it.
	if p.nest != nil && depth == 0 {
		if leads := isCombinatorAt(vals, 0); leads || !containsNesting(vals) {
			out.Compounds = append(out.Compounds, Compound{Pseudos: []Pseudo{p.nesting()}})
			if leads {
				var ok bool
				combinator, i, ok = p.combinator(vals, 0)
				if !ok {
					return Selector{}, false
				}
				if i >= len(vals) {
					p.fail(vals[len(vals)-1].Token.Offset,
						"the selector is only the combinator \""+combinator.String()+"\"")
					return Selector{}, false
				}
			}
		}
	}

	for i < len(vals) {
		// A run of simple selectors, up to the next combinator.
		end := i
		for end < len(vals) && !isCombinatorAt(vals, end) {
			end++
		}
		if end == i {
			p.fail(vals[i].Token.Offset, "a combinator with nothing before it")
			return Selector{}, false
		}

		if written != "" {
			// Only the subject may carry one, so anything after it is an error
			// rather than a second pseudo-element.
			p.fail(vals[i].Token.Offset,
				"nothing may follow the pseudo-element ::"+written)
			return Selector{}, false
		}

		c, pseudoElem, wrote, ok := p.compound(vals[i:end], depth)
		if !ok {
			return Selector{}, false
		}
		c.Combinator = combinator

		if wrote != "" && depth > 0 {
			// Selectors 4 §4.2: :is() "cannot represent pseudo-elements", and
			// nor can :where(), :not() or the "of S" of :nth-child(). Accepted,
			// "div:is(::before)" was a selector whose argument had nothing
			// left in it but a pseudo-element the matcher never looks at, so
			// it selected every div; ":is(*, ::before)" counted the
			// pseudo-element's specificity; and "p:not(p::after)" selected no
			// paragraph. A browser treats the argument as invalid, and so does
			// this: a forgiving list drops it, the other two are invalid.
			p.fail(vals[i].Token.Offset, "the pseudo-element ::"+wrote+
				" cannot be the argument of a pseudo-class")
			return Selector{}, false
		}
		written = wrote
		out.PseudoElement = pseudoElem
		out.Compounds = append(out.Compounds, c)

		i = end
		if i >= len(vals) {
			break
		}

		// Read the combinator, and the whitespace around it.
		combinator, i, ok = p.combinator(vals, i)
		if !ok {
			return Selector{}, false
		}
		if i >= len(vals) {
			p.fail(vals[len(vals)-1].Token.Offset,
				"the selector ends with the combinator \""+combinator.String()+"\"")
			return Selector{}, false
		}
	}

	out.Specificity = specificityOf(out)
	return out, true
}

// onlyWhitespace reports whether a function's arguments are empty, which is not
// the same as unusable.
func onlyWhitespace(vals []ComponentValue) bool {
	for _, v := range vals {
		if !v.IsToken() || v.Token.Kind != Whitespace {
			return false
		}
	}
	return true
}

// isPseudoAt reports whether the value at i begins a pseudo-class, which is the
// one kind of thing that may follow a pseudo-element — and of those, only the
// user-action ones: see pseudo.
func isPseudoAt(vals []ComponentValue, i int) bool {
	return i < len(vals) && vals[i].IsToken() && vals[i].Token.Kind == Colon
}

// isCombinatorAt reports whether a combinator begins at i. Whitespace counts,
// because the descendant combinator *is* whitespace — but only whitespace that
// separates two compounds, which the caller settles by trimming the ends first.
func isCombinatorAt(vals []ComponentValue, i int) bool {
	v := vals[i]
	if !v.IsToken() {
		return false
	}
	switch v.Token.Kind {
	case Whitespace:
		return true
	case Delim:
		return v.Token.IsDelim('>') || v.Token.IsDelim('+') || v.Token.IsDelim('~')
	}
	return false
}

// combinator reads one combinator and any whitespace around it, returning where
// the next compound begins.
//
// Whitespace is only the descendant combinator when nothing else is there:
// "a > b" has a space on each side of the ">" and is one child combinator, not
// three combinators.
func (p *selParser) combinator(vals []ComponentValue, i int) (Combinator, int, bool) {
	sawSpace := false
	out := Descendant
	explicit := false

	for i < len(vals) && isCombinatorAt(vals, i) {
		t := vals[i].Token
		if t.Kind == Whitespace {
			sawSpace = true
			i++
			continue
		}
		if explicit {
			p.fail(t.Offset, "two combinators in a row")
			return out, i, false
		}
		switch {
		case t.IsDelim('>'):
			out = Child
		case t.IsDelim('+'):
			out = NextSibling
		case t.IsDelim('~'):
			out = SubsequentSibling
		}
		explicit = true
		i++
	}
	if !explicit && !sawSpace {
		p.fail(offsetOf(vals[i:]), "expected a combinator")
		return out, i, false
	}
	return out, i, true
}

func trimWhitespace(vals []ComponentValue) []ComponentValue {
	for len(vals) > 0 && vals[0].IsToken() && vals[0].Token.Kind == Whitespace {
		vals = vals[1:]
	}
	for len(vals) > 0 && vals[len(vals)-1].IsToken() && vals[len(vals)-1].Token.Kind == Whitespace {
		vals = vals[:len(vals)-1]
	}
	return vals
}

// compound parses one compound selector — the simple selectors that all
// constrain the same element — and the pseudo-element that may follow it.
//
// It returns the pseudo-element twice. elem is the one this engine generates,
// which is what the selector's PseudoElement becomes; written is whichever was
// written, including one that matches nothing here, which is what the grammar
// around it is checked against.
func (p *selParser) compound(vals []ComponentValue, depth int) (out Compound, elem, written string, ok bool) {
	i := 0

	// A type or universal selector, if present, must come first.
	if len(vals) > 0 && vals[0].IsToken() {
		switch t := vals[0].Token; {
		case t.Kind == Ident:
			out.Type = t.Value
			i = 1
		case t.IsDelim('*'):
			out.Universal = true
			i = 1
		case t.IsDelim('|'):
			p.unsupported(t.Offset, "namespaces in selectors are not implemented")
			return out, "", "", false
		}
	}

	for i < len(vals) {
		v := vals[i]

		// A pseudo-element ends its compound. Selectors 4 §3.3: it "must appear
		// after the compound selector" and only a pseudo-class from a short
		// list may follow it — no class, no id, no attribute, no element name.
		//
		// The check across a combinator was here and the one inside a compound
		// was not, so "a::before.foo" was accepted and then *reordered*: the
		// class was collected into the same compound as the pseudo-element and
		// matched against the element itself, so the rule applied to every <a>
		// of that class rather than to none of them. A pseudo-element is not
		// something a class can narrow. Which pseudo-classes may follow is
		// pseudo's to say.
		if written != "" && !isPseudoAt(vals, i) {
			p.fail(v.Token.Offset, "nothing may follow the pseudo-element ::"+
				written+" in the same compound selector")
			return out, "", "", false
		}

		// A namespace separator anywhere makes this a qualified name.
		if v.IsToken() && v.Token.IsDelim('|') {
			p.unsupported(v.Token.Offset, "namespaces in selectors are not implemented")
			return out, "", "", false
		}

		if v.IsBlock() && v.Token.Kind == LeftSquare {
			a, ok := p.attribute(v)
			if !ok {
				return out, "", "", false
			}
			out.Attrs = append(out.Attrs, a)
			i++
			continue
		}

		if !v.IsToken() && !v.IsFunction() {
			p.fail(v.Token.Offset, "unexpected "+v.Token.String()+" in a selector")
			return out, "", "", false
		}

		t := v.Token
		switch {
		case t.Kind == Hash:
			if !t.IsID {
				p.fail(t.Offset, "\"#"+t.Value+"\" is a colour, not an identifier selector")
				return out, "", "", false
			}
			out.IDs = append(out.IDs, t.Value)
			i++

		case t.IsDelim('.'):
			if i+1 >= len(vals) || !vals[i+1].IsToken() || vals[i+1].Token.Kind != Ident {
				p.fail(t.Offset, "expected a class name after \".\"")
				return out, "", "", false
			}
			out.Classes = append(out.Classes, vals[i+1].Token.Value)
			i += 2

		case t.IsDelim('&'):
			// The nesting selector, which is a simple selector like any other:
			// "div&", "&.x" and "&&" are all compounds. It may not come before a
			// type selector — "&div" — and that falls out of the type being
			// read only at the start, as for ".x div".
			out.Pseudos = append(out.Pseudos, p.nesting())
			i++

		case t.Kind == Colon:
			var ok bool
			var gen, wrote string
			i, gen, wrote, ok = p.pseudo(vals, i, &out, depth, written)
			if !ok {
				return out, "", "", false
			}
			if wrote != "" {
				if written != "" {
					p.fail(t.Offset, "a second pseudo-element, ::"+wrote)
					return out, "", "", false
				}
				elem, written = gen, wrote
			}

		case t.Kind == Ident:
			p.fail(t.Offset, "an element name must come first in \""+t.Value+"\"")
			return out, "", "", false

		default:
			p.fail(t.Offset, "unexpected "+t.String()+" in a selector")
			return out, "", "", false
		}
	}
	return out, elem, written, true
}

// pseudo parses a pseudo-class or pseudo-element beginning at the colon in
// vals[i], adding a pseudo-class to out. It returns the index after it and, for
// a pseudo-element, the name of the one this engine generates (elem) and the
// name that was written (written) — which differ for one that matches nothing
// here, whose elem is empty.
//
// after is the pseudo-element that came earlier in the compound, if one did:
// then only a user-action pseudo-class may follow.
func (p *selParser) pseudo(vals []ComponentValue, i int, out *Compound, depth int,
	after string) (next int, elem, written string, ok bool) {

	colon := vals[i].Token
	i++

	// A second colon means a pseudo-element.
	element := false
	if i < len(vals) && vals[i].IsToken() && vals[i].Token.Kind == Colon {
		element = true
		i++
	}
	if i >= len(vals) {
		p.fail(colon.Offset, "expected a name after \":\"")
		return i, "", "", false
	}

	v := vals[i]
	if !v.IsToken() && !v.IsFunction() {
		p.fail(colon.Offset, "expected a name after \":\"")
		return i, "", "", false
	}
	name := v.Token.Value
	lower := ascii.Lower(name)

	// One colon may still be a pseudo-element, for the four that predate the
	// two-colon notation.
	if element || (!v.IsFunction() && legacyPseudoElements[lower]) {
		if pseudoElements[lower] {
			if v.IsFunction() {
				p.unsupported(colon.Offset, "the pseudo-element ::"+name+"() is not implemented")
				return i, "", "", false
			}
			return i + 1, lower, lower, true
		}
		never, known := otherPseudoElements[lower]
		if !known {
			p.unsupported(colon.Offset, "the pseudo-element ::"+name+" is not implemented")
			return i, "", "", false
		}
		// A pseudo-element this engine generates no box or style for. It is a
		// valid selector all the same — which is the whole difference between
		// "p::selection, .x" keeping ".x" and losing it — and it selects
		// nothing: the compound it ends is given a pseudo-class that never
		// matches, and the selector carries no pseudo-element for the cascade
		// to compute.
		kind := PseudoUnanswered
		msg := "the pseudo-element ::" + name + " is not implemented" +
			reasonForPseudoElement(lower) + ", so the selector matches nothing"
		if never {
			kind = PseudoNever
			p.inapplicable(colon.Offset, msg)
		} else {
			p.unanswered++
			p.unsupported(colon.Offset, msg)
		}
		out.Pseudos = append(out.Pseudos, Pseudo{Kind: kind, Name: "::" + lower})
		return i + 1, "", lower, true
	}

	if after != "" && !userActionPseudoClasses[lower] {
		// Selectors 4 §3.6.3. What was accepted here instead was moved: the
		// pseudo-class was collected into the compound the pseudo-element ends,
		// and matched against the element, so "a::before:first-child" styled
		// the ::before of every <a> that is a first child — a rule that in a
		// browser styles nothing, because it is not a selector at all.
		p.fail(colon.Offset, "\":"+name+"\" may not follow the pseudo-element ::"+
			after+"; only a user-action pseudo-class such as :hover may")
		return i, "", "", false
	}

	kind, known := pseudoClasses[lower]
	if !known {
		return p.unimplementedPseudoClass(v, i, colon, name, lower, out)
	}

	ps := Pseudo{Kind: kind, Name: lower}

	switch kind {
	case PseudoNthChild, PseudoNthLastChild, PseudoNthOfType, PseudoNthLastOfType:
		if !v.IsFunction() {
			p.fail(colon.Offset, "\":"+name+"\" needs an An+B in parentheses")
			return i, "", "", false
		}
		before := p.unanswered
		anb, of, ok := p.nth(v, lower, depth)
		if !ok {
			return i, "", "", false
		}
		ps.AnB, ps.Of = anb, of
		if p.unanswered > before {
			// "of S" decides which siblings are counted, so a selector in it
			// that answers "no" where the document says "yes" moves every
			// position after it, and the rule lands on elements it does not
			// select. Nothing it says can be used.
			ps.Kind = PseudoUnanswered
		}

	case PseudoNot, PseudoIs, PseudoWhere:
		if !v.IsFunction() {
			p.fail(colon.Offset, "\":"+name+"\" needs a selector list in parentheses")
			return i, "", "", false
		}
		if kind != PseudoNot && onlyWhitespace(v.Values) {
			// Written with nothing in it, which is *valid* for the two
			// forgiving ones. Selectors 4 §3.5 makes the argument a forgiving
			// selector list, and an empty one is a list of one unknown
			// selector: it matches nothing, and ":is()" is a selector all the
			// same.
			//
			// The difference is the rest of the list. A style rule's selector
			// list is all-or-nothing — one invalid selector invalidates the
			// lot, which is the specification's own rule — so refusing ":is()"
			// took "p" down with it in "p, q:is()", and a stylesheet using the
			// forgiving construct the way it is meant to be used lost rules
			// that had nothing to do with it.
			//
			// Asked before the list is parsed, because parsing an empty one
			// reports an empty selector — which is right for a rule's own
			// prelude and is not what this is.
			ps.Args = nil
			out.Pseudos = append(out.Pseudos, ps)
			return i + 1, "", "", true
		}
		before := p.unanswered
		args, all := p.list(v.Values, depth+1)
		// :is() and :where() are forgiving: an argument they cannot use is
		// dropped and the rest stand. :not() is not — the specification says an
		// invalid argument makes it invalid, and it has to be that way round,
		// because dropping an argument from :not() *widens* what the rule
		// matches. Being forgiving there would apply a style to elements the
		// author explicitly excluded.
		if kind == PseudoNot && !all {
			p.fail(colon.Offset, "\":not()\" has an argument this engine cannot use, "+
				"which would widen what the rule matches")
			return i, "", "", false
		}
		// A forgiving list whose every argument was dropped is the empty one
		// above by another route: it matches nothing and is still a selector,
		// as it is in a browser. The arguments were each reported as they were
		// dropped. Refusing it instead took the rest of the rule's list with
		// it — "p, q:is(::before)" lost "p".
		ps.Args = args
		if kind == PseudoNot && p.unanswered > before {
			// "Matches nothing" is a safe wrong answer only where it narrows a
			// rule, and under a negation it widens one: ":not(:checked)" would
			// select every checked box. So the negation is unanswered too, and
			// the rule is missing from elements rather than on ones it
			// excludes.
			ps.Kind = PseudoUnanswered
		}

	case PseudoLang:
		if !v.IsFunction() {
			p.fail(colon.Offset, "\":lang()\" needs a language in parentheses")
			return i, "", "", false
		}
		langs, ok := p.langs(v)
		if !ok {
			return i, "", "", false
		}
		ps.Langs = langs

	default:
		if v.IsFunction() {
			p.fail(colon.Offset, "\":"+name+"\" takes no arguments")
			return i, "", "", false
		}
	}

	out.Pseudos = append(out.Pseudos, ps)
	return i + 1, "", "", true
}

// functionalPseudoClasses says, for the pseudo-classes this engine keeps without
// matching, which are written as functions: true for one that must be, false
// for one that may be either. One absent takes no arguments, and written with
// them is invalid, as it is in a browser.
var functionalPseudoClasses = map[string]bool{
	"has": true, "dir": true, "nth-col": true, "nth-last-col": true,
	"host-context": true, "host": false,
}

// unimplementedPseudoClass reads a pseudo-class this engine does not match: a
// dynamic one, which is kept as PseudoNever or PseudoUnanswered, one of
// unimplementedPseudoClasses, which is kept as PseudoUnanswered, or a name no
// specification gives, which is invalid.
//
// v is the name or function at vals[i], and what is returned is pseudo's.
func (p *selParser) unimplementedPseudoClass(v ComponentValue, i int, colon Token,
	name, lower string, out *Compound) (int, string, string, bool) {

	dynamic, unimplemented := dynamicPseudoClasses[lower], unimplementedPseudoClasses[lower]
	if !dynamic && !unimplemented {
		p.unsupported(colon.Offset, "the pseudo-class \":"+name+"\" is not implemented")
		return i, "", "", false
	}
	if fn, listed := functionalPseudoClasses[lower]; (listed && fn && !v.IsFunction()) ||
		(!listed && v.IsFunction()) {
		p.fail(colon.Offset, "\":"+name+"\" is not written that way")
		return i, "", "", false
	}

	switch {
	case dynamic && !stateOnAPage[lower]:
		// Nobody hovers a printed page. See inapplicable.
		p.inapplicable(colon.Offset, "\":"+name+"\" depends on how a document is "+
			"being interacted with, which a page laid out once cannot know, so it "+
			"matches nothing here")
		out.Pseudos = append(out.Pseudos, Pseudo{Kind: PseudoNever, Name: lower})
	case dynamic:
		// The markup answers this one, and this engine does not: the rule is one
		// an author will not see and should be told about as a gap rather than
		// as a property of the medium.
		p.unanswered++
		p.unsupported(colon.Offset, "\":"+name+"\" is answered by the document, "+
			"which this engine does not read for it, so it matches nothing here")
		out.Pseudos = append(out.Pseudos, Pseudo{Kind: PseudoUnanswered, Name: lower})
	default:
		p.unanswered++
		p.unsupported(colon.Offset, "the pseudo-class \":"+name+"\" is not "+
			"implemented, so it matches nothing here")
		out.Pseudos = append(out.Pseudos, Pseudo{Kind: PseudoUnanswered, Name: lower})
	}
	return i + 1, "", "", true
}

// nth parses the argument of :nth-child() and its three siblings, which is an
// An+B optionally followed by "of" and a selector list.
func (p *selParser) nth(fn ComponentValue, name string, depth int) (AnB, []Selector, bool) {
	args := fn.Values

	// "of" splits the argument, and it is an identifier at the top level.
	split := -1
	for i, v := range args {
		if v.IsToken() && v.Token.Kind == Ident && ascii.EqualFold(v.Token.Value, "of") {
			split = i
			break
		}
	}
	rest := args
	var of []Selector
	if split >= 0 {
		rest = args[:split]
		if name != "nth-child" && name != "nth-last-child" {
			p.fail(fn.Token.Offset, "\":"+name+"\" does not take \"of\"")
			return AnB{}, nil, false
		}
		var all bool
		of, all = p.list(args[split+1:], depth+1)
		// "of S" narrows which elements are counted, so dropping one of its
		// selectors changes the indices every other part of the rule depends on.
		// It is not forgiving.
		if !all || len(of) == 0 {
			p.fail(fn.Token.Offset, "\":"+name+"\" has no usable selector after \"of\"")
			return AnB{}, nil, false
		}
	}

	anb, ok := ParseAnB(rest)
	if !ok {
		p.fail(fn.Token.Offset, "\":"+name+"\" needs an An+B, such as 2n+1 or odd")
		return AnB{}, nil, false
	}
	return anb, of, true
}

// langs parses the argument of :lang(), which is one or more language ranges,
// written as identifiers or strings.
func (p *selParser) langs(fn ComponentValue) ([]string, bool) {
	var out []string
	for _, part := range splitOnComma(fn.Values) {
		part = trimWhitespace(part)
		if len(part) != 1 || !part[0].IsToken() {
			p.fail(fn.Token.Offset, "\":lang()\" takes language names, such as :lang(en)")
			return nil, false
		}
		t := part[0].Token
		if t.Kind != Ident && t.Kind != String {
			p.fail(t.Offset, "\":lang()\" takes language names, such as :lang(en)")
			return nil, false
		}
		out = append(out, t.Value)
	}
	if len(out) == 0 {
		p.fail(fn.Token.Offset, "\":lang()\" needs a language")
		return nil, false
	}
	return out, true
}

// attribute parses one "[...]" selector.
func (p *selParser) attribute(block ComponentValue) (Attr, bool) {
	vals := trimWhitespace(block.Values)
	if len(vals) == 0 {
		p.fail(block.Token.Offset, "an empty attribute selector")
		return Attr{}, false
	}

	// The name. A namespace may precede it as "ns|", "*|" or a bare "|", and
	// each has to be told from a malformed name — an author who wrote a
	// namespace wrote correct CSS this engine does not implement, and saying
	// "expected an attribute name" sends them hunting for a typo.
	if vals[0].IsToken() && (vals[0].Token.IsDelim('|') ||
		(vals[0].Token.IsDelim('*') && len(vals) > 1 &&
			vals[1].IsToken() && vals[1].Token.IsDelim('|'))) {
		p.unsupported(vals[0].Token.Offset, "namespaces in selectors are not implemented")
		return Attr{}, false
	}
	if !vals[0].IsToken() || vals[0].Token.Kind != Ident {
		p.fail(vals[0].Token.Offset, "expected an attribute name")
		return Attr{}, false
	}
	out := Attr{Name: vals[0].Token.Value}
	vals = trimWhitespace(vals[1:])

	if len(vals) == 0 {
		return out, true // "[a]"
	}

	// A namespace separator between the name and the operator.
	if vals[0].IsToken() && vals[0].Token.IsDelim('|') &&
		!(len(vals) > 1 && vals[1].IsToken() && vals[1].Token.IsDelim('=')) {
		p.unsupported(vals[0].Token.Offset, "namespaces in selectors are not implemented")
		return Attr{}, false
	}

	// The operator. All but "=" are two delimiters, because the current
	// specification has no single token for them.
	op, n, ok := attrOpAt(vals)
	if !ok {
		p.fail(vals[0].Token.Offset, "expected =, ~=, |=, ^=, $= or *= after the attribute name")
		return Attr{}, false
	}
	out.Op = op
	vals = trimWhitespace(vals[n:])

	// The value.
	if len(vals) == 0 {
		p.fail(block.Token.Offset, "the attribute selector has no value after \""+op.String()+"\"")
		return Attr{}, false
	}
	if !vals[0].IsToken() || (vals[0].Token.Kind != Ident && vals[0].Token.Kind != String) {
		p.fail(vals[0].Token.Offset, "an attribute value must be a name or a quoted string")
		return Attr{}, false
	}
	out.Value = vals[0].Token.Value
	vals = trimWhitespace(vals[1:])

	// The optional case flag.
	if len(vals) == 0 {
		return out, true
	}
	if len(vals) > 1 || !vals[0].IsToken() || vals[0].Token.Kind != Ident {
		p.fail(vals[0].Token.Offset, "unexpected extra content in an attribute selector")
		return Attr{}, false
	}
	switch ascii.Lower(vals[0].Token.Value) {
	case "i":
		out.Insensitive = true
	case "s":
		out.Sensitive = true
	default:
		p.fail(vals[0].Token.Offset, "expected \"i\" or \"s\" after the attribute value")
		return Attr{}, false
	}
	return out, true
}

// attrOpAt reads the comparison operator, returning how many values it spans.
func attrOpAt(vals []ComponentValue) (AttrOp, int, bool) {
	if !vals[0].IsToken() {
		return 0, 0, false
	}
	if vals[0].Token.IsDelim('=') {
		return AttrEquals, 1, true
	}
	if len(vals) < 2 || !vals[1].IsToken() || !vals[1].Token.IsDelim('=') {
		return 0, 0, false
	}
	switch {
	case vals[0].Token.IsDelim('~'):
		return AttrIncludes, 2, true
	case vals[0].Token.IsDelim('|'):
		return AttrDashMatch, 2, true
	case vals[0].Token.IsDelim('^'):
		return AttrPrefix, 2, true
	case vals[0].Token.IsDelim('$'):
		return AttrSuffix, 2, true
	case vals[0].Token.IsDelim('*'):
		return AttrSubstring, 2, true
	}
	return 0, 0, false
}

// specificityOf computes (a, b, c) per Selectors Level 4 §17.
func specificityOf(s Selector) Specificity {
	var out Specificity
	for _, c := range s.Compounds {
		out = out.add(compoundSpecificity(c))
	}
	if s.PseudoElement != "" {
		out.C++
	}
	return out
}

func compoundSpecificity(c Compound) Specificity {
	var out Specificity
	out.A += len(c.IDs)
	out.B += len(c.Classes) + len(c.Attrs)
	if c.Type != "" {
		out.C++
	}
	// The universal selector contributes nothing, which is why "*" loses to
	// every other selector rather than tying with a type.
	for _, ps := range c.Pseudos {
		out = out.add(pseudoSpecificity(ps))
	}
	return out
}

func pseudoSpecificity(ps Pseudo) Specificity {
	switch ps.Kind {
	case PseudoWhere:
		// :where() is the whole point of :where(): it contributes nothing, so a
		// rule can be broadly targeted and still easy to override.
		return Specificity{}

	case PseudoNot, PseudoIs:
		// The specificity of the most specific argument, and nothing for the
		// pseudo-class itself.
		return mostSpecific(ps.Args)

	case PseudoNthChild, PseudoNthLastChild:
		// A pseudo-class, plus the most specific of the "of" list.
		return Specificity{0, 1, 0}.add(mostSpecific(ps.Of))

	case PseudoNesting:
		// ":is()" over the parent's list, worked out once when the parent was.
		if ps.Nest == nil {
			return Specificity{}
		}
		return ps.Nest.Specificity()

	case PseudoNever, PseudoUnanswered:
		// Matching nothing does not change what was written, and an :is()
		// around one of these takes its specificity whichever argument
		// matched. A pseudo-element counts as one; a :not() or an
		// ":nth-child(of S)" that was unanswered for what it holds counts as
		// it would have.
		switch {
		case strings.HasPrefix(ps.Name, "::"):
			return Specificity{0, 0, 1}
		case ps.Of != nil:
			return Specificity{0, 1, 0}.add(mostSpecific(ps.Of))
		case ps.Args != nil:
			return mostSpecific(ps.Args)
		}
	}
	return Specificity{0, 1, 0}
}

func mostSpecific(sels []Selector) Specificity {
	var out Specificity
	for _, s := range sels {
		out = out.max(s.Specificity)
	}
	return out
}
