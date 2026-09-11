package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A bidi control at the end of a line does not keep the space before it on the
// line.
//
// CSS Text §4.1.1 removes a collapsible space at the end of a line. What counts
// as the end is decided by walking back from the last item, over the spaces
// themselves and over an inline box's own boundary — neither is content, and
// neither takes the line anywhere. A bidi control is the third of those and was
// not on the list: it is not a character of the text, it is an instruction to
// the bidirectional algorithm, and it sets no paper.
//
// So a line ending in one stopped the walk, the space in front of it was not at
// the end of the line any more, and it kept its width. "a ‭ b" in a box two
// characters wide set "a" and "b"; the same text with a span boundary in it set
// "a " and "b", and the first line was a character wider than the text it shows.
//
// It is the same rule as TestABidiControlInABoxOfItsOwnDoesNotSeparateTwoSpaces
// — §4.1.1 looks through a control — at the other end of the same section: that
// one is the collapsing, this one is the removal at a line edge. They were
// found four days and one invariant apart.
func TestABidiControlAtALineEndDoesNotHoldTheSpaceBeforeIt(t *testing.T) {
	const lro = "‭"
	// Two Courier characters wide at 16px, so "a" and "b" cannot share a line.
	const narrow = 25
	whole := visibleLinesOf(t, "a "+lro+" b", narrow)
	for _, cut := range []string{
		`<span>a </span><span>` + lro + ` b</span>`,
		`<span>a ` + lro + `</span><span> b</span>`,
		`<span>a </span><span>` + lro + `</span><span> b</span>`,
	} {
		got := visibleLinesOf(t, cut, narrow)
		if len(whole) != 2 {
			t.Fatalf("%q set %d lines %v; it is two and the comparison below is "+
				"against the wrong answer", "a "+lro+" b", len(whole), whole)
		}
		if !sameVisibleLines(got, whole) {
			t.Errorf("%q set %v and the same text written as %q set %v; the "+
				"control draws nothing, so the space in front of it is still at "+
				"the end of the line and is still removed",
				"a "+lro+" b", whole, cut, got)
		}
	}
}

// TestAnOrdinaryCharacterStillHoldsTheSpaceBeforeIt is the containment case.
//
// Only the bidi controls are looked through. A character that draws — even one
// as narrow as a full stop — is content, and the space in front of it is not at
// the end of the line at all. Widening the rule to every default-ignorable, or
// to "draws nothing much", takes this with it.
func TestAnOrdinaryCharacterStillHoldsTheSpaceBeforeIt(t *testing.T) {
	// Three Courier characters wide, so "a ." fits on the first line and "b"
	// does not. At two the line ends after "a" and the full stop is on the next
	// one, where there is no space in front of it to keep.
	const wide = 30
	got := visibleLinesOf(t, "a . b", wide)
	if len(got) == 0 || got[0].text != "a ." {
		t.Fatalf("\"a . b\" set %v; the fixture wants \"a .\" on the first "+
			"line, and the assertion below is about its width", got)
	}
	// Against the same line without the space rather than against three
	// characters: "a ." is three items and "abc" is one run, and two runs
	// quantized separately are a sixty-fourth away from one. What is asked is
	// that the space took room, which is a comparison and not an arithmetic.
	without := visibleLinesOf(t, "a. b", wide)[0].width
	if got[0].width <= without {
		t.Errorf("\"a . b\" set a first line %v wide and \"a.\" is %v; a full "+
			"stop is content, so the space before it is not at the end of the "+
			"line and must still take room", got[0].width, without)
	}
}

// TestADefaultIgnorableAtALineEndIsNotLookedThrough is the containment case the
// full stop above cannot be, and it is the same trap this rule has sprung twice.
//
// Widening the test to IsDefaultIgnorable moves *both* spellings of a text
// together — "a &#xFE0F; b" and the same text in spans both lose the space — so
// no comparison between two spellings can see it, and neither can the invariant
// that found the defect. It has to be asserted against what the text says.
//
// What the text says is CollapseWhitespaceAfter's answer. Phase I makes the bidi
// controls transparent and nothing else, so it has already kept the space in
// front of a variation selector by the time a line edge is reached; looking
// through the selector here would remove a space Phase I decided to keep, and
// the two halves of §4.1.1 would be drawing the line in different places.
func TestADefaultIgnorableAtALineEndIsNotLookedThrough(t *testing.T) {
	// Two Courier characters wide, so the first line is "a" and whatever the
	// rules leave attached to it.
	const narrow = 25
	one := visibleLinesOf(t, "a", narrow)[0].width
	for _, tc := range []struct{ what, char string }{
		{"a variation selector", "\uFE0F"},
		{"a soft hyphen", "\u00AD"},
		{"a zero width joiner", "\u200D"},
	} {
		got := visibleLinesOf(t, "a "+tc.char+" b", narrow)
		if len(got) == 0 {
			t.Fatalf("%s: set no lines", tc.what)
		}
		if got[0].width <= one {
			t.Errorf("%s: the first line of %q is %v wide and one character is "+
				"%v; it is a character of the text, so the space in front of it "+
				"is not at the end of the line and keeps its width",
				tc.what, "a "+tc.char+" b", got[0].width, one)
		}
	}
}

// visibleLine is one line's text with the bidi controls left out, and the width
// its runs take.
//
// The controls go because what is compared is what a reader sees, and which side
// of an invisible character a line ends on is not that. The width is what says
// the space went: it is the half a text comparison cannot see, since a removed
// space and a space of no width read the same.
type visibleLine struct {
	text  string
	width style.Unit
}

func sameVisibleLines(a, b []visibleLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// visibleLinesOf lays markup out in a Courier box of the given width.
//
// Courier because it is monospace, so a width is a number of characters and a
// failure says how many were kept rather than a quantity of sixty-fourths.
func visibleLinesOf(t *testing.T, markup string, px float64) []visibleLine {
	t.Helper()
	root := layoutOf(t, px, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-family: Courier; font-size: 16px }`)
	var out []visibleLine
	for _, line := range find(t, root, "d").Lines {
		var one visibleLine
		for _, r := range line.Runs {
			for _, c := range r.Text {
				if !isBidiControl(c) {
					one.text += string(c)
				}
			}
			one.width = one.width.Add(r.Width)
		}
		out = append(out, one)
	}
	return out
}
