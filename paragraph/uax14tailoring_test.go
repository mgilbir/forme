package paragraph

import "testing"

// What this engine does differently from UAX #14 because CSS Text says so, one
// rule at a time. linebreakconformance_test.go holds the engine to
// LineBreakTest.txt with these as its allowances; these say each allowance is
// taken where it should be and nowhere else, which the file cannot, because it
// has no language, no word-break and no white-space in it.

// bar is SplitAtBreaks' answer written into the text: "|" where a line may break.
// A break word-break demotes rather than allows is written "¦".
func bar(text string, ws WhiteSpace, wb WordBreak, lb LineBreak) string {
	pieces, _ := SplitAtBreaks(text, ws, wb, lb, Hyphens{}, WritingSystemOther)
	out := ""
	for i, p := range pieces {
		switch {
		case i > 0 && p.BreakBefore && p.LastResort:
			out += "¦"
		case i > 0 && p.BreakBefore:
			out += "|"
		}
		out += p.Text
	}
	return out
}

var collapsing = WhiteSpace{Collapse: true, Wrap: true}

// TestTheNotesDeviationsAreTaken is CSS Text 3's note to line-break, which
// lists four deviations from UAX #14 as "desirable for maximum interoperability
// with existing implementations".
func TestTheNotesDeviationsAreTaken(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"a!important", "a!important", "no break between U+0021 and a letter"},
		{"23/Jan", "23/Jan", "nor between U+002F and one"},
		{"a|b", "a|b", "nor between U+007C and one"},
		{"!1", "!|1", "UAX #14's break stands before what is not a letter"},
		{"ABCD-1234", "ABCD-|1234", "a hyphen between letters and digits breaks"},
		{"1234-5678", "1234-|5678", "and between digits and digits"},
		{"a -13", "a |-13", "a minus sign does not"},
	} {
		if got := bar(tc.text, collapsing, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
	// And a letter break-all makes an ideograph is not one for this purpose:
	// word-break-break-all-028 breaks "XXX/X" after the slash.
	if got := bar("XXX/X", collapsing, WordBreak{BreakAll: true}, LineBreak{}); got != "X|X|X/|X" {
		t.Errorf("under break-all: %q, want %q", got, "X|X|X/|X")
	}
}

// TestADictionaryScriptBreaksOnlyInsideItself is §5's lexical breaking, which
// is "between pairs of typographic letter units in that writing system": a
// digit in front of New Tai Lue is not one of a pair, and UAX #14's NU × AL
// stands. New Tai Lue has no word list here, so between two of its letters the
// fallback breaks.
func TestADictionaryScriptBreaksOnlyInsideItself(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"0ᦤ", "0ᦤ"},
		{"ᦤᦤ", "ᦤ|ᦤ"},
		{"aᦤᦤ", "aᦤ|ᦤ"},
	} {
		if got := bar(tc.text, collapsing, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestAMarkAfterASpaceBeginsItsOwnUnit. UAX #29 puts a combining mark or a
// joiner in the cluster of the space before it; UAX #14 makes it a letter of
// its own (LB10), and the line breaks at the space. white-space-vs-joiners-002
// is the shape.
func TestAMarkAfterASpaceBeginsItsOwnUnit(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"a ‍is", "a |‍is"},
		{"a ́b", "a |́b"},
	} {
		if got := bar(tc.text, collapsing, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%+q: %+q, want %+q", tc.text, got, tc.want)
		}
	}
}

// TestBreakSpacesBreaksAfterEveryOtherSpaceSeparator. "A soft wrap opportunity
// exists after every preserved white space character and after every other
// space separator (including between adjacent spaces)" — where UAX #14 keeps two
// ideographic spaces together (× BA). The two no-break ones keep what follows,
// as the suite's trailing-other-space-separators-break-spaces-009 and -013 ask.
func TestBreakSpacesBreaksAfterEveryOtherSpaceSeparator(t *testing.T) {
	bs := WhiteSpace{Wrap: true, PreserveBreaks: true, BreakSpaces: true}
	pre := WhiteSpace{Wrap: true, PreserveBreaks: true}
	for _, tc := range []struct {
		text string
		ws   WhiteSpace
		want string
		what string
	}{
		{"ああ　　ああ", bs, "あ|あ　|　|あ|あ", "break-spaces"},
		{"ああ　　ああ", pre, "あ|あ　　|あ|あ", "pre-wrap, which is UAX #14's"},
		{"a b", bs, "a b", "a narrow no-break space under break-spaces"},
	} {
		if got := bar(tc.text, tc.ws, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %+q, want %+q", tc.what, got, tc.want)
		}
	}
}

// TestAWordBreakAllBoxDoesNotReachPastItsEdge. At a boundary between two boxes
// the later one's values decide — CSS Text leaves "which elements' line-break,
// word-break ... properties control" it undefined, and this engine has always
// answered with the later character's. So a break-all box's last letter is a
// letter again to the box after it: "<span class=break-all>bbb</span>ccc" does
// not break after the span (word-break-break-all-inline-007), and
// "aaa<span class=break-all>bbb</span>" does break before it.
func TestAWordBreakAllBoxDoesNotReachPastItsEdge(t *testing.T) {
	split := func(before, after string, wbBefore, wbAfter WordBreak) bool {
		_, tail := SplitAtBreaks(before, collapsing, wbBefore, LineBreak{}, Hyphens{},
			WritingSystemOther)
		pieces, _ := SplitAtBreaksAfter(after, collapsing, wbAfter, LineBreak{}, Hyphens{},
			WritingSystemOther, Carried{Context: tail.Context, Offered: tail.Offered,
				Deferred: tail.Deferred, Taken: tail.Taken, Prev: 'b', PrevBase: 'b'})
		return len(pieces) > 0 && pieces[0].BreakBefore
	}
	if split("bbb", "ccc", WordBreak{BreakAll: true}, WordBreak{}) {
		t.Error("a break after a break-all box, before a box of normal letters")
	}
	if !split("bbb", "ccc", WordBreak{}, WordBreak{BreakAll: true}) {
		t.Error("no break in front of a break-all box")
	}
}

// TestAnAtomicInlineBeginsTheRulesAgain. The rules are run over text, and an
// atomic inline is not text: after one they begin as at the start of a
// paragraph, so a combining mark after a picture is a letter of its own and
// holds the letter after it (line-breaking-atomic-017).
func TestAnAtomicInlineBeginsTheRulesAgain(t *testing.T) {
	var before BreakContext
	before.advance(lbTailoring{}.char('a'), Hyphens{})
	pieces, _ := SplitAtBreaksAfter("͏B", collapsing, WordBreak{}, LineBreak{},
		Hyphens{}, WritingSystemOther, Carried{Context: before.AfterObject()})
	for i, p := range pieces {
		if i > 0 && p.BreakBefore {
			t.Errorf("a break inside %+q after a picture: %+v", "͏B", pieces)
		}
	}
	if before.AfterObject().Started() {
		t.Error("the context after an atomic inline still has text in it")
	}
}

// TestAnOpportunityCSSGivesGoesPastASpace. An atomic inline's opportunity, a
// <wbr>'s, a hyphenation point's, arriving at a space: LB7 does not let a line
// end in front of the space, so it is taken after it — and taken there even
// where UAX #14 would refuse the character after the space, because it is not
// UAX #14's opportunity. "<img> )" may break before the bracket; " )" alone
// may not.
func TestAnOpportunityCSSGivesGoesPastASpace(t *testing.T) {
	for _, tc := range []struct {
		at   Carried
		want bool
		what string
	}{
		{Carried{Offered: true}, true, "an opportunity CSS gave"},
		{Carried{Prev: 'a', PrevBase: 'a'}, false, "a letter before the space"},
	} {
		pieces, _ := SplitAtBreaksAfter(" )", collapsing, WordBreak{}, LineBreak{},
			Hyphens{}, WritingSystemOther, tc.at)
		if len(pieces) != 2 || pieces[1].BreakBefore != tc.want {
			t.Errorf("%s: %+v, want a break before the bracket = %v", tc.what, pieces, tc.want)
		}
	}
}
