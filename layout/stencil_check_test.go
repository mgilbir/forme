package layout

import (
	"image"
	"image/color"
	"testing"
)

// Tests of the stencil rule in the picture comparison.
//
// This is measuring apparatus, so it is tested in both directions and the second
// matters more: a rule that lets two *different* pages compare equal does not
// produce a wrong page, it produces a wrong number, silently, in every
// measurement taken afterwards.
//
// The rule: a picture whose every pixel is either one fully opaque colour or
// fully transparent puts nothing new on a page that is already that colour. It
// is exact — every point it covers ends that colour whichever kind of pixel is
// there — and it is worth having because a stylesheet that draws a shape in the
// colour it is drawn on is a page with nothing on it, and the suite has one:
// background-image-transparency-001 tiles a green pattern on transparency over
// "background-color: #008000" and its reference simply draws a green image.

// stencilImage builds a picture of one opaque colour on transparency, in the
// pattern of a checkerboard so that nothing about it is uniform.
func stencilImage(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, c)
			} else {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	return img
}

var stencilGreen = color.NRGBA{G: 128, A: 255}

func TestAStencilIsRecognised(t *testing.T) {
	got, ok := stencilColor(stencilImage(4, 4, stencilGreen))
	if !ok {
		t.Fatal("a checkerboard of one opaque colour on transparency is not reported as a stencil")
	}
	if !sameColour(got, picGreen) {
		t.Errorf("the stencil's colour is %v, want %v", got, picGreen)
	}
}

func TestWhatIsNotAStencil(t *testing.T) {
	twoColours := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	twoColours.SetNRGBA(0, 0, color.NRGBA{G: 128, A: 255})
	twoColours.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})

	halfTransparent := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	halfTransparent.SetNRGBA(0, 0, color.NRGBA{G: 128, A: 255})
	halfTransparent.SetNRGBA(1, 0, color.NRGBA{G: 128, A: 128})

	allClear := image.NewNRGBA(image.Rect(0, 0, 2, 2))

	for _, tc := range []struct {
		name string
		img  image.Image
	}{
		{"two opaque colours", twoColours},
		{"a half-transparent pixel, which composites into a third colour", halfTransparent},
		{"nothing opaque at all, which has no colour to compare against", allClear},
		{"no picture", nil},
	} {
		if _, ok := stencilColor(tc.img); ok {
			t.Errorf("%s is reported as a stencil", tc.name)
		}
	}
}

func TestAStencilOnlyDisappearsOverItsOwnColour(t *testing.T) {
	img := stencilImage(4, 4, stencilGreen)
	area := picRect(0, 0, 100, 100)
	for _, tc := range []struct {
		name  string
		under []Op
		want  bool
	}{
		{"its own colour", []Op{picFill(0, 0, 100, 100, picGreen)}, true},
		{"a different colour", []Op{picFill(0, 0, 100, 100, picRed)}, false},
		{"nothing at all", nil, false},
		{"its own colour over only part of it", []Op{picFill(0, 0, 100, 50, picGreen)}, false},
		{"its own colour with a red corner", []Op{
			picFill(0, 0, 100, 100, picGreen), picFill(90, 90, 10, 10, picRed),
		}, false},
		{"its own colour with a red centre", []Op{
			picFill(0, 0, 100, 100, picGreen), picFill(40, 40, 20, 20, picRed),
		}, false},
	} {
		got := coversNothingNew(picFills(tc.under), area, picGreen)
		if got != tc.want {
			t.Errorf("over %s: coversNothingNew = %v, want %v", tc.name, got, tc.want)
		}
	}
	_ = img
}

func TestAStencilOverAPictureIsNotInvisible(t *testing.T) {
	// A picture under it is a mark this comparison cannot see through, so
	// nothing can be said about what the stencil leaves showing.
	pattern := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	pattern.SetNRGBA(0, 0, color.NRGBA{G: 128, A: 255})
	pattern.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	under := picFills([]Op{DrawImage{
		Rect: picRect(0, 0, 100, 100), Image: pattern, Key: "pattern",
	}})
	if coversNothingNew(under, picRect(0, 0, 100, 100), picGreen) {
		t.Error("a stencil over a picture is reported as covering nothing new")
	}
}

func TestTheStencilRuleStillSeesADifference(t *testing.T) {
	// The direction that matters. Two pages that really do differ — a green
	// stencil over green against a green stencil over red — are still different.
	img := stencilImage(4, 4, stencilGreen)
	overGreen := []Op{
		picFill(0, 0, 100, 100, picGreen),
		TileImage{Clip: picRect(0, 0, 100, 100), Tile: picRect(0, 0, 4, 4),
			StepX: picPx(4), StepY: picPx(4), Image: img, Key: "stencil"},
	}
	overRed := []Op{
		picFill(0, 0, 100, 100, picRed),
		TileImage{Clip: picRect(0, 0, 100, 100), Tile: picRect(0, 0, 4, 4),
			StepX: picPx(4), StepY: picPx(4), Image: img, Key: "stencil"},
	}
	plainGreen := []Op{picFill(0, 0, 100, 100, picGreen)}
	clip := picRect(0, 0, 200, 200)

	if !pictureEqual(overGreen, plainGreen, clip) {
		t.Error("a green stencil over green is not equal to plain green, and it is the same page")
	}
	if pictureEqual(overRed, plainGreen, clip) {
		t.Error("a green stencil over red is equal to plain green, and the two are different pages")
	}
	// And the case the rule is most dangerous in: a stencil over a colour that
	// is *not* its own must not vanish, or a page with a pattern on it would
	// compare equal to the bare background.
	plainRed := []Op{picFill(0, 0, 100, 100, picRed)}
	if pictureEqual(overRed, plainRed, clip) {
		t.Error("a green stencil over red is equal to plain red; dropping the " +
			"stencil without checking what is under it erases the pattern")
	}
}
