package layout

import (
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/style"
)

// An inline box at a stacking level of its own: CSS 2.1 §9.9 and Appendix E
// for a positioned one, CSS Color 4 for a translucent one. The paint used to
// put every mark on a line at step 6 with the block's text, whatever the box
// it was inside asked for (the deviation the painting-order commit recorded in
// inline.go). Each test here is one of the orderings that deviation got wrong,
// with the control that shows the fixture could tell.

// firstFill is the index of the first non-empty fill whose colour has rgb's
// channels, whatever its alpha, or -1.
func firstFill(ops []Op, rgb style.RGBA) int {
	for i, op := range ops {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() &&
			r.Color.R == rgb.R && r.Color.G == rgb.G && r.Color.B == rgb.B {
			return i
		}
	}
	return -1
}

// firstText is the index of the first run whose text is s, or -1.
func firstText(ops []Op, s string) int {
	for i, op := range ops {
		if r, ok := op.(DrawText); ok && strings.TrimSpace(r.Text) == s {
			return i
		}
	}
	return -1
}

// at requires every named mark to be painted, and returns where each was.
func marksAt(t *testing.T, ops []Op, what string, fills map[string]style.RGBA, texts ...string) map[string]int {
	t.Helper()
	got := map[string]int{}
	for name, c := range fills {
		if got[name] = firstFill(ops, c); got[name] < 0 {
			t.Fatalf("%s: no %s fill\n%s", what, name, sketchClips(ops))
		}
	}
	for _, s := range texts {
		if got[s] = firstText(ops, s); got[s] < 0 {
			t.Fatalf("%s: no run %q\n%s", what, s, sketchClips(ops))
		}
	}
	return got
}

// TestAPositionedSpanIsPaintedAtItsLevel is §E.2's steps 6 and 8: the inline
// content of a later block is step 6, and a positioned span — z-index auto or
// 0 — is step 8, after it, whatever the document order. With "z-index: 1" it
// is after a later positioned block at level 0 as well; at "auto" the two tie
// and document order puts the span first.
func TestAPositionedSpanIsPaintedAtItsLevel(t *testing.T) {
	const doc = `<div id="a"><span id="s">xx</span></div>` +
		`<div id="b"><span id="ib"></span></div><div id="c"></div>`
	const base = noDefaults + `body { font-family: Courier; font-size: 20px }
		#s { background-color: #008000 }
		#ib { display: inline-block; width: 40px; height: 20px; background-color: #ff0000 }
		#b { margin-top: -20px }
		#c { position: relative; height: 10px; margin-top: -20px; background-color: #0000ff }`
	colours := map[string]style.RGBA{"span": green, "later inline-block": red, "later positioned block": blue}

	static := marksAt(t, paintOf(t, doc, base), "static", colours, "xx")
	if !(static["span"] < static["later inline-block"] && static["xx"] < static["later inline-block"]) {
		t.Errorf("control: a static span is step 6 in tree order, before the later "+
			"block's inline-block; got %v", static)
	}
	auto := marksAt(t, paintOf(t, doc, base+`#s { position: relative }`), "auto", colours, "xx")
	if !(auto["later inline-block"] < auto["span"] && auto["later inline-block"] < auto["xx"]) {
		t.Errorf("a positioned span is step 8, after the later block's inline "+
			"content; got %v", auto)
	}
	if !(auto["xx"] < auto["later positioned block"]) {
		t.Errorf("a span and a block both at level 0 are in document order; got %v", auto)
	}
	one := marksAt(t, paintOf(t, doc, base+`#s { position: relative; z-index: 1 }`),
		"z-index 1", colours, "xx")
	if !(one["later positioned block"] < one["span"] && one["later positioned block"] < one["xx"]) {
		t.Errorf("a span at z-index 1 is painted after a block at level 0; got %v", one)
	}
}

// TestANegativeSpanIsPaintedUnderItsBlocksContent is §E.2 step 3: a stacking
// context with a negative z-index is painted after the root's background and
// before the in-flow blocks, so the paragraph's own background covers the
// span, words and all, while the paragraph's other words are painted over it.
func TestANegativeSpanIsPaintedUnderItsBlocksContent(t *testing.T) {
	const doc = `<div id="p">aa <span id="s">xx</span></div>`
	const base = noDefaults + `body { font-family: Courier; font-size: 20px }
		#p { background-color: #0000ff } #s { background-color: #008000 }`
	colours := map[string]style.RGBA{"span": green, "paragraph": blue}

	static := marksAt(t, paintOf(t, doc, base), "static", colours, "xx", "aa")
	if !(static["paragraph"] < static["span"] && static["paragraph"] < static["xx"]) {
		t.Errorf("control: a static span is painted over its paragraph's background; got %v", static)
	}
	neg := marksAt(t, paintOf(t, doc, base+`#s { position: relative; z-index: -1 }`),
		"z-index -1", colours, "xx", "aa")
	if !(neg["span"] < neg["paragraph"] && neg["xx"] < neg["paragraph"]) {
		t.Errorf("a span at z-index -1 is step 3, under the paragraph's background; got %v", neg)
	}
	if !(neg["paragraph"] < neg["aa"]) {
		t.Errorf("the paragraph's own words are still step 6, over its background; got %v", neg)
	}
}

// TestATranslucentSpanIsPaintedAsOneGroup is CSS Color 4's "painted at the
// stacking order a positioned element with z-index: 0 would have": every mark
// of the span — its words and an inline-block inside it — goes after the
// later block's inline content, together, with nothing of anyone else's
// between them. The paragraph's own words after the span are not in it.
func TestATranslucentSpanIsPaintedAsOneGroup(t *testing.T) {
	const doc = `<div id="a"><span id="s">xx<span id="ib"></span>zz</span> yy</div>` +
		`<div id="b"><span id="rb"></span></div>`
	const css = noDefaults + `body { font-family: Courier; font-size: 20px }
		#s { opacity: 0.5 }
		#ib { display: inline-block; width: 20px; height: 20px; background-color: #008000 }
		#b { margin-top: -20px }
		#rb { display: inline-block; width: 200px; height: 20px; background-color: #ff0000 }`
	ops := paintOf(t, doc, css)
	got := marksAt(t, ops, "translucent", map[string]style.RGBA{"inline-block": green, "later": red},
		"xx", "zz", "yy")
	if !(got["yy"] < got["later"]) {
		t.Errorf("the paragraph's own words are step 6, before the later block's; got %v", got)
	}
	first, last := len(ops), -1
	for _, k := range []string{"xx", "zz", "inline-block"} {
		first, last = min(first, got[k]), max(last, got[k])
	}
	if first < got["later"] {
		t.Errorf("the span's marks begin at %d, before the later block's inline "+
			"content at %d; a translucent span is painted at level 0, after step 6",
			first, got["later"])
	}
	for i := first; i <= last; i++ {
		switch v := ops[i].(type) {
		case DrawText:
			if s := strings.TrimSpace(v.Text); s != "" && s != "xx" && s != "zz" {
				t.Errorf("the run %q is painted inside the span's group", v.Text)
			}
		case FillRect:
			if v.Color.G != 128 {
				t.Errorf("the fill %v is painted inside the span's group", v)
			}
		}
	}
}

// TestASpanWithAZIndexSealsWhatItContains is §9.9.1: a z-index that is not auto
// makes the span a stacking context, so a "z-index: -1" box written inside it
// is painted inside the span's context — over the span's own background,
// under its words, and over the paragraph's background, which is outside it.
// Without the span's z-index the box is hoisted into the root's context and
// goes under the paragraph.
func TestASpanWithAZIndexSealsWhatItContains(t *testing.T) {
	const doc = `<div id="a">aa <span id="s">xx<span id="neg"></span></span></div>`
	const base = noDefaults + `body { font-family: Courier; font-size: 20px }
		#a { background-color: #0000ff }
		#s { background-color: #ffff00 }
		#neg { position: absolute; z-index: -1; width: 10px; height: 10px;
		       background-color: #008000 }`
	colours := map[string]style.RGBA{"paragraph": blue, "span": yellow, "inside": green}

	sealed := marksAt(t, paintOf(t, doc, base+`#s { position: relative; z-index: 1 }`),
		"sealed", colours, "xx")
	if !(sealed["paragraph"] < sealed["span"] && sealed["span"] < sealed["inside"] &&
		sealed["inside"] < sealed["xx"]) {
		t.Errorf("want the paragraph, then the span's own background, then the "+
			"negative box inside it, then its words; got %v", sealed)
	}
	if n := len(fillsOf(paintOf(t, doc, base+`#s { position: relative; z-index: 1 }`), yellow)); n != 1 {
		t.Errorf("the span's own background was painted %d times, want once", n)
	}
	hoisted := marksAt(t, paintOf(t, doc, base+`#s { position: relative }`),
		"not sealed", colours, "xx")
	if !(hoisted["inside"] < hoisted["paragraph"]) {
		t.Errorf("control: a span at z-index auto seals nothing, so the negative "+
			"box goes under the paragraph; got %v", hoisted)
	}
}

// TestASpanBrokenAroundABlockIsOneLevel: §9.2.1.1 makes two pieces of the span
// and a block between them, and §9.9 makes the element one stacking context.
// All three are painted together, after the later block's inline content,
// and inside that context in its own order: the lifted block's background
// (step 4) before the words either side of it (step 6).
func TestASpanBrokenAroundABlockIsOneLevel(t *testing.T) {
	const doc = `<div id="w"><span id="s">aa<div id="b"></div>cc</span></div>` +
		`<div id="later"><span id="rb"></span></div>`
	const css = noDefaults + `body { font-family: Courier; font-size: 20px }
		#s { position: relative; z-index: 1 }
		#b { height: 10px; background-color: #008000 }
		#later { margin-top: -50px }
		#rb { display: inline-block; width: 200px; height: 50px; background-color: #ff0000 }`
	ops := paintOf(t, doc, css)
	got := marksAt(t, ops, "split", map[string]style.RGBA{"block": green, "later": red}, "aa", "cc")
	if !(got["later"] < got["block"] && got["block"] < got["aa"] && got["block"] < got["cc"]) {
		t.Errorf("want the later block's content, then the span's context: its "+
			"block, then its words; got %v", got)
	}
}

// TestAnInlineLevelInsideOneThatDoesNotSealIsHoisted: a "position: relative"
// span with z-index auto paints its own words at level 0 but seals nothing,
// so a "z-index: 2" span inside it is sorted in the context around both — over
// a level-1 block that the outer span's words are under. With "z-index: 0" on
// the outer span the inner one is sealed in it, and goes under the block too.
func TestAnInlineLevelInsideOneThatDoesNotSealIsHoisted(t *testing.T) {
	const doc = `<div><span id="o">oo <span id="i">ii</span></span></div><div id="one"></div>`
	const base = noDefaults + `body { font-family: Courier; font-size: 20px }
		#i { position: relative; z-index: 2 }
		#one { position: relative; z-index: 1; height: 20px; margin-top: -20px;
		       background-color: #ff0000 }`
	colours := map[string]style.RGBA{"level 1": red}

	auto := marksAt(t, paintOf(t, doc, base+`#o { position: relative }`), "auto", colours, "oo", "ii")
	if !(auto["oo"] < auto["level 1"] && auto["level 1"] < auto["ii"]) {
		t.Errorf("want the outer span's words, the level-1 block, then the inner "+
			"span hoisted to level 2; got %v", auto)
	}
	zero := marksAt(t, paintOf(t, doc, base+`#o { position: relative; z-index: 0 }`),
		"zero", colours, "oo", "ii")
	if !(zero["ii"] < zero["level 1"]) {
		t.Errorf("control: sealed in a level-0 span, the inner span goes under the "+
			"level-1 block; got %v", zero)
	}
}

// TestPaintingFollowsOrderModifiedDocumentOrder is css-display-3 §3: flex and
// grid containers lay their items out in order-modified document order, and
// "this also affects the painting order, exactly as if the flex/grid items were
// reordered in the source document". Two positioned items at one level are
// painted in that order, and so is a positioned box inside an item that is not
// one: it moves with its item. An absolutely positioned child is treated as
// "order: 0".
func TestPaintingFollowsOrderModifiedDocumentOrder(t *testing.T) {
	for _, display := range []string{"flex", "grid"} {
		const items = `<div id="c"><div id="one"><div id="k"></div></div><div id="two"></div></div>`
		base := noDefaults + `#c { display: ` + display + ` }
			#one, #two, #k { width: 20px; height: 20px }
			#one { order: 2; position: relative; background-color: #008000 }
			#two { order: 1; position: relative; background-color: #ff0000 }
			#k { position: relative; background-color: #0000ff }`
		got := marksAt(t, paintOf(t, items, base), display,
			map[string]style.RGBA{"one": green, "two": red, "inside one": blue})
		if !(got["two"] < got["one"] && got["one"] < got["inside one"]) {
			t.Errorf("%s: want the order-1 item, then the order-2 item, then the "+
				"box inside it; got %v", display, got)
		}
		// The same with the items stacking contexts, which seal what they
		// contain: the tie between two level-1 items.
		got = marksAt(t, paintOf(t, items, base+`#one, #two { z-index: 1 }`), display+" z-index 1",
			map[string]style.RGBA{"one": green, "two": red})
		if !(got["two"] < got["one"]) {
			t.Errorf("%s: two items at z-index 1 are painted in order-modified "+
				"document order; got %v", display, got)
		}
		// An absolutely positioned child is order 0 whatever it declares, so it
		// goes after an item of order -1 that the document puts after it.
		got = marksAt(t, paintOf(t, `<div id="c"><div id="abs"></div><div id="item"></div></div>`,
			noDefaults+`#c { display: `+display+`; position: relative }
			#abs { position: absolute; order: -5; width: 20px; height: 20px;
			       background-color: #ff0000 }
			#item { order: -1; position: relative; width: 20px; height: 20px;
			        background-color: #008000 }`), display+" abspos",
			map[string]style.RGBA{"item": green, "abspos": red})
		if !(got["item"] < got["abspos"]) {
			t.Errorf("%s: an absolutely positioned child is order 0, after an "+
				"order -1 item; got %v", display, got)
		}
	}
	// Without an order, document order is what it always was.
	got := marksAt(t, paintOf(t,
		`<div id="c"><div id="one"></div><div id="two"></div></div>`,
		noDefaults+`#c { display: flex }
		#one, #two { position: relative; width: 20px; height: 20px }
		#one { background-color: #008000 } #two { background-color: #ff0000 }`), "control",
		map[string]style.RGBA{"one": green, "two": red})
	if !(got["one"] < got["two"]) {
		t.Errorf("control: items with no order are painted in document order; got %v", got)
	}
}

// TestCompareOrderReadsTheContainerItIsAbout pins the key comparison on its
// own, where each case is one reading of "as if reordered in the source".
func TestCompareOrderReadsTheContainerItIsAbout(t *testing.T) {
	f, g := &Box{Inner: InnerFlex}, &Box{Inner: InnerFlex}
	for _, c := range []struct {
		what string
		a, b []orderStep
		want int
	}{
		{"the same container: the order decides before the document",
			[]orderStep{{f, 2, 1}, {nil, 0, 1}}, []orderStep{{f, 1, 5}, {nil, 0, 5}}, 1},
		{"the same order: the document decides",
			[]orderStep{{f, 1, 1}, {nil, 0, 1}}, []orderStep{{f, 1, 5}, {nil, 0, 5}}, -1},
		{"two containers: their places in the document decide, not the orders",
			[]orderStep{{f, 9, 1}, {nil, 0, 1}}, []orderStep{{g, 0, 5}, {nil, 0, 5}}, -1},
		{"an item before its own descendant",
			[]orderStep{{f, 3, 4}, {nil, 0, 4}}, []orderStep{{f, 3, 4}, {nil, 0, 6}}, -1},
		{"a box outside the container, after it in the document",
			[]orderStep{{f, -5, 4}, {nil, 0, 4}}, []orderStep{{nil, 0, 9}}, -1},
	} {
		got := compareOrder(c.a, c.b)
		if (got < 0) != (c.want < 0) || (got > 0) != (c.want > 0) {
			t.Errorf("%s: compareOrder = %d, want the sign of %d", c.what, got, c.want)
		}
	}
}

// TestAnInlineOutlineBrokenAcrossLinesIsOneShape is CSS UI 4 §5: an element
// broken across several lines should have "an outline or minimum set of
// outlines that encloses all the element's boxes", each part "fully
// connected". Set solid, the pieces on two lines whose rings overlap make one
// shape: nothing of it crosses into a piece, no point of it is painted twice
// (a translucent outline would be darker there), and what it covers is the
// union of the rings' outer rectangles less the pieces. The union is computed
// here cell by cell, from the fragments, independently of the painter.
func TestAnInlineOutlineBrokenAcrossLinesIsOneShape(t *testing.T) {
	const doc = `<div id="w"><span id="s">aaa bbb ccc</span></div>`
	const base = noDefaults + `#w { width: 60px; font-family: Courier; font-size: 20px }
		#s { outline: 4px solid #0000ff }`

	root := layoutOf(t, 400, doc, base+`#w { line-height: 1 }`)
	pieces := inlineFragmentsOf(t, root, "s")
	if len(pieces) != 3 {
		t.Fatalf("the span is on %d lines, want 3", len(pieces))
	}
	w, _ := style.FromPx(4)
	outer := func(r Rect) Rect { return Rect{r.X.Sub(w), r.Y.Sub(w), r.W.Add(w).Add(w), r.H.Add(w).Add(w)} }
	if outer(pieces[0].BorderRect).Intersect(outer(pieces[1].BorderRect)).Empty() {
		t.Fatalf("the fixture's rings do not meet, so there is nothing to join: %v %v",
			pieces[0].BorderRect, pieces[1].BorderRect)
	}
	fills := fillsOf(Paint(root), blue)
	for i, a := range fills {
		for _, pc := range pieces {
			if !a.Intersect(pc.BorderRect).Empty() {
				t.Errorf("the outline fill %v crosses into the piece %v", a, pc.BorderRect)
			}
		}
		for _, b := range fills[i+1:] {
			if !a.Intersect(b).Empty() {
				t.Errorf("the outline fills %v and %v overlap, so that part is painted twice", a, b)
			}
		}
	}
	// The area of ∪outer − ∪inner, over the grid every edge makes.
	var xs, ys []style.Unit
	for _, pc := range pieces {
		for _, r := range []Rect{pc.BorderRect, outer(pc.BorderRect)} {
			xs, ys = append(xs, r.X, r.Right()), append(ys, r.Y, r.Bottom())
		}
	}
	xs, ys = sortedUnique(xs), sortedUnique(ys)
	in := func(r Rect, x, y style.Unit) bool { return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom() }
	var want float64
	for i := 0; i+1 < len(xs); i++ {
		for j := 0; j+1 < len(ys); j++ {
			x, y := xs[i], ys[j]
			var inOuter, inInner bool
			for _, pc := range pieces {
				inOuter = inOuter || in(outer(pc.BorderRect), x, y)
				inInner = inInner || in(pc.BorderRect, x, y)
			}
			if inOuter && !inInner {
				want += xs[i+1].Sub(x).Px() * ys[j+1].Sub(y).Px()
			}
		}
	}
	var got float64
	for _, f := range fills {
		got += f.W.Px() * f.H.Px()
	}
	if diff := got - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("the outline covers %.2f px², want the %.2f of the joined shape", got, want)
	}

	// Set far apart, the rings do not meet, and each piece keeps the four
	// bands of its own ring.
	apart := layoutOf(t, 400, doc, base+`#w { line-height: 3 }`)
	if got := fillsOf(Paint(apart), blue); len(got) != 12 {
		t.Errorf("three pieces whose rings do not meet drew %d bands, want 12", len(got))
	}
}

func sortedUnique(v []style.Unit) []style.Unit {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
	out := v[:0]
	for i, x := range v {
		if i == 0 || x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

// TestJoiningOutlinesIsCharged: the join compares pieces with pieces, which a
// document controls, so it is charged to the work budget. Refused, the pieces
// are drawn a ring each, as they were before they were joined, and the
// document is told.
func TestJoiningOutlinesIsCharged(t *testing.T) {
	root := layoutOf(t, 400, `<div id="w"><span id="s">aaa bbb ccc</span></div>`,
		noDefaults+`#w { width: 60px; font-family: Courier; font-size: 20px; line-height: 1 }
		#s { outline: 4px solid #0000ff }`)
	pieces := inlineFragmentsOf(t, root, "s")
	rec := NewRecorder(nil)
	// Room for every mark, and none for the join: charge keeps the reserve,
	// and chargeMark may spend it.
	rec.work = workBudget{left: 1 << 20, reserve: 1<<20 - 1}
	p := &painter{colors: map[string]style.RGBA{}, rec: rec}
	p.joinedOutline(pieces)
	if got := fillsOf(p.ops, blue); len(got) != 4*len(pieces) {
		t.Errorf("refused, the join drew %d bands, want a ring of four for each of "+
			"the %d pieces", len(got), len(pieces))
	}
	requireCut(t, rec.Findings(), "the joining of outlines broken across lines past that point")
}

// TestInlineLevelsCostTheMarks: every mark is listed once, under the one level
// that paints it, so a paragraph of many positioned spans, nested, costs the
// marks and not the marks times the levels. A walk of the block's lines per
// level, or of the nesting per mark, is quadratic here.
func TestInlineLevelsCostTheMarks(t *testing.T) {
	measure := func(root *Fragment) time.Duration {
		best := time.Duration(1 << 62)
		for i := 0; i < 3; i++ {
			start := time.Now()
			Paint(root)
			best = min(best, time.Since(start))
		}
		return best
	}
	for _, c := range []struct {
		what string
		doc  func(n int) string
		n    int
	}{
		// Side by side on one block: a walk of the block's lines per level.
		{"side by side", func(n int) string {
			return "<p>" + strings.Repeat(`<span class="r">w </span>`, n) + "</p>"
		}, 300},
		// Nested plain inline boxes inside one level, inside the parser's
		// depth bound: a walk of the nesting per mark.
		{"nested", func(n int) string {
			return `<p><span class="r">` + strings.Repeat(`<i>w `, n) + "</p>"
		}, 50},
	} {
		at := func(n int) *Fragment {
			return layoutOf(t, 400, c.doc(n), noDefaults+`.r { position: relative; z-index: 1 }`)
		}
		small, large := at(c.n), at(4*c.n)
		ratio := float64(measure(large)) / float64(measure(small))
		t.Logf("%s: %.1fx the time for 4x the spans", c.what, ratio)
		if ratio > 8 {
			t.Errorf("%s: four times the spans took %.1f times as long to paint; "+
				"linear is about 4 and quadratic about 16", c.what, ratio)
		}
	}
}

// TestEverythingInAnInlineLevelIsPaintedOnce. Four walks decide what an
// inline level holds and who paints it — the pre-pass, gather, hoist and the
// outline walk — and a mark the level holds that a walk also paints is drawn
// twice, while one each walk thinks another paints is drawn nowhere. Neither
// looks like a stacking bug on the page, so each is counted: the span's
// background and outline, a float and a positioned box inside it, an
// inline-block, an absolutely positioned box and its outline, and a block
// lifted out of the span, for each kind of level, inside each kind of box that
// is painted as a unit; and a span that holds nothing but its words.
func TestEverythingInAnInlineLevelIsPaintedOnce(t *testing.T) {
	magenta, cyan := style.RGBA{R: 255, B: 255, A: 1}, style.RGBA{G: 255, B: 255, A: 1}
	orange := style.RGBA{R: 255, G: 165, A: 1}
	const doc = `<div id="w"><span id="s">aa<span id="fl"><span id="in"></span></span>` +
		`<span id="ib"></span><span id="ab"></span><div id="bl"></div>cc</span></div>`
	// A span with nothing in it but words, whose level only its block's lines
	// can say is there.
	const words = `<div id="w"><span id="s">aa</span></div>`
	const base = noDefaults + `body { font-family: Courier; font-size: 20px }
		#s { background-color: #008000; outline: 2px solid #ff00ff }
		#fl { float: left; width: 10px; height: 10px; background-color: #ff0000 }
		#in { position: relative; display: inline-block; width: 5px; height: 5px;
		      background-color: #ffa500 }
		#ib { display: inline-block; width: 10px; height: 10px; background-color: #0000ff }
		#ab { position: absolute; width: 10px; height: 10px; background-color: #ffff00;
		      outline: 2px solid #00ffff }
		#bl { height: 10px; background-color: #808080 }`
	grey := style.RGBA{R: 128, G: 128, B: 128, A: 1}
	for _, level := range []string{
		`position: relative`, `position: relative; z-index: 1`,
		`position: relative; z-index: -1`, `opacity: 0.5`,
	} {
		for _, wrapper := range []string{
			``, `float: left; width: 300px`, `display: inline-block; width: 300px`,
			`position: relative`, `position: relative; z-index: 3`,
		} {
			ops := paintOf(t, doc, base+`#s { `+level+` } #w { `+wrapper+` }`)
			for _, c := range []struct {
				what string
				rgb  style.RGBA
				want int
			}{
				{"the span's background (one per piece)", green, 2},
				{"the float", red, 1}, {"the inline-block", blue, 1},
				{"the absolutely positioned box", yellow, 1}, {"the lifted block", grey, 1},
				{"the absolutely positioned box's outline", cyan, 4},
				{"the positioned box inside the float", orange, 1},
			} {
				if got := len(alphasOf(ops, c.rgb)); got != c.want {
					t.Errorf("%q in %q: %s drew %d fills, want %d",
						level, wrapper, c.what, got, c.want)
				}
			}
			if got := len(alphasOf(ops, magenta)); got == 0 || got > 8 {
				t.Errorf("%q in %q: the span's outline drew %d fills, want the "+
					"rings of its two pieces", level, wrapper, got)
			}
			ops = paintOf(t, words, base+`#s { `+level+` } #w { `+wrapper+` }`)
			if got := len(alphasOf(ops, green)); got != 1 {
				t.Errorf("%q in %q: a span of words drew its background %d times, want once",
					level, wrapper, got)
			}
		}
	}
}
