package layout

import "testing"

// What a leading byte order mark does to a page, which is what makes ignoring
// it a layout fix rather than a tidying of the parse tree.
//
// Left in, the mark is a character in an otherwise empty inline formatting
// context in front of the document's first element: the page gets a line of
// text nobody wrote at the top, and everything below it moves down by a line.
// CSS2/positioning/position-relative-nested-001 is a test with a mark against a
// reference without one, and 25.86px between them.

func TestALeadingByteOrderMarkDoesNotMoveThePage(t *testing.T) {
	const css = `#d { width: 40px; height: 40px; background: rgb(0,128,0) }`
	with := fillsOf(paintOf(t, "\ufeff<div id=\"d\"></div>", css), green)
	without := fillsOf(paintOf(t, `<div id="d"></div>`, css), green)
	if len(with) != 1 || len(without) != 1 {
		t.Fatalf("%d and %d green fills, want one each", len(with), len(without))
	}
	if with[0] != without[0] {
		t.Errorf("with a byte order mark the box is at %v and without one at %v; "+
			"the mark is not content and moves nothing", with[0], without[0])
	}
}

// TestAMarkInsideTheTextStillTakesPart is the containment. Away from the front
// U+FEFF is ZERO WIDTH NO-BREAK SPACE, a character that holds two words
// together, and an engine that dropped it would break a word the author joined.
func TestAMarkInsideTheTextStillTakesPart(t *testing.T) {
	const css = `#d { font-family: Courier; font-size: 20px; width: 60px }`
	// "ab\ufeffcd" is one unbreakable unit: the mark is a word joiner, so the
	// four characters stay on one line even though only three fit.
	got := lineTextsOf(t, layoutOf(t, 600, "<div id=\"d\">ab\ufeffcd</div>", css), "d")
	if len(got) != 1 {
		t.Errorf("the text broke into %q; U+FEFF inside a word joins it", got)
	}
}
