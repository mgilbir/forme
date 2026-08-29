package html

import (
	"strings"
	"testing"
)

// A stylesheet inside an XHTML document, where "&gt;" is a ">".
//
// HTML and XHTML disagree about exactly one thing that matters here. HTML makes
// the content of <style> and <script> *raw text*: neither "&" nor "<" means
// anything, so "&gt;" is four characters. XML makes it ordinary character data,
// where "&gt;" is a character reference for ">".
//
// The consequence is not a stray ampersand somewhere. An author writing a child
// combinator in an XHTML stylesheet has to spell it "&gt;", because a bare ">"
// would be markup — so "body &gt; div" is how every such document writes it, and
// read as HTML that selector matches nothing at all. Every rule in the block
// goes quietly inert and the page comes back unstyled, which looks like a layout
// bug and is a parsing one.
//
// Seven of the suite's reftests are that document, and four more were passing
// only because a stylesheet nobody could read asked for nothing.

// styleOf returns the text of the document's first <style>.
func styleOf(t *testing.T, src string) string {
	t.Helper()
	doc, _, _ := Parse(src)
	var found string
	var seen bool
	var walk func(*Node)
	walk = func(n *Node) {
		if !seen && n.Type == ElementNode && n.Name == "style" {
			seen = true
			for _, c := range n.Children {
				found += c.Text
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	if !seen {
		t.Fatalf("no <style> in %q", src)
	}
	return found
}

const xhtmlDoctype = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" ` +
	`"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`

// TestAnXHTMLStylesheetResolvesItsReferences.
func TestAnXHTMLStylesheetResolvesItsReferences(t *testing.T) {
	for _, tc := range []struct{ what, prologue string }{
		{"an XHTML doctype", xhtmlDoctype + `<html xmlns="x">`},
		{"an XML declaration", `<?xml version="1.0" encoding="utf-8"?><html>`},
		{"the XHTML namespace on the root", `<html xmlns="http://www.w3.org/1999/xhtml">`},
		{"the namespace in single quotes", `<html xmlns='http://www.w3.org/1999/xhtml'>`},
	} {
		got := styleOf(t, tc.prologue+`<head><style>body &gt; div { color: red }</style></head>`)
		if !strings.Contains(got, "body > div") {
			t.Errorf("%s: the stylesheet came out %q; in XML \"&gt;\" is a \">\"",
				tc.what, got)
		}
	}
}

// TestAnHTMLStylesheetIsRawText is the containment case, and it is the one that
// matters most: HTML is what an unmarked document overwhelmingly is, and a
// stylesheet there really does hold "&gt;" as four characters.
//
// The consequence of getting it the other way round is worse than the bug being
// fixed. A CSS content string of "&amp;" would become an ampersand, so a page
// generating "&" through content: "\&amp;" — which is a real thing to write when
// the same stylesheet is served to both parsers — would lose it.
func TestAnHTMLStylesheetIsRawText(t *testing.T) {
	for _, tc := range []struct{ what, prologue string }{
		{"no prologue at all", ``},
		{"an HTML5 doctype", `<!DOCTYPE html>`},
		{"an HTML 4 doctype", `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN">`},
		// The namespace on something that is not the root, far enough in that
		// the prologue scan should never have reached it, and on an attribute
		// that is not xmlns.
		{"an unrelated attribute", `<!DOCTYPE html><html data-ns="http://www.w3.org/1999/xhtml">`},
	} {
		got := styleOf(t, tc.prologue+`<head><style>body &gt; div { color: red }</style></head>`)
		if strings.Contains(got, "body > div") {
			t.Errorf("%s: the stylesheet came out %q; in HTML a <style> is raw text "+
				"and \"&gt;\" is four characters", tc.what, got)
		}
		if !strings.Contains(got, "&gt;") {
			t.Errorf("%s: the stylesheet came out %q and should be untouched",
				tc.what, got)
		}
	}
}

// TestACDATASectionIsLiteral. It is the XML syntax for "the characters between
// these markers mean themselves", and it is exactly what an author wraps a
// stylesheet in to keep "&" and "<" working — so resolving references inside one
// would defeat the thing it is written for.
//
// The markers themselves go, which they must: they are not CSS, and leaving them
// in makes the rule they open part of a declaration the parser has to throw away.
func TestACDATASectionIsLiteral(t *testing.T) {
	got := styleOf(t, xhtmlDoctype+`<html xmlns="http://www.w3.org/1999/xhtml"><head>`+
		`<style type="text/css"><![CDATA[
		 div:before { content: "&amp;" }
		]]></style></head>`)
	if !strings.Contains(got, `"&amp;"`) {
		t.Errorf("the stylesheet came out %q; inside CDATA the characters are "+
			"literal and \"&amp;\" is five of them", got)
	}
	for _, marker := range []string{"<![CDATA[", "]]>"} {
		if strings.Contains(got, marker) {
			t.Errorf("the stylesheet came out %q and still holds %q, which is not CSS",
				got, marker)
		}
	}
}

// TestReferencesOutsideACDATASectionStillResolve: a document may have both, and
// the section bounds the exception rather than the element.
func TestReferencesOutsideACDATASectionStillResolve(t *testing.T) {
	got := styleOf(t, `<?xml version="1.0"?><html><head><style>`+
		`a &gt; b { color: red }<![CDATA[ c &gt; d { color: lime } ]]>e &gt; f { color: blue }`+
		`</style></head>`)
	for _, want := range []string{"a > b", "c &gt; d", "e > f"} {
		if !strings.Contains(got, want) {
			t.Errorf("the stylesheet came out %q and is missing %q", got, want)
		}
	}
}

// TestAnUnterminatedCDATASectionIsLiteralToTheEnd. The markers asked for literal
// characters and only the closing one is missing; taking the rest as literal
// loses nothing, where resolving it would change characters the author fenced
// off on purpose.
func TestAnUnterminatedCDATASectionIsLiteralToTheEnd(t *testing.T) {
	got := styleOf(t, `<?xml version="1.0"?><html><head><style>`+
		`<![CDATA[ a &gt; b { color: red }</style></head>`)
	if !strings.Contains(got, "a &gt; b") {
		t.Errorf("the stylesheet came out %q; the section never closed, so what "+
			"follows it is literal", got)
	}
}

// TestTheRuleIsAboutRawTextRatherThanAboutStyle.
//
// <script> is the other raw-text element and takes the same path. Nothing
// downstream can see it — a script's content is dropped whole, because scripts
// are never run here — so <style> is the only element whose resolved text
// reaches a document, and this is the only place the other half can be
// observed at all. It is asserted rather than assumed, because "the same path"
// is a claim about the code and not about the specification.
func TestTheRuleIsAboutRawTextRatherThanAboutStyle(t *testing.T) {
	const src = "a &amp;&amp; b"
	xml := &tokenizer{xml: true}
	if got := xml.rawValue(src, 0); got != "a && b" {
		t.Errorf("XHTML raw text came out %q, want %q", got, "a && b")
	}
	plain := &tokenizer{}
	if got := plain.rawValue(src, 0); got != src {
		t.Errorf("HTML raw text came out %q and should be untouched", got)
	}
	// And the element really is one of the two: if <script> stopped being raw
	// text this would be testing nothing.
	for _, name := range []string{"style", "script"} {
		if !rawTextElements[name] {
			t.Errorf("<%s> is no longer raw text, so it no longer takes this path", name)
		}
	}
	// A script's content is dropped, which is why the DOM cannot show the
	// difference and why this test is written against the tokenizer.
	doc, _, _ := Parse(`<?xml version="1.0"?><html><head>` +
		`<script>a &amp;&amp; b</script></head>`)
	var found bool
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode && n.Name == "script" {
			found = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	if found {
		t.Errorf("a <script> reached the tree; if it now does, this test should " +
			"assert its text instead of the tokenizer's")
	}
}

// TestTheXMLSignalsAreLookedForOnlyInThePrologue.
//
// A document's body may say anything, including the XHTML namespace string in
// its own text, and what a document *is* cannot depend on what it says halfway
// down. The bound also keeps the check off the length of the input.
func TestTheXMLSignalsAreLookedForOnlyInThePrologue(t *testing.T) {
	filler := strings.Repeat("<p>x</p>", 400)
	src := `<!DOCTYPE html><body>` + filler +
		`<p>xmlns="http://www.w3.org/1999/xhtml"</p>` +
		`<style>body &gt; div { color: red }</style></body>`
	if got := styleOf(t, src); strings.Contains(got, "body > div") {
		t.Errorf("the stylesheet came out %q; the namespace appeared in the "+
			"document's own text, which says nothing about how it is parsed", got)
	}
}

// XML syntax that HTML has no equivalent of, in a document that says it is
// XHTML.
//
// None of what follows changes a rendering. What it changes is what this engine
// says about a document, which is a first-class output here: the parser refuses
// and reports rather than quietly reinterpreting, and a report an author cannot
// act on is worse than no report at all. Told that its own XML declaration is
// malformed markup, an author has nothing to do about it and no reason to trust
// the next finding either.
//
// Three constructs, a hundred and twenty-one false findings across the Web
// Platform Tests, and the same sentence behind all of them: valid XML is not
// malformed HTML.

// findingsOf returns the messages a document produced.
func findingsOf(src string) []string {
	_, errs, _ := Parse(src)
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Message)
	}
	return out
}

// TestAProcessingInstructionIsNotAFaultInXML. It is the first thing in half the
// suite's XHTML, and HTML has none — so every one of those documents was told
// its own declaration was a mistake.
func TestAProcessingInstructionIsNotAFaultInXML(t *testing.T) {
	if got := findingsOf(`<?xml version="1.0" encoding="utf-8"?><html><body>x</body></html>`); len(got) != 0 {
		t.Errorf("an XML declaration reported %v", got)
	}
	// A processing instruction anywhere else in an XML document is legal too,
	// and nothing is produced for one either way.
	if got := findingsOf(xhtmlDoctype + `<html xmlns="http://www.w3.org/1999/xhtml">` +
		`<body><?php echo 1 ?>x</body></html>`); len(got) != 0 {
		t.Errorf("a processing instruction in an XHTML body reported %v", got)
	}
	// And in HTML it is still a fault, because HTML still has none.
	got := findingsOf(`<!DOCTYPE html><body><?xml version="1.0"?>x</body>`)
	if len(got) != 1 || !strings.Contains(got[0], "processing instruction") {
		t.Errorf("an HTML document with a processing instruction reported %v, want "+
			"exactly the one finding", got)
	}
}

// TestAnEndTagForAVoidElementIsNotAFaultInXML. XML has no void elements: every
// element is closed, by an end tag or by an empty-element tag, so "<col></col>"
// is how XHTML writes what HTML writes as "<col>".
func TestAnEndTagForAVoidElementIsNotAFaultInXML(t *testing.T) {
	const markup = `<body><table><colgroup><col></col></colgroup></table><br></br></body>`
	if got := findingsOf(`<?xml version="1.0"?><html>` + markup + `</html>`); len(got) != 0 {
		t.Errorf("XHTML end tags for void elements reported %v", got)
	}
	// In HTML both are reported and they are not the same report. "</col>" is
	// an end tag for something that has none; "</br>" is a line break, which is
	// the one meaning HTML gives to a void element's end tag, and saying "void
	// element" about it would describe neither what it is nor what it did.
	got := findingsOf(`<!DOCTYPE html>` + markup)
	if len(got) != 2 {
		t.Fatalf("HTML end tags for void elements reported %v, want one each", got)
	}
	if !strings.Contains(got[0], "void element") || !strings.Contains(got[0], "col") {
		t.Errorf("</col> reported %q", got[0])
	}
	if !strings.Contains(got[1], "line break") {
		t.Errorf("</br> reported %q, want the finding that says it is a break", got[1])
	}
}

// TestANamespacePrefixIsPartOfTheNameInXML is the one that was breaking a parse
// rather than only reporting one.
//
// A name stopping at the colon read the end tag "</svg:svg>" as "</svg", which
// is not closed with a ">" — so the document was reported malformed, the
// recovery skipped to the next ">", and what the element contained was anyone's
// guess. Forty-seven of the suite's documents write their inline SVG that way.
func TestANamespacePrefixIsPartOfTheNameInXML(t *testing.T) {
	const svgNS = `xmlns:svg="http://www.w3.org/2000/svg"`
	src := `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml" ` + svgNS +
		`><body><svg:svg width="40" height="20">` +
		`<svg:rect width="40" height="20" fill="blue"/></svg:svg><p>after</p></body></html>`
	if got := findingsOf(src); len(got) != 0 {
		t.Errorf("a namespaced inline SVG reported %v", got)
	}

	doc, _, _ := Parse(src)
	var svg *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode && n.Name == "svg" {
			svg = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	if svg == nil {
		t.Fatal(`no <svg> element; "svg:svg" in the SVG namespace is the element "svg"`)
	}
	if !strings.Contains(svg.Foreign, "svg:rect") {
		t.Errorf("the subtree source is %q; the end tag has to match the start tag "+
			"or the skip stops in the wrong place", svg.Foreign)
	}
	if !strings.Contains(textOf(doc), "after") {
		t.Errorf("the document after the picture was swallowed: %q", textOf(doc))
	}
}

// TestAPrefixThisEngineCannotResolveIsKept is the containment case, and the
// reason the prefix is resolved rather than simply dropped.
//
// A prefix says which language a name belongs to. Dropping it unread would make
// "<x:svg>" an SVG whatever x was bound to — including nothing at all. Keeping
// it makes the name one this engine has never heard of, which is an ordinary
// element with an unusual name: laid out as an inline box, styled by whatever
// selects it, and emphatically not a picture.
func TestAPrefixThisEngineCannotResolveIsKept(t *testing.T) {
	for _, tc := range []struct{ what, root string }{
		{"a prefix bound to something else",
			`<html xmlns:s="http://example.com/not-svg">`},
		{"a prefix bound to nothing at all", `<html>`},
	} {
		src := `<?xml version="1.0"?>` + tc.root + `<body><s:svg/></body></html>`
		if got := findingsOf(src); len(got) != 0 {
			t.Errorf("%s: reported %v; an element with an unresolvable prefix is "+
				"an element with a long name", tc.what, got)
		}
		doc, _, _ := Parse(src)
		var found *Node
		doc.Walk(func(n *Node) bool {
			if n.Type == ElementNode && n.Name == "s:svg" {
				found = n
			}
			return true
		})
		if found == nil {
			t.Errorf("%s: no <s:svg> in the tree", tc.what)
			continue
		}
		// The whole point: it is not an SVG, so its content is not kept as
		// foreign source and nothing will try to draw it.
		if found.Foreign != "" {
			t.Errorf("%s: the element was read as foreign content (%q)",
				tc.what, found.Foreign)
		}
	}
	// The XHTML namespace resolves too, so "<html:div>" is a div.
	if got := findingsOf(`<?xml version="1.0"?>` +
		`<html xmlns:h="http://www.w3.org/1999/xhtml"><body><h:div>x</h:div></body></html>`); len(got) != 0 {
		t.Errorf("a prefixed XHTML element reported %v", got)
	}
}

// TestANameWithAColonIsNotAPrefixInHTML: the colon is part of a name in XML and
// is not in HTML, and this is the whole of the difference the tokenizer makes.
func TestANameWithAColonIsNotAPrefixInHTML(t *testing.T) {
	doc, _, _ := Parse(`<!DOCTYPE html><body><a:b>x</a:b></body>`)
	var names []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode {
			names = append(names, n.Name)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	for _, n := range names {
		if n == "a:b" {
			t.Errorf("an HTML document produced the element %q; a colon is not part "+
				"of a name there, and nothing binds a prefix in HTML", n)
		}
	}
}

// TestTheDocumentRecordsWhichLanguageItWasReadAs is the flag itself, and the
// only thing outside this package that reads it.
//
// The tokenizer decides XHTML from the prologue and used it for one thing here:
// a <style> element's content. An attribute name is the second, and it belongs
// to the caller rather than to the parse — XML makes the name case-sensitive and
// HTML lowercases it, so "attr(Title)" in a content property selects the title
// attribute of an HTML element and selects nothing at all in XHTML. See
// AttrExact, and layout's TestAttrMatchesTheCaseTheDocumentLanguageDoes.
func TestTheDocumentRecordsWhichLanguageItWasReadAs(t *testing.T) {
	for _, tc := range []struct {
		src string
		xml bool
	}{
		{`<p title="yes">x</p>`, false},
		{`<html xmlns="http://www.w3.org/1999/xhtml"><p title="yes">x</p></html>`, true},
		{`<?xml version="1.0"?><html><p title="yes">x</p></html>`, true},
	} {
		doc, _, _ := Parse(tc.src)
		if doc.XML != tc.xml {
			t.Errorf("%q read as XML = %v, want %v", tc.src, doc.XML, tc.xml)
		}
		// And every element in it answers the same, which is what the walk is
		// for: the flag is on the document node and nothing else.
		p := findElement(doc, "p")
		if p == nil {
			t.Fatalf("%q has no <p>", tc.src)
		}
		if p.XML {
			t.Errorf("%q: the flag was set on an element as well as the document",
				tc.src)
		}
		if got := p.XMLDocument(); got != tc.xml {
			t.Errorf("%q: the <p> says XMLDocument = %v, want %v", tc.src, got, tc.xml)
		}
	}
}

// TestAttrExactRefusesAQueryTheParseWouldHaveLowered is the lookup, held to what
// it claims: the names in Attrs are lowercase whichever language the document is
// in, because the tokenizer lowercases them, so what AttrExact does in practice
// is refuse a query that is not already lowercase.
//
// That is the answer that never invents a match. An XHTML document that really
// wrote "Title" has an attribute this engine has stored as "title" and cannot
// tell from one written that way, and refusing both is the only reading that is
// never wrong about which of the two it found.
func TestAttrExactRefusesAQueryTheParseWouldHaveLowered(t *testing.T) {
	doc, _, _ := Parse(`<html xmlns="http://www.w3.org/1999/xhtml">` +
		`<p Title="yes">x</p></html>`)
	p := findElement(doc, "p")
	if p == nil {
		t.Fatal("no <p>")
	}
	if got, ok := p.AttrExact("title"); !ok || got != "yes" {
		t.Errorf("AttrExact(\"title\") gave %q, %v; the parse stores the name "+
			"lowercased whichever language it read", got, ok)
	}
	if _, ok := p.AttrExact("Title"); ok {
		t.Error("AttrExact(\"Title\") found something; nothing in Attrs is spelled that way")
	}
	// Attr is the other reading and is unchanged.
	if got, ok := p.Attr("Title"); !ok || got != "yes" {
		t.Errorf("Attr(\"Title\") gave %q, %v, want the attribute", got, ok)
	}
}

// findElement is the first element of a name anywhere in a tree.
func findElement(n *Node, name string) *Node {
	if n == nil {
		return nil
	}
	if n.Type == ElementNode && n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if got := findElement(c, name); got != nil {
			return got
		}
	}
	return nil
}
