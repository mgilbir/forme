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
