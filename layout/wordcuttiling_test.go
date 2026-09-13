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

// What this did not fix, and what did.
//
// Over the seed corpus of FuzzBoundaryLines with "overflow-wrap: break-word"
// added, the two spellings disagreed 326 times before this and 36 after. The 36
// were all "AVATAR" in the bundled Noto Sans: a line holding one "A" was 654
// wide in one spelling and 613 in the other, which is the AV kern and two
// thirds of a pixel.
//
// That was a second fault in the same place and not this one, and it is fixed
// now — see TestAKernAgainstTheNextLineIsNotCharged. It was never the decision
// it looked like. unkernLineEnd already took the pair kerning out of a line's
// last run, deliberately and for the reason §8.1 gives; it just gave back
// nothing for a run of a merge group, because it rebuilt the group from the
// run's own text and got a string the group never had.
//
// Courier is still the face here, and still because Courier does not kern:
// these cases are the arithmetic alone, and keeping the kerning out of them is
// what makes a failure here mean the tiling and nothing else.
