package shape

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// format12 builds a cmap subtable whose groups are given as start/end/glyph
// triples. Format 12 states coverage as ranges, so a few bytes name as many
// characters as they like — which is the shape the work budget is for.
func format12(groups [][3]uint32) []byte {
	out := make([]byte, 16)
	binary.BigEndian.PutUint16(out[0:], 12)
	binary.BigEndian.PutUint32(out[12:], uint32(len(groups)))
	for _, g := range groups {
		row := make([]byte, 12)
		binary.BigEndian.PutUint32(row[0:], g[0])
		binary.BigEndian.PutUint32(row[4:], g[1])
		binary.BigEndian.PutUint32(row[8:], g[2])
		out = append(out, row...)
	}
	binary.BigEndian.PutUint32(out[4:], uint32(len(out)))
	return out
}

// TestACmapThatNamesMoreThanTheBudgetIsRefused is the bound between a hundred
// bytes of cmap and a walk over tens of millions of characters.
//
// Format 12 states coverage as ranges: twelve bytes name a start, an end and a
// glyph, so one group covers the whole of Unicode. Eight of them is ninety-six
// bytes of font and about nine million mappings, against a budget of four.
//
// Nothing had written one. Every cmap fixture here names a handful of
// characters, so the shape suite passes with maxFontWork raised — and the
// comment on that constant says what it is for: "A font reaching this is
// malformed or hostile; the readers stop rather than spinning, and Load refuses
// it rather than embedding a font it only half knows."
func TestACmapThatNamesMoreThanTheBudgetIsRefused(t *testing.T) {
	// A whole font, with outlines, whose cmap is the one under test. A font of
	// nothing but a cmap is refused for having no outlines, which would be a
	// different answer to a different question.
	fontWith := func(sub []byte) []byte {
		cmap := make([]byte, 12)
		binary.BigEndian.PutUint16(cmap[2:], 1) // one subtable
		binary.BigEndian.PutUint16(cmap[4:], 3) // platform 3
		binary.BigEndian.PutUint16(cmap[6:], 10)
		binary.BigEndian.PutUint32(cmap[8:], 12)
		return fonttest.SFNT(fonttest.SFNTOptions{
			Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 500, HasShape: true}},
			Extra:  map[string][]byte{"cmap": append(cmap, sub...)},
		})
	}

	// A font within the budget loads, or this says nothing about the bound.
	if _, err := Load(fontWith(format12([][3]uint32{{'A', 'Z', 1}}))); err != nil {
		t.Fatalf("a cmap naming twenty-six characters was refused: %v", err)
	}

	var groups [][3]uint32
	for i := 0; i < 8; i++ {
		groups = append(groups, [3]uint32{0, 0x10FFFF, 1})
	}
	big := fontWith(format12(groups))
	if len(big) > 4096 {
		t.Fatalf("the fixture is %d bytes; the point is that it is small", len(big))
	}

	_, err := Load(big)
	if err == nil {
		t.Fatalf("a cmap of %d bytes naming about nine million mappings was loaded; "+
			"the budget is what stops a font from asking for that walk", len(big))
	}
	// The refusal has to be the budget's own — the coverage is unknown because
	// the walk stopped — and not some other thing wrong with the font. Without
	// the bound the walk finishes and the font loads, so this is what moves.
	if !strings.Contains(err.Error(), "character map is truncated") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the "+
			"truncated map the budget stopped, not something else about the font", err)
	}
}
