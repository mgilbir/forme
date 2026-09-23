package style

import (
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// The cascade: deciding which declaration wins when several apply, and what an
// element's value is when none does.
//
// This is the part of styling that fails *silently* when it is wrong. A
// mismatched selector shows up as a rule that does nothing, which an author
// notices; a cascade that orders two declarations wrongly produces a page where
// the wrong one won, which looks like a design decision. So the ordering below
// follows CSS Cascade Level 4 §6 exactly, and the tests assert which declaration
// won rather than that a value was produced.

// Origin is where a stylesheet came from. It is the first and strongest term in
// the cascade, ahead of specificity — an author's ordinary rule beats a user
// agent's however specific the latter is.
type Origin uint8

const (
	// OriginUserAgent is this engine's own default stylesheet: what makes <p> a
	// block and <b> bold. It loses to everything.
	OriginUserAgent Origin = iota
	// OriginUser is a stylesheet the caller supplies on behalf of the reader.
	OriginUser
	// OriginAuthor is the document's own CSS — its <style> elements and the
	// stylesheets it links.
	OriginAuthor
)

// A Sheet is a stylesheet together with where it came from.
type Sheet struct {
	Origin Origin
	Rules  []css.Rule
	// Name says which stylesheet this is, for a finding to point at. It is
	// whatever the caller called it — a URL, a file name — and is empty for the
	// document's own <style>, which has no name to give.
	Name string
}

// A Finding is something the styling stage noticed and a caller should hear
// about.
//
// It is the same shape as the css and html packages' Error, and for the same
// reason: an author needs to tell "I wrote this wrongly" from "this engine does
// not do that". Layout turns each into one of its own Findings, under the rule
// ruleForStyleFinding chooses, so it carries what that needs: where, what, and
// whether it is unsupported or malformed.
type Finding struct {
	// Offset is the byte offset the finding came from, in whatever Sheet and
	// InMarkup say it is an offset into. It is -1 for a finding about the
	// styling as a whole, which is in no file at all — reporting one of those
	// at byte nought of a stylesheet sends an author to the top of a file to
	// look for something that is not there.
	Offset int
	// Sheet names the stylesheet Offset is in. It is empty for the document's
	// own <style>, which the caller supplies without a name, and for a finding
	// that is not in a stylesheet.
	//
	// A document is styled by several sheets — its own, every <link>, and
	// everything those @import — and a byte offset means nothing without the
	// one it is into. The stage that turns these into the caller's findings
	// cannot recover it: by the time it runs, the sheets have been prepared
	// into one list and the rule no longer says where it came from.
	Sheet string
	// InMarkup says Offset is a byte offset into the *document* rather than
	// into a stylesheet, because the declaration was written in a style
	// attribute. Pointing an author at "byte 412 of the stylesheet" for a
	// declaration in the markup sends them to the wrong file.
	InMarkup bool
	// Message says what happened.
	Message string
	// Unsupported marks correct CSS this engine does not implement — what
	// layout reports as unsupported-property — as against a stylesheet that is
	// malformed.
	Unsupported bool
	// Property is the declaration's name, when the finding is about one.
	Property string
}

// maxFindings bounds the report, so a stylesheet full of properties this engine
// does not implement produces a list a person can read.
const maxFindings = 200

// A declaration that matched an element, with everything the cascade sorts on.
type candidate struct {
	property string
	value    []css.ComponentValue
	// text is value serialised, which is what a winner is stored as.
	text      string
	important bool
	origin    Origin
	// layer is the cascade layer the declaration was written in — see layer.go.
	layer int
	spec  css.Specificity
	// order is the position of the declaration in the whole input, which breaks
	// the remaining ties. Two declarations that are equal in every other term
	// are decided by which was written later, so this has to be a single
	// sequence across all sheets rather than an index within one.
	//
	// It is numbered from zero across the real declarations, which leaves the
	// negative numbers free for the things that are ordered against them
	// without being in a stylesheet at all — see hintOrder.
	order int
	// offset is where it was written, for diagnostics.
	offset int
}

// Styler applies stylesheets to a document.
type Styler struct {
	matcher *Matcher
	// budget is each rule's allowance of matching work for the document, nil
	// where there is none to keep — a Styler built by hand to prepare rules.
	budget *matchBudget
	// media is the surface the document is being laid out for, which is what a
	// media query is asked about. Its zero value is a sheet of no size, and a
	// query about a width is false against it — see Media.
	media Media
	// viewport is the page area a viewport-relative font-size is a percentage
	// of, zero where the caller did not say. See ApplyOnPage.
	viewport Media
	findings []Finding
	// sheet is the name of the stylesheet being prepared, and is what report
	// stamps on a finding raised while one is. It is empty outside prepare,
	// which is where the findings that belong to no sheet are raised.
	sheet string
	// The cascade layer being prepared, and the tree of layers seen so far.
	// layer is zero outside any @layer, which is not a layer but the band above
	// every layer for a normal declaration — see layerRank. While preparing it
	// names a node of layers; finishLayers turns it into the node's place in
	// the order. See layer.go.
	layer  int
	layers []*layerNode
	// attrOffset is where in the *markup* the style attribute being expanded
	// was written, or -1 outside one.
	//
	// A declaration in an attribute has an offset of its own, and it is an
	// offset into the attribute's value — a string the author does not have a
	// file of. What they can be pointed at is the element that carries it, so
	// that is what a finding raised from in here says instead.
	attrOffset int
	// seen suppresses repeat reports of the same unsupported property. A
	// stylesheet using "flex-wrap" forty times is one thing an author needs to
	// be told, not forty.
	//
	// Per stylesheet, which is what suppressed reads: the same property in two
	// sheets is two files to edit, and telling an author about one of them
	// sends them back to a document that still has the finding in it. See
	// suppressed, which is the key.
	seen map[string]bool

	// intern shares what the document's computed styles have in common. See
	// styleInterner; it is per Styler because a Styler styles one document.
	intern *styleInterner

	// pages and fontFaces are the @page and @font-face rules the preparation
	// reached, in the order it reached them. See Prepared.
	pages, fontFaces []AtRule
}

// PseudoKey names one pseudo-element of one element.
type PseudoKey struct {
	Node *html.Node
	// Name is "before", "after" and so on, without the colons.
	Name string
}

// Styled is the result: a computed style for every element of the document.
type Styled struct {
	// Styles maps each element to its computed values.
	Styles map[*html.Node]ComputedStyle

	// Pseudo holds the computed values of the pseudo-elements that any rule
	// selected. An entry exists only where a rule matched, because a
	// pseudo-element that nothing styles generates nothing — unlike a real
	// element, which exists whether or not anything mentions it.
	Pseudo map[PseudoKey]ComputedStyle
	// OwnFontSize and OwnPseudoFontSize mark the elements whose font-size came
	// from a declaration of their own rather than from their parent.
	//
	// A font-size in Styles is normally an absolute length — computed here, and
	// written back, because that is what a computed value is and what a
	// descendant inherits. The exception is a value this engine cannot resolve,
	// which is left as the author wrote it rather than replaced by an answer
	// nobody has; and a consumer resolving *that* against the parent's size
	// would get the right answer for the element that declared it and the wrong
	// one for every descendant that merely inherited it — twice the parent at
	// each level, so a paragraph four levels down a "font-size: 2em" wrapper is
	// set in 256px.
	//
	// That was not hypothetical: it is what this map was added to stop, back
	// when every font-size was stored as written. What is left of it is the
	// unresolvable case, which is also the one where the consumer has an element
	// to report the failure against and the cascade does not.
	OwnFontSize       map[*html.Node]bool
	OwnPseudoFontSize map[PseudoKey]bool

	// Findings is everything worth telling the caller, in stylesheet order.
	Findings []Finding
	// Incomplete reports that the selector-matching budget tripped, so some
	// rules did not get the chance to apply: a match ran past the per-match
	// bound, or a rule spent its allowance for the document and was switched
	// off. Findings name each such rule. A caller rendering an incomplete
	// result is rendering something other than the stylesheet describes.
	Incomplete bool
}

// Metrics answers the one font question the cascade cannot answer for itself.
//
// font-size is computed here, because a computed length is an absolute one and
// the em in every other declaration is relative to the answer. CSS Values
// §5.1.1 makes the font-relative units in a *font-size* refer to the parent
// element's font — "font-size: 6ex" is six times the parent's x-height — and the
// cascade has no faces: which face sets an element is chosen from the
// font-family this stage computes, by a stage that runs after it.
//
// So a caller that has already loaded its fonts can offer them here. Without one
// the fallback CSS names is used, which is half an em, and that was the only
// answer available before this existed. It is an interface rather than a font
// set because this package must not know what a face is: the question asked is
// about a computed style and a size, and both are already this package's own.
//
// Only ex is asked. ch and ic need a glyph measured through a shaper, and they
// are not silently wrong without one — a length in a unit with no metric is
// declined, so the declaration is left as written and reported by whoever has an
// element to report it against.
type Metrics interface {
	// XHeight is the x-height of the face a computed style selects, at a size,
	// and whether that face states one at all. A face that states none is not
	// an error: §5.1.1 says to assume half an em, which is what a false here
	// produces.
	//
	// In CSS pixels, for the reason LengthContext gives: "6ex" is six of this
	// number, and multiplying before the rounding is what makes six of them the
	// height of six.
	XHeight(cs ComputedStyle, size Unit) (float64, bool)
}

// Apply computes a style for every element in a document.
func Apply(doc *html.Node, sheets []Sheet) Styled {
	return ApplyWith(doc, sheets, nil)
}

// ApplyWith is Apply with a source for the font metrics a font-size may need.
// See Metrics. A nil source is the same as Apply.
func ApplyWith(doc *html.Node, sheets []Sheet, m Metrics) Styled {
	return ApplyIn(doc, sheets, m, Media{})
}

// ApplyIn is ApplyWith for a known sheet of paper, which is what a media query
// is asked about.
//
// A caller that knows the page it is laying out for should say so: without it
// the two features a page can answer — its width and its height — are asked
// about a sheet of no size, and "@media (min-width: 1cm)" is false for a
// document that would have printed on anything at all. The media *type* is
// answered either way, because that one is a fact about this engine rather than
// about the page: it renders for paper.
//
// The sheet is also what a font-size in viewport units is resolved against,
// since it is the only page a caller of this has named. A caller that decides
// margins, as layout does, knows the page *area*, and says so through
// ApplyOnPage instead.
func ApplyIn(doc *html.Node, sheets []Sheet, m Metrics, media Media) Styled {
	out, _ := applyIn(doc, sheets, m, media)
	return out
}

// applyIn is ApplyIn, and the Styler that did the work, whose accounts of it the
// tests read.
func applyIn(doc *html.Node, sheets []Sheet, m Metrics, media Media) (Styled, *Styler) {
	p := Prepare(sheets, media)
	// A caller of ApplyIn has nowhere to receive the @font-face rules — it is
	// Prepare's caller that loads them — so each one is a rule that did
	// nothing, and says so as it did before the walk handed them over. @page
	// is not reported: it computes nothing on an element, and the stage that
	// lays a document out on paper is the one that reads it.
	for _, f := range p.FontFaces {
		p.findings = appendBounded(p.findings, Finding{
			Offset: f.Rule.Offset, Sheet: f.Sheet,
			Message:     "@font-face is not applied yet",
			Unsupported: true,
			Property:    "@font-face",
		})
	}
	return p.apply(doc, m, media)
}

// AtRule is an @page or @font-face rule the cascade's walk reached: one whose
// enclosing @media and @supports conditions are all true, with the cascade
// terms a caller that decides between two of them needs.
type AtRule struct {
	Rule css.Rule
	// Sheet is the stylesheet it was written in, as Sheet.Name gave it.
	Sheet  string
	Origin Origin
	// Layer is the cascade layer it was written in, zero for none; LayerRank
	// orders two of them as the cascade orders two declarations.
	Layer int
}

// Prepared is a set of stylesheets read the way the cascade reads them — every
// selector parsed, every shorthand expanded, every @media, @supports and @layer
// evaluated — and not yet applied to a document.
//
// It exists because two things a document needs are decided by at-rules the
// cascade walks past and does not apply: @page describes the paper and
// @font-face loads a file. Each had a walker of its own in layout, and each
// walker drifted from this one: the @page reader descended only into @media,
// so "@supports (display: block) { @page { size: A5 } }" and "@layer print {
// @page { … } }" were lost with nothing said, once @supports and @layer were
// applied here (audit C33); an @font-face inside any conditional was reported
// "not applied yet" (C138). Walking once and handing the at-rules over is how
// the three cannot disagree about which blocks are live.
//
// A caller that needs them before styling — the fonts have to be loaded before
// a font-size in ex can be computed, and the page decided before layout —
// calls Prepare, reads Pages and FontFaces, and then Apply. ApplyIn is the two
// in one call.
type Prepared struct {
	media    Media
	rules    []preparedRule
	findings []Finding
	seen     map[string]bool

	// Pages and FontFaces are the @page and @font-face rules the walk reached,
	// in stylesheet order: at the top of a sheet or inside any @media,
	// @supports or @layer whose condition held. One written inside a style
	// rule is not among them — CSS Nesting allows neither there — and was
	// reported as dropped.
	Pages     []AtRule
	FontFaces []AtRule
}

// Prepare reads sheets for the given medium: see Prepared.
//
// Every media query in them is answered about media, and that is the one
// answer for the document — see layout's pipeline for why it is the sheet the
// caller asked for and not the one @page goes on to choose.
func Prepare(sheets []Sheet, media Media) *Prepared {
	s := &Styler{media: media, seen: map[string]bool{}, attrOffset: -1}
	rules := s.prepare(sheets)
	return &Prepared{media: media, rules: rules, findings: s.findings, seen: s.seen,
		Pages: s.pages, FontFaces: s.fontFaces}
}

// Apply computes a style for every element in a document, from the prepared
// sheets. It may be called more than once, for more than one document: it
// changes nothing it was given.
func (p *Prepared) Apply(doc *html.Node, m Metrics) Styled {
	out, _ := p.apply(doc, m, Media{})
	return out
}

// ApplyOnPage is Apply with the page area known, which is what a
// viewport-relative length is a percentage of on paper (CSS 2 §10.1 makes the
// page area the initial containing block, and CSS Values 4 §6.1.2 measures the
// viewport units against that).
//
// Of all the lengths only font-size needs it here. Every other one is left as
// written for layout, which knows the page area and resolves "3vw" where the
// box is laid out; a font-size cannot wait, because it is inherited as a
// number and every em below it is relative to that number. Without the page,
// "font-size: 5vw" was left as written, reported by layout as unresolvable, and
// set at the inherited size (audit C157) — although the pipeline had decided
// the page before it styled anything.
func (p *Prepared) ApplyOnPage(doc *html.Node, m Metrics, area Media) Styled {
	out, _ := p.apply(doc, m, area)
	return out
}

func (p *Prepared) apply(doc *html.Node, m Metrics, viewport Media) (Styled, *Styler) {
	s := &Styler{matcher: NewMatcher(doc), media: p.media, viewport: viewport,
		seen:     maps.Clone(p.seen),
		findings: append([]Finding(nil), p.findings...), attrOffset: -1}

	// Shorthands were expanded and what the engine does not implement dropped
	// once for the whole run, in Prepare, rather than once per element — the
	// answer does not depend on the element, and a document of ten thousand
	// nodes would otherwise ask the same question ten thousand times. The
	// rules are copied because indexing them numbers them.
	rules := newRuleSet(append([]preparedRule(nil), p.rules...))
	s.budget = &matchBudget{rules: make([]ruleWork, len(rules.rules))}

	out := Styled{
		Styles:            map[*html.Node]ComputedStyle{},
		Pseudo:            map[PseudoKey]ComputedStyle{},
		OwnFontSize:       map[*html.Node]bool{},
		OwnPseudoFontSize: map[PseudoKey]bool{},
	}
	// The font size of every element, which is what an em in its own
	// declarations is relative to. It is resolved here rather than left to the
	// consumer because a computed length is an absolute one, and turning the
	// em into a number is the last thing that needs the element's own size.
	sizes := map[*html.Node]Unit{}
	initial, _ := FromPx(DefaultFontSize)
	// stated records the elements whose font-size came from somewhere — this
	// element or an ancestor — rather than from the default. It is filled in
	// document order beside sizes, which the walk below guarantees.
	stated := map[*html.Node]bool{}
	rootSize := initial
	rootSeen := false

	// Document order, so a parent is always computed before its children and
	// inheritance can read the parent's finished values.
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		s.budget.styled++
		// The parent's finished style, or none for the root — which is also
		// what an element whose parent was never styled inherits from, as it
		// did when this read a map that had no entry for it.
		var parent ComputedStyle
		if p := parentElement(n); p != nil {
			parent = out.Styles[p]
		}
		b, declared, own := s.computeFor(n, rules, parent, "")
		// What has been resolved so far, for the questions asked of it
		// below. It is a view of the builder and not a copy; the writes that
		// follow go through the builder.
		cs := b.cs

		// The parent's own size, which is what an em means here, and the
		// initial size for the root — a document that says nothing about
		// font-size is set at 16px.
		parentSize := initial
		if p := parentElement(n); p != nil {
			if got, ok := sizes[p]; ok {
				parentSize = got
			}
		}
		// On the root's *own* font-size a rem is the initial value, because the
		// value it would otherwise mean is the one being computed. Everything
		// else on the root resolves rem against the answer.
		// The face an "ex" here belongs to is the *parent's*, since this
		// element's own font-size is the thing being computed — and so is the
		// parent's whole computed style, because an element may declare a
		// font-family of its own beside the size and the two are not resolved
		// together. At the root there is no parent element and the element's own
		// style is the nearest thing there is.
		fontStyle := cs
		if !parent.IsZero() {
			fontStyle = parent
		}
		size, resolved := fontSizeOf(cs, own, parentSize, rootSize, s.viewport, m, fontStyle)
		// The scale a stated size is on is the one it was stated in, so this
		// asks only where nothing has been stated: by this element, and by
		// none of its ancestors either. See DefaultMonospaceFontSize.
		//
		// Where nothing has been stated the size is a *default*, and which of
		// the two defaults it is the element's own family answers — so this
		// picks one of them rather than adjusting what it inherited. A serif
		// element inside a monospaced document is back on the proportional
		// scale, and taking its parent's thirteen pixels would have left it on
		// the other one for no reason it ever gave.
		stated[n] = own || (parentElement(n) != nil && stated[parentElement(n)])
		if !stated[n] {
			size, resolved = initial, true
			if monospaceDefault(cs) {
				size = mustUnit(DefaultMonospaceFontSize)
			}
			if size != parentSize {
				// The element's size is its own now, and layout has to be told
				// so. It resolves a font-size from the *parent's* unless the
				// element owns one — the guard that stops a relative size
				// compounding at every level — and an element that merely
				// inherited would be measured at its parent's number however
				// this branch had written it back.
				//
				// Without this the two disagreed and neither knew: the cascade
				// absolutised the element's own "em" against thirteen pixels
				// while layout set its text at sixteen, so
				// "font-family: monospace; height: 19em" came out nineteen lines
				// tall and held sixteen of them.
				//
				// Only where the number changed. A proportional element's
				// default *is* its parent's own, so saying it owns one would
				// spend a parse per element to arrive back where inheritance
				// already was — which is why a planted defect that says it
				// unconditionally moves nothing and is still worth not writing.
				own = true
			}
		}
		sizes[n] = size
		if !rootSeen {
			rootSize, rootSeen = size, true
		}
		if resolved {
			// Written back, so that what is stored is the computed value: an
			// absolute length, which is what a descendant inherits. When it
			// could not be resolved the declaration is left as the author wrote
			// it, for layout to report against the element.
			b.set(fontSizeID, s.interner().value(pxValue(size)))
		}
		s.absolutiseLengths(b, declared, size, rootSize)

		cs = s.interner().finish(b)
		out.Styles[n] = cs
		if own {
			out.OwnFontSize[n] = true
		}
		for _, name := range pseudoElementNames {
			if !s.anyRuleTargets(rules, n, name) {
				continue
			}
			// A pseudo-element inherits from the element it belongs to, which is
			// why the parent style passed here is that element's own.
			key := PseudoKey{Node: n, Name: name}
			pb, pdeclared, own := s.computeForPseudo(n, rules, cs, name)
			pcs := pb.cs
			// A pseudo-element's em is relative to its own font-size, and it
			// inherits from the element it belongs to rather than from that
			// element's parent.
			// A pseudo-element's ex is its originating element's, for the same
			// reason its em is: it inherits from that element and not from that
			// element's parent.
			psize, presolved := fontSizeOf(pcs, own, size, rootSize, s.viewport, m, cs)
			if presolved {
				pb.set(fontSizeID, s.interner().value(pxValue(psize)))
			}
			s.absolutiseLengths(pb, pdeclared, psize, rootSize)
			out.Pseudo[key] = s.interner().finish(pb)
			if own {
				out.OwnPseudoFontSize[key] = true
			}
		}
		return true
	})

	out.Findings = s.findings
	// A rule switched off by its budget is as incomplete as one whose match ran
	// out, and it is a separate account: a rule is switched off by spending,
	// which it may do without any one match reaching the per-match bound.
	cut := s.reportMatchBudget(rules)
	out.Incomplete = s.matcher.Tripped() || cut
	if out.Incomplete {
		s.report(Finding{
			Offset: -1,
			Message: "matching stopped early: some rules did not get the chance " +
				"to apply, so this document is styled less than its stylesheet describes",
		})
		out.Findings = s.findings
	}
	return out, s
}

// preparedRule is a rule with its selectors parsed and its declarations
// expanded, ready to be matched against every element.
type preparedRule struct {
	selectors []css.Selector
	decls     []preparedDecl
	// keys is one thing per selector that an element must have for the
	// selector to select it — an id, a class or an element name — and is empty
	// when at least one selector names none of the three, "[hidden]" or ":root",
	// and so can select anything. It is what ruleSet indexes on; see there for
	// why.
	keys   []ruleKey
	origin Origin
	// index is the rule's place in the ruleSet it was indexed into, which is
	// what its share of the matching budget is kept under. See matchBudget.
	index int32
	// offset and sheet are where the rule was written, for a finding about it.
	offset int
	sheet  string
	// layer is the cascade layer the rule was written in, zero for none. See
	// layer.go: it is a term of the cascade between the origin and the
	// specificity, and it is carried on the rule because every declaration in
	// one block is in the same layer.
	layer int
}

type preparedDecl struct {
	property string
	value    []css.ComponentValue
	// text is value serialised, which is what a computed style stores. See
	// expand.
	text      string
	important bool
	order     int
	offset    int
}

func (s *Styler) prepare(sheets []Sheet) []preparedRule {
	var out []preparedRule
	order := 0

	for _, sheet := range sheets {
		s.sheet = sheet.Name
		if done, ok := preparedBefore(sheet, order, s.media); ok {
			// The same sheet, prepared before, at the same place in the order.
			// The rules are reused; the findings are raised again, because they
			// belong to this document. See preparedSheet.
			out = append(out, done.rules...)
			for _, f := range done.findings {
				s.report(f)
			}
			s.pages = append(s.pages, done.pages...)
			s.fontFaces = append(s.fontFaces, done.fontFaces...)
			order = done.endOrder
			continue
		}
		mark := preparation{start: order, rules: len(out), findings: len(s.findings),
			pages: len(s.pages), fontFaces: len(s.fontFaces)}
		for _, rule := range sheet.Rules {
			s.prepareRule(rule, nil, sheet.Origin, &out, &order)
		}
		s.remember(sheet, mark, out, s.findings, order)
	}
	// Everything raised after this belongs to no one sheet: the cascade reads
	// the prepared rules of all of them at once, and a style attribute is not
	// in a sheet at all.
	s.sheet = ""
	s.finishLayers(out)
	return out
}

// Preparing a stylesheet is parsing every selector in it and expanding every
// shorthand, and for one sheet it is the same work for every document.
//
// That sheet is the user agent's, which every document carries and no document
// changes. It is two hundred and sixty selectors, and preparing it was **fifty-
// six per cent of a whole Build** for a small document — more than parsing the
// markup, styling it and laying it out put together. Adding a rule to the
// default sheet cost every document that rule's selector parse, which is what
// made two kilobytes of table rules a thirty-eight per cent regression and is
// the fault here rather than the rules.
//
// The memo is keyed on the identity of the rule slice rather than on its
// content: the caller hands over the same slice for every document — layout
// parses the sheet once — so "the same rules" is a pointer comparison and not a
// fourteen-kilobyte one. A sheet built freshly per document has a different
// slice, misses, and is prepared as it always was. The slice's length and the
// medium are in the key as well — see preparedSheet.
//
// **It is one slot and not a map, and that is the whole of its memory
// behaviour.** A map keyed on whatever a caller hands over is a leak that
// outlives the render that filled it — every author sheet is a fresh slice, so
// every document would add an entry and none would ever be removed. One slot
// cannot hold more than one sheet, and it holds the one there is.
//
// Three things make it a memo of a pure function rather than a change of
// behaviour, and each is checked rather than assumed:
//
//   - **The findings are raised again**, through report, so the bound on their
//     number applies as it always did. Preparation reports an unsupported
//     property, an unreadable selector, a nested layer; those belong to the
//     document being styled, and a memo that reported them once would put them
//     on whichever document was styled first in the process.
//   - **The order numbers have to line up.** Every declaration carries the
//     number the shared counter gave it, and the cascade breaks its last tie
//     with it. The memo records the counter it started from and is used only
//     when the sheet is at that same point again — which for a sheet that is
//     always first means zero, always.
//   - **Nothing else about the Styler may have changed.** Preparing an @layer
//     assigns a layer number, and preparing a nested one raises a note kept to
//     one per document. A sheet whose preparation touched any of that is not
//     remembered; the user agent sheet has no at-rule of any kind, so it never
//     does.

// preparation is where one sheet's preparation began, in each of the three
// things it appends to.
type preparation struct {
	start     int
	rules     int
	findings  int
	pages     int
	fontFaces int
}

// preparedSheet is one remembered preparation.
//
// The key is the rule slice's first element *and* its length *and* the medium:
// a caller that hands over rules[:k] of the same backing array is handing over
// a different sheet, and a sheet with an @media in it prepares differently for
// a different page. Keyed on the first element alone, both of those reused a
// preparation that did not describe them (audit C156).
type preparedSheet struct {
	key       *css.Rule
	n         int
	media     Media
	rules     []preparedRule
	findings  []Finding
	pages     []AtRule
	fontFaces []AtRule
	start     int
	endOrder  int
}

// prepared is the one slot. See above for why it is not a map.
var prepared atomic.Pointer[preparedSheet]

// preparedBefore answers a sheet this has prepared before, at the same point in
// the cascade order.
func preparedBefore(sheet Sheet, order int, media Media) (*preparedSheet, bool) {
	done := prepared.Load()
	switch {
	case done == nil || len(sheet.Rules) == 0:
		return nil, false
	case done.key != &sheet.Rules[0] || done.n != len(sheet.Rules) ||
		done.media != media || done.start != order:
		return nil, false
	}
	return done, true
}

// remember keeps a preparation if it is one that can be repeated.
func (s *Styler) remember(sheet Sheet, mark preparation, out []preparedRule,
	findings []Finding, order int) {

	switch {
	case len(sheet.Rules) == 0 || sheet.Origin != OriginUserAgent:
		// Only the default sheet is handed over unchanged for every document.
		// An author's is a fresh slice each time, so remembering it would evict
		// the one that pays and keep one that never hits.
		return
	case s.layer != 0 || len(s.layers) > 1:
		// The sheet declared a cascade layer, so preparing it moved state the
		// next document would have to move again.
		return
	}
	prepared.Store(&preparedSheet{
		key:       &sheet.Rules[0],
		n:         len(sheet.Rules),
		media:     s.media,
		rules:     append([]preparedRule(nil), out[mark.rules:]...),
		findings:  append([]Finding(nil), findings[mark.findings:]...),
		pages:     append([]AtRule(nil), s.pages[mark.pages:]...),
		fontFaces: append([]AtRule(nil), s.fontFaces[mark.fontFaces:]...),
		start:     mark.start,
		endOrder:  order,
	})
}

// prepareMedia evaluates an @media query and, where it matches, prepares the
// rules inside it as though they had been written where the block is.
//
// "As though they had been written there" is the whole of §3's cascading
// behaviour: an @media block adds no specificity and no priority, so the rules
// in it are ordered among their neighbours by where the block sits. That falls
// out of preparing them here, in place, rather than gathering them for later.
//
// A query that does not match drops what is inside it, and that is not a
// failure to report: the stylesheet said those rules were for another medium
// and this is not it. What *is* reported is a query naming something this
// engine cannot answer — a feature about a screen's abilities, or a syntax
// beyond the "and"-joined list this reads — because there a browser printing
// the same document may apply rules this page does not have.
func (s *Styler) prepareMedia(rule css.Rule, parent *css.Nesting, origin Origin,
	out *[]preparedRule, order *int) {

	matches, unknown := MatchesMedia(rule.Prelude, s.media)
	if unknown != "" {
		s.report(Finding{
			Offset: rule.Offset,
			Message: "the media query \"" + strings.TrimSpace(serialize(rule.Prelude)) +
				"\" asks about \"" + unknown + "\", which this engine cannot " +
				"answer, so the rules inside it were not applied",
			Unsupported: true,
			Property:    "@media",
		})
	}
	if !matches || !rule.HasBlock {
		return
	}
	if parent != nil {
		s.prepareNestedConditional(rule, parent, origin, out, order)
		return
	}
	inner, errs := css.ParseRulesFromValues(rule.Block)
	for _, e := range errs {
		// The errors inside a block are the author's to act on exactly as the
		// ones outside it are, and they were thrown away here.
		s.report(Finding{Offset: e.Offset, Message: e.Message, Unsupported: e.Unsupported})
	}
	for _, r := range inner {
		s.prepareRule(r, parent, origin, out, order)
	}
}

// prepareSupports prepares the rules of an @supports whose condition this
// engine answers yes to, in place, exactly as prepareMedia does.
//
// A condition that answers no drops what is inside it, and that is not a
// failure to report: the block is the version an author wrote for an engine
// that understands the declaration, and the fallback they wrote outside it is
// what this page gets. Saying so on every such stylesheet would be reporting
// the rule working.
//
// What is reported is a condition this cannot read — selector(), font-tech(),
// or a shape beyond the and/or/not of §2 — for the reason a media query naming
// an unanswerable feature is: a browser printing the same document may apply
// rules this page does not have.
func (s *Styler) prepareSupports(rule css.Rule, parent *css.Nesting,
	origin Origin, out *[]preparedRule, order *int) {

	matches, unreadable, malformed := supportsCondition(rule.Prelude)
	if malformed {
		// Not a condition at all, so not an @supports rule: Conditional 3
		// §2.1 makes the whole rule invalid. It is the author's to fix, and
		// nothing is missing from the engine.
		s.report(Finding{
			Offset: rule.Offset,
			Message: "the @supports condition " + quoted(serialize(rule.Prelude)) +
				" is not a valid condition, so the rule was dropped",
			Property: "@supports",
		})
		return
	}
	if unreadable != "" {
		s.report(Finding{
			Offset: rule.Offset,
			Message: "the @supports condition " + quoted(serialize(rule.Prelude)) +
				" asks about " + unreadable + ", which this engine cannot answer, " +
				"so the rules inside it were not applied",
			Unsupported: true,
			Property:    "@supports",
		})
	}
	if !matches || !rule.HasBlock {
		return
	}
	if parent != nil {
		s.prepareNestedConditional(rule, parent, origin, out, order)
		return
	}
	inner, errs := css.ParseRulesFromValues(rule.Block)
	for _, e := range errs {
		s.report(Finding{Offset: e.Offset, Message: e.Message, Unsupported: e.Unsupported})
	}
	for _, r := range inner {
		s.prepareRule(r, parent, origin, out, order)
	}
}

// collectAtRule keeps an @page or @font-face rule for the stage that acts on
// it, or reports one written inside a style rule, where CSS Nesting §2 allows
// neither — "p { @page { size: A5 } }" is not a page rule, and it was dropped
// with nothing said.
func (s *Styler) collectAtRule(rule css.Rule, parent *css.Nesting, origin Origin) {
	name := "@" + strings.ToLower(rule.Name)
	if parent != nil {
		s.report(Finding{
			Offset:   rule.Offset,
			Message:  name + " cannot be written inside a style rule, so it was dropped",
			Property: name,
		})
		return
	}
	at := AtRule{Rule: rule, Sheet: s.sheet, Origin: origin, Layer: s.layer}
	if name == "@page" {
		s.pages = append(s.pages, at)
	} else {
		s.fontFaces = append(s.fontFaces, at)
	}
}

// quoted is a condition as it appears in a finding.
func quoted(s string) string { return strconv.Quote(strings.TrimSpace(s)) }

// prepareNestedConditional prepares an @media written *inside* a style rule.
//
// The block holds a style block rather than a rule list — CSS Conditional Rules
// 5 §3 — which is to say declarations, which belong to the rule the @media is
// written inside, and rules, which are relative to it. Read as a rule list, the
// way the top-level form is, "color: red" becomes a qualified rule with no
// block and is discarded by the parser: "p { color: blue; @media print { color:
// red } }" was blue on paper, with the error going nowhere and no finding
// raised. That is the shape every stylesheet written since nesting arrived
// uses.
//
// The selectors are the enclosing rule's own, already parsed, so the
// declarations land on exactly the elements the rule they were written in lands
// on, with that rule's specificity — CSS Nesting §3.2's nested declarations
// rule, and the WPT test css-nesting/nested-declarations-matching. They are the
// very list the enclosing rule was prepared with, shared rather than parsed
// again: this used to re-parse the parent's prelude for every @media it held.
// Rules nested in the block are relative to the same parent, so they get the
// same Nesting. The order counter runs on through, which is what puts a
// declaration inside the @media after one written above it.
func (s *Styler) prepareNestedConditional(rule css.Rule, parent *css.Nesting,
	origin Origin, out *[]preparedRule, order *int) {

	s.prepareStyleBlock(rule, parent.Selectors, parent, origin, out, order)
}

// charsetLabel is the encoding an @charset names, which is a single string.
func charsetLabel(prelude []css.ComponentValue) (string, bool) {
	var only css.ComponentValue
	n := 0
	for _, v := range prelude {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		only, n = v, n+1
	}
	if n != 1 || !only.IsToken() || only.Token.Kind != css.String {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(only.Token.Value)), true
}

// utf8Charset reports whether a label names UTF-8, from the Encoding Standard's
// own table of aliases.
func utf8Charset(label string) bool {
	switch label {
	case "utf-8", "utf8", "unicode-1-1-utf-8", "unicode11utf8", "unicode20utf8",
		"x-unicode20utf8":
		return true
	}
	return false
}

// prepareRule prepares one rule and every rule nested inside it.
//
// parent is the rule this one is written inside, or nil at the top of a
// stylesheet. It is carried down rather than looked up because nesting
// composes: a rule three deep is written against the rule above it, which was
// itself written against the one above that, and each level's "&" is the level
// above as it was parsed — a reference, never a copy. See css.Nesting.
//
// The order counter runs through the recursion rather than being restarted, so
// a nested rule's declarations come after the declarations of the rule holding
// them. That is what CSS Nesting asks for — the nested rule is at the place it
// was written — and it falls out of doing the parent's declarations first.
func (s *Styler) prepareRule(rule css.Rule, parent *css.Nesting, origin Origin,
	out *[]preparedRule, order *int) {

	if rule.At {
		if strings.EqualFold(rule.Name, "media") {
			s.prepareMedia(rule, parent, origin, out, order)
			return
		}
		if strings.EqualFold(rule.Name, "layer") {
			s.prepareLayer(rule, parent, origin, out, order)
			return
		}
		if strings.EqualFold(rule.Name, "supports") {
			s.prepareSupports(rule, parent, origin, out, order)
			return
		}
		if strings.EqualFold(rule.Name, "page") || strings.EqualFold(rule.Name, "font-face") {
			// @page selects no element and computes no value on one: it
			// describes the paper. @font-face loads a file. The stages that do
			// those read them, and this walk is where they are found — at any
			// depth under the @media, @supports and @layer this walk
			// evaluates, with the layer it was written in. See Prepared.
			s.collectAtRule(rule, parent, origin)
			return
		}
		if strings.EqualFold(rule.Name, "charset") {
			// @charset names the encoding the stylesheet is written in, which
			// is not something the cascade applies to anything — it is a fact
			// about the bytes, settled before they were parsed. A sheet saying
			// it is UTF-8 is saying what is already true here and is passed
			// over in silence; one naming anything else is the same report the
			// document's own <meta charset> gets, because this engine reads
			// UTF-8 and cannot decode another.
			//
			// It was reported as an at-rule "not applied yet", which is what
			// every unrecognised at-rule gets — so a stylesheet that opens with
			// the perfectly ordinary "@charset \"utf-8\";" put its document in
			// the bucket of pages carrying something unsupported, and the
			// measurement of how much this engine really does was that much
			// smaller.
			if label, named := charsetLabel(rule.Prelude); named && !utf8Charset(label) {
				s.report(Finding{
					Offset: rule.Offset,
					Message: "the stylesheet declares the " + strconv.Quote(label) +
						" encoding; this engine reads UTF-8 and cannot decode any other",
					Unsupported: true,
					Property:    "@charset",
				})
			}
			return
		}
		// An at-rule this package does not act on and no other stage does
		// either. @page and @font-face are handed over above; everything left
		// genuinely is not applied, and reporting it is how that stays visible
		// until it is.
		s.report(Finding{
			Offset:      rule.Offset,
			Message:     "@" + rule.Name + " is not applied yet",
			Unsupported: true,
			Property:    "@" + rule.Name,
		})
		return
	}

	// Nested or not, the prelude is parsed once and as the author wrote it: a
	// nested rule's selectors are relative, and their "&" is the parent by
	// reference. See css.ParseNestedSelectorList.
	sels, errs, ok := css.ParseNestedSelectorList(rule.Prelude, parent)
	for _, e := range errs {
		s.report(Finding{
			Offset:      e.Offset,
			Message:     e.Message,
			Unsupported: e.Unsupported,
		})
	}
	for _, sel := range sels {
		s.reportUncomputedPseudo(sel.PseudoElement, rule.Offset)
	}
	if !ok {
		// An unusable selector list invalidates the rule, which is what the
		// specification requires — and the findings above already said why.
		// Its nested rules go with it: each of them is written against this
		// one, so there is nothing left for them to be relative to.
		return
	}

	s.prepareStyleBlock(rule, sels, nil, origin, out, order)
}

// prepareStyleBlock prepares one style block: the declarations it holds, which
// belong to the given selector list, and the rules nested in it, which are
// written against it.
//
// nest is that selector list as a parent, when the caller already has one — a
// block inside a nested @media is its enclosing rule's, and shares that rule's
// Nesting. Otherwise one is made here, once for the block, and only if
// something is nested in it: every "&" in every rule below points at it.
//
// The two are interleaved by where they were written rather than done in two
// passes. The order counter is what the cascade breaks a tie with, so a pass
// that did every declaration and then every nested rule would put a rule
// written *above* a declaration after it — which nested rules rarely show,
// since a different selector usually differs in specificity, and which a nested
// @media shows immediately: its declarations land on the very selector they are
// written inside, so the only thing separating them is order.
func (s *Styler) prepareStyleBlock(rule css.Rule, sels []css.Selector,
	nest *css.Nesting, origin Origin, out *[]preparedRule, order *int) {

	decls, nested, derrs := css.ParseDeclarationValues(rule.Block)
	for _, e := range derrs {
		s.report(Finding{Offset: e.Offset, Message: e.Message, Unsupported: e.Unsupported})
	}
	if nest == nil && len(nested) > 0 {
		nest = css.NewNesting(sels)
	}

	prepared := preparedRule{selectors: sels, origin: origin, layer: s.layer,
		keys: keysOf(sels), offset: rule.Offset, sheet: s.sheet}
	di, ni := 0, 0
	for di < len(decls) || ni < len(nested) {
		if ni >= len(nested) || (di < len(decls) && decls[di].Offset <= nested[ni].Offset) {
			for _, e := range s.expand(decls[di], origin) {
				e.order = *order
				*order++
				prepared.decls = append(prepared.decls, e)
			}
			di++
			continue
		}
		s.prepareRule(nested[ni], nest, origin, out, order)
		ni++
	}
	if len(prepared.decls) > 0 {
		*out = append(*out, prepared)
	}
}

// expand turns one declaration into the longhands it sets, dropping and
// reporting anything the engine does not implement.
//
// Each longhand's value is written out as text here, once, because text is what
// a computed style holds and the declaration is the same for every element it
// matches. It was serialised again for every element a rule matched, which in a
// document of a thousand paragraphs is a thousand identical strings per
// declaration.
func (s *Styler) expand(d css.Declaration, origin Origin) []preparedDecl {
	out := s.expandDecl(d, origin)
	for i := range out {
		out[i].text = serialize(out[i].value)
	}
	return out
}

// expandDecl is expand without the text.
func (s *Styler) expandDecl(d css.Declaration, origin Origin) []preparedDecl {
	name := strings.ToLower(d.Name)

	// Custom properties, and every declaration whose value uses one.
	//
	// This is asked first because the question "is this value legal for this
	// property" cannot be answered while a var() stands in the middle of it —
	// the value is whatever the custom property holds, and nothing here
	// substitutes it. Asked afterwards, one feature got three different
	// answers: "--c: red" was reported as an unimplemented property, "color:
	// var(--c)" was dropped as not a colour, and "width: var(--w)" was kept
	// verbatim. None of the three was marked unsupported — a custom property
	// looked like a vendor prefix, and a vendor prefix is a spelling of
	// something this engine does implement — so a page set in the wrong colour
	// carried no claim that anything was missing from it, and the reftest
	// ratchet counted it as clean.
	//
	// One answer now, and it is the one a browser gives. A declaration whose
	// value cannot be resolved is "invalid at computed-value time" — CSS
	// Variables §3.3 — which is not the same as an invalid declaration: it
	// still wins the cascade, and it computes to "unset", so an inherited
	// property takes the parent's value and every other one its initial value.
	// Dropping it instead would restore whatever the user agent sheet said,
	// which is a third wrong answer.
	//
	// The finding says the engine does not do this, because it does not: a
	// document using custom properties gets a page that is defensible rather
	// than one that is right, and the claim on it has to say so.
	if isCustomProperty(name) {
		if !s.suppressed(name) {
			s.report(Finding{
				Offset: d.Offset,
				Message: "the custom property \"" + name + "\" was not applied: this engine " +
					"does not substitute custom properties, so nothing can refer to it",
				Unsupported: true,
				Property:    name,
			})
		}
		return nil
	}
	if usesVar(d.Value) {
		if !s.suppressed(name) {
			s.report(Finding{
				Offset: d.Offset,
				Message: "\"" + name + ": " + serialize(d.Value) + "\" refers to a custom " +
					"property, which this engine does not substitute, so the declaration " +
					"computes to \"unset\"",
				Unsupported: true,
				Property:    name,
			})
		}
		d.Value = unsetValue()
	}

	_, registered := properties[name]
	if (registered || isLogicalLonghand(name)) && wideKeyword(d.Value) == "" {
		// §4.2: the value is not one the property takes, so there is no
		// declaration here at all and the one before it stands — or it is one
		// this engine does not evaluate, and the same is done for the same
		// reason, since the declaration before it is the fallback its author
		// wrote. See valuegate.go and grammar.go.
		//
		// The first is not marked unsupported: nothing is missing from the
		// engine, a stylesheet said something CSS forbids and CSS says what to
		// do. The second is.
		if j := judgeLonghand(name, d.Value); j.drop {
			s.report(Finding{Offset: d.Offset, Message: j.why, Property: name,
				Unsupported: j.unsupported})
			return nil
		}
	}

	if registered {
		// A registered property that nothing reads is reported here rather than
		// dropped. The value still cascades — inheritance and the computed
		// value are right, and the day the property is implemented there is
		// nothing to undo — but the silence that the registry entry bought is
		// given back. See unimplemented.go for why that silence is the failure
		// mode this guards.
		// Only for a declaration someone wrote. The engine's own default sheet
		// uses several of these — "a { text-decoration: underline }" among them
		// — and reporting those would put a finding on every document ever
		// rendered, including documents with no link in them: this runs when the
		// sheet is parsed, not when a rule matches. That is noise, and noise in
		// the one channel that says what the page is missing is worse than
		// silence, because it is what makes the channel stop being read.
		//
		// The gap is still real for the default sheet. It belongs in the note on
		// the property rather than in every document's findings.
		if reason, missing := unimplementedReason(name); missing &&
			origin != OriginUserAgent && !s.suppressed(name) &&
			!isInertDeclaration(name, d.Value) {
			s.report(Finding{
				Offset: d.Offset,
				Message: "the property \"" + name + "\" is not implemented, so " +
					reason,
				Unsupported: true,
				Property:    name,
			})
		}
		// And a property that is read, declared with a value nothing acts on.
		// See unimplementedValues.
		if value, reason, missing := unimplementedValueReason(name, d.Value); missing &&
			origin != OriginUserAgent && !s.suppressed(name+"\x00"+value) {
			s.report(Finding{
				Offset: d.Offset,
				Message: "the value \"" + value + "\" of \"" + name +
					"\" is not implemented, so " + reason,
				Unsupported: true,
				Property:    name,
			})
		}
		return []preparedDecl{{
			property: name, value: d.Value, important: d.Important, offset: d.Offset,
		}}
	}

	if isLogicalLonghand(name) {
		// A logical property is implemented by being renamed to the physical
		// one it sets, which happens per element once the direction is known.
		// It is not in the registry — a logical name never survives into a
		// computed style, so it has no initial value and nothing would read one
		// — so it has to be let through here rather than falling to the
		// unimplemented report below.
		return []preparedDecl{{
			property: name, value: d.Value, important: d.Important, offset: d.Offset,
		}}
	}

	if sh, ok := shorthands[name]; ok {
		// A CSS-wide keyword on a shorthand sets every longhand to it, which
		// the expander below cannot express — it splits on whitespace and would
		// hand "inherit" to the first slot only.
		if kw := wideKeyword(d.Value); kw != "" {
			var out []preparedDecl
			for _, longhand := range shorthandLonghands(name) {
				out = append(out, preparedDecl{
					property: longhand, value: d.Value,
					important: d.Important, offset: d.Offset,
				})
			}
			return out
		}
		parts, unsupported, ok := sh.expand(d.Value)
		for _, part := range unsupported {
			// A part of the shorthand this engine understood and cannot
			// produce. Naming it is the difference between an author learning
			// their background image did not appear and wondering why the page
			// is blank.
			key := name + "\x00" + part
			if !s.suppressed(key) {
				s.report(Finding{
					Offset: d.Offset,
					Message: "\"" + part + "\" in the " + name +
						" shorthand is not implemented, so it was not applied",
					Unsupported: true,
					Property:    name,
				})
			}
		}
		if !ok {
			// Unless the expander has already said what it could not produce,
			// in which case this would be a second finding contradicting the
			// first: "font: menu" is a system font, which is reported as
			// unsupported above and is not a value the author got wrong.
			//
			// An expander tells its parts apart by the same terms the value
			// grammar judges them with, so a part that is valid CSS this engine
			// does not evaluate — "border: calc(1px + 1px) solid oklch(…)" —
			// lands in its slot and is judged below, rather than failing here
			// and being reported as the author's mistake (audit C59). What
			// fails here is a value no slot takes.
			if len(unsupported) == 0 {
				s.report(Finding{
					Offset:   d.Offset,
					Message:  invalidReason(name, d.Value),
					Property: name,
				})
			}
			return nil
		}
		// Every longhand the shorthand set is a declaration of that longhand,
		// and is judged as one: "margin: 1px foo" is as invalid as
		// "margin-right: foo", and §4.2 drops the shorthand whole.
		if j := judgeExpansion(name, d.Value, parts); j.drop {
			s.report(Finding{Offset: d.Offset, Message: j.why, Property: name,
				Unsupported: j.unsupported})
			return nil
		}
		out := make([]preparedDecl, 0, len(parts))
		for longhand, value := range parts {
			out = append(out, preparedDecl{
				property: longhand, value: value,
				important: d.Important, offset: d.Offset,
			})
		}
		// Map iteration is random, so the order numbers these longhands receive
		// would otherwise differ from run to run.
		//
		// Nothing observable depends on it today, and that is worth writing down
		// rather than leaving as an implied guarantee: two longhands of one
		// shorthand are different properties, so they never compete, and every
		// declaration outside this shorthand is numbered entirely before or
		// entirely after all four. Removing this sort does not change a single
		// computed value, and the determinism test does not fail — which was
		// checked rather than assumed.
		//
		// It stays because the numbers are reproducible with it and arbitrary
		// without it, and the moment anything begins to read them — a finding
		// per longhand, a cache keyed on them — the cost of not having it is a
		// bug that appears one run in ten.
		sort.Slice(out, func(i, j int) bool { return out[i].property < out[j].property })
		return out
	}

	// A declaration of an unimplemented property whose value is that property's
	// own initial value asks for the page that is already there, so there is
	// nothing to report. See inert.go, which is careful that this is about the
	// value and not the property: "resize: none" is inert and "resize: both" is
	// not.
	if isInertDeclaration(name, d.Value) {
		return nil
	}

	// The unsupported-property finding: a declaration parsed and then not
	// applied. It is on by default and it is the cheapest guardrail in the
	// design — a page where a property was dropped is plausible and wrong.
	//
	// A vendor-prefixed name is the one case where dropping it is not a gap.
	// "-moz-tab-size" is Gecko's property, not CSS's, and every engine that is
	// not Gecko drops it — which is precisely what the prefix is for and why an
	// author writes the standard property beside it. Saying the page differs
	// from the one the stylesheet describes would be wrong: the stylesheet
	// describes this page to every engine but one.
	//
	// It is still reported, because an author who wrote *only* the prefixed
	// spelling has a page missing what they asked for and no other way to learn
	// it. What changes is the claim, not the message. See css/selector.go's
	// inapplicable for the same distinction drawn about a selector.
	//
	// A property with nothing to apply to on a page is the second case of the
	// same kind, and nomedium.go is the list: nobody puts a caret in a printed
	// paragraph, so "caret-color" colours nothing there and a browser printing
	// the document applies it exactly as little.
	if !s.suppressed(name) {
		s.report(Finding{
			Offset:      d.Offset,
			Message:     "the property \"" + name + "\" is not implemented, so it was not applied",
			Unsupported: !vendorPrefixed(name) && !hasNothingToApplyTo(name),
			Property:    name,
		})
	}
	return nil
}

// legalBackgroundImage reports whether a value is one background-image takes: a
// comma-separated list, each entry an <image> or "none".
//
// It says nothing about which images this engine can *paint*. An image it
// cannot paint is a gap in the engine and is reported as one; a value that is
// not an image is a stylesheet mistake, and CSS says what becomes of it. Both
// leave the box bare and only one of them is worth telling an author about as a
// missing feature.
//
// Every <image> is a single token — a url(), or a function: the gradients,
// image-set(), cross-fade(), element(), and whatever comes next. So the shape
// is checkable without a list of function names, which is what keeps this from
// rejecting an image nobody has written yet.
func legalBackgroundImage(vals []css.ComponentValue) bool {
	layers := splitOnComma(vals)
	for _, layer := range layers {
		parts := splitOnWhitespace(layer)
		if len(parts) != 1 || len(parts[0]) != 1 {
			return false
		}
		v := parts[0][0]
		if v.IsFunction() {
			continue
		}
		if !v.IsToken() {
			return false
		}
		switch v.Token.Kind {
		case css.URL:
			continue
		case css.Ident:
		default:
			return false
		}
		switch strings.ToLower(v.Token.Value) {
		case "none":
		case kwInherit, kwInitial, kwUnset, kwRevert, kwRevertLayer:
			// A CSS-wide keyword is the whole value or it is nothing:
			// "none, inherit" is not a layer list with a keyword in it.
			if len(layers) != 1 {
				return false
			}
		default:
			return false
		}
	}
	return len(layers) > 0
}

// legalDisplay reports whether a "display" value is one css-display-3 defines.
//
// The list is that specification's, plus the two legacy shapes CSS has always
// had — "inline-block" and friends — and "-webkit-box", which this engine
// implements as a block for "-webkit-line-clamp" to be written on. A value not
// on it is not a strange display, it is not a display at all.
//
// The two-value syntax is accepted loosely: any combination of an outside
// keyword, an inside keyword and "list-item", in any order. Being permissive is
// the safe direction here, because the cost of the two mistakes is not
// symmetric — keeping a value gives the element what layout makes of it, which
// layout/box.go's parseDisplay either lays out or reports, and dropping one
// that is really a display silently restores the user agent sheet's answer
// instead.
//
// The one combination refused is the one the grammar itself rules out rather
// than leaves open: §2.3's <display-listitem> takes "flow" or "flow-root" as its
// inside value and nothing else, so "list-item flex" is not a display value of
// any kind — a browser drops it, and so does this.
func legalDisplay(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if v.IsFunction() || v.IsBlock() {
			// A value this engine has not finished reading. "var()" is the one
			// that matters: custom properties are not substituted here, so what
			// the declaration says is not known yet, and calling it invalid
			// would be deciding that on no evidence.
			return true
		}
	}
	parts := splitOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) != 1 || part[0].Token.Kind != css.Ident {
			// A string, a number, a punctuation mark. Whatever else it is, it
			// is not a keyword, and every display value is one.
			return false
		}
		words = append(words, strings.ToLower(part[0].Token.Value))
	}
	if len(words) == 1 {
		switch words[0] {
		case kwInherit, kwInitial, kwUnset, kwRevert, kwRevertLayer:
			return true
		}
		if singleDisplay[words[0]] {
			return true
		}
		// "flow" and "flow-root" are inside keywords and are also valid alone.
		return displayInside[words[0]]
	}
	var outside, inside, item int
	for _, w := range words {
		switch {
		case displayOutside[w]:
			outside++
		case displayInside[w]:
			inside++
		case w == "list-item":
			item++
		default:
			return false
		}
	}
	if item == 1 && inside == 1 && !listItemInside(words) {
		return false
	}
	return outside <= 1 && inside <= 1 && item <= 1
}

// listItemInside reports whether a display value's inside keyword is one a list
// item may have: "flow" or "flow-root".
func listItemInside(words []string) bool {
	for _, w := range words {
		if displayInside[w] {
			return w == "flow" || w == "flow-root"
		}
	}
	return true
}

var displayOutside = map[string]bool{"block": true, "inline": true, "run-in": true}

var displayInside = map[string]bool{
	"flow": true, "flow-root": true, "table": true,
	"flex": true, "grid": true, "ruby": true,
}

// singleDisplay is every value that stands on its own: the box keywords, the
// legacy pairs, and the layout-internal ones a table is built from.
var singleDisplay = map[string]bool{
	"none": true, "contents": true,
	"block": true, "inline": true, "run-in": true, "list-item": true,
	"inline-block": true, "inline-table": true,
	"inline-flex": true, "inline-grid": true,
	"table": true, "table-row-group": true, "table-header-group": true,
	"table-footer-group": true, "table-row": true, "table-cell": true,
	"table-column-group": true, "table-column": true, "table-caption": true,
	"flex": true, "grid": true, "ruby": true,
	"ruby-base": true, "ruby-text": true,
	"ruby-base-container": true, "ruby-text-container": true,
	"math": true,
	// Not a specification value. This engine reads it as a block, because
	// css-overflow-4's compatibility section is written around it.
	"-webkit-box": true,
}

// nonNegative lists the longhands whose value CSS 2.1 says may not be negative.
//
// Each entry is a property whose definition carries the words "Negative values
// are illegal" or "Negative lengths are not allowed": the sizes of §10.2, §10.4,
// §10.5 and §10.7, the paddings of §8.4 and the border widths of §8.5.1. The
// list is deliberately short and deliberately not "everything that looks like a
// length" — a negative margin, a negative text-indent, a negative letter-spacing
// and a negative word-spacing are all legal and all useful, and dropping one of
// those would break a page that is doing nothing wrong.
//
// The border widths differ from the paddings in what dropping them produces, and
// that is why they cannot be handled where they are read. A padding's initial
// value is zero, so clamping a negative one to zero gives the right answer by
// accident; a border width's initial value is "medium", which is three pixels of
// ink. Layout clamped, so "border-top-width: -1pt" drew no border where CSS asks
// for the initial one — fourteen tests in css/CSS2/borders, one per unit per
// side, and every one of them invisible until inline boxes started painting
// their borders, because the reference draws its two rules on a <span>.
//
// The shorthands are here too, and the table below says which and why. They were
// not, and the gap was the shape §4.2 warns about: "padding: 8px; padding: -8px"
// dropped the eight pixels and clamped the second declaration to zero, so a
// declaration CSS says does not exist overrode one that does.
var nonNegative = map[string]bool{
	"width": true, "height": true,
	"min-width": true, "min-height": true,
	"max-width": true, "max-height": true,
	"padding-top": true, "padding-right": true,
	"padding-bottom": true, "padding-left": true,
	"border-top-width": true, "border-right-width": true,
	"border-bottom-width": true, "border-left-width": true,
	// And the ones CSS 2.1 does not have. Each definition states its range as
	// a non-negative one, in the notation the later specifications use:
	// line-height is <number [0,∞]> | <length [0,∞]> | <percentage [0,∞]>,
	// border-spacing is two non-negative lengths, outline-width is a border
	// width, background-size takes non-negative lengths and percentages, and
	// tab-size is <number [0,∞]> | <length [0,∞]>.
	"line-height": true, "border-spacing": true, "outline-width": true,
	"background-size": true, "tab-size": true,

	// And the shorthands every one of whose numeric components is one of the
	// above. §4.2 drops an invalid declaration whole, so "padding: 1px -2px" is
	// no more a declaration than "padding-top: -2px" is — and the negative used
	// to reach two of the four longhands and be clamped there, which is the
	// worst of the three answers: the author's earlier "padding: 8px" was
	// overridden by a declaration CSS says does not exist.
	//
	// A shorthand is listed only where a negative number cannot be anything but
	// an illegal component. In the border family the only length is the width;
	// a style is a keyword and a colour is a keyword, a hash or a function, and
	// hasNegativeNumber does not look inside a function. In "font" the numbers
	// are the weight, the size and the line-height, and none of the three may be
	// negative.
	//
	// "margin" and "background" are deliberately absent and are the reason this
	// is a list rather than a rule about shorthands. A negative margin is legal
	// and useful, and so is a negative background-position — "background: url(x)
	// -10px 0" places an image off its own left edge, which is how a sprite
	// sheet works. Dropping either would break a page doing nothing wrong.
	"padding": true, "border-width": true,
	"border": true, "border-top": true, "border-right": true,
	"border-bottom": true, "border-left": true,
	"outline": true, "font": true,

	// And the properties that arrived after this list was written, each with
	// its range stated the same way. flex-grow and flex-shrink are
	// <number [0,∞]>, flex-basis is a <'width'>, column-count and the two
	// line-clamps are <integer [1,∞]>, column-width is <length [0,∞]>, and the
	// gaps are non-negative lengths or percentages.
	//
	// The cost of the omission is not that the negative was drawn — it is that
	// it was *kept*. "flex-grow: 2; flex-grow: -1" reached layout as the second
	// declaration, layout could make nothing of it and fell back to the initial
	// value of 0, and the author's 2 was lost. That is exactly the failure the
	// comment at the head of this list describes, in a property added later.
	"flex-grow": true, "flex-shrink": true, "flex-basis": true,
	"column-count": true, "column-width": true,
	"column-gap": true, "row-gap": true, "gap": true, "columns": true,
	"line-clamp": true, "-webkit-line-clamp": true,
	"flex": true,
}

// The logical longhands and shorthands whose physical counterparts may not be
// negative.
//
// They are derived rather than typed, because the two lists cannot be allowed
// to drift: a longhand added to logicalSides is covered the day it is added.
// The check has to know the logical name at all because the rename to a
// physical one happens per element, several steps after §4.2's drop — so
// "padding-inline-start: -8px" was a declaration this file never looked at and
// computed to "padding-left: -8px".
func init() {
	for logical := range logicalLonghands {
		if proxy, _ := logicalProxy(logical); nonNegative[proxy] {
			nonNegative[logical] = true
		}
	}
	// A shorthand is non-negative when every longhand it sets is, which is the
	// rule the physical list above is written by and states one by one.
	//
	// Read out of logicalShorthands rather than out of the merged table, since
	// the merge is another package-level init and nothing orders the two.
	for name, sh := range logicalShorthands {
		if len(sh.longhands) == 0 {
			continue
		}
		all := true
		for _, l := range sh.longhands {
			all = all && nonNegative[l]
		}
		if all {
			nonNegative[name] = true
		}
	}
}

// legalQuotes reports whether a "quotes" value matches §12.3.2's grammar.
//
// The CSS-wide keywords never reach it — expand deals with those before any
// property-specific check — so what is left is "none" or an even, non-zero
// number of strings and nothing else between them.
func legalQuotes(vals []css.ComponentValue) bool {
	seen := 0
	for _, v := range vals {
		if !v.IsToken() {
			return false
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.String:
			seen++
		case css.Ident:
			// "none" is the only identifier the grammar admits, and only alone.
			return seen == 0 && strings.EqualFold(v.Token.Value, "none") && onlyIdent(vals)
		default:
			return false
		}
	}
	return seen >= 2 && seen%2 == 0
}

// legalCounterFunctions reports whether every counter() and counters() in a
// "content" value has the arguments §12.2 gives it.
//
// Only those two functions are judged. The rest of the property's grammar is
// deliberately left alone: a value this engine cannot *produce* — an image, an
// identifier that is not one of the quote keywords — is reported where it is
// read, by name and with the element it was on, and turning that into a silent
// drop here would take the one message that says what is missing from a page and
// replace it with nothing. What is checked here is different in kind: a value
// that is not CSS at all, where §4.2 says the declaration goes and the earlier
// one stands.
//
// Anything that is not one of the two functions passes, including a nested one:
// neither function takes a function as an argument, so a counter() inside
// something else is already outside the grammar being checked and is left to the
// reader to refuse.
func legalCounterFunctions(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if !v.IsFunction() {
			continue
		}
		name := strings.ToLower(v.Token.Value)
		if name != "counter" && name != "counters" {
			continue
		}
		if !legalCounterArguments(name == "counters", v.Values) {
			return false
		}
	}
	return true
}

// legalCounterArguments checks one call's argument list.
//
// The arguments are read positionally rather than by type, which is the point:
// counter(name, style) and counters(name, string, style) put different things in
// the second slot, and a reader that took whichever it recognised would accept
// counter(name, string) — the value this exists to refuse.
func legalCounterArguments(isCounters bool, vals []css.ComponentValue) bool {
	var args [][]css.ComponentValue
	cur := []css.ComponentValue{}
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if v.IsToken() && v.Token.Kind == css.Comma {
			args = append(args, cur)
			cur = nil
			continue
		}
		cur = append(cur, v)
	}
	args = append(args, cur)

	// The name, then the separator counters() takes and counter() does not,
	// then the style both may end with.
	want := []css.Kind{css.Ident}
	if isCounters {
		want = append(want, css.String)
	}
	if len(args) == len(want)+1 {
		want = append(want, css.Ident)
	}
	if len(args) != len(want) {
		return false
	}
	for i, arg := range args {
		if len(arg) != 1 || !arg[0].IsToken() || arg[0].Token.Kind != want[i] {
			return false
		}
	}
	return true
}

// onlyIdent reports that a value holds exactly one identifier and no other
// token, which is what "quotes: none" has to be to mean none.
func onlyIdent(vals []css.ComponentValue) bool {
	seen := 0
	for _, v := range vals {
		if !v.IsToken() {
			return false
		}
		if v.Token.Kind == css.Whitespace {
			continue
		}
		if v.Token.Kind != css.Ident {
			return false
		}
		seen++
	}
	return seen == 1
}

// hasNegativeNumber reports whether any numeric token in a value is negative.
//
// It reads the tokens rather than parsing a length, because this runs when a
// sheet is prepared and there is no element, no font size and no containing
// block yet. A negative number is negative whatever unit it carries and
// whatever it would have resolved to, which is what makes the syntactic test
// exactly as strong as the semantic one for this rule.
//
// A function's arguments are not looked into. "calc(10px - 20px)" is negative
// and this does not say so, and that is the same reason as the paragraph above
// rather than a gap: a calc() is evaluated per element, against a font size and
// a containing block that do not exist while a sheet is being prepared, so its
// sign is not a fact about the declaration. "calc(1em - 20px)" is negative in
// one element and positive in the next. Guessing would drop declarations that
// are perfectly legal in the element they land on.
func hasNegativeNumber(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if !v.IsToken() {
			continue
		}
		switch v.Token.Kind {
		case css.Number, css.Percentage, css.Dimension:
			if v.Token.Number < 0 {
				return true
			}
		}
	}
	return false
}

// shorthandLonghands lists what a shorthand sets, for the CSS-wide-keyword path.
func shorthandLonghands(name string) []string {
	sh, ok := shorthands[name]
	if !ok {
		return nil
	}
	out := append([]string(nil), sh.longhands...)
	sort.Strings(out)
	return out
}

// suppressed reports whether this stylesheet has already been told about key,
// and records that it has been.
//
// The sheet is part of it because a finding names one: an unsupported property
// in two files is two findings, and it was one until the second was dropped for
// having the same words as the first.
func (s *Styler) suppressed(key string) bool {
	full := s.sheet + "\x00" + key
	if s.seen[full] {
		return true
	}
	s.seen[full] = true
	return false
}

func (s *Styler) report(f Finding) {
	switch {
	case s.attrOffset >= 0 && !f.InMarkup:
		f.Offset, f.InMarkup, f.Sheet = s.attrOffset, true, ""
	case f.Sheet == "" && !f.InMarkup:
		f.Sheet = s.sheet
	}
	s.findings = appendBounded(s.findings, f)
}

// appendBounded adds a finding to a list held to maxFindings, with a note in
// place of the first one past it.
//
// The note is what the findings past the bound become, so it has to carry the
// one thing about them a caller acts on: whether any was Unsupported. A page
// with CSS this engine does not implement is not a clean page, and a caller —
// the WPT ratchet is one — tells the two apart by whether any finding is
// Unsupported. The note used to be a plain styling problem, so two hundred
// author errors followed by a transform (audit C58) came out as a page with
// nothing unsupported on it: the errors are never de-duplicated, and old-web
// hacks like "*zoom" and "_height" reach two hundred on their own. So the first
// Unsupported finding the bound drops turns the note into one, with that
// finding's property — which is what decides the rule a caller maps it to — and
// its message, so what the page lacked is named rather than hinted at.
func appendBounded(findings []Finding, f Finding) []Finding {
	if len(findings) < maxFindings {
		return append(findings, f)
	}
	if len(findings) == maxFindings {
		findings = append(findings, Finding{
			Offset:  -1,
			Message: "further styling problems were not reported",
		})
	}
	if note := &findings[maxFindings]; f.Unsupported && !note.Unsupported {
		note.Unsupported, note.Property = true, f.Property
		note.Message = "further styling problems were not reported, among them " +
			"CSS this engine does not implement: " + f.Message
	}
	return findings
}

// reportUncomputedPseudo names a pseudo-element the selector parser accepts and
// this stage does not compute a style for.
//
// The parser's list and this one are two answers to "which pseudo-elements does
// this engine have", and where they differ the rules written for the difference
// do nothing at all. "::first-letter" parsed, matched, and was never computed:
// a drop cap written the ordinary way was silently an ordinary first letter,
// and the page carried no claim that anything was missing from it.
//
// Derived rather than listed, so that a pseudo-element the parser learns to
// accept is reported until this stage learns to compute it. The two lists are
// equal today — ::first-letter was the last of the difference — so no document
// reaches this, and it is driven directly by
// TestAPseudoElementThatIsParsedAndNotComputedIsReported rather than left as a
// guard nothing has been seen to fire. The property it exists for is asserted
// separately, from the parser's side: see
// TestEveryPseudoElementTheParserAcceptsIsComputed.
func (s *Styler) reportUncomputedPseudo(name string, offset int) {
	if name == "" {
		return
	}
	for _, computed := range pseudoElementNames {
		if name == computed {
			return
		}
	}
	if key := "::" + name; s.suppressed(key) {
		return
	}
	key := "::" + name
	s.report(Finding{
		Offset: offset,
		Message: "\"" + key + "\" is not implemented, so what was written for it " +
			"was not applied",
		Unsupported: true,
		Property:    key,
	})
}

// pseudoElementNames are the ones this stage computes a style for.
//
// Three of them generate a box. ::first-line does not — it styles part of
// something that already exists, and there is no first line until the breaking
// has happened — so nothing downstream asks this stage to make one. What it does
// need is the style, resolved here like any other: its font-size is relative to
// the element's own, and every em in it is absolutised against the answer, which
// is work only the cascade can do.
//
// ::first-letter does not generate one either, and for a reason that reads the
// same and is not: there is no first letter until the text has been through
// white-space processing and its element's own text-transform, which the box
// builder does. What this stage owes it is the style, absolutised here like
// ::first-line's — a ::first-letter font-size is relative to the element's own —
// and the builder divides the text.
var pseudoElementNames = []string{"before", "after", "marker", "first-line", "first-letter"}

// anyRuleTargets reports whether any rule selects a pseudo-element of an
// element.
//
// It exists so that a pseudo-element with nothing said about it costs nothing: a
// document of ten thousand elements would otherwise compute three extra styles
// each, all of them the initial values, and generate nothing from any of them.
func (s *Styler) anyRuleTargets(rules *ruleSet, n *html.Node, name string) bool {
	found := false
	rules.forEach(n, func(r *preparedRule) bool {
		for _, sel := range r.selectors {
			if sel.PseudoElement != name {
				continue
			}
			if s.matchRule(r, sel, n) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// computeForPseudo resolves the properties of a pseudo-element.
//
// It inherits from the element it belongs to rather than from that element's
// parent, which is what makes "p { color: red } p::before { content: '>' }" draw
// a red marker without the author saying so twice.
func (s *Styler) computeForPseudo(n *html.Node, rules *ruleSet,
	owner ComputedStyle, name string) (*styleBuilder, []propID, bool) {
	return s.computeFor(n, rules, owner, name)
}

// computeFor resolves every property for one element, or for one of its
// pseudo-elements when pseudo is not empty, whose parent's style is parent —
// the zero style for the root, and the element's own for a pseudo-element.
//
// It hands back the style still under construction, because the caller has
// two things left to write into it — the resolved font-size and the lengths
// that depend on it — together with the properties a declaration decided,
// which are the only ones those can have changed. See absolutiseLengths.
//
// It resolves only those properties. Every other one is what an undeclared
// property is — the parent's value where it inherits and the initial value
// where it does not — and the builder starts out holding exactly that, by
// sharing the parent's inherited block and storing nothing else. It used to
// resolve all hundred and forty-eight for every element, and serialise each
// winner again for every element it won on.
//
// It also reports whether font-size came from a declaration rather than by
// inheritance, which is the one thing a consumer cannot recover from the style
// it returns. See Styled.OwnFontSize for why that matters.
func (s *Styler) computeFor(n *html.Node, rules *ruleSet,
	parent ComputedStyle, pseudo string) (*styleBuilder, []propID, bool) {

	var cands []candidate
	rules.forEach(n, func(r *preparedRule) bool {
		spec, ok := s.matchSpecificityFor(r, n, pseudo)
		if !ok {
			return true
		}
		for _, d := range r.decls {
			cands = append(cands, candidate{
				property: d.property, value: d.value, text: d.text,
				important: d.important,
				origin:    r.origin, layer: r.layer, spec: spec,
				order: d.order, offset: d.offset,
			})
		}
		return true
	})

	// The presentational hints of hints.go, at the very bottom of the author
	// origin: zero specificity and an order number below every declaration an
	// author wrote, so any author rule at all beats them and no user-agent rule
	// ever does. They belong to the element and not to its pseudo-elements,
	// which have no attributes of their own.
	if pseudo == "" {
		for property, value := range presentationalHints(n) {
			cands = append(cands, candidate{
				property: property, value: value,
				text:   s.interner().value(serialize(value)),
				origin: OriginAuthor, order: hintOrder, offset: n.Offset,
			})
		}
	}

	// An inline style="..." attribute, which cascades above every author rule
	// of the same importance. It has no selector, so it has no specificity;
	// what puts it on top is its own step in the cascade order.
	//
	// It applies to the element and not to its pseudo-elements: there is no
	// syntax for writing one on a ::before, so a style attribute reaching one
	// would be a rule the author had no way to express.
	var inline map[string]preparedDecl
	if pseudo == "" {
		inline = s.inlineDeclarations(n)
	}

	winners := map[string]candidate{}
	pick := func() {
		clear(winners)
		for _, c := range cands {
			if best, ok := winners[c.property]; ok && !beats(c, best) {
				continue
			}
			winners[c.property] = c
		}
	}
	pick()

	// CSS Logical Properties. "margin-inline-start" is the margin before the
	// first character of a line, which is the left one in English and the right
	// one in Arabic — so which physical property it sets is this element's own
	// answer, and cannot be settled where the shorthands were expanded.
	//
	// The rename happens before the winner is chosen and the winners are then
	// picked again, because a logical declaration and a physical one compete:
	// css-logical says they set the same thing, so "margin-left: 1px;
	// margin-inline-start: 2px" is 2px in English and swapping the lines makes
	// it 1px. Renaming first is what lets the ordinary cascade decide that.
	//
	// Writing mode and direction are resolved by the same function the main
	// loop below resolves every property with, because it is the same
	// question — a winner, an inline style against it, and inheritance under
	// both — asked early.
	writingMode := s.early("writing-mode", winners, inline, parent)
	rtl := strings.EqualFold(s.early("direction", winners, inline, parent), "rtl")
	if renameLogical(cands, inline, writingMode, rtl) {
		pick()
	}

	// The properties something declared, in the registry's order so that
	// whatever resolve reports is reported in the same order on every run. A
	// winner under a name that is not registered — a logical longhand this
	// element did not rename — is not a property and was never stored.
	declared := make([]propID, 0, len(winners)+len(inline))
	for name := range winners {
		if id, ok := registry.ids[name]; ok {
			declared = append(declared, id)
		}
	}
	for name := range inline {
		if id, ok := registry.ids[name]; ok {
			declared = append(declared, id)
		}
	}
	sort.Slice(declared, func(i, j int) bool { return declared[i] < declared[j] })
	declared = compactIDs(declared)

	b := childStyleBuilder(parent)
	ownFontSize := false
	for _, id := range declared {
		name := registry.names[id]
		prop := registry.slots[id].property()
		value, have := "", false

		value, have = s.winning(name, winners, inline)

		b.set(id, s.resolve(name, prop, value, have, parent))
		if name == "font-size" {
			ownFontSize = have && declaresItsOwnValue(value, prop)
		}
	}
	return b, declared, ownFontSize
}

// declaresItsOwnValue reports whether a winning declaration says something about
// the element rather than deferring to its parent.
//
// The three CSS-wide keywords that defer are the ones that reach inheritFrom:
// "inherit" always, and "unset" and "revert" on a property that inherits. Every
// other value — including "initial", which is a statement about this element —
// is the element's own.
func declaresItsOwnValue(value string, prop property) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case kwInherit:
		return false
	case kwUnset, kwRevert, kwRevertLayer:
		return !prop.inherits
	}
	return true
}

// resolve turns a winning declaration — or the absence of one — into a computed
// value, applying the CSS-wide keywords and inheritance.
func (s *Styler) resolve(name string, prop property, value string, have bool, parent ComputedStyle) string {
	inheritFrom := func() string {
		if v, ok := parent.Lookup(name); ok {
			return v
		}
		// The root has no parent to inherit from, so it takes the initial
		// value — which is what "the initial value" means for the root.
		return prop.initial
	}

	if have {
		if name == "color" && strings.EqualFold(strings.TrimSpace(value), "currentcolor") {
			// CSS Color 4 §7.2: "If the 'currentcolor' keyword is set on the
			// 'color' property itself, it is treated as 'color: inherit'."
			//
			// It is answered here because this is where inheritance is, and
			// because the alternative is answering it at paint time with no
			// parent to hand — which is what happened, and came out as the
			// initial value: black, on a paragraph inside a green div that had
			// asked for the green.
			return inheritFrom()
		}
		switch strings.ToLower(value) {
		case kwInherit:
			return inheritFrom()
		case kwInitial:
			return prop.initial
		case kwUnset:
			// "unset" is "inherit if the property inherits, initial if it does
			// not" — the keyword that means "as though nothing had been said".
			if prop.inherits {
				return inheritFrom()
			}
			return prop.initial
		case kwRevert, kwRevertLayer:
			// Reverting to the previous origin is not implemented. Treating it
			// as "unset" is the closest available answer and is wrong whenever a
			// user-agent rule set the property, so it is reported rather than
			// quietly substituted.
			//
			// "revert-layer" is read the same way. It rolls back to the
			// cascade layers below the declaration's own, and to the previous
			// origin only where there are none, so it differs from "unset"
			// wherever a lower layer set the property as well. It was not
			// recognised at all, so a declaration using it was read as a value
			// of the property and dropped for not being one: "color:
			// revert-layer" left the colour the *earlier* declaration had set,
			// which is the opposite of what it asks for.
			said := strings.ToLower(value)
			lower := "a lower-priority stylesheet"
			if said == kwRevertLayer {
				lower = "a lower cascade layer or a lower-priority stylesheet"
			}
			if !s.suppressed(said) {
				s.report(Finding{
					Offset: -1,
					Message: "\"" + said + "\" is not implemented and was read as \"unset\", " +
						"which differs wherever " + lower + " set the property",
					Unsupported: true,
					Property:    name,
				})
			}
			if prop.inherits {
				return inheritFrom()
			}
			return prop.initial
		}
		return value
	}

	if prop.inherits {
		return inheritFrom()
	}
	return prop.initial
}

// ruleSet is the prepared rules, with an index from what an element carries —
// its id, its classes and its name — to the rules that could select it.
//
// Every element used to be matched against every rule. The subject compound of
// a selector is tested for its type before anything else, so most of those
// comparisons failed on the first byte — but there were a great many of them:
// the user agent sheet alone is two hundred selectors, and the cost is the
// product of that and the document.
//
// The index is built once per document and asked once per element, which is the
// part that matters. The first attempt asked *per rule* instead — a set on each
// rule, tested in the loop — and was ten per cent slower than no filter at all,
// because hashing the element's name two hundred times costs more than two
// hundred failed byte comparisons. A few lookups, then short slices.
//
// It filed rules under element names only, so every rule whose subject was a
// class or an id — which is most of what an author writes — was matched against
// every element of the document. An element carries one id and a handful of
// classes, and a rule that needs one it does not carry cannot select it, so
// those rules are filed under the id or class and reached only through it.
type ruleSet struct {
	rules []preparedRule
	// byID, byClass and byName hold, per id, class and element name, the rules
	// filed under it — see keysOf for which. A rule is filed once per selector,
	// so a rule of three selectors may be in three lists, and an element that
	// reaches it through two of them is still given it once: see candidates.
	byID    map[string][]int32
	byClass map[string][]int32
	byName  map[string][]int32
	// any holds the rules that can select anything, which is every rule with a
	// selector that needs none of the three. They are walked for every element.
	any []int32

	// seen, stamp and the last pair are candidates' own state: which rules the
	// element in hand has already been given, and the answer for the element
	// asked about last. An element is asked about once for its own style and
	// again for each pseudo-element, one after the other.
	seen     []uint32
	stamp    uint32
	lastNode *html.Node
	last     []int32
}

// ruleKey is one thing an element must carry for a selector to select it.
type ruleKey struct {
	kind ruleKeyKind
	name string
}

type ruleKeyKind uint8

const (
	keyID ruleKeyKind = iota
	keyClass
	keyName
)

func newRuleSet(rules []preparedRule) *ruleSet {
	rs := &ruleSet{rules: rules, byID: map[string][]int32{},
		byClass: map[string][]int32{}, byName: make(map[string][]int32, 64),
		seen: make([]uint32, len(rules))}
	for i := range rules {
		rules[i].index = int32(i)
		if len(rules[i].keys) == 0 {
			rs.any = append(rs.any, int32(i))
			continue
		}
		for _, k := range rules[i].keys {
			switch k.kind {
			case keyID:
				rs.byID[k.name] = appendOnce(rs.byID[k.name], int32(i))
			case keyClass:
				rs.byClass[k.name] = appendOnce(rs.byClass[k.name], int32(i))
			default:
				rs.byName[k.name] = appendOnce(rs.byName[k.name], int32(i))
			}
		}
	}
	return rs
}

// appendOnce files a rule under a key it may already be filed under, by
// another of its selectors: "p, p.x" is two selectors and one rule.
func appendOnce(list []int32, i int32) []int32 {
	if n := len(list); n > 0 && list[n-1] == i {
		return list
	}
	return append(list, i)
}

// forEach calls fn for every rule that could select the element, in no
// particular order.
//
// Order does not matter and that is not an accident: every declaration carries
// its own order number, and beats decides between two candidates from that
// rather than from the sequence they were collected in.
func (rs *ruleSet) forEach(n *html.Node, fn func(r *preparedRule) bool) {
	for _, i := range rs.candidates(n) {
		if !fn(&rs.rules[i]) {
			return
		}
	}
}

// candidates is the rules forEach walks for an element, each once.
func (rs *ruleSet) candidates(n *html.Node) []int32 {
	if n == rs.lastNode && n != nil {
		return rs.last
	}
	rs.stamp++
	if rs.stamp == 0 {
		// Wrapped, after four billion elements: start the marks again rather
		// than let an old one read as this element's.
		clear(rs.seen)
		rs.stamp = 1
	}
	out := make([]int32, 0, len(rs.last))
	add := func(list []int32) {
		for _, i := range list {
			if rs.seen[i] != rs.stamp {
				rs.seen[i] = rs.stamp
				out = append(out, i)
			}
		}
	}
	name := asciiLowerName(n.Name)
	if name == "" {
		// A name this cannot fold, which is a name no HTML element has. Every
		// rule is considered rather than guessed about.
		for i := range rs.rules {
			out = append(out, int32(i))
		}
	} else {
		add(rs.any)
		add(rs.byName[name])
		if len(rs.byID) > 0 {
			if id, ok := n.Attr("id"); ok {
				add(rs.byID[id])
			}
		}
		if len(rs.byClass) > 0 {
			if class, ok := n.Attr("class"); ok {
				for _, c := range asciiFields(class) {
					add(rs.byClass[c])
				}
			}
		}
	}
	rs.lastNode, rs.last = n, out
	return out
}

// keysOf chooses, for each selector of a list, one thing an element must carry
// to be selected by it, or answers nothing when some selector needs none.
//
// The subject compound is what an element must satisfy, and of what it names
// the rarest is the best key: an id before a class before a name. Any of them
// is correct — the element has to carry all of them — and the choice is only
// about how short the list it lands in is. The id and class are compared
// exactly, as the matcher compares them (see hasClass).
//
// A selector whose subject names none of the three can select anything, and so
// can one whose type this cannot fold. The matcher compares a type with
// strings.EqualFold, which is Unicode's folding rather than ASCII's, and the two
// differ on characters no element name has — but "differ only on characters
// nobody uses" is not an argument for an index that decides whether a rule is
// looked at. A type with a byte above ASCII is filed under nothing and so is
// walked for every element, exactly as before.
func keysOf(sels []css.Selector) []ruleKey {
	keys := make([]ruleKey, 0, len(sels))
	for _, sel := range sels {
		if len(sel.Compounds) == 0 {
			return nil
		}
		c := sel.Compounds[len(sel.Compounds)-1]
		switch {
		case len(c.IDs) > 0:
			keys = append(keys, ruleKey{keyID, c.IDs[0]})
		case len(c.Classes) > 0:
			keys = append(keys, ruleKey{keyClass, c.Classes[0]})
		default:
			name := asciiLowerName(c.Type)
			if name == "" {
				return nil
			}
			keys = append(keys, ruleKey{keyName, name})
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// asciiLowerName lower-cases an element or type name, or answers empty for one
// that holds a byte the ASCII fold does not decide.
func asciiLowerName(s string) string {
	// The common case by far, and it must not allocate: this is asked once per
	// element per style computation, and a copy of every element name would
	// cost more than the loop it is saving.
	upper := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return ""
		}
		if upper < 0 && c >= 'A' && c <= 'Z' {
			upper = i
		}
	}
	if upper < 0 {
		return s
	}
	lower := []byte(s)
	for i := upper; i < len(lower); i++ {
		if c := lower[i]; c >= 'A' && c <= 'Z' {
			lower[i] = c + 'a' - 'A'
		}
	}
	return string(lower)
}

// matchSpecificityFor reports whether a rule applies to an element, and with what
// specificity.
//
// The two answers come together because they are one walk: asking "does it
// match" and then "how specific" would match every selector of every rule twice,
// for every element in the document.
//
// The specificity is that of the most specific selector that *matched*, not the
// most specific in the list. "a, #b {…}" applies to an <a> with the specificity
// of "a"; taking "#b" would let the rule beat declarations it should lose to.
func (s *Styler) matchSpecificityFor(r *preparedRule, n *html.Node, pseudo string) (css.Specificity, bool) {
	var best css.Specificity
	found := false
	for _, sel := range r.selectors {
		// A rule with a pseudo-element styles that and nothing else, and a rule
		// without one never styles a pseudo-element. Getting this backwards
		// gives every ::before its element's whole style twice over.
		if sel.PseudoElement != pseudo {
			continue
		}
		if !s.matchRule(r, sel, n) {
			continue
		}
		if !found || best.Less(sel.Specificity) {
			best, found = sel.Specificity, true
		}
	}
	return best, found
}

// The work one rule may spend on matching over a whole document: a base, and so
// much more for every try — every time one of its selectors is matched against
// an element.
//
// Per try because that is the work a rule is *meant* to cost: an
// ordinary rule is tried on many elements, and a hundred thousand of them should
// not switch off a rule that is cheap on each. The base is what lets a rule
// afford an expensive match or two on a short document — six at the per-match
// bound.
//
// Real selectors settle in tens of steps (see maxMatchSteps), and a descendant
// search is at most one walk of the ancestors per compound (see matchResult) —
// and the html package nests no deeper than 256 — so 256 a try on average is a
// long way from anything a stylesheet does on purpose. What reaches it is a rule
// whose matches run to the per-match bound, which is ten thousand steps: the
// audit's one kilobyte of such rules against thirty of markup spent 33 seconds
// on them.
const (
	ruleBaseSteps   = 1 << 16
	ruleStepsPerTry = 256
)

// matchBudget is the bound on matching work for a document, kept per rule.
//
// The bound per match (maxMatchSteps) stops one match running away and bounds
// nothing else: it is ten thousand steps for every pair of rule and element, and
// the pairs are the product of the stylesheet and the document. So each rule
// also has an allowance for the document, ruleBaseSteps and ruleStepsPerTry for
// every element it is tried against, and a rule that has spent more is switched
// off for the rest of the document and reported. The per-match bound becomes
// the early exit it always was, and the average is what is bounded: a rule may
// be expensive on a few elements and not on all of them.
//
// Per rule, and not one allowance for everything, because the bound has to land
// on what caused it. A single flag for the document is what the budget used to
// be, and one deep selector on one paragraph then turned matching off for every
// later rule and element, so the page was styled by whatever happened to come
// first. Scoped to the rule, a pathological selector costs the document that
// selector and nothing else. The total is the sum of the allowances: the rules
// times the base, and ruleStepsPerTry for every pair of rule and element that
// is tried at all — which the rule index keeps to the pairs that could match.
type matchBudget struct {
	// styled counts the elements styled so far, for saying where a rule was
	// switched off.
	styled int
	rules  []ruleWork
}

// ruleWork is one rule's account.
type ruleWork struct {
	spent int
	// tries counts the times one of the rule's selectors was matched against an
	// element, which is what its allowance grows with.
	tries int
	// trips counts matches of this rule that ran out of the per-match budget.
	trips int
	// off says the rule has spent its allowance, and offAt how many elements
	// had been styled when it did.
	off   bool
	offAt int
}

// allowance is what a rule tried against so many elements may spend.
func allowance(tries int) int { return ruleBaseSteps + ruleStepsPerTry*tries }

// matchRule matches one selector of a rule against an element, and charges
// what it cost to the rule.
//
// A rule that has spent its allowance is not matched at all. Its "no" is not
// an answer about the element, which is why switching it off is reported: see
// reportMatchBudget.
func (s *Styler) matchRule(r *preparedRule, sel css.Selector, n *html.Node) bool {
	b := s.budget
	if b == nil || int(r.index) >= len(b.rules) {
		return s.matcher.Match(sel, n)
	}
	w := &b.rules[r.index]
	if w.off {
		return false
	}
	got := s.matcher.Match(sel, n)
	steps, trips := s.matcher.takeWork()
	w.spent += steps
	w.tries++
	w.trips += trips
	if w.spent > allowance(w.tries) {
		w.off, w.offAt = true, b.styled
	}
	return got
}

// reportMatchBudget says which rules the matching budget cut short, one finding
// per rule, at the rule.
//
// Two ways: a rule that ran out of the per-match budget on some elements may be
// missing from them, and a rule switched off is missing from every element after
// the point it was. Both say what was not done, and both are about the rule an
// author can go and look at rather than about the document as a whole.
func (s *Styler) reportMatchBudget(rules *ruleSet) bool {
	b := s.budget
	if b == nil {
		return false
	}
	cut := false
	for i := range b.rules {
		w := &b.rules[i]
		if !w.off && w.trips == 0 {
			continue
		}
		cut = true
		r := &rules.rules[i]
		f := Finding{Offset: r.offset, Sheet: r.sheet}
		if w.off {
			f.Message = "matching this rule's selector cost more than this engine allows " +
				"(" + strconv.Itoa(w.spent) + " steps on " + strconv.Itoa(w.tries) +
				" elements, against " + strconv.Itoa(allowance(w.tries)) + "), so it was not " +
				"tried against any element after the " + ordinal(w.offAt) +
				"; elements after that which it selects are not styled by it"
		} else {
			f.Message = "matching this rule's selector against an element ran past the " +
				"bound on the work one match may take, " + strconv.Itoa(w.trips) +
				" times; it may be missing from elements it selects"
		}
		s.report(f)
	}
	return cut
}

// ordinal is n with its English suffix: 1st, 2nd, 3rd, 11th.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

// beats reports whether a wins over b, by CSS Cascade Level 4 §6.
//
// The order of the terms is the whole of it, and each one is only consulted when
// everything above it ties:
//
//  1. Importance and origin *together*. An important declaration reverses the
//     origin order, so an important user-agent rule beats an important author
//     one — which is how a user stylesheet can force a minimum contrast that a
//     page cannot override.
//  2. Specificity.
//  3. Order of appearance.
func beats(a, b candidate) bool {
	ao, bo := cascadeRank(a), cascadeRank(b)
	if ao != bo {
		return ao > bo
	}
	// The layer, which sits between the origin and the specificity: that is
	// what the feature is for, so that a rule in a later layer wins without
	// having to out-specify anything. Reaching here means the two agree on
	// origin and on importance, since CascadeRank tells every pair of those
	// apart, so one call decides the direction for both.
	if al, bl := layerRank(a.layer, a.important), layerRank(b.layer, b.important); al != bl {
		return al > bl
	}
	if a.spec != b.spec {
		return b.spec.Less(a.spec)
	}
	// Later wins. Equal orders cannot happen — every declaration gets its own
	// number — so this is a total order and the result does not depend on the
	// order candidates were collected in.
	return a.order > b.order
}

// cascadeRank is the combined importance-and-origin term, highest wins.
//
// Importance does not simply beat non-importance: it *inverts* the origin
// ordering. The sequence, weakest first, is user-agent, user, author, then
// important author, important user, important user-agent.
func cascadeRank(c candidate) int {
	return CascadeRank(c.origin, c.important)
}

// CascadeRank is that term for a declaration outside the cascade.
//
// It is exported because @page is decided by it and is not styling anything: an
// at-rule that describes the paper never reaches this file, and the caller that
// does read it has the same two declarations of the same margin to choose
// between. The rule belongs to the package that owns the cascade, and one
// definition of it is how a second reader cannot drift from the first.
func CascadeRank(origin Origin, important bool) int {
	if !important {
		return int(origin) // 0, 1, 2
	}
	// 3, 4, 5 with the origins reversed: author important is 3, user is 4,
	// user-agent is 5.
	return 3 + (int(OriginAuthor) - int(origin))
}

// inlineDeclarations reads an element's style attribute.
//
// The attribute holds a declaration list with no selector and no braces, so it
// is parsed as a block's contents rather than as a stylesheet.
func (s *Styler) inlineDeclarations(n *html.Node) map[string]preparedDecl {
	raw, ok := n.Attr("style")
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	decls, _, errs := css.ParseDeclarations(raw)
	for _, e := range errs {
		s.report(Finding{
			Offset: n.Offset, InMarkup: true,
			Message:     "in a style attribute: " + e.Message,
			Unsupported: e.Unsupported,
		})
	}

	// Everything expanded from here on is in the attribute, and says so.
	s.attrOffset = n.Offset
	defer func() { s.attrOffset = -1 }()

	out := map[string]preparedDecl{}
	for i, d := range decls {
		for _, e := range s.expand(d, OriginAuthor) {
			// Where it was written in the attribute. Nothing here needs it to
			// choose between two declarations of the same property — the loop
			// order does that — but renaming a logical property to a physical
			// one can put two of them in the same slot after the fact, and then
			// the order is the only thing that separates them. See
			// renameLogical.
			e.order = i
			// A later declaration in the same attribute wins, and importance
			// wins over its absence — the same rules as any other block, with
			// no specificity to separate them.
			if prev, ok := out[e.property]; ok && !inlineBeats(e, prev) {
				continue
			}
			out[e.property] = e
		}
	}
	return out
}

// inlineBeats reports whether one declaration in a style attribute wins over
// another of the same property there: importance first, and then the later of
// the two. It is the rule inlineDeclarations keeps and the one renameLogical
// needs when a logical and a physical spelling land on one property.
func inlineBeats(d, was preparedDecl) bool {
	if d.important != was.important {
		return d.important
	}
	return d.order > was.order
}

// vendorPrefixed reports whether a property name is one engine's rather than
// CSS's.
//
// The four prefixes are the ones CSS 2.1 §4.1.2.1 describes and the ones in use:
// a name beginning with "-" and a vendor identifier is reserved for that vendor,
// and no other engine is expected to know it.
//
// "-webkit-line-clamp" is prefixed and *is* implemented here, which is not a
// contradiction: this is only reached for a name nothing acts on, and a prefixed
// property the engine implements never gets that far.
func vendorPrefixed(name string) bool {
	// A leading "--" is not a prefix. CSS Variables §2 reserves that shape for
	// custom properties, which are a feature and not another engine's spelling
	// of one — see expand, which takes them before this is ever asked.
	if isCustomProperty(name) {
		return false
	}
	for _, prefix := range []string{"-webkit-", "-moz-", "-ms-", "-o-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	// The general form: a leading "-" followed by an identifier and another "-".
	// It catches the prefixes nobody has heard of, which is what the syntax
	// reserves the shape for.
	if len(name) > 1 && name[0] == '-' {
		return strings.Contains(name[1:], "-")
	}
	return false
}

// isCustomProperty reports whether a name is a custom property: CSS Variables
// §2's two leading dashes, which are reserved for exactly this.
func isCustomProperty(name string) bool { return strings.HasPrefix(name, "--") }

// unsetValue is the CSS-wide keyword "unset" as a value, which is what a
// declaration this engine cannot resolve computes to. See expand.
func unsetValue() []css.ComponentValue {
	return []css.ComponentValue{{Token: css.Token{Kind: css.Ident, Value: kwUnset}}}
}

// UsesVar is usesVar for a reader outside the cascade — @page's — that meets
// the same construct and has to give it the same answer: correct CSS this
// engine does not substitute, not a mistake.
func UsesVar(vals []css.ComponentValue) bool { return usesVar(vals) }

// usesVar reports whether a value refers to a custom property, at any depth. A
// var() inside a calc() inside a shorthand is still a value this engine cannot
// know.
func usesVar(vals []css.ComponentValue) bool {
	for _, v := range vals {
		if v.IsFunction() && strings.EqualFold(v.Token.Value, "var") {
			return true
		}
		if len(v.Values) > 0 && usesVar(v.Values) {
			return true
		}
	}
	return false
}

// winning is the declared value of a property on an element: the cascade's
// winner among the stylesheet candidates, or the inline style's declaration
// where it beats that winner. have is false where neither said anything.
//
// It is one function because two places ask it, and they drifted. computeFor's
// main loop decides every property with it, and the rename of logical
// properties has to know the element's writing mode and direction before that
// loop runs. The early question was answered by a copy of the old rule — the
// inline style wins unless the winner is important — after the loop had been
// corrected to the cascade's, so "div { direction: ltr !important }" with
// style="direction: rtl !important; margin-inline-start: 10px" computed
// direction rtl and put the margin on the left (audit C108).
func (s *Styler) winning(name string, winners map[string]candidate,
	inline map[string]preparedDecl) (string, bool) {

	value, have := "", false
	if c, ok := winners[name]; ok {
		value, have = c.text, true
	}
	if d, ok := inline[name]; ok {
		// A style attribute is an author declaration whose specificity is
		// above every selector — Cascade 4 §3.1 — so it is decided by the same
		// two terms every other declaration is, and only the second of them is
		// settled in advance.
		//
		// Importance is the first term and inverts the origins, so an important
		// inline declaration beats an important author rule and still loses to
		// an important user-agent one; a normal inline declaration loses to any
		// important rule. The specificity is the second and the inline always
		// wins it, which is why equal ranks go to the inline.
		//
		// It was read as "inline wins unless the author rule is important",
		// which said the opposite about the one case authors write it for:
		// "style=\"color: red !important\"" lost to a stylesheet's own important
		// rule.
		c, beaten := winners[name]
		if !beaten || CascadeRank(OriginAuthor, d.important) >= cascadeRank(c) {
			// Interned, because an attribute is read per element and its text
			// made afresh for each one.
			value, have = s.interner().value(d.text), true
		}
	}
	return value, have
}

// early is a property's computed value asked before the main loop computes it:
// winning, then inheritance and the CSS-wide keywords — the same two steps the
// loop takes. It is how the rename of logical properties learns the element's
// writing mode and direction.
func (s *Styler) early(name string, winners map[string]candidate,
	inline map[string]preparedDecl, parent ComputedStyle) string {

	value, have := s.winning(name, winners, inline)
	return strings.TrimSpace(s.resolve(name, properties[name], value, have, parent))
}
