package layout

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// A right-to-left table in the collapsing border model.
//
// §17.2 makes the direction of a table the direction of its columns, so the
// first column of a right-to-left table is its rightmost. Everything else about
// the table is physical: "border-left" is the border on a box's left whichever
// way its table runs, and a grid line is drawn where it physically is.
//
// Those two facts together give a test that needs no expected numbers at all. A
// right-to-left table is the mirror image of the left-to-right table whose cells
// are written in the same order with their left and right declarations swapped:
// the first cell is at the other end, its left border is where its right border
// was, and every grid line is where the mirror puts it. Even §17.6.2.1's last
// tie-break mirrors, because it is stated in terms of the direction — "the one
// further to the left (if the table's 'direction' is 'ltr'; right, if it is
// 'rtl')" — so the cell nearer the start of the row wins in both tables.
//
// Every width here is an even number of layout units, so that the two halves of
// each line are equal and the mirror is exact rather than one unit out.

// mirroredTable writes the same table twice: once as given, left to right, and
// once right to left with every left and right swapped.
func mirroredTable(dir string) string {
	swap := func(decl string) string {
		if dir == "ltr" {
			return decl
		}
		decl = strings.ReplaceAll(decl, "left", "\x00")
		decl = strings.ReplaceAll(decl, "right", "left")
		return strings.ReplaceAll(decl, "\x00", "right")
	}
	cell := func(id, decl string, span int) string {
		return fmt.Sprintf(`<td id=%s colspan=%d style="%s">%s</td>`, id, span, swap(decl), id)
	}
	return `<table id=t style="direction:` + dir + `; ` +
		swap(`border-left: 12px solid #010101; border-right: 4px dashed #020202; `+
			`border-top: 6px solid #030303; border-bottom: 6px solid #040404`) + `">` +
		`<colgroup id=g><col id=c0 style="` + swap(`border-right: 8px solid #050505`) + `">` +
		`<col id=c1><col id=c2 style="` + swap(`border-left: 2px double #060606`) + `">` +
		`</colgroup>` +
		`<tr>` +
		cell("a", `width:40px; padding-left:6px; border-left: 20px solid #111111; border-right: 2px solid #121212`, 1) +
		// b and c meet on a line where both declare the same width and style,
		// so only the element tie-break decides it.
		cell("b", `width:30px; border-left: 4px solid #131313; border-right: 6px solid #141414`, 1) +
		cell("c", `width:50px; padding-right:10px; border-left: 6px solid #151515; border-right: 2px solid #161616`, 1) +
		`</tr><tr>` +
		cell("d", `border-left: 2px solid #171717; border-right: 10px solid #181818`, 2) +
		cell("e", `border-left: 4px solid #191919`, 1) +
		`</tr></table>`
}

type sideBand struct {
	x, y, w, h int32
	id         string
	side       side
}

// collapsedBandsOf lays out a document and returns the grid lines its one
// collapsing table draws, each named by the element whose border won it, with
// x measured from the table's border box.
func collapsedBandsOf(t *testing.T, doc string) ([]sideBand, *Fragment, *Fragment) {
	t.Helper()
	root := layoutOf(t, 1000, doc, collapsing)
	table := find(t, root, "t")
	var holder *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if len(f.collapsed) > 0 {
			holder = f
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if holder == nil {
		t.Fatalf("the table drew no collapsed grid lines:\n%s", sketchFragments(root))
	}
	var out []sideBand
	for _, b := range holder.collapsed {
		id := ""
		if b.box != nil && b.box.Element != nil {
			id, _ = b.box.Element.Attr("id")
		}
		out = append(out, sideBand{
			x: int32(b.rect.X), y: int32(b.rect.Y), w: int32(b.rect.W), h: int32(b.rect.H),
			id: id, side: b.side,
		})
	}
	return out, holder, table
}

func sortBands(bs []sideBand) {
	sort.Slice(bs, func(i, j int) bool {
		a, b := bs[i], bs[j]
		if a.x != b.x {
			return a.x < b.x
		}
		if a.y != b.y {
			return a.y < b.y
		}
		if a.w != b.w {
			return a.w < b.w
		}
		if a.h != b.h {
			return a.h < b.h
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.side < b.side
	})
}

func mirrorSide(s side) side {
	switch s {
	case sideLeft:
		return sideRight
	case sideRight:
		return sideLeft
	}
	return s
}

// TestARightToLeftCollapsedTableIsTheMirrorOfItsLeftToRightTwin is audit C39.
func TestARightToLeftCollapsedTableIsTheMirrorOfItsLeftToRightTwin(t *testing.T) {
	ltr, lholder, ltable := collapsedBandsOf(t, mirroredTable("ltr"))
	rtl, rholder, rtable := collapsedBandsOf(t, mirroredTable("rtl"))
	if lholder.BorderRect.W != rholder.BorderRect.W {
		t.Fatalf("the two tables are %v and %v wide; the mirror needs them equal",
			lholder.BorderRect.W, rholder.BorderRect.W)
	}
	width := int32(lholder.BorderRect.W)

	want := make([]sideBand, len(ltr))
	for i, b := range ltr {
		b.x = width - b.x - b.w
		b.side = mirrorSide(b.side)
		want[i] = b
	}
	sortBands(want)
	sortBands(rtl)
	if fmt.Sprint(want) != fmt.Sprint(rtl) {
		t.Errorf("the right-to-left table's grid lines are not the mirror of the "+
			"left-to-right one's.\nwant %v\ngot  %v", want, rtl)
	}

	// The table's own used border is the outer half of its outermost lines, and
	// those swap ends too: the 12px border is on the left of the left-to-right
	// table and on the right of the other.
	if ltable.Border.Left != rtable.Border.Right || ltable.Border.Right != rtable.Border.Left {
		t.Errorf("the table's used border is %v left to right and %v right to left; "+
			"each should be the other's mirror", ltable.Border, rtable.Border)
	}

	// And each cell is where the mirror puts it, with its used border and its
	// padding on the mirrored sides — which is what decides where its content
	// sits.
	mirroredCells(t, collapsing)
}

// TestARightToLeftTableSpansTheRightColumns is the half of the mirror that is not
// about borders at all, and so holds in the separated model too: a cell or a
// column group spanning several columns covers them wherever they are. Its first
// column is its rightmost in a right-to-left table, and reading the span from
// that column's left edge put a two-column cell over the wrong column and gave
// the column group a negative width.
func TestARightToLeftTableSpansTheRightColumns(t *testing.T) {
	mirroredCells(t, `html, body, p, div { margin: 0; padding: 0 }
		table { border-collapse: separate; border-spacing: 6px 4px }
		td, th { padding: 0 }
		#g { background: #0a0a0a }`)
}

// mirroredCells checks that every cell, column and column group of the right-to-
// left table is where the mirror of the left-to-right one puts it.
func mirroredCells(t *testing.T, css string) {
	t.Helper()
	lroot := layoutOf(t, 1000, mirroredTable("ltr"), css)
	rroot := layoutOf(t, 1000, mirroredTable("rtl"), css)
	lt, rt := find(t, lroot, "t"), find(t, rroot, "t")
	if lt.BorderRect.W != rt.BorderRect.W {
		t.Fatalf("the two tables are %v and %v wide; the mirror needs them equal",
			lt.BorderRect.W, rt.BorderRect.W)
	}
	for _, id := range []string{"a", "b", "c", "d", "e", "g", "c0", "c2"} {
		l, r := find(t, lroot, id), find(t, rroot, id)
		lx, rx := l.BorderRect.X.Sub(lt.BorderRect.X), r.BorderRect.X.Sub(rt.BorderRect.X)
		if r.BorderRect.W != l.BorderRect.W {
			t.Errorf("#%s is %v wide in the right-to-left table and %v in the other",
				id, r.BorderRect.W, l.BorderRect.W)
		}
		if mirrored := lt.BorderRect.W.Sub(lx).Sub(l.BorderRect.W); rx != mirrored {
			t.Errorf("#%s is at %v in the right-to-left table, want %v", id, rx, mirrored)
		}
		if l.Border.Left != r.Border.Right || l.Border.Right != r.Border.Left {
			t.Errorf("#%s's used border is %v left to right and %v right to left",
				id, l.Border, r.Border)
		}
		if l.Padding.Left != r.Padding.Right || l.Padding.Right != r.Padding.Left {
			t.Errorf("#%s's padding is %v left to right and %v right to left",
				id, l.Padding, r.Padding)
		}
	}
}

// TestARightToLeftCollapsedTableKeepsItsFrame is the audit's own document, stated
// physically: a 10px frame round two cells has a top and a bottom the width of
// the table and a line down each outside edge, and the one interior line is
// between the cells rather than at the table's edge.
func TestARightToLeftCollapsedTableKeepsItsFrame(t *testing.T) {
	bands, holder, _ := collapsedBandsOf(t,
		`<table id=t style="direction: rtl; border: 10px solid black">`+
			`<tr><td id=a style="width:80px; border: 2px solid red">a</td>`+
			`<td id=b style="width:80px; border: 2px solid blue">b</td></tr></table>`)
	width := int32(holder.BorderRect.W)
	var top, left, right, middle bool
	for _, b := range bands {
		switch {
		case b.id == "t" && b.side == sideTop && b.x == 0 && b.w == width:
			top = true
		case b.id == "t" && b.side == sideLeft && b.x == 0:
			left = true
		case b.id == "t" && b.side == sideRight && b.x+b.w == width:
			right = true
		case b.id != "t" && b.x > 0 && b.x+b.w < width:
			middle = true
		}
	}
	if !top || !left || !right || !middle {
		t.Errorf("top across the table %v, left edge %v, right edge %v, an interior line %v; "+
			"want all four:\n%v", top, left, right, middle, bands)
	}
}
