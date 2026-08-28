package paragraph

import "strings"

// The writing system a piece of text is set in, which is not the same question
// as what language it is in.
//
// CSS Text asks it once, in the second sentence of §4.1.1's segment break rule:
// "if the writing system of the segment break is Chinese, Japanese, or Yi". The
// rule is about how those three are *typeset* — without spaces between words,
// so a newline in the source is not a word boundary and must not become a space
// — and that is a property of the script rather than of the language.
//
// The suite says so by name. writing-system-segment-break-001 writes
// lang="ain-Kana": Ainu, which is not Japanese, written in katakana, which is.
// Its comment is "the writing system is Katakana, which is classified as
// Japanese, despite a non Japanese content language (Ainu)". So the script
// subtag decides where there is one, and the language decides where there is
// not.
//
// The other direction matters as much and is why this cannot simply read the
// language: "ja-Latn" is Japanese romanised, written with spaces, and a newline
// in it is a word boundary like any other.

// WritingSystem is one of the three CSS Text names, or none of them.
//
// None of them is the zero value, so text that says nothing about itself gets
// the answer every conforming engine gives it: the rule does not apply.
type WritingSystem uint8

const (
	// WritingSystemOther is every writing system the rule is not about.
	WritingSystemOther WritingSystem = iota
	WritingSystemChinese
	WritingSystemJapanese
	WritingSystemYi
)

// SpacesNoWords reports whether the writing system is one of the three CSS
// Text's segment break rule names.
//
// The name is what the three have in common and why they are named together:
// they are written without spaces between words, so a line break in the source
// falls in the middle of a sentence rather than between two words.
func (w WritingSystem) SpacesNoWords() bool { return w != WritingSystemOther }

// WritingSystemOf reads a language tag as the writing system its text is in.
//
// The script subtag wins where there is one, because that is what a script
// subtag is for: "ain-Kana" is Ainu written in katakana and is typeset as
// Japanese, "ja-Latn" is Japanese romanised and is not. Where there is none the
// primary language answers, which is the ordinary case — "ja" is Japanese.
//
// Korean is not one of the three, and neither is a tag that names the Hangul
// script. That is the same carve-out the first sentence of the rule makes by
// name: Korean is written with spaces between its words.
func WritingSystemOf(tag string) WritingSystem {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return WritingSystemOther
	}
	primary, rest, _ := strings.Cut(tag, "-")
	for rest != "" {
		var sub string
		sub, rest, _ = strings.Cut(rest, "-")
		// A script subtag is four letters, which is what tells it from a region
		// (two letters or three digits) and from a variant. The first one wins:
		// a tag has at most one, and anything after it is a region or a variant.
		if len(sub) == 4 && isAlpha(sub) {
			return writingSystemOfScript(sub)
		}
	}
	switch primary {
	case "zh":
		return WritingSystemChinese
	case "ja":
		return WritingSystemJapanese
	case "ii":
		return WritingSystemYi
	}
	return WritingSystemOther
}

// writingSystemOfScript maps an ISO 15924 script subtag.
//
// Only the scripts the three writing systems are written in. Han is Chinese
// here and not Japanese, which is the answer a tag with no other information
// deserves: "Hani" alone says the characters and not which tradition sets them,
// and the rule treats all three the same anyway — what it asks is whether the
// writing system is one of the three, and every entry below says yes.
func writingSystemOfScript(script string) WritingSystem {
	switch script {
	case "hani", "hans", "hant":
		return WritingSystemChinese
	case "jpan", "hira", "kana":
		return WritingSystemJapanese
	case "yiii":
		return WritingSystemYi
	}
	return WritingSystemOther
}
