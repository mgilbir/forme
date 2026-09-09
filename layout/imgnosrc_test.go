package layout

import (
	"github.com/mgilbir/forme/style"
	"strings"
	"testing"
)

// An <img> that names no file.
//
// HTML says such an element represents nothing at all — there is no reference
// for a resolver to have refused, so nothing is missing and nothing is reported.
// It is still *replaced* nothing, and that is not a distinction without a
// difference: width and height do not apply to a non-replaced inline, so a
// document that sizes one gets the box it asked for only if the element is
// replaced; and a replaced element is content, so the white space either side of
// it does not collapse together across it.
//
// Its intrinsic dimensions are nought and *stated*, which is a third thing again
// from having none: the absence is §10.3.2's 300 by 150, and a box that size is
// the opposite of what an element representing nothing should draw.

// TestAnImgWithNoSrcTakesTheSizeCSSGivesIt is the first half. A non-replaced
// inline ignores both properties, so this fails by a box of no width rather than
// by a box of the wrong one.
func TestAnImgWithNoSrcTakesTheSizeCSSGivesIt(t *testing.T) {
	root := replacedLayout(t, 500, `<div><img id="i"></div>`, noDefaults,
		`#i { width: 20px; height: 10px }`)
	w, h := contentSize(find(t, root, "i"))
	px(t, "the used width", w, 20)
	px(t, "the used height", h, 10)
}

// TestAnImgWithNoSrcAndNoSizeIsNothing is the other end of the same rule, and
// the reason the dimensions are stated rather than absent: an element that
// represents nothing draws nothing, where §10.3.2's default size would draw a
// three-hundred-pixel box.
func TestAnImgWithNoSrcAndNoSizeIsNothing(t *testing.T) {
	root := replacedLayout(t, 500, `<div><img id="i"></div>`, noDefaults)
	w, h := contentSize(find(t, root, "i"))
	px(t, "the used width", w, 0)
	px(t, "the used height", h, 0)
}

// TestAnImgWithNoSrcSeparatesTheSpacesAroundIt.
//
// §4.1.1 collapses a sequence of white space, and an inline element boundary is
// transparent to it: "a <span></span> b" is one space. A replaced element is not
// a boundary but content, so the two spaces around it are two sequences and both
// survive — which is what the reference for text-wrap-balance-word-spacing-001
// is built on.
//
// Asserted against an inline-block of the same width rather than against
// arithmetic. An inline-block is unarguably content, so the two lines are the
// same line if the image is content too — and stating it that way avoids
// asserting a sum of separately quantized advances, which is a sixty-fourth of
// a pixel away from the sum of the same advances measured together.
func TestAnImgWithNoSrcSeparatesTheSpacesAroundIt(t *testing.T) {
	const css = `#d { font-family: Courier; font-size: 16px }
		#i { width: 20px; height: 10px }
		.box { display: inline-block; width: 20px; height: 10px }`

	img := reachOfLine(t, `<div id="d">aa <img id="i"> bb</div>`, css)
	box := reachOfLine(t, `<div id="d">aa <span class="box"></span> bb</div>`, css)
	if img != box {
		t.Errorf("with an <img> the line reaches %v and with an inline-block of "+
			"the same width %v; an image naming no file is still content, and "+
			"content is what keeps the space either side of it from collapsing "+
			"into one", img, box)
	}

	// And the collapsed reading is a different number, so the comparison above
	// is not two ways of writing the same thing.
	collapsed := reachOfLine(t, `<div id="d">aa  bb</div>`, css)
	if img.Sub(collapsed) <= 0 {
		t.Fatalf("the line with the image reaches %v and the same text with the "+
			"image taken out reaches %v, so this test cannot tell the two "+
			"readings apart", img, collapsed)
	}
}

// TestAnImgWithNoSrcAndAltTextIsItsText is the case that is not this rule.
//
// HTML says what an image that cannot be shown contains is its alt text, and CSS
// says an element whose replaced content is unavailable is not a replaced
// element at all. An <img> carrying alt text is that element whether or not it
// named a file, so it keeps the answer it had.
func TestAnImgWithNoSrcAndAltTextIsItsText(t *testing.T) {
	built := Build(Input{HTML: `<img id="i" alt="a caption">`})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	b := findBox(t, built.Root, "i")
	if b.Replaced != nil {
		t.Error("an <img> with alt text was made a replaced element; what it " +
			"contains is the text, and a replaced box has no children to put it in")
	}
	if got := textOfTree(built.Root); !strings.Contains(got, "a caption") {
		t.Errorf("the alt text is not on the page: %q", got)
	}
}

// reachOfLine lays out one div and returns how far its line reaches.
func reachOfLine(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	root := replacedLayout(t, 5000, markup, noDefaults, css)
	var end style.Unit
	for _, line := range find(t, root, "d").Lines {
		for _, r := range line.Runs {
			if e := r.X.Add(r.Width); e > end {
				end = e
			}
		}
	}
	return end
}
