package html

import (
	"testing"

	"github.com/mgilbir/forme/internal/ascii"
)

// TestTheForeignNameTables holds the tables to the shape §13.2.6.1 and
// §13.2.6.5 give them: each entry maps a name folded to lower case back to the
// SVG name it came from, and SVG's name is not already lower case — an entry
// that were would be one the table does not need. The counts are the
// standard's, 37 element names and 58 attribute names; the entries were
// compared with it one by one when they were written.
func TestTheForeignNameTables(t *testing.T) {
	for name, table := range map[string]struct {
		m    map[string]string
		want int
	}{"tag names": {svgTagNames, 37}, "attributes": {svgAttributeNames, 58}} {
		if len(table.m) != table.want {
			t.Errorf("%s: %d entries, want %d", name, len(table.m), table.want)
		}
		for lower, svg := range table.m {
			if ascii.Lower(svg) != lower || svg == lower {
				t.Errorf("%s: %q -> %q is not a case restored", name, lower, svg)
			}
		}
	}
	for in, want := range map[string]string{
		"foreignobject": "foreignObject", "fedropshadow": "feDropShadow", "rect": "rect",
		"lineargradient": "linearGradient",
	} {
		if got := AdjustSVGTagName(in); got != want {
			t.Errorf("AdjustSVGTagName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"viewbox": "viewBox", "preserveaspectratio": "preserveAspectRatio",
		"width": "width", "xlink:href": "xlink:href",
	} {
		if got := AdjustSVGAttributeName(in); got != want {
			t.Errorf("AdjustSVGAttributeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := AdjustMathMLAttributeName("definitionurl"); got != "definitionURL" {
		t.Errorf("AdjustMathMLAttributeName(definitionurl) = %q", got)
	}
}

// TestAForeignRootHasItsOwnNamesInHTML. HTML folds "VIEWBOX" to "viewbox" and
// the tree builder gives SVG its "viewBox" back, so the <svg> element carries
// the name SVG defines — and a selector or attr() that asks for it by that
// name, as XML's rules for a foreign element say it must, finds it.
func TestAForeignRootHasItsOwnNamesInHTML(t *testing.T) {
	doc := mustParseHTML(t, `<p>a</p><svg VIEWBOX="0 0 1 1" Width="3"></svg>`+
		`<math DEFINITIONURL="u"></math>`)
	svg := findElement(doc, "svg")
	if got, ok := svg.AttrExact("viewBox"); !ok || got != "0 0 1 1" {
		t.Errorf("the <svg> has no viewBox: %v", svg.Attrs)
	}
	if _, ok := svg.AttrNamed("viewbox", false); ok {
		t.Error("svg[viewbox] matched: an SVG element's names are case-sensitive once adjusted")
	}
	if _, ok := svg.AttrNamed("viewBox", false); !ok {
		t.Error("svg[viewBox] did not match")
	}
	if got, ok := svg.Attr("width"); !ok || got != "3" {
		t.Errorf("width is %q, %v", got, ok)
	}
	math := findElement(doc, "math")
	if _, ok := math.AttrExact("definitionURL"); !ok {
		t.Errorf("the <math> has no definitionURL: %v", math.Attrs)
	}
	// XHTML is XML, and nothing is adjusted: "VIEWBOX" stays what it says.
	xdoc, _, _ := Parse(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body>` +
		`<svg xmlns="http://www.w3.org/2000/svg" VIEWBOX="0 0 1 1"/></body></html>`)
	xsvg := findElement(xdoc, "svg")
	if _, ok := xsvg.AttrExact("viewBox"); ok {
		t.Error("XHTML: VIEWBOX was read as viewBox")
	}
}

// TestANamespaceNameIsComparedAsSpelled. Namespaces in XML §2.3: two
// namespace names are the same only when they are the same characters. The
// SVG namespace with its "svg" in capitals is another namespace, and a prefix
// bound to it leaves the element one this engine does not know; the MathML
// namespace is spelled with capitals and resolves as spelled.
func TestANamespaceNameIsComparedAsSpelled(t *testing.T) {
	for _, tc := range []struct {
		ns, tag, want string
	}{
		{"http://www.w3.org/2000/svg", "s:svg", "svg"},
		{"http://www.w3.org/2000/SVG", "s:svg", "s:svg"},
		{"http://www.w3.org/1998/Math/MathML", "s:math", "math"},
		{"http://www.w3.org/1998/math/mathml", "s:math", "s:math"},
	} {
		doc, _, _ := Parse(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:s="` +
			tc.ns + `"><body><` + tc.tag + `/></body></html>`)
		if findElement(doc, tc.want) == nil {
			t.Errorf("<%s> with s bound to %s: no element named %q", tc.tag, tc.ns, tc.want)
		}
	}
}

// TestAContentLanguagePragmaIsTheDocumentsLanguage is HTML §4.2.5.3's content
// language state and §3.2.6.2's last step: with no lang above it, a node's
// language is the one a <meta http-equiv="content-language"> set.
func TestAContentLanguagePragmaIsTheDocumentsLanguage(t *testing.T) {
	for _, tc := range []struct {
		head, want string
		declared   bool
		what       string
	}{
		{`<meta http-equiv="content-language" content="tr">`, "tr", true, "the pragma"},
		{"<meta http-equiv=\"Content-Language\" content=\" \t\n tr  en\">", "tr", true,
			"the keyword folded, white space skipped, the first token taken"},
		{`<meta http-equiv="content-language" content="tr, en">`, "", false,
			"a comma: the whole value is ignored"},
		{`<meta http-equiv="content-language" content="  ">`, "", false, "nothing but white space"},
		{`<meta http-equiv="content-language">`, "", false, "no content"},
		{`<meta http-equiv=" content-language" content="tr">`, "", false,
			"an http-equiv that is no keyword"},
		{`<meta http-equiv="content-language" content="de">` +
			`<meta http-equiv="content-language" content="tr">`, "tr", true, "the last one"},
		{`<meta http-equiv="content-language" content="tr">` +
			`<meta http-equiv="content-language" content="de,en">`, "tr", true,
			"a later one that stops early leaves the earlier standing"},
		{`<meta name="content-language" content="tr">`, "", false, "a name, not an http-equiv"},
		{"<meta http-equiv=\"content-language\" content=\"\u00a0tr\">", "\u00a0tr", true,
			"a no-break space is not ASCII white space"},
	} {
		doc := mustParseHTML(t, `<!DOCTYPE html><html><head>`+tc.head+`</head><body><p>x</p></body></html>`)
		p := findElement(doc, "p")
		tag, ok := p.Language()
		if tag != tc.want || ok != tc.declared {
			t.Errorf("%s: Language() = %q, %v, want %q, %v", tc.what, tag, ok, tc.want, tc.declared)
		}
		var l Languages
		if tag2, ok2 := l.Of(p); tag2 != tag || ok2 != ok {
			t.Errorf("%s: Languages.Of = %q, %v, and Language() = %q, %v", tc.what, tag2, ok2, tag, ok)
		}
	}
	// A lang anywhere above wins, and lang="" too: it is an answer.
	doc := mustParseHTML(t, `<!DOCTYPE html><html lang="de"><head>`+
		`<meta http-equiv="content-language" content="tr"></head><body><p>x</p><div lang=""><span>y</span></div></body></html>`)
	if tag, _ := findElement(doc, "p").Language(); tag != "de" {
		t.Errorf("under <html lang=de> the language is %q", tag)
	}
	if tag, ok := findElement(doc, "span").Language(); tag != "" || !ok {
		t.Errorf("under lang=\"\" the language is %q, %v", tag, ok)
	}
	// A pragma in the body is inserted into the document too.
	doc = mustParseHTML(t, `<!DOCTYPE html><html><body><p>x</p><meta http-equiv="content-language" content="tr"></body></html>`)
	if tag, _ := findElement(doc, "p").Language(); tag != "tr" {
		t.Errorf("a pragma in the body: the language is %q", tag)
	}
}
