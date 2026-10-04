package shape

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAMorxRunGrowsNoLongerThanHarfBuzzLetsIt holds a morx insertion that
// never stops to HarfBuzz's max_len: a run may grow to 256 glyphs for each it
// started with, and to 65,536 however short it was. TestMORXThirtyfour inserts
// a glyph at every step until its allowance is spent, and a sentence of 43
// characters came back as 166,885 glyphs.
func TestAMorxRunGrowsNoLongerThanHarfBuzzLetsIt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "aat", "fonts", "TestMORXThirtyfour.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	grew := false
	for _, s := range []string{"a", "The quick brown fox jumps over the lazy dog", strings.Repeat("abc ", 100)} {
		glyphs, _ := f.ShapeGlyphs(s)
		n := len([]rune(s))
		if bound := max(256*n, 65536); len(glyphs) > bound {
			t.Errorf("%d characters came back as %d glyphs, past %d", n, len(glyphs), bound)
		}
		grew = grew || len(glyphs) > 1000*n
	}
	if !grew {
		t.Fatal("no run grew, so this test measures nothing")
	}
}
