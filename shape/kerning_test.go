package shape

import (
	"os"
	"path/filepath"
	"testing"
)

// TestKerningIsApplied pins a fact style/inert.go depends on.
//
// "font-kerning: auto" asks for the face's own kerning, and "none" asks for it
// to be turned off. Which of the two is inert here depends entirely on whether
// this engine kerns — and it does, so "auto" asks for what it produces and
// "none" asks for something it does not do.
//
// It is checked here rather than asserted there because it is a fact about
// shaping. If kerning were ever turned off or lost, the entry in that table
// would go on saying "font-kerning: auto is inert" while the engine had started
// producing "none", and would suppress a finding about a real difference.
func TestKerningIsApplied(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face with kern pairs")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSans-Regular.ttf"))
	if err != nil {
		t.Skipf("no such font in this checkout: %v", err)
	}
	face, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	advance := func(s string) float64 {
		gs, _ := face.ShapeGlyphs(s)
		total := 0.0
		for _, g := range gs {
			total += g.XAdvance
		}
		return total
	}
	// A pair the face kerns, measured together and apart. Kerning is the whole
	// of the difference: the same two glyphs, in the same order, shaped as one
	// run and as two.
	kerned := 0
	for _, pair := range []string{"AV", "To", "LT", "Yo"} {
		if advance(pair) != advance(pair[:1])+advance(pair[1:]) {
			kerned++
		}
	}
	if kerned == 0 {
		t.Errorf("none of the four kern pairs was kerned; style/inert.go records " +
			"that this engine applies a face's kerning, and if it no longer does " +
			"then \"font-kerning: auto\" is the value that differs and \"none\" is " +
			"the one that does not")
	}
}
