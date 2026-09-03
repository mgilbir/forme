package paragraph

import (
	"strings"
	"unicode"

	"github.com/mgilbir/forme/shape"
)

// Questions about characters.
//
// Whether a run would draw anything, whether a face has the glyphs for it, and
// how to name a character in a finding. They are here rather than beside the
// code that reports, because none of them needs a document — a run of formatting
// characters marks no paper whoever is asking.

// DescribeRune names a character for a diagnostic, by code point as well as by
// its shape — the shape is what the author recognises and the code point is what
// they can search for, and a character with no glyph often cannot be shown at
// all in whatever is reading the report.
func DescribeRune(r rune) string {
	out := "U+" + strings.ToUpper(hex(uint32(r)))
	if unicode.IsPrint(r) {
		out += " (" + string(r) + ")"
	}
	return out
}

func hex(v uint32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0000"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{digits[v&0xf]}, b...)
		v >>= 4
	}
	for len(b) < 4 {
		b = append([]byte{'0'}, b...)
	}
	return string(b)
}

// UnsupportedScript names why a rune cannot be laid out, or reports false.
//
// The right-to-left scripts used to be here, and are not any more: the
// bidirectional algorithm is applied per paragraph and its reordering per line —
// see bidi.go — so Hebrew, Arabic, Syriac and Thaana are set in the order they
// are read. Shaping them was already forme's, and still is.
func UnsupportedScript(r rune) (string, bool) {
	if NeedsDictionaryBreaking(r) && !HasDictionary(r) {
		// The ranges are UAX #14's class SA rather than a list of blocks, which
		// is the same set said by the standard that defines it — see
		// NeedsDictionaryBreaking, which is also what breaks the text.
		//
		// And only where the vocabulary is missing. A script this engine has a
		// word list for is broken where its words are, which is what the
		// property asks for and leaves nothing to report; the fallback below is
		// what the rest of class SA still gets. See HasDictionary.
		return "this script writes no spaces between words, so finding a line " +
			"break needs a dictionary, which this engine does not have for it; " +
			"the line is broken between typographic character units instead, " +
			"which is not where the words are", true
	}
	return "", false
}

// MissesVisible reports whether a face cannot set some character of text that
// would have put ink on the page.
//
// It is one predicate rather than two because it has two callers that must not
// disagree: the guardrail that reports a missing glyph, and the fallback that
// goes looking for a face which has it. They did disagree, briefly, and the
// result was the fallback substituting a whole different font for a paragraph
// whose only "missing" character was a no-break space — changing every metric on
// the page to fix nothing. Asking the same question twice in two ways is how
// that happens, so it is asked once.
//
// Shaping the whole run first is what keeps it cheap: the answer is almost
// always no, and only then is it worth walking the characters.
func MissesVisible(face *shape.Face, text string) bool {
	if _, missing := face.ShapeGlyphs(text); missing == 0 {
		return false
	}
	for _, r := range text {
		if r == '\n' || r == '\t' || SubstitutesExactly(r) {
			continue
		}
		if _, missing := face.ShapeGlyphs(string(r)); missing > 0 {
			return true
		}
	}
	return false
}

// MarksNoPaper reports whether a character is a space by definition.
//
// A face that cannot encode one of these is not a problem to report. The encoder
// substitutes a space for anything it cannot represent, and for a character that
// was never going to put ink down that substitution is either exactly right — a
// no-break space *is* a space, differing only in whether a line may break at it,
// which is settled long before the face is asked — or wrong by a fraction of an
// em, as for the fixed-width spaces whose whole purpose is to be a particular
// width.
//
// The distinction matters because the substitution is not harmless in general. A
// Hebrew letter the face cannot encode also becomes a space, so the word does
// not appear as a row of boxes — it is simply absent, from the page and from the
// text extracted out of it. That is worth an error. A no-break space becoming a
// space is not, and reporting it was the most common finding this engine
// produced: 154 documents in the reftest suite raised it for U+00A0 alone.
//
// # Why the format characters are not listed here
//
// They were, and the list could not be observed. A planted defect that deleted
// the whole format-character branch — soft hyphen, the zero-width spaces, the
// bidi embeddings and isolates, the byte order mark — broke nothing, and the
// reason is that shaping already answers "not missing" for every one of them,
// on both kinds of face. A simple face encodes through WinAnsi and drops them;
// a composite face shapes them to no glyph and no advance, which is what they
// are for. Measured on Ahem: every one reports missing=0, and so does the
// no-break space, because a composite face has a real glyph for it.
//
// So only the space separators are here, and only the simple faces need them —
// which is to say the fourteen standard PDF faces, which is what a document gets
// unless a caller supplies something else.
func MarksNoPaper(r rune) bool {
	switch {
	case r == 0x00A0, // no-break space
		r == 0x1680,                // ogham space mark
		r >= 0x2000 && r <= 0x200A, // en quad through hair space
		r == 0x202F,                // narrow no-break space
		r == 0x205F,                // medium mathematical space
		r == 0x3000:                // ideographic space
		return true
	}
	return false
}

// SubstitutesExactly reports whether a face that cannot encode this character
// loses nothing by putting a space there instead.
//
// This is a narrower question than MarksNoPaper and the difference is the whole
// point of having both. MarksNoPaper asks whether a character puts ink on the
// page, and answers the *finding*: a space the reader will never see is not a
// glyph worth reporting missing. This one asks whether the substitution changes
// the page at all, and answers whether to look for another *face*.
//
// Only the no-break space. It differs from a space in whether a line may break
// at it, which is settled long before a face is asked, so a space in its place
// is exactly right and 154 documents in the reftest suite are quiet because of
// it.
//
// Every other space separator has a width of its own and that width is the whole
// of what it is for. An ideographic space is one em; the standard PDF faces have
// no glyph for it and give it their ordinary space's quarter em, which is how
// "ああ　" came out four pixels short of the "あああ" it has to cover in
// trailing-ideographic-space-001. The em space, the en space, the figure space
// and the thin space are the same case. A face that *has* them is a face that
// gets the width right, and it is worth going to find one.
func SubstitutesExactly(r rune) bool {
	return r == 0x00A0
}
