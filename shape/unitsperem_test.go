package shape

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// An em outside 16..16384 is refused.
//
// OpenType allows no other, and the programs a page goes through disagree
// about what such a font is: HarfBuzz reads a thousand units (get_upem),
// FreeType refuses the face (sfobjs.c, "Invalid_Table"), and pdf.js takes the
// em as stated. A face loaded here would be measured on one em and read by a
// reader on another, or not at all, so it is refused where it is read, with
// the number in the error.

// withUnitsPerEm is data with head's unitsPerEm set to u, in a copy.
func withUnitsPerEm(t *testing.T, data []byte, u int) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	n := font.Be16(out, 4)
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if string(out[rec:rec+4]) == "head" {
			binary.BigEndian.PutUint16(out[font.Be32(out, rec+8)+18:], uint16(u))
			return out
		}
	}
	t.Fatal("the font has no head")
	return nil
}

// TestAnEmTheFormatDoesNotAllowIsRefused loads a face at each end of the range
// and one past each, as Load, LoadSimple and LoadInstance, for glyf and CFF2
// outlines alike.
func TestAnEmTheFormatDoesNotAllowIsRefused(t *testing.T) {
	latin := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	})
	variable := notoSansBytes(t)
	cff2, err := os.ReadFile("../testdata/harfbuzz/fonts/CFF2Blend.otf")
	if err != nil {
		t.Fatal(err)
	}
	loaders := []struct {
		name string
		data []byte
		load func([]byte) (*Face, error)
	}{
		{"Load", latin, Load},
		{"LoadSimple", variable, LoadSimple},
		{"LoadInstance of glyf outlines", variable, func(d []byte) (*Face, error) {
			return LoadInstance(d, map[string]float64{"wght": 700})
		}},
		{"LoadInstance of CFF2 outlines", cff2, func(d []byte) (*Face, error) {
			return LoadInstance(d, nil)
		}},
	}
	for _, l := range loaders {
		for _, u := range []int{0, 1, 15, 16, 16384, 16385, 65535} {
			t.Run(l.name+"/"+strconv.Itoa(u), func(t *testing.T) {
				f, err := l.load(withUnitsPerEm(t, l.data, u))
				if u >= 16 && u <= 16384 {
					if err != nil {
						t.Fatalf("an em of %d, which the format allows, was refused: %v", u, err)
					}
					if f.UnitsPerEm() != u {
						t.Errorf("the face says %d units to the em, and head %d", f.UnitsPerEm(), u)
					}
					return
				}
				if err == nil {
					t.Fatalf("an em of %d was read as %d units, and is outside 16..16384", u, f.UnitsPerEm())
				}
				if !strings.Contains(err.Error(), "states "+strconv.Itoa(u)+" units to the em") {
					t.Errorf("the refusal does not say which em: %v", err)
				}
			})
		}
	}
}
