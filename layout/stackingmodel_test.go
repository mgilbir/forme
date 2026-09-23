package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// One model of who stacks where, used by every step of the paint. Each test
// here is a place where one painter asked its own question instead: opacity
// read the fragment tree and missed the inline boxes that have no fragment
// (audit C32), the outline pass walked the tree without the clips and without
// the lines (C95, C96), the stacking level read a z-index that does not apply
// (C97), and the gather did not know a flex or grid item from a block (C98).

// paintedRuns is every drawn run whose text contains s, in paint order.
func paintedRuns(ops []Op, s string) []DrawText {
	var out []DrawText
	for _, op := range ops {
		if r, ok := op.(DrawText); ok && strings.Contains(r.Text, s) {
			out = append(out, r)
		}
	}
	return out
}

// TestOpacityOnAnInlineDimsWhatItContains is CSS Color 4: opacity
// applies to every element, a <span> included, and what it dims is the span
// and everything in it. The span has no fragment of its own among the block's
// children, and a paint that only read fragments printed "opacity: 0" text at
// full strength.
func TestOpacityOnAnInlineDimsWhatItContains(t *testing.T) {
	const doc = `<p>aa <span id="s">secret</span> cc</p>`

	hidden := paintOf(t, doc, noDefaults+`#s { opacity: 0 }`)
	if got := paintedRuns(hidden, "secret"); len(got) != 0 {
		t.Errorf("\"opacity: 0\" on a span drew its text %d times, at alpha %v",
			len(got), got[0].Color.A)
	}
	if got := paintedRuns(hidden, "aa"); len(got) != 1 || got[0].Color.A != 1 {
		t.Errorf("the text outside the invisible span came out as %v, want one opaque run", got)
	}

	half := paintOf(t, doc, noDefaults+`#s { opacity: 0.5; background-color: #008000 }`)
	runs := paintedRuns(half, "secret")
	if len(runs) != 1 {
		t.Fatalf("%d runs of the span's text, want one\n%s", len(runs), sketchClips(half))
	}
	if runs[0].Color.A != 0.5 {
		t.Errorf("the span's text was drawn at alpha %v, want the span's 0.5", runs[0].Color.A)
	}
	// Its background is its fragment's, which hangs from the line and not from
	// the block — the other place a fragment walk does not reach.
	if got := soleAlpha(t, half, green, "a translucent span's background"); got != 0.5 {
		t.Errorf("the span's background was painted at alpha %v, want 0.5", got)
	}
	for _, r := range append(paintedRuns(half, "aa"), paintedRuns(half, "cc")...) {
		if r.Color.A != 1 {
			t.Errorf("the run %q outside the span was dimmed to %v", r.Text, r.Color.A)
		}
	}
}

// TestOpacityOnAnInlineReachesWhatHangsFromItsBlock: an inline-block inside a
// translucent span hangs from the block's fragment, as though the span were
// not there, and it is still the span's descendant. So is a float.
func TestOpacityOnAnInlineReachesWhatHangsFromItsBlock(t *testing.T) {
	ops := paintOf(t,
		`<p>aa <span id="s">b <span id="ib"></span> <span id="fl"></span></span></p>`,
		noDefaults+`#s { opacity: 0.5 }
		#ib { display: inline-block; width: 20px; height: 20px; background-color: #008000 }
		#fl { float: left; width: 20px; height: 20px; background-color: #ff0000 }`)
	if got := soleAlpha(t, ops, green, "an inline-block inside a translucent span"); got != 0.5 {
		t.Errorf("the inline-block was painted at alpha %v, want the span's 0.5", got)
	}
	if got := soleAlpha(t, ops, red, "a float inside a translucent span"); got != 0.5 {
		t.Errorf("the float was painted at alpha %v, want the span's 0.5", got)
	}
}

// TestNestedInlineOpacitiesMultiply: a translucent span inside a translucent
// span inside a translucent paragraph is three groups, and the words carry the
// product of all three.
func TestNestedInlineOpacitiesMultiply(t *testing.T) {
	ops := paintOf(t, `<p id="p">a <span id="o">b <span id="i">word</span></span></p>`,
		noDefaults+`#p { opacity: 0.5 } #o { opacity: 0.5 } #i { opacity: 0.5 }`)
	for _, c := range []struct {
		text string
		want float64
	}{{"a", 0.5}, {"b", 0.25}, {"word", 0.125}} {
		runs := paintedRuns(ops, c.text)
		if len(runs) == 0 {
			t.Fatalf("no run of %q\n%s", c.text, sketchClips(ops))
		}
		if got := runs[0].Color.A; got != c.want {
			t.Errorf("%q was drawn at alpha %v, want %v", c.text, got, c.want)
		}
	}
}

// TestASpanBrokenAroundABlockIsOneGroup: §9.2.1.1 makes three boxes of one
// <span> with a block inside it — the span's two halves and the block lifted
// out between them — and CSS Color 4's group is the element, not the box.
// Here the lifted block is pulled up over the first half's words, so as a
// group the green hides "a" and folded mark by mark "a" shows through it. That
// is only visible to a check that holds the three boxes' marks in one account;
// three accounts of one mark each say nothing.
func TestASpanBrokenAroundABlockIsOneGroup(t *testing.T) {
	got := composeOf(t,
		`<div id="w"><span id="s">a<div id="b"></div>c</span></div>`, Options{},
		noDefaults+`#w { padding-top: 40px } #s { opacity: 0.5 }
		#b { height: 30px; margin-top: -20px; background-color: #008000 }`)
	var said int
	for _, f := range got.Findings {
		if f.Property == "opacity" {
			said++
			if !strings.HasSuffix(f.Path, "span#s") {
				t.Errorf("the opacity finding names %s, want the span", f.Path)
			}
		}
	}
	if said != 1 {
		t.Errorf("%d opacity findings, want one about the span, whose block covers "+
			"its own first words: %v", said, got.Findings)
	}
	if a := soleAlpha(t, got.Ops, green, "the block lifted out of the span"); a != 0.5 {
		t.Errorf("the lifted block was painted at alpha %v, want 0.5", a)
	}
	for _, s := range []string{"a", "c"} {
		for _, r := range paintedRuns(got.Ops, s) {
			if r.Color.A != 0.5 {
				t.Errorf("the half %q was drawn at alpha %v, want 0.5", s, r.Color.A)
			}
		}
	}
}

// TestAnOutlineIsClippedLikeTheBoxItRings is §11.1.1: an "overflow: hidden"
// box clips everything its descendants render, and an outline is part of that
// rendering. The ring below sits wholly outside the clip, so none of it may
// reach the page — and the same box without the clip is the control.
func TestAnOutlineIsClippedLikeTheBoxItRings(t *testing.T) {
	const doc = `<div id="c"><div id="o"></div></div>`
	const base = noDefaults + `#c { width: 100px; height: 100px }
		#o { margin-left: 150px; width: 20px; height: 20px; outline: 5px solid #0000ff }`
	if got := fillsOf(paintOf(t, doc, base), blue); len(got) != 4 {
		t.Fatalf("the control drew %d outline bands, want 4", len(got))
	}
	if got := fillsOf(paintOf(t, doc, base+`#c { overflow: hidden }`), blue); len(got) != 0 {
		t.Errorf("an outline wholly outside an \"overflow: hidden\" box drew %v", got)
	}

	// And one the clip cuts: every band stops at the padding edge.
	cut := paintOf(t, doc, base+`#c { overflow: hidden } #o { margin-left: 90px }`)
	bands := fillsOf(cut, blue)
	if len(bands) == 0 {
		t.Fatal("an outline the clip only cuts was not drawn at all")
	}
	for _, r := range bands {
		if r.Right().Px() > 100 {
			t.Errorf("an outline band reaches %vpx, past the 100px clip", r.Right().Px())
		}
	}
}

// TestAnOutlineIsCutByItsOwnClip: §11.1.2's "clip" cuts the rendering of the
// box that declares it, its outline included.
func TestAnOutlineIsCutByItsOwnClip(t *testing.T) {
	ops := paintOf(t, `<div id="o"></div>`, noDefaults+
		`#o { position: absolute; left: 50px; top: 50px; width: 40px; height: 40px;
		      clip: rect(0, 40px, 40px, 0); outline: 5px solid #0000ff }`)
	if got := fillsOf(ops, blue); len(got) != 0 {
		t.Errorf("an outline outside the box's own clip rectangle drew %v", got)
	}
}

// TestAnInlineBoxIsOutlined is §18.4 on a <span>: a ring round each of its
// fragments, with or without a background to paint.
func TestAnInlineBoxIsOutlined(t *testing.T) {
	for _, extra := range []string{"", "background-color: #ffff00;"} {
		root := layoutOf(t, A4.Content().W.Px(),
			`<p>aa <span id="s">bb</span> cc</p>`,
			noDefaults+`#s { outline: 3px solid #0000ff; `+extra+` }`)
		ops := Paint(root)
		bands := fillsOf(ops, blue)
		if len(bands) != 4 {
			t.Errorf("with %q: %d outline bands, want the four of a ring\n%s",
				extra, len(bands), sketchClips(ops))
			continue
		}
		// The ring is outside the span's own fragment.
		frag := inlineFragmentOf(t, root, "s")
		for _, r := range bands {
			if !r.Intersect(frag.BorderRect).Empty() {
				t.Errorf("with %q: an outline band %v reaches inside the span's box %v",
					extra, r, frag.BorderRect)
			}
		}
	}
}

// TestAnInlineBoxsOutlineIsCutByItsBlock: an inline box clips nothing of its
// own, and what cuts its ring is what cuts its words — the content clip of the
// block whose line it is on. The span below starts at the block's left edge,
// so the left band of its ring lies wholly outside the block's padding box.
func TestAnInlineBoxsOutlineIsCutByItsBlock(t *testing.T) {
	const doc = `<div id="c"><span id="s">bb</span></div>`
	const base = noDefaults + `#c { margin-left: 20px; width: 50px }
		#s { outline: 5px solid #0000ff }`
	if got := fillsOf(paintOf(t, doc, base), blue); len(got) != 4 {
		t.Fatalf("the control drew %d outline bands, want 4", len(got))
	}
	for _, r := range fillsOf(paintOf(t, doc, base+`#c { overflow: hidden }`), blue) {
		if r.X.Px() < 20 {
			t.Errorf("an outline band at %v reaches left of the clipping block's edge at 20px", r)
		}
	}
}

func hasID(n *html.Node, id string) bool {
	got, ok := n.Attr("id")
	return ok && got == id
}

// inlineFragmentOf is the one fragment an inline element has, on whichever
// line it is.
func inlineFragmentOf(t *testing.T, root *Fragment, id string) *Fragment {
	t.Helper()
	got := inlineFragmentsOf(t, root, id)
	if len(got) != 1 {
		t.Fatalf("#%s has %d inline fragments, want one", id, len(got))
	}
	return got[0]
}

// inlineFragmentsOf is every fragment an inline element has, line by line.
func inlineFragmentsOf(t *testing.T, root *Fragment, id string) []*Fragment {
	t.Helper()
	var got []*Fragment
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		for _, l := range f.Lines {
			for _, b := range l.Boxes {
				if b.Box != nil && b.Box.Element != nil && hasID(b.Box.Element, id) {
					got = append(got, b)
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if len(got) == 0 {
		t.Fatalf("no inline fragment for #%s", id)
	}
	return got
}

// TestAnOutlineIsPaintedWithItsStackingContext is §E.2's step 10 read as it is
// written: the outlines "from this stacking context", as the last step of that
// context. The ring of a box at "z-index: -1" is part of that box's context,
// which is painted under the in-flow content of the context around it — so the
// green block over it covers its outline as it covers the box.
func TestAnOutlineIsPaintedWithItsStackingContext(t *testing.T) {
	ops := paintOf(t, `<div id="ctx"><div id="neg"></div><div id="over"></div></div>`,
		noDefaults+`#ctx { position: relative; z-index: 0 }
		#neg { position: absolute; z-index: -1; left: 10px; top: 10px;
		       width: 20px; height: 20px; outline: 5px solid #0000ff }
		#over { height: 60px; background-color: #008000 }`)
	order := paintOrder(ops)
	firstGreen, lastBlue := -1, -1
	blues := 0
	for i, c := range order {
		if c == green && firstGreen < 0 {
			firstGreen = i
		}
		if c == blue {
			lastBlue = i
			blues++
		}
	}
	if blues != 4 {
		t.Fatalf("%d outline bands, want one ring painted once: %v", blues, order)
	}
	if lastBlue > firstGreen {
		t.Errorf("the outline of a \"z-index: -1\" box was painted over the in-flow "+
			"block of the context around it: %v", order)
	}
}

// TestZIndexDoesNotApplyToAStaticBox is §9.9.1: z-index applies to positioned
// boxes, and a static block with an opacity is a stacking context at level 0
// whatever it declares: CSS Color 4 paints it where a positioned element
// with "z-index: 0" would be. The negative number put the red box behind the
// in-flow green block it follows.
func TestZIndexDoesNotApplyToAStaticBox(t *testing.T) {
	order := paintOrder(paintOf(t, `<div id="g"></div><div id="r"></div>`,
		noDefaults+`#g { background-color: #008000; height: 50px }
		#r { opacity: 0.5; z-index: -1; background-color: #ff0000; height: 50px;
		     margin-top: -25px }`))
	if len(order) != 2 {
		t.Fatalf("%d fills, want two: %v", len(order), order)
	}
	if order[0].G == 0 {
		t.Errorf("a static box's z-index was honoured: %v; it stacks at 0, over the "+
			"in-flow block before it", order)
	}
}

// TestAFlexOrGridItemsZIndexStacksIt is css-flexbox §4.3 and css-grid §9.5:
// z-index applies to a flex or grid item even when it is not positioned, and
// makes a stacking context of it. The red item comes first and asks to be
// painted over the green one it overlaps.
func TestAFlexOrGridItemsZIndexStacksIt(t *testing.T) {
	for _, display := range []string{"flex", "grid; grid-template-columns: 50px 100px"} {
		order := paintOrder(paintOf(t, `<div id="c"><div id="r"></div><div id="g"></div></div>`,
			noDefaults+`#c { display: `+display+` }
			#r { z-index: 2; background-color: #ff0000; margin-right: -50px;
			     width: 100px; height: 50px }
			#g { background-color: #008000; width: 100px; height: 50px }`))
		if len(order) != 2 {
			t.Fatalf("display: %s: %d fills, want two: %v", display, len(order), order)
		}
		if order[1].R == 0 {
			t.Errorf("display: %s: the item with \"z-index: 2\" was painted first, "+
				"under the item after it: %v", display, order)
		}
	}
}

// TestAFlexItemIsPaintedAtomically is the other half of that sentence: items
// "paint exactly the same as inline blocks", so each one's text goes with its
// own background, and the next item's background is painted over the text of
// the one before it rather than under it.
func TestAFlexItemIsPaintedAtomically(t *testing.T) {
	ops := paintOf(t, `<div id="c"><div id="a">first</div><div id="b"></div></div>`,
		noDefaults+`#c { display: flex }
		#a { width: 100px; margin-right: -60px }
		#b { width: 100px; height: 40px; background-color: #008000 }`)
	text, bg := -1, -1
	for i, op := range ops {
		switch v := op.(type) {
		case DrawText:
			if strings.Contains(v.Text, "first") {
				text = i
			}
		case FillRect:
			if v.Color == green {
				bg = i
			}
		}
	}
	if text < 0 || bg < 0 {
		t.Fatalf("the fixture drew the text at %d and the background at %d", text, bg)
	}
	if text > bg {
		t.Error("the first item's text was painted over the second item's " +
			"background; flex items paint atomically, like inline-blocks")
	}
}

// TestAFlexItemWithoutAZIndexHoistsWhatIsPositionedInIt: painted atomically is
// not a stacking context. A "z-index: -1" inside an item with no z-index of its
// own belongs to the context around the container, and goes behind its
// background.
func TestAFlexItemWithoutAZIndexHoistsWhatIsPositionedInIt(t *testing.T) {
	order := paintOrder(paintOf(t,
		`<div id="c"><div id="i"><div id="neg"></div></div></div>`,
		noDefaults+`#c { display: flex; background-color: #008000; height: 50px }
		#i { position: static; width: 50px; height: 50px }
		#neg { position: absolute; z-index: -1; width: 20px; height: 20px;
		       background-color: #ff0000 }`))
	if len(order) != 2 || order[0] != red {
		t.Errorf("the fills came out %v; the negative box is in the root's "+
			"context and goes behind the container's background", order)
	}

	sealed := paintOrder(paintOf(t,
		`<div id="c"><div id="i"><div id="neg"></div></div></div>`,
		noDefaults+`#c { display: flex; height: 50px }
		#i { z-index: 0; width: 50px; height: 50px; background-color: #008000 }
		#neg { position: absolute; z-index: -1; width: 20px; height: 20px;
		       background-color: #ff0000 }`))
	if len(sealed) != 2 || sealed[0] != green {
		t.Errorf("the fills came out %v; an item with \"z-index: 0\" is a stacking "+
			"context, so the negative box inside it goes over its background", sealed)
	}
}
