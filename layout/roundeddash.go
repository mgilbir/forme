package layout

import (
	"math"
	"sort"

	"github.com/mgilbir/forme/style"
)

// Dotted and dashed rounded borders.
//
// CSS Backgrounds 3 §3.2 makes a dotted border "a series of round dots" and a
// dashed one "a series of square-ended dashes", and leaves the spacing open
// with one request: "Implementations are encouraged to choose a spacing that
// makes the corners symmetrical." Round a corner and the series has to go round
// it: a mark on the curve is a piece of the band between the outer curve and
// the inner one, and the marks are spaced along the curve, not along the box.
//
// # The track
//
// Each side owns the part of the ring §4.4 gives it: from where its colour
// begins on the corner before it to where it ends on the corner after, which
// is what sideRegion draws a solid side as (see radius.go). That part is three
// pieces — the rest of the corner before, the straight run, the start of the
// corner after — and a mark is measured along it by the side's track: the
// average of the outer curve's length and the inner curve's. For a circular
// corner the two share a centre and the average is the length of the circle
// half-way between them, the middle of the border, exactly; for an ellipse it
// is the same mean of two exact lengths, and where the inner curve has shrunk
// to the corner of the padding box (a border wider than its radius) it is half
// the outer curve, so no corner is ever a place of no length that a mark could
// not be put on.
//
// A position on the track is a cut across the band: on a corner, from the
// outer curve to the inner one in one direction from the corner's centre,
// which is the cut sideRegion ends a side on; on the straight run, from the
// same fraction of the way along the outer edge to that fraction of the inner.
// A mark is the band between two cuts, so it never leaves the band and a mark
// that ends on a side's end ends exactly where that side's region does.
//
// An ellipse's arc length has no closed form, and it is integrated here —
// Gauss–Legendre on panels fine enough that a quarter ellipse up to 2,000
// pixels across and a thousand times as wide as it is tall is measured to
// within a thousandth of a pixel (checked against the complete elliptic
// integral, computed by the arithmetic–geometric mean, in the tests). What
// comes out is a direction from the corner's centre, and the display list
// states arcs by their angles, so the mark is drawn from where it is placed
// with no rounding but the backend's own Béziers.
//
// # The spacing
//
// A dash is three times the side's width long and a dot is as long as the
// width, each followed by a gap as long as itself, as paintEdge draws a
// square border (with the same one-pixel floor on the length of a mark). A
// side's track is cut into a whole number n of periods, the number nearest to
// what fits, and a mark is centred on each of the n+1 cuts: on the two ends of
// the track as well as between. The marks keep their length and the gaps take
// up what a whole number of periods leaves over, so each end of a side is
// exactly half a mark, and where two sides meet — on the line sideRegion draws
// between their colours — the two halves are one mark centred on the meeting,
// half of it on each side whatever the two sides' lengths. On a corner whose
// two sides are the same width that line is the corner's diagonal, and the
// corner's mark is symmetrical about it, which is the symmetry §3.2 asks for;
// on a square corner it is the mitre, and the mark is centred on the mitre.
//
// A dash is the band between its two cuts. A dot is a circle centred half-way
// across the band at its cut, as wide as the band is there, and never wider
// than the distance to the next dot, so no two overlap. A dot at an end is cut
// in half by the side's region, and the side's dots are drawn inside it: a
// circle is not a piece of the band, and on an ellipse or where the inner
// curve is the padding box's corner a circle as wide as the band can reach a
// little past it.
//
// A side whose track is shorter than half a period is drawn solid: it has no
// room for the gap that makes a mark a mark. Every mark is charged to the
// document's work budget before any is made, as paintDashes charges a square
// border's, and past the budget the side is drawn solid and the budget says
// so.

// trackKnots is how many intervals a corner's part of a track is tabulated
// in, for turning a length along the track back into a direction.
const trackKnots = 32

// trackPiece is one of the three pieces of a side's track.
type trackPiece struct {
	// start and length are where the piece is along the track, in pixels.
	start, length float64
	// corner is the corner the piece goes round, or -1 for the straight run.
	corner int
	// d0 and d1 are the directions from the corner's centre the piece runs
	// between, in degrees, as cornerPoint reads a direction.
	d0, d1 float64
	// dirs are directions from d0 to d1 and cum the track's length from d0 to
	// each, for inverting: see dirAt.
	dirs, cum []float64
	// o and i are the straight run's outer and inner edges, from and to, in
	// pixels.
	o, i [4]float64
}

// sideTrack is one side's part of a rounded ring, measured along its length.
type sideTrack struct {
	outer, inner Rect
	oR, iR       Radii
	pieces       [3]trackPiece
	length       float64
}

// newSideTrack is the track of side s of the ring between outer and inner,
// from the transition at the corner before it to the one at the corner after,
// with at the transitions as transitions gives them.
func newSideTrack(outer Rect, oR Radii, inner Rect, iR Radii, s side, at [4]float64) *sideTrack {
	t := &sideTrack{outer: outer, inner: inner, oR: oR, iR: iR}
	prev, next := int(s), (int(s)+1)%4
	t.pieces[0] = t.cornerPiece(prev, at[prev], cornerStart[prev]+90)
	ox0, oy0 := pointXY(cornerPoint(outer, oR, prev, cornerStart[prev]+90))
	ox1, oy1 := pointXY(cornerPoint(outer, oR, next, cornerStart[next]))
	ix0, iy0 := pointXY(cornerPoint(inner, iR, prev, cornerStart[prev]+90))
	ix1, iy1 := pointXY(cornerPoint(inner, iR, next, cornerStart[next]))
	t.pieces[1] = trackPiece{corner: -1,
		o:      [4]float64{ox0, oy0, ox1, oy1},
		i:      [4]float64{ix0, iy0, ix1, iy1},
		length: (math.Hypot(ox1-ox0, oy1-oy0) + math.Hypot(ix1-ix0, iy1-iy0)) / 2,
	}
	t.pieces[2] = t.cornerPiece(next, cornerStart[next], at[next])
	for k := range t.pieces {
		t.pieces[k].start = t.length
		t.length += t.pieces[k].length
	}
	return t
}

func pointXY(p Point) (float64, float64) { return p.X.Px(), p.Y.Px() }

// cornerPiece is the part of corner k's band between two directions, with its
// length along the track tabulated.
func (t *sideTrack) cornerPiece(k int, d0, d1 float64) trackPiece {
	pc := trackPiece{corner: k, d0: d0, d1: d1}
	oc, ic := t.oR.corners()[k], t.iR.corners()[k]
	if (oc.X <= 0 || oc.Y <= 0) || d1 <= d0 {
		// A square corner, or a side given none of it: a point on the track.
		return pc
	}
	pc.dirs = make([]float64, trackKnots+1)
	pc.cum = make([]float64, trackKnots+1)
	for j := 0; j <= trackKnots; j++ {
		pc.dirs[j] = d0 + (d1-d0)*float64(j)/trackKnots
	}
	pc.dirs[trackKnots] = d1
	for j := 1; j <= trackKnots; j++ {
		pc.cum[j] = pc.cum[j-1] + cornerTrackLength(k, oc, ic, pc.dirs[j-1], pc.dirs[j])
	}
	pc.length = pc.cum[trackKnots]
	return pc
}

// cornerTrackLength is the track's length round corner k between two
// directions: the mean of the outer curve's length and the inner's, which is
// nothing where the inner corner is square.
func cornerTrackLength(k int, oc, ic Corner, d0, d1 float64) float64 {
	l := arcLength(oc, curveAngle(oc, k, d0), curveAngle(oc, k, d1))
	if ic.X > 0 && ic.Y > 0 {
		l += arcLength(ic, curveAngle(ic, k, d0), curveAngle(ic, k, d1))
	}
	return l / 2
}

// arcPanels and the nodes and weights below are the composite Gauss–Legendre
// rule arcLength integrates with: eight points on each of arcPanels equal
// panels of a quarter turn.
const arcPanels = 16

var glNodes = [8]float64{
	-0.9602898564975363, -0.7966664774136267, -0.5255324099163290, -0.1834346424956498,
	0.1834346424956498, 0.5255324099163290, 0.7966664774136267, 0.9602898564975363,
}

var glWeights = [8]float64{
	0.1012285362903763, 0.2223810344533745, 0.3137066458778873, 0.3626837833783620,
	0.3626837833783620, 0.3137066458778873, 0.2223810344533745, 0.1012285362903763,
}

// arcLength is the length of an arc of a corner's ellipse between two of its
// angles, in degrees as a PathSegment writes them: the integral of the speed
// |d/dθ (rx cos θ, ry sin θ)| = √(rx² sin² θ + ry² cos² θ).
func arcLength(c Corner, a0, a1 float64) float64 {
	rx, ry := c.X.Px(), c.Y.Px()
	if rx <= 0 || ry <= 0 || a0 == a1 {
		return 0
	}
	t0, t1 := a0*math.Pi/180, a1*math.Pi/180
	if t1 < t0 {
		t0, t1 = t1, t0
	}
	// arcPanels to a quarter turn, and at least one: a knot interval of the
	// track is a small part of a quarter, and is measured at the same grain.
	panels := int(math.Ceil(arcPanels * (t1 - t0) / (math.Pi / 2)))
	if panels < 1 {
		panels = 1
	}
	h := (t1 - t0) / float64(panels)
	sum := 0.0
	for p := 0; p < panels; p++ {
		mid := t0 + h*(float64(p)+0.5)
		for j, x := range glNodes {
			th := mid + x*h/2
			s, c := math.Sincos(th)
			sum += glWeights[j] * math.Hypot(rx*s, ry*c)
		}
	}
	return sum * h / 2
}

// dirAt is the direction from the corner's centre at a length along a corner
// piece: the knot interval that holds it, and then bisection on the length
// from that interval's start, to a millionth of a degree.
func (pc *trackPiece) dirAt(t *sideTrack, s float64) float64 {
	if pc.length <= 0 {
		return pc.d0
	}
	switch {
	case s <= 0:
		return pc.d0
	case s >= pc.length:
		return pc.d1
	}
	j := sort.SearchFloat64s(pc.cum, s)
	if j == 0 {
		return pc.d0
	}
	lo, hi := pc.dirs[j-1], pc.dirs[j]
	want := s - pc.cum[j-1]
	oc, ic := t.oR.corners()[pc.corner], t.iR.corners()[pc.corner]
	base := lo
	for hi-lo > 1e-6 {
		mid := (lo + hi) / 2
		if cornerTrackLength(pc.corner, oc, ic, base, mid) < want {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// piece is the piece of the track a length along it is on: the last whose
// start is not past it.
func (t *sideTrack) piece(s float64) int {
	k := 0
	for j := 1; j < len(t.pieces); j++ {
		if t.pieces[j].length > 0 && s >= t.pieces[j].start {
			k = j
		}
	}
	return k
}

// cut is the two ends of the cut across the band at a length along the track:
// the point on the outer curve and the point on the inner, in pixels.
func (t *sideTrack) cut(s float64) (ox, oy, ix, iy float64) {
	pc := &t.pieces[t.piece(s)]
	local := s - pc.start
	if pc.corner < 0 {
		u := 0.0
		if pc.length > 0 {
			u = math.Max(0, math.Min(1, local/pc.length))
		}
		return pc.o[0] + (pc.o[2]-pc.o[0])*u, pc.o[1] + (pc.o[3]-pc.o[1])*u,
			pc.i[0] + (pc.i[2]-pc.i[0])*u, pc.i[1] + (pc.i[3]-pc.i[1])*u
	}
	d := pc.dirAt(t, local)
	ox, oy = curvePointPx(t.outer, t.oR, pc.corner, d)
	ix, iy = curvePointPx(t.inner, t.iR, pc.corner, d)
	return ox, oy, ix, iy
}

// curvePointPx is cornerPoint in pixels and unrounded.
func curvePointPx(box Rect, radii Radii, k int, dir float64) (float64, float64) {
	c := radii.corners()[k]
	ctr := cornerCentre(box, k, c)
	if c.X <= 0 || c.Y <= 0 {
		return ctr.X.Px(), ctr.Y.Px()
	}
	return arcPoint(PathSegment{Center: ctr, RadiusX: c.X, RadiusY: c.Y}, curveAngle(c, k, dir))
}

// span is the band between two lengths along the track, as one closed shape:
// along the outer curve from the first cut to the second, across, and back
// along the inner.
func (t *sideTrack) span(s0, s1 float64, p Path) Path {
	ox, oy, _, _ := t.cut(s0)
	p = append(p, PathSegment{Op: MoveTo, Point: pointPx(ox, oy)})
	type part struct {
		k      int
		a, b   float64 // lengths along the piece
		da, db float64 // directions, for a corner
	}
	var parts []part
	for k := range t.pieces {
		pc := &t.pieces[k]
		a, b := math.Max(s0, pc.start), math.Min(s1, pc.start+pc.length)
		if b < a {
			continue
		}
		pt := part{k: k, a: a - pc.start, b: b - pc.start}
		if pc.corner >= 0 {
			pt.da, pt.db = pc.dirAt(t, pt.a), pc.dirAt(t, pt.b)
		}
		parts = append(parts, pt)
	}
	for _, pt := range parts {
		pc := &t.pieces[pt.k]
		if pc.corner >= 0 {
			p = cornerArc(p, t.outer, t.oR, pc.corner, pt.da, pt.db)
			continue
		}
		x, y, _, _ := t.cut(pc.start + pt.b)
		p = append(p, PathSegment{Op: LineTo, Point: pointPx(x, y)})
	}
	_, _, ix, iy := t.cut(s1)
	p = append(p, PathSegment{Op: LineTo, Point: pointPx(ix, iy)})
	for j := len(parts) - 1; j >= 0; j-- {
		pt := parts[j]
		pc := &t.pieces[pt.k]
		if pc.corner >= 0 {
			p = cornerArc(p, t.inner, t.iR, pc.corner, pt.db, pt.da)
			continue
		}
		_, _, x, y := t.cut(pc.start + pt.a)
		p = append(p, PathSegment{Op: LineTo, Point: pointPx(x, y)})
	}
	return append(p, PathSegment{Op: ClosePath})
}

// markLength is the length of a dash or a dot along a side of width w: see the
// note at the top of this file.
func markLength(w style.Unit, dotted bool) float64 {
	unit := w.Px()
	if unit < 1 {
		unit = 1
	}
	if !dotted {
		unit *= 3
	}
	return unit
}

// roundedMarks paints one dotted or dashed side of a rounded ring, between
// outer and inner, as the note at the top of this file describes.
func (p *painter) roundedMarks(outer Rect, oR Radii, inner Rect, iR Radii, s side,
	at [4]float64, w style.Unit, dotted bool, colour style.RGBA) {

	region := sideRegion(outer, oR, inner, iR, s, at)
	t := newSideTrack(outer, oR, inner, iR, s, at)
	mark := markLength(w, dotted)
	n := math.Round(t.length / (2 * mark))
	if !(n >= 1) || n > float64(maxInt32) ||
		!p.rec.charge(int64(n+1)*costOp, "the dashes and dots of the borders past that point, drawn solid") {
		p.emit(FillPath{Path: region, Color: colour})
		return
	}
	count := int(n)
	step := t.length / float64(count)
	var marks Path
	if !dotted {
		// The nearest whole number of periods leaves each at least one and a
		// half marks long (and one exactly at worst, when there is one), so a
		// dash of its own length never reaches the next.
		half := math.Min(mark, step) / 2
		for k := 0; k <= count; k++ {
			at := step * float64(k)
			marks = t.span(math.Max(0, at-half), math.Min(t.length, at+half), marks)
		}
		// Paid for above, so appended rather than emitted.
		p.ops = append(p.ops, FillPath{Path: marks, Color: colour})
		return
	}
	for k := 0; k <= count; k++ {
		ox, oy, ix, iy := t.cut(step * float64(k))
		r := math.Min(math.Hypot(ix-ox, iy-oy), step) / 2
		ru, _ := style.FromPx(r)
		if ru <= 0 {
			continue
		}
		ctr := pointPx((ox+ix)/2, (oy+iy)/2)
		marks = append(marks,
			PathSegment{Op: MoveTo, Point: Point{X: ctr.X.Add(ru), Y: ctr.Y}},
			PathSegment{Op: ArcTo, Center: ctr, RadiusX: ru, RadiusY: ru, StartAngle: 0, SweepAngle: 360},
			PathSegment{Op: ClosePath})
	}
	if len(marks) > 0 {
		p.ops = append(p.ops, ClipPath{Path: region, Ops: []Op{FillPath{Path: marks, Color: colour}}})
	}
}

// maxInt32 bounds a count of marks before it is converted to an int, far past
// anything the budget pays for.
const maxInt32 = 1<<31 - 1
