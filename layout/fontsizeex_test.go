package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The layout half of "font-size: 6ex": the cascade's Metrics wired to the font
// set the document is laid out in.
//
// Ahem is the face to assert against, because its metrics are exact by design —
// x-height 800 out of 1000 units, so eight tenths of an em — and because
// CSS2/values/numbers-units-012 is written against it: 6ex on a 20px parent is
// six sixteenths of an inch, which the test draws beside an inch.

// sizedBox lays a document out against a font set that has Ahem in it, with the
// set given to Build as well as to Layout — the cascade computes font-size, so
// a set handed only to Layout arrives too late to be asked.
func sizedBox(t *testing.T, set FontSet, htmlSrc, cssSrc, id string) *Fragment {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}, Fonts: set})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	return find(t, Layout(built.Root, Size{W: w, H: h}, set, rec), id)
}

const exCSS = `#parent { font-family: Ahem; font-size: 20px; line-height: 1 }
	#child { font-size: 6ex }
	#box { width: 1em; height: 1em; background: black }
	#inch { width: 1in; height: 1in; background: black }`

const exHTML = `<div id="parent"><div id="child">
	<div id="box"></div><div id="inch"></div></div></div>`

// TestAnExFontSizeIsSixOfTheFacesXHeights is the bug, in the shape the suite
// draws it: a box of one em against a box of one inch.
func TestAnExFontSizeIsSixOfTheFacesXHeights(t *testing.T) {
	set := loadAhem(t)
	box := sizedBox(t, set, exHTML, exCSS, "box")
	inch := sizedBox(t, set, exHTML, exCSS, "inch")
	if got, want := box.BorderRect.W, inch.BorderRect.W; got != want {
		t.Errorf("one em of the ex-sized element is %v and an inch is %v; Ahem's "+
			"x-height is 0.8em, so 6ex of 20px is 96px", got, want)
	}
	if got := box.BorderRect.W.Px(); got != 96 {
		t.Errorf("the em box is %gpx wide, want 96", got)
	}
}

// TestTheSizeReachesEveryLengthOnTheElement. The font-size is what every em on
// the element is measured against, so getting it wrong is not one wrong box —
// it is every length the element and its descendants declare.
func TestTheSizeReachesEveryLengthOnTheElement(t *testing.T) {
	set := loadAhem(t)
	f := sizedBox(t, set, exHTML, exCSS+` #box { margin-left: 0.5em }`, "box")
	if got := f.Margin.Left.Px(); got != 48 {
		t.Errorf("half an em of the ex-sized element is %gpx, want 48", got)
	}
}

// TestAFamilyNobodyHasStillGetsHalfAnEm is the containment argument, and it
// pins a deliberate decision: faceForStyle does not fall back the way layout's
// own font choice does. A size resolved against a substituted face is a size the
// author never asked for, and §5.1.1 already says what to assume when no
// x-height can be determined.
//
// The fourteen standard faces all state one — Courier 426, Times 450,
// Helvetica 523 out of 1000 — so this needs a family that is genuinely absent
// rather than one that merely has nothing to say.
func TestAFamilyNobodyHasStillGetsHalfAnEm(t *testing.T) {
	f := sizedBox(t, StandardFonts(),
		`<div id="parent"><div id="child"><div id="box"></div></div></div>`,
		`#parent { font-family: NoSuchFamily; font-size: 20px }
		#child { font-size: 6ex }
		#box { width: 1em; height: 1em }`, "box")
	if got := f.BorderRect.W.Px(); got != 60 {
		t.Errorf("the em box is %gpx wide, want 60 — six halves of the parent's "+
			"20px, which is what CSS says to assume when no x-height can be "+
			"determined", got)
	}
}

// TestAStandardFaceIsAskedToo. The other side of the same decision: a family
// the set really has is asked, and every one of the standard fourteen answers.
// Courier states 426 out of 1000, so 6ex of 20px is 51.09px and not 60.
func TestAStandardFaceIsAskedToo(t *testing.T) {
	f := sizedBox(t, StandardFonts(),
		`<div id="parent"><div id="child"><div id="box"></div></div></div>`,
		`#parent { font-family: Courier; font-size: 20px }
		#child { font-size: 6ex }
		#box { width: 1em; height: 1em }`, "box")
	if got := f.BorderRect.W.Px(); got == 60 {
		t.Errorf("the em box is %gpx wide, which is the half-em fallback; Courier "+
			"states an x-height and it must be the one used", got)
	}
}
