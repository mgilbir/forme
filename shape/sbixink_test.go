package shape

import (
	"testing"

	"github.com/mgilbir/forme/font"
)

// TestSbixSurvivesEveryByteChanged reads SbixInk's sbix table with each of its
// bytes set to nothing and to everything in turn, and asks for the ink of
// every glyph and of the glyphs either side of the count. The table is offsets
// into itself throughout — strikes, glyph records, duplicates naming glyphs —
// and each byte changed is an offset or a count a reader could trust.
func TestSbixSurvivesEveryByteChanged(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "SbixInk.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	table := font.SFNTTables(f.Program())["sbix"]
	if len(table) == 0 || f.sbix == nil {
		t.Fatal("SbixInk.ttf loaded without its sbix table read")
	}
	n := f.NumGlyphs()
	read := 0
	for i := range table {
		for _, b := range []byte{0x00, 0xFF} {
			if table[i] == b {
				continue
			}
			mutated := append([]byte(nil), table...)
			mutated[i] = b
			noPanic(t, "sbix ink", func() {
				s := newSbixInk(map[string][]byte{"sbix": mutated}, n, f.unitsPerEm)
				if s == nil {
					return
				}
				read++
				for gid := -1; gid <= n+1; gid++ {
					s.extents(gid)
				}
			})
		}
	}
	// Most changes leave a table the sanitizer takes: a test in which every
	// one was refused would have asked for no glyph's ink at all.
	if read < len(table) {
		t.Errorf("only %d of %d changed tables were read", read, 2*len(table))
	}
}
