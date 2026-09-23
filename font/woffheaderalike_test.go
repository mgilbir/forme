package font

import (
	"encoding/binary"
	"sort"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// aWOFF2 is a small, well-formed WOFF 2 font with the same tables as aWOFF.
func aWOFF2(t *testing.T) []byte {
	t.Helper()
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	})
	var list []fonttest.WOFF2Table
	for tag, body := range SFNTTables(sfnt) {
		list = append(list, fonttest.WOFF2Table{Tag: tag, Data: body})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Tag < list[j].Tag })
	return fonttest.WOFF2(fonttest.WOFF2Options{Tables: list})
}

// TestBothWOFFsAnswerTheHeaderQuestionsAlike.
//
// The two formats have the same header fields for the same things — a reserved
// field, and an offset and lengths for the metadata and the private block —
// and answered questions about them differently. WOFF 1 refused a non-zero
// reserved field and WOFF 2 never read its own; WOFF 1 refused a block with
// no offset and a length, by the accident of its bounds check, and WOFF 2
// skipped the block whenever the offset was zero, so "metaOffset 0,
// metaLength 1000" was a font. Each question is asked of both here, and each
// answer has to be a refusal.
func TestBothWOFFsAnswerTheHeaderQuestionsAlike(t *testing.T) {
	type field struct{ at, size int }
	for _, q := range []struct {
		question    string
		woff, woff2 field
	}{
		{"a reserved field that is not zero", field{14, 2}, field{14, 2}},
		{"a metadata block with no offset and a length", field{28, 4}, field{32, 4}},
		{"a metadata block with no offset and an uncompressed length", field{32, 4}, field{36, 4}},
		{"a private block with no offset and a length", field{40, 4}, field{44, 4}},
	} {
		for _, f := range []struct {
			format string
			build  func(*testing.T) []byte
			at     field
		}{
			{"WOFF", aWOFF, q.woff},
			{"WOFF 2", aWOFF2, q.woff2},
		} {
			b := f.build(t)
			if _, err := DecodeWOFF(b); err != nil {
				t.Fatalf("%s: the fixture itself was refused: %v", f.format, err)
			}
			if f.at.size == 2 {
				binary.BigEndian.PutUint16(b[f.at.at:], 1)
			} else {
				binary.BigEndian.PutUint32(b[f.at.at:], 1000)
			}
			if _, err := DecodeWOFF(b); err == nil {
				t.Errorf("%s with %s was accepted", f.format, q.question)
			}
		}
	}
}
