package layout

import (
	"testing"
)

// TestAnItemEndingAtALineStartsItsSpanBeforeIt is the arithmetic of "span n /
// m".
//
// The item ends at line m and takes the span it asked for, so it begins n
// tracks earlier. It was read as though the span were always one — the branch's
// own comment says "one track wide" — so "span 2 / 4" started at line 3 instead
// of line 2, and one track further right for every track of span past the
// first.
func TestAnItemEndingAtALineStartsItsSpanBeforeIt(t *testing.T) {
	const doc = `<div id="g"><div id="i">x</div></div>`
	for _, tc := range []struct {
		name, place string
		wantX       float64
		wantW       float64
	}{
		{"span one", "span 1 / 4", 200, 100},
		{"span two", "span 2 / 4", 100, 200},
		{"span three", "span 3 / 4", 0, 300},
		{"an explicit start and end", "2 / 4", 100, 200},
		{"a start and a span", "2 / span 2", 100, 200},
	} {
		css := noDefaults + `#g { display: grid; width: 400px;
			grid-template-columns: 100px 100px 100px 100px }
			#i { grid-column: ` + tc.place + `; background: red }`
		got, ok := gridRect(t, doc, css, "i")
		if !ok {
			t.Errorf("%s: the item generated no fragment", tc.name)
			continue
		}
		if got.X.Px() != tc.wantX || got.W.Px() != tc.wantW {
			t.Errorf("%s (grid-column: %s): the item is at x=%.0f and %.0f wide, "+
				"want x=%.0f and %.0f", tc.name, tc.place, got.X.Px(), got.W.Px(),
				tc.wantX, tc.wantW)
		}
	}
}

// TestAutoFitCollapsesTheTracksNothingLandedIn is the question asked of the
// wrong thing.
//
// "auto-fit" collapses the tracks no item landed in, and that is a question
// only the placement can answer. It was asked of the item *count*, before
// anything had been placed — the same number only when every item takes one
// track. One item spanning two of four hundred-pixel tracks left one track
// standing and came out four hundred pixels wide.
func TestAutoFitCollapsesTheTracksNothingLandedIn(t *testing.T) {
	for _, tc := range []struct {
		name, doc, item string
		wantW           float64
	}{
		{"one item spanning two tracks",
			`<div id="g"><div id="i">x</div></div>`,
			`#i { grid-column: span 2; background: red }`, 200},
		{"one item spanning three",
			`<div id="g"><div id="i">x</div></div>`,
			`#i { grid-column: span 3; background: red }`, 300},
		{"one item in one track",
			`<div id="g"><div id="i">x</div></div>`,
			`#i { background: red }`, 100},
	} {
		css := noDefaults + `#g { display: grid; width: 400px;
			grid-template-columns: repeat(auto-fit, 100px) }` + tc.item
		got, ok := gridRect(t, tc.doc, css, "i")
		if !ok {
			t.Errorf("%s: the item generated no fragment", tc.name)
			continue
		}
		if got.W.Px() != tc.wantW {
			t.Errorf("%s: the item is %.0f wide, want %.0f", tc.name, got.W.Px(), tc.wantW)
		}
	}
}

// TestAutoFitStillLeavesNoRoomForTheTrackNothingIsIn is what auto-fit is for,
// and what separates it from auto-fill: three cards fill the row rather than
// leaving a gap where a fourth and fifth would go. It is the case that already
// worked, kept working, since collapsing by occupancy rather than by item count
// must give the same answer whenever every item takes one track.
func TestAutoFitStillLeavesNoRoomForTheTrackNothingIsIn(t *testing.T) {
	const three = `<div id="g"><div>a</div><div>b</div><div>c</div></div>`
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fit, minmax(60px, 1fr)) }`),
		[][4]float64{{0, 0, 100, 20}, {100, 0, 100, 20}, {200, 0, 100, 20}},
		"three items in five automatic tracks, the empty ones collapsed")

	// auto-fill keeps them, which is what says the collapse is doing the work.
	wantCells(t, gridCells(t, three,
		`#g { width: 300px; grid-template-columns: repeat(auto-fill, minmax(60px, 1fr)) }`),
		[][4]float64{{0, 0, 60, 20}, {60, 0, 60, 20}, {120, 0, 60, 20}},
		"the same three with the empty tracks standing")
}

// TestAutoFitLeavesATrackAnItemSpansInto is the case the item count could not
// see: the tracks *inside* an item's span are occupied by it, so none of them
// collapses, and an item that asked for two of five tracks gets two.
func TestAutoFitLeavesATrackAnItemSpansInto(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`
	css := noDefaults + `#g { display: grid; width: 500px;
		grid-template-columns: repeat(auto-fit, 100px) }
		#a { grid-column: span 2; background: red }
		#b { background: red }`
	a, ok := gridRect(t, doc, css, "a")
	if !ok {
		t.Fatal("the spanning item generated no fragment")
	}
	b, _ := gridRect(t, doc, css, "b")
	if a.W.Px() != 200 {
		t.Errorf("the item spanning two tracks is %.0f wide, want 200", a.W.Px())
	}
	if b.X.Px() != 200 {
		t.Errorf("the item after it starts at %.0f, want 200", b.X.Px())
	}
}
