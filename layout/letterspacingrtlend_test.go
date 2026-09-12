package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §8.2's trailing letter-spacing hangs off the end of a right-to-left line the
// same way it hangs off a left-to-right one.
//
// The fill discounts it from the line's *measure* — "a line's measure ends at
// its last glyph", as the comment beside that subtraction puts it, and without
// the discount a centred or right-aligned line of tracked text sits a tracking
// width off. What it must not also do is *move* the line by it.
//
// A right-to-left line is shifted by everything discounted from its measure,
// because a right-to-left line ends at its left and what was discounted hangs
// off that edge: a trailing space does, and so does §8.4's hanging punctuation.
// The tracking does not. A run is drawn from its own origin accumulating
// advances with the spacing added after each glyph, so the one after the last
// glyph drawn is at the run's **right** edge whatever the run's direction —
// letterspacingboundary.go establishes that against the display list rather than
// reasoning about it. Including it in the shift discounted it and then handed it
// straight back, which cancelled the fix for exactly the lines that needed it.
//
// The invariant is a symmetry rather than an absolute, so it does not restate
// the arithmetic it is checking: the tracking hangs past the right edge of a
// right-aligned left-to-right line, and it must hang past the right edge of a
// right-to-left line by the same amount. Under the old shift the left-to-right
// line overhung by the tracking and the right-to-left one by nothing.
//
// The suite measures the same thing at letter-spacing-bidi-003, whose "dir=rtl"
// line was displaced by exactly 1ch while the left-to-right line above it was
// correct glyph for glyph.

// farEdgeOf is the rightmost X+Width over a line's runs, which is where the
// content reaches including anything hanging past the block's edge.
func farEdgeOf(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	root := layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 3000px }`+css)
	runs := runsOf(t, root, "p")
	if len(runs) == 0 {
		t.Fatalf("%q produced no runs", markup)
	}
	edge := runs[0].X.Add(runs[0].Width)
	for _, r := range runs[1:] {
		if e := r.X.Add(r.Width); e > edge {
			edge = e
		}
	}
	return edge
}

func TestTheTrailingTrackingHangsOffEitherDirectionsEnd(t *testing.T) {
	const tracking = `letter-spacing: 20px`
	for _, tc := range []struct {
		markup, ltr, rtl, what string
	}{
		{"abc", `text-align: right`, `direction: rtl`,
			"left-to-right text"},
		{"&#x5d0;&#x5d1;&#x5d2;", `text-align: right`, `direction: rtl`,
			"right-to-left text"},
	} {
		ltrPlain := farEdgeOf(t, tc.markup, `#p {`+tc.ltr+`}`)
		ltrTracked := farEdgeOf(t, tc.markup, `#p {`+tc.ltr+`;`+tracking+`}`)
		rtlPlain := farEdgeOf(t, tc.markup, `#p {`+tc.rtl+`}`)
		rtlTracked := farEdgeOf(t, tc.markup, `#p {`+tc.rtl+`;`+tracking+`}`)

		ltrHang := ltrTracked.Sub(ltrPlain)
		rtlHang := rtlTracked.Sub(rtlPlain)
		if ltrHang == 0 {
			t.Fatalf("%s: the tracking hangs off nothing even left-to-right, so "+
				"this measures the wrong thing", tc.what)
		}
		if ltrHang != rtlHang {
			t.Errorf("%s: the tracking hangs %v past a right-aligned "+
				"left-to-right line and %v past a right-to-left one — the same "+
				"spacing sits at the same edge whatever the direction",
				tc.what, ltrHang, rtlHang)
		}
	}
}
