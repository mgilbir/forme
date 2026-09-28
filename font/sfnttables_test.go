package font

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A table that runs past the end of the file is the bytes that are there.

// lastTable is the directory record of the table that starts furthest into
// data, and that table's offset and stated length.
func lastTable(t *testing.T, data []byte) (rec, off, length int) {
	t.Helper()
	rec = -1
	for i := 0; i < Be16(data, 4); i++ {
		r := 12 + 16*i
		if o := int(Be32(data, r+8)); rec < 0 || o > off {
			rec, off, length = r, o, int(Be32(data, r+12))
		}
	}
	if rec < 0 {
		t.Fatal("the font has no tables")
	}
	return rec, off, length
}

// TestATableRunningPastTheFileIsReadAsFarAsItGoes states the last table of a
// font as longer than the file, and as starting at and past its end.
func TestATableRunningPastTheFileIsReadAsFarAsItGoes(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra:  map[string][]byte{"GPOS": fonttest.GPOS([]fonttest.KernPair{{Left: 1, Right: 1, Adjust: -50}})},
	})
	rec, off, length := lastTable(t, data)
	tag := string(data[rec : rec+4])
	whole := SFNTTables(data)[tag]
	if len(whole) != length {
		t.Fatalf("the %s table is %d bytes as written, and states %d", tag, len(whole), length)
	}

	// Stated a hundred thousand bytes longer than it is: the bytes there.
	long := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(long[rec+12:], uint32(length+100000))
	got, ok := SFNTTables(long)[tag]
	if !ok {
		t.Fatalf("%s, stated as running past the end of the file, was dropped", tag)
	}
	if want := long[off:]; !bytes.Equal(got, want) {
		t.Errorf("%s is %d bytes, and %d are there", tag, len(got), len(want))
	}
	if cap(got) != len(got) {
		t.Errorf("%s has room for %d bytes and holds %d", tag, cap(got), len(got))
	}

	// Cut short, as a file truncated in transit is: what is left of it.
	cut := data[:off+length/2]
	if got := SFNTTables(cut)[tag]; !bytes.Equal(got, data[off:off+length/2]) {
		t.Errorf("%s, cut to %d of its %d bytes, reads as %d", tag, length/2, length, len(got))
	}

	// Starting at the end of the file or past it: no bytes, no table.
	for _, at := range []int{len(data), len(data) + 1, 1<<32 - 1} {
		moved := append([]byte(nil), data...)
		binary.BigEndian.PutUint32(moved[rec+8:], uint32(at))
		if got, ok := SFNTTables(moved)[tag]; ok {
			t.Errorf("%s, starting at %d of a %d-byte file, reads as %d bytes", tag, at, len(data), len(got))
		}
	}
	// And the largest length the field holds, which must not wrap.
	huge := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(huge[rec+12:], 1<<32-1)
	if got := SFNTTables(huge)[tag]; len(got) != len(data)-off {
		t.Errorf("%s, stated as 4 GiB long, reads as %d bytes of the %d there", tag, len(got), len(data)-off)
	}
}

// TestEveryTableLiesInsideTheFileAtEveryLength cuts a real font at every
// length its tables begin and end at, and one either side of each, and
// requires every table read to be the file's own bytes at its stated offset,
// no longer than what is there, with no room past its end.
func TestEveryTableLiesInsideTheFileAtEveryLength(t *testing.T) {
	data, err := os.ReadFile("../testdata/harfbuzz/fonts/NotoSansBalinese.ttf")
	if err != nil {
		t.Fatal(err)
	}
	var cuts []int
	for i := 0; i < Be16(data, 4); i++ {
		r := 12 + 16*i
		for _, at := range []int{int(Be32(data, r+8)), int(Be32(data, r+8)) + int(Be32(data, r+12))} {
			cuts = append(cuts, at-1, at, at+1)
		}
	}
	checked := 0
	for _, n := range cuts {
		if n < 0 || n > len(data) {
			continue
		}
		cut := data[:n]
		tables := SFNTTables(cut)
		if tables == nil {
			continue
		}
		for i := 0; i < Be16(cut, 4); i++ {
			r := 12 + 16*i
			tab, ok := tables[string(cut[r:r+4])]
			off, length := int(Be32(cut, r+8)), int(Be32(cut, r+12))
			switch {
			case off >= n && length > 0:
				if ok {
					t.Errorf("cut at %d: %q starts at %d and was read", n, cut[r:r+4], off)
				}
			case !ok:
				t.Errorf("cut at %d: %q starts at %d and was dropped", n, cut[r:r+4], off)
			default:
				if want := cut[off:min(off+length, n)]; !bytes.Equal(tab, want) || cap(tab) != len(tab) {
					t.Errorf("cut at %d: %q is %d bytes (room for %d), and %d are there",
						n, cut[r:r+4], len(tab), cap(tab), len(want))
				}
				checked++
			}
		}
	}
	if checked < 100 {
		t.Fatalf("%d tables were checked; the cuts are not reaching the directory", checked)
	}
}
