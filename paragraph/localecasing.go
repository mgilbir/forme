package paragraph

import (
	"strings"
	"unicode"

	"github.com/mgilbir/forme/shape"
)

// The case mappings that depend on something other than the character.
//
// SpecialCasing.txt states most of its entries unconditionally — those are
// casingtable.go, generated — and sixteen of them with a condition attached.
// Three of the conditions name a language and one names the text around the
// character, and none of them is a table: what they need is code, so they are
// written here with the file's own lines quoted beside them.
//
//	03A3; 03C2; 03A3; 03A3; Final_Sigma
//	0130; 0069; 0130; 0130; tr        and az
//	0069; 0069; 0130; 0130; tr        and az
//	0049; 0131; 0049; 0049; tr Not_Before_Dot      and az
//	0307;     ; 0307; 0307; tr After_I             and az
//	0049; 0069 0307; 0049; 0049; lt More_Above
//	004A; 006A 0307; 004A; 004A; lt More_Above
//	012E; 012F 0307; 012E; 012E; lt More_Above
//	00CC; 0069 0307 0300; 00CC; 00CC; lt
//	00CD; 0069 0307 0301; 00CD; 00CD; lt
//	0128; 0069 0307 0303; 0128; 0128; lt
//	0307;     ; 0307; 0307; lt After_Soft_Dotted
//
// # Why this is not a nicety
//
// Turkish and Azerbaijani have two i's — dotted and dotless — and they are
// different letters, not different shapes of one. Lowercasing "I" to "i" in
// Turkish is the same class of error as lowercasing "O" to "e" in English: it
// spells a different word. The Greek final sigma is the same kind of thing at a
// smaller scale, and is not about language at all: σ at the end of a word is ς,
// and a reader of Greek sees the difference immediately.
//
// # What is not here
//
// The tailorings that are not in SpecialCasing.txt: Greek uppercasing dropping
// its accents, and Dutch titlecasing IJ as two capitals. Both are real, both are
// in CLDR rather than in the UCD, and neither is a conditional mapping of the
// kind this file is about.

// Language is the primary subtag of an element's language, lowercased — "tr",
// "lt", "el" — or empty where the document said nothing.
//
// The primary subtag and not the whole tag: "tr-CY" is Turkish, and a case
// mapping that only recognised "tr" would set a Cypriot Turkish page in the
// wrong alphabet.
type Language string

// LanguageOf reads a lang attribute's value as one of these.
//
// A tag that names a script the language's tailoring is not about gives no
// language at all, and the suite forces that rather than leaving it to taste:
// writing-system-text-transform-001 is Turkish written in Cyrillic —
// lang="tr-Cyrl" — and asks for the I to lowercase to a *dotted* i. The dotless
// one is a letter of the Turkish Latin alphabet and there is no such letter in
// Cyrillic, so the tailoring is about the script as much as the language.
//
// Which script that is depends on the language, and "not Latin" would be the
// wrong test: Greek drops its accents in the Greek alphabet, so "el-Grek" is
// Greek and "el-Latn" — Greek transliterated — is not.
//
// A tag with no script subtag is taken at its word: "tr" is Turkish in the
// alphabet Turkish is written in.
func LanguageOf(tag string) Language {
	tag = strings.ToLower(strings.TrimSpace(tag))
	primary, rest, _ := strings.Cut(tag, "-")
	for rest != "" {
		var sub string
		sub, rest, _ = strings.Cut(rest, "-")
		// A script subtag is four letters, which is what distinguishes it from
		// a region (two letters or three digits) and from a variant.
		if len(sub) == 4 && isAlpha(sub) {
			if want, ok := tailoredScript[primary]; ok && sub != want {
				return ""
			}
			break
		}
	}
	return Language(primary)
}

// tailoredScript is the alphabet each tailored language is tailored *in*.
//
// A language not named here has no tailoring, so what script it is written in
// decides nothing and is not asked about.
var tailoredScript = map[string]string{
	"tr": "latn", "az": "latn", "lt": "latn", "nl": "latn",
	"el": "grek",
}

// isAlpha reports whether every byte of a subtag is a letter, which the tag
// syntax makes an ASCII question.
func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// dotless says the language writes an undotted lower-case i, which is Turkish
// and Azerbaijani.
func (l Language) dotless() bool { return l == "tr" || l == "az" }

// keepsDot says the language keeps a dot above a lowercased letter that had
// one, which is Lithuanian.
func (l Language) keepsDot() bool { return l == "lt" }

// localeUpper is the language-dependent part of uppercasing one character, and
// says whether it applied.
//
// The uppercase side is the short one: only the dotted and dotless i differ, and
// only in Turkish and Azerbaijani.
func localeUpper(r rune, lang Language) (string, bool) {
	if lang.dotless() && r == 'i' {
		// "0069; 0069; 0130; 0130; tr" — an ordinary i uppercases to the dotted
		// capital, because the dotless one is a different letter with a capital
		// of its own.
		return "İ", true
	}
	return "", false
}

// localeLower is the language-dependent part of lowercasing one character.
//
// before and after are the rest of the run either side of it, which three of the
// conditions read.
func localeLower(r rune, before, after string, lang Language) (string, bool) {
	switch {
	case lang.dotless() && r == 0x0130:
		// "0130; 0069; ...; tr" — the dotted capital lowercases to a plain i,
		// where the unconditional mapping keeps the dot as a separate mark.
		return "i", true
	case lang.dotless() && r == 'I' && !beforeDot(after):
		// "0049; 0131; ...; tr Not_Before_Dot" — a capital I lowercases to the
		// dotless ı, unless a combining dot above follows, in which case the two
		// together are the dotted i and the dot is removed instead.
		return "ı", true
	case lang.dotless() && r == 0x0307 && afterI(before):
		// "0307; ; ...; tr After_I" — that removal.
		return "", true
	case lang.keepsDot() && r == 0x0307 && afterSoftDotted(before):
		// "0307; ; ...; lt After_Soft_Dotted" — Lithuanian keeps one dot and
		// not two, so an explicit one after a letter that already has it goes.
		return "", true
	case lang.keepsDot() && moreAbove(after):
		// "0049; 0069 0307; ...; lt More_Above" and its two neighbours. A
		// Lithuanian lower-case i keeps its dot under an accent, which is the
		// opposite of what every other language does with one.
		switch r {
		case 'I':
			return "i̇", true
		case 'J':
			return "j̇", true
		case 0x012E:
			return "į̇", true
		}
	case lang.keepsDot():
		// The three precomposed letters, which have the accent already.
		switch r {
		case 0x00CC:
			return "i̇̀", true
		case 0x00CD:
			return "i̇́", true
		case 0x0128:
			return "i̇̃", true
		}
	case r == 0x03A3 && finalSigma(before, after):
		// "03A3; 03C2; ...; Final_Sigma" — the one condition that is not about
		// language. A lower-case sigma ending a word is ς and one inside a word
		// is σ, in every language that writes Greek.
		return "ς", true
	}
	return "", false
}

// The four conditions SpecialCasing.txt names, in its own words.

// beforeDot is Not_Before_Dot negated: "C is followed by U+0307 COMBINING DOT
// ABOVE ... with no intervening character of combining class 0 or 230 (Above)".
func beforeDot(after string) bool {
	for _, r := range after {
		if r == 0x0307 {
			return true
		}
		if ccc := shape.CombiningClass(r); ccc == 0 || ccc == 230 {
			return false
		}
	}
	return false
}

// afterI is After_I: "there is an uppercase I before C, and there is no
// intervening combining character class 230 (Above) or 0".
func afterI(before string) bool {
	for i := len(before); i > 0; {
		r, size := lastRuneIn(before[:i])
		i -= size
		if r == 'I' {
			return true
		}
		if ccc := shape.CombiningClass(r); ccc == 0 || ccc == 230 {
			return false
		}
	}
	return false
}

// afterSoftDotted is After_Soft_Dotted: "there is a Soft_Dotted character before
// C, with no intervening character of combining class 0 or 230 (Above)".
func afterSoftDotted(before string) bool {
	for i := len(before); i > 0; {
		r, size := lastRuneIn(before[:i])
		i -= size
		if unicode.Is(unicode.Properties["Soft_Dotted"], r) {
			return true
		}
		if ccc := shape.CombiningClass(r); ccc == 0 || ccc == 230 {
			return false
		}
	}
	return false
}

// moreAbove is More_Above: "C is followed by a character of combining class 230
// (Above) with no intervening character of combining class 0 or 230 (Above)".
//
// The two halves of that read oddly together and are the specification's own:
// the scan stops at the first character of class 0 *or* 230, and the answer is
// whether the one it stopped at was 230.
func moreAbove(after string) bool {
	for _, r := range after {
		switch ccc := shape.CombiningClass(r); {
		case ccc == 230:
			return true
		case ccc == 0:
			return false
		}
	}
	return false
}

// finalSigma is Final_Sigma: "C is preceded by a sequence consisting of a cased
// letter and then zero or more case-ignorable characters, and C is not followed
// by a sequence consisting of zero or more case-ignorable characters and then a
// cased letter".
//
// A sigma at the end of a word, in other words — where "the end of a word" is
// stated without reference to any word-breaking algorithm, because an
// apostrophe or a full stop after it does not make it non-final.
func finalSigma(before, after string) bool {
	preceded := false
	for i := len(before); i > 0; {
		r, size := lastRuneIn(before[:i])
		i -= size
		if caseIgnorable(r) {
			continue
		}
		preceded = cased(r)
		break
	}
	if !preceded {
		return false
	}
	for _, r := range after {
		if caseIgnorable(r) {
			continue
		}
		return !cased(r)
	}
	return true
}

// cased is Unicode's Cased property: Lowercase, Uppercase, or the titlecase
// category. Go's tables carry every part of it under another name.
func cased(r rune) bool {
	return unicode.Is(unicode.Ll, r) || unicode.Is(unicode.Lu, r) ||
		unicode.Is(unicode.Lt, r) ||
		unicode.Is(unicode.Properties["Other_Lowercase"], r) ||
		unicode.Is(unicode.Properties["Other_Uppercase"], r)
}

// caseIgnorable is Unicode's Case_Ignorable: the marks and modifiers, plus the
// handful of punctuation characters that appear *inside* words.
//
// The punctuation is Word_Break's MidLetter, MidNumLet and Single_Quote, which
// Go's tables do not carry. It is written out because it is short and because
// every character in it is one somebody's spelling depends on: an apostrophe in
// "ΟΔΟΣ'" must not make the sigma non-final.
func caseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0x00A8, 0x00AD, 0x00AF, 0x00B4, 0x00B7, 0x00B8,
		0x02D8, 0x02D9, 0x02DA, 0x02DB, 0x02DC, 0x02DD, 0x0374, 0x0387, 0x055A,
		0x055B, 0x055D, 0x055F, 0x05F4, 0x0559, 0x058A, 0x05F3, 0x0F0B, 0x2018,
		0x2019, 0x2024, 0x2027, 0x2054, 0xFE13, 0xFE52, 0xFE55, 0xFF07, 0xFF0E,
		0xFF1A, 0xFF3E, 0xFF40, 0xFF70, 0xFF9E, 0xFF9F:
		return true
	}
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Lm, r) ||
		unicode.Is(unicode.Sk, r)
}

// lastRuneIn is utf8.DecodeLastRuneInString by another name, kept here so the
// backward scans above read as scans rather than as decoding.
func lastRuneIn(s string) (rune, int) {
	for i := len(s) - 1; i >= 0 && i > len(s)-5; i-- {
		if s[i]&0xC0 != 0x80 {
			r := []rune(s[i:])
			if len(r) > 0 {
				return r[0], len(s) - i
			}
		}
	}
	return 0, 1
}
