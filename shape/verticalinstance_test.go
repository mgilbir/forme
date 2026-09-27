package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// A variable face set upright away from its default instance, and a kern
// table's vertical subtables, held to HarfBuzz.
//
// testdata/harfbuzz/verticalinstance.py asks HarfBuzz, at several weights of
// three variable faces, for each glyph's vertical advance and origin and for a
// few strings shaped top to bottom; and shapes a few strings in a face whose
// kern table kerns down the page. The answers are checked in as
// verticalinstance.expected.txt. Two of the variable faces and the kerned one
// are built by verticalinstance_fixture.py; Noto Sans JP's variable face is in
// the corpora (NOTO_FONTS).

var verticalInstanceFaces = map[string]func(t *testing.T) []byte{
	"VerticalVariable.ttf":       func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalVariable.ttf") },
	"VerticalVariableNoVVAR.ttf": func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalVariableNoVVAR.ttf") },
	"NotoSansJP-VF.ttf":          func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSansJP-VF.ttf") },
	"VerticalKern.ttf":           func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalKern.ttf") },
	"VerticalKernNoVkrn.ttf":     func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalKernNoVkrn.ttf") },
}

// verticalInstanceStrings are the strings verticalinstance.py shapes in each
// face, in its order.
var verticalInstanceStrings = map[string][]string{
	"VerticalVariable.ttf":       {"ABD", "A B", "CEF"},
	"VerticalVariableNoVVAR.ttf": {"ABD", "A B", "CEF"},
	"NotoSansJP-VF.ttf":          {"\u65E5\u672C\u8A9E", "\u3042\u3001\u3044\u3002", "\uFF08\u6F22\u5B57\uFF09"},
	"VerticalKern.ttf":           {"ABCA", "CAB", "AB BC", "BCBC", "A\u0301B"},
	"VerticalKernNoVkrn.ttf":     {"ABCA", "CAB", "AB BC", "BCBC", "A\u0301B"},
}

// takesComponentMetrics are the glyphs of each face that take their metrics
// from a component (USE_MY_METRICS), whose own phantom points gvar moves
// differently from the component's: HarfBuzz gives such a glyph the
// component's off the default, and fontTools' instancer its own, and this
// package follows HarfBuzz (see the note at the top of instance.go). Each is
// compared with HarfBuzz like every other glyph; the entry says the face holds
// the case, and the test requires that it does — a composite so flagged,
// compared somewhere off the default.
var takesComponentMetrics = map[string]map[int]bool{
	"VerticalVariable.ttf":       {4: true, 6: true, 7: true},
	"VerticalVariableNoVVAR.ttf": {4: true, 6: true, 7: true},
}

// hbLocation is one location's expectations for a face: the weight, or zero
// for a static face, each sampled glyph's vertical metrics, and the strings
// shaped, each with the feature it was shaped with.
type hbLocation struct {
	weight  float64
	metrics map[int][3]int
	shaped  []hbShaped
}

type hbShaped struct {
	upright bool
	tags    string
	glyphs  []hbPosition
}

type hbInstanceFace struct {
	name, sum string
	locations []*hbLocation
}

func readVerticalInstanceGolden(t *testing.T) []*hbInstanceFace {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "verticalinstance.expected.txt")
	refuseUnpinnedOracle(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbverticalinstance` to generate it.", path, err)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var faces []*hbInstanceFace
	var face *hbInstanceFace
	var loc *hbLocation
	atoi := func(line int, s string) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("%s:%d: %v", path, line, err)
		}
		return v
	}
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		kind, rest, _ := strings.Cut(text, " ")
		switch kind {
		case "face":
			name, sum, _ := strings.Cut(rest, " ")
			face = &hbInstanceFace{name: name, sum: sum}
			faces = append(faces, face)
			continue
		case "location":
			if face == nil {
				t.Fatalf("%s:%d: a location before any face", path, line)
			}
			loc = &hbLocation{metrics: map[int][3]int{}}
			if w, ok := strings.CutPrefix(rest, "wght="); ok {
				loc.weight = float64(atoi(line, w))
			}
			face.locations = append(face.locations, loc)
			continue
		}
		if loc == nil {
			t.Fatalf("%s:%d: %q before any location", path, line, kind)
		}
		fields := strings.Fields(rest)
		switch kind {
		case "M":
			if len(fields) != 4 {
				t.Fatalf("%s:%d: %q", path, line, text)
			}
			loc.metrics[atoi(line, fields[0])] = [3]int{atoi(line, fields[1]), atoi(line, fields[2]), atoi(line, fields[3])}
		case "V", "H":
			s := hbShaped{upright: kind == "V", tags: fields[0]}
			if s.tags == "-" {
				s.tags = ""
			}
			for _, f := range fields[1:] {
				parts := strings.Split(f, ",")
				if len(parts) != 5 {
					t.Fatalf("%s:%d: %q has %d parts, want 5", path, line, f, len(parts))
				}
				var n [5]int
				for i, p := range parts {
					n[i] = atoi(line, p)
				}
				s.glyphs = append(s.glyphs, hbPosition{n[0], n[1], n[2], n[3], n[4]})
			}
			loc.shaped = append(loc.shaped, s)
		default:
			t.Fatalf("%s:%d: unknown line %q", path, line, kind)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(verticalInstanceFaces) {
		t.Fatalf("%d faces in %s and %d here to read them", len(faces), path, len(verticalInstanceFaces))
	}
	return faces
}

// loadAt loads a face at a location's weight, or as it is for a static one,
// having checked it is the face the expectations were generated from.
func (want *hbInstanceFace) loadAt(t *testing.T, loc *hbLocation) *Face {
	t.Helper()
	data := verticalInstanceFaces[want.name](t)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbverticalinstance` to regenerate them.", want.name, want.sum, got)
	}
	var f *Face
	var err error
	if loc.weight == 0 {
		f, err = Load(data)
	} else {
		f, err = LoadInstance(data, map[string]float64{"wght": loc.weight})
	}
	if err != nil {
		t.Fatalf("loading %s: %v", want.name, err)
	}
	return f
}

// checkLocationShaped holds a location's shaped strings to HarfBuzz's.
func checkLocationShaped(t *testing.T, f *Face, loc *hbLocation, strs []string) {
	t.Helper()
	perString := len(loc.shaped) / len(strs)
	if perString == 0 || perString*len(strs) != len(loc.shaped) {
		t.Fatalf("%d shaped lines for %d strings", len(loc.shaped), len(strs))
	}
	for i, want := range loc.shaped {
		s := strs[i/perString]
		off := Features{Tags: want.tags, Vertical: want.upright}
		if want.tags == "nokern" {
			off = Features{NoKerning: true, Vertical: want.upright}
		}
		got, _ := f.ShapeGlyphsInContext(s, "", "", off)
		if want.upright {
			if same, why := sameUpright(f, got, want.glyphs); !same {
				t.Errorf("%s %q down the page: %s", describeRunes(s), want.tags, why)
			}
			continue
		}
		if len(got) != len(want.glyphs) {
			t.Errorf("%s across: %d glyphs, want %d", describeRunes(s), len(got), len(want.glyphs))
			continue
		}
		for k, g := range got {
			w := want.glyphs[k]
			if g.GID != w.gid || f.units(g.XAdvance) != w.xAdvance || f.units(g.XOffset) != w.dx || f.units(g.YOffset) != w.dy {
				t.Errorf("%s across: glyph %d is %d advancing %d at (%d, %d), want %d advancing %d at (%d, %d)",
					describeRunes(s), k, g.GID, f.units(g.XAdvance), f.units(g.XOffset), f.units(g.YOffset),
					w.gid, w.xAdvance, w.dx, w.dy)
			}
		}
	}
}

// TestInstancedVerticalMetricsAgreeWithHarfBuzz holds a face from
// LoadInstance to HarfBuzz at the same location: each sampled glyph's vertical
// advance — VVAR's, or the phantom points' — and where it is hung, and strings
// set upright.
func TestInstancedVerticalMetricsAgreeWithHarfBuzz(t *testing.T) {
	for _, want := range readVerticalInstanceGolden(t) {
		if strings.HasPrefix(want.name, "VerticalKern") {
			continue
		}
		t.Run(want.name, func(t *testing.T) {
			if len(want.locations) < 2 {
				t.Fatal("fewer than two locations, so no instance is compared")
			}
			offDefault := map[int]bool{}
			for _, loc := range want.locations {
				f := want.loadAt(t, loc)
				if len(loc.metrics) == 0 {
					t.Fatalf("wght %v: no glyph was compared", loc.weight)
				}
				for gid, m := range loc.metrics {
					advance, x, y := f.verticalUnits(gid)
					same := advance == m[0] && x == m[1] && y == m[2]
					if !same {
						t.Errorf("wght %v: glyph %d advances %d hung (%d, %d), want %d hung (%d, %d)",
							loc.weight, gid, advance, x, y, m[0], m[1], m[2])
					}
					if takesComponentMetrics[want.name][gid] && loc.weight != 100 {
						offDefault[gid] = true
					}
				}
				checkLocationShaped(t, f, loc, verticalInstanceStrings[want.name])
			}
			f := want.loadAt(t, want.locations[0])
			for gid := range takesComponentMetrics[want.name] {
				if !offDefault[gid] {
					t.Errorf("glyph %d, listed in takesComponentMetrics, is not compared off the default", gid)
				}
				g, _ := f.vert.glyfBytes(gid)
				flagged := false
				if len(g) >= 10 && signed16(font.Be16(g, 0)) < 0 {
					eachComponent(g, func(flags, _ int) bool {
						flagged = flagged || flags&compUseMyMetrics != 0
						return true
					})
				}
				if !flagged {
					t.Errorf("glyph %d, listed in takesComponentMetrics, has no component whose metrics it takes", gid)
				}
			}
		})
	}
}

// TestVerticalKernAgreesWithHarfBuzz holds two faces whose kern table kerns
// down the page, across it and along a horizontal line to HarfBuzz: the
// vertical subtables apply to a run set upright only where 'vkrn' is asked for
// and the face's layout tables state it, which one face does and the other
// does not, and the horizontal one only across the page. And a mark the face
// does not position is left where it is while the table kerns across the line,
// whether or not kerning is asked for.
func TestVerticalKernAgreesWithHarfBuzz(t *testing.T) {
	kerned := 0
	for _, want := range readVerticalInstanceGolden(t) {
		if !strings.HasPrefix(want.name, "VerticalKern") {
			continue
		}
		t.Run(want.name, func(t *testing.T) {
			f := want.loadAt(t, want.locations[0])
			checkLocationShaped(t, f, want.locations[0], verticalInstanceStrings[want.name])
		})
		kerned++
	}
	if kerned != 2 {
		t.Fatalf("the expectations hold %d kerned faces, want 2", kerned)
	}
}

// TestInstancedVmtxStatesHowManyAdvancesItHolds: an instance's vmtx drops the
// advances that repeat the last one, as the format allows, and vhea has to say
// how many it kept, or a reader takes bearings for advances. The faces above
// keep every advance at every location, so this asks the writer directly.
func TestInstancedVmtxStatesHowManyAdvancesItHolds(t *testing.T) {
	vhea := make([]byte, 36)
	binary.BigEndian.PutUint16(vhea[34:], 4)
	advances := []int{500, 700, 600, 600}
	tops := []int{880, 880, 880, 880}
	bounds := []glyphBounds{{empty: true}, {0, 0, 100, 800, false}, {0, 0, 100, 700, false}, {0, 0, 100, 600, false}}
	vmtx, out := buildVerticalMetrics(vhea, advances, tops, bounds)
	if got := int(binary.BigEndian.Uint16(out[34:])); got != 3 {
		t.Fatalf("vhea says %d advances, and the last two are alike so vmtx holds 3", got)
	}
	if len(vmtx) != 4*3+2 {
		t.Fatalf("vmtx is %d bytes, want %d", len(vmtx), 4*3+2)
	}
	if tsb := int16(binary.BigEndian.Uint16(vmtx[12:])); tsb != 880-600 {
		t.Errorf("the last glyph's top side bearing is %d, want %d", tsb, 880-600)
	}
}
