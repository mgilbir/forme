package font

import (
	"encoding/binary"
	"errors"
)

// The glyf transform, W3C WOFF 2.0 §5.1 — the part of WOFF 2 that is a
// re-encoding rather than a compression.
//
// A TrueType glyph is a header, a list of contour end points, some hinting
// instructions, and then the points themselves as three interleaved arrays:
// flags, x deltas, y deltas, each entry one or two bytes depending on a bit in
// the flag. The transform takes every glyph in the font apart and writes the
// pieces into seven streams — all the contour counts together, all the point
// counts together, all the flags together, and so on — because a compressor
// finds far more to say about ten thousand contour counts in a row than about
// ten thousand glyphs each holding one.
//
// Rebuilding is therefore not decompression. It is reading seven streams in
// step and writing a glyph at a time, and the encoding of a point is chosen
// here rather than carried: the transform stores a delta and this decides
// whether it fits in a byte. That is why the reconstruction is exact only if
// the choice matches the one the specification states, and why a font rebuilt
// with the rule slightly wrong still parses, still draws, and is a different
// font.
//
// It also loses nothing that matters and does not reproduce the original file:
// glyphs come out padded to four bytes whatever they were, and a bounding box
// that a glyph did not state is computed from its points. That is what the
// format intends — the box is derivable, so storing it was waste.

// The flag bits of a simple glyph's points.
const (
	glyfOnCurve   = 1 << 0
	glyfXShort    = 1 << 1
	glyfYShort    = 1 << 2
	glyfRepeat    = 1 << 3
	glyfXSameOrUp = 1 << 4 // "x is the same", or with xShort, "the delta is positive"
	glyfYSameOrUp = 1 << 5
	glyfOverlap   = 1 << 6
)

// The flag bits of a composite glyph's components, which say how much of the
// component record follows.
const (
	compArgsAreWords   = 1 << 0
	compHaveScale      = 1 << 3
	compMoreComponents = 1 << 5
	compHaveXYScale    = 1 << 6
	compHave2x2        = 1 << 7
	compHaveInstrs     = 1 << 8
)

// maxWOFF2Points bounds one glyph. The point count is a sum of per-contour
// counts, each read as a number rather than as bytes present, so it is the one
// figure here that could ask for an allocation the file does not back.
const maxWOFF2Points = 1 << 20

// point is one outline point, in font units, with the coordinates already
// accumulated from their deltas.
type point struct {
	x, y    int32
	onCurve bool
}

// stream is one of the seven, read in step with the others.
type stream struct {
	b  []byte
	at int
}

func (s *stream) u8() (uint8, bool) {
	if s.at >= len(s.b) {
		return 0, false
	}
	v := s.b[s.at]
	s.at++
	return v, true
}

func (s *stream) u16() (uint16, bool) {
	if s.at+2 > len(s.b) {
		return 0, false
	}
	v := binary.BigEndian.Uint16(s.b[s.at:])
	s.at += 2
	return v, true
}

func (s *stream) u32() (uint32, bool) {
	if s.at+4 > len(s.b) {
		return 0, false
	}
	v := binary.BigEndian.Uint32(s.b[s.at:])
	s.at += 4
	return v, true
}

func (s *stream) take(n int) ([]byte, bool) {
	if n < 0 || s.at+n > len(s.b) {
		return nil, false
	}
	v := s.b[s.at : s.at+n]
	s.at += n
	return v, true
}

func (s *stream) rest() []byte { return s.b[s.at:] }

// short255 reads a 255UInt16: one byte for almost every value, and an escape
// for the rest. It is what the counts in these streams are written as, because
// a contour usually has fewer than 253 points.
func (s *stream) short255() (int, bool) {
	code, ok := s.u8()
	if !ok {
		return 0, false
	}
	switch code {
	case 253:
		v, ok := s.u16()
		return int(v), ok
	case 254:
		v, ok := s.u8()
		return int(v) + 253*2, ok
	case 255:
		v, ok := s.u8()
		return int(v) + 253, ok
	}
	return int(code), true
}

var errShortGlyf = errors.New("fonts: the WOFF 2's transformed glyf table is cut short")

// reconstructGlyf rebuilds glyf and, immediately after it, loca.
//
// The two are written together because loca is the offsets of the glyphs and
// they are only known as the glyphs are laid down. The caller has already put
// glyf's start where it wants it; loca lands directly after, which is where the
// directory record written for it will say it is.
func reconstructGlyf(out, src []byte, glyf *woff2Table, f *woff2Font) ([]byte, uint32, error) {
	s := &stream{b: src}
	if _, ok := s.u16(); !ok { // the transform version, which has only ever been 0
		return nil, 0, errShortGlyf
	}
	flags, ok := s.u16()
	if !ok {
		return nil, 0, errShortGlyf
	}
	if f.numGlyphs, ok = s.u16(); !ok {
		return nil, 0, errShortGlyf
	}
	if f.indexFormat, ok = s.u16(); !ok {
		return nil, 0, errShortGlyf
	}
	// loca's size follows from the glyph count and the offset width, and the
	// two are stated in different places. They have to agree, because a loca
	// one entry short is a font whose last glyph is whatever came after it.
	want := uint32(2) * uint32(f.numGlyphs+1)
	if f.indexFormat != 0 {
		want *= 2
	}
	if f.loca == nil || f.loca.origLength != want {
		return nil, 0, errors.New("fonts: the WOFF 2's loca table is not the size its glyph count calls for")
	}

	// Seven streams. All seven lengths come first, as a block, and then all
	// seven bodies — not each length in front of its own body, which is the
	// natural reading and is wrong.
	var sizes [7]uint32
	for i := range sizes {
		if sizes[i], ok = s.u32(); !ok {
			return nil, 0, errShortGlyf
		}
	}
	var subs [7]*stream
	for i, n := range sizes {
		b, ok := s.take(int(n))
		if !ok {
			return nil, 0, errShortGlyf
		}
		subs[i] = &stream{b: b}
	}
	nContourStream, nPointsStream, flagStream := subs[0], subs[1], subs[2]
	glyphStream, compositeStream, bboxStream, instructionStream := subs[3], subs[4], subs[5], subs[6]

	// A bit per glyph saying whether its points may overlap, which is a hint to
	// the rasteriser and is carried outside the glyphs because most fonts set
	// it for none of them.
	var overlapBitmap []byte
	if flags&1 != 0 {
		if overlapBitmap, ok = s.take((int(f.numGlyphs) + 7) >> 3); !ok {
			return nil, 0, errShortGlyf
		}
	}

	// A bit per glyph saying whether it stated its own bounding box. The ones
	// that did not have it computed from their points.
	bboxBitmap, ok := bboxStream.take(((int(f.numGlyphs) + 31) >> 5) << 2)
	if !ok {
		return nil, 0, errShortGlyf
	}

	start := len(out)
	locas := make([]uint32, int(f.numGlyphs)+1)
	f.xMins = make([]int16, f.numGlyphs)
	var sum uint32
	var points []point

	for i := 0; i < int(f.numGlyphs); i++ {
		haveBbox := bboxBitmap[i>>3]&(0x80>>(i&7)) != 0
		nContours, ok := nContourStream.u16()
		if !ok {
			return nil, 0, errShortGlyf
		}

		var glyph []byte
		var err error
		switch {
		case nContours == 0xffff:
			glyph, err = rebuildComposite(compositeStream, glyphStream, bboxStream,
				instructionStream, haveBbox)
		case nContours > 0:
			overlap := overlapBitmap != nil && overlapBitmap[i>>3]&(0x80>>(i&7)) != 0
			glyph, points, err = rebuildSimple(nContours, nPointsStream, flagStream,
				glyphStream, bboxStream, instructionStream, haveBbox, overlap, points)
		default:
			// No contours at all — a space, and most fonts have several. It
			// occupies no bytes, and stating a box for a glyph that has no
			// points is a contradiction rather than a redundancy.
			if haveBbox {
				return nil, 0, errors.New("fonts: the WOFF 2 gives a bounding box to a glyph with no outline")
			}
		}
		if err != nil {
			return nil, 0, err
		}

		locas[i] = uint32(len(out) - start)
		out = append(out, glyph...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		sum += computeULongSum(glyph)
		if len(out) > maxWOFFSfntSize {
			return nil, 0, errors.New("fonts: the WOFF 2's glyphs come to more than this engine will hold")
		}
		// The left edge of the outline, which is what hmtx's transform drops
		// and what puts it back. A glyph with no outline has no edge and keeps
		// the zero.
		if nContours != 0 {
			f.xMins[i] = int16(binary.BigEndian.Uint16(glyph[2:]))
		}
	}

	glyf.dstLength = uint32(len(out)) - glyf.dstOffset
	locas[f.numGlyphs] = glyf.dstLength

	f.loca.dstOffset = uint32(len(out))
	at := len(out)
	for _, v := range locas {
		if f.indexFormat != 0 {
			out = binary.BigEndian.AppendUint32(out, v)
		} else {
			// The short form stores half the offset, which is why every glyph
			// is padded to an even boundary above.
			out = binary.BigEndian.AppendUint16(out, uint16(v>>1))
		}
	}
	f.locaChecksum = computeULongSum(out[at:])
	f.loca.dstLength = uint32(len(out)) - f.loca.dstOffset
	return out, sum, nil
}

// rebuildComposite puts back a glyph made of other glyphs. Its component
// records are copied through unchanged — the transform does not touch them —
// so the work is finding where they end, which only the flags in them say.
func rebuildComposite(composite, glyphs, bboxes, instructions *stream, haveBbox bool) ([]byte, error) {
	// A composite has no points of its own, so its box cannot be computed and
	// has to have been stored.
	if !haveBbox {
		return nil, errors.New("fonts: the WOFF 2 gives no bounding box to a composite glyph")
	}
	size, haveInstructions, err := sizeOfComposite(composite)
	if err != nil {
		return nil, err
	}
	instrLen := 0
	if haveInstructions {
		var ok bool
		if instrLen, ok = glyphs.short255(); !ok {
			return nil, errShortGlyf
		}
	}
	g := make([]byte, 0, 12+size+instrLen)
	g = binary.BigEndian.AppendUint16(g, 0xffff)
	box, ok := bboxes.take(8)
	if !ok {
		return nil, errShortGlyf
	}
	g = append(g, box...)
	body, ok := composite.take(size)
	if !ok {
		return nil, errShortGlyf
	}
	g = append(g, body...)
	if haveInstructions {
		g = binary.BigEndian.AppendUint16(g, uint16(instrLen))
		ins, ok := instructions.take(instrLen)
		if !ok {
			return nil, errShortGlyf
		}
		g = append(g, ins...)
	}
	return g, nil
}

// sizeOfComposite measures one composite glyph's component records without
// consuming them, which is what the stream has to be left holding for them to
// be copied.
func sizeOfComposite(s *stream) (size int, haveInstructions bool, err error) {
	at := s.at
	defer func() { s.at = at }()
	for more := true; more; {
		flags, ok := s.u16()
		if !ok {
			return 0, false, errShortGlyf
		}
		more = flags&compMoreComponents != 0
		haveInstructions = haveInstructions || flags&compHaveInstrs != 0
		n := 2 // the component's glyph index
		if flags&compArgsAreWords != 0 {
			n += 4
		} else {
			n += 2
		}
		switch {
		case flags&compHaveScale != 0:
			n += 2
		case flags&compHaveXYScale != 0:
			n += 4
		case flags&compHave2x2 != 0:
			n += 8
		}
		if _, ok := s.take(n); !ok {
			return 0, false, errShortGlyf
		}
	}
	return s.at - at, haveInstructions, nil
}

// rebuildSimple puts back a glyph that is contours of points.
func rebuildSimple(nContours uint16, counts, flagBits, glyphs, bboxes, instructions *stream,
	haveBbox, overlap bool, scratch []point) ([]byte, []point, error) {

	ends := make([]int, nContours)
	total := 0
	for j := range ends {
		n, ok := counts.short255()
		if !ok {
			return nil, scratch, errShortGlyf
		}
		total += n
		if total > maxWOFF2Points {
			return nil, scratch, errors.New("fonts: a WOFF 2 glyph declares more points than this engine will hold")
		}
		// A contour's end is an index and the format writes it in sixteen bits.
		if total-1 >= 65536 {
			return nil, scratch, errors.New("fonts: a WOFF 2 glyph has more points than a contour index can name")
		}
		ends[j] = total - 1
	}
	flags, ok := flagBits.take(total)
	if !ok {
		return nil, scratch, errShortGlyf
	}
	points, consumed, err := tripletDecode(flags, glyphs.rest(), total, scratch)
	if err != nil {
		return nil, points, err
	}
	if _, ok := glyphs.take(consumed); !ok {
		return nil, points, errShortGlyf
	}
	instrLen, ok := glyphs.short255()
	if !ok {
		return nil, points, errShortGlyf
	}

	g := make([]byte, 0, 12+2*int(nContours)+5*total+instrLen)
	g = binary.BigEndian.AppendUint16(g, nContours)
	if haveBbox {
		box, ok := bboxes.take(8)
		if !ok {
			return nil, points, errShortGlyf
		}
		g = append(g, box...)
	} else {
		g = appendBbox(g, points)
	}
	for _, e := range ends {
		g = binary.BigEndian.AppendUint16(g, uint16(e))
	}
	g = binary.BigEndian.AppendUint16(g, uint16(instrLen))
	ins, ok := instructions.take(instrLen)
	if !ok {
		return nil, points, errShortGlyf
	}
	g = append(g, ins...)
	return appendPoints(g, points, overlap), points, nil
}

// withSign is how the triplet encoding carries a sign: in the low bit of the
// flag, separately from the magnitude.
func withSign(flag uint8, v int64) int64 {
	if flag&1 != 0 {
		return v
	}
	return -v
}

// tripletDecode reads the points of one glyph.
//
// This is the heart of the transform. A point is a flag byte and one to four
// bytes of data, and the flag says both how many bytes follow and how the two
// deltas are cut out of them — small deltas share a byte, a delta along one
// axis only costs a byte, and only a point that moves far in both costs four.
// The encoding is W3C WOFF 2.0 §5.1's table and there is no way to derive it;
// what is below is that table.
func tripletDecode(flags, in []byte, n int, scratch []point) ([]point, int, error) {
	points := scratch
	if cap(points) < n {
		points = make([]point, n)
	}
	points = points[:n]

	var x, y int64
	at := 0
	for i := 0; i < n; i++ {
		flag := flags[i]
		// The top bit is the one thing the flag says that is not about size.
		onCurve := flag>>7 == 0
		flag &= 0x7f

		var width int
		switch {
		case flag < 84:
			width = 1
		case flag < 120:
			width = 2
		case flag < 124:
			width = 3
		default:
			width = 4
		}
		if at+width > len(in) {
			return points, 0, errShortGlyf
		}

		var dx, dy int64
		switch {
		case flag < 10:
			// Straight down or up: eleven bits of y and no x at all.
			dy = withSign(flag, int64(flag&14)<<7+int64(in[at]))
		case flag < 20:
			dx = withSign(flag, int64((flag-10)&14)<<7+int64(in[at]))
		case flag < 84:
			// Both small: six bits each, half of them in the flag.
			b0 := int64(flag) - 20
			b1 := int64(in[at])
			dx = withSign(flag, 1+(b0&0x30)+(b1>>4))
			dy = withSign(flag>>1, 1+((b0&0x0c)<<2)+(b1&0x0f))
		case flag < 120:
			// Both middling: ten bits each.
			b0 := int64(flag) - 84
			dx = withSign(flag, 1+((b0/12)<<8)+int64(in[at]))
			dy = withSign(flag>>1, 1+(((b0%12)>>2)<<8)+int64(in[at+1]))
		case flag < 124:
			// Twelve bits each, sharing a byte between them.
			b2 := int64(in[at+1])
			dx = withSign(flag, int64(in[at])<<4+(b2>>4))
			dy = withSign(flag>>1, (b2&0x0f)<<8+int64(in[at+2]))
		default:
			dx = withSign(flag, int64(in[at])<<8+int64(in[at+1]))
			dy = withSign(flag>>1, int64(in[at+2])<<8+int64(in[at+3]))
		}
		at += width

		// The coordinates are running totals, so a long enough glyph could
		// carry them out of range. Refusing is right: the alternative is a
		// number that wrapped, and an outline drawn from one is nonsense that
		// looks like a glyph.
		x += dx
		y += dy
		if x < -(1<<31) || x >= 1<<31 || y < -(1<<31) || y >= 1<<31 {
			return points, 0, errors.New("fonts: a WOFF 2 glyph's outline runs out of the coordinate space")
		}
		points[i] = point{x: int32(x), y: int32(y), onCurve: onCurve}
	}
	return points, at, nil
}

// appendBbox computes the bounding box of a glyph that did not state one, which
// is most of them: the box is derivable from the points, so storing it was
// waste and the transform drops it.
func appendBbox(g []byte, points []point) []byte {
	var xMin, yMin, xMax, yMax int32
	if len(points) > 0 {
		xMin, xMax = points[0].x, points[0].x
		yMin, yMax = points[0].y, points[0].y
	}
	for _, p := range points[1:] {
		if p.x < xMin {
			xMin = p.x
		}
		if p.x > xMax {
			xMax = p.x
		}
		if p.y < yMin {
			yMin = p.y
		}
		if p.y > yMax {
			yMax = p.y
		}
	}
	for _, v := range [4]int32{xMin, yMin, xMax, yMax} {
		g = binary.BigEndian.AppendUint16(g, uint16(int16(v)))
	}
	return g
}

// appendPoints writes the points as TrueType stores them: a run-length coded
// flag per point, then every x delta, then every y delta, each one or two bytes
// according to a bit in its flag.
//
// The choice of width is made here rather than carried, and that is the whole
// reason the reconstruction is exact rather than merely equivalent: the
// transform kept the deltas and threw the encoding away, so an encoder and a
// decoder that disagree about when a delta is short produce different fonts
// from the same file.
func appendPoints(g []byte, points []point, overlap bool) []byte {
	var flags, xs, ys []byte
	lastFlag := -1
	repeat := 0
	var lastX, lastY int32

	for i, p := range points {
		flag := byte(0)
		if p.onCurve {
			flag |= glyfOnCurve
		}
		// The overlap bit lives on the first point and means the whole glyph.
		if overlap && i == 0 {
			flag |= glyfOverlap
		}
		dx, dy := p.x-lastX, p.y-lastY
		switch {
		case dx == 0:
			flag |= glyfXSameOrUp
		case dx > -256 && dx < 256:
			flag |= glyfXShort
			if dx > 0 {
				flag |= glyfXSameOrUp
			}
			xs = append(xs, byte(abs32(dx)))
		default:
			xs = binary.BigEndian.AppendUint16(xs, uint16(int16(dx)))
		}
		switch {
		case dy == 0:
			flag |= glyfYSameOrUp
		case dy > -256 && dy < 256:
			flag |= glyfYShort
			if dy > 0 {
				flag |= glyfYSameOrUp
			}
			ys = append(ys, byte(abs32(dy)))
		default:
			ys = binary.BigEndian.AppendUint16(ys, uint16(int16(dy)))
		}

		// A run of identical flags is written once with a count after it. The
		// count is a byte, so a run of more than 256 starts again.
		if int(flag) == lastFlag && repeat != 255 {
			flags[len(flags)-1] |= glyfRepeat
			repeat++
		} else {
			if repeat != 0 {
				flags = append(flags, byte(repeat))
			}
			flags = append(flags, flag)
			repeat = 0
		}
		lastX, lastY = p.x, p.y
		lastFlag = int(flag)
	}
	if repeat != 0 {
		flags = append(flags, byte(repeat))
	}

	g = append(g, flags...)
	g = append(g, xs...)
	return append(g, ys...)
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
