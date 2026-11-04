package font

import (
	"encoding/binary"
	"os"
	"testing"
)

// MarkComposite is ParseSFNT's reading of a composite's components, exported
// for callers outside the module that account a font's glyphs by it.

// TestMarkCompositeIsLenient pins what it marks: each component of a
// composite, nothing for a simple glyph, and — leniently, as ParseSFNT reads
// — nothing for an index past the font, with the records read as far as they
// go.
func TestMarkCompositeIsLenient(t *testing.T) {
	record := func(flags, gid int) []byte {
		return binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, uint16(flags)), uint16(gid))
	}
	header := []byte{0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0}
	const more, words = 0x0020, 0x0001
	var g []byte
	g = append(g, header...)
	g = append(append(g, record(more|words, 42)...), 0, 0, 0, 0)
	g = append(append(g, record(more|words, 999)...), 0, 0, 0, 0) // past the font
	g = append(append(g, record(words, 7)...), 0, 0)              // cut short
	out := make([]bool, 100)
	MarkComposite(g, 100, out)
	for gid, marked := range out {
		if marked != (gid == 42 || gid == 7) {
			t.Errorf("glyph %d marked %v", gid, marked)
		}
	}

	simple := []byte{0x00, 0x02, 0, 0, 0, 0, 0, 0, 0, 0}
	out = make([]bool, 100)
	MarkComposite(simple, 100, out)
	for gid, marked := range out {
		if marked {
			t.Errorf("a simple glyph marked glyph %d", gid)
		}
	}
}

// TestMarkCompositeAgreesWithParseSFNT: over every glyph of the bundled face,
// the components MarkComposite marks are Program.ComponentGID.
func TestMarkCompositeAgreesWithParseSFNT(t *testing.T) {
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p := ParseSFNT(data, 1<<22)
	if p == nil {
		t.Fatal("the bundled face does not parse")
	}
	tables := SFNTTables(data)
	loca, glyf := tables["loca"], tables["glyf"]
	long := Be16(tables["head"], 50) == 1
	got := make([]bool, p.NumGlyphs)
	for gid := 0; gid < p.NumGlyphs; gid++ {
		var start, end int
		if long {
			start, end = int(Be32(loca, 4*gid)), int(Be32(loca, 4*gid+4))
		} else {
			start, end = 2*Be16(loca, 2*gid), 2*Be16(loca, 2*gid+2)
		}
		if start < end && end <= len(glyf) {
			MarkComposite(glyf[start:end], p.NumGlyphs, got)
		}
	}
	components := 0
	for gid, want := range p.ComponentGID {
		if got[gid] != want {
			t.Errorf("glyph %d: MarkComposite marks %v and ParseSFNT %v", gid, got[gid], want)
		}
		if want {
			components++
		}
	}
	if components == 0 {
		t.Fatal("the face uses no glyph as a component; the comparison proves nothing")
	}
}
