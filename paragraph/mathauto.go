package paragraph

import "unicode/utf8"

// text-transform: math-auto, CSS Text 4 §2.1.1 and MathML Core §3.2.2.
//
// Mathematics sets a single-letter variable in italic and everything else
// upright: "x" is a variable and "sin" is a function name, and the difference is
// what tells a reader which is which. The rule is a *typographic convention with
// a Unicode encoding*: the italic letters are characters of their own, in the
// Mathematical Alphanumeric Symbols block, so the transform is a character
// mapping rather than a font choice — which is why it is text-transform's
// business and not font-style's.
//
// # Only where the text is one character
//
// "This value only has an effect on a text node with a single character",
// which is the whole of what makes it automatic: "x" becomes 𝑥 and "sin" is
// left alone, without the author marking either. The suite's
// text-transform-math-auto-002 is that in three rows — one character, two, and
// three — and the last two must come out exactly as they went in.
//
// A single character means a single code point. A cluster of two — a letter and
// a combining mark — is two characters and is left alone, which is the same
// answer MathML Core gives and is what keeps the rule from having to decide what
// the italic form of a marked letter would be.

// mathItalic is the italic mapping, and it is one contiguous stretch of Unicode
// read backwards.
//
// U+1D434..U+1D467 is Mathematical Italic Latin, in the order A–Z then a–z, and
// U+1D6E2..U+1D71B is Mathematical Italic Greek in Unicode's own order for that
// block — which is not the order of the Greek alphabet: the capitals run Α to Ρ,
// then the *capital theta symbol* ϴ stands where Σ would be, then Σ to Ω, then
// the nabla, then the small letters α to ω, then the partial differential, then
// the six variant letterforms.
//
// So the table is the ranges those two stretches cover, written against the
// characters they came from, plus the three characters that are not in a range:
// the two dotless letters, which sit at the end of the Latin stretch rather than
// with their dotted forms, and "h", whose italic form is not in the block at all
// — U+1D455 is a hole, because U+210E PLANCK CONSTANT was already encoded and
// Unicode does not encode a character twice.
var mathItalic = [...]struct{ lo, hi, to rune }{
	{'A', 'Z', 0x1D434},       // Latin capitals
	{'a', 'z', 0x1D44E},       // Latin smalls, with "h" carved out below
	{0x0131, 0x0131, 0x1D6A4}, // dotless i
	{0x0237, 0x0237, 0x1D6A5}, // dotless j
	{0x0391, 0x03A1, 0x1D6E2}, // Greek capitals Α..Ρ
	{0x03F4, 0x03F4, 0x1D6F3}, // capital theta symbol, in Σ's place
	{0x03A3, 0x03A9, 0x1D6F4}, // Greek capitals Σ..Ω
	{0x2207, 0x2207, 0x1D6FB}, // nabla
	{0x03B1, 0x03C9, 0x1D6FC}, // Greek smalls α..ω
	{0x2202, 0x2202, 0x1D715}, // partial differential
	{0x03F5, 0x03F5, 0x1D716}, // lunate epsilon
	{0x03D1, 0x03D1, 0x1D717}, // theta symbol
	{0x03F0, 0x03F0, 0x1D718}, // kappa symbol
	{0x03D5, 0x03D5, 0x1D719}, // phi symbol
	{0x03F1, 0x03F1, 0x1D71A}, // rho symbol
	{0x03D6, 0x03D6, 0x1D71B}, // pi symbol
}

// mathItalicOf is the italic form of one character, or the character itself.
func mathItalicOf(r rune) rune {
	if r == 'h' {
		// U+1D455 is a hole in the block: U+210E PLANCK CONSTANT is the italic
		// h and was encoded first, so Unicode left the slot empty rather than
		// encode the same character twice.
		return 0x210E
	}
	for _, e := range mathItalic {
		if r >= e.lo && r <= e.hi {
			return e.to + (r - e.lo)
		}
	}
	return r
}

// mathAuto applies the transform to one text node.
//
// A node of anything but exactly one character is returned as it arrived, and so
// is one whose character has no italic form — a digit, a bracket, a letter of a
// script the block does not cover.
func mathAuto(text string) string {
	r, size := utf8.DecodeRuneInString(text)
	if size != len(text) || r == utf8.RuneError {
		return text
	}
	got := mathItalicOf(r)
	if got == r {
		return text
	}
	return string(got)
}
