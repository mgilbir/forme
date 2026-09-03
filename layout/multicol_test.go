package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Multi-column Layout 1: a block whose content is poured into columns.
//
// The content is laid out once, in a box one column wide, and the result is
// then cut into columns and stood side by side. That is what the specification
// describes rather than a shortcut — the feature is fragmentation, so nothing
// inside a multicol container needs to know how many columns there are.
//
// Courier at 20px over a 20px line makes every number here a whole one: a
// character is 12px along the line and a line is 20px down it.

const colCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     width: 200px; column-gap: 0 }`

// columnLines is where each line of the container's text was drawn, relative to
// the container's content box and in the order it was laid out.
//
// It walks the subtree rather than reading the container's own lines, because a
// container holding a block child has none of its own: the text is in anonymous
// blocks, which is exactly the shape the refusal fixtures need.
func columnLines(t *testing.T, htmlSrc, cssSrc string) []Point {
	t.Helper()
	d := find(t, layoutOf(t, 400, htmlSrc, cssSrc), "d")
	var walk func(*Fragment, Point) []Point
	walk = func(f *Fragment, at Point) []Point {
		var out []Point
		for _, line := range f.Lines {
			out = append(out, Point{
				X: at.X.Add(line.Rect.X), Y: at.Y.Add(line.Rect.Y),
			})
		}
		for _, c := range f.Children {
			// A child's rectangles are relative to its parent's content box,
			// and its own lines are relative to its content box — so the walk
			// carries the child's content origin, which is what ContentRect
			// gives before absolutise has run.
			inner := c.ContentRect()
			out = append(out, walk(c, Point{
				X: at.X.Add(inner.X), Y: at.Y.Add(inner.Y),
			})...)
		}
		return out
	}
	return walk(d, Point{})
}

// TestContentIsPouredIntoColumns.
//
// Four lines in two columns is two lines each, and the second column's lines
// begin one column width to the right and at the top of the box rather than
// below the first column's.
func TestContentIsPouredIntoColumns(t *testing.T) {
	got := columnLines(t, `<div id="d">a<br>b<br>c<br>d</div>`,
		colCSS+`
	#d { column-count: 2 }`)
	want := []Point{
		{X: upx(t, 0), Y: upx(t, 0)},
		{X: upx(t, 0), Y: upx(t, 20)},
		{X: upx(t, 100), Y: upx(t, 0)},
		{X: upx(t, 100), Y: upx(t, 20)},
	}
	if len(got) != len(want) {
		t.Fatalf("the box drew %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is at %v, want %v — two columns of two lines, the "+
				"second column a hundred pixels right and back at the top",
				i, got[i], want[i])
		}
	}
}

// TestTheColumnsAreBalanced.
//
// §3.5's initial "balance": the columns are as short as they can be while
// holding the content between them. Five lines in two columns is three and two
// — not four and one, which also fits, and not two and three, which does not.
func TestTheColumnsAreBalanced(t *testing.T) {
	d := find(t, layoutOf(t, 400, `<div id="d">a<br>b<br>c<br>d<br>e</div>`,
		colCSS+`
	#d { column-count: 2 }`), "d")
	if got := d.ContentRect().H.Px(); got != 60 {
		t.Errorf("the container is %gpx tall, want 60 — five lines in two columns "+
			"balance at three and two", got)
	}
	first, second := 0, 0
	for _, line := range d.Lines {
		if line.Rect.X == 0 {
			first++
			continue
		}
		second++
	}
	if first != 3 || second != 2 {
		t.Errorf("the columns hold %d and %d lines, want 3 and 2", first, second)
	}
}

// TestColumnFillAutoFillsEachColumnInTurn.
//
// The other value: a container told how tall to be fills each column to that
// height in turn, so the last one ends short. Five lines in a hundred-pixel box
// is five in the first column, because five lines are exactly a hundred pixels.
func TestColumnFillAutoFillsEachColumnInTurn(t *testing.T) {
	got := columnLines(t, `<div id="d">a<br>b<br>c<br>d<br>e</div>`,
		colCSS+`
	#d { column-count: 2; column-fill: auto; height: 100px }`)
	if len(got) != 5 {
		t.Fatalf("the box drew %d lines, want 5", len(got))
	}
	for i, at := range got {
		if at.X != 0 {
			t.Errorf("line %d is in the second column at x=%v; a hundred pixels "+
				"holds all five, and auto fills the first column before it starts "+
				"the second", i, at.X)
		}
	}
	// And with room for only three, the fourth begins the second column.
	got = columnLines(t, `<div id="d">a<br>b<br>c<br>d<br>e</div>`,
		colCSS+`
	#d { column-count: 2; column-fill: auto; height: 60px }`)
	if len(got) != 5 {
		t.Fatalf("the box drew %d lines, want 5", len(got))
	}
	if got[2].X != 0 || got[3].X == 0 {
		t.Errorf("with room for three lines the columns break after line %d; the "+
			"third line is at x=%v and the fourth at x=%v", 3, got[2].X, got[3].X)
	}
}

// TestTheGapGoesBetweenTheColumns.
func TestTheGapGoesBetweenTheColumns(t *testing.T) {
	got := columnLines(t, `<div id="d">a<br>b</div>`,
		colCSS+`
	#d { column-count: 2; column-gap: 20px }`)
	if len(got) != 2 {
		t.Fatalf("the box drew %d lines, want 2", len(got))
	}
	// Two columns and one 20px gap in 200px is 90px a column, and the second
	// begins at 90 + 20.
	if got[1].X.Px() != 110 {
		t.Errorf("the second column begins at x=%g, want 110 — two ninety-pixel "+
			"columns with a twenty-pixel gap between them", got[1].X.Px())
	}
}

// TestAColumnWidthIsAMinimumAndACountIsAMaximum.
//
// §3.4 takes both declarations as constraints rather than as results. A width
// says how narrow a column may be, so the count is what fits; a count says how
// many there are to be. Where both are given the count is a maximum the width
// may reduce.
func TestAColumnWidthIsAMinimumAndACountIsAMaximum(t *testing.T) {
	for _, c := range []struct {
		what, css string
		columns   int
	}{
		{"a width of 50px in 200px", "column-width: 50px", 4},
		{"a width of 60px in 200px", "column-width: 60px", 3},
		{"a count of 3", "column-count: 3", 3},
		{"a width of 50px capped by a count of 2", "column-width: 50px; column-count: 2", 2},
		{"a width of 100px under a count of 4", "column-width: 100px; column-count: 4", 2},
	} {
		lines := columnLines(t, `<div id="d">a<br>b<br>c<br>d<br>e<br>f<br>g<br>h</div>`,
			colCSS+`
	#d { `+c.css+` }`)
		seen := map[style.Unit]bool{}
		for _, at := range lines {
			seen[at.X] = true
		}
		if len(seen) != c.columns {
			t.Errorf("%s produced %d columns, want %d", c.what, len(seen), c.columns)
		}
	}
}

// TestABoxTheEngineCannotDivideIsReported, and laid out in one column.
//
// Each row is a way for the cut to go through something CSS assigns edges to,
// or for the content to need a fragmentation this engine does not do. What is
// asserted is both halves: that something was said, and that it names the
// reason — a finding that blamed the wrong thing sends a reader to change the
// wrong declaration.
func TestABoxTheEngineCannotDivideIsReported(t *testing.T) {
	for _, c := range []struct{ what, html, names string }{
		{"a float inside", `<div id="d"><p style="float: left">ab</p>c</div>`,
			"holds a float"},
		{"a positioned box inside", `<div id="d"><p style="position: relative">ab</p>c</div>`,
			"positioned box"},
		{"a form control inside", `<div id="d"><input>c</div>`, "form control"},
		{"a spanning element", `<div id="d"><p style="column-span: all">ab</p>c</div>`,
			"column-span: all"},
		{"a vertical writing mode",
			`<div id="d" style="writing-mode: vertical-rl; height: 100px">ab</div>`,
			"turns a box or columns it"},
	} {
		var said string
		for _, f := range findingsOf(t, c.html, `#d { width: 200px; column-count: 2 }`) {
			if f.Property == "column-count" && said == "" {
				said = f.Message
			}
		}
		if said == "" {
			t.Errorf("nothing was reported about a box with %s, so a page laid out "+
				"in one column says nothing about it", c.what)
			continue
		}
		if !strings.Contains(said, c.names) {
			t.Errorf("the finding for %s is %q, which does not name %q",
				c.what, said, c.names)
		}
	}
}

// TestARefusedBoxIsLaidOutAtItsOwnWidth.
//
// The other half of the sentence the finding ends with. A refused box is not a
// narrow one: its content was laid out at the column width to see whether it
// would divide, and where it will not, it is laid out again at the box's own —
// otherwise a document that asked for columns and was refused would come out
// with its lines broken half as often as a document that never asked.
func TestARefusedBoxIsLaidOutAtItsOwnWidth(t *testing.T) {
	// Sixteen Courier characters are 192px: one line in a 200px box, and two in
	// a 100px column. The box is refused because of the spanning element, so
	// the answer has to be the one-line one.
	const text = `<div id="d"><div style="column-span: all">x</div>` +
		`aaaaaaaaaaaaaaaa</div>`
	refused := columnLines(t, text, colCSS+`
	#d { column-count: 2 }`)
	plain := columnLines(t, text, colCSS)
	if len(plain) != 2 {
		t.Fatalf("with no columns asked for the box came to %d lines, want 2 — the "+
			"spanning element's own line and one line of text", len(plain))
	}
	if len(refused) != len(plain) {
		t.Errorf("refused, the box came to %d lines; with no columns asked for, %d. "+
			"A box laid out in one column is laid out at that column's width, "+
			"which is its own", len(refused), len(plain))
	}
	// The refusal above is decided before anything is laid out. The other one
	// is decided after — the content turned out not to divide where a column
	// would end — and that box *has* been laid out at the column width and has
	// to be laid out again. A child with a background is the case: a box with a
	// background cut in half is a picture whose edges CSS assigns.
	const drawn = `<div id="d"><div style="background: red">` +
		`aaaaaaaaaaaaaaaa<br>b<br>c</div></div>`
	lineWidth := func(cssSrc string) float64 {
		t.Helper()
		d := find(t, layoutOf(t, 400, drawn, cssSrc), "d")
		if len(d.Children) != 1 || len(d.Children[0].Lines) == 0 {
			t.Fatalf("the drawn fixture came out as %d children", len(d.Children))
		}
		return d.Children[0].Lines[0].Rect.W.Px()
	}
	late := lineWidth(colCSS + `
	#d { column-count: 2 }`)
	loose := lineWidth(colCSS)
	if loose != 200 {
		t.Fatalf("with no columns asked for the drawn box's lines are %gpx wide, "+
			"want 200", loose)
	}
	if late != loose {
		t.Errorf("refused after being laid out, the box's lines are %gpx wide and "+
			"with no columns asked for %gpx — the column-width layout has to be "+
			"thrown away, not kept", late, loose)
	}
}

// TestNothingHappensWithoutADeclaration, which is what keeps this off every
// document that has no columns in it.
func TestNothingHappensWithoutADeclaration(t *testing.T) {
	got := columnLines(t, `<div id="d">a<br>b<br>c<br>d</div>`, colCSS)
	for i, at := range got {
		if at.X != 0 || at.Y != upx(t, float64(20*i)) {
			t.Errorf("line %d is at %v; with no column-count and no column-width "+
				"the lines stack down one column", i, at)
		}
	}
	// And a count of one is one column, which is the page that was already
	// there rather than a fragmentation of it.
	got = columnLines(t, `<div id="d">a<br>b</div>`, colCSS+`
	#d { column-count: 1 }`)
	if len(got) != 2 || got[1].X != 0 {
		t.Errorf("with column-count: 1 the second line is at %v, want the first "+
			"column", got[1].X)
	}
}
