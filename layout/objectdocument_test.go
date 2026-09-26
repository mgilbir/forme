package layout

import (
	"strings"
	"testing"
)

// innerPage is the suite's support/root-canvas-001a.html, cut to what makes it
// what it is: an XHTML page, doctype first.
const innerPage = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
 <head><style type="text/css">html, body { background: red; } p { background: green; }</style></head>
 <body><p>This square must be green.</p></body>
</html>`

const smallSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"></svg>`

// TestAnObjectNamingADocumentSaysSo is CSS2/box-display/root-canvas-001: an
// <object type="text/html"> whose data is a page.
//
// Showing it needs a browsing context of its own, which this engine does not
// create — an <iframe>'s is refused the same way — so its fallback content is
// on the page and the finding says why. It said "image: unknown format", under
// image-undecodable, because the resource went to the image decoder with every
// other: true of no HTML file, and an author reading it looks for a broken
// picture that is not there.
func TestAnObjectNamingADocumentSaysSo(t *testing.T) {
	for _, tc := range []struct {
		what, html, doc string
	}{
		{
			what: "root-canvas-001: typed text/html",
			html: `<div><object type="text/html" data="support/root-canvas-001a.html">FAIL</object></div>`,
			doc:  "text/html",
		},
		{
			// No type anywhere: the bytes sniff as HTML, by the doctype.
			what: "untyped, sniffed as HTML",
			html: `<div><object data="support/root-canvas-001a.html">FAIL</object></div>`,
			doc:  "text/html",
		},
		{
			// An XML type other than SVG's is a document too.
			what: "typed application/xhtml+xml",
			html: `<div><object type="application/xhtml+xml" data="support/root-canvas-001a.html">FAIL</object></div>`,
			doc:  "application/xhtml+xml",
		},
		{
			// A data: URL's declared type is the Content-Type, and outranks the
			// attribute.
			what: "a data: URL declaring text/html",
			html: `<div><object type="image/png" data="data:text/html,<p>page</p>">FAIL</object></div>`,
			doc:  "text/html",
		},
	} {
		built := Build(Input{
			HTML:      tc.html,
			Resources: mapResolver{"support/root-canvas-001a.html": []byte(innerPage)},
		})
		box := boxFor(built.Root, "object")
		if box == nil {
			t.Fatalf("%s: the object generated no box: %v", tc.what, built.Findings)
		}
		if box.Replaced != nil {
			t.Errorf("%s: a page was drawn as a picture", tc.what)
		}
		if got := textIn(box); got != "FAIL" {
			t.Errorf("%s: the fallback content is %q, want %q", tc.what, got, "FAIL")
		}
		for _, f := range findingsWith(built.Findings, RuleImageUndecodable) {
			t.Errorf("%s: a document was reported as an undecodable image: %s", tc.what, f.Error())
		}
		blocked := findingsWith(built.Findings, RuleResourceBlocked)
		if len(blocked) != 1 {
			t.Fatalf("%s: %d blocked-resource findings, want 1: %v", tc.what, len(blocked), built.Findings)
		}
		msg := blocked[0].Message
		for _, want := range []string{
			"is a " + tc.doc + " document",
			"this engine creates no nested browsing context",
			"fallback content was laid out in its place",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: the finding %q does not say %q", tc.what, msg, want)
			}
		}
	}
}

// TestAnObjectThatIsNotADocumentIsReadAsBefore is the other side: pictures stay
// pictures, and bytes that are neither stay the decoder's to refuse.
func TestAnObjectThatIsNotADocumentIsReadAsBefore(t *testing.T) {
	res := mapResolver{
		"chart.svg":    []byte(smallSVG),
		"licensed.svg": []byte("<!-- a licence, as SVGs open with -->\n" + smallSVG),
		"page.html":    []byte(innerPage),
		"app.wasm":     []byte("\x00asm not a picture"),
	}
	for _, tc := range []struct {
		what, html string
		picture    bool
		rule       Rule
	}{
		{what: "an untyped SVG", html: `<object data="chart.svg">f</object>`, picture: true},
		{
			// The sniffing table's "<!--" is an HTML pattern, and an SVG with a
			// leading comment is not a page.
			what: "an untyped SVG opening with a comment", picture: true,
			html: `<object data="licensed.svg">f</object>`,
		},
		{what: "an SVG typed as SVG", html: `<object type="image/svg+xml" data="chart.svg">f</object>`, picture: true},
		{
			what: "an SVG under a generic XML type", picture: true,
			html: `<object type="application/xml" data="chart.svg">f</object>`,
		},
		{what: "an SVG in a data: URL", html: `<object data='data:image/svg+xml,` + smallSVG + `'>f</object>`, picture: true},
		{
			// With no Content-Type the attribute is the type, and it says
			// image: the bytes are not asked, and the decoder refuses them.
			what: "a page typed as a picture",
			html: `<object type="image/png" data="page.html">f</object>`,
			rule: RuleImageUndecodable,
		},
		{what: "bytes that are neither", html: `<object data="app.wasm">f</object>`, rule: RuleImageUndecodable},
	} {
		built := Build(Input{HTML: tc.html, Resources: res})
		box := boxFor(built.Root, "object")
		if box == nil {
			t.Fatalf("%s: the object generated no box: %v", tc.what, built.Findings)
		}
		if got := box.Replaced != nil; got != tc.picture {
			t.Errorf("%s: replaced is %v, want %v: %v", tc.what, got, tc.picture, built.Findings)
		}
		for _, f := range built.Findings {
			if strings.Contains(f.Message, "browsing context") {
				t.Errorf("%s: reported as a document: %s", tc.what, f.Error())
			}
		}
		if tc.rule != "" && len(findingsWith(built.Findings, tc.rule)) == 0 {
			t.Errorf("%s: no %s finding: %v", tc.what, tc.rule, built.Findings)
		}
	}
}

// TestTheTypeAttributeIsPartOfTheReading: one file named by two objects under
// two types is two readings, and the memo must not answer the second with the
// first. The same SVG is a page under text/html and a picture under its own
// type.
func TestTheTypeAttributeIsPartOfTheReading(t *testing.T) {
	built := Build(Input{
		HTML: `<object id=a type="text/html" data="chart.svg">first</object>` +
			`<object id=b type="image/svg+xml" data="chart.svg">second</object>`,
		Resources: mapResolver{"chart.svg": []byte(smallSVG)},
	})
	var objects []*Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b == nil {
			return
		}
		if b.Element != nil && b.Element.Name == "object" {
			objects = append(objects, b)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if len(objects) != 2 {
		t.Fatalf("%d object boxes, want 2", len(objects))
	}
	if objects[0].Replaced != nil {
		t.Error("the object typed text/html was drawn as a picture")
	}
	if objects[1].Replaced == nil {
		t.Errorf("the object typed image/svg+xml was not drawn: %v", built.Findings)
	}
}

// TestSniffingForHTML pins the MIME Sniffing standard's HTML patterns: case
// folded, after leading white space, and each ending on a space or a ">" —
// which is what keeps "<abbr>" and "<pre>" from being "<a" and "<p".
func TestSniffingForHTML(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"<!DOCTYPE html PUBLIC", true},
		{"<!doctype HTML>", true},
		{" \t\r\n\f<html>", true},
		{"<P class=x>", true},
		{"<a href=x>", true},
		{"<!-- a comment -->", true},
		{"<abbr>", false},
		{"<pre>", false},
		{"<!DOCTYPE htmlx>", false},
		{"<html", false},              // no byte after the pattern to terminate it
		{"\xef\xbb\xbf<html>", false}, // a byte order mark is not white space
		{"x<html>", false},
		{strings.Repeat(" ", 1445) + "<html>", false}, // past the resource header
		{strings.Repeat(" ", 1438) + "<html>", true},
	} {
		if got := sniffsAsHTML([]byte(tc.in)); got != tc.want {
			t.Errorf("sniffsAsHTML(%.40q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
