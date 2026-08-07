package shape

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
)

// Instancing checked against fontTools and HarfBuzz.
//
// Everything a variable font says about a point in its design space is arrived
// at by arithmetic — a weight for each tuple, a delta for each point, an
// interpolation for each point no tuple listed — and every step of it is
// plausible when read back by the code that wrote it. A test built from this
// package's own reading would agree with whatever this package decided, which is
// exactly what happened twice while this was being written: outlines that
// round-tripped perfectly and were wrong, because the inference was interpolating
// against points a previous tuple had already moved.
//
// fontTools is what a foundry cuts a static instance with, and HarfBuzz is what
// a browser renders with. Neither is this package, and where they disagree with
// it the page a reader sees is theirs.
//
// # What each answers
//
// The outlines, the bounding boxes and the side bearings come from fontTools.
// The advances come from HarfBuzz, because a font may state its advances twice —
// in HVAR and in gvar's phantom points — and where the two disagree what a
// renderer draws is HVAR's answer. Noto Sans states thirteen of them differently
// at weight 700, so this is a real choice and not a formality; the case with
// HVAR taken out of the font is the other path, checked against fontTools, whose
// instancer works from the phantom points.
//
// # The allowance, and why it is not a fudge
//
// The three implementations do not agree to the last font unit and cannot: each
// reaches the weight of a tuple by its own arithmetic. HarfBuzz quantizes the
// location to the fourteen fractional bits the format stores; fontTools' full
// instancer routes a pinned axis through its partial-instancing solver, which
// multiplies where a direct computation divides once.
//
// So each expectation file carries a measurement of that floor, made by the
// oracle about itself: 'noise-sampled' is how many values fontTools' instancer
// differs from fontTools' *own* supportScalar and IUP over the very glyphs the
// file lists. This test allows this package no more disagreement than that, and
// none of it by more than one unit — and where the floor is zero, which is five
// of the eight cases, it demands exact agreement on every number.
//
// # Why a checked-in file
//
// Regenerating it needs Python, fontTools and uharfbuzz (make varinstance);
// running it needs a Go toolchain, which is what makes it an oracle that gets
// consulted on every change rather than one nobody can run.

const varInstanceDir = "../testdata/varinstance"

// varInstanceCases are the expectation files, and how far this package is
// allowed to differ from them.
//
// allow is a count of values — one point coordinate, one bounding-box edge, one
// bearing, one advance — and is a ratchet: it may go down and never up. Each is
// bounded above by the file's own noise-sampled header, which is checked, so no
// number here can be larger than the disagreement the oracle has with itself.
var varInstanceCases = []struct {
	name  string
	allow int
	// why says what the allowance is for. A bare number is a number nobody can
	// judge.
	why string
}{
	// Every axis at its own default. The stored outlines already are this
	// instance, so nothing may move at all — which makes this the one case that
	// checks the whole decode-and-encode path on its own, with the arithmetic
	// held at zero.
	{name: "noto-default"},
	// The axis minimum, and the case the whole file exists for: this font's
	// Thin, which is what a caller who takes the default gets from 105 of
	// Google's faces.
	{name: "noto-thin"},
	{name: "noto-bold", allow: 61,
		why: "wght 700 normalizes to 0.61000732, which fontTools' instancer reaches\n" +
			"through its solver and this package by one division. The file's\n" +
			"noise-sampled header measures the same 61 values, so this package is\n" +
			"exactly as far from the instancer as fontTools' own direct computation."},
	{name: "noto-thin-condensed", allow: 1199,
		why: "two axes off their defaults at once, which is where the solver's\n" +
			"arithmetic differs most; the file's noise-sampled header measures the\n" +
			"same 1199 values."},
	{name: "noto-bold-nohvar", allow: 62,
		why: "the same location as noto-bold, so the same solver difference, plus\n" +
			"the advances — which here come from the phantom points, HVAR having\n" +
			"been taken out of the font."},
	{name: "arabic-black"},
	{name: "tibetan-light"},
	{name: "khmer-light-condensed", allow: 2,
		why: "two axes off their defaults; the file's noise-sampled header measures\n" +
			"the same two values."},
}

// TestInstancingAgreesWithFontToolsAndHarfBuzz compares every glyph of every
// expectation file.
func TestInstancingAgreesWithFontToolsAndHarfBuzz(t *testing.T) {
	for _, tc := range varInstanceCases {
		t.Run(tc.name, func(t *testing.T) {
			header, want := readVarInstanceGolden(t, tc.name)
			if noise := atoiHeader(t, header, "noise-sampled"); tc.allow > noise {
				t.Fatalf("the allowance is %d and the oracle's own noise over these glyphs is %d.\n"+
					"An allowance larger than the disagreement the oracle has with itself is\n"+
					"not an allowance for rounding, it is room for a defect.", tc.allow, noise)
			}
			// Two units, not one: a composite's box is measured from components
			// that may each be a unit out, and the two add.
			worst := atoiHeader(t, header, "noise-worst")
			if worst > 2 {
				t.Fatalf("the oracle disagrees with itself by %d units, which is not rounding", worst)
			}

			data := varInstanceFont(t, header)
			f, err := LoadInstance(data, varInstanceLocation(t, header))
			if err != nil {
				t.Fatalf("instancing: %v", err)
			}
			checkNormalized(t, header, f)

			glyf, loca, numGlyphs := instancedOutlines(t, f)
			if numGlyphs != atoiHeader(t, header, "glyphs") {
				t.Fatalf("the instance has %d glyphs and the expectations are for %d",
					numGlyphs, atoiHeader(t, header, "glyphs"))
			}
			advances, bearings := instancedMetrics(t, f, numGlyphs)

			var differing, reported int
			for _, w := range want {
				got := readGlyphOutline(t, glyf, loca, w.gid)
				got.advance, got.bearing = advances[w.gid], bearings[w.gid]
				for _, d := range compareGlyphs(w, got) {
					differing++
					if d.by > worst {
						t.Errorf("glyph %d: %s is %d and the oracle says %d", w.gid, d.what, d.got, d.want)
						continue
					}
					if tc.allow == 0 && reported < 20 {
						reported++
						t.Errorf("glyph %d: %s is %d and the oracle says %d", w.gid, d.what, d.got, d.want)
					}
				}
			}
			t.Logf("%d glyphs, %d values differ (allowance %d, oracle's own noise %d)",
				len(want), differing, tc.allow, atoiHeader(t, header, "noise-sampled"))
			if differing > tc.allow {
				t.Errorf("%d values differ and the allowance is %d.\n%s", differing, tc.allow, tc.why)
			}
			if differing < tc.allow {
				t.Errorf("%d values differ and the allowance is %d — fewer than it was.\n"+
					"Lower the allowance to %d so that what has been fixed cannot come back.",
					differing, tc.allow, differing)
			}
		})
	}
}

// TestTheInstancingOracleHasTeeth checks that the comparison would notice, by
// making the kinds of mistake instancing invites and requiring each to be seen.
//
// A test that reads an oracle can fail to compare it: a parse that quietly finds
// no glyphs, a comparison of a field against itself. Each case here is a
// plausible defect rather than a scrambled font, and each must be reported.
func TestTheInstancingOracleHasTeeth(t *testing.T) {
	header, want := readVarInstanceGolden(t, "noto-bold")
	if len(want) < 100 {
		t.Fatalf("the expectation file holds %d glyphs; it is not being read", len(want))
	}
	data := varInstanceFont(t, header)
	f, err := LoadInstance(data, varInstanceLocation(t, header))
	if err != nil {
		t.Fatal(err)
	}
	glyf, loca, numGlyphs := instancedOutlines(t, f)
	advances, bearings := instancedMetrics(t, f, numGlyphs)

	// The instance must differ from the font it was cut from, or a comparison
	// that did nothing at all would still pass every case below.
	plainGlyf, plainLoca, _ := instancedOutlinesOf(t, data)
	moved := 0
	for _, w := range want {
		if w.kind != 's' {
			continue
		}
		before := readGlyphOutline(t, plainGlyf, plainLoca, w.gid)
		after := readGlyphOutline(t, glyf, loca, w.gid)
		if len(compareGlyphs(varInstanceExpectation{gid: w.gid, outlineGlyph: before}, after)) > 0 {
			moved++
		}
	}
	if moved < len(want)/4 {
		t.Fatalf("only %d of %d glyphs moved between the stored font and the instance at %s;\n"+
			"the expectations would be met by doing nothing", moved, len(want), header["location"])
	}

	for _, tc := range []struct {
		what   string
		damage func(g *outlineGlyph)
	}{
		{"a point moved by one unit", func(g *outlineGlyph) {
			if len(g.points) > 0 {
				g.points[len(g.points)/2].x++
			}
		}},
		{"an advance one unit out", func(g *outlineGlyph) { g.advance++ }},
		{"a bearing one unit out", func(g *outlineGlyph) { g.bearing++ }},
		{"a bounding box one unit out", func(g *outlineGlyph) { g.xMax++ }},
		{"two points transposed", func(g *outlineGlyph) {
			if len(g.points) > 1 {
				g.points[0], g.points[1] = g.points[1], g.points[0]
			}
		}},
	} {
		seen := 0
		for _, w := range want {
			got := readGlyphOutline(t, glyf, loca, w.gid)
			got.advance, got.bearing = advances[w.gid], bearings[w.gid]
			tc.damage(&got)
			if len(compareGlyphs(w, got)) > 0 {
				seen++
			}
		}
		if seen == 0 {
			t.Errorf("%s is not reported for any glyph, so the comparison is not comparing that", tc.what)
		}
	}
}

// varInstanceExpectation is one glyph's expected shape.
type varInstanceExpectation struct {
	gid int
	outlineGlyph
}

// outlineGlyph is a glyph as read back out of an instanced font: its metrics,
// its box, and either its points or its components.
type outlineGlyph struct {
	kind                   byte // 's' simple, 'c' composite, 'e' empty
	advance, bearing       int
	xMin, yMin, xMax, yMax int
	points                 []outlinePoint
}

// outlinePoint is a point of a simple glyph, or a component of a composite —
// where gid is the component's glyph and x, y its offset.
type outlinePoint struct {
	gid  int
	x, y int
}

type outlineDiff struct {
	what      string
	got, want int
	by        int
}

func compareGlyphs(want varInstanceExpectation, got outlineGlyph) []outlineDiff {
	var out []outlineDiff
	add := func(what string, g, w int) {
		if g != w {
			by := g - w
			if by < 0 {
				by = -by
			}
			out = append(out, outlineDiff{what: what, got: g, want: w, by: by})
		}
	}
	if got.kind != want.kind {
		return []outlineDiff{{what: fmt.Sprintf("is a %q glyph and the oracle says %q", got.kind, want.kind), by: 1 << 30}}
	}
	add("its advance", got.advance, want.advance)
	add("its bearing", got.bearing, want.bearing)
	if want.kind == 'e' {
		return out
	}
	add("its xMin", got.xMin, want.xMin)
	add("its yMin", got.yMin, want.yMin)
	add("its xMax", got.xMax, want.xMax)
	add("its yMax", got.yMax, want.yMax)
	if len(got.points) != len(want.points) {
		return append(out, outlineDiff{
			what: fmt.Sprintf("has %d points and the oracle says %d", len(got.points), len(want.points)),
			by:   1 << 30,
		})
	}
	for i := range want.points {
		add(fmt.Sprintf("point %d's x", i), got.points[i].x, want.points[i].x)
		add(fmt.Sprintf("point %d's y", i), got.points[i].y, want.points[i].y)
		add(fmt.Sprintf("point %d's component", i), got.points[i].gid, want.points[i].gid)
	}
	return out
}

// readGlyphOutline reads one glyph out of an instanced font's glyf table.
//
// It is a second reader, written from the specification rather than shared with
// the one instancing uses, and deliberately so: the production path decodes a
// glyph, moves it and encodes it again, and a decoder that shares the encoder's
// misunderstanding hands back exactly what was put in. This one only has to
// read, which is what makes it able to disagree — and it is what caught x and y
// coordinates written interleaved instead of in two runs, a font that this
// package read back perfectly and no other program could read at all.
func readGlyphOutline(t *testing.T, glyf []byte, loca []uint32, gid int) outlineGlyph {
	t.Helper()
	if gid+1 >= len(loca) {
		t.Fatalf("glyph %d is past the end of loca", gid)
	}
	start, end := loca[gid], loca[gid+1]
	if start > end || int(end) > len(glyf) {
		t.Fatalf("glyph %d lies outside glyf", gid)
	}
	if start == end {
		return outlineGlyph{kind: 'e'}
	}
	b := glyf[start:end]
	be16 := func(at int) int {
		if at+2 > len(b) {
			t.Fatalf("glyph %d: reading past its entry at %d", gid, at)
		}
		return int(binary.BigEndian.Uint16(b[at:]))
	}
	i16 := func(at int) int { return int(int16(uint16(be16(at)))) }

	g := outlineGlyph{xMin: i16(2), yMin: i16(4), xMax: i16(6), yMax: i16(8)}
	nc := i16(0)
	if nc < 0 {
		g.kind = 'c'
		at := 10
		for {
			flags, cgid := be16(at), be16(at+2)
			at += 4
			var dx, dy int
			if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
				dx, dy = i16(at), i16(at+2)
				at += 4
			} else {
				if at+2 > len(b) {
					t.Fatalf("glyph %d: its component offsets run past its entry", gid)
				}
				dx, dy = int(int8(b[at])), int(int8(b[at+1]))
				at += 2
			}
			switch {
			case flags&0x0008 != 0: // WE_HAVE_A_SCALE
				at += 2
			case flags&0x0040 != 0: // WE_HAVE_AN_X_AND_Y_SCALE
				at += 4
			case flags&0x0080 != 0: // WE_HAVE_A_TWO_BY_TWO
				at += 8
			}
			g.points = append(g.points, outlinePoint{gid: cgid, x: dx, y: dy})
			if flags&0x0020 == 0 { // MORE_COMPONENTS
				break
			}
		}
		return g
	}

	g.kind = 's'
	n := 0
	if nc > 0 {
		n = be16(10+2*(nc-1)) + 1
	}
	at := 10 + 2*nc
	at += 2 + be16(at) // the instruction block
	flags := make([]byte, 0, n)
	for len(flags) < n {
		if at >= len(b) {
			t.Fatalf("glyph %d: its flags end early", gid)
		}
		f := b[at]
		at++
		flags = append(flags, f)
		if f&0x08 == 0 { // REPEAT_FLAG
			continue
		}
		if at >= len(b) {
			t.Fatalf("glyph %d: its repeat count is past the end", gid)
		}
		repeat := int(b[at])
		at++
		for j := 0; j < repeat && len(flags) < n; j++ {
			flags = append(flags, f)
		}
	}
	// Every point's x, and then every point's y.
	read := func(short, same byte) []int {
		out := make([]int, n)
		v := 0
		for i, f := range flags {
			switch {
			case f&short != 0:
				if at >= len(b) {
					t.Fatalf("glyph %d: its coordinates end early", gid)
				}
				d := int(b[at])
				at++
				if f&same == 0 {
					d = -d
				}
				v += d
			case f&same == 0:
				v += i16(at)
				at += 2
			}
			out[i] = v
		}
		return out
	}
	xs := read(0x02, 0x10)
	ys := read(0x04, 0x20)
	for i := 0; i < n; i++ {
		g.points = append(g.points, outlinePoint{x: xs[i], y: ys[i]})
	}
	return g
}

// instancedOutlines takes the glyf and loca out of the font a face was built
// from. The instance always writes the long loca form.
func instancedOutlines(t *testing.T, f *Face) (glyf []byte, loca []uint32, numGlyphs int) {
	t.Helper()
	return instancedOutlinesOf(t, f.data)
}

// instancedOutlinesOf is the same for a font program that has not been loaded,
// which is how the stored font is read back for comparison. The long loca form
// is what an instance writes; the stored font may use the short one.
func instancedOutlinesOf(t *testing.T, data []byte) (glyf []byte, loca []uint32, numGlyphs int) {
	t.Helper()
	tables := font.SFNTTables(data)
	if tables == nil {
		t.Fatal("the instance is not an sfnt")
	}
	numGlyphs = font.Be16(tables["maxp"], 4)
	raw := tables["loca"]
	long := font.Be16(tables["head"], 50) == 1
	need := 2 * (numGlyphs + 1)
	if long {
		need = 4 * (numGlyphs + 1)
	}
	if len(raw) < need {
		t.Fatalf("loca holds %d bytes for %d glyphs", len(raw), numGlyphs)
	}
	loca = make([]uint32, numGlyphs+1)
	for i := range loca {
		if long {
			loca[i] = binary.BigEndian.Uint32(raw[4*i:])
		} else {
			loca[i] = uint32(binary.BigEndian.Uint16(raw[2*i:])) * 2
		}
	}
	return tables["glyf"], loca, numGlyphs
}

func instancedMetrics(t *testing.T, f *Face, numGlyphs int) (advances, bearings []int) {
	t.Helper()
	tables := font.SFNTTables(f.data)
	hmtx, hhea := tables["hmtx"], tables["hhea"]
	metrics := font.Be16(hhea, 34)
	if metrics < 1 || metrics > numGlyphs {
		t.Fatalf("hhea states %d horizontal metrics for %d glyphs", metrics, numGlyphs)
	}
	advances = make([]int, numGlyphs)
	bearings = make([]int, numGlyphs)
	for gid := 0; gid < numGlyphs; gid++ {
		if gid < metrics {
			advances[gid] = font.Be16(hmtx, 4*gid)
			bearings[gid] = int(int16(uint16(font.Be16(hmtx, 4*gid+2))))
			continue
		}
		advances[gid] = font.Be16(hmtx, 4*(metrics-1))
		bearings[gid] = int(int16(uint16(font.Be16(hmtx, 4*metrics+2*(gid-metrics)))))
	}
	return advances, bearings
}

// readVarInstanceGolden reads one expectation file: its header and its glyphs.
func readVarInstanceGolden(t *testing.T, name string) (map[string]string, []varInstanceExpectation) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(varInstanceDir, name+".txt"))
	if err != nil {
		t.Fatalf("reading the expectations: %v\nRun `make varinstance` to generate them.", err)
	}
	header := map[string]string{}
	var out []varInstanceExpectation
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "s", "c", "e":
		default:
			header[fields[0]] = strings.Join(fields[1:], " ")
			continue
		}
		out = append(out, parseGlyphLine(t, fields))
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no glyphs, so it proves nothing", name)
	}
	return header, out
}

func parseGlyphLine(t *testing.T, f []string) varInstanceExpectation {
	t.Helper()
	num := func(s string) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("%q is not a number in %q", s, strings.Join(f, " "))
		}
		return v
	}
	e := varInstanceExpectation{gid: num(f[1])}
	e.kind = f[0][0]
	e.advance, e.bearing = num(f[2]), num(f[3])
	if e.kind == 'e' {
		return e
	}
	e.xMin, e.yMin, e.xMax, e.yMax = num(f[4]), num(f[5]), num(f[6]), num(f[7])
	for _, p := range f[8:] {
		var pt outlinePoint
		if i := strings.IndexByte(p, ':'); i >= 0 {
			pt.gid = num(p[:i])
			p = p[i+1:]
		}
		i := strings.IndexByte(p, ',')
		if i < 0 {
			t.Fatalf("%q is not a point", p)
		}
		pt.x, pt.y = num(p[:i]), num(p[i+1:])
		e.points = append(e.points, pt)
	}
	return e
}

// varInstanceFont loads the font a case is about, refusing one the expectations
// were not generated from — every expectation is about a glyph index, so a
// different font would leave this asserting yesterday's answers about today's
// glyphs.
func varInstanceFont(t *testing.T, header map[string]string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", header["font"]))
	if err != nil {
		t.Fatalf("reading the font: %v", err)
	}
	sum := sha256.Sum256(data)
	if got, want := hex.EncodeToString(sum[:]), header["font-sha256"]; got != want {
		t.Fatalf("the expectations were generated against font %s and this one is %s.\n"+
			"Run `make varinstance` to regenerate them.", want, got)
	}
	if header["strip-hvar"] == "yes" {
		data = fontWithoutTable(t, data, "HVAR")
	}
	return data
}

// fontWithoutTable rebuilds a font with one table left out, which is how the
// phantom-point path is reached in a font that has HVAR.
func fontWithoutTable(t *testing.T, data []byte, tag string) []byte {
	t.Helper()
	tables := font.SFNTTables(data)
	if _, ok := tables[tag]; !ok {
		t.Fatalf("the font has no %s table to take out", tag)
	}
	out := map[string][]byte{}
	for k, v := range tables {
		if k != tag {
			out[k] = v
		}
	}
	return assembleSFNT(out)
}

// varInstanceLocation reads the location the case is at, in user coordinates.
func varInstanceLocation(t *testing.T, header map[string]string) map[string]float64 {
	t.Helper()
	loc := map[string]float64{}
	if header["location"] == "default" {
		return loc
	}
	for _, part := range strings.Fields(header["location"]) {
		tag, value, ok := strings.Cut(part, "=")
		if !ok {
			t.Fatalf("%q is not an axis setting", part)
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatalf("%q is not a coordinate: %v", part, err)
		}
		loc[tag] = v
	}
	return loc
}

// checkNormalized compares the normalized location this package computed
// against fontTools'.
//
// It is checked separately from the outlines because it is upstream of all of
// them: a location half a percent out moves every point of every glyph a little,
// which reads as rounding noise rather than as the wrong weight. The tolerance
// is for the last bits of a division, not for a difference anyone could see —
// a coordinate is between -1 and 1 and an axis is a thousand units wide.
func checkNormalized(t *testing.T, header map[string]string, f *Face) {
	t.Helper()
	want := strings.Fields(header["normalized"])
	if len(want) != len(f.varCoords) {
		t.Fatalf("the location has %d axes and the oracle states %d", len(f.varCoords), len(want))
	}
	tags := strings.Fields(header["axes"])
	for i, s := range want {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("%q is not a coordinate: %v", s, err)
		}
		if math.Abs(v-f.varCoords[i]) > 1e-12 {
			t.Errorf("%s normalizes to %v and the oracle says %v", tags[i], f.varCoords[i], v)
		}
	}
}

func atoiHeader(t *testing.T, header map[string]string, key string) int {
	t.Helper()
	v, err := strconv.Atoi(header[key])
	if err != nil {
		t.Fatalf("the %q header is %q, which is not a number", key, header[key])
	}
	return v
}
