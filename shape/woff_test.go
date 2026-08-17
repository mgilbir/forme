package shape

import (
	"os"
	"path/filepath"
	"testing"
)

// A WOFF read through Load, which is the join this change is for: the decoder
// lives in the font package and nothing else in the engine knows the format
// exists, so what has to be pinned here is that Load unwraps one at all.
//
// Gated on the suite's checkout because a WOFF is a real font and this module
// vendors none — see the note beside NOTO_DIR in the Makefile.
func TestLoadReadsAWOFF(t *testing.T) {
	dir := os.Getenv("WPT_TESTS")
	if dir == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) to read a real WOFF")
	}
	path := filepath.Join(dir, "fonts", "Revalia.woff")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no such font in this checkout: %v", err)
	}
	face, err := Load(data)
	if err != nil {
		t.Fatalf("Load on a WOFF: %v", err)
	}
	// It has to be a usable face and not merely a parse that returned something:
	// a face that shapes nothing would satisfy "no error" and be no use.
	glyphs, missing := face.ShapeGlyphs("Revalia")
	if len(glyphs) == 0 {
		t.Fatalf("the face shaped no glyphs")
	}
	if missing != 0 {
		t.Errorf("%d of the face's own name is missing from it", missing)
	}
	for _, g := range glyphs {
		if g.XAdvance != 0 {
			return
		}
	}
	t.Errorf("every glyph came back with a zero advance, so the metrics did not survive")
}
