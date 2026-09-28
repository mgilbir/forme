package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// A TrueType glyph's points at any coordinates, as HarfBuzz's Glyph::get_points
// gathers them for a VARC leaf: gvar's deltas applied where any coordinate is
// not zero, components placed and matched, phantom points carried, and the
// whole shifted at the top by where the left phantom point ended up.

// hbMaxGlyfPoints is HB_GLYF_MAX_POINTS, as colrink.go's maxGlyfPoints.
const hbMaxGlyfPoints = maxGlyfPoints

// glyfAtWalk gathers one leaf's points: HarfBuzz's all_points, its contours'
// ends and each point's on-curve bit for whoever draws them, and its bounds
// on the walk.
type glyfAtWalk struct {
	f       *Face
	coords  []float64
	varied  bool
	points  []point32
	onCurve []bool
	ends    []int
	edges   int
	dec     decycler
	budget  *int64
}

// glyfAt gathers a glyph of the face at the coordinates, and reports false
// where HarfBuzz's glyf reader draws nothing of it.
func (f *Face) glyfAt(gid int, coords []int, budget *int64) (*glyfAtWalk, bool) {
	v := f.varc
	if v == nil || v.glyf == nil || gid < 0 || gid >= v.glyfGlyphs {
		return nil, false
	}
	w := &glyfAtWalk{f: f, dec: decycler{tortoise: -1}, budget: budget}
	for _, c := range coords {
		w.varied = w.varied || c != 0
	}
	if w.varied && v.gvar != nil {
		w.coords = make([]float64, v.axisCount)
		for i := range w.coords {
			if i < len(coords) {
				w.coords[i] = float64(coords[i]) / 16384
			}
		}
	}
	ph, ok := w.glyph(gid, 0)
	if !ok {
		return nil, false
	}
	if shift := ph[0].x; shift != 0 {
		for i := range w.points {
			w.points[i].x = float32(w.points[i].x - shift)
		}
	}
	return w, true
}

// glyph appends a glyph's outline points and returns its phantom points, as
// Glyph::get_points does below the top.
func (w *glyfAtWalk) glyph(gid, depth int) ([4]point32, bool) {
	var none [4]point32
	if depth > hbMaxNesting || w.edges > hbMaxEdges {
		return none, false
	}
	w.edges++
	v := w.f.varc
	b := v.glyfEntry(gid)
	g, err := decodeVarGlyph(b, v.glyfGlyphs)
	if err != nil {
		return none, false
	}
	if *w.budget -= int64(g.numPoints()) + 16; *w.budget < 0 {
		return none, false
	}
	xMin, yMax := 0, 0
	if len(b) >= 10 {
		xMin, yMax = signed16(font.Be16(b, 2)), signed16(font.Be16(b, 8))
	}
	// The phantom points before any variation: glyfPhantoms, which HarfBuzz
	// sets from hmtx and vmtx, or an em where there is no vmtx.
	p := (&colrInk{f: w.f}).glyfPhantoms(gid, xMin, yMax)
	g.setPhantoms(0, 0, 0)
	n := len(g.x)
	for i := 0; i < 4; i++ {
		g.x[n-4+i], g.y[n-4+i] = float64(p[i].x), float64(p[i].y)
	}
	if w.coords != nil {
		if err := v.gvar.applyGlyph(gid, g, w.coords, w.budget); err != nil {
			return none, false
		}
	}
	phantoms := [4]point32{}
	for i := range phantoms {
		phantoms[i] = point32{float32(g.x[n-4+i]), float32(g.y[n-4+i])}
	}
	if !g.composite {
		start := len(w.points)
		for i := 0; i < g.numOutlinePoints(); i++ {
			w.points = append(w.points, point32{float32(g.x[i]), float32(g.y[i])})
			w.onCurve = append(w.onCurve, g.flags[i]&0x01 != 0)
		}
		for _, e := range g.ends {
			w.ends = append(w.ends, start+e)
		}
		return phantoms, len(w.points) <= hbMaxGlyfPoints
	}
	w.dec.enter()
	defer w.dec.leave()
	for i, c := range g.comps {
		if !w.dec.visit(c.glyph) {
			continue
		}
		old := len(w.points)
		own, ok := w.glyph(c.glyph, depth+1)
		if !ok {
			return none, false
		}
		w.points = append(w.points, own[:]...)
		w.onCurve = append(w.onCurve, false, false, false, false)
		pts := w.points[old:]
		if c.flags&compUseMyMetrics != 0 {
			phantoms = own
		}
		if *w.budget -= int64(len(pts)); *w.budget < 0 {
			return none, false
		}
		placeComponent32(pts, c, float32(g.x[i]), float32(g.y[i]))
		if c.matched && c.p1 < len(w.points) && c.p2 < len(pts) {
			translate32(pts, float32(w.points[c.p1].x-pts[c.p2].x), float32(w.points[c.p1].y-pts[c.p2].y))
		}
		w.points = w.points[:len(w.points)-4]
		w.onCurve = w.onCurve[:len(w.onCurve)-4]
		if len(w.points) > hbMaxGlyfPoints {
			return none, false
		}
	}
	return phantoms, true
}

// placeComponent32 is CompositeGlyphRecord::transform_points in single
// precision, as HarfBuzz's contour points are.
func placeComponent32(pts []point32, c varComponent, tx, ty float32) {
	m := [4]float32{float32(c.scale[0]), float32(c.scale[1]), float32(c.scale[2]), float32(c.scale[3])}
	transform := func() {
		if m == [4]float32{1, 0, 0, 1} {
			return
		}
		for k, p := range pts {
			pts[k] = point32{float32(p.x*m[0]) + float32(p.y*m[2]), float32(p.x*m[1]) + float32(p.y*m[3])}
		}
	}
	if c.flags&(compScaledOffset|compUnscaledOffset) == compScaledOffset {
		translate32(pts, tx, ty)
		transform()
		return
	}
	transform()
	translate32(pts, tx, ty)
}

// extents is glyf's points_aggregator_t: the box of the points, each edge
// rounded as roundf rounds, and nothing for a box with no area.
func (w *glyfAtWalk) extents() extents {
	b := box32{math.MaxFloat32, math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, p := range w.points {
		b.xMin, b.yMin = min(b.xMin, p.x), min(b.yMin, p.y)
		b.xMax, b.yMax = max(b.xMax, p.x), max(b.yMax, p.y)
	}
	if b.xMin >= b.xMax || b.yMin >= b.yMax {
		return extents{}
	}
	r := func(v float32) float64 { return math.Floor(float64(float32(v + 0.5))) }
	x, y := r(b.xMin), r(b.yMax)
	return scaledExtents(extents{
		xBearing: int(clampToInt32(x)),
		width:    int(clampToInt32(r(b.xMax) - x)),
		yBearing: int(clampToInt32(y)),
		height:   int(clampToInt32(r(b.yMin) - y)),
	})
}
