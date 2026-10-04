package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The points kerx attaches by in a CFF face (cffContourPoints) held to the
// points FreeType's CFF loader builds, unscaled and unhinted, which is what
// HarfBuzz over FreeType hands kerx: every glyph of the CFF faces in the tree,
// and every glyph of the CFF fonts of `make cff-fonts` (CFF_FONTS), static,
// CFF2 at its default and CFF2 cut at a location. The answers are checked in
// as testdata/freetype/points.expected.txt; see points.py there.

// freetypeDivergent are the glyphs of CFFInk.otf, and of VarCompositeCFF.otf,
// which carries the same charstrings in the same order, whose points are not
// FreeType's: each by its index, with its name in cffink_fixture.py and why. Each is a charstring HarfBuzz and FreeType's Adobe engine read
// differently, and cffContourPoints reads it as measuring does, HarfBuzz's
// way: so the outline the glyph is measured and drawn by is the one its points
// are numbered on.
var freetypeDivergent = map[int]struct{ name, why string }{
	10:  {"rcurveline.short", "too few operands: FreeType draws none, HarfBuzz what they reach"},
	12:  {"rlinecurve.two", "operands past a whole curve: FreeType refuses the glyph"},
	13:  {"rlinecurve.short", "too few operands: FreeType draws none, HarfBuzz what they reach"},
	23:  {"vhcurveto.10", "an operand count neither engine expects, read two ways"},
	37:  {"hvcurveto.10", "an operand count neither engine expects, read two ways"},
	47:  {"hflex.wrong", "a flex of the wrong count, read two ways"},
	49:  {"flex.wrong", "a flex of the wrong count, read two ways"},
	51:  {"hflex1.wrong", "a flex of the wrong count, read two ways"},
	53:  {"flex1.wrong", "a flex of the wrong count, read two ways"},
	65:  {"numbers", "a number past what a 16.16 holds, which FreeType wraps"},
	69:  {"far", "coordinates past FreeType's outline limits: FreeType refuses the glyph"},
	70:  {"far.left", "coordinates past FreeType's outline limits: FreeType refuses the glyph"},
	71:  {"shortint.truncated", "a number the charstring ends inside, read two ways"},
	75:  {"stack.fits", "more operands than FreeType's stack holds: FreeType refuses the glyph"},
	77:  {"no.endchar", "no endchar: HarfBuzz refuses the glyph, FreeType draws it"},
	78:  {"escape.last", "an escape as the last byte: HarfBuzz refuses the glyph, FreeType draws it"},
	87:  {"subr.noreturn", "a subroutine with no return, read two ways"},
	92:  {"subr.fixed", "a subroutine number with a fraction: FreeType refuses the glyph"},
	94:  {"subr.depth11", "subroutines nested past HarfBuzz's limit, and within FreeType's"},
	104: {"seac.nocode", "a seac whose accent code names no glyph: FreeType draws the base alone"},
	111: {"ops.200000", "past HarfBuzz's operator cap, which FreeType does not have"},
}

// ftFace is one face of points.expected.txt and its glyphs' points: whole
// (full and every=N), or hashed.
type ftFace struct {
	name, sum, location, kind string
	glyphs                    map[int]string
}

func readFreeTypePoints(t *testing.T) []*ftFace {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "testdata", "freetype", "points.expected.txt"))
	if err != nil {
		t.Fatalf("%v; run `make ftpoints`", err)
	}
	defer file.Close()
	var faces []*ftFace
	sc := bufio.NewScanner(file)
	sc.Buffer(nil, 1<<22)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		switch {
		case len(f) == 0 || f[0] == "#":
		case f[0] == "face" && len(f) == 5:
			faces = append(faces, &ftFace{name: f[1], sum: f[2], location: f[3], kind: f[4], glyphs: map[int]string{}})
		case (f[0] == "G" || f[0] == "H") && len(faces) > 0 && len(f) >= 2:
			gid, err := strconv.Atoi(f[1])
			if err != nil {
				t.Fatalf("%q: %v", sc.Text(), err)
			}
			if f[0] == "G" && len(f) > 2 && f[2] != "none" {
				// The points less their tags.
				pts := make([]string, len(f)-2)
				for i, p := range f[2:] {
					pts[i] = p[:strings.LastIndex(p, ",")]
				}
				faces[len(faces)-1].glyphs[gid] = strings.Join(pts, " ")
			} else {
				faces[len(faces)-1].glyphs[gid] = strings.Join(f[2:], " ")
			}
		default:
			t.Fatalf("%q: a face, or a glyph's points", sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return faces
}

// pointsText is a glyph's points as points.expected.txt writes them, x,y
// separated by spaces.
func pointsText(points [][2]int) string {
	s := make([]string, len(points))
	for i, p := range points {
		s[i] = fmt.Sprintf("%d,%d", p[0], p[1])
	}
	return strings.Join(s, " ")
}

func TestCFFPointsAreFreeTypes(t *testing.T) {
	faces := readFreeTypePoints(t)
	if len(faces) < 3 {
		t.Fatalf("points.expected.txt holds %d faces; run `make ftpoints`", len(faces))
	}
	for _, ft := range faces {
		path := filepath.Join(harfbuzzDir, "fonts", ft.name)
		if _, err := os.Stat(path); err != nil {
			dir := os.Getenv("CFF_FONTS")
			if dir == "" {
				t.Logf("%s: CFF_FONTS is not set, so it is not compared", ft.name)
				continue
			}
			path = filepath.Join(dir, ft.name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != ft.sum {
			t.Fatalf("%s is not the face the points were recorded for; run `make ftpoints`", ft.name)
		}
		var f *Face
		if ft.location == "-" {
			f, err = Load(data)
		} else {
			coords := map[string]float64{}
			for _, a := range strings.Split(ft.location, ",") {
				tag, v, _ := strings.Cut(a, "=")
				coords[tag], _ = strconv.ParseFloat(v, 64)
			}
			f, err = LoadInstance(data, coords)
		}
		if err != nil {
			t.Fatalf("%s: %v", ft.name, err)
		}
		divergent := ft.name == "CFFInk.otf" || ft.name == "VarCompositeCFF.otf"
		label := ft.name + "@" + ft.location
		compared, diverged := 0, 0
		for gid, want := range ft.glyphs {
			got := pointsText(f.cffContourPoints(gid))
			if ft.kind == "hash" {
				if want == "none" {
					t.Errorf("%s glyph %d: FreeType loads no outline", label, gid)
					continue
				}
				h := sha256.Sum256([]byte(got))
				got = hex.EncodeToString(h[:])[:16]
			}
			if want == "none" {
				want = ""
			}
			compared++
			listed, known := freetypeDivergent[gid]
			known = known && divergent
			switch {
			case got == want && known:
				t.Errorf("%s glyph %d (%s) has FreeType's points, and is listed as not having them: %s",
					label, gid, listed.name, listed.why)
			case got == want:
			case known:
				diverged++
			case ft.location != "-" && withinAUnit(got, want):
				// FreeType blends a CFF2 location in 16.16 fixed point and
				// this in floating point, as HarfBuzz and fontTools do; the
				// two can stand either side of a whole unit, which the floor
				// an unscaled load takes then parts by one.
			default:
				t.Errorf("%s glyph %d:\n  FreeType %s\n  forme    %s", label, gid, want, got)
			}
		}
		if compared == 0 {
			t.Errorf("%s: no glyph compared", label)
		}
		t.Logf("%s: %d glyphs compared, %d of them listed as read otherwise", label, compared, diverged)
	}
}

// withinAUnit reports whether two glyphs' points are the same points, each
// coordinate within a unit of the other's.
func withinAUnit(a, b string) bool {
	pa, pb := strings.Fields(a), strings.Fields(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		var ax, ay, bx, by int
		if _, err := fmt.Sscanf(pa[i], "%d,%d", &ax, &ay); err != nil {
			return false
		}
		if _, err := fmt.Sscanf(pb[i], "%d,%d", &bx, &by); err != nil {
			return false
		}
		if ax-bx > 1 || bx-ax > 1 || ay-by > 1 || by-ay > 1 {
			return false
		}
	}
	return true
}
