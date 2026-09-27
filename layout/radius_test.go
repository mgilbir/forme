package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Rounded corners, CSS Backgrounds 3 §4.

func rpx(v float64) style.Unit { u, _ := style.FromPx(v); return u }

func ptPx(x, y float64) Point { return Point{X: rpx(x), Y: rpx(y)} }

// radiusFragment lays out a document and returns the fragment of one element.
func radiusFragment(t *testing.T, htmlSrc, cssSrc, id string) *Fragment {
	t.Helper()
	root := layoutOf(t, A4.Content().W.Px(), htmlSrc, cssSrc)
	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found != nil || f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil && attrIs(f.Box, id) && f.Box.Pseudo == "" {
			found = f
			return
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no fragment for #%s", id)
	}
	return found
}

func attrIs(b *Box, id string) bool {
	if b == nil || b.Element == nil {
		return false
	}
	v, ok := b.Element.Attr("id")
	return ok && v == id
}

func corner(x, y float64) Corner { return Corner{X: rpx(x), Y: rpx(y)} }

// TestTheOverlappingCurvesExample is §4.5's example: "width: 6em; height: 2em;
// border-radius: 0.5em 2em 0.5em 2em" with border-box sizing, where "all
// corners need to be reduced by a factor 0.8", and the same at 2.5em, where
// nothing is.
func TestTheOverlappingCurvesExample(t *testing.T) {
	const css = noDefaults + `div { box-sizing: border-box; width: 6em; font-size: 10px;
		border-radius: 0.5em 2em 0.5em 2em } #b { height: 2em } #a { height: 2.5em }`
	a := radiusFragment(t, `<div id="a"></div><div id="b"></div>`, css, "a")
	if want := (Radii{corner(5, 5), corner(20, 20), corner(5, 5), corner(20, 20)}); a.radii != want {
		t.Errorf("at 2.5em the radii are %+v, want %+v", a.radii, want)
	}
	b := radiusFragment(t, `<div id="a"></div><div id="b"></div>`, css, "b")
	if want := (Radii{corner(4, 4), corner(16, 16), corner(4, 4), corner(16, 16)}); b.radii != want {
		t.Errorf("at 2em the radii are %+v, want %+v", b.radii, want)
	}
}

// TestARadiusPercentageIsOfItsOwnAxis: "Percentages for the horizontal radius
// refer to the width of the border box, whereas percentages for the vertical
// radius refer to the height of the border box."
func TestARadiusPercentageIsOfItsOwnAxis(t *testing.T) {
	f := radiusFragment(t, `<div id="a"></div>`, noDefaults+`#a { width: 190px; height: 90px;
		border: 5px solid; border-radius: 50% / 25% }`, "a")
	if want := corner(100, 25); f.radii.TopLeft != want || f.radii.BottomRight != want {
		t.Errorf("the radii are %+v, want 100 by 25 at every corner", f.radii)
	}
	// And a corner with one radius of nothing is square.
	f = radiusFragment(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 90px;
		border-top-left-radius: 10px 0 }`, "a")
	if !f.radii.IsZero() {
		t.Errorf("a corner of 10px by 0 is %+v, want square", f.radii)
	}
}

// TestTheInnerRadiusIsTheOuterLessTheBorder is §4.2, including the case where
// the border is thicker than the radius and the padding edge's corner is square.
func TestTheInnerRadiusIsTheOuterLessTheBorder(t *testing.T) {
	outer := Radii{corner(10, 10), corner(20, 30), corner(4, 4), corner(0, 0)}
	e := Edges{Top: rpx(5), Right: rpx(8), Bottom: rpx(2), Left: rpx(15)}
	got := insetRadii(outer, e)
	// The bottom right loses its horizontal radius to the eight-pixel right
	// border, and a corner with one radius of nothing is square.
	want := Radii{Corner{}, corner(12, 25), Corner{}, Corner{}}
	if got != want {
		t.Errorf("the inner radii are %+v, want %+v", got, want)
	}
}

// TestATableRoundsOnlyWhereItsBordersAreSeparate is §4.6.
func TestATableRoundsOnlyWhereItsBordersAreSeparate(t *testing.T) {
	const doc = `<table id="t"><tr id="r"><td id="c">x</td></tr></table>`
	for _, tc := range []struct {
		css, id string
		round   bool
	}{
		{`#t, #r, #c { border-radius: 5px; border: 1px solid }`, "t", true},
		{`#t, #r, #c { border-radius: 5px; border: 1px solid }`, "c", true},
		{`#t, #r, #c { border-radius: 5px; border: 1px solid }`, "r", false},
		{`#t, #r, #c { border-radius: 5px; border: 1px solid } #t { border-collapse: collapse }`, "t", false},
		{`#t, #r, #c { border-radius: 5px; border: 1px solid } #t { border-collapse: collapse }`, "c", false},
	} {
		f := radiusFragment(t, doc, noDefaults+tc.css, tc.id)
		if got := !f.radii.IsZero(); got != tc.round {
			t.Errorf("%s with %s: rounded=%v, want %v", tc.id, tc.css, got, tc.round)
		}
	}
}

// TestABrokenInlineBoxIsRoundedAtItsEnds is §8.6's slice model: one box cut
// into pieces, so the first piece has the start corners and the last the end
// ones, and nothing between.
func TestABrokenInlineBoxIsRoundedAtItsEnds(t *testing.T) {
	root := layoutOf(t, 100, `<p>aaa <span id="s">bbb ccc ddd eee</span></p>`,
		noDefaults+`p { width: 60px; font: 10px/20px monospace } #s { border-radius: 4px; background: red }`)
	var pieces []*Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		for _, l := range f.Lines {
			for _, b := range l.Boxes {
				if attrIs(b.Box, "s") {
					pieces = append(pieces, b)
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if len(pieces) < 3 {
		t.Fatalf("the span is in %d pieces, want at least three", len(pieces))
	}
	r := corner(4, 4)
	if want := (Radii{TopLeft: r, BottomLeft: r}); pieces[0].radii != want {
		t.Errorf("the first piece is %+v, want its left corners", pieces[0].radii)
	}
	for _, mid := range pieces[1 : len(pieces)-1] {
		if !mid.radii.IsZero() {
			t.Errorf("a middle piece is %+v, want square", mid.radii)
		}
	}
	if want := (Radii{TopRight: r, BottomRight: r}); pieces[len(pieces)-1].radii != want {
		t.Errorf("the last piece is %+v, want its right corners", pieces[len(pieces)-1].radii)
	}
}

// pathsOf is every FillPath in a display list, including inside groups.
func pathsOf(ops []Op) []FillPath {
	var out []FillPath
	for _, op := range ops {
		switch v := op.(type) {
		case FillPath:
			out = append(out, v)
		case ClipPath:
			out = append(out, pathsOf(v.Ops)...)
		}
	}
	return out
}

func groupsOf(ops []Op) []ClipPath {
	var out []ClipPath
	for _, op := range ops {
		if v, ok := op.(ClipPath); ok {
			out = append(out, v)
			out = append(out, groupsOf(v.Ops)...)
		}
	}
	return out
}

// TestARoundedBackgroundIsItsShape: the colour is the rounded border box, and
// the corner outside the curve is not in it.
func TestARoundedBackgroundIsItsShape(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px;
		background: red; border-radius: 20px }`)
	ps := pathsOf(ops)
	if len(ps) != 1 || ps[0].Color != red {
		t.Fatalf("%d paths, want one red one: %v", len(ps), ops)
	}
	p := ps[0].Path
	for _, tc := range []struct {
		x, y float64
		in   bool
	}{
		{50, 30, true}, {1, 1, false}, {99, 1, false}, {99, 59, false}, {1, 59, false},
		// On the corner's diagonal, just inside and just outside the curve:
		// the circle of radius 20 about (20, 20) meets it at 20 - 20/√2.
		{20 - 20/math.Sqrt2 + 0.3, 20 - 20/math.Sqrt2 + 0.3, true},
		{20 - 20/math.Sqrt2 - 0.3, 20 - 20/math.Sqrt2 - 0.3, false},
		{0.5, 30, true}, {50, 0.5, true},
	} {
		if got := p.Contains(ptPx(tc.x, tc.y)); got != tc.in {
			t.Errorf("(%g, %g): inside=%v, want %v", tc.x, tc.y, got, tc.in)
		}
	}
	if b := p.Bounds(); b != (Rect{W: rpx(100), H: rpx(60)}) {
		t.Errorf("the shape's bounds are %v", b)
	}
	// And the same box with square corners paints the rectangle it always did.
	ops = paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px;
		background: red; border-radius: 0 }`)
	if len(pathsOf(ops)) != 0 {
		t.Error("a box with square corners painted a path")
	}
}

// TestABackgroundIsClippedToTheCurveOfItsClipBox: background-clip picks the
// edge, and its curve is §4.2's inner one.
func TestABackgroundIsClippedToTheCurveOfItsClipBox(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px;
		border: 10px solid transparent; background: red; background-clip: padding-box;
		border-radius: 30px 5px }`)
	ps := pathsOf(ops)
	if len(ps) != 1 {
		t.Fatalf("%d paths", len(ps))
	}
	want := roundedRect(Rect{X: rpx(10), Y: rpx(10), W: rpx(100), H: rpx(60)},
		Radii{corner(20, 20), Corner{}, corner(20, 20), Corner{}})
	if ps[0].Path.String() != want.String() {
		t.Errorf("the background is %s, want %s", ps[0].Path, want)
	}
}

// TestARoundedBorderIsARing: one colour and one style all round is one shape,
// the band between the two curves.
func TestARoundedBorderIsARing(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px;
		border: 10px solid blue; border-radius: 20px }`)
	ps := pathsOf(ops)
	if len(ps) != 1 || ps[0].Color != blue {
		t.Fatalf("%d paths, want one blue ring: %v", len(ps), ops)
	}
	p := ps[0].Path
	for _, tc := range []struct {
		x, y float64
		in   bool
	}{
		{60, 5, true}, {60, 15, false}, {5, 40, true}, {60, 40, false},
		{1, 1, false},
		// The inner curve has radius 10 about (20, 20): a point at the
		// corner of the padding box is outside it, so in the ring.
		{11, 11, true},
	} {
		if got := p.Contains(ptPx(tc.x, tc.y)); got != tc.in {
			t.Errorf("(%g, %g): in the ring=%v, want %v", tc.x, tc.y, got, tc.in)
		}
	}
}

// TestRoundedBorderColoursMeetOnTheCurve is §4.4: each side its own colour,
// meeting at 45 degrees where the widths are equal, and a side of no width
// giving the whole corner to its neighbour.
func TestRoundedBorderColoursMeetOnTheCurve(t *testing.T) {
	colourAt := func(ops []Op, x, y float64) (style.RGBA, int) {
		n := 0
		var c style.RGBA
		for _, p := range pathsOf(ops) {
			if p.Path.Contains(ptPx(x, y)) {
				c, n = p.Color, n+1
			}
		}
		return c, n
	}
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 100px;
		border: 10px solid blue; border-top-color: red; border-radius: 30px }`)
	for _, tc := range []struct {
		x, y float64
		want style.RGBA
	}{
		{60, 5, red}, {5, 60, blue},
		// Along the top left corner's 45 degree line, the ring runs from 30 -
		// 30/√2 ≈ 8.8 to 30 - 20/√2 ≈ 15.9 in both coordinates. Just above the
		// line is the top's, just below it the left's.
		{12, 11, red}, {11, 12, blue},
	} {
		c, n := colourAt(ops, tc.x, tc.y)
		if n != 1 || c != tc.want {
			t.Errorf("(%g, %g) is in %d sides, colour %v, want one %v", tc.x, tc.y, n, c, tc.want)
		}
	}
	// No left border: the top has the whole corner.
	ops = paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 100px;
		border: 10px solid blue; border-left: none; border-top-color: red; border-radius: 30px }`)
	// At x=3 the ring runs from y≈16.9 (the outer curve, 30 by 30) to y≈21.3
	// (the inner, 30 by 20, since the top border is ten pixels).
	if c, n := colourAt(ops, 3, 19); n != 1 || c != red {
		t.Errorf("down the corner with no left border: %d sides, %v, want red", n, c)
	}
}

// TestADoubleRoundedBorderIsTwoRings: the bands paintEdge draws square, drawn
// between the curves a third and two thirds of the way in.
func TestADoubleRoundedBorderIsTwoRings(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 100px;
		border: 9px double blue; border-radius: 30px }`)
	ps := pathsOf(ops)
	if len(ps) != 2 {
		t.Fatalf("%d paths, want two rings", len(ps))
	}
	in := func(x, y float64) int {
		n := 0
		for _, p := range ps {
			if p.Path.Contains(ptPx(x, y)) {
				n++
			}
		}
		return n
	}
	if in(60, 1.5) != 1 || in(60, 4.5) != 0 || in(60, 7.5) != 1 {
		t.Errorf("the top is not two lines with a gap: %d %d %d", in(60, 1.5), in(60, 4.5), in(60, 7.5))
	}
}

// TestOverflowIsClippedToThePaddingCurve is §4.3: content of a box that clips
// on both axes is clipped to the curve of its padding edge. What lies clear of
// the corners is not wrapped at all, and a box that clips on one axis only is
// not curved.
func TestOverflowIsClippedToThePaddingCurve(t *testing.T) {
	ops := paintOf(t, `<div id="o"><div id="c"></div><div id="m"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow: hidden; border-radius: 40px; border: 5px solid transparent }
		#c { height: 20px; background: red }
		#m { margin: 20px 40px; height: 10px; background: blue }`)
	gs := groupsOf(ops)
	if len(gs) != 1 {
		t.Fatalf("%d groups, want the corner-reaching child in one: %v", len(gs), ops)
	}
	want := roundedRect(Rect{X: rpx(5), Y: rpx(5), W: rpx(100), H: rpx(100)},
		Radii{corner(35, 35), corner(35, 35), corner(35, 35), corner(35, 35)})
	if gs[0].Path.String() != want.String() {
		t.Errorf("the group clips to %s, want the padding curve %s", gs[0].Path, want)
	}
	if len(gs[0].Ops) != 1 || gs[0].Ops[0].(FillRect).Color != red {
		t.Errorf("the group holds %v, want the red child", gs[0].Ops)
	}
	for _, op := range ops {
		if f, ok := op.(FillRect); ok && f.Color == blue {
			goto blueOutside
		}
	}
	t.Error("the child clear of the corners is not painted outside the group")
blueOutside:
	ops = paintOf(t, `<div id="o"><div id="c"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow-x: clip; border-radius: 40px }
		#c { height: 20px; background: red }`)
	if len(groupsOf(ops)) != 0 {
		t.Error("a box clipping on one axis curved its content")
	}
}

// TestAReplacedElementIsClippedToItsContentCurve: "replaced element content to
// the curved content edge".
func TestAReplacedElementIsClippedToItsContentCurve(t *testing.T) {
	ops := paintOf(t, `<img id="i" src="data:image/gif;base64,R0lGODlhAQABAIAAAP8AAP///yH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==">`,
		noDefaults+`#i { width: 50px; height: 50px; padding: 5px; border-radius: 30px }`)
	gs := groupsOf(ops)
	if len(gs) != 1 {
		t.Fatalf("%d groups: %v", len(gs), ops)
	}
	want := roundedRect(Rect{X: rpx(5), Y: rpx(5), W: rpx(50), H: rpx(50)},
		Radii{corner(25, 25), corner(25, 25), corner(25, 25), corner(25, 25)})
	if gs[0].Path.String() != want.String() {
		t.Errorf("the picture clips to %s, want the content curve %s", gs[0].Path, want)
	}
}

// TestWhatIsRoundedSaysNothing: a rounded border in any style, and an outline
// round a rounded box, follow the curve (roundeddash.go, outlineshape.go) and
// say nothing. What is still drawn square is TestABrokenRoundedOutline...'s.
func TestWhatIsRoundedSaysNothing(t *testing.T) {
	for _, css := range []string{
		`border: 4px solid; border-radius: 20px`,
		`border: 4px dotted; border-radius: 20px`,
		`border: 4px dashed; border-radius: 20px`,
		`outline: 2px solid blue; border-radius: 20px`,
		`outline: 2px dotted blue; outline-offset: -4px; border-radius: 20px`,
	} {
		built := Build(Input{HTML: `<div id="a">x</div>`, CSS: []Stylesheet{{Source: noDefaults +
			`#a { width: 100px; height: 60px; ` + css + ` }`}}})
		rec := NewRecorder(nil)
		PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, rec), rec)
		if hasRule(rec.Findings(), RuleUnsupportedValue) || hasRule(rec.Findings(), RuleUnsupportedProperty) {
			t.Errorf("%s was reported: %v", css, rec.Findings())
		}
	}
}

// TestALinkIsNeverInsideAClip: a link is areas, not ink, and gatherLinks reads
// the top of the list.
func TestALinkIsNeverInsideAClip(t *testing.T) {
	ops := paintOf(t, `<div id="o"><a href="#x" style="display: block; height: 30px; background: red">x</a></div>`,
		noDefaults+`#o { width: 100px; height: 100px; overflow: hidden; border-radius: 40px }`)
	links := 0
	for _, op := range ops {
		if _, ok := op.(Link); ok {
			links++
		}
	}
	if links != 1 || len(groupsOf(ops)) == 0 {
		t.Errorf("%d links at the top and %d groups", links, len(groupsOf(ops)))
	}
	for _, g := range groupsOf(ops) {
		for _, op := range g.Ops {
			if _, ok := op.(Link); ok {
				t.Error("a link is inside a clip")
			}
		}
	}
}

// TestARoundedShapeTakesOpacityAndClipping: a path's colour takes an alpha, and
// a rectangle clip rides on it where it cuts.
func TestARoundedShapeTakesOpacityAndClipping(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 100px; height: 60px;
		background: red; border-radius: 20px; opacity: 0.5 }`)
	ps := pathsOf(ops)
	if len(ps) != 1 || ps[0].Color.A != 0.5 {
		t.Errorf("the translucent rounded background is %v", ps)
	}
	ops = paintOf(t, `<div id="o"><div id="a"></div></div>`, noDefaults+`
		#o { width: 50px; height: 200px; overflow: hidden }
		#a { width: 100px; height: 60px; background: red; border-radius: 20px }`)
	ps = pathsOf(ops)
	if len(ps) != 1 || !ps[0].Clip.Active || ps[0].Clip.Rect != (Rect{W: rpx(50), H: rpx(200)}) {
		t.Errorf("the clipped rounded background is %+v", ps)
	}
}

// TestAPathContainsWhatItEncloses holds the inside test to account on arcs that
// cross the top and bottom of their ellipse, which is where a naive crossing
// count goes wrong, and on the even-odd rule.
func TestAPathContainsWhatItEncloses(t *testing.T) {
	// A full ellipse as one arc, centred at 50,30 with semi-axes 40 by 20.
	e := Path{{Op: ArcTo, Center: ptPx(50, 30), RadiusX: rpx(40), RadiusY: rpx(20), StartAngle: -30, SweepAngle: 360},
		{Op: ClosePath}}
	for y := 0.5; y < 60; y += 1.7 {
		for x := 0.5; x < 100; x += 1.9 {
			dx, dy := (x-50)/40, (y-30)/20
			want := dx*dx+dy*dy < 1
			if math.Abs(dx*dx+dy*dy-1) < 0.02 {
				continue
			}
			if got := e.Contains(ptPx(x, y)); got != want {
				t.Fatalf("(%g, %g): inside=%v, want %v", x, y, got, want)
			}
		}
	}
	if b := e.Bounds(); b != (Rect{X: rpx(10), Y: rpx(10), W: rpx(80), H: rpx(40)}) {
		t.Errorf("the ellipse's bounds are %v", b)
	}
	// Two nested squares by the even-odd rule are a frame.
	frame := append(roundedRect(Rect{W: rpx(100), H: rpx(100)}, Radii{}),
		roundedRect(Rect{X: rpx(25), Y: rpx(25), W: rpx(50), H: rpx(50)}, Radii{})...)
	if !frame.Contains(ptPx(10, 50)) || frame.Contains(ptPx(50, 50)) {
		t.Error("a frame is not its frame")
	}
}

// TestPictureSeesARoundedShape: the comparison renders a path as its inside, so
// a rounded box is not its bounding rectangle, and is itself.
func TestPictureSeesARoundedShape(t *testing.T) {
	r := Rect{X: rpx(10), Y: rpx(10), W: rpx(100), H: rpx(60)}
	round := FillPath{Path: roundedRect(r, Radii{corner(20, 20), corner(20, 20), corner(20, 20), corner(20, 20)}), Color: picRed}
	if !pictureEqual([]Op{round}, []Op{round}, picPage) {
		t.Error("a rounded shape is not the same page as itself")
	}
	if pictureEqual([]Op{round}, []Op{FillRect{Rect: r, Color: picRed}}, picPage) {
		t.Error("a rounded shape is the same page as its bounding rectangle")
	}
	// A rectangle drawn as a path is the rectangle.
	square := FillPath{Path: roundedRect(r, Radii{}), Color: picRed}
	if !pictureEqual([]Op{square}, []Op{FillRect{Rect: r, Color: picRed}}, picPage) {
		t.Error("a square path is not the rectangle it is")
	}
	// A clip group shows what it holds inside its shape and nothing outside.
	clipped := ClipPath{Path: round.Path, Ops: []Op{FillRect{Rect: r, Color: picRed}}}
	if !pictureEqual([]Op{clipped}, []Op{round}, picPage) {
		t.Error("a rectangle clipped to a rounded shape is not the rounded shape")
	}
	under := []Op{picFill(0, 0, 200, 200, picGreen), clipped}
	if !pictureEqual(under, []Op{picFill(0, 0, 200, 200, picGreen), round}, picPage) {
		t.Error("what is under a clipped group's corners is not seen")
	}
}

// TestTheRestOfTheListReadsRoundedShapes: the overflow-page guardrail sees a
// rounded box off the page, whether it is a path or inside a group, and skips it
// when it is an overhang; a rectangle clip reaches into a group; an opacity is
// folded into what a group holds; and an inline box's rounded background is an
// overhang, inside a group or not.
func TestTheRestOfTheListReadsRoundedShapes(t *testing.T) {
	off := FillPath{Path: roundedRect(Rect{X: rpx(-50), W: rpx(10), H: rpx(10)},
		Radii{corner(2, 2), corner(2, 2), corner(2, 2), corner(2, 2)}), Color: red}
	page := Size{W: rpx(600), H: rpx(800)}
	for name, tc := range map[string]struct {
		ops  []Op
		want bool
	}{
		"a path":              {[]Op{off}, true},
		"a fill in a group":   {[]Op{ClipPath{Path: off.Path, Ops: []Op{FillRect{Rect: Rect{X: rpx(-50), W: rpx(10), H: rpx(10)}, Color: red}}}}, true},
		"an overhanging path": {[]Op{FillPath{Path: off.Path, Color: red, Overhang: true}}, false},
	} {
		rec := NewRecorder(nil)
		checkPageOverflow(rec, tc.ops, page, 1)
		if got := hasRule(rec.Findings(), RuleOverflowPage); got != tc.want {
			t.Errorf("%s off the page: reported=%v, want %v", name, got, tc.want)
		}
	}

	// A rounded background image is grouped as it is painted, and the box's
	// parent's rectangle clip is applied to the group afterwards.
	ops := paintOf(t, `<div id="w"><div id="o"></div></div>`, noDefaults+`
		#w { width: 30px; overflow: hidden }
		#o { width: 100px; height: 100px; border-radius: 40px; opacity: 0.5;
			background-image: linear-gradient(red, red) }`)
	gs := groupsOf(ops)
	if len(gs) != 1 || len(gs[0].Ops) != 1 {
		t.Fatalf("%d groups: %v", len(gs), ops)
	}
	fill := gs[0].Ops[0].(FillRect)
	if fill.Rect.W != rpx(30) {
		t.Errorf("the fill inside the group is %v, want it cut to the outer 30px", fill.Rect)
	}
	if fill.Color.A != 0.5 {
		t.Errorf("the fill inside the group has alpha %g, want 0.5", fill.Color.A)
	}

	ops = paintOf(t, `<p>a <span id="s">word</span></p>`, noDefaults+`
		#s { background: red; border-radius: 3px }`)
	ps := pathsOf(ops)
	if len(ps) != 1 || !ps[0].Overhang {
		t.Errorf("an inline box's rounded background is %+v, want one overhang", ps)
	}
	ops = paintOf(t, `<p>a <span id="s">word</span></p>`, noDefaults+`
		#s { background-image: linear-gradient(red 50%, blue 50%); border-radius: 3px }`)
	for _, g := range groupsOf(ops) {
		for _, op := range g.Ops {
			if f, ok := op.(FillRect); ok && !f.Overhang {
				t.Errorf("a fill of an inline box's rounded background image is not an overhang: %v", f)
			}
		}
	}
	if len(groupsOf(ops)) == 0 {
		t.Error("an inline box's rounded background image is not clipped to its curve")
	}
}

// TestWhatIsWhollyOutsideTheCurveIsNotPainted: a mark in a corner and past its
// curve is dropped rather than wrapped, since nothing of it would show.
func TestWhatIsWhollyOutsideTheCurveIsNotPainted(t *testing.T) {
	ops := paintOf(t, `<div id="o"><div id="c"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow: hidden; border-radius: 50px }
		#c { width: 10px; height: 10px; background: red }`)
	for _, op := range append(ops, func() []Op {
		var in []Op
		for _, g := range groupsOf(ops) {
			in = append(in, g.Ops...)
		}
		return in
	}()...) {
		if f, ok := op.(FillRect); ok && f.Color == red {
			t.Errorf("a box wholly outside the curve was painted: %v", f)
		}
	}
}

// TestAnAbsoluteBoxTakesItsContainingBlocksCurve: an out-of-flow box is clipped
// by its containing block chain and not by where it was written, and the curve
// goes with the rectangle.
func TestAnAbsoluteBoxTakesItsContainingBlocksCurve(t *testing.T) {
	ops := paintOf(t, `<div id="o"><div id="a"></div></div>`, noDefaults+`
		#o { position: relative; width: 100px; height: 100px; overflow: hidden; border-radius: 40px }
		#a { position: absolute; left: 0; top: 0; width: 30px; height: 30px; background: red }`)
	if len(groupsOf(ops)) != 1 {
		t.Errorf("the positioned box in a rounded containing block is in %d groups", len(groupsOf(ops)))
	}
	ops = paintOf(t, `<div id="o"><div id="a"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow: hidden; border-radius: 40px }
		#a { position: absolute; left: 0; top: 0; width: 30px; height: 30px; background: red }`)
	if len(groupsOf(ops)) != 0 {
		t.Error("a positioned box whose containing block is the page was curved by a box it is not in")
	}
}

// TestTextCutByACurveIsItsOwnMark: the comparison keys a run by the curve that
// cuts it, as it does by a rectangle.
func TestTextCutByACurveIsItsOwnMark(t *testing.T) {
	run := picText("Test", 8, 29)
	round := roundedRect(picRect(0, 0, 100, 40), Radii{corner(20, 20), corner(20, 20), corner(20, 20), corner(20, 20)})
	if pictureEqual([]Op{ClipPath{Path: round, Ops: []Op{run}}}, []Op{run}, picPage) {
		t.Error("a run cut by a curve is the same mark as the run drawn whole")
	}
	if !pictureEqual([]Op{ClipPath{Path: round, Ops: []Op{run}}}, []Op{ClipPath{Path: round, Ops: []Op{run}}}, picPage) {
		t.Error("a run cut by a curve is not itself")
	}
}

// TestAPercentageRadiusOnABrokenInlineBoxIsReported.
func TestAPercentageRadiusOnABrokenInlineBoxIsReported(t *testing.T) {
	for _, tc := range []struct {
		radius string
		want   bool
	}{{"20%", true}, {"4px", false}} {
		built := Build(Input{HTML: `<p>aaa <span id="s">bbb ccc ddd eee</span></p>`,
			CSS: []Stylesheet{{Source: noDefaults + `p { width: 60px; font: 10px/20px monospace }
			#s { background: red; border-radius: ` + tc.radius + ` }`}}})
		rec := NewRecorder(nil)
		Layout(built.Root, Size{W: rpx(100), H: rpx(1000)}, nil, rec)
		got := false
		for _, f := range rec.Findings() {
			got = got || strings.Contains(f.Message, "broken across lines")
		}
		if got != tc.want {
			t.Errorf("border-radius %s: reported=%v, want %v", tc.radius, got, tc.want)
		}
	}
}

// TestAGroupIsChargedAsAMark: a ClipPath is an operation a backend draws, and
// a document decides how many there are, so each is charged to the work budget
// as emit charges a mark — and refused, and reported, where the budget ends.
func TestAGroupIsChargedAsAMark(t *testing.T) {
	const doc = `<div id="o"><div class="c"></div><div class="m"></div><div class="c"></div></div>`
	spent := func(radius string) ([]Op, int64) {
		built := Build(Input{HTML: doc, CSS: []Stylesheet{{Source: noDefaults + `
			#o { width: 100px; height: 100px; overflow: hidden; border-radius: ` + radius + ` }
			.c { height: 10px; background: red } .m { height: 70px; background: blue }`}}})
		frag := Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, NewRecorder(nil))
		rec := NewRecorder(nil)
		before := rec.work.left
		ops := PaintReporting(frag, rec)
		return ops, before - rec.work.left
	}
	round, roundCost := spent("40px")
	_, squareCost := spent("0")
	groups := len(groupsOf(round))
	if groups == 0 {
		t.Fatal("the rounded box made no groups; this test needs some")
	}
	if got := roundCost - squareCost; got != int64(groups)*costOp {
		t.Errorf("the rounded page cost %d more than the square one, want %d for %d groups",
			got, int64(groups)*costOp, groups)
	}
	// With the budget ending between the fill and the group that clips it,
	// neither is drawn, and that is said. The box holds one child in its
	// corner, so the fill is the first charge and its group the second.
	built := Build(Input{HTML: `<div id="o"><div class="c"></div></div>`, CSS: []Stylesheet{{Source: noDefaults + `
		#o { width: 100px; height: 100px; overflow: hidden; border-radius: 40px }
		.c { height: 10px; background: red }`}}})
	frag := Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, NewRecorder(nil))
	rec := NewRecorder(nil)
	rec.work = workBudget{left: costOp + costOp/2}
	ops := PaintReporting(frag, rec)
	if len(ops) != 0 {
		t.Errorf("what the budget refused was drawn: %v", ops)
	}
	if !hasRule(rec.Findings(), RuleLimit) {
		t.Errorf("the refusal was not reported: %v", rec.Findings())
	}
}
