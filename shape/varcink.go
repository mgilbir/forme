package shape

import (
	"fmt"
	"math"
	"sync"

	"github.com/mgilbir/forme/font"
)

// varcFace is what a face with a VARC table keeps to measure and to draw its
// glyphs through it: the table, the glyf outlines its leaves are drawn from
// and the gvar that moves them, and each glyph's ink once it has been asked.
// It is shared by the face's clones.
type varcFace struct {
	f          *Face
	t          *varcTable
	glyf, loca []byte
	longLoca   bool
	glyfGlyphs int
	gvar       *gvarTable
	axisCount  int
	// coords are the coordinates the face is drawn at, in 2.14: a zero for
	// each axis of a face at its default, and nil for a face with no design
	// space.
	coords []int

	// work is what measuring and drawing the face's glyphs may still spend,
	// one walk at a time: see varcFaceWork.
	work int64

	mu      sync.Mutex
	answers map[int]varcAnswer
	spent   bool
	undrawn bool
}

type varcAnswer struct {
	ext extents
	ok  bool
}

// newVARCFace reads a face's VARC table, and returns nil where it has none
// or HarfBuzz's sanitizer would refuse it.
func newVARCFace(f *Face, tables map[string][]byte, numGlyphs int) *varcFace {
	t := readVARC(tables["VARC"])
	if t == nil {
		return nil
	}
	v := &varcFace{f: f, t: t}
	head := tables["head"]
	if glyf, loca := tables["glyf"], tables["loca"]; len(glyf) > 0 && len(loca) > 0 && len(head) >= 54 {
		v.glyf, v.loca = glyf, loca
		v.longLoca = signed16(font.Be16(head, 50)) != 0
		size := 2
		if v.longLoca {
			size = 4
		}
		v.glyfGlyphs = min(max(1, len(loca)/size)-1, numGlyphs)
	}
	// A face with a design space is drawn at coordinates even at its default:
	// hb_font_create sets every axis to it, so HarfBuzz hands VARC a zero for
	// each — which is not the same as none, since a leaf with coordinates is
	// measured by its points and one with none by its glyph header.
	if n := hbFvarAxisCount(tables["fvar"]); n > 0 {
		v.coords = make([]int, n)
	}
	// gvar is read at its own count of axes, which HarfBuzz reads it at
	// whether or not fvar agrees, or has any: a component sets coordinates
	// of its own. A gvar that does not parse moves nothing, which is how
	// HarfBuzz reads one its sanitizer refuses.
	if g := tables["gvar"]; len(g) >= 6 {
		v.axisCount = font.Be16(g, 4)
		v.gvar, _ = parseGvar(g, numGlyphs, v.axisCount)
	}
	v.work = varcFaceWork(tables)
	return v
}

// varcFaceWork is the work all of a face's VARC glyphs share: a unit for each
// component, axis value and leaf point a walk visits, sixty-four a byte of
// the VARC, glyf and gvar tables, with a floor of maxFontWork. A glyph's own
// walk may spend up to HarfBuzz's 2^24 units of it, so that no one glyph a
// font asks for can take more than HarfBuzz would give it, and none after the
// face's share is gone can take anything.
func varcFaceWork(tables map[string][]byte) int64 {
	return int64(max(maxFontWork, 64*(len(tables["VARC"])+len(tables["glyf"])+len(tables["gvar"]))))
}

// glyfEntry is a glyph's glyf bytes, and nil for one loca does not place.
func (v *varcFace) glyfEntry(gid int) []byte {
	var start, end int
	if v.longLoca {
		start, end = int(font.Be32(v.loca, 4*gid)), int(font.Be32(v.loca, 4*gid+4))
	} else {
		start, end = 2*font.Be16(v.loca, 2*gid), 2*font.Be16(v.loca, 2*gid+2)
	}
	if start > end || end > len(v.glyf) {
		return nil
	}
	return v.glyf[start:end]
}

// extents is VARC::get_extents for a glyph of the face, and false where
// HarfBuzz's answer is none.
func (v *varcFace) extents(gid int) (extents, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if a, ok := v.answers[gid]; ok {
		return a.ext, a.ok
	}
	var e extents
	ok := false
	if v.work > 0 {
		e, ok = v.measure(gid, v.coords, &v.work)
	} else {
		// The face's share is gone: the glyph is left without ink, as
		// Face.LayoutLimits says.
		v.spent = true
	}
	if v.answers == nil {
		v.answers = map[int]varcAnswer{}
	}
	v.answers[gid] = varcAnswer{e, ok}
	return e, ok
}

// measure walks a glyph at the coordinates for its ink: each leaf's box,
// turned through the leaf's transform and joined. It spends from work, and
// marks the face as measured short where the walk ran out of it.
func (v *varcFace) measure(gid int, coords []int, work *int64) (extents, bool) {
	box := void32
	var w *varcWalk
	w = newVarcWalk(v.t, coords, min(maxVarcWork, *work), func(leaf int, at []int, xf xform32) bool {
		e, ok := v.leafExtents(leaf, at, &w.work)
		if !ok {
			return false
		}
		box.union(xf.box(boxOfExtents(e)))
		return true
	})
	ok := w.glyph(gid, coords, identity32, -1)
	*work -= w.used()
	if w.spent || w.work < 0 {
		// HarfBuzz stops at a budget of its own, which this one is not;
		// what was measured stands, and the face says it stopped.
		v.spent = true
	}
	if !ok {
		return extents{}, false
	}
	return box.glyphExtents(), true
}

// outlineBox is the box of what a glyph draws through VARC, for a colour
// glyph clipped to it: every point of every leaf, turned. A glyph it cannot
// draw — a CFF leaf, or past the work it may cost — is a void box, and the
// face says so.
func (v *varcFace) outlineBox(gid int) box32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	o, err := v.outline(gid, v.coords, &v.work)
	if err != nil {
		v.undrawn = true
		return void32
	}
	b := void32
	for _, p := range o.points {
		b.add(p.x, p.y)
	}
	return b
}

// limits are the bounds measuring and drawing VARC glyphs have run into. See
// Face.LayoutLimits.
func (v *varcFace) limits() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []string
	if v.spent {
		out = append(out, "measuring its variable composite glyphs (VARC) ran past the work one face "+
			"may cost, so the glyphs measured after that have no ink of their own")
	}
	if v.undrawn {
		out = append(out, "a colour glyph is clipped to a variable composite glyph (VARC) that cannot be "+
			"drawn here — one of CFF glyphs, or past the work it may cost — and is measured as clipped to nothing")
	}
	return out
}

// leafExtents is a leaf's ink at its coordinates, as glyf's get_extents_at
// answers it and then CFF's: the glyph header's box with no coordinates, the
// box of its points with any.
func (v *varcFace) leafExtents(gid int, coords []int, budget *int64) (extents, bool) {
	if len(coords) == 0 || v.glyf == nil {
		return v.f.outlineExtents(gid)
	}
	w, ok := v.f.glyfAt(gid, coords, budget)
	if !ok {
		return extents{}, false
	}
	return w.extents(), true
}

// boxOfExtents is hb_extents_t made from integer extents.
func boxOfExtents(e extents) box32 {
	x0, y0 := float64(e.xBearing), float64(e.yBearing)
	x1, y1 := x0+float64(e.width), y0+float64(e.height)
	return box32{float32(min(x0, x1)), float32(min(y0, y1)), float32(max(x0, x1)), float32(max(y0, y1))}
}

// varcOutline is a VARC glyph drawn: every leaf's points, through its
// transform, in single precision, with the ends of their contours and which
// points are on the curve — what a glyf simple glyph holds.
type varcOutline struct {
	points  []point32
	onCurve []bool
	ends    []int
}

// outline draws a glyph at the coordinates as VARC::get_path draws it,
// spending from work, and reports what it cannot draw: a CFF leaf, which has
// no glyf points, or a walk that ran out of work.
func (v *varcFace) outline(gid int, coords []int, work *int64) (*varcOutline, error) {
	out := &varcOutline{}
	var failed error
	var w *varcWalk
	w = newVarcWalk(v.t, coords, min(maxVarcWork, *work), func(leaf int, at []int, xf xform32) bool {
		if v.glyf == nil {
			if failed == nil {
				failed = fmt.Errorf("a component draws glyph %d, whose outline is CFF", leaf)
			}
			return false
		}
		g, ok := v.f.glyfAt(leaf, at, &w.work)
		if !ok {
			return false
		}
		base := len(out.points)
		for _, p := range g.points {
			x, y := xf.point(p.x, p.y)
			out.points = append(out.points, point32{x, y})
		}
		out.onCurve = append(out.onCurve, g.onCurve...)
		for _, e := range g.ends {
			out.ends = append(out.ends, base+e)
		}
		return true
	})
	w.glyph(gid, coords, identity32, -1)
	*work -= w.used()
	if w.spent || w.work < 0 {
		return nil, fmt.Errorf("drawing it needs more work than a VARC glyph may cost (%d units at most, "+
			"and what is left of the face's share)", maxVarcWork)
	}
	if failed != nil {
		return nil, failed
	}
	return out, nil
}

// flatGlyph writes a VARC glyph's outline as a glyf simple glyph: its points
// rounded to whole units, as glyf stores them, and its advance's phantom
// points given. The points are the glyph's as drawn from its origin, so the
// glyph's left side bearing must be written as its xMin, which puts the
// origin where HarfBuzz drew from.
func (o *varcOutline) flatGlyph() (*varGlyph, error) {
	if len(o.points) > 0xFFFF || len(o.ends) > math.MaxInt16 {
		return nil, fmt.Errorf("its %d points in %d contours are more than a glyf glyph holds", len(o.points), len(o.ends))
	}
	g := &varGlyph{ends: o.ends}
	g.x = make([]float64, len(o.points)+4)
	g.y = make([]float64, len(o.points)+4)
	g.flags = make([]byte, len(o.points))
	for i, p := range o.points {
		g.x[i], g.y[i] = float64(p.x), float64(p.y)
		if o.onCurve[i] {
			g.flags[i] = 0x01
		}
	}
	return g, nil
}

// outlineExtents is a glyph's ink from its outline alone: the glyph header's
// box with the side bearing hmtx states for a TrueType face, and what the
// charstring draws for a CFF one. See glyphExtents.
func (f *Face) outlineExtents(gid int) (extents, bool) {
	if f.ink != nil {
		return f.ink.extents(gid)
	}
	if f.prog == nil || f.prog.GlyphBBox == nil || gid < 0 || gid >= len(f.prog.GlyphBBox) {
		return extents{}, false
	}
	if !f.prog.GlyphNonEmpty[gid] {
		return extents{}, true
	}
	b := f.prog.GlyphBBox[gid]
	lsb := min(b[0], b[2])
	if v, ok := f.leftSideBearing(gid); ok {
		lsb = v
	}
	return extents{
		xBearing: lsb,
		yBearing: max(b[1], b[3]),
		width:    max(b[0], b[2]) - min(b[0], b[2]),
		height:   min(b[1], b[3]) - max(b[1], b[3]),
	}, true
}
