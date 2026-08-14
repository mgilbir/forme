package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// The three properties that change how much room text takes: letter-spacing,
// word-spacing and text-indent.
//
// All three were in the registry and unread, and all three fail in the way §6.3
// is written about — a paragraph set without the letter-spacing its author asked
// for is a plausible paragraph. What makes them worth grouping is that none of
// them is a painting property, however much they look like one:
//
//   - letter-spacing and word-spacing change the *advance* of a run, so they
//     change where a line breaks and how wide a float around the text wants to
//     be. An implementation that spread the glyphs at paint time and left the
//     measurement alone would produce lines that overflow their box by exactly
//     the spacing it added, with the guardrails silent because layout never saw
//     a run too wide for its line.
//   - text-indent shortens the first line, so it changes what fits on it.
//
// # Why the measurement cache had to learn about them
//
// l.measure is memoized on the face, the text and the size, because the same
// words recur constantly. Two boxes with different letter-spacing produce
// different advances for the same word in the same face at the same size, so the
// spacing is part of the key. Leaving it out would let the first box to measure
// "the" decide its width for every other box in the document — which is exactly
// the bug lengthKey.zeroAdvance records for the "ch" unit, in the same map, for
// the same reason. It produces a wrong page only in a document that uses two
// values, which is the hardest kind to notice.

// spacingFor reads the two properties off a box.
//
// Both are inherited, so this is answered from the box's own computed style and
// needs no walk. "normal" is zero for both — for letter-spacing that is what the
// keyword means outright, and for word-spacing it means "the font's own space,
// unmodified", which is the same thing expressed as an addition of nothing.
func (l *layouter) spacingFor(b *Box) textSpacing {
	var out textSpacing
	if v, ok := l.spacingValue(b, "letter-spacing"); ok {
		out.Letter = v
	}
	if v, ok := l.spacingValue(b, "word-spacing"); ok {
		out.Word = v
	}
	return out
}

// spacingValue resolves one of the two, or reports that it is "normal".
//
// A percentage is not accepted by either property; the basis of zero means one
// resolves to nothing rather than to a fraction of something arbitrary.
func (l *layouter) spacingValue(b *Box, property string) (style.Unit, bool) {
	raw := strings.ToLower(strings.TrimSpace(b.Style[property]))
	if raw == "" || raw == "normal" {
		return 0, false
	}
	return l.lengthOf(b, property, 0)
}

// textIndent is how far the first line of a block container is pushed in.
//
// CSS 2.1 §16.1. A percentage is of the containing block's width, which for a
// block container's own first line is its content width — the number this is
// given. A negative value is legal and is how a hanging indent is written, so it
// is not clamped.
func (l *layouter) textIndent(b *Box, width style.Unit) style.Unit {
	raw := strings.TrimSpace(b.Style["text-indent"])
	if raw == "" || raw == "0" {
		return 0
	}
	if v, ok := l.lengthOf(b, "text-indent", width); ok {
		return v
	}
	// A value that did not resolve. The keyword forms — "each-line" and
	// "hanging" — are the likely ones, and both change which lines are indented
	// rather than by how much, so treating them as a plain length would indent
	// the wrong lines. Reported rather than guessed at.
	l.reportIndent(b, raw)
	return 0
}

// reportIndent names an indent that did not resolve, once per value for the
// whole document.
//
// Not once per element, and not with a path: the value came from a stylesheet
// that a hundred paragraphs may share, and a hundred findings naming a hundred
// paragraphs would bury the one thing the author has to change. Leaving the Path
// off is what produces that — the Recorder suppresses a repeat of a finding
// identical in rule, message, property and place, and with no place to differ by
// every call after the first collapses into the first. rec.Count still knows how
// many there were.
func (l *layouter) reportIndent(b *Box, raw string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "the text-indent " + quoteValue(raw) +
			" could not be resolved, so the first line was not indented",
		Property: "text-indent",
	})
}
