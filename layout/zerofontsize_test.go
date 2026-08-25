package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// "font-size: 0", which is a real declaration and a common one.
//
// It is how a stylesheet removes the white space between inline-blocks, and the
// suite writes it in white-space-zero-fontsize-001 and -002 and in
// table-vertical-align-baseline-008. Layout read a zero size as a field nobody
// had filled in and put sixteen pixels back, which is invisible in the text —
// there are no glyphs to draw at either size — and visible in the *strut*: every
// line in such a box came out the height of a font nobody asked for.

// zeroFontBox lays out one box and returns it.
func zeroFontBox(t *testing.T, css, inner string) *Fragment {
	t.Helper()
	root := layoutOf(t, 600, `<div id="w">`+inner+`</div>`, noDefaults+`#w { `+css+` }`)
	return find(t, root, "w")
}

// TestAZeroFontSizeIsAZero.
func TestAZeroFontSizeIsAZero(t *testing.T) {
	for _, v := range []string{"0", "0px", "0em", "0.0px"} {
		f := zeroFontBox(t, "font-size: "+v, "x")
		if f.Box.FontSize != 0 {
			t.Errorf("font-size: %s came out as %v", v, f.Box.FontSize)
		}
		if h := f.BorderRect.H.Px(); h != 0 {
			t.Errorf("font-size: %s made a %gpx line; a font of no size has no "+
				"strut", v, h)
		}
	}
}

// TestAnInlineBlockInAZeroFontSizeBoxHasNoStrutUnderIt is the shape the suite
// writes and the reason the declaration is used at all: the strut is what puts
// space under a row of inline-blocks, and setting the font to nothing is how an
// author removes it.
func TestAnInlineBlockInAZeroFontSizeBoxHasNoStrutUnderIt(t *testing.T) {
	const ib = `<div style="display: inline-block; width: 50px; height: 100px"></div>`
	f := zeroFontBox(t, "float: left; font-size: 0", ib)
	if got := f.BorderRect.H.Px(); got != 100 {
		t.Errorf("the box is %gpx tall around a 100px inline-block; at font-size 0 "+
			"the strut adds nothing under it", got)
	}
	// And with a font size it does, which is what says the fixture is about the
	// zero rather than about inline-blocks.
	if got := zeroFontBox(t, "float: left; font-size: 16px", ib).BorderRect.H.Px(); got <= 100 {
		t.Errorf("at 16px the same box is %gpx tall; the strut's descent is what "+
			"the zero removes", got)
	}
}

// TestABoxNobodyGaveAFontSizeStillGetsOne is the containment half, and the case
// ensureFontSize exists for: a box tree a caller assembled itself has no cascade
// behind it, and an "em" in one of its declarations has to resolve against
// something.
func TestABoxNobodyGaveAFontSizeStillGetsOne(t *testing.T) {
	b := &Box{Outer: OuterBlock, Inner: InnerFlow, Style: style.ComputedStyle{
		"height": "2em",
	}}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(b, Size{W: w, H: h}, nil, rec)
	if frag == nil {
		t.Fatal("the hand-built box produced no fragment")
	}
	if got := frag.BorderRect.H.Px(); got != 32 {
		t.Errorf("a hand-built box at \"height: 2em\" is %gpx tall, want 32 — nothing "+
			"decided its font size, so the em resolves against the default", got)
	}
}

// TestABoxThatTakesItsParentsSizeTakesItsParentsAnswer.
//
// Four boxes are made with a parent's font size copied onto them, and every one
// copied the number and not whether it was decided — so a zero the document
// asked for became sixteen the moment the box was wrapped in anything, and a
// "font-size: 0" block with a block child among its inline ones got its strut
// back.
//
// This asks the one of the four that a document can see. The other three — the
// anonymous table box §17.2.1 inserts, §17.4's table wrapper, and the text an
// <img>'s alt attribute becomes — answer the same either way, because nothing
// asks ensureFontSize about them; see the note on Box.fontSizeKnown.
func TestABoxThatTakesItsParentsSizeTakesItsParentsAnswer(t *testing.T) {
	const ib = `<div style="display: inline-block; width: 50px; height: 100px"></div>`
	for _, tc := range []struct {
		what, inner, extra string
		want               float64
	}{
		// An anonymous block: a block child among the inline ones is what makes
		// CSS Display §2.1 wrap the rest.
		{"an anonymous block", `<div style="height: 1px"></div>` + ib, ``, 101},
		// The same, one element deeper, so the wrapping happens around a piece
		// of a split inline.
		{"a split inline's pieces",
			`<span style="font-size: 0"><div style="height: 1px"></div>` + ib + `</span>`, ``, 101},
	} {
		root := layoutOf(t, 600, `<div id="w">`+tc.inner+`</div>`,
			noDefaults+`#w { float: left; font-size: 0 }`+tc.extra)
		if got := find(t, root, "w").BorderRect.H.Px(); got != tc.want {
			t.Errorf("%s: the box is %gpx tall, want %g — the font size it took from "+
				"its parent is a zero the document asked for", tc.what, got, tc.want)
		}
	}
}
