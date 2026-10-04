package shape

import (
	"errors"
	"fmt"
	"image/color"
	"math"

	"github.com/mgilbir/forme/font"
)

// A glyph's colour, painted, for a caller that draws the glyphs itself.
//
// GlyphOutline hands out a glyph's outline, which for a colour emoji font is
// its monochrome fallback or nothing at all. The colour is in other tables:
// COLR, whose glyphs are layers of filled outlines (version 0) or a graph of
// paints — fills, gradients, transforms, clips and composites (version 1) —
// coloured from CPAL's palettes; and CBDT and sbix, whose glyphs are PNG
// images at a few sizes. forme reads all three already, to measure a colour
// glyph's ink (colrink.go, bitmapink.go, sbixink.go). PaintGlyph hands out
// what that measuring walks, so the glyph painted is the glyph measured.
//
// # The calls
//
// They are HarfBuzz's, hb_font_paint_glyph's at the release the oracle is
// pinned to, call for call, and shape/paint_test.go holds them to it over
// every glyph of the colour faces in testdata/harfbuzz (see paint.py): a
// Painter is an hb_paint_funcs_t, and a renderer written against either draws
// from the other. The one difference is the transforms that are the identity.
// HarfBuzz pushes the font's own scale around a glyph, and its inverse around
// each outline a fill is clipped to; painting here is in font units, where
// both are the identity, and they are left out.
//
// Which table a glyph is painted from is HarfBuzz's order too: COLR, then CBDT,
// then sbix, and a glyph none of them paints is painted as its outline, filled
// with the foreground colour. GlyphColour says which, ahead of painting.
//
// # What it costs, and what is refused
//
// HarfBuzz bounds a COLRv1 glyph's painting at 64 paints deep and 16,384 paint
// edges, and stops where it reaches either, having painted part of the glyph.
// Painting here has those bounds and a budget of its own per glyph, which also
// counts each gradient's colour stops; and a glyph that would reach any of them
// is refused whole, with ErrPaintLimit, before anything is handed to the
// Painter, rather than painted in part. The budget is the glyph's own, not the
// face's: a caller that paints the same emoji ten thousand times is not the
// reason a later one is measured short (colrInkWork).

// ErrPaintLimit is what PaintGlyph reports for a colour glyph whose painting
// runs past the bounds on it; nothing of the glyph has been painted.
var ErrPaintLimit = errors.New("shape: painting the glyph runs past its bounds")

// Color is a colour, not premultiplied: red, green, blue and alpha, each from 0
// to 255. It is an image/color Color.
type Color struct{ R, G, B, A uint8 }

// RGBA is the colour premultiplied, as image/color states it.
func (c Color) RGBA() (r, g, b, a uint32) {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}.RGBA()
}

// Transform is an affine transform, from what is painted inside it to what
// is outside: a point (x, y) is painted at
//
//	(XX*x + XY*y + X0, YX*x + YY*y + Y0)
type Transform struct{ XX, YX, XY, YY, X0, Y0 float64 }

// Rect is a box in font units, y increasing upwards.
type Rect struct{ XMin, YMin, XMax, YMax float64 }

// Extend is what a gradient paints past the ends of its colour line.
type Extend uint8

// The extend modes, in COLR's numbering.
const (
	// ExtendPad paints the colour of the nearer end.
	ExtendPad Extend = iota
	// ExtendRepeat starts the colour line again.
	ExtendRepeat
	// ExtendReflect runs the colour line back and forth.
	ExtendReflect
)

// ColorStop is one colour of a gradient's colour line, at an offset along it.
// Foreground says that the colour is the foreground's, with the stop's alpha
// applied; see PaintOptions.Foreground.
type ColorStop struct {
	Offset     float64
	Color      Color
	Foreground bool
}

// ColorLine is a gradient's colours. The stops are as the font states them:
// not sorted, and with offsets that may lie outside 0 to 1.
type ColorLine struct {
	Stops  []ColorStop
	Extend Extend
}

// LinearGradient is COLR's linear gradient: the colour line runs from P0
// towards P1, and P2 turns it, the gradient's colours standing along lines
// parallel to the one from P0 to P2.
type LinearGradient struct {
	Line       ColorLine
	P0, P1, P2 Point
}

// RadialGradient is COLR's radial gradient, between a circle about C0 of
// radius R0 and one about C1 of radius R1.
type RadialGradient struct {
	Line ColorLine
	C0   Point
	R0   float64
	C1   Point
	R1   float64
}

// SweepGradient is COLR's sweep gradient, around Center from StartAngle to
// EndAngle: in radians, counter-clockwise from the positive x axis, the bias
// of 180 degrees the font stores them with taken off.
type SweepGradient struct {
	Line                 ColorLine
	Center               Point
	StartAngle, EndAngle float64
}

// CompositeMode is how a group is combined with what is beneath it: COLR's
// modes, in its numbering, which are those of W3C Compositing and Blending.
type CompositeMode uint8

// The composite modes.
const (
	CompositeClear CompositeMode = iota
	CompositeSrc
	CompositeDest
	CompositeSrcOver
	CompositeDestOver
	CompositeSrcIn
	CompositeDestIn
	CompositeSrcOut
	CompositeDestOut
	CompositeSrcAtop
	CompositeDestAtop
	CompositeXor
	CompositePlus
	CompositeScreen
	CompositeOverlay
	CompositeDarken
	CompositeLighten
	CompositeColorDodge
	CompositeColorBurn
	CompositeHardLight
	CompositeSoftLight
	CompositeDifference
	CompositeExclusion
	CompositeMultiply
	CompositeHSLHue
	CompositeHSLSaturation
	CompositeHSLColor
	CompositeHSLLuminosity
)

// ImageFormat is what an Image's data is.
type ImageFormat uint8

// The image formats. CBDT and sbix glyphs are painted from PNG images only, as
// HarfBuzz paints them.
const (
	ImagePNG ImageFormat = iota + 1
)

// Image is a bitmap glyph's image, for its strike: the image file, its width
// and height in pixels, and Box, where it is drawn, in font units — the image
// is scaled to fill it. Data is the font's own bytes, and is not to be
// changed.
type Image struct {
	Format        ImageFormat
	Data          []byte
	Width, Height int
	Box           Rect
}

// Painter receives a glyph's painting from PaintGlyph.
//
// The pushes and pops nest: each PushTransform is undone by a PopTransform,
// each PushClipGlyph and PushClipRect by a PopClip, and each PushGroup by a
// PopGroup. A transform applies to everything painted inside it, after the
// transforms around it; a clip is applied in addition to the clips around it;
// a group paints into a surface of its own, which PopGroup combines with what
// is beneath in the mode it is given. Solid, the gradients and Image paint
// everywhere inside the current clips.
type Painter interface {
	PushTransform(t Transform)
	PopTransform()
	// PushClipGlyph clips to a glyph's outline, which GlyphOutline draws.
	PushClipGlyph(gid int)
	PushClipRect(r Rect)
	PopClip()
	PushGroup()
	PopGroup(mode CompositeMode)
	// Solid paints one colour. foreground says it is the foreground's, with
	// an alpha from the font applied; see PaintOptions.Foreground.
	Solid(c Color, foreground bool)
	LinearGradient(g LinearGradient)
	RadialGradient(g RadialGradient)
	SweepGradient(g SweepGradient)
	Image(img Image)
}

// PaintOptions are what a glyph is painted with.
type PaintOptions struct {
	// Palette is which of the font's CPAL palettes colours a COLR glyph. One
	// the font does not have is its first, palette 0.
	Palette int
	// Foreground is the colour the font names as the text's own: what a COLR
	// paint in colour index 0xFFFF is filled with, its alpha multiplied by the
	// paint's, and what a glyph with no colour is filled with. The zero Color
	// is taken as opaque black. A caller resolving the foreground itself,
	// from a CSS currentColor say, passes an opaque one, so that the alpha a
	// fill marked as the foreground carries is the font's alone.
	Foreground Color
	// PPEM is the size the glyph is drawn at, in pixels per em, which picks
	// a CBDT or sbix glyph's strike: the smallest at least that large, or
	// failing any the largest. Zero picks the largest.
	PPEM int
	// PaletteOverrides replaces entries of the palette, by index, as CSS
	// font-palette's override-colors does: a COLR paint, a colour stop or a
	// COLRv0 layer naming an index here takes its colour from here rather than
	// from the font, its alpha multiplied by the paint's as a palette entry's
	// is. An index past the end of the palette may be given, and is used. The
	// foreground's index, 0xFFFF, is not a palette entry and is not
	// overridden: Foreground is what colours it. It is HarfBuzz's
	// custom_palette_color.
	PaletteOverrides map[int]Color
}

// GlyphColour is which of its representations PaintGlyph paints a glyph from.
type GlyphColour uint8

// The representations, in the order PaintGlyph asks for them.
const (
	// ColourNone is a glyph with no colour: it is painted as its outline,
	// filled with the foreground, or, in a face with no outlines
	// (BitmapOnly), not at all.
	ColourNone GlyphColour = iota
	// ColourPaint is a COLRv1 glyph, a graph of paints.
	ColourPaint
	// ColourLayers is a COLRv0 glyph, layers of outlines each filled with
	// one colour.
	ColourLayers
	// ColourBitmap is a CBDT or sbix glyph, an image.
	ColourBitmap
)

// GlyphColour says which representation PaintGlyph paints a glyph from, at a
// size in pixels per em (see PaintOptions.PPEM), so that a caller drawing
// only some of them knows which glyphs to draw otherwise. A glyph the face
// does not have, and every glyph of a standard face, is ColourNone.
func (f *Face) GlyphColour(gid, ppem int) GlyphColour {
	if f.prog == nil || gid < 0 || gid >= f.prog.NumGlyphs {
		return ColourNone
	}
	if t := f.colrTable(); t != nil {
		if t.version >= 1 {
			if _, ok := t.basePaint(gid); ok {
				return ColourPaint
			}
		}
		if _, _, ok := t.baseGlyphRecord(gid); ok {
			return ColourLayers
		}
	}
	if _, ok := f.bitmapImage(gid, ppem); ok {
		return ColourBitmap
	}
	return ColourNone
}

// PaintGlyph paints a glyph through p: its COLR paints, or its CBDT or sbix
// image, or, for a glyph with none, its outline in the foreground; see
// GlyphColour. A face whose glyphs are only bitmaps (BitmapOnly) paints nothing
// for a glyph with no image. Coordinates are in font units, y increasing
// upwards.
//
// It is an error for a glyph the face does not have, and for a standard face,
// which has no glyphs to paint (ErrNoOutline). A COLR glyph whose painting
// runs past its bounds is refused with ErrPaintLimit before p is called at
// all. It may be called on a face from several goroutines at once.
func (f *Face) PaintGlyph(gid int, opts PaintOptions, p Painter) error {
	if f.std != nil || f.prog == nil {
		return fmt.Errorf("%w: a standard face has no font program to paint from", ErrNoOutline)
	}
	if gid < 0 || gid >= f.prog.NumGlyphs {
		return fmt.Errorf("shape: glyph %d is not one of the face's %d glyphs", gid, f.prog.NumGlyphs)
	}
	if opts.Foreground == (Color{}) {
		opts.Foreground = Color{A: 255}
	}
	fg := opts.Foreground
	if t := f.colrTable(); t != nil {
		if painted, err := f.paintCOLR(t, gid, opts, p, paintWork); painted || err != nil {
			return err
		}
	}
	if img, ok := f.bitmapImage(gid, opts.PPEM); ok {
		p.Image(img)
		return nil
	}
	if f.bitmapOnly {
		// A face with no outlines paints nothing for a glyph its strike has
		// no image of, where HarfBuzz fills an empty outline: the same
		// nothing, said without the calls.
		return nil
	}
	p.PushClipGlyph(gid)
	p.Solid(fg, true)
	p.PopClip()
	return nil
}

// colrTable is the face's COLR table, read, and nil for a face with none.
func (f *Face) colrTable() *colrTable {
	if f.colr == nil {
		return nil
	}
	return f.colr.table()
}

// paintWork is the budget one glyph's painting has: the paints visited, the
// layers, and each gradient's colour stops. HarfBuzz's edges bound the paints
// at 16,384; this is what keeps 16,384 gradients of 65,535 stops each from
// being handed out.
const paintWork = maxFontWork

// paintCOLR paints a COLR glyph within a budget of work, and reports whether
// the table has it. It is walked twice: once counting, which is where a glyph
// that runs past its bounds is refused, and once painting, which then cannot.
func (f *Face) paintCOLR(t *colrTable, gid int, opts PaintOptions, p Painter, work int) (bool, error) {
	c := f.colr
	counter := &paintCounter{t: t, budget: font.NewBudget(work)}
	painted, refused := c.paintGlyphWith(gid, counter, true, counter.budget)
	if !painted {
		return false, nil
	}
	if refused || counter.budget.Err() != nil {
		return true, fmt.Errorf("%w: glyph %d", ErrPaintLimit, gid)
	}
	a := &paintAdapter{t: t, p: p, palette: c.palette(opts.Palette), fg: opts.Foreground, overrides: opts.PaletteOverrides}
	c.paintGlyphWith(gid, a, true, font.NewBudget(work))
	return true, nil
}

// paintCounter is the painter of the counting walk: it paints nothing, and
// charges each gradient's colour stops to the budget the walk is charged to.
type paintCounter struct {
	t      *colrTable
	budget *font.Budget
}

func (*paintCounter) pushTransform(xform32) {}
func (*paintCounter) popTransform()         {}
func (*paintCounter) pushClipGlyph(int)     {}
func (*paintCounter) pushClipRect(box32)    {}
func (*paintCounter) popClip()              {}
func (*paintCounter) pushGroup()            {}
func (*paintCounter) popGroup(int)          {}

func (c *paintCounter) paint(fill paintFill) {
	if fill.at < 0 {
		return
	}
	// Only a gradient has a colour line. A solid fill's bytes past its
	// format are a colour index and an alpha, which read as an offset name
	// a colour line that is not there.
	if f := c.t.u8(fill.at); f >= 4 && f <= 9 {
		if _, n, _ := c.t.colorLine(fill.at); n > 0 {
			c.budget.Charge(n, "painting a colour glyph")
		}
	}
}

// paintAdapter is the painter of the painting walk: it hands each call to a
// Painter, its fills resolved to colours, less the transforms that are the
// identity.
type paintAdapter struct {
	t       *colrTable
	p       Painter
	palette cpalPalette
	fg      Color
	// overrides are the caller's colours for palette entries, which win over
	// the palette's. See PaintOptions.PaletteOverrides.
	overrides map[int]Color
	// pushed records, for each transform pushed and not yet popped, whether
	// it was handed on.
	pushed []bool
}

func (a *paintAdapter) pushTransform(t xform32) {
	identity := t == identity32
	a.pushed = append(a.pushed, !identity)
	if !identity {
		a.p.PushTransform(Transform{
			XX: float64(t.xx), YX: float64(t.yx), XY: float64(t.xy), YY: float64(t.yy),
			X0: float64(t.x0), Y0: float64(t.y0),
		})
	}
}

func (a *paintAdapter) popTransform() {
	if n := len(a.pushed); n > 0 {
		handed := a.pushed[n-1]
		a.pushed = a.pushed[:n-1]
		if handed {
			a.p.PopTransform()
		}
	}
}

func (a *paintAdapter) pushClipGlyph(gid int) { a.p.PushClipGlyph(gid) }

func (a *paintAdapter) pushClipRect(r box32) {
	a.p.PushClipRect(Rect{XMin: float64(r.xMin), YMin: float64(r.yMin), XMax: float64(r.xMax), YMax: float64(r.yMax)})
}

func (a *paintAdapter) popClip()          { a.p.PopClip() }
func (a *paintAdapter) pushGroup()        { a.p.PushGroup() }
func (a *paintAdapter) popGroup(mode int) { a.p.PopGroup(CompositeMode(mode)) }

// paint resolves a fill: each format's paint_glyph, and a COLRv0 layer's.
func (a *paintAdapter) paint(fill paintFill) {
	t, at := a.t, fill.at
	if at < 0 {
		c, fg := a.colour(fill.index, 1)
		a.p.Solid(c, fg)
		return
	}
	fword := func(off, v int) float64 {
		return float64(float32(float32(signed16(t.u16(at+off))) + t.delta(at, v)))
	}
	ufword := func(off, v int) float64 {
		return float64(float32(float32(t.u16(at+off)) + t.delta(at, v)))
	}
	switch t.u8(at) {
	case 2, 3: // PaintSolid, PaintVarSolid
		c, fg := a.colour(t.u16(at+1), t.f2dot14(at+3, t.delta(at, 0)))
		a.p.Solid(c, fg)
	case 4, 5: // PaintLinearGradient
		a.p.LinearGradient(LinearGradient{
			Line: a.line(at),
			P0:   Point{fword(4, 0), fword(6, 1)},
			P1:   Point{fword(8, 2), fword(10, 3)},
			P2:   Point{fword(12, 4), fword(14, 5)},
		})
	case 6, 7: // PaintRadialGradient
		a.p.RadialGradient(RadialGradient{
			Line: a.line(at),
			C0:   Point{fword(4, 0), fword(6, 1)},
			R0:   ufword(8, 2),
			C1:   Point{fword(10, 3), fword(12, 4)},
			R1:   ufword(14, 5),
		})
	case 8, 9: // PaintSweepGradient
		angle := func(off, v int) float64 {
			return float64(float32(float32(t.f2dot14(at+off, t.delta(at, v))+1) * hbPi))
		}
		a.p.SweepGradient(SweepGradient{
			Line:       a.line(at),
			Center:     Point{fword(4, 0), fword(6, 1)},
			StartAngle: angle(8, 2),
			EndAngle:   angle(10, 3),
		})
	}
}

// line is a gradient's colour line, each stop's colour resolved.
func (a *paintAdapter) line(at int) ColorLine {
	t := a.t
	cl, n, size := t.colorLine(at)
	var line ColorLine
	switch t.u8(cl) {
	case 1:
		line.Extend = ExtendRepeat
	case 2:
		line.Extend = ExtendReflect
	}
	if n == 0 {
		return line
	}
	line.Stops = make([]ColorStop, n)
	for i := range line.Stops {
		s := cl + 3 + size*i
		base := uint32(noVariation)
		if size == varColorStopSize {
			base = t.u32(s + 6)
		}
		c, fg := a.colour(t.u16(s+2), t.f2dot14(s+4, t.varDelta(base, 1)))
		line.Stops[i] = ColorStop{
			Offset:     float64(t.f2dot14(s, t.varDelta(base, 0))),
			Color:      c,
			Foreground: fg,
		}
	}
	return line
}

// colour is hb_paint_context_t::get_color: the caller's override of a palette
// entry, or the entry, or the foreground for 0xFFFF, its alpha multiplied by
// the paint's, which is held between zero and one.
func (a *paintAdapter) colour(index int, alpha float32) (Color, bool) {
	c, fg := a.fg, true
	if index != foregroundIndex {
		var overridden bool
		if c, overridden = a.overrides[index]; !overridden {
			c = a.palette.colour(index)
		}
		fg = false
	}
	alpha = min(max(alpha, 0), 1)
	c.A = uint8(math.Round(float64(float32(float32(c.A) * alpha))))
	return c, fg
}

// foregroundIndex is the colour index that names the foreground.
const foregroundIndex = 0xFFFF

// The sizes of a ColorStop and a VarColorStop, which carries its VarIdxBase.
const (
	colorStopSize    = 6
	varColorStopSize = 10
)

// colorLine is a gradient's ColorLine, at its offset in COLR, with as many of
// its stops as the table holds and the size of each: a variable gradient's are
// VarColorStops. A gradient whose offset is null has HarfBuzz's Null line: no
// stops, padded.
func (t *colrTable) colorLine(at int) (cl, n, size int) {
	cl = t.offset24(at, 1)
	if cl < 0 {
		return -1, 0, colorStopSize
	}
	size = colorStopSize
	if t.u8(at)%2 == 1 {
		size = varColorStopSize
	}
	n = t.u16(cl + 1)
	if room := (len(t.b) - cl - 3) / size; n > room {
		n = max(room, 0)
	}
	return cl, n, size
}

// cpalPalette is one of CPAL's palettes: get_palette_colors, the run of
// colour records a palette's index names, as many as each palette has and no
// further than the records go. A colour past its end is HarfBuzz's Null
// colour, transparent black.
type cpalPalette struct {
	b     []byte
	at, n int
}

// colour is the palette's colour at an index.
func (p cpalPalette) colour(i int) Color {
	if i < 0 || i >= p.n {
		return Color{}
	}
	r := p.at + 4*i
	// A record is blue, green, red and alpha.
	return Color{R: p.b[r+2], G: p.b[r+1], B: p.b[r], A: p.b[r+3]}
}

// palette is the face's palette at an index, the first for one the font does
// not have (HarfBuzz #5116), and none for a CPAL table HarfBuzz's sanitizer
// refuses or a face with none.
func (c *colrInk) palette(index int) cpalPalette {
	b := c.cpal
	if len(b) < 12 {
		return cpalPalette{}
	}
	version, numColors, numPalettes, numRecords := font.Be16(b, 0), font.Be16(b, 2), font.Be16(b, 4), font.Be16(b, 6)
	records := int(font.Be32(b, 8))
	indices := 12
	if records > len(b) || len(b)-records < 4*numRecords || len(b)-indices < 2*numPalettes {
		return cpalPalette{}
	}
	if version >= 1 {
		// The offsets of the palette types and labels, and of the entry
		// labels, and the arrays they name where they are not null.
		v1 := indices + 2*numPalettes
		if v1+12 > len(b) {
			return cpalPalette{}
		}
		for i, n := range []int{4 * numPalettes, 2 * numPalettes, 2 * numColors} {
			if off := int(font.Be32(b, v1+4*i)); off != 0 && (off > len(b) || len(b)-off < n) {
				return cpalPalette{}
			}
		}
	}
	if index < 0 || index >= numPalettes {
		index = 0
	}
	if index >= numPalettes {
		return cpalPalette{}
	}
	start := font.Be16(b, indices+2*index)
	if start > numRecords {
		return cpalPalette{}
	}
	return cpalPalette{b: b, at: records + 4*start, n: min(numColors, numRecords-start)}
}

// bitmapImage is a glyph's image as CBDT or sbix paints it, CBDT first, in the
// strike for a size: paint_glyph for each, which paints a glyph whose box,
// whose image's size in pixels and whose image it can read. The box is the
// glyph's extents at that size, which HarfBuzz asks of sbix first.
func (f *Face) bitmapImage(gid, ppem int) (Image, bool) {
	if f.bitmap == nil && f.sbix == nil {
		return Image{}, false
	}
	var data []byte
	var width, height int
	ok := false
	if f.bitmap != nil {
		data, width, height, ok = f.bitmap.png(gid, ppem)
	}
	if !ok && f.sbix != nil {
		strike, sppem := f.sbix.strikeFor(ppem)
		_, _, width, height, data, ok = f.sbix.pngIn(gid, strike, sppem)
	}
	if !ok {
		return Image{}, false
	}
	e, ok := f.glyphExtentsAt(gid, ppem)
	if !ok {
		return Image{}, false
	}
	return Image{
		Format: ImagePNG,
		Data:   data,
		Width:  width,
		Height: height,
		Box: Rect{
			XMin: float64(e.xBearing), YMin: float64(e.yBearing + e.height),
			XMax: float64(e.xBearing + e.width), YMax: float64(e.yBearing),
		},
	}, true
}
