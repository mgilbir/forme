package font

import (
	"encoding/binary"
	"maps"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The post table's glyph names, and the name → glyph lookup they give.
//
// Every table here is written by hand against the post chapter of the OpenType
// specification, into a synthetic font whose glyph i+1 advances 100*(i+1) units
// in a 1000-unit em, so a width says which glyph it belongs to.

// postFont is a synthetic font of n glyphs, .notdef included, carrying post.
func postFont(n int, post []byte) *Program {
	glyphs := make([]fonttest.Glyph, n-1)
	for i := range glyphs {
		glyphs[i] = fonttest.Glyph{Rune: rune('A' + i), Advance: 100 * (i + 1)}
	}
	extra := map[string][]byte{}
	if post != nil {
		extra["post"] = post
	}
	return ParseSFNT(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs, Extra: extra}), 1<<20)
}

// postHeader is the 32 bytes every version begins with, version first.
func postHeader(version uint32) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b, version)
	return b
}

// postV2 is a version 2.0 table: the glyph count it states, an index per
// glyph, and the Pascal strings after them. A string longer than 255 bytes is
// not something this writes.
func postV2(count int, index []int, names ...string) []byte {
	b := postHeader(0x00020000)
	b = binary.BigEndian.AppendUint16(b, uint16(count))
	for _, i := range index {
		b = binary.BigEndian.AppendUint16(b, uint16(i))
	}
	for _, s := range names {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	return b
}

// postV25 is a version 2.5 table: the glyph count and one offset per glyph.
func postV25(count int, offsets ...int8) []byte {
	b := postHeader(0x00025000)
	b = binary.BigEndian.AppendUint16(b, uint16(count))
	for _, o := range offsets {
		b = append(b, byte(o))
	}
	return b
}

// wantNames checks the three maps against the one answer they have to agree
// on: which glyph each name selects, and that glyph's width.
func wantNames(t *testing.T, fp *Program, want map[string]int) {
	t.Helper()
	if fp == nil {
		t.Fatal("ParseSFNT returned nil")
	}
	if !maps.Equal(fp.GlyphByName, want) {
		t.Errorf("GlyphByName = %v, want %v", fp.GlyphByName, want)
	}
	if len(fp.GlyphNames) != len(want) {
		t.Errorf("GlyphNames = %v, want the %d names %v", fp.GlyphNames, len(want), want)
	}
	for name, gid := range want {
		if !fp.GlyphNames[name] {
			t.Errorf("GlyphNames lacks %q", name)
		}
		if w, ok := fp.WidthByName[name]; !ok || w != fp.WidthByGID[gid] {
			t.Errorf("WidthByName[%q] = %v, %v; glyph %d advances %v",
				name, w, ok, gid, fp.WidthByGID[gid])
		}
	}
}

// wantNoNames checks that a table giving no names leaves all three maps nil,
// which is how a program that identifies glyphs by index only says so.
func wantNoNames(t *testing.T, fp *Program) {
	t.Helper()
	if fp == nil {
		t.Fatal("ParseSFNT returned nil")
	}
	if fp.GlyphNames != nil || fp.GlyphByName != nil || fp.WidthByName != nil {
		t.Errorf("names from a table that gives none: GlyphNames %v, GlyphByName %v, WidthByName %v",
			fp.GlyphNames, fp.GlyphByName, fp.WidthByName)
	}
}

// TestTheStandardOrder pins the standard Macintosh names where a slip would
// show: both ends, the digits, the letters, and two from the accented middle.
// The compiler counts the array; this checks that what is counted is in the
// right places. The whole list was compared with fontTools'
// standardGlyphOrder when it was written, and these are its answers.
func TestTheStandardOrder(t *testing.T) {
	for i, want := range map[int]string{
		0: ".notdef", 1: ".null", 2: "nonmarkingreturn", 3: "space", 19: "zero",
		28: "nine", 36: "A", 61: "Z", 68: "a", 93: "z", 129: "udieresis",
		158: "ordmasculine", 256: "ccaron", 257: "dcroat",
	} {
		if macStandardNames[i] != want {
			t.Errorf("standard name %d is %q, want %q", i, macStandardNames[i], want)
		}
	}
	seen := map[string]int{}
	for i, name := range macStandardNames {
		if j, dup := seen[name]; dup {
			t.Errorf("standard names %d and %d are both %q", j, i, name)
		}
		seen[name] = i
	}
}

// TestPostVersion1NamesTheStandardOrder: version 1.0 names glyph i the i-th
// standard name, for as many glyphs as the font has.
func TestPostVersion1NamesTheStandardOrder(t *testing.T) {
	wantNames(t, postFont(5, postHeader(0x00010000)), map[string]int{
		".notdef": 0, ".null": 1, "nonmarkingreturn": 2, "space": 3, "exclam": 4,
	})
}

// TestPostVersion2 mixes the two kinds of index: below 258 a standard name,
// from 258 on a string of the table's own, in the order they are written.
func TestPostVersion2(t *testing.T) {
	fp := postFont(5, postV2(5, []int{0, 36, 259, 258, 68}, "f_i", "uni2603"))
	wantNames(t, fp, map[string]int{
		".notdef": 0, "A": 1, "uni2603": 2, "f_i": 3, "a": 4,
	})
}

// TestPostVersion25 adds each glyph's offset to its index: a font that drops
// glyphs 1 and 2 of the standard order names its glyph 1 "space".
func TestPostVersion25(t *testing.T) {
	fp := postFont(4, postV25(4, 0, 2, 2, 33))
	wantNames(t, fp, map[string]int{".notdef": 0, "space": 1, "exclam": 2, "A": 3})
}

// TestPostVersionsWithoutNames: 3.0 says the font names no glyphs, 4.0 maps
// them to character codes rather than names, and any other version is not one
// this can read. None is a name, and a table too short for its own header is
// not a table.
func TestPostVersionsWithoutNames(t *testing.T) {
	for _, tc := range []struct {
		what string
		post []byte
	}{
		{"version 3.0", postHeader(0x00030000)},
		{"version 4.0", append(postHeader(0x00040000), 0, 1, 0, 2, 0, 3)},
		{"version 2.1", append(postHeader(0x00021000), 0, 1, 0, 0)},
		{"no post table", nil},
		{"a header cut short", postHeader(0x00010000)[:31]},
		{"version 2.0 with no glyph count", postHeader(0x00020000)},
		{"version 2.5 with no glyph count", postHeader(0x00025000)},
	} {
		t.Run(tc.what, func(t *testing.T) { wantNoNames(t, postFont(3, tc.post)) })
	}
}

// TestPostCountsDisagreeingWithMaxp: maxp says which glyphs exist and post's
// count says where its index array ends. A name for a glyph maxp does not have
// names nothing; a glyph post's count stops short of is unnamed; and the
// strings are found after post's array however long maxp says the font is.
func TestPostCountsDisagreeingWithMaxp(t *testing.T) {
	t.Run("post counts more glyphs", func(t *testing.T) {
		// Five indices for a font of three glyphs. Were the strings looked
		// for after three indices, the first would start inside the array.
		fp := postFont(3, postV2(5, []int{0, 258, 259, 260, 36}, "one", "two", "three"))
		wantNames(t, fp, map[string]int{".notdef": 0, "one": 1, "two": 2})
	})
	t.Run("post counts fewer glyphs", func(t *testing.T) {
		fp := postFont(5, postV2(2, []int{0, 258}, "one"))
		wantNames(t, fp, map[string]int{".notdef": 0, "one": 1})
	})
	t.Run("version 1.0 in a font of more than 258 glyphs", func(t *testing.T) {
		fp := postFont(300, postHeader(0x00010000))
		if len(fp.GlyphByName) != 258 || fp.GlyphByName["dcroat"] != 257 {
			t.Errorf("%d names, dcroat %d; want 258 names, the last for glyph 257",
				len(fp.GlyphByName), fp.GlyphByName["dcroat"])
		}
	})
	t.Run("version 2.5 counting more glyphs", func(t *testing.T) {
		fp := postFont(2, postV25(4, 0, 2, 2, 33))
		wantNames(t, fp, map[string]int{".notdef": 0, "space": 1})
	})
}

// TestPostMalformedTablesAreBounded is each way a table can point past what it
// holds. What is there is read, and what is not names nothing.
func TestPostMalformedTablesAreBounded(t *testing.T) {
	t.Run("an index array the table does not hold", func(t *testing.T) {
		// Four indices declared and three written. The strings come after
		// the array, so nothing past it can be found and nothing is named.
		post := postV2(4, []int{0, 36, 37})
		wantNoNames(t, postFont(4, post))
	})
	t.Run("an index past the strings", func(t *testing.T) {
		fp := postFont(4, postV2(4, []int{0, 258, 259, 0xFFFF}, "one"))
		wantNames(t, fp, map[string]int{".notdef": 0, "one": 1})
	})
	t.Run("a string running off the end", func(t *testing.T) {
		// "two" claims 200 bytes and has 3, so it is not there; nor is
		// anything after it, since that is found by stepping over it.
		post := postV2(4, []int{0, 258, 259, 260}, "one")
		post = append(post, 200, 't', 'w', 'o')
		wantNames(t, postFont(4, post), map[string]int{".notdef": 0, "one": 1})
	})
	t.Run("an empty string", func(t *testing.T) {
		fp := postFont(3, postV2(3, []int{0, 258, 259}, "", "two"))
		wantNames(t, fp, map[string]int{".notdef": 0, "two": 2})
	})
	t.Run("version 2.5 offsets outside the standard order", func(t *testing.T) {
		// Glyph 1 at -2 and glyph 3 at -4 both land on index -1, before the
		// standard order begins; glyph 2 at +127 lands on 129, "udieresis".
		// And a table that stops before the last glyph's offset is cut
		// inside its array, and names nothing.
		fp := postFont(4, postV25(4, 0, -2, 127, -4))
		wantNames(t, fp, map[string]int{".notdef": 0, "udieresis": 2})
		wantNoNames(t, postFont(4, postV25(4, 0, 1, 1)))
	})
	t.Run("version 2.5 offsets past the standard order", func(t *testing.T) {
		// 200 glyphs at their own index, but glyph 150 at +120, which is
		// index 270 of an order that ends at 257.
		offsets := make([]int8, 200)
		offsets[150] = 120
		fp := postFont(200, postV25(200, offsets...))
		if len(fp.GlyphByName) != 199 || fp.GlyphNames[macStandardNames[150]] {
			t.Errorf("%d names, %q named: want 199, and glyph 150 unnamed",
				len(fp.GlyphByName), macStandardNames[150])
		}
		if fp.GlyphByName[macStandardNames[149]] != 149 || fp.GlyphByName[macStandardNames[151]] != 151 {
			t.Errorf("the glyphs either side of it are named %v", fp.GlyphByName)
		}
	})
}

// TestTwoGlyphsWithOneName: the format does not stop a font naming two glyphs
// alike. The lower index is the glyph the name selects, and the width under it
// is that glyph's, so a consumer reading both maps reads one glyph.
func TestTwoGlyphsWithOneName(t *testing.T) {
	fp := postFont(5, postV2(5, []int{0, 258, 36, 258, 36}, "dup"))
	wantNames(t, fp, map[string]int{".notdef": 0, "dup": 1, "A": 2})
}

// TestCFFGlyphByName: a CFF that is not CID-keyed names its glyphs through its
// charset, and answers the same question by the same rule, the first of two
// glyphs named alike included. A CID-keyed one names none.
func TestCFFGlyphByName(t *testing.T) {
	// Glyph 1 is "A" (SID 34) and advances 50, glyph 2 is "B" (35) and
	// glyph 3 is "A" again advancing 100: a Type 2 charstring's first operand,
	// when it has an odd number, is the width less nominalWidthX.
	fp := ParseCFF(fonttest.CFF(fonttest.CFFOptions{
		Glyphs:      4,
		CharsetSIDs: []int{34, 35, 34},
		Charstrings: [][]byte{{14}, {139 + 50, 14}, {14}, {139 + 100, 14}},
	}))
	if fp == nil {
		t.Fatal("ParseCFF returned nil")
	}
	want := map[string]int{".notdef": 0, "A": 1, "B": 2}
	if !maps.Equal(fp.GlyphByName, want) {
		t.Errorf("GlyphByName = %v, want %v", fp.GlyphByName, want)
	}
	if fp.WidthByName["A"] != fp.WidthByGID[1] || fp.WidthByGID[1] == fp.WidthByGID[3] {
		t.Errorf("WidthByName[A] = %v; glyph 1 advances %v and glyph 3 %v",
			fp.WidthByName["A"], fp.WidthByGID[1], fp.WidthByGID[3])
	}

	cid := ParseCFF(fonttest.CFF(fonttest.CFFOptions{Glyphs: 3, CIDKeyed: true}))
	if cid == nil {
		t.Fatal("ParseCFF returned nil for a CID-keyed font")
	}
	if cid.GlyphByName != nil {
		t.Errorf("a CID-keyed CFF names glyphs: %v", cid.GlyphByName)
	}
}

// FuzzSFNTPost fuzzes the post table on its own, in a font of up to 300
// glyphs, and holds the three maps to what a consumer indexes them by: the same
// names in GlyphNames and GlyphByName, every index a glyph the font has, and
// every width the width of the glyph the name selects. The seeds are each
// version, and the malformed tables above, so the mutations start on the
// fields.
func FuzzSFNTPost(f *testing.F) {
	for _, seed := range []struct {
		glyphs int
		post   []byte
	}{
		{5, postHeader(0x00010000)},
		{300, postHeader(0x00010000)},
		{5, postV2(5, []int{0, 36, 259, 258, 68}, "f_i", "uni2603")},
		{3, postV2(5, []int{0, 258, 259, 260, 36}, "one", "two", "three")},
		{5, postV2(2, []int{0, 258}, "one")},
		{4, append(postV2(4, []int{0, 258, 259, 260}, "one"), 200, 't', 'w', 'o')},
		{5, postV2(5, []int{0, 258, 36, 258, 36}, "dup")},
		{4, postV25(4, 0, 2, 2, 33)},
		{4, postV25(4, 0, -2, 127, -4)},
		{3, postHeader(0x00030000)},
	} {
		f.Add(uint16(seed.glyphs), seed.post)
	}
	f.Fuzz(func(t *testing.T, glyphs uint16, post []byte) {
		fp := postFont(1+int(glyphs)%300, post)
		if fp == nil {
			t.Fatal("ParseSFNT returned nil for a whole font")
		}
		if (fp.GlyphNames == nil) != (fp.GlyphByName == nil) || len(fp.GlyphNames) != len(fp.GlyphByName) {
			t.Fatalf("GlyphNames %v and GlyphByName %v disagree", fp.GlyphNames, fp.GlyphByName)
		}
		if (fp.WidthByName == nil) != (fp.GlyphByName == nil) {
			t.Fatalf("widths %v for the names %v", fp.WidthByName, fp.GlyphByName)
		}
		for name, gid := range fp.GlyphByName {
			if name == "" || !fp.GlyphNames[name] {
				t.Fatalf("GlyphByName names %q, which GlyphNames does not", name)
			}
			if gid < 0 || gid >= fp.NumGlyphs {
				t.Fatalf("%q is glyph %d of %d", name, gid, fp.NumGlyphs)
			}
			if fp.WidthByName[name] != fp.WidthByGID[gid] {
				t.Fatalf("%q advances %v, and glyph %d %v", name, fp.WidthByName[name], gid, fp.WidthByGID[gid])
			}
		}
	})
}
