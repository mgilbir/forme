package font

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/mgilbir/forme/fonttest"
)

// The cmap formats added after 0, 4, 6 and 12: 8 (mixed 16/32-bit), 10 (trimmed
// 32-bit array) and 13 (many-to-one). A font whose only Unicode subtable is one
// of these came back with no character map at all, which callers read as
// "unknown" and stopped checking — so every glyph in it went unverified.

// TestCmapFormat13MapsEveryCodeToOneGlyph is the defining property of format 13
// and the one thing that separates it from format 12, whose bytes are otherwise
// identical: a group's startGlyphID is *the* glyph for the whole range, not the
// first of a run.
//
// The two are read from the same bytes here, so a parser that treated 13 as 12 —
// the easy mistake, since the header and group array are the same — fails on the
// second code of the range rather than passing by coincidence.
func TestCmapFormat13MapsEveryCodeToOneGlyph(t *testing.T) {
	groups := [][3]uint32{{0x0041, 0x0045, 700}}
	m13, _ := ParseCmapSubtable(fonttest.CmapFormat13(groups), generousCmapWork)
	if len(m13) != 5 {
		t.Fatalf("format 13: %d entries, want the five codes U+0041..U+0045: %v", len(m13), m13)
	}
	for c := rune(0x41); c <= 0x45; c++ {
		if m13[c] != 700 {
			t.Errorf("format 13: cmap[U+%04X] = %d, want 700 for every code in the group", c, m13[c])
		}
	}

	// And the same bytes as a format 12 walk the glyph ids, so this is not
	// passing because the parser ignores startGlyphID.
	m12, _ := ParseCmapSubtable(fonttest.CmapFormat12(groups), generousCmapWork)
	for c := rune(0x41); c <= 0x45; c++ {
		if want := 700 + int(c-0x41); m12[c] != want {
			t.Errorf("format 12: cmap[U+%04X] = %d, want %d", c, m12[c], want)
		}
	}
}

// TestCmapFormat13Astral: like format 12, format 13 carries 32-bit codes, and
// reaching past the BMP is most of why a font would use it.
func TestCmapFormat13Astral(t *testing.T) {
	m, _ := ParseCmapSubtable(fonttest.CmapFormat13([][3]uint32{
		{0x1F600, 0x1F602, 42},
	}), generousCmapWork)
	if len(m) != 3 || m[0x1F600] != 42 || m[0x1F601] != 42 || m[0x1F602] != 42 {
		t.Errorf("format 13 past the BMP: got %v, want U+1F600..U+1F602 all -> 42", m)
	}
}

// TestCmapFormat13Budget. Format 13 is the format that can actually reach the
// expansion ceiling: a sequential group runs out of 16-bit glyph ids after 65536
// codes and the rest are dropped, but a many-to-one group's glyph stays valid for
// as long as the group runs, so a single group spanning Unicode records every one
// of its 1.1M codes. One line of a font's bytes, so the budget has to hold.
func TestCmapFormat13Budget(t *testing.T) {
	// Sixty-four groups each spanning the whole of Unicode: 71M iterations and,
	// unbudgeted, a map of every code point.
	groups := make([][3]uint32, 64)
	for i := range groups {
		groups[i] = [3]uint32{0, 0x10FFFF, uint32(i + 1)}
	}
	b := fonttest.CmapFormat13(groups)
	done := make(chan int, 1)
	start := time.Now()
	go func() {
		m, _ := ParseCmapSubtable(b, generousCmapWork)
		done <- len(m)
	}()
	select {
	case n := <-done:
		t.Logf("format 13 full-range groups: %d entries in %v", n, time.Since(start))
		if n == 0 {
			t.Errorf("returned nothing; want the partial map read so far")
		}
		if n > 1<<18 {
			t.Errorf("expanded to %d entries, want at most %d", n, 1<<18)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("ParseCmapSubtable did not terminate within the work budget")
	}
}

// TestCmapFormat13ReportsItsBudget: a table cut short by the budget must say so,
// because the mappings it did read are correct but the ones it did not are
// unknown — and a caller that reads them as absent reports a font-wide fault.
func TestCmapFormat13ReportsItsBudget(t *testing.T) {
	m, partial := ParseCmapSubtable(fonttest.CmapFormat13([][3]uint32{{0, 0x10FFFF, 9}}), 4096)
	if !partial {
		t.Errorf("a group of 0x110000 codes read under a 4096 budget reported complete")
	}
	if len(m) == 0 {
		t.Errorf("nothing read at all; want the prefix the budget allowed")
	}
}

// TestCmapFormat8IsReadAsItsGroups. Format 8's is32 bitmap says how to cut a byte
// *stream* into character codes; the codes are already cut in the groups, so it
// says nothing about the mapping. A table whose bitmap is all ones must read the
// same as one whose bitmap is all zeros — otherwise the parser is letting 8192
// bytes of irrelevance change a font's character map.
func TestCmapFormat8IsReadAsItsGroups(t *testing.T) {
	groups := [][3]uint32{{0x0041, 0x0043, 10}, {0x1F600, 0x1F600, 99}}
	want := map[rune]int{0x41: 10, 0x42: 11, 0x43: 12, 0x1F600: 99}

	zeros := fonttest.CmapFormat8(groups)
	ones := append([]byte(nil), zeros...)
	for i := 12; i < 12+8192; i++ {
		ones[i] = 0xFF
	}

	for name, b := range map[string][]byte{"is32 all zero": zeros, "is32 all one": ones} {
		m, _ := ParseCmapSubtable(b, generousCmapWork)
		if len(m) != len(want) {
			t.Errorf("%s: %d entries, want %d: %v", name, len(m), len(want), m)
		}
		for c, gid := range want {
			if m[c] != gid {
				t.Errorf("%s: cmap[U+%04X] = %d, want %d", name, c, m[c], gid)
			}
		}
	}
}

// TestCmapFormat10Runs: the 32-bit twin of format 6. A glyph id of zero is
// .notdef and is not a mapping, exactly as in format 6.
func TestCmapFormat10Runs(t *testing.T) {
	m, _ := ParseCmapSubtable(fonttest.CmapFormat10(0x1F600, []uint16{5, 0, 7}), generousCmapWork)
	if len(m) != 2 || m[0x1F600] != 5 || m[0x1F602] != 7 {
		t.Errorf("format 10: got %v, want U+1F600->5 and U+1F602->7 with the .notdef dropped", m)
	}
}

// TestCmapFormat10PastUnicode ensures entries running past U+10FFFF are dropped
// rather than recorded under a key that is not a character. The run only climbs,
// so everything after the first out-of-range code goes too.
func TestCmapFormat10PastUnicode(t *testing.T) {
	m, _ := ParseCmapSubtable(fonttest.CmapFormat10(0x10FFFE, []uint16{1, 2, 3, 4}), generousCmapWork)
	if len(m) != 2 || m[0x10FFFE] != 1 || m[0x10FFFF] != 2 {
		t.Errorf("format 10 past Unicode: got %v, want only U+10FFFE->1 and U+10FFFF->2", m)
	}

	// A startCharCode past Unicode maps nothing at all, and "nothing" is nil.
	if m, _ := ParseCmapSubtable(fonttest.CmapFormat10(0xFFFFFF00, []uint16{1, 2}), generousCmapWork); m != nil {
		t.Errorf("format 10 starting past Unicode: got %v, want nil", m)
	}
}

// TestCmapNewFormatsRefuseTruncation. A table claiming more than it carries must
// come back nil rather than half-read: callers treat a non-nil map as
// authoritative, so a prefix presented as complete turns every code the table
// never reached into a .notdef finding.
func TestCmapNewFormatsRefuseTruncation(t *testing.T) {
	f13 := fonttest.CmapFormat13([][3]uint32{{0x41, 0x42, 7}})
	f8 := fonttest.CmapFormat8([][3]uint32{{0x41, 0x42, 7}})
	f10 := fonttest.CmapFormat10(0x41, []uint16{7, 8})

	cases := map[string][]byte{
		"13, body cut off":   f13[:len(f13)-4],
		"13, header cut off": f13[:12],
		"13, nGroups overstated": func() []byte {
			b := append([]byte(nil), f13...)
			binary.BigEndian.PutUint32(b[12:], 1<<20)
			return b
		}(),
		"13, length past buffer": func() []byte {
			b := append([]byte(nil), f13...)
			binary.BigEndian.PutUint32(b[4:], uint32(len(b))+1)
			return b
		}(),

		"8, body cut off":   f8[:len(f8)-4],
		"8, is32 cut off":   f8[:4000],
		"8, header cut off": f8[:12],
		"8, nGroups overstated": func() []byte {
			b := append([]byte(nil), f8...)
			binary.BigEndian.PutUint32(b[12+8192:], 1<<20)
			return b
		}(),
		"8, length past buffer": func() []byte {
			b := append([]byte(nil), f8...)
			binary.BigEndian.PutUint32(b[4:], uint32(len(b))+1)
			return b
		}(),
		"8, length too small": func() []byte {
			b := append([]byte(nil), f8...)
			binary.BigEndian.PutUint32(b[4:], 16)
			return b
		}(),

		"10, body cut off":   f10[:len(f10)-2],
		"10, header cut off": f10[:16],
		"10, numChars overstated": func() []byte {
			b := append([]byte(nil), f10...)
			binary.BigEndian.PutUint32(b[16:], 1<<20)
			return b
		}(),
		"10, length past buffer": func() []byte {
			b := append([]byte(nil), f10...)
			binary.BigEndian.PutUint32(b[4:], uint32(len(b))+1)
			return b
		}(),
		"10, length too small": func() []byte {
			b := append([]byte(nil), f10...)
			binary.BigEndian.PutUint32(b[4:], 12)
			return b
		}(),
	}
	for name, b := range cases {
		if m, _ := ParseCmapSubtable(b, generousCmapWork); m != nil {
			t.Errorf("format %s: got %d entries, want nil", name, len(m))
		}
	}
}

// TestCmapNewFormatsMappingNothingIsNil: the same rule the other formats keep. A
// well-formed subtable that happens to map no character is nil, not an empty map
// — an empty one answers ".notdef" for every code in the font.
func TestCmapNewFormatsMappingNothingIsNil(t *testing.T) {
	cases := map[string][]byte{
		"13, no groups":   fonttest.CmapFormat13(nil),
		"8, no groups":    fonttest.CmapFormat8(nil),
		"10, no entries":  fonttest.CmapFormat10(0x41, nil),
		"10, all .notdef": fonttest.CmapFormat10(0x41, []uint16{0, 0, 0}),
		"13, group outside Unicode": fonttest.CmapFormat13([][3]uint32{
			{0x30303030, 0x30303030, 7},
		}),
		"8, glyph id wider than 16 bits": fonttest.CmapFormat8([][3]uint32{
			{0x41, 0x41, 0x10000},
		}),
	}
	for name, b := range cases {
		if m, _ := ParseCmapSubtable(b, generousCmapWork); m != nil {
			t.Errorf("format %s: got %d entries, want nil", name, len(m))
		}
	}
}

// TestCmapNewFormatsSkipMalformedGroups: an inverted group, one wholly past
// Unicode and a glyph id wider than 16 bits are each skipped without disturbing
// the group beside them. Format 13 shares the group loop with 12, so this is what
// says the sharing did not lose 12's checks.
func TestCmapNewFormatsSkipMalformedGroups(t *testing.T) {
	bad := [][3]uint32{
		{0x0050, 0x0040, 300},     // inverted
		{0x110000, 0x110002, 400}, // past the end of Unicode
		{0x0060, 0x0060, 0x10000}, // glyph id wider than 16 bits
		{0x0041, 0x0041, 200},
	}
	for name, b := range map[string][]byte{
		"13": fonttest.CmapFormat13(bad),
		"8":  fonttest.CmapFormat8(bad),
	} {
		m, _ := ParseCmapSubtable(b, generousCmapWork)
		if len(m) != 1 || m[0x41] != 200 {
			t.Errorf("format %s malformed groups: got %v, want only U+0041->200", name, m)
		}
	}
}

// TestAFontWhoseOnlyUnicodeSubtableIsFormat13 is the issue itself, at the level a
// caller sees it: ParseSFNT hands back a character map instead of nil, so the
// font's glyphs can be resolved at all.
func TestAFontWhoseOnlyUnicodeSubtableIsFormat13(t *testing.T) {
	type sub = fonttest.CmapSub
	for name, data := range map[string][]byte{
		"13": fonttest.CmapFormat13([][3]uint32{{0x0041, 0x0043, 77}}),
		"8":  fonttest.CmapFormat8([][3]uint32{{0x0041, 0x0041, 77}}),
		"10": fonttest.CmapFormat10(0x0041, []uint16{77}),
	} {
		fp := ParseSFNT(fonttest.SFNTWithCmapSubtables([]sub{
			{Plat: 3, Enc: 10, Data: data},
		}), generousCmapWork)
		if fp == nil {
			t.Fatalf("format %s: ParseSFNT returned nil", name)
		}
		if fp.Cmap == nil {
			t.Errorf("format %s: no character map at all; the font's glyphs cannot be resolved", name)
			continue
		}
		if fp.Cmap[0x41] != 77 {
			t.Errorf("format %s: cmap[U+0041] = %d, want 77", name, fp.Cmap[0x41])
		}
	}
}

// TestFormat14DoesNotDisplaceARealCmap. Format 14 maps (base, selector) pairs,
// not codes, so it has no character map to give and comes back nil. What matters
// is that the nil is inert: the (0,5) slot it sits in is ranked as a Unicode
// subtable, so a parser that let it win would leave a font carrying a perfectly
// good format-4 table with no cmap.
func TestFormat14DoesNotDisplaceARealCmap(t *testing.T) {
	type sub = fonttest.CmapSub
	f14 := make([]byte, 10)
	f14[1] = 14
	binary.BigEndian.PutUint32(f14[2:], uint32(len(f14)))
	bmp := fonttest.CmapFormat4([][3]int{{0x0041, 0x0041, 100 - 0x41}, {0xFFFF, 0xFFFF, 1}})

	for _, order := range [][]sub{
		{{Plat: 0, Enc: 3, Data: bmp}, {Plat: 0, Enc: 5, Data: f14}},
		{{Plat: 0, Enc: 5, Data: f14}, {Plat: 0, Enc: 3, Data: bmp}},
	} {
		fp := ParseSFNT(fonttest.SFNTWithCmapSubtables(order), generousCmapWork)
		if fp.Cmap[0x41] != 100 {
			t.Errorf("order %v: cmap[U+0041] = %d, want the format-4 mapping 100", order, fp.Cmap[0x41])
		}
	}
}
