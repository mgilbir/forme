package paragraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The breaker measures an upright run by its glyphs' vertical advances where the
// face states them, and by CSS Writing Modes §4.4's em box where it does not —
// and a stretch of a run cut across lines by the same numbers as the whole
// (#771). VerticalComposites.ttf's "A" advances 0.9 em down the page;
// VerticalFallbacks.ttf states nothing, and shaping's own answer for it (1.2 em,
// HarfBuzz's) is not CSS's.
func TestAnUprightRunIsMeasuredByItsVerticalMetrics(t *testing.T) {
	for _, tc := range []struct {
		face string
		em   float64 // along the line, per letter, in ems
	}{
		{"VerticalComposites.ttf", 0.9},
		{"VerticalFallbacks.ttf", 1},
	} {
		data, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", tc.face))
		if err != nil {
			t.Fatal(err)
		}
		face, err := shape.Load(data)
		if err != nil {
			t.Fatal(err)
		}
		size, _ := style.FromPx(20)
		br := NewBreaker(nil)
		want := func(n int) float64 { return float64(n) * tc.em * 20 }
		if got := br.MeasureSpacedInContext(face, "AAAA", size, TextSpacing{},
			Shaping{Upright: true}).Px(); got != want(4) {
			t.Errorf("%s: four letters measure %gpx, want %g", tc.face, got, want(4))
		}
		// Long enough to be cut from a table rather than read again (see
		// tabulateFrom), cut across two lines.
		item := Item{Text: strings.Repeat("A", 300), Face: face, Size: size, Upright: true}
		head, tail := br.SplitItem(item, 100)
		if head.Width.Px() != want(100) || tail.Width.Px() != want(200) {
			t.Errorf("%s: cut at 100 the halves are %gpx and %gpx, want %g and %g",
				tc.face, head.Width.Px(), tail.Width.Px(), want(100), want(200))
		}
	}
}
