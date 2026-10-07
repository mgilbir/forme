package shape

import (
	"sort"

	"github.com/mgilbir/forme/font"
)

// A face's bitmap strikes, listed, and a glyph's image in one of them.
//
// PaintGlyph chooses a strike for the size it is asked at, as HarfBuzz does —
// the smallest at least that large, or failing any the largest — and a glyph
// the chosen strike does not hold is painted from nothing else, though another
// strike may hold it: a face's largest strike commonly holds fewer glyphs than
// its others. A caller that falls back to the nearest strike holding a glyph,
// as a writer of Type 3 fonts drawn from a face's bitmaps does, needs what the
// face has rather than what one size chooses (issue 915). Strikes is that, and
// StrikeImage is a glyph's image in exactly one of them; Image.Strike says
// which strike a painted image came from.

// Strike is one of a face's bitmap strikes: the images of its glyphs drawn for
// one size.
type Strike struct {
	// Table is the table it is in: "CBDT", "sbix", "EBDT", or "bdat", Apple's
	// EBDT.
	Table string
	// PPEMX and PPEMY are the size it was drawn for, in pixels per em across
	// and down. An sbix strike states one size, which is both.
	PPEMX, PPEMY int
	// at is where it is in its table: the offset of its BitmapSizeTable in
	// CBLC or EBLC, or of the strike itself in sbix.
	at int
}

// PPEM is the size a strike is chosen by for PaintOptions.PPEM: the larger of
// its two.
func (s Strike) PPEM() int { return max(s.PPEMX, s.PPEMY) }

// Strikes lists the face's bitmap strikes — CBDT's, sbix's, and EBDT's or
// bdat's — from the smallest size to the largest, and in that order of tables
// where two are the same size. A strike that holds no glyph a reader of it
// could paint is not listed: an sbix null strike, one of no size, and an EBDT
// one of a bit depth the table does not have. EBDT strikes are listed for a
// face with outlines too; see PaintOptions.Bitmaps.
func (f *Face) Strikes() []Strike {
	var out []Strike
	if c := f.bitmap; c != nil {
		n := int64(font.Be32(c.cblc, 4))
		if 8+bitmapSizeTableSize*n <= int64(len(c.cblc)) {
			for i := range int(n) {
				at := 8 + bitmapSizeTableSize*i
				if x, y := int(c.cblc[at+44]), int(c.cblc[at+45]); x > 0 && y > 0 {
					out = append(out, Strike{Table: "CBDT", PPEMX: x, PPEMY: y, at: at})
				}
			}
		}
	}
	if s := f.sbix; s != nil {
		for i := range int(font.Be32(s.table, 4)) {
			at := int(font.Be32(s.table, 8+4*i))
			if at == 0 {
				continue // the null strike
			}
			if ppem := font.Be16(s.table, at); ppem > 0 {
				out = append(out, Strike{Table: "sbix", PPEMX: ppem, PPEMY: ppem, at: at})
			}
		}
	}
	if e := f.strikes; e != nil {
		// newEBDTStrikes has checked that the count fits in the table.
		for i := range int(font.Be32(e.loc, 4)) {
			if st, ok := e.strikeAt(8 + bitmapSizeTableSize*i); ok {
				out = append(out, e.public(st))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PPEM() < out[j].PPEM() })
	return out
}

// public is an EBDT strike as Strikes lists it.
func (e *ebdtStrikes) public(st strike) Strike {
	return Strike{Table: e.tag, PPEMX: st.ppemX, PPEMY: st.ppemY, at: st.at}
}

// StrikeImage is a glyph's image in one of the face's strikes, as PaintGlyph
// would paint it from that strike: a PNG from CBDT or sbix, or from EBDT or
// bdat a mask, whose Color is the zero Color for the caller to paint in its
// own. It is Exact where the strike is as many pixels per em across as down,
// as PaintGlyph's is at that size — one that is not is drawn for pixels that
// are not square, and is scaled at any one size — and placed in Box as
// PaintGlyph places it. It is false where the strike holds no image of the
// glyph, and for a strike that is not one Strikes lists for this face.
//
// It reads the one strike and decodes the one image, so a caller asking each
// strike in turn for the nearest that holds a glyph pays for the strikes it
// asks and not for painting.
func (f *Face) StrikeImage(gid int, s Strike) (Image, bool) {
	if f.prog == nil || gid < 0 || gid >= f.prog.NumGlyphs || !f.hasStrike(s) {
		return Image{}, false
	}
	var img Image
	switch s.Table {
	case "CBDT":
		data, width, height, ok := f.bitmap.pngIn(gid, s.at)
		if !ok {
			return Image{}, false
		}
		m, _, ppemX, ppemY, _ := f.bitmap.metricsIn(gid, s.at)
		e := bitmapExtents(readStrikeMetrics(m), ppemX, ppemY, f.unitsPerEm)
		img = Image{Format: ImagePNG, Data: data, Width: width, Height: height, Box: extentsBox(e)}
	case "sbix":
		_, _, width, height, data, ok := f.sbix.pngIn(gid, s.at, s.PPEMX)
		if !ok {
			return Image{}, false
		}
		e, ok := f.sbix.extentsIn(gid, s.at, s.PPEMX)
		if !ok {
			return Image{}, false
		}
		img = Image{Format: ImagePNG, Data: data, Width: width, Height: height, Box: extentsBox(e)}
	default:
		st, ok := f.strikes.strikeAt(s.at)
		if !ok {
			return Image{}, false
		}
		if img, ok = f.strikes.imageIn(gid, st); !ok {
			return Image{}, false
		}
	}
	img.Exact, img.Strike = s.PPEMX == s.PPEMY, s
	return img, true
}

// hasStrike reports whether a strike is one Strikes lists for the face, which
// is what makes its offset one StrikeImage may read at.
func (f *Face) hasStrike(s Strike) bool {
	for _, have := range f.Strikes() {
		if have == s {
			return true
		}
	}
	return false
}

// extentsBox is the box a bitmap glyph's image is drawn in, from its extents:
// what bitmapImage draws a CBDT or sbix image in.
func extentsBox(e extents) Rect {
	return Rect{
		XMin: float64(e.xBearing), YMin: float64(e.yBearing + e.height),
		XMax: float64(e.xBearing + e.width), YMax: float64(e.yBearing),
	}
}
