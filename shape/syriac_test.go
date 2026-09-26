package shape

import "testing"

// TestSyriacJoinsAsHarfBuzzJoinsIt: the Alaph's three forms of its own, from
// the joining scan's states — the second final form after a letter that does
// not join forward, the third after a Dalath, and the second medial where a
// letter follows a final Alaph. The glyphs are the fixture's: 1 Alaph, 2 Beth,
// 3 Dalath, 4 to 7 the Alaph's fina, fin2, fin3 and med2, 8 to 10 the Beth's
// init, medi and fina. Drawn right to left.
func TestSyriacJoinsAsHarfBuzzJoinsIt(t *testing.T) {
	checkModelCases(t, "syriac-alaph", []modelCase{
		{"\u0710", "1,400", "an Alaph alone"},
		{"\u0712\u0710", "4,410 8,510", "after a Beth, the ordinary final form"},
		{"\u0715\u0710", "6,430 3,450", "after a Dalath, fin3"},
		{"\u0710\u0710", "5,420 1,400", "after an Alaph, fin2"},
		{"\u0712\u0710\u0712", "2,500 7,440 8,510", "a letter after a final Alaph makes it med2"},
		{"\u0712\u0712\u0712", "10,530 9,520 8,510", "three Beths"},
		{"\u0710\u0710\u0710", "5,420 1,400 1,400", "fin2 after fin2 is isolated"},
		{"\u0712\u0710\u0710", "5,420 7,440 8,510", "an Alaph after med2"},
		// The marks of a Syriac letter are ordered as an Arabic letter's are:
		// the hamza against the letter, before a mark of its class written
		// first. Glyphs 12 and 13 are U+0654 and U+0742.
		{"\u0712\u0654\u0742", "13,0,-150,-862 12,0,-150,862 2,500", "hamza above, then a mark below"},
		{"\u0712\u0742\u0654", "13,0,-150,-862 12,0,-150,862 2,500", "the same, written the other way"},
	})
}

// TestTheAbbreviationMarkStretchesOverItsWord: 'stch' takes U+070F apart into
// a fixed piece (3, 100 wide), a repeating one (4, 40) and another fixed one
// (5, 120), and the pieces are drawn over the word before the mark, with no
// advance of their own, the repeating one as often as the word's width takes.
// A mark over nothing, or over a word no wider than its fixed pieces, is drawn
// as the pieces once. The word is what follows the mark as it is written, up to
// what is not a letter, and the glyphs a substitution made of its letters: 7
// is a Gamal 'ccmp' takes apart into two of 8, 9 a Heth two of which 'rlig'
// joins into 10, and 6 a space.
func TestTheAbbreviationMarkStretchesOverItsWord(t *testing.T) {
	checkModelCases(t, "syriac-stch", []modelCase{
		{"\u070F\u0712 \u0712", "1,500 6,250 1,500 5,0,-360,0 4,0,-240,0 4,0,-200,0 4,0,-160,0 4,0,-120,0 4,0,-80,0 4,0,-40,0 4,0 3,0,40,0",
			"over one Beth, and not past the space"},
		{"\u070F\u0713", "8,260 8,260 5,0,-526,0 4,0,-406,0 4,0,-368,0 4,0,-330,0 4,0,-292,0 4,0,-254,0 4,0,-216,0 4,0,-178,0 4,0,-140,0 3,0,-100,0",
			"over the two pieces of a Gamal, drawn a little closer to fit"},
		{"\u070F\u0717\u0717", "10,700 5,0,-460,0 4,0,-340,0 4,0,-300,0 4,0,-260,0 4,0,-220,0 4,0,-180,0 4,0,-140,0 4,0,-100,0 4,0,-60,0 4,0,-20,0 4,0,20,0 4,0,60,0 4,0,100,0 3,0,140,0",
			"over the ligature of two Heths"},
		{"\u0712\u070F", "5,0,-370,0 4,0,-250,0 3,0,-210,0 1,500", "over one Beth"},
		{"\u0712\u0712\u070F\u0712", "1,500 5,0,-360,0 4,0,-240,0 4,0,-200,0 4,0,-160,0 4,0,-120,0 4,0,-80,0 4,0,-40,0 4,0 3,0,40,0 1,500 1,500",
			"over two Beths, the repeating piece seven times"},
		{"\u0712\u0712\u0712\u0712\u070F", "5,0,-370,0 4,0,-250,0 3,0,-210,0 1,500 1,500 1,500 1,500",
			"at the end of a word, over nothing after it"},
		{"\u070F", "5,0,-370,0 4,0,-250,0 3,0,-210,0", "alone"},
	})
}
