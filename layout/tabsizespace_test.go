package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Which font's space a tab stop is a multiple of.
//
// css-text §8.1: "a <number> represents the measure as a multiple of the space
// character's advance width (U+0020)". It names a *character*, and a character
// is set in the first available font that has a glyph for it — so a family
// declared first and lacking U+0020 does not supply the measure. A font with no
// space in it answers .notdef, whose advance has nothing to do with a space and
// in the suite's own font is much wider.
//
// tab-size-integer-005 is that, and says so in a comment above the @font-face:
// "this font-family does not support <space>, so should not be used to resolve
// tab-size".

// noSpaceSet serves a face with no U+0020 under one name and the standard faces
// under every other, which is the shape tab-size-integer-005 declares.
type noSpaceSet struct {
	noSpace  *shape.Face
	standard FontSet
}

func (n noSpaceSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if family == "nospace" {
		return n.noSpace, true
	}
	return n.standard.Face(family, bold, italic)
}

func loadNoSpace(t *testing.T) noSpaceSet {
	t.Helper()
	dir := os.Getenv(wptEnv)
	if dir == "" {
		t.Skipf("set %s for a face that has no space in it", wptEnv)
	}
	data, err := os.ReadFile(filepath.Join(dir, "fonts", "CanvasTest-nospace.ttf"))
	if err != nil {
		t.Skipf("no CanvasTest-nospace.ttf: %v", err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("loading CanvasTest-nospace: %v", err)
	}
	if _, covered := face.GlyphID(' '); covered {
		t.Fatalf("CanvasTest-nospace has a space in it, so this test measures nothing")
	}
	return noSpaceSet{noSpace: face, standard: StandardFonts()}
}

// tabbedX is where the character after a tab lands, given a font-family list.
func tabbedX(t *testing.T, set FontSet, families string) style.Unit {
	t.Helper()
	built := Build(Input{
		HTML: "<div id=\"p\">\tE</div>",
		CSS: []Stylesheet{{Source: noDefaults +
			`#p { font-family: ` + families + `; font-size: 40px; white-space: pre;
			      tab-size: 8 }`}},
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(2000)
	h, _ := style.FromPx(10000)
	root := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	for _, op := range Paint(root) {
		if d, ok := op.(DrawText); ok && d.Text == "E" {
			return d.At.X
		}
	}
	t.Fatalf("no \"E\" was drawn for %q", families)
	return 0
}

// TestATabStopIsMeasuredInTheFaceThatHasTheSpace is the rule.
//
// The two documents name the same face for the space — the second family in one
// and the only family in the other — so their tab stops are the same distance.
// Measuring in the first family instead gives the .notdef advance of a font that
// has no space at all, and the two disagree.
func TestATabStopIsMeasuredInTheFaceThatHasTheSpace(t *testing.T) {
	set := loadNoSpace(t)
	got := tabbedX(t, set, "nospace, Helvetica")
	want := tabbedX(t, set, "Helvetica")
	if got != want {
		t.Errorf("a tab in \"nospace, Helvetica\" put the next character at %v and "+
			"in \"Helvetica\" at %v; the space is Helvetica's either way, because "+
			"the first family has none", got, want)
	}
}

// TestAFaceWithNoSpaceIsNotTheOneAsked. The containment: this must be a real
// difference and not two ways of measuring the same number, or the test above
// would pass under any implementation.
func TestAFaceWithNoSpaceIsNotTheOneAsked(t *testing.T) {
	set := loadNoSpace(t)
	if got := tabbedX(t, set, "nospace"); got == tabbedX(t, set, "Helvetica") {
		t.Skip("the face with no space measures a space exactly as Helvetica does, " +
			"so nothing here can tell the two apart")
	}
}

// TestTheFamiliesThisReachesAreTheOnesTheDocumentNamed. faceWithGlyph does not
// reach past the document's own families into the fallback set: the question is
// which of *those* sets the character, and a family list that names nothing with
// a space leaves the answer to fontFor, exactly as before.
func TestTheFamiliesThisReachesAreTheOnesTheDocumentNamed(t *testing.T) {
	set := loadNoSpace(t)
	l := &layouter{fontSet: set, fonts: map[fontKey]resolvedFont{}}
	b := &Box{Style: style.ComputedStyle{"font-family": "nospace"}}
	if _, ok := l.faceWithGlyph(b, ' '); ok {
		t.Error("a family list of one face with no space answered with a face; the " +
			"fallback set is not this function's business")
	}
	b = &Box{Style: style.ComputedStyle{"font-family": "nospace, Helvetica"}}
	if _, ok := l.faceWithGlyph(b, ' '); !ok {
		t.Error("\"nospace, Helvetica\" answered with nothing; Helvetica has a space")
	}
}
