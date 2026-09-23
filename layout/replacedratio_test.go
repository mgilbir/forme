package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// aspect-ratio on a replaced element, and under box-sizing: border-box.
//
// CSS Sizing 4 §5.1: "<ratio>" is the box's preferred ratio, and it is used in
// place of the natural one in CSS 2.1's replaced-element sizing; "auto &&
// <ratio>" keeps the natural ratio where there is one. A "<ratio>" relates the
// dimensions of the box box-sizing names, and "auto && <ratio>" the content
// box's, always. The replaced sizing never read the property, so "width: 200px;
// aspect-ratio: 1" on a 100 by 50 picture drew 200 by 100, and the ratio was
// applied to the content box whatever box-sizing said (audit C80).

// ratioFixtures is a picture 100 by 50, whose natural ratio is 2.
var ratioFixtures = mapResolver{
	"wide.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50">` +
		`<rect width="100%" height="100%" fill="green"/></svg>`),
}

// sizeOf lays a document out 600 wide and answers #e's content box.
func sizeOf(t *testing.T, doc, css string) (w, h float64) {
	t.Helper()
	built := Build(Input{HTML: doc, Resources: ratioFixtures,
		CSS: []Stylesheet{{Source: "body { margin: 0 } " + css}}})
	for _, f := range built.Findings {
		if f.Rule == RuleImageUndecodable || f.Rule == RuleResourceBlocked {
			t.Fatalf("the fixture did not load: %v", f)
		}
	}
	width, _ := style.FromPx(600)
	height, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: width, H: height}, built.Fonts, NewRecorder(nil))
	e := find(t, frag, "e")
	return e.BorderRect.W.Px() - e.Border.Horizontal().Px() - e.Padding.Horizontal().Px(),
		e.BorderRect.H.Px() - e.Border.Vertical().Px() - e.Padding.Vertical().Px()
}

func TestAspectRatioSizesAReplacedElement(t *testing.T) {
	const img = `<img id=e src="wide.svg">`
	const frame = `<iframe id=e></iframe>`
	for _, tc := range []struct {
		name, doc, css string
		w, h           float64
	}{
		// The control: the picture's own ratio.
		{"no declaration", img, `#e { display: block; width: 200px }`, 200, 100},
		// "<ratio>" replaces the natural one in both directions.
		{"a width", img, `#e { display: block; width: 200px; aspect-ratio: 1 / 1 }`, 200, 200},
		{"a height", img, `#e { display: block; height: 100px; aspect-ratio: 3 }`, 300, 100},
		{"the idiom", `<div style="width: 320px">` + img + `</div>`,
			`#e { display: block; width: 100%; aspect-ratio: 16 / 9 }`, 320, 180},
		// Both auto: the natural width stands and the height follows the
		// declared ratio, §5.2's ratio-dependent axis.
		{"no size", img, `#e { display: block; aspect-ratio: 1 }`, 100, 100},
		// "auto && <ratio>" keeps a natural ratio where there is one, and
		// takes the declared one where there is none.
		{"auto with a natural ratio", img,
			`#e { display: block; width: 200px; aspect-ratio: auto 1 / 1 }`, 200, 100},
		{"auto with no natural ratio", frame,
			`#e { display: block; width: 200px; border: 0; aspect-ratio: auto 2 }`, 200, 100},
		// Inline, which is the same function.
		{"inline", img, `#e { width: 50px; aspect-ratio: 1 }`, 50, 50},
		// A box shrunk to fit around the picture measures it by the same ratio
		// it is laid out by.
		{"shrunk to fit", `<div id=e style="float: left"><img src="wide.svg"></div>`,
			`img { display: block; height: 50px; aspect-ratio: 1 }`, 50, 50},
		// The limits of §10.4 keep the declared shape, not the picture's.
		{"a maximum", img, `#e { display: block; width: 200px; max-height: 50px; aspect-ratio: 1 }`,
			50, 50},
	} {
		w, h := sizeOf(t, tc.doc, tc.css)
		if w != tc.w || h != tc.h {
			t.Errorf("%s: %s is %gx%g, want %gx%g", tc.name, tc.css, w, h, tc.w, tc.h)
		}
	}
}

// TestAspectRatioIsOfTheBoxBoxSizingNames: a "<ratio>" under border-box is a
// ratio of the border box, so a box a hundred wide with forty pixels of
// padding each side and a ratio of one is a hundred tall, not twenty.
func TestAspectRatioIsOfTheBoxBoxSizingNames(t *testing.T) {
	const img = `<img id=e src="wide.svg">`
	for _, tc := range []struct {
		name, doc, css string
		w, h           float64
	}{
		{"a block", `<div id=e></div>`,
			`#e { width: 100px; padding: 0 40px; box-sizing: border-box; aspect-ratio: 1 }`, 20, 100},
		{"a block with padding on both axes", `<div id=e></div>`,
			`#e { width: 100px; padding: 10px 40px; box-sizing: border-box; aspect-ratio: 1 }`, 20, 80},
		{"a block, content-box", `<div id=e></div>`,
			`#e { width: 100px; padding: 0 40px; aspect-ratio: 1 }`, 100, 100},
		// "auto && <ratio>" is of the content box whatever box-sizing says.
		{"a block, auto", `<div id=e></div>`,
			`#e { width: 100px; padding: 0 40px; box-sizing: border-box; aspect-ratio: auto 1 }`, 20, 20},
		{"a picture", img,
			`#e { display: block; width: 100px; padding: 0 40px; box-sizing: border-box; aspect-ratio: 1 }`,
			20, 100},
		{"a picture from its height", img,
			`#e { display: block; height: 100px; padding: 10px 0; box-sizing: border-box; aspect-ratio: 2 }`,
			200, 80},
		// The natural ratio is of the content box, and so is "auto".
		{"a picture, natural", img,
			`#e { display: block; width: 100px; padding: 0 20px; box-sizing: border-box }`, 60, 30},
		{"a picture, auto", img,
			`#e { display: block; width: 100px; padding: 0 20px; box-sizing: border-box; aspect-ratio: auto 1 }`,
			60, 30},
		// §10.4's table in the border box's sizes: 200 by 200 there, clamped to
		// a border box 100 tall, keeps the ratio of the border box.
		{"a picture and a maximum", img,
			`#e { display: block; width: 200px; padding: 0 20px; box-sizing: border-box; ` +
				`max-height: 100px; aspect-ratio: 1 }`, 60, 100},
	} {
		w, h := sizeOf(t, tc.doc, tc.css)
		if w != tc.w || h != tc.h {
			t.Errorf("%s: %s is %gx%g, want %gx%g", tc.name, tc.css, w, h, tc.w, tc.h)
		}
	}
}

// TestAShrinkToFitAspectRatioIsReported is the comment on aspectratio.go made
// a test: where CSS Sizing 4 would have the ratio answer a box's width — its
// width auto and its height not — this engine does not, and says so, for a
// box shrunk to fit as much as for one in the flow.
func TestAShrinkToFitAspectRatioIsReported(t *testing.T) {
	for _, css := range []string{
		`#e { height: 50px; aspect-ratio: 2 }`,
		`#e { float: left; height: 50px; aspect-ratio: 2 }`,
		`#e { display: inline-block; height: 50px; aspect-ratio: 2 }`,
	} {
		built := Build(Input{HTML: `<div id=e>x</div>`, CSS: []Stylesheet{{Source: css}}})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
		found := false
		for _, f := range rec.Findings() {
			if f.Property == "aspect-ratio" && strings.Contains(f.Message, "width") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: nothing said the ratio was not used for the width; findings %v",
				css, rec.Findings())
		}
	}
}

// TestAShrinkToFitBoxMeasuresAPictureAsItIsLaidOut: the width a box shrunk to
// fit takes from a replaced child is the width that child is laid out at. It
// was the natural width alone, so content with only a natural height and a
// ratio, or with no dimensions at all, measured nought — a float around it
// shrank to nothing and the picture hung out of it.
func TestAShrinkToFitBoxMeasuresAPictureAsItIsLaidOut(t *testing.T) {
	fixtures := mapResolver{"tall.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" ` +
		`height="50" viewBox="0 0 100 50"><rect width="100%" height="100%" fill="green"/></svg>`)}
	for _, child := range []string{
		`<img src="tall.svg" style="display: block">`,
		`<iframe style="display: block; border: 0"></iframe>`,
		`<video style="display: block"></video>`,
	} {
		measure := func(doc string) float64 {
			built := Build(Input{HTML: doc, Resources: fixtures,
				CSS: []Stylesheet{{Source: "body { margin: 0 }"}}})
			w, _ := style.FromPx(600)
			h, _ := style.FromPx(10000)
			frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
			return find(t, frag, "e").BorderRect.W.Px()
		}
		alone := measure(strings.Replace(child, "style=", "id=e style=", 1))
		around := measure(`<div id=e style="float: left">` + child + `</div>`)
		if alone == 0 || around != alone {
			t.Errorf("%s is %g wide, and a float around it %g", child, alone, around)
		}
	}
}
