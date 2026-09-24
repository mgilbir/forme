package style

import "testing"

// Names an author writes in a selector are matched as the document's language
// matches them: ASCII case-insensitively on an HTML element in an HTML
// document, and exactly everywhere else — in XHTML, whose names are XML's, and
// on an <svg> in HTML once the tree builder has given its names their SVG case
// back. See html.Node.AttrNamed.

const xhtmlOpen = `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body>`

func TestATypeSelectorIsExactInXHTML(t *testing.T) {
	red := func(markup, css string) string {
		return computed(t, markup, author(t, css))["x"].Get("color")
	}
	if got := red(xhtmlOpen+`<p id="x">a</p></body></html>`, `p { color: red }`); got != "red" {
		t.Fatalf("p did not select <p> in XHTML: %q", got)
	}
	if got := red(xhtmlOpen+`<p id="x">a</p></body></html>`, `P { color: red }`); got == "red" {
		t.Error("P selected <p> in XHTML")
	}
	if got := red(xhtmlOpen+`<P id="x">a</P></body></html>`, `p { color: red }`); got == "red" {
		t.Error("p selected <P> in XHTML")
	}
	if got := red(`<p id="x">a</p>`, `P { color: red }`); got != "red" {
		t.Errorf("P did not select <p> in HTML: %q", got)
	}
}

func TestAnAttributeSelectorNameIsExactOutsideHTML(t *testing.T) {
	red := func(markup, css string) bool {
		return computed(t, markup, author(t, css))["x"].Get("color") == "red"
	}
	for _, tc := range []struct {
		markup, css string
		want        bool
		what        string
	}{
		{`<p id="x" LANG="tr">a</p>`, `[lang] { color: red }`, true, "HTML folds the name"},
		{`<p id="x" lang="tr">a</p>`, `[LANG] { color: red }`, true, "and the selector's"},
		{xhtmlOpen + `<p id="x" LANG="tr">a</p></body></html>`, `[LANG] { color: red }`, true, "XHTML: as written"},
		{xhtmlOpen + `<p id="x" LANG="tr">a</p></body></html>`, `[lang] { color: red }`, false, "XHTML: LANG is not lang"},
		{xhtmlOpen + `<p id="x" lang="tr">a</p></body></html>`, `[LANG] { color: red }`, false, "XHTML: lang is not LANG"},
		{`<svg id="x" VIEWBOX="0 0 1 1"></svg>`, `[viewBox] { color: red }`, true, "an <svg> in HTML has viewBox"},
		{`<svg id="x" viewbox="0 0 1 1"></svg>`, `[viewbox] { color: red }`, false, "and not viewbox"},
		// HTML lists type's value as case-insensitive; the list is keyed by the
		// attribute, however the selector spells its name.
		{`<ol id="x" type="A"></ol>`, `[TYPE=a] { color: red }`, true, "the folded-value list, by a capitalised name"},
		{xhtmlOpen + `<ol id="x" type="A"></ol></body></html>`, `[type=a] { color: red }`, false, "and not in XHTML"},
	} {
		if got := red(tc.markup, tc.css); got != tc.want {
			t.Errorf("%s: %s on %s matched = %v, want %v", tc.what, tc.css, tc.markup, got, tc.want)
		}
	}
}
