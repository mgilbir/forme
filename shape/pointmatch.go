package shape

import "fmt"

// Components placed by matching points, in an instance.
//
// A composite glyph places each component either at an offset or by matching
// points: with ARGS_ARE_XY_VALUES clear, its two arguments are the number of a
// point already gathered for the composite and the number of one of the
// component's, and the component is moved so that the second lands on the
// first. In a variable font both points move with gvar, so where the component
// goes is decided at the location, from the points as they are there.
//
// HarfBuzz decides it in Glyph::get_points. The composite's points so far are
// one list, counted from the start of the outermost glyph drawn — not from the
// start of the composite the component is in. A component's points are
// gathered into it, followed by the component's four phantom points; then they
// are put through the component's transform and moved by its offset, which
// for a matched component is nothing but what gvar moves its point by; then
// the match is made, point p1 of the whole list and point p2 of the component's
// own, phantom points included; and the phantom points are dropped. A match
// naming a point either list does not have is not made.
//
// # Kept, or placed
//
// fontTools' instancer and HarfBuzz's both write a matched component as it
// was: the same two point numbers, for whatever reads the instance to match
// again among the instanced points. For the ordinary match — p1 a point
// gathered before the component, p2 one of the component's outline points —
// that is right, since the points it matches are points of the instance, and
// this does the same.
//
// Every other match comes out elsewhere when the instance is read than where
// HarfBuzz draws it at the location. A phantom point is where the glyph's
// metrics put it, and an instance states a vertical one only where it has
// vmtx and a horizontal one only as far as its advance agrees with gvar's.
// And gvar's delta for a matched component, which an ordinary match makes
// moot, stays in the drawing when the match names a point of the component
// itself, which moved with it, or names no point at all — while an instance
// has nowhere to keep a delta for a matched component. Tested against
// HarfBuzz 14.5.0, its own instancer's output of such a face draws the
// component up to thirty-three units from where it draws the variable face.
// (fontTools' instancer cannot instance such a face at all: it counts no
// phantom points and indexes past the end of its list.)
//
// So such a component is placed here: made into one at an offset, the offset
// being where HarfBuzz put it at the location, rounded. The one thing that
// cannot be placed once for good is a composite placed so in the middle of
// another — its match counts from the start of the outermost glyph, and so
// lands differently under each — and that is refused.

// matchWalk gathers a glyph's points as Glyph::get_points does, to make its
// matches.
type matchWalk struct {
	glyph  func(gid int) (*varGlyph, error)
	points [][2]float64
	budget *int64
	// placed, where it is not nil, receives for each component of the glyph
	// walked at the top where HarfBuzz put it: see componentPlacement.
	placed []componentPlacement
}

// componentPlacement is where one component went: the offset from its own
// points, through its transform, to where they were drawn — an offset in the
// sense of a component whose flags do not ask for it to be scaled — and
// whether it was matched in a way an instance keeps (see ordinaryMatch).
type componentPlacement struct {
	dx, dy   float64
	ordinary bool
}

// walk appends a glyph's outline points to the walk and returns its four
// phantom points.
func (w *matchWalk) walk(gid, depth int) ([4][2]float64, error) {
	if depth > maxComponentDepth {
		return [4][2]float64{}, fmt.Errorf("its components nest more than %d deep", maxComponentDepth)
	}
	g, err := w.glyph(gid)
	if err != nil || g == nil {
		return [4][2]float64{}, err
	}
	if *w.budget -= int64(g.numPoints()); *w.budget < 0 {
		return [4][2]float64{}, fmt.Errorf("placing this font's matched components needs more than %d point operations", int64(maxInstanceWork))
	}
	if !g.composite {
		for i := 0; i < g.numOutlinePoints(); i++ {
			w.points = append(w.points, [2]float64{g.x[i], g.y[i]})
		}
		return g.phantoms(), nil
	}
	phantoms := g.phantoms()
	for i, c := range g.comps {
		old := len(w.points)
		own, err := w.walk(c.glyph, depth+1)
		if err != nil {
			return [4][2]float64{}, err
		}
		w.points = append(w.points, own[:]...)
		pts := w.points[old:]
		if c.flags&compUseMyMetrics != 0 {
			phantoms = own
		}
		placeComponent(pts, c, g.x[i], g.y[i])
		var dx, dy float64
		if c.matched && c.p1 < len(w.points) && c.p2 < len(pts) {
			dx, dy = w.points[c.p1][0]-pts[c.p2][0], w.points[c.p1][1]-pts[c.p2][1]
			for k := range pts {
				pts[k][0] += dx
				pts[k][1] += dy
			}
		}
		if depth == 0 && w.placed != nil {
			tx, ty := g.x[i], g.y[i]
			if c.flags&(compScaledOffset|compUnscaledOffset) == compScaledOffset {
				s := c.scale
				tx, ty = float64(s[0]*g.x[i])+float64(s[2]*g.y[i]), float64(s[1]*g.x[i])+float64(s[3]*g.y[i])
			}
			w.placed = append(w.placed, componentPlacement{
				dx: tx + dx, dy: ty + dy,
				ordinary: c.matched && ordinaryMatch(c, old, len(pts)-4),
			})
		}
		w.points = w.points[:len(w.points)-4]
	}
	return phantoms, nil
}

// ordinaryMatch is whether a matched component matches a point gathered before
// it to one of its own outline points: the match an instance keeps, since both
// points are points of the instance. old is how many points were gathered
// before it and n how many outline points it has.
func ordinaryMatch(c varComponent, old, n int) bool {
	return c.p1 < old && c.p2 < n
}

// placeComponent is CompositeGlyphRecord::transform_points: a component's
// points through its 2x2 and moved by its offset — moved first where its flags
// ask for the offset to be scaled.
func placeComponent(pts [][2]float64, c varComponent, tx, ty float64) {
	s := c.scale
	transform := func() {
		if s == [4]float64{1, 0, 0, 1} {
			return
		}
		for k, p := range pts {
			pts[k] = [2]float64{float64(s[0]*p[0]) + float64(s[2]*p[1]), float64(s[1]*p[0]) + float64(s[3]*p[1])}
		}
	}
	translate := func() {
		for k := range pts {
			pts[k][0] += tx
			pts[k][1] += ty
		}
	}
	if c.flags&(compScaledOffset|compUnscaledOffset) == compScaledOffset {
		translate()
		transform()
		return
	}
	transform()
	translate()
}

// placeMatchedComponents makes each matched component an instance cannot keep
// into one at the offset HarfBuzz placed it at, at the location the glyphs were
// moved to; see the note at the top of this file. glyphs are every glyph at
// the location, phantom points included.
func placeMatchedComponents(glyphs []*varGlyph, budget *int64) error {
	get := func(gid int) (*varGlyph, error) {
		if gid < 0 || gid >= len(glyphs) {
			return nil, nil
		}
		return glyphs[gid], nil
	}
	placed := make([]bool, len(glyphs))
	any := false
	for gid, g := range glyphs {
		if g == nil || !g.composite || !hasMatchedComponent(g) {
			continue
		}
		w := &matchWalk{glyph: get, budget: budget, placed: []componentPlacement{}}
		if _, err := w.walk(gid, 0); err != nil {
			return fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		for i := range g.comps {
			c := &g.comps[i]
			if !c.matched || w.placed[i].ordinary {
				continue
			}
			c.matched = false
			c.flags = (c.flags | compArgsAreXY) &^ compScaledOffset
			g.x[i], g.y[i] = w.placed[i].dx, w.placed[i].dy
			placed[gid], any = true, true
		}
	}
	if !any {
		return nil
	}
	return refusePlacedInside(glyphs, placed)
}

func hasMatchedComponent(g *varGlyph) bool {
	for _, c := range g.comps {
		if c.matched {
			return true
		}
	}
	return false
}

// refusePlacedInside refuses a font in which a composite whose matched
// component was placed is itself a component anywhere but at the very start
// of another glyph's points: there its match counts from somewhere else, and
// lands somewhere else, than where it was placed for.
func refusePlacedInside(glyphs []*varGlyph, placed []bool) error {
	counts := make([]int, len(glyphs))
	counted := make([]bool, len(glyphs))
	var count func(gid, depth int) int
	count = func(gid, depth int) int {
		if gid < 0 || gid >= len(glyphs) || glyphs[gid] == nil || depth > maxComponentDepth {
			return 0
		}
		if counted[gid] {
			return counts[gid]
		}
		g := glyphs[gid]
		n := 0
		if g.composite {
			for _, c := range g.comps {
				n += count(c.glyph, depth+1)
			}
		} else {
			n = g.numOutlinePoints()
		}
		counts[gid], counted[gid] = n, true
		return n
	}
	// seen[k][gid] is set once the glyph has been visited at the start of the
	// points drawn (k = 1) or anywhere else (k = 0): the answer is the same
	// each time, and a glyph that begins a thousand others is asked once.
	seen := [2][]bool{make([]bool, len(glyphs)), make([]bool, len(glyphs))}
	var visit func(gid int, atStart bool, depth int) error
	visit = func(gid int, atStart bool, depth int) error {
		if gid < 0 || gid >= len(glyphs) || glyphs[gid] == nil || !glyphs[gid].composite || depth > maxComponentDepth {
			return nil
		}
		k := 0
		if atStart {
			k = 1
		}
		if seen[k][gid] {
			return nil
		}
		seen[k][gid] = true
		off := 0
		for _, c := range glyphs[gid].comps {
			start := atStart && off == 0
			if !start && c.glyph >= 0 && c.glyph < len(placed) && placed[c.glyph] {
				return fmt.Errorf("fonts: glyph %d places a component by matching points in a way "+
					"an instance cannot keep, and glyph %d has it as a component after other points, "+
					"where its match would count from elsewhere; this cannot be instanced", c.glyph, gid)
			}
			if err := visit(c.glyph, start, depth+1); err != nil {
				return err
			}
			off += count(c.glyph, depth+1)
		}
		return nil
	}
	for gid := range glyphs {
		if err := visit(gid, true, 0); err != nil {
			return err
		}
	}
	return nil
}

// matchedBounds is the box of a composite glyph with a matched component
// somewhere in it, measured from an instance's own glyf as HarfBuzz reads it:
// every point gathered, each component placed and its match made. Every match
// left in an instance is an ordinary one, so no phantom point is needed.
func matchedBounds(glyf []byte, loca []uint32, numGlyphs, gid int, budget *int64) (floatBounds, error) {
	get := func(gid int) (*varGlyph, error) {
		if gid < 0 || gid >= numGlyphs {
			return nil, nil
		}
		start, end := loca[gid], loca[gid+1]
		if start >= end {
			return &varGlyph{}, nil
		}
		return decodeVarGlyph(glyf[start:end], numGlyphs)
	}
	w := &matchWalk{glyph: get, budget: budget}
	var box floatBounds
	if _, err := w.walk(gid, 0); err != nil {
		return box, err
	}
	for _, p := range w.points {
		box.add(p[0], p[1])
	}
	return box, nil
}

// matchIndex answers, for each glyph of an instance's glyf, whether a matched
// component is in it at any depth, each glyph's answer found once.
type matchIndex struct {
	glyf      []byte
	loca      []uint32
	numGlyphs int
	// known is 0 for a glyph not yet asked about, 1 for one with a match in
	// it and -1 for one without.
	known []int8
}

func (m *matchIndex) has(gid, depth int) (bool, error) {
	if gid < 0 || gid >= m.numGlyphs || depth > maxComponentDepth {
		return false, nil
	}
	if k := m.known[gid]; k != 0 {
		return k > 0, nil
	}
	has := false
	start, end := m.loca[gid], m.loca[gid+1]
	if start < end && int16(uint16(m.glyf[start])<<8|uint16(m.glyf[start+1])) < 0 {
		g, err := decodeVarGlyph(m.glyf[start:end], m.numGlyphs)
		if err != nil {
			return false, err
		}
		for _, c := range g.comps {
			if c.matched {
				has = true
				break
			}
			if has, err = m.has(c.glyph, depth+1); has || err != nil {
				break
			}
		}
		if err != nil {
			return false, err
		}
	}
	m.known[gid] = -1
	if has {
		m.known[gid] = 1
	}
	return has, nil
}
