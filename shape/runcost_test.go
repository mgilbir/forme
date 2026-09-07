package shape

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// What a substitution costs, and why it is asked about at two lengths rather
// than at one.
//
// A lookup that changes how many glyphs there are used to rebuild the run: the
// part before, the replacement, the part after. That is the whole run copied per
// substitution, so a run of ligatures copied itself once per ligature — 16,000
// characters of "fi" took 796 ms against 11 ms for the same length with nothing
// to ligate, and 64,000 took 14.9 s. The same shape was in the syllabic scripts
// twice over: each syllable was shaped where it lay and its change in length
// shifted every glyph after it.
//
// Neither is visible in an answer, so neither can be pinned by one. What can be
// is how the cost *grows*: four times the text is four times the work if the
// work is linear and sixteen times if it is quadratic, and those two are far
// enough apart that a bound between them is not a threshold anybody has to tune.

// ligatingRun is text every pair of which the bundled face draws as one glyph.
func ligatingRun(n int) string { return strings.Repeat("fi", n/2) }

// TestALigatingRunIsNotCopiedOncePerLigature measures the allocation, which is
// the part of the old cost that leaves a trace: rebuilding the run took a new
// array of the run's length for every substitution.
func TestALigatingRunIsNotCopiedOncePerLigature(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	// The layout tables are read on the first run shaped with them, and that
	// reading is not what this is about.
	f.ShapeGlyphs(ligatingRun(64))

	bytes := func(n int) uint64 {
		t.Helper()
		s := ligatingRun(n)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		glyphs, _ := f.ShapeGlyphs(s)
		runtime.ReadMemStats(&after)
		// The fixture has to ligate, or this measures a run of plain text and
		// would pass with every substitution path removed.
		if len(glyphs) != n/2 {
			t.Fatalf("%d characters came to %d glyphs, want %d: the face is not "+
				"ligating this and the test is watching nothing", n, len(glyphs), n/2)
		}
		return after.TotalAlloc - before.TotalAlloc
	}

	const small, large = 4000, 16000
	a, b := bytes(small), bytes(large)
	// Four times the text. Linear allocates four times as much and quadratic
	// sixteen; eight is out of reach of the one and comfortable for the other.
	if float64(b) > 8*float64(a) {
		t.Errorf("%d characters allocated %d bytes and %d allocated %d, which is "+
			"%.1f times for four times the text; a run rebuilt once per ligature "+
			"is sixteen", small, a, large, b, float64(b)/float64(a))
	}
}

// TestASyllableIsNotShapedWhereItLies is the other half, which allocates
// nothing and so has to be timed.
//
// Devanagari is shaped a syllable at a time, and a syllable that ligates used to
// move every glyph after it in the run. The measurement is a ratio for the
// reason above, and a ratio is also what survives the race detector: it slows
// both lengths, and what is being asked about is the shape of the curve.
func TestASyllableIsNotShapedWhereItLies(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	const syllable = "क्ष"
	f.ShapeGlyphs(strings.Repeat(syllable, 8))

	took := func(n int) time.Duration {
		t.Helper()
		s := strings.Repeat(syllable, n)
		start := time.Now()
		glyphs, _ := f.ShapeGlyphs(s)
		d := time.Since(start)
		// Three characters to a syllable and one glyph out of it: the fixture
		// has to be ligating, or nothing is moved and nothing is measured.
		if len(glyphs) != n {
			t.Fatalf("%d syllables came to %d glyphs, want %d", n, len(glyphs), n)
		}
		return d
	}

	// Sixteen times the text. Measured on this machine, with the plant in and
	// out: shaped where they lie the sixteen-fold run takes 61 times as long,
	// and shaped on their own it takes 15.9 — which is linear to within the
	// noise. Twice linear is the bound, which is a factor of two away from each
	// of them.
	//
	// The smaller size is two thousand rather than a handful so that the
	// measurement it is a ratio *of* is a tenth of a second and not a
	// microsecond: a ratio against noise is noise.
	const small, large = 2000, 32000
	a, b := took(small), took(large)
	if ratio := float64(b) / float64(a); ratio > 2*float64(large/small) {
		t.Errorf("%d syllables took %v and %d took %v, which is %.1f times for "+
			"%d times the text; a syllable shaped where it lies is four times "+
			"that", small, a, large, b, ratio, large/small)
	}
}
