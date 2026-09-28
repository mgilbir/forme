package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The MATH table, held to HarfBuzz.
//
// testdata/harfbuzz/mathtable.py asks HarfBuzz — and fontTools, which must
// agree with it — for everything each face's MATH table states: its
// constants, and for every glyph its italics correction, top accent
// attachment, extended shape, kerning, size variants and assemblies. The
// answers are checked in as mathtable.expected.txt, and here every glyph of
// every face is asked the same questions: a glyph the file says nothing about
// must answer "not stated", so a reader that invented a value for a glyph the
// font does not cover fails as surely as one that misread a value it does.
//
// The faces are MathTable.ttf, built by mathtable_fixture.py with every part
// of the table in it; the suite's math test fonts (WPT_TESTS), whose subtests
// skip or fail as every test of that corpus does; and Noto Sans Math and STIX
// Two Math from the Google Fonts library, which `make googlefonts` fetches at
// a pinned commit and which are skipped, and said so, where it has not been
// fetched.

// hbMath is one face's expectations.
type hbMath struct {
	name, sum string
	constants []int // nil for "C none"
	overlap   int
	italics   map[int]int
	accents   map[int]int
	extended  map[int]bool
	// kerns is each probe: glyph, corner, height and the kern there.
	kerns    [][4]int
	variants [2]map[int][]MathGlyphVariant // [horizontal, vertical]
	assembly [2]map[int]MathGlyphAssembly
}

func readMathGolden(t *testing.T) []*hbMath {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "mathtable.expected.txt")
	refuseUnpinnedOracle(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbmath` to generate it.", path, err)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var faces []*hbMath
	var cur *hbMath
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		atoi := func(s string) int {
			n, err := strconv.Atoi(s)
			if err != nil {
				t.Fatalf("%s:%d: %v", path, line, err)
			}
			return n
		}
		ints := func(fields []string) []int {
			out := make([]int, len(fields))
			for i, f := range fields {
				out[i] = atoi(f)
			}
			return out
		}
		kind, rest, _ := strings.Cut(text, " ")
		if kind == "face" {
			name, sum, _ := strings.Cut(rest, " ")
			cur = &hbMath{name: name, sum: sum, italics: map[int]int{}, accents: map[int]int{},
				extended: map[int]bool{},
				variants: [2]map[int][]MathGlyphVariant{{}, {}},
				assembly: [2]map[int]MathGlyphAssembly{{}, {}}}
			faces = append(faces, cur)
			continue
		}
		if cur == nil {
			t.Fatalf("%s:%d: %q before any face", path, line, kind)
		}
		fields := strings.Fields(rest)
		switch kind {
		case "C":
			if rest != "none" {
				cur.constants = ints(fields)
				if len(cur.constants) != int(MathConstantCount) {
					t.Fatalf("%s:%d: %d constants", path, line, len(cur.constants))
				}
			}
		case "O":
			cur.overlap = atoi(rest)
		case "I":
			n := ints(fields)
			cur.italics[n[0]] = n[1]
		case "A":
			n := ints(fields)
			cur.accents[n[0]] = n[1]
		case "X":
			cur.extended[atoi(rest)] = true
		case "K":
			n := ints(fields)
			cur.kerns = append(cur.kerns, [4]int{n[0], n[1], n[2], n[3]})
		case "V", "H":
			axis := 0
			if kind == "V" {
				axis = 1
			}
			var vs []MathGlyphVariant
			for _, f := range fields[1:] {
				g, a, _ := strings.Cut(f, ":")
				vs = append(vs, MathGlyphVariant{Glyph: atoi(g), Advance: atoi(a)})
			}
			cur.variants[axis][atoi(fields[0])] = vs
		case "VA", "HA":
			axis := 0
			if kind == "VA" {
				axis = 1
			}
			a := MathGlyphAssembly{ItalicsCorrection: atoi(fields[1])}
			for _, f := range fields[2:] {
				n := ints(strings.Split(f, ","))
				a.Parts = append(a.Parts, MathGlyphPart{Glyph: n[0], StartConnector: n[1],
					EndConnector: n[2], FullAdvance: n[3], Extender: n[4]&1 != 0})
			}
			cur.assembly[axis][atoi(fields[0])] = a
		default:
			t.Fatalf("%s:%d: unknown line %q", path, line, kind)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return faces
}

// mathFaceFile reads a face the expectations name, from wherever it is: the
// fixture beside the oracle, the suite's fonts, or the Google Fonts library.
func mathFaceFile(t *testing.T, name string) []byte {
	t.Helper()
	switch {
	case name == "MathTable.ttf":
		return harfbuzzFont(t, name)
	case strings.HasSuffix(name, ".woff"):
		root := os.Getenv("WPT_TESTS")
		if root == "" {
			t.Skip("set WPT_TESTS (or run `make test-wpt`) for the suite's math fonts")
		}
		data, err := os.ReadFile(filepath.Join(root, "fonts", "math", name))
		if err != nil {
			t.Fatalf("WPT_TESTS is set and %s is not in it: %v", name, err)
		}
		return data
	}
	dir := strings.ToLower(strings.TrimSuffix(name, "-Regular.ttf"))
	path := filepath.Join("..", "testdata", "googlefonts", "ofl", dir, name)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not there; `make googlefonts` fetches it", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMathTableAgreesWithHarfBuzz(t *testing.T) {
	faces := readMathGolden(t)
	if len(faces) < 3 {
		t.Fatalf("the expectations hold %d faces", len(faces))
	}
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			data := mathFaceFile(t, want.name)
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != want.sum {
				t.Fatalf("%s is not the file the expectations were generated from "+
					"(sha256 %s, want %s); regenerate with `make hbmath`", want.name, got, want.sum)
			}
			face, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			m, err := face.MathTable()
			if err != nil || m == nil {
				t.Fatalf("MathTable() = %v, %v", m, err)
			}
			checkMathTable(t, face, m, want)
		})
	}
}

func checkMathTable(t *testing.T, face *Face, m *MathTable, want *hbMath) {
	t.Helper()
	for c := MathConstant(0); c < MathConstantCount; c++ {
		got, ok := m.Constant(c)
		switch {
		case want.constants == nil && ok:
			t.Errorf("%v = %d, and the font has no MathConstants", c, got)
		case want.constants != nil && (!ok || got != want.constants[c]):
			t.Errorf("%v = %d (%v), want %d", c, got, ok, want.constants[c])
		}
	}
	if got := m.MinConnectorOverlap(); got != want.overlap {
		t.Errorf("MinConnectorOverlap = %d, want %d", got, want.overlap)
	}
	kerned := map[[2]int]bool{}
	for _, k := range want.kerns {
		kerned[[2]int{k[0], k[1]}] = true
		got, ok := m.Kern(k[0], MathKernCorner(k[1]), k[2])
		if !ok || got != k[3] {
			t.Errorf("Kern(%d, %d, %d) = %d (%v), want %d", k[0], k[1], k[2], got, ok, k[3])
		}
	}
	errs := 0
	fail := func(format string, args ...any) {
		if errs++; errs <= 20 {
			t.Errorf(format, args...)
		}
	}
	for gid := 0; gid < face.NumGlyphs(); gid++ {
		got, ok := m.ItalicsCorrection(gid)
		if v, stated := want.italics[gid]; ok != stated || got != v {
			fail("ItalicsCorrection(%d) = %d (%v), want %d (%v)", gid, got, ok, v, stated)
		}
		got, ok = m.TopAccentAttachment(gid)
		if v, stated := want.accents[gid]; ok != stated || got != v {
			fail("TopAccentAttachment(%d) = %d (%v), want %d (%v)", gid, got, ok, v, stated)
		}
		if got := m.IsExtendedShape(gid); got != want.extended[gid] {
			fail("IsExtendedShape(%d) = %v", gid, got)
		}
		for corner := MathKernTopRight; corner <= MathKernBottomLeft; corner++ {
			if kerned[[2]int{gid, int(corner)}] {
				continue
			}
			if got, ok := m.Kern(gid, corner, 0); ok {
				fail("Kern(%d, %d, 0) = %d, and the font states none there", gid, corner, got)
			}
		}
		for axis, vertical := range []bool{false, true} {
			variants, stated := want.variants[axis][gid]
			if has := m.HasConstruction(gid, vertical); has != stated {
				fail("HasConstruction(%d, %v) = %v", gid, vertical, has)
			}
			if got := m.Variants(gid, vertical); !slices.Equal(got, variants) {
				fail("Variants(%d, %v) = %v, want %v", gid, vertical, got, variants)
			}
			a, asm := want.assembly[axis][gid]
			gotA, ok := m.Assembly(gid, vertical)
			if ok != asm || gotA.ItalicsCorrection != a.ItalicsCorrection || !slices.Equal(gotA.Parts, a.Parts) {
				fail("Assembly(%d, %v) = %v (%v), want %v (%v)", gid, vertical, gotA, ok, a, asm)
			}
		}
	}
	if errs > 20 {
		t.Errorf("and %d more", errs-20)
	}
}

// TestMathTableLimitsOfTheCorpusFaces: a well-formed table of a static font
// has nothing to report, and the fixture — which gives one top accent
// attachment a variation device table — says it reads that value at the
// default instance.
func TestMathTableLimitsOfTheCorpusFaces(t *testing.T) {
	face, err := Load(harfbuzzFont(t, "MathTable.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := face.MathTable()
	if err != nil {
		t.Fatal(err)
	}
	limits := m.Limits()
	if len(limits) != 1 || !strings.Contains(limits[0], "default instance") {
		t.Errorf("Limits() = %q, want the one note about the variation device", limits)
	}
	for _, name := range []string{"NotoSansMath-Regular.ttf", "STIXTwoMath-Regular.ttf"} {
		t.Run(name, func(t *testing.T) {
			face, err := Load(mathFaceFile(t, name))
			if err != nil {
				t.Fatal(err)
			}
			m, err := face.MathTable()
			if err != nil {
				t.Fatal(err)
			}
			for gid := 0; gid < face.NumGlyphs(); gid++ {
				m.Variants(gid, true)
				m.Variants(gid, false)
				m.Assembly(gid, true)
				m.Assembly(gid, false)
			}
			if got := m.Limits(); len(got) != 0 {
				t.Errorf("Limits() = %q, want none for a well-formed table", got)
			}
		})
	}
}
