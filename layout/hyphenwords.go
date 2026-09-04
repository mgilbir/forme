package layout

import (
	"strconv"
	"strings"
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

// hyphenLimits is hyphenate-limit-chars: how long a word must be before it may
// be divided, how many letters must stay on the line and how many must go to the
// next.
//
// Zero is "auto" in all three, and auto is the dictionary's own answer rather
// than a number chosen here — the hyphenmins a pattern file states are the ones
// its author decided the language wants.
type hyphenLimits struct{ word, before, after int }

// limitsOf reads hyphenate-limit-chars, which is "auto | <integer>{1,3}".
//
// One value is the word length; two are the word length and the letters before,
// with the letters after taking the same number; three are all of them. A value
// this cannot read leaves every limit at auto, which is the property's initial
// value and the answer a browser gives an unreadable declaration.
func limitsOf(value string) hyphenLimits {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(fields) == 0 || len(fields) > 3 {
		return hyphenLimits{}
	}
	var got [3]int
	for i, f := range fields {
		if f == "auto" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return hyphenLimits{}
		}
		got[i] = n
	}
	out := hyphenLimits{word: got[0], before: got[1], after: got[2]}
	if len(fields) == 2 {
		// "The third value takes the second's" — two numbers are the word and
		// the letters at *both* ends, not the word and the letters before.
		out.after = out.before
	}
	return out
}

// hyphenGather is one word being collected across boxes.
type hyphenGather struct {
	// word is the letters gathered so far.
	word []rune
	// from is where each of those letters came from: the box and the rune
	// offset within its text.
	from []hyphenSource
	// limits are the ones in force where the word began. hyphenate-limit-chars
	// is about a word and a word has one beginning, so the box that started it
	// is the one asked — the same box the language is taken from.
	limits hyphenLimits
	// conditional says the word being gathered has a soft hyphen in it, which
	// is the author dividing it themselves. See flush.
	conditional bool
	out         map[*Box][]int
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
			// Not in the text either side of it, and not a break in it either.
			// An overlay hung off the middle of a word does not divide the word,
			// which is what hyphens-out-of-flow-002 asserts by writing one
			// between every pair of letters in turn — so the word being
			// gathered is set aside rather than flushed, and picked up again
			// where it left off.
			//
			// It still holds text of its own. A float establishes a formatting
			// context and the words in it are words like any other; skipping the
			// subtree meant a floated box never got a hyphenation point at all,
			// however loudly it asked. hyphens-vs-float-clearance-001 is four
			// floated divs each holding one long word, and every one of them
			// came out unhyphenated and overflowing.
			g.inside(child)
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
		if r == softHyphen {
			// §6.3.1: "Automatic hyphenation opportunities elsewhere within a
			// word must be ignored if the word contains a conditional hyphen
			// (&shy; or U+00AD SOFT HYPHEN), in favor of the conditional
			// hyphen(s)." An author who has divided a word has said where it
			// divides, and a dictionary that adds three more places is
			// overruling them.
			//
			// So the mark neither ends the word nor joins it: the letters
			// either side are one word — "frag&shy;ilistic" is one word divided
			// once and not two words — and the word it makes takes no automatic
			// points at all. Ending the word here instead is what this did, and
			// it hyphenated each half on its own.
			g.conditional = true
			continue
		}
		if !unicode.IsLetter(r) {
			g.flush()
			continue
		}
		if len(g.word) == 0 {
			g.limits = limitsOf(b.Style["hyphenate-limit-chars"])
		}
		g.word = append(g.word, r)
		g.from = append(g.from, hyphenSource{box: b, at: i})
	}
}

// flush asks the dictionary about the word gathered so far and records where it
// said the word may be divided.
func (g *hyphenGather) flush() {
	word, from, conditional := g.word, g.from, g.conditional
	g.word, g.from, g.conditional = g.word[:0], g.from[:0], false
	if len(word) == 0 || conditional {
		// A word the author divided takes the division they wrote and no
		// others. The section allows one exception — "if, even after breaking
		// at such opportunities, a portion of that word is still too long to
		// fit on one line, an automatic hyphenation opportunity *may* be used"
		// — and it is a may, so a UA that offers none is conforming and this
		// one offers none.
		return
	}
	limits := g.limits
	if limits.word > len(word) {
		// Too short to be divided at all, which is the property's first value.
		return
	}
	// The language is the one the word's letters are in, and every box that
	// contributed to it agreed — text() refuses a box that did not.
	points := paragraph.HyphenPoints(string(word), boxLanguage(from[0].box),
		limits.before, limits.after)
	for _, p := range points {
		// A point after the p-th letter of the word is a point after the letter
		// from[p-1] came from, which is an offset in *that* box.
		src := from[p-1]
		g.out[src.box] = append(g.out[src.box], src.at+1)
	}
}

// inside gathers an out-of-flow box's own text without disturbing the word being
// gathered around it.
//
// The two are different formatting contexts and neither is part of the other's
// words: the box's first letter does not continue the word outside it, and the
// letter after the box does not continue a word inside it. So the state is set
// aside and put back, which is the difference between this and the flush-walk-
// flush a block-level child gets — a block *ends* the word around it and an
// out-of-flow box does not.
func (g *hyphenGather) inside(b *Box) {
	word, from, limits, cond := g.word, g.from, g.limits, g.conditional
	g.word, g.from, g.limits, g.conditional = nil, nil, hyphenLimits{}, false
	g.walk(b)
	g.flush()
	g.word, g.from, g.limits, g.conditional = word, from, limits, cond
}

// softHyphen is U+00AD, the mark an author writes inside a word to say where it
// divides.
const softHyphen = '\u00ad'
