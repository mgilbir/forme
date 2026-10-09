package shape

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// The monochrome and greyscale strikes of a face with no outlines, #909: EBLC
// and EBDT, and Apple's bloc and bdat, painted as masks. Every glyph of every
// strike of Strikes.ttf and StrikesApple.ttf (testdata/freetype/fonts, built by
// strikes_fixture.py) is held to the bitmap FreeType loads for it, checked in
// as testdata/freetype/strikes.expected.txt; see strikes.py there.

// strikesFont is a face of testdata/freetype/fonts.
func strikesFont(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "freetype", "fonts", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// ftStrike is one strike of strikes.expected.txt: its ppem across and down,
// and each glyph's bitmap as FreeType loads it, nil for none.
type ftStrike struct {
	ppemX, ppemY int
	glyphs       map[int]*ftBitmap
}

// ftBitmap is a glyph's bitmap as FreeType loads it: its samples' depth, its
// size, where its top left corner is, and its samples, row by row.
type ftBitmap struct {
	depth, width, rows, left, top int
	samples                       []byte
}

// ftStrikeFace is one face of strikes.expected.txt.
type ftStrikeFace struct {
	name, sum string
	strikes   []*ftStrike
}

func readFreeTypeStrikes(t *testing.T) []*ftStrikeFace {
	t.Helper()
	return readFreeTypeStrikesFrom(t, filepath.Join("..", "testdata", "freetype", "strikes.expected.txt"))
}

// readFreeTypeStrikesFrom reads a file strikes.py wrote.
func readFreeTypeStrikesFrom(t *testing.T, path string) []*ftStrikeFace {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var faces []*ftStrikeFace
	atoi := func(s string) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("strikes.expected.txt: %v", err)
		}
		return v
	}
	sc := bufio.NewScanner(file)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		switch f[0] {
		case "face":
			faces = append(faces, &ftStrikeFace{name: f[1], sum: f[2]})
		case "S":
			face := faces[len(faces)-1]
			face.strikes = append(face.strikes, &ftStrike{ppemX: atoi(f[2]), ppemY: atoi(f[3]), glyphs: map[int]*ftBitmap{}})
		case "G":
			face := faces[len(faces)-1]
			s := face.strikes[len(face.strikes)-1]
			gid := atoi(f[1])
			if f[2] == "none" {
				s.glyphs[gid] = nil
				continue
			}
			b := &ftBitmap{depth: atoi(f[2]), width: atoi(f[3]), rows: atoi(f[4]), left: atoi(f[5]), top: atoi(f[6])}
			for _, row := range f[7:] {
				for _, v := range strings.Split(row, ",") {
					n, err := strconv.ParseUint(v, 16, 8)
					if err != nil {
						t.Fatalf("strikes.expected.txt: %v", err)
					}
					b.samples = append(b.samples, byte(n))
				}
			}
			if len(b.samples) != b.width*b.rows {
				t.Fatalf("strikes.expected.txt: glyph %d has %d samples for %d by %d", gid, len(b.samples), b.width, b.rows)
			}
			s.glyphs[gid] = b
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return faces
}

// ftComposite is a composite glyph's samples as FreeType's own bitmaps of its
// components make it, each placed where the composite puts it — the offsets
// read from the font, the pixels FreeType's — and nil for a glyph that is not
// a composite.
//
// FreeType's own composites are not held to: it places a component at a
// horizontal offset counted in bits rather than in pixels (ttsbit.c's
// load_byte_aligned and load_bit_aligned take x_pos >> 3 as the byte whatever
// the bit depth), so that in a greyscale strike a component placed one pixel
// right lands a fraction of a pixel right; and in a one-bit strike it drops
// bits of a bit-aligned component narrower than what is left of the byte it
// starts in. The format says the offsets are pixels, and the components are
// combined as FreeType combines them where it places them right: Strikes.ttf's
// one-bit composites of components that are byte-aligned or wider are
// FreeType's own, pixel for pixel.
func ftComposite(t *testing.T, f *Face, s *ftStrike, gid int) []byte {
	t.Helper()
	st, ok := f.strikes.strikeFor(s.ppemX)
	if !ok {
		t.Fatalf("no strike for %d", s.ppemX)
	}
	components := func(gid int) [][3]int {
		img, ok := f.strikes.locate(st, gid, font.NewBudget(strikeWork))
		if !ok || img.format != 8 && img.format != 9 {
			return nil
		}
		_, rest, _ := img.metrics()
		var cs [][3]int
		for i := range font.Be16(rest, 0) {
			c := rest[2+4*i:]
			cs = append(cs, [3]int{font.Be16(c, 0), int(int8(c[2])), int(int8(c[3]))})
		}
		return cs
	}
	if components(gid) == nil {
		return nil
	}
	out := s.glyphs[gid]
	samples := make([]byte, out.width*out.rows)
	var place func(gid, x, y int)
	place = func(gid, x, y int) {
		if cs := components(gid); cs != nil {
			for _, c := range cs {
				place(c[0], x+c[1], y+c[2])
			}
			return
		}
		b := s.glyphs[gid]
		for row := range b.rows {
			for col := range b.width {
				samples[(y+row)*out.width+x+col] |= b.samples[row*b.width+col]
			}
		}
	}
	place(gid, 0, 0)
	return samples
}

// imagePainter keeps the images a glyph is painted with, and counts the other
// calls, which a bitmap glyph should make none of.
type imagePainter struct {
	balancedPainter
	images []Image
	others int
}

func (p *imagePainter) Image(img Image)               { p.images = append(p.images, img) }
func (p *imagePainter) Solid(Color, bool)             { p.others++ }
func (p *imagePainter) PushClipGlyph(gid int)         { p.others++; p.balancedPainter.PushClipGlyph(gid) }
func (p *imagePainter) LinearGradient(LinearGradient) { p.others++ }

// TestEveryStrikeIsPaintedAsFreeTypeLoadsIt paints every glyph of every strike
// of the two faces at its strike's size, and requires each to be the mask
// FreeType loads — the same pixels, the same size, the same place — painted in
// the foreground, and Exact; and a glyph FreeType loads nothing of, or an
// empty bitmap, to be painted as nothing.
func TestEveryStrikeIsPaintedAsFreeTypeLoadsIt(t *testing.T) {
	faces := readFreeTypeStrikes(t)
	if len(faces) != 2 {
		t.Fatalf("%d faces in strikes.expected.txt, and strikes.py writes two", len(faces))
	}
	fg := Color{R: 10, G: 20, B: 30, A: 200}
	for _, want := range faces {
		data := strikesFont(t, want.name)
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != want.sum {
			t.Fatalf("%s is not the face strikes.expected.txt was written from; run `make ftstrikes`", want.name)
		}
		f, err := Load(data)
		if err != nil {
			t.Fatalf("%s: %v", want.name, err)
		}
		if !f.BitmapOnly() {
			t.Errorf("%s is not BitmapOnly", want.name)
		}
		upem := float64(f.UnitsPerEm())
		painted, depths := 0, map[int]bool{}
		for _, s := range want.strikes {
			for gid := range f.NumGlyphs() {
				b, listed := s.glyphs[gid]
				if !listed {
					t.Fatalf("%s at %d: glyph %d is not in strikes.expected.txt", want.name, s.ppemX, gid)
				}
				p := &imagePainter{}
				if err := f.PaintGlyph(gid, PaintOptions{PPEM: s.ppemX, Foreground: fg}, p); err != nil {
					t.Fatalf("%s at %d, glyph %d: %v", want.name, s.ppemX, gid, err)
				}
				kind := f.GlyphColour(gid, s.ppemX)
				if b == nil || b.width == 0 || b.rows == 0 {
					if len(p.images) != 0 || p.others != 0 || kind != ColourNone {
						t.Errorf("%s at %d, glyph %d: FreeType loads no bitmap, and it is painted as %d images and %d calls, %d",
							want.name, s.ppemX, gid, len(p.images), p.others, kind)
					}
					continue
				}
				if len(p.images) != 1 || p.others != 0 || kind != ColourMask {
					t.Errorf("%s at %d, glyph %d: %d images and %d other calls, %d", want.name, s.ppemX, gid, len(p.images), p.others, kind)
					continue
				}
				img := p.images[0]
				painted++
				depths[b.depth] = true
				if img.Format != ImageMask || img.Color != fg || !img.Exact {
					t.Errorf("%s at %d, glyph %d: format %d, colour %v, exact %v", want.name, s.ppemX, gid, img.Format, img.Color, img.Exact)
				}
				if img.Width != b.width || img.Height != b.rows {
					t.Errorf("%s at %d, glyph %d: %d by %d, and FreeType's %d by %d", want.name, s.ppemX, gid, img.Width, img.Height, b.width, b.rows)
					continue
				}
				// The box in pixels, which is where FreeType places the bitmap.
				px := func(v float64, ppem int) float64 { return v * float64(ppem) / upem }
				box := [4]float64{px(img.Box.XMin, s.ppemX), px(img.Box.YMax, s.ppemY), px(img.Box.XMax, s.ppemX), px(img.Box.YMin, s.ppemY)}
				wantBox := [4]float64{float64(b.left), float64(b.top), float64(b.left + b.width), float64(b.top - b.rows)}
				for i := range box {
					if math.Abs(box[i]-wantBox[i]) > 1e-9 {
						t.Errorf("%s at %d, glyph %d: the box is %v pixels, and FreeType places it at %v", want.name, s.ppemX, gid, box, wantBox)
						break
					}
				}
				scale := byte(255 / (1<<b.depth - 1))
				samples := b.samples
				if c := ftComposite(t, f, s, gid); c != nil {
					// Glyph 22 places a narrow bit-aligned component, glyph 9,
					// one pixel in; FreeType's own one-bit composites are held
					// to otherwise. See ftComposite.
					if b.depth == 1 && gid != 22 && string(c) != string(b.samples) {
						t.Errorf("%s at %d, glyph %d: FreeType's composite is not its components'", want.name, s.ppemX, gid)
					}
					samples = c
				}
				for i, v := range samples {
					if img.Data[i] != v*scale {
						t.Errorf("%s at %d, glyph %d: pixel %d is %d, and FreeType's sample %d of %d bits is %d",
							want.name, s.ppemX, gid, i, img.Data[i], v, b.depth, v*scale)
						break
					}
				}
			}
		}
		if painted < 80 || len(depths) != 4 {
			t.Errorf("%s: %d glyphs painted, at depths %v; the fixture holds more", want.name, painted, depths)
		}
	}
}

// TestAStrikeIsChosenForASizeAsACBDTOneIs paints at sizes between and past the
// strikes, and requires each glyph to be the image of the strike CBDT's choice
// picks — the smallest at least that large, or failing any the largest — not
// Exact, as one at the strike's own size is.
func TestAStrikeIsChosenForASizeAsACBDTOneIs(t *testing.T) {
	f, err := Load(strikesFont(t, "Strikes.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ asked, strike int }{
		{1, 12}, {11, 12}, {12, 12}, {13, 16}, {17, 20}, {24, 24}, {25, 32}, {32, 32}, {33, 32}, {200, 32}, {0, 32},
	} {
		for _, gid := range []int{1, 3, 5, 20} {
			got, want := &imagePainter{}, &imagePainter{}
			_ = f.PaintGlyph(gid, PaintOptions{PPEM: c.asked}, got)
			_ = f.PaintGlyph(gid, PaintOptions{PPEM: c.strike}, want)
			if len(got.images) != len(want.images) {
				t.Errorf("glyph %d at %d: %d images, and at %d, %d", gid, c.asked, len(got.images), c.strike, len(want.images))
				continue
			}
			if len(got.images) == 0 {
				if gid < 5 {
					t.Errorf("glyph %d at %d: nothing painted", gid, c.asked)
				}
				continue
			}
			g, w := got.images[0], want.images[0]
			if g.Width != w.Width || g.Height != w.Height || g.Box != w.Box || string(g.Data) != string(w.Data) {
				t.Errorf("glyph %d at %d is not its image in the strike of %d", gid, c.asked, c.strike)
			}
			if g.Exact != (c.asked == c.strike) || !w.Exact {
				t.Errorf("glyph %d at %d: exact %v, and at %d, %v", gid, c.asked, g.Exact, c.strike, w.Exact)
			}
		}
	}
}

// TestAColourBitmapSaysWhetherItsStrikeIsTheSizeAskedFor paints the CBDT and
// sbix fixtures at each strike's size and beside it: Image.Exact is set for
// the first and not for the second, nor for a glyph painted at no size.
func TestAColourBitmapSaysWhetherItsStrikeIsTheSizeAskedFor(t *testing.T) {
	for _, name := range []string{"BitmapInk.ttf", "SbixInk.ttf"} {
		f, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		var sizes []int
		if f.bitmap != nil {
			for i := range int(font.Be32(f.bitmap.cblc, 4)) {
				at := 8 + bitmapSizeTableSize*i
				if x, y := int(f.bitmap.cblc[at+44]), int(f.bitmap.cblc[at+45]); x == y {
					sizes = append(sizes, x)
				}
			}
		} else {
			for i := range int(font.Be32(f.sbix.table, 4)) {
				if off := int(font.Be32(f.sbix.table, 8+4*i)); off != 0 {
					sizes = append(sizes, font.Be16(f.sbix.table, off))
				}
			}
		}
		exact, scaled := 0, 0
		for _, ppem := range sizes {
			for gid := range f.NumGlyphs() {
				for _, asked := range []int{ppem, ppem + 1, 0} {
					p := &imagePainter{}
					if err := f.PaintGlyph(gid, PaintOptions{PPEM: asked}, p); err != nil || len(p.images) != 1 || p.images[0].Format != ImagePNG {
						continue
					}
					// The strike painted from at ppem+1 may be another one of
					// that size; only the size asked for and none are certain.
					switch asked {
					case ppem:
						if !p.images[0].Exact {
							t.Errorf("%s glyph %d at %d, a strike's size, is not Exact", name, gid, asked)
						}
						exact++
					case 0:
						if p.images[0].Exact {
							t.Errorf("%s glyph %d at no size is Exact", name, gid)
						}
						scaled++
					}
				}
			}
		}
		if exact == 0 || scaled == 0 {
			t.Errorf("%s: %d images exact and %d at no size, so this test measures nothing", name, exact, scaled)
		}
	}
}

// TestABitmapOnlyFaceIsMeasuredByItsStrikes requires a glyph's ink to be the
// box its metrics state in the largest strike, as a CBDT glyph's is measured,
// scaled to font units; a glyph that strike does not hold to have none; a
// glyph's outline to be refused as a bitmap's; and a run's ink to be measured
// from its glyphs'.
func TestABitmapOnlyFaceIsMeasuredByItsStrikes(t *testing.T) {
	f, err := Load(strikesFont(t, "Strikes.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for gid := 1; gid <= 4; gid++ {
		p := &imagePainter{}
		_ = f.PaintGlyph(gid, PaintOptions{PPEM: 32}, p)
		if len(p.images) != 1 {
			t.Fatalf("glyph %d is not painted at 32", gid)
		}
		box := p.images[0].Box
		x, y, w, h, ok := f.GlyphExtents(gid)
		round := func(v float64) int { return int(math.Floor(v + 0.5)) }
		if !ok || x != round(box.XMin) || y != round(box.YMax) || w != round(box.XMax-box.XMin) || h != round(box.YMin-box.YMax) {
			t.Errorf("glyph %d: extents %d %d %d %d %v, and its image's box is %v", gid, x, y, w, h, ok, box)
		}
		if err := f.GlyphOutline(gid, func(Segment) bool { return true }); !errors.Is(err, ErrNoOutline) {
			t.Errorf("glyph %d: GlyphOutline says %v, and it is a bitmap", gid, err)
		}
	}
	// Glyph 5 is in every strike but the largest.
	if _, _, _, _, ok := f.GlyphExtents(5); ok {
		t.Error("glyph 5 has extents, and the largest strike does not hold it")
	}
	if above, below, ok := f.InkExtent("AB", 32); !ok || above <= 0 || below < 0 {
		t.Errorf("the ink of AB is %v above and %v below, %v", above, below, ok)
	}
}

// TestAFontWithOutlinesIsPaintedFromThemWhateverStrikesItCarries gives a face
// with glyf outlines the fixture's strikes, as EBLC and EBDT and as bloc and
// bdat, as Courier New carries both, and requires it to be painted and
// measured from its outlines as it was without them.
func TestAFontWithOutlinesIsPaintedFromThemWhateverStrikesItCarries(t *testing.T) {
	strikes := font.SFNTTables(strikesFont(t, "Strikes.ttf"))
	gs := []fonttest.Glyph{{Rune: 'A', Advance: 600, HasShape: true}, {Rune: 'B', Advance: 600, HasShape: true}}
	plain, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tags := range [][2]string{{"EBLC", "EBDT"}, {"bloc", "bdat"}} {
		f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs, Extra: map[string][]byte{
			tags[0]: strikes["EBLC"], tags[1]: strikes["EBDT"],
		}}))
		if err != nil {
			t.Fatal(err)
		}
		if f.BitmapOnly() {
			t.Errorf("%s: a face with outlines is BitmapOnly", tags[0])
		}
		for gid := range f.NumGlyphs() {
			for _, ppem := range []int{0, 12, 16} {
				got, want := &recordingPainter{}, &recordingPainter{}
				_ = f.PaintGlyph(gid, PaintOptions{PPEM: ppem}, got)
				_ = plain.PaintGlyph(gid, PaintOptions{PPEM: ppem}, want)
				if strings.Join(got.lines, ";") != strings.Join(want.lines, ";") || f.GlyphColour(gid, ppem) != ColourNone {
					t.Errorf("%s glyph %d at %d: painted %q, and without strikes %q", tags[0], gid, ppem, got.lines, want.lines)
				}
			}
			gx, gy, gw, gh, gok := f.GlyphExtents(gid)
			wx, wy, ww, wh, wok := plain.GlyphExtents(gid)
			if [5]any{gx, gy, gw, gh, gok} != [5]any{wx, wy, ww, wh, wok} {
				t.Errorf("%s glyph %d: extents %d %d %d %d, and without strikes %d %d %d %d", tags[0], gid, gx, gy, gw, gh, wx, wy, ww, wh)
			}
		}
	}
}

// TestAFontWithOutlinesPaintsItsStrikesWhenAskedForItsBitmaps is
// PaintOptions.Bitmaps (issue 918): the same face, asked for its bitmaps,
// paints a glyph its strike has an image of as that image — the mask the
// strike's own bitmap-only face paints — and every other glyph as its outline,
// and is measured as before.
func TestAFontWithOutlinesPaintsItsStrikesWhenAskedForItsBitmaps(t *testing.T) {
	data := strikesFont(t, "Strikes.ttf")
	strikes := font.SFNTTables(data)
	only, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	gs := []fonttest.Glyph{{Rune: 'A', Advance: 600, HasShape: true}, {Rune: 'B', Advance: 600, HasShape: true}}
	plain, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tags := range [][2]string{{"EBLC", "EBDT"}, {"bloc", "bdat"}} {
		f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs, Extra: map[string][]byte{
			tags[0]: strikes["EBLC"], tags[1]: strikes["EBDT"],
		}}))
		if err != nil {
			t.Fatal(err)
		}
		masks := 0
		for gid := range f.NumGlyphs() {
			for _, ppem := range []int{0, 12, 16} {
				opts := PaintOptions{PPEM: ppem, Bitmaps: true}
				want, isMask := paintedMask(only, gid, ppem)
				got := &imagePainter{}
				_ = f.PaintGlyph(gid, PaintOptions{PPEM: ppem, Bitmaps: true, Foreground: Color{A: 1}}, got)
				colour := f.GlyphColourFor(gid, opts)
				if !isMask {
					// No image: the outline, as without the option.
					outline, plainOutline := &recordingPainter{}, &recordingPainter{}
					_ = f.PaintGlyph(gid, opts, outline)
					_ = plain.PaintGlyph(gid, PaintOptions{PPEM: ppem}, plainOutline)
					if strings.Join(outline.lines, ";") != strings.Join(plainOutline.lines, ";") || colour != ColourNone {
						t.Errorf("%s glyph %d at %d, which no strike has: painted %q, %v", tags[0], gid, ppem, outline.lines, colour)
					}
					continue
				}
				masks++
				if len(got.images) != 1 || got.others != 0 {
					t.Errorf("%s glyph %d at %d: %d images and %d other calls, want its mask alone", tags[0], gid, ppem, len(got.images), got.others)
					continue
				}
				img := got.images[0]
				if img.Format != ImageMask || !bytes.Equal(img.Data, want.Data) || img.Width != want.Width ||
					img.Height != want.Height || img.Exact != want.Exact || img.Color != (Color{A: 1}) {
					t.Errorf("%s glyph %d at %d: painted %+v, want the strike's %+v", tags[0], gid, ppem, img, want)
				}
				if colour != ColourMask {
					t.Errorf("%s glyph %d at %d: GlyphColourFor is %v, want ColourMask", tags[0], gid, ppem, colour)
				}
				// Without the option, the outline.
				if f.GlyphColour(gid, ppem) != ColourNone {
					t.Errorf("%s glyph %d at %d: painted from its strike without being asked", tags[0], gid, ppem)
				}
			}
		}
		if masks == 0 {
			t.Fatalf("%s: no glyph of the face has a strike image, so this tests nothing", tags[0])
		}
	}
}

// TestAnAppleBitmapFontIsMeasuredByItsBhed is StrikesApple.ttf, whose head is
// bhed, with the units per em bhed states changed to 2048: the face has them,
// and its masks are placed in them.
func TestAnAppleBitmapFontIsMeasuredByItsBhed(t *testing.T) {
	tables := font.SFNTTables(strikesFont(t, "StrikesApple.ttf"))
	if _, ok := tables["head"]; ok {
		t.Fatal("StrikesApple.ttf has a head")
	}
	bhed := append([]byte(nil), tables["bhed"]...)
	binary.BigEndian.PutUint16(bhed[18:], 2048)
	tables["bhed"] = bhed
	f, err := Load(assembleSFNT(tables))
	if err != nil {
		t.Fatal(err)
	}
	if f.UnitsPerEm() != 2048 || !f.BitmapOnly() {
		t.Errorf("%d units per em, BitmapOnly %v", f.UnitsPerEm(), f.BitmapOnly())
	}
	img, ok := paintedMask(f, 1, 16)
	if !ok || math.Abs((img.Box.XMax-img.Box.XMin)*16/2048-float64(img.Width)) > 1e-9 {
		t.Errorf("glyph 1 at 16 is %d pixels wide, in a box of %v units", img.Width, img.Box)
	}
}

// The tests below are of strikes built here a glyph at a time, each a
// format 1 index subtable of its own, in a face of Strikes.ttf's other tables.

// strikeGlyph is one glyph of a strike built here: its index, its image
// format and the image's bytes.
type strikeGlyph struct {
	gid, format int
	data        []byte
}

// buildStrike is EBLC and EBDT of one strike of a size and bit depth.
func buildStrike(ppem, depth int, glyphs []strikeGlyph) (eblc, ebdt []byte) {
	be16 := func(b []byte, v int) []byte { return binary.BigEndian.AppendUint16(b, uint16(v)) }
	be32 := func(b []byte, v int) []byte { return binary.BigEndian.AppendUint32(b, uint32(v)) }
	ebdt = be32(nil, 0x00020000)
	n := len(glyphs)
	array := 8 + bitmapSizeTableSize
	eblc = be32(be32(nil, 0x00020000), 1)
	eblc = be32(eblc, array)
	eblc = be32(eblc, n*(8+16))
	eblc = be32(eblc, n)
	eblc = be32(eblc, 0)
	eblc = append(eblc, make([]byte, 24)...)
	eblc = be16(be16(eblc, 0), 0xFFFF)
	eblc = append(eblc, byte(ppem), byte(ppem), byte(depth), 1)
	for i, g := range glyphs {
		eblc = be16(be16(eblc, g.gid), g.gid)
		eblc = be32(eblc, 8*n+16*i)
	}
	for _, g := range glyphs {
		eblc = be16(be16(eblc, 1), g.format)
		eblc = be32(eblc, len(ebdt))
		eblc = be32(be32(eblc, 0), len(g.data))
		ebdt = append(ebdt, g.data...)
	}
	return eblc, ebdt
}

// bigImage is a glyph's image in format 7 or 9: big metrics of a size and
// place, and then what follows them.
func bigImage(width, height, bearingX, bearingY int, rest ...byte) []byte {
	return append([]byte{byte(height), byte(width), byte(int8(bearingX)), byte(int8(bearingY)), byte(width), 0, 0, 0}, rest...)
}

// composite is a format 9 image's bytes past its metrics: components, each a
// glyph and its offset.
func composite(comps ...[3]int) []byte {
	b := binary.BigEndian.AppendUint16(nil, uint16(len(comps)))
	for _, c := range comps {
		b = binary.BigEndian.AppendUint16(b, uint16(c[0]))
		b = append(b, byte(int8(c[1])), byte(int8(c[2])))
	}
	return b
}

// strikeFace is Strikes.ttf with its strikes replaced.
func strikeFace(t *testing.T, eblc, ebdt []byte) *Face {
	t.Helper()
	f, err := loadStrikeFace(t, eblc, ebdt)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// loadStrikeFace is strikeFace, and Load's error for a face it refuses.
func loadStrikeFace(t *testing.T, eblc, ebdt []byte) (*Face, error) {
	t.Helper()
	tables := font.SFNTTables(strikesFont(t, "Strikes.ttf"))
	tables["EBLC"], tables["EBDT"] = eblc, ebdt
	return Load(assembleSFNT(tables))
}

// paintedMask is a glyph painted at a size, and whether it was painted as one
// mask.
func paintedMask(f *Face, gid, ppem int) (Image, bool) {
	p := &imagePainter{}
	if err := f.PaintGlyph(gid, PaintOptions{PPEM: ppem}, p); err != nil || len(p.images) != 1 {
		return Image{}, false
	}
	return p.images[0], true
}

// TestACompositeIsRefusedWhereFreeTypeRefusesIt builds composites FreeType
// refuses whole — a component that does not fit, one that is not in the
// strike, one that names the composite itself — and an image whose bytes are
// fewer than its bitmap, and requires each to paint nothing, and a composite
// beside them that is sound to paint its components.
func TestACompositeIsRefusedWhereFreeTypeRefusesIt(t *testing.T) {
	leaf := bigImage(8, 2, 0, 2, 0xFF, 0x0F) // one bit a pixel, bit-aligned
	eblc, ebdt := buildStrike(10, 1, []strikeGlyph{
		{1, 7, leaf},
		{2, 9, bigImage(9, 3, 0, 3, composite([3]int{1, 1, 1})...)},
		{3, 9, bigImage(8, 2, 0, 2, composite([3]int{1, 1, 0})...)},
		{4, 9, bigImage(8, 2, 0, 2, composite([3]int{9, 0, 0})...)},
		{5, 9, bigImage(8, 2, 0, 2, composite([3]int{5, 0, 0})...)},
		{6, 7, bigImage(8, 3, 0, 3, 0xFF, 0x0F)},
		{7, 9, bigImage(8, 2, 0, 2, composite([3]int{1, 0, -1})...)},
	})
	f := strikeFace(t, eblc, ebdt)
	img, ok := paintedMask(f, 2, 10)
	want := []byte{
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 255, 255, 255, 255, 255, 255, 255, 255,
		0, 0, 0, 0, 0, 255, 255, 255, 255,
	}
	if !ok || img.Width != 9 || img.Height != 3 || string(img.Data) != string(want) {
		t.Errorf("the sound composite is %v %d by %d, %v", ok, img.Width, img.Height, img.Data)
	}
	for gid, why := range map[int]string{
		3: "a component past the right edge",
		4: "a component the strike does not hold",
		5: "a composite of itself",
		6: "a bitmap of three rows in two rows' bytes",
		7: "a component above the top",
	} {
		if _, ok := paintedMask(f, gid, 10); ok {
			t.Errorf("glyph %d, %s, is painted", gid, why)
		}
	}
}

// TestACompositeThatWouldWriteBillionsOfPixelsIsRefused builds a composite
// of 255 composites of 255 bitmaps of 255 pixels square each: four billion
// pixels written, every one inside the box. It is refused, quickly, when its
// budget runs out.
func TestACompositeThatWouldWriteBillionsOfPixelsIsRefused(t *testing.T) {
	leaf := bigImage(255, 255, 0, 255, make([]byte, (255*255+7)/8)...)
	var many [][3]int
	for range 255 {
		many = append(many, [3]int{1, 0, 0})
	}
	var outer [][3]int
	for range 255 {
		outer = append(outer, [3]int{2, 0, 0})
	}
	eblc, ebdt := buildStrike(10, 1, []strikeGlyph{
		{1, 7, leaf},
		{2, 9, bigImage(255, 255, 0, 255, composite(many...)...)},
		{3, 9, bigImage(255, 255, 0, 255, composite(outer...)...)},
	})
	f := strikeFace(t, eblc, ebdt)
	if _, ok := paintedMask(f, 1, 10); !ok {
		t.Fatal("the bitmap the composites are made of is not painted")
	}
	start := time.Now()
	if _, ok := paintedMask(f, 3, 10); ok {
		t.Error("a composite of four billion pixels is painted")
	}
	if f.GlyphColour(3, 10) != ColourNone {
		t.Error("a composite of four billion pixels is said to be a mask")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("refusing it took %v", d)
	}
}

// TestGlyphColourSaysMaskWhereAndOnlyWhereOneIsPainted holds GlyphColour,
// which checks a glyph's bitmap without drawing it, to PaintGlyph, which draws
// it: over every glyph of both fixture faces, of the composites FreeType
// refuses, and of Strikes.ttf with its EBDT cut short, at sizes on, between
// and past the strikes, a glyph is ColourMask exactly where it is painted as
// one mask. Each refusal of draw's — bytes too few for the bitmap, a component
// past the box, one that cannot be read, one of itself — is among them.
func TestGlyphColourSaysMaskWhereAndOnlyWhereOneIsPainted(t *testing.T) {
	var faces []*Face
	for _, name := range []string{"Strikes.ttf", "StrikesApple.ttf"} {
		f, err := Load(strikesFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		faces = append(faces, f)
	}
	leaf := bigImage(8, 2, 0, 2, 0xFF, 0x0F)
	eblc, ebdt := buildStrike(10, 1, []strikeGlyph{
		{1, 7, leaf},
		{2, 9, bigImage(9, 3, 0, 3, composite([3]int{1, 1, 1})...)},
		{3, 9, bigImage(8, 2, 0, 2, composite([3]int{1, 1, 0})...)},
		{4, 9, bigImage(8, 2, 0, 2, composite([3]int{9, 0, 0})...)},
		{5, 9, bigImage(8, 2, 0, 2, composite([3]int{5, 0, 0})...)},
		{6, 7, bigImage(8, 3, 0, 3, 0xFF, 0x0F)},
		{7, 9, bigImage(8, 2, 0, 2, composite([3]int{1, 0, -1})...)},
		{8, 7, bigImage(0, 2, 0, 2)},
	})
	faces = append(faces, strikeFace(t, eblc, ebdt))
	tables := font.SFNTTables(strikesFont(t, "Strikes.ttf"))
	for n := 0; n < len(tables["EBDT"]); n += 5 {
		if f, err := loadStrikeFace(t, tables["EBLC"], tables["EBDT"][:n]); err == nil {
			faces = append(faces, f)
		}
	}
	masks, none := 0, 0
	for i, f := range faces {
		for gid := -1; gid <= f.NumGlyphs(); gid++ {
			for _, ppem := range []int{0, 8, 10, 12, 14, 16, 24, 40} {
				_, painted := paintedMask(f, gid, ppem)
				said := f.GlyphColour(gid, ppem) == ColourMask
				if said != painted {
					t.Errorf("face %d glyph %d at %d: GlyphColour says a mask %v, and one is painted %v", i, gid, ppem, said, painted)
				}
				if painted {
					masks++
				} else {
					none++
				}
			}
		}
	}
	if masks == 0 || none == 0 {
		t.Fatalf("%d masks and %d glyphs with none: the faces do not divide, so this tests nothing", masks, none)
	}
}

// TestATruncatedStrikeTableIsReadWithoutPanicking cuts Strikes.ttf's EBLC and
// EBDT at every length, and paints and measures every glyph at every strike's
// size from each: whatever is read, nothing panics and nothing unbalanced is
// handed to a painter.
func TestATruncatedStrikeTableIsReadWithoutPanicking(t *testing.T) {
	tables := font.SFNTTables(strikesFont(t, "Strikes.ttf"))
	eblc, ebdt := tables["EBLC"], tables["EBDT"]
	try := func(l, d []byte) {
		f, err := loadStrikeFace(t, l, d)
		if err != nil {
			// Cut to nothing, the face has neither outlines nor bitmaps.
			if len(l) != 0 && len(d) != 0 {
				t.Fatalf("cut to %d and %d bytes: %v", len(l), len(d), err)
			}
			return
		}
		for gid := -1; gid <= f.NumGlyphs(); gid++ {
			for _, ppem := range []int{0, 12, 14, 24, 40} {
				paintBalanced(f, gid, PaintOptions{PPEM: ppem})
				_ = f.GlyphColour(gid, ppem)
			}
			_, _, _, _, _ = f.GlyphExtents(gid)
			_ = f.GlyphOutline(gid, func(Segment) bool { return true })
		}
	}
	step := 1
	if testing.Short() {
		step = 7
	}
	for n := 0; n < len(eblc); n += step {
		try(eblc[:n], ebdt)
	}
	for n := 0; n < len(ebdt); n += step {
		try(eblc, ebdt[:n])
	}
}

// TestAStrikeTableStatingMoreThanItHoldsIsRefused states counts no table of
// its size could hold — strikes, ranges, glyphs of a sparse subtable — and
// requires each to be read as far as it goes and no further.
func TestAStrikeTableStatingMoreThanItHoldsIsRefused(t *testing.T) {
	tables := font.SFNTTables(strikesFont(t, "Strikes.ttf"))
	eblc, ebdt := tables["EBLC"], tables["EBDT"]
	set32 := func(at int, v uint32) []byte {
		b := append([]byte(nil), eblc...)
		binary.BigEndian.PutUint32(b[at:], v)
		return b
	}
	// The strikes' count.
	if f := strikeFace(t, set32(4, 0xFFFFFFFF), ebdt); f.strikes != nil {
		t.Error("a strike count past the table is read")
	}
	// The first strike's count of ranges: read until the table ends.
	f := strikeFace(t, set32(8+8, 0xFFFFFFFF), ebdt)
	if _, ok := paintedMask(f, 1, 12); !ok {
		t.Error("glyph 1, in the first range, is not painted when the count of ranges is past the table")
	}
	_, _ = paintedMask(f, 0, 12)
	// A sparse subtable's count of glyphs: the first strike's sixth range is
	// format 4.
	st := strike{at: 8}
	array := int(font.Be32(eblc, 8))
	sub := array + int(font.Be32(eblc, array+8*5+4))
	if font.Be16(eblc, sub) != 4 {
		t.Fatalf("the sixth range is format %d", font.Be16(eblc, sub))
	}
	f = strikeFace(t, set32(sub+8, 0xFFFFFFFF), ebdt)
	if _, ok := f.strikes.locate(st, 11, font.NewBudget(strikeWork)); ok {
		t.Error("a sparse subtable stating four billion glyphs is read")
	}
}

// TestSystemBitmapFonts loads the fonts of macOS that carry strikes
// (SYSTEM_FONTS, a directory holding /System/Library/Fonts/Supplemental's
// fonts): NISC18030, whose glyphs are only bdat strikes under a bhed, is
// loaded and painted from them, and Courier New, which has outlines and
// strikes in EBDT and bdat both, is painted from its outlines.
func TestSystemBitmapFonts(t *testing.T) {
	dir := os.Getenv("SYSTEM_FONTS")
	if dir == "" {
		t.Skip("SYSTEM_FONTS is not set; set it to /System/Library/Fonts/Supplemental on macOS")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NISC18030.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("NISC18030: %v", err)
	}
	if !f.BitmapOnly() || f.strikes == nil {
		t.Fatal("NISC18030 is not read as a face of strikes")
	}
	glyphs, missing := f.ShapeGlyphs("中文字体")
	if missing != 0 || len(glyphs) != 4 {
		t.Fatalf("中文字体 shapes to %d glyphs, %d missing", len(glyphs), missing)
	}
	n := int(font.Be32(f.strikes.loc, 4))
	for i := range n {
		at := 8 + bitmapSizeTableSize*i
		ppem := int(f.strikes.loc[at+44])
		for _, g := range glyphs {
			img, ok := paintedMask(f, g.GID, ppem)
			if !ok || img.Format != ImageMask || !img.Exact || img.Width == 0 {
				t.Errorf("NISC18030 glyph %d at %d is not painted as a mask of its strike", g.GID, ppem)
				continue
			}
			inked := 0
			for _, v := range img.Data {
				if v != 0 {
					inked++
				}
			}
			if inked == 0 {
				t.Errorf("NISC18030 glyph %d at %d has no ink", g.GID, ppem)
			}
		}
	}
	data, err = os.ReadFile(filepath.Join(dir, "Courier New.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	if f, err = Load(data); err != nil {
		t.Fatal(err)
	}
	gid, _ := f.GlyphID('A')
	if f.BitmapOnly() || f.GlyphColour(gid, 12) != ColourNone {
		t.Error("Courier New is not painted from its outlines")
	}
	// Asked for its bitmaps, at the size of each of its strikes, a glyph a
	// strike has is painted from it, and one none has from its outline. Its
	// strikes hold three glyphs, 371 to 373, and not the letters.
	if f.strikes == nil {
		t.Fatal("Courier New's strikes are not read")
	}
	for i := range int(font.Be32(f.strikes.loc, 4)) {
		ppem := int(f.strikes.loc[8+bitmapSizeTableSize*i+44])
		opts := PaintOptions{PPEM: ppem, Bitmaps: true}
		p := &imagePainter{}
		_ = f.PaintGlyph(371, opts, p)
		if len(p.images) != 1 || p.images[0].Format != ImageMask || !p.images[0].Exact ||
			f.GlyphColourFor(371, opts) != ColourMask {
			t.Errorf("Courier New's glyph 371 at %d, asked for its bitmaps, is not painted from its strike", ppem)
		}
		if f.GlyphColourFor(gid, opts) != ColourNone {
			t.Errorf("Courier New's A at %d, which no strike has, is not painted from its outline", ppem)
		}
	}
}
