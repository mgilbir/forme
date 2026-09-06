package paragraph

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/segment"
)

// The text either side of a run, as much of it as a shaper can read.
//
// A run is shaped with its neighbours because §8.1's boundary between two
// inline elements does not break shaping, and because §5.4 says a word broken
// across a line is still shaped as though it were whole. Both of those are
// questions about what stands *next to* the run: an Arabic letter's form is
// decided by the joining type of the character beside it, looking through the
// transparent ones, and a kern pair is two glyphs. Nothing a shaper does reads
// further than that.
//
// Keeping more is not more correct, and it is expensive in two ways at once.
// The context is part of what is shaped, so a run measured with the rest of the
// paragraph after it shapes the rest of the paragraph; and it is part of the
// memo key, so every lookup hashes it and every entry retains it. A word broken
// across three hundred lines was, at each of them, shaped together with the
// twenty thousand characters that followed — twelve seconds and nine gigabytes
// for a page with one long word on it.
//
// A ligature spanning a boundary is a separate question with a separate answer:
// see Item.MergePre, which names the whole group and is shaped once for it.

// maxContextBytes is how much of the text on one side is kept.
//
// Bytes rather than clusters, so that the trimming itself is O(1) — the point
// is not to walk the text being discarded. It is far past what any script reads
// across a boundary: a hundred and twenty-eight bytes is thirty-two characters
// of the widest encoding, and a shaper reads one or two.
const maxContextBytes = 128

// ContextAfter is the text following a run, trimmed to what a shaper can read.
func ContextAfter(s string) string {
	if len(s) <= maxContextBytes {
		return s
	}
	w := s[:maxContextBytes]
	// Back up to a rune boundary before asking about clusters, since a window
	// cut mid-rune is not text.
	for len(w) > 0 && !utf8.ValidString(w[len(w)-1:]) {
		w = w[:len(w)-1]
	}
	// And to a cluster boundary, so that the trimming never separates a letter
	// from its accent — which would change the very form it is here to decide.
	if b := segment.Boundaries(nil, w); len(b) > 0 {
		return w[:b[len(b)-1]]
	}
	return w
}

// ContextBefore is the text preceding a run, trimmed the same way from the
// other end.
func ContextBefore(s string) string {
	if len(s) <= maxContextBytes {
		return s
	}
	w := s[len(s)-maxContextBytes:]
	for len(w) > 0 && !utf8.RuneStart(w[0]) {
		w = w[1:]
	}
	if b := segment.Boundaries(nil, w); len(b) > 0 {
		return w[b[0]:]
	}
	return w
}
