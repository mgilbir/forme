package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// Where the comparison takes a run's ink to be across the line: its glyphs'
// own boxes, and not its advance. See markInk.
//
// The fixture face is 1000 units to the em and drawn at 20px, so a unit is a
// fiftieth of a pixel:
//
//	"a"  advance 1000, ink from 0 to 250   — a mark in the left quarter, as a
//	     full-width closing bracket is
//	"b"  advance 1000, ink from -200 to 1200 — reaching 4px past both ends
//	"c"  advance 1000, ink from 750 to 1000 — the right quarter, and under
//	     'halt' placed 500 back and advancing 500
//	" "  advance 250, no ink
//	"e"  advance 500, no ink: a letter that draws nothing
//	"　" advance 1000, and a box drawn in it, as a face can
func inkFace(t *testing.T) *shape.Face {
	t.Helper()
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Ink",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 1000, HasShape: true, Ink: [4]int{0, 0, 250, 700}},
			{Rune: 'b', Advance: 1000, HasShape: true, Ink: [4]int{-200, 0, 1200, 700}},
			{Rune: 'c', Advance: 1000, HasShape: true, Ink: [4]int{750, 0, 1000, 700}},
			{Rune: ' ', Advance: 250},
			{Rune: 'e', Advance: 500},
			// An ideographic space in a face that draws something for it.
			{Rune: 0x3000, Advance: 1000, HasShape: true, Ink: [4]int{100, 0, 900, 700}},
		},
		// 'halt' moves the "c" half an em back into the space its blank
		// vacated, as a full-width opening bracket's half-width form does:
		// a placement of -500 and an advance 500 shorter.
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
				{Type: 1, Subtables: [][]byte{fonttest.SinglePosSubtable(3, -500, 0, -500)}},
			}, map[string][]int{"halt": {0}}),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// inkRun is a red run of text in the fixture face at 20px, its baseline at 30px.
func inkRun(face *shape.Face, text string, x float64) DrawText {
	return DrawText{Text: text, At: Point{X: picPx(x), Y: picPx(30)},
		Size: picPx(20), Face: face, Color: picRed}
}

// TestTheComparisonReadsAGlyphsInkAcrossTheLine is markInk's arithmetic, one
// glyph at a time and then along a run.
func TestTheComparisonReadsAGlyphsInkAcrossTheLine(t *testing.T) {
	face := inkFace(t)
	for _, tc := range []struct {
		text, tags  string
		spacing     float64
		left, right float64
		what        string
	}{
		{"a", "", 0, 10, 15, "the left quarter of a 20px advance"},
		{"b", "", 0, 6, 34, "4px past each end of its advance"},
		{"c", "", 0, 25, 30, "the right quarter"},
		{"ac", "", 0, 10, 50, "from the first glyph's ink to the last's"},
		// The pen moves as glyphMarks moves it: letter-spacing after each
		// character puts the "c" 5px further along.
		{"ac", "", 5, 10, 55, "with the letter-spacing after the first character"},
		// A space moves the pen and has no ink of its own.
		{"a c", "", 0, 10, 55, "across a space"},
		// White space marks no paper for the comparison whatever the face
		// draws in it — glyphMarks makes no mark of it — so its box is not ink
		// here either, and the two agree about what is on the page.
		{"a\u3000", "", 0, 10, 15, "a space the face draws in"},
		// A letter with no ink moves the pen and puts nothing on the page, so
		// the ink begins at the "a" after it and not at the run's origin.
		{"ea", "", 0, 20, 25, "after a glyph that draws nothing"},
		// The glyph's own offset moves its ink, and the shortened advance
		// moves what follows: the "c" is drawn 10px back, and an "a" after it
		// begins 10px after the run does.
		{"c", "halt", 0, 15, 20, "placed half an em back"},
		{"ca", "halt", 0, 15, 25, "placed back, with a glyph after it"},
	} {
		run := inkRun(face, tc.text, 10)
		run.CharSpacing = picPx(tc.spacing)
		run.Features.Tags = tc.tags
		got := markInk(run)
		if got.X != picPx(tc.left) || got.X.Add(got.W) != picPx(tc.right) {
			t.Errorf("%q, %s: the ink runs from %v to %v, want %v to %v", tc.text,
				tc.what, got.X, got.X.Add(got.W), picPx(tc.left), picPx(tc.right))
		}
		// Across the line it is textInk's, which this does not change.
		if ink := textInk(run); got.Y != ink.Y || got.H != ink.H {
			t.Errorf("%q: the ink reaches %v tall from %v, and textInk's %v from %v",
				tc.text, got.H, got.Y, ink.H, ink.Y)
		}
	}
}

// TestAGlyphsBlankIsNotInkThatCanBeSeen is the case that made the change:
// line-break-anywhere-001's full-width bracket in a column narrower than its
// advance, under an opaque box that covers all of its ink. The run is buried,
// and the page is the box alone.
func TestAGlyphsBlankIsNotInkThatCanBeSeen(t *testing.T) {
	face := inkFace(t)
	run := inkRun(face, "a", 10)
	ink := markInk(run)
	// A box over the ink and 2px to either side of it — and 13px short of the
	// run's advance, which ends at 30.
	cover := FillRect{Rect: Rect{X: ink.X.Sub(picPx(2)), Y: ink.Y.Sub(picPx(2)),
		W: ink.W.Add(picPx(4)), H: ink.H.Add(picPx(4))}, Color: picGreen}
	if !pictureEqual([]Op{run, cover}, []Op{cover}, picPage) {
		t.Error("a glyph whose ink is wholly under an opaque box was counted as " +
			"a mark: the blank part of its advance is not ink anybody can see")
	}
	// The control: a box a pixel short of the ink on the right leaves some of
	// it showing, and the run is a mark.
	short := cover
	short.Rect.W = ink.X.Add(ink.W).Sub(picPx(1)).Sub(short.Rect.X)
	if pictureEqual([]Op{run, short}, []Op{short}, picPage) {
		t.Error("a glyph with a pixel of ink outside the box was treated as buried")
	}
}

// TestAGlyphsOverhangIsInkThatCanBeSeen is the other direction, and the one
// that keeps this from being a loosening: ink past the advance is ink. A box
// exactly as wide as the run's advance buried the "b" when the advance was all
// the comparison read, and 4px of it show on each side.
func TestAGlyphsOverhangIsInkThatCanBeSeen(t *testing.T) {
	face := inkFace(t)
	run := inkRun(face, "b", 10)
	across := markInk(run)
	advance := FillRect{Rect: Rect{X: picPx(10), Y: across.Y.Sub(picPx(2)),
		W: picPx(20), H: across.H.Add(picPx(4))}, Color: picGreen}
	if pictureEqual([]Op{run, advance}, []Op{advance}, picPage) {
		t.Error("a glyph reaching 4px past a box as wide as its advance was treated " +
			"as buried")
	}
	all := advance
	all.Rect.X, all.Rect.W = across.X, across.W
	if !pictureEqual([]Op{run, all}, []Op{all}, picPage) {
		t.Error("a box over the whole of the glyph's ink did not bury it")
	}
}

// TestAFaceThatCannotSayKeepsTheAdvance: a standard face states its boxes by
// character name, not by glyph, so its runs are read as they always were.
func TestAFaceThatCannotSayKeepsTheAdvance(t *testing.T) {
	run := picRun(t, "FAIL", 0, 14)
	if got, want := markInk(run), textInk(run); got != want {
		t.Errorf("a run in a standard face has ink %v, want textInk's %v", got, want)
	}
	// And a run of nothing but blanks, which has no box to read either.
	blank := inkRun(inkFace(t), "  ", 10)
	if got, want := markInk(blank), textInk(blank); got != want {
		t.Errorf("a run of spaces has ink %v, want textInk's %v", got, want)
	}
}

// TestASidewaysRunsInkRunsDownThePage: a run on a vertical line is the
// horizontal run turned a quarter clockwise, so the ink along it is down the
// page from its origin, and across it is textInk's.
func TestASidewaysRunsInkRunsDownThePage(t *testing.T) {
	run := inkRun(inkFace(t), "a", 10)
	run.At = Point{X: picPx(100), Y: picPx(40)}
	run.Sideways = true
	got := markInk(run)
	if got.Y != picPx(40) || got.H != picPx(5) {
		t.Errorf("a sideways \"a\" has ink from %v, %v tall; want from 40px, 5px tall",
			got.Y, got.H)
	}
	if ink := textInk(run); got.X != ink.X || got.W != ink.W {
		t.Errorf("across the line the ink is %v wide at %v, and textInk's %v at %v",
			got.W, got.X, ink.W, ink.X)
	}
}
