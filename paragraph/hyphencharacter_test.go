package paragraph

import "testing"

// A hyphen a document actually contains, UAX #14 and CSS Text §6.1.
//
// Three characters are hyphens for line breaking and they are not the same
// class. U+002D HYPHEN-MINUS is HY; U+2010 HYPHEN and U+2013 EN DASH are HH,
// the unambiguous hyphen Unicode 15.1 separated out. What the three classes
// differ about is what may come *before* a line — linebreak.go's isLatinHyphen
// is that half, and §5.3 tailors it three ways. On the other side they agree: a
// line may end after any of them, which is what lets a hyphenated compound break
// where it is written.
//
// The soft hyphen is not one of them and is not here. U+00AD is a request rather
// than a character — nothing is drawn for it unless a line breaks there — and
// §6.1's hyphens property is about that request. A hyphen that is *written* is
// an ordinary break opportunity whatever the property says, which is what
// hyphens-none-013 asserts in as many words.

// TestALineEndsAfterAHyphenWhicheverHyphenItIs is the rule.
func TestALineEndsAfterAHyphenWhicheverHyphenItIs(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		what string
	}{
		{'-', "U+002D HYPHEN-MINUS, class HY"},
		{0x2010, "U+2010 HYPHEN, class HH"},
		{0x2013, "U+2013 EN DASH, class HH"},
	} {
		text := "regu" + string(tc.r) + "lation"
		want := "regu" + string(tc.r) + "|lation"
		if got := splitsWith(t, text, WordBreak{}, LineBreak{}); got != want {
			t.Errorf("%s: %q came out %q, want %q", tc.what, text, got, want)
		}
	}
}

// TestTheHyphensPropertyDoesNotReachAWrittenHyphen is what hyphens-none-013 is
// for: the property governs where a word may be broken *without* a hyphen in it,
// and a hyphen that is there is not that.
//
// All four values, because the assertion is that none of them matters. "none" is
// the one the suite names — its whole document is "regu‐lation imple‐menta‐tion"
// under "hyphens: none" in six characters of room, and it wants five lines.
func TestTheHyphensPropertyDoesNotReachAWrittenHyphen(t *testing.T) {
	for _, value := range []string{"none", "manual", "auto", ""} {
		hy, _ := HyphensOf(value)
		pieces, _ := SplitAtBreaks("regu‐lation",
			WhiteSpace{Collapse: true, Wrap: true}, WordBreak{}, LineBreak{}, hy, WritingSystemOther)
		out := ""
		for _, p := range pieces {
			if p.BreakBefore {
				out += "|"
			}
			out += p.Text
		}
		if want := "regu‐|lation"; out != want {
			t.Errorf("hyphens: %q gave %q, want %q", value, out, want)
		}
	}
}

// TestAHyphenWithNothingAfterItOffersNothing is the guard the ASCII hyphen has
// always carried, held to for the other two.
//
// There would be nothing to move to the next line. The same goes for a hyphen in
// front of a space: the line ends after the space, not before it, and an
// opportunity there is one LB7 forbids anyway.
func TestAHyphenWithNothingAfterItOffersNothing(t *testing.T) {
	for _, r := range []rune{'-', 0x2010, 0x2013} {
		for _, tc := range []struct{ text, want string }{
			{"regu" + string(r), "regu" + string(r)},
			{"regu" + string(r) + " lation", "regu" + string(r) + " |lation"},
		} {
			if got := splitsWith(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
				t.Errorf("%#04X: %q came out %q, want %q", r, tc.text, got, tc.want)
			}
		}
	}
}

// TestALineStillMayNotBeginWithAnUnambiguousHyphen is the other half, and the
// containment case for the change: the two classes differ about the start of a
// line and this must not have levelled them.
//
// §5.3: "loose" lets a line begin with U+2010 or U+2013 and the other three
// values do not. line-break-loose-hyphens-001 says "the second line starts with
// a hyphen" and line-break-normal-hyphens-001, over the same text, says it "ends
// with a hyphen".
func TestALineStillMayNotBeginWithAnUnambiguousHyphen(t *testing.T) {
	for _, r := range []rune{0x2010, 0x2013} {
		for _, tc := range []struct {
			value string
			begin bool
		}{{"auto", false}, {"normal", false}, {"strict", false}, {"loose", true}} {
			lb, _ := LineBreakOf(tc.value)
			if got := MayNotBeginLine(string(r), lb); got == tc.begin {
				t.Errorf("%#04X under %q: may not begin a line = %v, want %v",
					r, tc.value, got, !tc.begin)
			}
		}
	}
}
