package layout

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// The context a run is shaped in when its neighbour is set in another face.
//
// Font fallback puts the two sides of a boundary in two fonts, and the two
// things the context decides do not both cross it. Which of its four shapes a
// letter takes is decided by the characters beside it, and a character is the
// same character whichever font sets it. A kerning pair is one font's statement
// about two of its own glyphs, and a font change is a change in formatting.
//
// The suite says the first half three times over — shaping-join-002 and
// shaping-tatweel-002 and -003 pull a zero width joiner or a tatweel out of
// another font with unicode-range and ask for the Arabic letters either side to
// join anyway.

// TestAFaceChangeStillGivesTheRunItsContext. The two families are the same font
// file loaded twice, so the faces differ and their coverage cannot: what is
// being changed is the face and nothing else.
func TestAFaceChangeStillGivesTheRunItsContext(t *testing.T) {
	dir := os.Getenv(notoEnv)
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that joins")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansArabic-Regular.ttf"))
	if err != nil {
		t.Skip("no Arabic face: ", err)
	}
	res := &fileResolver{files: map[string][]byte{"a.ttf": data, "b.ttf": data}}
	glyphs := func(markup string) []int {
		t.Helper()
		ops := paintWith(t, res, `<div id="d" dir="rtl">`+markup+`</div>`,
			`@font-face { font-family: A; src: url(a.ttf) }
			 @font-face { font-family: B; src: url(b.ttf) }
			 #d { font-family: A; font-size: 40px }
			 .other { font-family: B }`)
		var out []int
		for _, op := range ops {
			v, ok := op.(DrawText)
			if !ok {
				continue
			}
			gs, _ := ShapedGlyphs(v)
			for _, g := range gs {
				out = append(out, g.GID)
			}
		}
		return out
	}
	// Sorted, because the three runs of the split word are painted in the
	// order the pen meets them and the whole word is one run reversed: the same
	// glyphs, the other way round. What is being asserted is which shapes were
	// chosen, and a word set in isolated forms is three of one glyph however it
	// is ordered.
	whole := glyphs("ععع")
	split := glyphs(`ع<span class="other">ع</span>ع`)
	sort.Ints(whole)
	sort.Ints(split)
	if len(whole) != 3 {
		t.Fatalf("the whole word drew %d glyphs, want 3: %v", len(whole), whole)
	}
	if len(split) != len(whole) {
		t.Fatalf("the split word drew %v and the whole one %v", split, whole)
	}
	for i := range whole {
		if split[i] != whole[i] {
			t.Errorf("split across a font change the word draws %v and whole it "+
				"draws %v; a letter's joined shape is decided by the characters "+
				"beside it, and a character is the same character whichever font "+
				"sets it", split, whole)
			break
		}
	}
}

// TestThePainterHonoursWhichContextItHas is the other half, asked of the one
// function a backend calls: the same run, the same neighbour, and the only
// difference is whether the pair across the boundary is this font's to apply.
func TestThePainterHonoursWhichContextItHas(t *testing.T) {
	dir := os.Getenv(notoEnv)
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face with kern pairs")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSans-Regular.ttf"))
	if err != nil {
		t.Skipf("no such font in this checkout: %v", err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !face.HasKerning() {
		t.Skip("the face carries no kerning this package could read")
	}
	advance := func(v DrawText) float64 {
		gs, _ := ShapedGlyphs(v)
		total := 0.0
		for _, g := range gs {
			total += g.XAdvance
		}
		return total
	}
	var pair string
	for _, p := range []string{"AV", "AW", "Ta", "Vo", "AT"} {
		alone := advance(DrawText{Face: face, Text: p[:1], ContextKerns: true})
		with := advance(DrawText{Face: face, Text: p[:1], PostContext: p[1:],
			ContextKerns: true})
		if alone != with {
			pair = p
			break
		}
	}
	if pair == "" {
		t.Skip("none of the candidate pairs is kerned by this face")
	}
	alone := advance(DrawText{Face: face, Text: pair[:1], ContextKerns: true})
	across := advance(DrawText{Face: face, Text: pair[:1], PostContext: pair[1:]})
	if across != alone {
		t.Errorf("%q drawn across a face change measures %g, want the unkerned "+
			"%g — the pair belongs to whichever font holds both glyphs",
			pair, across, alone)
	}
}

// TestAFaceChangeIsNotKernedAcross is the same document with a Latin face that
// kerns: the two families are one font file loaded twice, so the pair either
// side of the boundary is a pair this font states — and still not one it may
// apply, because the two glyphs are in two fonts.
func TestAFaceChangeIsNotKernedAcross(t *testing.T) {
	dir := os.Getenv(notoEnv)
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face with kern pairs")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSans-Regular.ttf"))
	if err != nil {
		t.Skipf("no such font in this checkout: %v", err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A pair the face really kerns, found from the face itself so that the
	// choice cannot depend on the rule being tested.
	advance := func(s string) float64 {
		gs, _ := face.ShapeGlyphs(s)
		total := 0.0
		for _, g := range gs {
			total += g.XAdvance
		}
		return total
	}
	var pair string
	for _, p := range []string{"AV", "AW", "Ta", "Vo", "AT"} {
		if advance(p) != advance(p[:1])+advance(p[1:]) {
			pair = p
			break
		}
	}
	if pair == "" {
		t.Skip("none of the candidate pairs is kerned by this face")
	}

	res := &fileResolver{files: map[string][]byte{"a.ttf": data, "b.ttf": data}}
	secondRunAt := func(markup string) float64 {
		t.Helper()
		ops := paintWith(t, res, `<div id="d">`+markup+`</div>`,
			`@font-face { font-family: A; src: url(a.ttf) }
			 @font-face { font-family: B; src: url(b.ttf) }
			 body { margin: 0 }
			 #d { font-family: A; font-size: 40px }
			 .other { font-family: B }`)
		n := 0
		for _, op := range ops {
			v, ok := op.(DrawText)
			if !ok {
				continue
			}
			n++
			if n == 2 {
				return v.At.X.Px()
			}
		}
		t.Fatalf("%q drew %d runs, want at least 2", markup, n)
		return 0
	}
	same := secondRunAt(`<span>` + pair[:1] + `</span><span>` + pair[1:] + `</span>`)
	across := secondRunAt(`<span>` + pair[:1] + `</span><span class="other">` + pair[1:] + `</span>`)
	if same >= across {
		t.Errorf("%q begins at %g across a font change and at %g within one; "+
			"the kern was applied across a boundary the font's pairs do not "+
			"cross", pair, across, same)
	}
}
