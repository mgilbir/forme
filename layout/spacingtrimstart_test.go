package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// §8.2's text-spacing-trim at the start of a line. A full-width opening bracket
// is the glyph plus half an em of blank in front of it, and trim-start,
// trim-both and space-first set it half-width where it begins a line — the
// blank taken away and the ink moved back into it, which is what the face's
// 'halt' states and what the run is drawn with.
//
// NotoSansJP at 20px: "国" and "（" are 20px each, and "（" is 10px trimmed.

// trimStartLines lays out a paragraph in the CJK face and returns its lines as
// runs, so a test can read both the text and how each run is set.
func trimStartLines(t *testing.T, text, css string) [][]TextRun {
	t.Helper()
	root := layoutWithFonts(t, cjkFace(t), `<div id="p" lang="ja">`+text+`</div>`,
		`body { margin: 0 } #p { font-size: 20px; `+css+` }`)
	var out [][]TextRun
	for _, line := range find(t, root, "p").Lines {
		out = append(out, line.Runs)
	}
	return out
}

func trimRunsText(runs []TextRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// halted reports whether a run is drawn in the half-width form.
func halted(r TextRun) bool { return hasFeatureTag(r.Features.Tags, "halt") }

// TestTrimStartTrimsTheBracketThatBeginsEachLine is the suite's
// text-spacing-trim-start-002: a box 3.6em wide holds "（国国国" — four
// characters — only if the bracket gives up its half, and it does on the first
// line, on the line a soft wrap began, and on the line after a forced break.
func TestTrimStartTrimsTheBracketThatBeginsEachLine(t *testing.T) {
	const text = "（国国国（国国国<br>（国国国"
	lines := trimStartLines(t, text, "width: 72px; text-spacing-trim: trim-start")
	if len(lines) != 3 {
		var got []string
		for _, l := range lines {
			got = append(got, trimRunsText(l))
		}
		t.Fatalf("trim-start set %d lines %q, want three of %q", len(lines), got, "（国国国")
	}
	for k, runs := range lines {
		if got := trimRunsText(runs); got != "（国国国" {
			t.Errorf("line %d is %q, want %q", k+1, got, "（国国国")
		}
		first := runs[0]
		if first.Text != "（" || !halted(first) || first.Width != bgpx(10) {
			t.Errorf("line %d begins with %q, %v wide, drawn with %q; want the bracket "+
				"alone, 10px, drawn with 'halt'", k+1, first.Text, first.Width, first.Features.Tags)
		}
		// The ideograph after it is where the half-width bracket ends.
		if len(runs) > 1 && runs[1].X != bgpx(10) {
			t.Errorf("line %d: the text after the bracket starts at %v, want 10px", k+1, runs[1].X)
		}
	}
	// The control: "normal" keeps the blank at the start of every line, so the
	// same box holds only three characters to a line.
	if n := len(trimStartLines(t, text, "width: 72px")); n == 3 {
		t.Errorf("\"normal\" set three lines as well; the box is not narrow enough to " +
			"tell the two apart")
	}
}

// TestSpaceFirstSparesTheFirstLineAndTheLineAfterABreak. space-first is
// trim-start with the first line and every line after a forced break set as
// space-all would set them. The box is 4em: the first line holds its full-width
// bracket and three ideographs exactly.
func TestSpaceFirstSparesTheFirstLineAndTheLineAfterABreak(t *testing.T) {
	lines := trimStartLines(t, "（国国国（国国国<br>（国",
		"width: 80px; text-spacing-trim: space-first")
	want := []struct {
		text    string
		trimmed bool
	}{
		{"（国国国", false}, // the first line
		{"（国国国", true},  // a soft wrap began it
		{"（国", false},   // a forced break began it
	}
	if len(lines) != len(want) {
		t.Fatalf("space-first set %d lines, want %d", len(lines), len(want))
	}
	for k, w := range want {
		if got := trimRunsText(lines[k]); got != w.text {
			t.Errorf("line %d is %q, want %q", k+1, got, w.text)
		}
		if got := halted(lines[k][0]); got != w.trimmed {
			t.Errorf("line %d: its bracket trimmed %v, want %v", k+1, got, w.trimmed)
		}
	}
}

// TestABracketInsideALineIsNotTrimmed: only the start of a line. "国（国" on one
// line keeps the bracket's blank.
func TestABracketInsideALineIsNotTrimmed(t *testing.T) {
	lines := trimStartLines(t, "国（国", "text-spacing-trim: trim-start")
	for _, r := range lines[0] {
		if halted(r) {
			t.Errorf("%q in the middle of the line was set half-width", r.Text)
		}
	}
	if w := lines[0][len(lines[0])-1].X; w != bgpx(40) {
		t.Errorf("the last ideograph starts at %v, want 40px", w)
	}
}

// TestWhatIsNotContentDoesNotStopTheTrim is the suite's
// text-spacing-trim-start-oof-001: a float, an absolutely positioned box and an
// empty span in front of the bracket put nothing a reader sees on the line, so
// the bracket still begins it.
func TestWhatIsNotContentDoesNotStopTheTrim(t *testing.T) {
	for _, before := range []string{
		`<div style="float: left"></div>`,
		`<div style="position: absolute"></div>`,
		`<span></span>`,
		`<span>`,
	} {
		lines := trimStartLines(t, before+"（国国", "text-spacing-trim: trim-start")
		if len(lines) == 0 || !halted(lines[0][0]) {
			t.Errorf("after %s the bracket was not trimmed", before)
		}
	}
}

// TestABracketTheFeatureAlreadyHalvedIsNotTrimmedAgain. The suite's references
// draw a trimmed bracket with font-feature-settings: 'halt', and
// text-spacing-trim-start-002-ref declares trim-start as well. The bracket has
// no blank left to give up, and taking another half em off moved every line's
// text over it.
func TestABracketTheFeatureAlreadyHalvedIsNotTrimmedAgain(t *testing.T) {
	lines := trimStartLines(t, `<span style="font-feature-settings: 'halt' 1">（</span>国国`,
		"text-spacing-trim: trim-start")
	if got := lines[0][1].X; got != bgpx(10) {
		t.Errorf("the text after a bracket 'halt' already halved starts at %v, want "+
			"10px: it was trimmed twice", got)
	}
	// And a run that turns the feature off has refused the half-width form.
	lines = trimStartLines(t, `<span style="font-feature-settings: 'halt' 0">（</span>国国`,
		"text-spacing-trim: trim-start")
	if got := lines[0][1].X; got != bgpx(20) || halted(lines[0][0]) {
		t.Errorf("the text after a bracket whose 'halt' is turned off starts at %v, "+
			"want 20px, and it was drawn with %q", got, lines[0][0].Features.Tags)
	}
}

// TestTheTrimIsDrawnWhereTheLineLeftRoomForIt: the display list carries the
// half-width form, so the bracket's ink is drawn half an em left of where its
// full-width glyph would be — which is where the blank was — and the line's
// room and the drawing agree.
func TestTheTrimIsDrawnWhereTheLineLeftRoomForIt(t *testing.T) {
	root := layoutWithFonts(t, cjkFace(t), `<div id="p" lang="ja">（国</div>`,
		`body { margin: 0 } #p { font-size: 20px; text-spacing-trim: trim-start }`)
	var bracket *DrawText
	for _, op := range Paint(root) {
		if d, ok := op.(DrawText); ok && d.Text == "（" {
			bracket = &d
		}
	}
	if bracket == nil {
		t.Fatal("no run draws the bracket")
	}
	glyphs, _ := ShapedGlyphs(*bracket)
	if len(glyphs) != 1 {
		t.Fatalf("the bracket shaped to %d glyphs", len(glyphs))
	}
	g := glyphs[0]
	if g.XOffset != -500 || g.XAdvance != 500 {
		t.Errorf("the bracket is drawn offset %v and advancing %v per 1000, want -500 "+
			"and 500: the face's half-width form", g.XOffset, g.XAdvance)
	}
}

// TestAShrinkToFitBoxIsAsWideAsItsTrimmedLine: a float around "（国国国" under
// trim-start is 70px, the width of its one line, and a min-content box is as
// wide as its widest unbreakable run with the bracket that begins it trimmed.
func TestAShrinkToFitBoxIsAsWideAsItsTrimmedLine(t *testing.T) {
	for _, tc := range []struct {
		what, text, css string
		want            float64
	}{
		{"a float, trim-start", "（国国国", "float: left; text-spacing-trim: trim-start", 70},
		{"a float, normal", "（国国国", "float: left", 80},
		// space-first spares the one line a float's maximum has.
		{"a float, space-first", "（国国国", "float: left; text-spacing-trim: space-first", 80},
		// The runs are "国" and "（国": the second begins a line at the
		// minimum, which is a soft wrap's for both values.
		{"min-content, trim-start", "国（国", "width: min-content; text-spacing-trim: trim-start", 30},
		{"min-content, space-first", "国（国", "width: min-content; text-spacing-trim: space-first", 30},
		{"min-content, normal", "国（国", "width: min-content", 40},
	} {
		root := layoutWithFonts(t, cjkFace(t), `<div id="p" lang="ja">`+tc.text+`</div>`,
			`body { margin: 0 } #p { font-size: 20px; `+tc.css+` }`)
		if got := find(t, root, "p").BorderRect.W; got != bgpx(tc.want) {
			t.Errorf("%s: the box is %v wide, want %vpx", tc.what, got, tc.want)
		}
	}
}

// TestAFirstLineAtAnotherSizeTrimsItsOwnHalf: ::first-line sets the first line
// at twice the size, and its bracket gives up half of *its* em — 20px, not the
// 10px the paragraph's own size would give.
func TestAFirstLineAtAnotherSizeTrimsItsOwnHalf(t *testing.T) {
	lines := trimStartLines(t, "（国",
		"text-spacing-trim: trim-start } #p::first-line { font-size: 40px")
	if got := lines[0][1].X; got != bgpx(20) {
		t.Errorf("the ideograph after a 40px bracket trimmed starts at %v, want 20px", got)
	}
}

// TestAnUprightBracketIsReportedNotTrimmed: in upright vertical text the
// half-width form is the one 'vhal' states, on the other axis, and it is named
// rather than guessed at. Sideways text is the horizontal line turned, so its
// half-width form is the one the face's 'halt' states, and it is trimmed.
func TestAnUprightBracketIsReportedNotTrimmed(t *testing.T) {
	for _, tc := range []struct {
		orientation string
		trimmed     bool
	}{
		{"upright", false},
		{"sideways", true},
	} {
		frag, findings := layoutWith(t, cjkFace(t), `<div id="p" lang="ja">（国</div>`,
			`body { margin: 0 } #p { font-size: 20px; writing-mode: vertical-rl; `+
				`text-orientation: `+tc.orientation+`; height: 200px; `+
				`text-spacing-trim: trim-start }`)
		lines := find(t, frag, "p").Lines
		if len(lines) == 0 || len(lines[0].Runs) == 0 {
			t.Fatalf("%s: no line", tc.orientation)
		}
		if got := halted(lines[0].Runs[0]); got != tc.trimmed {
			t.Errorf("%s: the bracket was trimmed %v, want %v", tc.orientation, got, tc.trimmed)
		}
		said := false
		for _, f := range findings {
			if f.Property == "text-spacing-trim" && strings.Contains(f.Message, "upright") {
				said = true
			}
		}
		if said == tc.trimmed {
			t.Errorf("%s: reported %v, want %v", tc.orientation, said, !tc.trimmed)
		}
	}
}

// TestTheLineStartValuesAreNotReported: trim-start and space-first are done, and
// a document that declares them is not told otherwise.
func TestTheLineStartValuesAreNotReported(t *testing.T) {
	for _, v := range []string{"trim-start", "space-first"} {
		_, findings := layoutWith(t, cjkFace(t), `<div id="p" lang="ja">（国国</div>`,
			`#p { font-size: 20px; text-spacing-trim: `+v+` }`)
		for _, f := range findings {
			if f.Property == "text-spacing-trim" {
				t.Errorf("%s was reported: %s", v, f.Message)
			}
		}
	}
}

// TestTheHalfWidthFormIsAskedForInTheSettledForm: the tags a run is shaped
// with are sorted and name each feature once, because the list is part of the
// key a shaped group is memoised under (see shape.Features.Tags). 'halt' joins
// a run's own list in its place and is not named twice.
func TestTheHalfWidthFormIsAskedForInTheSettledForm(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "halt"},
		{"kern,liga", "halt,kern,liga"},
		{"aalt,zero", "aalt,halt,zero"},
		{"halt", "halt"},
		{"aalt,halt,zero", "aalt,halt,zero"},
	} {
		got := startTrimmedFeatures(shape.Features{Tags: tc.in}).Tags
		if got != tc.want {
			t.Errorf("%q with 'halt' asked for is %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTheBracketsOwnValueDecides: the property inherits, and the value that
// decides is the one on the element the bracket is in (audit C145, which the
// end's trim was fixed for). A span asking for trim-start in a "normal"
// paragraph trims its bracket, and a span asking for "normal" in a trim-start
// one does not.
func TestTheBracketsOwnValueDecides(t *testing.T) {
	for _, tc := range []struct {
		block, span string
		want        bool
	}{
		{"normal", "trim-start", true},
		{"trim-start", "normal", false},
		{"normal", "space-first", false}, // the first line, which space-first spares
	} {
		lines := trimStartLines(t,
			`<span style="text-spacing-trim: `+tc.span+`">（国</span>`,
			"text-spacing-trim: "+tc.block)
		if got := halted(lines[0][0]); got != tc.want {
			t.Errorf("a %s span in a %s block: trimmed %v, want %v", tc.span, tc.block,
				got, tc.want)
		}
	}
}
