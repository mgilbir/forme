package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// CSS Fonts 4 §6.7's font-variant-numeric, which is a *set* where the capitals
// beside it are one value: a document may ask for oldstyle figures, tabular
// spacing and a slashed zero at once, and the three are three of the font's
// rules over the same digits.
//
// That is what makes the order between them a question, and the answer is not
// the specification's order. A shaper does not apply 'onum' and then 'zero'; it
// collects the lookups every enabled feature names and walks them in index
// order, because that is the order the designer wrote them in. Noto Sans has
// three pairs where it decides the page:
//
//	onum + zero  the oldstyle slashed zero, a glyph neither alone reaches
//	onum + pnum  the proportional oldstyle figures, likewise
//	onum + frac  the fraction's numerators, because 'frac' is stated later
//
// The numbers below are HarfBuzz's, over the bundled Noto Sans. Every pair of
// the seven features it declares was compared against it and every one matched;
// what is kept here is the rows that say something a single feature does not.

// TestEachNumericFeatureIsAskedForOnItsOwn.
func TestEachNumericFeatureIsAskedForOnItsOwn(t *testing.T) {
	f := numericFace(t)
	for _, c := range []struct {
		what string
		n    Numeric
		want []int
	}{
		{"normal", 0, []int{19, 20, 21, 22}},
		{"lining-nums", NumericLining, []int{19, 20, 21, 22}},
		{"oldstyle-nums", NumericOldstyle, []int{2221, 2222, 2223, 2224}},
		{"proportional-nums", NumericProportional, []int{2241, 2242, 2243, 2244}},
		{"tabular-nums", NumericTabular, []int{19, 20, 21, 22}},
		{"diagonal-fractions", NumericDiagonalFractions, []int{2272, 2273, 2274, 2275}},
		{"slashed-zero", NumericSlashedZero, []int{2251, 20, 21, 22}},
	} {
		got, _ := f.ShapeGlyphsInContext("0123", "", "", Features{Numeric: c.n})
		if !equalInts(gids(got), c.want) {
			t.Errorf("%s set %q as %v, want HarfBuzz's %v", c.what, "0123",
				gids(got), c.want)
		}
	}
}

// TestTwoNumericFeaturesApplyInTheFontsOrderAndNotTheSpecifications.
//
// The three rows the specification's order gets wrong or cannot reach. The
// first two are chains — a glyph neither feature alone produces, which is only
// reachable if one of them sees the other's output — and the third is the one
// where §6.7's order picks the loser: 'onum' is written before 'frac' there, so
// applying the tags in turn sets a line of oldstyle digits and the fraction
// never forms.
func TestTwoNumericFeaturesApplyInTheFontsOrderAndNotTheSpecifications(t *testing.T) {
	f := numericFace(t)
	alone := map[string][]int{}
	for _, c := range []struct {
		tag string
		n   Numeric
	}{{"onum", NumericOldstyle}, {"pnum", NumericProportional},
		{"frac", NumericDiagonalFractions}, {"zero", NumericSlashedZero}} {
		got, _ := f.ShapeGlyphsInContext("0123", "", "", Features{Numeric: c.n})
		alone[c.tag] = gids(got)
	}
	for _, c := range []struct {
		what string
		n    Numeric
		want []int
		// neither says the pair reaches a glyph neither feature alone does,
		// which is what makes it a chain rather than a choice between two.
		neither bool
	}{
		{"onum+zero", NumericOldstyle | NumericSlashedZero,
			[]int{3752, 2222, 2223, 2224}, true},
		{"onum+pnum", NumericOldstyle | NumericProportional,
			[]int{2231, 2232, 2233, 2234}, true},
		{"onum+frac", NumericOldstyle | NumericDiagonalFractions,
			[]int{2272, 2273, 2274, 2275}, false},
	} {
		got, _ := f.ShapeGlyphsInContext("0123", "", "", Features{Numeric: c.n})
		if !equalInts(gids(got), c.want) {
			t.Errorf("%s set %q as %v, want HarfBuzz's %v", c.what, "0123",
				gids(got), c.want)
			continue
		}
		if !c.neither {
			continue
		}
		for tag, one := range alone {
			if equalInts(gids(got), one) {
				t.Errorf("%s gave what %s gives alone; the pair reaches a glyph "+
					"neither of them does, and getting it means one of them saw "+
					"the other's output", c.what, tag)
			}
		}
	}
	// And the third is the row that says the order is the font's: 'onum' comes
	// before 'frac' in §6.7 and after it in this face, and the fraction wins.
	if equalInts(alone["onum"], []int{2272, 2273, 2274, 2275}) {
		t.Fatal("'onum' alone gives the fraction's glyphs, so the row above " +
			"cannot tell the two orders apart")
	}
}

// TestALookupTwoFeaturesNameIsRunOnce.
//
// A lookup is one piece of work however many features point at it, and running
// it twice is not the same as running it once — a substitution applied to its
// own output is a second substitution. The fixture states one lookup under two
// tags and maps a to b and b to c: run once the answer is b, run twice it is c.
func TestALookupTwoFeaturesNameIsRunOnce(t *testing.T) {
	f := twoTagsOneLookupFace(t)
	got, _ := f.ShapeGlyphsInContext("a", "", "",
		Features{Numeric: NumericOldstyle | NumericSlashedZero})
	if want := []int{2}; !equalInts(gids(got), want) {
		t.Errorf("the run came out as %v, want %v — the lookup the two features "+
			"share was applied to its own output", gids(got), want)
	}
}

// TestNumericFeaturesReturnTheirTagsInOrder.
func TestNumericFeaturesReturnTheirTagsInOrder(t *testing.T) {
	for _, c := range []struct {
		n    Numeric
		want []string
	}{
		{0, nil},
		{NumericLining, []string{"lnum"}},
		{NumericOldstyle | NumericTabular, []string{"onum", "tnum"}},
		{NumericOrdinal | NumericSlashedZero, []string{"ordn", "zero"}},
		{NumericOldstyle | NumericProportional | NumericStackedFractions |
			NumericOrdinal | NumericSlashedZero,
			[]string{"onum", "pnum", "afrc", "ordn", "zero"}},
	} {
		if got := c.n.Features(); !equalStringSlices(got, c.want) {
			t.Errorf("Numeric(%b).Features() = %v, want %v", c.n, got, c.want)
		}
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// numericFace is the bundled face, which declares seven of §6.7's eight.
func numericFace(t *testing.T) *Face {
	t.Helper()
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading the bundled face: %v", err)
	}
	for _, tag := range []string{"lnum", "onum", "pnum", "tnum", "frac", "ordn", "zero"} {
		if !offers(f, tag) {
			t.Fatalf("the bundled face declares %v and not %s; the numbers in "+
				"this file no longer describe it", f.Features(), tag)
		}
	}
	if offers(f, "afrc") {
		t.Fatal("the bundled face now declares afrc, so it is no longer the " +
			"face these numbers were taken from")
	}
	return f
}

// twoTagsOneLookupFace declares one lookup under two feature tags.
//
// It is the only way to state the case: a font whose 'onum' and 'zero' point at
// the same lookup, which real fonts do whenever two features share a rule. The
// lookup maps a to b and b to c, so running it once gives b and running it twice
// gives c — which is what tells the two apart.
func twoTagsOneLookupFace(t *testing.T) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "TwoTags",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true}, // 1
			{Rune: 'b', Advance: 500, HasShape: true}, // 2
			{Rune: 'c', Advance: 500, HasShape: true}, // 3
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{
				Type:      1,
				Subtables: [][]byte{fonttest.SingleSubst([]int{1, 2}, []int{2, 3})},
			}}, map[string][]int{"onum": {0}, "zero": {0}}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}
