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
// # What is not done
//
// The case mappings are Go's, which are Unicode's simple one-to-one mappings.
// The full mappings of SpecialCasing.txt are not applied, so "straße" uppercases
// to "STRAßE" rather than "STRASSE", and a final sigma does not become ς when a
// word is lowercased. Both are wrong and both are wrong *visibly* — a reader
// sees the letter that was not mapped — rather than in the silent way this
// engine's guardrails exist for, and doing them properly means a case-mapping
// table this module does not otherwise need. One of the suite's tests is exactly
// this: text-transform-uppercase-002 is a sharp s and nothing else.
//
// # What is done and looks like a fault
//
// Uppercasing Georgian Mkhedruli produces Mtavruli, so "ა" becomes "Ა". The
// suite's text-transform-unicase-001 asserts that it must not — "verifies that
// text-transform does not capitalize a unicase script" — and that test is older
// than its answer. Unicode 11 added the Mtavruli block in 2018 and gave every
// Mkhedruli letter an uppercase mapping into it, so the mapping applied here is
// the one Unicode states. The test is left failing rather than special-cased:
// the alternative is a table of scripts this engine declines to uppercase, which
// is a rule no specification asks for.

// TextTransform is what the property asks for.
type TextTransform uint8

const (
	TransformNone TextTransform = iota
	TransformUppercase
	TransformLowercase
	TransformCapitalize
)

// TransformOf reads the property.
//
// An unrecognised value is "none", which is what the cascade would have produced
// had the declaration been thrown out. "full-width" and "full-size-kana" are
// among the unrecognised ones: both remap characters rather than changing case,
// and treating either as a case change would silently do something else.
func TransformOf(value string) TextTransform {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "uppercase":
		return TransformUppercase
	case "lowercase":
		return TransformLowercase
	case "capitalize":
		return TransformCapitalize
	}
	return TransformNone
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
func TransformText(text string, kind TextTransform, inWord bool) (string, bool) {
	if text == "" {
		return text, inWord
	}
	switch kind {
	case TransformUppercase:
		text = strings.ToUpper(text)
	case TransformLowercase:
		text = strings.ToLower(text)
	case TransformCapitalize:
		text = capitalizeWords(text, inWord)
	}
	return text, EndsInWord(text)
}

// capitalizeWords titlecases the first letter of every word.
//
// Titlecase rather than uppercase, which matters for exactly the digraphs it was
// invented for: U+01F3 "ǳ" titlecases to "ǲ" and uppercases to "Ǳ", and a name
// set in the second form is set wrongly.
func capitalizeWords(text string, inWord bool) string {
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		i += size

		if !inWord && unicode.IsLetter(r) {
			out.WriteRune(unicode.ToTitle(r))
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
