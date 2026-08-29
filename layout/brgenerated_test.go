package layout

import (
	"strings"
	"testing"
)

// ::before and ::after on a <br>.
//
// A <br> is an element like any other. It generates a box, that box carries the
// two pseudo-elements the box builder gives every element, and CSS puts them
// where it puts them everywhere else: ::before at the start of the element's
// content and ::after at the end. What the element *does* — end the line — falls
// between them.
//
// The flattening replaced the whole box with the break, so both were dropped
// without a word. CSS2/generated-content/quotes-036 is where it showed up:
// three open-quotes on a <br>, and the three marks they draw simply were not on
// the page.

// brLines is the text of each line of #p, runs joined.
func brLines(t *testing.T, htmlSrc, cssSrc string) []string {
	t.Helper()
	f := find(t, layoutOf(t, 600, htmlSrc, cssSrc), "p")
	var out []string
	for _, ln := range f.Lines {
		var parts []string
		for _, r := range ln.Runs {
			parts = append(parts, r.Text)
		}
		out = append(out, strings.Join(parts, ""))
	}
	return out
}

const brCSS = `#p { font-family: Courier; font-size: 20px; width: 400px }`

// TestGeneratedContentOnABreakIsDrawn is the bug.
func TestGeneratedContentOnABreakIsDrawn(t *testing.T) {
	got := brLines(t, `<div id="p">x<br>y</div>`,
		brCSS+` #p br:before { content: "B" } #p br:after { content: "A" }`)
	want := []string{"xB", "Ay"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestEachHalfGoesOnItsOwnSideOfTheBreak. Which line each lands on is the whole
// of the rule, and an implementation that emitted both before the break — or
// both after it — would put the right characters on the page in the wrong place.
func TestEachHalfGoesOnItsOwnSideOfTheBreak(t *testing.T) {
	if got := brLines(t, `<div id="p">x<br>y</div>`,
		brCSS+` #p br:before { content: "B" }`); strings.Join(got, "|") != "xB|y" {
		t.Errorf("with only a ::before the lines are %q, want [\"xB\" \"y\"]", got)
	}
	if got := brLines(t, `<div id="p">x<br>y</div>`,
		brCSS+` #p br:after { content: "A" }`); strings.Join(got, "|") != "x|Ay" {
		t.Errorf("with only an ::after the lines are %q, want [\"x\" \"Ay\"]", got)
	}
}

// TestTheBreakStillBreaks is the containment argument. What a <br> is for is
// ending the line, and generated content around it must not swallow that.
func TestTheBreakStillBreaks(t *testing.T) {
	for _, css := range []string{
		brCSS,
		brCSS + ` #p br:before { content: "B" }`,
		brCSS + ` #p br:after { content: "A" }`,
		brCSS + ` #p br:before { content: "B" } #p br:after { content: "A" }`,
		// An empty string is content: it generates a box with no text in it,
		// and it still must not stand in for the break.
		brCSS + ` #p br:before { content: "" } #p br:after { content: "" }`,
	} {
		if got := brLines(t, `<div id="p">x<br>y</div>`, css); len(got) != 2 {
			t.Errorf("with %q the content is on %d lines, want 2: %q",
				css[len(brCSS):], len(got), got)
		}
	}
}

// TestAPlainBreakIsUnchanged: a <br> with nothing generated on it is what nearly
// every <br> in every document is, and it must cost nothing and change nothing.
func TestAPlainBreakIsUnchanged(t *testing.T) {
	got := brLines(t, `<div id="p">one<br>two</div>`, brCSS)
	if strings.Join(got, "|") != "one|two" {
		t.Errorf("the lines are %q, want [\"one\" \"two\"]", got)
	}
}

// TestWhatFollowsTheBreakStillBeginsALine. A collapsible space at the start of a
// line is removed, and with nothing generated after the break the text is at the
// start of one. With an ::after there, the space is no longer first — it follows
// the generated character — and it stays, which is what §4.1.1 says about a
// space between two things rather than at the head of a line.
func TestWhatFollowsTheBreakStillBeginsALine(t *testing.T) {
	if got := brLines(t, `<div id="p">x<br> y</div>`, brCSS); strings.Join(got, "|") != "x|y" {
		t.Errorf("the lines are %q, want [\"x\" \"y\"] — the space after the break "+
			"begins a line and is removed", got)
	}
	if got := brLines(t, `<div id="p">x<br> y</div>`,
		brCSS+` #p br:after { content: "A" }`); strings.Join(got, "|") != "x|A y" {
		t.Errorf("the lines are %q, want [\"x\" \"A y\"] — the generated character "+
			"is what begins the line, so the space after it is between two "+
			"things and stays", got)
	}
}
