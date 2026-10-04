package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// The ink of an sbix glyph: the size of the PNG image its strike holds and
// where the strike places it, which HarfBuzz asks before anything else about a
// glyph's extents — before CBDT (bitmapink.go), before COLR (colrink.go),
// before the outline.
//
// sbix is Apple's bitmap table. It holds strikes, each a set of images for one
// size in pixels per em, and each image is a whole file — PNG, JPEG or TIFF —
// with the offset from its bottom left corner to the glyph's origin in front of
// it. An image may also be a 'dupe', which names another glyph whose image it
// is. HarfBuzz reads PNG only: it takes the image's width and height from its
// IHDR chunk and answers a box that size, standing at the offset, scaled from
// pixels to font units.
//
// What is read is sbix::accelerator_t::get_extents at the release the oracle is
// pinned to, number for number:
//
//   - The strike is the one choose_strike picks when asked at no size, as
//     measuring asks: the largest ppem, the first of equals, starting from the
//     first strike even where that one is null.
//   - A glyph's record is found as get_glyph_blob finds it, with a 'dupe'
//     followed to the glyph it names eight times at most, and a record that
//     does not hold more than its eight-byte header is no image.
//   - An image that is not PNG has no box, and neither has a PNG stating a
//     side of 65,536 pixels or more; either is measured by what HarfBuzz asks
//     next. A PNG too short to hold an IHDR is read as HarfBuzz reads a blob
//     shorter than the struct it is cast to: as zeros — a box of nothing,
//     standing at the offset.
//   - The scaling is in single precision, rounding half up, as HarfBuzz's
//     roundf does: a half unit below zero rounds towards it, and a count of
//     units past 2^23, where single precision counts in whole numbers, rounds
//     its half to even first.
//   - The table is refused whole where HarfBuzz's sanitizer refuses it: a
//     strike, or its glyph offsets, not inside the table, or more checking of
//     them than the sanitizer allows a table that size.
//
// # Painting
//
// Face.PaintGlyph hands the image itself to a caller that draws (paint.go), as
// it hands out a CBDT one, from the strike choose_strike picks for the size
// the glyph is drawn at, which strikeFor is. Measuring still asks at no size.

// sbixInk is what a face with an sbix table keeps to measure its bitmap
// glyphs: the table and the strike HarfBuzz reads, chosen once.
type sbixInk struct {
	table     []byte
	numGlyphs int
	upem      int
	// strike is the offset of the chosen strike in the table and ppem its
	// size; a ppem of zero is a null strike, which answers for no glyph.
	strike, ppem int
}

// The layout of what is read: a glyph record's header is eight bytes, its
// offsets and its type, and a PNG's width and height are sixteen and twenty
// bytes into it, past the signature and the IHDR chunk's length and type.
const (
	sbixGlyphHeader = 8
	pngHeaderSize   = 29
	// sbixDupeRetries is how many times a duplicate is followed.
	sbixDupeRetries = 8
)

// Sanitizer bounds, as HarfBuzz states them: the checking a table may cost is
// 64 units a byte of it, and never less than 16,384.
const (
	sanitizeOpsFactor = 64
	sanitizeOpsMin    = 16384
	sanitizeOpsMax    = 0x3FFFFFFF
)

// newSbixInk reads an sbix table as HarfBuzz's sanitizer admits it and chooses
// the strike, or returns nil where there is no table or HarfBuzz would refuse
// it.
func newSbixInk(tables map[string][]byte, numGlyphs, upem int) *sbixInk {
	t := tables["sbix"]
	if !sbixSane(t, numGlyphs) {
		return nil
	}
	s := &sbixInk{table: t, numGlyphs: numGlyphs, upem: upem}
	s.strike, s.ppem = s.strikeFor(0)
	return s
}

// strikeFor is choose_strike: the strike for a size in pixels per em, and its
// own size, which is zero for none and for a null strike. Asked at no size,
// which it takes as 2^30, it is the largest; asked at one, the smallest at
// least that large, or failing any the largest.
func (s *sbixInk) strikeFor(requested int) (strike, ppem int) {
	t := s.table
	count := int(font.Be32(t, 4))
	if count == 0 {
		return 0, 0
	}
	if requested <= 0 {
		requested = 1 << 30
	}
	ppemOf := func(i int) int {
		off := int(font.Be32(t, 8+4*i))
		if off == 0 {
			return 0 // the null strike, whose fields read as zero
		}
		return font.Be16(t, off)
	}
	best, bestPPEM := 0, ppemOf(0)
	for i := 1; i < count; i++ {
		ppem := ppemOf(i)
		if requested <= ppem && ppem < bestPPEM || requested > bestPPEM && ppem > bestPPEM {
			best, bestPPEM = i, ppem
		}
	}
	return int(font.Be32(t, 8+4*best)), bestPPEM
}

// sbixSane is sbix::sanitize: a header, a version of at least one, and every
// strike's own header and glyph offsets inside the table, all within the
// checking HarfBuzz's sanitizer allows a table of this length. Each range
// checked costs its length in bytes, and the table is refused once the total
// reaches the allowance.
func sbixSane(t []byte, numGlyphs int) bool {
	if len(t) < 8 || font.Be16(t, 0) < 1 {
		return false
	}
	ops := int64(max(min(sanitizeOpsFactor*int64(len(t)), sanitizeOpsMax), sanitizeOpsMin))
	charge := func(n int64) bool {
		ops -= n
		return ops > 0
	}
	count := int64(font.Be32(t, 4))
	if 8+4*count > int64(len(t)) || !charge(4*count) {
		return false
	}
	offsets := 4 * (int64(numGlyphs) + 1)
	for i := int64(0); i < count; i++ {
		off := int64(font.Be32(t, int(8+4*i)))
		if off == 0 {
			continue
		}
		// The strike's ppem and resolution, and then its numGlyphs+1 offsets.
		if off+4 > int64(len(t)) || off+4+offsets > int64(len(t)) || !charge(offsets) {
			return false
		}
	}
	return true
}

// extents is a glyph's sbix ink in font units, and false where the strike
// holds no PNG HarfBuzz reads a box from.
func (s *sbixInk) extents(gid int) (extents, bool) {
	return s.extentsIn(gid, s.strike, s.ppem)
}

// extentsIn is extents in a strike strikeFor chose, of the size it gave.
func (s *sbixInk) extentsIn(gid, strike, ppem int) (extents, bool) {
	x, y, width, height, _, ok := s.pngIn(gid, strike, ppem)
	if !ok {
		return extents{}, false
	}
	scale := float32(s.upem) / float32(ppem)
	conv := func(v int) int {
		r := float32(float32(v) * scale)
		return int(clampToInt32(math.Floor(float64(float32(r + 0.5)))))
	}
	return scaledExtents(extents{
		xBearing: conv(x),
		yBearing: conv(height + y),
		width:    conv(width),
		height:   conv(-height),
	}), true
}

// scaledExtents is hb_font_t::scale_glyph_extents at a font's own scale — the
// scale this package measures at, one unit a unit. It changes nothing but
// where single precision cannot hold the numbers: each edge goes through a
// float, and past 2^24 units a float holds only every other whole number, so
// an edge there moves to one it can hold — the far edges outwards, by ceil,
// measured from the near ones moved inwards, by floor, and the difference
// taken in single precision too.
func scaledExtents(e extents) extents {
	floor := func(v int) float32 { return float32(math.Floor(float64(float32(v)))) }
	ceil := func(v int) float32 { return float32(math.Ceil(float64(float32(v)))) }
	x1, y1 := floor(e.xBearing), floor(e.yBearing)
	x2, y2 := ceil(e.xBearing+e.width), ceil(e.yBearing+e.height)
	return extents{
		xBearing: int(x1),
		yBearing: int(y1),
		width:    int(float32(x2 - x1)),
		height:   int(float32(y2 - y1)),
	}
}

// pngIn is get_png_extents unscaled, and the image: a glyph's PNG in a
// strike, its offsets, and the width and height its IHDR states, in pixels.
// It is false where the strike is null or holds no PNG for the glyph, and for
// a PNG stating a side of 65,536 pixels or more.
func (s *sbixInk) pngIn(gid, strike, ppem int) (x, y, width, height int, data []byte, ok bool) {
	if ppem == 0 {
		return 0, 0, 0, 0, nil, false
	}
	x, y, data, ok = s.imageIn(gid, strike)
	if !ok {
		return 0, 0, 0, 0, nil, false
	}
	var w, h uint32
	if len(data) >= pngHeaderSize {
		w, h = uint32(font.Be32(data, 16)), uint32(font.Be32(data, 20))
	}
	if w >= 65536 || h >= 65536 {
		return 0, 0, 0, 0, nil, false
	}
	return x, y, int(w), int(h), data, true
}

// imageIn is SBIXStrike::get_glyph_blob for a PNG: the offsets in front of a
// glyph's image in a strike and the image, following duplicates.
func (s *sbixInk) imageIn(gid, strike int) (x, y int, data []byte, ok bool) {
	t := s.table
	// How much of the table lies past the strike's start, which every offset
	// in it is measured from.
	room := int64(len(t) - strike)
	for retries := sbixDupeRetries; ; retries-- {
		if gid < 0 || gid >= s.numGlyphs {
			return 0, 0, nil, false
		}
		at := strike + 4 + 4*gid
		lo, hi := int64(font.Be32(t, at)), int64(font.Be32(t, at+4))
		if hi <= lo || hi-lo <= sbixGlyphHeader || hi > room {
			return 0, 0, nil, false
		}
		rec := t[int64(strike)+lo : int64(strike)+hi]
		data = rec[sbixGlyphHeader:]
		switch string(rec[4:8]) {
		case "dupe":
			if len(data) < 2 || retries == 0 {
				return 0, 0, nil, false
			}
			gid = font.Be16(data, 0)
			continue
		case "png ":
			return signed16(font.Be16(rec, 0)), signed16(font.Be16(rec, 2)), data, true
		}
		return 0, 0, nil, false
	}
}
