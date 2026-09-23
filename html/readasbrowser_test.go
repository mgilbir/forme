package html

import (
	"strings"
	"testing"
)

// Places where the tokenizer and the tree builder read valid markup other than
// as a browser does, or read malformed markup and said the wrong thing about it.

// TestListingDropsItsFirstNewline. <listing> is <pre> under the name it had
// before <pre>, and the standard's rule about the newline after the start tag
// names both.
func TestListingDropsItsFirstNewline(t *testing.T) {
	doc := mustParseHTML(t, "<listing>\ncode\n</listing>")
	if got := findElement(doc, "listing").TextContent(); got != "code\n" {
		t.Errorf("the listing holds %q, want \"code\\n\"", got)
	}
	// One newline, the first, and only there.
	doc = mustParseHTML(t, "<listing>\n\ncode</listing>")
	if got := findElement(doc, "listing").TextContent(); got != "\ncode" {
		t.Errorf("the listing holds %q, want \"\\ncode\"", got)
	}
}

// TestPlaintextHasNoEndTag. The tokenizer's PLAINTEXT state is never left, so
// "</plaintext>" is text and the element runs to the end of the document. It
// was read as an end tag, which put what followed back into the page as markup.
//
// The document is still refused, and once: the standard makes an element open
// at the end of the file a parse error, and <plaintext> is always open there —
// it is obsolete, and this is why.
func TestPlaintextHasNoEndTag(t *testing.T) {
	doc, errs, _ := Parse(`<p>a<plaintext><b>x</b></plaintext>y`)
	if got := bodyShape(t, doc); got != `<p>('a')<plaintext>('<b>x</b></plaintext>y')` {
		t.Errorf("the body is %s", got)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "<plaintext> is never closed") {
		t.Errorf("the findings are %v, want the one", errs)
	}
}

// TestAnUnquotedValueKeepsItsQuote is the audit's document: the quote is part of
// the value, as HTML reads it, and the tag ends at its own ">". The attribute
// reader used to refuse the quote by skipping to the next ">" — the tag's own —
// and the attribute loop then went on reading attributes out of the text after
// the tag, so "hello" became an attribute of the link and left the page.
func TestAnUnquotedValueKeepsItsQuote(t *testing.T) {
	doc, errs, ok := Parse(`<p><a href=x"y>hello</a> world</p>`)
	if ok {
		t.Error("a quote in an unquoted value is a parse error, and was accepted")
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "part of the value") {
		t.Errorf("the findings are %v, want the one about the quote", errs)
	}
	if got := shapeOfTree(findElement(doc, "p")); got != `<p>(<a>('hello')'world')` {
		t.Errorf("the paragraph is %s", got)
	}
	a := findElement(doc, "a")
	if v, _ := a.Attr("href"); v != `x"y` || len(a.Attrs) != 1 {
		t.Errorf("the link's attributes are %v, want href=x\"y alone", a.Attrs)
	}

	// Every character the standard names, and a name that holds a quote.
	for _, tc := range []struct{ src, name, value string }{
		{`<p a=x'y>t</p>`, "a", `x'y`},
		{`<p a=x<y>t</p>`, "a", `x<y`},
		{`<p a=x=y>t</p>`, "a", `x=y`},
		{"<p a=x`y>t</p>", "a", "x`y"},
		{`<p a"b=1>t</p>`, `a"b`, "1"},
		{`<p =a>t</p>`, "=a", ""},
	} {
		doc, errs, ok := Parse(tc.src)
		if ok || len(errs) != 1 {
			t.Errorf("%q: ok=%v, findings %v; want one", tc.src, ok, errs)
		}
		p := findElement(doc, "p")
		if v, has := p.Attr(tc.name); !has || v != tc.value || len(p.Attrs) != 1 {
			t.Errorf("%q: the attributes are %v, want %s=%q", tc.src, p.Attrs, tc.name, tc.value)
		}
		if got := p.TextContent(); got != "t" {
			t.Errorf("%q: the text is %q", tc.src, got)
		}
	}

	// An "=" with no value after it: the standard's missing-attribute-value,
	// an attribute whose value is the empty string, and the tag ends at its ">".
	doc, errs, _ = Parse(`<p a= >t</p>`)
	p := findElement(doc, "p")
	if v, has := p.Attr("a"); !has || v != "" || len(p.Attrs) != 1 {
		t.Errorf("the attributes are %v, want a with the empty string", p.Attrs)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "has no value") {
		t.Errorf("the findings are %v", errs)
	}
}

// TestWhiteSpaceIsHTMLsFiveCharacters. Where the tree builder asks whether text
// is only white space, it means tab, line feed, form feed, carriage return and
// space — not Unicode's spaces, which are characters of the page.
func TestWhiteSpaceIsHTMLsFiveCharacters(t *testing.T) {
	doc := mustParseHTML(t, `&nbsp;<p>x</p>`)
	if got := findElement(doc, "body").TextContent(); got != "\u00a0x" {
		t.Errorf("the body's text is %q; the no-break space was dropped as if it "+
			"were the newline between two tags", got)
	}
	doc, errs, _ := Parse(`<table><tr>&#x205F;<td>x</table>`)
	if kids := findElement(doc, "body").Children; len(kids) != 2 || kids[0].Text != "\u205f" {
		t.Errorf("the body is %s; a space that is not HTML's belongs before the table",
			tree(findElement(doc, "body")))
	}
	if !hasMessage(errs, "inside a table, outside any cell") {
		t.Errorf("the fostered text was not reported: %v", errs)
	}
}

// TestTheNamespaceAloneIsNotXHTML is the audit's document: an HTML5 doctype with
// the XHTML namespace on the root, which is Pandoc's template and a great deal of
// boilerplate, and which a browser opening the file reads as HTML.
func TestTheNamespaceAloneIsNotXHTML(t *testing.T) {
	src := "<!DOCTYPE html>\n<html xmlns=\"http://www.w3.org/1999/xhtml\" lang=\"en\"><body>" +
		"<pre>\ncode</pre><p>it&#146;s</p><br></br>"
	doc, errs, _ := Parse(src)
	if doc.XML {
		t.Error("the document was read as XHTML")
	}
	if got := findElement(doc, "pre").TextContent(); got != "code" {
		t.Errorf("the <pre> holds %q; HTML drops the newline after the tag", got)
	}
	if got := findElement(doc, "p").TextContent(); got != "it\u2019s" {
		t.Errorf("the paragraph reads %q; &#146; is a right single quotation mark in HTML", got)
	}
	brs := 0
	doc.Walk(func(n *Node) bool {
		if n.Type == ElementNode && n.Name == "br" {
			brs++
		}
		return true
	})
	if brs != 2 {
		t.Errorf("%d line breaks, want two: \"</br>\" is a <br> in HTML", brs)
	}
	// And the one thing to say about it is the "</br>".
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "</br>") {
		t.Errorf("the findings are %v, want the one about </br>", errs)
	}
}

// TestACallerCanSayTheDocumentIsXHTML. The type a server would send is the
// caller's to state, and a document that says nothing about itself is XHTML when
// it is: its stylesheet's CDATA is literal, its "&#146;" is U+0092, "<br></br>"
// is one break and "<pre>\n" keeps its newline.
func TestACallerCanSayTheDocumentIsXHTML(t *testing.T) {
	for _, src := range []string{
		`<html><head><style><![CDATA[a > b {}]]></style></head><body>`,
		`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head>` +
			`<style><![CDATA[a > b {}]]></style></head><body>`,
	} {
		src += "<pre>\ncode</pre><p>it&#146;s</p><br></br></body></html>"
		doc, errs, ok := ParseXHTML(src)
		if !ok {
			t.Errorf("%q was refused: %v", src, errs)
		}
		if !doc.XML {
			t.Errorf("%q: not read as XHTML", src)
		}
		if got := findElement(doc, "style").TextContent(); got != "a > b {}" {
			t.Errorf("%q: the stylesheet is %q", src, got)
		}
		if got := findElement(doc, "pre").TextContent(); got != "\ncode" {
			t.Errorf("%q: the <pre> holds %q", src, got)
		}
		if got := findElement(doc, "p").TextContent(); got != "it\u0092s" {
			t.Errorf("%q: the paragraph reads %q", src, got)
		}
		// Parse, told nothing, reads the same text as HTML: the second has an
		// HTML5 doctype and the namespace, which is not a signal.
		if doc, _, _ := Parse(src); doc.XML {
			t.Errorf("%q: Parse read it as XHTML", src)
		}
	}
	// The document's own signals still decide when the caller says nothing.
	if doc, _, _ := Parse(`<?xml version="1.0"?><html><body>x</body></html>`); !doc.XML {
		t.Error("an XML declaration no longer makes a document XHTML")
	}
	// And the frame handed back for an oversized document says what was asked.
	if doc, _, _ := ParseXHTML(strings.Repeat(" ", maxInputBytes+1)); !doc.XML {
		t.Error("the oversized document's frame is not marked XHTML")
	}
}

// TestCDATAInForeignContentIsNotADeclaration. SVG and MathML keep XML's CDATA
// sections inside an HTML document, and Illustrator and Inkscape put an SVG's
// <style> in one. It was refused as a declaration this engine does not read,
// and the skip ended at the ">" inside it.
func TestCDATAInForeignContentIsNotADeclaration(t *testing.T) {
	const style = `<style><![CDATA[ .a > .b {fill:red} ]]></style>`
	doc, errs, ok := Parse(`<p>a</p><svg>` + style + `</svg><p>b</p>`)
	if !ok {
		t.Errorf("refused: %v", errs)
	}
	if got := bodyShape(t, doc); got != `<p>('a')<svg><p>('b')` {
		t.Errorf("the body is %s", got)
	}
	if got := findElement(doc, "svg").Foreign; got != style {
		t.Errorf("the SVG's source is %q, want %q", got, style)
	}
	// Outside foreign content HTML has no such syntax, and it is still refused.
	if _, _, ok := Parse(`<p><![CDATA[x]]></p>`); ok {
		t.Error("a CDATA section in HTML content was accepted")
	}
}

// TestANULIsDealtWithEverywhere is one policy wherever the tokenizer makes text
// or an attribute: reported, and what the standard makes of it in that state —
// U+FFFD everywhere but ordinary text, where the tree builder drops it. The
// first fix reached ordinary text alone, and a <textarea>, a <title>, a
// stylesheet and every attribute value still carried the byte into the tree.
func TestANULIsDealtWithEverywhere(t *testing.T) {
	for _, tc := range []struct{ what, src, el, want string }{
		{"ordinary text", "<p>a\x00b</p>", "p", "ab"},
		{"a textarea", "<textarea>a\x00b</textarea>", "textarea", "a\uFFFDb"},
		{"a title", "<title>a\x00b</title>", "title", "a\uFFFDb"},
		{"a stylesheet", "<style>p{}\x00</style>", "style", "p{}\uFFFD"},
		{"raw text left open", "<style>p{}\x00", "style", "p{}\uFFFD"},
		{"an XHTML CDATA section",
			`<?xml version="1.0"?><html><body><p><![CDATA[a` + "\x00" + `b]]></p></body></html>`,
			"p", "a\uFFFDb"},
	} {
		doc, errs, ok := Parse(tc.src)
		if ok || !hasMessage(errs, "NUL") {
			t.Errorf("%s: the NUL was not reported: %v", tc.what, errs)
		}
		if got := findElement(doc, tc.el).TextContent(); got != tc.want {
			t.Errorf("%s: the text is %q, want %q", tc.what, got, tc.want)
		}
	}
	for _, tc := range []struct{ what, src, name, value string }{
		{"a quoted value", "<p title=\"a\x00b\">x</p>", "title", "a\uFFFDb"},
		{"an unquoted value", "<p title=a\x00b>x</p>", "title", "a\uFFFDb"},
		{"a value never closed", "<p title=\"a\x00b", "title", "a\uFFFDb"},
		{"a name", "<p a\x00b=1>x</p>", "a\uFFFDb", "1"},
	} {
		doc, errs, ok := Parse(tc.src)
		if ok || !hasMessage(errs, "NUL") {
			t.Errorf("%s: the NUL was not reported: %v", tc.what, errs)
		}
		if got, has := findElement(doc, "p").Attr(tc.name); !has || got != tc.value {
			t.Errorf("%s: the attribute is %q (present %v), want %q", tc.what, got, has, tc.value)
		}
	}
	// A CDATA section inside an SVG is kept as the SVG's source, but its NUL is
	// still read and still reported.
	if _, errs, _ := Parse("<svg><style><![CDATA[a\x00b]]></style></svg>"); !hasMessage(errs, "NUL") {
		t.Errorf("a NUL in foreign CDATA was not reported: %v", errs)
	}
}

// TestAnEmptyLangIsAnUnknownLanguage. HTML §3.2.6.2: lang="" says the language
// is unknown, and it stops the walk as a tag does. It was skipped as though
// absent, so Turkish casing and :lang(tr) reached text its author had marked as
// being in no language.
func TestAnEmptyLangIsAnUnknownLanguage(t *testing.T) {
	doc := mustParseHTML(t, `<div lang="tr"><span lang="">I</span><b>I</b></div>`)
	if v, ok := findElement(doc, "span").Language(); !ok || v != "" {
		t.Errorf("the span's language is %q (stated %v), want the empty string, stated", v, ok)
	}
	// Its sibling still inherits.
	if v, ok := findElement(doc, "b").Language(); !ok || v != "tr" {
		t.Errorf("the sibling's language is %q (stated %v), want \"tr\"", v, ok)
	}
	// And xml:lang="" in XHTML is the same statement.
	doc, _, _ = Parse(xhtmlDoctype + `<html xmlns="http://www.w3.org/1999/xhtml"><body>` +
		`<div xml:lang="tr"><span xml:lang="">I</span></div></body></html>`)
	if v, ok := findElement(doc, "span").Language(); !ok || v != "" {
		t.Errorf("in XHTML the span's language is %q (stated %v), want the empty string", v, ok)
	}
}

// TestTheTruncationIsReportedPastTheCap. A document with a hundred problems
// that then reaches a bound was handed back as a short tree whose only word
// about it was "further problems were not reported", which says the list is
// short and not that the tree is.
func TestTheTruncationIsReportedPastTheCap(t *testing.T) {
	src := strings.Repeat("</x>", maxErrors+5) + strings.Repeat("<div>", maxDepth+10)
	_, errs, _ := Parse(src)
	if !hasMessage(errs, "further problems in this document were not reported") {
		t.Fatalf("the fixture did not reach the cap: %d findings", len(errs))
	}
	var stop *Error
	for i := range errs {
		if strings.Contains(errs[i].Message, "nested more deeply") {
			stop = &errs[i]
		}
	}
	if stop == nil {
		t.Fatalf("nothing said the rest of the document was not read: the last findings are %v",
			errs[len(errs)-2:])
	}
	if !stop.Limit || !strings.Contains(stop.Message, "the rest was not read") {
		t.Errorf("the truncation finding is %+v", *stop)
	}
	// Still bounded: the cap, its own marker, and the one that stopped the read.
	if len(errs) != maxErrors+2 {
		t.Errorf("%d findings, want %d", len(errs), maxErrors+2)
	}
}
