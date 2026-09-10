package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The rule a document turns on rather than off.
//
// Features carried three flags and every one of them suppressed something, so
// the shaping layer had no way to be *asked* for a feature by anything but a
// caller naming a tag — and a tag named that way is applied last, after the
// ligatures. "font-variant: small-caps" over "office" came out as three small
// capitals and an ffi ligature standing in the middle of them, because 'liga'
// ran first and matched the lowercase letters that were still there.
//
// The numbers below are HarfBuzz's, taken from the bundled Noto Sans:
//
//	office  plain  82 1656 70 72          (the ffi ligature is 1656)
//	office  smcp   1939 1902 1902 1911 1882 1892
//	aA1     smcp   1868 36 20             (only the lowercase letter changes)
//
// So the ligature does not form, and the reason it does not is the order: by
// the time 'liga' is reached there is no "ffi" left to match.

// smallCapsFace is the bundled face, which declares 'smcp'.
func smallCapsFace(t *testing.T) *Face {
	t.Helper()
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading the bundled face: %v", err)
	}
	if !offers(f, "smcp") {
		t.Fatalf("the bundled face declares %v and not smcp; the test has "+
			"nothing to ask for", f.Features())
	}
	return f
}

func offers(f *Face, tag string) bool {
	for _, t := range f.Features() {
		if t == tag {
			return true
		}
	}
	return false
}

// TestSmallCapsAsksTheFaceForTheCapitalsItDrew is the feature applied at all.
func TestSmallCapsAsksTheFaceForTheCapitalsItDrew(t *testing.T) {
	f := smallCapsFace(t)
	plain, _ := f.ShapeGlyphsInContext("abc", "", "", Features{})
	small, _ := f.ShapeGlyphsInContext("abc", "", "", Features{Caps: CapsSmall})
	if want := []int{68, 69, 70}; !equalInts(gids(plain), want) {
		t.Fatalf("the plain run is %v, want %v; the oracle's numbers no longer "+
			"describe this face and the rest of the file cannot be read",
			gids(plain), want)
	}
	if want := []int{1868, 1881, 1882}; !equalInts(gids(small), want) {
		t.Errorf("small capitals gave %v, want HarfBuzz's %v", gids(small), want)
	}
}

// TestSmallCapsIsAppliedBeforeTheLigatures is the ordering, which is the whole
// reason this is not a caller-named tag.
func TestSmallCapsIsAppliedBeforeTheLigatures(t *testing.T) {
	f := smallCapsFace(t)
	plain, _ := f.ShapeGlyphsInContext("office", "", "", Features{})
	if len(plain) != 4 {
		t.Fatalf("the plain run came out as %d glyphs (%v), want 4 with the "+
			"ffi ligature; without that ligature this test asserts nothing",
			len(plain), gids(plain))
	}
	small, _ := f.ShapeGlyphsInContext("office", "", "", Features{Caps: CapsSmall})
	want := []int{1939, 1902, 1902, 1911, 1882, 1892}
	if !equalInts(gids(small), want) {
		t.Errorf("small capitals over \"office\" gave %v, want HarfBuzz's %v.\n"+
			"Six glyphs and no ligature: 'liga' is stated over the lowercase "+
			"letters, and applying 'smcp' first leaves it nothing to match.",
			gids(small), want)
	}
	// And the clusters still map back to the characters, which is what a caller
	// cutting a run at a byte offset depends on.
	for i, g := range small {
		if g.Cluster != i {
			t.Errorf("glyph %d has cluster %d, want %d; every character of "+
				"\"office\" is one glyph of its own here", i, g.Cluster, i)
		}
	}
}

// TestSmallCapsLeavesAloneWhatTheFaceLeavesAlone: the feature is the font's, so
// a capital and a digit come through untouched.
func TestSmallCapsLeavesAloneWhatTheFaceLeavesAlone(t *testing.T) {
	f := smallCapsFace(t)
	small, _ := f.ShapeGlyphsInContext("aA1", "", "", Features{Caps: CapsSmall})
	want := []int{1868, 36, 20}
	if !equalInts(gids(small), want) {
		t.Errorf("small capitals over %q gave %v, want HarfBuzz's %v; only the "+
			"lowercase letter has a small capital drawn for it", "aA1",
			gids(small), want)
	}
}

// TestSmallCapsOnAFaceWithoutItSetsTheTextPlainly is the case every one of the
// fourteen standard PDF faces is in.
//
// Nothing is refused and nothing is dropped: the run comes out in ordinary
// letters, at the same width, and it is the *caller* that has to notice — see
// Face.Features and layout's report.
func TestSmallCapsOnAFaceWithoutItSetsTheTextPlainly(t *testing.T) {
	f, err := NotoSansSimple()
	if err != nil {
		t.Fatalf("loading the face: %v", err)
	}
	plain, _ := f.ShapeGlyphsInContext("abc", "", "", Features{})
	small, _ := f.ShapeGlyphsInContext("abc", "", "", Features{Caps: CapsSmall})
	if !equalInts(gids(plain), gids(small)) {
		t.Errorf("a face read without its layout tables gave %v with small "+
			"capitals and %v without; asking for a feature a face has not got "+
			"must change nothing", gids(small), gids(plain))
	}
	if MeasureGlyphs(small, 12) != MeasureGlyphs(plain, 12) {
		t.Errorf("the run measured %v with small capitals and %v without",
			MeasureGlyphs(small, 12), MeasureGlyphs(plain, 12))
	}
}

// The other five values of §6.6, which are the same mechanism with other tags.
//
// Every one of them is a request for features the face declares, so what has to
// be checked is that each value asks for its own and applies them — and that is
// not something the fetched faces can show. Noto Sans declares 'smcp' and 'c2sc'
// and none of 'pcap', 'c2pc', 'unic' or 'titl', which is what almost every face
// in the world declares: a test written against it would report that four of the
// six values do nothing, and could not tell that from their doing nothing
// because the engine forgot to ask.
//
// So the fixture is a font that declares all six, each mapping a different
// letter to a glyph of its own. A value that asked for the wrong tag would
// substitute the wrong letter, and a value that asked for none would substitute
// nothing.

// Glyph indices in capsFace, in the order the glyphs are declared.
const (
	capsGidA = 1 + iota // 'a', what 'smcp' and 'pcap' cover
	capsGidB            // 'B', what 'c2sc', 'c2pc' and 'titl' cover
	capsGidSmcp
	capsGidC2sc
	capsGidPcap
	capsGidC2pc
	capsGidUnicA
	capsGidTitl
)

// capsFace declares all six of §6.6's features, each over one letter.
//
// 'unic' covers the lowercase 'a' alone rather than both letters, because it is
// the one value that acts on either case and a fixture where it covered both
// could not tell "asked for unic" from "asked for unic and something else".
func capsFace(t *testing.T) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Caps",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'B', Advance: 500, HasShape: true},
			{Rune: 0xE000, Advance: 400, HasShape: true}, // a.smcp
			{Rune: 0xE001, Advance: 400, HasShape: true}, // B.c2sc
			{Rune: 0xE002, Advance: 300, HasShape: true}, // a.pcap
			{Rune: 0xE003, Advance: 300, HasShape: true}, // B.c2pc
			{Rune: 0xE004, Advance: 450, HasShape: true}, // a.unic
			{Rune: 0xE005, Advance: 550, HasShape: true}, // B.titl
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"smcp": {{capsGidA}, {capsGidSmcp}},
				"c2sc": {{capsGidB}, {capsGidC2sc}},
				"pcap": {{capsGidA}, {capsGidPcap}},
				"c2pc": {{capsGidB}, {capsGidC2pc}},
				"unic": {{capsGidA}, {capsGidUnicA}},
				"titl": {{capsGidB}, {capsGidTitl}},
			}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestEachCapsValueAsksForItsOwnFeatures.
func TestEachCapsValueAsksForItsOwnFeatures(t *testing.T) {
	f := capsFace(t)
	for _, c := range []struct {
		what string
		caps Caps
		want []int
	}{
		{"normal", CapsNormal, []int{capsGidA, capsGidB}},
		{"small-caps", CapsSmall, []int{capsGidSmcp, capsGidB}},
		{"all-small-caps", CapsAllSmall, []int{capsGidSmcp, capsGidC2sc}},
		{"petite-caps", CapsPetite, []int{capsGidPcap, capsGidB}},
		{"all-petite-caps", CapsAllPetite, []int{capsGidPcap, capsGidC2pc}},
		{"unicase", CapsUnicase, []int{capsGidUnicA, capsGidB}},
		{"titling-caps", CapsTitling, []int{capsGidA, capsGidTitl}},
	} {
		got, _ := f.ShapeGlyphsInContext("aB", "", "", Features{Caps: c.caps})
		if !equalInts(gids(got), c.want) {
			t.Errorf("%s set %q as %v, want %v", c.what, "aB", gids(got), c.want)
		}
	}
}

// TestTheTwoHalvesOfAllSmallCapsAreIndependent.
//
// "all-small-caps" is two features over two disjoint sets of letters, so a face
// declaring one of them carries out half the request — the lowercase letters
// lowered and the capitals left standing, which is a line in two heights. It is
// not an error and nothing here reports it; what matters at this layer is that
// the half that can be done is done, so that the caller's report is about the
// half that was not.
func TestTheTwoHalvesOfAllSmallCapsAreIndependent(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "HalfCaps",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'B', Advance: 500, HasShape: true},
			{Rune: 0xE000, Advance: 400, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"smcp": {{1}, {3}},
			}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	got, _ := f.ShapeGlyphsInContext("aB", "", "", Features{Caps: CapsAllSmall})
	if want := []int{3, 2}; !equalInts(gids(got), want) {
		t.Errorf("a face with smcp and no c2sc set %q as %v under all-small-caps, "+
			"want %v — the lowercase letter lowered and the capital left alone",
			"aB", gids(got), want)
	}
}

// TestAllSmallCapsLowersBothCasesInARealFace is the oracle's numbers for the
// value that Noto Sans really does declare both halves of.
//
//	aA1  plain      68 36 20
//	aA1  smcp       1868 36 20
//	aA1  c2sc       68 1868 20
//	aA1  c2sc+smcp  1868 1868 20
//
// The last row is what "all-small-caps" is: one glyph for both cases of the
// letter, and the digit untouched.
func TestAllSmallCapsLowersBothCasesInARealFace(t *testing.T) {
	f := smallCapsFace(t)
	plain, _ := f.ShapeGlyphsInContext("aA1", "", "", Features{})
	if want := []int{68, 36, 20}; !equalInts(gids(plain), want) {
		t.Fatalf("the plain run is %v, want %v; the oracle's numbers no longer "+
			"describe this face", gids(plain), want)
	}
	all, _ := f.ShapeGlyphsInContext("aA1", "", "", Features{Caps: CapsAllSmall})
	if want := []int{1868, 1868, 20}; !equalInts(gids(all), want) {
		t.Errorf("all-small-caps gave %v, want HarfBuzz's %v", gids(all), want)
	}
}
