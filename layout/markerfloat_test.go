package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Where an outside marker goes when a float is in the way.
//
// §12.5.1 leaves the marker box's position unspecified, and every renderer puts
// it before the item's first line box — which is the only answer that keeps a
// bullet next to the words it belongs to. The distinction from "the content
// edge" is invisible until something moves the line without moving the box, and
// a float is exactly that: a block's border box is not displaced by a float,
// only the lines inside it are shortened.

// itemMarker lays out a document and returns the marker of #i with its fragment.
func itemMarker(t *testing.T, doc string) (*Marker, *Fragment) {
	t.Helper()
	root := layoutOf(t, 600, doc, ``)
	f := find(t, root, "i")
	if f.Marker == nil {
		t.Fatal("#i generated no marker")
	}
	return f.Marker, f
}

const squareItem = `<div id="i" style="display:list-item; list-style:square">word</div>`

// TestAMarkerFollowsItsLinePastAFloat: the line starts an inch in, so the marker
// does too. Left at the content edge it sits *under* the float, an inch from the
// text it belongs to.
func TestAMarkerFollowsItsLinePastAFloat(t *testing.T) {
	m, f := itemMarker(t,
		`<div style="float:left; width:1in; height:1in"></div>`+squareItem)
	if len(f.Lines) == 0 {
		t.Fatal("the item has no line, so the fixture cannot show anything")
	}
	start := f.Lines[0].Rect.X
	if start == 0 {
		t.Fatal("the float did not shorten the line; the fixture is not the one described")
	}
	// Exactly its own width and a half-em gap before the line starts, and not
	// merely somewhere to the left of it: without the fix the marker sits at the
	// content edge, which is *also* to the left of a line a float has pushed an
	// inch in. "Before the line" is true of both and says nothing.
	gap := markerGap(f.Box.FontSize)
	w, _ := style.FromPx(m.Face.Measure(m.Text, m.Size.Px()))
	if want := start.Sub(w).Sub(gap); m.At.X != want {
		t.Errorf("the marker is at %v; the line starts at %v, so its own width %v "+
			"and the %v gap put it at %v", m.At.X, start, w, gap, want)
	}
}

// TestAMarkerWithNoFloatIsWhereItWas is the containment argument: everything
// that has no float must be untouched, so an ordinary list item's marker stays
// exactly where it has always been.
func TestAMarkerWithNoFloatIsWhereItWas(t *testing.T) {
	m, f := itemMarker(t, squareItem)
	if f.Lines[0].Rect.X != 0 {
		t.Fatalf("the first line starts at %v with no float in the document", f.Lines[0].Rect.X)
	}
	if m.At.X >= 0 {
		t.Errorf("the marker is at %v; an outside marker is in the margin, left of "+
			"the content box", m.At.X)
	}
}

// TestTextIndentDoesNotMoveTheMarker.
//
// text-indent moves the first line's *content* and not the line box, so a marker
// keyed on the line box is unaffected — which is the behaviour every renderer
// has, and would not be if the indent were part of what this reads.
func TestTextIndentDoesNotMoveTheMarker(t *testing.T) {
	plain, _ := itemMarker(t, squareItem)
	indented, _ := itemMarker(t,
		`<div id="i" style="display:list-item; list-style:square; text-indent:2em">word</div>`)
	if plain.At.X != indented.At.X {
		t.Errorf("text-indent moved the marker from %v to %v", plain.At.X, indented.At.X)
	}
}

// TestPaddingStillMovesTheMarker: the marker is placed from the *content* edge,
// so padding carries it along — which is the behaviour this change had to leave
// alone, and the one a fix keyed on the border box would have broken.
func TestPaddingStillMovesTheMarker(t *testing.T) {
	plain, _ := itemMarker(t, squareItem)
	padded, _ := itemMarker(t,
		`<div id="i" style="display:list-item; list-style:square; padding-left:20px">word</div>`)
	pad, _ := style.FromPx(20)
	if got := padded.At.X.Sub(plain.At.X); got != pad {
		t.Errorf("20px of padding moved the marker by %v, want %v", got, pad)
	}
}

// TestAMarkerSitsOnTheBaselineOfItsLine.
//
// The marker's y came from the strut, which is the right answer for a first line
// that *is* the strut and no answer at all about a line that is not. A line made
// taller by its own content — a larger span, an image — puts its baseline
// further down, and the bullet stayed level with a strut nothing was set in,
// floating above the text it belongs to.
//
// line-height is not the case that shows it: baselineOf already reads that, so
// the two agree there. It takes content the box's own font does not predict.
func TestAMarkerSitsOnTheBaselineOfItsLine(t *testing.T) {
	m, f := itemMarker(t,
		`<div id="i" style="display:list-item; list-style:square">`+
			`<span style="font-size:40px">W</span>ord</div>`)
	if len(f.Lines) == 0 {
		t.Fatal("the item has no line")
	}
	want := f.Lines[0].Rect.Y.Add(f.Lines[0].Baseline)
	if m.At.Y != want {
		t.Errorf("the marker's baseline is %v and its line's is %v", m.At.Y, want)
	}
	// And the line really is taller than the strut, or this asserts nothing.
	strut, _ := itemMarker(t, squareItem)
	if want == strut.At.Y {
		t.Fatalf("the big span did not move the line's baseline off the strut's %v", want)
	}
}

// TestAMarkerFollowsLineHeight is the case that already worked, kept so that a
// fix keyed on the line box cannot lose it: line-height moves the first line's
// baseline and the marker goes with it.
func TestAMarkerFollowsLineHeight(t *testing.T) {
	plain, _ := itemMarker(t, squareItem)
	tall, f := itemMarker(t,
		`<div id="i" style="display:list-item; list-style:square; line-height:3">word</div>`)
	if tall.At.Y == plain.At.Y {
		t.Errorf("line-height:3 left the marker at %v", tall.At.Y)
	}
	if want := f.Lines[0].Rect.Y.Add(f.Lines[0].Baseline); tall.At.Y != want {
		t.Errorf("the marker's baseline is %v and its line's is %v", tall.At.Y, want)
	}
}
