package layout

import (
	"math"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// The shape of an outline: where it is, and the curve of its corners.
//
// CSS UI 4 §3: "By default, the outline is drawn starting just outside the
// border edge", and outline-offset moves it: "If the computed value of
// outline-offset is anything other than 0, then the outline is outset from the
// border edge by that amount. Negative values must cause the outline to shrink
// into the border box. Both the height and the width of the outside of the
// shape drawn by the outline should not become smaller than twice the computed
// value of the outline-width property", "independently in each dimension". So
// the outline's inner edge is the border box moved out by the offset on each
// axis, the offset on an axis never less than minus half the box's size there,
// and the outline is outline-width wide outside that.
//
// "To the extent that the outline follows the border edge, it should follow the
// border-radius curve". Moving a rounded edge outwards is what CSS Backgrounds 3
// §4.2 calls computing the "outset-adjusted border radius", which it gives for
// "the margin edge, box-shadow spread, or overflow-clip-margin" and which is
// the same question here: a corner's radius grows by the outset where it is
// larger than the outset or where the corner is most of its box, and grows by
// less where a small curve would otherwise become a large one, so that a box
// with nearly square corners keeps nearly square ones. Moving it inwards is
// §4.2's inner radii and an inset box-shadow's: the radius less the distance,
// and square where that leaves nothing. The outer edge of the outline is its
// inner edge moved out by the outline's width, with the width added to every
// radius that is round — the same relation §4.2 puts between a border's two
// curves — so an outline is a band of one width that follows its corners.

// outlineOffsetOf is a box's used outline-offset, and zero where it cannot be
// read. It is read in layout and kept on the fragment, beside the outline's
// width and for the same reason: a length in "ch" or "ex" is measured from the
// box's face, which the painter does not have. The cascade has already refused
// anything that is not a <length>.
func (l *layouter) outlineOffsetOf(b *Box) style.Unit {
	if b == nil {
		return 0
	}
	raw := ascii.TrimCSSSpace(b.Style.Get("outline-offset"))
	if raw == "" || raw == "0" {
		return 0
	}
	v, ok := l.lengthOf(b, "outline-offset", 0)
	if !ok {
		l.reportOnce("outline-offset:"+raw, Finding{
			Rule:     RuleUnsupportedValue,
			Source:   AtHTML(offsetOf(b)),
			Message:  "the outline-offset " + quoteValue(raw) + " has a length this engine does not resolve; the outline was drawn at the border edge",
			Path:     PathOf(b.Element),
			Property: "outline-offset",
		})
		return 0
	}
	return v
}

// outlineStyle is a box's outline-style as a border style. "auto" is drawn as
// solid, which css-ui-4 §3.3 allows in as many words ("User agents may treat
// auto as solid"), and which the suite's outline-auto-width-001 accepts as one
// of its two references.
func outlineStyle(b *Box) borderStyle {
	raw := b.Style.Get("outline-style")
	if ascii.EqualFold(ascii.TrimCSSSpace(raw), "auto") {
		return borderSolid
	}
	return parseBorderStyle(raw)
}

// outlineColour is a box's outline-color. css-ui-4 §3.4's "auto" computes to
// currentColor for every style but "auto", and for "auto" is the accent colour
// the user agent chooses, which this one chooses to be currentColor as well.
func (p *painter) outlineColour(b *Box) (style.RGBA, bool) {
	if ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("outline-color")), "auto") {
		return p.color(b, "color")
	}
	return p.color(b, "outline-color")
}

// outlineEdges is an outline's two edges for a piece of a box: its inner edge
// and radii, the border box moved out by the offset, and its outer edge and
// radii, the inner moved out by the width. See the note at the top of this
// file.
func outlineEdges(border Rect, radii Radii, offset, width style.Unit) (inner Rect, iR Radii, outer Rect, oR Radii) {
	ox, oy := offset, offset
	if half := border.W.Div(2); ox < 0 && ox < -half {
		ox = -half
	}
	if half := border.H.Div(2); oy < 0 && oy < -half {
		oy = -half
	}
	inner = border.Outset(Edges{Top: oy, Right: ox, Bottom: oy, Left: ox})
	iR = outsetRadii(radii, border, ox, oy)
	grow := Edges{Top: width, Right: width, Bottom: width, Left: width}
	outer = inner.Outset(grow)
	c := iR.corners()
	for k := range c {
		if c[k].X > 0 && c[k].Y > 0 {
			c[k].X, c[k].Y = c[k].X.Add(width), c[k].Y.Add(width)
		}
	}
	return inner, iR, outer, radiiOf(c)
}

// outsetRadii is a rounded rectangle's radii when its edge is moved out by ox
// and oy (in by their size when they are negative): CSS Backgrounds 3 §4.2's
// outset-adjusted border radius, and a shrinking radius floored at nothing.
func outsetRadii(radii Radii, box Rect, ox, oy style.Unit) Radii {
	c := radii.corners()
	for k := range c {
		if c[k].X <= 0 || c[k].Y <= 0 {
			c[k] = Corner{}
			continue
		}
		coverage := 2 * math.Min(c[k].X.Px()/box.W.Px(), c[k].Y.Px()/box.H.Px())
		x := adjustedRadius(coverage, c[k].X.Px(), ox.Px())
		y := adjustedRadius(coverage, c[k].Y.Px(), oy.Px())
		xu, _ := style.FromPx(x)
		yu, _ := style.FromPx(y)
		if xu <= 0 || yu <= 0 {
			c[k] = Corner{}
			continue
		}
		c[k] = Corner{X: xu, Y: yu}
	}
	return radiiOf(c)
}

// adjustedRadius is §4.2's "adjusted radius dimension given numbers coverage,
// radius, and outset", for an outset that grows the edge, and the radius less
// the inset, and nothing where that is negative, for one that shrinks it:
//
//	If radius is greater than [outset], or if coverage is greater than 1,
//	then return radius + outset.
//	Let ratio be radius / outset.
//	Return radius + outset * (1 - (1 - ratio)^3 * (1 - coverage^3)).
func adjustedRadius(coverage, radius, outset float64) float64 {
	switch {
	case outset <= 0:
		return math.Max(0, radius+outset)
	case radius > outset || coverage > 1:
		return radius + outset
	}
	ratio := radius / outset
	return radius + outset*(1-math.Pow(1-ratio, 3)*(1-math.Pow(coverage, 3)))
}
