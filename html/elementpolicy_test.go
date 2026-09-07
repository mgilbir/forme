package html

import (
	"strings"
	"testing"
)

// TestNoscriptContentIsShown is the element whose reason was backwards.
//
// <noscript> is what an author writes for a reader whose engine runs no script,
// and this engine runs no script — so a document is *always* in the case the
// element was written for and its content is always what should be shown. It
// was dropped, with the reason "there is no script for this to be an
// alternative to".
func TestNoscriptContentIsShown(t *testing.T) {
	doc, _, ok := Parse(`<p>a</p><noscript><p>b</p></noscript>`)
	const want = "#doc(<html>(<head><body>(<p>('a')<noscript>(<p>('b')))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}
	if !ok {
		t.Error("a document with a <noscript> in it was refused")
	}
}

// TestScriptIsStillDropped is the control. The reason <script> is dropped has
// nothing to do with <noscript>'s: it is the code-execution surface, and a
// renderer that read it silently would still be one that had read it.
func TestScriptIsStillDropped(t *testing.T) {
	doc, errs, ok := Parse(`<p>a</p><script>alert(1)</script>`)
	const want = "#doc(<html>(<head><body>(<p>('a'))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}
	if ok {
		t.Error("a document with a <script> in it was accepted without a word")
	}
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "scripts are never run") {
		t.Errorf("reported %v, want the script message", errs)
	}
}

// TestAnEmptyElementTagIsEmptyInXML is the rule applied to half the elements.
//
// XML has self-closing syntax and it means an empty element, and this parser
// read it that way for the elements HTML has never heard of and not for the
// ones it has — so "<my-widget/>" was right and "<div/>" in the same document
// was reported as a mistake and opened, swallowing everything after it.
func TestAnEmptyElementTagIsEmptyInXML(t *testing.T) {
	const head = `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml">`
	for _, tc := range []struct{ name, src, want string }{
		{"a known element", head + `<body><div/><p>after</p></body></html>`,
			"#doc(<html>(<head><body>(<div><p>('after'))))"},
		{"an unknown element", head + `<body><my-widget/><p>after</p></body></html>`,
			"#doc(<html>(<head><body>(<my-widget><p>('after'))))"},
		{"a span", head + `<body><span/>after</body></html>`,
			"#doc(<html>(<head><body>(<span>'after')))"},
	} {
		doc, errs, ok := Parse(tc.src)
		if got := shapeOfTree(doc); got != tc.want {
			t.Errorf("%s: %s\n   want %s", tc.name, got, tc.want)
		}
		if !ok {
			t.Errorf("%s: refused: %v", tc.name, errs)
		}
	}
}

// TestAnEmptyElementTagIsStillRefusedInHTML is the other half: HTML has no
// self-closing syntax outside the void elements, a browser reads "<div/>" as an
// open div, and a document that wrote one has a shape that moves half the page.
func TestAnEmptyElementTagIsStillRefusedInHTML(t *testing.T) {
	doc, errs, ok := Parse(`<div/><p>after</p>`)
	const want = "#doc(<html>(<head><body>(<div>(<p>('after')))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}
	if ok {
		t.Error("<div/> in an HTML document was accepted without a word")
	}
	var said bool
	for _, e := range errs {
		if strings.Contains(e.Message, "no self-closing syntax") {
			said = true
		}
	}
	if !said {
		t.Errorf("reported %v, want the self-closing message", errs)
	}
}

// TestACDATASectionIsTextInXML is XML's way of saying "the characters between
// these markers are literal".
//
// It was skipped to the first ">", which is what a declaration gets: the
// content was dropped, and where the content held a ">" the rest of the
// document went with it.
func TestACDATASectionIsTextInXML(t *testing.T) {
	const head = `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml">`
	for _, tc := range []struct{ name, src, want string }{
		{"ordinary content", head + `<body><p><![CDATA[x < y]]></p></body></html>`,
			"#doc(<html>(<head><body>(<p>('x < y'))))"},
		{"a greater-than inside it",
			head + `<body><p><![CDATA[a > b]]>after</p></body></html>`,
			"#doc(<html>(<head><body>(<p>('a > b' + 'after'))))"},
		{"markup that is not markup",
			head + `<body><p><![CDATA[<div>]]></p></body></html>`,
			"#doc(<html>(<head><body>(<p>('<div>'))))"},
	} {
		doc, errs, ok := Parse(tc.src)
		got := shapeOfTree(doc)
		if !strings.Contains(got, "x < y") && !strings.Contains(got, "a > b") &&
			!strings.Contains(got, "<div>") {
			t.Errorf("%s: %s — the section's content is gone", tc.name, got)
		}
		if !ok {
			t.Errorf("%s: refused: %v", tc.name, errs)
		}
	}

	// And the whole of the document after it survives, which is the part that
	// was being swallowed.
	doc, _, _ := Parse(head + `<body><p><![CDATA[a > b]]></p><p>after</p></body></html>`)
	if got := shapeOfTree(doc); !strings.Contains(got, "'after'") {
		t.Errorf("%s — the content after the section is gone", got)
	}
}

// TestAReferenceInAnAttributeFollowedByAnEqualsIsNotOne is HTML's one clause
// about an attribute.
//
// A name with no ";" followed by "=" or an alphanumeric is not a reference at
// all, and is not a parse error either. "?q=1&copy=2" is a query string, in
// this engine and in every browser — and it was reported as something "a
// character reference in some browsers", which no browser makes it.
func TestAReferenceInAnAttributeFollowedByAnEqualsIsNotOne(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a query string", `<a href="/s?q=1&copy=2">x</a>`},
		{"a name followed by a letter", `<a href="/s?a&copyright">x</a>`},
		{"a name followed by a digit", `<a href="/s?a&copy2">x</a>`},
	} {
		_, errs, ok := Parse(tc.src)
		if !ok {
			t.Errorf("%s: %q was refused: %v", tc.name, tc.src, errs)
		}
	}
}

// TestAReferenceInAnAttributeThatABrowserWouldExpandIsStillReported is the
// other side of the same clause: where the name is *not* followed by "=" or an
// alphanumeric, a browser does expand it and this engine does not, and that
// difference is worth telling an author about.
func TestAReferenceInAnAttributeThatABrowserWouldExpandIsStillReported(t *testing.T) {
	_, errs, ok := Parse(`<a href="/s?a&copy">x</a>`)
	if ok {
		t.Fatal(`"&copy" at the end of an attribute was accepted without a word`)
	}
	var said bool
	for _, e := range errs {
		if strings.Contains(e.Message, "character reference in some browsers") {
			said = true
		}
	}
	if !said {
		t.Errorf("reported %v, want the difference from a browser", errs)
	}
}
