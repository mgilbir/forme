package layout

import (
	"strings"
	"testing"
)

// text-align, CSS 2.1 §16.2.
//
// Every position below is arithmetic that can be read rather than a number
// recorded from a run: Courier is 600/1000, so a character at 20px is 12px wide
// and a six-character word is 72px. A recorded number would agree just as well
// with a wrong implementation.

// lineX returns the x of the first run of the first line of an element.
func lineX(t *testing.T, root *Fragment, id string) float64 {
	t.Helper()
	f := find(t, root, id)
	if len(f.Lines) == 0 || len(f.Lines[0].Runs) == 0 {
		t.Fatalf("#%s has no line runs to align", id)
	}
	return f.Lines[0].Runs[0].X.Px()
}

const alignCSS = `#p { font-family: Courier; font-size: 20px; width: 300px }`

func TestTextAlignPositionsTheLine(t *testing.T) {
	// "abcdef" is six characters: 6 x 0.6 x 20 = 72px in a 300px line.
	// left 0, right 228, centre 114.
	cases := map[string]float64{
		"left":   0,
		"start":  0,
		"right":  228,
		"end":    228,
		"center": 114,
	}
	for value, want := range cases {
		root := layoutOf(t, 600, `<div id="p">abcdef</div>`,
			alignCSS+` #p { text-align: `+value+` }`)
		if got := lineX(t, root, "p"); got != want {
			t.Errorf("text-align:%s put the line at %gpx, want %g", value, got, want)
		}
	}
}

func TestTextAlignIsInherited(t *testing.T) {
	// The property is inherited, which is how "body { text-align: center }"
	// works at all. A version that read it only off the element declaring it
	// would leave every paragraph flush left.
	root := layoutOf(t, 600, `<div id="outer"><div id="p">abcdef</div></div>`,
		alignCSS+` #outer { text-align: center }`)
	if got := lineX(t, root, "p"); got != 114 {
		t.Errorf("an inherited text-align:center put the line at %gpx, want 114", got)
	}
}

// TestTextAlignIgnoresUnconditionallyHangingSpace is §4.1.2's hang at its full
// strength: a line that ended at a *soft wrap* leaves its preserved trailing
// space outside the width the line is aligned at.
//
// The line has to be one that wrapped, and that is the whole point of the second
// word. A trailing space at the end of the content hangs only *conditionally* —
// see the test below — so a document written without the wrap would be asking
// this rule a question the other rule answers, which is the shape of test this
// repository has been caught by twice.
func TestTextAlignIgnoresUnconditionallyHangingSpace(t *testing.T) {
	// In 100px — eight characters and a third of Courier at 20px — "abcdef  "
	// is 96 wide and the second "abcdef" does not fit after it, so the first
	// line ends at a soft wrap with two preserved spaces on it. Aligned at 72
	// rather than 96, the line centres at (100-72)/2 = 14.
	root := layoutOf(t, 600, `<div id="p">abcdef  abcdef</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 100px;
		      text-align: center; white-space: pre-wrap }`)
	lines := linesOf(t, root, "p")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), lineTexts(lines))
	}
	if got := lines[0].Runs[0].X.Px(); got != 14 {
		t.Errorf("a soft-wrapped line with two hanging spaces centred at %gpx, "+
			"want 14 — the hanging space is being counted", got)
	}
}

// TestTextAlignCountsAConditionallyHangingSpace is the other half, and it is the
// half that is easy to get backwards — this engine had it backwards, and a test
// asserting the wrong answer pinned it there.
//
// §4.1.2: preserved white space at the end of a line hangs unconditionally
// "unless the sequence is followed by a forced line break, in which case it must
// conditionally hang the sequence instead", and something that conditionally
// hangs "hangs only if it does not otherwise fit in the line". The end of the
// content is such a break — the specification's own example is a paragraph whose
// only content is " 0 ", with no <br> in it anywhere.
func TestTextAlignCountsAConditionallyHangingSpace(t *testing.T) {
	// The example from §4.1.2, in Courier rather than in ch: five characters is
	// 60px, " 0 " is three of them, and centring 36 in 60 puts the line at 12.
	// Aligning it as though the trailing space hung would put it at 18 — half a
	// character off, which is exactly what the specification says must not
	// happen.
	root := layoutOf(t, 600, `<div id="p"> 0 </div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px;
		      text-align: center; white-space: pre-wrap }`)
	if got := lineX(t, root, "p"); got != 12 {
		t.Errorf("the specification's centred \" 0 \" example is at %gpx, want 12", got)
	}

	// And a space that does not fit hangs even here, which is what makes the
	// rule conditional rather than simply off — but it is the space that does
	// not fit and not the sequence it belongs to. §4.1.2's next sentence is the
	// one that says so: the UA "may also visually collapse the character advance
	// widths of any that would otherwise overflow", so a sequence that half fits
	// counts up to the line's edge and hangs the rest.
	//
	// "abcdef  " is 96 in a line 84 wide. Six characters and one space are 84 —
	// exactly the line — and the second space is what overflows. So the line is
	// as wide as the space it has, a right-aligned line has nothing left over,
	// and it starts at 0.
	//
	// This assertion said 12 and was wrong, which is worth recording because it
	// is the second time this rule has been pinned backwards here. The evidence
	// is not a reading: css-text/white-space/white-space-pre-wrap-trailing-
	// spaces-001 centres "    S" followed by thirty-two spaces in nine
	// characters, and its reference puts the S at the fourth character — which
	// is where a line that fills its width puts it and two characters from where
	// a line of five puts it. Nothing in the suite requires the other answer.
	root = layoutOf(t, 600, `<div id="p">abcdef  </div>`,
		`#p { font-family: Courier; font-size: 20px; width: 84px;
		      text-align: right; white-space: pre-wrap }`)
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("a right-aligned line whose trailing spaces overflow starts at "+
			"%gpx, want 0 — one space fills the line and only the second hangs", got)
	}

	// The sequence that overflows *entirely* is the case the clamp has to get
	// right at its other end: "abcdef" alone is 72 of the 84, and eight spaces
	// after it are 96 more. The line is still only as wide as it has room for,
	// so it still starts at 0 — and the six characters are not pushed off the
	// left edge by a line measured at 168.
	root = layoutOf(t, 600, `<div id="p">abcdef        </div>`,
		`#p { font-family: Courier; font-size: 20px; width: 84px;
		      text-align: right; white-space: pre-wrap }`)
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("a right-aligned line whose trailing spaces overflow far starts "+
			"at %gpx, want 0", got)
	}

	// The centred form of the same thing, which is the shape the suite measures
	// and the one where the two readings differ by a visible two characters.
	// Nine characters is 108; "    S" is 60 of it and the thirty-two spaces
	// after it are 384 more, so the line fills its width and does not move.
	// Counting it as five characters would centre it 24px in.
	root = layoutOf(t, 600,
		`<div id="p">    S                                </div>`,
		`#p { font-family: Courier; font-size: 20px; width: 108px;
		      text-align: center; white-space: pre-wrap }`)
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("a centred line of five characters and thirty-two spaces starts "+
			"at %gpx, want 0 — the spaces fill the line before they hang", got)
	}
}

// TestAConditionalHangIsMeasuredOnARightToLeftLine is the clamp's other half,
// and it is the half a left-to-right document cannot show.
//
// Going one way, the clamp to the line's width changes nothing that can be seen:
// a line longer than the space it has gets no slack either way, and a line that
// fits is its own length either way. What the clamp decides is *how far past the
// edge the sequence hangs*, and that only moves anything where the hang is at the
// left — which is a right-to-left line, where §4.1.2's hang goes off the start
// edge and the content has to be pulled back over it.
//
// Measured over the whole suite, dropping the clamp moves nothing at all. It is
// not dead: it moves this box by sixty pixels, which is the width of the part of
// the sequence that hangs.
func TestAConditionalHangIsMeasuredOnARightToLeftLine(t *testing.T) {
	// Five characters of room, two of Hebrew and eight preserved spaces after
	// it. The line is 120 long in 60 of room, so 60 of the spaces hang; rule L1
	// gives them the paragraph's own level, so they are drawn leftmost and the
	// word sits at the line's right edge, from 36 to 60.
	root := layoutOf(t, 600, `<div id="p" dir="rtl">`+hebrewAB+`        </div>`,
		`#p { font-family: Courier; font-size: 20px; line-height: 20px;
		      width: 60px; white-space: pre-wrap }`)
	runs := runsOf(t, root, "p")
	if got := runAt(t, runs, hebrewAB).X.Px(); got != 36 {
		t.Errorf("the word on a right-to-left line with a hanging sequence is at "+
			"%gpx, want 36 — its right edge should be the line's, with the part of "+
			"the sequence that does not fit hanging off the left", got)
	}
}

func TestTextAlignDoesNotMoveAnOverfullLine(t *testing.T) {
	// A line wider than the space it has overflows to the right whatever the
	// alignment says. Centring it would push it off the left edge as well, which
	// loses content rather than moving it.
	root := layoutOf(t, 600, `<div id="p">abcdefghijklmnopqrstuvwxyz</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 40px;
		      white-space: nowrap; text-align: center }`)
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("an overfull centred line was moved to %gpx; it should stay at 0", got)
	}
}

func TestTextAlignMovesAtomicInlines(t *testing.T) {
	// An inline-block is placed as a child of the block rather than as a run, so
	// an implementation that shifted only the runs would centre the text and
	// leave the picture behind — the two would come apart, which is worse than
	// not aligning at all.
	root := layoutOf(t, 600, `<div id="p"><span id="box"></span></div>`,
		`#p { width: 300px; text-align: center; font-size: 20px }
		 #box { display: inline-block; width: 100px; height: 10px }`)
	// The fragment's rectangle is in page coordinates, so the offset is measured
	// from the block's own content edge rather than from the page — otherwise
	// the assertion is really about the body's margin.
	box := find(t, root, "box")
	within := box.BorderRect.X.Sub(find(t, root, "p").ContentRect().X)
	// 300 - 100 = 200 of slack, half of it is 100.
	if got := within.Px(); got != 100 {
		t.Errorf("a centred inline-block sits %gpx into its block, want 100", got)
	}
}

// TestTextAlignJustifyIsNotReported.
//
// It was, for as long as it was not implemented. It is implemented now, and a
// line with nowhere to put the slack — one long word — is left where "start"
// puts it, which is what CSS Text 3 §7.3 requires of a line with no expansion
// opportunity. Reporting that would be this engine calling a conforming
// rendering a limitation, which is the fault the finding vocabulary exists to
// avoid rather than to commit.
func TestTextAlignJustifyIsNotReported(t *testing.T) {
	for _, doc := range []string{
		`<div id="p">one two three four five six seven eight nine ten</div>`,
		`<div id="p">aaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbb</div>`,
	} {
		rec := NewRecorder(nil)
		built := Build(Input{
			HTML: doc,
			CSS:  []Stylesheet{{Source: `#p { font-family: Courier; font-size: 20px; width: 80px; text-align: justify }`}},
		})
		Layout(built.Root, Size{W: picPx(600), H: picPx(10000)}, nil, rec)
		for _, f := range rec.Findings() {
			if f.Property == "text-align" {
				t.Errorf("%s reported %s", doc, f.Error())
			}
		}
	}
}

// TestARightAlignedLineTooLongHangsOffTheStart is §16.2 applied to a line that
// does not fit.
//
// Alignment places the line box inside the block, and a line wider than the
// block is still placed: its right edge stays at the block's right edge and
// what does not fit hangs off the left. Reading "no room to distribute" as "do
// not move it" sets such a line flush left instead — so it overflows the way a
// left-aligned one would, and the two alignments become the same declaration
// for exactly the text that most needs them apart.
//
// It is a right-to-left box here because that is how the suite reaches it:
// direction alone makes "start" mean right, and an absolutely positioned box
// whose width shrinks to fit is then narrower than a child that a max-width
// has cut — absolute-non-replaced-width-021 to -024.
func TestARightAlignedLineTooLongHangsOffTheStart(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 40px;
	              white-space: nowrap; %s }`
	// Courier is 600/1000, so a character at 20px is 12 wide and six of them
	// are 72 in 40 of room: the line is 32 too long, and aligning its right edge
	// with the block's puts it at -32.
	for _, c := range []struct {
		what, decl string
		want       float64
	}{
		{"text-align: right", `text-align: right`, -32},
		{"direction: rtl", `direction: rtl`, -32},
		// Centring is the exception and stays put: half of it would go off the
		// start edge, which on a page is unreachable rather than merely outside.
		{"text-align: center", `text-align: center`, 0},
		{"text-align: left", `text-align: left`, 0},
	} {
		root := layoutOf(t, 600, `<div id="p">abcdef</div>`,
			strings.Replace(css, "%s", c.decl, 1))
		if got := lineX(t, root, "p"); got != c.want {
			t.Errorf("with %s an overfull line is at %gpx, want %g",
				c.what, got, c.want)
		}
	}

	// And a box whose overflow can be scrolled keeps its content reachable:
	// what goes off the start edge cannot be scrolled back to, so the line is
	// left where it begins however it is aligned.
	root := layoutOf(t, 600, `<div id="p">abcdef</div>`,
		strings.Replace(css, "%s", `text-align: right; overflow: auto`, 1))
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("an overfull right-aligned line in a scrollable box is at %gpx, "+
			"want 0 — what goes off the start edge could never be scrolled to", got)
	}
}

// Which edge an overfull line overflows past, and why the property does not
// decide it.
//
// A line wider than the room it has cannot be aligned: wherever it is put, it
// runs off one side. What settles which side is the *direction*, not the
// alignment — the line is pinned to the block's start edge and runs off the
// end, because what goes past the start edge of a scrollable box is unreachable
// (scrolling only ever goes the other way) and on a page it is simply lost.
//
// In a right-to-left block the start edge is the right, and that half was
// missing: every alignment sent an overfull line off to the right, which is the
// left-to-right answer. The suite's trailing-space-and-text-alignment-rtl-002
// draws five textareas with five different text-aligns and the same rendering
// in each, which is the whole assertion.

// overfullRTL lays out one overfull line in a scrollable right-to-left block
// and returns where its run begins, measured from the block's content edge.
func overfullRTL(t *testing.T, align string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p" dir="rtl">abcdefghij</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px;
		      white-space: nowrap; overflow-x: auto; text-align: `+align+` }`)
	return lineX(t, root, "p")
}

// TestAnOverfullLineInARightToLeftBlockRunsOffTheLeft. Ten Courier characters
// at 20px are 120px in a 60px block, so the line is 60px too wide: its right
// edge belongs against the block's right edge and the rest hangs off the left,
// which puts the run 60px *before* the content edge.
func TestAnOverfullLineInARightToLeftBlockRunsOffTheLeft(t *testing.T) {
	for _, align := range []string{"left", "right", "center", "start", "end"} {
		if got := overfullRTL(t, align); got != -60 {
			t.Errorf("text-align:%s put an overfull right-to-left line at %gpx, "+
				"want -60 — its right edge is the block's, whatever the alignment "+
				"says, because the right is the edge it starts at", align, got)
		}
	}
}

// TestAnOverfullLineInALeftToRightBlockStillRunsOffTheRight is the half that
// already worked and must not move: the same document read the other way round.
func TestAnOverfullLineInALeftToRightBlockStillRunsOffTheRight(t *testing.T) {
	for _, align := range []string{"left", "right", "center", "start", "end"} {
		root := layoutOf(t, 600, `<div id="p">abcdefghij</div>`,
			`#p { font-family: Courier; font-size: 20px; width: 60px;
			      white-space: nowrap; overflow-x: auto; text-align: `+align+` }`)
		if got := lineX(t, root, "p"); got != 0 {
			t.Errorf("text-align:%s put an overfull left-to-right line at %gpx, want 0",
				align, got)
		}
	}
}

// TestAPageKeepsAnOverfullRightAlignedLineAgainstItsRightEdge. Where there is
// no scrolling the reasoning above does not apply, and §16.2's literal reading
// stands: a right-aligned line keeps its right edge at the block's, and what
// does not fit hangs off the left. This is the case the scrollable rule is an
// exception to, and it must stay an exception.
func TestAPageKeepsAnOverfullRightAlignedLineAgainstItsRightEdge(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">abcdefghij</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px;
		      white-space: nowrap; text-align: right }`)
	if got := lineX(t, root, "p"); got != -60 {
		t.Errorf("an overfull right-aligned line on a page is at %gpx, want -60", got)
	}
}

// TestAnOverfullCentredLineStartsWhereItsDirectionStarts. Centring is refused
// for a line that does not fit — it would push the line off the start edge as
// well as the end — and what is left is "where it starts", which on a page is
// the same question as above and has the same answer: the left in a
// left-to-right block and the right in a right-to-left one.
//
// No scrolling here, so this is the rule in its own right rather than the
// scrollable exception.
func TestAnOverfullCentredLineStartsWhereItsDirectionStarts(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want float64
	}{{"", 0}, {` dir="rtl"`, -60}} {
		root := layoutOf(t, 600, `<div id="p"`+tc.dir+`>abcdefghij</div>`,
			`#p { font-family: Courier; font-size: 20px; width: 60px;
			      white-space: nowrap; text-align: center }`)
		if got := lineX(t, root, "p"); got != tc.want {
			t.Errorf("an overfull centred line in a%q block is at %gpx, want %g",
				tc.dir, got, tc.want)
		}
	}
}
