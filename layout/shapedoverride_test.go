package layout

import (
	"os"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// TestNoGlyphOfARightToLeftRunComesFromItsOverride holds ShapedGlyphs to the
// promise a backend reads clusters by: each glyph's cluster is a character of
// the shaped text that the glyph draws.
//
// ShapedText puts an override in front of a right-to-left run, and the shaper
// takes it out as it takes out any character nothing is drawn for, merging its
// cluster into the glyph after it, as HarfBuzz merges one. Left there, the
// run's first letter said it came from the override: a backend mapping glyphs
// back to characters read the letter as a control, and the comparison the
// suite's reftests are judged by drew it as a gap in every right-to-left run.
func TestNoGlyphOfARightToLeftRunComesFromItsOverride(t *testing.T) {
	data, err := os.ReadFile("../testdata/harfbuzz/fonts/NotoSansArabic.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	v := DrawText{Text: "ععع", RTL: true, Face: f, ContextKerns: true}
	text := ShapedText(v)
	prefix := len(text) - len(v.Text)
	if prefix == 0 {
		t.Fatal("ShapedText put no override in front of the run, so this test measures nothing")
	}
	glyphs, _ := ShapedGlyphs(v)
	if len(glyphs) != 3 {
		t.Fatalf("%d glyphs for three letters", len(glyphs))
	}
	seen := map[int]bool{}
	for _, g := range glyphs {
		if g.Cluster < prefix {
			t.Errorf("glyph %d is in cluster %d, inside the override", g.GID, g.Cluster)
		}
		seen[g.Cluster] = true
	}
	for at := prefix; at < len(text); at += len("ع") {
		if !seen[at] {
			t.Errorf("no glyph draws the letter at %d", at)
		}
	}
}
