package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
)

// What a variable face states at a point of its design space besides its
// outlines, held to HarfBuzz: its font-wide metrics, which MVAR moves
// (mvar.go), and its kerning and mark anchors, which GPOS's VariationIndex
// devices move (gposvar.go).
//
// testdata/harfbuzz/instancevaried.py asks HarfBuzz, at several locations of
// four faces, for every hb_ot_metrics_tag_t it reports and for strings shaped
// across, and the answers are checked in as instancevaried.expected.txt. Two of
// the faces are built by variedlayout_fixture.py to state every MVAR tag and a
// device on every kind of GPOS record; the other two are the bundled Noto Sans
// and the Arabic face beside the fixtures. None is in a corpus, so this always
// runs.

var instanceVariedFaces = map[string]func(t *testing.T) []byte{
	"VariedLayout.ttf":      func(t *testing.T) []byte { return harfbuzzFont(t, "VariedLayout.ttf") },
	"VariedLayoutTypo.ttf":  func(t *testing.T) []byte { return harfbuzzFont(t, "VariedLayoutTypo.ttf") },
	"NotoSans-Variable.ttf": notoSansBytes,
	"NotoSansArabic.ttf":    func(t *testing.T) []byte { return harfbuzzFont(t, "NotoSansArabic.ttf") },
}

var variedFixtureStrings = []string{"AB", "BA", "CE", "DF", "G", "H", "Á́", "ﬁ́", "DE", "DEDE"}

// instanceVariedStrings are the strings instancevaried.py shapes in each face,
// in its order.
var instanceVariedStrings = map[string][]string{
	"VariedLayout.ttf":     variedFixtureStrings,
	"VariedLayoutTypo.ttf": variedFixtureStrings,
	"NotoSans-Variable.ttf": {"AVATAR Toyota WAVE", "Ta Te To Vo Yo", "é ñ ǘ ẫ q̣̂",
		"क्षत्रिय नमस्ते",
		"Αύριο Журнал"},
	"NotoSansArabic.ttf": {"مَرْحَبًا بِالْعَالَمِ",
		"لا كتاب بيت"},
}

type variedLocation struct {
	coords  map[string]float64
	metrics map[string]int
	shaped  [][]hbPosition
}

type variedFace struct {
	name, sum string
	locations []*variedLocation
}

func readInstanceVariedGolden(t *testing.T) []*variedFace {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "instancevaried.expected.txt")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v; run `make hbinstancevaried`", err)
	}
	defer file.Close()
	var faces []*variedFace
	var loc *variedLocation
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "face":
			faces = append(faces, &variedFace{name: fields[1], sum: fields[2]})
		case "location":
			loc = &variedLocation{coords: map[string]float64{}, metrics: map[string]int{}}
			for _, kv := range fields[1:] {
				k, v, _ := strings.Cut(kv, "=")
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					t.Fatalf("location %q: %v", kv, err)
				}
				loc.coords[k] = f
			}
			faces[len(faces)-1].locations = append(faces[len(faces)-1].locations, loc)
		case "metric":
			v, err := strconv.Atoi(fields[2])
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			loc.metrics[fields[1]] = v
		case "H":
			var glyphs []hbPosition
			for _, g := range fields[1:] {
				parts := strings.Split(g, ",")
				var n [5]int
				for i := range n {
					if n[i], err = strconv.Atoi(parts[i]); err != nil {
						t.Fatalf("%q: %v", g, err)
					}
				}
				glyphs = append(glyphs, hbPosition{n[0], n[1], n[2], n[3], n[4]})
			}
			loc.shaped = append(loc.shaped, glyphs)
		default:
			t.Fatalf("unknown line %q", line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(instanceVariedFaces) {
		t.Fatalf("the expectations hold %d faces, want %d", len(faces), len(instanceVariedFaces))
	}
	return faces
}

// variedMetric reads the number HarfBuzz reports for a metrics tag out of a
// static face's own tables, the way hb-ot-metrics.cc reads it: the tag's field,
// and for the horizontal ascender, descender and line gap OS/2's where the face
// sets USE_TYPO_METRICS and hhea's where it does not — with HarfBuzz's own
// fix-up of the two signs. ok is false where the face has no such field.
func variedMetric(tables map[string][]byte, tag string) (int, bool) {
	s16 := func(table string, off int) (int, bool) {
		t := tables[table]
		if off+2 > len(t) {
			return 0, false
		}
		return signed16(font.Be16(t, off)), true
	}
	u16 := func(table string, off int) (int, bool) {
		t := tables[table]
		if off+2 > len(t) {
			return 0, false
		}
		return font.Be16(t, off), true
	}
	os2 := tables["OS/2"]
	typo := len(os2) >= 64 && font.Be16(os2, 62)&0x80 != 0
	abs := func(v int, ok bool) (int, bool) {
		if v < 0 {
			v = -v
		}
		return v, ok
	}
	neg := func(v int, ok bool) (int, bool) {
		v, ok = abs(v, ok)
		return -v, ok
	}
	switch tag {
	case "HORIZONTAL_ASCENDER":
		if typo {
			return abs(s16("OS/2", 68))
		}
		return abs(s16("hhea", 4))
	case "HORIZONTAL_DESCENDER":
		if typo {
			return neg(s16("OS/2", 70))
		}
		return neg(s16("hhea", 6))
	case "HORIZONTAL_LINE_GAP":
		if typo {
			return s16("OS/2", 72)
		}
		return s16("hhea", 8)
	case "HORIZONTAL_CLIPPING_ASCENT":
		return u16("OS/2", 74)
	case "HORIZONTAL_CLIPPING_DESCENT":
		return u16("OS/2", 76)
	case "VERTICAL_ASCENDER":
		return abs(s16("vhea", 4))
	case "VERTICAL_DESCENDER":
		return neg(s16("vhea", 6))
	case "VERTICAL_LINE_GAP":
		return s16("vhea", 8)
	case "HORIZONTAL_CARET_RISE":
		return s16("hhea", 18)
	case "HORIZONTAL_CARET_RUN":
		return s16("hhea", 20)
	case "HORIZONTAL_CARET_OFFSET":
		return s16("hhea", 22)
	case "VERTICAL_CARET_RISE":
		return s16("vhea", 18)
	case "VERTICAL_CARET_RUN":
		return s16("vhea", 20)
	case "VERTICAL_CARET_OFFSET":
		return s16("vhea", 22)
	case "X_HEIGHT":
		return s16("OS/2", 86)
	case "CAP_HEIGHT":
		return s16("OS/2", 88)
	case "SUBSCRIPT_EM_X_SIZE":
		return s16("OS/2", 10)
	case "SUBSCRIPT_EM_Y_SIZE":
		return s16("OS/2", 12)
	case "SUBSCRIPT_EM_X_OFFSET":
		return s16("OS/2", 14)
	case "SUBSCRIPT_EM_Y_OFFSET":
		return s16("OS/2", 16)
	case "SUPERSCRIPT_EM_X_SIZE":
		return s16("OS/2", 18)
	case "SUPERSCRIPT_EM_Y_SIZE":
		return s16("OS/2", 20)
	case "SUPERSCRIPT_EM_X_OFFSET":
		return s16("OS/2", 22)
	case "SUPERSCRIPT_EM_Y_OFFSET":
		return s16("OS/2", 24)
	case "STRIKEOUT_SIZE":
		return s16("OS/2", 26)
	case "STRIKEOUT_OFFSET":
		return s16("OS/2", 28)
	case "UNDERLINE_SIZE":
		return s16("post", 10)
	case "UNDERLINE_OFFSET":
		return s16("post", 8)
	}
	return 0, false
}

// TestInstanceMetricsAgreeWithHarfBuzz holds an instance's font-wide numbers
// to what HarfBuzz reports for the variable face at the same location: every
// tag it reports, read from the static instance's tables as HarfBuzz would read
// them from a static face. And the Descriptor, which is what layout reads.
func TestInstanceMetricsAgreeWithHarfBuzz(t *testing.T) {
	moved := map[string]bool{}
	for _, want := range readInstanceVariedGolden(t) {
		t.Run(want.name, func(t *testing.T) {
			data := want.load(t)
			for _, loc := range want.locations {
				f, err := LoadInstance(data, loc.coords)
				if err != nil {
					t.Fatalf("%v: %v", loc.coords, err)
				}
				tables := font.SFNTTables(f.Program())
				if len(loc.metrics) == 0 {
					t.Fatalf("%v: HarfBuzz reported no metric", loc.coords)
				}
				for tag, w := range loc.metrics {
					got, ok := variedMetric(tables, tag)
					if !ok {
						t.Errorf("%v: the instance has no field for %s, which HarfBuzz reports as %d", loc.coords, tag, w)
						continue
					}
					if got != w {
						t.Errorf("%v: %s is %d, want %d", loc.coords, tag, got, w)
					}
				}
				d := f.Descriptor()
				for _, c := range []struct {
					field string
					got   int
					tag   string
				}{
					{"XHeight", d.XHeight, "X_HEIGHT"},
					{"CapHeight", d.CapHeight, "CAP_HEIGHT"},
					{"StrikeoutPosition", d.StrikeoutPosition, "STRIKEOUT_OFFSET"},
					{"StrikeoutSize", d.StrikeoutSize, "STRIKEOUT_SIZE"},
					{"UnderlinePosition", d.UnderlinePosition, "UNDERLINE_OFFSET"},
					{"UnderlineThickness", d.UnderlineThickness, "UNDERLINE_SIZE"},
				} {
					if w, ok := loc.metrics[c.tag]; ok && c.got != w {
						t.Errorf("%v: Descriptor().%s is %d, want %d", loc.coords, c.field, c.got, w)
					}
				}
				asc, desc, gap := d.Ascent, d.Descent, d.LineGap
				if d.UseTypoMetrics {
					asc, desc, gap = d.TypoAscent, d.TypoDescent, d.TypoLineGap
				}
				if asc != loc.metrics["HORIZONTAL_ASCENDER"] || desc != loc.metrics["HORIZONTAL_DESCENDER"] ||
					gap != loc.metrics["HORIZONTAL_LINE_GAP"] {
					t.Errorf("%v: the Descriptor's line is %d/%d/%d, want %d/%d/%d", loc.coords, asc, desc, gap,
						loc.metrics["HORIZONTAL_ASCENDER"], loc.metrics["HORIZONTAL_DESCENDER"],
						loc.metrics["HORIZONTAL_LINE_GAP"])
				}
			}
			// The control: at the lightest and heaviest location something
			// has to differ from the default, or the fixture tests nothing.
			base, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			baseTables := font.SFNTTables(base.Program())
			for _, loc := range want.locations {
				for tag, w := range loc.metrics {
					if v, ok := variedMetric(baseTables, tag); ok && v != w {
						moved[want.name+" "+tag] = true
					}
				}
			}
		})
	}
	// Every tag MVAR can move, moved in the fixture: a field written to the
	// wrong offset would otherwise pass wherever the delta happened to be zero.
	for _, tag := range []string{"HORIZONTAL_ASCENDER", "HORIZONTAL_DESCENDER", "HORIZONTAL_LINE_GAP",
		"HORIZONTAL_CLIPPING_ASCENT", "HORIZONTAL_CLIPPING_DESCENT", "VERTICAL_ASCENDER",
		"VERTICAL_DESCENDER", "VERTICAL_LINE_GAP", "HORIZONTAL_CARET_RISE", "HORIZONTAL_CARET_RUN",
		"HORIZONTAL_CARET_OFFSET", "VERTICAL_CARET_RISE", "VERTICAL_CARET_RUN", "VERTICAL_CARET_OFFSET",
		"X_HEIGHT", "CAP_HEIGHT", "SUBSCRIPT_EM_X_SIZE", "SUBSCRIPT_EM_Y_SIZE", "SUBSCRIPT_EM_X_OFFSET",
		"SUBSCRIPT_EM_Y_OFFSET", "SUPERSCRIPT_EM_X_SIZE", "SUPERSCRIPT_EM_Y_SIZE", "SUPERSCRIPT_EM_X_OFFSET",
		"SUPERSCRIPT_EM_Y_OFFSET", "STRIKEOUT_SIZE", "STRIKEOUT_OFFSET", "UNDERLINE_SIZE", "UNDERLINE_OFFSET"} {
		for _, face := range []string{"VariedLayout.ttf", "VariedLayoutTypo.ttf"} {
			if !moved[face+" "+tag] {
				t.Errorf("%s: %s is the default's at every location, so nothing here checks it", face, tag)
			}
		}
	}
	for _, tag := range []string{"X_HEIGHT", "STRIKEOUT_OFFSET"} {
		if !moved["NotoSans-Variable.ttf "+tag] {
			t.Errorf("Noto Sans: %s does not move, and its MVAR says it does", tag)
		}
	}
}

// TestInstancePositioningAgreesWithHarfBuzz holds strings shaped in an
// instance to HarfBuzz shaping them in the variable face at the same location:
// the kerning and the mark and cursive attachments, which a VariationIndex
// moves, as well as the advances.
func TestInstancePositioningAgreesWithHarfBuzz(t *testing.T) {
	varied := 0
	for _, want := range readInstanceVariedGolden(t) {
		t.Run(want.name, func(t *testing.T) {
			data := want.load(t)
			strs := instanceVariedStrings[want.name]
			var atDefault [][]hbPosition
			for _, loc := range want.locations {
				if len(loc.shaped) != len(strs) {
					t.Fatalf("%v: %d shaped lines for %d strings", loc.coords, len(loc.shaped), len(strs))
				}
				f, err := LoadInstance(data, loc.coords)
				if err != nil {
					t.Fatal(err)
				}
				for i, s := range strs {
					got, _ := f.ShapeGlyphs(s)
					w := loc.shaped[i]
					if len(got) != len(w) {
						t.Errorf("%v %s: %d glyphs, want %d", loc.coords, describeRunes(s), len(got), len(w))
						continue
					}
					for k, g := range got {
						p := hbPosition{g.GID, f.units(g.XAdvance), f.units(g.YAdvance), f.units(g.XOffset), f.units(g.YOffset)}
						if p != w[k] {
							t.Errorf("%v %s: glyph %d is %v, want %v", loc.coords, describeRunes(s), k, p, w[k])
						}
					}
				}
				if atDefault == nil {
					atDefault = loc.shaped
					continue
				}
				for i := range loc.shaped {
					if !sameHBRun(atDefault[i], loc.shaped[i]) {
						varied++
					}
				}
			}
		})
	}
	if varied == 0 {
		t.Fatal("no string is placed differently at any two locations, so nothing here checks a device")
	}
}

func sameHBRun(a, b []hbPosition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (want *variedFace) load(t *testing.T) []byte {
	t.Helper()
	data := instanceVariedFaces[want.name](t)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbinstancevaried` to regenerate them.", want.name, want.sum, got)
	}
	return data
}

// TestInstanceKernsAcrossARunBoundaryAtItsLocation is the same pairs cut into
// two runs, each shaped with the other as its context: the pair across the
// boundary is looked up by boundarykern.go through the flat pair reading, not
// by the positioning pass, and it has to be varied the same way or a pair set
// in two spans kerns by the default instance's amount.
func TestInstanceKernsAcrossARunBoundaryAtItsLocation(t *testing.T) {
	for _, want := range readInstanceVariedGolden(t) {
		if want.name != "VariedLayout.ttf" {
			continue
		}
		data := want.load(t)
		for _, loc := range want.locations {
			f, err := LoadInstance(data, loc.coords)
			if err != nil {
				t.Fatal(err)
			}
			// The pairs of the fixture: listed ("AB", "BA") and by class ("CE",
			// "DF"). Their places in variedFixtureStrings are their places in
			// the expectations.
			for i, s := range variedFixtureStrings[:4] {
				w := loc.shaped[i]
				first, second := s[:1], s[1:]
				a, _ := f.ShapeGlyphsInContext(first, "", second, Features{})
				b, _ := f.ShapeGlyphsInContext(second, first, "", Features{})
				if len(a) != 1 || len(b) != 1 || len(w) != 2 {
					t.Fatalf("%q: %d and %d glyphs, want 1 and 1 of %d", s, len(a), len(b), len(w))
				}
				for k, g := range []Glyph{a[0], b[0]} {
					p := hbPosition{g.GID, f.units(g.XAdvance), f.units(g.YAdvance), f.units(g.XOffset), f.units(g.YOffset)}
					if p != w[k] {
						t.Errorf("%v %q cut in two: glyph %d is %v, want %v", loc.coords, s, k, p, w[k])
					}
				}
			}
		}
	}
}
