package fonttest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"sort"
)

// WOFFTable is one table of a synthetic WOFF: its four-character tag and its
// uncompressed content.
type WOFFTable struct {
	Tag  string
	Data []byte
	// Store writes the table uncompressed, which a real WOFF does whenever
	// deflating a table would not make it smaller — so it is an ordinary shape a
	// reader has to handle and not a malformed one.
	Store bool
}

// WOFFOptions configures a synthetic WOFF 1.0 font.
type WOFFOptions struct {
	// Flavor is the sfnt version the wrapped font declares; 0x00010000 if zero.
	Flavor uint32
	Tables []WOFFTable
	// Unsorted writes the table directory in the order given rather than in tag
	// order. A real WOFF is required to be sorted; one that is not is still
	// perfectly readable, which is the case this exists to build.
	Unsorted bool
	// LieAboutOrigLength writes this value as every deflated table's origLength
	// instead of the true one. It is how a decompression bomb is expressed: the
	// stream yields megabytes and the header promises bytes.
	LieAboutOrigLength uint32
	// Signature overrides the leading four bytes, for the WOFF 2 refusal.
	Signature uint32
}

// WOFF builds a WOFF 1.0 font from the given tables.
//
// It is a fixture rather than an encoder: it writes a well-formed container
// around content it does not look at, which is what a decoder test needs. The
// tables need not be real sfnt tables for the container itself to be exercised,
// and are real when the test is about what comes out the other side.
func WOFF(opts WOFFOptions) []byte {
	flavor := opts.Flavor
	if flavor == 0 {
		flavor = 0x00010000
	}
	tables := append([]WOFFTable(nil), opts.Tables...)
	if !opts.Unsorted {
		sort.SliceStable(tables, func(i, j int) bool { return tables[i].Tag < tables[j].Tag })
	}

	const headerSize = 44
	dirSize := 20 * len(tables)

	// Compress first: the directory cannot be written until every compressed
	// length is known, because the offsets depend on them.
	type built struct {
		tag        uint32
		body       []byte
		origLength uint32
	}
	var bodies []built
	for _, t := range tables {
		var body []byte
		if t.Store {
			body = t.Data
		} else {
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			zw.Write(t.Data)
			zw.Close()
			body = buf.Bytes()
			// A WOFF stores a table whose deflated form is no smaller, and a
			// decoder tells the two apart by comparing the lengths — so a
			// fixture that emitted a "compressed" table longer than the original
			// would be building a malformed font by accident.
			if len(body) >= len(t.Data) {
				body = t.Data
			}
		}
		orig := uint32(len(t.Data))
		if opts.LieAboutOrigLength != 0 && len(body) < len(t.Data) {
			orig = opts.LieAboutOrigLength
		}
		bodies = append(bodies, built{tag: tag4(t.Tag), body: body, origLength: orig})
	}

	out := make([]byte, headerSize+dirSize)
	sig := opts.Signature
	if sig == 0 {
		sig = 0x774F4646 // "wOFF"
	}
	binary.BigEndian.PutUint32(out[0:], sig)
	binary.BigEndian.PutUint32(out[4:], flavor)
	binary.BigEndian.PutUint16(out[12:], uint16(len(tables)))
	// totalSfntSize, which a decoder must not size anything from.
	total := 12 + 16*len(tables)
	for _, b := range bodies {
		total += (int(b.origLength) + 3) &^ 3
	}
	binary.BigEndian.PutUint32(out[16:], uint32(total))
	binary.BigEndian.PutUint16(out[20:], 1) // majorVersion

	for i, b := range bodies {
		off := len(out)
		p := headerSize + 20*i
		binary.BigEndian.PutUint32(out[p:], b.tag)
		binary.BigEndian.PutUint32(out[p+4:], uint32(off))
		binary.BigEndian.PutUint32(out[p+8:], uint32(len(b.body)))
		binary.BigEndian.PutUint32(out[p+12:], b.origLength)
		binary.BigEndian.PutUint32(out[p+16:], 0) // origChecksum
		out = append(out, b.body...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	binary.BigEndian.PutUint32(out[8:], uint32(len(out))) // length
	return out
}

// WOFFBomb builds a WOFF whose one table deflates from size zero bytes and
// declares an origLength of declared.
//
// declared has to sit *above* the compressed length or the container is
// malformed in a duller way — a WOFF stores a table whole whenever deflating it
// would not shrink it, so compLength above origLength is refused before any
// stream is read, and a fixture that produced one would test that check instead
// of this one. Between the two is the real shape: a header a decoder can believe
// right up until the bytes arrive.
//
// It is what a decoder must refuse without allocating what the stream would
// produce, and the reason DecodeWOFF reads each table through a limit rather
// than into a buffer sized from the header.
func WOFFBomb(size int, declared uint32) []byte {
	return WOFF(WOFFOptions{
		Tables:             []WOFFTable{{Tag: "glyf", Data: make([]byte, size)}},
		LieAboutOrigLength: declared,
	})
}

func tag4(s string) uint32 {
	var b [4]byte
	copy(b[:], "    ")
	copy(b[:], s)
	return binary.BigEndian.Uint32(b[:])
}
