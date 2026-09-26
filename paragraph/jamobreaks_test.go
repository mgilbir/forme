package paragraph

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/segment"
)

// A Hangul syllable spelt in conjoining jamo breaks as the precomposed syllable
// does: between one syllable and the next, never inside one.
//
// UAX #14 names the jamo in two rules. LB26 forbids a break inside a syllable —
// JL × (JL | JV | H2 | H3), (JV | H2) × (JV | JT), (JT | H3) × JT — and LB27
// keeps a syllable to a postfix after it and a prefix before it. Everywhere
// else LB31 allows the break, which is what a paragraph of Korean wraps at.
// Each expectation below is LineBreakTest.txt's answer for the same classes
// (Unicode 17.0.0), and the keep-all column is §5.2's: the jamo are letters,
// as the syllables are, so the opportunities between them are withheld.

// breaksOf is where a text may break, as byte offsets inside it; a break keep-all
// only demotes is not one.
func breaksOf(text string, wb WordBreak) []int { return breaksWith(text, wb, LineBreak{}) }

func TestJamoSyllablesBreakAsSyllablesDo(t *testing.T) {
	const (
		l, v, t_ = "\u1100", "\u1161", "\u11A8" // JL, JV, JT
		lv, lvt  = "\uAC00", "\uAC01"           // H2, H3
	)
	for _, c := range []struct {
		what, text      string
		normal, keepAll []int
	}{
		// Two syllables, each spelt either way.
		{"jamo, then jamo", l + v + t_ + l + v + t_, []int{9}, nil},
		{"jamo, then precomposed", l + v + t_ + lv, []int{9}, nil},
		{"precomposed, then jamo", lvt + l + v, []int{3}, nil},
		{"precomposed, then precomposed", lv + lvt, []int{3}, nil},
		// LB26, inside one syllable.
		{"a leading jamo written twice before its vowel", l + l + v, nil, nil},
		{"a syllable and the trailing jamo after it", lv + t_, nil, nil},
		{"two trailing jamo", l + v + t_ + t_, nil, nil},
		// LB31, jamo that make no syllable together.
		{"a leading and a trailing jamo", l + t_, []int{3}, nil},
		{"a trailing jamo, then a leading one", t_ + l, []int{3}, nil},
		{"a vowel jamo, then a leading one", v + l, []int{3}, nil},
		// LB31 again, against a letter of another script.
		{"a letter, then a syllable in jamo", "a" + l + v, []int{1}, nil},
	} {
		if got := breaksOf(c.text, WordBreak{}); fmt.Sprint(got) != fmt.Sprint(c.normal) {
			t.Errorf("%s (%+q): breaks at %v, want %v", c.what, c.text, got, c.normal)
		}
		if got := breaksOf(c.text, WordBreak{KeepAll: true}); fmt.Sprint(got) != fmt.Sprint(c.keepAll) {
			t.Errorf("%s (%+q), keep-all: breaks at %v, want %v", c.what, c.text, got, c.keepAll)
		}
	}
}

// TestAJamoSyllableMeetsItsNeighboursAsAPrecomposedOneDoes.
//
// Outside the syllable, whatever a precomposed syllable does next to a
// character, a syllable spelt in jamo does too. LB27 is the rule that says so
// for a postfix and a prefix, and the tailorings §5.3 makes of those under each
// value of line-break are this engine's and not UAX #14's — under "auto" it
// lets a line begin with a per-cent sign, which LB27 does not. The comparison
// is with the precomposed syllable rather than with the rule, so that whatever
// the engine does for Korean, it does for both spellings of it.
func TestAJamoSyllableMeetsItsNeighboursAsAPrecomposedOneDoes(t *testing.T) {
	const jamo, composed = "\u1100\u1161\u11A8", "\uAC01" // 각, both ways
	shift := len(jamo) - len(composed)
	neighbours := []string{"%", "$", "a", "1", "\u4E2D", "\u3001", "!", ")", "(",
		"\u0301", "\u2014", "-", "\u00A0", "\u2060", "\u30FC", "\u3041", "\U0001F600"}
	values := []struct {
		what string
		wb   WordBreak
		lb   LineBreak
	}{
		{"auto", WordBreak{}, LineBreak{}},
		{"normal", WordBreak{}, LineBreak{Normal: true}},
		{"strict", WordBreak{}, LineBreak{Strict: true}},
		{"loose", WordBreak{}, LineBreak{Loose: true}},
		{"keep-all", WordBreak{KeepAll: true}, LineBreak{}},
		{"break-all", WordBreak{BreakAll: true}, LineBreak{}},
	}
	for _, n := range neighbours {
		for _, val := range values {
			for _, side := range []string{"after", "before"} {
				a, b := composed+n, jamo+n
				if side == "before" {
					a, b = n+composed, n+jamo
				}
				want := breaksWith(a, val.wb, val.lb)
				got := breaksWith(b, val.wb, val.lb)
				// The jamo spelling is longer, so a break after it is further
				// along by the difference.
				for i, at := range want {
					if side == "after" || at > len(n) {
						want[i] = at + shift
					}
				}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("%s %+q, %s: the jamo syllable breaks at %v, the "+
						"precomposed one at the same places would be %v",
						side, n, val.what, got, want)
				}
			}
		}
	}
}

// breaksWith is breaksOf under a value of line-break.
func breaksWith(text string, wb WordBreak, lb LineBreak) []int {
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true}, wb,
		lb, Hyphens{}, WritingSystemOther)
	var out []int
	at := 0
	for i, p := range pieces {
		if i > 0 && p.BreakBefore && !p.LastResort {
			out = append(out, at)
		}
		at += len(p.Text)
	}
	return out
}

// TestASyllableWrittenAcrossTwoBoxesIsNotCut.
//
// The opportunity a jamo offers is carried to the next box when the jamo is the
// last thing in its own, and the next box's first character decides whether the
// syllable ended. The grapheme scan starts afresh in that box and calls its
// first character a boundary, so the question is answered from the character
// before instead — see clusterContinues.
func TestASyllableWrittenAcrossTwoBoxesIsNotCut(t *testing.T) {
	for _, c := range []struct {
		what, before, text string
		breaks             bool
	}{
		{"a leading jamo, then its vowel", "\u1100", "\u1161", false},
		{"a syllable in jamo, then its trailing jamo", "\u1100\u1161", "\u11A8", false},
		{"a precomposed syllable, then its trailing jamo", "\uAC00", "\u11A8", false},
		{"an ideograph, then a combining mark", "\u65E5", "\u0301", false},
		{"a syllable in jamo, then the next", "\u1100\u1161", "\u1100\u1161", true},
		{"an ideograph, then another", "\u65E5", "\u672C", true},
	} {
		_, tail := SplitAtBreaks(c.before, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		last := []rune(c.before)
		pieces, _ := SplitAtBreaksAfter(c.text, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther, Carried{
				Offered: tail.Offered, Deferred: tail.Deferred, Held: tail.Held,
				Taken: tail.Taken, Prev: last[len(last)-1], PrevBase: last[len(last)-1],
			})
		got := len(pieces) > 0 && pieces[0].BreakBefore
		if got != c.breaks {
			t.Errorf("%s (%+q | %+q): a break at the boundary is %v, want %v",
				c.what, c.before, c.text, got, c.breaks)
		}
	}
}

// TestClusterContinuesIsTheSegmentersAnswer holds the pair rules to package
// segment's, for a character of every grapheme cluster break class on each side.
// The one pair they are allowed to differ on is two regional indicators, which
// GB12 and GB13 decide by counting the ones before them: segment, given the pair
// alone, pairs them, and one character of context cannot say whether it should.
func TestClusterContinuesIsTheSegmentersAnswer(t *testing.T) {
	reps := []rune{
		'\r', '\n', 0x01, 'a', 0x0301, 0x200D, 0x1F1E6, 0x0600, 0x0903,
		0x1100, 0x1161, 0x11A8, 0xAC00, 0xAC01, 0x231A, 0x0915, 0x094D,
	}
	for _, a := range reps {
		for _, b := range reps {
			if segment.BreakOf(a) == segment.RegionalIndicator &&
				segment.BreakOf(b) == segment.RegionalIndicator {
				continue
			}
			s := string([]rune{a, b})
			joined := len(segment.Boundaries(nil, s)) == 0
			if got := clusterContinues(a, b); got != joined {
				t.Errorf("clusterContinues(%U, %U) = %v; segment says the two are one "+
					"cluster: %v", a, b, got, joined)
			}
		}
	}
}
