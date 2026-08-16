package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// CharacterCollection is what a PDF writes into a CIDFont's /CIDSystemInfo, and
// ISO 32000-2 9.7.4.2 requires that to be compatible with the character
// collection of the glyph source. So the two things worth testing are that a
// face says the right thing, and that a face with nothing to say says nothing —
// because the fallback a caller reaches for when it gets an empty answer is
// Adobe-Identity-0, which is not a neutral default but a specific claim, and it
// is wrong for exactly the fonts this is here to tell apart.

// ottoFace wraps a synthetic CFF in an OpenType container and loads it, so the
// CFF paths can be exercised against a font that is deliberately malformed —
// which is not something a fetched face can be asked to be.
func ottoFace(t *testing.T, opts fonttest.CFFOptions) *Face {
	t.Helper()
	glyphs := []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
	}
	opts.Glyphs = len(glyphs) + 1 // .notdef and the two above
	data := fonttest.OTTO(fonttest.CFF(opts), fonttest.SFNTOptions{Glyphs: glyphs})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("the synthetic OpenType/CFF face did not load: %v", err)
	}
	if !f.IsCFF() {
		t.Fatal("the fixture was not read as CFF, so it exercises none of this")
	}
	return f
}

func TestACIDKeyedFaceStatesItsCharacterCollection(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	reg, ord, sup, ok := f.CharacterCollection()
	if !ok {
		t.Fatal("a CID-keyed face reports no character collection, so a document " +
			"embedding it has nothing true to write into /CIDSystemInfo")
	}
	// Noto's CJK faces are numbered in their own collection rather than one of
	// Adobe's published ones, which is what Adobe-Identity-0 says.
	if reg != "Adobe" || ord != "Identity" || sup != 0 {
		t.Errorf("the collection is %s-%s-%d, want Adobe-Identity-0", reg, ord, sup)
	}
}

// TestASubsetStatesTheSameCollectionAsTheFaceItCameFrom is the invariant a
// consumer relies on whether or not it knows it does.
//
// The /CIDSystemInfo has to describe the program the document actually carries,
// which is the subset — but a caller reads the collection off the face, because
// that is where the method is. The two are the same answer only because
// subsetting copies the ROS operator and the whole string INDEX through
// untouched, so the SIDs still resolve to the same names. Nothing else here
// checks that, and a change to what the Top DICT rewrite copies would break it
// silently, in the one direction that matters: a document making a false
// statement about its own numbering.
func TestASubsetStatesTheSameCollectionAsTheFaceItCameFrom(t *testing.T) {
	f, err := Load(cidKeyedFace(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantReg, wantOrd, wantSup, ok := f.CharacterCollection()
	if !ok {
		t.Fatal("the face states no collection, so there is nothing to preserve")
	}
	// Latin as well as ideographs: the two are hinted separately in such a
	// face, so the subset keeps glyphs from more than one Font DICT and the
	// rewrite has real work to do.
	f.Encode("日本語 and Latin")

	prog, _, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	g, err := Load(prog)
	if err != nil {
		t.Fatalf("the subset does not load: %v", err)
	}
	gotReg, gotOrd, gotSup, ok := g.CharacterCollection()
	if !ok {
		t.Fatal("the subset states no character collection where the face did — " +
			"a document embedding it would describe a numbering it does not carry")
	}
	if gotReg != wantReg || gotOrd != wantOrd || gotSup != wantSup {
		t.Errorf("the subset is numbered in %s-%s-%d and the face it came from "+
			"in %s-%s-%d", gotReg, gotOrd, gotSup, wantReg, wantOrd, wantSup)
	}
}

// TestASyntheticCIDKeyedFaceStatesItsCollection is the positive control for the
// fixture the tests below are built on. Without it those tests could pass by
// the face never being CID-keyed at all, which is the failure they are meant to
// distinguish from.
func TestASyntheticCIDKeyedFaceStatesItsCollection(t *testing.T) {
	f := ottoFace(t, fonttest.CFFOptions{
		CIDKeyed: true, Registry: "Acme", Ordering: "Japan9", Supplement: 3,
	})
	reg, ord, sup, ok := f.CharacterCollection()
	if !ok {
		t.Fatal("a CID-keyed face states no collection")
	}
	if reg != "Acme" || ord != "Japan9" || sup != 3 {
		t.Errorf("the collection is %s-%s-%d, want Acme-Japan9-3", reg, ord, sup)
	}
}

// TestAFaceWithNoCollectionToStateSaysSo covers the three separate ways a
// caller can end up writing a collection that is not there. They are one answer
// from the outside, which is the point of the method: a caller gets one branch
// to remember instead of three.
func TestAFaceWithNoCollectionToStateSaysSo(t *testing.T) {
	t.Run("no CFF at all", func(t *testing.T) {
		// A TrueType face. There is no ROS anywhere in the format, and a
		// CIDFontType0's collection is not a thing it can have.
		assertNoCollection(t, latinFace(t))
	})
	t.Run("CFF that is not CID-keyed", func(t *testing.T) {
		// The glyph index is the only numbering such a font has, so there is no
		// collection to name — and this is the case that a check written
		// against "is it CFF" rather than "is it CID-keyed" gets wrong.
		assertNoCollection(t, ottoFace(t, fonttest.CFFOptions{}))
	})
	t.Run("CID-keyed but names no collection", func(t *testing.T) {
		// A ROS whose SIDs point past the strings the font carries. It is
		// CID-keyed, it has a numbering, and it has not said which.
		assertNoCollection(t, ottoFace(t, fonttest.CFFOptions{
			CIDKeyed: true, UnnamedCollection: true,
		}))
	})
	t.Run("CID-keyed with a supplement below zero", func(t *testing.T) {
		// A supplement is a version of the collection and counts up. This is
		// the dangerous one: the two names parse, so the answer is most of a
		// collection, and most of one is what reaches a document unnoticed.
		assertNoCollection(t, ottoFace(t, fonttest.CFFOptions{
			CIDKeyed: true, NegativeSupplement: true,
		}))
	})
}

func assertNoCollection(t *testing.T, f *Face) {
	t.Helper()
	reg, ord, sup, ok := f.CharacterCollection()
	if ok {
		t.Errorf("the face states the collection %s-%s-%d, which it does not have",
			reg, ord, sup)
	}
	// And nothing comes back beside the false, so a caller that writes the
	// values without checking ok writes empties rather than a plausible name.
	if reg != "" || ord != "" || sup != 0 {
		t.Errorf("ok is false and the values are %q-%q-%d", reg, ord, sup)
	}
}
