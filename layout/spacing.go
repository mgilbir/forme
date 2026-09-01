package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
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
// Both take a percentage, and for both it is a percentage of the element's font
// size. css-text-4 gives each of them "normal | <length-percentage>", and the
// suite states the basis twice in the same words — "percentage values of
// word-spacing are relative to the current font-size" in
// word-spacing-percent-001 and the same sentence about letter-spacing in
// letter-spacing-percent-001 — each writing "1em" and "100%" on two lines it
// asks to be identical.
//
// The font size is read at *use* rather than at computed-value time, which is
// what the same test's fourth line is for: a "word-spacing: 100%" on a div at
// "font-size: 0.1em" holding a div at 20px has to give the inner div twenty
// pixels of spacing and not two. So the percentage inherits as a percentage and
// every element resolves it against its own size.
//
// A space's own advance is the other reading of word-spacing and it is wrong,
// though nothing in word-spacing-001 can tell: that test is set in Ahem, whose
// space is exactly one em, so the two answers agree everywhere in it.
func (l *layouter) spacingValue(b *Box, property string) (style.Unit, bool) {
	raw := strings.ToLower(strings.TrimSpace(b.Style[property]))
	if raw == "" || raw == "normal" {
		return 0, false
	}
	return l.lengthOf(b, property, b.FontSize)
}

// indentMode is which line boxes §7.1's indent applies to.
//
// The length says how far; these two say where. "each-line" adds the line after
// every forced break to the one a plain indent moves — the block's first — and
// "hanging" inverts the whole set, so that every line the indent would have
// moved stays where it is and every other line moves instead.
//
// Both are modifiers rather than values, and either may be written on its own or
// with the other, in any order and on either side of the length. That is why
// they are pulled out of the value before the length is read: "4em each-line
// hanging" is a length with two words round it, not a length this engine cannot
// parse.
type indentMode struct{ hanging, eachLine bool }

// indentsLine reports whether a line takes the indent.
//
// first says the line is the block's first, afterForced that the line before it
// ended at a forced break. The two questions become one as soon as "each-line"
// has said whether the second kind counts as a beginning, and "hanging" then
// answers the opposite of whatever that came to.
func (m indentMode) indentsLine(first, afterForced bool) bool {
	begins := first || (m.eachLine && afterForced)
	return begins != m.hanging
}

// textIndent is how far a block container's indented lines are pushed in, and
// which lines those are.
//
// CSS 2.1 §16.1 and css-text-3 §7.1. A percentage is of the containing block's
// width, which for a block container's own lines is its content width — the
// number this is given. A negative value is legal and is one way to write a
// hanging indent by hand, so it is not clamped; "hanging" is the other way, and
// the two are not the same thing — a negative indent moves the first line out,
// the keyword moves every other line in.
func (l *layouter) textIndent(b *Box, width style.Unit) (style.Unit, indentMode) {
	raw := strings.TrimSpace(b.Style["text-indent"])
	if raw == "" || raw == "0" {
		return 0, indentMode{}
	}
	vals, _ := css.ParseComponentValues(raw)
	var mode indentMode
	var length []css.ComponentValue
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Ident {
			switch strings.ToLower(v.Token.Value) {
			case "hanging":
				mode.hanging = true
				continue
			case "each-line":
				mode.eachLine = true
				continue
			}
		}
		length = append(length, v)
	}
	parsed, ok := l.lengthOfValues(b, length)
	if !ok {
		l.reportIndent(b, raw)
		return 0, indentMode{}
	}
	v, ok := parsed.Resolve(width, true)
	if !ok {
		l.reportIndent(b, raw)
		return 0, indentMode{}
	}
	return v, mode
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
