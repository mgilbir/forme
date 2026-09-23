package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Where an outside marker goes: on the inline-start side of the item's first
// formatted line, which is neither always the left nor always the item's own
// first line box.

// TestAnRTLItemsMarkerIsOnItsRight is audit C30.
//
// css-lists-3 places an outside marker before the first line in the inline
// direction, and §12.5.1 outside the principal box: in a right-to-left item that
// is past the right border edge, with the half-em gap between the marker and the
// text on its left. The left-to-right item is the same arrangement mirrored, and
// the test holds the two to that rather than to a number read off a run.
func TestAnRTLItemsMarkerIsOnItsRight(t *testing.T) {
	const item = `<div id="i" style="display:list-item; list-style:square; width:300px; ` +
		`padding:0 20px; border:0 solid; border-width:0 10px">word</div>`
	ltr, lf := itemMarker(t, `<div style="margin-left:100px">`+item+`</div>`)
	rtl, rf := itemMarker(t, `<div dir=rtl style="margin-left:100px">`+item+`</div>`)

	gap := markerGap(rf.Box.FontSize)
	w, _ := style.FromPx(rtl.Face.Measure(rtl.Text, rtl.Size.Px()))
	if want := rf.BorderRect.W.Add(gap); rtl.At.X != want {
		t.Errorf("the right-to-left marker is at %v; the item's border box is %v wide, "+
			"so the gap of %v past its right edge puts it at %v", rtl.At.X, rf.BorderRect.W, gap, want)
	}
	// The mirror of the left-to-right marker, whose far edge is a gap short of
	// the left border edge: here its near edge is a gap past the right one.
	if mirrored := lf.BorderRect.W.Sub(ltr.At.X).Sub(w); rtl.At.X != mirrored {
		t.Errorf("the right-to-left marker is at %v, the mirror of the left-to-right "+
			"one's %v is %v", rtl.At.X, ltr.At.X, mirrored)
	}
}

// TestAnRTLMarkerFollowsItsLinePastAFloat: the float on the right shortens the
// first line from its start, so the marker moves in with it — the mirror of
// TestAMarkerFollowsItsLinePastAFloat.
func TestAnRTLMarkerFollowsItsLinePastAFloat(t *testing.T) {
	for _, tc := range []struct{ what, content string }{
		{"with a line of its own", `word`},
		{"with no content of its own", ``},
		{"with its first line in a child", `<p style="margin:0">word</p>`},
	} {
		m, f := itemMarker(t, `<div dir=rtl style="width:500px">`+
			`<div style="float:right; width:1in; height:1in"></div>`+
			`<div id="i" style="display:list-item; list-style:square">`+tc.content+`</div></div>`)
		inch, _ := style.FromPx(96)
		if want := f.BorderRect.W.Sub(inch).Add(markerGap(f.Box.FontSize)); m.At.X != want {
			t.Errorf("%s: the marker is at %v; the float takes an inch off the right of "+
				"the first line, so it belongs a gap past %v, at %v",
				tc.what, m.At.X, f.BorderRect.W.Sub(inch), want)
		}
	}
}

// TestTheMarkerSitsOnTheFirstLineInABlockChild is audit C89.
//
// §5.12.1's first formatted line is found inside the first in-flow block-level
// child when the item's own content is block-level, which is how an item holding
// a paragraph or a heading is written. The marker's baseline is that line's.
func TestTheMarkerSitsOnTheFirstLineInABlockChild(t *testing.T) {
	for _, tc := range []struct{ what, content string }{
		{"a larger child", `<div id="c" style="font-size:40px">Big</div>`},
		{"a child with padding above", `<div id="c" style="padding-top:30px">text</div>`},
		{"a grandchild", `<div style="margin-top:0; border-top:7px solid"><p id="c" style="font-size:30px; margin:0">x</p></div>`},
	} {
		root := layoutOf(t, 600, `<div id="i" style="display:list-item; list-style:square">`+
			tc.content+`</div>`)
		item, child := find(t, root, "i"), find(t, root, "c")
		if item.Marker == nil || len(child.Lines) == 0 {
			t.Fatalf("%s: the fixture has no marker or no line", tc.what)
		}
		line := child.Lines[0]
		want := child.ContentRect().Y.Add(line.Rect.Y).Add(line.Baseline).Sub(item.BorderRect.Y)
		if item.Marker.At.Y != want {
			t.Errorf("%s: the marker's baseline is %v from the item's top, and the first "+
				"formatted line's is %v", tc.what, item.Marker.At.Y, want)
		}
	}
}

// TestAFirstLineBelowAnOutOfFlowChildIsStillTheFirstLine: a float or an
// absolutely positioned box ahead of the text is not the first formatted line,
// so the marker passes over it to the paragraph after it.
func TestAFirstLineBelowAnOutOfFlowChildIsStillTheFirstLine(t *testing.T) {
	root := layoutOf(t, 600, `<div id="i" style="display:list-item; list-style:square">`+
		`<div style="float:left; font-size:50px">F</div>`+
		`<div style="position:absolute; font-size:50px">A</div>`+
		`<p id="c" style="margin:0; padding-top:20px">text</p></div>`)
	item, child := find(t, root, "i"), find(t, root, "c")
	line := child.Lines[0]
	want := child.ContentRect().Y.Add(line.Rect.Y).Add(line.Baseline).Sub(item.BorderRect.Y)
	if item.Marker.At.Y != want {
		t.Errorf("the marker's baseline is %v and the paragraph's first line is at %v",
			item.Marker.At.Y, want)
	}
}

// TestAnEmptyItemsMarkerBaselineIsNotCountedTwice.
//
// A marker's At is measured from the item's border box, border and padding
// included. The two baseline searches that fall back to an empty item's marker —
// the first for a table cell or a flex item, the last for an inline-block —
// added the item's border and padding to it again, so an empty item with a
// padding-top was aligned that much below the line its marker is drawn on.
func TestAnEmptyItemsMarkerBaselineIsNotCountedTwice(t *testing.T) {
	root := layoutOf(t, 600, `<div id="i" style="display:list-item; list-style:square; `+
		`padding-top:30px; border-top:5px solid"></div>`)
	f := find(t, root, "i")
	if f.Marker == nil {
		t.Fatal("#i generated no marker")
	}
	if got, ok := firstBaseline(f); !ok || got != f.Marker.At.Y {
		t.Errorf("the first baseline is %v, the marker's own is %v", got, f.Marker.At.Y)
	}
	if got, ok := lastLineBaseline(f); !ok || got != f.Marker.At.Y {
		t.Errorf("the last baseline is %v, the marker's own is %v", got, f.Marker.At.Y)
	}
}

// TestAFloatThatEndsAboveTheFirstLineDoesNotMoveTheMarker: when the first
// formatted line is in a child, what moves the marker is the float beside *that
// line*, not one beside the item's top edge. Here the float is 20px tall and the
// child's line begins 40px down, clear of it, so the marker is where it would be
// with no float at all — in either direction.
func TestAFloatThatEndsAboveTheFirstLineDoesNotMoveTheMarker(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		side := "left"
		if dir == "rtl" {
			side = "right"
		}
		item := `<div id="i" style="display:list-item; list-style:square">` +
			`<p style="margin:0; padding-top:40px">word</p></div>`
		plain, _ := itemMarker(t, `<div dir=`+dir+` style="width:500px">`+item+`</div>`)
		floated, _ := itemMarker(t, `<div dir=`+dir+` style="width:500px">`+
			`<div style="float:`+side+`; width:1in; height:20px"></div>`+item+`</div>`)
		if floated.At.X != plain.At.X {
			t.Errorf("%s: a float that ends above the first line moved the marker from %v to %v",
				dir, plain.At.X, floated.At.X)
		}
	}
}
