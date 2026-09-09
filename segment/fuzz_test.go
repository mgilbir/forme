package segment

import (
	"testing"
)

// Grapheme cluster boundaries over arbitrary bytes.
//
// The conformance suite next door runs UAX #29's own cases and settles where the
// boundaries *are*. What it cannot settle is what happens to bytes nobody
// tabulated — and this walks a string a rune at a time with a state machine
// behind it, over text that came out of a document.
//
// The contract, which is what a caller depends on rather than the answers:
//
//   - Totality. Neither function panics, whatever the bytes are.
//   - Shape. Every offset is strictly increasing, inside the string, and at the
//     start of a character. A caller cuts the string at these, and a cut in the
//     middle of a character is a mojibake it cannot see coming.
//   - Agreement. All three ways of asking give one answer. Count and Boundaries
//     walk the string the same way, so the number of clusters is one more than
//     the number of places to cut; they did not once, and a caller holding both
//     numbers had one cluster more in one than in the other. A Scanner driven by
//     the protocol its documentation states — ask InvalidByte, take a yes as a
//     boundary, start again after it — gives the same offsets in the same order;
//     it did not, because the protocol was not stated anywhere and its one
//     caller could not have followed it.
//   - Reuse. Appending to a caller's buffer leaves what was already in it alone,
//     which is the whole reason the signature takes one.
//
// What this deliberately does not assert is where the boundaries *should* be for
// bytes that are not text. Unicode tabulates nothing about them, so there is no
// right answer to check against — only the requirement that this package give
// one answer rather than three.

func FuzzBoundaries(f *testing.F) {
	for _, s := range []string{
		"", "a", "abc",
		// The combinations the rules are about: a base and a mark, a
		// regional-indicator pair, an emoji sequence with a joiner, a Hangul
		// syllable in jamo, and a conjunct.
		"é", "\U0001F1E6\U0001F1E7", "\U0001F468‍\U0001F469",
		"각", "क्ष",
		"à́̂",
		// A prepend, and a control that ends a cluster whatever follows.
		"؀a", "a\r\nb", "a\nb",
		// Not text at all, which is the half the conformance files do not cover.
		"\xff\xfe", "a\xffb", "\xed\xa0\x80", "\x00",
		// And the half of that half where it matters what the walk does *after*
		// an invalid byte rather than at it: a combining mark and a second
		// regional indicator both read the state the byte before them left, so
		// a walk that cuts at the byte without starting again gives an answer
		// no seed above can tell from the right one.
		"a\xff\u0301b", "\U0001F1E6\xff\U0001F1E7",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<14 {
			return
		}

		// A buffer with something already in it, so that appending to a
		// caller's slice cannot quietly overwrite it.
		const sentinel = -7
		dst := Boundaries([]int{sentinel}, s)
		if len(dst) == 0 || dst[0] != sentinel {
			t.Fatalf("Boundaries overwrote what was in the buffer: %v", dst)
		}
		got := dst[1:]

		// The offsets a character begins at, which for well-formed text is what
		// utf8.RuneStart says and for anything else is what Go's own decoder
		// does: one byte per replacement character. Taking it from the range
		// rather than from RuneStart is what makes this true of both — a
		// continuation byte standing alone *is* a character to a caller reading
		// the string the way the standard library does, and cutting between two
		// of them is not a cut this package invented.
		starts := map[int]bool{}
		for i := range s {
			starts[i] = true
		}

		last := 0
		for _, at := range got {
			if at <= last {
				t.Fatalf("boundaries are %v, which does not increase from %d", got, last)
			}
			if at >= len(s) {
				t.Fatalf("a boundary at %d of a %d-byte string", at, len(s))
			}
			if !starts[at] {
				t.Fatalf("a boundary at %d is not where a character begins: %q", at, s)
			}
			last = at
		}

		// One more cluster than there are places to cut, for every string with
		// anything in it.
		n := Count(s)
		if s == "" {
			if n != 0 || len(got) != 0 {
				t.Fatalf("the empty string has %d clusters and %d boundaries", n, len(got))
			}
			return
		}
		if n != len(got)+1 {
			t.Fatalf("%d clusters and %d places to cut in %q; the two walks "+
				"disagree", n, len(got), s)
		}

		// And the Scanner, which is the same walk offered a rune at a time,
		// driven the way its documentation says to drive it: ask InvalidByte
		// first, because a rune API cannot tell the decoder's U+FFFD from one
		// the string contains. A caller that follows the contract has to get the
		// answers the two functions above give, or three readings of one string
		// exist — which is what this found before InvalidByte was exported.
		var sc Scanner
		var scanned []int
		for i, r := range s {
			var at bool
			if InvalidByte(s, i) {
				sc, at = Scanner{}, true
			} else {
				at = sc.Boundary(r)
			}
			if at && i > 0 {
				scanned = append(scanned, i)
			}
		}
		if len(scanned) != len(got) {
			t.Fatalf("the scanner found %d boundaries and Boundaries found %d in %q",
				len(scanned), len(got), s)
		}
		for i := range scanned {
			if scanned[i] != got[i] {
				t.Fatalf("boundary %d is at %d by the scanner and %d by Boundaries",
					i, scanned[i], got[i])
			}
		}
	})
}
