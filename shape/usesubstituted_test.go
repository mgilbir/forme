package shape

import "testing"

// TestTheUniversalEngineReadsWhatWasSubstituted: 'pref' and 'rphf' say which
// glyph they are for by substituting it, and the engine reads the first glyph
// they substituted — not where a rule of theirs matched. The fixture states
// each as a contextual rule that matches at a letter and substitutes the glyph
// after it: 'pref' turns the vowel sign U+11440 (3) into a form drawn before
// its letter (4), and 'rphf' the halant (5) after a ra (7) into another glyph
// (6). Read where the rules matched, the letter was taken for the pre-base
// form and moved in front of itself, and the pre-base form stayed after it;
// Noto Sans Newa is written that way. Read where 'rphf' matched, a letter
// whose nukta it substituted was taken for a repha and moved after it. The
// glyphs are the fixture's: 1 and 2 letters, 3 the vowel sign, 4 its pre-base
// form, 5 the halant, 6 what 'rphf' makes of it, 7 ra, 8 the nukta U+11446,
// 9 what 'rphf' makes of that.
func TestTheUniversalEngineReadsWhatWasSubstituted(t *testing.T) {
	checkModelCases(t, "newa", []modelCase{
		{"\U00011431\U00011440", "4,350 2,610", "the pre-base form, in front of its letter"},
		{"\U0001142B\U00011442\U0001140E", "7,620 6,0,-200,0 1,600", "a halant 'rphf' substituted is not a repha at the head"},
		{"\U0001140E\U00011440", "1,600 3,300", "no rule for this letter"},
		{"\U0001140E\U00011446\U00011440", "1,600 9,0,-90,0 3,300", "a nukta 'rphf' substituted leaves the letter where it is"},
	})
}
