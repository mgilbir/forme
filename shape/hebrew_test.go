package shape

import "testing"

// TestHebrewIsSetAsHarfBuzzSetsIt: in a face that positions no marks, a letter
// and its point are composed into the presentation form Unicode excludes from
// composition, where the face has it; a meteg is moved in front of a sheva or
// hiriq after a patah or qamats; and in a face that declares 'mark', nothing is
// composed. The glyphs are the fixture's: 1 alef, 2 patah, 3 alef with patah,
// 4 bet, 5 dagesh, 6 bet with dagesh, 7 qamats, 8 sheva, 9 meteg, 10 shin, 11
// shin dot, 12 shin with dagesh, 13 shin with dagesh and shin dot, 14 hiriq.
func TestHebrewIsSetAsHarfBuzzSetsIt(t *testing.T) {
	checkModelCases(t, "hebrew", []modelCase{
		{"\u05D0\u05B7", "3,610", "alef and patah, composed"},
		{"\u05D1\u05BC", "6,630", "bet and dagesh, composed"},
		{"\u05E9\u05BC\u05C1", "11,0,-90,862 12,710", "shin and dagesh composed; the shin dot is ordered first and blocks nothing"},
		{"\u05D0\u05B8\u05B0\u05BD", "8,0,-100,-2586 9,0,-100,-1724 7,0,-100,-862 1,600", "the meteg moved in front of the sheva"},
		{"\u05D0\u05B7\u05B4\u05BD", "14,0,-95,-1724 9,0,-95,-862 3,610", "the meteg moved in front of the hiriq"},
		{"\u05E9\u05C1", "11,0,-100,862 10,700", "a pair the face has no form for"},
	})
	checkModelCases(t, "hebrew-mark", []modelCase{
		{"\u05D0\u05B7", "2,0 1,600", "a face that declares 'mark' composes nothing"},
		{"\u05D1\u05BC", "5,0 4,620", "nor this"},
		{"\u05D0\u05B8\u05B0\u05BD", "8,0 9,0 7,0 1,600", "and still moves the meteg"},
	})
}
