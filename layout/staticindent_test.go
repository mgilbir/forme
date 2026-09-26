package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §10.6.4's static position and §16.1's indent.
//
// The static position is where a box would have been had it been static, and a
// box the document wrote as inline would have been on the first line. That line
// begins an indent in from the start edge, so the box does too — and it did not:
// it stood at the block's own edge while the words beside it began an indent
// further in.
//
// §9.7 blockifies every absolutely positioned box, so the *used* display says
// nothing about which hypothetical box §10.6.4 means. What decides is what the
// document wrote, which is Box.staticInline.

// staticX is where an absolutely positioned child with no offsets lands.
func staticX(t *testing.T, css, content string) style.Unit {
	t.Helper()
	root := layoutOf(t, 500, `<div id="c">`+content+`</div>`,
		noDefaults+mono+`#c { width: 300px; font-size: 16px }
		 #a { position: absolute; width: 20px; height: 20px }`+css)
	return find(t, root, "a").BorderRect.X
}

// TestAnInlineStaticPositionTakesTheIndent, on a line that exists: the block has
// words on it as well, so there is a line box and a pen position on it.
func TestAnInlineStaticPositionTakesTheIndent(t *testing.T) {
	px(t, "the static position on an indented first line",
		staticX(t, `#c { text-indent: 20px }`, `<span id="a"></span>xx`), 20)
	px(t, "the same block with no indent",
		staticX(t, ``, `<span id="a"></span>xx`), 0)
}

// TestAnInlineStaticPositionTakesTheIndentWithNoLine is the other half, and the
// one a block walk has to answer: a block whose only child is the box makes no
// line box at all, so there is no pen position to read. The box would still have
// been on the first line, and the first line would still have been indented.
func TestAnInlineStaticPositionTakesTheIndentWithNoLine(t *testing.T) {
	px(t, "the static position where the block made no line",
		staticX(t, `#c { text-indent: 20px }`, `<span id="a"></span>`), 20)
	px(t, "the same block with no indent",
		staticX(t, ``, `<span id="a"></span>`), 0)
}

// TestABlockStaticPositionDoesNotTakeTheIndent is the boundary. §16.1 indents a
// line box, and a box the document wrote as a block would not have been on one:
// its hypothetical box fills the parent's content width and starts at its edge.
func TestABlockStaticPositionDoesNotTakeTheIndent(t *testing.T) {
	px(t, "the static position of a box written as a block",
		staticX(t, `#c { text-indent: 20px } #a { display: block }`, `<div id="a"></div>`), 0)
}

// The same three questions right to left, where the line starts at its right
// edge and so does the indent. §10.3.7 then anchors "right" rather than "left",
// and the static position it anchors is the pen read from the right-hand edge.
//
// Both halves were missing the indent there: the line walk recorded the pen as
// though every right-to-left line began at the block's right edge, and the
// block walk took the indent for a left-to-right line only. The dir=rtl half of
// text-indent/text-indent-with-absolute-pos-child is these cases.

// staticXRTL is staticX in a right-to-left block. #c is 300 wide at the page's
// left edge, so its content right edge is at 300 and a 20px box whose right edge
// is on the pen is at the pen less 20.
func staticXRTL(t *testing.T, css, content string) style.Unit {
	t.Helper()
	return staticX(t, `#c { direction: rtl } `+css, content)
}

// TestAnInlineStaticPositionTakesTheIndentRightToLeft, on a line that exists.
func TestAnInlineStaticPositionTakesTheIndentRightToLeft(t *testing.T) {
	px(t, "the static position on an indented right-to-left first line",
		staticXRTL(t, `#c { text-indent: 20px }`, `<span id="a"></span>xx`), 260)
	px(t, "the same block with no indent",
		staticXRTL(t, ``, `<span id="a"></span>xx`), 280)
}

// TestAnInlineStaticPositionTakesTheIndentWithNoLineRightToLeft: the block walk's
// half, where the box is the block's only content and there is no line box.
func TestAnInlineStaticPositionTakesTheIndentWithNoLineRightToLeft(t *testing.T) {
	px(t, "the static position where a right-to-left block made no line",
		staticXRTL(t, `#c { text-indent: 20px }`, `<span id="a"></span>`), 260)
	px(t, "the same block with no indent",
		staticXRTL(t, ``, `<span id="a"></span>`), 280)
	px(t, "a box written as a block, which the indent does not reach",
		staticXRTL(t, `#c { text-indent: 20px } #a { display: block }`, `<div id="a"></div>`), 280)
	// Inside an empty inline, or before a space that collapses away, the box is
	// met by the line walk and not the block walk, and the line it is met on has
	// nothing on it: no runs, no alignment, and so only the indent to say where
	// its content would have started.
	for _, content := range []string{`<b><span id="a"></span></b>`, `<span id="a"></span> `} {
		px(t, "an empty right-to-left line: "+content,
			staticXRTL(t, `#c { text-indent: 20px }`, content), 260)
		px(t, "an empty left-to-right line: "+content,
			staticX(t, `#c { text-indent: 20px }`, content), 20)
	}
}

// TestAnInlineStaticPositionFollowsTheAlignmentRightToLeft: the pen is where the
// content ended up, and "text-align: left" puts a right-to-left line's content
// at the left. A 60px inline-block comes first, so it spans 0 to 60 and the box
// written after it — logically after, so visually to its left — has its right
// edge at 0. Counting the pen from the block's right edge put it at 240, as
// though the line had been start-aligned.
func TestAnInlineStaticPositionFollowsTheAlignmentRightToLeft(t *testing.T) {
	const pre = `#pre { display: inline-block; width: 60px; height: 10px }`
	px(t, "after a left-aligned right-to-left line's content",
		staticXRTL(t, `#c { text-align: left } `+pre, `<span id="pre"></span><span id="a"></span>`), -20)
	px(t, "and start-aligned, where the content is at the right",
		staticXRTL(t, pre, `<span id="pre"></span><span id="a"></span>`), 220)
}
