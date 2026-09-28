package layout

import (
	"math"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// DrawGlyphs draws glyphs of a face by their indices, each where it is placed:
// what a formula draws where the glyph it needs stands for no character.
//
// A stretched operator is a size variant of its character's glyph, or an
// assembly of pieces — the top, the middle and the bottom of a brace and the
// extenders between them — and none of those glyphs is in the face's
// character map: no text shapes to them, so a DrawText cannot say them.
// MathML Core §3.2.4.3 and §3.3.3 draw them all the same, and this is how the
// display list says so. It is the one operation MathML adds, and it was added
// only because exactness needs it: everything else a formula draws is a
// DrawText or a FillRect.
//
// The glyphs are placed as ShapedGlyphs places a run's: the pen starts at At,
// on the baseline; each glyph is drawn at the pen displaced by its XOffset and
// YOffset — in thousandths of an em, y upward, as shaping states them — and
// the pen then moves by its XAdvance. A size variant is one glyph advancing
// by its width; an assembly's pieces advance nothing and are each placed by
// their offsets. So a backend that draws a DrawText's shaped glyphs draws
// these with the same code, handed these glyphs instead of shaping a string.
//
// Text is what the glyphs stand for: the operator's character, the "√" of a
// radical. It is the document's text, and what a reader copying the page
// expects back — once, however many pieces draw it — and it is not to be
// shaped: the glyphs are the drawing. It is empty for glyphs that stand for no
// text of the document: the drop shadow of a formula's glyphs is glyphs, and
// is not the formula a second time.
//
// A backend that meets this and cannot draw it has met a kind of operation it
// has no case for, and says so, as Op asks of every new kind.
type DrawGlyphs struct {
	At     Point
	Text   string
	Glyphs []shape.Glyph
	Face   *shape.Face
	Size   style.Unit
	Color  style.RGBA
	// Clip is §11.1's clipping, when something cuts the glyphs, as a
	// DrawText's is: set only when the clip really does cut them.
	Clip Clip
}

func (DrawGlyphs) isOp() {}

// glyphsInk is the rectangle a DrawGlyphs marks: the union of its glyphs' ink
// boxes, each where it is drawn.
//
// The glyphs are named, so their ink is known exactly, and it is what every
// question about the operation is asked of — whether a clip cuts it, whether
// a rounded corner does, what a group's marks are. It is not a line's box, as
// a run of text's reserved ink is, since no line was made for these. A glyph
// whose ink the face cannot state is taken to fill its em square on the
// baseline from its pen, which errs towards the glyph being there. Each edge
// is rounded outwards to the unit.
func glyphsInk(v DrawGlyphs) Rect {
	if v.Face == nil || v.Face.UnitsPerEm() <= 0 {
		return Rect{}
	}
	size := v.Size.Px()
	upem := float64(v.Face.UnitsPerEm())
	left, top := math.Inf(1), math.Inf(1)
	right, bottom := math.Inf(-1), math.Inf(-1)
	pen := 0.0
	for _, g := range v.Glyphs {
		x := (pen + g.XOffset) * size / 1000
		y := -g.YOffset * size / 1000
		xb, yb, w, h, ok := v.Face.GlyphExtents(g.GID)
		if !ok {
			em := int(upem)
			xb, yb, w, h = 0, em, em, -em
		}
		pen += g.XAdvance
		if w == 0 && h == 0 {
			continue // a glyph with no ink
		}
		left = math.Min(left, x+float64(xb)*size/upem)
		right = math.Max(right, x+float64(xb+w)*size/upem)
		top = math.Min(top, y-float64(yb)*size/upem)
		bottom = math.Max(bottom, y-float64(yb+h)*size/upem)
	}
	if math.IsInf(left, 0) {
		return Rect{}
	}
	// Outwards to the unit, a sixty-fourth of a pixel; FromPx saturates what
	// does not fit.
	lo := func(px float64) style.Unit { u, _ := style.FromPx(math.Floor(px*64) / 64); return u }
	hi := func(px float64) style.Unit { u, _ := style.FromPx(math.Ceil(px*64) / 64); return u }
	x0, y0 := v.At.X.Add(lo(left)), v.At.Y.Add(lo(top))
	return Rect{X: x0, Y: y0, W: v.At.X.Add(hi(right)).Sub(x0), H: v.At.Y.Add(hi(bottom)).Sub(y0)}
}
