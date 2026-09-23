package layout

import (
	"strings"
	"testing"
)

// css-display-3's grammar, every value the cascade accepts, and the box each
// one builds (audit C92). The single keywords were read and one fixed-order
// two-word form, and everything else — "flow", "flex inline", "list-item
// block" — fell through to "inline" with nothing said.

// builtBox builds a document and returns the box of #x.
func builtBox(t *testing.T, htmlSrc, cssSrc string) *Box {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	return findBoxWhere(t, built.Root, func(b *Box) bool {
		if b.Element == nil {
			return false
		}
		id, _ := b.Element.Attr("id")
		return id == "x"
	})
}

func TestEveryDisplayValueBuildsTheBoxItNames(t *testing.T) {
	for _, tc := range []struct {
		value    string
		outer    Outer
		inner    Inner
		listItem bool
	}{
		// Inside values alone default the outside to block (§2.1).
		{"flow", OuterBlock, InnerFlow, false},
		{"flow-root", OuterBlock, InnerFlowRoot, false},
		{"flex", OuterBlock, InnerFlex, false},
		{"grid", OuterBlock, InnerGrid, false},
		{"table", OuterBlock, InnerTable, false},
		// Outside values alone default the inside to flow.
		{"block", OuterBlock, InnerFlow, false},
		{"inline", OuterInline, InnerFlow, false},
		// Both, in either order.
		{"block flow", OuterBlock, InnerFlow, false},
		{"inline flow-root", OuterInline, InnerFlowRoot, false},
		{"flow-root inline", OuterInline, InnerFlowRoot, false},
		{"flex inline", OuterInline, InnerFlex, false},
		{"inline grid", OuterInline, InnerGrid, false},
		{"grid inline", OuterInline, InnerGrid, false},
		{"table inline", OuterInline, InnerTable, false},
		{"flow-root block", OuterBlock, InnerFlowRoot, false},
		// And list items, in any order and with either inside value.
		{"list-item", OuterBlock, InnerFlow, true},
		{"block list-item", OuterBlock, InnerFlow, true},
		{"list-item block", OuterBlock, InnerFlow, true},
		{"list-item flow-root", OuterBlock, InnerFlowRoot, true},
		{"flow list-item block", OuterBlock, InnerFlow, true},
		// The legacy single keywords, unchanged.
		{"inline-block", OuterInline, InnerFlowRoot, false},
		{"inline-flex", OuterInline, InnerFlex, false},
		{"inline-grid", OuterInline, InnerGrid, false},
		{"inline-table", OuterInline, InnerTable, false},
		// audit C131: the legacy flexible box is a formatting-context root.
		{"-webkit-box", OuterBlock, InnerFlowRoot, false},
	} {
		b := builtBox(t, `<div><span id="x">text</span></div>`,
			`#x { display: `+tc.value+` }`)
		outer := b.Outer
		if b.Parent != nil && b.Parent.TableWrapper {
			// §17.4: a table's outer display is its wrapper's, and the table
			// box inside is always block-level within it.
			outer = b.Parent.Outer
		}
		if outer != tc.outer || b.Inner != tc.inner || b.ListItem != tc.listItem {
			t.Errorf("display: %s built outer %v inner %v list item %v, want %v %v %v",
				tc.value, outer, b.Inner, b.ListItem, tc.outer, tc.inner, tc.listItem)
		}
	}
}

// TestADisplayValueLaidOutAsSomethingElseIsReported is the other half: a value
// the engine recognises and lays out as something else says so, and the values
// it lays out as asked say nothing.
func TestADisplayValueLaidOutAsSomethingElseIsReported(t *testing.T) {
	for _, tc := range []struct {
		value    string
		reported bool
	}{
		{"run-in", true},
		{"run-in flow-root", true},
		{"inline list-item", true},
		{"inline flow-root list-item", true},
		{"list-item inline", true},
		{"flow", false},
		{"flex inline", false},
		{"list-item block", false},
		{"inline flow-root", false},
	} {
		got := unlaidFindings(t, `<div><span id="x">text</span></div>`,
			`#x { display: `+tc.value+` }`)
		if (len(got) > 0) != tc.reported {
			t.Errorf("display: %s reported %v; want reported=%v", tc.value, got, tc.reported)
		}
	}
	// An inline list item's marker is not drawn, so it is not built as a list
	// item: a marker with no block layout to place it would be a list item in
	// name only.
	if b := builtBox(t, `<div><span id="x">text</span></div>`,
		`#x { display: inline list-item }`); b.ListItem || b.Outer != OuterInline {
		t.Errorf("display: inline list-item built outer %v list item %v", b.Outer, b.ListItem)
	}
}

// TestAListItemWithAnotherInsideIsNotADisplay: §2.3's <display-listitem> takes
// "flow" or "flow-root" as its inside value and nothing else. "list-item flex"
// is not a display value, so the cascade drops it and says so, and the element
// keeps the display it had.
func TestAListItemWithAnotherInsideIsNotADisplay(t *testing.T) {
	for _, value := range []string{"list-item flex", "grid list-item", "list-item table"} {
		b := builtBox(t, `<div id="x">text</div>`, `#x { display: `+value+` }`)
		if b.Outer != OuterBlock || b.Inner != InnerFlow || b.ListItem {
			t.Errorf("display: %s was applied: outer %v inner %v list item %v; the "+
				"<div> should have kept its block", value, b.Outer, b.Inner, b.ListItem)
		}
		got := Compose(Input{HTML: `<div id="x">text</div>`,
			CSS: []Stylesheet{{Source: `#x { display: ` + value + ` }`}}}, Options{})
		reported := false
		for _, f := range got.Findings {
			if f.Property == "display" && f.Rule == RuleInvalidCSS {
				reported = true
			}
		}
		if !reported {
			t.Errorf("display: %s was dropped without a finding", value)
		}
	}
}

// TestAWebkitBoxContainsItsChildrensMargins is audit C131: the legacy flexible
// box is, in every browser, a container that its children's margins do not
// escape. Read as a plain block, a paragraph's top margin collapsed out through
// it and moved it down. It is now laid out exactly as a flow root is.
func TestAWebkitBoxContainsItsChildrensMargins(t *testing.T) {
	const doc = `<div style="border-top:1px solid"><div id="w"><p id="p">text</p></div></div>`
	at := func(display string) (Rect, Rect) {
		root := layoutOf(t, 600, doc, noDefaults+`#w { display: `+display+`;
			-webkit-box-orient: vertical; -webkit-line-clamp: 2 }
			#p { margin: 30px 0 }`)
		return find(t, root, "w").BorderRect, find(t, root, "p").BorderRect
	}
	w, p := at("-webkit-box")
	fw, fp := at("flow-root")
	if w != fw || p != fp {
		t.Errorf("-webkit-box put the box at %v and the paragraph at %v; a flow root "+
			"puts them at %v and %v", w, p, fw, fp)
	}
	if got := p.Y.Sub(w.Y); got != upx(t, 30) {
		t.Errorf("the paragraph is %v below the top of the box; its margin is inside "+
			"the box, so it is 30px", got)
	}
}

// overflow: clip (audit C91). CSS Overflow 3: "Unlike hidden, this value does
// not cause the element to establish a new formatting context", and a box that
// clips is not a scroll container.

// TestAClipBoxIsNotAFormattingContext: beside a float, a formatting-context root
// is narrowed and moved past the float; an ordinary block is not moved at all,
// and only its lines are shortened. overflow: clip is the second.
func TestAClipBoxIsNotAFormattingContext(t *testing.T) {
	const doc = `<div style="float:left; width:50px; height:100px"></div>` +
		`<div id="c">text</div>`
	root := layoutOf(t, 600, doc, noDefaults+`#c { overflow: clip }`)
	c := find(t, root, "c")
	if c.BorderRect.X != 0 || c.BorderRect.W != upx(t, 600) {
		t.Errorf("the clip box is at x=%v, %v wide; it is not a formatting context, "+
			"so it stays at the left edge and full width", c.BorderRect.X, c.BorderRect.W)
	}
	if len(c.Lines) == 0 || c.Lines[0].Rect.X != upx(t, 50) {
		t.Errorf("the clip box's first line is not shortened by the float: %v", c.Lines)
	}
	// And hidden still is one: the containment half, so the test cannot pass by
	// an engine that made nothing a formatting context.
	root = layoutOf(t, 600, doc, noDefaults+`#c { overflow: hidden }`)
	if c := find(t, root, "c"); c.BorderRect.X != upx(t, 50) {
		t.Errorf("overflow: hidden put the box at x=%v; it is a formatting context "+
			"and belongs past the float", c.BorderRect.X)
	}
	// Nor does it contain its children's margins: a child's top margin
	// collapses through it, as it would through any block.
	root = layoutOf(t, 600, `<div id="c"><p id="p">x</p></div>`,
		noDefaults+`#c { overflow: clip } #p { margin-top: 20px }`)
	if c, p := find(t, root, "c"), find(t, root, "p"); p.BorderRect.Y != c.BorderRect.Y {
		t.Errorf("the child's margin did not collapse through the clip box: box at %v, "+
			"child at %v", c.BorderRect.Y, p.BorderRect.Y)
	}
}

// TestAClipOnOneAxisLeavesTheOtherOpen: "overflow-x: clip" with "overflow-y"
// visible cuts the content at the left and right padding edges and not at the
// top or bottom — beside "clip", "visible" stays visible (§3.1), where beside a
// scrolling value it would compute to "auto".
func TestAClipOnOneAxisLeavesTheOtherOpen(t *testing.T) {
	clipOf := func(css string) (Clip, Rect) {
		root := layoutOf(t, 600, `<div id="c" style="height:20px"><div style="height:200px">x</div></div>`,
			noDefaults+css)
		c := find(t, root, "c")
		return c.clipContent, c.PaddingRect()
	}
	got, pad := clipOf(`#c { overflow-x: clip }`)
	if !got.Active || got.Rect.X != pad.X || got.Rect.W != pad.W {
		t.Errorf("overflow-x: clip clips to %v; want the padding box's left and right, %v", got, pad)
	}
	if got.Rect.Y >= pad.Y || got.Rect.Bottom() <= pad.Bottom().Add(upx(t, 180)) {
		t.Errorf("overflow-x: clip also cut the content vertically: %v inside a padding box %v", got, pad)
	}
	// Both clip, or clip beside hidden: the padding box on both axes.
	for _, css := range []string{`#c { overflow: clip }`, `#c { overflow-x: clip; overflow-y: hidden }`,
		`#c { overflow-x: visible; overflow-y: hidden }`} {
		if got, pad := clipOf(css); !got.Active || got.Rect != pad {
			t.Errorf("%s clips to %v; want the padding box %v", css, got, pad)
		}
	}
}

// TestAClippingFlexItemKeepsItsAutomaticMinimum: Flexbox §4.5 gives the
// automatic minimum of nothing to a scroll container, and clip is not one, so
// an item with "overflow-x: clip" is still as wide as its longest word.
func TestAClippingFlexItemKeepsItsAutomaticMinimum(t *testing.T) {
	word := strings.Repeat("W", 60)
	doc := `<div id="f"><div id="a">` + word + `</div><div id="b">x</div></div>`
	base := noDefaults + `#f { display: flex; width: 300px; font-family: Courier;
		font-size: 10px }
		#a { flex: 1 1 0 }
		#b { flex: 0 0 50px }`
	for _, tc := range []struct {
		overflow string
		shrinks  bool
	}{
		{"overflow-x: clip", false},
		{"overflow: clip", false},
		{"overflow-x: hidden", true},
	} {
		got, ok := gridRect(t, doc, base+`#a { `+tc.overflow+` }`, "a")
		if !ok {
			t.Fatalf("%s: the item generated no fragment", tc.overflow)
		}
		if shrank := got.W.Px() == 250; shrank != tc.shrinks {
			t.Errorf("%s: the item is %.0f wide; shrinks to the room left = %v, want %v",
				tc.overflow, got.W.Px(), shrank, tc.shrinks)
		}
	}
}

// inline-flex and inline-grid take their baseline from their first item
// (audit C93): Flexbox §8.5 and Grid §11.8 give a flex or grid container a
// first baseline set, which is its first item's — the first in the first row,
// for a grid — and not the last line box an inline-block's is.

func TestAnInlineFlexSitsOnItsFirstItem(t *testing.T) {
	const css = noDefaults + `#d { font-family: Courier; font-size: 20px; line-height: 20px }
		#f { display: inline-flex; flex-direction: column }`
	for _, extra := range []string{"", "#f { overflow: hidden }"} {
		root := layoutOf(t, 600, `<div id="d">x<span id="f"><span id="one">one</span>`+
			`<span>two</span><span>three</span></span>y</div>`, css+extra)
		line := baselineOfFirstRun(t, root, "d")
		one := find(t, root, "one")
		want := one.ContentRect().Y.Add(one.Lines[0].Rect.Y).Add(one.Lines[0].Baseline)
		if line != want {
			t.Errorf("%q: the line's baseline is at %v and the first item's at %v; an "+
				"inline flex container sits on its first item", extra, line, want)
		}
	}
}

func TestAnInlineGridSitsOnTheFirstItemInItsFirstRow(t *testing.T) {
	// The first item in the document is placed in the second row, and is set in
	// a larger size, so that taking it — or the last line of the grid — puts
	// the baseline somewhere else.
	root := layoutOf(t, 600, `<div id="d">x<span id="g"><span id="late">late</span>`+
		`<span id="first">first</span></span>y</div>`,
		noDefaults+`#d { font-family: Courier; font-size: 20px; line-height: 20px }
		#g { display: inline-grid }
		#late { grid-row: 2; font-size: 40px; line-height: 40px }
		#first { grid-row: 1 }`)
	line := baselineOfFirstRun(t, root, "d")
	first := find(t, root, "first")
	want := first.ContentRect().Y.Add(first.Lines[0].Rect.Y).Add(first.Lines[0].Baseline)
	if line != want {
		t.Errorf("the line's baseline is at %v and the first-row item's at %v", line, want)
	}
}
