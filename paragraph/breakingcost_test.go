package paragraph

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// The cost of breaking a paragraph, as the shape of a curve.
//
// Every guard here is a ratio: the same work at four times the input, against
// the work at the input. Linear is four and quadratic is sixteen, on any
// machine and under the race detector, which runs everything ten times slower
// and so rules out a bound on the clock tight enough to mean anything. Eight is
// between the two with room on both sides rather than against either.
//
// Five things keep the ratio honest, and each was learned from getting it
// wrong.
//
//   - It is measured on the thread's own processor clock and not on the wall
//     (see onThreadClock). A test suite runs packages side by side, and with the
//     machine that busy a wall-clock ratio of a linear walk came out anywhere
//     from three to twelve.
//   - The input is small enough that four times it still sits in the cache. An
//     Item is a large struct, and a paragraph of sixteen thousand of them read a
//     linear walk as a factor of seven, which is a cache and not a curve.
//   - Both sizes are built and run once before either is timed, and the two are
//     then timed in turn, so that both are measured in the same heap and the
//     same stretch of the machine's day.
//   - The collector is off while the work is timed; see costRatio.
//   - The work is repeated until it takes long enough to measure, because a
//     ratio of two numbers in the noise is noise.
//
// Each was planted — the fix it guards put back the way it was — and seen to
// fail, which is the only evidence that a guard can fail at all. The factor the
// plant produced is written beside each.

// costRatio times reps calls of run at n and at four times n, each as the best
// of five because the clock is noisy upward only, and returns both and their
// ratio.
func costRatio(t *testing.T, n, reps int, run func(n int)) (small, large time.Duration, ratio float64) {
	t.Helper()
	took := func(n int) time.Duration {
		// The collector waits while the work is timed, and runs again between
		// timings. A collection marks what is live, and while the work runs what
		// is live includes the output it is building — so work that is linear
		// and allocates as it goes is charged more per collection the larger its
		// input, which is the runtime's curve and not the code's. A line of a
		// thousand words and boxes read as a factor of six with it running.
		//
		// It waits for a limit and not for ever. Work that is quadratic in what
		// it allocates — which is what a regression here would be — would
		// otherwise grow the heap until the process was killed, and a guard
		// that fails by taking the test binary down with it reports nothing.
		// Past the limit the collector runs, the curve steepens, and the ratio
		// says so.
		//
		// And it runs to completion before each timing, so that every timing
		// starts from the same clean heap. Left to run only in the moments
		// between timings, it fell behind, the garbage of earlier timings piled
		// up to the limit, and the collection the limit forced landed inside
		// whichever timing was unlucky: a linear walk read as thirteen.
		runtime.GC()
		defer debug.SetGCPercent(debug.SetGCPercent(-1))
		defer debug.SetMemoryLimit(debug.SetMemoryLimit(timedHeapLimit))
		return onThreadClock(func() {
			for r := 0; r < reps; r++ {
				run(n)
			}
		})
	}
	run(n)
	run(4 * n)
	small, large = time.Duration(1<<62), time.Duration(1<<62)
	for i := 0; i < 5; i++ {
		small, large = min(small, took(n)), min(large, took(4*n))
	}
	if small <= 0 {
		t.Fatalf("%d measured as %v; there is nothing to compare", n, small)
	}
	return small, large, float64(large) / float64(small)
}

// timedHeapLimit is how far the heap may grow while work is timed before the
// collector runs anyway. See costRatio.
const timedHeapLimit = 512 << 20

// checkLinear fails when four times the input took more than eight times as
// long, three times running.
//
// Three, because a measurement can be disturbed for longer than the best of
// five covers — once in thirty-two runs a linear walk read as a factor of 8.3,
// every one of its five timings slow together — and a quadratic is not a
// disturbance: it reads sixteen every time it is measured. A failure that
// does not reproduce is the machine; one that does is the code.
func checkLinear(t *testing.T, what string, n, reps int, run func(n int)) {
	t.Helper()
	var small, large time.Duration
	var ratio float64
	for attempt := 0; attempt < 3; attempt++ {
		small, large, ratio = costRatio(t, n, reps, run)
		t.Logf("%s: %d took %v and %d took %v, a factor of %.1f", what, n, small, 4*n, large, ratio)
		if ratio <= 8 {
			return
		}
	}
	t.Errorf("%s: %d took %v and %d took %v, a factor of %.1f for four times the "+
		"input, three times running; linear work is four and quadratic sixteen",
		what, n, small, 4*n, large, ratio)
}

// TestResumingAWordCostsTheLineNotThePosition is what the fill asks at the
// start of every line of a word broken across lines, where the line resumes
// the word at the offset the line before reached.
//
// Each of these questions was asked of the whole run, or of everything in front
// of the cut, once per line — so the line at offset k cost k, and a word cost
// the square of its length. Each is asked here on its own, at every sixteenth
// cluster of a long word, because the rest of what a line does costs enough to
// hide any one of them at a length a test can afford. And the word is shaped,
// and its tables built, before anything is timed: that is work done once for
// the word, and it is linear however it is done, so timing it too only dilutes
// the curve being asked about.
func TestResumingAWordCostsTheLineNotThePosition(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	size := unit(t, 10)
	spaced := TextSpacing{Letter: unit(t, 1), Word: unit(t, 2)}
	const every = 16

	// resumed is one long word and what the breaker has learned of it.
	type resumed struct {
		br     *Breaker
		item   Item
		cut    *RunCut
		bounds clusters
	}
	// word is n units of one kind as an item, with a breaker of its own.
	//
	// The breaker has cut other runs before this one, as it has on any page
	// with more than one broken word, and that is not scenery. A map that
	// holds eight entries or fewer is searched by comparing keys, which for
	// the run's own string is a comparison of one pointer; past eight it
	// hashes the key, which reads the whole run. A test on a breaker that had
	// only ever seen one word would pass with every lookup of the run's
	// boundaries hashing it.
	word := func(unit string, sp TextSpacing, upright bool) func(n int) resumed {
		return cached(func(n int) resumed {
			br := NewBreaker(nil)
			for i := 0; i < 16; i++ {
				other := Item{Text: strings.Repeat(string(rune('a'+i)), 300+i), Face: face,
					Size: size, BreakWord: true}
				other.Cut = &RunCut{Text: other.Text}
				br.clustersOf(other)
			}
			text := strings.Repeat(unit, n)
			item := Item{Text: text, Face: face, Size: size, BreakWord: true, Spacing: sp,
				Upright: upright}
			return resumed{br: br, item: item, cut: &RunCut{Text: text}, bounds: br.clustersOf(item)}
		})
	}
	// stretch is the rest of the word from its kth cluster, as the fill's
	// split makes it.
	stretch := func(w resumed, k int) Item {
		at := w.bounds.at(k)
		tail := w.item
		tail.Text, tail.Cut, tail.CutAt = w.item.Text[at:], w.cut, at
		return tail
	}

	for _, tc := range []struct {
		name string
		n    int
		reps int
		ask  func(n int)
	}{{
		// Audit C49. The line resumes the original item, and where the tail
		// begins in the bidi paragraph was the characters in front of the cut,
		// counted — 38% of the time a quarter of a million characters took to
		// lay out. "é" is one character of two bytes, so that the count decodes.
		// Planted (counted again): a factor of 15.1.
		name: "the characters in front of the cut", n: 16000, reps: 2,
		ask: func() func(n int) {
			words := word("é", TextSpacing{}, false)
			return func(n int) {
				w := words(n)
				for k := 0; k < w.bounds.len(); k += every {
					w.br.SplitTail(w.item, w.bounds.at(k))
				}
			}
		}(),
	}, {
		// Audit C173. Where the rest of the word may be cut was found through
		// a map keyed on the run's text, whose hash reads every byte of it.
		// Planted (the map asked every time): a factor of 13.2.
		name: "where the rest may be cut", n: 32000, reps: 8,
		ask: func() func(n int) {
			words := word("a", TextSpacing{}, false)
			return func(n int) {
				w := words(n)
				for k := 0; k < w.bounds.len(); k += every {
					w.br.clustersOf(stretch(w, k))
				}
			}
		}(),
	}, {
		// Audit C173 again: the shaping a stretch is measured from, found the
		// same way. Planted (no comparison before the map): a factor of
		// 13.6.
		name: "the shaping a head is measured from", n: 32000, reps: 8,
		ask: func() func(n int) {
			words := word("a", TextSpacing{}, false)
			return func(n int) {
				w := words(n)
				for k := 0; k < w.bounds.len(); k += every {
					tail := stretch(w, k)
					w.br.spanWidth(tail, 0, 1, Item{Text: tail.Text[:1]})
				}
			}
		}(),
	}, {
		// The letter-spacing of a stretch of a run with a mark in it, which the
		// first table declined and so counted; and its word-spacing, which had
		// no table at all. Planted: the letters counted, a factor of 17.8;
		// the word separators counted, 17.0.
		name: "the spacing a stretch carries", n: 4000, reps: 8,
		ask: func() func(n int) {
			words := word("e\u0301\u00a0", spaced, false)
			return func(n int) {
				w := words(n)
				for k := 0; k < w.bounds.len(); k += every {
					tail := stretch(w, k)
					w.br.spacingIn(tail, 0, len(tail.Text), tail.Text)
				}
			}
		}(),
	}, {
		// Whether §8.2 takes the spacing off a stretch of a cursive run, which
		// reads to the end of the stretch to find that no unit follows.
		// Planted (counted): a factor of 17.1.
		name: "whether a cursive stretch is tracked", n: 4000, reps: 8,
		ask: func() func(n int) {
			words := word("ب", spaced, false)
			return func(n int) {
				w := words(n)
				for k := 0; k < w.bounds.len(); k += every {
					w.br.trailingSpacing(stretch(w, k))
				}
			}
		}(),
	}, {
		// The advance of a stretch set upright, which is a count of its
		// characters and was counted. A breaker of its own each time, because
		// the old path measured the stretch as text through a memo, and a
		// memo answers the second time it is asked — which a line never does,
		// since each line's stretch is a different one. Planted (measured as
		// text): a factor of 17.7.
		name: "the advance of an upright stretch", n: 4000, reps: 4,
		ask: func() func(n int) {
			words := word("x", TextSpacing{}, true)
			return func(n int) {
				w := words(n)
				w.br = NewBreaker(nil)
				for k := 0; k < w.bounds.len(); k += every {
					tail := stretch(w, k)
					w.br.spanWidth(tail, 0, len(tail.Text), tail)
				}
			}
		}(),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			checkLinear(t, tc.name, tc.n, tc.reps, tc.ask)
		})
	}
}

// cached builds an input once for each size it is asked for, so that what is
// timed is the work and not the building of what it works on.
func cached[T any](build func(n int) T) func(n int) T {
	made := map[int]T{}
	return func(n int) T {
		v, ok := made[n]
		if !ok {
			v = build(n)
			made[n] = v
		}
		return v
	}
}

// breakEveryLine breaks a prepared paragraph to the end.
func breakEveryLine(t *testing.T, lines *Lines, width style.Unit) {
	t.Helper()
	for i, iByte := 0, 0; i < len(lines.Items()); {
		_, next, nextByte, _, _, _ := lines.BreakOneLine(i, iByte, width, 0)
		if !CursorAdvanced(i, iByte, next, nextByte) {
			t.Fatal("the breaker stopped making progress")
		}
		i, iByte = next, nextByte
	}
}

// TestALineDoesNotWalkTheRestOfTheParagraph is audit C50: a paragraph with no
// forced break in it, broken line by line.
//
// The fill found the next forced break by walking from the line's start, and
// the white space in front of it by walking back — the whole rest of the
// paragraph for every line. Prepared once as Lines, each line reads both.
// Planted (the walk put back into the prepared path): a factor of 10.1.
func TestALineDoesNotWalkTheRestOfTheParagraph(t *testing.T) {
	face := courier(t)
	br := NewBreaker(nil)
	paragraph := cached(func(n int) []Item {
		return words(t, br, face, strings.TrimSpace(strings.Repeat("ab ", n)))
	})
	// Lines of a word or two, so that the paragraph has as many lines as it
	// can: the walk was per line, and a wide measure hides it behind the work
	// of filling each one.
	checkLinear(t, "breaking a paragraph of words", 1000, 8, func(n int) {
		breakEveryLine(t, br.Lines(paragraph(n)), u(60))
	})
}

// TestAParagraphEndingInHangingSpaceIsNotWalkedBackPerLine is the other half of
// C50: the walk back from the end of the paragraph over the white space that
// ends it, which a paragraph ending in a long run of ideographic spaces — an
// item each — paid on every line before it. Planted: a factor of 13.4.
func TestAParagraphEndingInHangingSpaceIsNotWalkedBackPerLine(t *testing.T) {
	face := courier(t)
	br := NewBreaker(nil)
	paragraph := cached(func(n int) []Item {
		items := words(t, br, face, strings.TrimSpace(strings.Repeat("ab ", n)))
		for i := 0; i < n; i++ {
			items = append(items, Item{Text: "\u3000", Face: face, Size: u(size20),
				Width: u(24), Space: true, Hangs: true, BreakBefore: true})
		}
		return items
	})
	checkLinear(t, "breaking a paragraph that ends in hanging spaces", 500, 8, func(n int) {
		breakEveryLine(t, br.Lines(paragraph(n)), u(300))
	})
}

// TestBalancingProbesDoNotWalkTheRestOfTheParagraph is C50 where the audit says
// it is paid again: every probe text-wrap: balance and line-clamp make breaks
// the paragraph from the start, and a clamp of a million lines counts every
// line of every probe. Planted (every line of a probe preparing the paragraph
// again): a factor of 14.6 for the clamp and 14.5 for the count.
func TestBalancingProbesDoNotWalkTheRestOfTheParagraph(t *testing.T) {
	face := courier(t)
	br := NewBreaker(nil)
	paragraph := cached(func(n int) []Item {
		return words(t, br, face, strings.TrimSpace(strings.Repeat("ab ", n)))
	})
	checkLinear(t, "balancing a clamped paragraph", 500, 2, func(n int) {
		br.BalanceClampedWidth(paragraph(n), nil, u(300), 0, u(12), maxClampLines)
	})
	checkLinear(t, "counting the lines of a paragraph", 1000, 4, func(n int) {
		br.countLinesInBands(paragraph(n), nil, []style.Unit{u(300)}, u(300), 0, n)
		br.countLines(paragraph(n), nil, u(300), 0, n)
	})
}

// TestTopAlignedBoxesAreFoundByTheirSubtree is audit C51: a line holding many
// "vertical-align: top" boxes, each its own aligned subtree. Each was looked
// for among all the subtrees gathered so far, while the line was stacked and
// again when each box was placed. Planted (the scan): a factor of 21.2.
func TestTopAlignedBoxesAreFoundByTheirSubtree(t *testing.T) {
	line := cached(func(n int) []Item {
		runs := make([]Item, n)
		for i := range runs {
			runs[i] = Item{Leads: true, Above: u(10), Below: u(2),
				Valign: VAlignState{LineAlign: VAlignTop, Subtree: new(int)}}
		}
		return runs
	})
	checkLinear(t, "stacking a line of top-aligned boxes", 1000, 16, func(n int) {
		runs := line(n)
		ls := StackLine(runs, Strut{Height: u(12), Baseline: u(10)})
		for _, r := range runs {
			ls.Shift(r.Valign, r.Above, r.Below)
		}
	})
}

// TestTheRunsOfAMergeGroupDoNotEachBuildTheGroup is the measure of every run
// of one merge group, which is what a word written as many spans is, and what
// word-spacing makes of a run of no-break spaces.
//
// Each run built the group's text from its two sides and then hashed it twice
// over — once to find its own memo entry, whose key holds both sides, and once
// to find the group's shaping — so a group of n runs cost n times the group.
// Planted, building the text again is a factor of 11.5 and the memo entry a
// factor of 12.5.
func TestTheRunsOfAMergeGroupDoNotEachBuildTheGroup(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	checkLinear(t, "measuring every run of one merge group", 16000, 1, func(n int) {
		br := NewBreaker(nil)
		group := strings.Repeat("ab", n)
		for i := 0; i < n; i++ {
			br.MeasureSpacedInContext(face, group[2*i:2*i+2], unit(t, 10), TextSpacing{},
				Shaping{
					MergeBefore: group[:2*i], MergeAfter: group[2*i+2:], MergeGroup: group,
					ContextKerns: true,
				})
		}
	})
}

// TestLithuanianLowercaseAsksAboutTheLetterFirst is audit C48: an "I" followed
// by a long run of below marks under lang="lt". More_Above reads forward over
// such marks, and it was asked of every character from the "I" on rather than
// of the three letters it is about. Planted: a factor of 15.5.
func TestLithuanianLowercaseAsksAboutTheLetterFirst(t *testing.T) {
	text := cached(func(n int) string { return "I" + strings.Repeat("\u0316", n) })
	checkLinear(t, "lowercasing a run of marks in Lithuanian", 4000, 4, func(n int) {
		TransformText(text(n), TransformLowercase, WordClosed, "lt")
	})
}

// TestThePreparedParagraphFindsWhatTheWalkFound holds the two ways of finding
// where the white space at the end of a line's material begins to one answer:
// the tables Lines builds once, and the walk BreakOneLine still makes for a
// caller that asks for one line. Every start, over every paragraph of up to six
// items that puts forced breaks, spaces, insets and boxes out of flow in any
// order.
func TestThePreparedParagraphFindsWhatTheWalkFound(t *testing.T) {
	kinds := []Item{
		{Text: "a"},
		{Text: " ", Space: true, TrimAtEnd: true},
		{Text: " ", Space: true, Hangs: true},
		{Text: "\u3000", Space: true},
		{Inset: true},
		{Abs: 1},
		{Forced: true},
	}
	var check func(items []Item)
	check = func(items []Item) {
		lines := NewBreaker(nil).Lines(items)
		for from := 0; from <= len(items); from++ {
			if got, want := lines.tailFrom(from), scanTailFrom(items, from); got != want {
				t.Errorf("%s from %d: the table says %d and the walk says %d",
					describeKinds(items), from, got, want)
			}
		}
		if len(items) == 6 {
			return
		}
		for _, k := range kinds {
			check(append(items[:len(items):len(items)], k))
		}
	}
	check(nil)
}

// describeKinds spells a paragraph of the kinds above, for a failure message.
func describeKinds(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		switch {
		case it.Forced:
			b.WriteString("⏎")
		case it.Inset:
			b.WriteString("|")
		case it.Abs != nil:
			b.WriteString("@")
		case it.Space && it.TrimAtEnd:
			b.WriteString("_")
		case it.Space && it.Hangs:
			b.WriteString("~")
		case it.Space:
			b.WriteString("□")
		default:
			b.WriteString(it.Text)
		}
	}
	return b.String()
}

// TestTheTablesAnswerWhatCountingAnswers holds every question a run's tables
// answer to the counting it replaced, asked the way the breaker asks it — of a
// stretch of a long run, where the tables are used — at every head, every tail
// and every single cluster of runs that mix everything the tables have to get
// right: marks with and without a base in front of them, cursive and Latin
// letters side by side, word separators, characters that draw nothing, and a
// byte that is not UTF-8.
func TestTheTablesAnswerWhatCountingAnswers(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	sp := TextSpacing{Letter: unit(t, 1), Word: unit(t, 3)}
	for _, unitText := range []string{
		"ab\u00a0cé",
		"بب\u200b\u064e\u064eبa",
		"e\u0301\u0316\u200b\u0301x",
		"\U0001F469\u200d\U0001F4BB\u00a0",
		"a\xff\u0301 \U00010100",
		"ب\u064e",
	} {
		text := strings.Repeat(unitText, 300/len(unitText)+2)
		if len(text) < tabulateFrom {
			t.Fatalf("%q is too short to be tabulated; the test would ask nothing", text)
		}
		br := NewBreaker(nil)
		for _, upright := range []bool{false, true} {
			item := Item{Text: text, Face: face, Size: unit(t, 10), Spacing: sp, Upright: upright}
			bounds := br.clustersOf(item)
			cuts := []int{0}
			for k := 0; k < bounds.len(); k++ {
				cuts = append(cuts, bounds.at(k))
			}
			cuts = append(cuts, len(text))
			cut := &RunCut{Text: text}
			check := func(from, to int) {
				t.Helper()
				piece := text[from:to]
				stretch := item
				stretch.Text, stretch.Cut, stretch.CutAt = piece, cut, from
				if got, want := br.spacingIn(stretch, 0, len(piece), piece), SpacingAdvance(piece, sp); got != want {
					t.Errorf("%q[%d:%d]: the tables give a spacing of %v and counting %v",
						text, from, to, got, want)
				}
				if got, want := br.trailingSpacing(stretch), TrailingSpacing(stretch); got != want {
					t.Errorf("%q[%d:%d]: the tables give a trailing spacing of %v and "+
						"counting %v", text, from, to, got, want)
				}
				if upright {
					if got, ok := br.uprightIn(stretch, 0, len(piece), piece); !ok ||
						got != UprightUnits(piece) {
						t.Errorf("%q[%d:%d]: the tables give %d upright units (%v) and "+
							"counting %d", text, from, to, got, ok, UprightUnits(piece))
					}
				}
			}
			for i, c := range cuts {
				check(0, c)
				check(c, len(text))
				if i+1 < len(cuts) {
					check(c, cuts[i+1])
				}
				if got, want := br.runesBefore(item, c), len([]rune(text[:c])); got != want {
					t.Errorf("%q[:%d]: the table counts %d characters and there are %d",
						text, c, got, want)
				}
			}
		}
	}
}
