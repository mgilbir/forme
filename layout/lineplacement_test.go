package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// -forme-line-placement: where a line's text sits within its line box, which
// is not CSS and is what setting a Word document's line rules needs. See
// paragraph.LinePlacement.
//
// The face is one built here, family "Box", whose numbers are exact: its
// ascent is four fifths of an em and its descent one fifth, so at 20px the
// type reaches 16px above the baseline and 4px below it.

// boxSet answers "Box" with that face and everything else as the standard
// fonts do.
type boxSet struct {
	box      *shape.Face
	standard FontSet
}

func (b boxSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if family == "Box" {
		return b.box, true
	}
	return b.standard.Face(family, bold, italic)
}

// placementLayout lays a document out against the Box face.
func placementLayout(t *testing.T, htmlSrc, cssSrc string) *Fragment {
	t.Helper()
	var glyphs []fonttest.Glyph
	for _, r := range "xyX " {
		glyphs = append(glyphs, fonttest.Glyph{Rune: r, Advance: 1000, HasShape: r != ' '})
	}
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatal(err)
	}
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	return Layout(built.Root, Size{W: w, H: h}, boxSet{box: face, standard: StandardFonts()}, NewRecorder(nil))
}

// placedBaselines lays out a block and returns where each of its lines'
// baselines sits, from the top of the line, and each line's height.
func placedBaselines(t *testing.T, markup, css string) (baselines, heights []float64) {
	t.Helper()
	root := placementLayout(t, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-family: Box; font-size: 20px; line-height: 40px }`+css)
	f := find(t, root, "d")
	if len(f.Lines) == 0 {
		t.Fatalf("%q produced no lines", markup)
	}
	for _, l := range f.Lines {
		baselines = append(baselines, l.Baseline.Px())
		heights = append(heights, l.Rect.H.Px())
	}
	return baselines, heights
}

// TestALinePutsItsTextWhereThePlacementSays is the property's four answers on a
// line of one size of type: 40px tall, holding 20px of it.
func TestALinePutsItsTextWhereThePlacementSays(t *testing.T) {
	for _, c := range []struct {
		value string
		want  float64
	}{
		// CSS's half-leading: 10px above the 16px ascent.
		{"auto", 26},
		// The type's top at the line's: all the leading below.
		{"top", 16},
		// The type's bottom at the line's: all the leading above.
		{"bottom", 36},
		// A baseline four fifths of the way down, which is Word's "exactly".
		{"0.8", 32},
		{"50%", 20},
		// Out of range, and so not a declaration at all: the middle.
		{"2", 26},
		{"-0.5", 26},
	} {
		got, heights := placedBaselines(t, "x", `#d { -forme-line-placement: `+c.value+` }`)
		if got[0] != c.want {
			t.Errorf("-forme-line-placement: %s put the baseline %gpx down, want %g", c.value, got[0], c.want)
		}
		if heights[0] != 40 {
			t.Errorf("-forme-line-placement: %s made the line %gpx tall; it moves the text and not the line",
				c.value, heights[0])
		}
	}
}

// TestEachLineIsPlacedByItsOwnTallestText is why the placement is per line
// and not a matter of moving the paragraph: the line holding the 40px span
// puts the span's top at its top, and the line after it, holding only the
// paragraph's own 20px type, puts that at its top.
func TestEachLineIsPlacedByItsOwnTallestText(t *testing.T) {
	const markup = `<span style="font-size: 40px">X</span> x<br>x`
	// The span inherits the 60px line-height, so with half-leading its box
	// reaches 10 + 32 = 42px above the baseline and the block's strut 24px
	// below it: the first line is 66px tall, and the second 60px.
	const css = `#d { line-height: 60px }`
	_, heights := placedBaselines(t, markup, css)
	if heights[0] != 66 || heights[1] != 60 {
		t.Fatalf("the lines are %v tall, want [66 60]; this test's arithmetic assumes it", heights)
	}
	for _, c := range []struct {
		value string
		want  [2]float64
	}{
		// The 40px span's ascent, and then the paragraph's.
		{"top", [2]float64{32, 16}},
		// The line's height less the deepest descent on it: the span's 8px,
		// and then the paragraph's 4px.
		{"bottom", [2]float64{58, 56}},
		{"0.5", [2]float64{33, 30}},
	} {
		got, heights := placedBaselines(t, markup, css+` #d { -forme-line-placement: `+c.value+` }`)
		if got[0] != c.want[0] || got[1] != c.want[1] {
			t.Errorf("-forme-line-placement: %s put the baselines at %v, want %v", c.value, got, c.want)
		}
		if heights[0] != 66 || heights[1] != 60 {
			t.Errorf("-forme-line-placement: %s made the lines %v tall, want [66 60]", c.value, heights)
		}
	}
}

// TestThePlacementIsInheritedAndReadFromTheBlock: a paragraph inside a <div>
// that asks for it is placed by it, as a paragraph inside a <div> with a
// line-height is spaced by that; and a span asking for something else does
// not move the line it is on, whose placement is its block's.
func TestThePlacementIsInheritedAndReadFromTheBlock(t *testing.T) {
	root := placementLayout(t, `<div id="d"><p id="p">x</p></div>`,
		noDefaults+`#d { font-family: Box; font-size: 20px; line-height: 40px; -forme-line-placement: top }`)
	if p := find(t, root, "p"); len(p.Lines) == 0 || p.Lines[0].Baseline.Px() != 16 {
		t.Errorf("a paragraph inside a block placing its lines at the top was not placed by it")
	}
	got, _ := placedBaselines(t, `x <span style="-forme-line-placement: bottom">y</span>`,
		`#d { -forme-line-placement: top }`)
	if got[0] != 16 {
		t.Errorf("a span asking for its own placement moved its line to %gpx, want the block's 16", got[0])
	}
}

// TestALineSetDownThePageIsLeftInTheMiddle: a vertical line's runs are aligned
// by their central baseline, and the placement is not defined there.
func TestALineSetDownThePageIsLeftInTheMiddle(t *testing.T) {
	plain, _ := placedBaselines(t, "x", `#d { writing-mode: vertical-rl }`)
	placed, _ := placedBaselines(t, "x", `#d { writing-mode: vertical-rl; -forme-line-placement: top }`)
	if plain[0] != placed[0] {
		t.Errorf("a vertical line was placed at %gpx, want the %gpx it has without the property", placed[0], plain[0])
	}
}
