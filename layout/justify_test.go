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
