package layout

import (
	"testing"
)

// An <svg>'s width and height attributes, which are CSS properties.
//
// SVG calls them presentation attributes and its rendering section maps each to
// the property of the same name. This engine read them only as *intrinsic
// dimensions* — a number the picture carries — and that is right for "50" and
// wrong for "50%", because a percentage is a proportion of something the picture
// cannot see. Twelve of the suite's reftests turn on the difference.

// svgBoxWidth returns the size of the box an inline <svg> was given, measured
// through a rectangle that covers its viewport.
func svgBox(t *testing.T, attrs, css string) Rect {
	t.Helper()
	ops := paintOf(t,
		`<div id="d"><svg `+attrs+`><rect width="100%" height="100%" fill="rgb(0,0,255)"/></svg></div>`,
		css)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills for <svg %s>, want 1: %v", len(got), attrs, got)
	}
	return got[0]
}

// TestAPercentageDimensionResolvesAgainstTheContainingBlock is the fix.
//
// "height=50%" states no intrinsic height — there is nothing for a percentage to
// be a proportion *of* inside the picture — so read as one it meant nothing and
// the element fell back to the 300 by 150 of a replaced element with no
// dimensions. As the CSS height property it resolves against the containing
// block, which is what absolute-replaced-height-013 asserts.
func TestAPercentageDimensionResolvesAgainstTheContainingBlock(t *testing.T) {
	got := svgBox(t, `height="50%" width="80"`,
		`#d { height: 200px; font-size: 0 }`)
	if got.H != bgpx(100) {
		t.Errorf("the box is %v tall, want %v — half of the containing block's 200",
			got.H, bgpx(100))
	}
	if got.W != bgpx(80) {
		t.Errorf("the box is %v wide, want %v", got.W, bgpx(80))
	}
}

// TestAPlainDimensionStillSizesTheBox: the common case, unchanged. It arrives by
// a different route now — the property rather than the intrinsic size — and must
// come out the same.
func TestAPlainDimensionStillSizesTheBox(t *testing.T) {
	got := svgBox(t, `width="120" height="60"`, `#d { font-size: 0 }`)
	if got.W != bgpx(120) || got.H != bgpx(60) {
		t.Errorf("the box is %v by %v, want 120 by 60", got.W, got.H)
	}
}

// TestAStylesheetBeatsTheAttribute. A hint is not an inline style: it sits below
// every stylesheet declaration, so an author who writes a rule for the element
// overrides what the markup said. That ordering is the whole reason hints are a
// separate mechanism.
func TestAStylesheetBeatsTheAttribute(t *testing.T) {
	got := svgBox(t, `width="120" height="60"`,
		`#d { font-size: 0 } svg { width: 40px; height: 20px }`)
	if got.W != bgpx(40) || got.H != bgpx(20) {
		t.Errorf("the box is %v by %v, want the stylesheet's 40 by 20", got.W, got.H)
	}
}

// TestAPercentageIsStillNotAnIntrinsicDimension is the containment case.
//
// The hint gives the percentage somewhere to mean something; it must not also
// make the picture claim an intrinsic size it has not got. An <svg> stating only
// percentages has no intrinsic width, height or ratio, so a box with no other
// constraint gets the 300 by 150 CSS 2.1 §10.3.2 gives a replaced element with
// none — not a percentage of something.
func TestAPercentageIsStillNotAnIntrinsicDimension(t *testing.T) {
	c := svgContent([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="50%" height="50%">`+
		`<rect width="100%" height="100%" fill="green"/></svg>`), svgAsImage)
	if c == nil {
		t.Fatal("the picture was refused")
	}
	if c.Width != 0 || c.Height != 0 || c.Ratio != 0 {
		t.Errorf("the picture claims %v by %v ratio %v; a percentage is a "+
			"proportion of something it cannot see", c.Width, c.Height, c.Ratio)
	}
	// What it does claim is the percentage itself, kept apart from the three
	// above so that only a caller with something to be a percentage *of* can
	// turn it into a number. background-size is such a caller — the positioning
	// area is §5.4's default object size — and backgroundintrinsic_test.go is
	// where that half is held.
	if c.WidthPercent != 0.5 || c.HeightPercent != 0.5 {
		t.Errorf("the percentages read as %v and %v, want 0.5 and 0.5",
			c.WidthPercent, c.HeightPercent)
	}
}
