package layout

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/shape"
)

// A Hangul syllable is one shaping cluster, and that hides nothing.
//
// The shaper sets Hangul as HarfBuzz does: a syllable written as jamo is
// composed where the face has it, or kept as jamo and made one cluster where it
// has not, and a tone mark is put in front of its syllable and made one cluster
// with it. So characters that were each their own cluster now share one, and
// the question graphemecluster_test.go's tripwire asks is whether that can hide
// a position this engine needs: a soft wrap opportunity, the place
// letter-spacing goes, or a cut overflow-wrap makes.
//
// It cannot, and the reason is in what gets merged. Every merge the Hangul model
// makes is of characters in one extended grapheme cluster, and inside one
// UAX #14 forbids a break:
//
//   - a leading, a vowel and a trailing jamo: GB6 and GB7 (L × V, V × T), and
//     LB26 (JL × JV, JV × JT);
//   - a syllable of no trailing consonant and the trailing jamo after it: GB8
//     (LV × T), and LB26 (H2 × JT);
//   - a syllable and the tone mark after it: U+302E and U+302F are
//     Other_Grapheme_Extend, so GB9 (× Extend), and line break class CM, so
//     LB9.
//
// HarfBuzz's own name for the operation says as much: merge_out_grapheme_clusters.
// And every position the engine asks about is a grapheme boundary — the breaker
// cuts at package segment's boundaries and nowhere else, letter-spacing goes
// after each grapheme cluster, and the widths of the pieces between are read by
// byte offset off the shaped glyphs (shape.GroupAdvances), which is exact at a
// boundary as long as no cluster straddles it. A cluster that stays inside one
// grapheme cluster straddles none.
//
// What the tests below hold is that, measured rather than argued: every
// grapheme boundary of a Hangul string is still the start of a shaping cluster,
// in a face that composes, one that draws jamo, and one that has no Hangul at
// all; and, end to end, the break and the spacing UAX #14 and CSS Text allow
// between two syllables are still where they were, whether the syllables are
// written composed, as jamo, or one of each.

// hangulFace is a face with the jamo of one syllable, a tone mark and a dotted
// circle, and — where composed is true — the syllables 가 and 각 as well. The
// advances make a syllable 1000 units composed and 1300 as jamo, so a line
// width tells the two apart.
func hangulFace(t *testing.T, composed bool) *shape.Face {
	t.Helper()
	glyphs := []fonttest.Glyph{
		{Rune: 0x1100, Advance: 600, HasShape: true},
		{Rune: 0x1161, Advance: 400, HasShape: true},
		{Rune: 0x11A8, Advance: 300, HasShape: true},
		{Rune: 0x1113, Advance: 610, HasShape: true},
		{Rune: 0x1176, Advance: 410, HasShape: true},
		{Rune: 0x302E, Advance: 200, HasShape: true},
		{Rune: 0x25CC, Advance: 500, HasShape: true},
	}
	name := "HangulJamo"
	if composed {
		name = "HangulSyllables"
		glyphs = append(glyphs,
			fonttest.Glyph{Rune: 0xAC00, Advance: 1000, HasShape: true},
			fonttest.Glyph{Rune: 0xAC01, Advance: 1000, HasShape: true})
	}
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{Name: name, Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}

// The strings, as the characters they are written with.
var (
	hangulLVT     = "\u1100\u1161\u11A8" // 각 as jamo
	hangulLVTLVT  = hangulLVT + hangulLVT
	hangulTwoS    = "\uAC01\uAC01"       // 각각, composed
	hangulMixed   = "\uAC01" + hangulLVT // one of each
	hangulMixed2  = hangulLVT + "\uAC01" // the other way round
	hangulLVandT  = "\uAC00\u11A8\uAC00" // 가 and a trailing jamo, then 가
	hangulTone    = "\uAC00\u302E\uAC00" // a tone mark after a syllable
	hangulJamoT   = "\u1100\u1161\u302E\u1100\u1161"
	hangulLoneT   = "\u302E\uAC00"             // a tone mark after nothing
	hangulLLVV    = "\u1100\u1100\u1161\u1161" // jamo that make no syllable alone
	hangulOldJamo = "\u1113\u1176\uAC00"       // Old Hangul, which composes into nothing
)

func TestAHangulShapingClusterNeverSpansAGraphemeBoundary(t *testing.T) {
	bundled, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the bundled face: %v", err)
	}
	faces := []struct {
		what string
		face *shape.Face
	}{
		{"a face with the syllables", hangulFace(t, true)},
		{"a face with jamo only", hangulFace(t, false)},
		{"a face with no Hangul", bundled},
	}
	for _, f := range faces {
		for _, text := range []string{
			hangulLVT, hangulLVTLVT, hangulTwoS, hangulMixed, hangulLVandT,
			hangulTone, hangulJamoT, hangulLoneT, hangulLLVV, hangulOldJamo,
		} {
			runs, _ := shape.NewStack(f.face).ShapeRuns(text)
			starts := map[int]bool{}
			for _, r := range runs {
				for _, g := range r.Glyphs {
					starts[g.Cluster] = true
				}
			}
			for _, at := range segment.Boundaries(nil, text) {
				if !starts[at] {
					t.Errorf("%s: %+q has a grapheme boundary at byte %d that no "+
						"shaping cluster starts at \u2014 a cluster spans it, so the widths "+
						"either side of a break or a spacing there are misread",
						f.what, text, at)
				}
			}
		}
	}
}

// hangulSet answers the family "H" with a Hangul fixture face.
func hangulSet(t *testing.T, composed bool) FontSet {
	return namedFaceSet{family: "H", face: hangulFace(t, composed), standard: StandardFonts()}
}

// TestABreakBetweenTwoHangulSyllablesIsStillOffered.
//
// Two syllables with nothing between them may be broken between (UAX #14's
// LB31, since LB26 and LB27 hold only inside a syllable), and a box's
// min-content width is the widest piece between two such breaks: one syllable.
// Under keep-all the break is gone and it is both. The widths are read off the
// shaped glyphs by byte offset, so a cluster that took in both syllables would
// charge both to the first and make the first piece two syllables wide.
//
// Every spelling is here: both syllables composed, both in jamo, and one of
// each in either order. Two syllables in jamo once had no break between them
// at all, because the line breaker offered one only around the classes ID, CJ,
// H2 and H3; see paragraph.BreaksLikeAnIdeograph.
func TestABreakBetweenTwoHangulSyllablesIsStillOffered(t *testing.T) {
	for _, c := range []struct {
		what     string
		composed bool
		// syllable is one syllable's width at 20px.
		syllable float64
	}{
		{"a face with the syllables", true, 20},
		{"a face with jamo only", false, 26},
	} {
		for _, text := range []string{hangulTwoS, hangulLVTLVT, hangulMixed, hangulMixed2} {
			for _, wb := range []struct {
				value string
				want  float64
			}{
				{"normal", c.syllable},
				{"keep-all", 2 * c.syllable},
			} {
				frag, _ := layoutWith(t, hangulSet(t, c.composed),
					`<div id="d">`+text+`</div>`,
					`body{margin:0} #d{font-family:H; font-size:20px; line-height:20px;
					 width:min-content; word-break:`+wb.value+`}`)
				if got := find(t, frag, "d").BorderRect.W.Px(); got != wb.want {
					t.Errorf("%s, %+q, word-break: %s: min-content width %gpx, want %g",
						c.what, text, wb.value, got, wb.want)
				}
			}
			// And the break is taken: at a width one syllable fits, two lines,
			// one syllable on each.
			frag, _ := layoutWith(t, hangulSet(t, c.composed),
				`<div id="d">`+text+`</div>`,
				fmt.Sprintf(`body{margin:0} #d{font-family:H; font-size:20px;
				 line-height:20px; width:%gpx}`, c.syllable+4))
			lines := map[float64]string{}
			for _, op := range Paint(frag) {
				if v, ok := op.(DrawText); ok {
					lines[v.At.Y.Px()] += v.Text
				}
			}
			if len(lines) != 2 {
				t.Errorf("%s, %+q: set on %d lines at one syllable's width (%v), want 2",
					c.what, text, len(lines), lines)
			}
		}
	}
}

// TestLetterSpacingFallsAfterEachHangulSyllable.
//
// CSS Text §8.2 puts letter-spacing after each typographic character unit, a
// grapheme cluster, and the comparison puts it after the last glyph of the
// shaping cluster a unit ends in. So each syllable gets one spacing, after its
// own glyphs, whether it is one composed glyph or three jamo.
func TestLetterSpacingFallsAfterEachHangulSyllable(t *testing.T) {
	for _, composed := range []bool{true, false} {
		for _, text := range []string{hangulTwoS, hangulLVTLVT, hangulMixed, hangulTone} {
			frag, _ := layoutWith(t, hangulSet(t, composed),
				`<div id="d">`+text+`</div>`,
				`body{margin:0} #d{font-family:H; font-size:20px; letter-spacing:3px}`)
			// Every run drawn: a line may be drawn as more than one.
			var runs []DrawText
			for _, op := range Paint(frag) {
				if d, ok := op.(DrawText); ok && d.Text != "" {
					runs = append(runs, d)
				}
			}
			if len(runs) == 0 {
				t.Fatalf("%+q: no run was drawn", text)
			}
			for _, v := range runs {
				checkSyllableSpacing(t, composed, text, v)
			}
		}
	}
}

// checkSyllableSpacing holds one drawn run to one spacing per syllable, after
// the syllable's last drawn glyph.
func checkSyllableSpacing(t *testing.T, composed bool, text string, v DrawText) {
	t.Helper()
	shaped := ShapedText(v)
	glyphs, _ := ShapedGlyphs(v)
	after := spacingAfterGlyph(v, shaped, glyphs)
	bounds := append(segment.Boundaries(nil, shaped), len(shaped))
	// syllable is which grapheme cluster a byte offset is in.
	syllable := func(at int) int {
		k := 0
		for k < len(bounds) && bounds[k] <= at {
			k++
		}
		return k
	}
	for k, lo := 0, 0; k < len(bounds); k++ {
		hi := bounds[k]
		n := 0
		for i, g := range glyphs {
			if g.Cluster >= lo && g.Cluster < hi {
				n += after[i]
			}
		}
		if n != 1 {
			t.Errorf("composed=%v, %+q: %d spacings after the glyphs of the "+
				"syllable at bytes %d-%d, want 1", composed, text, n, lo, hi)
		}
		lo = hi
	}
	// And each goes after the syllable, not inside it: the glyph drawn
	// next is another syllable's, or there is none. A tone mark is drawn
	// in front of its syllable, so a spacing after the tone mark's glyph
	// would push the syllable away from its own mark.
	for i := range glyphs {
		if after[i] > 0 && i+1 < len(glyphs) &&
			syllable(glyphs[i+1].Cluster) == syllable(glyphs[i].Cluster) {
			t.Errorf("composed=%v, %+q: a spacing after glyph %d, inside the "+
				"syllable its next glyph is part of", composed, text, i)
		}
	}
}

// TestAJamoSyllableAcrossTwoBoxesIsNotCut.
//
// A syllable whose jamo are in two elements is still one syllable, and a line
// may not end inside it; two syllables in two elements may be broken between.
// The opportunity a jamo offers crosses into the next box, and it is the next
// box's first character that decides whether the syllable ended there.
func TestAJamoSyllableAcrossTwoBoxesIsNotCut(t *testing.T) {
	for _, c := range []struct {
		html  string
		lines int
	}{
		{"<span>\u1100</span>\u1161", 1},
		{"<span>\u1100\u1161</span>\u11A8", 1},
		{"\u1100<span>\u1161\u11A8</span>", 1},
		{"<span>\u1100\u1161\u11A8</span>\u1100\u1161\u11A8", 2},
		{"\u1100\u1161\u11A8<span>\u1100\u1161\u11A8</span>", 2},
		{"a<span>\u1100\u1161\u11A8</span>", 2},
		{"<span>a</span>\u1100\u1161\u11A8", 2},
	} {
		root := layoutOf(t, 600, `<div id="p">`+c.html+`</div>`,
			`#p { font-family: Courier; font-size: 20px; width: 1px }`)
		if got := len(find(t, root, "p").Lines); got != c.lines {
			t.Errorf("%+q in a box one pixel wide: %d line(s), want %d", c.html, got, c.lines)
		}
	}
}
