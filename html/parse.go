package html

import (
	"strconv"
	"strings"
)

// The tree builder.
//
// HTML5's has twenty-three insertion modes, most of them describing how to
// rebuild a tree from tags that arrived in an impossible order. This one has
// none: tags that cannot nest are refused (see the package comment), and the
// only reordering it does is the one HTML actually defines — the optional end
// tags in elements.go, which are correct markup rather than recovery.

// The bounds on one document. A template is untrusted input, and each of these
// is a place where a few kilobytes of markup would otherwise cost unboundedly
// more.
const (
	// maxDepth bounds how deeply elements may nest. Each level is a stack frame
	// in anything that walks the tree recursively — layout does — and real
	// documents nest tens of levels, not hundreds.
	maxDepth = 256

	// maxNodes bounds the size of the tree. It is the cap that matters for
	// memory: a node is far larger than the markup that creates it, so "<b>"
	// repeated is an amplification.
	maxNodes = 1 << 20

	// maxInputBytes bounds the document itself. It is generous — a book's worth
	// of markup is a few megabytes — and exists so that the caps above are
	// never the first thing a runaway input meets.
	maxInputBytes = 64 << 20
)

// Parse reads a document.
//
// It returns the tree it built, everything it refused, and whether the document
// was read without refusing anything that changes what the page says. A caller
// that renders a document with ok false is rendering something other than what
// its author wrote, so the flag is not advisory.
//
// The tree is returned even when ok is false, because a caller showing an author
// what went wrong is better served by both than by either — which is the
// opposite of the selector parser's arrangement, and for the opposite reason:
// there, a partial result is silently applicable and so dangerous to hand back;
// here, the findings name what is missing and the tree cannot be mistaken for
// complete.
func Parse(src string) (doc *Node, errs []Error, ok bool) {
	if len(src) > maxInputBytes {
		return nil, []Error{{
			Offset: maxInputBytes,
			Message: "the document is larger than this engine will read (" +
				strconv.Itoa(len(src)) + " bytes, limit " + strconv.Itoa(maxInputBytes) + ")",
		}}, false
	}

	p := &parser{tok: newTokenizer(src), src: src}
	p.run()
	return p.doc, p.tok.errs, len(p.tok.errs) == 0
}

type parser struct {
	tok *tokenizer
	src string

	doc  *Node
	html *Node
	head *Node
	body *Node

	// open is the stack of elements not yet closed, innermost last.
	open []*Node

	nodes int
	// bodyStarted records that something has been put in the body, after which
	// head-only elements are out of place.
	bodyStarted bool
	// truncated records that a bound stopped the tree being built, so the
	// caller is never handed a short document that looks complete.
	truncated bool
	// stripNewline records that the element just opened is one whose first
	// character, if it is a line feed, HTML throws away. See dropFirstNewline.
	stripNewline bool
	// ns maps a namespace prefix to the URI it was bound to. See bindNamespaces.
	ns map[string]string
}

// dropFirstNewline are the elements HTML §13.2.6.4.7 ignores a leading line
// feed inside.
//
// The rule exists because their content is preserved white space and the
// newline after the start tag is punctuation of the *markup* rather than of the
// text. "<pre>\nhello</pre>" is how everyone writes a <pre>, and an engine that
// kept that newline would put a blank line at the top of every one — a whole
// line of paper, from a character the author did not mean to write.
//
// It is one newline and only the first, and it applies whatever the element's
// white-space property says: the rule is in the tree builder, before any
// stylesheet has been consulted.
var dropFirstNewline = map[string]bool{"pre": true, "textarea": true}

func (p *parser) run() {
	p.doc = &Node{Type: DocumentNode, XML: p.tok.xml}
	p.html = p.element("html", 0)
	p.doc.appendChild(p.html)
	p.head = p.element("head", 0)
	p.html.appendChild(p.head)
	p.body = p.element("body", 0)
	p.html.appendChild(p.body)
	p.open = []*Node{p.html, p.head}

	for {
		tk := p.tok.next()
		// HTML's rule is about "the next token" after the start tag, so the flag
		// is read and cleared once per token however that token turns out. A
		// start tag for <pre> sets it again below, after this line has already
		// taken the old value — which is what makes "<pre><pre>\nx" drop one
		// newline rather than none.
		strip := p.stripNewline
		p.stripNewline = false
		switch tk.kind {
		case tokEOF:
			p.finish()
			return
		case tokDoctype:
			// Nothing to do with it: there is one document model here, and it
			// is not chosen by a doctype.
		case tokText:
			if strip {
				tk.text = strings.TrimPrefix(tk.text, "\n")
			}
			p.text(tk)
		case tokStartTag:
			p.bindNamespaces(tk.attrs)
			tk.name = p.resolveName(tk.name)
			p.startTag(tk)
		case tokEndTag:
			tk.name = p.resolveName(tk.name)
			p.endTag(tk)
		}
		if p.truncated {
			p.finish()
			return
		}
	}
}

// Namespaces this engine knows a name in, and what a name in one of them is
// called here.
//
// The list is short because the parser's own vocabulary is: an element is HTML,
// or it is one of the two foreign roots. A prefix bound to anything else is left
// on the name, so the element is reported as one this engine does not lay out —
// which is true, and is better than guessing that a prefix nobody declared for a
// language this engine reads meant one of these.
var knownNamespaces = map[string]bool{
	"http://www.w3.org/1999/xhtml":       true,
	"http://www.w3.org/2000/svg":         true,
	"http://www.w3.org/1998/math/mathml": true,
}

// bindNamespaces records the "xmlns:p" declarations on a start tag.
//
// The bindings are not scoped to the element that made them, which every XML
// parser does and this does not. It is a simplification with a stated limit
// rather than an oversight: a document declares its prefixes on its root — that
// is what the suite's XHTML does, and what a document generated by anything
// does — and honouring a *rebinding* deeper down would need a stack that the
// only documents needing it would be constructed to exercise. A prefix rebound
// halfway through a document is resolved here by its later binding throughout,
// and the worst that costs is an element treated as foreign that was not.
func (p *parser) bindNamespaces(attrs []Attribute) {
	for _, a := range attrs {
		if len(a.Name) > 6 && strings.HasPrefix(a.Name, "xmlns:") {
			if p.ns == nil {
				p.ns = map[string]string{}
			}
			p.ns[a.Name[len("xmlns:"):]] = strings.ToLower(a.Value)
		}
	}
}

// resolveName drops a namespace prefix this engine can make sense of.
//
// "<svg:svg>" in a document that bound "svg" to the SVG namespace is the same
// element as "<svg>", and the whole of what the prefix says is which language
// the name belongs to. A prefix bound to something else, or bound to nothing at
// all, is kept: the name is then not one of this engine's, which is exactly what
// an unresolvable prefix means.
func (p *parser) resolveName(name string) string {
	i := strings.IndexByte(name, ':')
	if i < 0 {
		return name
	}
	if knownNamespaces[p.ns[name[:i]]] {
		return name[i+1:]
	}
	return name
}

func (p *parser) element(name string, offset int) *Node {
	p.nodes++
	return &Node{Type: ElementNode, Name: name, Offset: offset}
}

// current is the element things are being put into.
func (p *parser) current() *Node {
	if len(p.open) == 0 {
		return p.body
	}
	return p.open[len(p.open)-1]
}

func (p *parser) text(tk token) {
	if tk.text == "" {
		return
	}
	parent := p.current()

	// Text only decides where the body begins when it is *loose* — directly
	// inside the head or the html element rather than inside something open.
	// Inside an element it simply belongs to that element, and this distinction
	// is the whole of it: a <title> or a <style> lives in the head and has text
	// in it, and treating that text as the start of the body would close the
	// element out from under its own end tag.
	if parent == p.head || parent == p.html {
		// Whitespace between elements belongs to neither head nor body;
		// dropping it keeps "</head>\n<p>" from putting a stray text node at
		// the front of the document.
		if strings.TrimSpace(tk.text) == "" {
			return
		}
		p.enterBody()
		parent = p.current()
	}
	if !p.room(tk.offset) {
		return
	}
	// Adjacent runs are merged, so no element ever has two text children in a
	// row — a shape every consumer would otherwise have to handle.
	if n := len(parent.Children); n > 0 && parent.Children[n-1].Type == TextNode {
		parent.Children[n-1].Text += tk.text
		return
	}
	p.nodes++
	parent.appendChild(&Node{Type: TextNode, Text: tk.text, Offset: tk.offset})
}

// room reports whether another node may be added, recording the trip if not.
func (p *parser) room(off int) bool {
	if p.nodes >= maxNodes {
		if !p.truncated {
			p.tok.fail(off, "the document has more elements than this engine will build ("+
				strconv.Itoa(maxNodes)+"); the rest was not read")
			p.truncated = true
		}
		return false
	}
	return true
}

func (p *parser) startTag(tk token) {
	name := tk.name

	// The three frame elements always exist already, because run() built them
	// before reading anything. A start tag for one of them is therefore not an
	// element to create — it is the author naming a box that is already there,
	// and the only thing it carries that the frame does not is its attributes.
	//
	// Creating a second one instead is a fault that hides well: <html> and
	// <body> would nest inside the frame, every document with explicit tags
	// would apply body's margin twice, and a test asking only whether an <html>
	// element *exists* would pass — which is exactly how this survived until a
	// layout comparison noticed the doubled margin.
	switch name {
	case "html":
		mergeAttributes(p.html, tk.attrs)
		return
	case "head":
		mergeAttributes(p.head, tk.attrs)
		return
	case "body":
		mergeAttributes(p.body, tk.attrs)
		p.enterBody()
		return
	}

	if why, dropped := droppedElements[name]; dropped {
		p.tok.unsupported(tk.offset, "<"+name+"> is dropped: "+why)
		// Its content goes with it. For the raw-text ones that means consuming
		// to the end tag, or the script body would be read as markup.
		if rawTextElements[name] && !tk.selfClosing {
			p.skipRaw(name, tk.offset)
		} else if !voidElements[name] && !tk.selfClosing {
			p.skipElement(name)
		}
		return
	}

	if foreignElements[name] {
		// A foreign element is a replaced element: it has a box, and its content
		// is not HTML. The element stays, its source is kept for whoever can
		// read it, and the subtree is not parsed on — which is what used to
		// splice an SVG's text into the paragraph around it.
		el := p.insert(tk)
		if el != nil && !tk.selfClosing {
			start := p.tok.pos
			end := p.skipElement(name)
			if end > start && end <= len(p.tok.src) {
				el.Foreign = p.tok.src[start:end]
			}
		}
		return
	}

	if !knownElements[name] {
		// An element nobody has heard of is an element all the same. HTML gives
		// it no special behaviour and no user agent style, which leaves it an
		// ordinary inline box that inherits everything and that a stylesheet may
		// select — "<my-widget>" is laid out by a browser exactly as a <span>
		// with the same rules on it would be.
		//
		// It used to be dropped and reported, on the reading that an element
		// this engine does not know is one it cannot lay out. That is true of
		// <canvas> and <video>, which need something this engine does not have,
		// and they are refused above by name. It was never true of a custom
		// element: the box is not a special one, and dropping it lost every rule
		// an author had written for it. CSS2/linebox/line-breaking-font-size-
		// zero-001 styles <inline-block> and <sep> and is the shape a modern
		// document is full of.
		p.insertUnknown(tk)
		return
	}

	if tk.selfClosing && !voidElements[name] {
		// "<div/>" is not an empty div. HTML has no self-closing syntax for
		// ordinary elements, so a browser reads this as an open <div> and every
		// following element ends up inside it — a shape that looks like a
		// typo's worth of markup and moves half the page.
		//
		// It is reported and then parsed that way, which is what the tree
		// construction stage says to do: the flag on a non-void element is a
		// parse error and is left unacknowledged, so the element is opened as
		// though the slash were not there. The document is still refused — see
		// TestSelfClosingNonVoidIsRefused — and this is about what a caller that
		// renders it anyway is given. Dropping the element was a worse answer
		// than either reading of the markup: XHTML says it is an empty div, HTML
		// says it is an open one, and *neither* says it is nothing at all.
		p.tok.fail(tk.offset, "<"+name+"/> is not an empty element; HTML has no "+
			"self-closing syntax outside void elements, so it is read as <"+name+">")
	}

	// The optional end tags of HTML: an incoming start tag can close what is
	// open.
	p.closeImplied(name)

	if headElements[name] && !p.bodyStarted {
		p.appendTo(p.head, tk)
		return
	}
	if !metadataElements[name] && !p.bodyStarted {
		p.enterBody()
	}

	el := p.insert(tk)
	if el == nil {
		return
	}
	if voidElements[name] {
		return
	}
	if contentSkippedElements[name] && !tk.selfClosing {
		// The element stays and its content does not. An iframe's children are
		// what a browser without frame support would show instead, so a browser
		// with them renders none of it — and the content is not markup either
		// way, which is why this consumes to the end tag rather than parsing on.
		p.skipRaw(name, tk.offset)
		return
	}
	if rawTextElements[name] {
		p.tok.raw, p.tok.rcdata = name, false
	} else if rcdataElements[name] {
		p.tok.raw, p.tok.rcdata = name, true
	}
	p.stripNewline = !p.tok.xml && dropFirstNewline[name]
	p.open = append(p.open, el)

	if len(p.open) > maxDepth {
		p.tok.fail(tk.offset, "elements are nested more deeply than this engine will read ("+
			strconv.Itoa(maxDepth)+")")
		p.truncated = true
	}
}

// insertUnknown opens an element HTML gives no behaviour to.
//
// It is the ordinary path with everything that is keyed on a name left out:
// there is no optional end tag to close, no head to belong to, no raw text, no
// void form and no newline to strip, because every one of those is a rule about
// a *particular* element and this is not one of them. What is left is an element
// that opens, holds its children and closes — which is the whole of what HTML
// says about a name nobody has defined.
//
// A self-closing "<my-widget/>" is the one thing to say about it, and HTML says
// it too: outside the void elements the slash is a parse error and the element
// is opened anyway. The document is refused either way; this is about what a
// caller that renders it regardless is handed.
func (p *parser) insertUnknown(tk token) {
	if !p.bodyStarted {
		p.enterBody()
	}
	if tk.selfClosing && p.tok.xml {
		// XML *does* have self-closing syntax, and it means an empty element.
		// The element is inserted and never opened, which is the whole of what
		// "<my-widget/>" says there.
		p.insert(tk)
		return
	}
	if tk.selfClosing {
		p.tok.fail(tk.offset, "<"+tk.name+"/> is not an empty element; HTML has no "+
			"self-closing syntax outside void elements, so it is read as <"+tk.name+">")
	}
	el := p.insert(tk)
	if el == nil {
		return
	}
	p.open = append(p.open, el)
	if len(p.open) > maxDepth {
		p.tok.fail(tk.offset, "elements are nested more deeply than this engine will read ("+
			strconv.Itoa(maxDepth)+")")
		p.truncated = true
	}
}

// mergeAttributes copies the attributes a frame element's start tag carried onto
// the element the parser had already made.
//
// An attribute already present wins over the one arriving, which is the rule
// HTML gives: the first value of a repeated attribute is the one that counts, and
// the frame's own is the first by construction.
func mergeAttributes(el *Node, attrs []Attribute) {
	if el == nil {
		return
	}
	for _, a := range attrs {
		if el.HasAttr(a.Name) {
			continue
		}
		el.Attrs = append(el.Attrs, a)
	}
}

// appendTo puts an element in a named parent rather than the current one, which
// is what the head elements need.
func (p *parser) appendTo(parent *Node, tk token) {
	if !p.room(tk.offset) {
		return
	}
	el := p.element(tk.name, tk.offset)
	el.Attrs = tk.attrs
	parent.appendChild(el)
	if rawTextElements[tk.name] {
		p.tok.raw, p.tok.rcdata = tk.name, false
	} else if rcdataElements[tk.name] {
		p.tok.raw, p.tok.rcdata = tk.name, true
	}
	if !voidElements[tk.name] {
		p.open = append(p.open, el)
	}
}

func (p *parser) insert(tk token) *Node {
	if !p.room(tk.offset) {
		return nil
	}
	el := p.element(tk.name, tk.offset)
	el.Attrs = tk.attrs
	p.current().appendChild(el)
	return el
}

// enterBody moves from the head to the body.
func (p *parser) enterBody() {
	if p.bodyStarted {
		return
	}
	p.bodyStarted = true
	// Everything the head had open is closed by the move.
	for len(p.open) > 0 && p.open[len(p.open)-1] != p.html {
		p.open = p.open[:len(p.open)-1]
	}
	p.open = append(p.open, p.body)
}

// closeImplied pops the elements that an incoming start tag ends.
func (p *parser) closeImplied(incoming string) {
	for len(p.open) > 0 {
		top := p.open[len(p.open)-1]
		closers, ok := closedByStartTag[top.Name]
		if !ok || !closers[incoming] {
			return
		}
		p.open = p.open[:len(p.open)-1]
	}
}

func (p *parser) endTag(tk token) {
	name := tk.name

	if name == "br" && !p.tok.xml {
		// "</br>" is a line break. HTML's tree construction says so in as many
		// words — "act as if this was a br start tag token with no attributes,
		// rather than the end tag token that it actually is" — and it says it
		// because authors write "<br></br>" and "<br/></br>" and have done
		// since XHTML was a thing people aimed at.
		//
		// It is not the void-element report below, which is the right answer
		// for every other void element and the wrong one for this: reporting
		// "</br> is an end tag for a void element" and stopping loses a break
		// the document asked for, and a lost break is a line of text joined to
		// the next one. Three of the working group's own fixtures write it,
		// and the two that show it draw their reference with "<br>X</br>Y" and
		// expect three lines.
		//
		// Only outside XML, where "<br></br>" is one element properly closed
		// and the end tag really is an end tag.
		//
		// It is still reported, because what it does is a surprise worth
		// telling an author about: "<br></br>" is two breaks and not one, and
		// nothing about the page says which of the two blank lines the author
		// asked for.
		p.tok.fail(tk.offset,
			"</br> is a line break, not an end tag: HTML reads it as another <br>")
		// The rule also says to drop the token's attributes, and there are
		// none to drop: this tokenizer refuses an end tag with attributes
		// where it reads them, so "</br class=x>" never arrives here as a
		// break carrying a class. Assigning nil to them was written first and
		// a planted defect showed it changed nothing.
		p.startTag(tk)
		return
	}

	if _, dropped := droppedElements[name]; dropped {
		// Its start tag was reported and its content skipped, so a matching end
		// tag here is the tail of something already dealt with.
		return
	}
	if voidElements[name] {
		// XML has no void elements: every element is closed, by an end tag or by
		// an empty-element tag, so "<col></col>" is how XHTML writes what HTML
		// writes as "<col>". The end tag closes an element that has already
		// ended, which is to say it does nothing — and in an XHTML document it
		// is not a mistake either.
		if !p.tok.xml {
			p.tok.fail(tk.offset,
				"</"+name+"> is an end tag for a void element, which has none")
		}
		return
	}
	// An element HTML gives no behaviour to is closed like any other. It used to
	// be ignored here, which was right while its start tag was dropped as well
	// and is wrong now that the element is opened: an end tag nobody acts on
	// leaves the element open, and the next block's end tag reports the
	// mis-nesting it caused.

	// Find it on the stack. Anything above it may only be there if HTML lets it
	// end without a tag of its own.
	at := -1
	for i := len(p.open) - 1; i >= 0; i-- {
		if p.open[i].Name == name {
			at = i
			break
		}
	}
	if at < 0 {
		p.tok.fail(tk.offset, "</"+name+"> closes nothing: no <"+name+"> is open here")
		return
	}
	for i := len(p.open) - 1; i > at; i-- {
		if !closedByParentEnd[p.open[i].Name] {
			p.tok.fail(tk.offset, "</"+name+"> would close <"+p.open[i].Name+
				">, which is still open; tags have to nest")
			break
		}
	}
	p.open = p.open[:at]
}

// skipElement consumes to the matching end tag of an element being dropped,
// counting nesting so an inner <iframe> does not end the outer one.
// It returns the offset at which the matching end tag begins, which is the end
// of the element's content — or the end of the source when there is no end tag,
// since an unclosed element runs to the document.
func (p *parser) skipElement(name string) int {
	depth := 1
	for {
		tk := p.tok.next()
		switch tk.kind {
		case tokEOF:
			return len(p.tok.src)
		case tokStartTag:
			if p.resolveName(tk.name) == name && !tk.selfClosing && !voidElements[name] {
				depth++
			}
		case tokEndTag:
			if p.resolveName(tk.name) == name {
				if depth--; depth == 0 {
					return tk.offset
				}
			}
		}
	}
}

// skipRaw consumes the body of a dropped raw-text element, whose content is not
// markup and must not be tokenized as any.
func (p *parser) skipRaw(name string, off int) {
	end := p.tok.findEndTag(name, p.tok.pos)
	if end < 0 {
		p.tok.pos = len(p.tok.src)
		return
	}
	p.tok.pos = end
	// Consume the end tag itself.
	if tk := p.tok.next(); tk.kind != tokEndTag {
		p.tok.fail(off, "<"+name+"> is never closed")
	}
}

// finish reports the elements still open at the end of the document.
func (p *parser) finish() {
	for i := len(p.open) - 1; i >= 0; i-- {
		el := p.open[i]
		if el == p.html || el == p.head || el == p.body {
			continue
		}
		if closedByParentEnd[el.Name] {
			continue
		}
		p.tok.fail(el.Offset, "<"+el.Name+"> is never closed")
	}
	p.open = nil
}
