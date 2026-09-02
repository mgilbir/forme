package paragraph

import "testing"

// The two questions §8.2 asks about a character nothing is drawn for.
//
// Both are about the same rule from opposite ends — SpacedUnits counts the
// characters a spacing follows, and these two say *which* they are and what it
// means when a run has none of them.
func TestACharacterNothingIsDrawnForCarriesNoLetterSpacing(t *testing.T) {
	const zw = "\u200B\u200C\u200D\uFEFF\u2060"
	for _, c := range []struct {
		text string
		all  bool
		what string
	}{
		{"", false, "no text at all is an inline box's edge, not a run of controls"},
		{zw, true, "a run of nothing but formatting characters"},
		{"x" + zw, false, "a letter with formatting characters behind it"},
		{zw + "x", false, "a letter with formatting characters in front of it"},
		{"xx", false, "two letters"},
		{" ", false, "a space is a character, and a spacing goes after one"},
	} {
		if got := AllIgnorable(c.text); got != c.all {
			t.Errorf("AllIgnorable(%q) = %v, want %v: %s", c.text, got, c.all, c.what)
		}
	}
	// And which of a run's characters a spacing follows, which is what places
	// each glyph rather than measuring the whole.
	after := SpacingAfter("x" + zw + "y")
	if len(after) != len([]rune("x"+zw+"y")) {
		t.Fatalf("SpacingAfter returned %d answers for %d runes",
			len(after), len([]rune("x"+zw+"y")))
	}
	if !after[0] || !after[len(after)-1] {
		t.Error("no spacing goes after either letter of \"x...y\"")
	}
	for i, spaced := range after[1 : len(after)-1] {
		if spaced {
			t.Errorf("a spacing goes after formatting character %d, which is not a "+
				"typographic character unit", i)
		}
	}
	// The count and the placement are the same rule and have to agree.
	n := 0
	for _, spaced := range after {
		if spaced {
			n++
		}
	}
	if n != SpacedUnits("x"+zw+"y") {
		t.Errorf("SpacingAfter says %d characters carry a spacing and SpacedUnits "+
			"says %d; they are the same question", n, SpacedUnits("x"+zw+"y"))
	}
}
