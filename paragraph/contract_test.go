package paragraph

import (
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/style"
)

// What a caller may hand in.
//
// The line breaking takes a cursor — an index into the items and a byte offset
// into the text of the one at that index — and the three splits take a byte
// offset into an item's text. A caller advances a cursor by handing back what
// the last call returned, and a caller that computes one itself can compute one
// that is not there.
//
// Four such numbers were unchecked. A negative index was a slice panic, an
// offset past the end of an item's text was another, an item with text and no
// face was a nil dereference — and an offset *inside* a character was worse than
// any of them, because it did not fail at all: the two halves were invalid UTF-8
// and invalid UTF-8 does not stop being text, it just stops being the author's.
//
// None of these is a case to answer differently. They are a caller's arithmetic
// gone wrong, and what this owes them is a defined answer rather than a crash
// inside a document generator.

// hostileCursors are the numbers that were not answers.
func hostileCursors(items []Item) []struct {
	name           string
	from, fromByte int
} {
	return []struct {
		name           string
		from, fromByte int
	}{
		{"a negative index", -1, 0},
		{"an index far below zero", -1 << 30, 0},
		{"an index past the end", len(items) + 5, 0},
		{"a negative offset", 0, -1},
		{"an offset past the item's text", 0, 1 << 20},
		{"an offset inside a character", 0, 1},
		{"both wrong at once", -3, -7},
	}
}

// TestALineBeginsWhereItCan.
func TestALineBeginsWhereItCan(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// Text whose characters are more than one byte, so an offset inside one is
	// reachable.
	items := itemsOf(t, br, face, "héllo wörld again", WhiteSpaceOf("collapse"), OverflowWrap{})

	for _, tc := range hostileCursors(items) {
		line, next, nextByte, _, _, _ := br.BreakOneLine(items, tc.from, tc.fromByte, u(120), 0)
		for _, it := range line {
			if !utf8.ValidString(it.Text) {
				t.Errorf("%s: a line came back holding %q, which is not text",
					tc.name, it.Text)
			}
		}
		if next < 0 || next > len(items) {
			t.Errorf("%s: the next line begins at item %d of %d",
				tc.name, next, len(items))
		}
		if next < len(items) && (nextByte < 0 || nextByte > len(items[next].Text)) {
			t.Errorf("%s: the next line begins %d bytes into an item of %d",
				tc.name, nextByte, len(items[next].Text))
		}
	}
}

// TestACutFallsOnACharacter. Backwards to the character's start rather than
// forwards, so a cut inside a letter puts the whole letter on the second line:
// the first is short by a character it could not draw either way, and no
// character is drawn twice.
func TestACutFallsOnACharacter(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	const text = "héllo"
	item := Item{Text: text, Face: face, Size: u(size20)}

	for at := -4; at <= len(text)+4; at++ {
		head, tail := br.SplitItem(item, at)
		if !utf8.ValidString(head.Text) || !utf8.ValidString(tail.Text) {
			t.Errorf("cut at %d gave %q and %q, which are not text",
				at, head.Text, tail.Text)
		}
		if head.Text+tail.Text != text {
			t.Errorf("cut at %d gave %q and %q, which are not the word",
				at, head.Text, tail.Text)
		}
		// The halves the other two entry points give have to be the same halves.
		if got := br.SplitHead(item, at); got.Text != head.Text {
			t.Errorf("cut at %d: SplitHead gave %q and SplitItem %q",
				at, got.Text, head.Text)
		}
		if got := br.SplitTail(item, at); got.Text != tail.Text {
			t.Errorf("cut at %d: SplitTail gave %q and SplitItem %q",
				at, got.Text, tail.Text)
		}
	}
}

// TestAnItemWithNoFaceMeasuresNothing. What a glyph advances is the face's to
// say, and an item carrying text and no face has none. Zero is a caller's
// mistake shown on the page; a nil dereference is a crash in a process that was
// doing something else.
func TestAnItemWithNoFaceMeasuresNothing(t *testing.T) {
	br := NewBreaker(nil)
	item := Item{Text: "abc"}
	head, tail := br.SplitItem(item, 1)
	if head.Width != 0 || tail.Width != 0 {
		t.Errorf("an item with no face measured %v and %v", head.Width, tail.Width)
	}
	if got := br.MeasureSpacedInContext(nil, "abc", u(20), TextSpacing{}, Shaping{}); got != 0 {
		t.Errorf("measuring with no face gave %v", got)
	}
	// And a line made of such items still comes back.
	items := []Item{item, {Text: " ", Space: true, Collapsible: true, BreakBefore: true}, item}
	if _, _, _, _, _, _ = br.BreakOneLine(items, 0, 0, u(50), 0); false {
		t.Fatal("unreachable")
	}
	_ = style.Unit(0)
}

// FuzzBreakerCursor is the general form of the two tables above: the numbers a
// caller hands in, rather than the text.
//
// The tables name the cases somebody thought of. This runs the same properties
// over whatever arithmetic the fuzzer produces — an index and an offset that
// bear no relation to the items, over text whose characters are of every length
// — and asks the two things that must hold whatever it is handed: nothing
// panics, and no piece of a caller's text comes back as something that is not
// text.
func FuzzBreakerCursor(f *testing.F) {
	for _, text := range []string{
		"héllo wörld", "abc", "", "­", "ẫb",
		"\U0001f600‍\U0001f600", "日本語のテキスト",
	} {
		for _, from := range []int{0, -1, 3} {
			f.Add(text, from, 1, 120)
		}
	}
	f.Fuzz(func(t *testing.T, text string, from, fromByte, width int) {
		if len(text) > 1<<12 {
			t.Skip("a fixture, not a corpus")
		}
		br := NewBreaker(nil)
		face := courier(t)
		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})

		line, next, nextByte, _, _, _ := br.BreakOneLine(items, from, fromByte, u(float64(width)), 0)
		for _, it := range line {
			if !utf8.ValidString(it.Text) {
				t.Fatalf("a line came back holding %q, which is not text", it.Text)
			}
		}
		if next < 0 || next > len(items) {
			t.Fatalf("the next line begins at item %d of %d", next, len(items))
		}
		if next < len(items) && (nextByte < 0 || nextByte > len(items[next].Text)) {
			t.Fatalf("the next line begins %d bytes into an item of %d",
				nextByte, len(items[next].Text))
		}

		// And the splits, over the same arithmetic.
		for _, it := range items {
			head, tail := br.SplitItem(it, fromByte)
			if !utf8.ValidString(head.Text) || !utf8.ValidString(tail.Text) {
				t.Fatalf("cutting %q at %d gave %q and %q", it.Text, fromByte,
					head.Text, tail.Text)
			}
			if head.Text+tail.Text != it.Text {
				t.Fatalf("cutting %q at %d gave %q and %q, which are not it",
					it.Text, fromByte, head.Text, tail.Text)
			}
		}
	})
}
