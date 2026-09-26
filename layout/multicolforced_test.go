package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// CSS Fragmentation 3 §3.1 and §4.1: the breaks a multicol container's content
// forces. The fixture is colCSS's — Courier on a 20px line, a 200px container
// with no gap — so two columns are 100px apart and every column height is a
// whole number of lines, and each case is checked against the same document
// without the declaration, so that the fixture is known to put the columns
// somewhere else without it.

// pxUnit is a number of pixels as a length.
func pxUnit(t *testing.T, v float64) style.Unit {
	t.Helper()
	u, ok := style.FromPx(v)
	if !ok {
		t.Fatalf("%v px is not a length", v)
	}
	return u
}

func wantPlaced(t *testing.T, what, doc, css, id string, want ...[2]float64) {
	t.Helper()
	got := placed(t, doc, css, id)
	if len(got) != len(want) {
		t.Errorf("%s: #%s is in %d pieces at %v, want %d at %v", what, id, len(got), got,
			len(want), want)
		return
	}
	for i, w := range want {
		if got[i].X != pxUnit(t, w[0]) || got[i].Y != pxUnit(t, w[1]) {
			t.Errorf("%s: #%s piece %d is at (%g, %g), want (%g, %g)", what, id, i,
				got[i].X.Px(), got[i].Y.Px(), w[0], w[1])
		}
	}
}

// TestAForcedColumnBreakEndsTheColumn is css-multicol-1's own example, "p {
// break-after: column }", with as many paragraphs as columns: each paragraph
// begins a column. Four lines — one, then three — balance at two and two
// without it, which cuts the second paragraph; with it the first column holds
// the first paragraph alone, and the balancing makes the columns as tall as
// the second, three lines. The break after the last paragraph is at the end of
// the columns, where there is nothing to break to, and does nothing.
func TestAForcedColumnBreakEndsTheColumn(t *testing.T) {
	const doc = `<div id="d"><p id="a">a</p><p id="b">b<br>c<br>d</p></div>`
	css := colCSS + `#d { column-count: 2 } p { margin: 0 }`
	wantPlaced(t, "without the declaration", doc, css, "b", [2]float64{0, 20}, [2]float64{100, 0})
	for _, decl := range []string{"p { break-after: column }", "#a { break-after: column }",
		"#b { break-before: column }", "#b { break-before: always }",
		"#a { break-after: all }"} {
		wantPlaced(t, decl, doc, css+decl, "b", [2]float64{100, 0})
		if h := find(t, layoutOf(t, 400, doc, css+decl), "d").ContentRect().H.Px(); h != 60 {
			t.Errorf("%s: the container is %gpx tall, want the three lines of the second "+
				"column, 60", decl, h)
		}
	}
}

// TestAForcedBreakPropagatesToTheBoxItBegins is §3.1's propagation: a
// "break-before" on a first in-flow child is its parent's, and a "break-after"
// on a last child too. The break before the first paragraph of a section is
// the break before the section, and the section begins the column; the break
// after the last paragraph of one is the break after it.
func TestAForcedBreakPropagatesToTheBoxItBegins(t *testing.T) {
	css := colCSS + `#d { column-count: 2 } p { margin: 0 }`
	// One line, then a section of a paragraph and two lines: four lines, two
	// and two without the break.
	before := `<div id="d"><div>a</div><div id="s"><p id="p">b</p>c<br>d</div></div>`
	wantPlaced(t, "without the declaration", before, css, "s", [2]float64{0, 20}, [2]float64{100, 0})
	wantPlaced(t, "break-before on a first child", before, css+`#p { break-before: column }`,
		"s", [2]float64{100, 0})

	// A section of two paragraphs, the second breaking after itself, then
	// three lines: five lines, three and two without it.
	after := `<div id="d"><div><p>a</p><p id="p">b</p></div><div id="n">c<br>d<br>e</div></div>`
	wantPlaced(t, "without the declaration", after, css, "n", [2]float64{0, 40}, [2]float64{100, 0})
	wantPlaced(t, "break-after on a last child", after, css+`#p { break-after: column }`,
		"n", [2]float64{100, 0})
}

// TestAForcedBreakAtTheEdgeOfTheColumnsDoesNothing: the propagation "stops
// before it breaks through the nearest matching fragmentation context", so a
// break before the container's first box or after its last is at the start or
// the end of the columns, and there is no column to break from or to. The
// content balances as it would without it, and nothing is reported.
func TestAForcedBreakAtTheEdgeOfTheColumnsDoesNothing(t *testing.T) {
	const doc = `<div id="d"><p id="a">a<br>b</p><p id="b">c<br>d</p></div>`
	css := colCSS + `#d { column-count: 2 } p { margin: 0 }`
	for _, decl := range []string{"", "#a { break-before: column }", "#b { break-after: column }",
		"#a { break-before: always } #b { break-after: always }"} {
		wantPlaced(t, decl, doc, css+decl, "b", [2]float64{100, 0})
		if got := breakFindings(doc, css+decl); len(got) != 0 {
			t.Errorf("%s: %v; a break at the edge of the columns does what CSS says", decl, got)
		}
	}
}

// TestBalancingRespectsAForcedBreak is §3.5's balance with a forced break in
// the content: the columns are the shortest that hold it with a column ending
// at the break. One line, a break, and four lines in three columns: the first
// column holds the one line, and the four share the other two, two each — the
// columns are 40 tall. Balanced as though the break were not there they would
// be 40 too, but the first column would hold two lines and the break would be
// lost; and balanced over the breakpoints alone, as it was, 40 is a breakpoint
// only by luck — see the next case.
func TestBalancingRespectsAForcedBreak(t *testing.T) {
	const doc = `<div id="d"><p id="a">a</p><p id="b">b<br>c<br>d<br>e</p></div>`
	css := colCSS + `#d { column-count: 3; width: 300px } p { margin: 0 } #a { break-after: column }`
	wantPlaced(t, "one line, a break, four lines", doc, css, "b",
		[2]float64{100, 0}, [2]float64{200, 0})
	if h := find(t, layoutOf(t, 400, doc, css), "d").ContentRect().H.Px(); h != 40 {
		t.Errorf("the container is %gpx tall, want 40", h)
	}

	// Columns whose heights are not breakpoints. A 30px box, a break, then a
	// line and a 50px box: the second column needs 70, which is no
	// breakpoint's height — the breakpoints are at 30, 50 and 100. The first
	// one the content fitted under was 100, and the first column is then as
	// tall as the whole content: 100 where 70 holds it.
	const odd = `<div id="d"><div id="x" style="height: 30px"></div>` +
		`<p id="y">a</p><div id="z" style="height: 50px"></div></div>`
	oddCSS := colCSS + `#d { column-count: 2 } p { margin: 0 } #x { break-after: column }`
	wantPlaced(t, "a column that begins at a break", odd, oddCSS, "y", [2]float64{100, 0})
	if h := find(t, layoutOf(t, 400, odd, oddCSS), "d").ContentRect().H.Px(); h != 70 {
		t.Errorf("the container is %gpx tall, want the second column's 70", h)
	}
}

// TestABalancedColumnEndsWhereTheBalancingCountedIt: a 70px box, a break, and
// five lines in three columns. The first column is the box, and the balancing
// makes every column 70 tall: the five lines are three and two. The second
// column begins at 70, and 70 below that is the middle of the fourth line; it
// ends at the last line that fits, 60 down, which is where the balancing
// counted it ending. Cut at exactly a column height it went through the line,
// and the pour was refused.
func TestABalancedColumnEndsWhereTheBalancingCountedIt(t *testing.T) {
	const doc = `<div id="d"><div id="x" style="height: 70px"></div>` +
		`<p id="y">a<br>b<br>c<br>d<br>e</p></div>`
	css := colCSS + `#d { column-count: 3; width: 300px } p { margin: 0 } #x { break-after: column }`
	wantPlaced(t, "a box, a break, five lines", doc, css, "y",
		[2]float64{100, 0}, [2]float64{200, 0})
	if h := find(t, layoutOf(t, 400, doc, css), "d").ContentRect().H.Px(); h != 70 {
		t.Errorf("the container is %gpx tall, want 70", h)
	}
}

// TestAForcedBreakOverridesAnAvoid is §4.1: "a forced break value effectively
// overrides any avoid break value that also applies at that break point" — and
// §4.4 relaxes the rules for unforced breaks, never a forced one. The break
// after the first paragraph is made although the second asks not to be broken
// before, and although the box round both asks not to be broken inside.
func TestAForcedBreakOverridesAnAvoid(t *testing.T) {
	const doc = `<div id="d"><div id="k"><p id="a">a</p><p id="b">b<br>c<br>d</p></div></div>`
	css := colCSS + `#d { column-count: 2 } p { margin: 0 } #a { break-after: column }`
	for _, decl := range []string{"#b { break-before: avoid }", "#k { break-inside: avoid }",
		"#a { break-inside: avoid } #b { break-before: avoid-column }"} {
		wantPlaced(t, decl, doc, css+decl, "b", [2]float64{100, 0})
	}
}

// TestTheMarginAfterAForcedBreakIsKept is §5.2: "When a forced break occurs
// there, adjoining margins before the break are truncated, but margins after
// the break are preserved." The paragraph after the break begins its column 10px
// down, which is its own top margin; the 30px the paragraph before carried
// down stays behind.
func TestTheMarginAfterAForcedBreakIsKept(t *testing.T) {
	const doc = `<div id="d"><p id="a">a</p><p id="b">b<br>c</p></div>`
	css := colCSS + `#d { column-count: 2 } p { margin: 0 }
		#a { margin-bottom: 30px; break-after: column } #b { margin-top: 10px }`
	wantPlaced(t, "the margins at a forced break", doc, css, "b", [2]float64{100, 10})
}

// TestAForcedBreakInAColumnOfStatedHeight is column-fill: auto with a height:
// the columns are filled in turn, each as tall as the container, and a forced
// break ends one early. Two lines, a break, and six lines in 80px columns: two
// in the first, four in the second, two in the third.
func TestAForcedBreakInAColumnOfStatedHeight(t *testing.T) {
	const doc = `<div id="d"><p id="a">a<br>b</p><p id="b">c<br>d<br>e<br>f<br>g<br>h</p></div>`
	css := colCSS + `#d { column-count: 3; width: 300px; column-fill: auto; height: 80px }
		p { margin: 0 }`
	wantPlaced(t, "without the declaration", doc, css, "b", [2]float64{0, 40}, [2]float64{100, 0})
	wantPlaced(t, "a forced break", doc, css+`#a { break-after: column }`, "b",
		[2]float64{100, 0}, [2]float64{200, 0})
}

// breakFindings is the findings a document raises about the break properties.
func breakFindings(doc, css string) []Finding {
	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}}, Options{})
	var out []Finding
	for _, f := range got.Findings {
		if strings.Contains(f.Property, "break") ||
			strings.Contains(f.Message, "column break") {
			out = append(out, f)
		}
	}
	return out
}

// TestAForcedBreakNotMadeIsReported is what is still not made, and where it
// is said. A multicol container's forced breaks are made where its content is
// poured, and reported where they cannot be:
//
//   - on a float, or on a box inside one: not in the flow the columns divide,
//     and not moved to the next column;
//   - in a container whose content cannot be poured, which is reported whole;
//   - "always" and "all" outside a multicol container, which are page breaks,
//     and "all" inside one, whose page break is not made either.
//
// This test used to assert two more: that more forced breaks than columns,
// and a forced break in a container of one column, were refused and laid out
// in one column. Both need css-multicol-1 §8.2's overflow columns, which were
// not made; they are now, and TestForcedBreaksOverflowTheColumns holds them.
// What is left of those cases here is that they say nothing.
//
// "column" outside a multicol container has no effect by §3.1 — "if the flow
// is not within a multi-column context, they have no effect" — and says
// nothing: that is what CSS asks of it.
func TestAForcedBreakNotMadeIsReported(t *testing.T) {
	css := colCSS + `#d { column-count: 2 } p { margin: 0 }`
	three := `<div id="d"><p>a</p><p>b</p><p>c</p></div>`
	if got := breakFindings(three, css+`p { break-after: column }`); len(got) != 0 {
		t.Errorf("three paragraphs breaking after each in two columns: %v; the third "+
			"column is an overflow column, and made", got)
	}

	float := `<div id="d"><p>a</p><div id="f" style="float: left; width: 50px">` +
		`<p id="q">b</p><p id="r">c</p></div><p>d<br>e</p></div>`
	for _, decl := range []string{"#f { break-before: column }", "#r { break-before: column }"} {
		if got := breakFindings(float, css+decl); len(got) != 1 ||
			!strings.Contains(got[0].Message, "not in the flow") {
			t.Errorf("%s: %v, want it reported as not made", decl, got)
		}
	}

	// A block inside an inline-block is on a line of the flow, not in it.
	inline := `<div id="d"><p>a <span style="display: inline-block">` +
		`<span style="display: block; break-before: column">b</span>c</span></p><p>d</p></div>`
	if got := breakFindings(inline, css); len(got) != 1 ||
		!strings.Contains(got[0].Message, "not in the flow") {
		t.Errorf("a forced break inside an inline-block: %v, want it reported", got)
	}

	if got := breakFindings(`<div id="d"><p>a</p><p id="b">b</p></div>`,
		colCSS+`#d { column-count: 1 } #b { break-before: column }`); len(got) != 0 {
		t.Errorf("a forced break in one column: %v; its second column is an overflow "+
			"column, and made", got)
	}
	// Where the content cannot be poured, the container is reported, and the
	// break inside it is not reported again.
	refused := Compose(Input{HTML: `<div id="d"><p>a</p><p id="b">b</p>` +
		`<div style="position: relative">c</div></div>`, CSS: []Stylesheet{{Source: colCSS +
		`#d { column-count: 1 } #b { break-before: column }`}}}, Options{})
	var said []string
	for _, f := range refused.Findings {
		if strings.Contains(f.Property, "break") || f.Property == "column-count" {
			said = append(said, f.Message)
		}
	}
	if len(said) != 1 || !strings.Contains(said[0], "overflow columns") {
		t.Errorf("a forced break in one column holding a positioned box: %q, want the "+
			"container reported for the overflow columns it needs", said)
	}

	outside := `<div id="d"><p id="a">a</p><p id="b">b</p></div>`
	for decl, want := range map[string]int{
		"#b { break-before: column }": 0,
		"#b { break-before: always }": 1,
		"#b { break-after: all }":     1,
	} {
		if got := breakFindings(outside, colCSS+decl); len(got) != want {
			t.Errorf("%s outside a multicol container: %v, want %d", decl, got, want)
		}
	}
	if got := breakFindings(outside, css+`#a { break-after: all }`); len(got) != 1 ||
		!strings.Contains(got[0].Message, "the column break is made") {
		t.Errorf("break-after: all in a multicol container: %v, want the page break "+
			"reported and the column break said to be made", got)
	}
	// And the ones that are made say nothing.
	for _, decl := range []string{"#a { break-after: column }", "#b { break-before: always }"} {
		if got := breakFindings(outside, css+decl); len(got) != 0 {
			t.Errorf("%s in a multicol container: %v; it is made", decl, got)
		}
	}
}

// TestAForcedBreakInsideANestedMulticolIsItsOwn: the propagation stops at the
// nearest multi-column container, so a break inside an inner container is the
// inner one's, and does not end a column of the outer.
func TestAForcedBreakInsideANestedMulticolIsItsOwn(t *testing.T) {
	const doc = `<div id="d"><div id="in"><p id="a">a</p><p>b</p></div><p id="c">c<br>d</p></div>`
	css := colCSS + `#d { column-count: 2 } p { margin: 0 } #in { column-count: 2 }`
	plain := placed(t, doc, css, "c")
	withBreak := placed(t, doc, css+`#a { break-after: column }`, "c")
	if len(plain) != len(withBreak) || len(plain) == 0 || plain[0] != withBreak[0] {
		t.Errorf("#c is at %v without the inner break and %v with it; the inner "+
			"container's break is not the outer's", plain, withBreak)
	}
}

// TestForcedBreaksPourInLinearTime: n paragraphs of a line each, each breaking
// after itself, into as many columns — n forced breaks, n columns. The breaks
// are found in one walk of the content, the balancing walks them beside the
// breakpoints rather than searching for each, and each column is cut once.
func TestForcedBreaksPourInLinearTime(t *testing.T) {
	doc := func(n int) Built {
		return Build(Input{HTML: `<div style="column-count:1000000;width:600px">` +
			strings.Repeat(`<p style="margin:0;break-after:column">w</p>`, n) + `</div>`})
	}
	small, large := doc(500), doc(2000)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(100000)
	var smallLines, largeLines int
	c := costtest.Time(t, "pouring n forced breaks", func() {
		smallLines = pouredLines(Layout(small.Root, Size{W: w, H: h}, nil, nil))
	}, func() {
		largeLines = pouredLines(Layout(large.Root, Size{W: w, H: h}, nil, nil))
	})
	if smallLines != 500 || largeLines != 2000 {
		t.Fatalf("%d and %d lines were poured; the fixture is meant to pour 500 and 2000",
			smallLines, largeLines)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the breaks took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}

// TestBalancingWithForcedBreaksIsLinear is the balancing alone, over n
// breakpoints and a forced break at every other one, into as many columns as
// that takes: each fit walks the forced breaks beside the breakpoints, once.
// Planted, a fit asking whether each breakpoint is among the forced breaks by
// looking through them reads as 17.9.
//
// A thousand breakpoints and four thousand, and not four thousand and sixteen
// thousand. The larger pair is 190 kilobytes at the larger size and several
// times that under the race detector, which keeps a shadow of every word the
// test touches, and the race job on a GitHub runner read the linear walk over
// it as 8.3: a cache that held the smaller input and not the larger. At these
// sizes both are a few tens of kilobytes, which every cache holds.
func TestBalancingWithForcedBreaksIsLinear(t *testing.T) {
	content := func(n int) ([]style.Unit, []style.Unit) {
		breaks := make([]style.Unit, n)
		var forced []style.Unit
		for i := range breaks {
			breaks[i] = style.Unit((i + 1) * 64)
			if i%2 == 1 {
				forced = append(forced, breaks[i])
			}
		}
		return breaks, forced
	}
	sb, sf := content(1000)
	lb, lf := content(4000)
	var hs, hl style.Unit
	c := costtest.Time(t, "balancing n breakpoints with forced breaks",
		func() { hs, _ = balancedHeight(sb, sf, len(sf)+1) },
		func() { hl, _ = balancedHeight(lb, lf, len(lf)+1) })
	// Every column is two lines: the answer, and not only a cost.
	if hs != 128 || hl != 128 {
		t.Fatalf("the balanced heights are %d and %d, want two lines, 128", hs, hl)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the breakpoints took %.1f times as long (%v against %v)",
			c.Ratio, c.Large, c.Small)
	}
}
