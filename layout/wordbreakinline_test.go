package layout

import (
	"strings"
	"testing"
)

// word-break: break-all declared on an *inline* box, and the boundary at its
// leading edge.
//
// §5.2 says what break-all does in terms of line-breaking classes: "any
// typographic character units resolving to the NU, AL, or ID Unicode line
// breaking classes are instead treated as ID for the purpose of line breaking".
// UAX #14 allows a line to end in front of an ID, so a run of Latin letters
// inside a break-all inline may begin a line even though the run before it —
// governed by word-break: normal — may not be broken inside.
//
// The split that finds break opportunities is given one box's text at a time,
// which is right for everything inside a box and blind to exactly one boundary:
// the first character of the box, whose neighbour is in the box before it. So
// "aaaaaaa<span style='word-break: break-all'>bbb</span>" kept the b's welded to
// the a's and then broke inside them — "aaaaaaab" and "bb" — where every browser
// and the suite's own reference put "aaaaaaa" and "bbb".

// wrapped returns what each line of a box reads.
func wrapped(t *testing.T, root *Fragment, id string) []string {
	t.Helper()
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f.Box != nil && f.Box.Element != nil {
			if got, _ := f.Box.Element.Attr("id"); got == id {
				for _, ln := range f.Lines {
					var b strings.Builder
					for _, r := range ln.Runs {
						b.WriteString(r.Text)
					}
					out = append(out, b.String())
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// The suite's own fixture, in this engine's arithmetic. Courier advances 0.6em,
// so at 12px a character is 7.2px and 6.1 of them are 43.92px — the "6.1ch" the
// reftests set, written out because there is no ch unit in a test's own head.
const breakAllCSS = `div, span { font-family: Courier; font-size: 12px }`

const sixPointOneCh = `width: 43.92px`

// TestBreakAllOnAnInlineBreaksAtItsLeadingEdge is the bug, and it is
// word-break-break-all-inline-004 with the numbers spelled out.
//
// Seven a's are wider than the box and there is nowhere to break them, so they
// overflow the first line. The b's are break-all and may begin a line, so they
// make the second. The wrong answer takes one b up to keep the first line as
// full as it can and cuts the word the author marked as breakable in a place
// nothing asked for.
func TestBreakAllOnAnInlineBreaksAtItsLeadingEdge(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="`+sixPointOneCh+`">aaaaaaa`+
		`<span style="word-break:break-all">bbb</span></div>`,
		noDefaults+breakAllCSS)
	got := wrapped(t, root, "d")
	want := []string{"aaaaaaa", "bbb"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the text broke to %q, want %q: the span's first character is an "+
			"ID under break-all, and a line may end in front of one", got, want)
	}
}

// TestTheOpportunitiesInsideTheInlineAreStillThere is the half that already
// worked, kept beside the new one because a rule about the *edge* of a box that
// lost the ones inside it would be a poor trade.
//
// Nine b's in a box that holds six characters: the second line takes six of
// them and the rest go on. That is break-all doing what it always did, and it
// is the control that says the leading-edge opportunity was added rather than
// substituted.
func TestTheOpportunitiesInsideTheInlineAreStillThere(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="`+sixPointOneCh+`">aaaaaaa`+
		`<span style="word-break:break-all">bbbbbbbbb</span>ccc</div>`,
		noDefaults+breakAllCSS)
	got := wrapped(t, root, "d")
	want := []string{"aaaaaaa", "bbbbbb", "bbbccc"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the text broke to %q, want %q", got, want)
	}
}

// TestAnInlineWithoutBreakAllStillHasNoEdgeToBreakAt is the containment case,
// and the one the change would most plausibly have got wrong: an opportunity
// offered at every inline boundary would break "aaaaaaa<span>bbb</span>" too,
// and that is a document with no break opportunity in it at all.
func TestAnInlineWithoutBreakAllStillHasNoEdgeToBreakAt(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="`+sixPointOneCh+`">aaaaaaa`+
		`<span>bbb</span></div>`, noDefaults+breakAllCSS)
	got := wrapped(t, root, "d")
	if len(got) != 1 || got[0] != "aaaaaaabbb" {
		t.Errorf("the text broke to %q, want one overflowing line of %q: neither box "+
			"asked for a break and there is nowhere to put one", got, "aaaaaaabbb")
	}
}

// TestALineStillMayNotBeginWithANonStarter.
//
// The opportunity added here is the same kind as one arriving from the box
// before, and UAX #14's prohibitions apply to it in the same way: a line may not
// begin with a closing bracket, a hyphen or a non-starter, whichever box the
// character is written in. "〜" is the one the suite's line-break family uses.
//
// Without the check the break would be taken in front of the mark and the line
// below would open with it, which is the fault that whole family exists to
// catch.
func TestALineStillMayNotBeginWithANonStarter(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="`+sixPointOneCh+`">aaaaaaa`+
		`<span style="word-break:break-all">〜bb</span></div>`,
		noDefaults+breakAllCSS)
	got := wrapped(t, root, "d")
	if len(got) > 0 && strings.HasPrefix(got[len(got)-1], "〜") {
		t.Errorf("the text broke to %q and a line begins with a wave dash; UAX #14 "+
			"forbids it, and break-all does not repeal it", got)
	}
}

// TestLineBreakAnywhereBreaksAtTheEdgeToo. §5.3 puts an opportunity around every
// typographic character unit "including around any punctuation character or
// preserved white space", and the edge of an inline box is not an exception it
// carves out — so the same boundary opens under line-break: anywhere, and opens
// even in front of the non-starter the test above holds shut.
func TestLineBreakAnywhereBreaksAtTheEdgeToo(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="`+sixPointOneCh+`">aaaaaaa`+
		`<span style="line-break:anywhere">bbb</span></div>`,
		noDefaults+breakAllCSS)
	got := wrapped(t, root, "d")
	want := []string{"aaaaaaa", "bbb"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the text broke to %q, want %q", got, want)
	}
}
