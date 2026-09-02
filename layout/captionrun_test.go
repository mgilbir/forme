package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A caption is a proper table child, so it joins the anonymous table the cells
// around it make — whichever side of them it is written on.
//
// §17.2.1 generates an anonymous table around a misparented proper table child
// "and all consecutive siblings of C that are proper table children", and its
// own list of those is row, row group, header group, footer group, column,
// column group and *caption*. This engine extended the run over the internal
// boxes only, so a caption written after the cells was left outside the wrapper
// — and a caption outside the wrapper is an ordinary block that caption-side
// cannot move, so it stayed below the table however it was set.
//
// caption-position-001's first rectangle is exactly that: a "display: table-cell"
// holding a picture followed by a "display: table-caption" holding the word
// ABOVE, and the test asks for ABOVE above the picture.

// textYs is where a fixture's runs are drawn, in paint order.
func textYs(t *testing.T, htmlSrc, cssSrc string) map[string]style.Unit {
	t.Helper()
	out := map[string]style.Unit{}
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			if _, seen := out[v.Text]; !seen {
				out[v.Text] = v.At.Y
			}
		}
	}
	return out
}

const captionCSS = `.cap { display: table-caption } .cell { display: table-cell } p { margin: 0 }`

func TestACaptionAfterTheCellsIsStillACaption(t *testing.T) {
	got := textYs(t, `<div><p class=cell>CELL</p><p class=cap>CAP</p></div>`, captionCSS)
	if got["CAP"] >= got["CELL"] {
		t.Errorf("the caption is at y=%v and the cell at y=%v; caption-side is "+
			"\"top\" and decides, not the order the two were written in",
			got["CAP"], got["CELL"])
	}
}

func TestACaptionBeforeTheCellsIsUnchanged(t *testing.T) {
	got := textYs(t, `<div><p class=cap>CAP</p><p class=cell>CELL</p></div>`, captionCSS)
	if got["CAP"] >= got["CELL"] {
		t.Errorf("the caption is at y=%v and the cell at y=%v", got["CAP"], got["CELL"])
	}
}

func TestCaptionSideStillMovesIt(t *testing.T) {
	for _, side := range []string{"bottom", "BOTTOM", " bottom "} {
		got := textYs(t, `<div><p class=cell>CELL</p><p class=cap>CAP</p></div>`,
			captionCSS+` .cap { caption-side: `+side+` }`)
		if got["CAP"] <= got["CELL"] {
			t.Errorf("with caption-side: %q the caption is at y=%v and the cell "+
				"at y=%v; a CSS keyword is case-insensitive and this one is read "+
				"like every other", side, got["CAP"], got["CELL"])
		}
	}
}

func TestTwoCaptionsAroundTheCellsBothJoin(t *testing.T) {
	// One before and one after. Both are proper table children of the same run,
	// so both are in the wrapper and caption-side puts each where it asks.
	got := textYs(t, `<div><p class=cap>ONE</p><p class=cell>CELL</p><p class=cap2>TWO</p></div>`,
		captionCSS+` .cap2 { display: table-caption; caption-side: bottom }`)
	if !(got["ONE"] < got["CELL"] && got["CELL"] < got["TWO"]) {
		t.Errorf("the three are at %v, %v and %v; the top caption belongs above "+
			"the table and the bottom one below it", got["ONE"], got["CELL"], got["TWO"])
	}
}

func TestProperTableChildIsTheSpecificationsList(t *testing.T) {
	for _, inner := range []Inner{InnerTableRow, InnerTableRowGroup,
		InnerTableColumn, InnerTableColumnGroup, InnerTableCaption} {
		if !properTableChild(&Box{Inner: inner}) {
			t.Errorf("%v is in §17.2.1's list of proper table children and is "+
				"not reported as one", inner)
		}
	}
	for _, inner := range []Inner{InnerFlow, InnerFlowRoot, InnerTable, InnerTableCell} {
		if properTableChild(&Box{Inner: inner}) {
			t.Errorf("%v is not a proper table child and is reported as one", inner)
		}
	}
}
