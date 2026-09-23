package layout

import (
	"strings"
	"testing"
)

// An outside marker's text is a paragraph of its own. css-lists-3 gives
// ::marker "unicode-bidi: isolate" in the user agent's sheet, and it inherits
// the list item's direction, so UAX #9 resolves "12." with the item's direction
// as its base: in a right-to-left item the stop is resolved to the paragraph's
// direction (W4 joins a separator only between two numbers) and goes to the
// left of the number. The marker was drawn as one run in logical order.

// markerTexts is every run drawn left of the item's own text, which in these
// fixtures is the marker and nothing else, in paint order.
func markerTexts(ops []Op, item string) []DrawText {
	var out []DrawText
	for _, op := range ops {
		if r, ok := op.(DrawText); ok && r.Text != item {
			out = append(out, r)
		}
	}
	return out
}

// TestAnOutsideMarkerIsBidiResolved: "12." in a right-to-left item is drawn as
// the stop, right to left, and then the number, left to right, with no gap
// between them, beginning where the marker in one piece began. A bullet is one
// right-to-left run.
func TestAnOutsideMarkerIsBidiResolved(t *testing.T) {
	const doc = `<ol><li value="12">x</li></ol>`
	const css = noDefaults + `ol { font-family: Courier; font-size: 20px }`

	ltr := markerTexts(paintOf(t, doc, css), "x")
	if len(ltr) != 1 || ltr[0].Text != "12." || ltr[0].RTL {
		t.Fatalf("control: a left-to-right marker is one run of \"12.\", got %v", ltr)
	}

	rtl := markerTexts(paintOf(t, doc, css+`ol { direction: rtl }`), "x")
	if len(rtl) != 2 {
		t.Fatalf("the right-to-left marker is %d runs, want the stop and the number: %v",
			len(rtl), rtl)
	}
	stop, number := rtl[0], rtl[1]
	if stop.Text != "." || number.Text != "12" {
		t.Fatalf("the runs are %q and %q, want \".\" then \"12\"", stop.Text, number.Text)
	}
	if !stop.RTL || number.RTL {
		t.Errorf("the stop is at an odd level and the number at an even one; got "+
			"RTL %v and %v", stop.RTL, number.RTL)
	}
	// Courier is 0.6em a character: the stop is 12px wide and the number 24.
	if got := number.At.X.Sub(stop.At.X).Px(); got != 12 {
		t.Errorf("the number starts %vpx after the stop, want the stop's 12", got)
	}
	whole := markerTexts(paintOf(t, doc, css+`ol { direction: rtl } li { list-style-type: disc }`), "x")
	if len(whole) != 1 || !whole[0].RTL {
		t.Errorf("a bullet in a right-to-left item is one right-to-left run, got %v", whole)
	}
	// The marker begins a gap past the item's right border edge, where the
	// marker in one piece began: the pieces are laid from there.
	root := layoutOf(t, A4.Content().W.Px(), doc, css+`ol { direction: rtl }`)
	li := firstListItem(t, root)
	want := li.BorderRect.Right().Add(li.Box.FontSize.Mul(0.5))
	if d := stop.At.X.Sub(want).Px(); d > 0.01 || d < -0.01 {
		t.Errorf("the marker begins at %v, want a half-em gap past the item's "+
			"right edge, at %v", stop.At.X, want)
	}
}

// firstListItem is the fragment of the first list item.
func firstListItem(t *testing.T, root *Fragment) *Fragment {
	t.Helper()
	var found *Fragment
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		if found != nil || f == nil {
			return
		}
		if f.Box != nil && f.Box.ListItem && f.Marker != nil {
			found = f
			return
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatal("no list item with a marker")
	}
	return found
}

// TestAnInsideMarkerIsIsolated: the inside marker is a box on the line, and
// its text is in an isolate of its own. In a right-to-left item whose text
// begins with a number, the stop of "1." sat between two European numbers,
// which rule W4 turns into one number — "1.2024", left to right, with the
// marker at its left end, the far end of the line from where a right-to-left
// item begins.
func TestAnInsideMarkerIsIsolated(t *testing.T) {
	root := layoutOf(t, 500, `<ol id="o"><li id="d">2024</li></ol>`,
		noDefaults+mono+`ol { direction: rtl } li { list-style-position: inside }`)
	var digit, year *TextRun
	for _, line := range find(t, root, "d").Lines {
		for i := range line.Runs {
			switch r := &line.Runs[i]; strings.TrimSpace(r.Text) {
			case "1", "1.":
				digit = r
			case "2024":
				year = r
			}
		}
	}
	if digit == nil || year == nil {
		t.Fatalf("the line has no marker digit or no year")
	}
	if !(digit.X > year.X) {
		t.Errorf("the marker's digit is at %v and the item's text at %v; the marker "+
			"begins a right-to-left line, at its right", digit.X, year.X)
	}
}
