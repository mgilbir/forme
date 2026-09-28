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

// uprightRunsOf lays text followed by "<span>D</span>" out upright at 20px in
// the face and returns the runs it draws: the first is the run of text, and
// the second starts where layout ended it.
func uprightRunsOf(t *testing.T, face *shape.Face, text string) []DrawText {
	t.Helper()
	set := namedFaceSet{family: "V", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="d">` + text + `<span>D</span></div>`,
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
	if len(out) != 2 || !out[0].Upright || out[0].Text != text {
		t.Fatalf("%q drew %+v, want two upright runs, the first of it", text, out)
	}
	return out
}

// TestShapedGlyphsAdvanceAnUprightRunAsLayoutMeasuredIt is the contract
// ShapedGlyphs states for an upright run, asked the way a backend asks it: from
// the DrawText alone. Its glyphs, stepped by -YAdvance, end where layout
// started the next run — in a face with vertical metrics, whose advances are
// its vmtx's, and in one with none, whose advances are the em per character
// that CSS Writing Modes §4.4 has layout synthesize. The runs hold a combining
// mark and a zero width joiner, which take no advance of their own.
//
// Before, ShapedGlyphs shaped the run as a horizontal one: every YAdvance was
// zero, and the face's horizontal advances — "A" is 350 units wide in the face
// with vmtx, and advances 900 down — were all a backend had.
func TestShapedGlyphsAdvanceAnUprightRunAsLayoutMeasuredIt(t *testing.T) {
	for _, name := range []string{"VerticalComposites.ttf", "VerticalFallbacks.ttf"} {
		face := uprightFace(t, name)
		for _, text := range []string{"AAAA", "AÁA‍A"} {
			runs := uprightRunsOf(t, face, text)
			glyphs, _ := ShapedGlyphs(runs[0])
			if len(glyphs) == 0 {
				t.Fatalf("%s, %q: no glyphs", name, text)
			}
			var pen float64
			for _, g := range glyphs {
				if g.XAdvance != 0 {
					t.Errorf("%s, %q: glyph %d advances %g across the column", name, text, g.GID, g.XAdvance)
				}
				pen -= g.YAdvance
			}
			end, _ := style.FromPx(pen * runs[0].Size.Px() / 1000)
			if got := runs[0].At.Y.Add(end); got != runs[1].At.Y {
				t.Errorf("%s, %q: the glyphs ShapedGlyphs gives end at %gpx, and layout "+
					"starts the next run at %gpx", name, text, got.Px(), runs[1].At.Y.Px())
			}
		}
	}
}

// TestEmPerUnitKeepsTheTotal is the distribution on its own, over glyphs
// arranged as a face may arrange them: a character drawn into the glyph of the
// one before it, a character drawn as two glyphs, a mark, and the glyphs out
// of their clusters' order.
func TestEmPerUnitKeepsTheTotal(t *testing.T) {
	// "ab́cd": units start at 0 (a), 1 (b with its mark), 4 (c), 5 (d).
	text := "ab́cd"
	glyphs := []shape.Glyph{
		{GID: 1, Cluster: 0, YAdvance: -7}, // a
		{GID: 2, Cluster: 1, YAdvance: -7}, // b, first of two glyphs
		{GID: 3, Cluster: 1, YAdvance: -7}, // b, second
		{GID: 4, Cluster: 2, YAdvance: -7}, // the mark
		{GID: 5, Cluster: 4, YAdvance: -7}, // c and d, drawn as one
	}
	// Out of order, which a caller cannot rule out: the answer is by cluster.
	glyphs[0], glyphs[4] = glyphs[4], glyphs[0]
	emPerUnit(glyphs, text)
	want := map[int]float64{1: -1000, 2: -1000, 3: 0, 4: 0, 5: -2000}
	for _, g := range glyphs {
		if g.YAdvance != want[g.GID] {
			t.Errorf("glyph %d advances %g, want %g", g.GID, g.YAdvance, want[g.GID])
		}
	}
}
