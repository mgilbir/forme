package html

import (
	"strings"
	"testing"
)

// elementNames is every element in a parsed document, in tree order.
func elementNames(doc *Node) []string {
	var out []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode {
			out = append(out, n.Name)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// parentOf is the name of the element holding the first element called name.
func parentOf(doc *Node, name string) string {
	var found string
	var walk func(*Node, string)
	walk = func(n *Node, parent string) {
		if n.Type == ElementNode && n.Name == name && found == "" {
			found = parent
		}
		for _, c := range n.Children {
			next := parent
			if n.Type == ElementNode {
				next = n.Name
			}
			walk(c, next)
		}
	}
	walk(doc, "")
	return found
}

// TestACommentEndsWhereHTMLSaysItDoes.
//
// Three ways to close a comment and only "-->" was read, so the other two ran
// to the end of the document and took every element after them with it — one
// "<!-->" in a template emptied the page, reported as a comment that was never
// closed.
func TestACommentEndsWhereHTMLSaysItDoes(t *testing.T) {
	for _, tc := range []struct{ src, what string }{
		{`<!-- ordinary --><p>after</p>`, "the ordinary close"},
		{`<!--><p>after</p>`, "an abrupt close of an empty comment"},
		{`<!---><p>after</p>`, "an abrupt close after one dash"},
		{`<!-- x --!><p>after</p>`, "an incorrectly closed comment"},
		{`<!----!><p>after</p>`, "an incorrect close of an empty comment"},
		{`<!-- a --><!-- b --><p>after</p>`, "two comments"},
		{`<!-- --!> --><p>after</p>`, "an incorrect close before an ordinary one"},
	} {
		doc, _, _ := Parse(tc.src)
		if got := doc.TextContent(); !strings.Contains(got, "after") {
			t.Errorf("%s: the document after the comment is %q, and the text after "+
				"it is gone", tc.what, got)
		}
	}
	// And one that really is never closed is still reported.
	if _, _, ok := Parse(`<!-- never<p>after</p>`); ok {
		t.Error("a comment with no terminator at all was accepted")
	}
}

// TestAVoidElementNobodyKnowsIsStillVoid. A void element has no content and no
// end tag; opening one puts every following element inside it.
func TestAVoidElementNobodyKnowsIsStillVoid(t *testing.T) {
	doc, errs, ok := Parse(`<p>a</p><track kind=subs><p id=after>b</p>`)
	if !ok {
		t.Errorf("a void element was reported: %v", errs)
	}
	if got := parentOf(doc, "p"); got == "track" {
		t.Error("the paragraph after a <track> is inside it")
	}
	for _, e := range errs {
		if strings.Contains(e.Message, "never closed") {
			t.Errorf("a tag that is never written closed was reported as one: %s", e.Message)
		}
	}
}

// TestAnUnknownElementStillClosesAParagraph. The rule is about the tag's name
// and not about what this engine can lay out, so an element it has not heard of
// ends an open paragraph exactly as a known one does.
func TestAnUnknownElementStillClosesAParagraph(t *testing.T) {
	doc, _, _ := Parse(`<p>a<menu>b</menu>`)
	if got := parentOf(doc, "menu"); got == "p" {
		t.Error("a <menu> was nested inside the open <p> it ends")
	}
}

// TestANumericReferenceInTheC1RangeIsWindows1252.
//
// It is what the standard's own table says and what every browser does: a page
// written by a Windows editor spells a curly apostrophe "&#146;" and a euro
// "&#128;", and reading those as control characters puts a character nothing
// draws where a letter belongs.
func TestANumericReferenceInTheC1RangeIsWindows1252(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		what string
	}{
		{"&#128;", "\u20ac", "a euro"},
		{"&#146;", "\u2019", "a right single quote"},
		{"&#147;&#148;", "\u201c\u201d", "curly double quotes"},
		{"&#133;", "\u2026", "an ellipsis"},
		{"&#x85;", "\u2026", "the same in hex"},
		{"&#129;", "\u0081", "one of the seven windows-1252 leaves unassigned"},
		{"&#160;", "\u00a0", "past the range, which is a no-break space"},
		{"&#65;", "A", "below it"},
	} {
		doc, _, ok := Parse("<p>" + tc.src + "</p>")
		if !ok {
			t.Errorf("%s: %q was reported", tc.what, tc.src)
		}
		if got := doc.TextContent(); got != tc.want {
			t.Errorf("%s: %q is %q, want %q", tc.what, tc.src, got, tc.want)
		}
	}
	// Not in XHTML. XML 1.0 §4.1 says a reference is the code point it names,
	// which is what the suite's control-characters-002.xht is written to test.
	doc, _, _ := Parse(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml">` +
		`<body><p>&#133;</p></body></html>`)
	if got := doc.TextContent(); got != "\u0085" {
		t.Errorf("in XHTML &#133; is %q, want U+0085 — the mapping is HTML's", got)
	}
}

// TestRawTextElementsAreNotMarkup. <xmp> says on its face that the tags in it
// are to be shown.
func TestRawTextElementsAreNotMarkup(t *testing.T) {
	for _, name := range []string{"xmp", "noembed", "noframes"} {
		doc, _, _ := Parse("<" + name + "><b>not bold</b></" + name + ">")
		for _, n := range elementNames(doc) {
			if n == "b" {
				t.Errorf("<%s> had its content read as markup: a <b> was parsed out of it", name)
			}
		}
		if got := doc.TextContent(); !strings.Contains(got, "<b>not bold</b>") {
			t.Errorf("<%s> holds %q, want the tags as text", name, got)
		}
	}
}

// TestANULByteIsNotText. U+0000 is a parse error in every tokenizer state that
// can meet one, and the states that produce text drop it. Keeping it put a byte
// that is not a character into a text node — into the shaper, into whatever a
// caller does with Node.Text — and reported success.
func TestANULByteIsNotText(t *testing.T) {
	doc, errs, ok := Parse("<p>a\x00b\x00c</p>")
	if ok {
		t.Error("a document holding NUL bytes parsed with no errors")
	}
	if got := doc.TextContent(); got != "abc" {
		t.Errorf("the text is %q, want %q", got, "abc")
	}
	n := 0
	for _, e := range errs {
		if strings.Contains(e.Message, "NUL") {
			n++
			if !strings.Contains(e.Message, "2 NUL bytes") {
				t.Errorf("the finding is %q; it should count them", e.Message)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d findings for a run holding two NULs, want one", n)
	}
	// And a document with none gains nothing.
	if _, errs, ok := Parse("<p>ab</p>"); !ok || len(errs) != 0 {
		t.Errorf("a document with no NUL in it was reported: %v", errs)
	}
}
