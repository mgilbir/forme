package font

import (
	"encoding/binary"
	"errors"
	"sort"

	"github.com/mgilbir/forme/brotli"
)

// WOFF 2, the format the web actually serves fonts in — W3C WOFF 2.0.
//
// It shares a name with WOFF 1 and almost nothing else. Where WOFF 1 deflates
// each table on its own and leaves the bytes alone, WOFF 2 concatenates every
// table into one Brotli stream and *re-encodes* three of them: glyf and loca
// are taken apart into seven parallel streams and put back point by point, and
// hmtx has the left side bearings dropped where they can be recovered from the
// outlines. So a WOFF 2 is not a compressed font; it is a font that has to be
// rebuilt, and what comes out here is the sfnt every other reader in this
// module already understands.
//
// # Why it is worth the size
//
// Fifty-one of the CSS Working Group's reftests declare an @font-face pointing
// at a .woff2 — forty-three of them at a file this checkout has — and every one
// of them was laid out in a substitute face. It is also, simply, the format:
// a page that serves a webfont in 2026 serves it as WOFF 2.
//
// # What is not here
//
// A collection — a .ttc wrapped as WOFF 2 — is refused by name. The format
// allows it and nothing on the web does it: a collection shares tables between
// fonts, which means per-font table lists, per-font offset tables and a shared
// checksum map, for a case no corpus this is measured against contains. It is
// refused rather than half-read, so a caller finds out.
//
// # Untrusted input
//
// Everything here is attacker-controlled, and two numbers in particular: the
// size the Brotli stream decompresses to, and the number of points in a glyph.
// The first is bounded by handing brotli.Decode an exact limit rather than a
// buffer to fill; the second by the streams the points are read from, each of
// which has a stated length inside a table whose own length is checked. No
// allocation here is sized from a number the file states without the bytes
// behind it being known to exist.

const woff2HeaderSize = 48

// woff2KnownTags is the table tags a WOFF 2 may name by index instead of
// spelling out — W3C WOFF 2.0 §5.2's table, in its order, which is the whole of
// what the index means.
var woff2KnownTags = [63]uint32{
	tagOf("cmap"), tagOf("head"), tagOf("hhea"), tagOf("hmtx"),
	tagOf("maxp"), tagOf("name"), tagOf("OS/2"), tagOf("post"),
	tagOf("cvt "), tagOf("fpgm"), tagOf("glyf"), tagOf("loca"),
	tagOf("prep"), tagOf("CFF "), tagOf("VORG"), tagOf("EBDT"),
	tagOf("EBLC"), tagOf("gasp"), tagOf("hdmx"), tagOf("kern"),
	tagOf("LTSH"), tagOf("PCLT"), tagOf("VDMX"), tagOf("vhea"),
	tagOf("vmtx"), tagOf("BASE"), tagOf("GDEF"), tagOf("GPOS"),
	tagOf("GSUB"), tagOf("EBSC"), tagOf("JSTF"), tagOf("MATH"),
	tagOf("CBDT"), tagOf("CBLC"), tagOf("COLR"), tagOf("CPAL"),
	tagOf("SVG "), tagOf("sbix"), tagOf("acnt"), tagOf("avar"),
	tagOf("bdat"), tagOf("bloc"), tagOf("bsln"), tagOf("cvar"),
	tagOf("fdsc"), tagOf("feat"), tagOf("fmtx"), tagOf("fvar"),
	tagOf("gvar"), tagOf("hsty"), tagOf("just"), tagOf("lcar"),
	tagOf("mort"), tagOf("morx"), tagOf("opbd"), tagOf("prop"),
	tagOf("trak"), tagOf("Zapf"), tagOf("Silf"), tagOf("Glat"),
	tagOf("Gloc"), tagOf("Feat"), tagOf("Sill"),
}

// tagOf packs a four-character table tag, which is how an sfnt writes one.
func tagOf(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

var (
	tagGlyf = tagOf("glyf")
	tagLoca = tagOf("loca")
	tagHmtx = tagOf("hmtx")
	tagHhea = tagOf("hhea")
	tagHead = tagOf("head")
	tagTtcf = tagOf("ttcf")
)

// woff2Table is one entry of the table directory, and then where it landed.
type woff2Table struct {
	tag         uint32
	transformed bool
	version     uint8 // which transform, where a tag has more than one

	srcOffset  uint32 // into the decompressed stream
	srcLength  uint32 // its length there, transformed
	origLength uint32 // what it will be once rebuilt

	dstOffset uint32 // into the sfnt being built
	dstLength uint32
	record    int // where its 16-byte directory record is
}

// DecodeWOFF2 unwraps a WOFF 2 font into the sfnt it was made from.
//
// The result is an ordinary TrueType or OpenType font program. It is not the
// file the WOFF 2 was made from byte for byte — nothing could be, since the
// transform discards the original glyph encoding and the padding between
// tables — but it is a font with the same outlines, metrics and character map,
// and it is what a browser builds from the same bytes.
func DecodeWOFF2(data []byte) ([]byte, error) {
	if !IsWOFF2(data) {
		return nil, errors.New("fonts: not a WOFF 2 font")
	}
	if len(data) < woff2HeaderSize {
		return nil, errors.New("fonts: the WOFF 2 header is cut short")
	}
	flavor := binary.BigEndian.Uint32(data[4:])
	if length := binary.BigEndian.Uint32(data[8:]); uint64(length) != uint64(len(data)) {
		return nil, errors.New("fonts: the WOFF 2 states a length that is not its own")
	}
	numTables := int(binary.BigEndian.Uint16(data[12:]))
	if numTables == 0 {
		return nil, errors.New("fonts: the WOFF 2 declares no tables")
	}
	if numTables > maxWOFFTables {
		return nil, errors.New("fonts: the WOFF 2 declares more tables than an sfnt can address")
	}
	compressedLength := binary.BigEndian.Uint32(data[20:])
	if flavor == tagTtcf {
		return nil, errors.New("fonts: this is a WOFF 2 font collection, which this engine does not read")
	}

	r := &woff2Reader{b: data, at: woff2HeaderSize}
	tables, err := readWOFF2Directory(r, numTables)
	if err != nil {
		return nil, err
	}

	// Everything after the directory, for compressedLength bytes, is one
	// Brotli stream holding every table end to end. After it come the metadata
	// and private blocks, if the header named them, and then the end.
	//
	// The walk below is not bookkeeping: the format lays the blocks out one
	// after another on four-byte boundaries, so their offsets are derivable,
	// and a header whose figures do not add up to the file is describing a
	// different file. Reading on regardless is how a font with something extra
	// hidden between its blocks gets read as though it were ordinary.
	at := round4(uint64(r.at) + uint64(compressedLength))
	if at > uint64(len(data)) {
		return nil, errors.New("fonts: the WOFF 2's compressed font data runs past the end of the file")
	}
	metaOffset := uint64(binary.BigEndian.Uint32(data[28:]))
	metaLength := uint64(binary.BigEndian.Uint32(data[32:]))
	privOffset := uint64(binary.BigEndian.Uint32(data[40:]))
	privLength := uint64(binary.BigEndian.Uint32(data[44:]))
	if metaOffset != 0 {
		if metaOffset != at || metaOffset+metaLength > uint64(len(data)) {
			return nil, errors.New("fonts: the WOFF 2's metadata block is not where its header says")
		}
		at = round4(metaOffset + metaLength)
	}
	if privOffset != 0 {
		if privOffset != at || privOffset+privLength > uint64(len(data)) {
			return nil, errors.New("fonts: the WOFF 2's private block is not where its header says")
		}
		at = round4(privOffset + privLength)
	}
	if at != round4(uint64(len(data))) {
		return nil, errors.New("fonts: the WOFF 2's blocks do not account for the whole file")
	}
	last := tables[len(tables)-1]
	total := uint64(last.srcOffset) + uint64(last.srcLength)
	if total > maxWOFFSfntSize {
		return nil, errors.New("fonts: the WOFF 2's tables come to more than this engine will decompress")
	}
	// An exact limit rather than a buffer to fill: brotli.Decode refuses a
	// stream that wants more, and a stream that produces less is caught here,
	// so neither a short stream nor a bomb reaches the transforms.
	body, err := brotli.Decode(data[r.at:r.at+int(compressedLength)], int(total))
	if err != nil {
		return nil, errors.New("fonts: the WOFF 2's font data could not be decompressed: " + err.Error())
	}
	if uint64(len(body)) != total {
		return nil, errors.New("fonts: the WOFF 2's font data decompressed to a different size than its tables come to")
	}

	return rebuildSfnt(flavor, tables, body)
}

// readWOFF2Directory reads the table directory, whose entries are variable
// length: a flag byte that usually stands for the tag, then one or two lengths
// written seven bits at a time.
func readWOFF2Directory(r *woff2Reader, numTables int) ([]woff2Table, error) {
	tables := make([]woff2Table, 0, numTables)
	var srcOffset uint32
	for i := 0; i < numTables; i++ {
		flags, ok := r.u8()
		if !ok {
			return nil, errors.New("fonts: the WOFF 2 table directory is cut short")
		}
		var t woff2Table
		if known := flags & 0x3f; known == 0x3f {
			if t.tag, ok = r.u32(); !ok {
				return nil, errors.New("fonts: the WOFF 2 table directory is cut short")
			}
		} else {
			t.tag = woff2KnownTags[known]
		}
		t.version = (flags >> 6) & 3

		// Which version means "transformed" depends on the table, because glyf
		// and loca have one transform and it is the default, while everything
		// else has none and version 0 is what says so.
		if t.tag == tagGlyf || t.tag == tagLoca {
			t.transformed = t.version == 0
		} else {
			t.transformed = t.version != 0
		}

		if t.origLength, ok = r.base128(); !ok {
			return nil, errors.New("fonts: a WOFF 2 table's length is not a readable number")
		}
		t.srcLength = t.origLength
		if t.transformed {
			if t.srcLength, ok = r.base128(); !ok {
				return nil, errors.New("fonts: a WOFF 2 table's transformed length is not a readable number")
			}
			// A transformed loca carries nothing: every byte of it comes back
			// out of the glyf stream.
			if t.tag == tagLoca && t.srcLength != 0 {
				return nil, errors.New("fonts: the WOFF 2's transformed loca table is not empty")
			}
		}
		if uint64(srcOffset)+uint64(t.srcLength) > maxWOFFSfntSize {
			return nil, errors.New("fonts: the WOFF 2's tables come to more than this engine will decompress")
		}
		t.srcOffset = srcOffset
		srcOffset += t.srcLength
		tables = append(tables, t)
	}
	return tables, nil
}

// woff2Reader reads the header and directory, refusing to run off the end.
type woff2Reader struct {
	b  []byte
	at int
}

func (r *woff2Reader) u8() (uint8, bool) {
	if r.at >= len(r.b) {
		return 0, false
	}
	v := r.b[r.at]
	r.at++
	return v, true
}

func (r *woff2Reader) u32() (uint32, bool) {
	if r.at+4 > len(r.b) {
		return 0, false
	}
	v := binary.BigEndian.Uint32(r.b[r.at:])
	r.at += 4
	return v, true
}

// base128 reads a UIntBase128: seven bits per byte, most significant first,
// with the top bit set on every byte but the last.
//
// A leading 0x80 is refused, and that refusal is the specification's: it is a
// zero written the long way, and one number has to have one spelling or two
// files that differ byte for byte are the same font.
//
// The overflow check is what rejects a number too large to be one, and it is
// worth saying that it reaches every over-long encoding before the loop bound
// does: a sixth byte can only follow five with their top bits set, and the
// smallest such number a non-zero first byte can make is already past
// twenty-five bits. The bound is a bound on the loop and not the thing that
// says no.
func (r *woff2Reader) base128() (uint32, bool) {
	var v uint32
	for i := 0; i < 5; i++ {
		b, ok := r.u8()
		if !ok {
			return 0, false
		}
		if i == 0 && b == 0x80 {
			return 0, false
		}
		if v&0xfe000000 != 0 {
			return 0, false
		}
		v = v<<7 | uint32(b&0x7f)
		if b&0x80 == 0 {
			return v, true
		}
	}
	return 0, false
}

// woff2Font is what the tables tell each other as they are rebuilt.
//
// Three of them are read out of one table and needed by another, which is why
// the order tables are written in is the order the directory gives rather than
// anything this chooses: hhea carries the count of metrics hmtx needs, and glyf
// produces the left edge of every outline, which is what hmtx's transform threw
// away.
type woff2Font struct {
	numGlyphs    uint16
	indexFormat  uint16
	numHMetrics  uint16
	xMins        []int16
	locaChecksum uint32
	loca         *woff2Table
}

// rebuildSfnt assembles the font: a header, a record per table in tag order,
// and the tables themselves in the order the directory named them.
func rebuildSfnt(flavor uint32, tables []woff2Table, body []byte) ([]byte, error) {
	n := len(tables)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	// The records are sorted by tag because a reader may binary-search them.
	// The tables themselves are not moved: their order is the directory's, and
	// it has to be, because glyf writes loca as it goes and hmtx needs what
	// glyf found.
	sort.SliceStable(order, func(i, j int) bool { return tables[order[i]].tag < tables[order[j]].tag })
	for k := 1; k < n; k++ {
		if tables[order[k]].tag == tables[order[k-1]].tag {
			return nil, errors.New("fonts: the WOFF 2 declares the same table twice")
		}
	}

	out := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(out, flavor)
	binary.BigEndian.PutUint16(out[4:], uint16(n))
	entrySelector := uint16(0)
	for 1<<(entrySelector+1) <= uint16(n) {
		entrySelector++
	}
	searchRange := uint16(16) << entrySelector
	binary.BigEndian.PutUint16(out[6:], searchRange)
	binary.BigEndian.PutUint16(out[8:], entrySelector)
	binary.BigEndian.PutUint16(out[10:], uint16(16*n)-searchRange)
	for k, i := range order {
		tables[i].record = 12 + 16*k
		binary.BigEndian.PutUint32(out[tables[i].record:], tables[i].tag)
	}

	// The whole header, records included, with every checksum and offset still
	// zero. The zeroes are replaced below and what replaced them is added on,
	// which is how the total ends up being of the finished font.
	checksum := computeULongSum(out)

	var f woff2Font
	glyf, loca := findWOFF2Table(tables, tagGlyf), findWOFF2Table(tables, tagLoca)
	if (glyf == nil) != (loca == nil) {
		return nil, errors.New("fonts: the WOFF 2 has one of glyf and loca and not the other")
	}
	if glyf != nil {
		if glyf.transformed != loca.transformed {
			return nil, errors.New("fonts: the WOFF 2 transformed one of glyf and loca and not the other")
		}
		// glyf writes loca as it goes, so a directory that names loca first is
		// asking for a table before the thing that produces it. Every real font
		// names them the other way round — an sfnt's directory is sorted by tag
		// and "glyf" comes before "loca" — so nothing is lost by refusing.
		//
		// It is refused rather than worked around because of what the
		// alternative looks like: the reference decoder accepts such a font and
		// writes it out with a loca record of offset zero and length zero,
		// which is a font of the right size, with all its outlines in it, and
		// no way to find any of them. A reader gets no glyphs and no reason.
		if indexOfWOFF2Table(tables, tagGlyf) > indexOfWOFF2Table(tables, tagLoca) {
			return nil, errors.New("fonts: the WOFF 2 names its loca table before the glyf table it comes out of")
		}
		f.loca = loca
	}

	for i := range tables {
		t := &tables[i]
		if uint64(t.srcOffset)+uint64(t.srcLength) > uint64(len(body)) {
			return nil, errors.New("fonts: a WOFF 2 table lies outside the decompressed font data")
		}
		src := body[t.srcOffset : t.srcOffset+t.srcLength]

		if t.tag == tagHhea {
			if len(src) < 36 {
				return nil, errors.New("fonts: the WOFF 2's hhea table is too short to hold its metric count")
			}
			f.numHMetrics = binary.BigEndian.Uint16(src[34:])
		}

		var sum uint32
		var err error
		switch {
		case !t.transformed:
			t.dstOffset = uint32(len(out))
			out = append(out, src...)
			// The checksum in the head table is of the finished font and
			// cannot be known yet, so it is zeroed, summed as zero, and filled
			// in at the end — which is what makes the total come out right.
			if t.tag == tagHead {
				if len(src) < 12 {
					return nil, errors.New("fonts: the WOFF 2's head table is too short")
				}
				binary.BigEndian.PutUint32(out[t.dstOffset+8:], 0)
			}
			sum = computeULongSum(out[t.dstOffset:])
			t.dstLength = t.origLength
		case t.tag == tagGlyf:
			t.dstOffset = uint32(len(out))
			out, sum, err = reconstructGlyf(out, src, t, &f)
		case t.tag == tagLoca:
			// glyf wrote it, along with everything needed to check it.
			sum = f.locaChecksum
		case t.tag == tagHmtx:
			t.dstOffset = uint32(len(out))
			out, sum, err = reconstructHmtx(out, src, &f)
			t.dstLength = uint32(len(out)) - t.dstOffset
			if t.dstLength != t.origLength {
				return nil, errors.New("fonts: the WOFF 2's hmtx table rebuilt to a different size than it declared")
			}
		default:
			return nil, errors.New("fonts: the WOFF 2 transforms a table this engine does not know how to rebuild")
		}
		if err != nil {
			return nil, err
		}

		binary.BigEndian.PutUint32(out[t.record+4:], sum)
		binary.BigEndian.PutUint32(out[t.record+8:], t.dstOffset)
		binary.BigEndian.PutUint32(out[t.record+12:], t.dstLength)
		checksum += sum
		checksum += computeULongSum(out[t.record+4 : t.record+16])

		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		if uint64(t.dstOffset)+uint64(t.dstLength) > uint64(len(out)) {
			return nil, errors.New("fonts: a WOFF 2 table was rebuilt shorter than its record says")
		}
		if len(out) > maxWOFFSfntSize {
			return nil, errors.New("fonts: the WOFF 2 rebuilt to more than this engine will hold")
		}
	}

	// head's checkSumAdjustment: the number that makes the whole font sum to a
	// constant. It is the last thing written because it is of everything else.
	if head := findWOFF2Table(tables, tagHead); head != nil {
		if head.dstLength < 12 {
			return nil, errors.New("fonts: the WOFF 2's head table is too short")
		}
		binary.BigEndian.PutUint32(out[head.dstOffset+8:], 0xB1B0AFBA-checksum)
	}
	return out, nil
}

// round4 rounds up to the next four-byte boundary, which is where every block
// of a WOFF 2 begins.
func round4(v uint64) uint64 { return (v + 3) &^ 3 }

func indexOfWOFF2Table(tables []woff2Table, tag uint32) int {
	for i := range tables {
		if tables[i].tag == tag {
			return i
		}
	}
	return -1
}

func findWOFF2Table(tables []woff2Table, tag uint32) *woff2Table {
	for i := range tables {
		if tables[i].tag == tag {
			return &tables[i]
		}
	}
	return nil
}

// computeULongSum is the sfnt checksum: the table read as big-endian 32-bit
// words and added up, wrapping, with a short last word padded with zeroes.
func computeULongSum(b []byte) uint32 {
	var sum uint32
	whole := len(b) &^ 3
	for i := 0; i < whole; i += 4 {
		sum += binary.BigEndian.Uint32(b[i:])
	}
	if whole != len(b) {
		var v uint32
		for i := whole; i < len(b); i++ {
			v |= uint32(b[i]) << (24 - 8*(i&3))
		}
		sum += v
	}
	return sum
}

// reconstructHmtx puts back the left side bearings the transform dropped.
//
// W3C WOFF 2.0 §5.3: a left side bearing is the distance from the origin to the
// left edge of the outline, and for most glyphs in most fonts it *is* the left
// edge of the outline — so it can be recovered from glyf rather than stored.
// The flag byte says which of the two runs were dropped: the ones belonging to
// glyphs with their own advance width, the ones after them that share the last,
// or either.
func reconstructHmtx(out, src []byte, f *woff2Font) ([]byte, uint32, error) {
	if len(src) < 1 {
		return nil, 0, errors.New("fonts: the WOFF 2's transformed hmtx table is empty")
	}
	flags := src[0]
	at := 1
	if flags&0xfc != 0 {
		return nil, 0, errors.New("fonts: the WOFF 2's hmtx table sets bits the format reserves")
	}
	haveProportional := flags&1 == 0
	haveMonospace := flags&2 == 0
	if haveProportional && haveMonospace {
		// Both runs present means nothing was dropped, and a table that was
		// not transformed must not say that it was.
		return nil, 0, errors.New("fonts: the WOFF 2's hmtx table is marked transformed and drops nothing")
	}
	if f.numHMetrics < 1 || f.numHMetrics > f.numGlyphs {
		return nil, 0, errors.New("fonts: the WOFF 2's hhea and maxp disagree about how many glyphs have metrics")
	}

	read16 := func() (int16, bool) {
		if at+2 > len(src) {
			return 0, false
		}
		v := int16(binary.BigEndian.Uint16(src[at:]))
		at += 2
		return v, true
	}
	widths := make([]uint16, f.numHMetrics)
	for i := range widths {
		v, ok := read16()
		if !ok {
			return nil, 0, errors.New("fonts: the WOFF 2's transformed hmtx table is cut short")
		}
		widths[i] = uint16(v)
	}
	bearings := make([]int16, f.numGlyphs)
	for i := range bearings {
		// Which glyphs kept their bearing depends on which side of the metric
		// count they fall.
		stored := haveProportional
		if i >= int(f.numHMetrics) {
			stored = haveMonospace
		}
		if !stored {
			bearings[i] = f.xMins[i]
			continue
		}
		v, ok := read16()
		if !ok {
			return nil, 0, errors.New("fonts: the WOFF 2's transformed hmtx table is cut short")
		}
		bearings[i] = v
	}

	at = len(out)
	for i := 0; i < int(f.numGlyphs); i++ {
		if i < int(f.numHMetrics) {
			out = binary.BigEndian.AppendUint16(out, widths[i])
		}
		out = binary.BigEndian.AppendUint16(out, uint16(bearings[i]))
	}
	return out, computeULongSum(out[at:]), nil
}
