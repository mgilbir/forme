package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A table's own background and border are painted before its captions'.
//
// §E.2's step 4 paints block backgrounds in tree order, and in the element tree
// a <caption> is a child of its <table>. The wrapper §17.4 puts round them has
// them the other way up — a caption above the grid is the wrapper's first child
// and the table its second — so the caption's background went down first and a
// caption pulled over the table by a negative margin was drawn under the
// table's border. root-box-002 hides a red top border that way.
//
// Only the table's own background moves: what is inside it, the rows and
// cells, is still painted after a caption above it.

var (
	captionOrderRed   = style.RGBA{R: 255, A: 1}
	captionOrderBlue  = style.RGBA{B: 255, A: 1}
	captionOrderGreen = style.RGBA{G: 255, A: 1}
)

func TestATablesBorderIsPaintedBeforeItsCaption(t *testing.T) {
	ops := paintOf(t,
		`<table><caption>x</caption><tr><td>y</td></tr></table>`,
		noDefaults+`
		 table { border-collapse: separate; border-spacing: 0;
		         border-top: 10px solid rgb(255,0,0) }
		 caption { background: rgb(0,0,255); margin-bottom: -10px }
		 td { background: rgb(0,255,0); padding: 0 }`)
	border := indexOfFill(ops, captionOrderRed)
	caption := indexOfFill(ops, captionOrderBlue)
	cell := indexOfFill(ops, captionOrderGreen)
	if border < 0 || caption < 0 || cell < 0 {
		t.Fatalf("the table's border, the caption's background and the cell's "+
			"are ops %d, %d and %d; each has to be painted to be ordered", border, caption, cell)
	}
	if border > caption {
		t.Errorf("the table's border is op %d and the caption's background op %d; "+
			"the table is the caption's parent and is painted first", border, caption)
	}
	if cell < caption {
		t.Errorf("the cell's background is op %d and the caption's op %d; only "+
			"the table's own background moves, and the cells stay after a caption "+
			"above them", cell, caption)
	}
}

// TestATranslucentTableIsLeftToItsOwnLayer is the case the reordering must stay
// out of. A table with an opacity below one is a stacking context, painted
// whole at level zero (§E.2 step 8 by way of CSS Color 4) — after the in-flow
// blocks, so a later block pulled up under it by a negative margin is painted
// first. Taking the table's background into the block layer with its captions
// would put it back under that block.
func TestATranslucentTableIsLeftToItsOwnLayer(t *testing.T) {
	ops := paintOf(t,
		`<table><caption>x</caption><tr><td>y</td></tr></table><div></div>`,
		noDefaults+`
		 table { opacity: 0.5; background: rgb(255,0,0); border-spacing: 0 }
		 td { padding: 0 }
		 div { margin-top: -10px; height: 20px; background: rgb(0,255,0) }`)
	table, block := -1, indexOfFill(ops, captionOrderGreen)
	count := 0
	for i, op := range ops {
		// The opacity reaches the fill as its alpha, so the colour is matched
		// on its channels alone.
		if v, ok := op.(FillRect); ok && v.Color.R == 255 && v.Color.G == 0 && v.Color.B == 0 {
			table = i
			count++
		}
	}
	if count != 1 || block < 0 {
		t.Fatalf("the table's background is painted %d times and the block's is op %d; "+
			"want once each", count, block)
	}
	if table < block {
		t.Errorf("the translucent table is op %d and the later block op %d; a "+
			"stacking context is painted after the in-flow blocks", table, block)
	}
}
