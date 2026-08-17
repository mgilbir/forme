package font

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"sort"
)

// WOFF, the Web Open Font Format — W3C WOFF 1.0.
//
// A WOFF is an sfnt taken apart, each table deflated on its own, and put back
// behind a 44-byte header. Nothing about the outlines, the character map or the
// metrics is changed by it, so unwrapping one yields a font every other reader
// in this module already understands: the whole of WOFF support is this file,
// and nothing downstream needs to know a font arrived that way.
//
// # Why this is here rather than skipped
//
// The web serves fonts in this format. Ninety-one of the CSS Working Group's
// reftests carry one, and a document whose @font-face points at a .woff was
// laid out in a substitute face — which is not a wrong page so much as a
// different one, and the reason those tests could prove nothing.
//
// # WOFF 2 is not this
//
// WOFF 2 is a different format that happens to share a name: Brotli rather than
// deflate, and — the part that matters — a *transformed* glyf table, which is
// re-encoded rather than merely compressed and has to be reconstructed point by
// point. That is a second parser of comparable size to this one, and it is not
// here. DecodeWOFF refuses a WOFF 2 by signature rather than failing obscurely
// part-way in.
//
// # Untrusted input
//
// Every length in the header and the table directory is attacker-controlled,
// and one of them — origLength — is the *output* size of a deflate stream. A
// forty-byte table can claim four gigabytes and, deflated from zeros, very
// nearly deliver it. So the rule throughout is that no allocation is sized from
// a number the file states: the total is capped, each table's output is read
// through a limit one byte past what it claimed, and a stream that produces
// more than it declared is a fault rather than a bigger buffer.

// maxWOFFSfntSize bounds the reconstructed font.
//
// Sixty-four megabytes is far past any real face — the largest CJK OTF in the
// corpora this is tested against is under twenty — and far below what a deflate
// bomb wants to reach. It is checked against the *running* total as the tables
// are decompressed rather than against totalSfntSize, because the header's
// figure is one of the numbers being defended against.
const maxWOFFSfntSize = 64 << 20

// maxWOFFTables bounds the table directory. An sfnt addresses tables with a
// 16-bit count and a real font carries a few dozen; the cap is only here so that
// numTables cannot size an allocation before the bytes behind it are known to
// exist.
const maxWOFFTables = 4096

const woffSignature = 0x774F4646  // "wOFF"
const woff2Signature = 0x774F4632 // "wOF2"

// IsWOFF reports whether the data begins with a WOFF 1.0 signature.
func IsWOFF(data []byte) bool {
	return len(data) >= 4 && binary.BigEndian.Uint32(data) == woffSignature
}

// IsWOFF2 reports whether the data begins with a WOFF 2 signature, which this
// module reads far enough to refuse by name.
func IsWOFF2(data []byte) bool {
	return len(data) >= 4 && binary.BigEndian.Uint32(data) == woff2Signature
}

// woffEntry is one table directory record, as written.
type woffEntry struct {
	tag        uint32
	offset     uint32
	compLength uint32
	origLength uint32
	checksum   uint32
}

// DecodeWOFF unwraps a WOFF 1.0 font into the sfnt it was made from.
//
// The result is an ordinary TrueType or OpenType font program: SFNTTables,
// ParseSFNT and ParseCFF read it exactly as they read a file that was never
// compressed. It does not reproduce the original file byte for byte — the
// padding between tables is written as zeros and the tables come out in tag
// order — and it is not meant to: what has to survive is every table's content
// and the offsets that address it.
//
// The metadata and private blocks a WOFF may carry are not part of the font and
// are not returned. Neither is read, which is also why neither is validated.
func DecodeWOFF(data []byte) ([]byte, error) {
	if IsWOFF2(data) {
		return nil, errors.New("fonts: this is a WOFF 2 font, which this engine does not read; " +
			"WOFF 2 re-encodes the outline table rather than compressing it")
	}
	if !IsWOFF(data) {
		return nil, errors.New("fonts: not a WOFF font")
	}
	const headerSize = 44
	if len(data) < headerSize {
		return nil, errors.New("fonts: the WOFF header is cut short")
	}
	flavor := binary.BigEndian.Uint32(data[4:])
	numTables := int(binary.BigEndian.Uint16(data[12:]))
	if numTables == 0 {
		return nil, errors.New("fonts: the WOFF declares no tables")
	}
	if numTables > maxWOFFTables {
		return nil, errors.New("fonts: the WOFF declares more tables than an sfnt can address")
	}
	// The directory has to be there before it is read. numTables is two
	// attacker-controlled bytes and this is the only thing standing between
	// them and a 4096-entry allocation.
	dirEnd := headerSize + 20*numTables
	if dirEnd > len(data) {
		return nil, errors.New("fonts: the WOFF table directory runs past the end of the file")
	}

	entries := make([]woffEntry, 0, numTables)
	for i := 0; i < numTables; i++ {
		p := headerSize + 20*i
		e := woffEntry{
			tag:        binary.BigEndian.Uint32(data[p:]),
			offset:     binary.BigEndian.Uint32(data[p+4:]),
			compLength: binary.BigEndian.Uint32(data[p+8:]),
			origLength: binary.BigEndian.Uint32(data[p+12:]),
			checksum:   binary.BigEndian.Uint32(data[p+16:]),
		}
		// The compressed extent must lie inside the file. Checked in 64 bits
		// because offset+compLength is exactly the addition an attacker wraps.
		if uint64(e.offset)+uint64(e.compLength) > uint64(len(data)) {
			return nil, errors.New("fonts: a WOFF table lies outside the file")
		}
		// A table is stored when the two lengths agree and deflated when the
		// compressed one is smaller. Larger is neither, and reading it either
		// way would be guessing.
		if e.compLength > e.origLength {
			return nil, errors.New("fonts: a WOFF table claims to compress to more than its own size")
		}
		entries = append(entries, e)
	}

	// Tag order. The spec requires the directory to be sorted and the sfnt
	// record array must be, because a reader may binary-search it — but this
	// sorts rather than rejecting, since a font whose directory is merely out of
	// order is perfectly readable and refusing it would lose a face over the
	// order of a list nothing has read yet.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })
	for i := 1; i < len(entries); i++ {
		if entries[i].tag == entries[i-1].tag {
			return nil, errors.New("fonts: the WOFF declares the same table twice")
		}
	}

	// The output: a 12-byte sfnt header, a 16-byte record per table, then the
	// tables themselves each padded to a four-byte boundary.
	//
	// This total is computed from origLength, which is stated by the file, so it
	// is checked against the cap here *and* the decompressed bytes are counted
	// again below — a table that lies about its size in the safe direction would
	// otherwise buy itself room.
	body := 12 + 16*len(entries)
	total := uint64(body)
	for _, e := range entries {
		total += uint64(e.origLength+3) &^ 3
		if total > maxWOFFSfntSize {
			return nil, errors.New("fonts: the WOFF's tables come to more than this engine will decompress")
		}
	}

	out := make([]byte, body, total)
	binary.BigEndian.PutUint32(out, flavor)
	binary.BigEndian.PutUint16(out[4:], uint16(len(entries)))
	// searchRange, entrySelector and rangeShift: the binary-search hints. They
	// are derived, not carried, because a WOFF does not store them.
	entrySelector := uint16(0)
	for 1<<(entrySelector+1) <= uint16(len(entries)) {
		entrySelector++
	}
	searchRange := uint16(16) << entrySelector
	binary.BigEndian.PutUint16(out[6:], searchRange)
	binary.BigEndian.PutUint16(out[8:], entrySelector)
	binary.BigEndian.PutUint16(out[10:], uint16(16*len(entries))-searchRange)

	for i, e := range entries {
		raw := data[e.offset : e.offset+e.compLength]
		var table []byte
		if e.compLength == e.origLength {
			table = raw
		} else {
			var err error
			table, err = inflateWOFFTable(raw, e.origLength)
			if err != nil {
				return nil, err
			}
		}
		if uint32(len(table)) != e.origLength {
			return nil, errors.New("fonts: a WOFF table decompressed to a different size than it declared")
		}
		rec := 12 + 16*i
		binary.BigEndian.PutUint32(out[rec:], e.tag)
		binary.BigEndian.PutUint32(out[rec+4:], e.checksum)
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:], e.origLength)
		out = append(out, table...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		// The running total, against the bytes that actually arrived rather
		// than the ones the file claimed will.
		if len(out) > maxWOFFSfntSize {
			return nil, errors.New("fonts: the WOFF decompressed to more than this engine will hold")
		}
	}
	return out, nil
}

// inflateWOFFTable decompresses one table, refusing a stream that produces more
// than it said it would.
//
// The limit is want+1 rather than want, and that extra byte is the whole point:
// reading exactly want bytes cannot tell a stream that ended from one that was
// merely cut off at the cap, so the reader is given room to overrun by one and
// is a fault if it uses it. Without that, a deflate bomb declaring a small
// origLength would be silently truncated to a valid-looking table.
func inflateWOFFTable(raw []byte, want uint32) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("fonts: a WOFF table is not a readable zlib stream: " + err.Error())
	}
	defer zr.Close()

	out := make([]byte, 0, want)
	buf := bytes.NewBuffer(out)
	n, err := io.Copy(buf, io.LimitReader(zr, int64(want)+1))
	if err != nil {
		return nil, errors.New("fonts: a WOFF table could not be decompressed: " + err.Error())
	}
	if n > int64(want) {
		return nil, errors.New("fonts: a WOFF table decompressed to more than it declared")
	}
	return buf.Bytes(), nil
}
