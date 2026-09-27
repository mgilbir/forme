package shape

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
)

// The ink of colour glyphs, held to HarfBuzz.
//
// testdata/harfbuzz/colrink.py asks HarfBuzz for the extents of every glyph of
// two faces colrink_fixture.py builds: ColourInk.ttf, which paints with every
// paint format COLR has and does to a box each thing painting can do, and
// BitmapInk.ttf, a CBDT face whose tables say each thing HarfBuzz reads its
// extents from. It shapes a few strings in ColourInk across the page and down
// it, where HarfBuzz places the marks and hangs the glyphs by their painted
// boxes. The answers are checked in as colrink.expected.txt.
//
// Every glyph of every colour face in the Google Fonts tree was compared out of
// tree when colrink.go and bitmapink.go were written, and every one agrees.

// colourInkFaces reads each face the expectations were generated from. A name
// with a weight after an @ is the face loaded at that weight, which is how
// colrink.py names ColourInk measured away from its default.
var colourInkFaces = map[string]func(t *testing.T) []byte{
	"ColourInk.ttf":          func(t *testing.T) []byte { return harfbuzzFont(t, "ColourInk.ttf") },
	"ColourInk.ttf@wght=100": func(t *testing.T) []byte { return harfbuzzFont(t, "ColourInk.ttf") },
	"ColourInk.ttf@wght=650": func(t *testing.T) []byte { return harfbuzzFont(t, "ColourInk.ttf") },
	"ColourInk.ttf@wght=900": func(t *testing.T) []byte { return harfbuzzFont(t, "ColourInk.ttf") },
	"ColourInkStatic.ttf":    func(t *testing.T) []byte { return harfbuzzFont(t, "ColourInkStatic.ttf") },
	"BitmapInk.ttf":          func(t *testing.T) []byte { return harfbuzzFont(t, "BitmapInk.ttf") },
}

// colourInkStrings are the strings colrink.py shapes in ColourInk, in its
// order.
var colourInkStrings = []string{
	"Á",
	"Ạ",
	"Ḅ́",
	"Á́",
	"AB",
	"BẠB",
}

func readColourInkGolden(t *testing.T) []*hbInk {
	t.Helper()
	return readInkGolden(t, "colrink.expected.txt", "hbcolrink", len(colourInkFaces))
}

func loadColourInkFace(t *testing.T, want *hbInk) *Face {
	t.Helper()
	read, ok := colourInkFaces[want.name]
	if !ok {
		t.Fatalf("the expectations name %s, which this test does not know how to read", want.name)
	}
	data := read(t)
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want.sum {
		t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
			"Run `make hbcolrink` to regenerate them.", want.name, want.sum, got)
	}
	var f *Face
	var err error
	if _, weight, ok := strings.Cut(want.name, "@wght="); ok {
		w, perr := strconv.Atoi(weight)
		if perr != nil {
			t.Fatalf("%s: %v", want.name, perr)
		}
		f, err = LoadInstance(data, map[string]float64{"wght": float64(w)})
	} else {
		f, err = Load(data)
	}
	if err != nil {
		t.Fatalf("loading %s: %v", want.name, err)
	}
	return f
}

// TestColourInkAgreesWithHarfBuzz holds every glyph's extents to what HarfBuzz
// answers for it, including where it answers that there are none — ColourInk's
// at its default and at three weights, where its variable paints have moved.
func TestColourInkAgreesWithHarfBuzz(t *testing.T) {
	golden := readColourInkGolden(t)
	byName := map[string]*hbInk{}
	for _, want := range golden {
		byName[want.name] = want
	}
	// The weights are there for the paints that move with them, so each has
	// to be somewhere the default is not.
	for name, want := range byName {
		def, ok := byName["ColourInk.ttf"]
		if !strings.Contains(name, "@") || !ok {
			continue
		}
		moved := 0
		for gid, e := range want.extents {
			if d := def.extents[gid]; (d == nil) != (e == nil) || d != nil && *d != *e {
				moved++
			}
		}
		if moved == 0 {
			t.Errorf("%s measures every glyph as the default does, so it tests no variation", name)
		}
	}
	for _, want := range golden {
		t.Run(want.name, func(t *testing.T) {
			f := loadColourInkFace(t, want)
			if len(want.extents) != f.NumGlyphs() {
				t.Fatalf("%d glyphs compared of the face's %d", len(want.extents), f.NumGlyphs())
			}
			if strings.HasPrefix(want.name, "ColourInk.ttf") && f.colr == nil || want.name == "BitmapInk.ttf" && f.bitmap == nil {
				t.Fatalf("%s did not load with its colour table read", want.name)
			}
			for gid, w := range want.extents {
				got, ok := f.glyphExtents(gid)
				switch {
				case w == nil && ok:
					t.Errorf("glyph %d has extents %+v, and HarfBuzz has none", gid, got)
				case w != nil && !ok:
					t.Errorf("glyph %d has no extents, and HarfBuzz's are %+v", gid, *w)
				case w != nil && got != *w:
					t.Errorf("glyph %d has extents %+v, want %+v", gid, got, *w)
				}
			}
		})
	}
}

// TestColourInkPlacesMarksAsHarfBuzzDoes shapes ColourInk's strings, whose
// marks the face does not position and whose glyphs it states no vertical
// metrics for: across the page each mark is placed against its base's painted
// box, and down it each glyph is hung by its own.
func TestColourInkPlacesMarksAsHarfBuzzDoes(t *testing.T) {
	var want *hbInk
	for _, f := range readColourInkGolden(t) {
		if f.name == "ColourInk.ttf" {
			want = f
		}
	}
	if want == nil || len(want.across) != len(colourInkStrings) || len(want.down) != len(colourInkStrings) {
		t.Fatalf("the expectations do not hold the %d strings shaped both ways", len(colourInkStrings))
	}
	f := loadColourInkFace(t, want)
	for i, s := range colourInkStrings {
		glyphs, _ := f.ShapeGlyphs(s)
		if len(glyphs) != len(want.across[i]) {
			t.Errorf("%s: %d glyphs across, want %d", describeRunes(s), len(glyphs), len(want.across[i]))
			continue
		}
		for k, g := range glyphs {
			w := want.across[i][k]
			if g.GID != w.gid || f.units(g.XAdvance) != w.xAdvance ||
				f.units(g.XOffset) != w.dx || f.units(g.YOffset) != w.dy {
				t.Errorf("%s: glyph %d is %d advancing %d at (%d, %d), want %d advancing %d at (%d, %d)",
					describeRunes(s), k, g.GID, f.units(g.XAdvance), f.units(g.XOffset),
					f.units(g.YOffset), w.gid, w.xAdvance, w.dx, w.dy)
			}
		}
		down, _ := f.ShapeGlyphsInContext(s, "", "", Features{Vertical: true})
		if same, why := sameUpright(f, down, want.down[i]); !same {
			t.Errorf("%s down the page: %s", describeRunes(s), why)
		}
	}
	if len(want.verticalByGlyph) != f.NumGlyphs() {
		t.Fatalf("%d glyphs' vertical metrics compared of the face's %d", len(want.verticalByGlyph), f.NumGlyphs())
	}
	for gid, m := range want.verticalByGlyph {
		advance, x, y := f.verticalUnits(gid)
		if advance != m[0] || x != m[1] || y != m[2] {
			t.Errorf("glyph %d advances %d hung (%d, %d), want %d hung (%d, %d)",
				gid, advance, x, y, m[0], m[1], m[2])
		}
	}
}

// TestColourInkPaintsEachGlyphOnce: a colour glyph is painted the first time
// its ink is asked for and never again, by the face or a clone of it. The work
// charged is the measure, not a clock.
func TestColourInkPaintsEachGlyphOnce(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "ColourInk.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := f.GlyphID('A')
	if !ok {
		t.Fatal("the fixture has no A")
	}
	first, ok := f.glyphExtents(gid)
	if !ok {
		t.Fatal("A has no extents")
	}
	once := f.colr.budget.Spent()
	if once == 0 {
		t.Fatal("measuring A charged nothing, so this test measures nothing")
	}
	clone := f.Clone()
	for range 100 {
		if again, _ := f.glyphExtents(gid); again != first {
			t.Fatalf("A measured again is %+v, and was %+v", again, first)
		}
		clone.glyphExtents(gid)
		f.InkExtent("AAAA", 12)
	}
	if spent := f.colr.budget.Spent(); spent != once {
		t.Errorf("asking for A's ink 300 more times cost %d on top of the %d the first "+
			"time did; it should cost none", spent-once, once)
	}
}

// TestColourInkWorkIsBoundedAndReported measures every glyph of ColourInk
// against a budget too small for them — set in place of the face's own, which
// no table short of one built to reach HarfBuzz's edge limit on every glyph
// runs out of — and requires that painting stops within it, that a glyph
// measured after it ran out is measured by what was painted of it rather than
// as HarfBuzz measures it, and that the face says so.
func TestColourInkWorkIsBoundedAndReported(t *testing.T) {
	data := harfbuzzFont(t, "ColourInk.ttf")
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if limits := f.LayoutLimits(); len(limits) != 0 {
		t.Fatalf("a face that has measured nothing reports %q", limits)
	}
	const budget = 200
	f.colr.once.Do(func() {
		f.colr.t = readCOLR(f.colr.table, f.varCoords)
		f.colr.budget = font.NewBudget(budget)
	})
	fresh, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	differ := 0
	for gid := range f.NumGlyphs() {
		got, gotOK := f.glyphExtents(gid)
		want, wantOK := fresh.glyphExtents(gid)
		if got != want || gotOK != wantOK {
			differ++
		}
	}
	if differ == 0 {
		t.Fatal("no glyph measured differently for the budget running out, so this test measures nothing")
	}
	if spent := f.colr.budget.Spent(); spent > budget {
		t.Errorf("painting spent %d, past the budget of %d", spent, budget)
	}
	if limits := fresh.LayoutLimits(); len(limits) != 0 {
		t.Errorf("the face measured within its own budget reports %q", limits)
	}
	limits := strings.Join(f.LayoutLimits(), "\n")
	if !strings.Contains(limits, "colour glyphs ran past the work one face may cost") {
		t.Errorf("LayoutLimits does not say the budget ran out; it says %q", limits)
	}
}
