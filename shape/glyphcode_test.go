package shape

import (
	"testing"

	"github.com/mgilbir/forme/font"
)

// GlyphCode is the number a format writes wherever it names a glyph: the key of
// a width table, the bit set in a /CIDSet, the code in a content stream.
//
// For most faces it is the glyph index, and for a CID-keyed CFF it is the CID.
// The two agree for a font whose charset is the identity — in Noto Sans JP,
// 'A' is glyph 34 and CID 34 — and differ for 17,564 of its 17,936 glyphs,
// among them 日, which is glyph 6369 and CID 20220. That is why a caller that
// uses the glyph index produces a document which is right for Latin and wrong
// for the CJK it went to the trouble of embedding.

// cidsOf reads the charset directly, as the oracle for what a code should be.
// It is the font package rather than this one: comparing GlyphCode against
// Encode would compare codeForGID with itself, and both would be wrong together.
func cidsOf(t *testing.T, data []byte) []int {
	t.Helper()
	p := font.ParseCFF(font.SFNTTables(data)["CFF "])
	if p == nil {
		t.Fatal("the CFF did not parse, so there is no oracle")
	}
	if p.GIDToCID == nil {
		t.Fatal("the face is not CID-keyed, so it cannot tell a code from an index")
	}
	return p.GIDToCID
}

func TestGlyphCodeIsTheCIDForACIDKeyedFace(t *testing.T) {
	data := cidKeyedFace(t)
	want := cidsOf(t, data)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	differ := 0
	for gid, cid := range want {
		if got := f.GlyphCode(gid); got != cid {
			t.Fatalf("glyph %d has code %d, want CID %d", gid, got, cid)
		}
		if gid != cid {
			differ++
		}
	}
	// Without this the test would pass against a GlyphCode that returned the
	// glyph index, on a font whose charset happened to be the identity.
	if differ == 0 {
		t.Fatal("no glyph in this face has a CID differing from its index, so " +
			"the fixture cannot tell the two numberings apart")
	}
	t.Logf("%d of %d glyphs have a CID differing from their index", differ, len(want))
}

// TestGlyphCodeAgreesWithTheCodesEncodeWrites is the property that makes a
// width table usable: /W is keyed on GlyphCode and the content stream carries
// what Encode wrote, and a document is only coherent if they are the same
// number. They come from one place today; this is here so that they still
// agree if they ever do not.
func TestGlyphCodeAgreesWithTheCodesEncodeWrites(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const text = "日本語とLatin"
	glyphs, _ := f.ShapeGlyphs(text)
	codes, missing := f.Encode(text)
	if missing != 0 {
		t.Fatalf("%d characters had no code", missing)
	}
	if len(codes) != 2*len(glyphs) {
		t.Fatalf("%d bytes of codes for %d glyphs", len(codes), len(glyphs))
	}
	for i, g := range glyphs {
		wrote := int(codes[2*i])<<8 | int(codes[2*i+1])
		if got := f.GlyphCode(g.GID); got != wrote {
			t.Errorf("glyph %d: Encode wrote %d and GlyphCode says %d — a width "+
				"table keyed on one describes a glyph the stream does not name",
				g.GID, wrote, got)
		}
	}
}

// TestEveryGlyphASubsetKeepsHasADistinctCode: /W states a width per code and
// /CIDSet sets a bit per code, so two kept glyphs sharing one would silently
// describe only one of them.
func TestEveryGlyphASubsetKeepsHasADistinctCode(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.Encode("日本語とLatin too")
	_, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	if len(kept) < 2 {
		t.Fatalf("the subset kept %d glyphs, which cannot collide", len(kept))
	}
	seen := make(map[int]int, len(kept))
	for _, gid := range kept {
		code := f.GlyphCode(gid)
		if prev, ok := seen[code]; ok {
			t.Errorf("glyphs %d and %d both have code %d", prev, gid, code)
		}
		seen[code] = gid
	}
}

// TestGlyphCodeIsTheGlyphIndexForAnOrdinaryFace: for every face that is not a
// CID-keyed CFF the glyph index is the only numbering there is, which is what
// Identity-H means.
func TestGlyphCodeIsTheGlyphIndexForAnOrdinaryFace(t *testing.T) {
	f := latinFace(t)
	n := f.NumGlyphs()
	if n < 2 {
		t.Fatalf("the face has %d glyphs", n)
	}
	for gid := 0; gid < n; gid++ {
		if got := f.GlyphCode(gid); got != gid {
			t.Fatalf("glyph %d has code %d; for a face that is not CID-keyed the "+
				"code is the glyph index", gid, got)
		}
	}
}

// TestGlyphCodeLeavesAGlyphTheFaceHasNotGotAlone: a glyph id out of range has
// no code, and inventing one would hide the caller's mistake rather than the
// font's. It comes back unchanged, which is what the documented contract says.
func TestGlyphCodeLeavesAGlyphTheFaceHasNotGotAlone(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, gid := range []int{-1, f.NumGlyphs(), f.NumGlyphs() + 1000} {
		if got := f.GlyphCode(gid); got != gid {
			t.Errorf("glyph %d, which the face has not got, has code %d", gid, got)
		}
	}
}
