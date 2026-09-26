package layout

import (
	"strconv"
	"strings"
	"testing"
)

// UAX #14 across box boundaries, as a document meets it.
//
// paragraph's TestABoxBoundaryIsNotABoundaryToTheRules cuts every case of
// LineBreakTest.txt in two and hands the halves over as layout would. These are
// the three things only layout can get wrong: finding the text after a box for
// the rules that look ahead, collapsing a space into the one before it, and an
// inline box's margin, which takes the opportunity in front of the box.

// TestALookaheadRuleReadsTheNextBox. "× QU_Pf" — LB15b — refuses a break in
// front of a closing quotation mark only where what follows it is a space, a
// closing punctuation mark or the end of the text. Written "x ”a", the "a"
// after it lets the space's opportunity stand; written "x <span>”</span>a", the
// "a" is in the box after the one the rule is asked in, and a box that could
// not see it read the end of the text there and refused the break.
func TestALookaheadRuleReadsTheNextBox(t *testing.T) {
	const narrow = 20
	for _, tc := range []struct{ whole, cut string }{
		{"x ”a", "x <span>”</span>a"},
		{"x ”a", "x <span>”</span><span>a</span>"},
		{"x ” a", "x <span>”</span> a"},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if strings.Join(whole, "|") != strings.Join(cut, "|") {
			t.Errorf("%q set %q and %q set %q", tc.whole, whole, tc.cut, cut)
		}
	}
	// And the first row does break, so the comparison is not between two
	// unbroken lines.
	if got := linesOfMarkup(t, "x ”a", narrow); len(got) != 2 {
		t.Errorf("%q set %q; LB18 lets it break after the space", "x ”a", got)
	}
}

// TestACollapsedSpaceKeepsOnlyTheOpportunityItHas. §4.1.1's fourth rule
// collapses a space into the one before it and it "retains its soft wrap
// opportunity, if any" — and whether it has one is UAX #14's answer at the next
// character. "a )" does not break at the space (LB13), and "a <span> )</span>"
// has to agree, where it used to take an opportunity for the collapsed space
// whatever followed it.
func TestACollapsedSpaceKeepsOnlyTheOpportunityItHas(t *testing.T) {
	const narrow = 12
	for _, tc := range []struct{ whole, cut string }{
		{"a )", "a <span> )</span>"},
		{"a b", "a <span> b</span>"},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if strings.Join(whole, "|") != strings.Join(cut, "|") {
			t.Errorf("%q set %q and %q set %q", tc.whole, whole, tc.cut, cut)
		}
	}
}

// TestAMarginTakesTheOpportunityInFrontOfItsBox. CSS Text §5: "for soft wrap
// opportunities before the first ... character of a box, the break occurs
// immediately before ... the box (at its margin edge)". The margin takes it,
// and the text inside the box does not ask the rules about the same boundary a
// second time — which would let the line end between the margin and the word,
// leaving the margin at the end of the line above.
//
// Courier at 20px, so a character is 12px: "a-" is 24, the margin 30 and "b"
// 12, which is 66 in a line of 60. The break after the hyphen is taken at the
// margin edge, and "b" begins the next line 30px in.
func TestAMarginTakesTheOpportunityInFrontOfItsBox(t *testing.T) {
	root := layoutOf(t, 60, `<p id="p">a-<span style="margin-left: 30px">b</span></p>`,
		courier)
	if got := runX(t, root, "p", "b"); got != 30 {
		t.Errorf("the text of the box is at %g, want 30: the margin went with it "+
			"to the next line", got)
	}
}

// TestTheRulesBeginAgainAfterAPicture. An atomic inline is not text, and UAX
// #14's rules begin again after one: a combining mark after a picture is held
// to it by §5.1's exception and is a letter of its own to the rules (LB10), so
// it holds the letter after it — whatever came before the picture. Reading the
// character before the picture as the one before the mark made the mark part
// of an ideograph there, and a line could end between it and the "a".
func TestTheRulesBeginAgainAfterAPicture(t *testing.T) {
	css := widthCSS(1, "") + `#s { display: inline-block; width: ` +
		strconv.FormatFloat(ch, 'f', -1, 64) + `px; height: 10px }`
	root := layoutOf(t, 10000, "<p id=\"p\">中<span id=\"s\"></span>́a</p>", css)
	if got := len(linesOf(t, root, "p")); got != 2 {
		t.Errorf("%d lines, want 2: the ideograph, then the picture with the "+
			"mark and the letter it holds", got)
	}
}
