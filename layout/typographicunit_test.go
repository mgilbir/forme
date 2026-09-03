package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// §8.2's letter-spacing goes between typographic character units, and a unit is
// a grapheme cluster. Two things have to agree about that: what the engine
// measures a run to, and where the comparison puts the run's marks. A page
// filled to one and drawn to the other is the failure this package has a comment
// about wherever it could happen.

// TestARunIsMeasuredInUnitsAndNotInCodePoints.
//
// What the tracking adds to a box, which is the run's own width less the width
// it has without any: one spacing per unit, with the last hanging past the end
// of the line. Asked as a difference because the glyphs' own advances are not
// what this is about — a combining acute may have an advance of its own in the
// face, and that number would drown the one being measured.
func TestARunIsMeasuredInUnitsAndNotInCodePoints(t *testing.T) {
	for _, c := range []struct {
		what, text string
		added      float64
	}{
		// Two units, two spacings, the last hanging: ten pixels added.
		{"two letters", "ab", 10},
		// One unit however many code points, so one spacing and it hangs:
		// nothing added at all. Counted as runes this would be ten.
		{"a letter with a combining acute", "a\u0301", 0},
		// Two units again. Counted as runes it would be twenty.
		{"a letter with a mark, then a letter", "a\u0301b", 10},
		// Still two: both marks belong to the letter under them.
		{"two marks, then a letter", "a\u0301\u0308b", 10},
	} {
		bare := unitWidth(t, c.text, "0")
		spaced := unitWidth(t, c.text, "10px")
		if got := spaced - bare; got != c.added {
			t.Errorf("%s: ten pixels of tracking added %gpx to the box, want %g — "+
				"one spacing per typographic character unit, and the last of them "+
				"hangs past the end of the line", c.what, got, c.added)
		}
	}
}

func unitWidth(t *testing.T, body, spacing string) float64 {
	t.Helper()
	root := layoutOf(t, 400, `<div id="d">`+body+`</div>`,
		`body { margin: 0 }
	#d { font-family: Courier; font-size: 20px; line-height: 20px;
	     width: min-content; letter-spacing: `+spacing+` }`)
	return find(t, root, "d").BorderRect.W.Px()
}

// TestTheComparisonPutsTheSpacingWhereTheEngineDid.
//
// The other half. The comparison walks a run's glyphs and adds the tracking as
// it goes, so it has to add it after the last glyph of each cluster rather than
// after each glyph — a Thai letter with two marks on it is one unit and three
// glyphs, and a pen that spaced all three would move the marks off the letter.
func TestTheComparisonPutsTheSpacingWhereTheEngineDid(t *testing.T) {
	face := ahemFace(t)
	size, _ := style.FromPx(20)
	spacing, _ := style.FromPx(10)
	for _, c := range []struct {
		what, text string
		spacings   int
	}{
		{"two letters", "ab", 2},
		{"a letter with a mark", "a\u0301", 1},
		{"a letter with a mark, then a letter", "a\u0301b", 2},
		{"two marks on one letter", "a\u0301\u0308", 1},
	} {
		v := DrawText{
			Text: c.text, Face: face, Size: size, CharSpacing: spacing,
			Color: style.RGBA{A: 1},
		}
		glyphs, _ := ShapedGlyphs(v)
		got := 0
		for _, n := range spacingAfterGlyph(v, ShapedText(v), glyphs) {
			got += n
		}
		if got != c.spacings {
			t.Errorf("%s: the comparison adds %d spacings over %d glyphs, want %d "+
				"— one per unit", c.what, got, len(glyphs), c.spacings)
		}
		// And it is the same count the engine measured the run with, which is
		// the invariant that matters: the two are one rule.
		if got != spacedUnits(c.text) {
			t.Errorf("%s: the comparison adds %d spacings and the engine counts %d "+
				"units", c.what, got, spacedUnits(c.text))
		}
	}
}

// TestALigatureIsAsManyUnitsAsItHasLetters.
//
// A shaping cluster and a typographic character unit are two different
// divisions of the same text, and a ligature is where they come apart: "ffi" is
// one glyph and three units, so three spacings fall after that one glyph. A
// comparison that added one spacing per *glyph* would add one, and a page filled
// for three and drawn with one is two spacings out.
//
// §8.2 asks a face not to ligate under a non-zero spacing at all. That is a
// separate rule and this engine does not do it, which is exactly why this one
// has to be right.
func TestALigatureIsAsManyUnitsAsItHasLetters(t *testing.T) {
	face := ligatureFace(t)
	size, _ := style.FromPx(20)
	spacing, _ := style.FromPx(10)
	v := DrawText{
		Text: "ffi", Face: face, Size: size, CharSpacing: spacing,
		Color: style.RGBA{A: 1},
	}
	glyphs, _ := ShapedGlyphs(v)
	if len(glyphs) != 1 {
		t.Skipf("this face set \"ffi\" as %d glyphs rather than a ligature", len(glyphs))
	}
	got := 0
	for _, n := range spacingAfterGlyph(v, ShapedText(v), glyphs) {
		got += n
	}
	if got != 3 {
		t.Errorf("the comparison adds %d spacings after the ligature, want 3 — it "+
			"is one glyph and three typographic character units", got)
	}
	if got != spacedUnits(v.Text) {
		t.Errorf("the comparison adds %d spacings and the engine counts %d units",
			got, spacedUnits(v.Text))
	}
}

// ligatureFace is a face that really ligates, for the one question that needs
// one. The suite ships it for its own ligature tests.
func ligatureFace(t *testing.T) *shape.Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(wptDir(t), "fonts", "Lato-Medium-Liga.ttf"))
	if err != nil {
		t.Skip("the checkout has no Lato-Medium-Liga.ttf")
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Skipf("Lato-Medium-Liga.ttf could not be read: %v", err)
	}
	return face
}
