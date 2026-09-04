package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A ligature that spans the boundary between two runs, CSS Text §8.1.
//
//	the boundary between two inline elements does not break shaping
//
// A ligature is shaping. "of<span>f</span>ice" is one word and the face's ffi
// is what a reader of it expects, so the three runs have to be shaped as one
// string — and the glyph then belongs to whichever run holds the first
// character it came from, because a glyph is drawn once and cannot be drawn by
// two runs at half a glyph each.
//
// The run on the far side draws nothing for the characters the ligature
// swallowed and takes none of their width. That is the arithmetic that makes the
// two halves add up to what one run would have measured, and it is why the
// measure goes through the same call the painting does.

// mergeFace is a face whose 'calt' joins the fixture's a and b into one glyph.
func mergeFace(t *testing.T) *Face {
	t.Helper()
	return ligatureFace(t, 0,
		[]fonttest.Ligature{{Components: []int{gidA, gidB}, Glyph: gidBalt}}, nil)
}

func mergedGIDs(t *testing.T, f *Face, s, before, after, pre, post string) []int {
	t.Helper()
	glyphs, _ := f.ShapeGlyphsMerged(s, before, after, pre, post, true, Features{})
	out := make([]int, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.GID
	}
	return out
}

// TestAMergedNeighbourFormsTheLigatureThatSpansTheBoundary, and the run that
// holds the ligature's first character is the one that draws it.
func TestAMergedNeighbourFormsTheLigatureThatSpansTheBoundary(t *testing.T) {
	f := mergeFace(t)
	// The control: one run of "ab" is the ligature, which is what the two runs
	// below have to add up to.
	if got := shapedGIDs(t, f, "ab"); !sameGIDs(got, []int{gidBalt}) {
		t.Fatalf("the fixture shaped \"ab\" to %v, want the ligature %d — this "+
			"test is not exercising what it claims to", got, gidBalt)
	}
	// "a" with "b" after it, merged: the ligature's first character is this
	// run's, so this run draws it.
	if got := mergedGIDs(t, f, "a", "", "b", "", "b"); !sameGIDs(got, []int{gidBalt}) {
		t.Errorf("the run holding the ligature's first character drew %v, want %v",
			got, []int{gidBalt})
	}
	// "b" with "a" before it, merged: the ligature belongs to the other run, so
	// this one draws nothing at all.
	if got := mergedGIDs(t, f, "b", "a", "", "a", ""); len(got) != 0 {
		t.Errorf("the run whose character the ligature swallowed drew %v, want "+
			"nothing — the glyph is the other run's and is drawn once", got)
	}
}

// TestAnUnmergedNeighbourIsStillOnlyContext is the other half, and the
// behaviour every run of every document keeps: the text either side decides the
// forms and contributes no glyphs.
func TestAnUnmergedNeighbourIsStillOnlyContext(t *testing.T) {
	f := mergeFace(t)
	for _, tc := range []struct {
		s, before, after string
		want             []int
		what             string
	}{
		{"a", "", "b", []int{gidA}, "the run before the ligature keeps its own letter"},
		{"b", "a", "", []int{gidB}, "the run after it keeps its own letter"},
	} {
		if got := mergedGIDs(t, f, tc.s, tc.before, tc.after, "", ""); !sameGIDs(got, tc.want) {
			t.Errorf("%s: drew %v, want %v", tc.what, got, tc.want)
		}
		// And through the older call, which is the one every other caller uses.
		if got, _ := f.ShapeGlyphsInContext(tc.s, tc.before, tc.after, Features{}); len(got) != 1 ||
			got[0].GID != tc.want[0] {
			t.Errorf("%s, in context: drew %v, want %v", tc.what, got, tc.want)
		}
	}
}

// TestMergingOneSideLeavesTheOtherAsContext. A run with a mergeable neighbour on
// one side and a plain one on the other still takes its forms from both, which
// is what keeps the two questions separate rather than one overruling the other.
func TestMergingOneSideLeavesTheOtherAsContext(t *testing.T) {
	f := mergeFace(t)
	// "b" between "a" and "c", merging only backwards: the ligature takes the
	// b, so nothing is left of this run — and the "c" on the far side, which is
	// not merged, contributes no glyph of its own either way.
	if got := mergedGIDs(t, f, "b", "a", "c", "a", ""); len(got) != 0 {
		t.Errorf("merging backwards only drew %v, want nothing", got)
	}
	// The same run merging only forwards, where the ligature is not formed
	// because its first character is in the run that was left as context.
	if got := mergedGIDs(t, f, "b", "a", "c", "", "c"); !sameGIDs(got, []int{gidB}) {
		t.Errorf("merging forwards only drew %v, want the letter itself %v",
			got, []int{gidB})
	}
}

// TestAMergedRunReportsOnlyItsOwnMissingCharacters. The count is what names a
// face that cannot set the text, and a run named for its neighbour's missing
// characters would report a gap it does not have.
func TestAMergedRunReportsOnlyItsOwnMissingCharacters(t *testing.T) {
	f := mergeFace(t)
	// U+0041 is not in the fixture's repertoire; "a" is.
	if _, missing := f.ShapeGlyphsMerged("a", "A", "", "A", "", true, Features{}); missing != 0 {
		t.Errorf("a run whose *neighbour* has an unset character reported %d "+
			"missing, want 0", missing)
	}
	if _, missing := f.ShapeGlyphsMerged("A", "a", "", "a", "", true, Features{}); missing != 1 {
		t.Errorf("a run with one unset character reported %d missing, want 1", missing)
	}
}

// TestAGroupsRunsAddUpToTheWholeWord.
//
// A run of a merge group is measured from the two ends of the span it covers
// within the group rather than as a width of its own, so that a caller rounding
// to a layout unit can round the *ends*. Every run then begins where the one
// before it ended, and the rounded widths sum to the group's own rounded width.
//
// Rounding the differences instead leaves a word written in three runs a
// sixty-fourth of a pixel from the same word written in one, which is what the
// suite's shaping_lig-000 came out as: a lam-alef ligature in the right glyphs
// at the wrong pen.
func TestAGroupsRunsAddUpToTheWholeWord(t *testing.T) {
	f := mergeFace(t)
	const size = 36
	_, whole := f.MeasureShapedMergedSpan("aab", size, "", "", "", "", true, Features{})
	for _, parts := range [][]string{
		{"a", "ab"},
		{"aa", "b"},
		{"a", "a", "b"},
	} {
		var head, through []float64
		for i := range parts {
			pre, post := "", ""
			for k := 0; k < i; k++ {
				pre += parts[k]
			}
			for k := i + 1; k < len(parts); k++ {
				post += parts[k]
			}
			h, t2 := f.MeasureShapedMergedSpan(parts[i], size, pre, post, pre, post,
				true, Features{})
			head = append(head, h)
			through = append(through, t2)
		}
		if head[0] != 0 {
			t.Errorf("%v: the first run begins at %v, want 0", parts, head[0])
		}
		for i := 1; i < len(parts); i++ {
			if head[i] != through[i-1] {
				t.Errorf("%v: run %d begins at %v where run %d ended at %v; the "+
					"spans have to meet or the rounded widths do not telescope",
					parts, i, head[i], i-1, through[i-1])
			}
		}
		if got := through[len(through)-1]; got != whole {
			t.Errorf("%v: the divided word ends at %v and the whole one at %v",
				parts, got, whole)
		}
	}
}
