package shape

import "github.com/mgilbir/forme/font"

// The ink of a colour bitmap glyph: the metrics its CBDT bitmap states, which
// HarfBuzz asks before anything else about a glyph's extents.
//
// A face with CBDT and CBLC draws its glyphs as images at one or more sizes,
// each with metrics in pixels: the image's box and where it sits. HarfBuzz
// answers a glyph's extents from those, at the largest size the face has —
// which is the one it picks when asked at no particular size, as this package
// asks to measure — scaled from pixels to font units, and only then looks at
// COLR or the outline. Noto Color Emoji's compatibility build carries both
// CBDT and COLR, and its emoji were measured by their colour layers where
// HarfBuzz measures their bitmaps: 3,688 of its 39,235 glyphs.
//
// What is read is CBDT::accelerator_t::get_extents at the pinned release: the
// index subtable formats 1 and 3, and the image formats 17 and 18, whose
// metrics precede the image. The other formats HarfBuzz reads no metrics from,
// and neither does this. sbix, the other bitmap table, HarfBuzz asks before
// this one; it is read in sbixink.go.

// cbdtInk is the two tables a face's bitmap glyphs are read from.
type cbdtInk struct {
	cblc, cbdt []byte
	upem       int
}

func newCBDTInk(tables map[string][]byte, upem int) *cbdtInk {
	if len(tables["CBLC"]) < 8 || len(tables["CBDT"]) == 0 {
		return nil
	}
	return &cbdtInk{cblc: tables["CBLC"], cbdt: tables["CBDT"], upem: upem}
}

// The layout of what is read: a BitmapSizeTable is 48 bytes, its ppem at 44
// and 45; an IndexSubtableRecord 8; an IndexSubtableHeader 8.
const (
	bitmapSizeTableSize = 48
	indexSubtableRecord = 8
)

// extents is a glyph's bitmap ink in font units, and false where the face
// has no bitmap HarfBuzz reads metrics from.
func (c *cbdtInk) extents(gid int) (extents, bool) {
	return c.extentsAt(gid, 0)
}

// extentsAt is extents in the strike strikeFor picks for a size in pixels per
// em, or the largest for none.
func (c *cbdtInk) extentsAt(gid, ppem int) (extents, bool) {
	m, _, ppemX, ppemY, ok := c.metrics(gid, ppem)
	if !ok {
		return extents{}, false
	}
	// From pixels to font units, rounding half up in floats as HarfBuzz's
	// roundf does.
	return bitmapExtents(readStrikeMetrics(m), ppemX, ppemY, c.upem), true
}

// png is CBDT::reference_png with the metrics in front of the image: a
// glyph's PNG in the strike for a size, its width and height in pixels as
// those metrics state them, and whether the strike was drawn for that size
// (exactStrike). It is false where get_extents is, and for an image with no
// bytes, which HarfBuzz paints nothing of.
func (c *cbdtInk) png(gid, ppem int) (data []byte, width, height int, exact, ok bool) {
	strike, ok := c.strikeFor(ppem)
	if !ok {
		return nil, 0, 0, false, false
	}
	data, width, height, ok = c.pngIn(gid, strike)
	if !ok {
		return nil, 0, 0, false, false
	}
	return data, width, height, exactStrike(ppem, int(c.cblc[strike+44]), int(c.cblc[strike+45])), true
}

// pngIn is png in one strike, the offset of its BitmapSizeTable, with no size
// to say whether it is exact for.
func (c *cbdtInk) pngIn(gid, strike int) (data []byte, width, height int, ok bool) {
	m, format, _, _, ok := c.metricsIn(gid, strike)
	if !ok {
		return nil, 0, 0, false
	}
	// The data's length is after the metrics: small ones in format 17, five
	// bytes, and big ones in 18, eight.
	at := 5
	if format == 18 {
		at = 8
	}
	n := int(font.Be32(m, at))
	data = m[at+4:]
	if n < len(data) {
		data = data[:n]
	}
	if len(data) == 0 {
		return nil, 0, 0, false
	}
	return data, int(m[1]), int(m[0]), true
}

// metrics is where a glyph's image starts in CBDT, its metrics first, in the
// strike for a size, with its image format and the strike's ppem across and
// down: what get_extents and reference_png both read, for the two image
// formats with metrics.
func (c *cbdtInk) metrics(gid, ppem int) (m []byte, format, ppemX, ppemY int, ok bool) {
	strike, ok := c.strikeFor(ppem)
	if !ok {
		return nil, 0, 0, 0, false
	}
	return c.metricsIn(gid, strike)
}

// metricsIn is metrics in one strike, the offset of its BitmapSizeTable in
// CBLC, which the caller has checked is one of the table's.
func (c *cbdtInk) metricsIn(gid, strike int) (m []byte, format, ppemX, ppemY int, ok bool) {
	ppemX, ppemY = int(c.cblc[strike+44]), int(c.cblc[strike+45])
	array := int(font.Be32(c.cblc, strike))
	count := int(font.Be32(c.cblc, strike+8))
	if array <= 0 || array > len(c.cblc) {
		return nil, 0, 0, 0, false
	}
	// find_table: the first record whose range holds the glyph.
	rec := -1
	for i := 0; i < count; i++ {
		at := array + indexSubtableRecord*i
		if at+indexSubtableRecord > len(c.cblc) {
			return nil, 0, 0, 0, false
		}
		if first, last := font.Be16(c.cblc, at), font.Be16(c.cblc, at+2); first <= gid && gid <= last {
			rec = at
			break
		}
	}
	if rec < 0 || ppemX == 0 || ppemY == 0 {
		return nil, 0, 0, 0, false
	}
	offset, length, format, ok := c.imageData(array, rec, gid)
	if !ok || offset > len(c.cbdt) || len(c.cbdt)-offset < length {
		return nil, 0, 0, 0, false
	}
	switch {
	case format == 17 && length >= 9, format == 18 && length >= 12:
	default:
		return nil, 0, 0, 0, false
	}
	return c.cbdt[offset:], format, ppemX, ppemY, true
}

// strikeFor is the BitmapSizeTable HarfBuzz's choose_strike picks for a size
// in pixels per em: the smallest at least that large, or failing any the
// largest, the first of equals. Asked at no size, which it takes as 2^30, it is
// the largest. EBLC's strikes are chosen the same way (strikes.go).
func (c *cbdtInk) strikeFor(requested int) (int, bool) {
	return chooseStrike(c.cblc, requested)
}

// imageData is where a glyph's image is in CBDT, how long it is, and its
// format: IndexSubtable::get_image_data for the two index formats HarfBuzz
// reads, 1 and 3, whose offsets are four and two bytes.
func (c *cbdtInk) imageData(array, rec, gid int) (offset, length, format int, ok bool) {
	sub := array + int(font.Be32(c.cblc, rec+4))
	if sub+8 > len(c.cblc) {
		return 0, 0, 0, false
	}
	index, format := font.Be16(c.cblc, sub), font.Be16(c.cblc, sub+2)
	base := int(font.Be32(c.cblc, sub+4))
	i := gid - font.Be16(c.cblc, rec)
	var lo, hi int
	switch index {
	case 1:
		at := sub + 8 + 4*i
		if at+8 > len(c.cblc) {
			return 0, 0, 0, false
		}
		lo, hi = int(font.Be32(c.cblc, at)), int(font.Be32(c.cblc, at+4))
	case 3:
		at := sub + 8 + 2*i
		if at+4 > len(c.cblc) {
			return 0, 0, 0, false
		}
		lo, hi = font.Be16(c.cblc, at), font.Be16(c.cblc, at+2)
	default:
		return 0, 0, 0, false
	}
	if hi <= lo {
		return 0, 0, 0, false
	}
	return base + lo, hi - lo, format, true
}
