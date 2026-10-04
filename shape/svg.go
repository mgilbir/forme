package shape

import "github.com/mgilbir/forme/font"

// A glyph drawn as SVG: the OpenType SVG table, which HarfBuzz paints after
// COLR and before the bitmap tables (hb_ot_paint_glyph_or_fail).
//
// The table is an index of glyph ranges, each naming one SVG document, and the
// documents. A document may draw every glyph of its range, each the element
// whose id is "glyph" and the glyph's index, and may be gzip-compressed. What
// is handed out is the document, as HarfBuzz hands it to its image callback:
// the whole document, compressed or not, with no size in pixels and no extents
// — the document places the glyph itself, in font units with y running down,
// as the table's specification says. A renderer finds the glyph's element in
// it and draws that.
//
// What is read is OT::SVG at the release the oracle is pinned to: the table is
// refused where its header or index does not fit it, which is all HarfBuzz's
// sanitizer checks; a glyph's document is the one whose range the index's
// binary search finds it in; and a document is cut at the end of the table,
// and is none where it starts at or past the end, or states no length.

// svgTable is a face's SVG table: the table, and where its index starts.
type svgTable struct {
	b     []byte
	index int
	count int
}

// readSVG reads a face's SVG table, and nil where it has none, its index is
// null, or its header or index does not fit it.
func readSVG(b []byte) *svgTable {
	if len(b) < 10 {
		return nil
	}
	index := int(font.Be32(b, 2))
	if index == 0 {
		return nil
	}
	if index > len(b)-2 {
		return nil
	}
	count := font.Be16(b, index)
	if 12*count > len(b)-index-2 {
		return nil
	}
	return &svgTable{b: b, index: index, count: count}
}

// document is a glyph's SVG document: the index's binary search for the range
// that holds the glyph, then the document it names, cut at the end of the
// table. A face with no SVG table has no documents.
func (t *svgTable) document(gid int) ([]byte, bool) {
	if t == nil {
		return nil, false
	}
	lo, hi := 0, t.count-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		e := t.index + 2 + 12*mid
		switch first, last := font.Be16(t.b, e), font.Be16(t.b, e+2); {
		case gid < first:
			hi = mid - 1
		case gid > last:
			lo = mid + 1
		default:
			// The offset is from the index, added in 32 bits as HarfBuzz
			// adds it, so that one large enough to wrap names what the
			// wrapped sum does.
			at := int64(uint32(t.index) + font.Be32(t.b, e+4))
			n := int64(font.Be32(t.b, e+8))
			if n == 0 || at >= int64(len(t.b)) {
				return nil, false
			}
			end := min(at+n, int64(len(t.b)))
			return t.b[at:end:end], true
		}
	}
	return nil, false
}
