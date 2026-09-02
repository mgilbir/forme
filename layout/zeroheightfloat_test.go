package layout

import "testing"

// A float with no height, and the lines it still shortens.
//
// §9.5 shortens a line box beside a float, and an empty floated <div> is the
// oldest clearance hack there is — so the rule has to say which lines such a
// float is beside. It is the ones whose vertical span holds its y *strictly*
// inside: a float at 30 shortens a line running from 20 to 40, and shortens
// neither the line that ends at 30 nor the line that begins there.
//
// The suite's floats-zero-height-wrap-002 asks all three at once, and its
// reference writes the answer as three margins: ten, a hundred, nothing.

const zeroFloatCSS = `body { margin: 0 } .box { width: 500px; font-size: 12px }
	span { display: inline-block; vertical-align: bottom; height: 20px; width: 300px }`

// zeroFloatLines lays out three 20px lines in a 500px block and returns where
// each of them starts.
func zeroFloatLines(t *testing.T, floats string) [3]float64 {
	t.Helper()
	root := layoutOf(t, 800, `<div class="box">`+floats+
		`<span id="a"></span> <span id="b"></span> <span id="c"></span></div>`,
		zeroFloatCSS)
	var out [3]float64
	for i, id := range []string{"a", "b", "c"} {
		out[i] = find(t, root, id).BorderRect.X.Px()
	}
	return out
}

// TestAFloatWithNoHeightShortensTheLineAroundIt.
func TestAFloatWithNoHeightShortensTheLineAroundIt(t *testing.T) {
	got := zeroFloatLines(t,
		`<div style="float:left;width:10px;height:30px"></div>`+
			`<div style="float:left;clear:left;width:100px;height:0"></div>`)
	if want := [3]float64{10, 100, 0}; got != want {
		t.Errorf("the three lines start at %v, want %v — the second runs from "+
			"20 to 40 and the hundred-pixel float of no height sits at 30, "+
			"inside it", got, want)
	}
}

// TestAFloatWithNoHeightIsNotOnTheLinesItTouches is the other half, and it is
// the half that keeps the clearance hack working: a line that ends where such a
// float begins is above it, and a line that begins there is below it.
func TestAFloatWithNoHeightIsNotOnTheLinesItTouches(t *testing.T) {
	got := zeroFloatLines(t,
		`<div style="float:left;width:10px;height:20px"></div>`+
			`<div style="float:left;clear:left;width:100px;height:0"></div>`)
	if want := [3]float64{10, 0, 0}; got != want {
		t.Errorf("the three lines start at %v, want %v — the float of no height "+
			"is at 20, where the first line ends and the second begins, so it "+
			"is beside neither", got, want)
	}
}

// TestAnEmptyFloatAtTheTopMovesNothing. The clearance hack in its plainest
// form: nothing precedes it, so it sits at zero and the first line begins
// there too.
func TestAnEmptyFloatAtTheTopMovesNothing(t *testing.T) {
	got := zeroFloatLines(t, `<div style="float:left;width:100px;height:0"></div>`)
	if want := [3]float64{0, 0, 0}; got != want {
		t.Errorf("the three lines start at %v, want %v — an empty float at the "+
			"top of a block is beside no line at all", got, want)
	}
}

// TestAFloatWithNoHeightShortensFromItsOwnSide. The right-hand staircase is a
// separate array with the opposite comparison in it, so it needs its own row.
//
// 500 less 450 leaves fifty, which is not room for the box, so the line that
// holds the float gives way and the box goes in the band that begins at it.
func TestAFloatWithNoHeightShortensFromItsOwnSide(t *testing.T) {
	at := func(flat string) float64 {
		t.Helper()
		root := layoutOf(t, 800,
			`<div class="box">`+
				`<div style="float:right;width:10px;height:30px"></div>`+flat+
				`<span id="a"></span> <span id="b"></span></div>`, zeroFloatCSS)
		return find(t, root, "b").BorderRect.Y.Px()
	}
	const wide = `<div style="float:right;clear:right;width:450px;height:0"></div>`
	if got := at(wide); got != 30 {
		t.Errorf("the second box is at y=%g, want 30 — the line from 20 to 40 "+
			"has a 450px float of no height at 30 in it and fifty pixels left",
			got)
	}
	// The same float narrow enough to leave room changes nothing, which is what
	// says the drop above was the float's doing and not the clear's.
	narrow := `<div style="float:right;clear:right;width:100px;height:0"></div>`
	if got := at(narrow); got != 20 {
		t.Errorf("with a 100px float of no height the second box is at y=%g, "+
			"want 20 — four hundred pixels is room for it", got)
	}
}
