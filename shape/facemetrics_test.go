package shape

import (
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Two numbers a font states and this reader did not read.
//
// A Descriptor is what a PDF's /FontDescriptor is written from, and both of
// these end up in one. Neither showed as an error: a wrong italic angle is a
// synthetic oblique leaning by the wrong amount, and a cap height that was never
// stated is a number that happens to be the ascent.

// postWith builds a post table version 3.0 with the given italic angle, as
// 16.16 fixed-point degrees.
func postWith(angleTimes65536 int32) []byte {
	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post[0:], 0x00030000)
	binary.BigEndian.PutUint32(post[4:], uint32(angleTimes65536))
	binary.BigEndian.PutUint16(post[8:], uint16(0x10000-100)) // underlinePosition
	binary.BigEndian.PutUint16(post[10:], uint16(50))         // underlineThickness
	return post
}

// os2With builds an OS/2 table of a given version with a given sCapHeight.
func os2With(version uint16, capHeight int16) []byte {
	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2[0:], version)
	binary.BigEndian.PutUint16(os2[88:], uint16(capHeight))
	return os2
}

func faceWithTables(t *testing.T, extra map[string][]byte) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "Metrics",
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra:  extra,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestTheItalicAngleIsTheOneTheFontStates.
//
// It was macStyle's italic bit and the constant -12: every italic face embedded
// at twelve degrees whatever it was drawn at, and an oblique instance of a
// variable font — where the angle is the axis being varied — embedded at zero,
// because the bit is not set on the default instance it was read from.
func TestTheItalicAngleIsTheOneTheFontStates(t *testing.T) {
	// -9.5 degrees, which is a real value and not the constant.
	f := faceWithTables(t, map[string][]byte{"post": postWith(-9*65536 - 32768)})
	d := f.Descriptor()
	if d.ItalicAngle != -9.5 {
		t.Errorf("the face leans %v degrees and post says -9.5", d.ItalicAngle)
	}
	if !d.Has(MetricItalicAngle) {
		t.Error("the font states an italic angle and the descriptor does not say so")
	}

	// A font that states nothing keeps the guess, and says it is a guess.
	up := faceWithTables(t, map[string][]byte{"post": postWith(0)})
	if a := up.Descriptor().ItalicAngle; a != 0 {
		t.Errorf("an upright face with no angle in post leans %v degrees", a)
	}
	if up.Descriptor().Has(MetricItalicAngle) {
		t.Error("a font that states no angle is reported as stating one")
	}
}

// TestACapHeightIsDeclaredOnlyWhereItIsStated.
//
// sCapHeight arrived in OS/2 version 2, and what said a font stated one was the
// table's *length*. A version 1 table long enough to reach offset 88 has
// something else there; a version 2 table with a zero there has not measured its
// capitals. Both were read as declared, and the zero was then replaced by the
// ascent — so the descriptor said "this font states a cap height" and handed
// back a number the font never wrote.
func TestACapHeightIsDeclaredOnlyWhereItIsStated(t *testing.T) {
	for _, c := range []struct {
		name     string
		version  uint16
		stated   int16
		declared bool
	}{
		{"version 2 with a height", 2, 700, true},
		{"version 4 with a height", 4, 700, true},
		{"version 2 with a zero", 2, 0, false},
		{"version 1, whatever is at that offset", 1, 700, false},
		{"version 0", 0, 700, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := faceWithTables(t, map[string][]byte{"OS/2": os2With(c.version, c.stated)})
			d := f.Descriptor()
			if got := d.Has(MetricCapHeight); got != c.declared {
				t.Errorf("declared = %v, want %v", got, c.declared)
			}
			switch {
			case c.declared && d.CapHeight != int(c.stated):
				t.Errorf("the cap height is %d and the font states %d", d.CapHeight, c.stated)
			case !c.declared && d.CapHeight != d.Ascent:
				t.Errorf("an undeclared cap height came back as %d; the fallback "+
					"is the ascent, %d", d.CapHeight, d.Ascent)
			}
		})
	}
}
