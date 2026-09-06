package paragraph

import (
	"strings"
	"testing"
	"time"
)

// Two walks that were quadratic in the length of one run, both of them because
// a question with the same answer every time was asked once per character.
//
// The bound is wall clock, which is the right assertion when the quantity under
// test is time and the margin is three orders of magnitude. Each of these took
// minutes on an input a document can hold and takes milliseconds now.

// TestCollapsingIsLinearInARunOfUndrawnCharacters is §4.1.1's first phase.
//
// The rule that decides what a collapsed run of white space comes out as needs
// the first character a *reader* would see after it, which means scanning past
// everything that is not drawn. That scan was made for every character rather
// than for every run: on a megabyte of soft hyphens each one scanned past all
// the rest, which the audit measured at about fifty minutes.
func TestCollapsingIsLinearInARunOfUndrawnCharacters(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"soft hyphens", " " + strings.Repeat("­", 60000)},
		{"zero-width joiners", " " + strings.Repeat("‍", 60000)},
		{"soft hyphens between spaces", strings.Repeat(" ­", 30000)},
		{"bidi controls", " " + strings.Repeat("‪", 60000)},
	} {
		start := time.Now()
		CollapseWhitespace(tc.text, "collapse", WordSpaceTransform{})
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("collapsing %d characters of %s took %v; it is linear work",
				len(tc.text), tc.name, elapsed)
		}
	}
}

// TestUprightUnitsIsLinearInTheRun is the same shape in the vertical pass: the
// cluster boundaries of the whole string were found again for every cluster,
// which is the same answer every time.
func TestUprightUnitsIsLinearInTheRun(t *testing.T) {
	text := strings.Repeat("あ", 30000)
	start := time.Now()
	got := UprightUnits(text)
	elapsed := time.Since(start)
	if got != 30000 {
		t.Fatalf("counted %d units, want 30000", got)
	}
	if elapsed > 5*time.Second {
		t.Errorf("counting %d clusters took %v; the boundaries are found once",
			got, elapsed)
	}
}

// TestCollapsingStillCollapses is what the scans are for, kept: a run of white
// space becomes one space, and the character a reader sees after it still
// decides what happens to a segment break.
func TestCollapsingStillCollapses(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a run of spaces", "a   b", "a b"},
		{"a tab", "a\tb", "a b"},
		{"a newline", "a\nb", "a b"},
		{"spaces around a newline", "a  \n  b", "a b"},
		{"a soft hyphen after a space", "a ­b", "a ­b"},
		{"an ideograph either side of a break", "社\n福", "社福"},
		{"an ideograph seen past a variation selector", "社︀\n福︀", "社︀福︀"},
	} {
		if got := CollapseWhitespaceAfter(tc.in, "collapse", WordSpaceTransform{}, Boundary{}, WritingSystemJapanese); got != tc.want {
			t.Errorf("%s: %q collapsed to %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
