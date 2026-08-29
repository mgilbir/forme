package paragraph

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// text-autospace, CSS Text 4: the space a typesetter puts between an ideograph
// and the Latin word next to it.
//
// Japanese and Chinese are set without word spaces, so a Latin word or a number
// dropped into a line of them has nothing separating it from the ideographs on
// either side. Typography has an answer that predates CSS by a century — a
// quarter-space, or in this specification an eighth of the ideographic advance —
// and the property is what turns it on. Its initial value turns it on, which is
// the part worth saying twice: a document that says nothing about the property
// still gets the spacing, because a page of Japanese set without it is a page
// set wrong.
//
// It is spacing rather than a character. Nothing is added to the text a reader
// copies out of the page, no line may break at it, and it is not a word
// separator that word-spacing would then space again.

// Autospace is which of §8.1's classes of boundary get the spacing.
//
// The zero value inserts nothing, which is "no-autospace". The initial value is
// "normal", which the parser expands to the two ideograph boundaries — see
// AutospaceOf, where the third class is also explained.
type Autospace struct {
	// IdeographAlpha spaces an ideograph against a non-ideographic *letter*.
	IdeographAlpha bool
	// IdeographNumeric spaces an ideograph against a non-ideographic *numeral*.
	IdeographNumeric bool
}

// Any reports whether this value asks for anything at all, which is the test
// that keeps the walk off a document that turned the property off.
func (a Autospace) Any() bool { return a.IdeographAlpha || a.IdeographNumeric }

// AutospaceOf reads the property. The second result is the part to report as
// unimplemented, on the model of WordBreakOf.
//
// "normal" is "ideograph-alpha ideograph-numeric": §8.1 gives the keyword that
// meaning, and every browser that ships the property ships it on by default.
//
// The third class the grammar allows is "punctuation", which asks for spacing
// around full-width punctuation and is a different rule with different
// arithmetic — it is read and reported rather than silently taken as one of
// these two. "insert" and "replace" say what to do where the author already
// wrote a space; only "insert" is implemented, which is the value that adds
// spacing where there was none, and "replace" is reported.
func AutospaceOf(value string) (Autospace, string) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "normal" {
		return Autospace{IdeographAlpha: true, IdeographNumeric: true}, ""
	}
	if value == "no-autospace" {
		return Autospace{}, ""
	}
	var out Autospace
	unhandled := ""
	for _, word := range strings.Fields(value) {
		switch word {
		case "ideograph-alpha":
			out.IdeographAlpha = true
		case "ideograph-numeric":
			out.IdeographNumeric = true
		case "insert":
			// The initial half of the pair, and what this engine does: spacing
			// is added at a boundary that has none.
		default:
			// "punctuation", "replace", and anything else the grammar allows.
			// The first value wins the report, so a document naming two gets one
			// finding rather than a list.
			if unhandled == "" {
				unhandled = word
			}
		}
	}
	return out, unhandled
}

// IsAutospaceIdeograph reports whether a character is one of §8.1's ideographs.
//
// The specification names the scripts rather than a property, and the scripts
// are the ones written without word spaces: Han, and the two Japanese syllabaries
// that are set among it. Bopomofo and Yi are here with them because they are
// used the same way and are what "ideograph" means in the sentence, and because
// leaving them out would space a Bopomofo annotation and not the Han it
// annotates.
//
// The iteration marks and the prolonged sound mark are ideographs by use rather
// than by script — U+3005 repeats the Han character before it, U+30FC lengthens
// the kana before it — and Unicode gives them the Common script, so a test on
// the script alone would put a boundary in the middle of a Japanese word.
func IsAutospaceIdeograph(r rune) bool {
	switch r {
	case 0x3005, // IDEOGRAPHIC ITERATION MARK
		0x3006, // IDEOGRAPHIC CLOSING MARK
		0x3007, // IDEOGRAPHIC NUMBER ZERO
		0x303B, // VERTICAL IDEOGRAPHIC ITERATION MARK
		0x30FC, // KATAKANA-HIRAGANA PROLONGED SOUND MARK
		0x30A0, // KATAKANA-HIRAGANA DOUBLE HYPHEN
		0x309D, // HIRAGANA ITERATION MARK
		0x309E, // HIRAGANA VOICED ITERATION MARK
		0x30FD, // KATAKANA ITERATION MARK
		0x30FE: // KATAKANA VOICED ITERATION MARK
		return true
	}
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Bopomofo, r) ||
		unicode.Is(unicode.Yi, r)
}

// IsAutospaceLetter reports whether a character is one of §8.1's *non-ideographic*
// letters, which is the other side of an ideograph-alpha boundary.
//
// A letter that is not an ideograph, and the halfwidth kana are carved out of it
// by the test above rather than here: they are Katakana script, so they are
// ideographs to this and no boundary is put between a halfwidth kana and the
// fullwidth one beside it.
func IsAutospaceLetter(r rune) bool {
	return unicode.IsLetter(r) && !IsAutospaceIdeograph(r)
}

// IsAutospaceNumeral reports whether a character is one of §8.1's
// non-ideographic numerals: a decimal digit that is not full-width.
//
// The full-width digits are excluded because they are set on the ideographic
// advance and among ideographs — "第１章" is one word to a reader — and putting an
// eighth of an em on each side of the digit would break it apart.
func IsAutospaceNumeral(r rune) bool {
	if r >= 0xFF10 && r <= 0xFF19 {
		return false
	}
	return unicode.Is(unicode.Nd, r)
}

// AutospaceBase is the character a boundary is judged by when combining marks
// stand next to it.
//
// §8.1 is stated over typographic character units, and a mark is part of the one
// before it: "c" with an acute over it is a Latin letter however many marks
// follow, and the boundary after it is the same boundary. The suite writes it as
// text-autospace-elements-006 — "a<COMBINING ACUTE>b<COMBINING ACUTE>c<COMBINING
// ACUTE>永" — whose reference puts the spacing exactly where the unmarked text
// would have it.
func AutospaceBase(r rune) bool {
	return !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Me, r) &&
		!unicode.Is(unicode.Mc, r) && !IsDefaultIgnorable(r)
}

// LastAutospaceBase and FirstAutospaceBase are the characters a boundary is
// judged by: the last base character before it and the first base character
// after it.
//
// Both skip the marks and the invisibles, which is AutospaceBase's business, and
// both answer false where a run has no base of its own. That is not a corner: a
// variation selector is a run's whole content often enough — "国<VS>A" hands the
// selector its own run when the face for it differs — and the boundary is then
// between the characters on either side of that run rather than with it. The
// caller keeps walking.
func LastAutospaceBase(s string) (rune, bool) { return lastBase(s) }

// FirstAutospaceBase is LastAutospaceBase from the other end.
func FirstAutospaceBase(s string) (rune, bool) { return firstBase(s) }

func lastBase(s string) (rune, bool) {
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		if AutospaceBase(r) {
			return r, true
		}
	}
	return 0, false
}

func firstBase(s string) (rune, bool) {
	for _, r := range s {
		if AutospaceBase(r) {
			return r, true
		}
	}
	return 0, false
}

// AutospaceAt reports whether §8.1 puts spacing between two characters, given
// what the element containing both of them asked for.
//
// The rule is symmetric: it is a boundary *between* an ideograph and something
// else, and which side the ideograph is on does not change the answer.
func AutospaceAt(a, b rune, as Autospace) bool {
	if !as.Any() {
		return false
	}
	switch {
	case IsAutospaceIdeograph(a):
		return (as.IdeographAlpha && IsAutospaceLetter(b)) ||
			(as.IdeographNumeric && IsAutospaceNumeral(b))
	case IsAutospaceIdeograph(b):
		return (as.IdeographAlpha && IsAutospaceLetter(a)) ||
			(as.IdeographNumeric && IsAutospaceNumeral(a))
	}
	return false
}

// SplitAtAutospace cuts a run of text where §8.1 puts spacing inside it.
//
// The spacing is a gap *between* two runs, because that is the only shape a
// backend can be handed — one advance per run, and the layout places each run
// itself. A boundary inside a single run therefore has to become a boundary
// between two, and this is what makes one.
//
// It is a cut and nothing else: no character is added or removed, the pieces
// join back into the original text, and the caller gives the second piece no
// break opportunity it did not already have. What it changes is that there is
// now an edge for the gap to open at.
//
// The common case returns the input untouched with no allocation: a run with no
// ideograph in it, or none beside a letter, has nothing to cut.
func SplitAtAutospace(text string, as Autospace) []string {
	if !as.Any() || text == "" {
		return nil
	}
	var out []string
	start, prev, havePrev := 0, rune(0), false
	for i, r := range text {
		if !AutospaceBase(r) {
			// A mark or an invisible belongs to the character before it and
			// cannot begin a unit, so it is never a place to cut.
			continue
		}
		if havePrev && AutospaceAt(prev, r, as) {
			out = append(out, text[start:i])
			start = i
		}
		prev, havePrev = r, true
	}
	if out == nil {
		return nil
	}
	return append(out, text[start:])
}
