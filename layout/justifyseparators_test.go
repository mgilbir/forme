package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Justification puts its slack at every word separator, one share each.
//
// CSS Text §7.3's inter-word justification "adjusts spacing at word separators",
// and §8.3 says which characters those are: the ordinary space, the no-break
// space, and the Ethiopic and Aegean separators. It is the same list word-spacing
// uses, and this engine used to read a narrower one here than it reads there —
// the ordinary space alone, and a run of them as a single gap.
//
// The suite states both halves in one document. white-space-pre-wrap-justify-002
// sets "one two  three   four …" with white-space: pre-wrap beside the same text
// written "one two&nbsp; three &nbsp; four …" with white-space: normal, and asks
// for the two to render identically. They can only do that if a run of three
// preserved spaces is three opportunities and a no-break space is one: the two
// spellings hold the same characters in the same places, and the second is the
// first with some of its spaces made unbreakable.
func TestJustificationExpandsAtEveryWordSeparator(t *testing.T) {
	// A monospace box of 22 characters holding fewer, so there is slack to
	// divide. text-align-last as well as text-align, because the only line of a
	// block is its last one and a last line is not justified.
	const css = `#p { font-family: Courier; font-size: 20px; width: 22ch;
		text-align: justify; text-align-last: justify }`

	// The second spelling of each is the first with some of its spaces written
	// U+00A0, which is what keeps them under white-space: normal — the same
	// characters in the same places, differing only in which of them a line may
	// break at.
	for _, tc := range []struct{ preWrap, normal, what string }{
		{"one two  three   four", "one two\u00a0 three \u00a0 four",
			"the suite's own text: runs of two and three"},
		// A no-break space closing a word, which is the shape that says the
		// separator list is §8.3's and not the ordinary space alone.
		{"a  b c d", "a\u00a0 b c d", "a no-break space closing a word"},
		// And one with a space on either side of it, which is the shape that
		// says the runs are cut: uncut, three preserved spaces are one
		// opportunity where the three characters beside them are three.
		{"aa   bb cc", "aa \u00a0 bb cc", "a no-break space between two spaces"},
	} {
		got := justifiedWordXs(t, tc.preWrap, css+" #p { white-space: pre-wrap }")
		want := justifiedWordXs(t, tc.normal, css+" #p { white-space: normal }")
		if len(got) != len(want) {
			t.Errorf("%s: the two spellings drew %d and %d words",
				tc.what, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: word %d is at %v under pre-wrap and %v written with "+
					"no-break spaces; the lines are %v and %v",
					tc.what, i, got[i], want[i], got, want)
				break
			}
		}
	}
}

// TestJustificationIsStillDividedAtAll is the containment case for the test
// above, which is an equality between two renderings and would hold if
// justification did nothing whatever.
func TestJustificationIsStillDividedAtAll(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 22ch;
		white-space: pre-wrap; text-align: justify; text-align-last: justify }`
	plain := justifiedWordXs(t, "one two  three   four",
		css+" #p { text-align: start; text-align-last: start }")
	spread := justifiedWordXs(t, "one two  three   four", css)
	if len(plain) != 4 || len(spread) != 4 {
		t.Fatalf("the fixture drew %d and %d words, want four of each",
			len(plain), len(spread))
	}
	if plain[0] != spread[0] {
		t.Errorf("justification moved the first word, from %v to %v", plain[0], spread[0])
	}
	for i := 1; i < len(plain); i++ {
		if spread[i] <= plain[i] {
			t.Errorf("word %d is at %v justified and %v not, so the slack went "+
				"nowhere", i, spread[i], plain[i])
		}
	}
}

// justifiedWordXs lays out one line and returns the x of every run that carries
// something other than white space.
func justifiedWordXs(t *testing.T, text, css string) []style.Unit {
	t.Helper()
	f := find(t, layoutOf(t, 4000, `<div id="p">`+text+`</div>`, noDefaults+css), "p")
	if len(f.Lines) != 1 {
		t.Fatalf("%q laid out as %d lines, want one", text, len(f.Lines))
	}
	var out []style.Unit
	for _, r := range f.Lines[0].Runs {
		if strings.TrimFunc(r.Text, func(r rune) bool {
			return r == ' ' || r == '\u00a0'
		}) == "" {
			continue
		}
		out = append(out, r.X)
	}
	return out
}
