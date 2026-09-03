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

// TestTheOpportunityAfterOneIsStillRefusedWhereARuleSaysSo. It is deferred to
// the next boundary rather than taken where it is offered, which is what runs
// the prohibitions over it: LB13 says a line may not begin with a closing
// bracket, and that is true after a danda as much as after an ideograph.
func TestTheOpportunityAfterOneIsStillRefusedWhereARuleSaysSo(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"एक।)दो", "एक।)|दो"},
		{"एक।!दो", "एक।!|दो"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%q breaks as %q, want %q — the opportunity moves past the "+
				"character a line may not begin with", tc.text, got, tc.want)
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
			WordBreak{}, LineBreak{}, h)
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
