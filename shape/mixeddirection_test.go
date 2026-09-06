package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// mixedFace has a Latin pair to kern, a Latin ligature to form, and a Hebrew
// letter — everything a string that changes direction needs.
func mixedFace(t *testing.T) *Face {
	t.Helper()
	const (
		gidA = 1 + iota
		gidV
		gidF
		gidAlef
		gidFF
	)
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Mixed",
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 500, HasShape: true},
			{Rune: 'V', Advance: 500, HasShape: true},
			{Rune: 'f', Advance: 500, HasShape: true},
			{Rune: 0x05D0, Advance: 500, HasShape: true}, // ALEF
			{Rune: 0xE000, Advance: 700, HasShape: true}, // the ff ligature
		},
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOS([]fonttest.KernPair{{Left: gidA, Right: gidV, Adjust: -80}}),
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{
				Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
					{Components: []int{gidF, gidF}, Glyph: gidFF},
				})},
			}}, map[string][]int{"liga": {0}}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestAFeatureTurnedOffStaysOffWhereTheDirectionChanges is the option dropped
// at a boundary the caller never wrote.
//
// A string that changes direction is shaped run by run, and the context each
// run was given was rebuilt without what the caller had turned off. So
// "font-kerning: none" was honoured for a Latin word and ignored for the same
// word beside a Hebrew one — the same declaration, applied or not by whether
// the paragraph happened to change direction.
func TestAFeatureTurnedOffStaysOffWhereTheDirectionChanges(t *testing.T) {
	f := mixedFace(t)
	for _, tc := range []struct{ name, text string }{
		{"one direction", "AV"},
		{"a Hebrew letter before it", "א AV"},
		{"a Hebrew letter after it", "AV א"},
		{"Hebrew on both sides", "א AV א"},
	} {
		on := f.MeasureShapedInContext(tc.text, 1, "", "", true, Features{})
		off := f.MeasureShapedInContext(tc.text, 1, "", "", true, Features{NoKerning: true})
		if on == off {
			t.Errorf("%s: %q measures the same with kerning on and off (%v) — either "+
				"the option was dropped where the direction changes, or the pair is "+
				"not kerned at all and this proves nothing", tc.name, tc.text, on)
			continue
		}
		if off-on < 0.079 || off-on > 0.081 {
			t.Errorf("%s: turning kerning off changed the width by %v, want the pair's "+
				"own 0.08 — the option is dropped where the direction changes",
				tc.name, off-on)
		}
	}
}

// TestALigatureIsStillFormedWhereTheDirectionChanges is the other half of the
// same context: the sides that may contribute glyphs were dropped too, so a
// ligature across an element boundary was formed for a run of one direction and
// not for the same run beside text of the other.
func TestALigatureIsStillFormedWhereTheDirectionChanges(t *testing.T) {
	f := mixedFace(t)
	const gidFF = 5

	// One direction: an "f" with another merged in from the neighbour after it.
	// The two shape as one string and the ligature is formed.
	plain, _ := f.ShapeGlyphsMerged("f", "", "", "", "f", true, Features{})
	if len(plain) != 1 || plain[0].GID != gidFF {
		t.Fatalf("the run shaped to %v on its own, want the ligature; the fixture "+
			"says nothing", gidsOfGlyphs(plain))
	}

	// The same run at the end of a string that changes direction twice, so the
	// string really is shaped run by run: "f", the Hebrew letter, and the "f"
	// this is about. The last run is still the last of the string, and what
	// follows the string still follows it.
	mixed, _ := f.ShapeGlyphsMerged("f\u05D0f", "", "", "", "f", true, Features{})
	if got := gidsOfGlyphs(mixed); len(got) != 3 || got[2] != gidFF {
		t.Errorf("across two changes of direction the run shaped to %v, want the "+
			"ligature it forms without them at the end", got)
	}
}
