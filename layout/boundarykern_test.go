package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Kerning across a boundary a *box* makes, and across one this engine's own
// passes make.
//
// shape/boundarykern.go answers the pair once the two runs know about each
// other. These are the two questions layout has to answer for it to be reached
// at all: which runs are given a context, and when.

// kernFaces are the fallback faces, or a skip.
func kernFaces(t *testing.T) []*shape.Face {
	t.Helper()
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face with kern pairs")
	}
	return faces
}

// kernLayout lays a document out in those faces.
func kernLayout(t *testing.T, faces []*shape.Face, htmlSrc, cssSrc string) *Fragment {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(1000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	if frag == nil {
		t.Fatal("layout produced no fragment")
	}
	return frag
}

// runsWidth is what a set of runs comes to, which is what a line is filled
// against and what a box is shrink-wrapped to. lineWidth beside it takes a
// whole line; this takes the runs, which is what the fixtures here compare.
func runsWidth(runs []TextRun) style.Unit {
	var total style.Unit
	for _, r := range runs {
		total = total.Add(r.Width)
	}
	return total
}

// faceWithGlyphFor returns the first of the faces that can set a string, which
// is the one the fallback stack will have chosen.
func faceWithGlyphFor(faces []*shape.Face, s string) (*shape.Face, bool) {
	for _, f := range faces {
		ok := true
		for _, r := range s {
			if _, has := f.GlyphID(r); !has {
				ok = false
				break
			}
		}
		if ok {
			return f, true
		}
	}
	return nil, false
}

// kernOf is what a face takes off a pair, in pixels, at a size.
func kernOf(f *shape.Face, left, right string, size float64) float64 {
	return f.MeasureShaped(left+right, size) -
		f.MeasureShaped(left, size) - f.MeasureShaped(right, size)
}

// TestAPairSplitByAnElementBoundaryIsStillKerned.
//
// §8.1: "the boundary between two inline elements does not break shaping", and a
// pair the font kerns is as much a part of shaping as a cursive form is. The
// gate on the whole context pass used to ask only whether the face had
// positional forms, so every Latin, Greek, Cyrillic and CJK document was waved
// through and a pair split by a <span> came out a kern wider than the same two
// characters written together.
//
// The fixture is Japanese because that is where a kerned face is reachable here.
// The fourteen standard PDF faces carry no kern pairs at all — layout/kerning.go
// has a whole finding resting on it — so the only faces in this harness that
// kern anything are the Noto ones, and those are reached as *fallback*, for text
// the standard faces have no glyph for.
func TestAPairSplitByAnElementBoundaryIsStillKerned(t *testing.T) {
	faces := kernFaces(t)
	const size = 60
	const css = `#d { font-family: monospace; font-size: 60px }`

	face, ok := faceWithGlyphFor(faces, "す。")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	kern := kernOf(face, "す", "。", size)
	if kern == 0 {
		t.Skip("this face does not kern す before a full stop, so the fixture " +
			"asserts nothing")
	}

	joined := runsWidth(runsOf(t, kernLayout(t, faces,
		`<div id="d" lang="ja">す。</div>`, css), "d"))
	split := runsOf(t, kernLayout(t, faces,
		`<div id="d" lang="ja">す<span>。</span></div>`, css), "d")
	if len(split) < 2 {
		t.Fatalf("the span made %d run(s), so there is no boundary in this fixture",
			len(split))
	}
	if got := runsWidth(split); got != joined {
		t.Errorf("\"す<span>。\" is %v wide and \"す。\" is %v; the element boundary "+
			"does not break shaping, so the pair is kerned either way", got, joined)
	}
	// And the pair really is kerned, so the equality above is about the boundary
	// rather than about two numbers the font never made differ.
	if unkerned, _ := style.FromPx(face.MeasureShaped("す", size) +
		face.MeasureShaped("。", size)); joined == unkerned {
		t.Fatalf("\"す。\" is %v wide either way; this face does not kern the fixture",
			joined)
	}
}

// TestABoundaryThatDoesBreakShapingLosesThePair is the containment half, and the
// half that keeps the test above from being satisfied by never splitting a run
// at all.
//
// §8.1 breaks shaping where the two sides differ in what shaping depends on, and
// a different size is one of the four the suite states in shaping-008 through
// -011. Two runs at different sizes are not one pair and must not be kerned as
// one.
func TestABoundaryThatDoesBreakShapingLosesThePair(t *testing.T) {
	faces := kernFaces(t)
	const size = 60
	const css = `#d { font-family: monospace; font-size: 60px } ` +
		`#d span { font-size: 30px }`

	face, ok := faceWithGlyphFor(faces, "す。")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	if kernOf(face, "す", "。", size) == 0 {
		t.Skip("this face does not kern す before a full stop")
	}
	runs := runsOf(t, kernLayout(t, faces,
		`<div id="d" lang="ja">す<span>。</span></div>`, css), "d")
	want, _ := style.FromPx(face.MeasureShaped("す", size))
	if got := runAt(t, runs, "す").Width; got != want {
		t.Errorf("the す is %v wide before a full stop at half its size and %v alone; "+
			"the sizes differ, so the boundary breaks shaping and there is no pair",
			got, want)
	}
}

// TestACharacterCutOutToHangKeepsItsNeighboursKern is the ordering, and the
// reason the context pass runs after §8.4's cut rather than before it.
//
// Every other pass over the items reads them and leaves their text alone. The
// hanging-punctuation cut is the one that *makes* a boundary: it takes the full
// stop off the end of a run and stands it up as a run of its own. Settled before
// the cut, the two halves are never told about each other — and NotoSansJP kerns
// them, so the stop was hung a tenth of an em further right than the font asks
// for. That is the suite's hanging-punctuation-block-bound-001, where the tenth
// of an em is six visible pixels.
func TestACharacterCutOutToHangKeepsItsNeighboursKern(t *testing.T) {
	faces := kernFaces(t)
	const size = 60
	const text = "まだよくています。しかし特"
	// The suite's own fixture: four ideographs to a 240px line at 60px, and a
	// full stop that has to hang off the end of the second line.
	const css = `#d { font-family: monospace; font-size: 60px; line-height: 1.5em; ` +
		`width: 240px; hanging-punctuation: allow-end }`

	face, ok := faceWithGlyphFor(faces, "す。")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	kern := kernOf(face, "す", "。", size)
	if kern == 0 {
		t.Skip("this face does not kern す before a full stop, so the fixture " +
			"asserts nothing")
	}

	lines := linesOf(t, kernLayout(t, faces, `<div id="d" lang="ja">`+text+`</div>`, css), "d")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %v", len(lines), lineTexts(lines))
	}
	stop := runAt(t, lines[1].Runs, "。")
	// Where the four ideographs before it end, which is four ems less whatever
	// the face takes off the pair.
	want, _ := style.FromPx(4*size + kern)
	// One layout unit of slack, and only that: the ideographs are measured one
	// item at a time and rounded one at a time, and the arithmetic here rounds
	// once. A tenth of an em at this size is 384 units, so a unit of slack
	// cannot make a wrong answer look right.
	if diff := stop.X.Sub(want); diff > style.Unit(1) || want.Sub(stop.X) > style.Unit(1) {
		t.Errorf("the full stop hangs at %v; the four ideographs before it are %v "+
			"wide once the pair is kerned, and four whole ems would be %v",
			stop.X, want, bgpx(4*size))
	}
}
