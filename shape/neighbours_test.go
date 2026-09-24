package shape

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// What the text either side of a run can change, asked of the run and answered
// from the rules its script selects. See neighbours.go.
//
// Every case is checked both ways against the shaper itself: where the answer
// is no, no context of the set below may change a glyph or a position of the
// run; where it is yes, the case is built so that one does. An answer that is
// only compared with a constant would pass whatever the shaper did.

// contexts is text of each kind a neighbour can be: none, Latin, Arabic, Han,
// Devanagari and Balinese letters, and a digit.
var contexts = []string{"", "a", "\u0628", "\u4E2D", "\u0915", "\u1B13", "1"}

// visible is what a caller can see of a run's glyphs.
func visible(glyphs []Glyph) string {
	var b strings.Builder
	for _, g := range glyphs {
		fmt.Fprintf(&b, "%d@%d+%v(%v,%v) ", g.GID, g.Cluster, g.XAdvance, g.XOffset, g.YOffset)
	}
	return b.String()
}

// contextChanges reports whether any pair of contexts changes what s is drawn
// as, and names the first that does.
func contextChanges(f *Face, s string) (bool, string) {
	bare, _ := f.ShapeGlyphsInContext(s, "", "", Features{})
	for _, before := range contexts {
		for _, after := range contexts {
			g, _ := f.ShapeGlyphsInContext(s, before, after, Features{})
			if visible(g) != visible(bare) {
				return true, fmt.Sprintf("%q before and %q after", before, after)
			}
		}
	}
	return false, ""
}

func loadNeighbourFace(t *testing.T, glyphs []fonttest.Glyph, extra map[string][]byte) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Neighbours", Glyphs: glyphs, Extra: extra}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// checkNeighbours holds ContextCanChange and FormsFollowNeighbours to what the
// shaper does, for each run.
func checkNeighbours(t *testing.T, f *Face, cases []neighbourCase) {
	t.Helper()
	for _, c := range cases {
		if got := f.ContextCanChange(c.run, Features{}); got != c.can {
			t.Errorf("ContextCanChange(%q) = %v, want %v", c.run, got, c.can)
		}
		if got := f.FormsFollowNeighbours(c.run, Features{}); got != c.forms {
			t.Errorf("FormsFollowNeighbours(%q) = %v, want %v", c.run, got, c.forms)
		}
		changes, how := contextChanges(f, c.run)
		switch {
		case changes && !c.can:
			t.Errorf("%q is changed by its context (%s), and the answer is that nothing can change it",
				c.run, how)
		case !changes && c.changes:
			t.Errorf("%q is changed by no context; the case was meant to show one that does", c.run)
		}
	}
}

type neighbourCase struct {
	run string
	// can and forms are the two answers; changes says the case shows a context
	// that changes the run, which is what makes a "yes" more than caution.
	can, forms, changes bool
}

// TestTheNeighbourChoosesTheScriptOfPunctuation is the case the face-wide
// answer could not see. The face has no positional forms, no kerning and no
// 'liga': nothing the old question asked about. It does state a 'locl' form of
// the fullwidth colon under 'hani' and not under 'DFLT', which is how a CJK
// face corrects punctuation for Chinese. A colon set between two Han
// characters is Han (see scriptRuns), so its form is the neighbour's to choose
// — and the run was shaped without the neighbour, under 'DFLT'.
func TestTheNeighbourChoosesTheScriptOfPunctuation(t *testing.T) {
	const gidHan, gidColon, gidColonHan, gidA = 1, 2, 3, 4
	f := loadNeighbourFace(t, []fonttest.Glyph{
		{Rune: '\u4E2D', Advance: 1000, HasShape: true},
		{Rune: '\uFF1A', Advance: 1000, HasShape: true},
		{Rune: '\uE000', Advance: 500, HasShape: true},
		{Rune: 'a', Advance: 500, HasShape: true},
	}, map[string][]byte{
		"GSUB": fonttest.GSUBTable(
			[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
				fonttest.SingleSubst([]int{gidColon}, []int{gidColonHan})}}},
			[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}},
			map[string]fonttest.Script{
				"DFLT": {Required: fonttest.NoFeature},
				"hani": fonttest.AllFeatures(1),
			}),
	})
	if f.HasJoiningForms() || f.HasKerning() || f.HasLigatures() {
		t.Fatal("the fixture was meant to have none of what the face-wide answer asked about")
	}
	g, _ := f.ShapeGlyphsInContext("\uFF1A", "\u4E2D", "\u4E2D", Features{})
	if len(g) != 1 || g[0].GID != gidColonHan {
		t.Fatalf("the colon between two Han characters is %v; the fixture's 'hani' form was not reached", g)
	}
	checkNeighbours(t, f, []neighbourCase{
		{run: "\uFF1A", can: true, forms: false, changes: true},
		{run: "\u4E2D", can: false, forms: false},
		{run: "a", can: false, forms: false},
		{run: "a\uFF1A", can: false, forms: false},
	})
}

// TestTheRulesOfTheRunsScriptAreAsked: a face with Arabic forms and kerning
// under 'arab' and nothing for Latin. The face-wide answer said a Latin run's
// neighbours could change it, because the font had forms and pairs somewhere;
// they cannot, and the Arabic run's can.
func TestTheRulesOfTheRunsScriptAreAsked(t *testing.T) {
	const gidBeh, gidBehFina, gidBehInit, gidA, gidB = 1, 2, 3, 4, 5
	glyphs := []fonttest.Glyph{
		{Rune: '\u0628', Advance: 600, HasShape: true},
		{Rune: '\uE001', Advance: 400, HasShape: true},
		{Rune: '\uE002', Advance: 300, HasShape: true},
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
	}
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{gidBeh}, []int{gidBehFina})}},
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{gidBeh}, []int{gidBehInit})}},
		},
		[]fonttest.Feature{{Tag: "fina", Lookups: []int{0}}, {Tag: "init", Lookups: []int{1}}},
		map[string]fonttest.Script{
			"DFLT": {Required: fonttest.NoFeature},
			"latn": {Required: fonttest.NoFeature},
			"arab": fonttest.AllFeatures(2),
		})
	pairs := fonttest.PairPosSubtable([]fonttest.KernPair{{Left: gidBeh, Right: gidBeh, Adjust: -50}})
	gpos := fonttest.GPOSTable(
		[]fonttest.Lookup{{Type: 2, Subtables: [][]byte{pairs}}},
		[]fonttest.Feature{{Tag: "kern", Lookups: []int{0}}},
		map[string]fonttest.Script{
			"DFLT": {Required: fonttest.NoFeature},
			"latn": {Required: fonttest.NoFeature},
			"arab": fonttest.AllFeatures(1),
		})
	f := loadNeighbourFace(t, glyphs, map[string][]byte{"GSUB": gsub, "GPOS": gpos})
	if !f.HasJoiningForms() || !f.HasKerning() {
		t.Fatal("the fixture was meant to have forms and kerning, under 'arab'")
	}
	checkNeighbours(t, f, []neighbourCase{
		{run: "ab", can: false, forms: false},
		{run: "\u0628", can: true, forms: true, changes: true},
		// A run whose script is its neighbour's is answered for whichever
		// script that is, and one of this face's has forms.
		{run: "1", can: true, forms: true},
	})

	// The same forms with the kerning moved to 'latn': the Latin run's pairs
	// across its edge are now the font's, and its forms still are not.
	gpos = fonttest.GPOSTable(
		[]fonttest.Lookup{{Type: 2, Subtables: [][]byte{
			fonttest.PairPosSubtable([]fonttest.KernPair{{Left: gidB, Right: gidA, Adjust: -80}})}}},
		[]fonttest.Feature{{Tag: "kern", Lookups: []int{0}}},
		map[string]fonttest.Script{
			"DFLT": {Required: fonttest.NoFeature},
			"latn": fonttest.AllFeatures(1),
		})
	f = loadNeighbourFace(t, glyphs, map[string][]byte{"GSUB": gsub, "GPOS": gpos})
	checkNeighbours(t, f, []neighbourCase{
		{run: "ab", can: true, forms: false, changes: true},
		{run: "\u0628", can: true, forms: true, changes: true},
	})

	// And turned off by the document, the pairs are no reason.
	if f.ContextCanChange("ab", Features{NoKerning: true}) {
		t.Error(`with the kerning turned off, "ab" is answered as though its pairs could change`)
	}
}

// TestTheIndicWordInitialFormIsTheNeighbours: a pre-base vowel sign opening a
// word is drawn with 'init', and whether the syllable opens a word is what
// precedes the run. A face with 'init' under 'dev2' has a Devanagari run whose
// form is its neighbour's to choose; the same face without it does not.
func TestTheIndicWordInitialFormIsTheNeighbours(t *testing.T) {
	const gidKa, gidI, gidIInit = 1, 2, 3
	glyphs := []fonttest.Glyph{
		{Rune: '\u0915', Advance: 600, HasShape: true},
		{Rune: '\u093F', Advance: 300, HasShape: true},
		{Rune: '\uE003', Advance: 250, HasShape: true},
	}
	face := func(tag string) *Face {
		return loadNeighbourFace(t, glyphs, map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
					fonttest.SingleSubst([]int{gidI}, []int{gidIInit})}}},
				[]fonttest.Feature{{Tag: tag, Lookups: []int{0}}},
				map[string]fonttest.Script{
					"DFLT": {Required: fonttest.NoFeature},
					"dev2": fonttest.AllFeatures(1),
				}),
		})
	}
	checkNeighbours(t, face("init"), []neighbourCase{
		{run: "\u0915\u093F", can: true, forms: true, changes: true},
	})
	checkNeighbours(t, face("pres"), []neighbourCase{
		{run: "\u0915\u093F", can: false, forms: false},
	})
}

// TestAUniversalRunOfNoCursiveScriptChoosesNoForm: the universal engine
// chooses joining forms only for a run with a letter of a cursive script in
// it. A Balinese face stating 'fina' has no form a neighbour could choose,
// however the plan names the feature.
func TestAUniversalRunOfNoCursiveScriptChoosesNoForm(t *testing.T) {
	const gidKa, gidKaFina = 1, 2
	f := loadNeighbourFace(t, []fonttest.Glyph{
		{Rune: '\u1B13', Advance: 600, HasShape: true},
		{Rune: '\uE004', Advance: 400, HasShape: true},
	}, map[string][]byte{
		"GSUB": fonttest.GSUBTable(
			[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
				fonttest.SingleSubst([]int{gidKa}, []int{gidKaFina})}}},
			[]fonttest.Feature{{Tag: "fina", Lookups: []int{0}}},
			map[string]fonttest.Script{
				"DFLT": {Required: fonttest.NoFeature},
				"bali": fonttest.AllFeatures(1),
			}),
	})
	checkNeighbours(t, f, []neighbourCase{
		{run: "\u1B13", can: false, forms: false},
	})
}

// TestAFaceSetByCodeReadsNoContext: the standard faces and the simple ones
// apply no rule at all, so nothing beside a run can change it.
func TestAFaceSetByCodeReadsNoContext(t *testing.T) {
	f, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"AV", ":", " a"} {
		if f.ContextCanChange(s, Features{}) || f.FormsFollowNeighbours(s, Features{}) {
			t.Errorf("Helvetica is answered as reading the context of %q", s)
		}
		if changes, how := contextChanges(f, s); changes {
			t.Errorf("Helvetica's %q is changed by %s", s, how)
		}
	}
}
