package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/segment"
)

// CSS Text §2's typographic character unit, which is a grapheme cluster.
//
// Every rule in this package that counts "characters" counts these: §8.2's
// letter-spacing goes between them, §7.3's inter-character justification
// distributes between them, and §4.4's upright vertical advance is one em each.
// For a long time they were counted as code points, and a code point is not a
// unit: a Thai letter carries its vowel sign and its tone mark, a Khmer
// consonant carries the vowel that follows it, and a spacing inserted between a
// base and a mark that belongs to it moves the mark off the letter it is drawn
// on.

// TestAClusterIsOneUnit.
func TestAClusterIsOneUnit(t *testing.T) {
	for _, c := range []struct {
		what, text string
		units      int
	}{
		{"three Latin letters", "abc", 3},
		{"a letter and a combining acute", "e\u0301", 1},
		{"a letter with two marks", "e\u0301\u0308", 1},
		{"a Thai letter, a vowel sign and a tone mark", "\u0e01\u0e34\u0e48", 1},
		{"the same followed by another letter", "\u0e01\u0e34\u0e48\u0e02", 2},
		{"two Khmer consonants each with a vowel", "\u179f\u17b6\u17a0\u17b6", 2},
		{"a Bengali conjunct", "\u0995\u09cd\u09af", 1},
		{"a Hangul syllable written as jamo", "\u1100\u1161\u11a8", 1},
	} {
		if got := SpacedUnits(c.text); got != c.units {
			t.Errorf("%s: %d units, want %d", c.what, got, c.units)
		}
		// The same count, asked the way the upright vertical advance asks it.
		// The two rules are one definition and would be a bug apart.
		if got := UprightUnits(c.text); got != c.units {
			t.Errorf("%s: %d upright units, want %d — it is the same unit",
				c.what, got, c.units)
		}
		// And the same count again, from the offsets the spacing is placed at.
		if got := len(SpacingAfterOffsets(c.text)); got != c.units {
			t.Errorf("%s: the spacing is placed at %d offsets, want %d",
				c.what, got, c.units)
		}
	}
}

// TestTheSpacingGoesAfterTheWholeCluster.
//
// Which character it goes after is the half that decides where the marks land:
// after the *last* of the cluster, so that nothing is inserted between a letter
// and what is written on it.
func TestTheSpacingGoesAfterTheWholeCluster(t *testing.T) {
	// A Thai letter with a vowel sign and a tone mark, then another letter.
	const text = "\u0e01\u0e34\u0e48\u0e02"
	after := SpacingAfter(text)
	want := []bool{false, false, true, true}
	if len(after) != len(want) {
		t.Fatalf("SpacingAfter gave %d answers for %d runes", len(after), len(want))
	}
	for i := range want {
		if after[i] != want[i] {
			t.Errorf("rune %d: spacing after = %v, want %v — the spacing goes after "+
				"the tone mark and not between the letter and its vowel",
				i, after[i], want[i])
		}
	}
	// The byte-offset form has to agree with it, because the comparison places
	// the marks from one and the engine measures the run from the other.
	offsets := SpacingAfterOffsets(text)
	n := 0
	for i := range text {
		if !after[n] != !offsets[i] {
			t.Errorf("rune at byte %d: SpacingAfter says %v and SpacingAfterOffsets "+
				"says %v", i, after[n], offsets[i])
		}
		n++
	}
}

// TestTheCursiveRuleStillCutsBeforeAUnit.
//
// The scan reports two places for every unit now — where it starts and where it
// ends — because the two rules that read it want different ones: the spacing
// goes after a unit's last character and the run is cut before its first. Using
// one for the other put an Arabic letter in the Latin piece.
func TestTheCursiveRuleStillCutsBeforeAUnit(t *testing.T) {
	// A Latin letter with an accent, an Arabic letter with a fathatan, a letter.
	const text = "a\u0301\u0645\u064bb"
	parts := SplitAtCursiveTracking(text)
	want := []string{"a\u0301", "\u0645\u064b", "b"}
	if len(parts) != len(want) {
		t.Fatalf("%q was cut into %q, want %q", text, parts, want)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Errorf("piece %d is %q, want %q — the cut goes before the first "+
				"character of the unit whose answer differs", i, parts[i], want[i])
		}
	}
}

// TestTheUnitIsUnicodesAndNotThisPackages.
//
// The clustering is segment's, which is UAX #29 with the tailoring CSS asks for,
// and this is the check that nothing here counts its own way. Every string is
// asked of both.
func TestTheUnitIsUnicodesAndNotThisPackages(t *testing.T) {
	for _, text := range []string{
		"abc", "e\u0301", "\u0e01\u0e34\u0e48", "\u0995\u09cd\u09af",
		"\u1100\u1161\u11a8", "\U0001f1ec\U0001f1e7", "a\u200db",
	} {
		if got, want := SpacedUnits(text), segment.Count(text); got != want {
			t.Errorf("%q: %d units here and %d clusters in segment", text, got, want)
		}
	}
}
