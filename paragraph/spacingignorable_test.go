package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/segment"
)

// Which characters letter-spacing goes after.
//
// CSS Text §8.2 adds the spacing after each typographic character unit, and a
// character nothing is drawn for is not one. The set was written out here by
// hand — the bidi controls, the joiners, the word joiner and the invisible
// operators — and it had the characters somebody had met: it stopped at U+2064,
// so each of the six deprecated format controls above it collected a spacing of
// its own, and a document using one came out wider than it should be.
//
// It is Unicode's property now, from the table the shaping package already
// generates, so there is one statement of it rather than two.

// TestTheSpacedUnitsAreTheCharactersThatAreDrawn.
func TestTheSpacedUnitsAreTheCharactersThatAreDrawn(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
		what string
	}{
		{"", 0, "nothing"},
		{"abc", 3, "three letters"},
		{"a\u200Bb", 2, "a zero width space between two letters"},
		{"a\u200Cb", 2, "a zero width non-joiner"},
		{"a\u200Db", 2, "a zero width joiner"},
		{"a\u00ADb", 2, "a soft hyphen"},
		{"a\uFEFFb", 2, "a byte order mark"},
		{"a\u2060b", 2, "a word joiner"},
		// The six the hand-written list stopped short of.
		{"a\u206Ab", 2, "inhibit symmetric swapping"},
		{"a\u206Bb", 2, "activate symmetric swapping"},
		{"a\u206Cb", 2, "inhibit Arabic form shaping"},
		{"a\u206Db", 2, "activate Arabic form shaping"},
		{"a\u206Eb", 2, "national digit shapes"},
		{"a\u206Fb", 2, "nominal digit shapes"},
		// And the rest of the property, which the list never had at all.
		{"a\u061Cb", 2, "the Arabic letter mark"},
		{"a\u180Eb", 2, "a Mongolian vowel separator"},
		{"a\uFE0Fb", 2, "a variation selector"},
		{"a\U000E0101b", 2, "a variation selector from plane 14"},
		{"a\U0001D173b", 2, "a musical beam mark"},

		// The Hangul fillers are the exception and are counted: they are
		// letters, written to leave a slot of a syllable empty, and they occupy
		// width on the page. Spacing goes after one as after any other letter.
		{"a\u115Fb", 3, "a choseong filler"},
		{"a\u3164b", 3, "the Hangul filler"},
		{"a\uFFA0b", 3, "a halfwidth filler"},
	} {
		if got := SpacedUnits(tc.text); got != tc.want {
			t.Errorf("%s: %d units, want %d", tc.what, got, tc.want)
		}
	}
}

// TestSpacingIsStillCountedForOrdinaryText is the containment case. The set
// widened, and a set that widened too far would take the spacing off characters
// that are drawn — which is the same fault in the other direction and is the one
// nobody would think to look for.
func TestSpacingIsStillCountedForOrdinaryText(t *testing.T) {
	for _, text := range []string{
		"hello", "ΚΑΛΗΜΕΡΑ", "日本語", "a1!", " ", "\t\n",
		// A letter and a combining acute: two units, and both are drawn.
		"e\u0301",
		// Arabic used to be here and is not, and the reason is not this set.
		// §8.2's cursive tracking takes the spacing off a cursive script
		// whatever is drawn for it — see SpacedUnits and
		// paragraph/cursivetracking_test.go. Leaving it here would have made
		// this test the place a future change to that rule failed, which is
		// three files away from the rule.
	} {
		// Clusters and not code points: "e" with a combining acute on it is one
		// unit, because one grapheme cluster is one typographic character unit.
		// What this test is about is that a character which *is* drawn counts,
		// and segment.Count is the count with nothing taken out.
		if got, want := SpacedUnits(text), segment.Count(text); got != want {
			t.Errorf("%q: %d units, want %d — every character of it is drawn",
				text, got, want)
		}
	}
}

// TestTheSetIsUnicodesAndNotAList guards against the list coming back. Every
// character the property covers must answer yes, and it is asked as a range walk
// rather than as a handful of examples so that a future Unicode release cannot
// quietly leave part of it behind.
func TestTheSetIsUnicodesAndNotAList(t *testing.T) {
	// The Hangul fillers, which this package counts and Unicode's property
	// covers: the one place the two deliberately differ.
	fillers := map[rune]bool{0x115F: true, 0x1160: true, 0x3164: true, 0xFFA0: true}
	// Unicode 17's Default_Ignorable_Code_Point, as ranges. Written out here
	// rather than read from the shaping package's table, so that this is a
	// second statement of the property and not the same one twice.
	ranges := [][2]rune{
		{0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x061C, 0x061C}, {0x115F, 0x1160},
		{0x17B4, 0x17B5}, {0x180B, 0x180F}, {0x200B, 0x200F}, {0x202A, 0x202E},
		{0x2060, 0x206F}, {0x3164, 0x3164}, {0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF},
		{0xFFA0, 0xFFA0}, {0xFFF0, 0xFFF8}, {0x1BCA0, 0x1BCA3},
		{0x1D173, 0x1D17A}, {0xE0000, 0xE0FFF},
	}
	in := map[rune]bool{}
	for _, r := range ranges {
		for c := r[0]; c <= r[1]; c++ {
			in[c] = true
			if got, want := IsDefaultIgnorable(c), !fillers[c]; got != want {
				t.Fatalf("%#04X: IsDefaultIgnorable is %v, want %v", c, got, want)
			}
		}
	}
	// And nothing outside it, sampled across the planes a document uses.
	for _, r := range []rune{'a', 'Z', '0', ' ', '\t', 0x00A0, 0x2010, 0x3000,
		0x0301, 0x05D0, 0x0627, 0x4E00, 0x1F600, 0x10FFFF, 0x2065, 0x1BCA4} {
		if in[r] {
			continue
		}
		if IsDefaultIgnorable(r) {
			t.Errorf("%#04X %q is not default-ignorable and was called one", r, r)
		}
	}
}

// TestSpacedUnitsCountsRunesAndNotBytes, which is what makes the count a count
// of characters for text that is not ASCII.
func TestSpacedUnitsCountsRunesAndNotBytes(t *testing.T) {
	const text = "\u65E5\u672C\u8A9E"
	if len(text) == len([]rune(text)) {
		t.Fatal("the fixture is ASCII and proves nothing")
	}
	if got := SpacedUnits(strings.Repeat(text, 3)); got != 9 {
		t.Errorf("%d units for nine characters", got)
	}
}
