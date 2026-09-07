package fonttest

import (
	"testing"

	"github.com/mgilbir/forme/font"
)

func TestSyntheticFontParses(t *testing.T) {
	data := SFNT(SFNTOptions{Name: "Probe", Glyphs: []Glyph{
		{Rune: 'A', Advance: 600, HasShape: true},
		{Rune: 'B', Advance: 700, HasShape: true},
		{Rune: ' ', Advance: 250, HasShape: false},
	}})
	fp := font.ParseSFNT(data, 1<<20)
	if fp == nil {
		t.Fatal("this module's own reader rejected the synthetic font")
	}
	if fp.NumGlyphs != 4 {
		t.Errorf("NumGlyphs = %d, want 4 (.notdef + 3)", fp.NumGlyphs)
	}
	for r, wantGID := range map[rune]int{'A': 1, 'B': 2, ' ': 3} {
		if got := fp.Cmap[r]; got != wantGID {
			t.Errorf("cmap[%q] = %d, want %d", r, got, wantGID)
		}
	}
	for gid, want := range map[int]float64{1: 600, 2: 700, 3: 250} {
		if got := fp.WidthByGID[gid]; got != want {
			t.Errorf("width[gid %d] = %v, want %v", gid, got, want)
		}
	}
	if !fp.GlyphNonEmpty[1] || !fp.GlyphNonEmpty[2] {
		t.Error("glyphs given a shape parsed as empty")
	}
	if fp.GlyphNonEmpty[3] {
		t.Error("the space glyph parsed as having an outline")
	}
	if !fp.GlyphPresent[1] || !fp.GlyphPresent[3] {
		t.Error("a glyph parsed as missing from glyf")
	}
}

// TestTheFixtureGlyphHeaderIsTheOutlineItDraws.
//
// A glyph's header states the box its contour fits in, and every reader trusts
// it — checking would mean interpreting the outline, which is what the header
// exists to save. So a fixture whose header disagrees with its own points hands
// each of them a box the fixture does not draw, and a test that then measures
// ink is measuring the header.
//
// simpleSquare rises from a baseline of zero by hi-lo and its header said hi:
// a hundred units of ink, on a thousand-unit em, that are not there.
func TestTheFixtureGlyphHeaderIsTheOutlineItDraws(t *testing.T) {
	for _, em := range []int{1000, 2048, 512} {
		g := simpleSquare(em)
		if len(g) < 10 {
			t.Fatalf("em %d: the glyph is %d bytes", em, len(g))
		}
		be := func(at int) int { return int(int16(uint16(g[at])<<8 | uint16(g[at+1]))) }
		header := [4]int{be(2), be(4), be(6), be(8)} // xMin, yMin, xMax, yMax

		// The points, read as the format writes them: four on-curve points with
		// long vectors, x deltas then y deltas, after the contour ends and the
		// zero instruction length.
		const at = 10 + 2 + 2 + 4 // header, endPts, instructionLength, flags
		var x, y int
		box := [4]int{1 << 30, 1 << 30, -1 << 30, -1 << 30}
		for k := 0; k < 4; k++ {
			x += be(at + 2*k)
			y += be(at + 8 + 2*k)
			box[0] = min(box[0], x)
			box[1] = min(box[1], y)
			box[2] = max(box[2], x)
			box[3] = max(box[3], y)
		}
		if header != box {
			t.Errorf("em %d: the header says %v and the outline draws %v",
				em, header, box)
		}
	}
}
