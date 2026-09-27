package layout

import (
	"fmt"
	"math"
	"strings"

	"github.com/mgilbir/forme/style"
)

// Shapes that are not rectangles.
//
// The display list drew only rectangles, and said so as a virtue: a border was
// four bands, a background a fill, and anything a backend could not draw
// directly was decomposed here. A rounded corner cannot be decomposed into
// rectangles — any number of them is a staircase — so it needs a shape the
// backend draws as a shape, and this is that shape: a path of straight lines
// and arcs of axis-aligned ellipses, which is exactly what CSS Backgrounds 3 §4
// makes a rounded box out of and exactly what a PDF path can hold.
//
// # Exact, and where it is not
//
// Every arc here is a quarter ellipse or a piece of one. A PDF path has no arc
// operator: a backend draws one as cubic Béziers, and the four-segment cubic
// approximation of a quarter ellipse is within 0.03% of its radius. That is the
// only approximation between this list and a PDF, it is the backend's, and it
// is under a hundredth of a pixel for any radius under thirty pixels; a backend
// wanting less splits each arc into more segments. Everything else — which
// corners are round, how big, where one border colour gives way to the next —
// is decided here and stated exactly.
//
// # Filling, and the even-odd rule
//
// A path is filled by the even-odd rule: a point is inside when a ray from it
// crosses the path an odd number of times. So a ring is two closed shapes, the
// border edge and the padding edge, in either direction, and a backend draws
// it with PDF's "f*" rather than having to know which way each was drawn.

// Corner is one corner's radii: the horizontal and vertical semi-axes of the
// quarter ellipse that rounds it. A corner with either one zero is square, and
// is stated with both zero.
type Corner struct{ X, Y style.Unit }

// Radii are the four corners of a rounded rectangle, CSS Backgrounds 3 §4.1.
type Radii struct {
	TopLeft, TopRight, BottomRight, BottomLeft Corner
}

// IsZero reports whether every corner is square.
func (r Radii) IsZero() bool { return r == Radii{} }

// corners is the four corners in clockwise order from the top left, which is
// the order every loop here goes round a box in.
func (r Radii) corners() [4]Corner {
	return [4]Corner{r.TopLeft, r.TopRight, r.BottomRight, r.BottomLeft}
}

func radiiOf(c [4]Corner) Radii {
	return Radii{TopLeft: c[0], TopRight: c[1], BottomRight: c[2], BottomLeft: c[3]}
}

// PathOp is what one segment of a path does.
type PathOp uint8

const (
	// MoveTo begins a new closed shape at Point.
	MoveTo PathOp = iota + 1
	// LineTo draws a straight line from the current point to Point.
	LineTo
	// ArcTo draws an arc of the ellipse centred on Center with semi-axes
	// RadiusX and RadiusY, which are aligned with the page. The points of the
	// ellipse are (Center.X + RadiusX·cos a, Center.Y + RadiusY·sin a) for an
	// angle a in degrees; since y grows down the page, a growing angle goes
	// round clockwise as the page is seen. The arc runs from StartAngle through
	// SweepAngle more, so a negative sweep goes anticlockwise. If the current
	// point is not where the arc starts, a straight line joins them first, as
	// PostScript's arc does; the arc's end is the current point afterwards.
	ArcTo
	// ClosePath draws a straight line back to where the shape began.
	ClosePath
)

// PathSegment is one step of a path. Which fields it reads is its Op's.
type PathSegment struct {
	Op    PathOp
	Point Point

	Center                 Point
	RadiusX, RadiusY       style.Unit
	StartAngle, SweepAngle float64
}

// Path is a sequence of closed shapes, each begun by a MoveTo and ended by a
// ClosePath or by the next MoveTo, which closes it as well. It is filled by the
// even-odd rule.
type Path []PathSegment

// arcPoint is a point of an arc's ellipse at an angle, in pixels.
func arcPoint(s PathSegment, deg float64) (x, y float64) {
	a := deg * math.Pi / 180
	return s.Center.X.Px() + s.RadiusX.Px()*math.Cos(a), s.Center.Y.Px() + s.RadiusY.Px()*math.Sin(a)
}

// edgePiece is one piece of a path's boundary that is monotonic down the page,
// so that a horizontal line crosses it at most once: a straight line, or a part
// of an arc that does not pass the top or bottom of its ellipse.
type edgePiece struct {
	// A straight line when arc is false.
	x0, y0, x1, y1 float64
	arc            bool
	seg            PathSegment
	a0, a1         float64 // degrees, a0 < a1
}

// pieces is the path's boundary, every shape closed, cut into pieces each of
// which a horizontal line crosses at most once.
func (p Path) pieces() []edgePiece {
	var out []edgePiece
	var sx, sy, cx, cy float64
	open := false
	line := func(x, y float64) {
		if x != cx || y != cy {
			out = append(out, edgePiece{x0: cx, y0: cy, x1: x, y1: y})
		}
		cx, cy = x, y
	}
	closeShape := func() {
		if open {
			line(sx, sy)
		}
		open = false
	}
	for _, s := range p {
		switch s.Op {
		case MoveTo:
			closeShape()
			sx, sy = s.Point.X.Px(), s.Point.Y.Px()
			cx, cy = sx, sy
			open = true
		case LineTo:
			line(s.Point.X.Px(), s.Point.Y.Px())
		case ArcTo:
			x, y := arcPoint(s, s.StartAngle)
			if !open {
				sx, sy, cx, cy, open = x, y, x, y, true
			}
			line(x, y)
			lo, hi := s.StartAngle, s.StartAngle+s.SweepAngle
			if lo > hi {
				lo, hi = hi, lo
			}
			// Cut at every angle where the ellipse is at its top or bottom —
			// 90 and 270 degrees, and every turn from them — which is where it
			// stops being monotonic down the page.
			cuts := []float64{lo}
			for k := math.Floor((lo-90)/180) + 1; 90+180*k < hi; k++ {
				cuts = append(cuts, 90+180*k)
			}
			cuts = append(cuts, hi)
			for i := 0; i+1 < len(cuts); i++ {
				if cuts[i+1] > cuts[i] {
					out = append(out, edgePiece{arc: true, seg: s, a0: cuts[i], a1: cuts[i+1]})
				}
			}
			cx, cy = arcPoint(s, s.StartAngle+s.SweepAngle)
		case ClosePath:
			closeShape()
		}
	}
	closeShape()
	return out
}

// crossesRightOf reports whether a piece crosses the horizontal line at y to
// the right of x. The ends of a piece are taken half-open — a piece counts at
// its lower end and not its upper — so that a line through the point where two
// pieces meet is counted once, which is the usual rule and what makes the count
// exact at a vertex.
func (e edgePiece) crossesRightOf(x, y float64) bool {
	if !e.arc {
		if (e.y0 > y) == (e.y1 > y) {
			return false
		}
		cx := e.x0 + (y-e.y0)*(e.x1-e.x0)/(e.y1-e.y0)
		return cx > x
	}
	x0, y0 := arcPoint(e.seg, e.a0)
	x1, y1 := arcPoint(e.seg, e.a1)
	if (y0 > y) == (y1 > y) {
		return false
	}
	ry := e.seg.RadiusY.Px()
	if ry <= 0 {
		// A flat ellipse is a straight line.
		return x0+(y-y0)*(x1-x0)/(y1-y0) > x
	}
	// Where on the piece the line crosses: sin a = (y - cy) / ry, with a in
	// the piece's own range, which is monotonic in y so has one answer.
	s := math.Max(-1, math.Min(1, (y-e.seg.Center.Y.Px())/ry))
	a := math.Asin(s) * 180 / math.Pi // in [-90, 90]
	// The same sine at a and at 180 - a, each plus whole turns: the one in
	// range is the crossing.
	mid := (e.a0 + e.a1) / 2
	best, bestD := a, math.Inf(1)
	for _, c := range []float64{a, 180 - a} {
		t := c + 360*math.Round((mid-c)/360)
		if d := math.Abs(t - mid); d < bestD {
			best, bestD = t, d
		}
	}
	cx, _ := arcPoint(e.seg, best)
	return cx > x
}

// Contains reports whether a point is inside the path by the even-odd rule.
// A point exactly on the boundary may go either way.
func (p Path) Contains(pt Point) bool {
	return p.containsPx(pt.X.Px(), pt.Y.Px(), p.pieces())
}

func (p Path) containsPx(x, y float64, pieces []edgePiece) bool {
	in := false
	for _, e := range pieces {
		if e.crossesRightOf(x, y) {
			in = !in
		}
	}
	return in
}

// Bounds is the smallest rectangle the path's shapes lie inside.
func (p Path) Bounds() Rect {
	var x0, y0, x1, y1 float64
	first := true
	add := func(x, y float64) {
		if first {
			x0, y0, x1, y1, first = x, y, x, y, false
			return
		}
		x0, y0 = math.Min(x0, x), math.Min(y0, y)
		x1, y1 = math.Max(x1, x), math.Max(y1, y)
	}
	for _, e := range p.pieces() {
		if !e.arc {
			add(e.x0, e.y0)
			add(e.x1, e.y1)
			continue
		}
		// A piece is monotonic down the page but not across it, so its extent
		// across is at its ends or where the ellipse is widest.
		for _, a := range []float64{e.a0, e.a1} {
			add(arcPoint(e.seg, a))
		}
		for k := math.Ceil(e.a0 / 180); 180*k <= e.a1; k++ {
			add(arcPoint(e.seg, 180*k))
		}
	}
	if first {
		return Rect{}
	}
	a, _ := style.FromPx(math.Floor(x0*64) / 64)
	b, _ := style.FromPx(math.Floor(y0*64) / 64)
	c, _ := style.FromPx(math.Ceil(x1*64) / 64)
	d, _ := style.FromPx(math.Ceil(y1*64) / 64)
	return Rect{X: a, Y: b, W: c.Sub(a), H: d.Sub(b)}
}

// String writes a path out exactly, which is what a key made of one needs.
func (p Path) String() string {
	parts := make([]string, 0, len(p))
	for _, s := range p {
		switch s.Op {
		case MoveTo:
			parts = append(parts, fmt.Sprintf("M%d,%d", s.Point.X, s.Point.Y))
		case LineTo:
			parts = append(parts, fmt.Sprintf("L%d,%d", s.Point.X, s.Point.Y))
		case ArcTo:
			parts = append(parts, fmt.Sprintf("A%d,%d %d,%d %g%+g", s.Center.X, s.Center.Y,
				s.RadiusX, s.RadiusY, s.StartAngle, s.SweepAngle))
		case ClosePath:
			parts = append(parts, "Z")
		}
	}
	return strings.Join(parts, " ")
}

// roundedRect is a rectangle whose corners are rounded by radii, as a path: one
// closed shape, clockwise from the start of the top edge. A square corner is a
// corner of the path and not an arc of nothing.
func roundedRect(r Rect, radii Radii) Path {
	c := radii.corners()
	x0, y0, x1, y1 := r.X, r.Y, r.Right(), r.Bottom()
	// The centre of each corner's ellipse, and the angle its quarter starts
	// at going clockwise: the top left from the left edge (180), the top
	// right from the top (270), the bottom right from the right (0) and the
	// bottom left from the bottom (90).
	centres := [4]Point{
		{X: x0.Add(c[0].X), Y: y0.Add(c[0].Y)},
		{X: x1.Sub(c[1].X), Y: y0.Add(c[1].Y)},
		{X: x1.Sub(c[2].X), Y: y1.Sub(c[2].Y)},
		{X: x0.Add(c[3].X), Y: y1.Sub(c[3].Y)},
	}
	squares := [4]Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
	p := Path{{Op: MoveTo, Point: Point{X: x0.Add(c[0].X), Y: y0}}}
	for i := 1; i <= 4; i++ {
		k := i % 4
		// The corners in the order the path meets them from the top edge:
		// top right, bottom right, bottom left, top left.
		if c[k].X <= 0 || c[k].Y <= 0 {
			p = append(p, PathSegment{Op: LineTo, Point: squares[k]})
			continue
		}
		p = append(p, PathSegment{Op: ArcTo, Center: centres[k],
			RadiusX: c[k].X, RadiusY: c[k].Y,
			StartAngle: cornerStart[k], SweepAngle: 90})
	}
	return append(p, PathSegment{Op: ClosePath})
}

// cornerStart is the angle each corner's quarter ellipse begins at going
// clockwise: the top left at 180 (its left end), the top right at 270, the
// bottom right at 0 and the bottom left at 90.
var cornerStart = [4]float64{180, 270, 0, 90}

// cornerCentre is the centre of one corner's ellipse for a rectangle and radii.
func cornerCentre(r Rect, k int, c Corner) Point {
	switch k {
	case 0:
		return Point{X: r.X.Add(c.X), Y: r.Y.Add(c.Y)}
	case 1:
		return Point{X: r.Right().Sub(c.X), Y: r.Y.Add(c.Y)}
	case 2:
		return Point{X: r.Right().Sub(c.X), Y: r.Bottom().Sub(c.Y)}
	}
	return Point{X: r.X.Add(c.X), Y: r.Bottom().Sub(c.Y)}
}

// insetRadii is CSS Backgrounds 3 §4.2's inner radii: each radius less the
// thickness beside it, and nothing where that is negative. A corner left with
// one radius and not the other is square, since that is what a corner with a
// zero radius is.
func insetRadii(r Radii, e Edges) Radii {
	c := r.corners()
	sub := func(a, b style.Unit) style.Unit {
		if d := a.Sub(b); d > 0 {
			return d
		}
		return 0
	}
	out := [4]Corner{
		{X: sub(c[0].X, e.Left), Y: sub(c[0].Y, e.Top)},
		{X: sub(c[1].X, e.Right), Y: sub(c[1].Y, e.Top)},
		{X: sub(c[2].X, e.Right), Y: sub(c[2].Y, e.Bottom)},
		{X: sub(c[3].X, e.Left), Y: sub(c[3].Y, e.Bottom)},
	}
	for i := range out {
		if out[i].X <= 0 || out[i].Y <= 0 {
			out[i] = Corner{}
		}
	}
	return radiiOf(out)
}

// safeFrom reports whether a rectangle lies clear of every rounded corner of a
// rounded rectangle, so that clipping it to the rounded shape is clipping it to
// the rectangle — which has already been done.
func safeFrom(r Rect, box Rect, radii Radii) bool {
	for k, c := range radii.corners() {
		if c.X <= 0 || c.Y <= 0 {
			continue
		}
		if !r.Intersect(cornerBox(box, k, c)).Empty() {
			return false
		}
	}
	return true
}

// cornerBox is the rectangle one corner's quarter ellipse is drawn in.
func cornerBox(box Rect, k int, c Corner) Rect {
	switch k {
	case 0:
		return Rect{X: box.X, Y: box.Y, W: c.X, H: c.Y}
	case 1:
		return Rect{X: box.Right().Sub(c.X), Y: box.Y, W: c.X, H: c.Y}
	case 2:
		return Rect{X: box.Right().Sub(c.X), Y: box.Bottom().Sub(c.Y), W: c.X, H: c.Y}
	}
	return Rect{X: box.X, Y: box.Bottom().Sub(c.Y), W: c.X, H: c.Y}
}

// outsideCorner reports whether a rectangle lies wholly outside a rounded
// rectangle's curve, in one of its corners: inside the corner's box and past
// its ellipse at the point of the rectangle nearest the ellipse's centre, which
// is the point of it nearest to being inside.
func outsideCorner(r Rect, box Rect, radii Radii) bool {
	for k, c := range radii.corners() {
		if c.X <= 0 || c.Y <= 0 || !cornerBox(box, k, c).Contains(r) {
			continue
		}
		ctr := cornerCentre(box, k, c)
		px := math.Max(r.X.Px(), math.Min(ctr.X.Px(), r.Right().Px()))
		py := math.Max(r.Y.Px(), math.Min(ctr.Y.Px(), r.Bottom().Px()))
		dx := (px - ctr.X.Px()) / c.X.Px()
		dy := (py - ctr.Y.Px()) / c.Y.Px()
		if dx*dx+dy*dy >= 1 {
			return true
		}
	}
	return false
}

// FillPath fills a shape in a solid colour, by the even-odd rule.
//
// It is what a rounded background and a rounded border are painted as. A
// FillRect is still a FillRect wherever the shape is one: a box with square
// corners paints what it always did.
type FillPath struct {
	Path  Path
	Color style.RGBA
	// Clip is §11.1's clipping, when something cuts the shape. Unlike a
	// rectangle's, a path's clip cannot be folded into it by arithmetic, so it
	// travels on the operation as a DrawImage's does. It is set only when the
	// clip cuts the shape.
	Clip Clip
	// Overhang is FillRect.Overhang, for the rounded background and border of
	// an inline box.
	Overhang bool
}

func (FillPath) isOp() {}

// ClipPath paints its operations clipped to a shape: nothing it holds marks the
// page outside Path.
//
// It holds its operations rather than bracketing them, and that is the
// difference between it and the push/pop pair visualeffects.go refuses to put
// in the list. A pair can be left open and a clip left open blanks the rest of
// the page; a ClipPath cannot be left open, because what it clips is inside it
// and nothing after it is. A PDF backend draws it as a q, the path, "W* n", the
// operations, and a Q — balanced by construction, whatever the operations are.
//
// It is emitted only where a curve cuts something: an operation that lies
// clear of every rounded corner is left where it was, unwrapped, since
// clipping it to the rectangle already happened and is all the curve would do.
// A Link is never inside one; see painter.roundClip.
type ClipPath struct {
	Path Path
	Ops  []Op
}

func (ClipPath) isOp() {}
