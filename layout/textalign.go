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
	switch strings.ToLower(strings.TrimSpace(b.Style["text-align"])) {
	case "right":
		return alignRight
	case "center":
		return alignCenter
	case "justify":
		return alignJustify
	case "end":
		if rtl {
			return alignLeft
		}
		return alignRight
	case "left":
		return alignLeft
	}
	// start, match-parent, and anything unrecognised. §16.2 makes start the
	// initial value, so this is also the answer for a block that says nothing.
	if rtl {
		return alignRight
	}
	return alignLeft
}

// alignLine returns how far a line's content moves within the width it was given.
//
// used is the width the content actually occupies with its hanging white space
// already discounted. A line at least as wide as the space it has does not move:
// an overfull line overflows to the right whatever the alignment says, because
// moving it would push it off the other edge as well.
func (l *layouter) alignLine(b *Box, rtl bool, lineWidth, used style.Unit) style.Unit {
	slack := lineWidth.Sub(used)
	align := alignmentOf(b, rtl)
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
		total = total.Sub(item.Width)
	}
	return style.Max(total, 0)
}
