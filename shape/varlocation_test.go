package shape

import (
	"os"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A location is reached as HarfBuzz reaches it.
//
// Every table of a variable face is read at one location in normalized
// coordinates, and there were three readings of it here: the glyf path's,
// unrounded; the CFF2 path's, rounded once to 2.14 as fontTools rounds it; and
// VARC's, HarfBuzz's own. They put the same face at three places a hair apart,
// and a hair is a font unit on a point in a hundred: Noto Sans at weight 850,
// read unrounded, advanced 75 glyphs and drew 704 simple glyphs a unit away
// from HarfBuzz. There is now one (f2Dot14Location), and these hold it to
// HarfBuzz.

// harfBuzzLocations are the 2.14 coordinates HarfBuzz 14.5.0 (uharfbuzz 0.56.2)
// reports for a location — hb_font_set_variations, then
// hb_font_get_var_coords_normalized — each chosen because a plausible reading
// gets it wrong.
var harfBuzzLocations = []struct {
	why  string
	file string
	loc  map[string]float64
	want []int
}{
	{"avar maps weight 700 through a 16.16 rounding on each side and a half rounding up to 2.14; " +
		"fontTools, mapping in double precision and rounding once, reaches 9994",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wght": 700}, []int{9995, 0}},
	{"the unrounded 0.894995 is not a 2.14 value",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wght": 850}, []int{14664, 0}},
	{"two axes at once, one of them mapped by avar to exactly a half",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wght": 300, "wdth": 80}, []int{-8192, -9284}},
	{"an axis named alone leaves the other at its default",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wdth": 71.3}, []int{0, -12924}},
	{"a coordinate past what a float holds clamps to the axis's end",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wght": 1e300}, []int{16384, 0}},
	{"and past the other end",
		"../fonts/notosans/NotoSans-Variable.ttf", map[string]float64{"wght": -1e300}, []int{-16384, 0}},
	// -5/131072 is -2.5 in 16.16. HarfBuzz rounds with floorf(x + .5f), to
	// -2, which is 0 in 2.14; C's roundf takes it to -3, which is -1.
	{"a negative half rounds towards +infinity, not away from zero",
		"../testdata/harfbuzz/fonts/VarComposite.ttf", map[string]float64{"0000": -5.0 / 131072}, []int{0, 0, 0}},
	{"and a positive half rounds up",
		"../testdata/harfbuzz/fonts/VarComposite.ttf", map[string]float64{"0000": 5.0 / 131072}, []int{0, 1, 0}},
	{"a face with no avar",
		"../testdata/harfbuzz/fonts/VariedAxes.ttf",
		map[string]float64{"wght": 333, "wdth": 77, "opsz": 33, "slnt": -7}, []int{-3659, -7537, 2395, -7646, 0}},
}

// TestALocationIsNormalizedAsHarfBuzzNormalizesIt reads each location every way
// this package reads one — for a glyf instance, for a CFF2 one, for VARC, and
// on the face LoadInstance returns — and requires HarfBuzz's coordinates from
// each, to the bit.
func TestALocationIsNormalizedAsHarfBuzzNormalizesIt(t *testing.T) {
	for _, tc := range harfBuzzLocations {
		t.Run(tc.why, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			tables := font.SFNTTables(data)
			axes, err := parseFvar(tables["fvar"])
			if err != nil {
				t.Fatal(err)
			}
			glyf, err := normalizeLocation(axes, tables["avar"], tc.loc)
			if err != nil {
				t.Fatal(err)
			}
			cff2, _, err := cff2Location(tables["fvar"], tables["avar"], tc.loc)
			if err != nil {
				t.Fatal(err)
			}
			f, err := LoadInstance(data, tc.loc)
			if err != nil {
				t.Fatal(err)
			}
			varc := hbNormalizedCoords(tables["fvar"], tables["avar"], tc.loc)
			for i, want := range tc.want {
				for _, got := range []struct {
					how string
					v   float64
				}{
					{"normalizeLocation", glyf[i]},
					{"cff2Location", cff2[i]},
					{"the instance's face", f.varCoords[i]},
					{"hbNormalizedCoords", float64(varc[i]) / 16384},
				} {
					if got.v*16384 != float64(want) {
						t.Errorf("axis %d: %s reads %v×16384 and HarfBuzz %d", i, got.how, got.v*16384, want)
					}
				}
			}
		})
	}
}

// TestAnInstanceDrawsWhereHarfBuzzDraws holds Noto Sans at weight 850 to what
// HarfBuzz states for the variable face there — advances, which HVAR moves, and
// ink boxes, which are the outline's extremes, which gvar moves. Each of these
// glyphs was a unit away from HarfBuzz while the location was read unrounded.
func TestAnInstanceDrawsWhereHarfBuzzDraws(t *testing.T) {
	f, err := LoadInstance(notoSansBytes(t), map[string]float64{"wght": 850})
	if err != nil {
		t.Fatal(err)
	}
	_, _, n := instancedOutlines(t, f)
	advances, _ := instancedMetrics(t, f, n)
	for _, w := range []struct {
		name    string
		gid     int
		advance int
	}{
		{"k", 78, 649}, {"y", 92, 600}, {"braceleft", 94, 423}, {"Phi", 370, 933}, {"uni042C", 452, 635},
	} {
		if advances[w.gid] != w.advance {
			t.Errorf("%s (glyph %d) advances %d, and HarfBuzz %d", w.name, w.gid, advances[w.gid], w.advance)
		}
	}
	for _, w := range []struct {
		name string
		gid  int
		ink  [4]int
	}{
		{"three", 22, [4]int{35, 724, 507, -734}},
		{"E", 40, [4]int{80, 714, 421, -714}},
		{"H", 43, [4]int{80, 714, 605, -714}},
		{"U", 56, [4]int{76, 714, 605, -724}},
		{"bracketleft", 62, [4]int{60, 729, 242, -894}},
	} {
		x, y, width, height, ok := f.GlyphExtents(w.gid)
		if got := [4]int{x, y, width, height}; !ok || got != w.ink {
			t.Errorf("%s (glyph %d) has ink %v, and HarfBuzz %v", w.name, w.gid, got, w.ink)
		}
	}
}
