package shape

import (
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/font"
)

// TestATableStatedPastTheFileIsReadAsHarfBuzzReadsIt states Noto Sans's GPOS
// as running 100,000 bytes past the end of the file. HarfBuzz reads the bytes
// that are there, and kerns A and V as it does in the font as written: 599,
// 560, 599, 600 (uharfbuzz 0.56.2). The table was dropped here, and the pair
// went unkerned.
func TestATableStatedPastTheFileIsReadAsHarfBuzzReadsIt(t *testing.T) {
	data := append([]byte(nil), notoSansBytes(t)...)
	found := false
	for i := 0; i < font.Be16(data, 4); i++ {
		rec := 12 + 16*i
		if string(data[rec:rec+4]) == "GPOS" {
			off := int(font.Be32(data, rec+8))
			binary.BigEndian.PutUint32(data[rec+12:], uint32(len(data)-off+100000))
			found = true
		}
	}
	if !found {
		t.Fatal("the face has no GPOS")
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs("AVAV")
	want := []float64{599, 560, 599, 600}
	if len(glyphs) != len(want) {
		t.Fatalf("AVAV shapes to %d glyphs", len(glyphs))
	}
	for i, g := range glyphs {
		if g.XAdvance != want[i] {
			t.Errorf("glyph %d advances %v, and HarfBuzz %v", i, g.XAdvance, want[i])
		}
	}
}
