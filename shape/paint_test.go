package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
)

// The tests here hold Face.PaintGlyph to HarfBuzz's hb_font_paint_glyph, call
// for call, over every glyph of the colour faces in testdata/harfbuzz: the
// fills of ColourPaint.ttf in three palettes and at three weights, every paint
// format and every thing painting does to a box in ColourInk.ttf, and the
// bitmaps of BitmapInk.ttf and SbixInk.ttf at sizes on, between and past their
// strikes. The answers are checked in as paint.expected.txt; see paint.py.

// recordingPainter writes each call as paint.py writes HarfBuzz's.
type recordingPainter struct{ lines []string }

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func colourText(c Color, foreground bool) string {
	fg := 0
	if foreground {
		fg = 1
	}
	return fmt.Sprintf("%d %d %d %d %d", c.R, c.G, c.B, c.A, fg)
}

var extendNames = map[Extend]string{ExtendPad: "pad", ExtendRepeat: "repeat", ExtendReflect: "reflect"}

func lineText(l ColorLine, geometry ...float64) string {
	var b strings.Builder
	b.WriteString(extendNames[l.Extend])
	for _, v := range geometry {
		b.WriteString(" " + num(v))
	}
	b.WriteString(" |")
	for i, s := range l.Stops {
		if i > 0 {
			b.WriteString(" |")
		}
		b.WriteString(" " + num(s.Offset) + " " + colourText(s.Color, s.Foreground))
	}
	return b.String()
}

func (r *recordingPainter) add(f string, a ...any) { r.lines = append(r.lines, fmt.Sprintf(f, a...)) }

func (r *recordingPainter) PushTransform(t Transform) {
	r.add("T %s %s %s %s %s %s", num(t.XX), num(t.YX), num(t.XY), num(t.YY), num(t.X0), num(t.Y0))
}
func (r *recordingPainter) PopTransform()       { r.add("t") }
func (r *recordingPainter) PushClipGlyph(g int) { r.add("CG %d", g) }
func (r *recordingPainter) PushClipRect(b Rect) {
	r.add("CR %s %s %s %s", num(b.XMin), num(b.YMin), num(b.XMax), num(b.YMax))
}
func (r *recordingPainter) PopClip()                    { r.add("c") }
func (r *recordingPainter) PushGroup()                  { r.add("G") }
func (r *recordingPainter) PopGroup(mode CompositeMode) { r.add("g %d", mode) }
func (r *recordingPainter) Solid(c Color, fg bool)      { r.add("S %s", colourText(c, fg)) }
func (r *recordingPainter) LinearGradient(g LinearGradient) {
	r.add("L %s", lineText(g.Line, g.P0.X, g.P0.Y, g.P1.X, g.P1.Y, g.P2.X, g.P2.Y))
}
func (r *recordingPainter) RadialGradient(g RadialGradient) {
	r.add("R %s", lineText(g.Line, g.C0.X, g.C0.Y, g.R0, g.C1.X, g.C1.Y, g.R1))
}
func (r *recordingPainter) SweepGradient(g SweepGradient) {
	r.add("W %s", lineText(g.Line, g.Center.X, g.Center.Y, g.StartAngle, g.EndAngle))
}
func (r *recordingPainter) Image(img Image) {
	sum := sha256.Sum256(img.Data)
	if img.Format == ImageSVG {
		if img.Width != 0 || img.Height != 0 || img.Box != (Rect{}) {
			r.add("I an SVG document with a size %d %d %v", img.Width, img.Height, img.Box)
			return
		}
		r.add("I 0 0 svg 0.0 none %d %s", len(img.Data), hex.EncodeToString(sum[:])[:16])
		return
	}
	if img.Format != ImagePNG {
		r.add("I unknown format %d", img.Format)
		return
	}
	r.add("I %d %d png 0.0 %s %s %s %s %d %s", img.Width, img.Height,
		num(img.Box.XMin), num(img.Box.YMax), num(img.Box.XMax-img.Box.XMin), num(img.Box.YMin-img.Box.YMax),
		len(img.Data), hex.EncodeToString(sum[:])[:16])
}

// paintCase is one face of paint.expected.txt, as it was painted, and the
// calls of each glyph painted: every glyph of a face in the tree, and some of
// a corpus face's.
type paintCase struct {
	name, sum string
	weight    int
	opts      PaintOptions
	gids      []int
	glyphs    [][]string
}

// paintFaces are the corpus faces paint.expected.txt paints, by the variable
// naming the directory each is fetched to; every other face is in the tree.
var paintFaces = map[string]string{"Noto-COLRv1.ttf": "EMOJI_FONTS", "NotoColorEmoji.ttf": "EMOJI_FONTS"}

// paintFont is a face of paint.expected.txt, and nil for a corpus face that has
// not been fetched.
func paintFont(t *testing.T, name string) []byte {
	t.Helper()
	env, corpus := paintFaces[name]
	if !corpus {
		return harfbuzzFont(t, name)
	}
	dir := os.Getenv(env)
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("%s names %s, which it does not hold: %v", env, name, err)
	}
	return data
}

func readPaintGolden(t *testing.T) []*paintCase {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "paint.expected.txt")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v; run `make hbpaint` to generate it", err)
	}
	defer file.Close()
	var cases []*paintCase
	var c *paintCase
	sc := bufio.NewScanner(file)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		switch {
		case f[0] == "face" && len(f) == 9:
			c = &paintCase{name: f[1], sum: f[8]}
			if name, w, ok := strings.Cut(f[1], "@wght="); ok {
				c.name = name
				if c.weight, err = strconv.Atoi(w); err != nil {
					t.Fatalf("%s: %v", line, err)
				}
			}
			p, err1 := strconv.Atoi(f[3])
			ppem, err2 := strconv.Atoi(f[5])
			if err1 != nil || err2 != nil {
				t.Fatalf("%s: a palette and a ppem", line)
			}
			c.opts = PaintOptions{Palette: p, PPEM: ppem, Foreground: Color{0x33, 0x66, 0x99, 0xCC}}
			if f[7] != "-" {
				c.opts.PaletteOverrides = map[int]Color{}
				for _, o := range strings.Split(f[7], "+") {
					index, rgba, ok := strings.Cut(o, "/")
					i, err1 := strconv.Atoi(index)
					v, err2 := strconv.ParseUint(rgba, 16, 32)
					if !ok || err1 != nil || err2 != nil || len(rgba) != 8 {
						t.Fatalf("%s: an override is index/RRGGBBAA", line)
					}
					c.opts.PaletteOverrides[i] = Color{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
				}
			}
			cases = append(cases, c)
		case c == nil:
			t.Fatalf("%q before any face", line)
		case f[0] == "G" && len(f) == 2:
			gid, err := strconv.Atoi(f[1])
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			c.gids = append(c.gids, gid)
			c.glyphs = append(c.glyphs, nil)
		case len(c.glyphs) == 0:
			t.Fatalf("%q before any glyph", line)
		default:
			c.glyphs[len(c.glyphs)-1] = append(c.glyphs[len(c.glyphs)-1], line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// sameCall compares a call as HarfBuzz made it and as PaintGlyph did, token by
// token, the numbers as numbers: HarfBuzz's are single precision written by
// Python, and these the same values written by Go.
func sameCall(want, got string) bool {
	w, g := strings.Fields(want), strings.Fields(got)
	if len(w) != len(g) {
		return false
	}
	for i := range w {
		if w[i] == g[i] {
			continue
		}
		a, err1 := strconv.ParseFloat(w[i], 64)
		b, err2 := strconv.ParseFloat(g[i], 64)
		if err1 != nil || err2 != nil || a != b {
			return false
		}
	}
	return true
}

func TestPaintGlyphAgreesWithHarfBuzz(t *testing.T) {
	cases := readPaintGolden(t)
	if len(cases) < 20 {
		t.Fatalf("paint.expected.txt holds %d faces; run `make hbpaint`", len(cases))
	}
	for _, c := range cases {
		label := fmt.Sprintf("%s@%d palette %d ppem %d", c.name, c.weight, c.opts.Palette, c.opts.PPEM)
		data := paintFont(t, c.name)
		if data == nil {
			t.Logf("%s: %s is not set, so it is not painted", label, paintFaces[c.name])
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != c.sum {
			t.Fatalf("the expectations were generated against %s %s and this one is %s.\n"+
				"Run `make hbpaint` to regenerate them.", c.name, c.sum, got)
		}
		var f *Face
		var err error
		if c.weight != 0 {
			f, err = LoadInstance(data, map[string]float64{"wght": float64(c.weight)})
		} else {
			f, err = Load(data)
		}
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if _, corpus := paintFaces[c.name]; !corpus && f.NumGlyphs() != len(c.glyphs) {
			t.Fatalf("%s: %d glyphs, and HarfBuzz painted %d", label, f.NumGlyphs(), len(c.glyphs))
		}
		for i, want := range c.glyphs {
			gid := c.gids[i]
			if f.BitmapOnly() && len(want) == 3 && want[0] == fmt.Sprintf("CG %d", gid) {
				// HarfBuzz fills the empty outline of a glyph with no image;
				// a face with no outlines paints nothing for it.
				want = nil
			}
			r := &recordingPainter{}
			if err := f.PaintGlyph(gid, c.opts, r); err != nil {
				t.Errorf("%s glyph %d: %v", label, gid, err)
				continue
			}
			for i := 0; i < max(len(want), len(r.lines)); i++ {
				var w, g string
				if i < len(want) {
					w = want[i]
				}
				if i < len(r.lines) {
					g = r.lines[i]
				}
				if !sameCall(w, g) {
					t.Errorf("%s glyph %d, call %d:\n  HarfBuzz %q\n  forme    %q", label, gid, i, w, g)
					break
				}
			}
		}
	}
}

// TestGlyphColourSaysWhatIsPainted holds GlyphColour to what PaintGlyph paints:
// a COLRv1 glyph's first call is never a bare fill, a COLRv0 glyph is fills of
// outlines, a bitmap is one image, and a glyph with no colour is its own
// outline in the foreground.
func TestGlyphColourSaysWhatIsPainted(t *testing.T) {
	seen := map[GlyphColour]bool{}
	for _, c := range readPaintGolden(t) {
		data := paintFont(t, c.name)
		if data == nil {
			continue
		}
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		for gid := range f.NumGlyphs() {
			r := &recordingPainter{}
			if err := f.PaintGlyph(gid, c.opts, r); err != nil {
				t.Fatal(err)
			}
			kind := f.GlyphColour(gid, c.opts.PPEM)
			seen[kind] = true
			plain := len(r.lines) == 3 && r.lines[0] == fmt.Sprintf("CG %d", gid) &&
				strings.HasSuffix(r.lines[1], " 1") && r.lines[2] == "c"
			image := len(r.lines) == 1 && strings.HasPrefix(r.lines[0], "I ")
			var ok bool
			switch kind {
			case ColourNone:
				ok = plain || f.BitmapOnly() && len(r.lines) == 0
			case ColourBitmap:
				ok = image && !strings.Contains(r.lines[0], " svg ")
			case ColourSVG:
				ok = image && strings.Contains(r.lines[0], " svg ")
			case ColourLayers:
				ok = !image && len(r.lines)%3 == 0
			case ColourPaint:
				ok = !image && !plain
			}
			if !ok {
				t.Errorf("%s glyph %d is %d and was painted as %q", c.name, gid, kind, r.lines)
			}
		}
	}
	for _, kind := range []GlyphColour{ColourNone, ColourPaint, ColourLayers, ColourBitmap, ColourSVG} {
		if !seen[kind] {
			t.Errorf("no glyph is %d, so this test does not reach it", kind)
		}
	}
}

// countingPainter counts the calls it is handed.
type countingPainter struct{ calls int }

func (c *countingPainter) PushTransform(Transform)       { c.calls++ }
func (c *countingPainter) PopTransform()                 { c.calls++ }
func (c *countingPainter) PushClipGlyph(int)             { c.calls++ }
func (c *countingPainter) PushClipRect(Rect)             { c.calls++ }
func (c *countingPainter) PopClip()                      { c.calls++ }
func (c *countingPainter) PushGroup()                    { c.calls++ }
func (c *countingPainter) PopGroup(CompositeMode)        { c.calls++ }
func (c *countingPainter) Solid(Color, bool)             { c.calls++ }
func (c *countingPainter) LinearGradient(LinearGradient) { c.calls++ }
func (c *countingPainter) RadialGradient(RadialGradient) { c.calls++ }
func (c *countingPainter) SweepGradient(SweepGradient)   { c.calls++ }
func (c *countingPainter) Image(Image)                   { c.calls++ }

// TestAGlyphPastItsBoundsIsRefusedWhole paints every COLR glyph of the two
// faces under a budget too small for some and enough for others, and requires
// that each is either painted as it is with the whole budget or refused with
// nothing handed to the painter: never painted in part.
func TestAGlyphPastItsBoundsIsRefusedWhole(t *testing.T) {
	refused, painted := 0, 0
	for _, name := range []string{"ColourPaint.ttf", "ColourInk.ttf"} {
		f, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		opts := PaintOptions{Foreground: Color{A: 255}}
		for gid := range f.NumGlyphs() {
			whole := &recordingPainter{}
			if ok, err := f.paintCOLR(f.colrTable(), gid, opts, whole, paintWork); !ok || err != nil {
				continue
			}
			short := &recordingPainter{}
			_, err := f.paintCOLR(f.colrTable(), gid, opts, short, 6)
			switch {
			case errors.Is(err, ErrPaintLimit):
				refused++
				if len(short.lines) != 0 {
					t.Errorf("%s glyph %d was refused after %d calls", name, gid, len(short.lines))
				}
			case err != nil:
				t.Errorf("%s glyph %d: %v", name, gid, err)
			default:
				painted++
				if strings.Join(short.lines, "\n") != strings.Join(whole.lines, "\n") {
					t.Errorf("%s glyph %d painted differently within a budget it fits in", name, gid)
				}
			}
		}
	}
	if refused == 0 || painted == 0 {
		t.Fatalf("%d refused and %d painted: the budget does not divide the glyphs, so this test measures nothing", refused, painted)
	}
}

// TestPaintingSpendsNothingOfTheFace paints every glyph of ColourInk many
// times and requires that measuring's budget, the face's, is not touched.
func TestPaintingSpendsNothingOfTheFace(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "ColourInk.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f.glyphExtents(0)
	before := f.colr.budget.Spent()
	for range 20 {
		for gid := range f.NumGlyphs() {
			if err := f.PaintGlyph(gid, PaintOptions{}, &countingPainter{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if spent := f.colr.budget.Spent(); spent != before {
		t.Errorf("painting spent %d of the face's measuring budget", spent-before)
	}
}

func TestPaintGlyphRefusesWhatItCannotPaint(t *testing.T) {
	f, err := Load(harfbuzzFont(t, "ColourPaint.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range []int{-1, f.NumGlyphs()} {
		c := &countingPainter{}
		if err := f.PaintGlyph(gid, PaintOptions{}, c); err == nil || c.calls != 0 {
			t.Errorf("glyph %d: %v after %d calls", gid, err, c.calls)
		}
		if got := f.GlyphColour(gid, 0); got != ColourNone {
			t.Errorf("glyph %d is %d", gid, got)
		}
	}
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if err := std.PaintGlyph(1, PaintOptions{}, &countingPainter{}); !errors.Is(err, ErrNoOutline) {
		t.Errorf("a standard face: %v", err)
	}
}

func TestColorIsAnImageColor(t *testing.T) {
	r, g, b, a := Color{R: 255, G: 128, B: 0, A: 128}.RGBA()
	if a != 0x8080 || r != 0x8080 || b != 0 || g != 0x4080 {
		t.Errorf("RGBA is %#x %#x %#x %#x", r, g, b, a)
	}
}

// quietPainter paints nothing and charges nothing: the walk alone.
type quietPainter struct{}

func (quietPainter) pushTransform(xform32) {}
func (quietPainter) popTransform()         {}
func (quietPainter) pushClipGlyph(int)     {}
func (quietPainter) pushClipRect(box32)    {}
func (quietPainter) popClip()              {}
func (quietPainter) pushGroup()            {}
func (quietPainter) popGroup(int)          {}
func (quietPainter) paint(paintFill)       {}

// stopCounter counts the colour stops PaintGlyph hands out.
type stopCounter struct {
	countingPainter
	stops int
}

func (s *stopCounter) LinearGradient(g LinearGradient) { s.stops += len(g.Line.Stops) }
func (s *stopCounter) RadialGradient(g RadialGradient) { s.stops += len(g.Line.Stops) }
func (s *stopCounter) SweepGradient(g SweepGradient)   { s.stops += len(g.Line.Stops) }

// TestPaintingIsChargedForTheStopsItHandsOut is #886: the counting walk
// charged a solid fill for a colour line it does not have, reading its colour
// index and alpha as an offset to one, and refused three of Noto Color Emoji's
// flags — a thousand solid fills each and more — that HarfBuzz paints whole.
// What the walk charges beyond the walk itself is the stops handed out, to the
// stop, for every glyph of the colour faces.
func TestPaintingIsChargedForTheStopsItHandsOut(t *testing.T) {
	for _, name := range []string{"ColourPaint.ttf", "ColourInk.ttf"} {
		f, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		tb := f.colrTable()
		for gid := range f.NumGlyphs() {
			counter := &paintCounter{t: tb, budget: font.NewBudget(paintWork)}
			if painted, _ := f.colr.paintGlyphWith(gid, counter, true, counter.budget); !painted {
				continue
			}
			walk := font.NewBudget(paintWork)
			f.colr.paintGlyphWith(gid, quietPainter{}, true, walk)
			handed := &stopCounter{}
			if err := f.PaintGlyph(gid, PaintOptions{}, handed); err != nil {
				t.Fatalf("%s glyph %d: %v", name, gid, err)
			}
			if got := counter.budget.Spent() - walk.Spent(); got != handed.stops {
				t.Errorf("%s glyph %d was charged %d for stops and handed out %d", name, gid, got, handed.stops)
			}
		}
	}
}

// TestEveryNotoColorEmojiGlyphPaints paints every glyph of Noto Color Emoji's
// COLRv1 build (EMOJI_FONTS), none of which reaches HarfBuzz's bounds, and
// requires that none is refused.
func TestEveryNotoColorEmojiGlyphPaints(t *testing.T) {
	data := paintFont(t, "Noto-COLRv1.ttf")
	if data == nil {
		t.Skip("EMOJI_FONTS is not set; run `make emoji-fonts`")
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	painted := 0
	for gid := range f.NumGlyphs() {
		if f.GlyphColour(gid, 0) != ColourPaint {
			continue
		}
		painted++
		if err := f.PaintGlyph(gid, PaintOptions{}, &countingPainter{}); err != nil {
			t.Errorf("glyph %d: %v", gid, err)
		}
	}
	if painted < 4000 {
		t.Errorf("%d COLRv1 glyphs, where the font has over four thousand", painted)
	}
}

// paintedText is a glyph painted within a budget of work, as recordingPainter
// writes it, and the error.
func paintedText(f *Face, gid int, opts PaintOptions, work int) string {
	r := &recordingPainter{}
	painted, err := f.paintCOLR(f.colrTable(), gid, opts, r, work)
	if !painted {
		return "not COLR"
	}
	return fmt.Sprintf("%v\n%s", err, strings.Join(r.lines, "\n"))
}

// TestAGlyphPaintedAgainIsPaintedAsItWasFirst paints every COLR glyph of the
// colour faces twice, in two palettes and within the whole budget and one too
// small for some: the second time from what counting it the first time found
// and kept, the first time by counting it. Each glyph is painted the same, or
// refused the same, both times; and the faces reach every verdict — refused,
// painted with a clip box, and with none both bounded and not — so that none
// of what is kept goes unread.
func TestAGlyphPaintedAgainIsPaintedAsItWasFirst(t *testing.T) {
	names := []string{"ColourPaint.ttf", "ColourInk.ttf"}
	if os.Getenv("EMOJI_FONTS") != "" {
		names = append(names, "Noto-COLRv1.ttf")
	}
	seen := map[paintVerdict]int{}
	for _, name := range names {
		f, err := Load(paintFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, work := range []int{paintWork, 6} {
			for _, palette := range []int{0, 1} {
				opts := PaintOptions{Palette: palette, Foreground: Color{R: 1, G: 2, B: 3, A: 255}}
				first := make([]string, f.NumGlyphs())
				for gid := range f.NumGlyphs() {
					first[gid] = paintedText(f, gid, opts, work)
				}
				for gid := range f.NumGlyphs() {
					if again := paintedText(f, gid, opts, work); again != first[gid] {
						t.Errorf("%s glyph %d within %d, palette %d: painted\n%s\nand again\n%s", name, gid, work, palette, first[gid], again)
					}
				}
			}
		}
		for _, v := range f.colr.verdicts {
			seen[v]++
		}
	}
	// A refusal is reached whatever the glyph's bounds: the tree's own faces
	// refuse only glyphs with a ClipBox, and Noto's (under EMOJI_FONTS) also
	// ones without, so the refusal is asked for by itself.
	refused := 0
	for v, n := range seen {
		if v.painted && v.refused {
			refused += n
		}
	}
	if refused == 0 {
		t.Errorf("no glyph was counted refused, so this test does not reach it (%v)", seen)
	}
	for _, v := range []paintVerdict{
		{painted: true},
		{painted: true, bounds: boundsBounded},
		{painted: true, bounds: boundsUnbounded},
	} {
		if seen[v] == 0 {
			t.Errorf("no glyph was counted %+v, so this test does not reach it (%v)", v, seen)
		}
	}
}

// TestAFaceIsPaintedFromSeveralGoroutinesAtOnce paints every glyph of
// ColourInk from goroutines sharing a face and its clones, none of them
// counted yet, and requires each to be painted as it is alone: what counting
// finds is kept behind the face's lock (run it with -race).
func TestAFaceIsPaintedFromSeveralGoroutinesAtOnce(t *testing.T) {
	data := harfbuzzFont(t, "ColourInk.ttf")
	alone, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, alone.NumGlyphs())
	for gid := range want {
		want[gid] = paintedText(alone, gid, PaintOptions{}, paintWork)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 8*len(want))
	for i := range 8 {
		face := f
		if i%2 == 1 {
			face = f.Clone()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gid := range want {
				if got := paintedText(face, gid, PaintOptions{}, paintWork); got != want[gid] {
					errs <- fmt.Sprintf("glyph %d: %s, and alone %s", gid, got, want[gid])
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
