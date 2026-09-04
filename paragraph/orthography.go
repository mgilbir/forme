package paragraph

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CSS Text §6.3's other half: what a language does to a word broken inside it,
// beyond printing a hyphen.
//
// §6.3.1 says it of "hyphens: manual" in one sentence — the UA "must use the
// appropriate language-specific hyphenation character(s) and should apply any
// appropriate spelling changes just as for automatic hyphenation at the same
// point" — and §6.3 gives the table it is about:
//
//	English     Unbroken     Un‐         broken
//	Dutch       cafeetje     café‐       tje
//	Hungarian   Összeg       Ösz‐        szeg
//	Mandarin    tú’àn        tú‐         àn
//	Mandarin    àizēng‐fēnmíng  àizēng‐  ‐fēnmíng
//	Uyghur      داميدى        دامي‍ـ       ‍دى
//	Cree        ᑲᓯᑕᓂᐘᓂᓂᐠ      ᑲᓯᑕᓂ᐀        ᐘᓂᓂᐠ
//
// English is the row that needs nothing, and it is the row every engine
// implements. The rest are four different things: a hyphen character that is
// not a hyphen, letters restored at the end of the line, a character dropped
// from the start of the next one, and a joining control at both.
//
// # What is here and what is not
//
// Hungarian, Mandarin in the Latin alphabet, and Uyghur. Each of those is a
// rule over the characters at the break, so it can be applied to any word.
//
// Dutch is not, and the reason is worth stating: "cafeetje" divides as
// "café‐tje" because the doubled vowel is a spelling artefact of a French
// loanword, and "zeetje" — the same five letters at the break — divides as
// "zee‐tje". No rule over the characters can tell them apart, so Dutch needs a
// list of the words it is true of, which is a dictionary rather than an
// orthography. Cree is not here either: the rule is one character, U+1400
// CANADIAN SYLLABICS HYPHEN, but its document needs a syllable to break at and
// nothing here finds one.

// Orthography is a language's rules for a word broken inside it.
type Orthography uint8

const (
	// OrthographyPlain prints a hyphen and changes nothing, which is English
	// and almost everything else.
	OrthographyPlain Orthography = iota
	// OrthographyHungarian writes a doubled digraph out in full on both sides
	// of the break: "Összeg" is "Ösz-" and "szeg", not "Ös-" and "szeg".
	OrthographyHungarian
	// OrthographyPinyin drops the apostrophe that separates two syllables,
	// because the hyphen has taken over its job: "tú’àn" is "tú-" and "àn".
	OrthographyPinyin
	// OrthographyUyghur hyphenates with U+0640 ARABIC TATWEEL and keeps the
	// letters joined across the break.
	OrthographyUyghur
)

// OrthographyOf reads a language tag as the rules its text is broken by.
//
// The whole tag and not the primary subtag, which is the difference between
// this and LanguageOf: "zh" is Chinese and has no hyphenation to speak of,
// "zh-Latn" is Chinese written in the Latin alphabet and is what these rules
// are about. That is the same reading WritingSystemOf makes of the same
// attribute, and for the same reason — the script decides.
func OrthographyOf(tag string) Orthography {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return OrthographyPlain
	}
	primary, rest, _ := strings.Cut(tag, "-")
	script := ""
	for rest != "" {
		var sub string
		sub, rest, _ = strings.Cut(rest, "-")
		// A script subtag is four letters, which is what tells it from a region
		// (two letters or three digits) and from a variant. The first one wins.
		if len(sub) == 4 && isAlpha(sub) {
			script = sub
			break
		}
	}
	switch primary {
	case "hu":
		return OrthographyHungarian
	case "zh":
		// Romanised Chinese, which in practice means pinyin — the suite writes
		// the variant as well, "zh-Latn-pinyin", and the rule is the same for
		// any romanisation that separates syllables with an apostrophe.
		if script == "latn" {
			return OrthographyPinyin
		}
	case "ug":
		// Uyghur in its own alphabet. Written in Latin — "ug-Latn" is a
		// romanisation this has nothing to say about — it is an ordinary
		// hyphen.
		if script == "" || script == "arab" {
			return OrthographyUyghur
		}
	}
	return OrthographyPlain
}

// Hyphenation is what happens to a word broken between two stretches of text,
// beyond the hyphen itself.
//
// The four fields are the four things §6.3's table asks for, and every one of
// them is empty for English.
type Hyphenation struct {
	// Restored is spelling put back at the end of the broken line, in front of
	// the hyphen. Hungarian's second half of a digraph is one.
	Restored string
	// Character is the hyphen the language prints, or the empty string for the
	// one the engine would choose. Uyghur's tatweel is one.
	//
	// It does not overrule the document: "hyphenate-character" is what the
	// author said and this is what the language says, and the author wins.
	Character string
	// Dropped is how many bytes are taken off the start of the next line: a
	// character the hyphen has replaced. Pinyin's apostrophe is one.
	Dropped int
	// Lead is text put at the start of the next line. Uyghur's zero width
	// joiner is one — the letters either side of a break are still shaped as
	// though the word were whole, and the control is what says so to a shaper
	// that has only the next line to look at.
	Lead string
}

// Any reports whether the language asks for anything at all here, which for
// almost every document is no.
func (h Hyphenation) Any() bool {
	return h.Restored != "" || h.Character != "" || h.Dropped != 0 || h.Lead != ""
}

// HyphenateBetween is what the language does to a word broken between before
// and after.
//
// before is the text that would end the line and after the text that would
// begin the next one, each as much of it as the rules read — a few characters
// either side is all any of them need.
//
// The soft hyphen the author wrote is at the end of before, because a piece
// keeps the character that marked it; so are the joiners and the other controls
// that draw nothing. None of them is the letter these rules are about, and every
// one of them was a rule that silently did nothing until it was trimmed here.
func (o Orthography) HyphenateBetween(before, after string) Hyphenation {
	before = strings.TrimRightFunc(before, IsDefaultIgnorable)
	switch o {
	case OrthographyHungarian:
		return hungarianDigraph(before, after)
	case OrthographyPinyin:
		return pinyinApostrophe(after)
	case OrthographyUyghur:
		return uyghurTatweel(before, after)
	}
	return Hyphenation{}
}

// hungarianDigraphs are the two- and three-letter consonants Hungarian spells
// with more than one letter.
//
// A doubled one is written with the first letter twice — "ssz" is two "sz" —
// and dividing the word puts the whole digraph on each side. The list is the
// alphabet's own: cs, dz, dzs, gy, ly, ny, sz, ty, zs, and no others.
var hungarianDigraphs = []string{"dzs", "cs", "dz", "gy", "ly", "ny", "sz", "ty", "zs"}

// hungarianDigraph restores the rest of a digraph the break cut in half.
//
// "Ös" and "szeg" is "Összeg" divided between its two z-sounds: the word is
// written with one "s" and one "sz" and is pronounced with two "sz", so the
// line ends "Ösz-" and the next begins "szeg". What identifies the case is that
// the text after the break begins with a digraph whose *first* letter is the
// one the text before it ends with.
//
// It is a rule and not a dictionary, so it fires on a compound that happens to
// have the same letters at a break the author marked — "kis" and "szoba" would
// become "kisz-". That is the rule Hungarian typesetting uses and the risk it
// carries; the author put the soft hyphen there, and a word this is wrong for
// is a word to mark differently.
func hungarianDigraph(before, after string) Hyphenation {
	last, size := utf8.DecodeLastRuneInString(before)
	if size == 0 || !unicode.IsLetter(last) {
		return Hyphenation{}
	}
	lower := strings.ToLower(after)
	for _, digraph := range hungarianDigraphs {
		if !strings.HasPrefix(lower, digraph) {
			continue
		}
		head, rest := utf8.DecodeRuneInString(digraph)
		if unicode.ToLower(last) != head {
			continue
		}
		// The case of the letters as the document wrote them, which is the
		// text after the break: "Ös|szeg" restores "z" and "ÖS|SZEG" "Z".
		return Hyphenation{Restored: after[rest:len(digraph)]}
	}
	return Hyphenation{}
}

// pinyinApostrophe drops the syllable separator the hyphen has replaced.
//
// Pinyin writes an apostrophe where two syllables would otherwise be read as
// one — "tú’àn" is two syllables and "tuán" is one — and a line broken there
// has the hyphen doing that work, so the apostrophe goes. Both spellings of it
// are taken: U+2019 is the typographic one and the suite writes it, and U+0027
// is what a keyboard produces.
func pinyinApostrophe(after string) Hyphenation {
	r, size := utf8.DecodeRuneInString(after)
	if r == '\'' || r == '’' {
		return Hyphenation{Dropped: size}
	}
	return Hyphenation{}
}

// uyghurTatweel hyphenates with the character Arabic script uses for it.
//
// U+0640 ARABIC TATWEEL is the kashida, the stroke that extends a joined
// letter, and it is what the Arabic script draws at the end of a divided word —
// a hyphen there would be a Latin mark in the middle of a Uyghur word. The
// joiners either side are what §6.3's own note asks for: "when shaping scripts
// such as Arabic are allowed to break within words due to hyphenation, the
// characters are still shaped as if the word were not broken", so the letter
// before the break keeps its medial form and the letter after it keeps its
// initial one.
//
// Only where the letters either side are joining ones. A break beside a
// character that joins nothing — a digit, a full stop, the end of the text —
// has nothing to keep joined, and a tatweel drawn there would be a stroke
// hanging off nothing.
func uyghurTatweel(before, after string) Hyphenation {
	last, size := utf8.DecodeLastRuneInString(before)
	if size == 0 || !IsCursiveScript(last) {
		return Hyphenation{}
	}
	h := Hyphenation{Character: "ـ", Restored: "‍"}
	if r, n := utf8.DecodeRuneInString(after); n > 0 && IsCursiveScript(r) {
		h.Lead = "‍"
	}
	return h
}
