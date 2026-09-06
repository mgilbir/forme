package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestLayoutTakesANilRecorder is a contract that held for some documents.
//
// PaintReporting beside it has always taken a nil recorder, and this took one
// for as long as the document said nothing worth reporting — the first family
// it could not resolve dereferenced it. So whether a caller's own nil was fatal
// depended on the document, which is the worst way for it to depend on
// anything: it works in development and crashes on a page from a customer.
func TestLayoutTakesANilRecorder(t *testing.T) {
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	for _, tc := range []struct{ name, html, css string }{
		{"plain text", `<p>hello</p>`, ""},
		{"a family nobody has", `<p id="p">hello</p>`, `#p { font-family: Nonesuch }`},
		{"a glyph no face has", `<p id="p">שלום</p>`, `#p { font-family: Helvetica }`},
		{"a script this engine reports", `<p id="p">مرحبا</p>`, `#p { font-family: Helvetica }`},
		{"an image that cannot be loaded", `<img src="nope.png">`, ""},
		{"a value that is not implemented", `<p id="p">x</p>`, `#p { mix-blend-mode: multiply }`},
	} {
		in := Input{HTML: tc.html}
		if tc.css != "" {
			in.CSS = []Stylesheet{{Source: tc.css}}
		}
		built := Build(in)
		if built.Root == nil {
			t.Errorf("%s: no boxes", tc.name)
			continue
		}
		// The whole point: no recorder, and no panic.
		if Layout(built.Root, Size{W: w, H: h}, built.Fonts, nil) == nil {
			t.Errorf("%s: no fragments", tc.name)
		}
	}
}

// TestPaintingTakesANilRecorderToo pins the half that already worked, so that a
// change to one of the two is a change to both.
func TestPaintingTakesANilRecorderToo(t *testing.T) {
	built := Build(Input{HTML: `<p id="p">hello</p>`,
		CSS: []Stylesheet{{Source: `#p { font-family: Nonesuch }`}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, nil)
	if PaintReporting(frag, nil) == nil {
		t.Error("painting with no recorder produced no operations")
	}
}
