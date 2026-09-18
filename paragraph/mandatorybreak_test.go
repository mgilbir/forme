package paragraph

import (
	"testing"
	"unicode"
)

// UAX #14's mandatory breaks, CSS Text §5.1.
//
// Class BK — U+000B, U+000C, U+2028, U+2029 — and class NL, U+0085. LB4 and LB5
// make each of them end a line wherever it appears, and no value of white-space
// is written over them.
//
// They are not segment breaks and the difference is the whole of why they are
// asked about separately: a segment break is *collapsible*, so a newline under
// "white-space: normal" becomes a space and the line goes on. These do not
// collapse and do not become spaces. The suite writes the difference as
// line-breaking/line-breaking-022, five of them between six spans in a column
// one character wide, against a reference six lines tall.

// TestAMandatoryBreakEndsTheLineAtEveryWhiteSpaceValue.
func TestAMandatoryBreakEndsTheLineAtEveryWhiteSpaceValue(t *testing.T) {
	for _, tc := range []struct{ text, what string }{
		{"\u000b", "U+000B LINE TABULATION"},
		{"\u000c", "U+000C FORM FEED"},
		{"\u0085", "U+0085 NEXT LINE"},
		{"\u2028", "U+2028 LINE SEPARATOR"},
		{"\u2029", "U+2029 PARAGRAPH SEPARATOR"},
	} {
		if !IsMandatoryBreak([]rune(tc.text)[0]) {
			t.Errorf("%s is not a mandatory break", tc.what)
		}
		for _, value := range []string{"normal", "pre", "pre-wrap", "pre-line", "nowrap"} {
			ws := WhiteSpaceOf(value)
			collapsed := CollapseWhitespace("1"+tc.text+"2", value, WordSpaceTransform{})
			pieces, _ := SplitAtBreaks(collapsed, ws, WordBreak{}, LineBreak{},
				Hyphens{}, WritingSystemOther)
			var forced int
			for _, p := range pieces {
				if p.Segment {
					forced++
				}
			}
			if forced != 1 {
				t.Errorf("%s under white-space: %s made %d forced breaks in %q, want 1",
					tc.what, value, forced, collapsed)
			}
		}
	}
}

// TestAMandatoryBreakIsNotASegmentBreak is the distinction stated as the pair it
// is: a newline under "normal" is a space and the line goes on, and these five
// are not.
func TestAMandatoryBreakIsNotASegmentBreak(t *testing.T) {
	ws := WhiteSpaceOf("normal")
	collapsed := CollapseWhitespace("1\n2", "normal", WordSpaceTransform{})
	if collapsed != "1 2" {
		t.Fatalf("a newline under normal collapsed to %q, want a space", collapsed)
	}
	pieces, _ := SplitAtBreaks(collapsed, ws, WordBreak{}, LineBreak{}, Hyphens{},
		WritingSystemOther)
	for _, p := range pieces {
		if p.Segment {
			t.Errorf("a newline under normal made a forced break; it is a segment " +
				"break and collapses to a space")
		}
	}
	if IsMandatoryBreak('\n') || IsMandatoryBreak('\r') {
		t.Error("a newline is a segment break and not a mandatory one")
	}
}

// TestAMandatoryBreakIsStillACharacter. §5.1's note asks for both halves —
// "control characters ... are otherwise rendered as a visible glyph" — so the
// character stays in the text that ends the line rather than being swallowed
// the way a newline is. Three of the suite's control-chars documents are
// mismatch references against a blank page and say exactly that.
func TestAMandatoryBreakIsStillACharacter(t *testing.T) {
	ws := WhiteSpaceOf("normal")
	pieces, _ := SplitAtBreaks("1\f2", ws, WordBreak{}, LineBreak{}, Hyphens{},
		WritingSystemOther)
	var text string
	for _, p := range pieces {
		text += p.Text
	}
	if text != "1\f2" {
		t.Errorf("the pieces spell %q, want the form feed kept: a control character "+
			"that ends a line is still drawn", text)
	}
}

// TestAMandatoryBreakKeepsItsCharacter is the other half of the distinction
// above, and the one a fuzzer found: **the pieces spell the input**, whatever
// the character was and wherever it ends up.
//
// Where it ends up is the second claim here, and it is not the same for all
// five. A *control* character is set — §5.1's note asks for that and the suite's
// control-chars documents say so by name — so it stays in the piece before the
// break and the break carries nothing. U+2028 and U+2029 are separators rather
// than control characters, categories Zl and Zp, and nothing draws a separator:
// they ride on the break piece itself, exactly as a preserved newline does,
// whose text layout does not set.
//
// Both halves matter and they used to be one. Writing a separator into the piece
// before it put two glyphs on the page that the document does not contain, which
// CSS2's bidi-breaking-003 counts; swallowing it outright would fix that and
// lose a character, which is what the spelling invariant is here to refuse. See
// spellPieces in invariants_test.go, and testdata/fuzz/FuzzSplitAtBreaks, where
// the input that found the first version of this is kept.
func TestAMandatoryBreakKeepsItsCharacter(t *testing.T) {
	for _, tc := range []struct {
		text string
		r    rune
	}{
		{"\v", 0x000B}, {"\f", 0x000C}, {"\u0085", 0x0085},
		{"\u2028", 0x2028}, {"\u2029", 0x2029}, {"a\vb", 0x000B},
	} {
		text := tc.text
		for _, value := range []string{"pre", "pre-wrap", "break-spaces"} {
			pieces, _ := SplitAtBreaks(text, WhiteSpaceOf(value), WordBreak{},
				LineBreak{}, Hyphens{}, WritingSystemOther)
			var spelled string
			breaks := 0
			for _, p := range pieces {
				spelled += p.Text
				if p.Segment {
					breaks++
					// A separator rides on the break; a control character does
					// not, because it is set and the break is not.
					want := ""
					if !unicode.Is(unicode.Cc, tc.r) {
						want = string(tc.r)
					}
					if p.Text != want {
						t.Errorf("%q under %s: the break carries %q, want %q",
							text, value, p.Text, want)
					}
				}
			}
			if spelled != text {
				t.Errorf("%q under %s: the pieces spell %q — a preserving value "+
					"loses and invents nothing", text, value, spelled)
			}
			if breaks != 1 {
				t.Errorf("%q under %s made %d breaks, want 1", text, value, breaks)
			}
		}
	}

	// And the segment break it is not: a preserved newline *is* its piece's
	// text, so the same reading spells the input back.
	pieces, _ := SplitAtBreaks("a\nb", WhiteSpaceOf("pre"), WordBreak{}, LineBreak{},
		Hyphens{}, WritingSystemOther)
	var spelled string
	for _, p := range pieces {
		spelled += p.Text
		if p.Segment && p.Text != "\n" {
			t.Errorf("the piece for a preserved newline carries %q, want the "+
				"newline it was made from", p.Text)
		}
	}
	if spelled != "a\nb" {
		t.Errorf("a preserved newline spelled %q", spelled)
	}
}
