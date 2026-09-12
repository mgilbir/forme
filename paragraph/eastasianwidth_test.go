package paragraph

import "testing"

// TestTheTwoWidthQuestionsAreTheTableReadBothWays.
func TestTheTwoWidthQuestionsAreTheTableReadBothWays(t *testing.T) {
	for _, c := range []struct {
		r       rune
		has, is bool
		what    string
	}{
		{'A', true, false, "an ASCII letter has a full-width form"},
		{'0', true, false, "so does a digit"},
		{' ', true, false, "and a space, whose wide twin is the ideographic space"},
		{'Ａ', false, true, "a full-width letter is one"},
		{'０', false, true, "and a full-width digit"},
		{0x3000, false, true, "and the ideographic space"},
		{0xFF71, true, false, "a halfwidth katakana has a full-width form"},
		{'ア', false, true, "and the full katakana is it"},
		{'あ', false, false, "a hiragana has neither: it is not a width pair at all"},
		{'漢', false, false, "nor an ideograph"},
		{'д', false, false, "nor a Cyrillic letter"},
	} {
		if got := HasFullWidthForm(c.r); got != c.has {
			t.Errorf("HasFullWidthForm(%q) = %v, want %v — %s", c.r, got, c.has, c.what)
		}
		if got := IsFullWidthForm(c.r); got != c.is {
			t.Errorf("IsFullWidthForm(%q) = %v, want %v — %s", c.r, got, c.is, c.what)
		}
	}
}

// TestEveryPairIsFoundFromBothEnds is the two readers against the table itself,
// so that the reverse index cannot drift from the forward one.
func TestEveryPairIsFoundFromBothEnds(t *testing.T) {
	if len(fullWidthForms) == 0 {
		t.Fatal("the width table is empty, so this checks nothing")
	}
	for _, pair := range fullWidthForms {
		if !HasFullWidthForm(pair.from) {
			t.Errorf("the table maps %q to %q and HasFullWidthForm(%q) is false",
				pair.from, pair.to, pair.from)
		}
		if !IsFullWidthForm(pair.to) {
			t.Errorf("the table maps %q to %q and IsFullWidthForm(%q) is false",
				pair.from, pair.to, pair.to)
		}
	}
	// And no character is both, which is what makes the two questions two: a
	// character that had a full-width form and *was* one would be a chain, and
	// asking a font for one width would be ambiguous.
	for _, pair := range fullWidthForms {
		if HasFullWidthForm(pair.to) {
			t.Errorf("%q is a full-width form and has one; the table has a chain "+
				"in it and the two questions are no longer exclusive", pair.to)
		}
	}
}
