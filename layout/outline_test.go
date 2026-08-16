package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §18.4's outline: a ring drawn just outside the border edge, which
// takes no space and is painted after everything else.
//
// The three facts worth pinning are the three that make it not a border. It is
// outside the box rather than inside it, it does not move anything, and it goes
// on last.

// greenFills returns the fills of one colour, which for these fixtures is the
// outline and nothing else.
func fillsOfColour(ops []Op, want style.RGBA) []FillRect {
	var out []FillRect
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && r.Color == want {
			out = append(out, r)
		}
	}
	return out
}

var outlineGreen = style.RGBA{R: 0, G: 128, B: 0, A: 1}

// TestAnOutlineIsDrawnOutsideTheBorderEdge is §18.4's geometry: the ring's inner
// edge is the border box, so its outer edge is the border box grown by the
// width on every side.
func TestAnOutlineIsDrawnOutsideTheBorderEdge(t *testing.T) {
	ops := paintOf(t, `<div id="d">x</div>`,
		`#d { width: 100px; height: 40px; outline: 5px solid green }`)
	fills := fillsOfColour(ops, outlineGreen)
	if len(fills) != 4 {
		t.Fatalf("%d green fills, want the four bands of a ring: %v", len(fills), fills)
	}

	// The union of the four bands is the outer rectangle, and none of them may
	// reach inside the border box.
	var minX, minY, maxX, maxY style.Unit
	for i, f := range fills {
		if i == 0 || f.Rect.X < minX {
			minX = f.Rect.X
		}
		if i == 0 || f.Rect.Y < minY {
			minY = f.Rect.Y
		}
		if i == 0 || f.Rect.Right() > maxX {
			maxX = f.Rect.Right()
		}
		if i == 0 || f.Rect.Bottom() > maxY {
			maxY = f.Rect.Bottom()
		}
	}
	px(t, "the ring's width", maxX.Sub(minX), 110) // 100 + 5 either side
	px(t, "the ring's height", maxY.Sub(minY), 50) // 40 + 5 either side
	// Every band is exactly one outline-width thick on one axis. Without this
	// the union above would be satisfied by a single filled rectangle covering
	// the box, which is what an outline drawn as a box rather than a ring is.
	thick, _ := style.FromPx(5)
	for _, f := range fills {
		if f.Rect.W != thick && f.Rect.H != thick {
			t.Errorf("a band is %vx%v and neither side is the 5px width, so it is "+
				"a filled box rather than an edge of a ring", f.Rect.W, f.Rect.H)
		}
	}
	// And nothing is painted over the middle of the box.
	inner := Rect{X: minX.Add(thick), Y: minY.Add(thick),
		W: maxX.Sub(minX).Sub(thick).Sub(thick), H: maxY.Sub(minY).Sub(thick).Sub(thick)}
	for _, f := range fills {
		if f.Rect.X > inner.X && f.Rect.Right() < inner.Right() &&
			f.Rect.Y > inner.Y && f.Rect.Bottom() < inner.Bottom() {
			t.Errorf("the band at %v is inside the border box; an outline is drawn "+
				"outside it", f.Rect)
		}
	}
}

// TestAnOutlineTakesNoSpace is the sentence §18.4 leads with. It is the whole
// reason an outline is not a border, and the way to see it is that adding one
// moves nothing.
func TestAnOutlineTakesNoSpace(t *testing.T) {
	const markup = `<div id="a">one</div><div id="b">two</div>`
	plain := layoutOf(t, A4.Content().W.Px(), markup, `#a { height: 20px }`)
	ringed := layoutOf(t, A4.Content().W.Px(), markup,
		`#a { height: 20px; outline: 12px solid green }`)

	got := find(t, ringed, "b").BorderRect
	want := find(t, plain, "b").BorderRect
	if got != want {
		t.Errorf("the box after an outlined one is at %v and was at %v; an "+
			"outline moved something, and §18.4 says it takes no space",
			got, want)
	}
}

// TestAnOutlineIsPaintedOverWhatIsBesideIt is §E.2 step 10, and it is the
// consequence of the geometry above: the ring is outside its own box, so it
// lies over the next box's background, and painting it with its own border
// would put that background on top of it.
func TestAnOutlineIsPaintedOverWhatIsBesideIt(t *testing.T) {
	ops := paintOf(t,
		`<div id="a">one</div><div id="b">two</div>`,
		`#a { height: 20px; outline: 8px solid green }
		 #b { height: 40px; background: red }`)

	lastRed, firstGreen := -1, -1
	for i, op := range ops {
		r, ok := op.(FillRect)
		if !ok {
			continue
		}
		if r.Color.R == 255 && r.Color.G == 0 {
			lastRed = i
		}
		if r.Color == outlineGreen && firstGreen < 0 {
			firstGreen = i
		}
	}
	if lastRed < 0 || firstGreen < 0 {
		t.Fatalf("the fixture drew red at %d and green at %d", lastRed, firstGreen)
	}
	if firstGreen < lastRed {
		t.Error("the outline is painted before a later box's background, so that " +
			"background covers it; §E.2 puts outlines in step 10, after everything")
	}
}

// TestAnOutlineIsOverhang.
//
// The overflow-page guardrail reads the display list for boxes leaving the
// paper, and an outline is by definition outside its box: no layout decision
// accounted for its position, so a ring near the page edge must not be read as
// a box that overflowed. That guard refuses the document outright, so getting
// this wrong turns a two-pixel ring into no output at all.
func TestAnOutlineIsOverhang(t *testing.T) {
	ops := paintOf(t, `<div id="d">x</div>`,
		`#d { width: 50px; height: 20px; outline: 4px solid green }`)
	fills := fillsOfColour(ops, outlineGreen)
	if len(fills) == 0 {
		t.Fatal("no outline was drawn")
	}
	for _, f := range fills {
		if !f.Overhang {
			t.Errorf("the band at %v is not marked Overhang, so the overflow "+
				"guardrail will read it as a box leaving the page", f.Rect)
		}
	}
}

// TestOutlineNoneDrawsNothingAndSaysNothing.
//
// Fourteen of the suite's uses of this property are "outline: none", and a
// document that asks for no outline and gets none is a document that came out
// exactly as written. Reporting it would be the engine calling its own correct
// output a limitation.
func TestOutlineNoneDrawsNothingAndSaysNothing(t *testing.T) {
	for _, css := range []string{
		`#d { outline: none }`,
		`#d { outline: 5px none green }`,
		`#d { outline: 0 solid green }`,
	} {
		built := Build(Input{HTML: `<div id="d">x</div>`, CSS: []Stylesheet{{Source: css}}})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(A4.Content().W.Px())
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h}, nil, rec)
		for _, f := range append(built.Findings, rec.Findings()...) {
			t.Errorf("%s reported %s", css, f.Error())
		}
		if n := len(fillsOfColour(Paint(frag), outlineGreen)); n != 0 {
			t.Errorf("%s drew %d green fills", css, n)
		}
	}
}

// TestOutlineColorInvertIsReportedRatherThanGuessed.
//
// "invert" is CSS 2.1's initial value for outline-color and asks for the pixels
// underneath to be inverted. A display list of fills cannot express that without
// reading back what it has drawn, and an outline in a colour this engine chose
// would be a mark on the page that no one asked for and nothing declared.
func TestOutlineColorInvertIsReportedRatherThanGuessed(t *testing.T) {
	for _, css := range []string{
		`#d { outline: 5px solid invert }`,
		// The initial value, reached by naming no colour at all.
		`#d { outline: 5px solid }`,
		`#d { outline-style: solid; outline-width: 5px }`,
	} {
		built := Build(Input{HTML: `<div id="d">x</div>`, CSS: []Stylesheet{{Source: css}}})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(A4.Content().W.Px())
		h, _ := style.FromPx(10000)
		Layout(built.Root, Size{W: w, H: h}, nil, rec)

		var said bool
		for _, f := range append(built.Findings, rec.Findings()...) {
			if f.Rule == RuleUnsupportedValue && f.Property == "outline-color" {
				said = true
			}
		}
		if !said {
			t.Errorf("%s drew or ignored an inverted outline without saying so: %v",
				css, rec.Findings())
		}
	}
}

// TestOutlineStyleHiddenIsNotAnOutlineStyle: §18.4 accepts the border styles
// "except that 'hidden' is not a legal outline style". The word means "this
// border loses to its neighbour" in the collapsing table model, and an outline
// has no neighbours.
func TestOutlineStyleHiddenIsNotAnOutlineStyle(t *testing.T) {
	built := Build(Input{
		HTML: `<div id="d">x</div>`,
		CSS:  []Stylesheet{{Source: `#d { outline: 5px hidden green }`}},
	})
	// An invalid shorthand sets nothing, so the width keeps its initial value
	// rather than the 5px the declaration named.
	var d *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b == nil {
			return
		}
		if b.Element != nil {
			if id, _ := b.Element.Attr("id"); id == "d" {
				d = b
			}
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if d == nil {
		t.Fatal("no box for #d")
	}
	if got := d.Style["outline-width"]; got == "5px" {
		t.Error("\"outline: 5px hidden green\" was accepted; 'hidden' is not a " +
			"legal outline style and an invalid shorthand sets nothing")
	}
}
