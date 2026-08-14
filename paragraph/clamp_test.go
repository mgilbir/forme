package paragraph

import (
	"testing"
)

// Balancing a block that CSS Overflow 4's clamp has already cut off.
//
// The ordinary balancer evens the lines of a paragraph that is shown whole. This
// one evens the lines of a paragraph the reader will only see the first few of,
// and the two are different problems: what is being preserved is not the number
// of lines but how far into the text the reader gets. The suite states it as a
// picture — line-clamp-003 shows *more* text balanced than unbalanced, because a
// narrower measure lets the last line hold a word beside the ellipsis.
//
// So the search cannot be held to "the same lines", and it must not be held to
// monotonicity either: reaching further at a narrower width is the behaviour, not
// a fault.

// clampTexts are paragraphs long enough that a clamp of two or three lines
// actually cuts something off.
var clampTexts = []string{
	"one two three four five six seven eight nine ten",
	"a bb ccc dddd eeeee ffffff ggggggg hhhhhhhh",
	"the quick brown fox jumps over the lazy dog and then some more",
	"supercalifragilisticexpialidocious and a few short words after it",
}

// TestTheClampedSearchNeverShowsLessThanTheBoxDid is the contract, and it is the
// whole of what the caller is promised.
//
// Balancing may rearrange the lines of a clamped block however it likes, and it
// may not cost the reader a word: the narrowest width it settles on has to reach
// at least as far into the content as the box's own width did. A search that
// returned something merely tidier would be trading text the author wrote for an
// even right edge, which is not a trade §5.1 offers and which nobody would see
// until they noticed a sentence ending early.
func TestTheClampedSearchNeverShowsLessThanTheBoxDid(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range clampTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{100, 200, 400} {
			for _, maxLines := range []int{1, 2, 3} {
				for _, ellipsis := range []float64{0, 12, 36} {
					got := br.BalanceClampedWidth(items, u(width), 0, u(ellipsis), maxLines)
					if got > u(width) {
						t.Errorf("%q at %gpx, %d lines, %gpx of ellipsis: balanced to "+
							"%gpx, which is wider than the box",
							text, width, maxLines, ellipsis, got.Px())
					}
					wantI, wantByte := br.clampedReach(items, u(width), 0, u(ellipsis), maxLines)
					gotI, gotByte := br.clampedReach(items, got, 0, u(ellipsis), maxLines)
					if gotI < wantI || (gotI == wantI && gotByte < wantByte) {
						t.Errorf("%q at %gpx, %d lines, %gpx of ellipsis: the box reached "+
							"item %d byte %d and the balanced %gpx reaches only item %d "+
							"byte %d — balancing may not cost the reader a word",
							text, width, maxLines, ellipsis, wantI, wantByte,
							got.Px(), gotI, gotByte)
					}
				}
			}
		}
	}
}

// TestTheClampedSearchIsTheNarrowestThatStillReaches is the search's own half of
// it.
//
// Without this the assertion above is satisfied by handing the box's width
// straight back, which reaches exactly as far and balances nothing.
func TestTheClampedSearchIsTheNarrowestThatStillReaches(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range clampTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{200, 400} {
			for _, maxLines := range []int{2, 3} {
				got := br.BalanceClampedWidth(items, u(width), 0, u(12), maxLines)
				if got <= 2 {
					continue
				}
				wantI, wantByte := br.clampedReach(items, u(width), 0, u(12), maxLines)
				narrowI, narrowByte := br.clampedReach(items, got.Sub(1), 0, u(12), maxLines)
				if narrowI > wantI || (narrowI == wantI && narrowByte >= wantByte) {
					t.Errorf("%q at %gpx, %d lines: balanced to %gpx, and one unit "+
						"narrower still reaches item %d byte %d against the box's %d/%d "+
						"— the search stopped before the narrowest width that reaches",
						text, width, maxLines, got.Px(), narrowI, narrowByte, wantI, wantByte)
				}
			}
		}
	}
}

// TestAWiderEllipsisNeverReachesFurther is the one monotonicity the clamped
// search does have.
//
// How far a clamped block reaches is *not* monotonic in the width — a narrower
// measure can show more, which is the whole reason this search exists. But it is
// monotonic in the mark: the ellipsis takes room on the last line and nowhere
// else, so a wider one can only leave less space for text.
func TestAWiderEllipsisNeverReachesFurther(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// Whether the mark ever costs anything. Every assertion below compares one
	// reach against another, so a reach that ignored the ellipsis entirely would
	// agree with itself for every width — and did: planted, that defect passed
	// this test until the counter arrived.
	shortened := 0
	for _, text := range clampTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{100, 200, 400} {
			for _, maxLines := range []int{1, 2, 3} {
				prevI, prevByte, first := 0, 0, true
				for _, ellipsis := range []float64{0, 6, 12, 24, 48} {
					i, iByte := br.clampedReach(items, u(width), 0, u(ellipsis), maxLines)
					if !first {
						if i > prevI || (i == prevI && iByte > prevByte) {
							t.Errorf("%q at %gpx, %d lines: a %gpx ellipsis reaches item %d "+
								"byte %d, further than the narrower mark before it reached "+
								"(%d/%d) — the mark takes room and cannot give any back",
								text, width, maxLines, ellipsis, i, iByte, prevI, prevByte)
						}
						if i < prevI || (i == prevI && iByte < prevByte) {
							shortened++
						}
					}
					prevI, prevByte, first = i, iByte, false
				}
			}
		}
	}
	if shortened == 0 {
		t.Error("no ellipsis anywhere in the corpus cost the reader a character, so " +
			"this proves only that the reach is consistent with itself — the mark is " +
			"not being taken off the last line at all, or the fixtures are all too " +
			"short for it to matter")
	}
}

// TestAClampedBlockNeverReachesPastItsContent is the bound the cursor has to
// respect.
//
// The reach is a cursor into the items, and it is handed straight back to the
// caller to resume from. One past the end would be read as "there is more", and a
// clamped block that thought it had cut something off would draw an ellipsis
// after text it had shown in full.
func TestAClampedBlockNeverReachesPastItsContent(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range clampTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{1, 30, 100, 400} {
			for _, maxLines := range []int{1, 2, 5} {
				i, iByte := br.clampedReach(items, u(width), 0, u(12), maxLines)
				if i < 0 || i > len(items) {
					t.Fatalf("%q at %gpx, %d lines: the reach is item %d of %d",
						text, width, maxLines, i, len(items))
				}
				if i < len(items) && (iByte < 0 || iByte > len(items[i].Text)) {
					t.Fatalf("%q at %gpx, %d lines: the reach is %d bytes into %q",
						text, width, maxLines, iByte, items[i].Text)
				}
				if i == len(items) && iByte != 0 {
					t.Errorf("%q at %gpx, %d lines: the reach is past the last item and "+
						"%d bytes into nothing", text, width, maxLines, iByte)
				}
			}
		}
	}
}

// TestMoreClampedLinesNeverShowLess is the clamp's own monotonicity.
//
// The clamp is a limit on how many lines are shown, and raising it can only let
// the reader further in. A search or a reach that lost ground as the limit rose
// would make "line-clamp: 3" show less than "line-clamp: 2", which is the one
// thing the property cannot mean.
func TestMoreClampedLinesNeverShowLess(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, text := range clampTexts {
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})
		for _, width := range []float64{60, 100, 200, 400} {
			var prevI, prevByte int
			for maxLines := 1; maxLines <= 6; maxLines++ {
				i, iByte := br.clampedReach(items, u(width), 0, u(12), maxLines)
				if maxLines > 1 && (i < prevI || (i == prevI && iByte < prevByte)) {
					t.Errorf("%q at %gpx: %d lines reach item %d byte %d and %d lines "+
						"reached %d/%d — showing another line cannot show less",
						text, width, maxLines, i, iByte, maxLines-1, prevI, prevByte)
				}
				prevI, prevByte = i, iByte
			}
		}
	}
}
