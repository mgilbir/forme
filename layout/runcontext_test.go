package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Whether the text beside a run can change it is the run's question, answered
// from the rules its own script selects. It was asked of the face — did the
// font have positional forms, pair kerning or a 'liga' ligature under any
// script — and three places in layout acted on that answer. Each is held here
// to a font where the face's answer and the run's differ. See
// shape.Face.ContextCanChange.

// hanColonFace states a Chinese form of the fullwidth colon under 'hani' and
// nothing else: no forms, no kerning, no ligature.
func hanColonFace(t *testing.T) *shape.Face {
	t.Helper()
	const gidColon, gidColonHan = 2, 3
	f, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "HanColon",
		Glyphs: []fonttest.Glyph{
			{Rune: '中', Advance: 1000, HasShape: true},
			{Rune: '：', Advance: 1000, HasShape: true},
			{Rune: '', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
					fonttest.SingleSubst([]int{gidColon}, []int{gidColonHan})}}},
				[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}},
				map[string]fonttest.Script{
					"DFLT": {Required: fonttest.NoFeature},
					"hani": fonttest.AllFeatures(1),
				}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestPunctuationBetweenHanIsSetAsHan: a colon in a span of its own colour
// between two Han characters is Han, and takes the face's Chinese form — in
// the glyph drawn and in the room measured for it. The face-wide answer saw a
// font with nothing that joins, kerns or ligates, gave the span no neighbours,
// and the colon was set under 'DFLT', a full em wide.
func TestPunctuationBetweenHanIsSetAsHan(t *testing.T) {
	set := namedFaceSet{family: "Han", face: hanColonFace(t), standard: StandardFonts()}
	const css = `body{margin:0} p{font-family:Han; font-size:20px} span{color:red}`
	frag := laidOutIn(t, set, "<p>中<span>：</span>中</p>", css)
	colon, _ := glyphsDrawnFor(t, frag, "：")
	if len(colon) != 1 || colon[0] != 3 {
		t.Errorf("the colon between two Han characters is drawn as %v, want the "+
			"face's 'hani' form [3]", colon)
	}
	var xs []float64
	for _, op := range Paint(frag) {
		if v, ok := op.(DrawText); ok {
			xs = append(xs, v.At.X.Px())
		}
	}
	// 中 is 20px, the Chinese colon half an em: the second 中 starts at 30.
	if len(xs) != 3 || xs[2] != 30 {
		t.Errorf("the three runs start at %v, want [0 20 30]: the colon is "+
			"measured as the form it is drawn in", xs)
	}
}

// arabicAndLatinFace has Arabic positional forms under 'arab' and an A–V kern
// pair under 'latn' — a face that sets both scripts, as Noto's Arabic faces
// and most Arabic web fonts do. A Latin run in it has no form a neighbour can
// choose, whatever the face has for Arabic.
func arabicAndLatinFace(t *testing.T) *shape.Face {
	t.Helper()
	const gidA, gidV, gidBeh, gidBehFina = 1, 2, 3, 4
	f, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "ArabicAndLatin",
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 600, HasShape: true},
			{Rune: 'V', Advance: 600, HasShape: true},
			{Rune: 'ب', Advance: 600, HasShape: true},
			{Rune: '', Advance: 400, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{
					fonttest.SingleSubst([]int{gidBeh}, []int{gidBehFina})}}},
				[]fonttest.Feature{{Tag: "fina", Lookups: []int{0}}},
				map[string]fonttest.Script{
					"DFLT": {Required: fonttest.NoFeature},
					"latn": {Required: fonttest.NoFeature},
					"arab": fonttest.AllFeatures(1),
				}),
			"GPOS": fonttest.GPOSTable(
				[]fonttest.Lookup{{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable(
					[]fonttest.KernPair{{Left: gidA, Right: gidV, Adjust: -200}})}}},
				[]fonttest.Feature{{Tag: "kern", Lookups: []int{0}}},
				map[string]fonttest.Script{
					"DFLT": {Required: fonttest.NoFeature},
					"latn": fonttest.AllFeatures(1),
				}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !f.HasJoiningForms() {
		t.Fatal("the fixture was meant to have Arabic forms")
	}
	return f
}

// TestALatinRunIsNotKernedAcrossASizeChange: a kern is a distance one font
// states at one size, so a pair across a change of size belongs to neither
// run — unless the two sides join, where the form wins (see sameShaping). The
// face has Arabic forms, and asked of the face that made every run of it
// "joining": "A" at 16px was kerned against a "V" at 32px.
func TestALatinRunIsNotKernedAcrossASizeChange(t *testing.T) {
	face := arabicAndLatinFace(t)
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	size, _ := style.FromPx(16)
	a := paragraph.NewBreaker(nil).Measure(face, "A", size).Px()
	const css = `body{margin:0} p{font-family:T; font-size:16px} .big{font-size:32px}`
	frag := laidOutIn(t, set, `<p><span>A</span><span class="big">V</span></p>`, css)
	var xs []float64
	for _, op := range Paint(frag) {
		if v, ok := op.(DrawText); ok {
			xs = append(xs, v.At.X.Px())
		}
	}
	// A is 600 units, 9.6px at 16px, and nothing is taken off it.
	if len(xs) != 2 || xs[1] != a {
		t.Errorf("the runs begin at %v, want the V at %v — A's own advance. The "+
			"pair is stated at one size and the two letters are at two", xs, a)
	}
}

// TestALatinRunAtALineEndIsUnkernedInAFaceThatAlsoJoins is
// TestAKernAgainstTheNextLineIsNotCharged in a face that also has Arabic
// forms. The correction is skipped for a run whose forms follow its
// neighbours, since taking the context away there changes the letter and not
// only the kern; asked of the face, it was skipped for every Latin line end
// in any font that also sets Arabic.
func TestALatinRunAtALineEndIsUnkernedInAFaceThatAlsoJoins(t *testing.T) {
	face := arabicAndLatinFace(t)
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	size, _ := style.FromPx(16)
	want := paragraph.NewBreaker(nil).Measure(face, "A", size)
	const sheet = `#d { font-family: T; font-size: 16px; overflow-wrap: break-word }`
	for _, markup := range []string{"AV", `<span>A</span><span>V</span>`} {
		t.Run(markup, func(t *testing.T) {
			lines, ok := linesOfSpanned(t, set, markup, sheet, 5)
			if !ok || len(lines) != 2 || lines[0].Text != "A" {
				t.Fatalf("the document set %v, want A and V on two lines", lines)
			}
			if lines[0].Width != want {
				t.Errorf("the line holding the A is %v wide, want %v — its own "+
					"advance; the V it would be kerned against is on the next line",
					lines[0].Width, want)
			}
		})
	}
}
