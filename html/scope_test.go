package html

import (
	"strings"
	"testing"
)

// The optional end tags, held to the standard's own statement of them.
//
// They were a table of pairs consulted against the top of the stack alone, which
// is right exactly when the element being ended is the innermost one. The
// documents where it is not are ordinary ones — "<li><p>a<li>" leaves a
// paragraph open inside the item — and in each of them the next item or cell
// went inside the paragraph, silently, because the final end tag then closed
// the whole mis-nest as optional end tags. They are written now in the
// standard's primitives, and these tables are written from the standard: each
// expected tree is what §13.2.6.4.7 and the table modes build, which is what a
// browser shows.

// bodyShape is the shape of what a document puts in its body: elements by name,
// text quoted and trimmed. See shapeOfTree.
func bodyShape(t *testing.T, doc *Node) string {
	t.Helper()
	el := doc.Element("body")
	if el == nil {
		t.Fatal("no body")
	}
	var b strings.Builder
	for _, c := range el.Children {
		b.WriteString(shapeOfTree(c))
	}
	return b.String()
}

// TestOmittedEndTagsParseAsABrowserParsesThem is markup the standard's parser
// reads without a parse error: every end tag left out here is one the tree
// construction rules imply, so every document must parse with nothing to report
// and to the tree a browser builds.
func TestOmittedEndTagsParseAsABrowserParsesThem(t *testing.T) {
	for _, tc := range []struct{ what, src, want string }{
		// The audit's three: a paragraph left open inside an item or a cell
		// does not swallow the next one.
		{"a paragraph in a list item", `<ul><li><p>a<li><p>b</ul>`,
			`<ul>(<li>(<p>('a'))<li>(<p>('b')))`},
		{"a paragraph in a definition term", `<dl><dt><p>a<dd>b</dl>`,
			`<dl>(<dt>(<p>('a'))<dd>('b'))`},
		{"a paragraph in a definition", `<dl><dd><p>a<dt>b<dd><p>c<dd>d</dl>`,
			`<dl>(<dd>(<p>('a'))<dt>('b')<dd>(<p>('c'))<dd>('d'))`},
		{"a paragraph in a cell", `<table><tr><td><p>a<td><p>b<tr><td>c</table>`,
			`<table>(<tr>(<td>(<p>('a'))<td>(<p>('b')))<tr>(<td>('c')))`},
		{"a paragraph in a header cell", `<table><tr><th><p>a<th>b<td>c</table>`,
			`<table>(<tr>(<th>(<p>('a'))<th>('b')<td>('c')))`},
		{"a paragraph in a caption", `<table><caption><p>a<tr><td>b</table>`,
			`<table>(<caption>(<p>('a'))<tr>(<td>('b')))`},

		// Row groups and rows end on the next one, from however deep inside.
		{"a row group after a cell", `<table><tr><td>a<tbody><tr><td>b</table>`,
			`<table>(<tr>(<td>('a'))<tbody>(<tr>(<td>('b'))))`},
		{"every row group in turn",
			`<table><thead><tr><td>a<tbody><tr><td>b<tfoot><tr><td>c</table>`,
			`<table>(<thead>(<tr>(<td>('a')))<tbody>(<tr>(<td>('b')))<tfoot>(<tr>(<td>('c'))))`},
		{"a body after a body", `<table><tbody><tr><td>a<tbody><tr><td>b</table>`,
			`<table>(<tbody>(<tr>(<td>('a')))<tbody>(<tr>(<td>('b'))))`},
		{"a column group after a column group's column",
			`<table><colgroup><col><tr><td>a</table>`,
			`<table>(<colgroup>(<col>)<tr>(<td>('a')))`},

		// A table inside a cell is its own table: its cells end its cells.
		{"a table nested in a cell",
			`<table><tr><td><table><tr><td>a<td>b</table><td>c</table>`,
			`<table>(<tr>(<td>(<table>(<tr>(<td>('a')<td>('b'))))<td>('c')))`},
		// And an end tag for a table part closes through the parts inside it.
		{"a row end tag closes the cell", `<table><tr><td><p>a</tr><tr><td>b</table>`,
			`<table>(<tr>(<td>(<p>('a')))<tr>(<td>('b')))`},

		// "Close a p element": every start tag on the standard's list, including
		// the ones the old table left out.
		{"listing", "<p>a<listing>\nb</listing>", `<p>('a')<listing>('b')`},
		{"xmp", `<p>a<xmp><b>b</b></xmp>`, `<p>('a')<xmp>('<b>b</b>')`},
		{"center", `<p>a<center>b</center>`, `<p>('a')<center>('b')`},
		{"dir", `<p>a<dir><li>b</dir>`, `<p>('a')<dir>(<li>('b'))`},
		{"search", `<p>a<search>b</search>`, `<p>('a')<search>('b')`},
		{"menu", `<p>a<menu><li>b</menu>`, `<p>('a')<menu>(<li>('b'))`},
		{"li", `<ul><li><p>a<ul><li>b</ul></ul>`, `<ul>(<li>(<p>('a')<ul>(<li>('b'))))`},
		{"a table", `<p>a<table><tr><td>b</table>`, `<p>('a')<table>(<tr>(<td>('b')))`},
		// And a paragraph the table is inside is not a cell's to close.
		{"a paragraph outside the table",
			`<p>a<object><table><tr><td><p>b</table></object>c</p>`,
			`<p>('a'<object>(<table>(<tr>(<td>(<p>('b')))))'c')`},
		// Nor is one outside a button.
		{"a paragraph outside a button", `<p>a<button><div>b</div></button>c</p>`,
			`<p>('a'<button>(<div>('b'))'c')`},

		// Ruby's annotations end one another, and an annotation container
		// survives the annotations inside it.
		{"ruby", `<ruby>a<rb>b<rt>c<rtc>d<rt>e<rp>f</ruby>`,
			`<ruby>('a'<rb>('b')<rt>('c')<rtc>('d'<rt>('e')<rp>('f')))`},
		{"ruby parentheses", `<ruby>a<rp>(<rt>b<rp>)</ruby>`,
			`<ruby>('a'<rp>('(')<rt>('b')<rp>(')'))`},
		{"options", `<select><option>a<optgroup><option>b<optgroup><option>c</select>`,
			`<select>(<option>('a')<optgroup>(<option>('b'))<optgroup>(<option>('c')))`},

		// Block end tags close the implied end tags inside them, and no more.
		{"an object's paragraph", `<object><p>a</object>b`, `<object>(<p>('a'))'b'`},
		{"a button's paragraph", `<button><p>a</button>b`, `<button>(<p>('a'))'b'`},
		{"a noscript's paragraph closed by its own tag",
			`<p>a</p><noscript><p>b</p></noscript>`, `<p>('a')<noscript>(<p>('b'))`},
		{"the obsolete void elements", `<p><basefont>a<bgsound>b<keygen>c</p>`,
			`<p>(<basefont>'a'<bgsound>'b'<keygen>'c')`},
		// A link in a cell is not inside the link outside the table: a cell is
		// a marker on the list of formatting elements.
		{"a link in a cell, inside a link", `<a>a<table><tr><td><a>b</a></table></a>`,
			`<a>('a'<table>(<tr>(<td>(<a>('b')))))`},
	} {
		doc, errs, ok := Parse(tc.src)
		if !ok || len(errs) != 0 {
			t.Errorf("%s: %q was refused, and it is valid HTML: %v", tc.what, tc.src, errs)
		}
		if got := bodyShape(t, doc); got != tc.want {
			t.Errorf("%s: %q\n got %s\nwant %s", tc.what, tc.src, got, tc.want)
		}
	}
}

// TestANoscriptInTheHeadStaysInTheHead is HTML's "in head noscript" mode, which
// scripting being off puts a head <noscript> in — and scripting is always off
// here. It started the body, and so moved what the head still had to say into
// it, then refused the document for a "</head>" that closed nothing.
func TestANoscriptInTheHeadStaysInTheHead(t *testing.T) {
	for _, tc := range []struct{ what, src, want string }{
		{"the audit's document",
			`<head><noscript><link rel=stylesheet href=a.css></noscript><title>t</title></head>` +
				`<body><p>x</p></body>`,
			`#doc(<html>(<head>(<noscript>(<link>)<title>('t'))<body>(<p>('x'))))`},
		{"a stylesheet and a meta",
			`<!DOCTYPE html><html><head><noscript><style>p{}</style><meta name=x></noscript></head><p>x`,
			`#doc(<html>(<head>(<noscript>(<style>('p{}')<meta>))<body>(<p>('x'))))`},
		{"with no head tag at all, which puts it in the head all the same",
			`<noscript><link href=a.css rel=stylesheet></noscript><p>x`,
			`#doc(<html>(<head>(<noscript>(<link>))<body>(<p>('x'))))`},
		{"white space inside it", "<head><noscript> \n </noscript></head><p>x",
			`#doc(<html>(<head>(<noscript>())<body>(<p>('x'))))`},
	} {
		doc, errs, ok := Parse(tc.src)
		if !ok || len(errs) != 0 {
			t.Errorf("%s: refused: %v", tc.what, errs)
		}
		if got := shapeOfTree(doc); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.what, got, tc.want)
		}
	}
}

// TestMalformedNestingIsStillRefused is the other half, and the package's
// design: what is not valid HTML is refused with a message. Each of these is a
// parse error in the standard, so each must leave ok false, and each tree is
// the one the standard builds, so that a caller rendering the document anyway
// is handed a browser's reading and not a third one.
func TestMalformedNestingIsStillRefused(t *testing.T) {
	for _, tc := range []struct{ what, src, want, message string }{
		{"a heading inside a heading", `<h1>a<h2>b</h2></h1>c`,
			`<h1>('a')<h2>('b')'c'`, "headings do not nest"},
		{"a heading end tag for another heading", `<h1>a</h2>b`,
			`<h1>('a')'b'`, "does not match the heading it ends"},
		{"an inline element open when the next item starts",
			`<ul><li><span>a<li>b</ul>`,
			`<ul>(<li>(<span>('a'))<li>('b'))`, "<span> inside it is still open"},
		{"a block open when the next item starts, which the search passes",
			`<ul><li><div>a<li>b</ul>`,
			`<ul>(<li>(<div>('a'))<li>('b'))`, "<div> inside it is still open"},
		{"a block open when the next cell starts",
			`<table><tr><td><div>a<td>b</table>`,
			`<table>(<tr>(<td>(<div>('a'))<td>('b')))`, "<div> inside it is still open"},
		{"an inline element open when a block ends the paragraph",
			`<p><b>a<div>b</div>`, `<p>(<b>('a'))<div>('b')`, "<b> inside it is still open"},
		{"an inline end tag outside the paragraph it would close", `<b><p>a</b>b</p>`,
			`<b>(<p>('ab'))`, "cannot close the <b> outside the <p>"},
		{"a block end tag outside the cell it is in",
			`<div><table><tr><td>a</div>b</table></div>c`,
			`<div>(<table>(<tr>(<td>('ab'))))'c'`, "cannot close the <div> outside the <td>"},
		{"a paragraph end tag outside the button it is in", `<p>a<button>b</p>c</button>`,
			`<p>('a'<button>('bc'))`, "cannot close the <p> outside the <button>"},
		{"an end tag for nothing", `<div>a</span>b</div>`,
			`<div>('ab')`, "closes nothing"},
		{"a button inside a button", `<button>a<button>b</button>`,
			`<button>('a')<button>('b')`, "buttons do not nest"},
		{"a link inside a link", `<a href=x>a<a href=y>b</a></a>`,
			`<a>('a'<a>('b'))`, "they do not nest"},
		{"a nobr inside a nobr", `<nobr>a<nobr>b</nobr></nobr>`,
			`<nobr>('a'<nobr>('b'))`, "they do not nest"},
		{"a form inside a form", `<form><div><form></form></div></form>`,
			`<form>(<div>(<form>))`, "forms do not nest"},
		{"a table directly inside a table", `<table><tr><table><tr><td>a</table>`,
			`<table>(<tr>)<table>(<tr>(<td>('a')))`, "ends the first table"},
		// A cell ends what was fostered out of its row — "clear the stack back
		// to a table row context" — and is a cell of the row, not of the
		// fostered block in front of the table.
		{"a cell after a block fostered out of the row",
			`<table><tr><td>a</td><div>b<td>c</table>`,
			`<div>('b')<table>(<tr>(<td>('a')<td>('c')))`, "not table content"},
		// Anything but a <col> ends a column group, a foreign element included,
		// and then it is not table content.
		{"a picture in a column group", `<table><colgroup><svg></svg></table>`,
			`<svg><table>(<colgroup>)`, "not table content"},
		{"an annotation outside its ruby", `<ruby>a<span>b<rt>c</rt></span></ruby>`,
			`<ruby>('a'<span>('b'<rt>('c')))`, "belongs directly in a <ruby>"},
		{"a paragraph inside a noscript in the head", `<head><noscript><p>x</p></noscript>`,
			`<p>('x')`, "cannot be inside a <noscript> in the head"},
		{"text inside a noscript in the head", `<head><noscript>x</noscript>`,
			`'x'`, "text cannot be inside a <noscript> in the head"},
		// "</noscript>" is "any other end tag", which does not reach past the
		// paragraph: a <p>'s end tag may not be left out inside a <noscript>.
		{"a paragraph left open in a noscript", `<p>a</p><noscript><p>b</noscript>c`,
			`<p>('a')<noscript>(<p>('bc'))`, "cannot close the <noscript> outside the <p>"},
	} {
		doc, errs, ok := Parse(tc.src)
		if ok {
			t.Errorf("%s: %q was accepted", tc.what, tc.src)
		}
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, tc.message) {
				found = true
				if e.Unsupported || e.Limit {
					t.Errorf("%s: %q is marked as something other than malformed markup",
						tc.what, e.Message)
				}
			}
		}
		if !found {
			t.Errorf("%s: nothing said %q: %v", tc.what, tc.message, errs)
		}
		if got := bodyShape(t, doc); got != tc.want {
			t.Errorf("%s: %q\n got %s\nwant %s", tc.what, tc.src, got, tc.want)
		}
	}
}

// TestADroppedElementStillEndsTheParagraph. What a tag ends it ends whether or
// not the element is kept: a <details> is refused, and the text after it is
// still not in the paragraph it ended.
func TestADroppedElementStillEndsTheParagraph(t *testing.T) {
	doc, _, _ := Parse(`<p>a<details>x</details>b`)
	if got := bodyShape(t, doc); got != `<p>('a')'b'` {
		t.Errorf("the body is %s", got)
	}
}

// TestAHeadNoscriptKeepsWhatItMayHold. Only a noscript in the head is limited:
// in the body it is an ordinary element and holds anything.
func TestAHeadNoscriptKeepsWhatItMayHold(t *testing.T) {
	doc, errs, _ := Parse(`<head><noscript><link href=a.css rel=stylesheet><p>x</p></noscript>`)
	if got := shapeOfTree(doc); got != `#doc(<html>(<head>(<noscript>(<link>))<body>(<p>('x'))))` {
		t.Errorf("the tree is %s", got)
	}
	// Two findings: the <p> that ends the <noscript>, and the "</noscript>" that
	// then has nothing to close.
	if len(errs) != 2 {
		t.Errorf("%d findings, want two: %v", len(errs), errs)
	}
}
