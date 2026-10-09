package shape

import (
	"encoding/binary"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// Stretching a glyph, MathML Core §5.3, in exact arithmetic on a face built
// for it.
//
// The face's glyphs are rectangles of chosen sizes and its constructions are
// chosen so that each step of the algorithm is decided by a number a reader
// can check by hand: which of the glyph, its variants and its assembly is
// taken for a target, how many times an extender is repeated, how much the
// parts overlap, and where each one is drawn.

const (
	gParen = 1 + iota
	gParenV1
	gParenV2
	gParenBot
	gParenExt
	gParenTop
	gArrow
	gArrowLeft
	gArrowExt
	gArrowRight
	gBrace
	gBar
	gBarExt
)

func stretchFace(t *testing.T, opts fonttest.MathOptions) (*Face, *MathTable) {
	t.Helper()
	glyph := func(r rune, adv int, ink [4]int) fonttest.Glyph {
		return fonttest.Glyph{Rune: r, Advance: adv, HasShape: true, Ink: ink}
	}
	part := func(adv int, ink [4]int) fonttest.Glyph {
		return fonttest.Glyph{Advance: adv, HasShape: true, Ink: ink, Unmapped: true}
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			glyph('(', 300, [4]int{0, -200, 300, 800}),
			part(350, [4]int{0, -400, 350, 1100}),
			part(400, [4]int{0, -600, 400, 1400}),
			part(500, [4]int{0, 0, 500, 800}),
			part(450, [4]int{0, 0, 450, 600}),
			part(500, [4]int{0, 0, 500, 800}),
			glyph('→', 1000, [4]int{0, 200, 1000, 400}),
			part(400, [4]int{0, 250, 400, 350}),
			part(600, [4]int{0, 240, 600, 360}),
			part(500, [4]int{0, 150, 500, 450}),
			glyph('{', 300, [4]int{0, -200, 300, 800}),
			glyph('|', 200, [4]int{80, -100, 120, 700}),
			// An extender whose ink begins below its origin.
			part(200, [4]int{80, -50, 120, 450}),
		},
		Extra: map[string][]byte{"MATH": fonttest.MATH(opts)},
	})
	face, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	m, err := face.MathTable()
	if err != nil || m == nil {
		t.Fatalf("MathTable() = %v, %v", m, err)
	}
	return face, m
}

func stretchOptions() fonttest.MathOptions {
	return fonttest.MathOptions{
		Constants:           map[string]int{},
		ItalicsCorrection:   map[int]int{gParenV1: 17},
		MinConnectorOverlap: 50,
		VertVariants: map[int][]fonttest.MathVariant{
			gParen: {{Glyph: gParenV1, Advance: 1500}, {Glyph: gParenV2, Advance: 2000}},
			gBrace: {{Glyph: gParenV1, Advance: 1500}},
		},
		VertAssembly: map[int]fonttest.MathAssembly{
			gParen: {ItalicsCorrection: 9, Parts: []fonttest.MathPart{
				{Glyph: gParenBot, Start: 0, End: 200, Full: 800},
				{Glyph: gParenExt, Start: 300, End: 300, Full: 600, Extender: true},
				{Glyph: gParenTop, Start: 200, End: 0, Full: 800},
			}},
			// No extender: not an assembly MathML Core builds.
			gBrace: {Parts: []fonttest.MathPart{
				{Glyph: gParenBot, Start: 0, End: 200, Full: 800},
				{Glyph: gParenTop, Start: 200, End: 0, Full: 800},
			}},
			gBar: {Parts: []fonttest.MathPart{
				{Glyph: gBarExt, Start: 100, End: 100, Full: 500, Extender: true},
			}},
		},
		HorizAssembly: map[int]fonttest.MathAssembly{
			gArrow: {ItalicsCorrection: -4, Parts: []fonttest.MathPart{
				{Glyph: gArrowLeft, Start: 0, End: 100, Full: 400},
				{Glyph: gArrowExt, Start: 150, End: 150, Full: 600, Extender: true},
				{Glyph: gArrowRight, Start: 100, End: 0, Full: 500},
			}},
		},
	}
}

func TestStretchTakesTheFirstThatReaches(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	for _, tc := range []struct {
		what   string
		target float64
		glyph  int
		ic     int
		asc    float64
		desc   float64
	}{
		// The glyph's ink is a thousand units tall.
		{"the glyph itself, when its ink is tall enough", 1000, gParen, 0, 800, 200},
		// Past it, the variants by the advance the table states, not by
		// their ink: the first variant's ink is 1500 and its advance 1500.
		{"the first variant that reaches", 1001, gParenV1, 17, 1100, 400},
		{"a variant reaches exactly", 1500, gParenV1, 17, 1100, 400},
		{"the second variant", 1501, gParenV2, 0, 1400, 600},
	} {
		s, ok := m.Stretch(gParen, true, tc.target)
		if !ok || len(s.Parts) != 0 || s.Glyph != tc.glyph || s.ItalicsCorrection != tc.ic ||
			s.Ascent != tc.asc || s.Descent != tc.desc {
			t.Errorf("%s: Stretch(%v) = %+v (%v), want glyph %d ic %d ascent %v descent %v",
				tc.what, tc.target, s, ok, tc.glyph, tc.ic, tc.asc, tc.desc)
		}
	}
	if _, ok := m.Stretch(gArrow, true, 5000); ok {
		t.Error("the arrow has no vertical construction, and stretching it down succeeded")
	}
	if _, ok := m.Stretch(gParenBot, true, 5000); ok {
		t.Error("a part has no construction of its own, and stretching it succeeded")
	}
}

// TestAGlyphIsMeasuredAsStretchMeasuresIt: Glyph is Stretch's answer for one
// glyph — its advance, its ink either side of the baseline, its italics
// correction — for any glyph, a size variant included.
func TestAGlyphIsMeasuredAsStretchMeasuresIt(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	for _, gid := range []int{gParen, gParenV1, gParenV2, gArrow} {
		want := m.single(gid)
		if got := m.Glyph(gid); got.Glyph != gid || len(got.Parts) != 0 || got.Width != want.Width ||
			got.Ascent != want.Ascent || got.Descent != want.Descent || got.ItalicsCorrection != want.ItalicsCorrection {
			t.Errorf("Glyph(%d) = %+v, want %+v", gid, got, want)
		}
	}
	if g := m.Glyph(gParenV1); g.Width != 350 || g.Ascent != 1100 || g.Descent != 400 || g.ItalicsCorrection != 17 {
		t.Errorf("the first variant is measured as %+v, want 350 wide, 1100 up, 400 down and leaning 17", g)
	}
}

// TestStretchBuildsTheAssembly is §5.3.1's arithmetic for a target of 2500:
// the ends are 800 each (1600) and the extender 600, the least overlap 50, so
//
//	rmin = ceil((2500 − 1600 + 50·(2 − 1)) / (600 − 50)) = ceil(950/550) = 2,
//
// four glyphs; the even share of an assembly with no overlap is
// (1600 + 2·600 − 2500) / 3 = 100, under every joining connector (200, 300,
// 300, 200), so the overlap is 100 and the assembly is exactly 2500 tall.
func TestStretchBuildsTheAssembly(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	s, ok := m.Stretch(gParen, true, 2500)
	if !ok {
		t.Fatal("no stretch")
	}
	want := []MathStretchPart{
		{Glyph: gParenBot, Y: 0},
		{Glyph: gParenExt, Y: 700},
		{Glyph: gParenExt, Y: 1200},
		{Glyph: gParenTop, Y: 1700},
	}
	if !slices.Equal(s.Parts, want) {
		t.Errorf("parts = %v, want %v", s.Parts, want)
	}
	if s.Width != 500 || s.Ascent != 2500 || s.Descent != 0 || s.ItalicsCorrection != 9 || s.Capped {
		t.Errorf("stretch = %+v, want width 500, ascent 2500, descent 0, italics 9", s)
	}

	// A target the connectors cannot meet exactly: one extender of 500 whose
	// connectors are 100 long, to 1200. rmin = ceil((1200 − 0 + 50·(0 − 1)) /
	// 450) = 3; the even share would be (1500 − 1200)/2 = 150, but no
	// connector allows more than 100, so the three overlap by 100 and the bar
	// is 1300 tall. The extender's ink begins 50 below its origin, and its
	// bottom — not its origin — is what goes at each point reached.
	s, _ = m.Stretch(gBar, true, 1200)
	want = []MathStretchPart{{Glyph: gBarExt, Y: 50}, {Glyph: gBarExt, Y: 450}, {Glyph: gBarExt, Y: 850}}
	if !slices.Equal(s.Parts, want) || s.Ascent != 1300 || s.Width != 200 {
		t.Errorf("bar = %+v, want parts %v and 1300 tall", s, want)
	}
}

// TestStretchAcrossBuildsTheAssembly is the same arithmetic along the line,
// to 2000: ends 400 and 500 (900), the extender 600, so rmin =
// ceil((2000 − 900 + 50) / 550) = 3, five glyphs, and the even share
// (900 + 1800 − 2000) / 4 = 175 is more than the shortest joining connector,
// 100, which is the overlap. The arrow is then 900 + 1800 − 400 = 2300 long.
func TestStretchAcrossBuildsTheAssembly(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	if s, ok := m.Stretch(gArrow, false, 1000); !ok || s.Glyph != gArrow || len(s.Parts) != 0 {
		t.Errorf("the arrow is 1000 across and is what a target of 1000 takes; got %+v", s)
	}
	s, _ := m.Stretch(gArrow, false, 2000)
	want := []MathStretchPart{
		{Glyph: gArrowLeft, X: 0},
		{Glyph: gArrowExt, X: 300},
		{Glyph: gArrowExt, X: 800},
		{Glyph: gArrowExt, X: 1300},
		{Glyph: gArrowRight, X: 1800},
	}
	if !slices.Equal(s.Parts, want) {
		t.Errorf("parts = %v, want %v", s.Parts, want)
	}
	// Across, the height is the tallest part's ink: 450 above, 0 below — the
	// right end reaches 150..450 and the rest sit higher up.
	if s.Width != 2300 || s.Ascent != 450 || s.Descent != -150 || s.ItalicsCorrection != -4 {
		t.Errorf("arrow = %+v, want 2300 across, ascent 450, descent -150, italics -4", s)
	}
}

// TestStretchFallsBackToTheLastTried: an assembly MathML Core refuses — here,
// one with no extender — leaves the last variant tried, short of the target.
func TestStretchFallsBackToTheLastTried(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	s, ok := m.Stretch(gBrace, true, 3000)
	if !ok || s.Glyph != gParenV1 || len(s.Parts) != 0 {
		t.Errorf("Stretch = %+v (%v), want the variant, the last thing tried", s, ok)
	}
}

// TestStretchIsCappedAndSaysSo: a target no assembly of MaxMathAssemblyGlyphs
// glyphs reaches is built with that many, at the least overlap — the longest
// it can be — and says it is short.
func TestStretchIsCappedAndSaysSo(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	s, _ := m.Stretch(gParen, true, 1e9)
	if !s.Capped || len(s.Parts) != MaxMathAssemblyGlyphs {
		t.Fatalf("%d parts, capped %v; want %d and capped", len(s.Parts), s.Capped, MaxMathAssemblyGlyphs)
	}
	reps := float64(MaxMathAssemblyGlyphs - 2)
	if want := 1600 + reps*600 - 50*float64(MaxMathAssemblyGlyphs-1); s.Ascent != want {
		t.Errorf("capped assembly is %v tall, want %v", s.Ascent, want)
	}
}

func TestPreferredStretchWidthIsTheWidest(t *testing.T) {
	_, m := stretchFace(t, stretchOptions())
	// The glyph 300, its variants 350 and 400, its parts 500 and 450.
	if got := m.PreferredStretchWidth(gParen); got != 500 {
		t.Errorf("PreferredStretchWidth = %v, want 500", got)
	}
	// The brace's assembly is refused — it has no extender — so its parts are
	// not a width it can be drawn at: the glyph 300 and its variant 350.
	if got := m.PreferredStretchWidth(gBrace); got != 350 {
		t.Errorf("PreferredStretchWidth(brace) = %v, want 350", got)
	}
	if got := m.PreferredStretchWidth(gArrow); got != 1000 {
		t.Errorf("the arrow has no vertical construction; its width is its advance, got %v", got)
	}
}

// TestMathTableBoundsAreReported: a list longer than this reader walks is
// cut or refused, and Limits names it once.
func TestMathTableBoundsAreReported(t *testing.T) {
	opts := stretchOptions()
	var many []fonttest.MathVariant
	for i := 0; i < maxMathVariants+6; i++ {
		many = append(many, fonttest.MathVariant{Glyph: gParenV1, Advance: 100 + i})
	}
	opts.VertVariants[gParen] = many
	var parts []fonttest.MathPart
	for i := 0; i <= maxMathAssemblyParts; i++ {
		parts = append(parts, fonttest.MathPart{Glyph: gArrowExt, Start: 100, End: 100, Full: 300, Extender: true})
	}
	opts.HorizAssembly[gArrow] = fonttest.MathAssembly{Parts: parts}
	_, m := stretchFace(t, opts)
	if got := len(m.Variants(gParen, true)); got != maxMathVariants {
		t.Errorf("%d variants, want the first %d", got, maxMathVariants)
	}
	m.Variants(gParen, true)
	if _, ok := m.Assembly(gArrow, false); ok {
		t.Error("an assembly of too many parts was built")
	}
	limits := strings.Join(m.Limits(), "\n")
	if strings.Count(limits, "size variants") != 1 || strings.Count(limits, "parts for glyph") != 1 {
		t.Errorf("Limits() = %q, want each bound named once", limits)
	}
}

// TestAMalformedMathTableIsRefusedInPart: offsets and counts that run past the
// table leave that part unread and say so, and never read out of range.
func TestAMalformedMathTableIsRefusedInPart(t *testing.T) {
	table := fonttest.MATH(stretchOptions())
	face := func(tbl []byte) (*MathTable, error) {
		data := fonttest.SFNT(fonttest.SFNTOptions{
			Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500}},
			Extra:  map[string][]byte{"MATH": tbl},
		})
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		return f.MathTable()
	}
	if _, err := face(table[:8]); err == nil {
		t.Error("a table shorter than its header was read")
	}
	v2 := append([]byte(nil), table...)
	binary.BigEndian.PutUint16(v2, 2)
	if _, err := face(v2); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("a version 2 table: %v", err)
	}
	past := append([]byte(nil), table...)
	binary.BigEndian.PutUint16(past[4:], uint16(len(past)-10)) // MathConstants
	binary.BigEndian.PutUint16(past[8:], uint16(len(past)-4))  // MathVariants
	m, err := face(past)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Constant(MathAxisHeight); ok {
		t.Error("a constant was read from a subtable that runs past the table")
	}
	if m.HasConstruction(gParen, true) {
		t.Error("a construction was read from a subtable that runs past the table")
	}
	if got := strings.Join(m.Limits(), "\n"); !strings.Contains(got, "MathConstants") || !strings.Contains(got, "MathVariants") {
		t.Errorf("Limits() = %q, want both refusals", got)
	}

	// A coverage naming more glyphs than there are records: the italics
	// correction of glyph 2 is stated by the coverage and by no record. It
	// is not stated, rather than read from whatever follows the records.
	short := fonttest.MATH(fonttest.MathOptions{ItalicsCorrection: map[int]int{1: 11, 2: 22}})
	info := int(binary.BigEndian.Uint16(short[6:]))
	italics := info + int(binary.BigEndian.Uint16(short[info:]))
	binary.BigEndian.PutUint16(short[italics+2:], 1) // italicsCorrectionCount
	m, err = face(short)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := m.ItalicsCorrection(1); !ok || v != 11 {
		t.Errorf("ItalicsCorrection(1) = %d (%v), want 11", v, ok)
	}
	if v, ok := m.ItalicsCorrection(2); ok {
		t.Errorf("ItalicsCorrection(2) = %d, and no record states it", v)
	}
}

// FuzzMathTable reads a MATH table of arbitrary bytes and asks it everything,
// for glyphs in range and out: nothing may panic or read out of range.
func FuzzMathTable(f *testing.F) {
	f.Add(fonttest.MATH(stretchOptions()))
	f.Add([]byte{0, 1, 0, 0, 0, 10, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, tbl []byte) {
		data := fonttest.SFNT(fonttest.SFNTOptions{
			Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
			Extra:  map[string][]byte{"MATH": tbl},
		})
		face, err := Load(data)
		if err != nil {
			return
		}
		m, err := face.MathTable()
		if err != nil || m == nil {
			return
		}
		for c := MathConstant(-1); c <= MathConstantCount; c++ {
			m.Constant(c)
		}
		for _, g := range []int{-1, 0, 1, 2, 70000} {
			m.ItalicsCorrection(g)
			m.TopAccentAttachment(g)
			m.IsExtendedShape(g)
			for c := MathKernCorner(-1); c <= 4; c++ {
				m.Kern(g, c, 0)
			}
			for _, v := range []bool{false, true} {
				m.Variants(g, v)
				m.Assembly(g, v)
				m.Stretch(g, v, 3000)
			}
			m.PreferredStretchWidth(g)
		}
		m.Limits()
	})
}

// TestScriptOffsetsAreOS2s: ySubscriptYOffset and ySuperscriptYOffset, which
// MathML Core falls back to where a face has no MATH table, and nothing where
// the face has no OS/2 table long enough to state them.
func TestScriptOffsetsAreOS2s(t *testing.T) {
	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2[16:], uint16(0xFF00)) // -256
	binary.BigEndian.PutUint16(os2[24:], 300)
	for _, tc := range []struct {
		table    []byte
		sub, sup int
		ok       bool
	}{
		{os2, -256, 300, true},
		{os2[:25], 0, 0, false},
		{nil, 0, 0, false},
	} {
		extra := map[string][]byte{}
		if tc.table != nil {
			extra["OS/2"] = tc.table
		}
		f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500}}, Extra: extra}))
		if err != nil {
			t.Fatal(err)
		}
		sub, sup, ok := f.ScriptOffsets()
		if sub != tc.sub || sup != tc.sup || ok != tc.ok {
			t.Errorf("an OS/2 of %d bytes: ScriptOffsets() = %d, %d, %v; want %d, %d, %v",
				len(tc.table), sub, sup, ok, tc.sub, tc.sup, tc.ok)
		}
	}
}

// manyVariantsFace is stretchFace with a parenthesis of more size variants
// than are considered, which Variants notes in Limits the first time a table
// is asked for them.
func manyVariantsFace(t *testing.T) *Face {
	t.Helper()
	opts := stretchOptions()
	var many []fonttest.MathVariant
	for i := 0; i < maxMathVariants+6; i++ {
		many = append(many, fonttest.MathVariant{Glyph: gParenV1, Advance: 100 + i})
	}
	opts.VertVariants[gParen] = many
	f, _ := stretchFace(t, opts)
	return f
}

// TestMathTableIsReadOnceAndHandedOutApart: a face reads its MATH table and
// its table directory once, for it and its clones, and each MathTable handed
// out is the table read, its own face's, with Limits of its own — what one
// caller's reading noted is not another's.
func TestMathTableIsReadOnceAndHandedOutApart(t *testing.T) {
	f := manyVariantsFace(t)
	first, err := f.MathTable()
	if err != nil || first == nil {
		t.Fatalf("MathTable: %v, %v", first, err)
	}
	if f.math.t == nil {
		t.Fatal("the table read is not kept")
	}
	first.Variants(gParen, true)
	if !strings.Contains(strings.Join(first.Limits(), "\n"), "size variants") {
		t.Fatalf("Limits() = %q, and Variants noted nothing", first.Limits())
	}
	fresh, err := f.readMathTable()
	if err != nil {
		t.Fatal(err)
	}
	clone := f.Clone()
	for _, face := range []*Face{f, clone} {
		again, err := face.MathTable()
		if err != nil {
			t.Fatal(err)
		}
		if again == first {
			t.Fatal("the same MathTable was handed out twice")
		}
		if len(again.Limits()) != 0 {
			t.Errorf("a table asked for after another noted a limit has Limits() = %q", again.Limits())
		}
		if again.face != face {
			t.Error("a table handed out is not its face's")
		}
		want := *fresh
		want.face = face
		if !reflect.DeepEqual(*again, want) {
			t.Errorf("the table handed out is %+v, and read afresh %+v", *again, want)
		}
	}
	if n := testing.AllocsPerRun(10, func() { _ = f.sfntTables() }); n != 0 {
		t.Errorf("taking the face apart into its tables allocated %v times; it is done once, at load", n)
	}
	if &f.sfntTables()["MATH"][0] != &font.SFNTTables(f.data)["MATH"][0] {
		t.Error("the tables kept are not the program's")
	}
}

// TestMathTableIsAskedForFromSeveralGoroutinesAtOnce asks a face and its
// clones for the table from goroutines at once, each reading glyphs from its
// own and noting a limit in it (run it with -race).
func TestMathTableIsAskedForFromSeveralGoroutinesAtOnce(t *testing.T) {
	f := manyVariantsFace(t)
	var wg sync.WaitGroup
	for i := range 8 {
		face := f
		if i%2 == 1 {
			face = f.Clone()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				m, err := face.MathTable()
				if err != nil || m == nil {
					t.Errorf("MathTable: %v, %v", m, err)
					return
				}
				if len(m.Limits()) != 0 {
					t.Errorf("a table handed out has Limits() = %q before it is read", m.Limits())
				}
				if len(m.Variants(gParen, true)) != maxMathVariants || len(m.Limits()) != 1 {
					t.Errorf("Limits() = %q", m.Limits())
				}
			}
		}()
	}
	wg.Wait()
}
