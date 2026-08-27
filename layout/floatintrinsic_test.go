package layout

import "testing"

// What a float contributes to the width of the box that holds it.
//
// At the *maximum* width nothing wraps, so the whole of a box's inline content
// is one line and a float in it stands beside that line rather than above or
// below it. A box shrink-wrapped around a 100px float and a 100px inline-block
// is 200 wide, and this engine made it 100: it took the widest of the two where
// it should have taken the pair. intrinsic-size-float-and-line says so by
// painting the container red — every pixel the shrink-wrap leaves short is a
// pixel of red the green boxes do not cover.
//
// At the *minimum* width the text wraps under the float instead, so the two
// stand one above the other and the widest of them is what the box must hold.
// That half was already right and is the second test here.

// shrinkToFit is the width a floated box comes out at, which is the box's
// maximum content width unless something else binds.
func shrinkToFit(t *testing.T, inner, extra string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+inner+`</div>`,
		`#d { float: left; font-family: Courier; font-size: 20px }
		 .f { float: right; width: 100px; height: 40px }
		 .ib { display: inline-block; width: 100px; height: 40px }
		 .c { clear: right }`+extra)
	return find(t, root, "d").BorderRect.W.Px()
}

// TestAFloatStandsBesideTheLineAtTheMaximumWidth is the fixture's own shape.
func TestAFloatStandsBesideTheLineAtTheMaximumWidth(t *testing.T) {
	if got := shrinkToFit(t, `<div class="f"></div><span class="ib"></span>`, ""); got != 200 {
		t.Errorf("a box holding a 100px float and a 100px inline-block shrank to "+
			"%gpx, want 200: at the maximum width they are side by side", got)
	}
	// Two of them and the box is three boxes wide, which is the half that says
	// this is a sum rather than "the float plus the widest thing beside it".
	if got := shrinkToFit(t,
		`<div class="f"></div><span class="ib"></span><div class="f"></div>`, ""); got != 300 {
		t.Errorf("two floats and an inline-block shrank to %gpx, want 300", got)
	}
}

// TestAFloatStandsAboveTheLineAtTheMinimumWidth is the other half, and the one
// that must not move: at the minimum the text wraps under the float, so the box
// needs the widest of the two and not their sum.
func TestAFloatStandsAboveTheLineAtTheMinimumWidth(t *testing.T) {
	got := shrinkToFit(t, `<div class="f"></div><span class="ib"></span>`,
		` #d { width: min-content }`)
	if got != 100 {
		t.Errorf("at min-content the box is %gpx, want 100: the text wraps under "+
			"the float, so they stand one above the other", got)
	}
}

// TestAClearedFloatBeginsANewRowOfThem. A float that clears goes below the
// floats before it, so it is not beside them and its width is not added to
// theirs.
//
// letter-spacing-206 is eight floated paragraphs, every one of them
// "clear: left", inside a floated container. Summing them makes the container
// eight paragraphs wide and the test's three blue boxes stop being identical.
func TestAClearedFloatBeginsANewRowOfThem(t *testing.T) {
	// The first row is the float and the inline-block, 200. The second is the
	// cleared float alone, 100. The box needs the wider of the two.
	if got := shrinkToFit(t,
		`<div class="f"></div><span class="ib"></span><div class="f c"></div>`, ""); got != 200 {
		t.Errorf("a cleared float made the box %gpx, want 200: it goes below the "+
			"row before it rather than beside it", got)
	}
	// And clearing does not take the float out of the measurement: the text
	// still sits beside it, because there is nothing above it to clear past.
	// "xx" is two Courier characters at 20px, which is 24.
	if got := shrinkToFit(t, `<div class="f c"></div>xx`, ""); got != 124 {
		t.Errorf("a cleared float with text after it made the box %gpx, want 124", got)
	}
}

// TestAForcedBreakDoesNotEndARowOfFloats. A <br> ends a line and clears
// nothing, so two right floats written either side of one stand beside each
// other and the box has to hold both.
//
// This is why the floats are counted apart from the line rather than added into
// it: the line is reset at every break and the row of floats is not.
func TestAForcedBreakDoesNotEndARowOfFloats(t *testing.T) {
	// Two 100px floats and a widest line of "xx", which is two Courier
	// characters at 20px: 224.
	if got := shrinkToFit(t, `<div class="f"></div>xx<br><div class="f"></div>yy`, ""); got != 224 {
		t.Errorf("two floats either side of a <br> made the box %gpx, want 224: a "+
			"forced break clears nothing, so they are side by side", got)
	}
}

// TestTheRowAFloatClearsPastIsStillMeasured. The row a cleared float goes below
// is what the box had to be wide enough for while it was being built, and
// nothing after the clear will ask for it again — so it has to be kept where
// the row ends rather than carried forward into a row it is not part of.
func TestTheRowAFloatClearsPastIsStillMeasured(t *testing.T) {
	// The first row is an inline-block and two floats: 300. The second is one
	// cleared float: 100. Forgetting the first leaves the box 200 wide, which
	// is the inline-block plus the cleared float — a row that never existed.
	got := shrinkToFit(t,
		`<span class="ib"></span><div class="f"></div><div class="f"></div>`+
			`<div class="f c"></div>`, "")
	if got != 300 {
		t.Errorf("the box is %gpx, want 300: the row the last float cleared past "+
			"held an inline-block and two floats", got)
	}
}
