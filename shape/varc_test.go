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

// Variable composites, held to HarfBuzz and to fontTools.
//
// testdata/harfbuzz/varc.py asks HarfBuzz for the extents and the path of every
// glyph of VarComposite.ttf, which varc_fixture.py builds, at its default with
// no coordinates and at six locations, and fontTools' glyph set for each VARC
// glyph's path. The answers are checked in as varc.expected.txt.

// varcGolden is the face at one location.
type varcGolden struct {
	name     string
	loc      map[string]float64 // nil at the default, where nothing was set
	sum      string
	extents  []*extents
	harfbuzz map[int][][][2]float64 // each glyph's contours as HarfBuzz draws them
	fonttool map[int][][][2]float64 // and as fontTools does, where it can
}

func readVARCGolden(t *testing.T) []*varcGolden {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "varc.expected.txt")
	refuseUnpinnedOracle(t, path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v\nRun `make hbvarc` to generate it.", err)
	}
	defer f.Close()
	var out []*varcGolden
	var cur *varcGolden
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
			name, at, _ := strings.Cut(fields[0], "@")
			cur = &varcGolden{name: name, sum: fields[1], harfbuzz: map[int][][][2]float64{}, fonttool: map[int][][][2]float64{}}
			if at != "default" {
				cur.loc = map[string]float64{}
				for _, kv := range strings.Split(at, ",") {
					k, v, _ := strings.Cut(kv, "=")
					cur.loc[k] = parseNum(t, line, v)
				}
			}
			out = append(out, cur)
		case "E":
			f := strings.Fields(rest)
			if f[1] == "none" {
				cur.extents = append(cur.extents, nil)
				continue
			}
			var n [4]int
			for i := range n {
				n[i], _ = strconv.Atoi(f[1+i])
			}
			cur.extents = append(cur.extents, &extents{n[0], n[1], n[2], n[3]})
		case "H", "F":
			gid, contours, ok := parseContours(t, line, rest)
			if !ok {
				continue
			}
			if tag == "H" {
				cur.harfbuzz[gid] = contours
			} else {
				cur.fonttool[gid] = contours
			}
		default:
			t.Fatalf("line %d: %q", line, text)
		}
	}
	return out
}

// parseContours reads a path of lines as its contours' points, the line back
// to where each started left out; false for a path an oracle could not draw.
func parseContours(t *testing.T, line int, rest string) (int, [][][2]float64, bool) {
	f := strings.Fields(rest)
	gid, _ := strconv.Atoi(f[0])
	if len(f) > 1 && f[1] == "unavailable" {
		return gid, nil, false
	}
	var out [][][2]float64
	for i := 1; i < len(f); {
		switch f[i] {
		case "M", "L":
			p := [2]float64{parseNum(t, line, f[i+1]), parseNum(t, line, f[i+2])}
			if f[i] == "M" {
				out = append(out, nil)
			}
			out[len(out)-1] = append(out[len(out)-1], p)
			i += 3
		case "Z":
			if c := out[len(out)-1]; len(c) > 1 && c[len(c)-1] == c[0] {
				out[len(out)-1] = c[:len(c)-1]
			}
			i++
		default:
			t.Fatalf("line %d: %q in a path", line, f[i])
		}
	}
	return gid, out, true
}

func varcFont(t *testing.T, want *varcGolden) []byte {
	t.Helper()
	data := harfbuzzFont(t, want.name)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbvarc` to regenerate them.", want.name, want.sum, got)
	}
	return data
}

func varcName(g *varcGolden) string {
	if g.loc == nil {
		return g.name + "@default"
	}
	var parts []string
	for _, k := range []string{"0000", "0001", "wght"} {
		if v, ok := g.loc[k]; ok {
			parts = append(parts, k+"="+strconv.FormatFloat(v, 'g', -1, 64))
		}
	}
	return strings.Join(parts, ",")
}

// TestVARCInkAgreesWithHarfBuzz: every glyph of the face as Load reads it has
// the extents HarfBuzz gives it with no coordinates set — the VARC glyphs'
// the union of their leaves' turned boxes, and every other glyph's through
// VARC too, down to an empty glyph's void box — and every VARC glyph of a face
// from LoadInstance has the extents HarfBuzz gives it at the location.
func TestVARCInkAgreesWithHarfBuzz(t *testing.T) {
	golden := readVARCGolden(t)
	compared := 0
	for _, want := range golden {
		t.Run(varcName(want), func(t *testing.T) {
			data := varcFont(t, want)
			var f *Face
			var err error
			if want.loc == nil {
				f, err = Load(data)
			} else {
				f, err = LoadInstance(data, want.loc)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(want.extents) != f.NumGlyphs() {
				t.Fatalf("%d glyphs compared of the face's %d", len(want.extents), f.NumGlyphs())
			}
			// HarfBuzz takes all of these tables but two, and those it
			// refuses whole.
			refused := want.name == "VarCompositeDeep65.ttf" || want.name == "VarCompositeBroken.ttf"
			if want.loc == nil && (f.varc == nil) != refused {
				t.Fatalf("%s loaded with its VARC table read %v, and HarfBuzz reads it %v",
					want.name, f.varc != nil, !refused)
			}
			for gid, w := range want.extents {
				// An instance measures a glyph VARC does not compose as the
				// static glyf it now is, as every instance measures its glyphs.
				if _, ok := f.varcInk[gid]; want.loc != nil && !ok {
					continue
				}
				got, ok := f.glyphExtents(gid)
				switch {
				case w == nil && ok:
					t.Errorf("glyph %d has extents %+v, and HarfBuzz has none", gid, got)
				case w != nil && !ok:
					t.Errorf("glyph %d has no extents, and HarfBuzz's are %+v", gid, *w)
				case w != nil && got != *w:
					t.Errorf("glyph %d has extents %+v, want %+v", gid, got, *w)
				default:
					compared++
				}
			}
		})
	}
	if compared == 0 {
		t.Fatal("no glyph was compared")
	}
}

// glyfContours are a glyf entry's points by contour, as a reader of it draws
// them: moved by how far its left side bearing puts its origin from its box.
func glyfContours(t *testing.T, program []byte, gid int) [][][2]float64 {
	t.Helper()
	tables := font.SFNTTables(program)
	head, loca, glyf := tables["head"], tables["loca"], tables["glyf"]
	var start, end int
	if binary.BigEndian.Uint16(head[50:]) == 1 {
		start, end = int(binary.BigEndian.Uint32(loca[4*gid:])), int(binary.BigEndian.Uint32(loca[4*gid+4:]))
	} else {
		start, end = 2*int(binary.BigEndian.Uint16(loca[2*gid:])), 2*int(binary.BigEndian.Uint16(loca[2*gid+2:]))
	}
	if start == end {
		return nil
	}
	g, err := decodeVarGlyph(glyf[start:end], 1<<16)
	if err != nil || g.composite {
		t.Fatalf("glyph %d is not a simple glyph: %v", gid, err)
	}
	hhea, hmtx := tables["hhea"], tables["hmtx"]
	long := int(binary.BigEndian.Uint16(hhea[34:]))
	at := 4*gid + 2
	if gid >= long {
		at = 4*long + 2*(gid-long)
	}
	shift := float64(signed16(font.Be16(glyf[start:end], 2)) - signed16(font.Be16(hmtx, at)))
	var out [][][2]float64
	prev := -1
	for _, e := range g.ends {
		var c [][2]float64
		for i := prev + 1; i <= e; i++ {
			c = append(c, [2]float64{g.x[i] - shift, g.y[i]})
		}
		out = append(out, c)
		prev = e
	}
	return out
}

// roundedContours are an oracle's contours as glyf stores them: each point
// rounded half up to a whole unit.
func roundedContours(cs [][][2]float64) [][][2]float64 {
	out := make([][][2]float64, len(cs))
	for i, c := range cs {
		for _, p := range c {
			out[i] = append(out[i], [2]float64{float64(otRound(p[0])), float64(otRound(p[1]))})
		}
	}
	return out
}

// checkMaxp holds maxp's counts of points and contours to cover the glyphs
// written out: a reader sizes what it reads a glyph into by them.
func checkMaxp(t *testing.T, program []byte, gid int, contours [][][2]float64) {
	t.Helper()
	maxp := font.SFNTTables(program)["maxp"]
	points := 0
	for _, c := range contours {
		points += len(c)
	}
	if mp, mc := font.Be16(maxp, 6), font.Be16(maxp, 8); points > mp || len(contours) > mc {
		t.Errorf("glyph %d has %d points in %d contours, and maxp says at most %d in %d", gid, points, len(contours), mp, mc)
	}
}

func sameContours(a, b [][][2]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !samePoints(a[i], b[i], 0) {
			return false
		}
	}
	return true
}

// TestVARCInstanceDrawsWhatHarfBuzzDraws: a face from LoadInstance carries
// each VARC glyph written out as a glyf outline, and that outline, drawn from
// the origin its side bearing gives it, is HarfBuzz's drawing of the glyph at
// the location with each point rounded to the whole unit glyf stores — every
// contour, every point, in HarfBuzz's order. Where fontTools draws the glyph,
// its drawing rounds to the same.
func TestVARCInstanceDrawsWhatHarfBuzzDraws(t *testing.T) {
	golden := readVARCGolden(t)
	compared, withFontTools := 0, 0
	for _, want := range golden {
		if want.loc == nil || want.name != "VarComposite.ttf" {
			continue
		}
		t.Run(varcName(want), func(t *testing.T) {
			f, err := LoadInstance(varcFont(t, want), want.loc)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := font.SFNTTables(f.Program())["VARC"]; ok {
				t.Error("the instance carries VARC, which a reader would draw at a design space it does not have")
			}
			for gid := range f.varcInk {
				got := glyfContours(t, f.Program(), gid)
				checkMaxp(t, f.Program(), gid, got)
				hb := roundedContours(want.harfbuzz[gid])
				if !sameContours(got, hb) {
					t.Errorf("glyph %d draws %v, and HarfBuzz %v", gid, got, hb)
					continue
				}
				compared++
				if ft, ok := want.fonttool[gid]; ok {
					if !sameContours(got, roundedContours(ft)) {
						t.Errorf("glyph %d draws %v, and fontTools %v", gid, got, roundedContours(ft))
					}
					withFontTools++
				}
			}
		})
	}
	if compared == 0 || withFontTools == 0 {
		t.Fatalf("%d glyphs compared with HarfBuzz and %d with fontTools", compared, withFontTools)
	}
}

// TestVARCSubsetDrawsWhatHarfBuzzDraws: a subset of the face as Load reads it
// — what a PDF embeds — writes each VARC glyph it keeps as the glyf outline
// HarfBuzz draws for it with no coordinates set, rounded to whole units.
func TestVARCSubsetDrawsWhatHarfBuzzDraws(t *testing.T) {
	var want *varcGolden
	for _, g := range readVARCGolden(t) {
		if g.loc == nil && g.name == "VarComposite.ttf" {
			want = g
		}
	}
	f, err := Load(varcFont(t, want))
	if err != nil {
		t.Fatal(err)
	}
	for gid := 0; gid < f.NumGlyphs(); gid++ {
		f.used[gid] = true
	}
	sub, err := f.Subset()
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for gid := 0; gid < f.NumGlyphs(); gid++ {
		if f.varc.t.coverageIndex(gid) < 0 {
			continue
		}
		got := glyfContours(t, sub, gid)
		checkMaxp(t, sub, gid, got)
		if hb := roundedContours(want.harfbuzz[gid]); !sameContours(got, hb) {
			t.Errorf("glyph %d draws %v in the subset, and HarfBuzz %v", gid, got, hb)
		}
		compared++
	}
	if compared == 0 {
		t.Fatal("no VARC glyph was compared")
	}
}

// TestVARCOutlinesAreHarfBuzzsToTheBit: the transforms HarfBuzz composes in single
// precision are the ones this composes: a VARC glyph's outline at a location
// is HarfBuzz's to the bit, before glyf's rounding.
func TestVARCOutlinesAreHarfBuzzsToTheBit(t *testing.T) {
	golden := readVARCGolden(t)
	compared := 0
	for _, want := range golden {
		if want.name != "VarComposite.ttf" {
			continue
		}
		t.Run(varcName(want), func(t *testing.T) {
			data := varcFont(t, want)
			src, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			var coords []int
			if want.loc != nil {
				tables := font.SFNTTables(data)
				coords = hbNormalizedCoords(tables["fvar"], tables["avar"], want.loc)
			}
			for gid := 0; gid < src.NumGlyphs(); gid++ {
				if src.varc.t.coverageIndex(gid) < 0 {
					continue
				}
				work := int64(maxVarcWork)
				o, err := src.varc.outline(gid, coords, &work)
				if err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
				var got [][][2]float64
				prev := -1
				for _, e := range o.ends {
					var c [][2]float64
					for i := prev + 1; i <= e; i++ {
						c = append(c, [2]float64{float64(o.points[i].x), float64(o.points[i].y)})
					}
					got = append(got, c)
					prev = e
				}
				if !sameContours(got, want.harfbuzz[gid]) {
					t.Errorf("glyph %d draws %v, and HarfBuzz %v", gid, got, want.harfbuzz[gid])
				}
				compared++
			}
		})
	}
	if compared == 0 {
		t.Fatal("no glyph was compared")
	}
}

// TestVARCSurvivesEveryByteChanged loads VarComposite.ttf with each byte of its
// VARC table set to nothing and to everything in turn, and measures, draws,
// subsets and instances every glyph. The table is offsets and counts
// throughout — the coverage, the store's regions and rows, the conditions
// nested in each other, the records, each component's variable-length fields —
// and each byte changed is one of them a reader could trust.
func TestVARCSurvivesEveryByteChanged(t *testing.T) {
	data := harfbuzzFont(t, "VarComposite.ttf")
	off, length := tableRange(t, data, "VARC")
	read := 0
	for i := off; i < off+length; i++ {
		for _, b := range []byte{0x00, 0xFF} {
			if data[i] == b {
				continue
			}
			mutated := append([]byte(nil), data...)
			mutated[i] = b
			noPanic(t, "VARC", func() {
				f, err := Load(mutated)
				if err != nil {
					return
				}
				if f.varc != nil {
					read++
				}
				for gid := 0; gid < f.NumGlyphs(); gid++ {
					f.glyphExtents(gid)
					f.used[gid] = true
				}
				_, _ = f.Subset()
				_, _ = LoadInstance(mutated, map[string]float64{"wght": 900, "0000": 0.5})
			})
		}
	}
	if read < length {
		t.Errorf("only %d of %d changed tables were read", read, 2*length)
	}
}

// varcFanOut is a VARC table in which glyph 1 has n components that are each
// glyph 2, which has n components that are each glyph 3, and so on to glyph
// depth, whose n components are glyph depth+1, a leaf: n^depth leaves.
func varcFanOut(n, depth int) []byte {
	cov := []byte{0, 1, 0, byte(depth)}
	for g := 1; g <= depth; g++ {
		cov = append(cov, 0, byte(g))
	}
	var records [][]byte
	for g := 1; g <= depth; g++ {
		var r []byte
		for i := 0; i < n; i++ {
			r = append(r, 0, 0, byte(g+1)) // flags 0, glyph g+1
		}
		records = append(records, r)
	}
	// A CFF2 INDEX of the records with four-byte offsets.
	index := binary.BigEndian.AppendUint32(nil, uint32(len(records)))
	index = append(index, 4)
	o := 1
	for _, r := range records {
		index = binary.BigEndian.AppendUint32(index, uint32(o))
		o += len(r)
	}
	index = binary.BigEndian.AppendUint32(index, uint32(o))
	for _, r := range records {
		index = append(index, r...)
	}
	t := make([]byte, 24)
	binary.BigEndian.PutUint16(t, 1)
	binary.BigEndian.PutUint32(t[4:], 24)
	binary.BigEndian.PutUint32(t[20:], uint32(24+len(cov)))
	return append(append(t, cov...), index...)
}

// TestVARCWorkIsBoundedAndReported: glyphs whose components fan out to more
// leaves than there are atoms are measured within the face's share of work,
// which each glyph spends from and none can exceed HarfBuzz's own allowance
// of, and once the share is gone the glyphs after are left without ink and
// the face says so. Nothing is counted in time.
func TestVARCWorkIsBoundedAndReported(t *testing.T) {
	const depth = 6
	glyphs := []fonttest.Glyph{}
	for g := 0; g <= depth+1; g++ {
		glyphs = append(glyphs, fonttest.Glyph{Rune: rune('a' + g), Advance: 500, HasShape: true})
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: glyphs,
		Extra:  map[string][]byte{"VARC": varcFanOut(64, depth)},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.varc == nil {
		t.Fatal("the fan-out table was not read")
	}
	share := f.varc.work
	for gid := 1; gid <= depth; gid++ {
		f.glyphExtents(gid)
	}
	if spent := share - f.varc.work; spent > share+maxVarcWork {
		t.Errorf("measuring spent %d units of a share of %d", spent, share)
	}
	if f.varc.work > 0 {
		t.Fatalf("64^%d leaves were measured within the face's share of work, so this measures no bound", depth)
	}
	limits := strings.Join(f.LayoutLimits(), "; ")
	if !strings.Contains(limits, "variable composite glyphs (VARC) ran past the work") {
		t.Errorf("the face ran out of work and its limits say %q", limits)
	}
	if _, ok := f.glyphExtents(1); !ok {
		t.Error("the glyph measured first lost the ink it was measured with")
	}
	if _, ok := f.glyphExtents(depth); ok {
		t.Error("a glyph measured after the face's share was gone has ink")
	}
}

// TestVARCRefusesWhatItCannotWriteOut: where a VARC glyph cannot be written
// out as the glyf outline HarfBuzz draws — its leaves are CFF, or a glyf
// composite has it as a component, which draws its own glyf entry where the
// glyph draws its composite — a subset and an instance are refused, and say
// which glyph, rather than drawing its placeholder.
func TestVARCRefusesWhatItCannotWriteOut(t *testing.T) {
	cff, err := Load(harfbuzzFont(t, "VarCompositeCFF.otf"))
	if err != nil {
		t.Fatal(err)
	}
	cff.used[1] = true
	if _, err := cff.Subset(); err == nil || !strings.Contains(err.Error(), "glyph 1 is a variable composite (VARC) in a CFF face") {
		t.Errorf("a CFF face's VARC glyph was subset, with %v", err)
	}
	// And one whose VARC glyph was not used subsets as before.
	plain, err := Load(harfbuzzFont(t, "VarCompositeCFF.otf"))
	if err != nil {
		t.Fatal(err)
	}
	plain.used[40] = true
	if _, err := plain.Subset(); err != nil {
		t.Errorf("a CFF face with a VARC table was not subset for a glyph VARC does not compose: %v", err)
	}

	// Glyph 1 is composed by VARC of glyph 2; glyph 2 is a glyf composite of
	// glyph 1.
	f := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph(), fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 1, DX: 50}})},
		advances: []int{0, 500, 900},
		tuples: [][]fonttest.VarTuple{
			nil, {{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}}}, nil,
		},
		extra: map[string][]byte{"VARC": varcFanOut(1, 1)},
	}
	data := f.build(t)
	want := "glyph 2 has glyph 1 as a glyf component"
	if _, err := LoadInstance(data, map[string]float64{"wght": 900}); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("instancing it said %v, and should say %q", err, want)
	}
	face, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	face.used[2] = true
	if _, err := face.Subset(); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("subsetting it said %v, and should say %q", err, want)
	}
}

// TestAVARCOverCFF2IsRefusedNotDropped: a CFF2 font is instanced as the CFF
// font it draws, and VARC is among the tables an instance drops, since the
// glyf path writes VARC glyphs out first. Nothing writes out a composite
// over CFF2 leaves, so such an instance is refused, and a subset of the font
// as it stands refuses its VARC glyphs, rather than either drawing each
// composite as its empty base glyph.
func TestAVARCOverCFF2IsRefusedNotDropped(t *testing.T) {
	tables := font.SFNTTables(harfbuzzFont(t, "CFF2Blend.otf"))
	varc := font.SFNTTables(harfbuzzFont(t, "VarComposite.ttf"))["VARC"]
	if tables == nil || tables["CFF2"] == nil || varc == nil {
		t.Fatal("the fixtures are not a CFF2 font and a VARC table")
	}
	tables["VARC"] = varc
	data := assembleOTTO(tables)

	if _, err := LoadInstance(data, map[string]float64{"wght": 700}); err == nil ||
		!strings.Contains(err.Error(), "variable composite glyphs (VARC) are built on CFF2 outlines") {
		t.Errorf("a CFF2 font with a VARC table was instanced, with %v", err)
	}
	// The control: the same font without VARC is instanced.
	delete(tables, "VARC")
	if _, err := LoadInstance(assembleOTTO(tables), map[string]float64{"wght": 700}); err != nil {
		t.Errorf("a CFF2 font with no VARC table was refused: %v", err)
	}

	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.varc == nil {
		t.Fatal("the VARC table was not read, so this watches nothing")
	}
	for gid := 0; gid < f.NumGlyphs(); gid++ {
		if f.varc.t.coverageIndex(gid) >= 0 {
			f.used[gid] = true
			if _, err := f.Subset(); err == nil {
				t.Errorf("glyph %d, a variable composite over CFF2 outlines, was subset", gid)
			}
			return
		}
	}
	t.Fatal("no glyph of the font is a variable composite, so this watches nothing")
}
