package paragraph

import (
	"strings"
	"unicode"
)

// Uppercasing Greek, which drops the accents.
//
// Greek is written with an accent on the stressed syllable of every polysyllabic
// word, and it is not written in capitals: ΚΑΛΗΜΕΡΑ, not ΚΑΛΗΜΈΡΑ. A page whose
// headings are set with text-transform: uppercase and keep their accents is
// wrong in a way every reader of Greek sees at once, and the suite has four
// tests of it.
//
// The rule is not in SpecialCasing.txt — the mappings there are the same in
// every language — but in CLDR, as a tailoring for the Greek language. It is
// three rules and not one:
//
//	καλημέρα αύριο   ->  ΚΑΛΗΜΕΡΑ ΑΥΡΙΟ    the accent goes
//	θεϊκό            ->  ΘΕΪΚΟ             the dialytika stays
//	ευφυΐα Νεράιδα   ->  ΕΥΦΥΪΑ ΝΕΡΑΪΔΑ    and one is *added*
//	ήσουν ή εγώ      ->  ΗΣΟΥΝ Ή ΕΓΩ       except on the word for "or"
//
// # The added dialytika
//
// The third line is the one that is not obvious. "Νεράιδα" is spelled with an
// accent on the alpha, and that accent is doing two things: marking the stress,
// and saying that the alpha and the iota after it are two syllables rather than
// the diphthong "αι". Drop it and the word reads differently. So where an accent
// is removed from a vowel that an iota or an upsilon follows, a dialytika goes on
// that iota or upsilon to keep the two apart.
//
// # The disjunctive eta
//
// "ή" on its own is the word for "or", and it keeps its accent in capitals —
// otherwise it would be "Η", the definite article. CLDR's rule is exactly that:
// the single-letter word, and no other.
//
// # What is not done
//
// Polytonic Greek. Its breathings and its circumflex are a second set of marks
// with their own removal rules, and the suite tests none of them; monotonic is
// what Greek has been written in since 1982 and is what these four tests are.
// A polytonic accent is left alone rather than half-handled.

// greekUppercase is the tailoring, applied to a whole run.
func greekUppercase(text string) string {
	if !hasGreek(text) {
		return ""
	}
	var out strings.Builder
	out.Grow(len(text) + 8)
	runes := []rune(text)
	// lostAccent says the character just written had an accent taken off it, so
	// an iota or upsilon after it needs a dialytika to stay a syllable of its
	// own.
	lostAccent := false
	for i, r := range runes {
		upper, dropped, ok := greekUpperOf(r)
		if !ok {
			// A character with no tailoring of its own, which is every
			// unaccented letter — and is where the added dialytika lands, since
			// a plain iota is exactly what needs one.
			upper = unicode.ToUpper(r)
			dropped = false
		}
		if ok && dropped && isDisjunctiveEta(runes, i) {
			// The word for "or" keeps its accent, so nothing was dropped.
			out.WriteRune(0x0389)
			lostAccent = false
			continue
		}
		if lostAccent && needsDialytika(r) {
			// The vowel before this one lost the accent that kept the two
			// apart, so this one says so instead.
			upper = withDialytika(upper)
		}
		out.WriteRune(upper)
		lostAccent = dropped
	}
	return out.String()
}

// hasGreek reports whether a run holds a character this tailoring is about,
// which is what keeps it off every other run of every other document.
func hasGreek(text string) bool {
	for _, r := range text {
		if r >= 0x0370 && r <= 0x03FF {
			return true
		}
	}
	return false
}

// greekUpperOf is one character's uppercase in Greek, and whether an accent was
// taken off it.
//
// The third result is false for a character this tailoring has nothing to say
// about, which is every character outside Greek and the unaccented Greek letters
// — those take the ordinary mapping.
func greekUpperOf(r rune) (upper rune, dropped, ok bool) {
	switch r {
	// The seven accented vowels of monotonic Greek, lower case.
	case 0x03AC:
		return 0x0391, true, true // ά -> Α
	case 0x03AD:
		return 0x0395, true, true // έ -> Ε
	case 0x03AE:
		return 0x0397, true, true // ή -> Η
	case 0x03AF:
		return 0x0399, true, true // ί -> Ι
	case 0x03CC:
		return 0x039F, true, true // ό -> Ο
	case 0x03CD:
		return 0x03A5, true, true // ύ -> Υ
	case 0x03CE:
		return 0x03A9, true, true // ώ -> Ω

	// The same seven already in capitals, which uppercasing must also strip:
	// a heading written in capitals with accents is the fault this is about.
	case 0x0386:
		return 0x0391, true, true
	case 0x0388:
		return 0x0395, true, true
	case 0x0389:
		return 0x0397, true, true
	case 0x038A:
		return 0x0399, true, true
	case 0x038C:
		return 0x039F, true, true
	case 0x038E:
		return 0x03A5, true, true
	case 0x038F:
		return 0x03A9, true, true

	// The dialytika alone, which is not an accent and stays.
	case 0x03CA:
		return 0x03AA, false, true // ϊ -> Ϊ
	case 0x03CB:
		return 0x03AB, false, true // ϋ -> Ϋ

	// The dialytika and an accent together: the accent goes and the dialytika
	// stays, so nothing is "dropped" in the sense the next character cares
	// about — this vowel already says it is a syllable of its own.
	case 0x0390:
		return 0x03AA, false, true // ΐ -> Ϊ
	case 0x03B0:
		return 0x03AB, false, true // ΰ -> Ϋ
	}
	return 0, false, false
}

// needsDialytika reports whether a character is one that would read as the
// second half of a diphthong: an iota or an upsilon, in either case.
func needsDialytika(r rune) bool {
	switch r {
	case 0x03B9, 0x03C5, 0x0399, 0x03A5: // ι υ Ι Υ
		return true
	}
	return false
}

// withDialytika is the capital iota or upsilon with one.
func withDialytika(upper rune) rune {
	switch upper {
	case 0x0399:
		return 0x03AA // Ϊ
	case 0x03A5:
		return 0x03AB // Ϋ
	}
	return upper
}

// isDisjunctiveEta reports whether the eta at runes[i] is the whole of a word,
// which is the Greek for "or" and is the one accent capitals keep.
//
// A word here is a run of letters, which is enough: the character either side of
// a one-letter word is a space or a mark of punctuation, and neither is a letter.
func isDisjunctiveEta(runes []rune, i int) bool {
	if runes[i] != 0x03AE && runes[i] != 0x0389 {
		return false
	}
	if i > 0 && unicode.IsLetter(runes[i-1]) {
		return false
	}
	if i+1 < len(runes) && unicode.IsLetter(runes[i+1]) {
		return false
	}
	return true
}

// dutchCapitalize is the other tailoring CLDR carries and the UCD does not: a
// Dutch word beginning "ij" is capitalised on both letters.
//
// IJ is a single letter of the Dutch alphabet written as two, and "Ijsland" is
// as wrong as "IJsland" would be in English. It applies to titlecasing only —
// uppercasing gives IJSLAND either way, and lowercasing has nothing to do.
//
// The second result says whether it applied, so that the caller can leave the
// ordinary titlecasing to do its work where it did not.
func dutchCapitalize(text string, at int) (string, bool) {
	rest := text[at:]
	if len(rest) < 2 {
		return "", false
	}
	if (rest[0] != 'i' && rest[0] != 'I') || (rest[1] != 'j' && rest[1] != 'J') {
		return "", false
	}
	return "IJ", true
}
