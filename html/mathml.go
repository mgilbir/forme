package html

import (
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
)

// MathML in the tree.
//
// A <math> element used to be kept the way an <svg> still is: the element, and
// its content as unparsed source, because nothing here could lay MathML out
// and parsing it on spliced its text into the paragraph around it. MathML is
// laid out now, and its content is a tree like any other: styled by the
// cascade, selected by stylesheets, and laid out by what MathML Core says each
// element is. So the tree builder builds it, by HTML's rules for foreign
// content (§13.2.6.5) and the dispatcher that decides when they apply
// (§13.2.6):
//
//   - Inside MathML, a start tag makes a MathML element, whatever its name —
//     "<mrow><form>" is a MathML element called form — except for the tags
//     HTML breaks out for: a <p>, a <div>, a <table> and the rest of the list
//     below end the MathML they are written in and are read as HTML after it,
//     which is what every browser does with "<math><p>".
//   - The token elements mi, mo, mn, ms and mtext are "text integration
//     points": what is written inside one is HTML again, so
//     "<mtext><b>x</b></mtext>" holds a <b>. An <annotation-xml> whose encoding
//     is HTML's is an "HTML integration point" and holds HTML the same way,
//     and one holds an <svg> as SVG whatever its encoding.
//   - An end tag inside MathML closes the nearest open element of its name,
//     looking outward past the MathML — and at the first HTML element it
//     meets it is read by HTML's own rules instead.
//
// An XHTML document is XML, and its tree is the one its tags write: an element
// inside a <math> is MathML unless it says otherwise, by an xmlns attribute or
// a prefix naming another namespace. There are no break-outs and no
// integration points to decide.
//
// Everything HTML's stack rules ask by name — whether an element is "in
// scope", whether it is "special", which element an end tag closes — asks
// about HTML elements; the MathML token elements and <annotation-xml> are
// scope boundaries and special elements of their own (§13.2.4.2), which is
// what stops a "<p>" inside an <mtext> closing the paragraph the formula is
// written in. See isIn.

// mathMLSpecial are the MathML elements HTML's parser treats as "special" and
// as boundaries of every scope but table scope: the text integration points
// and annotation-xml.
var mathMLSpecial = setOf("mi", "mo", "mn", "ms", "mtext", "annotation-xml")

// mathMLTextIntegration are the text integration points.
var mathMLTextIntegration = setOf("mi", "mo", "mn", "ms", "mtext")

// breakOutOfForeign are the start tags that end foreign content in an HTML
// document: §13.2.6.5's list. <font> is on it only with a color, face or size
// attribute; see breaksOut.
var breakOutOfForeign = setOf(
	"b", "big", "blockquote", "body", "br", "center", "code", "dd", "div", "dl",
	"dt", "em", "embed", "h1", "h2", "h3", "h4", "h5", "h6", "head", "hr", "i",
	"img", "li", "listing", "menu", "meta", "nobr", "ol", "p", "pre", "ruby",
	"s", "small", "span", "strong", "strike", "sub", "sup", "table", "tt", "u",
	"ul", "var",
)

// isIn reports whether an open element is one of a set of HTML's element
// names, asked as HTML asks it: of an HTML element by its name, and of a
// MathML element only where it is one of the MathML elements the set names —
// which only the scopes and the special category do.
func isIn(set map[string]bool, n *Node) bool {
	switch n.Namespace {
	case NamespaceHTML:
		return set[n.Name] && !mathMLSpecial[n.Name]
	case NamespaceMathML:
		return mathMLSpecial[n.Name] && set[n.Name]
	}
	return false
}

// isHTML reports whether an open element is the HTML element of a name.
func isHTML(n *Node, name string) bool {
	return n.Namespace == NamespaceHTML && n.Name == name
}

// htmlIntegrationPoint is §13.2.6's HTML integration point, of the kinds that
// can be open here: an <annotation-xml> whose encoding is "text/html" or
// "application/xhtml+xml", compared ASCII case-insensitively. (SVG's are
// inside an <svg>, whose content is never on the stack.)
func htmlIntegrationPoint(n *Node) bool {
	if n.Namespace != NamespaceMathML || n.Name != "annotation-xml" {
		return false
	}
	enc, _ := n.Attr("encoding")
	return ascii.EqualFold(enc, "text/html") || ascii.EqualFold(enc, "application/xhtml+xml")
}

// foreignRules reports whether a tag token is read by the rules for foreign
// content rather than HTML's: §13.2.6's dispatcher, for the one kind of
// foreign element that is ever the current node.
func (p *parser) foreignRules(tk token) bool {
	cur := p.current()
	if cur == nil || cur.Namespace != NamespaceMathML && cur.Namespace != NamespaceOther {
		return false
	}
	if p.tok.xml || tk.kind == tokEndTag {
		return true
	}
	switch {
	case mathMLTextIntegration[cur.Name] && tk.name != "mglyph" && tk.name != "malignmark":
		return false
	case cur.Name == "annotation-xml" && tk.name == "svg":
		return false
	case htmlIntegrationPoint(cur):
		return false
	}
	return true
}

// breaksOut reports whether a start tag in foreign content ends it.
func breaksOut(tk token) bool {
	if breakOutOfForeign[tk.name] {
		return true
	}
	if tk.name != "font" {
		return false
	}
	for _, a := range tk.attrs {
		switch a.Name {
		case "color", "face", "size":
			return true
		}
	}
	return false
}

// popForeign pops the stack until the current node is an HTML element or an
// integration point: where HTML's rules take over again.
func (p *parser) popForeign() {
	for len(p.open) > 0 {
		cur := p.current()
		if cur.Namespace == NamespaceHTML || mathMLTextIntegration[cur.Name] || htmlIntegrationPoint(cur) {
			return
		}
		p.open = p.open[:len(p.open)-1]
	}
}

// foreignStartTag is a start tag read by the rules for foreign content.
func (p *parser) foreignStartTag(tk token) {
	ns := NamespaceMathML
	if p.tok.xml {
		ns = p.xmlNamespaceOf(tk)
		switch ns {
		case NamespaceHTML, NamespaceSVG:
			// Another vocabulary, which HTML's own rules read: an <svg> is kept
			// as its source, an HTML element is an HTML element.
			p.htmlStartTag(tk)
			return
		}
	} else if breaksOut(tk) {
		p.tok.fail(tk.offset, "<"+shown(tk.name)+"> cannot be inside MathML: it ends the "+
			"MathML it is written in and is read as HTML after it, as a browser reads it")
		p.popForeign()
		p.startTag(tk)
		return
	}
	el := p.insertForeign(tk, ns)
	if el == nil {
		return
	}
	if tk.selfClosing {
		// Foreign content has self-closing syntax, and "<mspace/>" is an empty
		// element: it is inserted and never opened.
		return
	}
	p.open = append(p.open, el)
	p.tooDeep(tk.offset)
}

// insertForeign makes an element in a namespace other than HTML's and puts it
// in the current node. Nothing written inside MathML is foster-parented: the
// MathML is the current node, not the table.
func (p *parser) insertForeign(tk token, ns Namespace) *Node {
	if !p.room(tk.offset) {
		return nil
	}
	el := p.element(tk.name, tk.offset)
	el.Namespace = ns
	el.Attrs = tk.attrs
	if ns == NamespaceMathML && !p.tok.xml {
		for i := range el.Attrs {
			el.Attrs[i].Name = AdjustMathMLAttributeName(el.Attrs[i].Name)
		}
	}
	p.current().appendChild(el)
	return el
}

// xmlNamespaceOf is the namespace an XHTML start tag inside MathML puts its
// element in: the one its prefix names; or, with no prefix, its default
// namespace — the one its own xmlns attribute declares, or the nearest open
// element's that declares one, or MathML's inside the <math> that declares
// none. A prefix this engine does not know names a namespace it does not know.
func (p *parser) xmlNamespaceOf(tk token) Namespace {
	uri := tk.ns
	switch {
	case uri != "":
	case strings.IndexByte(tk.name, ':') >= 0:
		return NamespaceOther
	default:
		uri = p.defaultNamespace(tk.attrs)
	}
	switch uri {
	case HTMLNamespaceURI:
		return NamespaceHTML
	case SVGNamespaceURI:
		return NamespaceSVG
	case MathMLNamespaceURI:
		return NamespaceMathML
	}
	return NamespaceOther
}

// defaultNamespace is the default namespace in force for a start tag carrying
// attrs: see xmlNamespaceOf.
func (p *parser) defaultNamespace(attrs []Attribute) string {
	for _, a := range attrs {
		if a.Name == "xmlns" {
			return a.Value
		}
	}
	for i := len(p.open) - 1; i >= 0; i-- {
		n := p.open[i]
		if v, ok := n.AttrExact("xmlns"); ok {
			return v
		}
		if n.Namespace == NamespaceMathML && n.Name == "math" {
			return MathMLNamespaceURI
		}
	}
	return HTMLNamespaceURI
}

// foreignEndTag is an end tag read by the rules for foreign content: "any
// other end tag", and in an HTML document the </br> and </p> that break out.
func (p *parser) foreignEndTag(tk token) {
	name := tk.name
	if !p.tok.xml && (name == "br" || name == "p") {
		p.tok.fail(tk.offset, "</"+name+"> cannot be inside MathML: it ends the MathML it is "+
			"written in and is read as HTML after it")
		p.popForeign()
		p.htmlEndTag(tk)
		return
	}
	i := len(p.open) - 1
	if !ascii.EqualFold(p.open[i].Name, name) {
		p.tok.fail(tk.offset, "</"+shown(name)+"> is written inside <"+shown(p.open[i].Name)+
			">, which is still open; tags have to nest")
	}
	for ; i > 0; i-- {
		n := p.open[i]
		if n.Namespace != NamespaceHTML && ascii.EqualFold(n.Name, name) {
			p.open = p.open[:i]
			return
		}
		if p.open[i-1].Namespace == NamespaceHTML {
			// The next element out is HTML, and HTML's rules say what the tag
			// does from here.
			p.htmlEndTag(tk)
			return
		}
	}
}
