package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §8.1's gap and characters that stand upright.
//
// The spacing is between an ideograph and a *non-ideographic* letter or number.
// A character typeset upright in vertical text is set the way an ideograph is —
// standing as it does in the code charts, one em to the next, which is what
// Item.Upright is about — so the boundary the property spaces is not there and
// neither is the spacing.
//
// The suite writes it as text-autospace-vertical-upright-001, whose reference is
// the same four lines with "text-autospace: no-autospace" on them. That document
// needs something else as well — it puts text-orientation on descendants of the
// box that turns, and this engine gives a turned subtree one orientation — so
// the case asserted here is the one it can reach: the orientation on the turned
// box itself.

// uprightAutospaceReach is how far a turned box's line runs, which in a vertical
// mode is down the page.
func uprightAutospaceReach(t *testing.T, extra string) style.Unit {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">国X国</div>`, noDefaults+
		`#d { writing-mode: vertical-rl; text-orientation: upright;
		      font-size: 20px; `+extra+` }`)
	var end style.Unit
	for _, line := range find(t, root, "d").Lines {
		for _, r := range line.Runs {
			if e := r.X.Add(r.Width); e > end {
				end = e
			}
		}
	}
	return end
}

// TestAnUprightRunTakesNoIdeographSpacing.
//
// The comparison is against the same box with the property turned off, because
// what is asserted is that the property changes nothing here — an absolute
// number would be asserting the advance of an upright Latin capital, which is
// not what this is about.
func TestAnUprightRunTakesNoIdeographSpacing(t *testing.T) {
	on := uprightAutospaceReach(t, `text-autospace: normal`)
	off := uprightAutospaceReach(t, `text-autospace: no-autospace`)
	if on != off {
		t.Errorf("with \"text-autospace: normal\" the upright line runs %v and "+
			"with it off %v; every character on it stands the way an ideograph "+
			"does, so there is no ideograph-to-letter boundary for §8.1 to space",
			on, off)
	}

	// And the same text laid out across the page *does* take the spacing, so
	// what the test above watches is the upright setting and not a fixture that
	// never had a gap in it.
	across := func(extra string) style.Unit {
		t.Helper()
		root := layoutOf(t, 600, `<div id="d">国X国</div>`,
			noDefaults+`#d { font-size: 20px; `+extra+` }`)
		var end style.Unit
		for _, line := range find(t, root, "d").Lines {
			for _, r := range line.Runs {
				if e := r.X.Add(r.Width); e > end {
					end = e
				}
			}
		}
		return end
	}
	if across(`text-autospace: normal`) == across(`text-autospace: no-autospace`) {
		t.Fatal("the same text set across the page takes no spacing either, so " +
			"this fixture cannot tell the rule from the absence of one")
	}
}
