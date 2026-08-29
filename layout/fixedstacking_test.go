package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// What makes a stacking context, CSS 2.2 §9.9.1.
//
//	auto: The stack level of the generated box in the current stacking context
//	is 0. If the box has 'position: fixed' or if it is the root, it also
//	establishes a new stacking context.
//
// The second sentence is CSS 2.2's addition — the changes appendix the suite's
// fixed-pos-stacking-001 links from — and it was missing. A "z-index: -1" inside
// a fixed box was hoisted into the context around it and painted under the
// page's background, which is what that test draws in red to be covered.

// stackFills paints a document and returns the fills of one colour, in the order
// they were painted.
func stackFills(t *testing.T, htmlSrc, cssSrc string, want style.RGBA) []Rect {
	t.Helper()
	return fillsOf(paintOf(t, htmlSrc, noDefaults+cssSrc), want)
}

// TestAFixedBoxSealsItsDescendants is the rule.
//
// The negative z-index inside the fixed box belongs to the context the fixed box
// makes, so it is painted inside it — above the fixed box's own background,
// which is what "sealed" comes to on the page. Hoisted out instead, it goes
// behind everything the fixed box is behind.
func TestAFixedBoxSealsItsDescendants(t *testing.T) {
	ops := paintOf(t,
		`<div id="outer"><div id="inner"><div id="deep"></div></div></div>`,
		noDefaults+
			`#outer { position: fixed; left: 0; top: 0; width: 100px; height: 100px; `+
			`background: rgb(255,0,0) } `+
			`#inner { position: absolute; left: 0; top: 0; width: 100px; height: 100px } `+
			`#deep { position: absolute; z-index: -1; left: 0; top: 0; `+
			`width: 100px; height: 100px; background: rgb(0,128,0) }`)
	order := paintOrder(ops)
	if len(order) != 2 {
		t.Fatalf("%d fills, want the fixed box's and the negative one: %v", len(order), order)
	}
	if order[0] != red || order[1] != green {
		t.Errorf("the fills came out %v; the negative z-index is inside the context "+
			"the fixed box makes, so it is painted over the fixed box's own "+
			"background and not under it", order)
	}
}

// TestAnAbsoluteBoxDoesNotSealItsDescendants is the containment half, and the
// half §9.9.1's first sentence is: "z-index: auto" on anything but a fixed box
// makes no context, so a negative z-index inside it belongs to the context
// around it and is painted behind that box.
func TestAnAbsoluteBoxDoesNotSealItsDescendants(t *testing.T) {
	ops := paintOf(t,
		`<div id="outer"><div id="inner"><div id="deep"></div></div></div>`,
		noDefaults+
			`#outer { position: absolute; left: 0; top: 0; width: 100px; height: 100px; `+
			`background: rgb(255,0,0) } `+
			`#inner { position: absolute; left: 0; top: 0; width: 100px; height: 100px } `+
			`#deep { position: absolute; z-index: -1; left: 0; top: 0; `+
			`width: 100px; height: 100px; background: rgb(0,128,0) }`)
	order := paintOrder(ops)
	if len(order) != 2 {
		t.Fatalf("%d fills, want two: %v", len(order), order)
	}
	if order[0] != green || order[1] != red {
		t.Errorf("the fills came out %v; an absolute box with an auto z-index is no "+
			"stacking context, so the negative one inside it is hoisted out and "+
			"painted first", order)
	}
}

// TestAFixedBoxStillStacksAtZeroInTheContextAroundIt. Sealing what is inside it
// says nothing about where the box itself goes: §9.9.1's first sentence gives it
// stack level 0 like any other auto, so a "z-index: 1" sibling is still above it.
func TestAFixedBoxStillStacksAtZeroInTheContextAroundIt(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div><div id="b"></div>`,
		noDefaults+
			`#a { position: fixed; left: 0; top: 0; width: 100px; height: 100px; `+
			`background: rgb(255,0,0) } `+
			`#b { position: absolute; z-index: 1; left: 0; top: 0; `+
			`width: 100px; height: 100px; background: rgb(0,128,0) }`)
	order := paintOrder(ops)
	if len(order) != 2 || order[0] != red || order[1] != green {
		t.Errorf("the fills came out %v; the fixed box is at level 0 and the "+
			"sibling at 1, so the sibling is painted over it", order)
	}
}

// paintOrder is the colour of each filled rectangle, in the order painted.
func paintOrder(ops []Op) []style.RGBA {
	var out []style.RGBA
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && r.Color.A != 0 && !r.Rect.Empty() {
			out = append(out, r.Color)
		}
	}
	return out
}

// red and green are declared in visibility_test.go.
