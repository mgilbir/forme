package layout

import "testing"

// A word cut inside tiles the way a word cut at its edges does.
//
// The merge group exists so that the runs of one word add up to the word: each
// run's two ends are rounded separately, so the sum of the parts is the
// quantization of the whole rather than a sixty-fourth away from it. A line that
// ends *inside* a run — which is overflow-wrap and word-break: break-all, and
// nothing else — cuts a run into two pieces, and those pieces have to add up the
// same way.
//
// They did not. The measurement fell back to measuring the piece on its own
// wherever the run was part of a group, which is the one case where the group's
// string is not the run's and the fallback was written for. So:
//
//	abc                            "ab" at 1228 and "c" at 615
//	<span>a</span><span>bc</span>  "ab" at 1228 and "c" at 614
//
// The same word, the same group, the same two lines, and a line a sixty-fourth
// of a pixel narrower for having a span boundary in it.
//
// FuzzRunTiling cannot reach this: it lays every case out under nowrap, so no
// line ever ends inside a run. FuzzBoundaryLines wraps but sets no declaration,
// and without one a line never ends inside a word either. It was found by
// running that target's comparison with "overflow-wrap: break-word" added.
func TestAWordCutInsideTilesTheSameWay(t *testing.T) {
	const sheet = `#d { font-family: Courier; font-size: 16px; overflow-wrap: break-word }`
	for _, tc := range []struct {
		what, whole, cut string
		px               float64
	}{
		{"cut before the break", "abc", `<span>a</span><span>bc</span>`, 25},
		{"cut at the break", "abc", `<span>ab</span><span>c</span>`, 25},
		{"a longer word", "abcdef", `<span>abc</span><span>def</span>`, 25},
		{"three boxes", "abcdef",
			`<span>ab</span><span>cd</span><span>ef</span>`, 25},
		{"a box apiece", "abcd",
			`<span>a</span><span>b</span><span>c</span><span>d</span>`, 25},
		// Broken across more than two lines, so the tail of one cut is cut
		// again: the piece being measured is then a stretch of a run that is
		// itself a stretch, and the two offsets have to be added rather than
		// one of them used. Without the longer words below, a planted version
		// that forgot the inner offset passed.
		{"five lines", "abcdefghij", `<span>abcde</span><span>fghij</span>`, 25},
		{"four lines of three", "abcdefghijkl",
			`<span>ab</span><span>cdefghijkl</span>`, 30},
	} {
		whole := visibleLinesWith(t, tc.whole, sheet, tc.px)
		cut := visibleLinesWith(t, tc.cut, sheet, tc.px)
		if len(whole) < 2 {
			t.Fatalf("%s: %q set %d lines %v; it has to be broken inside for "+
				"there to be anything to compare", tc.what, tc.whole, len(whole), whole)
		}
		if !sameVisibleLines(cut, whole) {
			t.Errorf("%s: %q set %v and %q set %v", tc.what, tc.whole, whole,
				tc.cut, cut)
		}
	}
}

// What this does not fix, measured rather than left to be discovered.
//
// Over the seed corpus of FuzzBoundaryLines with "overflow-wrap: break-word"
// added, the two spellings disagreed 326 times before this and 36 after. The 36
// are all "AVATAR" in the bundled Noto Sans, and they are not rounding: a line
// holding one "A" is 654 wide in one spelling and 613 in the other, which is the
// AV kern and two thirds of a pixel.
//
// So it is a different fault in the same place — how much of a kern each side of
// a cut keeps, where the cut falls inside a word that a box boundary also runs
// through — and the tiling this commit fixes is not it. Courier is the face here
// because Courier does not kern: these cases are the arithmetic alone, and the
// kerning one is kept out of the fixture rather than smuggled into it.
//
// It is a *decision* rather than a defect with an obvious fix, which is why it
// is not done here. The whole text puts each letter on its line unkerned, which
// is what a kern is — an adjustment between two adjacent glyphs, and two glyphs
// on different lines are not adjacent. The cut version splits the kerned pair
// across the two lines, because its items are part of a merge group and the
// stretch is measured inside the group's shaping.
//
// Deciding for the first reading means a cut edge stops inheriting the shaping
// of what follows it — and SplitHead's note records that decision being taken
// the other way, deliberately, for cursive joining: an Arabic letter at a line
// end is drawn in its final form, and measuring it isolated picked a cut against
// one width and drew it at another. Kerning is the same mechanism with the
// opposite answer, so whatever settles it has to keep that.
