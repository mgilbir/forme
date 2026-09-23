package shape

import (
	"slices"
	"testing"
)

// Which of a font's own rules each switch turns off.
//
// It is a classification and the classification is the claim, so it is asserted
// directly rather than through a face: a font that happened to state one of
// these features would observe the answer, and a font that did not would say
// nothing either way. What matters is which tags are in which set, and that is
// written here.

// TestEachSwitchNamesItsOwnSet.
func TestEachSwitchNamesItsOwnSet(t *testing.T) {
	for _, c := range []struct {
		what string
		off  Features
		gone []string
	}{
		{"nothing", Features{}, nil},
		// CSS Text §8.2's "optional ligatures": the ones a font offers rather
		// than the ones a script requires.
		{"the optional ligatures", Features{NoOptionalLigatures: true},
			[]string{"liga", "clig", "dlig", "hlig"}},
		// CSS Fonts 4 §6.4's "no-contextual", which is one of the four things
		// "font-variant-ligatures: none" expands to.
		{"the contextual alternates", Features{NoContextualAlternates: true},
			[]string{"calt"}},
		{"both", Features{NoOptionalLigatures: true, NoContextualAlternates: true},
			[]string{"liga", "clig", "dlig", "hlig", "calt"}},
		// Kerning is positioning and no substitution tag belongs to it.
		{"kerning", Features{NoKerning: true}, nil},
	} {
		for _, tag := range []string{"liga", "clig", "dlig", "hlig", "calt",
			"rlig", "rclt", "ccmp", "locl", "kern"} {
			want := slices.Contains(c.gone, tag)
			if got := c.off.suppresses(tag); got != want {
				t.Errorf("%s: suppresses(%q) = %v, want %v", c.what, tag, got, want)
			}
		}
	}
}

// TestTheRequiredLigaturesCannotBeTurnedOff.
//
// "rlig" is not among them and nothing here can reach it. A lam-alef in Arabic
// is not an embellishment: the pair is written as one letter, and setting it as
// two is not the word. CSS Fonts 4 does not give font-variant-ligatures a value
// that turns them off either.
func TestTheRequiredLigaturesCannotBeTurnedOff(t *testing.T) {
	every := Features{NoOptionalLigatures: true, NoContextualAlternates: true,
		NoKerning: true}
	for _, tag := range []string{"rlig", "rclt", "ccmp", "locl"} {
		if every.suppresses(tag) {
			t.Errorf("%q is suppressed with everything turned off; it is required "+
				"and no switch here names it", tag)
		}
	}
}

// TestWhatIsTurnedOffIsOutOfThePlanAndNothingElse.
//
// A switch takes its features out of every stage and leaves the rest of the
// plan as it was: the required ligatures and the contextual alternates stay
// when only the optional ligatures go, and the ligatures stay when only the
// alternates go. And a plan built with something turned off leaves the next
// plan alone — the lists every plan is built from are package-level, and a
// build that wrote back into one would shape every document afterwards with the
// wrong features.
func TestWhatIsTurnedOffIsOutOfThePlanAndNothingElse(t *testing.T) {
	// A layout that names one lookup per feature, so that which lookups are in
	// a plan says exactly which features are.
	l := &layout{featureLookups: map[string][]int{
		"ccmp": {0}, "locl": {1}, "rlig": {2}, "rclt": {3},
		"calt": {4}, "liga": {5}, "clig": {6}, "dlig": {7},
	}}
	inPlan := func(off Features) map[int]bool {
		out := map[int]bool{}
		for _, st := range buildPlan(l, planKey{model: modelDefault, features: off}, nil).stages {
			for _, lk := range st {
				out[lk.index] = true
			}
		}
		return out
	}
	set := func(idx ...int) map[int]bool {
		out := map[int]bool{}
		for _, i := range idx {
			out[i] = true
		}
		return out
	}
	optional, alternates, kept := set(5, 6), set(4), set(0, 1, 2, 3)
	for _, c := range []struct {
		off  Features
		want map[int]bool
	}{
		{Features{}, union(optional, alternates, kept)},
		{Features{NoOptionalLigatures: true}, union(alternates, kept)},
		{Features{NoContextualAlternates: true}, union(optional, kept)},
		{Features{NoOptionalLigatures: true, NoContextualAlternates: true}, kept},
		// And the plain plan once more, after all of that.
		{Features{}, union(optional, alternates, kept)},
	} {
		got := inPlan(c.off)
		for idx := range union(got, c.want) {
			if got[idx] != c.want[idx] {
				t.Errorf("with %+v lookup %d in the plan is %v, want %v", c.off, idx, got[idx], c.want[idx])
			}
		}
	}
}

func union(sets ...map[int]bool) map[int]bool {
	out := map[int]bool{}
	for _, s := range sets {
		for k := range s {
			out[k] = true
		}
	}
	return out
}
