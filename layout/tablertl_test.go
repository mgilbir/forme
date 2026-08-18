package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §17.2's direction on a table: which end the first column is at.
//
// "The 'direction' property on the table element determines the direction of the
// columns" — so in a right-to-left table the first column in the markup is the
// rightmost one, and the grid is read the other way. The engine laid the columns
// out left to right whatever the direction, so the first cell of every
// right-to-left table was at the wrong end of it.
//
// It is a mirror of the positions and nothing else. Which cell is in which
// column is a question about the markup; the direction says only where that
// column sits. The widths are unchanged, and every spacing keeps its place —
// the mirror of an evenly spaced row is an evenly spaced row.

// yellow is the table's own background, which is where the grid starts. Every
// position below is measured from it, so the body's margin is not in any of the
// numbers a case has to state.
var yellow = style.RGBA{R: 255, G: 255, A: 1}

// cellXs is the left edge of each cell background in the first row, relative to
// the table's own left edge, in the order the cells were written.
func cellXs(t *testing.T, css string) []style.Unit {
	t.Helper()
	ops := paintOf(t,
		`<div id="t"><div class="r">`+
			`<div class="c" id="one"></div><div class="c" id="two"></div>`+
			`<div class="c" id="three"></div></div></div>`,
		`#t { display: table; table-layout: fixed; width: 300px;
		      background: rgb(255,255,0); `+css+` }
		 .r { display: table-row }
		 .c { display: table-cell; height: 20px }
		 #one { background: rgb(0,0,255) }
		 #two { background: rgb(0,128,0) }
		 #three { background: rgb(255,0,0) }`)
	table := fillsOf(ops, yellow)
	if len(table) != 1 {
		t.Fatalf("%d table backgrounds: %v", len(table), table)
	}
	out := make([]style.Unit, 0, 3)
	for _, want := range []style.RGBA{blue, green, {R: 255, A: 1}} {
		got := fillsOf(ops, want)
		if len(got) != 1 {
			t.Fatalf("%d fills of %v: %v", len(got), want, got)
		}
		out = append(out, got[0].X.Sub(table[0].X))
	}
	return out
}

// TestTheFirstColumnOfARightToLeftTableIsTheRightmost is the rule.
func TestTheFirstColumnOfARightToLeftTableIsTheRightmost(t *testing.T) {
	ltr := cellXs(t, `direction: ltr`)
	if !(ltr[0] < ltr[1] && ltr[1] < ltr[2]) {
		t.Fatalf("a left-to-right table put its cells at %v, want them in order", ltr)
	}
	rtl := cellXs(t, `direction: rtl`)
	if !(rtl[0] > rtl[1] && rtl[1] > rtl[2]) {
		t.Errorf("a right-to-left table put its cells at %v; the first column is the "+
			"rightmost, so the three run the other way", rtl)
	}
}

// TestTheColumnsAreMirroredAndNotReordered. The grid is the same grid: each
// column keeps the width it was given and the row still spans the same band, so
// the two directions are reflections of one another.
func TestTheColumnsAreMirroredAndNotReordered(t *testing.T) {
	ltr := cellXs(t, `direction: ltr`)
	rtl := cellXs(t, `direction: rtl`)
	// Reflected about the middle of the 300px grid: a cell at x in one is at
	// 300 - x - width in the other, and every column here is 100 wide.
	for i := range ltr {
		if want := bgpx(300).Sub(ltr[i]).Sub(bgpx(100)); rtl[i] != want {
			t.Errorf("cell %d is at %v in a left-to-right table and %v in a "+
				"right-to-left one, want %v", i, ltr[i], rtl[i], want)
		}
	}
}

// TestTheSpacingKeepsItsPlaceUnderTheMirror is the case a fixture with no
// border-spacing cannot see: the spacing outside the grid has to stay outside it.
func TestTheSpacingKeepsItsPlaceUnderTheMirror(t *testing.T) {
	// 340px with four 10px gaps leaves 300 for three columns, which is 100 each
	// exactly. A width that did not divide evenly would leave the columns a
	// layout unit apart and the mirror would swap which one carried the odd
	// unit — a real difference of a 64th of a pixel, and not the one this is
	// about.
	const spacing = `width: 340px; border-collapse: separate; border-spacing: 10px`
	ltr := cellXs(t, `direction: ltr; `+spacing)
	rtl := cellXs(t, `direction: rtl; `+spacing)
	if ltr[0] != bgpx(10) {
		t.Fatalf("the first cell of a left-to-right table is at %v, want the spacing "+
			"at 10", ltr[0])
	}
	if want := bgpx(340).Sub(bgpx(10)).Sub(bgpx(100)); rtl[0] != want {
		t.Errorf("the first cell of a right-to-left table is at %v, want %v — one "+
			"spacing in from the right edge", rtl[0], want)
	}
	// And the gaps between the cells are the same on both sides.
	for i := 0; i+1 < len(ltr); i++ {
		if got := ltr[i+1].Sub(ltr[i]); got != rtl[i].Sub(rtl[i+1]) {
			t.Errorf("the step between cells %d and %d is %v one way and %v the "+
				"other", i, i+1, got, rtl[i].Sub(rtl[i+1]))
		}
	}
}

// TestAnInlineTableIsMirroredToo. The rule is the table's, not the block's, and
// an inline-table is a table — which is the second of the suite's two tests and
// the reason the direction is read from the table box rather than from whatever
// contains it.
func TestAnInlineTableIsMirroredToo(t *testing.T) {
	xs := func(display string) []style.Unit {
		ops := paintOf(t,
			`<div id="w"><div id="t"><div class="r">`+
				`<div class="c" id="one"></div><div class="c" id="two"></div></div></div></div>`,
			`#w { font-size: 0 }
			 #t { display: `+display+`; table-layout: fixed; width: 200px; direction: rtl }
			 .r { display: table-row }
			 .c { display: table-cell; height: 20px }
			 #one { background: rgb(0,0,255) }
			 #two { background: rgb(0,128,0) }`)
		var out []style.Unit
		for _, want := range []style.RGBA{blue, green} {
			got := fillsOf(ops, want)
			if len(got) != 1 {
				t.Fatalf("%s: %d fills of %v", display, len(got), want)
			}
			out = append(out, got[0].X)
		}
		return out
	}
	for _, display := range []string{"table", "inline-table"} {
		got := xs(display)
		if got[0] <= got[1] {
			t.Errorf("a right-to-left %s put its first cell at %v and its second at "+
				"%v; the first column is the rightmost", display, got[0], got[1])
		}
	}
}

// TestALeftToRightTableIsUnchanged is the containment case, and it is nearly
// every table: the mirror runs only when the table says so.
func TestALeftToRightTableIsUnchanged(t *testing.T) {
	plain := cellXs(t, ``)
	ltr := cellXs(t, `direction: ltr`)
	for i := range plain {
		if plain[i] != ltr[i] {
			t.Errorf("cell %d is at %v with no direction stated and %v with ltr",
				i, plain[i], ltr[i])
		}
	}
	if plain[0] != 0 {
		t.Errorf("the first cell of an ordinary table is at %v, want the left edge",
			plain[0])
	}
}
