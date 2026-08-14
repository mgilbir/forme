package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Balancing, and balancing beside a float.
//
// CSS Text §5.1 gives no algorithm for "text-wrap-style: balance" and states the
// goal: the empty space left on each line should be as even as it can be made.
// The search that implements it is a binary hunt for the narrowest width that
// still takes the same number of lines, so what is worth asserting is the goal
// and the search's own contract — not the widths it happens to try.
//
// The banded half is the same search where a float has shortened some of the
// lines, and it had no test of any kind: nothing in this repository, reftests
// included, had ever executed it. A float is not needed to reach it. A band is a
// width for the nth line and nothing more, so a paragraph beside a float and a
// paragraph handed [60, 60, 100, 100] are the same problem, and the second can be
// written down.

// bandSets are the shapes a run of floats leaves behind. Each is a width per
// line, shortest-lived first.
var bandSets = []struct {
	name  string
	bands []style.Unit
}{
	{"no float at all", nil},
	{"one wide band", []style.Unit{u(200)}},
	{"a float at the top", []style.Unit{u(60), u(60), u(200), u(200)}},
	{"a float at the bottom", []style.Unit{u(200), u(200), u(60), u(60)}},
	{"a float beside every line", []style.Unit{u(80)}},
	{"bands that run out before the lines do", []style.Unit{u(200), u(60)}},
	{"a band narrower than one word", []style.Unit{u(6), u(200), u(200)}},
}

// balanceTexts are the paragraphs worth balancing: between two and six lines at
// the widths below, which is the range the engine will balance at all.
var balanceTexts = []string{
	"the quick brown fox jumps over the lazy dog",
	"a b c d e f g h i j k l m n o p",
	"one two three four five six seven eight",
	"supercalifragilisticexpialidocious and a few short words after it",
	"日本語のテキストです and some English too",
}

// TestBandedCountingAgreesWithPlainCountingWhenTheBandsAreUniform is the
// differential the banded code never had.
//
// Counting lines in bands and counting them at one width are two implementations
// of one question, and they are only ever both right or both wrong. Where every
// band is the width being probed, the second is the first with the floats taken
// away, so their answers must be identical for every paragraph and every width —
// and any divergence is a fault in the banded one, since the plain one is
// exercised by every balanced box in the suite.
func TestBandedCountingAgreesWithPlainCountingWhenTheBandsAreUniform(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{30, 60, 100, 200, 400} {
			for _, indent := range []float64{0, 24} {
				plain := br.countLines(items, u(width), u(indent), 99)
				banded := br.countLinesInBands(items, []style.Unit{u(width)}, u(width), u(indent), 99)
				if plain != banded {
					t.Errorf("%q at %gpx with a %gpx indent: counted %d lines plainly and "+
						"%d in one uniform band — the two must agree where there is no "+
						"float", text, width, indent, plain, banded)
				}
			}
		}
	}
}

// TestBandedBalancingAgreesWithPlainBalancingWhenTheBandsAreUniform is the same
// differential one level up.
func TestBandedBalancingAgreesWithPlainBalancingWhenTheBandsAreUniform(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{60, 100, 200, 400} {
			plain := br.BalanceWidth(items, u(width), 0)
			banded := br.BalanceWidthInBands(items, []style.Unit{u(width)}, u(width), 0)
			if plain != banded {
				t.Errorf("%q at %gpx: balanced to %v plainly and %v in one uniform band",
					text, width, plain, banded)
			}
		}
	}
}

// TestABalancedWidthNeverWidensTheBoxAndNeverAddsALine is the contract the
// caller relies on.
//
// The cap comes back as a width to lay the box out at again. One wider than the
// box would be ignored, and one that took an extra line would have made the
// paragraph taller in order to make it tidier — which is not a trade §5.1 offers.
func TestABalancedWidthNeverWidensTheBoxAndNeverAddsALine(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			for _, width := range []float64{60, 100, 200, 400} {
				cap := br.BalanceWidthInBands(items, bs.bands, u(width), 0)
				if cap == style.MaxUnit {
					// Refused: fewer than two lines, or more than the engine
					// balances. Either way it is not a cap.
					continue
				}
				if cap > u(width) {
					t.Errorf("%q, %s, %gpx: balanced to %gpx, which is wider than the box",
						text, bs.name, width, cap.Px())
				}
				full := br.countLinesInBands(items, bs.bands, u(width), 0, MaxBalanceLines+1)
				capped := br.countLinesInBands(items, bs.bands, cap, 0, MaxBalanceLines+2)
				if capped > full {
					t.Errorf("%q, %s, %gpx: the box takes %d lines and the balanced "+
						"width %gpx takes %d — balancing may even the lines and may not "+
						"add one", text, bs.name, width, full, cap.Px(), capped)
				}
			}
		}
	}
}

// TestABalancedWidthIsTheNarrowestThatHolds is the search's own contract: it
// returns a width, and one unit narrower would have cost a line.
//
// Without it every assertion above is satisfied by returning the box's own
// width, which adds no line, never widens the box, and balances nothing.
func TestABalancedWidthIsTheNarrowestThatHolds(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, bs := range bandSets {
			for _, width := range []float64{100, 200, 400} {
				cap := br.BalanceWidthInBands(items, bs.bands, u(width), 0)
				if cap == style.MaxUnit || cap <= 2 {
					continue
				}
				full := br.countLinesInBands(items, bs.bands, u(width), 0, MaxBalanceLines+1)
				narrower := br.countLinesInBands(items, bs.bands, cap.Sub(1), 0, MaxBalanceLines+2)
				if narrower <= full {
					t.Errorf("%q, %s, %gpx: balanced to %gpx, and one unit narrower still "+
						"takes %d lines against the box's %d — the search stopped before "+
						"the narrowest width that holds",
						text, bs.name, width, cap.Px(), narrower, full)
				}
			}
		}
	}
}

// TestBalancingNeverLengthensTheLongestLine is what balancing is *for*, as
// distinct from what the search does.
//
// §5.1 asks for the empty space on each line to be evened out, and the search
// reaches that by narrowing until one more unit would cost a line — at which
// point the longest line is as short as it can be without adding one. So the
// longest line after balancing must be no longer than before. A search that
// returned something arbitrary could satisfy every count-based assertion above
// and still set the paragraph worse than it found it.
func TestBalancingNeverLengthensTheLongestLine(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{100, 200, 400} {
			cap := br.BalanceWidth(items, u(width), 0)
			if cap == style.MaxUnit {
				continue
			}
			before := longestLine(t, br, items, u(width))
			after := longestLine(t, br, items, cap)
			if after > before {
				t.Errorf("%q at %gpx: the longest line was %gpx and balancing to %gpx "+
					"made it %gpx — the ragged edge got worse",
					text, width, before.Px(), cap.Px(), after.Px())
			}
		}
	}
}

// longestLine is the widest line the items break to at a measure, which is the
// number §5.1's "balance" is trying to reduce.
func longestLine(t *testing.T, br *Breaker, items []Item, width style.Unit) style.Unit {
	t.Helper()
	var most style.Unit
	from, fromByte := 0, 0
	for from < len(items) {
		line, next, nextByte, _, _ := br.BreakOneLine(items, from, fromByte, width, 0)
		if w := lineWidth(line); w > most {
			most = w
		}
		if !CursorAdvanced(from, fromByte, next, nextByte) {
			break
		}
		from, fromByte = next, nextByte
	}
	return most
}

// TestANarrowerBandNeverNeedsFewerLines is the banded counting's own
// monotonicity, and the property the search rests on.
//
// The binary hunt assumes the line count falls as the probe widens; if it did
// not, halving the bracket would settle on noise. Bands are the other axis of
// the same assumption: shortening the room a line has cannot make the paragraph
// shorter.
func TestANarrowerBandNeverNeedsFewerLines(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{100, 200, 400} {
			wide := []style.Unit{u(200), u(200), u(200), u(200)}
			narrow := []style.Unit{u(120), u(120), u(120), u(120)}
			nWide := br.countLinesInBands(items, wide, u(width), 0, 99)
			nNarrow := br.countLinesInBands(items, narrow, u(width), 0, 99)
			if nNarrow < nWide {
				t.Errorf("%q at %gpx: %d lines in 200px bands and %d in 120px ones — "+
					"less room cannot need fewer lines", text, width, nWide, nNarrow)
			}
		}
	}
}

// TestANarrowUniformBandActsExactlyLikeANarrowWidth is the assertion that ties
// the bands to something outside themselves.
//
// Everything else here compares one banded count against another, and a banded
// count that ignored its bands entirely would agree with itself perfectly — that
// is not a hypothetical, it is a defect that survived the first version of this
// file. A single band narrower than the width being probed leaves every line
// exactly that much room, which is the same paragraph as one laid out at that
// width with no float at all. So the plain counter, which every balanced box in
// the suite exercises, is the oracle: the two must give the same number.
func TestANarrowUniformBandActsExactlyLikeANarrowWidth(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, band := range []float64{30, 60, 100} {
			for _, width := range []float64{200, 400} {
				// The band is the narrower of the two, so it is what every line gets.
				banded := br.countLinesInBands(items, []style.Unit{u(band)}, u(width), 0, 99)
				plain := br.countLines(items, u(band), 0, 99)
				if banded != plain {
					t.Errorf("%q: %d lines in a %gpx band probed at %gpx, and %d lines "+
						"at a plain %gpx — a band narrower than the probe is the line's "+
						"room, so the two are the same paragraph",
						text, banded, band, width, plain, band)
				}
			}
		}
	}
}

// TestBandsRunningOutRepeatTheLastOne is the other half of the same worry.
//
// A probe may make more lines than the layout that produced the bands did, and
// the answer for those is the last band recorded — the room below the lowest
// float, which is what every line after it had. Writing that band out explicitly
// must change nothing, and a lookup that ran off the end into "no limit at all"
// would break the equality.
func TestBandsRunningOutRepeatTheLastOne(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		short := []style.Unit{u(200), u(60)}
		long := []style.Unit{u(200), u(60), u(60), u(60), u(60), u(60), u(60), u(60)}
		for _, width := range []float64{100, 200, 400} {
			a := br.countLinesInBands(items, short, u(width), 0, 99)
			b := br.countLinesInBands(items, long, u(width), 0, 99)
			if a != b {
				t.Errorf("%q at %gpx: %d lines with bands %v and %d with the last one "+
					"written out — a band list that runs out repeats its last entry",
					text, width, a, short, b)
			}
		}
	}
}

// TestTheNthLineGetsTheNthBand pins the one thing about the bands that the
// equalities above cannot see: which line gets which.
//
// Comparing two banded counts is blind to a lookup that is off by one, because
// both sides shift together. So this builds the answer out of pieces that are
// not the banded counter at all — break the first line at the first band, then
// count what is left at the second — and requires the banded counter to agree.
func TestTheNthLineGetsTheNthBand(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// A float beside the first line only: 60px there, 200px below it.
	bands := []style.Unit{u(60), u(200)}
	for _, text := range balanceTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})

		// The first line, in the first band.
		first, next, nextByte, _, forced := br.BreakOneLine(items, 0, 0, u(60), 0)
		if nextByte != 0 {
			// It ended inside a word, so "what is left" is not a suffix of the
			// items and the oracle cannot be assembled by hand.
			continue
		}
		want := 0
		if len(first) > 0 || forced {
			want = 1
		}
		// And everything after it, in the second.
		want += br.countLines(items[next:], u(200), 0, 99)

		got := br.countLinesInBands(items, bands, u(400), 0, 99)
		if got != want {
			t.Errorf("%q with bands %v: the banded count says %d lines, and breaking "+
				"the first line at 60px and the rest at 200px gives %d — the nth line "+
				"must get the nth band", text, bands, got, want)
		}
	}
}

// TestTheIndentIsTakenOffTheFirstLineInBandsToo keeps the equality above honest
// about the indent.
//
// The plain and banded counters both take §16.1's indent off the first line, and
// comparing them proves nothing about it unless the indent changes an answer
// somewhere — which for most paragraphs at most widths it does not. So the
// fixture is chosen to sit on the boundary, and the test first checks that it
// really does: an indent that moved no line would make this vacuous, and it
// would look exactly like a pass.
func TestTheIndentIsTakenOffTheFirstLineInBandsToo(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// "the quick" is 108px. In 120px it fits; with a 24px indent the first line
	// has 96px and it does not, so the indent is worth a line.
	items := itemsOf(t, br, face, "the quick brown fox",
		WhiteSpaceOf("collapse"), OverflowWrap{})
	const width, indent = 120.0, 24.0

	plainNoIndent := br.countLines(items, u(width), 0, 99)
	plainIndented := br.countLines(items, u(width), u(indent), 99)
	if plainIndented <= plainNoIndent {
		t.Fatalf("the fixture is dead: %gpx of text takes %d lines with no indent and "+
			"%d with a %gpx one, so the indent decides nothing here and the comparison "+
			"below would hold however the indent were handled",
			width, plainNoIndent, plainIndented, indent)
	}
	banded := br.countLinesInBands(items, []style.Unit{u(width)}, u(width), u(indent), 99)
	if banded != plainIndented {
		t.Errorf("with a %gpx indent: %d lines plainly and %d in one uniform band — "+
			"§16.1's indent comes off the first line either way",
			indent, plainIndented, banded)
	}
}
