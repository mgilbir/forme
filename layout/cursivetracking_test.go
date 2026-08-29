package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// CSS Text §8.2's cursive tracking, over a document.
//
// paragraph/cursivetracking_test.go states the rule over the text. This is what
// reaches the page, and it is the suite's own shape: letter-spacing-cursive-001
// sets the same Arabic word twice, once under a letter-spacing, and asks for the
// two to be identical.

// arabicRuns lays a document out in the fallback faces and returns the runs of
// the first line of one element.
func arabicRuns(t *testing.T, faces []*shape.Face, htmlSrc, cssSrc string) []TextRun {
	t.Helper()
	return runsWithFonts(t, suiteFonts{standard: StandardFonts(), fallback: faces},
		htmlSrc, cssSrc)
}

// runsWithFonts is the same over a font library the caller chose.
func runsWithFonts(t *testing.T, set FontSet, htmlSrc, cssSrc string) []TextRun {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(1000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	return runsOf(t, frag, "d")
}

// everyFace is a font library of one face, which is how a fixture asks what
// happens when one face answers for text the fallback stack would otherwise
// split between two.
type everyFace struct{ f *shape.Face }

func (o everyFace) Face(string, bool, bool) (*shape.Face, bool)    { return o.f, true }
func (o everyFace) FaceFor(string, bool, bool) (*shape.Face, bool) { return o.f, true }

const arabicWord = "مرحباً"

// TestALetterSpacingDoesNotWidenACursiveWord is the bug, and it is the suite's
// assertion: the same Arabic word with and without a letter-spacing occupies the
// same width.
func TestALetterSpacingDoesNotWidenACursiveWord(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, arabicWord); !ok {
		t.Skip("no face here can set the fixture")
	}
	const css = `#d { font-family: serif; font-size: 24px }`
	plain := runsWidth(arabicRuns(t, faces, `<div id="d" lang="ar">`+arabicWord+`</div>`, css))
	spaced := runsWidth(arabicRuns(t, faces,
		`<div id="d" lang="ar">`+arabicWord+`</div>`,
		css+` #d { letter-spacing: 10px }`))
	if spaced != plain {
		t.Errorf("%q is %v wide under \"letter-spacing: 10px\" and %v without one; "+
			"§8.2 adds none inside a cursive script", arabicWord, spaced, plain)
	}
	// And the run is drawn without one, which is the other half: a width that
	// agrees and a display list that does not would put the letters back apart.
	for _, r := range arabicRuns(t, faces, `<div id="d" lang="ar">`+arabicWord+`</div>`,
		css+` #d { letter-spacing: 10px }`) {
		if r.LetterSpacing != 0 {
			t.Errorf("the run %q is drawn with %v of letter-spacing", r.Text, r.LetterSpacing)
		}
	}
}

// TestALetterSpacingStillWidensWhatIsBesideACursiveWord, which is
// letter-spacing-cursive-002's arithmetic: two Arabic words and a space under
// "letter-spacing: 1em" get one em and not two.
func TestALetterSpacingStillWidensWhatIsBesideACursiveWord(t *testing.T) {
	faces := kernFaces(t)
	const text = "الإبداع المتجدد"
	if _, ok := faceWithGlyphFor(faces, "الإبداع"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const css = `#d { font-family: serif; font-size: 24px }`
	plain := runsWidth(arabicRuns(t, faces, `<div id="d" lang="ar">`+text+`</div>`, css))
	spaced := runsWidth(arabicRuns(t, faces, `<div id="d" lang="ar">`+text+`</div>`,
		css+` #d { letter-spacing: 24px }`))
	if got, want := spaced.Sub(plain), bgpx(24); got != want {
		t.Errorf("a letter-spacing of an em widened two Arabic words and a space by "+
			"%v, want %v — the space takes one and neither word takes any", got, want)
	}
}

// TestACursiveWordSplitAcrossElementsTakesNoSpacingEither is
// letter-spacing-ligatures-005: one letter per <span>, and the boundaries
// between them are inside a word.
func TestACursiveWordSplitAcrossElementsTakesNoSpacingEither(t *testing.T) {
	faces := kernFaces(t)
	const text = "تفاحة"
	if _, ok := faceWithGlyphFor(faces, text); !ok {
		t.Skip("no face here can set the fixture")
	}
	const css = `#d { font-family: serif; font-size: 50px; letter-spacing: 10px }`
	whole := runsWidth(arabicRuns(t, faces, `<div id="d" lang="ar">`+text+`</div>`, css))
	split := runsWidth(arabicRuns(t, faces,
		`<div id="d" lang="ar"><span>ت</span><span>ف</span><span>ا</span><span>ح</span>ة</div>`,
		css))
	if split != whole {
		t.Errorf("the word is %v wide in five elements and %v in one; an element "+
			"boundary inside a cursive word is not a place for a spacing either",
			split, whole)
	}
}

// TestARunThatMixesScriptsIsCutSoTheDisplayListCanSayIt.
//
// A display list carries one letter-spacing per run — an advance added after
// every glyph — so a run holding an Arabic letter beside a Latin one cannot say
// that only one of them is followed by a gap. The cut is what makes the drawing
// able to state the rule at all.
//
// The fixture takes two goes to be about the cut at all. It is one face
// covering both scripts, because the fallback stack cuts a run wherever the face
// changes — "abمرحبا" in a library of single-script faces is already two runs.
// And the cursive script is *Mongolian*, not Arabic, because §3.4's bidi
// resolution cuts a run wherever the embedding level changes, and every cursive
// script but two is written right to left. Mongolian and Phags-pa are the two,
// so a Latin word beside a Mongolian one is one face, one level, and one run —
// and is the only shape in which this cut is the thing that separates them.
func TestARunThatMixesScriptsIsCutSoTheDisplayListCanSayIt(t *testing.T) {
	face := oneFaceFor(t, "Unifont-Regular.otf")
	runs := runsWithFonts(t, everyFace{face}, `<div id="d">abᠠᠡᠮ</div>`,
		`#d { font-family: serif; font-size: 24px; letter-spacing: 10px }`)
	mixed := 0
	for _, r := range runs {
		spaced, plain := 0, 0
		for _, c := range r.Text {
			if isCursiveScript(c) {
				spaced++
			} else {
				plain++
			}
		}
		if spaced > 0 && plain > 0 {
			t.Errorf("the run %q mixes a cursive script with another, and carries "+
				"one letter-spacing for both", r.Text)
		}
		if spaced > 0 && r.LetterSpacing != 0 {
			t.Errorf("the cursive run %q is drawn with %v of letter-spacing",
				r.Text, r.LetterSpacing)
		}
		if plain > 0 && r.LetterSpacing == 0 {
			t.Errorf("the run %q is not cursive and is drawn with no letter-spacing",
				r.Text)
		}
		if spaced > 0 || plain > 0 {
			mixed++
		}
	}
	if mixed < 2 {
		t.Errorf("the text came out as %d run(s); one face sets all of it, so the "+
			"cut is the only thing that could have separated the two scripts", mixed)
	}
}

// oneFaceFor loads a single fallback face by name, so that a fixture can ask
// what happens when one face answers for text the stack would otherwise split.
func oneFaceFor(t *testing.T, name string) *shape.Face {
	t.Helper()
	dir := os.Getenv(notoEnv)
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a pan-Unicode face")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Skipf("%s is not in this checkout: %v", name, err)
	}
	face, err := loadSuiteFace(data)
	if err != nil {
		t.Fatalf("%s did not load: %v", name, err)
	}
	return face
}

// TestAZeroLetterSpacingStillTakesTheBoundarysValue is the containment case, and
// the one this change broke on the way in.
//
// §8.2's boundary rule gives the spacing between two characters to their nearest
// common ancestor, so a run inside "letter-spacing: 0" still takes the enclosing
// paragraph's value at its far edge. That is a different question from "does
// this run take any spacing at all", and answering the two with one predicate
// cost letter-spacing-203 and -206.
func TestAZeroLetterSpacingStillTakesTheBoundarysValue(t *testing.T) {
	root := layoutOf(t, 1000,
		`<div id="d"><span class="z">AAA</span><span class="z">BBB</span></div>`,
		noDefaults+`#d { font-family: Courier; font-size: 100px; letter-spacing: 100px } `+
			`.z { letter-spacing: 0 }`)
	// Six Courier characters at 100px, and one spacing at the one boundary the
	// div owns. Nothing else on the line takes any.
	want := bgpx(6*ch + 100)
	if got := lineWidth(linesOf(t, root, "d")[0]); got != want {
		t.Errorf("the line is %v wide, want %v — six characters and the one "+
			"boundary between the two spans, which the div owns", got, want)
	}
}

// TestACursiveWordShrinkWrapsToTheSameWidthEitherWay is the intrinsic half, and
// the one the suite's fixture actually measures: letter-spacing-cursive-001
// draws an outline around an inline-block, so what is compared is the *box* and
// not where the letters sit.
//
// It has its own test because the intrinsic pass keeps its own account of the
// spacing after a run's last character — it takes that one off again, since a
// spacing at the end of a line hangs — and an account that added nothing and
// subtracted one made the box a spacing *narrower* than the plain word. The
// letters were right and the box was ten pixels short.
func TestACursiveWordShrinkWrapsToTheSameWidthEitherWay(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, arabicWord); !ok {
		t.Skip("no face here can set the fixture")
	}
	width := func(extra string) style.Unit {
		built := Build(Input{
			HTML: `<div id="d" lang="ar"><span>` + arabicWord + `</span></div>`,
			CSS: []Stylesheet{{Source: noDefaults +
				`#d { font-family: serif; font-size: 24px; display: inline-block; ` +
				`white-space: nowrap; ` + extra + ` }`}}})
		w, _ := style.FromPx(1000)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h},
			suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
		return find(t, frag, "d").BorderRect.W
	}
	plain, spaced := width(""), width("letter-spacing: 10px")
	if spaced != plain {
		t.Errorf("the box is %v wide under \"letter-spacing: 10px\" and %v without "+
			"one; §8.2 adds none inside a cursive script, so it takes none away "+
			"either", spaced, plain)
	}
}
