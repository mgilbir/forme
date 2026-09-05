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
		{"a right-to-left row", `#f { direction: rtl }`, "runs from the right"},
		{"wrapping", `#f { flex-wrap: wrap }`, "more than one line"},
		{"a safe alignment", `#f { justify-content: safe center }`, "packed by a rule"},
		{"items on a baseline", `#f { align-items: baseline }`, "shared baseline"},
		{"an item on the baseline", `#f > div:first-child { align-self: baseline }`, "shared baseline"},
		{"a reordered item", `#f > div:first-child { order: 2 }`, "moved in the order"},
		{"an automatic margin", `#f > div:first-child { margin-left: auto }`, "automatic margin"},
		{"an automatic cross margin", `#f > div:first-child { margin-top: auto }`, "automatic margin"},
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
		`#f { width: 300px; justify-content: space-between; align-items: flex-end }`,
		`#f { width: 300px; justify-content: right; align-items: center }`,
		`#f { width: 300px } #f > div { align-self: auto; order: 0 }`,
		`#f { width: 300px } #f > div:first-child { align-self: flex-start }`,
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

// TestJustifyContentPacksTheItemsAlongTheLine is §9.5. Three 12px items in
// 300px leave 264px over, and each keyword is a different answer to where that
// 264 goes — which is the whole of the property.
//
// The numbers are written out rather than derived so that a wrong one is a
// wrong number: 264 into two gaps is 132, into three shares is 88, and into
// four is 66.
func TestJustifyContentPacksTheItemsAlongTheLine(t *testing.T) {
	for _, c := range []struct {
		value string
		want  [][2]float64
	}{
		{"normal", [][2]float64{{0, 12}, {12, 12}, {24, 12}}},
		{"flex-start", [][2]float64{{0, 12}, {12, 12}, {24, 12}}},
		{"flex-end", [][2]float64{{264, 12}, {276, 12}, {288, 12}}},
		{"right", [][2]float64{{264, 12}, {276, 12}, {288, 12}}},
		{"center", [][2]float64{{132, 12}, {144, 12}, {156, 12}}},
		// A gap of 132 between each pair and none at the ends.
		{"space-between", [][2]float64{{0, 12}, {144, 12}, {288, 12}}},
		// Three shares of 88, half of one at each end and a whole one between.
		{"space-around", [][2]float64{{44, 12}, {144, 12}, {244, 12}}},
		// Four gaps of 66, all of them equal, which is what "evenly" means and
		// is where it differs from "around".
		{"space-evenly", [][2]float64{{66, 12}, {144, 12}, {222, 12}}},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := flexRow(t, threeItems, `#f { width: 300px; justify-content: `+c.value+` }`)
			wantRow(t, got, c.want, c.value)
		})
	}
}

// TestAnOverfullLineFallsBackToPackingIt is Box Alignment §4.4. The items cannot
// shrink below the width of their own text, so 300px of content in a 240px row
// has 60px too much rather than none — and a distribution with nothing to
// distribute has to become a packing or it would pull the items back over each
// other.
//
// Five "abcde"s of Courier at 20px are 60px each. The three distribution values
// are the ones with a fallback; flex-end and center keep their arithmetic and
// overflow the near edge, which is what an alignment that is not "safe" does.
func TestAnOverfullLineFallsBackToPackingIt(t *testing.T) {
	const five = `<div id="f"><div>abcde</div><div>abcde</div><div>abcde</div>` +
		`<div>abcde</div><div>abcde</div></div>`
	packed := [][2]float64{{0, 60}, {60, 60}, {120, 60}, {180, 60}, {240, 60}}
	centred := [][2]float64{{-30, 60}, {30, 60}, {90, 60}, {150, 60}, {210, 60}}
	for _, c := range []struct {
		value string
		want  [][2]float64
	}{
		{"space-between", packed},
		{"space-around", centred},
		{"space-evenly", centred},
		{"center", centred},
		{"flex-end", [][2]float64{{-60, 60}, {0, 60}, {60, 60}, {120, 60}, {180, 60}}},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := flexRow(t, five, `#f { width: 240px; justify-content: `+c.value+` }`)
			wantRow(t, got, c.want, "an overfull row with "+c.value)
		})
	}
}

// TestOneItemHasNoGapToPutSpaceIn. §9.5's other fallback, and the reason
// space-between is not written as free/(n-1): with one item there is no pair to
// go between, so it packs at the start, while the two that fill the ends still
// have ends and centre it.
func TestOneItemHasNoGapToPutSpaceIn(t *testing.T) {
	const one = `<div id="f"><div>a</div></div>`
	for _, c := range []struct {
		value string
		x     float64
	}{
		{"space-between", 0},
		{"space-around", 144},
		{"space-evenly", 144},
	} {
		got := flexRow(t, one, `#f { width: 300px; justify-content: `+c.value+` }`)
		wantRow(t, got, [][2]float64{{c.x, 12}}, "one item with "+c.value)
	}
}

// flexCross is where each item of #f sits across the line and how tall it is,
// in pixels.
func flexCross(t *testing.T, htmlSrc, extra string) []struct{ y, h float64 } {
	t.Helper()
	root := layoutOf(t, 1000, htmlSrc, flexCSS+extra)
	var out []struct{ y, h float64 }
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				for _, c := range f.Children {
					out = append(out, struct{ y, h float64 }{
						c.BorderRect.Y.Px(), c.BorderRect.H.Px()})
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

func wantCross(t *testing.T, got []struct{ y, h float64 }, want [][2]float64, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d items, want %d: %v", what, len(got), len(want), got)
	}
	for i := range want {
		if got[i].y != want[i][0] || got[i].h != want[i][1] {
			t.Errorf("%s: item %d is at y=%g height=%g, want y=%g height=%g\n  whole row: %v",
				what, i, got[i].y, got[i].h, want[i][0], want[i][1], got)
		}
	}
}

// TestAlignItemsPutsTheItemsAcrossTheLine is §9.6. The second item is two 20px
// lines, so the line is 40px and the first item — one line — has 20px of room
// to sit in.
//
// The height is asserted with the position because the two are the same
// decision: only a stretched item is the line's height, and an item that is 20px
// tall under "center" and 40px tall under "stretch" is the difference between
// aligning it and resizing it.
func TestAlignItemsPutsTheItemsAcrossTheLine(t *testing.T) {
	const twoLines = `<div id="f"><div id="a">a</div><div id="b">b<br>c</div></div>`
	for _, c := range []struct {
		value string
		want  [][2]float64
	}{
		{"normal", [][2]float64{{0, 40}, {0, 40}}},
		{"stretch", [][2]float64{{0, 40}, {0, 40}}},
		{"flex-start", [][2]float64{{0, 20}, {0, 40}}},
		{"flex-end", [][2]float64{{20, 20}, {0, 40}}},
		{"center", [][2]float64{{10, 20}, {0, 40}}},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := flexCross(t, twoLines, `#f { width: 300px; align-items: `+c.value+` }`)
			wantCross(t, got, c.want, c.value)
		})
	}
}

// TestAlignSelfOverridesTheContainer is §6.2: an item states its own alignment
// and "auto" — the initial value — is what defers to the container's.
func TestAlignSelfOverridesTheContainer(t *testing.T) {
	const twoLines = `<div id="f"><div id="a">a</div><div id="b">b<br>c</div></div>`

	// The container stretches and the item refuses to be stretched.
	got := flexCross(t, twoLines, `#f { width: 300px } #a { align-self: flex-end }`)
	wantCross(t, got, [][2]float64{{20, 20}, {0, 40}}, "an item at the end of a stretching row")

	// And the other way: the container aligns and the item asks to be stretched
	// after all.
	got = flexCross(t, twoLines,
		`#f { width: 300px; align-items: center } #a { align-self: stretch }`)
	wantCross(t, got, [][2]float64{{0, 40}, {0, 40}}, "an item stretched in a centring row")

	// "auto" is not a fourth alignment: it is the container's.
	got = flexCross(t, twoLines,
		`#f { width: 300px; align-items: flex-end } #a { align-self: auto }`)
	wantCross(t, got, [][2]float64{{20, 20}, {0, 40}}, "an item deferring to the container")
}

// TestAnItemTallerThanItsLineHangsOffIt. The container states a height, so the
// line is that height whatever the items came to, and an item taller than it
// overflows. Where it overflows is the property's answer, not zero: flex-end
// keeps the item's bottom on the line's bottom and lets the top hang off.
//
// Flooring the room left over at zero would put every one of these at y=0,
// which is a page where align-items silently stops working the moment an item
// overruns — the failure that is plausible enough to ship.
func TestAnItemTallerThanItsLineHangsOffIt(t *testing.T) {
	const tall = `<div id="f"><div id="a">a<br>b</div></div>`
	for _, c := range []struct {
		value string
		y     float64
	}{
		{"flex-start", 0},
		{"flex-end", -20},
		{"center", -10},
	} {
		got := flexCross(t, tall, `#f { width: 300px; height: 20px; align-items: `+c.value+` }`)
		wantCross(t, got, [][2]float64{{c.y, 40}}, "a 40px item on a 20px line with "+c.value)
	}
}

// TestTheLastItemLandsOnTheFarEdge is why each offset is a fraction of the whole
// free space rather than a gap added once per item.
//
// 265px over three gaps is 88 and a third, which is not a whole number of layout
// units. Rounding it once and adding it three times loses a unit off the end of
// the row; taking the fraction each time does not, and the last item's right
// edge is on the container's — where "space-between" says it is.
func TestTheLastItemLandsOnTheFarEdge(t *testing.T) {
	got := flexRow(t, `<div id="f"><div>a</div><div>b</div><div>c</div><div>d</div></div>`,
		`#f { width: 313px; justify-content: space-between }`)
	wantRow(t, got, [][2]float64{
		{0, 12}, {100.328125, 12}, {200.671875, 12}, {301, 12},
	}, "four items over a free space that does not divide")

	if len(got) == 4 && got[3].x+got[3].w != 313 {
		t.Errorf("the row ends at %gpx and the container is 313px wide",
			got[3].x+got[3].w)
	}
}

// TestAGapComesOutOfTheLineBeforeTheItemsDo is §8 of Box Alignment. The gaps are
// not free space: they are taken off the main axis first, and what the items
// then divide is what is left.
//
// Three 12px items in 300px with a 30px gap: 60px of gap, 240px of line, and the
// items sit 12 and 30 apart instead of 12.
func TestAGapComesOutOfTheLineBeforeTheItemsDo(t *testing.T) {
	got := flexRow(t, threeItems, `#f { width: 300px; column-gap: 30px }`)
	wantRow(t, got, [][2]float64{{0, 12}, {42, 12}, {84, 12}}, "three items 30px apart")

	// The clause that makes this more than an offset: an item that grows
	// divides the line and not the container, so the row still ends on the
	// container's edge. 240px over three equal factors is 80 each.
	got = flexRow(t, threeItems, `#f { width: 300px; column-gap: 30px } #f > div { flex: 1 }`)
	wantRow(t, got, [][2]float64{{0, 80}, {110, 80}, {220, 80}}, "three growing items with a gap")
	if len(got) == 3 && got[2].x+got[2].w != 300 {
		t.Errorf("the row ends at %gpx and the container is 300px wide",
			got[2].x+got[2].w)
	}

	// And shrinking gives back to the line, not to the gap. Two items asking
	// for 200px each in 300px with a 20px gap have 280px to divide, so each
	// gives up 60.
	got = flexRow(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 300px; column-gap: 20px } #f > div { flex-basis: 200px }`)
	wantRow(t, got, [][2]float64{{0, 140}, {160, 140}}, "two items shrinking around a gap")
}

// TestTheFreeSpaceThatIsPackedIsWhatTheGapsLeft. justify-content distributes
// what is over after the gaps, and it adds its share on top of each gap rather
// than instead of it.
func TestTheFreeSpaceThatIsPackedIsWhatTheGapsLeft(t *testing.T) {
	// 240px of line less 36px of items is 204px over, all of it before the
	// first item — which is 60px nearer the start than it would be with no gap.
	got := flexRow(t, threeItems,
		`#f { width: 300px; column-gap: 30px; justify-content: flex-end }`)
	wantRow(t, got, [][2]float64{{204, 12}, {246, 12}, {288, 12}}, "a gapped row packed at the end")

	got = flexRow(t, threeItems,
		`#f { width: 300px; column-gap: 30px; justify-content: center }`)
	wantRow(t, got, [][2]float64{{102, 12}, {144, 12}, {186, 12}}, "a gapped row centred")

	// space-between adds 102 to each 30px gap, and the ends stay put.
	got = flexRow(t, threeItems,
		`#f { width: 300px; column-gap: 30px; justify-content: space-between }`)
	wantRow(t, got, [][2]float64{{0, 12}, {144, 12}, {288, 12}}, "a gapped row spread out")
}

// TestAGapWiderThanTheRowOverflowsIt. The gaps are taken first even when there
// is not enough room for them, because they are not what gives way — the items
// are, and these cannot shrink below their own text.
func TestAGapWiderThanTheRowOverflowsIt(t *testing.T) {
	got := flexRow(t, threeItems, `#f { width: 300px; column-gap: 200px }`)
	wantRow(t, got, [][2]float64{{0, 12}, {212, 12}, {424, 12}}, "gaps wider than the row")
}

// TestWhatAGapIsWrittenAs. "normal" is the initial value and in a flex container
// it is zero — the same keyword is one em in a multi-column container, which is
// why the cascade keeps it as a keyword. A percentage is of the container, and a
// negative length is not a gap at all.
func TestWhatAGapIsWrittenAs(t *testing.T) {
	none := [][2]float64{{0, 12}, {12, 12}, {24, 12}}
	apart := [][2]float64{{0, 12}, {42, 12}, {84, 12}}
	for _, c := range []struct {
		value string
		want  [][2]float64
	}{
		{"normal", none},
		{"0", none},
		{"-30px", none},
		{"30px", apart},
		{"10%", apart},
		{"1.5em", apart},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := flexRow(t, threeItems, `#f { width: 300px; column-gap: `+c.value+` }`)
			wantRow(t, got, c.want, "column-gap: "+c.value)
		})
	}
}

// TestTheGapShorthandIsNotApplied records the edge of what this does, because an
// edge that is not written down is one that moves without anyone deciding to
// move it.
//
// "gap" sets row-gap as well, and a row gap needs rows to separate: this engine
// arranges one line of one row, and neither has a second row in it. So the
// shorthand is not registered, an author who writes it is told the declaration
// was not applied, and the day something here has rows — a column of items, or
// a container whose lines wrap — the two arrive together.
func TestTheGapShorthandIsNotApplied(t *testing.T) {
	got := Compose(Input{HTML: threeItems, CSS: []Stylesheet{{
		Source: flexCSS + `#f { width: 300px; gap: 30px }`}}}, Options{})
	said := false
	for _, f := range got.Findings {
		if f.Property == "gap" || f.Property == "row-gap" {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing was reported about \"gap: 30px\", which is not applied: %v",
			got.Findings)
	}
	wantRow(t, flexRow(t, threeItems, `#f { width: 300px; gap: 30px }`),
		[][2]float64{{0, 12}, {12, 12}, {24, 12}}, "a row with an unapplied gap")
}
