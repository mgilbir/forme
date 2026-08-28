package layout

import (
	"unicode"

	"github.com/mgilbir/forme/paragraph"
)

// Gathering the words of an inline formatting context, so that a dictionary can
// be asked about them.
//
// A word is not a box. "high<span>way</span>" is one word written in two, and
// hyphens-span-002 writes it seven ways — around the whole word, around each
// half, with an empty span between them — and asks for one answer from all of
// them. hyphens-out-of-flow-002 does the same with an absolutely positioned
// element written between the letters, which is not in the text at all.
//
// So the points cannot be worked out where the pieces are built, which happens a
// box at a time and cannot see the box after. This is a pass of its own over the
// inline subtree, in the same document order the flattening walks it in, and it
// answers per text box: the rune offsets in *that box's* text where the word it
// is part of may be divided.

// hyphenPointsIn is where the text of an inline formatting context may be
// broken, keyed by the text box each point falls in.
//
// It returns nothing where nothing asked: a subtree with no "hyphens: auto" in a
// language this has patterns for costs one walk and no dictionary lookups.
func (l *layouter) hyphenPointsIn(root *Box) map[*Box][]int {
	g := hyphenGather{out: map[*Box][]int{}}
	g.walk(root)
	g.flush()
	if len(g.out) == 0 {
		return nil
	}
	return g.out
}

// hyphenGather is one word being collected across boxes.
type hyphenGather struct {
	// word is the letters gathered so far.
	word []rune
	// from is where each of those letters came from: the box and the rune
	// offset within its text.
	from []hyphenSource
	out  map[*Box][]int
}

type hyphenSource struct {
	box *Box
	at  int
}

// walk visits the subtree in the order the flattening does.
func (g *hyphenGather) walk(b *Box) {
	for _, child := range b.Children {
		switch {
		case child.Position.outOfFlow() || child.Float != FloatNone:
			// Not in the text either side of it. An overlay hung off the middle
			// of a word does not divide the word, which is what
			// hyphens-out-of-flow-002 asserts by writing one between every pair
			// of letters in turn.
			continue
		case child.Replaced != nil || isAtomicInline(child):
			// A picture is not a letter, so the word ends at it.
			g.flush()
		case child.IsText():
			g.text(child)
		case child.Outer != OuterInline:
			// A block ends the word on both sides of it. Nothing spans two
			// inline formatting contexts, and a word that appeared to would be
			// hyphenated by letters on a different line of a different
			// paragraph.
			g.flush()
			g.walk(child)
			g.flush()
		default:
			g.walk(child)
		}
	}
}

// text adds a text box's letters to the word being gathered, ending the word at
// every character a dictionary has nothing to say about.
func (g *hyphenGather) text(b *Box) {
	// A box that is not asking for automatic hyphenation ends the word rather
	// than joining it. That is the conservative reading of a property that
	// applies to text: "<span style='hyphens:none'>way</span>" has asked for its
	// own letters not to be divided, and dividing the word it is half of would
	// divide them.
	//
	// Asked first because it is the cheap question and almost always the
	// answer: this pass runs over every document, and one that never says
	// "hyphens: auto" should pay a string switch per text box and not a walk up
	// the tree for a language nothing will use.
	if hy, _ := hyphensOf(b.Style["hyphens"]); !hy.Auto {
		g.flush()
		return
	}
	if !hyphenatesLanguage(boxLanguage(b)) {
		g.flush()
		return
	}
	// "line-break: anywhere" turns it off, and §5.3 says so in three words:
	// "Hyphenation is not applied." The value already offers a break between
	// every pair of characters, so a hyphenation point adds no opportunity —
	// what it would add is the *hyphen*, printed at a break the value would have
	// taken anyway. line-break-anywhere-002 is a column one character wide and
	// asks for no red, and a hyphen is a character sticking out of the column.
	if lb, _ := lineBreakOf(b.Style["line-break"]); lb.Anywhere {
		g.flush()
		return
	}
	for i, r := range []rune(b.Text) {
		if !unicode.IsLetter(r) {
			g.flush()
			continue
		}
		g.word = append(g.word, r)
		g.from = append(g.from, hyphenSource{box: b, at: i})
	}
}

// flush asks the dictionary about the word gathered so far and records where it
// said the word may be divided.
func (g *hyphenGather) flush() {
	word, from := g.word, g.from
	g.word, g.from = g.word[:0], g.from[:0]
	if len(word) == 0 {
		return
	}
	// The language is the one the word's letters are in, and every box that
	// contributed to it agreed — text() refuses a box that did not.
	for _, p := range paragraph.HyphenPoints(string(word), boxLanguage(from[0].box), 0, 0) {
		// A point after the p-th letter of the word is a point after the letter
		// from[p-1] came from, which is an offset in *that* box.
		src := from[p-1]
		g.out[src.box] = append(g.out[src.box], src.at+1)
	}
}
