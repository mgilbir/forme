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
		{"an automatic width", `#d { writing-mode: vertical-rl; height: 100px }`,
			`<div id="d">ab</div>`, "width is automatic"},
		{"upright text", turnedCSS, `<div id="d">日本</div>`, "stand upright"},
		{"text-combine-upright", turnedCSS,
			`<div id="d"><span style="text-combine-upright: all">12</span></div>`,
			"text-combine-upright: all"},
		{"a floated child", turnedCSS, `<div id="d"><p style="float: left">ab</p></div>`,
			"floated or positioned"},
		{"a positioned box", `#d { writing-mode: vertical-rl; width: 60px; height: 100px;
		                          position: relative }`,
			`<div id="d">ab</div>`, "positioned or replaced"},
		{"a child with a width", turnedCSS, `<div id="d"><p style="width: 10px">ab</p></div>`,
			`"width" is declared inside it`},
		{"a child with a margin", turnedCSS, `<div id="d"><p style="margin-top: 1px">ab</p></div>`,
			"margin, border or padding is declared inside it"},
		{"a form control", turnedCSS, `<div id="d"><input></div>`,
			"form control"},
		{"another writing mode", turnedCSS,
			`<div id="d"><p style="writing-mode: horizontal-tb">ab</p></div>`,
			"changes the writing mode again"},
		{"a mode that is not laid out", `#d { writing-mode: sideways-lr; width: 60px; height: 100px }`,
			`<div id="d">ab</div>`, `only "vertical-rl" and "vertical-lr" are laid out`},
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
		      writing-mode: sideways-lr; width: 60px; height: 100px }`)
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

// TestVerticalLRStacksItsLinesFromTheOtherEdge.
//
// The two vertical modes differ by one thing and this is it. Both run their
// text down the page and turn their glyphs the same quarter turn clockwise;
// vertical-rl stacks its lines back from the right edge and vertical-lr stacks
// them forwards from the left. So the *first* line of one is where the last
// line of the other is, and everything inside a line is identical.
func TestVerticalLRStacksItsLinesFromTheOtherEdge(t *testing.T) {
	const lrCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-lr; width: 60px; height: 100px }`
	rl := turnedRuns(t, `<div id="d">abcd<br>ef</div>`, turnedCSS)
	lr := turnedRuns(t, `<div id="d">abcd<br>ef</div>`, lrCSS)
	if len(rl) != 2 || len(lr) != 2 {
		t.Fatalf("the fixtures drew %d and %d runs, want 2 each", len(rl), len(lr))
	}
	for _, r := range lr {
		if !r.Sideways {
			t.Errorf("the vertical-lr run %q was not marked sideways", r.Text)
		}
	}
	// The box is 60 wide and a line is 20, so vertical-rl puts its first line in
	// the column [40,60] and vertical-lr puts it in [0,20]. Both measure the
	// baseline back from the same edge of their own line box, so the two
	// baselines are two whole line columns apart.
	if got := rl[0].At.X.Sub(lr[0].At.X).Px(); got != 40 {
		t.Errorf("the first line of vertical-rl is %gpx to the right of the first "+
			"line of vertical-lr, want 40 — one is at the right of a 60px box and "+
			"the other at its left, and a line is 20 wide", got)
	}
	if got := lr[1].At.X.Sub(lr[0].At.X).Px(); got != 20 {
		t.Errorf("the second vertical-lr line is %gpx to the right of the first, "+
			"want 20 — lines stack from the left edge rightwards", got)
	}
	// And within a line nothing changed: the same offset from the baseline, the
	// same start at the top.
	if lr[0].At.Y != rl[0].At.Y || lr[1].At.Y != rl[1].At.Y {
		t.Errorf("the vertical-lr lines begin at y=%v and %v and the vertical-rl "+
			"lines at y=%v and %v; both run their text from the top downwards",
			lr[0].At.Y, lr[1].At.Y, rl[0].At.Y, rl[1].At.Y)
	}
}

// TestAVerticalBlockWithNoHeightFitsItsLinesToThePage.
//
// CSS Writing Modes §7.3. A vertical box with an automatic height has no length
// to break its lines against, and the length it uses is not its containing
// block's: the containing block runs the other way, and its block size is the
// one thing a box laid out inside it cannot know. So the box falls back to the
// size of what it is being laid out *on* — the page here — and shrinks to fit
// inside that.
func TestAVerticalBlockWithNoHeightFitsItsLinesToThePage(t *testing.T) {
	const autoCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; width: 60px }`
	root := layoutOf(t, 400, `<div id="d">abcd</div>`, autoCSS)
	d := find(t, root, "d")
	if got := d.ContentRect().H.Px(); got != 48 {
		t.Errorf("a vertical box with no height came out %gpx tall, want 48 — four "+
			"Courier characters at 20px are 12px each, and the box shrinks to fit "+
			"them rather than filling the page", got)
	}
	if got := d.ContentRect().W.Px(); got != 60 {
		t.Errorf("its width is %gpx, want 60 — the block axis is still declared", got)
	}
	// And the fallback really is a ceiling: text longer than the page is cut to
	// the page's height and wraps into a second column rather than running off
	// the bottom. The page here is 10000px, so this asks the other way round —
	// that a short paragraph is *not* stretched to it.
	if got := d.ContentRect().H.Px(); got >= 10000 {
		t.Errorf("the box is %gpx tall; the fallback is an available size to shrink "+
			"inside, not a size to fill", got)
	}
}

// TestAFloatedVerticalBlockIsTurnedLikeAnyOther.
//
// A float already seals its own formatting context, which is the one thing a
// turned box needs from the box it is; the rest of the float rules are about
// where the box goes and are answered before its content is laid out at all.
func TestAFloatedVerticalBlockIsTurnedLikeAnyOther(t *testing.T) {
	runs := turnedRuns(t, `<div id="d" style="float: left">abcd</div>`, turnedCSS)
	if len(runs) != 1 {
		t.Fatalf("the fixture drew %d runs, want 1", len(runs))
	}
	if !runs[0].Sideways {
		t.Error("a floated vertical block was not turned")
	}
	plain := turnedRuns(t, `<div id="d">abcd</div>`, turnedCSS)
	if runs[0].At != plain[0].At {
		t.Errorf("floated, the text is at %v; in flow at %v. The float is at the "+
			"same place on this page and its content is turned the same way",
			runs[0].At, plain[0].At)
	}
}

// The other typesetting mode a vertical line has: "text-orientation: upright".
//
// It is not a rotation, which is why it needs anything at all. Every character
// stands the way it does in the code charts and the pen moves one em to the
// next one — CSS Writing Modes §4.4's synthesized vertical metrics, since no
// face this engine reads states any — so an upright run's *width* is a count of
// its characters and not a sum of its advances.

const uprightCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; text-orientation: upright;
	     width: 60px; height: 100px }`

// TestAnUprightRunAdvancesOneEmPerCharacter.
//
// Courier's advance is 12px at 20px, so a run measured the ordinary way is
// 48px long and an upright one is 80. The difference is what decides where the
// line breaks and how wide the box that shrinks to fit it is, so it is a
// measurement question before it is a drawing one.
func TestAnUprightRunAdvancesOneEmPerCharacter(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd</div>`, uprightCSS)
	if len(runs) != 1 {
		t.Fatalf("the fixture drew %d runs, want 1", len(runs))
	}
	if !runs[0].Upright {
		t.Fatal("the run was not marked upright, so a backend would turn its glyphs on their side")
	}
	if !runs[0].Sideways {
		t.Error("the run was not marked sideways; an upright run still goes down the page")
	}
	marks := glyphMarks(runs[0], "run", "run", false)
	if len(marks) != 4 {
		t.Fatalf("the run put %d marks on the page, want 4", len(marks))
	}
	for i, m := range marks {
		if got := m.y.Sub(marks[0].y).Px(); got != float64(i)*20 {
			t.Errorf("letter %d is %gpx below the first, want %g — one em each, and "+
				"not Courier's 12px advance", i, got, float64(i)*20)
		}
	}
	// And the box shrinks to the same number. Four ems is 80px, which is more
	// than the 100px line this box has room for, so the run stays on one line.
	turned := turnedRuns(t, `<div id="d">abcd</div>`, turnedCSS)
	if turned[0].Upright {
		t.Error("a run with no text-orientation was marked upright")
	}
}

// TestAnUprightBoxIsTurnedEvenWhereItsCharactersStandUpright.
//
// The UAX #50 gate is about "text-orientation: mixed", which turns the
// characters the table calls rotatable and leaves the rest upright — and this
// engine cannot leave one character upright on a turned line. Under
// "text-orientation: upright" the question does not arise: every character
// stands upright, which is a thing the engine can now do, so a box of Japanese
// is turned where a box of Japanese in mixed is refused.
func TestAnUprightBoxIsTurnedEvenWhereItsCharactersStandUpright(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">日本</div>`, uprightCSS)
	if len(runs) == 0 {
		t.Fatal("a box of ideographs set upright drew nothing")
	}
	// One run per face, and the fallback library may set the two characters in
	// two: what is asked here is that each of them is turned and upright, not
	// how many pieces the font stack cut them into.
	for _, r := range runs {
		if !r.Upright || !r.Sideways {
			t.Errorf("the run %q is sideways=%v upright=%v; a box of ideographs set "+
				"upright is both", r.Text, r.Sideways, r.Upright)
		}
	}
	var said string
	for _, f := range findingsOf(t, `<div id="d">日本</div>`,
		`#d { writing-mode: vertical-rl; text-orientation: upright;
		      width: 60px; height: 100px }`) {
		if f.Property == "writing-mode" {
			said = f.Message
		}
	}
	if said != "" {
		t.Errorf("a box of ideographs set upright was reported: %q", said)
	}
}

// TestOneOrientationPerTurnedBox.
//
// The turn is one decision for a whole subtree, so a box inside it that asks
// for the other typesetting mode is a second mode on the same line — and the
// engine has one. Refusing is the honest answer, and it is the same refusal
// whichever way round the two are.
func TestOneOrientationPerTurnedBox(t *testing.T) {
	for _, c := range []struct{ what, outer, inner string }{
		{"upright inside mixed", "mixed", "upright"},
		{"mixed inside upright", "upright", "mixed"},
	} {
		var said string
		for _, f := range findingsOf(t,
			`<div id="d"><p style="text-orientation: `+c.inner+`">ab</p></div>`,
			`#d { writing-mode: vertical-rl; text-orientation: `+c.outer+`;
			      width: 60px; height: 100px }`) {
			if f.Property == "writing-mode" && said == "" {
				said = f.Message
			}
		}
		if !strings.Contains(said, "not the orientation the box is set in") {
			t.Errorf("%s was reported as %q, which does not name the orientation",
				c.what, said)
		}
	}
}

// TestTheInkOfAnUprightRunIsAnEmWideOnTheBaseline.
//
// The other synthesized metric. An upright character sits centred on the
// vertical baseline in a box one em across, so a run of them inks a column an
// em wide — and not the face's ascent and descent, which describe a line of
// text lying the other way.
func TestTheInkOfAnUprightRunIsAnEmWideOnTheBaseline(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd</div>`, uprightCSS)
	if len(runs) != 1 {
		t.Fatalf("the fixture drew %d runs, want 1", len(runs))
	}
	ink := textInk(runs[0])
	if ink.W.Px() != 20 {
		t.Errorf("the run inks %gpx across the line, want 20 — one em, centred on "+
			"the baseline", ink.W.Px())
	}
	if ink.H.Px() != 80 {
		t.Errorf("the run inks %gpx along the line, want 80 — four characters at "+
			"one em each", ink.H.Px())
	}
	if got := runs[0].At.X.Sub(ink.X).Px(); got != 10 {
		t.Errorf("the baseline is %gpx from the left of the ink, want 10 — half an "+
			"em either side of it", got)
	}
}

// TestSidewaysTurnsEveryCharacterIncludingTheUprightOnes.
//
// "text-orientation: sideways" is the quarter turn this file performs, applied
// to every character rather than only the ones UAX #50 calls rotatable. So a
// box of ideographs is turnable under it and is not under "mixed" — which is
// the whole of what the table is for, and is why the check is asked of the
// orientation rather than of the text alone.
func TestSidewaysTurnsEveryCharacterIncludingTheUprightOnes(t *testing.T) {
	for _, orientation := range []string{"sideways", "sideways-right"} {
		css := `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; text-orientation: ` + orientation + `;
	     width: 60px; height: 100px }`
		runs := turnedRuns(t, `<div id="d">日本</div>`, css)
		if len(runs) == 0 {
			t.Fatalf("%s: a box of ideographs drew nothing", orientation)
		}
		for _, r := range runs {
			if !r.Sideways {
				t.Errorf("%s: the run %q was not turned", orientation, r.Text)
			}
			if r.Upright {
				t.Errorf("%s: the run %q was set upright; sideways turns every "+
					"character with the page", orientation, r.Text)
			}
		}
	}
	// And under "mixed" the same box is refused, because one line cannot hold
	// both orientations here.
	var said string
	for _, f := range findingsOf(t, `<div id="d">日本</div>`,
		`#d { writing-mode: vertical-rl; width: 60px; height: 100px }`) {
		if f.Property == "writing-mode" && said == "" {
			said = f.Message
		}
	}
	if !strings.Contains(said, "stand upright on a vertical line") {
		t.Errorf("a box of ideographs in the initial orientation was reported as "+
			"%q, which does not name the characters that stand upright", said)
	}
}
