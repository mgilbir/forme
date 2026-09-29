package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// Face.GlyphOutline, held to two other readers of the same font programs.
//
// testdata/harfbuzz/outline.py draws glyphs with HarfBuzz — glyf composites,
// CFF and CFF2 charstrings, VARC, and any of them at a point of a design space
// — and with fontTools, an independent parse of the bytes, and checks the
// answers in as outline.expected.txt. Each is a list of contours, and a
// contour's start point is not compared: every reader begins one at another of
// its points, so the edges are matched as a cycle.

// outlineTolerance is how far a coordinate may be from the oracle's, per case.
// A face at its default is drawn in the units the font states it in, and the
// only slack is single precision arithmetic in a composite's transform. A face
// cut at a location has been through this package's instancer, which rounds
// every glyf point to a unit as glyf stores them, where HarfBuzz keeps the
// fraction; and each arrives at the location's scalar by its own arithmetic
// (see testdata/varinstance/instance.py, which measures the same noise).
func outlineTolerance(loc string) float64 {
	if loc == "{}" {
		return 0.01
	}
	return 2
}

// outlineContour is a contour as the oracle writes it: a start and the edges
// that go on from it.
type outlineContour struct {
	start [2]float64
	edges []outlineEdge
}

type outlineEdge struct {
	op  byte
	pts []float64
}

// contoursOf reads a face's segments as contours.
func contoursOf(segs []Segment) []outlineContour {
	var out []outlineContour
	var cur *outlineContour
	closeUp := func() {
		if cur != nil && len(cur.edges) > 0 {
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, s := range segs {
		switch s.Op {
		case MoveTo:
			closeUp()
			cur = &outlineContour{start: [2]float64{s.Pts[0].X, s.Pts[0].Y}}
		case LineTo:
			cur.edges = append(cur.edges, outlineEdge{'L', []float64{s.Pts[0].X, s.Pts[0].Y}})
		case QuadTo:
			cur.edges = append(cur.edges, outlineEdge{'Q', []float64{s.Pts[0].X, s.Pts[0].Y, s.Pts[1].X, s.Pts[1].Y}})
		case CubicTo:
			cur.edges = append(cur.edges, outlineEdge{'C', []float64{
				s.Pts[0].X, s.Pts[0].Y, s.Pts[1].X, s.Pts[1].Y, s.Pts[2].X, s.Pts[2].Y}})
		}
	}
	closeUp()
	return out
}

// closed is a contour with its closing settled, so that two of them can be
// compared as cycles. A contour is closed by the line back to where it began,
// so a last line that ends within snap of the start is that line, and is
// dropped; a contour without one is closed all the same. The oracle writes the
// line explicitly where a contour does not end at its start, and this package
// leaves it out, and each is the other's contour once neither has one.
//
// A face at its default ends a contour where it began, or says so with a line.
// One cut at a location has been through this package's instancer, which writes
// a charstring's points to whole units, one step at a time, so a contour may
// come back a unit short of where it began; snap is what that is allowed.
func closed(c outlineContour, snap float64) outlineContour {
	out := outlineContour{start: c.start, edges: append([]outlineEdge(nil), c.edges...)}
	last := out.edges[len(out.edges)-1]
	d := math.Hypot(last.pts[len(last.pts)-2]-out.start[0], last.pts[len(last.pts)-1]-out.start[1])
	if last.op == 'L' && d <= snap && len(out.edges) > 1 {
		out.edges = out.edges[:len(out.edges)-1]
	}
	return out
}

// outlineOf is a face's outline of a glyph, as the segments the callback is
// handed.
func outlineOf(f *Face, gid int) ([]Segment, error) {
	var segs []Segment
	err := f.GlyphOutline(gid, func(s Segment) bool { segs = append(segs, s); return true })
	return segs, err
}

// sameCycle says whether two contours are the same edges from some starting
// edge, each coordinate within tol, and how far the nearest rotation's worst
// coordinate was, which is the number to set a tolerance from.
func sameCycle(a, b outlineContour, tol float64) (ok bool, worst float64) {
	n := len(a.edges)
	if n != len(b.edges) {
		return false, math.Inf(1)
	}
	best := math.Inf(1)
	for r := 0; r < n; r++ {
		worstHere := 0.0
		for i := 0; i < n; i++ {
			x, y := a.edges[i], b.edges[(i+r)%n]
			if x.op != y.op || len(x.pts) != len(y.pts) {
				worstHere = math.Inf(1)
				break
			}
			for k := range x.pts {
				// HarfBuzz keeps a coordinate in a single precision float, which
				// a charstring that walks a million units from the origin, as
				// CFFInk's does, does not keep to the unit.
				worstHere = math.Max(worstHere, math.Abs(x.pts[k]-y.pts[k])-1e-6*math.Abs(y.pts[k]))
			}
		}
		best = math.Min(best, worstHere)
	}
	return best <= tol, best
}

// outlineCase is one case of the expectation file.
type outlineCase struct {
	name, path, sha, loc string
	hb, ft               map[int][]outlineContour
	// ftNone are the glyphs fontTools could not read.
	ftNone map[int]bool
}

func readOutlineCases(t *testing.T) []*outlineCase {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "outline.expected.txt")
	refuseUnpinnedOracle(t, path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []*outlineCase
	var cur *outlineCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		kind, rest, _ := strings.Cut(line, " ")
		if kind == "case" {
			p := strings.SplitN(rest, " ", 4)
			if len(p) != 4 {
				t.Fatalf("a case line of %d fields: %q", len(p), line)
			}
			cur = &outlineCase{name: p[0], path: p[1], sha: p[2], loc: p[3],
				hb: map[int][]outlineContour{}, ft: map[int][]outlineContour{}, ftNone: map[int]bool{}}
			cases = append(cases, cur)
			continue
		}
		gidText, body, _ := strings.Cut(rest, " ")
		gid, err := strconv.Atoi(gidText)
		if err != nil || cur == nil {
			t.Fatalf("a glyph line that is not one: %q", line)
		}
		if body == "none" {
			cur.ftNone[gid] = true
			continue
		}
		var wire [][2]json.RawMessage
		if err := json.Unmarshal([]byte(body), &wire); err != nil {
			t.Fatalf("%s glyph %d: %v", cur.name, gid, err)
		}
		contours := make([]outlineContour, 0, len(wire))
		for _, c := range wire {
			var oc outlineContour
			var edges [][]json.RawMessage
			if err := json.Unmarshal(c[0], &oc.start); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(c[1], &edges); err != nil {
				t.Fatal(err)
			}
			for _, e := range edges {
				var op string
				if err := json.Unmarshal(e[0], &op); err != nil {
					t.Fatal(err)
				}
				edge := outlineEdge{op: op[0]}
				for _, v := range e[1:] {
					var x float64
					if err := json.Unmarshal(v, &x); err != nil {
						t.Fatal(err)
					}
					edge.pts = append(edge.pts, x)
				}
				oc.edges = append(oc.edges, edge)
			}
			contours = append(contours, oc)
		}
		switch kind {
		case "hb":
			cur.hb[gid] = contours
		case "ft":
			cur.ft[gid] = contours
		default:
			t.Fatalf("a glyph line of kind %q", kind)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// outlineFont reads a case's font program, from the tree or from the corpus
// that has it, and says where it is not there.
func outlineFont(t *testing.T, c *outlineCase) []byte {
	t.Helper()
	dir, name := filepath.Split(c.path)
	var data []byte
	switch filepath.Clean(dir) {
	case "testdata/harfbuzz/fonts":
		data = harfbuzzFont(t, name)
	case "testdata/fonts-noto":
		data = fonttest.NotoFile(t, name)
	case "testdata/cff-fonts":
		data = fonttest.CFFFile(t, name)
	default:
		t.Fatalf("%s: the expectations name a font in %s, which the test does not know how to find", c.name, dir)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != c.sha {
		t.Fatalf("%s: %s is not the font the expectations were drawn from (sha256 %s, want %s)",
			c.name, c.path, got, c.sha)
	}
	return data
}

// outlineFace loads a case's face: at its default, or cut at its location.
func outlineFace(t *testing.T, c *outlineCase, data []byte) *Face {
	t.Helper()
	var loc map[string]float64
	if err := json.Unmarshal([]byte(c.loc), &loc); err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	var f *Face
	var err error
	if len(loc) == 0 {
		f, err = Load(data)
	} else {
		f, err = LoadInstance(data, loc)
	}
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	return f
}

func TestGlyphOutlineIsWhatHarfBuzzAndFontToolsDraw(t *testing.T) {
	cases := readOutlineCases(t)
	if len(cases) < 15 {
		t.Fatalf("%d cases in the expectations, want at least 15", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := outlineFont(t, c)
			f := outlineFace(t, c, data)
			tol := outlineTolerance(c.loc)
			snap := 1e-4
			if c.loc != "{}" {
				snap = tol
			}
			glyphs, worst, contours := 0, 0.0, 0
			for _, oracle := range []struct {
				name string
				of   map[int][]outlineContour
			}{{"HarfBuzz", c.hb}, {"fontTools", c.ft}} {
				for gid, want := range oracle.of {
					segs, err := outlineOf(f, gid)
					if f.IsCFF() {
						// HarfBuzz draws what a charstring drew before it failed;
						// this refuses the glyph, as it has no ink for it.
						if _, ink := f.glyphExtents(gid); !ink {
							if err == nil {
								t.Errorf("glyph %d has no ink, and GlyphOutline drew %d segments of it", gid, len(segs))
							}
							continue
						}
					}
					if err != nil {
						t.Errorf("%s: glyph %d: %v", oracle.name, gid, err)
						continue
					}
					got := contoursOf(segs)
					glyphs++
					if len(got) != len(want) {
						t.Errorf("%s: glyph %d has %d contours, and %s draws %d", c.name, gid, len(got), oracle.name, len(want))
						continue
					}
					for i := range got {
						ok, w := sameCycle(closed(got[i], snap), closed(want[i], snap), tol)
						if !math.IsInf(w, 1) {
							worst = math.Max(worst, w)
						}
						contours++
						if !ok {
							t.Errorf("%s: glyph %d contour %d is not the %s's within %g:\n got  %v\n want %v",
								c.name, gid, i, oracle.name, tol, got[i], want[i])
						}
					}
				}
			}
			if glyphs == 0 {
				t.Errorf("%s: no glyph was compared", c.name)
			}
			t.Logf("%s: %d glyphs, %d contours compared, the widest coordinate %.4f from the oracle's (allowed %g)",
				c.name, glyphs, contours, worst, tol)
		})
	}
}

// The expectation file has to say something about each kind of outline, or a
// face that stopped being compared would go unnoticed.
func TestTheOutlineExpectationsCoverEachFormat(t *testing.T) {
	cases := readOutlineCases(t)
	want := map[string]bool{
		"CFFInk": false, "CFF2Blend": false, "CFF2Blend-mid": false, "PointMatch": false,
		"VerticalComposites": false, "VarComposite": false, "VarComposite-bold": false,
		"VariedAxes-bold-narrow": false, "NotoSansArabic": false, "NotoSansArabic-black": false,
	}
	curves := map[byte]int{}
	for _, c := range cases {
		if _, ok := want[c.name]; ok {
			want[c.name] = len(c.hb) > 0
		}
		for _, cs := range c.hb {
			for _, ct := range cs {
				for _, e := range ct.edges {
					curves[e.op]++
				}
			}
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("the expectations hold nothing for %s", name)
		}
	}
	for _, op := range []byte{'L', 'Q', 'C'} {
		if curves[op] == 0 {
			t.Errorf("no %c edge is in the expectations", op)
		}
	}
}

// controlBox is the box of every point a segment names, a MoveTo that nothing
// follows excepted: what HarfBuzz measures a CFF glyph by, and the box of a
// TrueType glyph's points.
func controlBox(segs []Segment) (b [4]float64, any bool) {
	b = [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	var move *Point
	add := func(p Point) {
		b[0], b[1] = math.Min(b[0], p.X), math.Min(b[1], p.Y)
		b[2], b[3] = math.Max(b[2], p.X), math.Max(b[3], p.Y)
		any = true
	}
	for _, s := range segs {
		switch s.Op {
		case MoveTo:
			p := s.Pts[0]
			move = &p
		case LineTo, QuadTo, CubicTo:
			if move != nil {
				add(*move)
				move = nil
			}
			for _, p := range s.Pts[:map[SegmentOp]int{LineTo: 1, QuadTo: 2, CubicTo: 3}[s.Op]] {
				add(p)
			}
		}
	}
	return b, any
}

// cffInkSeacMerge are the glyphs of CFFInk.otf drawn by a seac of which one
// glyph is only a line (hyphen.acute, acute.hyphen and line.seac). HarfBuzz's
// box for a seac merges the two glyphs' boxes and replaces one that has no area
// rather than joining it (bounds_t::merge, which cffink.go reproduces), where
// drawing the seac draws both; so HarfBuzz's own extents for these are not the
// box of its own outline of them, and the outline, which is HarfBuzz's
// drawing, is held to that in the oracle test.
var cffInkSeacMerge = map[int]bool{102: true, 103: true, 109: true}

// TestTheOutlineIsTheBoxTheFaceMeasures holds the outline to GlyphExtents over
// every glyph of a face: the box of the points drawn is the ink the face says
// the glyph has, since both are the one walk. A glyf glyph's is the header's
// box, which a font states and a CFF glyph's is measured, so the two are held
// to it differently.
func TestTheOutlineIsTheBoxTheFaceMeasures(t *testing.T) {
	type source struct {
		name string
		data func(t *testing.T) []byte
		loc  map[string]float64
	}
	sources := []source{
		{"CFFInk.otf", func(t *testing.T) []byte { return harfbuzzFont(t, "CFFInk.otf") }, nil},
		{"NotoSansArabic.ttf", func(t *testing.T) []byte { return harfbuzzFont(t, "NotoSansArabic.ttf") }, nil},
		{"NotoSansArabic.ttf@900", func(t *testing.T) []byte { return harfbuzzFont(t, "NotoSansArabic.ttf") }, map[string]float64{"wght": 900}},
		{"NotoSansKhmer.ttf", func(t *testing.T) []byte { return harfbuzzFont(t, "NotoSansKhmer.ttf") }, nil},
		{"NotoSans-Regular.ttf", func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSans-Regular.ttf") }, nil},
		{"NotoSansJP-VF.ttf", func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSansJP-VF.ttf") }, nil},
		{"Unifont-Regular.otf", func(t *testing.T) []byte { return fonttest.NotoFile(t, "Unifont-Regular.otf") }, nil},
		{"SourceSans3-Regular.otf", func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") }, nil},
		{"SourceSerif4Variable-Roman.otf", func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSerif4Variable-Roman.otf") }, nil},
		{"SourceSerif4Variable-Roman.otf@700", func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSerif4Variable-Roman.otf") }, map[string]float64{"wght": 700}},
	}
	for _, s := range sources {
		t.Run(s.name, func(t *testing.T) {
			data := s.data(t)
			var f *Face
			var err error
			if s.loc == nil {
				f, err = Load(data)
			} else {
				f, err = LoadInstance(data, s.loc)
			}
			if err != nil {
				t.Fatal(err)
			}
			drawn, empty, refused, wrong := 0, 0, 0, 0
			for gid := 0; gid < f.NumGlyphs(); gid++ {
				e, ink := f.outlineExtents(gid)
				segs, err := outlineOf(f, gid)
				if err != nil {
					// The glyphs the face measures no ink for are the ones it
					// cannot draw, and no others.
					if ink {
						t.Errorf("glyph %d has ink %+v and GlyphOutline says: %v", gid, e, err)
					}
					refused++
					continue
				}
				if !ink {
					t.Errorf("glyph %d has no ink, and GlyphOutline drew %d segments of it", gid, len(segs))
					continue
				}
				b, any := controlBox(segs)
				if !any {
					empty++
					if e.width != 0 || e.height != 0 {
						t.Errorf("glyph %d is drawn as nothing and measured as %+v", gid, e)
					}
					continue
				}
				if s.name == "CFFInk.otf" && cffInkSeacMerge[gid] {
					continue
				}
				drawn++
				var got extents
				if f.IsCFF() {
					box := newCFFBounds()
					box.update(b[0], b[1])
					box.update(b[2], b[3])
					got = box.extents()
				} else {
					r := func(v float64) int { return int(math.Floor(v + 0.5)) }
					got = extents{xBearing: r(b[0]), yBearing: r(b[3]), width: r(b[2]) - r(b[0]), height: r(b[1]) - r(b[3])}
					// The header states the left side bearing hmtx does, and a
					// glyph is drawn at it.
					if math.Abs(float64(e.xBearing-got.xBearing)) <= 1 {
						got.xBearing = e.xBearing
					}
				}
				// A header states a box in whole units, and a composite's
				// components are placed in single precision: a unit is the
				// room for either.
				d := func(a, b int) bool { return a-b > 1 || b-a > 1 }
				if d(got.xBearing, e.xBearing) || d(got.yBearing, e.yBearing) || d(got.width, e.width) || d(got.height, e.height) {
					wrong++
					if wrong <= 5 {
						t.Errorf("glyph %d is drawn in %+v and measured as %+v", gid, got, e)
					}
				}
			}
			t.Logf("%s: %d glyphs drawn, %d empty, %d refused, %d off by more than a unit", s.name, drawn, empty, refused, wrong)
			if wrong > 0 {
				t.Errorf("%d of %d glyphs are drawn in a box other than the one measured", wrong, drawn)
			}
		})
	}
}

// A seac draws the base and the accent, the accent moved by its offset.
func TestGlyphOutlineDrawsASeacsTwoGlyphs(t *testing.T) {
	const endchar, rmoveto, hlineto, vlineto = 14, 21, 6, 7
	cs := func(parts ...[]byte) []byte {
		var out []byte
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	num := cffNumber
	base := cs(num(0), num(0), []byte{rmoveto}, num(200), []byte{hlineto}, num(200), []byte{vlineto}, []byte{endchar})
	accent := cs(num(50), num(0), []byte{rmoveto}, num(100), []byte{hlineto}, num(100), []byte{vlineto}, []byte{endchar})
	seac := cs(num(50), num(300), num(standardCode(t, "A")), num(standardCode(t, "acute")), []byte{endchar})
	sid := func(name string) int {
		v, ok := font.CFFStandardSID(name)
		if !ok {
			t.Fatalf("%q is not a predefined CFF string", name)
		}
		return v
	}
	cff := fonttest.CFF(fonttest.CFFOptions{
		Glyphs:      4,
		CharsetSIDs: []int{sid("A"), sid("acute"), sid("Aacute")},
		Charstrings: [][]byte{{endchar}, base, accent, seac},
	})
	f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'A', Advance: 500, HasShape: true},
		{Rune: 0x00B4, Advance: 0, HasShape: true},
		{Rune: 0x00C1, Advance: 500, HasShape: true},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	pt := func(x, y float64) Point { return Point{x, y} }
	want := []Segment{
		{Op: MoveTo, Pts: [3]Point{pt(0, 0)}}, {Op: LineTo, Pts: [3]Point{pt(200, 0)}}, {Op: LineTo, Pts: [3]Point{pt(200, 200)}},
		// The accent: its own moveto, and then the offset the seac states.
		{Op: MoveTo, Pts: [3]Point{pt(100, 300)}}, {Op: LineTo, Pts: [3]Point{pt(200, 300)}}, {Op: LineTo, Pts: [3]Point{pt(200, 400)}},
	}
	got, err := outlineOf(f, 3)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Aacute is drawn as\n %v\nwant\n %v", got, want)
	}
	// And the base alone is what it is without the accent.
	if got, err = outlineOf(f, 1); err != nil || len(got) != 3 {
		t.Errorf("A is %v (%v), want its three segments", got, err)
	}
}

// Every way of not being able to draw a glyph is an error, and none is a panic.
func TestGlyphOutlineRefusesWhatItCannotDraw(t *testing.T) {
	faces := map[string]*Face{}
	load := func(name string, data []byte) *Face {
		f, err := Load(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		faces[name] = f
		return f
	}
	glyf := load("glyf", harfbuzzFont(t, "VerticalComposites.ttf"))
	load("cff", harfbuzzFont(t, "CFFInk.otf"))
	load("cff2", harfbuzzFont(t, "CFF2Blend.otf"))
	load("varc", harfbuzzFont(t, "VarComposite.ttf"))
	for name, f := range faces {
		for _, gid := range []int{-1, f.NumGlyphs(), f.NumGlyphs() + 1, math.MaxInt32, math.MinInt32} {
			if segs, err := outlineOf(f, gid); err == nil {
				t.Errorf("%s: glyph %d, which the face does not have, is drawn as %d segments", name, gid, len(segs))
			}
		}
	}
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outlineOf(std, 3); !errors.Is(err, ErrNoOutline) {
		t.Errorf("a standard face's glyph: %v, want ErrNoOutline", err)
	}

	// A VARC glyph built on CFF glyphs cannot be drawn, and says why.
	vc, err := Load(harfbuzzFont(t, "VarCompositeCFF.otf"))
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for gid := 0; gid < vc.NumGlyphs(); gid++ {
		if _, err := outlineOf(vc, gid); err != nil && vc.varc.t.coverageIndex(gid) >= 0 {
			if !strings.Contains(err.Error(), "CFF") {
				t.Errorf("glyph %d of a VARC face on CFF leaves is refused without saying so: %v", gid, err)
			}
			refused++
		}
	}
	if refused == 0 {
		t.Error("no glyph of VarCompositeCFF.otf is refused, and its composites have CFF leaves")
	}

	// A glyph with nothing to draw is drawn as nothing, and is not an error.
	space, _ := glyf.GlyphID(' ')
	if segs, err := outlineOf(glyf, space); err != nil || len(segs) != 0 {
		t.Errorf("a space is %d segments (%v), want none", len(segs), err)
	}

	// Stopping is not an error, and stops.
	n := 0
	for gid := 0; gid < glyf.NumGlyphs(); gid++ {
		n = 0
		if err := glyf.GlyphOutline(gid, func(Segment) bool { n++; return false }); err != nil || n > 1 {
			t.Fatalf("glyph %d: yield asked to stop after one, and got %d segments, error %v", gid, n, err)
		}
	}
}

// A glyph that is drawn as a bitmap has no outline to hand out, and says so.
//
// The fixtures' glyphs all carry an outline as well as an image, so each is
// emptied first: loca is zeroed on a copy of the program, which makes every
// glyph's glyf entry empty as a font with bitmaps and no outlines has them.
func TestGlyphOutlineRefusesABitmapGlyph(t *testing.T) {
	for _, name := range []string{"SbixInk.ttf", "BitmapInk.ttf"} {
		data := append([]byte(nil), harfbuzzFont(t, name)...)
		whole, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		// With their outlines, none of the glyphs is refused.
		for gid := 0; gid < whole.NumGlyphs(); gid++ {
			if _, err := outlineOf(whole, gid); err != nil {
				t.Errorf("%s glyph %d, which has an outline: %v", name, gid, err)
			}
		}
		loca := font.SFNTTables(data)["loca"]
		clear(loca)
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		refused, empty := 0, 0
		for gid := 0; gid < f.NumGlyphs(); gid++ {
			_, err := outlineOf(f, gid)
			_, image := f.glyphExtents(gid)
			switch {
			case errors.Is(err, ErrNoOutline):
				refused++
			case err != nil:
				t.Errorf("%s glyph %d: %v", name, gid, err)
			default:
				empty++
			}
			if errors.Is(err, ErrNoOutline) && !image {
				t.Errorf("%s glyph %d is refused as a bitmap, and the face measures no image for it", name, gid)
			}
		}
		if refused == 0 {
			t.Errorf("%s: no glyph is refused as a bitmap, and its glyphs are bitmaps", name)
		}
		t.Logf("%s: %d glyphs refused as bitmaps, %d drawn as nothing", name, refused, empty)
	}
}

// Drawing is bounded by a budget of the face's own, which does not touch the
// budget the face measures under, and which a glyph drawn once is not charged
// against again.
func TestGlyphOutlineWorkIsBounded(t *testing.T) {
	for _, name := range []string{"CFFInk.otf", "NotoSansArabic.ttf"} {
		f, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		gid := 0
		for ; gid < f.NumGlyphs(); gid++ {
			if segs, err := outlineOf(f, gid); err == nil && len(segs) > 2 {
				break
			}
		}
		if gid == f.NumGlyphs() {
			t.Fatalf("%s: no glyph to draw", name)
		}
		// The same glyph, drawn far more times than the work allows a
		// glyph drawn each time: it is drawn once.
		spent := f.outlines.budget.Spent()
		for i := 0; i < 20000; i++ {
			if _, err := outlineOf(f, gid); err != nil {
				t.Fatalf("%s: draw %d of one glyph: %v", name, i, err)
			}
		}
		if got := f.outlines.budget.Spent(); got != spent {
			t.Errorf("%s: drawing a glyph again cost %d units", name, got-spent)
		}
		if f.ink != nil {
			f.ink.load()
			if f.ink.budget.Spent() != 0 && f.ink.budget.Exhausted() {
				t.Errorf("%s: drawing spent the budget glyphs are measured under", name)
			}
		}

		// A face with nothing left to spend refuses the glyphs it has not drawn,
		// and yields nothing of them.
		g, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		g.outlines.budget = font.NewBudget(1)
		refused := 0
		for gid := 0; gid < g.NumGlyphs(); gid++ {
			n := 0
			err := g.GlyphOutline(gid, func(Segment) bool { n++; return true })
			if err != nil {
				refused++
				if n != 0 {
					t.Errorf("%s: glyph %d yielded %d segments and then an error", name, gid, n)
				}
			}
		}
		if refused == 0 {
			t.Errorf("%s: a face with one unit of work drew every glyph", name)
		}
	}
}

// A face is drawn from by many goroutines, as a document's clones are; a seac
// reads the charset lazily, so this is what the race detector looks at.
func TestGlyphOutlineIsSafeToShare(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "CFFInk.otf"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := f.Clone()
			for gid := 0; gid < c.NumGlyphs(); gid++ {
				_, _ = outlineOf(c, gid)
				_, _, _, _, _ = c.GlyphExtents(gid)
			}
		}()
	}
	wg.Wait()
}

// The contours of a glyf glyph, taken apart as the format says, for the cases
// a font has few of: a contour that starts off the curve, one with no on-curve
// point at all, and one of a single point.
func TestQuadContoursAreTheFormatsQuadratics(t *testing.T) {
	p := func(x, y float64) Point { return Point{x, y} }
	for _, c := range []struct {
		name string
		pts  []Point
		on   []bool
		want []Segment
	}{
		{"on-curve start", []Point{p(0, 0), p(10, 0), p(10, 10)}, []bool{true, true, true},
			[]Segment{{Op: MoveTo, Pts: [3]Point{p(0, 0)}}, {Op: LineTo, Pts: [3]Point{p(10, 0)}}, {Op: LineTo, Pts: [3]Point{p(10, 10)}}}},
		{"an off-curve point between two on", []Point{p(0, 0), p(10, 0), p(10, 10)}, []bool{true, false, true},
			[]Segment{{Op: MoveTo, Pts: [3]Point{p(0, 0)}}, {Op: QuadTo, Pts: [3]Point{p(10, 0), p(10, 10)}}}},
		{"two off-curve points imply the one between", []Point{p(0, 0), p(10, 0), p(10, 10), p(0, 10)}, []bool{true, false, false, true},
			[]Segment{{Op: MoveTo, Pts: [3]Point{p(0, 0)}}, {Op: QuadTo, Pts: [3]Point{p(10, 0), p(10, 5)}},
				{Op: QuadTo, Pts: [3]Point{p(10, 10), p(0, 10)}}}},
		{"an off-curve start begins at the last point", []Point{p(10, 0), p(10, 10), p(0, 0)}, []bool{false, true, true},
			[]Segment{{Op: MoveTo, Pts: [3]Point{p(0, 0)}}, {Op: QuadTo, Pts: [3]Point{p(10, 0), p(10, 10)}}}},
		{"no on-curve point begins at a midpoint", []Point{p(0, 0), p(10, 0), p(10, 10), p(0, 10)}, []bool{false, false, false, false},
			[]Segment{{Op: MoveTo, Pts: [3]Point{p(0, 5)}}, {Op: QuadTo, Pts: [3]Point{p(0, 0), p(5, 0)}},
				{Op: QuadTo, Pts: [3]Point{p(10, 0), p(10, 5)}}, {Op: QuadTo, Pts: [3]Point{p(10, 10), p(5, 10)}},
				{Op: QuadTo, Pts: [3]Point{p(0, 10), p(0, 5)}}}},
		{"one point draws nothing", []Point{p(3, 3)}, []bool{true}, nil},
	} {
		got := quadContours(c.pts, c.on, []int{len(c.pts) - 1})
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
		}
	}
}
