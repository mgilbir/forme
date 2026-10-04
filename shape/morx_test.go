package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The morx reader (morx.go) held to HarfBuzz over the text-rendering tests'
// morx suite, every case of which HarfBuzz sets as the suite expects: the
// rearrangement, contextual, ligature, noncontextual and insertion subtables,
// chains and their flags, the subtables that walk the run backwards, glyphs
// deleted and inserted, and state machines that loop. The answers are checked
// in as morx.expected.txt; see morx.py.

// morxCase is one case of morx.expected.txt.
type morxCase struct {
	test, font, sum string
	text            string
	fails           bool
	glyphs          [][5]int
}

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
		if len(f) < 4 {
			t.Fatalf("%q: a test, a font, its sum and code points", line)
		}
		c := morxCase{test: f[0], font: f[1], sum: f[2]}
		for _, cp := range strings.Split(f[3], ",") {
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			c.text += string(rune(r))
		}
		if len(f) == 5 && f[4] == "fails" {
			c.fails = true
		} else {
			for _, g := range f[4:] {
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

func TestMorxAgreesWithHarfBuzz(t *testing.T) {
	cases := readMorxGolden(t)
	if len(cases) < 170 {
		t.Fatalf("morx.expected.txt holds %d cases; run `make hbmorx`", len(cases))
	}
	faces := map[string]*Face{}
	for _, c := range cases {
		f, ok := faces[c.font]
		if !ok {
			dir := filepath.Join(harfbuzzDir, "aat", "fonts")
			if c.font == "MorxCases.ttf" {
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
				t.Fatalf("%s has no morx read", c.font)
			}
			faces[c.font] = f
		}
		glyphs, _ := f.ShapeGlyphs(c.text)
		if c.fails {
			continue
		}
		units := func(v float64) int { return int(math.Round(v * float64(f.unitsPerEm) / 1000)) }
		var got [][5]int
		for _, g := range glyphs {
			got = append(got, [5]int{g.GID, g.Cluster, units(g.XAdvance), units(g.XOffset), units(g.YOffset)})
		}
		if len(got) != len(c.glyphs) {
			t.Errorf("%s %q: %d glyphs %v, HarfBuzz %d %v", c.test, c.text, len(got), got, len(c.glyphs), c.glyphs)
			continue
		}
		for i := range got {
			if got[i] != c.glyphs[i] {
				t.Errorf("%s %q glyph %d: %v, HarfBuzz %v\n  forme    %v\n  HarfBuzz %v", c.test, c.text, i, got[i], c.glyphs[i], got, c.glyphs)
				break
			}
		}
	}
}
