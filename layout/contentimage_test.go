package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A picture that is the whole of "content" makes the pseudo-element a replaced
// element; a picture among other content is a box of its own inside it.
//
// CSS 2.1 §12.2: "if the value is a URI, the pseudo-element is replaced by the
// object". css-content-3 says the same of a single <image> and only of a single
// one, and everything a stylesheet can do to the box follows from which of the
// two it is — a width or a height sizes the *picture* in the first case and the
// line's box in the second, and the border is drawn once either way.
//
// This engine built the picture as a child box carrying the pseudo-element's
// whole computed style, which is neither. The border was drawn twice — the
// pseudo-element's around the picture's, both padded — and a height stretched the
// picture whether or not the picture was all there was.
func TestAPictureIsTheWholeOfTheContentOrABoxInsideIt(t *testing.T) {
	res := mapResolver{"mark.png": encodePNG(t, 20, 10)}
	const face = ` p { font-family: Courier; font-size: 20px }`

	picture := func(css string) Rect {
		t.Helper()
		var drawn []Rect
		for _, op := range paintWith(t, res, `<p id=p>x</p>`, css+face) {
			if d, ok := op.(DrawImage); ok {
				drawn = append(drawn, d.Rect)
			}
		}
		if len(drawn) != 1 {
			t.Fatalf("%q drew %d pictures, want one", css, len(drawn))
		}
		return drawn[0]
	}

	for _, tc := range []struct {
		css  string
		w, h float64
		what string
	}{
		{`p::before { content: url(mark.png) }`, 20, 10,
			"a lone picture, at its own size"},
		{`p::before { content: url(mark.png); height: 40px }`, 80, 40,
			"a lone picture with a height, which sizes it and its ratio gives the width"},
		{`p::before { content: url(mark.png); width: 60px }`, 60, 30,
			"a lone picture with a width"},

		{`p::before { content: "A" url(mark.png) "B" }`, 20, 10,
			"a picture among words, at its own size"},
		{`p::before { content: "A" url(mark.png) "B"; height: 40px }`, 20, 10,
			"a height on the pseudo-element, which is not the picture's"},
		{`p::before { content: "A" url(mark.png) "B"; width: 60px }`, 20, 10,
			"a width on the pseudo-element, which is not the picture's"},
		{`p::before { content: "" url(mark.png) }`, 20, 10,
			"an empty string beside it, which is still not a lone picture"},
	} {
		got := picture(tc.css)
		w, _ := style.FromPx(tc.w)
		h, _ := style.FromPx(tc.h)
		if got.W != w || got.H != h {
			t.Errorf("%s: the picture is %v by %v, want %v by %v",
				tc.what, got.W, got.H, w, h)
		}
	}
}

// TestTheBorderOfAPseudoElementIsDrawnOnce is the other half of the same
// question, and the one a reader sees first.
//
// A pseudo-element is one box. Built as a box holding a box, both carrying the
// same computed style, its border was drawn twice and its padding counted twice
// — a 5px border around a 20 by 10 picture came out as two frames.
func TestTheBorderOfAPseudoElementIsDrawnOnce(t *testing.T) {
	res := mapResolver{"mark.png": encodePNG(t, 20, 10)}
	for _, tc := range []struct{ content, what string }{
		{`url(mark.png)`, "a lone picture"},
		{`"A" url(mark.png)`, "a picture after a word"},
		{`url(mark.png) "B"`, "a picture before a word"},
	} {
		ops := paintWith(t, res, `<p id=p>x</p>`,
			`p::before { content: `+tc.content+`; border: 5px solid red; padding: 10px }
			 p { font-family: Courier; font-size: 20px }`)
		red := 0
		for _, op := range ops {
			if f, ok := op.(FillRect); ok && f.Color.R == 255 && f.Color.G == 0 && f.Color.B == 0 {
				red++
			}
		}
		// Four edges, once.
		if red != 4 {
			t.Errorf("%s: %d red rectangles, want the four edges of one border",
				tc.what, red)
		}
	}
}
