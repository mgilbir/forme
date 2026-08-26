package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// css-text-5's text-fit, at the "consistent" granularity.
//
// The property scales the size a block's text is *set* in so that its lines fill
// the box, without changing the computed font-size — so every font-relative
// length the document wrote goes on meaning what it meant and only the type is
// drawn bigger.
//
// Ahem is the face to assert against: every glyph is an em square and
// "line-height: normal" comes to exactly one em, so a line of n characters at
// size s is n×s wide and s tall, and the arithmetic is checkable by hand. The
// suite's own text-fit tests are written in it for that reason, and their
// references write the answer out as a plain font-size.

// fitted lays a document out against Ahem and returns the size, width and height
// of each line: one string per line, "size w×h".
func fitted(t *testing.T, cssSrc, htmlSrc string) []string {
	t.Helper()
	set := loadAhem(t)
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}, Fonts: set})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	f := find(t, Layout(built.Root, Size{W: w, H: h}, set, rec), "d")
	var out []string
	for _, ln := range f.Lines {
		size := 0.0
		width := 0.0
		for _, r := range ln.Runs {
			size = r.Size.Px()
			width += r.Width.Px()
		}
		out = append(out, sizeLine(size, width, ln.Rect.H.Px()))
	}
	return out
}

func sizeLine(size, w, h float64) string {
	return fmt.Sprintf("%g %gx%g", size, w, h)
}

const fitCSS = `#d { white-space: pre; font-family: Ahem; line-height: normal }`

// TestGrowConsistentFillsTheWidestLine is the suite's grow-consistent: two lines
// of 10px Ahem in a 120px box, the longer six characters. Six tens fill half the
// box, so the factor is two and the reference writes "font-size: 20px".
func TestGrowConsistentFillsTheWidestLine(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: grow consistent }`,
		"<div id=\"d\">ABCDEF\nGHI</div>")
	want := []string{"20 120x20", "20 60x20"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestShrinkConsistentBringsTheWidestLineIn is the mirror, and the suite's
// shrink-consistent: eight characters of 20px in an 80px box halve to 10px.
func TestShrinkConsistentBringsTheWidestLineIn(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 80px; font-size: 20px; text-fit: shrink consistent }`,
		"<div id=\"d\">ABCDEFGH\nABCD</div>")
	want := []string{"10 80x10", "10 40x10"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestGrowNeverShrinksAndShrinkNeverGrows, which is what makes the two values
// different requests rather than two spellings of one.
func TestGrowNeverShrinksAndShrinkNeverGrows(t *testing.T) {
	// Already too wide for the box: "grow" has nothing to do.
	got := fitted(t, fitCSS+` #d { width: 40px; font-size: 20px; text-fit: grow }`,
		"<div id=\"d\">ABCDEFGH</div>")
	if got[0] != "20 160x20" {
		t.Errorf("a line that overflows came out %q under \"grow\", want it left "+
			"at 20px", got[0])
	}
	// Room to spare: "shrink" has nothing to do.
	got = fitted(t, fitCSS+` #d { width: 400px; font-size: 20px; text-fit: shrink }`,
		"<div id=\"d\">ABCD</div>")
	if got[0] != "20 80x20" {
		t.Errorf("a short line came out %q under \"shrink\", want it left at 20px", got[0])
	}
}

// TestThePercentageBoundsTheFactor. css-text-5 makes it the maximum for grow and
// the minimum for shrink.
func TestThePercentageBoundsTheFactor(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: grow consistent 150% }`,
		"<div id=\"d\">ABCDEF</div>")
	if got[0] != "15 90x15" {
		t.Errorf("the line came out %q; the factor the box asks for is two and the "+
			"declaration caps it at one and a half", got[0])
	}
	got = fitted(t, fitCSS+` #d { width: 40px; font-size: 20px; text-fit: shrink consistent 75% }`,
		"<div id=\"d\">ABCDEFGH</div>")
	if got[0] != "15 120x15" {
		t.Errorf("the line came out %q; the factor the box asks for is a quarter "+
			"and the declaration floors it at three quarters", got[0])
	}
}

// TestOneFactorForTheWholeBlock is what "consistent" means: the widest line
// decides, and every line is scaled by the same number rather than each filling
// the box on its own.
func TestOneFactorForTheWholeBlock(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: grow consistent }`,
		"<div id=\"d\">ABCDEF\nGH\nI</div>")
	want := []string{"20 120x20", "20 40x20", "20 20x20"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q — the second and third are not each "+
			"stretched to the box", got, want)
	}
}

// TestADeclaredLineHeightDoesNotScale. css-text-5: the property "does not affect
// the font-size computed value, and thus does not affect font-size-relative
// length values of other properties. For example, 'line-height: 1.5em' ... [is]
// not affected."
func TestADeclaredLineHeightDoesNotScale(t *testing.T) {
	got := fitted(t, `#d { white-space: pre; font-family: Ahem; width: 120px;
		font-size: 10px; line-height: 20px; text-fit: grow consistent }`,
		"<div id=\"d\">ABCDEF\nGHI</div>")
	want := []string{"20 120x20", "20 60x20"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q — the type doubled and the declared "+
			"line-height did not", got, want)
	}
	// The two font-relative spellings, which are the ones the sentence is about:
	// an em and a bare number are both resolved against the *computed*
	// font-size, and text-fit does not change that.
	for _, tc := range []struct{ height, want string }{
		{"1.5em", "20 120x15"},
		{"2", "20 120x20"},
		{"0.5em", "20 120x5"},
	} {
		got := fitted(t, `#d { white-space: pre; font-family: Ahem; width: 120px;
			font-size: 10px; text-fit: grow consistent; line-height: `+tc.height+` }`,
			"<div id=\"d\">ABCDEF</div>")
		if got[0] != tc.want {
			t.Errorf("with \"line-height: %s\" the line is %q, want %q — the "+
				"height is of the ten pixels the document computed, not of the "+
				"twenty the type is set in", tc.height, got[0], tc.want)
		}
	}
}

// TestNoTextFitChangesNothing is the containment argument: the property is at
// "none" in every document that does not write it, and the initial value must
// leave the page exactly as it was.
func TestNoTextFitChangesNothing(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 120px; font-size: 10px }`,
		"<div id=\"d\">ABCDEF\nGHI</div>")
	want := []string{"10 60x10", "10 30x10"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
	got = fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: none }`,
		"<div id=\"d\">ABCDEF\nGHI</div>")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("with \"text-fit: none\" the lines are %q, want %q", got, want)
	}
}

// TestPerLineScalesEachLineOnItsOwn, and leaves the two css-text-5 excepts:
// "the last line of the block and lines that end in a forced break are not
// scaled". It is the same exception §16.2 makes for justification and for the
// same reason — a line the author ended is short because they said so.
func TestPerLineScalesEachLineOnItsOwn(t *testing.T) {
	// A soft wrap: "ABCD" is 80 of the 120 available and grows by half; the
	// second line is the block's last and is left alone.
	got := fitted(t, `#d { font-family: Ahem; line-height: normal; width: 120px;
		font-size: 20px; text-fit: grow per-line }`,
		`<div id="d">ABCD EFGHIJ</div>`)
	want := []string{"30 120x30", "20 120x20"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
	// Forced breaks: under "per-line" neither line is scaled, because the first
	// ends at one and the second is the last.
	got = fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: grow per-line }`,
		"<div id=\"d\">GHIJ\nKLM</div>")
	want = []string{"10 40x10", "10 30x10"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("with forced breaks the lines are %q, want %q — \"per-line\" "+
			"leaves both alone", got, want)
	}
}

// TestPerLineAllScalesThemToo, which is the whole of the difference between the
// two granularities. This is the suite's grow-per-line-all, whose reference
// writes each line's answer out as a span with a font-size on it.
func TestPerLineAllScalesThemToo(t *testing.T) {
	got := fitted(t, fitCSS+` #d { width: 120px; font-size: 10px; text-fit: grow per-line-all }`,
		"<div id=\"d\">GHIJ\nKLM</div>")
	want := []string{"30 120x30", "40 120x40"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestAnInlineBoxsOwnInkScalesWithTheLine. §10.6.1 gives an inline box a content
// area the height of its font, and text-fit is the size that font is being used
// at — so the border round two letters on a line scaled by two is twice as tall.
// The suite's grow-per-line-all-line-height is a one-pixel lime border and
// nothing else.
func TestAnInlineBoxsOwnInkScalesWithTheLine(t *testing.T) {
	height := func(fit string) float64 {
		set := loadAhem(t)
		built := Build(Input{
			HTML: `<div id="d">AB<span id="s">CD</span>E</div>`,
			CSS: []Stylesheet{{Source: `#d { white-space: pre; font-family: Ahem;
				font-size: 10px; line-height: normal; width: 100px; ` + fit + ` }
				#s { background: lime }`}},
			Fonts: set,
		})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		root := Layout(built.Root, Size{W: w, H: h}, set, rec)
		for _, ln := range find(t, root, "d").Lines {
			for _, bx := range ln.Boxes {
				if id, _ := bx.Box.Element.Attr("id"); id == "s" {
					return bx.BorderRect.H.Px()
				}
			}
		}
		t.Fatalf("no fragment for the span")
		return 0
	}
	// Five characters of 10px Ahem in a 100px box grow by two, and Ahem's
	// content area is exactly one em.
	if got := height("text-fit: grow per-line-all"); got != 20 {
		t.Errorf("the span's border box is %gpx tall, want 20 — the type on its "+
			"line is set at twice the declared size", got)
	}
	if got := height(""); got != 10 {
		t.Errorf("with no text-fit the span's border box is %gpx tall, want 10", got)
	}
}

// TestAGranularityThisEngineDoesIsNotReported, which is the other half: a
// document that writes the implemented form must come out clean, or the ratchet
// counts every one of them as vacuous.
func TestAGranularityThisEngineDoesIsNotReported(t *testing.T) {
	for _, decl := range []string{
		"none", "grow", "shrink", "grow consistent", "shrink consistent",
		"grow consistent 150%", "consistent grow",
		"grow per-line", "grow per-line-all", "shrink per-line-all 75%",
	} {
		rec := NewRecorder(nil)
		built := Build(Input{
			HTML: `<div id="d">ABCDEF</div>`,
			CSS:  []Stylesheet{{Source: `#d { width: 120px; text-fit: ` + decl + ` }`}},
		})
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		Layout(built.Root, Size{W: w, H: h}, nil, rec)
		for _, f := range rec.Findings() {
			if f.Property == "text-fit" {
				t.Errorf("%q was reported: %q", decl, f.Message)
			}
		}
	}
}

// TestTextFitParses is the grammar, read directly: the keywords in any order,
// the percentage anywhere, and a word that is not in it refused whole.
func TestTextFitParses(t *testing.T) {
	fitOf := func(v string) (textFit, string) {
		return textFitOf(&Box{Style: map[string]string{"text-fit": v}})
	}
	if f, un := fitOf("grow consistent 150%"); f.mode != fitGrow || !f.hasLimit ||
		f.limit != 1.5 || un != "" {
		t.Errorf("\"grow consistent 150%%\" read as %+v, unhandled %q", f, un)
	}
	if f, _ := fitOf("150% shrink"); f.mode != fitShrink || f.limit != 1.5 {
		t.Errorf("\"150%% shrink\" read as %+v; the order is free", f)
	}
	if f, _ := fitOf(""); f.mode != fitNone {
		t.Errorf("the empty value read as %+v, want none", f)
	}
	if f, _ := fitOf("none"); f.mode != fitNone {
		t.Errorf("\"none\" read as %+v", f)
	}
	// A granularity with no mode is not a request to scale.
	if f, _ := fitOf("consistent"); f.mode != fitNone {
		t.Errorf("\"consistent\" alone read as %+v, want none", f)
	}
	if f, _ := fitOf("grow per-line"); !f.perLine || f.consistent() {
		t.Errorf("\"grow per-line\" read as %+v", f)
	}
	if f, _ := fitOf("grow per-line-all"); !f.perLineAll || f.consistent() {
		t.Errorf("\"grow per-line-all\" read as %+v", f)
	}
	if f, un := fitOf("grow sideways"); f.mode != fitNone || un != "sideways" {
		t.Errorf("\"grow sideways\" read as %+v, unhandled %q; a word outside the "+
			"grammar refuses the declaration", f, un)
	}
}

// TestAHangingSpaceIsNotTypeToFit. §4.1.2's white space at the end of a line is
// not on the page, so it is neither part of what has to fit nor part of what
// scales. The two have to agree: counting it in one and not the other makes the
// factor come out of an arithmetic where the line's width and the type on it are
// measured over different runs.
//
// "ABCD EFGH" under pre-wrap in a 60px box breaks after the space, which then
// hangs. Four characters of 10px Ahem are 40 of the 60 available, so the line
// grows by half; counting the hanging space as type gives 1.4 instead.
func TestAHangingSpaceIsNotTypeToFit(t *testing.T) {
	got := fitted(t, `#d { white-space: pre-wrap; font-family: Ahem; line-height: normal;
		width: 60px; font-size: 10px; text-fit: grow per-line-all }`,
		`<div id="d">ABCD EFGH</div>`)
	if !strings.HasPrefix(got[0], "15 ") {
		t.Errorf("the first line is %q, want its type at 15px — the space past the "+
			"end of it is neither type to fit nor type to scale", got[0])
	}
}
