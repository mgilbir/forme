package layout

import (
	"strings"
	"testing"
)

// TestAnItemThatClipsItsOverflowCanShrinkToNothing is §4.5's second clause.
//
// A flex item's automatic minimum is its content-based minimum, which is what
// stops a row of text collapsing to nothing. The clause after it is what makes
// that rule safe: an item that clips its own overflow along the main axis has
// an automatic minimum of zero, because the minimum exists so that content is
// not cut off invisibly and a box that says it will cut its content off has
// asked for exactly that.
//
// Without it the ellipsis idiom cannot work at all: "flex: 1 1 0" with
// "overflow: hidden" on an unbreakable word stayed as wide as the word.
func TestAnItemThatClipsItsOverflowCanShrinkToNothing(t *testing.T) {
	word := strings.Repeat("W", 60)
	doc := `<div id="f"><div id="a">` + word + `</div><div id="b">x</div></div>`
	const base = noDefaults + `#f { display: flex; width: 300px; font-family: Courier;
		font-size: 10px }
		#a { flex: 1 1 0 }
		#b { flex: 0 0 50px }`

	visible, ok := gridRect(t, doc, base, "a")
	if !ok {
		t.Fatal("the item generated no fragment")
	}
	if visible.W.Px() <= 250 {
		t.Fatalf("the item is %.0f wide with visible overflow; it should be as wide as "+
			"the word, so the test below proves nothing", visible.W.Px())
	}

	for _, tc := range []struct{ name, overflow string }{
		{"hidden", "overflow: hidden"},
		{"scroll", "overflow: scroll"},
		{"auto", "overflow: auto"},
		{"hidden on the main axis only", "overflow-x: hidden"},
	} {
		got, ok := gridRect(t, doc, base+`#a { `+tc.overflow+` }`, "a")
		if !ok {
			t.Errorf("%s: the item generated no fragment", tc.name)
			continue
		}
		if got.W.Px() != 250 {
			t.Errorf("%s: the item is %.0f wide, want 250 — it clips its overflow, so "+
				"it may shrink to the room left over", tc.name, got.W.Px())
		}
	}
}

// TestAnItemThatDoesNotClipStillRefusesToShrink is the rule the clause is an
// exception to, kept: a change that zeroed every automatic minimum would let a
// row of words collapse into each other.
func TestAnItemThatDoesNotClipStillRefusesToShrink(t *testing.T) {
	word := strings.Repeat("W", 60)
	doc := `<div id="f"><div id="a">` + word + `</div><div id="b">x</div></div>`
	css := noDefaults + `#f { display: flex; width: 300px; font-family: Courier;
		font-size: 10px }
		#a { flex: 1 1 0; overflow-y: hidden }
		#b { flex: 0 0 50px }`
	got, ok := gridRect(t, doc, css, "a")
	if !ok {
		t.Fatal("the item generated no fragment")
	}
	if got.W.Px() <= 250 {
		t.Errorf("the item is %.0f wide; it clips only the cross axis, so its automatic "+
			"minimum is still the width of the word", got.W.Px())
	}
}

// TestAFlexContainerDoesNotSitUnderAFloat is Flexbox §3, which the comment
// above establishesBFC already named and the list below it left out.
//
// A flex container establishes a formatting context of its own and floats do
// not intrude into it. It was absent from the list, so an in-flow flex
// container beside a left float sat underneath it at x=0 and full width, while
// a grid or a flow-root in the same place narrowed correctly.
func TestAFlexContainerDoesNotSitUnderAFloat(t *testing.T) {
	const doc = `<div id="fl"></div><div id="c">x</div>`
	const base = noDefaults + `#fl { float: left; width: 100px; height: 50px }
		#c { background: red; height: 20px }`

	want, ok := gridRect(t, doc, base+`#c { display: flow-root }`, "c")
	if !ok {
		t.Fatal("the flow-root generated no fragment")
	}
	if want.X.Px() != 100 {
		t.Fatalf("a flow-root beside a 100px float is at x=%.0f; the fixture is wrong",
			want.X.Px())
	}
	for _, display := range []string{"flex", "grid", "inline-flex"} {
		got, ok := gridRect(t, doc, base+`#c { display: `+display+` }`, "c")
		if !ok {
			t.Errorf("%s: no fragment", display)
			continue
		}
		if display == "inline-flex" {
			// An atomic inline sits on a line, which the float already shortens.
			if got.X.Px() < 100 {
				t.Errorf("%s is at x=%.0f, under the float", display, got.X.Px())
			}
			continue
		}
		if got.X != want.X || got.W != want.W {
			t.Errorf("a %s container beside a float is at x=%.0f and %.0f wide; a "+
				"flow-root in the same place is at x=%.0f and %.0f wide",
				display, got.X.Px(), got.W.Px(), want.X.Px(), want.W.Px())
		}
	}
}

// TestASpanHoldingABlockIsOneFlexItem is §9.2.1.1 applied where it does not
// belong.
//
// The block-in-inline split exists because a block inside an inline formatting
// context has nowhere to be placed. A flex or grid container has no inline
// formatting context: §4 blockifies every in-flow child into an item. Splitting
// there turned one item into three — a row of three cells where the author
// wrote one.
func TestASpanHoldingABlockIsOneFlexItem(t *testing.T) {
	for _, display := range []string{"flex", "grid"} {
		css := noDefaults + `#c { display: ` + display + `; width: 300px }`
		built := Build(Input{
			HTML: `<div id="c"><span id="s">a<div>b</div>c</span></div>`,
			CSS:  []Stylesheet{{Source: css}},
		})
		container := findBox(t, built.Root, "c")
		if n := len(container.Children); n != 1 {
			t.Errorf("a %s container holding one span has %d children, want one item",
				display, n)
		}
	}
}
