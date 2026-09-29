package shape

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
)

// A gate is an optimisation and may change no answer. These shape the same
// text with every gate held open and with them shut, on fresh faces so that
// nothing either shaped is remembered by the other, and require the glyphs to
// be the same in every field.

func shapeAll(f *Face, texts []string) [][]Glyph {
	out := make([][]Glyph, len(texts))
	for i, s := range texts {
		g, _ := f.ShapeGlyphs(s)
		out[i] = g
	}
	return out
}

func requireSameWithAndWithoutGates(t *testing.T, name string, data []byte, texts []string) {
	t.Helper()
	load := func() *Face {
		f, err := Load(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return f
	}
	gposGateOff = true
	defer func() { gposGateOff = false }()
	want := shapeAll(load(), texts)
	gposGateOff = false
	got := shapeAll(load(), texts)
	for i := range texts {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("%s: %s\n  gated   %v\n  ungated %v", name, describeRunes(texts[i]), got[i], want[i])
		}
	}
}

// gateOf builds the gate of a single-adjustment lookup whose one subtable
// names the coverage given, which is all a gate reads of it.
func gateOf(coverage []byte, copies int) []uint64 {
	sub := append([]byte{0, 1, 0, 6, 0, 0}, coverage...)
	lk := rawLookup{kind: 1, markSet: -1}
	for i := 0; i < copies; i++ {
		lk.subs = append(lk.subs, sub)
	}
	return buildGate(lk)
}

func TestAGateNamesTheGlyphsOfItsCoverageAndNoOthers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		coverage []byte
		in       []int
	}{
		{"format 1", []byte{0, 1, 0, 2, 0, 3, 0, 200}, []int{3, 200}},
		{"format 2", []byte{0, 2, 0, 2, 0, 64, 0, 130, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 2},
			[]int{64, 65, 127, 128, 130, 65535}},
	} {
		g := &gposGate{words: gateOf(tc.coverage, 1)}
		want := map[int]bool{}
		for _, gid := range tc.in {
			want[gid] = true
		}
		if tc.name == "format 2" {
			for gid := 64; gid <= 130; gid++ {
				want[gid] = true
			}
		}
		for gid := -1; gid <= 65536; gid++ {
			if got := g.startsAt(gid); got != want[gid] {
				t.Fatalf("%s: startsAt(%d) = %v, want %v", tc.name, gid, got, want[gid])
			}
		}
	}
}

func TestAGateThatWouldCostMoreThanItMayIsLeftOpen(t *testing.T) {
	full := []byte{0, 2, 0, 1, 0, 0, 0xff, 0xff, 0, 0}
	if words := gateOf(full, 20); words == nil || !(&gposGate{words: words}).startsAt(65535) {
		t.Fatalf("twenty subtables naming every glyph should be read, and name every glyph")
	}
	if words := gateOf(full, 2000); words != nil {
		t.Fatalf("2000 subtables of 65536 glyphs each were read whole; want the lookup left ungated")
	}
	if !(*gposGate)(nil).startsAt(7) || !(&gposGate{}).startsAt(7) {
		t.Fatalf("a lookup with no gate must be open to every glyph")
	}
}

func TestGatedPositioningIsTheUngatedAnswerOverTheHarfBuzzCorpora(t *testing.T) {
	for _, tc := range harfbuzzCases {
		t.Run(tc.name, func(t *testing.T) {
			corpus, _, _ := readHarfBuzzGolden(t, tc.corpus, tc.expected)
			data := notoSansBytes(t)
			if tc.font != "" {
				var err error
				data, err = os.ReadFile(filepath.Join(harfbuzzDir, tc.font))
				if err != nil {
					t.Fatal(err)
				}
			}
			requireSameWithAndWithoutGates(t, tc.name, data, corpus)
		})
	}
}

// Text drawn at random from what a face's own character map holds is where the
// lookups for marks, for ligature components and for other scripts' pairs are
// all reached, which a corpus written to check HarfBuzz reaches less evenly.
func TestGatedPositioningIsTheUngatedAnswerOverTheNotoFaces(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("NOTO_FONTS is not set")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.[to]tf"))
	if len(paths) == 0 {
		t.Skip("no fonts in NOTO_FONTS")
	}
	rng := rand.New(rand.NewSource(863))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(data)
		if err != nil {
			continue
		}
		var runes []rune
		for r := range f.Cmap() {
			runes = append(runes, r)
		}
		sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
		if len(runes) == 0 {
			continue
		}
		texts := make([]string, 0, 400)
		for len(texts) < 400 {
			n := 1 + rng.Intn(12)
			var s []rune
			for k := 0; k < n; k++ {
				s = append(s, runes[rng.Intn(len(runes))])
			}
			texts = append(texts, string(s))
		}
		requireSameWithAndWithoutGates(t, filepath.Base(path), data, texts)
	}
}

// TestGatesAreBuiltOnceUnderClonesShapingAtOnce shapes through clones of one
// face from several goroutines. Clones share the face's layouts, and so its
// gates, and the first walk of a lookup builds its gate: run under the race
// detector, this is what says that build is safe to race. With the Once
// replaced by a nil check it reports a data race in gateFor.
func TestGatesAreBuiltOnceUnderClonesShapingAtOnce(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("NOTO_FONTS is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSans-Regular.ttf"))
	if err != nil {
		t.Skip(err)
	}
	base, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		f := base.Clone()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				f.ShapeGlyphs(fmt.Sprintf("AVATAR Wórld fi fl %d", w*1000+i))
			}
		}()
	}
	wg.Wait()
}
