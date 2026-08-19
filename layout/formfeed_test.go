package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A form feed reaching the page as the visible glyph CSS Text 3 asks for.
//
// The character is the whole of what the suite's control-chars-00C document
// contains, injected through "content" — the test says why in a comment of its
// own, "to avoid any mangling by the html parser" — and it asserts visibility by
// requiring the rendering to *differ* from an empty box.
//
// It took two changes to reach the page and either alone would have done
// nothing. The form feed was collapsible white space, so a lone one collapsed to
// nothing before anything could draw it; and it was named out of the visible
// controls, so even preserved it drew no box. See isCollapsibleSpace and
// isVisibleControl.

// marksFor is what a document put on the page: the rectangles it filled, and
// how many runs of text it set.
//
// The two are counted apart on purpose. "Something was drawn" is too weak an
// assertion to pin this, and was measured to be: a form feed that is neither
// collapsible nor a visible control is set as *text*, and a face with no glyph
// for it draws its .notdef — which puts a mark on the page and passes a test
// that asks only whether one is there. What the specification asks for is the
// synthesized box, and a box is a fill with no text beside it.
func marksFor(t *testing.T, htmlSrc, cssSrc string) (fills []Rect, texts int) {
	t.Helper()
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		switch v := op.(type) {
		case FillRect:
			if !v.Rect.Empty() {
				fills = append(fills, v.Rect)
			}
		case DrawText:
			texts++
		}
	}
	return fills, texts
}

func TestAFormFeedIsDrawn(t *testing.T) {
	const css = `#d { font-size: 40px }`
	box := func(content string) []Rect {
		fills, texts := marksFor(t, `<div id="d"></div>`,
			css+`#d::after { content: "`+content+`" }`)
		if texts != 0 {
			t.Errorf("%s was set as text; a control character is drawn as a box and "+
				"never asks a face for a glyph", content)
		}
		return fills
	}
	if got := box(`\00000C`); len(got) == 0 {
		t.Errorf("a form feed put nothing on the page")
	}
	// The control: an empty box really does draw nothing, so the assertion
	// above is about the character and not about the fixture.
	if fills, texts := marksFor(t, `<div id="d"></div>`, css); len(fills)+texts != 0 {
		t.Errorf("the empty control drew %v and %d runs", fills, texts)
	}
	// And it draws the same thing every other control character does, in the
	// same place, which is what makes this one rule rather than a special case
	// for one code point.
	if one, ff := box(`\000001`), box(`\00000C`); !sameRects(ff, one...) {
		t.Errorf("U+0001 drew %v and U+000C drew %v", one, ff)
	}
}

// TestAFormFeedTakesRoomOnTheLine is the half that is not about drawing at all,
// and is the one an ordinary document would notice: collapsible white space is
// removed at the end of a line and merged with its neighbours, so a form feed
// between two letters used to take itself out of the line entirely and leave
// them joined.
func TestAFormFeedTakesRoomOnTheLine(t *testing.T) {
	span := func(text string) style.Unit {
		var lo, hi style.Unit
		first := true
		for _, op := range paintOf(t, `<div id="d">`+text+`</div>`,
			`#d { font-family: Courier; font-size: 20px; width: 600px; white-space: pre }`) {
			v, ok := op.(DrawText)
			if !ok {
				continue
			}
			if first {
				lo, first = v.At.X, false
			}
			hi = v.At.X
		}
		return hi.Sub(lo)
	}
	// Three characters either way, and in a monospaced face the middle one
	// takes the same advance whichever it is.
	if got, want := span("a\fb"), span("a b"); got != want {
		t.Errorf("a form feed between two letters spans %v and a space spans %v; a "+
			"form feed is a character and takes a character's room", got, want)
	}
}
