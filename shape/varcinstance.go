package shape

import (
	"fmt"
	"math"

	"github.com/mgilbir/forme/font"
)

// VARC glyphs in an instance.
//
// An instance is a static font: it carries no fvar, so a VARC table left in
// it would be read at the default of a design space the font no longer has,
// and a reader without VARC — a PDF reader among them — draws a VARC glyph's
// own glyf entry, which is empty or a placeholder. So an instance writes
// each VARC glyph out as the glyf outline HarfBuzz draws for it at the
// location — every leaf's points through its transform, rounded to the whole
// units glyf holds — and drops the table. fontTools' glyph set draws a VARC
// glyph the same way, and its TTGlyphPen rounds the same way.
//
// The ink of such a glyph is not its outline's box: HarfBuzz measures a VARC
// glyph by its leaves' boxes, turned (varc.go). So the instance keeps each
// VARC glyph's ink as HarfBuzz states it at the location, for everything that
// asks for ink, and draws the outline.
//
// The coordinates are HarfBuzz's: the location normalized in single
// precision, rounded to 16.16, mapped by avar and rounded to 2.14, as
// hb_ot_var_normalize_coords does it — a VARC glyph's components set axes by
// whole 2.14 units, so the one the instance is cut at is the one HarfBuzz
// draws at, and not one a rounding error from it.

// hbNormalizedCoords is hb_ot_var_normalize_coords for a location in user
// coordinates: every axis of fvar, its own default where the location does
// not name it, and none where HarfBuzz reads no fvar.
func hbNormalizedCoords(fvar, avar []byte, want map[string]float64) []int {
	count := hbFvarAxisCount(fvar)
	if count == 0 {
		return nil
	}
	at := font.Be16(fvar, 4)
	size := font.Be16(fvar, 10)
	fixed := func(off int) float32 { return float32(float32(int32(font.Be32(fvar, off))) * float32(1.0/65536)) }
	coords := make([]int, count)
	for i := range coords {
		rec := at + size*i
		lo, def, hi := fixed(rec+4), fixed(rec+8), fixed(rec+12)
		v := def
		if w, ok := want[string(fvar[rec:rec+4])]; ok {
			v = float32(w)
		}
		v = min(max(v, lo), hi)
		var n float32
		switch {
		case v == def:
		case v < def:
			n = float32(v-def) / float32(def-lo)
		default:
			n = float32(v-def) / float32(hi-def)
		}
		coords[i] = int(math.Floor(float64(float32(float32(n*65536) + 0.5))))
	}
	if len(avar) >= 8 && font.Be16(avar, 0) == 1 {
		n := min(font.Be16(avar, 6), count)
		p := 8
		for i := 0; i < n; i++ {
			if p+2 > len(avar) {
				break
			}
			segs := font.Be16(avar, p)
			if p+2+4*segs > len(avar) {
				break
			}
			m := make([][2]float32, segs)
			for k := range m {
				m[k] = [2]float32{f2dot14f(avar, p+2+4*k), f2dot14f(avar, p+4+4*k)}
			}
			mapped := avarMapFloat(m, float32(float32(coords[i])/65536))
			coords[i] = int(math.Floor(float64(float32(float32(mapped*65536) + 0.5))))
			p += 2 + 4*segs
		}
	}
	for i := range coords {
		coords[i] = (coords[i] + 2) >> 2
	}
	return coords
}

// hbFvarAxisCount is how many axes HarfBuzz reads from fvar: its count where
// fvar::sanitize takes the table — version 1, twenty-byte axis records, an
// instance record long enough for their coordinates, and every record inside
// the table — and none where it does not.
func hbFvarAxisCount(fvar []byte) int {
	if len(fvar) < 16 || font.Be16(fvar, 0) != 1 || font.Be16(fvar, 10) != 20 {
		return 0
	}
	first, count := font.Be16(fvar, 4), font.Be16(fvar, 8)
	instances, size := font.Be16(fvar, 12), font.Be16(fvar, 14)
	if size < count*4+4 || first+20*count > len(fvar) || first+20*count+instances*size > len(fvar) {
		return 0
	}
	return count
}

func f2dot14f(b []byte, at int) float32 {
	return float32(float32(signed16(font.Be16(b, at))) * float32(1.0/16384))
}

// avarMapFloat is SegmentMaps::map_float, CoreText's cases included.
func avarMapFloat(m [][2]float32, value float32) float32 {
	if len(m) < 2 {
		if len(m) == 0 {
			return value
		}
		return float32(float32(value-m[0][0]) + m[0][1])
	}
	start, end := 0, len(m)
	if m[start][0] == -1 && m[start][1] == -1 && m[start+1][0] == -1 {
		start++
	}
	if m[end-1][0] == 1 && m[end-1][1] == 1 && m[end-2][0] == 1 {
		end--
	}
	i := start
	for ; i < end; i++ {
		if value == m[i][0] {
			break
		}
	}
	if i < end {
		j := i
		for ; j+1 < end; j++ {
			if value != m[j+1][0] {
				break
			}
		}
		switch {
		case i == j:
			return m[i][1]
		case i+2 == j:
			return m[i+1][1]
		case value < 0:
			return m[j][1]
		case value > 0:
			return m[i][1]
		}
		if float32(math.Abs(float64(m[i][1]))) < float32(math.Abs(float64(m[j][1]))) {
			return m[i][1]
		}
		return m[j][1]
	}
	for i = start; i < end; i++ {
		if value < m[i][0] {
			break
		}
	}
	if i == start {
		return float32(float32(value-m[start][0]) + m[start][1])
	}
	if i == end {
		return float32(float32(value-m[end-1][0]) + m[end-1][1])
	}
	before, after := m[i-1], m[i]
	denom := float32(after[0] - before[0])
	return float32(before[1] + float32(float32(float32(after[1]-before[1])*float32(value-before[0]))/denom))
}

// flattenVARC writes each glyph the VARC table composes out as a glyf simple
// glyph drawn at the location, in varied, and returns each one's ink as
// HarfBuzz measures it there; nil where the font has no VARC table HarfBuzz
// reads. The glyph's metrics stay its glyf entry's, as HarfBuzz's do: VARC
// composes an outline and says nothing of advances.
//
// Where it cannot write a glyph exactly as drawn it refuses the font, since
// the instance would otherwise draw the glyph's placeholder: a leaf whose
// outline is CFF, a glyph past the work one may cost, one with more points
// than glyf holds — and a glyf composite that has a VARC glyph as a
// component, which HarfBuzz draws from the component's own glyf entry while
// the VARC glyph itself draws its composite, and one glyph index cannot hold
// both.
func flattenVARC(data []byte, tables map[string][]byte, fvar []byte, want map[string]float64,
	varied []*varGlyph, budget *int64) (map[int]extents, error) {
	if len(tables["VARC"]) == 0 {
		return nil, nil
	}
	src, err := loadFace(data, nil)
	if err != nil {
		return nil, fmt.Errorf("fonts: the font's VARC glyphs cannot be drawn to be instanced: %w", err)
	}
	v := src.varc
	if v == nil {
		return nil, nil
	}
	coords := hbNormalizedCoords(fvar, tables["avar"], want)
	covered := make([]bool, len(varied))
	for gid := range varied {
		covered[gid] = v.t.coverageIndex(gid) >= 0
	}
	for gid, g := range varied {
		if g == nil || !g.composite || covered[gid] {
			continue
		}
		for _, c := range g.comps {
			if c.glyph >= 0 && c.glyph < len(covered) && covered[c.glyph] {
				return nil, fmt.Errorf("fonts: glyph %d has glyph %d as a glyf component, which draws "+
					"that glyph's own glyf entry where the glyph itself draws its variable composite (VARC); "+
					"an instance cannot hold both under one glyph index", gid, c.glyph)
			}
		}
	}
	ink := map[int]extents{}
	for gid := range varied {
		if !covered[gid] {
			continue
		}
		o, err := v.outline(gid, coords, budget)
		if err == nil {
			varied[gid], err = o.flatGlyph()
		}
		if err != nil {
			return nil, fmt.Errorf("fonts: glyph %d is a variable composite (VARC) that cannot be written "+
				"out as a glyf outline, so it cannot be instanced: %w", gid, err)
		}
		if e, ok := v.measure(gid, coords, budget); ok {
			ink[gid] = e
		}
		if v.spent || *budget < 0 {
			return nil, fmt.Errorf("fonts: measuring glyph %d, a variable composite (VARC), ran past the work "+
				"one face may cost", gid)
		}
	}
	return ink, nil
}
