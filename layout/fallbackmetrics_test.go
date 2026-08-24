package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// How tall a run is when the font that sets it is not the font the document
// asked for, CSS 2.1 §10.8.1.
//
// The height of a non-replaced inline box is "based on the font", and a run the
// first available font could not set is not in that font — the fallback stack
// found another. Measured against the declared face instead, a Japanese word in
// a paragraph declared in a Latin serif came out the serif's height: the same
// 111.6px at 100px whatever set it, and the line as much too short as the two
// faces differ.

// blockHeight lays out one line in the fallback faces and returns how tall the
// block came out.
func blockHeight(t *testing.T, faces []*shape.Face, text, css string) style.Unit {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">` + text + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-family: serif; font-size: 100px; ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(2000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	return find(t, frag, "d").BorderRect.H
}

// TestARunTakesTheHeightOfTheFaceThatSetsIt is the bug.
func TestARunTakesTheHeightOfTheFaceThatSetsIt(t *testing.T) {
	faces := kernFaces(t)
	cjk, ok := faceWithGlyphFor(faces, "永")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	latin := blockHeight(t, faces, "ab", `line-height: normal`)
	fallback := blockHeight(t, faces, "永永", `line-height: normal`)
	if fallback == latin {
		t.Errorf("a line of ideographs is %v tall and a line of Latin is %v; the "+
			"ideographs are set in another face, and §10.8.1 measures against the "+
			"font the run is in", fallback, latin)
	}
	// And it reaches that face's own height, not merely a different number. It
	// is "at least" because the block's strut is on the line as well — the
	// strut is an inline box like any other and is measured in the face the
	// block declared, so a line is never shorter than it.
	top, bottom, upem, mok := lineMetrics(cjk)
	if !mok {
		t.Skip("this face states no line metrics")
	}
	want, _ := style.FromPx(100 * (top - bottom) / upem)
	if fallback < want {
		t.Errorf("the line is %v tall and the face that sets it comes to %v",
			fallback, want)
	}
}

// TestAMixedLineIsAsTallAsTheTallestFaceOnIt, which is the union §10.8.1 builds:
// every box on the line is aligned to the baseline and the line runs from the
// highest top to the lowest bottom.
func TestAMixedLineIsAsTallAsTheTallestFaceOnIt(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	latin := blockHeight(t, faces, "ab", `line-height: normal`)
	cjk := blockHeight(t, faces, "永永", `line-height: normal`)
	mixed := blockHeight(t, faces, "ab永", `line-height: normal`)
	tallest := latin
	if cjk > tallest {
		tallest = cjk
	}
	if mixed != tallest {
		t.Errorf("a line of Latin and ideographs is %v tall; the Latin alone is %v "+
			"and the ideographs alone %v, and the line is the union", mixed, latin, cjk)
	}
}

// TestADeclaredLineHeightIsNotTheFacesToChange is the containment half, and the
// one this change cost eighteen tests by getting wrong on the way in.
//
// A declared line-height fixes the height whatever the font. The half-leading
// inside it is what is left once the face's own ascent and descent are taken
// out — so measuring a fallback run against its own face gives that run a
// different baseline offset from the rest of its box, and the line grows past
// the height the document asked for. §10.8.1 puts the half-leading on the inline
// *box*, and a run the fallback stack found is not a box of its own.
func TestADeclaredLineHeightIsNotTheFacesToChange(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	for _, tc := range []struct{ what, css string }{
		{"a length", `line-height: 150px`},
		{"a multiple", `line-height: 1.5`},
	} {
		latin := blockHeight(t, faces, "ab", tc.css)
		fallback := blockHeight(t, faces, "永永", tc.css)
		mixed := blockHeight(t, faces, "ab永", tc.css)
		if latin != bgpx(150) {
			t.Fatalf("%s: a line of Latin is %v tall, want 150", tc.what, latin)
		}
		if fallback != latin || mixed != latin {
			t.Errorf("%s: the lines are %v, %v and %v tall for Latin, ideographs "+
				"and both; a declared line-height is not the font's to change",
				tc.what, latin, fallback, mixed)
		}
	}
}
