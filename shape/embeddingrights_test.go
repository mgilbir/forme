package shape

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// A face says what its licence allows a document to do with it, and hands over
// the program to embed whole when the licence forbids subsetting it.

// os2FSType builds an OS/2 table of the given version and length with the given
// fsType, which every version carries at offset 8.
func os2FSType(version uint16, length int, fsType uint16) []byte {
	os2 := make([]byte, max(length, 10))
	binary.BigEndian.PutUint16(os2[0:], version)
	binary.BigEndian.PutUint16(os2[8:], fsType)
	return os2[:length] // a table too short for the field loses it
}

func TestEmbeddingPermissionsAreTheFontsFSType(t *testing.T) {
	for _, c := range []struct {
		name   string
		os2    []byte // nil: the font carries no OS/2 table
		want   FSType
		stated bool
	}{
		// Each bit the issue names, alone, at the version a current font has.
		{"restricted", os2FSType(4, 96, 0x0002), FSTypeRestricted, true},
		{"no subsetting", os2FSType(4, 96, 0x0100), FSTypeNoSubsetting, true},
		{"bitmap only", os2FSType(4, 96, 0x0200), FSTypeBitmapOnly, true},
		{"preview and print, no subsetting", os2FSType(4, 96, 0x0104),
			FSTypePreviewPrint | FSTypeNoSubsetting, true},
		{"installable", os2FSType(4, 96, 0), 0, true},
		// Version 0 is the shortest table and has fsType where every later
		// version has it. Its usage bits may be combined, which the value
		// carries as it is.
		{"version 0", os2FSType(0, 78, 0x000A), FSTypeRestricted | FSTypeEditable, true},
		// Apple's original version 0 stopped at 68 bytes. It still states
		// fsType, and a reader that asked for a whole version 0 table before
		// reading it would have lost a restriction the font did state.
		{"version 0, 68 bytes", os2FSType(0, 68, 0x0002), FSTypeRestricted, true},
		// Nothing stated: the table is absent, or too short to reach the field.
		{"no OS/2", nil, 0, false},
		{"an OS/2 of 9 bytes", os2FSType(3, 9, 0x0002), 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var extra map[string][]byte
			if c.os2 != nil {
				extra = map[string][]byte{"OS/2": c.os2}
			}
			f := faceWithTables(t, extra)
			got, stated := f.EmbeddingPermissions()
			if got != c.want || stated != c.stated {
				t.Errorf("EmbeddingPermissions is %#04x, stated %v; want %#04x, %v",
					uint16(got), stated, uint16(c.want), c.stated)
			}
		})
	}
}

// TestTheFSTypeBitsAreTheSpecifications pins the constants to the OpenType
// OS/2 table's bit numbers, written out here rather than read from the code.
func TestTheFSTypeBitsAreTheSpecifications(t *testing.T) {
	for _, c := range []struct {
		name string
		bit  FSType
		want uint16
	}{
		{"Restricted License embedding", FSTypeRestricted, 1 << 1},
		{"Preview & Print embedding", FSTypePreviewPrint, 1 << 2},
		{"Editable embedding", FSTypeEditable, 1 << 3},
		{"No subsetting", FSTypeNoSubsetting, 1 << 8},
		{"Bitmap embedding only", FSTypeBitmapOnly, 1 << 9},
	} {
		if uint16(c.bit) != c.want {
			t.Errorf("%s is %#04x, and the specification's bit is %#04x", c.name, uint16(c.bit), c.want)
		}
	}
}

func TestAStandardFaceStatesNoPermissionsAndHasNoProgram(t *testing.T) {
	f, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if p, stated := f.EmbeddingPermissions(); stated || p != 0 {
		t.Errorf("a standard face states fsType %#04x (%v); it has no program to say it in", uint16(p), stated)
	}
	if p := f.Program(); p != nil {
		t.Errorf("a standard face returned a %d-byte program", len(p))
	}
}

// TestProgramIsTheWholeProgramTheFaceWasLoadedFrom: the bytes a document
// embeds when the licence forbids a subset.
func TestProgramIsTheWholeProgramTheFaceWasLoadedFrom(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"OS/2": os2FSType(4, 96, 0x0100)},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	// Set some text first: what a face has encoded decides its subset, and must
	// not decide this.
	f.Encode("a")
	if got := f.Program(); !bytes.Equal(got, data) {
		t.Fatalf("Program is %d bytes and not the %d the face was loaded from", len(got), len(data))
	}
	sub, err := f.Subset()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sub, f.Program()) {
		t.Fatal("the subset is the whole program, so this cannot tell the two apart")
	}
	// A clone is the same face for another document, and has the same program.
	if !bytes.Equal(f.Clone().Program(), data) {
		t.Error("a clone's program is not the face's")
	}
	// Handed out, not copied: a document asks for it once per face embedded,
	// and for the CJK faces this matters for, the program is megabytes.
	if n := testing.AllocsPerRun(10, func() { _ = f.Program() }); n != 0 {
		t.Errorf("Program allocates %v times a call; it is the face's own bytes", n)
	}
}

// TestProgramOfAWOFFIsTheSFNTItWraps: a WOFF is a container a document format
// cannot embed, and what Load reads from it is the font program inside.
func TestProgramOfAWOFFIsTheSFNTItWraps(t *testing.T) {
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra:  map[string][]byte{"OS/2": os2FSType(4, 96, 0x0100)},
	})
	var tables []fonttest.WOFFTable
	for tag, body := range font.SFNTTables(sfnt) {
		tables = append(tables, fonttest.WOFFTable{Tag: tag, Data: body})
	}
	woff := fonttest.WOFF(fonttest.WOFFOptions{Tables: tables})
	f, err := Load(woff)
	if err != nil {
		t.Fatalf("Load on the WOFF: %v", err)
	}
	want, err := font.DecodeWOFF(woff)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Program(); !bytes.Equal(got, want) {
		t.Errorf("Program is %d bytes; the sfnt the WOFF wraps is %d", len(got), len(want))
	}
	if font.IsWOFF(f.Program()) {
		t.Error("Program handed back the WOFF container")
	}
	// And the rights are read from it, as from any other program.
	if p, stated := f.EmbeddingPermissions(); !stated || p != FSTypeNoSubsetting {
		t.Errorf("a WOFF's fsType read as %#04x (%v)", uint16(p), stated)
	}
}
