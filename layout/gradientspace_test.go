package layout

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// Gradients interpolated in a colour space: CSS Color 4 §13 and §14.2, and
// how they are restated as sRGB stops. See colorspace.go and gradientspace.go.

func nearVec(t *testing.T, what string, got, want vec3, tol float64) {
	t.Helper()
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol {
			t.Errorf("%s is %v, want %v (within %v)", what, got, want, tol)
			return
		}
	}
}

// pct is a colour written with percentages, as fractions.
func pct(r, g, b float64) vec3 { return vec3{r / 100, g / 100, b / 100} }

// TestTheMatricesAreTheSpecsAndConsistent checks §19's matrices as transcribed
// against what they are made from, so that a mistyped digit cannot hide: each
// pair is inverse to the other, sRGB's white is D65's four-figure
// chromaticities, and the Bradford adaptation, rebuilt here from the Bradford
// cone matrix and the two whites, is the matrix transcribed. Oklab is held to
// the table of example values Björn Ottosson published with it, to its three
// decimals.
func TestTheMatricesAreTheSpecsAndConsistent(t *testing.T) {
	product := func(a, b *[3][3]float64) (out [3][3]float64) {
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				for k := 0; k < 3; k++ {
					out[i][j] += a[i][k] * b[k][j]
				}
			}
		}
		return out
	}
	identity := func(what string, m [3][3]float64, tol float64) {
		t.Helper()
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				want := 0.0
				if i == j {
					want = 1
				}
				if math.Abs(m[i][j]-want) > tol {
					t.Errorf("%s is not the identity: %v", what, m)
					return
				}
			}
		}
	}
	identity("sRGB there and back", product(&xyzToLinSRGB, &linSRGBToXYZ), 1e-12)
	identity("Display P3 there and back", product(&xyzToLinP3, &linP3ToXYZ), 1e-12)
	identity("A98 there and back", product(&xyzToLinA98, &linA98ToXYZ), 1e-12)
	identity("Rec. 2020 there and back", product(&xyzToLin2020, &lin2020ToXYZ), 1e-12)
	identity("ProPhoto there and back", product(&xyzToLinProPhoto, &linProPhotoToXYZ), 1e-12)
	identity("D65 to D50 and back", product(&d50ToD65, &d65ToD50), 1e-12)
	identity("LMS there and back", product(&lmsToXYZ, &xyzToLMS), 1e-12)
	identity("Oklab there and back", product(&oklabToLMS, &lmsToOklab), 1e-12)

	d65 := vec3{0.3127 / 0.3290, 1, (1 - 0.3127 - 0.3290) / 0.3290}
	nearVec(t, "sRGB white", mul3(&linSRGBToXYZ, vec3{1, 1, 1}), d65, 1e-12)
	nearVec(t, "Display P3 white", mul3(&linP3ToXYZ, vec3{1, 1, 1}), d65, 1e-12)
	nearVec(t, "D65 adapted to D50", mul3(&d65ToD50, d65), d50White, 1e-12)
	bradford := [3][3]float64{{0.8951, 0.2664, -0.1614}, {-0.7502, 1.7135, 0.0367}, {0.0389, -0.0685, 1.0296}}
	inverse := func(m [3][3]float64) (out [3][3]float64) {
		det := m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) - m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
			m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				a, b := (j+1)%3, (j+2)%3
				c, d := (i+1)%3, (i+2)%3
				out[i][j] = (m[a][c]*m[b][d] - m[a][d]*m[b][c]) / det
			}
		}
		return out
	}
	src, dst := mul3(&bradford, d65), mul3(&bradford, d50White)
	scale := [3][3]float64{{dst[0] / src[0]}, {0, dst[1] / src[1]}, {0, 0, dst[2] / src[2]}}
	inv := inverse(bradford)
	rebuilt := product(&inv, &scale)
	rebuilt = product(&rebuilt, &bradford)
	for i := 0; i < 3; i++ {
		nearVec(t, "the Bradford adaptation", vec3(d65ToD50[i]), vec3(rebuilt[i]), 1e-9)
	}
	for _, tc := range [][2]vec3{
		{{0.950, 1.000, 1.089}, {1.000, 0.000, 0.000}},
		{{1, 0, 0}, {0.450, 1.236, -0.019}},
		{{0, 1, 0}, {0.922, -0.671, 0.263}},
		{{0, 0, 1}, {0.153, -1.415, -0.449}},
	} {
		nearVec(t, fmt.Sprintf("XYZ %v in Oklab", tc[0]), oklabOfXYZ(tc[0]), tc[1], 0.0015)
	}
}

// TestTheConversionsAreTheSpecsSampleValues holds the conversions to the values
// CSS Color 4 publishes: §9's "sRGB blue is lab(29.567% 68.298 -112.0294)
// while sRGB yellow is lab(97.607% -15.753 93.388)", and §13.4's two colours in
// Lab and LCH, one of them in Display P3. Those worked examples were computed
// before §19's matrices were recalculated to 64-bit precision (the draft's own
// change notes), and their last digits differ from what the current matrices
// give by up to 0.014; they are held to 0.02, and the matrices themselves to
// their construction by TestTheMatricesAreTheSpecsAndConsistent.
func TestTheConversionsAreTheSpecsSampleValues(t *testing.T) {
	nearVec(t, "sRGB blue in Lab", spaceLab.fromSRGB(vec3{0, 0, 1}), vec3{29.567, 68.298, -112.0294}, 0.02)
	nearVec(t, "sRGB yellow in Lab", spaceLab.fromSRGB(vec3{1, 1, 0}), vec3{97.607, -15.753, 93.388}, 0.02)
	nearVec(t, "rgb(76% 62% 3%) in Lab", spaceLab.fromSRGB(pct(76, 62, 3)), vec3{66.927, 4.873, 68.622}, 0.02)
	nearVec(t, "rgb(76% 62% 3%) in LCH", spaceLCH.fromSRGB(pct(76, 62, 3)), vec3{66.93, 68.79, 85.94}, 0.02)
	p3 := spaceDisplayP3.toXYZ(vec3{0.84, 0.19, 0.72})
	nearVec(t, "color(display-p3 0.84 0.19 0.72) in Lab", spaceLab.fromXYZ(p3), vec3{53.503, 82.672, -33.901}, 0.02)
	nearVec(t, "color(display-p3 0.84 0.19 0.72) in LCH", spaceLCH.fromXYZ(p3), vec3{53.5, 89.35, 337.7}, 0.02)
	// Oklab's matrices are chosen so that white is L = 1 with no chroma, and
	// black is nothing.
	nearVec(t, "white in Oklab", spaceOklab.fromSRGB(vec3{1, 1, 1}), vec3{1, 0, 0}, 1e-6)
	nearVec(t, "black in Oklab", spaceOklab.fromSRGB(vec3{0, 0, 0}), vec3{0, 0, 0}, 1e-12)
	// §7 and §8's hues, whitenesses and blacknesses.
	nearVec(t, "sRGB orange in HSL", spaceHSL.fromSRGB(vec3{1, 0.5, 0}), vec3{30, 1, 0.5}, 1e-12)
	// max is green: hue = 60 × ((b − r)/(max − min) + 2) = 60 × (−2/3 + 2).
	nearVec(t, "rgb(60% 80% 20%) in HWB", spaceHWB.fromSRGB(pct(60, 80, 20)), vec3{80, 0.2, 0.2}, 1e-12)
}

// TestEverySpaceGoesThereAndBack: a colour inside sRGB converted to each space
// and back is itself, which is what makes restating a gradient's end stops in
// sRGB exact. It is asked of colours at the corners and the middle of the
// cube, where the transfer functions' linear pieces and the hue's cases are.
func TestEverySpaceGoesThereAndBack(t *testing.T) {
	colours := []vec3{{0, 0, 0}, {1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 0},
		{0, 1, 1}, {1, 0, 1}, {0.5, 0.5, 0.5}, {0.02, 0.03, 0.01}, {0.8, 0.3, 0.6}, {0.25, 0.9, 0.4}}
	for s := spaceSRGB; s <= spaceOklch; s++ {
		for _, c := range colours {
			back := s.toSRGB(s.fromSRGB(c))
			nearVec(t, fmt.Sprintf("space %d: %v there and back", s, c), back, c, 1e-9)
		}
	}
}

// TestAPowerlessHueIsMissing is §19's thresholds: a colour with no chroma, or
// no saturation, or all white and black, has a powerless hue, which the
// conversion reports as missing.
func TestAPowerlessHueIsMissing(t *testing.T) {
	for _, tc := range []struct {
		s    colorSpace
		c    vec3
		want bool
	}{
		{spaceHSL, vec3{0.5, 0.5, 0.5}, true}, {spaceHSL, vec3{1, 1, 1}, true}, {spaceHSL, vec3{0, 0, 0}, true},
		{spaceHWB, vec3{0.3, 0.3, 0.3}, true}, {spaceLCH, vec3{0.4, 0.4, 0.4}, true},
		{spaceOklch, vec3{0.7, 0.7, 0.7}, true}, {spaceOklch, vec3{1, 0, 0}, false},
		{spaceHSL, vec3{1, 0, 0}, false},
	} {
		got := hueIn(tc.s, tc.c)
		if math.IsNaN(got) != tc.want {
			t.Errorf("space %d: %v has hue %v, want missing=%v", tc.s, tc.c, got, tc.want)
		}
	}
}

// hueIn is a colour's hue in a polar space.
func hueIn(space colorSpace, c vec3) float64 { return space.fromSRGB(c)[space.hueIndex()] }

// TestInterpolationIsTheSpecsExamples is §13.4's premultiplied midpoints and
// §13.5's four hue methods, to the digits they print.
func TestInterpolationIsTheSpecsExamples(t *testing.T) {
	rgba := func(c vec3, a float64) style.RGBA { return rgbaOf(c, a) }
	// sRGB: rgb(24% 12% 98% / 0.4) and rgb(62% 26% 64% / 0.6) meet at
	// rgb(46.8% 20.4% 77.6% / 0.5).
	m := newColourMix(spaceSRGB, hueShorter, rgba(pct(24, 12, 98), 0.4), rgba(pct(62, 26, 64), 0.6))
	got, a := m.coordsAt(0.5)
	nearVec(t, "the sRGB midpoint", got, pct(46.8, 20.4, 77.6), 1e-9)
	if math.Abs(a-0.5) > 1e-12 {
		t.Errorf("the sRGB midpoint's alpha is %v", a)
	}
	// Lab: rgb(76% 62% 03% / 0.4) and color(display-p3 0.84 0.19 0.72 / 0.6)
	// meet at lab(58.873% 51.552 7.108) / 0.5, and in LCH, along the shorter
	// arc, at lch(58.873% 81.126 31.82) / 0.5. The second colour is outside
	// sRGB, and is given in sRGB's extended range.
	p3 := srgbOfXYZ(spaceDisplayP3.toXYZ(vec3{0.84, 0.19, 0.72}))
	m = newColourMix(spaceLab, hueShorter, rgba(pct(76, 62, 3), 0.4), rgba(p3, 0.6))
	got, _ = m.coordsAt(0.5)
	nearVec(t, "the Lab midpoint", got, vec3{58.873, 51.552, 7.108}, 0.02)
	m = newColourMix(spaceLCH, hueShorter, rgba(pct(76, 62, 3), 0.4), rgba(p3, 0.6))
	got, _ = m.coordsAt(0.5)
	nearVec(t, "the LCH midpoint", got, vec3{58.873, 81.126, 31.82}, 0.02)

	// §13.5's hue examples, as hues.
	for _, tc := range []struct {
		m        hueMethod
		h1, h2   float64
		midpoint float64
	}{
		{hueShorter, 30, 90, 60},
		{hueLonger, 30, 90, 240},
		{hueIncreasing, 30, 190, 110},
		{hueIncreasing, 30, 230, 130},
		{hueDecreasing, 30, 190, 290},
		{hueDecreasing, 30, 230, 310},
		// And the cases between the examples: a shorter arc through zero,
		// and a longer one between equal hues, which is a whole turn.
		{hueShorter, 350, 10, 0},
		{hueLonger, 100, 100, 280},
	} {
		a, b := fixHues(tc.h1, tc.h2, tc.m)
		if mid := math.Mod((a+b)/2+360, 360); math.Abs(mid-tc.midpoint) > 1e-9 {
			t.Errorf("method %d from %v to %v: the midpoint is %v, want %v", tc.m, tc.h1, tc.h2, mid, tc.midpoint)
		}
	}
}

// restatedOf is the one gradient a value paints on a 200 by 100 box, as the
// operation states it.
func restatedOf(t *testing.T, value string) Gradient {
	t.Helper()
	return laidGradientOf(t, value).Gradient
}

// premultipliedOff is how far two colours are apart, as the tolerance measures.
func premultipliedOff(a, b style.RGBA) float64 { return premultipliedDistance(a, b) }

// TestHSLLongerHueIsTheSixColours is the suite's gradient-longer-hue-hsl-001
// with an oracle that does not share this engine's arithmetic: red to blue the
// long way round HSL passes yellow, lime and cyan at the quarters, and between
// any two of them a fully saturated HSL hue is a straight line in sRGB. So the
// gradient is the sRGB gradient of those five colours, everywhere.
func TestHSLLongerHueIsTheSixColours(t *testing.T) {
	got := restatedOf(t, `linear-gradient(to right in hsl longer hue, red, blue)`)
	want := restatedOf(t, `linear-gradient(to right, red, yellow, lime, cyan, blue)`)
	worst := 0.0
	for k := 0; k <= 4000; k++ {
		x := float64(k) / 4000
		worst = math.Max(worst, premultipliedOff(got.ColorAtOffset(x), want.ColorAtOffset(x)))
	}
	if worst > interpolationTolerance {
		t.Errorf("the longer hue is %.5f from the six colours at worst, want within %.5f",
			worst, interpolationTolerance)
	}
}

// TestSRGBLinearIsXYZ: both spaces are linear light, one a matrix of the other,
// so interpolating in either is the same picture — and the true colour at each
// point is the linear mix gamma-encoded, which is computed here from the sRGB
// transfer function alone.
func TestSRGBLinearIsXYZ(t *testing.T) {
	lin := restatedOf(t, `linear-gradient(to right in srgb-linear, rgb(255, 0, 0), rgb(0, 255, 0))`)
	xyz := restatedOf(t, `linear-gradient(to right in xyz, rgb(255, 0, 0), rgb(0, 255, 0))`)
	encode := func(v float64) float64 {
		if v <= 0.0031308 {
			return 12.92 * v
		}
		return 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	for k := 0; k <= 2000; k++ {
		x := float64(k) / 2000
		truth := style.RGBA{R: 255 * encode(1-x), G: 255 * encode(x), A: 1}
		for name, g := range map[string]Gradient{"srgb-linear": lin, "xyz": xyz} {
			if d := premultipliedOff(g.ColorAtOffset(x), truth); d > interpolationTolerance {
				t.Fatalf("%s at %.4f is %v, want %v (%.5f off)", name, x, g.ColorAtOffset(x), truth, d)
			}
		}
	}
}

// TestTheIncreasingHueEquivalences are the suite's gradient-increasing-hue-hsl
// and gradient-decreasing-hue-lch: a gradient whose hue increases (or
// decreases) from one stop to the next is the same gradient with stops put in
// at the hues it passes.
func TestTheIncreasingHueEquivalences(t *testing.T) {
	for _, tc := range [][2]string{
		{`linear-gradient(to right in hsl increasing hue, hsl(0deg, 100%, 50%), hsl(40deg, 100%, 50%))`,
			`linear-gradient(to right in hsl increasing hue, hsl(0deg, 100%, 50%), hsl(10deg, 100%, 50%), hsl(20deg, 100%, 50%), hsl(30deg, 100%, 50%), hsl(40deg, 100%, 50%))`},
		{`linear-gradient(to right in hsl increasing hue, hsl(40deg, 100%, 50%), hsl(0deg, 100%, 50%))`,
			`linear-gradient(to right in hsl increasing hue, hsl(40deg, 100%, 50%), hsl(120deg, 100%, 50%), hsl(200deg, 100%, 50%), hsl(280deg, 100%, 50%), hsl(360deg, 100%, 50%))`},
		{`linear-gradient(to right in hsl decreasing hue, hsl(0deg, 100%, 50%), hsl(270deg, 100%, 50%))`,
			`linear-gradient(to right in hsl decreasing hue, hsl(0deg, 100%, 50%), hsl(270deg, 100%, 50%))`},
		{`linear-gradient(to right in hsl increasing hue, hsl(270deg, 100%, 50%), hsl(0deg, 100%, 50%))`,
			`linear-gradient(to right in hsl increasing hue, hsl(270deg, 100%, 50%), hsl(300deg, 100%, 50%), hsl(330deg, 100%, 50%), hsl(360deg, 100%, 50%))`},
		{`linear-gradient(to right in lch decreasing hue, lime, blue)`,
			`linear-gradient(to right in lch longer hue, lime, blue)`},
	} {
		a, b := restatedOf(t, tc[0]), restatedOf(t, tc[1])
		for k := 0; k <= 2000; k++ {
			x := float64(k) / 2000
			if d := premultipliedOff(a.ColorAtOffset(x), b.ColorAtOffset(x)); d > 2*interpolationTolerance {
				t.Fatalf("%s\nand %s\ndiffer at %.4f by %.5f", tc[0], tc[1], x, d)
			}
		}
	}
}

// TestAPowerlessHueTakesTheOthers is the suite's gradient-powerless-hue-hsl:
// "transparent" is black with no saturation, whose hue is powerless, so red to
// transparent in HSL keeps red's hue and fades; a transparent green has a hue,
// and red to it passes yellow at the midpoint, half faded.
func TestAPowerlessHueTakesTheOthers(t *testing.T) {
	g := restatedOf(t, `linear-gradient(to right in hsl, red, transparent)`)
	if c := g.ColorAtOffset(0.5); premultipliedOff(c, style.RGBA{R: 255, A: 0.5}) > interpolationTolerance {
		t.Errorf("red to transparent in hsl is %v half-way, want red half faded", c)
	}
	g = restatedOf(t, `linear-gradient(to right in hsl, red, rgba(0, 255, 0, 0))`)
	if c := g.ColorAtOffset(0.5); premultipliedOff(c, style.RGBA{R: 255, G: 255, A: 0.5}) > interpolationTolerance {
		t.Errorf("red to transparent lime in hsl is %v half-way, want yellow half faded", c)
	}
	// Either end may be the one missing its hue, and the hue taken is the
	// other's, whatever it is: blue's is 240.
	for _, v := range []string{`linear-gradient(to right in hsl, transparent, blue)`,
		`linear-gradient(to left in hsl, blue, transparent)`} {
		g = restatedOf(t, v)
		if c := g.ColorAtOffset(0.5); premultipliedOff(c, style.RGBA{B: 255, A: 0.5}) > interpolationTolerance {
			t.Errorf("%s is %v half-way, want blue half faded", v, c)
		}
	}
	// White has no chroma, so in OkLCh it takes blue's hue, and every colour
	// between them is of that hue.
	blueHue := hueIn(spaceOklch, vec3{0, 0, 1})
	m := newColourMix(spaceOklch, hueShorter, style.RGBA{R: 255, G: 255, B: 255, A: 1}, style.RGBA{B: 255, A: 1})
	for k := 0; k <= 100; k++ {
		if c, _ := m.coordsAt(float64(k) / 100); math.Abs(c[2]-blueHue) > 1e-9 {
			t.Fatalf("white to blue in oklch has hue %v at %d%%, want blue's %v", c[2], k, blueHue)
		}
	}
}

// TestALongerHueBetweenOneColourIsAWholeTurn: two stops of one colour are one
// colour in any space but the longer way round a hue, which is the whole
// circle — so such a gradient is not taken for a fill.
func TestALongerHueBetweenOneColourIsAWholeTurn(t *testing.T) {
	g := restatedOf(t, `linear-gradient(to right in hsl longer hue, red, red)`)
	if c := g.ColorAtOffset(0.5); premultipliedOff(c, style.RGBA{G: 255, B: 255, A: 1}) > interpolationTolerance {
		t.Errorf("red to red the long way is %v half-way, want cyan", c)
	}
	if _, ok := tryGradientOf(t, `linear-gradient(in hsl, red, red)`, ""); ok {
		t.Error("red to red the short way was drawn as a gradient, not a fill")
	}
}

// TestTheStatedToleranceHolds is gradientspace.go's claim, asked of a range of
// gradients in every space: at every one of four thousand points along each,
// the stops restated in sRGB are within half an 8-bit step of the true colour,
// computed afresh from the colour mix and the stops' exponent.
func TestTheStatedToleranceHolds(t *testing.T) {
	values := []string{
		`linear-gradient(to right in oklab, red, lime)`,
		`linear-gradient(to right in oklch longer hue, red 10%, 40%, blue, rgba(0, 128, 0, 0.3))`,
		`linear-gradient(to right in lch decreasing hue, #f80, 70%, #08f)`,
		`linear-gradient(to right in lab, white, transparent, black)`,
		`linear-gradient(to right in display-p3, red, blue)`,
		`linear-gradient(to right in display-p3-linear, yellow, purple)`,
		`linear-gradient(to right in a98-rgb, teal, orange)`,
		`linear-gradient(to right in prophoto-rgb, navy, 20%, gold)`,
		`linear-gradient(to right in rec2020, red, cyan)`,
		`linear-gradient(to right in xyz-d50, maroon, lime)`,
		`linear-gradient(to right in hwb increasing hue, #c00, #0c0 60%, #00c)`,
		`linear-gradient(to right in oklch, black, white)`,
	}
	space := map[string]colorSpace{"oklab": spaceOklab, "oklch": spaceOklch, "lch": spaceLCH, "lab": spaceLab,
		"display-p3": spaceDisplayP3, "display-p3-linear": spaceDisplayP3Linear, "a98-rgb": spaceA98RGB,
		"prophoto-rgb": spaceProPhotoRGB, "rec2020": spaceRec2020, "xyz-d50": spaceXYZD50, "hwb": spaceHWB}
	hue := map[string]hueMethod{"longer": hueLonger, "decreasing": hueDecreasing, "increasing": hueIncreasing}
	for _, v := range values {
		// The method, and the same gradient without it, whose stops are the
		// author's fixed up and left as they are.
		words := strings.Fields(strings.NewReplacer(",", " ", "(", " ").Replace(v))
		var sp colorSpace
		hm := hueShorter
		method := ""
		for i, w := range words {
			if w == "in" {
				sp, method = space[words[i+1]], " in "+words[i+1]
				if i+3 < len(words) && words[i+3] == "hue" {
					hm, method = hue[words[i+2]], method+" "+words[i+2]+" hue"
				}
			}
		}
		orig := restatedOf(t, strings.Replace(v, method, "", 1)).Stops
		g := restatedOf(t, v)
		worst, at := 0.0, 0.0
		for k := 0; k <= 4000; k++ {
			x := float64(k) / 4000
			truth := trueColourAt(orig, sp, hm, x)
			if d := premultipliedOff(g.ColorAtOffset(x), truth); d > worst {
				worst, at = d, x
			}
		}
		if worst > interpolationTolerance {
			t.Errorf("%s is %.6f off at %.4f, past the stated %.6f", v, worst, at, interpolationTolerance)
		}
		t.Logf("%s: %d stops, %.6f at worst", v, len(g.Stops), worst)
	}
}

// trueColourAt is the colour §13 gives a gradient's stops at an offset,
// worked out directly: the segment that holds it, its exponent, and the mix.
func trueColourAt(stops []GradientStop, s colorSpace, h hueMethod, x float64) style.RGBA {
	if x <= stops[0].Offset {
		return stops[0].Color
	}
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if x >= b.Offset {
			continue
		}
		p := (x - a.Offset) / (b.Offset - a.Offset)
		return newColourMix(s, h, a.Color, b.Color).at(math.Pow(p, b.Exponent))
	}
	return stops[len(stops)-1].Color
}

// TestAColourOutsideSRGBIsGamutMapped is §14.2.2: the oklch longer way from red
// to blue passes colours no sRGB device can show, and each is brought inside
// sRGB at its own lightness and hue with its chroma reduced, so every stop is
// in gamut and a sample keeps the hue it was interpolated at to within the
// clipping the algorithm allows.
func TestAColourOutsideSRGBIsGamutMapped(t *testing.T) {
	g := restatedOf(t, `linear-gradient(to right in oklch longer hue, red, blue)`)
	for _, st := range g.Stops {
		for _, v := range []float64{st.Color.R, st.Color.G, st.Color.B} {
			if v < 0 || v > 255 {
				t.Fatalf("a stop is out of sRGB: %v", st.Color)
			}
		}
	}
	// In gamut, a colour is unchanged; past white and black it is white and
	// black; out of gamut the result is in gamut, of about the lightness and
	// hue asked for, and of less chroma.
	nearVec(t, "an sRGB colour", gamutMapSRGB(spaceSRGB, vec3{0.2, 0.4, 0.6}), vec3{0.2, 0.4, 0.6}, 0)
	nearVec(t, "lightness past white", gamutMapSRGB(spaceOklch, vec3{1.2, 0.3, 40}), vec3{1, 1, 1}, 0)
	nearVec(t, "lightness past black", gamutMapSRGB(spaceOklch, vec3{-0.1, 0.3, 40}), vec3{0, 0, 0}, 0)
	// "greater than or equal to", which at a lightness of exactly one or
	// nothing is the colour white or black and not the search's result.
	nearVec(t, "the lightness of white", gamutMapSRGB(spaceOklch, vec3{1, 0.3, 40}), vec3{1, 1, 1}, 0)
	nearVec(t, "the lightness of black", gamutMapSRGB(spaceOklch, vec3{0, 0.3, 40}), vec3{0, 0, 0}, 0)
	for _, c := range []vec3{{0.7, 0.4, 30}, {0.5, 0.35, 260}, {0.9, 0.3, 140}, {0.3, 0.3, 320}} {
		got := gamutMapSRGB(spaceOklch, c)
		if !inSRGB(got) {
			t.Errorf("oklch%v mapped to %v, outside sRGB", c, got)
		}
		lch := spaceOklch.fromSRGB(got)
		if math.Abs(lch[0]-c[0]) > 0.02 || lch[1] >= c[1] || math.Abs(math.Remainder(lch[2]-c[2], 360)) > 5 {
			t.Errorf("oklch%v mapped to oklch%v", c, lch)
		}
	}
}

// TestInterpolatingIsBoundedAndReported: past maxInterpolatedStops the gradient
// is not drawn and says so, and the work is charged to the document's budget.
func TestInterpolatingIsBoundedAndReported(t *testing.T) {
	old := maxInterpolatedStops
	maxInterpolatedStops = 8
	t.Cleanup(func() { maxInterpolatedStops = old })
	value := `linear-gradient(in oklch longer hue, red, blue)`
	if _, ok := tryGradientOf(t, value, ""); ok {
		t.Error("a gradient past the bound was drawn")
	}
	fs := paintFindingsOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 200px; height: 100px; background-image: `+value+` }`)
	found := false
	for _, f := range fs {
		found = found || (f.Rule == RuleLimit && strings.Contains(f.Message, "colour stops this engine draws"))
	}
	if !found {
		t.Errorf("nothing said the gradient was past the bound: %v", fs)
	}
	maxInterpolatedStops = old

	lowWork(t, 30*costColour)
	built := Build(Input{HTML: `<div id="d"></div>`, CSS: []Stylesheet{{Source: noDefaults +
		`#d { width: 200px; height: 100px; background-image: ` + value + ` }`}}})
	rec := NewRecorder(nil)
	PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, rec), rec)
	requireCut(t, rec.Findings(), "the colour interpolation of the gradients past that point, which were not drawn")
}

// TestOneGradientOnManyBoxesIsInterpolatedOnce: a rule that puts one gradient on
// many boxes of one size restates its stops once, and charges the document
// once for it.
func TestOneGradientOnManyBoxesIsInterpolatedOnce(t *testing.T) {
	spent := func(n int) int64 {
		built := Build(Input{HTML: strings.Repeat(`<div></div>`, n), CSS: []Stylesheet{{Source: noDefaults +
			`div { width: 200px; height: 10px; background-image: linear-gradient(in oklch longer hue, red, blue) }`}}})
		rec := NewRecorder(nil)
		before := rec.work.left
		PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(10000)}, nil, rec), rec)
		return before - rec.work.left
	}
	one, ten := spent(1), spent(10)
	evaluations := 0
	stops := []GradientStop{{Offset: 0, Color: style.RGBA{R: 255, A: 1}, Exponent: 1},
		{Offset: 1, Color: style.RGBA{B: 255, A: 1}, Exponent: 1}}
	interpolateStops(stops, spaceOklch, hueLonger, func() bool { evaluations++; return true })
	if ten-one >= int64(evaluations)*costColour {
		t.Errorf("nine more boxes cost %d steps, as much as restating the gradient again (%d)",
			ten-one, int64(evaluations)*costColour)
	}
}

// TestInterpolatingCostsItsStops: a gradient of four times the stops, in a
// space every segment of which is cut, works out about four times as many
// colours, counted by what it is charged, and takes about four times as long.
func TestInterpolatingCostsItsStops(t *testing.T) {
	stopsOf := func(n int) []GradientStop {
		out := make([]GradientStop, n)
		for i := range out {
			out[i] = GradientStop{Offset: float64(i) / float64(n-1), Exponent: 1,
				Color: style.RGBA{R: float64(i%2) * 255, B: float64((i+1)%2) * 255, A: 1}}
		}
		return out
	}
	small, large := stopsOf(2), stopsOf(5)
	count := func(stops []GradientStop) int64 {
		n := int64(0)
		interpolateStops(stops, spaceOklch, hueLonger, func() bool { n++; return true })
		return n
	}
	if r := costtest.Count(t, "colours worked out restating stops in oklch", count(small), count(large)); r > 8 {
		t.Errorf("four times the stops worked out %.1f times the colours; want about four", r)
	}
	c := costtest.Time(t, "restating stops in oklch",
		func() { interpolateStops(small, spaceOklch, hueLonger, nil) },
		func() { interpolateStops(large, spaceOklch, hueLonger, nil) })
	if c.Ratio > 8 {
		t.Errorf("four times the stops cost %.1f times as much; want about four", c.Ratio)
	}
}

// TestTheInterpolationMethodIsRead: every space and hue method CSS Color 4
// §13.2 names, before or after the rest of the first argument, and what is not
// one refused.
func TestTheInterpolationMethodIsRead(t *testing.T) {
	for _, v := range []string{
		`linear-gradient(in oklab, red, blue)`,
		`linear-gradient(to right in lch longer hue, red, blue)`,
		`linear-gradient(in hsl decreasing hue to left, red, blue)`,
		`linear-gradient(45deg in xyz-d65, red, blue)`,
		`radial-gradient(circle in srgb-linear, red, blue)`,
		`radial-gradient(in oklch shorter hue, red, blue)`,
		`conic-gradient(from 10deg in hwb increasing hue, red, blue)`,
		`repeating-linear-gradient(in display-p3, red, blue 20px)`,
	} {
		if _, ok := tryGradientOf(t, v, ""); !ok {
			t.Errorf("%s was not drawn", v)
		}
	}
	for _, v := range []string{
		`linear-gradient(in oklab longer hue, red, blue)`,
		`linear-gradient(in lch longer, red, blue)`,
		`linear-gradient(in lch longer shade, red, blue)`,
		`linear-gradient(in cmyk, red, blue)`,
		`linear-gradient(in, red, blue)`,
		`linear-gradient(in hsl in oklab, red, blue)`,
		`linear-gradient(in hsl to right in oklab, red, blue)`,
	} {
		if _, ok := tryGradientOf(t, v, ""); ok {
			t.Errorf("%s was drawn", v)
		}
	}
}

// TestRestatedStopsKeepTheirOffsets: a segment's last stop is at the author's
// offset exactly, not at the first's plus the span, which a float need not add
// back to — 0.15 + (0.45 − 0.15) is 0.45000000000000007 — and which would put
// it past a hard stop at 0.45 and the offsets out of order.
func TestRestatedStopsKeepTheirOffsets(t *testing.T) {
	stops := []GradientStop{
		{Offset: 0.15, Color: style.RGBA{R: 255, A: 1}, Exponent: 1},
		{Offset: 0.45, Color: style.RGBA{B: 255, A: 1}, Exponent: 1},
		{Offset: 0.45, Color: style.RGBA{G: 255, A: 1}, Exponent: 1},
		{Offset: 0.7, Color: style.RGBA{R: 255, A: 1}, Exponent: 1},
	}
	out, how := interpolateStops(stops, spaceOklab, hueShorter, nil)
	if how != restatedAll {
		t.Fatalf("restating ended %v", how)
	}
	found := 0
	for i, s := range out {
		if i > 0 && s.Offset < out[i-1].Offset {
			t.Fatalf("stop %d at %v is before stop %d at %v", i, s.Offset, i-1, out[i-1].Offset)
		}
		if s.Offset == 0.45 {
			found++
		}
	}
	if found != 2 {
		t.Errorf("%d stops are at 0.45 exactly, want the blue and the lime", found)
	}
}
