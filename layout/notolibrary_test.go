package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// Getting at the fallback font library from a test, once.
//
// Two things used to be written out at every call site, and both were wrong in
// the same direction — towards reporting success having checked nothing.
//
// The first is where the library is. `dir := os.Getenv("NOTO_FONTS"); if dir ==
// "" { t.Skip(...) }`, about thirty times, skips on a path that is set and
// wrong; fonttest.NotoDir is that decision made properly and made once, and the
// helpers here are its callers.
//
// The second is what to do when the library is there and empty. `if faces :=
// notoFaces(); len(faces) > 0 { ... }` runs the body when there are faces and
// says nothing at all when there are none, so a library that stopped loading
// would take a dozen assertions with it and no run would say so.
//
// They also load it once rather than per test. notoFaces parses fourteen faces
// of a few megabytes apiece; fontSetForWPT already holds the parsed result
// behind a sync.Once, and calling notoFaces again returns a second copy of the
// same fonts at the same price — which two of the block-glyph cases were doing,
// twice each.

// fallbackLibrary is the fallback faces the harness lends the engine.
//
// It skips a checkout that never fetched them, fails a NOTO_FONTS that names a
// directory without them, and fails a library that is there and loaded nothing
// — the three being different situations that a bare `len(faces) == 0` cannot
// tell apart.
func fallbackLibrary(t *testing.T) []*shape.Face {
	t.Helper()
	fonttest.NotoDir(t)
	faces := fallbackFacesInUse()
	if len(faces) == 0 {
		t.Fatalf("the font library is in place and not one of its %d faces "+
			"loaded: %s", len(notoFaceNames),
			strings.Join(missingFallbackFaces.list(), "; "))
	}
	return faces
}

// fallbackFontSet is the standard faces with the fallback library behind them,
// which is what a caller supplies and what makes a fallback possible at all.
func fallbackFontSet(t *testing.T) FontSet {
	t.Helper()
	fallbackLibrary(t)
	return fontSetForWPT()
}

// TestTheHarnessLoadsEveryFallbackFaceItNames turns notoFaces' record into a
// failure.
//
// notoFaces has no *testing.T and cannot fail: the reftest run has to go ahead
// with whatever loaded, and the ratchet reports what was missing when the count
// drops. But a face that is absent and costs nothing measurable is a face that
// silently stops covering its script, so somewhere it has to be an error, and
// here is where.
func TestTheHarnessLoadsEveryFallbackFaceItNames(t *testing.T) {
	faces := fallbackLibrary(t)
	if len(faces) != len(notoFaceNames) {
		t.Errorf("the harness lent the engine %d of the %d faces it names: %s\n"+
			"A face that is not loaded changes what every document holding "+
			"text it covers is set in.",
			len(faces), len(notoFaceNames),
			strings.Join(missingFallbackFaces.list(), "; "))
	}
}

// kerningFallbackFace is a fallback face that carries pair kerning.
//
// It fails rather than skipping when none of the library does. shape's
// TestKerningIsApplied pins that Noto Sans kerns, so a library where nothing
// kerns is not the library `make noto-fonts` fetches — and the two tests that
// want one are about what happens *with* kerning, so skipping them leaves the
// narrowing they check unchecked in both directions.
func kerningFallbackFace(t *testing.T) *shape.Face {
	t.Helper()
	for _, f := range fallbackLibrary(t) {
		if f.HasKerning() {
			return f
		}
	}
	t.Fatalf("none of the %d fallback faces carries pair kerning, and Noto "+
		"Sans does", len(notoFaceNames))
	return nil
}
