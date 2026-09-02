package paragraph

import (
	"math"
	"reflect"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The scored balancer, and the promise every caller of this package relies on
// without ever stating it: breaking a paragraph does not change it.

// TestBreakingNeverWritesBackToItsItems is a contract the code depends on in
// writing and nothing checked.
//
//	The cursor is an index *and* an offset rather than a rewritten items slice:
//	the caller re-runs this over several band widths, so anything written back
//	would be seen by the next attempt and the split would compound.
//
// The balancer lays the same paragraph out a dozen times over, at widths it is
// choosing between, and every pass but the last is thrown away. A pass that left
// a mark would make the next one measure a paragraph that had been cut once
// already — and the marks would accumulate, so the fault would grow with the
// number of candidate widths rather than appearing at any one of them. That is
// the shape of bug this is here to make impossible.
func TestBreakingNeverWritesBackToItsItems(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, w := range whiteSpaces {
			items := itemsOf(t, br, face, tc.text, w.ws, OverflowWrap{BreakWord: true})
			before := append([]Item(nil), items...)

			// Everything a caller does with a set of items, in the order the
			// balancer does it: count, search for a cap, and break.
			for _, width := range widths {
				br.countLines(items, u(width), 0, 99)
				br.BalanceWidth(items, u(width), 0)
				br.BalanceWidthInBands(items, []style.Unit{u(60), u(200)}, u(width), 0)
				br.BalanceScoredCaps(items, []style.Unit{u(60), u(200)}, 0, 3)
				from, fromByte := 0, 0
				for from < len(items) {
					_, next, nextByte, _, _ := br.BreakOneLine(items, from, fromByte, u(width), 0)
					if !CursorAdvanced(from, fromByte, next, nextByte) {
						break
					}
					from, fromByte = next, nextByte
				}
			}

			if len(items) != len(before) {
				t.Fatalf("%s under %s: the item slice was %d long and is now %d",
					tc.name, w.name, len(before), len(items))
			}
			if len(items) > 0 && !reflect.DeepEqual(items, before) {
				for i := range items {
					if !reflect.DeepEqual(items[i], before[i]) {
						t.Fatalf("%s under %s: item %d was %+v and is now %+v — laying a "+
							"paragraph out changed the paragraph",
							tc.name, w.name, i, before[i], items[i])
					}
				}
			}
		}
	}
}

// linesWithCaps breaks a paragraph the way a caller does once the scored search
// has named a width for each line: the room is the narrower of the band the line
// sits in and the cap chosen for it.
func linesWithCaps(t *testing.T, br *Breaker, items []Item, bands, caps []style.Unit,
	indent style.Unit) (lines [][]Item, rooms []style.Unit) {

	t.Helper()
	from, fromByte := 0, 0
	for from < len(items) {
		n := len(lines)
		room := bandAt(bands, n)
		if n < len(caps) && caps[n] < room {
			room = caps[n]
		}
		if n == 0 {
			room = room.Sub(indent)
		}
		line, next, nextByte, _, _ := br.BreakOneLine(items, from, fromByte, room, 0)
		lines = append(lines, line)
		rooms = append(rooms, room)
		if !CursorAdvanced(from, fromByte, next, nextByte) {
			break
		}
		from, fromByte = next, nextByte
		if len(lines) > maxLines(items) {
			t.Fatalf("breaking with caps %v did not terminate", caps)
		}
	}
	return lines, rooms
}

// raggedness is what §5.1's balancing minimises, computed here from the finished
// lines rather than taken from the code that chooses them.
//
// The sum of the squares of the space left over on each line. The square is what
// makes it balance rather than merely fit: one line left half empty costs far
// more than two left a quarter empty each, which is the difference a reader sees.
func raggedness(lines [][]Item, rooms []style.Unit) float64 {
	var total float64
	for i, line := range lines {
		left := rooms[i].Sub(lineWidth(line)).Px()
		if left < 0 {
			left = 0
		}
		total += left * left
	}
	return total
}

// TestTheScoredCapsAreNoWorseThanBreakingGreedily is the scored search held to
// its own purpose rather than to its method.
//
// It exists because greedy filling in a narrower measure cannot reach every
// arrangement once a float has made the lines different lengths — the suite's
// text-wrap-balance-float-001 is three lines whose reference no narrower measure
// produces. So the search chooses between break sets, and what it claims is that
// the set it picks leaves the space more evenly distributed.
//
// The score is recomputed here from the lines that come out, by the definition in
// the function's own comment, and compared against filling the same bands
// greedily. A search that returned an arbitrary set of caps would pass every
// count-based check and fail this one.
func TestTheScoredCapsAreNoWorseThanBreakingGreedily(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			if len(bs.bands) == 0 {
				continue
			}
			for _, lines := range []int{2, 3, 4} {
				caps := br.BalanceScoredCaps(items, bs.bands, 0, lines)
				if caps == nil {
					continue // nothing to choose, or too much
				}
				got, rooms := linesWithCaps(t, br, items, bs.bands, caps, 0)
				if len(got) != lines {
					t.Errorf("%q, %s: the caps for %d lines produced %d",
						text, bs.name, lines, len(got))
					continue
				}
				greedy, greedyRooms := linesWithCaps(t, br, items, bs.bands, nil, 0)
				if len(greedy) != lines {
					// Greedy makes a different number of lines, so the two are not
					// arrangements of the same paragraph and the scores are not
					// comparable.
					continue
				}
				if a, b := raggedness(got, rooms), raggedness(greedy, greedyRooms); a > b {
					t.Errorf("%q, %s, %d lines: the scored caps leave a raggedness of "+
						"%g and breaking greedily leaves %g — the search chose the "+
						"worse of the two", text, bs.name, lines, a, b)
				}
			}
		}
	}
}

// TestTheScoredCapsNeverChangeTheLineCount is the constraint the score is
// minimised under.
//
// Balancing may not cost a line: a paragraph that grew one to look tidier would
// be balancing at the expense of the thing being balanced. The count is therefore
// a constraint rather than a term in the score, and it is worth asserting
// separately because a search that traded a line for a better score would look
// like a *better* result by every measure except this one.
func TestTheScoredCapsNeverChangeTheLineCount(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			// The count is a precondition: layout passes the number of lines the
			// greedy layout came to, never a number picked out of the air. No width
			// reaches this search — it works inside the bands — so the count is
			// taken there too, with nothing capping them.
			want, _ := br.countLinesInBands(items, bs.bands, style.MaxUnit, 0, MaxBalanceLines+1)
			{
				caps := br.BalanceScoredCaps(items, bs.bands, 0, want)
				if caps == nil {
					continue
				}
				if len(caps) != want {
					t.Errorf("%q, %s: asked for %d lines and got %d caps",
						text, bs.name, want, len(caps))
					continue
				}
				got, _ := linesWithCaps(t, br, items, bs.bands, caps, 0)
				if len(got) != want {
					t.Errorf("%q, %s: the caps for %d lines break to %d — balancing may "+
						"even the lines and may not add or drop one",
						text, bs.name, want, len(got))
				}
			}
		}
	}
}

// TestBreakingWithCapsNeverOverflowsAvoidably holds the scored path to the same
// rule as the plain one.
//
// A cap is a narrower width to break one line at, and every reason a line may
// overflow is unchanged by having been given one: a word wider than the room
// still goes somewhere. What must not happen is a line pushed past its room while
// holding an opportunity that would have saved it — the arrangement the search
// chose has to be one the page can actually set.
//
// A cap *wider* than its band is deliberately not a fault: the caller takes the
// narrower of the two, so such a cap means "nothing to say about this line".
func TestBreakingWithCapsNeverOverflowsAvoidably(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			if len(bs.bands) == 0 {
				continue
			}
			for _, want := range []int{2, 3, 4} {
				caps := br.BalanceScoredCaps(items, bs.bands, 0, want)
				if caps == nil {
					continue
				}
				lines, rooms := linesWithCaps(t, br, items, bs.bands, caps, 0)
				for n, line := range lines {
					if lineWidth(line) <= rooms[n] {
						continue
					}
					var prefix style.Unit
					for k, it := range line {
						if k > 0 && it.BreakBefore && !it.NoWrap && prefix <= rooms[n] {
							t.Errorf("%q, %s, %d lines: line %d has %gpx of room and "+
								"spends %gpx, and breaking before its item %d would have "+
								"left %gpx — the arrangement the search chose overflows "+
								"where it need not",
								text, bs.name, want, n, rooms[n].Px(),
								lineWidth(line).Px(), k, prefix.Px())
							break
						}
						if !it.Hangs {
							prefix = prefix.Add(it.Width)
						}
					}
				}
			}
		}
	}
}

// usedWidth is the width the scored search charges a line: every run on it,
// hanging ones included.
//
// It differs from lineWidth, which leaves a hanging space out because §4.1.2
// excludes one "for fit". Fitting and scoring are different questions: a line
// whose trailing space hangs past the measure still fits, and the space is still
// room the arrangement spent. This is the number the search's own comment is
// written about, so it is the number an oracle for the search has to use.
func usedWidth(line []Item) style.Unit {
	var used style.Unit
	for _, it := range line {
		used = used.Add(it.Width)
	}
	return used
}

// arrangementScore is §5.1's objective over a finished set of lines: the sum of
// the squares of the space left over in each line's room.
//
// The slack is not clamped at zero. A line set wider than its room has spent more
// than it had, and squaring a negative charges it exactly as much as leaving the
// same amount empty — which is what makes the search prefer an arrangement that
// fits over one that overflows by the same margin.
func arrangementScore(lines [][]Item, bands []style.Unit) float64 {
	var total float64
	for n, line := range lines {
		slack := bandAt(bands, n).Sub(usedWidth(line)).Px()
		total += slack * slack
	}
	return total
}

// everyArrangement calls yield with each way of cutting the items into exactly
// n lines at break opportunities.
//
// Any such cut is one the breaker can be made to produce: asked for a width equal
// to that prefix's own, a greedy break stops exactly there. So this enumerates
// the arrangements the search is choosing between, without knowing anything about
// how it chooses.
func everyArrangement(items []Item, n int, yield func(lines [][]Item)) {
	var opps []int
	for i := 1; i < len(items); i++ {
		if items[i].BreakBefore && !items[i].NoWrap {
			opps = append(opps, i)
		}
	}
	cuts := make([]int, 0, n-1)
	var choose func(start, need int)
	choose = func(start, need int) {
		if need == 0 {
			lines := make([][]Item, 0, n)
			prev := 0
			for _, c := range cuts {
				lines = append(lines, items[prev:c])
				prev = c
			}
			lines = append(lines, items[prev:])
			// The breaker trims what §4.1.2 removes from a line's edge before it
			// reports the line, and the search scores what the breaker reported. An
			// arrangement scored with the untrimmed slices is charged for spaces no
			// line ever spent.
			trimmed := make([][]Item, len(lines))
			for i, l := range lines {
				trimmed[i] = trimLineEdge(l)
			}
			yield(trimmed)
			return
		}
		for i := start; i < len(opps); i++ {
			cuts = append(cuts, opps[i])
			choose(i+1, need-1)
			cuts = cuts[:len(cuts)-1]
		}
	}
	choose(0, n-1)
}

// TestTheScoredSearchFindsTheBestArrangement is the search held against an
// exhaustive one.
//
// Comparing it to greedy filling, as the test above does, is too weak to catch a
// search that picks the *worst* of its candidates: greedy is a different
// arrangement that is often worse than all of them, so "no worse than greedy"
// leaves the choice among the candidates unconstrained. Both of those defects
// were planted and both survived that comparison.
//
// This tries every arrangement into the same number of lines and requires the
// search to have found one that scores as well as the best. The paragraphs are
// small on purpose — the enumeration is combinatorial, and a dozen opportunities
// is enough to distinguish a search that minimises from one that does not.
func TestTheScoredSearchFindsTheBestArrangement(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range []string{
		"one two three four five six",
		"a bb ccc dddd eeeee",
		"alpha beta gamma delta",
		"short longerword mid x",
	} {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			if len(bs.bands) == 0 {
				continue
			}
			for _, lines := range []int{2, 3} {
				caps := br.BalanceScoredCaps(items, bs.bands, 0, lines)
				if caps == nil {
					continue
				}
				got, _ := linesWithCaps(t, br, items, bs.bands, caps, 0)
				if len(got) != lines {
					continue
				}
				chosen := arrangementScore(got, bs.bands)

				best, found := math.Inf(1), false
				everyArrangement(items, lines, func(cand [][]Item) {
					if s := arrangementScore(cand, bs.bands); s < best {
						best, found = s, true
					}
				})
				if !found {
					continue
				}
				if chosen > best+1e-6 {
					t.Errorf("%q, %s, %d lines: the search chose an arrangement scoring "+
						"%g and the best of the %d it was choosing between scores %g — "+
						"the search is not minimising",
						text, bs.name, lines, chosen, lines, best)
				}
			}
		}
	}
}
