package layout

import (
	"strings"
	"testing"
)

// Where §8.3's word-spacing lands, as against how much of it there is.
//
// A display list carries a run's text, its face, its width and its
// letter-spacing, and TextRun.LetterSpacing says why that last one is there: the
// width decided where the next run starts, so a backend that draws the glyphs
// without the same spacing leaves the whole run's worth of it as a gap. Word
// spacing has no such field, and could not usefully have one — it goes after
// particular characters rather than after all of them, so a single number would
// not say where.
//
// So the run is cut instead. Every word separator ends one, and the spacing
// after it is the tail of that run's width, which is a thing a width can express.
//
// This is invisible for an ordinary space, which already gets a piece of its own
// because a line may end after one. It is the no-break space that shows it, and
// a no-break space is what an author writes precisely when the two words must
// stay on one line — so the character most likely to sit in the middle of a run
// was the one whose spacing was measured and not drawn.

// runAt returns the first run of a line whose text is exactly what is asked for,
// and where it sits.
func separatedRunAt(t *testing.T, root *Fragment, id, text string) (x float64, ok bool) {
	t.Helper()
	f := find(t, root, id)
	if len(f.Lines) == 0 {
		t.Fatalf("#%s has no lines", id)
	}
	for _, r := range f.Lines[0].Runs {
		if r.Text == text {
			return r.X.Px(), true
		}
	}
	return 0, false
}

// lineText is every run of the first line, in order, with a bar between them —
// so a failure says what the cut actually did.
func lineText(t *testing.T, root *Fragment, id string) string {
	t.Helper()
	f := find(t, root, id)
	var parts []string
	for _, r := range f.Lines[0].Runs {
		parts = append(parts, r.Text)
	}
	return strings.Join(parts, "|")
}

// TestWordSpacingMovesWhatFollowsTheNoBreakSpace is the bug, in the arrangement
// css/CSS2/text/word-spacing-remove-space-005 uses: a word-spacing wide enough
// to see, and a letter after the separator whose position says whether the
// spacing was drawn.
func TestWordSpacingMovesWhatFollowsTheNoBreakSpace(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">a&#160;b</div>`,
		noDefaults+spaceCSS+` #p { word-spacing: 10px }`)
	// Courier at 20px is 12px a character. "a" and the no-break space are 24px,
	// and the spacing that goes after the separator is 10 more.
	x, ok := separatedRunAt(t, root, "p", "b")
	if !ok {
		t.Fatalf("the line is %q; nothing after the no-break space begins a run, so "+
			"the spacing after it is inside one and cannot be drawn", lineText(t, root, "p"))
	}
	if x != 34 {
		t.Errorf("the letter after the no-break space is at %gpx, want 34 (2 x 12 + 10); "+
			"the line is %q", x, lineText(t, root, "p"))
	}
}

// TestAnOrdinarySpaceIsUnaffected. The cut must not change the arrangement that
// already worked — a space is its own run either way, and a document with
// word-spacing on ordinary text is far more common than one with a no-break
// space in it.
func TestAnOrdinarySpaceIsUnaffected(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">a b</div>`,
		noDefaults+spaceCSS+` #p { word-spacing: 10px }`)
	if got := lineText(t, root, "p"); got != "a| |b" {
		t.Errorf("the line is %q, want \"a| |b\"", got)
	}
	if x, ok := separatedRunAt(t, root, "p", "b"); !ok || x != 34 {
		t.Errorf("the letter after the space is at %gpx (found=%v), want 34", x, ok)
	}
}

// TestNoWordSpacingLeavesTheRunWhole is the containment argument. Cutting a run
// costs something — two runs are shaped apart, measured apart and drawn apart —
// and there is nothing to gain by it where the spacing is zero, which is nearly
// every run of nearly every document.
func TestNoWordSpacingLeavesTheRunWhole(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">a&#160;b</div>`, noDefaults+spaceCSS)
	if got := lineText(t, root, "p"); got != "a b" {
		t.Errorf("the line is %q, want one run \"a\\u00a0b\"; with no word-spacing "+
			"there is nothing to put at the end of a run", got)
	}
}

// TestTheSeparatorEndsTheRunRatherThanBeginningTheNext says which side of the
// separator the cut falls on, and it is the whole of the fix: the spacing goes
// *after* the character, so the character has to be last.
func TestTheSeparatorEndsTheRunRatherThanBeginningTheNext(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">ab&#160;cd</div>`,
		noDefaults+spaceCSS+` #p { word-spacing: 10px }`)
	if got := lineText(t, root, "p"); got != "ab |cd" {
		t.Errorf("the line is %q, want \"ab\\u00a0|cd\"", got)
	}
	if x, _ := separatedRunAt(t, root, "p", "cd"); x != 46 {
		t.Errorf("\"cd\" is at %gpx, want 46 (3 x 12 + 10)", x)
	}
}
