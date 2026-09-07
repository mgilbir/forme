package font

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A CFF DICT operand is a float64 because CFF's real-number operands are, and a
// font writes whatever it likes into one. A PostScript number is digits, and a
// font writes as many as it likes. Both end up as offsets and lengths, and both
// used to get there through a conversion with no answer for the values a font
// can state.

// TestADictOperandThatIsNotAnOffsetIsRefused pins the conversion, which is the
// check.
func TestADictOperandThatIsNotAnOffsetIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    float64
		want int
		ok   bool
	}{
		{"zero", 0, 0, true},
		{"an ordinary offset", 1234, 1234, true},
		{"the largest a four-byte offset can name", math.MaxInt32, math.MaxInt32, true},
		{"one past it", math.MaxInt32 + 1, 0, false},
		{"a real a font can write", 9.2e18, 0, false},
		{"another", 1e17, 0, false},
		{"beyond every integer", math.MaxFloat64, 0, false},
		{"negative", -1, 0, false},
		{"not a whole number", 1.5, 0, false},
		{"not a number at all", math.NaN(), 0, false},
		{"infinite", math.Inf(1), 0, false},
	} {
		got, ok := dictOffset(tc.v)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s (%v): got %d, %v; want %d, %v", tc.name, tc.v, got, ok, tc.want, tc.ok)
		}
	}
}

// TestAPrivateDictOutsideTheFontIsNotRead is the crash those values reached.
//
// The Private DICT operator carries a size and an offset, and the two were
// added together and compared with the font's length. Written as reals, they
// carried the sum out of the address space: it came back negative, which is
// less than every length, and the slice that followed took the process down.
// The bytes are reachable from an @font-face URL and nothing above them
// recovers.
func TestAPrivateDictOutsideTheFontIsNotRead(t *testing.T) {
	data := make([]byte, 4096)
	for _, tc := range []struct {
		name       string
		size, offs float64
	}{
		{"a size and offset written as large reals", 1e17, 9.2e18},
		{"an offset alone", 16, 9.2e18},
		{"a size alone", 9.2e18, 16},
		{"a sum just past the end", float64(len(data)), 16},
		{"a negative offset", 16, -1},
		{"a negative size", -1, 16},
		{"the two at the width of the type", math.MaxFloat64, math.MaxFloat64},
	} {
		def, nom, subrs := parseCFFPrivate(data, []float64{tc.size, tc.offs})
		if def != 0 || nom != 0 || len(subrs.items) != 0 {
			t.Errorf("%s was read: def %v, nom %v, %d subrs", tc.name, def, nom, len(subrs.items))
		}
	}
}

// TestAPrivateDictInsideTheFontIsStillRead is the control, so that the refusals
// above are not a refusal to read a Private DICT at all.
func TestAPrivateDictInsideTheFontIsStillRead(t *testing.T) {
	// A DICT holding "defaultWidthX 42" and "nominalWidthX 7": each is a
	// 32-bit integer operand (byte 29) followed by its operator.
	var dict []byte
	dict = append(dict, 29)
	dict = binary.BigEndian.AppendUint32(dict, 42)
	dict = append(dict, 20)
	dict = append(dict, 29)
	dict = binary.BigEndian.AppendUint32(dict, 7)
	dict = append(dict, 21)

	data := make([]byte, 64)
	const at = 16
	copy(data[at:], dict)

	def, nom, _ := parseCFFPrivate(data, []float64{float64(len(dict)), at})
	if def != 42 || nom != 7 {
		t.Errorf("the Private DICT read as def %v nom %v, want 42 and 7", def, nom)
	}
}

// TestALocaEntryOutsideGlyfIsNotUsed is the same fault in the sfnt reader.
//
// A long loca entry is thirty-two bits and the offset it becomes is an int, so
// on a build where an int is thirty-two bits an entry above two gigabytes is a
// negative number — less than every end, and so admitted by a check written
// only against the top. Comparing the entry as it is read rather than after the
// conversion is what makes the answer the same on every build.
func TestALocaEntryOutsideGlyfIsNotUsed(t *testing.T) {
	base := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Loca",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 500, HasShape: true},
		},
	})
	tabs := SFNTTables(base)
	glyf, loca := tabs["glyf"], tabs["loca"]
	if len(loca) < 8 {
		t.Fatalf("the fixture's loca is %d bytes", len(loca))
	}

	for _, tc := range []struct {
		name  string
		entry uint32
	}{
		{"an entry past the end of glyf", uint32(len(glyf)) + 1},
		{"an entry at the top of the range", 0xFFFFFFFF},
		{"an entry that is negative in thirty-two bits", 0x80000000},
	} {
		broken := append([]byte(nil), loca...)
		binary.BigEndian.PutUint32(broken[4:], tc.entry) // glyph 1's start
		data := fonttest.SFNT(fonttest.SFNTOptions{
			Name: "Loca",
			Glyphs: []fonttest.Glyph{
				{Rune: 'a', Advance: 500, HasShape: true},
				{Rune: 'b', Advance: 500, HasShape: true},
			},
			Extra: map[string][]byte{"loca": broken},
		})
		fp := ParseSFNT(data, 0)
		if fp == nil {
			continue // refused outright is a fine answer
		}
		if len(fp.GlyphPresent) > 1 && fp.GlyphPresent[1] {
			t.Errorf("%s: glyph 1 is reported present", tc.name)
		}
	}
}

// TestParseType1DoesNotReadTheDictionaryHeaderAsAGlyph is a fixture bent around
// a reader, straightened.
//
// Every real Type 1 font opens its charstring dictionary with "/CharStrings 228
// dict dup begin". A name followed by a number was read as a charstring entry,
// so "CharStrings" was registered as a glyph and the next 228 bytes — every
// glyph after it — were swallowed as its outline. The fixture in fonttest said
// so in as many words and left the count out to avoid it, which means this
// parser had never met a real Type 1 font.
func TestParseType1DoesNotReadTheDictionaryHeaderAsAGlyph(t *testing.T) {
	want := []string{"A", "B", "C", "D", "E"}
	fp := ParseType1(fonttest.Type1Program(want))
	if fp == nil {
		t.Fatal("a Type 1 program was not read at all")
	}
	for _, n := range want {
		if !fp.GlyphNames[n] {
			t.Errorf("the glyph %q was not read; the dictionary header swallowed it", n)
		}
	}
	for name := range fp.GlyphNames {
		if name == "CharStrings" || name == "Private" {
			t.Errorf("%q was registered as a glyph; it is the dictionary, not an outline", name)
		}
	}
	if len(fp.GlyphNames) != len(want) {
		t.Errorf("the program declares %d glyphs, want %d: %v",
			len(fp.GlyphNames), len(want), fp.GlyphNames)
	}
}

// TestParseType1RefusesALengthThatCannotBeALength is the overflow.
//
// A charstring's length is digits in a PostScript file and the file writes as
// many as it likes. Nineteen of them wrapped the accumulator: the length came
// out negative, which is less than everything left in the file and so passed
// the check that the bytes are there, and the slice that followed panicked.
func TestParseType1RefusesALengthThatCannotBeALength(t *testing.T) {
	for _, tc := range []struct{ name, length string }{
		{"nineteen digits", "9999999999999999999"},
		{"twenty-five digits", "1234567890123456789012345"},
		{"the width of the type", "9223372036854775808"},
		{"one past a four-byte offset", "2147483648"},
	} {
		// The program is built by hand, because the fixture writes honest
		// lengths and this is about a file that does not.
		src := "dup /Private 8 dict dup begin\n/lenIV 0 def\n" +
			"2 index /CharStrings 2 dict dup begin\n" +
			"/A 1 RD \x8b ND\n" +
			"/B " + tc.length + " RD \x8b ND\n" +
			"end\nend\nmark currentfile closefile\n"
		fp := ParseType1(type1Wrap(src))
		if fp == nil {
			continue // refused outright is a fine answer
		}
		if fp.GlyphNames["B"] {
			t.Errorf("%s: the glyph after an unreadable length was read anyway", tc.name)
		}
		if !fp.GlyphNames["A"] {
			t.Errorf("%s: the glyph before it was lost too", tc.name)
		}
	}
}

// type1Wrap eexec-encrypts a private section and puts the clear header in front
// of it, which is what a Type 1 program is.
func type1Wrap(priv string) []byte {
	plain := append([]byte("pad!"), priv...)
	var r uint16 = 55665
	const c1, c2 = 52845, 22719
	enc := make([]byte, 0, len(plain))
	for _, p := range plain {
		c := p ^ byte(r>>8)
		r = (uint16(c)+r)*c1 + c2
		enc = append(enc, c)
	}
	header := "%!PS-AdobeFont-1.0\n/FontMatrix [0.001 0 0 0.001 0 0] readonly def\ncurrentfile eexec\n"
	return append([]byte(header), enc...)
}

// TestParseLeadingIntStopsBeforeItWraps pins the helper both of those go
// through, since a bound that is only ever seen through a caller is a bound
// nobody can read.
func TestParseLeadingIntStopsBeforeItWraps(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"0", 0, true},
		{"228", 228, true},
		{"2147483647", math.MaxInt32, true},
		{"2147483648", 0, false},
		{"9999999999999999999", 0, false},
		{"12x", 12, true},
		{"x12", 0, false},
		{"", 0, false},
	} {
		got, ok := parseLeadingInt(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseLeadingInt(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	// And the message the caller would give, so a reader knows what tripped.
	if _, ok := parseLeadingInt(strings.Repeat("9", 30)); ok {
		t.Error("thirty digits were accepted as a length")
	}
}
