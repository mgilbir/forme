package fonttest

import (
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A fixture builds what it is asked for, or refuses.
//
// Two builders here produced something other than what they were given, with
// nothing to say so: SFNT wrote a character above U+FFFF into a sixteen-bit
// field and kept the low half, and LigatureSubst found a ligature's offset slot
// by a search two ligatures could both answer. Each fixture was read by the
// same understanding of the format that wrote it, so a test built on either
// passed or failed for a reason unrelated to what it was testing.

// TestASupplementaryCharacterIsTheCharacter: a face built with 𝐀 maps 𝐀, and
// not U+D400, which is where the low sixteen bits of U+1D400 point.
func TestASupplementaryCharacterIsTheCharacter(t *testing.T) {
	data := SFNT(SFNTOptions{Glyphs: []Glyph{
		{Rune: 'A', Advance: 600, HasShape: true},
		{Rune: 0x1D400, Advance: 700, HasShape: true},
		{Rune: 0x10FFFD, Advance: 800, HasShape: true},
	}})
	fp := font.ParseSFNT(data, 1<<20)
	if fp == nil {
		t.Fatal("the font does not parse")
	}
	for r, want := range map[rune]int{'A': 1, 0x1D400: 2, 0x10FFFD: 3} {
		if got := fp.Cmap[r]; got != want {
			t.Errorf("cmap[U+%04X] = %d, want %d", r, got, want)
		}
	}
	for _, r := range []rune{0xD400, 0xFFFD} {
		if gid, ok := fp.Cmap[r]; ok {
			t.Errorf("U+%04X, the low half of a supplementary character, maps glyph %d", r, gid)
		}
	}

	// A font with nothing past the BMP keeps its one (3,1) subtable, byte
	// for byte what it was, since a hundred tests read one.
	bmp := SFNT(SFNTOptions{Glyphs: []Glyph{{Rune: 'A', Advance: 600}}})
	cmap := font.SFNTTables(bmp)["cmap"]
	if n := binary.BigEndian.Uint16(cmap[2:]); n != 1 {
		t.Errorf("a BMP-only font has %d cmap subtables, want 1", n)
	}
}

// TestWhatACmapCannotSayIsRefused: a character twice, the format-4 sentinel,
// a surrogate and a value past the code space, each a panic rather than a
// font that says something else.
func TestWhatACmapCannotSayIsRefused(t *testing.T) {
	for name, glyphs := range map[string][]Glyph{
		"a character twice":    {{Rune: 'A'}, {Rune: 'A'}},
		"U+FFFF":               {{Rune: 0xFFFF}},
		"a surrogate":          {{Rune: 0xD800}},
		"past the code space":  {{Rune: 0x110000}},
		"a negative character": {{Rune: -1}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: built a font", name)
				}
			}()
			SFNT(SFNTOptions{Glyphs: glyphs})
		}()
	}
	defer func() {
		if recover() == nil {
			t.Error("a format-4 segment past U+FFFF was written")
		}
	}()
	CmapFormat4([][3]int{{0x1D400, 0x1D400, 1}, {0xFFFF, 0xFFFF, 1}})
}

// TestWhatAFieldCannotHoldIsRefused: the metrics SFNT writes and the
// ligatures LigatureSubst writes, each out of what its field can hold.
func TestWhatAFieldCannotHoldIsRefused(t *testing.T) {
	for name, build := range map[string]func(){
		"an advance past sixteen bits": func() { SFNT(SFNTOptions{Glyphs: []Glyph{{Rune: 'A', Advance: 70000}}}) },
		"a negative advance":           func() { SFNT(SFNTOptions{Glyphs: []Glyph{{Rune: 'A', Advance: -1}}}) },
		"units per em past its field":  func() { SFNT(SFNTOptions{UnitsPerEm: 0x10000}) },
		"an ascent past int16":         func() { SFNT(SFNTOptions{Ascent: 40000}) },
		"a ligature of one glyph":      func() { LigatureSubst([]Ligature{{Components: []int{1}, Glyph: 2}}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: built", name)
				}
			}()
			build()
		}()
	}
}

// TestEveryLigatureOfASetIsReachable: two ligatures of one first glyph that
// make the same glyph from the same number of components, which is where the
// slot search went wrong — both offsets have to be set, and each has to lead
// to its own ligature.
func TestEveryLigatureOfASetIsReachable(t *testing.T) {
	ligs := []Ligature{
		{Components: []int{1, 2}, Glyph: 5},
		{Components: []int{1, 3}, Glyph: 5},
		{Components: []int{1, 2, 3}, Glyph: 6},
	}
	sub := LigatureSubst(ligs)
	u16 := func(b []byte, at int) int { return int(binary.BigEndian.Uint16(b[at:])) }
	if u16(sub, 0) != 1 || u16(sub, 4) != 1 {
		t.Fatalf("format %d with %d sets, want one set", u16(sub, 0), u16(sub, 4))
	}
	set := sub[u16(sub, 6):]
	if n := u16(set, 0); n != len(ligs) {
		t.Fatalf("the set holds %d ligatures, want %d", n, len(ligs))
	}
	for i, want := range ligs {
		off := u16(set, 2+2*i)
		if off < 2+2*len(ligs) {
			t.Errorf("ligature %d's offset is %d, inside the set's own header", i, off)
			continue
		}
		lig := set[off:]
		comps := []int{1}
		for k := 0; k < u16(lig, 2)-1; k++ {
			comps = append(comps, u16(lig, 4+2*k))
		}
		if u16(lig, 0) != want.Glyph || len(comps) != len(want.Components) {
			t.Errorf("ligature %d reads as %v→%d, want %v→%d", i, comps, u16(lig, 0), want.Components, want.Glyph)
			continue
		}
		for k := range comps {
			if comps[k] != want.Components[k] {
				t.Errorf("ligature %d reads as %v→%d, want %v→%d", i, comps, u16(lig, 0), want.Components, want.Glyph)
				break
			}
		}
	}
}
