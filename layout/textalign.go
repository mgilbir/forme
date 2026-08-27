package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// text-align: where a line box sits in the width it was given.
//
// CSS 2.1 §16.2. This is the last step of laying out a line and it is deliberately
// separate from breaking it: the width a line is *aligned* at is not the width it
// was *broken* at. §4.1.2 says a trailing space is excluded "for fit, alignment,
// or justification", and the fit has already happened by the time anything here
// runs — so the space that let the break happen must not be counted again when
// deciding where the line goes, or a centred line sits half a space off centre.
//
// The property was registered as understood long before anything read it, which
// meant a centred heading came out flush left and the engine said nothing. That
// is the exact failure the finding vocabulary exists to prevent, and it is worth
// recording that a property table claiming support is not support.

// textAlign is a resolved alignment.
type textAlign uint8

const (
	alignLeft textAlign = iota
	alignRight
	alignCenter
	// alignJustify is recognised and not performed; see alignmentOf.
	alignJustify
)

// alignmentOf resolves the text-align of a block container.
//
// "start" and "end" are resolved against the inline base direction, which is what
// the direction property sets. That is why the initial value matters more than it
// looks: it is "start", so a block with "direction: rtl" and no text-align at all
// is right-aligned, and getting this wrong would leave every right-to-left
// paragraph flush against the edge its text runs away from.
//
// "left" and "right" are physical and are *not* affected by direction. The pair
// exists precisely so that an author can say "that edge" rather than "the edge
// the text starts at".
// rtl is the inline base direction the line was laid out in, which is the
// block's own direction except under "unicode-bidi: plaintext" — there each
// paragraph decides its own, and each has to be aligned against the one it was
// set in or a paragraph of Hebrew would be flush against the left edge of a
// block the algorithm just set right to left.
func alignmentOf(b *Box, rtl bool) textAlign {
	// The direction a value that never becomes physical is resolved against.
	// It is the one the line was set in and it does not move as the walk below
	// climbs, which is the whole of the root case — see matchParent.
	logical := rtl
	for {
		switch strings.ToLower(strings.TrimSpace(b.Style["text-align"])) {
		case "right":
			return alignRight
		case "center":
			return alignCenter
		case "justify", "justify-all":
			// justify-all is CSS Text 3's shorthand value for "justify every
			// line, the last one included". Every line but the last is
			// justified either way; which of the two was written is what
			// lineAlignment reads when the line *is* the last.
			return alignJustify
		case "end":
			return endAlignment(rtl)
		case "left":
			return alignLeft
		case "match-parent":
			if b.Parent == nil {
				// Off the top, and the value stays logical. See matchParent.
				return startAlignment(logical)
			}
			// From here on "start" and "end" are the parent's to resolve, which
			// is the whole of the property.
			b = b.Parent
			rtl = isRTL(b)
			continue
		}
		// start and anything unrecognised. §16.2 makes start the initial value,
		// so this is also the answer for a block that says nothing.
		return startAlignment(rtl)
	}
}

// matchParent is CSS Text §7.1's one value that has to look outside the box it
// is written on, and it is a walk in alignmentOf above rather than a function of
// its own.
//
// "This value behaves the same as inherit, except that an inherited value of
// start or end is calculated against the *parent's* direction value and results
// in a computed value of either left or right."
//
// The first half is nothing: text-align inherits already, so a child with no
// declaration has the parent's value whatever this does. The second half is the
// whole property, and it exists for a paragraph of one direction quoted inside a
// block of another — the quote is aligned with the text around it rather than
// with itself, which is what a reader of the surrounding language expects.
//
// The suite's text-align-match-parent-01 is eight rows of the same shape, and
// the four that matter are the ones where the two directions disagree: a
// left-to-right block whose "start" is inherited by a right-to-left child aligns
// that child *left*, and without this it aligned right.
//
// # The root, which is the case that is easy to get backwards
//
// A chain of them is legal — "div > div { text-align: match-parent }" makes
// every div in a nest one — so the walk can run off the top of the tree. What it
// must produce there is "start" *still logical*: the specification computes
// match-parent against the parent's value, the root has no parent, and there is
// nothing to make physical against.
//
// text-align-match-parent-root-logical says so in its own assert and is built to
// catch the mistake: the root is dir=rtl, the body inside it is dir=ltr, and the
// line must come out flush left. Resolving the root's own direction there
// answers right, which is the reading this is written to avoid — and it is why
// the direction the walk started with is carried along beside the one it climbs
// through.

// startAlignment and endAlignment are the two logical edges, resolved against
// the inline base direction the line was set in.
func startAlignment(rtl bool) textAlign {
	if rtl {
		return alignRight
	}
	return alignLeft
}

func endAlignment(rtl bool) textAlign {
	if rtl {
		return alignLeft
	}
	return alignRight
}

// justificationOf reads text-justify: whether a justified line is stretched at
// all, and which value to report as unhandled if it asked for a method this
// engine does not have.
//
// §7.3 has five values and this engine performs three of them. "auto" is
// deliberately left to the user agent — the specification says so — and
// spreading the word spaces is what every engine does for text that has word
// spaces, which is what "inter-word" names explicitly. "none" is the one value
// that changes the answer rather than the method.
//
// "inter-character" and "distribute" put the slack between *letters* as well,
// which is how Thai and Chinese are justified. "distribute" is the older name
// for the same method and §7.3 says so, so the two are one answer here.
//
// The remaining value, "inter-word" aside, is "auto"'s tailoring per script,
// which this does not attempt: a document that says nothing gets the word
// spaces, and one that names a method gets that method.

// justifyMethod is which things a justified line is stretched between.
type justifyMethod int

const (
	// justifyNone is "text-justify: none": the line is placed at its start edge
	// and not stretched at all.
	justifyNone justifyMethod = iota
	// justifyWords spreads the slack over the word spaces, which is what "auto"
	// and "inter-word" ask for.
	justifyWords
	// justifyCharacters spreads it between every pair of typographic character
	// units, which is "inter-character" and its older name "distribute".
	justifyCharacters
)

func justificationOf(b *Box) (method justifyMethod, unhandled string) {
	switch v := strings.ToLower(strings.TrimSpace(b.Style["text-justify"])); v {
	case "none":
		return justifyNone, ""
	case "", "auto", "inter-word":
		return justifyWords, ""
	case "inter-character", "distribute":
		return justifyCharacters, ""
	default:
		return justifyWords, v
	}
}

// lineAlignment resolves where one line sits, and whether its slack is spread
// across it rather than left at one end.
//
// The last line of a block is not aligned like the rest of it, and CSS Text 3
// §7.2 is where that lives: text-align-last. The reason is typographic and old.
// A justified paragraph looks justified because its lines share two straight
// edges; the last line has no text to fill with, so stretching it would leave a
// handful of words spread across the measure with a hand's breadth between
// them. So the default is to place it where the paragraph starts and leave it
// short — and text-align-last is how an author asks for something else.
//
// "a line right before a forced line break" counts as a last line too, which is
// the specification's own wording and is why the caller passes a flag rather
// than an index: a <br> ends a line for this purpose exactly as the end of the
// block does.
func lineAlignment(b *Box, rtl, last bool) (align textAlign, spread bool) {
	a := alignmentOf(b, rtl)
	if last {
		a = lastLineAlignment(b, rtl)
	}
	if a != alignJustify {
		return a, false
	}
	// A justified line is placed where its start edge is and stretched from
	// there. text-justify: none asks for the placement without the stretching,
	// which leaves an ordinary line at its start edge.
	if m, _ := justificationOf(b); m == justifyNone {
		return startAlignment(rtl), false
	}
	return alignJustify, true
}

// lastLineAlignment is §7.2's own resolution, without the separate question of
// whether justification is switched on at all.
func lastLineAlignment(b *Box, rtl bool) textAlign {
	switch strings.ToLower(strings.TrimSpace(b.Style["text-align-last"])) {
	case "left":
		return alignLeft
	case "right":
		return alignRight
	case "center":
		return alignCenter
	case "start":
		return startAlignment(rtl)
	case "end":
		return endAlignment(rtl)
	case "justify":
		return alignJustify
	}
	// auto, and anything unrecognised. §7.2: "content on the affected line is
	// aligned per text-align-all unless text-align-all is justify, in which
	// case it is aligned per the start value of text-align".
	//
	// justify-all is the exception: it is the spelling that asks for the last
	// line as well, and is the whole difference between the two.
	if strings.EqualFold(strings.TrimSpace(b.Style["text-align"]), "justify-all") {
		return alignJustify
	}
	if a := alignmentOf(b, rtl); a != alignJustify {
		return a
	}
	return startAlignment(rtl)
}

// alignLine returns how far a line's content moves within the width it was given.
//
// used is the width the content actually occupies with its hanging white space
// already discounted. A line at least as wide as the space it has does not move:
// an overfull line overflows to the right whatever the alignment says, because
// moving it would push it off the other edge as well.
func (l *layouter) alignLine(b *Box, align textAlign, lineWidth, used style.Unit) style.Unit {
	slack := lineWidth.Sub(used)
	// A justified line starts where "start" would put it, and the slack is then
	// spread across its spaces by justifyLine, which is the caller's next step
	// and needs the line's runs rather than a single offset. So there is nothing
	// to do here for it, and nothing to report either: the report belongs to the
	// case justifyLine cannot handle, and only that call knows which lines those
	// are.
	switch align {
	case alignRight:
		// The slack may be negative, and then this is the whole of what the
		// alignment does. §16.2 aligns the line box inside the block, and a line
		// too long to fit is still aligned: its right edge stays at the block's
		// right edge and what does not fit hangs off the left. Returning zero
		// for a negative slack — which is what "no room to distribute" reads
		// like — sets such a line flush *left* instead, so it overflows the way
		// a left-aligned one would and the two alignments become the same
		// declaration for exactly the text that most needs them apart.
		if slack < 0 && overflowIsScrollable(b.Style) {
			// Except where the overflow can be scrolled to, and then it cannot:
			// what goes off the *start* edge of a scrollable box is unreachable,
			// because scrolling only ever reaches the other way. The suite says
			// so in the assert of trailing-space-and-text-alignment-002 —
			// preserved spaces under "pre" do not hang, so they "may cause
			// overflow and activate the scrollbars" — and a right-aligned
			// textarea that pushed its own text off the left would be a box
			// whose content no reader could get to.
			return 0
		}
		return slack
	case alignCenter:
		if slack <= 0 {
			// Centring a line that does not fit would push it off the *start*
			// edge as well, and what goes off that edge is unreachable rather
			// than merely outside — there is no scrolling back to it on a page.
			// So an overfull centred line is left where it starts and overflows
			// one way, which is what TestTextAlignDoesNotMoveAnOverfullLine
			// pins and what the suite's trailing-space-and-text-alignment pairs
			// agree with.
			return 0
		}
		// Half the slack, in layout units rather than pixels, so a line with an
		// odd number of units left over is not rounded twice.
		return slack.Div(2)
	}
	return 0
}

// alignedWidth is the width a line occupies for the purpose of aligning it.
//
// It is the pen position at the end of the line, less any run of white space
// hanging past the break. §4.1.2 removes a *collapsible* trailing space outright
// — trimLineEdge does that, and it happens before this — but a *preserved* one
// stays in the runs so that the document's text is what the author wrote. It
// still must not be counted here, or "pre-wrap" text would centre around
// characters that mark no paper.
//
// An inline box's own margin, border and padding has no text either and is the
// opposite case: it marks no paper and it is still part of what the line
// occupies, because it is the box's own width and not a space that happened to
// fall at the break. So it is stepped over rather than subtracted, which leaves
// a hanging space *before* a closing margin still discounted.
func alignedWidth(runs []inlineItem, total style.Unit) style.Unit {
	for i, hangs := range hangingTail(runs) {
		if hangs {
			total = total.Sub(runs[i].Width)
		}
	}
	return style.Max(total, 0)
}

// hangingTail marks the white space at the logical end of a line that §4.1.2
// hangs past it.
//
// It is a walk from the end of the *logical* order and not a test of where
// anything sits, and on a left-to-right line the two would agree. On a
// right-to-left one they do not: rule L1 gives the trailing spaces the
// paragraph's own level, so they are drawn at the line's left edge — before
// everything else. Justification asked "is this space past where the content
// ends", got "no, it is at the very beginning", and stretched the hang. A
// right-to-left justified line came out a space short of its own margin with
// the gap between its words too narrow by the same amount.
//
// The alignment and the justification have to agree about which items these
// are, which is why there is one walk and not two.
func hangingTail(runs []inlineItem) []bool {
	hangs := make([]bool, len(runs))
	for i := len(runs) - 1; i >= 0; i-- {
		item := runs[i]
		if item.Inset {
			continue
		}
		if item.AtomicBox != nil || item.Atomic != nil {
			break
		}
		if strings.TrimSpace(item.Text) != "" {
			break
		}
		if !item.Hangs {
			// break-spaces. Its trailing space is not hanging past the end of the
			// line, it *is* the end of the line — the value exists so that the
			// spaces are content — so it counts towards where the line sits. A
			// right-aligned "a " under break-spaces ends a space short of the
			// edge, and under pre-wrap it does not.
			break
		}
		hangs[i] = true
	}
	return hangs
}
