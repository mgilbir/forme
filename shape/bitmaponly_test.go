package shape

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A font whose glyphs are only bitmaps, #887: Noto Color Emoji's CBDT build
// and most bitmap emoji fonts have CBDT or sbix and no outlines at all, and
// Load refused them for having neither glyf nor CFF.

// withoutOutlines is a face's font rebuilt without its glyf and loca: the
// same bitmaps, metrics and character map, and no outlines.
func withoutOutlines(t *testing.T, data []byte) []byte {
	t.Helper()
	tables := font.SFNTTables(data)
	if tables == nil {
		t.Fatal("not an sfnt")
	}
	delete(tables, "glyf")
	delete(tables, "loca")
	return assembleSFNT(tables)
}

// TestABitmapOnlyFaceIsTheBitmapsOfOneWithOutlines loads the bitmap fixtures
// with their outlines taken out, and requires that each glyph is the same
// bitmap the face with outlines paints, that a glyph with none paints nothing
// and has no outline to draw, and that the face says what it is and refuses to
// be embedded.
func TestABitmapOnlyFaceIsTheBitmapsOfOneWithOutlines(t *testing.T) {
	for _, name := range []string{"BitmapInk.ttf", "SbixInk.ttf"} {
		data := harfbuzzFont(t, name)
		with, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(withoutOutlines(t, data))
		if err != nil {
			t.Fatalf("%s without outlines: %v", name, err)
		}
		if with.BitmapOnly() || !f.BitmapOnly() {
			t.Fatalf("%s: BitmapOnly is %v with outlines and %v without", name, with.BitmapOnly(), f.BitmapOnly())
		}
		bitmaps := 0
		for gid := range f.NumGlyphs() {
			for _, ppem := range []int{0, 12, 20} {
				want, got := &recordingPainter{}, &recordingPainter{}
				if err := with.PaintGlyph(gid, PaintOptions{PPEM: ppem}, want); err != nil {
					t.Fatal(err)
				}
				if err := f.PaintGlyph(gid, PaintOptions{PPEM: ppem}, got); err != nil {
					t.Fatal(err)
				}
				kind := f.GlyphColour(gid, ppem)
				if kind != with.GlyphColour(gid, ppem) {
					t.Errorf("%s glyph %d at %d: %d without outlines and %d with", name, gid, ppem, kind, with.GlyphColour(gid, ppem))
				}
				switch {
				case kind == ColourBitmap:
					bitmaps++
					if len(got.lines) != 1 || got.lines[0] != want.lines[0] {
						t.Errorf("%s glyph %d at %d: %q, and with outlines %q", name, gid, ppem, got.lines, want.lines)
					}
				case len(got.lines) != 0:
					t.Errorf("%s glyph %d at %d, which has no bitmap, painted %q", name, gid, ppem, got.lines)
				}
			}
			var segs int
			err := f.GlyphOutline(gid, func(Segment) bool { segs++; return true })
			if segs != 0 || (err != nil) != (f.GlyphColour(gid, 0) == ColourBitmap) || err != nil && !errors.Is(err, ErrNoOutline) {
				t.Errorf("%s glyph %d: %d segments, %v", name, gid, segs, err)
			}
		}
		if bitmaps == 0 {
			t.Fatalf("%s: no glyph is a bitmap, so this test measures nothing", name)
		}
		if _, err := f.Subset(); err == nil {
			t.Errorf("%s: a face with no outlines was subsetted", name)
		}
		if _, err := LoadSimple(withoutOutlines(t, data)); err == nil {
			t.Errorf("%s: a face with no outlines was loaded to be embedded as a simple font", name)
		}
		useFace(f)
	}
}

// A font with neither outlines nor bitmaps is still refused.
func TestAFontWithNeitherOutlinesNorBitmapsIsRefused(t *testing.T) {
	tables := font.SFNTTables(withoutOutlines(t, harfbuzzFont(t, "BitmapInk.ttf")))
	delete(tables, "CBDT")
	delete(tables, "CBLC")
	if _, err := Load(assembleSFNT(tables)); err == nil {
		t.Fatal("a font with no glyphs to draw loaded")
	}
}

// TestNotoColorEmojiLoadsAndPaints is the font #887 was found with: Noto
// Color Emoji's CBDT build (EMOJI_FONTS), which has no outlines. Its emoji
// sequences shape to one glyph each, and each is painted as its bitmap.
func TestNotoColorEmojiLoadsAndPaints(t *testing.T) {
	dir := os.Getenv("EMOJI_FONTS")
	if dir == "" {
		t.Skip("EMOJI_FONTS is not set; run `make emoji-fonts`")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoColorEmoji.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if !f.BitmapOnly() {
		t.Error("Noto Color Emoji's CBDT build is not BitmapOnly")
	}
	counts := map[GlyphColour]int{}
	for gid := range f.NumGlyphs() {
		counts[f.GlyphColour(gid, 0)]++
	}
	if counts[ColourBitmap] < 4000 {
		t.Errorf("%d bitmap glyphs, where the font has over four thousand", counts[ColourBitmap])
	}
	for _, s := range []string{"👍🏽", "👩‍💻", "👨‍👩‍👧‍👦", "🏳️‍🌈", "🇳🇱", "1️⃣"} {
		glyphs, missing := f.ShapeGlyphs(s)
		if len(glyphs) != 1 || missing != 0 {
			t.Errorf("%q shapes to %d glyphs with %d missing", s, len(glyphs), missing)
			continue
		}
		r := &recordingPainter{}
		if err := f.PaintGlyph(glyphs[0].GID, PaintOptions{}, r); err != nil || len(r.lines) != 1 || r.lines[0][0] != 'I' {
			t.Errorf("%q is painted as %q, %v", s, r.lines, err)
		}
	}
}
