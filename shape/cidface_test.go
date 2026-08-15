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

// TestACIDKeyedFaceSubsetsAndStillSaysTheSameThings is the round trip, which is
// the only way to know a rewritten font is right without a reader to open it.
//
// Subsetting a CID-keyed program touches three structures that name each other:
// the charset, which maps glyph to CID; the FDSelect, which maps glyph to Font
// DICT; and the FDArray, whose Font DICTs name their Private DICTs by absolute
// offset — and those offsets move when anything before them changes size. What
// makes it tractable is that the glyph numbering does not change: a dropped
// glyph becomes a bare endchar, so charset and FDSelect are copied through and
// only where things sit is rewritten.
//
// So the test is that nothing a caller can observe changed. The subset loads,
// shapes the same text to the same glyphs, encodes them to the same CIDs, and
// reports the same widths — and it is smaller, or the subsetting did nothing.
func TestACIDKeyedFaceSubsetsAndStillSaysTheSameThings(t *testing.T) {
	data := cidKeyedFace(t)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const text = "日本語"
	wantCodes, _ := f.Encode(text)
	wantGlyphs, _ := f.ShapeGlyphs(text)

	prog, _, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("a CID-keyed face could not be subsetted: %v", err)
	}
	if len(prog) >= len(data) {
		t.Errorf("the subset is %d bytes against the face's %d, so nothing was "+
			"dropped", len(prog), len(data))
	}

	g, err := Load(prog)
	if err != nil {
		t.Fatalf("the subsetted font does not load: %v", err)
	}
	gotCodes, missing := g.Encode(text)
	if missing != 0 {
		t.Errorf("%d characters had no code after subsetting", missing)
	}
	if string(gotCodes) != string(wantCodes) {
		t.Errorf("the subset encodes %v where the face encoded %v — the charset "+
			"no longer maps those glyphs to the CIDs the codes name",
			gotCodes, wantCodes)
	}
	gotGlyphs, _ := g.ShapeGlyphs(text)
	if len(gotGlyphs) != len(wantGlyphs) {
		t.Fatalf("the subset shapes %d glyphs where the face shaped %d",
			len(gotGlyphs), len(wantGlyphs))
	}
	for i := range wantGlyphs {
		if gotGlyphs[i].GID != wantGlyphs[i].GID {
			t.Errorf("glyph %d is %d after subsetting and was %d — the numbering "+
				"is meant to be retained", i, gotGlyphs[i].GID, wantGlyphs[i].GID)
		}
		if gotGlyphs[i].XAdvance != wantGlyphs[i].XAdvance {
			t.Errorf("glyph %d advances %v after subsetting and did %v",
				i, gotGlyphs[i].XAdvance, wantGlyphs[i].XAdvance)
		}
	}
}

// TestSubsettingKeepsEachGlyphWithItsOwnFontDICT is the half of the rewriting a
// round trip through this module cannot see.
//
// Every Font DICT names its Private DICT by an absolute offset, and every one of
// those moves when the charstrings shrink. Getting one wrong gives a range of
// glyphs the hinting and the width defaults of another — which changes nothing
// this module reads, because it takes its advances from hmtx, and shows up only
// in a renderer.
//
// So the assignment is compared directly rather than inferred from the widths it
// produces. Inferring was the first version of this test and it was too weak
// twice over: the glyphs it kept were all in Font DICTs whose defaults happen to
// agree, and the FDSelect's own tail is not reached by any glyph a short string
// keeps.
func TestSubsettingKeepsEachGlyphWithItsOwnFontDICT(t *testing.T) {
	data := cidKeyedFace(t)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Both scripts, so the glyphs kept come from more than one Font DICT: the
	// ideographs and the Latin are hinted separately in a face like this.
	f.Encode("日本語ABC")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}

	before := font.ParseCFF(font.SFNTTables(data)["CFF "])
	after := font.ParseCFF(font.SFNTTables(prog)["CFF "])
	if before == nil || after == nil {
		t.Fatal("a CFF did not parse")
	}
	if before.GIDToFD == nil || after.GIDToFD == nil {
		t.Fatal("a CFF came back without its Font DICT assignment")
	}

	// Every glyph, not only the kept ones. The numbering is retained, so the
	// FDSelect still covers the whole font — and its last range is reached by no
	// short string, which is exactly where a truncation would hide.
	if len(after.GIDToFD) != len(before.GIDToFD) {
		t.Fatalf("the subset assigns %d glyphs to Font DICTs and the face assigned %d",
			len(after.GIDToFD), len(before.GIDToFD))
	}
	differ := 0
	for gid := range before.GIDToFD {
		if before.GIDToFD[gid] != after.GIDToFD[gid] {
			if differ < 3 {
				t.Errorf("glyph %d is in Font DICT %d after subsetting and was in %d",
					gid, after.GIDToFD[gid], before.GIDToFD[gid])
			}
			differ++
		}
	}
	if differ > 0 {
		t.Errorf("%d glyphs of %d changed Font DICT", differ, len(before.GIDToFD))
	}

	// The FDSelect itself, byte for byte. Comparing the assignment it produces is
	// not enough: parseCFFFDs slices from the FDSelect's offset to the end of the
	// font, so a structure two bytes short reads its last range's bound out of
	// whatever happens to follow — which in a rewritten font is the Private DICTs,
	// and which for this face happens to give the right answer. A structure that
	// is right by luck is a structure that will stop being right.
	if a, b := fdSelectOf(t, data), fdSelectOf(t, prog); string(a) != string(b) {
		t.Errorf("the FDSelect is %d bytes after subsetting and was %d; it is copied "+
			"through unchanged, because the glyph numbering it names did not change",
			len(b), len(a))
	}

	// And the Font DICTs themselves still say what they said, which is what the
	// rewritten offsets decide.
	usedFDs := map[int]bool{}
	for _, gid := range kept {
		if gid < len(before.GIDToFD) {
			usedFDs[before.GIDToFD[gid]] = true
		}
	}
	if len(usedFDs) < 2 {
		t.Fatalf("the kept glyphs come from %d Font DICT(s); this test needs more "+
			"than one or it cannot tell them apart", len(usedFDs))
	}
	for _, gid := range kept {
		if gid >= len(after.WidthByGID) || gid >= len(before.WidthByGID) {
			t.Fatalf("kept glyph %d is outside the font", gid)
		}
		if before.WidthByGID[gid] != after.WidthByGID[gid] {
			t.Errorf("kept glyph %d takes width %v after subsetting and took %v — its "+
				"Font DICT's Private DICT is not where the Font DICT now says",
				gid, after.WidthByGID[gid], before.WidthByGID[gid])
		}
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

// fdSelectOf returns a font's FDSelect bytes, read the way the subsetter reads
// them so that the two agree about where the structure ends.
func fdSelectOf(t *testing.T, sfnt []byte) []byte {
	t.Helper()
	data := font.SFNTTables(sfnt)["CFF "]
	if data == nil {
		t.Fatal("no CFF table")
	}
	nameIndex, err := readCFFIndex(data, int(data[2]))
	if err != nil {
		t.Fatal(err)
	}
	topIndex, err := readCFFIndex(data, nameIndex.end)
	if err != nil {
		t.Fatal(err)
	}
	top, _, err := parseCFFDict(topIndex.items[0])
	if err != nil {
		t.Fatal(err)
	}
	var csOff, fdsOff int
	for _, e := range top {
		switch e.op {
		case opCharStrings:
			if len(e.operands) == 1 {
				csOff = e.operands[0]
			}
		case opFDSelect:
			if len(e.operands) == 1 {
				fdsOff = e.operands[0]
			}
		}
	}
	cs, err := readCFFIndex(data, csOff)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sliceFDSelect(data, fdsOff, len(cs.items))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestSliceFDSelectMeasuresTheStructure is measured against arithmetic written
// out here rather than against itself.
//
// The comparison in the subsetting test reads both fonts through this same
// function, so a function that measured the structure wrongly would measure it
// wrongly twice and the two would agree — which is what happened: a size that
// dropped the format 3 sentinel, and one a byte short in format 0, both survived
// a byte-for-byte comparison of two fonts.
//
// So the sizes are stated here from the specification: format 0 is the format
// byte and one byte per glyph; format 3 is the format byte, a two-byte range
// count, three bytes per range, and a two-byte sentinel. A marker is written
// immediately after each structure, and it must not come back.
func TestSliceFDSelectMeasuresTheStructure(t *testing.T) {
	const marker = 0xAB
	for _, c := range []struct {
		name   string
		body   []byte
		glyphs int
		want   int
	}{
		{
			name:   "format 0, five glyphs",
			body:   []byte{0, 0, 1, 1, 0, 2},
			glyphs: 5,
			want:   1 + 5,
		},
		{
			name: "format 3, two ranges",
			body: []byte{
				3,
				0, 2, // nRanges
				0, 0, 1,
				0, 3, 0,
				0, 6, // sentinel
			},
			glyphs: 6,
			want:   3 + 2*3 + 2,
		},
		{
			name:   "format 3, no ranges at all",
			body:   []byte{3, 0, 0, 0, 0},
			glyphs: 0,
			want:   3 + 0 + 2,
		},
	} {
		// A pad so the structure does not begin at zero, which the reader treats
		// as absent, and a marker after it that must stay outside.
		data := append(make([]byte, 8), c.body...)
		data = append(data, marker)

		got, err := sliceFDSelect(data, 8, c.glyphs)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got) != c.want {
			t.Errorf("%s: measured %d bytes, want %d", c.name, len(got), c.want)
			continue
		}
		for _, b := range got {
			if b == marker {
				t.Errorf("%s: the structure came back holding the byte after it",
					c.name)
				break
			}
		}
		if string(got) != string(c.body) {
			t.Errorf("%s: the bytes came back as %v, want %v", c.name, got, c.body)
		}
	}
}

func TestSliceFDSelectRefusesWhatItCannotRead(t *testing.T) {
	for _, c := range []struct {
		name        string
		data        []byte
		off, glyphs int
	}{
		{"an offset past the end", []byte{0, 0}, 9, 2},
		{"an offset of zero, which means absent", []byte{0, 0, 0}, 0, 1},
		{"a format this does not read", append(make([]byte, 4), 7), 4, 1},
		{"format 0 truncated", append(make([]byte, 4), 0, 1), 4, 8},
		{"format 3 truncated", append(make([]byte, 4), 3, 0, 9), 4, 4},
	} {
		if _, err := sliceFDSelect(c.data, c.off, c.glyphs); err == nil {
			t.Errorf("%s was accepted; a structure that cannot be measured cannot "+
				"be copied, and copying the wrong bytes writes a font that reads as "+
				"valid and draws the wrong character", c.name)
		}
	}
}
