package layout

import "testing"

// An opportunity a box edge refuses, CSS Text §5.1 and UAX #14 together.
//
// UAX #14 forbids a line beginning with a closing bracket, a hyphen or a
// non-starter, and an opportunity offered in front of one is not one. What the
// rule does with it is *move* it rather than delete it: "× CL" says a line may
// not begin with a closing bracket and says nothing against one beginning with
// what comes after it. SplitAtBreaks holds it forward for that reason inside a
// run — see the heldBreak accounting there — and this is the same rule at a box
// boundary, where the character that takes the opportunity may be two boxes on.
//
// The fixtures are one text written three ways. A <span> is not something a
// reader can see, so all three have to break in the same place; the shapes are
// the ones that put the prohibition in a box of its own, in a box with more
// text after it, and in no box at all.

// heldCSS makes the opportunity after the comma the *only* one there is.
//
// "word-break: keep-all" suppresses the opportunities between ideographs — which
// is what a CJK line usually breaks at — and §5.2 leaves the ones punctuation
// creates alone. So the comma is the only place the text can break, and a
// three-character line either finds it or overflows. Anything that reached the
// wrong answer by filling the line greedily is ruled out too: three characters
// is one short of the four that would fit before the comma's own opportunity.
const heldCSS = `#p { word-break: keep-all; width: 36px }`

// TestAnOpportunityRefusedAtABoxEdgeIsOfferedAfterTheProhibition is the rule.
func TestAnOpportunityRefusedAtABoxEdgeIsOfferedAfterTheProhibition(t *testing.T) {
	want := []string{"中中、", "中中"}
	for _, tc := range []struct{ markup, what string }{
		{"中中、中中", "written plainly"},
		{"中中<span>、</span>中中", "the comma in a box of its own"},
		{"中中<span>、中</span>中", "the comma in a box with more after it"},
		{"中中<span>、</span><span>中</span>中", "a box apiece"},
	} {
		got := cjkLines(t, tc.markup, heldCSS)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%s: %q broke as %q, want %q — the opportunity the comma "+
				"stands in front of is the one after it, whichever box the "+
				"character that takes it is written in",
				tc.what, tc.markup, got, want)
		}
	}
}

// TestALineStillMayNotBeginWithTheProhibition is the containment case: the
// opportunity is moved, not granted where it was refused.
//
// Two characters of room and the comma at the third. If the refusal were dropped
// rather than held, the comma would begin the second line — which is the one
// thing UAX #14's rule is there to prevent.
func TestALineStillMayNotBeginWithTheProhibition(t *testing.T) {
	for _, markup := range []string{
		"中中、中中",
		"中中<span>、</span>中中",
		"中中<span>、中</span>中",
	} {
		for _, line := range cjkLines(t, markup, `#p { word-break: keep-all; width: 24px }`) {
			if len(line) > 0 && string([]rune(line)[0]) == "、" {
				t.Errorf("%q gave a line beginning with the comma: %q",
					markup, cjkLines(t, markup, `#p { word-break: keep-all; width: 24px }`))
				break
			}
		}
	}
}
