package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A gradient whose colour never interpolates, which is a stack of solid bands.
//
// The suite writes one wherever it wants a two-colour marker of a stated size:
// "linear-gradient(to bottom, red 50%, green 50%)" is a red half above a green
// half, and there is nothing gradual about it. Reading it as unpaintable left
// five documents with a blank box where the assertion was.

// bandBox is the fixture every test here paints: a 100 by 100 box with one
// background, and the same box painted with a plain colour so that a test can
// say where the box *is* without knowing the page's margins.
const bandBox = `<div id="d">x</div>`

func bandFixture(t *testing.T, background string) (ops []Op, box Rect) {
	t.Helper()
	ops = paintOf(t, bandBox, `#d { width: 100px; height: 100px; `+background+` }`)
	ref := fillsOf(paintOf(t, bandBox, `#d { width: 100px; height: 100px;
		background-color: rgb(1,2,3) }`), style.RGBA{R: 1, G: 2, B: 3, A: 1})
	if len(ref) != 1 {
		t.Fatalf("the reference fixture painted %d fills, want 1", len(ref))
	}
	return ops, ref[0]
}

// TestAHardStopIsTwoSolidBands is the shape the suite writes.
//
// Between the two stops there is no distance for a colour to change over, so the
// picture is a red rectangle and a green one and no interpolation was skipped to
// get there.
func TestAHardStopIsTwoSolidBands(t *testing.T) {
	ops, box := bandFixture(t,
		`background-image: linear-gradient(to bottom, red 50%, green 50%, green 100%)`)

	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	grn := fillsOf(ops, green)
	if len(red) != 1 || len(grn) != 1 {
		t.Fatalf("%d red and %d green fills, want one of each\n%v\n%v", len(red), len(grn), red, grn)
	}
	want := Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(50)}
	if red[0] != want {
		t.Errorf("the red band is %v, want %v (the top half)", red[0], want)
	}
	want = Rect{X: box.X, Y: box.Y.Add(bgpx(50)), W: bgpx(100), H: bgpx(50)}
	if grn[0] != want {
		t.Errorf("the green band is %v, want %v (the bottom half)", grn[0], want)
	}
}

// TestABandRunsToTheEndOfTheLine. A gradient's colour before its first stop is
// the first stop's and after its last is the last stop's, so the bands cover the
// whole box however the stops are placed — CSS Images 3 §3.4.
//
// This is the half a naive reading gets wrong: with stops at 25% and 25% it is
// tempting to paint only between them, which is nothing at all.
func TestABandRunsToTheEndOfTheLine(t *testing.T) {
	ops, box := bandFixture(t,
		`background-image: linear-gradient(to bottom, red 25%, green 25%)`)

	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	grn := fillsOf(ops, green)
	if len(red) != 1 || len(grn) != 1 {
		t.Fatalf("%d red and %d green fills, want one of each\n%v\n%v", len(red), len(grn), red, grn)
	}
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(25)}); red[0] != want {
		t.Errorf("the red band is %v, want %v: the colour before the first stop is "+
			"that stop's", red[0], want)
	}
	if want := (Rect{X: box.X, Y: box.Y.Add(bgpx(25)), W: bgpx(100), H: bgpx(75)}); grn[0] != want {
		t.Errorf("the green band is %v, want %v: the colour after the last stop is "+
			"that stop's", grn[0], want)
	}
}

// TestAStopNeverMovesBackwards is §3.4.2's colour stop fixup, and it is what
// makes "green 4em, red 3em" a hard stop rather than a gradient running
// backwards: the second stop is moved up to the first, and the two then sit at
// the same place.
//
// white-space-intrinsic-size-017 and -018 are written exactly that way.
func TestAStopNeverMovesBackwards(t *testing.T) {
	ops, box := bandFixture(t,
		`font-size: 16px; background-image: linear-gradient(to right, green 4em, red 3em)`)

	grn := fillsOf(ops, green)
	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(grn) != 1 || len(red) != 1 {
		t.Fatalf("%d green and %d red fills, want one of each\n%v\n%v", len(grn), len(red), grn, red)
	}
	// 4em at 16px is 64px, and the box is 100 wide.
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(64), H: bgpx(100)}); grn[0] != want {
		t.Errorf("the green band is %v, want %v", grn[0], want)
	}
	if want := (Rect{X: box.X.Add(bgpx(64)), Y: box.Y, W: bgpx(36), H: bgpx(100)}); red[0] != want {
		t.Errorf("the red band is %v, want %v: the 3em stop should have been moved "+
			"up to the 4em one, not left where it was written", red[0], want)
	}
}

// TestToTopRunsTheBandsTheOtherWay. The gradient line's direction is the whole
// difference between a document that passes and one that fails upside down, and
// a band placed from the wrong edge is a mistake no amount of geometry catches:
// both renderings have a red half and a green half.
func TestToTopRunsTheBandsTheOtherWay(t *testing.T) {
	down, box := bandFixture(t,
		`background-image: linear-gradient(to bottom, red 50%, green 50%)`)
	up, _ := bandFixture(t,
		`background-image: linear-gradient(to top, red 50%, green 50%)`)

	d, u := fillsOf(down, green), fillsOf(up, green)
	if len(d) != 1 || len(u) != 1 {
		t.Fatalf("%d and %d green fills, want one each", len(d), len(u))
	}
	if want := (Rect{X: box.X, Y: box.Y.Add(bgpx(50)), W: bgpx(100), H: bgpx(50)}); d[0] != want {
		t.Errorf("to bottom put green at %v, want %v", d[0], want)
	}
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(50)}); u[0] != want {
		t.Errorf("to top put green at %v, want %v: the line runs from the bottom "+
			"edge, so the last stop's colour is at the top", u[0], want)
	}
}

// TestATransparentBandDrawsNothing. "green 50%, transparent 50%" is how the
// suite says "cover the top half and leave the rest showing", and filling the
// rest with transparent black would paint over what is meant to show through if
// a backend ever took the alpha at face value.
func TestATransparentBandDrawsNothing(t *testing.T) {
	ops, box := bandFixture(t,
		`background-image: linear-gradient(to bottom, green 50%, transparent 50%, transparent 100%)`)

	grn := fillsOf(ops, green)
	if len(grn) != 1 {
		t.Fatalf("%d green fills, want 1: %v", len(grn), grn)
	}
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(50)}); grn[0] != want {
		t.Errorf("the green band is %v, want %v", grn[0], want)
	}
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && r.Color.A == 0 {
			t.Errorf("a transparent band was drawn as %v", r)
		}
	}
}

// TestTheBandsSpanTheTileNotTheBox. A gradient is an image, so the line its
// stops are measured along is the *image's* — which background-size sets and
// background-repeat then tiles. Measuring against the box instead looks right
// in every default case and wrong in every other.
func TestTheBandsSpanTheTileNotTheBox(t *testing.T) {
	ops, box := bandFixture(t, `background-image:
		linear-gradient(to bottom, red 50%, green 50%);
		background-size: 100px 40px; background-repeat: no-repeat`)

	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(red) != 1 {
		t.Fatalf("%d red fills, want 1: %v", len(red), red)
	}
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(20)}); red[0] != want {
		t.Errorf("the red band is %v, want %v: half of the 40px tile, not half of "+
			"the 100px box", red[0], want)
	}
}

// TestEveryTileGetsTheWholeStack is the merge that must not happen.
//
// solidTiles collapses an abutting tiling into one rectangle because every tile
// of one colour paints the same thing everywhere. A banded tile does not, and
// collapsing it would paint the first band over the entire clip.
func TestEveryTileGetsTheWholeStack(t *testing.T) {
	ops, box := bandFixture(t, `background-image:
		linear-gradient(to bottom, red 50%, green 50%);
		background-size: 100px 50px; background-repeat: repeat`)

	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(red) != 2 {
		t.Fatalf("%d red bands, want 2: the box is two tiles tall and each has its "+
			"own\n%v", len(red), red)
	}
	for i, want := range []Rect{
		{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(25)},
		{X: box.X, Y: box.Y.Add(bgpx(50)), W: bgpx(100), H: bgpx(25)},
	} {
		if red[i] != want {
			t.Errorf("red band %d is %v, want %v", i, red[i], want)
		}
	}
}

// TestAGradientThatInterpolatesIsStillReported is the line this draws, and the
// far side of it has to hold. A gradient with two colours over a distance is one
// this display list cannot express, and painting it as either colour would be
// worse than painting nothing: nothing missing is visible and a wrong colour is
// not.
func TestAGradientThatInterpolatesIsStillReported(t *testing.T) {
	for _, value := range []string{
		"linear-gradient(to bottom, lime, green)",
		"linear-gradient(to bottom, red 25%, green 75%)",
		// An angle and a corner are read and refused: their bands would be
		// diagonal strips, which a rectangle cannot express any better than the
		// interpolation could.
		"linear-gradient(45deg, red 50%, green 50%)",
		"linear-gradient(to bottom right, red 50%, green 50%)",
		// A repeating gradient's bands repeat along the line, which is a
		// tiling of stripes and not a stack of them.
		"repeating-linear-gradient(to bottom, red 50%, green 50%)",
		// A length and a percentage in one gradient have an order that depends
		// on the box, so whether this is a hard stop cannot be decided here.
		"linear-gradient(to bottom, green 4em, red 50%)",
		// A bare position between two stops is an interpolation hint, which
		// says the colour between them is *not* either of theirs.
		"linear-gradient(to bottom, red, 30%, green)",
		// Not gradients this engine reads at all.
		"radial-gradient(red 50%, green 50%)",
		"linear-gradient(to nowhere, red 50%, green 50%)",
	} {
		rec := NewRecorder(nil)
		built := Build(Input{
			HTML: bandBox,
			CSS: []Stylesheet{{Source: `#d { width: 100px; height: 100px;
				background-image: ` + value + ` }`}},
		})
		if built.Root == nil {
			t.Fatalf("%s: the document produced no boxes", value)
		}
		ops := Paint(Layout(built.Root, Size{W: bgpx(600), H: bgpx(10000)}, nil, rec))
		for _, op := range ops {
			r, ok := op.(FillRect)
			if !ok {
				continue
			}
			if r.Color == green || r.Color == (style.RGBA{R: 255, A: 1}) ||
				r.Color == (style.RGBA{G: 255, A: 1}) {
				t.Errorf("%s: painted %v", value, r)
			}
		}
		if !hasRule(rec.Findings(), RuleUnsupportedValue) {
			t.Errorf("%s: painted nothing and said nothing", value)
		}
	}
}

// TestAGradientOfOneColourIsStillOneFill. bandsOf subsumes the uniform case —
// "red, red" is a stack of one band — and must not start emitting it as
// something else. A page written that way and a page written with
// background-color paint the same thing, and a reftest comparing them reads the
// display list rather than the pixels.
func TestAGradientOfOneColourIsStillOneFill(t *testing.T) {
	ops, box := bandFixture(t, `background-image: linear-gradient(to bottom, green, green)`)
	grn := fillsOf(ops, green)
	if len(grn) != 1 {
		t.Fatalf("%d green fills, want 1: %v", len(grn), grn)
	}
	if want := (Rect{X: box.X, Y: box.Y, W: bgpx(100), H: bgpx(100)}); grn[0] != want {
		t.Errorf("the fill is %v, want the whole box %v", grn[0], want)
	}
}
