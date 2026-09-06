package html

import (
	"strings"
	"testing"
)

// shapeOfTree spells a tree's element structure and its text, which is what
// these tests compare.
func shapeOfTree(n *Node) string {
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		switch n.Type {
		case ElementNode:
			b.WriteString("<" + n.Name + ">")
		case TextNode:
			if t := strings.TrimSpace(n.Text); t != "" {
				b.WriteString("'" + t + "'")
			}
		default:
			b.WriteString("#doc")
		}
		if len(n.Children) > 0 {
			b.WriteString("(")
			for _, c := range n.Children {
				walk(c)
			}
			b.WriteString(")")
		}
	}
	walk(n)
	return b.String()
}

// TestContentAfterTheBodyEndTagIsStillInTheBody is HTML's "after body" mode.
//
// A document does not end because a tag said so, and there is nowhere else for
// content to go. Popping <body> instead made what followed a *sibling* of it —
// and then </html> popped the last frame and sent the content back into the
// body, so the two tags together were a no-op and either one alone was not.
// Both readings put content somewhere no browser puts it, and neither was
// refused.
func TestContentAfterTheBodyEndTagIsStillInTheBody(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"after </body>", `<body><p>a</p></body><p>b</p>`},
		{"after </body></html>", `<body><p>a</p></body></html><p>b</p>`},
		{"after </html>", `<body><p>a</p></html><p>b</p>`},
		{"after both, twice", `<p>a</p></body></html></body></html><p>b</p>`},
	} {
		doc, _, _ := Parse(tc.src)
		const want = "#doc(<html>(<head><body>(<p>('a')<p>('b'))))"
		if got := shapeOfTree(doc); got != want {
			t.Errorf("%s: %s\n   want %s", tc.name, got, want)
		}
	}
}

// TestMetadataBeforeTheBodyKeepsItsPlaceInTheDocument is the ordering the
// cascade rests on.
//
// <body> is appended to <html> when the document is opened, so anything put
// directly into <html> afterwards lands *after* it. A <style> written between
// </head> and <body> therefore came after every stylesheet in the body, and won
// the cascade against all of them — the document said the opposite, and nothing
// was reported.
func TestMetadataBeforeTheBodyKeepsItsPlaceInTheDocument(t *testing.T) {
	doc, _, _ := Parse(`<head><title>t</title></head><style>red</style>` +
		`<body><style>blue</style><p>x</p></body>`)
	const want = "#doc(<html>(<head>(<title>('t')<style>('red'))<body>(<style>('blue')<p>('x'))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}

	// Which is to say: the sheets are met in the order they were written.
	var sheets []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode && n.Name == "style" {
			sheets = append(sheets, strings.TrimSpace(n.TextContent()))
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc)
	if strings.Join(sheets, ",") != "red,blue" {
		t.Errorf("the stylesheets are met in the order %v, want red then blue", sheets)
	}
}

// TestAStyleInTheBodyStaysInTheBody is the reason <style> is not simply moved
// into the head: a sheet written after the content it styles has to stay after
// it, or the order two declarations apply in changes.
func TestAStyleInTheBodyStaysInTheBody(t *testing.T) {
	doc, _, _ := Parse(`<p>x</p><style>late</style>`)
	const want = "#doc(<html>(<head><body>(<p>('x')<style>('late'))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}
}

// TestAnOrdinaryDocumentIsUnchanged is the control for both, since a change to
// where things are inserted touches every document there is.
func TestAnOrdinaryDocumentIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare paragraph", `<p>x</p>`, "#doc(<html>(<head><body>(<p>('x'))))"},
		{"a title and a paragraph", `<title>t</title><p>x</p>`,
			"#doc(<html>(<head>(<title>('t'))<body>(<p>('x'))))"},
		{"a meta before the body", `<meta charset=utf-8><p>x</p>`,
			"#doc(<html>(<head>(<meta>)<body>(<p>('x'))))"},
		{"an explicit head and body",
			`<html><head><title>t</title></head><body><p>x</p></body></html>`,
			"#doc(<html>(<head>(<title>('t'))<body>(<p>('x'))))"},
		{"nested elements", `<div><p>x</p></div>`,
			"#doc(<html>(<head><body>(<div>(<p>('x')))))"},
	} {
		doc, _, _ := Parse(tc.src)
		if got := shapeOfTree(doc); got != tc.want {
			t.Errorf("%s: %s\n   want %s", tc.name, got, tc.want)
		}
	}
}
