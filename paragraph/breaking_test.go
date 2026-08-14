package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Line breaking, stated over items rather than over documents.
//
// The engine that uses this package tests these rules through HTML and CSS, and
// that is the right way to test most of them: a rule about "white-space: pre-wrap"
// is a rule about a declaration, and a test that writes the declaration is a test
// of the thing an author will actually meet.
//
// What those tests cannot do is put the breaker in a state a stylesheet does not
// conveniently reach. A line whose second item is an unbreakable word longer than
// any box, a run of items from four different faces, a float marker between two
// words with no float behind it — each is three lines here and a contrived
// document there, if it can be written at all. That is what this file is for, and
// it is possible because an Item is a value: text, a face, a width and some
// flags, with everything that belonged to the box tree now held opaquely.

// courier is the face the arithmetic below is stated in. It advances 600/1000,
// so at 20px every character is 12px and a run of n of them is 12n px — which is
// what lets every expected number here be read rather than recorded.
func courier(t *testing.T) *shape.Face {
	t.Helper()
	face, err := shape.Standard("Courier")
	if err != nil {
		t.Fatalf("loading Courier: %v", err)
	}
	return face
}

const size20 = 20

// words builds the items a sentence flattens to: a run per word, a run per
// space, and a break opportunity before every word after the first.
//
// It measures each run the way the breaker will, so the widths are the breaker's
// own rather than a second opinion about them.
func words(t *testing.T, br *Breaker, face *shape.Face, s string) []Item {
	t.Helper()
	size := u(size20)
	var out []Item
	for i, w := range strings.Split(s, " ") {
		if i > 0 {
			out = append(out, Item{
				Text: " ", Face: face, Size: size, Space: true, Collapsible: true,
				Width: br.MeasureSpaced(face, " ", size, TextSpacing{}),
			})
		}
		out = append(out, Item{
			Text: w, Face: face, Size: size, BreakBefore: i > 0,
			Width: br.MeasureSpaced(face, w, size, TextSpacing{}),
		})
	}
	return out
}

// lineText is what a broken line reads, which is what these assertions are about.
func lineText(line []Item) string {
	var b strings.Builder
	for _, it := range line {
		b.WriteString(it.Text)
	}
	return b.String()
}

// breakAll runs the breaker to exhaustion at one width and returns the lines.
//
// The cursor is an index and a byte offset because a line may end inside a word,
// and carrying both is the only way to resume where the last line stopped. A
// guard against a cursor that does not advance is here rather than in the
// breaker because a breaker that failed to advance would hang this test rather
// than fail it, and a hanging test says nothing.
func breakAll(t *testing.T, br *Breaker, items []Item, width float64) []string {
	t.Helper()
	var out []string
	i, iByte := 0, 0
	for n := 0; i < len(items); n++ {
		if n > len(items)+8 {
			t.Fatalf("the breaker did not reach the end of %d items in %d lines; "+
				"it is not advancing", len(items), n)
		}
		line, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, u(width), 0)
		if next == i && nextByte == iByte {
			t.Fatalf("the cursor did not move at item %d byte %d", i, iByte)
		}
		out = append(out, lineText(line))
		i, iByte = next, nextByte
	}
	return out
}

func TestAWordThatFitsStaysOnTheLine(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// "aaa bbb" is 3 + 1 + 3 characters at 12px: 84px. In 200px it is one line.
	got := breakAll(t, br, words(t, br, face, "aaa bbb"), 200)
	if len(got) != 1 || got[0] != "aaa bbb" {
		t.Errorf("in a 200px line the text broke to %q, want one line of %q", got, "aaa bbb")
	}
}

func TestALineEndsBeforeAnOpportunityThatWillNotFit(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// Each word is 36px and each space 12px. In 100px: "aaa bbb" is 84px and
	// fits, adding " ccc" would make 132px and does not — so the line ends before
	// "ccc", which carries the opportunity.
	got := breakAll(t, br, words(t, br, face, "aaa bbb ccc"), 100)
	if len(got) != 2 {
		t.Fatalf("the text took %d lines, want 2: %q", len(got), got)
	}
	// The trailing space stays on the line: §4.1.2 hangs whatever white space its
	// third rule leaves at the end rather than sending it down, so the line reads
	// "aaa bbb " and is measured as "aaa bbb".
	if got[0] != "aaa bbb " {
		t.Errorf("the first line reads %q, want %q — the breaker did not fill the "+
			"line, or it did not hang the space", got[0], "aaa bbb ")
	}
	if strings.TrimSpace(got[1]) != "ccc" {
		t.Errorf("the second line reads %q, want %q", got[1], "ccc")
	}
}

// TestALineIsSentBackToItsLastOpportunity is the other way a line can end, and
// the one that needs the breaker to remember something.
//
// A line ends at an item that carries a break opportunity simply by stopping
// before it — no memory needed. But an item may *not* carry one: "ab<span>cd</span>"
// is two items with no opportunity between them, because there is no break inside
// a word. When such an item is the one that overflows, the line has to be sent
// back to the last opportunity it passed, and everything after that point
// re-broken onto the next line.
//
// The fixture is two runs of one word. Nothing a stylesheet forbids, but awkward
// to write as a document and trivial to write here.
func TestALineIsSentBackToItsLastOpportunity(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	size := u(size20)
	w := func(s string) style.Unit { return br.MeasureSpaced(face, s, size, TextSpacing{}) }

	sp := func() Item {
		return Item{Text: " ", Face: face, Size: size, Space: true, Collapsible: true, Width: w(" ")}
	}
	items := []Item{
		{Text: "aaa", Face: face, Size: size, Width: w("aaa")},
		sp(),
		// One opportunity here...
		{Text: "bbb", Face: face, Size: size, BreakBefore: true, Width: w("bbb")},
		sp(),
		// ...and a second one here. Two are needed: with only one, the last
		// opportunity passed is also the first, and a breaker that kept the wrong
		// one of the two would be indistinguishable from a correct one.
		{Text: "cc", Face: face, Size: size, BreakBefore: true, Width: w("cc")},
		// The rest of that same word: no opportunity before it.
		{Text: "dd", Face: face, Size: size, Width: w("dd")},
	}

	// 36 + 12 + 36 + 12 + 24 = 120 fits in 130; adding "dd" makes 144 and does
	// not. "dd" carries no opportunity, so the line goes back to the last one it
	// passed — the space before "cc", not the one before "bbb".
	got := breakAll(t, br, items, 130)
	if len(got) != 2 {
		t.Fatalf("the text took %d lines, want 2: %q", len(got), got)
	}
	if got[0] != "aaa bbb " {
		t.Errorf("the first line reads %q, want %q. %q means the line went back to the "+
			"*first* opportunity it passed rather than the last; anything ending in "+
			"\"cc\" means it did not go back at all and split a word where no rule "+
			"allows a break", got[0], "aaa bbb ", "aaa ")
	}
	if got[1] != "ccdd" {
		t.Errorf("the second line reads %q, want %q — the word must arrive whole", got[1], "ccdd")
	}
}

// TestAWordWiderThanTheLineIsNotBrokenWithoutLeave is CSS Text §5.5: a word with
// no opportunity in it overflows rather than being cut, unless overflow-wrap
// asked for the cut.
//
// It is the case a document states awkwardly — it needs a box narrower than one
// word of the face it is set in — and the case a breaker is most likely to get
// wrong in the direction of looking right, because cutting the word produces a
// tidy page that no rule asked for.
func TestAWordWiderThanTheLineIsNotBrokenWithoutLeave(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// One word of six characters, 72px, in a line of 30px.
	items := words(t, br, face, "abcdef")
	got := breakAll(t, br, items, 30)
	if len(got) != 1 || got[0] != "abcdef" {
		t.Errorf("an unbreakable word in a 30px line broke to %q, want it whole and "+
			"overflowing — nothing in the items allows a break inside it", got)
	}
}

// TestOverflowWrapCutsTheWordItIsAskedTo is the other half of the same rule, and
// the pair is the point: an assertion that the word is never cut is satisfied by
// a breaker that cannot cut, and one that it is always cut by a breaker that
// ignores the property.
func TestOverflowWrapCutsTheWordItIsAskedTo(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := words(t, br, face, "abcdef")
	items[0].BreakWord = true

	got := breakAll(t, br, items, 30)
	if len(got) < 2 {
		t.Fatalf("with overflow-wrap the word took %d lines, want at least 2: %q",
			len(got), got)
	}
	if strings.Join(got, "") != "abcdef" {
		t.Errorf("the cut word reads %q in total, want %q — breaking it must not "+
			"lose or duplicate a character", strings.Join(got, ""), "abcdef")
	}
	// 30px holds two 12px characters and not three.
	if len(got[0]) != 2 {
		t.Errorf("the first line took %q, want two characters — 30px holds 24px of "+
			"Courier and not 36", got[0])
	}
}

// TestAForcedBreakEndsTheLineWhereverItFalls is the difference between an
// opportunity and an instruction. The line has room for everything after it.
func TestAForcedBreakEndsTheLineWhereverItFalls(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := words(t, br, face, "aaa bbb")
	// A <br> between the two words: no width, no text, and it ends the line.
	items = append([]Item{items[0], {Forced: true}}, items[1:]...)

	got := breakAll(t, br, items, 500)
	if len(got) != 2 {
		t.Fatalf("a forced break in a 500px line gave %d lines, want 2: %q", len(got), got)
	}
	if got[0] != "aaa" {
		t.Errorf("the line before the forced break reads %q, want %q — it ends at the "+
			"break and not where the text ran out", got[0], "aaa")
	}
}

// TestAFloatMarkerIsHandedBackWithHowFarAlongItWasReached is the seam itself.
//
// A float among the words takes no room on the line and is not text, but *where*
// it was reached decides which line box it is placed against — so the breaker
// records it and hands it back. What the float is, is the caller's business:
// here it is a string, which is exactly the point of holding it opaquely.
func TestAFloatMarkerIsHandedBackWithHowFarAlongItWasReached(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := words(t, br, face, "aaa bbb")
	// After "aaa " and before "bbb", so it is reached at 48px.
	marker := Item{Float: "a float, and this package never asks what kind"}
	items = append([]Item{items[0], items[1], marker}, items[2:]...)

	line, _, _, outOfFlow, _ := br.BreakOneLine(items, 0, 0, u(500), 0)
	if len(outOfFlow) != 1 {
		t.Fatalf("the line reported %d out-of-flow boxes, want 1", len(outOfFlow))
	}
	if got := outOfFlow[0].Used.Px(); got != 48 {
		t.Errorf("the float was reached at %gpx along the line, want 48 — after "+
			"\"aaa\" and its space", got)
	}
	if outOfFlow[0].Box != marker.Float {
		t.Errorf("the float handed back is %v, want the one that went in", outOfFlow[0].Box)
	}
	// And it is not text: the line reads as though it were not there.
	if got := lineText(line); got != "aaa bbb" {
		t.Errorf("the line reads %q, want %q — a float takes no room among the words", got, "aaa bbb")
	}
}

func TestMeasuringIsMemoizedPerFaceSizeAndSpacing(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	size := u(size20)
	plain := br.MeasureSpaced(face, "abcde", size, TextSpacing{})
	spaced := br.MeasureSpaced(face, "abcde", size, TextSpacing{Letter: u(3)})
	if plain.Px() != 60 {
		t.Errorf("\"abcde\" at 20px Courier is %gpx, want 60", plain.Px())
	}
	if spaced.Px() != 75 {
		t.Errorf("the same run with 3px of letter-spacing is %gpx, want 75 — the "+
			"spacing is not part of the memo's key", spaced.Px())
	}
}
