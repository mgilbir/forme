package style

import (
	"math/rand"
	"strings"
	"testing"
)

// mightHoldAFontRelativeLength is an optimisation and nothing else: made to
// answer true always, every test in this package still passes. So the only
// thing a test of it can hold is that it answers what it answered before, and
// that is what this does — against the four strings.Contains calls it replaced,
// over the values a computed style actually holds and then over random strings.
//
// It was four scans of every value, and a computed style holds a hundred and
// forty-eight of them, so a small document made nearly six hundred passes over
// a string per element. A profile put the function at seventeen per cent of the
// whole Build.

// containsEMTheOldWay is the expression this replaced, kept as the oracle.
func containsEMTheOldWay(v string) bool {
	return strings.Contains(v, "em") || strings.Contains(v, "EM") ||
		strings.Contains(v, "eM") || strings.Contains(v, "Em")
}

func TestTheEmTestAnswersWhatItAnsweredBefore(t *testing.T) {
	// The values a computed style is made of, including the ones that make this
	// answer true for no length at all — "medium" is the initial border width
	// and is why four properties of every element reach the parse.
	for _, v := range []string{
		"", "e", "m", "em", "EM", "eM", "Em", "medium", "MEDIUM", "1em", "1.5rem",
		"0", "auto", "none", "serif", "Emblem", "1px", "50%", "rgb(1, 2, 3)",
		"calc(1em + 2px)", "translate(1EM)", "\"em\"", "e­s", "ｅｍ",
		"solid", "disc", "inherit", "normal", "currentcolor", "1e5px",
	} {
		if got, want := mightHoldAFontRelativeLength(v), containsEMTheOldWay(v); got != want {
			t.Errorf("mightHoldAFontRelativeLength(%q) = %v, want %v", v, got, want)
		}
	}
	// And over random strings from an alphabet dense in the letters that
	// decide it, which is where a one-pass scan goes wrong if it is going to:
	// an "e" at the very end, an "m" at the very start, "eem", "emm".
	r := rand.New(rand.NewSource(1))
	const alphabet = "eEmM x1."
	disagreements, sawTrue, sawFalse := 0, 0, 0
	for i := 0; i < 200000; i++ {
		n := r.Intn(6)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		v := string(b)
		got, want := mightHoldAFontRelativeLength(v), containsEMTheOldWay(v)
		if got != want {
			if disagreements < 5 {
				t.Errorf("mightHoldAFontRelativeLength(%q) = %v, want %v", v, got, want)
			}
			disagreements++
		}
		if want {
			sawTrue++
		} else {
			sawFalse++
		}
	}
	// Both answers have to be reached, or the agreement is between two
	// functions that always said the same thing.
	if sawTrue < 1000 || sawFalse < 1000 {
		t.Errorf("the random strings gave %d trues and %d falses; the alphabet "+
			"is not exercising both answers", sawTrue, sawFalse)
	}
}
