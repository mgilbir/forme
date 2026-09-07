package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A hyphenation that takes a character off the next line, and the balancer.
//
// BreakOneLine says where the next line begins as an item and a byte offset into
// it, and a non-zero offset is what "a word was cut here" looks like: it is
// where overflow-wrap put the break. §5.1's balancing reads it that way and
// refuses any width that produces one, because balancing a paragraph by cutting
// its words is not balancing it.
//
// The same offset is set for the opposite reason. Some languages take a
// character off the start of the next line when a word is divided — pinyin's
// syllable separator is the one this engine has, "tú’àn" broken as "tú-" and
// "àn" — and that is a *hyphenation*, which balancing is supposed to use. Told
// apart from the cut only by the number, every width that hyphenated was
// refused, and a paragraph whose breaks all hyphenate could not be balanced at
// any width at all.

// hyphenatingItems is a paragraph of words that divide in the middle, each
// division printing a hyphen and dropping the separator that follows it.
//
// Built rather than measured from text: the pieces a document produces carry the
// hyphen fields from the face, and what is under test is the line breaking's
// reading of them.
func hyphenatingItems(br *Breaker, face *shape.Face, words int) []Item {
	size := u(size20)
	measure := func(s string) style.Unit { return br.MeasureSpaced(face, s, size, TextSpacing{}) }
	var out []Item
	for i := 0; i < words; i++ {
		if i > 0 {
			out = append(out, Item{
				Text: " ", Width: measure(" "), Face: face, Size: size,
				Space: true, Collapsible: true, TrimAtEnd: true, BreakBefore: true,
			})
		}
		out = append(out,
			Item{
				Text: "tu", Width: measure("tu"), Face: face, Size: size,
				// The division: a hyphen is printed here, and the separator at
				// the start of the next line goes with it.
				Hyphen: measure("-"), HyphenText: "-", HyphenSkip: len("’"),
			},
			Item{
				Text: "’an", Width: measure("’an"), Face: face, Size: size,
				BreakBefore: true,
			})
	}
	return out
}

// TestAWidthThatHyphenatesIsNotAWidthThatCutAWord.
//
// Three words of five characters in Courier at 20px, so twelve pixels a
// character. Two lines of them without hyphenating needs 132px — "tu’an tu’an"
// and "tu’an" — and hyphenating the second word makes the same two lines fit in
// 108: "tu’an tu-" and "an tu’an". Balancing is the search for the narrowest
// width that still takes the same number of lines, so 108 is the answer and 132
// is the answer of an engine that will not hyphenate.
func TestAWidthThatHyphenatesIsNotAWidthThatCutAWord(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := hyphenatingItems(br, face, 3)

	// The fixture has to hyphenate, or this measures a paragraph of plain words
	// and would pass with the whole rule removed.
	narrow := u(size20 * 2)
	line, _, nextByte, _, _, hyphenated := br.BreakOneLine(items, 0, 0, narrow, 0)
	if !hyphenated {
		t.Fatalf("the fixture did not hyphenate at %v: it ended at %d items with "+
			"an offset of %d", narrow, len(line), nextByte)
	}
	if nextByte == 0 {
		t.Fatalf("a hyphenation that drops a character left no offset behind, so " +
			"there is nothing here to be mistaken for a cut")
	}

	// And the box it is balanced in must not need the hyphen itself: a fill that
	// already hyphenated would let every width through, and the refusal being
	// tested for would never be reached.
	const full = size20 * 8
	if _, split := br.countLines(items, u(full), 0, 99); split {
		t.Fatalf("the paragraph already breaks a word at %v, so the balancing "+
			"below is not refusing anything", u(full))
	}

	const withoutHyphenating = size20 * 6.6 // 132px, two whole words and one
	got := br.BalanceWidth(items, u(full), 0)
	if got >= u(withoutHyphenating) {
		t.Errorf("balancing a paragraph whose breaks hyphenate returned %v; "+
			"%v is what a search that refuses to hyphenate reaches, and 108px is "+
			"what hyphenating makes possible", got, u(withoutHyphenating))
	}
}

// TestAWidthThatCutAWordIsStillRefused is the other side: the rule the fix has
// to leave alone. A word longer than the box is cut by overflow-wrap, and a
// balance that reached that width would be balancing by breaking words.
func TestAWidthThatCutAWordIsStillRefused(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := itemsOf(t, br, face, "short words and supercalifragilisticexpialidocious",
		WhiteSpaceOf("collapse"), OverflowWrap{Anywhere: false, BreakWord: true})

	const full = size20 * 30
	got := br.BalanceWidth(items, u(full), 0)
	// The long word is 34 characters. A balanced width that cut it would be
	// narrower than the word, and there is no such width this may return.
	if got < u(size20*34) && got != style.MaxUnit {
		t.Errorf("balancing returned %v, which is narrower than the one word the "+
			"paragraph cannot break without cutting it", got)
	}
}

// TestOneFactAboutAHyphenIsAskedInOnePlace.
//
// Whether a line may end here with a hyphen printed was two questions: a
// non-zero width reserved for the hyphen, and a character to print. Two of the
// places that needed it asked differently — the printing wanted both, the room
// reserved wanted only the width — so a face on which the two part company gets
// two different answers about the same item.
//
// They part company both ways, and both are wrong.
func TestOneFactAboutAHyphenIsAskedInOnePlace(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	width := u(size20 * 2)

	// A hyphen with a character and no width: a face whose hyphen glyph has no
	// advance. It is still a hyphen — it is what a reader sees — and reading the
	// width as the claim said this was not a hyphenation point at all, so the
	// word was set unbroken and overflowed.
	t.Run("no width", func(t *testing.T) {
		items := hyphenatingItems(br, face, 3)
		for i := range items {
			if items[i].HyphenText != "" {
				items[i].Hyphen = 0
			}
		}
		line, _, _, _, _, hyphenated := br.BreakOneLine(items, 0, 0, width, 0)
		if !hyphenated {
			t.Fatalf("a hyphen of no width did not hyphenate")
		}
		if last := line[len(line)-1]; last.Text != "-" {
			t.Errorf("the line ends with %q, want the hyphen", last.Text)
		}
	})

	// And a width with no character: room reserved on the line for a hyphen
	// nothing will print, which is a line a hyphen's width short of what it
	// could hold.
	t.Run("no character", func(t *testing.T) {
		items := hyphenatingItems(br, face, 3)
		for i := range items {
			items[i].HyphenText = ""
		}
		plain := hyphenatingItems(br, face, 3)
		for i := range plain {
			plain[i].Hyphen, plain[i].HyphenText, plain[i].HyphenSkip = 0, "", 0
		}
		for _, w := range []float64{size20 * 2, size20 * 3, size20 * 5, size20 * 8} {
			a, _, _, _, _, _ := br.BreakOneLine(items, 0, 0, u(w), 0)
			b, _, _, _, _, _ := br.BreakOneLine(plain, 0, 0, u(w), 0)
			if len(a) != len(b) {
				t.Errorf("at %v a line whose items reserve room for a hyphen "+
					"nothing prints holds %d items and one that reserves none "+
					"holds %d", u(w), len(a), len(b))
			}
		}
	})
}
