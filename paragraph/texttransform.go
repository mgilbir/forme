package paragraph

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// text-transform: changing the case of the text before anything measures it.
//
// CSS Text §2.1. The property was registered and unread, so "text-transform:
// uppercase" on a heading produced lower-case text and no finding — and this one
// is worse than most of its neighbours, because it is the sort of declaration a
// house style is built on: every heading in a template is wrong at once.
//
// # Why this happens in the box tree and not at paint time
//
// It changes the text, and the text is what everything downstream measures,
// breaks, sets and *extracts*. Transforming at paint time would leave the line
// breaking measuring "internationalization" and the page showing
// "INTERNATIONALIZATION", which is wider in every face this engine has — so the
// lines would overflow by the difference, silently, because layout never saw the
// wider string.
//
// It also means the PDF carries the transformed text, so a reader copying a
// transformed heading out of the page gets it in the case it was drawn in rather
// than the case it was written in. A browser copies the source text, because a
// browser still has the DOM beside the rendering; a PDF has only what was drawn.
// Carrying the original as well would need /ActualText on a marked-content span
// around every transformed run, which is a change to how text is emitted rather
// than an addition to it.
//
// # What "word" means here
//
// "capitalize" titlecases the first letter of each word, and CSS Text defines a
// word by UAX #29's word boundaries. What is implemented is narrower and stated
// rather than discovered: a letter starts a word when the character before it is
// not a letter, a digit or an apostrophe. That gives "well-known" two capitals
// and "don't" one, which is what browsers produce for both, and it differs from
// UAX #29 for scripts that mark word boundaries some other way — the same
// scripts inline.go already refuses to break at all.
//
// The boundary is carried *across* text nodes, because a word can be: in
// "<b>e</b>xample" the "x" does not begin a word, and a version that started
// each text node afresh would set "EXample". That is what boxBuilder.afterWord
// is for.
//
// # Why the case mappings are not Go's alone
//
// strings.ToUpper applies Unicode's *simple* mappings, which are one character
// to one character by construction. A great deal of ordinary text does not have
// a one-to-one case: "straße" uppercases to "STRASSE", and a simple mapping
// cannot say so, so Go leaves the ß alone and produces "STRAßE". CSS Text
// §2.1.1 asks for the full mappings by name, and casingtable.go holds them —
// see cmd/gencasing for where they come from and which were left out.
//
// The tables are consulted first and Go's mapping is the fallback, so a
// character with no full mapping — which is all but a hundred of them — costs a
// binary search over a table of a hundred entries and nothing else. Text that
// contains no such character is not copied at all: it takes the same
// strings.ToUpper it always did.
//
// # What is still not done
//
// The conditional mappings. Three of them are language tailorings — Turkish and
// Azeri map i and I to their dotted and dotless forms, Lithuanian keeps a dot
// above a lowercased vowel — and applying one needs the element's language,
// which the box tree does not carry down to here. The fourth is Final_Sigma: a
// lowercased Σ is ς at the end of a word and σ inside one, which needs the
// characters either side rather than a table. Both are visible faults — a
// reader sees the wrong letter — rather than the silent kind this engine's
// guardrails exist for.
//
// # What is done and looks like a fault
//
// Two of the suite's tests assert the *simple* mapping for characters Unicode
// gives a full one, and both are left failing rather than special-cased.
// text-transform-upperlower-016 wants "ᾀ" to uppercase to "ᾈ" and
// text-transform-upperlower-006 wants "İ" to lowercase to "i"; Unicode says
// "ἈΙ" and "i̇", and so do the two newer tests beside them —
// text-transform-upperlower-035 spells out the same mappings and
// text-transform-lowercase-102 is exactly the "İ" case. ᾈ is the *titlecase*
// of ᾀ, which is a third mapping and is applied where a third mapping belongs.
// The suite contradicts itself here and the specification does not.
//
// Uppercasing Georgian Mkhedruli produces Mtavruli, so "ა" becomes "Ა". The
// suite's text-transform-unicase-001 asserts that it must not — "verifies that
// text-transform does not capitalize a unicase script" — and that test is older
// than its answer. Unicode 11 added the Mtavruli block in 2018 and gave every
// Mkhedruli letter an uppercase mapping into it, so the mapping applied here is
// the one Unicode states. The test is left failing rather than special-cased:
// the alternative is a table of scripts this engine declines to uppercase, which
// is a rule no specification asks for.

// TextTransform is what the property asks for, as a set rather than a choice.
//
// CSS Text 3 §2.1.1 states the grammar as
//
//	none | [ capitalize | uppercase | lowercase ] || full-width || full-size-kana
//
// which is one case change *and* either remapping, in any combination and in any
// order — "text-transform: full-width full-size-kana lowercase" is a declaration
// the suite writes. So the value is a set of bits and not one of five things.
type TextTransform uint8

const (
	TransformNone TextTransform = 0

	// The three case changes, which the grammar makes mutually exclusive.
	TransformUppercase TextTransform = 1 << iota
	TransformLowercase
	TransformCapitalize

	// The two remappings, which combine with a case change and with each other.
	TransformFullWidth
	TransformFullSizeKana

	// TransformMathAuto is CSS Text 4's math-auto, which is a keyword of its
	// own: the grammar puts it beside "none" rather than among the three that
	// combine, so it is valid alone and invalid beside anything else. See
	// mathauto.go.
	TransformMathAuto

	// transformCase is the part of a value that changes case, for the places
	// that need to ask which of the three was given without naming all three.
	transformCase = TransformUppercase | TransformLowercase | TransformCapitalize
)

// TransformOf reads the property.
//
// An unrecognised value is "none", which is what the cascade would have produced
// had the declaration been thrown out — and that goes for the whole declaration
// rather than the keyword that was not recognised, because a declaration with
// one bad keyword in it is invalid CSS and is dropped entire. Two case changes
// are refused for the same reason: the grammar allows one.
func TransformOf(value string) TextTransform {
	var out TextTransform
	for _, word := range strings.Fields(strings.ToLower(value)) {
		var bit TextTransform
		switch word {
		case "none":
			// Valid on its own, and invalid beside anything else. Both answers
			// are the same one, so there is nothing to distinguish here.
			return TransformNone
		case "uppercase":
			bit = TransformUppercase
		case "lowercase":
			bit = TransformLowercase
		case "capitalize":
			bit = TransformCapitalize
		case "full-width":
			bit = TransformFullWidth
		case "full-size-kana":
			bit = TransformFullSizeKana
		case "math-auto":
			// Alone or not at all. §2.1.1's grammar is
			// "none | math-auto | [ [capitalize|uppercase|lowercase] ||
			// full-width || full-size-kana ]", so math-auto is its own branch
			// and shares the alternation with none rather than the set.
			if len(strings.Fields(strings.ToLower(value))) != 1 {
				return TransformNone
			}
			return TransformMathAuto
		default:
			return TransformNone
		}
		if out&bit != 0 || (bit&transformCase != 0 && out&transformCase != 0) {
			return TransformNone
		}
		out |= bit
	}
	return out
}

// TransformText applies the property to one text node.
//
// inWord says whether the character before this text — which may be in another
// element — was part of a word, so that "capitalize" does not capitalise the
// middle of one. It returns the same answer for the text it produced, for the
// node after it.
//
// It allocates once at most: "none" returns the string it was given, and the
// case transforms build one buffer of the size of the input. A megabyte of text
// is a megabyte of work and not a rune of garbage per character.
func TransformText(text string, kind TextTransform, inWord bool, lang Language) (string, bool) {
	if text == "" {
		return text, inWord
	}
	// The order is the specification's and is not the order the keywords were
	// written in: case first, then width, then size. §2.1.1's own example is
	// "full-width full-size-kana lowercase", which lowercases first.
	//
	// Only the first of the two orderings is observable, and that was measured
	// rather than assumed: over every character of Unicode, seventeen tell case
	// from width apart — ß among them, because "SS" has a fullwidth form and ß
	// has none — and *none* tells full-width from full-size-kana, or either of
	// them from a case change. The code follows the specification's order all the
	// same; texttransform_test.go says which part of it a test can hold.
	if kind == TransformMathAuto {
		// On its own by construction — see TransformOf — so it is answered
		// first and nothing else runs. A text node of anything but exactly one
		// character comes back as it went in.
		return mathAuto(text), EndsInWord(text)
	}
	switch kind & transformCase {
	case TransformUppercase:
		text = localeCased(text, lang, true)
	case TransformLowercase:
		text = localeCased(text, lang, false)
	case TransformCapitalize:
		text = capitalizeWords(text, inWord, lang)
	}
	if kind&TransformFullWidth != 0 {
		text = remapped(text, fullWidthForms[:])
	}
	if kind&TransformFullSizeKana != 0 {
		text = remapped(text, fullSizeKana[:])
	}
	return text, EndsInWord(text)
}

// remapped replaces every character that one of the width tables names.
//
// Unlike a case change this is one character for one character, so the result
// is the same number of characters as the text — though not the same number of
// bytes, since "a" is one and "ａ" is three. Text that names none of them is
// returned as it arrived rather than copied, which is the ordinary case for
// full-size-kana in particular: a page setting it has kana on some of its lines
// and not on the rest.
func remapped(text string, table []widthPair) string {
	i := firstRemapped(text, table)
	if i < 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + 8)
	out.WriteString(text[:i])
	for _, r := range text[i:] {
		if to, ok := lookupWidth(r, table); ok {
			out.WriteRune(to)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// firstRemapped returns the byte offset of the first character the table names,
// or -1 if it names none of them.
//
// There is no ASCII shortcut here, and that is the difference from
// firstFullCase: nearly half of the fullwidth table is ASCII, because turning
// "6" into "６" is what the value is for.
func firstRemapped(text string, table []widthPair) int {
	for i, r := range text {
		if _, ok := lookupWidth(r, table); ok {
			return i
		}
	}
	return -1
}

// lookupWidth searches one of the generated width tables, which are sorted.
func lookupWidth(r rune, table []widthPair) (rune, bool) {
	i, j := 0, len(table)
	for i < j {
		h := int(uint(i+j) >> 1)
		if table[h].from < r {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(table) && table[i].from == r {
		return table[i].to, true
	}
	return 0, false
}

// localeCased is the case change with the conditional mappings applied.
//
// The conditions are a handful of characters in a handful of languages plus the
// Greek final sigma — see localecasing.go — so the whole of this is skipped
// unless the text really contains one of the characters they are about. Every
// other run takes the same path it always did.
func localeCased(text string, lang Language, upper bool) string {
	if upper && lang == "el" {
		// Greek drops its accents in capitals, which is a whole-run rule rather
		// than a per-character mapping: an accent removed from one vowel puts a
		// dialytika on the next. See greekcasing.go.
		if got := greekUppercase(text); got != "" {
			return got
		}
	}
	if i := firstConditional(text, lang, upper); i >= 0 {
		return conditionalCased(text, lang, upper, i)
	}
	if upper {
		return fullCased(text, fullUppercase[:], unicode.ToUpper, strings.ToUpper)
	}
	return fullCased(text, fullLowercase[:], unicode.ToLower, strings.ToLower)
}

// firstConditional is the byte offset of the first character a conditional
// mapping could be about, or -1.
//
// It asks the mappings themselves rather than carrying a second list of
// characters that would have to be kept in step with them: for the first
// character that answers, the answer is the whole test.
func firstConditional(text string, lang Language, upper bool) int {
	for i, r := range text {
		if upper {
			if _, ok := localeUpper(r, lang); ok {
				return i
			}
			continue
		}
		// The backward-looking conditions cannot fire on a character the
		// forward-looking test would miss, because every one of them names the
		// character itself: it is I, or the dotted capital, or a combining dot,
		// or a sigma. So the cheap test is whether this character is one of
		// those at all, which is what asking with empty context does — except
		// for the two whose condition is *absence*, which answer true there.
		if r == 0x0130 || r == 'I' || r == 0x0307 || r == 0x03A3 ||
			r == 'J' || r == 0x012E || r == 0x00CC || r == 0x00CD || r == 0x0128 {
			return i
		}
	}
	return -1
}

// conditionalCased maps the text a character at a time from the first character
// a condition could be about, which is where the cheap whole-string path stops
// being available.
func conditionalCased(text string, lang Language, upper bool, from int) string {
	var out strings.Builder
	out.Grow(len(text) + 8)
	if upper {
		out.WriteString(fullCased(text[:from], fullUppercase[:], unicode.ToUpper, strings.ToUpper))
	} else {
		out.WriteString(fullCased(text[:from], fullLowercase[:], unicode.ToLower, strings.ToLower))
	}
	for i, r := range text[from:] {
		at := from + i
		var (
			s  string
			ok bool
		)
		if upper {
			s, ok = localeUpper(r, lang)
		} else {
			s, ok = localeLower(r, text[:at], text[at+len(string(r)):], lang)
		}
		if ok {
			out.WriteString(s)
			continue
		}
		table, simple := fullUppercase[:], unicode.ToUpper
		if !upper {
			table, simple = fullLowercase[:], unicode.ToLower
		}
		if s, ok := lookupFullCase(r, table); ok {
			out.WriteString(s)
		} else {
			out.WriteRune(simple(r))
		}
	}
	return out.String()
}

// fullCased maps every character of a string, preferring the full mapping.
//
// The whole-string function is the fast path and does the work whenever no
// character of the text has a full mapping — which is the ordinary case, and
// keeps an ASCII heading on the byte-wise loop inside strings.ToUpper rather
// than on a rune-by-rune one here. Only text that really does contain one of
// the hundred characters in the table is rebuilt.
func fullCased(text string, table []fullCase, simple func(rune) rune, whole func(string) string) string {
	i := firstFullCase(text, table)
	if i < 0 {
		return whole(text)
	}
	var out strings.Builder
	// The mappings are longer than what they replace, so this is a floor rather
	// than a guess; it saves the first growth and not the rest.
	out.Grow(len(text) + 8)
	out.WriteString(whole(text[:i]))
	for _, r := range text[i:] {
		if s, ok := lookupFullCase(r, table); ok {
			out.WriteString(s)
		} else {
			out.WriteRune(simple(r))
		}
	}
	return out.String()
}

// firstFullCase returns the byte offset of the first character of the text that
// has a full mapping, or -1 if none has.
//
// Every character in the tables is above U+007F, so ASCII — which is most text
// this will ever see — is rejected a byte at a time without decoding.
func firstFullCase(text string, table []fullCase) int {
	for i, r := range text {
		if r < utf8.RuneSelf {
			continue
		}
		if _, ok := lookupFullCase(r, table); ok {
			return i
		}
	}
	return -1
}

// lookupFullCase searches one of the generated tables, which are sorted.
func lookupFullCase(r rune, table []fullCase) (string, bool) {
	i, j := 0, len(table)
	for i < j {
		h := int(uint(i+j) >> 1)
		if table[h].r < r {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(table) && table[i].r == r {
		return table[i].s, true
	}
	return "", false
}

// capitalizeWords titlecases the first letter of every word.
//
// Titlecase rather than uppercase, which matters for exactly the digraphs it was
// invented for: U+01F3 "ǳ" titlecases to "ǲ" and uppercases to "Ǳ", and a name
// set in the second form is set wrongly. It is also a third mapping rather than
// a variation on the other two — "ß" titlecases to "Ss" and uppercases to "SS" —
// so it has a table of its own.
func capitalizeWords(text string, inWord bool, lang Language) string {
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		if !inWord && lang == "nl" {
			// IJ is one letter of the Dutch alphabet written as two, so a word
			// beginning with it takes two capitals. See dutchCapitalize.
			if got, ok := dutchCapitalize(text, i); ok {
				out.WriteString(got)
				i += 2
				inWord = true
				continue
			}
		}
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		i += size

		if !inWord && unicode.IsLetter(r) {
			if s, ok := lookupFullCase(r, fullTitlecase[:]); ok {
				out.WriteString(s)
			} else {
				out.WriteRune(unicode.ToTitle(r))
			}
		} else {
			out.WriteRune(r)
		}
		inWord = isWordRune(r)
	}
	return out.String()
}

// isWordRune reports whether a character continues a word rather than ending it.
//
// The apostrophes are here for the reason the file comment gives: without them
// "don't" comes out "Don'T", which is a real word set wrongly rather than a
// theoretical one. Both spellings are included because a document may carry
// either.
func isWordRune(r rune) bool {
	switch r {
	case '\'', '’':
		return true
	}
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

// EndsInWord reports whether the last character of a string continues a word,
// which is what the next text node needs to know.
func EndsInWord(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text)
	return isWordRune(r)
}
