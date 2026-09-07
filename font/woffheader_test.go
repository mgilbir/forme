package font

import (
	"encoding/binary"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// One question, asked of both formats.
//
// WOFF 2 checks that its header's figures describe the file it is in: every
// block where the header says, and nothing left over. WOFF 1 asked none of it —
// not the length the header states, not the reserved field, not whether the
// metadata and private blocks are inside the file at all — so a WOFF with
// something appended, or with a metadata block pointing past its end, was read
// as though it were ordinary.
//
// The two are checked as far as each one's layout allows, which is not equally
// far: WOFF 2 lays its blocks one after another on four-byte boundaries, so
// every offset is derivable; WOFF 1's tables are wherever it says they are, and
// what §3 of that format requires is what is required here.

// aWOFF is a small, well-formed WOFF 1 font.
func aWOFF(t *testing.T) []byte {
	t.Helper()
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	})
	var tables []fonttest.WOFFTable
	for tag, body := range SFNTTables(sfnt) {
		tables = append(tables, fonttest.WOFFTable{Tag: tag, Data: body})
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Tag < tables[j].Tag })
	return fonttest.WOFF(fonttest.WOFFOptions{Tables: tables})
}

// TestAWOFFHeaderHasToDescribeItsOwnFile.
func TestAWOFFHeaderHasToDescribeItsOwnFile(t *testing.T) {
	good := aWOFF(t)
	if _, err := DecodeWOFF(good); err != nil {
		t.Fatalf("the fixture itself was refused: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mangle func(b []byte) []byte
		says   string
	}{
		{"a length that is not the file's", func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[8:], uint32(len(b))+1)
			return b
		}, "bytes and it is"},
		{"something appended after it", func(b []byte) []byte {
			return append(b, 0, 0, 0, 0)
		}, "bytes and it is"},
		{"a reserved field that is not zero", func(b []byte) []byte {
			binary.BigEndian.PutUint16(b[14:], 1)
			return b
		}, "reserved field"},
		{"a metadata block past the end", func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[24:], uint32(len(b)))
			binary.BigEndian.PutUint32(b[28:], 64)
			return b
		}, "metadata block is not inside"},
		{"a metadata block inside the header", func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[24:], 4)
			binary.BigEndian.PutUint32(b[28:], 8)
			return b
		}, "metadata block is not inside"},
		{"a private block past the end", func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[36:], uint32(len(b))-2)
			binary.BigEndian.PutUint32(b[40:], 64)
			return b
		}, "private block is not inside"},
	} {
		b := tc.mangle(append([]byte(nil), good...))
		_, err := DecodeWOFF(b)
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: refused with %q, which does not say %q",
				tc.name, err, tc.says)
		}
	}
}

// TestAWOFFWithNoExtraBlocksIsStillAWOFF is the other side: a font that carries
// neither block says so with zeros, and zeros are not an offset outside the
// file.
func TestAWOFFWithNoExtraBlocksIsStillAWOFF(t *testing.T) {
	b := aWOFF(t)
	for _, at := range []int{24, 28, 36, 40} {
		if got := binary.BigEndian.Uint32(b[at:]); got != 0 {
			t.Fatalf("the fixture writes %d at byte %d, so it does carry a block", got, at)
		}
	}
	if _, err := DecodeWOFF(b); err != nil {
		t.Errorf("a WOFF carrying neither block was refused: %v", err)
	}
}
