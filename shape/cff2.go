package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/mgilbir/forme/font"
)

// Reading a CFF2 table: the outlines of an OpenType variable font whose
// charstrings blend their own variations.
//
// CFF2 is CFF rebuilt for variation. What a CFF says once per font — the name,
// the strings, the charset, the encoding — is gone, since the sfnt around it
// says all of that; the INDEXes count in 32 bits; there is one Top DICT,
// written straight after the header rather than in an INDEX of its own; and
// every glyph is drawn through a Font DICT, whether the font is CJK or not.
// And a charstring may say where its numbers move to across the design space:
// the blend operator takes a default and a delta per region of the design
// space for each of the numbers before it, and leaves in their place the
// numbers at the location being drawn. The regions are the font's item
// variation store (varstore.go), which the Top DICT names; vsindex says which
// of its groups of regions a glyph's blends are written against.
//
// What is read here is what drawing a glyph needs: the charstrings, the global
// and local subroutines, which Font DICT each glyph is in, each Private DICT's
// entries and its vsindex, the variation store, and the Top DICT's FontMatrix.
// It is read once and whole, and refused when any of it does not lie where it
// says it does: every count and offset is a number in the file, and each is
// checked against the table before anything is allocated for it.
//
// # What drawing a glyph means here
//
// The charstrings are run by the Type 2 interpreter in cffink.go, which is
// HarfBuzz's cff1 and cff2 interpreters for the parts that decide where a glyph
// draws; its blend method and its fetch are the two operators CFF2 adds and
// the one way its charstrings end differently. A blend is resolved, by
// cff2Blend below, in one of two ways:
//
//   - as HarfBuzz resolves it when it draws a variable font — each region's
//     scalar a float at the location, each value its default plus the deltas
//     weighed by them, in doubles, never rounded — which is what the tests
//     hold the reader to, at several locations, against HarfBuzz itself, and
//     which nothing here draws with;
//   - or as fontTools' instancer resolves it when it cuts a static font — the
//     deltas weighed and summed as its tuple arithmetic does, and the sum
//     rounded to a whole number before it is added to the default — which is
//     what an instance is cut with (cff2cff.go).
//
// At the default instance the two are the same: nothing is blended.

// The CFF2 Top DICT and Private DICT operators this reads.
const (
	opVstore     = 24   // Top DICT: the item variation store
	opFontMatrix = 1207 // Top DICT
	opVsindex    = 22   // Private DICT: the group of regions its blends use
	opBlend      = 23   // Private DICT: blended operands
)

// maxCFF2FontDicts bounds the FDArray. A CFF (CFF1) FDSelect names a Font DICT
// in a byte, and a CFF2 font becomes one to be embedded (cff2cff.go), so a
// font with more than this many could not be written as one. The fonts that
// exist have one, or for a CJK font a few dozen.
const maxCFF2FontDicts = 256

// cff2Font is a CFF2 table read for what drawing its glyphs needs.
type cff2Font struct {
	charStrings [][]byte
	global      [][]byte
	// fds is the Font DICT of each glyph; each Font DICT's local subroutines,
	// the vsindex its Private DICT states, and the Private DICT's entries.
	fds      []int
	locals   [][][]byte
	ivs      []int
	privates [][]cff2Entry
	// store is the item variation store, nil where the font has none: a
	// font whose blends then leave every default as it is.
	store *varStore
	// fontMatrix is the Top DICT's FontMatrix entry as the font wrote it,
	// operands and operator, or nil where it states none.
	fontMatrix []byte
}

// cff2Entry is one Private DICT entry: its operator and its operands, each
// with the deltas a blend gave it, and its bytes as written when nothing is
// blended.
type cff2Entry struct {
	op       int
	operands []cff2Operand
	raw      []byte
	blended  bool
}

// cff2Operand is a DICT operand: its value at the default instance and, when
// a blend gave it any, a delta for each region of its vsindex's group.
type cff2Operand struct {
	v      float64
	deltas []float64
}

// readCFF2Index parses the CFF2 INDEX at off: the same as a CFF INDEX but for
// a count of four bytes.
func readCFF2Index(b []byte, off int) (*cffIndex, error) {
	if off < 0 || off > len(b)-4 {
		return nil, fmt.Errorf("fonts: CFF2 INDEX at %d is outside the font", off)
	}
	count := binary.BigEndian.Uint32(b[off:])
	if count == 0 {
		return &cffIndex{end: off + 4}, nil
	}
	if count > maxCFFIndexItems {
		return nil, fmt.Errorf("fonts: CFF2 INDEX declares %d items", count)
	}
	n := int(count)
	if off+5 > len(b) {
		return nil, errors.New("fonts: truncated CFF2 INDEX header")
	}
	offSize := int(b[off+4])
	if offSize < 1 || offSize > 4 {
		return nil, fmt.Errorf("fonts: CFF2 INDEX offset size %d is not 1..4", offSize)
	}
	offArray := off + 5
	if (n+1)*offSize > len(b)-offArray {
		return nil, errors.New("fonts: truncated CFF2 INDEX offset array")
	}
	dataStart := offArray + (n+1)*offSize - 1
	read := func(i int) int {
		p := offArray + i*offSize
		v := 0
		for k := 0; k < offSize; k++ {
			v = v<<8 | int(b[p+k])
		}
		return v
	}
	idx := &cffIndex{items: make([][]byte, 0, n)}
	for i := 0; i < n; i++ {
		start, end := read(i), read(i+1)
		if start < 1 || end < start || end > len(b)-dataStart {
			return nil, fmt.Errorf("fonts: CFF2 INDEX item %d lies outside the font", i)
		}
		idx.items = append(idx.items, b[dataStart+start:dataStart+end])
	}
	idx.end = dataStart + read(n)
	return idx, nil
}

// cff2DictEntry is a DICT entry as parseCFF2Dict reads it.
type cff2DictEntry struct {
	op       int
	operands []cff2Operand
	raw      []byte
	blended  bool
}

// parseCFF2Dict reads a CFF2 DICT. Beside CFF's operators it has vsindex and
// blend, which only a Private DICT uses: vsindex names the group of regions
// the blends after it are written against, and blend replaces the n defaults,
// n·k deltas and the count before it with n operands carrying their deltas.
// regions says how many regions a group has; a vsindex naming a group the
// store does not have, or a blend before a vsindex in a DICT whose default
// group is not there, blends against none.
//
// The operands are kept as numbers here, reals included, because a blended
// value has to be computed; an entry nothing blended is re-emitted from its
// own bytes, so what was written is what is kept.
func parseCFF2Dict(b []byte, regions func(ivs int) int) ([]cff2DictEntry, int, error) {
	var (
		entries  []cff2DictEntry
		operands []cff2Operand
		start    int
		blended  bool
		ivs      int
	)
	for i := 0; i < len(b); {
		v := int(b[i])
		switch {
		case v == 28:
			if i+3 > len(b) {
				return nil, 0, errors.New("fonts: truncated CFF2 DICT operand")
			}
			operands = append(operands, cff2Operand{v: float64(int16(binary.BigEndian.Uint16(b[i+1:])))})
			i += 3
		case v == 29:
			if i+5 > len(b) {
				return nil, 0, errors.New("fonts: truncated CFF2 DICT operand")
			}
			operands = append(operands, cff2Operand{v: float64(int32(binary.BigEndian.Uint32(b[i+1:])))})
			i += 5
		case v == 30:
			r, n, err := parseCFFReal(b[i+1:])
			if err != nil {
				return nil, 0, err
			}
			operands = append(operands, cff2Operand{v: r})
			i += 1 + n
		case v >= 32 && v <= 246:
			operands = append(operands, cff2Operand{v: float64(v - 139)})
			i++
		case v >= 247 && v <= 250:
			if i+2 > len(b) {
				return nil, 0, errors.New("fonts: truncated CFF2 DICT operand")
			}
			operands = append(operands, cff2Operand{v: float64((v-247)*256 + int(b[i+1]) + 108)})
			i += 2
		case v >= 251 && v <= 254:
			if i+2 > len(b) {
				return nil, 0, errors.New("fonts: truncated CFF2 DICT operand")
			}
			operands = append(operands, cff2Operand{v: float64(-(v-251)*256 - int(b[i+1]) - 108)})
			i += 2
		case v <= 24:
			op, n := v, 1
			if v == 12 {
				if i+1 >= len(b) {
					return nil, 0, errors.New("fonts: truncated CFF2 DICT operator")
				}
				op, n = 1200+int(b[i+1]), 2
			}
			i += n
			switch op {
			case opBlend:
				if len(operands) == 0 {
					return nil, 0, errors.New("fonts: a CFF2 DICT blend with no count")
				}
				count := operands[len(operands)-1].v
				operands = operands[:len(operands)-1]
				k := regions(ivs)
				if count < 0 || count != math.Trunc(count) || count*float64(k+1) > float64(len(operands)) {
					return nil, 0, errors.New("fonts: a CFF2 DICT blend asks for more operands than it has")
				}
				n := int(count)
				base := len(operands) - n*(k+1)
				for j := 0; j < n; j++ {
					d := make([]float64, k)
					for r := range d {
						d[r] = operands[base+n+j*k+r].v
					}
					operands[base+j].deltas = d
				}
				operands = operands[:base+n]
				blended = true
				continue // the operands stay for the operator they belong to
			case opVsindex:
				if len(operands) != 1 || operands[0].v < 0 || operands[0].v != math.Trunc(operands[0].v) ||
					operands[0].v > math.MaxUint16 {
					return nil, 0, errors.New("fonts: a CFF2 DICT vsindex that names no group")
				}
				ivs = int(operands[0].v)
			}
			entries = append(entries, cff2DictEntry{op: op, operands: operands, raw: b[start:i], blended: blended})
			operands, blended, start = nil, false, i
		default:
			return nil, 0, fmt.Errorf("fonts: reserved byte %d in a CFF2 DICT", v)
		}
	}
	if len(operands) > 0 {
		return nil, 0, errors.New("fonts: a CFF2 DICT ends in operands with no operator")
	}
	return entries, ivs, nil
}

// parseCFFReal reads the binary-coded decimal a DICT writes a real number in,
// b starting after its 30, and how many bytes it took. The nibbles spell the
// number: digits, a point, an exponent, a minus, and an end — nothing else, so
// what is handed to ParseFloat is a decimal number and cannot be an infinity or
// a NaN, which ParseFloat would otherwise read from a spelling of its own.
func parseCFFReal(b []byte) (float64, int, error) {
	var s []byte
	for i := 0; i < len(b); i++ {
		for _, nib := range [2]byte{b[i] >> 4, b[i] & 0xF} {
			switch {
			case nib <= 9:
				s = append(s, '0'+nib)
			case nib == 0xA:
				s = append(s, '.')
			case nib == 0xB:
				s = append(s, 'E')
			case nib == 0xC:
				s = append(s, 'E', '-')
			case nib == 0xE:
				s = append(s, '-')
			case nib == 0xF:
				if len(s) == 0 {
					return 0, 0, errors.New("fonts: an empty real in a CFF DICT")
				}
				v, err := strconv.ParseFloat(string(s), 64)
				if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
					return 0, 0, fmt.Errorf("fonts: %q is not a real a CFF DICT can hold", s)
				}
				return v, i + 1, nil
			default:
				return 0, 0, errors.New("fonts: a reserved nibble in a CFF DICT real")
			}
			if len(s) > 64 {
				return 0, 0, errors.New("fonts: a CFF DICT real longer than any number needs")
			}
		}
	}
	return 0, 0, errors.New("fonts: a truncated real in a CFF DICT")
}

// readCFF2 reads a CFF2 table whose font has numGlyphs glyphs. What reading
// its structures costs is charged to budget.
func readCFF2(t []byte, numGlyphs int, budget *font.Budget) (*cff2Font, error) {
	if len(t) > maxCFFSize {
		return nil, fmt.Errorf("fonts: CFF2 table of %d bytes is too large", len(t))
	}
	if len(t) < 5 {
		return nil, errors.New("fonts: the CFF2 table is too short to hold its header")
	}
	if t[0] != 2 {
		return nil, fmt.Errorf("fonts: the CFF2 table is version %d", t[0])
	}
	hdrSize := int(t[2])
	topLen := int(binary.BigEndian.Uint16(t[3:]))
	if hdrSize < 5 || topLen > len(t)-hdrSize {
		return nil, errors.New("fonts: the CFF2 Top DICT lies outside the table")
	}
	noRegions := func(int) int { return 0 }
	top, _, err := parseCFF2Dict(t[hdrSize:hdrSize+topLen], noRegions)
	if err != nil {
		return nil, err
	}
	f := &cff2Font{}
	offset := func(e cff2DictEntry) (int, error) {
		if len(e.operands) != 1 || e.blended {
			return 0, errors.New("fonts: a CFF2 Top DICT offset that is not one number")
		}
		v := e.operands[0].v
		if v <= 0 || v >= float64(len(t)) || v != math.Trunc(v) {
			return 0, errors.New("fonts: a CFF2 Top DICT offset outside the table")
		}
		return int(v), nil
	}
	var csOff, fdArrayOff, fdSelectOff, vstoreOff int
	for _, e := range top {
		var err error
		switch e.op {
		case opCharStrings:
			csOff, err = offset(e)
		case opFDArray:
			fdArrayOff, err = offset(e)
		case opFDSelect:
			fdSelectOff, err = offset(e)
		case opVstore:
			vstoreOff, err = offset(e)
		case opFontMatrix:
			f.fontMatrix = e.raw
		}
		if err != nil {
			return nil, err
		}
	}
	gidx, err := readCFF2Index(t, hdrSize+topLen)
	if err != nil {
		return nil, fmt.Errorf("fonts: the CFF2 global subroutines: %w", err)
	}
	f.global = gidx.items
	if csOff == 0 {
		return nil, errors.New("fonts: the CFF2 Top DICT names no CharStrings")
	}
	cs, err := readCFF2Index(t, csOff)
	if err != nil {
		return nil, fmt.Errorf("fonts: the CFF2 CharStrings: %w", err)
	}
	if len(cs.items) != numGlyphs {
		return nil, fmt.Errorf("fonts: the CFF2 table holds %d charstrings and the font %d glyphs",
			len(cs.items), numGlyphs)
	}
	f.charStrings = cs.items
	if !budget.Charge(len(cs.items)+len(gidx.items), "the CFF2 INDEXes") {
		return nil, budget.Err()
	}
	if vstoreOff != 0 {
		if vstoreOff > len(t)-2 {
			return nil, errors.New("fonts: the CFF2 variation store lies outside the table")
		}
		n := int(binary.BigEndian.Uint16(t[vstoreOff:]))
		if n > len(t)-vstoreOff-2 {
			return nil, errors.New("fonts: the CFF2 variation store runs past the table")
		}
		if n > 0 {
			if f.store, err = parseVarStore(t[vstoreOff+2 : vstoreOff+2+n]); err != nil {
				return nil, fmt.Errorf("fonts: the CFF2 variation store: %w", err)
			}
		}
	}
	regions := func(ivs int) int {
		if f.store == nil || ivs >= len(f.store.data) {
			return 0
		}
		return len(f.store.data[ivs].regions)
	}

	if fdArrayOff == 0 {
		return nil, errors.New("fonts: the CFF2 Top DICT names no FDArray")
	}
	fdIndex, err := readCFF2Index(t, fdArrayOff)
	if err != nil {
		return nil, fmt.Errorf("fonts: the CFF2 FDArray: %w", err)
	}
	nFD := len(fdIndex.items)
	if nFD == 0 || nFD > maxCFF2FontDicts {
		return nil, fmt.Errorf("fonts: the CFF2 FDArray holds %d Font DICTs, and a font needs 1 to %d",
			nFD, maxCFF2FontDicts)
	}
	for _, fd := range fdIndex.items {
		if !budget.Charge(len(fd), "the CFF2 Font DICTs") {
			return nil, budget.Err()
		}
		ops, _, err := parseCFF2Dict(fd, noRegions)
		if err != nil {
			return nil, err
		}
		size, off := 0, 0
		for _, e := range ops {
			if e.op != opPrivate {
				continue
			}
			if len(e.operands) != 2 || e.blended {
				return nil, errors.New("fonts: a CFF2 Font DICT whose Private entry is not a size and an offset")
			}
			sz, of := e.operands[0].v, e.operands[1].v
			if sz < 0 || of < 0 || sz != math.Trunc(sz) || of != math.Trunc(of) ||
				of > float64(len(t)) || sz > float64(len(t))-of {
				return nil, errors.New("fonts: a CFF2 Private DICT lies outside the table")
			}
			size, off = int(sz), int(of)
		}
		if !budget.Charge(size, "a CFF2 Private DICT") {
			return nil, budget.Err()
		}
		entries, ivs, err := parseCFF2Dict(t[off:off+size], regions)
		if err != nil {
			return nil, err
		}
		var subrs [][]byte
		var priv []cff2Entry
		for _, e := range entries {
			if e.op == opSubrs {
				if len(e.operands) != 1 || e.blended || e.operands[0].v <= 0 ||
					e.operands[0].v != math.Trunc(e.operands[0].v) || e.operands[0].v > float64(len(t)-off) {
					return nil, errors.New("fonts: a CFF2 Private DICT names local subroutines outside the table")
				}
				idx, err := readCFF2Index(t, off+int(e.operands[0].v))
				if err != nil {
					return nil, fmt.Errorf("fonts: a CFF2 Private DICT's local subroutines: %w", err)
				}
				if !budget.Charge(len(idx.items), "a CFF2 local subroutine INDEX") {
					return nil, budget.Err()
				}
				subrs = idx.items
				continue
			}
			priv = append(priv, cff2Entry(e))
		}
		f.locals = append(f.locals, subrs)
		f.ivs = append(f.ivs, ivs)
		f.privates = append(f.privates, priv)
	}
	if f.fds, err = readCFF2FDSelect(t, fdSelectOff, numGlyphs, nFD); err != nil {
		return nil, err
	}
	return f, nil
}

// readCFF2FDSelect is the Font DICT of each glyph. A font with no FDSelect
// draws every glyph with the first Font DICT, as HarfBuzz reads one; format 0
// is a byte a glyph, 3 and 4 ranges of glyphs, and 4 the same in wider fields.
// Every Font DICT named has to be one the FDArray holds, and the ranges have to
// start at the first glyph, rise, and end at a sentinel that is the glyph
// count.
func readCFF2FDSelect(t []byte, off, numGlyphs, nFD int) ([]int, error) {
	fds := make([]int, numGlyphs)
	if off == 0 {
		return fds, nil
	}
	bad := errors.New("fonts: a CFF2 FDSelect that does not give every glyph a Font DICT the font has")
	b := t[off:]
	switch b[0] {
	case 0:
		if len(b) < 1+numGlyphs {
			return nil, bad
		}
		for g := range fds {
			if fds[g] = int(b[1+g]); fds[g] >= nFD {
				return nil, bad
			}
		}
		return fds, nil
	case 3, 4:
		wide := b[0] == 4
		countSize, firstSize, fdSize := 2, 2, 1
		if wide {
			countSize, firstSize, fdSize = 4, 4, 2
		}
		if len(b) < 1+countSize {
			return nil, bad
		}
		read := func(at, size int) int {
			v := 0
			for k := 0; k < size; k++ {
				v = v<<8 | int(b[at+k])
			}
			return v
		}
		ranges := read(1, countSize)
		rec := firstSize + fdSize
		if ranges == 0 || ranges > numGlyphs || rec*ranges+firstSize > len(b)-1-countSize {
			return nil, bad
		}
		at := 1 + countSize
		for i := 0; i < ranges; i++ {
			first := read(at, firstSize)
			fd := read(at+firstSize, fdSize)
			next := read(at+rec, firstSize) // the next range's first, or the sentinel
			if (i == 0 && first != 0) || next <= first || next > numGlyphs || fd >= nFD ||
				(i == ranges-1 && next != numGlyphs) {
				return nil, bad
			}
			for g := first; g < next; g++ {
				fds[g] = fd
			}
			at += rec
		}
		return fds, nil
	}
	return nil, fmt.Errorf("fonts: CFF2 FDSelect format %d is not one this reads", b[0])
}

// outlines is the font as the Type 2 interpreter reads it: its charstrings,
// its subroutines, and which Font DICT each glyph is drawn with, with the
// blend a glyph's charstring is run through at a location.
func (f *cff2Font) outlines(blend *cff2Blend) *cffOutlines {
	fds := make([]byte, len(f.fds))
	for g, fd := range f.fds {
		fds[g] = byte(fd) // fewer than maxCFF2FontDicts, which a byte holds
	}
	return &cffOutlines{charStrings: f.charStrings, global: f.global, locals: f.locals, fds: fds, cff2: blend}
}

// cff2Blend is how the blends of a font's charstrings are resolved at one
// location: each region's scalars there, worked out for a group of regions the
// first time a charstring blends against it, and whether a blended number is
// rounded.
//
// A blended number is its default plus, for each region of its group, its
// delta multiplied by the region's scalars one after another, summed in the
// order of the regions. That one loop is both ways of resolving a blend the
// top of this file describes; what differs is the scalars and the rounding.
// fontTools' way, which is the one an instance is cut with, is
// newFontToolsBlend. HarfBuzz's — one float scalar a region and nothing
// rounded — is what the tests hold the reader to HarfBuzz with (cff2_test.go).
type cff2Blend struct {
	store *varStore
	ivs   []int // each Font DICT's vsindex
	// located is whether the location is anywhere but the default, where
	// nothing is blended; factorsOf is a region's scalars there, and whether
	// the region is dropped; round rounds a blended number's deltas, half to
	// even, before they are added to its default.
	located   bool
	factorsOf func(region []varRegion) ([]float64, bool)
	round     bool
	// groups is each group's regions' factors, by vsindex, once worked out.
	groups []*cff2Group
}

// cff2Group is one group of regions' scalars at a location.
type cff2Group struct {
	factors [][]float64
	dropped []bool
}

// newFontToolsBlend resolves blends as fontTools' instancer does, at the
// normalized location coords whose axes are tagged tags, in fvar's order: see
// fontToolsFactors, and the rounding, which is Python's — half to even.
func newFontToolsBlend(f *cff2Font, coords []float64, tags []string) *cff2Blend {
	b := &cff2Blend{store: f.store, ivs: f.ivs, round: true}
	for _, c := range coords {
		b.located = b.located || c != 0
	}
	// fontTools pins the axes in the order of their tags, and a region's
	// deltas are multiplied by each axis's scalar in that order.
	order := make([]int, len(tags))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return tags[order[i]] < tags[order[j]] })
	b.factorsOf = func(region []varRegion) ([]float64, bool) { return fontToolsFactors(region, coords, order) }
	return b
}

// regionCount is how many regions the group vsindex names has: the deltas a
// blend reads per value. A group the store does not have has none.
func (b *cff2Blend) regionCount(ivs int) int {
	if b == nil || b.store == nil || ivs < 0 || ivs >= len(b.store.data) {
		return 0
	}
	return len(b.store.data[ivs].regions)
}

// resolve is a blended value: def moved by deltas, one per region of the
// group ivs, at the location.
func (b *cff2Blend) resolve(def float64, deltas []float64, ivs int) float64 {
	if b == nil || b.store == nil || !b.located {
		return def
	}
	grp := b.group(ivs)
	if len(grp.dropped) != len(deltas) {
		return def
	}
	// Each surviving region's delta multiplied by its scalars one at a time,
	// summed in region order starting from the first.
	var v float64
	started := false
	for i, d := range deltas {
		if grp.dropped[i] {
			continue
		}
		x := d
		for _, f := range grp.factors[i] {
			x *= f
		}
		if !started {
			v, started = x, true
			continue
		}
		v += x
	}
	if b.round {
		v = math.RoundToEven(v)
	}
	return def + v
}

// group is group ivs's scalars at the location, worked out the first time
// it is asked for. A group the store does not have has no regions.
func (b *cff2Blend) group(ivs int) *cff2Group {
	if ivs < 0 || ivs >= len(b.store.data) {
		return &cff2Group{}
	}
	if b.groups == nil {
		b.groups = make([]*cff2Group, len(b.store.data))
	}
	if g := b.groups[ivs]; g != nil {
		return g
	}
	idx := b.store.data[ivs].regions
	g := &cff2Group{factors: make([][]float64, len(idx)), dropped: make([]bool, len(idx))}
	for i, r := range idx {
		g.factors[i], g.dropped[i] = b.factorsOf(b.store.regions[r])
	}
	b.groups[ivs] = g
	return g
}

// fontToolsFactors are a region's scalars as fontTools' instancer pins it at
// the location coords (changeTupleVariationAxisLimit and the solver's
// rebaseTent), one per axis that takes part, in the order order names the
// axes in, and whether it drops the region. An axis whose peak is zero takes
// no part. A region whose tent on some axis is not one — its peak outside its
// span, or its span across the default — is dropped, and so is one the
// location lies outside. Each axis's scalar is (v−lower)/(peak−lower) below
// the peak and (upper−v)/(upper−peak) above it. They are kept apart rather
// than multiplied together because fontTools scales a delta set once per
// axis, and a delta times one and then the other is not always the same
// double as the delta times their product.
func fontToolsFactors(region []varRegion, coords []float64, order []int) ([]float64, bool) {
	var fs []float64
	for _, a := range order {
		if a >= len(region) {
			continue
		}
		r := region[a]
		if r.peak == 0 {
			continue
		}
		if !(r.start <= r.peak && r.peak <= r.end) || (r.start < 0 && r.end > 0) {
			return nil, true
		}
		var v float64
		if a < len(coords) {
			v = coords[a]
		}
		switch {
		case v == r.peak:
			continue
		case v <= r.start || v >= r.end:
			return nil, true
		case v < r.peak:
			fs = append(fs, (v-r.start)/(r.peak-r.start))
		default:
			fs = append(fs, (r.end-v)/(r.end-r.peak))
		}
	}
	return fs, false
}

// cff2Location is the normalized location a CFF2 font is cut at, and its
// axes' tags in fvar's order: each axis normalized and mapped through avar,
// and held to the fourteen fractional bits the format stores a coordinate in,
// as for any variable font (normalizeLocation, which reaches it as HarfBuzz
// does). A location between two F2Dot14 values is not one any table of the
// font is written against.
func cff2Location(fvar, avar []byte, want map[string]float64) ([]float64, []string, error) {
	axes, err := parseFvar(fvar)
	if err != nil {
		return nil, nil, err
	}
	coords, err := normalizeLocation(axes, avar, want)
	if err != nil {
		return nil, nil, err
	}
	tags := make([]string, len(axes))
	for i, a := range axes {
		tags[i] = a.tag
	}
	return coords, tags, nil
}
