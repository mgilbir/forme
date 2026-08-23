package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Justification: CSS 2.1 §16.2 and the expansion rules of CSS Text 3 §7.
//
// Courier throughout, because it is one of the fourteen and its every glyph is
// 600/1000 of the em — so at 20px a character is 12 wide, a line's natural width
// is a count of characters, and an assertion can name the number rather than
// whatever the face happened to measure.

// justified lays a document out in Courier at a stated width.
func justified(t *testing.T, htmlSrc, extra string) *Fragment {
	t.Helper()
	css := `#p { font-family: Courier; font-size: 20px; width: 240px; ` + extra + ` }`
	return layoutOf(t, 600, htmlSrc, css)
}

// lineEnd is where the content of a line stops: the right edge of its last run.
func lineEnd(line LineFragment) style.Unit {
	var end style.Unit
	for _, r := range line.Runs {
		if e := r.X.Add(r.Width); e > end {
			end = e
		}
	}
	return end
}

// TestJustifyFillsEveryLineButTheLast is the whole point of the property: the
// right edge lines up.
func TestJustifyFillsEveryLineButTheLast(t *testing.T) {
	root := justified(t,
		`<div id="p">one two three four five six seven eight nine ten eleven twelve</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	if len(f.Lines) < 3 {
		t.Fatalf("%d lines; the fixture needs at least three to have a middle", len(f.Lines))
	}
	width := f.ContentRect().W
	for i, line := range f.Lines[:len(f.Lines)-1] {
		if got := lineEnd(line); got != width {
			t.Errorf("line %d ends at %v and the block is %v wide; a justified "+
				"line reaches the far margin", i, got, width)
		}
	}
}

// TestTheLastLineIsNotJustified. §16.2 aligns it as "start" instead — a last
// line stretched to the margin is one word and a line's worth of white to spread
// over, which is the river a justified paragraph exists to avoid.
func TestTheLastLineIsNotJustified(t *testing.T) {
	root := justified(t,
		`<div id="p">one two three four five six seven eight nine ten eleven twelve</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	last := f.Lines[len(f.Lines)-1]
	if got, width := lineEnd(last), f.ContentRect().W; got == width {
		t.Errorf("the last line reaches the far margin at %v; it is aligned as "+
			"start, not justified", got)
	}
}

// TestALineEndedByABreakIsNotJustified: a line the author ended is short because
// they said so, not because it filled, so it is a last line for this purpose.
func TestALineEndedByABreakIsNotJustified(t *testing.T) {
	root := justified(t,
		`<div id="p">one two<br>three four five six seven eight nine ten</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines, want the break to have made at least two", len(f.Lines))
	}
	if got, width := lineEnd(f.Lines[0]), f.ContentRect().W; got == width {
		t.Errorf("the line before the <br> was stretched to %v; the author ended "+
			"it, so it is not a line that failed to fill", got)
	}
}

// TestJustifyStretchesTheSpacesAndNotTheWords.
//
// The slack goes between the words. A run of letters keeps the width its face
// gives it, so "three" is five Courier characters wide wherever it lands, and an
// implementation that spread the slack across every character instead would
// widen it.
func TestJustifyStretchesTheSpacesAndNotTheWords(t *testing.T) {
	root := justified(t,
		`<div id="p">one two three four five six seven eight nine ten eleven twelve</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	// 20px Courier: 600/1000 of the em, so 12px a character.
	per, _ := style.FromPx(12)
	for _, line := range f.Lines[:len(f.Lines)-1] {
		for _, r := range line.Runs {
			if justifiableSpace(r.Text) {
				continue
			}
			want := per.Mul(float64(len([]rune(r.Text))))
			if r.Width != want {
				t.Errorf("the word %q is %v wide and its face makes it %v; the "+
					"slack went into the letters", r.Text, r.Width, want)
			}
		}
	}
}

// TestJustifySpreadsTheSlackEvenly: every gap on a line gets the same share, to
// within the unit that cannot be divided further. A line whose first gap took
// everything would be one word pushed against the left margin.
func TestJustifySpreadsTheSlackEvenly(t *testing.T) {
	root := justified(t,
		`<div id="p">one two three four five six seven eight nine ten eleven twelve</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	per, _ := style.FromPx(12) // an unstretched Courier space at 20px

	for i, line := range f.Lines[:len(f.Lines)-1] {
		var gaps []style.Unit
		for _, r := range line.Runs {
			if justifiableSpace(r.Text) && r.X.Add(r.Width) < lineEnd(line) {
				gaps = append(gaps, r.Width)
			}
		}
		if len(gaps) < 2 {
			continue
		}
		for _, g := range gaps {
			if g < per {
				t.Errorf("line %d has a gap of %v, narrower than the %v space it "+
					"started as", i, g, per)
			}
			// Every gap within one unit of the first: the remainder is spread
			// one unit at a time and cannot make a gap differ by more.
			if d := g.Sub(gaps[0]); d > 1 || d < -1 {
				t.Errorf("line %d has gaps of %v and %v; the slack was not spread "+
					"evenly", i, gaps[0], g)
			}
		}
	}
}

// TestJustifyLeavesALineWithNoSpaceAlone: nowhere to put the slack, so the line
// stays where "start" put it — which is what CSS Text 3 §7.3 says to do with a
// line that has no expansion opportunity.
func TestJustifyLeavesALineWithNoSpaceAlone(t *testing.T) {
	root := justified(t,
		`<div id="p">aaaaaaaaaaaaaaaaaaaaaaaa bb</div>`,
		`text-align: justify; overflow-wrap: break-word`)
	f := find(t, root, "p")
	for i, line := range f.Lines {
		for _, r := range line.Runs {
			if !justifiableSpace(r.Text) {
				continue
			}
			per, _ := style.FromPx(12)
			if r.Width > per {
				t.Errorf("line %d stretched a space to %v", i, r.Width)
			}
		}
	}
}

// TestJustifyDoesNotStretchTheHangingSpace.
//
// §4.1.2 hangs a line's trailing white space past the end of the line, and
// §7.3's expansion opportunities are *between* words. A trailing space is
// neither between two words nor on the page, so stretching it would push the
// line's last word back from the margin to make room for white space no reader
// can see — the one thing justification is there to prevent.
//
// It takes "white-space: pre-wrap" to see: under a collapsing value the trailing
// space is not a run at all, so the guard has nothing to guard and the fault
// cannot show.
func TestJustifyDoesNotStretchTheHangingSpace(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="p">one two   three four five six seven</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 220px;
		      white-space: pre-wrap; text-align: justify }`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines; the fixture needs a line that is not the last", len(f.Lines))
	}
	line := f.Lines[0]
	last := line.Runs[len(line.Runs)-1]
	if !justifiableSpace(last.Text) {
		t.Fatalf("the line ends with %q, not the hanging space the fixture is about",
			last.Text)
	}
	per, _ := style.FromPx(12)
	if last.Width != per {
		t.Errorf("the hanging space is %v wide and its face makes it %v; the slack "+
			"went past the end of the line", last.Width, per)
	}
	// And the content still reaches the margin, so the line really was justified
	// and this is not passing because nothing happened.
	var content style.Unit
	for _, r := range line.Runs[:len(line.Runs)-1] {
		if e := r.X.Add(r.Width); e > content {
			content = e
		}
	}
	if content != f.ContentRect().W {
		t.Errorf("the line's content ends at %v and the block is %v wide", content,
			f.ContentRect().W)
	}
}

// TestOnlyJustifyStretchesAnything is the containment argument: every other
// alignment must be untouched by this, so a left-aligned paragraph of the same
// text keeps its spaces exactly as its face set them.
func TestOnlyJustifyStretchesAnything(t *testing.T) {
	const doc = `<div id="p">one two three four five six seven eight nine ten eleven twelve</div>`
	per, _ := style.FromPx(12)
	for _, align := range []string{"left", "right", "center", "start"} {
		root := justified(t, doc, "text-align: "+align)
		f := find(t, root, "p")
		for i, line := range f.Lines {
			for _, r := range line.Runs {
				if justifiableSpace(r.Text) && r.Width != per {
					t.Errorf("text-align: %s, line %d: a space is %v wide and its "+
						"face makes it %v", align, i, r.Width, per)
				}
			}
		}
	}
}

// What justification moves, and what it used to leave behind.
//
// A line carries more than the text drawn on it: the atomic inlines, and the
// margin, border and padding an inline box contributes at each of its edges.
// All of them move when a space between them grows. Justification used to run
// over the *drawn runs* after everything had been placed, so the text moved and
// the ink of the boxes it was inside did not — a <span> around a justified line
// was painted a space short of its own last word, with the background stopping
// where the unjustified line had ended.
//
// It now runs over the line's items before anything is placed, which is the one
// place where moving something moves everything derived from it.

// TestAnInlineBoxCoversItsJustifiedContent is the bug, in the shape the suite
// writes it: text-justify-and-trailing-spaces-001, a span with a background
// holding a line that is justified and a trailing space that hangs.
func TestAnInlineBoxCoversItsJustifiedContent(t *testing.T) {
	// 240px is twenty characters. "aa aa" is five, so the one space grows by
	// fifteen and the line reaches the margin.
	ops := paintOf(t, `<div id="d"><span id="s">aa aa</span> bb</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 240px;
		      text-align: justify }
		 #s { background: rgb(0,128,0) }`)
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds, want 1: %v", len(got), got)
	}
	// The span holds "aa aa", which after justification runs from the line's
	// start to its last letter.
	f := find(t, layoutOf(t, 600, `<div id="d"><span id="s">aa aa</span> bb</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 240px; text-align: justify }`), "d")
	if len(f.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(f.Lines))
	}
	// Where the span's own last letter ends: the run before the final " bb".
	var want style.Unit
	for _, r := range f.Lines[0].Runs {
		if r.Text == "aa" {
			if e := r.X.Add(r.Width); e > want {
				want = e
			}
		}
	}
	if got[0].W != want {
		t.Errorf("the span's background is %v wide and its text ends at %v; the "+
			"background was left where the unjustified line put it",
			got[0].W, want)
	}
}

// TestAnInlineBoxEndingInAStretchedSpaceCoversIt is the same rule where the
// stretched space is the *last* thing in the box rather than in the middle of
// it. Only then does the space's own width reach the box's right edge, and a
// reconstruction that used the width the font gave rather than the width the
// line gave stops a gap short — which is invisible in every fixture where the
// box ends on a letter.
func TestAnInlineBoxEndingInAStretchedSpaceCoversIt(t *testing.T) {
	// Twenty characters of room. The first line takes "aa bb cc" and the long
	// word goes to the second, so the first is justified rather than being the
	// last line — which is the only kind this stretches.
	const src = `<div id="d"><span id="s">aa </span>bb cc dddddddddddddddddddd</div>`
	css := `#d { font-family: Courier; font-size: 20px; width: 240px;
	             text-align: justify }
	        #s { background: rgb(0,128,0) }`
	got := fillsOf(paintOf(t, src, css), green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds, want 1: %v", len(got), got)
	}
	// The span holds "aa " and the space in it is a gap the line stretched, so
	// the background has to reach where the next word begins.
	f := find(t, layoutOf(t, 600, src, `#d { font-family: Courier; font-size: 20px;
	        width: 240px; text-align: justify }`), "d")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines, want 2: %q", len(f.Lines), lineTexts(f.Lines))
	}
	var next style.Unit
	for _, r := range f.Lines[0].Runs {
		if r.Text == "bb" {
			next = r.X
		}
	}
	if next == 0 {
		t.Fatal("the fixture has no second word to measure against")
	}
	// The span begins the line, so its background's width is where the next
	// word begins. The comparison is of widths and not of positions because the
	// fill carries the page's own offset and the run does not.
	if got[0].W != next {
		t.Errorf("the span's background is %v wide and the next word begins %v "+
			"into the line; the stretched space is inside the span and its ink "+
			"stops short of it", got[0].W, next)
	}
}

// TestEveryGapOnALineIsStretched. The opportunities are decided before any of
// them moves, and this is why: the test reads a position, the loop changes
// positions as it goes, and asking again part-way through compares a space that
// has already been pushed along against a content edge that has not. Every gap
// after the first then looks like a hanging space, and a line with two of them
// came out with one stretched and one not.
func TestEveryGapOnALineIsStretched(t *testing.T) {
	root := justified(t, `<div id="p">a b c ddddddddddddddddd</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines; the fixture needs a line that is not the last", len(f.Lines))
	}
	line := f.Lines[0]
	per, _ := style.FromPx(12)
	var gaps []style.Unit
	for _, r := range line.Runs {
		if justifiableSpace(r.Text) {
			gaps = append(gaps, r.Width)
		}
	}
	if len(gaps) < 2 {
		t.Fatalf("%d gaps on the first line, want at least 2: %q", len(gaps),
			lineTexts(f.Lines))
	}
	for i, w := range gaps {
		if w <= per {
			t.Errorf("gap %d is %v wide and the face makes a space %v; every gap "+
				"on a justified line takes a share", i, w, per)
		}
	}
	// And they are within one unit of each other, which is what "evenly" means.
	for i := 1; i < len(gaps); i++ {
		if d := gaps[i].Sub(gaps[0]); d > 1 || d < -1 {
			t.Errorf("the gaps are %v; the slack is spread evenly", gaps)
			break
		}
	}
}

// TestARightToLeftLineDoesNotStretchItsHang.
//
// Rule L1 gives a line's trailing spaces the paragraph's own level, so on a
// right-to-left line they are drawn at its *left* edge — before everything
// else. Justification asked "is this space past where the content ends", got
// "no, it is at the very beginning", and stretched the hang: the line came out
// a space short of its own margin with the gap between its words too narrow by
// the same amount.
//
// So which items hang is decided by a walk from the logical end of the line,
// and the alignment and the justification share it rather than each having a
// rule of its own.
func TestARightToLeftLineDoesNotStretchItsHang(t *testing.T) {
	// The suite's own fixture, in Courier: seven characters of room, pre-wrap,
	// and "XX XX XXX" — which breaks after the space, leaving "XX XX " on a
	// line whose trailing space hangs. The direction is the block's; the script
	// is beside the point, and Latin keeps the widths exact.
	const src = `<div id="p">XX XX XXX</div>`
	css := `#p { font-family: Courier; font-size: 20px; width: 84px;
	             direction: rtl; white-space: pre-wrap; text-align: justify }`
	f := find(t, layoutOf(t, 600, src, css), "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines, want 2: %q", len(f.Lines), lineTexts(f.Lines))
	}
	line := f.Lines[0]
	per, _ := style.FromPx(12)

	// The one gap between the words takes the whole slack. The line holds
	// "XX XX " in seven characters of room: five of content, one space hanging,
	// and two characters of slack for the single gap between the words — so it
	// ends up three characters wide.
	var gaps []style.Unit
	for _, r := range line.Runs {
		if justifiableSpace(r.Text) {
			gaps = append(gaps, r.Width)
		}
	}
	if len(gaps) != 2 {
		t.Fatalf("%d spaces on the line, want 2 — one between the words and one "+
			"hanging: %q", len(gaps), lineTexts(f.Lines))
	}
	stretched, hanging := 0, 0
	for _, w := range gaps {
		if w > per {
			stretched++
		} else if w == per {
			hanging++
		}
	}
	if stretched != 1 || hanging != 1 {
		t.Errorf("the spaces are %v and the face makes one %v; the gap between the "+
			"words takes the slack and the hanging space keeps its own width",
			gaps, per)
	}

	// And the content reaches the block's width, which is what says the slack
	// went into the line rather than past its end.
	var right style.Unit
	for _, r := range line.Runs {
		if justifiableSpace(r.Text) && r.Width == per {
			// The hang, which is outside the content.
			continue
		}
		if e := r.X.Add(r.Width); e > right {
			right = e
		}
	}
	if right != f.ContentRect().W {
		t.Errorf("the line's content ends at %v and the block is %v wide; a "+
			"right-to-left justified line reaches its margin too",
			right, f.ContentRect().W)
	}
}

// Justification and preserved tabs, CSS Text 4's rule for text-align: "if an
// element's white space is not collapsible ... the UA must ensure that tab stops
// continue to line up as required by the white space processing rules".
//
// A tab's advance is the distance from where the line has got to to the next tab
// stop. Widening a space in front of one therefore buys nothing: the tab shrinks
// by as much and the text after it does not move — until the pen crosses a stop,
// and then the tab jumps a whole stop and every column after it on the line goes
// with it. Neither is justification, so the slack goes after the last tab and
// nowhere else.

// tabbed lays out one preserved, tab-bearing line at a stated alignment.
//
// Courier at 20px is 12px a character and tab-size: 8 puts the stops at 96px, so
// every number below is a count of characters.
func tabbed(t *testing.T, body, align string) LineFragment {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p">`+body+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 240px;
		      white-space: pre-wrap; tab-size: 8; text-align: `+align+` }`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%q made %d lines; the fixture needs a line that is not the last, "+
			"because the last line is never justified", body, len(f.Lines))
	}
	return f.Lines[0]
}

// TestJustifySpendsNoSlackBeforeATab is the rule, and the fixture has spaces on
// both sides of one tab so that the same line answers for both halves.
func TestJustifySpendsNoSlackBeforeATab(t *testing.T) {
	const body = "a b\tc d e f g h i j"
	line := tabbed(t, body, "justify")
	natural, _ := style.FromPx(12)

	seenTab, before, after := false, 0, 0
	for _, r := range line.Runs {
		if r.Text == "\t" {
			seenTab = true
			continue
		}
		if !justifiableSpace(r.Text) {
			continue
		}
		if !seenTab {
			before++
			if r.Width != natural {
				t.Errorf("a space before the tab is %v wide and its face makes it %v; "+
					"slack spent there does not reach the margin, it is taken back by "+
					"the tab", r.Width, natural)
			}
			continue
		}
		// The hanging space at the end is not an opportunity either, and
		// TestJustifyDoesNotStretchTheHangingSpace is what holds that.
		if r.X.Add(r.Width) > line.Rect.W {
			continue
		}
		after++
		if r.Width <= natural {
			t.Errorf("a space after the tab is %v wide; the slack belongs to the "+
				"spaces the tab does not swallow", r.Width)
		}
	}
	if before == 0 || after == 0 {
		t.Fatalf("the fixture found %d spaces before the tab and %d after it; it is "+
			"meant to have both", before, after)
	}
	// And the line really was justified, so none of this passes because nothing
	// happened: its last word is further right than the plain line's is.
	//
	// Asked this way rather than against the margin, because the margin is not
	// quite where the last word ends. The slack divides as evenly as the unit
	// allows and the remainder is spread a unit at a time over the leading gaps,
	// so the last word lands a fraction of a unit past the edge rather than on
	// it — see justifyItems, which does that on purpose so that a paragraph's
	// lines do not each lose a fraction and drift.
	plain := tabbed(t, body, "left")
	lastText := func(l LineFragment) style.Unit {
		var x style.Unit
		for _, r := range l.Runs {
			if !justifiableSpace(r.Text) && r.Text != "\t" {
				x = r.X
			}
		}
		return x
	}
	if got, want := lastText(line), lastText(plain); got <= want {
		t.Errorf("the last word of the justified line begins at %v and of the plain "+
			"line at %v; the line was not stretched at all", got, want)
	}
}

// TestALineEndingAtATabIsSetAsThoughItWereNotJustified is the other end of the
// same rule, and it is the suite's text-align-justify-tabs-001: every space on
// the line is in front of a tab, so there is no opportunity left anywhere and
// the line comes out exactly as an unjustified one does.
//
// Its reference is that unjustified line, written as the same markup without the
// declaration — which is what this compares against rather than a table of
// numbers, for the same reason the suite does it that way.
func TestALineEndingAtATabIsSetAsThoughItWereNotJustified(t *testing.T) {
	const body = "a b c\td e\tf\tghijkl mno"
	got, want := tabbed(t, body, "justify"), tabbed(t, body, "left")
	if len(got.Runs) != len(want.Runs) {
		t.Fatalf("the justified line has %d runs and the plain one %d",
			len(got.Runs), len(want.Runs))
	}
	tabs := 0
	for i, r := range got.Runs {
		if r.Text == "\t" {
			tabs++
		}
		if r.X != want.Runs[i].X || r.Width != want.Runs[i].Width {
			t.Errorf("run %d (%q) is at %v wide %v justified and at %v wide %v plain",
				i, r.Text, r.X, r.Width, want.Runs[i].X, want.Runs[i].Width)
		}
	}
	if tabs < 2 {
		t.Fatalf("the fixture holds %d tabs; it is meant to have several, with the "+
			"last of them at the end of the line", tabs)
	}
	if got.Runs[len(got.Runs)-1].Text != "\t" {
		t.Fatalf("the line ends with %q, not the tab the fixture is about",
			got.Runs[len(got.Runs)-1].Text)
	}
}

// TestALineWithNoTabIsJustifiedAsItAlwaysWas is the containment case. The rule
// added here reads every item on the line looking for a tab, and almost no line
// in almost any document has one — so the answer for all of them must be the one
// they had before.
func TestALineWithNoTabIsJustifiedAsItAlwaysWas(t *testing.T) {
	root := justified(t,
		`<div id="p">one two three four five six seven eight nine ten</div>`,
		`text-align: justify`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines", len(f.Lines))
	}
	natural, _ := style.FromPx(12)
	stretched := 0
	for _, r := range f.Lines[0].Runs {
		if justifiableSpace(r.Text) && r.Width > natural {
			stretched++
		}
	}
	if stretched == 0 {
		t.Errorf("no space on the first line was stretched; a line with no tab on it " +
			"has every one of its spaces to spend the slack on")
	}
	if got, want := lineEnd(f.Lines[0]), f.ContentRect().W; got != want {
		t.Errorf("the line ends at %v and the block is %v wide", got, want)
	}
}
