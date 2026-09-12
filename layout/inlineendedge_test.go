package layout

import "testing"

// An inline box's own end edge takes room on the line it ends on.
//
// A span's margin, border and padding is ink, and the end edge of one is ink at
// the end of a line: there is nowhere between the character it closes over and
// the edge itself to break, so the two stand or fall together and the room the
// line needs for that character is the character plus the edge.
//
// The breaker used to count the *start* edge and not the end one. Nothing in the
// vendored suite writes the shape — the count is the same with this fixed and
// without it — so the measurements are here, stated as the pair a reader can
// check by arithmetic: ten columns of Courier, nine columns of text, and an edge
// of two.
//
// Both directions matter and only one of them is the obvious one. A *negative*
// end margin is how a document asks for content to hang past the edge of its
// box — the suite's hanging-punctuation-first-and-last-together draws a hanging
// bracket with "margin: 0 -1em" — and a line that would not otherwise hold the
// last word does hold it when the box it is in gives the room back. An engine
// that counts the end edge only when it is positive fixes the overflow and
// leaves that half wrong.

// lineCountOf is how many lines a markup takes in ten Courier columns.
//
// Courier advances 0.6em, so at 16px a column is 9.6px and the block below is
// 96px: "abcd efgh" is nine columns and fits, "abcd efghij" is eleven and does
// not, and every case here moves one of those two across the edge by two columns
// or by one.
func lineCountOf(t *testing.T, markup string) int {
	t.Helper()
	root := layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 16px; width: 10ch }`)
	f := find(t, root, "p")
	return len(f.Lines)
}

func TestAnInlinesEndEdgeTakesRoomOnItsLine(t *testing.T) {
	for _, tc := range []struct {
		markup string
		want   int
		what   string
	}{
		// Nine columns of text, which fit on their own.
		{`<span>abcd efgh</span>`, 1, "nine columns and no edge"},
		{`<span style="margin-right:2ch">abcd efgh</span>`, 2,
			"an end margin of two, which takes it to eleven"},
		{`<span style="padding-right:2ch">abcd efgh</span>`, 2, "an end padding"},
		{`<span style="border-right:2ch solid red">abcd efgh</span>`, 2, "an end border"},
		{`<span style="margin-left:2ch">abcd efgh</span>`, 2,
			"and the start margin, which was always counted"},
		// The edge on a span that opens in the middle of the line, where the
		// opportunity before it is the box boundary and not the word.
		{`abcd <span style="margin-right:2ch">efgh</span>`, 2,
			"an end margin on a span that opens mid-line"},

		// Eleven columns, which do not fit on their own.
		{`<span>abcd efghij</span>`, 2, "eleven columns and no edge"},
		{`<span style="margin-right:-1ch">abcd efghij</span>`, 1,
			"a negative end margin, which gives back the column it needed"},
		{`<span style="margin-left:-1ch">abcd efghij</span>`, 1,
			"and a negative start margin, which always did"},
	} {
		if got := lineCountOf(t, tc.markup); got != tc.want {
			t.Errorf("%s: %d lines, want %d\n\t%s", tc.what, got, tc.want, tc.markup)
		}
	}
}
