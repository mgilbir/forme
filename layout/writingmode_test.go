package layout

import (
	"strings"
	"testing"
)

// A block turned on its side, and the boxes that are reported instead.
//
// The fixture is the shape the suite's hyphens-vertical tests are: a block with
// both its sizes declared, holding nothing but text. Courier at 20px with a
// 20px line height makes every number in here a whole one — a line is 20 wide on
// the page and each character 12 long — so a wrong axis is a wrong number and
// not a rounding.

const turnedCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; width: 60px; height: 100px }`

// turnedRuns is every run of text in a document, with where it was drawn and
// which way it went.
func turnedRuns(t *testing.T, htmlSrc, cssSrc string) []DrawText {
	t.Helper()
	var out []DrawText
	for _, op := range Paint(layoutOf(t, 400, htmlSrc, cssSrc)) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v)
		}
	}
	return out
}

// TestAVerticalBlockRunsItsTextDownThePage.
//
// Three facts, and each of them is a different way for the turn to be wrong.
// The text goes *down*: two runs on one line share an x and differ in y. The
// lines stack *leftwards from the right edge*: the second line's x is a line
// height less than the first's, and the first's is at the box's right. And the
// runs say so, because a backend that drew them the way it draws every other
// run would put the whole page back on its side.
func TestAVerticalBlockRunsItsTextDownThePage(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd<br>ef</div>`, turnedCSS)
	if len(runs) != 2 {
		t.Fatalf("the fixture drew %d runs, want 2 — it cannot say what it means to say", len(runs))
	}
	first, second := runs[0], runs[1]
	for _, r := range runs {
		if !r.Sideways {
			t.Errorf("the run %q was not marked sideways, so a backend would draw it across the page", r.Text)
		}
	}
	// The block's content box is the whole box: no border, no padding. Its right
	// edge is at 60, and the first line's baseline sits one ascent in from it.
	if x := first.At.X.Px(); x <= 40 || x >= 60 {
		t.Errorf("the first line's baseline is at x=%g, want it between the box's "+
			"right edge at 60 and the next line's column at 40", first.At.X.Px())
	}
	if got := first.At.X.Sub(second.At.X).Px(); got != 20 {
		t.Errorf("the second line is %gpx to the left of the first, want 20 — "+
			"lines stack from the right edge leftwards by a line height", got)
	}
	if first.At.Y.Px() != 0 || second.At.Y.Px() != 0 {
		t.Errorf("the two lines begin at y=%g and %g, want 0 and 0 — every line "+
			"starts at the top of a vertical block", first.At.Y.Px(), second.At.Y.Px())
	}
}

// TestTheGlyphsOfAVerticalBlockGoDownTheLine.
//
// The run above is one operation whichever way it goes, so it cannot tell a
// turn from a translation. This asks where the *glyphs* land, which is the
// question the reftest oracle asks and the one a wrong rotation fails.
func TestTheGlyphsOfAVerticalBlockGoDownTheLine(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd</div>`, turnedCSS)
	if len(runs) != 1 {
		t.Fatalf("the fixture drew %d runs, want 1", len(runs))
	}
	marks := glyphMarks(runs[0], "run", "run", false)
	if len(marks) != 4 {
		t.Fatalf("the run put %d marks on the page, want 4 — one per letter", len(marks))
	}
	for i, m := range marks {
		if m.x != marks[0].x {
			t.Errorf("letter %d is at x=%v and the first at x=%v; every glyph on a "+
				"vertical line shares its baseline's x", i, m.x, marks[0].x)
		}
		if got := m.y.Sub(marks[0].y).Px(); got != float64(i)*12 {
			t.Errorf("letter %d is %gpx below the first, want %g — Courier's advance "+
				"is 12px at 20px and it goes down the page", i, got, float64(i)*12)
		}
	}
}

// TestAVerticalBlockKeepsItsOwnEdgesPhysical.
//
// The turn is of what a box holds and not of the box. "padding-left" is still
// the left of the page inside a vertical block — CSS Writing Modes leaves the
// physical properties physical — so the text starts an inch from the *right*
// edge because that is where the block axis begins, and an inch from the top
// because that is where the inline axis does.
func TestAVerticalBlockKeepsItsOwnEdgesPhysical(t *testing.T) {
	plain := turnedRuns(t, `<div id="d">ab</div>`, turnedCSS)
	padded := turnedRuns(t, `<div id="d">ab</div>`,
		turnedCSS+`
	#d { padding-left: 10px; padding-top: 5px }`)
	if len(plain) != 1 || len(padded) != 1 {
		t.Fatalf("the fixtures drew %d and %d runs, want 1 each", len(plain), len(padded))
	}
	if got := padded[0].At.X.Sub(plain[0].At.X).Px(); got != 10 {
		t.Errorf("padding-left moved the first line %gpx, want 10 — the left of the "+
			"page is still the left of the page inside a vertical block, and it "+
			"carries the content box's other edge with it", got)
	}
	if got := padded[0].At.Y.Sub(plain[0].At.Y).Px(); got != 5 {
		t.Errorf("padding-top moved the text %gpx down, want 5 — it is where a "+
			"vertical line begins", got)
	}
}

// TestADecorationOnAVerticalLineIsRuledDownIt.
//
// An underline is a rectangle beside the letters rather than under them, and it
// is as long as the run rather than as wide. Everything about it is measured
// from the baseline it crosses, so getting it right is the same arithmetic the
// glyphs use and getting it wrong leaves a stripe across the page.
func TestADecorationOnAVerticalLineIsRuledDownIt(t *testing.T) {
	var bands []Rect
	for _, op := range Paint(layoutOf(t, 400,
		`<div id="d"><span style="text-decoration: underline">abcd</span></div>`,
		turnedCSS)) {
		if v, ok := op.(FillRect); ok && !v.Rect.Empty() {
			bands = append(bands, v.Rect)
		}
	}
	if len(bands) != 1 {
		t.Fatalf("the fixture painted %d rectangles, want 1 — the underline", len(bands))
	}
	b := bands[0]
	if b.H.Px() != 48 {
		t.Errorf("the underline is %gpx tall, want 48 — it runs the length of four "+
			"Courier characters, down the page", b.H.Px())
	}
	if b.W >= b.H {
		t.Errorf("the underline is %gpx wide and %gpx tall; a rule down a vertical "+
			"line is longer than it is thick", b.W.Px(), b.H.Px())
	}
}

// The boxes this engine will not turn, and the finding each of them gets.
//
// Every row is a way for the quarter turn to stop being the same picture as the
// writing mode it stands in for. What is asserted is both halves of the answer:
// that something was said, and that what was said names the reason — a finding
// that blamed the wrong thing would send a reader to change the wrong
// declaration.
func TestABoxThisEngineCannotTurnIsReported(t *testing.T) {
	for _, c := range []struct {
		what, css, html, names string
	}{
		{"an automatic height", `#d { writing-mode: vertical-rl; width: 60px }`,
			`<div id="d">ab</div>`, "height is automatic"},
		{"an automatic width", `#d { writing-mode: vertical-rl; height: 100px }`,
			`<div id="d">ab</div>`, "width is automatic"},
		{"upright text", turnedCSS, `<div id="d">日本</div>`, "stand upright"},
		{"text-orientation", turnedCSS + `
	#d { text-orientation: upright }`, `<div id="d">ab</div>`, "text-orientation: upright"},
		{"text-combine-upright", turnedCSS,
			`<div id="d"><span style="text-combine-upright: all">12</span></div>`,
			"text-combine-upright: all"},
		{"a floated child", turnedCSS, `<div id="d"><p style="float: left">ab</p></div>`,
			"floated or positioned"},
		{"a child with a width", turnedCSS, `<div id="d"><p style="width: 10px">ab</p></div>`,
			`"width" is declared inside it`},
		{"a child with a margin", turnedCSS, `<div id="d"><p style="margin-top: 1px">ab</p></div>`,
			"margin, border or padding is declared inside it"},
		{"a form control", turnedCSS, `<div id="d"><input></div>`,
			"form control"},
		{"another writing mode", turnedCSS,
			`<div id="d"><p style="writing-mode: horizontal-tb">ab</p></div>`,
			"changes the writing mode again"},
		{"a mode that is not laid out", `#d { writing-mode: vertical-lr; width: 60px; height: 100px }`,
			`<div id="d">ab</div>`, `only "vertical-rl" is laid out`},
	} {
		t.Run(c.what, func(t *testing.T) {
			// The first, which is the outermost box's: turns() is asked in
			// blockIn before the children are laid out, so a finding about an
			// inner box can only come after.
			var said string
			for _, f := range findingsOf(t, c.html, c.css) {
				if f.Property == "writing-mode" {
					said = f.Message
					break
				}
			}
			if said == "" {
				t.Fatalf("nothing was reported about a box with %s, so a page laid "+
					"out the wrong way round says nothing about it", c.what)
			}
			if !strings.Contains(said, c.names) {
				t.Errorf("the finding for %s is %q, which does not name %q",
					c.what, said, c.names)
			}
		})
	}
}

// TestABoxThisEngineCannotTurnIsLaidOutHorizontally.
//
// The other half of the sentence the finding ends with. A refused box is not a
// blank one and it is not a broken one: it is exactly the page this engine drew
// before any of this existed, which is what makes the finding the whole of the
// difference.
func TestABoxThisEngineCannotTurnIsLaidOutHorizontally(t *testing.T) {
	refused := turnedRuns(t, `<div id="d">ab</div>`,
		`#d { font-family: Courier; font-size: 20px; line-height: 20px;
		      writing-mode: vertical-lr; width: 60px; height: 100px }`)
	plain := turnedRuns(t, `<div id="d">ab</div>`,
		`#d { font-family: Courier; font-size: 20px; line-height: 20px;
		      width: 60px; height: 100px }`)
	if len(refused) != 1 || len(plain) != 1 {
		t.Fatalf("the fixtures drew %d and %d runs, want 1 each", len(refused), len(plain))
	}
	if refused[0].Sideways {
		t.Error("a box the engine refused to turn was drawn sideways anyway")
	}
	if refused[0].At != plain[0].At {
		t.Errorf("the refused box drew its text at %v and a box with no writing mode "+
			"at %v; a mode that is not laid out changes nothing at all",
			refused[0].At, plain[0].At)
	}
}

// TestABoxInsideATurnedBoxIsNotReportedAgain.
//
// writing-mode inherits, so every box inside a vertical one computes to
// vertical too. Each of those is laid out by its ancestor's turn and has nothing
// of its own to say — and a finding repeated once per element is how the one
// channel that says what a page is missing stops being read.
func TestABoxInsideATurnedBoxIsNotReportedAgain(t *testing.T) {
	for _, f := range findingsOf(t, `<div id="d"><p>ab</p><p>cd</p></div>`,
		`#d { font-family: Courier; font-size: 20px; line-height: 20px;
		      writing-mode: vertical-rl; width: 60px; height: 100px }`) {
		if f.Property == "writing-mode" {
			t.Errorf("a box inside a turned one was reported: %q", f.Message)
		}
	}
}

// TestARaisedRunInAVerticalBlockMovesTowardsTheTopOfItsLine.
//
// §10.8.1's vertical-align is along the block axis of the line, so on a turned
// line it is along the page's x. Which way is the question: the line's "over"
// side is its block-start edge, which for vertical-rl is its *right* one, so a
// raised run moves right and a lowered one left.
//
// It is worth a test of its own because it is the one part of the run's
// arithmetic that is zero in ordinary text, and a sign error there is invisible
// until a document has a superscript in it.
func TestARaisedRunInAVerticalBlockMovesTowardsTheTopOfItsLine(t *testing.T) {
	runs := turnedRuns(t,
		`<div id="d">a<span style="vertical-align: 4px">b</span><span style="vertical-align: -4px">c</span></div>`,
		turnedCSS)
	if len(runs) != 3 {
		t.Fatalf("the fixture drew %d runs, want 3 — a, b and c", len(runs))
	}
	plain, raised, lowered := runs[0], runs[1], runs[2]
	if got := raised.At.X.Sub(plain.At.X).Px(); got != 4 {
		t.Errorf("a run raised 4px is %gpx to the right of the one beside it, want 4 "+
			"— the top of a vertical-rl line is its right-hand side", got)
	}
	if got := lowered.At.X.Sub(plain.At.X).Px(); got != -4 {
		t.Errorf("a run lowered 4px is %gpx to the right of the one beside it, want -4", got)
	}
	// And neither of them moved along the line, which is the other half of
	// saying the displacement went into the right axis.
	if raised.At.Y.Sub(plain.At.Y).Px() != 12 || lowered.At.Y.Sub(raised.At.Y).Px() != 12 {
		t.Errorf("the three runs are at y=%v, %v and %v; each should be one Courier "+
			"advance further down the line than the last, and vertical-align "+
			"should not have moved any of them along it",
			plain.At.Y, raised.At.Y, lowered.At.Y)
	}
}

// TestAVerticalBlockIsNotCutIntoByAFloatBesideIt.
//
// A turned box lays its content out in a frame of its own, and a float outside
// it is in the page's frame. The two do not share an axis: a float that takes
// 50px off the left of the page would, if the turned box's lines were broken
// against the same context, take 50px off the *top* of every line inside it.
//
// A box that changes the writing mode establishes an independent formatting
// context, which is what makes that impossible, and this is that rule with a
// document behind it.
func TestAVerticalBlockIsNotCutIntoByAFloatBesideIt(t *testing.T) {
	// A short float, so that the block sits beside it rather than below it:
	// what is being asked is whether a float the block is next to reaches into
	// the lines inside it, and a block pushed clear of one is not next to it.
	const floatCSS = `
	.f { float: left; width: 50px; height: 10px }`
	alone := turnedRuns(t, `<div id="d">abcdefgh</div>`, turnedCSS+floatCSS)
	beside := turnedRuns(t, `<div class="f"></div><div id="d">abcdefgh</div>`, turnedCSS+floatCSS)
	if len(alone) != 1 {
		t.Fatalf("the fixture drew %d runs with no float, want 1 — eight Courier "+
			"characters are 96px and the block's lines are 100px long", len(alone))
	}
	if len(beside) != len(alone) {
		t.Fatalf("with a float beside it the block drew %d runs and without it %d; "+
			"a float outside a vertical block does not shorten the lines inside it",
			len(beside), len(alone))
	}
	if beside[0].At != alone[0].At {
		t.Errorf("with a float beside it the block drew its text at %v and without "+
			"it at %v; a float outside a vertical block reaches neither along its "+
			"lines nor across them", beside[0].At, alone[0].At)
	}
}
