package paragraph

import (
	"strings"
	"unicode"
)

// hanging-punctuation, CSS Text §8.4: letting a bracket or a quote sit outside
// the line rather than inside it.
//
// A paragraph that begins with an opening quotation mark has a ragged left edge
// without it — the quote is narrow and light, and the letters after it start a
// character further in than the letters of every line below. Typographers have
// hung the mark in the margin for as long as there has been type, and this is
// the property that asks for it.
//
//	none | [ first || [ force-end | allow-end ] || last ]
//
// first  an opening bracket or quote at the start of the first formatted line
//        hangs into the margin before it
// last   a closing bracket or quote at the end of the last formatted line hangs
//        past the end of it
//
// The two "end" values are about a stop or a comma at the end of *any* line, and
// they hang a different set of characters from the other two: not a bracket or a
// quote but a full stop or a comma, which is why HangsAsStopOrComma is a list of
// its own rather than a category.
//
// allow-end hangs one "only if it does not otherwise fit prior to justification"
// — which is to say, only where hanging it is what lets the line hold it.
// force-end hangs it always, and is not implemented: it is recognised so that a
// document using it is not read as using none, and reported so that the
// difference is not silent.

// HangingPunctuation is what the property asks for.
type HangingPunctuation struct {
	// First hangs an opening bracket or quote into the margin before the first
	// formatted line.
	First bool
	// Last hangs a closing bracket or quote past the end of the last one.
	Last bool
	// EndAllow hangs a stop or a comma past the end of *any* line, but only
	// where the line would not otherwise hold it. It is the one of §8.4's two
	// end values this engine does: the other hangs one always, which is a
	// decision about every line rather than about the line that overflowed.
	EndAllow bool
}

// HangingPunctuationOf reads the property, and names the value it could not
// honour.
//
// The grammar is a set, like text-transform's: "first last" is legal and so is
// "last first". A keyword repeated, or both of the two end values together, is
// invalid — and an invalid declaration is dropped whole, which is what the
// cascade does with one.
func HangingPunctuationOf(value string) (HangingPunctuation, string) {
	var out HangingPunctuation
	var end, unhandled string
	seenFirst, seenLast := false, false
	for _, word := range strings.Fields(strings.ToLower(value)) {
		switch word {
		case "none":
			// Valid alone and invalid beside anything else, and both answers are
			// the same one.
			return HangingPunctuation{}, ""
		case "first":
			if seenFirst {
				return HangingPunctuation{}, ""
			}
			seenFirst, out.First = true, true
		case "last":
			if seenLast {
				return HangingPunctuation{}, ""
			}
			seenLast, out.Last = true, true
		case "allow-end":
			if end != "" {
				return HangingPunctuation{}, ""
			}
			end, out.EndAllow = word, true
		case "force-end":
			if end != "" {
				return HangingPunctuation{}, ""
			}
			end, unhandled = word, word
		default:
			return HangingPunctuation{}, ""
		}
	}
	return out, unhandled
}

// HangsAtStart reports whether a character is one §8.4 hangs into the margin
// before a line: "an opening bracket or quote".
//
// Unicode's own categories say which those are — Ps for the brackets, Pi and Pf
// for the directional quotation marks — and the two ASCII quotes are named
// separately because they belong to no category of their own: an apostrophe is
// Po and so is a great deal else.
//
// Pf is in the *opening* set as well as the closing one, and that is the
// specification's own reading rather than an oversight here: a language that
// quotes with guillemets pointing outward opens with U+00BB, and a set built
// from Pi alone would leave every German quotation unhung.
//
// U+3000 IDEOGRAPHIC SPACE is in the set and is not punctuation, which is the
// one entry that has to be argued for rather than read off a category. It is
// what a Japanese paragraph is indented with — the language has no first-line
// indent of its own, so an author writes a full-width space and the indent is
// one character — and §8.4's whole subject is the ragged edge that a mark at the
// start of a line leaves. hanging-punctuation-first-002 is that fixture, and it
// puts an arrow after the space and asks for it to line up with an arrow that
// has no space in front of it at all.
func HangsAtStart(r rune) bool {
	return r == '\'' || r == '"' || r == 0x3000 ||
		unicode.Is(unicode.Ps, r) || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r)
}

// HangsAtEnd reports whether a character is one §8.4 hangs past the end of a
// line: "a closing bracket or quote".
func HangsAtEnd(r rune) bool {
	return r == '\'' || r == '"' ||
		unicode.Is(unicode.Pe, r) || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r)
}

// LeadingHang is how many bytes at the start of a run §8.4 would hang into the
// margin, which is one character or none.
//
// One character: the specification hangs "an opening bracket or quote" and not a
// run of them, so "((word" hangs one bracket and sets the other inside the line.
func LeadingHang(text string) int {
	for i, r := range text {
		if i > 0 {
			return 0
		}
		if HangsAtStart(r) {
			return len(string(r))
		}
		return 0
	}
	return 0
}

// TrailingHang is how many bytes at the end of a run would hang past it.
func TrailingHang(text string) int {
	last, size := rune(0), 0
	for i, r := range text {
		last, size = r, len(text)-i
	}
	if size > 0 && HangsAtEnd(last) {
		return size
	}
	return 0
}

// HangsAsStopOrComma reports whether a character is one of §8.4's "stops or
// commas", which is what its two end values hang.
//
// It is a list rather than a Unicode category, and the specification gives it as
// one: a full stop is Po and so are a hundred characters that are not stops.
// These are the thirteen §8.4 names, and the set is closed — a document using
// some other mark gets no hang, which is the answer a browser gives.
func HangsAsStopOrComma(r rune) bool {
	switch r {
	case ',', '.', // U+002C COMMA, U+002E FULL STOP
		0x060C, // ARABIC COMMA
		0x06D4, // ARABIC FULL STOP
		0x3001, // IDEOGRAPHIC COMMA
		0x3002, // IDEOGRAPHIC FULL STOP
		0xFF0C, // FULLWIDTH COMMA
		0xFF0E, // FULLWIDTH FULL STOP
		0xFE50, // SMALL COMMA
		0xFE51, // SMALL IDEOGRAPHIC COMMA
		0xFE52, // SMALL FULL STOP
		0xFF61, // HALFWIDTH IDEOGRAPHIC FULL STOP
		0xFF64: // HALFWIDTH IDEOGRAPHIC COMMA
		return true
	}
	return false
}

// TrailingStopOrComma is how many bytes at the end of a run §8.4's end values
// would hang, which is one character or none.
//
// One character, for the reason LeadingHang gives: the specification hangs "a
// stop or comma" and not a run of them. It is also what makes the rule decidable
// — a line that would still not fit with one character outside it is a line the
// value does not rescue, and the suite says so in as many words: "ab c、、" in
// four characters of room does not hang, because the overflow happened at the
// first comma and hanging the second is not what would fix it.
func TrailingStopOrComma(text string) int {
	last, size := rune(0), 0
	for i, r := range text {
		last, size = r, len(text)-i
	}
	if size > 0 && HangsAsStopOrComma(last) {
		return size
	}
	return 0
}
