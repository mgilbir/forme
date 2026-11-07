package shape

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// BenchmarkSubsetGlyphs is what a caller embedding one subset per document
// per font pays per document (#985): a fresh clone, the glyphs of a short
// label, and the subset.
func BenchmarkSubsetGlyphs(b *testing.B) {
	for _, tc := range []struct {
		name string
		data func(testing.TB) []byte
	}{
		{"NotoSans-Regular", func(t testing.TB) []byte { return fonttest.NotoFile(t, "NotoSans-Regular.ttf") }},
		{"SourceSans3-Regular", func(t testing.TB) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") }},
		{"NotoSansJP-Regular", func(t testing.TB) []byte { return fonttest.CJKFile(t, "NotoSansJP-Regular.otf") }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			face, err := Load(tc.data(b))
			if err != nil {
				b.Fatal(err)
			}
			var gids []int
			for _, r := range "Measurement 0123456789" {
				g, _ := face.CmapGlyph(r)
				gids = append(gids, g)
			}
			size := 0
			b.ReportAllocs()
			for b.Loop() {
				c := face.Clone()
				c.Use(gids...)
				p, _, err := c.SubsetGlyphs()
				if err != nil {
					b.Fatal(err)
				}
				size = len(p)
			}
			b.ReportMetric(float64(size), "program-B")
		})
	}
}

// TestAssembleSFNTAllocatesTheProgramOnce (#985): the program's length is
// known before a byte of it is written, so it is allocated once, not grown by
// each table; and every table reads back where the directory puts it, on a
// four-byte boundary, with the last one unpadded.
func TestAssembleSFNTAllocatesTheProgramOnce(t *testing.T) {
	tables := map[string][]byte{
		"cmap": make([]byte, 5),
		"glyf": make([]byte, 4001),
		"head": make([]byte, 54),
		"loca": make([]byte, 8),
		"name": make([]byte, 3),
	}
	for tag, b := range tables {
		for i := range b {
			b[i] = tag[0] + byte(i)
		}
	}
	var out []byte
	// The tag list, the directory and the program.
	if allocs := testing.AllocsPerRun(20, func() { out = assembleSFNT(tables) }); allocs > 3 {
		t.Errorf("assembleSFNT allocates %v times, want 3", allocs)
	}
	got := font.SFNTTables(out)
	for tag, want := range tables {
		if !bytes.Equal(got[tag], want) {
			t.Errorf("%s reads back as %d bytes that differ", tag, len(got[tag]))
		}
	}
	for i := 0; i < len(tables); i++ {
		if off := binary.BigEndian.Uint32(out[12+16*i+8:]); off%4 != 0 {
			t.Errorf("table %d starts at %d", i, off)
		}
	}
	if want := 12 + 16*5 + 8 + 4004 + 56 + 8 + 3; len(out) != want {
		t.Errorf("the program is %d bytes, want %d", len(out), want)
	}
}
