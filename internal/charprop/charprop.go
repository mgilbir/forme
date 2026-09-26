// Package charprop answers the Unicode property questions the engine asks
// about a character, from the release the rest of its tables are generated
// from.
//
// They were Go's package unicode, which answers from the release the toolchain
// shipped — Unicode 15.0.0 for Go 1.26 — while every table beside it said
// 17.0.0. A character Unicode 16 or 17 assigned was a letter to one question
// and not a letter to the next: U+0897 ARABIC PEPET was a mark to the shaper's
// tables and nothing to the cluster walk beside them, and the Garay digits were
// digits to no one. Which answer a document got depended on the toolchain that
// built the engine. See cmd/gencharprop, which generates tables.go, and
// cmd/pinnedunicode_test.go, which keeps package unicode out of every property
// question in the tree.
//
// The case mappings are not here. They are paragraph's, from cmd/gencasing,
// and paragraph exports the two simple ones layout asks.
//
// The package is internal because it is a statement of Unicode, not an API of
// the engine's: what a caller does with the answers is the caller's.
package charprop

// Set is a set of General_Category values. Every character has exactly one, so
// Of returns a set of one, and Is asks whether it is in a set: Is(r, Mn|Me) is
// "a non-spacing or an enclosing mark".
type Set uint32

// The General_Category values, one bit each, named as UnicodeData.txt names
// them.
const (
	Lu Set = 1 << iota // uppercase letter
	Ll                 // lowercase letter
	Lt                 // titlecase letter
	Lm                 // modifier letter
	Lo                 // other letter
	Mn                 // non-spacing mark
	Mc                 // spacing mark
	Me                 // enclosing mark
	Nd                 // decimal digit
	Nl                 // letter number
	No                 // other number
	Pc                 // connector punctuation
	Pd                 // dash punctuation
	Ps                 // open punctuation
	Pe                 // close punctuation
	Pi                 // initial quotation mark
	Pf                 // final quotation mark
	Po                 // other punctuation
	Sm                 // math symbol
	Sc                 // currency symbol
	Sk                 // modifier symbol
	So                 // other symbol
	Zs                 // space separator
	Zl                 // line separator
	Zp                 // paragraph separator
	Cc                 // control
	Cf                 // format
	Cs                 // surrogate
	Co                 // private use
	Cn                 // unassigned
)

// The major classes, as UAX #44 groups the values: L is every letter, and so
// on. They are what package unicode's IsLetter, IsMark, IsNumber, IsPunct and
// IsSymbol asked.
const (
	L = Lu | Ll | Lt | Lm | Lo
	M = Mn | Mc | Me
	N = Nd | Nl | No
	P = Pc | Pd | Ps | Pe | Pi | Pf | Po
	S = Sm | Sc | Sk | So
	Z = Zs | Zl | Zp
	C = Cc | Cf | Cs | Co | Cn
)

// latin1 is the category of each of the first 256 code points, read once from
// the table, so that the characters most text is made of are answered without
// a search.
var latin1 = func() (t [0x100]Set) {
	for i := range t {
		t[i] = Cn
	}
	for _, s := range categoryRanges {
		for r := s.lo; r <= s.hi && r < 0x100; r++ {
			t[r] = s.cat
		}
	}
	return t
}()

// Of is a character's General_Category, as a set of one. A value that is not a
// code point — negative, past U+10FFFF — is unassigned.
func Of(r rune) Set {
	if uint32(r) < 0x100 {
		return latin1[r]
	}
	lo, hi := 0, len(categoryRanges)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		switch s := &categoryRanges[mid]; {
		case r < s.lo:
			hi = mid - 1
		case r > s.hi:
			lo = mid + 1
		default:
			return s.cat
		}
	}
	return Cn
}

// Is reports whether a character's General_Category is in the set.
func Is(r rune, set Set) bool { return Of(r)&set != 0 }

// WhiteSpace is Unicode's White_Space property, which is exactly what package
// unicode's IsSpace asked: the ASCII spaces and controls from tab to carriage
// return, NEL, the no-break spaces, and the space, line and paragraph
// separators. It is not CSS's white space, which is four characters; a caller
// asking that question wants the four.
func WhiteSpace(r rune) bool { return inRanges(r, whiteSpaceRanges[:]) }

// SoftDotted is Unicode's Soft_Dotted property: i, j and the letters like them,
// whose dot is dropped when an accent is written above.
func SoftDotted(r rune) bool { return inRanges(r, softDottedRanges[:]) }

// Cased is Unicode's Cased property (UAX #44; Unicode §3.13, D135).
func Cased(r rune) bool { return inRanges(r, casedRanges[:]) }

// CaseIgnorable is Unicode's Case_Ignorable property (Unicode §3.13, D136).
func CaseIgnorable(r rune) bool { return inRanges(r, caseIgnorableRanges[:]) }

// inRanges reports whether r is in one of a sorted list of ranges.
func inRanges(r rune, ranges []struct{ lo, hi rune }) bool {
	lo, hi := 0, len(ranges)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		switch {
		case r < ranges[mid].lo:
			hi = mid - 1
		case r > ranges[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}
