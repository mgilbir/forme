package paragraph

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// TestBreakingOneLongWordIsNotQuadratic is the cost of overflow-wrap on the
// input it is reached for.
//
// A word too long for its line is cut, the rest begins the next line, and the
// rest is cut again — so the work of a line has to be the work of *that* line.
// Four things made it the work of everything still to come: the split measured
// the half it threw away, the search for the cut bisected the whole remaining
// word and so measured half of it on its first step, the cluster boundaries of
// the remainder were found again at every line, and the context each half was
// shaped in was the whole of the other half.
//
// The comment above the search says the input is untrusted and that this is
// reached precisely for the longest word in a document, which is the whole
// reason to measure it. Twenty thousand characters in two-hundred-pixel lines:
// twelve seconds and nine gigabytes before, a quarter of a second and 240 MB
// now.
func TestBreakingOneLongWordIsNotQuadratic(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	const chars = 20000
	item := Item{
		Text: strings.Repeat("a", chars), Face: face, BreakWord: true,
		Size: unit(t, 10),
	}
	item.Width = NewBreaker(nil).MeasureSpaced(item.Face, item.Text, item.Size, TextSpacing{})
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
	// The bound is on memory rather than on time, which is not the usual choice
	// and is the right one here. The race detector's job runs this ten times
	// slower and allocates the same, so a wall-clock bound loose enough to pass
	// there is loose enough for the quadratic to pass as well — while the
	// allocation separates the two by thirty-seven times under either. The
	// elapsed time is reported so that a failure says how long it took.
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<30 {
		t.Errorf("breaking %d characters into %d lines allocated %d bytes in %v; "+
			"the work of a line is the work of that line", chars, lines, grew, elapsed)
	}
}

// TestSplittingAnItemStillMeasuresBothHalves is what the split is for, kept.
//
// Re-measuring rather than apportioning the original width is the point of
// SplitItem: a face may kern or ligate across the cut, so the two pieces do not
// in general add up to the whole. A change that stopped measuring the tail
// there — rather than in the half that throws it away — would make every
// hyphenated line wrong by a fraction of a glyph.
func TestSplittingAnItemStillMeasuresBothHalves(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	br := NewBreaker(nil)
	item := Item{Text: "office", Face: face, Size: unit(t, 20)}
	head, tail := br.SplitItem(item, 3)

	if head.Width <= 0 {
		t.Error("the head was not measured")
	}
	if tail.Width <= 0 {
		t.Error("the tail was not measured")
	}
	if got := br.SplitHead(item, 3); got.Width != head.Width {
		t.Errorf("the head alone measures %v and the head of the pair %v; they are "+
			"the same text in the same context", got.Width, head.Width)
	}
	// And the two halves are the same items either way.
	only := br.SplitHead(item, 3)
	if only.Text != head.Text || only.PostContext != head.PostContext ||
		only.BidiEnd != head.BidiEnd || only.Autospace != head.Autospace {
		t.Errorf("the head alone is %+v and the head of the pair %+v", only, head)
	}
}

func unit(t *testing.T, px float64) style.Unit {
	t.Helper()
	u, ok := style.FromPx(px)
	if !ok {
		t.Fatalf("%v px is not a layout unit", px)
	}
	return u
}

var _ = shape.Features{}

// TestBalancingALongBreakableWordIsBounded is the bound that was on the wrong
// quantity.
//
// The scored search is quadratic in the places a line can begin, and where
// overflow-wrap may cut a word every cluster of that word is one of them. The
// bound counted *items*: one unbreakable word of sixteen hundred characters is
// one item, far under a bound of four hundred, and the audit measured it at
// three minutes and seventeen seconds. An ordinary document reaches it — a box
// with "text-wrap: balance" and a float in it takes this path.
func TestBalancingALongBreakableWordIsBounded(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	br := NewBreaker(nil)
	item := Item{
		Text: strings.Repeat("a", 6000), Face: face, BreakWord: true, Size: unit(t, 10),
	}
	item.Width = br.MeasureSpaced(item.Face, item.Text, item.Size, TextSpacing{})
	items := []Item{item}
	bands := []style.Unit{unit(t, 200), unit(t, 200), unit(t, 150)}

	start := time.Now()
	br.BalanceScoredCaps(items, nil, bands, 0, 3)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("balancing one six-thousand-character word took %v; the search is "+
			"bounded by the places it can start a line at, and there are six "+
			"thousand of them here", elapsed)
	}
}

// TestBalancingAnOrdinaryParagraphIsStillScored is the other side: only the
// items a word may be cut inside are counted by their text, so a paragraph of
// prose is bounded by this exactly as it was by the item count, and nothing
// that was scored before stops being scored.
func TestBalancingAnOrdinaryParagraphIsStillScored(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	br := NewBreaker(nil)
	var items []Item
	for i := 0; i < 60; i++ {
		it := Item{Text: "balanced ", Face: face, Size: unit(t, 10), BreakBefore: i > 0}
		it.Width = br.MeasureSpaced(it.Face, it.Text, it.Size, TextSpacing{})
		items = append(items, it)
	}
	if got := scoredPositions(items); got > maxScoredPositions {
		t.Errorf("sixty words of prose come to %d positions, past the bound of %d; "+
			"an ordinary paragraph must still be scored", got, maxScoredPositions)
	}
}
