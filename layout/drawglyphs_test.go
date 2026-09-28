package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// DrawGlyphs through every stage of the paint that handles an operation by its
// kind: its ink, the clip, the rounded corner, opacity, the colour matrix, the
// drop shadow and the page overflow guard. Each is asked what it asks of a
// DrawText, and of the glyphs' ink, which is known exactly.

// mathVariantGlyphs is the parenthesis's variant of the stretchy face — ink
// from 50 to 400 across and from 500 below its baseline to 1100 above — at
// 16px, where a font unit is a style.Unit, drawn from (1000, 2000).
func mathVariantGlyphs(t *testing.T) DrawGlyphs {
	return DrawGlyphs{At: Point{X: 1000, Y: 2000}, Text: "(", Face: mathStretchFace(t, nil), Size: 1024,
		Color: style.RGBA{B: 255, A: 1}, Glyphs: []shape.Glyph{{GID: 22, XAdvance: thousandths(450)}}}
}

// TestTheInkOfGlyphsIsTheirs: one glyph's ink box where it is drawn; two, the
// second moved up by its offset and along by the first's advance; and a glyph
// whose face cannot say where its ink is, its em square.
func TestTheInkOfGlyphsIsTheirs(t *testing.T) {
	v := mathVariantGlyphs(t)
	if got := glyphsInk(v); got != (Rect{X: 1050, Y: 900, W: 350, H: 1600}) {
		t.Errorf("the variant's ink is %v, want 350 by 1600 at (1050, 900)", got)
	}
	// At a size where the edges fall between units, each is rounded outwards:
	// at 1000 units, 15.625px, the ink is from 48.83 to 390.63 across and
	// from 1074.22 above to 488.28 below.
	odd := v
	odd.Size = 1000
	if got := glyphsInk(odd); got != (Rect{X: 1048, Y: 925, W: 343, H: 1564}) {
		t.Errorf("the variant's ink at 1000 is %v, want 343 by 1564 at (1048, 925)", got)
	}
	// The assembly's bottom, and its top 500 up and 450 along: the top's ink
	// is 50 to 450 across and 0 to 600 up of its own origin.
	v.Glyphs = []shape.Glyph{{GID: 23, XAdvance: thousandths(450)}, {GID: 25, YOffset: thousandths(500)}}
	if got := glyphsInk(v); got != (Rect{X: 1050, Y: 900, W: 850, H: 1100}) {
		t.Errorf("two pieces' ink is %v, want 850 by 1100 at (1050, 900)", got)
	}
	// A glyph with no ink adds nothing: the space.
	v.Glyphs = []shape.Glyph{{GID: 1, XAdvance: thousandths(256)}, {GID: 22}}
	if got := glyphsInk(v); got != (Rect{X: 1306, Y: 900, W: 350, H: 1600}) {
		t.Errorf("a space and the variant: %v, want the variant 256 along", got)
	}
	// A standard face states no glyph's ink: an em on the baseline.
	std, _ := StandardFonts().Face("serif", false, false)
	v.Face, v.Glyphs = std, []shape.Glyph{{GID: 40}}
	if got := glyphsInk(v); got != (Rect{X: 1000, Y: 976, W: 1024, H: 1024}) {
		t.Errorf("a glyph of a standard face: %v, want its em, 1024 square, from (1000, 976)", got)
	}
	if got := glyphsInk(DrawGlyphs{Glyphs: v.Glyphs}); !got.Empty() {
		t.Errorf("glyphs with no face have ink %v", got)
	}
}

// TestAClipCutsGlyphsAsItCutsText: wholly inside, the glyphs carry no clip;
// cut, they carry it; wholly outside, they are gone.
func TestAClipCutsGlyphsAsItCutsText(t *testing.T) {
	v := mathVariantGlyphs(t)
	for _, tc := range []struct {
		clip    Rect
		kept    bool
		clipped bool
	}{
		{Rect{X: 0, Y: 0, W: 5000, H: 5000}, true, false},
		{Rect{X: 0, Y: 0, W: 5000, H: 1500}, true, true},
		{Rect{X: 0, Y: 3000, W: 5000, H: 500}, false, false},
	} {
		got := clipOps([]Op{v}, 0, Clip{Rect: tc.clip, Active: true})
		if kept := len(got) == 1; kept != tc.kept {
			t.Errorf("clip %v: kept %v, want %v", tc.clip, kept, tc.kept)
			continue
		}
		if tc.kept {
			if c := got[0].(DrawGlyphs).Clip; c.Active != tc.clipped {
				t.Errorf("clip %v: the glyphs carry %v, want a clip %v", tc.clip, c, tc.clipped)
			}
		}
	}
	if r, ok := opBounds(v); !ok || r != glyphsInk(v) {
		t.Errorf("a rounded corner asks %v (%v) of the glyphs, want their ink", r, ok)
	}
}

// TestOpacityAndColourReachGlyphs: an alpha folds into their colour, and an
// alpha of nought drops them; a colour matrix maps their colour; a drop
// shadow is them, moved and tinted, standing for no text.
func TestOpacityAndColourReachGlyphs(t *testing.T) {
	v := mathVariantGlyphs(t)
	got, marks := dimOps([]Op{v}, 0, 0.5)
	if len(got) != 1 || got[0].(DrawGlyphs).Color.A != 0.5 || len(marks) != 1 || !marks[0].text {
		t.Errorf("dimmed by half: %+v, marks %+v", got, marks)
	}
	if got, marks := dimOps([]Op{v}, 0, 0); len(got) != 0 || len(marks) != 1 {
		t.Errorf("dimmed to nothing: %+v, marks %+v", got, marks)
	}
	clear := v
	clear.Color.A = 0
	if got, marks := dimOps([]Op{clear}, 0, 0.5); len(got) != 1 || len(marks) != 0 {
		t.Errorf("transparent glyphs dimmed: %+v, marks %+v", got, marks)
	}

	var seen []style.RGBA
	if !eachColour([]Op{v}, func(c style.RGBA) bool { seen = append(seen, c); return true }) || len(seen) != 1 || seen[0] != v.Color {
		t.Errorf("the glyphs' colours are %v", seen)
	}
	red := style.RGBA{R: 255, A: 1}
	if got := mapColours([]Op{v}, func(style.RGBA) style.RGBA { return red }); got[0].(DrawGlyphs).Color != red {
		t.Errorf("mapped, the glyphs are %v", got[0].(DrawGlyphs).Color)
	}
	var identity [20]float64
	identity[0], identity[6], identity[12], identity[18] = 1, 1, 1, 1
	half := v
	half.Color.A = 0.5
	under := FillRect{Rect: Rect{X: 1000, Y: 1000, W: 500, H: 500}, Color: red}
	if unmixed([]Op{under, half}, identity) {
		t.Error("translucent glyphs over a fill are said not to mix with it")
	}

	if !shadowable([]Op{v}) {
		t.Error("glyphs are not shadowable")
	}
	sh := shadowMarks([]Op{v}, Point{X: 10, Y: 20}, style.RGBA{A: 0.5})
	if len(sh) != 1 {
		t.Fatalf("the shadow is %+v", sh)
	}
	s := sh[0].(DrawGlyphs)
	if s.At != (Point{X: 1010, Y: 2020}) || s.Text != "" || s.Color != (style.RGBA{A: 1}) || len(s.Glyphs) != 1 {
		t.Errorf("the shadow is %+v, want the glyphs 10 across and 20 down, black, standing for nothing", s)
	}
}

// TestGlyphsAreNotABoxOffThePage: the overflow guard is about boxes, and
// skips text; glyphs are text's.
func TestGlyphsAreNotABoxOffThePage(t *testing.T) {
	v := mathVariantGlyphs(t)
	considered := 0
	checkOp(v, func(Rect) { considered++ }, func([]Op) {})
	if considered != 0 {
		t.Error("the overflow guard considers a formula's glyphs")
	}
}

// TestTheComparisonSeesEveryGlyph: the reftest oracle compares glyphs drawn
// by index glyph by glyph, where each is drawn — a piece of an assembly moved
// up, another glyph, another colour, or no glyphs at all is another page —
// and the blank-page check sees them as marks.
func TestTheComparisonSeesEveryGlyph(t *testing.T) {
	v := mathVariantGlyphs(t)
	v.Glyphs = []shape.Glyph{{GID: 23}, {GID: 25, YOffset: thousandths(500)}}
	page := Rect{W: 10000, H: 10000}
	if !pictureEqual([]Op{v}, []Op{v}, page) {
		t.Error("the same glyphs are not the same picture")
	}
	moved := v
	moved.Glyphs = []shape.Glyph{{GID: 23}, {GID: 25, YOffset: thousandths(600)}}
	other := v
	other.Glyphs = []shape.Glyph{{GID: 23}, {GID: 24, YOffset: thousandths(500)}}
	red := v
	red.Color = style.RGBA{R: 255, A: 1}
	for name, w := range map[string][]Op{"a piece moved up": {moved}, "another piece": {other},
		"another colour": {red}, "nothing": nil} {
		if pictureEqual([]Op{v}, w, page) {
			t.Errorf("%s is the same picture", name)
		}
	}
	if normaliseOps([]Op{v}) == "" {
		t.Error("the blank-page check does not see glyphs")
	}
}
