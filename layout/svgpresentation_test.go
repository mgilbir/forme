package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// SVG 2's presentation attributes, as far as a filled rectangle needs them.
//
// fill, stroke and visibility are inherited (SVG 2 §6.7 and the property table
// of §13): written on the root, they are every rect's unless the rect says
// otherwise. display is not inherited, and "none" takes an element and
// everything in it out of the rendering (§6.8's rendering tree). The reducer
// read none of them, so "<svg fill=red><rect/></svg>" came out black and a
// rect with display="none" or visibility="hidden" was painted — against the
// reducer's whole promise, which is "exact or refused" (audit C81).

func TestAnSVGsPresentationIsInherited(t *testing.T) {
	const size = `xmlns="http://www.w3.org/2000/svg" width="10" height="10"`
	const cover = `width="100%" height="100%"`
	red := style.RGBA{R: 255, A: 1}
	for _, tc := range []struct {
		name, body string
		want       *style.RGBA // nil: paints nothing, and is read
	}{
		{"a fill on the root", `<svg ` + size + ` fill="red"><rect ` + cover + `/></svg>`, &red},
		{"a fill on the rect wins", `<svg ` + size + ` fill="blue"><rect ` + cover + ` fill="red"/></svg>`, &red},
		{"inherit on the rect", `<svg ` + size + ` fill="red"><rect ` + cover + ` fill="inherit"/></svg>`, &red},
		{"none on the root", `<svg ` + size + ` fill="none"><rect ` + cover + `/></svg>`, nil},
		{"a rect not displayed", `<svg ` + size + `><rect ` + cover + ` display="none" fill="red"/></svg>`, nil},
		{"a rect hidden", `<svg ` + size + `><rect ` + cover + ` visibility="hidden" fill="red"/></svg>`, nil},
		{"a rect collapsed", `<svg ` + size + `><rect ` + cover + ` visibility="collapse" fill="red"/></svg>`, nil},
		{"a root hidden", `<svg ` + size + ` visibility="hidden"><rect ` + cover + ` fill="red"/></svg>`, nil},
		{"a root not displayed", `<svg ` + size + ` display="none"><rect ` + cover + ` fill="red"/></svg>`, nil},
		// visibility is inherited and may be overridden; display is not
		// inherited and a child's cannot bring back what the root took out.
		{"a rect visible under a hidden root",
			`<svg ` + size + ` visibility="hidden"><rect ` + cover + ` visibility="visible" fill="red"/></svg>`, &red},
		{"a rect displayed under a root not displayed",
			`<svg ` + size + ` display="none"><rect ` + cover + ` display="inline" fill="red"/></svg>`, nil},
		// A stroke of none is no stroke, wherever it is written.
		{"no stroke on the root", `<svg ` + size + ` stroke="none"><rect ` + cover + ` fill="red"/></svg>`, &red},
		// What changes nothing a filled rectangle paints.
		{"attributes that change nothing",
			`<svg ` + size + ` version="1.1" id="a" class="b" xml:space="preserve" ` +
				`xmlns:xlink="http://www.w3.org/1999/xlink" aria-hidden="true" role="img" ` +
				`font-family="serif" fill-rule="evenodd" stroke-width="4" opacity="1">` +
				`<rect ` + cover + ` fill="red" data-x="1" shape-rendering="crispEdges"/></svg>`, &red},
	} {
		got := svgContent([]byte(tc.body), svgAsImage)
		if got == nil {
			t.Errorf("%s: refused; it is a picture this can draw exactly", tc.name)
			continue
		}
		switch {
		case tc.want == nil && len(got.SVG.rects) != 0:
			t.Errorf("%s: painted %v; it paints nothing", tc.name, got.SVG.rects)
		case tc.want != nil && (got.Solid == nil || *got.Solid != *tc.want):
			t.Errorf("%s: painted %v, want %v", tc.name, got.Solid, *tc.want)
		}
	}
}

// TestAnSVGWhosePresentationThisCannotDrawIsRefused: a root that is
// translucent, transformed, styled or stroked changes every rect under it, and
// an attribute nobody classified is not known to change nothing.
func TestAnSVGWhosePresentationThisCannotDrawIsRefused(t *testing.T) {
	const size = `xmlns="http://www.w3.org/2000/svg" width="10" height="10"`
	const rect = `<rect width="100%" height="100%" fill="green"/>`
	for what, root := range map[string]string{
		"a translucent root":      `opacity="0.5"`,
		"a root fill-opacity":     `fill-opacity="0.5"`,
		"a transformed root":      `transform="scale(2)"`,
		"a styled root":           `style="fill: red"`,
		"a stroked root":          `stroke="red"`,
		"a filtered root":         `filter="url(#f)"`,
		"a clipped root":          `clip-path="url(#c)"`,
		"a root it cannot fill":   `fill="currentColor"`,
		"a root showing overflow": `overflow="visible"`,
		"a conditional root":      `systemLanguage="fr"`,
		"an unknown attribute":    `frobnicate="yes"`,
		"a bad visibility":        `visibility="sometimes"`,
	} {
		body := `<svg ` + size + ` ` + root + `>` + rect + `</svg>`
		if got := svgContent([]byte(body), svgAsImage); got != nil {
			t.Errorf("%s: reduced to %v", what, got.Solid)
		}
	}
	for what, body := range map[string]string{
		"an unknown attribute on a rect": `<svg ` + size + `><rect width="100%" height="100%" ` +
			`fill="green" frobnicate="yes"/></svg>`,
		"a conditional rect": `<svg ` + size + `><rect width="100%" height="100%" ` +
			`fill="green" requiredExtensions="x"/></svg>`,
		"a stroke inherited": `<svg ` + size + ` stroke="blue"><rect width="100%" height="100%" ` +
			`fill="green" stroke="inherit"/></svg>`,
		"a translucent rect": `<svg ` + size + `><rect width="100%" height="100%" ` +
			`fill="green" fill-opacity="50%"/></svg>`,
	} {
		if got := svgContent([]byte(body), svgAsImage); got != nil {
			t.Errorf("%s: reduced to %v", what, got.Solid)
		}
	}
}

// TestAnInlineSVGsCascadedAttributesAreNotReadTwice: on an <svg> written in
// the page, "style" and "hidden" belong to the page's own cascade, which has
// applied them to the element's box — they are not an SVG file's CSS.
func TestAnInlineSVGsCascadedAttributesAreNotReadTwice(t *testing.T) {
	built := Build(Input{HTML: `<svg id=s style="display: block" width="10" height="10">` +
		`<rect width="100%" height="100%" fill="green"/></svg>`})
	var found *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b.Element != nil && strings.EqualFold(b.Element.Name, "svg") {
			found = b
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if found == nil || found.Replaced == nil || found.Replaced.Solid == nil {
		t.Fatalf("an inline <svg> with a style attribute was not drawn; findings %v", built.Findings)
	}
}

// The replaced-content memo is keyed by everything a reading depends on
// (audit C83).
//
// The same SVG is a picture in an <img> and a document in an <object>, and an
// SVG stating no size is 300 by 150 as the first and as wide as its containing
// block as the second. The memo was keyed by the reference alone, so whichever
// came first decided the other's size.
func TestTheSameSVGIsReadAsEachElementReadsIt(t *testing.T) {
	res := mapResolver{"s.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg">` +
		`<rect width="100%" height="100%" fill="green"/></svg>`)}
	sizes := func(doc string) map[string][2]float64 {
		built := Build(Input{HTML: doc, Resources: res,
			CSS: []Stylesheet{{Source: `body { margin: 0; width: 600px } img, object { display: block }`}}})
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
		out := map[string][2]float64{}
		for _, id := range []string{"i", "o"} {
			f := find(t, frag, id)
			out[id] = [2]float64{f.BorderRect.W.Px(), f.BorderRect.H.Px()}
		}
		return out
	}
	img, obj := `<img id=i src="s.svg">`, `<object id=o data="s.svg"></object>`
	a, b := sizes(img+obj), sizes(obj+img)
	if a["i"] != b["i"] || a["o"] != b["o"] {
		t.Errorf("the sizes depend on which element came first: %v with the <img> first, "+
			"%v with the <object> first", a, b)
	}
	if a["i"] != [2]float64{300, 150} {
		t.Errorf("the <img> is %v, want CSS 2.1's 300 by 150 for a picture stating no size", a["i"])
	}
	if a["o"][0] != 600 {
		t.Errorf("the <object> is %v, want as wide as its containing block", a["o"])
	}
}

// SVG sniffing (audit C137).
//
// Bytes from a resolver come with no type, and the MIME Sniffing standard has
// no signature for an SVG — a browser knows one by the type it was served
// with. So what makes one is what makes a file an SVG at all: XML whose root is
// <svg>, after XML 1.0 §2.8's prolog, however long that is. It gave up after a
// kilobyte, so an SVG with a licence comment first was "unknown format".
func TestAnSVGIsKnownByItsRootAfterAnyProlog(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>`
	long := "<!-- " + strings.Repeat("licence text ", 400) + " -->"
	for _, tc := range []struct {
		name, data string
		want       bool
	}{
		{"bare", svg, true},
		{"a declaration", `<?xml version="1.0" encoding="UTF-8"?>` + svg, true},
		{"a byte order mark", "\xef\xbb\xbf" + svg, true},
		{"white space", " \n\t" + svg, true},
		{"a long comment", `<?xml version="1.0"?>` + "\n" + long + "\n" + svg, true},
		{"a doctype", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" ` +
			`"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">` + svg, true},
		{"a doctype with a subset", `<!DOCTYPE svg [ <!ENTITY a "<svg>]>"> <!-- ]> --> <?pi ]> ?> ]>` + svg, true},
		{"a processing instruction", `<?xml-stylesheet href="x"?>` + svg, true},
		{"everything", "\xef\xbb\xbf<?xml version='1.0'?><!-- a --><!DOCTYPE svg><!-- b -->" + svg, true},
		// And what is not one.
		{"another root", `<?xml version="1.0"?><html/>`, false},
		{"a longer name", `<svgx/>`, false},
		{"an unclosed comment", `<!-- ` + svg, false},
		{"two doctypes", `<!DOCTYPE svg><!DOCTYPE svg>` + svg, false},
		{"text first", `hello` + svg, false},
		{"a PNG", "\x89PNG\r\n\x1a\n<svg", false},
		{"the root in a comment only", `<!-- <svg> -->`, false},
	} {
		if got := looksLikeSVG([]byte(tc.data)); got != tc.want {
			t.Errorf("%s: looksLikeSVG = %v, want %v", tc.name, got, tc.want)
		}
	}
	// And end to end: a picture with a licence header longer than the old
	// window is drawn.
	res := mapResolver{"l.svg": []byte(`<?xml version="1.0"?>` + long +
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
		`<rect width="100%" height="100%" fill="green"/></svg>`)}
	built := Build(Input{HTML: `<img src="l.svg">`, Resources: res})
	if hasRule(built.Findings, RuleImageUndecodable) {
		t.Errorf("an SVG with a long comment first was not read: %v", built.Findings)
	}
}

// TestADataURLsTypeDecidesWhatItIs is MIME Sniffing §8.2 for an image: a
// supplied XML type is the type, and any other sends the bytes to the image
// signatures, which no SVG matches.
func TestADataURLsTypeDecidesWhatItIs(t *testing.T) {
	const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='10' height='10'>` +
		`<rect width='100%' height='100%' fill='green'/></svg>`
	for _, tc := range []struct {
		src  string
		read bool
	}{
		{"data:image/svg+xml," + svg, true},
		{"data:application/xml," + svg, true},
		{"data:text/xml;charset=utf-8," + svg, true},
		{"data:image/png," + svg, false},
		{"data:," + svg, false},
	} {
		built := Build(Input{HTML: `<img src="` + tc.src + `">`})
		if got := !hasRule(built.Findings, RuleImageUndecodable); got != tc.read {
			t.Errorf("%.30q…: read %v, want %v; findings %v", tc.src, got, tc.read, built.Findings)
		}
	}
}

// TestHTMLsRubyIsReportedAndItsDatalistHidden (audit C84): HTML's own ruby
// elements are given the display types HTML's rendering section gives them, so
// the report an author's "display: ruby" gets is theirs too; and a <datalist>,
// which §15.3.1 hides, draws nothing.
func TestHTMLsRubyIsReportedAndItsDatalistHidden(t *testing.T) {
	built := Build(Input{HTML: `<p><ruby>漢<rp>(</rp><rt>kan</rt><rp>)</rp></ruby></p>`})
	found := false
	for _, f := range built.Findings {
		if f.Property == "display" && strings.Contains(f.Message, "ruby") {
			found = true
		}
	}
	if !found {
		t.Errorf("a <ruby> with an annotation laid out inline was not reported: %v", built.Findings)
	}
	// <rp> is hidden, as §15.3.1 hides it.
	if text := boxText(built.Root); strings.Contains(text, "(") || strings.Contains(text, ")") {
		t.Errorf("the <rp> parentheses were drawn: %q", text)
	}
	// A ruby with no annotation is the box it asks for, and says nothing.
	if built := Build(Input{HTML: `<p><ruby>漢</ruby></p>`}); len(built.Findings) != 0 {
		t.Errorf("a <ruby> with nothing to lift was reported: %v", built.Findings)
	}

	for _, doc := range []string{
		`<p>a<datalist><option>O</option></datalist>b</p>`,
		`<p>a<template><b>O</b></template>b</p>`,
		`<p>a<noframes>O</noframes>b</p>`,
	} {
		built := Build(Input{HTML: doc})
		if text := boxText(built.Root); strings.Contains(text, "O") {
			t.Errorf("%s: drew %q", doc, text)
		}
	}
}

// boxText is the text of every box in a tree.
func boxText(b *Box) string {
	if b == nil {
		return ""
	}
	var s strings.Builder
	s.WriteString(b.Text)
	for _, c := range b.Children {
		s.WriteString(boxText(c))
	}
	return s.String()
}
