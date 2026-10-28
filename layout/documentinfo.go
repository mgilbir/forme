package layout

import (
	"fmt"
	"strings"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
)

// What a document says about itself.
//
// <head> is not rendered, so its title and its <meta> names are in neither the
// display list nor the fragment tree, and the document's language is a question
// layout answers for every text node without ever reporting the answer. A
// backend writing a file has a place for each of them — a title, a language,
// an author, a reading direction — and the only way to fill them was to parse
// the HTML a second time beside forme, with its own HTML-or-XHTML decision and
// its own limits, and a language that could disagree with the one the page was
// hyphenated in. Composed.Document is those facts from the parse Compose did.
//
// The values are the document's, after HTML's own normalisation and nothing
// more: a title has its ASCII whitespace collapsed, a meta name is lowercased.
// Whether a language tag is well formed, or a title too long for a format, is
// the backend's question; its grammar for each differs from HTML's.

// DocumentInfo is what a document says about itself: its title, language,
// direction and <meta> names.
type DocumentInfo struct {
	// Title is the document's title as HTML defines document.title: the child
	// text of the first title element, with ASCII whitespace stripped from both
	// ends and each run of it inside collapsed to one space. HasTitle tells a
	// document with no title element from one whose title is empty.
	Title    string
	HasTitle bool

	// Language is the root element's language, resolved by the rule layout
	// sets the text with — hyphenation, casing, :lang() — which is
	// html.Node.Language: lang, xml:lang in XHTML, then a Content-Language
	// pragma. It is the value as written and is not validated. HasLanguage
	// tells lang="", which says the language is unknown, from no declaration.
	Language    string
	HasLanguage bool

	// Direction is the root element's dir attribute: "ltr", "rtl", or empty
	// where it is absent, "auto", or not a value dir takes.
	Direction string

	// Meta is every <meta> in the head with both a name and a content
	// attribute, in document order, the name ASCII-lowercased: author,
	// description, keywords, generator and so on.
	Meta []MetaName
}

// MetaName is one <meta name content> of a document.
type MetaName struct {
	Name, Content string
	// Source is the element, for a backend that reports on it.
	Source Source
}

// maxMetaNames bounds DocumentInfo.Meta. The input is untrusted and a <meta>
// is a dozen bytes, so a document could hold millions; a real one has a few
// dozen. Past the bound the rest are left out and RuleLimit says so.
const maxMetaNames = 256

// documentInfoOf reads a parsed document's own description of itself.
func documentInfoOf(doc *html.Node, rec *Recorder) DocumentInfo {
	var info DocumentInfo
	root := rootElement(doc)
	if root == nil {
		return info
	}
	info.Language, info.HasLanguage = root.Language()
	if v, ok := root.Attr("dir"); ok {
		switch d := ascii.Lower(ascii.TrimCSSSpace(v)); d {
		case "ltr", "rtl":
			info.Direction = d
		}
	}
	if t := firstHTMLElement(doc, "title"); t != nil {
		info.Title, info.HasTitle = collapseASCIIWhitespace(childText(t)), true
	}
	head := firstHTMLElement(root, "head")
	if head == nil {
		return info
	}
	cut := false
	head.Walk(func(n *html.Node) bool {
		if cut || n.Type != html.ElementNode || n.Namespace != html.NamespaceHTML || n.Name != "meta" {
			return !cut
		}
		name, ok := n.Attr("name")
		if !ok {
			return false
		}
		content, ok := n.Attr("content")
		if !ok {
			return false
		}
		if len(info.Meta) == maxMetaNames {
			cut = true
			rec.ReportDetail(Finding{
				Rule:   RuleLimit,
				Source: sourceOf(n),
				Message: fmt.Sprintf("the document has more than %d <meta name> elements; "+
					"the rest are not in Composed.Document.Meta", maxMetaNames),
				Path: PathOf(n),
			})
			return false
		}
		info.Meta = append(info.Meta, MetaName{
			Name: ascii.Lower(name), Content: content, Source: sourceOf(n),
		})
		return false
	})
	return info
}

// rootElement is a document's element child, or nil.
func rootElement(doc *html.Node) *html.Node {
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

// firstHTMLElement is the first HTML element of a name at or under n, in tree
// order. An SVG <title> is the title of a drawing and not of the document —
// though the parser keeps an <svg>'s content as source (html.Node.Foreign), so
// none reaches this tree today, and no test can tell this from a match on the
// name alone.
func firstHTMLElement(n *html.Node, name string) *html.Node {
	var found *html.Node
	n.Walk(func(c *html.Node) bool {
		if found != nil {
			return false
		}
		if c.Type == html.ElementNode && c.Namespace == html.NamespaceHTML && c.Name == name {
			found = c
			return false
		}
		return true
	})
	return found
}

// childText is HTML's child text content: the element's own text children,
// joined, and not its descendants'. Both parsers keep a title's content as
// text, so the two are the same for every title forme reads.
func childText(n *html.Node) string {
	var b strings.Builder
	for _, c := range n.Children {
		if c.Type == html.TextNode {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// collapseASCIIWhitespace is HTML's "strip and collapse ASCII whitespace":
// tab, line feed, form feed, carriage return and space.
func collapseASCIIWhitespace(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\f' || r == '\r' || r == ' '
	}), " ")
}
