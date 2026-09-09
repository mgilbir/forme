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
