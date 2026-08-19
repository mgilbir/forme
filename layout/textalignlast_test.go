package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// text-align-last — CSS Text 3 §7.2 — and the two values of text-justify that
// change whether a line is stretched at all.
//
// The last line of a justified paragraph is the one line that is not justified,
// and the reason is typographic: it has no text to fill with, so stretching it
// spreads three words across the measure with a hand's breadth between them.
// text-align-last is how an author asks for something else, and text-align's
// own "justify-all" is how they ask for the stretching everywhere.
//
// Neither was implemented, so every declaration of either did nothing and said
// nothing — which is the shape of failure the finding vocabulary exists to
// prevent, and cost forty-four of the suite's tests.
//
// Every number below is arithmetic. The text is Courier, whose every glyph is
// 600 units of 1000, so a character at 20px is 12px wide; lineX and alignCSS
// are in textalign_test.go.

// findingsFrom lays a document out and returns everything it reported.
func findingsFrom(t *testing.T, htmlSrc string, cssSrc ...string) []Finding {
	t.Helper()
	in := Input{HTML: htmlSrc}
	for _, c := range cssSrc {
		in.CSS = append(in.CSS, Stylesheet{Source: c})
	}
	built := Build(in)
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	return append(append([]Finding{}, built.Findings...), rec.Findings()...)
}

// lastLineX is where the last line of a paragraph begins.
func lastLineX(t *testing.T, root *Fragment, id string) float64 {
	t.Helper()
	f := find(t, root, id)
	if len(f.Lines) == 0 {
		t.Fatalf("#%s has no lines", id)
	}
	last := f.Lines[len(f.Lines)-1]
	if len(last.Runs) == 0 {
		t.Fatalf("#%s's last line has no runs", id)
	}
	return last.Runs[0].X.Px()
}

// lastLineEnd is where the last line's content stops, which is what says
// whether it was stretched.
func lastLineEnd(t *testing.T, root *Fragment, id string) float64 {
	t.Helper()
	f := find(t, root, id)
	last := f.Lines[len(f.Lines)-1]
	end := 0.0
	for _, r := range last.Runs {
		if x := r.X.Add(r.Width).Px(); x > end {
			end = x
		}
	}
	return end
}

// twoLines is a paragraph that breaks into two, the second of them short.
//
// The width is 300px and a character is 12px, so twenty-five fit and the
// twenty-four a's fill the first line. The last holds "c d" — three characters,
// 36px — and the space in it matters: §7.3 aligns a line with no expansion
// opportunity as start, so a last line of one word cannot be stretched however
// the properties are set, and a fixture without a space in it would show
// nothing.
const twoLines = `<div id="p">aaaaaaaaaaaaaaaaaaaaaaaa c d</div>`

// TestTheLastLineTakesItsOwnAlignment.
func TestTheLastLineTakesItsOwnAlignment(t *testing.T) {
	for _, tc := range []struct {
		value string
		x     float64
	}{
		{"left", 0},
		{"start", 0},
		{"right", 264},
		{"end", 264},
		{"center", 132},
	} {
		root := layoutOf(t, 600, twoLines,
			alignCSS+` #p { text-align: justify; text-align-last: `+tc.value+` }`)
		if got := lastLineX(t, root, "p"); got != tc.x {
			t.Errorf("text-align-last:%s put the last line at %gpx, want %g",
				tc.value, got, tc.x)
		}
		// The lines before it are still justified, which is the whole point of
		// having a separate property for the last one.
		if got := find(t, root, "p").Lines[0].Runs[0].X.Px(); got != 0 {
			t.Errorf("text-align-last:%s moved the first line to %gpx", tc.value, got)
		}
	}
}

// TestTheLastLineOfAJustifiedParagraphIsNotStretched is the default, and the
// reason the property exists.
func TestTheLastLineOfAJustifiedParagraphIsNotStretched(t *testing.T) {
	root := layoutOf(t, 600, twoLines, alignCSS+` #p { text-align: justify }`)
	if got := lastLineX(t, root, "p"); got != 0 {
		t.Errorf("the last line begins at %gpx, want 0 — a justified paragraph's "+
			"last line is placed where the paragraph starts", got)
	}
	if got := lastLineEnd(t, root, "p"); got != 36 {
		t.Errorf("the last line ends at %gpx, want 36 — three characters, unstretched", got)
	}
}

// TestJustifyAllStretchesTheLastLineToo, which is the whole difference between
// the two spellings of the value.
func TestJustifyAllStretchesTheLastLineToo(t *testing.T) {
	root := layoutOf(t, 600, twoLines, alignCSS+` #p { text-align: justify-all }`)
	if got := lastLineEnd(t, root, "p"); got != 300 {
		t.Errorf("the last line ends at %gpx, want 300 — justify-all stretches it "+
			"to the measure", got)
	}
	// And "justify" on its own still does not.
	plain := layoutOf(t, 600, twoLines, alignCSS+` #p { text-align: justify }`)
	if got := lastLineEnd(t, plain, "p"); got == 300 {
		t.Errorf("text-align:justify stretched the last line as well, so the test " +
			"above says nothing about justify-all")
	}
}

// TestTextAlignLastJustifyStretchesIt is the other spelling of the same ask.
func TestTextAlignLastJustifyStretchesIt(t *testing.T) {
	root := layoutOf(t, 600, twoLines,
		alignCSS+` #p { text-align: justify; text-align-last: justify }`)
	if got := lastLineEnd(t, root, "p"); got != 300 {
		t.Errorf("the last line ends at %gpx, want 300", got)
	}
}

// TestALineBeforeAForcedBreakIsALastLine. §7.2 says text-align-last describes
// "the last line of a block or a line right before a forced line break", and
// the second half is not a footnote: a <br> is how an address or a poem is
// written, and every line of one would otherwise be stretched.
func TestALineBeforeAForcedBreakIsALastLine(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="p">aa<br>bb</div>`,
		alignCSS+` #p { text-align: justify; text-align-last: right }`)
	f := find(t, root, "p")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines, want 2", len(f.Lines))
	}
	for i, line := range f.Lines {
		if got := line.Runs[0].X.Px(); got != 276 {
			t.Errorf("line %d begins at %gpx, want 276 — both lines end at a "+
				"forced break or at the block's end", i, got)
		}
	}
}

// TestTextJustifyNoneLeavesTheLineAlone. §7.3's one value that changes whether
// a line is stretched rather than how.
func TestTextJustifyNoneLeavesTheLineAlone(t *testing.T) {
	root := layoutOf(t, 600, twoLines,
		alignCSS+` #p { text-align: justify-all; text-justify: none }`)
	if got := lastLineEnd(t, root, "p"); got != 36 {
		t.Errorf("the last line ends at %gpx, want 36 — text-justify:none turns "+
			"the stretching off", got)
	}
	// And the line before it, which justify-all would also have stretched.
	first := find(t, root, "p").Lines[0]
	if got := first.Runs[0].X.Px(); got != 0 {
		t.Errorf("the first line begins at %gpx, want 0", got)
	}
	// The control: the same document without the property does stretch.
	on := layoutOf(t, 600, twoLines, alignCSS+` #p { text-align: justify-all }`)
	if got := lastLineEnd(t, on, "p"); got != 300 {
		t.Errorf("without text-justify the last line ends at %gpx, want 300 — the "+
			"test above says nothing otherwise", got)
	}
}

// TestTextJustifyAutoAndInterWordStillJustify: the two values this engine
// performs, which must not be turned off by the check that turns "none" off.
func TestTextJustifyAutoAndInterWordStillJustify(t *testing.T) {
	for _, value := range []string{"auto", "inter-word"} {
		root := layoutOf(t, 600, twoLines,
			alignCSS+` #p { text-align: justify-all; text-justify: `+value+` }`)
		if got := lastLineEnd(t, root, "p"); got != 300 {
			t.Errorf("text-justify:%s left the last line ending at %gpx, want 300",
				value, got)
		}
	}
}

// TestAJustificationMethodThisDoesNotPerformIsReported.
//
// inter-character puts the slack between letters as well as words, which is how
// Thai and Chinese are justified. Spreading it between the words instead
// produces a page with the right margins and the wrong text — which looks
// deliberate, and is exactly what a finding is for.
func TestAJustificationMethodThisDoesNotPerformIsReported(t *testing.T) {
	for _, tc := range []struct {
		value  string
		report bool
	}{
		{"inter-character", true},
		{"distribute", true},
		{"auto", false},
		{"inter-word", false},
		{"none", false},
	} {
		found := false
		for _, f := range findingsFrom(t, twoLines,
			alignCSS+` #p { text-align: justify-all; text-justify: `+tc.value+` }`) {
			if f.Property == "text-justify" && strings.Contains(f.Message, tc.value) {
				found = true
			}
		}
		if found != tc.report {
			t.Errorf("text-justify:%s reported=%v, want %v", tc.value, found, tc.report)
		}
	}
	// And a block that is not justified says nothing, whatever the value: the
	// property changes nothing there and a warning would be crying wolf.
	for _, f := range findingsFrom(t, twoLines,
		alignCSS+` #p { text-align: left; text-justify: inter-character }`) {
		if f.Property == "text-justify" {
			t.Errorf("text-justify was reported on a paragraph that is not justified")
		}
	}
}

// lastLineLeft is the left edge of the last line's content, which on a
// right-to-left line is not the first run's.
func lastLineLeft(t *testing.T, root *Fragment, id string) float64 {
	t.Helper()
	f := find(t, root, id)
	last := f.Lines[len(f.Lines)-1]
	if len(last.Runs) == 0 {
		t.Fatalf("#%s's last line has no runs", id)
	}
	left := last.Runs[0].X.Px()
	for _, r := range last.Runs {
		if x := r.X.Px(); x < left {
			left = x
		}
	}
	return left
}

// TestStartAndEndAreResolvedAgainstTheDirection. The two logical values are the
// reason text-align-last has six of them rather than four: "start" is the edge
// the text begins at, which in a right-to-left block is the right one. A
// version that read them as left and right would put every Hebrew paragraph's
// last line against the edge its text runs away from.
func TestStartAndEndAreResolvedAgainstTheDirection(t *testing.T) {
	for _, tc := range []struct {
		value string
		ltr   float64
		rtl   float64
	}{
		{"start", 0, 264},
		{"end", 264, 0},
		// The physical pair is not affected by direction, which is what the
		// pair exists for.
		{"left", 0, 0},
		{"right", 264, 264},
	} {
		for _, dir := range []struct {
			name string
			css  string
			want float64
		}{
			{"ltr", "", tc.ltr},
			{"rtl", "direction: rtl;", tc.rtl},
		} {
			root := layoutOf(t, 600, twoLines,
				alignCSS+` #p { `+dir.css+` text-align: justify; text-align-last: `+tc.value+` }`)
			if got := lastLineLeft(t, root, "p"); got != dir.want {
				t.Errorf("text-align-last:%s in a %s block put the last line at %gpx, want %g",
					tc.value, dir.name, got, dir.want)
			}
		}
	}
}

// TestJustifyAllStretchesEveryLineAndNotOnlyTheLast. The value is one word and
// it says two things; a reading that took only the second would leave a
// paragraph whose last line is stretched and whose others are not, which is the
// opposite of what any typographer has ever wanted.
func TestJustifyAllStretchesEveryLineAndNotOnlyTheLast(t *testing.T) {
	const src = `<div id="p">aaaaaaaa aaaaaaaa aaaaaaaa c d</div>`
	root := layoutOf(t, 600, src, alignCSS+` #p { text-align: justify-all }`)
	f := find(t, root, "p")
	if len(f.Lines) < 2 {
		t.Fatalf("%d lines, want at least 2: %q", len(f.Lines), lineTexts(f.Lines))
	}
	for i, line := range f.Lines {
		end := 0.0
		for _, r := range line.Runs {
			if x := r.X.Add(r.Width).Px(); x > end {
				end = x
			}
		}
		if end != 300 {
			t.Errorf("line %d ends at %gpx, want 300 — justify-all stretches every "+
				"line: %q", i, end, lineTexts(f.Lines))
		}
	}
}

// TestTextJustifyIsInherited, for the same reason and by the same means: it is
// set on a block and has to reach the paragraphs in it.
func TestTextJustifyIsInherited(t *testing.T) {
	root := layoutOf(t, 600, `<div id="outer">`+twoLines+`</div>`,
		alignCSS+` #outer { text-align: justify-all; text-justify: none }`)
	if got := lastLineEnd(t, root, "p"); got != 36 {
		t.Errorf("an inherited text-justify:none left the last line ending at %gpx, "+
			"want 36", got)
	}
}

// TestTextAlignLastIsInherited. It is an inherited property, which is how
// "body { text-align-last: justify }" reaches a paragraph at all — and the same
// reason text-align itself inherits.
func TestTextAlignLastIsInherited(t *testing.T) {
	root := layoutOf(t, 600, `<div id="outer">`+twoLines+`</div>`,
		alignCSS+` #outer { text-align: justify; text-align-last: right }`)
	if got := lastLineX(t, root, "p"); got != 264 {
		t.Errorf("an inherited text-align-last:right put the last line at %gpx, "+
			"want 264", got)
	}
}
