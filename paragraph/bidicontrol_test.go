package paragraph

import "testing"

// A bidi control is not a character of the text.
//
// §4.1.1 collapses white space that is adjacent, and what counts as standing
// between two spaces is the question. A zero-width space does: it is a character,
// and the suite says so — "U+00A0 is exactly equivalent to U+200B U+0020 U+200B".
// A bidi control does not: its whole function is to instruct the bidirectional
// algorithm, and the text either side of it is as adjacent as it was.
//
// The two sets overlap — both are Default_Ignorable, and U+200B sits inside the
// same range as the marks U+200E and U+200F — so the predicate has to be
// Bidi_Control and not the ignorable property. Using the wider one would undo
// the zero-width space rule, which the suite tests in the opposite direction.
//
// The suite compares a boundary written as markup against the same boundary
// written as a control, and requires the two to be identical: bidi-003 and its
// neighbours are two blocks of the same words, one with spans and one with
// U+202E and U+202C.

func TestABidiControlDoesNotSeparateTwoSpaces(t *testing.T) {
	for _, tc := range []struct{ what, in, want string }{
		{"an override between two spaces", "ccc ‮ lll", "ccc ‮lll"},
		{"a pop between two spaces", "ccc ‬ lll", "ccc ‬lll"},
		{"a run of spaces on both sides", "ccc  ‮  lll", "ccc ‮lll"},
		{"two controls together", "ccc ‮‬ lll", "ccc ‮‬lll"},
		{"a control with no space beside it", "ccc‮lll", "ccc‮lll"},
		{"a control with a space on one side", "ccc ‮lll", "ccc ‮lll"},
		// The text may end with one, and then there is no character after the
		// run to write it out beside. It is still part of the text: the
		// bidirectional algorithm reads it, and a control silently dropped
		// leaves an override open that the document closed.
		{"a control at the very end", "ccc ‮", "ccc ‮"},
		{"a control at the end after a run", "ccc   ‮", "ccc ‮"},
	} {
		if got := CollapseWhitespace(tc.in, "normal"); got != tc.want {
			t.Errorf("%s: %q collapsed to %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestTheSurvivingSpaceIsOnTheSideMarkupWouldPutIt.
//
// The point of the tests this fixes is that a boundary written as a control and
// the same boundary written as two elements render alike. Two elements keep the
// space belonging to the first of them — the second one's is the one that
// collapses away — so the space that survives a control has to be on the same
// side of it, which is why the control is written out after the run rather than
// where it stood.
func TestTheSurvivingSpaceIsOnTheSideMarkupWouldPutIt(t *testing.T) {
	got := CollapseWhitespace("ccc ‮ lll", "normal")
	if got != "ccc ‮lll" {
		t.Errorf("collapsed to %q, want the space before the control", got)
	}
}

// TestAZeroWidthSpaceStillSeparates is the other half, and the reason the
// predicate is Bidi_Control rather than Default_Ignorable: U+200B is in the same
// ignorable range as the two directional marks and behaves the opposite way.
func TestAZeroWidthSpaceStillSeparates(t *testing.T) {
	if got := CollapseWhitespace("ccc ​ ​ lll", "normal"); got != "ccc ​ ​ lll" {
		t.Errorf("collapsed to %q; a zero-width space stands between two spaces and "+
			"they do not collapse", got)
	}
	// And a mark that *is* a bidi control, from inside that same range.
	if got := CollapseWhitespace("ccc ‎ lll", "normal"); got != "ccc ‎lll" {
		t.Errorf("a left-to-right mark collapsed to %q", got)
	}
}

// TestIsBidiControlIsExactlyUnicodesSet, so that the predicate is not quietly
// widened into the ignorable property it is deliberately narrower than.
func TestIsBidiControlIsExactlyUnicodesSet(t *testing.T) {
	want := map[rune]bool{
		0x061C: true, 0x200E: true, 0x200F: true,
		0x202A: true, 0x202B: true, 0x202C: true, 0x202D: true, 0x202E: true,
		0x2066: true, 0x2067: true, 0x2068: true, 0x2069: true,
	}
	for r := rune(0); r < 0x3000; r++ {
		if got := IsBidiControl(r); got != want[r] {
			t.Errorf("IsBidiControl(U+%04X) = %v, want %v", r, got, want[r])
		}
	}
	// The neighbours that are ignorable and are not controls, named so that a
	// change reaching for the wider property is caught by name.
	for _, r := range []rune{0x00AD, 0x034F, 0x200B, 0x200C, 0x200D, 0x2060, 0xFEFF} {
		if IsBidiControl(r) {
			t.Errorf("U+%04X is ignorable and is not a bidi control", r)
		}
	}
}
