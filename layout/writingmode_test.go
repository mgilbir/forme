package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
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
		{"a float with an automatic width",
			`#d { writing-mode: vertical-rl; height: 100px; float: left }`,
			`<div id="d">ab</div>`, "shrinking a box around its content"},
		{"text that needs both orientations", turnedCSS, `<div id="d">ab日本</div>`,
			"standing upright and characters lying along the line at once"},
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
		{"a child with a percentage margin", turnedCSS,
			`<div id="d"><p style="margin-top: 10%">ab</p></div>`,
			"percentage margin or padding"},
		{"a form control", turnedCSS, `<div id="d"><input></div>`,
			"form control"},
		{"another writing mode", turnedCSS,
			`<div id="d"><p style="writing-mode: horizontal-tb">ab</p></div>`,
			"changes the writing mode again"},
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
	// An inline-block, which is refused for being a box with sizing rules of
	// its own rather than for the mode it asked for. Every value of the
	// property is laid out now — the refusals that are left are all about the
	// box — so these two documents differ in the declaration and in nothing
	// else, which is what makes the comparison below mean anything.
	refused := turnedRuns(t, `<div id="d">ab</div>`,
		`#d { font-family: Courier; font-size: 20px; line-height: 20px;
		      display: inline-block;
		      writing-mode: vertical-rl; width: 60px; height: 100px }`)
	plain := turnedRuns(t, `<div id="d">ab</div>`,
		`#d { font-family: Courier; font-size: 20px; line-height: 20px;
		      display: inline-block; width: 60px; height: 100px }`)
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
	// And under "mixed" the same box is turned too — upright, because that is
	// what mixed says when every character in the box is one. What mixed cannot
	// have is both at once, and that is the case that is refused.
	runs := turnedRuns(t, `<div id="d">日本</div>`, turnedCSS)
	for _, r := range runs {
		if !r.Upright {
			t.Errorf("under the initial orientation the run %q was not set upright, "+
				"and every character in it stands upright", r.Text)
		}
	}
	var said string
	for _, f := range findingsOf(t, `<div id="d">ab日本</div>`,
		`#d { writing-mode: vertical-rl; width: 60px; height: 100px }`) {
		if f.Property == "writing-mode" && said == "" {
			said = f.Message
		}
	}
	if !strings.Contains(said, "at once") {
		t.Errorf("a box of Latin and ideographs in the initial orientation was "+
			"reported as %q, which does not name the mixture", said)
	}
}

// TestMixedIsWhicheverOrientationTheTextNeeds.
//
// "text-orientation: mixed" is the initial value, so it is what almost every
// vertical box is set in, and it does not name an orientation — it says to use
// the one each character needs. Where a box's characters all need the same one,
// that is the orientation the box is set in, and this engine can set it.
//
// The case that matters is the one the suite writes:
// text-transform-fullwidth-002 puts "Text sample" in a vertical box with no
// orientation and asks for it upright, which is what mixed says once the
// transform has made every letter a fullwidth form. The box tree carries the
// transformed text, so the question is asked of what will be drawn.
func TestMixedIsWhicheverOrientationTheTextNeeds(t *testing.T) {
	plain := turnedRuns(t, `<div id="d">ab</div>`, turnedCSS)
	if len(plain) != 1 || plain[0].Upright {
		t.Fatalf("Latin in the initial orientation drew %d runs, upright=%v; it "+
			"lies along the line", len(plain), plain[0].Upright)
	}
	wide := turnedRuns(t, `<div id="d" style="text-transform: full-width">ab</div>`,
		turnedCSS)
	if len(wide) == 0 {
		t.Fatal("the full-width box drew nothing")
	}
	for _, r := range wide {
		if !r.Upright {
			t.Errorf("the run %q is not upright; full-width forms stand upright and "+
				"mixed asks for the orientation each character needs", r.Text)
		}
	}
	// The advance follows: an upright run is an em a character whatever the
	// face's advance is, so the two runs are not the same length down the line.
	// Two Courier characters are 24px lying along it and 40px standing up.
	if got := textInk(plain[0]).H.Px(); got != 24 {
		t.Errorf("two characters lying along the line reach %gpx, want 24", got)
	}
	if got := textInk(wide[0]).H.Px(); got != 40 {
		t.Errorf("two characters standing upright reach %gpx, want 40 — an em each", got)
	}
}

// TestAVerticalBlockWithNoWidthIsAsWideAsItsLinesStack.
//
// The block axis of a turned box is its *width*, so a width of auto asks the
// question an automatic height asks of an ordinary block: how far did the
// content get. It is not the question the width rules answer, which is how much
// room the containing block has — that is the answer for a box whose lines run
// across the page, and this box's stack down it.
func TestAVerticalBlockWithNoWidthIsAsWideAsItsLinesStack(t *testing.T) {
	const autoCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; height: 40px }`
	// 40px of line holds three Courier characters and breaks before the fourth,
	// so the content stacks two lines and the box is two line heights wide.
	d := find(t, layoutOf(t, 400, `<div id="d">ab cd</div>`, autoCSS), "d")
	if got := d.ContentRect().W.Px(); got != 40 {
		t.Errorf("a vertical box with no width came out %gpx wide, want 40 — two "+
			"lines at a 20px line height, and not the 400px the page has", got)
	}
	if got := d.ContentRect().H.Px(); got != 40 {
		t.Errorf("its height is %gpx, want 40 — the inline axis is still declared", got)
	}
	// And the second line is where the box's own edge is, which is what says the
	// width was taken from the content rather than left at the page's.
	runs := turnedRuns(t, `<div id="d">ab cd</div>`, autoCSS)
	if len(runs) != 2 {
		t.Fatalf("the fixture drew %d runs, want 2", len(runs))
	}
	if got := runs[0].At.X.Sub(runs[1].At.X).Px(); got != 20 {
		t.Errorf("the second line is %gpx to the left of the first, want 20", got)
	}
}

// TestTurningTheEdgesBackAndForthChangesNothing.
//
// The two permutations that let a box inside a turned one declare a margin at
// all: one to read the declaration in the frame the box is laid out in, one to
// put it back on the page. They have to be exact inverses, because between them
// sits the whole of block layout — a margin read as a block-axis one and
// returned as an inline-axis one is a box that moved.
func TestTurningTheEdgesBackAndForthChangesNothing(t *testing.T) {
	e := Edges{Top: 1, Right: 2, Bottom: 4, Left: 8}
	for _, mode := range []writingMode{verticalRL, verticalLR, sidewaysRL, sidewaysLR} {
		if got := turnEdges(untuneEdges(e, mode), mode); got != e {
			t.Errorf("%v: turning %v back and forth gave %v", mode, e, got)
		}
		if got := untuneEdges(turnEdges(e, mode), mode); got != e {
			t.Errorf("%v: turning %v forth and back gave %v", mode, e, got)
		}
	}
	// And the two modes are not the same permutation, which is the thing that
	// was wrong before there were two of them: both run their text from the top
	// downwards, so both send the horizontal left to the page's top, and they
	// stack their lines from opposite edges.
	rl, lr := turnEdges(e, verticalRL), turnEdges(e, verticalLR)
	if rl.Top != lr.Top {
		t.Errorf("the two modes put the horizontal left at %v and %v; both run "+
			"their text downwards", rl.Top, lr.Top)
	}
	if rl == lr {
		t.Error("the two modes permute the edges the same way; they stack their " +
			"lines from opposite edges")
	}
	// And the four are four different permutations. sideways-rl is the same
	// turn as vertical-rl, so those two *are* one permutation and are compared
	// as such; sideways-lr is the other turn, and sends the horizontal left to
	// the foot of the page rather than to its head.
	if got := turnEdges(e, sidewaysRL); got != rl {
		t.Errorf("sideways-rl permuted the edges to %v and vertical-rl to %v; "+
			"they are the same quarter turn", got, rl)
	}
	other := turnEdges(e, sidewaysLR)
	if other.Bottom != lr.Top {
		t.Errorf("sideways-lr put the horizontal left at bottom=%v where the "+
			"clockwise modes put it at top=%v; it turns the other way",
			other.Bottom, lr.Top)
	}
	if other == lr {
		t.Error("sideways-lr and vertical-lr permute the edges the same way; " +
			"they stack their lines the same way and turn opposite ways")
	}
}

// TestAMarginInsideAVerticalBlockIsOnTheSideItNames.
//
// "margin-top" is the top of the page whichever way the text inside a box runs,
// so a margin on a box inside a turned one has to come out where the author
// wrote it — even though the engine that laid it out read it as a margin along
// the line.
//
// It is not a corner: the user agent sheet gives every heading and every
// paragraph a margin, so before this a vertical box holding an <h1> was a
// vertical box this engine would not lay out at all. The suite's
// text-transform-fullwidth-004 and -005 are exactly that document.
func TestAMarginInsideAVerticalBlockIsOnTheSideItNames(t *testing.T) {
	const css = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; width: 100px; height: 100px }
	p { margin: 0 }`
	plain := turnedRuns(t, `<div id="d"><p>ab</p></div>`, css)
	if len(plain) != 1 {
		t.Fatalf("the fixture drew %d runs, want 1", len(plain))
	}
	for _, c := range []struct {
		side   string
		dx, dy float64
		what   string
	}{
		{"top", 0, 10, "the top of the page is where a vertical line begins"},
		{"right", -10, 0, "the right of the page is where the first line of a " +
			"vertical-rl box stands"},
	} {
		got := turnedRuns(t, `<div id="d"><p style="margin-`+c.side+`: 10px">ab</p></div>`, css)
		if len(got) != 1 {
			t.Fatalf("margin-%s: the fixture drew %d runs, want 1", c.side, len(got))
		}
		dx := got[0].At.X.Sub(plain[0].At.X).Px()
		dy := got[0].At.Y.Sub(plain[0].At.Y).Px()
		if dx != c.dx || dy != c.dy {
			t.Errorf("margin-%s: 10px moved the text by (%g,%g), want (%g,%g) — %s",
				c.side, dx, dy, c.dx, c.dy, c.what)
		}
	}
}

// TestTheTurnedBoxKeepsItsOwnEdgesWhereTheyWere.
//
// The permutation is for what a turned box *holds* and not for the box. Its own
// margin, border and padding are in its parent's frame, its parent is not
// turned, and "margin-top" on it is the top of the page in the plainest sense:
// it is what separates it from whatever is above it in an ordinary horizontal
// flow.
func TestTheTurnedBoxKeepsItsOwnEdgesWhereTheyWere(t *testing.T) {
	const css = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; width: 100px; height: 100px }`
	plain := turnedRuns(t, `<div id="d">ab</div>`, css)
	pushed := turnedRuns(t, `<div id="d" style="margin-top: 10px">ab</div>`, css)
	if len(plain) != 1 || len(pushed) != 1 {
		t.Fatalf("the fixtures drew %d and %d runs, want 1 each", len(plain), len(pushed))
	}
	dx := pushed[0].At.X.Sub(plain[0].At.X).Px()
	dy := pushed[0].At.Y.Sub(plain[0].At.Y).Px()
	if dx != 0 || dy != 10 {
		t.Errorf("a 10px top margin on the turned box moved its text by (%g,%g), "+
			"want (0,10) — the box's own margins are in its parent's frame and "+
			"its parent is not turned", dx, dy)
	}
}

// TestATurnedBoxIsNotInsideItself.
//
// insideTurn is asked "which frame are this box's declarations in", and for the
// box that starts a turn the answer is its parent's. Asking it of the box
// itself would read its own margins in the frame it establishes rather than the
// one it sits in.
//
// It is tested here rather than through a document because the order of blockIn
// hides it: a box's edges are resolved before turns() records anything about it,
// so a walk that included the box would find nothing and give the right answer
// by accident. What this asserts is the definition, not the accident.
func TestATurnedBoxIsNotInsideItself(t *testing.T) {
	root := &Box{}
	child := &Box{Parent: root}
	grandchild := &Box{Parent: child}
	l := &layouter{turnedMode: map[*Box]writingMode{root: verticalRL}}
	if _, turned := l.insideTurn(root); turned {
		t.Error("the box that starts a turn was reported as inside one; its own " +
			"declarations are in its parent's frame")
	}
	for _, c := range []struct {
		b    *Box
		what string
	}{{child, "a child"}, {grandchild, "a grandchild"}} {
		mode, turned := l.insideTurn(c.b)
		if !turned || mode != verticalRL {
			t.Errorf("%s of a turned box was reported as turned=%v mode=%v",
				c.what, turned, mode)
		}
	}
	if _, turned := l.insideTurn(nil); turned {
		t.Error("no box at all was reported as inside a turn")
	}
	if _, turned := (&layouter{turnedMode: map[*Box]writingMode{}}).insideTurn(child); turned {
		t.Error("a box on a horizontal page was reported as inside a turn")
	}
}

// TestAControlCharacterInAVerticalBlockIsBoxedDownTheLine.
//
// CSS Text requires a control character to be visible, and no face has a glyph
// for one, so the mark is a ring the painter synthesizes out of four
// rectangles. A rectangle built in page coordinates on a line that runs down
// the page comes out lying across it — between two letters set one above the
// other, and reaching off the top of the box as well. Built in the run's own
// axes and placed afterwards, it is a ring either way.
func TestAControlCharacterInAVerticalBlockIsBoxedDownTheLine(t *testing.T) {
	var ring []Rect
	for _, op := range Paint(layoutOf(t, 400, "<div id=\"d\">a\u0001b</div>", turnedCSS)) {
		if v, ok := op.(FillRect); ok && !v.Rect.Empty() {
			ring = append(ring, v.Rect)
		}
	}
	if len(ring) != 4 {
		t.Fatalf("the control character drew %d rectangles, want 4 — a ring", len(ring))
	}
	// The two letters are one Courier advance apart down the same column, so the
	// ring belongs between them: longer across the line than along it is the
	// wrong way round, and anything above the box's top edge is off the page.
	lo, hi := ring[0].Y, ring[0].Bottom()
	left, right := ring[0].X, ring[0].Right()
	for _, r := range ring[1:] {
		lo, hi = style.Min(lo, r.Y), style.Max(hi, r.Bottom())
		left, right = style.Min(left, r.X), style.Max(right, r.Right())
	}
	if lo.Px() < 0 {
		t.Errorf("the ring reaches %gpx above the page, want nothing above 0", lo.Px())
	}
	if hi.Sub(lo) >= right.Sub(left) {
		t.Errorf("the ring is %gpx along the line and %gpx across it; a control "+
			"character takes one advance along the line and about two thirds of "+
			"an em across it", hi.Sub(lo).Px(), right.Sub(left).Px())
	}
	if lo.Px() <= 0 || hi.Px() >= 24 {
		t.Errorf("the ring spans y=%g..%g; it sits between the letter at y=0 and "+
			"the one an advance below it", lo.Px(), hi.Px())
	}
}

// TestTheEllipsisOfAClampedVerticalBlockIsSetTheWayItsLinesAre.
//
// A clamp keeps room on its last line for the ellipsis it will print there, and
// the room it keeps has to be measured the way that line is set: on an upright
// vertical line the ellipsis stands upright with the text and takes an em per
// character, not the face's horizontal advance for it.
//
// Two questions have to agree — the room reserved and the run drawn — which is
// why this checks the run and the line's fit together.
func TestTheEllipsisOfAClampedVerticalBlockIsSetTheWayItsLinesAre(t *testing.T) {
	const css = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; text-orientation: upright;
	     width: 100px; height: 200px;
	     -webkit-line-clamp: 1; display: -webkit-box; -webkit-box-orient: vertical }`
	var ellipsis *DrawText
	for _, op := range Paint(layoutOf(t, 400, `<div id="d">aaaaaaaaaa bbbbbbbbbb</div>`, css)) {
		if v, ok := op.(DrawText); ok && v.Text == "\u2026" {
			run := v
			ellipsis = &run
		}
	}
	if ellipsis == nil {
		t.Skip("this build does not clamp the fixture, so there is no ellipsis to ask about")
	}
	if !ellipsis.Upright {
		t.Error("the ellipsis was not set upright on a line whose text is; it is " +
			"drawn on that line and in that face")
	}
	if !ellipsis.Sideways {
		t.Error("the ellipsis was not marked sideways; it is on a line that runs " +
			"down the page")
	}
}

// TestAVerticalBoxIsNotTurnedWhereItsWidthWouldBeMeasured.
//
// The intrinsic pass measures a box's content the way a horizontal engine
// measures it, so for a turned box it returns the length of its lines. The
// width such a box wants is the room those lines *stack* in, and nothing can
// answer that before the box is laid out.
//
// So a box that has to be measured before it is laid out is refused. A float
// wrapped round a vertical div came out as wide as that div's text laid end to
// end — 168px of Courier around a box 60px wide, with the float's own
// background showing for the other hundred.
func TestAVerticalBoxIsNotTurnedWhereItsWidthWouldBeMeasured(t *testing.T) {
	const css = `body { margin: 0 }
	.v { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: vertical-rl; height: 100px }
	#f { float: left }`
	// Refused, so laid out horizontally, so the float is as wide as its content
	// on one line — and, whatever that number is, the two agree.
	root := layoutOf(t, 400, `<div id="f"><div class="v" id="v">abcd efgh ijkl</div></div>`, css)
	f, v := find(t, root, "f"), find(t, root, "v")
	if f.BorderRect.W != v.BorderRect.W {
		t.Errorf("the float is %gpx wide and the box inside it %gpx; a box shrunk "+
			"to fit its content is as wide as the content",
			f.BorderRect.W.Px(), v.BorderRect.W.Px())
	}
	var said string
	for _, fi := range findingsOf(t, `<div id="f"><div class="v">abcd</div></div>`, css) {
		if fi.Property == "writing-mode" && said == "" {
			said = fi.Message
		}
	}
	if !strings.Contains(said, "shrinking a box around its content") {
		t.Errorf("the refusal was reported as %q, which does not name the box that "+
			"would be shrunk", said)
	}
	// And a declared width on the way up is enough, even where the turned box
	// itself has none: from that ancestor down every width is already decided,
	// so nothing has to measure this box at all.
	inside := turnedRuns(t, `<div style="width: 200px"><div class="v">abcd</div></div>`, css)
	if len(inside) != 1 || !inside[0].Sideways {
		t.Fatalf("a vertical box with no width, under a box with a declared one, "+
			"drew %d runs and was not turned; nothing above it has to be measured",
			len(inside))
	}
	// The same box under a float is refused, which is the pair that says the
	// ancestor's width is what decides it and not the box's own.
	beside := turnedRuns(t, `<div style="float: left"><div class="v">abcd</div></div>`, css)
	if len(beside) != 1 {
		t.Fatalf("the float fixture drew %d runs, want 1", len(beside))
	}
	if beside[0].Sideways {
		t.Error("a vertical box with no width under a float was turned; the float " +
			"has to measure it before it is laid out")
	}
	// And the walk stops at the first width that is already decided, rather than
	// running to the root: a float *above* a box with a declared width shrinks
	// around that box and never asks this one. Without the stop the same
	// document is refused for a measurement nothing takes.
	sheltered := turnedRuns(t,
		`<div style="float: left"><div style="width: 200px"><div class="v">abcd</div></div></div>`,
		css)
	if len(sheltered) != 1 {
		t.Fatalf("the sheltered fixture drew %d runs, want 1", len(sheltered))
	}
	if !sheltered[0].Sideways {
		t.Error("a vertical box under a box with a declared width was refused " +
			"because a float stood above that box; from a decided width downwards " +
			"nothing is measured")
	}
}

// TestATabStopInAnUprightBlockIsCountedInEms.
//
// A tab stop is a multiple of the space's advance, and on an upright vertical
// line a space advances one em like every other character. Counted in the face's
// horizontal advance instead, a tab in such a block landed at three fifths of
// the column it belongs in — Courier's twelve pixels where the line is set in
// twenties.
func TestATabStopInAnUprightBlockIsCountedInEms(t *testing.T) {
	const css = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     white-space: pre; width: 200px; height: 400px;
	     writing-mode: vertical-rl; text-orientation: upright }`
	runs := turnedRuns(t, "<div id=\"d\">a\tb</div>", css)
	var after *DrawText
	for i := range runs {
		if runs[i].Text == "b" {
			after = &runs[i]
		}
	}
	if after == nil {
		t.Fatal("the fixture drew no run after the tab")
	}
	// tab-size is eight and the upright advance is an em, so the stop is 160px
	// down the line. The default eight Courier advances would be 96.
	if got := after.At.Y.Px(); got != 160 {
		t.Errorf("the text after a tab is %gpx down the line, want 160 — eight "+
			"stops of one em, and not eight of Courier's twelve pixels", got)
	}
}

// TestTrimmingASpaceOffAnUprightRunMovesItAnEm.
//
// The comparison trims the white space a run begins with and moves the run's
// origin past it, so that two documents that space their words differently are
// still compared by the words. How far to move is the space's advance, and on
// an upright vertical line that is one em rather than whatever the face makes a
// space.
//
// It is checked against the helper rather than through a document because the
// case needs a preserved leading space inside a turned box, and what would be
// wrong is a mark placed one advance out — which is the kind of difference this
// comparison exists to find and would then invent.
func TestTrimmingASpaceOffAnUprightRunMovesItAnEm(t *testing.T) {
	// A face whose space is *not* an em, so that the two answers are different
	// numbers: Ahem's is exactly one and would agree by accident.
	plain := turnedRuns(t, `<div id="d">x</div>`, turnedCSS)
	if len(plain) != 1 || plain[0].Face == nil {
		t.Fatal("the fixture drew no run to take a face from")
	}
	face, size := plain[0].Face, plain[0].Size
	if w := face.Measure(" ", size.Px()); w == size.Px() {
		t.Fatalf("this face's space is one em (%gpx), so the fixture cannot tell "+
			"an em from an advance", w)
	}
	run := DrawText{
		At: Point{}, Text: " x", Face: face, Size: size,
		Sideways: true, Upright: true, Color: style.RGBA{A: 1},
	}
	got := trimRunSpace(run)
	if got.Text != "x" {
		t.Fatalf("the run was trimmed to %q, want \"x\"", got.Text)
	}
	if got.At.Y.Px() != 20 {
		t.Errorf("trimming the space moved the run %gpx down the line, want 20 — "+
			"one em, which is what an upright space advances", got.At.Y.Px())
	}
	if got.At.X != run.At.X {
		t.Errorf("trimming the space moved the run across the line to %v; it "+
			"advances along it", got.At.X)
	}
}
