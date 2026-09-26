package paragraph

import "testing"

// A line may end after a character of UAX #14's class BA, whatever follows it.
//
// It is the rule every writing system that divides its words with a mark rather
// than with a space depends on. This package offered an opportunity at a space,
// between two ideographs and at a hyphen, and a script that uses none of the
// three had nowhere to wrap at all: the suite's word-break-normal-ethiopic is
// forty characters of Amharic in a zero-width box beside a reference that
// breaks it after each wordspace, and it came out as one line.

// TestALineMayEndAfterOneOfThese, one script at a time.
func TestALineMayEndAfterOneOfThese(t *testing.T) {
	for _, tc := range []struct {
		what, text, want string
	}{
		{"an Ethiopic wordspace", "ተወልዱ፡ኵሉ", "ተወልዱ፡|ኵሉ"},
		{"a Tibetan tsheg", "བོད་ཡིག", "བོད་|ཡིག"},
		{"a Devanagari danda", "एक।दो", "एक।|दो"},
		// Myanmar is also a dictionary script — see NeedsDictionaryBreaking —
		// so the fallback offers a boundary between its clusters as well. The
		// break after the little section is the one this row is about, and it
		// is there among them.
		// Burmese has a word list here, so what is either side of the section
		// mark is one word rather than four characters. The rule the row is
		// about is still the one it shows: the line may end *after* the mark.
		{"a Myanmar little section", "မြန်၊မာ", "မြန်၊|မာ"},
		{"a Mongolian colon", "\u182E\u1804\u182F", "\u182E\u1804|\u182F"},
		{"a hyphenation point", "co‧op", "co‧|op"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %q breaks as %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestALineStillDoesNotEndInTheMiddleOfAWord, which is what says the rule is
// about the class and not about every character.
func TestALineStillDoesNotEndInTheMiddleOfAWord(t *testing.T) {
	for _, text := range []string{"unbroken", "ኵሉሰብእ", "एकदो"} {
		if got := marks(t, text, WordBreak{}, LineBreak{}); got != text {
			t.Errorf("%q breaks as %q, want no opportunity in it", text, got)
		}
	}
}

// TestTheOpportunityAfterOneIsStillRefusedWhereARuleSaysSo. It is the pair rules
// that decide it, at the next boundary: LB13 says a line may not begin with a
// closing bracket or an exclamation mark, and that is true after a danda as much
// as after an ideograph.
//
// It used to be *moved* past the character a line may not begin with, and
// broke "एक।)|दो". UAX #14 does not move an opportunity: it decides each
// boundary, and the one after the bracket is LB30's — "CP × AL" for a bracket
// that is not East Asian — and the one after the exclamation mark is CSS Text's
// note, which keeps "!" with the letter after it. A fullwidth bracket is the
// row where the next boundary is an opportunity: LB30 is about the narrow ones.
func TestTheOpportunityAfterOneIsStillRefusedWhereARuleSaysSo(t *testing.T) {
	for _, tc := range []struct{ text, want, why string }{
		{"एक।)दो", "एक।)दो", "LB13, then LB30"},
		{"एक।!दो", "एक।!दो", "LB13, then CSS Text's note on U+0021"},
		{"एक।」दो", "एक।」|दो", "LB13, then LB31"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%q breaks as %q, want %q (%s)", tc.text, got, tc.want, tc.why)
		}
	}
}

// TestASoftHyphenIsNotOneOfThem is the class's one exception, and it is a CSS
// rule rather than a Unicode one: §6.1 makes the opportunity a soft hyphen
// offers conditional on the hyphens property, and "hyphens: none" suppresses
// it. U+00AD is class BA, so a table read straight would break there anyway.
func TestASoftHyphenIsNotOneOfThem(t *testing.T) {
	const text = "man­ual"
	breaks := func(h Hyphens) bool {
		pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, LineBreak{}, h, WritingSystemOther)
		for i, p := range pieces {
			if i > 0 && p.BreakBefore {
				return true
			}
		}
		return false
	}
	if breaks(Hyphens{None: true}) {
		t.Errorf("with hyphens: none the soft hyphen in %q still offers an "+
			"opportunity; the property says not to", text)
	}
	// And with the property allowing it the opportunity is there, so the row
	// above is about the property and not about the character being ignored.
	if !breaks(Hyphens{}) {
		t.Errorf("with hyphens: manual the soft hyphen offers no opportunity")
	}
}
