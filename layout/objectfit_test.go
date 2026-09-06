package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The fixture is a picture 40 wide and 20 tall — a ratio of two — put in a box
// 100 by 100. Every answer below is a whole number of pixels because of that
// choice, and a fit computed against the wrong dimension gives a different one
// rather than a rounding.
const objectFitDoc = `<img id="i" src="wide.png">`

// The body's own margin is the user agent's eight pixels and would sit between
// the page and the box, putting a constant in front of every number below.
const objectFitCSS = `body { margin: 0 } img { display: block; width: 100px; height: 100px }`

// laidOutWith lays a document out with a resolver, which is what a picture
// needs and what paintWith already does — this is the same walk stopping one
// step earlier, at the fragments.
func laidOutWith(t *testing.T, res ResourceResolver, htmlSrc, cssSrc string) *Fragment {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, Resources: res, CSS: []Stylesheet{{Source: cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(A4.Content().W.Px())
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	if frag == nil {
		t.Fatal("layout produced no fragment")
	}
	return frag
}

// drawnPicture is where the one picture on the page was drawn, and what it was
// clipped to.
func drawnPicture(t *testing.T, css string) (Rect, Clip) {
	t.Helper()
	res := mapResolver{"wide.png": encodePNG(t, 40, 20)}
	ops := paintWith(t, res, objectFitDoc, objectFitCSS+" "+css)
	var found []DrawImage
	for _, op := range ops {
		if d, ok := op.(DrawImage); ok {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d pictures drawn, want 1", len(found))
	}
	return found[0].Rect, found[0].Clip
}

// pictureAt is a rectangle as four numbers, which is how every expectation
// below is written: a fit computed against the wrong dimension gives a
// different number rather than a rounding.
func pictureAt(r Rect) [4]float64 {
	return [4]float64{r.X.Px(), r.Y.Px(), r.W.Px(), r.H.Px()}
}

// drawnPictureOf is where the picture went, as four numbers.
func drawnPictureOf(t *testing.T, css string) [4]float64 {
	t.Helper()
	rect, _ := drawnPicture(t, css)
	return pictureAt(rect)
}

// TestObjectFitFillStretchesTheContentToItsBox pins the initial value, which is
// what this engine did before the property was read at all. It is here so that
// the change is known to have left the default alone.
func TestObjectFitFillStretchesTheContentToItsBox(t *testing.T) {
	for _, css := range []string{``, `img { object-fit: fill }`} {
		rect, clip := drawnPicture(t, css)
		if got := pictureAt(rect); got != [4]float64{0, 0, 100, 100} {
			t.Errorf("%q drew the picture at %v, want the whole box", css, got)
		}
		if clip.Active {
			t.Errorf("%q clipped a picture that fills its box exactly", css)
		}
	}
}

// TestObjectFitContainFitsTheContentInsideItsBox. The picture is twice as wide
// as it is tall, so in a square box it is limited by the width: 100 by 50,
// centred, with 25 above and below.
func TestObjectFitContainFitsTheContentInsideItsBox(t *testing.T) {
	rect, clip := drawnPicture(t, `img { object-fit: contain }`)
	if got := pictureAt(rect); got != [4]float64{0, 25, 100, 50} {
		t.Errorf("contain drew the picture at %v, want 0,25 100x50", got)
	}
	// The display list drops a clip that cuts nothing, which is what keeps the
	// list of a picture inside a clip identical to the list of one with no clip
	// at all — the property the reftest comparison rests on.
	if clip.Active {
		t.Error("contain left a clip on a picture that is inside its box")
	}
}

// TestObjectFitCoverFillsTheBoxAndIsClippedToIt. Covering a square with a
// two-to-one picture is limited by the *height*: 200 by 100, which reaches
// 50px past each side and is cut off there.
func TestObjectFitCoverFillsTheBoxAndIsClippedToIt(t *testing.T) {
	rect, clip := drawnPicture(t, `img { object-fit: cover }`)
	if got := pictureAt(rect); got != [4]float64{-50, 0, 200, 100} {
		t.Errorf("cover drew the picture at %v, want -50,0 200x100", got)
	}
	if !clip.Active {
		t.Fatal("cover drew a picture past its box with no clip")
	}
	if got := pictureAt(clip.Rect); got != [4]float64{0, 0, 100, 100} {
		t.Errorf("cover clipped to %v, want the box at 0,0 100x100", got)
	}
}

// TestObjectFitNoneIsTheContentsOwnSize, centred in the box. It is the value
// that says the box has no say in how big the picture is.
func TestObjectFitNoneIsTheContentsOwnSize(t *testing.T) {
	rect, clip := drawnPicture(t, `img { object-fit: none }`)
	if got := pictureAt(rect); got != [4]float64{30, 40, 40, 20} {
		t.Errorf("none drew the picture at %v, want 30,40 40x20", got)
	}
	if clip.Active {
		t.Error("none clipped a picture smaller than its box")
	}
}

// TestObjectFitNoneIsClippedWhenItIsLargerThanItsBox. The same value, the other
// way round: a picture bigger than the box it was put in reaches outside it and
// is cut, which is the case that needs the clip and the one an engine that only
// tested "cover" would miss.
func TestObjectFitNoneIsClippedWhenItIsLargerThanItsBox(t *testing.T) {
	res := mapResolver{"wide.png": encodePNG(t, 400, 200)}
	ops := paintWith(t, res, objectFitDoc,
		objectFitCSS+` img { object-fit: none }`)
	var found []DrawImage
	for _, op := range ops {
		if d, ok := op.(DrawImage); ok {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d pictures drawn, want 1", len(found))
	}
	if got := pictureAt(found[0].Rect); got != [4]float64{-150, -50, 400, 200} {
		t.Errorf("none drew the picture at %v, want -150,-50 400x200", got)
	}
	if !found[0].Clip.Active {
		t.Fatal("a picture larger than its box was drawn with no clip")
	}
	if got := pictureAt(found[0].Clip.Rect); got != [4]float64{0, 0, 100, 100} {
		t.Errorf("it was clipped to %v, want the box", got)
	}
}

// TestObjectFitScaleDownIsTheSmallerOfNoneAndContain. It is the value a
// thumbnail wants: shrink what is too big, leave what is not alone.
func TestObjectFitScaleDownIsTheSmallerOfNoneAndContain(t *testing.T) {
	// 40 by 20 fits inside 100 by 100, so this is "none".
	rect, _ := drawnPicture(t, `img { object-fit: scale-down }`)
	if got := pictureAt(rect); got != [4]float64{30, 40, 40, 20} {
		t.Errorf("scale-down of a small picture drew it at %v, want its own size", got)
	}

	// 400 by 200 does not, so this is "contain": 100 by 50, centred.
	res := mapResolver{"wide.png": encodePNG(t, 400, 200)}
	ops := paintWith(t, res, objectFitDoc, objectFitCSS+` img { object-fit: scale-down }`)
	var found []DrawImage
	for _, op := range ops {
		if d, ok := op.(DrawImage); ok {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d pictures drawn, want 1", len(found))
	}
	if got := pictureAt(found[0].Rect); got != [4]float64{0, 25, 100, 50} {
		t.Errorf("scale-down of a large picture drew it at %v, want the contained 0,25 100x50", got)
	}
	if found[0].Clip.Active {
		t.Error("scale-down clipped a picture it had already shrunk to fit")
	}
}

// TestObjectFitLeavesTheBoxWhereItWas. The property is about the content and
// not about the box: the space the element takes in the flow, and so where the
// text after it goes, is the same whichever value was asked for. An engine that
// implemented this by resizing the element would pass every test above and move
// the rest of the page.
func TestObjectFitLeavesTheBoxWhereItWas(t *testing.T) {
	res := mapResolver{"wide.png": encodePNG(t, 40, 20)}
	var want [4]float64
	for i, css := range []string{`fill`, `contain`, `cover`, `none`, `scale-down`} {
		root := laidOutWith(t, res, `<img id="i" src="wide.png"><p id="after">x</p>`,
			objectFitCSS+` img { object-fit: `+css+` }`)
		box := fragmentFor(root, "i")
		after := fragmentFor(root, "after")
		if box == nil || after == nil {
			t.Fatalf("%s: the picture or the paragraph after it is missing", css)
		}
		got := [4]float64{
			box.BorderRect.W.Px(), box.BorderRect.H.Px(),
			after.BorderRect.X.Px(), after.BorderRect.Y.Px(),
		}
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Errorf("object-fit: %s moved the layout to %v, want %v", css, got, want)
		}
	}
}

// TestObjectFitOnContentWithNoSizeOfItsOwnFillsTheBox. §5.5 sizes the content
// with the default sizing algorithm, whose answer with no intrinsic dimensions
// and no ratio is the box itself — so there is nothing to fit and nothing to
// centre, and any value paints what "fill" would.
func TestObjectFitOnContentWithNoSizeOfItsOwnFillsTheBox(t *testing.T) {
	got, _ := fitContent(Rect{W: bgpx(100), H: bgpx(100)}, Size{}, objectContain)
	if r := pictureAt(got); r != [4]float64{0, 0, 100, 100} {
		t.Errorf("content with no size of its own was fitted to %v, want the box", r)
	}
}

// TestObjectFitKeepsAShapeStatedOnlyAsARatio. A picture can state a ratio
// without stating dimensions — CSS 2.1 §10.3.2 keeps the two apart because a
// format can supply either — and a shape is all fitting needs.
func TestObjectFitKeepsAShapeStatedOnlyAsARatio(t *testing.T) {
	box := Rect{W: bgpx(100), H: bgpx(100)}
	rect, _ := fitContent(box, naturalSizeOf(&ReplacedContent{Ratio: 2}), objectContain)
	if got := pictureAt(rect); got != [4]float64{0, 25, 100, 50} {
		t.Errorf("a two-to-one ratio was contained as %v, want 0,25 100x50", got)
	}
}

// TestAnObjectFitThisEngineCannotReadIsReported. A value that is not one of the
// five is the initial one, which is a stretched picture — plausible, wrong, and
// with nothing on the page to say the declaration did not take.
func TestAnObjectFitThisEngineCannotReadIsReported(t *testing.T) {
	res := mapResolver{"wide.png": encodePNG(t, 40, 20)}
	got := Compose(Input{
		HTML:      objectFitDoc,
		Resources: res,
		CSS:       []Stylesheet{{Source: objectFitCSS + ` img { object-fit: fit }`}},
	}, Options{})
	found := false
	for _, f := range got.Findings {
		if f.Property == "object-fit" && strings.Contains(f.Message, "not one this engine reads") {
			found = true
		}
	}
	if !found {
		t.Errorf("an unreadable object-fit was not reported; findings were %v", got.Findings)
	}

	// The finding says the content was stretched to its box, so it had better
	// have been: an unreadable value is the initial one and not some other.
	if got := drawnPictureOf(t, `img { object-fit: fit }`); got != [4]float64{0, 0, 100, 100} {
		t.Errorf("an unreadable object-fit drew the picture at %v, want the whole box", got)
	}

	// And a value it can read is not reported, or the finding above says
	// nothing about which values are understood.
	got = Compose(Input{
		HTML:      objectFitDoc,
		Resources: res,
		CSS:       []Stylesheet{{Source: objectFitCSS + ` img { object-fit: cover }`}},
	}, Options{})
	for _, f := range got.Findings {
		if f.Property == "object-fit" {
			t.Errorf("object-fit: cover was reported: %s", f.Message)
		}
	}
}
