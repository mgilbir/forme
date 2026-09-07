package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The two grids a face answers on, and which answer is on which.
//
// A Descriptor's lengths are the font's own units — whatever grid its head table
// declares. An advance is not: font.FontProgram scales the width table to a
// thousandth of an em on the way in, so that every format stating widths is
// handed one grid instead of each doing the division itself.
//
// The documentation said "font units" for both, and for a long time nothing
// caught it: the bundled face has a thousand units to the em, so the two numbers
// are the same number. This face does not.
func TestAnAdvanceAndADescriptorAreOnDifferentGrids(t *testing.T) {
	const (
		upem    = 2048
		ascent  = 1638
		descent = -410
		// Half an em on this grid, which is 500 on the other.
		advance = 1024
	)
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name:       "TwoGrids",
		UnitsPerEm: upem,
		Ascent:     ascent,
		Descent:    descent,
		Glyphs:     []fonttest.Glyph{{Rune: 'A', Advance: advance, HasShape: true}},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.UnitsPerEm(); got != upem {
		t.Fatalf("the face says %d units to the em, and the fixture declares %d",
			got, upem)
	}
	gid, ok := f.GlyphID('A')
	if !ok {
		t.Fatal("the fixture has no A")
	}

	// The advance, in thousandths of an em.
	if got, want := f.GlyphAdvance(gid), 1000.0*advance/upem; got != want {
		t.Errorf("GlyphAdvance is %v; the glyph advances %d of %d units, which is "+
			"%v thousandths of an em. %v would be the font's own units",
			got, advance, upem, want, float64(advance))
	}
	if got := f.GlyphAdvances()[gid]; got != f.GlyphAdvance(gid) {
		t.Errorf("the table says %v and the per-glyph answer %v",
			got, f.GlyphAdvance(gid))
	}

	// The descriptor, in the font's own units.
	d := f.Descriptor()
	if d.Ascent != ascent || d.Descent != descent {
		t.Errorf("the descriptor rises %d and falls %d; the font's own hhea says "+
			"%d and %d. Thousandths of an em would be %d and %d",
			d.Ascent, d.Descent, ascent, descent,
			1000*ascent/upem, 1000*descent/upem)
	}
}
