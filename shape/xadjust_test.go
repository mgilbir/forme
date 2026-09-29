package shape

import (
	"math"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// Glyph.XAdjust and YAdjust are the part of the advance shaping changed, so
// that XAdvance is always the font's own advance plus XAdjust. These tests hold
// the identity over everything the HarfBuzz oracles set, and hold the value to
// HarfBuzz's own.

// checkAdjustment holds a shaped run to the identity: each advance is the
// nominal one, plus its adjustment. The nominal advance is what HarfBuzz starts
// from — the glyph's horizontal advance in a run set across the page, and its
// vertical one, with no horizontal advance at all, in a run set upright.
func checkAdjustment(t *testing.T, f *Face, glyphs []Glyph, upright bool, what string) (adjusted int) {
	t.Helper()
	// Exact where the units convert exactly, and to the rounding of the
	// conversion where they cannot: a face whose em is not a power of two or a
	// thousand states its lengths in fractions that do not add exactly.
	exact := f.unitsPerEm == 1000 || f.unitsPerEm&(f.unitsPerEm-1) == 0
	same := func(a, b float64) bool {
		if exact {
			return a == b
		}
		return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(a))
	}
	for i, g := range glyphs {
		nomX, nomY := f.GlyphAdvance(g.GID), 0.0
		if upright {
			nomX = 0
			nomY, _, _ = f.GlyphVerticalMetrics(g.GID)
		}
		if !same(g.XAdvance, nomX+g.XAdjust) {
			t.Errorf("%s glyph %d (%d): XAdvance %v is not the nominal %v plus XAdjust %v",
				what, i, g.GID, g.XAdvance, nomX, g.XAdjust)
		}
		if !same(g.YAdvance, nomY+g.YAdjust) {
			t.Errorf("%s glyph %d (%d): YAdvance %v is not the nominal %v plus YAdjust %v",
				what, i, g.GID, g.YAdvance, nomY, g.YAdjust)
		}
		if g.XAdjust != 0 || g.YAdjust != 0 {
			adjusted++
		}
	}
	return adjusted
}

// TestAdjustmentIsTheAdvanceLessTheNominalOverTheOracleCorpora shapes every
// string of every HarfBuzz corpus — Latin, Arabic, Khmer, Javanese, Balinese,
// Tibetan and the face of ignorable characters — alone and between neighbours,
// which is where kerning across a run's edge is, and holds each glyph to the
// identity. A corpus the identity is trivially true of proves nothing, so the
// three that have adjustments (kerning, and the cursive and mark positioning of
// Arabic and Tibetan) must show some; the others state none, and say so.
func TestAdjustmentIsTheAdvanceLessTheNominalOverTheOracleCorpora(t *testing.T) {
	// What each corpus is known to adjust, well under what it does.
	mustAdjust := map[string]int{"latin": 5000, "arabic": 100, "tibetan": 50}
	for _, tc := range harfbuzzCases {
		t.Run(tc.name, func(t *testing.T) {
			corpus, _, header := readHarfBuzzGolden(t, tc.corpus, tc.expected)
			f := harfbuzzFace(t, tc.font, header)
			var glyphsSeen, adjusted int
			for _, s := range corpus {
				for _, ctx := range [][2]string{{"", ""}, {"A", "V"}, {"V", "A"}} {
					glyphs, _ := f.ShapeGlyphsInContext(s, ctx[0], ctx[1], Features{})
					glyphsSeen += len(glyphs)
					adjusted += checkAdjustment(t, f, glyphs, false, describeRunes(s))
					if t.Failed() {
						return
					}
				}
			}
			t.Logf("%d glyphs, %d of them adjusted", glyphsSeen, adjusted)
			if min := mustAdjust[tc.name]; adjusted < min {
				t.Errorf("%d glyphs of %s were adjusted, want at least %d: the identity was hardly tried",
					adjusted, tc.corpus, min)
			}
		})
	}
}

// The same over the upright corpora, in which the advance is the vertical one.
func TestAdjustmentIsTheAdvanceLessTheNominalOverTheUprightCorpora(t *testing.T) {
	corpus, featured, faces := readVerticalGolden(t)
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			f := loadVerticalFace(t, want)
			var adjusted int
			for _, s := range corpus {
				glyphs, _ := f.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
				adjusted += checkAdjustment(t, f, glyphs, true, describeRunes(s))
			}
			for _, l := range featured {
				glyphs, _ := f.ShapeGlyphsInContext(l.text, "", "", Features{Vertical: true, Tags: l.tags})
				adjusted += checkAdjustment(t, f, glyphs, true, l.tags+" "+describeRunes(l.text))
			}
			t.Logf("%d glyphs adjusted", adjusted)
		})
	}
}

// hbAdjusted is HarfBuzz's x_advance less hb_font_get_glyph_h_advance for each
// glyph of a string, at a scale of one unit per font unit, which is the
// adjustment its shaping made. Generated with uharfbuzz over the bundled face:
//
//	font = hb.Font(hb.Face(hb.Blob(data))); font.set_variations(location)
//	hb.shape(font, buf); pos.x_advance - font.get_glyph_h_advance(info.codepoint)
//
// at the location, for each of the strings below.
var hbAdjusted = []struct {
	location map[string]float64
	text     string
	gids     []int
	adjust   []int
}{
	{nil, "AVATAR", []int{36, 57, 36, 55, 36, 53}, []int{-40, -40, -70, -70, 0, 0}},
	{nil, "Yo", []int{60, 82}, []int{-50, 0}},
	{nil, "fi fl", []int{1654, 3, 1655}, []int{0, 0, 0}},
	{nil, "To", []int{55, 82}, []int{-70, 0}},
	{nil, "Te.", []int{55, 72, 17}, []int{-70, 0, 0}},
	{nil, "V,", []int{57, 15}, []int{-50, 0}},
	{nil, "P.A.", []int{51, 17, 36, 17}, []int{-130, 0, 0, 0}},
	{nil, "A V", []int{36, 3, 57}, []int{0, 0, 0}},
	{nil, string([]rune{0x57, 0x78, 0x301, 0x76}) /* "Wx" + acute + "v" */, []int{58, 91, 2665, 89}, []int{0, 0, 0, 0}},
	// The advances at the location are the nominal ones, HVAR's included: the
	// kerns move with it, and the advances do not count as adjustment.
	{map[string]float64{"wght": 700}, "AVATAR", []int{36, 57, 36, 55, 36, 53}, []int{-40, -40, -70, -70, 0, 0}},
	{map[string]float64{"wght": 700}, "P.A.", []int{51, 17, 36, 17}, []int{-130, 0, 0, 0}},
	{map[string]float64{"wght": 100, "wdth": 75}, "AVATAR", []int{36, 57, 36, 55, 36, 53}, []int{-15, -15, -33, -33, 0, 0}},
	{map[string]float64{"wght": 100, "wdth": 75}, "Yo", []int{60, 82}, []int{-36, 0}},
	{map[string]float64{"wght": 100, "wdth": 75}, "To", []int{55, 82}, []int{-50, 0}},
	{map[string]float64{"wght": 100, "wdth": 75}, "P.A.", []int{51, 17, 36, 17}, []int{-94, 0, 0, 0}},
}

// TestAdjustmentIsHarfBuzzsOwn holds XAdjust to what HarfBuzz says its shaping
// changed, for the strings the issue names ("AVATAR", "Yo", "fi fl") and a few
// more, at the default location and at two others where every advance is the
// varied one.
func TestAdjustmentIsHarfBuzzsOwn(t *testing.T) {
	data := notoSansBytes(t)
	for _, c := range hbAdjusted {
		f, err := Load(data)
		if c.location != nil {
			f, err = LoadInstance(data, c.location)
		}
		if err != nil {
			t.Fatal(err)
		}
		glyphs, _ := f.ShapeGlyphs(c.text)
		if len(glyphs) != len(c.gids) {
			t.Errorf("%v %q: %d glyphs, want %d", c.location, c.text, len(glyphs), len(c.gids))
			continue
		}
		for i, g := range glyphs {
			if g.GID != c.gids[i] || g.XAdjust != f.scale(c.adjust[i]) {
				t.Errorf("%v %q glyph %d: glyph %d adjusted by %v, HarfBuzz has glyph %d adjusted by %d",
					c.location, c.text, i, g.GID, g.XAdjust, c.gids[i], c.adjust[i])
			}
		}
		checkAdjustment(t, f, glyphs, false, c.text)
	}
}

// A mark whose advance shaping cancels has the advance it was given as its
// adjustment, negated, so that the identity holds for it: HarfBuzz zeroes the
// advance, and 0 less the nominal advance is what its x_advance is over
// hb_font_get_glyph_h_advance. A glyph a substitution made is nominal again.
func TestAMarksCancelledAdvanceIsItsAdjustment(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 0x0301, Advance: 300, HasShape: true},
	}})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs(string([]rune{0x61, 0x301}))
	if len(glyphs) != 2 {
		t.Fatalf("%d glyphs", len(glyphs))
	}
	if g := glyphs[0]; g.XAdvance != 500 || g.XAdjust != 0 {
		t.Errorf("the base advances %v with adjustment %v, want 500 and 0", g.XAdvance, g.XAdjust)
	}
	if g := glyphs[1]; g.XAdvance != 0 || g.XAdjust != -300 {
		t.Errorf("the mark advances %v with adjustment %v, want 0 and -300", g.XAdvance, g.XAdjust)
	}
	checkAdjustment(t, f, glyphs, false, "a + acute")

	// A model that trusts the font to give its marks no width leaves the
	// cancelling to the fallback that places the marks itself, which is
	// where a mark in a Hangul run is cancelled.
	// A base with no ink has nothing to place the mark against, and the
	// cancelling is done by another branch of the same fallback.
	for _, inked := range []bool{true, false} {
		hangul, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
			{Rune: 0xAC00, Advance: 900, HasShape: inked},
			{Rune: 0x0301, Advance: 300, HasShape: true},
		}}))
		if err != nil {
			t.Fatal(err)
		}
		glyphs, _ = hangul.ShapeGlyphs(string([]rune{0xAC00, 0x0301}))
		if len(glyphs) != 2 {
			t.Fatalf("%d glyphs", len(glyphs))
		}
		if g := glyphs[1]; g.XAdvance != 0 || g.XAdjust != -300 {
			t.Errorf("inked base %v: a combining mark advances %v with adjustment %v, want 0 and -300",
				inked, g.XAdvance, g.XAdjust)
		}
		checkAdjustment(t, hangul, glyphs, false, "a Hangul syllable and a combining mark")
	}
}

// The legacy 'kern' table adjusts the advance too, across the line and, in a
// run set upright, down it. The table is applied as HarfBuzz applies it: half
// the number on the first glyph and the rest on the second (see
// legacykern_test.go), and each half is an adjustment.
func TestALegacyKernTablesAdjustmentIsRecorded(t *testing.T) {
	across := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)
	glyphs, _ := across.ShapeGlyphs("AV")
	half := float64(lkTighten / 2)
	if len(glyphs) != 2 || glyphs[0].XAdjust != half || glyphs[1].XAdjust != half {
		t.Fatalf("AV adjusted by %+v, want %v on each glyph", glyphs, half)
	}
	if n := checkAdjustment(t, across, glyphs, false, "AV"); n != 2 {
		t.Errorf("%d glyphs adjusted, want both", n)
	}

	// A vertical subtable is applied where 'vkrn' is asked for and the face
	// lists it somewhere in its layout tables (legacykern.go), which a GSUB
	// with one feature and a substitution that changes nothing does.
	down := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: 0, // vertical
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)
	down = withGSUB(t, down, fonttest.GSUBSingle("vkrn", []int{lkV}, []int{lkV}))
	glyphs, _ = down.ShapeGlyphs("AV")
	if n := checkAdjustment(t, down, glyphs, false, "AV across a vertical table"); n != 0 {
		t.Errorf("a vertical table adjusted %d glyphs of a horizontal run", n)
	}
	glyphs, _ = down.ShapeGlyphsInContext("AV", "", "", Features{Vertical: true, Tags: "vkrn"})
	if len(glyphs) != 2 || glyphs[0].YAdjust != half || glyphs[1].YAdjust != half {
		t.Fatalf("AV set upright adjusted by %+v, want %v on each glyph down the line", glyphs, half)
	}
	if glyphs[0].XAdjust != 0 || glyphs[1].XAdjust != 0 {
		t.Errorf("an upright run's kerning moved the pen across the line: %+v", glyphs)
	}
	checkAdjustment(t, down, glyphs, true, "AV upright")
}

// A separator the face has no glyph for is set in the face's space glyph at the
// separator's own width, which is a change to the advance the font states and
// so is adjustment: HarfBuzz sets it by overwriting x_advance.
func TestAStandInSpacesWidthIsAdjustment(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: ' ', Advance: 250},
	}})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs(string([]rune{0x61, 0x2003, 0x61})) // an em space, which the face lacks
	if len(glyphs) != 3 {
		t.Fatalf("%d glyphs", len(glyphs))
	}
	g := glyphs[1]
	if g.XAdvance != 1000 || g.XAdjust != 750 {
		t.Errorf("the em space advances %v with adjustment %v, want 1000 and 750", g.XAdvance, g.XAdjust)
	}
	checkAdjustment(t, f, glyphs, false, "a em-space a")
}

// Cursive attachment states the advance outright: a glyph stops at its exit
// point and the next takes the rest, in a run across the page whichever way it
// is written, and down it. What it overwrites was the nominal advance, so what
// it did is the adjustment.
func TestACursiveJointsAdvanceIsAdjustment(t *testing.T) {
	across := cursiveFace(t, 0, joinedAnchors())
	glyphs, _ := across.ShapeGlyphs("abc")
	if n := checkAdjustment(t, across, glyphs, false, "abc"); n != 3 {
		t.Errorf("%d glyphs adjusted, want all three, each of which moved by a joint", n)
	}
	if want := float64(aExitX - curAdvance); glyphs[0].XAdjust != want {
		t.Errorf("a's adjustment is %v, want %v: it stops at its exit point", glyphs[0].XAdjust, want)
	}

	// Written right to left: Arabic letters, whose script runs that way, which
	// is what makes the run's direction the joint's. And down the page, where
	// 'curs' has to be asked for.
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "CursiveRTL",
		Glyphs: []fonttest.Glyph{
			{Rune: 0x0628, Advance: curAdvance, HasShape: true},
			{Rune: 0x062A, Advance: curAdvance, HasShape: true},
			{Rune: 0x062B, Advance: curAdvance, HasShape: true},
		},
		Extra: map[string][]byte{"GPOS": fonttest.GPOSCursive(joinedAnchors(), 0)},
	})
	rtl, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ = rtl.ShapeGlyphs(string([]rune{0x0628, 0x062A, 0x062B}))
	if n := checkAdjustment(t, rtl, glyphs, false, "a right-to-left joint"); n == 0 {
		t.Error("a right-to-left joint adjusted nothing, so its branch was not reached")
	}

	glyphs, _ = across.ShapeGlyphsInContext("abc", "", "", Features{Vertical: true, Tags: "curs"})
	if n := checkAdjustment(t, across, glyphs, true, "a joint down the page"); n == 0 {
		t.Error("a joint down the page adjusted nothing, so its branch was not reached")
	}
}

// withGSUB is the face of legacyKernFace with a GSUB table added to its font.
func withGSUB(t *testing.T, f *Face, gsub []byte) *Face {
	t.Helper()
	tables := font.SFNTTables(f.Program())
	extra := map[string][]byte{"GSUB": gsub, "kern": tables["kern"]}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "LegacyKern",
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: lkAdvance, HasShape: true},
			{Rune: 'V', Advance: lkAdvance, HasShape: true},
		},
		Extra: extra,
	})
	out, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
