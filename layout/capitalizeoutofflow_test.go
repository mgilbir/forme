package layout

import (
	"strings"
	"testing"
)

// An out-of-flow box in the middle of a word, and "text-transform: capitalize".
//
// §9.7 blockifies an absolutely positioned or floated inline, and the box
// builder read that as "a block-level box begins its text afresh" — which is
// right for a block in the flow and wrong for one that is not in it at all.
// Nothing of an out-of-flow box sits between the letters either side, so a word
// does not end because an author hung an overlay off the middle of it.
//
// css-text/text-transform/text-transform-capitalize-033 is the fixture:
// "p<span style='position: absolute'></span>ass" came out "PAss".

// capitalized is the text a document draws, runs joined.
func capitalized(t *testing.T, htmlSrc string) string {
	t.Helper()
	var b strings.Builder
	for _, op := range Paint(layoutOf(t, 600, htmlSrc, "")) {
		if v, ok := op.(DrawText); ok {
			b.WriteString(v.Text)
		}
	}
	return b.String()
}

// TestAnOutOfFlowBoxDoesNotEndAWord is the bug, for both ways out of the flow.
func TestAnOutOfFlowBoxDoesNotEndAWord(t *testing.T) {
	for _, style := range []string{
		"position: absolute",
		"position: fixed",
		"float: left",
		"float: right",
	} {
		src := `<p style="text-transform: capitalize">p<span style="` + style +
			`"></span>ass</p>`
		if got := capitalized(t, src); got != "Pass" {
			t.Errorf("with %q in the middle the word came out %q, want \"Pass\"",
				style, got)
		}
	}
}

// TestABlockInTheFlowStillEndsAWord is the containment argument, and it is what
// the reset is for: a word does not run from one paragraph into the next.
func TestABlockInTheFlowStillEndsAWord(t *testing.T) {
	if got := capitalized(t,
		`<div style="text-transform: capitalize">hi<div>there</div></div>`); got != "HiThere" {
		t.Errorf("the text came out %q, want \"HiThere\" — a block in the flow "+
			"begins its own text", got)
	}
	if got := capitalized(t,
		`<p style="text-transform: capitalize">hi</p><p style="text-transform: capitalize">there</p>`); got != "HiThere" {
		t.Errorf("two paragraphs came out %q, want \"HiThere\"", got)
	}
}

// TestABreakStillEndsAWord: a <br> is inline and ends the word anyway, because
// the author ended the line. It goes through the same branch and must not have
// been carried off with the blocks.
func TestABreakStillEndsAWord(t *testing.T) {
	if got := capitalized(t,
		`<p style="text-transform: capitalize">i ask<br>questions</p>`); got != "I AskQuestions" {
		t.Errorf("the text came out %q, want \"I AskQuestions\"", got)
	}
}

// TestAnInlineNeverEndedOneAnyway, so that a change to the out-of-flow case
// cannot quietly turn an ordinary span into a word boundary.
func TestAnInlineNeverEndedOneAnyway(t *testing.T) {
	if got := capitalized(t,
		`<p style="text-transform: capitalize">p<span></span>ass</p>`); got != "Pass" {
		t.Errorf("the text came out %q, want \"Pass\"", got)
	}
	if got := capitalized(t,
		`<p style="text-transform: capitalize">p<em>a</em>ss</p>`); got != "Pass" {
		t.Errorf("the text came out %q, want \"Pass\"", got)
	}
}
