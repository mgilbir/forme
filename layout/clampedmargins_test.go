package layout

import (
	"testing"
)

// What happens to the margins when min-width or max-width changes an auto
// width, CSS 2.1 §10.4:
//
//	If the tentative used width is greater than 'max-width', the rules above
//	are applied again, but this time using the computed value of 'max-width'
//	as the computed value for 'width'.
//
// and the same for 'min-width'. "The rules above" are §10.3.3's, so the margins
// are solved again against the clamped width. resolveWidth used to return the
// clamped width with the margins the fill had solved for — both zero — so a
// narrowed box sat at the left edge whatever its margins or its parent's
// direction said: "max-width: 100px; margin: 0 auto" did not centre, and a
// narrowed box in a right-to-left block did not hang from the right.

// clampedBox lays out one 10px-tall box in a 400px parent with the given
// direction and returns its fragment, so both the position and the resolved
// margins can be read.
func clampedBox(t *testing.T, dir, child string) *Fragment {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p"><div id="c"></div></div>`,
		noDefaults+`#p { width: 400px; direction: `+dir+` } #c { height: 10px; `+child+` }`)
	return find(t, root, "c")
}

// TestAClampedWidthResolvesItsMarginsAgain.
func TestAClampedWidthResolvesItsMarginsAgain(t *testing.T) {
	for _, tc := range []struct {
		what, dir, child  string
		x, w, left, right float64
	}{
		// The over-constrained case: no auto margin, so the end margin gives
		// way, and in right-to-left the end is the left.
		{"a maximum, left to right", "ltr", "max-width: 100px", 0, 100, 0, 300},
		{"a maximum, right to left", "rtl", "max-width: 100px", 300, 100, 300, 0},
		// Auto margins take the slack the clamp opened, whichever the direction.
		{"a maximum, centred", "ltr", "max-width: 100px; margin: 0 auto", 150, 100, 150, 150},
		{"a maximum, centred, right to left", "rtl", "max-width: 100px; margin: 0 auto", 150, 100, 150, 150},
		{"a maximum, pushed right", "ltr", "max-width: 100px; margin-left: auto", 300, 100, 300, 0},
		{"a maximum, pushed left in right to left", "rtl", "max-width: 100px; margin-right: auto", 0, 100, 0, 300},
		// A declared margin stays as written and the end one gives way.
		{"a maximum with a start margin, right to left", "rtl",
			"max-width: 100px; margin-right: 20px", 280, 100, 280, 20},
		// A minimum above the fill makes the box too wide, and the overflow
		// goes to the end: off the right edge in left to right, off the left
		// in right to left. §10.3.3's first sentence then treats the auto
		// margins as zero rather than sharing out a slack there is none of.
		{"a minimum, left to right", "ltr", "min-width: 500px", 0, 500, 0, -100},
		{"a minimum, right to left", "rtl", "min-width: 500px", -100, 500, -100, 0},
		{"a minimum with auto margins, right to left", "rtl",
			"min-width: 500px; margin: 0 auto", -100, 500, -100, 0},
		// A box-sizing: border-box maximum names the border box, so the slack
		// is what the border box leaves.
		{"a border-box maximum, right to left", "rtl",
			"box-sizing: border-box; max-width: 100px; padding: 0 10px", 300, 100, 300, 0},
	} {
		f := clampedBox(t, tc.dir, tc.child)
		if got := f.BorderRect.W; got != bgpx(tc.w) {
			t.Errorf("%s: the border box is %v wide, want %v", tc.what, got, bgpx(tc.w))
		}
		if got := f.BorderRect.X; got != bgpx(tc.x) {
			t.Errorf("%s: the box starts at %v, want %v — §10.4 solves §10.3.3 again "+
				"with the clamped width", tc.what, got, bgpx(tc.x))
		}
		if f.Margin.Left != bgpx(tc.left) || f.Margin.Right != bgpx(tc.right) {
			t.Errorf("%s: the margins came out %v and %v, want %v and %v", tc.what,
				f.Margin.Left, f.Margin.Right, bgpx(tc.left), bgpx(tc.right))
		}
	}
}

// TestAnUnclampedAutoWidthStillFills is the control: a limit the fill does not
// break changes nothing, and the auto margins against an auto width are zero.
func TestAnUnclampedAutoWidthStillFills(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		f := clampedBox(t, dir, "max-width: 500px; min-width: 100px; margin: 0 auto")
		if f.BorderRect.X != 0 || f.BorderRect.W != bgpx(400) ||
			f.Margin.Left != 0 || f.Margin.Right != 0 {
			t.Errorf("%s: got x %v, width %v, margins %v and %v; want a box filling "+
				"its parent with no margins", dir, f.BorderRect.X, f.BorderRect.W,
				f.Margin.Left, f.Margin.Right)
		}
	}
}

// TestAClampedWidthBesideAFloatResolvesItsMargins is the same rule in the band
// beside a float, where avoidFloats places a box that may not overlap one. It
// resolved auto margins only for a declared width, so a narrowed box stayed at
// the band's start: the band is the 300px right of a 100px float, and a 100px
// box centred in it starts at 200.
func TestAClampedWidthBesideAFloatResolvesItsMargins(t *testing.T) {
	for _, tc := range []struct {
		what, dir, child string
		x                float64
	}{
		{"centred", "ltr", "max-width: 100px; margin: 0 auto", 200},
		{"centred, right to left", "rtl", "max-width: 100px; margin: 0 auto", 200},
		{"pushed to the end", "ltr", "max-width: 100px; margin-left: auto", 300},
		// The controls: a declared width, which always worked, and a clamped
		// width with no auto margin, which in right to left hangs from the
		// right edge and in left to right sits against the float.
		{"a declared width, centred", "ltr", "width: 100px; margin: 0 auto", 200},
		{"no auto margin, right to left", "rtl", "max-width: 100px", 300},
		{"no auto margin", "ltr", "max-width: 100px", 100},
	} {
		root := layoutOf(t, 600, `<div id="p"><div id="f"></div><div id="c"></div></div>`,
			noDefaults+`#p { width: 400px; direction: `+tc.dir+` }
			#f { float: left; width: 100px; height: 50px }
			#c { overflow: hidden; height: 10px; `+tc.child+` }`)
		f := find(t, root, "c")
		if f.BorderRect.Y != 0 || f.BorderRect.W != bgpx(100) {
			t.Fatalf("%s: the box is at y %v and %v wide; want it beside the float "+
				"at 100px", tc.what, f.BorderRect.Y, f.BorderRect.W)
		}
		if got := f.BorderRect.X; got != bgpx(tc.x) {
			t.Errorf("%s: the box starts at %v, want %v", tc.what, got, bgpx(tc.x))
		}
	}
}

// TestATableBesideAFloatIsCentredInTheBand is the other width the band does not
// decide. §17.4 makes a table's wrapper as wide as the table, and resolveWidth
// solves a wrapper's auto margins against that width as though it had been
// declared — which is what centres "margin: 0 auto" tables. Beside a float the
// same box is fitted to the band, and its auto margins were left at zero, so
// the table sat against the float instead of in the middle of the room it has.
func TestATableBesideAFloatIsCentredInTheBand(t *testing.T) {
	for _, tc := range []struct {
		margin string
		x      float64
	}{
		{"margin: 0 auto", 200},
		{"margin-left: auto", 300},
		{"", 100},
	} {
		root := layoutOf(t, 600, `<div id="p"><div id="f"></div><table id="c"><tr><td>`+
			`<div style="width: 100px; height: 5px"></div></td></tr></table></div>`,
			noDefaults+`#p { width: 400px } #f { float: left; width: 100px; height: 50px }
			table { border-spacing: 0 } td { padding: 0 } #c { `+tc.margin+` }`)
		f := find(t, root, "c")
		if f.BorderRect.Y != 0 || f.BorderRect.W != bgpx(100) {
			t.Fatalf("%q: the table is at y %v and %v wide; want it beside the float "+
				"at 100px", tc.margin, f.BorderRect.Y, f.BorderRect.W)
		}
		if got := f.BorderRect.X; got != bgpx(tc.x) {
			t.Errorf("%q: the table starts at %v, want %v", tc.margin, got, bgpx(tc.x))
		}
	}
}
