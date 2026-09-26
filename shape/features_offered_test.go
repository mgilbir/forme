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
			if len(f.layout.featureLookups[tag]) == 0 && len(f.layout.gposFeatures[tag]) == 0 {
				t.Errorf("%s lists %q, and the plan would find no lookups "+
					"for it in either table, so asking would do nothing", name, tag)
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

// TestAPositioningFeatureIsListed: a feature a face offers through positioning
// alone is offered, because a plan positions with whatever it is asked for.
//
// 'halt' is the case the suite found: every CJK face states its trimmed
// bracket widths as a single adjustment in GPOS and nowhere in GSUB, so a
// face with them was listed as having none, and a document asking for them
// was reported as asking for something the face has not got — while the face
// applied it.
func TestAPositioningFeatureIsListed(t *testing.T) {
	f, err := Load(gposOrderFixtures()["requested"])
	if err != nil {
		t.Fatal(err)
	}
	listed := f.Features()
	count := 0
	for _, tag := range listed {
		if tag == "halt" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Features() came back with %v; the face offers 'halt' through "+
			"positioning and it should be listed once", listed)
	}

	// And asking for it does something, which is what being listed claims.
	named, _ := f.ShapeGlyphsWith("ab", "halt")
	checkShaped(t, "ab with 'halt' named", named,
		[]shapedAs{{goA, 250, -250, 0}, {goB, 600, 0, 0}})

	// A feature both tables name is one feature: a face that substitutes an
	// alternate under 'ss01' and spaces it under 'ss01' as well offers 'ss01'
	// once, not twice.
	both, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "BothTables",
		Glyphs: gposOrderGlyphs(false),
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{Type: 1,
				Subtables: [][]byte{fonttest.SingleSubst([]int{goA}, []int{goX})}}},
				map[string][]int{"ss01": {0}}),
			"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{{Type: 1,
				Subtables: [][]byte{fonttest.SinglePosSubtable(goX, 0, 0, 10)}}},
				map[string][]int{"ss01": {0}}),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := both.Features(); len(got) != 1 || got[0] != "ss01" {
		t.Errorf("a face naming 'ss01' in both tables listed %v, want [ss01]", got)
	}
}

// TestAFeatureWithNoLookupsIsNotListed: a tag a face declares with nothing
// under it changes nothing when asked for, and so is not offered — in either
// table. The layout keeps it all the same, for the shaper's own questions.
func TestAFeatureWithNoLookupsIsNotListed(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "EmptyFeatures",
		Glyphs: gposOrderGlyphs(false),
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups(nil, map[string][]int{"smcp": nil}),
			"GPOS": fonttest.GPOSLookups(nil, map[string][]int{"palt": nil}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := f.layout.featureLookups["smcp"]; !kept {
		t.Fatal("the fixture's empty 'smcp' was not read at all; the case is not being made")
	}
	if _, kept := f.layout.gposFeatures["palt"]; !kept {
		t.Fatal("the fixture's empty 'palt' was not read at all; the case is not being made")
	}
	if got := f.Features(); len(got) != 0 {
		t.Errorf("Features() came back with %v; neither tag has a lookup under it, "+
			"and asking for either changes nothing", got)
	}
}
