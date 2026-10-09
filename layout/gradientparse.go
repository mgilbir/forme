package layout

import (
	"fmt"
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// Reading a gradient, and laying it out for a tile.
//
// A gradient is read twice over, and at two different times. The value is read
// once per box, when its background is: that is where the colours are settled
// ("currentcolor" is the box's colour) and where a length in em becomes one in
// pixels. But where its stops fall on the gradient line, and how long that line
// is, depend on the size of the tile it is painted in — background-size decides
// that, and one value may be painted at two sizes on two boxes — so the second
// half happens in tiling, once the tile has a size. gradientSpec is what lies
// between the two.
//
// # What is read
//
// CSS Images 3's three gradients and their repeating forms, with CSS Images 4's
// conic gradient, its double-position stops, its transition hints, its single
// stop and its colour interpolation method: every space of CSS Color 4 §13.2,
// with a hue interpolation method for the polar ones (see gradientspace.go for
// how a gradient in one is drawn). What is not read is reported, as the value
// always was, and painted as nothing:
//
//   - CSS Images 4's two additions to <radial-size>, a percentage circle and
//     two extent keywords, which no browser reads yet either.
//
// An angle anywhere in a gradient — a direction, a conic gradient's "from", a
// conic stop's position — may be a calc(), which style.ParseAngle evaluates
// with the arithmetic lengths have.

// maxGradientStops bounds the colour stops and transition hints one gradient may
// have.
//
// It is a count a stylesheet writes out one by one, so it costs the document
// something to reach — but a backend expands each stop into a function of its
// own and every sample of the gradient searches them, and a thousand stops is
// already past what any gradient that is meant to be looked at needs. A
// gradient past it is not drawn, and says so.
//
// A variable so that a test can lower it.
var maxGradientStops = 1024

// maxGradientRepeats bounds how many periods of a repeating gradient one tile
// may show.
//
// It is a ratio a stylesheet controls both ends of — the tile from
// background-size, the period from the stop positions — and a backend that
// expands the repetitions, as a PDF stitching function does, is handed that
// count. Sixty-five thousand is maxLayerMarks's line and for the same reason: a
// stripe a pixel apart down a box sixty-five thousand pixels long. A gradient
// that repeats more finely than that in one tile is painted as CSS Images 3 §3.3
// says one too fine to render is — its average colour — and says why.
//
// A variable so that a test can lower it.
var maxGradientRepeats = 1 << 16

// radialExtent is how a radial gradient's ending shape is sized.
type radialExtent uint8

const (
	extentFarthestCorner radialExtent = iota // the initial value
	extentClosestSide
	extentFarthestSide
	extentClosestCorner
	extentExplicit
)

// gradientSpec is a gradient read from its value, before a tile gives it a size.
type gradientSpec struct {
	kind      GradientKind
	repeating bool

	// A linear gradient's direction: an angle, in degrees clockwise from up,
	// unless corner names one of the box's corners, which is a direction only a
	// box's shape can turn into an angle. cornerX and cornerY are +1 for right
	// and bottom and -1 for left and top.
	angle            float64
	corner           bool
	cornerX, cornerY float64

	// A radial gradient's ending shape.
	circle       bool
	extent       radialExtent
	sizeX, sizeY style.Length

	// The centre of a radial or conic gradient, which is where "at" puts it.
	center bgPosPair

	// The colour interpolation method: the space the stops are interpolated
	// in, sRGB unless one is named, and for a polar space the hue method.
	space colorSpace
	hue   hueMethod

	// A conic gradient's rotation, in degrees clockwise from up.
	from float64

	items []gradientItem

	// source is the value as written, which is what a report about it names.
	source string
}

// gradientItem is one entry of a colour stop list: a colour stop, or a
// transition hint between two.
//
// A position is a length-percentage along the line, or for a conic gradient a
// fraction of a turn written as a percentage of one — an angle is converted
// when it is read, since a turn is the only length the line of a conic gradient
// has.
type gradientItem struct {
	hint   bool
	colour style.RGBA
	pos    style.Length
	placed bool
}

// gradientStatus is why a value was not read as a gradient.
type gradientStatus uint8

const (
	gradientRead gradientStatus = iota
	// gradientNotOne is a value that is not a gradient this engine reads —
	// malformed, or a form listed above.
	gradientNotOne
	// gradientTooManyStops is a gradient past maxGradientStops.
	gradientTooManyStops
)

// parseGradient reads a background layer's value as a gradient.
func (l *layouter) parseGradient(b *Box, raw string) (*gradientSpec, gradientStatus) {
	vals, errs := css.ParseComponentValues(raw)
	if len(errs) > 0 {
		return nil, gradientNotOne
	}
	fn, ok := soleFunction(vals)
	if !ok {
		return nil, gradientNotOne
	}
	s := &gradientSpec{center: centred(), source: raw}
	name := ascii.Lower(fn.Token.Value)
	if rest, ok := strings.CutPrefix(name, "repeating-"); ok {
		s.repeating, name = true, rest
	}
	switch name {
	case "linear-gradient":
		s.kind, s.angle = LinearGradient, 180 // "to bottom"
	case "radial-gradient":
		s.kind = RadialGradient
	case "conic-gradient":
		s.kind = ConicGradient
	default:
		return nil, gradientNotOne
	}

	args := splitTopLevelCommas(fn.Values)
	if len(args) == 0 {
		return nil, gradientNotOne
	}
	parts := splitValueParts(args[0])
	if len(parts) == 0 {
		return nil, gradientNotOne
	}
	if _, isColour := l.gradientColour(b, parts[0]); !isColour {
		// The first argument is the gradient's shape rather than its first
		// stop. A stop begins with a colour and a shape never does — none of
		// "to", "at", "from", "in", the two shapes, the four extents, a length
		// or an angle is one — so the colour is what tells the two apart.
		if !l.readGradientShape(b, s, parts) {
			return nil, gradientNotOne
		}
		args = args[1:]
	}
	if !l.readColourStops(b, s, args) {
		return nil, gradientNotOne
	}
	if len(s.items) > maxGradientStops {
		// A stop with two positions is two stops, so the list can pass the
		// bound with fewer arguments than it.
		return nil, gradientTooManyStops
	}
	return s, gradientRead
}

// centred is the position "center": CSS Images 3's default centre for both
// radial and conic gradients.
func centred() bgPosPair {
	half := bgPos{offset: style.Length{Kind: style.LengthPercent, Percent: 50}}
	return bgPosPair{x: half, y: half}
}

// readGradientShape reads the first argument of a gradient, when it is not a
// colour stop: the direction, the ending shape, the centre, the rotation and the
// interpolation method, each in the grammar of its own gradient.
//
// The interpolation method may come before the rest of the argument or after
// it, and not inside it — "[ ... ] || <color-interpolation-method>" — so once it
// has followed anything, nothing else may.
func (l *layouter) readGradientShape(b *Box, s *gradientSpec, parts [][]css.ComponentValue) bool {
	var sawIn, sawDirection, sawAt, sawShape, sawSize, sawFrom bool
	// closed is set when the method followed something, after which the
	// argument is over.
	var begun, closed bool
	var sizes []style.Length
	for i := 0; i < len(parts); {
		word, isWord := identOf(parts[i])
		if isWord && word == "in" {
			// CSS Color 4 §13.2's <color-interpolation-method>: "in" and a
			// <rectangular-color-space>, or a <polar-color-space> and then
			// perhaps "<method> hue".
			if sawIn || i+1 >= len(parts) {
				return false
			}
			name, ok := identOf(parts[i+1])
			if !ok {
				return false
			}
			space, ok := colorSpaceNamed(name)
			if !ok {
				return false
			}
			s.space = space
			i += 2
			if space.hueIndex() >= 0 && i+1 < len(parts) {
				if m, isWord := identOf(parts[i]); isWord {
					if method, ok := hueMethodNamed(m); ok {
						if h, ok := identOf(parts[i+1]); !ok || h != "hue" {
							return false
						}
						s.hue = method
						i += 2
					}
				}
			}
			sawIn, closed = true, begun
			continue
		}
		if closed {
			return false
		}
		begun = true
		switch {
		case isWord && word == "to" && s.kind == LinearGradient:
			if sawDirection {
				return false
			}
			var x, y float64
			j := i + 1
			for ; j < len(parts); j++ {
				side, ok := identOf(parts[j])
				if !ok || side == "in" {
					break
				}
				switch {
				case side == "left" && x == 0:
					x = -1
				case side == "right" && x == 0:
					x = 1
				case side == "top" && y == 0:
					y = -1
				case side == "bottom" && y == 0:
					y = 1
				default:
					return false
				}
			}
			switch {
			case x != 0 && y != 0:
				s.corner, s.cornerX, s.cornerY = true, x, y
			case x > 0:
				s.angle = 90
			case x < 0:
				s.angle = 270
			case y < 0:
				s.angle = 0
			case y > 0:
				s.angle = 180
			default:
				return false
			}
			sawDirection = true
			i = j

		case isWord && word == "at" && s.kind != LinearGradient:
			if sawAt {
				return false
			}
			j := i + 1
			for j < len(parts) {
				if w, ok := identOf(parts[j]); ok && w == "in" {
					break
				}
				j++
			}
			var pos []css.ComponentValue
			for k := i + 1; k < j; k++ {
				if k > i+1 {
					pos = append(pos, css.ComponentValue{Token: css.Token{Kind: css.Whitespace, Value: " "}})
				}
				pos = append(pos, parts[k]...)
			}
			center, ok := l.parsePosition(b, pos)
			if !ok {
				return false
			}
			s.center, sawAt = center, true
			i = j

		case isWord && word == "from" && s.kind == ConicGradient:
			if sawFrom || sawAt || i+1 >= len(parts) {
				return false
			}
			a, ok := gradientAngle(parts[i+1])
			if !ok {
				return false
			}
			s.from, sawFrom = a, true
			i += 2

		case isWord && s.kind == RadialGradient && (word == "circle" || word == "ellipse"):
			if sawShape || sawAt {
				return false
			}
			s.circle, sawShape = word == "circle", true
			i++

		case isWord && s.kind == RadialGradient && radialExtentOf(word) != extentExplicit:
			if sawSize || sawAt {
				return false
			}
			s.extent, sawSize = radialExtentOf(word), true
			i++

		case !isWord && s.kind == LinearGradient:
			if sawDirection {
				return false
			}
			a, ok := gradientAngle(parts[i])
			if !ok {
				return false
			}
			s.angle, sawDirection = a, true
			i++

		case !isWord && s.kind == RadialGradient:
			// A size: one length, or two length-percentages, in a row.
			if sawSize || sawAt {
				return false
			}
			for i < len(parts) && len(sizes) < 3 {
				length, ok := l.lengthOfValues(b, parts[i])
				if !ok || !isLengthPercentage(length) || negativeLength(length) {
					break
				}
				sizes = append(sizes, length)
				i++
			}
			if len(sizes) == 0 {
				return false
			}
			sawSize = true

		default:
			return false
		}
	}
	if len(sizes) > 0 {
		// CSS Images 3 §3.2.1's expansion: a circle takes one length and no
		// percentage, an ellipse two length-percentages, and a shape left
		// unnamed is whichever of the two the size is.
		switch len(sizes) {
		case 1:
			if (sawShape && !s.circle) || sizes[0].Kind != style.LengthAbsolute {
				return false
			}
			s.circle = true
			s.sizeX, s.sizeY = sizes[0], sizes[0]
		case 2:
			if sawShape && s.circle {
				return false
			}
			s.circle = false
			s.sizeX, s.sizeY = sizes[0], sizes[1]
		default:
			return false
		}
		s.extent = extentExplicit
	}
	return true
}

// radialExtentOf reads one of the four <radial-extent> keywords, and answers
// extentExplicit for anything else.
func radialExtentOf(word string) radialExtent {
	switch word {
	case "closest-side":
		return extentClosestSide
	case "farthest-side":
		return extentFarthestSide
	case "closest-corner":
		return extentClosestCorner
	case "farthest-corner":
		return extentFarthestCorner
	}
	return extentExplicit
}

// isLengthPercentage reports whether a length is one a gradient can place:
// anything but auto.
func isLengthPercentage(l style.Length) bool {
	switch l.Kind {
	case style.LengthAbsolute, style.LengthPercent, style.LengthCalc, style.LengthMath:
		return true
	}
	return false
}

// gradientAngle reads an <angle>, a calc() of one, or the <zero> CSS Images 4
// allows in its place, in degrees.
func gradientAngle(part []css.ComponentValue) (float64, bool) {
	if len(part) != 1 {
		return 0, false
	}
	if part[0].IsToken() && part[0].Token.Kind == css.Number {
		return 0, part[0].Token.Number == 0
	}
	return style.ParseAngle(part)
}

// gradientColour reads a stop's colour. "currentcolor" is the box's own colour,
// which is what a gradient's stops resolve it against.
func (l *layouter) gradientColour(b *Box, part []css.ComponentValue) (style.RGBA, bool) {
	if w, ok := identOf(part); ok && w == "currentcolor" {
		c, ok := parseColorValue(b.Style.Get("color"))
		if !ok {
			return style.RGBA{A: 1}, true
		}
		return c, true
	}
	return style.ParseColor(part)
}

// readColourStops reads the colour stop list: stops, each a colour and up to two
// positions, with at most one transition hint between any two.
func (l *layouter) readColourStops(b *Box, s *gradientSpec, args [][]css.ComponentValue) bool {
	for i, arg := range args {
		parts := splitValueParts(arg)
		if len(parts) == 0 {
			return false
		}
		colour, isColour := l.gradientColour(b, parts[0])
		if !isColour {
			// A transition hint: one position, and nothing else, between two
			// stops.
			if len(parts) != 1 || i == 0 || i == len(args)-1 || s.items[len(s.items)-1].hint {
				return false
			}
			pos, ok := l.stopPosition(b, s, parts[0])
			if !ok {
				return false
			}
			s.items = append(s.items, gradientItem{hint: true, pos: pos, placed: true})
			continue
		}
		switch len(parts) {
		case 1:
			s.items = append(s.items, gradientItem{colour: colour})
		case 2, 3:
			// CSS Images 4's two positions are two stops of one colour.
			for _, part := range parts[1:] {
				pos, ok := l.stopPosition(b, s, part)
				if !ok {
					return false
				}
				s.items = append(s.items, gradientItem{colour: colour, pos: pos, placed: true})
			}
		default:
			return false
		}
	}
	return len(s.items) > 0
}

// stopPosition reads a stop's or a hint's position: a length-percentage, or for
// a conic gradient an angle-percentage, which is held as a percentage of a turn.
func (l *layouter) stopPosition(b *Box, s *gradientSpec, part []css.ComponentValue) (style.Length, bool) {
	if s.kind == ConicGradient {
		// An <angle-percentage>, a calc() summing an angle and a percentage
		// among them: a percentage is of a turn, so the two add as that.
		deg, pct, ok := style.ParseAnglePercentage(part)
		if !ok {
			if deg, ok = gradientAngle(part); !ok {
				return style.Length{}, false
			}
		}
		return style.Length{Kind: style.LengthPercent, Percent: pct + deg/360*100}, true
	}
	length, ok := l.lengthOfValues(b, part)
	if !ok || !isLengthPercentage(length) {
		return style.Length{}, false
	}
	return length, true
}

// resolvePx is a position in pixels along a line of the given length.
func resolvePx(l style.Length, line float64) float64 {
	switch l.Kind {
	case style.LengthPercent:
		return line * l.Percent / 100
	case style.LengthCalc:
		return line*l.Percent/100 + l.Value.Px()
	case style.LengthMath:
		// A math function over a percentage is run against the line, which
		// takes it through a Unit: a sixty-fourth of a pixel is below what a
		// gradient's stop can show.
		basis, _ := style.FromPx(line)
		v, _ := l.Resolve(basis, true)
		return v.Px()
	}
	return l.Value.Px()
}

// tinyRadius is the "arbitrary very small number greater than zero" CSS Images 3
// §3.2.3 sizes a degenerate ending shape by: a layout unit, the smallest length
// there is. What the gradient looks like with it differs from the limit the
// specification describes by less than a layout unit anywhere on the page.
const tinyRadius = style.Unit(1)

// laidGradient is a gradient laid out for one tile: the operation's value, or
// the one colour it comes to.
type laidGradient struct {
	gradient Gradient
	solid    *style.RGBA
	// tooFine is set when the solid colour is there because the gradient
	// repeats more often in the tile than maxGradientRepeats, which is a
	// limit and is reported, rather than because CSS says so.
	tooFine string
	// none is set for a gradient that is not drawn: one whose interpolation
	// in its colour space needs more stops than maxInterpolatedStops, which
	// tooMany says and is reported, or whose work the budget refused, which
	// the budget has reported.
	none    bool
	tooMany string
}

// layOut places a gradient in a tile of the given size.
//
// restate is what turns stops interpolated in the gradient's colour space into
// stops interpolated in sRGB (see gradientspace.go): interpolateStops, or the
// layouter's memo of it, which charges the document for the work. nil is
// interpolateStops charging nothing.
func (s *gradientSpec) layOut(w, h style.Unit, restate func([]GradientStop) ([]GradientStop, restated)) laidGradient {
	W, H := w.Px(), h.Px()
	var g Gradient
	g.Kind, g.Repeating = s.kind, s.repeating
	// line is what a percentage position is of, in pixels; unit is the length
	// of offset 1, which is what a position is divided by to become an offset.
	var line, unit float64
	// flat is set for CSS Images 3 §3.2.3's shape with no height, which paints
	// its last colour, or its average, everywhere.
	flat := false
	// middle is set for a linear gradient on a tile thinner than a layout unit.
	middle := false

	switch s.kind {
	case LinearGradient:
		dx, dy := math.Sin(s.angle*math.Pi/180), -math.Cos(s.angle*math.Pi/180)
		if s.corner {
			// Perpendicular to the diagonal between the two corners either
			// side of the one named, pointing towards it: see CSS Images 3
			// §3.1.1, whose consequence is that 50% runs through those two
			// corners.
			d := math.Hypot(W, H)
			dx, dy = s.cornerX*H/d, s.cornerY*W/d
		}
		line = math.Abs(W*dx) + math.Abs(H*dy)
		unit = line
		cx, cy := W/2, H/2
		g.Start = pointPx(cx-dx*line/2, cy-dy*line/2)
		g.End = pointPx(cx+dx*line/2, cy+dy*line/2)
		if g.Start == g.End {
			// A line shorter than a layout unit, which the tile then is too
			// along the axis the gradient runs, so every point of it is within
			// half a unit of the middle of the line: painted as the colour
			// there, which is exact to the grain the page is measured in.
			middle = true
		}

	case RadialGradient:
		cx := s.center.x.place(w, 0).Px()
		cy := s.center.y.place(h, 0).Px()
		g.Center = pointPx(cx, cy)
		rx, ry := s.radii(W, H, cx, cy)
		// Quantised before the cases are told apart, so that a radius under a
		// layout unit is the radius of nothing it rounds to.
		rxU, _ := style.FromPx(rx)
		ryU, _ := style.FromPx(ry)
		line = rx
		switch {
		case s.circle && rxU <= 0:
			// A circle of no radius is drawn as the smallest there is. A
			// percentage of nothing is nothing; a length is where it was.
			g.RadiusX, g.RadiusY = tinyRadius, tinyRadius
			line = 0
		case rxU <= 0:
			// No width: as small a width as there is and as great a height,
			// which is a linear gradient mirrored about the centre.
			g.RadiusX, g.RadiusY = tinyRadius, clipUnbounded
			line = 0
		case ryU <= 0:
			flat = true
		default:
			g.RadiusX, g.RadiusY = rxU, ryU
		}
		unit = g.RadiusX.Px()

	case ConicGradient:
		g.Center = pointPx(s.center.x.place(w, 0).Px(), s.center.y.place(h, 0).Px())
		g.FromAngle = s.from
		// Positions are already fractions of a turn, as percentages.
		line, unit = 1, 1
	}

	stops := s.fixUp(line)
	for i := range stops {
		if unit > 0 {
			stops[i].Offset /= unit
		}
	}
	if s.space != spaceSRGB {
		if restate == nil {
			restate = func(st []GradientStop) ([]GradientStop, restated) {
				return interpolateStops(st, s.space, s.hue, nil)
			}
		}
		out, how := restate(stops)
		switch how {
		case restatedTooMany:
			return laidGradient{none: true, tooMany: fmt.Sprintf(
				"interpolating it in its colour space to within half an 8-bit step takes "+
					"more than the %d colour stops this engine draws", maxInterpolatedStops)}
		case restatedRefused:
			return laidGradient{none: true}
		}
		stops = out
	}
	g.Stops = stops

	uniform := true
	for _, st := range stops[1:] {
		if st.Color != stops[0].Color {
			uniform = false
			break
		}
	}
	if uniform {
		// One colour, however it is arranged: the fill that colour is, and the
		// display list a page writing background-color would have.
		c := stops[0].Color
		return laidGradient{solid: &c}
	}
	if middle {
		c := g.ColorAtOffset(0.5)
		return laidGradient{solid: &c}
	}
	if flat {
		c := stops[len(stops)-1].Color
		if s.repeating {
			c = averageColor(stops)
		}
		return laidGradient{solid: &c}
	}
	if s.repeating {
		period := stops[len(stops)-1].Offset - stops[0].Offset
		// The shortest a period is anywhere on the page, in pixels: along the
		// line of a linear gradient, along the shorter axis of a radial one,
		// and round the widest circle a conic one draws in the tile.
		physical := period * unit
		repeats := 0.0
		switch s.kind {
		case RadialGradient:
			physical = period * math.Min(g.RadiusX.Px(), g.RadiusY.Px())
			far := 0.0
			for _, c := range [4][2]float64{{0, 0}, {W, 0}, {0, H}, {W, H}} {
				far = math.Max(far, g.offsetOf(c[0], c[1]))
			}
			repeats = far / period
		case ConicGradient:
			reach := 0.0
			for _, c := range [4][2]float64{{0, 0}, {W, 0}, {0, H}, {W, H}} {
				reach = math.Max(reach, math.Hypot(c[0]-g.Center.X.Px(), c[1]-g.Center.Y.Px()))
			}
			physical = period * 2 * math.Pi * reach
			repeats = 1 / period
		default:
			repeats = 1 / period
		}
		switch {
		case !(period > 0) || physical < tinyRadius.Px():
			// CSS Images 3 §3.3: a period of nothing, or one too small for any
			// device to show — and a layout unit is finer than any printer —
			// is its average colour.
			c := averageColor(stops)
			return laidGradient{solid: &c}
		case repeats > float64(maxGradientRepeats):
			c := averageColor(stops)
			return laidGradient{solid: &c, tooFine: fmt.Sprintf(
				"it repeats about %.0f times in each tile, past the %d this engine draws",
				repeats, maxGradientRepeats)}
		}
	}
	return laidGradient{gradient: g}
}

// pointPx is a point from coordinates in pixels.
func pointPx(x, y float64) Point {
	px, _ := style.FromPx(x)
	py, _ := style.FromPx(y)
	return Point{X: px, Y: py}
}

// radii is a radial gradient's ending shape, in pixels, for a tile W by H with
// the centre at cx, cy: CSS Images 3 §3.2.1's sizes.
//
// The box's sides are taken as lines running on for ever, which the definition
// asks for, so a distance to one is a distance to the line and is never
// negative. A zero comes back where the definition gives one; what that means is
// the caller's, in §3.2.3.
func (s *gradientSpec) radii(W, H, cx, cy float64) (rx, ry float64) {
	left, right := math.Abs(cx), math.Abs(W-cx)
	top, bottom := math.Abs(cy), math.Abs(H-cy)
	switch s.extent {
	case extentExplicit:
		return resolvePx(s.sizeX, W), resolvePx(s.sizeY, H)
	case extentClosestSide:
		if s.circle {
			r := math.Min(math.Min(left, right), math.Min(top, bottom))
			return r, r
		}
		return math.Min(left, right), math.Min(top, bottom)
	case extentFarthestSide:
		if s.circle {
			r := math.Max(math.Max(left, right), math.Max(top, bottom))
			return r, r
		}
		return math.Max(left, right), math.Max(top, bottom)
	}
	// The corners. The one closest to or farthest from the centre, and for an
	// ellipse the proportions the matching side keyword would give, scaled to
	// pass through it.
	closest := s.extent == extentClosestCorner
	dx, dy := math.Max(left, right), math.Max(top, bottom)
	sx, sy := dx, dy
	if closest {
		dx, dy = math.Min(left, right), math.Min(top, bottom)
		sx, sy = dx, dy
	}
	if s.circle {
		r := math.Hypot(dx, dy)
		return r, r
	}
	switch {
	case sx == 0:
		return 0, sy
	case sy == 0:
		return sx, 0
	}
	// x²/a² + y²/b² = 1 through (dx, dy), with a/b = sx/sy.
	a := math.Sqrt(dx*dx + dy*dy*sx*sx/(sy*sy))
	return a, a * sy / sx
}

// fixUp resolves the stop list against a line of the given length, in pixels
// (a turn, for a conic gradient), and applies CSS Images 4 §3.5.3's fixup: the
// ends placed, nothing before what precedes it, and runs of unplaced stops
// spread evenly. Hints then become the exponents of their segments.
//
// The positions that come back are in the same units the line was given in.
func (s *gradientSpec) fixUp(line float64) []GradientStop {
	type placed struct {
		hint   bool
		colour style.RGBA
		at     float64
		ok     bool
	}
	items := make([]placed, len(s.items))
	for i, it := range s.items {
		items[i] = placed{hint: it.hint, colour: it.colour, ok: it.placed}
		if it.placed {
			items[i].at = resolvePx(it.pos, line)
		}
	}
	// 1. The ends.
	firstStop, lastStop := -1, -1
	for i, it := range items {
		if !it.hint {
			if firstStop < 0 {
				firstStop = i
			}
			lastStop = i
		}
	}
	if !items[firstStop].ok {
		items[firstStop].at, items[firstStop].ok = 0, true
	}
	if !items[lastStop].ok {
		items[lastStop].at, items[lastStop].ok = line, true
	}
	// 2. Nothing before the largest position specified ahead of it.
	most := math.Inf(-1)
	for i := range items {
		if !items[i].ok {
			continue
		}
		if items[i].at < most {
			items[i].at = most
		}
		most = items[i].at
	}
	// 3. Each run of unplaced stops, evenly between the placed stops around it.
	// Hints are not stops and do not break a run.
	var stops []int
	for i, it := range items {
		if !it.hint {
			stops = append(stops, i)
		}
	}
	for k := 0; k < len(stops); k++ {
		if items[stops[k]].ok {
			continue
		}
		j := k
		for !items[stops[j]].ok {
			j++
		}
		lo, hi := items[stops[k-1]].at, items[stops[j]].at
		step := (hi - lo) / float64(j-k+1)
		for m := k; m < j; m++ {
			items[stops[m]].at = lo + step*float64(m-k+1)
			items[stops[m]].ok = true
		}
		k = j
	}

	out := make([]GradientStop, 0, len(stops)+2)
	for i := 0; i < len(items); i++ {
		it := items[i]
		if !it.hint {
			out = append(out, GradientStop{Offset: it.at, Color: it.colour, Exponent: 1})
			continue
		}
		// A hint: the next item is the stop it leads to, and the last stop
		// out is the one it leads from.
		a := out[len(out)-1]
		next := items[i+1]
		span := next.at - a.Offset
		i++
		if span <= 0 {
			// The two stops are at one position, so the colour changes there
			// at once and there is no transition for the hint to shape.
			out = append(out, GradientStop{Offset: next.at, Color: next.colour, Exponent: 1})
			continue
		}
		hint := (it.at - a.Offset) / span
		switch {
		case hint <= 0:
			// The halfway colour at the first stop: C = P^(log_0 .5) = P^0,
			// which is the second colour everywhere past the first stop. That
			// is a hard stop there, and is written as one.
			out = append(out,
				GradientStop{Offset: a.Offset, Color: next.colour, Exponent: 1},
				GradientStop{Offset: next.at, Color: next.colour, Exponent: 1})
		case hint >= 1:
			// And at the second, where it is the first colour all the way.
			out = append(out,
				GradientStop{Offset: next.at, Color: a.Color, Exponent: 1},
				GradientStop{Offset: next.at, Color: next.colour, Exponent: 1})
		default:
			out = append(out, GradientStop{
				Offset: next.at, Color: next.colour,
				Exponent: math.Log(0.5) / math.Log(hint),
			})
		}
	}
	return out
}
