package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A line may not end *inside* the white space that ends it.
//
// CSS Text 3 §4.1.2's third and fourth rules are both written over a "sequence"
// of white space at the end of a line: the third removes the collapsible part of
// it, the fourth hangs what is left. The fill knew that a *hanging* item causes
// no break, which is most of it, and it knew that the paragraph's own trailing
// run is out of bounds. What it did not know is that a sequence need not be all
// one kind.
//
// The suite's fixture is trailing-ideographic-space-005: "ああ␣␣ ␣ ␣ああ" in two
// and a half ems, under white-space: normal, where ␣ is U+3000 IDEOGRAPHIC SPACE.
// That run is two ideographic spaces, an ordinary collapsible one, and two more
// ideographic spaces. The ideographic ones hang and offer a break after
// themselves; the ordinary one does not hang — the third rule merely removes it —
// so the fill took the opportunity the ideographic space in front of it offered
// and ended the line there. The rest of the run went to the line below, where it
// was no longer at the end of anything, and the words after it went to a third.
// Three lines, and the middle one nothing but spaces, where the specification
// asks for two and a run hanging off the first.

// hangingSpace is an item of white space that §4.1.2's fourth rule hangs: an
// other space separator, or preserved white space under a wrapping value.
func hangingSpace(text string, face *shape.Face, width style.Unit) Item {
	return Item{
		Text: text, Face: face, Size: u(size20), Width: width,
		Space: true, Hangs: true, HangsHard: true, BreakBefore: true,
	}
}

// collapsibleSpace is what §4.1.2's *third* rule removes at the end of a line.
// It does not hang, and that is the whole of the difference this file is about.
func collapsibleSpace(face *shape.Face, width style.Unit) Item {
	return Item{
		Text: " ", Face: face, Size: u(size20), Width: width,
		Space: true, Collapsible: true, TrimAtEnd: true, BreakBefore: true,
	}
}

// mixedTailRun is the suite's fixture as items: a word, a run of white space
// with one collapsible space among the hanging ones, and a word.
//
// The widths are written down rather than measured. Courier has no glyph for an
// ideographic space, so measuring one would give whatever the fallback happens
// to advance and the arithmetic below could not be read — and an Item is a
// value, so stating the width is stating the fixture rather than hiding it.
// Twelve pixels each, which is what a Courier character is at 20px.
func mixedTailRun(t *testing.T, br *Breaker, face *shape.Face) []Item {
	t.Helper()
	size := u(size20)
	word := func(s string, breakBefore bool) Item {
		return Item{
			Text: s, Face: face, Size: size, BreakBefore: breakBefore,
			Width: br.MeasureSpaced(face, s, size, TextSpacing{}),
		}
	}
	return []Item{
		word("aa", false), // 24px
		hangingSpace("A", face, u(12)),
		hangingSpace("B", face, u(12)),
		collapsibleSpace(face, u(12)),
		hangingSpace("C", face, u(12)),
		word("bb", true), // 24px
	}
}

// TestALineDoesNotEndInsideItsOwnTrailingWhiteSpace is the bug.
//
// In 40px "aa" fits and nothing else does. Every item after it is white space
// the end of a line is about, so all of it belongs to the first line — the
// hanging ones past its edge, the collapsible one removed from the measure — and
// the line ends where the words begin again.
//
// The wrong answer is not "one line too many" but a different line: the fill
// stopped at the collapsible space, and the run's remainder came back as the
// *leading* white space of the next line, where the first rule dropped the
// collapsible part and kept the rest. So the second line reads "Cbb" rather than
// "bb", with a space in front of the word that the author put after one.
func TestALineDoesNotEndInsideItsOwnTrailingWhiteSpace(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	got := breakAll(t, br, mixedTailRun(t, br, face), 40)
	if len(got) != 2 {
		t.Fatalf("the text took %d lines, want 2: %q", len(got), got)
	}
	if got[0] != "aaAB C" {
		t.Errorf("the first line reads %q, want %q: the whole run ends this line, "+
			"and the collapsible space in the middle of it does not cut it in two",
			got[0], "aaAB C")
	}
	if got[1] != "bb" {
		t.Errorf("the second line reads %q, want %q — anything else is white space "+
			"that belonged to the line before it", got[1], "bb")
	}
}

// TestABreakIsStillTakenAtAnOrdinarySpace is the containment case that matters
// most, because the rule added here says "do not break at this white space" and
// almost every line in almost every document ends at white space.
//
// It does not end *at* it: the opportunity a space offers is the one after it,
// so the break is taken at the word and the space stays behind to be trimmed.
// That is what these lines have always done and it must not change.
func TestABreakIsStillTakenAtAnOrdinarySpace(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	got := breakAll(t, br, words(t, br, face, "aaa bbb ccc"), 100)
	if len(got) != 2 {
		t.Fatalf("the text took %d lines, want 2: %q", len(got), got)
	}
	if got[0] != "aaa bbb " || strings.TrimSpace(got[1]) != "ccc" {
		t.Errorf("the text broke to %q, want %q then %q", got, "aaa bbb ", "ccc")
	}
}

// TestBreakSpacesStillBreaksInsideARunOfSpaces is the value the rule must not
// reach, and the reason the question is asked about the *item* rather than
// about white space in general.
//
// Under white-space: break-spaces a preserved space is data: it is never
// removed, it never hangs, it takes its width on the page, and §3 puts an
// opportunity after every one of them. So a line may end inside a run of them —
// which is the whole difference between break-spaces and pre-wrap — and neither
// Hangs nor TrimAtEnd is set on one, which is what keeps this rule away from it.
func TestBreakSpacesStillBreaksInsideARunOfSpaces(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	size := u(size20)
	space := func() Item {
		return Item{
			Text: " ", Face: face, Size: size, Space: true, BreakBefore: true,
			Width: br.MeasureSpaced(face, " ", size, TextSpacing{}),
		}
	}
	items := []Item{{
		Text: "aa", Face: face, Size: size,
		Width: br.MeasureSpaced(face, "aa", size, TextSpacing{}),
	}}
	for i := 0; i < 4; i++ {
		items = append(items, space())
	}
	// 24px of word and 12px a space: in 48px the line holds "aa" and two spaces,
	// and the third does not fit. break-spaces ends the line there.
	got := breakAll(t, br, items, 48)
	if len(got) < 2 {
		t.Fatalf("the run took %d lines, want more than one: %q", len(got), got)
	}
	if got[0] != "aa  " {
		t.Errorf("the first line reads %q, want %q: under break-spaces the line ends "+
			"inside the run, at the last space that fits", got[0], "aa  ")
	}
}

// TestHangingPunctuationStillCausesNoBreak.
//
// hanging-punctuation sets Hangs on a quotation mark, which is not white space.
// The rule added here is asked *beside* the older "a hanging item causes no
// break" rather than in place of it, and this is why: a first draft replaced it,
// and three of the suite's hanging-punctuation reftests went red — the closing
// quotation mark stopped hanging and pushed itself onto a line of its own.
func TestHangingPunctuationStillCausesNoBreak(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	size := u(size20)
	items := []Item{
		{
			Text: "aaa", Face: face, Size: size,
			Width: br.MeasureSpaced(face, "aaa", size, TextSpacing{}),
		},
		// The mark hangs past the end of the line. It is not white space, so the
		// rule this file is about has nothing to say about it, and the older one
		// must still stop the break.
		{
			Text: "”", Face: face, Size: size, BreakBefore: true, Hangs: true,
			Width: u(12),
		},
	}
	// 36px of word in 40px of room: the mark would take it to 48 and does not fit.
	got := breakAll(t, br, items, 40)
	if len(got) != 1 {
		t.Fatalf("the text took %d lines, want 1: %q", len(got), got)
	}
	if got[0] != "aaa”" {
		t.Errorf("the line reads %q, want %q: a hanging mark sits past the line's "+
			"end rather than starting a new one", got[0], "aaa”")
	}
}
