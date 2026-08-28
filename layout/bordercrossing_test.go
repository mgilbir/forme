package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Where two collapsed grid lines cross, CSS 2.1 §17.6.2.1.
//
// The square where a vertical line meets a horizontal one is covered by both,
// and four borders have a claim on it: the two halves of the horizontal line
// either side of the crossing and the two halves of the vertical line above and
// below it. §17.6.2.1 decides between them like any other conflict — the widest
// first, and among equals "the one further to the left and further to the top".
//
// The geometry below is arithmetic and not a recorded number. Every cell is
// 20px of content inside a 10px border, so the grid lines fall at 0..10, 30..40
// and 60..70 on both axes and the middle crossing is the square (30,30 10x10).
// Its centre is (35, 35), which is the point every assertion here samples.

// crossingCSS is a two-by-two collapsing table with a colour per cell.
const crossingCSS = collapsing + `
td { border: 10px solid; width: 20px; height: 20px }
#a { border-color: #ff0000 }
#b { border-color: #00ff00 }
#c { border-color: #0000ff }
#d { border-color: #ffff00 }
`

const crossingHTML = `<table id=t>
  <tr><td id=a></td><td id=b></td></tr>
  <tr><td id=c></td><td id=d></td></tr>
</table>`

// lastFillAt is the colour left on a point once everything has been painted,
// which for two bands that overlap is the one drawn second.
//
// It reads the display list rather than a rasterisation for the reason the rest
// of this package does: the question is which band the engine put last, and a
// picture would answer it only by agreeing with itself.
func lastFillAt(t *testing.T, ops []Op, x, y float64) (style.RGBA, bool) {
	t.Helper()
	var got style.RGBA
	found := false
	for _, op := range ops {
		f, ok := op.(FillRect)
		if !ok || f.Rect.Empty() {
			continue
		}
		if f.Rect.X.Px() <= x && x < f.Rect.X.Add(f.Rect.W).Px() &&
			f.Rect.Y.Px() <= y && y < f.Rect.Y.Add(f.Rect.H).Px() {
			got, found = f.Color, true
		}
	}
	return got, found
}

var (
	crossRed    = style.RGBA{R: 255, A: 1}
	crossGreen  = style.RGBA{G: 255, A: 1}
	crossBlue   = style.RGBA{B: 255, A: 1}
	crossYellow = style.RGBA{R: 255, G: 255, A: 1}
)

// TestACrossingGoesToTheBorderAboveAndLeftOfIt is the rule.
//
// All four borders meeting in the middle are solid and ten pixels wide, so the
// widest rule and the style rule both pass without deciding anything and only
// the element order is left. The cell above and to the left of the crossing is
// #a, and its red is the answer.
//
// The three other colours are what the three wrong answers look like, which is
// why each cell has its own: blue is the cell below, green the cell to the
// right, yellow the cell diagonally across. Before the runs of a line were drawn
// back to front this came out blue.
func TestACrossingGoesToTheBorderAboveAndLeftOfIt(t *testing.T) {
	ops := paintOf(t, crossingHTML, crossingCSS)
	got, ok := lastFillAt(t, ops, 35, 35)
	if !ok {
		t.Fatal("nothing was painted where the two grid lines cross")
	}
	if got != crossRed {
		what := map[style.RGBA]string{
			crossBlue:   "the cell below it",
			crossGreen:  "the cell to its right",
			crossYellow: "the cell diagonally across from it",
		}[got]
		if what == "" {
			what = "something that is none of the four cells"
		}
		t.Errorf("the crossing was painted %v — %s. §17.6.2.1 gives it to the "+
			"one further to the left and further to the top, which is #a's red", got, what)
	}
}

// TestAWiderBorderOwnsTheCrossingWhereverItIs holds the order of the two rules.
//
// §17.6.2.1 asks the width before it asks the element, so a wider border owns a
// crossing from below and from the right as readily as from above. #d is the
// cell the rule above hands nothing to, and twenty pixels of it beat ten of #a.
//
// Without this the fix for the rule above would be free to be "the top left
// always wins", which is the rule with its first clause missing.
func TestAWiderBorderOwnsTheCrossingWhereverItIs(t *testing.T) {
	ops := paintOf(t, crossingHTML, crossingCSS+`#d { border-width: 20px }`)
	// The middle crossing is now twenty pixels square — the line is as wide as
	// the widest border on it — and runs from 30 to 50 on both axes.
	got, ok := lastFillAt(t, ops, 40, 40)
	if !ok {
		t.Fatal("nothing was painted where the two grid lines cross")
	}
	if got != crossYellow {
		t.Errorf("the crossing was painted %v, want #d's yellow: it is twice as "+
			"wide as everything else meeting there, and width is asked before "+
			"position", got)
	}
}

// TestACrossingBetweenTwoHorizontalRunsGoesToTheLeftOne isolates one half of
// the rule, and is built so that the other half cannot answer it.
//
// The two tests above are decided by a vertical run, because a vertical run
// covers a crossing whichever way the horizontal ones are ordered — so an engine
// that reversed only its columns would pass both. Here the middle crossing has
// no vertical band on it at all: only the third row's cells have side borders,
// so the column line has a width but nothing drawn on it where the first two
// rows meet. What is left is the horizontal run ending at the crossing against
// the one beginning there, and the rule says the first of them wins.
//
// The third row is black so that a band from it showing up where it should not
// is a colour the assertion has never heard of rather than a plausible answer.
func TestACrossingBetweenTwoHorizontalRunsGoesToTheLeftOne(t *testing.T) {
	ops := paintOf(t,
		`<table id=t>
		   <tr><td id=a></td><td id=b></td></tr>
		   <tr><td id=c></td><td id=d></td></tr>
		   <tr><td id=e></td><td id=f></td></tr>
		 </table>`,
		collapsing+`
		 td { border-top: 10px solid; border-bottom: 10px solid;
		      width: 20px; height: 20px }
		 #e, #f { border-left: 10px solid; border-right: 10px solid }
		 #a { border-color: #ff0000 } #b { border-color: #00ff00 }
		 #c { border-color: #0000ff } #d { border-color: #ffff00 }
		 #e, #f { border-color: #000000 }`)
	got, ok := lastFillAt(t, ops, 35, 35)
	if !ok {
		t.Fatal("nothing was painted where the two grid lines cross")
	}
	if got != crossRed {
		t.Errorf("the crossing was painted %v, want #a's red: the horizontal run "+
			"that ends there is #a's bottom border and the one that begins there "+
			"is #b's, and §17.6.2.1 gives it to the one further to the left", got)
	}
}

// TestACrossingBetweenTwoVerticalRunsGoesToTheUpperOne is the other half, the
// same document turned on its side.
//
// Only the third *column* has top and bottom borders, so the row line the first
// two columns share has a width and no band on it, and the crossing is decided
// by the vertical run ending there against the one beginning there.
func TestACrossingBetweenTwoVerticalRunsGoesToTheUpperOne(t *testing.T) {
	ops := paintOf(t,
		`<table id=t>
		   <tr><td id=a></td><td id=c></td><td id=e></td></tr>
		   <tr><td id=b></td><td id=d></td><td id=f></td></tr>
		 </table>`,
		collapsing+`
		 td { border-left: 10px solid; border-right: 10px solid;
		      width: 20px; height: 20px }
		 #e, #f { border-top: 10px solid; border-bottom: 10px solid }
		 #a { border-color: #ff0000 } #b { border-color: #00ff00 }
		 #c { border-color: #0000ff } #d { border-color: #ffff00 }
		 #e, #f { border-color: #000000 }`)
	got, ok := lastFillAt(t, ops, 35, 35)
	if !ok {
		t.Fatal("nothing was painted where the two grid lines cross")
	}
	if got != crossRed {
		t.Errorf("the crossing was painted %v, want #a's red: the vertical run "+
			"that ends there is #a's left border and the one that begins there is "+
			"#b's, and §17.6.2.1 gives it to the one further to the top", got)
	}
}

// There is no test here for "no crossing is left blank", and the reason is worth
// writing down rather than leaving as an omission. A crossing is covered twice
// over — by the runs of both lines through it — so no single change to either
// extension can empty one, and a test asserting it would pass under every defect
// that could be planted against it. What the coverage is for is a line whose
// *other* line has a width and no band on it, which is exactly the pair of
// fixtures above: each of them samples a crossing one of the two extensions is
// the only thing reaching.
