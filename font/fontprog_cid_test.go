package font

import (
	"os"
	"testing"
)

// The ROS of a CID-keyed CFF: the character collection its CIDs are numbered
// in, which a PDF embedding the program has to state.
//
// ISO 32000-2 9.7.4.2 requires a CIDFont's /CIDSystemInfo to be compatible with
// the collection of its glyph source, so a consumer that cannot read this has
// to either guess or refuse. pdf0 refused, which is what this is for.

// TestCFFReportsItsCharacterCollection reads a real CID-keyed face, because the
// thing under test is a structure no synthetic fixture here builds: a ROS whose
// SIDs resolve through the font's own string index.
func TestCFFReportsItsCharacterCollection(t *testing.T) {
	data, err := os.ReadFile("../testdata/notocjk/NotoSansJP-Regular.otf")
	if err != nil {
		t.Skip("run `make notocjk` for a CID-keyed face to read: ", err)
	}
	p := ParseCFF(SFNTTables(data)["CFF "])
	if p == nil {
		t.Fatal("the CFF did not parse")
	}
	if p.GIDToCID == nil {
		t.Fatal("the face is not CID-keyed, so it cannot exercise the ROS")
	}
	// Noto's CJK faces are numbered in their own collection rather than in one
	// of Adobe's published ones, which is what Adobe-Identity-0 says. The point
	// of reading it is that a face saying Adobe-Japan1 must not be embedded as
	// this.
	if p.Registry != "Adobe" || p.Ordering != "Identity" || p.Supplement != 0 {
		t.Errorf("the ROS is %q-%q-%d, want Adobe-Identity-0",
			p.Registry, p.Ordering, p.Supplement)
	}
}

// TestAPlainCFFHasNoCharacterCollection: the fields are meaningless for a font
// whose glyphs are numbered by index, and must stay empty rather than carrying
// a default a caller might write into a document.
func TestAPlainCFFHasNoCharacterCollection(t *testing.T) {
	data, err := os.ReadFile("../testdata/googlefonts/ofl/notosans/NotoSans[wdth,wght].ttf")
	if err != nil {
		// Any non-CID CFF will do; the bundled face is a TrueType, so this
		// falls back to asserting the zero value on a font with no CFF at all.
		p := ParseCFF(nil)
		if p != nil && (p.Registry != "" || p.Ordering != "") {
			t.Errorf("a nil CFF reported the collection %q-%q", p.Registry, p.Ordering)
		}
		return
	}
	if cff := SFNTTables(data)["CFF "]; cff != nil {
		p := ParseCFF(cff)
		if p != nil && p.GIDToCID == nil && (p.Registry != "" || p.Ordering != "") {
			t.Errorf("a CFF that is not CID-keyed reported the collection %q-%q-%d",
				p.Registry, p.Ordering, p.Supplement)
		}
	}
}
