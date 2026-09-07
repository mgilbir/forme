package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// What Features() is for is deciding whether to ask, so it has to name the
// features asking would change.
//
// It named a different set: the tags the flat single-substitution reader kept a
// table for, which is every feature whose lookups are all type 1 and no other.
// A feature offered through a ligature or a contextual rule was missing, so a
// caller that checked first was told the face has nothing — and asking would
// have worked. Over the fetched faces:
//
//	face                       listed   offered
//	NotoSans-Regular              22       25
//	NotoSansJP-VF                 13       16
//	NotoSansArabic-Regular         8       12
//	NotoSansDevanagari-Regular     1       12

// TestAFeatureOfferedThroughALigatureIsListed is the case stated as a font.
//
// One feature, "smcp", whose only lookup is a ligature substitution. Nothing
// about it is unusual — small capitals are commonly a mix of single and
// multiple substitutions — and it was invisible.
func TestAFeatureOfferedThroughALigatureIsListed(t *testing.T) {
	f := contextFaceWithFeature(t, []fonttest.Lookup{{
		Type: 4,
		Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
			{Components: []int{gidA, gidB}, Glyph: gidBalt},
		})},
	}}, "smcp", []int{0})

	// It is listed.
	listed := false
	for _, tag := range f.Features() {
		if tag == "smcp" {
			listed = true
		}
	}
	if !listed {
		t.Errorf("Features() came back with %v, and the face offers smcp; a "+
			"caller checking before asking is told there is nothing to ask for",
			f.Features())
	}

	// And asking for it does something, which is what being listed claims.
	plain, _ := f.ShapeGlyphs("ab")
	small, _ := f.ShapeGlyphsWith("ab", "smcp")
	if len(plain) != 2 {
		t.Fatalf("the plain run came back with %d glyphs, want 2", len(plain))
	}
	if len(small) != 1 || small[0].GID != gidBalt {
		t.Errorf("asking for smcp gave %d glyphs, want the one ligature; a "+
			"feature that is listed and does nothing is the same fault the "+
			"other way round", len(small))
	}
}

// TestEveryListedFeatureIsOneShapingActsOn is the other direction, over the
// fetched faces: nothing is listed that asking for would not reach.
func TestEveryListedFeatureIsOneShapingActsOn(t *testing.T) {
	for _, name := range []string{
		"NotoSans-Regular.ttf",
		"NotoSansArabic-Regular.ttf",
		"NotoSansDevanagari-Regular.ttf",
		"NotoSansJP-VF.ttf",
	} {
		f, err := Load(fonttest.NotoFile(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		listed := f.Features()
		if len(listed) == 0 {
			t.Errorf("%s lists no features at all", name)
			continue
		}
		for _, tag := range listed {
			if len(f.layout.featureLookups[tag]) == 0 {
				t.Errorf("%s lists %q, and applyNamedFeatures has no lookups "+
					"for it, so asking would do nothing", name, tag)
			}
		}
		for i := 1; i < len(listed); i++ {
			if listed[i] <= listed[i-1] {
				t.Errorf("%s: Features() is not sorted at %d: %v", name, i, listed)
				break
			}
		}
	}
}
