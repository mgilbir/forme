package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An inside marker on a list item whose content is block-level.
//
// §12.5.1 makes the marker "the first inline box in the principal block box,
// before the element's content" — and an item whose content is block-level has
// no inline box for it to be the first of. The anonymous box rule settles it:
// the marker is inline content of the item's own, so it goes into the anonymous
// block that holds the item's leading inline content, and where there is none,
// into an anonymous block of its own before the first block child.
//
// The engine used to take such an item down the *inline* path on the strength
// of the marker alone. Every block child was then never laid out: a nested list
// inside a list item drew its marker and nothing else, and the whole of the
// nested list vanished from the page. list-style-position-023 is the suite's
// case, and its reference writes the answer out as "<div>1. <div>1. ...".

// markerLines is the text a document draws with the position of each run, which
// is what these need: the question is which line the marker is on.
type markerLine struct {
	text string
	x, y style.Unit
}

func markerLines(t *testing.T, htmlSrc string) []markerLine {
	t.Helper()
	const css = `ol { margin: 0; padding: 0 }
		li { margin: 0; padding: 0; list-style-position: inside }`
	var out []markerLine
	for _, op := range paintOf(t, htmlSrc, css) {
		if v, ok := op.(DrawText); ok {
			out = append(out, markerLine{v.Text, v.At.X, v.At.Y})
		}
	}
	return out
}

func TestABlockInsideAListItemIsStillLaidOut(t *testing.T) {
	got := markerLines(t, `<ol><li>A<ol><li>B</li></ol></li></ol>`)
	want := []string{"1.", "A", "1.", "B"}
	if len(got) != len(want) {
		t.Fatalf("the document drew %v, want %v — a nested list inside a list "+
			"item is content and has to be laid out", got, want)
	}
	for i, w := range want {
		if got[i].text != w {
			t.Fatalf("the document drew %v, want %v", got, want)
		}
	}
	// The marker and the item's own text are on one line: the anonymous block
	// around "A" is where §12.5.1's first inline box goes, so no line is added.
	if got[0].y != got[1].y {
		t.Errorf("the marker is at y=%v and \"A\" at y=%v; the marker is the "+
			"first inline box of the block that already holds the item's text, "+
			"not a line of its own", got[0].y, got[1].y)
	}
	if got[1].x <= got[0].x {
		t.Errorf("the marker is at x=%v and \"A\" at x=%v; the marker comes "+
			"before the element's content", got[0].x, got[1].x)
	}
	// And the nested item is on a line below.
	if got[2].y <= got[0].y {
		t.Errorf("the nested item's marker is at y=%v, not below the outer "+
			"item's line at y=%v", got[2].y, got[0].y)
	}
}

func TestAnItemThatBeginsWithABlockGetsALineForItsMarker(t *testing.T) {
	// Nothing of the item's own is inline, so the marker needs a block. The
	// reference for list-style-position-023 writes it as a text node before the
	// nested list, which is the same box by another route.
	got := markerLines(t, `<ol><li><ol><li>B</li></ol></li></ol>`)
	if len(got) != 3 || got[0].text != "1." || got[1].text != "1." || got[2].text != "B" {
		t.Fatalf("the document drew %v, want an outer marker, then the nested "+
			"item's marker and its text", got)
	}
	if got[1].y <= got[0].y {
		t.Errorf("the outer marker is at y=%v and the nested list begins at "+
			"y=%v; the marker is before the element's content and takes a line "+
			"of its own", got[0].y, got[1].y)
	}
	if got[0].x != got[1].x {
		t.Errorf("the outer marker is at x=%v and the nested one at x=%v; with "+
			"no indent on either list the two lines begin in the same place",
			got[0].x, got[1].x)
	}
}

func TestAnEmptyItemStillGetsItsMarker(t *testing.T) {
	// The case the inline path was there for, and it still is: an item with
	// nothing in it has no block-level content either, so it is its own first
	// inline box.
	got := markerLines(t, `<ol><li></li></ol>`)
	if len(got) != 1 || got[0].text != "1." {
		t.Errorf("an empty list item drew %v, want its marker: §12.5.1 gives it "+
			"a line box, which is what gives it a height and a background", got)
	}
}

func TestAnOutsideMarkerIsNotHoused(t *testing.T) {
	// The default position is unaffected: an outside marker is drawn beside the
	// item's border box and needs no line to be the first thing on.
	var out []string
	for _, op := range paintOf(t, `<ol><li><ol><li>B</li></ol></li></ol>`,
		`ol { margin: 0; padding: 0 } li { margin: 0; padding: 0 }`) {
		if v, ok := op.(DrawText); ok {
			out = append(out, v.Text)
		}
	}
	if len(out) != 3 || out[0] != "1." || out[1] != "1." || out[2] != "B" {
		t.Errorf("with outside markers the document drew %v, want both markers "+
			"and the text", out)
	}
}
