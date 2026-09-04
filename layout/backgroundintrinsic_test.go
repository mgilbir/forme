package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The default sizing algorithm, CSS Images 3 §5.3, as background-size reaches it.
//
// An image with both an intrinsic width and an intrinsic height is the easy case
// and the only one a raster picture ever presents: a PNG always knows both. The
// cases that differ are the ones only a vector image can produce — a width and no
// height, a ratio and neither — and until SVGs were read at all, none of them
// could arise, so the code had two branches where the specification has seven.
//
// Each test below is one row of that algorithm, and the fixture is an SVG
// declaring exactly the dimensions the row is about.

// bgSVG builds an SVG with the given root attributes, painting solid green.
func bgSVG(attrs string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" ` + attrs + `>` +
		`<rect width="100%" height="100%" fill="rgb(0,128,0)"/></svg>`)
}

// backgroundRect is where and how large one solid background layer painted.
func backgroundRect(t *testing.T, svgAttrs, css string) Rect {
	t.Helper()
	res := mapResolver{"bg.svg": bgSVG(svgAttrs)}
	ops := paintWith(t, res, `<div id="d"></div>`,
		`#d { width: 100px; height: 50px; background-repeat: no-repeat;
		      background-image: url(bg.svg); `+css+` }`)
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d green fills, want 1: %v", len(got), got)
	}
	return got[0]
}

// bgpx is style.FromPx without the second result, for a table of expectations.
func bgpx(v float64) style.Unit { u, _ := style.FromPx(v); return u }

// TestBackgroundSizeAutoAuto walks every row of the algorithm that "auto auto"
// reaches. The box is 100 by 50, so a result equal to one of those is the
// positioning area standing in for a dimension the image did not have.
func TestBackgroundSizeAutoAuto(t *testing.T) {
	for _, tc := range []struct {
		what  string
		attrs string
		w, h  float64
	}{
		// Both: used as they are, and nothing about the area enters.
		{"a width and a height", `width="40" height="20"`, 40, 20},

		// One and a ratio: the other follows from the ratio. A viewBox is how
		// an SVG states a ratio without stating a size.
		{"a width and a ratio", `width="40" viewBox="0 0 2 1"`, 40, 20},
		{"a height and a ratio", `height="20" viewBox="0 0 2 1"`, 40, 20},

		// One and no ratio: the other is the positioning area's. This is the row
		// that was wrong — both came from the area, so an image with a width was
		// stretched to the full width of the box.
		{"a width alone", `width="40"`, 40, 50},
		{"a height alone", `height="20"`, 100, 20},

		// A ratio and neither: §3.9 renders it as though "contain" were given.
		// The box is 2:1 and the image 1:1, so the height is the constraint.
		{"a square ratio alone", `viewBox="0 0 1 1"`, 50, 50},
		// 4:1 is wider than the box's 2:1, so the width is the constraint.
		{"a wide ratio alone", `viewBox="0 0 4 1"`, 100, 25},

		// Nothing at all: the whole positioning area.
		{"no size at all", ``, 100, 50},

		// A percentage is not an intrinsic dimension — every rule that asks
		// whether the image has one gets "no" — and it is still a dimension once
		// there is something to be a percentage *of*. §5.4 says what: the
		// default object size, which for a background layer is the positioning
		// area. So these rows are not the row above with different numbers; the
		// image is sized and not defaulted, and forty per cent of a hundred is
		// forty rather than the whole of it.
		{"percentages of the area", `width="40%" height="60%"`, 40, 30},
		{"a percentage width alone", `width="40%"`, 40, 50},
		{"a percentage height alone", `height="60%"`, 100, 30},
		// A hundred per cent of the area is the area, which is what no size at
		// all gives as well. It is here to say the two agree rather than to
		// test either, and it is the row that made the distinction invisible
		// for as long as it was the only percentage written down.
		{"percentages of the whole area", `width="100%" height="100%"`, 100, 50},
	} {
		got := backgroundRect(t, tc.attrs, "")
		if got.W != bgpx(tc.w) || got.H != bgpx(tc.h) {
			t.Errorf("%s: painted %v by %v, want %v by %v",
				tc.what, got.W, got.H, bgpx(tc.w), bgpx(tc.h))
		}
	}
}

// TestBackgroundSizeWithOneLengthGiven: the other dimension is auto, and it comes
// from the ratio, or from the image's own, or from the area — in that order.
func TestBackgroundSizeWithOneLengthGiven(t *testing.T) {
	for _, tc := range []struct {
		what  string
		attrs string
		size  string
		w, h  float64
	}{
		{"a width given, a ratio to follow", `viewBox="0 0 2 1"`, "60px auto", 60, 30},
		{"a height given, a ratio to follow", `viewBox="0 0 2 1"`, "auto 30px", 60, 30},
		// No ratio: the image's own dimension is kept rather than invented.
		{"a width given, no ratio", `height="20"`, "60px auto", 60, 20},
		{"a height given, no ratio", `width="40"`, "auto 30px", 40, 30},
		// Neither: the area stands in.
		{"a width given, nothing to go on", ``, "60px auto", 60, 50},
		{"a height given, nothing to go on", ``, "auto 30px", 100, 30},
	} {
		got := backgroundRect(t, tc.attrs, "background-size: "+tc.size)
		if got.W != bgpx(tc.w) || got.H != bgpx(tc.h) {
			t.Errorf("%s (%s): painted %v by %v, want %v by %v",
				tc.what, tc.size, got.W, got.H, bgpx(tc.w), bgpx(tc.h))
		}
	}
}

// TestBackgroundSizeContainAndCover. Both preserve the ratio and differ only in
// which way they miss the area; without a ratio there is nothing to preserve and
// both are the area itself.
//
// This one calls the sizing directly rather than measuring what was painted, and
// the reason is the whole difference between the two values: a covering image is
// larger than the area on one axis *by definition*, and what reaches the page is
// clipped to the box. Measuring the fill would show cover and contain and "no
// ratio at all" as the same rectangle, and the test would pass with the
// arithmetic inverted — which is exactly what it did before this was rewritten.
func TestBackgroundSizeContainAndCover(t *testing.T) {
	area := Rect{W: bgpx(100), H: bgpx(50)} // 2:1
	for _, tc := range []struct {
		what          string
		iw, ih, ratio float64
		kind          bgSizeKind
		w, h          float64
	}{
		// A square image against a 2:1 area. Contained it fits the height;
		// covering it fits the width and overflows the height.
		{"a square contained", 0, 0, 1, bgSizeContain, 50, 50},
		{"a square covering", 0, 0, 1, bgSizeCover, 100, 100},
		// A 4:1 image is wider than the area: contained it fits the width,
		// covering it fits the height and overflows the width.
		{"a wide image contained", 0, 0, 4, bgSizeContain, 100, 25},
		{"a wide image covering", 0, 0, 4, bgSizeCover, 200, 50},
		// Same shape as the area: the two agree, and both are the area.
		{"the area's own ratio, contained", 0, 0, 2, bgSizeContain, 100, 50},
		{"the area's own ratio, covering", 0, 0, 2, bgSizeCover, 100, 50},
		// The intrinsic dimensions give the ratio when nothing else does.
		{"dimensions instead of a ratio, contained", 40, 20, 0, bgSizeContain, 100, 50},
		{"dimensions instead of a ratio, covering", 40, 20, 0, bgSizeCover, 100, 50},
		// Nothing to preserve, so nothing to scale.
		{"no ratio, contained", 0, 0, 0, bgSizeContain, 100, 50},
		{"no ratio, covering", 0, 0, 0, bgSizeCover, 100, 50},
	} {
		colour := green
		layer := backgroundLayer{
			sizeKind: tc.kind,
			image: &ReplacedContent{
				Width: bgpx(tc.iw), Height: bgpx(tc.ih), Ratio: tc.ratio, Solid: &colour,
			},
		}
		w, h, _, _ := (&layouter{}).tileSize(layer, area)
		if w != bgpx(tc.w) || h != bgpx(tc.h) {
			t.Errorf("%s: %v by %v, want %v by %v",
				tc.what, w, h, bgpx(tc.w), bgpx(tc.h))
		}
	}
}

// TestCoverIsNeverSmallerThanTheAreaAndContainNeverLarger is the property the
// table above states case by case, asserted as the rule it comes from — so that
// a future ratio nobody wrote a row for cannot be got wrong quietly.
func TestCoverIsNeverSmallerThanTheAreaAndContainNeverLarger(t *testing.T) {
	area := Rect{W: bgpx(100), H: bgpx(50)}
	for _, ratio := range []float64{0.1, 0.5, 1, 1.9, 2, 2.1, 4, 10} {
		colour := green
		for _, kind := range []bgSizeKind{bgSizeContain, bgSizeCover} {
			layer := backgroundLayer{
				sizeKind: kind,
				image:    &ReplacedContent{Ratio: ratio, Solid: &colour},
			}
			w, h, _, _ := (&layouter{}).tileSize(layer, area)
			// The ratio has to survive, whichever way the box misses.
			if got := w.Px() / h.Px(); got < ratio*0.999 || got > ratio*1.001 {
				t.Errorf("ratio %v, %v: came out %v by %v, a ratio of %v",
					ratio, kind, w, h, got)
				continue
			}
			fits := w <= area.W && h <= area.H
			covers := w >= area.W && h >= area.H
			if kind == bgSizeContain && !fits {
				t.Errorf("ratio %v contained: %v by %v does not fit inside %v by %v",
					ratio, w, h, area.W, area.H)
			}
			if kind == bgSizeCover && !covers {
				t.Errorf("ratio %v covering: %v by %v does not cover %v by %v",
					ratio, w, h, area.W, area.H)
			}
		}
	}
}

// TestBothLengthsGivenIgnoreTheImage is the containment case: when the style
// states both dimensions, nothing about the image may reach the result. It is
// worth its own test because every branch above reads the image, and a wrong
// one that fell through to them would be invisible in a fixture with no
// intrinsic size.
func TestBothLengthsGivenIgnoreTheImage(t *testing.T) {
	for _, attrs := range []string{
		``, `width="40"`, `height="20"`, `width="40" height="20"`, `viewBox="0 0 4 1"`,
	} {
		got := backgroundRect(t, attrs, "background-size: 70px 35px")
		if got.W != bgpx(70) || got.H != bgpx(35) {
			t.Errorf("with %q the image was painted %v by %v; both lengths were "+
				"given and neither may come from the image", attrs, got.W, got.H)
		}
	}
}

// TestABackgroundThatFailedToLoadPaintsNothing holds the behaviour the sizing
// algorithm relies on, so that it needs no case for content that has no size.
//
// It is worth stating plainly what this test does and does not do. It asserts
// the outcome — a picture that could not be decoded puts nothing on the page —
// and that outcome is protected three times over: the layer is dropped for
// content that paints nothing, tileSize returns nothing for content that is nil,
// and a tiling of no size is refused. Removing any *one* of those leaves the
// other two, and this test goes on passing, which was established by planting
// each in turn rather than assumed.
//
// So it pins no single guard, and a reader should not go looking for the one it
// protects. What it protects is the property, which is the thing that matters
// and the thing that would be noticed if all of them went.
//
// The reason it exists at all is that the sizing code used to carry a *fourth*
// check for the same thing, and that one was not redundant but dead: it caught
// every partial-dimension case as well, so an SVG with a width and no height
// came out sized to the area on both axes. It is gone, and this is what says
// nothing was lost with it.
func TestABackgroundThatFailedToLoadPaintsNothing(t *testing.T) {
	res := mapResolver{"bg.png": []byte("not a picture at all")}
	ops := paintWith(t, res, `<div id="d"></div>`,
		`#d { width: 100px; height: 50px; background-image: url(bg.png) }`)
	for _, op := range ops {
		switch op.(type) {
		case TileImage:
			t.Errorf("an image that could not be decoded was tiled")
		case DrawImage:
			t.Errorf("an image that could not be decoded was drawn")
		}
	}
	// And a layer that *can* paint still does, so this is not passing because
	// the fixture paints nothing whatever the image.
	ok := paintWith(t, mapResolver{"bg.svg": bgSVG(`width="10" height="10"`)},
		`<div id="d"></div>`,
		`#d { width: 100px; height: 50px; background-image: url(bg.svg) }`)
	if len(fillsOf(ok, green)) == 0 {
		t.Errorf("the control painted nothing; the test above would pass with the " +
			"gate removed")
	}
}

// TestAPercentageDimensionGivesNoRatioToPreserve is where the two halves of §5.4
// part company, and the reason the percentages are resolved *after* the ratio
// rather than before it.
//
// An image whose dimensions are percentages has no intrinsic ratio — the numbers
// are proportions of something it does not know, and two proportions of two
// different things are not a shape. So "contain" and "cover" have nothing to
// preserve and both mean the area, exactly as they do for an image with no size
// at all.
//
// Resolving the percentages first would give the pair a ratio the image does not
// have: forty per cent of a hundred by sixty per cent of fifty is 40 by 30,
// which is 4:3, and "contain" would then paint a 66-by-50 rectangle in a
// hundred-by-fifty area. The suite has no test for it and the specification is
// unambiguous, which is why this is here.
func TestAPercentageDimensionGivesNoRatioToPreserve(t *testing.T) {
	area := Rect{W: bgpx(100), H: bgpx(50)}
	for _, kind := range []bgSizeKind{bgSizeContain, bgSizeCover} {
		colour := green
		layer := backgroundLayer{
			sizeKind: kind,
			image: &ReplacedContent{
				WidthPercent: 0.4, HeightPercent: 0.6, Solid: &colour,
			},
		}
		w, h, _, _ := (&layouter{}).tileSize(layer, area)
		if w != area.W || h != area.H {
			t.Errorf("%v gave %v by %v, want the area, %v by %v — a percentage "+
				"pair is not a ratio", kind, w, h, area.W, area.H)
		}
	}
}

// TestAPercentageThatIsNotALengthIsNotOne, because the reader is a string test
// and a string test is where a wrong number comes from.
func TestAPercentageThatIsNotALengthIsNotOne(t *testing.T) {
	for _, attrs := range []string{
		`width="0%"`, `width="-40%"`, `width="%"`, `width="4 0%"`, `width="40"`,
	} {
		got := svgContent(bgSVG(attrs), svgAsImage)
		if got == nil {
			t.Fatalf("%s: the SVG was refused", attrs)
		}
		if got.WidthPercent != 0 {
			t.Errorf("%s: read as %v of the area, want no percentage at all",
				attrs, got.WidthPercent)
		}
	}
}
