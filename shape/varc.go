package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// Variable composites: the VARC table of OpenType 1.9.1.
//
// A VARC glyph is drawn from other glyphs, each through a transform — a
// translation, a rotation, a scale, a skew, about a centre — and each at a
// location of its own in the font's design space: a component names axes and
// the values it sets them to, which is how one glyph of a stroke is drawn at
// the weight and the length a character needs. The values, and the transform,
// vary in their turn by a MultiItemVariationStore at the location the VARC
// glyph is drawn at, and a component may be drawn only where a condition on
// that location holds. What a VARC glyph's own glyf entry holds is not what it
// draws: the entry is empty, or a glyph a reader without VARC falls back to.
//
// What is read is HarfBuzz 14.5.0's VARC::get_path_at, number for number:
//
//   - The table is taken as HarfBuzz's sanitizer takes it, or not at all: its
//     header, coverage, store, conditions, axis lists and records each inside
//     it, conditions nested no deeper than 64, and the checking within the
//     64-units-a-byte allowance sbixink.go describes. A table it refuses
//     leaves the face's glyphs to their outlines.
//   - A face with a VARC table has every glyph's ink asked of it, covered or
//     not, and a glyph it does not cover is a leaf of itself: the ink of its
//     glyf outline at the location, as a box, which is joined into nothing
//     where it has no area — so that an empty glyph in such a face has the
//     ink HarfBuzz gives a void box, a unit that is not there, and not none.
//   - The location is HarfBuzz's coordinates for the face, and a face with a
//     design space has them even at its default: hb_font_create sets every
//     axis to it, so a zero for each is handed down. It matters: a leaf with
//     any coordinates is measured by its points at them, and one with none —
//     in a face with no fvar — by its glyph header.
//   - A covered glyph's components are read one by one until one does not
//     decode. Each draws, where its condition holds, the glyph it names at
//     the coordinates it sets — rounded, in single precision, as roundf
//     rounds — through its transform composed in single precision with the
//     transforms around it. A component naming the glyph it is in draws that
//     glyph's own outline. Components nest 64 deep at most, 16,384 of them
//     are walked a glyph at most, and a glyph that is its own ancestor is
//     found by HarfBuzz's decycler and not drawn.
//   - A leaf's ink is its outline's box at its coordinates, measured as
//     glyf's get_extents_at measures it, and put through the transform
//     corner by corner: the ink of a VARC glyph is the union of its leaves'
//     boxes turned, not the box of their turned points. The union is rounded
//     half away from zero. What the glyph draws is its leaves' points turned
//     (varcink.go), which is what a colour glyph clipped to it is bounded by
//     and what an instance and a subset write out as glyf (varcinstance.go,
//     varcsubset.go).
//
// # What is not here
//
//   - HarfBuzz's own budget: it charges each component, each axis value and
//     each point of each leaf against 2^24 units a glyph and draws nothing
//     more once they are spent. This charges a budget of its own, which a
//     font reaches only by being built to, and reports the glyphs it stops
//     short of through Face.LayoutLimits instead of reproducing where
//     HarfBuzz's would have stopped.
//   - The cache HarfBuzz keeps of a region's scalar within one glyph, which
//     holds it to 2^-30 and gives a scalar under 2^-7 back a bit or two
//     coarser the second time it is asked.
//   - A leaf whose outline is CFF2, which nothing in this module reads. A CFF
//     leaf has its ink, as cffink.go measures it, and no outline to flatten.
//   - The points of a glyf leaf at its coordinates are moved by gvar in double
//     precision (gvar.go), where HarfBuzz moves them in single: a leaf's box
//     can round the other way where a coordinate lands within a rounding
//     error of a half.

// varcTable is a VARC table HarfBuzz's sanitizer takes, and where each of its
// parts is in it; a part at offset 0 is null, and reads as HarfBuzz reads its
// Null object.
type varcTable struct {
	t          []byte
	coverage   int
	store      int
	conditions int
	axes       varcIndex
	records    varcIndex
}

// varcIndex is a CFF2 INDEX: a 32-bit count, an offset size, count+1 offsets
// counted from one, and the data after them.
type varcIndex struct {
	count, offSize, offsets, data int
}

// Sanitizer bounds HarfBuzz states that VARC reaches: conditions nest no
// deeper than HB_MAX_NESTING_LEVEL, and neither do components.
const (
	hbMaxNesting    = 64
	hbMaxEdges      = 16384
	hbMaxVarcAxes   = 4096
	varcNoVariation = 0xFFFFFFFF
)

// varcSanitizer is hb_sanitize_context_t as far as VARC asks it: ranges
// checked against the table, and the checking charged against its allowance.
type varcSanitizer struct {
	t     []byte
	ops   int64
	depth int
}

// point is check_point: that a position is within the table, the end
// included. check_struct is this on a 64-bit build, and charges nothing.
func (s *varcSanitizer) point(at int64) bool { return at >= 0 && at <= int64(len(s.t)) }

// rng is check_range: that n bytes from at are in the table, charged.
func (s *varcSanitizer) rng(at, n int64) bool {
	if at < 0 || at > int64(len(s.t)) || int64(len(s.t))-at < n {
		return false
	}
	s.ops -= n
	return s.ops > 0
}

func readVARC(t []byte) *varcTable {
	if len(t) < 24 {
		return nil
	}
	s := &varcSanitizer{t: t, ops: int64(max(min(sanitizeOpsFactor*int64(len(t)), sanitizeOpsMax), sanitizeOpsMin))}
	if font.Be16(t, 0) != 1 {
		return nil
	}
	v := &varcTable{t: t}
	off := func(at int) int { return int(font.Be32(t, at)) }
	v.coverage, v.store, v.conditions = off(4), off(8), off(12)
	axes, records := off(16), off(20)
	if v.coverage != 0 && !s.coverage(int64(v.coverage)) {
		return nil
	}
	if v.store != 0 && !s.store(int64(v.store)) {
		return nil
	}
	if v.conditions != 0 && !s.conditionList(int64(v.conditions)) {
		return nil
	}
	var ok bool
	if axes != 0 {
		if v.axes, ok = s.index(int64(axes)); !ok {
			return nil
		}
	}
	if records != 0 {
		if v.records, ok = s.index(int64(records)); !ok {
			return nil
		}
	}
	return v
}

// coverage is Coverage::sanitize, all four formats; a format it does not
// know is taken, and covers nothing.
func (s *varcSanitizer) coverage(at int64) bool {
	if !s.point(at + 2) {
		return false
	}
	t := s.t
	switch font.Be16(t, int(at)) {
	case 1:
		return s.point(at+4) && s.rng(at+4, 2*int64(font.Be16(t, int(at)+2)))
	case 2:
		return s.point(at+4) && s.rng(at+4, 6*int64(font.Be16(t, int(at)+2)))
	case 3:
		return s.point(at+5) && s.rng(at+5, 3*int64(be24(t, int(at)+2)))
	case 4:
		return s.point(at+5) && s.rng(at+5, 8*int64(be24(t, int(at)+2)))
	}
	return true
}

// store is MultiItemVariationStore::sanitize: format 1, a region list and
// the data sets, each inside the table.
func (s *varcSanitizer) store(at int64) bool {
	t := s.t
	if !s.point(at+8) || font.Be16(t, int(at)) != 1 {
		return false
	}
	if regions := int64(font.Be32(t, int(at)+2)); regions != 0 {
		base := at + regions
		// SparseVarRegionList: offsets to regions, each an array of eight-byte
		// axes.
		if !s.point(base+2) || !s.rng(base+2, 4*int64(font.Be16(t, int(base)))) {
			return false
		}
		for i := int64(0); i < int64(font.Be16(t, int(base))); i++ {
			if r := int64(font.Be32(t, int(base+2+4*i))); r != 0 {
				if !s.point(base+r+2) || !s.rng(base+r+2, 8*int64(font.Be16(t, int(base+r)))) {
					return false
				}
			}
		}
	}
	n := int64(font.Be16(t, int(at)+6))
	if !s.rng(at+8, 4*n) {
		return false
	}
	for i := int64(0); i < n; i++ {
		d := int64(font.Be32(t, int(at+8+4*i)))
		if d == 0 {
			continue
		}
		// MultiVarData: format 1, region indices, and an INDEX of deltas.
		base := at + d
		if !s.point(base+1) || t[base] != 1 || !s.point(base+3) ||
			!s.rng(base+3, 2*int64(font.Be16(t, int(base+1)))) {
			return false
		}
		if _, ok := s.index(base + 3 + 2*int64(font.Be16(t, int(base+1)))); !ok {
			return false
		}
	}
	return true
}

// conditionList is ConditionList::sanitize: offsets to conditions, each read
// as Condition::sanitize reads it.
func (s *varcSanitizer) conditionList(at int64) bool {
	t := s.t
	if !s.point(at + 4) {
		return false
	}
	n := int64(font.Be32(t, int(at)))
	if !s.rng(at+4, 4*n) {
		return false
	}
	for i := int64(0); i < n; i++ {
		if c := int64(font.Be32(t, int(at+4+4*i))); c != 0 && !s.condition(at+c) {
			return false
		}
	}
	return true
}

// condition is Condition::sanitize, nested no deeper than HarfBuzz nests it.
func (s *varcSanitizer) condition(at int64) bool {
	if s.depth >= hbMaxNesting {
		return false
	}
	s.depth++
	defer func() { s.depth-- }()
	t := s.t
	if !s.point(at + 2) {
		return false
	}
	switch font.Be16(t, int(at)) {
	case 1, 2:
		return s.point(at + 8)
	case 3, 4:
		if !s.point(at + 3) {
			return false
		}
		n := int64(t[at+2])
		if !s.rng(at+3, 3*n) {
			return false
		}
		for i := int64(0); i < n; i++ {
			if c := int64(be24(t, int(at+3+3*i))); c != 0 && !s.condition(at+c) {
				return false
			}
		}
		return true
	case 5:
		if !s.point(at + 5) {
			return false
		}
		if c := int64(be24(t, int(at+2))); c != 0 {
			return s.condition(at + c)
		}
	}
	return true
}

// index is CFFIndex<HBUINT32>::sanitize.
func (s *varcSanitizer) index(at int64) (varcIndex, bool) {
	t := s.t
	if !s.point(at + 4) {
		return varcIndex{}, false
	}
	count := int64(font.Be32(t, int(at)))
	if count == 0 {
		return varcIndex{}, true
	}
	if count == math.MaxUint32 || !s.point(at+5) {
		return varcIndex{}, false
	}
	size := int64(t[at+4])
	if size < 1 || size > 4 || !s.rng(at+5, size*(count+1)) {
		return varcIndex{}, false
	}
	x := varcIndex{count: int(count), offSize: int(size), offsets: int(at + 5), data: int(at + 4 + size*(count+1))}
	if !s.rng(int64(x.data), int64(x.offsetAt(t, x.count))) {
		return varcIndex{}, false
	}
	return x, true
}

func (x varcIndex) offsetAt(t []byte, i int) int {
	at := x.offsets + x.offSize*i
	switch x.offSize {
	case 1:
		return int(t[at])
	case 2:
		return font.Be16(t, at)
	case 3:
		return be24(t, at)
	case 4:
		return int(font.Be32(t, at))
	}
	return 0
}

// item is CFFIndex::operator[]: the bytes of one entry, and none for an
// entry the index does not have or whose offsets run backwards or past the
// last.
func (x varcIndex) item(t []byte, i int) []byte {
	if i < 0 || i >= x.count {
		return nil
	}
	o0, o1 := x.offsetAt(t, i), x.offsetAt(t, i+1)
	if o1 < o0 || o1 > x.offsetAt(t, x.count) {
		return nil
	}
	return t[x.data+o0 : x.data+o1]
}

func be24(b []byte, at int) int { return int(b[at])<<16 | int(b[at+1])<<8 | int(b[at+2]) }

// coverageIndex is Coverage::get_coverage for the four formats, and -1 for a
// glyph the coverage does not have.
func (v *varcTable) coverageIndex(gid int) int {
	if v.coverage == 0 {
		return -1
	}
	t, at := v.t, v.coverage
	switch font.Be16(t, at) {
	case 1, 2:
		if i, ok := coverageIndex(t, at, gid); ok {
			return i
		}
	case 3:
		lo, hi := 0, be24(t, at+2)-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			g := be24(t, at+5+3*mid)
			switch {
			case gid < g:
				hi = mid - 1
			case gid > g:
				lo = mid + 1
			default:
				return mid
			}
		}
	case 4:
		lo, hi := 0, be24(t, at+2)-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			rec := at + 5 + 8*mid
			first, last := be24(t, rec), be24(t, rec+3)
			switch {
			case gid < first:
				hi = mid - 1
			case gid > last:
				lo = mid + 1
			default:
				return font.Be16(t, rec+6) + (gid - first)
			}
		}
	}
	return -1
}

// uint32Var is HBUINT32VAR: a number in one to five bytes, its length told by
// the leading bits of the first, and how many bytes it took; zero where the
// bytes run out.
func uint32Var(b []byte) (uint32, int) {
	if len(b) == 0 {
		return 0, 0
	}
	b0 := uint32(b[0])
	n := 1
	switch {
	case b0 < 0x80:
	case b0 < 0xC0:
		n = 2
	case b0 < 0xE0:
		n = 3
	case b0 < 0xF0:
		n = 4
	default:
		n = 5
	}
	if len(b) < n {
		return 0, 0
	}
	switch n {
	case 1:
		return b0, 1
	case 2:
		return (b0&0x3F)<<8 | uint32(b[1]), 2
	case 3:
		return (b0&0x1F)<<16 | uint32(b[1])<<8 | uint32(b[2]), 3
	case 4:
		return (b0&0x0F)<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), 4
	}
	return uint32(b[1])<<24 | uint32(b[2])<<16 | uint32(b[3])<<8 | uint32(b[4]), 5
}

// The flags of a VarComponent.
const (
	varcResetUnspecifiedAxes    = 1 << 0
	varcHaveAxes                = 1 << 1
	varcAxisValuesHaveVariation = 1 << 2
	varcTransformHasVariation   = 1 << 3
	varcHaveTranslateX          = 1 << 4
	varcHaveTranslateY          = 1 << 5
	varcHaveRotation            = 1 << 6
	varcHaveCondition           = 1 << 7
	varcHaveScaleX              = 1 << 8
	varcHaveScaleY              = 1 << 9
	varcHaveTCenterX            = 1 << 10
	varcHaveTCenterY            = 1 << 11
	varcGIDIs24Bit              = 1 << 12
	varcHaveSkewX               = 1 << 13
	varcHaveSkewY               = 1 << 14
	varcReservedMask            = ^uint32(1<<15 - 1)
)

// varcTransformFields are the nine transform fields in the order a record
// states them, each with its flag and how many of its bits are a fraction.
var varcTransformFields = [9]struct {
	flag  uint32
	shift int
}{
	{varcHaveTranslateX, 0}, {varcHaveTranslateY, 0}, {varcHaveRotation, 12},
	{varcHaveScaleX, 10}, {varcHaveScaleY, 10}, {varcHaveSkewX, 12},
	{varcHaveSkewY, 12}, {varcHaveTCenterX, 0}, {varcHaveTCenterY, 0},
}

// varcComponent is one VarComponent record decoded.
type varcComponent struct {
	flags         uint32
	gid           int
	condition     uint32
	axisIndices   []int
	axisValues    []float32
	axisValuesVar uint32
	transformVar  uint32
	transform     [9]float32 // translateX, translateY, rotation, scaleX, scaleY, skewX, skewY, tCenterX, tCenterY
	size          int
}

// decodeComponent is VarComponent::decompile_record, and false where the
// record does not decode.
func (v *varcTable) decodeComponent(rec []byte) (varcComponent, bool) {
	c := varcComponent{axisValuesVar: varcNoVariation, transformVar: varcNoVariation}
	c.transform = [9]float32{0, 0, 0, 1, 1, 0, 0, 0, 0}
	at := 0
	read := func() (uint32, bool) {
		if at >= len(rec) {
			return 0, false
		}
		x, n := uint32Var(rec[at:])
		if n == 0 {
			return 0, false
		}
		at += n
		return x, true
	}
	var ok bool
	if c.flags, ok = read(); !ok {
		return c, false
	}
	if c.flags&varcGIDIs24Bit != 0 {
		if len(rec)-at < 3 {
			return c, false
		}
		c.gid, at = be24(rec, at), at+3
	} else {
		if len(rec)-at < 2 {
			return c, false
		}
		c.gid, at = font.Be16(rec, at), at+2
	}
	if c.flags&varcHaveCondition != 0 {
		if c.condition, ok = read(); !ok {
			return c, false
		}
	}
	if c.flags&varcHaveAxes != 0 {
		idx, ok := read()
		if !ok {
			return c, false
		}
		c.axisIndices = tupleListItem(v.t, v.axes, int(idx))
		c.axisValues = make([]float32, len(c.axisIndices))
		n, ok := decompileTupleValues(rec[at:], c.axisValues)
		if !ok {
			return c, false
		}
		at += n
	}
	if c.flags&varcAxisValuesHaveVariation != 0 {
		if c.axisValuesVar, ok = read(); !ok {
			return c, false
		}
	}
	if c.flags&varcTransformHasVariation != 0 {
		if c.transformVar, ok = read(); !ok {
			return c, false
		}
	}
	for i, f := range varcTransformFields {
		if c.flags&f.flag == 0 {
			continue
		}
		if len(rec)-at < 2 {
			return c, false
		}
		c.transform[i] = float32(signed16(font.Be16(rec, at)))
		at += 2
	}
	for reserved := c.flags & varcReservedMask; reserved != 0; reserved &= reserved - 1 {
		if _, ok := read(); !ok {
			return c, false
		}
	}
	c.size = at
	return c, true
}

// tupleListItem is one entry of a TupleList read as TupleValues::iter_t
// reads it, malformed runs and all: the values it yields.
func tupleListItem(t []byte, x varcIndex, i int) []int {
	b := x.item(t, i)
	var out []int
	it := tupleIter{b: b}
	if it.ensureRun() {
		it.readValue()
	}
	for it.more() {
		out = append(out, it.current)
		it.next()
	}
	return out
}

// tupleIter is TupleValues::iter_t.
type tupleIter struct {
	b        []byte
	p        int
	current  int
	runCount int
	width    int
}

func (it *tupleIter) ensureRun() bool {
	if it.runCount > 0 {
		return true
	}
	if it.p >= len(it.b) {
		it.runCount, it.current = 0, 0
		return false
	}
	control := it.b[it.p]
	it.p++
	it.runCount = int(control&0x3F) + 1
	it.width = tupleWidth(control)
	if it.p+it.runCount*it.width > len(it.b) {
		it.runCount, it.current = 0, 0
		return false
	}
	return true
}

func (it *tupleIter) readValue() {
	it.current = tupleValue(it.b, it.p, it.width)
	it.p += it.width
}

func (it *tupleIter) more() bool { return it.runCount != 0 || it.p < len(it.b) }

func (it *tupleIter) next() {
	it.runCount--
	if !it.ensureRun() {
		return
	}
	it.readValue()
}

func tupleWidth(control byte) int {
	switch control & 0xC0 {
	case 0x80:
		return 0
	case 0x00:
		return 1
	case 0x40:
		return 2
	}
	return 4
}

func tupleValue(b []byte, at, width int) int {
	switch width {
	case 1:
		return int(int8(b[at]))
	case 2:
		return signed16(font.Be16(b, at))
	case 4:
		return int(int32(font.Be32(b, at)))
	}
	return 0
}

// decompileTupleValues is TupleValues::decompile for as many values as out
// holds, and how many bytes they took; false where the runs do not fill it
// exactly or run past the bytes.
func decompileTupleValues(b []byte, out []float32) (int, bool) {
	p, i := 0, 0
	for i < len(out) {
		if p+1 > len(b) {
			return p, false
		}
		control := b[p]
		p++
		run := int(control&0x3F) + 1
		stop := i + run
		if stop > len(out) {
			return p, false
		}
		width := tupleWidth(control)
		if p+run*width > len(b) {
			return p, false
		}
		for ; i < stop; i++ {
			out[i] = float32(tupleValue(b, p, width))
			p += width
		}
	}
	return p, true
}

// tupleFetcher is TupleValues::fetcher_t, which a store's deltas are read
// through: one region's row after another, skipped where the region's scalar
// is zero.
type tupleFetcher struct {
	b        []byte
	p        int
	runCount int
	width    int
}

func (f *tupleFetcher) ensureRun() bool {
	if f.runCount > 0 {
		return true
	}
	if f.p >= len(f.b) {
		f.runCount = 0
		return false
	}
	control := f.b[f.p]
	f.p++
	f.runCount = int(control&0x3F) + 1
	f.width = tupleWidth(control)
	if f.p+f.runCount*f.width > len(f.b) {
		f.runCount = 0
		return false
	}
	return true
}

func (f *tupleFetcher) skip(n int) {
	for n > 0 {
		if !f.ensureRun() {
			return
		}
		i := min(n, f.runCount)
		f.runCount -= i
		n -= i
		f.p += i * f.width
	}
}

func (f *tupleFetcher) addTo(out []float32, scale float32) {
	for i := 0; i < len(out); {
		if !f.ensureRun() {
			break
		}
		count := min(len(out)-i, f.runCount)
		if f.width != 0 {
			for j := 0; j < count; j++ {
				out[i+j] = float32(out[i+j] + float32(float32(tupleValue(f.b, f.p, f.width))*scale))
				f.p += f.width
			}
		}
		f.runCount -= count
		i += count
	}
}

// delta is MultiItemVariationStore::get_delta: each value of out moved by
// the store's row for varIdx at the coordinates.
func (v *varcTable) delta(varIdx uint32, coords []int, out []float32) {
	if v.store == 0 {
		return
	}
	t, at := v.t, v.store
	outer, inner := int(varIdx>>16), int(varIdx&0xFFFF)
	if outer >= font.Be16(t, at+6) {
		return
	}
	d := int(font.Be32(t, at+8+4*outer))
	if d == 0 {
		return
	}
	data := at + d
	regionCount := font.Be16(t, data+1)
	deltas, _ := (&varcSanitizer{t: t, ops: math.MaxInt64}).index(int64(data + 3 + 2*regionCount))
	f := tupleFetcher{b: deltas.item(t, inner)}
	skip := 0
	for r := 0; r < regionCount; r++ {
		scalar := v.regionScalar(font.Be16(t, data+3+2*r), coords)
		if scalar == 0 {
			skip += len(out)
			continue
		}
		if skip != 0 {
			f.skip(skip)
			skip = 0
		}
		f.addTo(out, scalar)
	}
}

// regionScalar is SparseVarRegionList::evaluate: the product of the region's
// axes' scalars, and 1 for a region with no axes — a null region among them.
func (v *varcTable) regionScalar(region int, coords []int) float32 {
	t := v.t
	list := int(font.Be32(t, v.store+2))
	if list == 0 {
		return 0
	}
	base := v.store + list
	if region >= font.Be16(t, base) {
		return 0
	}
	r := int(font.Be32(t, base+2+4*region))
	if r == 0 {
		return 1
	}
	r += base
	s := float32(1)
	for i := 0; i < font.Be16(t, r); i++ {
		axis := r + 2 + 8*i
		ai := font.Be16(t, axis)
		coord := 0
		if ai < len(coords) {
			coord = coords[ai]
		}
		factor := regionAxisScalar(signed16(font.Be16(t, axis+2)), signed16(font.Be16(t, axis+4)),
			signed16(font.Be16(t, axis+6)), coord)
		if factor == 0 {
			return 0
		}
		s = float32(s * factor)
	}
	return s
}

// regionAxisScalar is VarRegionAxis::evaluate on 2.14 integers.
func regionAxisScalar(start, peak, end, coord int) float32 {
	switch {
	case peak == 0 || coord == peak:
		return 1
	case coord == 0:
		return 0
	case start > peak || peak > end:
		return 1
	case start < 0 && end > 0:
		return 1
	case coord <= start || end <= coord:
		return 0
	case coord < peak:
		return float32(coord-start) / float32(peak-start)
	}
	return float32(end-coord) / float32(end-peak)
}

// condition is Condition::evaluate for the condition list's entry i, and
// false for an entry the list does not have.
func (v *varcTable) condition(i uint32, coords []int) bool {
	if v.conditions == 0 || int64(i) >= int64(font.Be32(v.t, v.conditions)) {
		return false
	}
	off := int(font.Be32(v.t, v.conditions+4+4*int(i)))
	if off == 0 {
		return false
	}
	return v.evalCondition(v.conditions+off, coords)
}

func (v *varcTable) evalCondition(at int, coords []int) bool {
	t := v.t
	switch font.Be16(t, at) {
	case 1:
		ai := font.Be16(t, at+2)
		coord := 0
		if ai < len(coords) {
			coord = coords[ai]
		}
		return signed16(font.Be16(t, at+4)) <= coord && coord <= signed16(font.Be16(t, at+6))
	case 2:
		value := float32(signed16(font.Be16(t, at+2)))
		if idx := font.Be32(t, at+4); len(coords) > 0 && idx != varcNoVariation {
			d := []float32{0}
			v.delta(idx, coords, d)
			value = float32(value + d[0])
		}
		return value > 0
	case 3, 4:
		and := font.Be16(t, at) == 3
		for k := 0; k < int(t[at+2]); k++ {
			off := be24(t, at+3+3*k)
			holds := off != 0 && v.evalCondition(at+off, coords)
			if and && !holds {
				return false
			}
			if !and && holds {
				return true
			}
		}
		return and
	case 5:
		off := be24(t, at+2)
		return !(off != 0 && v.evalCondition(at+off, coords))
	}
	return false
}

// toTransform is hb_transform_decomposed_t::to_transform, in single
// precision: translate by the translation and the centre, rotate, scale,
// skew, and translate back from the centre.
func varcTransform(f [9]float32) xform32 {
	tx, ty, rot, sx, sy, kx, ky, cx, cy := f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8]
	t := identity32
	t.translate(float32(tx+cx), float32(ty+cy))
	if rot != 0 {
		s, c := float32(math.Sin(float64(rot))), float32(math.Cos(float64(rot)))
		t = t.then(xform32{c, s, -s, c, 0, 0})
	}
	if sx != 1 || sy != 1 {
		t.xx, t.yx = float32(t.xx*sx), float32(t.yx*sx)
		t.xy, t.yy = float32(t.xy*sy), float32(t.yy*sy)
	}
	if kx != 0 || ky != 0 {
		// skew (-skewX, skewY): hb_transform_t::skewing puts tan(skewY)
		// below the diagonal and tan(-skewX) above it.
		var a, b float32
		if ky != 0 {
			b = float32(math.Tan(float64(ky)))
		}
		if -kx != 0 {
			a = float32(math.Tan(float64(-kx)))
		}
		t = t.then(xform32{1, b, a, 1, 0, 0})
	}
	t.translate(-cx, -cy)
	return t
}

// translate is hb_transform_t::translate, after the transform: nothing for
// no translation, since HarfBuzz returns before adding a zero.
func (t *xform32) translate(x, y float32) {
	if x == 0 && y == 0 {
		return
	}
	t.x0 = float32(t.x0 + float32(float32(t.xx*x)+float32(t.xy*y)))
	t.y0 = float32(t.y0 + float32(float32(t.yx*x)+float32(t.yy*y)))
}
