package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// §17.5.3's vertical alignment moves a cell's content down inside its row, and
// an absolutely positioned box written in that cell has to move with it: its
// static position is where it would have been in the flow, and the flow just
// shifted.
//
// Finding those boxes was a walk from the cell's own mark in the deferred queue
// to wherever the queue had grown to by the time the row was aligned — which is
// every out-of-flow box in every cell laid out after this one. Each was
// compared against this cell's fragment, matched nothing, and was walked again
// by the next cell. Work proportional to cells times boxes, for an answer that
// was right every time.
//
// Measured on a table of N rows, two cells each, one absolutely positioned box
// per row and the short cell aligned to the bottom so the alignment actually
// runs:
//
//	N        walked (before)   walked (after)
//	500          250,500            500
//	1,000      1,001,000          1,000
//	2,000      4,002,000          2,000
//	4,000     16,004,000          4,000
//
// Four times the walk for twice the table, three doublings running. It did not
// show as time — the loop body is a pointer comparison, and at these sizes the
// parsing and the layout cost fifty times as much — which is exactly why the
// count is asserted rather than the clock. A document is untrusted, and eight
// kilobytes of markup buying a hundred million comparisons is the beginning of
// a shape that gets worse.
func TestAligningTableCellsIsLinearInTheOutOfFlowBoxes(t *testing.T) {
	const rows = 2000

	var b strings.Builder
	b.WriteString(`<table id="t">`)
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, `<tr><td class="tall">x</td>`+
			`<td class="short"><span class="a">y</span></td></tr>`)
	}
	b.WriteString(`</table>`)

	built := Build(Input{HTML: b.String(), CSS: []Stylesheet{{Source: `
		#t { border-collapse: separate }
		td.tall { height: 60px }
		td.short { vertical-align: bottom; position: relative }
		span.a { position: absolute }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(1 << 20)

	l := newLayouter(built.Root, Size{W: w, H: h}, StandardFonts(), NewRecorder(nil))
	frag := l.layout()
	if frag == nil {
		t.Fatal("the document laid out to nothing")
	}

	// One box per row, so a walk that looks only where the boxes are does one
	// step per row. The bound is four times that: enough room for the walk to
	// be written differently, and a two-hundredth of what the scan to the end
	// of the queue costs at this size.
	if l.absScans > 4*rows {
		t.Errorf("aligning %d cells walked %d deferred entries; there are %d "+
			"out-of-flow boxes in the table, and a cell is meant to look only "+
			"at its own", 2*rows, l.absScans, rows)
	}
	// And it walked *something*: a bound met by an alignment that never ran is
	// not a result about the walk. Every row has one box to move.
	if l.absScans < rows {
		t.Errorf("aligning %d cells walked %d deferred entries, and there are "+
			"%d boxes to move; the alignment did not run", 2*rows, l.absScans, rows)
	}

	// The answer is still the answer. The box states neither top nor left, so
	// §10.6.4 puts it at its static position — where it would have been in the
	// flow — and the flow is a line at the bottom of a 60px cell. A box whose
	// static position was not moved with the content sits at the top of the
	// cell instead, which is what the walk exists to prevent.
	moved := absoluteInFirstShortCell(t, frag)
	if moved == nil {
		t.Fatal("the absolutely positioned box was not placed")
	}
	if moved.BorderRect.Y.Px() < 30 {
		t.Errorf("the box in a bottom-aligned cell is %gpx down the page; the "+
			"cell is 60px tall and its content is at the bottom, so a box that "+
			"did not move with it would be at the top", moved.BorderRect.Y.Px())
	}
}

// absoluteInFirstShortCell finds the first fragment of a box carrying class
// "a", which is the absolutely positioned span.
func absoluteInFirstShortCell(t *testing.T, root *Fragment) *Fragment {
	t.Helper()
	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found != nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if class, _ := f.Box.Element.Attr("class"); class == "a" {
				found = f
				return
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return found
}
