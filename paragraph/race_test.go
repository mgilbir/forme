package paragraph

import (
	"sync"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// Two paragraphs set at once.
//
// A Breaker owns a memo of measured runs, and a memo is the classic place for
// this to go wrong: a map written from two goroutines is a data race, and Go's is
// not merely unsynchronised but actively fatal — the runtime detects concurrent
// map writes and kills the process. So "is a Breaker shared?" is a question with
// a very sharp answer, and it is worth having one test that asks it rather than
// discovering it from a crash in something that laid two pages out at once.
//
// The supported shape is one Breaker per run, which is what the layout engine
// does: a layouter makes its own in Layout, so two concurrent layouts share
// nothing. This checks that shape really does share nothing — under -race, which
// is where it means something — and that the answers do not depend on how many
// of them are going at once.
//
// It does not check the *unsupported* shape. Racing one Breaker between two
// goroutines is a race by construction, and a test that did it would be asserting
// that Go's race detector works.

// TestBreakersInParallelShareNothing runs one breaker per goroutine over the
// whole corpus and requires every one of them to agree with the answer a single
// breaker gives on its own.
//
// Run this with -race for the half of it that matters. Without the detector it
// still says something — that the results are identical however many are running,
// which is the observable half of "they share nothing" — but the detector is what
// catches a memo reached from two places while both of them still get lucky.
func TestBreakersInParallelShareNothing(t *testing.T) {
	face := courier(t)

	// The answer, computed alone and unhurried.
	want := make([]string, 0, len(texts)*len(widths))
	solo := NewBreaker(nil)
	for _, tc := range texts {
		items := itemsOf(t, solo, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range widths {
			want = append(want, renderLines(brokenLines(t, solo, items, width)))
		}
	}

	const goroutines = 8
	got := make([][]string, goroutines)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// One breaker per goroutine, which is the shape the engine uses.
			br := NewBreaker(nil)
			out := make([]string, 0, len(texts)*len(widths))
			for _, tc := range texts {
				items := itemsOfNoT(br, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
				for _, width := range widths {
					out = append(out, renderLines(breakAllLines(br, items, width)))
				}
			}
			got[g] = out
		}()
	}
	wg.Wait()

	for g, out := range got {
		if len(out) != len(want) {
			t.Fatalf("goroutine %d produced %d answers, want %d", g, len(out), len(want))
		}
		for i := range out {
			if out[i] != want[i] {
				t.Fatalf("goroutine %d disagreed with a breaker running alone on case %d:\n"+
					"  alone:    %q\n  parallel: %q\n"+
					"a breaker's answer must not depend on what else is running",
					g, i, want[i], out[i])
			}
		}
	}
}

// TestOneBreakerReusedAcrossParagraphsStaysCorrect is the other half of the
// memo's contract, and the one a single-threaded caller depends on.
//
// The engine keeps one breaker for a whole document and measures every paragraph
// in it through the same memo. The key has to separate everything that changes an
// answer, and a key that missed something would show up here as one paragraph
// taking another's measurements — silently, and only in a document with both.
func TestOneBreakerReusedAcrossParagraphsStaysCorrect(t *testing.T) {
	face := courier(t)

	// Every text broken by a breaker of its own.
	alone := make([]string, len(texts))
	for i, tc := range texts {
		br := NewBreaker(nil)
		items := itemsOf(t, br, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
		alone[i] = renderLines(brokenLines(t, br, items, 100))
	}

	// And all of them through one, forwards and then backwards, so that every
	// text meets a memo already warmed by every other.
	shared := NewBreaker(nil)
	together := make([]string, len(texts))
	for i, tc := range texts {
		items := itemsOf(t, shared, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
		together[i] = renderLines(brokenLines(t, shared, items, 100))
	}
	for i := len(texts) - 1; i >= 0; i-- {
		items := itemsOf(t, shared, face, texts[i].text, WhiteSpaceOf("collapse"), OverflowWrap{})
		if got := renderLines(brokenLines(t, shared, items, 100)); got != together[i] {
			t.Errorf("%s broke to %q the first time through the shared breaker and %q the "+
				"second", texts[i].name, together[i], got)
		}
	}

	for i, tc := range texts {
		if together[i] != alone[i] {
			t.Errorf("%s broke to %q through a breaker of its own and %q through one "+
				"shared with every other paragraph — the memo's key is not separating "+
				"them", tc.name, alone[i], together[i])
		}
	}
}

// itemsOfNoT is itemsOf without the *testing.T, so that it can be called from a
// goroutine. Reporting from one is what testing.T forbids, and the assertions
// this test makes are all in the main one anyway.
func itemsOfNoT(br *Breaker, face *shape.Face, text string,
	ws WhiteSpace, ow OverflowWrap) []Item {

	size := u(size20)
	pieces, _ := SplitAtBreaks(text, ws, WordBreak{}, LineBreak{})
	out := make([]Item, 0, len(pieces))
	afterCollapsible := true
	for _, p := range pieces {
		if p.Segment {
			out = append(out, Item{Face: face, Size: size, Forced: true})
			afterCollapsible = true
			continue
		}
		if p.Collapsible && afterCollapsible {
			continue
		}
		it := Item{
			Text: p.Text, Face: face, Size: size,
			BreakBefore: p.BreakBefore,
			Space:       p.Space, Collapsible: p.Collapsible, TrimAtEnd: p.TrimAtEnd,
			Tab:       p.Tab,
			Hangs:     p.Space && !p.Collapsible && !ws.BreakSpaces && (ws.Collapse || ws.Wrap),
			HangsHard: p.Space && !p.Collapsible && ws.Collapse,
			NoWrap:    !ws.Wrap,
			BreakWord: ow.BreakWord,
			Anywhere:  ow.Anywhere,
		}
		if !p.Tab {
			it.Width = br.MeasureSpaced(face, p.Text, size, TextSpacing{})
		}
		out = append(out, it)
		afterCollapsible = p.Collapsible
	}
	return out
}

// breakAllLines is brokenLines without the *testing.T, for the same reason. The
// bound is the same one, and reaching it returns what it has rather than
// reporting: a breaker that stalled will disagree with the solo answer, which is
// what the caller checks.
func breakAllLines(br *Breaker, items []Item, width float64) [][]Item {
	var lines [][]Item
	i, iByte := 0, 0
	for i < len(items) && len(lines) <= maxLines(items) {
		line, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, u(width), 0)
		lines = append(lines, line)
		if !CursorAdvanced(i, iByte, next, nextByte) {
			break
		}
		i, iByte = next, nextByte
	}
	return lines
}
