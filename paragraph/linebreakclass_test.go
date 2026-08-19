package paragraph

import (
	"strings"
	"testing"
)

// A line may not begin with a closing bracket, a full stop, a hyphen or a
// non-starter — UAX #14's unconditional prohibitions, over the opportunities
// this package offers between ideographs and under word-break: break-all.
//
// The suite tests it a hundred and fifty-eight times, in one family, one
// character at a time: "中中X文" in a box three ideographs wide against a
// reference that breaks it as "中中" and "X文". Ninety-three of them failed,
// and every one failed the same way — the line began with the character the
// test is named after.

// marks returns the text with "|" where a line may begin, which is a readable
// way to state what a break rule does.
//
// A space belongs to the piece before it — UAX #14's LB7, "× SP": a line may
// not end between a word and the space after it — so "one two" comes out as
// "one |two" rather than "one| two". That is the existing rule showing through
// rather than anything this file is about, and it is worth seeing in the output
// rather than hidden by a helper that trimmed it.
func marks(t *testing.T, text string, wb WordBreak, lb LineBreak) string {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true}, wb, lb)
	var b strings.Builder
	for _, p := range pieces {
		if p.BreakBefore && b.Len() > 0 {
			b.WriteByte('|')
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

// TestALineDoesNotBeginWithOneOfThese walks the classes one at a time, each
// with the character the specification names in the rule it comes from.
//
// The fixture is an ideograph on either side, because that is where this
// package offers an opportunity at all: between two of them. Without the rule
// the break falls between the first ideograph and the punctuation, which puts
// the punctuation at the head of a line.
func TestALineDoesNotBeginWithOneOfThese(t *testing.T) {
	for _, tc := range []struct {
		rule, what string
		r          rune
	}{
		{"LB11", "a word joiner", '\u2060'},
		{"LB13", "a closing parenthesis", ')'},
		{"LB13", "a closing bracket", ']'},
		{"LB13", "a closing brace", '}'},
		{"LB13", "a fullwidth closing parenthesis", '）'},
		{"LB13", "an exclamation mark", '!'},
		{"LB13", "a question mark", '?'},
		{"LB13", "a solidus", '/'},
		{"LB15d", "a full stop", '.'},
		{"LB15d", "a comma", ','},
		{"LB15d", "a semicolon", ';'},
		{"LB21", "a hyphen-minus", '-'},
		{"LB21", "a Thai angkhankhu", '๚'},
		{"LB21", "a Khmer khan", '។'},
		{"LB21", "an ideographic iteration mark", '々'},
		{"LB21", "a wave dash", '〜'},
		{"LB21", "a katakana voiced sound mark", '゛'},
		{"LB21", "a double exclamation mark", '‼'},
		{"LB22", "an ellipsis", '…'},
	} {
		text := "中中" + string(tc.r) + "文"
		got := marks(t, text, WordBreak{}, LineBreak{})
		if strings.Contains(got, "|"+string(tc.r)) {
			t.Errorf("%s: a line may begin with %s: %s", tc.rule, tc.what, got)
		}
		// And the break between the two ideographs before it is still there,
		// so the rule withheld one opportunity rather than all of them.
		if !strings.HasPrefix(got, "中|中") {
			t.Errorf("%s: %s took the break between the ideographs with it: %s",
				tc.rule, tc.what, got)
		}
	}
}

// TestALineMayBeginWithTheseOnes is the containment half, and the half that
// says the set is UAX #14's rather than "punctuation".
func TestALineMayBeginWithTheseOnes(t *testing.T) {
	for _, tc := range []struct {
		what string
		r    rune
	}{
		// An opening bracket begins a line perfectly well. It is *ending* one
		// on it that LB14 forbids, which is a different question and one this
		// package answers elsewhere — see below.
		{"an opening parenthesis", '('},
		{"a medium left-pointing angle bracket ornament", '❬'},
		{"a left wiggly fence", '\u29D8'},
		// Small kana and the prolonged sound mark are class CJ, which UAX #14
		// resolves to NS by default and CSS Text's line-break: normal resolves
		// to ID. The default here is normal, so they break.
		{"a small hiragana a", 'ぁ'},
		{"a katakana-hiragana prolonged sound mark", 'ー'},
		// An ordinary letter, an ordinary ideograph, an em dash.
		{"a letter", 'A'},
		{"an ideograph", '国'},
		{"an em dash", '—'},
	} {
		text := "中中" + string(tc.r) + "文"
		got := marks(t, text, WordBreak{}, LineBreak{})
		if !strings.Contains(got, "|"+string(tc.r)) {
			t.Errorf("a line may not begin with %s, and nothing forbids it: %s",
				tc.what, got)
		}
	}
}

// TestBreakAllObeysTheProhibitionToo. break-all puts an opportunity between
// every pair of typographic character units, and §5.2 does not make it a
// licence to break the line-breaking rules: the suite's break-all-016 asserts
// exactly this, with "XX XXX..." in eight characters of room against a
// reference that breaks between the last two letters rather than inside the
// ellipsis.
func TestBreakAllObeysTheProhibitionToo(t *testing.T) {
	got := marks(t, "XX XXX...", WordBreak{BreakAll: true}, LineBreak{})
	if strings.Contains(got, "|.") {
		t.Errorf("break-all offered a line beginning with a full stop: %s", got)
	}
	// It still breaks between the letters, which is the opportunity break-all
	// is for.
	if !strings.Contains(got, "X|X") {
		t.Errorf("break-all stopped offering the opportunities it exists for: %s", got)
	}
}

// TestLineBreakAnywhereOverrulesIt, and by name: §5.3 puts an opportunity
// around every typographic character unit "including around any punctuation
// character or preserved white space". A value whose whole purpose is to break
// anywhere is not one to withhold breaks from.
func TestLineBreakAnywhereOverrulesIt(t *testing.T) {
	got := marks(t, "ab)c", WordBreak{}, LineBreak{Anywhere: true})
	if !strings.Contains(got, "|)") {
		t.Errorf("line-break: anywhere did not offer a break before a closing "+
			"bracket: %s", got)
	}
}

// TestAnOpportunityASpaceAlreadyOfferedIsNotWithdrawn.
//
// This is LB15c — "SP ÷ IS NU", break before a decimal mark that follows a
// space, so that "subtract .5" may wrap before the number — and it needs no
// code. The prohibition is applied where an opportunity is *offered*, and a
// space offers its own; what this withholds is the one the character before
// would otherwise have deferred.
//
// It is a test rather than a comment because the two are one line apart in
// SplitAtBreaks and a later edit could easily make the check cover both.
func TestAnOpportunityASpaceAlreadyOfferedIsNotWithdrawn(t *testing.T) {
	for _, tc := range []struct{ what, text, want string }{
		{"a decimal mark after a space", "subtract .5", "subtract |.5"},
		{"a bracket after a space", "see (a) or )b", "see |(a) |or |)b"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.what, got, tc.want)
		}
	}
}

// TestLatinTextIsUntouched. The prohibition applies to opportunities this
// package offers, and over ordinary prose it offers them at spaces and hyphens
// — neither of which this may take away.
func TestLatinTextIsUntouched(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"one two three", "one |two |three"},
		{"well-known word", "well-|known |word"},
		{"a, b; c.", "a, |b; |c."},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%q broke as %s, want %s", tc.text, got, tc.want)
		}
	}
}

// TestTheTableIsUnicodesAndNotAGuess spot-checks the generated set at its
// edges: the first range, the last, and a character on either side of a
// boundary. A lookup that answered by luck would pass everything above.
func TestTheTableIsUnicodesAndNotAGuess(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		{'\t', true, "a tab, which is class BA"},
		{'\t' - 1, false, "the character before the first range"},
		{'!', true, "an exclamation mark, class EX"},
		{' ', false, "a space, class SP — handled as a space, not by this"},
		{'\u2060', true, "a word joiner, class WJ"},
		{'\uFEFF', true, "a zero width no-break space, which is class WJ as well"},
		{'A', false, "a letter"},
		{0x10FFFF, false, "the last code point there is"},
		{-1, false, "not a character at all"},
	} {
		if got := noBreakBefore(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
	// The table has to be sorted and disjoint, or the search finds the wrong
	// range and the answer is silently wrong for a handful of characters.
	for i := 1; i < len(noBreakBeforeRanges); i++ {
		prev, this := noBreakBeforeRanges[i-1], noBreakBeforeRanges[i]
		if this.lo <= prev.hi {
			t.Fatalf("ranges %d and %d overlap: %04X..%04X then %04X..%04X",
				i-1, i, prev.lo, prev.hi, this.lo, this.hi)
		}
		if this.lo == prev.hi+1 {
			t.Errorf("ranges %d and %d are adjacent and were not merged: "+
				"%04X..%04X then %04X..%04X", i-1, i, prev.lo, prev.hi, this.lo, this.hi)
		}
	}
	// Every range holds at least one character, and none of them is empty.
	for i, r := range noBreakBeforeRanges {
		if r.lo > r.hi {
			t.Errorf("range %d runs backwards: %04X..%04X", i, r.lo, r.hi)
		}
	}
}

// TestBindsToAtomicInline is CSS Text §5.1's own sentence, read as a set.
//
// The rule is unusual in having a named exception, and the exception is the
// interesting half: a no-break space is class GL and a line breaks beside a
// picture anyway, "for Web-compatibility". Every other GL, WJ and ZWJ character
// holds on.
func TestBindsToAtomicInline(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		// The exception, named in the specification.
		{'\u00A0', false, "a no-break space, which is class GL and breaks anyway"},
		// GL.
		{'\u202F', true, "a narrow no-break space"},
		{'\u2007', true, "a figure space"},
		{'\u2011', true, "a non-breaking hyphen"},
		{'\u180E', true, "a Mongolian vowel separator"},
		{'༈', true, "a Tibetan mark sbrul shad"},
		{'༌', true, "a Tibetan mark delimiter tsheg bstar"},
		{'༒', true, "a Tibetan mark rgya gram shad"},
		// WJ.
		{'\u2060', true, "a word joiner"},
		{'\uFEFF', true, "a zero width no-break space"},
		// ZWJ.
		{'\u200D', true, "a zero width joiner"},
		// And the ones that hold on to nothing.
		{'A', false, "a letter"},
		{' ', false, "a space"},
		{'\u200B', false, "a zero width space, which is a break and not a bond"},
		{'-', false, "a hyphen-minus"},
		{'中', false, "an ideograph"},
		{0x10FFFF, false, "the last code point there is"},
		{-1, false, "not a character at all"},
	} {
		if got := BindsToAtomicInline(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
	// The table has to be sorted and disjoint, for the same reason the other
	// one does.
	for i := 1; i < len(bindingRanges); i++ {
		prev, this := bindingRanges[i-1], bindingRanges[i]
		if this.lo <= prev.hi {
			t.Fatalf("ranges %d and %d overlap: %04X..%04X then %04X..%04X",
				i-1, i, prev.lo, prev.hi, this.lo, this.hi)
		}
	}
	// U+00A0 is in the table and excluded by the code, which is the split this
	// file's two halves are about: the table is Unicode's classes and the
	// exception is CSS's decision. A table that had dropped it would make the
	// guard above unfalsifiable.
	found := false
	for _, r := range bindingRanges {
		if r.lo <= 0x00A0 && 0x00A0 <= r.hi {
			found = true
		}
	}
	if !found {
		t.Errorf("U+00A0 is not in the generated table, so nothing is excluding it")
	}
}
