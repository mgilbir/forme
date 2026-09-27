package layout

import (
	"math"
	"sort"

	"github.com/mgilbir/forme/style"
)

// Gradients as a paint operation.
//
// gradient.go draws the two shapes of gradient that are rectangles — one colour,
// and solid bands — as fills, and those stay exactly as they were: a page that
// writes linear-gradient(green, green) still produces the display list a page
// writing background-color produces. What is here is everything else CSS Images
// 3 and 4 call a gradient, and it needs an operation of its own because what it
// paints is not made of rectangles at all.
//
// # What the operation says, and why a PDF backend can draw it exactly
//
// A gradient leaves layout as a description rather than as pixels: its kind, the
// geometry of its gradient line in the coordinates of one tile, and its colour
// stops with every position already resolved and fixed up. The three kinds are
// the three shadings a PDF can express:
//
//   - A linear gradient is an axial shading (ISO 32000-2 §8.7.4.5.3, type 2):
//     Start and End are its Coords, offset 0 and offset 1.
//   - A radial gradient is a radial shading (type 3) with both circles centred
//     on Center, the first of radius 0 and the second of radius RadiusX, drawn
//     through a matrix that scales the vertical axis by RadiusY/RadiusX — CSS's
//     ending shape is an ellipse and PDF's circles are circles.
//   - A conic gradient has no shading type of its own, and is a function-based
//     shading (type 1) whose function is the angle about Center: a PostScript
//     calculator function (type 4) has atan, and that is the whole of it.
//
// In each the colour at an offset t is the same function of t, stated by
// ColorAtOffset: the stops' colours, interpolated in premultiplied,
// gamma-encoded sRGB, each segment along the curve its Exponent gives. That is a
// type 2 (exponential) function per segment and a type 3 (stitching) function
// across them, or one type 4 function for the whole; premultiplication is the
// one part a type 2 function cannot do on its own, and it matters only where
// two neighbouring stops differ in alpha. A repeating gradient's offsets are
// taken modulo its period, which a type 4 function also does in one operation.
//
// ColorAt is the whole definition in code: it is what the reftest comparison
// samples, and what a rasterising backend can call directly. A backend that
// draws what ColorAt says draws what CSS says.

// GradientKind is the shape of a gradient's colour field.
type GradientKind uint8

const (
	// LinearGradient's colour is constant along lines perpendicular to the
	// gradient line from Start to End.
	LinearGradient GradientKind = iota + 1
	// RadialGradient's colour is constant along ellipses centred on Center with
	// the ending shape's proportions.
	RadialGradient
	// ConicGradient's colour is constant along rays from Center.
	ConicGradient
)

// Gradient is one gradient image, laid out for a tile.
//
// Every point is measured from the top left of the tile it is painted in, in
// layout units, so one value describes every tile of a repeated background.
type Gradient struct {
	Kind GradientKind

	// Repeating says the stops repeat along the gradient line for ever in both
	// directions, the period being the distance from the first stop to the last
	// (CSS Images 3 §3.3). The last stop of a repeating gradient is always
	// strictly after its first: a period of nothing is a solid colour, and is
	// painted as one before it gets here.
	Repeating bool

	// Start and End are a linear gradient's gradient line: offset 0 is at Start
	// and offset 1 at End, and a point's offset is where the perpendicular
	// through it meets the line. They are never the same point.
	Start, End Point

	// Center is the centre of a radial or a conic gradient.
	Center Point
	// RadiusX and RadiusY are a radial gradient's ending shape: the ellipse on
	// which the offset is 1. A point's offset is
	//
	//	sqrt((dx/RadiusX)² + (dy/RadiusY)²)
	//
	// for dx, dy its distance from Center. Both are always greater than zero.
	// CSS Images 3 §3.2.3's degenerate shapes, with a radius of nothing, are
	// stated with the smallest radius a layout unit can hold in its place and,
	// for a shape of no width, the largest height; see resolveRadial.
	RadiusX, RadiusY style.Unit

	// FromAngle is where a conic gradient's offset 0 lies, in degrees clockwise
	// from straight up. A point's offset is the angle of the ray from Center
	// through it, measured clockwise from FromAngle, as a fraction of a turn in
	// [0, 1).
	FromAngle float64

	// Stops are the colour stops, in order, fixed up per CSS Images 4 §3.5.3:
	// their offsets never decrease, and there is at least one. Where two share
	// an offset the colour changes there at once.
	Stops []GradientStop
}

// GradientStop is one colour stop: a colour at an offset along the gradient
// line.
type GradientStop struct {
	// Offset is the position on the gradient line, where 0 and 1 are the ends
	// its Gradient's geometry names. It may be outside that range: CSS places
	// stops before the start and past the end, and they colour what is inside
	// through interpolation.
	Offset float64
	// Color is the stop's colour, not premultiplied.
	Color style.RGBA
	// Exponent shapes the transition from the stop before this one to this
	// one. At a fraction p of the way between the two, this stop's colour
	// weighs p^Exponent and the one before weighs the rest (CSS Images 4
	// §3.5.2's transition hints). It is 1 for a plain linear blend and is
	// always greater than zero and finite; a hint at either end of its
	// segment, which would need 0 or infinity here, is stated as the hard stop
	// it amounts to instead. It is read from every stop but the first.
	Exponent float64
}

// ColorAt is the colour the gradient paints at a point, measured from the top
// left of its tile.
func (g Gradient) ColorAt(p Point) style.RGBA {
	return g.ColorAtOffset(g.offsetOf(p.X.Px(), p.Y.Px()))
}

// offsetOf is where on the gradient line a point in the tile falls, in pixels.
func (g Gradient) offsetOf(x, y float64) float64 {
	switch g.Kind {
	case RadialGradient:
		rx, ry := g.RadiusX.Px(), g.RadiusY.Px()
		if rx <= 0 || ry <= 0 {
			return 0
		}
		dx := (x - g.Center.X.Px()) / rx
		dy := (y - g.Center.Y.Px()) / ry
		return math.Hypot(dx, dy)
	case ConicGradient:
		dx, dy := x-g.Center.X.Px(), y-g.Center.Y.Px()
		// Clockwise from straight up, in a coordinate system whose y grows
		// downwards: up is (0, -1) and a quarter turn clockwise is (1, 0).
		deg := math.Atan2(dx, -dy) * 180 / math.Pi
		t := math.Mod(deg-g.FromAngle, 360) / 360
		if t < 0 {
			t++
		}
		if t >= 1 {
			t = 0
		}
		return t
	}
	sx, sy := g.Start.X.Px(), g.Start.Y.Px()
	ex, ey := g.End.X.Px()-sx, g.End.Y.Px()-sy
	d := ex*ex + ey*ey
	if d == 0 {
		return 0
	}
	return ((x-sx)*ex + (y-sy)*ey) / d
}

// ColorAtOffset is the colour of the gradient line at an offset along it.
//
// CSS Images 4 §3.5.2: before the first stop the first stop's colour, after the
// last the last's, and between two the blend the second one's Exponent gives,
// in premultiplied alpha (CSS Color 4 §13.4) — so a stop fading to transparent
// fades its own colour out, rather than through the black that "transparent"
// is written as. A repeating gradient first brings the offset into its period.
func (g Gradient) ColorAtOffset(t float64) style.RGBA {
	n := len(g.Stops)
	if n == 0 {
		return style.RGBA{}
	}
	if math.IsNaN(t) {
		t = 0
	}
	first, last := g.Stops[0].Offset, g.Stops[n-1].Offset
	if g.Repeating && last > first {
		period := last - first
		t = first + math.Mod(t-first, period)
		if t < first {
			t += period
		}
	}
	// The first stop strictly past t. Stops at t itself are behind it, which is
	// what makes the colour at a hard stop the later colour: the transition is
	// "from the one specified first to the one specified last", and the point
	// itself is on the far side of it.
	j := sort.Search(n, func(i int) bool { return g.Stops[i].Offset > t })
	switch {
	case j == 0:
		return g.Stops[0].Color
	case j == n:
		return g.Stops[n-1].Color
	}
	a, b := g.Stops[j-1], g.Stops[j]
	span := b.Offset - a.Offset
	p := (t - a.Offset) / span
	e := b.Exponent
	if e <= 0 || math.IsNaN(e) || math.IsInf(e, 0) {
		e = 1
	}
	w := p
	if e != 1 {
		w = math.Pow(p, e)
	}
	return mixPremultiplied(a.Color, b.Color, w)
}

// mixPremultiplied blends two colours in premultiplied alpha: w of b and 1-w of
// a. It is CSS Color 4 §13.4's "premultiply, interpolate, divide by the
// interpolated alpha", and where that alpha is nothing the colour is nothing.
func mixPremultiplied(a, b style.RGBA, w float64) style.RGBA {
	alpha := a.A*(1-w) + b.A*w
	if alpha <= 0 {
		return style.RGBA{}
	}
	mix := func(x, y float64) float64 { return (x*a.A*(1-w) + y*b.A*w) / alpha }
	return style.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: alpha}
}

// averageColor is the solid colour CSS Images 3 §3.3 paints a repeating
// gradient as when its period is too small to be seen, or is nothing: each pair
// of neighbouring stops contributes both its colours, premultiplied, each
// weighted by half the distance between the two as a fraction of the whole.
//
// A period of nothing is averaged as though its stops were spread evenly over
// some distance instead, which the same sentence asks for, so a gradient of
// three stops at one position is the three colours in the proportions 1:2:1.
// Transition hints play no part, because the specification's steps give them
// none.
func averageColor(stops []GradientStop) style.RGBA {
	n := len(stops)
	if n == 0 {
		return style.RGBA{}
	}
	if n == 1 {
		return stops[0].Color
	}
	pos := make([]float64, n)
	total := stops[n-1].Offset - stops[0].Offset
	if total > 0 {
		for i, s := range stops {
			pos[i] = s.Offset
		}
	} else {
		for i := range stops {
			pos[i] = float64(i)
		}
		total = float64(n - 1)
	}
	var r, g, b, a float64
	for i := 0; i+1 < n; i++ {
		w := (pos[i+1] - pos[i]) / 2 / total
		for _, c := range [2]style.RGBA{stops[i].Color, stops[i+1].Color} {
			r += c.R * c.A * w
			g += c.G * c.A * w
			b += c.B * c.A * w
			a += c.A * w
		}
	}
	if a <= 0 {
		return style.RGBA{}
	}
	return style.RGBA{R: r / a, G: g / a, B: b / a, A: math.Min(a, 1)}
}

// FillGradient paints a gradient repeatedly across a rectangle.
//
// It is TileImage for a gradient: a first tile, a step, and the area the
// tiling may paint, with the count appearing nowhere. It was checked against
// maxBackgroundTiles before it was built, as a picture's tiling is, because
// whatever expands it into tiles — a PDF tiling pattern whose cell holds the
// shading — is entitled not to be handed billions of cells.
//
// The gradient is painted afresh in each tile: Gradient's geometry is measured
// from the tile's own top left, and a tile shows exactly the part of the
// gradient that falls inside it, clipped to it. That is CSS Images §3's
// "gradient box", which is the tile a background layer sizes, and it is why a
// gradient does not run on across a repeat.
type FillGradient struct {
	// Clip is the area painted. Nothing is drawn outside it.
	Clip Rect
	// Tile is the first tile: where it is and how large. The gradient is
	// clipped to each tile, as well as to Clip.
	Tile Rect
	// StepX and StepY are the distance to the next tile on each axis, and are
	// always greater than zero; see TileImage.
	StepX, StepY style.Unit

	Gradient Gradient

	// Overhang marks a fill no layout decision accounted for the position of,
	// exactly as FillRect.Overhang does: it is set on the background of an
	// inline box, so that the overflow-page guardrail reads the two alike.
	Overhang bool
}

// Tiles is how many tiles touch the clip on each axis.
func (f FillGradient) Tiles() (cols, rows int) {
	return TileImage{Clip: f.Clip, Tile: f.Tile, StepX: f.StepX, StepY: f.StepY}.Tiles()
}

func (FillGradient) isOp() {}
