package paragraph

import (
	"unicode/utf8"

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
func SpacedUnits(text string) int {
	n := 0
	for _, r := range text {
		if IsDefaultIgnorable(r) {
			continue
		}
		n++
	}
	return n
}

// IsDefaultIgnorable is the part of Unicode's Default_Ignorable_Code_Point
// property this engine meets: the bidi controls, the joiners, and the marks that
// are there to say something to an algorithm rather than to be seen.
func IsDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x00AD, // soft hyphen
		r == 0x034F,                // combining grapheme joiner
		r >= 0x200B && r <= 0x200F, // zero width space through RLM
		r >= 0x202A && r <= 0x202E, // the embedding and override controls
		r >= 0x2060 && r <= 0x2064, // word joiner and the invisible operators
		r >= 0x2066 && r <= 0x2069, // the isolates
		r == 0xFEFF:                // zero width no-break space
		return true
	}
	return false
}

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
