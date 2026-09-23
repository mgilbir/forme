package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestAGlyphPastTheLocaIsNotInGlyf.
//
// maxp says how many glyphs there are and loca says where each one is, and a
// loca shorter than maxp's count leaves the last glyphs with nowhere. Be16 and
// Be32 answer zero past the end of a table, so those glyphs were read as
// starting and ending at offset zero — present, and empty, which is a blank
// the font never drew and a glyph a caller would take as the font's. They are
// absent: HarfBuzz caps the glyph count at what the loca can address, and the
// subsetter in shape refuses the loca.
func TestAGlyphPastTheLocaIsNotInGlyf(t *testing.T) {
	glyphs := []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
		{Rune: 'c', Advance: 500, HasShape: true},
	}
	whole := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs})
	loca := SFNTTables(whole)["loca"]
	if len(loca) != 4*(len(glyphs)+2) {
		t.Fatalf("the fixture's loca is %d bytes; this test assumes the long form", len(loca))
	}

	// The control: the whole loca addresses every glyph.
	fp := ParseSFNT(whole, 1<<20)
	for gid := 0; gid < fp.NumGlyphs; gid++ {
		if !fp.GlyphPresent[gid] {
			t.Fatalf("glyph %d of the whole font is not present", gid)
		}
	}

	// Three entries: where glyphs 0, 1 and 2 begin. Glyph 1 ends where glyph 2
	// begins; glyph 2 has no end, and glyph 3 neither end nor start.
	short := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: glyphs,
		Extra:  map[string][]byte{"loca": loca[:4*3]},
	})
	fp = ParseSFNT(short, 1<<20)
	if fp.NumGlyphs != 4 {
		t.Fatalf("%d glyphs", fp.NumGlyphs)
	}
	for gid, want := range []bool{true, true, false, false} {
		if fp.GlyphPresent[gid] != want {
			t.Errorf("glyph %d: present %v, want %v", gid, fp.GlyphPresent[gid], want)
		}
		if gid >= 2 && fp.GlyphNonEmpty[gid] {
			t.Errorf("glyph %d, past the loca, has an outline", gid)
		}
	}
	if !fp.GlyphNonEmpty[1] {
		t.Error("glyph 1, which the short loca still addresses, lost its outline")
	}

	// And a short loca in the short form, whose entries are two bytes.
	head := SFNTTables(whole)["head"]
	shortHead := append([]byte(nil), head...)
	shortHead[50], shortHead[51] = 0, 0 // indexToLocFormat: short
	halves := make([]byte, 0, 2*3)
	for i := 0; i < 3; i++ {
		v := uint32(loca[4*i])<<24 | uint32(loca[4*i+1])<<16 | uint32(loca[4*i+2])<<8 | uint32(loca[4*i+3])
		halves = append(halves, byte(v/2>>8), byte(v/2))
	}
	shortForm := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: glyphs,
		Extra:  map[string][]byte{"loca": halves, "head": shortHead},
	})
	fp = ParseSFNT(shortForm, 1<<20)
	for gid, want := range []bool{true, true, false, false} {
		if fp.GlyphPresent[gid] != want {
			t.Errorf("short form, glyph %d: present %v, want %v", gid, fp.GlyphPresent[gid], want)
		}
	}
}
