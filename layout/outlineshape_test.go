package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The shape of an outline: its offset and its corners. See outlineshape.go.

// outlineRect is the smallest rectangle holding every outline mark of a colour.
func outlineRect(t *testing.T, ops []Op, c style.RGBA) Rect {
	t.Helper()
	var out Rect
	have := false
	add := func(r Rect) {
		if !have {
			out, have = r, true
			return
		}
		x0, y0 := style.Min(out.X, r.X), style.Min(out.Y, r.Y)
		x1, y1 := style.Max(out.Right(), r.Right()), style.Max(out.Bottom(), r.Bottom())
		out = Rect{X: x0, Y: y0, W: x1.Sub(x0), H: y1.Sub(y0)}
	}
	for _, f := range fillsOfColour(ops, c) {
		add(f.Rect)
	}
	for _, p := range pathsOf(ops) {
		if p.Color == c {
			add(p.Path.Bounds())
		}
	}
	if !have {
		t.Fatalf("nothing is drawn in %v: %v", c, ops)
	}
	return out
}

// TestAnOutlineFollowsTheCurve is CSS UI 4 §3's "it should follow the
// border-radius curve": round a box with 20px corners, a 4px outline is the
// band between the border edge's curve and the same curve four pixels out,
// whose radius is 24px, and nothing is reported.
func TestAnOutlineFollowsTheCurve(t *testing.T) {
	const css = `#a { width: 100px; height: 60px; border-radius: 20px; outline: 4px solid blue }`
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+css)
	f := radiusFragment(t, `<div id="a"></div>`, noDefaults+css, "a")
	ps := pathsOf(ops)
	if len(ps) != 1 {
		t.Fatalf("%d paths, want the outline's ring: %v", len(ps), ops)
	}
	r := f.BorderRect
	o := r.Outset(Edges{Top: rpx(4), Right: rpx(4), Bottom: rpx(4), Left: rpx(4)})
	want := ring(o, Radii{corner(24, 24), corner(24, 24), corner(24, 24), corner(24, 24)},
		r, Radii{corner(20, 20), corner(20, 20), corner(20, 20), corner(20, 20)})
	if ps[0].Path.String() != want.String() || ps[0].Color != blue {
		t.Errorf("the outline is %s in %v, want %s", ps[0].Path, ps[0].Color, want)
	}
	if !ps[0].Overhang {
		t.Error("a rounded outline is not Overhang, and the page guardrail would read it")
	}
	if fs := paintFindingsOf(t, `<div id="a"></div>`, noDefaults+css); hasRule(fs, RuleUnsupportedValue) {
		t.Errorf("a rounded outline was reported: %v", fs)
	}
}

// TestTheAdjustedRadius is CSS Backgrounds 3 §4.2's "adjusted radius dimension
// given numbers coverage, radius, and outset", case by case, and the radius of
// an edge moved in.
func TestTheAdjustedRadius(t *testing.T) {
	for _, tc := range []struct{ coverage, radius, outset, want float64 }{
		// A radius larger than the outset grows by it.
		{0.1, 20, 4, 24},
		// So does one whose corner is most of its box.
		{1.2, 2, 10, 12},
		// A square corner stays square.
		{0, 0, 10, 0},
		// Otherwise radius + outset × (1 − (1 − ratio)³ × (1 − coverage³)):
		// 5 + 10 × (1 − 0.125 × 0.999).
		{0.1, 5, 10, 5 + 10*(1-0.125*0.999)},
		{0.5, 2, 8, 2 + 8*(1-math.Pow(0.75, 3)*(1-0.125))},
		// Moved in: less the inset, and nothing past it.
		{0.1, 20, -4, 16},
		{0.1, 5, -10, 0},
	} {
		if got := adjustedRadius(tc.coverage, tc.radius, tc.outset); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("coverage %v, radius %v, outset %v: %v, want %v",
				tc.coverage, tc.radius, tc.outset, got, tc.want)
		}
	}
	// coverage is twice the smaller of a corner's two fractions of its box: a
	// 10px corner on a 100 by 40 box covers 2 × 10/100 = 0.2 of it, so an
	// outset of 30 grows it to 10 + 30 × (1 − (2/3)³ × (1 − 0.008)).
	got := outsetRadii(Radii{TopLeft: corner(10, 10)}, Rect{W: rpx(100), H: rpx(40)}, rpx(30), rpx(30))
	want := 10 + 30*(1-math.Pow(1-10.0/30, 3)*(1-math.Pow(0.2, 3)))
	if math.Abs(got.TopLeft.X.Px()-want) > 1.0/64 || math.Abs(got.TopLeft.Y.Px()-want) > 1.0/64 {
		t.Errorf("the corner is %v, want %.4f on both axes", got.TopLeft, want)
	}
}

// TestOutlineOffsetMovesTheOutline is §3.5: the outline is outset from the
// border edge by outline-offset, and a negative offset shrinks it into the
// border box — but never so far that the outline's outside is less than twice
// its width on either axis, which is the offset held at minus half the box.
func TestOutlineOffsetMovesTheOutline(t *testing.T) {
	vw := math.Floor(A4.Content().W.Px()/100*64) / 64
	for _, tc := range []struct {
		css        string
		x, y, w, h float64
	}{
		{`outline: 4px solid blue; outline-offset: 6px`, -10, -10, 120, 80},
		{`outline: 4px solid blue; outline-offset: 0.5em`, -9, -9, 118, 78},
		// A length the cascade leaves to layout: a hundredth of the page.
		{`outline: 4px solid blue; outline-offset: 1vw`, -4 - vw, -4 - vw, 108 + 2*vw, 68 + 2*vw},
		{`outline: 4px solid blue; outline-offset: -10px`, 6, 6, 88, 48},
		// Held at minus half the box on the axis where it would pass it:
		// 30 down, and 50 across.
		{`outline: 4px solid blue; outline-offset: -35px`, 31, 26, 38, 8},
		{`outline: 4px solid blue; outline-offset: -60px`, 46, 26, 8, 8},
	} {
		ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px; font-size: 10px; `+tc.css+` }`)
		got := outlineRect(t, ops, blue)
		want := Rect{X: rpx(tc.x), Y: rpx(tc.y), W: rpx(tc.w), H: rpx(tc.h)}
		if got != want {
			t.Errorf("%s: the outline covers %v, want %v", tc.css, got, want)
		}
	}
}

// TestAnOutlineOffsetRoundsItsCorners: a rounded outline moved out by its
// offset grows its radii by §4.2's outset-adjusted radius, and moved in, loses
// the inset from them. A 30px radius is larger than a 10px offset, so it grows
// to 40, and the outline's outer curve is 4px further out; moved in 35px on a
// box 60 tall, the offset is held at 30 down and the corner is square.
func TestAnOutlineOffsetRoundsItsCorners(t *testing.T) {
	const doc = `<div id="a"></div>`
	for _, tc := range []struct {
		css    string
		off    float64
		ri, ro float64
	}{
		{`outline-offset: 10px`, 10, 40, 44},
		{`outline-offset: -10px`, -10, 20, 24},
		{`outline-offset: -35px`, -35, 0, 0},
	} {
		css := noDefaults + `#a { width: 100px; height: 60px; border-radius: 30px; outline: 4px solid blue; ` + tc.css + ` }`
		f := radiusFragment(t, doc, css, "a")
		ops := paintOf(t, doc, css)
		in, iR, out, oR := outlineEdges(f.BorderRect, f.radii, rpx(tc.off), rpx(4))
		if iR.TopLeft != corner(tc.ri, tc.ri) || oR.TopLeft != corner(tc.ro, tc.ro) {
			t.Errorf("%s: the radii are %v inside and %v outside, want %v and %v",
				tc.css, iR.TopLeft, oR.TopLeft, tc.ri, tc.ro)
		}
		if tc.ro == 0 {
			if len(pathsOf(ops)) != 0 || outlineRect(t, ops, blue) != out {
				t.Errorf("%s: a square outline is not the bands of %v: %v", tc.css, out, ops)
			}
			continue
		}
		ps := pathsOf(ops)
		if len(ps) != 1 || ps[0].Path.String() != ring(out, oR, in, iR).String() {
			t.Errorf("%s: the outline is %v, want the ring %s", tc.css, ps, ring(out, oR, in, iR))
		}
	}
}

// TestAnAutoOutlineIsSolidInTheTextColour: css-ui-4 lets "outline-style: auto"
// be drawn as solid, and "outline-color: auto" is currentColor for a solid
// outline and the accent colour, chosen here to be currentColor too, for an
// auto one. "outline: auto" sets both. The suite's outline-auto-width-001
// accepts a reference in which "auto" is "solid" at the same width.
func TestAnAutoOutlineIsSolidInTheTextColour(t *testing.T) {
	solid := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px; outline: 1em solid blue }`)
	for _, css := range []string{
		`outline-style: auto; outline-width: 1em; outline-color: blue`,
		`outline: auto 1em; color: blue`,
		`outline: 1em solid auto; color: blue`,
	} {
		auto := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px; `+css+` }`)
		if !pictureEqual(auto, solid, picPage) || len(fillsOfColour(auto, blue)) != 4 {
			t.Errorf("%s is not the solid blue outline: %v", css, auto)
		}
	}
}

// TestADashedOutlineGoesRoundItsCorners: an outline round a rounded box is a
// rounded ring in every style, and a dashed one's marks go round the curve as
// a dashed border's do.
func TestADashedOutlineGoesRoundItsCorners(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 200px; height: 100px;
		border-radius: 30px; outline: 4px dashed blue; outline-offset: 2px }`)
	ps := pathsOf(ops)
	if len(ps) != 4 {
		t.Fatalf("%d paths, want a side of dashes each: %v", len(ps), ops)
	}
	// The corner's dash is centred on the diagonal of the top left corner,
	// whose curves are all centred on (30, 30): the offset edge's radius is
	// the 30px one grown by the 2px offset, and the outline's middle is half
	// its width further out, 34 from the centre.
	d := 30 - 34/math.Sqrt2
	if !dashedAt(ps, d, d) {
		t.Errorf("the corner's diagonal (%.2f, %.2f) is not in a dash", d, d)
	}
	for _, p := range ps {
		if !p.Overhang {
			t.Error("a dashed outline's marks are not Overhang")
		}
	}
}

// TestABrokenRoundedOutlineIsRoundWhereItsPiecesAreApart: an inline box broken
// across lines is rounded at its two ends (§8.6's slice model), and where the
// outlines of its pieces do not meet each piece is its own rounded ring. Where
// they meet, the union is drawn square, and that is reported.
func TestABrokenRoundedOutlineIsRoundWhereItsPiecesAreApart(t *testing.T) {
	// Two pieces of four letters, one above the other.
	const doc = `<div id="c"><span id="s">bbbb bbbb</span></div>`
	apart := noDefaults + `#c { width: 30px; font: 10px/40px monospace }
		#s { border-radius: 6px; outline: 2px solid blue }`
	ops := paintOf(t, doc, apart)
	if n := len(pathsOf(ops)); n != 2 {
		t.Errorf("%d rounded rings, want one for each of the two pieces: %v", n, ops)
	}
	if fs := paintFindingsOf(t, doc, apart); hasRule(fs, RuleUnsupportedValue) {
		t.Errorf("pieces apart were reported: %v", fs)
	}
	close := noDefaults + `#c { width: 30px; font: 10px/10px monospace }
		#s { border-radius: 6px; outline: 2px solid blue }`
	found := false
	for _, f := range paintFindingsOf(t, doc, close) {
		found = found || (f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "square corners"))
	}
	if !found {
		t.Error("the square union of rounded pieces whose outlines meet was not reported")
	}
}

// TestAnOutlineInsideACircleCoversItsBox is the suite's css-ui/outline-005
// (a "should" test all three browsers pass): a green circle whose green
// outline, a hundred pixels wide and moved in by one, follows the circle, and
// together they cover the red box around them. Drawn with square corners, the
// hole in the ring is a square the circle does not fill, and red shows in its
// corners.
func TestAnOutlineInsideACircleCoversItsBox(t *testing.T) {
	test := paintOf(t, `<div id="o"><div id="i"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow: hidden; background: red; display: table-cell }
		#i { width: 50px; height: 50px; margin: 25px; background: green; border-radius: 100%;
			outline: solid green 100px; outline-offset: -1px }`)
	ref := paintOf(t, `<div></div>`, noDefaults+`div { width: 100px; height: 100px; background: green }`)
	if !pictureEqual(test, ref, picPage) {
		t.Errorf("the outlined circle does not cover its box: %v", test)
	}
}

// TestOutlineOffsetInChIsMeasuredFromTheFace: "ch" is the width of the box's
// face's zero, which layout measures and the painter cannot, so the offset is
// read in layout. A box one "ch" wide, laid out beside it, is the measure.
func TestOutlineOffsetInChIsMeasuredFromTheFace(t *testing.T) {
	const css = noDefaults + `div { font: 20px monospace } #m { width: 3ch; height: 1px }
		#a { width: 100px; height: 60px; outline: 4px solid blue; outline-offset: 3ch }`
	m := radiusFragment(t, `<div id="a"></div><div id="m"></div>`, css, "m")
	ch := m.BorderRect.W
	if ch <= 0 {
		t.Fatal("the measure is empty")
	}
	a := radiusFragment(t, `<div id="a"></div><div id="m"></div>`, css, "a")
	if a.outlineOffset != ch {
		t.Errorf("an outline-offset of 3ch is %v, want %v", a.outlineOffset, ch)
	}
	if fs := paintFindingsOf(t, `<div id="a"></div><div id="m"></div>`, css); hasRule(fs, RuleUnsupportedValue) {
		t.Errorf("a ch offset was reported: %v", fs)
	}
}

// TestTheMeetingOfRoundedPiecesIsCharged: whether the outlines of a broken
// box's rounded pieces meet is a comparison per nearby piece, which a document
// controls, and each is charged as the join's are: past the budget the pieces
// are drawn a ring each, and the budget says so.
func TestTheMeetingOfRoundedPiecesIsCharged(t *testing.T) {
	lowWork(t, 40*costOp)
	built := Build(Input{HTML: `<div id="c"><span id="s">` + strings.Repeat("bbbb ", 60) + `</span></div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#c { width: 30px; font: 10px/40px monospace }
			#s { border-radius: 6px; outline: 2px solid blue }`}}})
	rec := NewRecorder(nil)
	PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(10000)}, nil, rec), rec)
	requireCut(t, rec.Findings(), "the joining of outlines broken across lines past that point, drawn a ring per piece")
}
