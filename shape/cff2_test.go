package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// CFF2, held to HarfBuzz and to fontTools.
//
// testdata/harfbuzz/cff2.py draws a sample of the glyphs of three CFF2 variable
// fonts (CFF_FONTS) at several locations each, as HarfBuzz draws them — their
// extents, their outlines point for point, their advances — and instances each
// font there with fontTools, recording the instance's advances, its outlines,
// its bounds and HarfBuzz's extents of it. The answers are checked in as
// cff2.expected.txt.

// cff2Faces reads each face the expectations were generated from.
var cff2Faces = map[string]func(t *testing.T) []byte{
	"CFF2Blend.otf":                  func(t *testing.T) []byte { return harfbuzzFont(t, "CFF2Blend.otf") },
	"SourceSans3VF-Upright.otf":      func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSans3VF-Upright.otf") },
	"SourceSerif4Variable-Roman.otf": func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSerif4Variable-Roman.otf") },
	"NotoSansJP-VF.otf":              func(t *testing.T) []byte { return fonttest.CFFFile(t, "NotoSansJP-VF.otf") },
}

// cff2Golden is one face's expectations.
type cff2Golden struct {
	name, sum string
	locs      []*cff2Loc
}

// cff2Loc is one location's: what HarfBuzz draws of the variable font there,
// and what fontTools cut.
type cff2Loc struct {
	label  string
	want   map[string]float64
	coords []int
	// HarfBuzz, of the variable font.
	extents  map[int]*extents
	advances map[int][]int // h advance, and v advance and origin where the face has them
	paths    map[int][]string
	// fontTools' instance.
	instAdvances map[int][]int
	instExtents  map[int]*extents
	instBounds   map[int][]float64 // nil for none
	instPaths    map[int][]string
	instMetrics  map[string][]string
	// For the fixture: each glyph's hints in the instance — its horizontal
	// and vertical stems as edges, and its masks — and each Font DICT's
	// Private DICT numbers, by operator name.
	instHints   map[int][3][]string
	instPrivate map[int]map[string][]float64
}

func readCFF2Golden(t *testing.T) []*cff2Golden {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "cff2.expected.txt")
	refuseUnpinnedOracle(t, path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (make hbcff2)", err)
	}
	defer f.Close()
	ints := func(ws []string) []int {
		out := make([]int, len(ws))
		for i, w := range ws {
			v, err := strconv.Atoi(w)
			if err != nil {
				t.Fatalf("%s: %q is not an integer", path, w)
			}
			out[i] = v
		}
		return out
	}
	ext := func(ws []string) *extents {
		if len(ws) == 1 && ws[0] == "none" {
			return nil
		}
		v := ints(ws)
		return &extents{xBearing: v[0], yBearing: v[1], width: v[2], height: v[3]}
	}
	var faces []*cff2Golden
	var loc *cff2Loc
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<22), 1<<22)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		w := strings.Fields(line)
		switch w[0] {
		case "face":
			faces = append(faces, &cff2Golden{name: w[1], sum: w[2]})
			continue
		case "loc":
			loc = &cff2Loc{label: w[1], want: map[string]float64{},
				extents: map[int]*extents{}, advances: map[int][]int{}, paths: map[int][]string{},
				instAdvances: map[int][]int{}, instExtents: map[int]*extents{},
				instBounds: map[int][]float64{}, instPaths: map[int][]string{}, instMetrics: map[string][]string{},
				instHints: map[int][3][]string{}, instPrivate: map[int]map[string][]float64{}}
			if w[1] != "default" {
				for _, kv := range strings.Split(w[1], ",") {
					k, v, _ := strings.Cut(kv, "=")
					x, err := strconv.ParseFloat(v, 64)
					if err != nil {
						t.Fatalf("%s: location %q", path, w[1])
					}
					loc.want[k] = x
				}
			}
			fc := faces[len(faces)-1]
			fc.locs = append(fc.locs, loc)
			continue
		case "coords":
			loc.coords = ints(w[1:])
			continue
		case "IV":
			fd, err := strconv.Atoi(w[1])
			if err != nil {
				t.Fatalf("%s: %q", path, line)
			}
			if loc.instPrivate[fd] == nil {
				loc.instPrivate[fd] = map[string][]float64{}
			}
			for _, x := range w[3:] {
				v, err := strconv.ParseFloat(x, 64)
				if err != nil {
					t.Fatalf("%s: %q", path, line)
				}
				loc.instPrivate[fd][w[2]] = append(loc.instPrivate[fd][w[2]], v)
			}
			continue
		case "IM":
			for i := 1; i+1 < len(w); i += 2 {
				loc.instMetrics[w[i]] = []string{w[i+1]}
			}
			// strikeout carries two numbers.
			for i := 1; i < len(w); i++ {
				if w[i] == "strikeout" {
					loc.instMetrics["strikeout"] = w[i+1 : i+3]
				}
			}
			continue
		}
		gid, err := strconv.Atoi(w[1])
		if err != nil {
			t.Fatalf("%s: a line this does not read: %q", path, line)
		}
		switch w[0] {
		case "E":
			loc.extents[gid] = ext(w[2:])
		case "A":
			loc.advances[gid] = ints(w[2:])
		case "P":
			loc.paths[gid] = splitPath(w[2:])
		case "I":
			loc.instAdvances[gid] = ints(w[2:])
		case "IE":
			loc.instExtents[gid] = ext(w[2:])
		case "IB":
			if w[2] != "none" {
				b := make([]float64, 4)
				for i := range b {
					if b[i], err = strconv.ParseFloat(w[2+i], 64); err != nil {
						t.Fatalf("%s: %q", path, line)
					}
				}
				loc.instBounds[gid] = b
			} else {
				loc.instBounds[gid] = nil
			}
		case "IP":
			loc.instPaths[gid] = splitPath(w[2:])
		case "IH":
			var h [3][]string
			part := -1
			for _, x := range w[2:] {
				switch x {
				case "h":
					part = 0
				case "v":
					part = 1
				case "m":
					part = 2
				default:
					h[part] = append(h[part], x)
				}
			}
			loc.instHints[gid] = h
		default:
			t.Fatalf("%s: a line this does not read: %q", path, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(cff2Faces) {
		t.Fatalf("%s holds %d faces and the test reads %d", path, len(faces), len(cff2Faces))
	}
	return faces
}

// splitPath groups a recorded path's tokens into one string per operation —
// "M x y", "L x y", "C x1 y1 x2 y2 x3 y3" or "Z".
func splitPath(w []string) []string {
	var out []string
	for i := 0; i < len(w); {
		n := map[string]int{"M": 2, "L": 2, "C": 6, "Z": 0}[w[i]]
		out = append(out, strings.Join(w[i:i+1+n], " "))
		i += 1 + n
	}
	return out
}

// cff2FaceData loads a golden face's bytes and checks they are the ones the
// expectations were generated from.
func cff2FaceData(t *testing.T, g *cff2Golden) []byte {
	t.Helper()
	load, ok := cff2Faces[g.name]
	if !ok {
		t.Fatalf("the expectations name %s, which the test does not read", g.name)
	}
	data := load(t)
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != g.sum {
		t.Fatalf("%s is not the face the expectations were generated from", g.name)
	}
	return data
}

// readCFF2Of reads a face's CFF2 table.
func readCFF2Of(t *testing.T, data []byte) (*cff2Font, map[string][]byte) {
	t.Helper()
	tables := font.SFNTTables(data)
	numGlyphs := font.Be16(tables["maxp"], 4)
	f, err := readCFF2(tables["CFF2"], numGlyphs, font.NewBudget(maxFontWork))
	if err != nil {
		t.Fatalf("reading the CFF2 table: %v", err)
	}
	return f, tables
}

// hbDrawn is a recorded outline as HarfBuzz's draw functions hand it over:
// a move only once something is drawn from it, a path that starts without one
// starting at the origin, each subpath closed with a line back to where it
// started when it did not end there, and a close after each. The coordinates
// are floats, as HarfBuzz passes them.
func hbDrawn(segs []t2Seg) []string {
	var out []string
	f := func(v float64) string { return strconv.FormatFloat(float64(float32(v)), 'g', -1, 32) }
	var startX, startY, curX, curY float64
	open, pending := false, true
	var pendX, pendY float64
	closePath := func() {
		if open {
			if startX != curX || startY != curY {
				out = append(out, "L "+f(startX)+" "+f(startY))
			}
			out = append(out, "Z")
		}
		open = false
	}
	begin := func() {
		if !open {
			out = append(out, "M "+f(pendX)+" "+f(pendY))
			startX, startY, curX, curY = pendX, pendY, pendX, pendY
			open = true
		}
	}
	for _, s := range segs {
		p := s.pts
		switch s.op {
		case 'M':
			closePath()
			pendX, pendY, pending = p[0], p[1], true
		case 'L':
			begin()
			out = append(out, "L "+f(p[0])+" "+f(p[1]))
			curX, curY = p[0], p[1]
		case 'C':
			begin()
			out = append(out, "C "+f(p[0])+" "+f(p[1])+" "+f(p[2])+" "+f(p[3])+" "+f(p[4])+" "+f(p[5]))
			curX, curY = p[4], p[5]
		}
	}
	_ = pending
	closePath()
	return out
}

// normalizePath rewrites a golden path's numbers as float32s spelled the way
// hbDrawn spells them, so that the two compare as strings.
func normalizePath(t *testing.T, ops []string) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		w := strings.Fields(op)
		for j := 1; j < len(w); j++ {
			v, err := strconv.ParseFloat(w[j], 64)
			if err != nil {
				t.Fatalf("a path number %q", w[j])
			}
			w[j] = strconv.FormatFloat(float64(float32(v)), 'g', -1, 32)
		}
		out[i] = strings.Join(w, " ")
	}
	return out
}

func extentsString(e *extents, ok bool) string {
	if !ok || e == nil {
		return "none"
	}
	return fmt.Sprintf("%d %d %d %d", e.xBearing, e.yBearing, e.width, e.height)
}

// TestCFF2DrawsAsHarfBuzzDoes runs each sampled glyph at each location as
// HarfBuzz resolves the blends there, and holds its extents and its outline,
// point for point, to HarfBuzz's.
func TestCFF2DrawsAsHarfBuzzDoes(t *testing.T) {
	for _, g := range readCFF2Golden(t) {
		t.Run(g.name, func(t *testing.T) {
			data := cff2FaceData(t, g)
			f, tables := readCFF2Of(t, data)
			for _, loc := range g.locs {
				o := f.outlines(newHarfBuzzBlend(f, loc.coords))
				budget := font.NewBudget(cffInkWork(len(tables["CFF2"])))
				bad := 0
				for gid, want := range loc.extents {
					r := t2Run{o: o, budget: budget}
					b, ok := r.bounds(gid, false)
					var got *extents
					if ok {
						e := b.extents()
						got = &e
					}
					if extentsString(got, ok) != extentsString(want, want != nil) {
						if bad++; bad <= 5 {
							t.Errorf("%s glyph %d: extents %s, HarfBuzz %s", loc.label, gid,
								extentsString(got, ok), extentsString(want, want != nil))
						}
					}
				}
				for gid, want := range loc.paths {
					var segs []t2Seg
					r := t2Run{o: o, budget: budget, draw: true, path: func(s t2Seg) { segs = append(segs, s) }}
					r.bounds(gid, false)
					got, w := hbDrawn(segs), normalizePath(t, want)
					if strings.Join(got, "|") != strings.Join(w, "|") {
						if bad++; bad <= 5 {
							t.Errorf("%s glyph %d draws\n  %v\nHarfBuzz\n  %v", loc.label, gid, got, w)
						}
					}
				}
				if bad > 0 {
					t.Errorf("%s: %d glyphs differ from HarfBuzz", loc.label, bad)
				}
			}
		})
	}
}

// fontToolsPath is a golden fontTools outline as the CFF interpreter records
// one: its moves, lines and curves, without the closes its pen adds.
func fontToolsPath(ops []string) []string {
	var out []string
	for _, op := range ops {
		if op != "Z" {
			out = append(out, op)
		}
	}
	return out
}

// recordedPath spells recorded segments as a golden path spells them.
func recordedPath(segs []t2Seg) []string {
	f := func(v float64) string {
		if v == math.Trunc(v) {
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	out := make([]string, len(segs))
	for i, s := range segs {
		n := 2
		if s.op == 'C' {
			n = 6
		}
		w := []string{string(s.op)}
		for j := 0; j < n; j++ {
			w = append(w, f(s.pts[j]))
		}
		out[i] = strings.Join(w, " ")
	}
	return out
}

// convertAt writes a golden face at a location as the CFF font an instance is
// cut as, with the advances hmtx states: the outlines are what is compared, and
// a width is only a number in front of them.
func convertAt(t *testing.T, f *cff2Font, tables map[string][]byte, want map[string]float64) (*cff2Converted, []float64) {
	t.Helper()
	coords, tags, err := cff2Location(tables["fvar"], tables["avar"], want)
	if err != nil {
		t.Fatal(err)
	}
	n := len(f.charStrings)
	advances, _, err := parseHmtx(tables["hmtx"], font.Be16(tables["hhea"], 34), n)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := cff2ToCFF(f, newFontToolsBlend(f, coords, tags), advances, "Test",
		font.NewBudget(cffConvertWork(len(tables["CFF2"]))))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	return conv, coords
}

// TestACFF2InstanceDrawsWhatFontToolsCuts writes each face as CFF at each
// location and holds the result to fontTools' instance there: every sampled
// glyph's outline point for point, its bounds, and the extents HarfBuzz gives
// the instance's glyph — which is what a face cut here measures as its ink.
func TestACFF2InstanceDrawsWhatFontToolsCuts(t *testing.T) {
	for _, g := range readCFF2Golden(t) {
		t.Run(g.name, func(t *testing.T) {
			data := cff2FaceData(t, g)
			f, tables := readCFF2Of(t, data)
			n := len(f.charStrings)
			for _, loc := range g.locs {
				conv, coords := convertAt(t, f, tables, loc.want)
				for i, c := range coords {
					if int(c*16384) != loc.coords[i] {
						t.Errorf("%s: axis %d normalizes to %v (%d), HarfBuzz's %d",
							loc.label, i, c, int(c*16384), loc.coords[i])
					}
				}
				if len(conv.limits) > 0 {
					t.Errorf("%s: %q", loc.label, conv.limits)
				}
				o, err := readCFFOutlines(conv.cff, n)
				if err != nil {
					t.Fatalf("%s: the CFF written cannot be read: %v", loc.label, err)
				}
				bad := 0
				fail := func(format string, args ...any) {
					if bad++; bad <= 6 {
						t.Errorf(loc.label+": "+format, args...)
					}
				}
				for gid, want := range loc.instPaths {
					segs, ok := cffOutline(o, gid)
					if !ok {
						fail("glyph %d cannot be run", gid)
						continue
					}
					got, w := recordedPath(segs), fontToolsPath(want)
					if strings.Join(got, "|") != strings.Join(w, "|") {
						fail("glyph %d draws\n  %v\nfontTools\n  %v", gid, got, w)
					}
				}
				for gid, want := range loc.instExtents {
					r := t2Run{o: o, budget: font.NewBudget(maxFontWork)}
					b, ok := r.bounds(gid, false)
					var got *extents
					if ok {
						e := b.extents()
						got = &e
					}
					if extentsString(got, ok) != extentsString(want, want != nil) {
						fail("glyph %d: extents %s, HarfBuzz's of fontTools' instance %s",
							gid, extentsString(got, ok), extentsString(want, want != nil))
					}
				}
				for gid, want := range loc.instBounds {
					b := conv.bounds[gid]
					got := []float64(nil)
					if b.set {
						got = []float64{b.xMin, b.yMin, b.xMax, b.yMax}
					}
					if fmt.Sprint(got) != fmt.Sprint(want) {
						fail("glyph %d: bounds %v, fontTools %v", gid, got, want)
					}
				}
				if bad > 0 {
					t.Errorf("%s: %d differences from fontTools", loc.label, bad)
				}
			}
		})
	}
}

// cff2FaceAt is a golden face at a golden location as a caller gets it: from
// Load at the default, and from LoadInstance anywhere else.
func cff2FaceAt(t *testing.T, data []byte, loc *cff2Loc) *Face {
	t.Helper()
	var f *Face
	var err error
	if len(loc.want) == 0 {
		f, err = Load(data)
	} else {
		f, err = LoadInstance(data, loc.want)
	}
	if err != nil {
		t.Fatalf("%s: %v", loc.label, err)
	}
	return f
}

// TestACFF2FaceIsCutWhereHarfBuzzAndFontToolsCutIt loads each golden face at
// each location as a caller does and holds the face to both oracles: its
// advances to HarfBuzz's at the location and to fontTools' instance's, its
// vertical advances and origins to HarfBuzz's, its ink to the extents HarfBuzz
// gives fontTools' instance — the outlines it embeds being that instance's —
// its box to the bounds fontTools measures, and its font-wide numbers to the
// instance's. And it says what it is: a CFF face, CID-keyed in Adobe-Identity-0,
// variable at the default and static where it was cut, with nothing reported
// against it.
func TestACFF2FaceIsCutWhereHarfBuzzAndFontToolsCutIt(t *testing.T) {
	for _, g := range readCFF2Golden(t) {
		t.Run(g.name, func(t *testing.T) {
			data := cff2FaceData(t, g)
			for _, loc := range g.locs {
				f := cff2FaceAt(t, data, loc)
				if !f.IsCFF() || !f.IsCIDKeyed() {
					t.Errorf("%s: IsCFF %v, IsCIDKeyed %v; a CFF2 font is embedded as a CID-keyed CFF",
						loc.label, f.IsCFF(), f.IsCIDKeyed())
				}
				if r, o, s, ok := f.CharacterCollection(); !ok || r != "Adobe" || o != "Identity" || s != 0 {
					t.Errorf("%s: the collection is %s-%s-%d (%v), want Adobe-Identity-0", loc.label, r, o, s, ok)
				}
				if variable := len(loc.want) == 0; f.IsVariable() != variable {
					t.Errorf("%s: IsVariable %v", loc.label, f.IsVariable())
				}
				if lim := f.LayoutLimits(); len(lim) > 0 {
					t.Errorf("%s: limits reported: %q", loc.label, lim)
				}
				tables := font.SFNTTables(f.Program())
				if tables["CFF2"] != nil || tables["CFF "] == nil || tables["fvar"] != nil {
					t.Errorf("%s: the program carries CFF2 %v, CFF %v, fvar %v; it is the static CFF font drawn",
						loc.label, tables["CFF2"] != nil, tables["CFF "] != nil, tables["fvar"] != nil)
				}
				// The widths the program's own charstrings state are the
				// advances, which a PDF's /W is written from.
				prog := font.ParseCFF(tables["CFF "])
				if prog == nil {
					t.Fatalf("%s: the font package cannot read the CFF written", loc.label)
				}
				bad, halves := 0, 0
				fail := func(format string, args ...any) {
					if bad++; bad <= 6 {
						t.Errorf(loc.label+": "+format, args...)
					}
				}
				defer func() { t.Logf("%s: %d advances on a half, set where HarfBuzz sets them", loc.label, halves) }()
				for gid, a := range loc.advances {
					got := f.advanceUnits(gid)
					if got != a[0] {
						fail("glyph %d advances %d, HarfBuzz %d", gid, got, a[0])
					}
					// fontTools rounds an advance's delta half to even, and
					// the face rounds it half up as HarfBuzz does (see
					// instanceCFF2): the two may differ where the delta is a
					// half, and only there, and there the face is HarfBuzz's.
					if ia := loc.instAdvances[gid]; got != ia[0] {
						if a[0] == ia[0] {
							fail("glyph %d advances %d, fontTools' instance %d", gid, got, ia[0])
						}
						halves++
					}
					if w := prog.WidthByCID[gid]; w != f.GlyphAdvance(gid) {
						fail("glyph %d: the charstring's width is %v and the advance %v", gid, w, f.GlyphAdvance(gid))
					}
					if len(a) == 4 {
						adv, ox, oy := f.verticalUnits(gid)
						if adv != -a[1] || ox != a[2] || oy != a[3] {
							fail("glyph %d: vertical advance and origin %d (%d, %d), HarfBuzz %d (%d, %d)",
								gid, adv, ox, oy, -a[1], a[2], a[3])
						}
					}
				}
				var box floatBounds
				for gid, want := range loc.instExtents {
					got, ok := f.glyphExtents(gid)
					if extentsString(&got, ok) != extentsString(want, want != nil) {
						fail("glyph %d: ink %s, HarfBuzz's of fontTools' instance %s",
							gid, extentsString(&got, ok), extentsString(want, want != nil))
					}
				}
				for _, b := range loc.instBounds {
					if b != nil {
						box.add(math.Floor(b[0]), math.Floor(b[1]))
						box.add(math.Ceil(b[2]), math.Ceil(b[3]))
					}
				}
				d := f.Descriptor()
				// The sample's bounds lie inside the font's box; the box is
				// every glyph's, which the sample is not.
				if len(loc.want) > 0 && box.set && (float64(d.BBox[0]) > box.xMin || float64(d.BBox[1]) > box.yMin ||
					float64(d.BBox[2]) < box.xMax || float64(d.BBox[3]) < box.yMax) {
					fail("the box %v does not hold the sample's bounds %v", d.BBox, box)
				}
				// And an instance's box is every glyph's bounds, as the pen the
				// sample was held to fontTools with measures them, rounded
				// out — measured here from the program the face embeds. The
				// default states its own, which is the font's to state.
				if len(loc.want) > 0 {
					o, err := readCFFOutlines(tables["CFF "], f.NumGlyphs())
					if err != nil {
						t.Fatal(err)
					}
					var all floatBounds
					for gid := 0; gid < f.NumGlyphs(); gid++ {
						segs, _ := cffOutline(o, gid)
						events := make([]t2Event, len(segs))
						for i, s := range segs {
							events[i].seg = s
						}
						if b := penBounds(events); b.set {
							all.add(math.Floor(b.xMin), math.Floor(b.yMin))
							all.add(math.Ceil(b.xMax), math.Ceil(b.yMax))
						}
					}
					if want := [4]int{int(all.xMin), int(all.yMin), int(all.xMax), int(all.yMax)}; d.BBox != want {
						fail("the box is %v, and the glyphs' bounds come to %v", d.BBox, want)
					}
				}
				im := loc.instMetrics
				check := func(what string, got int, key string, i int) {
					if w, err := strconv.Atoi(im[key][i]); err != nil || got != w {
						fail("%s is %d, fontTools' instance %s", what, got, im[key][i])
					}
				}
				// At the default the face is the font as it states itself,
				// which Load does not rewrite; an instance is cut.
				if len(loc.want) > 0 {
					check("the weight", d.Weight, "weight", 0)
					check("the x-height", d.XHeight, "xheight", 0)
					if d.Has(MetricCapHeight) {
						check("the cap height", d.CapHeight, "capheight", 0)
					}
				}
				if bad > 0 {
					t.Errorf("%s: %d differences", loc.label, bad)
				}
			}
		})
	}
}

// decodeHints reads a Type 2 charstring that calls no subroutine for its
// hints: its horizontal and vertical stems as the edges each stem operator
// comes to from zero, the stems a first mask declares by its operands counted
// as vertical, and each mask's bytes — the form cff2.py records fontTools'
// instance's hints in. A width in front of the first operator that may carry
// one is left out, as fontTools' reading leaves it.
func decodeHints(t *testing.T, code []byte) [3][]string {
	t.Helper()
	var out [3][]string
	var stack []float64
	first, seenMask := true, false
	stems := 0
	edges := func(part int, args []float64) {
		pos := 0.0
		for _, a := range args {
			pos += a
			out[part] = append(out[part], strconv.FormatFloat(pos, 'f', -1, 64))
		}
		stems += len(args) / 2
	}
	width := func(parity bool) {
		if first && len(stack) > 0 && (len(stack)%2 == 1) != parity {
			stack = stack[1:]
		}
		first = false
	}
	for at := 0; at < len(code); {
		v := int(code[at])
		switch {
		case v == 28:
			stack = append(stack, float64(int16(uint16(code[at+1])<<8|uint16(code[at+2]))))
			at += 3
			continue
		case v == 255:
			stack = append(stack, float64(int32(uint32(code[at+1])<<24|uint32(code[at+2])<<16|
				uint32(code[at+3])<<8|uint32(code[at+4])))/65536)
			at += 5
			continue
		case v >= 32 && v <= 246:
			stack = append(stack, float64(v-139))
			at++
			continue
		case v >= 247 && v <= 250:
			stack = append(stack, float64((v-247)*256+int(code[at+1])+108))
			at += 2
			continue
		case v >= 251 && v <= 254:
			stack = append(stack, float64(-(v-251)*256-int(code[at+1])-108))
			at += 2
			continue
		}
		at++
		switch v {
		case 1, 18:
			width(false)
			edges(0, stack)
		case 3, 23:
			width(false)
			edges(1, stack)
		case 19, 20:
			width(false)
			if !seenMask {
				edges(1, stack)
				seenMask = true
			}
			n := (stems + 7) / 8
			out[2] = append(out[2], hex.EncodeToString(code[at:at+n]))
			at += n
		case 21, 14:
			width(false)
		case 12:
			at++
		}
		stack = stack[:0]
	}
	return out
}

// TestACFF2InstanceKeepsItsHints holds the fixture's hints, written as CFF, to
// fontTools' instance's at each location: the same stems at the same edges,
// the same masks — except where the glyph declares more stems than a CFF
// charstring may, 96. fontTools writes those anyway, and a CFF reader refuses
// the masks; the glyph is written with no hints and its outline unchanged.
func TestACFF2InstanceKeepsItsHints(t *testing.T) {
	for _, g := range readCFF2Golden(t) {
		if g.name != "CFF2Blend.otf" {
			continue
		}
		data := cff2FaceData(t, g)
		f, tables := readCFF2Of(t, data)
		for _, loc := range g.locs {
			conv, _ := convertAt(t, f, tables, loc.want)
			glyphs, err := CharStringsForTest(conv.cff)
			if err != nil {
				t.Fatal(err)
			}
			for gid, want := range loc.instHints {
				got := decodeHints(t, glyphs[gid])
				if len(want[0])/2+len(want[1])/2 > maxCFF2Stems {
					if len(got[0])+len(got[1])+len(got[2]) != 0 {
						t.Errorf("%s glyph %d declares %d stems, past what CFF allows, and keeps hints %v",
							loc.label, gid, len(want[0])/2+len(want[1])/2, got)
					}
					continue
				}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("%s glyph %d hints %v, fontTools' instance %v", loc.label, gid, got, want)
				}
			}
		}
	}
}

// TestACFF2InstanceResolvesItsPrivateDicts holds the fixture's Private DICTs,
// written as CFF, to the numbers fontTools' instance states — read as the
// specification reads a blended delta array rather than as fontTools does.
// A blend in a DICT leaves its values for the operator after it, and
// BlueValues and its kind are delta-encoded, each value from the one before:
// so the values the instance states are the defaults' running sum with each
// value's own delta added in. fontTools adds each value's own delta to its
// absolute default instead, and states a zone whose edges moved apart by the
// delta of the one before it as not having moved. At the default, where no
// delta moves anything, the two readings agree and fontTools' numbers are
// held to exactly.
func TestACFF2InstanceResolvesItsPrivateDicts(t *testing.T) {
	deltaArrays := map[string]bool{"BlueValues": true, "OtherBlues": true, "FamilyBlues": true,
		"FamilyOtherBlues": true, "StemSnapH": true, "StemSnapV": true}
	names := map[int]string{6: "BlueValues", 7: "OtherBlues", 8: "FamilyBlues", 9: "FamilyOtherBlues",
		10: "StdHW", 11: "StdVW", 1212: "StemSnapH", 1213: "StemSnapV"}
	for _, g := range readCFF2Golden(t) {
		if g.name != "CFF2Blend.otf" {
			continue
		}
		data := cff2FaceData(t, g)
		f, tables := readCFF2Of(t, data)
		defaults := g.locs[0].instPrivate
		if g.locs[0].label != "default" || len(defaults) == 0 {
			t.Fatal("the fixture's first location is not its default, or states no Private DICT")
		}
		for _, loc := range g.locs {
			conv, _ := convertAt(t, f, tables, loc.want)
			got := cffPrivateNumbers(t, conv.cff, names, deltaArrays)
			for fd, entries := range loc.instPrivate {
				for key, ft := range entries {
					want := ft
					if deltaArrays[key] {
						def := defaults[fd][key]
						want = make([]float64, len(ft))
						moved := 0.0
						for i := range ft {
							moved += ft[i] - def[i]
							want[i] = def[i] + moved
						}
					}
					if fmt.Sprint(got[fd][key]) != fmt.Sprint(want) {
						t.Errorf("%s Font DICT %d %s is %v; want %v (fontTools states %v)",
							loc.label, fd, key, got[fd][key], want, ft)
					}
				}
			}
		}
	}
}

// cffPrivateNumbers reads a CID-keyed CFF's Private DICTs, by Font DICT: each
// operator names gives, as its numbers, a delta array as the absolute values
// its deltas come to.
func cffPrivateNumbers(t *testing.T, cff []byte, names map[int]string, deltaArrays map[string]bool) map[int]map[string][]float64 {
	t.Helper()
	top, err := topDictOf(cff)
	if err != nil {
		t.Fatal(err)
	}
	fdArrayAt := -1
	for _, e := range top {
		if e.op == opFDArray {
			fdArrayAt = e.operands[0]
		}
	}
	idx, err := readCFFIndex(cff, fdArrayAt)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]map[string][]float64{}
	for fd, dict := range idx.items {
		ops, _, err := parseCFFDict(dict)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ops {
			if e.op != opPrivate {
				continue
			}
			entries, _, err := parseCFF2Dict(cff[e.operands[1]:e.operands[1]+e.operands[0]], func(int) int { return 0 })
			if err != nil {
				t.Fatal(err)
			}
			out[fd] = map[string][]float64{}
			for _, pe := range entries {
				name, ok := names[pe.op]
				if !ok {
					continue
				}
				pos := 0.0
				for _, v := range pe.operands {
					x := v.v
					if deltaArrays[name] {
						pos += x
						x = pos
					}
					out[fd][name] = append(out[fd][name], x)
				}
			}
		}
	}
	return out
}

// newHarfBuzzBlend resolves blends as HarfBuzz does at the normalized location
// coords, in F2Dot14 units: hb_cff2 blend_deltas, whose scalars are floats,
// widened, each delta multiplied by its region's, summed in a double from
// zero and added to the default, and nothing rounded. No coordinates, or all
// of them zero, is the default instance.
func newHarfBuzzBlend(f *cff2Font, coords []int) *cff2Blend {
	b := &cff2Blend{store: f.store, ivs: f.ivs}
	for _, c := range coords {
		b.located = b.located || c != 0
	}
	b.factorsOf = func(region []varRegion) ([]float64, bool) {
		return []float64{float64(hbRegionScalar(region, coords))}, false
	}
	return b
}

// hbRegionScalar is a region's scalar as HarfBuzz computes it
// (VarRegionList::evaluate_impl): in floats, from the F2Dot14 integers, one
// axis at a time in the store's order, and zero the moment one axis is.
func hbRegionScalar(region []varRegion, coords []int) float32 {
	v := float32(1)
	for i, r := range region {
		coord := 0
		if i < len(coords) {
			coord = coords[i]
		}
		f := hbAxisScalar(f2Dot14Int(r.start), f2Dot14Int(r.peak), f2Dot14Int(r.end), coord)
		if f == 0 {
			return 0
		}
		v *= f
	}
	return v
}

// hbAxisScalar is VarRegionAxis::evaluate.
func hbAxisScalar(start, peak, end, coord int) float32 {
	switch {
	case peak == 0 || coord == peak:
		return 1
	case coord == 0:
		return 0
	case start > peak || peak > end:
		return 1
	case start < 0 && end > 0:
		return 1
	case coord <= start || end <= coord:
		return 0
	case coord < peak:
		return float32(coord-start) / float32(peak-start)
	}
	return float32(end-coord) / float32(end-peak)
}

// f2Dot14Int is a coordinate read as F2Dot14 back in the integer it was.
func f2Dot14Int(v float64) int { return int(math.Round(v * 16384)) }
