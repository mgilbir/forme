package layout

import (
	"strconv"
	"testing"
)

// Where a line may break around a picture — CSS Text §5.1, and the sentence
// with the exception in it:
//
//	"For Web-compatibility there is a soft wrap opportunity before and after
//	each replaced element or other atomic inline, even when adjacent to a
//	character that would normally suppress them, including U+00A0 NO-BREAK
//	SPACE. However, with the exception of U+00A0 NO-BREAK SPACE, there must be
//	no soft wrap opportunity between atomic inlines and adjacent characters
//	belonging to the Unicode GL, WJ, or ZWJ line breaking classes."
//
// The engine offered the opportunity unconditionally, so a word joiner written
// between a word and a picture — which is the only reason anyone writes one —
// did nothing. The suite has twenty-seven tests for it, one per character.
//
// Each fixture below is the suite's own shape: a box one character wide, a
// letter, the character under test, and an inline-block one character wide. Two
// lines means the break was offered and one means it was not, and nothing else
// about the page can produce either answer.

// atomicLines lays out "A<mid><inline-block>" in a box one character wide and
// says how many lines it took. mono, ch and widthCSS are in the files named in
// their comments.
func atomicLines(t *testing.T, mid string) int {
	t.Helper()
	css := widthCSS(1, "") + `#s { display: inline-block; width: ` +
		strconv.FormatFloat(ch, 'f', -1, 64) + `px; height: 10px }`
	root := layoutOf(t, 10000, `<p id="p">A`+mid+`<span id="s"></span></p>`, css)
	return len(linesOf(t, root, "p"))
}

// atomicLinesAfter is the mirror: the picture first, then the character.
func atomicLinesAfter(t *testing.T, mid string) int {
	t.Helper()
	css := widthCSS(1, "") + `#s { display: inline-block; width: ` +
		strconv.FormatFloat(ch, 'f', -1, 64) + `px; height: 10px }`
	root := layoutOf(t, 10000, `<p id="p"><span id="s"></span>`+mid+`A</p>`, css)
	return len(linesOf(t, root, "p"))
}

// TestAPictureIsNotWrappedAwayFromWhatHoldsIt.
func TestAPictureIsNotWrappedAwayFromWhatHoldsIt(t *testing.T) {
	for _, tc := range []struct {
		what string
		mid  string
	}{
		{"a word joiner", "\u2060"},
		{"a zero width no-break space", "\uFEFF"},
		{"a zero width joiner", "\u200D"},
		{"a narrow no-break space", "\u202F"},
		{"a figure space", "\u2007"},
		{"a non-breaking hyphen", "\u2011"},
		{"a Mongolian vowel separator", "\u180E"},
		{"a Tibetan mark sbrul shad", "\u0F08"},
	} {
		if got := atomicLines(t, tc.mid); got != 1 {
			t.Errorf("%s before a picture: %d lines, want 1 — the two are held together",
				tc.what, got)
		}
		if got := atomicLinesAfter(t, tc.mid); got != 1 {
			t.Errorf("%s after a picture: %d lines, want 1 — the two are held together",
				tc.what, got)
		}
	}
}

// TestAPictureIsWrappedAwayFromEverythingElse is the containment half, and the
// larger one: the whole point of the Web-compatibility sentence is that a
// picture *is* wrapped away from ordinary text.
func TestAPictureIsWrappedAwayFromEverythingElse(t *testing.T) {
	for _, tc := range []struct {
		what string
		mid  string
	}{
		{"nothing between them", ""},
		{"a letter", "B"},
		{"a space", " "},
		{"a zero width space", "\u200B"},
		{"a hyphen", "-"},
		// The exception the specification names, and the reason this cannot be
		// "class GL holds on": a no-break space is class GL and breaks anyway.
		{"a no-break space", "\u00A0"},
	} {
		if got := atomicLines(t, tc.mid); got != 2 {
			t.Errorf("%s before a picture: %d lines, want 2 — the picture wraps",
				tc.what, got)
		}
	}
}

// TestTheHoldIsOnlyOnTheCharacterBesideIt. A word joiner two characters away
// from the picture holds nothing: the rule is about what is adjacent to it, and
// a rule that looked further would weld a whole paragraph to a picture at the
// end of it.
func TestTheHoldIsOnlyOnTheCharacterBesideIt(t *testing.T) {
	if got := atomicLines(t, "\u2060B"); got != 2 {
		t.Errorf("a word joiner with a letter between it and the picture: %d lines, "+
			"want 2 — the letter is what the picture is beside", got)
	}
}

// TestOnlyAPicturesOwnOpportunityIsHeld.
//
// The rule is about the boundary between an atomic inline and a character. An
// opportunity from anywhere else — a space, most of all — is not one a word
// joiner after it may take away: UAX #14's LB12a is "[^SP BA HY] × GL", and the
// space is exactly the exception in it.
//
// It needs the opportunity to cross a box boundary to be visible, because
// within one text node the opportunity belongs to the piece rather than to the
// state, and only the state is what the rule reads.
func TestOnlyAPicturesOwnOpportunityIsHeld(t *testing.T) {
	css := widthCSS(2, "")
	// A backtick string would put the six characters "\u2060" on the page; the
	// character has to arrive as itself.
	root := layoutOf(t, 10000, "<p id=\"p\">AA <span>\u2060BB</span></p>", css)
	got := lineTexts(linesOf(t, root, "p"))
	if len(got) != 2 {
		t.Errorf("%d lines, want 2: %q — the break after the space is not the "+
			"picture rule's to withhold", len(got), got)
	}
}
