package shape

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// A glyph's own vertical metrics, held to HarfBuzz and to what shaping starts
// an upright run from.
//
// GlyphVerticalMetrics is what a writer states for a glyph of vertical text —
// PDF's W2 — and a run set upright is drawn from Glyph.YAdvance, VOriginX and
// VOriginY. If the two were read differently, a page would state one advance
// for a glyph in its font and move the pen by another, so the accessor is held
// to both: to HarfBuzz's hb_font_get_glyph_v_advance and v_origin for every
// glyph the oracles ask about, and to a run of the glyph alone for every glyph
// a character reaches.
//
// The faces are the ones the oracles already hold: every one the vertical
// oracle reads — vmtx and VORG, vmtx alone, neither, CFF and TrueType — the
// CFF fixture, whose glyphs are hung by their charstrings' ink, the colour
// fixture, hung by what it paints, and the variable faces loaded at several
// weights, where the instance's VVAR and gvar say.

// verticalMetricsFace is a face some oracle holds, and HarfBuzz's vertical
// advance and origin for its glyphs, as the oracles write them: the advance
// positive, down the page.
type verticalMetricsFace struct {
	name    string
	load    func(t *testing.T) *Face
	metrics map[int][3]int
}

func verticalMetricsFaces(t *testing.T) []verticalMetricsFace {
	t.Helper()
	var out []verticalMetricsFace
	_, _, vertical := readVerticalGolden(t)
	for _, want := range vertical {
		out = append(out, verticalMetricsFace{want.name,
			func(t *testing.T) *Face { return loadVerticalFace(t, want) }, want.metrics})
	}
	for _, want := range readCFFInkGolden(t) {
		if want.name == "CFFInk.otf" {
			out = append(out, verticalMetricsFace{want.name,
				func(t *testing.T) *Face { return loadCFFInkFace(t, want) }, want.verticalByGlyph})
		}
	}
	for _, want := range readColourInkGolden(t) {
		if want.name == "ColourInk.ttf" {
			out = append(out, verticalMetricsFace{want.name,
				func(t *testing.T) *Face { return loadColourInkFace(t, want) }, want.verticalByGlyph})
		}
	}
	for _, want := range readVerticalInstanceGolden(t) {
		for _, loc := range want.locations {
			if len(loc.metrics) == 0 {
				continue // the kerned faces, which the oracle shapes and does not measure
			}
			out = append(out, verticalMetricsFace{fmt.Sprintf("%s@wght=%v", want.name, loc.weight),
				func(t *testing.T) *Face { return want.loadAt(t, loc) }, loc.metrics})
		}
	}
	return out
}

// TestGlyphVerticalMetricsAgreeWithHarfBuzz holds the accessor to HarfBuzz for
// every glyph an oracle asks about, in the units GlyphAdvance answers in.
func TestGlyphVerticalMetricsAgreeWithHarfBuzz(t *testing.T) {
	for _, c := range verticalMetricsFaces(t) {
		t.Run(c.name, func(t *testing.T) {
			f := c.load(t)
			if len(c.metrics) == 0 {
				t.Fatal("no glyph was compared")
			}
			for gid, m := range c.metrics {
				advance, x, y := f.GlyphVerticalMetrics(gid)
				if advance != -f.scale(m[0]) || x != f.scale(m[1]) || y != f.scale(m[2]) {
					t.Errorf("glyph %d advances %v hung (%v, %v), want %v hung (%v, %v)",
						gid, advance, x, y, -f.scale(m[0]), f.scale(m[1]), f.scale(m[2]))
				}
			}
		})
	}
}

// TestGlyphVerticalMetricsAreWhatShapingStartsFrom shapes every character each
// face maps, alone and upright, and holds the glyph it is set in to the
// accessor: where it is hung, which nothing in shaping changes; and its
// advance, shaped again with every feature the face offers turned off, since
// a positioning feature may change it — Noto Sans JP's GPOS 'vert' shortens
// U+3127's, which is the font's rule for the run and not the glyph's metric.
//
// A mark's advance is taken away by shaping, as HarfBuzz takes it away, so for
// a mark — by the face's GDEF or, where it has none, by Unicode — only the
// origin is compared; and a character set in more than one glyph, or in none,
// is not a glyph alone.
func TestGlyphVerticalMetricsAreWhatShapingStartsFrom(t *testing.T) {
	for _, c := range verticalMetricsFaces(t) {
		t.Run(c.name, func(t *testing.T) {
			f := c.load(t)
			runes := make([]rune, 0, len(f.prog.Cmap))
			for r := range f.prog.Cmap {
				runes = append(runes, r)
			}
			sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
			plain := Features{Vertical: true, TagsOff: strings.Join(f.Features(), ",")}
			advances, origins := 0, 0
			glyphs := map[int]bool{}
			for _, r := range runes {
				got, _ := f.ShapeGlyphsInContext(string(r), "", "", Features{Vertical: true})
				if len(got) == 1 {
					g := got[0]
					_, x, y := f.GlyphVerticalMetrics(g.GID)
					glyphs[g.GID] = true
					if g.VOriginX != x || g.VOriginY != y {
						t.Errorf("%U is set in glyph %d hung (%v, %v), and the glyph says (%v, %v)",
							r, g.GID, g.VOriginX, g.VOriginY, x, y)
					}
					origins++
				}
				got, _ = f.ShapeGlyphsInContext(string(r), "", "", plain)
				if len(got) != 1 {
					continue
				}
				g := got[0]
				if isCombiningMark(r) || f.layout.classOf(g) == classMark {
					continue
				}
				glyphs[g.GID] = true
				if advance, _, _ := f.GlyphVerticalMetrics(g.GID); g.YAdvance != advance {
					t.Errorf("%U is set in glyph %d advancing %v, and the glyph says %v",
						r, g.GID, g.YAdvance, advance)
				}
				advances++
			}
			if advances == 0 || origins == 0 {
				t.Fatalf("%d advances and %d origins compared", advances, origins)
			}
			t.Logf("%d characters, %d glyphs: %d advances and %d origins compared",
				len(runes), len(glyphs), advances, origins)
		})
	}
}

// TestGlyphVerticalMetricsOfNoGlyph: a glyph index the face does not have
// answers zero, as GlyphAdvance answers it, and so does every glyph index of a
// face that has none.
func TestGlyphVerticalMetricsOfNoGlyph(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "CFFInk.otf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range []int{-1, f.NumGlyphs(), f.NumGlyphs() + 1000} {
		if a, x, y := f.GlyphVerticalMetrics(gid); a != 0 || x != 0 || y != 0 {
			t.Errorf("glyph %d, which the face does not have, advances %v hung (%v, %v)", gid, a, x, y)
		}
	}
	if a, _, _ := f.GlyphVerticalMetrics(1); a == 0 {
		t.Error("glyph 1 advances nothing, so the zeroes above say nothing")
	}
	// A standard face is set by character code and has no glyph indices: the
	// program it would be read from is not there.
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range []int{0, 1, 36} {
		if a, x, y := std.GlyphVerticalMetrics(gid); a != 0 || x != 0 || y != 0 {
			t.Errorf("a standard face's glyph %d advances %v hung (%v, %v)", gid, a, x, y)
		}
	}
}

// TestCentredVerticalOriginsIsHarfBuzzsSynthesis holds CentredVerticalOrigins
// to HarfBuzz's own origins over every face the oracles measure. Where it says
// the face's glyphs are centred, each glyph HarfBuzz hangs has its ink centred
// in the line it names, up to the half unit HarfBuzz's floor moves it up — an
// empty glyph's box too, which is its baseline — or, where its ink cannot be
// read, is hung from the ascender, the line's top; and a face it calls centred
// that states no vertical advances advances by the line. Where it says they are not, the
// face states its own origins: VORG, or vmtx over TrueType outlines.
func TestCentredVerticalOriginsIsHarfBuzzsSynthesis(t *testing.T) {
	centredFaces, hungFaces := 0, 0
	for _, c := range verticalMetricsFaces(t) {
		t.Run(c.name, func(t *testing.T) {
			f := c.load(t)
			line, centred := f.CentredVerticalOrigins()
			v := &f.vert
			if states := v.vorg != nil || v.longMetrics > 0 && v.glyf != nil; states == centred {
				t.Fatalf("centred is %v for a face whose VORG is %v and vmtx records %d over glyf %v",
					centred, v.vorg != nil, v.longMetrics, v.glyf != nil)
			}
			if !centred {
				hungFaces++
				return
			}
			centredFaces++
			ascender, _ := f.fontExtentsUnits()
			lineUnits := line * float64(f.unitsPerEm) / 1000
			for gid, m := range c.metrics {
				if !f.StatesVerticalMetrics() && math.Abs(float64(m[0])-lineUnits) > 1e-9 {
					t.Errorf("glyph %d advances %d, and the line is %g", gid, m[0], lineUnits)
				}
				e, ok := f.glyphExtents(gid)
				if !ok {
					if m[2] != ascender {
						t.Errorf("glyph %d's ink cannot be read and it is hung %d down, want the ascender, %d",
							gid, m[2], ascender)
					}
					continue
				}
				// Measured from the pen, y up: the ink's middle, and the line's.
				ink := float64(e.yBearing) + float64(e.height)/2 - float64(m[2])
				if d := ink + lineUnits/2; d < 0 || d > 0.5 {
					t.Errorf("glyph %d's ink is centred %g from the line's middle, want within half a unit above",
						gid, d)
				}
			}
		})
	}
	if centredFaces == 0 || hungFaces == 0 {
		t.Errorf("%d faces centred and %d hung their own: both kinds must be compared", centredFaces, hungFaces)
	}
}
