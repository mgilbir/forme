package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The cascade's font-size and the one the text is set at have to be the same
// number.
//
// They are computed in two places for a reason — the cascade absolutises every
// "em" in a document against it, and layout needs it again to choose a face and
// measure a run — and the two are kept in step by the cascade writing the
// answer back as an absolute length. Layout re-reads that length only for an
// element that *owns* a font-size, because re-resolving an inherited one would
// compound a relative value at every level of the tree.
//
// The monospace default is the case where an element owns a size nobody
// declared. Without saying so, the cascade resolved "19em" against thirteen
// pixels while layout set the element's text at sixteen, and a box nineteen
// lines tall held sixteen of them.

// fontSizeAndBox is the size a fixture's text is drawn at and the height of the
// box beside it, which is the same length expressed the two ways.
func fontSizeAndBox(t *testing.T, htmlSrc, cssSrc string) (style.Unit, style.Unit) {
	t.Helper()
	var size, height style.Unit
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		switch v := op.(type) {
		case DrawText:
			if size == 0 {
				size = v.Size
			}
		case FillRect:
			if height == 0 {
				height = v.Rect.H
			}
		}
	}
	return size, height
}

func TestTheTwoFontSizesAgreeOnAMonospaceDefault(t *testing.T) {
	// "10em" of the same element, once as a height and once as the size its own
	// text is set at.
	size, height := fontSizeAndBox(t,
		`<div id=box></div><div id=text>x</div>`,
		`#box { font-family: monospace; height: 10em; background: green }
		 #text { font-family: monospace }`)
	if size == 0 || height == 0 {
		t.Fatalf("the fixture drew size=%v height=%v, want both", size, height)
	}
	if got, want := height, size.Mul(10); got != want {
		t.Errorf("the box is %v tall and the text is set at %v, so ten ems of it "+
			"would be %v; the cascade and the layout are using different numbers",
			got, size, want)
	}
}

func TestTheTwoAgreeWithoutMonospaceToo(t *testing.T) {
	// The case that was always right, so that the test above is about the
	// monospace default rather than about the fixture.
	size, height := fontSizeAndBox(t,
		`<div id=box></div><div id=text>x</div>`,
		`#box { height: 10em; background: green } #text { color: black }`)
	if got, want := height, size.Mul(10); got != want {
		t.Errorf("the box is %v tall against %v for ten ems of the text's %v",
			got, want, size)
	}
}

func TestAChIsTheSizeTheTextIsSetIn(t *testing.T) {
	// The same disagreement seen through the other font-relative unit: "1ch" is
	// the advance of a zero in the element's own font, so a box one ch wide has
	// to hold exactly one character of the text beside it.
	var boxW, glyphW style.Unit
	for _, op := range paintOf(t,
		`<div id=box></div><div id=text>0</div>`,
		`#box { font-family: monospace; width: 1ch; height: 4px; background: green }
		 #text { font-family: monospace }`) {
		switch v := op.(type) {
		case FillRect:
			if boxW == 0 {
				boxW = v.Rect.W
			}
		case DrawText:
			if glyphW == 0 && v.Face != nil {
				w, _ := style.FromPx(v.Face.Measure("0", v.Size.Px()))
				glyphW = w
			}
		}
	}
	if boxW == 0 || glyphW == 0 {
		t.Fatalf("the fixture drew boxW=%v glyphW=%v", boxW, glyphW)
	}
	if boxW != glyphW {
		t.Errorf("a 1ch box is %v wide and the zero it is a ch of is %v", boxW, glyphW)
	}
}
