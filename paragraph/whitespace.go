package paragraph

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// White space: the white-space property, and CSS Text §4's processing rules.
//
// # Why this is not one function
//
// The obvious shape is a function from a text node to the text that will be
// drawn, and it is wrong, because two of §4's rules are not about a text node.
//
// §4.1.1 collapses a space that follows another collapsible space "even one
// outside the boundary of the inline containing that space, provided both are
// within the same inline formatting context" — so "a <span> </span> b" is three
// text nodes and one space, and no function that sees one node at a time can
// say so. §4.1.2 then removes the collapsible space at the beginning and end of
// each *line*, and where a line ends is not known until it has been broken.
//
// So the processing is split across three stages, and the split follows what
// each rule needs rather than what is convenient:
//
//   - **Phase I, here and per node.** The segment break transformation, the
//     collapsing of a run of spaces and tabs within one node, and the removal
//     of the collapsible spaces around a segment break. What comes out still
//     has a space at each end of a node whose text had one: whether it survives
//     is not this stage's question.
//   - **The flattening, in inline.go.** The cross-boundary half of §4.1.1's
//     fourth rule, carried on inlineState because the flattening is the one
//     pass that walks an inline formatting context in document order.
//   - **Line breaking, in inline.go.** §4.1.2 entirely: the line-edge removal,
//     the tab stops, and the hanging of preserved spaces past the line's end.
//
// Only the first stage is about §4.1.1's white space, which is "spaces (U+0020),
// tabs (U+0009), and segment breaks" and nothing else. §4.1.2 is written over a
// wider set — "white space, other space separators, and/or preserved tabs" — so
// an ideographic space reaches the line breaker as text that nothing collapsed
// and hangs at the end of a line like any other space. IsOtherSpaceSeparator
// below is that set.
//
// # What is left out
//
// The segment break transformation's zero-width-space exception is applied
// within a text node and not across two: "a<span>​</span>\nb" gets the
// space that "a​\nb" would not. Closing it would mean carrying the last
// rune of the previous node through box construction, which is a channel that
// exists for nothing else.
//
// Bidi formatting characters are not "ignored as if they were not there" while
// white space is collapsed, as §4.1.1 requires: a formatting character between
// two spaces stops them collapsing into one. They *are* kept out of the way
// everywhere it matters afterwards — the algorithm removes them from its own
// view (rule X9) and the shaper draws nothing for them — so the cost is a
// stray space's width in a document that puts a directional control in the
// middle of one, and not text in the wrong order.

// WhiteSpace is what the property sets, which is three independent bits and one
// variant. Modelling it as the bits rather than as six keywords is what stops
// "does this wrap" and "does this collapse" being asked as "is the value one of
// these four strings" at each of the several places that need to know.
type WhiteSpace struct {
	// Collapse says a run of spaces and tabs becomes a single space, and that
	// the space is removed at a line edge.
	Collapse bool
	// PreserveBreaks says a segment break survives as a line break rather than
	// being transformed into a space.
	PreserveBreaks bool
	// Wrap says a line may break at an opportunity. It is independent of the
	// other two, which is the whole reason nowrap and pre-Wrap both exist.
	Wrap bool
	// BreakSpaces is the one value that is not a combination of the three.
	//
	// It differs from pre-wrap in two ways that go together: a preserved space
	// at the end of a line does not hang — it takes room and can overflow — and
	// there is a break opportunity after every one of them rather than after
	// the run. That is what it is for: it is the value for text where the
	// spaces are data.
	BreakSpaces bool
}

// WhiteSpaceFor reads the two longhands the white-space shorthand sets.
//
// They are two properties rather than one because "text-wrap: nowrap" sets the
// wrapping half without saying anything about collapsing, and the two spellings
// have to compete in the cascade rather than in the layout code — see the
// shorthands table in style/property.go.
//
// An unrecognised value in either gives that longhand's initial one. That is
// what the cascade would have produced had the declaration been thrown out, and
// it is the answer that cannot lose text: a collapse value read as "preserve" by
// mistake would leave a document's indentation in the page, but a mode read as
// "nowrap" by mistake would run a paragraph off the edge.
func WhiteSpaceFor(cs style.ComputedStyle) WhiteSpace {
	ws := WhiteSpaceOf(cs["white-space-collapse"])
	ws.Wrap = !strings.EqualFold(strings.TrimSpace(cs["text-wrap-mode"]), "nowrap")
	return ws
}

// WhiteSpaceOf reads white-space-collapse alone, leaving the wrapping half at
// its initial value. Every caller that has a style should use WhiteSpaceFor;
// this is for the places that have only the one value — and for the tests, whose
// cases are written as the collapse keyword.
func WhiteSpaceOf(value string) WhiteSpace {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "preserve":
		return WhiteSpace{PreserveBreaks: true, Wrap: true}
	case "preserve-breaks":
		return WhiteSpace{Collapse: true, PreserveBreaks: true, Wrap: true}
	case "break-spaces":
		return WhiteSpace{PreserveBreaks: true, Wrap: true, BreakSpaces: true}
	}
	return WhiteSpace{Collapse: true, Wrap: true}
}

// WordBreak is what the word-break property sets: whether a line may end
// between two characters of a word rather than only between words.
//
// CSS Text §5.2 gives it four values and this is a bool, which is a statement
// about two of them rather than a simplification. "normal" and "break-all" are
// the two this engine distinguishes; "keep-all" and "auto-phrase" change where
// CJK and Korean text may break and are read as normal *and reported*, because
// a value that moves a break and is silently ignored produces a line broken in
// a place the author asked it not to be.
type WordBreak struct {
	// BreakAll allows a line to end at any typographic character unit boundary
	// inside a word, which is the grapheme cluster — see internal/grapheme for
	// why that is the unit and why the shaper's clusters are not it.
	BreakAll bool
}

// WordBreakOf reads the property. The second result is the value to report as
// unhandled, or the empty string.
//
// As with white-space, an unrecognised value gives the initial one rather than a
// guess: normal is the value that breaks in the fewest places, so a
// misinterpretation overflows a line rather than cutting a word open.
func WordBreakOf(value string) (WordBreak, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "break-all":
		return WordBreak{BreakAll: true}, ""
	case "keep-all", "auto-phrase":
		return WordBreak{}, strings.ToLower(strings.TrimSpace(value))
	case "break-word":
		// Normal, and deliberately: the value's whole effect is on
		// overflow-wrap, which OverflowWrapOf reads for itself. It is named here
		// so that it is visibly handled rather than falling through with the
		// misspellings — and so that it is not reported as a value this engine
		// ignores, which it no longer does.
		return WordBreak{}, ""
	}
	return WordBreak{}, ""
}

// LineBreak is what the line-break property sets: how strict the rules are about
// where a line may end.
//
// Three of its four values are about CJK text — loose, normal and strict move
// breaks around small kana, iteration marks and centred punctuation — and this
// engine's CJK breaking is one rule, "between two ideographs", which none of the
// three refines. They are read as auto for that reason, and reported only over
// text that contains an ideograph: the suite has three tests whose whole
// assertion is that "line-break: loose" changes nothing about "XX    XX", and a
// warning there would be a false one.
//
// The fourth is different in kind and is implemented. CSS Text §5.3:
//
//	anywhere: There is a soft wrap opportunity around every typographic
//	character unit, including around any punctuation character or preserved
//	white spaces, or in the middle of words, disregarding any prohibition
//	against line breaks, even those introduced by characters with the GL, WJ, or
//	ZWJ character class or mandated by the word-break property.
type LineBreak struct {
	// Anywhere is that value: an opportunity at every grapheme cluster boundary,
	// and no prohibition survives it.
	Anywhere bool
}

// LineBreakOf reads the property. The second result is the value to report as
// unhandled, or the empty string — and reporting it is still conditional on the
// text, which is the caller's decision rather than this one's.
func LineBreakOf(value string) (LineBreak, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anywhere":
		return LineBreak{Anywhere: true}, ""
	case "loose", "normal", "strict":
		return LineBreak{}, strings.ToLower(strings.TrimSpace(value))
	}
	return LineBreak{}, ""
}

// OverflowWrap is what the overflow-wrap property sets: whether a word with
// nowhere to break may be broken anyway rather than overflowing its line.
//
// It is a different shape of rule from word-break and the difference decides
// where it is implemented. break-all adds opportunities to the text, so it
// belongs in SplitAtBreaks and can be decided by looking at the characters.
// overflow-wrap adds none: CSS Text §5.5 makes its opportunities exist only
// "if there are no otherwise-acceptable break points in the line", so it is not
// a property of the text at all but of what the breaker should do having
// already failed. It lives in breakOneLine for that reason.
type OverflowWrap struct {
	// BreakWord allows the last-resort break.
	BreakWord bool
	// Anywhere is the value that also shrinks the min-content size, so a
	// shrink-to-fit box narrows to its widest *character* rather than to its
	// widest word. break-word deliberately does not: §5.5 says its opportunities
	// "are not considered when calculating min-content intrinsic sizes".
	Anywhere bool
}

// OverflowWrapOf reads the property, taking the winner of overflow-wrap and its
// alias word-wrap.
//
// Both names are legal and mean the same thing, so the one to obey is whichever
// the cascade resolved last. The cascade cannot tell them apart — they are two
// entries in the registry, not one property with two spellings — so an author
// who sets overflow-wrap on a rule and word-wrap on a more specific one gets the
// wrong answer here. Taking the non-initial value is what makes the common case
// right: a document sets one of them.
func OverflowWrapOf(style map[string]string) OverflowWrap {
	// word-break: break-word is not a word-break value at all. CSS Text 3 §5.2
	// keeps it "for web-compatibility" and defines it by what it does elsewhere:
	// it "has the same effect as word-break: normal and overflow-wrap: anywhere,
	// regardless of the actual value of the overflow-wrap property". So it is
	// read here rather than there, it wins over whatever overflow-wrap says, and
	// word-break itself is left at normal.
	//
	// The "regardless" is the part worth writing down: this is the one value in
	// either property that overrides the other, and reading it as a *default*
	// for overflow-wrap would give the wrong answer for the document that sets
	// both — which is what word-break-break-word-overflow-wrap-interactions is.
	if strings.EqualFold(strings.TrimSpace(style["word-break"]), "break-word") {
		return OverflowWrap{BreakWord: true, Anywhere: true}
	}
	value := strings.ToLower(strings.TrimSpace(style["overflow-wrap"]))
	if value == "" || value == "normal" {
		value = strings.ToLower(strings.TrimSpace(style["word-wrap"]))
	}
	switch value {
	case "break-word":
		return OverflowWrap{BreakWord: true}
	case "anywhere":
		return OverflowWrap{BreakWord: true, Anywhere: true}
	}
	return OverflowWrap{}
}

// CollapseWhitespace is §4.1.1 Phase I over one text node.
//
// It is linear in the length of the text and allocates one builder, which is
// not a micro-optimisation: the input is untrusted, and a megabyte of
// alternating spaces and newlines is a document somebody will send.
func CollapseWhitespace(text, value string) string {
	ws := WhiteSpaceOf(value)
	if !ws.Collapse {
		// pre, pre-wrap and break-spaces keep every space and every tab, so all
		// that is left of Phase I is the segment break normalisation — which
		// applies to every value, because CSS Text counts a CRLF as one break
		// and this engine's HTML parser does not fold it.
		return NormaliseBreaks(text)
	}

	// U+200B ZERO WIDTH SPACE is the segment break transformation's one
	// exception, and it exists for source that has been hard-wrapped: an author
	// who marked a break opportunity at the end of a line meant the opportunity
	// and not a space as well.
	const zwsp = '​'

	var out strings.Builder
	out.Grow(len(text))

	// A run of collapsible white space is emitted when it *ends*, because what
	// it becomes depends on what was in it and on what follows it.
	var last rune // the last rune written, for the zero-width-space rule
	// The last character written that a reader would see, which is the one the
	// East Asian rule is about. It is not the same as last: a variation selector
	// or a soft hyphen written before a segment break is not the character
	// before the break, and the suite has a test of exactly that —
	// segment-break-transformation-ignorable-1 writes the Han characters with
	// their variation selectors and asks for the breaks to go anyway.
	var lastSeen rune
	inRun, breaks, afterCR := false, 0, false

	// flush takes the character that ends the run twice over: as it stands, for
	// the zero-width-space rule, and as a reader would see it, for the East
	// Asian one. They are not the same question — U+200B is itself
	// default-ignorable, so a rule that looked past what is not drawn would look
	// straight past the character the rule before it is about.
	flush := func(next, nextSeen rune) {
		if !inRun {
			return
		}
		n := breaks
		inRun, breaks, afterCR = false, 0, false
		switch {
		case n == 0:
			// Spaces and tabs only: §4.1.1's third and fourth rules, a tab
			// becoming a space and the run becoming one of them.
			out.WriteByte(' ')
			last = ' '
		case ws.PreserveBreaks:
			// pre-line. The first rule removed the spaces and tabs around the
			// breaks; the breaks themselves are not collapsible, so a blank
			// line in the source stays a blank line. Emitting one break for a
			// run of them would close up every paragraph gap in the document.
			for ; n > 0; n-- {
				out.WriteByte('\n')
			}
			last = '\n'
		case last == zwsp || next == zwsp:
			// The break is removed, leaving the zero-width space behind.
		case removesSegmentBreak(lastSeen) && removesSegmentBreak(nextSeen):
			// §4.1.1's East Asian rule. A newline between two ideographs is not
			// a word boundary — Japanese and Chinese are written without spaces
			// between words — so the break goes rather than becoming one.
			//
			// Without this, a paragraph hard-wrapped in the source gains a space
			// at the end of every line it was wrapped at, in the middle of
			// words, all through the text. It is the single most visible thing
			// this engine got wrong about CJK, and it is wrong in the direction
			// that looks deliberate.
		default:
			out.WriteByte(' ')
			last = ' '
		}
	}

	// pending holds the bidi controls met inside a run of white space. They are
	// written out after the run collapses rather than where they stood, so that
	// the space that survives is on the side of the control the markup would
	// have put it: a boundary written as two elements keeps the space belonging
	// to the first of them, and a boundary written as a control has to agree.
	var pending []rune

	for i, r := range text {
		if IsBidiControl(r) {
			// Not a character of the text: it is an instruction to the
			// bidirectional algorithm, and it must not break a run of white
			// space in two. "ccc RLO lll" has one space in it and not two,
			// which is what makes it identical to the same boundary written as
			// markup — the whole of what bidi-003 and its neighbours compare.
			if inRun {
				pending = append(pending, r)
			} else {
				out.WriteRune(r)
			}
			continue
		}
		if r < 0x80 && isCollapsibleSpace(byte(r)) {
			inRun = true
			switch r {
			case '\r':
				breaks++
				afterCR = true
			case '\n':
				// A CRLF is one segment break. Counting two would put a blank
				// line into every pre-line document written on Windows.
				if !afterCR {
					breaks++
				}
				afterCR = false
			default:
				afterCR = false
			}
			continue
		}
		flush(r, nextSeen(text[i:]))
		for _, c := range pending {
			out.WriteRune(c)
		}
		pending = pending[:0]
		out.WriteRune(r)
		last = r
		if !IsDefaultIgnorable(r) {
			lastSeen = r
		}
	}
	flush(0, 0)
	for _, c := range pending {
		out.WriteRune(c)
	}
	return out.String()
}

// NormaliseBreaks turns every CRLF and lone CR into a single LF.
//
// It is the part of the segment break transformation that applies even where
// nothing collapses: §4.1.1 counts "\r\n" as one segment break, so a <pre>
// written on Windows must not gain a blank line between every pair of its own.
func NormaliseBreaks(text string) string {
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] != '\r' {
			out.WriteByte(text[i])
			continue
		}
		out.WriteByte('\n')
		if i+1 < len(text) && text[i+1] == '\n' {
			i++
		}
	}
	return out.String()
}

// isCollapsibleSpace is the set CSS 2.1 §16.6.1 calls white space in the source.
//
// A no-break space is deliberately absent: it is not white space for this
// purpose, which is the whole reason an author writes one. A form feed is
// present and is treated as a space rather than as a segment break — CSS Text
// defines a segment break in terms of the document's newlines, and no HTML
// parser produces a line from a form feed.
func isCollapsibleSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// IsOtherSpaceSeparator is §4.1's term of art, and the definition is exact:
// "all characters in the Unicode general category Zs except space (U+0020) and
// no-break space (U+00A0)".
//
// The set matters because §4.1.2's fourth rule is written over "white space,
// other space separators, and/or preserved tabs" — so an ideographic space at
// the end of a line hangs exactly as an ordinary one does, while §4.1.1 never
// touches it, because Phase I is defined over "spaces (U+0020), tabs (U+0009),
// and segment breaks" and nothing else. The pair of rules is what makes
// "ああ　" set two characters wide with the third hanging past the edge.
//
// It is written out rather than taken from unicode.IsSpace, which is a different
// set: it holds the segment breaks and U+0085, and it holds U+00A0, and each of
// those three would be wrong here. Zs has not gained a member since Unicode 6.3
// removed U+180E from it, so the table is a table and not a snapshot.
func IsOtherSpaceSeparator(r rune) bool {
	switch {
	case r == 0x1680: // OGHAM SPACE MARK
		return true
	case r >= 0x2000 && r <= 0x200A: // EN QUAD .. HAIR SPACE
		return true
	case r == 0x202F: // NARROW NO-BREAK SPACE
		return true
	case r == 0x205F: // MEDIUM MATHEMATICAL SPACE
		return true
	case r == 0x3000: // IDEOGRAPHIC SPACE
		return true
	}
	return false
}

// SeparatorBreaksAfter reports whether a line may end after one of them.
//
// Hanging and breaking are different questions, and this is the second: §4.1.2
// says every one of these hangs, and UAX #14 says only some of them offer a
// soft wrap opportunity. The two that do not are the two that are no-break
// characters by name — U+2007 FIGURE SPACE, which holds a column of digits
// together, and U+202F NARROW NO-BREAK SPACE — and both are class GL. The rest
// are class BA, except U+3000, which is class ID and breaks on both sides like
// the ideographs it is spaced among.
//
// break-spaces overrides all of it: that value puts an opportunity "after every
// preserved white space character and after every other space separator", with
// no exception for the no-break ones, and it is the caller that applies it.
func SeparatorBreaksAfter(r rune) bool {
	return r != 0x2007 && r != 0x202F
}

// removesSegmentBreak reports whether a character is one of the pair §4.1.1
// names: "the East Asian Width property of both the character before and after
// the segment break is F, W, or H (not A), and neither side is Hangul".
//
// The two halves are two of Unicode's own tables, generated together — see
// cmd/geneastasian for what each is and why A is not among them. The policy is
// the "and", and it is here rather than folded into the data so that a reader
// can check either table against the file it came from.
//
// Hangul is carved out because Korean is written with spaces between its words.
// A newline between two Hangul syllables is a word boundary and must stay one,
// which is exactly what the rule would otherwise destroy: the syllables are wide
// and would satisfy every other part of it.
func removesSegmentBreak(r rune) bool {
	return inRanges(r, eastAsianWideRanges[:]) && !inRanges(r, hangulRanges[:])
}

// nextSeen is the first character of the text that a reader would see: the one
// the segment break rule means by "the character after the break".
//
// It looks past the characters nothing is drawn for, which is what makes
// "社︀\n福︀" behave like "社\n福" — the suite's
// segment-break-transformation-ignorable-1 writes Han characters with their
// variation selectors and asks for the break to go anyway, and a reader who
// cannot see the selector would not expect it to change the answer.
//
// It is bounded by that run rather than by the text: the scan stops at the first
// character that is not ignorable, and the loop that called it consumes what was
// scanned, so no character is looked at twice.
func nextSeen(text string) rune {
	for _, r := range text {
		if IsDefaultIgnorable(r) || IsBidiControl(r) {
			continue
		}
		return r
	}
	return 0
}

// inRanges searches one of the generated tables, which are sorted and disjoint.
func inRanges(r rune, table []struct{ lo, hi rune }) bool {
	if len(table) == 0 || r < table[0].lo {
		return false
	}
	lo, hi := 0, len(table)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		switch {
		case r < table[mid].lo:
			hi = mid - 1
		case r > table[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}
