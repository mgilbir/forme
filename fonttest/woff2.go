package fonttest

import (
	"encoding/binary"
	"sort"
)

// A synthetic WOFF 2 font.
//
// Like WOFF above it is a fixture rather than an encoder, and the difference
// matters more here: WOFF 2's glyf transform is a re-encoding of every outline
// in the font, and writing one would be a second implementation of the hardest
// part of the decoder — which would then agree with the decoder by
// construction, and prove nothing. So this builds the *container* and leaves
// glyf alone, declaring the null transform the format provides for exactly this
// case. What it does transform is hmtx, because that transform is a few lines
// and because no compressor in circulation emits one, so a fixture is the only
// way to reach the code that reads it.
//
// The font data is Brotli, as the format requires, written as stored blocks —
// Brotli's own way of saying "these bytes as they are". A stream of them is a
// perfectly ordinary Brotli stream and needs no compressor to produce.

// WOFF2Table is one table of a synthetic WOFF 2.
type WOFF2Table struct {
	Tag  string
	Data []byte // the table as it must appear in the rebuilt sfnt
	// Transform is the form written into the stream when the table is
	// transformed, and Version the transform number the directory declares.
	// Data is still what the rebuilt table has to come out as.
	Transform []byte
	Version   uint8
	// Transformed says the table is transformed even when Transform is empty,
	// which is what a transformed loca is: it carries nothing at all.
	Transformed bool
}

// WOFF2Options configures a synthetic WOFF 2 font.
type WOFF2Options struct {
	Flavor uint32 // the sfnt version; 0x00010000 if zero
	Tables []WOFF2Table
	// Signature overrides the leading four bytes.
	Signature uint32
	// StatedLength overrides the length the header claims, which a real one
	// states truthfully and a reader has to check.
	StatedLength uint32
	// SpellOutTags writes every tag in full rather than by index, which is what
	// a table outside the format's list of sixty-three requires and is legal
	// for any table.
	SpellOutTags bool
	// NumTables overrides the count in the header. It is a pointer because
	// zero is one of the values worth writing: a WOFF 2 that declares no
	// tables is malformed, and saying so is a test.
	NumTables *int
	// Garbage replaces the compressed font data, so that a stream that is not
	// a stream can be put in front of the decoder.
	Garbage []byte
	// Trailing is written after everything else, which a real WOFF 2 never has:
	// its blocks account for the file to the last four-byte boundary.
	Trailing []byte
	// Compressed replaces it with a Brotli stream made elsewhere. There is no
	// compressor here — stored blocks are what a fixture needs — but a fixture
	// that is going to be kept is worth making small, and a real stream is how.
	Compressed []byte
}

// WOFF2 builds a WOFF 2 font from the given tables.
func WOFF2(opts WOFF2Options) []byte {
	flavor := opts.Flavor
	if flavor == 0 {
		flavor = 0x00010000
	}
	tables := append([]WOFF2Table(nil), opts.Tables...)

	// The stream holds the tables in directory order, each in its transformed
	// form where it has one.
	var stream []byte
	var dir []byte
	for _, t := range tables {
		transformed := t.Transformed || len(t.Transform) > 0
		body := t.Data
		if transformed {
			body = t.Transform
		}
		stream = append(stream, body...)

		tag := woff2Tag(t.Tag)
		version := t.Version
		if version == 0 && !transformed && (t.Tag == "glyf" || t.Tag == "loca") {
			// For these two the null transform is version 3; version 0 means
			// the transform is applied.
			version = 3
		}
		if version == 0 && transformed && t.Tag != "glyf" && t.Tag != "loca" {
			version = 1
		}
		index := 0x3f
		if !opts.SpellOutTags {
			for i, k := range woff2Known {
				if k == t.Tag {
					index = i
					break
				}
			}
		}
		dir = append(dir, byte(index)|version<<6)
		if index == 0x3f {
			dir = binary.BigEndian.AppendUint32(dir, tag)
		}
		dir = appendBase128(dir, uint32(len(t.Data)))
		if transformed {
			dir = appendBase128(dir, uint32(len(body)))
		}
	}

	compressed := storedBrotli(stream)
	if opts.Compressed != nil {
		compressed = opts.Compressed
	}
	if opts.Garbage != nil {
		compressed = opts.Garbage
	}

	numTables := len(tables)
	if opts.NumTables != nil {
		numTables = *opts.NumTables
	}
	out := make([]byte, 48)
	signature := opts.Signature
	if signature == 0 {
		signature = 0x774F4632 // "wOF2"
	}
	binary.BigEndian.PutUint32(out[0:], signature)
	binary.BigEndian.PutUint32(out[4:], flavor)
	binary.BigEndian.PutUint16(out[12:], uint16(numTables))
	// totalSfntSize: the whole rebuilt font, padding included. A decoder is
	// free not to believe it, and one that sizes its output buffer from it
	// refuses a font whose figure is even four bytes short — which is what a
	// total that forgets the padding between tables is.
	total := 12 + 16*len(tables)
	for _, t := range tables {
		total += (len(t.Data) + 3) &^ 3
	}
	binary.BigEndian.PutUint32(out[16:], uint32(total))
	binary.BigEndian.PutUint32(out[20:], uint32(len(compressed)))
	out = append(out, dir...)
	out = append(out, compressed...)
	// The blocks of a WOFF 2 sit on four-byte boundaries and the last one is
	// padded to reach the end of the file, which a decoder checks.
	for len(out)%4 != 0 {
		out = append(out, 0)
	}

	out = append(out, opts.Trailing...)

	length := uint32(len(out))
	if opts.StatedLength != 0 {
		length = opts.StatedLength
	}
	binary.BigEndian.PutUint32(out[8:], length)
	return out
}

// WOFF2TransformHmtx returns the transformed form of an hmtx table: the
// advances kept, and the left side bearings dropped wherever they are already
// the left edge of the outline.
//
// This is W3C WOFF 2.0 §5.4 from the writing end, and it is here rather than in
// the decoder because it is what makes a fixture for the reading end. The flag
// byte says which of the two runs of bearings were dropped; this drops both,
// which is what the transform is for.
//
// A font using it must also transform glyf, and not because the format says so:
// the bearings are put back from the left edge of each outline, and the only
// thing that knows those is the code that rebuilds the outlines. A WOFF 2 with
// a transformed hmtx and an untransformed glyf is refused by every decoder
// there is, this one included.
func WOFF2TransformHmtx(hmtx []byte, numGlyphs, numHMetrics int) []byte {
	out := []byte{0x03} // neither run of bearings is present
	for i := 0; i < numHMetrics; i++ {
		out = append(out, hmtx[4*i], hmtx[4*i+1])
	}
	return out
}

// woff2Known is the format's list of tags that may be named by index, in the
// order that gives them their numbers.
var woff2Known = [63]string{
	"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post",
	"cvt ", "fpgm", "glyf", "loca", "prep", "CFF ", "VORG", "EBDT",
	"EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea",
	"vmtx", "BASE", "GDEF", "GPOS", "GSUB", "EBSC", "JSTF", "MATH",
	"CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt", "avar",
	"bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx", "fvar",
	"gvar", "hsty", "just", "lcar", "mort", "morx", "opbd", "prop",
	"trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill",
}

func woff2Tag(s string) uint32 {
	var b [4]byte
	copy(b[:], "    ")
	copy(b[:], s)
	return binary.BigEndian.Uint32(b[:])
}

// appendBase128 writes a length seven bits at a time, most significant first,
// with the top bit set on every byte but the last.
func appendBase128(dst []byte, v uint32) []byte {
	n := 1
	for x := v; x >= 128; x >>= 7 {
		n++
	}
	for i := 0; i < n; i++ {
		b := byte(v>>(7*uint(n-i-1))) & 0x7f
		if i < n-1 {
			b |= 0x80
		}
		dst = append(dst, b)
	}
	return dst
}

// storedBrotli wraps bytes in a Brotli stream that does not compress them.
//
// RFC 7932 §9.2's uncompressed meta-block: a header, padding to a byte
// boundary, and then the bytes. A meta-block holds at most 65536 of them, so a
// longer payload becomes several, and the stream ends with the empty last
// meta-block that says there are no more.
func storedBrotli(data []byte) []byte {
	w := &brotliBits{}
	w.write(0, 1) // a 16-bit window, which is the one-bit spelling
	for len(data) > 0 {
		n := len(data)
		if n > 1<<16 {
			n = 1 << 16
		}
		w.write(0, 1)            // not the last meta-block
		w.write(0, 2)            // four nibbles of length
		w.write(uint32(n-1), 16) //
		w.write(1, 1)            // stored rather than compressed
		for w.n%8 != 0 {
			w.write(0, 1)
		}
		w.out = append(w.out, data[:n]...)
		w.n += uint(n) * 8
		data = data[n:]
	}
	w.write(1, 1) // the last meta-block
	w.write(1, 1) // and it is empty
	return w.out
}

// brotliBits writes bits into bytes least-significant first, which is how
// Brotli packs them.
type brotliBits struct {
	out []byte
	n   uint
}

func (w *brotliBits) write(v uint32, bits uint) {
	for i := uint(0); i < bits; i++ {
		if w.n%8 == 0 {
			w.out = append(w.out, 0)
		}
		if v&(1<<i) != 0 {
			w.out[len(w.out)-1] |= 1 << (w.n % 8)
		}
		w.n++
	}
}

// SortWOFF2Tables puts tables in tag order, which is the order a real font's
// directory is in and is not the order they have to be written in.
func SortWOFF2Tables(tables []WOFF2Table) {
	sort.SliceStable(tables, func(i, j int) bool { return tables[i].Tag < tables[j].Tag })
}
