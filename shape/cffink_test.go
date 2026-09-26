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
	"github.com/mgilbir/forme/fonttest"
)

// The ink of a CFF glyph, held to HarfBuzz.
//
// testdata/harfbuzz/cffink.py asks HarfBuzz for the extents of every glyph of
// CFFInk.otf — built by cffink_fixture.py, a glyph for each thing a charstring
// can say — and of a sample of every CFF face in the corpora, and shapes a few
// strings in the fixture across the page and down it, where HarfBuzz places
// the marks and hangs the glyphs by their ink. The answers are checked in as
// cffink.expected.txt.
//
// The corpus faces are the CID-keyed Noto CJK faces (NOTO_CJK) and Unifont
// (NOTO_FONTS); their subtests skip or fail as every test of those corpora
// does.

// cffInkFaces reads each face the expectations were generated from.
var cffInkFaces = map[string]func(t *testing.T) []byte{
	"CFFInk.otf":               func(t *testing.T) []byte { return harfbuzzFont(t, "CFFInk.otf") },
	"Unifont-Regular.otf":      func(t *testing.T) []byte { return fonttest.NotoFile(t, "Unifont-Regular.otf") },
	"UnifontUpper-Regular.otf": func(t *testing.T) []byte { return fonttest.NotoFile(t, "UnifontUpper-Regular.otf") },
	"NotoSansJP-Regular.otf":   func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansJP-Regular.otf") },
	"NotoSansKR-Regular.otf":   func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansKR-Regular.otf") },
	"NotoSansSC-Regular.otf":   func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansSC-Regular.otf") },
	"NotoSansTC-Regular.otf":   func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansTC-Regular.otf") },
	"NotoSansHK-Regular.otf":   func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansHK-Regular.otf") },
	"NotoSerifJP-Regular.otf":  func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSerifJP-Regular.otf") },
}

// cffInkStrings are the strings cffink.py shapes in the fixture, in its order.
var cffInkStrings = []string{
	"A\u0301",
	"A\u0323",
	"A\u0323\u0301",
	"A\u0301\u0301",
	"B\u0300\u0301",
	"H\u0327",
	"H\u0328",
	"A\u0328\u0301",
	"AB",
	"HAB\u0301H",
}

// hbInk is one face's expectations: each glyph's extents, or none where
// HarfBuzz has none, and for the fixture the strings shaped and its glyphs'
// vertical metrics.
type hbInk struct {
	name, sum       string
	extents         map[int]*extents
	across, down    [][]hbPosition
	verticalByGlyph map[int][3]int
}

func readCFFInkGolden(t *testing.T) []*hbInk {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "cffink.expected.txt")
	refuseUnpinnedOracle(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbcffink` to generate it.", path, err)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var faces []*hbInk
	var cur *hbInk
	ints := func(line int, fields []string) []int {
		out := make([]int, len(fields))
		for i, f := range fields {
			if out[i], err = strconv.Atoi(f); err != nil {
				t.Fatalf("%s:%d: %v", path, line, err)
			}
		}
		return out
	}
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		kind, rest, _ := strings.Cut(text, " ")
		if kind == "face" {
			name, sum, _ := strings.Cut(rest, " ")
			cur = &hbInk{name: name, sum: sum, extents: map[int]*extents{}, verticalByGlyph: map[int][3]int{}}
			faces = append(faces, cur)
			continue
		}
		if cur == nil {
			t.Fatalf("%s:%d: %q before any face", path, line, kind)
		}
		fields := strings.Fields(rest)
		switch kind {
		case "E":
			if len(fields) == 2 && fields[1] == "none" {
				cur.extents[ints(line, fields[:1])[0]] = nil
				continue
			}
			if len(fields) != 5 {
				t.Fatalf("%s:%d: %q", path, line, text)
			}
			n := ints(line, fields)
			cur.extents[n[0]] = &extents{xBearing: n[1], yBearing: n[2], width: n[3], height: n[4]}
		case "M":
			n := ints(line, fields)
			if len(n) != 4 {
				t.Fatalf("%s:%d: %q", path, line, text)
			}
			cur.verticalByGlyph[n[0]] = [3]int{n[1], n[2], n[3]}
		case "H", "V":
			var run []hbPosition
			for _, field := range fields {
				n := ints(line, strings.Split(field, ","))
				if len(n) != 5 {
					t.Fatalf("%s:%d: %q has %d parts, want 5", path, line, field, len(n))
				}
				run = append(run, hbPosition{n[0], n[1], n[2], n[3], n[4]})
			}
			if kind == "H" {
				cur.across = append(cur.across, run)
			} else {
				cur.down = append(cur.down, run)
			}
		default:
			t.Fatalf("%s:%d: unknown line %q", path, line, kind)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(cffInkFaces) {
		t.Fatalf("%d faces in %s and %d here to read them", len(faces), path, len(cffInkFaces))
	}
	return faces
}

func loadCFFInkFace(t *testing.T, want *hbInk) *Face {
	t.Helper()
	read, ok := cffInkFaces[want.name]
	if !ok {
		t.Fatalf("the expectations name %s, which this test does not know how to read", want.name)
	}
	data := read(t)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbcffink` to regenerate them.", want.name, want.sum, got)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading %s: %v", want.name, err)
	}
	if !f.IsCFF() {
		t.Fatalf("%s did not load as a CFF face", want.name)
	}
	return f
}

// TestCFFInkAgreesWithHarfBuzz holds each glyph's extents to what HarfBuzz
// answers for it, including where it answers that there are none.
func TestCFFInkAgreesWithHarfBuzz(t *testing.T) {
	for _, want := range readCFFInkGolden(t) {
		t.Run(want.name, func(t *testing.T) {
			f := loadCFFInkFace(t, want)
			if len(want.extents) == 0 {
				t.Fatal("no glyph was compared")
			}
			for gid, w := range want.extents {
				got, ok := f.glyphExtents(gid)
				switch {
				case w == nil && ok:
					t.Errorf("glyph %d has extents %+v, and HarfBuzz has none", gid, got)
				case w != nil && !ok:
					t.Errorf("glyph %d has no extents, and HarfBuzz's are %+v", gid, *w)
				case w != nil && got != *w:
					t.Errorf("glyph %d has extents %+v, want %+v", gid, got, *w)
				}
			}
		})
	}
}

// TestCFFInkPlacesTheFixturesMarksAsHarfBuzzDoes shapes the fixture's strings,
// whose marks the face does not position and whose glyphs it states no
// vertical metrics for: across the page each mark is placed against its
// base's ink, and down it each glyph is hung by its own.
func TestCFFInkPlacesTheFixturesMarksAsHarfBuzzDoes(t *testing.T) {
	var want *hbInk
	for _, f := range readCFFInkGolden(t) {
		if f.name == "CFFInk.otf" {
			want = f
		}
	}
	if want == nil || len(want.across) != len(cffInkStrings) || len(want.down) != len(cffInkStrings) {
		t.Fatalf("the fixture's expectations do not hold the %d strings shaped both ways", len(cffInkStrings))
	}
	f := loadCFFInkFace(t, want)
	for i, s := range cffInkStrings {
		glyphs, _ := f.ShapeGlyphs(s)
		if len(glyphs) != len(want.across[i]) {
			t.Errorf("%s: %d glyphs across, want %d", describeRunes(s), len(glyphs), len(want.across[i]))
			continue
		}
		for k, g := range glyphs {
			w := want.across[i][k]
			if g.GID != w.gid || f.units(g.XAdvance) != w.xAdvance ||
				f.units(g.XOffset) != w.dx || f.units(g.YOffset) != w.dy {
				t.Errorf("%s: glyph %d is %d advancing %d at (%d, %d), want %d advancing %d at (%d, %d)",
					describeRunes(s), k, g.GID, f.units(g.XAdvance), f.units(g.XOffset),
					f.units(g.YOffset), w.gid, w.xAdvance, w.dx, w.dy)
			}
		}
		down, _ := f.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
		if same, why := sameUpright(f, down, want.down[i]); !same {
			t.Errorf("%s down the page: %s", describeRunes(s), why)
		}
	}
	for gid, m := range want.verticalByGlyph {
		advance, x, y := f.verticalUnits(gid)
		if advance != m[0] || x != m[1] || y != m[2] {
			t.Errorf("glyph %d advances %d hung (%d, %d), want %d hung (%d, %d)",
				gid, advance, x, y, m[0], m[1], m[2])
		}
	}
}

// TestCFFInkRunsEachGlyphOnce: a glyph's charstring is run the first time its
// ink is asked for and never again, by the face or by a clone of it, so that
// the cost of measuring is the glyphs a document uses and not the times it
// uses them. The count of operators charged is the measure, not a clock.
func TestCFFInkRunsEachGlyphOnce(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "CFFInk.otf"))
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := f.GlyphID('B') // a seac: its two components are run with it
	if !ok {
		t.Fatal("the fixture has no B")
	}
	first, ok := f.glyphExtents(gid)
	if !ok {
		t.Fatal("B has no extents")
	}
	once := f.ink.budget.Spent()
	if once == 0 {
		t.Fatal("measuring B charged nothing, so this test measures nothing")
	}
	clone := f.Clone()
	for range 100 {
		if again, _ := f.glyphExtents(gid); again != first {
			t.Fatalf("B measured again is %+v, and was %+v", again, first)
		}
		clone.glyphExtents(gid)
		f.InkExtent("BBBB", 12)
	}
	if spent := f.ink.budget.Spent(); spent != once {
		t.Errorf("asking for B's ink 300 more times cost %d operators on top of the %d "+
			"the first time did; it should cost none", spent-once, once)
	}
}

// TestCFFInkWorkIsBoundedAndReported builds a face whose every glyph runs to
// HarfBuzz's cap on one charstring, and measures more of them than the face's
// budget allows. The glyphs up to the budget are capped, as HarfBuzz caps
// them; the rest, and a glyph that would have had ink, have none; and both are
// reported, since the second is a place this differs from HarfBuzz.
func TestCFFInkWorkIsBoundedAndReported(t *testing.T) {
	const loops = 40
	// 0 0 rmoveto and nothing after it: the charstring runs out and reads the
	// end until the cap, which is the most one glyph can cost without an
	// error ending it sooner.
	spin := []byte{139, 139, 21}
	// 0 0 rmoveto 100 100 rlineto endchar.
	line := []byte{139, 139, 21, 239, 239, 5, 14}
	charstrings := [][]byte{{14}}
	var glyphs []fonttest.Glyph
	for i := range loops {
		charstrings = append(charstrings, spin)
		glyphs = append(glyphs, fonttest.Glyph{Rune: 'a' + rune(i), Advance: 500, HasShape: true})
	}
	charstrings = append(charstrings, line)
	glyphs = append(glyphs, fonttest.Glyph{Rune: 'Z', Advance: 500, HasShape: true})
	cff := fonttest.CFF(fonttest.CFFOptions{Glyphs: len(charstrings), Charstrings: charstrings})
	f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatal(err)
	}
	lineGID := len(charstrings) - 1

	// The line alone has ink, before anything has spent the budget.
	fresh, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := fresh.glyphExtents(lineGID); !ok || e != (extents{0, 100, 100, -100}) {
		t.Fatalf("the line glyph measures %+v, %v, want (0, 100, 100, -100)", e, ok)
	}
	if limits := fresh.LayoutLimits(); len(limits) != 0 {
		t.Fatalf("a face that ran into nothing reports %q", limits)
	}

	for gid := 1; gid <= loops; gid++ {
		if e, ok := f.glyphExtents(gid); ok {
			t.Fatalf("glyph %d, which never ends, has extents %+v", gid, e)
		}
	}
	if e, ok := f.glyphExtents(lineGID); ok {
		t.Errorf("the line glyph, measured after the budget ran out, has extents %+v", e)
	}
	budget := cffInkWork(len(cff))
	if spent := f.ink.budget.Spent(); spent > budget {
		t.Errorf("measuring spent %d operators, past the budget of %d", spent, budget)
	}
	if loops*cffMaxOps <= budget {
		t.Fatalf("%d glyphs at the cap is %d operators, within the budget of %d: "+
			"the test does not reach the bound it is about", loops, loops*cffMaxOps, budget)
	}
	limits := strings.Join(f.LayoutLimits(), "\n")
	for _, want := range []string{"200,000 operators", "ran past the work one face may cost"} {
		if !strings.Contains(limits, want) {
			t.Errorf("LayoutLimits does not say %q; it says %q", want, limits)
		}
	}
}

// TestStandardEncodingSIDsAreHarfBuzzs holds the SID each StandardEncoding code
// names, which a seac's two codes are read through, to HarfBuzz's own table
// (standard_encoding_to_sid in hb-ot-cff1-table.cc, 14.5.0).
func TestStandardEncodingSIDsAreHarfBuzzs(t *testing.T) {
	harfbuzz := [256]int{
		32: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
		24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45,
		46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67,
		68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89,
		90, 91, 92, 93, 94, 95,
		161: 96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110,
		177: 111, 112, 113, 114,
		182: 115, 116, 117, 118, 119, 120, 121, 122,
		191: 123,
		193: 124, 125, 126, 127, 128, 129, 130, 131,
		202: 132, 133,
		205: 134, 135, 136, 137,
		225: 138,
		227: 139,
		232: 140, 141, 142, 143,
		241: 144,
		245: 145,
		248: 146, 147, 148, 149,
	}
	for code := range 256 {
		if got := standardEncodingSID(byte(code)); got != harfbuzz[code] {
			t.Errorf("StandardEncoding code %d names SID %d, and HarfBuzz reads %d", code, got, harfbuzz[code])
		}
	}
}

// TestInkExtentReadsACFFFacesGlyphs: InkExtent answers for a CFF face from
// the boxes its charstrings draw, where it used to answer that it could not
// say — which sent every caller to the face's ascent and descent.
func TestInkExtentReadsACFFFacesGlyphs(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "CFFInk.otf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		s            string
		above, below float64
	}{
		// A is drawn from the baseline to 600; Á is A with the acute put 250
		// above it by a seac, reaching 900; H is drawn from 20 to 140, so it
		// reaches below the baseline by minus twenty.
		{"A", 600, 0},
		{"Á", 900, 0},
		{"H", 140, -20},
		{"AHÁ", 900, 0},
	} {
		above, below, ok := f.InkExtent(c.s, 1000)
		if !ok || above != c.above || below != c.below {
			t.Errorf("%s reaches %v above and %v below, %v; want %v and %v",
				describeRunes(c.s), above, below, ok, c.above, c.below)
		}
	}
	// A glyph whose charstring cannot be run has no ink to state, and a run
	// holding one has no extent this can report.
	if _, _, ok := f.InkExtent("Ą", 1000); ok {
		t.Error("a run holding a glyph with no ink reports an extent")
	}
}

// cidCFFOmitting is a CID-keyed CFF of two glyphs, .notdef and a line, with
// one Font DICT, and its charset, FDArray and FDSelect each left out on
// request: the parts a CID-keyed font cannot be measured without. fonttest's
// CFF names neither of the last two.
func cidCFFOmitting(charset, fdArray, fdSelect bool) []byte {
	line := []byte{139, 139, 21, 239, 239, 5, 14} // 0 0 rmoveto 100 100 rlineto endchar
	op5 := func(v int) []byte { return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	top := func(charsetOff, charStrings, fdArrayOff, fdSelectOff int) []byte {
		var d []byte
		d = append(d, op5(0)...)
		d = append(d, op5(0)...)
		d = append(d, op5(0)...)
		d = append(d, 12, 30) // ROS
		if charset {
			d = append(d, op5(charsetOff)...)
			d = append(d, 15)
		}
		d = append(d, op5(charStrings)...)
		d = append(d, 17)
		if fdArray {
			d = append(d, op5(fdArrayOff)...)
			d = append(d, 12, 36)
		}
		if fdSelect {
			d = append(d, op5(fdSelectOff)...)
			d = append(d, 12, 37)
		}
		return d
	}
	build := func(charsetOff, charStrings, fdArrayOff, fdSelectOff int) []byte {
		out := []byte{1, 0, 4, 4}
		out = append(out, writeCFFIndex([][]byte{[]byte("T")})...)
		out = append(out, writeCFFIndex([][]byte{top(charsetOff, charStrings, fdArrayOff, fdSelectOff)})...)
		out = append(out, 0, 0, 0, 0) // no strings, no global subroutines
		return out
	}
	head := len(build(0, 0, 0, 0))
	charsetOff := head
	body := []byte{0, 0, 1} // format 0: glyph 1 is CID 1
	fdSelectOff := head + len(body)
	body = append(body, 3, 0, 1, 0, 0, 0, 0, 2) // format 3: one range, glyphs 0 on in Font DICT 0
	fdArrayOff := head + len(body)
	body = append(body, writeCFFIndex([][]byte{{139, 139, 18}})...) // a Private DICT of no bytes
	charStrings := head + len(body)
	body = append(body, writeCFFIndex([][]byte{{14}, line})...)
	return append(build(charsetOff, charStrings, fdArrayOff, fdSelectOff), body...)
}

// TestACFFTheAcceleratorRefusesHasNoInk: a CFF table whose glyph count is not
// the font's is one HarfBuzz's accelerator refuses outright, measuring none of
// its glyphs, and so is a CID-keyed one with no charset of its own, no FDArray
// or no FDSelect.
func TestACFFTheAcceleratorRefusesHasNoInk(t *testing.T) {
	for name, cff := range map[string][]byte{
		"a CID-keyed CFF":                  cidCFFOmitting(true, true, true),
		"a CID-keyed CFF with no charset":  cidCFFOmitting(false, true, true),
		"a CID-keyed CFF with no FDArray":  cidCFFOmitting(true, false, true),
		"a CID-keyed CFF with no FDSelect": cidCFFOmitting(true, true, false),
	} {
		o, err := readCFFOutlines(cff, 2)
		if refused := o == nil; refused != (name != "a CID-keyed CFF") {
			t.Errorf("%s: refused %v (%v)", name, refused, err)
			continue
		}
		if o == nil {
			continue
		}
		r := t2Run{o: o, budget: font.NewBudget(maxFontWork)}
		if b, ok := r.bounds(1, false); !ok || b.extents() != (extents{0, 100, 100, -100}) {
			t.Errorf("%s: glyph 1 measures %+v, %v, want (0, 100, 100, -100)", name, b.extents(), ok)
		}
	}

	line := []byte{139, 139, 21, 239, 239, 5, 14} // 0 0 rmoveto 100 100 rlineto endchar
	glyphs := []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}
	good := fonttest.CFF(fonttest.CFFOptions{Glyphs: 2, Charstrings: [][]byte{{14}, line}})
	f, err := Load(fonttest.OTTO(good, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.glyphExtents(1); !ok {
		t.Fatal("the control case, a well-formed CFF, has no ink")
	}
	for name, cff := range map[string][]byte{
		"a glyph more than maxp": fonttest.CFF(fonttest.CFFOptions{Glyphs: 3,
			Charstrings: [][]byte{{14}, line, line}}),
		"CID-keyed with neither FDArray nor FDSelect": fonttest.CFF(fonttest.CFFOptions{Glyphs: 2,
			CIDKeyed: true, CharsetSIDs: []int{1}, Charstrings: [][]byte{{14}, line}}),
	} {
		f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if e, ok := f.glyphExtents(1); ok {
			t.Errorf("%s: glyph 1 has extents %+v", name, e)
		}
	}
}
