package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// A <canvas> is a replaced element whose bitmap this engine never draws into.
//
// The two halves are separate, and only one of them is a limitation. The bitmap
// is painted by script and script is never run — so what a canvas shows here is
// what a canvas shows in a browser with scripting turned off: a blank one. The
// *box*, though, is not a guess: HTML §4.12.5 puts the bitmap's dimensions on
// the element's own width and height attributes and defaults them to 300 by
// 150, so the size is knowable from the markup alone.
//
// The element used to be refused for the first half, which threw away the
// second. These pin the second half, and the last of them pins that the first
// half is not reported as a loss — because on a page with no script there is
// nothing lost.

// TestACanvasIsABlankBitmapOfItsOwnSize is the default: HTML's 300 by 150,
// which is CSS 2.1 §10.3.2's default object size and the same two numbers for
// the same reason.
func TestACanvasIsABlankBitmapOfItsOwnSize(t *testing.T) {
	root := replacedLayout(t, 500, `<div><canvas id="c"></canvas></div>`, noDefaults)
	w, h := contentSize(find(t, root, "c"))
	px(t, "the used width", w, 300)
	px(t, "the used height", h, 150)
}

// TestACanvasTakesItsSizeFromItsAttributes is where the size comes from when
// the markup states one.
func TestACanvasTakesItsSizeFromItsAttributes(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div><canvas id="c" width="120" height="45"></canvas></div>`, noDefaults)
	w, h := contentSize(find(t, root, "c"))
	px(t, "the used width", w, 120)
	px(t, "the used height", h, 45)
}

// TestACanvasAttributeIsNotADimensionProperty is the distinction the whole
// feature turns on.
//
// HTML's *dimension attributes* — <img width>, <iframe width>, <table width> —
// map to the width and height properties, and style/hints.go has the table. A
// canvas's do not: they are the size of the bitmap, which is an *intrinsic*
// dimension, and the difference is only visible when CSS states the other axis.
//
// A ten-by-ten canvas told to be a hundred tall is a hundred wide, because a
// square bitmap has a ratio of one and §10.3.2 solves the auto width from it. If
// the attributes were dimension properties the same element would be ten wide,
// with the declared height beating the hinted one on the same property and the
// hinted width standing unopposed. The suite writes it as
// CSS2/normal-flow/intrinsic-size-with-anonymous-block.
func TestACanvasAttributeIsNotADimensionProperty(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div><canvas id="c" width="10" height="10"></canvas></div>`, noDefaults,
		`#c { height: 100px }`)
	w, h := contentSize(find(t, root, "c"))
	px(t, "the width its ratio gives it", w, 100)
	px(t, "the declared height", h, 100)
}

// TestACanvasDimensionIsReadTheWayHTMLReadsOne pins the attribute parser
// against HTML's *rules for parsing non-negative integers*, which are not
// strconv.Atoi in either direction: they skip leading white space and a leading
// plus, they stop at the first character that is not a digit and ignore the
// rest, and anything that yields no digits at all leaves the element with its
// default rather than with nothing.
//
// Against the reader rather than through a layout, because one of the answers
// is not observable in a box: a stated zero *is* a valid non-negative integer
// and the reader returns it, but ReplacedContent has no way to say "an
// intrinsic dimension of nought" — zero is how it spells "none" — so the sizing
// above falls through to §10.3.2's default. That divergence is real, it is
// bounded to a bitmap with no area, and canvas() says so; asserting the reader
// here is asserting the half that is right rather than dressing the other half
// up.
func TestACanvasDimensionIsReadTheWayHTMLReadsOne(t *testing.T) {
	for _, tc := range []struct {
		attr string
		want float64
		why  string
	}{
		{"80", 80, "an ordinary integer"},
		{" 80 ", 80, "leading and trailing white space, which HTML skips"},
		{"+80", 80, "a leading plus, which HTML allows"},
		{"80px", 80, "trailing text, which HTML stops at and ignores"},
		{"0", 0, "zero, which is a valid non-negative integer"},
		{"-80", 300, "a negative, which is not one, so the default"},
		{"wide", 300, "no digits at all, so the default"},
		{"", 300, "an empty attribute, so the default"},
		{"99999999999", 300, "more digits than a page can mean, so the default"},
	} {
		n := &html.Node{Type: html.ElementNode, Name: "canvas",
			Attrs: []html.Attribute{{Name: "width", Value: tc.attr}}}
		px(t, "width="+quoteValue(tc.attr)+" — "+tc.why,
			canvasDimension(n, "width", 300), tc.want)
	}
	// An attribute that is not there at all, which is a different branch from
	// one that is there and says nothing.
	px(t, "no width attribute at all",
		canvasDimension(&html.Node{Type: html.ElementNode, Name: "canvas"}, "width", 300), 300)
}

// TestACanvasFallbackContentIsNotRendered.
//
// A canvas's children are what a user agent that cannot draw one would show
// instead, and one that can never renders them — the same rule that takes an
// <object>'s children off it once its data is embedded. This engine *can* draw
// the canvas: a blank bitmap of the right size is what a canvas nobody scripted
// holds, so the fallback is not what a reader should see.
func TestACanvasFallbackContentIsNotRendered(t *testing.T) {
	built := Build(Input{HTML: `<canvas id="c" width="10" height="10">no canvas here</canvas>`})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	b := findBox(t, built.Root, "c")
	if b.Replaced == nil {
		t.Fatal("the canvas is not a replaced element")
	}
	if len(b.Children) != 0 {
		t.Errorf("the canvas kept %d child boxes; its fallback content is not "+
			"rendered by an engine that draws the bitmap", len(b.Children))
	}
	if got := textOfTree(built.Root); strings.Contains(got, "no canvas here") {
		t.Errorf("the fallback text reached the page: %q", got)
	}
}

// TestABlankCanvasIsNotReportedAsMissing is the other half of the boundary, and
// the reason it is a test rather than an omission.
//
// A finding marked Unsupported says a reader is missing something. On a page
// with no script, a canvas is not: the bitmap is blank because nothing drew on
// it, which is exactly what the page says. Reporting it anyway would mark every
// document holding an empty canvas as one this engine could not render — the
// same mistake that made twenty-seven iframe reftests count as tainted while
// drawing the right picture. A canvas a script *would* have painted is a page
// whose <script> was thrown away, and that is reported where it happens.
func TestABlankCanvasIsNotReportedAsMissing(t *testing.T) {
	built := Build(Input{HTML: `<div><canvas width="10" height="10"></canvas></div>`})
	for _, f := range built.Findings {
		if strings.Contains(strings.ToLower(f.Message), "canvas") {
			t.Errorf("a plain canvas was reported: %s", f.Error())
		}
	}

	// And the <script> that would have drawn on it still is, so nothing is
	// hidden by the silence above.
	built = Build(Input{HTML: `<canvas id="c"></canvas><script>draw()</script>`})
	var saidScript bool
	for _, f := range built.Findings {
		if f.Unsupported() && strings.Contains(f.Message, "script") {
			saidScript = true
		}
	}
	if !saidScript {
		t.Error("the <script> beside the canvas was not reported; the canvas is " +
			"silent on the understanding that this is not")
	}
}

// textOfTree is every string a box tree would set, joined.
func textOfTree(b *Box) string {
	if b == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*Box)
	walk = func(n *Box) {
		sb.WriteString(n.Text)
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(b)
	return sb.String()
}
