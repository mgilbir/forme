package shape

import (
	"os"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A face whose outlines are a CID-keyed CFF.
//
// Every static Noto CJK face is one, which is why this matters: the correctly
// weighted CJK faces exist, are freely licensed, and were refused outright until
// now. Reading one needs nothing special — a charstring is found by glyph index
// like any other, the cmap gives glyph indices and the advances come from hmtx —
// and the refusal was about embedding, which is where the two numberings part.
//
// A CIDFontType0 is addressed by CID. The code a format writes goes into the
// font's charset and comes out as a glyph index, so writing the glyph index
// there sends the reader to whatever glyph happens to carry that number. The
// two agree for a font whose charset is the identity, and many are — which is
// why a mistake here shows on some CJK faces and not others.

// cidKeyedFace is the fetched Noto Sans JP, or a skip. `make notocjk` puts it
// there. It is a real font rather than a synthetic one because what is under
// test is a structure fonttest does not build: a charset that is not the
// identity, which is the only thing that tells a CID from a glyph index.
func cidKeyedFace(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/notocjk/NotoSansJP-Regular.otf")
	if err != nil {
		t.Skip("run `make notocjk` for a CID-keyed face to read: ", err)
	}
	return data
}

func TestACIDKeyedFaceLoadsAndShapes(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("a CID-keyed face was refused: %v", err)
	}
	if !f.IsCFF() {
		t.Error("the face does not report CFF outlines")
	}
	// Three ideographs, each a glyph of its own and each a full em wide.
	gs, missing := f.ShapeGlyphs("日本語")
	if missing != 0 {
		t.Errorf("%d of three ideographs had no glyph", missing)
	}
	if len(gs) != 3 {
		t.Fatalf("three ideographs shaped to %d glyphs, want 3", len(gs))
	}
	var total float64
	for _, g := range gs {
		if g.GID == 0 {
			t.Error("an ideograph shaped to .notdef")
		}
		total += g.XAdvance
	}
	if total != 3000 {
		t.Errorf("three full-width ideographs advance %v units, want 3000 — a "+
			"CID-keyed face's advances come from hmtx like any other", total)
	}
}

// TestACIDKeyedFaceEncodesCIDsAndNotGlyphIndices is the distinction the whole
// format turns on.
//
// The codes must be the CIDs, and for this face they are nothing like the glyph
// indices — 日 is glyph 6369 and CID 20220. A face that wrote glyph indices here
// would produce a PDF that draws confidently wrong characters.
func TestACIDKeyedFaceEncodesCIDsAndNotGlyphIndices(t *testing.T) {
	data := cidKeyedFace(t)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const text = "日本語"

	gs, _ := f.ShapeGlyphs(text)
	codes, missing := f.Encode(text)
	if missing != 0 {
		t.Errorf("%d characters had no code", missing)
	}
	if len(codes) != 2*len(gs) {
		t.Fatalf("%d bytes of codes for %d glyphs, want two bytes each",
			len(codes), len(gs))
	}

	// The charset is the authority on which CID a glyph carries, and it is read
	// here from the font rather than from the face, so this compares two
	// answers rather than one with itself.
	cff := font.ParseCFF(font.SFNTTables(data)["CFF "])
	if cff == nil || cff.GIDToCID == nil {
		t.Fatal("the fixture is not a CID-keyed CFF, so this tests nothing")
	}
	same := true
	for i, g := range gs {
		got := int(codes[2*i])<<8 | int(codes[2*i+1])
		if want := cff.GIDToCID[g.GID]; got != want {
			t.Errorf("glyph %d encoded as %d, want CID %d", g.GID, got, want)
		}
		if got != g.GID {
			same = false
		}
	}
	if same {
		t.Error("every code equalled its glyph index, so this face cannot tell a " +
			"CID from a GID and the test proves nothing about which was written")
	}
}

// TestTheCIDsAreDistinct is what makes the code a reader writes recoverable.
//
// A CIDFontType0 is addressed by CID and the charset maps back to a glyph, so
// two glyphs sharing a CID would make that lookup ambiguous — one of them could
// never be drawn. It holds for this face and for every CID-keyed font worth
// embedding, and it is the property Encode's answer relies on.
func TestTheCIDsAreDistinct(t *testing.T) {
	cff := font.ParseCFF(font.SFNTTables(cidKeyedFace(t))["CFF "])
	if cff == nil || cff.GIDToCID == nil {
		t.Fatal("not a CID-keyed CFF")
	}
	seen := make(map[int]int, len(cff.GIDToCID))
	for gid, cid := range cff.GIDToCID {
		if prev, ok := seen[cid]; ok {
			t.Fatalf("CID %d is carried by both glyph %d and glyph %d, so the "+
				"charset cannot say which one a code means", cid, prev, gid)
		}
		seen[cid] = gid
	}
}

// TestACIDKeyedProgramIsEmbeddedWhole records what is not done yet, so that a
// change of mind is a change to this test rather than a surprise.
//
// Subsetting a CID-keyed program means rewriting the charset, the FDSelect and
// the FDArray together and renumbering the glyphs underneath all three. Until
// that is written the program is embedded as it stands, which costs size and
// nothing else — there is no renumbering to get wrong because nothing is
// renumbered.
func TestACIDKeyedProgramIsEmbeddedWhole(t *testing.T) {
	data := cidKeyedFace(t)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.Encode("日本語") // three glyphs used out of eighteen thousand

	prog, _, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("a CID-keyed face could not be embedded: %v", err)
	}
	if len(prog) == 0 {
		t.Fatal("the subsetter produced nothing")
	}
	// The CFF is carried across untouched, so the result is within a table or
	// two of the original rather than the few kilobytes a subset would be.
	if len(prog) < len(data)/2 {
		t.Errorf("the embedded program is %d bytes against the face's %d — that is "+
			"small enough to be a real subset, so this test is out of date and the "+
			"charset it describes is being rewritten after all", len(prog), len(data))
	}
}

// TestAnOrdinaryFaceStillEncodesGlyphIndices is the other side: nothing above
// may change what a face that is not CID-keyed writes.
func TestAnOrdinaryFaceStillEncodesGlyphIndices(t *testing.T) {
	f := latinFace(t)
	const text = "abc"
	gs, _ := f.ShapeGlyphs(text)
	codes, _ := f.Encode(text)
	if len(codes) != 2*len(gs) {
		t.Fatalf("%d bytes of codes for %d glyphs", len(codes), len(gs))
	}
	for i, g := range gs {
		if got := int(codes[2*i])<<8 | int(codes[2*i+1]); got != g.GID {
			t.Errorf("glyph %d encoded as %d; for a face that is not CID-keyed the "+
				"code is the glyph index", g.GID, got)
		}
	}
}
