package layout

import (
	"math"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// CSS Backgrounds 3 §4: rounded corners.
//
// # What is rounded
//
// A box with a border-radius is rounded in four places, each by §4.3's rule
// that whatever clips to one of its edges clips to that edge's curve:
//
//   - its border, which is drawn between the curve of the border edge and the
//     curve of the padding edge, §4.2's inner radii;
//   - its background, clipped to the curve of the edge background-clip names;
//   - its content, where "overflow" clips it, to the curve of the padding
//     edge — but only when overflow clips on both axes, which §4.3 says in as
//     many words;
//   - a replaced element's picture, to the curve of the content edge.
//
// Three things are not, and each is reported where it is met rather than left
// square in silence: the rounded corners of a dotted or dashed border, which are
// drawn solid (the straight part between them keeps its dashes); an outline,
// which CSS UI 4 says should follow the curve and here does not; and a
// percentage radius on an inline box broken across lines, which §4.1 measures
// against the whole box and this measures against each piece.
//
// # Where the colours meet
//
// §4.4 puts a colour change at a corner somewhere on the curve and leaves the
// place "a continuous monotonic function of the ratio of the border widths",
// with the one fixed point that a side of no width gives the whole corner to the
// other. The function here is the simplest that is both: the corner's quarter
// turn is shared in proportion to the two widths, so equal borders meet at 45
// degrees, and the line between the two colours runs from that point on the
// outer curve to the point at the same angle on the inner one — which, for a
// corner with no radius, is the mitre every square border already has.

// usedRadii is §4.1's radii for a fragment, with §4.5's scaling applied: every
// radius shrunk by one factor until no two on a side overlap.
//
// The table rule is §4.6's: the properties apply to a table and a table cell in
// the separated borders model, and to no other part of a table, and to none of
// it in the collapsing model.
func (l *layouter) usedRadii(f *Fragment) Radii {
	b := f.Box
	if b == nil || f.BorderRect.Empty() {
		return Radii{}
	}
	names := [4]string{"border-top-left-radius", "border-top-right-radius",
		"border-bottom-right-radius", "border-bottom-left-radius"}
	raws := [4]string{}
	any := false
	for i, n := range names {
		raws[i] = b.Style.Get(n)
		any = any || (raws[i] != "" && raws[i] != "0" && raws[i] != "0px")
	}
	if !any {
		return Radii{}
	}
	switch b.Inner {
	case InnerTableRowGroup, InnerTableRow, InnerTableColumnGroup, InnerTableColumn:
		return Radii{}
	case InnerTable, InnerTableCell:
		if f.inCollapsedGrid || len(f.collapsed) > 0 || collapses(b) {
			return Radii{}
		}
	}
	w, h := f.BorderRect.W, f.BorderRect.H
	var c [4]Corner
	for i, raw := range raws {
		c[i] = l.cornerRadius(b, raw, w, h)
	}
	// §4.5: f = min(Li/Si), and every radius times f when f < 1.
	scale := 1.0
	for _, side := range [4]struct {
		length style.Unit
		sum    float64
	}{
		{w, c[0].X.Px() + c[1].X.Px()},
		{h, c[1].Y.Px() + c[2].Y.Px()},
		{w, c[2].X.Px() + c[3].X.Px()},
		{h, c[3].Y.Px() + c[0].Y.Px()},
	} {
		if side.sum > 0 {
			scale = math.Min(scale, side.length.Px()/side.sum)
		}
	}
	for i := range c {
		if scale < 1 {
			c[i].X, c[i].Y = c[i].X.Mul(scale), c[i].Y.Mul(scale)
		}
		if c[i].X <= 0 || c[i].Y <= 0 {
			// "If either length is zero, the corner is square."
			c[i] = Corner{}
		}
	}
	// §8.6's slice model, which is what an inline box broken across lines is
	// drawn by: as one box cut into pieces, so only the pieces at its two ends
	// have the ends' corners.
	if f.slicedLeft {
		c[0], c[3] = Corner{}, Corner{}
	}
	if f.slicedRight {
		c[1], c[2] = Corner{}, Corner{}
	}
	return radiiOf(c)
}

// collapses reports whether a table box lays its borders out in the collapsing
// model. A cell asks its table.
func collapses(b *Box) bool {
	for t := b; t != nil; t = t.Parent {
		if t.Inner == InnerTable {
			return t.Style.Get("border-collapse") == "collapse"
		}
	}
	return false
}

// cornerRadius reads one corner's computed value: one or two length-percentages,
// the horizontal radius first and of the border box's width, the vertical of
// its height. A value that is not one is a square corner — the cascade has
// already refused anything the grammar does not take.
func (l *layouter) cornerRadius(b *Box, raw string, w, h style.Unit) Corner {
	vals, _ := css.ParseComponentValues(raw)
	parts := splitValueParts(vals)
	if len(parts) == 0 || len(parts) > 2 {
		return Corner{}
	}
	lens := make([]style.Length, 0, 2)
	for _, part := range parts {
		length, ok := l.lengthOfValues(b, part)
		if !ok || !isLengthPercentage(length) {
			return Corner{}
		}
		lens = append(lens, length)
	}
	if len(lens) == 1 {
		lens = append(lens, lens[0])
	}
	x, _ := lens[0].Resolve(w, true)
	y, _ := lens[1].Resolve(h, true)
	if x < 0 || y < 0 {
		return Corner{}
	}
	return Corner{X: x, Y: y}
}

// hasPercentRadius reports whether any of a box's radii is a percentage, which
// is measured against the whole of a box broken across lines and here against
// each of its pieces.
func hasPercentRadius(b *Box) bool {
	for _, n := range [4]string{"border-top-left-radius", "border-top-right-radius",
		"border-bottom-right-radius", "border-bottom-left-radius"} {
		for i := 0; i < len(b.Style.Get(n)); i++ {
			if b.Style.Get(n)[i] == '%' {
				return true
			}
		}
	}
	return false
}

// paddingRadii and contentRadii are §4.2's inner curves: the border radii less
// the border, and less the padding as well.
func (f *Fragment) paddingRadii() Radii { return insetRadii(f.radii, f.Border) }

func (f *Fragment) contentRadii() Radii {
	return insetRadii(f.radii, f.Border.Add(f.Padding))
}

// radiiFor is the curve of the box an edge names: the border box, the padding
// box or the content box.
func (f *Fragment) radiiFor(which bgBox) Radii {
	switch which {
	case bgPaddingBox:
		return f.paddingRadii()
	case bgContentBox:
		return f.contentRadii()
	}
	return f.radii
}

// Drawing a rounded border.

// borderLayer is one band of a border's style, as fractions of its width from
// the border edge in: a solid border is one band, a double border two with a
// gap, and a groove or a ridge two in two tones.
type borderLayer struct {
	from, to float64
	colour   style.RGBA
}

// layersOf is a side's style as bands, the same bands and tones paintEdge
// draws a square border in.
func layersOf(kind borderStyle, colour style.RGBA, s side, thickness style.Unit) []borderLayer {
	topLeft := s == sideTop || s == sideLeft
	dark, light := shade(colour, 0.5), colour
	switch kind {
	case borderNone, borderHidden:
		return nil
	case borderDouble:
		if thickness.Div(3) <= 0 {
			break
		}
		return []borderLayer{{0, 1.0 / 3, colour}, {2.0 / 3, 1, colour}}
	case borderInset, borderOutset:
		lit := (kind == borderOutset) == topLeft
		if lit {
			return []borderLayer{{0, 1, light}}
		}
		return []borderLayer{{0, 1, dark}}
	case borderGroove, borderRidge:
		outer, inner := dark, light
		if kind == borderRidge {
			outer, inner = light, dark
		}
		if !topLeft {
			outer, inner = inner, outer
		}
		if thickness.Div(2) <= 0 {
			return []borderLayer{{0, 1, outer}}
		}
		return []borderLayer{{0, 0.5, outer}, {0.5, 1, inner}}
	}
	return []borderLayer{{0, 1, colour}}
}

// ringEdge is one of the curves a border is drawn between: the border edge
// moved in by a fraction of each side's width, and its radii moved in with it.
func ringEdge(outer Rect, radii Radii, e Edges, frac float64) (Rect, Radii) {
	in := Edges{Top: e.Top.Mul(frac), Right: e.Right.Mul(frac),
		Bottom: e.Bottom.Mul(frac), Left: e.Left.Mul(frac)}
	return outer.Inset(in), insetRadii(radii, in)
}

// transitions is, for each corner in clockwise order from the top left, the
// angle in degrees at which its two sides' colours meet: the corner's quarter
// turn begins at the side before it (clockwise) and is shared in proportion to
// the two widths. See the note at the top of this file.
func transitions(e Edges) [4]float64 {
	widths := [4]float64{e.Top.Px(), e.Right.Px(), e.Bottom.Px(), e.Left.Px()}
	var out [4]float64
	for k := 0; k < 4; k++ {
		first, second := widths[(k+3)%4], widths[k] // the side before, then after
		share := 0.5
		if first+second > 0 {
			share = first / (first + second)
		}
		out[k] = cornerStart[k] + 90*share
	}
	return out
}

// curveAngle is the angle an arc is written in, for a point on a corner's
// ellipse that lies in a given direction from its centre. The two differ for an
// ellipse that is not a circle: a direction of 45 degrees meets a wide ellipse
// at a smaller angle of the ellipse's own. The answer is kept in the corner's
// quarter turn.
func curveAngle(c Corner, k int, dir float64) float64 {
	d := dir * math.Pi / 180
	a := math.Atan2(c.X.Px()*math.Sin(d), c.Y.Px()*math.Cos(d)) * 180 / math.Pi
	for a < cornerStart[k] {
		a += 360
	}
	for a > cornerStart[k]+90 {
		a -= 360
	}
	return a
}

// cornerPoint is the point of a corner's curve in a direction from its centre,
// or the corner itself when it is square.
func cornerPoint(box Rect, radii Radii, k int, dir float64) Point {
	c := radii.corners()[k]
	ctr := cornerCentre(box, k, c)
	if c.X <= 0 || c.Y <= 0 {
		return ctr
	}
	x, y := arcPoint(PathSegment{Center: ctr, RadiusX: c.X, RadiusY: c.Y}, curveAngle(c, k, dir))
	return pointPx(x, y)
}

// cornerArc appends a corner's curve between two directions from its centre,
// or a line to the corner when it is square.
func cornerArc(p Path, box Rect, radii Radii, k int, fromDir, toDir float64) Path {
	c := radii.corners()[k]
	ctr := cornerCentre(box, k, c)
	if c.X <= 0 || c.Y <= 0 {
		return append(p, PathSegment{Op: LineTo, Point: ctr})
	}
	a0, a1 := curveAngle(c, k, fromDir), curveAngle(c, k, toDir)
	return append(p, PathSegment{Op: ArcTo, Center: ctr, RadiusX: c.X, RadiusY: c.Y,
		StartAngle: a0, SweepAngle: a1 - a0})
}

// sideRegion is the part of the ring between two curves that belongs to one
// side: from where its colour begins on the corner before it to where it ends on
// the corner after, along the outer curve, and back along the inner.
func sideRegion(outer Rect, oR Radii, inner Rect, iR Radii, s side, at [4]float64) Path {
	prev, next := int(s), (int(s)+1)%4
	p := Path{{Op: MoveTo, Point: cornerPoint(outer, oR, prev, at[prev])}}
	p = cornerArc(p, outer, oR, prev, at[prev], cornerStart[prev]+90)
	p = cornerArc(p, outer, oR, next, cornerStart[next], at[next])
	p = append(p, PathSegment{Op: LineTo, Point: cornerPoint(inner, iR, next, at[next])})
	p = cornerArc(p, inner, iR, next, at[next], cornerStart[next])
	p = cornerArc(p, inner, iR, prev, cornerStart[prev]+90, at[prev])
	return append(p, PathSegment{Op: ClosePath})
}

// ring is the whole band between two curves, as two closed shapes filled by the
// even-odd rule.
func ring(outer Rect, oR Radii, inner Rect, iR Radii) Path {
	p := roundedRect(outer, oR)
	if inner.Empty() {
		return p
	}
	return append(p, roundedRect(inner, iR)...)
}

// roundedBorders paints a box's border when any of its corners is round.
func (p *painter) roundedBorders(f *Fragment) {
	r, e := f.BorderRect, f.Border
	names := [4]string{"top", "right", "bottom", "left"}
	widths := [4]style.Unit{e.Top, e.Right, e.Bottom, e.Left}
	type sideStyle struct {
		kind   borderStyle
		colour style.RGBA
		paints bool
	}
	var sides [4]sideStyle
	for i, n := range names {
		c, ok := p.color(f.Box, "border-"+n+"-color")
		kind := parseBorderStyle(f.Box.Style.Get("border-" + n + "-style"))
		sides[i] = sideStyle{kind: kind, colour: c,
			paints: ok && c.A > 0 && widths[i] > 0 && kind != borderNone && kind != borderHidden}
	}
	at := transitions(e)

	// One ring per band when every side is the same solid or double border:
	// the common case, drawn without seams where the sides would meet.
	same := true
	for i := 1; i < 4; i++ {
		same = same && sides[i] == sides[0]
	}
	if same && sides[0].paints && (sides[0].kind == borderSolid || sides[0].kind == borderDouble) {
		for _, layer := range layersOf(sides[0].kind, sides[0].colour, sideTop, e.Top) {
			o, oR := ringEdge(r, f.radii, e, layer.from)
			in, iR := ringEdge(r, f.radii, e, layer.to)
			p.emit(FillPath{Path: ring(o, oR, in, iR), Color: layer.colour})
		}
		return
	}

	for i, s := range sides {
		if !s.paints {
			continue
		}
		if s.kind == borderDashed || s.kind == borderDotted {
			p.roundedDashes(f, side(i), s.kind, s.colour, at)
			continue
		}
		for _, layer := range layersOf(s.kind, s.colour, side(i), widths[i]) {
			o, oR := ringEdge(r, f.radii, e, layer.from)
			in, iR := ringEdge(r, f.radii, e, layer.to)
			p.emit(FillPath{Path: sideRegion(o, oR, in, iR, side(i), at), Color: layer.colour})
		}
	}
}

// roundedDashes paints a dotted or dashed side of a rounded border: its marks
// along the straight part between the corners, as paintEdge draws them, and its
// part of each rounded corner solid, which is reported. A mark that followed the
// curve would need the curve's length cut into equal pieces, and an ellipse's
// arc length has no closed form.
func (p *painter) roundedDashes(f *Fragment, s side, kind borderStyle, colour style.RGBA, at [4]float64) {
	r, e, c := f.BorderRect, f.Border, f.radii.corners()
	region := sideRegion(r, f.radii, r.Inset(e), f.paddingRadii(), s, at)
	bounds := region.Bounds()
	var band Rect
	horizontal := s == sideTop || s == sideBottom
	switch s {
	case sideTop, sideBottom:
		x0 := style.Max(r.X.Add(c[0].X), r.X.Add(e.Left))
		x1 := style.Min(r.Right().Sub(c[1].X), r.Right().Sub(e.Right))
		y, w := r.Y, e.Top
		if s == sideBottom {
			x0 = style.Max(r.X.Add(c[3].X), r.X.Add(e.Left))
			x1 = style.Min(r.Right().Sub(c[2].X), r.Right().Sub(e.Right))
			y, w = r.Bottom().Sub(e.Bottom), e.Bottom
		}
		band = Rect{X: x0, Y: y, W: x1.Sub(x0), H: w}
	default:
		y0 := style.Max(r.Y.Add(c[1].Y), r.Y.Add(e.Top))
		y1 := style.Min(r.Bottom().Sub(c[2].Y), r.Bottom().Sub(e.Bottom))
		x, w := r.Right().Sub(e.Right), e.Right
		if s == sideLeft {
			y0 = style.Max(r.Y.Add(c[0].Y), r.Y.Add(e.Top))
			y1 = style.Min(r.Bottom().Sub(c[3].Y), r.Bottom().Sub(e.Bottom))
			x, w = r.X, e.Left
		}
		band = Rect{X: x, Y: y0, W: w, H: y1.Sub(y0)}
	}
	thickness := band.H
	if !horizontal {
		thickness = band.W
	}
	// The corners: what of the side's region is not the straight band, solid.
	var rest []Op
	if band.Empty() {
		rest = append(rest, FillRect{Rect: bounds, Color: colour})
	} else if horizontal {
		rest = append(rest,
			FillRect{Rect: Rect{X: bounds.X, Y: bounds.Y, W: band.X.Sub(bounds.X), H: bounds.H}, Color: colour},
			FillRect{Rect: Rect{X: band.Right(), Y: bounds.Y, W: bounds.Right().Sub(band.Right()), H: bounds.H}, Color: colour})
	} else {
		rest = append(rest,
			FillRect{Rect: Rect{X: bounds.X, Y: bounds.Y, W: bounds.W, H: band.Y.Sub(bounds.Y)}, Color: colour},
			FillRect{Rect: Rect{X: bounds.X, Y: band.Bottom(), W: bounds.W, H: bounds.Bottom().Sub(band.Bottom())}, Color: colour})
	}
	kept := rest[:0]
	for _, op := range rest {
		if !op.(FillRect).Rect.Empty() {
			kept = append(kept, op)
		}
	}
	if len(kept) > 0 {
		p.emit(ClipPath{Path: region, Ops: kept})
		p.reportOnce(f.Box, "dashed-corner", Finding{
			Rule:     RuleUnsupportedValue,
			Source:   AtHTML(offsetOf(f.Box)),
			Message:  "the rounded corners of a dotted or dashed border were drawn solid; the straight part between them has its marks",
			Path:     PathOf(f.Box.Element),
			Property: "border-" + [4]string{"top", "right", "bottom", "left"}[s] + "-style",
		})
	}
	if !band.Empty() {
		p.paintEdge(band, kind, colour, s, thickness)
	}
}

// reportOnce raises a finding about a box once, however many of its fragments
// or sides meet the same thing.
func (p *painter) reportOnce(b *Box, what string, f Finding) {
	type key struct {
		b    *Box
		what string
	}
	if p.reported == nil {
		p.reported = map[any]bool{}
	}
	k := key{b, what}
	if p.reported[k] {
		return
	}
	p.reported[k] = true
	p.rec.ReportDetail(f)
}

// Clipping to a curve.

// roundClip is one rounded rectangle something is clipped to, and the ones
// around it. A list rather than a slice, for the reason opacityOwner gives:
// two siblings extend one chain, and a slice extended in place would hand the
// second the first's entry.
type roundClip struct {
	box   Rect
	radii Radii
	up    *roundClip
}

// with is a chain with one more rounded rectangle inside it, or the chain as it
// was when the rectangle's corners are all square.
func (c *roundClip) with(box Rect, radii Radii) *roundClip {
	if radii.IsZero() {
		return c
	}
	return &roundClip{box: box, radii: radii, up: c}
}

// opBounds is the rectangle an operation may mark, for the question of whether
// a rounded corner cuts it; ok is false for one that marks nothing a curve can
// cut.
func opBounds(op Op) (Rect, bool) {
	switch v := op.(type) {
	case FillRect:
		return v.Rect, true
	case TileImage:
		return v.Clip, true
	case FillGradient:
		return v.Clip, true
	case DrawImage:
		if v.Clip.Active {
			return v.Rect.Intersect(v.Clip.Rect), true
		}
		return v.Rect, true
	case DrawText:
		return textInkReserved(v), true
	case FillPath:
		b := v.Path.Bounds()
		if v.Clip.Active {
			b = b.Intersect(v.Clip.Rect)
		}
		return b, true
	case ClipPath:
		return v.Path.Bounds(), true
	case FilterGroup:
		return v.Extent(), true
	}
	return Rect{}, false
}

// roundOps clips every operation from index at onwards to each rounded
// rectangle of a chain: an operation clear of the corners is left alone, one
// wholly outside the curve in a corner is dropped, and runs of the rest are each
// held in one ClipPath. It returns how many ClipPaths it made.
func roundOps(ops []Op, at int, chain *roundClip) ([]Op, int) {
	made := 0
	for c := chain; c != nil; c = c.up {
		var n int
		ops, n = roundOpsOnce(ops, at, c.box, c.radii)
		made += n
	}
	return ops, made
}

// rounding is roundOps for the painter, which charges what it makes to the
// document's work budget as emit charges a mark: a group is an operation a
// backend has to draw, and a document controls how many there are — one for
// each paint step inside each rounded box around it. Past the budget, what was
// painted from at onwards is dropped, as emit drops the marks it is refused,
// and the budget has said so.
func (p *painter) rounding(at int, chain *roundClip) {
	if chain == nil || len(p.ops) <= at {
		return
	}
	ops, made := roundOps(p.ops, at, chain)
	p.ops = ops
	if made > 0 && !p.rec.chargeMark(int64(made)*costOp, "the marks past that point") {
		p.ops = p.ops[:at]
	}
}

func roundOpsOnce(ops []Op, at int, box Rect, radii Radii) ([]Op, int) {
	tail := append([]Op(nil), ops[at:]...)
	kept := ops[:at]
	var group []Op
	made := 0
	flush := func() {
		if len(group) > 0 {
			kept = append(kept, ClipPath{Path: roundedRect(box, radii), Ops: group})
			group = nil
			made++
		}
	}
	for _, op := range tail {
		r, marks := opBounds(op)
		switch {
		case !marks || r.Empty() || safeFrom(r, box, radii):
			// Including a Link, which marks nothing: it stays where it was,
			// outside any group. No painting step that is clipped here emits
			// one — a link's area is emitted beside the marks it is around,
			// through the rectangle clip alone — so this is the whole of the
			// rule and not a case that happens.
			flush()
			kept = append(kept, op)
		case outsideCorner(r, box, radii):
			// Nothing of it is inside the curve.
		default:
			group = append(group, op)
		}
	}
	flush()
	return kept, made
}
