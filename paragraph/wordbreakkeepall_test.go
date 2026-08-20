package paragraph

import (
	"strings"
	"testing"
)

// word-break: keep-all, CSS Text §5.2, and the rule it forced into the open.
//
// The value forbids a break "within words": "implicit soft wrap opportunities
// between typographic letter units (or other typographic character units
// belonging to the NU, AL, AI, or ID Line Breaking Classes) are suppressed".
// Both sides have to be a letter unit, and the suite's tests for the value are
// about the ones that are not — the break after an ideographic space, and the
// one after an ideographic comma, neither of which it may take.
//
// # The rule it forced into the open
//
// A prohibition *moves* an opportunity rather than deleting one, and this
// package used to delete. "× CL" says a line may not begin with a closing
// bracket; it says nothing against a line beginning with what comes after one.
// So "字字、字字" had a break between the two ideographs and none after the
// comma, and a four-character box set it as three characters and one. That was
// wrong at every value of word-break — word-break-keep-all-006 is only what
// noticed it — and putting it right moved eleven other reftests as well.

// keepAll and breakAll are read through the parser rather than built here: the
// value is what a document writes, and reading "keep-all" as the zero value is
// exactly the mistake a test that built the struct itself would not see.
func keepAll(t *testing.T) WordBreak {
	t.Helper()
	wb, unhandled := WordBreakOf("keep-all")
	if unhandled != "" {
		t.Fatalf("keep-all was reported as unhandled: %q", unhandled)
	}
	if wb == (WordBreak{}) {
		t.Fatal("keep-all read as the initial value")
	}
	return wb
}

// splits renders the pieces with a bar at each break opportunity.
func splits(t *testing.T, text string, wb WordBreak) string {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		wb, LineBreak{}, Hyphens{})
	var b strings.Builder
	for _, p := range pieces {
		if p.BreakBefore {
			b.WriteString("|")
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

// TestKeepAllSuppressesTheOpportunitiesInsideAWord.
func TestKeepAllSuppressesTheOpportunitiesInsideAWord(t *testing.T) {
	for _, tc := range []struct {
		text, normal, keepAll, what string
	}{
		{"字字字字", "字|字|字|字", "字字字字", "between ideographs"},
		{"中文english中文", "中|文|english中|文", "中文english中文",
			"between an ideograph and the letters after it"},
		{"字1字", "字|1字", "字1字", "between an ideograph and a digit"},
	} {
		if got := splits(t, tc.text, WordBreak{}); got != tc.normal {
			t.Errorf("%s, normal: %s, want %s", tc.what, got, tc.normal)
		}
		if got := splits(t, tc.text, keepAll(t)); got != tc.keepAll {
			t.Errorf("%s, keep-all: %s, want %s", tc.what, got, tc.keepAll)
		}
	}
}

// TestKeepAllLeavesTheOpportunitiesThatAreNotInsideAWord, which is what every
// one of the suite's fixtures for the value is about: what it must not take.
func TestKeepAllLeavesTheOpportunitiesThatAreNotInsideAWord(t *testing.T) {
	for _, tc := range []struct {
		text, want, what string
	}{
		{"字字　字字", "字字　|字字", "after an ideographic space"},
		{"字字、字字", "字字、|字字", "after an ideographic comma"},
		{"字字 字字", "字字 |字字", "after an ordinary space"},
		{"字字）字字", "字字）|字字", "after a fullwidth closing parenthesis"},
	} {
		if got := splits(t, tc.text, keepAll(t)); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.what, got, tc.want)
		}
	}
}

// TestAProhibitionMovesAnOpportunityRatherThanDeletingOne, which is the rule
// underneath the row above and is true at every value.
//
// A pair rule that forbids a line beginning with a character says nothing
// against a line beginning with the character after it. Deleting the
// opportunity instead is how "字字、字字" came to have a break in the middle of
// the first word and none after the comma.
func TestAProhibitionMovesAnOpportunityRatherThanDeletingOne(t *testing.T) {
	for _, tc := range []struct {
		text, want, what string
	}{
		{"字字、字字", "字|字、|字|字", "an ideographic comma, which is class CL"},
		{"字字。字字", "字|字。|字|字", "an ideographic full stop"},
		{"字字）字", "字|字）|字", "a fullwidth closing parenthesis"},
		{"字字…字", "字|字…|字", "an ellipsis, which is class IN"},
		// Two in a row: the opportunity travels past both.
		{"字字）。字", "字|字）。|字", "two of them"},
	} {
		if got := splits(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.what, got, tc.want)
		}
	}
	// And it is not *created* where there was none to move: a line may not begin
	// with a closing bracket, and one after a character that offers no
	// opportunity of its own leaves none behind it.
	if got := splits(t, "ab)cd", WordBreak{}); got != "ab)cd" {
		t.Errorf("%s: a bracket in Latin text invented an opportunity", got)
	}
	// Nor is it offered to a space. UAX #14's LB7 is "× SP" — do not break
	// before a space — and it is an earlier rule than everything that creates
	// an opportunity, so a space belongs to the word in front of it and the
	// break falls after the space where LB18 puts it. A moved opportunity is
	// still an opportunity and is held to that.
	for _, tc := range []struct{ text, want string }{
		{"字） 字", "字） |字"},
		{"字。 字", "字。 |字"},
	} {
		if got := splits(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%s, want %s: the opportunity the bracket displaced was offered "+
				"in front of the space rather than after it", got, tc.want)
		}
	}
}

// TestKeepAllChangesNothingAboutLatinText is the containment case. §5.2's rule
// is about typographic letter units, and Latin words are separated by spaces,
// which the value does not touch.
func TestKeepAllChangesNothingAboutLatinText(t *testing.T) {
	ws := WhiteSpace{Collapse: true, Wrap: true}
	for _, text := range []string{
		"hello world", "a-b", "one, two; three!", "a (b) c", "don't", "e.g. this",
	} {
		want, _ := SplitAtBreaks(text, ws, WordBreak{}, LineBreak{}, Hyphens{})
		got, _ := SplitAtBreaks(text, ws, keepAll(t), LineBreak{}, Hyphens{})
		if len(got) != len(want) {
			t.Errorf("%q: %d pieces under keep-all, want %d", text, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q: piece %d is %+v under keep-all, want %+v",
					text, i, got[i], want[i])
			}
		}
	}
}

// TestKeepAllIsNotBreakAll, which is the other half of the property and must be
// unaffected: one adds opportunities and the other takes them away.
func TestKeepAllIsNotBreakAll(t *testing.T) {
	ba, _ := WordBreakOf("break-all")
	if got := splits(t, "abcd", ba); got != "a|b|c|d" {
		t.Errorf("break-all gave %s", got)
	}
	if got := splits(t, "abcd", keepAll(t)); got != "abcd" {
		t.Errorf("keep-all gave %s over Latin text with no spaces in it", got)
	}
}
