package shape

import (
	"bytes"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A face's strikes listed, and a glyph's image in one of them (issue 915).

// strikeFaces are the fixtures with strikes of each kind: CBDT, sbix, EBDT,
// and bdat.
func strikeFaces(t *testing.T) map[string]*Face {
	t.Helper()
	out := map[string]*Face{}
	for _, name := range []string{"BitmapInk.ttf", "SbixInk.ttf"} {
		f, err := Load(harfbuzzFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = f
	}
	for _, name := range []string{"Strikes.ttf", "StrikesApple.ttf"} {
		f, err := Load(strikesFont(t, name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = f
	}
	return out
}

// TestAFaceListsItsStrikes: every strike of the face's tables, of the table
// it is in and the size it states, smallest first.
func TestAFaceListsItsStrikes(t *testing.T) {
	tables := map[string]string{
		"BitmapInk.ttf": "CBDT", "SbixInk.ttf": "sbix", "Strikes.ttf": "EBDT", "StrikesApple.ttf": "bdat",
	}
	for name, f := range strikeFaces(t) {
		strikes := f.Strikes()
		if len(strikes) == 0 {
			t.Fatalf("%s lists no strikes", name)
		}
		// What the table itself states, counted from its header.
		want := 0
		switch tables[name] {
		case "CBDT":
			want = int(font.Be32(f.bitmap.cblc, 4))
		case "sbix":
			for i := range int(font.Be32(f.sbix.table, 4)) {
				if at := int(font.Be32(f.sbix.table, 8+4*i)); at != 0 && font.Be16(f.sbix.table, at) > 0 {
					want++
				}
			}
		default:
			want = int(font.Be32(f.strikes.loc, 4))
		}
		if len(strikes) != want {
			t.Errorf("%s lists %d strikes, and its table states %d", name, len(strikes), want)
		}
		for i, s := range strikes {
			if s.Table != tables[name] || s.PPEMX <= 0 || s.PPEMY <= 0 {
				t.Errorf("%s strike %d is %+v", name, i, s)
			}
			if i > 0 && s.PPEM() < strikes[i-1].PPEM() {
				t.Errorf("%s lists a strike of %d after one of %d", name, s.PPEM(), strikes[i-1].PPEM())
			}
		}
	}
}

// TestAGlyphsImageInAStrikeIsWhatPaintingItThereDraws: StrikeImage in each
// strike is the image PaintGlyph paints at that strike's own size — which
// chooses it — and painting says it is from that strike.
func TestAGlyphsImageInAStrikeIsWhatPaintingItThereDraws(t *testing.T) {
	for name, f := range strikeFaces(t) {
		images := 0
		for _, s := range f.Strikes() {
			for gid := range f.NumGlyphs() {
				got, ok := f.StrikeImage(gid, s)
				p := &imagePainter{}
				_ = f.PaintGlyph(gid, PaintOptions{PPEM: s.PPEM()}, p)
				painted := len(p.images) == 1 && p.images[0].Strike == s
				if !ok {
					if painted {
						t.Errorf("%s glyph %d: painted from %+v, which StrikeImage says has no image of it", name, gid, s)
					}
					continue
				}
				images++
				if got.Exact != (s.PPEMX == s.PPEMY) || got.Strike != s {
					t.Errorf("%s glyph %d in %+v: Exact %v, Strike %+v", name, gid, s, got.Exact, got.Strike)
				}
				if !painted {
					// Another strike of the same size may be the one chosen.
					continue
				}
				want := p.images[0]
				if got.Format != want.Format || !bytes.Equal(got.Data, want.Data) || got.Width != want.Width ||
					got.Height != want.Height || got.Box != want.Box || got.Exact != want.Exact {
					t.Errorf("%s glyph %d in %+v:\n%+v\npainted\n%+v", name, gid, s, got, want)
				}
			}
		}
		if images == 0 {
			t.Errorf("%s: no glyph has an image in any strike, so this tests nothing", name)
		}
	}
}

// TestAGlyphMissingFromTheStrikeASizeChoosesIsFoundInAnother is the case the
// listing is for: Strikes.ttf's largest strike, 32 ppem, leaves some glyphs
// out, so painting at 32 paints them as nothing, and a smaller strike holds
// them.
func TestAGlyphMissingFromTheStrikeASizeChoosesIsFoundInAnother(t *testing.T) {
	f := strikeFaces(t)["Strikes.ttf"]
	strikes := f.Strikes()
	largest := strikes[len(strikes)-1]
	found := 0
	for gid := 1; gid < f.NumGlyphs(); gid++ {
		if _, ok := f.StrikeImage(gid, largest); ok {
			continue
		}
		p := &imagePainter{}
		_ = f.PaintGlyph(gid, PaintOptions{PPEM: largest.PPEM()}, p)
		if len(p.images) != 0 {
			t.Errorf("glyph %d, which the largest strike does not hold, was painted at its size", gid)
		}
		for i := len(strikes) - 2; i >= 0; i-- {
			if _, ok := f.StrikeImage(gid, strikes[i]); ok {
				found++
				break
			}
		}
	}
	if found == 0 {
		t.Fatal("no glyph missing from the largest strike is in another, so this tests nothing")
	}
}

// TestAStrikeImageIsReadOnlyInTheFacesOwnStrikes: a strike from another face,
// or one made up, is not read, though its offset may point anywhere.
func TestAStrikeImageIsReadOnlyInTheFacesOwnStrikes(t *testing.T) {
	faces := strikeFaces(t)
	f, other := faces["Strikes.ttf"], faces["BitmapInk.ttf"]
	for _, s := range append(other.Strikes(),
		Strike{Table: "EBDT", PPEMX: 12, PPEMY: 12, at: 1 << 30},
		Strike{Table: "EBDT", PPEMX: 12, PPEMY: 12, at: -5},
		Strike{}) {
		for _, gid := range []int{-1, 0, 1, f.NumGlyphs()} {
			if _, ok := f.StrikeImage(gid, s); ok {
				t.Errorf("glyph %d in %+v, which is not one of the face's strikes, has an image", gid, s)
			}
		}
	}
	if _, ok := f.StrikeImage(f.NumGlyphs(), f.Strikes()[0]); ok {
		t.Error("a glyph the face does not have has an image")
	}
}
