package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/bidi"
)

// A line separator is not a paragraph separator, and neither is a control
// character that happens to end a line.
//
// UAX #9 §3.3.1 divides text into bidi paragraphs at class B and nowhere else.
// Five characters end a line wherever they appear — IsMandatoryBreak's BK and NL
// — and only two of them are class B. This engine ended a bidi paragraph at all
// five, so text either side of a U+2028 was resolved twice instead of once, and
// the neutrals between two right-to-left characters took the block's direction
// where UAX #9's N1 gives them the characters'.

// TestALineSeparatorDoesNotEndTheBidiParagraph checks the split against the
// class, one character at a time, rather than against a list written here — a
// list is what went wrong.
func TestALineSeparatorDoesNotEndTheBidiParagraph(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		what string
	}{
		{0x000A, "a preserved newline"},
		{0x0085, "U+0085 NEXT LINE"},
		{0x2029, "U+2029 PARAGRAPH SEPARATOR"},
		{0x000B, "U+000B LINE TABULATION"},
		{0x000C, "U+000C FORM FEED"},
		{0x2028, "U+2028 LINE SEPARATOR"},
	} {
		want := bidi.ClassOf(tc.r) == bidi.B
		pieces, _ := SplitAtBreaks("a"+string(tc.r)+"b",
			WhiteSpace{}, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		var got, found bool
		for _, p := range pieces {
			if p.Segment {
				got, found = p.EndsBidiParagraph, true
			}
		}
		if !found {
			t.Errorf("%s produced no break at all", tc.what)
			continue
		}
		if got != want {
			t.Errorf("%s ends the bidi paragraph=%v, want %v — its class is %v and "+
				"UAX #9 divides paragraphs at class B and nowhere else",
				tc.what, got, want, bidi.ClassOf(tc.r))
		}
	}
}

// TestASeparatorRidesOnTheBreakAndAControlCharacterDoesNot.
//
// Both are kept — the pieces spell the input either way, which
// TestAMandatoryBreakKeepsItsCharacter holds — and where they are kept is what
// decides whether they reach the page. Layout sets the text of an ordinary
// piece and does not set the text of the break, so a character on the break is
// carried without being drawn, exactly as a preserved newline is.
//
// §5.1's note is written about control characters — "control characters other
// than [tab, newline] ... are otherwise rendered as a visible glyph" — and the
// suite's white-space/control-chars documents say so in as many words: "U+000C,
// which is in the unicode category CC, must be visible". U+2028 and U+2029 are
// categories Zl and Zp, and nothing draws a separator.
func TestASeparatorRidesOnTheBreakAndAControlCharacterDoesNot(t *testing.T) {
	for _, tc := range []struct {
		r       rune
		onBreak bool
		what    string
	}{
		{0x000B, false, "U+000B LINE TABULATION, category Cc"},
		{0x000C, false, "U+000C FORM FEED, category Cc"},
		{0x0085, false, "U+0085 NEXT LINE, category Cc"},
		{0x2028, true, "U+2028 LINE SEPARATOR, category Zl"},
		{0x2029, true, "U+2029 PARAGRAPH SEPARATOR, category Zp"},
	} {
		pieces, _ := SplitAtBreaks("a"+string(tc.r)+"b",
			WhiteSpace{}, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		var spelled strings.Builder
		var onBreak bool
		for _, p := range pieces {
			spelled.WriteString(p.Text)
			if p.Segment && strings.ContainsRune(p.Text, tc.r) {
				onBreak = true
			}
		}
		if got := spelled.String(); got != "a"+string(tc.r)+"b" {
			t.Errorf("%s: the pieces spell %q — nothing may be lost", tc.what, got)
		}
		if onBreak != tc.onBreak {
			t.Errorf("%s: rides on the break=%v, want %v", tc.what, onBreak, tc.onBreak)
		}
	}
	// And the break itself survives either way, which is the half that must not
	// be lost: line-breaking-022 writes all five in a column one character wide
	// and asks for six lines.
	for _, r := range []rune{0x000B, 0x000C, 0x0085, 0x2028, 0x2029} {
		pieces, _ := SplitAtBreaks("a"+string(r)+"b",
			WhiteSpace{}, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		var breaks int
		for _, p := range pieces {
			if p.Segment {
				breaks++
			}
		}
		if breaks != 1 {
			t.Errorf("U+%04X made %d mandatory breaks, want 1", r, breaks)
		}
	}
}
