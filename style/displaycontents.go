package style

import (
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
)

// What "display: contents" computes to where it is not the element's to give
// up its box.
//
// css-display-3 §2.5 defines the value, and in the same breath says where it
// does not hold:
//
//	This value computes to display: none on replaced elements and other
//	elements whose rendering is not entirely controlled by CSS; see
//	Appendix B for details.
//
// and §2.8 adds the one element it is never allowed on:
//
//	Additionally, a display of contents computes to block on the root
//	element.
//
// Both are computed values, so they are answered here, where the element is
// known, and not by each reader of the value. Layout, the counters, the
// guardrail and anything handed the styles see "none" or "block" and nothing
// has a list of its own to keep in step: before this, the box tree and the
// guardrail asked one predicate for which elements to refuse, laid each of
// them out as an inline box, and reported it.

var displayID = registry.ids["display"]

// contentsComputesToNone is Appendix B's first list, the HTML elements on
// which "display: contents computes to display: none". Each is either
// replaced or a form control whose content is a widget rather than its
// children — a <video>'s or a <canvas>'s children are fallback content that a
// rendering of the element never shows, so hoisting them in its place would
// draw what the element exists to hide.
//
// The Appendix's other HTML entries need nothing here, because they are what
// the value does anyway: <legend> "reacts to display: contents normally", and
// <button>, <details> and <fieldset> "don't have any special behavior;
// display: contents simply removes their principal box, and their contents
// render as normal".
var contentsComputesToNone = map[string]bool{
	"br": true, "wbr": true, "meter": true, "progress": true, "canvas": true,
	"embed": true, "object": true, "audio": true, "iframe": true, "img": true,
	"video": true, "frame": true, "frameset": true, "input": true,
	"textarea": true, "select": true,
}

// unusualDisplayContents is what display computes to on an element whose
// cascaded value is "contents", or "" where the value stands.
//
// root says the element is the document's root. It is asked first: §2.8 is a
// rule about the root whatever the element is, and an element the Appendix
// lists is no exception to it.
//
// An SVG-namespaced element is always one with CSS box layout in this engine,
// since the parser keeps an <svg>'s content as source and puts no SVG element
// in the tree below it — and for "an svg element that has CSS box layout" the
// Appendix's answer is none. Every MathML element is none as well: "For all
// MathML elements, display: contents computes to display: none."
func unusualDisplayContents(n *html.Node, root bool) string {
	if root {
		return "block"
	}
	switch n.Namespace {
	case html.NamespaceHTML:
		if contentsComputesToNone[ascii.Lower(n.Name)] {
			return "none"
		}
	case html.NamespaceSVG, html.NamespaceMathML:
		return "none"
	}
	return ""
}

// computeDisplayContents rewrites an element's computed display where
// unusualDisplayContents says the value is not "contents" on it.
func computeDisplayContents(b *styleBuilder, n *html.Node, root bool) {
	if !ascii.EqualFold(ascii.TrimCSSSpace(b.cs.Get("display")), "contents") {
		return
	}
	if to := unusualDisplayContents(n, root); to != "" {
		b.set(displayID, to)
	}
}
