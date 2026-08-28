package style

import (
	"sort"
	"strings"

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
}

// A Finding is something the styling stage noticed and a caller should hear
// about.
//
// It is the same shape as the css and html packages' Error, and for the same
// reason: an author needs to tell "I wrote this wrongly" from "this engine does
// not do that". The layer that turns these into pdf0.Violation values lands with
// the guardrail framework in phase 3; until then this carries the information so
// that nothing has to be reconstructed later.
type Finding struct {
	// Offset is the byte offset in the stylesheet the finding came from.
	Offset int
	// Message says what happened.
	Message string
	// Unsupported marks correct CSS this engine does not implement — the
	// unsupported-property finding of the proposal's §6.3 — as against a
	// stylesheet that is malformed.
	Unsupported bool
	// Property is the declaration's name, when the finding is about one.
	Property string
}

// maxFindings bounds the report, so a stylesheet full of properties this engine
// does not implement produces a list a person can read.
const maxFindings = 200

// A declaration that matched an element, with everything the cascade sorts on.
type candidate struct {
	property  string
	value     []css.ComponentValue
	important bool
	origin    Origin
	spec      css.Specificity
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
	matcher  *Matcher
	findings []Finding
	// seen suppresses repeat reports of the same unsupported property. A
	// stylesheet using "flex-wrap" forty times is one thing an author needs to
	// be told, not forty.
	seen map[string]bool
}

// ComputedStyle is one element's resolved property values, keyed by property
// name. Every property in the registry is present, so a caller never has to
// distinguish "unset" from "absent".
type ComputedStyle map[string]string

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
	// rules did not get the chance to apply. A caller rendering an incomplete
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
	XHeight(cs ComputedStyle, size Unit) (Unit, bool)
}

// Apply computes a style for every element in a document.
func Apply(doc *html.Node, sheets []Sheet) Styled {
	return ApplyWith(doc, sheets, nil)
}

// ApplyWith is Apply with a source for the font metrics a font-size may need.
// See Metrics. A nil source is the same as Apply.
func ApplyWith(doc *html.Node, sheets []Sheet, m Metrics) Styled {
	s := &Styler{matcher: NewMatcher(doc), seen: map[string]bool{}}

	// Expand shorthands and drop what the engine does not implement, once for
	// the whole run rather than once per element — the answer does not depend
	// on the element, and a document of ten thousand nodes would otherwise ask
	// the same question ten thousand times.
	rules := s.prepare(sheets)

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
	rootSize := initial
	rootSeen := false

	// Document order, so a parent is always computed before its children and
	// inheritance can read the parent's finished values.
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		cs, own := s.computeFor(n, rules, out.Styles, "")

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
		if p := parentElement(n); p != nil {
			if got, ok := out.Styles[p]; ok {
				fontStyle = got
			}
		}
		size, resolved := fontSizeOf(cs, own, parentSize, rootSize, m, fontStyle)
		sizes[n] = size
		if !rootSeen {
			rootSize, rootSeen = size, true
		}
		if resolved {
			// Written back, so that what is stored is the computed value: an
			// absolute length, which is what a descendant inherits. When it
			// could not be resolved the declaration is left as the author wrote
			// it, for layout to report against the element.
			cs["font-size"] = pxValue(size)
		}
		absolutiseLengths(cs, size, rootSize)

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
			pcs, own := s.computeForPseudo(n, rules, out.Styles[n], name)
			// A pseudo-element's em is relative to its own font-size, and it
			// inherits from the element it belongs to rather than from that
			// element's parent.
			// A pseudo-element's ex is its originating element's, for the same
			// reason its em is: it inherits from that element and not from that
			// element's parent.
			psize, presolved := fontSizeOf(pcs, own, size, rootSize, m, cs)
			if presolved {
				pcs["font-size"] = pxValue(psize)
			}
			absolutiseLengths(pcs, psize, rootSize)
			out.Pseudo[key] = pcs
			if own {
				out.OwnPseudoFontSize[key] = true
			}
		}
		return true
	})

	out.Findings = s.findings
	out.Incomplete = s.matcher.Tripped()
	if out.Incomplete {
		s.report(Finding{
			Message: "matching stopped early: some rules did not get the chance " +
				"to apply, so this document is styled less than its stylesheet describes",
		})
		out.Findings = s.findings
	}
	return out
}

// preparedRule is a rule with its selectors parsed and its declarations
// expanded, ready to be matched against every element.
type preparedRule struct {
	selectors []css.Selector
	decls     []preparedDecl
	origin    Origin
}

type preparedDecl struct {
	property  string
	value     []css.ComponentValue
	important bool
	order     int
	offset    int
}

func (s *Styler) prepare(sheets []Sheet) []preparedRule {
	var out []preparedRule
	order := 0

	for _, sheet := range sheets {
		for _, rule := range sheet.Rules {
			if rule.At {
				// At-rules are a stage of their own — @media has to be
				// evaluated against the page it is being laid out for, and
				// @page describes the surface rather than the content. Neither
				// belongs in the cascade, and reporting them here is how their
				// absence stays visible until they arrive.
				s.report(Finding{
					Offset:      rule.Offset,
					Message:     "@" + rule.Name + " is not applied yet",
					Unsupported: true,
					Property:    "@" + rule.Name,
				})
				continue
			}

			sels, errs, ok := css.ParseSelectorList(rule.Prelude)
			for _, e := range errs {
				s.report(Finding{
					Offset:      e.Offset,
					Message:     e.Message,
					Unsupported: e.Unsupported,
				})
			}
			if !ok {
				// An unusable selector list invalidates the rule, which is what
				// the specification requires — and the findings above already
				// said why.
				continue
			}

			decls, _, derrs := css.ParseDeclarationValues(rule.Block)
			for _, e := range derrs {
				s.report(Finding{Offset: e.Offset, Message: e.Message, Unsupported: e.Unsupported})
			}

			prepared := preparedRule{selectors: sels, origin: sheet.Origin}
			for _, d := range decls {
				for _, e := range s.expand(d, sheet.Origin) {
					e.order = order
					order++
					prepared.decls = append(prepared.decls, e)
				}
			}
			if len(prepared.decls) > 0 {
				out = append(out, prepared)
			}
		}
	}
	return out
}

// expand turns one declaration into the longhands it sets, dropping and
// reporting anything the engine does not implement.
func (s *Styler) expand(d css.Declaration, origin Origin) []preparedDecl {
	name := strings.ToLower(d.Name)

	if nonNegative[name] && hasNegativeNumber(d.Value) {
		// A declaration whose value is illegal is not a declaration with a
		// strange value: CSS 2.1 §4.2 says the whole declaration is dropped, and
		// what stands is whatever the cascade would have produced without it.
		//
		// That is why this cannot be done where the value is read. "height: 0;
		// height: -1px" has to compute to zero, and a layout that refuses the
		// negative number sees only the last declaration and falls back to
		// auto — which is a full-height box where the author asked for none.
		// The suite has thirty-five tests of exactly that shape, one per
		// property per unit, and they are what found it.
		//
		// The finding is not marked unsupported. Nothing is missing from the
		// engine here; a stylesheet said something CSS forbids and CSS says
		// what to do about it.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"" + name + ": " + serialize(d.Value) + "\" is negative, which " +
				name + " does not allow, so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if colourValued[name] && !legalColour(name, d.Value) {
		// §4.2 again, and the same reason it cannot wait until the value is
		// read: "color: 'red'" is a string where a colour belongs, so the
		// declaration is invalid and is dropped, and what stands is whatever the
		// cascade would have produced without it.
		//
		// Read at use time instead, the invalid declaration is still the winning
		// one — it has the higher specificity, that is why it is there — and the
		// colour comes out as the property's initial value. colors-007 is four
		// paragraphs that must each be green, and two of them came out black:
		// the lower-specificity "p.incorrect { color: green }" never got to
		// apply, because the declaration that should have been thrown away was
		// still standing in front of it.
		//
		// Not marked unsupported. Nothing is missing from the engine; a
		// stylesheet said something CSS forbids and CSS says what to do about
		// it.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"" + name + ": " + serialize(d.Value) + "\" is not a colour, " +
				"so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if name == "background-image" && !legalBackgroundImage(d.Value) {
		// §4.2 a third time. "background-image: url(x.png) repeat" is a
		// background-repeat value written where only an <image> belongs, so
		// there is no declaration here at all and nothing paints — which is
		// what every browser shows, and is why this is not a gap in the engine.
		//
		// It matters that this is not the unsupported report the painter would
		// otherwise raise. That report says "a browser draws something here and
		// this does not", and the whole reftest ratchet is built on the
		// difference: CSS2/backgrounds/background-image-005 asks for green text
		// and gets it, and was counted as a vacuous pass for years because the
		// engine claimed to be missing an image no engine draws.
		//
		// Not marked unsupported, for the reason the checks above are not.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"background-image: " + serialize(d.Value) + "\" is not an " +
				"image, so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if name == "display" && !legalDisplay(d.Value) {
		// §4.2 once more, and this one has a visible cost in the other
		// direction. An engine that reads an unrecognised display value as the
		// property's *initial* value makes the element inline, and the initial
		// value is what CSS says the property means when nobody has set it —
		// which is not this case. The declaration is invalid, so it never
		// happened, and what stands is what the cascade would have produced
		// without it: the user agent sheet's "div { display: block }".
		//
		// The two answers are as far apart as they can be. CSS2/abspos/
		// static-fixed-inside-abspos writes "display: absolute" — the author
		// meant "position" — on a div whose background is the green square the
		// test is about. Read as inline, the div has no in-flow content, so it
		// has no line box, so nothing of it is painted at all and the page is
		// the red square underneath.
		//
		// It is also what makes the prefixed idiom work. An author who writes
		// "display: -moz-box; display: flex" is relying on the first
		// declaration being thrown away by everything that does not know it,
		// and an engine that instead lets it stand as "inline" gets neither.
		//
		// Not marked unsupported, for the reason the checks above are not:
		// nothing is missing here, a stylesheet said something CSS forbids and
		// CSS says what to do about it.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"display: " + serialize(d.Value) + "\" is not a display " +
				"value, so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if name == "quotes" && !legalQuotes(d.Value) {
		// §12.3.2's grammar is "[<string> <string>]+ | none", so an odd number of
		// strings names a level with an opening mark and no closing one and is not
		// a value at all. §4.2 drops it, and dropping it here rather than where it
		// is read is what makes the *inherited* pairs stand: a child of an element
		// that set two good pairs must go on using them, and an engine that fell
		// back to the initial value at read time would quote the child in a
		// different alphabet from its parent.
		//
		// Not marked unsupported, for the same reason the negative lengths above
		// are not: nothing is missing from the engine, and CSS says what to do.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"quotes: " + serialize(d.Value) + "\" is not a list of pairs of " +
				"strings, so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if name == "content" && !legalCounterFunctions(d.Value) {
		// §12.2's grammar gives the two counter functions fixed argument lists:
		//
		//	counter(<identifier>) | counter(<identifier>, <list-style-type>)
		//	counters(<identifier>, <string>) | counters(<identifier>, <string>, <list-style-type>)
		//
		// so "counter(c, '.')" names a separator on the function that has none,
		// and "counter(c, decimal, decimal)" gives two styles to a function that
		// takes one. Neither is a value, and §4.2 drops the declaration.
		//
		// Dropping it here is what makes the *earlier* declaration stand, which
		// is the whole of what a test of this can observe: content-counter-016
		// writes "content: counter(c)" and then four malformed ones after it,
		// and requires the numbering to come out 1 to 12 from the first. Read at
		// use time instead, the last declaration is the only one left and the
		// page numbers every item 1000.
		//
		// Not marked unsupported, for the same reason the two checks above are
		// not: nothing is missing from the engine, and CSS says what to do.
		s.report(Finding{
			Offset: d.Offset,
			Message: "\"content: " + serialize(d.Value) + "\" calls a counter " +
				"function with arguments it does not take, so the declaration was dropped",
			Property: name,
		})
		return nil
	}

	if _, ok := properties[name]; ok {
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
			origin != OriginUserAgent && !s.seen[name] &&
			!isInertDeclaration(name, d.Value) {
			s.seen[name] = true
			s.report(Finding{
				Offset: d.Offset,
				Message: "the property \"" + name + "\" is not implemented, so " +
					reason,
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
			if !s.seen[key] {
				s.seen[key] = true
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
			s.report(Finding{
				Offset:   d.Offset,
				Message:  "\"" + name + ": " + serialize(d.Value) + "\" is not a value this engine can read",
				Property: name,
			})
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
	if !s.seen[name] {
		s.seen[name] = true
		s.report(Finding{
			Offset:      d.Offset,
			Message:     "the property \"" + name + "\" is not implemented, so it was not applied",
			Unsupported: !vendorPrefixed(name),
			Property:    name,
		})
	}
	return nil
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
// colourValued lists the properties whose whole value is a colour.
//
// A shorthand is not among them: "border" and "background" tell their parts
// apart by type, so a part that is not a colour is simply not the colour part,
// and the shorthand's own expander already refuses the declaration when nothing
// else will take it.
var colourValued = map[string]bool{
	"color": true, "background-color": true,
	"border-top-color": true, "border-right-color": true,
	"border-bottom-color": true, "border-left-color": true,
	"outline-color": true, "text-decoration-color": true,
}

// legalColour reports whether a value is one a colour property takes.
//
// The four CSS-wide keywords are not colours and are not this function's
// business — the cascade acts on them itself, and dropping "color: inherit" as
// an invalid colour would be a far worse bug than the one this fixes.
// "currentcolor" is a colour the cascade cannot resolve until it knows the
// element's own, and "invert" belongs to outline-color alone.
func legalColour(name string, vals []css.ComponentValue) bool {
	if parts := splitOnWhitespace(vals); len(parts) == 1 && len(parts[0]) == 1 {
		if v := parts[0][0]; v.IsToken() && v.Token.Kind == css.Ident {
			switch strings.ToLower(v.Token.Value) {
			case kwInherit, kwInitial, kwUnset, kwRevert, "currentcolor":
				return true
			case "invert":
				return name == "outline-color"
			}
		}
	}
	_, ok := ParseColor(vals)
	return ok
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
		case kwInherit, kwInitial, kwUnset, kwRevert:
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
// keyword, an inside keyword and "list-item". Being permissive is the safe
// direction here, because the cost of the two mistakes is not symmetric —
// keeping a value nobody implements gives the element the fallback it has
// always had, and dropping one that is really a display silently restores the
// user agent sheet's answer instead.
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
		case kwInherit, kwInitial, kwUnset, kwRevert:
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
	return outside <= 1 && inside <= 1 && item <= 1
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
// and this does not say so; calc is not implemented, so there is nothing here
// to be wrong about yet, and guessing at the sign of an expression that is not
// evaluated would drop declarations that are perfectly legal.
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

func (s *Styler) report(f Finding) {
	switch {
	case len(s.findings) > maxFindings:
		return
	case len(s.findings) == maxFindings:
		s.findings = append(s.findings, Finding{
			Message: "further styling problems were not reported",
		})
	default:
		s.findings = append(s.findings, f)
	}
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
// ::first-letter is still absent, because nothing reads it yet.
var pseudoElementNames = []string{"before", "after", "marker", "first-line"}

// anyRuleTargets reports whether any rule selects a pseudo-element of an
// element.
//
// It exists so that a pseudo-element with nothing said about it costs nothing: a
// document of ten thousand elements would otherwise compute three extra styles
// each, all of them the initial values, and generate nothing from any of them.
func (s *Styler) anyRuleTargets(rules []preparedRule, n *html.Node, name string) bool {
	for _, r := range rules {
		for _, sel := range r.selectors {
			if sel.PseudoElement != name {
				continue
			}
			if s.matcher.Match(sel, n) {
				return true
			}
		}
	}
	return false
}

// computeForPseudo resolves the properties of a pseudo-element.
//
// It inherits from the element it belongs to rather than from that element's
// parent, which is what makes "p { color: red } p::before { content: '>' }" draw
// a red marker without the author saying so twice.
func (s *Styler) computeForPseudo(n *html.Node, rules []preparedRule,
	owner ComputedStyle, name string) (ComputedStyle, bool) {
	return s.computeFor(n, rules, map[*html.Node]ComputedStyle{n: owner}, name)
}

// computeFor resolves every property for one element, or for one of its
// pseudo-elements when pseudo is not empty.
//
// It also reports whether font-size came from a declaration rather than by
// inheritance, which is the one thing a consumer cannot recover from the map it
// returns. See Styled.OwnFontSize for why that matters.
func (s *Styler) computeFor(n *html.Node, rules []preparedRule,
	done map[*html.Node]ComputedStyle, pseudo string) (ComputedStyle, bool) {

	var cands []candidate
	for _, r := range rules {
		spec, ok := s.matchSpecificityFor(r, n, pseudo)
		if !ok {
			continue
		}
		for _, d := range r.decls {
			cands = append(cands, candidate{
				property: d.property, value: d.value, important: d.important,
				origin: r.origin, spec: spec,
				order: d.order, offset: d.offset,
			})
		}
	}

	// The presentational hints of hints.go, at the very bottom of the author
	// origin: zero specificity and an order number below every declaration an
	// author wrote, so any author rule at all beats them and no user-agent rule
	// ever does. They belong to the element and not to its pseudo-elements,
	// which have no attributes of their own.
	if pseudo == "" {
		for property, value := range presentationalHints(n) {
			cands = append(cands, candidate{
				property: property, value: value,
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

	var parent ComputedStyle
	if pseudo != "" {
		// A pseudo-element inherits from its own element, which the caller put
		// in the map under that element's own key.
		parent = done[n]
	} else if p := parentElement(n); p != nil {
		parent = done[p]
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
	// Direction is resolved with the same three lines the main loop uses below,
	// because it is the same question — a winner, an inline style over it, and
	// inheritance under both.
	if renameLogical(cands, inline, s.isRTL(winners, inline, parent)) {
		pick()
	}

	out := make(ComputedStyle, len(properties))
	ownFontSize := false
	for name, prop := range properties {
		value, have := "", false

		if c, ok := winners[name]; ok {
			value, have = serialize(c.value), true
		}
		if d, ok := inline[name]; ok {
			// Inline wins over everything an author rule can say, important or
			// not — except an important author rule, which the cascade puts
			// above it. That case is rare enough, and the ordering subtle
			// enough, that it is spelled out rather than left to fall out.
			if c, ok := winners[name]; !ok || !c.important {
				value, have = serialize(d.value), true
			}
		}

		out[name] = s.resolve(name, prop, value, have, parent)
		if name == "font-size" {
			ownFontSize = have && declaresItsOwnValue(value, prop)
		}
	}
	return out, ownFontSize
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
	case kwUnset, kwRevert:
		return !prop.inherits
	}
	return true
}

// resolve turns a winning declaration — or the absence of one — into a computed
// value, applying the CSS-wide keywords and inheritance.
func (s *Styler) resolve(name string, prop property, value string, have bool, parent ComputedStyle) string {
	inheritFrom := func() string {
		if parent == nil {
			// The root has no parent to inherit from, so it takes the initial
			// value — which is what "the initial value" means for the root.
			return prop.initial
		}
		if v, ok := parent[name]; ok {
			return v
		}
		return prop.initial
	}

	if have {
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
		case kwRevert:
			// Reverting to the previous origin is not implemented. Treating it
			// as "unset" is the closest available answer and is wrong whenever a
			// user-agent rule set the property, so it is reported rather than
			// quietly substituted.
			if !s.seen["revert"] {
				s.seen["revert"] = true
				s.report(Finding{
					Message: "\"revert\" is not implemented and was read as \"unset\", " +
						"which differs wherever a lower-priority stylesheet set the property",
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

// matchSpecificity reports whether a rule applies to an element, and with what
// specificity.
//
// The two answers come together because they are one walk: asking "does it
// match" and then "how specific" would match every selector of every rule twice,
// for every element in the document.
//
// The specificity is that of the most specific selector that *matched*, not the
// most specific in the list. "a, #b {…}" applies to an <a> with the specificity
// of "a"; taking "#b" would let the rule beat declarations it should lose to.
func (s *Styler) matchSpecificityFor(r preparedRule, n *html.Node, pseudo string) (css.Specificity, bool) {
	var best css.Specificity
	found := false
	for _, sel := range r.selectors {
		// A rule with a pseudo-element styles that and nothing else, and a rule
		// without one never styles a pseudo-element. Getting this backwards
		// gives every ::before its element's whole style twice over.
		if sel.PseudoElement != pseudo {
			continue
		}
		if !s.matcher.Match(sel, n) {
			continue
		}
		if !found || best.Less(sel.Specificity) {
			best, found = sel.Specificity, true
		}
	}
	return best, found
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
	if !c.important {
		return int(c.origin) // 0, 1, 2
	}
	// 3, 4, 5 with the origins reversed: author important is 3, user is 4,
	// user-agent is 5.
	return 3 + (int(OriginAuthor) - int(c.origin))
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
			Offset: n.Offset, Message: "in a style attribute: " + e.Message,
			Unsupported: e.Unsupported,
		})
	}

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
			if prev, ok := out[e.property]; ok && prev.important && !e.important {
				continue
			}
			out[e.property] = e
		}
	}
	return out
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

// isRTL is whether this element's inline axis runs right to left, which is what
// turns a logical property into a physical one.
//
// It resolves "direction" exactly as computeFor's main loop resolves any
// property — the cascade's winner, an inline style above it unless the winner is
// important, and inheritance under both — because it is that same question asked
// early.
func (s *Styler) isRTL(winners map[string]candidate,
	inline map[string]preparedDecl, parent ComputedStyle) bool {

	value, have := "", false
	if c, ok := winners["direction"]; ok {
		value, have = serialize(c.value), true
	}
	if d, ok := inline["direction"]; ok {
		if c, ok := winners["direction"]; !ok || !c.important {
			value, have = serialize(d.value), true
		}
	}
	got := s.resolve("direction", properties["direction"], value, have, parent)
	return strings.EqualFold(strings.TrimSpace(got), "rtl")
}
