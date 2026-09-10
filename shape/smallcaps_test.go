package shape

import "testing"

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
	small, _ := f.ShapeGlyphsInContext("abc", "", "", Features{SmallCaps: true})
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
	small, _ := f.ShapeGlyphsInContext("office", "", "", Features{SmallCaps: true})
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
	small, _ := f.ShapeGlyphsInContext("aA1", "", "", Features{SmallCaps: true})
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
	small, _ := f.ShapeGlyphsInContext("abc", "", "", Features{SmallCaps: true})
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
