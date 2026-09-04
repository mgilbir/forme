package paragraph

import (
	"strings"
	"testing"
)

// A line break beside an ideograph, UAX #14.
//
// CJK is set without word spaces, so the algorithm puts an opportunity between
// ideographs — and there is nothing in it that stops one between an ideograph
// and a letter or a digit either. This engine offered the one *after* an
// ideograph and not the one before it, so "english中" was an unbreakable unit
// and a Latin word followed by CJK overflowed a line it could have broken.
//
// The suite says which it is, in as many words: the reference for
// word-break-keep-all-011 sets "中文english中文english…" under
// "word-break: normal" as 中文 / english / 中文 / english, one per line. That
// needs an opportunity after each 文 and one in front of each 中.

// barred renders the pieces with a bar at each break opportunity. splits in
// wordbreakkeepall_test.go is the same thing; this one is here so that the two
// files do not have to agree about a helper's name to compile.
func barred(t *testing.T, text string, wb WordBreak) string {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		wb, LineBreak{}, Hyphens{}, WritingSystemOther)
	var b strings.Builder
	for _, p := range pieces {
		if p.BreakBefore {
			b.WriteString("|")
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

// TestALineMayEndBeforeAnIdeograph is the rule.
func TestALineMayEndBeforeAnIdeograph(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"abc永def", "abc|永|def", "letters either side of one"},
		{"123永", "123|永", "after a number"},
		{"永abc", "永|abc", "the other side, which was already there"},
		{"abc永", "abc|永", "at the end of the text"},
	} {
		if got := barred(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.what, got, tc.want)
		}
	}
}

// TestTwoIdeographsHaveOneOpportunityBetweenThem is the containment half, and it
// is the one the corpus is most sensitive to.
//
// An ideograph *defers* its opportunity to the character after it, so a rule
// that also offered one before every ideograph would grant a second at the same
// place. That is not merely redundant: a prohibition refuses an opportunity by
// *holding* it, so the extra one reappears one character further along, in front
// of whatever the first was withheld from. Offered unconditionally it cost 63 of
// the suite's clean passes, which is why the rule asks what the character before
// is.
func TestTwoIdeographsHaveOneOpportunityBetweenThem(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"字字字字", "字|字|字|字"},
		{"字、字", "字、|字"},
		{"字。。字", "字。。|字"},
	} {
		if got := barred(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%q: %s, want %s", tc.text, got, tc.want)
		}
	}
}

// TestTheOpportunityBeforeAnIdeographIsStillSubjectToTheRules. It is offered
// like any other and then goes through the same prohibitions, rather than being
// placed directly.
func TestTheOpportunityBeforeAnIdeographIsStillSubjectToTheRules(t *testing.T) {
	// §5.2: keep-all suppresses the opportunities between typographic letter
	// units, and both sides of this one are letter units.
	wb, unhandled := WordBreakOf("keep-all")
	if unhandled != "" {
		t.Fatalf("keep-all was reported as unhandled: %q", unhandled)
	}
	if got := barred(t, "abc永def", wb); got != "abc永def" {
		t.Errorf("keep-all: %s, want no opportunity at all", got)
	}
	// A character that is not a letter unit before it offers nothing new, which
	// errs towards fewer opportunities — the direction this file takes wherever
	// a rule is not certain, because it overflows a line rather than breaking it
	// somewhere the algorithm did not sanction.
	for _, tc := range []struct{ text, want string }{
		{"a(永", "a(永"},
		{"a-永", "a-|永"},
	} {
		if got := barred(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%q: %s, want %s", tc.text, got, tc.want)
		}
	}
}

// TestASplitKeepsTheGapOnItsFarEdge.
//
// §8.1's gap sits at the far edge of the run it was added to, and a cut inside
// that run does not move the edge — it makes a new one. The head's far edge is
// the cut, which is a boundary the gap was never at; the tail's is the original.
//
// It matters because the two halves are measured again from their text, and the
// gap is not in the text. Left to that, a word cut by overflow-wrap loses an
// eighth of an em and everything after it on the line moves left.
func TestASplitKeepsTheGapOnItsFarEdge(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	gap := em.Div(8)
	whole := br.MeasureSpaced(face, "abcdef", onePx, TextSpacing{}).Add(gap)
	item := Item{Text: "abcdef", Face: face, Size: onePx, Width: whole, Autospace: gap}

	head, tail := br.SplitItem(item, 3)
	if head.Autospace != 0 {
		t.Errorf("the head carries %v of gap; its far edge is the cut, which had "+
			"none", head.Autospace)
	}
	if tail.Autospace != gap {
		t.Errorf("the tail carries %v of gap, want %v — the far edge is its now",
			tail.Autospace, gap)
	}
	if got := head.Width.Add(tail.Width); got != whole {
		t.Errorf("the two halves are %v wide and the item was %v; a cut does not "+
			"take an eighth of an em out of the line", got, whole)
	}
}
