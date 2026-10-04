package shape

import (
	"reflect"
	"testing"
)

// A run's working memory is the face's, and the next run works in the same
// arrays: shaping one run after another on a face does not allocate them
// again (issue 907). What a run leaves in them is not read by the next one,
// so a run shaped after a longer one comes out as it does on a fresh face.
func TestARunWorksInTheMemoryTheRunBeforeItDid(t *testing.T) {
	base, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	// A mark that continues a grapheme, a joiner taken out, and the
	// positioning of the whole.
	long := "Quarterly révenue, office‍ 001234 and a longer label"
	short := "näi‍ve"

	f := base.Clone()
	f.ShapeGlyphs(long)
	s := f.scratch
	if s == nil || cap(s.runes) == 0 || cap(s.offsets) == 0 || cap(s.continues) == 0 || cap(s.drop) == 0 ||
		cap(s.gpos.chain) == 0 || cap(s.gpos.kind) == 0 {
		t.Fatalf("a run was shaped without the face's scratch: %+v", s)
	}
	if s.run.a != nil {
		t.Error("the face holds on to the glyphs of the run it returned")
	}
	first := func(s *runScratch) [6]any {
		return [6]any{&s.runes[:1][0], &s.offsets[:1][0], &s.continues[:1][0], &s.drop[:1][0],
			&s.gpos.chain[:1][0], &s.gpos.kind[:1][0]}
	}
	arrays := first(s)

	got, _ := f.ShapeGlyphs(short)
	if f.scratch != s || first(s) != arrays {
		t.Error("the second run did not work in the arrays the first one left")
	}
	want, _ := base.Clone().ShapeGlyphs(short)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after a longer run, %q shaped as\n%v\nwant, on a fresh face,\n%v", short, got, want)
	}

	if f.Clone().scratch != nil {
		t.Error("a clone shares the scratch of the face it was made from")
	}
}

// The neighbours a pair across a boundary is found by are shaped inside the
// run, on the same face and in the same scratch, once the run is positioned:
// the run comes out as it does where its neighbours are shaped elsewhere.
func TestARunShapedBesideItsNeighboursIsTheRunAlone(t *testing.T) {
	base, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	f := base.Clone()
	f.ShapeGlyphs("a much longer run first, so the scratch has room to spare")
	got, _ := f.ShapeGlyphsMerged("AVA To", "Wa", "Ty", "", "", true, Features{})
	want, _ := base.Clone().ShapeGlyphsMerged("AVA To", "Wa", "Ty", "", "", true, Features{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("beside its neighbours, after another run:\n%v\nwant, on a fresh face,\n%v", got, want)
	}
}

// What shaping an ordinary label allocates, once the face has shaped a run
// as long, is the glyphs it hands back.
func TestShapingALabelAllocatesTheGlyphsItReturns(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	f = f.Clone()
	for _, s := range []string{"Revenue", "Quarterly revenue, office 001234", "näïve café", "AVA ffi"} {
		f.ShapeGlyphs(s)
		if n := testing.AllocsPerRun(50, func() { f.ShapeGlyphs(s) }); n > 1 {
			t.Errorf("shaping %q allocates %v times, want once, for the glyphs", s, n)
		}
	}
}
