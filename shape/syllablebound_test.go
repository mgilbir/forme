package shape

import "testing"

// TestALongMyanmarSyllableIsCutAtTheBound is the bound on how long one syllable
// may be.
//
// A syllable is scanned forward from a letter and a text node is untrusted, so
// a run of a thousand marks on one consonant is one syllable as far as the
// scanner is concerned — and every later stage works on a syllable at a time.
// The bound cuts it, and the comment above myanmarSyllables says what has to
// survive the cut: "The result covers the input exactly and in order, and every
// entry is at least one character long, so a caller walking it always makes
// progress."
//
// Nothing had written one: every Myanmar fixture here is a syllable of a few
// characters, so the shape suite passes with maxIndicSyllable raised.
func TestALongMyanmarSyllableIsCutAtTheBound(t *testing.T) {
	// One consonant carrying far more marks than the bound allows. A literal
	// rather than maxIndicSyllable+n, so that raising the bound does not grow
	// the input along with it.
	const marks = 500
	cats := make([]indicCat, 0, marks+1)
	cats = append(cats, catConsonant)
	for i := 0; i < marks; i++ {
		cats = append(cats, catSM)
	}

	got := myanmarSyllables(cats)
	if len(got) == 0 {
		t.Fatal("a consonant and five hundred marks produced no syllables at all")
	}

	// The three properties the comment promises, which the cut must not break.
	if got[0].start != 0 {
		t.Errorf("the first syllable starts at %d", got[0].start)
	}
	for i, s := range got {
		if s.end <= s.start {
			t.Fatalf("syllable %d is %d..%d, which is empty; a caller walking these "+
				"would not make progress", i, s.start, s.end)
		}
		if i > 0 && s.start != got[i-1].end {
			t.Errorf("syllable %d starts at %d and the one before it ended at %d; "+
				"the result has to cover the input exactly and in order",
				i, s.start, got[i-1].end)
		}
	}
	if last := got[len(got)-1]; last.end != len(cats) {
		t.Errorf("the syllables end at %d of %d characters", last.end, len(cats))
	}

	// And the cut itself: no syllable is longer than the bound, so one run of
	// marks becomes several syllables rather than one enormous one.
	for i, s := range got {
		if n := s.end - s.start; n > maxIndicSyllable {
			t.Errorf("syllable %d is %d characters long and the bound is %d",
				i, n, maxIndicSyllable)
		}
	}
	if len(got) == 1 {
		t.Errorf("a consonant and %d marks came out as one syllable; past the bound "+
			"of %d it is cut", marks, maxIndicSyllable)
	}
}
