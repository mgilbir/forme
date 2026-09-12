package layout

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A word is one word however inline boxes cut it, for the scripts whose words
// only a dictionary can find.
//
// CSS Text §5.1: where a script writes no spaces between its words, the line
// breaking is lexical — this engine looks the words up, and DictionaryBreaks is
// where. A segmentation is a statement about a stretch of text rather than about
// one character: "ภาษาไทย" divides after "ภาษา" because of what follows, and no
// walk that has only reached that point can know it.
//
// Each box was segmented on its own, so a word written across a boundary was two
// stretches and the division between them was nobody's. DictionaryBreaks
// excludes the first offset of a run on purpose — a break in front of the first
// character of a run is not one the run offers — and at a box boundary that
// offset is exactly where the break belongs.
//
// Found by FuzzRunTiling as a width, on "ะ๕ะ". The lines are the plainer
// symptom and this tests those: "<span>ภาษา</span><span>ไทย</span>" set one line
// where "ภาษาไทย" sets two, so Thai marked up with so much as a <b> in it lost
// the opportunity at each edge of the span.
func TestAWordIsOneWordHoweverInlineBoxesCutIt(t *testing.T) {
	// Wide enough for one of the two words and not for both.
	const narrow = 40
	for _, tc := range []struct{ what, whole, cut string }{
		{"at the word boundary", "ภาษาไทย", `<span>ภาษา</span><span>ไทย</span>`},
		{"inside the first word", "ภาษาไทย", `<span>ภาษ</span><span>าไทย</span>`},
		{"inside the second", "ภาษาไทย", `<span>ภาษาไ</span><span>ทย</span>`},
		{"three boxes", "ภาษาไทย", `<span>ภา</span><span>ษาไ</span><span>ทย</span>`},
		{"a box apiece", "ภาษาไทย",
			`<span>ภ</span><span>า</span><span>ษ</span><span>า</span>` +
				`<span>ไ</span><span>ท</span><span>ย</span>`},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if len(whole) != 2 {
			t.Fatalf("%q set %d lines %q at %gpx; it is two words and the "+
				"comparison below is against the wrong answer",
				tc.whole, len(whole), whole, float64(narrow))
		}
		if strings.Join(cut, "\x00") != strings.Join(whole, "\x00") {
			t.Errorf("%s: %q set %q and the same text in spans set %q",
				tc.what, tc.whole, whole, cut)
		}
	}
}

// TestABoxDoesNotInventADivisionItCannotSee is the other direction, and the
// half the commit below this one did not do.
//
// That one carried the context *backwards*, so a word written across a boundary
// keeps the division between its halves. This is forwards: segmentWords is
// greedy, so a word that runs past the end of a box is a word the box cannot
// match, and §5.1's fallback then puts a break between every typographic
// character unit of it. A division the text does not have, inside a word.
//
//	DictionaryBreaks("ด๗ไษภหทย") = {3, 6, 9, 12, 15}
//	DictionaryBreaks("ด๗ไษภหท")  = {3, 6, 9, 12, 15, 18}
//
// The lookahead is exact rather than generous — see DictionaryLookahead — and it
// is read off the tree rather than carried, because nothing has walked the text
// after a box by the time the box is flattened. See textAfter.
func TestABoxDoesNotInventADivisionItCannotSee(t *testing.T) {
	const narrow = 40
	for _, tc := range []struct {
		what, whole string
		at          []int
	}{
		{"the word runs one character past the box", "ด๗ไษภหทย", []int{21}},
		{"and two", "ด๗ไษภหทย", []int{18}},
		{"cut twice", "ด๗ไษภหทย", []int{6, 15}},
		{"a box apiece", "ด๗ไษภหทย", []int{3, 6, 9, 12, 15, 18, 21}},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, spanned(tc.whole, tc.at), narrow)
		if strings.Join(cut, "\x00") != strings.Join(whole, "\x00") {
			t.Errorf("%s: %q set %q and the same text cut at %v set %q",
				tc.what, tc.whole, whole, tc.at, cut)
		}
	}
}

// TestAWordBoundaryAtTheBoxEdgeSurvivesTheLookahead is the case the lookahead
// broke and had to be given back, and it is here because it is not obvious.
//
// What one box hands the next is the text since the last word boundary. With no
// lookahead a box never saw a boundary at its own last byte, because
// DictionaryBreaks leaves out the first offset of what it segments and the last
// offset is nobody's. With the lookahead it does see one — and trimming there
// handed the next box nothing, so the next box could not see the boundary at its
// own first character either, and the two halves of "ภาษาไทย" ran together.
//
// So the trim stops strictly before the end. A break *at* the end is the
// boundary itself being a word boundary, and it is the next box's to find.
func TestAWordBoundaryAtTheBoxEdgeSurvivesTheLookahead(t *testing.T) {
	const narrow = 40
	whole := linesOfMarkup(t, "ภาษาไทย", narrow)
	cut := linesOfMarkup(t, `<span>ภาษา</span><span>ไทย</span>`, narrow)
	if len(whole) != 2 {
		t.Fatalf("%q set %d lines %q; it is two words", "ภาษาไทย", len(whole), whole)
	}
	if strings.Join(cut, "\x00") != strings.Join(whole, "\x00") {
		t.Errorf("%q set %q and the same text cut exactly at the word boundary "+
			"set %q", "ภาษาไทย", whole, cut)
	}
}

// TestTheBoundOnWhatTravelsCostsNoBreak is what it says and not the bound.
//
// What one box hands the next is the part-word in progress — the text since the
// last word boundary, inside one script's run — rather than everything before
// it. That bound has no symptom a rendering can show: carrying more text gives
// the same answers, only slower, so nothing here can test it and the test that
// does is paragraph.TestWhatTravelsIsAboutAWord, which asserts the size of what
// travels. A planted widening passes every case below.
//
// What these cases hold is that the bound did not cost the rule. A space, or a
// character of another script, ends the run and the next box starts clean; the
// two spellings still have to agree across it.
func TestTheBoundOnWhatTravelsCostsNoBreak(t *testing.T) {
	const narrow = 40
	for _, tc := range []struct{ what, whole, cut string }{
		{"a space between the words", "ภาษา ไทย", `<span>ภาษา </span><span>ไทย</span>`},
		{"a Latin letter between", "ภาษาxไทย", `<span>ภาษาx</span><span>ไทย</span>`},
		{"the second box begins the run", "xภาษาไทย", `<span>x</span><span>ภาษาไทย</span>`},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if strings.Join(cut, "\x00") != strings.Join(whole, "\x00") {
			t.Errorf("%s: %q set %q and the same text in spans set %q",
				tc.what, tc.whole, whole, cut)
		}
	}
}

// TestTheLookaheadCopiesNoMoreThanItReads is what keeps a bound on a walk that
// can meet a text box of any size.
//
// The walk stops once it has enough, but it appends a box at a time, and a
// single text box can be a megabyte — copying one to read the first eighty bytes
// of it is a cost the document did nothing to ask for. Ending inside a character
// would be free and wrong in a quieter way: DictionaryBreaks would decode the
// half, find nothing, and that looks exactly like a run that ended.
func TestTheLookaheadCopiesNoMoreThanItReads(t *testing.T) {
	for _, tc := range []struct {
		text string
		n    int
		want string
	}{
		{"", 0, ""},
		{"abc", 10, "abc"},
		{"abc", 2, "ab"},
		{"abc", 0, ""},
		// Thai is three bytes a character, so a bound of four holds one of them
		// and a bound of two holds none: half a character is not a character.
		{"ภาษา", 4, "ภ"},
		{"ภาษา", 3, "ภ"},
		{"ภาษา", 2, ""},
		{"ภาษา", 12, "ภาษา"},
		{"aภ", 2, "a"},
	} {
		got := bounded(tc.text, tc.n)
		if got != tc.want {
			t.Errorf("bounded(%q, %d) = %q, want %q", tc.text, tc.n, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("bounded(%q, %d) = %q, which is not valid UTF-8",
				tc.text, tc.n, got)
		}
	}
	if got := bounded(strings.Repeat("ภ", 1<<20), 80); len(got) > 80 {
		t.Errorf("a megabyte of text bounded at 80 bytes gave %d", len(got))
	}
}
