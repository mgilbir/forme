package shape

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// TestALocaOffsetOf2To31IsRefused is #989 as it was reported: a long loca
// entry of 0x80000000, which where int is 32 bits read as a negative offset,
// passed the check that it lay inside glyf, and panicked slicing it. It is
// refused on every platform, as it always was on a 64-bit one.
func TestALocaOffsetOf2To31IsRefused(t *testing.T) {
	src := fonttest.NotoFile(t, "NotoSans-Regular.ttf")
	face, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := face.CmapGlyph('A')
	if !ok {
		t.Fatal("no A")
	}
	b := slices.Clone(src)
	tables := font.SFNTTables(b)
	if binary.BigEndian.Uint16(tables["head"][50:]) != 1 {
		t.Fatal("the face's loca is short, and this is about a long one")
	}
	for _, v := range []uint32{0x80000000, 0xFFFFFFFC} {
		binary.BigEndian.PutUint32(tables["loca"][4*gid:], v)
		f, err := Load(b)
		if err != nil {
			continue // refused at Load: as good as at the subset
		}
		f.Use(gid)
		noPanic(t, fmt.Sprintf("SubsetGlyphs with a loca entry of %#x", v), func() {
			if _, _, err := f.SubsetGlyphs(); err == nil {
				t.Errorf("a glyph at loca offset %#x was subset", v)
			}
		})
		noPanic(t, "GlyphExtents", func() { _, _, _, _, _ = f.GlyphExtents(gid) })
		noPanic(t, "GlyphOutline", func() { _ = f.GlyphOutline(gid, func(Segment) bool { return true }) })
	}
}

// TestHighOffsetsAreRefusedNotPanicked: every 32-bit field near the start of
// every table of a few real faces — where the offsets a reader follows first
// are — set to 2^31 and to just short of 2^32, and the face loaded and used as
// the fuzz target uses one. On a 64-bit platform it is a test that nothing
// panics; where int is 32 bits (make test-386) it is the one that found the
// offsets read as negative numbers.
func TestHighOffsetsAreRefusedNotPanicked(t *testing.T) {
	if testing.Short() {
		t.Skip("loads each face once for each field it changes")
	}
	faces := map[string]func(*testing.T) []byte{
		"NotoSans-Regular (glyf, GSUB, GPOS)": func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSans-Regular.ttf") },
		"NotoSansJP-VF (gvar, HVAR)":          func(t *testing.T) []byte { return fonttest.NotoFile(t, "NotoSansJP-VF.ttf") },
		"SourceSans3 (CFF)":                   func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") },
		"Noto-COLRv1 (COLR, CPAL)": func(t *testing.T) []byte {
			dir := os.Getenv("EMOJI_FONTS")
			if dir == "" {
				t.Skip("EMOJI_FONTS is not set")
			}
			data, err := os.ReadFile(filepath.Join(dir, "Noto-COLRv1.ttf"))
			if err != nil {
				t.Fatal(err)
			}
			return data
		},
	}
	const reach = 48 // bytes into each table, at every even offset
	for name, data := range faces {
		t.Run(name, func(t *testing.T) {
			src := data(t)
			tags := make([]string, 0)
			for tag := range font.SFNTTables(src) {
				tags = append(tags, tag)
			}
			slices.Sort(tags)
			for _, tag := range tags {
				if tag == "head" || tag == "maxp" {
					continue // no offsets: counts and checksums the font is refused by
				}
				for at := 0; at+4 <= reach; at += 2 {
					for _, v := range []uint32{0x80000000, 0xFFFFFFFC} {
						b := slices.Clone(src)
						tb := font.SFNTTables(b)[tag]
						if at+4 > len(tb) {
							continue
						}
						binary.BigEndian.PutUint32(tb[at:], v)
						what := fmt.Sprintf("%s at %d set to %#x", tag, at, v)
						noPanic(t, what, func() {
							f, err := Load(b)
							if err != nil {
								return
							}
							useFace(f)
							f.Encode("Measurement 0123456789 日本語 🙂")
							_, _, _ = f.SubsetGlyphs()
						})
					}
				}
			}
		})
	}
}
