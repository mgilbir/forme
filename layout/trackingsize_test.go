package layout

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A face with an AAT tracking table is tracked at the size its text is set at:
// TrakCases.ttf (testdata/harfbuzz/aatpos_fixture.py) states tracks that grow
// tighter with the size. Set at 9px and at 24px, a run of it is shaped at
// each — the size a browser hands HarfBuzz as the point size is the CSS pixel
// size, see shape.Face.FeaturesAt — so its advances are HarfBuzz's at that
// size, as aatpos.expected.txt records them, and the line is filled to the
// width the run is drawn at.

// trakAdvances is HarfBuzz's x advances, in font units, for TrakCases.ttf
// setting "AA" with kerning on at a size, read from aatpos.expected.txt.
func trakAdvances(t *testing.T, size string) []int {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "testdata", "harfbuzz", "aatpos.expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 || fields[0] != "TrakCases.ttf" || fields[2] != "0041,0041" ||
			fields[3] != size || fields[4] != "1" {
			continue
		}
		var out []int
		for _, g := range fields[5:] {
			parts := strings.Split(g, ",")
			x, err := strconv.Atoi(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, x)
		}
		return out
	}
	t.Fatalf("aatpos.expected.txt has no case of AA at %s; run `make hbaatpos`", size)
	return nil
}

// trakRuns lays "AA" out at two sizes in a face and returns each line's run,
// as layout filled it, and what it draws.
func trakRuns(t *testing.T, face *shape.Face) ([]TextRun, []DrawText) {
	t.Helper()
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="a">AA</div><div id="b">AA</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `body { margin: 0 }
			div { font-family: T; line-height: 40px; width: 600px }
			#a { font-size: 9px } #b { font-size: 24px }`}}})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(400)
	root := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	var runs []TextRun
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		for _, ln := range f.Lines {
			for _, r := range ln.Runs {
				if strings.TrimSpace(r.Text) != "" {
					runs = append(runs, r)
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	var drawn []DrawText
	for _, op := range Paint(root) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			drawn = append(drawn, v)
		}
	}
	if len(runs) != 2 || len(drawn) != 2 {
		t.Fatalf("the fixture laid out %d runs and drew %d, want two of each", len(runs), len(drawn))
	}
	return runs, drawn
}

func TestATrackedFaceIsTrackedAtTheSizeItIsSetAt(t *testing.T) {
	face := uprightFace(t, "TrakCases.ttf")
	runs, drawn := trakRuns(t, face)
	upem := float64(face.UnitsPerEm())
	perEm := make([]float64, 2)
	for i, size := range []string{"9", "24"} {
		v := drawn[i]
		px, _ := strconv.ParseFloat(size, 64)
		if v.Size.Px() != px || v.Features.PointSize != px {
			t.Fatalf("the run at %spx is drawn at %v with a point size of %v", size, v.Size.Px(), v.Features.PointSize)
		}
		want := trakAdvances(t, size)
		glyphs, _ := ShapedGlyphs(v)
		if len(glyphs) != len(want) {
			t.Fatalf("at %spx: %d glyphs, HarfBuzz %d", size, len(glyphs), len(want))
		}
		total := 0.0
		for j, g := range glyphs {
			if got := int(math.Round(g.XAdvance * upem / 1000)); got != want[j] {
				t.Errorf("at %spx, glyph %d advances %d units, and HarfBuzz %d", size, j, got, want[j])
			}
			total += g.XAdvance
		}
		// The line was filled to the width the run is drawn at.
		if drawnWidth, _ := style.FromPx(total * px / 1000); runs[i].Width != drawnWidth {
			t.Errorf("at %spx the line took %v for the run, which is drawn %v wide", size, runs[i].Width, drawnWidth)
		}
		perEm[i] = total
	}
	if perEm[0] == perEm[1] {
		t.Errorf("AA is %v thousandths of an em at both sizes, which a face tracking by size is not", perEm[0])
	}
}

// A face with no tracking table is shaped as it was: no size reaches its
// features, so every size shares one entry of each cache keyed by them.
func TestAFaceWithoutTrackingIsGivenNoSize(t *testing.T) {
	face := uprightFace(t, "TrakNoSTAT.ttf")
	_, drawn := trakRuns(t, face)
	for _, v := range drawn {
		if v.Features.PointSize != 0 {
			t.Errorf("a run of a face HarfBuzz does not track carries a point size of %v", v.Features.PointSize)
		}
	}
}
