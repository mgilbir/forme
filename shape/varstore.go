package shape

import (
	"errors"
	"fmt"

	"github.com/mgilbir/forme/font"
)

// The item variation store, and 'HVAR', the table built on it that says how a
// glyph's advance changes across the design space.
//
// It is the same idea as gvar's tuples in a different shape: a list of regions
// of the design space, and per item a delta for each region, weighted by how far
// into the region the location falls. gvar states its regions per glyph; this
// states them once and has every item point at them, which is what suits a
// table with one number per glyph.
//
// # Why HVAR wins over the phantom points
//
// A variable font can say how an advance varies twice: in HVAR, and in the
// phantom points gvar carries with each glyph's outline. Where both are present
// they are meant to agree, and mostly do — but not always. Noto Sans states
// thirteen advances differently in the two at weight 700, and what a renderer
// draws is HVAR: HarfBuzz reads it in preference and so does every browser built
// on it. A document laid out from the phantom points there would set thirteen
// glyphs one unit from where the same text is set on screen.
//
// So HVAR is used when it is there, and the phantom points are the fallback for
// a font without it — which is what the specification says, and what makes the
// two paths agree with the renderer rather than with each other.

// varRegion is one axis's part of a region: the span it covers and the point
// within it where it applies in full.
type varRegion struct{ start, peak, end float64 }

// varStore is a parsed item variation store.
type varStore struct {
	regions [][]varRegion
	data    []varStoreData
}

// varStoreData is one delta-set group: which regions its columns are, and a row
// per item.
type varStoreData struct {
	regions []int
	rows    [][]float64
}

func parseVarStore(t []byte) (*varStore, error) {
	if len(t) < 8 {
		return nil, errors.New("the item variation store is too short to hold its header")
	}
	if f := font.Be16(t, 0); f != 1 {
		return nil, fmt.Errorf("the item variation store is format %d, which this does not read", f)
	}
	regionsOff := int(font.Be32(t, 2))
	count := font.Be16(t, 6)
	if 8+4*count > len(t) {
		return nil, errors.New("the item variation store's offset array is cut short")
	}
	if regionsOff < 0 || regionsOff+4 > len(t) {
		return nil, errors.New("the item variation store's region list lies outside it")
	}
	rl := t[regionsOff:]
	axisCount := font.Be16(rl, 0)
	regionCount := font.Be16(rl, 2)
	// The count needs no cap: a region costs six bytes an axis in the table, and
	// nothing is allocated until they are known to be there.
	if 4+6*axisCount*regionCount > len(rl) {
		return nil, errors.New("the item variation store's region list is cut short")
	}
	s := &varStore{regions: make([][]varRegion, regionCount)}
	for i := range s.regions {
		axes := make([]varRegion, axisCount)
		for a := range axes {
			at := 4 + 6*(axisCount*i+a)
			axes[a] = varRegion{
				start: f2Dot14At(rl, at),
				peak:  f2Dot14At(rl, at+2),
				end:   f2Dot14At(rl, at+4),
			}
		}
		s.regions[i] = axes
	}

	s.data = make([]varStoreData, count)
	for i := range s.data {
		off := int(font.Be32(t, 8+4*i))
		if off < 0 || off+6 > len(t) {
			return nil, fmt.Errorf("the item variation store's group %d lies outside it", i)
		}
		d, err := parseVarStoreData(t[off:], regionCount)
		if err != nil {
			return nil, fmt.Errorf("the item variation store's group %d: %w", i, err)
		}
		s.data[i] = d
	}
	return s, nil
}

func parseVarStoreData(b []byte, regionCount int) (varStoreData, error) {
	var d varStoreData
	itemCount := font.Be16(b, 0)
	wordDeltaCount := font.Be16(b, 2)
	regionIndexCount := font.Be16(b, 4)
	if 6+2*regionIndexCount > len(b) {
		return d, errors.New("its region index list is cut short")
	}
	longWords := wordDeltaCount&0x8000 != 0
	wordCount := wordDeltaCount & 0x7FFF
	if wordCount > regionIndexCount {
		return d, fmt.Errorf("it states %d long columns of %d", wordCount, regionIndexCount)
	}
	d.regions = make([]int, regionIndexCount)
	for i := range d.regions {
		r := font.Be16(b, 6+2*i)
		if r >= regionCount {
			return d, fmt.Errorf("it names region %d of %d", r, regionCount)
		}
		d.regions[i] = r
	}
	wordSize, shortSize := 2, 1
	if longWords {
		wordSize, shortSize = 4, 2
	}
	rowSize := wordCount*wordSize + (regionIndexCount-wordCount)*shortSize
	at := 6 + 2*regionIndexCount
	// The rows have to be in the table before they are allocated for: itemCount
	// is a number in the file, and rowSize can be zero.
	if rowSize == 0 {
		itemCount = 0
	} else if need := itemCount * rowSize; need > len(b)-at {
		return d, fmt.Errorf("it states %d rows of %d bytes and carries %d", itemCount, rowSize, len(b)-at)
	}
	d.rows = make([][]float64, itemCount)
	for i := range d.rows {
		row := make([]float64, regionIndexCount)
		for j := range row {
			var v int
			if j < wordCount {
				v = signedAt(b, at, wordSize)
				at += wordSize
			} else {
				v = signedAt(b, at, shortSize)
				at += shortSize
			}
			row[j] = float64(v)
		}
		d.rows[i] = row
	}
	return d, nil
}

// signedAt reads a big-endian signed integer of one, two or four bytes. The
// caller has already checked the bytes are there.
func signedAt(b []byte, at, size int) int {
	switch size {
	case 1:
		return int(int8(b[at]))
	case 2:
		return int(int16(uint16(font.Be16(b, at))))
	default:
		return int(int32(font.Be32(b, at)))
	}
}

// delta is the adjustment a delta-set index names at a location, or zero when
// the index names nothing — which is what a font with a mapping longer than its
// store produces, and is not an error a caller can do anything about.
func (s *varStore) delta(outer, inner int, coords []float64) float64 {
	if outer < 0 || outer >= len(s.data) {
		return 0
	}
	d := s.data[outer]
	if inner < 0 || inner >= len(d.rows) {
		return 0
	}
	row := d.rows[inner]
	var total float64
	for i, r := range d.regions {
		scalar := regionScalar(s.regions[r], coords)
		if scalar == 0 {
			continue
		}
		// See the note in gvar.go's infer: the conversion keeps the multiply
		// from being fused into the add on the architectures that would.
		total += float64(row[i] * scalar)
	}
	return total
}

// regionScalar weighs one region at a location: one at its peak, zero outside
// it, a linear ramp between.
//
// A region that spans the default instance on some axis is not a ramp on that
// axis at all — the specification reserves that shape and says to ignore the
// axis — and so is one whose peak lies outside its own span.
func regionScalar(region []varRegion, coords []float64) float64 {
	scalar := 1.0
	for i, r := range region {
		if r.peak == 0 {
			continue
		}
		if r.start > r.peak || r.peak > r.end {
			continue
		}
		if r.start < 0 && r.end > 0 {
			continue
		}
		var v float64
		if i < len(coords) {
			v = coords[i]
		}
		if v == r.peak {
			continue
		}
		if v <= r.start || r.end <= v {
			return 0
		}
		if v < r.peak {
			scalar *= (v - r.start) / (r.peak - r.start)
		} else {
			scalar *= (r.end - v) / (r.end - r.peak)
		}
	}
	return scalar
}

// deltaSetIndexMap maps a glyph index to a delta-set index. When a font leaves
// it out, the glyph index *is* the inner index of group zero.
type deltaSetIndexMap struct {
	entries       []uint32
	innerBitCount uint
}

func parseDeltaSetIndexMap(b []byte) (*deltaSetIndexMap, error) {
	if len(b) < 4 {
		return nil, errors.New("the delta-set index map is too short to hold its header")
	}
	var count, at int
	switch b[0] {
	case 0:
		count = font.Be16(b, 2)
		at = 4
	case 1:
		if len(b) < 8 {
			return nil, errors.New("the delta-set index map is too short to hold its header")
		}
		n := font.Be32(b, 4)
		if n > uint32(len(b)) {
			// Every entry is at least one byte, so a count past the table's own
			// size cannot be honoured whatever the entry size turns out to be.
			return nil, fmt.Errorf("the delta-set index map states %d entries in %d bytes", n, len(b))
		}
		count = int(n)
		at = 8
	default:
		return nil, fmt.Errorf("the delta-set index map is format %d, which this does not read", b[0])
	}
	format := b[1]
	innerBits := uint(format&0x0F) + 1
	entrySize := int((format&0x30)>>4) + 1
	if at+count*entrySize > len(b) {
		return nil, errors.New("the delta-set index map's entries are cut short")
	}
	m := &deltaSetIndexMap{entries: make([]uint32, count), innerBitCount: innerBits}
	for i := 0; i < count; i++ {
		var v uint32
		for j := 0; j < entrySize; j++ {
			v = v<<8 | uint32(b[at+i*entrySize+j])
		}
		m.entries[i] = v
	}
	return m, nil
}

// lookup gives the two-level index for a glyph. A glyph past the end of the map
// takes the last entry, which is what the specification says and what lets a
// font state one entry for a long tail of glyphs that share a delta set.
func (m *deltaSetIndexMap) lookup(gid int) (outer, inner int) {
	if len(m.entries) == 0 {
		return 0, 0
	}
	if gid < 0 {
		gid = 0
	}
	if gid >= len(m.entries) {
		gid = len(m.entries) - 1
	}
	v := m.entries[gid]
	return int(v >> m.innerBitCount), int(v & (1<<m.innerBitCount - 1))
}

// hvarTable is 'HVAR': the store, and the map from glyph to delta set.
type hvarTable struct {
	store   *varStore
	advance *deltaSetIndexMap
}

func parseHVAR(t []byte) (*hvarTable, error) {
	if len(t) < 20 {
		return nil, errors.New("fonts: the HVAR table is too short to hold its header")
	}
	if font.Be16(t, 0) != 1 {
		return nil, fmt.Errorf("fonts: HVAR is version %d, which this does not read", font.Be16(t, 0))
	}
	storeOff := int(font.Be32(t, 4))
	if storeOff <= 0 || storeOff >= len(t) {
		return nil, errors.New("fonts: HVAR's item variation store lies outside it")
	}
	store, err := parseVarStore(t[storeOff:])
	if err != nil {
		return nil, fmt.Errorf("fonts: HVAR: %w", err)
	}
	h := &hvarTable{store: store}
	if off := int(font.Be32(t, 8)); off != 0 {
		if off < 0 || off >= len(t) {
			return nil, errors.New("fonts: HVAR's advance mapping lies outside it")
		}
		m, err := parseDeltaSetIndexMap(t[off:])
		if err != nil {
			return nil, fmt.Errorf("fonts: HVAR's advance mapping: %w", err)
		}
		h.advance = m
	}
	return h, nil
}

// advanceDelta is how much a glyph's advance changes at a location.
func (h *hvarTable) advanceDelta(gid int, coords []float64) float64 {
	outer, inner := 0, gid
	if h.advance != nil {
		outer, inner = h.advance.lookup(gid)
	}
	return h.store.delta(outer, inner, coords)
}
