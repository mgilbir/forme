package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// "text-justify: auto" asks for a script-appropriate algorithm, and word spaces
// are the wrong one for a script that has none.
//
// §7.3 says the value leaves the choice to the user agent and then says what the
// choice is expected to be: "inter-word for scripts using word separators" and
// "a script-appropriate algorithm for others (e.g. inter-character for CJK)".
// This engine only ever did the first, so a justified line of Japanese had no
// opportunity anywhere and was set flush to its start edge with all of its slack
// at the far end. text-align-last-justify-br is the suite's case: "<p>東京<br>
// 京城</p>" with "text-align-last: justify", whose reference puts one character
// at each margin on both lines.

// justifiedXs is where a fixture's runs are drawn, left to right.
func justifiedXs(t *testing.T, htmlSrc, cssSrc string) []string {
	t.Helper()
	var out []string
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v.Text+"@"+fmtPx(v.At.X))
		}
	}
	return out
}

const autoJustifyCSS = `p { text-align-last: justify; width: 200px; margin: 0 }`

// runXs is where a fixture's runs begin, as numbers.
func runXs(t *testing.T, htmlSrc, cssSrc string) []style.Unit {
	t.Helper()
	var out []style.Unit
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v.At.X)
		}
	}
	return out
}

func TestAJapaneseLineIsJustifiedBetweenItsCharacters(t *testing.T) {
	// Measured against the same line unjustified rather than against a number,
	// because the width of an ideograph is the fallback face's business and the
	// rule is not about any particular face. What justification does is put the
	// slack *between* the two characters: the first stays at the start edge and
	// the second moves the whole measure away from it.
	got := runXs(t, `<p>東京</p>`, autoJustifyCSS)
	flush := runXs(t, `<p>東京</p>`, `p { width: 200px; margin: 0 }`)
	if len(got) != 2 || len(flush) != 2 {
		t.Fatalf("the line drew %d runs justified and %d flush, want two of each",
			len(got), len(flush))
	}
	if got[0] != flush[0] {
		t.Errorf("the first character is at x=%v justified and x=%v flush; the "+
			"slack goes between the two and not in front of them", got[0], flush[0])
	}
	if got[1] <= flush[1] {
		t.Errorf("the second character is at x=%v justified and x=%v flush; it "+
			"has to move out to the far margin", got[1], flush[1])
	}
	// And exactly to it. The second character's advance is the distance between
	// the two when the line is flush, so where it ends is where the measure
	// ends: 8px of body margin plus the 200px the paragraph was given.
	advance := flush[1].Sub(flush[0])
	if end, want := got[1].Add(advance), mustPx(208); end != want {
		t.Errorf("the second character ends at x=%v, want %v — the far margin "+
			"of a 200px measure inside an 8px body margin", end, want)
	}
}

func TestALatinLineIsNotPulledApart(t *testing.T) {
	// A script with word separators keeps them, and a line with none is placed
	// at its start edge — §7.3's own answer for a line with no opportunity.
	got := justifiedXs(t, `<p>ab</p>`, autoJustifyCSS)
	if len(got) != 1 || !strings.HasSuffix(got[0], "@8px") {
		t.Errorf("the line drew %v, want \"ab\" left where it started: a Latin "+
			"word is not stretched between its letters", got)
	}
}

// runSpacing is the extra advance each of a fixture's runs carries after every
// character, which is where inter-character justification puts its slack.
func runSpacing(t *testing.T, htmlSrc, cssSrc string) []style.Unit {
	t.Helper()
	var out []style.Unit
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v.CharSpacing)
		}
	}
	return out
}

func TestAMixedLineKeepsItsWordSpaces(t *testing.T) {
	// One ideograph does not make the line CJK. The Latin word has a space
	// beside it to stretch, and pulling the word apart is what inter-character
	// justification would do to it — which is visible in the spacing the run
	// carries rather than in where the run begins, because either method leaves
	// the last unit at the far margin and the first where it started.
	got := runSpacing(t, `<p>ab 東</p>`, autoJustifyCSS)
	if len(got) != 2 {
		t.Fatalf("the line drew %d runs, want two", len(got))
	}
	for i, sp := range got {
		if sp != 0 {
			t.Errorf("run %d carries %v of extra advance after every character; "+
				"the line has a word space to stretch and its words stay whole",
				i, sp)
		}
	}
}

func TestALineOfPunctuationIsNotStretched(t *testing.T) {
	// Nothing on the line is a letter of any script, so there is nothing to be
	// script-appropriate about and §7.3's own answer for a line with no
	// opportunity applies: place it at the start edge and leave it short.
	// The spacing rather than the origin: two marks of punctuation are one run,
	// and inter-character justification spreads a run from inside it — the
	// origin stays where it was and every character after the first moves.
	for i, sp := range runSpacing(t, `<p>!?</p>`, autoJustifyCSS) {
		if sp != 0 {
			t.Errorf("run %d carries %v of extra advance after every character; "+
				"a line with no letters on it is not pulled apart", i, sp)
		}
	}
}

func TestANamedMethodIsStillObeyed(t *testing.T) {
	// "inter-word" is a document that has chosen. It keeps the word spaces even
	// where the script has none, which for a line with no space at all means no
	// opportunity and no stretching — so the line is set exactly as an
	// unjustified one is.
	got := runXs(t, `<p>東京</p>`, autoJustifyCSS+` p { text-justify: inter-word }`)
	flush := runXs(t, `<p>東京</p>`, `p { width: 200px; margin: 0 }`)
	if len(got) != 2 || len(flush) != 2 {
		t.Fatalf("the line drew %d runs, want two", len(got))
	}
	if got[0] != flush[0] || got[1] != flush[1] {
		t.Errorf("the line drew its characters at %v and %v against %v and %v "+
			"unjustified; \"inter-word\" was asked for by name and there is no "+
			"word space on the line", got[0], got[1], flush[0], flush[1])
	}
}

func TestTheLineIsAskedRatherThanTheElement(t *testing.T) {
	// There is no lang attribute anywhere in the suite's fixture, so the script
	// is the only evidence. A line of digits is not it.
	got := justifiedXs(t, `<p>12</p>`, autoJustifyCSS)
	if len(got) != 1 || !strings.HasSuffix(got[0], "@8px") {
		t.Errorf("the line drew %v, want the digits left where they started", got)
	}
}
