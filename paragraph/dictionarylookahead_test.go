package paragraph

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A stretch of text segmented with DictionaryLookahead's worth of what follows
// it divides the way the whole text does.
//
// That is the claim the number is, and it is arithmetic rather than a
// measurement: segmentWords is greedy and runs left to right, so the only thing
// the text beyond a stretch can change is what longestAt finds at a position
// inside it, and longestAt stops after the longest word in the language. A probe
// that can see that far sees everything that could change its answer.
//
// It is held over the longest word there is, cut at every character, which is
// the shape that needs the most: the earlier the cut, the further the probe has
// to read to find the word it is in the middle of. Nothing shorter can need more.
func TestTheLookaheadIsEnoughForTheLongestWord(t *testing.T) {
	word := longestWordIn(t, thaiWords)
	look := DictionaryLookahead([]rune(word)[0])
	if look == 0 {
		t.Fatalf("no dictionary claims %q, so this test says nothing", word)
	}
	whole := piecesOf(t, word, "")
	for cut := range word {
		if cut == 0 || !utf8.RuneStart(word[cut]) {
			continue
		}
		head, rest := word[:cut], word[cut:]
		if len(rest) > look {
			rest = rest[:look]
		}
		// The head, segmented with the lookahead, must divide the way the same
		// bytes divide inside the whole word — which is not at all, since a
		// word is one piece.
		if got := piecesOf(t, head, rest); len(got) != 1 {
			t.Errorf("cut at %d, %q with %d bytes of lookahead divides as %q; it "+
				"is the first %d bytes of %q, which is one word and divides as "+
				"%q", cut, head, len(rest), got, cut, word, whole)
		}
	}
}

// TestTheLookaheadIsNoMoreThanEnough is the other half, and it is what keeps the
// number from being "a large round one that works".
//
// Some cut of the longest word has to *need* nearly all of it, or the bound is
// slack and a smaller one would do. This finds the most any cut needs and
// asserts it is within one character of what is handed over — near enough that
// no smaller bound in characters would be safe, and far enough that the arithmetic
// above has something to be about.
func TestTheLookaheadIsNoMoreThanEnough(t *testing.T) {
	word := longestWordIn(t, thaiWords)
	worst := 0
	for cut := range word {
		if cut == 0 || !utf8.RuneStart(word[cut]) {
			continue
		}
		head, rest := word[:cut], word[cut:]
		// At rune boundaries only: half a character of lookahead is not a
		// quantity anything hands over.
		for look := 0; look <= len(rest); look++ {
			if look < len(rest) && !utf8.RuneStart(rest[look]) {
				continue
			}
			if len(piecesOf(t, head, rest[:look])) == 1 {
				if look > worst {
					worst = look
				}
				break
			}
		}
	}
	look := DictionaryLookahead([]rune(word)[0])
	if worst == 0 {
		t.Fatalf("no cut of %q needs any lookahead at all, so the bound is "+
			"unfalsifiable and this test says nothing", word)
	}
	// utf8.UTFMax is the slack: the bound is counted in characters and spent in
	// bytes, and a Thai character is three bytes rather than four.
	if worst > look {
		t.Errorf("some cut of %q needs %d bytes of lookahead and the bound hands "+
			"over %d", word, worst, look)
	}
	if worst*utf8.UTFMax < look {
		t.Errorf("the worst cut of %q needs only %d bytes and the bound hands "+
			"over %d, which is slack enough that the bound is not being tested",
			word, worst, look)
	}
}

// longestWordIn is the longest word a dictionary holds, read from the list
// rather than from the constant beside it — the constant is what is under test.
func longestWordIn(t *testing.T, words string) string {
	t.Helper()
	best := ""
	for _, w := range strings.Split(words, "\n") {
		if utf8.RuneCountInString(w) > utf8.RuneCountInString(best) {
			best = w
		}
	}
	if best == "" {
		t.Fatal("the dictionary holds no words at all")
	}
	return best
}

// piecesOf is the text of each Piece SplitAtBreaksAfter cuts head into, given
// what follows it.
func piecesOf(t *testing.T, head, after string) []string {
	t.Helper()
	ps, _ := SplitAtBreaksAfter(head, WhiteSpace{Wrap: true, Collapse: true},
		WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther,
		Carried{After: after})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Text)
	}
	return out
}
