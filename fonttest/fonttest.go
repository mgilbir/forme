// Package fonttest builds font programs for tests to read.
//
// A test about fonts needs a font, and a real one is a poor fixture: it is
// megabytes, it carries every table at once, and it cannot be made to say the
// one wrong thing a reader is supposed to survive. So these build the smallest
// sfnt, CFF or Type 1 program that has the feature under test — a kern pair, a
// ligature, a mark class, a cmap subtable of a chosen format — and nothing else.
//
// It is a package of its own because more than one package reads font
// programs: font's parsers, shape's shaper and subsetter, and the tests of
// layout and paragraph that need a face with particular metrics all build
// their fixtures here, and cmd/genwoff2hmtx builds one it commits. A second
// copy is the thing to avoid: both would be edited, neither would be edited the
// same way, and a difference between them would look like a difference between
// the readers.
//
// A fixture that quietly builds something other than what it was asked for — a
// character cut to sixteen bits, an offset left at zero — is read by the same
// understanding of the format that wrote it, so the test using it passes or
// fails for a reason unrelated to what it is testing. SFNT, CmapFormat4 and
// LigatureSubst refuse, with a panic, what they cannot write as asked. The
// other builders take glyph indices and values as ints and write them into
// the format's sixteen-bit fields as they come, so a caller asking for more
// than a field holds gets its low half.
//
// A fixture is not an oracle. Everything built here is written and read by the
// same understanding of the format, so a test using one catches a reader that
// contradicts itself and not a reader that is consistently wrong. That is what
// testdata/harfbuzz is for.
package fonttest

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Type1Program builds a minimal Type 1 font program defining exactly the named
// glyphs, in the eexec-encrypted form a Type 1 reader expects.
//
// The charstrings are filler. What a caller wants from this is the set of glyph
// names the program declares — which is what a font's outlines are addressed by
// in Type 1, and what a format carrying the font has to agree with it about.
func Type1Program(names []string) []byte {
	var priv strings.Builder
	// lenIV 0 keeps the filler charstrings from needing a decryption prefix.
	priv.WriteString("dup /Private 8 dict dup begin\n/lenIV 0 def\n")
	// The dict count is written, because every real Type 1 font writes one and
	// a fixture shaped around a reader's mistake tests the mistake.
	priv.WriteString("2 index /CharStrings " + strconv.Itoa(len(names)) + " dict dup begin\n")
	for _, n := range names {
		// "/name len RD <len bytes> ND"
		priv.WriteString("/" + n + " 1 RD \x8b ND\n")
	}
	priv.WriteString("end\nend\nmark currentfile closefile\n")

	// eexec encryption is the inverse of eexecDecrypt(data, 55665, 4): four
	// leading pad bytes are consumed by the decryptor's discard.
	plain := append([]byte("pad!"), priv.String()...)
	var r uint16 = 55665
	const c1, c2 = 52845, 22719
	enc := make([]byte, 0, len(plain))
	for _, p := range plain {
		c := p ^ byte(r>>8)
		r = (uint16(c)+r)*c1 + c2
		enc = append(enc, c)
	}
	return append([]byte(type1Header), enc...)
}

const type1Header = "%!PS-AdobeFont-1.0\n/FontMatrix [0.001 0 0 0.001 0 0] readonly def\ncurrentfile eexec\n"

// Type1ProgramHex is Type1Program with the encrypted portion in hexadecimal,
// which is the other form the format allows and the one a font that had to
// survive a seven-bit channel is in.
//
// Type1Hex says how to write the digits. The zero value is one unbroken line of
// lower-case, which is the simplest thing a writer does.
type Type1Hex struct {
	// Wrap is how many digits to put on a line before a newline, because a real
	// font in this form wraps — Adobe's own tools at 64 — and the decoder's
	// tolerance of white space between digits is the whole reason it does not
	// simply unhex. Zero writes one unbroken line.
	Wrap int

	// Upper writes A-F rather than a-f. Both are digits and fonts use both, so
	// a decoder that handled one case would be wrong for half the fonts in the
	// world and right for every fixture that forgot to ask.
	Upper bool

	// Odd drops the last digit, so the program is a hex string with an odd
	// number of digits. A reader pads it with a trailing zero, which changes the
	// final byte; the program is then damaged, and what a caller wants from it
	// is that the reader does something defined rather than reading off the end.
	Odd bool
}

func Type1ProgramHex(names []string, opts Type1Hex) []byte {
	body := Type1Program(names)[len(type1Header):]
	digits := make([]byte, 0, 2*len(body))
	for _, b := range body {
		digits = append(digits, hexDigit(b>>4, opts.Upper), hexDigit(b&0x0F, opts.Upper))
	}
	if opts.Odd {
		digits = digits[:len(digits)-1]
	}

	out := []byte(type1Header)
	for i, d := range digits {
		if opts.Wrap > 0 && i > 0 && i%opts.Wrap == 0 {
			out = append(out, '\n')
		}
		out = append(out, d)
	}
	return append(out, '\n')
}

func hexDigit(v byte, upper bool) byte {
	switch {
	case v < 10:
		return '0' + v
	case upper:
		return 'A' + v - 10
	default:
		return 'a' + v - 10
	}
}

// CmapFormat4 assembles a segment-mapping cmap subtable from {startCode,
// endCode, idDelta} segments, with no glyph index array.
//
// Every field of the format is sixteen bits, and a code or a length that does
// not fit is refused rather than cut down to the low sixteen: a start code of
// U+1D400 written as 0xD400 is a table that maps a different character, and a
// fixture that says one thing and builds another is the worst kind. The delta
// is the exception, because it is arithmetic modulo 65536 by definition.
func CmapFormat4(segs [][3]int) []byte {
	segX2 := len(segs) * 2
	b := make([]byte, 16+4*segX2)
	if len(b) > 0xFFFF {
		panic(fmt.Sprintf("fonttest: a format-4 subtable of %d segments is %d bytes, "+
			"more than its sixteen-bit length can say", len(segs), len(b)))
	}
	for i, seg := range segs {
		if seg[0] < 0 || seg[0] > 0xFFFF || seg[1] < 0 || seg[1] > 0xFFFF {
			panic(fmt.Sprintf("fonttest: format-4 segment %d is %#x..%#x, which "+
				"sixteen-bit codes cannot hold", i, seg[0], seg[1]))
		}
	}
	put16 := func(off, v int) { b[off] = byte(v >> 8); b[off+1] = byte(v) }
	put16(0, 4)      // format
	put16(2, len(b)) // length
	put16(6, segX2)  // segCountX2
	endBase := 14
	startBase := endBase + segX2 + 2
	deltaBase := startBase + segX2
	rangeBase := deltaBase + segX2
	for i, seg := range segs {
		put16(startBase+2*i, seg[0])
		put16(endBase+2*i, seg[1])
		put16(deltaBase+2*i, seg[2]&0xFFFF)
		put16(rangeBase+2*i, 0)
	}
	return b
}

func CmapFormat12(groups [][3]uint32) []byte {
	b := make([]byte, 16+12*len(groups))
	b[1] = 12                                               // format
	binary.BigEndian.PutUint32(b[4:], uint32(len(b)))       // length
	binary.BigEndian.PutUint32(b[12:], uint32(len(groups))) // nGroups
	for i, g := range groups {
		p := 16 + 12*i
		binary.BigEndian.PutUint32(b[p:], g[0])
		binary.BigEndian.PutUint32(b[p+4:], g[1])
		binary.BigEndian.PutUint32(b[p+8:], g[2])
	}
	return b
}

// CmapFormat13 assembles a many-to-one cmap subtable from {startCharCode,
// endCharCode, glyphID} groups. Its bytes are a format-12 table with the format
// number changed, which is exactly what the two formats are: the same header and
// the same group array, read differently.
func CmapFormat13(groups [][3]uint32) []byte {
	b := CmapFormat12(groups)
	b[1] = 13
	return b
}

// CmapFormat8 assembles a mixed 16/32-bit cmap subtable from the same groups,
// with an all-zero is32 bitmap — the parser does not read it, and a font that
// declared every code 16-bit is the honest thing for a builder to emit.
func CmapFormat8(groups [][3]uint32) []byte {
	const is32Len = 8192
	b := make([]byte, 16+is32Len+12*len(groups))
	b[1] = 8                                                        // format
	binary.BigEndian.PutUint32(b[4:], uint32(len(b)))               // length
	binary.BigEndian.PutUint32(b[12+is32Len:], uint32(len(groups))) // nGroups
	for i, g := range groups {
		p := 16 + is32Len + 12*i
		binary.BigEndian.PutUint32(b[p:], g[0])
		binary.BigEndian.PutUint32(b[p+4:], g[1])
		binary.BigEndian.PutUint32(b[p+8:], g[2])
	}
	return b
}

// CmapFormat10 assembles a trimmed 32-bit array mapping first, first+1, … to the
// given glyph indices.
func CmapFormat10(first uint32, gids []uint16) []byte {
	b := make([]byte, 20+2*len(gids))
	b[1] = 10                                             // format
	binary.BigEndian.PutUint32(b[4:], uint32(len(b)))     // length
	binary.BigEndian.PutUint32(b[12:], first)             // startCharCode
	binary.BigEndian.PutUint32(b[16:], uint32(len(gids))) // numChars
	for i, g := range gids {
		binary.BigEndian.PutUint16(b[20+2*i:], g)
	}
	return b
}

// CmapSub is one cmap subtable of a synthetic font: its platform and encoding
// IDs and its bytes.
type CmapSub struct {
	Plat, Enc int
	Data      []byte
}

// SFNTWithCmapSubtables is a font of one table, a cmap holding the given
// subtables in the given order.
func SFNTWithCmapSubtables(subs []CmapSub) []byte {
	cmap := cmapTable(subs)
	font := make([]byte, 12+16)
	binary.BigEndian.PutUint32(font, 0x00010000) // sfnt version 1.0
	binary.BigEndian.PutUint16(font[4:], 1)      // numTables
	copy(font[12:], "cmap")                      // tag
	binary.BigEndian.PutUint32(font[12+8:], 28)  // offset
	binary.BigEndian.PutUint32(font[12+12:], uint32(len(cmap)))
	return append(font, cmap...)
}

// cmapTable is a cmap table: its header, one encoding record per subtable, and
// the subtables in order after them.
func cmapTable(subs []CmapSub) []byte {
	cmap := make([]byte, 4+8*len(subs))
	binary.BigEndian.PutUint16(cmap[2:], uint16(len(subs)))
	for i, s := range subs {
		binary.BigEndian.PutUint16(cmap[4+8*i:], uint16(s.Plat))
		binary.BigEndian.PutUint16(cmap[4+8*i+2:], uint16(s.Enc))
		binary.BigEndian.PutUint32(cmap[4+8*i+4:], uint32(len(cmap)))
		cmap = append(cmap, s.Data...)
	}
	return cmap
}
