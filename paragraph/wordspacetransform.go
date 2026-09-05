package paragraph

import (
	"strings"
	"unicode/utf8"
)

// word-space-transform, CSS Text 4: making a break opportunity visible.
//
// A zero width space and a <wbr> mark a place a line may end and put nothing on
// the page. That is what an author of English wants and not what an author of
// Japanese wants: Japanese is written without spaces between words, and a reader
// learning it — or a dictionary, or a children's book — wants the word divisions
// shown. The property turns each of those marks into a space one can see.
//
// It is the whole reason both of its values exist. "space" sets a U+0020, which
// is what a Latin-script document would use; "ideographic-space" sets a U+3000,
// which is the width of one character and is what a Japanese document uses,
// because a Latin space between two ideographs looks like a mistake.

// WordSpaceTransform is what the property sets.
type WordSpaceTransform struct {
	// Separator is what a virtual word separator becomes: "" for none, a space,
	// or an ideographic space. It is the character rather than a keyword because
	// the character is what every later stage needs and there are exactly two.
	Separator string
	// AutoPhrase is the value's other half: as well as expanding the separators
	// the document wrote, find the ones it did not.
	//
	// §2.2 gives it the whole of §6.1's phrase detection to work from — "the
	// user agent must detect phrase boundaries", and where one is found with no
	// separator already at it, a *virtual* expandable separator goes there. So
	// the two auto-phrases are one analysis read twice: word-break's decides
	// where a line may end, and this one decides where a space is drawn.
	AutoPhrase bool
}

// Transforms reports whether anything is to be done, which is the question every
// caller actually asks.
func (w WordSpaceTransform) Transforms() bool { return w.Separator != "" }

// Invents reports whether the value asks for separators the document did not
// write. The grammar is "[ space | ideographic-space ] && auto-phrase?", so
// auto-phrase never stands alone — there would be nothing for the separators it
// finds to become.
func (w WordSpaceTransform) Invents() bool { return w.AutoPhrase && w.Separator != "" }

// The two characters, named.
const (
	ordinarySpace    = " "
	ideographicSpace = "　"
)

// WordSpaceTransformOf reads the property, and returns what it could not act on.
//
// The grammar is "none | [ space | ideographic-space ] || auto-phrase", so the
// value is up to two words and auto-phrase may come on either side of the other.
//
// auto-phrase asks for separators to be *invented* at phrase boundaries the
// author did not mark, which takes the same model word-break's own auto-phrase
// takes. It is not reported here: whether the analysis can be done is a question
// about the content language and the text, not about the declaration, and §2.2
// answers it for the UA — "if the content language is unknown, or if the user
// agent does not support detecting phrase boundaries for that language, there
// are no virtual expandable separators". See PhrasesUnfound for what is left to
// report, which is a language that has phrases and no model here.
func WordSpaceTransformOf(value string) (WordSpaceTransform, string) {
	var out WordSpaceTransform
	seen := false
	for _, word := range strings.Fields(strings.ToLower(strings.TrimSpace(value))) {
		switch word {
		case "none":
			// Explicit and the initial value both; nothing to record.
			seen = true
		case "space":
			out.Separator, seen = ordinarySpace, true
		case "ideographic-space":
			out.Separator, seen = ideographicSpace, true
		case "auto-phrase":
			out.AutoPhrase, seen = true, true
		default:
			// Not a value of this property. Nothing is done, and nothing is
			// reported either: an unreadable declaration is the cascade's to
			// report and it drops one before it reaches here.
			return WordSpaceTransform{}, ""
		}
	}
	if !seen {
		return WordSpaceTransform{}, ""
	}
	return out, ""
}

// IsVirtualWordSeparator reports whether a character is one of the marks this
// property expands.
//
// U+200B ZERO WIDTH SPACE is the one a document writes in its text. <wbr> is the
// other and is an element, so it is not a character and cannot be asked here —
// the box that builds it turns it into a U+200B of its own, which is what the
// HTML specification says it is rendered as and is what puts the two on the same
// path through everything below.
func IsVirtualWordSeparator(r rune) bool { return r == 0x200B }

// InsertPhraseSeparators writes the property's separator into the text at every
// phrase boundary that has none already.
//
// §2.2: "If the content language is known and the user agent supports
// linguistic analysis for this language, the user agent must detect phrase
// boundaries. If a word-separator character, other space separator, or U+200B
// ZERO WIDTH SPACE character does not already occur at that boundary, then the
// UA must insert a virtual expandable separator." A virtual separator has no
// existence beyond this — it is not in the document and it is not a break
// opportunity of its own — so it is written into the text here rather than
// carried alongside it, and everything downstream measures, breaks and draws
// what the reader will see. The suite's word-space-transform-030 is the
// statement that this is the right shape: "Transform effects, notably
// transforming virtual word separators into spaces, affect line breaking."
//
// The three documents that say where one may *not* go are the same three
// word-break's auto-phrase has, and for the same reason: a virtual word
// boundary between a letter and an adjacent character of UAX #14's GL, WJ or
// ZWJ classes is a boundary in a place a line may not be divided, so it is not
// a boundary. See isBinding, which is those three classes and is one table.
//
// The text has already had the separators the document *did* write expanded, so
// "already occurs at that boundary" covers them too: a U+200B that became a
// U+3000 is an other space separator by the time this reads it.
func InsertPhraseSeparators(text string, wst WordSpaceTransform, w WritingSystem) string {
	if !wst.Invents() {
		return text
	}
	breaks := PhraseBreaks(text, w)
	if len(breaks) == 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + len(wst.Separator)*len(breaks))
	last := 0
	for at := 1; at < len(text); at++ {
		if boundary, scored := breaks[at]; !scored || !boundary {
			continue
		}
		prev, _ := utf8.DecodeLastRuneInString(text[:at])
		next, _ := utf8.DecodeRuneInString(text[at:])
		if !PhraseSeparatorAt(prev, next) {
			continue
		}
		out.WriteString(text[last:at])
		out.WriteString(wst.Separator)
		last = at
	}
	if last == 0 {
		return text
	}
	return out.String() + text[last:]
}

// SeparatorBoundaries is where §2.2 puts a virtual word separator, keyed by byte
// offset, with true at a boundary.
//
// Two sources, and which one answers is a question about the language rather
// than about the property. §2.2 defers to §6.1's phrase detection, and where
// this engine has a phrase model — Japanese — that is the answer. Where it has
// none but does have the language's *words*, the dictionary that already decides
// where a line may be divided decides where a separator goes: a script written
// without spaces between its words has one place a word separator could belong,
// and it is the place the dictionary names. The suite says so from the other
// side — word-space-transform-018 is Thai, and its reference puts a space at
// every word boundary the dictionary finds.
//
// The dictionary is not offered to §5.2's phrase-based line breaking, and the
// difference is real rather than cautious. A dictionary knows words and
// "auto-phrase" suppresses breaks *inside a phrase*, which is a coarser thing —
// a Japanese bunsetsu is several words. Reading a word list as a phrase model
// would claim knowledge it does not have, in the direction that glues text
// together.
//
// The language gate is §2.2's own: "if the content language is unknown, or if
// the user agent does not support detecting phrase boundaries for that language,
// there are no virtual expandable separators". DictionaryBreaks is keyed by
// character and would answer for untagged Thai; this must not.
//
// # It must be asked of a whole stretch of text
//
// A phrase model reads three characters either side of each offset and degrades
// quietly when it is shown a fragment. A dictionary matches whole words and does
// not: asked about the "สวยง" of "<em>สวยง</em>าม" it finds "สวย" and a leftover
// letter, which is not a word boundary and is not anywhere a separator goes. So
// this is asked of the text a reader sees between one interruption and the next,
// and never of one node of it. See layout/phraseedge.go, which is the caller
// that has that text.
func SeparatorBoundaries(text string, lang Language, w WritingSystem) map[int]bool {
	if HasPhraseModel(w) {
		return PhraseBreaks(text, w)
	}
	if !HasWordDictionary(lang) {
		return nil
	}
	// Returned as it stands. DictionaryBreaks already excepts the first offset
	// — "a break before the first character of a run is not one the run offers"
	// — and an offset at the end of the text is not a place a word begins, so
	// the two boundaries a caller must not be told about are not in it. A pass
	// that trimmed them was written and could not be made to fail.
	return DictionaryBreaks(text)
}

// PhraseSeparatorAt reports whether a virtual separator belongs between two
// characters that a phrase boundary was found between.
//
// It is exported because the boundary is not always inside one run of text: a
// phrase can end where an inline box does, and the caller that inserts a
// separator *there* has the two characters and nothing else. Both callers ask
// the same question and must get the same answer, which is what this being one
// function is for.
//
// The two clauses are §2.2's. A separator already at the boundary is not
// doubled — the text has had the ones the document wrote expanded by the time
// this is asked, so a U+200B that became a U+3000 counts. And a boundary beside
// one of UAX #14's GL, WJ or ZWJ characters is a boundary in a place a line may
// not be divided, so it is not a word boundary at all; see isBinding, which is
// those three classes and is the table word-break's own auto-phrase uses.
func PhraseSeparatorAt(prev, next rune) bool {
	return !separatorAlready(prev) && !separatorAlready(next) &&
		!isBinding(prev) && !isBinding(next)
}

// separatorAlready reports whether a character is one §2.2 counts as a
// separator already occurring at a boundary: a word-separator character, an
// other space separator, or a zero width space.
func separatorAlready(r rune) bool {
	return isWordSeparator(r) || IsOtherSpaceSeparator(r) || IsVirtualWordSeparator(r)
}
