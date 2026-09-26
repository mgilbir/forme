package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// An upright run is as long as its glyphs' vertical advances, where the face
// states them (#771).
//
// Two of the faces the vertical shaping oracle is run over are committed and
// answer the question two ways. VerticalComposites.ttf states vertical metrics:
// its "A" advances nine tenths of an em down the page and is 350 units wide.
// VerticalFallbacks.ttf states none, and shaping gives it HarfBuzz's synthesis,
// the height of its line — 1.2 em — which is not CSS's: CSS Writing Modes §4.4
// has the UA synthesize an em box.

func uprightFace(t *testing.T, name string) *shape.Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", name))
	if err != nil {
		t.Fatal(err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// uprightRunsIn lays "AAAA<span>D</span>" out upright at 20px in the face and
// returns the runs it draws.
func uprightRunsIn(t *testing.T, face *shape.Face) []DrawText {
	t.Helper()
	set := namedFaceSet{family: "V", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="d">AAAA<span>D</span></div>`,
		CSS: []Stylesheet{{Source: noDefaults + `body { margin: 0 }
			#d { font-family: V; font-size: 20px; line-height: 20px;
			     writing-mode: vertical-rl; text-orientation: upright;
			     width: 60px; height: 400px }`}}})
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(1000)
	var out []DrawText
	for _, op := range Paint(Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v)
		}
	}
	if len(out) != 2 || !out[0].Upright {
		t.Fatalf("the fixture drew %+v, want two upright runs", out)
	}
	return out
}

// TestAnUprightRunAdvancesByItsVerticalMetrics.
func TestAnUprightRunAdvancesByItsVerticalMetrics(t *testing.T) {
	for _, tc := range []struct {
		face          string
		along, across float64
		what          string
	}{
		{"VerticalComposites.ttf", 72, 7, "a face with vmtx: 0.9 em a letter, 350 units across"},
		{"VerticalFallbacks.ttf", 80, 20, "a face with none: §4.4's em box, not HarfBuzz's 1.2 em"},
	} {
		runs := uprightRunsIn(t, uprightFace(t, tc.face))
		// Where the run after it starts is where layout says this one ends —
		// the width the line was filled to.
		if got := runs[1].At.Y.Sub(runs[0].At.Y).Px(); got != tc.along {
			t.Errorf("%s: the run after four letters starts %gpx down, want %g",
				tc.what, got, tc.along)
		}
		ink := textInk(runs[0])
		if ink.H.Px() != tc.along {
			t.Errorf("%s: the run reaches %gpx along the line, want %g", tc.what,
				ink.H.Px(), tc.along)
		}
		if ink.W.Px() != tc.across {
			t.Errorf("%s: the run reaches %gpx across the line, want %g", tc.what,
				ink.W.Px(), tc.across)
		}
		// Centred on the line's middle, where each glyph is hung from.
		if got := runs[0].At.X.Sub(ink.X).Px(); got != tc.across/2 {
			t.Errorf("%s: the middle of the line is %gpx from the left of the run, "+
				"want %g", tc.what, got, tc.across/2)
		}
	}
}

// TestAnUprightRunIsWhereItsShapedGlyphsAre. A backend that shapes the run with
// shape.Features.Vertical and steps its pen by YAdvance ends the run where layout
// placed the next one.
func TestAnUprightRunIsWhereItsShapedGlyphsAre(t *testing.T) {
	face := uprightFace(t, "VerticalComposites.ttf")
	runs := uprightRunsIn(t, face)
	off := runs[0].Features
	off.Vertical = true
	glyphs, _ := face.ShapeGlyphsInContext(runs[0].Text, runs[0].PreContext,
		runs[0].PostContext, off)
	var pen float64
	for _, g := range glyphs {
		pen -= g.YAdvance
	}
	end, _ := style.FromPx(pen * runs[0].Size.Px() / 1000)
	if got := runs[0].At.Y.Add(end); got != runs[1].At.Y {
		t.Errorf("the shaped glyphs end at %gpx, and the next run starts at %gpx",
			got.Px(), runs[1].At.Y.Px())
	}
}
