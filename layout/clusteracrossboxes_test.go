package layout

import (
	"strings"
	"testing"
)

// A grapheme cluster written across elements is one cluster on the page.
//
// Under line-break: anywhere every cluster boundary is an opportunity and
// nothing else is, so in a box narrower than any cluster each cluster is a line
// of its own and the lines are the clusters. The three shapes are the rules of
// UAX #29 that look further back than one character, each with an element
// boundary inside what they look at: GB12 and GB13's pairs of regional
// indicators, GB11's emoji sequence and GB9c's conjunct.
func TestAClusterWrittenAcrossElementsIsOneCluster(t *testing.T) {
	const css = `#d { line-break: anywhere }`
	for _, tc := range []struct {
		markup string
		want   []string
		what   string
	}{
		{"<span>\U0001F1F7\U0001F1FA\U0001F1F8</span>\U0001F1EA",
			[]string{"\U0001F1F7\U0001F1FA", "\U0001F1F8\U0001F1EA"}, "two flags"},
		{"\U0001F468<span>‍\U0001F469</span>",
			[]string{"\U0001F468‍\U0001F469"}, "an emoji sequence"},
		{"क्<span>ष</span>",
			[]string{"क्ष"}, "a conjunct"},
	} {
		got := linesOfMarkupCSS(t, tc.markup, 1, css)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: lines %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestAClusterDoesNotReachAcrossAPicture. An atomic inline is not a character,
// and a cluster does not continue across one: an emoji joiner after a picture
// joins nothing, so the pictograph after it begins a cluster of its own. Read
// with the scan the text before the picture left, the joiner continued the
// emoji before the picture, and the two pictographs became one sequence with
// an inline-block in the middle of it.
func TestAClusterDoesNotReachAcrossAPicture(t *testing.T) {
	const css = `#d { line-break: anywhere } .b { display: inline-block; width: 1px; height: 1px }`
	got := linesOfMarkupCSS(t, "\U0001F468<span class=b></span>‍\U0001F469", 1, css)
	if len(got) != 3 {
		t.Errorf("lines %q, want three: the man, the picture with the joiner it "+
			"holds, and the woman", got)
	}
}
