package layout

import (
	"testing"
)

// A bidi control does not make a line non-empty.
//
// CSS Text §5.1 lets an overlong word overflow a line that has nothing on it —
// "some form of fallback line breaking must occur [...] Overflowing is not
// allowed" is about the other case, and a line cannot hold less than one
// character. The fill asks whether the line holds anything a reader would see,
// and answered yes for a bidi control: it is not content, it is an instruction
// to the bidirectional algorithm, and it sets no paper.
//
// So a control at the start of a line made the line look occupied, the letter
// after it was moved off to the next line, and the document got an empty first
// line taking a line's height. It is the third place §4.1.1's "look through a
// control" rule was missing — the collapsing, the trim at a line edge, and this
// — and the only one of the three whose symptom is a line rather than a space.
//
// Found by FuzzBoundaryLines, within a minute of the target existing.
func TestABidiControlDoesNotMakeALineNonEmpty(t *testing.T) {
	const lro = "‭"
	// Narrower than one Courier character, which is 9.6px at this size, so the
	// letter has to overflow a line rather than wait for one with room. At ten
	// pixels the letter fits and there is nothing to see — which is how the
	// first version of this test came to pass under its own planted defect.
	const narrow = 8
	want := visibleLinesOf(t, " "+lro+" A", narrow)
	if len(want) != 1 {
		t.Fatalf("%q set %d lines %v; the leading white space collapses away and "+
			"the letter overflows, so it is one", " "+lro+" A", len(want), want)
	}
	for _, cut := range []string{
		`<span> </span><span>` + lro + ` A</span>`,
		`<span>` + lro + `</span><span> A</span>`,
		`<span> </span><span>` + lro + `</span><span> A</span>`,
	} {
		got := visibleLinesOf(t, cut, narrow)
		if !sameVisibleLines(got, want) {
			t.Errorf("%q set %v and the same text written as %q set %v; the "+
				"control puts nothing on the line, so the line the letter "+
				"overflows is still the first one", " "+lro+" A", want, cut, got)
		}
	}
}

// TestAnOrdinaryCharacterDoesMakeALineNonEmpty is the containment case.
//
// The rule that lets a word overflow is for a line with *nothing* on it. A
// character that draws is something, so the letter after it waits for a line of
// its own — which is the whole point of the rule and what a widening of this
// would take away.
func TestAnOrdinaryCharacterDoesMakeALineNonEmpty(t *testing.T) {
	const narrow = 8
	got := visibleLinesOf(t, "​.​A", narrow)
	if len(got) < 2 {
		t.Errorf("%q set %v; a full stop is content, so the line it is on is not "+
			"empty and the letter that does not fit belongs on the next one",
			"​.​A", got)
	}
}
