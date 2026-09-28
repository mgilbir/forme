package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// The ink of a colour bitmap glyph: the metrics its CBDT bitmap states, which
// HarfBuzz asks before anything else about a glyph's extents.
//
// A face with CBDT and CBLC draws its glyphs as images at one or more sizes,
// each with metrics in pixels: the image's box and where it sits. HarfBuzz
// answers a glyph's extents from those, at the largest size the face has —
// which is the one it picks when asked at no particular size, as this package
// always asks — scaled from pixels to font units, and only then looks at
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
	strike, ok := c.largestStrike()
	if !ok {
		return extents{}, false
	}
	ppemX, ppemY := int(c.cblc[strike+44]), int(c.cblc[strike+45])
	array := int(font.Be32(c.cblc, strike))
	count := int(font.Be32(c.cblc, strike+8))
	if array <= 0 || array > len(c.cblc) {
		return extents{}, false
	}
	// find_table: the first record whose range holds the glyph.
	rec := -1
	for i := 0; i < count; i++ {
		at := array + indexSubtableRecord*i
		if at+indexSubtableRecord > len(c.cblc) {
			return extents{}, false
		}
		if first, last := font.Be16(c.cblc, at), font.Be16(c.cblc, at+2); first <= gid && gid <= last {
			rec = at
			break
		}
	}
	if rec < 0 || ppemX == 0 || ppemY == 0 {
		return extents{}, false
	}
	offset, length, format, ok := c.imageData(array, rec, gid)
	if !ok || offset > len(c.cbdt) || len(c.cbdt)-offset < length {
		return extents{}, false
	}
	switch {
	case format == 17 && length >= 9, format == 18 && length >= 12:
	default:
		return extents{}, false
	}
	m := c.cbdt[offset:]
	height, width := int(m[0]), int(m[1])
	bearingX, bearingY := int(int8(m[2])), int(int8(m[3]))
	// From pixels to font units, rounding half up in floats as HarfBuzz's
	// roundf does.
	sx := float32(c.upem) / float32(ppemX)
	sy := float32(c.upem) / float32(ppemY)
	conv := func(v int, s float32) int {
		return int(clampToInt32(float64(float32(math.Floor(float64(float32(float32(v)*s) + 0.5))))))
	}
	return extents{
		xBearing: conv(bearingX, sx),
		yBearing: conv(bearingY, sy),
		width:    conv(width, sx),
		height:   conv(-height, sy),
	}, true
}

// largestStrike is the BitmapSizeTable HarfBuzz's choose_strike picks when
// asked at no size: the one with the largest ppem, the first of equals.
func (c *cbdtInk) largestStrike() (int, bool) {
	n := int(font.Be32(c.cblc, 4))
	if n == 0 || 8+bitmapSizeTableSize*n > len(c.cblc) {
		return 0, false
	}
	best := 8
	bestPPEM := max(int(c.cblc[best+44]), int(c.cblc[best+45]))
	for i := 1; i < n; i++ {
		at := 8 + bitmapSizeTableSize*i
		if ppem := max(int(c.cblc[at+44]), int(c.cblc[at+45])); ppem > bestPPEM {
			best, bestPPEM = at, ppem
		}
	}
	return best, true
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
