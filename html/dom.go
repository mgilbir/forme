// Package html reads the subset of HTML this engine lays out.
//
// # A subset, refused at the edges
//
// This is not an HTML5 parser. HTML5's parsing algorithm is defined to accept
// *every* byte sequence and produce a tree from it, with several hundred
// recovery rules that reconstruct what an author probably meant. That
// definition exists because browsers must render the whole web, including two
// decades of documents nobody will ever fix.
//
// A document generator is not in that position. Its input is a template its
// caller wrote, and an unclosed tag there is a bug the caller wants to hear
// about — not something to be silently repaired into a tree that renders
// almost right. So this reads a declared subset and *refuses* what falls
// outside it. The cost is real and worth restating: markup a browser accepts,
// this will reject.
//
// Refusing is not the same as failing. Every refusal names what was wrong and
// where, and the ones that are correct-HTML-we-do-not-implement are marked
// apart from the ones that are malformed, because those send an author to
// different places.
//
// # What is deliberately absent
//
// No scripting, at any point, under any option. <script> and <embed> are
// dropped and reported, per §4.1, along with everything else whose content is
// produced by running something — see droppedElements for the list and a reason
// against each name. A renderer that quietly ignored them would still be one
// that had read them.
//
// Dropping the element is not the same as dropping its *box*, and the two were
// once confused here. An <iframe> and an <object> are replaced elements: their
// boxes are on the page whether or not anything was ever loaded into them, and
// they are in knownElements for those boxes. What would have been inside is not
// laid out — an iframe's children are the fallback a browser without frames
// would show, and no browser with them renders it.
//
// No network and no filesystem. Nothing here resolves a URL; an <img src> is
// recorded as written and left for the caller's resolver to decide about.
package html

import "strings"

// NodeType says what a node is. There are only three, because a renderer needs
// only three: the document, its elements, and the text in them.
//
// Comments are not among them. They are dropped rather than recorded, which
// costs nothing — nothing in layout, painting or the tagged-PDF structure has
// any use for one — and saves every consumer from walking past them.
type NodeType uint8

const (
	// DocumentNode is the root. It has exactly one element child, <html>.
	DocumentNode NodeType = iota
	// ElementNode is a tag.
	ElementNode
	// TextNode is character data. Adjacent runs are merged, so no element ever
	// has two text children in a row.
	TextNode
)

// Attribute is one attribute of an element.
type Attribute struct {
	// Name is lowercased, because HTML attribute names are case-insensitive and
	// leaving the case as written would mean every consumer folding it again.
	Name string
	// Value has its character references resolved.
	Value string
}

// Node is one node of the document tree.
type Node struct {
	Type NodeType

	// Name is the element's tag name, lowercased. It is empty for the other two
	// kinds.
	Name string

	// Attrs is in source order, with duplicates already refused, and holds at
	// most maxAttributes: the rest are dropped and reported, because every
	// lookup is a walk of this list.
	Attrs []Attribute

	// Text is the character data of a TextNode, with references resolved.
	Text string

	// Parent is nil for the document node.
	Parent *Node

	// Children is in source order.
	Children []*Node

	// XML says the source was XHTML rather than HTML. It is set on the document
	// node and nowhere else — see XMLDocument, which is how anything else asks.
	//
	// One thing in this package depends on it and one thing outside does. Here
	// it is that a <style> element holds ordinary character data, so "&gt;" in a
	// stylesheet is a ">"; outside, it is that an attribute name is
	// case-sensitive in XML and is not in HTML, which is what attr() in a
	// content property has to know. See looksLikeXML for how it is decided,
	// which is a guess about the source rather than a content type nobody gave
	// this engine.
	XML bool

	// Offset is the byte offset in the source at which the node begins, so a
	// finding from layout can point back at the markup that caused it. That is
	// what a finding needs to say *where* a guardrail fired, and it cannot be
	// recovered later.
	Offset int

	// Foreign is the unparsed source of a subtree that is not HTML, and is empty
	// for everything else.
	//
	// An <svg> element's children are SVG. They are not laid out as HTML — that
	// spliced their text into the flow around them, see foreignElements — and
	// they are not thrown away either, because the element is a replaced element
	// and this is its content. Keeping the source rather than a parsed tree is
	// deliberate: the reader that makes anything of it already reads bytes,
	// because an SVG referenced by <img> arrives as a file, and one reader for
	// the two is one set of rules about what an SVG may be.
	Foreign string
}

// Attr returns the value of an attribute and whether it was present. The name
// is matched lowercased, as HTML matches it.
func (n *Node) Attr(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	name = strings.ToLower(name)
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// AttrExact is Attr with the name matched as written.
//
// HTML lowercases an attribute name as it is parsed and XML does not, so a
// lookup that lowercases what it is given is right for one and wrong for the
// other: "attr(Title)" selects the title attribute of an HTML element and
// selects nothing at all in XHTML. content-attr-case-001 and -002 are the same
// document in the two languages and assert exactly that pair.
//
// The names in Attrs are lowercase either way — the tokenizer lowercases them —
// so what this does in practice is refuse a query that is not already lowercase.
// That is the right answer for the same reason: an XHTML document that really
// wrote "Title" has an attribute this engine has stored as "title" and cannot
// tell from one written that way, and refusing both is the answer that never
// invents a match.
func (n *Node) AttrExact(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Language is the language in force at a node: the value of the nearest lang
// attribute at or above it, and whether there was one.
//
// It is here rather than in each caller because it had five copies — the casing
// tailoring, the hyphenation patterns, the orthography, the writing system, and
// :lang() in the selector matcher — and the answer has to be the same in all
// five. They ask different questions *of* the tag; they must not ask different
// tags.
//
// **xml:lang is the same attribute.** HTML §3.2.6 says so: an element with an
// xml:lang in the XML namespace and no lang in no namespace takes its language
// from the xml:lang, and the precedence is per element rather than per document
// — a lang on a child beats an xml:lang on its parent because it is nearer, and
// loses to an xml:lang on the child itself only by being absent. Reading lang
// alone made "<div xml:lang='tr'>" a document with no language at all, which in
// XHTML — where xml:lang is the natural spelling and half the older test suite
// is written — turned the Turkish casing tailoring off and typeset the wrong
// letters.
//
// Only in a document that is XML. The HTML parser stores "xml:lang" as a
// literal attribute name with no namespace, and a browser reading an HTML
// document ignores it for exactly that reason; honouring it there would be a
// language this engine invents. XMLDocument is asked only once, and only when
// an xml:lang was found with no lang above it.
//
// **An empty value is an answer.** HTML §3.2.6.2: lang="" says the language is
// unknown, and it stops the walk as surely as a tag does — it is how an author
// marks a name or a code sample inside Turkish prose as not Turkish. It was
// skipped as though absent, so the parent's language reached text its author
// had marked as being in no language at all, and the Turkish casing, the
// hyphenation patterns and :lang(tr) all applied to it. It is returned as the
// empty string with ok true: every reader here maps the empty tag to its own
// "no language" answer.
//
// It walks every ancestor's attributes, which is right for one question and
// wrong for a question asked of every node in a tree: see Languages.
func (n *Node) Language() (string, bool) {
	for cur := n; cur != nil; cur = cur.Parent {
		if v, ok := cur.ownLanguage(cur.XMLDocument); ok {
			return v, true
		}
	}
	return "", false
}

// ownLanguage is the language a node itself declares, if it declares one: the
// one rule Language and Languages both walk, so that the two cannot come to
// answer differently. xml is asked only when an xml:lang is found without a
// lang beside it, which is the only time the answer depends on it.
func (n *Node) ownLanguage(xml func() bool) (string, bool) {
	if n.Type != ElementNode {
		return "", false
	}
	if v, ok := n.Attr("lang"); ok {
		return v, true
	}
	if v, ok := n.Attr("xml:lang"); ok && xml() {
		return v, true
	}
	return "", false
}

// Languages answers Language for the nodes of a tree that is not changing, each
// node's answer worked out once.
//
// Language is a walk to the root reading two attributes of every element on the
// way, and the layout asks it of every text node — its casing, its hyphenation,
// its writing system, several times each — while :lang() asks it of every
// element a rule is tried on. A paragraph of nested spans, each holding a word,
// cost the square of its depth times the attributes on each span: sixty spans
// with twenty attributes each took 6 ms to lay out, and 240 took 68.
//
// A node's language is its own declaration or else its parent's answer, so the
// memo is keyed by the node and filled on the way back down a walk that stops
// at the first node already answered. Every node is walked past once however
// many times it is asked about. Whether an xml:lang counts depends on the
// document the node is in, and that is carried down the same way, from the
// document node to every answer below it. The answers are the tree's, which is
// why the memo belongs to one pass over one unchanging tree and is not kept on
// the nodes: the tree's fields are exported, and a caller may change them
// between passes.
//
// The zero value is ready to use. It is not safe for concurrent use.
type Languages struct {
	answers map[*Node]languageAnswer
}

type languageAnswer struct {
	tag      string
	declared bool
	// xml is whether the tree the node is in was read as XHTML, carried so
	// that a walk stopping at this node knows it without going on to the root.
	xml bool
}

// Of is n.Language(), memoized.
func (l *Languages) Of(n *Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if l.answers == nil {
		l.answers = map[*Node]languageAnswer{}
	}
	if a, ok := l.answers[n]; ok {
		return a.tag, a.declared
	}
	// Up to the first node answered, or past the root.
	var path []*Node
	var above languageAnswer
	for cur := n; cur != nil; cur = cur.Parent {
		if a, ok := l.answers[cur]; ok {
			above = a
			break
		}
		path = append(path, cur)
	}
	xml := func() bool { return above.xml }
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Type == DocumentNode {
			// What XMLDocument answers for everything below: the nearest
			// document node above decides, and nothing above one does.
			above.xml = path[i].XML
		}
		if v, ok := path[i].ownLanguage(xml); ok {
			above = languageAnswer{tag: v, declared: true, xml: above.xml}
		}
		l.answers[path[i]] = above
	}
	return above.tag, above.declared
}

// XMLDocument reports whether the node is in a document parsed as XHTML.
//
// It walks to the document node, which is the only one the flag is set on. The
// walk is over the depth of the tree and is asked once per attr() in a content
// property, which is a place no document has many of.
func (n *Node) XMLDocument() bool {
	for ; n != nil; n = n.Parent {
		if n.Type == DocumentNode {
			return n.XML
		}
	}
	return false
}

// HasAttr reports whether an attribute is present, whatever its value. It is
// the question a boolean attribute such as "hidden" asks.
func (n *Node) HasAttr(name string) bool {
	_, ok := n.Attr(name)
	return ok
}

// TextContent returns the concatenated text of a node and everything inside it.
//
// It is what a <style> element's stylesheet is read from. It walks
// iteratively, because the tree came from untrusted input and a recursive walk
// over a deep one would need the stack the parser's depth cap exists to
// protect.
func (n *Node) TextContent() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	stack := []*Node{n}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur.Type == TextNode {
			b.WriteString(cur.Text)
			continue
		}
		// Children are pushed in reverse so they come off in source order.
		for i := len(cur.Children) - 1; i >= 0; i-- {
			stack = append(stack, cur.Children[i])
		}
	}
	return b.String()
}

// Walk calls fn for the node and every node under it, in document order.
//
// It stops descending into a node for which fn returns false, which is what
// skipping a subtree needs — and it is iterative for the same reason
// TextContent is.
func (n *Node) Walk(fn func(*Node) bool) {
	if n == nil {
		return
	}
	stack := []*Node{n}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !fn(cur) {
			continue
		}
		for i := len(cur.Children) - 1; i >= 0; i-- {
			stack = append(stack, cur.Children[i])
		}
	}
}

// Element finds the first element with the given name, in document order.
func (n *Node) Element(name string) *Node {
	name = strings.ToLower(name)
	var found *Node
	n.Walk(func(c *Node) bool {
		if found != nil {
			return false
		}
		if c.Type == ElementNode && c.Name == name {
			found = c
			return false
		}
		return true
	})
	return found
}

// appendChild adds a child and sets its parent, which are always done together.
func (n *Node) appendChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

// childBefore is the node insertBefore would put a new child immediately after,
// or nil when the insertion would be at the front.
//
// It exists so that foster parenting can merge a run of text into the text
// already there, the way ordinary insertion does. Two adjacent text nodes are a
// shape no consumer should have to handle, and a table is the one place the
// parser inserts somewhere other than where it stands.
func (n *Node) childBefore(before *Node) *Node {
	at := n.childIndex(before)
	if at == 0 {
		return nil
	}
	return n.Children[at-1]
}

// insertBefore adds a child immediately in front of one already there, or at
// the end when that one is not a child of n.
//
// It exists for foster parenting, which is the one rule of HTML that puts a
// node somewhere other than where the parser stands. See parser.fosterParent.
func (n *Node) insertBefore(c, before *Node) {
	at := n.childIndex(before)
	c.Parent = n
	n.Children = append(n.Children, nil)
	copy(n.Children[at+1:], n.Children[at:])
	n.Children[at] = c
}

// childIndex is where a child is among n's children, or len(n.Children) when it
// is not one of them.
//
// It searches from the end, and that is the whole of what keeps foster
// parenting linear. Every node fostered out of a table goes immediately in
// front of it, so the table moves one place further from the *front* of its
// parent with each of them, and a search from the front walked past every node
// fostered before: "<table>" and a million "<br>" was half a million million
// comparisons. From the end, the only nodes between the table and the end of
// its parent are the tables it was itself fostered in front of — nothing else
// is put in that parent while the table is open, because the parser does not
// return to the parent until the table has been closed, and every one of those
// tables is open too, so they are bounded by maxDepth. The same bound is on the
// copy insertBefore makes of what follows the table, so an insertion costs at
// most that and usually one step.
func (n *Node) childIndex(c *Node) int {
	for i := len(n.Children) - 1; i >= 0; i-- {
		if n.Children[i] == c {
			return i
		}
	}
	return len(n.Children)
}
