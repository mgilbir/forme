package paragraph

import "testing"

// East Asian punctuation on either side of a segment break.
//
// §4.1.1's sentence about punctuation is written for a mark the Latin script
// *shares*: it asks for East Asian Width Ambiguous — a quotation mark that could
// be either script's — and then for a wide character on the other side, because
// that is the evidence that this one is the East Asian one. A mark that is
// itself Fullwidth, Wide or Halfwidth needs no evidence. "。" and "・" and "｣"
// belong to no other script, so a paragraph hard-wrapped after one of them is
// one sentence however the next line begins.
//
// The suite's three segment-break-transformation-punctuation tests are this and
// nothing else, and each has a reference with its lines run together. They are
// "should" tests citing Gecko bugs rather than a section of the specification,
// which is what a sentence that does not yet cover them looks like from here.

func TestWidePunctuationTakesTheBreakWithoutHelp(t *testing.T) {
	// The far side is a Latin letter, which the Ambiguous sentence would have
	// refused. Every one of these is drawn from the suite's own fixtures.
	for _, tc := range []struct{ text, want, what string }{
		{"。\nInternet", "。Internet", "an ideographic full stop before a Latin word"},
		{"Chrome\n・", "Chrome・", "a katakana middle dot after a Latin word"},
		{"、\nEdge", "、Edge", "an ideographic comma"},
		{"ID\n｢smith｣", "ID｢smith｣", "a halfwidth corner bracket"},
		{"｡\ny/N", "｡y/N", "a halfwidth full stop"},
		{"E\n～", "E～", "a fullwidth tilde, which is a symbol rather than punctuation"},
		{"301C\n〜", "301C〜", "a wave dash"},
	} {
		if got := collapsedIn(tc.text, WritingSystemJapanese); got != tc.want {
			t.Errorf("%s: %q collapsed to %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

func TestTheIdeographicSpaceIsTheOnlyWideSpace(t *testing.T) {
	// The claim widePunctuationAtSegmentBreak rests on when it names U+3000
	// rather than reading a table: no other space separator has an East Asian
	// width the rule is about. Checked against the width data itself, so a
	// regenerated table that changed the answer would say so here.
	spaces := []rune{
		0x0020, 0x00A0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006,
		0x2007, 0x2008, 0x2009, 0x200A,
		0x202F, 0x205F, 0x3000,
	}
	for _, r := range spaces {
		wide := wideAtSegmentBreak(r)
		if want := r == ideographicSpaceRune; wide != want {
			t.Errorf("U+%04X: wideAtSegmentBreak is %v, want %v — the rule names "+
				"U+3000 on the ground that it is the only space separator whose "+
				"East Asian Width is F, W or H", r, wide, want)
		}
	}
}

func TestTheIdeographicSpaceTakesTheBreakWithIt(t *testing.T) {
	// segment-break-transformation-punctuation-003: a line broken after "！　",
	// whose reference runs the two lines together with the one ideographic
	// space between them and no ordinary space beside it.
	if got := collapsedIn("！　\nEnglish", WritingSystemJapanese); got != "！　English" {
		t.Errorf("%q collapsed to %q, want %q", "！　\nEnglish", got, "！　English")
	}
}

func TestTheWidthIsStillRequired(t *testing.T) {
	// Punctuation that is not East Asian does not take the break: an ASCII full
	// stop in a Japanese paragraph is an ASCII full stop.
	if got := collapsedIn(".\na", WritingSystemJapanese); got != ". a" {
		t.Errorf("%q collapsed to %q, want %q — the mark has to be East Asian",
			".\na", got, ". a")
	}
}

func TestTheWritingSystemIsStillRequired(t *testing.T) {
	// The sentence is about Chinese, Japanese and Yi. The same characters in an
	// English paragraph keep their space, which is what makes this a tailoring
	// rather than a rule about the characters alone.
	if got := collapsedIn("。\nInternet", WritingSystemOther); got != "。 Internet" {
		t.Errorf("%q collapsed to %q in a document that is not East Asian, want %q",
			"。\nInternet", got, "。 Internet")
	}
}

func TestAWideLetterIsNotPunctuation(t *testing.T) {
	// The far side of the older sentence is any wide character; the near side of
	// this one is punctuation only. A katakana beside a Latin letter is two
	// words and keeps its space.
	if got := collapsedIn("ア\na", WritingSystemJapanese); got != "ア a" {
		t.Errorf("%q collapsed to %q, want %q — a letter is not punctuation",
			"ア\na", got, "ア a")
	}
}
