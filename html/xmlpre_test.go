package html

import "testing"

// The leading newline inside a <pre>, and the one document model that keeps it.
//
// HTML §13.2.6.4.7 drops the line feed immediately after a <pre> or <textarea>
// start tag, and TestLeadingNewlineIsDropped pins that. The rule lives in
// HTML's *tree construction*, and XML has no tree construction — an XHTML
// document is parsed by an XML parser, which hands the newline through like any
// other character. The suite says so on the page: white-space-pre-001.xht is a
// <pre> whose reference is a box one line taller than the pattern in it, and
// that line is the newline after the start tag.

// xmlPrologue is the shortest thing looksLikeXML recognises, so that these
// fixtures differ from the HTML ones by the document model and nothing else.
const xmlPrologue = `<?xml version="1.0" encoding="UTF-8"?>`

func TestAnXHTMLDocumentKeepsTheNewlineAfterItsPre(t *testing.T) {
	for _, tc := range []struct{ name, elem, src string }{
		{"pre", "pre", "<pre>\nhello</pre>"},
		{"textarea", "textarea", "<textarea>\nhello</textarea>"},
	} {
		doc := mustParseHTML(t, xmlPrologue+tc.src)
		if !doc.XML {
			t.Fatalf("%s: the fixture was not read as XHTML, so it cannot say "+
				"what it means to say", tc.name)
		}
		el := doc.Element(tc.elem)
		if el == nil {
			t.Fatalf("%s: no <%s>:\n%s", tc.name, tc.elem, tree(doc))
		}
		if got := el.TextContent(); got != "\nhello" {
			t.Errorf("%s: content is %q, want %q — XML has no rule that drops "+
				"it", tc.name, got, "\nhello")
		}
	}
}

// TestAnHTMLDocumentStillDropsIt, because the fixtures above differ from the
// ones in TestLeadingNewlineIsDropped by one prologue and it is worth saying
// that the prologue is what does it.
func TestAnHTMLDocumentStillDropsIt(t *testing.T) {
	doc := mustParseHTML(t, "<pre>\nhello</pre>")
	if doc.XML {
		t.Fatal("a document with no prologue was read as XHTML")
	}
	if got := doc.Element("pre").TextContent(); got != "hello" {
		t.Errorf("content is %q, want %q", got, "hello")
	}
}
