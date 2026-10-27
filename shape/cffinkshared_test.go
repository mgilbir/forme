package shape

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A CFF face's ink and a VARC face's are kept on what every clone of the face
// shares, and the face is shared by every document set through it at once:
// the reads are a load, and only measuring takes the lock.

// TestOutlineInkIsSharedAcrossGoroutines measures every glyph of CFF, CFF2 and
// VARC faces from eight goroutines at once, each through its own clone of one
// face and each in its own order, and holds every answer to what a face used
// by one goroutine alone answers, and the work the shared face spent to what
// the face alone spent: a glyph measured twice would be charged twice. It asks
// glyph indices the faces do not have as well. Under -race it is also the test
// that reading the kept answers without the lock is sound.
func TestOutlineInkIsSharedAcrossGoroutines(t *testing.T) {
	type source struct {
		name string
		data func(t *testing.T) []byte
	}
	fixture := func(name string) source {
		return source{name, func(t *testing.T) []byte { return harfbuzzFont(t, name) }}
	}
	corpus := func(name string) source {
		return source{name, func(t *testing.T) []byte {
			dir := os.Getenv("CFF_FONTS")
			if dir == "" {
				t.Skip("CFF_FONTS is not set")
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			return data
		}}
	}
	for _, tc := range []source{
		fixture("CFFInk.otf"),
		fixture("CFF2Blend.otf"),
		fixture("VarComposite.ttf"),
		fixture("VarCompositeCFF.otf"),
		corpus("SourceSerif4-Regular.otf"),
		corpus("SourceSans3VF-Upright.otf"),
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.data(t)
			alone, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			shared, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			if alone.ink == nil && alone.varc == nil {
				t.Fatalf("%s loaded with neither CFF ink nor a VARC table read", tc.name)
			}
			// What the face has spent measuring: the CFF budget, and the
			// VARC work, which counts down from what it was loaded with.
			var varcWork int64
			if alone.varc != nil {
				varcWork = alone.varc.work
			}
			spent := func(f *Face) (cff, varc int64) {
				if f.ink != nil && f.ink.budget != nil {
					cff = int64(f.ink.budget.Spent())
				}
				if f.varc != nil {
					varc = varcWork - f.varc.work
				}
				return cff, varc
			}
			n := alone.NumGlyphs()
			gids := []int{-1, n, n + 7}
			for gid := range n {
				gids = append(gids, gid)
			}
			type answer struct {
				ext extents
				ok  bool
			}
			ask := func(f *Face, gid int) answer {
				ext, ok := f.glyphExtents(gid)
				return answer{ext, ok}
			}
			want := make([]answer, len(gids))
			inked := 0
			for i, gid := range gids {
				want[i] = ask(alone, gid)
				if want[i].ok && want[i].ext != (extents{}) {
					inked++
				}
			}
			if inked == 0 {
				t.Fatalf("%s has ink for none of its glyphs, so this tests nothing", tc.name)
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
			ac, av := spent(alone)
			sc, sv := spent(shared)
			if ac != sc || av != sv {
				t.Errorf("shared, the face spent %d CFF and %d VARC units measuring, and alone %d and %d", sc, sv, ac, av)
			}
			if ac == 0 && av == 0 {
				t.Errorf("%s spent nothing measuring, so what it spent says nothing", tc.name)
			}
			if a, b := alone.LayoutLimits(), shared.LayoutLimits(); len(a) != len(b) {
				t.Errorf("limits shared %v, and alone %v", b, a)
			}
		})
	}
}

// cffBenchFace is a face of the CFF corpus and a run of eighty characters it
// draws, or a skip without the corpus.
func cffBenchFace(b *testing.B, name string, from rune) (*Face, string) {
	dir := os.Getenv("CFF_FONTS")
	if dir == "" {
		b.Skip("CFF_FONTS is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		b.Skip(err)
	}
	f, err := Load(data)
	if err != nil {
		b.Fatal(err)
	}
	if f.ink == nil {
		b.Fatalf("%s loaded with no CFF ink", name)
	}
	var text []rune
	for r := from; len(text) < 80; r++ {
		if gid, ok := f.prog.Cmap[r]; ok && gid > 0 {
			text = append(text, r)
		}
	}
	return f, string(text)
}

// BenchmarkCFFInkExtentParallel measures the ink of a run through clones of
// one shared CFF face, and of one shared CFF2 face, as concurrent layouts do
// (textInk asks it of every run drawn), once every glyph has been measured.
// Run it with -cpu 1,4,8: what it is for is how the time scales.
func BenchmarkCFFInkExtentParallel(b *testing.B) {
	for _, tc := range []struct {
		name string
		from rune
	}{
		{"SourceSerif4-Regular.otf", '!'},
		{"NotoSansJP-VF.otf", 0x4E00},
	} {
		b.Run(tc.name, func(b *testing.B) {
			f, text := cffBenchFace(b, tc.name, tc.from)
			if _, _, ok := f.InkExtent(text, 16); !ok {
				b.Fatalf("%s has no ink for %q", tc.name, text)
			}
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				c := f.Clone()
				for pb.Next() {
					c.InkExtent(text, 16)
				}
			})
		})
	}
}

// BenchmarkVARCInkParallel measures every glyph of VarComposite.ttf through
// clones of one shared face, once every glyph has been measured. There is no
// VARC face a foundry made in the corpora; this one is twenty glyphs built to
// say what HarfBuzz reads of the table, so it shows the lock and not a real
// face's mix of glyphs.
func BenchmarkVARCInkParallel(b *testing.B) {
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", "VarComposite.ttf"))
	if err != nil {
		b.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		b.Fatal(err)
	}
	if f.varc == nil {
		b.Fatal("VarComposite.ttf loaded with no VARC table read")
	}
	n := f.NumGlyphs()
	for gid := range n {
		f.glyphExtents(gid)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		c := f.Clone()
		for pb.Next() {
			for gid := range n {
				c.glyphExtents(gid)
			}
		}
	})
}

// TestACFFGlyphIsMeasuredAfterItClipsAColourGlyph measures every glyph of a CFF
// face after a colour glyph's clip has drawn one, which reads the table
// through a path of its own (colrInk.cffOutline), and holds each to a face
// that only measured. No face of the corpora has both COLR and CFF outlines,
// so the clip is asked of the CFF face directly.
func TestACFFGlyphIsMeasuredAfterItClipsAColourGlyph(t *testing.T) {
	data := harfbuzzFont(t, "CFFInk.otf")
	clipped, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	measured, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if clipped.ink == nil || clipped.ink.cff2 != nil {
		t.Fatal("CFFInk.otf loaded with no CFF ink")
	}
	if b := (&colrInk{f: clipped}).cffOutline(1); b == void32 {
		t.Fatal("glyph 1 of CFFInk.otf draws nothing, so its clip read nothing")
	}
	for gid := -1; gid <= clipped.NumGlyphs(); gid++ {
		ge, gok := clipped.glyphExtents(gid)
		we, wok := measured.glyphExtents(gid)
		if ge != we || gok != wok {
			t.Errorf("glyph %d: %+v %v after a clip, and %+v %v alone", gid, ge, gok, we, wok)
		}
	}
}
