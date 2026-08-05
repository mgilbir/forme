package shape

import (
	"testing"

	"github.com/mgilbir/forme/internal/fonttest"
)

// Shaping checked against a font whose kerning and ligatures the test states,
// so an assertion can name the exact adjustment rather than whatever a real
// face happens to contain.

// shapingFace builds a face with glyphs A V f i, a kern pair and an fi
// ligature. Glyph indices follow the order given: A=1, V=2, f=3, i=4, fi=5.
func shapingFace(t *testing.T) *Face {
	t.Helper()
	glyphs := []fonttest.Glyph{
		{Rune: 'A', Advance: 700, HasShape: true},
		{Rune: 'V', Advance: 700, HasShape: true},
		{Rune: 'f', Advance: 300, HasShape: true},
		{Rune: 'i', Advance: 250, HasShape: true},
		{Rune: 'ﬁ', Advance: 520, HasShape: true}, // U+FB01
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "Shape",
		Glyphs: glyphs,
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOS([]fonttest.KernPair{{Left: 1, Right: 2, Adjust: -80}}),
			"GSUB": fonttest.GSUB([]fonttest.Ligature{{Components: []int{3, 4}, Glyph: 5}}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestKerningIsRead pins that a GPOS pair adjustment reaches the reader at all.
// Everything else here rests on it, and a font whose kerning went unread would
// otherwise just produce unkerned text, which no other assertion notices.
func TestKerningIsRead(t *testing.T) {
	f := shapingFace(t)
	if !f.HasKerning() {
		t.Fatal("the font's GPOS kern feature was not read")
	}
	// Kerning is kept per lookup, so the pair is looked for in all of them —
	// which is also what says a pair stated in one lookup is not lost by
	// another naming the same glyphs.
	pair := func(a, b int) (pairAdjust, bool) {
		for _, kl := range f.layout.kern {
			if adj, ok := kl.pairs[[2]int{a, b}]; ok {
				return adj, true
			}
		}
		return pairAdjust{}, false
	}
	if got, ok := pair(1, 2); !ok || got.firstAdvance != -80 {
		t.Errorf("kern(A,V) = %v (found %v), want -80", got.firstAdvance, ok)
	}
	if _, ok := pair(2, 1); ok {
		t.Error("kerning was applied in the wrong order: (V,A) is not a declared pair")
	}
}

// TestShapeMeasuresWhatItDraws pins that measurement and shaping agree. If they
// could disagree, a layout engine would reserve one width and the renderer
// would paint another — the defect that shows up as a line overflowing its box
// in a viewer but not in the engine that produced it.
func TestShapeMeasuresWhatItDraws(t *testing.T) {
	f := shapingFace(t)
	// A=700, V=700, kern -80 → 1320/1000 em.
	if got, want := f.MeasureShaped("AV", 10), 13.2; got != want {
		t.Errorf("MeasureShaped(\"AV\") = %v, want %v", got, want)
	}
	// Unshaped, the same string is wider: no kerning is applied.
	if got, want := f.Measure("AV", 10), 14.0; got != want {
		t.Errorf("Measure(\"AV\") = %v, want %v", got, want)
	}
	// The ligature is narrower than its parts: f=300 + i=250 = 550, fi = 520.
	if got, want := f.MeasureShaped("fi", 10), 5.2; got != want {
		t.Errorf("MeasureShaped(\"fi\") = %v, want %v", got, want)
	}
}

// markedFace has A V and a combining mark, a kern pair for (A,V), and a GDEF
// classifying the mark. Glyph indices: A=1, V=2, mark=3.
func markedFace(t *testing.T, flag int) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Marked",
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 700, HasShape: true},
			{Rune: 'V', Advance: 700, HasShape: true},
			{Rune: '́', Advance: 0, HasShape: true}, // combining acute
		},
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOSWithFlag([]fonttest.KernPair{{Left: 1, Right: 2, Adjust: -80}}, flag),
			"GDEF": fonttest.GDEF(map[int]int{1: 1, 2: 1, 3: 3}), // base, base, mark
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}
