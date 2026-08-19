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
