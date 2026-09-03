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

// TestTheOrderOfWhatIsLeftIsTheOrderItWasGiven.
//
// The default list is in the order the specification requires — composition
// before the rules that read its output, required ligatures before optional
// ones, contextual alternates last so they see the glyphs that survived — and
// dropping a tag from the middle must not disturb it.
func TestTheOrderOfWhatIsLeftIsTheOrderItWasGiven(t *testing.T) {
	if got := (Features{}).keeps(afterJoiningFeatures); !slices.Equal(got, afterJoiningFeatures) {
		t.Errorf("with nothing turned off the list is %v, want %v", got, afterJoiningFeatures)
	}
	for _, c := range []struct {
		off  Features
		want []string
	}{
		{Features{NoOptionalLigatures: true}, []string{"rlig", "rclt", "calt"}},
		// The one that catches aliasing, which is why it is here rather than
		// left to the pair above: dropping "calt" from the *middle* moves the
		// two tags after it forward, so a filter that wrote back into the list
		// it was given would leave "liga" where "calt" was and every document
		// afterwards would be shaped with the wrong features.
		{Features{NoContextualAlternates: true},
			[]string{"rlig", "rclt", "liga", "clig"}},
		{Features{NoOptionalLigatures: true, NoContextualAlternates: true},
			[]string{"rlig", "rclt"}},
	} {
		if got := c.off.keeps(afterJoiningFeatures); !slices.Equal(got, c.want) {
			t.Errorf("with %+v the list is %v, want %v", c.off, got, c.want)
		}
	}
	// And none of that wrote back into the list, which is a package-level
	// variable every run of every document reads.
	if want := []string{"rlig", "rclt", "calt", "liga", "clig"}; !slices.Equal(afterJoiningFeatures, want) {
		t.Errorf("filtering changed the default list to %v, want %v",
			afterJoiningFeatures, want)
	}
}
