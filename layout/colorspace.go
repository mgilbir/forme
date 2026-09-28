package layout

import (
	"math"

	"github.com/mgilbir/forme/style"
)

// Colour spaces, for interpolating a gradient in one.
//
// CSS Color 4 §13 interpolates two colours in a named space: both are
// converted to it, a hue is fixed up by the hue interpolation method, the
// components are premultiplied (all three in a rectangular space, all but the
// hue in a polar one), each is interpolated linearly, and the premultiplication
// is undone. What is here is the conversions that takes, from and back to the
// gamma-encoded sRGB every colour this engine reads is in, and §14.2's gamut
// mapping for a colour the interpolation carries outside sRGB.
//
// The arithmetic is §19's sample code, transcribed: the same transfer
// functions, the same matrices (rational where the specification gives them
// rationally) and the same thresholds for a powerless hue. Every space is
// expressed through CIE XYZ, relative to D65, with Bradford adaptation for the
// three that are relative to D50 (Lab and LCH, ProPhoto, and xyz-d50).

// colorSpace is a <color-space> of CSS Color 4 §13.2.
type colorSpace uint8

const (
	spaceSRGB colorSpace = iota
	spaceSRGBLinear
	spaceDisplayP3
	spaceDisplayP3Linear
	spaceA98RGB
	spaceProPhotoRGB
	spaceRec2020
	spaceLab
	spaceOklab
	spaceXYZD50
	spaceXYZD65
	spaceHSL
	spaceHWB
	spaceLCH
	spaceOklch
)

// colorSpaceNamed reads a <color-space> keyword, already lower case.
func colorSpaceNamed(name string) (colorSpace, bool) {
	switch name {
	case "srgb":
		return spaceSRGB, true
	case "srgb-linear":
		return spaceSRGBLinear, true
	case "display-p3":
		return spaceDisplayP3, true
	case "display-p3-linear":
		return spaceDisplayP3Linear, true
	case "a98-rgb":
		return spaceA98RGB, true
	case "prophoto-rgb":
		return spaceProPhotoRGB, true
	case "rec2020":
		return spaceRec2020, true
	case "lab":
		return spaceLab, true
	case "oklab":
		return spaceOklab, true
	case "xyz-d50":
		return spaceXYZD50, true
	case "xyz", "xyz-d65":
		return spaceXYZD65, true
	case "hsl":
		return spaceHSL, true
	case "hwb":
		return spaceHWB, true
	case "lch":
		return spaceLCH, true
	case "oklch":
		return spaceOklch, true
	}
	return 0, false
}

// hueIndex is which of a polar space's components is its hue, or -1 for a
// rectangular space.
func (s colorSpace) hueIndex() int {
	switch s {
	case spaceHSL, spaceHWB:
		return 0
	case spaceLCH, spaceOklch:
		return 2
	}
	return -1
}

// hueMethod is a <hue-interpolation-method>, §13.5.
type hueMethod uint8

const (
	hueShorter hueMethod = iota // the default
	hueLonger
	hueIncreasing
	hueDecreasing
)

// hueMethodNamed reads the keyword before "hue".
func hueMethodNamed(name string) (hueMethod, bool) {
	switch name {
	case "shorter":
		return hueShorter, true
	case "longer":
		return hueLonger, true
	case "increasing":
		return hueIncreasing, true
	case "decreasing":
		return hueDecreasing, true
	}
	return 0, false
}

type vec3 [3]float64

func mul3(m *[3][3]float64, v vec3) vec3 {
	return vec3{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

// §19's matrices, each converting linear light to or from CIE XYZ.
var (
	linSRGBToXYZ = [3][3]float64{
		{506752.0 / 1228815, 87881.0 / 245763, 12673.0 / 70218},
		{87098.0 / 409605, 175762.0 / 245763, 12673.0 / 175545},
		{7918.0 / 409605, 87881.0 / 737289, 1001167.0 / 1053270},
	}
	xyzToLinSRGB = [3][3]float64{
		{12831.0 / 3959, -329.0 / 214, -1974.0 / 3959},
		{-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810},
		{705.0 / 12673, -2585.0 / 12673, 705.0 / 667},
	}
	linP3ToXYZ = [3][3]float64{
		{608311.0 / 1250200, 189793.0 / 714400, 198249.0 / 1000160},
		{35783.0 / 156275, 247089.0 / 357200, 198249.0 / 2500400},
		{0, 32229.0 / 714400, 5220557.0 / 5000800},
	}
	xyzToLinP3 = [3][3]float64{
		{446124.0 / 178915, -333277.0 / 357830, -72051.0 / 178915},
		{-14852.0 / 17905, 63121.0 / 35810, 423.0 / 17905},
		{11844.0 / 330415, -50337.0 / 660830, 316169.0 / 330415},
	}
	linProPhotoToXYZ = [3][3]float64{
		{0.79776664490064230, 0.13518129740053308, 0.03134773412839220},
		{0.28807482881940130, 0.71183523424187300, 0.00008993693872564},
		{0, 0, 0.82510460251046020},
	}
	xyzToLinProPhoto = [3][3]float64{
		{1.34578688164715830, -0.25557208737979464, -0.05110186497554526},
		{-0.54463070512490190, 1.50824774284514680, 0.02052744743642139},
		{0, 0, 1.21196754563894520},
	}
	linA98ToXYZ = [3][3]float64{
		{573536.0 / 994567, 263643.0 / 1420810, 187206.0 / 994567},
		{591459.0 / 1989134, 6239551.0 / 9945670, 374412.0 / 4972835},
		{53769.0 / 1989134, 351524.0 / 4972835, 4929758.0 / 4972835},
	}
	xyzToLinA98 = [3][3]float64{
		{1829569.0 / 896150, -506331.0 / 896150, -308931.0 / 896150},
		{-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810},
		{16779.0 / 1248040, -147721.0 / 1248040, 1266979.0 / 1248040},
	}
	lin2020ToXYZ = [3][3]float64{
		{63426534.0 / 99577255, 20160776.0 / 139408157, 47086771.0 / 278816314},
		{26158966.0 / 99577255, 472592308.0 / 697040785, 8267143.0 / 139408157},
		{0, 19567812.0 / 697040785, 295819943.0 / 278816314},
	}
	xyzToLin2020 = [3][3]float64{
		{30757411.0 / 17917100, -6372589.0 / 17917100, -4539589.0 / 17917100},
		{-19765991.0 / 29648200, 47925759.0 / 29648200, 467509.0 / 29648200},
		{792561.0 / 44930125, -1921689.0 / 44930125, 42328811.0 / 44930125},
	}
	d65ToD50 = [3][3]float64{
		{1.0479297925449969, 0.022946870601609652, -0.05019226628920524},
		{0.02962780877005599, 0.9904344267538799, -0.017073799063418826},
		{-0.009243040646204504, 0.015055191490298152, 0.7518742814281371},
	}
	d50ToD65 = [3][3]float64{
		{0.955473421488075, -0.02309845494876471, 0.06325924320057072},
		{-0.0283697093338637, 1.0099953980813041, 0.021041441191917323},
		{0.012314014864481998, -0.020507649298898964, 1.330365926242124},
	}
	xyzToLMS = [3][3]float64{
		{0.8190224379967030, 0.3619062600528904, -0.1288737815209879},
		{0.0329836539323885, 0.9292868615863434, 0.0361446663506424},
		{0.0481771893596242, 0.2642395317527308, 0.6335478284694309},
	}
	lmsToOklab = [3][3]float64{
		{0.2104542683093140, 0.7936177747023054, -0.0040720430116193},
		{1.9779985324311684, -2.4285922420485799, 0.4505937096174110},
		{0.0259040424655478, 0.7827717124575296, -0.8086757549230774},
	}
	lmsToXYZ = [3][3]float64{
		{1.2268798758459243, -0.5578149944602171, 0.2813910456659647},
		{-0.0405757452148008, 1.1122868032803170, -0.0717110580655164},
		{-0.0763729366746601, -0.4214933324022432, 1.5869240198367816},
	}
	oklabToLMS = [3][3]float64{
		{1, 0.3963377773761749, 0.2158037573099136},
		{1, -0.1055613458156586, -0.0638541728258133},
		{1, -0.0894841775298119, -1.2914855480194092},
	}
)

// d50White is §19's D50 reference white, from its four-figure chromaticities.
var d50White = vec3{0.3457 / 0.3585, 1, (1 - 0.3457 - 0.3585) / 0.3585}

// Transfer functions, each extended to negative values by reflection as §19's
// are.
func signed(v float64, f func(float64) float64) float64 {
	if v < 0 {
		return -f(-v)
	}
	return f(v)
}

func linSRGB(v float64) float64 {
	return signed(v, func(a float64) float64 {
		if a <= 0.04045 {
			return a / 12.92
		}
		return math.Pow((a+0.055)/1.055, 2.4)
	})
}

func gamSRGB(v float64) float64 {
	return signed(v, func(a float64) float64 {
		if a > 0.0031308 {
			return 1.055*math.Pow(a, 1/2.4) - 0.055
		}
		return 12.92 * a
	})
}

func linProPhoto(v float64) float64 {
	return signed(v, func(a float64) float64 {
		if a <= 16.0/512 {
			return a / 16
		}
		return math.Pow(a, 1.8)
	})
}

func gamProPhoto(v float64) float64 {
	return signed(v, func(a float64) float64 {
		if a >= 1.0/512 {
			return math.Pow(a, 1/1.8)
		}
		return 16 * a
	})
}

func powSigned(v, e float64) float64 {
	return signed(v, func(a float64) float64 { return math.Pow(a, e) })
}

func each(v vec3, f func(float64) float64) vec3 { return vec3{f(v[0]), f(v[1]), f(v[2])} }

// xyzOfSRGB is gamma-encoded sRGB in CIE XYZ relative to D65.
func xyzOfSRGB(rgb vec3) vec3 { return mul3(&linSRGBToXYZ, each(rgb, linSRGB)) }

// srgbOfXYZ is XYZ relative to D65 in gamma-encoded sRGB, unclamped.
func srgbOfXYZ(xyz vec3) vec3 { return each(mul3(&xyzToLinSRGB, xyz), gamSRGB) }

// labOfXYZ50 and xyz50OfLab are §19's XYZ_to_Lab and Lab_to_XYZ.
func labOfXYZ50(xyz vec3) vec3 {
	const eps, kappa = 216.0 / 24389, 24389.0 / 27
	var f vec3
	for i := range f {
		v := xyz[i] / d50White[i]
		if v > eps {
			f[i] = math.Cbrt(v)
		} else {
			f[i] = (kappa*v + 16) / 116
		}
	}
	return vec3{116*f[1] - 16, 500 * (f[0] - f[1]), 200 * (f[1] - f[2])}
}

func xyz50OfLab(lab vec3) vec3 {
	const eps, kappa = 216.0 / 24389, 24389.0 / 27
	f1 := (lab[0] + 16) / 116
	f0 := lab[1]/500 + f1
	f2 := f1 - lab[2]/200
	cube := func(f float64) float64 {
		if f*f*f > eps {
			return f * f * f
		}
		return (116*f - 16) / kappa
	}
	y := lab[0] / kappa
	if lab[0] > kappa*eps {
		y = math.Pow((lab[0]+16)/116, 3)
	}
	return vec3{cube(f0) * d50White[0], y * d50White[1], cube(f2) * d50White[2]}
}

// oklabOfXYZ and xyzOfOklab are §19's XYZ_to_OKLab and OKLab_to_XYZ.
func oklabOfXYZ(xyz vec3) vec3 {
	return mul3(&lmsToOklab, each(mul3(&xyzToLMS, xyz), math.Cbrt))
}

func xyzOfOklab(lab vec3) vec3 {
	return mul3(&lmsToXYZ, each(mul3(&oklabToLMS, lab), func(c float64) float64 { return c * c * c }))
}

// polarOf and rectangularOf are §19's Lab_to_LCH and LCH_to_Lab, for either
// Lab: a chroma at or under eps leaves the hue powerless, which is a missing
// hue (NaN here).
func polarOf(lab vec3, eps float64) vec3 {
	c := math.Hypot(lab[1], lab[2])
	h := math.Atan2(lab[2], lab[1]) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	if c <= eps {
		h = math.NaN()
	}
	return vec3{lab[0], c, h}
}

func rectangularOf(lch vec3) vec3 {
	h := lch[2]
	if math.IsNaN(h) {
		h = 0
	}
	s, c := math.Sincos(h * math.Pi / 180)
	return vec3{lch[0], lch[1] * c, lch[1] * s}
}

// hslOf and srgbOfHSL are §7's rgbToHsl and hslToRgb, with saturation and
// lightness as fractions rather than percentages.
func hslOf(rgb vec3) vec3 {
	r, g, b := rgb[0], rgb[1], rgb[2]
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	hue, sat, light := math.NaN(), 0.0, (min+max)/2
	d := max - min
	if d != 0 {
		if light != 0 && light != 1 {
			sat = (max - light) / math.Min(light, 1-light)
		}
		switch max {
		case r:
			hue = (g - b) / d
			if g < b {
				hue += 6
			}
		case g:
			hue = (b-r)/d + 2
		default:
			hue = (r-g)/d + 4
		}
		hue *= 60
	}
	if sat < 0 {
		hue += 180
		sat = -sat
	}
	if hue >= 360 {
		hue -= 360
	}
	if sat <= 1.0/100000 {
		hue = math.NaN()
	}
	return vec3{hue, sat, light}
}

func srgbOfHSL(hsl vec3) vec3 {
	hue, sat, light := hsl[0], hsl[1], hsl[2]
	if math.IsNaN(hue) {
		hue = 0
	}
	f := func(n float64) float64 {
		k := math.Mod(n+hue/30, 12)
		if k < 0 {
			k += 12
		}
		a := sat * math.Min(light, 1-light)
		return light - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	return vec3{f(0), f(8), f(4)}
}

// hwbOf and srgbOfHWB are §8's rgbToHwb and hwbToRgb, with whiteness and
// blackness as fractions.
func hwbOf(rgb vec3) vec3 {
	r, g, b := rgb[0], rgb[1], rgb[2]
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	hue := math.NaN()
	if d := max - min; d != 0 {
		switch max {
		case r:
			hue = (g - b) / d
			if g < b {
				hue += 6
			}
		case g:
			hue = (b-r)/d + 2
		default:
			hue = (r-g)/d + 4
		}
		hue *= 60
	}
	if hue >= 360 {
		hue -= 360
	}
	white, black := min, 1-max
	if white+black >= 1-1.0/100000 {
		hue = math.NaN()
	}
	return vec3{hue, white, black}
}

func srgbOfHWB(hwb vec3) vec3 {
	white, black := hwb[1], hwb[2]
	if white+black >= 1 {
		gray := white / (white + black)
		return vec3{gray, gray, gray}
	}
	rgb := srgbOfHSL(vec3{hwb[0], 1, 0.5})
	for i := range rgb {
		rgb[i] = rgb[i]*(1-white-black) + white
	}
	return rgb
}

// toXYZ is a colour in space s in CIE XYZ relative to D65.
func (s colorSpace) toXYZ(c vec3) vec3 {
	switch s {
	case spaceSRGB:
		return xyzOfSRGB(c)
	case spaceSRGBLinear:
		return mul3(&linSRGBToXYZ, c)
	case spaceDisplayP3:
		return mul3(&linP3ToXYZ, each(c, linSRGB))
	case spaceDisplayP3Linear:
		return mul3(&linP3ToXYZ, c)
	case spaceA98RGB:
		return mul3(&linA98ToXYZ, each(c, func(v float64) float64 { return powSigned(v, 563.0/256) }))
	case spaceProPhotoRGB:
		return mul3(&d50ToD65, mul3(&linProPhotoToXYZ, each(c, linProPhoto)))
	case spaceRec2020:
		return mul3(&lin2020ToXYZ, each(c, func(v float64) float64 { return powSigned(v, 2.4) }))
	case spaceLab:
		return mul3(&d50ToD65, xyz50OfLab(c))
	case spaceLCH:
		return mul3(&d50ToD65, xyz50OfLab(rectangularOf(c)))
	case spaceOklab:
		return xyzOfOklab(c)
	case spaceOklch:
		return xyzOfOklab(rectangularOf(c))
	case spaceXYZD50:
		return mul3(&d50ToD65, c)
	case spaceXYZD65:
		return c
	case spaceHSL:
		return xyzOfSRGB(srgbOfHSL(c))
	case spaceHWB:
		return xyzOfSRGB(srgbOfHWB(c))
	}
	return vec3{}
}

// fromXYZ is a colour in CIE XYZ relative to D65 in space s. A polar space's
// hue is NaN where it is powerless.
func (s colorSpace) fromXYZ(xyz vec3) vec3 {
	switch s {
	case spaceSRGB:
		return srgbOfXYZ(xyz)
	case spaceSRGBLinear:
		return mul3(&xyzToLinSRGB, xyz)
	case spaceDisplayP3:
		return each(mul3(&xyzToLinP3, xyz), gamSRGB)
	case spaceDisplayP3Linear:
		return mul3(&xyzToLinP3, xyz)
	case spaceA98RGB:
		return each(mul3(&xyzToLinA98, xyz), func(v float64) float64 { return powSigned(v, 256.0/563) })
	case spaceProPhotoRGB:
		return each(mul3(&xyzToLinProPhoto, mul3(&d65ToD50, xyz)), gamProPhoto)
	case spaceRec2020:
		return each(mul3(&xyzToLin2020, xyz), func(v float64) float64 { return powSigned(v, 1/2.4) })
	case spaceLab:
		return labOfXYZ50(mul3(&d65ToD50, xyz))
	case spaceLCH:
		return polarOf(labOfXYZ50(mul3(&d65ToD50, xyz)), 0.0015)
	case spaceOklab:
		return oklabOfXYZ(xyz)
	case spaceOklch:
		return polarOf(oklabOfXYZ(xyz), 0.000004)
	case spaceXYZD50:
		return mul3(&d65ToD50, xyz)
	case spaceXYZD65:
		return xyz
	case spaceHSL:
		return hslOf(srgbOfXYZ(xyz))
	case spaceHWB:
		return hwbOf(srgbOfXYZ(xyz))
	}
	return vec3{}
}

// fromSRGB is a gamma-encoded sRGB colour, components in [0, 1], in space s.
// The sRGB-based spaces are converted directly, which is exact where a round
// trip through XYZ would not be.
func (s colorSpace) fromSRGB(rgb vec3) vec3 {
	switch s {
	case spaceSRGB:
		return rgb
	case spaceHSL:
		return hslOf(rgb)
	case spaceHWB:
		return hwbOf(rgb)
	case spaceSRGBLinear:
		return each(rgb, linSRGB)
	}
	return s.fromXYZ(xyzOfSRGB(rgb))
}

// toSRGB is a colour in space s in gamma-encoded sRGB, unclamped.
func (s colorSpace) toSRGB(c vec3) vec3 {
	switch s {
	case spaceSRGB:
		return c
	case spaceHSL:
		return srgbOfHSL(c)
	case spaceHWB:
		return srgbOfHWB(c)
	case spaceSRGBLinear:
		return each(c, gamSRGB)
	}
	return srgbOfXYZ(s.toXYZ(c))
}

// Gamut mapping: CSS Color 4 §14.2.2, the binary search with local MINDE, to
// sRGB.

// inSRGB reports whether a gamma-encoded sRGB colour is inside the gamut.
func inSRGB(rgb vec3) bool {
	for _, v := range rgb {
		if v < 0 || v > 1 {
			return false
		}
	}
	return true
}

func clampSRGB(rgb vec3) vec3 {
	return each(rgb, func(v float64) float64 { return math.Max(0, math.Min(1, v)) })
}

// deltaEOK is §20's colour difference in Oklab: the distance between two
// colours.
func deltaEOK(a, b vec3) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// gamutMapSRGB is a colour in space s mapped into sRGB by §14.2.2's
// algorithm, gamma-encoded and in [0, 1]: a colour inside the gamut unchanged,
// and one outside it the colour of the same lightness and hue in OkLCh whose
// chroma is reduced until its clipped form is within a just noticeable
// difference of it.
func gamutMapSRGB(s colorSpace, c vec3) vec3 {
	rgb := s.toSRGB(c)
	if inSRGB(rgb) {
		return rgb
	}
	origin := polarOf(oklabOfXYZ(s.toXYZ(c)), 0)
	if origin[0] >= 1 {
		return vec3{1, 1, 1}
	}
	if origin[0] <= 0 {
		return vec3{0, 0, 0}
	}
	const jnd, eps = 0.02, 0.0001
	oklabOf := func(lch vec3) vec3 { return rectangularOf(lch) }
	clip := func(lch vec3) vec3 { return clampSRGB(srgbOfXYZ(xyzOfOklab(rectangularOf(lch)))) }
	delta := func(clipped vec3, current vec3) float64 {
		return deltaEOK(oklabOfXYZ(xyzOfSRGB(clipped)), oklabOf(current))
	}
	current := origin
	clipped := clip(current)
	if delta(clipped, current) < jnd {
		return clipped
	}
	min, max := 0.0, origin[1]
	minInGamut := true
	for max-min > eps {
		chroma := (min + max) / 2
		current[1] = chroma
		if minInGamut && inSRGB(srgbOfXYZ(xyzOfOklab(rectangularOf(current)))) {
			min = chroma
			continue
		}
		clipped = clip(current)
		e := delta(clipped, current)
		if e < jnd {
			if jnd-e < eps {
				return clipped
			}
			minInGamut = false
			min = chroma
		} else {
			max = chroma
		}
	}
	return clipped
}

// srgbComponents is a colour's components as fractions, and back.
func srgbComponents(c style.RGBA) vec3 { return vec3{c.R / 255, c.G / 255, c.B / 255} }

func rgbaOf(rgb vec3, alpha float64) style.RGBA {
	return style.RGBA{R: rgb[0] * 255, G: rgb[1] * 255, B: rgb[2] * 255, A: alpha}
}

// colourMix is two colours prepared for §13's interpolation in a space: each
// converted, a missing hue taken from the other, the hues fixed up, and every
// component premultiplied but a hue.
type colourMix struct {
	space  colorSpace
	a, b   vec3 // premultiplied
	aa, ab float64
}

func newColourMix(s colorSpace, h hueMethod, ca, cb style.RGBA) colourMix {
	a, b := s.fromSRGB(srgbComponents(ca)), s.fromSRGB(srgbComponents(cb))
	hi := s.hueIndex()
	if hi >= 0 {
		// §13.3: a missing component takes the other colour's value; where
		// both are missing the result is missing, which converts as zero.
		switch {
		case math.IsNaN(a[hi]) && math.IsNaN(b[hi]):
			a[hi], b[hi] = 0, 0
		case math.IsNaN(a[hi]):
			a[hi] = b[hi]
		case math.IsNaN(b[hi]):
			b[hi] = a[hi]
		}
		a[hi], b[hi] = fixHues(a[hi], b[hi], h)
	}
	for i := range a {
		if i != hi {
			a[i] *= ca.A
			b[i] *= cb.A
		}
	}
	return colourMix{space: s, a: a, b: b, aa: ca.A, ab: cb.A}
}

// fixHues is §13.5: the two hues adjusted so that interpolating from one to the
// other goes the way the method asks.
func fixHues(h1, h2 float64, m hueMethod) (float64, float64) {
	d := h2 - h1
	switch m {
	case hueLonger:
		switch {
		case 0 < d && d < 180:
			h1 += 360
		case -180 < d && d <= 0:
			h2 += 360
		}
	case hueIncreasing:
		if h2 < h1 {
			h2 += 360
		}
	case hueDecreasing:
		if h1 < h2 {
			h1 += 360
		}
	default:
		switch {
		case d > 180:
			h1 += 360
		case d < -180:
			h2 += 360
		}
	}
	return h1, h2
}

// coordsAt is the colour w of the way from the first colour to the second, in
// the space's own coordinates: the premultiplied components interpolated and
// the premultiplication undone (§13.4), with a hue brought back into a turn.
func (m colourMix) coordsAt(w float64) (vec3, float64) {
	alpha := m.aa*(1-w) + m.ab*w
	var c vec3
	hi := m.space.hueIndex()
	for i := range c {
		c[i] = m.a[i]*(1-w) + m.b[i]*w
		if i != hi && alpha != 0 {
			c[i] /= alpha
		}
	}
	if hi >= 0 {
		c[hi] = math.Mod(c[hi], 360)
		if c[hi] < 0 {
			c[hi] += 360
		}
	}
	return c, alpha
}

// at is coordsAt mapped into sRGB, which is the colour drawn.
func (m colourMix) at(w float64) style.RGBA {
	c, alpha := m.coordsAt(w)
	if alpha <= 0 {
		return style.RGBA{}
	}
	return rgbaOf(gamutMapSRGB(m.space, c), alpha)
}
