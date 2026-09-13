package shape

import (
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/font"
)

// Rebuilding the name table when a variable font is instanced.
//
// An instance gets its own PostScript name, so every record carrying name ID 6
// is rewritten. The table is rebuilt rather than patched, because its strings
// are packed into one block that records index into and may overlap: there is
// nowhere to write a longer string, and no way to know which other record shares
// the old one.
//
// The table has two formats and only one had ever been rebuilt. Format 1 states
// language tags of its own after the records, and a record names its language by
// pointing into that list — so the list has to travel with the table, with its
// offsets rebuilt into the new string block. That loop was at 0% across every
// unit test and all 6253 reftest documents, because a format 1 name table is
// rare and no corpus font has one.
//
// What it would do is not fail. The rebuilt table would declare language tags
// and point them at whatever was at offset zero of the new block, so a record
// naming a custom language would report the font's family name as its language
// tag — a font that loads, names itself correctly, and lies about one field.

// nameRecord is one record of the table being built.
type nameRecord struct {
	platform, encoding, language, id int
	value                            []byte
}

// buildNameTable assembles a name table in either format. Strings are packed in
// the order the records are given, and a record whose value is already in the
// block points at the existing copy — which is the overlap the rebuild exists to
// cope with.
func buildNameTable(format int, records []nameRecord, langTags []string) []byte {
	head := 6 + 12*len(records)
	if format == 1 {
		head += 2 + 4*len(langTags)
	}
	out := make([]byte, head)
	binary.BigEndian.PutUint16(out[0:], uint16(format))
	binary.BigEndian.PutUint16(out[2:], uint16(len(records)))
	binary.BigEndian.PutUint16(out[4:], uint16(head))

	var strs []byte
	place := func(v []byte) int {
		// Share an identical string rather than writing it twice.
		for at := 0; at+len(v) <= len(strs); at++ {
			if string(strs[at:at+len(v)]) == string(v) {
				return at
			}
		}
		at := len(strs)
		strs = append(strs, v...)
		return at
	}
	for i, r := range records {
		rec := 6 + 12*i
		binary.BigEndian.PutUint16(out[rec:], uint16(r.platform))
		binary.BigEndian.PutUint16(out[rec+2:], uint16(r.encoding))
		binary.BigEndian.PutUint16(out[rec+4:], uint16(r.language))
		binary.BigEndian.PutUint16(out[rec+6:], uint16(r.id))
		binary.BigEndian.PutUint16(out[rec+8:], uint16(len(r.value)))
		binary.BigEndian.PutUint16(out[rec+10:], uint16(place(r.value)))
	}
	if format == 1 {
		binary.BigEndian.PutUint16(out[6+12*len(records):], uint16(len(langTags)))
		for i, tag := range langTags {
			at := 6 + 12*len(records) + 2 + 4*i
			v := encodeNameString(tag, 3)
			binary.BigEndian.PutUint16(out[at:], uint16(len(v)))
			binary.BigEndian.PutUint16(out[at+2:], uint16(place(v)))
		}
	}
	return append(out, strs...)
}

// langTagsOf reads the language-tag list back out of a table, which nameByID
// does not do because nothing else needs it.
func langTagsOf(t *testing.T, name []byte) []string {
	t.Helper()
	if font.Be16(name, 0) != 1 {
		t.Fatalf("the table is format %d, which states no language tags",
			font.Be16(name, 0))
	}
	count := font.Be16(name, 2)
	storage := font.Be16(name, 4)
	at := 6 + 12*count
	n := font.Be16(name, at)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rec := at + 2 + 4*i
		length := font.Be16(name, rec)
		off := storage + font.Be16(name, rec+2)
		if off+length > len(name) {
			t.Fatalf("language tag %d runs past the table: %d+%d > %d",
				i, off, length, len(name))
		}
		raw := name[off : off+length]
		var s []byte
		for j := 0; j+1 < len(raw); j += 2 {
			s = append(s, raw[j+1])
		}
		out = append(out, string(s))
	}
	return out
}

// sampleRecords is a table with the shapes that matter: the name being replaced,
// a name that must not move, and two records sharing one string.
func sampleRecords() []nameRecord {
	shared := encodeNameString("Shared", 3)
	return []nameRecord{
		{platform: 3, encoding: 1, language: 0x409, id: 1, value: encodeNameString("Family", 3)},
		{platform: 3, encoding: 1, language: 0x409, id: 6, value: encodeNameString("Old-PS-Name", 3)},
		{platform: 3, encoding: 1, language: 0x409, id: 4, value: shared},
		{platform: 3, encoding: 1, language: 0x409, id: 5, value: shared},
	}
}

func TestTheNameTableIsRebuiltInEitherFormat(t *testing.T) {
	for _, c := range []struct {
		name     string
		format   int
		langTags []string
	}{
		{"format 0", 0, nil},
		{"format 1 with no tags", 1, nil},
		{"format 1 with one tag", 1, []string{"en-GB"}},
		{"format 1 with three tags", 1, []string{"en-GB", "gsw-u-sd-chzh", "x-private"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := buildNameTable(c.format, sampleRecords(), c.langTags)
			out := replacePostScriptName(in, "New-PS-Name")
			if out == nil {
				t.Fatal("the table was refused")
			}

			if got := font.Be16(out, 0); got != c.format {
				t.Errorf("the rebuilt table is format %d, want %d; the format "+
					"decides whether a reader looks for a language-tag list",
					got, c.format)
			}
			if got := nameByID(out, 6); got != "New-PS-Name" {
				t.Errorf("name ID 6 reads %q, want the instance's own name", got)
			}
			// Everything else survives, including the two records that shared
			// one string in the input.
			for id, want := range map[int]string{1: "Family", 4: "Shared", 5: "Shared"} {
				if got := nameByID(out, id); got != want {
					t.Errorf("name ID %d reads %q, want %q", id, got, want)
				}
			}

			if c.format != 1 {
				return
			}
			got := langTagsOf(t, out)
			if len(got) != len(c.langTags) {
				t.Fatalf("the rebuilt table states %d language tags, want %d: %q",
					len(got), len(c.langTags), got)
			}
			for i, want := range c.langTags {
				if got[i] != want {
					t.Errorf("language tag %d reads %q, want %q; a record names "+
						"its language by pointing into this list, so a tag that "+
						"moved names a different language", i, got[i], want)
				}
			}
		})
	}
}

// A longer PostScript name than the one it replaces is the reason the table is
// rebuilt rather than patched: there is nowhere to write it.
func TestALongerPostScriptNameFits(t *testing.T) {
	long := "A-Very-Much-Longer-PostScript-Name-Than-The-One-It-Replaces"
	in := buildNameTable(1, sampleRecords(), []string{"en-GB", "x-private"})
	out := replacePostScriptName(in, long)
	if out == nil {
		t.Fatal("the table was refused")
	}
	if got := nameByID(out, 6); got != long {
		t.Errorf("name ID 6 reads %q, want %q", got, long)
	}
	if got := nameByID(out, 1); got != "Family" {
		t.Errorf("name ID 1 reads %q; the longer name must not have overwritten "+
			"a neighbour, which is what patching in place would do", got)
	}
	if got := langTagsOf(t, out); len(got) != 2 || got[0] != "en-GB" || got[1] != "x-private" {
		t.Errorf("the language tags read %q, want [en-GB x-private]", got)
	}
}

// A format 1 table whose language-tag list is not all there is refused, because
// the count is the font's and the font is untrusted.
func TestATruncatedLanguageTagListIsRefused(t *testing.T) {
	in := buildNameTable(1, sampleRecords(), []string{"en-GB", "x-private"})

	// A count larger than the records behind it.
	tooMany := append([]byte(nil), in...)
	binary.BigEndian.PutUint16(tooMany[6+12*4:], 0x2000)
	if out := replacePostScriptName(tooMany, "New-PS-Name"); out != nil {
		t.Errorf("a table claiming 8192 language tags was rebuilt into %d bytes; "+
			"the list is not there to copy", len(out))
	}

	// The list's own header cut off.
	cut := in[:6+12*4+1]
	if out := replacePostScriptName(cut, "New-PS-Name"); out != nil {
		t.Errorf("a format 1 table with no language-tag count was rebuilt into "+
			"%d bytes", len(out))
	}
}

// A tag inside a list the right length, pointing outside the table.
//
// This is the other bound, and the two catch different things. The count's bound
// asks whether the list is there at all; this one asks whether a tag in a list
// that *is* there names bytes that are. A font can state a perfectly sized list
// whose entries are nonsense, and copying one would read past the table.
func TestALanguageTagPointingOutsideTheTableIsRefused(t *testing.T) {
	for _, c := range []struct {
		name        string
		length, off int
	}{
		{"an offset past the end", 10, 0xF000},
		{"a length past the end", 0xF000, 0},
		{"both just past the end", 4, 0xFFFF - 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := buildNameTable(1, sampleRecords(), []string{"en-GB"})
			at := 6 + 12*4 + 2 // the first language-tag record
			binary.BigEndian.PutUint16(in[at:], uint16(c.length))
			binary.BigEndian.PutUint16(in[at+2:], uint16(c.off))

			// The premise: the list itself is the size the count says, so the
			// count's own bound passes and this is the only thing left.
			if 6+12*4+2+4*1 > len(in) {
				t.Fatal("the fixture overclaims its count, which the other " +
					"bound would catch first")
			}
			if out := replacePostScriptName(in, "New-PS-Name"); out != nil {
				t.Errorf("a language tag naming bytes outside the table was "+
					"copied into a %d-byte rebuild", len(out))
			}
		})
	}
}

// A count larger than the list, where every stray read happens to be harmless.
//
// The two cases above are caught by the bound inside the copying loop, which
// refuses a tag whose bytes lie outside the table — they are caught by luck,
// because the bytes a bogus record lands on decode to an offset that is out of
// range. Removing the count's own bound left both of them passing.
//
// So this one is built to make every stray read benign: one record with an empty
// string, and zero padding after the table. A length of zero at an offset of
// zero is inside any table, so the loop copies as many empty tags as the count
// asks for and the rebuilt table declares language tags that were never there.
// Nothing further along would question them — they are well-formed and empty.
func TestALanguageTagCountLargerThanTheListIsRefused(t *testing.T) {
	const count, padding, claimed = 1, 40, 20
	head := 6 + 12*count + 2 // no real tags
	in := make([]byte, head)
	binary.BigEndian.PutUint16(in[0:], 1)            // format 1
	binary.BigEndian.PutUint16(in[2:], count)        //
	binary.BigEndian.PutUint16(in[4:], uint16(head)) // storage begins at the end
	binary.BigEndian.PutUint16(in[6+4:], 0x409)      // language
	binary.BigEndian.PutUint16(in[6+6:], 6)          // name ID 6, with an empty value
	binary.BigEndian.PutUint16(in[6+12*count:], claimed)
	in = append(in, make([]byte, padding)...)

	// The premise: the bound inside the loop cannot see this one. Every stray
	// read lands on a zero, which is a zero-length tag at offset zero.
	if 6+12*count+2+4*claimed <= len(in) {
		t.Fatalf("the fixture does not overclaim: %d tags fit in %d bytes",
			claimed, len(in))
	}

	if out := replacePostScriptName(in, "New-PS-Name"); out != nil {
		t.Errorf("a table claiming %d language tags with none behind it was "+
			"rebuilt into %d bytes; the count is the font's and there is "+
			"nothing there to copy", claimed, len(out))
	}
}
