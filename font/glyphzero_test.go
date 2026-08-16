package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Glyph 0 is .notdef, and .notdef is an answer.
//
// TrueTypeGID has three outcomes and they are not two: a code the font maps to
// .notdef and a font this reader cannot ask both come back with glyph 0, and
// only the second is ignorance. A consumer testing `gid == 0` merges a finding
// with the absence of one — which is what issue #47 records happening downstream.
//
// Nothing here asserts what a consumer does with the answer. What it asserts is
// that the answer is still there to be read, so that an API change that dropped
// the distinction — returning a bare int, or folding .notdef into "not found" —
// fails here rather than in whatever reads it next.

// TestNotdefIsAnAnswerAndNotAnAbsence.
func TestNotdefIsAnAnswerAndNotAnAbsence(t *testing.T) {
	type sub = fonttest.CmapSub
	// A font with a (3,1) cmap holding U+0041 only. It can be asked about every
	// code; most of them it answers .notdef.
	withCmap := ParseSFNT(fonttest.SFNTWithCmapSubtables([]sub{{
		Plat: 3, Enc: 1,
		Data: fonttest.CmapFormat4([][3]int{{0x0041, 0x0041, 100 - 0x41}, {0xFFFF, 0xFFFF, 1}}),
	}}), 1<<18)
	// A font with no cmap subtable this rule reads at all.
	noCmap := ParseSFNT(fonttest.SFNTWithCmapSubtables(nil), 1<<18)

	for _, tc := range []struct {
		what     string
		fp       *Program
		symbolic bool
		code     byte
		name     string
		wantGID  int
		wantOK   bool
	}{
		{"a code the font maps", withCmap, false, 'A', "A", 100, true},
		// The font has a cmap and this name is not in it. That is the font's
		// answer, not a gap in the reader.
		{"a named code the cmap does not hold", withCmap, false, 'B', "B", 0, true},
		// ISO 32000-1 9.6.6.4: a non-symbolic code with no glyph name renders
		// .notdef. Also an answer.
		{"a code with no name at all", withCmap, false, 'B', "", 0, true},
		// Nothing to ask. A rule may not assert against this.
		{"a font with no readable cmap", noCmap, false, 'A', "A", 0, false},
		{"a symbolic lookup with no symbol cmap", withCmap, true, 'A', "A", 0, false},
	} {
		gid, ok := TrueTypeGID(tc.fp, tc.symbolic, tc.code, tc.name)
		if gid != tc.wantGID || ok != tc.wantOK {
			t.Errorf("%s: TrueTypeGID = (%d, %v), want (%d, %v)",
				tc.what, gid, ok, tc.wantGID, tc.wantOK)
		}
	}
}

// TestGlyphZeroAloneCannotTellThemApart is the point stated as a property: among
// the cases above, glyph 0 comes back for both an answer and an absence, so the
// integer on its own carries strictly less than the pair does.
//
// If this ever fails because every glyph-0 case agrees on the boolean, the
// second result has become redundant and the API should say so — which is a
// finding either way, and the reason this is a test rather than a comment.
func TestGlyphZeroAloneCannotTellThemApart(t *testing.T) {
	type sub = fonttest.CmapSub
	withCmap := ParseSFNT(fonttest.SFNTWithCmapSubtables([]sub{{
		Plat: 3, Enc: 1,
		Data: fonttest.CmapFormat4([][3]int{{0x0041, 0x0041, 100 - 0x41}, {0xFFFF, 0xFFFF, 1}}),
	}}), 1<<18)
	noCmap := ParseSFNT(fonttest.SFNTWithCmapSubtables(nil), 1<<18)

	answered, ignorant := 0, 0
	for _, tc := range []struct {
		fp   *Program
		name string
	}{
		{withCmap, "B"}, {withCmap, ""}, {noCmap, "A"},
	} {
		gid, ok := TrueTypeGID(tc.fp, false, 'B', tc.name)
		if gid != 0 {
			t.Fatalf("the fixture stopped producing glyph 0: got %d", gid)
		}
		if ok {
			answered++
		} else {
			ignorant++
		}
	}
	if answered == 0 || ignorant == 0 {
		t.Errorf("of three glyph-0 results, %d were the font's answer and %d were "+
			"the reader's ignorance; the two must both occur or the second result "+
			"is carrying nothing", answered, ignorant)
	}
}
