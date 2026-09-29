package shape

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What a face says it is called and how it is styled, which is what a CSS
// font-family, font-weight, font-stretch and font-style are matched against.

func utf16be(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u>>8), byte(u))
	}
	return out
}

func win(id int, s string) nameRecord         { return nameRecord{3, 1, 0x409, id, utf16be(s)} }
func mac(id int, s string) nameRecord         { return nameRecord{1, 0, 0, id, []byte(s)} }
func winIn(lang, id int, s string) nameRecord { return nameRecord{3, 1, lang, id, utf16be(s)} }

// nameTableOf lays out a format 0 name table with the builder in nametable_test.go.
func nameTableOf(recs ...nameRecord) []byte { return buildNameTable(0, recs, nil) }

// os2Style builds an OS/2 table of the given length with the style fields set;
// a length too short for a field loses it, as a real short table does.
func os2Style(length int, weight, width, fsSelection uint16) []byte {
	os2 := make([]byte, max(length, 78))
	binary.BigEndian.PutUint16(os2[0:], 4)
	binary.BigEndian.PutUint16(os2[4:], weight)
	binary.BigEndian.PutUint16(os2[6:], width)
	binary.BigEndian.PutUint16(os2[62:], fsSelection)
	return os2[:length]
}

// styledFont is a synthetic font carrying the given name and OS/2 tables and
// head's macStyle. A nil table is left as SFNT builds it (the name it builds
// carries only the PostScript name, and it builds no OS/2).
func styledFont(t *testing.T, name, os2 []byte, macStyle uint16) *Face {
	t.Helper()
	glyphs := []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}
	base := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs})
	head := append([]byte(nil), font.SFNTTables(base)["head"]...)
	binary.BigEndian.PutUint16(head[44:], macStyle)
	extra := map[string][]byte{"head": head}
	if name != nil {
		extra["name"] = name
	}
	if os2 != nil {
		extra["OS/2"] = os2
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs, Extra: extra}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return f
}

func TestFamilyAndSubfamilyFollowTheNameRecords(t *testing.T) {
	for _, c := range []struct {
		name          string
		table         []byte
		family, style string
	}{
		{"typographic beats legacy",
			nameTableOf(win(1, "Taviraj ExtraBold"), win(2, "Italic"), win(16, "Taviraj"), win(17, "ExtraBold Italic")),
			"Taviraj", "ExtraBold Italic"},
		{"legacy when there is no typographic",
			nameTableOf(win(1, "Agdasima"), win(2, "Bold")),
			"Agdasima", "Bold"},
		{"each falls back on its own",
			nameTableOf(win(1, "Legacy"), win(2, "Bold"), win(16, "Typo")),
			"Typo", "Bold"},
		{"an empty typographic record is not an answer",
			nameTableOf(win(1, "Legacy"), win(2, "Regular"), win(16, ""), win(17, "")),
			"Legacy", "Regular"},
		{"an unreadable typographic record is not an answer",
			nameTableOf(win(1, "Legacy"), win(16, "\x00\x00")),
			"Legacy", ""},
		{"windows english before mac, whatever the order",
			nameTableOf(mac(1, "Mac Name"), win(1, "Win Name")),
			"Win Name", ""},
		{"mac roman English before windows in another language",
			nameTableOf(winIn(0x411, 1, "Japanese"), mac(1, "Mac Name")),
			"Mac Name", ""},
		{"windows in another language when it is all there is",
			nameTableOf(winIn(0x411, 1, "Japanese")),
			"Japanese", ""},
		{"mac roman alone",
			nameTableOf(mac(1, "Old Font"), mac(2, "Bold")),
			"Old Font", "Bold"},
		{"a mac record that is not ascii is passed over, not misread",
			nameTableOf(nameRecord{1, 0, 0, 1, []byte{'C', 0x8e, 'r', 'a'}}),
			"", ""},
		{"the unicode platform",
			nameTableOf(nameRecord{0, 3, 0, 1, utf16be("Unicode Name")}),
			"Unicode Name", ""},
		{"a character outside the BMP survives",
			nameTableOf(win(1, "Emoji \U0001F600 Sans")),
			"Emoji \U0001F600 Sans", ""},
		{"non-ascii text is kept, and padding is not",
			nameTableOf(win(1, " Noto Sans 日本\x00")),
			"Noto Sans 日本", ""},
		{"platform 3 encoding 10",
			nameTableOf(nameRecord{3, 10, 0x409, 1, utf16be("Ten")}),
			"Ten", ""},
		{"a record of another platform is not read",
			nameTableOf(nameRecord{2, 0, 0, 1, []byte("ISO")}),
			"", ""},
		{"the first of equal records wins",
			nameTableOf(win(1, "First"), win(1, "Second")),
			"First", ""},
		{"no records", nameTableOf(), "", ""},
		{"a table too short for its header", []byte{0, 0, 0}, "", ""},
		{"no table", []byte{}, "", ""},
		{"a count past the end of the table",
			func() []byte {
				b := nameTableOf(win(1, "Cut"))
				binary.BigEndian.PutUint16(b[2:], 60000)
				return b
			}(), "Cut", ""},
		{"a record whose string is outside the table",
			func() []byte {
				b := nameTableOf(win(1, "Cut"))
				binary.BigEndian.PutUint16(b[6+8:], 5000)
				return b
			}(), "", ""},
		{"an odd number of bytes loses the half unit",
			nameTableOf(nameRecord{3, 1, 0x409, 1, append(utf16be("Odd"), 0x41)}),
			"Odd", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := styledFont(t, c.table, nil, 0)
			if got := f.Family(); got != c.family {
				t.Errorf("Family = %q, want %q", got, c.family)
			}
			if got := f.Subfamily(); got != c.style {
				t.Errorf("Subfamily = %q, want %q", got, c.style)
			}
		})
	}
}

// The PostScript name is read as it always was: a reader of ID 16 must not have
// changed what /BaseFont is.
func TestReadingFamilyLeavesThePostScriptNameAlone(t *testing.T) {
	f := styledFont(t, nameTableOf(win(6, "Some-Bold"), win(1, "Some"), win(2, "Bold")), nil, 0)
	if f.Name() != "Some-Bold" || f.Family() != "Some" || f.Subfamily() != "Bold" {
		t.Errorf("Name %q Family %q Subfamily %q", f.Name(), f.Family(), f.Subfamily())
	}
}

func TestStyleBitsAreFsSelectionThenMacStyle(t *testing.T) {
	const italic, oblique, bold, regular = 1 << 0, 1 << 9, 1 << 5, 1 << 6
	for _, c := range []struct {
		name            string
		os2             []byte // nil: no OS/2 table
		macStyle        uint16
		italic, oblique bool
	}{
		{"italic bit", os2Style(96, 400, 5, italic), 0, true, false},
		{"oblique bit", os2Style(96, 400, 5, oblique), 0, false, true},
		{"both bits", os2Style(96, 400, 5, italic|oblique), 0, true, true},
		{"neither", os2Style(96, 700, 5, bold), 0, false, false},
		{"regular, with the other bits set", os2Style(96, 400, 5, regular|bold|0x80), 0, false, false},
		// OS/2 is the newer statement and wins when it exists, even by saying no.
		{"os2 says no, macStyle says italic", os2Style(96, 400, 5, regular), 2, false, false},
		{"os2 says italic, macStyle says not", os2Style(96, 400, 5, italic), 0, true, false},
		{"macStyle bold alone is not italic", os2Style(96, 400, 5, regular), 1, false, false},
		// No OS/2: macStyle is all there is.
		{"no os2, macStyle italic", nil, 2, true, false},
		{"no os2, macStyle bold", nil, 1, false, false},
		{"no os2, macStyle none", nil, 0, false, false},
		// An OS/2 too short to hold fsSelection is no OS/2 for this purpose.
		{"os2 truncated before fsSelection", os2Style(62, 400, 5, 0), 2, true, false},
		{"os2 truncated in fsSelection", os2Style(63, 400, 5, 0), 2, true, false},
		{"os2 exactly through fsSelection", os2Style(64, 400, 5, oblique), 2, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := styledFont(t, nil, c.os2, c.macStyle)
			d := f.Descriptor()
			if d.Italic != c.italic || d.Oblique != c.oblique {
				t.Errorf("Italic %v Oblique %v, want %v %v", d.Italic, d.Oblique, c.italic, c.oblique)
			}
			if !d.Has(MetricStyle) {
				t.Error("Has(MetricStyle) is false for a font that states a style")
			}
		})
	}
}

func TestWeightAndWidthClassAreOS2(t *testing.T) {
	f := styledFont(t, nil, os2Style(96, 900, 3, 0), 0)
	d := f.Descriptor()
	if d.Weight != 900 || d.WidthClass != 3 || !d.Has(MetricWeight) || !d.Has(MetricWidth) {
		t.Errorf("Weight %d WidthClass %d (declared %v %v), want 900 and 3", d.Weight, d.WidthClass,
			d.Has(MetricWeight), d.Has(MetricWidth))
	}
	// Without an OS/2 table there is nothing to state, and the zero is not a
	// width class: Declared says so.
	for _, os2 := range [][]byte{nil, os2Style(40, 900, 3, 0), os2Style(77, 900, 3, 0)} {
		d := styledFont(t, nil, os2, 0).Descriptor()
		if d.WidthClass != 0 || d.Has(MetricWidth) || d.Weight != 0 || d.Has(MetricWeight) {
			t.Errorf("a short OS/2 (%d bytes) stated Weight %d WidthClass %d", len(os2), d.Weight, d.WidthClass)
		}
	}
}

// A face with nothing to read answers with zero values and does not panic.
func TestAFaceWithNoMetadataAnswersWithZeroValues(t *testing.T) {
	f := styledFont(t, []byte{}, nil, 0)
	if f.Family() != "" || f.Subfamily() != "" {
		t.Errorf("Family %q Subfamily %q from a font with no name records", f.Family(), f.Subfamily())
	}
	for _, name := range StandardNames() {
		s, err := Standard(name)
		if err != nil {
			t.Fatal(err)
		}
		d := s.Descriptor()
		if s.Family() != "" || s.Subfamily() != "" || d.WidthClass != 0 || d.Italic || d.Oblique ||
			d.Has(MetricWidth) || d.Has(MetricStyle) {
			t.Errorf("%s: a standard face has no program to read and stated %q %q %+v", name, s.Family(), s.Subfamily(), d)
		}
	}
}

// Real fonts, against what fontTools reads out of the same files.
func TestFamilyWeightAndStyleOfRealFonts(t *testing.T) {
	type want struct {
		path, family, sub string
		weight, width     int
		italic, oblique   bool
	}
	for _, c := range []want{
		// The bundled face: no typographic names, the legacy pair.
		{"../fonts/notosans/NotoSans-Variable.ttf", "Noto Sans", "Regular", 400, 5, false, false},
		{"../testdata/googlefonts/ofl/notosans/NotoSans-Italic[wdth,wght].ttf", "Noto Sans", "Italic", 400, 5, true, false},
		{"../testdata/googlefonts/ofl/abeezee/ABeeZee-Italic.ttf", "ABeeZee", "Italic", 400, 5, true, false},
		// Names 16 and 17 differ from 1 and 2.
		{"../testdata/googlefonts/ofl/taviraj/Taviraj-ExtraBoldItalic.ttf", "Taviraj", "ExtraBold Italic", 800, 5, true, false},
		{"../testdata/googlefonts/ofl/barlowsemicondensed/BarlowSemiCondensed-Black.ttf", "Barlow Semi Condensed", "Black", 900, 4, false, false},
		{"../testdata/googlefonts/ofl/georama/Georama-Italic[wdth,wght].ttf", "Georama", "ExtraCondensed Thin Italic", 100, 2, true, false},
		// A legacy family that carries the weight, and a width, with no 16/17.
		{"../testdata/googlefonts/ofl/agdasima/Agdasima-Bold.ttf", "Agdasima", "Bold", 700, 3, false, false},
		{"../testdata/googlefonts/ofl/biorhymeexpanded/BioRhymeExpanded-Bold.ttf", "BioRhyme Expanded", "Bold", 700, 7, false, false},
		// OS/2 version 0, and macStyle bold that must not read as italic.
		{"../testdata/googlefonts/ofl/sansation/Sansation-Bold.ttf", "Sansation", "Bold", 700, 5, false, false},
	} {
		data, err := os.ReadFile(filepath.FromSlash(c.path))
		if err != nil {
			if filepath.Base(c.path) == "NotoSans-Variable.ttf" {
				t.Fatal(err)
			}
			t.Logf("skipping %s: %v", c.path, err)
			continue
		}
		f, err := Load(data)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		d := f.Descriptor()
		if f.Family() != c.family || f.Subfamily() != c.sub || d.Weight != c.weight ||
			d.WidthClass != c.width || d.Italic != c.italic || d.Oblique != c.oblique {
			t.Errorf("%s: got %q %q weight %d width %d italic %v oblique %v; want %q %q %d %d %v %v", filepath.Base(c.path),
				f.Family(), f.Subfamily(), d.Weight, d.WidthClass, d.Italic, d.Oblique,
				c.family, c.sub, c.weight, c.width, c.italic, c.oblique)
		}
	}
}

// LoadInstance rewrites the weight and width it cut at; the style names are the
// instance's, and the family is the font's.
func TestAnInstanceReportsWhereItWasCut(t *testing.T) {
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := LoadInstance(data, map[string]float64{"wght": 700, "wdth": 75})
	if err != nil {
		t.Fatal(err)
	}
	d := inst.Descriptor()
	if d.Weight != 700 || d.WidthClass != 3 {
		t.Errorf("Weight %d WidthClass %d, want 700 and 3", d.Weight, d.WidthClass)
	}
	if inst.Family() == "" {
		t.Error("an instance lost its family")
	}
}
