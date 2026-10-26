package shape

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// startsMemo is an optimisation and may change no answer, nor the work a run
// is charged. These shape the same text with the memo and without it, on fresh
// faces, and require the glyphs and the work to be the same; and bound what the
// memo may take.

// morxMemoTexts are runs of a face's own characters, as aatCharacters lists
// them: the first 64 alone, and 200 runs of up to twelve drawn at random.
func morxMemoTexts(f *Face, rng *rand.Rand) []string {
	chars := aatCharacters(f, 4096)
	if len(chars) == 0 {
		return nil
	}
	var texts []string
	for i, r := range chars {
		if i < 64 {
			texts = append(texts, string(r))
		}
	}
	for range 200 {
		n := 1 + rng.Intn(12)
		s := make([]rune, n)
		for k := range s {
			s[k] = chars[rng.Intn(len(chars))]
		}
		texts = append(texts, string(s))
	}
	return texts
}

func TestTheStartsMemoIsTheUnmemoisedAnswerOverTheAATFonts(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	shaped := 0
	for _, path := range aatFonts(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		load := func() *Face {
			f, err := Load(data)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			return f
		}
		probe := load()
		if probe.morx == nil {
			continue
		}
		texts := morxMemoTexts(probe, rng)
		if len(texts) == 0 {
			continue
		}
		// A face whose machine runs away (harfBuzzGivesUp) spends the whole
		// allowance on most runs of several characters, which takes long and
		// says nothing more about the memo: it is set its characters alone.
		if _, runaway := harfBuzzGivesUp[filepath.Base(path)]; runaway {
			texts = texts[:min(64, len(texts))]
		}
		shapeAllBounded := func(f *Face) []any {
			var out []any
			for _, s := range texts {
				r, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: s, Kerns: true}, RunLimits{})
				msg := ""
				if err != nil {
					msg = err.Error()
				}
				out = append(out, r, msg)
			}
			return out
		}
		startsMemoOff = true
		want := shapeAllBounded(load())
		startsMemoOff = false
		// Twice on one face: the second time every glyph is answered from
		// the memo the first filled.
		f := load()
		for pass := range 2 {
			got := shapeAllBounded(f)
			for i := range got {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("%s, pass %d: %q\n  memoised   %+v\n  unmemoised %+v",
						filepath.Base(path), pass, texts[i/2], got[i], want[i])
				}
			}
		}
		shaped++
	}
	// The tree's own fixtures hold more than this; fewer means the walk found
	// nothing, and a test that shapes nothing passes whatever it compares.
	if shaped < 35 {
		t.Fatalf("only %d faces with a morx or mort were shaped", shaped)
	}
}

// scatteredMorx is a morx of one noncontextual subtable whose lookup, of
// format 6, names the glyphs below n that in says to, and no others.
func scatteredMorx(n int, in func(gid int) bool) []byte {
	var units []byte
	count := 0
	for gid := 0; gid < n; gid++ {
		if in(gid) {
			units = u16(units, gid, gid)
			count++
		}
	}
	lookup := append(u16(nil, 6, 4, count, 0, 0, 0), units...)
	sub := append(u32(nil, 12+len(lookup), 0x20000004, 1), lookup...)
	chain := append(u32(nil, 1, 16+len(sub), 0, 1), sub...)
	return append(u32(u16(nil, 2, 0), 1), chain...)
}

// TestTheStartsMemoAnswersEachGlyphAsItsSubtableDoes: each glyph's two bits
// are its own. Asked about every glyph of a thousand, in an order that is not
// theirs, twice — once to fill the memo and once to read it — a subtable
// naming every third glyph, and every seventh from one, answers as startsAt
// does. The faces the tests above shape hold thirty glyphs or fewer, which
// fit in one word, so a memo that gave two glyphs one place passed them.
func TestTheStartsMemoAnswersEachGlyphAsItsSubtableDoes(t *testing.T) {
	const n = 1000
	in := func(gid int) bool { return gid%3 == 0 || gid%7 == 1 }
	m := readMorph(scatteredMorx(n, in), n, true)
	if m == nil || len(m.chains) != 1 || len(m.chains[0].subtables) != 1 {
		t.Fatal("the morx was not read")
	}
	s := &m.chains[0].subtables[0]
	order := rand.New(rand.NewSource(1)).Perm(n)
	for pass := range 2 {
		for _, gid := range order {
			if got := s.startsAtMemo(m, gid); got != in(gid) || got != s.startsAt(gid, n) {
				t.Fatalf("pass %d: glyph %d starts the subtable: memo %v, startsAt %v, the lookup %v",
					pass, gid, got, s.startsAt(gid, n), in(gid))
			}
		}
	}
	if s.memo.words == nil {
		t.Fatal("the subtable has no memo, so this asked nothing of one")
	}
}

// memoWordsTaken asks every subtable of m about glyph 1, which makes each
// subtable's memo where the allowance has room for it, and is how many words
// they took and how many subtables went without.
func memoWordsTaken(m *morxTable) (taken int64, without int) {
	for i := range m.chains {
		for j := range m.chains[i].subtables {
			s := &m.chains[i].subtables[j]
			s.startsAtMemo(m, 1)
			if s.memo.words == nil {
				without++
			}
			taken += int64(len(s.memo.words))
		}
	}
	return taken, without
}

// TestTheStartsMemoIsBoundedByTheTablesSize: a memo is two bits a glyph a
// subtable, and a morx of twelve-byte subtables in a face of 65,535 glyphs
// would take 16 KiB of memo for each of them — 1.3 GB for a table of a
// megabyte. What they take between them is held to morxMemoAllowance, the
// table's own size again and 128 KiB; past it, a subtable is asked without one.
func TestTheStartsMemoIsBoundedByTheTablesSize(t *testing.T) {
	const subtables = 80000
	b := emptyMorx(subtables)
	m := readMorph(b, 0xFFFF, true)
	if m == nil || len(m.chains) != 1 || len(m.chains[0].subtables) != subtables {
		t.Fatal("the morx was not read")
	}
	taken, without := memoWordsTaken(m)
	allowance := morxMemoAllowance(len(b))
	if taken > allowance {
		t.Errorf("%d subtables took %d words of memo, past the allowance of %d", subtables, taken, allowance)
	}
	if taken == 0 || without == 0 {
		t.Errorf("%d words taken, %d subtables without: the allowance was not reached, "+
			"so this bounds nothing", taken, without)
	}
	// And a subtable without a memo still answers, as it did before there was
	// one.
	s := &m.chains[0].subtables[subtables-1]
	if s.memo.words != nil || s.startsAtMemo(m, 1) != s.startsAt(1, m.numGlyphs) {
		t.Errorf("the last subtable has a memo, or answers differently without one")
	}
}

// TestEveryRealMorxHasRoomForItsMemos: the allowance is far above what any real
// table takes. Every subtable of every face with a morx or mort in the tree's
// fixtures, and in the Google Fonts checkout where it has been fetched — the
// largest there is Siemreap's, 181 subtables over 696 glyphs, 32 KiB of memo
// for a 90 KiB table — gets its memo.
func TestEveryRealMorxHasRoomForItsMemos(t *testing.T) {
	checked := 0
	for _, path := range aatFonts(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(data)
		if err != nil || f.morx == nil {
			continue
		}
		if _, without := memoWordsTaken(f.morx); without > 0 {
			t.Errorf("%s: %d subtables went without a memo", filepath.Base(path), without)
		}
		checked++
	}
	if checked < 35 {
		t.Fatalf("only %d faces with a morx or mort were checked", checked)
	}
}

// TestStartsMemosAreFilledUnderClonesShapingAtOnce shapes through clones of
// one face from several goroutines. Clones share the face's morx, and so its
// memos, which the first run to ask about a subtable makes and every run
// fills: run under the race detector, this is what says both are safe to race.
func TestStartsMemosAreFilledUnderClonesShapingAtOnce(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", "MorxCases.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if base.morx == nil {
		t.Fatal("MorxCases.ttf has no morx")
	}
	texts := morxMemoTexts(base, rand.New(rand.NewSource(1)))
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		f := base.Clone()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range texts {
				f.ShapeGlyphs(texts[(i+w*7)%len(texts)])
			}
		}()
	}
	wg.Wait()
}
