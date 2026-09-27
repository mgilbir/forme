package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/internal/costtest"
)

// Components placed by matching points, instanced, held to HarfBuzz and to
// fontTools.
//
// testdata/harfbuzz/pointmatch.py instances the two faces
// pointmatch_fixture.py builds at five weights, with HarfBuzz's instancer and
// with fontTools', draws each glyph with HarfBuzz at each weight, and draws
// fontTools' instance with HarfBuzz as well. The answers are checked in as
// pointmatch.expected.txt.

// pmGlyph is one glyph of an instance as an oracle states it: its advance,
// side bearing and box, and each component.
type pmGlyph struct {
	adv, lsb int
	box      string
	comps    []string
}

// pmFace is one face at one weight.
type pmFace struct {
	name, sum string
	weight    float64
	drawn     map[int][][2]float64 // H: HarfBuzz at the weight
	static    map[int][][2]float64 // S: HarfBuzz reading fontTools' instance
	flat      map[int][][2]float64 // P: fontTools' own flattening
	hbInst    map[int]pmGlyph      // I
	ftInst    map[int]pmGlyph      // F
	noFT      bool
}

func readPointMatchGolden(t *testing.T) []*pmFace {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "pointmatch.expected.txt")
	refuseUnpinnedOracle(t, path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v\nRun `make hbpointmatch` to generate it.", err)
	}
	defer f.Close()
	var faces []*pmFace
	var cur *pmFace
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		tag, rest, _ := strings.Cut(text, " ")
		switch tag {
		case "face":
			fields := strings.Fields(rest)
			name, weight, _ := strings.Cut(fields[0], "@wght=")
			w, err := strconv.ParseFloat(weight, 64)
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			cur = &pmFace{name: name, sum: fields[1], weight: w,
				drawn: map[int][][2]float64{}, static: map[int][][2]float64{}, flat: map[int][][2]float64{},
				hbInst: map[int]pmGlyph{}, ftInst: map[int]pmGlyph{}}
			faces = append(faces, cur)
		case "H", "S":
			gid, pts := parsePath(t, line, rest)
			if tag == "H" {
				cur.drawn[gid] = pts
			} else {
				cur.static[gid] = pts
			}
		case "P":
			fields := strings.Fields(rest)
			gid, _ := strconv.Atoi(fields[0])
			var pts [][2]float64
			for _, p := range fields[1:] {
				xs, ys, _ := strings.Cut(p, ",")
				pts = append(pts, [2]float64{parseNum(t, line, xs), parseNum(t, line, ys)})
			}
			cur.flat[gid] = pts
		case "I", "F":
			parts := strings.Split(rest, " | ")
			head := strings.Fields(parts[0])
			gid, _ := strconv.Atoi(head[0])
			g := pmGlyph{box: strings.Join(head[3:], " "), comps: parts[1:]}
			g.adv, _ = strconv.Atoi(head[1])
			g.lsb, _ = strconv.Atoi(head[2])
			if tag == "I" {
				cur.hbInst[gid] = g
			} else {
				cur.ftInst[gid] = g
			}
		case "fonttools":
			cur.noFT = true
		default:
			t.Fatalf("line %d: %q", line, text)
		}
	}
	return faces
}

func parseNum(t *testing.T, line int, s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("line %d: %v", line, err)
	}
	return v
}

// parsePath reads a glyph's path, all of it lines, as the points of its
// contours in order: the line back to where a contour started is HarfBuzz
// closing it, and is not a point.
func parsePath(t *testing.T, line int, rest string) (int, [][2]float64) {
	f := strings.Fields(rest)
	gid, _ := strconv.Atoi(f[0])
	var pts [][2]float64
	start := 0
	for i := 1; i < len(f); {
		switch f[i] {
		case "M", "L":
			p := [2]float64{parseNum(t, line, f[i+1]), parseNum(t, line, f[i+2])}
			if f[i] == "M" {
				start = len(pts)
			}
			pts = append(pts, p)
			i += 3
		case "Z":
			if n := len(pts); n-start > 1 && pts[n-1] == pts[start] {
				pts = pts[:n-1]
			}
			i++
		default:
			t.Fatalf("line %d: %q in a path", line, f[i])
		}
	}
	return gid, pts
}

// pmInstance is a face from LoadInstance taken apart: each glyph's record in
// the oracles' form, and its points as HarfBuzz reads them.
type pmInstance struct {
	f      *Face
	glyf   []byte
	loca   []uint32
	n      int
	glyphs map[int]pmGlyph
}

func loadPointMatch(t *testing.T, want *pmFace) *pmInstance {
	t.Helper()
	data := harfbuzzFont(t, want.name)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbpointmatch` to regenerate them.", want.name, want.sum, got)
	}
	f, err := LoadInstance(data, map[string]float64{"wght": want.weight})
	if err != nil {
		t.Fatalf("%s at %v: %v", want.name, want.weight, err)
	}
	tables := font.SFNTTables(f.Program())
	in := &pmInstance{f: f, glyf: tables["glyf"], n: f.NumGlyphs(), glyphs: map[int]pmGlyph{}}
	loca := tables["loca"]
	in.loca = make([]uint32, in.n+1)
	for i := range in.loca {
		in.loca[i] = binary.BigEndian.Uint32(loca[4*i:])
	}
	for gid := 0; gid < in.n; gid++ {
		lsb, _ := f.leftSideBearing(gid)
		g := pmGlyph{adv: f.advanceUnits(gid), lsb: lsb, box: "empty"}
		entry := in.glyf[in.loca[gid]:in.loca[gid+1]]
		if len(entry) > 0 {
			g.box = strings.Join([]string{
				strconv.Itoa(signed16(font.Be16(entry, 2))), strconv.Itoa(signed16(font.Be16(entry, 4))),
				strconv.Itoa(signed16(font.Be16(entry, 6))), strconv.Itoa(signed16(font.Be16(entry, 8))),
			}, " ")
			v, err := decodeVarGlyph(entry, in.n)
			if err != nil {
				t.Fatalf("glyph %d of the instance does not decode: %v", gid, err)
			}
			for i, c := range v.comps {
				// The flags fontTools keeps, which are the ones both oracles
				// state: the rest say how the record is stored.
				flags := c.flags & (0x0004 | 0x0010 | 0x0200 | 0x0400 | 0x0800 | 0x1000)
				args := "at " + strconv.Itoa(int(v.x[i])) + " " + strconv.Itoa(int(v.y[i]))
				if c.matched {
					args = "match " + strconv.Itoa(c.p1) + " " + strconv.Itoa(c.p2)
				}
				m := make([]string, 4)
				for k := range m {
					m[k] = strconv.Itoa(int(math.Round(c.scale[k] * 16384)))
				}
				g.comps = append(g.comps, strconv.Itoa(c.glyph)+" "+strconv.Itoa(flags)+" "+args+" "+strings.Join(m, ","))
			}
		}
		in.glyphs[gid] = g
	}
	return in
}

// points are a glyph's points as HarfBuzz reads the instance.
func (in *pmInstance) points(t *testing.T, gid int) [][2]float64 {
	t.Helper()
	budget := int64(maxInstanceWork)
	get := func(gid int) (*varGlyph, error) {
		start, end := in.loca[gid], in.loca[gid+1]
		if start >= end {
			return &varGlyph{}, nil
		}
		return decodeVarGlyph(in.glyf[start:end], in.n)
	}
	w := &matchWalk{glyph: get, budget: &budget}
	if _, err := w.walk(gid, 0); err != nil {
		t.Fatalf("glyph %d: %v", gid, err)
	}
	return w.points
}

// pmNested is PointMatch.ttf's glyph whose match fontTools and HarfBuzz count
// from different points; pmOwnMetrics its glyph taking its metrics from its
// matched component, whose advance and side bearing this package reads as
// HarfBuzz reads them, where both instancers take the composite's own (see
// instance.go on USE_MY_METRICS).
const (
	pmNested     = 5
	pmOwnMetrics = 6
)

// TestPointMatchInstancesAsFontToolsDoes: a face whose matches are all of the
// kind an instance keeps is instanced as fontTools' instancer instances it —
// every record the same, every box, every advance and side bearing, and every
// point as fontTools flattens it — and every record the same as HarfBuzz's
// instancer's.
func TestPointMatchInstancesAsFontToolsDoes(t *testing.T) {
	faces := readPointMatchGolden(t)
	compared := 0
	for _, want := range faces {
		if want.name != "PointMatch.ttf" {
			continue
		}
		t.Run(want.name+"@"+strconv.FormatFloat(want.weight, 'g', -1, 64), func(t *testing.T) {
			in := loadPointMatch(t, want)
			if want.noFT || len(want.ftInst) != in.n || len(want.hbInst) != in.n {
				t.Fatalf("the expectations hold %d glyphs from fontTools and %d from HarfBuzz, and the face has %d",
					len(want.ftInst), len(want.hbInst), in.n)
			}
			for gid := 0; gid < in.n; gid++ {
				got, ft, hb := in.glyphs[gid], want.ftInst[gid], want.hbInst[gid]
				if strings.Join(got.comps, " | ") != strings.Join(ft.comps, " | ") {
					t.Errorf("glyph %d's components are %q, and fontTools' %q", gid, got.comps, ft.comps)
				}
				if strings.Join(got.comps, " | ") != strings.Join(hb.comps, " | ") {
					t.Errorf("glyph %d's components are %q, and HarfBuzz's instancer's %q", gid, got.comps, hb.comps)
				}
				if gid == pmNested {
					continue // TestPointMatchIsReadAsHarfBuzzReadsIt
				}
				if got.box != ft.box {
					t.Errorf("glyph %d's box is %s, and fontTools' %s", gid, got.box, ft.box)
				}
				if gid != pmOwnMetrics && (got.adv != ft.adv || got.lsb != ft.lsb) {
					t.Errorf("glyph %d advances %d with a side bearing of %d, and fontTools' %d and %d",
						gid, got.adv, got.lsb, ft.adv, ft.lsb)
				}
				if pts := in.points(t, gid); !samePoints(pts, want.flat[gid], 0) {
					t.Errorf("glyph %d's points are %v, and fontTools' %v", gid, pts, want.flat[gid])
				}
				compared++
			}
		})
	}
	if compared == 0 {
		t.Fatal("no glyph was compared")
	}
}

// TestPointMatchIsReadAsHarfBuzzReadsIt: every glyph of the instance, drawn as
// HarfBuzz reads it, is the glyph HarfBuzz draws from fontTools' instance, point
// for point, and its box is the box of those points — including the nested
// composite, whose match HarfBuzz counts from the start of the outermost glyph
// and fontTools from the start of the composite the match is in. A box drawn
// from fontTools' count would not be the box of what HarfBuzz draws.
func TestPointMatchIsReadAsHarfBuzzReadsIt(t *testing.T) {
	faces := readPointMatchGolden(t)
	nestedDiffers := false
	for _, want := range faces {
		if want.name != "PointMatch.ttf" {
			continue
		}
		t.Run(want.name+"@"+strconv.FormatFloat(want.weight, 'g', -1, 64), func(t *testing.T) {
			in := loadPointMatch(t, want)
			for gid := 1; gid < in.n; gid++ {
				pts := in.points(t, gid)
				if !samePoints(pts, want.static[gid], 0) {
					t.Errorf("glyph %d's points are %v, and HarfBuzz reads %v", gid, pts, want.static[gid])
					continue
				}
				if box := boxOf(pts); box != in.glyphs[gid].box {
					t.Errorf("glyph %d's box is %s, and its points' %s", gid, in.glyphs[gid].box, box)
				}
			}
			if !samePoints(want.static[pmNested], want.flat[pmNested], 0) {
				nestedDiffers = true
			}
		})
	}
	if !nestedDiffers {
		t.Error("fontTools and HarfBuzz count the nested composite's points alike at every weight, " +
			"so the fixture no longer tells their counts apart")
	}
}

// TestPointMatchPlacesWhatAnInstanceCannotKeep: in PointMatchPhantom.ttf every
// match is one an instance cannot keep — a phantom point, the component's own
// point, a point nobody has — and each is placed at an offset, where HarfBuzz
// draws the variable face: every point within a unit, the rounding of the
// component's point and of the offset. HarfBuzz's own instancer keeps these
// matches, and its instance draws them up to thirty-three units away from
// where HarfBuzz draws the variable face; fontTools' cannot instance the face.
func TestPointMatchPlacesWhatAnInstanceCannotKeep(t *testing.T) {
	faces := readPointMatchGolden(t)
	source, err := Load(harfbuzzFont(t, "PointMatchPhantom.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for _, want := range faces {
		if want.name != "PointMatchPhantom.ttf" {
			continue
		}
		if !want.noFT {
			t.Error("fontTools instanced PointMatchPhantom.ttf, which the fixture means it not to")
		}
		t.Run(want.name+"@"+strconv.FormatFloat(want.weight, 'g', -1, 64), func(t *testing.T) {
			in := loadPointMatch(t, want)
			for gid := 3; gid < in.n; gid++ {
				for _, c := range in.glyphs[gid].comps {
					if strings.Contains(c, "match") {
						t.Errorf("glyph %d keeps its match: %s", gid, c)
					}
				}
				// HarfBuzz draws each glyph moved by its left phantom point,
				// which in these faces nothing moves: the source's box less
				// its side bearing.
				lsb, _ := source.leftSideBearing(gid)
				shift := float64(source.prog.GlyphBBox[gid][0] - lsb)
				pts := in.points(t, gid)
				for i := range pts {
					pts[i][0] -= shift
				}
				if !samePoints(pts, want.drawn[gid], 1) {
					t.Errorf("glyph %d's points are %v, and HarfBuzz draws %v", gid, pts, want.drawn[gid])
				}
				compared++
			}
		})
	}
	if compared == 0 {
		t.Fatal("no glyph was compared")
	}
}

// TestPointMatchDrawsWhereHarfBuzzDraws: PointMatch.ttf's instance, drawn, is
// HarfBuzz's drawing of the variable face at the weight to within the
// rounding an instance's whole units cost: half a unit for a point, and for a
// matched component the two points matched as well, through the transform.
func TestPointMatchDrawsWhereHarfBuzzDraws(t *testing.T) {
	faces := readPointMatchGolden(t)
	source, err := Load(harfbuzzFont(t, "PointMatch.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range faces {
		if want.name != "PointMatch.ttf" {
			continue
		}
		t.Run(want.name+"@"+strconv.FormatFloat(want.weight, 'g', -1, 64), func(t *testing.T) {
			in := loadPointMatch(t, want)
			for gid := 1; gid < in.n; gid++ {
				// The glyph taking its metrics from mark is drawn from mark's
				// left phantom point.
				from := gid
				if gid == pmOwnMetrics {
					from = 2
				}
				lsb, _ := source.leftSideBearing(from)
				shift := float64(source.prog.GlyphBBox[from][0] - lsb)
				pts := in.points(t, gid)
				for i := range pts {
					pts[i][0] -= shift
				}
				if !samePoints(pts, want.drawn[gid], 1.5) {
					t.Errorf("glyph %d's points are %v, and HarfBuzz draws %v", gid, pts, want.drawn[gid])
				}
			}
		})
	}
}

func samePoints(a, b [][2]float64, within float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i][0]-b[i][0]) > within || math.Abs(a[i][1]-b[i][1]) > within {
			return false
		}
	}
	return true
}

func boxOf(pts [][2]float64) string {
	var b floatBounds
	for _, p := range pts {
		b.add(p[0], p[1])
	}
	return strings.Join([]string{
		strconv.Itoa(otRound(b.xMin)), strconv.Itoa(otRound(b.yMin)),
		strconv.Itoa(otRound(b.xMax)), strconv.Itoa(otRound(b.yMax)),
	}, " ")
}

// TestAMatchedComponentIsWrittenAsItWas: a matched component's two point
// numbers go back out as they came in — a byte each where both fit, two where
// either does not — with its transform, and read back the same.
func TestAMatchedComponentIsWrittenAsItWas(t *testing.T) {
	for _, pts := range [][2]int{{3, 255}, {256, 0}, {7, 300}, {65535, 65535}} {
		// A component of glyph 1 with a scale, matched, then one at an offset.
		entry := make([]byte, 10)
		binary.BigEndian.PutUint16(entry, 0xFFFF)
		entry = binary.BigEndian.AppendUint16(entry, compArgsAreWords|compHaveScale|compMoreComponents|compUseMyMetrics)
		entry = binary.BigEndian.AppendUint16(entry, 1)
		entry = binary.BigEndian.AppendUint16(entry, uint16(pts[0]))
		entry = binary.BigEndian.AppendUint16(entry, uint16(pts[1]))
		entry = binary.BigEndian.AppendUint16(entry, 0x2000) // a scale of a half
		entry = binary.BigEndian.AppendUint16(entry, compArgsAreXY)
		entry = binary.BigEndian.AppendUint16(entry, 1)
		entry = append(entry, 5, 6)
		g, err := decodeVarGlyph(entry, 2)
		if err != nil {
			t.Fatal(err)
		}
		c := g.comps[0]
		if !c.matched || c.p1 != pts[0] || c.p2 != pts[1] || c.scale[0] != 0.5 || g.x[0] != 0 || g.y[0] != 0 {
			t.Fatalf("%v: read as %+v at (%v, %v)", pts, c, g.x[0], g.y[0])
		}
		out, err := encodeVarGlyph(g)
		if err != nil {
			t.Fatal(err)
		}
		words := pts[0] > 255 || pts[1] > 255
		wantFlags := compHaveScale | compMoreComponents | compUseMyMetrics
		wantLen := 10 + 4 + 2 + 2 + 4 + 2
		if words {
			wantFlags |= compArgsAreWords
			wantLen += 2
		}
		if got := font.Be16(out, 10); got != wantFlags || len(out) != wantLen {
			t.Errorf("%v: written with flags %#x in %d bytes, want %#x in %d", pts, got, len(out), wantFlags, wantLen)
		}
		again, err := decodeVarGlyph(out, 2)
		if err != nil {
			t.Fatal(err)
		}
		if a := again.comps[0]; !a.matched || a.p1 != pts[0] || a.p2 != pts[1] || a.scale != c.scale ||
			again.comps[1].matched || again.x[1] != 5 || again.y[1] != 6 {
			t.Errorf("%v: read back as %+v and %+v", pts, a, again.comps[1])
		}
	}
}

// TestTheNestingCheckIsLinear: checking where each composite is nested asks a
// glyph once for every place it can be — at the start of the points drawn or
// not — however many glyphs begin with it. Asked again for each, m glyphs
// beginning with one of m components is m² steps.
func TestTheNestingCheckIsLinear(t *testing.T) {
	font := func(m int) []*varGlyph {
		leaf := &varGlyph{x: make([]float64, 8), y: make([]float64, 8), ends: []int{3}, flags: make([]byte, 4)}
		shared := &varGlyph{composite: true, x: make([]float64, m+4), y: make([]float64, m+4)}
		for i := 0; i < m; i++ {
			shared.comps = append(shared.comps, varComponent{glyph: 1, flags: compArgsAreXY, scale: [4]float64{1, 0, 0, 1}})
		}
		glyphs := []*varGlyph{{}, leaf, shared}
		for i := 0; i < m; i++ {
			glyphs = append(glyphs, &varGlyph{composite: true, x: make([]float64, 6), y: make([]float64, 6),
				comps: []varComponent{
					{glyph: 2, flags: compArgsAreXY, scale: [4]float64{1, 0, 0, 1}},
					{glyph: 1, flags: compArgsAreXY, scale: [4]float64{1, 0, 0, 1}},
				}})
		}
		return glyphs
	}
	check := func(m int) func() {
		glyphs := font(m)
		placed := make([]bool, len(glyphs))
		return func() {
			if err := refusePlacedInside(glyphs, placed); err != nil {
				t.Fatal(err)
			}
		}
	}
	const small, large = 500, 2000
	c := costtest.Time(t, "checking m composites beginning with one of m components", check(small), check(large))
	// Four times the input: linear is 4 and quadratic 16.
	if c.Ratio > 8 {
		t.Errorf("checking %d composites took %v and %d took %v, a factor of %.1f for four times the input",
			small, c.Small, large, c.Large, c.Ratio)
	}
}

// TestAMatchSurvivesEveryByteChanged instances PointMatchPhantom.ttf with each
// byte of its glyf and gvar set to nothing and to everything in turn: a
// match's two point numbers, the components they count across and the deltas
// that move them are each a number a reader could trust.
func TestAMatchSurvivesEveryByteChanged(t *testing.T) {
	data := harfbuzzFont(t, "PointMatchPhantom.ttf")
	loaded := 0
	for _, tag := range []string{"glyf", "gvar"} {
		off, length := tableRange(t, data, tag)
		for i := off; i < off+length; i++ {
			for _, b := range []byte{0x00, 0xFF} {
				if data[i] == b {
					continue
				}
				mutated := append([]byte(nil), data...)
				mutated[i] = b
				noPanic(t, tag, func() {
					if f, err := LoadInstance(mutated, map[string]float64{"wght": 900}); err == nil {
						loaded++
						for gid := 0; gid < f.NumGlyphs(); gid++ {
							f.GlyphExtents(gid)
						}
					}
				})
			}
		}
	}
	if loaded == 0 {
		t.Error("no changed font was instanced, so none was read past its first check")
	}
}

// tableRange is where a table lies in an sfnt.
func tableRange(t *testing.T, data []byte, tag string) (int, int) {
	t.Helper()
	n := int(binary.BigEndian.Uint16(data[4:]))
	for i := 0; i < n; i++ {
		rec := data[12+16*i:]
		if string(rec[:4]) == tag {
			return int(binary.BigEndian.Uint32(rec[8:])), int(binary.BigEndian.Uint32(rec[12:]))
		}
	}
	t.Fatalf("no %s table", tag)
	return 0, 0
}
