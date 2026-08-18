package shape

import (
	"os"
	"path/filepath"
	"testing"
)

// Measuring a character the face has no glyph for and does have the pieces of.
//
// The shaper already answers this: normalize takes a character the face cannot
// set and emits its canonical decomposition instead, because the whole of that
// file's rule is "compose where the face has the composed character, decompose
// where it does not". Measure did not, and stopped at .notdef — so a layout
// engine filled a line to one width and the page was drawn at another, which is
// the disagreement MeasureShaped's own comment exists to warn about.
//
// The case is not exotic. Unicode states U+2000 EN QUAD as a singleton
// decomposition of U+2002 EN SPACE, and U+2001 EM QUAD of U+2003 EM SPACE, and a
// font that carries the spaces without the quads is ordinary. The suite's Ahem
// is one: it sets an en quad as half an em, and this measured it as a whole one.

// ahem loads the Web Platform Tests' Ahem, which is the font the case was found
// with and the one the suite is largely written in.
func ahem(t *testing.T) *Face {
	t.Helper()
	root := os.Getenv("WPT_TESTS")
	if root == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) for a font with the quads missing")
	}
	data, err := os.ReadFile(filepath.Join(root, "fonts", "Ahem.ttf"))
	if err != nil {
		t.Skipf("Ahem is not in the checkout: %v", err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Ahem did not load: %v", err)
	}
	return f
}

// TestMeasureAgreesWithShapingOnACharacterTheFaceLacks is the property, and it
// is stated as an agreement rather than as a number: what a caller measures has
// to be what the same caller draws, whatever the two happen to come to.
func TestMeasureAgreesWithShapingOnACharacterTheFaceLacks(t *testing.T) {
	f := ahem(t)
	for _, tc := range []struct{ what, text string }{
		{"an en quad, which Ahem has as an en space", " "},
		{"an em quad, which Ahem has as an em space", " "},
		{"three en quads", "   "},
		{"a quad between letters", "X X"},
		// The characters the face does have, which must not move.
		{"an en space", " "},
		{"an ordinary space", " "},
		{"a letter", "X"},
		{"a word", "XXXXX"},
	} {
		measured := f.Measure(tc.text, 16)
		shaped := f.MeasureShaped(tc.text, 16)
		if measured != shaped {
			t.Errorf("%s: measured %v and shaped %v", tc.what, measured, shaped)
		}
	}
}

// TestAQuadIsMeasuredAsTheSpaceItDecomposesTo is the same fact as a number, so
// that a change making both wrong in the same way is still caught.
func TestAQuadIsMeasuredAsTheSpaceItDecomposesTo(t *testing.T) {
	f := ahem(t)
	// Ahem is designed with every glyph one em, and its en space is the
	// exception at half of one — which is what makes it able to tell these
	// apart at all.
	if got, want := f.Measure(" ", 16), 8.0; got != want {
		t.Fatalf("Ahem's en space measured %v, want %v; the fixture cannot tell a "+
			"half em from a whole one", got, want)
	}
	if got, want := f.Measure(" ", 16), 8.0; got != want {
		t.Errorf("an en quad measured %v, want %v — the width of the en space it "+
			"decomposes to, not of .notdef", got, want)
	}
	if got, want := f.Measure("   ", 16), 24.0; got != want {
		t.Errorf("three en quads measured %v, want %v", got, want)
	}
}

// TestACharacterWithNoDecompositionIsStillNotdef is the containment case: the
// rule is about characters whose pieces the face has, and a character with no
// pieces at all is measured as what will be drawn for it.
func TestACharacterWithNoDecompositionIsStillNotdef(t *testing.T) {
	f := ahem(t)
	// U+4E00, a Han ideograph: Ahem has no glyph and Unicode gives it no
	// decomposition, so there is nothing to take apart.
	notdef, _ := f.Advance(0)
	if got := f.Measure("一", 16); got != notdef*16/1000 {
		t.Errorf("an undecomposable character measured %v, want .notdef's %v",
			got, notdef*16/1000)
	}
}

// TestAPartialDecompositionIsRefused. A decomposition is taken only when the
// face has *every* part: measuring half of one and drawing the whole as .notdef
// is the disagreement this exists to prevent, and it would be a subtler one than
// the case it fixes.
func TestAPartialDecompositionIsRefused(t *testing.T) {
	f := ahem(t)
	// U+01E2 (Æ with a macron) decomposes to U+00C6 and U+0304. Ahem has the
	// first and not the combining macron, so the pair is refused and the
	// character is measured as .notdef — which is what will be drawn.
	if _, ok := f.Advance(0x0304); ok {
		t.Skip("this Ahem has a combining macron, so the fixture proves nothing")
	}
	notdef, _ := f.Advance(0)
	if got := f.Measure("Ǣ", 16); got != notdef*16/1000 {
		t.Errorf("a character whose decomposition the face only half has measured "+
			"%v, want .notdef's %v", got, notdef*16/1000)
	}
}
