package font

import (
	"encoding/binary"
	"math"
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
		p := newCFFPrivates(data, testBudget()).read([]float64{tc.size, tc.offs})
		if p.def != 0 || p.nom != 0 || len(p.subrs.items) != 0 {
			t.Errorf("%s was read: def %v, nom %v, %d subrs", tc.name, p.def, p.nom, len(p.subrs.items))
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

	p := newCFFPrivates(data, testBudget()).read([]float64{float64(len(dict)), at})
	if p.def != 42 || p.nom != 7 {
		t.Errorf("the Private DICT read as def %v nom %v, want 42 and 7", p.def, p.nom)
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
