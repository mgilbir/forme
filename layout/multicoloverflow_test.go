package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// css-multicol-1 §8.2's overflow columns: "A multicol container can have more
// columns than it has room for due to: a declaration that constrains the
// column height (e.g., using height or max-height) ... [and] explicit column
// breaks. In this case, additional column boxes are created in the inline
// direction". These were refused, and the content laid out in one column at
// the container's width. The fixture is colCSS's: Courier on a 20px line in a
// 200px container with no gap, so the columns of a two-column container are
// 100px apart and those of a one-column container 200px.

// columnLineXs is the x of every line of the container's text, in pixels, in order.
func columnLineXs(t *testing.T, doc, css string) []float64 {
	t.Helper()
	var out []float64
	for _, p := range columnLines(t, doc, css) {
		out = append(out, p.X.Px())
	}
	return out
}

func wantLineXs(t *testing.T, what, doc, css string, want ...float64) {
	t.Helper()
	got := columnLineXs(t, doc, css)
	if len(got) != len(want) {
		t.Errorf("%s: the lines are at x=%v, want %v", what, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: the lines are at x=%v, want %v", what, got, want)
			return
		}
	}
}

func contentHeightOf(t *testing.T, doc, css string) float64 {
	t.Helper()
	return find(t, layoutOf(t, 400, doc, css), "d").ContentRect().H.Px()
}

// TestForcedBreaksOverflowTheColumns is the second cause. Three paragraphs,
// each breaking after itself, in two columns: the third begins a third column,
// past the container's edge, and the container stays as wide as it is. The
// columns are not balanced — "In continuous contexts, this property
// [column-fill] does not have any effect when there are overflow columns" —
// so each holds what is between two breaks, and an automatic height is the
// tallest of them: "Overflow columns can affect the height of the multicol
// container."
func TestForcedBreaksOverflowTheColumns(t *testing.T) {
	css := colCSS + `#d { column-count: 2 } p { margin: 0; break-after: column }`
	const three = `<div id="d"><p>a</p><p>b</p><p>c</p></div>`
	wantLineXs(t, "three paragraphs in two columns", three, css, 0, 100, 200)
	if h := contentHeightOf(t, three, css); h != 20 {
		t.Errorf("the container is %gpx tall, want one line, 20", h)
	}
	if w := find(t, layoutOf(t, 400, three, css), "d").ContentRect().W.Px(); w != 200 {
		t.Errorf("the container is %gpx wide, want its own 200; the overflow column is "+
			"outside it", w)
	}
	// The tallest run decides the height, and it may be an overflow column's.
	const tall = `<div id="d"><p>a</p><p>b</p><p>c<br>d<br>e</p></div>`
	wantLineXs(t, "an overflow column taller than the others", tall, css, 0, 100, 200, 200, 200)
	if h := contentHeightOf(t, tall, css); h != 60 {
		t.Errorf("the container is %gpx tall, want the overflow column's three lines, 60", h)
	}

	// A container of one column is poured when a forced break divides it.
	one := colCSS + `#d { column-count: 1 } p { margin: 0 } #b { break-before: column }`
	const two = `<div id="d"><p>a<br>b</p><p id="b">c</p></div>`
	wantLineXs(t, "one column and a forced break", two, one, 0, 0, 200)
	wantPlaced(t, "one column and a forced break", two, one, "b", [2]float64{200, 0})

	// With a stated height the columns are that tall, filled in turn, and a
	// forced break still ends one: two lines, a break, three lines in 40px
	// columns is two, then two, then one in an overflow column.
	stated := colCSS + `#d { column-count: 2; height: 40px } p { margin: 0 }
		#a { break-after: column }`
	wantLineXs(t, "a stated height and a forced break",
		`<div id="d"><p id="a">a<br>b</p><p>c<br>d<br>e</p></div>`, stated, 0, 0, 100, 100, 200)
}

// TestAConstrainedHeightOverflowsTheColumns is the first cause: six lines in
// two columns of 40px hold four, and the other two are an overflow column.
// Whichever value column-fill has, because the height is given (§3.5), and
// whichever property gave it.
func TestAConstrainedHeightOverflowsTheColumns(t *testing.T) {
	const doc = `<div id="d">a<br>b<br>c<br>d<br>e<br>f</div>`
	for _, decl := range []string{
		"height: 40px", "height: 40px; column-fill: auto",
		// max-height with an automatic height: the balance, 60, is more than
		// it allows, so the columns are 40 and filled in turn.
		"max-height: 40px",
		// And a height the maximum holds down: the columns are the 40 the box
		// is drawn at, not the 200 declared. They were 200, every line in
		// the first column and four of them below the box.
		"height: 200px; max-height: 40px",
	} {
		css := colCSS + `#d { column-count: 2; ` + decl + ` }`
		wantLineXs(t, decl, doc, css, 0, 0, 100, 100, 200, 200)
		if h := contentHeightOf(t, doc, css); h != 40 {
			t.Errorf("%s: the container is %gpx tall, want 40", decl, h)
		}
	}
	// A maximum the balance is under changes nothing: three and three.
	css := colCSS + `#d { column-count: 2; max-height: 100px }`
	wantLineXs(t, "a maximum that does not bind", doc, css, 0, 0, 0, 100, 100, 100)
	if h := contentHeightOf(t, doc, css); h != 60 {
		t.Errorf("under a maximum that does not bind the container is %gpx tall, want "+
			"the balance, 60", h)
	}
	// A minimum raises the columns with the box: 60px columns hold three each.
	css = colCSS + `#d { column-count: 2; height: 40px; min-height: 60px }`
	wantLineXs(t, "a minimum over the height", doc, css, 0, 0, 0, 100, 100, 100)
	// And one column with a height its content does not fit is poured too.
	css = colCSS + `#d { column-count: 1; height: 40px }`
	wantLineXs(t, "one column of stated height", doc, css, 0, 0, 200, 200, 400, 400)
}

// TestOverflowColumnsAreBounded: a column one pixel tall over a margin of ten
// thousand is ten thousand columns with nothing in them, which the pieces
// bound does not count. maxOverflowColumns bounds them, and a pour past it is
// refused, reported and laid out in one column, as every refused pour is.
func TestOverflowColumnsAreBounded(t *testing.T) {
	defer func(n int) { maxOverflowColumns = n }(maxOverflowColumns)
	maxOverflowColumns = 50
	const doc = `<div id="d"><div style="margin-top: 10000px">a</div></div>`
	css := colCSS + `#d { column-count: 2; height: 1px }`
	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}}, Options{})
	said := false
	for _, f := range got.Findings {
		if f.Property == "column-count" && strings.Contains(f.Message, "overflow columns") {
			said = true
		}
	}
	if !said {
		t.Errorf("a pour needing ten thousand columns against a bound of 50 said "+
			"nothing: %v", findingList(got.Findings))
	}
	if xs := columnLineXs(t, doc, css); len(xs) != 1 || xs[0] != 0 {
		t.Errorf("the line is at x=%v; a refused pour is one column", xs)
	}
	// And under the bound the same document is poured: a 100px column is a
	// hundred and one columns, the last holding the line.
	maxOverflowColumns = 200
	css = colCSS + `#d { column-count: 2; height: 100px }`
	if xs := columnLineXs(t, doc, css); len(xs) != 1 || xs[0] != 100*100 {
		t.Errorf("the line is at x=%v; the gap fills a hundred columns and the line "+
			"begins the next, at 10000", xs)
	}
}

// TestOverflowColumnsPourInLinearTime: n lines with a forced break after each
// but the last, into two columns — n-2 overflow columns. The height is one
// search over the breakpoints, each column is cut once, and the columns are
// placed as they are made. Measured on the machinery alone, over a fragment
// built for it, because at these sizes the layout around it hides a quadratic
// term: a planted walk of every column made so far, per column, passed when
// this was measured through Layout, and fails measured here.
func TestOverflowColumnsPourInLinearTime(t *testing.T) {
	plain := &Box{Style: style.Initial()}
	const line = style.Unit(64)
	content := func(n int) (*Fragment, []style.Unit) {
		f := &Fragment{Box: plain}
		forced := make([]style.Unit, 0, n)
		for i := 0; i < n; i++ {
			f.Lines = append(f.Lines, LineFragment{Rect: Rect{Y: line.Mul(float64(i)), H: line}})
			if i+1 < n {
				forced = append(forced, line.Mul(float64(i+1)))
			}
		}
		return f, forced
	}
	pour := func(n int, columnsMade *int) func() {
		return func() {
			f, forced := content(n)
			breaks := sortedBreaks(columnBreaks(f, 0, nil))
			h, ok := balancedHeight(breaks, forced, len(forced)+1)
			if !ok {
				t.Fatal("no height holds one line per column")
			}
			c := columns{n: 2, width: 640, gap: 64}
			if ok, why := fillColumnsWith(f, c, h, &columnEnds{forced: forced, breaks: breaks,
				overflow: true}); !ok {
				t.Fatalf("the pour was refused: %s", why)
			}
			*columnsMade = int(f.Lines[len(f.Lines)-1].Rect.X/c.width.Add(c.gap)) + 1
		}
	}
	var small, large int
	c := costtest.Time(t, "pouring into n overflow columns", pour(4000, &small), pour(16000, &large))
	if small != 4000 || large != 16000 {
		t.Fatalf("the last line is in column %d and %d; the fixture is meant to make "+
			"4000 and 16000", small, large)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the columns took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}
