package fonttest

import "encoding/binary"

// Synthetic variation tables, so that instancing can be tested against a font
// whose design space and deltas the test states.
//
// The real fonts answer whether instancing is *right* — see testdata/varinstance,
// which compares against two other implementations. What they cannot state is a
// font that is malformed, or one built to be expensive: no real font lists a
// point its glyph does not have, and none declares four thousand tuples over a
// glyph of sixty thousand points. Those are what a reader has to survive, and
// they have to be built.

// VarAxis is one axis of a synthetic design space, in user coordinates.
type VarAxis struct {
	Tag           string
	Min, Def, Max float64
}

// VarInstance is one named instance: the name records it points at, and where it
// sits. PostScriptNameID is written only when it is not zero, which is how a
// font states an instance that has no PostScript name of its own.
type VarInstance struct {
	SubfamilyNameID  int
	Coords           []float64
	PostScriptNameID int
}

// FVAR builds an fvar table.
func FVAR(axes []VarAxis, instances []VarInstance) []byte {
	instanceSize := 4 + 4*len(axes)
	for _, in := range instances {
		if in.PostScriptNameID != 0 {
			instanceSize = 6 + 4*len(axes)
			break
		}
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint16(out[0:], 1)  // majorVersion
	binary.BigEndian.PutUint16(out[2:], 0)  // minorVersion
	binary.BigEndian.PutUint16(out[4:], 16) // axesArrayOffset
	binary.BigEndian.PutUint16(out[6:], 2)  // reserved
	binary.BigEndian.PutUint16(out[8:], uint16(len(axes)))
	binary.BigEndian.PutUint16(out[10:], 20) // axisSize
	binary.BigEndian.PutUint16(out[12:], uint16(len(instances)))
	binary.BigEndian.PutUint16(out[14:], uint16(instanceSize))
	for i, a := range axes {
		rec := make([]byte, 20)
		copy(rec, a.Tag)
		binary.BigEndian.PutUint32(rec[4:], uint32(fixed1616(a.Min)))
		binary.BigEndian.PutUint32(rec[8:], uint32(fixed1616(a.Def)))
		binary.BigEndian.PutUint32(rec[12:], uint32(fixed1616(a.Max)))
		binary.BigEndian.PutUint16(rec[18:], uint16(256+i)) // axisNameID
		out = append(out, rec...)
	}
	for _, in := range instances {
		rec := make([]byte, instanceSize)
		binary.BigEndian.PutUint16(rec[0:], uint16(in.SubfamilyNameID))
		for i, c := range in.Coords {
			binary.BigEndian.PutUint32(rec[4+4*i:], uint32(fixed1616(c)))
		}
		if instanceSize > 4+4*len(axes) {
			binary.BigEndian.PutUint16(rec[4+4*len(axes):], uint16(in.PostScriptNameID))
		}
		out = append(out, rec...)
	}
	return out
}

// AVAR builds an axis variations table from one segment map per axis, each a
// list of (from, to) pairs in normalized coordinates.
func AVAR(maps [][][2]float64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint16(out[0:], 1) // majorVersion
	binary.BigEndian.PutUint16(out[2:], 0) // minorVersion
	binary.BigEndian.PutUint16(out[4:], 0) // reserved
	binary.BigEndian.PutUint16(out[6:], uint16(len(maps)))
	for _, m := range maps {
		seg := make([]byte, 2+4*len(m))
		binary.BigEndian.PutUint16(seg[0:], uint16(len(m)))
		for i, p := range m {
			binary.BigEndian.PutUint16(seg[2+4*i:], uint16(f2Dot14(p[0])))
			binary.BigEndian.PutUint16(seg[4+4*i:], uint16(f2Dot14(p[1])))
		}
		out = append(out, seg...)
	}
	return out
}

// VarTuple is one tuple of one glyph's variation data: where in the design space
// it applies in full, which points it moves, and how far.
//
// Points nil means every point of the glyph, which the format states as an empty
// list rather than as a full one. A tuple that lists points leaves the rest to be
// inferred from them, which is the part of instancing that cannot be skipped.
type VarTuple struct {
	Peak       []float64
	Start, End []float64 // an explicit region; nil for the one the peak implies
	Points     []int
	DX, DY     []int
}

// GVAR builds a glyph variations table: one list of tuples per glyph, in glyph
// index order.
func GVAR(axisCount int, glyphs [][]VarTuple) []byte {
	var data []byte
	offsets := make([]uint32, len(glyphs)+1)
	for i, tuples := range glyphs {
		offsets[i] = uint32(len(data))
		data = append(data, glyphVariationData(axisCount, tuples)...)
	}
	offsets[len(glyphs)] = uint32(len(data))

	head := 20 + 4*(len(glyphs)+1)
	out := make([]byte, head)
	binary.BigEndian.PutUint16(out[0:], 1) // majorVersion
	binary.BigEndian.PutUint16(out[2:], 0) // minorVersion
	binary.BigEndian.PutUint16(out[4:], uint16(axisCount))
	binary.BigEndian.PutUint16(out[6:], 0) // sharedTupleCount
	binary.BigEndian.PutUint32(out[8:], 0) // sharedTuplesOffset
	binary.BigEndian.PutUint16(out[12:], uint16(len(glyphs)))
	binary.BigEndian.PutUint16(out[14:], 1) // flags: long offsets
	binary.BigEndian.PutUint32(out[16:], uint32(head))
	for i, off := range offsets {
		binary.BigEndian.PutUint32(out[20+4*i:], off)
	}
	return append(out, data...)
}

// glyphVariationData writes one glyph's tuples: the headers, then each tuple's
// point numbers and deltas laid out after them.
func glyphVariationData(axisCount int, tuples []VarTuple) []byte {
	if len(tuples) == 0 {
		return nil
	}
	// A tuple that moves every point uses the shared list, which is written as a
	// count of zero — the format's way of saying "all of them".
	shared := false
	for _, t := range tuples {
		if t.Points == nil {
			shared = true
		}
	}
	var headers, body []byte
	if shared {
		body = append(body, 0)
	}
	for _, t := range tuples {
		var data []byte
		index := 0x8000 // an embedded peak
		if t.Points != nil {
			index |= 0x2000 // private point numbers
			data = append(data, packedPoints(t.Points)...)
		}
		data = append(data, packedDeltas(t.DX)...)
		data = append(data, packedDeltas(t.DY)...)
		if t.Start != nil {
			index |= 0x4000
		}
		h := make([]byte, 4)
		binary.BigEndian.PutUint16(h[0:], uint16(len(data)))
		binary.BigEndian.PutUint16(h[2:], uint16(index))
		h = append(h, tupleCoords(t.Peak, axisCount)...)
		if t.Start != nil {
			h = append(h, tupleCoords(t.Start, axisCount)...)
			h = append(h, tupleCoords(t.End, axisCount)...)
		}
		headers = append(headers, h...)
		body = append(body, data...)
	}
	out := make([]byte, 4)
	count := len(tuples)
	if shared {
		count |= 0x8000
	}
	binary.BigEndian.PutUint16(out[0:], uint16(count))
	binary.BigEndian.PutUint16(out[2:], uint16(4+len(headers)))
	out = append(out, headers...)
	return append(out, body...)
}

func tupleCoords(c []float64, axisCount int) []byte {
	out := make([]byte, 2*axisCount)
	for i := 0; i < axisCount && i < len(c); i++ {
		binary.BigEndian.PutUint16(out[2*i:], uint16(f2Dot14(c[i])))
	}
	return out
}

// packedPoints writes a point-number list as differences, in runs of sixteen-bit
// values — the longest form, and the one that needs no case analysis.
func packedPoints(points []int) []byte {
	var out []byte
	if len(points) < 128 {
		out = append(out, byte(len(points)))
	} else {
		out = append(out, byte(0x80|len(points)>>8), byte(len(points)))
	}
	prev := 0
	for i := 0; i < len(points); {
		n := len(points) - i
		if n > 128 {
			n = 128
		}
		out = append(out, byte(0x80|(n-1)))
		for j := 0; j < n; j++ {
			d := points[i+j] - prev
			prev = points[i+j]
			out = append(out, byte(d>>8), byte(d))
		}
		i += n
	}
	return out
}

// packedDeltas writes deltas in runs of sixteen-bit values.
func packedDeltas(d []int) []byte {
	var out []byte
	for i := 0; i < len(d); {
		n := len(d) - i
		if n > 64 {
			n = 64
		}
		out = append(out, byte(0x40|(n-1)))
		for j := 0; j < n; j++ {
			out = append(out, byte(d[i+j]>>8), byte(d[i+j]))
		}
		i += n
	}
	return out
}

// SimpleGlyph builds one glyf entry: a single contour through the given points,
// every one of them on the curve.
//
// The coordinates are written in the long form whatever their size, so the entry
// is a fixed four bytes a point. A test that wants a glyph with a great many
// points does not want the compact form — the point of such a glyph is that it
// is expensive to instance and cheap to state.
func SimpleGlyph(xs, ys []int) []byte {
	n := len(xs)
	out := make([]byte, 12)
	binary.BigEndian.PutUint16(out[0:], 1) // one contour
	putI16(out[2:], int16(minOf(xs)))
	putI16(out[4:], int16(minOf(ys)))
	putI16(out[6:], int16(maxOf(xs)))
	putI16(out[8:], int16(maxOf(ys)))
	binary.BigEndian.PutUint16(out[10:], uint16(n-1)) // the contour's last point
	out = append(out, 0, 0)                           // instructionLength
	for i := 0; i < n; i++ {
		out = append(out, 0x01) // on curve, both coordinates in the long form
	}
	prev := 0
	for _, x := range xs {
		var b [2]byte
		putI16(b[:], int16(x-prev))
		prev = x
		out = append(out, b[:]...)
	}
	prev = 0
	for _, y := range ys {
		var b [2]byte
		putI16(b[:], int16(y-prev))
		prev = y
		out = append(out, b[:]...)
	}
	return out
}

// CompositeComponent is one component of a synthetic composite glyph: which
// glyph it draws and where.
type CompositeComponent struct {
	Glyph  int
	DX, DY int
	// Scale, when not zero, is a uniform scale the component is drawn through.
	Scale float64
	// MatchPoints writes the arguments as point numbers to be made to coincide
	// rather than as an offset. It is the placement an instancer cannot honour,
	// since the point it would have to move is in another glyph, and a fixture
	// needs to be able to state it in order to check that it is refused.
	MatchPoints bool
}

// CompositeGlyph builds one composite glyf entry. The bounding box is left at
// zero, which is what an instancer writes before its second pass fills it in.
func CompositeGlyph(comps []CompositeComponent) []byte {
	out := make([]byte, 10)
	putI16(out[0:], -1) // numberOfContours: a composite
	for i, c := range comps {
		flags := 0x0001 // ARG_1_AND_2_ARE_WORDS
		if !c.MatchPoints {
			flags |= 0x0002 // ARGS_ARE_XY_VALUES
		}
		if c.Scale != 0 {
			flags |= 0x0008 // WE_HAVE_A_SCALE
		}
		if i < len(comps)-1 {
			flags |= 0x0020 // MORE_COMPONENTS
		}
		rec := make([]byte, 8)
		binary.BigEndian.PutUint16(rec[0:], uint16(flags))
		binary.BigEndian.PutUint16(rec[2:], uint16(c.Glyph))
		putI16(rec[4:], int16(c.DX))
		putI16(rec[6:], int16(c.DY))
		out = append(out, rec...)
		if c.Scale != 0 {
			var s [2]byte
			binary.BigEndian.PutUint16(s[:], uint16(f2Dot14(c.Scale)))
			out = append(out, s[:]...)
		}
	}
	return out
}

// VarRegion is one region of an item variation store, in normalized
// coordinates: per axis, where its influence starts, where it applies in full,
// and where it ends.
type VarRegion struct{ Start, Peak, End []float64 }

// HVAR builds an advance-variations table: regions are the columns of one
// delta-set group and deltas[i] is row i. Every delta is written in the
// sixteen-bit form, which needs no case analysis and is what a table of advance
// deltas mostly uses anyway.
//
// mapping, when given, is the advance mapping: mapping[gid] is the row glyph gid
// takes, which is how a font states one delta set shared by many glyphs. With it
// nil a glyph's index is its row.
//
// A font can say how an advance varies twice — here, and in the phantom points
// gvar carries — and a fixture that makes the two disagree is the only way to
// see which one a reader believes.
func HVAR(axisCount int, regions []VarRegion, deltas [][]int, mapping []int) []byte {
	// The region list.
	rl := make([]byte, 4+6*axisCount*len(regions))
	binary.BigEndian.PutUint16(rl[0:], uint16(axisCount))
	binary.BigEndian.PutUint16(rl[2:], uint16(len(regions)))
	for i, r := range regions {
		for a := 0; a < axisCount; a++ {
			at := 4 + 6*(axisCount*i+a)
			putF2Dot14At(rl[at:], r.Start, a)
			putF2Dot14At(rl[at+2:], r.Peak, a)
			putF2Dot14At(rl[at+4:], r.End, a)
		}
	}

	// One item variation data group, its columns being every region in order.
	data := make([]byte, 6+2*len(regions))
	binary.BigEndian.PutUint16(data[0:], uint16(len(deltas)))
	binary.BigEndian.PutUint16(data[2:], uint16(len(regions))) // every column is a word
	binary.BigEndian.PutUint16(data[4:], uint16(len(regions)))
	for i := range regions {
		binary.BigEndian.PutUint16(data[6+2*i:], uint16(i))
	}
	for _, row := range deltas {
		for i := range regions {
			var b [2]byte
			if i < len(row) {
				putI16(b[:], int16(row[i]))
			}
			data = append(data, b[:]...)
		}
	}

	// The store's header, then the one group, then the region list.
	store := make([]byte, 12)
	binary.BigEndian.PutUint16(store[0:], 1)                    // format
	binary.BigEndian.PutUint32(store[2:], uint32(12+len(data))) // variationRegionListOffset
	binary.BigEndian.PutUint16(store[6:], 1)                    // itemVariationDataCount
	binary.BigEndian.PutUint32(store[8:], 12)                   // the group's offset
	store = append(store, data...)
	store = append(store, rl...)

	out := make([]byte, 20)
	binary.BigEndian.PutUint16(out[0:], 1)  // majorVersion
	binary.BigEndian.PutUint16(out[2:], 0)  // minorVersion
	binary.BigEndian.PutUint32(out[4:], 20) // itemVariationStoreOffset
	// The left and right side-bearing mapping offsets stay null.
	out = append(out, store...)
	if mapping == nil {
		return out
	}
	// Format 0, with a sixteen-bit inner index and four-byte entries: the widest
	// form, which needs no case analysis and can state any index.
	binary.BigEndian.PutUint32(out[8:], uint32(len(out))) // advanceWidthMappingOffset
	m := make([]byte, 4+4*len(mapping))
	m[0] = 0    // format
	m[1] = 0x3F // entryFormat: four-byte entries, sixteen inner bits
	binary.BigEndian.PutUint16(m[2:], uint16(len(mapping)))
	for i, row := range mapping {
		binary.BigEndian.PutUint32(m[4+4*i:], uint32(row)) // outer 0, inner row
	}
	return append(out, m...)
}

func putF2Dot14At(b []byte, v []float64, i int) {
	var f float64
	if i < len(v) {
		f = v[i]
	}
	binary.BigEndian.PutUint16(b, uint16(f2Dot14(f)))
}

func minOf(v []int) int {
	m := 0
	for i, x := range v {
		if i == 0 || x < m {
			m = x
		}
	}
	return m
}

func maxOf(v []int) int {
	m := 0
	for i, x := range v {
		if i == 0 || x > m {
			m = x
		}
	}
	return m
}

func fixed1616(v float64) int32 {
	if v < 0 {
		return int32(v*65536 - 0.5)
	}
	return int32(v*65536 + 0.5)
}
