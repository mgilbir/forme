package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestEachFileLandsInItsOwnBucket: a font, a file shape.Load refuses, a font
// collection and a file that is not there are four answers, and the counts are
// what the sweep is for. A collection and an unreadable file were counted as
// refused, which sized a gap in the reader out of a format it does not claim
// and a file it was never given.
func TestEachFileLandsInItsOwnBucket(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	font := write("font.ttf", fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	}))
	refused := write("refused.ttf", append([]byte{0, 1, 0, 0}, make([]byte, 60)...))
	ttc := make([]byte, 16)
	copy(ttc, "ttcf")
	binary.BigEndian.PutUint32(ttc[4:], 0x00010000)
	binary.BigEndian.PutUint32(ttc[8:], 2)
	collection := write("pair.ttc", ttc)
	missing := filepath.Join(dir, "missing.ttf")

	var rows []row
	for path, want := range map[string]string{
		font:       resultOK,
		refused:    resultRefused,
		collection: resultCollection,
		missing:    resultUnreadable,
	} {
		r := read(path)
		if r.result != want {
			t.Errorf("%s: %s (%s), want %s", filepath.Base(path), r.result, r.detail, want)
		}
		rows = append(rows, r)
	}
	counts, reasons := tally(rows)
	for _, result := range []string{resultOK, resultRefused, resultCollection, resultUnreadable} {
		if counts[result] != 1 {
			t.Errorf("%d rows counted %s, want 1", counts[result], result)
		}
	}
	if len(reasons) != 1 {
		t.Errorf("%d reasons, want the one refusal's: %v", len(reasons), reasons)
	}
}
