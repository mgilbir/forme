package shape

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A COLR face's ink and its painting verdicts are kept on the colrInk every
// clone of the face shares, and the face is shared by every document set
// through it at once: the reads are a load, and only measuring takes the lock.

// TestColourInkIsSharedAcrossGoroutines measures and paints every glyph of the
// colour faces from eight goroutines at once, each through its own clone of
// one face and each in its own order, and holds every answer to what a face
// used by one goroutine alone answers. It asks glyph indices the face does not
// have as well, which are kept apart from the rest. Under -race it is also the
// test that reading the kept answers without the lock is sound.
func TestColourInkIsSharedAcrossGoroutines(t *testing.T) {
	for _, name := range []string{"ColourInk.ttf", "ColourPaint.ttf"} {
		t.Run(name, func(t *testing.T) {
			data := harfbuzzFont(t, name)
			alone, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			shared, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			if alone.colr == nil || alone.colrTable() == nil {
				t.Fatalf("%s loaded with no COLR table read", name)
			}
			n := alone.NumGlyphs()
			gids := []int{-1, n, n + 7}
			for gid := range n {
				gids = append(gids, gid)
			}
			type answer struct {
				ext     extents
				painted bool
				paint   string
			}
			ask := func(f *Face, gid int) answer {
				ext, painted := f.colr.extents(gid)
				a := answer{ext: ext, painted: painted}
				if gid >= 0 && gid < n {
					a.paint = paintedText(f, gid, PaintOptions{}, paintWork)
				}
				return a
			}
			want := make([]answer, len(gids))
			painted := 0
			for i, gid := range gids {
				want[i] = ask(alone, gid)
				if want[i].painted {
					painted++
				}
			}
			if painted == 0 {
				t.Fatalf("%s paints none of its glyphs, so this tests nothing", name)
			}
			const workers = 8
			got := make([][]answer, workers)
			var wg sync.WaitGroup
			for w := range workers {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					f := shared.Clone()
					got[w] = make([]answer, len(gids))
					// Each goroutine starts somewhere else and half go
					// backwards, so they race for different glyphs as well
					// as the same ones.
					for k := range gids {
						i := (k + w*len(gids)/workers) % len(gids)
						if w%2 == 1 {
							i = len(gids) - 1 - i
						}
						got[w][i] = ask(f, gids[i])
					}
				}(w)
			}
			wg.Wait()
			for w := range workers {
				for i, gid := range gids {
					if got[w][i] != want[i] {
						t.Errorf("goroutine %d, glyph %d: %+v, and alone %+v", w, gid, got[w][i], want[i])
					}
				}
			}
			if a, b := alone.LayoutLimits(), shared.LayoutLimits(); len(a) != len(b) {
				t.Errorf("limits shared %v, and alone %v", b, a)
			}
		})
	}
}

// colrBenchFace is Noto Color Emoji's COLRv1 build, its colour glyphs and a run
// of eighty emoji, or a skip without the corpus.
func colrBenchFace(b *testing.B) (*Face, []int, string) {
	dir := os.Getenv("EMOJI_FONTS")
	if dir == "" {
		b.Skip("EMOJI_FONTS is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "Noto-COLRv1.ttf"))
	if err != nil {
		b.Skip(err)
	}
	f, err := Load(data)
	if err != nil {
		b.Fatal(err)
	}
	var gids []int
	for gid := range f.NumGlyphs() {
		if f.GlyphColour(gid, 0) == ColourPaint {
			gids = append(gids, gid)
		}
	}
	var text []rune
	for r := rune(0x1F600); r < 0x1F650; r++ {
		text = append(text, r)
	}
	return f, gids, string(text)
}

// BenchmarkCOLRInkExtentParallel measures the ink of a run of emoji through
// clones of one shared COLR face, as concurrent layouts drawing emoji do
// (textInk asks it of every run drawn), once every glyph has been measured.
// Run it with -cpu 1,4,8: what it is for is how the time scales.
func BenchmarkCOLRInkExtentParallel(b *testing.B) {
	f, _, text := colrBenchFace(b)
	f.InkExtent(text, 16)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		c := f.Clone()
		for pb.Next() {
			c.InkExtent(text, 16)
		}
	})
}

// BenchmarkCOLRPaintParallel paints colour glyphs through clones of one shared
// COLR face, once every glyph has been counted.
func BenchmarkCOLRPaintParallel(b *testing.B) {
	f, gids, _ := colrBenchFace(b)
	gids = gids[:min(len(gids), 256)]
	for _, g := range gids {
		f.PaintGlyph(g, PaintOptions{}, &countingPainter{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		c := f.Clone()
		i := 0
		for pb.Next() {
			c.PaintGlyph(gids[i%len(gids)], PaintOptions{}, &countingPainter{})
			i++
		}
	})
}
