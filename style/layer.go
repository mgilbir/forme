package style

import (
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
)

// CSS Cascade 5 §6.4's @layer: a band of the cascade an author puts rules into,
// so that which rule wins is decided by where its layer sits rather than by how
// specific its selector is.
//
// Every rule inside one was dropped, and the block reported as an at-rule that
// is not applied. That is the worst of the three ways to be wrong about a
// cascade feature: a stylesheet written in layers — which is how a framework
// ships one — lost not some of its precedence but all of its rules.
//
// # The order, which is the whole of the feature
//
// Within one origin, §6.4.4 orders normal declarations so that an *unlayered*
// one beats every layered one, and among layers the one declared later wins.
// That is the point of the feature and the reason an author reaches for it: a
// layer is a promise that what is written outside it, or after it, is not going
// to be out-specified by what is inside.
//
// Importance reverses both halves, exactly as it reverses the origin order
// above: an important declaration in the *first* layer beats an important one in
// the second, and both beat an important unlayered one. The two reversals are
// the same rule applied to the two terms, which is why layerRank is written to
// look like CascadeRank.
//
// # What this does not do, and says so
//
// A *nested* @layer is given its own place in the order at the point it is
// first seen, rather than a place inside its parent. For the way layers are
// usually written the two agree — "@layer a { @layer x {} } @layer b {}" puts
// b last either way — and where they disagree is a document that fixes the
// order up front and fills it in afterwards:
//
//	@layer framework, app;
//	@layer framework { @layer base { ... } }
//
// Sorted within its parent, framework.base sits under framework and loses to
// app. Given its own place it is third and wins. This engine does the second
// and reports that it did, once per document, because the difference is a rule
// winning that the author ordered to lose and nothing about the page says so.
//
// The ordering *within* a parent is what is missing, not the layer: the rules
// still apply, and against everything outside their parent they are ordered
// correctly. Implementing it means a tree of layers rather than a counter, and
// a decision about where a layer's own rules sit relative to its sublayers that
// is worth reading the specification for rather than guessing at.
//
// An @import carrying layer() does not put its sheet in a layer, and the sheet
// does not arrive either: import expansion takes a bare reference only, so one
// carrying a layer name — or a media query, or a supports() condition — is left
// in the stylesheet and reported as an at-rule that was not applied. That is
// what keeps this safe rather than subtly wrong: an imported sheet cannot land
// *unlayered* and beat the layers around it, because it does not land at all.
//
// A revert-layer value is not implemented and is reported where every unknown
// value is. Neither is a quiet narrowing: one is a rule this engine declines to
// fetch and says so, the other a value it says it does not know.

// layerRank orders two declarations of the same origin and importance by the
// layer each was written in. Higher wins, as with CascadeRank.
//
// Unlayered is zero and is not a layer: it is the band above all of them for a
// normal declaration and below all of them for an important one.
func layerRank(layer int, important bool) int {
	if important {
		if layer == 0 {
			// Every layered important declaration beats an unlayered one.
			return math.MinInt32
		}
		// Earlier layers win, so the rank falls as the index rises.
		return -layer
	}
	if layer == 0 {
		return math.MaxInt32
	}
	return layer
}

// layerIndex is the number of a layer by name, assigning one the first time a
// name is seen.
//
// The order a name is *first mentioned* is the order of its layer, which is
// what makes the statement form worth having: "@layer base, theme;" at the top
// of a sheet fixes the order before either block is written, so a block written
// later cannot jump the queue by being written first.
func (s *Styler) layerIndex(name string) int {
	if s.layers == nil {
		s.layers = map[string]int{}
	}
	if at, seen := s.layers[name]; seen {
		return at
	}
	s.layerCount++
	s.layers[name] = s.layerCount
	return s.layerCount
}

// anonymousLayer is a layer with no name, which nothing can add to later.
func (s *Styler) anonymousLayer() int {
	s.layerCount++
	return s.layerCount
}

// prepareLayer prepares an @layer, in either of its two forms.
//
// The block form puts its rules in the named layer; the statement form names
// layers in order and has no rules of its own. A name written inside another
// layer is a sublayer of it — "@layer a { @layer b {} }" is "a.b" — and is
// ordered among its siblings rather than globally, which this reads by keeping
// the full path as the name.
func (s *Styler) prepareLayer(rule css.Rule, parent *css.Nesting, origin Origin,
	out *[]preparedRule, order *int) {

	names := layerNames(rule.Prelude)

	if !rule.HasBlock {
		// "@layer a, b;" — an order, and nothing else. Naming them is the whole
		// of its effect.
		for _, name := range names {
			s.layerIndex(s.layerPath(name))
		}
		return
	}
	if len(names) > 1 {
		// A block may name one layer. More than one is a parse error, and the
		// rule is dropped rather than guessed at.
		s.report(Finding{
			Offset: rule.Offset,
			Message: "@layer " + quoted(serialize(rule.Prelude)) +
				" names more than one layer and has a block; a block belongs to a " +
				"single layer, so the rule was dropped",
			Property: "@layer",
		})
		return
	}

	if s.layerName != "" || s.layer != 0 {
		// A layer inside a layer. See the note above: it is ordered as a layer
		// of its own rather than within its parent, which differs only for a
		// document that fixed the order before writing the block — and there it
		// differs by letting a rule win that was ordered to lose.
		s.reportNestedLayer(rule)
	}
	was, wasName := s.layer, s.layerName
	if len(names) == 0 {
		s.layer, s.layerName = s.anonymousLayer(), ""
	} else {
		s.layerName = s.layerPath(names[0])
		s.layer = s.layerIndex(s.layerName)
	}
	defer func() { s.layer, s.layerName = was, wasName }()

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

// layerPath is a name as it is known globally, under whatever layer is open.
func (s *Styler) layerPath(name string) string {
	if s.layerName == "" {
		return name
	}
	return s.layerName + "." + name
}

// layerNames reads the comma-separated list an @layer names.
//
// An empty prelude is the anonymous form and returns nothing. A name is an
// ident, or idents joined by full stops for a sublayer named in one go.
func layerNames(vals []css.ComponentValue) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if name := strings.TrimSpace(cur.String()); name != "" {
			out = append(out, name)
		}
		cur.Reset()
	}
	for _, v := range vals {
		switch {
		case v.Token.Kind == css.Comma:
			flush()
		case v.Token.Kind == css.Whitespace:
			// Between a name and a comma, and nowhere inside a name.
		case v.Token.Kind == css.Ident:
			cur.WriteString(v.Token.Value)
		case v.Token.Kind == css.Delim && v.Token.Value == ".":
			cur.WriteString(".")
		default:
			// Anything else makes the prelude unreadable; the caller reports a
			// block that named more than one layer, and a statement form with
			// nothing readable in it names nothing.
		}
	}
	flush()
	return out
}

// reportNestedLayer says that a layer inside a layer is ordered as its own
// rather than within its parent, once per document.
//
// Once, because a stylesheet that nests one nests many, and the thing to be
// told is that this engine orders them flatly — not which of them it did it to.
func (s *Styler) reportNestedLayer(rule css.Rule) {
	if s.reportedNestedLayer {
		return
	}
	s.reportedNestedLayer = true
	s.report(Finding{
		Offset: rule.Offset,
		Message: "a @layer written inside another is ordered as a layer of its " +
			"own rather than within the one it is nested in; where a document " +
			"fixes its layer order before filling the blocks in, a nested layer " +
			"can win against a layer its parent was ordered behind",
		Unsupported: true,
		Property:    "@layer",
	})
}
