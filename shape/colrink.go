package shape

import (
	"math"
	"sync"

	"github.com/mgilbir/forme/font"
)

// The ink of a colour glyph: the box a COLR glyph paints, which is what
// HarfBuzz answers for its extents before it looks at the glyph's outline.
//
// A face with a COLR table draws some of its glyphs as layers of other glyphs,
// each filled with a colour or a gradient and moved, scaled, rotated or
// composited on the way. HarfBuzz's extents for such a glyph are not its glyf
// box — which is usually the box of a placeholder outline, or of nothing — but
// the box its painting covers: the glyph's clip box where the table states one,
// and otherwise the union of every glyph outline a fill is clipped to, carried
// through every transform above it. It asks that before the glyf or CFF
// outline, and everything that reads a glyph's ink reads it: the marks placed
// for a face that positions none of its own (fallback.go), the ink centred in
// a line set upright (vertical.go), how far a run reaches (Face.InkExtent).
//
// Honk is the face that showed it. It positions none of its marks and draws
// every glyph in colour, and its accents were placed against the placeholder
// outlines' boxes where HarfBuzz placed them against the painted ones: forty
// strings of the Google Fonts sweep, every one of them a mark set in the wrong
// place.
//
// What is measured is hb_ot_get_glyph_extents's COLR answer at the release the
// oracle is pinned to, number for number, because every reader of it is held to
// HarfBuzz:
//
//   - A glyph the ClipList covers answers its clip box, varied where the box is
//     variable.
//   - Otherwise a COLRv1 glyph is painted with HarfBuzz's extents functions,
//     in floats as HarfBuzz paints it: each glyph a fill is clipped to is drawn
//     and its points boxed, the box put through the transform in force, and
//     intersected with the clips around it; each fill adds the clip it is
//     inside to its group, and a composite combines its two groups as its mode
//     says. The answer is the box, rounded. A glyph whose painting HarfBuzz
//     finds unbounded — a fill with no clip around it — is painted not at all,
//     and answers nothing but zero.
//   - A COLRv0 glyph is the union of its layers' outlines.
//   - A glyph the table does not paint has no colour ink, and its outline is
//     asked instead.
//
// The limits are HarfBuzz's: paint nested 64 deep and 16,384 paint edges a
// glyph, cycles through the layer list or through colour glyphs found by its
// decycler. Each glyph is painted once and its answer kept, and the work is
// charged to a budget of the face's own, as a CFF glyph's is (cffink.go); one
// that runs out leaves the glyphs after it painted with what was read, and says
// so through Face.LayoutLimits.
//
// # What is not here
//
// The two bitmap tables HarfBuzz asks before COLR are read elsewhere: sbix in
// sbixink.go and CBDT in bitmapink.go; and VARC, asked after it, in varc.go.
// And the table
// is read as far as it is sound rather than refused whole where HarfBuzz's
// sanitizer would refuse it: a malformed COLR table answers from what can be
// read of it where HarfBuzz answers from the outlines.

// colrInk is what a face with a COLR table keeps to measure its colour glyphs:
// the table and the outlines its layers are drawn from, and each glyph's answer
// once it has one. It is shared by the face's clones, as cffInk is.
type colrInk struct {
	f         *Face
	colr      []byte
	cpal      []byte
	glyf      []byte
	loca      []byte
	longLoca  bool
	numGlyphs int

	once sync.Once
	t    *colrTable

	mu       sync.Mutex
	answers  map[int]colrAnswer
	outlines map[int]box32
	budget   *font.Budget
	spent    bool
	// verdicts are what counting a glyph's painting found, by the glyph and
	// the budget it was counted within, so that PaintGlyph counts a glyph
	// once and not each time it is painted. See paintCOLR.
	verdicts map[paintKey]paintVerdict
}

// colrAnswer is one glyph's answer: its extents, and whether the table paints
// it at all.
type colrAnswer struct {
	ext     extents
	painted bool
}

func newCOLRInk(f *Face, tables map[string][]byte, numGlyphs int) *colrInk {
	c := &colrInk{f: f, colr: tables["COLR"], cpal: tables["CPAL"], numGlyphs: numGlyphs}
	head := tables["head"]
	if glyf, loca := tables["glyf"], tables["loca"]; len(glyf) > 0 && len(loca) > 0 && len(head) >= 54 {
		c.glyf, c.loca = glyf, loca
		c.longLoca = signed16(font.Be16(head, 50)) != 0
	}
	return c
}

// colrInkWork is the budget every colour glyph of a face shares: a unit for
// each paint visited and each outline point drawn, sixty-four a byte of the
// COLR and glyf tables, with a floor of maxFontWork.
//
// Painting is cheap a step and a colour font repeats itself: Bitcount draws
// every letter as dots, each dot a colour glyph painted again, so painting all
// 1,991 glyphs of Bitcount Prop Single Ink visits 4.8 million paints for 0.3
// MB of tables — sixteen a byte, the most of any COLR face in the Google Fonts
// tree. The budget is four times that for every face, which a document using
// every glyph of any of them does not reach; what does is a table built to
// have each glyph reach HarfBuzz's 16,384 edges.
func colrInkWork(c *colrInk) int {
	return max(maxFontWork, 64*(len(c.colr)+len(c.glyf)))
}

// extents is a glyph's colour ink, and whether the table paints the glyph; a
// glyph it does not paint is measured by its outline instead.
func (c *colrInk) extents(gid int) (extents, bool) {
	if c.table() == nil {
		return extents{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.answers[gid]; ok {
		return a.ext, a.painted
	}
	ext, painted := c.measure(gid)
	if c.answers == nil {
		c.answers = map[int]colrAnswer{}
	}
	c.answers[gid] = colrAnswer{ext, painted}
	return ext, painted
}

// table is the COLR table, read the first time it is asked for, and nil for
// one HarfBuzz reads as having nothing in it.
func (c *colrInk) table() *colrTable {
	c.once.Do(func() {
		c.t = readCOLR(c.colr, c.f.varCoords)
		c.budget = font.NewBudget(colrInkWork(c))
	})
	return c.t
}

// charge spends budget on painting, and reports whether there was any.
func (c *colrInk) charge(n int) bool {
	if c.budget.Charge(n, "the ink of the colour glyphs") {
		return true
	}
	c.spent = true
	return false
}

// limits are the bounds painting has run into so far. See Face.LayoutLimits.
func (c *colrInk) limits() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.spent {
		return []string{"measuring the ink of its colour glyphs ran past the work one face " +
			"may cost, so the glyphs measured after that are measured by what was painted of them"}
	}
	return nil
}

// measure is COLR::get_extents.
func (c *colrInk) measure(gid int) (extents, bool) {
	t := c.t
	if clip, ok := t.clipBox(gid); ok {
		return clip.extents(), true
	}
	e := &extentsPainter{outline: c.outline}
	e.clear()
	painted := c.paintGlyph(gid, e, true)
	box := e.groups[len(e.groups)-1].ext
	if box.void() {
		return extents{}, painted
	}
	return box.glyphExtents(), painted
}

// paintGlyph is COLR::paint_glyph: gid painted through funcs, in a paint
// context of its own. clip says whether the glyph's clip box is to be used, as
// HarfBuzz uses it, and a glyph with none is first painted with the bounded
// functions to learn whether painting it can be bounded at all.
func (c *colrInk) paintGlyph(gid int, funcs painter, clip bool) bool {
	painted, _ := c.paintGlyphWith(gid, funcs, clip, nil)
	return painted
}

// paintGlyphWith is paintGlyph charged to own, a budget of the caller's, where
// it is not nil, rather than to the face's: what Face.PaintGlyph paints with.
// refused says whether painting stopped short at a bound: the nesting, the
// edges, or the budget.
func (c *colrInk) paintGlyphWith(gid int, funcs painter, clip bool, own *font.Budget) (painted, refused bool) {
	painted, refused, _ = c.paintGlyphKnowing(gid, funcs, clip, own, boundsUnknown)
	return painted, refused
}

// boundedness is whether painting a COLRv1 glyph that has no clip box can be
// bounded, which painting it with the bounded functions finds: unknown until
// it has.
type boundedness uint8

const (
	boundsUnknown boundedness = iota
	boundsBounded
	boundsUnbounded
)

// paintGlyphKnowing is paintGlyphWith told whether painting the glyph can be
// bounded, where that is known, rather than painting it with the bounded
// functions to learn. It reports what it was told or learned, which is
// boundsUnknown for a glyph that has a clip box or is not painted with one.
func (c *colrInk) paintGlyphKnowing(gid int, funcs painter, clip bool, own *font.Budget, known boundedness) (painted, refused bool, bounds boundedness) {
	t := c.t
	p := &paintContext{c: c, funcs: funcs, depthLeft: maxPaintDepth, edges: maxPaintEdges, own: own}
	p.glyphs.enter()
	defer p.glyphs.leave()
	p.glyphs.visit(gid)
	if t.version >= 1 {
		if paint, ok := t.basePaint(gid); ok {
			bounded := true
			var box clipRect
			if clip {
				if box, clip = t.clipBox(gid); !clip {
					if known == boundsUnknown {
						b := &boundedPainter{bounded: true}
						_, inner := c.paintGlyphWith(gid, b, false, own)
						p.refused = p.refused || inner
						known = boundsUnbounded
						if b.bounded {
							known = boundsBounded
						}
					}
					bounded = known == boundsBounded
				}
			}
			funcs.pushTransform(identity32)
			if clip {
				funcs.pushClipRect(box.rect())
			}
			if bounded {
				p.recurse(paint)
			}
			if clip {
				funcs.popClip()
			}
			funcs.popTransform()
			return true, p.refused, known
		}
	}
	if first, n, ok := t.baseGlyphRecord(gid); ok {
		for i := first; i < first+n && i < t.numLayers; i++ {
			if !p.charge(1) {
				p.refused = true
				break
			}
			layer := t.layers + 4*i
			funcs.pushClipGlyph(t.u16(layer))
			funcs.paint(paintFill{at: -1, index: t.u16(layer + 2)})
			funcs.popClip()
		}
		return true, p.refused, known
	}
	return false, false, known
}

// HarfBuzz's bounds on one glyph's painting: HB_MAX_NESTING_LEVEL and
// HB_MAX_GRAPH_EDGE_COUNT.
const (
	maxPaintDepth = 64
	maxPaintEdges = 16384
)

// paintContext is hb_paint_context_t: the functions painted through, the two
// decyclers, and what is left of the nesting and the edges.
type paintContext struct {
	c         *colrInk
	funcs     painter
	glyphs    decycler
	layers    decycler
	depthLeft int
	edges     int
	// own is the budget painting is charged to, where it is not the face's;
	// refused records that a bound stopped it. See paintGlyphWith.
	own     *font.Budget
	refused bool
}

// charge spends n of the budget painting is charged to.
func (p *paintContext) charge(n int) bool {
	if p.own != nil {
		return p.own.Charge(n, "painting a colour glyph")
	}
	return p.c.charge(n)
}

func (p *paintContext) recurse(paint int) {
	if p.depthLeft <= 0 || p.edges <= 0 || !p.charge(1) {
		p.refused = true
		return
	}
	p.depthLeft--
	p.edges--
	p.dispatch(paint)
	p.depthLeft++
}

// The composite modes that decide how a PaintComposite's groups combine, in
// the table's numbering, which is HarfBuzz's.
const (
	compositeClear   = 0
	compositeSrc     = 1
	compositeDest    = 2
	compositeSrcOver = 3
	compositeSrcIn   = 5
	compositeDestIn  = 6
	compositeSrcOut  = 7
	compositeDestOut = 8
	compositeLast    = 27
)

// hbPi is HB_PI, the float the angles are scaled by.
const hbPi = float32(math.Pi)

// dispatch paints one Paint table, at an offset in COLR: each format's
// paint_glyph.
func (p *paintContext) dispatch(at int) {
	t, f := p.c.t, p.funcs
	sub := func(off int) int { return t.offset24(at, off) }
	f2 := func(off, v int) float32 { return t.f2dot14(at+off, t.delta(at, v)) }
	fword := func(off, v int) float32 { return float32(float32(signed16(t.u16(at+off))) + t.delta(at, v)) }
	switch t.u8(at) {
	case 1: // PaintColrLayers
		n, first := t.u8(at+1), int(t.u32(at+2))
		p.layers.enter()
		defer p.layers.leave()
		for i := first; i < first+n; i++ {
			if !p.layers.visit(i) {
				return
			}
			p.recurse(t.layerPaint(i))
		}
	case 2, 3, 4, 5, 6, 7, 8, 9: // the solid fill and the gradients
		f.paint(paintFill{at: at})
	case 10: // PaintGlyph
		child, gid := sub(1), t.u16(at+4)
		if p.depthLeft > 0 && p.edges > 0 && (t.u8(child) == 2 || t.u8(child) == 3) {
			// A glyph filled with one colour, which HarfBuzz paints as one
			// fill_glyph: the same clip and fill, one edge.
			p.edges--
			f.pushTransform(identity32)
			f.pushClipGlyph(gid)
			f.paint(paintFill{at: child})
			f.popClip()
			f.popTransform()
			return
		}
		f.pushTransform(identity32)
		f.pushClipGlyph(gid)
		f.pushTransform(identity32)
		p.recurse(child)
		f.popTransform()
		f.popClip()
		f.popTransform()
	case 11: // PaintColrGlyph
		gid := t.u16(at + 1)
		p.glyphs.enter()
		defer p.glyphs.leave()
		if !p.glyphs.visit(gid) {
			return
		}
		paint, painted := t.basePaint(gid)
		box, clipped := t.clipBox(gid)
		if clipped {
			f.pushClipRect(box.rect())
		}
		if painted {
			p.recurse(paint)
		}
		if clipped {
			f.popClip()
		}
	case 12, 13: // PaintTransform, PaintVarTransform
		// A null Affine2x3 is HarfBuzz's Null one, all zeroes.
		a := sub(4)
		fixed := func(i int) float32 {
			if a < 0 {
				return 0
			}
			return float32(float32(float32(int32(t.u32(a+4*i)))+t.varDelta(t.varIdxAt(at, a+24), i)) * float32(1.0/65536))
		}
		f.pushTransform(xform32{fixed(0), fixed(1), fixed(2), fixed(3), fixed(4), fixed(5)})
		p.recurse(sub(1))
		f.popTransform()
	case 14, 15: // PaintTranslate
		f.pushTransform(xform32{1, 0, 0, 1, fword(4, 0), fword(6, 1)})
		p.recurse(sub(1))
		f.popTransform()
	case 16, 17: // PaintScale
		f.pushTransform(xform32{f2(4, 0), 0, 0, f2(6, 1), 0, 0})
		p.recurse(sub(1))
		f.popTransform()
	case 18, 19: // PaintScaleAroundCenter
		f.pushTransform(scalingAroundCenter(f2(4, 0), f2(6, 1), fword(8, 2), fword(10, 3)))
		p.recurse(sub(1))
		f.popTransform()
	case 20, 21: // PaintScaleUniform
		s := f2(4, 0)
		f.pushTransform(xform32{s, 0, 0, s, 0, 0})
		p.recurse(sub(1))
		f.popTransform()
	case 22, 23: // PaintScaleUniformAroundCenter
		s := f2(4, 0)
		f.pushTransform(scalingAroundCenter(s, s, fword(6, 1), fword(8, 2)))
		p.recurse(sub(1))
		f.popTransform()
	case 24, 25: // PaintRotate
		f.pushTransform(rotationAroundCenter(float32(f2(4, 0)*hbPi), 0, 0, false))
		p.recurse(sub(1))
		f.popTransform()
	case 26, 27: // PaintRotateAroundCenter
		f.pushTransform(rotationAroundCenter(float32(f2(4, 0)*hbPi), fword(6, 1), fword(8, 2), true))
		p.recurse(sub(1))
		f.popTransform()
	case 28, 29: // PaintSkew
		f.pushTransform(skewingAroundCenter(float32(-f2(4, 0)*hbPi), float32(f2(6, 1)*hbPi), 0, 0))
		p.recurse(sub(1))
		f.popTransform()
	case 30, 31: // PaintSkewAroundCenter
		f.pushTransform(skewingAroundCenter(float32(-f2(4, 0)*hbPi), float32(f2(6, 1)*hbPi), fword(8, 2), fword(10, 3)))
		p.recurse(sub(1))
		f.popTransform()
	case 32: // PaintComposite
		mode := t.u8(at + 4)
		if mode > compositeLast {
			mode = compositeClear
		}
		f.pushGroup()
		p.recurse(sub(5))
		f.pushGroup()
		p.recurse(sub(1))
		f.popGroup(mode)
		f.popGroup(compositeSrcOver)
	}
}

// scalingAroundCenter is hb_transform_t::scaling_around_center.
func scalingAroundCenter(sx, sy, cx, cy float32) xform32 {
	t := xform32{xx: sx, yy: sy}
	if cx != 0 {
		t.x0 = float32((1 - sx) * cx)
	}
	if cy != 0 {
		t.y0 = float32((1 - sy) * cy)
	}
	return t
}

// rotationAroundCenter is hb_transform_t::rotation_around_center, or with
// centred false its rotation, which is the same matrix with no offset.
func rotationAroundCenter(radians, cx, cy float32, centred bool) xform32 {
	s, c := float32(math.Sin(float64(radians))), float32(math.Cos(float64(radians)))
	t := xform32{xx: c, yx: s, xy: -s, yy: c}
	if centred {
		t.x0 = float32(float32((1-c)*cx) + float32(s*cy))
		t.y0 = float32(float32(-s*cx) + float32((1-c)*cy))
	}
	return t
}

// skewingAroundCenter is hb_transform_t::skewing_around_center; with no centre
// it is its skewing.
func skewingAroundCenter(skewX, skewY, cx, cy float32) xform32 {
	if skewX != 0 {
		skewX = float32(math.Tan(float64(skewX)))
	}
	if skewY != 0 {
		skewY = float32(math.Tan(float64(skewY)))
	}
	t := xform32{xx: 1, yx: skewY, xy: skewX, yy: 1}
	if cy != 0 {
		t.x0 = float32(-skewX * cy)
	}
	if cx != 0 {
		t.y0 = float32(-skewY * cx)
	}
	return t
}

// painter is the part of hb_paint_funcs_t the painters here implement:
// HarfBuzz's extents functions and its bounded functions, and the two
// Face.PaintGlyph paints through, one counting and one handing each call out
// (paint.go).
type painter interface {
	pushTransform(t xform32)
	popTransform()
	pushClipGlyph(gid int)
	pushClipRect(r box32)
	popClip()
	pushGroup()
	popGroup(mode int)
	paint(fill paintFill)
}

// paintFill is what a fill paints with, for a painter that hands it out: the
// Paint table of a solid fill or a gradient, at its offset in COLR, or, where
// at is negative, a COLRv0 layer's colour, a palette index at full alpha. The
// painters that measure read nothing of it.
type paintFill struct {
	at, index int
}

// box32 is hb_extents_t<float>: a box in floats, void when xMin is past xMax,
// which is what it starts as.
type box32 struct{ xMin, yMin, xMax, yMax float32 }

var void32 = box32{0, 0, -1, -1}

func (b box32) empty() bool { return b.xMin >= b.xMax || b.yMin >= b.yMax }
func (b box32) void() bool  { return b.xMin > b.xMax }

func (b *box32) add(x, y float32) {
	if b.void() {
		*b = box32{x, y, x, y}
		return
	}
	b.xMin, b.yMin = min(b.xMin, x), min(b.yMin, y)
	b.xMax, b.yMax = max(b.xMax, x), max(b.yMax, y)
}

func (b *box32) union(o box32) {
	if o.empty() {
		return
	}
	if b.empty() {
		*b = o
		return
	}
	b.xMin, b.yMin = min(b.xMin, o.xMin), min(b.yMin, o.yMin)
	b.xMax, b.yMax = max(b.xMax, o.xMax), max(b.yMax, o.yMax)
}

func (b *box32) intersect(o box32) {
	if o.empty() || b.empty() {
		*b = void32
		return
	}
	b.xMin, b.yMin = max(b.xMin, o.xMin), max(b.yMin, o.yMin)
	b.xMax, b.yMax = min(b.xMax, o.xMax), min(b.yMax, o.yMax)
}

// glyphExtents is hb_extents_t::to_glyph_extents: each edge rounded half away
// from zero, in doubles.
func (b box32) glyphExtents() extents {
	x0, y0 := math.Round(float64(b.xMin)), math.Round(float64(b.yMin))
	x1, y1 := math.Round(float64(b.xMax)), math.Round(float64(b.yMax))
	for _, v := range [...]float64{x0, y0, x1, y1} {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return extents{}
		}
	}
	return extents{
		xBearing: int(clampToInt32(x0)),
		yBearing: int(clampToInt32(y1)),
		width:    int(clampToInt32(x1 - x0)),
		height:   int(clampToInt32(y0 - y1)),
	}
}

// The three states hb_bounds_t has.
const (
	unbounded = iota
	bounded
	emptyBounds
)

// bounds32 is hb_bounds_t<float>.
type bounds32 struct {
	status int
	ext    box32
}

func boundsOf(b box32) bounds32 {
	if b.empty() {
		return bounds32{emptyBounds, b}
	}
	return bounds32{bounded, b}
}

func (b *bounds32) union(o bounds32) {
	switch o.status {
	case unbounded:
		b.status = unbounded
	case bounded:
		switch b.status {
		case emptyBounds:
			*b = o
		case bounded:
			b.ext.union(o.ext)
		}
	}
}

func (b *bounds32) intersect(o bounds32) {
	switch o.status {
	case emptyBounds:
		b.status = emptyBounds
	case bounded:
		switch b.status {
		case unbounded:
			*b = o
		case bounded:
			b.ext.intersect(o.ext)
			if b.ext.empty() {
				b.status = emptyBounds
			}
		}
	}
}

// xform32 is hb_transform_t<float>.
type xform32 struct{ xx, yx, xy, yy, x0, y0 float32 }

var identity32 = xform32{xx: 1, yy: 1}

// then is hb_transform_t::multiply: this transform followed, inside it, by o.
// Each product is rounded to a float before it is added, as HarfBuzz's are.
func (a xform32) then(b xform32) xform32 {
	return xform32{
		float32(a.xx*b.xx) + float32(a.xy*b.yx),
		float32(a.yx*b.xx) + float32(a.yy*b.yx),
		float32(a.xx*b.xy) + float32(a.xy*b.yy),
		float32(a.yx*b.xy) + float32(a.yy*b.yy),
		float32(float32(a.xx*b.x0)+float32(a.xy*b.y0)) + a.x0,
		float32(float32(a.yx*b.x0)+float32(a.yy*b.y0)) + a.y0,
	}
}

func (t xform32) point(x, y float32) (float32, float32) {
	return float32(t.x0+float32(t.xx*x)) + float32(t.xy*y), float32(t.y0+float32(t.yx*x)) + float32(t.yy*y)
}

// box is hb_transform_t::transform_extents: the four corners through the
// transform, boxed — a void box's corners too, which makes a small box of it.
func (t xform32) box(b box32) box32 {
	out := void32
	for _, c := range [4][2]float32{{b.xMin, b.yMin}, {b.xMin, b.yMax}, {b.xMax, b.yMin}, {b.xMax, b.yMax}} {
		out.add(t.point(c[0], c[1]))
	}
	return out
}

// extentsPainter is HarfBuzz's hb_paint_extents_context_t and its functions.
type extentsPainter struct {
	outline    func(gid int) box32
	transforms []xform32
	clips      []bounds32
	groups     []bounds32
}

func (e *extentsPainter) clear() {
	e.transforms = []xform32{identity32}
	e.clips = []bounds32{{status: unbounded, ext: void32}}
	e.groups = []bounds32{{status: emptyBounds, ext: void32}}
}

func (e *extentsPainter) pushTransform(t xform32) {
	if t.x0 == 0 {
		t.x0 = 0 // -0 is 0, as HarfBuzz makes it
	}
	if t.y0 == 0 {
		t.y0 = 0
	}
	e.transforms = append(e.transforms, e.transforms[len(e.transforms)-1].then(t))
}

func (e *extentsPainter) popTransform() {
	if len(e.transforms) > 1 {
		e.transforms = e.transforms[:len(e.transforms)-1]
	}
}

func (e *extentsPainter) pushClip(b box32) {
	c := boundsOf(e.transforms[len(e.transforms)-1].box(b))
	c.intersect(e.clips[len(e.clips)-1])
	e.clips = append(e.clips, c)
}

func (e *extentsPainter) pushClipGlyph(gid int) { e.pushClip(e.outline(gid)) }
func (e *extentsPainter) pushClipRect(r box32)  { e.pushClip(r) }

func (e *extentsPainter) popClip() {
	if len(e.clips) > 1 {
		e.clips = e.clips[:len(e.clips)-1]
	}
}

func (e *extentsPainter) pushGroup() { e.groups = append(e.groups, bounds32{emptyBounds, void32}) }

func (e *extentsPainter) popGroup(mode int) {
	if len(e.groups) < 2 {
		return
	}
	src := e.groups[len(e.groups)-1]
	e.groups = e.groups[:len(e.groups)-1]
	backdrop := &e.groups[len(e.groups)-1]
	switch mode {
	case compositeClear:
		backdrop.status = emptyBounds
	case compositeSrc, compositeSrcOut:
		*backdrop = src
	case compositeDest, compositeDestOut:
	case compositeSrcIn, compositeDestIn:
		backdrop.intersect(src)
	default:
		backdrop.union(src)
	}
}

func (e *extentsPainter) paint(paintFill) { e.groups[len(e.groups)-1].union(e.clips[len(e.clips)-1]) }

// boundedPainter is HarfBuzz's hb_paint_bounded_context_t: whether anything
// is filled with no clip around it.
type boundedPainter struct {
	bounded bool
	clips   int
	groups  []bool
}

func (b *boundedPainter) pushTransform(xform32) {}
func (b *boundedPainter) popTransform()         {}
func (b *boundedPainter) pushClipGlyph(int)     { b.clips++ }
func (b *boundedPainter) pushClipRect(box32)    { b.clips++ }

func (b *boundedPainter) popClip() {
	if b.clips > 0 {
		b.clips--
	}
}

func (b *boundedPainter) pushGroup() {
	b.groups = append(b.groups, b.bounded)
	b.bounded = true
}

func (b *boundedPainter) popGroup(mode int) {
	if len(b.groups) == 0 {
		return
	}
	src := b.bounded
	b.bounded = b.groups[len(b.groups)-1]
	b.groups = b.groups[:len(b.groups)-1]
	switch mode {
	case compositeClear:
		b.bounded = true
	case compositeSrc, compositeSrcOut:
		b.bounded = src
	case compositeDest, compositeDestOut:
	case compositeSrcIn, compositeDestIn:
		b.bounded = b.bounded && src
	default:
		b.bounded = b.bounded || src
	}
}

func (b *boundedPainter) paint(paintFill) {
	if b.clips == 0 {
		b.bounded = false
	}
}

// colrTable is a COLR table read for painting.
type colrTable struct {
	b       []byte
	version int
	// The version 0 records: base glyphs and layers.
	baseGlyphs, numBaseGlyphs int
	layers, numLayers         int
	// The version 1 lists, as offsets into the table, zero where there is
	// none.
	baseGlyphList, layerList, clipList int
	// The variations, and where in the design space the face was cut.
	varIdxMap *deltaSetIndexMap
	varStore  *varStore
	coords    []float64
}

// readCOLR reads a COLR table's header, and nil for one HarfBuzz reads as
// having nothing in it.
func readCOLR(b []byte, coords []float64) *colrTable {
	if len(b) < 14 {
		return nil
	}
	t := &colrTable{
		b: b, version: font.Be16(b, 0),
		numBaseGlyphs: font.Be16(b, 2), baseGlyphs: int(font.Be32(b, 4)),
		layers: int(font.Be32(b, 8)), numLayers: font.Be16(b, 12),
		coords: coords,
	}
	if t.version >= 1 && len(b) >= 34 {
		t.baseGlyphList = int(font.Be32(b, 14))
		t.layerList = int(font.Be32(b, 18))
		t.clipList = int(font.Be32(b, 22))
		if off := int(font.Be32(b, 26)); off > 0 && off < len(b) {
			t.varIdxMap, _ = parseDeltaSetIndexMap(b[off:])
		}
		if off := int(font.Be32(b, 30)); off > 0 && off < len(b) {
			t.varStore, _ = parseVarStore(b[off:])
		}
	}
	if t.numBaseGlyphs == 0 && t.version == 0 {
		return nil
	}
	return t
}

func (t *colrTable) u8(at int) int {
	if at < 0 || at >= len(t.b) {
		return 0
	}
	return int(t.b[at])
}

func (t *colrTable) u16(at int) int {
	if at < 0 {
		return 0
	}
	return font.Be16(t.b, at)
}

func (t *colrTable) u32(at int) uint32 {
	if at < 0 {
		return 0
	}
	return font.Be32(t.b, at)
}

// offset24 is the table an Offset24 at at+off names, relative to at, and -1
// for a null one, which is HarfBuzz's Null Paint: a format that paints nothing.
func (t *colrTable) offset24(at, off int) int {
	o := t.u8(at+off)<<16 | t.u16(at+off+1)
	if o == 0 || at+off+3 > len(t.b) {
		return -1
	}
	return at + o
}

// f2dot14 is F2DOT14::to_float with a variation's delta.
func (t *colrTable) f2dot14(at int, delta float32) float32 {
	return float32(float32(float32(signed16(t.u16(at)))+delta) * float32(1.0/16384))
}

// delta is the variation delta of a paint's v-th varied value: its format
// says whether it is varied, and where its VarIdxBase is.
func (t *colrTable) delta(at, v int) float32 {
	var base int
	switch t.u8(at) {
	case 3: // PaintVarSolid
		base = at + 5
	case 5, 7: // PaintVarLinearGradient, PaintVarRadialGradient
		base = at + 16
	case 9: // PaintVarSweepGradient
		base = at + 12
	case 15:
		base = at + 8
	case 17:
		base = at + 8
	case 19:
		base = at + 12
	case 21:
		base = at + 6
	case 23:
		base = at + 10
	case 25:
		base = at + 6
	case 27:
		base = at + 10
	case 29:
		base = at + 8
	case 31:
		base = at + 12
	default:
		return 0
	}
	return t.varDelta(t.u32(base), v)
}

// varIdxAt is a PaintVarTransform's VarIdxBase, after its Affine2x3, and none
// for a PaintTransform.
func (t *colrTable) varIdxAt(paint, at int) uint32 {
	if t.u8(paint) != 13 {
		return noVariation
	}
	return t.u32(at)
}

// noVariation is VarIdx::NO_VARIATION.
const noVariation = 0xFFFFFFFF

// varDelta is ItemVarStoreInstancer's answer for a VarIdxBase and an offset
// from it: nothing at the default instance or for a value that does not vary.
func (t *colrTable) varDelta(base uint32, v int) float32 {
	if len(t.coords) == 0 || base == noVariation || t.varStore == nil {
		return 0
	}
	idx := int(base) + v
	outer, inner := idx>>16, idx&0xFFFF
	if t.varIdxMap != nil && len(t.varIdxMap.entries) > 0 {
		outer, inner = t.varIdxMap.lookup(idx)
	}
	return float32(t.varStore.delta(outer, inner, t.coords))
}

// basePaint is the Paint a COLRv1 base glyph record names for a glyph.
func (t *colrTable) basePaint(gid int) (int, bool) {
	list := t.baseGlyphList
	if list <= 0 || list+4 > len(t.b) {
		return 0, false
	}
	n := int(t.u32(list))
	lo, hi := 0, min(n, (len(t.b)-list-4)/6)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		rec := list + 4 + 6*mid
		switch g := t.u16(rec); {
		case gid < g:
			hi = mid - 1
		case gid > g:
			lo = mid + 1
		default:
			off := int(t.u32(rec + 2))
			if off == 0 {
				return -1, true
			}
			return list + off, true
		}
	}
	return 0, false
}

// layerPaint is the i-th Paint of the LayerList, and a null one past its end.
func (t *colrTable) layerPaint(i int) int {
	list := t.layerList
	if list <= 0 || i < 0 || i >= int(t.u32(list)) {
		return -1
	}
	off := int(t.u32(list + 4 + 4*i))
	if off == 0 {
		return -1
	}
	return list + off
}

// baseGlyphRecord is a COLRv0 glyph's layers: the first, and how many.
func (t *colrTable) baseGlyphRecord(gid int) (first, n int, ok bool) {
	if t.numBaseGlyphs == 0 {
		return 0, 0, false
	}
	lo, hi := 0, min(t.numBaseGlyphs, (len(t.b)-t.baseGlyphs)/6)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		rec := t.baseGlyphs + 6*mid
		switch g := t.u16(rec); {
		case gid < g:
			hi = mid - 1
		case gid > g:
			lo = mid + 1
		default:
			return t.u16(rec + 2), t.u16(rec + 4), true
		}
	}
	return 0, 0, false
}

// clipRect is a ClipBox, varied.
type clipRect struct{ xMin, yMin, xMax, yMax int }

// clipBox is the ClipList's box for a glyph.
func (t *colrTable) clipBox(gid int) (clipRect, bool) {
	list := t.clipList
	if list <= 0 || list+5 > len(t.b) {
		return clipRect{}, false
	}
	n := int(t.u32(list + 1))
	lo, hi := 0, min(n, (len(t.b)-list-5)/7)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		rec := list + 5 + 7*mid
		switch {
		case gid < t.u16(rec):
			hi = mid - 1
		case gid > t.u16(rec+2):
			lo = mid + 1
		default:
			// The box is named from the ClipList's start, and a null one is
			// HarfBuzz's Null box, of no format, which has no extents.
			at := t.offset24(rec, 4)
			if at < 0 {
				return clipRect{}, false
			}
			at = list + (at - rec)
			var r clipRect
			switch t.u8(at) {
			case 1, 2:
				r = clipRect{signed16(t.u16(at + 1)), signed16(t.u16(at + 3)),
					signed16(t.u16(at + 5)), signed16(t.u16(at + 7))}
			default:
				return clipRect{}, false
			}
			if t.u8(at) == 2 && len(t.coords) > 0 {
				base := t.u32(at + 9)
				r.xMin = saturatingAdd(r.xMin, int(clampToInt32(hbRound(float64(t.varDelta(base, 0))))))
				r.yMin = saturatingAdd(r.yMin, int(clampToInt32(hbRound(float64(t.varDelta(base, 1))))))
				r.xMax = saturatingAdd(r.xMax, int(clampToInt32(hbRound(float64(t.varDelta(base, 2))))))
				r.yMax = saturatingAdd(r.yMax, int(clampToInt32(hbRound(float64(t.varDelta(base, 3))))))
			}
			return r, true
		}
	}
	return clipRect{}, false
}

func saturatingAdd(a, b int) int { return int(clampToInt32(float64(a) + float64(b))) }

// extents is the clip box as the glyph's extents: ClipBox::get_extents, and
// the scaling HarfBuzz puts it through, which at one unit to the unit leaves
// it as it is.
func (r clipRect) extents() extents {
	return extents{
		xBearing: r.xMin, yBearing: r.yMax,
		width:  int(clampToInt32(float64(r.xMax) - float64(r.xMin))),
		height: int(clampToInt32(float64(r.yMin) - float64(r.yMax))),
	}
}

// rect is the clip box as the rectangle painting pushes, from its extents.
func (r clipRect) rect() box32 {
	e := r.extents()
	return box32{float32(e.xBearing), float32(e.yBearing + e.height), float32(e.xBearing + e.width), float32(e.yBearing)}
}

// outline is the box of the points a glyph's outline draws, which is what a
// fill clipped to the glyph is bounded by: hb_font_draw_glyph through the
// extents' draw functions. VARC is asked, then the glyf outline, and then the
// CFF one, as HarfBuzz asks them; a glyph none draws is a void box.
func (c *colrInk) outline(gid int) box32 {
	if b, ok := c.outlines[gid]; ok {
		return b
	}
	b := void32
	switch {
	case c.f.varc != nil && c.f.varc.t.coverageIndex(gid) >= 0:
		// HarfBuzz draws a glyph through VARC before glyf or CFF; a glyph
		// VARC does not compose it draws as glyf or CFF would.
		b = c.f.varc.outlineBox(gid)
	case c.glyf != nil:
		b = c.glyfOutline(gid)
	case c.f.ink != nil:
		b = c.cffOutline(gid)
	}
	if c.outlines == nil {
		c.outlines = map[int]box32{}
	}
	c.outlines[gid] = b
	return b
}

// cffOutline is the box of what a CFF glyph's charstring draws.
func (c *colrInk) cffOutline(gid int) box32 {
	ink := c.f.ink
	ink.once.Do(func() {
		ink.outlines, _ = readCFFOutlines(ink.table, ink.numGlyphs)
		ink.budget = font.NewBudget(cffInkWork(len(ink.table)))
	})
	if ink.outlines == nil {
		return void32
	}
	ink.mu.Lock()
	defer ink.mu.Unlock()
	r := t2Run{o: ink.outlines, budget: ink.budget, draw: true}
	b, _ := r.bounds(gid, false)
	if !b.drew() {
		return void32
	}
	return box32{float32(b.minX), float32(b.minY), float32(b.maxX), float32(b.maxY)}
}

// glyfNumGlyphs is how many glyphs HarfBuzz's glyf reader will draw: as many
// as loca can address and maxp states, whichever is fewer.
func (c *colrInk) glyfNumGlyphs() int {
	size := 2
	if c.longLoca {
		size = 4
	}
	return min(max(1, len(c.loca)/size)-1, c.numGlyphs)
}

// glyfBytes is a glyph's entry, and nil for one loca does not place in glyf,
// which HarfBuzz reads as a glyph with no header at all.
func (c *colrInk) glyfBytes(gid int) []byte {
	var start, end int
	if c.longLoca {
		start, end = int(font.Be32(c.loca, 4*gid)), int(font.Be32(c.loca, 4*gid+4))
	} else {
		start, end = 2*font.Be16(c.loca, 2*gid), 2*font.Be16(c.loca, 2*gid+2)
	}
	if start > end || end > len(c.glyf) {
		return nil
	}
	return c.glyf[start:end]
}

// point32 is one point of a glyph as HarfBuzz carries it, in floats.
type point32 struct{ x, y float32 }

// glyfWalk is one glyph's points being gathered: HarfBuzz's all_points, and
// its bounds on the walk.
type glyfWalk struct {
	points []point32
	edges  int
	dec    decycler
	// contours, when it is set, has the walk keep what the outline needs
	// beyond its points: onCurve says of each point in points whether it is
	// on the curve, and ends the index in points of the last point of each
	// contour. Those are what Face.GlyphOutline draws from; a box needs
	// neither, and leaves this off.
	contours bool
	onCurve  []bool
	ends     []int
}

// maxGlyfPoints is HB_GLYF_MAX_POINTS, the most points one glyph may gather.
const maxGlyfPoints = 200000

// glyfOutline is the box of a TrueType glyph's points as HarfBuzz draws them:
// Glyph::get_points at the default instance, every point of every contour,
// each component's put through its transform in floats as HarfBuzz transforms
// it, and the whole shifted, at the top, by how far the glyph's side bearing
// is from its box. A glyph HarfBuzz cannot read draws nothing.
func (c *colrInk) glyfOutline(gid int) box32 {
	if gid < 0 || gid >= c.glyfNumGlyphs() {
		return void32
	}
	w := &glyfWalk{dec: decycler{tortoise: -1}}
	if !c.glyfPoints(gid, 0, w) || !c.charge(len(w.points)) {
		return void32
	}
	n := len(w.points) - 4
	shift := w.points[n].x
	b := void32
	for _, p := range w.points[:n] {
		b.add(float32(p.x-shift), p.y)
	}
	return b
}

// glyfPoints appends a glyph's points to the walk, followed by its four
// phantom points, as HarfBuzz's all_points holds them while it walks: the
// component that matches a point by number may name a phantom one.
func (c *colrInk) glyfPoints(gid, depth int, w *glyfWalk) bool {
	if depth > maxPhantomDepth || w.edges > maxPhantomEdges {
		return false
	}
	w.edges++
	g := c.glyfBytes(gid)
	contours, xMin, yMax := 0, 0, 0
	if len(g) >= 10 {
		contours = signed16(font.Be16(g, 0))
		xMin, yMax = signed16(font.Be16(g, 2)), signed16(font.Be16(g, 8))
	}
	phantoms := c.glyfPhantoms(gid, xMin, yMax)
	switch {
	case contours > 0:
		v := &varGlyph{}
		if decodeSimple(v, g, contours) != nil {
			return false
		}
		start := len(w.points)
		for i := 0; i < v.numOutlinePoints(); i++ {
			w.points = append(w.points, point32{float32(v.x[i]), float32(v.y[i])})
		}
		if w.contours {
			for i := 0; i < v.numOutlinePoints(); i++ {
				w.onCurve = append(w.onCurve, v.flags[i]&0x01 != 0)
			}
			for _, e := range v.ends {
				w.ends = append(w.ends, start+e)
			}
		}
	case contours < 0:
		if !c.glyfComposite(g, depth, w, &phantoms) {
			return false
		}
	}
	w.points = append(w.points, phantoms[:]...)
	if w.contours {
		w.onCurve = append(w.onCurve, false, false, false, false)
	}
	return len(w.points) <= maxGlyfPoints
}

// glyfPhantoms are a glyph's four phantom points as HarfBuzz sets them before
// any variation: left and right from its side bearing and advance, top and
// bottom from its top side bearing and vertical advance — an em where the face
// has no vertical metrics.
func (c *colrInk) glyfPhantoms(gid, xMin, yMax int) [4]point32 {
	lsb, _ := c.f.leftSideBearing(gid)
	hDelta := xMin - lsb
	v := &c.f.vert
	top := yMax + v.topSideBearing(gid)
	vAdvance := c.f.unitsPerEm
	if v.longMetrics > 0 {
		vAdvance = v.vAdvance(gid)
	}
	return [4]point32{
		{x: float32(hDelta)},
		{x: float32(c.f.advanceUnits(gid) + hDelta)},
		{y: float32(top)},
		{y: float32(top - vAdvance)},
	}
}

// glyfComposite gathers a composite glyph's components: each walked, put
// through its transform with its phantom points, and for one placed by
// matching points, moved so that its point lands on the one already gathered;
// then its phantom points are dropped. A component that would close a cycle is
// skipped, as HarfBuzz's decycler skips it; one whose metrics the composite
// takes gives it its phantom points, as they were before the transform.
func (c *colrInk) glyfComposite(g []byte, depth int, w *glyfWalk, phantoms *[4]point32) bool {
	w.dec.enter()
	defer w.dec.leave()
	ok := true
	eachComponentRecord(g, func(r componentRecord) bool {
		if !w.dec.visit(r.gid) {
			return true
		}
		before := len(w.points)
		if !c.glyfPoints(r.gid, depth+1, w) {
			ok = false
			return false
		}
		pts := w.points[before:]
		if r.flags&compUseMyMetrics != 0 {
			copy(phantoms[:], pts[len(pts)-4:])
		}
		r.place(pts)
		if r.flags&compArgsAreXY == 0 {
			if p1, p2 := r.arg1, r.arg2; p1 < len(w.points) && p2 < len(pts) {
				dx := float32(w.points[p1].x - pts[p2].x)
				dy := float32(w.points[p1].y - pts[p2].y)
				translate32(pts, dx, dy)
			}
		}
		w.points = w.points[:len(w.points)-4]
		if w.contours {
			w.onCurve = w.onCurve[:len(w.points)]
		}
		if len(w.points) > maxGlyfPoints {
			ok = false
			return false
		}
		return true
	})
	return ok
}

// componentRecord is one component of a composite glyph as HarfBuzz reads it.
type componentRecord struct {
	flags, gid int
	// arg1 and arg2 are the offset, or for a component placed by matching
	// points the two point numbers, which are unsigned where an offset is not.
	arg1, arg2 int
	matrix     [4]float32
}

// Two more composite flags, beside the ones glyfpoints.go and vertical.go
// name.
const (
	compScaledOffset   = 0x0800
	compUnscaledOffset = 0x1000
)

// eachComponentRecord calls fn with each component record of a composite, in
// order, until fn returns false or the records end where eachComponent says
// they do.
func eachComponentRecord(g []byte, fn func(componentRecord) bool) {
	at := 10
	eachComponent(g, func(flags, gid int) bool {
		r := componentRecord{flags: flags, gid: gid, matrix: [4]float32{1, 0, 0, 1}}
		p := at + 4
		if flags&compGID24 != 0 {
			p++
		}
		matching := flags&compArgsAreXY == 0
		if flags&compArgsAreWords != 0 {
			if matching {
				r.arg1, r.arg2 = font.Be16(g, p), font.Be16(g, p+2)
			} else {
				r.arg1, r.arg2 = signed16(font.Be16(g, p)), signed16(font.Be16(g, p+2))
			}
			p += 4
		} else {
			if matching {
				r.arg1, r.arg2 = int(g[p]), int(g[p+1])
			} else {
				r.arg1, r.arg2 = int(int8(g[p])), int(int8(g[p+1]))
			}
			p += 2
		}
		f2 := func(i int) float32 { return float32(signed16(font.Be16(g, p+2*i))) * float32(1.0/16384) }
		switch {
		case flags&compHaveScale != 0:
			r.matrix[0], r.matrix[3] = f2(0), f2(0)
			p += 2
		case flags&compHaveXYScale != 0:
			r.matrix[0], r.matrix[3] = f2(0), f2(1)
			p += 4
		case flags&compHave2x2 != 0:
			r.matrix = [4]float32{f2(0), f2(1), f2(2), f2(3)}
			p += 8
		}
		at = p
		return fn(r)
	})
}

// place is CompositeGlyphRecord::transform_points: the component's points
// through its matrix and moved by its offset — moved first where the font asks
// for scaled offsets.
func (r componentRecord) place(pts []point32) {
	var tx, ty float32
	if r.flags&compArgsAreXY != 0 {
		tx, ty = float32(r.arg1), float32(r.arg2)
	}
	identity := r.matrix == [4]float32{1, 0, 0, 1}
	transform := func(ps []point32) {
		if identity {
			return
		}
		for i, p := range ps {
			ps[i] = point32{
				float32(p.x*r.matrix[0]) + float32(p.y*r.matrix[2]),
				float32(p.x*r.matrix[1]) + float32(p.y*r.matrix[3]),
			}
		}
	}
	if r.flags&(compScaledOffset|compUnscaledOffset) == compScaledOffset {
		translate32(pts, tx, ty)
		transform(pts)
		return
	}
	transform(pts)
	translate32(pts, tx, ty)
}

func translate32(ps []point32, dx, dy float32) {
	if dx == 0 && dy == 0 {
		return
	}
	for i := range ps {
		ps[i].x += dx
		ps[i].y += dy
	}
}
