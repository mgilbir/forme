package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// visibility, CSS 2.1 §11.2.
//
// "nothing was painted" is true for a great many wrong reasons — a box that was
// never laid out, a colour that did not parse, a document that produced no boxes
// at all — so none of these counts marks. Each one names the specific op it
// expects to be absent and, in the same document, a specific op it expects to
// still be there.

// filled reports whether a rectangle of exactly this colour was painted.
func filled(ops []Op, want style.RGBA) int {
	n := 0
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && r.Color == want && !r.Rect.Empty() {
			n++
		}
	}
	return n
}

// drew reports how many runs of text were drawn.
func drew(ops []Op, text string) int {
	n := 0
	for _, op := range ops {
		if t, ok := op.(DrawText); ok && t.Text == text {
			n++
		}
	}
	return n
}

var (
	red   = style.RGBA{R: 255, A: 1}
	green = style.RGBA{G: 128, A: 1}
)

const visCSS = `#h { background-color: #ff0000 } #v { background-color: #008000 }`

func TestHiddenBoxPaintsNothingOfItsOwn(t *testing.T) {
	// The hidden box has a background and a border and text, and none of the
	// three reaches the page. The visible sibling is in the same document and
	// must, or the test would pass on a document that laid nothing out at all.
	ops := paintOf(t,
		`<div id="h">gone</div><div id="v">here</div>`,
		noDefaults+visCSS+` #h { visibility: hidden; height: 10px;
		   border-top-style: solid; border-top-width: 4px; border-top-color: #0000ff }`)

	if n := filled(ops, red); n != 0 {
		t.Errorf("a hidden box painted its background %d times", n)
	}
	if n := filled(ops, style.RGBA{B: 255, A: 1}); n != 0 {
		t.Errorf("a hidden box painted its border %d times", n)
	}
	if n := drew(ops, "gone"); n != 0 {
		t.Errorf("a hidden box drew its text %d times", n)
	}
	if n := filled(ops, green); n != 1 {
		t.Errorf("the visible sibling painted its background %d times, want 1", n)
	}
	if n := drew(ops, "here"); n != 1 {
		t.Errorf("the visible sibling drew its text %d times, want 1", n)
	}
}

func TestHiddenBoxStillOccupiesItsSpace(t *testing.T) {
	// The difference between "visibility: hidden" and "display: none", asserted
	// as a position rather than as a presence. The hidden box is 40px tall, so
	// its sibling starts at 40 — at 0 it would mean the box had been pruned.
	root := layoutOf(t, 600,
		`<div id="h">x</div><div id="v">y</div>`,
		noDefaults+` #h { visibility: hidden; height: 40px }`)
	body := find(t, root, "v")
	px(t, "the box after a hidden one", body.BorderRect.Y, 40)
}

func TestVisibleDescendantReappearsInsideAHiddenAncestor(t *testing.T) {
	// visibility inherits, and a descendant may set it back. This is why the
	// property cannot be implemented by pruning the box tree: the subtree is
	// hidden and one box inside it is not.
	ops := paintOf(t,
		`<div id="h"><div id="v">here</div></div>`,
		noDefaults+visCSS+` #h { visibility: hidden; height: 30px }
		 #v { visibility: visible; height: 10px }`)

	if n := filled(ops, red); n != 0 {
		t.Errorf("the hidden ancestor painted its background %d times", n)
	}
	if n := filled(ops, green); n != 1 {
		t.Errorf("the visible descendant painted its background %d times, want 1", n)
	}
	if n := drew(ops, "here"); n != 1 {
		t.Errorf("the visible descendant drew its text %d times, want 1", n)
	}
}

func TestVisibilityIsAskedPerRunWithinALine(t *testing.T) {
	// A line box holds runs from several inline boxes, each with its own
	// visibility. A version that asked the block once would hide the whole line
	// or none of it.
	ops := paintOf(t,
		`<div id="p">shown<span id="s">gone</span></div>`,
		noDefaults+` #s { visibility: hidden }`)

	if n := drew(ops, "shown"); n != 1 {
		t.Errorf("the visible run was drawn %d times, want 1", n)
	}
	if n := drew(ops, "gone"); n != 0 {
		t.Errorf("the hidden run inside the same line was drawn %d times", n)
	}
}

func TestHiddenListItemDrawsNoMarker(t *testing.T) {
	// The marker is generated rather than written, so it is painted by a
	// different path from the text and needs its own answer.
	ops := paintOf(t, `<ul><li id="i">one</li></ul>`,
		noDefaults+` #i { visibility: hidden }`)
	for _, op := range ops {
		if t2, ok := op.(DrawText); ok && strings.Contains(t2.Text, "•") {
			t.Fatalf("a hidden list item drew its bullet")
		}
	}
}

// visibility: collapse on a table row or column, CSS 2.1 §17.5.5.
//
// "If a table row or column is collapsed, the row or column is not rendered, and
// the table is laid out as if the row or column did not exist." It is the one
// place the value means something other than "hidden", and the difference is a
// layout effect: a hidden row is invisible and still holds its height open, a
// collapsed one is gone and the table closes up.
//
// This engine drew it as "hidden" and reported the difference. A table with a
// gap where a row used to be looks deliberate, which is why it was reported —
// and is why it is worth doing rather than reporting.

// collapseTable is a two-row, three-column table with 50px cells.
const collapseTable = `<table id="t"><col id="c1"><col id="c2"><col id="c3">` +
	`<tr id="r1"><td>a</td><td>b</td><td>c</td></tr>` +
	`<tr id="r2"><td>d</td><td>e</td><td>f</td></tr></table>`

const collapseCSS = `table { border-spacing: 0; font-family: Courier; font-size: 20px }
	td { padding: 0; width: 50px; height: 40px }`

// TestACollapsedTrackIsTakenOutAndTheTableClosesUp.
func TestACollapsedTrackIsTakenOutAndTheTableClosesUp(t *testing.T) {
	for _, tc := range []struct {
		css  string
		w, h float64
		what string
	}{
		{``, 150, 80, "nothing collapsed"},
		{`#r1 { visibility: collapse }`, 150, 40, "a row"},
		{`#r1, #r2 { visibility: collapse }`, 150, 0, "every row"},
		{`#c2 { visibility: collapse }`, 100, 80, "a column"},
		{`#c1, #c3 { visibility: collapse }`, 50, 80, "two columns"},
		{`#r1 { visibility: collapse } #c2 { visibility: collapse }`, 100, 40, "one of each"},
		// The border-spacing that would have followed the track goes with it, or
		// a table would keep a gap for every row it does not draw. Ten pixels
		// of spacing over three columns and two rows is 190 by 110.
		{`table { border-spacing: 10px }`, 190, 110, "spacing, nothing collapsed"},
		{`table { border-spacing: 10px } #r1 { visibility: collapse }`, 190, 60,
			"spacing, a row: forty of height and one gap of ten"},
		{`table { border-spacing: 10px } #c2 { visibility: collapse }`, 130, 110,
			"spacing, a column: fifty of width and one gap of ten"},
		// hidden is the value this used to be treated as, and it holds the
		// space open — which is the whole difference and is what says the rows
		// above are measuring the right thing.
		{`#r1 { visibility: hidden }`, 150, 80, "hidden rather than collapsed"},
		{`#c2 { visibility: hidden }`, 150, 80, "a hidden column"},
	} {
		root := layoutOf(t, 600, collapseTable, noDefaults+collapseCSS+tc.css)
		f := find(t, root, "t")
		if f.BorderRect.W.Px() != tc.w || f.BorderRect.H.Px() != tc.h {
			t.Errorf("%s: the table is %gx%g, want %gx%g", tc.what,
				f.BorderRect.W.Px(), f.BorderRect.H.Px(), tc.w, tc.h)
		}
	}
}

// TestACollapsedColumnUnderTheFixedAlgorithm. The fixed algorithm takes a
// column's width from the <col> or from the cell in the first row rather than
// from what the cells demand, so it is a separate path and needs its own
// fixture. The table keeps the width it was given and the collapsed column's
// share goes to the columns that are left.
func TestACollapsedColumnUnderTheFixedAlgorithm(t *testing.T) {
	widths := func(css string) []float64 {
		t.Helper()
		root := layoutOf(t, 600, collapseTable, noDefaults+collapseCSS+
			`table { width: 300px; table-layout: fixed } `+css)
		var out []float64
		var walk func(*Fragment)
		walk = func(f *Fragment) {
			if f.Box != nil && f.Box.Inner == InnerTableCell {
				out = append(out, f.BorderRect.W.Px())
			}
			for _, c := range f.Children {
				walk(c)
			}
		}
		walk(find(t, root, "t"))
		return out
	}
	// Three columns of a 300px table are 100 each; with the middle one
	// collapsed there are two cells a row at 150.
	if got := widths(`#c2 { visibility: collapse }`); len(got) != 4 {
		t.Errorf("the table drew %d cells, want four — two rows of two", len(got))
	} else {
		for _, w := range got {
			if w != 150 {
				t.Errorf("a cell is %gpx, want 150 — the collapsed column's share "+
					"goes to the columns that are left; all: %v", w, got)
				break
			}
		}
	}
	if got := widths(``); len(got) != 6 {
		t.Errorf("with nothing collapsed the table drew %d cells, want six", len(got))
	}
}

// TestWhatFollowsACollapsedColumnMovesUpToIt, spacing and all.
//
// The table being the right *width* is not the same as the columns being in the
// right *places*: a version that took the collapsed column's width away and left
// the gap that followed it makes a table of exactly the right size with every
// column after the hole ten pixels too far along.
func TestWhatFollowsACollapsedColumnMovesUpToIt(t *testing.T) {
	xs := func(css string) []float64 {
		t.Helper()
		root := layoutOf(t, 600, collapseTable, noDefaults+collapseCSS+
			`table { border-spacing: 10px } `+css)
		var out []float64
		var walk func(*Fragment, style.Unit)
		walk = func(f *Fragment, off style.Unit) {
			if f.Box != nil && f.Box.Inner == InnerTableCell {
				out = append(out, f.BorderRect.X.Add(off).Px())
			}
			for _, c := range f.Children {
				walk(c, off.Add(f.BorderRect.X))
			}
		}
		walk(find(t, root, "t"), 0)
		return out
	}
	// Ten of spacing, fifty of column, ten, fifty, ten, fifty, ten. The cells of
	// the two rows are at 20, 80 and 140 counting the table's own edge.
	if got := xs(``); len(got) != 6 || got[0] != 20 || got[1] != 80 || got[2] != 140 {
		t.Fatalf("with nothing collapsed the cells are at %v, want 20, 80 and 140 twice", got)
	}
	// With the middle column gone the third moves up into its place *and* into
	// the gap that followed it: 20 and 80, not 20 and 90.
	if got := xs(`#c2 { visibility: collapse }`); len(got) != 4 ||
		got[0] != 20 || got[1] != 80 {
		t.Errorf("the cells are at %v, want 20 and 80 twice", got)
	}
}

// TestNothingInsideACollapsedTrackIsDrawn, not even a descendant that turns
// visibility back on.
//
// The property inherits and a child may set "visible" and reappear inside a
// hidden ancestor — that is the whole reason visibility is asked per box at
// painting time. §17.5.5 is not about visibility once the row is collapsed: the
// row "is not rendered", and a box that is not rendered has no inside for a
// child to reappear in.
//
// It matters more since the track started closing up. A collapsed row's cells
// sit where the row after it begins, so a child that painted anyway would land
// on top of the following row's text rather than in a space of its own.
func TestNothingInsideACollapsedTrackIsDrawn(t *testing.T) {
	drawn := func(css string) []string {
		t.Helper()
		ops := paintOf(t, `<table id="t"><tr id="r1"><td><span class=v>X</span></td></tr>`+
			`<tr><td>y</td></tr></table>`,
			noDefaults+`table { border-spacing: 0; font-family: Courier; font-size: 20px }
			 td { padding: 0 } .v { visibility: visible } `+css)
		var out []string
		for _, op := range ops {
			if d, ok := op.(DrawText); ok {
				out = append(out, d.Text)
			}
		}
		return out
	}
	if got := drawn(`#r1 { visibility: collapse }`); len(got) != 1 || got[0] != "y" {
		t.Errorf("a collapsed row drew %v, want only the row below it", got)
	}
	// hidden is the other value, and a visible child inside one *does* paint —
	// which is what says the row above is the collapse rule rather than the
	// visibility check going wrong.
	if got := drawn(`#r1 { visibility: hidden }`); len(got) != 2 {
		t.Errorf("a hidden row with a visible child drew %v, want both", got)
	}
	if got := drawn(``); len(got) != 2 {
		t.Errorf("an ordinary table drew %v, want both", got)
	}
}

// TestACollapsedRowLeavesNoBandBehindIt. A row group paints a background over
// the rows it holds, and a collapsed row is not one of them: the group is as
// tall as what it renders, not as tall as what it was written with.
func TestACollapsedRowLeavesNoBandBehindIt(t *testing.T) {
	bands := func(css string) []Rect {
		t.Helper()
		ops := paintOf(t, `<table id="t"><tbody style="background:rgb(0,128,0)">`+
			`<tr id="r1"><td>a</td></tr><tr id="r2"><td>b</td></tr></tbody></table>`,
			noDefaults+`table { border-spacing: 0; font-family: Courier; font-size: 20px }
			 td { padding: 0; height: 40px } `+css)
		return fillsOf(ops, green)
	}
	if got := bands(`#r2 { visibility: collapse }`); len(got) != 1 {
		t.Errorf("the group painted %v; the collapsed row is not one of its rows", got)
	}
	if got := bands(``); len(got) != 2 {
		t.Errorf("with nothing collapsed the group painted %v, want a band a row", got)
	}
}

// TestACellSpanningIntoACollapsedTrackIsReported is the half of §17.5.5 that is
// not done: such a cell keeps its span, and "the contents of spanned cells that
// intersect with a collapsed column are clipped". Nothing clips them, so the
// text comes out whole in a narrower space.
//
// A cell *wholly* inside a collapsed track is not this case and is not reported:
// the track is not rendered and neither is anything in it, so there is nothing a
// reader could see that should have been cut.
func TestACellSpanningIntoACollapsedTrackIsReported(t *testing.T) {
	count := func(html, css string) int {
		t.Helper()
		rec := NewRecorder(nil)
		built := Build(Input{HTML: html, CSS: []Stylesheet{{Source: noDefaults + collapseCSS + css}}})
		Layout(built.Root, Size{W: picPx(600), H: picPx(10000)}, nil, rec)
		n := 0
		for _, f := range rec.Findings() {
			if f.Property == "visibility" {
				n++
			}
		}
		return n
	}
	spanning := `<table id="t"><col id="c1"><col id="c2">` +
		`<tr><td colspan="2">ab</td></tr><tr><td>c</td><td>d</td></tr></table>`
	if got := count(spanning, `#c2 { visibility: collapse }`); got != 1 {
		t.Errorf("a cell spanning into a collapsed column was reported %d times, want once", got)
	}
	// Wholly inside one: nothing to clip and nothing to say.
	if got := count(collapseTable, `#r1 { visibility: collapse }`); got != 0 {
		t.Errorf("a cell inside a collapsed row was reported %d times, want none", got)
	}
	if got := count(collapseTable, `#c2 { visibility: collapse }`); got != 0 {
		t.Errorf("a cell inside a collapsed column was reported %d times, want none", got)
	}
	// And a table with nothing collapsed says nothing at all.
	if got := count(collapseTable, ``); got != 0 {
		t.Errorf("an ordinary table was reported %d times", got)
	}
}
