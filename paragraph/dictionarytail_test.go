package paragraph

import (
	"strings"
	"testing"
)

// What one stretch of text hands the next, for a script whose words a
// dictionary finds, is the part-word in progress and not the whole run.
//
// Both halves of that are claims and both are tested here. The first is
// correctness and is asserted at the top of the engine, in
// layout/dictionaryboundary_test.go: a word is one word however inline boxes cut
// it. The second is the bound, and it has no symptom a rendering can show —
// carrying more text gives the *same* answers, just slower, so no comparison
// between two spellings can see it. What it costs is time: two thousand Thai
// words in two thousand spans took 845ms with the whole run carried and 174ms
// with this, because each box re-segmented everything in front of it.
//
// So the bound is asserted directly, on the size of what travels.
func TestWhatTravelsIsAboutAWord(t *testing.T) {
	const word = "ภาษาไทย"
	// Long enough that a tail proportional to the text is unmistakable.
	text := strings.Repeat(word, 500)
	tail := dictionaryTail(text, DictionaryBreaks(text))
	if len(tail) > len(word) {
		t.Errorf("%d words of Thai hand on %d bytes; the segmentation is greedy "+
			"and runs left to right, so everything before the last word "+
			"boundary has been decided and cannot change what follows it — "+
			"about one word should travel, not %d", 500, len(tail), len(tail))
	}
	if tail == "" {
		t.Errorf("%d words of Thai hand on nothing; the text ends mid-run, so "+
			"the word in progress has to travel or the next box begins a run "+
			"the dictionary never saw the start of", 500)
	}
}

// TestWhatTravelsStopsAtAnythingTheDictionaryWouldNotSegment is the other bound,
// and the one that keeps a document with no such script in it paying nothing.
//
// DictionaryBreaks segments a maximal stretch of one script and stops at the
// first character of another, so nothing beyond such a stop can change how the
// text after it divides. A space ends the run as surely as a Latin letter does.
func TestWhatTravelsStopsAtAnythingTheDictionaryWouldNotSegment(t *testing.T) {
	for _, tc := range []struct{ what, text, want string }{
		{"ends in a space", "ภาษาไทย ", ""},
		{"ends in Latin", "ภาษาไทยx", ""},
		{"no such script at all", "hello world", ""},
		{"empty", "", ""},
		{"a space in the middle", "ภาษาไทย ไทย", "ไทย"},
		{"Latin in the middle", "ภาษาxไทย", "ไทย"},
	} {
		got := dictionaryTail(tc.text, DictionaryBreaks(tc.text))
		if got != tc.want {
			t.Errorf("%s: %q hands on %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}
