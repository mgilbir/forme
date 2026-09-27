package layout

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Gradients drawn as gradients. Most of these tests are the specifications' own
// examples, which come in two kinds: a value and the picture it describes, and
// two values the specification says describe the same picture. The second kind
// is the stronger, because it holds the geometry to account without this file
// having to restate it — CSS Images says "linear-gradient(135deg, yellow, blue)"
// and "linear-gradient(-45deg, blue, yellow)" are one gradient, and an engine
// that got the gradient line's length wrong would get it wrong differently for
// the two.

// laidGradientOf paints a 200 by 100 box with a background image and returns its
// one gradient operation.
func laidGradientOf(t *testing.T, value string) FillGradient {
	t.Helper()
	got, ok := tryGradientOf(t, value, "")
	if !ok {
		t.Fatalf("%s: no gradient was painted", value)
	}
	return got
}

func tryGradientOf(t *testing.T, value, extra string) (FillGradient, bool) {
	t.Helper()
	ops := paintOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 200px; height: 100px;
		background-image: `+value+`; `+extra+` }`)
	for _, op := range ops {
		if g, ok := op.(FillGradient); ok {
			return g, true
		}
	}
	return FillGradient{}, false
}

// colourIn is the colour a gradient's first tile paints at a point measured
// from the tile's top left, in pixels.
func colourIn(g FillGradient, x, y float64) style.RGBA {
	return g.Gradient.ColorAtOffset(g.Gradient.offsetOf(x, y))
}

// near reports whether two colours differ by less than a tenth of an 8-bit step
// on every channel, which is far inside what any device shows and outside the
// float noise of two routes to one number.
func near(a, b style.RGBA) bool {
	return math.Abs(a.R-b.R) < 0.1 && math.Abs(a.G-b.G) < 0.1 &&
		math.Abs(a.B-b.B) < 0.1 && math.Abs(a.A-b.A) < 0.1/255
}

// sameGradientPicture asserts that two values paint the same colour at every
// point of a grid over the 200 by 100 box and a little past its edges.
func sameGradientPicture(t *testing.T, a, b string) {
	t.Helper()
	ga, gb := laidGradientOf(t, a), laidGradientOf(t, b)
	for y := -10.0; y <= 110; y += 7.5 {
		for x := -10.0; x <= 210; x += 7.5 {
			ca, cb := colourIn(ga, x, y), colourIn(gb, x, y)
			if !near(ca, cb) {
				t.Errorf("%s and %s differ at %g,%g: %v against %v", a, b, x, y, ca, cb)
				return
			}
		}
	}
}

func rgb(r, g, b float64) style.RGBA { return style.RGBA{R: r, G: g, B: b, A: 1} }

// white is the one colour these tests use that the package's other tests do
// not already name; red, blue, yellow and green are theirs.
var white = rgb(255, 255, 255)

// TestTheSpecificationsLinearGradientsAreOne is CSS Images 3 §3.1.2's first
// example, five spellings of one vertical gradient.
func TestTheSpecificationsLinearGradientsAreOne(t *testing.T) {
	first := "linear-gradient(yellow, blue)"
	for _, other := range []string{
		"linear-gradient(to bottom, yellow, blue)",
		"linear-gradient(180deg, yellow, blue)",
		"linear-gradient(to top, blue, yellow)",
		"linear-gradient(to bottom, yellow 0%, blue 100%)",
		"linear-gradient(0.5turn, yellow, blue)",
		"linear-gradient(200grad, yellow, blue)",
		"linear-gradient(in srgb, yellow, blue)",
		"linear-gradient(to bottom in srgb, yellow, blue)",
		"linear-gradient(in srgb to bottom, yellow, blue)",
	} {
		sameGradientPicture(t, first, other)
	}
	g := laidGradientOf(t, first)
	if c := colourIn(g, 37, 0); !near(c, yellow) {
		t.Errorf("the top edge is %v, want yellow", c)
	}
	if c := colourIn(g, 150, 100); !near(c, blue) {
		t.Errorf("the bottom edge is %v, want blue", c)
	}
	// Halfway down, halfway between: 127.5 of each channel that differs.
	if c := colourIn(g, 99, 50); !near(c, rgb(127.5, 127.5, 127.5)) {
		t.Errorf("the middle is %v, want the even blend", c)
	}
}

// TestAnAngledGradientReachesItsCorners is the second example: "though the angle
// is not exactly the same as the angle between the corners, the gradient line is
// still sized so as to make the gradient yellow exactly at the upper-left corner,
// and blue exactly at the lower-right corner."
func TestAnAngledGradientReachesItsCorners(t *testing.T) {
	sameGradientPicture(t, "linear-gradient(135deg, yellow, blue)", "linear-gradient(-45deg, blue, yellow)")
	g := laidGradientOf(t, "linear-gradient(135deg, yellow, blue)")
	if c := colourIn(g, 0, 0); !near(c, yellow) {
		t.Errorf("the top left corner is %v, want yellow", c)
	}
	if c := colourIn(g, 200, 100); !near(c, blue) {
		t.Errorf("the bottom right corner is %v, want blue", c)
	}
	// And the line's length is §3.1.1's |W sin A| + |H cos A|.
	d := g.Gradient
	length := math.Hypot(d.End.X.Px()-d.Start.X.Px(), d.End.Y.Px()-d.Start.Y.Px())
	want := 200*math.Sin(135*math.Pi/180) + 100*math.Abs(math.Cos(135*math.Pi/180))
	if math.Abs(length-want) > 0.05 {
		t.Errorf("the gradient line is %g long, want %g", length, want)
	}
}

// TestACornerGradientCrossesTheOtherTwoCorners is the corner example: "red and
// blue exactly in the bottom-left and top-right corners ... the color at 50% (in
// this case, white) stretches across the top-left and bottom-right corners."
func TestACornerGradientCrossesTheOtherTwoCorners(t *testing.T) {
	g := laidGradientOf(t, "linear-gradient(to top right, red, white, blue)")
	for _, tc := range []struct {
		x, y float64
		want style.RGBA
	}{
		{0, 100, red}, {200, 0, blue}, {0, 0, white}, {200, 100, white}, {100, 50, white},
	} {
		if c := colourIn(g, tc.x, tc.y); !near(c, tc.want) {
			t.Errorf("at %g,%g the colour is %v, want %v", tc.x, tc.y, c, tc.want)
		}
	}
}

// TestTheFixupExamples are CSS Images 4 §3.5.3's six pairs, "a manually
// fixed-up version of the former ... both gradients will render identically",
// read on a line 100 pixels long so that a percentage and a pixel are the same
// number. The fourth writes its answer with calc(), which is resolved here.
func TestTheFixupExamples(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  []float64
	}{
		{"linear-gradient(red, white 20%, blue)", []float64{0, 20, 100}},
		{"linear-gradient(red 40%, white, black, blue)", []float64{40, 60, 80, 100}},
		{"linear-gradient(red -50%, white, blue)", []float64{-50, 25, 100}},
		{"linear-gradient(red -50px, white, blue)", []float64{-50, 25, 100}},
		{"linear-gradient(red 20px, white 0px, blue 40px)", []float64{20, 20, 40}},
		{"linear-gradient(red, white -50%, black 150%, blue)", []float64{0, 0, 150, 150}},
	} {
		g, ok := tryGradientOf(t, tc.value, "height: 100px")
		if !ok {
			t.Errorf("%s: no gradient", tc.value)
			continue
		}
		var got []float64
		for _, s := range g.Gradient.Stops {
			got = append(got, math.Round(s.Offset*100*1e6)/1e6)
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s: stops at %v%%, want %v%%", tc.value, got, tc.want)
		}
	}
}

// TestATransitionHintPlacesTheHalfwayColour is §3.5.2's hint: "the 'halfway
// color' ... occurring exactly where the hint specifies", by the curve
// C = P^log_H(.5).
func TestATransitionHintPlacesTheHalfwayColour(t *testing.T) {
	g := laidGradientOf(t, "linear-gradient(to right, red 0%, 25%, blue 100%)")
	if c := colourIn(g, 50, 50); !near(c, rgb(127.5, 0, 127.5)) {
		t.Errorf("at the hint the colour is %v, want the halfway blend", c)
	}
	// Three quarters of the way the weight is 0.75^0.5.
	w := math.Pow(0.75, 0.5)
	if c := colourIn(g, 150, 50); !near(c, rgb(255*(1-w), 0, 255*w)) {
		t.Errorf("at three quarters the colour is %v", c)
	}
	// A hint at the middle is the ordinary blend, and one at either stop is the
	// hard stop the curve comes to there.
	// At an angle rather than "to right", because a hard stop to one side is
	// a stack of fills (gradient.go) and not a gradient at all.
	sameGradientPicture(t, "linear-gradient(90deg, red, 50%, blue)", "linear-gradient(90deg, red, blue)")
	sameGradientPicture(t, "linear-gradient(80deg, red 20%, 20%, blue 60%)",
		"linear-gradient(80deg, red 20%, blue 20%, blue 60%)")
	sameGradientPicture(t, "linear-gradient(80deg, red 20%, 60%, blue 60%)",
		"linear-gradient(80deg, red 60%, blue 60%)")
}

// TestAGradientInterpolatesInPremultipliedAlpha is CSS Color 4 §13.4: red to
// transparent fades red out rather than through the black "transparent" is
// written as.
func TestAGradientInterpolatesInPremultipliedAlpha(t *testing.T) {
	g := laidGradientOf(t, "linear-gradient(to right, red, transparent)")
	if c := colourIn(g, 100, 50); !near(c, style.RGBA{R: 255, A: 0.5}) {
		t.Errorf("halfway to transparent the colour is %v, want half-transparent red", c)
	}
}

// TestADoublePositionIsTwoStops is §3.5.1: "A color stop with two positions is
// equivalent to specifying two color stops with the same color."
func TestADoublePositionIsTwoStops(t *testing.T) {
	sameGradientPicture(t, "linear-gradient(to right, red 10% 40%, blue)",
		"linear-gradient(to right, red 10%, red 40%, blue)")
}

// TestARepeatingGradientRepeatsItsStops is CSS Images 3 §3.3's example:
// "repeating-linear-gradient(red 10px, blue 50px) is equivalent to
// linear-gradient(..., red -30px, blue 10px, red 10px, blue 50px, red 50px, blue
// 90px, ...)".
func TestARepeatingGradientRepeatsItsStops(t *testing.T) {
	g := laidGradientOf(t, "repeating-linear-gradient(red 10px, blue 50px)")
	for _, y := range []float64{-30, 10, 50, 90} {
		if c := colourIn(g, 5, y+0.01); !near(c, red) {
			t.Errorf("just after %gpx the colour is %v, want red", y, c)
		}
	}
	for _, y := range []float64{30, 70, -10} {
		if c := colourIn(g, 5, y); !near(c, rgb(127.5, 0, 127.5)) {
			t.Errorf("at %gpx the colour is %v, want the halfway blend", y, c)
		}
	}
}

// TestARepeatingGradientOfNoLengthIsItsAverage is §3.3's average colour, with
// its own example: "repeating-linear-gradient(red 0px, white 0px, blue 0px) is
// rendered as a solid light-purple image (equal to rgb(75%,50%,75%))".
func TestARepeatingGradientOfNoLengthIsItsAverage(t *testing.T) {
	ops := paintOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 200px; height: 100px;
		background-image: repeating-linear-gradient(red 0px, white 0px, blue 0px) }`)
	want := rgb(191.25, 127.5, 191.25)
	found := false
	for _, op := range ops {
		switch v := op.(type) {
		case FillGradient:
			t.Errorf("a gradient of no length was painted as a gradient: %+v", v)
		case FillRect:
			if near(v.Color, want) {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no fill of rgb(75%%,50%%,75%%): %v", ops)
	}
	// And one repeating more finely than a layout unit is the same colour: the
	// specification's "physical resolution of the output device is
	// insufficient", which no device's is at that pitch.
	//
	// In percentages, because a length is a whole number of layout units by
	// the time it is read and a hundredth of a pixel is none; a percentage of
	// the line is not rounded until the line has a length. And unevenly, so
	// that the average is weighted: the red segment is three quarters of the
	// period, so red contributes seven eighths and blue one eighth.
	ops = paintOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 200px; height: 100px;
		background-image: repeating-linear-gradient(red 0%, red 0.0075%, blue 0.01%) }`)
	want = rgb(255*7/8.0, 0, 255/8.0)
	found = false
	for _, op := range ops {
		switch v := op.(type) {
		case FillGradient:
			t.Errorf("a gradient repeating every hundredth of a pixel was painted as one: %+v", v)
		case FillRect:
			found = found || near(v.Color, want)
		}
	}
	if !found {
		t.Errorf("no fill of the weighted average %v: %v", want, ops)
	}
}

// TestTheSpecificationsRadialGradientsAreOne is CSS Images 3 §3.2.4's examples,
// on its own 200 by 100 box.
func TestTheSpecificationsRadialGradientsAreOne(t *testing.T) {
	for _, set := range [][]string{
		{
			"radial-gradient(yellow, green)",
			"radial-gradient(ellipse at center, yellow 0%, green 100%)",
			"radial-gradient(farthest-corner at 50% 50%, yellow, green)",
		},
		{
			"radial-gradient(closest-side at 20px 30px, red, yellow, green)",
			"radial-gradient(20px 30px at 20px 30px, red, yellow, green)",
		},
		{
			"radial-gradient(closest-side circle at 20px 30px, red, yellow, green)",
			"radial-gradient(20px 20px at 20px 30px, red, yellow, green)",
		},
		{
			"radial-gradient(5em circle at top left, yellow, blue)",
			"radial-gradient(circle 80px at 0 0, yellow, blue)",
		},
	} {
		for _, other := range set[1:] {
			sameGradientPicture(t, set[0], other)
		}
	}
	// farthest-corner's ellipse keeps farthest-side's proportions and passes
	// through the corner, so the corner is exactly the last colour.
	g := laidGradientOf(t, "radial-gradient(yellow, green)")
	if c := colourIn(g, 0, 0); !near(c, green) {
		t.Errorf("the corner is %v, want green", c)
	}
	if rx, ry := g.Gradient.RadiusX.Px(), g.Gradient.RadiusY.Px(); math.Abs(rx/ry-2) > 1e-3 ||
		math.Abs(rx-100*math.Sqrt2) > 0.02 {
		t.Errorf("the ending shape is %g by %g, want 100√2 by 50√2", rx, ry)
	}
	// "radial-gradient(red -50px, yellow 100px) produces an elliptical gradient
	// that starts with a reddish-orange color in the center (specifically,
	// #f50)".
	g = laidGradientOf(t, "radial-gradient(red -50px, yellow 100px)")
	if c := colourIn(g, 100, 50); !near(c, rgb(255, 85, 0)) {
		t.Errorf("the centre is %v, want #f50", c)
	}
}

// TestADegenerateRadialGradient is §3.2.3's three cases, each of which the
// specification describes by what it looks like.
func TestADegenerateRadialGradient(t *testing.T) {
	// No width: "similar to a horizontal linear gradient that is mirrored
	// across the center", percentages resolving to nothing.
	g := laidGradientOf(t, "radial-gradient(closest-side at 0 50%, red, blue 40px)")
	for _, y := range []float64{0, 50, 99} {
		if c := colourIn(g, 20, y); !near(c, rgb(127.5, 0, 127.5)) {
			t.Errorf("twenty pixels from the centre line at y=%g the colour is %v", y, c)
		}
		if c := colourIn(g, 45, y); !near(c, blue) {
			t.Errorf("past the last stop at y=%g the colour is %v", y, c)
		}
	}
	// A circle of no radius stays a circle, its lengths where they were.
	g = laidGradientOf(t, "radial-gradient(circle 0px at 100px 50px, red, blue 40px)")
	if c := colourIn(g, 100, 70); !near(c, rgb(127.5, 0, 127.5)) {
		t.Errorf("twenty pixels below the centre the colour is %v", c)
	}
	if c := colourIn(g, 120, 50); !near(c, rgb(127.5, 0, 127.5)) {
		t.Errorf("twenty pixels right of the centre the colour is %v", c)
	}
	// No height: "a solid-color image equal to the color of the last
	// color-stop, or equal to the average color of the gradient if it's
	// repeating".
	for value, want := range map[string]style.RGBA{
		"radial-gradient(closest-side at 50% 0, red, blue)":           blue,
		"repeating-radial-gradient(closest-side at 50% 0, red, blue)": rgb(127.5, 0, 127.5),
	} {
		ops := paintOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 200px; height: 100px;
			background-image: `+value+` }`)
		var fills []FillRect
		for _, op := range ops {
			switch v := op.(type) {
			case FillGradient:
				t.Errorf("%s: painted as a gradient", value)
			case FillRect:
				fills = append(fills, v)
			}
		}
		if len(fills) != 1 || !near(fills[0].Color, want) {
			t.Errorf("%s: %v, want one fill of %v", value, fills, want)
		}
	}
}

// TestTheSpecificationsConicGradientsAreOne is CSS Images 4 §3.3.3's examples.
func TestTheSpecificationsConicGradientsAreOne(t *testing.T) {
	for _, set := range [][]string{
		{
			"conic-gradient(#f06, gold)",
			"conic-gradient(at 50% 50%, #f06, gold)",
			"conic-gradient(from 0deg, #f06, gold)",
			"conic-gradient(from 0deg at center, #f06, gold)",
			"conic-gradient(#f06 0%, gold 100%)",
			"conic-gradient(#f06 0deg, gold 1turn)",
		},
		{
			"conic-gradient(white -50%, black 150%)",
			"conic-gradient(white -180deg, black 540deg)",
			"conic-gradient(hsl(0,0%,75%), hsl(0,0%,25%))",
		},
		{
			"conic-gradient(from 45deg, white, black, white)",
			"conic-gradient(hsl(0,0%,75%), white 45deg, black 225deg, hsl(0,0%,75%))",
		},
		{
			"repeating-conic-gradient(black 0deg 25%, white 0deg 50%)",
			"conic-gradient(black 25%, white 0deg 50%, black 0deg 75%, white 0deg)",
		},
	} {
		for _, other := range set[1:] {
			sameGradientPicture(t, set[0], other)
		}
	}
	// "conic-gradient(red -50%, yellow 150%) ... starts with a reddish-orange
	// color at 0deg ... and transitions to an orangish-yellow color at
	// 360deg". The specification names the two as #f50 and #fa0, which are a
	// third and two thirds of the way and are the numbers of the radial example
	// above it: 0% is a quarter of the way from -50% to 150%, and 100% three
	// quarters, and the second set of equivalences above — which the
	// specification also states, and which pass — agree with the quarters.
	g := laidGradientOf(t, "conic-gradient(red -50%, yellow 150%)")
	if c := colourIn(g, 100, 10); !near(c, rgb(255, 63.75, 0)) {
		t.Errorf("straight up from the centre the colour is %v, want a quarter of the way", c)
	}
	if c := colourIn(g, 99.99, 10); math.Abs(c.G-191.25) > 0.5 {
		t.Errorf("just short of a full turn the colour is %v, want three quarters of the way", c)
	}
	// Clockwise from up: a quarter turn is to the right.
	if c := colourIn(g, 190, 50); !near(c, mixPremultiplied(red, yellow, 0.25/2+0.25)) {
		t.Errorf("a quarter turn round the colour is %v", c)
	}
}

// TestAGradientIsReportedWhereItIsNotRead is the far side of the line: what this
// engine does not read paints nothing and says so, and what it reads is not
// reported.
func TestAGradientIsReportedWhereItIsNotRead(t *testing.T) {
	for _, tc := range []struct {
		value string
		read  bool
	}{
		{"linear-gradient(red, blue)", true},
		{"linear-gradient(red)", true}, // CSS Images 4 allows one stop
		{"radial-gradient(circle closest-corner at 10px 20px, red, blue)", true},
		{"conic-gradient(from 90deg at 10% 20%, red, blue 30deg, 50%, green)", true},
		{"linear-gradient(in lab, red, blue)", true},
		{"linear-gradient(in hsl longer hue, red, blue)", true},
		{"linear-gradient(in cmyk, red, blue)", false},
		{"linear-gradient(to right in srgb, red, blue)", true},
		{"radial-gradient(circle in srgb at 10px 10px, red, blue)", false}, // the method inside the shape
		{"radial-gradient(circle 50%, red, blue)", false},                  // Images 3: no percentage circle
		{"radial-gradient(ellipse 50px, red, blue)", false},
		{"radial-gradient(circle 10px 20px, red, blue)", false},
		{"linear-gradient(calc(10deg), red, blue)", false},
		{"linear-gradient(red, 20%, 40%, blue)", false}, // two hints in a row
		{"linear-gradient(20%, red, blue)", false},      // a hint first
		{"linear-gradient(red, blue, 20%)", false},      // and last
		{"linear-gradient(red 1px 2px 3px, blue)", false},
		{"linear-gradient(to left left, red, blue)", false},
		{"conic-gradient(at 10px 10px from 30deg, red, blue)", false},
		{"linear-gradient()", false},
	} {
		built := Build(Input{
			HTML: `<div id="d"></div>`,
			CSS: []Stylesheet{{Source: `#d { width: 100px; height: 50px;
				background-image: ` + tc.value + ` }`}},
		})
		rec := NewRecorder(nil)
		ops := Paint(Layout(built.Root, Size{W: bgpx(600), H: bgpx(1000)}, nil, rec))
		painted := false
		for _, op := range ops {
			switch op.(type) {
			case FillGradient, FillRect:
				painted = true
			}
		}
		reported := hasRule(rec.Findings(), RuleUnsupportedValue)
		if painted != tc.read || reported == tc.read {
			t.Errorf("%s: painted=%v reported=%v, want read=%v", tc.value, painted, reported, tc.read)
		}
	}
}

// TestTheGradientBoundsFire lowers each bound until an ordinary gradient trips
// it, and requires the trip to be reported and to say what was not done.
func TestTheGradientBoundsFire(t *testing.T) {
	func() {
		was := maxGradientStops
		defer func() { maxGradientStops = was }()
		maxGradientStops = 3
		built := Build(Input{HTML: `<div id="d"></div>`, CSS: []Stylesheet{{Source: `#d { width: 100px;
			height: 50px; background-image: linear-gradient(red, blue, green, 30%, yellow) }`}}})
		rec := NewRecorder(nil)
		ops := Paint(Layout(built.Root, Size{W: bgpx(600), H: bgpx(1000)}, nil, rec))
		for _, op := range ops {
			if _, ok := op.(FillGradient); ok {
				t.Error("a gradient past the stop bound was drawn")
			}
		}
		if !reportedLimit(rec.Findings(), "colour stops") {
			t.Errorf("the stop bound fired silently: %v", rec.Findings())
		}
		// Two arguments, and four stops once each is read as the two it is.
		built = Build(Input{HTML: `<div id="d"></div>`, CSS: []Stylesheet{{Source: `#d { width: 100px;
			height: 50px; background-image: linear-gradient(red 0 10%, blue 20% 30%) }`}}})
		rec = NewRecorder(nil)
		ops = Paint(Layout(built.Root, Size{W: bgpx(600), H: bgpx(1000)}, nil, rec))
		for _, op := range ops {
			if _, ok := op.(FillGradient); ok {
				t.Error("a gradient of double-position stops past the bound was drawn")
			}
		}
		if !reportedLimit(rec.Findings(), "colour stops") {
			t.Errorf("the stop bound fired silently for double positions: %v", rec.Findings())
		}
	}()
	func() {
		was := maxGradientRepeats
		defer func() { maxGradientRepeats = was }()
		maxGradientRepeats = 8
		built := Build(Input{HTML: `<div id="d"></div>`, CSS: []Stylesheet{{Source: `#d { width: 100px;
			height: 50px; background-image: repeating-linear-gradient(red, blue 5px) }`}}})
		rec := NewRecorder(nil)
		ops := Paint(Layout(built.Root, Size{W: bgpx(600), H: bgpx(1000)}, nil, rec))
		avg := false
		for _, op := range ops {
			switch v := op.(type) {
			case FillGradient:
				t.Error("a gradient past the repeat bound was drawn as a gradient")
			case FillRect:
				avg = avg || near(v.Color, rgb(127.5, 0, 127.5))
			}
		}
		if !avg {
			t.Errorf("the gradient was not drawn as its average colour: %v", ops)
		}
		if !reportedLimit(rec.Findings(), "average colour") {
			t.Errorf("the repeat bound fired silently: %v", rec.Findings())
		}
	}()
}

func reportedLimit(findings []Finding, says string) bool {
	for _, f := range findings {
		if f.Rule == RuleLimit && strings.Contains(f.Message, says) {
			return true
		}
	}
	return false
}

// TestAGradientTakesTheOpacityAroundIt: a gradient carries colours, so a
// translucent box's alpha is folded into every stop, and exactly.
func TestAGradientTakesTheOpacityAroundIt(t *testing.T) {
	g, ok := tryGradientOf(t, "linear-gradient(red, rgba(0, 0, 255, 0.5))", "opacity: 0.5")
	if !ok {
		t.Fatal("no gradient was painted")
	}
	if a, b := g.Gradient.Stops[0].Color.A, g.Gradient.Stops[1].Color.A; a != 0.5 || b != 0.25 {
		t.Errorf("the stops' alphas are %g and %g, want 0.5 and 0.25", a, b)
	}
	if _, ok := tryGradientOf(t, "linear-gradient(red, blue)", "opacity: 0"); ok {
		t.Error("a gradient in a box of opacity 0 was painted")
	}
}

// TestAGradientIsClippedLikeATiling: "overflow: hidden" narrows the area the
// gradient may paint, and leaves the tiles where they were, so a cut gradient
// is the same gradient showing less.
func TestAGradientIsClippedLikeATiling(t *testing.T) {
	ops := paintOf(t, `<div id="o"><div id="d"></div></div>`, noDefaults+`
		#o { width: 50px; height: 40px; overflow: hidden }
		#d { width: 200px; height: 100px; background-image: linear-gradient(red, blue) }`)
	for _, op := range ops {
		if g, ok := op.(FillGradient); ok {
			if g.Clip != (Rect{W: bgpx(50), H: bgpx(40)}) {
				t.Errorf("the gradient paints %v, want the 50 by 40 the clip leaves", g.Clip)
			}
			if g.Tile != (Rect{W: bgpx(200), H: bgpx(100)}) {
				t.Errorf("the tile moved to %v", g.Tile)
			}
			return
		}
	}
	t.Fatal("no gradient was painted")
}

// TestAnInlineGradientIsAnOverhang: the background of an inline box is marked as
// one, gradient or not, so the overflow-page guardrail reads the two alike.
func TestAnInlineGradientIsAnOverhang(t *testing.T) {
	ops := paintOf(t, `<p>a <span id="s">word</span></p>`, noDefaults+`
		#s { background-image: linear-gradient(red, blue) }`)
	for _, op := range ops {
		if g, ok := op.(FillGradient); ok {
			if !g.Overhang {
				t.Error("an inline box's gradient is not marked as an overhang")
			}
			return
		}
	}
	t.Fatal("no gradient was painted")
}

// TestAGradientIsTiledByBackgroundSize: the gradient box is the tile, so a
// repeated gradient starts again in each tile and one value is one operation.
func TestAGradientIsTiledByBackgroundSize(t *testing.T) {
	g, ok := tryGradientOf(t, "linear-gradient(to right, red, blue)", "background-size: 50px 20px")
	if !ok {
		t.Fatal("no gradient was painted")
	}
	if cols, rows := g.Tiles(); cols != 4 || rows != 5 {
		t.Errorf("%d by %d tiles, want 4 by 5", cols, rows)
	}
	if c := colourIn(g, 25, 10); !near(c, rgb(127.5, 0, 127.5)) {
		t.Errorf("the middle of a tile is %v, want the even blend", c)
	}
}
