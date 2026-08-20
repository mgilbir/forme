package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A float at the start of a line, and the marker that used to hide it.
//
// §9.5 puts a float that begins a line at the top of that line's box rather than
// beside it: there is nothing beside it yet. An absolutely positioned box takes
// no room and is not on the line in any sense a reader would recognise — but it
// is an *item*, and the walk that finds the floats beginning a line stopped at
// it, so a float written after one was taken for a float met part way along and
// placed beside content that was not there.
//
// The suite's css-text/text-indent/below-float and its neighbour are that
// exactly, and say so in their own assert: "Floats are not part of lines, so if
// a float is too wide to fit any inline content beside it, the first formatted
// line goes below it".

// floatBox is the container the tests below use: a hundred pixels square, with
// an indent wide enough that a full-width float leaves no room beside it.
const floatBox = `#c { position: relative; width: 100px; height: 100px;
	font-family: Courier; font-size: 50px; line-height: 50px; text-indent: 50px }`

const (
	wideFloat = `<div id="f" style="float:left; width:100px; height:50px"></div>`
	marker    = `<div id="a" style="position:absolute; top:50px; width:50px; height:50px"></div>`
)

// floatAndLine is where the float's border box and the first line ended up.
func floatAndLine(t *testing.T, markup string) (floatY, lineY style.Unit) {
	t.Helper()
	root := layoutOf(t, 600, `<div id="c">`+markup+`</div>`, noDefaults+floatBox)
	c := find(t, root, "c")
	if len(c.Lines) == 0 {
		t.Fatalf("%s: no lines", markup)
	}
	return find(t, root, "f").BorderRect.Y, c.Lines[0].Rect.Y
}

// TestAFloatAfterAnOutOfFlowBoxStillBeginsTheLine.
func TestAFloatAfterAnOutOfFlowBoxStillBeginsTheLine(t *testing.T) {
	wantFloat, wantLine := floatAndLine(t, wideFloat+"x")
	// The float begins the line, so it sits at the top of the block and the
	// line goes below it: nothing fits beside a float as wide as the block.
	if wantFloat != 0 {
		t.Fatalf("the float is at %v with nothing before it, want 0", wantFloat)
	}
	if wantLine != px2(50) {
		t.Fatalf("the line is at %v, want 50 — below the float", wantLine)
	}
	for _, tc := range []struct{ markup, what string }{
		{marker + wideFloat + "x", "one marker before it"},
		{marker + marker + wideFloat + "x", "two"},
		{`<span>` + marker + wideFloat + `x</span>`, "inside an inline box"},
	} {
		gotFloat, gotLine := floatAndLine(t, tc.markup)
		if gotFloat != wantFloat || gotLine != wantLine {
			t.Errorf("%s: the float is at %v and the line at %v, want %v and %v",
				tc.what, gotFloat, gotLine, wantFloat, wantLine)
		}
	}
}

// TestTheOutOfFlowBoxIsStillPlaced is the half that would be lost by passing
// over the marker instead of moving the float in front of it: the box has to
// keep its static position, which is the line it was written on.
func TestTheOutOfFlowBoxIsStillPlaced(t *testing.T) {
	root := layoutOf(t, 600, `<div id="c">`+marker+wideFloat+`x</div>`, noDefaults+floatBox)
	a := find(t, root, "a")
	// top: 50px is absolute; the left comes from where the box was written.
	if got := a.BorderRect.Y; got != px2(50) {
		t.Errorf("the box is at y=%v, want 50 — its own top", got)
	}
	if got := a.BorderRect.X; got != 0 {
		t.Errorf("the box is at x=%v, want 0 — the static position it was written at", got)
	}
	if got := a.BorderRect.W; got != px2(50) {
		t.Errorf("the box is %v wide, want 50", got)
	}
}

// TestAFloatThatFitsBesideTheContentIsUnaffected. The rule is about which floats
// *begin* a line, and a float that begins one is placed at the top of the line
// box whether or not anything fits beside it. This is the case where something
// does.
func TestAFloatThatFitsBesideTheContentIsUnaffected(t *testing.T) {
	const narrow = `<div id="f" style="float:left; width:25px; height:50px"></div>`
	root := layoutOf(t, 600, `<div id="c">`+marker+narrow+`x</div>`, noDefaults+floatBox)
	if got := find(t, root, "f").BorderRect.Y; got != 0 {
		t.Errorf("the float is at %v, want 0", got)
	}
	if got := find(t, root, "c").Lines[0].Rect.Y; got != 0 {
		t.Errorf("the line is at %v, want 0 — the float leaves room beside it", got)
	}
}

// TestTheOrderOfTheOutOfFlowItemsIsOtherwiseKept. The move is a stable partition
// of a run of items that all take no room, so two floats keep their order and so
// do two markers — which is what decides the order they are painted in.
func TestTheOrderOfTheOutOfFlowItemsIsOtherwiseKept(t *testing.T) {
	f1, f2 := &Box{}, &Box{}
	a1, a2 := &Box{}, &Box{}
	text := inlineItem{Text: "x"}
	for _, tc := range []struct {
		in, want []inlineItem
		what     string
	}{
		{[]inlineItem{{Abs: a1}, {Float: f1}}, []inlineItem{{Float: f1}, {Abs: a1}},
			"one of each"},
		{[]inlineItem{{Abs: a1}, {Abs: a2}, {Float: f1}},
			[]inlineItem{{Float: f1}, {Abs: a1}, {Abs: a2}}, "two markers, one float"},
		{[]inlineItem{{Abs: a1}, {Float: f1}, {Float: f2}},
			[]inlineItem{{Float: f1}, {Float: f2}, {Abs: a1}}, "one marker, two floats"},
		{[]inlineItem{{Abs: a1}, {Abs: a2}, {Float: f1}, {Float: f2}},
			[]inlineItem{{Float: f1}, {Float: f2}, {Abs: a1}, {Abs: a2}}, "two of each"},
		// Nothing to move.
		{[]inlineItem{{Float: f1}, {Abs: a1}}, []inlineItem{{Float: f1}, {Abs: a1}},
			"a marker after a float"},
		{[]inlineItem{{Abs: a1}, text}, []inlineItem{{Abs: a1}, text},
			"a marker before text"},
		{[]inlineItem{text, {Abs: a1}, text}, []inlineItem{text, {Abs: a1}, text},
			"a marker among text"},
		{[]inlineItem{{Abs: a1}, text, {Float: f1}},
			[]inlineItem{{Abs: a1}, text, {Float: f1}}, "text between them"},
		{nil, nil, "nothing at all"},
	} {
		got := floatsBeforeOutOfFlow(append([]inlineItem{}, tc.in...))
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d items, want %d", tc.what, len(got), len(tc.want))
			continue
		}
		for i := range got {
			if got[i].Float != tc.want[i].Float || got[i].Abs != tc.want[i].Abs ||
				got[i].Text != tc.want[i].Text {
				t.Errorf("%s: item %d is %+v, want %+v", tc.what, i, got[i], tc.want[i])
			}
		}
	}
}

// TestAnOrdinaryParagraphIsUntouched is the containment case: a document with no
// out-of-flow boxes in it must come out of the pass exactly as it went in, which
// is every paragraph of almost every document.
func TestAnOrdinaryParagraphIsUntouched(t *testing.T) {
	in := []inlineItem{{Text: "a"}, {Text: " "}, {Text: "b"}, {Float: &Box{}}, {Text: "c"}}
	want := append([]inlineItem{}, in...)
	got := floatsBeforeOutOfFlow(in)
	for i := range want {
		if got[i].Text != want[i].Text || got[i].Float != want[i].Float {
			t.Errorf("item %d moved: %+v, want %+v", i, got[i], want[i])
		}
	}
}
