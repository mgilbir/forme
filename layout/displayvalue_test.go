package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The layout half of the invalid-display rule: what a dropped declaration means
// for the page.
//
// CSS2/abspos/static-fixed-inside-abspos writes "display: absolute" — the author
// meant "position" — on the div whose green background is the square the test is
// about. Read as the property's initial value the div is inline, it has no
// in-flow content of its own, so it has no line box and nothing of it is painted
// at all: the page is the red square underneath, which is exactly what the test
// says must not be there.

// TestAnInvalidDisplayStillGetsTheBlockBoxTheUAGaveIt is the bug, measured on
// the box rather than on the computed value.
func TestAnInvalidDisplayStillGetsTheBlockBoxTheUAGaveIt(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d" style="display:absolute; width:100px; height:100px">
  <span>x</span>
</div>`, "")
	d := find(t, root, "d")
	if got := d.BorderRect.W; got != picPx(100) {
		t.Errorf("the div's border box is %v wide, want 100px; an inline box takes "+
			"neither the width nor the height", got)
	}
	if got := d.BorderRect.H; got != picPx(100) {
		t.Errorf("the div's border box is %v tall, want 100px", got)
	}
}

// TestAnInvalidDisplayOnAnEmptyDivStillPaints is the reftest's own shape, and
// the reason the difference is visible at all: the div's only content is an
// absolutely positioned box, so as an inline it has nothing to give it a line
// and its background never reaches the page.
func TestAnInvalidDisplayOnAnEmptyDivStillPaints(t *testing.T) {
	ops := Paint(layoutOf(t, 600, `<div style="display:absolute; width:100px; height:100px; background:green">
  <div style="position:absolute; width:50px; height:50px"></div>
</div>`, ""))
	green := 0
	for _, op := range ops {
		if v, ok := op.(FillRect); ok && v.Color.G == 128 && v.Color.R == 0 {
			green++
			if v.Rect.W != picPx(100) || v.Rect.H != picPx(100) {
				t.Errorf("the green box is %v, want 100x100", v.Rect)
			}
		}
	}
	if green != 1 {
		t.Errorf("the page has %d green fills, want 1; the div with the invalid "+
			"display painted nothing at all", green)
	}
}

// TestAValidDisplayIsStillObeyed is the containment argument. Dropping a value
// that really is a display would put the user agent sheet back in charge of an
// element the author had restyled, and "display: inline" on a div is the
// cheapest way to see it happen.
func TestAValidDisplayIsStillObeyed(t *testing.T) {
	// An inline box has no fragment of its own — it is a run on a line — so the
	// question is put to the block around it. A block div takes the height; an
	// inline one leaves the wrapper one line tall.
	height := func(display string) style.Unit {
		root := layoutOf(t, 600, `<div id="w"><div style="display:`+display+
			`; width:100px; height:100px">x</div></div>`, "")
		return find(t, root, "w").BorderRect.H
	}
	if got := height("inline"); got >= picPx(100) {
		t.Errorf("a div with \"display: inline\" made its wrapper %v tall; a height "+
			"does not apply to a non-replaced inline box, and dropping the "+
			"declaration would have left it a block", got)
	}
	if got := height("absolute"); got != picPx(100) {
		t.Errorf("a div with the invalid display made its wrapper %v tall, want "+
			"100px — the fixture only says anything if the two differ", got)
	}
}

// TestDisplayNoneIsStillNone, because "none" is the value whose loss would be
// loudest: an element the author hid would come back.
func TestDisplayNoneIsStillNone(t *testing.T) {
	ops := Paint(layoutOf(t, 600, `<div style="display:none; width:100px; height:100px; background:green"></div>`, ""))
	for _, op := range ops {
		if v, ok := op.(FillRect); ok && v.Color.G == 128 && v.Color.R == 0 {
			t.Errorf("a hidden box painted %v", v.Rect)
		}
	}
}
