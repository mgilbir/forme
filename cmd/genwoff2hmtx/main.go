// Command genwoff2hmtx makes a WOFF 2 that uses the hmtx transform.
//
// W3C WOFF 2.0 §5.4 lets a font drop the left side bearings from hmtx wherever
// they are already the left edge of the outline, because the decoder can read
// them back off glyf. No compressor in circulation emits one — google/woff2 has
// the code and does not call it — so the only way to test the decoder's side is
// to build a font that uses it, and the only fonts that can are ones whose glyf
// is already transformed, since the bearings come from the rebuilt outlines.
//
// So this takes a WOFF 2 that has a transformed glyf, leaves that transform
// alone, and writes the same font back with its hmtx transformed as well. The
// result must decode to exactly the font the input decodes to, which is what
// font/woff2_test.go asserts and what makes the fixture worth keeping.
//
//	go run ./cmd/genwoff2hmtx in.woff2 out.woff2
//
// It needs the `brotli` command on the path, to compress the font data it
// rewrites. Stored blocks would do and would work with no tools at all, but a
// fixture that is going to be committed is worth making the size a real font is.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mgilbir/forme/brotli"
	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: genwoff2hmtx <in.woff2> <out.woff2>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err.Error())
	}
	// The font as it will be, which is where the bearings and the outlines are
	// compared. Reading it through the decoder under test is fine here and only
	// here: what the fixture has to be right about is checked against a second
	// decoder later, and this is choosing which bearings may be dropped.
	sfnt, err := font.DecodeWOFF2(data)
	if err != nil {
		fail("the input is not a readable WOFF 2: " + err.Error())
	}
	tables := font.SFNTTables(sfnt)

	dir, streamStart := readDirectory(data)
	compressedLength := binary.BigEndian.Uint32(data[20:])
	var total uint32
	for _, t := range dir {
		total = t.srcOffset + t.srcLength
	}
	body, err := brotli.Decode(data[streamStart:streamStart+int(compressedLength)], int(total))
	if err != nil {
		fail("the input's font data could not be decompressed: " + err.Error())
	}

	transformed, dropped := transformHmtx(tables)
	if !dropped {
		fail("no run of bearings in this font can be dropped, so it cannot make the fixture")
	}

	// Rebuilt in the input's own order, because glyf has to come before loca
	// and hhea before hmtx.
	var list []fonttest.WOFF2Table
	for _, t := range dir {
		tag := tagString(t.tag)
		w := fonttest.WOFF2Table{Tag: tag, Data: tables[tag]}
		switch {
		case tag == "hmtx":
			w.Transform = transformed
		case t.transformed:
			w.Transformed = true
			w.Transform = body[t.srcOffset : t.srcOffset+t.srcLength]
		}
		list = append(list, w)
	}

	var stream []byte
	for _, w := range list {
		if w.Transformed || len(w.Transform) > 0 {
			stream = append(stream, w.Transform...)
		} else {
			stream = append(stream, w.Data...)
		}
	}
	out := fonttest.WOFF2(fonttest.WOFF2Options{
		Flavor:     binary.BigEndian.Uint32(data[4:]),
		Tables:     list,
		Compressed: compress(stream),
	})
	if err := os.WriteFile(os.Args[2], out, 0o644); err != nil {
		fail(err.Error())
	}
	fmt.Fprintf(os.Stderr, "genwoff2hmtx: %s, %d bytes\n", os.Args[2], len(out))
}

// transformHmtx drops the bearings that can be recovered.
//
// They come in two runs — the glyphs with an advance of their own, and the ones
// after them that share the last — and the flag byte says which runs are still
// there. A run may be dropped only if every bearing in it already equals the
// left edge of its glyph, because that is what will be put back.
func transformHmtx(tables map[string][]byte) ([]byte, bool) {
	head, hhea, hmtx := tables["head"], tables["hhea"], tables["hmtx"]
	glyf, loca, maxp := tables["glyf"], tables["loca"], tables["maxp"]
	if head == nil || hhea == nil || hmtx == nil || glyf == nil || loca == nil || maxp == nil {
		fail("the input is missing a table this needs")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:]))
	long := binary.BigEndian.Uint16(head[50:]) != 0
	at := func(i int) int {
		if long {
			return int(binary.BigEndian.Uint32(loca[4*i:]))
		}
		return int(binary.BigEndian.Uint16(loca[2*i:])) * 2
	}
	// A glyph with no outline has no left edge, and the decoder puts back a
	// zero for it.
	xMin := make([]int16, numGlyphs)
	for i := 0; i < numGlyphs; i++ {
		if a, b := at(i), at(i+1); b > a {
			xMin[i] = int16(binary.BigEndian.Uint16(glyf[a+2:]))
		}
	}
	lsb := func(i int) int16 {
		if i < numHMetrics {
			return int16(binary.BigEndian.Uint16(hmtx[4*i+2:]))
		}
		return int16(binary.BigEndian.Uint16(hmtx[4*numHMetrics+2*(i-numHMetrics):]))
	}
	dropProportional, dropMonospace := true, numGlyphs > numHMetrics
	for i := 0; i < numGlyphs; i++ {
		if lsb(i) == xMin[i] {
			continue
		}
		if i < numHMetrics {
			dropProportional = false
		} else {
			dropMonospace = false
		}
	}
	if !dropProportional && !dropMonospace {
		return nil, false
	}
	var flags byte
	if dropProportional {
		flags |= 1
	}
	if dropMonospace {
		flags |= 2
	}
	out := []byte{flags}
	for i := 0; i < numHMetrics; i++ {
		out = append(out, hmtx[4*i], hmtx[4*i+1])
	}
	for i := 0; i < numGlyphs; i++ {
		keep := !dropProportional
		if i >= numHMetrics {
			keep = !dropMonospace
		}
		if keep {
			out = binary.BigEndian.AppendUint16(out, uint16(lsb(i)))
		}
	}
	return out, true
}

// entry is one table directory record, as far as this needs it.
type entry struct {
	tag         uint32
	transformed bool
	srcOffset   uint32
	srcLength   uint32
}

// readDirectory reads the table directory. It is a second reader of the format
// rather than a call into the one under test, which is the point: a fixture
// built by the decoder it is meant to check would agree with it whatever either
// of them did.
func readDirectory(data []byte) ([]entry, int) {
	if len(data) < 48 || binary.BigEndian.Uint32(data) != 0x774F4632 {
		fail("the input is not a WOFF 2")
	}
	n := int(binary.BigEndian.Uint16(data[12:]))
	at := 48
	u8 := func() byte {
		if at >= len(data) {
			fail("the table directory is cut short")
		}
		v := data[at]
		at++
		return v
	}
	base128 := func() uint32 {
		var v uint32
		for {
			b := u8()
			v = v<<7 | uint32(b&0x7f)
			if b&0x80 == 0 {
				return v
			}
		}
	}
	var out []entry
	var offset uint32
	for i := 0; i < n; i++ {
		flags := u8()
		var e entry
		if known := flags & 0x3f; known == 0x3f {
			e.tag = binary.BigEndian.Uint32(data[at:])
			at += 4
		} else {
			e.tag = knownTags[known]
		}
		version := (flags >> 6) & 3
		tag := tagString(e.tag)
		if tag == "glyf" || tag == "loca" {
			e.transformed = version == 0
		} else {
			e.transformed = version != 0
		}
		// The length it will have once rebuilt, and then — only when it is
		// transformed — the length it has in the stream.
		e.srcLength = base128()
		if e.transformed {
			e.srcLength = base128()
		}
		e.srcOffset = offset
		offset += e.srcLength
		out = append(out, e)
	}
	return out, at
}

// knownTags is W3C WOFF 2.0 §5.2's list of tags a directory may name by index.
// It is written out here rather than shared with the decoder for the same
// reason the directory is read again: a fixture that borrows the thing it is
// meant to check is not a check.
var knownTags = [63]uint32{
	tag("cmap"), tag("head"), tag("hhea"), tag("hmtx"), tag("maxp"), tag("name"),
	tag("OS/2"), tag("post"), tag("cvt "), tag("fpgm"), tag("glyf"), tag("loca"),
	tag("prep"), tag("CFF "), tag("VORG"), tag("EBDT"), tag("EBLC"), tag("gasp"),
	tag("hdmx"), tag("kern"), tag("LTSH"), tag("PCLT"), tag("VDMX"), tag("vhea"),
	tag("vmtx"), tag("BASE"), tag("GDEF"), tag("GPOS"), tag("GSUB"), tag("EBSC"),
	tag("JSTF"), tag("MATH"), tag("CBDT"), tag("CBLC"), tag("COLR"), tag("CPAL"),
	tag("SVG "), tag("sbix"), tag("acnt"), tag("avar"), tag("bdat"), tag("bloc"),
	tag("bsln"), tag("cvar"), tag("fdsc"), tag("feat"), tag("fmtx"), tag("fvar"),
	tag("gvar"), tag("hsty"), tag("just"), tag("lcar"), tag("mort"), tag("morx"),
	tag("opbd"), tag("prop"), tag("trak"), tag("Zapf"), tag("Silf"), tag("Glat"),
	tag("Gloc"), tag("Feat"), tag("Sill"),
}

func tag(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

func tagString(tag uint32) string {
	return string([]byte{byte(tag >> 24), byte(tag >> 16), byte(tag >> 8), byte(tag)})
}

// compress runs the font data through the brotli command.
func compress(stream []byte) []byte {
	dir, err := os.MkdirTemp("", "genwoff2hmtx")
	if err != nil {
		fail(err.Error())
	}
	defer os.RemoveAll(dir)
	raw := filepath.Join(dir, "stream")
	if err := os.WriteFile(raw, stream, 0o644); err != nil {
		fail(err.Error())
	}
	packed := filepath.Join(dir, "stream.br")
	cmd := exec.Command("brotli", "-q", "11", "-f", "-o", packed, raw)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fail("running brotli: " + err.Error())
	}
	out, err := os.ReadFile(packed)
	if err != nil {
		fail(err.Error())
	}
	return out
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "genwoff2hmtx: "+msg)
	os.Exit(1)
}
