package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A class-based pair adjustment reached through a contextual rule.
//
// GPOS type 2 has two formats. Format 1 lists pairs by glyph; format 2 states
// them by *class*, which is how a real font carries a large kerning table
// without listing every pair — Noto Sans Arabic's seventy thousand pairs are
// classes, not glyphs.
//
// Both are read here, and only one had ever been read *through a rule*. A
// contextual positioning lookup names another lookup to apply at a position, and
// pairPosAt is what applies it when that lookup is a pair adjustment; its
// format 1 arm is exercised and its format 2 arm was at 0% across every unit
// test and all 6253 reftest documents. Six statements, and the one that computes
// where in the rectangle of records a pair sits — a font whose classes came out
// wrong would be kerned by whatever record that arithmetic landed on, which is a
// plausible number rather than none.
//
// The lookup the rule names is not named by any feature, so nothing but the rule
// can reach it: a font's flat kern table is built from the lookups its features
// name, and this one is not among them.
const (
	cpcA, cpcB, cpcC = 1, 2, 3
	cpcAdvance       = 500
	cpcKern          = -120
	cpcOtherKern     = 60
)

// contextPairFace builds a face whose 'kern' feature is one contextual rule:
// where "ab" occurs, apply the pair lookup at position 0.
//
// The pair lookup states its pairs by class — A in class 1, B in class 1 of the
// second classDef — with class 0 left empty on both axes, which is the row and
// column a real font leaves for everything unclassified.
func contextPairFace(t *testing.T, pair []byte) *Face {
	t.Helper()
	lookups := []fonttest.Lookup{
		{Type: 2, Subtables: [][]byte{pair}},
		{Type: 7, Subtables: [][]byte{fonttest.SequenceContext3(
			[][]int{{cpcA}, {cpcB}}, []fonttest.SeqLookup{{At: 0, Lookup: 0}})}},
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "ContextPairClass",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: cpcAdvance, HasShape: true},
			{Rune: 'b', Advance: cpcAdvance, HasShape: true},
			{Rune: 'c', Advance: cpcAdvance, HasShape: true},
		},
		Extra: map[string][]byte{
			// Only lookup 1, the rule, is named. Lookup 0 is reachable through
			// it and nowhere else.
			"GPOS": fonttest.GPOSLookups(lookups, map[string][]int{"kern": {1}}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// classPairSubtable states the pair by class: A is class 1 on the first axis, B
// is class 1 on the second, and that cell carries the kern.
func classPairSubtable() []byte {
	return fonttest.PairPosClassSubtable(
		[]int{cpcA},
		map[int]int{cpcA: 1},
		map[int]int{cpcB: 1},
		2, 2,
		[]fonttest.ClassPair{{Class1: 1, Class2: 1, Adjust: cpcKern}},
	)
}

func pairAdvancesOf(t *testing.T, f *Face, s string) []float64 {
	t.Helper()
	glyphs, missing := f.ShapeGlyphs(s)
	if missing != 0 {
		t.Fatalf("shaping %q: %d characters have no glyph", s, missing)
	}
	out := make([]float64, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.XAdvance
	}
	return out
}

func TestAClassPairIsAppliedThroughAContextualRule(t *testing.T) {
	f := contextPairFace(t, classPairSubtable())

	if got, want := pairAdvancesOf(t, f, "ab")[0], float64(cpcAdvance+cpcKern); got != want {
		t.Errorf("in \"ab\" the a came out %v wide; the rule applies a "+
			"class-based pair adjustment of %d there, so it should be %v",
			got, cpcKern, want)
	}
	// The rule is what carries it: the same pair with no rule matching is
	// untouched, which says the adjustment did not arrive some other way.
	if got, want := pairAdvancesOf(t, f, "cb")[0], float64(cpcAdvance); got != want {
		t.Errorf("in \"cb\" the c came out %v wide; the rule matches \"ab\" "+
			"only, so nothing should have moved and it should be %v", got, want)
	}
	if got, want := pairAdvancesOf(t, f, "ac")[0], float64(cpcAdvance); got != want {
		t.Errorf("in \"ac\" the a came out %v wide; the rule matches \"ab\" "+
			"only, so it should be %v", got, want)
	}
}

// The record is looked up at class1*class2Count + class2, and getting that
// arithmetic wrong lands on a different cell of the rectangle rather than on
// none — a plausible number, silently.
//
// So the rectangle is filled with distinguishable numbers and the right cell is
// named. A subtable with one cell in it cannot tell a correct index from an
// index of zero.
//
// The rectangle is deliberately *not square*: three classes on one axis and four
// on the other. With equal counts a reader that strode by the wrong one — n1
// where the format says n2 — lands on the right cell anyway, and a square
// fixture cannot tell rows from columns. That plant was missed until this was
// widened, which is the whole reason it is 3 by 4.
func TestTheRightCellOfTheClassRectangleIsRead(t *testing.T) {
	const n1, n2 = 3, 4
	// Every cell distinct: cell (c1,c2) kerns by -(10*c1 + c2). A is class 2 and
	// B is class 1, so the record is at 2*4+1 and the answer is -21. Striding by
	// n1 would reach 2*3+1, which is the cell holding -12.
	var pairs []fonttest.ClassPair
	for c1 := 0; c1 < n1; c1++ {
		for c2 := 0; c2 < n2; c2++ {
			pairs = append(pairs, fonttest.ClassPair{
				Class1: c1, Class2: c2, Adjust: -(10*c1 + c2),
			})
		}
	}
	f := contextPairFace(t, fonttest.PairPosClassSubtable(
		[]int{cpcA},
		map[int]int{cpcA: 2},
		map[int]int{cpcB: 1},
		n1, n2, pairs,
	))

	got := pairAdvancesOf(t, f, "ab")[0]
	want := float64(cpcAdvance - 21)
	if got != want {
		t.Errorf("the a came out %v wide, want %v. The first glyph is class 2 "+
			"and the second class 1, so the record is at 2*%d+1; %v is the cell "+
			"holding %v instead", got, want, n2, got, cpcAdvance-got)
	}
}

// Class 0 is every glyph the subtable did not classify, and it is a real row and
// column rather than an absence.
//
// A font states the pairs it cares about and leaves the rest in class 0, so a
// reader that treated an unclassified glyph as "no match" would drop whatever
// the font put in row or column nought — which is where a font states a kern
// against *anything*.
func TestAnUnclassifiedGlyphIsClassZeroAndNotAMiss(t *testing.T) {
	f := contextPairFace(t, fonttest.PairPosClassSubtable(
		[]int{cpcA},
		map[int]int{}, // A is not classified: it is class 0
		map[int]int{cpcB: 1},
		2, 2,
		[]fonttest.ClassPair{{Class1: 0, Class2: 1, Adjust: cpcOtherKern}},
	))

	if got, want := pairAdvancesOf(t, f, "ab")[0], float64(cpcAdvance+cpcOtherKern); got != want {
		t.Errorf("the a came out %v wide; it is in no class, which is class 0, "+
			"and the font states %d for class 0 against class 1 — so it should "+
			"be %v", got, cpcOtherKern, want)
	}
}
