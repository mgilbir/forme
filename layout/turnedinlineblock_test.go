package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Inline-blocks and the turn: a vertical inline-block on a horizontal line —
// CSS Writing Modes §7.3's orthogonal flow — and an inline-block on a vertical
// line in the same mode as the line.
//
// The face is VerticalFallbacks.ttf again (see mixedorientation_test.go): at
// 20px "o" and "x" are 10 wide lying along a line, and a 25px line holds the
// face with no leading, its alphabetic baseline 20 in from the line's over
// edge.

// turnedLayout lays a document out in the fixture face on a 400 by 1000 page.
func turnedLayout(t *testing.T, htmlSrc, cssSrc string) (*Fragment, []DrawText, []Finding) {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(1000)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: w, H: h}, verticalFallbacksSet(t), rec)
	var runs []DrawText
	for _, op := range Paint(root) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			runs = append(runs, v)
		}
	}
	return root, runs, append(append([]Finding(nil), built.Findings...), rec.Findings()...)
}

const flatCSS = `body { margin: 0 }
	#p { font-family: V; font-size: 20px; line-height: 25px; white-space: pre }`

func rectInPx(r Rect) [4]float64 {
	return [4]float64{r.X.Px(), r.Y.Px(), r.W.Px(), r.H.Px()}
}

func writingModeFindings(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		if f.Property == "writing-mode" {
			out = append(out, f.Message)
		}
	}
	return out
}

// TestAVerticalInlineBlockIsAsWideAsItsLinesStack.
//
// A vertical-rl inline-block 60px tall on a horizontal line, holding two lines
// of 40 and 20: it is 50 wide, two lines of 25 stacked from its right edge,
// and the "oo" after it starts 50 after where it does. It has no baseline
// across the page, so it sits on its bottom margin edge (§4.3): 60 tall above
// the line's baseline, which is where the text beside it is drawn.
func TestAVerticalInlineBlockIsAsWideAsItsLinesStack(t *testing.T) {
	root, runs, fs := turnedLayout(t,
		"<div id=\"p\">oo<span id=\"ib\">oooo\noo</span>oo</div>",
		flatCSS+`
	#ib { display: inline-block; writing-mode: vertical-rl; height: 60px }`)
	if said := writingModeFindings(fs); len(said) > 0 {
		t.Fatalf("the inline-block was reported: %q", said)
	}
	if got, want := rectInPx(find(t, root, "ib").BorderRect), [4]float64{20, 0, 50, 60}; got != want {
		t.Errorf("the inline-block is at %v, want %v", got, want)
	}
	// In any order: an atomic inline is painted after the line it is on.
	want := map[turnedRunAt]bool{
		{false, 0, 60}:  true, // the "oo" before, on the line's baseline
		{true, 50, 0}:   true, // the inline-block's first line, 20 in from its right edge
		{true, 25, 0}:   true, // its second line, a line height to the left
		{false, 70, 60}: true, // the "oo" after it
	}
	checkRunsAt(t, runs, want)
}

// turnedRunAt is where a run is drawn and which way it goes.
type turnedRunAt struct {
	sideways bool
	x, y     float64
}

// checkRunsAt asserts that the runs are drawn at exactly the places given.
func checkRunsAt(t *testing.T, runs []DrawText, want map[turnedRunAt]bool) {
	t.Helper()
	got := map[turnedRunAt]bool{}
	for _, r := range runs {
		got[turnedRunAt{r.Sideways, r.At.X.Px(), r.At.Y.Px()}] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("no run is drawn sideways=%v at (%g, %g); the runs are at %v",
				w.sideways, w.x, w.y, got)
		}
	}
	if len(runs) != len(want) {
		t.Errorf("drew %d runs, want %d", len(runs), len(want))
	}
}

// TestAVerticalInlineBlockWithNoHeightFitsItsContainingBlock.
//
// With no height the inline-block's lines are broken against §7.3.2's
// constraint: the smallest of its containing block's height, where that is
// definite, and the page's, less its own margins, borders and padding on the
// sides its lines run between. The containing block is 100px tall and the
// inline-block has 10px margins, so its lines are 80 long: four words of 40,
// a space too many to put two on a line, stack four lines of 25 — 100 wide.
// Against the page whole it would be one line; against the 100 without the
// margins, two.
func TestAVerticalInlineBlockWithNoHeightFitsItsContainingBlock(t *testing.T) {
	root, _, fs := turnedLayout(t,
		`<div id="p"><span id="ib">oooo oooo oooo oooo</span></div>`,
		`body { margin: 0 }
	#p { font-family: V; font-size: 20px; line-height: 25px; height: 100px }
	#ib { display: inline-block; writing-mode: vertical-rl; margin: 10px }`)
	if said := writingModeFindings(fs); len(said) > 0 {
		t.Fatalf("the inline-block was reported: %q", said)
	}
	ib := find(t, root, "ib").BorderRect
	if ib.W.Px() != 100 || ib.H.Px() != 80 {
		t.Errorf("the inline-block is %gx%g, want 100x80", ib.W.Px(), ib.H.Px())
	}
}

// TestAnInlineBlockOnAVerticalLineIsTurnedWithIt.
//
// An inline-block in the vertical box's own writing mode is laid out on the
// turned line like any run: 20 along the line for its two letters, a line
// high across it, and the turn takes it with the line. Its "margin-top" is the
// top of the page, which is along the line, so it moves the inline-block and
// everything after it 5 down the column.
func TestAnInlineBlockOnAVerticalLineIsTurnedWithIt(t *testing.T) {
	for _, c := range []struct {
		css string
		top float64
	}{
		{"", 20},
		{"#ib { margin-top: 5px }", 25},
	} {
		root, runs, fs := turnedLayout(t,
			`<div id="d">oo<span id="ib">xx</span>oo</div>`,
			mixedCSS+`
	#ib { display: inline-block } `+c.css)
		if said := writingModeFindings(fs); len(said) > 0 {
			t.Fatalf("%q: the box was reported: %q", c.css, said)
		}
		if got, want := rectInPx(find(t, root, "ib").BorderRect),
			[4]float64{25, c.top, 25, 20}; got != want {
			t.Errorf("%q: the inline-block is at %v, want %v", c.css, got, want)
		}
		checkRunsAt(t, runs, map[turnedRunAt]bool{
			{true, 30, 0}: true, {true, 30, c.top}: true, {true, 30, c.top + 20}: true,
		})
	}
}

// TestTheInlineBlocksThisEngineCannotTurnAreReported.
//
// What is still refused, and says so: a declared width inside the turn (a
// side of the page is not a side of the text, as for any box there); a
// vertical inline-block inside something that shrinks to fit, which would
// measure it along the wrong axis; an inline flex container, whose sizing is
// its own.
func TestTheInlineBlocksThisEngineCannotTurnAreReported(t *testing.T) {
	for _, c := range []struct{ what, html, css, names string }{
		{"a width inside the turn",
			`<div id="d">oo<span id="ib">xx</span></div>`,
			mixedCSS + ` #ib { display: inline-block; width: 10px }`,
			`"width" is declared inside it`},
		{"a vertical inline-block in a float",
			`<div id="p"><div style="float: left"><span id="ib">oo</span></div></div>`,
			flatCSS + ` #ib { display: inline-block; writing-mode: vertical-rl; height: 60px }`,
			"shrinking a box around its content"},
		{"a vertical inline flex container",
			`<div id="p"><span id="ib">oo</span></div>`,
			flatCSS + ` #ib { display: inline-flex; writing-mode: vertical-rl; height: 60px }`,
			"not an ordinary block box"},
	} {
		_, _, fs := turnedLayout(t, c.html, c.css)
		said := writingModeFindings(fs)
		if len(said) == 0 || !strings.Contains(said[0], c.names) {
			t.Errorf("%s was reported as %q, want a finding naming %q", c.what, said, c.names)
		}
	}
	// And one that is not refused: the inline box between a vertical
	// inline-block and its block container has no width to measure.
	_, _, fs := turnedLayout(t,
		`<div id="p"><span><span id="ib">oo</span></span></div>`,
		flatCSS+` #ib { display: inline-block; writing-mode: vertical-rl; height: 60px }`)
	if said := writingModeFindings(fs); len(said) > 0 {
		t.Errorf("a vertical inline-block inside a span was reported: %q", said)
	}
}

// TestAHorizontalInlineBlockStandsOnAVerticalLine.
//
// An inline-block in horizontal-tb on a vertical-rl line: its "xx" is set
// across the page, 20 wide and a line of 25 high, and the vertical line holds
// it as a box 25 along and 20 across. Along the line it follows the "oo" at
// 20, and the "oo" after it starts at 45. Across it is centred on the line's
// central baseline, 37.5, so it spans 27.5 to 47.5; its text runs across it
// from its left edge on its own baseline, 20 below its top. A margin-top of 5,
// which is along the line, moves it and what follows 5 down the column.
func TestAHorizontalInlineBlockStandsOnAVerticalLine(t *testing.T) {
	for _, c := range []struct {
		css string
		top float64
	}{
		{"", 20},
		{"#ib { margin-top: 5px }", 25},
	} {
		root, runs, fs := turnedLayout(t,
			`<div id="d">oo<span id="ib">xx</span>oo</div>`,
			mixedCSS+`
	#ib { display: inline-block; writing-mode: horizontal-tb } `+c.css)
		if said := writingModeFindings(fs); len(said) > 0 {
			t.Fatalf("%q: the box was reported: %q", c.css, said)
		}
		if got, want := rectInPx(find(t, root, "ib").BorderRect),
			[4]float64{27.5, c.top, 20, 25}; got != want {
			t.Errorf("%q: the inline-block is at %v, want %v", c.css, got, want)
		}
		checkRunsAt(t, runs, map[turnedRunAt]bool{
			{true, 30, 0}:             true, // the "oo" before, lying along the line
			{false, 27.5, c.top + 20}: true, // the "xx", across the page
			{true, 30, c.top + 25}:    true, // the "oo" after
		})
	}
	// Under "text-orientation: sideways" the alphabetic baseline is the
	// dominant one, and the inline-block sits on its under margin edge: its
	// left edge, on the vertical-rl line, is the line's alphabetic baseline,
	// 30 across the page.
	root, _, _ := turnedLayout(t, `<div id="d">oo<span id="ib">xx</span>oo</div>`,
		mixedCSS+` #d { text-orientation: sideways }
	#ib { display: inline-block; writing-mode: horizontal-tb }`)
	if got := find(t, root, "ib").BorderRect.X.Px(); got != 30 {
		t.Errorf("under sideways the inline-block's left edge is at %g, want 30", got)
	}
}

// TestAHorizontalInlineBlockThatCannotStandIsReported: a percentage size, of
// a physical width no one knows on a vertical line, and a box inside it that
// turns again.
func TestAHorizontalInlineBlockThatCannotStandIsReported(t *testing.T) {
	for _, c := range []struct{ what, html, css, names string }{
		{"a percentage width",
			`<div id="d">oo<span id="ib">xx</span></div>`,
			mixedCSS + ` #ib { display: inline-block; writing-mode: horizontal-tb; width: 50% }`,
			`a percentage "width"`},
		{"a vertical box inside it",
			`<div id="d">oo<span id="ib">x<span style="display: inline-block; writing-mode: vertical-lr">x</span></span></div>`,
			mixedCSS + ` #ib { display: inline-block; writing-mode: horizontal-tb }`,
			"changes the writing mode again"},
	} {
		_, _, fs := turnedLayout(t, c.html, c.css)
		said := writingModeFindings(fs)
		if len(said) == 0 || !strings.Contains(said[0], c.names) {
			t.Errorf("%s was reported as %q, want a finding naming %q", c.what, said, c.names)
		}
	}
}

// TestAHorizontalInlineBlockIsSizedAcrossThePage.
//
// Its width is shrink-to-fit against §7.3.1's fallback, the initial containing
// block's width, 400 here, and not against the vertical line it stands on,
// which is 100 long: "ooooo ooooo" is 104.59375 across — ten letters of 10
// and the fixture's space of 230 units, in 64ths — and fits on one of its own
// lines. And where the vertical box is refused for something else — a float in
// it — the inline-block is an ordinary one on an ordinary line, and is not
// stood on its side.
func TestAHorizontalInlineBlockIsSizedAcrossThePage(t *testing.T) {
	root, _, fs := turnedLayout(t,
		`<div id="d"><span id="ib">ooooo ooooo</span></div>`,
		mixedCSS+` #d { height: 100px }
	#ib { display: inline-block; writing-mode: horizontal-tb }`)
	if said := writingModeFindings(fs); len(said) > 0 {
		t.Fatalf("the box was reported: %q", said)
	}
	ib := find(t, root, "ib").BorderRect
	if ib.W.Px() != 104.59375 || ib.H.Px() != 25 {
		t.Errorf("the inline-block is %gx%g, want 104.59375x25 — one line across the page",
			ib.W.Px(), ib.H.Px())
	}
	root, _, fs = turnedLayout(t,
		`<div id="d">oo<span id="ib">xx</span><span style="float: left">f</span></div>`,
		mixedCSS+` #ib { display: inline-block; writing-mode: horizontal-tb }`)
	if len(writingModeFindings(fs)) == 0 {
		t.Fatal("the control: a vertical box holding a float was not refused")
	}
	if got := rectInPx(find(t, root, "ib").BorderRect); got[2] != 20 || got[3] != 25 {
		t.Errorf("in a refused box the inline-block is %v, want 20 wide and 25 high", got)
	}
}

// TestTheTextOfAHorizontalInlineBlockIsNotOnAVerticalLine.
//
// Its text runs across the page, so nothing in it is set the way a vertical
// line sets its characters: an ideograph does not stand upright with an em of
// advance down the page, and a composition is not made.
func TestTheTextOfAHorizontalInlineBlockIsNotOnAVerticalLine(t *testing.T) {
	_, runs, _ := turnedLayout(t,
		`<div id="d">oo<span id="ib">x日<span style="text-combine-upright: all">oo</span></span></div>`,
		mixedCSS+` #ib { display: inline-block; writing-mode: horizontal-tb }`)
	inside := 0
	for _, r := range runs {
		if r.Sideways {
			continue
		}
		inside++
		if r.Upright || r.WidthScale != 0 {
			t.Errorf("the run %q in the horizontal inline-block is upright=%v squeezed %v",
				r.Text, r.Upright, r.WidthScale)
		}
	}
	if inside == 0 {
		t.Fatalf("nothing was drawn across the page: %+v", runs)
	}
}

// TestAHorizontalInlineBlockStandsOnEveryVerticalLine: the same inline-block
// in the other modes. In vertical-lr the first line is 0 to 25 across the page
// and its middle 12.5, so the box spans 2.5 to 22.5. sideways-rl and
// sideways-lr have the alphabetic baseline for their dominant one, so the box
// sits on its under margin edge: in sideways-rl that is the line's baseline,
// 20 in from the right at 30, the box reaching right from it; in sideways-lr,
// whose glyphs are turned the other way, the baseline is 20 in from the left
// and the box reaches left from it to 0. sideways-lr's line runs up the page,
// so the box is 20 to 45 up from the foot of the 400px column: 355 to 380.
func TestAHorizontalInlineBlockStandsOnEveryVerticalLine(t *testing.T) {
	for _, c := range []struct {
		mode string
		want [4]float64
	}{
		{"vertical-lr", [4]float64{2.5, 20, 20, 25}},
		{"sideways-rl", [4]float64{30, 20, 20, 25}},
		{"sideways-lr", [4]float64{0, 355, 20, 25}},
	} {
		root, _, fs := turnedLayout(t,
			`<div id="d">oo<span id="ib">xx</span>oo</div>`,
			mixedCSS+` #d { writing-mode: `+c.mode+` }
	#ib { display: inline-block; writing-mode: horizontal-tb }`)
		if said := writingModeFindings(fs); len(said) > 0 {
			t.Fatalf("%s: the box was reported: %q", c.mode, said)
		}
		if got := rectInPx(find(t, root, "ib").BorderRect); got != c.want {
			t.Errorf("%s: the inline-block is at %v, want %v", c.mode, got, c.want)
		}
	}
}
