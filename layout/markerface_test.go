package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The face a list marker is set in, CSS Fonts §5.2 as it reaches §12.5.
//
// A marker is text and is chosen the way every other character is: the first
// available font that can set it, passing over a family that cannot. The box's
// own font was used whatever it held — and a bullet is exactly the character a
// text face is most likely not to have. U+25AA BLACK SMALL SQUARE is not in the
// fourteen standard PDF faces, so "list-style-type: square" in a serif drew that
// face's notdef where every browser draws a small black square from elsewhere.
//
// content-counter-004 is the suite's document and it makes the point twice over:
// its test half writes the square through "content: counter(c, square)", which
// is ordinary text and always fell back, and its reference half writes the same
// square as an "<li>" marker, which did not. The two halves of one document
// disagreeing about one character is what a marker not going through the
// fallback looks like.

// markerRunOf is the marker's run on a list item's first line, laid out with the
// font library the suite harness lends the engine — the standard faces with the
// Noto faces behind them, which is what a caller supplies and what makes a
// fallback possible at all.
func markerRunOf(t *testing.T, css string) (TextRun, bool) {
	t.Helper()
	if len(fallbackFacesInUse()) == 0 {
		t.Skip("no fallback faces; set NOTO_FONTS")
	}
	built := Build(Input{
		HTML:  `<ul><li id="i">x</li></ul>`,
		CSS:   []Stylesheet{{Source: noDefaults + css}},
		Fonts: fontSetForWPT(),
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(400)
	laid := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	f := find(t, laid, "i")
	if len(f.Lines) == 0 || len(f.Lines[0].Runs) == 0 {
		return TextRun{}, false
	}
	return f.Lines[0].Runs[0], true
}

const markerCSS = `ul { display: block }
	li { display: list-item; list-style-position: inside;
	     font-family: Times-Roman; font-size: 20px }`

// TestAMarkerIsSetInAFaceThatHasIt is the rule.
//
// Times-Roman has the bullet and the degree-like circle and does not have the
// small square, so the three markers below split the answer: two keep the
// declared face and one has to go and find another. A marker that stayed in
// Times-Roman for all three is the behaviour this replaced.
func TestAMarkerIsSetInAFaceThatHasIt(t *testing.T) {
	for _, tc := range []struct {
		style, text string
		declared    bool
	}{
		{"disc", "•", true},
		{"decimal", "1.", true},
		{"square", "▪", false},
	} {
		run, ok := markerRunOf(t, markerCSS+` li { list-style-type: `+tc.style+` }`)
		if !ok {
			t.Errorf("%s: the item has no runs", tc.style)
			continue
		}
		if run.Text != tc.text {
			t.Errorf("%s: the first run is %q, want %q", tc.style, run.Text, tc.text)
			continue
		}
		if run.Face == nil {
			t.Errorf("%s: the marker has no face", tc.style)
			continue
		}
		if got := run.Face.Name() == "Times-Roman"; got != tc.declared {
			t.Errorf("%s: the marker is set in %s; the declared family %s have "+
				"this character", tc.style, run.Face.Name(),
				map[bool]string{true: "does", false: "does not"}[tc.declared])
		}
		if _, covered := run.Face.GlyphID([]rune(tc.text)[0]); !covered {
			t.Errorf("%s: the marker is set in %s, which has no glyph for it",
				tc.style, run.Face.Name())
		}
	}
}

// TestAMarkersLineIsAsTallAsTheFaceThatDrewIt is the other half, and the one a
// picture shows: §10.8.1 measures leading against "the font", and a marker the
// declared face could not set is not in that font.
//
// A square borrowed from a Noto face sat on a line the size of Times-Roman,
// which is the face that could not draw it. The two markers below are the same
// size in the same box and differ only in which face has them.
func TestAMarkersLineIsAsTallAsTheFaceThatDrewIt(t *testing.T) {
	if len(fallbackFacesInUse()) == 0 {
		t.Skip("no fallback faces; set NOTO_FONTS")
	}
	height := func(style string) style.Unit {
		t.Helper()
		built := Build(Input{
			HTML: `<ul><li id="i"></li></ul>`,
			CSS: []Stylesheet{{Source: noDefaults + markerCSS +
				` li { list-style-type: ` + style + ` }`}},
			Fonts: fontSetForWPT(),
		})
		laid := Layout(built.Root, wptViewport(), built.Fonts, NewRecorder(nil))
		f := find(t, laid, "i")
		if len(f.Lines) == 0 {
			t.Fatalf("%s: the item has no line", style)
		}
		return f.Lines[0].Rect.H
	}
	inDeclared, inFallback := height("disc"), height("square")
	if inDeclared == inFallback {
		t.Errorf("a bullet Times-Roman has and a square it does not gave the same "+
			"line height, %v; the two faces differ and the line is measured "+
			"against the one that drew the marker", inDeclared)
	}
}

// TestFirstRuneOfNothing, because markerRun asks for one and a marker with no
// text is a state markerItem has its own branch for.
func TestFirstRuneOfNothing(t *testing.T) {
	if got := firstRune(""); got != 0 {
		t.Errorf("firstRune(\"\") is %q, want 0", got)
	}
	if got := firstRune("1."); got != '1' {
		t.Errorf("firstRune(\"1.\") is %q, want '1' — a number's face is decided "+
			"by its first digit", got)
	}
}
