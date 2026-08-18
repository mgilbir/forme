package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The baseline of an atomic inline whose only content is a list marker.
//
// §10.8.1 gives an inline-block the baseline of its last in-flow line box, and
// an inline-table the baseline of its first row, and both fall back to the
// bottom margin edge when there is no line box to be found. §12.5.1 puts a list
// item's marker in the item's principal box — so an item with a marker and no
// words is not a box with nothing on any line, and the fallback is the wrong
// answer for it.
//
// It was reached anyway, and the result was not subtle: the marker of
// "<div style='display:inline-block'><span style='display:list-item'></span></div>"
// was drawn a whole ascent below where a browser puts it, hanging under the line
// instead of sitting on it. Six of the suite's list tests are that document —
// three for the inline-block and three for the inline-table, one each for
// list-style, list-style-type and list-style-image.
//
// The document is not as contrived as it looks. An empty list item is what a
// checklist renders while its label is still loading, and an inline-block round
// it is how a marker is kept beside something rather than above it.

// markerY is the baseline the one marker in a document was drawn at.
func markerY(t *testing.T, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	var got []Point
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok && v.Text == "▪" {
			got = append(got, v.At)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d markers drawn, want 1: %v", len(got), got)
	}
	return got[0].Y
}

// TestAnEmptyListItemsMarkerSitsWhereAFullOnesDoes.
//
// The suite compares each of the six documents against one reference — a plain
// block list item with a single non-breaking space in it — so the assertion is
// that all of them put the square in the same place, and the block case is the
// one that was already right.
func TestAnEmptyListItemsMarkerSitsWhereAFullOnesDoes(t *testing.T) {
	const square = `list-style: square; margin-left: 96px`

	want := markerY(t, `<p>P</p><div id="d">&#160;</div>`,
		`#d { display: list-item; `+square+` }`)

	for _, tc := range []struct{ what, css string }{
		{"a block list item with no content",
			`#w { ` + square + ` } #s { display: list-item }`},
		{"an inline-block round an empty item",
			`#w { display: inline-block; ` + square + ` } #s { display: list-item }`},
		{"an inline-table round an empty item",
			`#w { display: inline-table; ` + square + ` } #s { display: list-item }`},
		{"an inline-block round an item with content",
			`#w { display: inline-block; ` + square + ` } #s { display: list-item }`},
	} {
		got := markerY(t, `<p>P</p><div id="w"><span id="s"></span></div>`, tc.css)
		if got != want {
			t.Errorf("%s: the marker is at %v and a block item's is at %v",
				tc.what, got, want)
		}
	}
}

// TestAnAtomicInlineWithNoLineBoxAtAllStillFallsBack is the containment case,
// and the reason this is a marker check rather than "treat an empty box as
// having a line".
//
// §10.8.1's fallback is right for a box that really has nothing on any line, and
// it is what makes an empty inline-block sit *on* the text baseline rather than
// above it. Only a marker changes the answer.
func TestAnAtomicInlineWithNoLineBoxAtAllStillFallsBack(t *testing.T) {
	// An empty inline-block with a height: its bottom edge is the baseline, so
	// it reaches exactly its own height above the line and nothing below it.
	ops := paintOf(t, `<div id="c">x<span id="e"></span></div>`,
		`#c { font-size: 20px; width: 300px }
		 #e { display: inline-block; width: 30px; height: 40px;
		      background: rgb(0,0,255) }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want the empty box: %v", len(got), got)
	}
	var text style.Unit
	for _, op := range ops {
		if v, ok := op.(DrawText); ok {
			text = v.At.Y
		}
	}
	if bottom := got[0].Y.Add(got[0].H); bottom != text {
		t.Errorf("the empty box ends at %v and the baseline beside it is at %v; a "+
			"box with no line box in it sits on the baseline by its bottom margin "+
			"edge", bottom, text)
	}
}

// TestAMarkerDoesNotOverrideARealLineBox: the marker is the *fallback*, taken
// only when the walk found no line box at all. An item with words in it is
// aligned by its words, and for an inline-block that means its last line — which
// is not the line the marker is on.
//
// The fixture puts text beside the box so that the line has a baseline of its
// own to pin. Without it the box is the only thing on the line, its own baseline
// decides where the line's is, and one line or three would put the marker in the
// same place.
func TestAMarkerDoesNotOverrideARealLineBox(t *testing.T) {
	at := func(inner string) (marker, beside, secondLine style.Unit) {
		var got []Point
		for _, op := range paintOf(t,
			`<div id="c">X<span id="w"><span id="s">`+inner+`</span></span></div>`,
			`#c { font-size: 16px; width: 400px }
			 #w { display: inline-block; width: 100px; list-style: square }
			 #s { display: list-item }`) {
			v, ok := op.(DrawText)
			if !ok {
				continue
			}
			switch v.Text {
			case "\u25aa":
				got = append(got, v.At)
			case "X":
				beside = v.At.Y
			case "b":
				secondLine = v.At.Y
			}
		}
		if len(got) != 1 {
			t.Fatalf("%d markers for %q", len(got), inner)
		}
		return got[0].Y, beside, secondLine
	}

	oneMarker, oneText, _ := at("a")
	if oneMarker != oneText {
		t.Errorf("a one-line item's marker is at %v and the text beside the box at "+
			"%v; the box's only line is its last, and the marker is on it",
			oneMarker, oneText)
	}

	twoMarker, twoText, twoSecond := at("a<br/>b")
	if twoMarker >= twoText {
		t.Errorf("a two-line item's marker is at %v and the text beside the box at "+
			"%v; the box sits on its *last* line, so the first line and the marker "+
			"on it are lifted above the text", twoMarker, twoText)
	}
	// And the lift is exactly the distance between the item's own two lines,
	// which is the same thing said as an equality rather than an inequality: the
	// second line is what the box sits on, the marker is on the first, and there
	// is nothing else between them.
	if lift, gap := twoText.Sub(twoMarker), twoSecond.Sub(twoMarker); lift != gap {
		t.Errorf("the marker is lifted %v above the text beside the box and the "+
			"item's two lines are %v apart; the box sits on the second of them",
			lift, gap)
	}
}
