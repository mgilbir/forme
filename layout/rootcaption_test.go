package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A root element made a table, with a caption generated in front of it.
//
// §2.11.2 propagates the root element's background to the canvas, and a root
// declaring "display: table" is inside the anonymous wrapper §17.4 puts round
// every table, so the search for the root element looks through the wrapper.
// It took the first child with an element behind it — and an "html::before"
// made a caption is that child, because a pseudo-element carries its
// originating element. The caption's background became the page's, and the
// caption itself painted none. root-box-002 is the suite's document for it.

var (
	rootCaptionBlue = style.RGBA{B: 255, A: 1}
	rootCaptionRed  = style.RGBA{R: 255, A: 1}
)

const rootCaptionCSS = noDefaults + `
	html { display: table }
	html::before { content: "x"; display: table-caption; height: 20px;
	               background: rgb(0,0,255) }
	body { display: table-row }
	div { display: table-cell; width: 100px; height: 50px }`

// TestAGeneratedCaptionIsNotTheRootElement: the caption's blue is the caption's,
// painted at the caption's size, and the page has no background because the
// root element declares none.
func TestAGeneratedCaptionIsNotTheRootElement(t *testing.T) {
	ops := bgPaintOf(t, `<div></div>`, rootCaptionCSS)
	blue := fillsOfColour(ops, rootCaptionBlue)
	if len(blue) != 1 {
		t.Fatalf("%d blue fills, want the caption's one", len(blue))
	}
	if got := blue[0].Rect; got.W != bgpx(100) || got.H != bgpx(20) {
		t.Errorf("the caption's background is %v, want its own 100x20 box — "+
			"it was taken for the root element's and spread over the canvas", got)
	}
}

// TestTheRootTableStillGivesTheCanvasItsBackground is the other half: the
// root's own background is still found through the wrapper, past the caption,
// and still covers the page.
func TestTheRootTableStillGivesTheCanvasItsBackground(t *testing.T) {
	ops := bgPaintOf(t, `<div></div>`,
		rootCaptionCSS+` html { background: rgb(255,0,0) }`)
	red := fillsOfColour(ops, rootCaptionRed)
	if len(red) != 1 || red[0].Rect.W != bgpx(500) || red[0].Rect.H != bgpx(400) {
		t.Fatalf("the root's red is painted as %v, want once over the 500x400 canvas", red)
	}
	if blue := fillsOfColour(ops, rootCaptionBlue); len(blue) != 1 || blue[0].Rect.W != bgpx(100) {
		t.Errorf("the caption's own background is %v, want one 100px fill", blue)
	}
}
