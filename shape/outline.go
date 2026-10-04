package shape

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mgilbir/forme/font"
)

// The outline of a glyph, as path segments, for a caller that draws the glyphs
// itself.
//
// forme already reads every outline it can be asked about — a glyf glyph's
// points for its box, a CFF charstring's moves and curves for the same, VARC's
// leaves for a variable composite — and what it hands out of them is a box. A
// caller that rasterizes text, or turns it into shapes, had to parse the
// program (Face.Program) a second time with a library that has to agree with
// this one about which glyph is which and about what a composite, a seac or a
// variable instance draws. GlyphOutline hands out the walk that measuring
// already does, so the outline drawn is the outline measured.
//
// # Where each glyph's outline comes from
//
// In the order Face.GlyphExtents asks: a glyph a VARC table composes is drawn
// through it, then a TrueType face's glyf, then a CFF face's charstrings. A
// CFF2 face is read as the CFF it is cut down to at its default instance
// (cff2cff.go), which is what it is measured, subsetted and embedded as; a face
// from LoadInstance is the outlines of the instance, since that is what
// LoadInstance writes.
//
// # What it costs, and who pays
//
// Reading an outline is the same work as measuring the glyph, and is bounded
// the same way: a charstring runs at most HarfBuzz's 200,000 operators, a glyf
// glyph gathers at most its 200,000 points and a VARC glyph spends at most its
// 2^24 units. Beyond that, drawing is charged to a budget of the face's own,
// sized as the one its CFF glyphs are measured under (cffInkWork), so a font
// whose every glyph asks for as much as it may cannot be drawn without limit
// by a caller that asks for every glyph. It is not the measuring budget: a
// caller that draws the same digits ten thousand times must not be the reason
// a later paragraph's glyphs are placed as glyphs with no ink. What makes that
// safe is that a glyph drawn once is kept, and is charged only the once — up to
// maxCachedSegments segments, past which a glyph is drawn again every time it is
// asked for and charged for it every time.

// ErrNoOutline is what GlyphOutline reports, wrapped in an account of which
// case it is, for a glyph a face has no outline to give: a standard face,
// which has no font program, and a glyph that is drawn as a bitmap only.
var ErrNoOutline = errors.New("shape: the glyph has no outline")

// SegmentOp says what a Segment does.
type SegmentOp uint8

// The four things a path does. Each says which of a Segment's points it uses.
const (
	// MoveTo begins a contour at Pts[0].
	MoveTo SegmentOp = iota + 1
	// LineTo draws a straight line to Pts[0].
	LineTo
	// QuadTo draws a quadratic curve to Pts[1], with the control point Pts[0].
	QuadTo
	// CubicTo draws a cubic curve to Pts[2], with the control points Pts[0] and
	// Pts[1].
	CubicTo
)

// Point is a place in a glyph's outline, in font units, with y increasing
// upwards as the font states it.
type Point struct{ X, Y float64 }

// Segment is one step of a glyph's outline. The points a step does not use are
// zero; see the operations for which it does.
type Segment struct {
	Op  SegmentOp
	Pts [3]Point
}

// maxCachedSegments bounds what a face keeps of the outlines it has drawn: at
// 56 bytes a segment, about fourteen megabytes. It is the memory bound, where
// outlineWork is the time one.
const maxCachedSegments = 1 << 18

// outlineCache is what a face keeps to draw its glyphs: the work it may spend
// on it, and the outlines already drawn. It is shared by the face's clones, as
// the layout cache is, because an outline is the font's and no document
// changes it; the lock is there because it is filled in lazily.
type outlineCache struct {
	mu sync.Mutex
	// budget is the work drawing may still spend, in the units the font's
	// budget counts: an operator of a charstring, a point of a glyf glyph.
	budget *font.Budget
	// varcWork is the same for the glyphs VARC composes, which count their
	// work in the units varcFace does.
	varcWork int64
	done     map[int][]Segment
	cached   int
}

// outlineWork is the work drawing all of a face's glyphs may cost: as much as
// its CFF glyphs may cost to measure, sized by the whole program so that it is
// the same for a face of any of the outline formats.
func outlineWork(programLen int) int { return cffInkWork(programLen) }

func newOutlineCache(programLen int, tables map[string][]byte) *outlineCache {
	return &outlineCache{
		budget:   font.NewBudget(outlineWork(programLen)),
		varcWork: varcFaceWork(tables),
		done:     map[int][]Segment{},
	}
}

// GlyphOutline hands yield the outline of a glyph, in font units and one
// segment at a time, and reports why it cannot where it cannot. It is the
// outline forme measures and embeds: whatever the font's program draws for the
// glyph at the point of its design space the face was loaded at.
//
// The glyph is a glyph index, as Glyph.GID is. A contour is a MoveTo and what
// follows it up to the next MoveTo, and is closed: the line back to where it
// began is not a segment of its own. A TrueType contour's points are drawn as
// the quadratic curves they name, an off-curve point pair standing for the
// on-curve point between them, so QuadTo and LineTo are all it uses; a CFF
// charstring's are lines and cubic curves. A glyph with nothing to draw — a
// space — yields nothing and is not an error. Iteration stops when yield
// returns false, which is not an error either.
//
// The outline is drawn before anything is yielded, so an error comes with no
// segments: a caller never has half a glyph to discard.
//
// What is resolved for the caller:
//   - A TrueType composite is its components, each through its transform and its
//     offset, and one placed by matching points is moved so that the two land
//     together. The whole is set so that the glyph's left side bearing is the
//     one hmtx states, as HarfBuzz draws it; for a font whose glyph boxes
//     agree with hmtx, which is nearly every font, that is no move at all.
//   - A CFF charstring is run with its subroutines called, its hints ignored
//     and its flex drawn as the two curves it is; a seac draws the base and
//     the accent it names, the accent moved by the offset it states.
//   - A CFF2 charstring is drawn at the face's location, its blends resolved,
//     as the CFF it is written as.
//   - A glyph a VARC table composes is drawn through it, each of its leaves in
//     place, as far as its leaves are glyf glyphs: see below.
//
// Where it returns an error:
//   - A glyph index the face does not have.
//   - A standard face, which has no font program, and a glyph whose only image
//     is a bitmap (sbix or CBDT) and which has no outline to draw. Both are
//     ErrNoOutline. A colour glyph (COLR) is drawn as its base glyph's
//     outline, which is the monochrome form of it; its layers are not outlines
//     of their own here.
//   - A glyf entry that does not decode, or a composite that gathers more
//     points than one glyph may.
//   - A charstring that cannot be run: an operator it runs off the stack, a
//     subroutine that is not there, calls nested too deep, more operators than
//     HarfBuzz runs of one glyph. GlyphExtents has no ink for the same glyph.
//   - A VARC glyph one of whose leaves is a CFF glyph, which has no glyf points
//     to be moved and turned, and one that would cost more than a VARC glyph
//     may.
//   - The work the face may spend drawing running out: the glyphs drawn after
//     that are refused rather than drawn short.
//
// A CFF2 glyph the conversion to CFF could not write — one whose charstring
// cannot be run, or that came after the work or size allowed ran out — is
// written by it as an empty glyph, and is drawn as one here; Face.LayoutLimits
// says when that has happened, as it does for the glyph's box.
func (f *Face) GlyphOutline(gid int, yield func(Segment) bool) error {
	segs, err := f.glyphOutline(gid)
	if err != nil {
		return err
	}
	for _, s := range segs {
		if !yield(s) {
			return nil
		}
	}
	return nil
}

// glyphOutline is GlyphOutline's outline, kept once drawn.
func (f *Face) glyphOutline(gid int) ([]Segment, error) {
	if f.std != nil || f.prog == nil || f.outlines == nil {
		return nil, fmt.Errorf("%w: a standard face has no font program to read one from", ErrNoOutline)
	}
	if gid < 0 || gid >= f.prog.NumGlyphs {
		return nil, fmt.Errorf("shape: glyph %d is not one of the face's %d glyphs", gid, f.prog.NumGlyphs)
	}
	c := f.outlines
	c.mu.Lock()
	defer c.mu.Unlock()
	if segs, ok := c.done[gid]; ok {
		return segs, nil
	}
	var segs []Segment
	var err error
	switch {
	case f.varc != nil && f.varc.t.coverageIndex(gid) >= 0:
		segs, err = f.varcSegments(gid, c)
	case f.glyfOut != nil:
		segs, err = f.glyfSegments(gid, c)
	case f.ink != nil:
		segs, err = f.cffSegments(gid, c)
	case f.bitmapOnly:
		// No outlines at all: every glyph's is empty, and a bitmap glyph's is
		// said to be a bitmap below.
	default:
		err = errors.New("shape: the face has no glyf or CFF outlines")
	}
	if err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		if f.sbix != nil {
			if _, ok := f.sbix.extents(gid); ok {
				return nil, fmt.Errorf("%w: glyph %d is an sbix bitmap", ErrNoOutline, gid)
			}
		}
		if f.bitmap != nil {
			if _, ok := f.bitmap.extents(gid); ok {
				return nil, fmt.Errorf("%w: glyph %d is a CBDT bitmap", ErrNoOutline, gid)
			}
		}
	}
	if c.cached+len(segs) <= maxCachedSegments {
		c.cached += len(segs)
		c.done[gid] = segs
	}
	return segs, nil
}

// glyfSegments draws a glyf glyph, composites resolved, through the walk the
// glyph's box is measured by.
func (f *Face) glyfSegments(gid int, c *outlineCache) ([]Segment, error) {
	g := f.glyfOut
	if gid >= g.glyfNumGlyphs() || (f.prog.GlyphPresent != nil && !f.prog.GlyphPresent[gid]) {
		return nil, fmt.Errorf("shape: glyph %d is past the end of the glyf table", gid)
	}
	w := &glyfWalk{dec: decycler{tortoise: -1}, contours: true}
	if !g.glyfPoints(gid, 0, w) {
		return nil, fmt.Errorf("shape: glyph %d's glyf entry cannot be read, or gathers more than %d points", gid, maxGlyfPoints)
	}
	if !c.budget.Charge(len(w.points)+16, "the outlines of the glyphs") {
		return nil, outlineBudgetError(c)
	}
	// The whole outline is moved by where the left phantom point is, which is
	// how far the glyph's box sits from the side bearing hmtx states: the top
	// glyph's four phantom points are the last of the walk's.
	n := len(w.points) - 4
	shift := w.points[n].x
	pts := make([]Point, n)
	for i, p := range w.points[:n] {
		pts[i] = Point{float64(p.x - shift), float64(p.y)}
	}
	return quadContours(pts, w.onCurve[:n], w.ends), nil
}

// varcSegments draws a glyph through VARC, its leaves' points turned into the
// contours glyf would hold.
func (f *Face) varcSegments(gid int, c *outlineCache) ([]Segment, error) {
	v := f.varc
	if c.varcWork <= 0 {
		return nil, errOutlineWork
	}
	o, err := v.outline(gid, v.coords, &c.varcWork)
	if err != nil {
		return nil, fmt.Errorf("shape: glyph %d cannot be drawn through VARC: %w", gid, err)
	}
	pts := make([]Point, len(o.points))
	for i, p := range o.points {
		pts[i] = Point{float64(p.x), float64(p.y)}
	}
	return quadContours(pts, o.onCurve, o.ends), nil
}

// cffSegments runs a CFF glyph's charstring and takes down what it draws.
func (f *Face) cffSegments(gid int, c *outlineCache) ([]Segment, error) {
	ink := f.ink
	ink.load()
	o, run := ink.outlines, gid
	switch {
	case ink.cff2 != nil:
		// The one charstring, which calls no subroutine: a CFF2 font's are
		// run as it is written (cff2cff.go), as measuring runs them.
		if gid >= ink.numGlyphs {
			return nil, fmt.Errorf("shape: glyph %d is not one of the face's %d glyphs", gid, ink.numGlyphs)
		}
		o, run = &cffOutlines{charStrings: [][]byte{ink.cff2.glyph(gid)}}, 0
	case o == nil:
		return nil, errors.New("shape: the CFF table is one HarfBuzz's reader refuses, so none of its glyphs can be drawn")
	case gid >= len(o.charStrings):
		return nil, fmt.Errorf("shape: glyph %d has no charstring, the CFF table holding %d", gid, len(o.charStrings))
	}
	var pen outlinePen
	r := t2Run{o: o, budget: c.budget, draw: true, path: pen.add}
	// A seac looks its two glyphs up in the font's charset, which the reader
	// fills in lazily, so the outlines are not this run's alone to touch.
	ink.mu.Lock()
	_, ok := r.bounds(run, false)
	ink.mu.Unlock()
	switch {
	case r.spent:
		return nil, outlineBudgetError(c)
	case r.capped:
		return nil, fmt.Errorf("shape: glyph %d's charstring runs past the %d operators HarfBuzz runs of one", gid, cffMaxOps)
	case !ok:
		return nil, fmt.Errorf("shape: glyph %d's charstring cannot be run", gid)
	}
	return pen.segs, nil
}

// cffPointSegments is what a CFF glyph's charstring draws, for its points: the
// charstring itself, never a VARC table's composition of it, since FreeType's
// CFF loader reads none; a seac's accent before its base; and an allowance of
// its own, since kerx asks for a glyph's points once for each attachment and
// what it asks must not spend the face's drawing budget.
func (f *Face) cffPointSegments(gid int) []Segment {
	ink := f.ink
	ink.load()
	o, run := ink.outlines, gid
	switch {
	case gid < 0:
		return nil
	case ink.cff2 != nil:
		if gid >= ink.numGlyphs {
			return nil
		}
		o, run = &cffOutlines{charStrings: [][]byte{ink.cff2.glyph(gid)}}, 0
	case o == nil, gid >= len(o.charStrings):
		return nil
	}
	var pen outlinePen
	r := t2Run{o: o, budget: font.NewBudget(maxFontWork), draw: true, path: pen.add, accentFirst: true}
	ink.mu.Lock()
	_, ok := r.bounds(run, false)
	ink.mu.Unlock()
	if !ok || r.spent || r.capped {
		return nil
	}
	return pen.segs
}

var errOutlineWork = errors.New("shape: drawing the face's glyphs has run past the work one face may spend on it")

func outlineBudgetError(c *outlineCache) error {
	if err := c.budget.Err(); err != nil {
		return fmt.Errorf("shape: %w", err)
	}
	return errOutlineWork
}

// outlinePen takes down a charstring's path: a move is held until something is
// drawn from it, so that a move nothing follows is not a contour.
type outlinePen struct {
	segs    []Segment
	pending Segment
}

func (p *outlinePen) add(s t2Seg) {
	switch s.op {
	case 'M':
		p.pending = Segment{Op: MoveTo, Pts: [3]Point{{s.pts[0], s.pts[1]}}}
	case 'L':
		p.flush()
		p.segs = append(p.segs, Segment{Op: LineTo, Pts: [3]Point{{s.pts[0], s.pts[1]}}})
	case 'C':
		p.flush()
		p.segs = append(p.segs, Segment{Op: CubicTo, Pts: [3]Point{
			{s.pts[0], s.pts[1]}, {s.pts[2], s.pts[3]}, {s.pts[4], s.pts[5]}}})
	}
}

func (p *outlinePen) flush() {
	if p.pending.Op != 0 {
		p.segs = append(p.segs, p.pending)
		p.pending = Segment{}
	}
}

// quadContours turns TrueType contours — points, which of them are on the
// curve, and the index of the last point of each contour — into segments.
//
// A run of off-curve points is a run of quadratic curves whose on-curve points
// are the midpoints between them. A contour that starts off the curve begins at
// its last point where that is on the curve, and at the midpoint of its first
// and last where that is off too. A contour of one point draws nothing.
func quadContours(pts []Point, on []bool, ends []int) []Segment {
	var out []Segment
	first := 0
	for _, e := range ends {
		if e >= len(pts) || e < first {
			break
		}
		out = appendContour(out, pts[first:e+1], on[first:e+1])
		first = e + 1
	}
	return out
}

func appendContour(out []Segment, p []Point, on []bool) []Segment {
	n := len(p)
	if n < 2 {
		return out
	}
	mid := func(a, b Point) Point { return Point{(a.X + b.X) / 2, (a.Y + b.Y) / 2} }
	// start is where the contour begins; the points from from up to to are
	// what is still to be drawn.
	var start Point
	from, to := 0, n
	switch {
	case on[0]:
		start, from = p[0], 1
	case on[n-1]:
		start, to = p[n-1], n-1
	default:
		start = mid(p[0], p[n-1])
	}
	out = append(out, Segment{Op: MoveTo, Pts: [3]Point{start}})
	var ctrl Point
	pending := false
	for i := from; i < to; i++ {
		switch {
		case on[i] && pending:
			out = append(out, Segment{Op: QuadTo, Pts: [3]Point{ctrl, p[i]}})
			pending = false
		case on[i]:
			out = append(out, Segment{Op: LineTo, Pts: [3]Point{p[i]}})
		case pending:
			out = append(out, Segment{Op: QuadTo, Pts: [3]Point{ctrl, mid(ctrl, p[i])}})
			ctrl = p[i]
		default:
			ctrl, pending = p[i], true
		}
	}
	if pending {
		out = append(out, Segment{Op: QuadTo, Pts: [3]Point{ctrl, start}})
	}
	return out
}
