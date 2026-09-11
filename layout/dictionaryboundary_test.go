package layout

import (
	"strings"
	"testing"
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
