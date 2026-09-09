package layout

import (
	"fmt"
	"strings"
	"testing"
)

// A word breaks the same way however it is divided into elements.
//
// CSS Text §8.1: the boundary between two inline elements does not break
// shaping, and nothing in §5 makes it a place a break opportunity may be lost
// either. An opportunity offered by the last character of a text node belongs to
// whatever follows it, and the flattening already carries one across — the flag
// SplitAtBreaks returns is what it carries.
//
// Two opportunities were not offered when the character that offers them ended
// the node: an ordinary hyphen, and under line-break: loose a currency or number
// sign. What a document saw was a compound that overflowed its box for being
// written "high-<span>way</span>", and a Japanese line that broke in front of
// its yen sign rather than after it.
func TestAnOpportunityAtTheEndOfANodeReachesTheNextBox(t *testing.T) {
	// Five characters of room in a monospace face, and eight characters of word
	// — so a line that does not break overflows, and the break is the only thing
	// that can put the second half on a line of its own.
	const css = "#p { font-family: Courier; font-size: 20px; width: 5ch; %s }"

	for _, tc := range []struct {
		html, extra string
		want        []string
		what        string
	}{
		{`high-way`, "", []string{"high-", "way"}, "a hyphen inside one node"},
		{`high-<span>way</span>`, "", []string{"high-", "way"}, "a hyphen at a node edge"},
		{`<span>high-</span>way`, "", []string{"high-", "way"}, "a hyphen ending a span"},
		{`high<span>-</span>way`, "", []string{"high-", "way"}, "a hyphen alone in a span"},
		{`high‐<span>way</span>`, "", []string{"high‐", "way"}, "a U+2010 hyphen at a node edge"},

		{`サンプル€サンプル`, "line-break: loose",
			[]string{"サンプル€", "サンプル"}, "a prefix inside one node"},
		{`サンプル<span>€</span>サンプル`, "line-break: loose",
			[]string{"サンプル€", "サンプル"}, "a prefix in a span"},
		{`サンプル<span>￥</span>サンプル`, "line-break: loose",
			[]string{"サンプル￥", "サンプル"}, "a fullwidth yen sign in a span"},
	} {
		got := brokenLines(t, tc.html, fmt.Sprintf(css, tc.extra))
		if len(got) != len(tc.want) {
			t.Errorf("%s (%s): %d lines %q, want %q", tc.what, tc.html, len(got), got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s (%s): lines are %q, want %q", tc.what, tc.html, got, tc.want)
				break
			}
		}
	}
}

// TestALineStillDoesNotEndBeforeASpaceAfterAHyphen is the containment case: the
// guard the change above kept.
//
// A space is already an opportunity and a line may not end in front of one, so a
// hyphen with a space after it offers nothing of its own — there would be
// nothing to move to the next line but the space.
func TestALineStillDoesNotEndBeforeASpaceAfterAHyphen(t *testing.T) {
	got := brokenLines(t, `high- <span>way</span>`,
		`#p { font-family: Courier; font-size: 20px; width: 6ch }`)
	want := []string{"high-", "way"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("lines are %q, want %q — a line that ended before the space "+
			"would begin the next one with it", got, want)
	}
}

// lineTextsOf lays out one paragraph and returns each line's text, with the
// white space a line edge removes taken off the ends.
func brokenLines(t *testing.T, html, css string) []string {
	t.Helper()
	f := find(t, layoutOf(t, 4000, `<div id="p">`+html+`</div>`, noDefaults+css), "p")
	var out []string
	for _, ln := range f.Lines {
		var b strings.Builder
		for _, r := range ln.Runs {
			b.WriteString(r.Text)
		}
		out = append(out, strings.TrimSpace(b.String()))
	}
	return out
}
