package shape

import (
	"fmt"

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
// not name it, and none where HarfBuzz reads no fvar. The arithmetic is
// f2Dot14Location's, which every other reading of a location shares.
//
// It is asked only for a font LoadInstance has already read fvar and avar
// from, so an fvar or avar parseFvar or parseAvar refuses never reaches it;
// where one would, it answers nil, the default.
func hbNormalizedCoords(fvar, avar []byte, want map[string]float64) []int {
	if hbFvarAxisCount(fvar) == 0 {
		return nil
	}
	axes, err := parseFvar(fvar)
	if err != nil {
		return nil
	}
	segments, err := parseAvar(avar, len(axes))
	if err != nil {
		return nil
	}
	return f2Dot14Location(axes, segments, want)
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
