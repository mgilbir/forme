package paragraph

import (
	"unicode"

	"unicode/utf8"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What letter-spacing and word-spacing add to a run.
//
// CSS Text §8 states both over typographic character units and word-separator
// characters, which are questions about the text. Which values a particular box
// asked for is a question about the document, and stays with the layout engine.

// TextSpacing is the pair of extra advances, in layout units.
//
// The zero value is what "normal" means for both, which is what makes it cheap:
// a document that sets neither compares equal to the zero key and shares one
// cache entry per word with every other such document.
type TextSpacing struct {
	// Letter is added after every typographic character unit, including the last
	// one of a run. That is what CSS Text §8.2 says and what browsers do, and it
	// is why "Letter-spacing: 1em" on a single word leaves a gap after it.
	Letter style.Unit
	// Word is added to every Word-separator character.
	Word style.Unit
}

// SpacingAdvance is what the two properties add to a run of text.
//
// It walks the string once, without decoding it into runes: a text node is
// untrusted and arbitrarily large, and counting characters through a []rune
// would buffer four bytes per character to answer a question about a length.
// utf8.RuneCountInString does the same walk without the copy.
func SpacingAdvance(text string, sp TextSpacing) style.Unit {
	var out style.Unit
	if sp.Letter != 0 {
		out = out.Add(sp.Letter.Mul(float64(SpacedUnits(text))))
	}
	if sp.Word != 0 {
		out = out.Add(sp.Word.Mul(float64(countWordSeparators(text))))
	}
	return out
}

// SpacedUnits counts what letter-spacing is added between.
//
// CSS Text §8.2 spaces "adjacent typographic character units", and a code point
// the shaper drops before choosing any glyph is not one of them: it draws
// nothing and it is in the text only because the text is what a reader copies
// out of the page. Counting one gave a bidi override an em of its own under
// "letter-spacing: 1em", which pushed every letter after it along — the suite
// writes that document eight times over, as the second half of each
// bidi-text/bidi-00N pair.
//
// Grapheme clusters would be the exact reading of "typographic character unit"
// and this counts runes, so a base and its combining mark are still spaced
// apart. That is a separate question with a separate answer, and it is not
// mixed in here.
//
// # Cursive tracking
//
// §8.2 again: "spacing must not be inserted between the characters of a cursive
// script, as it would break the cursive connection". So a character of one is
// not a unit spacing goes after — not because it joins to what follows, which
// depends on the pair, but because it is *in* such a script, which does not.
// The suite states the rule that way and not the other:
// letter-spacing-cursive-001 asks for Arabic set with and without a
// letter-spacing to render identically, and "مرحباً" has an unjoined pair in the
// middle of it.
//
// It applies to the character rather than to its neighbours, so an Arabic word
// followed by a space gets no spacing after its last letter and one after the
// space. letter-spacing-cursive-002 is exactly that arithmetic — two Arabic
// words, "letter-spacing: 1em", and a reference that inserts one em and not two.
func SpacedUnits(text string) int {
	n := 0
	scanCursiveTracking(text, func(_ int, suppressed bool) {
		if !suppressed {
			n++
		}
	})
	return n
}

// IsCursiveScript reports whether a character belongs to a script whose letters
// join. See shape.InCursiveScript for why it is the script and not the pair.
func IsCursiveScript(r rune) bool { return shape.InCursiveScript(r) }

// scanCursiveTracking walks the characters letter-spacing could go after and
// says of each whether §8.2 forbids it, so that the count and the cut cannot
// answer differently.
//
// A combining mark is the case neither of them could answer alone: Unicode's
// joining table names the letters of the cursive scripts and not the marks
// written on them, so a fathatan reads as an ordinary character and collects a
// spacing of its own — which is what left "مرحباً" ten pixels wider than the same
// word without a letter-spacing, in letter-spacing-cursive-001. So a mark takes
// the answer of the base it sits on.
//
// It takes it *only* from a cursive base, which keeps the change to the script
// this rule is about. A mark on a Latin letter still counts as a unit of its
// own, as it always has — that is the grapheme-cluster question SpacedUnits
// records as a separate one, and it is still separate.
func scanCursiveTracking(text string, fn func(i int, suppressed bool)) {
	cursive := false
	for i, r := range text {
		switch {
		case IsDefaultIgnorable(r):
			// Nothing is drawn for it and nothing goes after it.
			continue
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
			fn(i, cursive)
			continue
		}
		cursive = IsCursiveScript(r)
		fn(i, cursive)
	}
}

// IsDefaultIgnorable reports whether nothing is drawn for a character and it
// occupies no width, which for §8.2 is the question "is this a typographic
// character unit letter-spacing goes after".
//
// It is Unicode's Default_Ignorable_Code_Point, less the Hangul fillers, which
// are letters. The answer comes from the shaping package, which holds the
// property as a generated table, because this used to be a list written here by
// hand: it had the characters somebody had met — the bidi controls, the joiners,
// the word joiner and the invisible operators — and stopped at U+2064, so each
// of the six deprecated format controls above it collected a letter-spacing of
// its own and a document using one came out wider than it should be.
func IsDefaultIgnorable(r rune) bool { return shape.DrawsNothing(r) }

// IsBidiControl reports whether a character is one of Unicode's Bidi_Control
// characters: a mark whose whole function is to instruct the bidirectional
// algorithm.
//
// It is narrower than IsDefaultIgnorable and the difference matters. Both sets
// hold characters nothing is drawn for, and only these are *transparent to the
// text*: a zero-width space is default-ignorable and is a character that stands
// between its neighbours — §4.1.1 does not collapse two spaces across one — while
// a bidi control is not there at all as far as the text is concerned, and two
// spaces with one between them are adjacent and collapse.
//
// The suite tests both, in opposite directions and with the same shape of
// document. See Piece.ZeroWidth for the other half.
func IsBidiControl(r rune) bool {
	switch r {
	case 0x061C, // arabic letter mark
		0x200E, 0x200F: // left-to-right and right-to-left marks
		return true
	}
	return (r >= 0x202A && r <= 0x202E) || // the embeddings and overrides
		(r >= 0x2066 && r <= 0x2069) // the isolates
}

// countWordSeparators counts the characters word-spacing applies to.
//
// CSS Text §8.1 names a short list of word-separator characters. The two that
// occur in real documents are the ordinary space and the no-break space; the
// remaining four are Ethiopic and Aegean word separators, which are counted too
// because leaving them out would make the property silently do nothing in the
// documents that need it most.
func countWordSeparators(text string) int {
	n := 0
	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		i += size
		switch r {
		case ' ', '\u00a0', // space, and the no-break space an author writes
			'\u1361',                   // Ethiopic wordspace
			'\U00010100', '\U00010101', // Aegean word separators
			'\U0001039F', '\U0001091F': // Ugaritic and Phoenician
			n++
		}
	}
	return n
}

// SplitAtCursiveTracking cuts text where §8.2's cursive tracking begins or ends,
// so that every run either takes letter-spacing after each of its characters or
// takes none.
//
// It exists for the backend and not for the rule. A display list carries one
// letter-spacing per run — an advance added after every glyph — so a run holding
// an Arabic letter beside a Latin one cannot say that only one of them is
// followed by a gap. Cutting at the change makes each run uniform, and the
// question the drawing has to answer becomes the question this file answers.
//
// It is the same argument, and the same shape, as the cut §8.1's ideograph
// spacing needs. See SplitAtAutospace.
//
// Nothing is returned where there is nothing to cut, which is every run of every
// document that does not mix a cursive script with another — so a caller pays a
// scan and no allocation.
func SplitAtCursiveTracking(text string) []string {
	var out []string
	start, prev, havePrev := 0, false, false
	scanCursiveTracking(text, func(i int, suppressed bool) {
		if havePrev && suppressed != prev {
			out = append(out, text[start:i])
			start = i
		}
		prev, havePrev = suppressed, true
	})
	if out == nil {
		return nil
	}
	return append(out, text[start:])
}
