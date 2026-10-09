package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The evidence for transform.go is arithmetic, for the reason writingmode.go
// gives: a reftest compares two renderings by one engine, so a transform
// applied wrongly to both halves of one comes out wrong the same way twice and
// still agrees. Nothing in the suite this engine runs declares a transform in
// any case. So each test here lays out a box, applies a transform to it, and
// compares where the display list puts it against the matrix of CSS
// Transforms 1 §6 worked by hand.
//
// The fixture is one box whose border box is 100 by 40 pixels with its top
// left corner at (200, 100) — "margin: 100px 0 0 200px" on a page with no
// default margins — so that its centre, the initial transform-origin, is
// (250, 120), and every number below is whole.

const transformBox = noDefaults + `
#a { margin: 100px 0 0 200px; width: 100px; height: 40px; background: rgb(0, 128, 0) }
#s { height: 10px; background: blue }`

// transformPaint lays out and paints a document, and returns what the paint
// drew and everything layout and the paint reported.
func transformPaint(t *testing.T, htmlSrc, cssSrc string) ([]Op, []Finding) {
	t.Helper()
	in := Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}}
	built := Build(in)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: A4.Content().W, H: rpx(10000)}, nil, rec)
	ops := PaintReporting(root, rec)
	return ops, append(built.Findings, rec.Findings()...)
}

// fillOf is the rectangle of the one fill of a colour, failing if there is
// not exactly one.
func fillOf(t *testing.T, ops []Op, c style.RGBA) Rect {
	t.Helper()
	var found []Rect
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			switch v := op.(type) {
			case FillRect:
				if v.Color == c {
					found = append(found, v.Rect)
				}
			case ClipPath:
				walk(v.Ops)
			case FilterGroup:
				walk(v.Ops)
			}
		}
	}
	walk(ops)
	if len(found) != 1 {
		t.Fatalf("want one fill of %v, found %v", c, found)
	}
	return found[0]
}

func xRect(x, y, w, h float64) Rect { return Rect{X: rpx(x), Y: rpx(y), W: rpx(w), H: rpx(h)} }

// transformFindings is the findings about transform.
func transformFindings(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Property == "transform" {
			out = append(out, f)
		}
	}
	return out
}

// TestATransformMovesTheBoxByTheMatrix is each function and the origin, worked
// by hand. For a box whose border box is (200, 100) to (300, 140) and an
// origin o, §6's matrix is T(o)·F·T(-o): a corner p is drawn at F(p - o) + o.
func TestATransformMovesTheBoxByTheMatrix(t *testing.T) {
	for _, tc := range []struct {
		transform, origin string
		want              Rect
	}{
		// A move, and a move in percentages of the border box: 10% of 100 is
		// 10, and 50% of 40 is 20.
		{"translate(30px, 5px)", "", xRect(230, 105, 100, 40)},
		{"translate(10%, 50%)", "", xRect(210, 120, 100, 40)},
		{"translateX(-20px)", "", xRect(180, 100, 100, 40)},
		{"translateY(25%)", "", xRect(200, 110, 100, 40)},
		// A quarter turn about the centre (250, 120): the corner (200, 100) is
		// (-50, -20) from it, which rotate(90deg) takes to (20, -50) — x' = -y,
		// y' = x — and so to (270, 70); the far corner to (230, 170).
		{"rotate(90deg)", "", xRect(230, 70, 40, 100)},
		{"rotate(-90deg)", "", xRect(230, 70, 40, 100)},
		{"rotate(180deg)", "", xRect(200, 100, 100, 40)},
		{"rotate(0.25turn)", "", xRect(230, 70, 40, 100)},
		{"rotate(100grad)", "", xRect(230, 70, 40, 100)},
		// Two turns that are not quarter turns, whose product is one.
		{"rotate(30deg) rotate(60deg)", "", xRect(230, 70, 40, 100)},
		// About the top left corner instead: (200, 100) stays, and (300, 140)
		// is (100, 40) from it, which turns to (-40, 100).
		{"rotate(90deg)", "0 0", xRect(160, 100, 40, 100)},
		{"rotate(90deg)", "left top", xRect(160, 100, 40, 100)},
		{"rotate(90deg)", "top left", xRect(160, 100, 40, 100)},
		// About the bottom right, (300, 140): (200, 100) is (-100, -40) from
		// it and turns to (40, -100), so to (340, 40).
		{"rotate(90deg)", "right bottom", xRect(300, 40, 40, 100)},
		{"rotate(90deg)", "100% 100%", xRect(300, 40, 40, 100)},
		// One keyword: "bottom" is the vertical one, and the other axis is
		// at the centre, so the origin is (250, 140). (200, 100) is (-50, -40)
		// and turns to (40, -50): (290, 90).
		{"rotate(90deg)", "bottom", xRect(250, 90, 40, 100)},
		// A length and a percentage: (210, 125), a 10px and 25% of 40.
		// (200, 100) is (-10, -25), turning to (25, -10): (235, 115); and
		// (300, 140) is (90, 15), turning to (-15, 90): (195, 215).
		{"rotate(90deg)", "10px 62.5%", xRect(195, 115, 40, 100)},
		// A scale about the centre: every corner twice as far from it.
		{"scale(2)", "", xRect(150, 80, 200, 80)},
		{"scale(50%)", "", xRect(225, 110, 50, 20)},
		{"scale(2, 0.5)", "", xRect(150, 110, 200, 20)},
		{"scaleX(3)", "", xRect(100, 100, 300, 40)},
		{"scaleY(3)", "", xRect(200, 60, 100, 120)},
		{"scale(2)", "0 0", xRect(200, 100, 200, 80)},
		// matrix(a, b, c, d, e, f) is x' = ax + cy + e, y' = bx + dy + f, of
		// the point from the origin: the quarter turn and a move of (10, 20).
		{"matrix(0, 1, -1, 0, 10, 20)", "", xRect(240, 90, 40, 100)},
		// A skew of nothing is nothing.
		{"skew(0)", "", xRect(200, 100, 100, 40)},
	} {
		css := transformBox + `#a { transform: ` + tc.transform + ` }`
		if tc.origin != "" {
			css += `#a { transform-origin: ` + tc.origin + ` }`
		}
		ops, findings := transformPaint(t, `<div id="a"></div><div id="s"></div>`, css)
		if got := fillOf(t, ops, green); got != tc.want {
			t.Errorf("%s about %q: the box is drawn at %v, want %v", tc.transform, tc.origin, got, tc.want)
		}
		if got := transformFindings(findings); len(got) > 0 {
			t.Errorf("%s about %q was reported: %v", tc.transform, tc.origin, got)
		}
		// §2: the transform does not affect the flow. The box after it is
		// where it would be without one.
		if got, want := fillOf(t, ops, blue), xRect(0, 140, A4.Content().W.Px(), 10); got != want {
			t.Errorf("%s: the next box moved to %v, want %v", tc.transform, got, want)
		}
	}
}

// TestTheFunctionsComposeRightToLeft: §6 multiplies the functions left to
// right, so the rightmost acts on the box first. "translate(100px, 0)
// rotate(90deg)" turns the box and then moves the result right;
// "rotate(90deg) translate(100px, 0)" moves the box right and then turns the
// move with it, downwards.
func TestTheFunctionsComposeRightToLeft(t *testing.T) {
	for _, tc := range []struct {
		transform string
		want      Rect
	}{
		// Turned: (230, 70) to (270, 170). Then 100 to the right.
		{"translate(100px, 0) rotate(90deg)", xRect(330, 70, 40, 100)},
		// (200, 100) is (-50, -20) from the centre; moved, (50, -20); turned,
		// (20, 50); so (270, 170). (300, 140): (150, 20), turned (-20, 150),
		// so (230, 270).
		{"rotate(90deg) translate(100px, 0)", xRect(230, 170, 40, 100)},
		// A scale after a move scales the move; before it, it does not.
		{"scale(2) translate(10px, 0)", xRect(170, 80, 200, 80)},
		{"translate(10px, 0) scale(2)", xRect(160, 80, 200, 80)},
	} {
		ops, _ := transformPaint(t, `<div id="a"></div>`, transformBox+`#a { transform: `+tc.transform+` }`)
		if got := fillOf(t, ops, green); got != tc.want {
			t.Errorf("%s: the box is drawn at %v, want %v", tc.transform, got, tc.want)
		}
	}
}

// TestAPercentageIsOfTheBorderBox: CSS Transforms 1 §6 resolves a percentage
// in translate() against the reference box, which for a CSS box is its
// border box — not its content box, and not its containing block. The box
// here is 100 + 2·10 padding + 2·5 border = 130 wide and 40 + 30 = 70 tall,
// and its containing block is the page.
func TestAPercentageIsOfTheBorderBox(t *testing.T) {
	css := transformBox + `#a { padding: 10px; border: 5px solid red; transform: translate(50%, 100%) }`
	ops, _ := transformPaint(t, `<div id="a"></div>`, css)
	// The background fills the border box, 130 by 70 at (200, 100) before
	// the move, and the move is (65, 70) — where the content box would have
	// made it (50, 40).
	if got, want := fillOf(t, ops, green), xRect(265, 170, 130, 70); got != want {
		t.Errorf("the box's border box is drawn at %v, want %v", got, want)
	}
	// And the origin's percentages are of the same box: 100% 0 is its top
	// right corner, (330, 100). A quarter turn about it takes (200, 100),
	// which is (-130, 0) from it, to (0, -130), so to (330, -30); and (330,
	// 170), which is (0, 70), to (-70, 0), so to (260, 100).
	css = transformBox + `#a { padding: 10px; border: 5px solid red; transform: rotate(90deg); transform-origin: 100% 0 }`
	ops, _ = transformPaint(t, `<div id="a"></div>`, css)
	if got, want := fillOf(t, ops, green), xRect(260, -30, 70, 130); got != want {
		t.Errorf("turned about its top right corner, the border box is at %v, want %v", got, want)
	}
}

// TestTextTurnsAndScalesWithItsBox: a run is drawn from its pen position, at
// its size, turned by DrawText's quarter turns; so a quarter turn is the pen
// position turned and the run Sideways, a turn the other way Anticlockwise,
// and a uniform scale is a font size.
func TestTextTurnsAndScalesWithItsBox(t *testing.T) {
	const doc = `<div id="a">ab</div>`
	css := transformBox + `#a { font: 20px/20px Courier }`
	plain, _ := transformPaint(t, doc, css)
	run := onlyText(t, plain)
	x, y := run.At.X.Px(), run.At.Y.Px()
	for _, tc := range []struct {
		transform               string
		at                      Point
		size                    float64
		sideways, anticlockwise bool
	}{
		// About (250, 120): x' = 250 - (y - 120), y' = 120 + (x - 250).
		{"rotate(90deg)", ptPx(370-y, x-130), 20, true, false},
		// x' = 250 + (y - 120), y' = 120 - (x - 250).
		{"rotate(-90deg)", ptPx(130+y, 370-x), 20, true, true},
		// Twice as far from (250, 120), at twice the size.
		{"scale(2)", ptPx(2*x-250, 2*y-120), 40, false, false},
		{"translate(7px, 3px)", ptPx(x+7, y+3), 20, false, false},
	} {
		ops, findings := transformPaint(t, doc, css+`#a { transform: `+tc.transform+` }`)
		if got := transformFindings(findings); len(got) > 0 {
			t.Fatalf("%s was reported: %v", tc.transform, got)
		}
		got := onlyText(t, ops)
		if got.At != tc.at || got.Size != rpx(tc.size) || got.Sideways != tc.sideways || got.Anticlockwise != tc.anticlockwise {
			t.Errorf("%s: the run is at %v, %vpx, sideways %v, anticlockwise %v; want %v, %vpx, %v, %v",
				tc.transform, got.At, got.Size.Px(), got.Sideways, got.Anticlockwise,
				tc.at, tc.size, tc.sideways, tc.anticlockwise)
		}
	}
}

func onlyText(t *testing.T, ops []Op) DrawText {
	t.Helper()
	var found []DrawText
	for _, op := range ops {
		if v, ok := op.(DrawText); ok {
			found = append(found, v)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one run, found %d", len(found))
	}
	return found[0]
}

// TestAVerticalRunTurnsBackAcross: a sideways run turned a quarter the other
// way is a run across the page again, which DrawText states exactly; turned
// the same way once more it would stand on its head, which nothing states.
func TestAVerticalRunTurnsBackAcross(t *testing.T) {
	const doc = `<div id="a">ab</div>`
	css := transformBox + `#a { font: 20px/20px Courier; writing-mode: vertical-rl; height: 100px }`
	ops, findings := transformPaint(t, doc, css+`#a { transform: rotate(-90deg) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Fatalf("a vertical box turned back was reported: %v", got)
	}
	if run := onlyText(t, ops); run.Sideways || run.Anticlockwise {
		t.Errorf("a vertical run turned back is sideways %v, anticlockwise %v; want across the page",
			run.Sideways, run.Anticlockwise)
	}
	_, findings = transformPaint(t, doc, css+`#a { transform: rotate(90deg) }`)
	if got := transformFindings(findings); len(got) != 1 || !strings.Contains(got[0].Message, "upside down") {
		t.Errorf("a vertical run turned upside down was not reported as such: %v", got)
	}
}

// TestWhatTheDisplayListCannotDrawIsRefused: each is reported at the box,
// and the box is drawn where layout put it.
func TestWhatTheDisplayListCannotDrawIsRefused(t *testing.T) {
	const img = `<img src="data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw==">`
	for _, tc := range []struct {
		doc, transform, why string
	}{
		{`<div id="a"></div>`, "rotate(45deg)", "not a multiple of ninety degrees"},
		{`<div id="a"></div>`, "skewX(10deg)", "not a multiple of ninety degrees"},
		{`<div id="a"></div>`, "scaleX(-1)", "mirrors"},
		{`<div id="a"></div>`, "rotateX(180deg)", "3D"},
		{`<div id="a"></div>`, "translate3d(1px, 0, 0)", "3D"},
		{`<div id="a"></div>`, "perspective(100px)", "3D"},
		{`<div id="a">ab</div>`, "rotate(180deg)", "upside down"},
		{`<div id="a">ab</div>`, "scale(2, 1)", "more along one axis"},
		{`<div id="a">` + img + `</div>`, "rotate(90deg)", "picture"},
	} {
		css := transformBox + `#a { font: 20px/20px Courier; transform: ` + tc.transform + ` }`
		ops, findings := transformPaint(t, tc.doc, css)
		got := transformFindings(findings)
		if len(got) != 1 || !strings.Contains(got[0].Message, tc.why) || !got[0].Unsupported() {
			t.Errorf("%s on %s: reported %v, want one finding saying %q", tc.transform, tc.doc, got, tc.why)
			continue
		}
		if strings.HasPrefix(tc.doc, `<div`) {
			if r := fillOf(t, ops, green); r != xRect(200, 100, 100, 40) {
				t.Errorf("%s was refused and the box was drawn at %v, not where layout put it", tc.transform, r)
			}
		}
	}
}

// TestAPictureMovesAndScales: a picture is drawn stretched to its rectangle,
// so a move and a scale along either axis are exact for it.
func TestAPictureMovesAndScales(t *testing.T) {
	const img = `<img id="a" style="display: block; width: 100px; height: 40px; margin: 100px 0 0 200px" src="data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw==">`
	ops, findings := transformPaint(t, img, noDefaults+`#a { transform: scale(2, 0.5) translate(10px, 4px) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Fatalf("a scaled picture was reported: %v", got)
	}
	var pic *DrawImage
	for _, op := range ops {
		if v, ok := op.(DrawImage); ok {
			pic = &v
		}
	}
	if pic == nil {
		t.Fatal("no picture was drawn")
	}
	// Moved (10, 4) and then scaled about (250, 120): (210, 104) to
	// (170, 112) and (310, 144) to (370, 132).
	if want := xRect(170, 112, 200, 20); pic.Rect != want {
		t.Errorf("the picture is drawn at %v, want %v", pic.Rect, want)
	}
}

// TestNothingOfAnUninvertibleBoxIsDrawn: §6, "If a transform function causes
// the current transformation matrix of an object to be non-invertible, the
// object and its content do not get displayed" — its text and its links
// included.
func TestNothingOfAnUninvertibleBoxIsDrawn(t *testing.T) {
	ops, findings := transformPaint(t, `<div id="a"><a href="https://example.com/">ab</a></div><div id="s"></div>`,
		transformBox+`#a { font: 20px/20px Courier; transform: scale(0) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Errorf("scale(0) was reported: %v", got)
	}
	for _, op := range ops {
		switch v := op.(type) {
		case DrawText, Link:
			t.Errorf("an uninvertible box drew %T %v", op, v)
		case FillRect:
			if v.Color == green {
				t.Errorf("an uninvertible box drew its background at %v", v.Rect)
			}
		}
	}
	fillOf(t, ops, blue)
}

// TestALinkMovesWithItsBox: a link's area is where a reader clicks, and the
// box it is the area of has moved.
func TestALinkMovesWithItsBox(t *testing.T) {
	const doc = `<div id="a"><a href="https://example.com/" style="display: block; height: 40px">x</a></div>`
	plain, _ := transformPaint(t, doc, transformBox)
	moved, _ := transformPaint(t, doc, transformBox+`#a { transform: rotate(90deg) }`)
	area := func(ops []Op) []Rect {
		for _, op := range ops {
			if l, ok := op.(Link); ok {
				return l.Rects
			}
		}
		t.Fatal("no link")
		return nil
	}
	if got := area(plain); len(got) != 1 || got[0] != xRect(200, 100, 100, 40) {
		t.Fatalf("the link is at %v untransformed", got)
	}
	if got := area(moved); len(got) != 1 || got[0] != xRect(230, 70, 40, 100) {
		t.Errorf("the link of a turned box is at %v, want %v", got, xRect(230, 70, 40, 100))
	}
}

// TestAClipOutsideCutsTheTransformedBox: what clips a transformed box from
// outside cuts it where it is drawn, and what clips inside it turns with it.
func TestAClipOutsideCutsTheTransformedBox(t *testing.T) {
	// A parent that clips at its padding box, (0, 0) to (260, 200): the box
	// moved 50 to the right reaches (350, 140) and is cut at 260.
	ops, _ := transformPaint(t, `<div id="c"><div id="a"></div></div>`,
		transformBox+`#c { width: 260px; height: 200px; overflow: hidden } #a { transform: translateX(50px) }`)
	if got, want := fillOf(t, ops, green), xRect(250, 100, 10, 40); got != want {
		t.Errorf("a box moved across its parent's clip is drawn at %v, want %v", got, want)
	}
	// The box clips its own child at its padding box, (200, 100) to (300,
	// 140), and turns: the child, (200, 100) to (400, 120), is cut to the
	// box's left half and the box's band (200, 100)-(300, 120), which a turn
	// about (250, 120) takes to (250, 70)-(270, 170).
	ops, _ = transformPaint(t, `<div id="a"><div id="k"></div></div>`,
		transformBox+`#a { overflow: hidden; transform: rotate(90deg) } #k { width: 200px; height: 20px; background: red }`)
	if got, want := fillOf(t, ops, red), xRect(250, 70, 20, 100); got != want {
		t.Errorf("a child clipped by the turned box is drawn at %v, want %v", got, want)
	}
}

// TestATransformedBoxContainsWhatIsPositionedInside: §2 makes it the
// containing block of absolutely and fixed positioned descendants, and they
// move with it.
func TestATransformedBoxContainsWhatIsPositionedInside(t *testing.T) {
	for _, scheme := range []string{"absolute", "fixed"} {
		ops, _ := transformPaint(t, `<div id="a"><div id="k"></div></div>`,
			transformBox+`#a { transform: translate(10px, 20px) }
			#k { position: `+scheme+`; left: 5px; top: 6px; width: 10px; height: 10px; background: red }`)
		// The box's padding box is at (200, 100), the child 5 and 6 into it,
		// and the whole moved by (10, 20).
		if got, want := fillOf(t, ops, red), xRect(215, 126, 10, 10); got != want {
			t.Errorf("a %s child of a transformed box is at %v, want %v", scheme, got, want)
		}
	}
}

// TestATransformDoesNotApplyToAnInlineBox: CSS Transforms 1 §1.2. The span
// is not transformed, which is the page a browser draws, so nothing is
// reported; a cell and the root, which this engine does not turn, are.
func TestATransformDoesNotApplyToAnInlineBox(t *testing.T) {
	_, findings := transformPaint(t, `<div><span id="i">ab</span></div>`,
		noDefaults+`#i { transform: translateX(10px) }`)
	if got := transformFindings(findings); len(got) != 0 {
		t.Errorf("a transform on a non-atomic inline box, which does not apply, was reported: %v", got)
	}
	for _, tc := range []struct{ doc, css, why string }{
		{`<table><tr><td id="i">a</td></tr></table>`, `#i { transform: translateX(10px) }`, "cell"},
		{`<p>a</p>`, `html { transform: translateX(10px) }`, "root element"},
	} {
		_, findings := transformPaint(t, tc.doc, noDefaults+tc.css)
		got := transformFindings(findings)
		if len(got) != 1 || !strings.Contains(got[0].Message, tc.why) {
			t.Errorf("%s in %s: reported %v, want one finding about %q", tc.css, tc.doc, got, tc.why)
		}
	}
}

// TestTheNaturalSizeIsOfTheTransformedBox: the page is scaled to fit what was
// drawn, and a box drawn past the page's edge is what has to fit.
func TestTheNaturalSizeIsOfTheTransformedBox(t *testing.T) {
	compose := func(css string) Composed {
		return Compose(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: transformBox + css}}},
			Options{Page: PageSize{Width: rpx(600), Height: rpx(800)}})
	}
	plain := compose(``)
	if plain.NaturalSize.W > rpx(600) || plain.Scale != 1 {
		t.Fatalf("the untransformed box needs %v at scale %v", plain.NaturalSize, plain.Scale)
	}
	moved := compose(`#a { transform: translateX(500px) }`)
	if moved.NaturalSize.W != rpx(800) {
		t.Errorf("a box drawn out to x = 800 needs %v of width, want 800px", moved.NaturalSize.W.Px())
	}
	if moved.Scale != 0.75 {
		t.Errorf("a box drawn out to x = 800 on a 600px page is scaled by %v, want 0.75", moved.Scale)
	}
	for _, f := range moved.Findings {
		if f.Rule == RuleOverflowPage {
			t.Errorf("the page-overflow guard fired on a scale computed from the drawn box: %v", f)
		}
	}
	// A transform the paint refused is drawn where the box was, and the
	// page is measured there too.
	refused := compose(`#a { transform: translateX(500px) rotate(45deg) }`)
	if refused.NaturalSize != plain.NaturalSize {
		t.Errorf("a refused transform changed the natural size from %v to %v", plain.NaturalSize, refused.NaturalSize)
	}
}

// TestAGroupSeesItsMarksWhereTheyAreDrawn: opacity is folded into each mark
// and is exact only where no two of a group's marks overlap, which is asked
// of where they are. Two boxes side by side do not overlap; moved onto one
// another by a transform, they do, and that is reported.
func TestAGroupSeesItsMarksWhereTheyAreDrawn(t *testing.T) {
	doc := `<div id="g"><div id="a"></div><div id="b"></div></div>`
	css := noDefaults + `#g { opacity: 0.5 } #a, #b { width: 100px; height: 40px; background: rgb(0, 128, 0) }`
	overlap := func(extra string) bool {
		_, findings := transformPaint(t, doc, css+extra)
		for _, f := range findings {
			if f.Property == "opacity" && strings.Contains(f.Message, "lie over each other") {
				return true
			}
		}
		return false
	}
	if overlap(``) {
		t.Fatal("two boxes one under the other were reported as overlapping")
	}
	if !overlap(`#b { transform: translateY(-20px) }`) {
		t.Error("a box moved over its sibling inside one translucent group was not reported")
	}
}

// TestASmallScaleIsASmallFont: the font-size floor is about the size text is
// drawn at, which a transform changes.
func TestASmallScaleIsASmallFont(t *testing.T) {
	compose := func(css string) Composed {
		return Compose(Input{HTML: `<div id="a">ab</div>`, CSS: []Stylesheet{{Source: transformBox + `#a { font: 20px Courier }` + css}}},
			Options{})
	}
	small := func(c Composed) bool {
		for _, f := range c.Findings {
			if f.Rule == RuleMinFontSize {
				return true
			}
		}
		return false
	}
	if small(compose(``)) {
		t.Fatal("20px text was reported as too small")
	}
	if !small(compose(`#a { transform: scale(0.1) }`)) {
		t.Error("20px text drawn at a tenth of its size was not reported as too small")
	}
}

// TestShapesGradientsAndGroupsTurnWithTheBox: a rounded box is a path, a
// gradient is a field laid out for its tile, and a filtered box is a group,
// and each is turned and scaled exactly.
func TestShapesGradientsAndGroupsTurnWithTheBox(t *testing.T) {
	// A rounded box turned about its centre: the path's bounds are the turned
	// border box, and each corner's arc begins a quarter further round.
	ops, findings := transformPaint(t, `<div id="a"></div>`,
		transformBox+`#a { border-radius: 10px / 5px; transform: rotate(90deg) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Fatalf("a rounded box was reported: %v", got)
	}
	var path *FillPath
	for _, op := range ops {
		if v, ok := op.(FillPath); ok && v.Color == green {
			path = &v
		}
	}
	if path == nil {
		t.Fatal("the rounded box drew no path")
	}
	if got, want := path.Path.Bounds(), xRect(230, 70, 40, 100); got != want {
		t.Errorf("the turned rounded box's path has bounds %v, want %v", got, want)
	}
	// And each arc is the untransformed one turned: where it begins and ends,
	// (x, y) on the page, is drawn at (370 - y, x - 130).
	plainOps, _ := transformPaint(t, `<div id="a"></div>`, transformBox+`#a { border-radius: 10px / 5px }`)
	var plainPath *FillPath
	for _, op := range plainOps {
		if v, ok := op.(FillPath); ok && v.Color == green {
			plainPath = &v
		}
	}
	if plainPath == nil || len(plainPath.Path) != len(path.Path) {
		t.Fatalf("the untransformed rounded box drew %v, the turned one %d segments", plainPath, len(path.Path))
	}
	arcs := 0
	for i, s := range plainPath.Path {
		if s.Op != ArcTo {
			continue
		}
		arcs++
		u := path.Path[i]
		for _, a := range []float64{0, s.SweepAngle} {
			x, y := arcPoint(s, s.StartAngle+a)
			gx, gy := arcPoint(u, u.StartAngle+a)
			if math.Abs(gx-(370-y)) > 1e-9 || math.Abs(gy-(x-130)) > 1e-9 {
				t.Errorf("arc %d: the point at %v° of its sweep is drawn at (%v, %v), want (%v, %v)",
					i, a, gx, gy, 370-y, x-130)
			}
		}
	}
	if arcs == 0 {
		t.Fatal("the rounded box's path has no arcs, which proves nothing about them")
	}

	// A gradient to the right, across a 100 by 40 tile at (200, 100). A
	// point (x, y) of the tile is (200 + x, 100 + y) on the page; turned
	// about (250, 120) that is (270 - y, 70 + x), and the turned tile is at
	// (230, 70), so the point is (40 - y, x) in it. The line runs from (0,
	// 20) to (100, 20), less what layout's own trigonometry rounds, and the
	// numbers are taken from the box drawn untransformed.
	gradientOf := func(ops []Op) *FillGradient {
		for _, op := range ops {
			if v, ok := op.(FillGradient); ok {
				return &v
			}
		}
		t.Fatal("no gradient was drawn")
		return nil
	}
	const gradCSS = `#a { background: linear-gradient(to right, red, blue) }`
	plain, _ := transformPaint(t, `<div id="a"></div>`, transformBox+gradCSS)
	before := gradientOf(plain).Gradient
	ops, findings = transformPaint(t, `<div id="a"></div>`, transformBox+gradCSS+`#a { transform: rotate(90deg) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Fatalf("a gradient was reported: %v", got)
	}
	grad := gradientOf(ops)
	if grad.Tile != xRect(230, 70, 40, 100) || grad.Clip != xRect(230, 70, 40, 100) {
		t.Errorf("the turned gradient's tile is %v and its area %v, want both %v", grad.Tile, grad.Clip, xRect(230, 70, 40, 100))
	}
	turned := func(p Point) Point { return Point{X: rpx(40).Sub(p.Y), Y: p.X} }
	if want := turned(before.Start); grad.Gradient.Start != want {
		t.Errorf("the turned gradient starts at %v, want %v", grad.Gradient.Start, want)
	}
	if want := turned(before.End); grad.Gradient.End != want {
		t.Errorf("the turned gradient ends at %v, want %v", grad.Gradient.End, want)
	}
	if before.Start.Y.Px() != 20 || before.End.X.Px() != 100 {
		t.Fatalf("the untransformed gradient runs from %v to %v, which is not across its tile", before.Start, before.End)
	}

	// A conic gradient turned a quarter starts a quarter further round.
	ops, _ = transformPaint(t, `<div id="a"></div>`,
		transformBox+`#a { background: conic-gradient(from 10deg, red, blue); transform: rotate(90deg) }`)
	for _, op := range ops {
		if v, ok := op.(FillGradient); ok && v.Gradient.FromAngle != 100 {
			t.Errorf("a conic gradient from 10deg turned a quarter starts from %v, want 100", v.Gradient.FromAngle)
		}
	}

	// A blurred box scaled twice over: the group's ops are scaled, and its
	// blur's deviation with them.
	ops, findings = transformPaint(t, `<div id="a"></div>`,
		transformBox+`#a { filter: blur(3px); transform: scale(2) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Fatalf("a filtered box was reported: %v", got)
	}
	var group *FilterGroup
	for _, op := range ops {
		if v, ok := op.(FilterGroup); ok {
			group = &v
		}
	}
	if group == nil || len(group.Filters) != 1 || group.Filters[0].StdDev != rpx(6) {
		t.Fatalf("the scaled blur is %+v, want one blur of 6px", group)
	}
	if got := fillOf(t, group.Ops, green); got != xRect(150, 80, 200, 80) {
		t.Errorf("the box inside the scaled group is at %v, want %v", got, xRect(150, 80, 200, 80))
	}
}

// TestATilingThatWouldDriftIsRefused: a tiling is a first tile and a step,
// and a step that a scale does not take to a whole number of layout units
// would be wrong once more with every tile. A step it does is exact.
func TestATilingThatWouldDriftIsRefused(t *testing.T) {
	css := transformBox + `#a { background: linear-gradient(red, blue) 0 0 / 10px 10px }`
	_, findings := transformPaint(t, `<div id="a"></div>`, css+`#a { transform: scale(2) }`)
	if got := transformFindings(findings); len(got) > 0 {
		t.Errorf("a tiling scaled to whole steps was reported: %v", got)
	}
	_, findings = transformPaint(t, `<div id="a"></div>`, css+`#a { transform: scale(1.001) }`)
	if got := transformFindings(findings); len(got) != 1 || !strings.Contains(got[0].Message, "drift") {
		t.Errorf("a tiling scaled to steps between layout units was not refused: %v", got)
	}
}

// TestTransformsAreChargedToTheDocument: each transformed box copies what is
// inside it, so a stack of them costs its depth times its contents, and the
// copies are charged to the document's work budget. Past it, the boxes left
// are drawn untransformed, and the budget's finding says so; the document is
// still drawn.
func TestTransformsAreChargedToTheDocument(t *testing.T) {
	lowWork(t, 1<<20)
	const depth = 200
	in := Input{
		HTML: strings.Repeat(`<div class="t">`, depth) + strings.Repeat("<p>x</p>", 100) + strings.Repeat(`</div>`, depth),
		CSS:  []Stylesheet{{Source: `.t { transform: translateX(1px) }`}},
	}
	got := Compose(in, Options{})
	requireCut(t, got.Findings, "the transforms past that point, which were drawn untransformed")
	if text := drawnText(got.Ops); strings.Count(text, "x") != 100 {
		t.Errorf("the document's text did not survive the budget: %d of 100 runs", strings.Count(text, "x"))
	}
	// And at the budget every document has, the same stack is drawn whole.
	workFloor = 1 << 28
	got = Compose(in, Options{})
	for _, f := range got.Findings {
		if f.Rule == RuleLimit {
			t.Errorf("a stack of %d transforms over 100 paragraphs met the ordinary budget: %v", depth, f)
		}
	}
}
