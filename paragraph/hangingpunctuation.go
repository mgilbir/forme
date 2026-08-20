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
// The two "end" values are about a stop or a comma at the end of *any* line —
// force-end hangs it always and allow-end only where it would otherwise
// overflow — and neither is implemented. They are recognised so that a document
// using one is not read as using none, and reported so that the difference is
// not silent: what they change is where a line breaks, which shows as a word
// moved to the next line and nothing on the page says why.

// HangingPunctuation is what the property asks for.
type HangingPunctuation struct {
	// First hangs an opening bracket or quote into the margin before the first
	// formatted line.
	First bool
	// Last hangs a closing bracket or quote past the end of the last one.
	Last bool
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
	var end string
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
		case "force-end", "allow-end":
			if end != "" {
				return HangingPunctuation{}, ""
			}
			end = word
		default:
			return HangingPunctuation{}, ""
		}
	}
	return out, end
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
func HangsAtStart(r rune) bool {
	return r == '\'' || r == '"' ||
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
