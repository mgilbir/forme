package shape

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// nameRecords reads a name table's records, or fails the test.
func nameRecords(t *testing.T, name []byte) []nameRecord {
	t.Helper()
	var got []nameRecord
	if rebuildName(name, func(r []nameRecord) []nameRecord { got = append(got, r...); return r }) == nil {
		t.Fatal("the name table cannot be read")
	}
	return got
}

func recordKeys(recs []nameRecord) map[string]string {
	m := map[string]string{}
	for _, r := range recs {
		m[fmt.Sprintf("%d/%d/%#x/%d", r.platform, r.encoding, r.language, r.id)] = string(r.value)
	}
	return m
}

// TestSubsetNameKeepsTheNoticesAndTheNames (#983): a subset's name table keeps
// the notices and the names, each record's string as the font states it, and
// drops the descriptive records, the IDs only tables a subset does not carry
// name, and the Macintosh duplicates of records the other platforms state. A
// record the Macintosh platform alone states stays.
func TestSubsetNameKeepsTheNoticesAndTheNames(t *testing.T) {
	in := buildNameTable(0, []nameRecord{
		win(0, "Copyright 2026 Someone"),
		mac(0, "Copyright 2026 Someone"),
		win(1, "Probe"), win(2, "Regular"), win(4, "Probe Regular"), win(6, "Probe-Regular"),
		win(7, "Probe is a trademark"),
		win(8, "Foundry"), win(9, "Designer"), win(10, "A long description of the font"),
		win(11, "https://vendor.example"), win(12, "https://designer.example"),
		win(13, "Licensed under the SIL Open Font License, Version 1.1"),
		win(14, "https://openfontlicense.org"),
		mac(16, "Probe Family"), // stated by the Macintosh platform alone
		win(19, "The quick brown fox"),
		win(25, "ProbeVar"),
		win(256, "Weight"), win(257, "Bold"),
		{0, 3, 0, 17, utf16be("Regular")},
	}, nil)
	out := subsetName(in)
	if out == nil {
		t.Fatal("the name table was not rebuilt")
	}
	got := recordKeys(nameRecords(t, out))
	want := map[int]bool{0: true, 1: true, 2: true, 4: true, 6: true, 7: true, 13: true, 14: true, 16: true, 17: true}
	ids := map[int]bool{}
	for _, r := range nameRecords(t, out) {
		ids[r.id] = true
		if r.platform == 1 && r.id != 16 {
			t.Errorf("the Macintosh record of ID %d stayed, and the Windows one states it", r.id)
		}
	}
	for id := range want {
		if !ids[id] {
			t.Errorf("ID %d was dropped", id)
		}
	}
	for id := range ids {
		if !want[id] {
			t.Errorf("ID %d was kept", id)
		}
	}
	for k, v := range recordKeys(nameRecords(t, in)) {
		if g, ok := got[k]; ok && g != v {
			t.Errorf("record %s reads %q, was %q", k, g, v)
		}
	}
	if len(out) >= len(in) {
		t.Errorf("the table is %d bytes, was %d", len(out), len(in))
	}
	if nameByID(out, 6) != "Probe-Regular" || readName(out, 1) != "Probe" {
		t.Errorf("the subset's table names the font %q, %q", nameByID(out, 6), readName(out, 1))
	}
}

// A format 1 table keeps its language tags, and a kept record naming one
// still names it.
func TestSubsetNameKeepsFormat1LanguageTags(t *testing.T) {
	in := buildNameTable(1, []nameRecord{
		win(9, "Designer"),
		winIn(0x8000, 1, "Sonde"),
		win(1, "Probe"),
	}, []string{"fr-CA"})
	out := subsetName(in)
	if out == nil {
		t.Fatal("the name table was not rebuilt")
	}
	recs := nameRecords(t, out)
	if len(recs) != 2 || font.Be16(out, 0) != 1 {
		t.Fatalf("format %d with %d records, want format 1 with 2", font.Be16(out, 0), len(recs))
	}
	at := 6 + 12*len(recs)
	if font.Be16(out, at) != 1 {
		t.Fatalf("%d language tags, want 1", font.Be16(out, at))
	}
	length, off := font.Be16(out, at+2), font.Be16(out, 4)+font.Be16(out, at+4)
	if tag := out[off : off+length]; !bytes.Equal(tag, utf16be("fr-CA")) {
		t.Errorf("the language tag reads %q", tag)
	}
	for _, r := range recs {
		if r.language == 0x8000 && string(r.value) != string(utf16be("Sonde")) {
			t.Errorf("the record in the tagged language reads %q", r.value)
		}
	}
}

// A table that cannot be read is carried as it is: what a subset could have
// done without is no reason to refuse one.
func TestSubsetNameLeavesAnUnreadableTable(t *testing.T) {
	in := buildNameTable(0, []nameRecord{win(1, "Probe"), win(9, "Designer")}, nil)
	in = in[:len(in)-4] // the last string runs past the table
	if out := subsetName(in); out != nil {
		t.Errorf("an unreadable table was rebuilt as %d bytes", len(out))
	}
	for kind, build := range keepsNumbering {
		f, err := Load(build(fonttest.SFNTOptions{Name: "Probe", Glyphs: alphabet(), Extra: map[string][]byte{"name": in}}))
		if err != nil {
			t.Fatal(err)
		}
		f.Encode("abc")
		data, err := f.Subset()
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got := font.SFNTTables(data)["name"]; !bytes.Equal(got, in) {
			t.Errorf("%s: the subset's name is %d bytes, the font's %d", kind, len(got), len(in))
		}
	}
}

// TestSubsetNameOfRealFonts: the subsets of real faces — glyf, a CFF that keeps
// its numbering, a variable face's default instance and a CID-keyed CFF2 —
// carry a smaller name table that names the font as the font does, with the
// same notices, and load.
func TestSubsetNameOfRealFonts(t *testing.T) {
	for _, tc := range []struct {
		name string
		data func(*testing.T) []byte
	}{
		{"NotoSans-Variable", func(t *testing.T) []byte {
			data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
			if err != nil {
				t.Fatal(err)
			}
			return data
		}},
		{"NotoSans-Regular", func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSans-Regular.ttf") }},
		{"SourceSans3-Regular", func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") }},
		{"NotoSansJP-VF", func(t *testing.T) []byte { return fonttest.CFFFile(t, "NotoSansJP-VF.otf") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.data(t)
			f, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			checkSubsetName(t, f, font.SFNTTables(data)["name"])
		})
	}
}

func checkSubsetName(t *testing.T, f *Face, fontName []byte) {
	t.Helper()
	f.ShapeGlyphs("Measurement 0123456789")
	prog, _, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	name := font.SFNTTables(prog)["name"]
	if len(name) >= len(fontName) {
		t.Errorf("the subset's name is %d bytes, the font's %d", len(name), len(fontName))
	}
	for _, id := range []int{0, 1, 2, 4, 6, 7, 13, 14, 16, 17} {
		if got, want := nameByID(name, id), nameByID(fontName, id); got != want {
			t.Errorf("ID %d reads %q in the subset, %q in the font", id, got, want)
		}
	}
	sub, err := Load(prog)
	if err != nil {
		t.Fatalf("the subset does not load: %v", err)
	}
	if sub.Family() != f.Family() {
		t.Errorf("the subset's family is %q, the font's %q", sub.Family(), f.Family())
	}
}
