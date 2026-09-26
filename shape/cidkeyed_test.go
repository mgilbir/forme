package shape

import (
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// IsCIDKeyed is the question CharacterCollection's ok folds into its false: is
// the face CID-keyed at all. A caller embedding a face needs it apart from the
// collection, because a face that is not CID-keyed is embedded by glyph index
// and one that is CID-keyed with an unusable ROS has to be refused.

// cidKeyedCases are the four kinds of face the answer has to tell apart. The
// CID-keyed ones give their two glyphs CIDs far from their indices, so a code
// that is a CID cannot be mistaken for one that is a glyph index.
func cidKeyedCases(t *testing.T) []struct {
	name     string
	face     *Face
	cidKeyed bool
	ok       bool
} {
	cids := []int{500, 700} // glyphs 1 and 2, 'a' and 'b'
	return []struct {
		name     string
		face     *Face
		cidKeyed bool
		ok       bool
	}{
		{"TrueType", latinFace(t), false, false},
		{"CFF that is not CID-keyed", ottoFace(t, fonttest.CFFOptions{}), false, false},
		{"CID-keyed with a usable ROS", ottoFace(t, fonttest.CFFOptions{
			CIDKeyed: true, Registry: "Acme", Ordering: "Japan9", Supplement: 3,
			CharsetSIDs: cids,
		}), true, true},
		{"CID-keyed naming no collection", ottoFace(t, fonttest.CFFOptions{
			CIDKeyed: true, UnnamedCollection: true, CharsetSIDs: cids,
		}), true, false},
		{"CID-keyed with a supplement below zero", ottoFace(t, fonttest.CFFOptions{
			CIDKeyed: true, NegativeSupplement: true, CharsetSIDs: cids,
		}), true, false},
	}
}

func TestIsCIDKeyedTellsTheKindOfFaceApartFromTheCollection(t *testing.T) {
	for _, c := range cidKeyedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			if got := c.face.IsCIDKeyed(); got != c.cidKeyed {
				t.Errorf("IsCIDKeyed is %v, want %v", got, c.cidKeyed)
			}
			// The two answers together are what an embedder decides on, and
			// the last two cases are the pair CharacterCollection alone cannot
			// separate from the first two.
			if _, _, _, ok := c.face.CharacterCollection(); ok != c.ok {
				t.Errorf("CharacterCollection's ok is %v, want %v", ok, c.ok)
			}
		})
	}
}

// TestIsCIDKeyedAgreesWithTheCodesEncodeWrites is the reason the answer comes
// from the face rather than from a second parse: true must be exactly the case
// in which the codes Encode wrote are CIDs.
//
// The CIDs are read here from the font package's own parse of the program, not
// from the face, so what is compared is Encode's output against an independent
// reading of the charset.
func TestIsCIDKeyedAgreesWithTheCodesEncodeWrites(t *testing.T) {
	for _, c := range cidKeyedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			f := c.face
			gs, _ := f.ShapeGlyphs("ab")
			codes, missing := f.Encode("ab")
			if missing != 0 || len(codes) != 2*len(gs) || len(gs) != 2 {
				t.Fatalf("\"ab\" set as %d glyphs and %d bytes of codes, %d missing",
					len(gs), len(codes), missing)
			}
			var cids []int
			if f.IsCFF() {
				cids = font.ParseCFF(font.SFNTTables(f.data)["CFF "]).GIDToCID
			}
			for i, g := range gs {
				code := int(codes[2*i])<<8 | int(codes[2*i+1])
				want := g.GID
				if cids != nil {
					want = cids[g.GID]
				}
				if code != want {
					t.Fatalf("glyph %d encoded as %d, and the program says %d", g.GID, code, want)
				}
				if isCID := code != g.GID; isCID != f.IsCIDKeyed() {
					t.Errorf("glyph %d encoded as %d, which is %s, and IsCIDKeyed "+
						"says %v", g.GID, code,
						map[bool]string{true: "a CID", false: "its glyph index"}[isCID],
						f.IsCIDKeyed())
				}
			}
		})
	}
}
