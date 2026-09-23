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
// # Layers inside layers
//
// Layers form a tree (§6.4.3). "@layer a { @layer b {} }" and "@layer a.b {}"
// name the same layer, b inside a, and a layer is ordered among its siblings by
// where its name first appears inside its parent — not globally. The rules
// written directly in a layer, outside any of its sublayers, are an implicit last
// sublayer of it, so they beat the sublayers'. Flattened, the order is the tree
// walked children first: every sublayer, in order, then the layer's own rules.
//
// It used to be a counter: each full path got a place in the order the first
// time it was seen, so in
//
//	@layer framework, app;
//	@layer framework.base { ... }
//
// framework.base came third and beat app, which the author had ordered to win.
// The nested spelling of the same thing was reported as ordered flatly; the
// dotted spelling said nothing at all (audit C114). The order is worked out
// once every sheet has been prepared, because a later "@layer a.x;" can put a
// layer inside one that already has rules — see finishLayers.
//
// An @import carrying layer() does not put its sheet in a layer, and the sheet
// does not arrive either: import expansion takes a bare reference only, so one
// carrying a layer name — or a media query, or a supports() condition — is left
// in the stylesheet and reported as an at-rule that was not applied. That is
// what keeps this safe rather than subtly wrong: an imported sheet cannot land
// *unlayered* and beat the layers around it, because it does not land at all.
//
// A revert-layer value is not implemented: it is read as "unset", as "revert"
// is, and reported as not implemented where the cascade resolves it (see
// Styler.resolve). Neither is a quiet narrowing: one is a rule this engine
// declines to fetch and says so, the other a keyword it says it does not act
// on.

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

// LayerRank is layerRank for a caller outside the cascade that decides between
// two things written in layers — two @page declarations of one margin, two
// @font-face rules for one family — as the cascade decides between two
// declarations: higher wins. Layer is AtRule.Layer.
func LayerRank(layer int, important bool) int { return layerRank(layer, important) }

// layerNode is one cascade layer: its sublayers in the order their names first
// appeared inside it, and those names. An anonymous sublayer is in children and
// not in named, since nothing can name it again.
type layerNode struct {
	children []int
	named    map[string]int
}

// sublayer is the layer a possibly dotted name names inside a parent layer,
// creating each part the first time it is seen. The order a name is *first
// mentioned* is the order of its layer among its siblings, which is what makes
// the statement form worth having: "@layer base, theme;" at the top of a sheet
// fixes the order before either block is written.
func (s *Styler) sublayer(parent int, dotted string) int {
	at := parent
	for _, part := range strings.Split(dotted, ".") {
		node := s.layerAt(at)
		if id, seen := node.named[part]; seen {
			at = id
			continue
		}
		id := s.newLayer(at)
		if node.named == nil {
			node.named = map[string]int{}
		}
		node.named[part] = id
		at = id
	}
	return at
}

// newLayer adds a sublayer after its parent's others and returns it.
func (s *Styler) newLayer(parent int) int {
	// The parent first: the root is made on first use, and a number taken
	// before it exists would be the root's own, making the first layer its
	// own parent — a cycle the flattening walk would follow for ever.
	p := s.layerAt(parent)
	id := len(s.layers)
	s.layers = append(s.layers, &layerNode{})
	p.children = append(p.children, id)
	return id
}

// layerAt is a layer by number, the root — the unlayered band — being zero.
func (s *Styler) layerAt(id int) *layerNode {
	if len(s.layers) == 0 {
		s.layers = []*layerNode{{}}
	}
	return s.layers[id]
}

// finishLayers turns every layer number the preparation gave out into its
// place in the flattened order, once all of it is known: the tree walked
// children first, so that each layer comes after all its sublayers. The
// unlayered band stays zero, which layerRank puts above every layer.
//
// The walk is iterative: a dotted name can nest a layer as deep as it is long,
// and that is not a depth to recurse to. It enters each layer once. A tree
// visits each node once anyway, so the check changes nothing for one; what it
// changes is a tree that is not one, which newLayer once built (see
// TestTheLayerTreeIsATree), and which an unchecked walk follows round its
// cycle until the process runs out of memory rather than until it is done.
func (s *Styler) finishLayers(out []preparedRule) {
	if len(s.layers) < 2 {
		return
	}
	rank := make([]int, len(s.layers))
	entered := make([]bool, len(s.layers))
	entered[0] = true
	type frame struct{ id, next int }
	stack := []frame{{0, 0}}
	k := 0
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		kids := s.layers[top.id].children
		if top.next < len(kids) {
			child := kids[top.next]
			top.next++
			if !entered[child] {
				entered[child] = true
				stack = append(stack, frame{child, 0})
			}
			continue
		}
		if top.id != 0 {
			k++
			rank[top.id] = k
		}
		stack = stack[:len(stack)-1]
	}
	for i := range out {
		out[i].layer = rank[out[i].layer]
	}
	for i := range s.pages {
		s.pages[i].Layer = rank[s.pages[i].Layer]
	}
	for i := range s.fontFaces {
		s.fontFaces[i].Layer = rank[s.fontFaces[i].Layer]
	}
}

// prepareLayer prepares an @layer, in either of its two forms.
//
// The block form puts its rules in the named layer; the statement form names
// layers in order and has no rules of its own. A name is inside the layer the
// rule is written in, so "@layer a { @layer b {} }" is "a.b".
func (s *Styler) prepareLayer(rule css.Rule, parent *css.Nesting, origin Origin,
	out *[]preparedRule, order *int) {

	names, ok := layerNames(rule.Prelude)
	if !ok {
		s.report(Finding{
			Offset: rule.Offset,
			Message: "@layer " + quoted(serialize(rule.Prelude)) +
				" is not a list of layer names, so the rule was dropped",
			Property: "@layer",
		})
		return
	}

	if !rule.HasBlock {
		// "@layer a, b;" — an order, and nothing else. Naming them is the whole
		// of its effect.
		for _, name := range names {
			s.sublayer(s.layer, name)
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

	was := s.layer
	if len(names) == 0 {
		s.layer = s.newLayer(s.layer)
	} else {
		s.layer = s.sublayer(s.layer, names[0])
	}
	defer func() { s.layer = was }()

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

// layerNames reads the comma-separated list an @layer names.
//
// An empty prelude is the anonymous form and returns nothing. A name is
// Cascade 5's <layer-name>: an ident, or idents joined by full stops with
// nothing between them, for a sublayer named in one go. ok is false for a
// prelude that is not such a list — "a b", "a..b", ".a", "a,,b" — which makes
// the rule invalid. Whitespace was skipped wherever it fell, so "@layer a b {…}"
// applied its rules in a layer called "ab" (audit C158).
func layerNames(vals []css.ComponentValue) (names []string, ok bool) {
	it := trimWhitespace(vals)
	if len(it) == 0 {
		return nil, true
	}
	for _, part := range splitOnComma(it) {
		part = trimWhitespace(part)
		if len(part) == 0 || len(part)%2 == 0 {
			return nil, false
		}
		var name strings.Builder
		for i, v := range part {
			if i%2 == 0 {
				if !v.IsToken() || v.Token.Kind != css.Ident {
					return nil, false
				}
				name.WriteString(v.Token.Value)
				continue
			}
			if !v.IsToken() || !v.Token.IsDelim('.') {
				return nil, false
			}
			name.WriteByte('.')
		}
		names = append(names, name.String())
	}
	return names, true
}
