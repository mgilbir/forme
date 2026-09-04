package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/segment"
)

// A line may never end in the middle of a character.
//
// "Character" here means what a reader means by it and not what a byte or a rune
// is: UAX #29's grapheme cluster. A Hangul syllable is up to three runes, an
// accented letter may be two, a family emoji is seven with joiners between them,
// and a Devanagari syllable is several with a virama holding them together. Every
// one of those is one thing on the page, and a line that ends inside one either
// draws two half-formed shapes or drops the second half entirely.
//
// This is not hypothetical for this engine: "Find grapheme clusters, and stop
// cutting Hangul syllables in half" is a commit in its history, and the fault it
// fixed was invisible in Latin text and in most tests.
//
// Two levels are checked, because they can fail independently. Where a line *may*
// end is decided when the text is cut into pieces; where one *does* end inside a
// word is decided by overflow-wrap, in the breaking, which cuts at a byte offset
// of its own choosing.

// clusterStarts is the set of byte offsets in s that begin a grapheme cluster,
// including the two ends.
func clusterStarts(s string) map[int]bool {
	at := map[int]bool{0: true, len(s): true}
	for _, i := range segment.Boundaries(nil, s) {
		at[i] = true
	}
	return at
}

// TestNoBreakOpportunityFallsInsideACluster is the first level: the places a line
// is *allowed* to end.
func TestNoBreakOpportunityFallsInsideACluster(t *testing.T) {
	for _, w := range whiteSpaces {
		for _, tc := range texts {
			pieces, _ := SplitAtBreaks(tc.text, w.ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)

			// The boundaries are taken over what the pieces actually spell rather
			// than over the input, because a collapsing value rewrites a tab as a
			// space and the offsets would not line up.
			var joined strings.Builder
			for _, p := range pieces {
				joined.WriteString(p.Text)
			}
			at := clusterStarts(joined.String())

			off := 0
			for i, p := range pieces {
				if i > 0 && !at[off] {
					t.Errorf("%s under white-space %s: a piece begins at byte %d of %q, "+
						"which is inside a grapheme cluster — a line ending there would "+
						"cut a character in half",
						tc.name, w.name, off, joined.String())
				}
				off += len(p.Text)
			}
		}
	}
}

// TestNoLineEndsInsideACluster is the second level, and the one overflow-wrap
// reaches: a line that ends part-way through a word.
//
// The breaking returns its cursor as an item and a byte offset into that item, so
// the offset is the thing to check. Every text is broken at a measure narrow
// enough to force the cut — with overflow-wrap allowing it, a single character
// per line — which is exactly the state in which a cut lands wherever the
// arithmetic says rather than where a rule does.
func TestNoLineEndsInsideACluster(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, width := range []float64{1, 7, 13, 30} {
			items := itemsOf(t, br, face, tc.text,
				WhiteSpaceOf("collapse"), OverflowWrap{BreakWord: true})
			from, fromByte := 0, 0
			for from < len(items) {
				_, next, nextByte, _, _ := br.BreakOneLine(items, from, fromByte, u(width), 0)
				if nextByte != 0 {
					if next >= len(items) {
						t.Fatalf("%s at %gpx: the cursor points %d bytes into item %d, "+
							"which does not exist", tc.name, width, nextByte, next)
					}
					text := items[next].Text
					if nextByte < 0 || nextByte > len(text) {
						t.Fatalf("%s at %gpx: the cursor points %d bytes into %q",
							tc.name, width, nextByte, text)
					}
					if !clusterStarts(text)[nextByte] {
						t.Errorf("%s at %gpx: a line ended %d bytes into %q, which is "+
							"inside a grapheme cluster — the character is cut in half",
							tc.name, width, nextByte, text)
					}
				}
				if !CursorAdvanced(from, fromByte, next, nextByte) {
					break
				}
				from, fromByte = next, nextByte
			}
		}
	}
}

// TestAClusterIsNeverSplitEvenWhenItAloneOverflows is the case that has to be
// got right by refusing rather than by arithmetic.
//
// One Devanagari syllable is several runes and wider than a one-pixel line. There
// is no offset inside it a line may end at, so the only correct answer is to
// overflow — and an engine that cut "as much as fits" would find an offset,
// because there always is one.
func TestAClusterIsNeverSplitEvenWhenItAloneOverflows(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, one := range []struct{ name, text string }{
		{"a Devanagari syllable", "क्षि"},
		{"a Hangul syllable", "각"},
		{"a letter with two combining marks", "ẫ"},
		{"a family emoji", "\U0001f468‍\U0001f469‍\U0001f466"},
	} {
		items := itemsOf(t, br, face, one.text,
			WhiteSpaceOf("collapse"), OverflowWrap{BreakWord: true})
		if len(items) == 0 {
			continue
		}
		line, next, nextByte, _, _ := br.BreakOneLine(items, 0, 0, u(1), 0)
		if nextByte != 0 {
			t.Errorf("%s: a 1px line cut %q at byte %d — the whole of it is one "+
				"character and there is nowhere inside it a line may end",
				one.name, items[next].Text, nextByte)
		}
		if got := lineRunes(line); got != one.text {
			t.Errorf("%s: a 1px line took %q, want the whole cluster %q — it does not "+
				"fit and must overflow rather than be divided", one.name, got, one.text)
		}
	}
}
