package paragraph

import (
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/segment"
)

// The table must answer what counting answers, at every cut the breaker can
// make. It is an optimisation of an existing function and this is the only
// thing that makes it one.
func TestTheSpacingTableAgreesWithCounting(t *testing.T) {
	for _, text := range []string{
		"hello", strings.Repeat("a", 50), "a b c", "",
		"x​z", "​​", "áb", "مرحبا",
		"é̂f", "\U0001F469‍\U0001F4BB!", "각x",
		"tab\there", "a­b", "\U0001F1E6\U0001F1E7c", "اً",
		// The runs the first table declined: a mark with no base in front of
		// it, marks on a cursive letter, a cursive run with a Latin letter in
		// it, and marks that a zero width space has cut off from their base —
		// the one shape where the whole run's scan and a fresh one disagree.
		"\u0301a", "بًب", "بaب", "ب\u200b\u064e\u064eب", "ب\u200b\u064ea",
		"e\u0301e\u0301", "\u064e\u064e", "ب\u200b\u064e", "a\u200b\u0301b",
		"\xff\u0301b", "a\u00a0b\u00a0", "\U00010100x",
	} {
		checkSpacingTable(t, text)
	}
}

func checkSpacingTable(t *testing.T, text string) {
	t.Helper()
	bounds := segment.Boundaries(nil, text)
	idx := newUnitIndex(text, bounds)
	run := &runIndex{text: text}
	cuts := append([]int{0}, bounds...)
	if len(text) > 0 {
		cuts = append(cuts, len(text))
	}

	// Every head, every tail, and every single cluster — which is what the
	// breaker actually asks for, and is linear in the cuts. Every *pair* of
	// cuts would be quadratic in them and quadratic again inside SpacedUnits,
	// which is cubic in the input: at four thousand bytes that took ten seconds
	// and killed the fuzz worker. A test that cannot run is not a check.
	check := func(from, to int) {
		t.Helper()
		if !idx.covers(from, to) {
			t.Errorf("%q[%d:%d]: both ends are cluster boundaries and the table "+
				"does not cover them", text, from, to)
			return
		}
		piece := text[from:to]
		if got, want := idx.spacedUnits(from, to), SpacedUnits(piece); got != want {
			t.Errorf("%q[%d:%d] = %q: the table says %d spaced units and counting says %d",
				text, from, to, piece, got, want)
		}
		if got, want := idx.uprightUnits(from, to), UprightUnits(piece); got != want {
			t.Errorf("%q[%d:%d] = %q: the table says %d upright units and counting says %d",
				text, from, to, piece, got, want)
		}
		if got, want := run.wordSeparators(from, to), countWordSeparators(piece); got != want {
			t.Errorf("%q[%d:%d] = %q: the table says %d word separators and counting says %d",
				text, from, to, piece, got, want)
		}
		if got, want := run.runesTo(to), utf8.RuneCountInString(text[:to]); got != want {
			t.Errorf("%q[:%d]: the table says %d characters and counting says %d",
				text, to, got, want)
		}
	}
	for i, c := range cuts {
		check(0, c)
		check(c, len(text))
		if i+1 < len(cuts) {
			check(c, cuts[i+1])
		}
	}
}

func FuzzTheSpacingTableAgreesWithCounting(f *testing.F) {
	for _, s := range []string{
		"hello", "áb", "مرحبا", "x​z",
		"\U0001F469‍\U0001F4BB", "각", "", "a­b",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1024 {
			return
		}
		checkSpacingTable(t, text)
	})
}

// TestBreakingOneLongWordWithLetterSpacingIsNotQuadratic is the same shape as
// TestBreakingOneLongWordIsNotQuadratic with a letter-spacing on it, which is
// the case that one did not cover and that stayed quadratic after it.
//
// The break search asks the width of a candidate head once per cluster of the
// word, and with a letter-spacing the width asks how many units that head
// carries. Counted, that is the whole remaining word read again at every
// candidate: sixteen thousand characters took 315ms and allocated 302MB where
// the same word without a letter-spacing took 44ms and allocated 33MB, and the
// curve climbed by a factor of four per doubling against the other's two.
//
// The bound is on allocation for the reason the test above gives: the race
// detector's job runs this ten times slower and allocates the same.
func TestBreakingOneLongWordWithLetterSpacingIsNotQuadratic(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	const chars = 16000
	spacing := TextSpacing{Letter: unit(t, 1)}
	item := Item{
		Text: strings.Repeat("a", chars), Face: face, BreakWord: true,
		Size: unit(t, 10), Spacing: spacing,
	}
	item.Width = NewBreaker(nil).MeasureSpaced(item.Face, item.Text, item.Size, spacing)
	width := unit(t, 200)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()

	br := NewBreaker(nil)
	items := []Item{item}
	lines := 0
	for from, fromByte := 0, 0; from < len(items); {
		line, next, nextByte, _, _, _ := br.BreakOneLine(items, from, fromByte, width, 0)
		if len(line) == 0 && next == from && nextByte == fromByte {
			t.Fatal("the breaker stopped making progress")
		}
		from, fromByte = next, nextByte
		lines++
		if lines > chars {
			t.Fatal("more lines than characters")
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	if lines < 10 {
		t.Fatalf("%d characters in a 200px line came to %d lines; the fixture is wrong",
			chars, lines)
	}
	// 33MB with the table and 302MB without it, so this sits between them with
	// room on both sides rather than against either.
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 128<<20 {
		t.Errorf("breaking %d characters with a letter-spacing into %d lines allocated "+
			"%d bytes in %v; a letter-spacing must not make the search read the whole "+
			"remaining word at every candidate", chars, lines, grew, elapsed)
	}
}
