package paragraph

import "testing"

// An opportunity the last character of a text node offers belongs to whatever
// comes after it, which may be another box.
//
// SplitAtBreaks returns a flag for exactly that, and the file says so where the
// soft hyphen is handled: "a soft hyphen that ends a text node has to offer its
// opportunity to whatever box comes next … which is what every other opportunity
// here already does". Two of them did not. An ordinary hyphen and — under
// line-break: loose — a currency or number sign each asked whether they were at
// the end of the *text*, and gave up their opportunity when they were.
//
// What that costs is a word that breaks differently for being written in two
// elements. "high-way" breaks after the hyphen and "high-<span>way</span>"
// broke nowhere, so the compound overflowed its box; and the suite's
// line-break-loose-018 writes its prefix in an element of its own so that it can
// be coloured, which is what a test for a *character* does.
func TestAnOpportunityAtTheEndOfANodeIsHandedOn(t *testing.T) {
	loose := LineBreak{Loose: true}
	for _, tc := range []struct {
		text string
		lb   LineBreak
		want bool
		what string
	}{
		{"high-", LineBreak{}, true, "an ordinary hyphen ending the text"},
		{"high‐", LineBreak{}, true, "a U+2010 hyphen ending the text"},
		{"high­", LineBreak{}, true, "a soft hyphen, which already did this"},
		{"high- x", LineBreak{}, false, "a hyphen with a space after it"},
		{"highway", LineBreak{}, false, "no opportunity at all"},

		{"€", loose, true, "a euro sign under line-break: loose"},
		{"№", loose, true, "a numero sign under line-break: loose"},
		{"￥", loose, true, "a fullwidth yen sign under line-break: loose"},
		{"€", LineBreak{}, false, "the same sign under the initial value"},
		{"€ x", loose, false, "a prefix with a space after it"},
	} {
		_, ended := SplitAtBreaks(tc.text, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, tc.lb, Hyphens{}, WritingSystemOther)
		if ended.Offered != tc.want {
			t.Errorf("%s (%q): the text ends at an opportunity = %v, want %v",
				tc.what, tc.text, ended.Offered, tc.want)
		}
	}

	// A space after one is still not an opportunity, which is the guard that
	// stays: the space is already one, and a line that ended in front of it
	// would begin the next with the space. Asked of the piece that follows,
	// because the flag above is about the end of the text and this is not.
	for _, tc := range []struct {
		text string
		lb   LineBreak
		what string
	}{
		{"high- x", LineBreak{}, "a hyphen"},
		{"€ x", loose, "a prefix under line-break: loose"},
	} {
		pieces, _ := SplitAtBreaks(tc.text, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, tc.lb, Hyphens{}, WritingSystemOther)
		for _, p := range pieces {
			if p.Space && p.BreakBefore {
				t.Errorf("%s (%q): a line may end in front of the space after it",
					tc.what, tc.text)
			}
		}
	}

	// And the soft hyphen with "hyphens: none", which is the value that takes
	// it away: the one opportunity here that already reached the next box, so
	// that the fixture above is not the whole of what the flag can say.
	_, ended := SplitAtBreaks("high­", WhiteSpace{Collapse: true, Wrap: true},
		WordBreak{}, LineBreak{}, Hyphens{None: true}, WritingSystemOther)
	if ended.Offered {
		t.Error("a soft hyphen offered an opportunity under hyphens: none")
	}
}
