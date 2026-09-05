package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Flexible Box Layout 1 §9, laid out.
//
// The suite has no document that arranges a flex container — the four that name
// a flex property all have a script in them — so the reftest count is a
// regression check here and nothing else. What says this is right is the
// arithmetic below, and the fixture is chosen so that every number in it is a
// whole one: Courier at 20px puts one character at exactly 12px, and every
// container is a width the free space divides into exactly. A share that came
// out a unit wrong would show as a wrong number rather than as a rounding.

const flexCSS = `body { margin: 0 }
	#f { display: flex; font-family: Courier; font-size: 20px; line-height: 20px }
	#f > div { font-family: Courier; font-size: 20px; line-height: 20px }`

// flexRow is where each item of #f was placed, in pixels: the left edge of its
// border box and its width.
func flexRow(t *testing.T, htmlSrc, extra string) []struct{ x, w float64 } {
	t.Helper()
	root := layoutOf(t, 1000, htmlSrc, flexCSS+extra)
	var out []struct{ x, w float64 }
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				for _, c := range f.Children {
					out = append(out, struct{ x, w float64 }{
						c.BorderRect.X.Px(), c.BorderRect.W.Px()})
				}
				return
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

func wantRow(t *testing.T, got []struct{ x, w float64 }, want [][2]float64, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d items, want %d: %v", what, len(got), len(want), got)
	}
	for i := range want {
		if got[i].x != want[i][0] || got[i].w != want[i][1] {
			t.Errorf("%s: item %d is at x=%g width=%g, want x=%g width=%g\n  whole row: %v",
				what, i, got[i].x, got[i].w, want[i][0], want[i][1], got)
		}
	}
}

const threeItems = `<div id="f"><div>a</div><div>b</div><div>c</div></div>`

// TestItemsSitInARowAtTheirOwnSize. The initial value of flex is "0 1 auto": an
// item takes the size its content asks for and neither grows nor is asked to
// give way while the line has room. Three single characters of Courier at 20px
// are three 12px items packed at the start.
func TestItemsSitInARowAtTheirOwnSize(t *testing.T) {
	got := flexRow(t, threeItems, `#f { width: 300px }`)
	wantRow(t, got, [][2]float64{{0, 12}, {12, 12}, {24, 12}}, "three characters in a wide row")
}

// TestFlexGrowSharesTheFreeSpaceInProportion. §9.7.4c for growth: the space left
// over is shared in the ratio of the flex-grow factors, and it is *added to*
// each item's base size rather than replacing it.
//
// 300px of room and 36px of content leaves 264. With equal factors that is 88
// each, on top of the 12 each item started at.
func TestFlexGrowSharesTheFreeSpaceInProportion(t *testing.T) {
	got := flexRow(t, threeItems, `#f { width: 300px } #f > div { flex-grow: 1 }`)
	wantRow(t, got, [][2]float64{{0, 100}, {100, 100}, {200, 100}}, "three equal factors")

	// 1:2:3 of the same 264 is 44, 88 and 132, and the bases are still there.
	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div><div id="c">c</div></div>`,
		`#f { width: 300px } #a { flex-grow: 1 } #b { flex-grow: 2 } #c { flex-grow: 3 }`)
	wantRow(t, got, [][2]float64{{0, 56}, {56, 100}, {156, 144}}, "factors of one, two and three")
}

// TestTheShorthandStartsFromZero. §7.1: "flex: 1" is "1 1 0", not "1 1 auto",
// because the shorthand resets an omitted basis to zero rather than to its
// initial value. It is the difference between a row of equal parts and a row of
// items sized from their text with the leftovers shared on top, and it is the
// whole reason authors write the shorthand.
func TestTheShorthandStartsFromZero(t *testing.T) {
	got := flexRow(t, threeItems, `#f { width: 300px } #f > div { flex: 1 }`)
	wantRow(t, got, [][2]float64{{0, 100}, {100, 100}, {200, 100}}, "flex: 1 on three items")

	// The same factors written as longhands keep the auto basis, so each item's
	// own 12px is *inside* its share and the row comes out the same only
	// because the factors are equal. With unequal ones the two disagree.
	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 300px } #a { flex: 1 } #b { flex: 2 }`)
	wantRow(t, got, [][2]float64{{0, 100}, {100, 200}}, "flex: 1 against flex: 2")

	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 300px } #a { flex-grow: 1 } #b { flex-grow: 2 }`)
	wantRow(t, got, [][2]float64{{0, 104}, {104, 196}}, "the same factors as longhands")
}

// TestFlexShrinkIsScaledByTheBaseSize. §9.7.4c for shrinkage, and the asymmetry
// that matters: what is shared is not the factor but the factor times the item's
// base size, so a long item gives up more than a short one that declared the
// same willingness.
//
// 100px and 300px of content in 200px of room is 200px too much. The scaled
// factors are 100 and 300, so the overflow is taken away in the ratio 1:3 —
// 50 from the first and 150 from the second.
func TestFlexShrinkIsScaledByTheBaseSize(t *testing.T) {
	got := flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 200px } #a { flex-basis: 100px } #b { flex-basis: 300px }`)
	wantRow(t, got, [][2]float64{{0, 50}, {50, 150}}, "two items shrinking")

	// With the shrink factors reversed the shares reverse with them: the scaled
	// factors become 200 and 300, and 200px is taken away in the ratio 2:3.
	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 200px } #a { flex: 0 2 100px } #b { flex: 0 1 300px }`)
	wantRow(t, got, [][2]float64{{0, 20}, {20, 180}}, "a doubled shrink factor")
}

// TestAnItemFrozenAtItsLimitLeavesItsShareToTheRest.
//
// §9.7.4d and e are why the resolution is a loop. An item that hits its own
// maximum stops moving, and the space it did not take goes back into the pot for
// the items that can still use it — so the row still adds up to the container.
// One pass would leave 100px of the container empty.
func TestAnItemFrozenAtItsLimitLeavesItsShareToTheRest(t *testing.T) {
	got := flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 300px } #f > div { flex: 1 } #a { max-width: 50px }`)
	wantRow(t, got, [][2]float64{{0, 50}, {50, 250}}, "one item capped")

	// And the other way: an item held up by a minimum takes more than its share
	// and the rest give way for it.
	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 200px } #f > div { flex-basis: 200px } #a { min-width: 150px }`)
	wantRow(t, got, [][2]float64{{0, 150}, {150, 50}}, "one item floored")
}

// TestAnItemIsNotShrunkNarrowerThanItsContent is §4.5's automatic minimum size,
// which is the clause that stops a row of words collapsing into a column of
// single letters. The items ask for 300px each in 100px of room; without the
// automatic minimum they would come out 50px each, and a Courier "abcde" is 60.
func TestAnItemIsNotShrunkNarrowerThanItsContent(t *testing.T) {
	got := flexRow(t, `<div id="f"><div>abcde</div><div>abcde</div></div>`,
		`#f { width: 100px } #f > div { flex-basis: 300px }`)
	wantRow(t, got, [][2]float64{{0, 60}, {60, 60}}, "two words too wide for the row")

	// A declared width below the content is the author saying so, and §4.5 caps
	// the automatic minimum by it: an item that asked to be 24px wide is 24px.
	got = flexRow(t, `<div id="f"><div>abcde</div><div>abcde</div></div>`,
		`#f { width: 100px } #f > div { width: 24px; flex-shrink: 0 }`)
	wantRow(t, got, [][2]float64{{0, 24}, {24, 24}}, "a declared width below the content")
}

// TestItemsAreStretchedAcrossTheLine is §9.6 with align-items at its initial
// value: an item that states no height is as tall as the line, and the line is
// as tall as the tallest item. Two lines of 20px against one make a 40px line
// and a 40px item beside it.
func TestItemsAreStretchedAcrossTheLine(t *testing.T) {
	root := layoutOf(t, 1000, `<div id="f"><div id="a">a</div><div id="b">b<br>c</div></div>`,
		flexCSS+`#f { width: 300px }`)
	var heights []float64
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				for _, c := range f.Children {
					heights = append(heights, c.BorderRect.H.Px())
				}
				return
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if len(heights) != 2 {
		t.Fatalf("%d items, want 2", len(heights))
	}
	if heights[0] != 40 || heights[1] != 40 {
		t.Errorf("the items are %gpx and %gpx tall; the taller one has two 20px "+
			"lines and the other is stretched to match it", heights[0], heights[1])
	}
}

// TestAContainerThisEngineCannotArrangeIsLaidOutAsABlockAndSaysSo.
//
// The gate. Each of these is a flex container this file does not arrange, and
// each must come out as the column of full-width blocks it was before — and be
// reported, because a row silently laid out as a column is exactly the
// plausible wrongness the finding exists for.
func TestAContainerThisEngineCannotArrangeIsLaidOutAsABlockAndSaysSo(t *testing.T) {
	for _, c := range []struct{ what, css, names string }{
		{"a column", `#f { flex-direction: column }`, "left-to-right row"},
		{"wrapping", `#f { flex-wrap: wrap }`, "more than one line"},
		{"centred content", `#f { justify-content: center }`, "packed at the start"},
		{"aligned items", `#f { align-items: center }`, "stretched across"},
		{"an item aligned by itself", `#f > div:first-child { align-self: center }`, "by itself"},
		{"a reordered item", `#f > div:first-child { order: 2 }`, "moved in the order"},
		{"an automatic margin", `#f > div:first-child { margin-left: auto }`, "automatic margin"},
		{"a percentage item", `#f > div:first-child { width: 50% }`, "percentage of the row"},
		{"a floated child", `#f > div:first-child { float: left }`, "floated or absolutely"},
	} {
		t.Run(c.what, func(t *testing.T) {
			got := Compose(Input{HTML: threeItems, CSS: []Stylesheet{{
				Source: flexCSS + `#f { width: 300px }` + c.css}}}, Options{})
			var said string
			for _, f := range got.Findings {
				if strings.Contains(f.Message, "flex container") {
					said = f.Message
				}
			}
			if said == "" {
				t.Fatalf("nothing was reported about a container with %s, so a row "+
					"laid out as a column says nothing about it: %v", c.what, got.Findings)
			}
			if !strings.Contains(said, c.names) {
				t.Errorf("the finding for %s is %q, which does not name %q",
					c.what, said, c.names)
			}
			// And it really was laid out as a block: three full-width children
			// stacked, which is the page this engine drew before flex existed.
			row := flexRow(t, threeItems, `#f { width: 300px }`+c.css)
			for i, it := range row {
				if it.x != 0 {
					t.Errorf("item %d of a refused container is at x=%g, want 0 — "+
						"a refused container is stacked", i, it.x)
				}
			}
		})
	}
}

// TestAnArrangedContainerSaysNothing is the containment argument. The finding
// must not fire on the containers this file *does* arrange, or every flex
// document in the world would carry a report of a page that is right.
func TestAnArrangedContainerSaysNothing(t *testing.T) {
	for _, css := range []string{
		`#f { width: 300px }`,
		`#f { width: 300px } #f > div { flex: 1 }`,
		`#f { width: 300px; flex-direction: row; flex-wrap: nowrap }`,
		`#f { width: 300px; justify-content: normal; align-items: stretch }`,
		`#f { width: 300px } #f > div { align-self: auto; order: 0 }`,
	} {
		got := Compose(Input{HTML: threeItems,
			CSS: []Stylesheet{{Source: flexCSS + css}}}, Options{})
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "flex") || f.Property == "display" {
				t.Errorf("%q reported %q, and the container was arranged", css, f.Message)
			}
		}
	}
}

// TestAnEmptyContainerIsNotReported. A container with no items has no
// arrangement to get wrong, and an out-of-flow child is not an item — §4.1
// places it against the container itself. Reporting either would be saying a
// page was wrong when it is the page the specification asks for.
func TestAnEmptyContainerIsNotReported(t *testing.T) {
	for _, doc := range []string{
		`<div id="f"></div>`,
		`<div id="f"><div style="position: absolute">a</div></div>`,
	} {
		got := Compose(Input{HTML: doc,
			CSS: []Stylesheet{{Source: flexCSS + `#f { width: 300px }`}}}, Options{})
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "flex") {
				t.Errorf("%s reported %q", doc, f.Message)
			}
		}
	}
	_ = style.Unit(0)
}

// TestFactorsBelowOneTakeOnlyThatFractionOfTheSpace is §9.7.4b, which is the
// clause that gives "flex-grow: 0.5" its meaning.
//
// Where the unfrozen factors add up to less than one, the items between them
// asked for only that fraction of the free space and the rest is left unused —
// so a row of two "flex: 0.25" items in 300px comes out 75px each with 150px of
// the container empty. Without the clause each item takes half of everything and
// the row fills, which is the same picture "flex: 0.5" gives and is why the two
// have to be told apart.
func TestFactorsBelowOneTakeOnlyThatFractionOfTheSpace(t *testing.T) {
	two := `<div id="f"><div>a</div><div>b</div></div>`

	got := flexRow(t, two, `#f { width: 300px } #f > div { flex: 0.25 }`)
	wantRow(t, got, [][2]float64{{0, 75}, {75, 75}}, "factors adding to a half")

	// Exactly one is the boundary, and the clause does not apply at it: the
	// items asked for the whole of the free space and get it.
	got = flexRow(t, two, `#f { width: 300px } #f > div { flex: 0.5 }`)
	wantRow(t, got, [][2]float64{{0, 150}, {150, 150}}, "factors adding to one")
}
