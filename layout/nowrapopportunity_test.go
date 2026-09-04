package layout

import (
	"strconv"
	"strings"
	"testing"
)

// An opportunity in front of an item a line may not begin at.
//
// CSS Text §3 says a "white-space: nowrap" span has no soft wrap opportunity
// inside it. SplitAtBreaks does not know that — it is given one box's text and
// the value is on the box — so the opportunities are cut as usual and the items
// carry NoWrap beside them, and the line fill is where the two meet.
//
// It met them in two places and only one of them was right. An item that may not
// begin a line was refused as a place to *end* one, which is the rule; it was
// still recorded as a place to send the line *back* to, and it still stopped the
// fill rewinding past it — so the fill either began a line in the middle of a
// span whose whole purpose is that no line begins in the middle of it, or found
// nowhere to go and let the line overflow.
//
// The fixtures are Courier at 20px, whose advance is 600 units of 1000, so a
// character is 12px and a width in characters is exact.

// nowrapCSS sets the fixture up: a monospaced face and a span that does not
// wrap.
const nowrapCSS = noDefaults + `
#d { font-family: Courier; font-size: 20px }
.n { white-space: nowrap }
`

// nowrapWidth is a width in Courier characters at 20px, whose advance is 600
// units of 1000 — so a character is exactly 12px and the arithmetic is exact.
func nowrapWidth(chars int) string {
	return "width: " + strconv.Itoa(chars*12) + "px"
}

// A line never begins inside a span that does not wrap, however badly the fill
// would like somewhere to go.
//
// The fill rewinds to the last opportunity it recorded, and the opportunities
// inside such a span are not opportunities: recording them let "bb cc" be split
// down the middle, which is the one thing the value was written to prevent.
func TestALineNeverBeginsInsideANowrapSpan(t *testing.T) {
	// "a bb ccdddddd" is thirteen characters in eight. The long word begins no
	// opportunity of its own — there is no space in front of it, only the end
	// of the span — so the fill has to rewind, and the only opportunity outside
	// the span is the one in front of it.
	got := lineTextsOf(t, layoutOf(t, 600,
		`<div id="d">a <span class="n">bb cc</span>dddddd</div>`,
		nowrapCSS+`#d { `+nowrapWidth(8)+` }`), "d")
	for _, line := range got {
		if strings.HasPrefix(line, "cc") {
			t.Errorf("the lines are %q — one begins in the middle of the span", got)
		}
	}
	want := []string{"a", "bb ccdddddd"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// And the other half: a span that does not fit sends the line back to the last
// opportunity outside it, rather than staying and overflowing.
//
// The item that does not fit is inside the span, so it has an opportunity in
// front of it and cannot take it. That combination reached no branch of the
// fill at all: the branch that ends a line at an item refuses one a line may
// not begin at, and the branch that rewinds past an item took only items with
// no opportunity in front of them.
//
// word-break-auto-phrase-wbr-nobr-002 is the suite's document for it, and its
// <nobr> is why it took this long to find: the element is not in any modern
// specification, so the rule it needs is one almost nothing else writes.
func TestASpanThatDoesNotFitSendsTheLineBackPastIt(t *testing.T) {
	// "ab cd ef" is eight characters in seven, and the only break outside the
	// span is the space after "ab".
	got := lineTextsOf(t, layoutOf(t, 600,
		`<div id="d">ab <span class="n">cd ef</span></div>`,
		nowrapCSS+`#d { `+nowrapWidth(7)+` }`), "d")
	want := []string{"ab", "cd ef"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q — the span stayed on the first line "+
			"and overflowed", got, want)
	}
	// The control: the same text with the span wrapping breaks inside it, so
	// the fixture is one where the value is doing the work.
	plain := lineTextsOf(t, layoutOf(t, 600,
		`<div id="d">ab <span>cd ef</span></div>`,
		nowrapCSS+`#d { `+nowrapWidth(7)+` }`), "d")
	if strings.Join(plain, "|") == strings.Join(want, "|") {
		t.Errorf("without nowrap the lines are the same %q, so the fixture says "+
			"nothing about the value", plain)
	}
}
