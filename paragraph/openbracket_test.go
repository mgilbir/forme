package paragraph

import "testing"

// LB14: a line may not end after an opening bracket.
//
// The bracket belongs to what it opens, and a line ending at one leaves it in
// the margin with nothing behind it. Ordinary text never asks — it offers no
// opportunity there for the rule to forbid — but "word-break: break-all" offers
// one at every character boundary in a word, and §5.2 does not reach past the
// punctuation rules to take it. word-break-break-all-020 says so in its own
// assertion: "break-all does not affect rules governing the soft wrap
// opportunities created by punctuation". It writes "あい）あ（い" three times
// over, once with break-all, and asks for all three to break at the same points.

func TestALineMayNotEndAfterAnOpeningBracket(t *testing.T) {
	for _, tc := range []struct {
		prev, next rune
		what       string
	}{
		{'（', 'い', "the suite's own fullwidth parenthesis"},
		{'(', 'a', "an ASCII parenthesis"},
		{'「', 'あ', "a corner bracket"},
		{'[', 'a', "a square bracket"},
		{'｛', 'あ', "a fullwidth brace"},
	} {
		if !gluedPair(tc.prev, tc.next) {
			t.Errorf("%s: a line may end between %q and %q, and LB14 says it may not",
				tc.what, tc.prev, tc.next)
		}
	}
}

func TestTheClosingBracketIsTheOtherTablesJob(t *testing.T) {
	// LB13 forbids a line to *begin* with a closing bracket, and that is the
	// base table's answer rather than this one's. Asked the other way round —
	// may a line end after a closing bracket — the answer is yes.
	if gluedPair('）', 'あ') {
		t.Errorf("a line may not end after a closing bracket; LB13 is about " +
			"beginning with one and noBreakBefore is where it lives")
	}
	if !noBreakBefore('）', LineBreak{}) {
		t.Errorf("a line may begin with a closing bracket, and LB13 says it may not")
	}
}

func TestAnOrdinaryPairIsStillFree(t *testing.T) {
	// Nothing else changed: break-all still breaks between two letters and
	// between two ideographs.
	for _, tc := range []struct{ prev, next rune }{
		{'a', 'b'}, {'あ', 'い'}, {'）', '（'}, {'1', '2'},
	} {
		if gluedPair(tc.prev, tc.next) {
			t.Errorf("a line may not end between %q and %q, and nothing forbids it",
				tc.prev, tc.next)
		}
	}
}

func TestTheTableIsUnicodesOwn(t *testing.T) {
	// A spot check against LineBreak.txt's class OP rather than against the
	// generated ranges, so that a table regenerated from a later Unicode is
	// still checked against what the class means.
	for _, r := range []rune{0x0028, 0x005B, 0x007B, 0x2985, 0x3008, 0xFF08, 0xFF5B} {
		if !inLineBreakRanges(r, openRanges[:]) {
			t.Errorf("U+%04X is class OP and is not in openRanges", r)
		}
	}
	for _, r := range []rune{0x0029, 0x005D, 0x0061, 0x3042, 0xFF09} {
		if inLineBreakRanges(r, openRanges[:]) {
			t.Errorf("U+%04X is not class OP and is in openRanges", r)
		}
	}
}
