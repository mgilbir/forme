package layout

import (
	"strings"
	"testing"
)

// A line that begins with a closing bracket, laid out.
//
// paragraph/linebreakclass_test.go pins the rule over a string; this is the
// same rule at the point it was visible, which is a page. The suite has it as
// word-break-break-all-016: "XX XXX..." in eight characters of room, against a
// reference that breaks between the last two letters. Its own assertion says
// why — "break-all breaks between the last two letters, because breaking
// opportunities between the punctuation characters are forbidden" — and this
// engine broke inside the ellipsis instead, leaving a line that began with a
// full stop.
//
// The numbers are arithmetic: the text is Courier, every glyph 600 units of
// 1000, so a character at 100px is 60px and a width in characters is exact.
// mono, ch and widthCSS are in the files named in their comments.

// TestALaidOutLineDoesNotBeginWithPunctuation is the suite's own fixture.
func TestALaidOutLineDoesNotBeginWithPunctuation(t *testing.T) {
	root := layoutOf(t, 10000, `<p id="p">XX XXX...</p>`,
		widthCSS(8, "word-break: break-all"))
	got := lineTexts(linesOf(t, root, "p"))
	if len(got) != 2 {
		t.Fatalf("%d lines, want 2: %q", len(got), got)
	}
	if strings.HasPrefix(got[1], ".") {
		t.Errorf("the second line begins with a full stop: %q", got)
	}
	// And it breaks where the reference does: five characters then four.
	if want := []string{"XX XX", "X..."}; got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestTheRuleHoldsForEveryClosingCharacterInARow. One bracket at the head of a
// line is a mistake; a run of them is the same mistake, and the rule has to
// hold for each in turn rather than for the first one only.
func TestTheRuleHoldsForEveryClosingCharacterInARow(t *testing.T) {
	root := layoutOf(t, 10000, `<p id="p">XXXX)))]]]</p>`,
		widthCSS(5, "word-break: break-all"))
	got := lineTexts(linesOf(t, root, "p"))
	if len(got) == 0 {
		t.Fatal("no lines")
	}
	for i, line := range got {
		if i > 0 && strings.ContainsAny(line[:1], ")]") {
			t.Errorf("line %d begins with %q: %q", i, line[:1], got)
		}
	}
	// The text has to arrive whole, whatever the breaking did with it.
	if joined := strings.Join(got, ""); joined != "XXXX)))]]]" {
		t.Errorf("the text came out as %q", joined)
	}
}

// TestAWordStillBreaksWhereBreakAllSaysItMay is the containment case, and the
// one that would catch a rule applied too widely: break-all exists to break
// inside a word, and a paragraph of letters must still do it.
func TestAWordStillBreaksWhereBreakAllSaysItMay(t *testing.T) {
	root := layoutOf(t, 10000, `<p id="p">XXXXXXXXXX</p>`,
		widthCSS(4, "word-break: break-all"))
	got := lineTexts(linesOf(t, root, "p"))
	want := []string{"XXXX", "XXXX", "XX"}
	if len(got) != len(want) {
		t.Fatalf("%d lines for ten characters in four of room, want %d: %q",
			len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("the lines are %q, want %q", got, want)
			break
		}
	}
}

// TestALineMayStillBeginWithAnOpeningBracket. LB14 forbids ending a line on
// one, not beginning a line with one, and a rule that confused the two would
// leave an opening bracket stranded at the end of a line instead.
func TestALineMayStillBeginWithAnOpeningBracket(t *testing.T) {
	root := layoutOf(t, 10000, `<p id="p">XXXX(XXX</p>`,
		widthCSS(4, "word-break: break-all"))
	got := lineTexts(linesOf(t, root, "p"))
	if len(got) < 2 {
		t.Fatalf("%d lines, want at least 2: %q", len(got), got)
	}
	if got[0] != "XXXX" {
		t.Errorf("the lines are %q; four characters fit in four characters of room "+
			"and the bracket begins the next", got)
	}
	if !strings.HasPrefix(got[1], "(") {
		t.Errorf("the second line is %q; an opening bracket may begin a line", got[1])
	}
}
