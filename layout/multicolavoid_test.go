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

// TestAvoidZonesAreLinearInTheContent is the cost of the questions the pour
// asks of the zones: the breakpoints they leave, and where a column may end,
// asked once per column. Every box may ask to be kept whole, so none of it may
// be the breakpoints times the boxes. Planted, a zone looked for by walking the
// zones rather than by a search reads as 11.5: a walk of a short list costs a
// step what a search costs, and the plants were 11.4 and 9.9 when this test was
// written at twice the size.
//
// The breakpoints and the zones are collected from content laid out once,
// before anything is timed, and the questions are timed on their own. They
// were timed with the collection, which walks the laid-out content: the walk
// was most of the time, and the fragment tree of two thousand boxes is
// megabytes where the zones are kilobytes. The collection is one walk of the
// content, and a fixture of sibling boxes one level deep could not show it
// walking anything twice; what this fixture can show is the questions.
//
// Two hundred and fifty boxes and a thousand, where it was five hundred and two
// thousand, which read 8.2 on a GitHub runner and 8.6 here with nothing else
// running. The questions are binary searches asked in order down the content,
// and a processor learns the branches of a search over a few hundred zones and
// not over a few thousand: each search took 12ns at a thousand zones and 23ns at
// two thousand, although it made one more comparison. That is linear work
// read as eight by the branch predictor, where the sizes fall and not what
// the code does. A thousand zones are sixteen kilobytes, and a search over them
// costs what one over two hundred and fifty does. The boxes are held apart by
// a margin so that their zones stay separate rather than merging into one.
func TestAvoidZonesAreLinearInTheContent(t *testing.T) {
	content := func(n int) *Fragment {
		doc := `<div id="d">` + strings.Repeat(`<div class="k">a<br>b<br>c</div>`, n) + `</div>`
		return find(t, layoutOf(t, 400, doc, colCSS+`.k { break-inside: avoid; margin-bottom: 10px }`), "d")
	}
	ask := func(n int) func() {
		f := content(n)
		breaks := sortedBreaks(columnBreaks(f, 0, nil))
		z := avoidZonesOf(f, func(b *Box) bool { _, ok := columnCount(b); return ok })
		if z == nil || len(z.zones) != n {
			t.Fatalf("%d boxes asking not to be broken made zones %v; want %d", n, z, n)
		}
		step := upx(t, 50)
		return func() {
			allowed := z.allowed(breaks, nil)
			for start := style.Unit(0); start < allowed[len(allowed)-1]; start = start.Add(step) {
				if z.forbids(start.Add(step)) {
					z.lastAllowed(start, start.Add(step))
				}
			}
		}
	}
	c := costtest.Time(t, "the avoid zones of n boxes", ask(250), ask(1000))
	if c.Ratio > 8 {
		t.Errorf("four times the boxes took %.1f times as long (%v against %v); "+
			"linear is about four", c.Ratio, c.Large, c.Small)
	}
}

// TestAnAvoidInsideANestedMulticolStaysInside is §3.1.1's "This propagation
// stops before it breaks through the nearest matching fragmentation context",
// for the avoid values as the forced ones already had it. A multicol container
// inside another is the nearest context for its own content.
//
// Three lines fill the outer 60px column exactly, so it ends between them and
// the inner container. "break-before: avoid" on the inner container's first
// child is a break before that child in the inner flow, and does not reach out
// through the inner container to forbid the outer one ending there. It did:
// the walk went through the inner container as through a block, and the
// outer column ended a line early.
func TestAnAvoidInsideANestedMulticolStaysInside(t *testing.T) {
	const doc = `<div id="d"><div id="c">a<br>b<br>c</div><div id="in">` +
		`<div id="h">d</div><div>e</div></div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 60px }
		#in { column-count: 2 }`
	wantPlaced(t, "without the declaration", doc, css, "in", [2]float64{100, 0})
	wantPlaced(t, "an avoid on the inner first child", doc, css+`#h { break-before: avoid }`,
		"in", [2]float64{100, 0})
	wantPlaced(t, "the lines before it", doc, css+`#h { break-before: avoid }`,
		"c", [2]float64{0, 0})
	// The inner container's own value is the outer flow's, and is honoured.
	wantPlaced(t, "an avoid on the inner container", doc, css+`#in { break-before: avoid }`,
		"in", [2]float64{100, 20})
}

// TestAnAvoidBetweenInnerColumnsIsNotAHeight: the inner container's children
// are already poured when the outer walk sees them, and two siblings the inner
// pour put in different columns have no height between them. "break-after:
// avoid" on #x, which a forced break ends the inner first column after, drew a
// zone from #x's foot to #y's head — from 40px in one inner column to 0 in the
// next, which came out as a zone at 40px. That forbade the outer column ending
// at the inner container's own bottom, and the outer pour cut the inner
// container in half instead, a line down.
func TestAnAvoidBetweenInnerColumnsIsNotAHeight(t *testing.T) {
	const doc = `<div id="d"><div id="in"><div id="x">a<br>b</div>` +
		`<div id="y">c<br>d</div></div><div id="z">e</div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 40px }
		#in { column-count: 2 } #y { break-before: column }`
	wantPlaced(t, "without the declaration", doc, css, "z", [2]float64{100, 0})
	wantPlaced(t, "with it", doc, css+`#x { break-after: avoid }`, "z", [2]float64{100, 0})
	wantPlaced(t, "the inner container", doc, css+`#x { break-after: avoid }`,
		"in", [2]float64{0, 0})
}

// TestABreakInsideAvoidInsideANestedMulticolIsHonoured is the half of the
// nested content the outer walk still reads. An outer column that ends through
// the inner container breaks what the inner columns hold there (§2.2), and a
// box that asked "break-inside: avoid" asked it of every break. #k fills the
// inner first column from 40px to 100px, and the outer 60px column would end
// through it; with the declaration the outer column ends before the inner
// container instead.
func TestABreakInsideAvoidInsideANestedMulticolIsHonoured(t *testing.T) {
	const doc = `<div id="d"><div>q<br>r</div><div id="in"><div id="k">a<br>b<br>c</div>` +
		`<div>d<br>e<br>f</div></div></div>`
	css := colCSS + `#d { column-count: 2; column-fill: auto; height: 60px }
		#in { column-count: 2 }`
	if got := placed(t, doc, css, "in"); len(got) != 2 {
		t.Fatalf("without the declaration the inner container is in %d pieces at %v; the "+
			"fixture needs the outer column to end through it", len(got), got)
	}
	wantPlaced(t, "with it", doc, css+`#k { break-inside: avoid }`, "in", [2]float64{100, 0})
}
