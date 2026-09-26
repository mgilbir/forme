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
//
// The clause is about a *scroll container* (§4.5), so what keeps the minimum is
// an item that is not one: overflow visible, or "clip" beside "visible" — CSS
// Overflow 3 §3.1 leaves that pair as it is, because neither value scrolls, so
// the item clips its content and does not scroll it. Either way it is as wide
// as its word.
//
// This test used to write "overflow-y: hidden" here, on the reading that an
// item clipping only its cross axis keeps its main-axis minimum. It does not:
// §3.1 computes the "visible" beside "hidden" to "auto", so the item scrolls on
// both axes and may shrink — which the last case below now holds.
func TestAnItemThatDoesNotClipStillRefusesToShrink(t *testing.T) {
	word := strings.Repeat("W", 60)
	doc := `<div id="f"><div id="a">` + word + `</div><div id="b">x</div></div>`
	const base = noDefaults + `#f { display: flex; width: 300px; font-family: Courier;
		font-size: 10px }
		#a { flex: 1 1 0 }
		#b { flex: 0 0 50px }`
	for _, overflow := range []string{
		"overflow: visible", "overflow-x: clip", "overflow-y: clip",
		"overflow-x: clip; overflow-y: visible",
	} {
		got, ok := gridRect(t, doc, base+`#a { `+overflow+` }`, "a")
		if !ok {
			t.Fatalf("%s: the item generated no fragment", overflow)
		}
		if got.W.Px() <= 250 {
			t.Errorf("%s: the item is %.0f wide; it is not a scroll container, so "+
				"its automatic minimum is still the width of the word", overflow, got.W.Px())
		}
	}
	// And a scrolling value on the cross axis alone makes the item a scroll
	// container: "overflow-y: hidden" computes "overflow-x" to auto, and so
	// does "overflow-x: clip" beside it, which §3.1 turns into "hidden".
	for _, overflow := range []string{"overflow-y: hidden", "overflow-x: clip; overflow-y: hidden"} {
		got, ok := gridRect(t, doc, base+`#a { `+overflow+` }`, "a")
		if !ok {
			t.Fatalf("%s: the item generated no fragment", overflow)
		}
		if got.W.Px() != 250 {
			t.Errorf("%s: the item is %.0f wide, want 250 — it is a scroll container, "+
				"so it may shrink to the room left over", overflow, got.W.Px())
		}
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

// TestAScrollContainerThatStatesASizeCanShrinkToNothing is §4.5's clause read
// whole: "for scroll containers the automatic minimum size is zero, as usual".
// It does not say "unless the item states a size". The minimum returned the
// stated size here, so "width: 100px; overflow: hidden" could not shrink below
// 100px in a row with 50px to spare. The size is the flex base size, where the
// item starts; the shrink factor is what lets it leave it.
//
// The other half is the rule that stays: an item that is not a scroll
// container has a content-based minimum, and §4.5's specified size suggestion
// caps it — min(specified, content) — both ways round.
func TestAScrollContainerThatStatesASizeCanShrinkToNothing(t *testing.T) {
	word := strings.Repeat("W", 60) // 360px of Courier at 10px
	row := func(content, item string) string {
		return `<div id="f" style="display: flex; width: 300px; font-family: Courier; ` +
			`font-size: 10px"><div id="a" style="flex: 0 1 auto; ` + item + `">` + content +
			`</div><div style="flex: 0 0 250px"></div></div>`
	}
	for _, tc := range []struct {
		name, content, item string
		want                float64
	}{
		// Scroll containers, which may shrink to the 50px left over.
		{"a stated width", word, "width: 100px; overflow: hidden", 50},
		{"a stated width on a short word", "WW", "width: 100px; overflow: auto", 50},
		{"a keyword width", word, "width: max-content; overflow: scroll", 50},
		{"a stated width with only overflow-y hidden", word, "width: 100px; overflow-y: hidden", 50},
		// Not scroll containers: the specified size suggestion is the minimum
		// where it is the smaller, and the content where that is.
		{"a stated width below the content", word, "width: 100px", 100},
		{"a stated width that clips", word, "width: 100px; overflow: clip", 100},
		{"a stated width above the content", "WWWWWWWWWWWWWWW", "width: 200px", 90},
		{"a keyword width", word, "width: min-content", 360},
	} {
		got, ok := gridRect(t, row(tc.content, tc.item), noDefaults, "a")
		if !ok {
			t.Errorf("%s (%s): the item generated no fragment", tc.name, tc.item)
			continue
		}
		if got.W.Px() != tc.want {
			t.Errorf("%s (%s): the item is %.0f wide, want %.0f", tc.name, tc.item,
				got.W.Px(), tc.want)
		}
	}

	// Down a column the main size is a height, and the same holds: a 400px
	// scroll container in a 300px column beside a 250px item is 50px tall; the
	// same item without overflow keeps its content, the 200px its child needs.
	column := func(item string) string {
		return `<div style="display: flex; flex-direction: column; height: 300px">` +
			`<div id="a" style="flex: 0 1 auto; height: 400px; ` + item + `">` +
			`<div style="height: 200px"></div></div><div style="flex: 0 0 250px"></div></div>`
	}
	for _, tc := range []struct {
		item string
		want float64
	}{{"overflow: hidden", 50}, {"overflow: visible", 200}} {
		got, ok := gridRect(t, column(tc.item), noDefaults, "a")
		if !ok {
			t.Errorf("column, %s: the item generated no fragment", tc.item)
			continue
		}
		if got.H.Px() != tc.want {
			t.Errorf("column, %s: the item is %.0f tall, want %.0f", tc.item, got.H.Px(), tc.want)
		}
	}
}
