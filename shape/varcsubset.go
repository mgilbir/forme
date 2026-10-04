package shape

import (
	"encoding/binary"
	"fmt"
)

// VARC glyphs in a subset.
//
// What a PDF embeds is the subset's glyf, and a reader of it knows nothing of
// VARC: it draws a VARC glyph's own glyf entry, which is empty or a
// placeholder. So a subset writes each VARC glyph it keeps as the glyf
// outline HarfBuzz draws for it at the face's coordinates — the one
// LoadInstance writes for an instance (varcinstance.go) — and gives it a left
// side bearing of its xMin, which puts the origin where HarfBuzz drew from.
// The advance stays hmtx's, as HarfBuzz's does.
//
// It refuses where it cannot write a glyph exactly as drawn, since the
// document would otherwise show the placeholder with nothing to say so: a
// leaf whose outline is CFF, a glyph past the work one may cost or with more
// points than glyf holds, a glyf composite built on a VARC glyph (which
// HarfBuzz draws from that glyph's own glyf entry), and any VARC glyph of a
// CFF face, whose charstrings cannot hold what VARC draws.

// flattenKeptVARC is each kept VARC glyph written out as glyf bytes.
func (f *Face) flattenKeptVARC(keep []bool, offsets []uint32, glyf []byte) (map[int][]byte, error) {
	v := f.varc
	if v == nil {
		return nil, nil
	}
	n := len(keep)
	covered := func(gid int) bool { return v.t.coverageIndex(gid) >= 0 }
	var named []int
	for gid := 0; gid < n; gid++ {
		if !keep[gid] || covered(gid) {
			continue
		}
		start, end := offsets[gid], offsets[gid+1]
		if start >= end || int(end) > len(glyf) {
			continue
		}
		named = componentGlyphs(glyf[start:end], n, named[:0])
		for _, c := range named {
			if covered(c) {
				return nil, fmt.Errorf("fonts: glyph %d has glyph %d as a glyf component, which draws "+
					"that glyph's own glyf entry where the glyph itself draws its variable composite (VARC); "+
					"a subset cannot hold both under one glyph index", gid, c)
			}
		}
	}
	out := map[int][]byte{}
	// A subset's own share of work, as much as the face's, so that measuring
	// before it does not leave it none.
	work := varcFaceWork(f.sfntTables())
	for gid := 0; gid < n; gid++ {
		if !keep[gid] || !covered(gid) {
			continue
		}
		o, err := v.outline(gid, v.coords, &work)
		var g *varGlyph
		if err == nil {
			g, err = o.flatGlyph()
		}
		var b []byte
		if err == nil {
			b, err = encodeVarGlyph(g)
		}
		if err != nil {
			return nil, fmt.Errorf("fonts: glyph %d is a variable composite (VARC) that cannot be written "+
				"out as a glyf outline: %w", gid, err)
		}
		out[gid] = b
	}
	return out, nil
}

// flatBearings is hmtx with each written-out glyph's left side bearing set
// to its xMin, and nothing for one that draws nothing.
func flatBearings(hmtx []byte, longMetrics int, flat map[int][]byte) []byte {
	out := append([]byte(nil), hmtx...)
	for gid, b := range flat {
		lsb := 0
		if len(b) >= 10 {
			lsb = signed16(int(binary.BigEndian.Uint16(b[2:])))
		}
		at := 4*gid + 2
		if gid >= longMetrics {
			at = 4*longMetrics + 2*(gid-longMetrics)
		}
		if longMetrics > 0 && at+2 <= len(out) {
			binary.BigEndian.PutUint16(out[at:], uint16(int16(lsb)))
		}
	}
	return out
}

// usesVARCGlyph is the first glyph the face has used that VARC composes, if
// any.
func (f *Face) usesVARCGlyph() (int, bool) {
	if f.varc == nil {
		return 0, false
	}
	first, found := 0, false
	for gid := range f.used {
		if f.varc.t.coverageIndex(gid) >= 0 && (!found || gid < first) {
			first, found = gid, true
		}
	}
	return first, found
}

// flatGlyphs are the written-out glyphs' bytes.
func flatGlyphs(flat map[int][]byte) [][]byte {
	out := make([][]byte, 0, len(flat))
	for _, b := range flat {
		out = append(out, b)
	}
	return out
}

// raiseMaxp raises a version 1 maxp's maxPoints and maxContours, in place,
// to cover each of the simple glyphs given: what a reader sizes its buffers
// by, which a VARC glyph written out as glyf can exceed.
func raiseMaxp(maxp []byte, glyphs [][]byte) []byte {
	if len(maxp) < 32 || binary.BigEndian.Uint32(maxp) != 0x00010000 {
		return maxp
	}
	points, contours := int(binary.BigEndian.Uint16(maxp[6:])), int(binary.BigEndian.Uint16(maxp[8:]))
	for _, g := range glyphs {
		if len(g) < 12 {
			continue
		}
		nc := int(int16(binary.BigEndian.Uint16(g)))
		if nc <= 0 || len(g) < 10+2*nc {
			continue
		}
		contours = max(contours, nc)
		points = max(points, int(binary.BigEndian.Uint16(g[10+2*(nc-1):]))+1)
	}
	binary.BigEndian.PutUint16(maxp[6:], uint16(min(points, 0xFFFF)))
	binary.BigEndian.PutUint16(maxp[8:], uint16(min(contours, 0xFFFF)))
	return maxp
}
