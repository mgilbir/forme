package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The morx and mort reader (morx.go) held to HarfBuzz over the text-rendering
// tests' morx suite, every case of which HarfBuzz sets as the suite expects:
// the rearrangement, contextual, ligature, noncontextual and insertion
// subtables, chains and their flags, the subtables that walk the run
// backwards, glyphs deleted and inserted, and state machines that loop. And
// over the faces morx_fixture.py builds: what the suite does not reach, the
// features a caller asks for (aatfeatures.go), and a mort. The answers are
// checked in as morx.expected.txt; see morx.py.

// morxCase is one case of morx.expected.txt.
type morxCase struct {
	test, font, sum string
	text            string
	// on and off are the features the case asks for; ordered says it only
	// turns them on, in the order named, which ShapeGlyphsWith asks them in.
	on, off []string
	ordered bool
	// language is the run's language, where the case sets one after an @:
	// lang says it does, and language may be empty, which is no language.
	language string
	lang     bool
	fails    bool
	glyphs   [][5]int
}

// morxFixtures are the faces of morx.expected.txt that morx_fixture.py builds
// into testdata/harfbuzz/fonts; the rest are the suite's, in aat/fonts.
var morxFixtures = []string{"MorxCases.ttf", "MorxFeatures.ttf", "MorxFeaturesDeprecated.ttf", "MorxFeaturesNoFeat.ttf", "MortCases.ttf", "MorxLanguage.ttf"}

func readMorxGolden(t *testing.T) []morxCase {
	t.Helper()
	file, err := os.Open(filepath.Join(harfbuzzDir, "morx.expected.txt"))
	if err != nil {
		t.Fatalf("%v; run `make hbmorx`", err)
	}
	defer file.Close()
	var cases []morxCase
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			t.Fatalf("%q: a test, a font, its sum, code points and features", line)
		}
		c := morxCase{test: f[0], font: f[1], sum: f[2]}
		for _, cp := range strings.Split(f[3], ",") {
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			c.text += string(rune(r))
		}
		spec, language, lang := strings.Cut(f[4], "@")
		c.language, c.lang = language, lang
		if spec != "." {
			c.ordered = true
			for _, feature := range strings.Split(spec, ",") {
				if feature[0] == '+' {
					c.on = append(c.on, feature[1:])
				} else {
					c.off = append(c.off, feature[1:])
					c.ordered = false
				}
			}
		}
		if len(f) == 6 && f[5] == "fails" {
			c.fails = true
		} else {
			for _, g := range f[5:] {
				var v [5]int
				parts := strings.Split(g, ",")
				if len(parts) != 5 {
					t.Fatalf("%q: a glyph is five numbers", line)
				}
				for i, p := range parts {
					if v[i], err = strconv.Atoi(p); err != nil {
						t.Fatalf("%q: %v", line, err)
					}
				}
				c.glyphs = append(c.glyphs, v)
			}
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// shapeMorxCase shapes a case as morx.py asked HarfBuzz to: with ShapeGlyphsWith
// where it only turns features on, in its order; otherwise with Features,
// turned off and on.
func shapeMorxCase(f *Face, c morxCase) []Glyph {
	if c.lang {
		glyphs, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{Tags: strings.Join(c.on, ","),
			TagsOff: strings.Join(c.off, ","), Language: c.language})
		return glyphs
	}
	if c.ordered {
		glyphs, _ := f.ShapeGlyphsWith(c.text, c.on...)
		return glyphs
	}
	on, off := slices.Sorted(slices.Values(c.on)), slices.Sorted(slices.Values(c.off))
	glyphs, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{Tags: strings.Join(on, ","), TagsOff: strings.Join(off, ",")})
	return glyphs
}

func TestMorxAgreesWithHarfBuzz(t *testing.T) {
	cases := readMorxGolden(t)
	if len(cases) < 230 {
		t.Fatalf("morx.expected.txt holds %d cases; run `make hbmorx`", len(cases))
	}
	faces := map[string]*Face{}
	for _, c := range cases {
		f, ok := faces[c.font]
		if !ok {
			dir := filepath.Join(harfbuzzDir, "aat", "fonts")
			if slices.Contains(morxFixtures, c.font) {
				dir = filepath.Join(harfbuzzDir, "fonts")
			}
			data, err := os.ReadFile(filepath.Join(dir, c.font))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != c.sum {
				t.Fatalf("%s is %s, and the expectations were made from %s; run `make hbmorx`", c.font, got, c.sum)
			}
			if f, err = Load(data); err != nil {
				t.Fatalf("%s: %v", c.font, err)
			}
			if f.morx == nil {
				t.Fatalf("%s has no morx or mort read", c.font)
			}
			faces[c.font] = f
		}
		glyphs := shapeMorxCase(f, c)
		if c.fails {
			continue
		}
		units := func(v float64) int { return int(math.Round(v * float64(f.unitsPerEm) / 1000)) }
		var got [][5]int
		for _, g := range glyphs {
			got = append(got, [5]int{g.GID, g.Cluster, units(g.XAdvance), units(g.XOffset), units(g.YOffset)})
		}
		label := c.test + " " + c.font + " " + strconv.Quote(c.text) + " +" + strings.Join(c.on, ",+") + " -" + strings.Join(c.off, ",-")
		if len(got) != len(c.glyphs) {
			t.Errorf("%s: %d glyphs %v, HarfBuzz %d %v", label, len(got), got, len(c.glyphs), c.glyphs)
			continue
		}
		for i := range got {
			if got[i] != c.glyphs[i] {
				t.Errorf("%s glyph %d: %v, HarfBuzz %v\n  forme    %v\n  HarfBuzz %v", label, i, got[i], c.glyphs[i], got, c.glyphs)
				break
			}
		}
	}
}

// TestAMorxRunsGlyphsAreItsOwn: the output a morx writes into is the face's,
// kept from run to run, and a run's glyphs may end in it; so it is handed
// back only once the run is done with it, and the array a run returns is
// never written by the next. Every case of morx.expected.txt is shaped on one
// face per font, and each run's glyphs are what they were once every other
// case has been shaped on its face.
func TestAMorxRunsGlyphsAreItsOwn(t *testing.T) {
	type kept struct {
		label         string
		glyphs, saved []Glyph
	}
	var runs []kept
	faces := map[string]*Face{}
	for _, c := range readMorxGolden(t) {
		f, ok := faces[c.font]
		if !ok {
			dir := filepath.Join(harfbuzzDir, "aat", "fonts")
			if slices.Contains(morxFixtures, c.font) {
				dir = filepath.Join(harfbuzzDir, "fonts")
			}
			data, err := os.ReadFile(filepath.Join(dir, c.font))
			if err != nil {
				t.Fatal(err)
			}
			if f, err = Load(data); err != nil {
				t.Fatalf("%s: %v", c.font, err)
			}
			faces[c.font] = f
		}
		glyphs := shapeMorxCase(f, c)
		runs = append(runs, kept{c.font + " " + strconv.Quote(c.text), glyphs, slices.Clone(glyphs)})
	}
	for _, r := range runs {
		if !slices.Equal(r.glyphs, r.saved) {
			t.Errorf("%s: the run's glyphs became %v once later runs were shaped, and were %v",
				r.label, r.glyphs, r.saved)
		}
	}
}

// TestAGlyphSetHoldsWhatAMapWould: the set of glyphs a morx run has held,
// which decides whether a subtable is tried at all, holds each id added once
// and no other, across the whole sixteen-bit range and outside it, and is
// empty again once reset, as the map it replaced was. The morx comparison
// with HarfBuzz runs few glyphs of low ids, which no two bits of a word
// would tell apart.
func TestAGlyphSetHoldsWhatAMapWould(t *testing.T) {
	var s glyphSet
	seed := uint32(7)
	for round := range 50 {
		s.reset()
		want := map[int]bool{}
		for range 1 + round*20 {
			seed = seed*1664525 + 1013904223
			gid := int(seed>>8) % 0x10000
			switch seed % 7 {
			case 0:
				gid = 0xFFFF
			case 1:
				gid = 0x10000 + int(seed%5)
			case 2:
				gid = -1 - int(seed%3)
			case 3:
				gid &^= 63 // the first bit of a word
			}
			s.add(gid)
			want[gid] = true
		}
		got := map[int]bool{}
		for _, gid := range s.gids {
			if got[gid] {
				t.Fatalf("round %d: %d is in the set twice", round, gid)
			}
			got[gid] = true
		}
		if !maps.Equal(got, want) {
			t.Fatalf("round %d: the set holds %d ids, and %d were added", round, len(got), len(want))
		}
	}
}
