package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// CSS Values 4's math functions as layout reads them: a min(), max() or
// clamp() over a percentage is carried as a LengthMath and run against the
// containing block once there is one.

// TestAMathFunctionOverAPercentageSizesABox is the everyday shape: a width
// that is half its container but never more than 300px.
func TestAMathFunctionOverAPercentageSizesABox(t *testing.T) {
	for _, tc := range []struct {
		k    float64 // the container's width
		decl string
		want float64
	}{
		{400, "width: min(50%, 300px)", 200},
		{1000, "width: min(50%, 300px)", 300},
		{400, "width: max(50%, 300px)", 300},
		{400, "width: clamp(10px, 5% + 2em, 50%)", 60},  // 20 + 40
		{1000, "width: clamp(10px, 5% + 2em, 10%)", 90}, // 50 + 40 under 100
		{400, "width: clamp(300px, 10%, 100px)", 300},   // MIN wins over MAX
		{400, "width: calc(100% - min(10%, 10px))", 390},
		{400, "width: 100px; padding-left: min(10%, 100px)", 140},
		{400, "width: round(up, 33%, 50px)", 150},
	} {
		root := layoutOf(t, 1000, `<div id="k"><div id="a"></div></div>`,
			noDefaults+`#k { width: `+strconv.FormatFloat(tc.k, 'f', -1, 64)+`px } #a { height: 5px; font-size: 20px; `+
				tc.decl+` }`)
		if got := find(t, root, "a").BorderRect.W.Px(); got != tc.want {
			t.Errorf("in %gpx, %s: %gpx wide, want %g", tc.k, tc.decl, got, tc.want)
		}
	}
	// The margin is where it says, not merely counted.
	root := layoutOf(t, 1000, `<div id="k"><div id="a"></div></div>`,
		noDefaults+`#k { width: 400px } #a { height: 5px; width: 10px; margin-left: max(5%, 1em) }`)
	if got := relX(t, find(t, root, "a"), find(t, root, "k")).Px(); got != 20 {
		t.Errorf("margin-left: max(5%%, 1em) in 400px put the box at %g, want 20 "+
			"(5%% of 400, over the 16px em)", got)
	}
}

// TestAMathFunctionOverAnIndefiniteHeightIsAuto: a percentage height of a
// container whose height depends on its content computes to auto, and one
// inside min() is as indefinite as a bare one.
func TestAMathFunctionOverAnIndefiniteHeightIsAuto(t *testing.T) {
	root := layoutOf(t, 1000,
		`<div id="k"><div id="a"><div id="c"></div></div></div><div id="d"><div id="b"></div></div>`,
		noDefaults+`#c { height: 30px } #a { height: min(20%, 100px) }
		#d { height: 400px } #b { height: min(20%, 100px) }`)
	if got := find(t, root, "a").BorderRect.H.Px(); got != 30 {
		t.Errorf("min(20%%, 100px) of an auto height is %gpx, want its content's 30", got)
	}
	if got := find(t, root, "b").BorderRect.H.Px(); got != 80 {
		t.Errorf("min(20%%, 100px) of 400px is %gpx, want 80", got)
	}
}

// TestAnAbsolutelyPositionedBoxResolvesAMathFunction: its containing block is
// definite on both axes.
func TestAnAbsolutelyPositionedBoxResolvesAMathFunction(t *testing.T) {
	root := layoutOf(t, 1000, `<div id="k"><div id="a"></div></div>`,
		noDefaults+`#k { position: relative; width: 400px; height: 300px }
		#a { position: absolute; left: min(10%, 100px); top: 0;
			width: min(50%, 300px); height: max(10%, 5px) }`)
	a, k := find(t, root, "a"), find(t, root, "k")
	if w, h := a.BorderRect.W.Px(), a.BorderRect.H.Px(); w != 200 || h != 30 {
		t.Errorf("the box is %gx%g, want 200x30", w, h)
	}
	if x := relX(t, a, k).Px(); x != 40 {
		t.Errorf("left: min(10%%, 100px) put it at %g, want 40", x)
	}
}

// TestAPercentageInsideAFunctionReadsTheContainingBlock: a positioned box whose
// containing block this engine does not form is reported when anything about it
// is measured against that block, and a percentage inside calc() or min() is
// measured against it as much as a bare one. Both used to be missed.
func TestAPercentageInsideAFunctionReadsTheContainingBlock(t *testing.T) {
	for _, tc := range []struct {
		width    string
		reported bool
	}{
		{"50%", true},
		{"calc(50% + 1px)", true},
		{"min(50%, 100px)", true},
		{"50px", false},
		{"min(50px, 2em)", false},
	} {
		_, findings := filterFindings(t,
			`<table><tr id="r"><td>x<div id="a"></div></td></tr></table>`,
			`#r { position: relative } #a { position: absolute; width: `+tc.width+` }`)
		got := false
		for _, f := range findings {
			got = got || f.Rule == RulePositionApproximated
		}
		if got != tc.reported {
			t.Errorf("width: %s: reported=%v, want %v", tc.width, got, tc.reported)
		}
	}
}

// TestATableCellsMathWidthIsAutoAndSaid: a column's percentage is of the
// table's width, which the column is helping to decide, and a min() over one
// cannot be split into a length demand and a percentage demand as a calc()
// can. It is laid out as auto, and said so.
func TestATableCellsMathWidthIsAutoAndSaid(t *testing.T) {
	_, findings := filterFindings(t,
		`<table><tr><td id="c">x</td><td>y</td></tr></table>`,
		`#c { width: min(30%, 200px) }`)
	said := false
	for _, f := range findings {
		if f.Rule == RuleUnsupportedValue && f.Property == "width" &&
			strings.Contains(f.Message, "min(30%, 200px)") {
			said = true
		}
	}
	if !said {
		t.Errorf("a cell's min(30%%, 200px) was not reported: %v", findings)
	}
	// A min() without a percentage in it is an ordinary length there.
	_, findings = filterFindings(t,
		`<table><tr><td id="c">x</td><td>y</td></tr></table>`,
		`#c { width: min(30px, 200px) }`)
	for _, f := range findings {
		if f.Property == "width" {
			t.Errorf("a cell's min(30px, 200px) was reported: %v", f)
		}
	}
}

// TestABackgroundPositionUnderMinIsOfTheFreeSpace: a background position's
// percentage is of the area less the image, which can be negative, and a
// math function runs against the same number.
func TestABackgroundPositionUnderMinIsOfTheFreeSpace(t *testing.T) {
	offset := func(src string) style.Length {
		vals, _ := css.ParseComponentValues(src)
		l, _, ok := style.ParseLength(vals, style.LengthContext{FontSize: 16})
		if !ok || l.Kind != style.LengthMath {
			t.Fatalf("%s read as %+v, %v", src, l, ok)
		}
		return l
	}
	px := func(v float64) style.Unit { u, _ := style.FromPx(v); return u }
	for _, tc := range []struct {
		src       string
		area, img float64
		want      float64
	}{
		{"min(50%, 10px)", 100, 20, 10}, // 40 against 10
		{"max(50%, 10px)", 100, 20, 40},
		{"min(50%, 10px)", 20, 100, -40}, // the image is larger: -40 against 10
		{"calc(10px * sign(10%))", 20, 100, -10},
	} {
		p := bgPos{offset: offset(tc.src)}
		if got := p.place(px(tc.area), px(tc.img)).Px(); got != tc.want {
			t.Errorf("%s for a %g image in %g is at %g, want %g", tc.src, tc.img, tc.area,
				got, tc.want)
		}
	}
	// And background-size, whose percentage is of the area itself.
	if got, auto := resolveBgLength(offset("min(50%, 30px)"), px(100)); auto || got.Px() != 30 {
		t.Errorf("background-size min(50%%, 30px) of 100 is %g (auto=%v), want 30", got.Px(), auto)
	}
}

// TestAGradientStopUnderMinIsOfTheLine: a stop position is a percentage of
// the gradient line.
func TestAGradientStopUnderMinIsOfTheLine(t *testing.T) {
	vals, _ := css.ParseComponentValues("min(50%, 30px)")
	l, _, ok := style.ParseLength(vals, style.LengthContext{FontSize: 16})
	if !ok || !isLengthPercentage(l) {
		t.Fatalf("min(50%%, 30px) is not a stop position: %+v %v", l, ok)
	}
	if got := resolvePx(l, 40); got != 20 {
		t.Errorf("min(50%%, 30px) along 40px is %g, want 20", got)
	}
	if got := resolvePx(l, 100); got != 30 {
		t.Errorf("min(50%%, 30px) along 100px is %g, want 30", got)
	}
}

// TestATablesMathWidthIsOfTheWrappersContainingBlock: a table whose width has
// a percentage in it is sized against the block its wrapper sits in, as a bare
// percentage is — fixed-table-layout-023's 640px body and "width: 80%" is a
// 512px table — and not against the wrapper, which is shrinking to fit it.
func TestATablesMathWidthIsOfTheWrappersContainingBlock(t *testing.T) {
	for _, tc := range []struct {
		width string
		want  float64
	}{
		{"80%", 512},
		{"calc(80% + 1px)", 513},
		{"min(80%, 1000px)", 512},
		{"max(80%, 600px)", 600},
	} {
		root := layoutOf(t, 1000, `<div id="k"><table id="t"><tr><td>x</td></tr></table></div>`,
			noDefaults+`#k { width: 640px } #t { border-spacing: 0; width: `+tc.width+` }`)
		if got := find(t, root, "t").BorderRect.W.Px(); got != tc.want {
			t.Errorf("width: %s in 640px is a %gpx table, want %g", tc.width, got, tc.want)
		}
	}
}

// TestAReplacedHeightUnderMinTransfersItsWidth: an inline-block shrinks to
// fit a picture whose height is a percentage of a height written down above
// it, and the picture's ratio gives its width. A percentage inside min() or
// calc() is of the same height.
func TestAReplacedHeightUnderMinTransfersItsWidth(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want float64
	}{
		{`#e { display: inline-block; height: 200px } img { height: 50% }`, 200},
		{`#e { display: inline-block; height: 200px } img { height: min(50%, 300px) }`, 200},
		{`#e { display: inline-block; height: 200px } img { height: calc(50% + 10px) }`, 220},
		{`#k { height: 400px } #e { display: inline-block; height: min(50%, 300px) }
			img { height: 50% }`, 200},
	} {
		w, _ := sizeOf(t, `<div id="k"><span id="e"><img src="wide.svg"></span></div>`, tc.css)
		if w != tc.want {
			t.Errorf("%s: the inline-block is %gpx wide, want %g", tc.css, w, tc.want)
		}
	}
}

// TestAPageMarginUnderMinIsOfThePage: a page margin's percentage is of the
// page box, which is known before anything is laid out, so a min() over one
// is applied. It used to be dropped and reported as unevaluated.
func TestAPageMarginUnderMinIsOfThePage(t *testing.T) {
	opts := Options{Page: PageSizePt(600, 800).WithMarginPt(36)}
	for css, want := range map[string]float64{
		`@page { margin: min(1in, 10%) }`:          640, // 800px wide: 80px a side
		`@page { margin: max(1in, 10%) }`:          608, // 96px a side
		`@page { margin: 0 clamp(10px, 5%, 1in) }`: 720,
	} {
		if got := pageWidthPx(t, css, opts); got != want {
			t.Errorf("%s: %gpx of content, want %g", css, got, want)
		}
	}
}
