package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// One <col span="2"> is one box describing two columns, and one background
// across both of them.
//
// §17.2 lets a single <col> stand for several columns, and §17.5.1 draws it as
// one of the six layers behind the cells. The two together mean the element has
// one background box, as wide as everything it spans — not one box per column,
// and not one box the width of the first.
//
// Both arms that widen it were at 0% across every unit test and all 6253 reftest
// documents: one for a spanning <col> inside a <colgroup> and one for a spanning
// <col> reached without a group. Nothing writes the attribute.
//
// What a broken one looks like is a background that stops halfway. The fragment
// is created for the first column the box covers and then never widened, so a
// <col span="2"> with a colour paints the left half of what the author coloured
// and leaves the right half showing the table.

// collapsedTable paints a two-by-two table with the borders collapsed, where a
// background is one rectangle rather than a band per cell — which is what makes
// "one box, this wide" an assertion rather than a count.
func collapsedTable(t *testing.T, cols, css string, colour style.RGBA) []Rect {
	t.Helper()
	doc := `<table id="t">` + cols + `
		<tr><td>a</td><td>b</td></tr>
		<tr><td>c</td><td>d</td></tr>
	</table>`
	return fillsOf(paintOf(t, doc, `
		#t { border-collapse: collapse; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		`+css), colour)
}

func TestOneSpanningColIsOneBackgroundAcrossItsColumns(t *testing.T) {
	for _, c := range []struct {
		name string
		cols string
	}{
		{"inside a colgroup", `<colgroup><col id="c" span="2"/></colgroup>`},
		{"with no colgroup written", `<col id="c" span="2"/>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := collapsedTable(t, c.cols, `#c { background: rgb(0,0,255) }`, blue)
			if len(got) != 1 {
				t.Fatalf("the spanning col painted %d rectangles, want 1: it is "+
					"one element and one background box however many columns it "+
					"describes — %v", len(got), got)
			}
			// Two 30px columns with the borders collapsed and no spacing.
			if w := got[0].W.Px(); w != 60 {
				t.Errorf("the background is %gpx wide, want 60 — both columns. "+
					"30 is the first column alone, which is the box being "+
					"created and never widened", w)
			}
			// 8 is the body's own margin: with the borders collapsed there
			// is no border-spacing in front of the first column, so the table's
			// left edge is where it starts.
			if x := got[0].X.Px(); x != 8 {
				t.Errorf("the background starts at %gpx, want 8 — the left edge "+
					"of the first column it spans", x)
			}
			if h := got[0].H.Px(); h != 80 {
				t.Errorf("the background is %gpx tall, want 80 — both rows", h)
			}
		})
	}
}

// A spanning col that does not begin at its group's first column.
//
// The widened box is measured from its own left edge, and in a group whose first
// column is the spanning one that edge is zero — so a version that forgot to
// subtract it is right by accident. Here the group holds a plain col first, so
// the spanning one starts an offset in and the subtraction is load-bearing.
func TestASpanningColIsWidenedFromItsOwnLeftEdge(t *testing.T) {
	const rows = `<tr><td>a</td><td>b</td><td>c</td></tr>
		<tr><td>d</td><td>e</td><td>f</td></tr>`
	got := fillsOf(paintOf(t, `<table id="t">
		<colgroup><col/><col id="c" span="2"/></colgroup>`+rows+`</table>`, `
		#t { border-collapse: collapse; font-size: 0 }
		td { width: 30px; height: 40px; padding: 0; border: 0 }
		#c { background: rgb(0,0,255) }`), blue)

	if len(got) != 1 {
		t.Fatalf("the spanning col painted %d rectangles, want 1: %v", len(got), got)
	}
	// Three 30px columns from the body's 8px margin: 8, 38, 68. The col spans
	// the second and third.
	if x := got[0].X.Px(); x != 38 {
		t.Errorf("the background starts at %gpx, want 38 — the second column, "+
			"which is where this col begins", x)
	}
	if w := got[0].W.Px(); w != 60 {
		t.Errorf("the background is %gpx wide, want 60 — its two columns. 90 is "+
			"the width measured from the group's left edge rather than its own",
			w)
	}
}

// Two separate <col> elements are two boxes, which is what says the widening is
// the span rule rather than columns being merged whenever they are alike. Once
// in a group and once without, because each is a different arm.
func TestTwoSeparateColsAreTwoBackgroundsWithNoGroup(t *testing.T) {
	got := collapsedTable(t,
		`<col id="c1"/><col id="c2"/>`,
		`#c1, #c2 { background: rgb(0,0,255) }`, blue)
	if len(got) != 2 {
		t.Fatalf("two ungrouped cols painted %d rectangles, want 2: they are two "+
			"elements and two background boxes — %v", len(got), got)
	}
	for i, r := range got {
		if w := r.W.Px(); w != 30 {
			t.Errorf("background %d is %gpx wide, want 30 — one column each", i, w)
		}
	}
}

func TestTwoSeparateColsAreTwoBackgrounds(t *testing.T) {
	got := collapsedTable(t,
		`<colgroup><col id="c1"/><col id="c2"/></colgroup>`,
		`#c1, #c2 { background: rgb(0,0,255) }`, blue)
	if len(got) != 2 {
		t.Fatalf("two cols painted %d rectangles, want 2: they are two elements "+
			"and two background boxes, adjacent but not one — %v", len(got), got)
	}
	for i, r := range got {
		if w := r.W.Px(); w != 30 {
			t.Errorf("background %d is %gpx wide, want 30 — one column each", i, w)
		}
	}
}

// A span wider than the table is clamped to the columns that exist, since the
// attribute is the author's and the cells decide how many columns there are.
func TestASpanWiderThanTheTableStopsAtTheLastColumn(t *testing.T) {
	got := collapsedTable(t,
		`<colgroup><col id="c" span="9"/></colgroup>`,
		`#c { background: rgb(0,0,255) }`, blue)
	if len(got) != 1 {
		t.Fatalf("the over-wide col painted %d rectangles, want 1: %v", len(got), got)
	}
	if w := got[0].W.Px(); w != 60 {
		t.Errorf("the background is %gpx wide, want 60 — the table has two "+
			"columns however many the attribute claims", w)
	}
}
