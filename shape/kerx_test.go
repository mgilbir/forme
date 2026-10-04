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

// AAT positioning (kerx.go) and tracking (trak.go) held to HarfBuzz, over the
// faces aatpos_fixture.py builds — every kerx subtable format, the plan that
// decides whether kerx positions a run at all, and tracking at sizes either
// side of a table's — and HarfBuzz's own tracking face, TRAK.ttf, at the sizes
// its tests ask. The answers are checked in as aatpos.expected.txt; see
// aatpos.py.

type aatPosCase struct {
	font, sum, text string
	size            float64
	kern            bool
	glyphs          [][5]int
}

func readAATPosGolden(t *testing.T) []aatPosCase {
	t.Helper()
	file, err := os.Open(filepath.Join(harfbuzzDir, "aatpos.expected.txt"))
	if err != nil {
		t.Fatalf("%v; run `make hbaatpos`", err)
	}
	defer file.Close()
	var cases []aatPosCase
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			t.Fatalf("%q: a face, its sum, code points, a size and kerning", line)
		}
		c := aatPosCase{font: f[0], sum: f[1], kern: f[4] == "1"}
		for _, cp := range strings.Split(f[2], ",") {
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			c.text += string(rune(r))
		}
		var err error
		if c.size, err = strconv.ParseFloat(f[3], 64); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
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
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestAATPositioningAgreesWithHarfBuzz(t *testing.T) {
	cases := readAATPosGolden(t)
	if len(cases) < 150 {
		t.Fatalf("aatpos.expected.txt holds %d cases; run `make hbaatpos`", len(cases))
	}
	faces := map[string]*Face{}
	for _, c := range cases {
		f, ok := faces[c.font]
		if !ok {
			dir := filepath.Join(harfbuzzDir, "fonts")
			if c.font == "TRAK.ttf" {
				dir = filepath.Join(harfbuzzDir, "aat-inhouse", "fonts")
			}
			data, err := os.ReadFile(filepath.Join(dir, c.font))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != c.sum {
				t.Fatalf("%s is %s, and the expectations were made from %s; run `make hbaatpos`", c.font, got, c.sum)
			}
			if f, err = Load(data); err != nil {
				t.Fatalf("%s: %v", c.font, err)
			}
			faces[c.font] = f
		}
		glyphs, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{NoKerning: !c.kern, PointSize: c.size})
		units := func(v float64) int { return int(math.Round(v * float64(f.unitsPerEm) / 1000)) }
		var got [][5]int
		for _, g := range glyphs {
			got = append(got, [5]int{g.GID, g.Cluster, units(g.XAdvance), units(g.XOffset), units(g.YOffset)})
		}
		label := c.font + " " + strconv.Quote(c.text) + " at " + strconv.FormatFloat(c.size, 'g', -1, 64)
		if !c.kern {
			label += ", kerning off"
		}
		if len(got) != len(c.glyphs) {
			t.Errorf("%s: %v, HarfBuzz %v", label, got, c.glyphs)
			continue
		}
		for i := range got {
			if got[i] != c.glyphs[i] {
				t.Errorf("%s glyph %d:\n  forme    %v\n  HarfBuzz %v", label, i, got, c.glyphs)
				break
			}
		}
	}
}
