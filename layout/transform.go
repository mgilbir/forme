package layout

import (
	"fmt"
	"math"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// CSS Transforms 1: the transform and transform-origin properties.
//
// # What is applied
//
// A transform whose result maps every axis-aligned rectangle to an
// axis-aligned rectangle: any list of translate(), translateX(),
// translateY(), scale(), scaleX(), scaleY(), rotate() and matrix() whose
// product, about the transform-origin, is a turn by a multiple of a quarter,
// a scale along each axis and a move. Lists compose as §6 composes them —
// the functions multiplied left to right, so the rightmost acts on the box
// first, between a move to the origin and a move back — and a list whose
// pieces are not quarter turns is still applied when its product is one:
// "rotate(30deg) rotate(60deg)" is a quarter turn. Percentages in a
// translation are of the border box, which is the reference box of a box
// laid out by the CSS box model; so are the origin's.
//
// It works for the reason writing-mode's turn works (writingmode.go): a
// quarter turn and a scale along the axes take every rectangle in the display
// list — a background, a border, a clip, a link's area — to another
// rectangle, so they are expressed in the coordinates by the time a backend
// sees them, and the display list needed nothing new. The one thing a turn
// does not do for free is glyphs, and there DrawText already has the two
// quarter turns writing modes needed: a run across the page turned a quarter
// clockwise is a Sideways run, and turned anticlockwise an Anticlockwise one,
// exactly, because a backend draws a Sideways run as the horizontal run with
// its text matrix turned about At (see DrawText). A uniform scale is a font
// size. So the transform is applied to the finished display list of the
// box's stacking context, operation by operation, in mapOps.
//
// A coordinate a scale or a move by a fraction of a unit makes is taken to
// the nearest layout unit, a sixty-fourth of a pixel, by one rule for every
// coordinate (toUnit), so that edges that met before the transform meet
// after it. A quarter turn and a whole move make whole units, and are exact.
//
// # What is refused, and why
//
// Each of these is reported at the box, and the box is drawn untransformed,
// where layout put it, which is the page this engine drew before any of it
// was applied:
//
//   - a product that is not a quarter turn: rotate(45deg), skew(). The
//     display list has no operation that turns a picture or a glyph by an
//     arbitrary angle, and no rectangle survives one. Drawing these needs a
//     transformation matrix in the display list, which every backend would
//     have to learn — a decision about the display list's contract, and not
//     one this file takes;
//   - a mirror, scaleX(-1) and every product whose determinant is negative.
//     No operation draws a glyph or a picture backwards;
//   - every 3D function, perspective() among them. A 3D transform that
//     stays in the plane of the page is the 2D one it equals, and that is
//     the one to write;
//   - a box the display list cannot turn with it: one holding a picture,
//     upright text or a formula's glyphs when the product turns; text when
//     the product scales one axis more than the other, or turns it upside
//     down; and the few other operations mapOp names. These are found in the
//     operations themselves, since nothing before the paint knows what a box
//     paints, and are reported by the painter;
//   - the root element, whose background is the canvas's (CSS Backgrounds
//     3's canvas background) and which this engine does not turn without it;
//   - a row, a row group or a cell of a table, which share the table's
//     backgrounds and borders and are painted with them; and a box broken
//     across columns, whose fragments have no one border box to turn about.
//
// A transform that does not apply is not reported: CSS Transforms 1 §1.2
// gives it to transformable elements only, which a non-atomic inline box, a
// table column and a column group are not, and a browser does not transform
// them either. Drawing one untransformed is the right page, as a margin on a
// ::first-line ignored is, and a finding would keep a correct page off the
// clean count.
//
// A matrix that is not invertible — "scale(0)" — is applied and not refused:
// §6 says such a box "and its content do not get displayed", which is a page
// this engine can draw exactly.
//
// # What a transform does besides
//
// §2: "any value other than none for the transform property results in the
// creation of a stacking context", and the box becomes the containing block
// of every positioned box inside it. Both hold whether or not the transform
// itself is applied, since both are layout this engine does: see
// formsAStackingContext and containsFixed. Nothing else in layout moves —
// "the transform property does not affect the flow of the content
// surrounding the transformed element" — so the box's siblings stay where
// they were, and only what is measured from the page after layout sees the
// transformed box: the clip of an ancestor, which cuts the transformed box
// and not the box it was (visualeffects.go keeps the clips outside a
// transformed box apart from the ones inside it, for that), the natural size
// the page is scaled to fit and the page-overflow guard that checks it, and
// a link's area, which is an operation like any other and moves with it.

// affine is a 2D transformation: x' = a·x + c·y + e and y' = b·x + d·y + f,
// which is CSS's matrix(a, b, c, d, e, f). e and f are in layout units.
type affine struct{ a, b, c, d, e, f float64 }

var identityAffine = affine{a: 1, d: 1}

// times is m·n, the transformation that applies n and then m.
func (m affine) times(n affine) affine {
	return affine{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// translation is a move by (x, y) layout units.
func translation(x, y float64) affine { return affine{a: 1, d: 1, e: x, f: y} }

// snapTolerance is how close to a whole number an entry of a matrix has to be
// to be taken for it. Products of sines and cosines miss the whole numbers
// they stand for in the last bits — cos(90°) is 6e-17, and 100grad is
// 90.00000000000001 degrees — and a quarter turn written as two halves of one
// is still a quarter turn. A billionth of a scale factor is a billionth of a
// pixel on a box a thousand pixels across, so nothing that rounds here could
// have been seen.
const snapTolerance = 1e-9

func snap(v float64) float64 {
	if r := math.Round(v); math.Abs(v-r) < snapTolerance {
		return r
	}
	return v
}

// quarterTurn is a matrix in the slice decomposed: turns quarter turns
// clockwise, after a scale of sx along x and sy along y, both positive, and
// the move in m.
type quarterTurn struct {
	m      affine
	turns  int
	sx, sy float64
}

// uniform reports whether both axes are scaled alike, which is what a glyph
// and a blur need: a font size is one number.
func (q quarterTurn) uniform() bool { return q.sx == q.sy }

// classify decomposes a matrix into a quarter turn and a scale, or says why
// it is not one. A matrix with no inverse is hidden: what it would draw has
// no area.
//
// The four quarter turns of a scale S = diag(sx, sy) are, as matrix(a, b, c,
// d), (sx, 0, 0, sy), (0, sx, -sy, 0), (-sx, 0, 0, -sy) and (0, -sx, sy, 0)
// — rotate(90deg) is matrix(0, 1, -1, 0) and turns +x towards +y, which on a
// page whose y grows downwards is clockwise. Every other axis-aligned matrix
// has a negative determinant and is a mirror.
func classify(m affine) (q quarterTurn, hidden bool, why string) {
	for _, v := range []float64{m.a, m.b, m.c, m.d, m.e, m.f} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return quarterTurn{}, false, "it scales the box by more than a number can hold"
		}
	}
	m.a, m.b, m.c, m.d = snap(m.a), snap(m.b), snap(m.c), snap(m.d)
	if m.a*m.d-m.b*m.c == 0 {
		return quarterTurn{}, true, ""
	}
	q.m = m
	switch {
	case m.b == 0 && m.c == 0 && m.a > 0 && m.d > 0:
		q.turns, q.sx, q.sy = 0, m.a, m.d
	case m.a == 0 && m.d == 0 && m.b > 0 && m.c < 0:
		q.turns, q.sx, q.sy = 1, m.b, -m.c
	case m.b == 0 && m.c == 0 && m.a < 0 && m.d < 0:
		q.turns, q.sx, q.sy = 2, -m.a, -m.d
	case m.a == 0 && m.d == 0 && m.b < 0 && m.c > 0:
		q.turns, q.sx, q.sy = 3, -m.b, m.c
	case (m.b == 0 && m.c == 0) || (m.a == 0 && m.d == 0):
		return quarterTurn{}, false, "it mirrors the box, and no operation of the display list " +
			"draws a glyph or a picture backwards"
	default:
		return quarterTurn{}, false, "it turns the box by an angle that is not a multiple of " +
			"ninety degrees, or skews it, and the display list has no operation that turns a " +
			"picture or a glyph by any other angle"
	}
	return q, false, ""
}

// boxTransform is what layout decided about one fragment's transform.
type boxTransform struct {
	// on says the fragment's transform is applied: its stacking context is
	// painted and then mapped by q, unless hidden, which paints nothing.
	on     bool
	hidden bool
	q      quarterTurn
	// clip is what clips the transformed box from outside: the clip of every
	// box around it, which is applied after the transform and not before it.
	// The clips inside the box are its own and travel with it. See
	// resolveClips, which sets this, and its rounded corners in
	// Fragment.transformRound.
	clip Clip
}

// declaresTransform reports whether a box's transform is anything but none.
func declaresTransform(b *Box) bool {
	if b == nil || b.IsText() {
		return false
	}
	raw := ascii.TrimCSSSpace(b.Style.Get("transform"))
	return raw != "" && !ascii.EqualFold(raw, "none")
}

// transformsItsPaint reports whether a box's transform makes it a stacking
// context and the containing block of everything positioned inside it,
// which every value but none does on a transformable box (§2), applied or
// not. The root is left out: it is already a stacking context, and the
// containing block of the boxes inside it is the page, as it is for a
// filter (filterContains).
func transformsItsPaint(b *Box) bool {
	return b != nil && b.Parent != nil && transformable(b) && declaresTransform(b)
}

// resolveTransforms decides, for every fragment whose box declares a
// transform, whether it is applied, and reports the ones that are not.
//
// It runs after everything is placed, because a transform is of the box
// where layout put it and its percentages are of the border box layout gave
// it, and before resolveClips, which keeps apart the clips outside an applied
// transform from the ones inside it.
func (l *layouter) resolveTransforms(root *Fragment) {
	if root == nil || root.Box == nil {
		return
	}
	pieces := map[*Box]int{}
	var count func(f *Fragment)
	count = func(f *Fragment) {
		if f == nil || f.Box == nil {
			return
		}
		pieces[f.Box]++
		for _, c := range f.Children {
			count(c)
		}
	}
	count(root)

	reported := map[*Box]bool{}
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		if f == nil || f.Box == nil {
			return
		}
		f.transform = boxTransform{}
		if b := f.Box; declaresTransform(b) && transformable(b) {
			why := ""
			switch {
			case b.Parent == nil:
				why = "it is the root element's, whose background is the canvas's and " +
					"which this engine does not turn without it"
			case internalTableBox(b):
				why = "a row, a row group or a cell is painted with the backgrounds and " +
					"borders of the table around it, which would not move with it"
			case pieces[b] > 1:
				why = "the box is broken across columns, and its pieces have no one " +
					"border box to be transformed about"
			}
			var t boxTransform
			if why == "" {
				t, why = l.transformOf(b, f.BorderRect)
			}
			if why != "" {
				if !reported[b] {
					reported[b] = true
					l.reportTransform(b, why)
				}
			} else {
				f.transform = t
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
}

// reportTransform says a box's transform was not applied, and why.
func (l *layouter) reportTransform(b *Box, why string) { l.rec.ReportDetail(transformFinding(b, why)) }

// transformFinding is the one finding about a transform not applied, whether
// layout refused it or the paint did.
func transformFinding(b *Box, why string) Finding {
	return Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "the transform " + quoteValue(ascii.TrimCSSSpace(b.Style.Get("transform"))) +
			" was not applied to this box because " + why +
			"; it was drawn where layout put it, untransformed",
		Path:     PathOf(b.Element),
		Property: "transform",
	}
}

// transformOf reads a box's transform and transform-origin into the matrix
// that takes its border box r, in page coordinates, to where it is drawn —
// or says why it is not one this engine applies.
//
// §6's "transformation matrix": the origin is moved to, each function in
// order is multiplied in, and the origin is moved back. The leftmost
// function is the outermost, so the rightmost acts on the box first.
func (l *layouter) transformOf(b *Box, r Rect) (boxTransform, string) {
	ox, oy, ok := l.transformOrigin(b, r)
	if !ok {
		return boxTransform{}, "its transform-origin is not one this engine reads"
	}
	list, why := l.transformList(b, r)
	if why != "" {
		return boxTransform{}, why
	}
	at := translation(float64(r.X)+ox, float64(r.Y)+oy)
	back := translation(-(float64(r.X) + ox), -(float64(r.Y) + oy))
	q, hidden, why := classify(at.times(list).times(back))
	if why != "" {
		return boxTransform{}, why
	}
	return boxTransform{on: true, hidden: hidden, q: q}, ""
}

// transformList is the product of a box's transform functions, left to
// right, before the origin is moved to and back; its moves are in layout
// units, and r is the border box a percentage in a translation is of.
func (l *layouter) transformList(b *Box, r Rect) (affine, string) {
	vals, _ := css.ParseComponentValues(b.Style.Get("transform"))
	m := identityAffine
	unread := func(name string) (affine, string) {
		return affine{}, "its " + name + "() is not a value this engine reads"
	}
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if !v.IsFunction() {
			// The cascade has checked the grammar, so this is a keyword it
			// let through, which can only be none: nothing to multiply.
			continue
		}
		name := ascii.Lower(v.Token.Value)
		var args [][]css.ComponentValue
		for _, a := range splitTopLevelCommas(v.Values) {
			var kept []css.ComponentValue
			for _, c := range a {
				if c.IsToken() && c.Token.Kind == css.Whitespace {
					continue
				}
				kept = append(kept, c)
			}
			if len(kept) > 0 {
				args = append(args, kept)
			}
		}
		// length reads argument i as a length, a percentage of basis.
		length := func(i int, basis style.Unit) (float64, bool) {
			if i >= len(args) {
				return 0, true
			}
			v, ok := l.lengthOfValues(b, args[i])
			if !ok || v.Kind == style.LengthAuto {
				return 0, false
			}
			u, ok := v.Resolve(basis, true)
			return float64(u), ok
		}
		number := func(i int) (float64, bool) { return style.ParseNumberPercentage(args[i]) }
		angle := func(i int) (float64, bool) { return filterAngle(args[i]) }

		var f affine
		switch name {
		case "translate", "translatex", "translatey":
			x, y := 0.0, 0.0
			okx, oky := true, true
			switch name {
			case "translate":
				x, okx = length(0, r.W)
				y, oky = length(1, r.H)
			case "translatex":
				x, okx = length(0, r.W)
			case "translatey":
				y, oky = length(0, r.H)
			}
			if !okx || !oky || len(args) == 0 {
				return unread(name)
			}
			f = translation(x, y)
		case "scale", "scalex", "scaley":
			if len(args) == 0 {
				return unread(name)
			}
			sx, ok := number(0)
			if !ok {
				return unread(name)
			}
			sy := sx
			if len(args) > 1 {
				if sy, ok = number(1); !ok {
					return unread(name)
				}
			}
			switch name {
			case "scalex":
				sy = 1
			case "scaley":
				sx, sy = 1, sx
			}
			f = affine{a: sx, d: sy}
		case "rotate":
			if len(args) != 1 {
				return unread(name)
			}
			deg, ok := angle(0)
			if !ok {
				return unread(name)
			}
			f = rotation(deg)
		case "skew", "skewx", "skewy":
			if len(args) == 0 {
				return unread(name)
			}
			ax, ok := angle(0)
			if !ok {
				return unread(name)
			}
			ay := 0.0
			if name == "skewy" {
				ax, ay = 0, ax
			} else if len(args) > 1 {
				if ay, ok = angle(1); !ok {
					return unread(name)
				}
			}
			f = affine{a: 1, b: math.Tan(ay * math.Pi / 180), c: math.Tan(ax * math.Pi / 180), d: 1}
		case "matrix":
			if len(args) != 6 {
				return unread(name)
			}
			var n [6]float64
			for i := range n {
				var ok bool
				if n[i], ok = number(i); !ok {
					return unread(name)
				}
			}
			// The last two are a move, in pixels.
			px := float64(unitsPerPx())
			f = affine{a: n[0], b: n[1], c: n[2], d: n[3], e: n[4] * px, f: n[5] * px}
		case "matrix3d", "translate3d", "translatez", "scale3d", "scalez", "rotate3d",
			"rotatex", "rotatey", "rotatez", "perspective":
			return affine{}, "its " + name + "() is a 3D transform function, and this engine " +
				"applies only the 2D ones"
		default:
			return unread(name)
		}
		m = m.times(f)
	}
	return m, ""
}

// unitsPerPx is how many layout units one pixel is.
func unitsPerPx() style.Unit {
	u, _ := style.FromPx(1)
	return u
}

// rotation is rotate(deg): exactly a quarter turn where deg is a whole number
// of them — within snapTolerance, so that 100grad and 0.25turn are — and the
// sine and cosine of the angle otherwise.
func rotation(deg float64) affine {
	k := deg / 90
	if r := math.Round(k); math.Abs(k-r) < snapTolerance && math.Abs(r) < 1<<52 {
		switch ((int64(r) % 4) + 4) % 4 {
		case 0:
			return identityAffine
		case 1:
			return affine{b: 1, c: -1}
		case 2:
			return affine{a: -1, d: -1}
		case 3:
			return affine{b: -1, c: 1}
		}
	}
	s, c := math.Sincos(deg * math.Pi / 180)
	return affine{a: c, b: s, c: -s, d: c}
}

// transformOrigin reads a box's transform-origin as a point from the top left
// of its border box r, in layout units. §7: one value is a horizontal
// position unless it is top or bottom, with the other axis at the centre; two
// are horizontal then vertical, but for two keywords, which may come in
// either order; and a third, the depth, moves nothing a 2D transform does.
func (l *layouter) transformOrigin(b *Box, r Rect) (x, y float64, ok bool) {
	vals, _ := css.ParseComponentValues(b.Style.Get("transform-origin"))
	parts := splitValueParts(vals)
	half := style.Length{Kind: style.LengthPercent, Percent: 50}
	keyword := map[string]style.Length{
		"left": {Kind: style.LengthPercent}, "top": {Kind: style.LengthPercent},
		"center": half,
		"right":  {Kind: style.LengthPercent, Percent: 100},
		"bottom": {Kind: style.LengthPercent, Percent: 100},
	}
	read := func(part []css.ComponentValue) (style.Length, string, bool) {
		if kw, ok := identOf(part); ok {
			v, known := keyword[kw]
			return v, kw, known
		}
		v, ok := l.lengthOfValues(b, part)
		return v, "", ok && v.Kind != style.LengthAuto
	}
	var lx, ly style.Length
	switch len(parts) {
	case 1:
		v, kw, ok := read(parts[0])
		if !ok {
			return 0, 0, false
		}
		lx, ly = v, half
		if kw == "top" || kw == "bottom" {
			lx, ly = half, v
		}
	case 2, 3:
		v0, kw0, ok0 := read(parts[0])
		v1, kw1, ok1 := read(parts[1])
		if !ok0 || !ok1 {
			return 0, 0, false
		}
		lx, ly = v0, v1
		if kw0 == "top" || kw0 == "bottom" || kw1 == "left" || kw1 == "right" {
			lx, ly = v1, v0
		}
	default:
		return 0, 0, false
	}
	ux, okx := lx.Resolve(r.W, true)
	uy, oky := ly.Resolve(r.H, true)
	return float64(ux), float64(uy), okx && oky
}

// Mapping the display list.

// toUnit is a coordinate the matrix made, to the nearest layout unit. To the
// nearest, and by one function for every coordinate, so that two rectangles
// that shared an edge before the transform share it after: a box's background
// and the border band beside it meet where they met. A half goes up, on
// either side of nought, so that a whole number of units added before
// rounding is the same number added after: a tile one step along is exactly
// one step along.
func toUnit(v float64) style.Unit {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= float64(style.MaxUnit):
		return style.MaxUnit
	case v <= float64(style.MinUnit):
		return style.MinUnit
	}
	return style.Unit(math.Floor(v + 0.5))
}

func (m affine) point(p Point) Point {
	x, y := float64(p.X), float64(p.Y)
	return Point{X: toUnit(m.a*x + m.c*y + m.e), Y: toUnit(m.b*x + m.d*y + m.f)}
}

// rect is the rectangle a rectangle maps to, under a matrix that keeps the
// axes: its two far corners mapped, and put back in order.
func (m affine) rect(r Rect) Rect {
	p0 := m.point(Point{X: r.X, Y: r.Y})
	p1 := m.point(Point{X: r.Right(), Y: r.Bottom()})
	x0, x1 := style.Min(p0.X, p1.X), style.Max(p0.X, p1.X)
	y0, y1 := style.Min(p0.Y, p1.Y), style.Max(p0.Y, p1.Y)
	return Rect{X: x0, Y: y0, W: x1.Sub(x0), H: y1.Sub(y0)}
}

func (m affine) clip(c Clip) Clip {
	if c.Active {
		c.Rect = m.rect(c.Rect)
	}
	return c
}

// vector is a displacement under the matrix: the turn and the scale, and not
// the move.
func (m affine) vector(p Point) Point {
	x, y := float64(p.X), float64(p.Y)
	return Point{X: toUnit(m.a*x + m.c*y), Y: toUnit(m.b*x + m.d*y)}
}

func (q quarterTurn) point(p Point) Point { return q.m.point(p) }
func (q quarterTurn) rect(r Rect) Rect    { return q.m.rect(r) }
func (q quarterTurn) clip(c Clip) Clip    { return q.m.clip(c) }

// length is a length that lies along no axis — a font size, a blur — under
// a uniform scale.
func (q quarterTurn) length(u style.Unit) style.Unit { return toUnit(float64(u) * q.sx) }

// exact is a length along the box's x axis (or its y axis, when y is set)
// scaled, when the result is a whole number of layout units; a tiling's step
// has to be, or the error of rounding it gathers once per tile.
func (q quarterTurn) exact(u style.Unit, y bool) (style.Unit, bool) {
	s := q.sx
	if y {
		s = q.sy
	}
	v := float64(u) * s
	r := math.Round(v)
	return toUnit(r), math.Abs(v-r) < snapTolerance
}

func (q quarterTurn) path(p Path) Path {
	out := make(Path, len(p))
	for i, s := range p {
		s.Point = q.point(s.Point)
		if s.Op == ArcTo {
			// The scale stretches the ellipse along its own axes, and the turn
			// takes the point at angle a on it to the point at a + 90° on the
			// ellipse whose axes it exchanged; the sweep keeps its direction,
			// since nothing here mirrors.
			s.Center = q.point(s.Center)
			rx, ry := toUnit(float64(s.RadiusX)*q.sx), toUnit(float64(s.RadiusY)*q.sy)
			if q.turns%2 == 1 {
				rx, ry = ry, rx
			}
			s.RadiusX, s.RadiusY = rx, ry
			s.StartAngle += float64(90 * q.turns)
		}
		out[i] = s
	}
	return out
}

// text maps a run, or says why it cannot be: a run is drawn by a backend from
// its pen position, its size and which way it is turned, so the matrix has to
// be one a size and one of the three turns DrawText states can say.
func (q quarterTurn) text(v DrawText) (DrawText, string) {
	if !q.uniform() {
		return v, "it scales text more along one axis than the other, and a run of text " +
			"is drawn at one font size"
	}
	if v.Upright && q.turns != 0 {
		return v, "it turns text set upright, which no operation of the display list draws " +
			"lying along a line"
	}
	facing := 0
	switch {
	case v.Sideways && v.Anticlockwise:
		facing = 3
	case v.Sideways:
		facing = 1
	}
	facing = (facing + q.turns) % 4
	if !v.Upright {
		switch facing {
		case 2:
			return v, "it turns text upside down, which no operation of the display list draws"
		case 0:
			v.Sideways, v.Anticlockwise = false, false
		case 1:
			v.Sideways, v.Anticlockwise = true, false
		case 3:
			v.Sideways, v.Anticlockwise = true, true
		}
	}
	v.At = q.point(v.At)
	v.Size = q.length(v.Size)
	v.CharSpacing = q.length(v.CharSpacing)
	v.Clip = q.clip(v.Clip)
	return v, ""
}

// tiling maps the area, the first tile and the steps of a tiling, which have
// to come out whole: a step rounded is wrong once more with every tile.
func (q quarterTurn) tiling(area, tile Rect, stepX, stepY style.Unit) (Rect, Rect, style.Unit, style.Unit, string) {
	sx, okx := q.exact(stepX, false)
	sy, oky := q.exact(stepY, true)
	_, okw := q.exact(tile.W, false)
	_, okh := q.exact(tile.H, true)
	if !okx || !oky || !okw || !okh {
		return Rect{}, Rect{}, 0, 0, "it scales a tiled background by a factor that does not " +
			"take its tiles to a whole number of layout units, and the tiles would drift apart"
	}
	if q.turns%2 == 1 {
		sx, sy = sy, sx
	}
	return q.rect(area), q.rect(tile), sx, sy, ""
}

// gradient maps a gradient laid out for a tile whose top left was from to one
// laid out for the tile whose top left is to.
func (q quarterTurn) gradient(g Gradient, from, to Point) (Gradient, string) {
	at := func(p Point) Point {
		m := q.point(Point{X: from.X.Add(p.X), Y: from.Y.Add(p.Y)})
		return Point{X: m.X.Sub(to.X), Y: m.Y.Sub(to.Y)}
	}
	switch g.Kind {
	case LinearGradient:
		// A point's offset is where the perpendicular through it meets the
		// gradient line, and a scale that is not uniform keeps a right angle
		// only between the axes themselves.
		if !q.uniform() && g.Start.X != g.End.X && g.Start.Y != g.End.Y {
			return g, "it scales a gradient that runs at a slant more along one axis than " +
				"the other, which bends its stripes off the right angle a linear gradient keeps"
		}
		g.Start, g.End = at(g.Start), at(g.End)
	case RadialGradient:
		g.Center = at(g.Center)
		rx, ry := toUnit(float64(g.RadiusX)*q.sx), toUnit(float64(g.RadiusY)*q.sy)
		if q.turns%2 == 1 {
			rx, ry = ry, rx
		}
		g.RadiusX, g.RadiusY = style.Max(rx, 1), style.Max(ry, 1)
	case ConicGradient:
		if !q.uniform() {
			return g, "it scales a conic gradient more along one axis than the other, which " +
				"bends the rays it is constant along"
		}
		g.Center = at(g.Center)
		g.FromAngle = math.Mod(g.FromAngle+float64(90*q.turns), 360)
	default:
		return g, "it turns a gradient of a kind this engine does not know"
	}
	return g, ""
}

// mapOps is a list of operations under a transform, or the first reason one
// of them cannot be drawn under it.
func (q quarterTurn) mapOps(ops []Op) ([]Op, string) {
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		mapped, why := q.mapOp(op)
		if why != "" {
			return nil, why
		}
		out = append(out, mapped)
	}
	return out, ""
}

// mapOp is one operation under a transform. Every kind the display list has
// is here; one it does not know is refused, since a mark this does not know
// how to move would be drawn where the box was.
func (q quarterTurn) mapOp(op Op) (Op, string) {
	const turnsAPicture = "it turns a picture, and an image is drawn upright in the " +
		"rectangle it is given"
	switch v := op.(type) {
	case FillRect:
		v.Rect = q.rect(v.Rect)
		return v, ""
	case Link:
		rects := make([]Rect, len(v.Rects))
		for i, r := range v.Rects {
			rects[i] = q.rect(r)
		}
		v.Rects = rects
		return v, ""
	case FillPath:
		v.Path = q.path(v.Path)
		v.Clip = q.clip(v.Clip)
		return v, ""
	case ClipPath:
		inner, why := q.mapOps(v.Ops)
		if why != "" {
			return v, why
		}
		return ClipPath{Path: q.path(v.Path), Ops: inner}, ""
	case DrawImage:
		if q.turns != 0 {
			return v, turnsAPicture
		}
		v.Rect = q.rect(v.Rect)
		v.Clip = q.clip(v.Clip)
		return v, ""
	case TileImage:
		if q.turns != 0 {
			return v, turnsAPicture
		}
		var why string
		v.Clip, v.Tile, v.StepX, v.StepY, why = q.tiling(v.Clip, v.Tile, v.StepX, v.StepY)
		return v, why
	case FillGradient:
		from := v.Tile.Origin()
		var why string
		v.Clip, v.Tile, v.StepX, v.StepY, why = q.tiling(v.Clip, v.Tile, v.StepX, v.StepY)
		if why != "" {
			return v, why
		}
		v.Gradient, why = q.gradient(v.Gradient, from, v.Tile.Origin())
		return v, why
	case DrawText:
		return q.text(v)
	case DrawTextShadow:
		run, why := q.text(v.Run)
		if why != "" {
			return v, why
		}
		v.Run, v.StdDev = run, q.length(v.StdDev)
		return v, ""
	case DrawEmphasisMark:
		mark, why := q.text(v.Mark)
		v.Mark = mark
		return v, why
	case DrawGlyphs:
		// Placed by offsets in thousandths of an em, which a size carries,
		// and drawn upright: a formula's glyphs have no turn to be given.
		if q.turns != 0 || !q.uniform() {
			return v, "it turns or stretches a formula's glyphs, which are drawn upright at one size"
		}
		v.At = q.point(v.At)
		v.Size = q.length(v.Size)
		v.Clip = q.clip(v.Clip)
		return v, ""
	case FilterGroup:
		filters := make([]FilterFunction, len(v.Filters))
		for i, f := range v.Filters {
			switch f.Kind {
			case FilterBlur, FilterDropShadow:
				if f.StdDev > 0 && !q.uniform() {
					return v, "it scales a blur more along one axis than the other, and a blur " +
						"has one deviation"
				}
				f.StdDev = q.length(f.StdDev)
				f.Offset = q.m.vector(f.Offset)
			}
			filters[i] = f
		}
		inner, why := q.mapOps(v.Ops)
		if why != "" {
			return v, why
		}
		g := newFilterGroup(filters, inner)
		g.Clip = q.clip(v.Clip)
		return g, ""
	}
	return op, fmt.Sprintf("it would move a %T, which this engine does not know how to transform", op)
}

// Painting.

// transformLog is a mark recorded for an opacity group while a transform
// was being painted: it is in the coordinates of the box being transformed,
// and is moved with the operations it describes if the transform is applied.
type transformLog struct {
	g *group
	i int
}

// transforming paints a fragment's stacking context and applies its
// transform to what it painted, and then the clip and the curves around the
// box, which are outside the transform.
//
// A transform that cannot be drawn is reported, and what the box painted is
// left where layout put it — cut by the same clip it would have been, which
// resolveClips moved out of what it holds to here.
func (p *painter) transforming(f *Fragment, paint func()) {
	t := f.transform
	if !t.on {
		paint()
		return
	}
	if p.applied == nil {
		p.applied = transformsApplied{}
	}
	if t.hidden {
		// §6: "the object and its content do not get displayed". Nothing is
		// painted, so nothing — a mark, a link — is left to be found.
		p.applied[f] = true
		return
	}
	if t.clip.blocks() {
		return
	}
	at := len(p.ops)
	logAt := len(p.transformLogs)
	p.transformDepth++
	paint()
	p.transformDepth--
	// Mapping copies every operation the box painted, and a transformed box
	// inside another is copied again by the one around it: a stack of them
	// costs its depth times what is inside. So the copies are charged as
	// marks are, and past the budget the box is drawn where layout put it.
	n, _ := countOpsUpTo(p.ops[at:], math.MaxInt64)
	if !p.rec.chargeMark(satMul(n, costOp), "the transforms past that point, which were drawn untransformed") {
		// Reported once, by the budget: what was cut, and where.
	} else if mapped, why := t.q.mapOps(p.ops[at:]); why != "" {
		p.reportOnce(f.Box, "transform", transformFinding(f.Box, why))
	} else {
		p.ops = append(p.ops[:at], mapped...)
		for _, l := range p.transformLogs[logAt:] {
			if r := l.g.marks[l.i].rect; !r.Empty() {
				l.g.marks[l.i].rect = t.q.rect(r)
			}
		}
		p.applied[f] = true
	}
	if p.transformDepth == 0 {
		p.transformLogs = p.transformLogs[:logAt]
	}
	if t.clip.Active {
		p.ops = clipOps(p.ops, at, t.clip)
	}
	p.rounding(at, f.transformRound)
}

// addMarks hands marks to an opacity group, logging them while a transform is
// being painted so that they move with what they describe.
func (p *painter) addMarks(g *group, marks []groupMark) {
	if p.transformDepth > 0 {
		for i := range marks {
			p.transformLogs = append(p.transformLogs, transformLog{g: g, i: len(g.marks) + i})
		}
	}
	g.marks = append(g.marks, marks...)
}

// transformsApplied is the fragments whose transform the paint applied,
// hidden ones among them. A fragment whose transform the paint refused is not
// in it, and was drawn untransformed.
type transformsApplied map[*Fragment]bool

// underTransforms walks a fragment tree with the transform the paint applied
// to each fragment composed with the ones around it, and the clip outside
// each transformed box mapped to the page, so that what is measured from the
// page after layout — the natural size, the font sizes — sees the box where
// it was drawn. visit is told the fragment, the matrix it is drawn through
// (nil for none), and the clip outside every transform it is inside. A
// fragment whose transform is hidden is not visited, and nor is anything
// inside it: none of it is on the page.
func underTransforms(root *Fragment, applied transformsApplied, visit func(f *Fragment, m *affine, outer Clip)) {
	var walk func(f *Fragment, m *affine, outer Clip)
	walk = func(f *Fragment, m *affine, outer Clip) {
		if f == nil {
			return
		}
		if t := f.transform; t.on {
			c := t.clip
			if m != nil {
				c = m.clip(c)
			}
			outer = outer.meet(c)
			if applied[f] {
				if t.hidden {
					return
				}
				inner := t.q.m
				if m != nil {
					inner = m.times(inner)
				}
				m = &inner
			}
		}
		visit(f, m, outer)
		for _, c := range f.Children {
			walk(c, m, outer)
		}
	}
	walk(root, nil, Clip{})
}

// lengthScale is how much a matrix applied by the paint scales a length that
// lies along no axis, a font size: the square root of the area it scales by,
// which is the scale itself wherever text was drawn through it, since text
// is drawn only through a uniform one.
func (m *affine) lengthScale() float64 {
	if m == nil {
		return 1
	}
	return math.Sqrt(math.Abs(m.a*m.d - m.b*m.c))
}
