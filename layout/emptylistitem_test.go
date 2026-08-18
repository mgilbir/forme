package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A list item with no content of its own, and where its marker goes.
//
// It is not a rare shape. It is how the CSS Working Group writes "does this
// property apply to a list item" — an empty element with display: list-item and
// nothing in it — and nine of the suite's reftests are that sentence.
//
// §12.5.1 leaves the marker box's position unspecified and every renderer puts
// it before the first line box. An empty item has no line box, and the answer is
// not to give up and use the content edge: what a browser does is put the bullet
// where a line *would* have started, which is not the content edge whenever a
// float is in the way. A block's border box is not displaced by a float, only
// the lines inside it are.

// markerX finds the x of the marker one list item generated.
func markerX(t *testing.T, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	var found *Marker
	var walk func(*Fragment)
	root := layoutOf(t, 600, htmlSrc, cssSrc)
	walk = func(f *Fragment) {
		if found == nil && f.Marker != nil {
			found = f.Marker
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no marker was generated")
	}
	return found.At.X
}

// TestAnEmptyListItemPutsItsMarkerBesideAFloat is the bug.
//
// The float is an inch wide and the item beside it is empty. Its marker belongs
// where the item's first line would have begun — past the float — and it was
// going to the content edge instead, an inch to the left and out past the page's
// own margin.
func TestAnEmptyListItemPutsItsMarkerBesideAFloat(t *testing.T) {
	const css = `#f { float: left; width: 96px; height: 96px }
	             #i { display: list-item; list-style: square }`
	empty := markerX(t, `<div id="f"></div><div id="i"></div>`, css)

	// The same item with a line in it, which has always been placed from the
	// line box. The two must agree: whether an item has content cannot move its
	// bullet, and this is the equality the fix is really about.
	full := markerX(t, `<div id="f"></div><div id="i">x</div>`, css)
	if empty != full {
		t.Errorf("an empty item's marker is at %v and a full one's at %v; the "+
			"content of an item does not move its bullet", empty, full)
	}
	// And it really is past the float, so the test is not passing because both
	// went to the same wrong place.
	if empty <= 0 {
		t.Errorf("the marker is at %v, which is at or before the content edge; "+
			"the float is 96px wide and the line begins after it", empty)
	}
}

// TestAnEmptyListItemWithNoFloatIsUnmoved is the containment case: the new
// question is asked of every empty item, and must answer nothing at all when
// there is no float to answer about.
func TestAnEmptyListItemWithNoFloatIsUnmoved(t *testing.T) {
	const css = `#i { display: list-item; list-style: square }`
	empty := markerX(t, `<div id="i"></div>`, css)
	full := markerX(t, `<div id="i">x</div>`, css)
	if empty != full {
		t.Errorf("with no float, an empty item's marker is at %v and a full one's "+
			"at %v", empty, full)
	}
	if empty >= 0 {
		t.Errorf("the marker is at %v; an outside marker sits in the margin, "+
			"before the content edge", empty)
	}
}

// TestAMarkerIsPlacedFromTheItemsOwnEdges. The band is asked between the item's
// content edges rather than the containing block's, so an item indented past a
// float by its own margin is not moved twice.
func TestAMarkerIsPlacedFromTheItemsOwnEdges(t *testing.T) {
	// The float is 96 wide; the item's own margin already clears it, so there is
	// nothing left for the float to do and the marker sits at the item's edge.
	x := markerX(t,
		`<div id="f"></div><div id="i"></div>`,
		`#f { float: left; width: 96px; height: 96px }
		 #i { display: list-item; list-style: square; margin-left: 200px }`)
	plain := markerX(t, `<div id="i"></div>`,
		`#i { display: list-item; list-style: square; margin-left: 200px }`)
	if x != plain {
		t.Errorf("an item whose margin already clears the float has its marker at "+
			"%v, and the same item with no float has it at %v; the float is not "+
			"in the way and must move nothing", x, plain)
	}
}
