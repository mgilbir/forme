package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A run set upright, held to HarfBuzz shaping it top to bottom.
//
// testdata/harfbuzz/vertical.py shapes vertical.txt and vertical_features.txt
// with each of nine faces, top to bottom and across, and asks HarfBuzz for
// the vertical advance and origin of a sample of each face's glyphs; the
// answers are checked in as vertical.expected.txt. Each face answers the two
// questions a different way — vertical.py lists which — so between them they
// cover every rule vertical.go reads: VORG, the top phantom point of a
// TrueType glyph and of a composite that takes it from a component, the ink
// centred in the line where a face states neither, the vertical forms by
// 'vert' and by character, the positioning that moves glyphs down the page,
// and the horizontal rules an upright run does not apply.
//
// Four of the faces are in the corpora (NOTO_CJK and NOTO_FONTS), and their
// subtests skip or fail as every test of those corpora does. The other five
// are here and always run.

// verticalFaces reads each face the expectations were generated from, by the
// name the file gives it.
var verticalFaces = map[string]func(t *testing.T) []byte{
	"NotoSans-Variable.ttf":  notoSansBytes,
	"NotoSansArabic.ttf":     func(t *testing.T) []byte { return harfbuzzFont(t, "NotoSansArabic.ttf") },
	"VerticalComposites.ttf": func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalComposites.ttf") },
	"VerticalFallbacks.ttf":  func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalFallbacks.ttf") },
	"VerticalHhea.ttf":       func(t *testing.T) []byte { return harfbuzzFont(t, "VerticalHhea.ttf") },
	"NotoSansJP-Regular.otf": func(t *testing.T) []byte { return fonttest.CJKFile(t, "NotoSansJP-Regular.otf") },
	"NotoSansJP-VF.ttf":      func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSansJP-VF.ttf") },
	"ipag.ttf":               func(t *testing.T) []byte { return fonttest.NotoFile(t, "ipag.ttf") },
	"Unifont-Regular.otf":    func(t *testing.T) []byte { return fonttest.NotoFile(t, "Unifont-Regular.otf") },
}

// inkUnread are the faces whose vertical origins are not held to HarfBuzz's,
// and why: a CFF face with no VORG is hung by its ink, and this package does
// not read a CFF glyph's ink (see vertical.go). Such a face is hung from its
// ascender instead, which is asserted; everything else about its glyphs — which
// they are, how far they advance, where they sit across the line — is held to
// HarfBuzz as any other face's is. The test fails if the exception stops being
// needed, since an exception that cannot go stale is a hole.
var inkUnread = map[string]bool{"Unifont-Regular.otf": true}

func harfbuzzFont(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return data
}

// hbPosition is one glyph as HarfBuzz placed it, in font units, its offsets
// measured from the pen as HarfBuzz measures them.
type hbPosition struct{ gid, xAdvance, yAdvance, dx, dy int }

// hbVertical is one face's expectations.
type hbVertical struct {
	name, sum string
	// upright and across are the corpus shaped top to bottom and across the
	// page, a line each; featured is the features corpus shaped top to bottom.
	upright, across, featured [][]hbPosition
	// metrics is a glyph's vertical advance and origin, by glyph.
	metrics map[int][3]int
}

// featuredLine is one line of vertical_features.txt: the feature a document
// asks for, if any, and the text.
type featuredLine struct{ tags, text string }

func readVerticalGolden(t *testing.T) (corpus []string, featured []featuredLine, faces []*hbVertical) {
	t.Helper()
	corpus = readNonEmptyLines(t, filepath.Join(harfbuzzDir, "vertical.txt"))
	for _, l := range readNonEmptyLines(t, filepath.Join(harfbuzzDir, "vertical_features.txt")) {
		tags, text, _ := strings.Cut(l, " ")
		if tags == "-" {
			tags = ""
		}
		featured = append(featured, featuredLine{tags, text})
	}
	path := filepath.Join(harfbuzzDir, "vertical.expected.txt")
	refuseUnpinnedOracle(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbvertical` to generate it.", path, err)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var cur *hbVertical
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		kind, rest, _ := strings.Cut(text, " ")
		if kind == "#" || strings.HasPrefix(text, "#") {
			continue
		}
		if kind == "face" {
			name, sum, _ := strings.Cut(rest, " ")
			cur = &hbVertical{name: name, sum: sum, metrics: map[int][3]int{}}
			faces = append(faces, cur)
			continue
		}
		if cur == nil {
			t.Fatalf("%s:%d: %q before any face", path, line, kind)
		}
		if kind == "M" {
			var n [4]int
			for i, f := range strings.Fields(rest) {
				if i >= 4 {
					t.Fatalf("%s:%d: more than four numbers", path, line)
				}
				if n[i], err = strconv.Atoi(f); err != nil {
					t.Fatalf("%s:%d: %v", path, line, err)
				}
			}
			cur.metrics[n[0]] = [3]int{n[1], n[2], n[3]}
			continue
		}
		var glyphs []hbPosition
		for _, field := range strings.Fields(rest) {
			parts := strings.Split(field, ",")
			if len(parts) != 5 {
				t.Fatalf("%s:%d: %q has %d parts, want 5", path, line, field, len(parts))
			}
			var n [5]int
			for i, p := range parts {
				if n[i], err = strconv.Atoi(p); err != nil {
					t.Fatalf("%s:%d: %v", path, line, err)
				}
			}
			glyphs = append(glyphs, hbPosition{n[0], n[1], n[2], n[3], n[4]})
		}
		switch kind {
		case "V":
			cur.upright = append(cur.upright, glyphs)
		case "H":
			cur.across = append(cur.across, glyphs)
		case "F":
			cur.featured = append(cur.featured, glyphs)
		default:
			t.Fatalf("%s:%d: unknown line %q", path, line, kind)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(verticalFaces) {
		t.Fatalf("%d faces in %s and %d here to read them", len(faces), path, len(verticalFaces))
	}
	for _, f := range faces {
		if len(f.upright) != len(corpus) || len(f.across) != len(corpus) || len(f.featured) != len(featured) {
			t.Fatalf("%s: %d, %d and %d lines for %d strings and %d featured ones",
				f.name, len(f.upright), len(f.across), len(f.featured), len(corpus), len(featured))
		}
	}
	return corpus, featured, faces
}

// loadVerticalFace loads the face the expectations were generated from, and
// refuses one they were not: every expectation is about a glyph index.
func loadVerticalFace(t *testing.T, want *hbVertical) *Face {
	t.Helper()
	read, ok := verticalFaces[want.name]
	if !ok {
		t.Fatalf("the expectations name %s, which this test does not know how to read", want.name)
	}
	data := read(t)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbvertical` to regenerate them.", want.name, want.sum, got)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading %s: %v", want.name, err)
	}
	return f
}

// sameUpright compares a run shaped upright with HarfBuzz's, glyph by glyph,
// in font units. HarfBuzz measures an upright glyph's offsets from the pen,
// with its vertical origin taken off them; Glyph states the origin apart, so
// the comparison is of the offset less the origin. With inkUnread the height
// is left out: see inkUnread.
func sameUpright(f *Face, got []Glyph, want []hbPosition, inkUnread bool) (bool, string) {
	if len(got) != len(want) {
		return false, fmt.Sprintf("%d glyphs, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		dx, dy := f.units(g.XOffset-g.VOriginX), f.units(g.YOffset-g.VOriginY)
		switch {
		case g.GID != w.gid:
			return false, fmt.Sprintf("glyph %d is %d, want %d", i, g.GID, w.gid)
		case f.units(g.XAdvance) != w.xAdvance || f.units(g.YAdvance) != w.yAdvance:
			return false, fmt.Sprintf("glyph %d advances (%d, %d), want (%d, %d)",
				i, f.units(g.XAdvance), f.units(g.YAdvance), w.xAdvance, w.yAdvance)
		case dx != w.dx || dy != w.dy && !inkUnread:
			return false, fmt.Sprintf("glyph %d is placed at (%d, %d), want (%d, %d)", i, dx, dy, w.dx, w.dy)
		}
	}
	return true, ""
}

// TestUprightShapingAgreesWithHarfBuzz holds every line of both corpora, shaped
// upright in every face, to HarfBuzz's top-to-bottom answer.
func TestUprightShapingAgreesWithHarfBuzz(t *testing.T) {
	corpus, featured, faces := readVerticalGolden(t)
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			f := loadVerticalFace(t, want)
			unread := inkUnread[want.name]
			check := func(what, s string, off Features, expected []hbPosition) {
				off.Vertical = true
				glyphs, _ := f.ShapeGlyphsInContext(s, "", "", off)
				if same, why := sameUpright(f, glyphs, expected, unread); !same {
					t.Errorf("%s%s\n  %s", what, describeRunes(s), why)
				}
			}
			for i, s := range corpus {
				check("", s, Features{}, want.upright[i])
			}
			for i, l := range featured {
				check(l.tags+" ", l.text, Features{Tags: l.tags}, want.featured[i])
			}
		})
	}
}

// TestSidewaysRunsAreShapedAsBefore holds the same lines, shaped across the
// page as a run set sideways is, to HarfBuzz's horizontal answer, and to
// carrying no vertical metrics: an upright run is the only kind that has them.
//
// A line that changes direction is left out. HarfBuzz is handed it as one run,
// and this package cuts it where the direction changes and puts the pieces in
// the order they are drawn, which is a difference in who does the bidi and not
// in the shaping.
func TestSidewaysRunsAreShapedAsBefore(t *testing.T) {
	corpus, _, faces := readVerticalGolden(t)
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			f := loadVerticalFace(t, want)
			compared := 0
			for i, s := range corpus {
				if len(bidiVisualRuns(s)) > 1 {
					continue
				}
				compared++
				glyphs, _ := f.ShapeGlyphs(s)
				expected := want.across[i]
				if len(glyphs) != len(expected) {
					t.Errorf("%s: %d glyphs, want %d", describeRunes(s), len(glyphs), len(expected))
					continue
				}
				for k, g := range glyphs {
					w := expected[k]
					if g.GID != w.gid || f.units(g.XAdvance) != w.xAdvance ||
						f.units(g.XOffset) != w.dx || f.units(g.YOffset) != w.dy {
						t.Errorf("%s: glyph %d is %d advancing %d at (%d, %d), want %d advancing %d at (%d, %d)",
							describeRunes(s), k, g.GID, f.units(g.XAdvance), f.units(g.XOffset),
							f.units(g.YOffset), w.gid, w.xAdvance, w.dx, w.dy)
						break
					}
					if g.YAdvance != 0 || g.VOriginX != 0 || g.VOriginY != 0 {
						t.Errorf("%s: glyph %d set across the page carries vertical metrics %v, (%v, %v)",
							describeRunes(s), k, g.YAdvance, g.VOriginX, g.VOriginY)
						break
					}
				}
			}
			if compared == 0 {
				t.Fatal("no line was compared")
			}
		})
	}
}

// TestVerticalMetricsAgreeWithHarfBuzz holds each sampled glyph's vertical
// advance and origin to what HarfBuzz answers for it asked directly
// (hb_font_get_glyph_v_advance and hb_font_get_glyph_v_origin).
func TestVerticalMetricsAgreeWithHarfBuzz(t *testing.T) {
	_, _, faces := readVerticalGolden(t)
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			f := loadVerticalFace(t, want)
			ascender, _ := f.fontExtentsUnits()
			differ := 0
			for gid, m := range want.metrics {
				advance, x, y := f.verticalUnits(gid)
				if advance != m[0] || x != m[1] {
					t.Errorf("glyph %d advances %d hung %d across, want %d and %d", gid, advance, x, m[0], m[1])
				}
				switch {
				case inkUnread[want.name]:
					if y != ascender {
						t.Errorf("glyph %d of a CFF face with no VORG is hung %d down, want the ascender %d",
							gid, y, ascender)
					}
					if y != m[2] {
						differ++
					}
				case y != m[2]:
					t.Errorf("glyph %d is hung %d down, want %d", gid, y, m[2])
				}
			}
			if inkUnread[want.name] && differ == 0 {
				t.Errorf("every origin of %s agrees with HarfBuzz; it is listed in inkUnread, "+
					"and should no longer be", want.name)
			}
			if len(want.metrics) == 0 {
				t.Fatal("no glyph was compared")
			}
		})
	}
}

// TestTheVerticalOracleHasTeeth checks the expectations exercise what the
// tests above claim they hold, from HarfBuzz's answers alone: a corpus in
// which nothing an upright run does differently had come out differently
// would pass whatever vertical.go did.
func TestTheVerticalOracleHasTeeth(t *testing.T) {
	corpus, featured, faces := readVerticalGolden(t)
	byName := map[string]*hbVertical{}
	for _, f := range faces {
		byName[f.name] = f
	}
	gids := func(g []hbPosition) string {
		var b strings.Builder
		for _, p := range g {
			fmt.Fprintf(&b, "%d ", p.gid)
		}
		return b.String()
	}
	// The vertical forms, by 'vert' and, in Unifont, by character: some line
	// is drawn with other glyphs upright than across.
	for _, name := range []string{"NotoSansJP-Regular.otf", "NotoSansJP-VF.ttf", "ipag.ttf",
		"Unifont-Regular.otf", "VerticalComposites.ttf"} {
		f, changed := byName[name], false
		for i := range corpus {
			changed = changed || gids(f.upright[i]) != gids(f.across[i])
		}
		if !changed {
			t.Errorf("%s draws every line with the same glyphs upright and across", name)
		}
	}
	// The features that move a glyph along the page, and the one whose
	// advance stops applying: each featured line comes out differently from
	// the same text with no feature asked for, in some face.
	plain := map[string]int{}
	for i, l := range featured {
		if l.tags == "" {
			plain[l.text] = i
		}
	}
	for i, l := range featured {
		if l.tags == "" {
			continue
		}
		j, ok := plain[l.text]
		if !ok {
			t.Errorf("%q is asked for with %s and never without it, so nothing says the "+
				"feature did anything", l.text, l.tags)
			continue
		}
		moved := false
		for _, f := range faces {
			moved = moved || fmt.Sprint(f.featured[i]) != fmt.Sprint(f.featured[j])
		}
		if !moved {
			t.Errorf("%q with %s is placed as it is without it in every face", l.text, l.tags)
		}
	}
}

// An upright run's rules, where no corpus is needed to see them.

// TestAnUprightRunIsNotReordered is the one thing the direction does to the
// text: nothing. Arabic set upright is set in the order it is written, as CSS
// Writing Modes has every character of an upright run treated as strong
// left-to-right, where across the page it is reversed.
func TestAnUprightRunIsNotReordered(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "NotoSansArabic.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	const s = "\u0628\u0633\u0645"
	across, _ := f.ShapeGlyphs(s)
	upright, _ := f.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
	if len(across) == 0 || len(upright) == 0 {
		t.Fatalf("%d and %d glyphs", len(across), len(upright))
	}
	for i := 1; i < len(upright); i++ {
		if upright[i].Cluster < upright[i-1].Cluster {
			t.Errorf("upright glyph %d is from byte %d, after one from byte %d: the run was reordered",
				i, upright[i].Cluster, upright[i-1].Cluster)
		}
	}
	if across[0].Cluster != len(s)-2 {
		t.Errorf("across the page the first glyph drawn is from byte %d, want %d — the "+
			"control for the case above", across[0].Cluster, len(s)-2)
	}
}

// A synthetic TrueType face with vertical metrics, for the bounds the corpora
// cannot reach: a composite nested past where HarfBuzz stops following it, and
// one whose components name each other.

// verticalFont builds a TrueType face with the glyf entries given after an
// empty .notdef, each glyph 500 units wide, and a vmtx giving glyph i an
// advance of 1000 and a top side bearing of 10*i. glyphs[i] is glyph i+1's
// entry, and maps from U+4E00+i.
func verticalFont(glyphs [][]byte) []byte {
	n := len(glyphs) + 1
	var glyf []byte
	loca := make([]byte, 4*(n+1))
	for i, g := range glyphs {
		glyf = append(glyf, g...)
		binary.BigEndian.PutUint32(loca[4*(i+2):], uint32(len(glyf)))
	}
	vhea := make([]byte, 36)
	binary.BigEndian.PutUint32(vhea[0:], 0x00011000)
	binary.BigEndian.PutUint16(vhea[34:], uint16(n))
	vmtx := make([]byte, 4*n)
	for i := 0; i < n; i++ {
		binary.BigEndian.PutUint16(vmtx[4*i:], 1000)
		binary.BigEndian.PutUint16(vmtx[4*i+2:], uint16(10*i))
	}
	opts := fonttest.SFNTOptions{Extra: map[string][]byte{
		"glyf": glyf, "loca": loca, "vhea": vhea, "vmtx": vmtx,
	}}
	for i := range glyphs {
		opts.Glyphs = append(opts.Glyphs, fonttest.Glyph{Rune: rune(0x4E00 + i), Advance: 500})
	}
	return fonttest.SFNT(opts)
}

func verticalFixture(t *testing.T, glyphs [][]byte) *Face {
	t.Helper()
	f, err := Load(verticalFont(glyphs))
	if err != nil {
		t.Fatalf("loading the fixture: %v", err)
	}
	return f
}

// usingMetricsOf is a composite glyf entry whose box reaches yMax and whose
// components are the glyphs given, each flagged USE_MY_METRICS.
func usingMetricsOf(yMax int, comps ...int) []byte {
	out := make([]byte, 10)
	binary.BigEndian.PutUint16(out[0:], 0xFFFF) // numberOfContours -1
	binary.BigEndian.PutUint16(out[8:], uint16(yMax))
	for i, c := range comps {
		flags := compArgsAreWords | compArgsAreXY | compUseMyMetrics
		if i < len(comps)-1 {
			flags |= compMoreComponents
		}
		rec := make([]byte, 8)
		binary.BigEndian.PutUint16(rec[0:], uint16(flags))
		binary.BigEndian.PutUint16(rec[2:], uint16(c))
		out = append(out, rec...)
	}
	return out
}

// TestACompositeNestedPastTheBoundIsHungFromTheEm is where HarfBuzz stops
// following components (HB_MAX_NESTING_LEVEL), and what it answers when it
// does: the em. Just inside the bound the chain is followed to its end.
func TestACompositeNestedPastTheBoundIsHungFromTheEm(t *testing.T) {
	// Glyph 1 is simple; glyph k takes its metrics from glyph k-1.
	const n = maxPhantomDepth + 3
	glyphs := [][]byte{fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700})}
	for k := 2; k <= n; k++ {
		glyphs = append(glyphs, usingMetricsOf(0, k-1))
	}
	f := verticalFixture(t, glyphs)
	// A chain reaches glyph 1, whose point is its top plus its side bearing.
	const want = 700 + 10
	if _, _, y := f.verticalUnits(maxPhantomDepth + 1); y != want {
		t.Errorf("a chain %d deep is hung %d down, want glyph 1's %d", maxPhantomDepth, y, want)
	}
	if _, _, y := f.verticalUnits(maxPhantomDepth + 2); y != f.UnitsPerEm() {
		t.Errorf("a chain %d deep is hung %d down, want the em", maxPhantomDepth+1, y)
	}
}

// TestACompositeWalkPastTheEdgeBoundIsHungFromTheEm is the other bound
// HarfBuzz walks a composite under: how many glyphs one walk may visit
// (HB_MAX_GRAPH_EDGE_COUNT). Glyph 2 takes its metrics from glyph 1 a hundred
// times over; glyph 3 from glyph 2 163 times, which is 16,464 visits, and
// glyph 4 162 times, which is 16,363. The first is past the bound and hung an
// em down, the second inside it and hung from glyph 1 — HarfBuzz 14.5's
// answers for this face, which put the bound exactly where this does.
func TestACompositeWalkPastTheEdgeBoundIsHungFromTheEm(t *testing.T) {
	fan := func(n, target int) []byte {
		comps := make([]int, n)
		for i := range comps {
			comps[i] = target
		}
		return usingMetricsOf(0, comps...)
	}
	f := verticalFixture(t, [][]byte{
		fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700}),
		fan(100, 1), fan(163, 2), fan(162, 2),
	})
	for gid, want := range map[int]int{2: 700 + 10, 3: f.UnitsPerEm(), 4: 700 + 10} {
		if _, _, y := f.verticalUnits(gid); y != want {
			t.Errorf("glyph %d is hung %d down, want %d", gid, y, want)
		}
	}
}

// TestCompositesThatNameEachOtherAreWalkedOnce is two composites each taking
// its metrics from the other, which HarfBuzz follows until its decycler sees
// the cycle — from glyph 2 into 3, 2 and 3 again, where the tortoise is
// visiting 2 — so that glyph 2 is hung from 3's own point and 3 from 2's. The
// numbers are what HarfBuzz 14.5 answers for this face, asked when this was
// written, and they are not what a walk that stops the first time it meets a
// glyph it is inside answers: that one hangs each from its own point.
func TestCompositesThatNameEachOtherAreWalkedOnce(t *testing.T) {
	f := verticalFixture(t, [][]byte{
		fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700}),
		usingMetricsOf(300, 3),
		usingMetricsOf(400, 2),
	})
	if _, _, y := f.verticalUnits(2); y != 400+30 {
		t.Errorf("glyph 2 is hung %d down, want glyph 3's own %d", y, 400+30)
	}
	if _, _, y := f.verticalUnits(3); y != 300+20 {
		t.Errorf("glyph 3 is hung %d down, want glyph 2's own %d", y, 300+20)
	}
}

// TestCompositeWalksAreChargedToTheFont is the bound on the walks as a whole.
//
// One composite's walk may visit as many glyphs as HarfBuzz lets it — sixteen
// thousand — and a font may have thousands of composites, and name any of them
// in every position of every run. The walks are made once, at load, and
// charged to the font's budget with everything else Load reads, so a font
// whose composites cost more than that to hang is refused there, and says
// what cost too much, rather than walked again for every glyph set upright.
func TestCompositeWalksAreChargedToTheFont(t *testing.T) {
	// Glyph 1 is simple. Glyph 2 takes its metrics from glyph 1, a hundred
	// times over; every glyph after it takes them from glyph 2, a hundred
	// times over. So each of those is a walk of ten thousand glyphs — well
	// inside HarfBuzz's bound for one — and a thousand of them are ten
	// million.
	fan := func(target int) []byte {
		comps := make([]int, 100)
		for i := range comps {
			comps[i] = target
		}
		return usingMetricsOf(0, comps...)
	}
	glyphs := [][]byte{fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700}), fan(1)}
	for len(glyphs) < 1000 {
		glyphs = append(glyphs, fan(2))
	}
	_, err := Load(verticalFont(glyphs))
	if err == nil {
		t.Fatal("a font whose composites cost ten million steps to hang was loaded")
	}
	if !strings.Contains(err.Error(), "vertical origins") {
		t.Errorf("the refusal is %q, and does not name what cost too much", err)
	}
	// The same shape, small enough, loads — the refusal is the budget's and
	// not the shape's — and is hung from glyph 1.
	small := verticalFixture(t, glyphs[:10])
	if _, _, y := small.verticalUnits(10); y != 700+10 {
		t.Errorf("glyph 10 is hung %d down, want glyph 1's %d", y, 700+10)
	}
}

// TestTruncatedVerticalTablesAreReadAsFarAsTheyGo is the clamping: a vhea
// claiming more long records than vmtx holds, and a VORG claiming more records
// than it has, are read as what is there rather than past the end.
func TestTruncatedVerticalTablesAreReadAsFarAsTheyGo(t *testing.T) {
	vhea := make([]byte, 36)
	binary.BigEndian.PutUint32(vhea[0:], 0x00010000)
	binary.BigEndian.PutUint16(vhea[34:], 500)                     // five hundred records...
	vmtx := []byte{0x03, 0xE8, 0x00, 0x32, 0x02, 0x58}             // ...in one and a half
	vorg := []byte{0, 1, 0, 0, 0x03, 0x70, 0x00, 0x09, 0, 1, 0, 5} // nine claimed, one there
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 500, HasShape: true}},
		Extra:  map[string][]byte{"vhea": vhea, "vmtx": vmtx, "VORG": vorg},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	v := f.vert
	if v.longMetrics != 1 || v.vorg != nil {
		t.Fatalf("read %d long records and VORG %v, want one record and no VORG", v.longMetrics, v.vorg)
	}
	for gid := 0; gid < 4; gid++ {
		if advance, _, _ := f.verticalUnits(gid); advance != 1000 {
			t.Errorf("glyph %d advances %d, want the one advance there is, 1000", gid, advance)
		}
	}
}

// TestAFaceWithNoLineMetricsIsGivenHarfBuzzs is a face with no hhea at all:
// HarfBuzz gives it four fifths of an em above the baseline and the rest
// below for its line, and half an em for every glyph's horizontal advance,
// and hangs a glyph accordingly. The numbers are what HarfBuzz 14.5 answers
// for this face, asked when this was written.
func TestAFaceWithNoLineMetricsIsGivenHarfBuzzs(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 500, HasShape: true}}})
	tables := int(binary.BigEndian.Uint16(data[4:]))
	renamed := false
	for i := 0; i < tables; i++ {
		if at := 12 + 16*i; string(data[at:at+4]) == "hhea" {
			copy(data[at:], "zhea")
			renamed = true
		}
	}
	if !renamed {
		t.Fatal("the fixture has no hhea to take away")
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[int][3]int{0: {1000, 250, 500}, 1: {1000, 250, 900}} {
		if a, x, y := f.verticalUnits(gid); [3]int{a, x, y} != want {
			t.Errorf("glyph %d advances %d hung at (%d, %d), want %v", gid, a, x, y, want)
		}
	}
}

// TestAMalformedSimpleGlyphIsHungFromTheEm is the answer HarfBuzz gives where
// it cannot read a TrueType glyph's points: one whose end-point list runs past
// its bytes, and one whose last end point leaves fewer points than it has
// contours, are both hung an em down (HarfBuzz 14.5's answer for this face).
func TestAMalformedSimpleGlyphIsHungFromTheEm(t *testing.T) {
	f := verticalFixture(t, [][]byte{
		fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700}),
		{0, 1, 0, 0, 0, 0, 0, 100, 0x02, 0x58, 0, 3},
		{0, 3, 0, 0, 0, 0, 0, 100, 0x02, 0x58, 0, 0, 0, 0, 0, 0, 0, 0},
	})
	if _, _, y := f.verticalUnits(1); y != 700+10 {
		t.Errorf("the well-formed glyph is hung %d down, want %d — the control", y, 700+10)
	}
	for _, gid := range []int{2, 3} {
		if _, _, y := f.verticalUnits(gid); y != f.UnitsPerEm() {
			t.Errorf("malformed glyph %d is hung %d down, want the em", gid, y)
		}
	}
}

// TestComponentRecordsAreChargedToo is the other half of the walk's cost. A
// glyph visited is a unit, and so is each component record read: a composite
// with thousands of records that every walk passes through costs thousands of
// steps each time, and counting visits alone would let a font of a hundred
// kilobytes cost fifty million of them for a unit apiece.
func TestComponentRecordsAreChargedToo(t *testing.T) {
	// Glyph 2 has eight thousand components, the last flagged. Every glyph
	// after it takes its metrics from glyph 2, a hundred times over.
	wide := make([]int, 8000)
	for i := range wide {
		wide[i] = 1
	}
	many := usingMetricsOf(0, wide...)
	for at := 10; at+8 <= len(many)-8; at += 8 {
		// Unflag every record but the last, so that the walk reads them all
		// and follows one.
		many[at], many[at+1] = 0, byte(compArgsAreWords|compArgsAreXY|compMoreComponents)
	}
	fan := make([]int, 100)
	for i := range fan {
		fan[i] = 2
	}
	glyphs := [][]byte{fonttest.SimpleGlyph([]int{0, 100, 100}, []int{0, 0, 700}), many}
	for len(glyphs) < 62 {
		glyphs = append(glyphs, usingMetricsOf(0, fan...))
	}
	if _, err := Load(verticalFont(glyphs)); err == nil || !strings.Contains(err.Error(), "vertical origins") {
		t.Errorf("a font whose walks read fifty million component records loaded, or was refused "+
			"for something else: %v", err)
	}
	small := verticalFixture(t, glyphs[:3])
	if _, _, y := small.verticalUnits(3); y != 700+10 {
		t.Errorf("glyph 3 is hung %d down, want glyph 1's %d", y, 700+10)
	}
}

// TestAnUprightRunKernsNothingAcrossItsEdges is the pair that spans a run
// boundary, which a run set across the page is kerned by and a run set upright
// is not: the pairs are the 'kern' feature's, and a vertical run applies none
// of them.
func TestAnUprightRunKernsNothingAcrossItsEdges(t *testing.T) {
	f, err := Load(notoSansBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	alone, _ := f.ShapeGlyphsInContext("A", "", "", Features{})
	beside, _ := f.ShapeGlyphsInContext("A", "", "V", Features{})
	if fmt.Sprint(alone) == fmt.Sprint(beside) {
		t.Fatal("A before V is placed as A alone across the page, so this proves nothing")
	}
	alone, _ = f.ShapeGlyphsInContext("A", "", "", Features{Vertical: true})
	beside, _ = f.ShapeGlyphsInContext("A", "", "V", Features{Vertical: true})
	if fmt.Sprint(alone) != fmt.Sprint(beside) {
		t.Errorf("an upright A is placed %+v before V and %+v alone", beside, alone)
	}
}

// TestAFaceSetByCodeIsHungByTheSameRules is the path for a simple face and a
// standard one, whose glyphs are character codes and not glyph indices. A
// simple face is the same program read another way, so its upright glyphs are
// hung as the composite reading of it hangs them; a standard face has no
// program, and is given the line's height and half its width from its AFM.
func TestAFaceSetByCodeIsHungByTheSameRules(t *testing.T) {
	composite, err := Load(notoSansBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	simple, err := NotoSansSimple()
	if err != nil {
		t.Fatal(err)
	}
	const s = "Ag"
	want, _ := composite.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
	got, _ := simple.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
	if len(got) != len(want) {
		t.Fatalf("%d glyphs by code, %d by index", len(got), len(want))
	}
	for i := range got {
		if got[i].XAdvance != 0 || got[i].YAdvance != want[i].YAdvance ||
			got[i].VOriginX != want[i].VOriginX || got[i].VOriginY != want[i].VOriginY {
			t.Errorf("%q by code advances (%v, %v) hung at (%v, %v); by index (%v, %v) at (%v, %v)",
				s[i], got[i].XAdvance, got[i].YAdvance, got[i].VOriginX, got[i].VOriginY,
				want[i].XAdvance, want[i].YAdvance, want[i].VOriginX, want[i].VOriginY)
		}
	}
	helvetica, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := helvetica.ShapeGlyphsInContext("A", "", "", Features{Vertical: true})
	width, _ := helvetica.Advance('A')
	ascender, descender := helvetica.fontExtentsUnits()
	if len(glyphs) != 1 || glyphs[0].XAdvance != 0 || glyphs[0].YAdvance != float64(descender-ascender) ||
		glyphs[0].VOriginX != float64(int(width)/2) || glyphs[0].VOriginY <= 0 {
		t.Errorf("Helvetica's upright A is %+v, want it to advance %d and hang from (%d, above the baseline)",
			glyphs, descender-ascender, int(width)/2)
	}
}
