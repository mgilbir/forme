package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// Monochrome and greyscale bitmap strikes: EBLC and EBDT, and Apple's bloc
// and bdat, the same two tables under older names.
//
// A strike is a set of bitmaps drawn for one size in pixels per em, each pixel
// one, two, four or eight bits of coverage. EBLC says which strikes there are
// and where in EBDT each glyph's bitmap is; EBDT holds the bitmaps, and with
// most of them the glyph's metrics in pixels. They are what CBLC and CBDT
// were made from — the same BitmapSizeTables, the same index subtables — and
// carry what a colour bitmap does not: images that are a mask, painted in the
// colour of the text.
//
// A font with outlines that carries strikes as well — Courier New, Monaco,
// Geneva, PT Sans — states them as hand-tuned renderings of its outlines at a
// few small sizes, and its outlines are what it is drawn from here: a face is
// measured and painted from glyf or CFF wherever it has them, as HarfBuzz,
// which reads no EBDT, measures it. The strikes are read only for a face
// that has nothing else to draw (BitmapOnly), such as Apple's NISC18030,
// whose glyphs would otherwise be painted as nothing.
//
// # What is read
//
// HarfBuzz reads none of this, and FreeType is the reference: what is read is
// what its sfnt/ttsbit.c reads, and shape/strikes_test.go holds every glyph of
// every strike of two faces built for it to what FreeType loads
// (testdata/freetype/strikes.py).
//
//   - The strike is chosen for a size as a CBDT one is (chooseStrike): the
//     smallest at least that large, or failing any the largest. FreeType
//     selects only a strike of the size asked for; a caller that wants that
//     can tell an image of one apart by Image.Exact.
//   - A glyph's range is the first of the strike's IndexSubTableArray that
//     holds it, and the index subtable is any of formats 1 to 5: offsets of
//     four bytes or two, images of one size with the metrics in EBLC, and the
//     sparse formats that list their glyphs, read in order as FreeType reads
//     them. A glyph a range holds with no bytes is no image.
//   - The image is any of formats 1, 2, 5, 6 and 7: small metrics or big, or
//     none in format 5, whose metrics are its index subtable's; and the rows
//     each starting on a byte (1 and 6) or packed bit after bit (2, 5 and 7).
//     A bitmap whose bytes run past its image is refused, as FreeType refuses
//     it, rather than read in part.
//   - Formats 8 and 9 are composites: other glyphs of the strike, each placed
//     with its top left corner at an offset in pixels from the composite's,
//     and their samples combined by a bitwise or, as FreeType combines them.
//     A component that does not fit inside the composite's box refuses the
//     glyph, as it does in FreeType. Where FreeType places a component is
//     not always where the format says: it counts a horizontal offset in
//     bits, which in a greyscale strike is a fraction of a pixel, and drops
//     bits of a narrow bit-aligned component placed inside a byte. Those are
//     placed here where the format puts them, and the test holds them to
//     FreeType's own bitmaps of their components (ftComposite).
//   - Only the horizontal metrics are read; a strike's flags and vertical
//     metrics are not.
//
// # What it costs
//
// A bitmap is at most 255 pixels square, which the metrics' bytes bound. What
// is not bounded by the format is the work around it: the ranges and sparse
// lists searched for a glyph, which a table can make as long as it is, and the
// components, which can name composites that name others, the same ones
// again or the glyph itself. Each glyph read has a budget of its own for all
// of it — a unit for each range and entry searched, each component placed and
// each pixel written — and is refused whole where it runs out, or where
// components nest more than a hundred deep (maxStrikeDepth).

// ebdtStrikes is the two tables a face's monochrome and greyscale bitmaps are
// read from, and what reading them needs: the face's units per em, to place
// an image in font units.
type ebdtStrikes struct {
	loc, dat []byte
	upem     int
}

// The tables, in the order they are looked for: EBLC and EBDT, and failing
// them Apple's bloc and bdat. A pair is used only whole.
var strikeTablePairs = [...][2]string{{"EBLC", "EBDT"}, {"bloc", "bdat"}}

const (
	// strikeWork is the budget of one glyph's reading. A 255-pixel square
	// bitmap is 65,025 pixels; this is sixty-four of them.
	strikeWork = 1 << 22
	// maxStrikeDepth is how deep composites may nest: far past any font's,
	// and a bound on the recursion the budget alone would leave at a
	// composite of itself.
	maxStrikeDepth = 100
	// The offsets in a BitmapSizeTable of its bit depth, and the size of an
	// IndexSubTableArray entry.
	strikeBitDepth   = 46
	indexSubTableRec = 8
)

// newEBDTStrikes is the face's strikes, or nil where it has none: no pair of
// tables, or an EBLC whose version is older than 2.0 — EBLC's and bloc's are
// both 2.0, 0x00020000 — or whose strikes do not fit in it. FreeType refuses
// the table for the same.
func newEBDTStrikes(tables map[string][]byte, upem int) *ebdtStrikes {
	for _, pair := range strikeTablePairs {
		loc, dat := tables[pair[0]], tables[pair[1]]
		if len(loc) == 0 || len(dat) == 0 {
			continue
		}
		if len(loc) < 8 || font.Be32(loc, 0) < 0x00020000 {
			return nil
		}
		n := int64(font.Be32(loc, 4))
		if n == 0 || n >= 0x10000 || 8+bitmapSizeTableSize*n > int64(len(loc)) {
			return nil
		}
		return &ebdtStrikes{loc: loc, dat: dat, upem: upem}
	}
	return nil
}

// chooseStrike is HarfBuzz's choose_strike over the BitmapSizeTables a CBLC or
// EBLC table begins with: the offset of the one for a size in pixels per em,
// the smallest at least that large, or failing any the largest, the first of
// equals. A strike's size is the larger of its ppem across and down. Asked at
// no size, which it takes as 2^30, it is the largest.
func chooseStrike(loc []byte, requested int) (int, bool) {
	n := int64(font.Be32(loc, 4))
	if n == 0 || 8+bitmapSizeTableSize*n > int64(len(loc)) {
		return 0, false
	}
	if requested <= 0 {
		requested = 1 << 30
	}
	best := 8
	bestPPEM := max(int(loc[best+44]), int(loc[best+45]))
	for i := 1; i < int(n); i++ {
		at := 8 + bitmapSizeTableSize*i
		ppem := max(int(loc[at+44]), int(loc[at+45]))
		if requested <= ppem && ppem < bestPPEM || requested > bestPPEM && ppem > bestPPEM {
			best, bestPPEM = at, ppem
		}
	}
	return best, true
}

// exactStrike reports whether a strike of a ppem across and down was drawn for
// the size asked for: both are that size, and a size was asked for.
func exactStrike(requested, ppemX, ppemY int) bool {
	return requested > 0 && ppemX == requested && ppemY == requested
}

// strikeMetrics is a bitmap's horizontal metrics in pixels: its size, and its
// top left corner's place from the origin, y increasing upwards.
type strikeMetrics struct {
	width, height      int
	bearingX, bearingY int
}

// readStrikeMetrics reads the first four bytes small and big metrics share:
// height, width, and the two bearings, signed.
func readStrikeMetrics(b []byte) strikeMetrics {
	return strikeMetrics{
		height: int(b[0]), width: int(b[1]),
		bearingX: int(int8(b[2])), bearingY: int(int8(b[3])),
	}
}

// strikeImage is a glyph's image as EBLC finds it: its format, its bytes in
// EBDT, and the metrics its index subtable states for it, which only formats
// 2 and 5 do.
type strikeImage struct {
	format     int
	data       []byte
	locMetrics []byte
}

// strike is one strike of the table, as reading a glyph in it needs it.
type strike struct {
	at           int // its BitmapSizeTable's offset in EBLC
	ppemX, ppemY int
	depth        int
}

// strikeFor is the strike chooseStrike picks for a size, and false where its
// bit depth is not one EBDT has.
func (s *ebdtStrikes) strikeFor(ppem int) (strike, bool) {
	at, ok := chooseStrike(s.loc, ppem)
	if !ok {
		return strike{}, false
	}
	st := strike{at: at, ppemX: int(s.loc[at+44]), ppemY: int(s.loc[at+45]), depth: int(s.loc[at+strikeBitDepth])}
	switch st.depth {
	case 1, 2, 4, 8:
	default:
		return strike{}, false
	}
	return st, st.ppemX > 0 && st.ppemY > 0
}

// span is b[at:at+n], and false where that is not all inside b.
func span(b []byte, at, n int64) ([]byte, bool) {
	if at < 0 || n < 0 || at > int64(len(b)) || n > int64(len(b))-at {
		return nil, false
	}
	return b[at : at+n], true
}

// locate is tt_sbit_decoder_load_image's search: where a glyph's image is in a
// strike, its format, and the metrics its index subtable states for it.
func (s *ebdtStrikes) locate(st strike, gid int, work *font.Budget) (strikeImage, bool) {
	loc := s.loc
	array := int64(font.Be32(loc, st.at))
	count := int64(font.Be32(loc, st.at+8))
	if gid < 0 || gid > 0xFFFF {
		return strikeImage{}, false
	}
	// The first range that holds the glyph.
	var rec []byte
	for i := int64(0); i < count; i++ {
		r, ok := span(loc, array+indexSubTableRec*i, indexSubTableRec)
		if !ok || !work.Charge(1, "reading a bitmap strike") {
			return strikeImage{}, false
		}
		if first, last := font.Be16(r, 0), font.Be16(r, 2); first <= gid && gid <= last {
			rec = r
			break
		}
	}
	if rec == nil {
		return strikeImage{}, false
	}
	first := font.Be16(rec, 0)
	sub := array + int64(font.Be32(rec, 4))
	header, ok := span(loc, sub, 8)
	if !ok {
		return strikeImage{}, false
	}
	index, format := font.Be16(header, 0), font.Be16(header, 2)
	base := int64(font.Be32(header, 4))
	body := sub + 8
	i := int64(gid - first)
	var start, end int64
	var metrics []byte
	switch index {
	case 1, 3:
		// Offsets of four bytes or two, one a glyph of the range and one more
		// for the end of the last.
		size := int64(4)
		if index == 3 {
			size = 2
		}
		pair, ok := span(loc, body+size*i, 2*size)
		if !ok {
			return strikeImage{}, false
		}
		if size == 4 {
			start, end = int64(font.Be32(pair, 0)), int64(font.Be32(pair, 4))
		} else {
			start, end = int64(font.Be16(pair, 0)), int64(font.Be16(pair, 2))
		}
	case 2:
		// Images all of one size, and the big metrics they share.
		head, ok := span(loc, body, 4+8)
		if !ok {
			return strikeImage{}, false
		}
		size := int64(font.Be32(head, 0))
		start, end, metrics = size*i, size*(i+1), head[4:]
	case 4:
		// A sparse list of glyphs and offsets, one more for the end.
		head, ok := span(loc, body, 4)
		if !ok {
			return strikeImage{}, false
		}
		pairs, ok := span(loc, body+4, 4*(int64(font.Be32(head, 0))+1))
		if !ok {
			return strikeImage{}, false
		}
		k := -1
		for j := 0; j+4 < len(pairs); j += 4 {
			if !work.Charge(1, "reading a bitmap strike") {
				return strikeImage{}, false
			}
			if font.Be16(pairs, j) == gid {
				k = j
				break
			}
		}
		if k < 0 {
			return strikeImage{}, false
		}
		start, end = int64(font.Be16(pairs, k+2)), int64(font.Be16(pairs, k+6))
	case 5:
		// Images all of one size and their big metrics, as format 2, for a
		// sparse list of glyphs.
		head, ok := span(loc, body, 4+8+4)
		if !ok {
			return strikeImage{}, false
		}
		size, n := int64(font.Be32(head, 0)), int64(font.Be32(head, 12))
		ids, ok := span(loc, body+16, 2*n)
		if !ok {
			return strikeImage{}, false
		}
		k := int64(-1)
		for j := range n {
			if !work.Charge(1, "reading a bitmap strike") {
				return strikeImage{}, false
			}
			if font.Be16(ids, int(2*j)) == gid {
				k = j
				break
			}
		}
		if k < 0 {
			return strikeImage{}, false
		}
		start, end, metrics = size*k, size*(k+1), head[4:12]
	default:
		return strikeImage{}, false
	}
	if end <= start {
		return strikeImage{}, false
	}
	data, ok := span(s.dat, base+start, end-start)
	if !ok {
		return strikeImage{}, false
	}
	return strikeImage{format: format, data: data, locMetrics: metrics}, true
}

// metrics is an image's own metrics, where its format states them, or its
// index subtable's, and the bytes after them: the bitmap, or a composite's
// components.
func (img strikeImage) metrics() (m strikeMetrics, rest []byte, ok bool) {
	d := img.data
	switch img.format {
	case 1, 2, 8:
		if len(d) < 5 {
			return strikeMetrics{}, nil, false
		}
		m, rest = readStrikeMetrics(d), d[5:]
		if img.format == 8 {
			// A byte of padding before the components.
			if len(rest) < 1 {
				return strikeMetrics{}, nil, false
			}
			rest = rest[1:]
		}
	case 6, 7, 9:
		if len(d) < 8 {
			return strikeMetrics{}, nil, false
		}
		m, rest = readStrikeMetrics(d), d[8:]
	case 5:
		if img.locMetrics == nil {
			return strikeMetrics{}, nil, false
		}
		m, rest = readStrikeMetrics(img.locMetrics), d
	default:
		return strikeMetrics{}, nil, false
	}
	return m, rest, true
}

// glyphMetrics is a glyph's metrics in the strike for a size, and the strike.
func (s *ebdtStrikes) glyphMetrics(gid, ppem int) (strikeMetrics, strike, bool) {
	st, ok := s.strikeFor(ppem)
	if !ok {
		return strikeMetrics{}, strike{}, false
	}
	img, ok := s.locate(st, gid, font.NewBudget(strikeWork))
	if !ok {
		return strikeMetrics{}, strike{}, false
	}
	m, _, ok := img.metrics()
	return m, st, ok
}

// extents is a glyph's ink in font units from its metrics in the largest
// strike, and false where no strike has it.
func (s *ebdtStrikes) extents(gid int) (extents, bool) {
	return s.extentsAt(gid, 0)
}

// extentsAt is extents in the strike for a size in pixels per em, scaled from
// pixels to font units and rounded as a CBDT glyph's are (bitmapink.go).
func (s *ebdtStrikes) extentsAt(gid, ppem int) (extents, bool) {
	m, st, ok := s.glyphMetrics(gid, ppem)
	if !ok {
		return extents{}, false
	}
	return bitmapExtents(m, st.ppemX, st.ppemY, s.upem), true
}

// bitmapExtents is a bitmap's metrics in pixels as extents in font units,
// rounding half up in single precision as HarfBuzz's roundf does for CBDT.
func bitmapExtents(m strikeMetrics, ppemX, ppemY, upem int) extents {
	sx := float32(upem) / float32(ppemX)
	sy := float32(upem) / float32(ppemY)
	conv := func(v int, s float32) int {
		return int(clampToInt32(float64(float32(math.Floor(float64(float32(float32(v)*s) + 0.5))))))
	}
	return extents{
		xBearing: conv(m.bearingX, sx),
		yBearing: conv(m.bearingY, sy),
		width:    conv(m.width, sx),
		height:   conv(-m.height, sy),
	}
}

// image is a glyph's bitmap in the strike for a size, as an ImageMask, and
// false where the strike has none of it, the bitmap has no pixels, or reading
// it is refused.
func (s *ebdtStrikes) image(gid, ppem int) (Image, bool) {
	st, ok := s.strikeFor(ppem)
	if !ok {
		return Image{}, false
	}
	work := font.NewBudget(strikeWork)
	img, ok := s.locate(st, gid, work)
	if !ok {
		return Image{}, false
	}
	m, _, ok := img.metrics()
	if !ok || m.width == 0 || m.height == 0 {
		return Image{}, false
	}
	samples := make([]byte, m.width*m.height)
	if !s.draw(st, img, samples, m.width, m.height, 0, 0, 0, work) {
		return Image{}, false
	}
	// From samples of the strike's depth to coverage from 0 to 255: 255 is
	// divisible by 1, 3, 15 and 255, so each sample is a whole number.
	scale := byte(255 / (1<<st.depth - 1))
	for i, v := range samples {
		samples[i] = v * scale
	}
	upem := float64(s.upem)
	sx, sy := upem/float64(st.ppemX), upem/float64(st.ppemY)
	return Image{
		Format: ImageMask,
		Data:   samples,
		Width:  m.width,
		Height: m.height,
		Box: Rect{
			XMin: float64(m.bearingX) * sx, YMin: float64(m.bearingY-m.height) * sy,
			XMax: float64(m.bearingX+m.width) * sx, YMax: float64(m.bearingY) * sy,
		},
		Exact: exactStrike(ppem, st.ppemX, st.ppemY),
	}, true
}

// draw writes an image's samples into dst, a bitmap w by h, with the image's
// top left corner at (x, y), or the images of a composite's components at
// their offsets from there, each sample combined with what is there by a
// bitwise or. It is false where the image is refused: its bytes are too few,
// it does not fit, a component cannot be read, or the work runs out.
func (s *ebdtStrikes) draw(st strike, img strikeImage, dst []byte, w, h, x, y, depth int, work *font.Budget) bool {
	m, rest, ok := img.metrics()
	if !ok {
		return false
	}
	switch img.format {
	case 8, 9:
		// A composite's own box is not where its components must fit: they
		// are placed in the glyph's bitmap, the outermost composite's, as
		// FreeType places them.
		if depth >= maxStrikeDepth || len(rest) < 2 {
			return false
		}
		n := font.Be16(rest, 0)
		comps, ok := span(rest, 2, 4*int64(n))
		if !ok {
			return false
		}
		for i := range n {
			c := comps[4*i:]
			if !work.Charge(1, "reading a bitmap strike") {
				return false
			}
			ci, ok := s.locate(st, font.Be16(c, 0), work)
			if !ok || !s.draw(st, ci, dst, w, h, x+int(int8(c[2])), y+int(int8(c[3])), depth+1, work) {
				return false
			}
		}
		return true
	}
	if x < 0 || y < 0 || m.width > w-x || m.height > h-y {
		return false
	}
	if !work.Charge(m.width*m.height, "reading a bitmap strike") {
		return false
	}
	bits := st.depth
	// The bits of a row, and the bytes the bitmap takes: each row starting on
	// a byte in formats 1 and 6, and packed one after the other in 2, 5 and 7.
	rowBits := m.width * bits
	aligned := img.format == 1 || img.format == 6
	need := (rowBits*m.height + 7) / 8
	if aligned {
		need = (rowBits + 7) / 8 * m.height
	}
	if len(rest) < need {
		return false
	}
	mask := byte(1<<bits - 1)
	at := 0 // in bits
	for row := range m.height {
		if aligned {
			at = row * ((rowBits + 7) / 8) * 8
		}
		out := dst[(y+row)*w+x:]
		for col := range m.width {
			v := rest[at/8] >> (8 - bits - at%8) & mask
			out[col] |= v
			at += bits
		}
	}
	return true
}
