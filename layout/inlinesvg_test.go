package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An inline <svg>, which is a replaced element whose content is a picture.
//
// It was refused outright: not an element this engine lays out, dropped, and —
// worse — its children parsed on as HTML, so an <svg><text>x</text></svg> put an
// x in the paragraph around it. Fifty-two of the suite's reftests carry one, and
// it was the largest single gap in the whole corpus.
//
// What is drawn is rectangles and nothing else, and the reason is the display
// list rather than the format: FillRect is the only shape a backend here is
// given, and a circle or a path needs an operation that does not exist. An SVG
// whose drawing is rectangles is drawn; one whose drawing is not is reported and
// laid out empty, which is still a box of the right size.

// blue is declared in inlinepaint_test.go.

// svgBox lays out one inline SVG and returns the fills it painted.
func svgFills(t *testing.T, markup, css string, want style.RGBA) []Rect {
	t.Helper()
	return fillsOf(paintOf(t, `<div id="d">`+markup+`</div>`, css), want)
}

// TestAnInlineSVGIsSizedFromItsOwnAttributes.
//
// This is why it is a replaced element and not a refusal. The size is on the
// element — width, height, viewBox — and is knowable whether or not anything can
// draw the content, so a box for one is not a guess. It is the same argument
// that moved <iframe> out of the dropped set.
func TestAnInlineSVGIsSizedFromItsOwnAttributes(t *testing.T) {
	for _, tc := range []struct {
		what  string
		attrs string
		w, h  float64
	}{
		{"a width and a height", `width="120" height="60"`, 120, 60},
		// A ratio and one dimension: the other follows. 2:1 and a height of 60.
		{"a viewBox and a height", `height="60" viewBox="0 0 200 100"`, 120, 60},
		// No intrinsic size at all: CSS 2.1 §10.3.2's 300 by 150.
		{"nothing at all", ``, 300, 150},
		// One dimension and no ratio: the other is the default.
		{"a height alone", `height="60"`, 300, 60},
	} {
		// The box is measured by what fills it. An inline replaced element lives
		// in a line box rather than in a fragment of its own, so there is no
		// fragment to ask — and a rectangle covering the viewport is exactly the
		// content box, which is the thing under test.
		got := svgFills(t, `<svg `+tc.attrs+`><rect width="100%" height="100%" `+
			`fill="blue"/></svg>`, `#d { font-size: 0 }`, blue)
		if len(got) != 1 {
			t.Errorf("%s: %d fills, want 1", tc.what, len(got))
			continue
		}
		if got[0].W != bgpx(tc.w) || got[0].H != bgpx(tc.h) {
			t.Errorf("%s: the box is %v by %v, want %v by %v",
				tc.what, got[0].W, got[0].H, bgpx(tc.w), bgpx(tc.h))
		}
	}
}

// TestAnInlineSVGPaintsItsRectanglesWhereTheyAre is the suite's own fixture, and
// the reason a whole-viewport fill was not enough.
//
// The rect is 200 by 100 in an svg 300 by 50: it starts at the origin, fills the
// height and overflows it, and stops well short of the right edge. What must
// reach the page is a 200 by 50 mark — the rect, clipped — and the test the suite
// writes around this compares its width against a 200px box beside it.
func TestAnInlineSVGPaintsItsRectanglesWhereTheyAre(t *testing.T) {
	got := svgFills(t,
		`<svg height="50"><rect x="0" y="0" width="200" height="100" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 1 {
		t.Fatalf("%d blue fills, want 1: %v", len(got), got)
	}
	if got[0].W != bgpx(200) {
		t.Errorf("the mark is %v wide and the rect is 200; a fill of the whole "+
			"viewport would be 300", got[0].W)
	}
	if got[0].H != bgpx(50) {
		t.Errorf("the mark is %v tall and the viewport is 50; the rect is 100 and "+
			"is clipped to the element", got[0].H)
	}
}

// TestAnInlineSVGClipsToItsViewport. An outermost <svg> hides its overflow by
// initial value, so a rectangle running past the viewport is cut rather than
// drawn over the page around it.
func TestAnInlineSVGClipsToItsViewport(t *testing.T) {
	got := svgFills(t,
		`<svg width="40" height="40"><rect x="-20" y="-20" width="200" height="200" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 1 {
		t.Fatalf("%d blue fills, want 1: %v", len(got), got)
	}
	if got[0].W != bgpx(40) || got[0].H != bgpx(40) {
		t.Errorf("the mark is %v by %v; a 200 by 200 rect in a 40 by 40 viewport "+
			"is cut to the viewport", got[0].W, got[0].H)
	}
}

// TestARectOutsideTheViewportPaintsNothing: the other end of the same rule.
func TestARectOutsideTheViewportPaintsNothing(t *testing.T) {
	got := svgFills(t,
		`<svg width="40" height="40"><rect x="100" y="100" width="10" height="10" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 0 {
		t.Errorf("%d fills for a rect wholly outside the viewport: %v", len(got), got)
	}
}

// TestAViewBoxScalesTheUserUnits, and the two ways it does.
//
// A viewBox states the coordinate system the rectangles are in. The default
// fits it into the viewport uniformly and centres the remainder;
// preserveAspectRatio="none" stretches it to the viewport instead, which is what
// the suite's fixtures ask for when they want exact arithmetic.
func TestAViewBoxScalesTheUserUnits(t *testing.T) {
	// A 100x100 viewBox in a 200x200 element: everything doubles.
	got := svgFills(t,
		`<svg width="200" height="200" viewBox="0 0 100 100">`+
			`<rect x="10" y="10" width="50" height="50" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1", len(got))
	}
	if got[0].W != bgpx(100) || got[0].H != bgpx(100) {
		t.Errorf("the mark is %v by %v; a 50-unit rect at scale 2 is 100 by 100",
			got[0].W, got[0].H)
	}

	// Stretched: a 100x50 viewBox in a 200x200 element scales x by 2 and y by 4.
	got = svgFills(t,
		`<svg width="200" height="200" viewBox="0 0 100 50" preserveAspectRatio="none">`+
			`<rect x="0" y="0" width="50" height="25" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1", len(got))
	}
	if got[0].W != bgpx(100) || got[0].H != bgpx(100) {
		t.Errorf("the mark is %v by %v; \"none\" scales the two axes apart, so 50 "+
			"by 25 becomes 100 by 100", got[0].W, got[0].H)
	}
}

// TestAUniformViewBoxCentresWhatIsLeftOver. "meet" fits the whole viewBox in and
// shares the remainder between the two edges, which is where the picture sits and
// not merely how large it is.
func TestAUniformViewBoxCentresWhatIsLeftOver(t *testing.T) {
	// A square viewBox in a 200x100 element: scale 1, and 50 spare on each side.
	ops := paintOf(t, `<div id="d"><svg width="200" height="100" viewBox="0 0 100 100">`+
		`<rect x="0" y="0" width="100" height="100" fill="blue"/></svg></div>`,
		`#d { font-size: 0 }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1", len(got))
	}
	if got[0].W != bgpx(100) || got[0].H != bgpx(100) {
		t.Fatalf("the mark is %v by %v, want 100 by 100", got[0].W, got[0].H)
	}
	// The element's own box, to measure the offset against.
	box := fillsOf(paintOf(t, `<div id="d"><svg width="200" height="100"><rect `+
		`width="100%" height="100%" fill="blue"/></svg></div>`,
		`#d { font-size: 0 }`), blue)
	if len(box) != 1 {
		t.Fatalf("the reference fixture painted %d fills", len(box))
	}
	if got[0].X != box[0].X.Add(bgpx(50)) {
		t.Errorf("the picture starts at %v and the box at %v; 100 units in a 200 "+
			"wide box leaves 50 on each side", got[0].X, box[0].X)
	}
}

// TestPercentagesResolveAgainstTheViewport, which is what "width=100%" on a rect
// means and is how the suite writes a full-bleed fill.
func TestPercentagesResolveAgainstTheViewport(t *testing.T) {
	got := svgFills(t,
		`<svg width="80" height="40"><rect x="50%" y="0" width="50%" height="100%" fill="blue"/></svg>`,
		`#d { font-size: 0 }`, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1: %v", len(got), got)
	}
	if got[0].W != bgpx(40) || got[0].H != bgpx(40) {
		t.Errorf("the mark is %v by %v; half of 80 by all of 40 is 40 by 40",
			got[0].W, got[0].H)
	}
}

// TestSeveralRectanglesAreAllPainted, in source order, which is SVG's painting
// order and is what makes a red rectangle under a green one invisible.
func TestSeveralRectanglesAreAllPainted(t *testing.T) {
	ops := paintOf(t, `<div id="d"><svg width="100" height="100">`+
		`<rect width="100" height="100" fill="rgb(255,0,0)"/>`+
		`<rect width="100" height="100" fill="rgb(0,0,255)"/></svg></div>`,
		`#d { font-size: 0 }`)
	var order []style.RGBA
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && (r.Color == blue || r.Color == (style.RGBA{R: 255, A: 1})) {
			order = append(order, r.Color)
		}
	}
	if len(order) != 2 {
		t.Fatalf("%d fills, want both rectangles: %v", len(order), order)
	}
	if order[0] != (style.RGBA{R: 255, A: 1}) || order[1] != blue {
		t.Errorf("the fills came out %v; SVG paints in source order, so the second "+
			"rectangle is on top", order)
	}
}

// TestAnSVGThisCannotDrawIsStillABox is the containment case, and the half that
// keeps the finding honest.
//
// A circle is something there is no operation for. The element is still laid out
// — at its own size, which it states — and nothing is painted inside it, and the
// caller is told. Dropping the box instead was the old behaviour and is what made
// a page look plausible with a hole in it.
func TestAnSVGThisCannotDrawIsStillABox(t *testing.T) {
	built := Build(Input{
		HTML: `<div id="d"><svg width="120" height="60"><circle cx="30" cy="30" r="20" fill="blue"/></svg></div>`,
		CSS:  []Stylesheet{{Source: `#d { font-size: 0 }`}},
	})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)

	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found == nil && f.Box != nil && f.Box.Element != nil && f.Box.Element.Name == "svg" {
			found = f
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if found == nil {
		t.Fatal("an svg with an undrawable picture produced no box")
	}
	if r := found.ContentRect(); r.W != bgpx(120) || r.H != bgpx(60) {
		t.Errorf("the box is %v by %v, want the 120 by 60 the element states",
			r.W, r.H)
	}
	if got := fillsOf(Paint(frag), blue); len(got) != 0 {
		t.Errorf("a circle was painted as %v", got)
	}
	all := append(append([]Finding{}, built.Findings...), rec.Findings()...)
	if !hasRule(all, RuleUnsupportedElement) {
		t.Errorf("an svg this engine cannot draw was not reported: %v", all)
	}
}
