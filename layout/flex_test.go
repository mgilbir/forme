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
		{"a wrapping column", `#f { flex-wrap: wrap; flex-direction: column }`, "wrap into columns"},
		{"a column wrapping backwards", `#f { flex-wrap: wrap-reverse; flex-direction: column }`, "wrap into columns"},
		{"an axis with no name", `#f { flex-direction: sideways }`, "four flex-direction names"},
		{"lines that wrap some other way", `#f { flex-wrap: reverse }`, "wrap by a rule"},
		{"lines on a baseline", `#f { flex-wrap: wrap; align-content: baseline }`, "placed by a rule"},
		{"a safe alignment", `#f { justify-content: safe center }`, "packed by a rule"},
		{"items on the last baseline", `#f { align-items: last baseline }`, "last baseline of their text"},
		{"items on a baseline down a column",
			`#f { align-items: baseline; flex-direction: column }`,
			"across the axis they would be aligned on"},
		{"an item on a baseline in lines that stack backwards",
			`#f { flex-wrap: wrap-reverse } #f > div:first-child { align-self: baseline }`,
			"part the baselines again"},
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
		`#f { width: 300px } #f > div:first-child { margin-left: auto }`,
		`#f { width: 300px } #f > div:first-child { order: 2 }`,
		`#f { width: 300px; flex-wrap: wrap }`,
		`#f { width: 300px; height: 100px; flex-wrap: wrap; align-content: space-around }`,
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

// TestTheGapShorthandSetsTheGapOnBothAxes. "gap" is the two together, in the
// order row then column — block axis first, as "margin" is, and not the order
// the names suggest read left to right. A container reads the one that lies
// across its own axis and the other has nothing to say to it.
func TestTheGapShorthandSetsTheGapOnBothAxes(t *testing.T) {
	wantRow(t, flexRow(t, threeItems, `#f { width: 300px; gap: 30px }`),
		[][2]float64{{0, 12}, {42, 12}, {84, 12}}, "one value in a row")
	wantCross(t, flexCross(t, threeItems, `#f { width: 300px; flex-direction: column; gap: 30px }`),
		[][2]float64{{0, 20}, {50, 20}, {100, 20}}, "one value in a column")

	// Two values, and each container takes the half that is across its axis:
	// the row is 30 apart and the column 10.
	wantRow(t, flexRow(t, threeItems, `#f { width: 300px; gap: 10px 30px }`),
		[][2]float64{{0, 12}, {42, 12}, {84, 12}}, "two values in a row")
	wantCross(t, flexCross(t, threeItems,
		`#f { width: 300px; flex-direction: column; gap: 10px 30px }`),
		[][2]float64{{0, 20}, {30, 20}, {60, 20}}, "two values in a column")
}

// flexHeight is what the container itself came to.
func flexHeight(t *testing.T, htmlSrc, extra string) float64 {
	t.Helper()
	root := layoutOf(t, 1000, htmlSrc, flexCSS+extra)
	var out float64
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				out = f.BorderRect.H.Px()
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

const twoHeights = `<div id="f"><div id="a">a</div><div id="b">b<br>c</div></div>`

// TestAColumnStacksItsItemsDownTheContainer is flex-direction: column, which is
// the same algorithm with the axes exchanged.
//
// A 20px item and a 40px one stack to 60px, and each is as wide as the
// container because "stretch" across a column's axis is a width. That looks
// like the block stacking it replaces, and the two differ everywhere it
// matters: the items' margins do not collapse, the free space is shared along
// the height, and every alignment property changes which axis it acts on.
func TestAColumnStacksItsItemsDownTheContainer(t *testing.T) {
	const column = `#f { width: 300px; flex-direction: column }`
	wantCross(t, flexCross(t, twoHeights, column),
		[][2]float64{{0, 20}, {20, 40}}, "two items stacked")
	wantRow(t, flexRow(t, twoHeights, column),
		[][2]float64{{0, 300}, {0, 300}}, "two items stretched across the column")
	if h := flexHeight(t, twoHeights, column); h != 60 {
		t.Errorf("the container is %gpx tall and its items are 20 and 40", h)
	}
}

// TestAColumnWithNoHeightHasNothingToShareOut. §9.4: a container whose main size
// is indefinite takes the size its items asked for, so there is no free space —
// and no free space is not a small amount of it. "flex: 1" on every item is a
// declaration about a share of nothing, and the column comes out exactly as
// tall as it would have without it.
//
// This is the clause behind the thing every author has hit: a column of
// "flex: 1" children fills its parent only once the parent has a height.
func TestAColumnWithNoHeightHasNothingToShareOut(t *testing.T) {
	const grow = `#f { width: 300px; flex-direction: column } #f > div { flex: 1 }`
	wantCross(t, flexCross(t, twoHeights, grow),
		[][2]float64{{0, 20}, {20, 40}}, "growing items in a column of no stated height")
	if h := flexHeight(t, twoHeights, grow); h != 60 {
		t.Errorf("the container is %gpx tall, and with no height of its own it is "+
			"as tall as its items", h)
	}

	// State the height and the same declaration means something: 200px over two
	// equal factors from a zero basis is 100 each.
	wantCross(t, flexCross(t, twoHeights, grow+`#f { height: 200px }`),
		[][2]float64{{0, 100}, {100, 100}}, "growing items in a column that has a height")
}

// TestAColumnPacksAndAlignsAcrossTheOtherAxis. Every alignment property acts on
// the axis it names and not on a direction, so in a column justify-content moves
// the items down and align-items moves them across.
//
// The widths are the other half of it: an item that is not stretched is
// fit-content wide, which for one Courier character is 12px — so "align-items:
// center" is a 12px box in the middle of a 300px container and not a 300px box
// with its text centred, which is what the same page would look like if the
// property were being applied to the wrong thing.
func TestAColumnPacksAndAlignsAcrossTheOtherAxis(t *testing.T) {
	const tall = `#f { width: 300px; height: 200px; flex-direction: column }`

	wantCross(t, flexCross(t, twoHeights, tall+`#f { justify-content: space-between }`),
		[][2]float64{{0, 20}, {160, 40}}, "a column spread down the container")
	wantCross(t, flexCross(t, twoHeights, tall+`#f { justify-content: center }`),
		[][2]float64{{70, 20}, {90, 40}}, "a column centred down the container")

	wantRow(t, flexRow(t, twoHeights, tall+`#f { align-items: center }`),
		[][2]float64{{144, 12}, {144, 12}}, "a column of centred items")
	wantRow(t, flexRow(t, twoHeights, tall+`#f { align-items: flex-end }`),
		[][2]float64{{288, 12}, {288, 12}}, "a column of items at the far side")

	// "left" and "right" name a side of the inline axis, which is not a
	// column's main axis. §6.2 says they behave as "start" there, and a column
	// packed at the start is a column that has not moved.
	wantCross(t, flexCross(t, twoHeights, tall+`#f { justify-content: right }`),
		[][2]float64{{0, 20}, {20, 40}}, "a column packed to the right")
}

// TestAnItemInAColumnIsNotShrunkBelowItsContent is §4.5's automatic minimum on
// the block axis, and it is the one place the two axes are not mirror images.
//
// A row can shrink a word by breaking the line it is on; a block cannot be made
// shorter than the lines it holds. So a column item's content-based minimum is
// the height its content came to, which is why two 40px items in a 40px column
// overflow it instead of halving.
func TestAnItemInAColumnIsNotShrunkBelowItsContent(t *testing.T) {
	const two = `<div id="f"><div>a<br>b</div><div>c<br>d</div></div>`
	wantCross(t, flexCross(t, two, `#f { width: 300px; height: 40px; flex-direction: column }`),
		[][2]float64{{0, 40}, {40, 40}}, "two items too tall for the column")

	// A declared height below the content is the author saying so, and §4.5
	// caps the automatic minimum by it.
	wantCross(t, flexCross(t, two,
		`#f { width: 300px; height: 40px; flex-direction: column } #f > div { height: 10px }`),
		[][2]float64{{0, 10}, {10, 10}}, "two items that stated a height")
}

// TestAColumnIsSeparatedByARowGap. Which gap a container reads is a question
// about its axis: a column's items are stacked one above the next, and what
// lies between them is a row gap. The column gap has nothing to say about it,
// and saying so is the whole of the test — a container that read the wrong one
// would look right in every document that sets both.
func TestAColumnIsSeparatedByARowGap(t *testing.T) {
	const column = `#f { width: 300px; flex-direction: column }`

	wantCross(t, flexCross(t, twoHeights, column+`#f { row-gap: 30px }`),
		[][2]float64{{0, 20}, {50, 40}}, "a column with a row gap")
	if h := flexHeight(t, twoHeights, column+`#f { row-gap: 30px }`); h != 90 {
		t.Errorf("the container is %gpx tall: 20 and 40 of items with 30 between", h)
	}

	wantCross(t, flexCross(t, twoHeights, column+`#f { column-gap: 30px }`),
		[][2]float64{{0, 20}, {20, 40}}, "a column with a column gap")

	// And the same the other way round: a row reads the column gap and not the
	// row gap.
	wantRow(t, flexRow(t, threeItems, `#f { width: 300px; row-gap: 30px }`),
		[][2]float64{{0, 12}, {12, 12}, {24, 12}}, "a row with a row gap")
}

// TestAColumnSaysNothing. The containment argument again, for the direction
// this file has just started arranging.
func TestAColumnSaysNothing(t *testing.T) {
	for _, css := range []string{
		`#f { width: 300px; flex-direction: column }`,
		`#f { width: 300px; height: 200px; flex-direction: column; justify-content: center }`,
		`#f { width: 300px; flex-direction: column; align-items: flex-end; gap: 10px }`,
	} {
		got := Compose(Input{HTML: twoHeights,
			CSS: []Stylesheet{{Source: flexCSS + css}}}, Options{})
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "flex") || f.Property == "display" {
				t.Errorf("%q reported %q, and the container was arranged", css, f.Message)
			}
		}
	}
}

// TestWhatADiscardedLayoutTookOutOfTheFlowGoesWithIt.
//
// Both directions lay an item out and throw the answer away: a row that has to
// stretch an item lays it out again at the line's height, and a column lays
// every item out once just to see how tall it is. An absolutely positioned box
// found inside a discarded fragment hangs off a fragment nobody will paint —
// but its *record* is on the list placeAbsolutes works through, and that list
// has a budget. Spending it on boxes that are not on the page ends with a
// document being told it holds more out-of-flow boxes than this engine will
// place, which is a report of a broken page that is not broken.
//
// The cap is lowered to make the arithmetic small: four items, four absolutely
// positioned boxes, and a limit of five. Before this was handled the row
// recorded seven and the column eight.
func TestWhatADiscardedLayoutTookOutOfTheFlowGoesWithIt(t *testing.T) {
	held := maxAbsolutes
	defer func() { maxAbsolutes = held }()
	maxAbsolutes = 5

	const doc = `<div id="f">` +
		`<div>a<i id="p1" style="position: absolute">1</i></div>` +
		`<div>b<i id="p2" style="position: absolute">2</i></div>` +
		`<div>c<i id="p3" style="position: absolute">3</i></div>` +
		`<div>d<br>e<i id="p4" style="position: absolute">4</i></div></div>`

	for _, dir := range []string{"row", "column"} {
		t.Run(dir, func(t *testing.T) {
			css := flexCSS + `#f { width: 300px; flex-direction: ` + dir + ` }`
			got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}}, Options{})
			for _, f := range got.Findings {
				if f.Rule == RuleLimit {
					t.Errorf("four out-of-flow boxes with a limit of five reported "+
						"%q, so the discarded layouts are still on the list", f.Message)
				}
			}

			// And the four that are real are still placed: a rebuild that threw
			// away too much would leave the page short of them, which is the
			// failure in the other direction.
			root := layoutOf(t, 1000, doc, css)
			for _, id := range []string{"p1", "p2", "p3", "p4"} {
				if n := fragmentsWithID(root, id); n != 1 {
					t.Errorf("%s is on the page %d times, want once", id, n)
				}
			}
		})
	}
}

// fragmentsWithID counts the fragments generated by one element.
func fragmentsWithID(f *Fragment, id string) int {
	if f == nil {
		return 0
	}
	n := 0
	if f.Box != nil && f.Box.Element != nil {
		if got, _ := f.Box.Element.Attr("id"); got == id {
			n++
		}
	}
	for _, c := range f.Children {
		n += fragmentsWithID(c, id)
	}
	return n
}

// TestAColumnReadsTheSizesOnItsOwnAxis. Which property is a main size is the
// whole of what flex-direction changes, and getting it wrong is invisible in
// the ordinary case: an item's measured height already accounts for a height it
// declared, so a column that read "width" where it meant "height" lays most
// documents out correctly.
//
// These are the documents where it does not. An item that states a width in a
// column has stated a cross size, and its main size is still whatever its
// content came to; an item that states a maximum width has said nothing at all
// about how tall it may be.
func TestAColumnReadsTheSizesOnItsOwnAxis(t *testing.T) {
	const one = `<div id="f"><div id="a">a</div></div>`
	const column = `#f { width: 300px; flex-direction: column }`

	wantCross(t, flexCross(t, one, column+`#a { width: 50px }`),
		[][2]float64{{0, 20}}, "an item that stated a width")
	wantRow(t, flexRow(t, one, column+`#a { width: 50px }`),
		[][2]float64{{0, 50}}, "an item that stated a width")

	wantCross(t, flexCross(t, one, column+`#a { max-width: 10px }`),
		[][2]float64{{0, 20}}, "an item that stated a maximum width")
}

// TestAColumnShrinksInProportionToWhatItsItemsAskedFor is §9.7.4c on the block
// axis. An item's share of the shortfall is its flex-shrink scaled by its flex
// base size, and in a column the base size is what the item's content came to
// — so the two items below give up 20 and 40 of the 60px the container is short
// rather than 30 each.
//
// It takes "min-height: 0" to see at all. §4.5's automatic minimum would
// otherwise hold both items at their content height, which is the right default
// and the reason a column of text does not squash; the declaration is the
// documented way of asking for the other behaviour.
func TestAColumnShrinksInProportionToWhatItsItemsAskedFor(t *testing.T) {
	const two = `<div id="f"><div>a<br>b</div><div>c<br>d<br>e<br>f</div></div>`
	got := flexCross(t, two,
		`#f { width: 300px; height: 60px; flex-direction: column } #f > div { min-height: 0 }`)
	wantCross(t, got, [][2]float64{{0, 20}, {20, 40}}, "two items shrinking down a column")
}

// TestAColumnMeasuresAnItemFromTheEdgeItIsAlignedTo. The margins across a
// column's axis are the horizontal ones, and an item is placed from the one on
// the side it is aligned to.
func TestAColumnMeasuresAnItemFromTheEdgeItIsAlignedTo(t *testing.T) {
	const one = `<div id="f"><div id="a">a</div></div>`
	const column = `#f { width: 300px; flex-direction: column }`

	// Stretched, so the item is the container less its own margins and starts
	// at the left one.
	wantRow(t, flexRow(t, one, column+`#a { margin-left: 10px }`),
		[][2]float64{{10, 290}}, "a stretched item with a left margin")

	// Aligned to the far side, so the margin on that side is what holds it off
	// the edge: 300 less 12 of item and 10 of margin.
	wantRow(t, flexRow(t, one, column+`#f { align-items: flex-end } #a { margin-right: 10px }`),
		[][2]float64{{278, 12}}, "an item at the far side with a right margin")
}

// The wrapping fixture: four items that are each exactly a third of the
// container and cannot give way, so three fit on a line and the fourth starts
// another. Every number in the tests below comes off those two lines.
const fourWide = `<div id="f"><div>a</div><div>b</div><div>c</div><div>d</div></div>`

const wrapping = `#f { width: 300px; flex-wrap: wrap } #f > div { flex: 0 0 100px }`

// TestAWrappingRowBreaksWhenTheLineIsFull is §9.3. An item goes on the line in
// hand while it fits and starts a new one when it does not, and the container
// is as deep as the lines it ended up with.
func TestAWrappingRowBreaksWhenTheLineIsFull(t *testing.T) {
	wantRow(t, flexRow(t, fourWide, wrapping),
		[][2]float64{{0, 100}, {100, 100}, {200, 100}, {0, 100}}, "four items over two lines")
	wantCross(t, flexCross(t, fourWide, wrapping),
		[][2]float64{{0, 20}, {0, 20}, {0, 20}, {20, 20}}, "the second line below the first")
	if h := flexHeight(t, fourWide, wrapping); h != 40 {
		t.Errorf("the container is %gpx deep and holds two 20px lines", h)
	}

	// A container that does not wrap keeps all four on one line and overflows,
	// which is the same document and the property is the whole difference.
	wantRow(t, flexRow(t, fourWide, `#f { width: 300px } #f > div { flex: 0 0 100px }`),
		[][2]float64{{0, 100}, {100, 100}, {200, 100}, {300, 100}}, "four items that do not wrap")
}

// TestAnItemTooWideForItsLineGetsOneOfItsOwn. A line always holds at least one
// item: an item wider than the whole container has nowhere narrower to go, and
// putting it on a line by itself is what keeps the overflow to that one item
// instead of pushing the next one out with it.
func TestAnItemTooWideForItsLineGetsOneOfItsOwn(t *testing.T) {
	const two = `<div id="f"><div id="a">a</div><div id="b">b</div></div>`
	const wide = `#f { width: 300px; flex-wrap: wrap } #f > div { flex: 0 0 400px }`
	wantRow(t, flexRow(t, two, wide),
		[][2]float64{{0, 400}, {0, 400}}, "two items too wide for the container")

	// Two lines and not three. An empty line before the first item would sit
	// there costing nothing until the lines are spaced apart, and then it is a
	// gap above the first row that no declaration asked for.
	wantCross(t, flexCross(t, two, wide+`#f { row-gap: 10px }`),
		[][2]float64{{0, 20}, {30, 20}}, "two overwide items with a gap between the lines")
}

// TestTheGapCountsWhenTheLineIsMeasured. The room an item needs on a line is
// its own size and the gap that separates it from the one before, so the gap is
// part of what decides where the line breaks — three 95px items fit across
// 300px until 10px of gap is asked for between them.
func TestTheGapCountsWhenTheLineIsMeasured(t *testing.T) {
	const three = `<div id="f"><div>a</div><div>b</div><div>c</div></div>`
	const tight = `#f { width: 300px; flex-wrap: wrap } #f > div { flex: 0 0 95px }`

	wantRow(t, flexRow(t, three, tight),
		[][2]float64{{0, 95}, {95, 95}, {190, 95}}, "three items that fit together")
	wantRow(t, flexRow(t, three, tight+`#f { column-gap: 10px }`),
		[][2]float64{{0, 95}, {105, 95}, {0, 95}}, "three items that no longer fit")
}

// TestEachLineSharesOutItsOwnFreeSpace. §9.7 is resolved per line, not per
// container: the three items on the full line have nothing left over between
// them, and the one on the second line has the whole width to itself.
//
// This is the clause behind the row of cards that comes out with one enormous
// card at the bottom, and it is what tells the two readings apart — a container
// that resolved the sizes once for all four items would have given every one of
// them a quarter.
func TestEachLineSharesOutItsOwnFreeSpace(t *testing.T) {
	got := flexRow(t, fourWide,
		`#f { width: 300px; flex-wrap: wrap } #f > div { flex: 1 1 100px }`)
	wantRow(t, got, [][2]float64{{0, 100}, {100, 100}, {200, 100}, {0, 300}},
		"three items on a full line and one with the line to itself")
}

// TestTheLinesAreSeparatedByARowGap. The gap between the lines of a row is the
// row gap, which is the other half of the pair the items themselves are
// separated by.
func TestTheLinesAreSeparatedByARowGap(t *testing.T) {
	wantCross(t, flexCross(t, fourWide, wrapping+`#f { row-gap: 10px }`),
		[][2]float64{{0, 20}, {0, 20}, {0, 20}, {30, 20}}, "two lines 10px apart")
	if h := flexHeight(t, fourWide, wrapping+`#f { row-gap: 10px }`); h != 50 {
		t.Errorf("the container is %gpx deep: two 20px lines with 10 between", h)
	}
}

// TestAlignContentPlacesTheLines is §9.6's align-content, which moves the lines
// and not what is on them.
//
// Two 20px lines in a 100px container leave 60px over, and each keyword is a
// different answer to where it goes — the same six answers justify-content
// gives on the other axis, because in Box Alignment they are one property.
func TestAlignContentPlacesTheLines(t *testing.T) {
	for _, c := range []struct {
		value string
		want  [][2]float64
	}{
		// The initial value divides the leftover between the lines instead of
		// leaving it anywhere: 30px onto each 20px line, and the items stretch
		// to the lines they are on.
		{"normal", [][2]float64{{0, 50}, {0, 50}, {0, 50}, {50, 50}}},
		{"stretch", [][2]float64{{0, 50}, {0, 50}, {0, 50}, {50, 50}}},
		{"flex-start", [][2]float64{{0, 20}, {0, 20}, {0, 20}, {20, 20}}},
		{"flex-end", [][2]float64{{60, 20}, {60, 20}, {60, 20}, {80, 20}}},
		{"center", [][2]float64{{30, 20}, {30, 20}, {30, 20}, {50, 20}}},
		{"space-between", [][2]float64{{0, 20}, {0, 20}, {0, 20}, {80, 20}}},
		// Three shares of 20: half a share above the first line and below the
		// last, a whole one between them.
		{"space-around", [][2]float64{{15, 20}, {15, 20}, {15, 20}, {65, 20}}},
		// Three gaps of 20, all equal.
		{"space-evenly", [][2]float64{{20, 20}, {20, 20}, {20, 20}, {60, 20}}},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := flexCross(t, fourWide,
				wrapping+`#f { height: 100px; align-content: `+c.value+` }`)
			wantCross(t, got, c.want, "two lines aligned by "+c.value)
		})
	}
}

// TestAlignContentSaysNothingToAContainerThatDoesNotWrap. The property moves the
// lines of a container that has them, and a container that does not wrap has
// one line whose size is the container's own — there is nothing left for
// align-content to place, and the specification says so rather than leaving it
// to fall out.
//
// The difference is visible, which is why this is a test and not a comment: the
// same declaration on the same document leaves the items 100px tall without the
// wrap and 20px tall with it.
func TestAlignContentSaysNothingToAContainerThatDoesNotWrap(t *testing.T) {
	const two = `<div id="f"><div>a</div><div>b</div></div>`
	const sized = `#f { width: 300px; height: 100px; align-content: center }` +
		`#f > div { flex: 0 0 100px }`

	wantCross(t, flexCross(t, two, sized),
		[][2]float64{{0, 100}, {0, 100}}, "a container that does not wrap")

	// Allowed to wrap, the same container has a line of its own size again —
	// 20px of content — and the leftover is align-content's to place.
	wantCross(t, flexCross(t, two, sized+`#f { flex-wrap: wrap }`),
		[][2]float64{{40, 20}, {40, 20}}, "a container that wraps and did not need to")
}

// TestAReversedAxisRunsFromTheOtherEnd. flex-direction's two reversed values and
// "direction: rtl" all say the same thing — the axis starts at the far end — and
// they compose rather than override, so a right-to-left row-reverse runs left to
// right again.
//
// The items keep their order in the document and are laid out along the axis;
// what changes is which end that is. Reading the numbers back in the tree order,
// a reversed row's first item has the largest position.
func TestAReversedAxisRunsFromTheOtherEnd(t *testing.T) {
	back := [][2]float64{{288, 12}, {276, 12}, {264, 12}}
	forth := [][2]float64{{0, 12}, {12, 12}, {24, 12}}
	for _, c := range []struct {
		what string
		css  string
		want [][2]float64
	}{
		{"a row", ``, forth},
		{"a reversed row", `#f { flex-direction: row-reverse }`, back},
		{"a right-to-left row", `#f { direction: rtl }`, back},
		{"both at once", `#f { direction: rtl; flex-direction: row-reverse }`, forth},
	} {
		t.Run(c.what, func(t *testing.T) {
			wantRow(t, flexRow(t, threeItems, `#f { width: 300px }`+c.css), c.want, c.what)
		})
	}

	// The gaps go with it: 30px between each pair, measured from the right.
	wantRow(t, flexRow(t, threeItems,
		`#f { width: 300px; flex-direction: row-reverse; column-gap: 30px }`),
		[][2]float64{{288, 12}, {246, 12}, {204, 12}}, "a reversed row with a gap")

	// And a column-reverse stacks upwards: the 20px item ends up below the 40px
	// one, at the bottom of the 60px the two came to.
	wantCross(t, flexCross(t, twoHeights, `#f { width: 300px; flex-direction: column-reverse }`),
		[][2]float64{{40, 20}, {0, 40}}, "a reversed column")
}

// TestWhichEndAnAlignmentKeywordNames is the part of §6.2 that only shows up
// once an axis runs backwards: three families of keyword name the same two ends
// by different routes, and they part company there.
//
// "flex-start" is the end the items are laid out from, whichever end that is.
// "start" is the writing mode's, which flex-direction can turn away from but
// "rtl" cannot — because "rtl" moves the writing mode and the axis together.
// "left" is the page's own and answers to both at once.
func TestWhichEndAnAlignmentKeywordNames(t *testing.T) {
	// Read these as pictures: "packed left" is the three characters against the
	// left edge, whatever order they are in.
	left := [][2]float64{{24, 12}, {12, 12}, {0, 12}}
	right := [][2]float64{{288, 12}, {276, 12}, {264, 12}}
	for _, c := range []struct {
		what, css string
		want      [][2]float64
	}{
		{"flex-start in a reversed row is the right",
			`#f { flex-direction: row-reverse; justify-content: flex-start }`, right},
		{"flex-end in a reversed row is the left",
			`#f { flex-direction: row-reverse; justify-content: flex-end }`, left},
		{"start in a reversed row is still the inline start",
			`#f { flex-direction: row-reverse; justify-content: start }`, left},
		{"start in a right-to-left row moves with the writing mode",
			`#f { direction: rtl; justify-content: start }`, right},
		{"left is the left of the page in a reversed row",
			`#f { flex-direction: row-reverse; justify-content: left }`, left},
		{"left is the left of the page in a right-to-left row",
			`#f { direction: rtl; justify-content: left }`, left},
		{"right is the right of the page in a right-to-left row",
			`#f { direction: rtl; justify-content: right }`, right},
	} {
		t.Run(c.what, func(t *testing.T) {
			wantRow(t, flexRow(t, threeItems, `#f { width: 300px }`+c.css), c.want, c.what)
		})
	}
}

// TestTheLinesStackFromTheOtherEndUnderWrapReverse. "wrap-reverse" turns the
// cross axis round, so the first line is at the bottom and the last at the top —
// and every alignment across that axis turns with it, which is the whole reason
// the two ends are named twice in Box Alignment.
func TestTheLinesStackFromTheOtherEndUnderWrapReverse(t *testing.T) {
	const reversed = `#f { width: 300px; flex-wrap: wrap-reverse } #f > div { flex: 0 0 100px }`

	// Four items over two lines: the first three are on the lower line.
	wantCross(t, flexCross(t, fourWide, reversed),
		[][2]float64{{20, 20}, {20, 20}, {20, 20}, {0, 20}}, "two lines stacked upwards")

	// With room to spare, "flex-start" packs the lines at the end they are
	// stacked from, which is now the bottom.
	wantCross(t, flexCross(t, fourWide, reversed+`#f { height: 100px; align-content: flex-start }`),
		[][2]float64{{80, 20}, {80, 20}, {80, 20}, {60, 20}}, "lines packed at the flex start")

	// "start" is the block start and does not turn with the axis: the same two
	// lines, in the same order, at the top of the container.
	wantCross(t, flexCross(t, fourWide, reversed+`#f { height: 100px; align-content: start }`),
		[][2]float64{{20, 20}, {20, 20}, {20, 20}, {0, 20}}, "lines packed at the block start")
}

// TestAnItemIsAlignedAcrossTheAxisItsContainerReversed. The same two families of
// keyword, one line down: align-items names an end of the cross axis, and under
// wrap-reverse "flex-start" is the bottom of the line while "start" is the top.
func TestAnItemIsAlignedAcrossTheAxisItsContainerReversed(t *testing.T) {
	const reversed = `#f { width: 300px; flex-wrap: wrap-reverse }`

	wantCross(t, flexCross(t, twoHeights, reversed+`#f { align-items: flex-start }`),
		[][2]float64{{20, 20}, {0, 40}}, "an item at the flex start of its line")
	wantCross(t, flexCross(t, twoHeights, reversed+`#f { align-items: start }`),
		[][2]float64{{0, 20}, {0, 40}}, "an item at the block start of its line")
	wantCross(t, flexCross(t, twoHeights, reversed+`#f { align-items: flex-end }`),
		[][2]float64{{0, 20}, {0, 40}}, "an item at the flex end of its line")
	wantCross(t, flexCross(t, twoHeights, reversed+`#f { align-items: end }`),
		[][2]float64{{20, 20}, {0, 40}}, "an item at the block end of its line")

	// And with nothing said, the item is stretched, which has no end to name.
	wantCross(t, flexCross(t, twoHeights, reversed),
		[][2]float64{{0, 40}, {0, 40}}, "an item stretched across a reversed line")
}

// TestARightToLeftColumnTurnsOnlyTheAxisTheWritingModeIsOn. "rtl" is a
// declaration about the inline axis, and which of a container's two axes that
// is depends on its direction. In a column it is the cross axis: the items
// still stack downwards, and it is the side they are aligned to that moves.
//
// A container that turned both would stack its items from the bottom, which is
// what "column-reverse" is for and what nobody wrote here.
func TestARightToLeftColumnTurnsOnlyTheAxisTheWritingModeIsOn(t *testing.T) {
	const rtlColumn = `#f { width: 300px; flex-direction: column; direction: rtl }`

	wantCross(t, flexCross(t, twoHeights, rtlColumn),
		[][2]float64{{0, 20}, {20, 40}}, "a right-to-left column stacks downwards")

	// The cross axis is the one that turned: an item at the start of it is
	// against the right edge, where a left-to-right column would put it at 0.
	wantRow(t, flexRow(t, twoHeights, rtlColumn+`#f { align-items: flex-start }`),
		[][2]float64{{288, 12}, {288, 12}}, "items at the start of a right-to-left column")
	wantRow(t, flexRow(t, twoHeights,
		`#f { width: 300px; flex-direction: column; align-items: flex-start }`),
		[][2]float64{{0, 12}, {0, 12}}, "items at the start of a left-to-right column")

	// "end" is the other side of the same coin: the inline end of a
	// right-to-left column is the left edge, which is where a left-to-right one
	// puts its items at the *start*. Naming the two ends by the writing mode is
	// what makes that read the same way in both.
	wantRow(t, flexRow(t, twoHeights, rtlColumn+`#f { align-items: end }`),
		[][2]float64{{0, 12}, {0, 12}}, "items at the inline end of a right-to-left column")
}

const threeNamed = `<div id="f"><div id="a">a</div><div id="b">b</div><div id="c">c</div></div>`

// TestAnAutomaticMarginTakesTheFreeSpaceFirst is §9.5's first sentence, which
// runs before the packing and usually instead of it.
//
// An auto margin on the main axis takes what is left on the line, and where
// there are several they take an equal share each. It is the idiom for pushing
// one item to the far end of a toolbar and for centring one between its
// neighbours, and both are here.
func TestAnAutomaticMarginTakesTheFreeSpaceFirst(t *testing.T) {
	// All 264px of it goes to the left of the first item, so the row is pushed
	// against the far end.
	wantRow(t, flexRow(t, threeNamed, `#f { width: 300px } #a { margin-left: auto }`),
		[][2]float64{{264, 12}, {276, 12}, {288, 12}}, "the whole row pushed over")

	// On the second item it splits the row in two, which is the toolbar.
	wantRow(t, flexRow(t, threeNamed, `#f { width: 300px } #b { margin-left: auto }`),
		[][2]float64{{0, 12}, {276, 12}, {288, 12}}, "one item pushed away from the first")

	// Both margins of one item, and it is centred between the two ends.
	wantRow(t, flexRow(t, threeNamed,
		`#f { width: 300px } #b { margin-left: auto; margin-right: auto }`),
		[][2]float64{{0, 12}, {144, 12}, {288, 12}}, "one item centred between its neighbours")

	// Three margins share 264 equally: 88 each.
	wantRow(t, flexRow(t, threeNamed, `#f { width: 300px } #f > div { margin-left: auto }`),
		[][2]float64{{88, 12}, {188, 12}, {288, 12}}, "three margins sharing the free space")

	// A margin on the trailing side takes the same space from the other end,
	// which leaves the row where it started.
	wantRow(t, flexRow(t, threeNamed, `#f { width: 300px } #c { margin-right: auto }`),
		[][2]float64{{0, 12}, {12, 12}, {24, 12}}, "a trailing margin taking the space")
}

// TestAnAutomaticMarginLeavesNothingForJustifyContent. The two are the same
// budget spent twice, and §9.5 spends it in this order: the margins take the
// free space, and the packing is then handed a line with none.
//
// It is the surprising half of the property — "justify-content: center" on a
// container holding an auto margin does nothing at all — and it is the half a
// document depends on, because an item pushed to the far end has to stay there
// whatever the container was told to do with its slack.
func TestAnAutomaticMarginLeavesNothingForJustifyContent(t *testing.T) {
	wantRow(t, flexRow(t, threeNamed,
		`#f { width: 300px; justify-content: center } #b { margin-left: auto }`),
		[][2]float64{{0, 12}, {276, 12}, {288, 12}}, "centred against an automatic margin")

	// And nothing is left for the margin either, when §9.7 got there first: a
	// row of growing items has no free space by the time this is asked.
	wantRow(t, flexRow(t, threeNamed,
		`#f { width: 300px } #f > div { flex: 1 } #b { margin-left: auto }`),
		[][2]float64{{0, 100}, {100, 100}, {200, 100}}, "an automatic margin on a growing item")

	// A line with nothing to spare gives the margin nothing rather than
	// something negative: three 120px items overflow 300px, and the margin is
	// zero.
	wantRow(t, flexRow(t, threeNamed,
		`#f { width: 300px } #f > div { flex: 0 0 120px } #b { margin-left: auto }`),
		[][2]float64{{0, 120}, {120, 120}, {240, 120}}, "an automatic margin on an overfull line")
}

// TestAnAutomaticMarginAcrossTheLineAlignsTheItem is §9.6's version: the room
// left across the line goes to whichever cross-axis margin asked for it, and to
// both equally where both did.
//
// An item with one is not stretched, which is the part that has to be said out
// loud — a stretched item fills its line and has no leftover to give away, so
// the margin would silently do nothing.
func TestAnAutomaticMarginAcrossTheLineAlignsTheItem(t *testing.T) {
	// The line is 40px because of the two-line item beside it, and the 20px one
	// is pushed to the bottom of it.
	wantCross(t, flexCross(t, twoHeights, `#f { width: 300px } #a { margin-top: auto }`),
		[][2]float64{{20, 20}, {0, 40}}, "an item pushed down its line")
	wantCross(t, flexCross(t, twoHeights, `#f { width: 300px } #a { margin-bottom: auto }`),
		[][2]float64{{0, 20}, {0, 40}}, "an item held at the top of its line")
	wantCross(t, flexCross(t, twoHeights,
		`#f { width: 300px } #a { margin-top: auto; margin-bottom: auto }`),
		[][2]float64{{10, 20}, {0, 40}}, "an item centred in its line")

	// Without the margin the same item is stretched to the line, which is what
	// the margin is being asked instead of.
	wantCross(t, flexCross(t, twoHeights, `#f { width: 300px }`),
		[][2]float64{{0, 40}, {0, 40}}, "an item with no automatic margin")

	// An automatic margin has taken the room the alignment would have moved the
	// item through, so there is nothing left for align-self to do and the two
	// do not add up. §9.6 says the margin wins, and the arithmetic agrees: a
	// margin that took the leftover leaves an item that already fills its line.
	wantCross(t, flexCross(t, twoHeights,
		`#f { width: 300px } #a { margin-top: auto; align-self: flex-end }`),
		[][2]float64{{20, 20}, {0, 40}}, "an automatic margin against an alignment")
}

// TestAnAutomaticMarginKnowsWhichAxisItIsOn. A margin is on the main axis or the
// cross axis depending on which way the container runs, and the two rules are
// different — one shares a line's leftover between every margin that asked,
// the other gives one item the room across its own line.
func TestAnAutomaticMarginKnowsWhichAxisItIsOn(t *testing.T) {
	const column = `#f { width: 300px; height: 200px; flex-direction: column }`

	// Vertical, so main: the 140px left in the column goes below the first item
	// and pushes the second to the bottom.
	wantCross(t, flexCross(t, twoHeights, column+`#a { margin-bottom: auto }`),
		[][2]float64{{0, 20}, {160, 40}}, "a main-axis margin in a column")

	// Horizontal, so cross: the first item is not stretched across the column
	// and is pushed to the far side of it instead.
	wantRow(t, flexRow(t, twoHeights,
		`#f { width: 300px; flex-direction: column } #a { margin-left: auto }`),
		[][2]float64{{288, 12}, {0, 300}}, "a cross-axis margin in a column")
}

// TestOrderMovesAnItemAmongItsSiblings is §5.4. The property changes where an
// item sits and nothing else about it: its size, its alignment and its own
// content are what they were, and the document it came from is unchanged.
//
// The three items are 12, 24 and 36 wide, so each row below can be read back as
// a picture: the widths say which box is which, and the order they are listed in
// is the order the container holds them — which is the order they were laid out
// in, and the order they will be painted in.
func TestOrderMovesAnItemAmongItsSiblings(t *testing.T) {
	const named = `<div id="f"><div id="a">a</div><div id="b">bb</div><div id="c">ccc</div></div>`

	// Untouched: 12, 24 and 36 of Courier, in the order they were written.
	wantRow(t, flexRow(t, named, `#f { width: 300px }`),
		[][2]float64{{0, 12}, {12, 24}, {36, 36}}, "three items in document order")

	// The first item asked to come last, so it is laid out last: b and c move
	// up and a starts where the other two end.
	wantRow(t, flexRow(t, named, `#f { width: 300px } #a { order: 2 }`),
		[][2]float64{{0, 24}, {24, 36}, {60, 12}}, "the first item sent to the end")

	// A negative order brings an item forward, which is the only way to get in
	// front of a sibling that named nothing: the initial value is zero.
	wantRow(t, flexRow(t, named, `#f { width: 300px } #c { order: -1 }`),
		[][2]float64{{0, 36}, {36, 12}, {48, 24}}, "the last item brought to the front")
}

// TestItemsThatNameTheSameOrderKeepTheirOwn. The sort is stable, and that is
// the specification's requirement rather than an artefact: "order-modified
// document order" is document order for every pair that named the same value,
// which is what makes the property usable at all — an author who moves one item
// has not shuffled the rest.
func TestItemsThatNameTheSameOrderKeepTheirOwn(t *testing.T) {
	const named = `<div id="f"><div id="a">a</div><div id="b">bb</div><div id="c">ccc</div></div>`

	wantRow(t, flexRow(t, named, `#f { width: 300px } #a { order: 1 } #b { order: 1 }`),
		[][2]float64{{0, 36}, {36, 12}, {48, 24}}, "two items with the same order")

	// And a value that is not an integer is not a value: the declaration is
	// thrown out and the item keeps the initial zero, rather than being rounded
	// to a number nobody wrote.
	wantRow(t, flexRow(t, named, `#f { width: 300px } #a { order: 2.5 }`),
		[][2]float64{{0, 12}, {12, 24}, {36, 36}}, "an order with a fraction in it")
}

// flexIDs is which items the container holds, named, in the order it holds
// them — which is the order they were laid out in and the order they will be
// painted in.
func flexIDs(t *testing.T, htmlSrc, extra string) []string {
	t.Helper()
	root := layoutOf(t, 1000, htmlSrc, flexCSS+extra)
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				for _, c := range f.Children {
					id := ""
					if c.Box != nil && c.Box.Element != nil {
						id, _ = c.Box.Element.Attr("id")
					}
					out = append(out, id)
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

// TestAReorderedItemIsPaintedInTheOrderItIsLaidOut. §5.4 says the property
// affects painting as well as placement — the items are painted in
// order-modified document order — and here that is not a second mechanism but
// the same one: the fragments are made in the order they are laid out, and the
// painter walks them in the order they were made.
func TestAReorderedItemIsPaintedInTheOrderItIsLaidOut(t *testing.T) {
	got := flexIDs(t, `<div id="f"><div id="a">a</div><div id="b">b</div></div>`,
		`#f { width: 300px } #a { order: 1 }`)
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Errorf("the container's children are %v, and the item that asked to "+
			"come last is painted last", got)
	}
}

// TestTheOrderOfEqualItemsSurvivesAWholeContainerOfThem is the stability of the
// sort, at a size where it can be seen.
//
// A sort that is not stable will still put three items in the right order —
// small runs come out of any reasonable algorithm in the order they went in, so
// a test with three items proves nothing about the property. Fourteen is past
// the point where the sort stops being an insertion sort, and an unstable one
// shuffles the items that named the same value into an order nobody wrote.
func TestTheOrderOfEqualItemsSurvivesAWholeContainerOfThem(t *testing.T) {
	doc, css := `<div id="f">`, `#f { width: 1000px }`
	var want []string
	for i := 0; i < 14; i++ {
		name := string(rune('a' + i))
		doc += `<div id="` + name + `">x</div>`
		if i%2 == 0 {
			css += `#` + name + ` { order: 1 }`
		}
	}
	doc += `</div>`
	for i := 1; i < 14; i += 2 {
		want = append(want, string(rune('a'+i)))
	}
	for i := 0; i < 14; i += 2 {
		want = append(want, string(rune('a'+i)))
	}

	got := flexIDs(t, doc, css)
	if strings.Join(got, "") != strings.Join(want, "") {
		t.Errorf("the container holds %v, want %v — seven items named the same "+
			"order and the document put them in that one", got, want)
	}
}

// TestTextInsideAFlexContainerBecomesAnItemOfItsOwn is §4's anonymous flex item.
//
// A flex container has no inline formatting context of its own — every in-flow
// child is an item — so a run of text written straight inside one is put in a
// box the document does not contain. It is the commonest markup there is:
// "<div class=row>Total <span>9</span></div>" is two items, and before this the
// whole container was refused and stacked.
func TestTextInsideAFlexContainerBecomesAnItemOfItsOwn(t *testing.T) {
	// Three characters of Courier are 36px, and the element beside them is an
	// item of its own rather than part of the same box.
	wantRow(t, flexRow(t, `<div id="f">abc<div>d</div></div>`, `#f { width: 300px }`),
		[][2]float64{{0, 36}, {36, 12}}, "text beside an element")

	// A run is contiguous text and nothing else, so an element between two runs
	// makes two items and not one: three items here, in the order written.
	wantRow(t, flexRow(t, `<div id="f">ab<div>x</div>cd</div>`, `#f { width: 300px }`),
		[][2]float64{{0, 24}, {24, 12}, {36, 24}}, "an element between two runs of text")

	// An inline element is *not* wrapped with the text: it is an in-flow child,
	// so it is an item in its own right and blockified where it stands.
	wantRow(t, flexRow(t, `<div id="f">ab<span>x</span>cd</div>`, `#f { width: 300px }`),
		[][2]float64{{0, 24}, {24, 12}, {36, 24}}, "an inline element between two runs")
}

// TestWhiteSpaceBetweenItemsIsNotAnItem. Every document in the suite writes a
// newline between its elements, so a rule that made an item of every text node
// would make a row of three <div>s into seven items — four of them empty, each
// taking a share of the line and a gap.
//
// It is the collapsing that decides, not the characters: under "white-space:
// pre" the same space is content and does become an item.
func TestWhiteSpaceBetweenItemsIsNotAnItem(t *testing.T) {
	const spaced = "<div id=\"f\">\n  <div>a</div>\n  <div>b</div>\n</div>"
	wantRow(t, flexRow(t, spaced, `#f { width: 300px }`),
		[][2]float64{{0, 12}, {12, 12}}, "two items with newlines between them")

	// Preserved, each run between them is a text run like any other and the
	// row comes out five items. Their widths are the widest line each holds,
	// because a preserved newline is a line break: "\n  " is two spaces on a
	// second line and 24px wide, and the trailing "\n" is a break with nothing
	// after it and no width at all.
	wantRow(t, flexRow(t, spaced, `#f { width: 300px; white-space: pre }`),
		[][2]float64{{0, 24}, {24, 12}, {36, 24}, {60, 12}, {72, 0}},
		"two items with preserved space between them")
}

// TestAnAnonymousItemIsAnonymous. The box holds the text and nothing else: it
// inherits its parent's font, so the text is set the way the author wrote it,
// and it takes none of the parent's own padding, border or margin — a box that
// took those would draw the container's padding twice and be that much wider.
//
// It also cannot be selected, which is the specification's point and not an
// omission: "#f > div { flex: 1 }" moves the elements beside it and leaves the
// anonymous item at the size of its own text.
func TestAnAnonymousItemIsAnonymous(t *testing.T) {
	// Inherited: at 40px the three characters are 24px each.
	wantRow(t, flexRow(t, `<div id="f">abc<div>d</div></div>`,
		`#f { width: 300px; font-size: 40px }`),
		[][2]float64{{0, 72}, {72, 12}}, "an anonymous item at the container's size")

	// Not inherited: 100px of padding on the container is the container's.
	got := flexRow(t, `<div id="f">abc<div>d</div></div>`,
		`#f { width: 300px; padding-left: 100px }`)
	if len(got) != 2 || got[0].w != 36 {
		t.Errorf("the anonymous item is %v; it holds three characters and none "+
			"of its container's padding", got)
	}

	// Unselectable: the element grows into the whole of what is left and the
	// anonymous item stays as wide as its text.
	wantRow(t, flexRow(t, `<div id="f">abc<div>d</div></div>`,
		`#f { width: 300px } #f > div { flex: 1 }`),
		[][2]float64{{0, 36}, {36, 264}}, "a growing element beside an anonymous item")
}

// flexBaselines is where each item's first baseline ended up on the page, which
// is the thing baseline alignment is about and the only thing about it that can
// be stated without knowing a font's metrics.
func flexBaselines(t *testing.T, htmlSrc, extra string) []float64 {
	t.Helper()
	root := layoutOf(t, 1000, htmlSrc, flexCSS+extra)
	var out []float64
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "f" {
				for _, c := range f.Children {
					v, ok := firstBaseline(c)
					if !ok {
						v = c.BorderRect.H
					}
					out = append(out, c.BorderRect.Y.Add(v).Px())
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

// TestItemsOnABaselineLineUpTheirText is §9.4.8. The items are moved across the
// line until the first line of text in each sits on one line with the others,
// which is what a row of labels beside a heading needs and what no other
// alignment can do — every other one names an end of the item, and this names
// something inside it.
//
// The assertion is that the baselines meet, not where they are: where they are
// depends on the ascent of a font, and a test written against that number would
// be checking Courier rather than this file.
func TestItemsOnABaselineLineUpTheirText(t *testing.T) {
	const two = `<div id="f"><div id="a">a</div><div id="b">b</div></div>`
	const big = `#f { width: 300px; align-items: baseline }` +
		`#f > div#b { font-size: 40px; line-height: 40px }`

	got := flexBaselines(t, two, big)
	if len(got) != 2 || got[0] != got[1] {
		t.Errorf("the two baselines are at %v; the items are aligned to one", got)
	}

	// The item with the deeper baseline does not move: the group sits against
	// the start of the line rather than floating in the middle of it.
	wantCross(t, flexCross(t, two, big),
		[][2]float64{{15.546875, 20}, {0, 40}}, "a small item beside a large one")

	// Without the alignment they are both at the top and the baselines are not
	// on one line, which is the picture this is instead of.
	got = flexBaselines(t, two, `#f { width: 300px; align-items: flex-start }`+
		`#f > div#b { font-size: 40px; line-height: 40px }`)
	if len(got) != 2 || got[0] == got[1] {
		t.Errorf("the two baselines are at %v, and nothing asked them to meet", got)
	}
}

// TestAPaddingMovesAnItemsBaselineAndTheOthersFollow says the same thing in
// whole numbers.
//
// Ten pixels of padding above one item's text puts its baseline ten pixels
// lower, so every item aligned with it moves down by exactly ten — no font
// metric anywhere in the arithmetic, which is what makes this the test that
// pins the behaviour rather than the one that describes it.
func TestAPaddingMovesAnItemsBaselineAndTheOthersFollow(t *testing.T) {
	const two = `<div id="f"><div id="a">a</div><div id="b">b</div></div>`
	const opposite = `#f { width: 300px; align-items: baseline }` +
		`#f > div#a { padding-top: 10px } #f > div#b { padding-bottom: 30px }`

	wantCross(t, flexCross(t, two,
		`#f { width: 300px; align-items: baseline } #f > div#b { padding-top: 10px }`),
		[][2]float64{{10, 20}, {0, 30}}, "an item pushed down by another's padding")

	// And the line holds the deepest of each half, which need not be the same
	// item: one item is 10px deeper above its baseline and another 30px deeper
	// below, so the line is 60px while neither item is more than 50.
	wantCross(t, flexCross(t, two, opposite),
		[][2]float64{{0, 30}, {10, 50}}, "two items deeper on opposite sides")
	if h := flexHeight(t, two, opposite); h != 60 {
		t.Errorf("the line is %gpx and holds 10px above the deepest baseline and "+
			"30px below the shallowest", h)
	}
}

// TestAnItemWithNoTextIsAlignedByItsBottomEdge is §9.4.8's fallback: an item
// with no baseline of its own is given one at the end of its border box.
//
// It is the same rule an empty inline-block follows in a line of text — it sits
// on the baseline rather than straddling it — and it is the only answer that
// does not need to invent a font for a box that has none.
func TestAnItemWithNoTextIsAlignedByItsBottomEdge(t *testing.T) {
	// Two boxes with nothing in them, 30px and 50px: their bottom edges meet,
	// so the shallower one is pushed down by the difference.
	got := flexCross(t, `<div id="f"><div id="a"></div><div id="b"></div></div>`,
		`#f { width: 300px; align-items: baseline }`+
			`#f > div#a { height: 30px } #f > div#b { height: 50px }`)
	wantCross(t, got, [][2]float64{{20, 30}, {0, 50}}, "two empty boxes on a baseline")
}

// TestAMarginIsPartOfTheDistanceToABaseline. A baseline is measured from the
// start of the item's *margin* box, because that is where the item begins on
// the line — an item held 10px off the top of the line by a margin has its text
// 10px further down, and every item aligned with it follows.
func TestAMarginIsPartOfTheDistanceToABaseline(t *testing.T) {
	const two = `<div id="f"><div id="a">a</div><div id="b">b</div></div>`
	wantCross(t, flexCross(t, two,
		`#f { width: 300px; align-items: baseline } #f > div#b { margin-top: 10px }`),
		[][2]float64{{10, 20}, {10, 20}}, "an item held off the top by a margin")
}
