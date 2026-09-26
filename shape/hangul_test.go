package shape

import "testing"

// TestHangulIsSetAsHarfBuzzSetsIt: a syllable composed where the face has it
// and taken apart into jamo where it does not, the jamo given the features that
// assemble them, 'calt' kept off them, a tone mark moved in front of its
// syllable or set against a dotted circle, and nothing composed that the text
// wrote apart. The glyphs are the fixture's: 1 to 3 the jamo U+1100, U+1161 and
// U+11A8, 4 and 5 the syllables U+AC00 and U+AC01, 6 and 7 the tone marks (the
// second of no width), 8 the dotted circle, 9 the compatibility jamo U+3131, 10
// and 11 the Old Hangul U+1113 and U+1176, 12 to 14 what 'ljmo', 'vjmo' and
// 'tjmo' make, 15 what 'calt' makes, 16 to 18 '=', U+0338 and '≠', 19 U+1102.
func TestHangulIsSetAsHarfBuzzSetsIt(t *testing.T) {
	checkModelCases(t, "hangul", []modelCase{
		{"\uAC00", "4,1000", "a syllable the face has"},
		{"\u1100\u1161", "4,1000", "its jamo, composed"},
		{"\u1100\u1161\u11A8", "5,1010", "three jamo, composed"},
		{"\uAC00\u11A8", "5,1010", "a syllable and a trailing jamo, composed"},
		{"\u1102\u1161", "12,620 13,420", "jamo whose syllable the face lacks, with their features"},
		{"\u1113\u1176", "12,620 13,420", "Old Hangul jamo, with their features and without 'calt'"},
		{"\uB098", "12,620 13,420", "a syllable the face lacks, taken apart"},
		{"\uAC00\u302E", "6,200 4,1000", "a tone mark, in front of its syllable"},
		{"\uAC00\u302F", "4,1000 7,0,-900,0", "a tone mark of no width, left where it is"},
		{"\u3131\u302E", "15,111 6,0,-455,0 8,0,-455,0", "a tone mark after no syllable, against a dotted circle"},
		{"\u302F", "8,500 7,0", "a tone mark of no width after nothing"},
		{"\u3131", "15,111", "'calt' on what is not a jamo"},
		{"\uAC00=\u0338", "4,1000 16,500 17,0,-650,0", "nothing composed"},
		{"\uAC00\u2260", "4,1000 18,520", "nothing taken apart"},
		{"\uAC00\u2260\u0301", "4,1000 18,520 0,0,-260,862", "nothing taken apart, even with a mark on it"},
	})
}
