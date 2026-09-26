package font

import (
	"bytes"
	"encoding/binary"
	"sort"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The transform, fuzzed directly.
//
// FuzzDecodeWOFF2 reads a whole file, and the fixtures that carry a transformed
// glyf are Brotli streams: nearly every mutation of one breaks the compression,
// so the decoder stops at the stream and the transform — the part that reads
// point counts, contour ends and coordinate deltas out of a font's own numbers,
// which is the part with the arithmetic in it — is never reached. Handing it
// those bytes directly is the only way to fuzz it.

// transformedGlyf builds the transformed form of a glyf table with one simple
// glyph, which is a seed a fuzzer can make progress from: every length in it is
// a number a mutation can change, and every stream it names is somewhere a
// wrong length can point.
func transformedGlyf(numGlyphs uint16, nContours, nPoints, flags, glyphs, composite, bbox, instr []byte) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, 0) // the transform version
	b = binary.BigEndian.AppendUint16(b, 0) // no overlap bitmap
	b = binary.BigEndian.AppendUint16(b, numGlyphs)
	b = binary.BigEndian.AppendUint16(b, 0) // short loca
	for _, s := range [][]byte{nContours, nPoints, flags, glyphs, composite, bbox, instr} {
		b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	}
	for _, s := range [][]byte{nContours, nPoints, flags, glyphs, composite, bbox, instr} {
		b = append(b, s...)
	}
	return b
}

// FuzzWOFF2Glyf.
//
// A webfont arrives from the network, and every number below is one the font
// chose. What has to hold for arbitrary bytes is what holds for the reader
// around it: it finishes, it does not read past the end of anything, it does
// not return a font *and* an error, and it gives the same answer twice.
func FuzzWOFF2Glyf(f *testing.F) {
	// One glyph, one contour, three points, no bounding box of its own.
	oneGlyph := transformedGlyf(1,
		[]byte{0x00, 0x01},       // one contour
		[]byte{3},                // with three points
		[]byte{0x00, 0x00, 0x00}, // three flags, each one byte of data
		[]byte{0x10, 0x10, 0x10}, // the deltas
		nil,
		[]byte{0x00, 0x00, 0x00, 0x00}, // the bbox bitmap: none stated
		nil,
	)
	f.Add(oneGlyph)
	// A composite glyph, which is the other branch of the rebuild.
	f.Add(transformedGlyf(1,
		[]byte{0xFF, 0xFF}, // -1 contours: a composite
		nil, nil, nil,
		[]byte{0x00, 0x00, 0x00, 0x00},
		[]byte{0x00, 0x00, 0x00, 0x00},
		nil,
	))
	// No glyphs at all, and a header that stops part way through.
	f.Add(transformedGlyf(0, nil, nil, nil, nil, nil, nil, nil))
	f.Add([]byte{0, 0, 0, 0, 0, 1})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, src []byte) {
		out, err := reconstructOnce(src)
		if err != nil {
			if out != nil {
				t.Fatalf("refused with %v and returned %d bytes", err, len(out))
			}
			return
		}
		// glyf and loca together, and loca's size follows from the glyph count
		// the stream states — which is checked before any glyph is read, so a
		// rebuild that got this far agrees with it.
		if len(out) > maxWOFFSfntSize {
			t.Fatalf("rebuilt %d bytes, past the cap of %d", len(out), maxWOFFSfntSize)
		}
		again, err2 := reconstructOnce(src)
		if err2 != nil {
			t.Fatalf("succeeded and then refused with %v", err2)
		}
		if !bytes.Equal(out, again) {
			t.Fatal("two reads of the same bytes rebuilt different tables")
		}
	})
}

// reconstructOnce runs the transform over a stream, with the loca table sized
// the way a directory that agreed with the stream would have sized it.
//
// The size is derived from the stream rather than fixed, because a fixed one
// would refuse every mutation that changed the glyph count and the fuzzer would
// never get past the first check.
func reconstructOnce(src []byte) ([]byte, error) {
	var f woff2Font
	numGlyphs, indexFormat := uint32(0), uint32(0)
	if len(src) >= 8 {
		numGlyphs = uint32(binary.BigEndian.Uint16(src[4:]))
		indexFormat = uint32(binary.BigEndian.Uint16(src[6:]))
	}
	want := 2 * (numGlyphs + 1)
	if indexFormat != 0 {
		want *= 2
	}
	loca := woff2Table{tag: tagLoca, origLength: want}
	f.loca = &loca
	glyf := woff2Table{tag: tagGlyf, transformed: true, srcLength: uint32(len(src))}
	out, _, err := reconstructGlyf(nil, src, &glyf, &f)
	return out, err
}

// FuzzDecodeWOFF is the WOFF 1 container, which had no target at all.
//
// It is a different reader from WOFF 2's — a table directory of its own, a
// per-table zlib stream, and a metadata block — and every number in it is one
// the file chose. The same three properties hold.
func FuzzDecodeWOFF(f *testing.F) {
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 600, HasShape: true},
			{Rune: 'B', Advance: 700, HasShape: true},
		},
	})
	var tables []fonttest.WOFFTable
	for tag, body := range SFNTTables(sfnt) {
		tables = append(tables, fonttest.WOFFTable{Tag: tag, Data: body})
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Tag < tables[j].Tag })
	f.Add(fonttest.WOFF(fonttest.WOFFOptions{Tables: tables}))
	f.Add([]byte("wOFF"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, src []byte) {
		got, err := DecodeWOFF(src)
		if err != nil {
			if len(got) != 0 {
				t.Fatalf("refused with %v and returned %d bytes", err, len(got))
			}
			return
		}
		if len(got) > maxWOFFSfntSize {
			t.Fatalf("rebuilt %d bytes, past the cap of %d", len(got), maxWOFFSfntSize)
		}
		again, err2 := DecodeWOFF(src)
		if err2 != nil {
			t.Fatalf("succeeded and then refused with %v", err2)
		}
		if !bytes.Equal(got, again) {
			t.Fatal("two reads of the same bytes rebuilt different fonts")
		}
	})
}

// FuzzParseType1 is the Type 1 reader, which had no target either.
//
// It reads a PostScript program: an eexec-encrypted body, a CharStrings
// dictionary whose entries state their own lengths, and numbers written as
// text. A length in a font's own numbers is the shape of input every other
// target here exists for.
func FuzzParseType1(f *testing.F) {
	f.Add([]byte("%!PS-AdobeFont-1.0\n/FontName /Test def\ncurrentfile eexec\n"))
	f.Add([]byte("/CharStrings 2 dict dup begin\n/a 5 RD 12345 ND\nend\n"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, src []byte) {
		p := ParseType1(src)
		if p == nil {
			return
		}
		if p.NumGlyphs < 0 {
			t.Fatalf("the program reports %d glyphs", p.NumGlyphs)
		}
		again := ParseType1(src)
		if (again == nil) != (p == nil) {
			t.Fatal("two reads of the same bytes disagreed about whether it parses")
		}
	})
}
