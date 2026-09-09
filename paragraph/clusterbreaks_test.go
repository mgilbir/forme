package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/segment"
)

// A line may only be cut where a grapheme cluster of the emitted text begins.
//
// CSS Text §5.2 and §5.3 both name the unit: break-all offers an opportunity at
// every "typographic character unit boundary" inside a word, and line-break:
// anywhere offers one around every typographic character unit. That unit is the
// grapheme cluster, and package segment is the one place that decides where the
// clusters are.
//
// The text it is asked about is the text the pieces carry, which is not always
// the text that came in: this walk decodes and re-writes, so a CRLF leaves as
// one character and a byte that is not UTF-8 leaves as the U+FFFD the reader
// says it is read as. segment.Boundaries, given the original bytes, would answer
// about a string these pieces do not contain — it calls an invalid byte its own
// cluster on both sides, and a mark that followed one would begin a piece of its
// own. A line starting with a combining mark is the visible form of asking the
// wrong string, so the invariant is stated over the emitted text and the breaker
// reads runes to match it. See the Scanner in SplitAtBreaks and
// segment.InvalidByte, which is the other reading and says who it is for.
func TestALineIsOnlyCutWhereAClusterBegins(t *testing.T) {
	corpus := []struct{ text, what string }{
		{"abc", "letters"},
		{"áb", "a letter and a combining mark"},
		{"क्षि", "a conjunct"},
		{"\U0001F1E6\U0001F1E7", "a regional indicator pair"},
		{"a\r\nb", "a CRLF, which is one cluster and leaves as one character"},
		{"a\xffb", "an invalid byte between two letters"},
		{"a\xff́b", "a combining mark after an invalid byte"},
		{"\xff\xfe\xfd", "three invalid bytes"},
		{"\xed\xa0\x80", "an encoded surrogate, which is three invalid bytes"},
		{"caf\xe9 x", "windows-1252 text with a space in it"},
		{"\U0001f1e6\xff\U0001f1e8", "an invalid byte between regional indicators"},
		{"a\xff́\U0001F1E6\xffb", "invalid bytes among things that combine"},
	}
	// The two values that put an opportunity at a cluster boundary, which are
	// the two that ask where the boundaries are.
	for _, mode := range []struct {
		what string
		wb   WordBreak
		lb   LineBreak
	}{
		{"word-break: break-all", WordBreak{BreakAll: true}, LineBreak{}},
		{"line-break: anywhere", WordBreak{}, LineBreak{Anywhere: true}},
	} {
		for _, tc := range corpus {
			pieces, _ := SplitAtBreaks(tc.text, WhiteSpace{Collapse: true, Wrap: true},
				mode.wb, mode.lb, Hyphens{}, WritingSystemOther)

			var b strings.Builder
			for _, p := range pieces {
				b.WriteString(p.Text)
			}
			emitted := b.String()

			cluster := map[int]bool{0: true, len(emitted): true}
			for _, at := range segment.Boundaries(nil, emitted) {
				cluster[at] = true
			}

			at := 0
			for _, p := range pieces {
				if p.BreakBefore && !cluster[at] {
					t.Errorf("%s, %s (%q): a line may be cut at %d of %q, which "+
						"is inside a grapheme cluster",
						mode.what, tc.what, tc.text, at, emitted)
				}
				at += len(p.Text)
			}
		}
	}
}
