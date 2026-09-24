package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// CSS Fragmentation 3 §3: the breaks a document asks a multicol pour to avoid.
//
// These were declared inert — "this engine does not fragment at all" — and the
// claim stopped being true when multicol began cutting boxes across columns: a
// box that asked not to be split was split, and the one declaration that would
// have said so was kept quiet (audit C41). Every fixture below is lines of
// Courier on a 20px line, so each break is a whole number of lines, and each
// is checked against the same document without the declaration, so that the
// fixture is known to put a break exactly where the declaration forbids one.

// placed is where each fragment of an element was put, relative to the
// multicol container's content box: one entry per piece. The laid-out tree is
// in page coordinates, so it is one subtraction.
func placed(t *testing.T, htmlSrc, cssSrc, id string) []Point {
	t.Helper()
	d := find(t, layoutOf(t, 400, htmlSrc, cssSrc), "d")
	origin := d.ContentRect()
	var out []Point
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		for _, c := range f.Children {
			if c.Box != nil && c.Box.Element != nil {
				if got, _ := c.Box.Element.Attr("id"); got == id {
					out = append(out, Point{X: c.BorderRect.X.Sub(origin.X),
						Y: c.BorderRect.Y.Sub(origin.Y)})
				}
			}
			walk(c)
		}
	}
	walk(d)
	return out
}

// TestAnAvoidedBoxIsNotSplitAcrossColumns is the audit's case: a box in the
// middle of balanced content, where the balance point falls inside it.
//
// Eight lines in two columns balance at four and four, which cuts #k — lines
// three to six — in half. With "break-inside: avoid" the columns are made as
// tall as it takes to keep it whole, and it is one piece.
func TestAnAvoidedBoxIsNotSplitAcrossColumns(t *testing.T) {
	const doc = `<div id="d"><div>a<br>b</div><div id="k">c<br>d<br>e<br>f</div>` +
		`<div>g<br>h</div></div>`
	css := colCSS + `#d { column-count: 2 }`
	if got := placed(t, doc, css, "k"); len(got) != 2 {
		t.Fatalf("without the declaration #k is in %d pieces; the fixture needs the "+
			"balance to cut it in two", len(got))
	}
	for _, decl := range []string{"break-inside: avoid", "break-inside: avoid-column",
		"page-break-inside: avoid"} {
		got := placed(t, doc, css+`#k { `+decl+` }`, "k")
		if len(got) != 1 {
			t.Errorf("%s: #k is in %d pieces, want one", decl, len(got))
		}
		// Kept whole by the columns and not by giving them up: the shortest
		// balance that does not cut #k is six lines and two, and the last two
		// lines begin the second column. A pour refused for want of a height
		// would keep #k whole too, in one column, and is not the answer.
		d := find(t, layoutOf(t, 400, doc, css+`#k { `+decl+` }`), "d")
		if h := d.ContentRect().H; h != upx(t, 120) {
			t.Errorf("%s: the columns are %v tall, want 120px — #k and the two lines "+
				"before it in the first", decl, h)
		}
		if lines := columnLines(t, doc, css+`#k { `+decl+` }`); len(lines) != 8 ||
			lines[5].X != 0 || lines[6].X == 0 {
			t.Errorf("%s: the lines are at %v; six belong in the first column and two "+
				"in the second", decl, lines)
		}
	}
	// "avoid-page" asks nothing of a column.
	if got := placed(t, doc, css+`#k { break-inside: avoid-page }`, "k"); len(got) != 2 {
		t.Errorf("break-inside: avoid-page kept #k whole across a column break; it "+
			"is about pages, and #k is in %d pieces", len(got))
	}
}

// TestAColumnEndsBeforeABoxItMayNotBreakInside: with the height given and
// "column-fill: auto", the first column ends at 100px, two lines into #k. The
// column ends before #k instead, and #k begins the second.
func TestAColumnEndsBeforeABoxItMayNotBreakInside(t *testing.T) {
	const doc = `<div id="d"><div>a<br>b<br>c</div><div id="k">d<br>e<br>f</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 100px }
		#k { break-inside: avoid }`
	got := placed(t, doc, css, "k")
	if len(got) != 1 || got[0].X != upx(t, 100) || got[0].Y != 0 {
		t.Errorf("#k is at %v; it belongs whole at the top of the second column", got)
	}
}

// TestABreakBetweenTwoBoxesIsAvoided is "break-after" and "break-before": a
// heading and the paragraph after it. Three lines fill the 60px column exactly,
// so the column ends between #h and #p — which either declaration forbids, so
// it ends a line earlier and #h goes with #p.
func TestABreakBetweenTwoBoxesIsAvoided(t *testing.T) {
	const doc = `<div id="d"><div>a<br>b</div><div id="h">c</div><div id="p">d<br>e</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 60px }`
	if got := placed(t, doc, css, "h"); len(got) != 1 || got[0].X != 0 {
		t.Fatalf("without a declaration #h is at %v; the fixture needs it to end the "+
			"first column", got)
	}
	for _, decl := range []string{
		"#h { break-after: avoid }", "#p { break-before: avoid }",
		"#h { page-break-after: avoid }", "#p { page-break-before: avoid }",
		"#h { break-after: avoid-column }",
	} {
		got := placed(t, doc, css+decl, "h")
		if len(got) != 1 || got[0].X != upx(t, 100) || got[0].Y != 0 {
			t.Errorf("%s: #h is at %v; it belongs at the top of the second column, "+
				"with the paragraph it may not be parted from", decl, got)
		}
	}
}

// TestABreakInAMarginBetweenTwoBoxesIsAvoided: the break between two boxes is
// anywhere in the gap between them, not only at the first one's bottom edge.
// #p's 40px top margin puts that gap from 60px to 100px, and an 80px column
// ends in the middle of it.
func TestABreakInAMarginBetweenTwoBoxesIsAvoided(t *testing.T) {
	const doc = `<div id="d"><div>a<br>b</div><div id="h">c</div><div id="p">d</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 80px }
		#p { margin-top: 40px }`
	if got := placed(t, doc, css, "h"); len(got) != 1 || got[0].X != 0 {
		t.Fatalf("without a declaration #h is at %v; the fixture needs it in the "+
			"first column", got)
	}
	for _, decl := range []string{"#h { break-after: avoid }", "#p { break-before: avoid }"} {
		got := placed(t, doc, css+decl, "h")
		if len(got) != 1 || got[0].X != upx(t, 100) || got[0].Y != 0 {
			t.Errorf("%s: #h is at %v; the cut in the margin is a break between #h "+
				"and #p, so the column ends before #h", decl, got)
		}
	}
}

// TestAnAvoidedBreakPropagatesThroughAParentsEdge is §3.1's propagation: the
// break after a last child is the break after its parent, so "break-after:
// avoid" on #h keeps #w's end from the start of the paragraph after #w.
func TestAnAvoidedBreakPropagatesThroughAParentsEdge(t *testing.T) {
	// #w's bottom padding puts 40px between #h and the paragraph after #w, and
	// an 80px column ends in the middle of it: that cut is after #h and before
	// the paragraph, so it is the break "break-after: avoid" on #h forbids.
	const doc = `<div id="d"><div id="w" style="padding-bottom:40px"><div>a<br>b</div>` +
		`<div id="h">c</div></div><div>d</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 80px }`
	if got := placed(t, doc, css, "h"); len(got) != 1 || got[0].X != 0 {
		t.Fatalf("without a declaration #h is at %v; the fixture needs it in the "+
			"first column", got)
	}
	got := placed(t, doc, css+`#h { break-after: avoid }`, "h")
	if len(got) != 1 || got[0].X != upx(t, 100) || got[0].Y != 0 {
		t.Errorf("#h is at %v; the break after it runs to the paragraph after #w, "+
			"so the column ends before #h", got)
	}
	// And forwards: "break-before" on a first child is the break before its
	// parent, and runs back through the parent's top padding to the box before
	// it. Without it the column ends 20px into #w's padding and #h is 20px down
	// the second column; with it the column ends before "c" and #h is 60px down.
	const doc2 = `<div id="d"><div>a<br>b<br>c</div><div id="w" style="padding-top:40px">` +
		`<div id="h">d</div></div></div>`
	if got := placed(t, doc2, css, "h"); len(got) != 1 || got[0].Y != upx(t, 20) {
		t.Fatalf("without a declaration #h is at %v; the fixture needs it 20px down "+
			"the second column", got)
	}
	got = placed(t, doc2, css+`#h { break-before: avoid }`, "h")
	if len(got) != 1 || got[0].X != upx(t, 100) || got[0].Y != upx(t, 60) {
		t.Errorf("#h is at %v; the break before #w is the break before #h, so the "+
			"column ends after \"b\" and #h is 60px down the second", got)
	}
}

// TestAnAvoidIsRelaxedWhenNothingElseFits is §4.4: "avoid" is the first rule to
// give way when there are not enough breaks that satisfy it. A box taller than
// the column cannot be kept whole, and holding to the declaration would lose
// content; it is broken, and every line is still drawn.
func TestAnAvoidIsRelaxedWhenNothingElseFits(t *testing.T) {
	const doc = `<div id="d"><div id="k">a<br>b<br>c<br>d<br>e</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 60px }
		#k { break-inside: avoid }`
	if got := placed(t, doc, css, "k"); len(got) != 2 {
		t.Errorf("#k is in %d pieces; taller than a column, it has to be broken", len(got))
	}
	if got := columnLines(t, doc, css); len(got) != 5 {
		t.Errorf("%d lines were drawn, want all five", len(got))
	}
}

// TestAForcedBreakIsReportedWhereItIsDeclared: the values that ask for a page
// break are not made anywhere — a document is not broken into pages — and they
// are said to be missing where they are written; so is "always" outside a
// multicol container, where it is a page break. "avoid" is honoured and is not
// reported. The column breaks a multicol container makes, and the ones it
// cannot, are multicolforced_test.go's.
func TestAForcedBreakIsReportedWhereItIsDeclared(t *testing.T) {
	findingsAbout := func(css string) []Finding {
		got := Compose(Input{HTML: `<div id="d"><p id="p">x</p></div>`,
			CSS: []Stylesheet{{Source: css}}}, Options{})
		var out []Finding
		for _, f := range got.Findings {
			if strings.Contains(f.Property, "break") {
				out = append(out, f)
			}
		}
		return out
	}
	for _, decl := range []string{"break-after: page",
		"break-before: always", "page-break-before: always", "page-break-after: left"} {
		if got := findingsAbout(`#p { ` + decl + ` }`); len(got) != 1 {
			t.Errorf("%s raised %d findings, want 1: %v", decl, len(got), got)
		}
	}
	for _, decl := range []string{"break-before: avoid", "break-after: avoid-column",
		"break-inside: avoid", "page-break-inside: avoid", "break-before: auto"} {
		if got := findingsAbout(`#p { ` + decl + ` }`); len(got) != 0 {
			t.Errorf("%s raised %v; it is honoured", decl, got)
		}
	}
}

// TestTheEndOfTheContentIsAlwaysABreak: a "break-after: avoid" on the last box
// asks about a break after it, inside the container or out of it, and none of
// them is where the content ends — the last column ends there whatever was
// asked. Forbidding that too left the balancing not knowing how far the content
// reached, and the pour was refused.
func TestTheEndOfTheContentIsAlwaysABreak(t *testing.T) {
	const doc = `<div id="d"><div>a<br>b<br>c</div><div id="z">d</div></div>`
	css := colCSS + `#d { column-count: 2 } #z { break-after: avoid }`
	lines := columnLines(t, doc, css)
	if len(lines) != 4 || lines[1].X != 0 || lines[2].X == 0 {
		t.Errorf("the lines are at %v; four lines balance at two and two", lines)
	}
}

// TestAvoidZonesAreLinearInTheContent is the cost of the three questions the
// pour asks: the zones collected from the laid-out content, the breakpoints
// they leave, and where a column may end, asked once per column. Every box may
// ask to be kept whole, so none of it may be the breakpoints times the boxes.
// Measured on the machinery alone, over content laid out once, because at these
// sizes the layout around it would hide a quadratic term; the boxes are held
// apart by a margin so that their zones stay separate rather than merging into
// one.
func TestAvoidZonesAreLinearInTheContent(t *testing.T) {
	content := func(n int) *Fragment {
		doc := `<div id="d">` + strings.Repeat(`<div class="k">a<br>b<br>c</div>`, n) + `</div>`
		return find(t, layoutOf(t, 400, doc, colCSS+`.k { break-inside: avoid; margin-bottom: 10px }`), "d")
	}
	ask := func(f *Fragment) func() {
		return func() {
			breaks := sortedBreaks(columnBreaks(f, 0, nil))
			z := avoidZonesOf(f)
			allowed := z.allowed(breaks, nil)
			step := upx(t, 50)
			for start := style.Unit(0); start < allowed[len(allowed)-1]; start = start.Add(step) {
				if z.forbids(start.Add(step)) {
					z.lastAllowed(start, start.Add(step))
				}
			}
		}
	}
	small, large := content(500), content(2000)
	c := costtest.Time(t, "the avoid zones of n boxes", ask(small), ask(large))
	if c.Ratio > 8 {
		t.Errorf("four times the boxes took %.1f times as long (%v against %v); "+
			"linear is about four", c.Ratio, c.Large, c.Small)
	}
}
