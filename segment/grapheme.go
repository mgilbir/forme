// Package segment finds text boundaries: at present grapheme cluster
// boundaries, which is UAX #29's definition of where one user-perceived
// character ends and the next begins.
//
// It is named for the general question rather than the one boundary it answers
// today, because the others UAX #29 defines — word and sentence boundaries —
// belong beside it, and so does UAX #14's line breaking when it arrives.
//
// It exists because CSS needs to know where text may be cut. CSS Text §2 defines
// a soft wrap opportunity as falling between "typographic character units", and
// says a typographic character unit is the grapheme cluster — so "break-all",
// which lets a line end between two characters rather than only between two
// words, is a question about grapheme clusters and about nothing else. Cutting
// anywhere else corrupts the text it was asked to fit: it separates a letter
// from the accent that belongs to it, splits a Hangul syllable into the jamo it
// is spelled with, halves a flag, or breaks a Devanagari conjunct in the middle.
//
// # Why not the shaper's clusters
//
// The obvious candidate is the cluster index a shaper returns, and it is not
// this. It was measured rather than assumed: forme's clusters are *finer* than
// grapheme clusters — a base with two combining marks breaks between the marks,
// a keycap breaks off its digit, conjoining Hangul breaks into three, a flag
// breaks into two letters — and in a right-to-left run they are not even in
// text order, because the glyphs come back in the order they are drawn. A
// position in the glyph stream is not a position in the string.
//
// So this works on the characters, which is also where it is needed: the line
// breaker splits a string and never sees a glyph.
//
// # What is implemented
//
// All of UAX #29's grapheme rules, GB1 through GB999, including the two that an
// implementation usually skips and that are exactly the ones that corrupt Indic
// and emoji text when they are missing: GB9c, which holds a consonant conjunct
// together across its virama, and GB11, which holds an emoji ZWJ sequence
// together. The tables are generated from the Unicode Character Database by
// cmd/gensegment and the whole of it is checked against Unicode's own
// GraphemeBreakTest.txt — see conformance_test.go.
package segment

import (
	"sort"
	"unicode/utf8"
)

// Break is a character's Grapheme_Cluster_Break property.
type Break uint8

// The Grapheme_Cluster_Break values. Other is the default.
const (
	Other Break = iota
	CR
	LF
	Control
	Extend
	ZWJ
	RegionalIndicator
	Prepend
	SpacingMark
	HangulL
	HangulV
	HangulT
	HangulLV
	HangulLVT
)

// conjunct is a character's Indic_Conjunct_Break property, which rule GB9c
// tests. None is the default.
type conjunct uint8

const (
	conjunctNone conjunct = iota
	conjunctConsonant
	conjunctExtend
	conjunctLinker
)

// asciiLimit is the code point below which two of the three properties are
// known without a table: everything under it is Indic_Conjunct_Break=None and
// not Extended_Pictographic. The break property is read from its table either
// way, because the C0 controls and the two line terminators are in it.
//
// Derived from the tables rather than written down, which is what it has to be:
// it was 0x300, on the reading that nothing below the combining marks is
// pictographic — and U+00A9 © and U+00AE ® are, so the fast path answered "not
// pictographic" for the two characters GB11 exists to hold together. "©‍©" came
// apart at the joiner.
//
// The cost of the true floor is that Latin-1 text bisects where ASCII does not,
// which is one comparison and a short search on a table of a hundred and forty
// ranges. The cost of the wrong floor was a rule that did not apply.
var asciiLimit = func() rune {
	limit := conjunctRanges[0].lo
	if pictRanges[0].lo < limit {
		limit = pictRanges[0].lo
	}
	return limit
}()

// BreakOf returns a character's Grapheme_Cluster_Break.
func BreakOf(r rune) Break {
	if r < asciiLimit {
		switch {
		case r == '\r':
			return CR
		case r == '\n':
			return LF
		case r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F) || r == 0xAD:
			return Control
		}
		return Other
	}
	i := sort.Search(len(breakRanges), func(i int) bool { return breakRanges[i].hi >= r })
	if i < len(breakRanges) && r >= breakRanges[i].lo {
		return breakRanges[i].val
	}
	return Other
}

// pictographic reports Extended_Pictographic, which rule GB11 tests.
func pictographic(r rune) bool {
	if r < 0xA9 { // the lowest character in the property
		return false
	}
	i := sort.Search(len(pictRanges), func(i int) bool { return pictRanges[i].hi >= r })
	return i < len(pictRanges) && r >= pictRanges[i].lo
}

// conjunctOf returns a character's Indic_Conjunct_Break.
func conjunctOf(r rune) conjunct {
	if r < asciiLimit {
		return conjunctNone
	}
	i := sort.Search(len(conjunctRanges), func(i int) bool { return conjunctRanges[i].hi >= r })
	if i < len(conjunctRanges) && r >= conjunctRanges[i].lo {
		return conjunctRanges[i].val
	}
	return conjunctNone
}

// props is everything the rules ask about one character, looked up once.
type props struct {
	br   Break
	cj   conjunct
	pict bool
}

func propsOf(r rune) props {
	// A character below the limit is Other, InCB=None and not pictographic, and
	// the great majority of the text this runs over is below it.
	if r < asciiLimit {
		return props{br: BreakOf(r)}
	}
	return props{br: BreakOf(r), cj: conjunctOf(r), pict: pictographic(r)}
}

// A scanner walks a string and answers, at each character, whether a cluster
// boundary falls before it.
//
// The state is what the rules need to see behind them, and each field is one
// rule's memory:
//
//   - prev is the character before, which most of the rules are stated over;
//   - ri counts the regional indicators immediately behind, because GB12 and
//     GB13 pair flags off from the start of the run and so turn on the parity
//     of that count rather than on the previous character alone;
//   - pict tracks how much of GB11's "pictographic, then any number of
//     extending marks, then a joiner" has been seen;
//   - conj tracks the same for GB9c's "consonant, then a linker, with only
//     extending marks and further linkers between".
//
// Two of those are why this cannot be a table of previous-class against
// next-class: the answer depends on a run behind the previous character, not
// just on the previous character.
type scanner struct {
	prev props
	ri   int
	pict pictState
	conj conjState
	set  bool // whether prev holds a character at all
}

type pictState uint8

const (
	pictNone   pictState = iota
	pictSeen             // Extended_Pictographic Extend*
	pictJoined           // Extended_Pictographic Extend* ZWJ
)

type conjState uint8

const (
	conjNone      conjState = iota
	conjConsonant           // Consonant [Extend Linker]*
	conjLinked              // Consonant [Extend Linker]* Linker [Extend Linker]*
)

// boundaryBefore reports whether a cluster boundary falls between the previous
// character and this one, and folds this one into the state.
//
// The rules are applied in the order UAX #29 states them, and the order is not
// cosmetic: GB4 and GB5 isolate the controls before GB9 could attach a mark to
// one, and GB9c and GB11 are tried before GB999 gives up.
func (s *scanner) boundaryBefore(r rune) bool {
	cur := propsOf(r)
	brk := s.decide(cur)
	s.advance(cur)
	return brk
}

func (s *scanner) decide(cur props) bool {
	if !s.set {
		return true // GB1: sot ÷ Any
	}
	p, c := s.prev, cur
	switch {
	case p.br == CR && c.br == LF:
		return false // GB3
	case p.br == Control || p.br == CR || p.br == LF:
		return true // GB4
	case c.br == Control || c.br == CR || c.br == LF:
		return true // GB5
	case p.br == HangulL && (c.br == HangulL || c.br == HangulV || c.br == HangulLV || c.br == HangulLVT):
		return false // GB6
	case (p.br == HangulLV || p.br == HangulV) && (c.br == HangulV || c.br == HangulT):
		return false // GB7
	case (p.br == HangulLVT || p.br == HangulT) && c.br == HangulT:
		return false // GB8
	case c.br == Extend || c.br == ZWJ:
		return false // GB9
	case c.br == SpacingMark:
		return false // GB9a
	case p.br == Prepend:
		return false // GB9b
	case s.conj == conjLinked && c.cj == conjunctConsonant:
		return false // GB9c
	case s.pict == pictJoined && c.pict:
		return false // GB11
	case p.br == RegionalIndicator && c.br == RegionalIndicator && s.ri%2 == 1:
		return false // GB12, GB13
	}
	return true // GB999
}

// advance folds a character into the state.
func (s *scanner) advance(cur props) {
	if cur.br == RegionalIndicator {
		s.ri++
	} else {
		s.ri = 0
	}

	// GB11's prefix. A pictographic starts one; extending marks continue it; a
	// joiner completes it; anything else ends it. Testing pictographic first
	// matters because a pictographic *after* a completed prefix starts a fresh
	// one rather than leaving the old one standing.
	switch {
	case cur.pict:
		s.pict = pictSeen
	case s.pict == pictSeen && cur.br == Extend:
		// unchanged
	case s.pict == pictSeen && cur.br == ZWJ:
		s.pict = pictJoined
	default:
		s.pict = pictNone
	}

	// GB9c's prefix, in the same shape. A linker is what promotes a consonant's
	// run to one that may attach to the next consonant.
	switch {
	case cur.cj == conjunctConsonant:
		s.conj = conjConsonant
	case s.conj != conjNone && cur.cj == conjunctLinker:
		s.conj = conjLinked
	case s.conj != conjNone && cur.cj == conjunctExtend:
		// unchanged
	default:
		s.conj = conjNone
	}

	s.prev = cur
	s.set = true
}

// A Scanner reports cluster boundaries one character at a time.
//
// It exists because the caller that needs this most — the line breaker — is
// already walking the text rune by rune, and a boundary *list* would cost an
// allocation proportional to the text for the common case where every character
// is its own cluster. A Scanner costs nothing and answers in constant time.
//
// The zero Scanner is ready to use and is positioned before the first character.
// One Scanner reads one string: reset it by assigning Scanner{}.
type Scanner struct {
	sc scanner
}

// Boundary reports whether a grapheme cluster boundary falls immediately before
// r, and advances past r.
//
// It is true for the first character of a string, which is a boundary in UAX
// #29's terms (rule GB1). A caller looking for the positions it may cut at
// should ignore that first answer, as Boundaries does.
//
// This is a rune API and a rune cannot carry one distinction the bytes make: Go's
// decoder hands back U+FFFD for a byte that is not UTF-8, and the same U+FFFD is
// an ordinary character a document may contain. Boundaries has the bytes and
// separates them; a Scanner cannot.
//
// So a caller walking a *string* and wanting Boundaries' answer has to ask
// InvalidByte at each offset first, take a yes as a boundary without asking here,
// and start a new Scanner after it. That is the whole protocol, and following it
// reproduces Boundaries exactly — TestCountAndBoundariesAgreeOnEveryString and
// FuzzBoundaries both check that it does.
//
// A caller that wants the *rune* reading is right not to ask, and there is one:
// paragraph's line breaker decodes the text and re-writes it, so what leaves it
// is a U+FFFD the pieces really contain, and the clusters it must not cut are
// that string's. Which reading is wanted depends on which string the caller is
// going to hand on. See InvalidByte.
func (s *Scanner) Boundary(r rune) bool { return s.sc.boundaryBefore(r) }

// InvalidByte reports whether the character at i in s is Go's replacement for a
// byte that is not UTF-8, rather than a U+FFFD the string contains.
//
// The two are the same rune and only the bytes tell them apart: a decode error
// is one byte wide and a written U+FFFD is three. Every walk over grapheme
// clusters in this repository asks it at the same point and for the same reason
// — an invalid byte is its own cluster on both sides, because a mark that
// attached to one would make a cluster spanning something that is not a
// character.
//
// It is exported so that the rule is stated once and can be asked for. It lived
// inside walkClusters, so a caller holding a string and a Scanner could not get
// the answer Boundaries gives however carefully it read — the difference was
// written down as one the Scanner "legitimately" makes and then skipped over in
// the test that would have pinned it, which is how a third reading of one string
// goes unnoticed. A fuzz target found the two disagreeing in three minutes.
func InvalidByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	r, n := utf8.DecodeRuneInString(s[i:])
	return r == utf8.RuneError && n == 1
}

// Boundaries appends to dst the byte offsets *inside* s at which a grapheme
// cluster begins, in increasing order.
//
// The two ends are left out because they are not choices: every string starts
// and ends a cluster, so a caller looking for the places it may cut wants
// neither. The result is therefore empty for a string of one cluster, and dst is
// returned unchanged for the empty string.
//
// dst is appended to so a caller in a loop can reuse one buffer; pass nil for a
// fresh slice.
func Boundaries(dst []int, s string) []int {
	walkClusters(s, func(i int) {
		// The boundary at nought is not a place to cut: every string starts a
		// cluster, and a caller asking where it may cut wants neither end.
		if i > 0 {
			dst = append(dst, i)
		}
	})
	return dst
}

// Count returns the number of grapheme clusters in s.
//
// It walks the string exactly as Boundaries does, including what both of them
// make of a byte that is not UTF-8, because two answers about the same string
// that disagree are worse than either. They did: Boundaries cut on both sides
// of an invalid byte and reset the scanner, and this let range's U+FFFD through
// as an ordinary character — so a following combining mark attached to it here
// and did not there, and a caller holding both numbers had one cluster more in
// one than in the other.
func Count(s string) int {
	n := 0
	walkClusters(s, func(int) { n++ })
	return n
}

// walkClusters calls at with the byte offset of every cluster start in s,
// including the one at nought.
//
// The one place that decides what a cluster is, so that Boundaries and Count
// cannot drift apart. An invalid byte is its own cluster on both sides: range
// yields U+FFFD for one, which is Other and would let a following mark attach
// to it — a cluster spanning a byte that is not a character — so it is cut
// instead and the scanner starts again after it.
func walkClusters(s string, at func(offset int)) {
	var sc scanner
	for i, r := range s {
		if InvalidByte(s, i) {
			at(i)
			sc = scanner{}
			continue
		}
		if sc.boundaryBefore(r) {
			at(i)
		}
	}
}
