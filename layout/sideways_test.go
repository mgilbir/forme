package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Writing Modes §3.1's two sideways modes.
//
// They are the other half of writing-mode, and what separates them from the two
// vertical modes beside them is small and stated twice below: every character
// lies along the line, whatever UAX #50 would have made of it, and sideways-lr
// turns the other way.
//
// The fixture is the vertical tests' — Courier at 20px on a 20px line, in a
// 60x100 box at the origin — so that every number here is a whole one and a
// wrong axis is a wrong number rather than a rounding. The box is 100 tall, so
// the frame its content is laid out in is 100 wide, and a line that begins at
// the foot of the page begins at exactly 100.

const sidewaysLRCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: sideways-lr; width: 60px; height: 100px }`

const sidewaysRLCSS = `body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     writing-mode: sideways-rl; width: 60px; height: 100px }`

// TestASidewaysLRBlockRunsItsTextUpThePage.
//
// The three facts the vertical test pins, each with the sign the other turn
// gives it. The text goes *up*: two lines share a y and it is the foot of the
// box, not the head. The lines stack *left to right*, like vertical-lr's. And
// the runs say which turn they took, because the two turns are the one thing
// about this mode that a permutation of the four sides cannot carry.
func TestASidewaysLRBlockRunsItsTextUpThePage(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd<br>ef</div>`, sidewaysLRCSS)
	if len(runs) != 2 {
		t.Fatalf("the fixture drew %d runs, want 2 — it cannot say what it means to say", len(runs))
	}
	first, second := runs[0], runs[1]
	for _, r := range runs {
		if !r.Sideways {
			t.Errorf("the run %q was not marked sideways, so a backend would draw "+
				"it across the page", r.Text)
		}
		if !r.Anticlockwise {
			t.Errorf("the run %q was not marked anticlockwise, so a backend would "+
				"stand its letters upside down and start them at the wrong end", r.Text)
		}
	}
	// The block's content box is the whole box: no border, no padding. Its left
	// edge is at 0, and the first line's baseline sits one ascent in from it —
	// measured from the *left*, because that is the side the glyphs' up is on
	// after this turn.
	//
	// Which is asserted against vertical-rl rather than against a number,
	// because the ascent is the face's and a number here would be Courier's.
	// The same face over the same line height puts the baseline the same
	// distance from the line box's block-start edge in both modes; the modes
	// disagree about which edge that is, and the box is 60 wide, so the two
	// distances from the page's own edges add up to 60. Under a baseline
	// measured from the wrong side they add up to 20 more than twice the
	// ascent, which is 60 only for a face whose ascent is half a line.
	if x := first.At.X.Px(); x <= 0 || x >= 20 {
		t.Errorf("the first line's baseline is at x=%g, want it between the box's "+
			"left edge at 0 and the next line's column at 20", x)
	}
	clockwise := turnedRuns(t, `<div id="d">abcd<br>ef</div>`, turnedCSS)
	if len(clockwise) != 2 {
		t.Fatalf("the vertical-rl fixture drew %d runs, want 2", len(clockwise))
	}
	if got := first.At.X.Add(clockwise[0].At.X).Px(); got != 60 {
		t.Errorf("the two turns put their first baseline %gpx apart in total, want "+
			"the box's 60 — one measures the ascent in from the left edge and the "+
			"other in from the right, and they are the same ascent", got)
	}
	if got := second.At.X.Sub(first.At.X).Px(); got != 20 {
		t.Errorf("the second line is %gpx to the right of the first, want 20 — "+
			"sideways-lr stacks its lines from the left edge rightwards by a "+
			"line height", got)
	}
	if first.At.Y.Px() != 100 || second.At.Y.Px() != 100 {
		t.Errorf("the two lines begin at y=%g and %g, want 100 and 100 — every "+
			"line of a sideways-lr block starts at the foot of the box, which "+
			"here is 100px down", first.At.Y.Px(), second.At.Y.Px())
	}
}

// TestTheGlyphsOfASidewaysLRBlockGoUpTheLine.
//
// The run above is one operation whichever way its pen goes, so it cannot tell
// a turn from a translation. This asks where the *glyphs* land, which is the
// question the reftest oracle asks and the one a turn taken the wrong way
// fails.
func TestTheGlyphsOfASidewaysLRBlockGoUpTheLine(t *testing.T) {
	runs := turnedRuns(t, `<div id="d">abcd</div>`, sidewaysLRCSS)
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
				"sideways line shares its baseline's x", i, m.x, marks[0].x)
		}
		if got := marks[0].y.Sub(m.y).Px(); got != float64(i)*12 {
			t.Errorf("letter %d is %gpx above the first, want %g — Courier's advance "+
				"is 12px at 20px and it goes up the page", i, got, float64(i)*12)
		}
	}
	if marks[0].y.Px() != 100 {
		t.Errorf("the first letter is at y=%g, want the box's foot at 100", marks[0].y.Px())
	}
}

// TestTheTwoTurnsStandTheirGlyphsOppositeWaysUp.
//
// Which way is up, asked of something that has a side: an underline sits below
// the baseline, and "below" after a quarter turn is a direction on the page.
// A clockwise turn puts the glyphs' up to the right, so the rule is to the left
// of the text; the other turn puts it to the left, so the rule is to the right.
//
// It is the assertion the reftest suite cannot make. Both documents of a
// sideways-lr reftest go through the same turn, so a turn taken consistently
// the wrong way agrees with itself and the suite sees nothing — which is why
// this is arithmetic in a unit test. See forme's writing-mode notes.
func TestTheTwoTurnsStandTheirGlyphsOppositeWaysUp(t *testing.T) {
	rule := func(t *testing.T, css string) (Point, Rect) {
		t.Helper()
		ops := Paint(layoutOf(t, 400, `<div id="d">ab</div>`,
			css+`
	#d { text-decoration: underline }`))
		var at Point
		var seen bool
		for _, op := range ops {
			if v, ok := op.(DrawText); ok && v.Text == "ab" {
				at, seen = v.At, true
			}
		}
		if !seen {
			t.Fatal("the fixture drew no text")
		}
		var bands []Rect
		for _, op := range ops {
			if r, ok := op.(FillRect); ok && r.Overhang && !r.Rect.Empty() {
				bands = append(bands, r.Rect)
			}
		}
		if len(bands) != 1 {
			t.Fatalf("the fixture drew %d decoration bands, want 1", len(bands))
		}
		return at, bands[0]
	}

	at, band := rule(t, turnedCSS)
	if band.Right() > at.X {
		t.Errorf("in vertical-rl the underline runs to x=%g and the baseline is at "+
			"x=%g; a clockwise turn puts the glyphs' up to the right, so the rule "+
			"is to the left of them", band.Right().Px(), at.X.Px())
	}

	at, band = rule(t, sidewaysLRCSS)
	if band.X < at.X {
		t.Errorf("in sideways-lr the underline starts at x=%g and the baseline is "+
			"at x=%g; the other turn puts the glyphs' up to the left, so the rule "+
			"is to the right of them", band.X.Px(), at.X.Px())
	}
}

// TestASidewaysModeLaysEveryCharacterAlongTheLine.
//
// §5.1: text-orientation has no effect in a horizontal typographic mode, and
// both sideways modes are one. So a run of CJK, which vertical-rl stands
// upright because "mixed" says to, lies along the line here — and the engine
// says so on the run, because an upright run is measured at one em a character
// and a turned one at the face's own advances.
func TestASidewaysModeLaysEveryCharacterAlongTheLine(t *testing.T) {
	upright := turnedRuns(t, `<div id="d">国国</div>`, turnedCSS)
	along := turnedRuns(t, `<div id="d">国国</div>`, sidewaysRLCSS)
	if len(upright) == 0 || len(along) == 0 {
		t.Fatalf("the fixtures drew %d and %d runs, want text in both",
			len(upright), len(along))
	}
	// Every run, because a CJK paragraph is set in a fallback face and may reach
	// the page as more than one. The claim is about all of them either way.
	for _, r := range upright {
		if !r.Upright {
			t.Fatalf("vertical-rl did not stand %q upright, so this test has "+
				"nothing to contrast with", r.Text)
		}
	}
	for _, r := range along {
		if r.Upright {
			t.Errorf("sideways-rl stood %q upright; every character of a sideways "+
				"mode lies along the line", r.Text)
		}
		if !r.Sideways || r.Anticlockwise {
			t.Errorf("sideways-rl drew %q sideways=%v anticlockwise=%v, want the "+
				"same clockwise turn vertical-rl takes", r.Text, r.Sideways, r.Anticlockwise)
		}
	}
}

// TestASidewaysModeIgnoresTheTwoPropertiesThatCannotApply.
//
// text-orientation (§5.1) and text-combine-upright (§9.1) both "have no effect
// in horizontal typographic modes", and a sideways mode is one. Refusing to
// turn a box because it declared either would be refusing it for asking for
// something that was never going to happen — and it is exactly what
// text-autospace-004 writes, with a comment saying so.
func TestASidewaysModeIgnoresTheTwoPropertiesThatCannotApply(t *testing.T) {
	for _, decl := range []string{
		"text-orientation: upright",
		"text-combine-upright: all",
	} {
		for _, mode := range []struct{ name, css string }{
			{"sideways-rl", sidewaysRLCSS},
			{"sideways-lr", sidewaysLRCSS},
		} {
			// On a descendant, which is where the refusal is decided: the
			// walk compares each box against the one the turn started at, so
			// a declaration on that box itself agrees with itself and never
			// reaches the clause at all.
			runs := turnedRuns(t, `<div id="d">a<span style="`+decl+`">b</span></div>`, mode.css)
			if len(runs) == 0 {
				t.Fatalf("%s with %q drew nothing", mode.name, decl)
			}
			for _, r := range runs {
				if !r.Sideways {
					t.Errorf("%s refused to turn because of %q, which has no effect "+
						"in a horizontal typographic mode", mode.name, decl)
				}
				if r.Upright {
					t.Errorf("%s stood %q upright because of %q", mode.name, r.Text, decl)
				}
			}
		}
	}
	// The control: the same declaration in a *vertical* mode still refuses,
	// because there it does mean something. Without this the test above passes
	// for an engine that has stopped reading either property at all.
	for _, decl := range []string{
		"text-orientation: upright",
		"text-combine-upright: all",
	} {
		runs := turnedRuns(t, `<div id="d">a<span style="`+decl+`">b</span></div>`, turnedCSS)
		if len(runs) == 0 {
			t.Fatalf("%q in a vertical box drew nothing", decl)
		}
		for _, r := range runs {
			if r.Sideways {
				t.Errorf("%q inside a vertical-rl box was turned anyway, so the "+
					"case above proves nothing about sideways modes", decl)
				break
			}
		}
	}
}

// TestTheFourTurnsPutARectangleWhereItsModeSays is the arithmetic on its own,
// away from any document.
//
// turnRect is the whole of the geometry, and a document exercises it through so
// much else that a sign error in one mode reads as a layout bug in another.
// Here it is four numbers in and four out.
func TestTheFourTurnsPutARectangleWhereItsModeSays(t *testing.T) {
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	// A frame whose block extent is 60 and whose inline extent is 100, which is
	// the fixture above: a 60x100 box on the page.
	in := Size{W: u(60), H: u(100)}
	// A rectangle 5 along the line and 10 into the block, 30 long and 20 thick.
	r := Rect{X: u(5), Y: u(10), W: u(30), H: u(20)}

	for _, c := range []struct {
		mode writingMode
		want Rect
	}{
		// Both -rl modes measure the block coordinate back from the right edge:
		// 60 - 10 - 20 = 30. The inline coordinate is untouched.
		{verticalRL, Rect{X: u(30), Y: u(5), W: u(20), H: u(30)}},
		{sidewaysRL, Rect{X: u(30), Y: u(5), W: u(20), H: u(30)}},
		// vertical-lr takes the block coordinate as it stands.
		{verticalLR, Rect{X: u(10), Y: u(5), W: u(20), H: u(30)}},
		// sideways-lr does too, and mirrors the *inline* one instead:
		// 100 - 5 - 30 = 65.
		{sidewaysLR, Rect{X: u(10), Y: u(65), W: u(20), H: u(30)}},
	} {
		if got := turnRect(r, c.mode, in); got != c.want {
			t.Errorf("%v turned %v into %v, want %v", c.mode, r, got, c.want)
		}
	}
}

// TestARaisedRunInASidewaysLRBlockMovesTowardsTheLeftOfItsLine.
//
// §10.8.1's vertical-align is a distance off the baseline, and off the baseline
// after a quarter turn is a direction on the page — the *opposite* direction for
// the two turns, because they stand their glyphs opposite ways up. A raised run
// moves right in vertical-rl and left here.
//
// It needs a fixture of its own for the reason
// TestARaisedRunInAVerticalBlockMovesTowardsTheTopOfItsLine gives: the offset is
// zero in ordinary text, so a paragraph with nothing raised in it exercises this
// arithmetic with a number that cannot have a wrong sign. The fixture is that
// test's, so the two read as the pair they are.
func TestARaisedRunInASidewaysLRBlockMovesTowardsTheLeftOfItsLine(t *testing.T) {
	runs := turnedRuns(t,
		`<div id="d">a<span style="vertical-align: 4px">b</span><span style="vertical-align: -4px">c</span></div>`,
		sidewaysLRCSS)
	if len(runs) != 3 {
		t.Fatalf("the fixture drew %d runs, want 3 — a, b and c", len(runs))
	}
	plain, raised, lowered := runs[0], runs[1], runs[2]
	if got := raised.At.X.Sub(plain.At.X).Px(); got != -4 {
		t.Errorf("a run raised 4px is %gpx to the right of the one beside it, want -4 "+
			"— the top of a sideways-lr line is its left-hand side", got)
	}
	if got := lowered.At.X.Sub(plain.At.X).Px(); got != 4 {
		t.Errorf("a run lowered 4px is %gpx to the right of the one beside it, want 4", got)
	}
	// And neither of them moved along the line, which is the other half of
	// saying the displacement went into the right axis. Up the line here, so
	// each run is one Courier advance *above* the last.
	if plain.At.Y.Sub(raised.At.Y).Px() != 12 || raised.At.Y.Sub(lowered.At.Y).Px() != 12 {
		t.Errorf("the three runs are at y=%v, %v and %v; each should be one Courier "+
			"advance further up the line than the last, and vertical-align "+
			"should not have moved any of them along it",
			plain.At.Y, raised.At.Y, lowered.At.Y)
	}
}
