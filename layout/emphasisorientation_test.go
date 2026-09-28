package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Emphasis marks on a vertical line that holds both kinds of run.
//
// The fixture face is VerticalFallbacks.ttf (see mixedorientation_test.go),
// whose ascent and descent are 20 and 5 at 20px — not an em between them, so
// a run lying along the line and a run standing on it reach different
// distances across it, which is what these tests are about. Its central
// baseline is 7.5 above its alphabetic one. The mark is its own "o"
// (text-emphasis-style: "o"): at 10px its line is 10 over its baseline and 2.5
// under, a band of 12.5, and standing upright it is an em of 10 along the line
// and 5 either side of its middle across it.
//
// So, from the alphabetic baseline towards the line's over side (right, in
// vertical-rl): a run lying along the line reaches 20, and its marks' middle is
// 20 + 6.25 = 26.25 out; an upright run reaches its em's half, 10, past the
// central baseline, 17.5, and its marks' middle is 17.5 + 6.25 = 23.75 out.

const emphasisVerticalCSS = `body { margin: 0 }
	#d { font-family: V; font-size: 20px; writing-mode: vertical-rl;
	     width: 100px; height: 400px; text-emphasis-style: "o" }`

// verticalMarks lays a document out in the fixture face and returns its runs
// and its marks.
func verticalMarks(t *testing.T, set FontSet, htmlSrc, cssSrc string) (*Fragment, []DrawText, []DrawText) {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(1000)
	root := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	ops := Paint(root)
	var runs []DrawText
	for _, op := range ops {
		if v, ok := op.(DrawText); ok {
			runs = append(runs, v)
		}
	}
	return root, runs, marksIn(ops)
}

func markAt(t *testing.T, what string, m DrawText, x, y float64) {
	t.Helper()
	if m.At.X.Px() != x || m.At.Y.Px() != y || !m.Upright || !m.Sideways {
		t.Errorf("%s: the mark is at (%g, %g) upright=%v sideways=%v, want an upright "+
			"mark at (%g, %g)", what, m.At.X.Px(), m.At.Y.Px(), m.Upright, m.Sideways, x, y)
	}
}

// TestAMarkIsSetAgainstItsOwnRun.
//
// "o" lying along the line and "x" standing upright in a span, on one line
// 50px high with room for the marks: the line box is 50..100, its alphabetic
// baseline 12.5 of half-leading and 20 of ascent in from the right, at 67.5,
// and its central baseline 7.5 further right, at 75, which is where the
// upright "x" is drawn from. The "o"'s mark is beyond the "o", 26.25 right of
// the alphabetic baseline: 93.75. The "x"'s mark is beyond the "x", 23.75
// right of it: 91.25. Along the line each is centred on its own character: the
// "o" is 0..10, so its mark's pen is 0; the "x" stands in 10..30, so 15.
//
// Measured from the upright run's own pen and past the element's ascent, as the
// marks of a box holding both kinds were placed when a box was one orientation,
// the "x"'s mark would be 26.25 right of 75: 101.25.
func TestAMarkIsSetAgainstItsOwnRun(t *testing.T) {
	_, runs, marks := verticalMarks(t, verticalFallbacksSet(t),
		`<div id="d">o<span style="text-orientation: upright">x</span></div>`,
		emphasisVerticalCSS+` #d { line-height: 50px }`)
	if len(runs) != 2 || len(marks) != 2 {
		t.Fatalf("drew %d runs and %d marks, want 2 and 2: %+v %+v", len(runs), len(marks), runs, marks)
	}
	if runs[0].At.X.Px() != 67.5 || runs[1].At.X.Px() != 75 || !runs[1].Upright {
		t.Fatalf("the runs are drawn from x=%g and x=%g (upright %v), want 67.5 and an "+
			"upright 75", runs[0].At.X.Px(), runs[1].At.X.Px(), runs[1].Upright)
	}
	markAt(t, "over the lying o", marks[0], 93.75, 0)
	markAt(t, "over the standing x", marks[1], 91.25, 15)
}

// TestTheLeadingForAnUprightRunsMarks.
//
// A 25px line has no leading, and the marks' band of 12.5 has to be found. A
// run lying along the line needs it all over the text, which reaches 20 over
// the baseline and 5 under: the line is 32.5 + 5, its alphabetic baseline
// 32.5 in from the right edge, at 67.5.
//
// An upright run reaches 17.5 over the baseline and 2.5 under — its em hung
// from the central baseline — which leaves 2.5 of the text's 20 and 5 unused
// on each side. The 7.5 still missing goes over (CSS Ruby §3.6, as
// withEmphasis reads it), and the line is 27.5 + 5: the baseline 27.5 in, at
// 72.5, and the upright run drawn from 7.5 right of it, at 80. Its marks'
// middle is 23.75 right of the baseline: 96.25.
func TestTheLeadingForAnUprightRunsMarks(t *testing.T) {
	for _, c := range []struct {
		what, css, text   string
		width, pen, markX float64
	}{
		{"lying along the line", "", "oo", 37.5, 67.5, 93.75},
		{"standing upright", "#d { text-orientation: upright }", "xx", 32.5, 80, 96.25},
	} {
		root, runs, marks := verticalMarks(t, verticalFallbacksSet(t),
			`<div id="d">`+c.text+`</div>`,
			emphasisVerticalCSS+` #d { line-height: 25px } `+c.css)
		line := find(t, root, "d").Lines[0]
		if line.Rect.W.Px() != c.width {
			t.Errorf("%s: the line is %g across, want %g", c.what, line.Rect.W.Px(), c.width)
		}
		if len(runs) != 1 || runs[0].At.X.Px() != c.pen {
			t.Errorf("%s: the run is drawn from %+v, want x=%g", c.what, runs, c.pen)
		}
		if len(marks) != 2 || marks[0].At.X.Px() != c.markX {
			t.Errorf("%s: the marks are %+v, want x=%g", c.what, marks, c.markX)
		}
	}
}

// TestAFallbackRunsMarksAreOnTheElementsLine.
//
// The box's face is the fixture and its "a" is set in Courier by the fallback
// stack, lying along the line like the rest. Courier's central baseline is
// 5.546875 above its alphabetic one to the fixture's 7.5, so its alphabetic
// baseline is 1.953125 above the box's, and its extents — 16.09375 over its
// baseline and 5 under — reach 18.046875 over the box's baseline and 3.046875
// under it. The marks' band is asked for beyond the fixture's 20 and 5, from
// the box's baseline, against those moved extents: they leave 1.953125 short
// on each side, and the band of 12.5 goes over, as for the fixture's own runs.
// So the line is 32.5 + 5, as it is without the Courier letter, its baseline
// at 67.5, and all three marks are on one line 26.25 right of it: 93.75.
//
// Asked before the extents were moved, the marks took their band over
// Courier's unmoved 16.09375 and the move then carried it 1.953125 further
// over: a line 39.453125 across. And measured from the Courier run's own pen,
// which is 1.953125 right of the box's baseline, its mark was off the line of
// the other two by as much.
func TestAFallbackRunsMarksAreOnTheElementsLine(t *testing.T) {
	courier, _ := StandardFonts().Face("Courier", false, false)
	set := fallbackSet{namedFaceSet{family: "V", face: uprightFace(t, "VerticalFallbacks.ttf"),
		standard: StandardFonts()}, courier}
	root, runs, marks := verticalMarks(t, set, `<div id="d">oao</div>`,
		emphasisVerticalCSS+` #d { line-height: normal }`)
	if len(runs) != 3 || runs[1].Face != courier {
		t.Fatalf("drew %+v, want three runs, the middle one in Courier", runs)
	}
	if got := runs[1].At.X.Px(); got != 69.453125 {
		t.Errorf("the Courier run is drawn from x=%g, want 69.453125", got)
	}
	if w := find(t, root, "d").Lines[0].Rect.W.Px(); w != 37.5 {
		t.Errorf("the line is %g across, want 37.5", w)
	}
	if len(marks) != 3 {
		t.Fatalf("drew %d marks, want 3", len(marks))
	}
	for i, m := range marks {
		if m.At.X.Px() != 93.75 {
			t.Errorf("mark %d is at x=%g, want 93.75 — on the element's line", i, m.At.X.Px())
		}
	}
}

// TestABoxsStrutHoldsTheMarksOfEveryKindItsTextHolds.
//
// The block's strut takes the leading its text's marks need, of every kind the
// text holds: here an upright span holding "xx " on the first line and a lying
// "oo" wrapped onto the second (the line is 45 long: the two upright letters
// are an em each, and the space hangs). The first line holds only upright
// text, whose own marks need 27.5 over the baseline; the strut's is the larger
// of the two kinds', 32.5, so the first line is 37.5 across like the second,
// and the upright run is drawn 7.5 right of the baseline at 67.5: 75.
//
// A strut that asked for one kind only would take the lying kind's 32.5 for a
// box holding only upright text — the case above — and the upright kind's
// 27.5 for one holding only upright text as it sees it.
func TestABoxsStrutHoldsTheMarksOfEveryKindItsTextHolds(t *testing.T) {
	root, runs, _ := verticalMarks(t, verticalFallbacksSet(t),
		`<div id="d"><span style="text-orientation: upright">xx </span>oo</div>`,
		emphasisVerticalCSS+` #d { line-height: 25px; height: 45px }`)
	lines := find(t, root, "d").Lines
	if len(lines) != 2 || lines[0].Rect.W.Px() != 37.5 || lines[1].Rect.W.Px() != 37.5 {
		t.Fatalf("the lines are %+v, want two 37.5 across", lines)
	}
	if runs[0].Text != "xx" || runs[0].At.X.Px() != 75 {
		t.Errorf("the upright run is %q at x=%g, want \"xx\" at 75", runs[0].Text, runs[0].At.X.Px())
	}
}

// TestOnlyAMarkedCharacterAsksForItsKindOfLeading.
//
// Which kinds of run a box's text holds is asked of the characters that take a
// mark: an ideographic full stop stands upright and takes none (§3.1 marks no
// punctuation but the symbols it names), so a lying "o" beside it is a box of
// one kind, and an ideograph, which takes one, makes it two.
//
// It shows only where the upright kind reaches further than the lying one,
// which is a face whose ascent and descent come to less than an em: the
// upright em box then reaches past them. VerticalHhea.ttf is such a face, as
// this engine reads it — an ascent of 14 at 20px and a descent of -6 — so its
// central baseline is 10 over the alphabetic one and an upright run reaches 20
// over it and 0 under, where the lying text reaches 14 and -6. Its "o" is the
// mark: 7 over the mark's baseline and -3 under at 10px, a band of 4.
//
// On an 8px line, which is the text's own 14 - 6, the lying kind has no
// leading and takes the band over: 18 over and -6 under, a line 12 across. The
// upright kind is 6 short on each side and takes the band over its own reach:
// 24 over and 0 under, a line 24 across.
func TestOnlyAMarkedCharacterAsksForItsKindOfLeading(t *testing.T) {
	set := namedFaceSet{family: "V", face: uprightFace(t, "VerticalHhea.ttf"), standard: StandardFonts()}
	for _, c := range []struct {
		text  string
		width float64
	}{
		{"o", 12},
		{"o。", 12},
		{"o日", 24},
	} {
		root, _, _ := verticalMarks(t, set, `<div id="d">`+c.text+`</div>`,
			emphasisVerticalCSS+` #d { line-height: 8px }`)
		if w := find(t, root, "d").Lines[0].Rect.W.Px(); w != c.width {
			t.Errorf("%q: the line is %g across, want %g", c.text, w, c.width)
		}
	}
}

// TestACompositionTakesOneMark.
//
// A text-combine-upright composition is one character, U+FFFC, to its marks
// (CSS Writing Modes §9.1.2; CSS Text Decoration 3 §3.1 marks a character, and
// the composition is one): one mark, centred on its em along the line and set
// beyond it as an upright character's, whatever its text. On the 50px line of
// the first test, "o", a composition of "oo" and "o": the composition's square
// is 10..30 along the line, so its mark's pen is 15, and it is 23.75 right of
// the baseline at 67.5, 91.25, where the lying letters' marks are 26.25 right
// of it at 93.75. Its text, three characters wide by their own letters, gets
// none of its own.
func TestACompositionTakesOneMark(t *testing.T) {
	_, _, marks := verticalMarks(t, verticalFallbacksSet(t),
		`<div id="d">o<span style="text-combine-upright: all">oo</span>o</div>`,
		emphasisVerticalCSS+` #d { line-height: 50px }`)
	if len(marks) != 3 {
		t.Fatalf("drew %d marks, want 3: %+v", len(marks), marks)
	}
	markAt(t, "over the first o", marks[0], 93.75, 0)
	markAt(t, "over the composition", marks[1], 91.25, 15)
	markAt(t, "over the last o", marks[2], 93.75, 30)
}

// TestACompositionsLeadingIsAnUprightCharacters: a box holding only a
// composition asks for the leading an upright character's marks need — on the
// 25px line of TestTheLeadingForAnUprightRunsMarks, 27.5 over the baseline and
// 5 under, a line 32.5 across — and not the 37.5 its letters would ask for
// lying along the line.
func TestACompositionsLeadingIsAnUprightCharacters(t *testing.T) {
	root, _, _ := verticalMarks(t, verticalFallbacksSet(t),
		`<div id="d"><span style="text-combine-upright: all">oo</span></div>`,
		emphasisVerticalCSS+` #d { line-height: 25px }`)
	if w := find(t, root, "d").Lines[0].Rect.W.Px(); w != 32.5 {
		t.Errorf("the line is %g across, want 32.5", w)
	}
}

// TestAMarkInAHorizontalInlineBlockOnAVerticalLineLiesOverItsText.
//
// A horizontal inline-block standing on a vertical line is laid out across the
// page, and its text is in a horizontal typographic mode: its marks are over
// it and lie along its line, as on any horizontal page — not upright beside a
// column. Its own 25px line grows by the marks' band to 37.5, the text's
// baseline 32.5 below its top; the marks' baseline is over the text's 20 and
// under the mark's 2.5: 22.5 above the text's. Each mark, the fixture's "o" at
// 10px, is 5 wide and centred over its 10px letter: 2.5 into it.
func TestAMarkInAHorizontalInlineBlockOnAVerticalLineLiesOverItsText(t *testing.T) {
	root, runs, marks := verticalMarks(t, verticalFallbacksSet(t),
		`<div id="d">o<span id="ib">xx</span></div>`,
		emphasisVerticalCSS+` #d { line-height: 50px }
	#ib { display: inline-block; writing-mode: horizontal-tb; line-height: 25px }`)
	var across *DrawText
	for i := range runs {
		if !runs[i].Sideways {
			across = &runs[i]
		}
	}
	if across == nil || len(marks) != 3 {
		t.Fatalf("drew %+v and %d marks", runs, len(marks))
	}
	if h := find(t, root, "ib").BorderRect.W.Px(); h != 20 {
		t.Fatalf("the inline-block is %g across, want 20", h)
	}
	for i, m := range marks[1:] {
		if m.Sideways || m.Upright {
			t.Errorf("mark %d over the inline-block's text is sideways=%v upright=%v, "+
				"want it lying along the page", i, m.Sideways, m.Upright)
		}
		if want := across.At.X.Px() + 2.5 + float64(10*i); m.At.X.Px() != want ||
			m.At.Y.Px() != across.At.Y.Px()-22.5 {
			t.Errorf("mark %d is at (%g, %g), want (%g, %g)", i, m.At.X.Px(), m.At.Y.Px(),
				want, across.At.Y.Px()-22.5)
		}
	}
}
