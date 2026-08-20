package paragraph

import "testing"

// The segment break transformation, CSS Text §4.1.1.
//
// A newline in collapsible white space usually becomes a space. Between two
// East Asian characters it does not: it is removed outright, because Japanese
// and Chinese are written without spaces between words.
//
// The rule is worth the table it needs. A CJK paragraph hard-wrapped in the
// source — which is how a great deal of it is written — gained a space at the
// end of every line it was wrapped at, in the middle of words, all through the
// text. It reads as deliberate spacing rather than as a bug, which is the kind
// of wrongness this engine's guardrails exist for and which no finding could
// have reported, because nothing was unsupported: the wrong answer was
// confidently produced.
//
//	Otherwise, if the East Asian Width property of both the character before
//	and after the segment break is F, W, or H (not A), and neither side is
//	Hangul, then the segment break is removed.

// TestASegmentBreakBetweenEastAsianCharactersIsRemoved walks the rule's four
// corners, and every row is one of the suite's own fixtures.
func TestASegmentBreakBetweenEastAsianCharactersIsRemoved(t *testing.T) {
	for _, tc := range []struct {
		in, want, what string
	}{
		// Removed: both sides F, W or H, in every pairing the suite writes.
		{"ＡＢ\nＣＤ", "ＡＢＣＤ", "fullwidth and fullwidth"},
		{"ＡＢ\nﾃｽﾄ", "ＡＢﾃｽﾄ", "fullwidth and halfwidth katakana"},
		{"ＡＢ\n測試", "ＡＢ測試", "fullwidth and wide"},
		{"ﾃｽﾄ\nＡＢ", "ﾃｽﾄＡＢ", "halfwidth and fullwidth"},
		{"ﾃｽﾄ\nﾃｽﾄ", "ﾃｽﾄﾃｽﾄ", "halfwidth and halfwidth"},
		{"ﾃｽﾄ\n測試", "ﾃｽﾄ測試", "halfwidth and wide"},
		{"一些\n中文", "一些中文", "wide and wide"},
		{"あ\nい", "あい", "hiragana"},

		// Kept: the other side is not East Asian, or is the ambiguous set, or
		// is Hangul.
		{"ＡＢ\nnarrow", "ＡＢ narrow", "fullwidth and narrow Latin"},
		{"ＡＢ\n■", "ＡＢ ■", "fullwidth and an ambiguous character"},
		{"ＡＢ\nآزمون", "ＡＢ آزمون", "fullwidth and Arabic"},
		{"ＡＢ\n테스트", "ＡＢ 테스트", "fullwidth and Hangul"},
		{"테스트\nＡＢ", "테스트 ＡＢ", "Hangul and fullwidth"},
		{"테스트\n테스트", "테스트 테스트", "Hangul and Hangul"},
		{"a\nb", "a b", "Latin, which is every other document"},
	} {
		if got := CollapseWhitespace(tc.in, "normal", WordSpaceTransform{}); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestTheAmbiguousCharactersAreNotWideForThisRule states the exclusion on its
// own, because it is the one a reader is most likely to get wrong: those
// characters *are* wide when East Asian text is set, and the rule names F, W and
// H rather than saying "wide".
func TestTheAmbiguousCharactersAreNotWideForThisRule(t *testing.T) {
	for _, r := range []rune{
		0x25A0, // BLACK SQUARE, which the suite tests by name
		0x00A7, // SECTION SIGN
		0x03B1, // GREEK SMALL LETTER ALPHA
		0x2010, // HYPHEN
		0x2502, // BOX DRAWINGS LIGHT VERTICAL
	} {
		if removesSegmentBreak(r) {
			t.Errorf("%#04X is East Asian Width A and was treated as wide", r)
		}
	}
	// And the three that are.
	for _, r := range []rune{0xFF21, 0xFF83, 0x4E00} {
		if !removesSegmentBreak(r) {
			t.Errorf("%#04X is F, W or H and was not treated as wide", r)
		}
	}
	// Hangul is wide and is carved out.
	for _, r := range []rune{0xD14C, 0x1100, 0x3131} {
		if removesSegmentBreak(r) {
			t.Errorf("%#04X is Hangul and the rule excludes it", r)
		}
	}
}

// TestTheCharacterEitherSideIsTheOneAReaderSees.
//
// The rule says "the character before and after the segment break", and a
// variation selector or a soft hyphen written there is not a character a reader
// sees. The suite's segment-break-transformation-ignorable-1 writes Han
// characters with their variation selectors and asks for the break to go anyway.
func TestTheCharacterEitherSideIsTheOneAReaderSees(t *testing.T) {
	for _, tc := range []struct{ in, want, what string }{
		{"社︀\n福︀", "社︀福︀", "variation selectors on both sides"},
		{"葛\U000E0100\n禰\U000E0100", "葛\U000E0100禰\U000E0100", "selectors from plane 14"},
		{"葛­\n葛", "葛­葛", "a soft hyphen before the break"},
		{"葛‎\n葛", "葛‎葛", "a left-to-right mark before the break"},
		{"葛\n‎葛", "葛‎葛", "one after it"},
		{"葛\n︀葛", "葛︀葛", "a variation selector after it"},
		// And the other way: a character a reader *does* see keeps the space,
		// even though it is invisible in the sense of having no ink.
		{"葛\n 葛", "葛葛", "a space, which the earlier phase removes"},
	} {
		if got := CollapseWhitespace(tc.in, "normal", WordSpaceTransform{}); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestTheZeroWidthSpaceStillWins, which is the rule stated before this one and
// applies whatever the scripts either side are.
func TestTheZeroWidthSpaceStillWins(t *testing.T) {
	for _, tc := range []struct{ in, want, what string }{
		{"一些​\n中文", "一些​中文", "before the break, both sides wide"},
		{"一些\n​中文", "一些​中文", "after it"},
		{"abc​\ndef", "abc​def", "before it, Latin either side"},
		{"abc\n​def", "abc​def", "after it, Latin either side"},
	} {
		if got := CollapseWhitespace(tc.in, "normal", WordSpaceTransform{}); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestTheEarlierPhasesRunFirst. §4.1.1 removes the collapsible spaces around a
// segment break and folds a run of breaks into one *before* this rule decides,
// so the characters it looks at are the ones left either side. The suite writes
// each of these as its own fixture, segment-break-transformation-removable-1
// through -4.
func TestTheEarlierPhasesRunFirst(t *testing.T) {
	const want = "一些中文"
	for _, tc := range []struct{ in, what string }{
		{"一些\n中文", "one break"},
		{"一些\n\n\n中文", "three breaks"},
		{"一些  \n  中文", "spaces either side of the break"},
		{"一些 \n \n \n 中文", "spaces and breaks alternating"},
		{"一些\t\n\t中文", "tabs either side"},
	} {
		if got := CollapseWhitespace(tc.in, "normal", WordSpaceTransform{}); got != want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, want)
		}
	}
}

// TestAnOrdinarySpaceBetweenIdeographsSurvives is the containment case, and it
// is the one that would be silently destroyed by a rule written a little too
// wide. The transformation is about *segment breaks*: a space somebody typed
// between two ideographs is content and stays.
func TestAnOrdinarySpaceBetweenIdeographsSurvives(t *testing.T) {
	for _, tc := range []struct{ in, want, what string }{
		{"一些 中文", "一些 中文", "one space"},
		{"一些   中文", "一些 中文", "a run of them, collapsed to one but not removed"},
		{"一些\t中文", "一些 中文", "a tab, which becomes a space"},
		{"一些　中文", "一些　中文", "an ideographic space, which never collapses"},
	} {
		if got := CollapseWhitespace(tc.in, "normal", WordSpaceTransform{}); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestPreLineKeepsTheBreakItself. Under pre-line a segment break is preserved
// rather than transformed, so there is nothing for this rule to do — and a rule
// applied there would delete a line the author wrote.
func TestPreLineKeepsTheBreakItself(t *testing.T) {
	// The values are white-space-collapse's, which is what this is given.
	for _, value := range []string{"preserve-breaks", "preserve", "break-spaces"} {
		if got := CollapseWhitespace("一些\n中文", value, WordSpaceTransform{}); got != "一些\n中文" {
			t.Errorf("%s: %q; the break is preserved and is not this rule's to remove",
				value, got)
		}
	}
}

// TestTheWidthTablesAreSortedAndDisjoint, which the search over them needs and
// which nothing else would notice: an out-of-order range is not found, and the
// rule then quietly stops applying to whatever is in it.
func TestTheWidthTablesAreSortedAndDisjoint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []struct{ lo, hi rune }
	}{
		{"eastAsianWideRanges", eastAsianWideRanges[:]},
		{"hangulRanges", hangulRanges[:]},
	} {
		if len(tc.table) == 0 {
			t.Fatalf("%s is empty", tc.name)
		}
		for i, s := range tc.table {
			if s.lo > s.hi {
				t.Errorf("%s[%d] is %#04X..%#04X, which is empty", tc.name, i, s.lo, s.hi)
			}
			if i > 0 && s.lo <= tc.table[i-1].hi+1 {
				t.Errorf("%s[%d] starts at %#04X and the range before it ends at %#04X; "+
					"they should have been merged or are out of order",
					tc.name, i, s.lo, tc.table[i-1].hi)
			}
			// Every range is found, at both ends and in the middle.
			for _, r := range []rune{s.lo, s.hi, s.lo + (s.hi-s.lo)/2} {
				if !inRanges(r, tc.table) {
					t.Errorf("%s: %#04X is in range %d and was not found", tc.name, r, i)
				}
			}
			// And the character before the range is not, unless the range
			// before it holds one.
			if s.lo > 0 && (i == 0 || tc.table[i-1].hi < s.lo-1) && inRanges(s.lo-1, tc.table) {
				t.Errorf("%s: %#04X is not in any range and was found", tc.name, s.lo-1)
			}
		}
	}
}
