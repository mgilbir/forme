package layout

import (
	"strings"
	"testing"
)

// Rule L2 reverses "any contiguous sequence of characters", and contiguous is a
// fact about the text rather than about the items a line is made of.
//
// An explicit formatting code is a character with a level of its own and no item
// to carry it. "לום<span style='unicode-bidi: isolate'>x</span>" resolves to
// levels 1 1 1 0 2 0: the level-0 isolate initiator stands between the Hebrew at
// level 1 and the "x" at level 2 and keeps them in two separate runs, each
// reversed in place. Ordering the two *items* alone sees 1 2, reverses them
// together as one run at level one or higher, and puts the isolated "x" in front
// of the Hebrew.
//
// The test the suite states it with is bidi-flag-emoji, and it states it by
// pairing: "לום🇱🇮" against "לום<span style='unicode-bidi: isolate'>🇱🇮</span>",
// which have to draw the same page. A regional indicator is bidi class L, so
// isolating it changes which rule gives it its level and changes nothing about
// where it goes.

// runOrder is a fixture's runs in the order they are drawn across the page.
func runOrder(t *testing.T, htmlSrc string) []string {
	t.Helper()
	type run struct {
		text string
		x    float64
	}
	var runs []run
	for _, op := range paintOf(t, htmlSrc, `div { margin: 0 }`) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			runs = append(runs, run{v.Text, v.At.X.Px()})
		}
	}
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].x < runs[j-1].x; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.text
	}
	return out
}

func TestAnIsolateDoesNotMoveWhatIsBesideIt(t *testing.T) {
	// The suite's pairing, and the two must draw the same page: isolating a
	// left-to-right run that is already left-to-right changes nothing.
	for _, tc := range []struct{ plain, isolated, what string }{
		{`<div>לום x</div>`, `<div>לום <span style="unicode-bidi:isolate">x</span></div>`,
			"Latin after Hebrew"},
		{`<div>לוםx</div>`, `<div>לום<span style="unicode-bidi:isolate">x</span></div>`,
			"Latin after Hebrew with no space"},
		{`<div>abcשלום</div>`, `<div>abc<span style="unicode-bidi:isolate">שלום</span></div>`,
			"Hebrew after Latin"},
	} {
		plain, isolated := runOrder(t, tc.plain), runOrder(t, tc.isolated)
		if strings.Join(plain, "|") != strings.Join(isolated, "|") {
			t.Errorf("%s: plain draws %v and isolated draws %v", tc.what, plain, isolated)
		}
	}
}

func TestTheIsolatedRunStaysAfterTheHebrew(t *testing.T) {
	// Stated as an order rather than as a comparison, so that the test says what
	// the right answer is and not merely that two documents agree.
	got := runOrder(t, `<div>לום<span style="unicode-bidi:isolate">x</span></div>`)
	if len(got) != 2 || got[0] != "לום" || got[1] != "x" {
		t.Errorf("the line draws %v, want the Hebrew first and the isolated "+
			"\"x\" after it: the paragraph reads left to right, so its one "+
			"right-to-left run comes first", got)
	}
}

func TestANestedIsolateIsTwoLevelsOfGap(t *testing.T) {
	// Two isolate initiators with no text between them, so the gap holds two
	// different levels. Which of them the gap is given decides the answer: L2
	// splits a run wherever *any* character is below the level, so the deciding
	// one is the lowest. Taking the highest instead merges the Hebrew and the
	// isolated "x" into one run and swaps them, and the single-isolate fixtures
	// above cannot tell, because a one-character gap has only one level in it.
	got := runOrder(t,
		`<div>לום<span style="unicode-bidi:isolate">`+
			`<span style="unicode-bidi:isolate">x</span></span></div>`)
	if len(got) != 2 || got[0] != "לום" || got[1] != "x" {
		t.Errorf("the line draws %v, want the Hebrew first: a nested isolate is "+
			"still an isolate and does not move what is beside it", got)
	}
}

func TestAnOrdinaryLineIsUnaffected(t *testing.T) {
	// Nothing without a gap may change, which is every ordinary line.
	for _, tc := range []struct{ src, want string }{
		{`<div>abc</div>`, "abc"},
		{`<div>שלום</div>`, "שלום"},
		{`<div>abc שלום</div>`, "abc|שלום"},
		{`<div dir=rtl>שלום abc</div>`, "abc|שלום"},
	} {
		if got := strings.Join(runOrder(t, tc.src), "|"); got != tc.want {
			t.Errorf("%s draws %q, want %q", tc.src, got, tc.want)
		}
	}
}
