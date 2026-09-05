package layout

import (
	"testing"
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

// TestAFormFeedTakesRoomOnTheLineItEnds is the half that is not about drawing
// at all, and is the one an ordinary document would notice.
//
// Two facts, and each was a bug on its own. Collapsible white space is removed
// at the end of a line and merged with its neighbours, so a form feed between
// two letters used to take itself out of the line entirely and leave them
// joined — that is the room. And UAX #14 puts it in class BK, so the line ends
// after it whatever white-space says — that is the break, which the suite writes
// as line-breaking-022.
//
// The two are asserted together because a fix for either alone looks like a fix
// for both: a form feed that ends the line trivially "takes room" if the room
// is measured across the break, and one that takes room without ending the line
// passes any test that only counts lines it never made.
func TestAFormFeedTakesRoomOnTheLineItEnds(t *testing.T) {
	const css = `#d { font-family: Courier; font-size: 20px; width: 600px; white-space: pre }`
	// Where each run of text was drawn, in paint order.
	runs := func(text string) []Point {
		t.Helper()
		var out []Point
		for _, op := range paintOf(t, `<div id="d">`+text+`</div>`, css) {
			if v, ok := op.(DrawText); ok {
				out = append(out, v.At)
			}
		}
		return out
	}
	// The control: a space between the same two letters, which under "pre" is
	// a run of its own, so the three characters are three runs on one line.
	space := runs("a b")
	if len(space) != 3 || space[0].Y != space[1].Y || space[1].Y != space[2].Y {
		t.Fatalf("the control drew %v; it is meant to be three runs on one line", space)
	}
	feed := runs("a\fb")
	if len(feed) != 2 {
		t.Fatalf("a form feed between two letters drew %v, want two runs", feed)
	}
	// The break: the letter after it begins a line of its own, at the start.
	if feed[1].Y == feed[0].Y {
		t.Errorf("both letters are on the baseline %v; a form feed is UAX #14's "+
			"class BK and ends the line", feed[0].Y)
	}
	if feed[1].X != feed[0].X {
		t.Errorf("the second letter is at %v and the first at %v; a line begins "+
			"at the start", feed[1].X, feed[0].X)
	}
	// The room: what the form feed draws lies inside the character cell a space
	// occupies, which is the cell one along from the letter before it. The
	// glyph is the four thin strokes of an open rectangle rather than one fill,
	// and it is inset a little inside its cell, so the assertion is containment
	// and not an equality — an equality here would be pinning the inset.
	fills, _ := marksFor(t, "<div id=\"d\">a\fb</div>", css)
	if len(fills) == 0 {
		t.Fatalf("a form feed drew nothing between two letters")
	}
	lo, hi := space[1].X, space[2].X
	for _, r := range fills {
		if r.X < lo || r.X.Add(r.W) > hi {
			t.Errorf("the form feed drew %v, which is outside the cell %v to %v "+
				"that a space occupies; it takes one character's room on the "+
				"line it ends", r, lo.Px(), hi.Px())
		}
	}
}
