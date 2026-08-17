package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// An SVG, read as a size and a colour.
//
// The suite's SVGs exist to test the *sizing* rules around them — CSS 2.1
// §10.3.2 asks what an element does with an intrinsic width but no height, with
// a ratio but no dimensions, with neither — and each is a solid swatch so that a
// reference can be written as a coloured box. Reading them gets both halves:
// the box comes out the right size, and what fills it is the colour.
//
// Most of what follows is about what must be *refused*. This is not a renderer,
// and an SVG reduced wrongly paints a colour where a browser paints a picture,
// which is worse than the hole it replaces because a hole is visible.

func svgOf(t *testing.T, body string) *ReplacedContent {
	t.Helper()
	return svgContent([]byte(body))
}

// mapResolver serves resources from memory, which is what an SVG fixture wants:
// the bytes are the test.
type mapResolver map[string][]byte

func (m mapResolver) Resolve(ref string) ([]byte, error) {
	if b, ok := m[ref]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("no such resource %q", ref)
}

// paintWith lays a document out against a resolver and paints it.
func paintWith(t *testing.T, res ResourceResolver, htmlSrc string, cssSrc ...string) []Op {
	t.Helper()
	in := Input{HTML: htmlSrc, Resources: res}
	for _, c := range cssSrc {
		in.CSS = append(in.CSS, Stylesheet{Source: c})
	}
	built := Build(in)
	if built.Root == nil {
		t.Fatalf("the document produced no boxes")
	}
	w, _ := style.FromPx(A4.Content().W.Px())
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	if frag == nil {
		t.Fatal("layout produced no fragment")
	}
	return Paint(frag)
}

// TestAnSVGsIntrinsicDimensions walks the cases CSS 2.1 §10.3.2 distinguishes,
// which is the whole reason the suite's fixtures are shaped the way they are.
func TestAnSVGsIntrinsicDimensions(t *testing.T) {
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	for _, tc := range []struct {
		what   string
		attrs  string
		w, h   style.Unit
		ratio  float64
		refuse bool
	}{
		{what: "width and height", attrs: `width="50" height="25"`, w: u(50), h: u(25), ratio: 2},
		{what: "width only", attrs: `width="60"`, w: u(60)},
		{what: "height only", attrs: `height="25"`, h: u(25)},
		// A viewBox is a ratio and nothing else: it says the shape of the
		// picture, not how large it is.
		{what: "viewBox only", attrs: `viewBox="0 0 1000 500"`, ratio: 2},
		{what: "height and a viewBox", attrs: `height="25" viewBox="0 0 1000 500"`, h: u(25), ratio: 2},
		// The two dimensions win over the viewBox when both are there, which is
		// what preserveAspectRatio="none" in the suite's fixtures is asking for.
		{what: "both, against a disagreeing viewBox",
			attrs: `width="50" height="25" viewBox="0 0 1000 1000"`, w: u(50), h: u(25), ratio: 2},
		{what: "neither", attrs: ``},
		// A percentage is a proportion of something the image does not know, so
		// it is not an intrinsic dimension at all.
		{what: "a percentage width", attrs: `width="100%"`},
		{what: "a viewBox with no area", attrs: `viewBox="0 0 0 500"`},
		{what: "a viewBox of three numbers", attrs: `viewBox="0 0 100"`},
	} {
		got := svgOf(t, `<svg xmlns="http://www.w3.org/2000/svg" `+tc.attrs+`>`+
			`<rect width="100%" height="100%" fill="green"/></svg>`)
		if got == nil {
			t.Errorf("%s: refused", tc.what)
			continue
		}
		if got.Width != tc.w || got.Height != tc.h {
			t.Errorf("%s: %v by %v, want %v by %v", tc.what, got.Width, got.Height, tc.w, tc.h)
		}
		if got.Ratio != tc.ratio {
			t.Errorf("%s: ratio %v, want %v", tc.what, got.Ratio, tc.ratio)
		}
	}
}

// TestAnSVGsColour reads the fill, including the one nobody wrote.
func TestAnSVGsColour(t *testing.T) {
	for _, tc := range []struct {
		fill string
		want style.RGBA
	}{
		{`fill="green"`, style.RGBA{G: 128, A: 1}},
		{`fill="#008000"`, style.RGBA{G: 128, A: 1}},
		{`fill="rgb(0,128,0)"`, style.RGBA{G: 128, A: 1}},
		{`fill="black"`, style.RGBA{A: 1}},
		// SVG's initial fill is black. Absent is not "no colour".
		{``, style.RGBA{A: 1}},
	} {
		got := svgOf(t, `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">`+
			`<rect width="100%" height="100%" `+tc.fill+`/></svg>`)
		if got == nil || got.Solid == nil {
			t.Errorf("%q: no colour", tc.fill)
			continue
		}
		if *got.Solid != tc.want {
			t.Errorf("%q: %v, want %v", tc.fill, *got.Solid, tc.want)
		}
	}
}

// TestTheSVGsThisEngineRefuses is the containment argument, and it is most of
// the value of this file.
//
// Everything here draws something a fill cannot express. Reducing any of it to a
// colour would paint a flat rectangle where a browser paints a picture — and
// unlike the hole that was there before, a wrong colour looks deliberate.
func TestTheSVGsThisEngineRefuses(t *testing.T) {
	const open = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">`
	for what, body := range map[string]string{
		"a circle":              open + `<circle cx="50" cy="50" r="40" fill="green"/></svg>`,
		"a path":                open + `<path d="M0 0 L100 100" fill="green"/></svg>`,
		"text":                  open + `<text x="0" y="20">hello</text></svg>`,
		"two rects":             open + `<rect width="100%" height="100%" fill="green"/><rect width="10" height="10" fill="red"/></svg>`,
		"a rect and a circle":   open + `<rect width="100%" height="100%" fill="green"/><circle r="4"/></svg>`,
		"a group":               open + `<g><rect width="100%" height="100%" fill="green"/></g></svg>`,
		"a stylesheet":          open + `<style>rect{fill:red}</style><rect width="100%" height="100%" fill="green"/></svg>`,
		"a script":              open + `<script>x</script><rect width="100%" height="100%" fill="green"/></svg>`,
		"a use":                 open + `<use href="#x"/></svg>`,
		"nothing drawn at all":  open + `</svg>`,
		"a rect away from 0":    open + `<rect x="10" width="100%" height="100%" fill="green"/></svg>`,
		"a rect that is short":  open + `<rect width="50" height="100" fill="green"/></svg>`,
		"a rounded rect":        open + `<rect width="100%" height="100%" rx="4" fill="green"/></svg>`,
		"a transformed rect":    open + `<rect width="100%" height="100%" transform="rotate(4)" fill="green"/></svg>`,
		"a translucent rect":    open + `<rect width="100%" height="100%" opacity="0.5" fill="green"/></svg>`,
		"a stroked rect":        open + `<rect width="100%" height="100%" stroke="red" fill="green"/></svg>`,
		"a styled rect":         open + `<rect width="100%" height="100%" style="fill:red" fill="green"/></svg>`,
		"an invisible rect":     open + `<rect width="100%" height="100%" fill="none"/></svg>`,
		"a gradient fill":       open + `<rect width="100%" height="100%" fill="url(#g)"/></svg>`,
		"a fill it cannot read": open + `<rect width="100%" height="100%" fill="notacolour"/></svg>`,
		"not an SVG at all":     `<html><body>x</body></html>`,
		"not markup at all":     "\x89PNG\r\n\x1a\n",
		"empty":                 ``,
		// A rect sized in user units against a viewBox it does not fill.
		"a rect short of its viewBox": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` +
			`<rect width="50" height="100" fill="green"/></svg>`,
	} {
		if got := svgContent([]byte(body)); got != nil {
			t.Errorf("%s: reduced to %v, and it draws something a fill cannot express",
				what, got.Solid)
		}
	}
}

// TestARectCoveringItsViewBoxInUserUnits: the suite writes its rects in the
// viewBox's coordinates rather than as percentages, so "width=1000" against
// "viewBox=0 0 1000 500" covers and has to be seen to.
func TestARectCoveringItsViewBoxInUserUnits(t *testing.T) {
	got := svgOf(t, `<svg xmlns="http://www.w3.org/2000/svg" version="1.1" `+
		`viewBox="0 0 1000 500" preserveAspectRatio="none">`+
		`<rect fill="gray" x="0" y="0" width="1000" height="500" /></svg>`)
	if got == nil {
		t.Fatal("a rect covering its viewBox in user units was refused")
	}
	if got.Ratio != 2 {
		t.Errorf("ratio %v, want 2 from the viewBox", got.Ratio)
	}
	if got.Solid == nil || *got.Solid != (style.RGBA{R: 128, G: 128, B: 128, A: 1}) {
		t.Errorf("colour %v, want gray", got.Solid)
	}
}

// TestASolidSVGIsBoundedByWhatItDeclares. The byte cap and the element cap are
// the two places an SVG could make the *parse* the attack rather than the page.
func TestASolidSVGIsBoundedByWhatItDeclares(t *testing.T) {
	big := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
		strings.Repeat(" ", maxSVGBytes) +
		`<rect width="100%" height="100%" fill="green"/></svg>`
	if svgContent([]byte(big)) != nil {
		t.Errorf("an SVG past the byte cap was read")
	}
	var many strings.Builder
	many.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">`)
	for i := 0; i < maxSVGElements+10; i++ {
		fmt.Fprintf(&many, "<desc>%d</desc>", i)
	}
	many.WriteString(`<rect width="100%" height="100%" fill="green"/></svg>`)
	if svgContent([]byte(many.String())) != nil {
		t.Errorf("an SVG past the element cap was read")
	}
}

// TestAnImageOfASolidSVGPaintsWhatABoxPaints is the join, and the same equality
// the gradient work rests on: two documents that put the same colour in the same
// place must build the same display list, because that is what the reftest
// comparing them reads.
//
// The suite's references for these tests are literally that — an <img> of a
// green SVG on one side, a <span> with a green background on the other.
func TestAnImageOfASolidSVGPaintsWhatABoxPaints(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="50" height="25">` +
		`<rect width="100%" height="100%" fill="rgb(0,128,0)"/></svg>`
	res := mapResolver{"s.svg": []byte(svg)}

	img := paintWith(t, res, `<img src="s.svg">`, `img { display: block }`)
	box := paintWith(t, res, `<div id="d"></div>`,
		`#d { display: block; width: 50px; height: 25px; background: rgb(0,128,0) }`)

	g, c := fillsOf(img, green), fillsOf(box, green)
	if len(c) != 1 {
		t.Fatalf("the reference fixture painted %d fills, want 1", len(c))
	}
	if len(g) != 1 {
		t.Fatalf("the SVG painted %d fills, want 1: %v", len(g), g)
	}
	if g[0] != c[0] {
		t.Errorf("the SVG painted %v and the box painted %v; the two put the same "+
			"colour in the same place and must draw the same thing", g[0], c[0])
	}
}

// TestAnSVGThisCannotReduceIsStillReported. The other half of the line: an SVG
// that draws a picture must keep its finding and paint nothing, exactly as
// before.
func TestAnSVGThisCannotReduceIsStillReported(t *testing.T) {
	res := mapResolver{"c.svg": []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
			`<circle cx="5" cy="5" r="4" fill="green"/></svg>`)}
	built := Build(Input{HTML: `<img src="c.svg">`, Resources: res})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	for _, op := range Paint(frag) {
		if r, ok := op.(FillRect); ok && r.Color == green {
			t.Errorf("an SVG this engine cannot render painted green")
		}
	}
	findings := append(built.Findings, rec.Findings()...)
	if !hasRule(findings, RuleImageUndecodable) {
		t.Errorf("an SVG this engine cannot reduce was not reported: %v", findings)
	}
}
