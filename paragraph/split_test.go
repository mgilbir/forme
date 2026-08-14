package paragraph

import (
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/style"
)

// Cutting one item in two, and finding the next tab stop.

// TestSplittingAnItemKeepsItsTextAndItsRange is the arithmetic a real bug in this
// engine turned on.
//
//	Move a split word's bidi range by runes, and not by bytes
//
// An item carries the span of the paragraph buffer its characters occupy, and
// the levels UAX #9 resolved are indexed by that span. The offset a line breaks
// at is a *byte* offset into the text; the span counts *runes*. Adding one to the
// other is right for Latin and wrong for everything that needs the algorithm at
// all — a Hebrew letter is two bytes, so a word cut in half moved its range twice
// as far as its text, and the tail read its level from two characters further on.
//
// What that looked like was a number drawn on the wrong side of the word it
// belonged to, on the line the tail began, while the same text unbroken ordered
// correctly. Nothing about the split *text* was wrong, which is why the text
// assertions below are not enough on their own.
func TestSplittingAnItemKeepsItsTextAndItsRange(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		if tc.text == "" {
			continue
		}
		item := Item{
			Text: tc.text, Face: face, Size: u(size20),
			BidiPara: 1, BidiStart: 7, BidiEnd: 7 + utf8.RuneCountInString(tc.text),
			Width: br.MeasureSpaced(face, tc.text, u(size20), TextSpacing{}),
		}
		// Every rune boundary is a place the breaking may cut, so every one is
		// checked rather than a chosen few.
		for at := range tc.text {
			head, tail := br.SplitItem(item, at)

			if head.Text+tail.Text != item.Text {
				t.Fatalf("%s cut at %d: the halves spell %q and %q, which is not %q",
					tc.name, at, head.Text, tail.Text, item.Text)
			}
			if head.BidiStart != item.BidiStart {
				t.Errorf("%s cut at %d: the head starts at %d of the paragraph and the "+
					"whole item started at %d", tc.name, at, head.BidiStart, item.BidiStart)
			}
			if tail.BidiEnd != item.BidiEnd {
				t.Errorf("%s cut at %d: the tail ends at %d and the whole item ended at %d",
					tc.name, at, tail.BidiEnd, item.BidiEnd)
			}
			if head.BidiEnd != tail.BidiStart {
				t.Errorf("%s cut at %d: the head ends at %d and the tail starts at %d — "+
					"the two halves must meet, or the characters between them have no "+
					"level and the ones after them have the wrong one",
					tc.name, at, head.BidiEnd, tail.BidiStart)
			}
			// The one that matters: the range moves by characters, not by bytes.
			if want := item.BidiStart + utf8.RuneCountInString(head.Text); tail.BidiStart != want {
				t.Errorf("%s cut at %d: the tail's range starts at %d and its text starts "+
					"%d characters into the item, so it should be %d — the range was "+
					"moved by %d bytes instead",
					tc.name, at, tail.BidiStart, utf8.RuneCountInString(head.Text), want, at)
			}
			// And the two halves between them still span exactly the item.
			if got := (head.BidiEnd - head.BidiStart) + (tail.BidiEnd - tail.BidiStart); got !=
				item.BidiEnd-item.BidiStart {
				t.Errorf("%s cut at %d: the halves span %d characters and the item spans %d",
					tc.name, at, got, item.BidiEnd-item.BidiStart)
			}
		}
	}
}

// TestCuttingTwiceIsCuttingOnce is the property that makes a line ending inside a
// word safe to resume from.
//
// The breaker cuts an item, sets the head, and comes back to the tail — which it
// may cut again for the next line. So a word broken across three lines is one
// item cut twice, and the second cut is measured from the tail's own start rather
// than from the original item's. If the two disagreed, the third line of a long
// word would be right about its text and wrong about everything else, and only a
// word that broke twice would show it.
func TestCuttingTwiceIsCuttingOnce(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		if utf8.RuneCountInString(tc.text) < 3 {
			continue
		}
		item := Item{
			Text: tc.text, Face: face, Size: u(size20),
			BidiPara: 1, BidiStart: 3, BidiEnd: 3 + utf8.RuneCountInString(tc.text),
			Width: br.MeasureSpaced(face, tc.text, u(size20), TextSpacing{}),
		}
		for a := range tc.text {
			if a == 0 {
				continue
			}
			_, tail := br.SplitItem(item, a)
			for b := range tail.Text {
				if b == 0 {
					continue
				}
				// Cut the tail at b, and cut the original at the same place.
				midOfTail, endOfTail := br.SplitItem(tail, b)
				headDirect, tailDirect := br.SplitItem(item, a+b)

				if midOfTail.Text != headDirect.Text[a:] {
					t.Fatalf("%s cut at %d then %d: the middle reads %q, and cutting the "+
						"whole item at %d leaves %q before the same point",
						tc.name, a, b, midOfTail.Text, a+b, headDirect.Text[a:])
				}
				if endOfTail.Text != tailDirect.Text {
					t.Fatalf("%s cut at %d then %d: the end reads %q against %q",
						tc.name, a, b, endOfTail.Text, tailDirect.Text)
				}
				if endOfTail.BidiStart != tailDirect.BidiStart {
					t.Errorf("%s cut at %d then %d: the end's range starts at %d, and "+
						"cutting the whole item at %d starts it at %d — cutting twice "+
						"must land where cutting once does",
						tc.name, a, b, endOfTail.BidiStart, a+b, tailDirect.BidiStart)
				}
				if endOfTail.Width != tailDirect.Width {
					t.Errorf("%s cut at %d then %d: the end is %gpx and the same text cut "+
						"directly is %gpx", tc.name, a, b,
						endOfTail.Width.Px(), tailDirect.Width.Px())
				}
			}
		}
	}
}

// TestATailBeginsALineAndSoTakesNoOpportunityFromWhatWasBeforeIt is the small
// piece of bookkeeping the split also does.
func TestATailBeginsALineAndSoTakesNoOpportunityFromWhatWasBeforeIt(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	item := Item{
		Text: "abcdef", Face: face, Size: u(size20), BreakBefore: true,
		Width: br.MeasureSpaced(face, "abcdef", u(size20), TextSpacing{}),
	}
	head, tail := br.SplitItem(item, 3)
	if !head.BreakBefore {
		t.Error("the head lost the opportunity the item carried; it is still where the " +
			"item was")
	}
	if tail.BreakBefore {
		t.Error("the tail kept an opportunity to break before it — there is nothing in " +
			"front of it any more, because it begins a line")
	}
}

// TestATabAlwaysLandsOnAStop is what tab stops are for.
//
// §4.1.2 puts them at multiples of the tab size from the content edge, and the
// whole value of that is that two lines of text written with the same tabs line
// up. So the position after a tab must be a multiple of the size — exactly, not
// nearly. The arithmetic is fixed-point for this reason: a stop computed in
// floating point drifts along a line of tabs until two columns that should align
// do not, by a fraction of a pixel that no test of one tab would see.
func TestATabAlwaysLandsOnAStop(t *testing.T) {
	for _, stop := range []float64{8, 12, 32, 0.5, 100} {
		for _, floor := range []float64{0, 1, 4} {
			for x := 0.0; x < 200; x += 0.25 {
				d := TabAdvance(u(x), u(stop), u(floor))
				if d <= 0 {
					t.Fatalf("a tab at %gpx with %gpx stops advanced %gpx — it must move "+
						"the pen forward", x, stop, d.Px())
				}
				if landed := u(x).Add(d); landed%u(stop) != 0 {
					t.Errorf("a tab at %gpx with %gpx stops landed at %gpx, which is not a "+
						"stop — the columns after it would not line up",
						x, stop, landed.Px())
				}
				// At most one whole stop, plus another where the floor sent it past
				// the nearest one.
				if d > u(stop*2) {
					t.Errorf("a tab at %gpx with %gpx stops advanced %gpx, more than two "+
						"stops", x, stop, d.Px())
				}
			}
		}
	}
}

// TestATinyShiftIsSentToTheStopAfterTheNearest is §4.1.2's threshold, stated as
// the rule says it rather than as it is tempting to read it.
//
//	if this distance is less than 0.5ch, then the subsequent tab stop is used
//	instead
//
// The subsequent stop — one of them. So the guarantee is not "the advance is at
// least half a character"; it is "the nearest stop is skipped". The two are the
// same wherever the threshold is smaller than the tab size, which is every
// document that has not set "tab-size" to a fraction of a character, and they
// part where it is not: a 0.5px tab size with a 1px threshold still advances less
// than the threshold after skipping a stop, and one skip is what was asked for.
//
// Asserting the advance against the threshold would have been the easy reading,
// and it fails on that case for a tab that is behaving exactly as specified.
func TestATinyShiftIsSentToTheStopAfterTheNearest(t *testing.T) {
	// Realistic numbers: an eight-character stop at 20px Courier is 96px, and
	// the threshold is half a character, 6px.
	const stop, floor = 96.0, 6.0
	for _, tc := range []struct {
		name string
		x    float64
		want float64
	}{
		{"well before a stop", 10, 86},
		{"exactly on a stop", 96, 96},
		{"a hair past a stop", 97, 95},
		{"just inside the threshold", 91, 5 + stop},
		{"one unit inside the threshold", 90.5, 5.5 + stop},
		{"just outside the threshold", 90, 6},
	} {
		got := TabAdvance(u(tc.x), u(stop), u(floor))
		if got != u(tc.want) {
			t.Errorf("%s: a tab at %gpx advanced %gpx, want %g",
				tc.name, tc.x, got.Px(), tc.want)
		}
		if landed := u(tc.x).Add(got); landed%u(stop) != 0 {
			t.Errorf("%s: it landed at %gpx, which is not a stop", tc.name, landed.Px())
		}
	}
}

// TestATabSizeOfZeroRendersNoTab is §4.1.2's one way to ask for a tab that takes
// no room, and the only case where the advance may be nothing.
func TestATabSizeOfZeroRendersNoTab(t *testing.T) {
	for _, x := range []float64{0, 1, 37.5, 1000} {
		if d := TabAdvance(u(x), 0, u(4)); d != 0 {
			t.Errorf("a tab at %gpx with a zero tab size advanced %gpx, want 0", x, d.Px())
		}
	}
}

// TestTabsOnOneLineKeepTheirColumns is the property a single tab cannot show.
//
// Every tab on a line is measured from the same origin, so a run of them lands on
// a run of stops — and the columns are the point. This walks a line of alternating
// text and tabs and requires each tab to leave the pen on a stop, which is what
// a table laid out with tabs depends on and what fixed-point arithmetic is here
// to guarantee.
func TestTabsOnOneLineKeepTheirColumns(t *testing.T) {
	const stop = 32.0
	for _, widths := range [][]float64{
		{1, 1, 1, 1, 1, 1},
		{5, 40, 3, 70, 31, 33},
		{31.75, 0.25, 63.5, 0.5},
	} {
		x := style.Unit(0)
		for i, w := range widths {
			x = x.Add(u(w))
			x = x.Add(TabAdvance(x, u(stop), 0))
			if x%u(stop) != 0 {
				t.Fatalf("after %d columns of %v the pen is at %gpx, which is not a "+
					"multiple of the %gpx stop", i+1, widths, x.Px(), stop)
			}
		}
	}
}

// TestATabNeverGoesBackwards guards the one shape of arithmetic slip that would
// be catastrophic and quiet: a modulus taken of a negative number.
func TestATabNeverGoesBackwards(t *testing.T) {
	for _, x := range []float64{-1, -0.25, -1000} {
		d := TabAdvance(u(x), u(8), 0)
		if d <= 0 {
			t.Errorf("a tab at %gpx advanced %gpx — a pen position before the content "+
				"edge is clamped, not negated", x, d.Px())
		}
	}
}
