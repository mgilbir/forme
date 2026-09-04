package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Which boundaries a *glyph* crosses, CSS Text §8.1.
//
// This file is about the third of the three questions shapingcontext.go asks
// about a boundary between two runs, and the three answers are different:
//
//   - A *form* crosses almost everything. §8.1's boundary does not break
//     shaping, so an Arabic letter in a <span> still joins to its neighbours,
//     and it joins across a change of colour.
//   - A *kern pair* crosses a colour but not a font or a size, because a kern is
//     a distance one font states at one size.
//   - A *glyph* crosses neither, and less besides. One glyph is drawn once, in
//     one colour, on one baseline, with one line ruled across it: a ligature
//     spanning a change of any of those would paint half a word wrong.
//
// The suite writes the third question as boundary-shaping-003 through -005,
// where "of<span>f</span>ice" must set the ffi ligature, and as shaping-023 to
// -025 and boundary-shaping-002 and -006, where it must not.

// ligatureFaceSet is the suite's Linux Libertine, which carries the ffi
// ligature this file is about, answering every family.
func ligatureFaceSet(t *testing.T) FontSet {
	t.Helper()
	dir := os.Getenv("WPT_TESTS")
	if dir == "" {
		t.Skip("set WPT_TESTS (or run `make wpt`) for a face with ligatures")
	}
	data, err := os.ReadFile(filepath.Join(dir,
		"css/css-text/boundary-shaping/resources/LinLibertine_Re-4.7.5.woff"))
	if err != nil {
		t.Skipf("reading the ligature face: %v", err)
	}
	face, err := loadSuiteFace(data)
	if err != nil {
		t.Skipf("parsing the ligature face: %v", err)
	}
	if !face.HasLigatures() {
		t.Skip("the face carries no ligatures, so it cannot answer this")
	}
	return oneFace{face}
}

// TestALigatureIsFormedAcrossAnInlineBoundary.
//
// "of<span>f</span>ice" is one word written as three runs, and the face's ffi
// covers a character from two of them. The word must set exactly as "office"
// does: the ligature is drawn by the run holding its first character, and the
// pen reaches the "ce" at the same place either way.
func TestALigatureIsFormedAcrossAnInlineBoundary(t *testing.T) {
	set := ligatureFaceSet(t)
	// The word ends in the same place either way: the ligature took the width
	// of the characters it covers, and the run that lost them takes none.
	wholeEnd := textEndOf(t, set, `<div id="p">office</div>`)
	splitEnd := textEndOf(t, set, `<div id="p">of<span>f</span>ice</div>`)
	// Within a layout unit, because the divided word is measured in three
	// pieces and each is rounded to a sixty-fourth of a pixel where the whole
	// one is rounded once. A ligature that failed to form would be out by the
	// width of the characters it covers — nine pixels at this size — so the
	// tolerance is nowhere near what the test is about.
	if d := wholeEnd.Sub(splitEnd); d > 1 || d < -1 {
		t.Errorf("the word ends at %v written whole and at %v written in three "+
			"runs; the ligature that spans the boundary must be formed either "+
			"way, and must be paid for once", wholeEnd.Px(), splitEnd.Px())
	}
}

// TestEveryCharacterOfAMergedWordIsDrawnOnce is the fault a group finds that a
// neighbour does not, and it is why the merge is over the whole group rather
// than over each pair.
//
// Shaped a neighbour at a time the runs of one word disagree about where a
// ligature begins. Given "of|f|ice" the first run shapes "off" and forms an ff,
// the last shapes "fice" and forms an fi, and the "i" between them belongs to a
// ligature in one reading and to a glyph in the other — so it is drawn by
// neither and falls off the page. The count is what notices: a character that
// nobody draws leaves the word a glyph short.
func TestEveryCharacterOfAMergedWordIsDrawnOnce(t *testing.T) {
	set := ligatureFaceSet(t)
	whole := glyphCountOf(t, set, `<div id="p">office</div>`)
	if whole == 0 {
		t.Fatal("the whole word drew no glyphs")
	}
	for _, tc := range []struct{ markup, what string }{
		{`of<span>f</span>ice`, "one span in the middle of the word"},
		{`o<span>f</span>f<span>i</span>ce`, "two spans, so three boundaries"},
		{`<span>o</span><span>f</span><span>f</span><span>i</span><span>c</span><span>e</span>`,
			"a span around every letter"},
	} {
		if got := glyphCountOf(t, set, `<div id="p">`+tc.markup+`</div>`); got != whole {
			t.Errorf("%s: the word drew %d glyphs where written whole it draws %d; "+
				"every character belongs to exactly one run's glyphs", tc.what, got, whole)
		}
	}
}

// glyphCountOf is how many glyphs a document's text came to, over every run.
func glyphCountOf(t *testing.T, set FontSet, htmlSrc string) int {
	t.Helper()
	root := layoutWithFonts(t, set, htmlSrc, `#p { font-size: 36px }`)
	n := 0
	for _, op := range Paint(root) {
		v, ok := op.(DrawText)
		if !ok || v.Face == nil {
			continue
		}
		glyphs, _ := ShapedGlyphs(v)
		n += len(glyphs)
	}
	return n
}

// TestAGlyphDoesNotCrossAChangeInHowItIsPainted is the other half, and the one
// that keeps this from being a licence to merge anything: each row changes
// something a glyph carries and cannot carry twice, so the ligature must not be
// formed and the word comes out as wide as three unligatured runs.
func TestAGlyphDoesNotCrossAChangeInHowItIsPainted(t *testing.T) {
	set := ligatureFaceSet(t)
	merged := textEndOf(t, set, `<div id="p">of<span>f</span>ice</div>`)
	apart := textEndOf(t, set, `<div id="p">of<span style="padding-left:10px">f</span>ice</div>`)
	if merged == apart {
		t.Fatal("a padded span sets the word to the same width as an unpadded " +
			"one, so this test cannot tell a formed ligature from an unformed one")
	}
	for _, tc := range []struct{ style, what string }{
		{"color: blue", "a colour, which a glyph is drawn in once"},
		{"vertical-align: super", "a raised baseline, which a glyph sits on once"},
		{"text-decoration: underline", "a line ruled across it"},
		{"font-style: italic", "a style the face is chosen from"},
		{"font-weight: bold", "a weight the face is chosen from"},
	} {
		got := textEndOf(t, set, `<div id="p">of<span style="`+tc.style+`">f</span>ice</div>`)
		if got == merged {
			t.Errorf("%s: the word is as wide as the merged one, so the ligature "+
				"was formed across %s", tc.style, tc.what)
		}
	}
}

// textEndOf is the pen position at the end of a document's text: the furthest
// right any run reached. It is what a formed ligature shortens.
func textEndOf(t *testing.T, set FontSet, htmlSrc string) style.Unit {
	t.Helper()
	root := layoutWithFonts(t, set, htmlSrc, `#p { font-size: 36px }`)
	var end style.Unit
	for _, op := range Paint(root) {
		v, ok := op.(DrawText)
		if !ok || v.Face == nil {
			continue
		}
		w, _ := style.FromPx(v.Face.MeasureShapedMerged(v.Text, 36, v.PreContext,
			v.PostContext, v.MergePre, v.MergePost, v.ContextKerns, v.Features))
		if e := v.At.X.Add(w); e > end {
			end = e
		}
	}
	return end
}
