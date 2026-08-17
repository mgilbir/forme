package layout

import (
	"reflect"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A gradient of one colour, which is that colour.
//
// The web writes linear-gradient(green, green) where it wants a solid swatch,
// because CSS has no way to say "an image that is a colour" and background-color
// cannot be sized or placed. Forty of the forty-two gradients in the suite are
// that, and reading them as unpaintable left the page blank where an author had
// asked for a rectangle.

// green is declared in visibility_test.go, and is the same rgb(0,128,0) the
// suite's documents use to mean "this passed".

// TestAUniformGradientIsReadAsItsColour is the parse, and the table is mostly
// about what must *not* be read as uniform — a value this engine gets wrong here
// paints a colour a browser does not.
func TestAUniformGradientIsReadAsItsColour(t *testing.T) {
	red := style.RGBA{R: 255, A: 1}
	for _, tc := range []struct {
		value string
		want  style.RGBA
		ok    bool
	}{
		{"linear-gradient(red, red)", red, true},
		{"linear-gradient(green, green)", green, true},
		{"linear-gradient(to right, green, green)", green, true},
		{"linear-gradient(45deg, #008000, #008000)", green, true},
		{"linear-gradient(to bottom left, red, red)", red, true},
		// Stop positions cannot change the colour of a gradient whose stops are
		// one colour, so they are skipped rather than resolved.
		{"linear-gradient(to right, red 0%, red 100%)", red, true},
		{"linear-gradient(red 0px, red 4em)", red, true},
		// Every repetition of a repeating gradient paints the same thing.
		{"repeating-linear-gradient(red, red)", red, true},
		{"linear-gradient(rgb(0,128,0), rgb(0,128,0))", green, true},

		// Two colours is a gradient, and this engine cannot paint one.
		{"linear-gradient(red, blue)", style.RGBA{}, false},
		// One unit apart is still two colours. Nothing here is approximate.
		{"linear-gradient(rgb(0,128,0), rgb(0,128,1))", style.RGBA{}, false},
		{"linear-gradient(to right, green 4em, red 3em)", style.RGBA{}, false},
		// One stop is not a gradient any specification allows. Painting it
		// would be this engine reading a value differently from a browser.
		{"linear-gradient(red)", style.RGBA{}, false},
		// A stop that is not a colour makes the gradient unreadable, not
		// uniform — the alternative is dropping a stop from the comparison.
		{"linear-gradient(green, notacolour)", style.RGBA{}, false},
		{"linear-gradient(to nowhere, red, red)", style.RGBA{}, false},
		// Untested shapes are refused rather than guessed at.
		{"radial-gradient(red, red)", style.RGBA{}, false},
		{"conic-gradient(red, red)", style.RGBA{}, false},
		{"url(x.png)", style.RGBA{}, false},
		{"none", style.RGBA{}, false},
		{"", style.RGBA{}, false},
		{"linear-gradient(red, red) url(x.png)", style.RGBA{}, false},
	} {
		got, ok := uniformGradient(tc.value)
		if ok != tc.ok {
			t.Errorf("%s: uniform=%v, want %v", tc.value, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("%s: %v, want %v", tc.value, got, tc.want)
		}
	}
}

// TestAUniformGradientPaintsWhatBackgroundColourPaints is the whole point, and
// it is an equality between two documents rather than an assertion about one.
//
// The two say the same thing in different words, so they must produce the same
// display list — not merely the same pixels. A synthesised one-pixel picture
// stretched over the area would paint identically and compare unequal, which is
// exactly the failure a reftest comparing these two would report.
func TestAUniformGradientPaintsWhatBackgroundColourPaints(t *testing.T) {
	const doc = `<div id="d">x</div>`
	gradient := paintOf(t, doc, `#d { width: 100px; height: 50px;
		background-image: linear-gradient(rgb(0,128,0), rgb(0,128,0)) }`)
	colour := paintOf(t, doc, `#d { width: 100px; height: 50px;
		background-color: rgb(0,128,0) }`)

	g, c := fillsOf(gradient, green), fillsOf(colour, green)
	if len(c) != 1 {
		t.Fatalf("the fixture is wrong: background-color painted %d fills, want 1", len(c))
	}
	if !reflect.DeepEqual(g, c) {
		t.Errorf("the gradient painted %v and background-color painted %v; the two "+
			"documents say the same thing and must draw the same thing", g, c)
	}
}

// TestAGradientIsSizedAndPlacedLikeAnImage. This is why an author writes one: a
// background-color fills the box and cannot be put somewhere else, and a
// gradient is an *image*, so background-size and background-position apply to it.
//
// A gradient has no intrinsic dimensions, so "auto" is the positioning area —
// CSS Images §4 — which is what makes the plain case fill the box at all.
func TestAGradientIsSizedAndPlacedLikeAnImage(t *testing.T) {
	ops := paintOf(t, `<div id="d">x</div>`,
		`#d { width: 100px; height: 100px; background-image: linear-gradient(green, green);
		      background-size: 40px 20px; background-position: 10px 30px;
		      background-repeat: no-repeat }`)
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d green fills, want 1: %v", len(got), got)
	}
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	// The box is at the page's content origin; the fill sits 10 and 30 into it.
	if got[0].W != u(40) || got[0].H != u(20) {
		t.Errorf("the fill is %v by %v, want 40 by 20: background-size was not applied",
			got[0].W, got[0].H)
	}
	box := fillsOf(paintOf(t, `<div id="d">x</div>`,
		`#d { width: 100px; height: 100px; background-color: green }`), green)
	if len(box) != 1 {
		t.Fatalf("the reference fixture painted %d fills", len(box))
	}
	if got[0].X != box[0].X.Add(u(10)) || got[0].Y != box[0].Y.Add(u(30)) {
		t.Errorf("the fill is at (%v,%v) and the box at (%v,%v); background-position "+
			"asked for 10 and 30 into it", got[0].X, got[0].Y, box[0].X, box[0].Y)
	}
}

// TestARealGradientIsStillReported. The line this change draws is between a
// gradient of one colour and a gradient, and the second half of it has to hold:
// a two-colour gradient must paint nothing and say so, exactly as before.
//
// Painting it as one of its colours would be worse than painting nothing,
// because nothing is visibly missing and a wrong colour is not.
func TestARealGradientIsStillReported(t *testing.T) {
	built := Build(Input{
		HTML: `<div id="d">x</div>`,
		CSS: []Stylesheet{{Source: `#d { width: 100px; height: 50px;
			background-image: linear-gradient(red, blue) }`}},
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, nil, rec)
	ops := Paint(frag)
	for _, op := range ops {
		if r, ok := op.(FillRect); ok {
			if r.Color == (style.RGBA{R: 255, A: 1}) || r.Color == (style.RGBA{B: 255, A: 1}) {
				t.Errorf("a two-colour gradient painted %v", r.Color)
			}
		}
	}
	if !hasRule(rec.Findings(), RuleUnsupportedValue) {
		t.Errorf("a gradient this engine cannot paint was not reported: %v", rec.Findings())
	}
}

// TestASpacedTilingOfOneColourKeepsItsGaps.
//
// A tiling of one colour is still a tiling. "background-repeat: space" leaves
// room between its tiles and the gaps show, so the merge that turns an abutting
// tiling into a single fill must not run here — filling the clip would paint over
// exactly what the property asks to leave bare.
func TestASpacedTilingOfOneColourKeepsItsGaps(t *testing.T) {
	ops := paintOf(t, `<div id="d">x</div>`,
		`#d { width: 100px; height: 20px; background-image: linear-gradient(green, green);
		      background-size: 30px 20px; background-repeat: space }`)
	got := fillsOf(ops, green)
	if len(got) < 2 {
		t.Fatalf("%d green fills, want one per tile: a spaced tiling was merged into "+
			"a single fill and its gaps were painted over", len(got))
	}
	for _, r := range got {
		u30, _ := style.FromPx(30)
		if r.W > u30 {
			t.Errorf("a tile is %v wide, more than the 30px background-size asked "+
				"for; the tiles were merged", r.W)
		}
	}
}

// TestAnAbuttingTilingOfOneColourIsOneFill is the other half: where the tiles do
// meet edge to edge, their union is a rectangle and must be drawn as one.
//
// Not for tidiness — a page written with "repeat" and one written with
// background-color paint the same thing, and a reftest comparing them reads the
// display list.
func TestAnAbuttingTilingOfOneColourIsOneFill(t *testing.T) {
	ops := paintOf(t, `<div id="d">x</div>`,
		`#d { width: 100px; height: 20px; background-image: linear-gradient(green, green);
		      background-size: 25px 20px; background-repeat: repeat }`)
	got := fillsOf(ops, green)
	if len(got) != 1 {
		t.Fatalf("%d green fills, want 1: four tiles that meet edge to edge cover a "+
			"rectangle and are one fill\n%v", len(got), got)
	}
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	if got[0].W != u(100) || got[0].H != u(20) {
		t.Errorf("the merged fill is %v by %v, want the whole 100 by 20 box",
			got[0].W, got[0].H)
	}
}
