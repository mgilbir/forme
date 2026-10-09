package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The evidence for TransformGroup, which is arithmetic for the reason
// transform_test.go gives, and a picture drawn by a renderer written here
// rather than by the engine: refRaster below draws a display list by taking
// each pixel of the page back through the matrices around a fill to the fill's
// own coordinates, which is the definition of drawing through a matrix and
// shares no code with the engine's mapping of rectangles. Every fixture is
// transformBox's: a box whose border box is (200, 100) to (300, 140), its
// centre, the initial origin, at (250, 120).

// groupPaint is transformPaint with Options.TransformGroups on.
func groupPaint(t *testing.T, htmlSrc, cssSrc string) ([]Op, []Finding) {
	t.Helper()
	in := Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}}
	built := Build(in)
	rec := NewRecorder(nil)
	root := layoutGrouping(built.Root, Size{W: A4.Content().W, H: rpx(10000)}, nil, rec, true)
	ops := PaintReporting(root, rec)
	return ops, append(built.Findings, rec.Findings()...)
}

// groupsIn is every TransformGroup in a list, outermost first.
func groupsIn(ops []Op) []TransformGroup {
	var out []TransformGroup
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			switch v := op.(type) {
			case TransformGroup:
				out = append(out, v)
				walk(v.Ops)
			case ClipPath:
				walk(v.Ops)
			case FilterGroup:
				walk(v.Ops)
			}
		}
	}
	walk(ops)
	return out
}

// oneGroup is the only TransformGroup of a list, failing if there is not one.
func oneGroup(t *testing.T, ops []Op) TransformGroup {
	t.Helper()
	gs := groupsIn(ops)
	if len(gs) != 1 {
		t.Fatalf("want one TransformGroup, found %d in %v", len(gs), ops)
	}
	return gs[0]
}

// mat is a matrix [a b c d e f] in pixels, the moves made layout units.
func mat(a, b, c, d, epx, fpx float64) [6]float64 {
	u := float64(unitsPerPx())
	return [6]float64{a, b, c, d, epx * u, fpx * u}
}

// mul is m·n, n acting first, written out again so that a test does not
// check the engine's multiplication with itself.
func mul(m, n [6]float64) [6]float64 {
	return [6]float64{
		m[0]*n[0] + m[2]*n[1],
		m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3],
		m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4],
		m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

// about is the matrix of a, b, c, d about an origin at (ox, oy) pixels: §6's
// T(o)·M·T(-o).
func about(a, b, c, d, ox, oy float64) [6]float64 {
	return mul(mul(mat(1, 0, 0, 1, ox, oy), mat(a, b, c, d, 0, 0)), mat(1, 0, 0, 1, -ox, -oy))
}

func sameMatrix(got, want [6]float64) bool {
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-6*math.Max(1, math.Abs(want[i])) {
			return false
		}
	}
	return true
}

// TestATransformNoRectangleSurvivesIsDrawnThroughItsMatrix: with the option
// on, each matrix #935 refuses for its angle is the group's, worked by hand,
// and what the group holds is the box exactly as it was painted without one.
func TestATransformNoRectangleSurvivesIsDrawnThroughItsMatrix(t *testing.T) {
	r3 := math.Sqrt(3) / 2
	h2 := math.Sqrt2 / 2
	for _, tc := range []struct {
		transform, origin string
		want              [6]float64
		bounds            Rect
	}{
		// About the centre (250, 120). The rectangle around the turned box
		// is the corners (±50, ±20) turned: ±(50·cos 30° + 20·sin 30°) =
		// ±53.30127 across and ±(50·sin 30° + 20·cos 30°) = ±42.32051 down,
		// in units 12588.72 to 19411.28 and 4971.49 to 10388.51, taken
		// outwards.
		{"rotate(30deg)", "", about(r3, 0.5, -0.5, r3, 250, 120),
			Rect{X: 12588, Y: 4971, W: 19412 - 12588, H: 10389 - 4971}},
		// skewX(45deg) is x' = x + y about the centre: (200, 100) goes to
		// (180, 100) and (300, 140) to (320, 140).
		{"skewX(45deg)", "", about(1, 0, 1, 1, 250, 120), xRect(180, 100, 140, 40)},
		// A mirror about the centre: the box lands on itself.
		{"scaleX(-1)", "", about(-1, 0, 0, 1, 250, 120), xRect(200, 100, 100, 40)},
		{"scaleY(-1)", "0 0", about(1, 0, 0, -1, 200, 100), xRect(200, 60, 100, 40)},
		// A list about an origin that is neither a corner nor the centre,
		// (210, 130): rotate(90deg)·scaleX(-1) is matrix(0, -1, -1, 0), a
		// reflection in the line y = -x, and the move goes after it. So e =
		// 210 + 10 − (0·210 − 1·130) = 350 and f = 130 + 20 − (−1·210 + 0·130)
		// = 360: (200, 100) is drawn at (−100 + 350, −200 + 360) = (250, 160)
		// and (300, 140) at (210, 60).
		{"translate(10px, 20px) rotate(90deg) scaleX(-1)", "10px 30px",
			mat(0, -1, -1, 0, 350, 360), xRect(210, 60, 40, 100)},
		// Percentages of the border box in the move, about the top left: 50%
		// of 100 and 25% of 40.
		{"translate(50%, 25%) rotate(45deg)", "0 0",
			mul(mat(1, 0, 0, 1, 50, 10), about(h2, h2, -h2, h2, 200, 100)), Rect{}},
		// And a turn of a quarter that is not exactly one: drawn through the
		// matrix rather than taken for the quarter turn it nearly is.
		{"matrix(0, 1, -1, 0.000001, 0, 0)", "", about(0, 1, -1, 0.000001, 250, 120), Rect{}},
	} {
		css := transformBox + `#a { transform: ` + tc.transform + ` }`
		if tc.origin != "" {
			css += `#a { transform-origin: ` + tc.origin + ` }`
		}
		ops, findings := groupPaint(t, `<div id="a"></div><div id="s"></div>`, css)
		if got := transformFindings(findings); len(got) > 0 {
			t.Errorf("%s: reported with the option on: %v", tc.transform, got)
		}
		g := oneGroup(t, ops)
		if !sameMatrix(g.Matrix, tc.want) {
			t.Errorf("%s about %q: the group's matrix is %v, want %v", tc.transform, tc.origin, g.Matrix, tc.want)
		}
		// What it holds is the box where layout put it, untransformed.
		if got := fillOf(t, g.Ops, green); got != xRect(200, 100, 100, 40) {
			t.Errorf("%s: the group holds the box at %v, want it untransformed", tc.transform, got)
		}
		if tc.bounds != (Rect{}) {
			if got := g.Extent(); got != tc.bounds {
				t.Errorf("%s: the group reaches %v, want %v", tc.transform, got, tc.bounds)
			}
		}
		// And the box after it is where it always was, outside the group.
		if got := fillOf(t, ops, blue); got != xRect(0, 140, A4.Content().W.Px(), 10) {
			t.Errorf("%s: the next box moved to %v", tc.transform, got)
		}
	}
}

// TestWithoutTheOptionNothingIsGrouped: the option is the backend's promise
// to draw the operation, so without it no list holds one and the transforms
// are reported as they were.
func TestWithoutTheOptionNothingIsGrouped(t *testing.T) {
	for _, tr := range []string{"rotate(30deg)", "skewX(45deg)", "scaleX(-1)"} {
		css := transformBox + `#a { transform: ` + tr + ` }`
		ops, findings := transformPaint(t, `<div id="a"></div>`, css)
		if gs := groupsIn(ops); len(gs) > 0 {
			t.Errorf("%s: a TransformGroup without the option: %v", tr, gs)
		}
		if len(transformFindings(findings)) != 1 {
			t.Errorf("%s: want the one finding, got %v", tr, transformFindings(findings))
		}
		c := Compose(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: css}}}, Options{})
		if gs := groupsIn(c.Ops); len(gs) > 0 {
			t.Errorf("%s: Compose without the option made a TransformGroup", tr)
		}
		c = Compose(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: css}}}, Options{TransformGroups: true})
		if gs := groupsIn(c.Ops); len(gs) != 1 {
			t.Errorf("%s: Compose with the option made %d TransformGroups, want 1", tr, len(gs))
		}
	}
}

// TestWhatAQuarterTurnCannotMapTheMatrixDraws: a quarter turn keeps the
// mapping it had, and what it refused — a picture turned, text stretched —
// is drawn through the same matrix instead.
func TestWhatAQuarterTurnCannotMapTheMatrixDraws(t *testing.T) {
	// Mapped, as without the option: no group.
	ops, _ := groupPaint(t, `<div id="a"></div>`, transformBox+`#a { transform: rotate(90deg) }`)
	if gs := groupsIn(ops); len(gs) > 0 {
		t.Errorf("a quarter turn of a box was grouped: %v", gs)
	}
	if got := fillOf(t, ops, green); got != xRect(230, 70, 40, 100) {
		t.Errorf("the quarter-turned box is at %v", got)
	}
	// Text stretched along one axis: refused without the option, grouped with it.
	css := transformBox + `#a { font: 20px Courier; transform: scale(2, 1) }`
	if _, f := transformPaint(t, `<div id="a">ab</div>`, css); len(transformFindings(f)) != 1 {
		t.Fatalf("stretched text was not refused without the option: %v", f)
	}
	ops, f := groupPaint(t, `<div id="a">ab</div>`, css)
	if len(transformFindings(f)) != 0 {
		t.Errorf("stretched text was reported with the option: %v", transformFindings(f))
	}
	g := oneGroup(t, ops)
	if want := about(2, 0, 0, 1, 250, 120); !sameMatrix(g.Matrix, want) {
		t.Errorf("the stretched box's group has %v, want %v", g.Matrix, want)
	}
	if run := onlyText(t, g.Ops); run.Size != rpx(20) {
		t.Errorf("the run inside the group is %v, want it at the size it was set at", run.Size)
	}
}

// TestStillRefused: what is refused for a reason other than its angle stays
// refused with the option on — a 3D function, the root — and a matrix with
// no inverse still draws nothing.
func TestStillRefused(t *testing.T) {
	for _, tc := range []struct{ doc, css string }{
		{`<div id="a"></div>`, transformBox + `#a { transform: rotateX(10deg) }`},
		{`<div id="a"></div>`, transformBox + `#a { transform: perspective(100px) rotate(30deg) }`},
		{`<div id="a"></div>`, transformBox + `html { transform: rotate(30deg) }`},
		{`<table><tr><td id="a">x</td></tr></table>`, noDefaults + `#a { transform: rotate(30deg) }`},
	} {
		ops, findings := groupPaint(t, tc.doc, tc.css)
		if gs := groupsIn(ops); len(gs) > 0 {
			t.Errorf("%s: grouped: %v", tc.css, gs)
		}
		if len(transformFindings(findings)) != 1 {
			t.Errorf("%s: want one finding, got %v", tc.css, transformFindings(findings))
		}
	}
	ops, _ := groupPaint(t, `<div id="a"></div>`, transformBox+`#a { transform: skewX(30deg) scale(0) }`)
	for _, op := range ops {
		if v, ok := op.(FillRect); ok && v.Color == green {
			t.Error("a box with no inverse was drawn")
		}
	}
	if gs := groupsIn(ops); len(gs) > 0 {
		t.Errorf("a box with no inverse was grouped: %v", gs)
	}
}

// TestNestedTransformsMultiply: a group inside a quarter turn takes the turn
// into its matrix, the turn acting last; two groups nest, each with its own.
func TestNestedTransformsMultiply(t *testing.T) {
	r3 := math.Sqrt(3) / 2
	doc := `<div id="a"><div id="i"></div></div>`
	// #i is 50 by 20 at (200, 100), its centre (225, 110).
	base := transformBox + `#i { width: 50px; height: 20px; background: blue }`
	inner := about(r3, 0.5, -0.5, r3, 225, 110)

	ops, _ := groupPaint(t, doc, base+`#a { transform: rotate(90deg) } #i { transform: rotate(30deg) }`)
	g := oneGroup(t, ops)
	if want := mul(about(0, 1, -1, 0, 250, 120), inner); !sameMatrix(g.Matrix, want) {
		t.Errorf("a turned group inside a quarter turn has %v, want the turn after it, %v", g.Matrix, want)
	}

	ops, _ = groupPaint(t, doc, base+`#a { transform: skewX(30deg) } #i { transform: rotate(30deg) }`)
	gs := groupsIn(ops)
	if len(gs) != 2 {
		t.Fatalf("want two groups, one inside the other: %v", gs)
	}
	if want := about(1, 0, math.Tan(math.Pi/6), 1, 250, 120); !sameMatrix(gs[0].Matrix, want) {
		t.Errorf("the outer group has %v, want %v", gs[0].Matrix, want)
	}
	if !sameMatrix(gs[1].Matrix, inner) {
		t.Errorf("the inner group has %v, want its own %v", gs[1].Matrix, inner)
	}
	if fillOf(t, gs[1].Ops, blue) != xRect(200, 100, 50, 20) {
		t.Error("the inner group does not hold its box untransformed")
	}
}

// refRaster draws a display list of fills inside TransformGroups on a grid of
// pixel centres, and says which colour is on top at each pixel it reaches. It
// works backwards: each pixel is taken through the inverse of every matrix
// around a fill, to the fill's own coordinates, and asked whether it is
// inside. A group's clip is asked in the coordinates outside the group.
func refRaster(t *testing.T, ops []Op) map[[2]int]style.RGBA {
	t.Helper()
	out := map[[2]int]style.RGBA{}
	u := float64(unitsPerPx())
	type test func(x, y float64) bool
	inverse := func(m [6]float64) func(x, y float64) (float64, float64) {
		det := m[0]*m[3] - m[1]*m[2]
		return func(x, y float64) (float64, float64) {
			x, y = x-m[4], y-m[5]
			return (m[3]*x - m[2]*y) / det, (-m[1]*x + m[0]*y) / det
		}
	}
	inside := func(r Rect, x, y float64) bool {
		return x >= float64(r.X) && x < float64(r.Right()) && y >= float64(r.Y) && y < float64(r.Bottom())
	}
	var draw func(ops []Op, m [6]float64, clips []test)
	draw = func(ops []Op, m [6]float64, clips []test) {
		back := inverse(m)
		for _, op := range ops {
			switch v := op.(type) {
			case FillRect:
				// The pixels to try: the rectangle around the drawn corners.
				x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
				for _, p := range [][2]float64{{float64(v.Rect.X), float64(v.Rect.Y)},
					{float64(v.Rect.Right()), float64(v.Rect.Y)}, {float64(v.Rect.X), float64(v.Rect.Bottom())},
					{float64(v.Rect.Right()), float64(v.Rect.Bottom())}} {
					x := m[0]*p[0] + m[2]*p[1] + m[4]
					y := m[1]*p[0] + m[3]*p[1] + m[5]
					x0, x1, y0, y1 = math.Min(x0, x), math.Max(x1, x), math.Min(y0, y), math.Max(y1, y)
				}
				for py := int(math.Floor(y0/u)) - 1; py <= int(math.Ceil(y1/u)); py++ {
					for px := int(math.Floor(x0/u)) - 1; px <= int(math.Ceil(x1/u)); px++ {
						cx, cy := (float64(px)+0.5)*u, (float64(py)+0.5)*u
						lx, ly := back(cx, cy)
						if !inside(v.Rect, lx, ly) {
							continue
						}
						ok := true
						for _, c := range clips {
							ok = ok && c(cx, cy)
						}
						if ok {
							out[[2]int{px, py}] = v.Color
						}
					}
				}
			case TransformGroup:
				cs := clips
				if v.Clip.Active {
					r := v.Clip.Rect
					cs = append(append([]test(nil), clips...), func(x, y float64) bool {
						lx, ly := back(x, y)
						return inside(r, lx, ly)
					})
				}
				draw(v.Ops, mul(m, v.Matrix), cs)
			case Link:
			default:
				t.Fatalf("refRaster draws no %T", op)
			}
		}
	}
	draw(ops, [6]float64{1, 0, 0, 1, 0, 0}, nil)
	return out
}

// TestTheGroupLandsWhereTheMatrixSays: drawn by refRaster, a box inside a
// group is where the matrix puts it. A matrix a millionth from a quarter turn
// is drawn through a group, and the same quarter turn exactly is drawn by
// moving the operations: the two pictures are the same, pixel for pixel, and
// both reach the pixels worked out by hand.
func TestTheGroupLandsWhereTheMatrixSays(t *testing.T) {
	doc := `<div id="a"><div id="i"></div></div>`
	css := transformBox + `#a { border-left: 10px solid red; border-top: 4px solid blue }
		#i { width: 30px; height: 10px; background: rgb(0, 0, 0) }`
	for _, tc := range []struct{ exact, near string }{
		{"rotate(90deg)", "matrix(0, 1, -1, 0.000001, 0, 0)"},
		{"rotate(-90deg) translate(7px, 3px)", "matrix(0, -1, 1, 0.000001, 0, 0) translate(7px, 3px)"},
		{"rotate(180deg)", "matrix(-1, 0.000001, 0, -1, 0, 0)"},
	} {
		exact, _ := groupPaint(t, doc, css+`#a { transform: `+tc.exact+` }`)
		if len(groupsIn(exact)) > 0 {
			t.Fatalf("%s was grouped", tc.exact)
		}
		near, _ := groupPaint(t, doc, css+`#a { transform: `+tc.near+` }`)
		if len(groupsIn(near)) != 1 {
			t.Fatalf("%s was not grouped", tc.near)
		}
		a, b := refRaster(t, exact), refRaster(t, near)
		if len(a) == 0 || len(a) != len(b) {
			t.Errorf("%s: %d pixels drawn, %s: %d", tc.exact, len(a), tc.near, len(b))
		}
		for p, c := range a {
			if b[p] != c {
				t.Errorf("%s: pixel %v is %v moved and %v through the matrix", tc.exact, p, c, b[p])
				break
			}
		}
	}

	// rotate(90deg) by hand: with its borders the box is 110 by 44 from
	// (200, 100), its centre (255, 122), and a point (x, y) is drawn at (255
	// − (y − 122), 122 + (x − 255)). So the red left border, x from 200 to
	// 210, is turned to y from 67 to 77, across x from 233 to 277.
	ops, _ := groupPaint(t, doc, css+`#a { transform: matrix(0, 1, -1, 0.000001, 0, 0) }`)
	pic := refRaster(t, ops)
	if c := pic[[2]int{250, 75}]; c != red {
		t.Errorf("the turned left border is not at (250, 75): %v", c)
	}
	// A mirror by hand: the border box is 110 wide with the border, from x =
	// 200 to 310, and scaleX(-1) about its centre puts the left border on the
	// right, x from 300 to 310.
	ops, _ = groupPaint(t, doc, css+`#a { transform: scaleX(-1) }`)
	pic = refRaster(t, ops)
	if pic[[2]int{305, 120}] != red || pic[[2]int{205, 120}] == red {
		t.Errorf("the mirrored left border is at the right %v, at the left %v", pic[[2]int{305, 120}], pic[[2]int{205, 120}])
	}
	// rotate(30deg) by hand: the local point (205, 105), (-45, -15) from the
	// centre, is drawn at (250 - 45·cos30 + 15·sin30, 120 - 45·sin30 -
	// 15·cos30) = (218.53, 84.51); and the corner pixel (201, 101), which the
	// unturned box covers, is outside the turned one.
	ops, _ = groupPaint(t, `<div id="a"></div>`, transformBox+`#a { transform: rotate(30deg) }`)
	pic = refRaster(t, ops)
	if pic[[2]int{218, 84}] != green {
		t.Errorf("(218, 84) is %v, want the turned box", pic[[2]int{218, 84}])
	}
	if c, ok := pic[[2]int{201, 101}]; ok {
		t.Errorf("(201, 101), outside the turned box, is %v", c)
	}
}

// TestTheNaturalSizeIsOfWhereTheGroupDraws: the page is scaled to fit the box
// as drawn, nested matrices multiplied with the inner acting first. The
// oracle is refRaster's picture: the far edge of every pixel it draws is
// within a pixel of the natural size.
func TestTheNaturalSizeIsOfWhereTheGroupDraws(t *testing.T) {
	for _, css := range []string{
		`#a { transform: rotate(30deg) }`,
		`#a { transform: rotate(45deg); transform-origin: 0 0 }`,
		`#a { transform: skewX(60deg) }`,
		// An outer group with a box scaled along x inside it: the scale is
		// mapped as a quarter turn and the turn after it is a group, and
		// R·S is not S·R.
		`#a { transform: rotate(60deg); transform-origin: 0 0 } #i { transform: scaleX(3); transform-origin: 0 0 }`,
		`#a { transform: skewY(40deg); transform-origin: 0 0 } #i { transform: rotate(70deg); transform-origin: 0 0 }`,
	} {
		// The root narrower than everything, so that the natural width is
		// what the boxes reach and not the page's.
		in := Input{HTML: `<div id="a"><div id="i"></div></div>`, CSS: []Stylesheet{{Source: transformBox +
			`html, body { width: 10px } #i { width: 60px; height: 10px; background: blue }` + css}}}
		c := Compose(in, Options{Page: PageSize{Width: rpx(2000), Height: rpx(2000)}, TransformGroups: true})
		if len(transformFindings(c.Findings)) > 0 {
			t.Fatalf("%s: %v", css, c.Findings)
		}
		var w, h int
		for p := range refRaster(t, c.Ops) {
			w, h = max(w, p[0]+1), max(h, p[1]+1)
		}
		if d := math.Abs(c.NaturalSize.W.Px() - float64(w)); d > 1 {
			t.Errorf("%s: the natural width is %v, the picture reaches %d", css, c.NaturalSize.W.Px(), w)
		}
		if d := math.Abs(c.NaturalSize.H.Px() - float64(h)); d > 1 {
			t.Errorf("%s: the natural height is %v, the picture reaches %d", css, c.NaturalSize.H.Px(), h)
		}
		for _, f := range c.Findings {
			if f.Rule == RuleOverflowPage {
				t.Errorf("%s: the overflow guard fired: %v", css, f.Message)
			}
		}
	}
}

// TestALinkInAGroupIsTheRectangleAroundIt: a link annotation is a rectangle
// of the page, which no matrix moves, so a link inside a turned box is put
// ahead of the group with its area the rectangle around where it is drawn.
func TestALinkInAGroupIsTheRectangleAroundIt(t *testing.T) {
	ops, _ := groupPaint(t, `<div id="a"><a href="https://example.com/" style="display: block; height: 40px"></a></div>`,
		transformBox+`#a { transform: rotate(30deg) }`)
	var links []Link
	for _, op := range ops {
		if l, ok := op.(Link); ok {
			links = append(links, l)
		}
	}
	for _, g := range groupsIn(ops) {
		for _, op := range g.Ops {
			if _, ok := op.(Link); ok {
				t.Error("a Link is inside a TransformGroup")
			}
		}
	}
	// The <a> fills #a, so its area is the group's reach, worked by hand in
	// TestATransformNoRectangleSurvivesIsDrawnThroughItsMatrix.
	want := Rect{X: 12588, Y: 4971, W: 19412 - 12588, H: 10389 - 4971}
	if len(links) != 1 || len(links[0].Rects) != 1 || links[0].Rects[0] != want {
		t.Errorf("the link's areas are %v, want %v", links, want)
	}
}

// TestClipsAroundAndInsideAGroup: the clip of a box around the transformed
// one cuts it where it is drawn, so it is on the group; the box's own
// overflow clip moves with it, so it is on what the group holds.
func TestClipsAroundAndInsideAGroup(t *testing.T) {
	doc := `<div id="c"><div id="a"><div id="w"></div></div></div>`
	css := transformBox + `#c { overflow: hidden; width: 280px; height: 300px }
		#a { overflow: hidden; transform: rotate(30deg) }
		#w { width: 500px; height: 10px; background: blue }`
	ops, _ := groupPaint(t, doc, css)
	g := oneGroup(t, ops)
	if !g.Clip.Active || g.Clip.Rect != xRect(0, 0, 280, 300) {
		t.Errorf("the group's clip is %v, want #c's padding box", g.Clip)
	}
	if got := fillOf(t, g.Ops, blue); got != xRect(200, 100, 100, 10) {
		t.Errorf("the wide child is %v inside the group, want it cut to #a, untransformed", got)
	}
}

// TestAGroupsMarksAreWhereItDraws: an opacity group sees marks inside a
// TransformGroup where the matrix draws them, by the rectangle around them.
func TestAGroupsMarksAreWhereItDraws(t *testing.T) {
	doc := `<div id="g"><div id="a"></div><div id="b"></div></div>`
	css := noDefaults + `#g { opacity: 0.5 } #a, #b { width: 100px; height: 40px; background: rgb(0, 128, 0) }`
	overlap := func(extra string) bool {
		_, findings := groupPaint(t, doc, css+extra)
		for _, f := range findings {
			if f.Property == "opacity" && strings.Contains(f.Message, "lie over each other") {
				return true
			}
		}
		return false
	}
	if overlap(`#b { transform: rotate(5deg) scale(0.5) }`) {
		t.Error("a box turned and halved in place was reported over its sibling above")
	}
	if !overlap(`#b { transform: rotate(30deg) }`) {
		t.Error("a box turned over its sibling inside one translucent group was not reported")
	}

	// dimOps itself, on a group: what it holds dimmed, and its mark moved.
	g := newTransformGroup(affine{b: 1, c: -1, e: float64(rpx(100))}, []Op{FillRect{Rect: xRect(0, 0, 10, 20), Color: green}})
	kept, marks := dimOps([]Op{g}, 0, 0.5)
	if len(kept) != 1 || fillOf(t, kept[0].(TransformGroup).Ops, style.RGBA{G: 128, A: 0.5}) != xRect(0, 0, 10, 20) {
		t.Errorf("the group was dimmed to %v", kept)
	}
	if len(marks) != 1 || marks[0].rect != xRect(80, 0, 20, 10) {
		t.Errorf("the group's marks are %v, want the turned rectangle", marks)
	}
}

// TestTheFontFloorIsTheSizeAcrossTheLine: under a matrix, the size text is
// drawn at is the em measured across the line as drawn — halved by scaleY,
// left by scaleX and by a skew along the line.
func TestTheFontFloorIsTheSizeAcrossTheLine(t *testing.T) {
	small := func(css string) bool {
		c := Compose(Input{HTML: `<div id="a">ab</div>`, CSS: []Stylesheet{{Source: transformBox + `#a { font: 20px Courier }` + css}}},
			Options{TransformGroups: true})
		if len(transformFindings(c.Findings)) > 0 {
			t.Fatalf("%s: %v", css, transformFindings(c.Findings))
		}
		for _, f := range c.Findings {
			if f.Rule == RuleMinFontSize {
				return true
			}
		}
		return false
	}
	// 20px is 15pt; the floor is 6pt, so a quarter of it, 3.75pt, is under it.
	for css, want := range map[string]bool{
		`#a { transform: scaleY(0.25) }`:               true,
		`#a { transform: scaleX(0.25) }`:               false,
		`#a { transform: skewX(60deg) }`:               false,
		`#a { transform: rotate(30deg) scale(0.25) }`:  true,
		`#a { transform: rotate(30deg) scaleX(0.25) }`: false,
		`#a { transform: rotate(30deg) scaleY(0.25) }`: true,
		// A vertical line runs down the box, so the size across it is along x.
		`#a { writing-mode: vertical-rl; transform: scaleX(0.25) }`: true,
		`#a { writing-mode: vertical-rl; transform: scaleY(0.25) }`: false,
	} {
		if got := small(css); got != want {
			t.Errorf("%s: reported too small %v, want %v", css, got, want)
		}
	}
}

// TestAPictureTurnsInAGroup: a picture is drawn through the matrix like
// anything else, and so is a gradient at a slant under an uneven scale,
// which a quarter turn refused.
func TestAPictureTurnsInAGroup(t *testing.T) {
	ops, findings := groupPaint(t, `<div id="a"><img style="display: block; width: 20px; height: 10px" src="data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw=="></div>`,
		transformBox+`#a { transform: rotate(90deg) }`)
	if len(transformFindings(findings)) > 0 {
		t.Fatalf("a turned picture was reported with the option: %v", transformFindings(findings))
	}
	g := oneGroup(t, ops)
	found := false
	for _, op := range g.Ops {
		if v, ok := op.(DrawImage); ok && v.Rect == xRect(200, 100, 20, 10) {
			found = true
		}
	}
	if !found {
		t.Errorf("the picture is not in the group, untransformed: %v", g.Ops)
	}
	_, findings = groupPaint(t, `<div id="a"></div>`,
		transformBox+`#a { background: linear-gradient(45deg, red, blue); transform: scale(2, 1) }`)
	if len(transformFindings(findings)) > 0 {
		t.Errorf("a slanted gradient stretched was reported with the option: %v", transformFindings(findings))
	}
}

// TestAColourMatrixIsFoldedThroughAGroup: a filter's colour matrix commutes
// with a matrix that only moves ink, so it is folded into the colours inside
// the group rather than left in a group of its own for the backend.
func TestAColourMatrixIsFoldedThroughAGroup(t *testing.T) {
	ops, _ := groupPaint(t, `<div id="f"><div id="a"></div></div>`,
		transformBox+`#f { filter: grayscale(1) } #a { transform: rotate(30deg) }`)
	for _, op := range ops {
		if _, ok := op.(FilterGroup); ok {
			t.Fatalf("the grayscale was left in a FilterGroup: %v", ops)
		}
	}
	g := oneGroup(t, ops)
	for _, op := range g.Ops {
		if v, ok := op.(FillRect); ok && (v.Color.R != v.Color.G || v.Color.G != v.Color.B) {
			t.Errorf("the fill inside the group is %v, not grey", v.Color)
		}
	}
}

// TestTheOverflowGuardReadsAGroupWhereItDraws: the guard measures what a
// group holds through its matrix and its clip.
func TestTheOverflowGuardReadsAGroupWhereItDraws(t *testing.T) {
	fired := func(ops []Op) bool {
		rec := NewRecorder(nil)
		checkPageOverflow(rec, ops, Size{W: rpx(100), H: rpx(100)}, 1)
		return len(rec.Findings()) > 0
	}
	fill := []Op{FillRect{Rect: xRect(150, 10, 20, 20), Color: green}}
	// Off the page untransformed, and moved on by the group.
	g := newTransformGroup(translation(float64(rpx(-100)), 0), fill)
	if fired([]Op{g}) {
		t.Error("a fill the group moves onto the page was taken for one off it")
	}
	g = newTransformGroup(translation(float64(rpx(100)), 0), []Op{FillRect{Rect: xRect(10, 10, 20, 20), Color: green}})
	if !fired([]Op{g}) {
		t.Error("a fill the group moves off the page was not reported")
	}
	if n, _ := countOpsUpTo([]Op{g}, 100); n != 2 {
		t.Errorf("a group of one fill counts as %d operations, want 2", n)
	}
}

// TestAGroupIsChargedToTheDocument: a TransformGroup is an operation a
// backend draws, and is charged as one, as a ClipPath is: the paint of a
// turned box costs one operation more than the paint of the same box
// unturned. It copies nothing, so a stack of them is linear in what they
// hold. And past the budget the group is not made: the box is drawn where
// layout put it, and the budget, not the transform, reports it.
func TestAGroupIsChargedToTheDocument(t *testing.T) {
	spend := func(css string, left int64) ([]Op, []Finding, int64) {
		built := Build(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: transformBox + css}}})
		rec := NewRecorder(nil)
		root := layoutGrouping(built.Root, Size{W: A4.Content().W, H: rpx(10000)}, nil, rec, true)
		if left >= 0 {
			rec.work.left = left
		}
		before := rec.work.left
		ops := PaintReporting(root, rec)
		return ops, rec.Findings(), before - rec.work.left
	}
	_, _, plain := spend(``, -1)
	ops, _, turned := spend(`#a { transform: rotate(30deg) }`, -1)
	if len(groupsIn(ops)) != 1 {
		t.Fatal("the turned box was not grouped")
	}
	if turned-plain != costOp {
		t.Errorf("the turned box cost %d more than the plain one, want one operation, %d", turned-plain, costOp)
	}
	// With enough for the marks and not the group.
	ops, findings, _ := spend(`#a { transform: rotate(30deg) }`, plain)
	if len(groupsIn(ops)) != 0 {
		t.Error("a group was made past the budget")
	}
	requireCut(t, findings, "the transforms past that point, which were drawn untransformed")
	if got := fillOf(t, ops, green); got != xRect(200, 100, 100, 40) {
		t.Errorf("the box refused its group is drawn at %v, want where layout put it", got)
	}
	if got := transformFindings(findings); len(got) > 0 {
		t.Errorf("the budget's cut was reported as the transform's: %v", got)
	}
}
